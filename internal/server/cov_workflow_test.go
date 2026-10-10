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

// ============ M2a 步骤变量传递 ============

// TestWorkflowStepRefValidation 定义期引用校验（V2）：未知名/前向/自引用
// 400，指向更靠前步骤的合法引用 201。
func TestWorkflowStepRefValidation(t *testing.T) {
	s := newBackupTestServer(t)
	stepA := `{"type":"agent.exec","target":"a1","parameters":{"name":"a","command":"x"}}`
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"unknown ref", `{"name":"v","steps":[` + stepA + `,` +
			`{"type":"agent.exec","target":"a1","parameters":{"name":"b","command":"echo {{steps.nope.output}}"}}]}`, false},
		{"forward ref", `{"name":"v","steps":[` +
			`{"type":"agent.exec","target":"a1","parameters":{"name":"a","command":"echo {{steps.b.output}}"}},` + stepA + `]}`, false},
		{"self ref", `{"name":"v","steps":[` +
			`{"type":"agent.exec","target":"a1","parameters":{"name":"a","command":"echo {{steps.a.output}}"}}]}`, false},
		{"valid ref", `{"name":"v","steps":[` + stepA + `,` +
			`{"type":"agent.exec","target":"a1","parameters":{"name":"b","command":"echo {{steps.a.output}}"}}]}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows", strings.NewReader(c.body)), "/workflows")
			want := http.StatusCreated
			if !c.ok {
				want = http.StatusBadRequest
			}
			if rec.Code != want {
				t.Fatalf("code = %d, want %d, body: %s", rec.Code, want, rec.Body.String())
			}
		})
	}
}

// TestWorkflowStepVariableResolution 两步链解析（V1/V3/V4）：第二步派发的
// command 是第一步输出替换结果；非 {{steps.*.output}} 形态的 {{...}} 原样
// 保留；嵌套参数树替换；meta 键不漏到 agent（连带修复 M1 dispatch 传定义
// 原参数的遗留缺陷）。
func TestWorkflowStepVariableResolution(t *testing.T) {
	s := newBackupTestServer(t)
	var got []map[string]interface{}
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		got = append(got, params)
		return map[string]interface{}{"exit_code": 0, "output": "scan-out", "error": ""}, ""
	})

	body := `{"name":"v","steps":[` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"scan","command":"do-scan"}},` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"use","command":"echo {{steps.scan.output}} | keep ${{nothing}} {{ not.a.ref }}","extra":{"inner":"[{{steps.scan.output}}]","list":["{{steps.scan.output}}","raw"]}}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)
	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusSuccess {
		t.Fatalf("run = %+v", run)
	}
	if len(got) != 2 {
		t.Fatalf("dispatched = %d calls", len(got))
	}
	for i, p := range got {
		if _, ok := p["name"]; ok {
			t.Fatalf("call %d: meta key leaked to agent: %v", i, p)
		}
	}
	want := "echo scan-out | keep ${{nothing}} {{ not.a.ref }}"
	if c, _ := got[1]["command"].(string); c != want {
		t.Fatalf("resolved command = %q, want %q", c, want)
	}
	// 嵌套参数树替换：Job 行 Parameters 里 extra 已解析（快照即事实）
	var steps []workflowStepRun
	json.Unmarshal([]byte(run.Steps), &steps)
	job, err := s.db.GetJob(steps[1].JobID)
	if err != nil {
		t.Fatalf("step job: %v", err)
	}
	var jp map[string]interface{}
	json.Unmarshal([]byte(job.Parameters), &jp)
	extra, _ := jp["extra"].(map[string]interface{})
	list, _ := extra["list"].([]interface{})
	if extra["inner"] != "[scan-out]" || len(list) != 2 || list[0] != "scan-out" || list[1] != "raw" {
		t.Fatalf("nested params = %v", extra)
	}
}

// TestWorkflowStepResolutionRejectsOversize 替换后命令超 16KB（V5）：该步
// 不派发直接 failed，快照 stopReason 记拒绝原因，run failed。
func TestWorkflowStepResolutionRejectsOversize(t *testing.T) {
	s := newBackupTestServer(t)
	calls := 0
	bigOut := strings.Repeat("x", 20*1024)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		calls++
		return map[string]interface{}{"exit_code": 0, "output": bigOut, "error": ""}, ""
	})

	body := `{"name":"v","steps":[` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"big","command":"gen"}},` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"use","command":"{{steps.big.output}}"}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)
	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusFailed {
		t.Fatalf("run = %+v", run)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (step 2 must not dispatch)", calls)
	}
	var steps []workflowStepRun
	json.Unmarshal([]byte(run.Steps), &steps)
	if steps[1].Status != "failed" || steps[1].JobID != "" ||
		!strings.Contains(steps[1].StopReason, "rejected") {
		t.Fatalf("step 1 = %+v", steps[1])
	}
}

// TestWorkflowStepRefFailedSource 引用失败步（V4 + W4 组合）：continue_on_error
// 放行的失败步，后步取其已存输出（错误输出照常注入），run success。
func TestWorkflowStepRefFailedSource(t *testing.T) {
	s := newBackupTestServer(t)
	var commands []string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		cmd, _ := params["command"].(string)
		commands = append(commands, cmd)
		if len(commands) == 1 {
			return map[string]interface{}{"exit_code": 3, "output": "err-out", "error": ""}, ""
		}
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	body := `{"name":"v","steps":[` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"soft","command":"false","continue_on_error":true}},` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"after","command":"echo {{steps.soft.output}}"}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)
	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusSuccess {
		t.Fatalf("run = %+v", run)
	}
	if len(commands) != 2 || commands[1] != "echo err-out" {
		t.Fatalf("commands = %q", commands)
	}
}

// TestCovStepOutputOfNil 被引步骤 Job 行未建成（存储错误）时输出按空串
// 处理（M2a V4：与失败步同语义）。
func TestCovStepOutputOfNil(t *testing.T) {
	if stepOutputOf(nil) != "" {
		t.Fatal("nil job output should be empty")
	}
	if stepOutputOf(&storage.Job{Output: "x"}) != "x" {
		t.Fatal("job output should pass through")
	}
}

// ============ M2b 多目标扇出 ============

// TestWorkflowFanoutValidation targets 校验矩阵（F1）与 standalone Job 拒收。
func TestWorkflowFanoutValidation(t *testing.T) {
	s := newBackupTestServer(t)
	tg := make([]string, workflowMaxTargets+1)
	for i := range tg {
		tg[i] = `"` + strings.Repeat("a", i+1) + `"`
	}
	for _, c := range []struct {
		name string
		body string
	}{
		{"target and targets exclusive", `{"name":"n","steps":[{"type":"agent.exec","target":"a1","targets":["a2"],"parameters":{"name":"s","command":"x"}}]}`},
		{"empty target entry", `{"name":"n","steps":[{"type":"agent.exec","targets":["a1","  "],"parameters":{"name":"s","command":"x"}}]}`},
		{"duplicate targets", `{"name":"n","steps":[{"type":"agent.exec","targets":["a1","a1"],"parameters":{"name":"s","command":"x"}}]}`},
		{"too many targets", `{"name":"n","steps":[{"type":"agent.exec","targets":[` + strings.Join(tg, ",") + `],"parameters":{"name":"s","command":"x"}}]}`},
		{"empty targets array", `{"name":"n","steps":[{"type":"agent.exec","targets":[],"parameters":{"name":"s","command":"x"}}]}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows", strings.NewReader(c.body)), "/workflows")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
			}
		})
	}
	// standalone Job 携 targets → 400（扇出仅 workflow steps 合法）
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(`{"type":"agent.exec","targets":["a1"],"parameters":{"command":"x"}}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "workflow steps") {
		t.Fatalf("standalone targets: %d %s", rec.Code, rec.Body.String())
	}
}

// TestWorkflowFanoutSuccess 两台顺序扇出（F2）：执行序=targets 序，全成功聚合
// success，逐台 Job 落台账；同 run 的单目标步快照形状不变（无 targets 键）。
func TestWorkflowFanoutSuccess(t *testing.T) {
	s := newBackupTestServer(t)
	var order []string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		order = append(order, "a1")
		return map[string]interface{}{"exit_code": 0, "output": "from-a1", "error": ""}, ""
	})
	withFakeBackupAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		order = append(order, "a2")
		return map[string]interface{}{"exit_code": 0, "output": "from-a2", "error": ""}, ""
	})

	body := `{"name":"f","steps":[` +
		`{"type":"agent.exec","targets":["a2","a1"],"parameters":{"name":"fan","command":"uptime"}},` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"tail","command":"date"}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	if rec.Code != http.StatusCreated {
		t.Fatalf("run code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)

	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusSuccess {
		t.Fatalf("run = %+v", run)
	}
	// 顺序扇出：a2 先于 a1，扇出步先于尾步
	if len(order) != 3 || order[0] != "a2" || order[1] != "a1" || order[2] != "a1" {
		t.Fatalf("dispatch order = %v", order)
	}
	var steps []workflowStepRun
	json.Unmarshal([]byte(run.Steps), &steps)
	if len(steps) != 2 {
		t.Fatalf("steps = %+v", steps)
	}
	fan := steps[0]
	if fan.Status != "success" || fan.Target != "" || fan.JobID != "" || fan.StopReason != "" {
		t.Fatalf("fan step = %+v", fan)
	}
	if len(fan.Targets) != 2 ||
		fan.Targets[0].AgentID != "a2" || fan.Targets[0].Status != "success" || fan.Targets[0].JobID == "" || fan.Targets[0].Attempts != 1 ||
		fan.Targets[1].AgentID != "a1" || fan.Targets[1].Status != "success" || fan.Targets[1].JobID == "" {
		t.Fatalf("fan targets = %+v", fan.Targets)
	}
	// 单目标步形状回归：无 targets 键、JobID 仍在步级
	var raw []map[string]json.RawMessage
	json.Unmarshal([]byte(run.Steps), &raw)
	if _, ok := raw[1]["targets"]; ok {
		t.Fatalf("single-target step should not carry targets key: %s", raw[1])
	}
	if steps[1].Status != "success" || steps[1].JobID == "" || steps[1].Target != "a1" {
		t.Fatalf("tail step = %+v", steps[1])
	}
	// 逐台 Job 落台账（3 行：a2 + a1 + 尾步 a1）
	jobs, _ := s.db.ListJobsFiltered("", "", "", rv.ID, 10)
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
}

// TestWorkflowFanoutFailStops 首台失败即停（F3）：后续目标不跑（pending）、
// 下一步不执行、run failed。
func TestWorkflowFanoutFailStops(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 3, "output": "boom", "error": ""}, ""
	})
	withFakeBackupAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	body := `{"name":"f","steps":[` +
		`{"type":"agent.exec","targets":["a2","a1"],"parameters":{"name":"fan","command":"uptime"}},` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"never","command":"date"}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)

	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusFailed {
		t.Fatalf("run = %+v", run)
	}
	jobs, _ := s.db.ListJobsFiltered("", "", "", rv.ID, 10)
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2 (second target not reached)", len(jobs))
	}
	var steps []workflowStepRun
	json.Unmarshal([]byte(run.Steps), &steps)
	if steps[0].Status != "failed" || steps[0].StopReason != "failed" ||
		steps[0].Targets[0].Status != "success" || steps[0].Targets[1].Status != "failed" || steps[0].Targets[1].StopReason != "failed" {
		t.Fatalf("steps = %+v", steps)
	}
	if steps[1].Status != "pending" {
		t.Fatalf("tail step = %+v", steps[1])
	}
}

// TestWorkflowFanoutContinueOnError continue_on_error 跑完全部目标（F3）：
// 步 failed 但链继续，run success。
func TestWorkflowFanoutContinueOnError(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 3, "output": "boom", "error": ""}, ""
	})
	withFakeBackupAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	body := `{"name":"f","steps":[` +
		`{"type":"agent.exec","targets":["a1","a2"],"parameters":{"name":"fan","command":"uptime","continue_on_error":true}},` +
		`{"type":"agent.exec","target":"a2","parameters":{"name":"tail","command":"date"}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)

	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusSuccess {
		t.Fatalf("run = %+v", run)
	}
	jobs, _ := s.db.ListJobsFiltered("", "", "", rv.ID, 10)
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3 (all targets ran)", len(jobs))
	}
	var steps []workflowStepRun
	json.Unmarshal([]byte(run.Steps), &steps)
	if steps[0].Status != "failed" || steps[0].Targets[0].Status != "failed" || steps[0].Targets[1].Status != "success" {
		t.Fatalf("steps = %+v", steps)
	}
	if steps[1].Status != "success" {
		t.Fatalf("tail step = %+v", steps[1])
	}
}

// TestWorkflowFanoutRetryPerTarget 按台独立重试周期（F4）：a1 首败 retry 后
// 成功（attempts 2），a2 一次过（attempts 1）。
func TestWorkflowFanoutRetryPerTarget(t *testing.T) {
	saveRetry := workflowRetryInterval
	workflowRetryInterval = 20 * time.Millisecond
	t.Cleanup(func() { workflowRetryInterval = saveRetry })
	s := newBackupTestServer(t)
	a1Calls := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		a1Calls++
		if a1Calls == 1 {
			return map[string]interface{}{"exit_code": 1, "output": "", "error": ""}, ""
		}
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})
	withFakeBackupAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	body := `{"name":"f","steps":[{"type":"agent.exec","targets":["a1","a2"],"parameters":{"name":"fan","command":"uptime","retry":1}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)

	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusSuccess {
		t.Fatalf("run = %+v", run)
	}
	if a1Calls != 2 {
		t.Fatalf("a1 calls = %d, want 2", a1Calls)
	}
	var steps []workflowStepRun
	json.Unmarshal([]byte(run.Steps), &steps)
	if steps[0].Status != "success" || steps[0].Targets[0].Attempts != 2 || steps[0].Targets[1].Attempts != 1 {
		t.Fatalf("steps = %+v", steps)
	}
	// 逐台 attempt 独立 Job 行：a1 两条 + a2 一条
	jobs, _ := s.db.ListJobsFiltered("", "", "", rv.ID, 10)
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
}

// TestWorkflowFanoutCancelBoundary 目标边界取消（F5）：已取消 run 上扇出，
// 全部目标记 cancelled（stopReason=run cancelled），零派发；快照整步回写后
// 步级 cancelled 且无 StopReason（未跑原因在逐台条目）。
func TestWorkflowFanoutCancelBoundary(t *testing.T) {
	s := newBackupTestServer(t)
	calls := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		calls++
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})
	withFakeBackupAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		calls++
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	wf := wfCreate(t, s, `{"name":"c","steps":[{"type":"agent.exec","targets":["a1","a2"],"parameters":{"name":"fan","command":"uptime"}}]}`)
	stepsJSON := `[{"name":"fan","type":"agent.exec","target":"","targets":[` +
		`{"agentId":"a1","status":"pending"},{"agentId":"a2","status":"pending"}],"status":"pending"}]`
	run := &storage.WorkflowRun{
		ID: "r-cancelled", WorkflowID: wf.ID, WorkflowName: wf.Name,
		Status: storage.WorkflowRunStatusCancelled, Steps: stepsJSON, CreatedAt: time.Now(),
	}
	if err := s.db.CreateWorkflowRun(run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	step := &jobCreatePayload{Type: "agent.exec", Targets: []string{"a1", "a2"},
		Parameters: map[string]interface{}{"name": "fan", "command": "x"}}
	status, reason, results := s.runWorkflowStepFanout(step, stepOptions{}, run.ID, "tester", map[string]string{})
	if status != storage.JobStatusCancelled || reason != "" || len(results) != 2 {
		t.Fatalf("fanout = %s %q %+v", status, reason, results)
	}
	for _, tr := range results {
		if tr.Status != storage.JobStatusCancelled || tr.StopReason != "run cancelled" || tr.JobID != "" {
			t.Fatalf("target result = %+v", tr)
		}
	}
	if calls != 0 {
		t.Fatalf("dispatched %d times on cancelled run", calls)
	}

	// advanceWorkflow 同款回写：快照按索引合并、步级 cancelled
	s.recordStepTargets(run, 0, results, status, reason)
	saved, err := s.db.GetWorkflowRun(run.ID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	var steps []workflowStepRun
	json.Unmarshal([]byte(saved.Steps), &steps)
	if steps[0].Status != "cancelled" || steps[0].StopReason != "" ||
		steps[0].Targets[0].Status != "cancelled" || steps[0].Targets[1].AgentID != "a2" || steps[0].Targets[1].Status != "cancelled" {
		t.Fatalf("saved steps = %+v", steps)
	}
}

// TestWorkflowFanoutOfflineTarget 任一目标离线 → 503 不建 run（F8）。
func TestWorkflowFanoutOfflineTarget(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)

	body := `{"name":"f","steps":[{"type":"agent.exec","targets":["a1","ghost"],"parameters":{"name":"fan","command":"uptime"}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "ghost") {
		t.Fatalf("code = %d, body: %s", rec.Code, rec.Body.String())
	}
}

// TestWorkflowFanoutOutputEmpty 扇出步不提供步骤输出（F7）：引用解析空串；
// 单条目 targets 走扇出路径。
func TestWorkflowFanoutOutputEmpty(t *testing.T) {
	s := newBackupTestServer(t)
	var got []map[string]interface{}
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		got = append(got, params)
		return map[string]interface{}{"exit_code": 0, "output": "fan-out", "error": ""}, ""
	})

	body := `{"name":"f","steps":[` +
		`{"type":"agent.exec","targets":["a1"],"parameters":{"name":"fan","command":"uptime"}},` +
		`{"type":"agent.exec","target":"a1","parameters":{"name":"use","command":"echo [{{steps.fan.output}}]"}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)

	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusSuccess {
		t.Fatalf("run = %+v", run)
	}
	if len(got) != 2 {
		t.Fatalf("dispatched = %d calls", len(got))
	}
	if c, _ := got[1]["command"].(string); c != "echo []" {
		t.Fatalf("resolved command = %q, want %q", c, "echo []")
	}
}

// TestWorkflowFanoutMidRunCancel 运行中扇出目标边界取消（F5 全链）：a1 在途
// 时取消 run（handler 内同步取消，取消本身确定性），a1 正常返回 success，
// a2 目标边界检查记 cancelled，run 保持 cancelled 终态（不走 failed 落终态；
// 断言前等快照逐台行落定，终态早于回写）。
func TestWorkflowFanoutMidRunCancel(t *testing.T) {
	s := newBackupTestServer(t)
	runIDCh := make(chan string, 1)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		// a1 在途：等测试拿到 run ID 后取消 run，再正常返回
		id := <-runIDCh
		run, err := s.db.GetWorkflowRun(id)
		if err != nil {
			t.Errorf("load run: %v", err)
		} else {
			run.Status = storage.WorkflowRunStatusCancelled
			finished := time.Now()
			run.FinishedAt = &finished
			if err := s.db.UpdateWorkflowRun(run); err != nil {
				t.Errorf("cancel run: %v", err)
			}
		}
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})
	withFakeBackupAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	body := `{"name":"c","steps":[{"type":"agent.exec","targets":["a1","a2"],"parameters":{"name":"fan","command":"uptime"}}]}`
	created := wfCreate(t, s, body)
	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil), "/workflows/"+created.ID+"/run")
	if rec.Code != http.StatusCreated {
		t.Fatalf("run code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var rv workflowRunView
	json.Unmarshal(rec.Body.Bytes(), &rv)
	runIDCh <- rv.ID

	// run goroutine 推进到 a2 目标边界后落终态
	run := waitRunTerminal(t, s, rv.ID)
	if run.Status != storage.WorkflowRunStatusCancelled {
		t.Fatalf("run = %+v (cancel must not be overwritten by failed)", run)
	}
	// 等逐台行全部落定再断言：cancel 只改 run 状态，快照由扇出 runner 在
	// 目标边界回写（与单目标路径 recordStepResult 同窗口），慢机上轮询可
	// 抢在回写前读到全 pending 快照（CI 复现：两 target 全 pending）
	deadline := time.Now().Add(5 * time.Second)
	for {
		cur, err := s.db.GetWorkflowRun(rv.ID)
		if err != nil {
			t.Fatalf("load run: %v", err)
		}
		run = cur
		var settled []workflowStepRun
		if json.Unmarshal([]byte(run.Steps), &settled) == nil &&
			len(settled) == 1 && len(settled[0].Targets) == 2 {
			allFinal := true
			for _, tg := range settled[0].Targets {
				if tg.Status == storage.JobStatusPending {
					allFinal = false
				}
			}
			if allFinal {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("step targets never settled: %+v", run)
		}
		time.Sleep(5 * time.Millisecond)
	}
	var steps []workflowStepRun
	json.Unmarshal([]byte(run.Steps), &steps)
	if len(steps) != 1 || steps[0].Status != "cancelled" ||
		steps[0].Targets[0].AgentID != "a1" || steps[0].Targets[0].Status != "success" ||
		steps[0].Targets[1].AgentID != "a2" || steps[0].Targets[1].Status != "cancelled" || steps[0].Targets[1].StopReason != "run cancelled" {
		t.Fatalf("steps = %+v", steps)
	}
}

// TestWorkflowFanoutDBErrors db 失败面：扇出中加载 run 失败即停（同
// advanceWorkflow 加载失败口径）、快照回写加载失败静默、jobIDOf nil 防御。
func TestWorkflowFanoutDBErrors(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)
	if err := s.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	step := &jobCreatePayload{Type: "agent.exec", Targets: []string{"a1"},
		Parameters: map[string]interface{}{"name": "fan", "command": "x"}}
	status, reason, results := s.runWorkflowStepFanout(step, stepOptions{}, "r-x", "tester", map[string]string{})
	if status != storage.JobStatusSuccess || reason != "" || len(results) != 0 {
		t.Fatalf("fanout on closed db = %s %q %+v", status, reason, results)
	}
	// 快照回写在加载失败时静默返回（与 recordStepResult 同口径）
	run := &storage.WorkflowRun{ID: "r-x"}
	s.recordStepTargets(run, 0, results, status, reason)
	// jobIDOf nil 防御（Job 行未建成）
	if jobIDOf(nil) != "" {
		t.Fatal("jobIDOf(nil) should be empty")
	}
}
