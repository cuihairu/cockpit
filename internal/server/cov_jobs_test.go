package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// ============ POST /api/jobs ============

// TestJobsCreateSuccess 经假 agent（job.exec 应答）验证创建-执行-终态回写全链。
// 断言透传 method/params、输出落库、actor 无 auth 上下文时为 unknown。
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
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}
	if gotMethod != "job.exec" {
		t.Fatalf("method = %q, want job.exec", gotMethod)
	}
	if gotParams["command"] != "uptime" {
		t.Fatalf("params = %+v", gotParams)
	}

	var v jobView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v.ID == "" || v.Status != "success" || v.Output != "hello from agent" {
		t.Fatalf("view = %+v", v)
	}

	// 库内终态（含 Actor/时间戳）
	job, err := s.db.GetJob(v.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
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
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var v jobView
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Status != "failed" || v.ExitCode != 3 || v.Output != "NOK" {
		t.Fatalf("view = %+v", v)
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
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var v jobView
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Status != "success" || v.Output != "" {
		t.Fatalf("view = %+v", v)
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
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var v jobView
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Status != "failed" || !strings.Contains(v.Error, "agent rejected the operation") {
		t.Fatalf("view = %+v", v)
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
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var v jobView
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Status != "failed" || !strings.Contains(v.Error, "permission denied") {
		t.Fatalf("view = %+v", v)
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