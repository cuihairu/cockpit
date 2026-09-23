package server

// cov_more_gaps_test.go 第二轮收尾：Start 错误路径、远控 WS 升级失败、
// 审计中间件用户上下文分支、SPA 静态目录、agent 密钥管理、stack 部署
// 跟踪、代理运行态列表、空列表兜底、注册拒绝透传、录制写失败等。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/proxy/mgr"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ Start 错误路径 ============

func TestCovStartErrorPaths(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "cov-strong-pass-9")
	s := covLoopServer(t)
	s.cfg = config.Normalize(s.cfg)
	// watch + strict + 空 path → startInventorySync 报错；关库触发 InitAdmin
	// 与 proxyMgr.Start 的失败日志（均不中断启动）
	s.cfg.Inventory = &config.InventoryConfig{Watch: true, Strict: true}
	covCloseDB(t, s)

	err := s.Start()
	if err == nil || !strings.Contains(err.Error(), "start inventory sync") {
		t.Fatalf("Start() = %v, want inventory sync error", err)
	}
}

// ============ 远控 WS 升级失败 ============

func TestCovRemoteWSUpgradeFailures(t *testing.T) {
	s := covRemoteSetup(t)
	covRegisterBareAgent(t, s, "agent-up2")

	cases := []struct {
		desc    string
		handler http.HandlerFunc
		ticket  string
	}{
		{"terminal upgrade", s.handleTerminalWebSocket, covTerminalTicket(t, s, "agent-up2")},
		{"desktop upgrade", s.handleDesktopWebSocket, covTicket(t, s, map[string]string{
			"agent_id": "agent-up2", "host": "127.0.0.1", "port": "3389", "protocol": "rdp",
		})},
		{"vnc upgrade", s.handleVNCWebSocket, covTicket(t, s, map[string]string{
			"agent_id": "agent-up2", "host": "127.0.0.1", "port": "5900", "protocol": "vnc",
		})},
	}
	for _, c := range cases {
		req := covReq(http.MethodGet, "/api/remote/x", nil)
		req.Header.Add("Sec-WebSocket-Protocol", c.ticket) // 合法票据，通过校验进入升级
		rec := covRec()                                    // recorder 不支持 Hijack → Upgrade 失败
		c.handler(rec, req)
		if rec.Code == 0 {
			t.Errorf("%s: no status written", c.desc)
		}
	}
}

// ============ 审计中间件用户上下文 ============

func TestCovAuditMiddlewareUserContext(t *testing.T) {
	s := covLoopServer(t)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(210) })
	// auth.Middleware 在闭包内重绑定 r，生产链路中外层 AuditMiddleware 拿到的
	// 原始 r 不含用户上下文（见报告疑似 bug #4）；这里直接验证被包裹后的行为
	h := auth.Middleware(http.HandlerFunc(s.AuditMiddleware(inner).ServeHTTP))

	rec := covRec()
	h.ServeHTTP(rec, covAuthReq(http.MethodPost, "/api/users", nil, "7", "covctx", "admin"))
	if rec.Code != 210 {
		t.Fatalf("audit middleware chain = %d", rec.Code)
	}
	logs, _, err := s.db.GetAuditLogs(0, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if l.Action == "create" && l.Resource == "users" && l.Username == "covctx" {
			found = true
		}
	}
	if !found {
		t.Errorf("audit entry with covctx not found among %d logs", len(logs))
	}
}

// ============ SPA 静态目录 ============

func TestCovSPAStaticDir(t *testing.T) {
	s := covLoopServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>cov-spa</html>"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STATIC_DIR", dir)
	h := s.spaHandler()

	rec := covRec()
	h.ServeHTTP(rec, covReq(http.MethodGet, "/api/whatever", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("spa api path = %d, want 404", rec.Code)
	}
	rec = covRec()
	h.ServeHTTP(rec, covReq(http.MethodGet, "/ws", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("spa ws path = %d, want 404", rec.Code)
	}
	// 已存在文件直出 + SPA fallback 到 index.html
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log(1)"), 0644); err != nil {
		t.Fatal(err)
	}
	rec = covRec()
	h.ServeHTTP(rec, covReq(http.MethodGet, "/app.js", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "console.log") {
		t.Errorf("spa static file = %d %s", rec.Code, rec.Body.String())
	}
	rec = covRec()
	h.ServeHTTP(rec, covReq(http.MethodGet, "/deep/route", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "cov-spa") {
		t.Errorf("spa fallback = %d %s", rec.Code, rec.Body.String())
	}
}

// ============ agent 密钥管理 ============

func TestCovAgentSecretHandler(t *testing.T) {
	s := covLoopServer(t)
	if err := s.db.UpsertAgent(&storage.Agent{ID: "agent-sec", Hostname: "h"}); err != nil {
		t.Fatal(err)
	}
	do := func(method, id string) *httptest.ResponseRecorder {
		return covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
			s.handleAgentSecret(w, r, id)
		}, covAuthReq(method, "/api/agents/"+id+"/secret", nil, "1", "covadmin", "admin"))
	}
	// agent 不存在 → 404（非 admin 判定已收敛到 RBAC 中间件）
	rec := do(http.MethodGet, "ghost")
	covWantCode(t, "secret missing", rec, http.StatusNotFound)

	rec = do(http.MethodGet, "agent-sec")
	covWantCode(t, "secret status", rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), `"hasSecret":true`) {
		t.Errorf("fresh agent should have no secret: %s", rec.Body.String())
	}
	rec = do(http.MethodPost, "agent-sec")
	covWantCode(t, "secret regenerate", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"secret"`) {
		t.Errorf("regenerate body = %s", rec.Body.String())
	}
	rec = do(http.MethodGet, "agent-sec")
	if !strings.Contains(rec.Body.String(), `"hasSecret":true`) {
		t.Errorf("agent should have secret: %s", rec.Body.String())
	}
	rec = do(http.MethodDelete, "agent-sec")
	covWantCode(t, "secret remove", rec, http.StatusOK)
	got, err := s.db.GetAgent("agent-sec")
	if err != nil || got.SecretHash != "" {
		t.Errorf("secret should be removed: %v %v", got, err)
	}
}

// do2 以管理员身份调用 handleAgentSecret
func do2(s *Server, method, id string) *httptest.ResponseRecorder {
	return covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleAgentSecret(w, r, id)
	}, covAuthReq(method, "/api/agents/"+id+"/secret", nil, "1", "covadmin", "admin"))
}

// ============ stack 部署跟踪（auditAction + taskId） ============

func TestCovStackDeploymentTracking(t *testing.T) {
	s := covLoopServer(t)
	covFakeAgent(t, s, "agent-dep", []string{"docker"}, func(method string, params map[string]interface{}) map[string]interface{} {
		switch method {
		case "stack.up":
			return covOKPayload(map[string]interface{}{"taskId": "task-dep"})
		case "stack.task.get":
			return covOKPayload(map[string]interface{}{"status": "success"})
		}
		return covErrPayload("unexpected " + method)
	})
	rec := covRec()
	s.handleStackRPC(rec, covReq(http.MethodPost, "/x", nil), "agent-dep", "stack.up",
		map[string]interface{}{"name": "web"}, audit.ActionStackUp)
	covWantCode(t, "stack up", rec, http.StatusOK)

	// 部署历史应先 running 后 success（轮询间隔 2s）
	covWaitGone(t, "deployment success", func() bool {
		list, err := s.db.ListStackDeployments("agent-dep", "web", 10)
		return err == nil && len(list) == 1 && list[0].Status == "success"
	})
}

// ============ CallAgent 发送超时（Send 缓冲塞满） ============

// covStuckStackAgent 注册带 docker 能力且 Send 已塞满的 agent（发送 5s 超时）
func covStuckStackAgent(t *testing.T, s *Server, agentID string) {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Capabilities = append(agent.Capabilities, protocol.Capability{Type: "docker"})
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	for i := 0; i < 256; i++ {
		agent.Send <- protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{})
	}
	t.Cleanup(func() { s.registry.Unregister(agentID) })
}

func TestCovSendTimeoutErrorMapping(t *testing.T) {
	s := covLoopServer(t)
	covStuckStackAgent(t, s, "agent-slow")
	covStuckStackAgent(t, s, "agent-slow2")

	start := time.Now()
	rec := covRec()
	s.handleStackRPC(rec, covReq(http.MethodPost, "/x", nil), "agent-slow", "stack.list", nil, "")
	covWantCode(t, "stack rpc send timeout", rec, http.StatusGatewayTimeout)

	rec = covRec()
	s.handleAgentStacks(rec, covReq(http.MethodGet, "/x", nil), "agent-slow2")
	covWantCode(t, "agent stacks send timeout", rec, http.StatusGatewayTimeout)
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Errorf("timeouts took too long: %v", elapsed)
	}
}

func TestCovPollBackupTaskSendTimeout(t *testing.T) {
	s := covLoopServer(t)
	covStuckAgent(t, s, "agent-poll") // Send 已满 → 5s send timeout → failed 终态
	res, done := s.pollBackupTask("agent-poll", "task-t")
	if !done || res.Status != "failed" || !strings.Contains(res.Error, "send timeout") {
		t.Fatalf("poll send timeout = %+v %v", res, done)
	}
}

// ============ 代理运行态 ============

func TestCovProxyRunningStatus(t *testing.T) {
	s := covLoopServer(t)
	covSeedProxyAgent(t, s, "agent-px")
	covSeedProxy(t, s, "prun", 18600)
	s.proxyMgr = mgr.NewManager(s, s.db)
	cfg, err := s.db.GetProxy("prun")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.proxyMgr.StartProxy(cfg); err != nil {
		t.Fatal(err)
	}

	// 列表应带运行状态（GetProxyStatus 成功分支）
	rec := covCallAuth(s, s.handleProxies,
		covAuthReq(http.MethodGet, "/api/proxies", nil, "1", "covadmin", "admin"))
	covWantCode(t, "proxies running", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"status":"running"`) {
		t.Errorf("proxies body = %s", rec.Body.String())
	}

	// 单个代理状态成功分支
	rec = covCallAuth(s, s.handleProxyStatus,
		covAuthReq(http.MethodGet, "/api/proxies/status?id=prun", nil, "1", "covadmin", "admin"))
	covWantCode(t, "proxy status one", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "running") {
		t.Errorf("proxy status body = %s", rec.Body.String())
	}
	s.proxyMgr.StopProxy("prun")
}

// ============ 空列表兜底 ============

func TestCovEmptyListFallbacks(t *testing.T) {
	s := covLoopServer(t)

	rec := covRec()
	s.handleRecordingsList(rec, covReq(http.MethodGet, "/api/recordings", nil))
	covWantCode(t, "recordings empty", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("recordings empty body = %s", rec.Body.String())
	}

	rec = covRec()
	s.handleProbeHistory(rec, covReq(http.MethodGet,
		"/api/probe/history?resource_type=service&resource_id=svc-1", nil))
	covWantCode(t, "probe history empty", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"results":[]`) {
		t.Errorf("probe history body = %s", rec.Body.String())
	}
}

// ============ 注册拒绝透传 ============

func TestCovRegisterRejectionPassthrough(t *testing.T) {
	s := covLoopServer(t)
	hash, err := storage.HashAgentSecret("right-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertAgent(&storage.Agent{ID: "agent-rej", Hostname: "h", SecretHash: hash}); err != nil {
		t.Fatal(err)
	}
	_, rejection, err := s.registerAgentConnection(nil, &protocol.RegisterPayload{
		AgentID: "agent-rej", Secret: "wrong-secret",
	})
	if err != nil || rejection == nil || rejection.code != "authentication_failed" {
		t.Fatalf("rejection passthrough = %v, %v", rejection, err)
	}
}

// ============ 录制写失败 ============

func TestCovRecordingWriteFailure(t *testing.T) {
	dir := t.TempDir()
	rec, err := newCastRecorder(filepath.Join(dir, "wfail.cast"), "s1", "t", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// 将底层文件换成 /dev/full：写必失败 → 停录分支
	full, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("/dev/full unavailable: %v", err)
	}
	rec.f = full
	rec.WriteOutput([]byte("will-fail"))
	if rec.f != nil {
		t.Error("recorder should stop after write failure")
	}
	// 停录后的输出直接丢弃（f 已置 nil）
	rec.WriteOutput([]byte("after-stop"))
	rec.Close()
}
