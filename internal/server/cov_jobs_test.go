package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ POST /api/jobs ============

// waitJobTerminal 轮询等待 Job 落终态（异步派发下创建返回 pending，
// 终态由后台 goroutine 回写）
func waitJobTerminal(t *testing.T, s *Server, id string) *storage.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := s.db.GetJob(id)
		if err == nil && job.Status != storage.JobStatusPending && job.Status != storage.JobStatusRunning {
			return job
		}
		time.Sleep(5 * time.Millisecond)
	}
	job, err := s.db.GetJob(id)
	if err != nil {
		t.Fatalf("job %s never reached terminal state (gone)", id)
	}
	t.Fatalf("job %s stuck in %s", id, job.Status)
	return nil
}

// TestJobsCreateSuccess 经假 agent（job.exec 应答）验证创建-异步派发-终态回写
// 全链。断言透传 method/params、201 即回 pending、输出落库、actor 无 auth
// 上下文时为 unknown。
func TestJobsCreateSuccess(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	var gotParams map[string]interface{}
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		gotParams = params
		return map[string]interface{}{"exit_code": 0, "output": "hello from agent", "error": ""}, ""
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"a1","parameters":{"command":"uptime"}}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}

	var v jobView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v.ID == "" || v.Status != storage.JobStatusPending {
		t.Fatalf("view should be created pending, got %+v", v)
	}

	// 终态后断言透传（派发是异步的，创建返回时可能尚未触达 agent）
	waitJobTerminal(t, s, v.ID)
	if gotMethod != "job.exec" {
		t.Fatalf("method = %q, want job.exec", gotMethod)
	}
	if gotParams["command"] != "uptime" {
		t.Fatalf("params = %+v", gotParams)
	}

	// 库内终态（含 Actor/时间戳）
	job := waitJobTerminal(t, s, v.ID)
	if job.Status != "success" || job.ExitCode != 0 || job.Actor != "unknown" {
		t.Fatalf("stored job = %+v", job)
	}
	if job.FinishedAt == nil || job.StartedAt == nil {
		t.Fatalf("timestamps missing: %+v", job)
	}
	if !strings.Contains(job.Output, "hello from agent") {
		t.Fatalf("output not persisted: %q", job.Output)
	}
}

// TestJobsCreateNonZeroExitFails 非零退出码 → failed，输出仍带回。
func TestJobsCreateNonZeroExitFails(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 3, "output": "NOK", "error": ""}, ""
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"a1","parameters":{"command":"false"}}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var v jobView
	json.Unmarshal(rec.Body.Bytes(), &v)
	job := waitJobTerminal(t, s, v.ID)
	if job.Status != "failed" || job.ExitCode != 3 || job.Output != "NOK" {
		t.Fatalf("stored job = %+v", job)
	}
}

// TestJobsCreateNilData 成功应答但 data 为空 → success（空 map 兜底路径）。
func TestJobsCreateNilData(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, ""
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"a1","parameters":{"command":"true"}}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var v jobView
	json.Unmarshal(rec.Body.Bytes(), &v)
	job := waitJobTerminal(t, s, v.ID)
	if job.Status != "success" || job.Output != "" {
		t.Fatalf("stored job = %+v", job)
	}
}

// TestJobsCreateBareError agent 报错但无错误讯息 → 兜底文案落库。
func TestJobsCreateBareError(t *testing.T) {
	s := newBackupTestServer(t)
	// withFakeBackupAgent 空 rpcErr 即成功；这里复刻为「error 状态 + 空 error 字段」
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{{Type: "backup"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	go func() {
		for reqMsg := range agent.Send {
			if reqMsg.Type != protocol.MessageTypeRPCRequest {
				continue
			}
			resp := protocol.NewMessage(protocol.MessageTypeRPCResponse,
				map[string]interface{}{"status": "error"})
			resp.ID = reqMsg.ID
			s.handleRPCResponse(resp)
		}
	}()
	t.Cleanup(func() { agent.Close() })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"a1","parameters":{"command":"true"}}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var v jobView
	json.Unmarshal(rec.Body.Bytes(), &v)
	job := waitJobTerminal(t, s, v.ID)
	if job.Status != "failed" || !strings.Contains(job.Error, "agent rejected the operation") {
		t.Fatalf("stored job = %+v", job)
	}
}

// TestJobsCreateAgentRPCError agent 侧 RPC 报错 → failed，错误讯息落库。
func TestJobsCreateAgentRPCError(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "permission denied"
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"a1","parameters":{"command":"rm -rf /"}}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var v jobView
	json.Unmarshal(rec.Body.Bytes(), &v)
	job := waitJobTerminal(t, s, v.ID)
	if job.Status != "failed" || !strings.Contains(job.Error, "permission denied") {
		t.Fatalf("stored job = %+v", job)
	}
}

// TestJobsCreateValidation 入参校验各 400 分支。
func TestJobsCreateValidation(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)

	cases := []struct {
		name string
		body string
	}{
		{"missing type", `{"target":"a1","parameters":{"command":"x"}}`},
		{"unknown type", `{"type":"nope","target":"a1","parameters":{"command":"x"}}`},
		{"missing target", `{"type":"agent.exec","parameters":{"command":"x"}}`},
		{"missing command", `{"type":"agent.exec","target":"a1","parameters":{}}`},
		{"oversized command", `{"type":"agent.exec","target":"a1","parameters":{"command":"` + strings.Repeat("a", 17*1024) + `"}}`},
		{"timeout zero", `{"type":"agent.exec","target":"a1","parameters":{"command":"x","timeout_s":0}}`},
		{"timeout too large", `{"type":"agent.exec","target":"a1","parameters":{"command":"x","timeout_s":301}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(c.body))
			s.handleJobsAPI(rec, req, "/jobs")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
			}
		})
	}

	// 坏 JSON
	rec := httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(`{not json`)), "/jobs")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json code = %d", rec.Code)
	}
}

// TestJobsCreateAgentOffline 目标 agent 离线 → 503（不建记录）。
func TestJobsCreateAgentOffline(t *testing.T) {
	s := newBackupTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"ghost","parameters":{"command":"uptime"}}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}
	// 无幽灵记录
	jobs, _ := s.db.ListJobs(10)
	if len(jobs) != 0 {
		t.Fatalf("jobs should be empty, got %d", len(jobs))
	}
}

// ============ GET /api/jobs[/{id}] ============

// TestJobsListAndGet 列表/详情/404/405 全路径。
func TestJobsListAndGet(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	// 先跑一条供列表/详情
	rec := httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"a1","parameters":{"command":"uptime"}}`)), "/jobs")
	var created jobView
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == "" {
		t.Fatalf("created = %+v", created)
	}
	waitJobTerminal(t, s, created.ID)

	// 列表
	rec = httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodGet, "/api/jobs", nil), "/jobs")
	if rec.Code != http.StatusOK {
		t.Fatalf("list code = %d", rec.Code)
	}
	var list struct {
		Jobs []jobView `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Jobs) != 1 {
		t.Fatalf("list = %+v (err %v)", list, err)
	}
	// 详情带 output 与 parameters 回显
	if list.Jobs[0].ID != created.ID || list.Jobs[0].Output != "ok" || list.Jobs[0].Parameters["command"] != "uptime" {
		t.Fatalf("list[0] = %+v", list.Jobs[0])
	}

	// 详情
	rec = httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodGet, "/api/jobs/"+created.ID, nil), "/jobs/"+created.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail code = %d", rec.Code)
	}
	var detail jobView
	json.Unmarshal(rec.Body.Bytes(), &detail)
	if detail.ID != created.ID || detail.Status != "success" {
		t.Fatalf("detail = %+v", detail)
	}

	// 不存在 → 404
	rec = httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodGet, "/api/jobs/nope", nil), "/jobs/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("404 code = %d", rec.Code)
	}

	// 方法不允许 → 405
	rec = httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodDelete, "/api/jobs/"+created.ID, nil), "/jobs/"+created.ID)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("405 code = %d", rec.Code)
	}
}

// TestJobsEmptyList 空库列表形状 {"jobs":[]}。
func TestJobsEmptyList(t *testing.T) {
	s := newBackupTestServer(t)
	rec := httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodGet, "/api/jobs", nil), "/jobs")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"jobs":[]`) {
		t.Fatalf("empty list: code=%d body=%s", rec.Code, rec.Body.String())
	}
	// /jobs 空 id → 404
	rec = httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodGet, "/api/jobs/", nil), "/jobs/")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}

	// /jobs 未知方法 → 404
	rec = httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodPut, "/api/jobs", nil), "/jobs")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

// TestJobsListFilter 过滤参数（status/workflow_run_id）。
func TestJobsListFilter(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	// 一条独立 Job（跑完）+ 一条挂 workflow run 的
	rec := httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"a1","parameters":{"command":"uptime"}}`)), "/jobs")
	var solo jobView
	json.Unmarshal(rec.Body.Bytes(), &solo)
	waitJobTerminal(t, s, solo.ID)

	runJob := &storage.Job{ID: "j-run", Type: "agent.exec", Target: "a1",
		Actor: "workflow", Parameters: `{"command":"x"}`, Status: storage.JobStatusSuccess,
		WorkflowRunID: "run-1", CreatedAt: time.Now()}
	if err := s.db.CreateJob(runJob); err != nil {
		t.Fatalf("seed run job: %v", err)
	}

	// status 过滤：cancelled 应为空
	rec = httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodGet, "/api/jobs?status=cancelled", nil), "/jobs")
	var list struct {
		Jobs []jobView `json:"jobs"`
	}
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Jobs) != 0 {
		t.Fatalf("cancelled filter should be empty, got %+v", list.Jobs)
	}

	// workflow_run_id 过滤只中挂 run 的
	rec = httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodGet, "/api/jobs?workflow_run_id=run-1", nil), "/jobs")
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Jobs) != 1 || list.Jobs[0].ID != "j-run" {
		t.Fatalf("run filter = %+v", list.Jobs)
	}

	// status=success 过滤两条都在
	rec = httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodGet, "/api/jobs?status=success", nil), "/jobs")
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Jobs) != 2 {
		t.Fatalf("success filter = %+v", list.Jobs)
	}
}

// ============ POST /api/jobs/{id}/cancel ============

// TestJobsCancelPending pending Job 可撤：cancelled 终态 + 记录保留。
func TestJobsCancelPending(t *testing.T) {
	s := newBackupTestServer(t)
	// 挂一个假 agent 但 handler 阻塞直到放行——保住 pending 窗口
	release := make(chan struct{})
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		<-release
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	rec := httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"a1","parameters":{"command":"uptime"}}`)), "/jobs")
	var created jobView
	json.Unmarshal(rec.Body.Bytes(), &created)

	// 创建后立即取消——dispatch goroutine 还堵在 handler 里，但 dispatchJob
	// 已把状态推到 running？不确定，所以两种情况都合法：
	//   - 仍是 pending → 200 cancelled
	//   - 已 running → 409
	// 为确定性断言，直接播种一条 pending Job 测 cancel 路径：
	pending := &storage.Job{ID: "j-pending", Type: "agent.exec", Target: "a1",
		Actor: "u", Parameters: `{"command":"x"}`, Status: storage.JobStatusPending,
		CreatedAt: time.Now()}
	if err := s.db.CreateJob(pending); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rec = httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodPost, "/api/jobs/j-pending/cancel", nil), "/jobs/j-pending/cancel")
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel code = %d, body: %s", rec.Code, rec.Body.String())
	}
	job, _ := s.db.GetJob("j-pending")
	if job.Status != storage.JobStatusCancelled || job.FinishedAt == nil {
		t.Fatalf("cancelled job = %+v", job)
	}

	close(release)
}

// TestJobsCancelReject 非 pending 拒绝：终态 409、404；running 409 走的是
// agent 不可达路径（a1 未注册，CallAgent 返 ErrAgentNotFound——C6 绝不假称
// 已停）。running+agent 在线的取消成功路径见 TestJobsCancelRunning。
func TestJobsCancelReject(t *testing.T) {
	s := newBackupTestServer(t)
	seed := func(id, status string) {
		if err := s.db.CreateJob(&storage.Job{ID: id, Type: "agent.exec", Target: "a1",
			Actor: "u", Parameters: `{"command":"x"}`, Status: status,
			CreatedAt: time.Now()}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("j-run", storage.JobStatusRunning)
	seed("j-done", storage.JobStatusSuccess)

	for _, c := range []struct {
		id   string
		want int
	}{
		{"j-run", http.StatusConflict},
		{"j-done", http.StatusConflict},
		{"j-nope", http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		s.handleJobsAPI(rec, httptest.NewRequest(http.MethodPost, "/api/jobs/"+c.id+"/cancel", nil), "/jobs/"+c.id+"/cancel")
		if rec.Code != c.want {
			t.Fatalf("cancel %s code = %d, want %d", c.id, rec.Code, c.want)
		}
	}
	// GET /cancel 方法不允许
	rec := httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodGet, "/api/jobs/j-run/cancel", nil), "/jobs/j-run/cancel")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("405 code = %d", rec.Code)
	}
}

// withFakeJobAgent 注册一个 job.* 假 agent（M2c）：job.cancel 一律即时应答
// cancelled=true（镜像真 agent 的并发派发——exec 阻塞时 cancel 帧仍可被
// 处理），其余方法（job.exec）走 execHandler，可阻塞。withFakeBackupAgent
// 的 handler 循环是串行的，exec 堵住后 cancel 会排队，测不了 M2c 语义。
func withFakeJobAgent(t *testing.T, s *Server, agentID string, execHandler func(params map[string]interface{}) (interface{}, string)) {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Capabilities = []protocol.Capability{{Type: "backup"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	respond := func(reqMsg *protocol.Message, data interface{}, rpcErr string) {
		payload := map[string]interface{}{"status": "success", "data": data}
		if rpcErr != "" {
			payload = map[string]interface{}{"status": "error", "error": rpcErr}
		}
		resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, payload)
		resp.ID = reqMsg.ID
		s.handleRPCResponse(resp)
	}
	go func() {
		for reqMsg := range agent.Send {
			if reqMsg.Type != protocol.MessageTypeRPCRequest {
				continue
			}
			method, _ := reqMsg.Payload["method"].(string)
			params, _ := reqMsg.Payload["params"].(map[string]interface{})
			if method == "job.cancel" {
				respond(reqMsg, map[string]interface{}{"cancelled": true}, "")
				continue
			}
			// job.exec 等其余方法：execHandler 可能阻塞——独立 goroutine 跑，
			// 让后续 cancel 帧仍能被本循环读取应答
			go func(reqMsg *protocol.Message, params map[string]interface{}) {
				var data interface{}
				var rpcErr string
				if execHandler != nil {
					data, rpcErr = execHandler(params)
				}
				respond(reqMsg, data, rpcErr)
			}(reqMsg, params)
		}
	}()
	t.Cleanup(func() { agent.Close() })
}

// waitJobStatus 轮询 Job 到达期望状态（M2c 测试时序锚点）
func waitJobStatus(t *testing.T, s *Server, id string, want ...string) *storage.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := s.db.GetJob(id)
		if err == nil {
			for _, w := range want {
				if job.Status == w {
					return job
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s never reached status %v", id, want)
	return nil
}

// TestJobsCancelRunning M2c C6 主路径：running Job 经 job.cancel RPC 击杀 →
// 200 + cancelled 终态
func TestJobsCancelRunning(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeJobAgent(t, s, "a1", func(params map[string]interface{}) (interface{}, string) {
		// exec 不会被走到（直接播种 running Job），防御性返回
		return map[string]interface{}{"exit_code": 0, "output": "x", "error": ""}, ""
	})
	if err := s.db.CreateJob(&storage.Job{ID: "j-run-cancel", Type: "agent.exec", Target: "a1",
		Actor: "u", Parameters: `{"command":"x"}`, Status: storage.JobStatusRunning,
		CreatedAt: time.Now()}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rec := httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodPost, "/api/jobs/j-run-cancel/cancel", nil), "/jobs/j-run-cancel/cancel")
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel running code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var view jobView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Status != storage.JobStatusCancelled {
		t.Fatalf("view status = %s, want cancelled", view.Status)
	}
	job, _ := s.db.GetJob("j-run-cancel")
	if job.Status != storage.JobStatusCancelled || job.FinishedAt == nil {
		t.Fatalf("job after cancel = %+v", job)
	}
}

// TestJobsCancelPendingRaceFallsToRunning M2c C6 翻转竞态的确定性覆盖：
// 库内行已是 running，调用方仍持 pending 视图（GetJob 与条件 UPDATE 之间
// dispatch 启动的窗口）→ 条件 UPDATE 0 行 → 重读按 running 重走 → RPC
// 击杀成功落 cancelled。直调 cancelJob 构造，不经 handler——handler 层
// GetJob→cancelJob 之间的微秒竞态不可确定性命中（同 requireAgentRclone
// 直调先例）。
func TestJobsCancelPendingRaceFallsToRunning(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeJobAgent(t, s, "a1", nil)
	seed := &storage.Job{ID: "j-race", Type: "agent.exec", Target: "a1",
		Actor: "u", Parameters: `{"command":"x"}`, Status: storage.JobStatusRunning,
		CreatedAt: time.Now()}
	if err := s.db.CreateJob(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// stale pending 视图（真实竞态中 handler 手里的旧快照）
	stale := &storage.Job{ID: "j-race", Type: "agent.exec", Target: "a1",
		Status: storage.JobStatusPending}
	if err := s.cancelJob(stale); err != nil {
		t.Fatalf("cancelJob with stale pending view: %v", err)
	}
	job, _ := s.db.GetJob("j-race")
	if job.Status != storage.JobStatusCancelled {
		t.Fatalf("job status = %s, want cancelled (race fell through to running path)", job.Status)
	}
}

// TestJobsCancelRunningExecOverwriteGuard M2c C7：击杀后 exec 回写不得把
// cancelled 覆盖成 failed——被杀 exec 的输出补进台账，状态保持 cancelled。
func TestJobsCancelRunningExecOverwriteGuard(t *testing.T) {
	s := newBackupTestServer(t)
	release := make(chan struct{})
	withFakeJobAgent(t, s, "a1", func(params map[string]interface{}) (interface{}, string) {
		<-release
		// 被击杀后的 exec 回程：非零退出 + killed 讯息
		return map[string]interface{}{"exit_code": -1, "output": "partial output", "error": "signal: killed"}, ""
	})

	rec := httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"a1","parameters":{"command":"sleep 300"}}`)), "/jobs")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var created jobView
	json.Unmarshal(rec.Body.Bytes(), &created)

	// 等 dispatch 把状态推到 running（fake agent 的 exec handler 阻塞中）
	waitJobStatus(t, s, created.ID, storage.JobStatusRunning)

	// 取消：fake agent 即时应答 cancel，server 落 cancelled
	rec = httptest.NewRecorder()
	s.handleJobsAPI(rec, httptest.NewRequest(http.MethodPost, "/api/jobs/"+created.ID+"/cancel", nil), "/jobs/"+created.ID+"/cancel")
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel code = %d, body: %s", rec.Code, rec.Body.String())
	}

	// 放行 exec 回程（killed 结果）→ dispatchJobAsync 的 C7 守卫应保留 cancelled
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := s.db.GetJob(created.ID)
		if err == nil && job.Output != "" {
			break // MarkJobCancelledOutput 已回写
		}
		time.Sleep(5 * time.Millisecond)
	}
	job, _ := s.db.GetJob(created.ID)
	if job.Status != storage.JobStatusCancelled {
		t.Fatalf("status = %s, want cancelled (exec overwrite guard)", job.Status)
	}
	if job.Output != "partial output" || job.ExitCode != -1 {
		t.Fatalf("cancelled job output not recorded: %+v", job)
	}
}

// TestJobsCancelRunningStaleViewAfterFinish C6 击杀与回写竞态的确定性覆盖：
// 库内行已是终态（exec 先到落了 success），调用方仍持 running 视图 →
// RPC 击杀应答成功但条件 UPDATE 0 行 → 如实拒绝（409 文案），不得改写
// 真实终态。直调 cancelJob 构造，同 TestJobsCancelPendingRaceFallsToRunning。
func TestJobsCancelRunningStaleViewAfterFinish(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeJobAgent(t, s, "a1", nil)
	seed := &storage.Job{ID: "j-fin", Type: "agent.exec", Target: "a1",
		Actor: "u", Parameters: `{"command":"x"}`, Status: storage.JobStatusSuccess,
		CreatedAt: time.Now()}
	if err := s.db.CreateJob(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// stale running 视图（真实竞态中 handler 手里的旧快照）
	stale := &storage.Job{ID: "j-fin", Type: "agent.exec", Target: "a1",
		Status: storage.JobStatusRunning}
	if err := s.cancelJob(stale); err == nil {
		t.Fatal("cancelJob with stale running view should reject (already finished)")
	}
	job, _ := s.db.GetJob("j-fin")
	if job.Status != storage.JobStatusSuccess {
		t.Fatalf("job status = %s, want success (real terminal state preserved)", job.Status)
	}
}

// TestDispatchJobNilParamsInjectsID C1：payload.Parameters 为 nil 时 dispatchJob
// 仍须注入 Job ID（workflow 步骤参数经校验恒非 nil，standalone 走 handler 校验
// 亦非 nil；直调构造兜底分支，确认不 panic 且 id 到达 agent）
func TestDispatchJobNilParamsInjectsID(t *testing.T) {
	s := newBackupTestServer(t)
	var gotID string
	var sawParams bool
	withFakeJobAgent(t, s, "a1", func(params map[string]interface{}) (interface{}, string) {
		sawParams = true
		gotID, _ = params["id"].(string)
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})
	job := &storage.Job{ID: "j-nil", Type: "agent.exec", Target: "a1"}
	ok, jobErr := s.dispatchJob(job, jobCreatePayload{Type: "agent.exec", Target: "a1"})
	if !ok || jobErr.Message != "" {
		t.Fatalf("dispatch = %v/%+v, want success", ok, jobErr)
	}
	if !sawParams || gotID != "j-nil" {
		t.Fatalf("agent saw params id=%q (sawParams=%v), want j-nil", gotID, sawParams)
	}
}
