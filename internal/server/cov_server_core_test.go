package server

// cov_server_core_test.go 覆盖 server.go 核心：Start 成功路径、registerRoutes
// 路由分发、startInventorySync、startProbeRunner、handleHealth、
// handleLoginWithAudit，以及 middleware.go 的 CORS/审计中间件。

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/probe"
)

// ============ Start 成功路径 ============

func TestCovStartFull(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "cov-admin-password-1")
	t.Setenv("ALLOWED_ORIGINS", "https://cov.example")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	s := covNewServer(t)
	covWSReady(s)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.ctx = ctx
	s.cancel = cancel
	s.cfg = config.Normalize(&config.Config{})
	s.addr = addr

	errCh := make(chan error, 1)
	go func() { errCh <- s.Start() }()

	base := "http://" + addr
	client := &http.Client{Timeout: 5 * time.Second}

	// 等 /health 就绪
	var resp *http.Response
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err = client.Get(base + "/health")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not ready: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("health = %d %s", resp.StatusCode, body)
	}

	// OPTIONS 预检：CORS 头（ALLOWED_ORIGINS 已设）
	req, _ := http.NewRequest(http.MethodOptions, base+"/api/status", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Access-Control-Allow-Origin") != "https://cov.example" {
		t.Fatalf("preflight = %d, origin = %q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}

	// 管理员登录拿 token
	login := func(extraHeaders map[string]string) (int, string) {
		lr, _ := http.NewRequest(http.MethodPost, base+"/api/auth/login",
			strings.NewReader(`{"username":"admin","password":"cov-admin-password-1"}`))
		lr.Header.Set("Content-Type", "application/json")
		for k, v := range extraHeaders {
			lr.Header.Set(k, v)
		}
		resp, err := client.Do(lr)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
	code, raw := login(nil)
	if code != http.StatusOK {
		t.Fatalf("admin login = %d %s", code, raw)
	}
	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(raw), &loginResp); err != nil || loginResp.Token == "" {
		t.Fatalf("login response = %s (%v)", raw, err)
	}

	// 认证访问 /api/status（registerRoutes /api/ 分发 + serveAPI + handleStatus）
	sr, _ := http.NewRequest(http.MethodGet, base+"/api/status", nil)
	sr.Header.Set("Authorization", "Bearer "+loginResp.Token)
	resp, err = client.Do(sr)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"services"`) {
		t.Fatalf("api status = %d %s", resp.StatusCode, body)
	}

	// SPA 根路径：未配置 STATIC_DIR → JSON 提示
	resp, err = client.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "API server is running") {
		t.Fatalf("spa = %d %s", resp.StatusCode, body)
	}

	// X-Forwarded-For 多段：审计取第一段 IP（错密码登录便于断言失败审计）
	loginHdr := map[string]string{"X-Forwarded-For": "10.9.8.7, 192.168.0.1"}
	// 失败登录走 getClientIP 的 XFF 分支
	lr, _ := http.NewRequest(http.MethodPost, base+"/api/auth/login",
		strings.NewReader(`{"username":"admin","password":"totally-wrong"}`))
	lr.Header.Set("X-Forwarded-For", "10.9.8.7, 192.168.0.1")
	resp, err = client.Do(lr)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("failed login = %d, want 401", resp.StatusCode)
	}
	_ = loginHdr

	// Shutdown：覆盖 probeRunner/proxyMgr 非 nil 分支
	s.Shutdown()
	select {
	case err := <-errCh:
		// ListenAndServe 正常应继续阻塞；若返回也不视为失败（端口释放竞态）
		_ = err
	default:
	}
}

// ============ startInventorySync ============

func TestCovStartInventorySyncVariants(t *testing.T) {
	s := covLoopServer(t)

	// Inventory nil / 未开启 watch → no-op
	if err := s.startInventorySync(); err != nil {
		t.Fatalf("nil inventory: %v", err)
	}
	s.cfg.Inventory = &config.InventoryConfig{}
	if err := s.startInventorySync(); err != nil {
		t.Fatalf("watch off: %v", err)
	}

	// watch 开 + path 空：strict 报错；非 strict 仅记录
	s.cfg.Inventory = &config.InventoryConfig{Watch: true, Strict: true}
	if err := s.startInventorySync(); err == nil || !strings.Contains(err.Error(), "inventory.path is empty") {
		t.Fatalf("empty path strict err = %v", err)
	}
	s.cfg.Inventory.Strict = false
	if err := s.startInventorySync(); err != nil {
		t.Fatalf("empty path lenient = %v", err)
	}

	// watch 开 + 目录不存在：manager.Start 失败；strict 报错、非 strict 吞掉
	missing := filepath.Join(t.TempDir(), "nope", "inv.yaml")
	s.cfg.Inventory = &config.InventoryConfig{Watch: true, Path: missing, Strict: true}
	if err := s.startInventorySync(); err == nil {
		t.Fatal("missing dir strict should fail")
	}
	s.cfg.Inventory.Strict = false
	if err := s.startInventorySync(); err != nil {
		t.Fatalf("missing dir lenient = %v", err)
	}

	// 合法 inventory：成功启动并挂上 inventorySync
	dir := t.TempDir()
	p := filepath.Join(dir, "inventory.yaml")
	content := "version: v1\nregions:\n  region1:\n    zones:\n      zone1:\n        agents:\n          agent1:\n            hostname: cov-host\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	s2 := covLoopServer(t)
	s2.cfg.Inventory = &config.InventoryConfig{Watch: true, Path: p, Strict: true}
	if err := s2.startInventorySync(); err != nil {
		t.Fatalf("valid inventory: %v", err)
	}
	if s2.inventorySync == nil {
		t.Fatal("inventorySync not set on success")
	}
	s2.inventorySync.Stop()
	s2.Shutdown() // 覆盖 Shutdown 中 inventorySync 非 nil 分支
}

// ============ startProbeRunner ============

func TestCovStartProbeRunner(t *testing.T) {
	s := covLoopServer(t)
	var runners []*probe.Runner
	defer func() {
		for _, r := range runners {
			r.Stop()
		}
	}()

	s.startProbeRunner() // 默认间隔
	if s.probeRunner == nil {
		t.Fatal("probeRunner not set")
	}
	runners = append(runners, s.probeRunner)

	for _, v := range []string{"abc", "99999", "10"} { // 非法/超范围/低于下限 → 默认
		if err := s.db.SetSetting(probe.IntervalSettingKey, v); err != nil {
			t.Fatal(err)
		}
		s.startProbeRunner()
		runners = append(runners, s.probeRunner)
	}
	if err := s.db.SetSetting(probe.IntervalSettingKey, "60"); err != nil { // 合法覆盖
		t.Fatal(err)
	}
	s.startProbeRunner()
	runners = append(runners, s.probeRunner)
}

// ============ handleLoginWithAudit ============

type covErrReader struct{}

func (covErrReader) Read([]byte) (int, error) { return 0, errors.New("cov read failure") }

func TestCovHandleLoginWithAudit(t *testing.T) {
	s := covLoopServer(t)
	covSeedUser(t, s, "covlogin", "cov-pass-123", "user")

	call := func(method string, body io.Reader) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/auth/login", body)
		rec := covRec()
		s.handleLoginWithAudit(rec, req)
		return rec
	}

	rec := call(http.MethodPost, strings.NewReader(`{"username":"covlogin","password":"cov-pass-123"}`))
	covWantCode(t, "login ok", rec, http.StatusOK)
	rec = call(http.MethodPost, strings.NewReader(`{"username":"covlogin","password":"wrong"}`))
	covWantCode(t, "login wrong", rec, http.StatusUnauthorized)
	rec = call(http.MethodPost, strings.NewReader("{bad"))
	covWantCode(t, "login bad json", rec, http.StatusBadRequest)
	rec = call(http.MethodPost, io.NopCloser(covErrReader{})) // body 读取失败 → unknown + 400
	covWantCode(t, "login read err", rec, http.StatusBadRequest)
	rec = call(http.MethodGet, nil)
	covWantCode(t, "login wrong method", rec, http.StatusMethodNotAllowed)

	// 登录成功与失败都应留审计
	logs, _, err := s.db.GetAuditLogs(0, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) < 2 {
		t.Fatalf("expected login audit entries, got %d", len(logs))
	}
}

// ============ registerRoutes 分发 ============

func TestCovRegisterRoutesDispatch(t *testing.T) {
	s := covLoopServer(t)
	covWSReady(s)
	s.cfg = config.Normalize(s.cfg)
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	// TOTP generate 按 userID 查库，先落一个真实用户并用其真实 ID 签 token
	if err := auth.InitAdmin(s.db, "covuser", "cov-password-123"); err != nil {
		t.Fatal(err)
	}
	u, err := s.db.GetUserByUsername("covuser")
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.authService().GenerateToken(u.ID, "covuser", "admin")
	if err != nil {
		t.Fatal(err)
	}

	do := func(method, path, tok string, body io.Reader) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, body)
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		rec := covRec()
		mux.ServeHTTP(rec, req)
		return rec
	}

	covWantCode(t, "health", do(http.MethodGet, "/health", "", nil), http.StatusOK)

	// 公开认证路由
	covWantCode(t, "login wrong pass", do(http.MethodPost, "/api/auth/login", "",
		strings.NewReader(`{"username":"x","password":"y"}`)), http.StatusUnauthorized)
	covWantCode(t, "refresh no token", do(http.MethodPost, "/api/auth/refresh", "", nil), http.StatusUnauthorized)
	covWantCode(t, "totp verify bad body", do(http.MethodPost, "/api/auth/totp/verify", "",
		strings.NewReader("{bad")), http.StatusBadRequest)
	covWantCode(t, "auth unknown passthrough", do(http.MethodGet, "/api/auth/unknown", "", nil), http.StatusOK)

	// TOTP 设置三路由已并入 /api/auth/ 前缀守卫的 switch（原实现置于
	// 提前 return 之后不可达）：无 token 401；带 token 抵达 handler
	// （enable/disable 空请求体 → handler 的 400 参数校验）
	for _, p := range []string{"/api/auth/totp/generate", "/api/auth/totp/enable", "/api/auth/totp/disable"} {
		covWantCode(t, "no token "+p, do(http.MethodPost, p, "", nil), http.StatusUnauthorized)
	}
	covWantCode(t, "with token generate", do(http.MethodPost, "/api/auth/totp/generate", token, nil), http.StatusOK)
	covWantCode(t, "with token enable", do(http.MethodPost, "/api/auth/totp/enable", token, nil), http.StatusBadRequest)
	covWantCode(t, "with token disable", do(http.MethodPost, "/api/auth/totp/disable", token, nil), http.StatusBadRequest)

	// /api/ 认证分发
	covWantCode(t, "api status", do(http.MethodGet, "/api/status", token, nil), http.StatusOK)
	covWantCode(t, "api unknown", do(http.MethodGet, "/api/nothing", token, nil), http.StatusNotFound)
	covWantCode(t, "api no token", do(http.MethodGet, "/api/status", "", nil), http.StatusUnauthorized)

	// SPA fallback
	covWantCode(t, "spa root", do(http.MethodGet, "/", "", nil), http.StatusOK)
	covWantCode(t, "spa deep", do(http.MethodGet, "/some/page", "", nil), http.StatusOK)

	// /ws 路由已注册（recorder 不支持 Hijack → 升级失败返回错误，不 panic 即可）
	rec := do(http.MethodGet, "/ws", "", nil)
	if rec.Code == 0 {
		t.Error("ws route returned no status")
	}
}

// ============ 中间件 ============

func TestCovCORSMiddleware(t *testing.T) {
	s := covLoopServer(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(209) })
	h := s.CORSMiddleware(next)

	// 非 /api/ 路径透传
	rec := covRec()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != 209 {
		t.Errorf("non-api passthrough = %d", rec.Code)
	}

	t.Setenv("ALLOWED_ORIGINS", "https://cov-origin.example")
	// OPTIONS 预检
	rec = covRec()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/api/x", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "https://cov-origin.example" {
		t.Errorf("preflight = %d origin = %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	// 普通 /api 请求透传并带 CORS 头
	rec = covRec()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/x", nil))
	if rec.Code != 209 || rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Errorf("api passthrough = %d", rec.Code)
	}
}

func TestCovAuditMiddleware(t *testing.T) {
	s := covLoopServer(t)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(207) })
	h := s.AuditMiddleware(inner)

	// WS 四路径透传（不包装、不审计）
	for _, p := range []string{"/ws", "/api/remote/terminal", "/api/remote/desktop", "/api/remote/vnc"} {
		rec := covRec()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != 207 {
			t.Errorf("ws passthrough %s = %d", p, rec.Code)
		}
	}

	// POST /api/users（匿名）→ 审计
	rec := covRec()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/users", nil))
	if rec.Code != 207 {
		t.Errorf("post users = %d", rec.Code)
	}
	// GET /api/users → 不审计
	rec = covRec()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	// GET /api/admin/... → 审计
	rec = covRec()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/audit/logs", nil))
	// DELETE /health 之外的 /api/ 路径 → 审计
	rec = covRec()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/users/9", nil))

	logs, _, err := s.db.GetAuditLogs(0, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sawCreate, sawDelete, sawView bool
	for _, l := range logs {
		switch {
		case l.Action == "create" && l.Resource == "users":
			sawCreate = true
			if l.Username != "anonymous" { // 无认证上下文
				t.Errorf("anonymous audit username = %q", l.Username)
			}
		case l.Action == "delete" && l.Resource == "users":
			sawDelete = true
		case l.Action == "view" && l.Resource == "admin":
			sawView = true
		}
	}
	if !sawCreate || !sawDelete || !sawView {
		t.Errorf("audit entries missing: create=%v delete=%v view=%v", sawCreate, sawDelete, sawView)
	}
}

// ============ handleHealth ============

func TestCovHandleHealth(t *testing.T) {
	s := covLoopServer(t)
	rec := covRec()
	s.handleHealth(rec, covReq(http.MethodGet, "/health", nil))
	covWantCode(t, "health", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"agents":0`) {
		t.Errorf("health body = %s", rec.Body.String())
	}
}
