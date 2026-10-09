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

// ============ Workflow 定义 CRUD ============

const wfStepsTwo = `{"name":"chain","steps":[` +
	`{"type":"agent.exec","target":"a1","parameters":{"name":"uptime","command":"uptime","timeout_s":60}},` +
	`{"type":"agent.exec","target":"a1","parameters":{"name":"date","command":"date"}}]}`

func wfCreate(t *testing.T, s *Server, body string) workflowView {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows", strings.NewReader(body)), "/workflows")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var v workflowView
	json.Unmarshal(rec.Body.Bytes(), &v)
	return v
}

func waitRunTerminal(t *testing.T, s *Server, id string) *storage.WorkflowRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, err := s.db.GetWorkflowRun(id)
		if err == nil && run.Status != storage.WorkflowRunStatusRunning {
			return run
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("run %s never reached terminal state", id)
	return nil
}

// TestWorkflowCRUDRoutes 定义创建/列表/详情/更新/删除/校验面。
func TestWorkflowCRUDRoutes(t *testing.T) {
	s := newBackupTestServer(t)
	created := wfCreate(t, s, wfStepsTwo)
	if created.ID == "" || len(created.Steps) != 2 {
		t.Fatalf("created = %+v", created)
	}

	// 列表
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodGet, "/api/workflows", nil), "/workflows")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"chain"`) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}

	// 详情
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodGet, "/api/workflows/"+created.ID, nil), "/workflows/"+created.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}
	// 404
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodGet, "/api/workflows/nope", nil), "/workflows/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("404: %d", rec.Code)
	}

	// 更新
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPut, "/api/workflows/"+created.ID,
		strings.NewReader(`{"name":"renamed","steps":[{"type":"agent.exec","target":"a1","parameters":{"name":"s","command":"x"}}]}`)),
		"/workflows/"+created.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}

	// 校验面 400
	for _, c := range []struct {
		name string
		body string
	}{
		{"missing name", `{"steps":[{}]}`},
		{"name too long", `{"name":"` + strings.Repeat("a", 65) + `","steps":[{"type":"agent.exec","target":"a1","parameters":{"name":"s","command":"x"}}]}`},
		{"empty steps", `{"name":"n","steps":[]}`},
		{"bad step", `{"name":"n","steps":[{"type":"nope","target":"a1"}]}`},
		{"dup step names", `{"name":"n","steps":[` +
			`{"type":"agent.exec","target":"a1","parameters":{"name":"s","command":"x"}},` +
			`{"type":"agent.exec","target":"a1","parameters":{"name":"s","command":"y"}}]}`},
		{"missing step name", `{"name":"n","steps":[{"type":"agent.exec","target":"a1","parameters":{"command":"x"}}]}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows", strings.NewReader(c.body)), "/workflows")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
			}
		})
	}
	// 步数上限
	steps := make([]string, storage.WorkflowMaxSteps+1)
	for i := range steps {
		steps[i] = `{"type":"agent.exec","target":"a1","parameters":{"name":"s` + strings.Repeat("a", i+1) + `","command":"x"}}`
	}
	tooMany := `{"name":"n","steps":[` + strings.Join(steps, ",") + `]}`
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows", strings.NewReader(tooMany)), "/workflows")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("too many steps: %d", rec.Code)
	}

	// 删除
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodDelete, "/api/workflows/"+created.ID, nil), "/workflows/"+created.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
	if _, err := s.db.GetWorkflow(created.ID); err == nil {
		t.Fatal("deleted workflow readable")
	}
}

// ============ run 推进 ============

// TestWorkflowRunSuccessChain 成功链：两步全过，run success、快照各步 success、
// 每步一条 Job 且挂 workflow_run_id。
func TestWorkflowRunSuccessChain(t *testing.T) {
	s := newBackupTestServer(t)
	calls := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		calls++
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	created := wfCreate(t, s, wfStepsTwo)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	if rec.Code != http.StatusCreated {
		t.Fatalf("run code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)
	if rv.Status != storage.WorkflowRunStatusRunning {
		t.Fatalf("run view = %+v", rv)
	}

	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusSuccess {
		t.Fatalf("run = %+v", run)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	var steps []workflowStepRun
	json.Unmarshal([]byte(run.Steps), &steps)
	if len(steps) != 2 || steps[0].Status != "success" || steps[1].Status != "success" ||
		steps[0].JobID == "" || steps[0].Attempts != 1 {
		t.Fatalf("steps = %+v", steps)
	}
	job, _ := s.db.GetJob(steps[0].JobID)
	if job.WorkflowRunID != rv.ID || job.Status != "success" {
		t.Fatalf("step job = %+v", job)
	}
	// 步骤参数已剥编排元参数（name 不下发）
	if strings.Contains(job.Parameters, `"name"`) {
		t.Fatalf("orchestration meta leaked to job params: %s", job.Parameters)
	}

	// run 台账
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodGet, "/api/workflows/"+created.ID+"/runs", nil), "/workflows/"+created.ID+"/runs")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), rv.ID) {
		t.Fatalf("runs list: %d %s", rec.Code, rec.Body.String())
	}
	// run 详情
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodGet, "/api/workflow-runs/"+rv.ID, nil), "/workflow-runs/"+rv.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("run get: %d", rec.Code)
	}
}

// TestWorkflowRunFailStops 中间失败：run failed、第三步不执行。
func TestWorkflowRunFailStops(t *testing.T) {
	s := newBackupTestServer(t)
	calls := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		calls++
		if calls == 2 {
			return map[string]interface{}{"exit_code": 3, "output": "boom", "error": ""}, ""
		}
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	body := `{"name":"f","steps":[` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"ok","command":"true"}},` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"bad","command":"false"}},` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"never","command":"true"}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)

	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusFailed {
		t.Fatalf("run = %+v", run)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (third step must not run)", calls)
	}
	var steps []workflowStepRun
	json.Unmarshal([]byte(run.Steps), &steps)
	if len(steps) != 3 || steps[0].Status != "success" || steps[1].Status != "failed" || steps[2].Status != "pending" {
		t.Fatalf("steps = %+v", steps)
	}
}

// TestWorkflowRunContinueOnError continue_on_error：失败步跳过继续，run success。
func TestWorkflowRunContinueOnError(t *testing.T) {
	s := newBackupTestServer(t)
	calls := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		calls++
		if calls == 1 {
			return map[string]interface{}{"exit_code": 9, "output": "", "error": ""}, ""
		}
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	body := `{"name":"c","steps":[` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"soft","command":"false","continue_on_error":true}},` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"after","command":"true"}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)

	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusSuccess {
		t.Fatalf("run = %+v", run)
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
	var steps []workflowStepRun
	json.Unmarshal([]byte(run.Steps), &steps)
	if steps[0].Status != "failed" || steps[0].ContinueOnError != true || steps[1].Status != "success" {
		t.Fatalf("steps = %+v", steps)
	}
}

// TestWorkflowRunRetry retry：首试失败重试成功（每次尝试独立 Job 行）。
func TestWorkflowRunRetry(t *testing.T) {
	saveRetry := workflowRetryInterval
	workflowRetryInterval = 20 * time.Millisecond
	t.Cleanup(func() { workflowRetryInterval = saveRetry })
	s := newBackupTestServer(t)
	calls := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		calls++
		if calls == 1 {
			return map[string]interface{}{"exit_code": 1, "output": "", "error": ""}, ""
		}
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	body := `{"name":"r","steps":[{"type":"agent.exec","target":"a1","parameters":{"name":"flaky","command":"x","retry":2}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)

	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusSuccess {
		t.Fatalf("run = %+v", run)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	var steps []workflowStepRun
	json.Unmarshal([]byte(run.Steps), &steps)
	if steps[0].Attempts != 2 || steps[0].Status != "success" {
		t.Fatalf("steps = %+v", steps)
	}
	// 两次尝试两条 Job（workflow_run_id 过滤可见）
	jobs, _ := s.db.ListJobsFiltered("", "", "", rv.ID, 10)
	if len(jobs) != 2 {
		t.Fatalf("attempt jobs = %d, want 2", len(jobs))
	}
}

// TestWorkflowRunOfflineTarget 步骤目标离线 → 503 不建 run。
func TestWorkflowRunOfflineTarget(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)
	body := `{"name":"o","steps":[` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"ok","command":"x"}},` +
		`{"type":"agent.exec","target":"ghost","parameters":{"name":"g","command":"x"}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}
	runs, _ := s.db.ListWorkflowRuns(created.ID)
	if len(runs) != 0 {
		t.Fatalf("ghost run created: %+v", runs)
	}
}

// TestWorkflowRunReentryAndMutationLock 重入 409；active run 期间更新/删除 409。
func TestWorkflowRunReentryAndMutationLock(t *testing.T) {
	s := newBackupTestServer(t)
	release := make(chan struct{})
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		<-release
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})
	created := wfCreate(t, s, `{"name":"l","steps":[{"type":"agent.exec","target":"a1","parameters":{"name":"slow","command":"x"}}]}`)

	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	if rec.Code != http.StatusCreated {
		t.Fatalf("first run: %d %s", rec.Code, rec.Body.String())
	}
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)

	// 重入 409
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	if rec.Code != http.StatusConflict {
		t.Fatalf("reentry code = %d", rec.Code)
	}
	// 更新 409
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPut, "/api/workflows/"+created.ID,
		strings.NewReader(`{"name":"x","steps":[{"type":"agent.exec","target":"a1","parameters":{"name":"y","command":"z"}}]}`)),
		"/workflows/"+created.ID)
	if rec.Code != http.StatusConflict {
		t.Fatalf("update lock code = %d", rec.Code)
	}
	// 删除 409
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodDelete, "/api/workflows/"+created.ID, nil), "/workflows/"+created.ID)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete lock code = %d", rec.Code)
	}

	close(release)
	waitRunTerminal(t, s, rv.ID)

	// 终态后更新/删除恢复
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodDelete, "/api/workflows/"+created.ID, nil), "/workflows/"+created.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete after finish code = %d", rec.Code)
	}
}

// TestWorkflowRunCancel 取消 run：在途步骤后停止推进，run cancelled。
func TestWorkflowRunCancel(t *testing.T) {
	s := newBackupTestServer(t)
	// 步骤 1 慢（堵住推进），取消后步骤 2 不得执行
	step1Started := make(chan struct{})
	release := make(chan struct{})
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if params["command"] == "slow" {
			close(step1Started)
			<-release
			return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
		}
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})
	body := `{"name":"cc","steps":[` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"slow","command":"slow"}},` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"never","command":"fast"}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)

	<-step1Started // 步骤 1 已派发在途
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflow-runs/"+rv.ID+"/cancel", nil), "/workflow-runs/"+rv.ID+"/cancel")
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel code = %d, body: %s", rec.Code, rec.Body.String())
	}

	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusCancelled {
		t.Fatalf("run = %+v", run)
	}
	// 步骤 2 未执行（fast 只在步骤 2 出现）
	jobs, _ := s.db.ListJobsFiltered("", "", "", rv.ID, 10)
	for _, j := range jobs {
		if strings.Contains(j.Parameters, "fast") {
			t.Fatalf("step 2 executed after cancel: %+v", j)
		}
	}

	// 终态后再取消 409
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflow-runs/"+rv.ID+"/cancel", nil), "/workflow-runs/"+rv.ID+"/cancel")
	if rec.Code != http.StatusConflict {
		t.Fatalf("cancel twice code = %d", rec.Code)
	}
	close(release)
	// 等编排器回写步骤快照后再结束（cleanup 关库，避免 goroutine 撞上
	// closed db 刷错误日志）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := s.db.GetWorkflowRun(rv.ID)
		if err == nil && strings.Contains(snap.Steps, `"attempts":1`) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // loop-top 取消检查的余量
}

// TestWorkflowRunsAPIEdges run 详情 404/方法面/未知路径。
func TestWorkflowRunsAPIEdges(t *testing.T) {
	s := newBackupTestServer(t)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodGet, "/api/workflow-runs/nope", nil), "/workflow-runs/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("404: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/x/run", nil), "/workflows/x/run")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("run of missing wf: %d", rec.Code)
	}
	// run 定义存在但删除后 runs 台账 404
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodGet, "/api/workflows/ghost/runs", nil), "/workflows/ghost/runs")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("runs of missing wf: %d", rec.Code)
	}
	// 空 id
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodGet, "/api/workflows/", nil), "/workflows/")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("empty id: %d", rec.Code)
	}
	// 方法不允许
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodGet, "/api/workflows/x/run", nil), "/workflows/x/run")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("405: %d", rec.Code)
	}
}

// TestWorkflowAPIDispatch serveAPI 中央分发器路由（api.go case 分支）与
// 入参解码面（坏 JSON 400、更新不存在 404）。
func TestWorkflowAPIDispatch(t *testing.T) {
	s := newBackupTestServer(t)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, httptest.NewRequest(http.MethodGet, "/api/workflows", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"workflows":[]`) {
		t.Fatalf("dispatch list: %d %s", rec.Code, rec.Body.String())
	}

	// 坏 JSON → 400
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows",
		strings.NewReader(`{not json`)), "/workflows")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json: %d", rec.Code)
	}

	// 更新不存在的定义 → 404
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPut, "/api/workflows/ghost",
		strings.NewReader(`{"name":"n","steps":[{"type":"agent.exec","target":"a1","parameters":{"name":"s","command":"x"}}]}`)),
		"/workflows/ghost")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("update ghost: %d", rec.Code)
	}
}

// TestWorkflowRBACRules workflows 路由已登记 RBAC（D8 教训：未登记则
// governed=false 整体放行）。
func TestWorkflowRBACRules(t *testing.T) {
	for _, c := range []struct {
		path   string
		method string
		perm   string
	}{
		{"/api/workflows", "GET", "workflows:read"},
		{"/api/workflows", "POST", "workflows:write"},
		{"/api/workflows/wf-1/run", "POST", "workflows:write"},
		{"/api/workflow-runs/r-1/cancel", "POST", "workflows:write"},
	} {
		perms, governed := requiredPerms(c.path, c.method)
		if !governed {
			t.Fatalf("%s %s not governed by RBAC", c.method, c.path)
		}
		found := false
		for _, p := range perms {
			if p == c.perm {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s %s perms = %v, want %q", c.method, c.path, perms, c.perm)
		}
	}
	// 权限点在合法清单内（角色 seed 白名单）
	if !storage.PermissionValid("workflows:read") || !storage.PermissionValid("workflows:write") {
		t.Fatal("workflows perms not valid")
	}
}

// ============ 分发边界 / 传输面错误 / closed db 500 ============

// TestWorkflowAPIRouteEdges 分发面 404/405 兜底与 ghost 目标分支。
func TestWorkflowAPIRouteEdges(t *testing.T) {
	s := newBackupTestServer(t)
	cases := []struct {
		name   string
		method string
		path   string
		body   string
		code   int
	}{
		{"bare run path", http.MethodGet, "/api/workflows//run", "", http.StatusNotFound},
		{"runs bare path", http.MethodGet, "/api/workflow-runs/", "", http.StatusNotFound},
		{"cancel wrong method", http.MethodGet, "/api/workflow-runs/r1/cancel", "", http.StatusMethodNotAllowed},
		{"runs wrong method", http.MethodPut, "/api/workflow-runs/r1", "", http.StatusMethodNotAllowed},
		{"step path wrong method", http.MethodPatch, "/api/workflows/wf-x", "", http.StatusMethodNotAllowed},
		{"update bad json", http.MethodPut, "/api/workflows/ghost", `{bad`, http.StatusBadRequest},
		{"delete ghost", http.MethodDelete, "/api/workflows/ghost", "", http.StatusNotFound},
		{"cancel ghost run", http.MethodPost, "/api/workflow-runs/ghost/cancel", "", http.StatusNotFound},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		sub := strings.TrimPrefix(c.path, "/api")
		s.handleWorkflowAPI(rec, httptest.NewRequest(c.method, c.path, strings.NewReader(c.body)), sub)
		if rec.Code != c.code {
			t.Fatalf("%s: code = %d, want %d (body: %s)", c.name, rec.Code, c.code, rec.Body.String())
		}
	}
}

// TestWorkflowRunTransportErrors dispatchJob 两个传输面错误分支（单测此前
// 不可达：离线在 run 创建前被 503 拦截）——run 执行中途目标注销（CallAgent
// 失败）与假 agent 回包 payload 类型非法（DecodeRPCResponse 失败）。
func TestWorkflowRunTransportErrors(t *testing.T) {
	// 1) 步骤 1 阻塞持住 run，中途注销 a1 后放行 → 步骤 2 派发失败
	s := newBackupTestServer(t)
	release := make(chan struct{})
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method == "job.exec" {
			<-release
		}
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})
	created := wfCreate(t, s, `{"name":"t","steps":[`+
		`{"type":"agent.exec","target":"a1","parameters":{"name":"s1","command":"x"}},`+
		`{"type":"agent.exec","target":"a1","parameters":{"name":"s2","command":"x"}}]}`)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil),
		"/workflows/"+created.ID+"/run")
	if rec.Code != http.StatusCreated {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)
	s.registry.Unregister("a1")
	close(release)
	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusFailed {
		t.Fatalf("mid-run unregister: status = %s, want failed", run.Status)
	}

	// 2) 回包 payload 非法（status 非 string → DecodePayload 解码失败）
	s2 := newBackupTestServer(t)
	agent := NewAgent("bad", nil)
	agent.Capabilities = []protocol.Capability{{Type: "backup"}}
	if err := s2.registry.Register(agent); err != nil {
		t.Fatalf("register: %v", err)
	}
	go func() {
		for reqMsg := range agent.Send {
			if reqMsg.Type != protocol.MessageTypeRPCRequest {
				continue
			}
			resp := protocol.NewMessage(protocol.MessageTypeRPCResponse,
				map[string]interface{}{"status": 123})
			resp.ID = reqMsg.ID
			s2.handleRPCResponse(resp)
		}
	}()
	t.Cleanup(func() { agent.Close() })
	created2 := wfCreate(t, s2, `{"name":"t2","steps":[`+
		`{"type":"agent.exec","target":"bad","parameters":{"name":"s","command":"x"}}]}`)
	rec = httptest.NewRecorder()
	s2.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created2.ID+"/run", nil),
		"/workflows/"+created2.ID+"/run")
	if rec.Code != http.StatusCreated {
		t.Fatalf("run2: %d %s", rec.Code, rec.Body.String())
	}
	var rv2 workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv2)
	run2 := waitRunTerminal(t, s2, rv2.ID)
	if run2.Status != storage.WorkflowRunStatusFailed {
		t.Fatalf("bad payload: status = %s, want failed", run2.Status)
	}
}

// TestWorkflowAPIDBErrorBranches closed db 下 API 500 分支（校验/离线拦截
// 先行通过，落库或查询时失败——SQLite 面唯一可确定性构造的失败形态，
// 同 storage TestWorkflowReadErrorBranches 口径）。
func TestWorkflowAPIDBErrorBranches(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)
	if err := s.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// workflow 列表查询失败 → 500
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodGet, "/api/workflows", nil), "/workflows")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	// workflow 创建写入失败 → 500
	rec = httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows", strings.NewReader(
		`{"name":"n","steps":[{"type":"agent.exec","target":"a1","parameters":{"name":"s","command":"x"}}]}`)),
		"/workflows")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	// jobs 台账查询失败 → 500
	rec = httptest.NewRecorder()
	s.handleJobsList(rec, httptest.NewRequest(http.MethodGet, "/api/jobs", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("jobs list: %d %s", rec.Code, rec.Body.String())
	}
	// job 创建写入失败 → 500（离线拦截先过 registry，落库时失败）
	rec = httptest.NewRecorder()
	s.handleJobCreate(rec, httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(
		`{"type":"agent.exec","target":"a1","parameters":{"name":"s","command":"x"}}`)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("job create: %d %s", rec.Code, rec.Body.String())
	}
}
