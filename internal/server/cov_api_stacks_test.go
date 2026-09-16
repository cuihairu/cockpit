package server

// api_stacks.go 的覆盖率补充测试：registerStacksAPI、handleStackRPC 错误映射、
// 部署跟踪、pollStackTask 全分支、聚合视图与缓存补齐。

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

var covDockerCaps = []string{"docker"}

// covStuckDockerAgent 注册具备 docker 能力但 Send 缓冲已满的 agent，
// 使后续每个 CallAgent 都在 5s 后触发 "send timeout"。
func covStuckDockerAgent(t *testing.T, s *Server, agentID string) {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Capabilities = []protocol.Capability{{Type: "docker"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	for i := 0; i < 256; i++ {
		agent.Send <- protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{})
	}
	t.Cleanup(func() { s.registry.Unregister(agentID) })
}

// ============ registerStacksAPI ============

func TestCovRegisterStacksAPI(t *testing.T) {
	s := covNewServer(t)
	mux := http.NewServeMux()
	s.registerStacksAPI(mux)

	// 无认证 → 401（mux 前缀带尾斜杠，无尾斜杠会被 ServeMux 301）
	rec := covRec()
	mux.ServeHTTP(rec, covReq(http.MethodGet, "/api/stacks/", nil))
	covWantCode(t, "no auth", rec, http.StatusUnauthorized)

	// 带认证 → 聚合视图 200（token 由该 server 的 authService 签发）
	token, err := s.authService().GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	req := covReq(http.MethodGet, "/api/stacks/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = covRec()
	mux.ServeHTTP(rec, req)
	covWantCode(t, "with auth", rec, http.StatusOK)
}

// ============ handleStacks 路由分支（405/400）============

func TestCovStacksRouteValidation(t *testing.T) {
	s := covNewServer(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"agent list wrong method", http.MethodPost, "/api/stacks/agents/a1", "", http.StatusMethodNotAllowed},
		{"compose put bad json", http.MethodPut, "/api/stacks/agents/a1/web/compose", "not-json", http.StatusBadRequest},
		{"compose post", http.MethodPost, "/api/stacks/agents/a1/web/compose", "", http.StatusMethodNotAllowed},
		{"up wrong method", http.MethodGet, "/api/stacks/agents/a1/web/up", "", http.StatusMethodNotAllowed},
		{"down wrong method", http.MethodGet, "/api/stacks/agents/a1/web/down", "", http.StatusMethodNotAllowed},
		{"restart wrong method", http.MethodGet, "/api/stacks/agents/a1/web/restart", "", http.StatusMethodNotAllowed},
		{"pull wrong method", http.MethodGet, "/api/stacks/agents/a1/web/pull", "", http.StatusMethodNotAllowed},
		{"logs wrong method", http.MethodPost, "/api/stacks/agents/a1/web/logs", "", http.StatusMethodNotAllowed},
		{"status patch", http.MethodPatch, "/api/stacks/agents/a1/web", "", http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body *strings.Reader
			if c.body != "" {
				body = strings.NewReader(c.body)
			} else {
				body = strings.NewReader("")
			}
			rec := covRec()
			s.handleStacks(rec, covReq(c.method, c.path, body))
			covWantCode(t, c.name, rec, c.want)
		})
	}

	// 非法 URL 转义：task id / stack name 解码失败 → 400（httptest.NewRequest 拒绝，手工构造）
	for _, path := range []string{
		"/api/stacks/agents/a1/tasks/%zz",
		"/api/stacks/agents/a1/%zz",
	} {
		req := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: path}}
		rec := covRec()
		s.handleStacks(rec, req)
		covWantCode(t, path, rec, http.StatusBadRequest)
	}
}

// ============ handleStackRPC 错误映射 ============

func TestCovStackRPCErrorMapping(t *testing.T) {
	// rpc error: busy → 409
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covErrPayload("stack busy: another task running")
	})
	rec := covRec()
	s.handleStacks(rec, covReq(http.MethodPost, "/api/stacks/agents/a1/web/up", nil))
	covWantCode(t, "rpc busy", rec, http.StatusConflict)

	// rpc error: not found → 404；rpc error 其他 → 502（同一 agent 按 method 区分）
	s2 := covNewServer(t)
	covFakeAgent(t, s2, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		if method == "stack.remove" {
			return covErrPayload("stack not found")
		}
		return covErrPayload("boom")
	})
	rec = covRec()
	s2.handleStacks(rec, covReq(http.MethodDelete, "/api/stacks/agents/a1/web", nil))
	covWantCode(t, "rpc not found", rec, http.StatusNotFound)

	rec = covRec()
	s2.handleStacks(rec, covReq(http.MethodGet, "/api/stacks/agents/a1/web", nil))
	covWantCode(t, "rpc other", rec, http.StatusBadGateway)

	// 应答 payload 非法（status 非字符串）→ decode err → 502
	s3 := covNewServer(t)
	covFakeAgent(t, s3, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})
	rec = covRec()
	s3.handleStacks(rec, covReq(http.MethodGet, "/api/stacks/agents/a1/web", nil))
	covWantCode(t, "decode err", rec, http.StatusBadGateway)
}

// ============ handleStackRPC 成功路径 + 审计 + 部署历史 ============

func TestCovStackRPCSuccessAuditDeploy(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		switch method {
		case "stack.status":
			return covOKPayload(map[string]interface{}{"name": "web", "state": "running"})
		case "stack.remove":
			return covOKPayload(nil)
		case "stack.file.save":
			return covOKPayload(map[string]interface{}{"created": true})
		case "stack.up":
			return covOKPayload(map[string]interface{}{"taskId": "cov-task-1"})
		case "stack.task.get":
			return covOKPayload(map[string]interface{}{"taskId": "cov-task-1", "status": "success", "finishedAt": float64(1700000000)})
		}
		return covErrPayload("unexpected method " + method)
	})

	// status 成功（无审计）
	rec := covRec()
	s.handleStacks(rec, covReq(http.MethodGet, "/api/stacks/agents/a1/web", nil))
	covWantCode(t, "status ok", rec, http.StatusOK)

	// remove：审计 username=unknown（无用户上下文）+ data 非 map
	rec = covRec()
	s.handleStacks(rec, covReq(http.MethodDelete, "/api/stacks/agents/a1/web", nil))
	covWantCode(t, "remove ok", rec, http.StatusOK)

	// compose PUT：带用户上下文审计 + created=true → 升级为 stack_create
	body := `{"compose":"services:\n  web:\n    image: nginx\n","env":"A=1"}`
	req := covAuthReq(http.MethodPut, "/api/stacks/agents/a1/web/compose", strings.NewReader(body), "1", "admin", "admin")
	rec = covCallAuth(s, s.handleStacks, req)
	covWantCode(t, "compose save", rec, http.StatusOK)

	// up：taskId → startStackDeployment 落库 + 后台跟踪终态
	req = covAuthReq(http.MethodPost, "/api/stacks/agents/a1/web/up", nil, "1", "admin", "admin")
	rec = covCallAuth(s, s.handleStacks, req)
	covWantCode(t, "up ok", rec, http.StatusOK)

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		list, err := s.db.ListStackDeployments("a1", "web", 10)
		if err == nil && len(list) > 0 && list[0].Status == "success" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	list, err := s.db.ListStackDeployments("a1", "web", 10)
	if err != nil || len(list) == 0 {
		t.Fatalf("deployments = %v, err = %v", list, err)
	}
	if list[0].Status != "success" || list[0].Action != "up" || list[0].TaskID != "cov-task-1" {
		t.Errorf("deployment = %+v", list[0])
	}
}

// ============ startStackDeployment 直调分支 ============

func TestCovStartStackDeploymentBranches(t *testing.T) {
	s := covNewServer(t)

	// 非 tracking 动作 → 直接返回
	s.startStackDeployment("a1", "web", audit.ActionStackUpdate, "t0")
	// 空 stack 名 → 直接返回
	s.startStackDeployment("a1", "", audit.ActionStackUp, "t0")

	// closed db：CreateStackDeployment 失败 → printf 分支
	s2 := covNewServer(t)
	covCloseDB(t, s2)
	s2.startStackDeployment("a1", "web", audit.ActionStackUp, "t1")

	// 成功：记录落库 + 后台跟踪（ghost agent 保持 running，ctx 取消后收尾）
	s.startStackDeployment("a1", "web", audit.ActionStackDown, "cov-t2")
	list, err := s.db.ListStackDeployments("a1", "web", 10)
	if err != nil || len(list) != 1 || list[0].TaskID != "cov-t2" || list[0].Action != "down" {
		t.Fatalf("deployments = %+v, err = %v", list, err)
	}
}

// ============ trackStackTask 直调分支 ============

func TestCovTrackStackTaskBranches(t *testing.T) {
	t.Parallel()

	// ctx 已取消 → 立即返回
	sA := covNewServer(t)
	ctxA, cancelA := context.WithCancel(context.Background())
	sA.ctx = ctxA
	cancelA()
	done := make(chan struct{})
	go func() {
		defer close(done)
		sA.trackStackTask("a1", "cov-tc", 1)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("trackStackTask did not return after ctx cancel")
	}

	// agent 先报 running 再报 success：覆盖 continue 分支 + 终态回填
	sB := covNewServer(t)
	polls := 0
	covFakeAgent(t, sB, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		polls++
		if polls == 1 {
			return covOKPayload(map[string]interface{}{"taskId": params["taskId"], "status": "running"})
		}
		return covOKPayload(map[string]interface{}{"taskId": params["taskId"], "status": "success", "finishedAt": float64(1700000001)})
	})
	if err := sB.db.CreateStackDeployment(&storage.StackDeployment{
		AgentID: "a1", StackName: "web", Action: "up", Status: "running", TaskID: "cov-tb", StartedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	doneB := make(chan struct{})
	go func() {
		defer close(doneB)
		sB.trackStackTask("a1", "cov-tb", 1)
	}()
	select {
	case <-doneB:
	case <-time.After(10 * time.Second):
		t.Fatal("trackStackTask did not reach terminal state")
	}
	list, err := sB.db.ListStackDeployments("a1", "web", 10)
	if err != nil || len(list) == 0 || list[0].Status != "success" {
		t.Errorf("deployment after track = %+v, err = %v", list, err)
	}

	// closed db：FinishStackDeployment 失败 → printf 分支后返回
	sC := covNewServer(t)
	covFakeAgent(t, sC, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"taskId": params["taskId"], "status": "failed", "finishedAt": float64(1700000002)})
	})
	covCloseDB(t, sC)
	doneC := make(chan struct{})
	go func() {
		defer close(doneC)
		sC.trackStackTask("a1", "cov-tc2", 1)
	}()
	select {
	case <-doneC:
	case <-time.After(6 * time.Second):
		t.Fatal("trackStackTask did not return after finish error")
	}
}

// ============ pollStackTask 直调分支 ============

func TestCovPollStackTaskBranches(t *testing.T) {
	// agent 离线 → ("", 0, false)
	s := covNewServer(t)
	if st, _, done := s.pollStackTask("ghost", "t"); st != "" || done {
		t.Errorf("ghost: (%q, %v)", st, done)
	}

	// decode err → false
	s2 := covNewServer(t)
	covFakeAgent(t, s2, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})
	if _, _, done := s2.pollStackTask("a1", "t"); done {
		t.Error("decode err should not be done")
	}

	// rpc error 含 not found → failed 终态
	s3 := covNewServer(t)
	covFakeAgent(t, s3, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covErrPayload("task not found")
	})
	if st, _, done := s3.pollStackTask("a1", "t"); !done || st != "failed" {
		t.Errorf("task not found: (%q, %v)", st, done)
	}

	// rpc error 其他 → false
	s4 := covNewServer(t)
	covFakeAgent(t, s4, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covErrPayload("boom")
	})
	if _, _, done := s4.pollStackTask("a1", "t"); done {
		t.Error("rpc other error should not be done")
	}

	// data 非 map → false
	s5 := covNewServer(t)
	covFakeAgent(t, s5, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload([]interface{}{"x"})
	})
	if _, _, done := s5.pollStackTask("a1", "t"); done {
		t.Error("non-map data should not be done")
	}

	// running（非终态）→ false
	s6 := covNewServer(t)
	covFakeAgent(t, s6, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"status": "running"})
	})
	if _, _, done := s6.pollStackTask("a1", "t"); done {
		t.Error("running should not be done")
	}

	// success / failed 终态
	for _, status := range []string{"success", "failed"} {
		sx := covNewServer(t)
		want := status
		covFakeAgent(t, sx, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
			return covOKPayload(map[string]interface{}{"status": want, "finishedAt": float64(1700000003)})
		})
		st, at, done := sx.pollStackTask("a1", "t")
		if !done || st != want || at != 1700000003 {
			t.Errorf("%s: (%q, %d, %v)", want, st, at, done)
		}
	}
}

// ============ send timeout（5s，并行摊销）============

func TestCovStackTimeoutBranches(t *testing.T) {
	t.Parallel()
	s := covNewServer(t)
	for _, id := range []string{"stuck1", "stuck2", "stuck3", "stuck4"} {
		covStuckDockerAgent(t, s, id)
	}

	calls := []struct {
		name string
		run  func()
	}{
		{"stack rpc timeout", func() {
			rec := covRec()
			s.handleStackRPC(rec, covReq(http.MethodPost, "/u", nil), "stuck1", "stack.up",
				map[string]interface{}{"name": "web"}, audit.ActionStackUp)
			covWantCode(t, "stack rpc timeout", rec, http.StatusGatewayTimeout)
		}},
		{"agent stacks timeout", func() {
			rec := covRec()
			s.handleAgentStacks(rec, covReq(http.MethodGet, "/s", nil), "stuck2")
			covWantCode(t, "agent stacks timeout", rec, http.StatusGatewayTimeout)
		}},
		{"poll timeout", func() {
			if _, _, done := s.pollStackTask("stuck3", "t"); done {
				t.Error("send timeout should not be done")
			}
		}},
		{"stacks all fetch error", func() {
			rec := covRec()
			s.handleStacksAll(rec, covReq(http.MethodGet, "/api/stacks", nil))
			covWantCode(t, "stacks all fetch error", rec, http.StatusOK)
		}},
	}
	var wg sync.WaitGroup
	for _, c := range calls {
		wg.Add(1)
		go func(c struct {
			name string
			run  func()
		}) {
			defer wg.Done()
			c.run()
		}(c)
	}
	wg.Wait()
}

// ============ handleAgentStacks ============

func TestCovAgentStacksBranches(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		switch method {
		case "stack.list":
			return covOKPayload([]interface{}{
				map[string]interface{}{
					"name": "web", "running": float64(1), "total": float64(2),
					"services": []interface{}{map[string]interface{}{"name": "nginx"}},
				},
				"not-a-map", // 非 map 项 → continue
			})
		case "stack.info":
			return covOKPayload(map[string]interface{}{"composeVersion": "v2"})
		}
		return covErrPayload("unexpected method " + method)
	})

	rec := covRec()
	s.handleAgentStacks(rec, covReq(http.MethodGet, "/api/stacks/agents/a1", nil), "a1")
	covWantCode(t, "agent stacks ok", rec, http.StatusOK)
	body := rec.Body.String()
	if !strings.Contains(body, `"name":"web"`) || !strings.Contains(body, "composeVersion") {
		t.Errorf("body missing stacks/info: %s", body)
	}

	// decode err → 502
	s2 := covNewServer(t)
	covFakeAgent(t, s2, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})
	rec = covRec()
	s2.handleAgentStacks(rec, covReq(http.MethodGet, "/api/stacks/agents/a1", nil), "a1")
	covWantCode(t, "agent stacks decode err", rec, http.StatusBadGateway)

	// rpc status 非 success → 502
	s3 := covNewServer(t)
	covFakeAgent(t, s3, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covErrPayload("boom")
	})
	rec = covRec()
	s3.handleAgentStacks(rec, covReq(http.MethodGet, "/api/stacks/agents/a1", nil), "a1")
	covWantCode(t, "agent stacks rpc err", rec, http.StatusBadGateway)
}

// ============ fetchStackInfo 直调分支 ============

func TestCovFetchStackInfoBranches(t *testing.T) {
	// 成功返回 map
	s := covNewServer(t)
	covFakeAgent(t, s, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"composeVersion": "v2"})
	})
	if m := s.fetchStackInfo("a1"); m == nil || m["composeVersion"] != "v2" {
		t.Errorf("info = %v", m)
	}

	// agent 离线 → nil
	if m := s.fetchStackInfo("ghost"); m != nil {
		t.Errorf("ghost info = %v", m)
	}

	// decode err → nil
	s2 := covNewServer(t)
	covFakeAgent(t, s2, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})
	if m := s2.fetchStackInfo("a1"); m != nil {
		t.Errorf("decode err info = %v", m)
	}

	// rpc error → nil
	s3 := covNewServer(t)
	covFakeAgent(t, s3, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covErrPayload("boom")
	})
	if m := s3.fetchStackInfo("a1"); m != nil {
		t.Errorf("rpc err info = %v", m)
	}

	// data 非 map → nil
	s4 := covNewServer(t)
	covFakeAgent(t, s4, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload("just a string")
	})
	if m := s4.fetchStackInfo("a1"); m != nil {
		t.Errorf("non-map info = %v", m)
	}
}

// ============ handleStacksAll 聚合视图 ============

func TestCovStacksAllAggregate(t *testing.T) {
	s := covNewServer(t)

	// a1：在线 + docker，返回两个条目（其一非 map 被过滤）
	covFakeAgent(t, s, "a1", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		switch method {
		case "stack.list":
			return covOKPayload([]interface{}{
				map[string]interface{}{"name": "web", "running": float64(1), "total": float64(2)},
				"not-a-map",
			})
		case "stack.info":
			return covOKPayload(map[string]interface{}{"composeVersion": "v2"})
		}
		return covErrPayload("unexpected method " + method)
	})
	// plain：无 docker 能力 → 聚合时被过滤
	covFakeAgent(t, s, "plain", nil, nil)
	// a3：应答非法 → fetch 失败（灰态）
	covFakeAgent(t, s, "a3", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})

	// 缓存：a1 在线（跳过）、a4 离线但 db 有 agent 记录、a5 离线无记录
	if err := s.db.UpsertStack(&storage.Stack{AgentID: "a1", Name: "cached-online"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertStack(&storage.Stack{AgentID: "a4", Name: "gray", Running: 1, Total: 3, LastAction: "up", LastStatus: "success"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertStack(&storage.Stack{AgentID: "a5", Name: "gray2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertAgent(&storage.Agent{ID: "a4", Hostname: "host-a4"}); err != nil {
		t.Fatal(err)
	}

	rec := covRec()
	s.handleStacksAll(rec, covReq(http.MethodGet, "/api/stacks", nil))
	covWantCode(t, "stacks all", rec, http.StatusOK)
	body := rec.Body.String()
	for _, want := range []string{`"name":"web"`, `"name":"gray"`, `"name":"gray2"`, "host-a4", "composeVersion"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "cached-online") {
		t.Errorf("online agent should skip cache: %s", body)
	}
}

// 聚合超时：agent 应答慢于 10s deadline → 主循环 deadline 分支截断
func TestCovStacksAllDeadline(t *testing.T) {
	t.Parallel()
	s := covNewServer(t)
	covFakeAgent(t, s, "slow", covDockerCaps, func(method string, params map[string]interface{}) map[string]interface{} {
		if method == "stack.list" {
			time.Sleep(11 * time.Second) // 超过 stackAggregateTimeout
			return covOKPayload([]interface{}{})
		}
		return covOKPayload(map[string]interface{}{})
	})

	start := time.Now()
	rec := covRec()
	s.handleStacksAll(rec, covReq(http.MethodGet, "/api/stacks", nil))
	covWantCode(t, "deadline", rec, http.StatusOK)
	if elapsed := time.Since(start); elapsed < 9*time.Second || elapsed > 20*time.Second {
		t.Errorf("elapsed = %v, want ~10s deadline window", elapsed)
	}
}

// ============ stackAgentName / cacheStack ============

func TestCovStackAgentNameBranches(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "reg", nil, nil)
	if got := s.stackAgentName("reg"); got != "host-reg" {
		t.Errorf("registry name = %q", got)
	}
	if err := s.db.UpsertAgent(&storage.Agent{ID: "dbonly", Hostname: "host-db"}); err != nil {
		t.Fatal(err)
	}
	if got := s.stackAgentName("dbonly"); got != "host-db" {
		t.Errorf("db name = %q", got)
	}
	if got := s.stackAgentName("nobody"); got != "nobody" {
		t.Errorf("fallback name = %q", got)
	}
}

func TestCovCacheStackDBError(t *testing.T) {
	s := covNewServer(t)
	covCloseDB(t, s)
	// UpsertStack 失败 → printf 分支（不 panic 即可）
	s.cacheStack("a1", map[string]interface{}{"name": "web", "running": float64(1)})
}

// ============ handleStackHistory ============

func TestCovStackHistoryBranches(t *testing.T) {
	s := covNewServer(t)

	// 空表 → nil 转 []
	rec := covRec()
	s.handleStackHistory(rec, covReq(http.MethodGet, "/api/stacks/agents/a1/web/history", nil), "a1", "web")
	covWantCode(t, "empty history", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"deployments":[]`) {
		t.Errorf("empty history should marshal as [], got %s", rec.Body.String())
	}

	// 有记录
	if err := s.db.CreateStackDeployment(&storage.StackDeployment{
		AgentID: "a1", StackName: "web", Action: "up", Status: "success", TaskID: "h1", StartedAt: 1, FinishedAt: 2,
	}); err != nil {
		t.Fatal(err)
	}
	rec = covRec()
	s.handleStackHistory(rec, covReq(http.MethodGet, "/api/stacks/agents/a1/web/history?limit=5", nil), "a1", "web")
	covWantCode(t, "history with rows", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"taskId":"h1"`) {
		t.Errorf("history body missing row: %s", rec.Body.String())
	}

	// closed db → 500
	s2 := covNewServer(t)
	covCloseDB(t, s2)
	rec = covRec()
	s2.handleStackHistory(rec, covReq(http.MethodGet, "/h", nil), "a1", "web")
	covWantCode(t, "history db error", rec, http.StatusInternalServerError)
}

// ============ 纯函数 ============

func TestCovStackAccessors(t *testing.T) {
	m := map[string]interface{}{"s": "x", "i": float64(7)}
	if stackString(m, "s") != "x" || stackString(m, "missing") != "" {
		t.Error("stackString failed")
	}
	if stackInt64(m, "i") != 7 || stackInt64(m, "missing") != 0 {
		t.Error("stackInt64 failed")
	}
}
