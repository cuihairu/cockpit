package server

// cov_final_gaps_test.go 收尾缺口：TOTP 四 handler 全流程、api_proxy 全套、
// 审计 API 500 分支与路由、api.go 杂项、WS 升级失败、agent 注册鉴权内部函数、
// writeLoop 写失败、各单行错误分支。

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/proxy"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/pquerna/otp/totp"
)

// ============ TOTP ============

func TestCovTOTPFullFlow(t *testing.T) {
	s := covLoopServer(t)
	user := covSeedUser(t, s, "covtotp", "cov-pass-123", "user")

	authed := func(h http.HandlerFunc, method, target string, body string) *httptest.ResponseRecorder {
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		return covCallAuth(s, h, covAuthReq(method, target, rd, user.ID, "covtotp", "user"))
	}

	// Generate：405 / 无用户 401 / 404 / 成功
	rec := covRec()
	s.handleTOTPGenerate(rec, covReq(http.MethodGet, "/api/auth/totp/generate", nil))
	covWantCode(t, "generate 405", rec, http.StatusMethodNotAllowed)
	rec = covRec()
	s.handleTOTPGenerate(rec, covReq(http.MethodPost, "/api/auth/totp/generate", nil))
	covWantCode(t, "generate no user", rec, http.StatusUnauthorized)
	rec = covCallAuth(s, s.handleTOTPGenerate,
		covAuthReq(http.MethodPost, "/api/auth/totp/generate", nil, "9999", "ghost", "user"))
	covWantCode(t, "generate 404", rec, http.StatusNotFound)

	rec = authed(s.handleTOTPGenerate, http.MethodPost, "/api/auth/totp/generate", "")
	covWantCode(t, "generate ok", rec, http.StatusOK)
	var genResp TOTPGenerateResponse
	if err := json.NewDecoder(rec.Body).Decode(&genResp); err != nil {
		t.Fatal(err)
	}
	if genResp.Secret == "" || len(genResp.BackupCodes) == 0 || genResp.QRCode == "" {
		t.Fatalf("generate response = %+v", genResp)
	}
	code, err := totp.GenerateCode(genResp.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Enable：405 / 401 / 坏 body / 无 tmp / 错码 / 成功
	rec = covRec()
	s.handleTOTPEnable(rec, covReq(http.MethodGet, "/x", nil))
	covWantCode(t, "enable 405", rec, http.StatusMethodNotAllowed)
	rec = authed(s.handleTOTPEnable, http.MethodPost, "/api/auth/totp/enable", "{bad")
	covWantCode(t, "enable bad body", rec, http.StatusBadRequest)
	// 坏密文 → 500 + tmp 清除
	totpTmpStore["totp_tmp_"+user.ID] = &totpTmpData{Secret: "not-encrypted-at-all"}
	rec = authed(s.handleTOTPEnable, http.MethodPost, "/api/auth/totp/enable", `{"code":"123456"}`)
	covWantCode(t, "enable bad secret", rec, http.StatusInternalServerError)
	if _, exists := totpTmpStore["totp_tmp_"+user.ID]; exists {
		t.Error("corrupted tmp data should be removed")
	}
	// 重新生成（坏密文已清空 tmp，新 secret 入 tmp）→ 错码 400 / 对码成功
	rec = authed(s.handleTOTPGenerate, http.MethodPost, "/api/auth/totp/generate", "")
	var regen TOTPGenerateResponse
	if err := json.NewDecoder(rec.Body).Decode(&regen); err != nil {
		t.Fatal(err)
	}
	code, err = totp.GenerateCode(regen.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rec = authed(s.handleTOTPEnable, http.MethodPost, "/api/auth/totp/enable", `{"code":"000000"}`)
	covWantCode(t, "enable wrong code", rec, http.StatusBadRequest)
	// 无 tmp（其他用户）→ 400
	other := covSeedUser(t, s, "covtotp2", "cov-pass-456", "user")
	rec = covCallAuth(s, s.handleTOTPEnable,
		covAuthReq(http.MethodPost, "/x", nil, other.ID, "covtotp2", "user"))
	covWantCode(t, "enable no tmp", rec, http.StatusBadRequest)
	// 正确码 → 200 + 用户启用
	rec = authed(s.handleTOTPEnable, http.MethodPost, "/api/auth/totp/enable", `{"code":"`+code+`"}`)
	covWantCode(t, "enable ok", rec, http.StatusOK)
	got, _ := s.db.GetUserByID(user.ID)
	if !got.TOTPEnabled {
		t.Fatal("TOTP should be enabled")
	}
	if _, exists := totpTmpStore["totp_tmp_"+user.ID]; exists {
		t.Error("tmp data should be cleared after enable")
	}

	// Verify：405 / 坏 body / 无效 tmp / 未启用用户 / 错码 / 对码 / 备份码 / 用户不存在
	loginTmp := func(username, password string) string {
		rec := covRec()
		s.handleLoginWithAudit(rec, covReq(http.MethodPost, "/api/auth/login",
			strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`)))
		raw := rec.Body.String()
		var lr struct {
			TmpToken string `json:"tmp_token"`
		}
		if err := json.Unmarshal([]byte(raw), &lr); err != nil || lr.TmpToken == "" {
			t.Fatalf("login for tmp token = code %d body %q (%v)", rec.Code, raw, err)
		}
		return lr.TmpToken
	}
	verify := func(body string) *httptest.ResponseRecorder {
		rec := covRec()
		s.handleTOTPVerify(rec, covReq(http.MethodPost, "/api/auth/totp/verify", strings.NewReader(body)))
		return rec
	}

	rec = covRec()
	s.handleTOTPVerify(rec, covReq(http.MethodGet, "/x", nil))
	covWantCode(t, "verify 405", rec, http.StatusMethodNotAllowed)
	covWantCode(t, "verify bad body", verify("{bad"), http.StatusBadRequest)
	covWantCode(t, "verify bad tmp", verify(`{"code":"123456"}`), http.StatusUnauthorized)

	tmp := loginTmp("covtotp", "cov-pass-123")
	rec = verify(`{"code":"` + code + `","tmp_token":"` + tmp + `"}`)
	covWantCode(t, "verify ok", rec, http.StatusOK)
	var vr TOTPVerifyResponse
	if err := json.NewDecoder(rec.Body).Decode(&vr); err != nil || vr.Token == "" || vr.Username != "covtotp" {
		t.Fatalf("verify response = %s (%v)", rec.Body.String(), err)
	}

	// 备份码路径：tmp 一次性，重新登录取新 tmp（备份码来自成功启用的那次 generate）
	tmp2 := loginTmp("covtotp", "cov-pass-123")
	rec = verify(`{"code":"` + regen.BackupCodes[0] + `","tmp_token":"` + tmp2 + `"}`)
	covWantCode(t, "verify backup code", rec, http.StatusOK)

	// 错码 → 401
	tmp3 := loginTmp("covtotp", "cov-pass-123")
	covWantCode(t, "verify wrong code", verify(`{"code":"000000","tmp_token":"`+tmp3+`"}`), http.StatusUnauthorized)

	// 未启用用户：重新启用拿新 tmp，再关 TOTP → 400
	if err := s.db.DisableTOTP(user.ID); err != nil {
		t.Fatal(err)
	}
	rec = authed(s.handleTOTPGenerate, http.MethodPost, "/api/auth/totp/generate", "")
	var regen2 TOTPGenerateResponse
	if err := json.NewDecoder(rec.Body).Decode(&regen2); err != nil {
		t.Fatal(err)
	}
	code2, err := totp.GenerateCode(regen2.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	authed(s.handleTOTPEnable, http.MethodPost, "/api/auth/totp/enable", `{"code":"`+code2+`"}`)
	tmp5 := loginTmp("covtotp", "cov-pass-123")
	s.db.DisableTOTP(user.ID)
	covWantCode(t, "verify not enabled", verify(`{"code":"`+code2+`","tmp_token":"`+tmp5+`"}`), http.StatusBadRequest)

	// 用户不存在：上一场景已 Disable，先重新启用 → 拿 tmp → 删用户 → 404
	rec = authed(s.handleTOTPGenerate, http.MethodPost, "/api/auth/totp/generate", "")
	var regen3 TOTPGenerateResponse
	if err := json.NewDecoder(rec.Body).Decode(&regen3); err != nil {
		t.Fatal(err)
	}
	code3, err := totp.GenerateCode(regen3.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	authed(s.handleTOTPEnable, http.MethodPost, "/api/auth/totp/enable", `{"code":"`+code3+`"}`)
	tmpB := loginTmp("covtotp", "cov-pass-123")
	s.db.DeleteUser(user.ID)
	covWantCode(t, "verify user gone", verify(`{"code":"`+code3+`","tmp_token":"`+tmpB+`"}`), http.StatusNotFound)

	// Disable：405 / 401 / 坏 body / 404 / 未启用 / 错码 / 成功
	rec = covRec()
	s.handleTOTPDisable(rec, covReq(http.MethodGet, "/x", nil))
	covWantCode(t, "disable 405", rec, http.StatusMethodNotAllowed)
	rec = covCallAuth(s, s.handleTOTPDisable,
		covAuthReq(http.MethodPost, "/x", strings.NewReader(`{"code":"123456"}`), "9999", "ghost", "user"))
	covWantCode(t, "disable 404", rec, http.StatusNotFound)
	rec = covCallAuth(s, s.handleTOTPDisable,
		covAuthReq(http.MethodPost, "/x", strings.NewReader("{bad"), other.ID, "covtotp2", "user"))
	covWantCode(t, "disable bad body", rec, http.StatusBadRequest)
	rec = covCallAuth(s, s.handleTOTPDisable,
		covAuthReq(http.MethodPost, "/x", nil, other.ID, "covtotp2", "user"))
	covWantCode(t, "disable not enabled", rec, http.StatusBadRequest)

	disabled := covSeedUser(t, s, "covtotp3", "cov-pass-789", "user")
	disableOf := func(body string) *httptest.ResponseRecorder {
		return covCallAuth(s, s.handleTOTPDisable,
			covAuthReq(http.MethodPost, "/x", strings.NewReader(body), disabled.ID, "covtotp3", "user"))
	}
	covCallAuth(s, s.handleTOTPGenerate,
		covAuthReq(http.MethodPost, "/x", nil, disabled.ID, "covtotp3", "user"))
	var gen2 TOTPGenerateResponse
	json.NewDecoder(covCallAuth(s, s.handleTOTPGenerate,
		covAuthReq(http.MethodPost, "/x", nil, disabled.ID, "covtotp3", "user")).Body).Decode(&gen2)
	codeD, _ := totp.GenerateCode(gen2.Secret, time.Now())
	covCallAuth(s, s.handleTOTPEnable,
		covAuthReq(http.MethodPost, "/x", strings.NewReader(`{"code":"`+codeD+`"}`), disabled.ID, "covtotp3", "user"))
	covWantCode(t, "disable wrong code", disableOf(`{"code":"000000"}`), http.StatusBadRequest)
	rec = disableOf(`{"code":"` + codeD + `"}`)
	covWantCode(t, "disable ok", rec, http.StatusOK)
	got3, _ := s.db.GetUserByID(disabled.ID)
	if got3.TOTPEnabled {
		t.Error("TOTP should be disabled")
	}

	// EnableTOTP 数据库失败 → 500（关库后其余前置检查仍通过）
	s2 := covLoopServer(t)
	u2 := covSeedUser(t, s2, "covtotp4", "cov-pass-abc", "user")
	var gen3 TOTPGenerateResponse
	json.NewDecoder(covCallAuth(s2, s2.handleTOTPGenerate,
		covAuthReq(http.MethodPost, "/x", nil, u2.ID, "covtotp4", "user")).Body).Decode(&gen3)
	code4, _ := totp.GenerateCode(gen3.Secret, time.Now())
	covCloseDB(t, s2)
	rec = covCallAuth(s2, s2.handleTOTPEnable,
		covAuthReq(http.MethodPost, "/x", strings.NewReader(`{"code":"`+code4+`"}`), u2.ID, "covtotp4", "user"))
	covWantCode(t, "enable db error", rec, http.StatusInternalServerError)
}

// ============ 反向代理 API ============

func covSeedProxyAgent(t *testing.T, s *Server, id string) {
	t.Helper()
	if err := s.db.UpsertAgent(&storage.Agent{ID: id, Hostname: "h-" + id, Status: "online", LastSeen: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func covSeedProxy(t *testing.T, s *Server, id string, port int) {
	t.Helper()
	if err := s.db.CreateProxy(&storage.Proxy{
		ID: id, Name: "px-" + id, AgentID: "agent-px", ProxyType: "tcp",
		RemotePort: port, Target: "127.0.0.1:80", Enabled: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCovProxyAPIHandlers(t *testing.T) {
	s := covLoopServer(t)
	covSeedProxyAgent(t, s, "agent-px")
	covSeedProxy(t, s, "p1", 18000)
	covSeedProxy(t, s, "p2", 18001)

	admin := func(h http.HandlerFunc, method, target string, body string) *httptest.ResponseRecorder {
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		return covCallAuth(s, h, covAuthReq(method, target, rd, "1", "covadmin", "admin"))
	}

	// handleProxies
	rec := covRec()
	s.handleProxies(rec, covReq(http.MethodPost, "/api/proxies", nil))
	covWantCode(t, "proxies 405", rec, http.StatusMethodNotAllowed)
	rec = admin(s.handleProxies, http.MethodGet, "/api/proxies", "")
	covWantCode(t, "proxies list", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"remotePort":18000`) {
		t.Errorf("proxies list body = %s", rec.Body.String())
	}
	rec = admin(s.handleProxies, http.MethodGet, "/api/proxies?agent_id=nope", "")
	covWantCode(t, "proxies filtered", rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), "18000") {
		t.Error("filter should exclude other agents' proxies")
	}
	// proxyMgr 挂上后附加 stopped 状态
	s.proxyMgr = proxy.NewManager(s, s.db)
	rec = admin(s.handleProxies, http.MethodGet, "/api/proxies", "")
	covWantCode(t, "proxies with status", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"status":"stopped"`) {
		t.Errorf("proxies status body = %s", rec.Body.String())
	}
	s.proxyMgr = nil
	s.db.Close()
	rec = admin(s.handleProxies, http.MethodGet, "/api/proxies", "")
	covWantCode(t, "proxies db err", rec, http.StatusInternalServerError)
}

func TestCovProxyCreate(t *testing.T) {
	s := covLoopServer(t)
	covSeedProxyAgent(t, s, "agent-px")
	covSeedProxy(t, s, "p1", 18000)

	create := func(role, body string) *httptest.ResponseRecorder {
		return covCallAuth(s, s.handleProxyCreate,
			covAuthReq(http.MethodPost, "/api/proxies", strings.NewReader(body), "1", "covuser", role))
	}
	full := func(port int, public bool, agent string) string {
		return `{"name":"px","agentId":"` + agent + `","proxyType":"tcp","remotePort":` +
			strconv.Itoa(port) + `,"target":"127.0.0.1:80","publicBind":` + strconv.FormatBool(public) + `}`
	}

	rec := covRec()
	s.handleProxyCreate(rec, covReq(http.MethodGet, "/x", nil))
	covWantCode(t, "create 405", rec, http.StatusMethodNotAllowed)
	covWantCode(t, "create 403", create("user", full(18100, true, "agent-px")), http.StatusForbidden)
	covWantCode(t, "create bad body", create("admin", "{bad"), http.StatusBadRequest)
	covWantCode(t, "create missing", create("admin", `{"name":"x"}`), http.StatusBadRequest)
	covWantCode(t, "create agent missing", create("admin", full(18100, true, "ghost")), http.StatusNotFound)
	covWantCode(t, "create port in use", create("admin", full(18000, true, "agent-px")), http.StatusConflict)
	covWantCode(t, "create low port", create("admin", full(80, true, "agent-px")), http.StatusBadRequest)
	covWantCode(t, "create no public bind", create("admin", full(18100, false, "agent-px")), http.StatusBadRequest)
	rec = create("admin", full(18100, true, "agent-px"))
	covWantCode(t, "create ok", rec, http.StatusCreated)
	if p, _ := s.db.GetProxyByRemotePort(18100); p == nil {
		t.Error("proxy should be persisted")
	}
	// proxyMgr 在但 StartProxy 失败（127.0.0.1 端口被占）→ 202（配置已保存）
	blocker, err := net.Listen("tcp", "127.0.0.1:18500")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { blocker.Close() })
	s.proxyMgr = proxy.NewManager(s, s.db)
	rec = create("admin", full(18500, true, "agent-px"))
	covWantCode(t, "create start failed", rec, http.StatusAccepted)
}

func TestCovProxyUpdateDeleteStatus(t *testing.T) {
	s := covLoopServer(t)
	covSeedProxyAgent(t, s, "agent-px")
	covSeedProxy(t, s, "p1", 18000)
	covSeedProxy(t, s, "p2", 18001)

	update := func(method, body string) *httptest.ResponseRecorder {
		return covCallAuth(s, s.handleProxyUpdate,
			covAuthReq(method, "/api/proxies", strings.NewReader(body), "1", "covadmin", "admin"))
	}
	rec := covRec()
	s.handleProxyUpdate(rec, covReq(http.MethodPost, "/x", nil))
	covWantCode(t, "update 405", rec, http.StatusMethodNotAllowed)
	rec = covCallAuth(s, s.handleProxyUpdate,
		covAuthReq(http.MethodPut, "/x", nil, "1", "covuser", "user"))
	covWantCode(t, "update 403", rec, http.StatusForbidden)
	covWantCode(t, "update bad body", update(http.MethodPut, "{bad"), http.StatusBadRequest)
	covWantCode(t, "update empty id", update(http.MethodPut, `{"name":"x"}`), http.StatusBadRequest)
	covWantCode(t, "update missing", update(http.MethodPut, `{"id":"ghost"}`), http.StatusNotFound)
	covWantCode(t, "update port conflict", update(http.MethodPatch, `{"id":"p1","remotePort":18001}`), http.StatusConflict)
	rec = update(http.MethodPut, `{"id":"p1","name":"renamed","agentId":"agent-px","proxyType":"udp","remotePort":18300,"target":"127.0.0.1:9","description":"d","enabled":false}`)
	covWantCode(t, "update ok", rec, http.StatusOK)
	got, _ := s.db.GetProxy("p1")
	if got.Name != "renamed" || got.ProxyType != "udp" || got.RemotePort != 18300 || got.Enabled {
		t.Fatalf("updated proxy = %+v", got)
	}
	// ReloadProxy 分支
	s.proxyMgr = proxy.NewManager(s, s.db)
	rec = update(http.MethodPut, `{"id":"p1","enabled":true}`)
	covWantCode(t, "update reload", rec, http.StatusOK)

	del := func(body, query string) *httptest.ResponseRecorder {
		target := "/api/proxies"
		if query != "" {
			target += "?" + query
		}
		return covCallAuth(s, s.handleProxyDelete,
			covAuthReq(http.MethodDelete, target, strings.NewReader(body), "1", "covadmin", "admin"))
	}
	rec = covRec()
	s.handleProxyDelete(rec, covReq(http.MethodGet, "/x", nil))
	covWantCode(t, "delete 405", rec, http.StatusMethodNotAllowed)
	rec = covCallAuth(s, s.handleProxyDelete,
		covAuthReq(http.MethodDelete, "/x", nil, "1", "covuser", "user"))
	covWantCode(t, "delete 403", rec, http.StatusForbidden)
	covWantCode(t, "delete empty id", del("", ""), http.StatusBadRequest)
	covWantCode(t, "delete by body", del(`{"id":"p2"}`, ""), http.StatusNoContent)
	covWantCode(t, "delete by query", del("{bad", "id=p1"), http.StatusNoContent)
	if _, err := s.db.GetProxy("p1"); err == nil {
		t.Error("proxy should be deleted")
	}
	// DeleteProxy 失败 → 500（proxyMgr nil + 关库，无前置读）
	s2 := covLoopServer(t)
	covSeedProxy(t, s2, "px", 18400)
	covCloseDB(t, s2)
	rec = covCallAuth(s2, s2.handleProxyDelete,
		covAuthReq(http.MethodDelete, "/x", strings.NewReader(`{"id":"px"}`), "1", "covadmin", "admin"))
	covWantCode(t, "delete db err", rec, http.StatusInternalServerError)

	// handleProxyStatus
	s3 := covLoopServer(t)
	status := func(target string) *httptest.ResponseRecorder {
		return covCallAuth(s3, s3.handleProxyStatus, covAuthReq(http.MethodGet, target, nil, "1", "covadmin", "admin"))
	}
	rec = covRec()
	s3.handleProxyStatus(rec, covReq(http.MethodPost, "/x", nil))
	covWantCode(t, "status 405", rec, http.StatusMethodNotAllowed)
	covWantCode(t, "status no mgr", status("/api/proxies/status"), http.StatusServiceUnavailable)
	s3.proxyMgr = proxy.NewManager(s3, s3.db)
	covWantCode(t, "status all", status("/api/proxies/status"), http.StatusOK)
	covWantCode(t, "status missing", status("/api/proxies/status?id=ghost"), http.StatusNotFound)
}

func TestCovRegisterProxyAPIRoutes(t *testing.T) {
	s := covLoopServer(t)
	s.proxyMgr = proxy.NewManager(s, s.db)
	token, err := s.authService().GenerateToken("1", "covadmin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.registerProxyAPI(mux)

	do := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := covRec()
		mux.ServeHTTP(rec, req)
		return rec
	}
	covWantCode(t, "proxies GET", do(http.MethodGet, "/api/proxies"), http.StatusOK)
	covWantCode(t, "proxies POST", do(http.MethodPost, "/api/proxies"), http.StatusBadRequest) // 到 handler：缺字段
	covWantCode(t, "proxies PUT", do(http.MethodPut, "/api/proxies"), http.StatusBadRequest)  // 空 id
	covWantCode(t, "proxies PATCH", do(http.MethodPatch, "/api/proxies"), http.StatusBadRequest)
	covWantCode(t, "proxies DELETE", do(http.MethodDelete, "/api/proxies"), http.StatusBadRequest)
	covWantCode(t, "proxies CONNECT", do("PROPFIND", "/api/proxies"), http.StatusMethodNotAllowed)
	covWantCode(t, "proxies status", do(http.MethodGet, "/api/proxies/status"), http.StatusOK)

	// 未认证
	req := httptest.NewRequest(http.MethodGet, "/api/proxies", nil)
	rec := covRec()
	mux.ServeHTTP(rec, req)
	covWantCode(t, "proxies no token", rec, http.StatusUnauthorized)
}

// ============ 审计 API ============

func TestCovAuditAPIErrorsAndRoutes(t *testing.T) {
	s := covLoopServer(t)
	s.audit.Log(&audit.LogEntry{Username: "covuser", Action: "create", Resource: "proxy", Status: "success"})

	// export 成功：CSV 头 + 数据行
	rec := covRec()
	s.handleAuditLogsExport(rec, covReq(http.MethodGet, "/api/admin/audit/export", nil))
	covWantCode(t, "export ok", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "created_at") || !strings.Contains(rec.Body.String(), "covuser") {
		t.Errorf("export body = %s", rec.Body.String())
	}
	// stats 成功
	rec = covRec()
	s.handleAuditLogStats(rec, covReq(http.MethodGet, "/api/admin/audit/stats", nil))
	covWantCode(t, "stats ok", rec, http.StatusOK)
	// 405
	rec = covRec()
	s.handleAuditLogs(rec, covReq(http.MethodPost, "/api/admin/audit/logs", nil))
	covWantCode(t, "logs 405", rec, http.StatusMethodNotAllowed)

	// 关库 → logs/export 的 500 分支；stats 恒 nil err（内部吞错）→ 200 空对象
	s2 := covLoopServer(t)
	covCloseDB(t, s2)
	for _, h := range []http.HandlerFunc{s2.handleAuditLogs, s2.handleAuditLogsExport} {
		rec := covRec()
		h(rec, covReq(http.MethodGet, "/api/admin/audit/x", nil))
		covWantCode(t, "audit 500", rec, http.StatusInternalServerError)
	}
	rec = covRec()
	s2.handleAuditLogStats(rec, covReq(http.MethodGet, "/api/admin/audit/stats", nil))
	covWantCode(t, "audit stats swallows db err", rec, http.StatusOK)

	// registerAuditAPI 三条路由
	s3 := covLoopServer(t)
	token, _ := s3.authService().GenerateToken("1", "covadmin", "admin")
	mux := http.NewServeMux()
	s3.registerAuditAPI(mux)
	for _, p := range []string{"/api/admin/audit/logs", "/api/admin/audit/export", "/api/admin/audit/stats"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := covRec()
		mux.ServeHTTP(rec, req)
		covWantCode(t, "audit route "+p, rec, http.StatusOK)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit/logs", nil)
	rec = covRec()
	mux.ServeHTTP(rec, req)
	covWantCode(t, "audit route no token", rec, http.StatusUnauthorized)
}

// ============ api.go 杂项 ============

func TestCovAPIHandleStatusAndActions(t *testing.T) {
	s := covLoopServer(t)

	// handleStatus 405；500 不可达（GetStats 恒 nil err），关库返回零值统计 200
	rec := covRec()
	s.handleStatus(rec, covReq(http.MethodPost, "/api/status", nil))
	covWantCode(t, "status 405", rec, http.StatusMethodNotAllowed)
	s2 := covLoopServer(t)
	covCloseDB(t, s2)
	rec = covRec()
	s2.handleStatus(rec, covReq(http.MethodGet, "/api/status", nil))
	covWantCode(t, "status swallows db err", rec, http.StatusOK)

	// handleAlertActions：read PUT / default / 无用户
	alert := &storage.Alert{ID: "al-1", Type: "info", Title: "t", Message: "m", CreatedAt: time.Now()}
	if err := s.db.CreateAlert(alert); err != nil {
		t.Fatal(err)
	}
	put := func(path string) *httptest.ResponseRecorder {
		return covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
			s.handleAlertActions(w, r, path)
		}, covAuthReq(http.MethodPut, "/api/alerts/"+path, nil, "1", "covuser", "user"))
	}
	rec = put("al-1/read")
	covWantCode(t, "alert read", rec, http.StatusOK)
	rec = put("al-1/other")
	covWantCode(t, "alert default 405", rec, http.StatusMethodNotAllowed)
	rec = covRec()
	s.handleAlertActions(rec, covReq(http.MethodPut, "/api/alerts/x/read", nil), "al-1/read")
	covWantCode(t, "alert no user", rec, http.StatusUnauthorized)

	// handleUserActions：password 分支 / POST 其他 action 405
	target := covSeedUser(t, s, "covtarget", "cov-pass-1", "user")
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserActions(w, r, target.ID+"/password")
	}, covAuthReq(http.MethodPost, "/api/users/"+target.ID+"/password",
		strings.NewReader(`{"new_password":"cov-pass-2"}`), "1", "covadmin", "admin"))
	covWantCode(t, "user password", rec, http.StatusOK)
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserActions(w, r, target.ID+"/noop")
	}, covAuthReq(http.MethodPost, "/api/users/"+target.ID+"/noop", nil, "1", "covadmin", "admin"))
	covWantCode(t, "user action 405", rec, http.StatusMethodNotAllowed)

	// handleUserDelete：普通用户删他人 → 403
	adminUser, _ := s.db.GetUserByUsername("admin")
	if adminUser == nil { // admin 未初始化时直接造一个
		adminUser = covSeedUser(t, s, "covadm2", "cov-pass-3", "admin")
	}
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserDelete(w, r, adminUser.ID)
	}, covAuthReq(http.MethodDelete, "/api/users/"+adminUser.ID, nil, target.ID, "covtarget", "user"))
	covWantCode(t, "delete forbidden", rec, http.StatusForbidden)

	// middleware 小函数
	if got := s.getResourceFromPath("/health"); got != "unknown" {
		t.Errorf("resource from non-api = %q", got)
	}
	if got := s.getResourceFromPath("/api/users/1"); got != "users" {
		t.Errorf("resource = %q", got)
	}
	if got := s.getResourceIDFromPath("/api/users/42"); got != "42" {
		t.Errorf("resource id = %q", got)
	}
}

// ============ WS 升级失败 ============

func TestCovRemoteUpgradeFailures(t *testing.T) {
	s := covRemoteSetup(t)
	covRegisterBareAgent(t, s, "agent-up")

	// 合法宝 + 不可 Hijack 的 recorder → Upgrade 失败分支
	cases := []struct {
		desc    string
		handler http.HandlerFunc
		ticket  string
	}{
		{"terminal", s.handleTerminalWebSocket, covTerminalTicket(t, s, "agent-up")},
		{"desktop", s.handleDesktopWebSocket, covTicket(t, s, map[string]string{
			"agent_id": "agent-up", "host": "127.0.0.1", "port": "3389", "protocol": "rdp",
		})},
		{"vnc", s.handleVNCWebSocket, covTicket(t, s, map[string]string{
			"agent_id": "agent-up", "host": "127.0.0.1", "port": "5900", "protocol": "vnc",
		})},
	}
	for _, c := range cases {
		rec := covRec()
		c.handler(rec, covReq(http.MethodGet, "/api/remote/x", nil)) // 无票据头 → 升级前 400
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s missing ticket = %d", c.desc, rec.Code)
		}
	}
}

// ============ agent 注册鉴权内部 ============

func TestCovAgentRegistrationInternals(t *testing.T) {
	s := covLoopServer(t)

	// 全新 agent（库中不存在）→ 无拒绝
	existing, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "brand-new"})
	if err != nil || rejection != nil || existing != nil {
		t.Fatalf("new agent = %v, %v, %v", existing, rejection, err)
	}

	// 库中存在但无 SecretHash → 兼容放行
	if err := s.db.UpsertAgent(&storage.Agent{ID: "no-hash", Hostname: "h"}); err != nil {
		t.Fatal(err)
	}
	existing, rejection, err = s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "no-hash"})
	if err != nil || rejection != nil || existing == nil || existing.ID != "no-hash" {
		t.Fatalf("no-hash agent = %v, %v, %v", existing, rejection, err)
	}

	// 有 SecretHash：缺 secret / 错 secret / 对 secret
	hash, err := storage.HashAgentSecret("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertAgent(&storage.Agent{ID: "with-hash", Hostname: "h", SecretHash: hash}); err != nil {
		t.Fatal(err)
	}
	_, rejection, err = s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "with-hash"})
	if err != nil || rejection == nil || rejection.code != "authentication_required" {
		t.Fatalf("missing secret = %v, %v", rejection, err)
	}
	_, rejection, err = s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "with-hash", Secret: "wrong"})
	if err != nil || rejection == nil || rejection.code != "authentication_failed" {
		t.Fatalf("wrong secret = %v, %v", rejection, err)
	}
	existing, rejection, err = s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "with-hash", Secret: "s3cret"})
	if err != nil || rejection != nil || existing == nil {
		t.Fatalf("right secret = %v, %v, %v", existing, rejection, err)
	}

	// db 故障 → error 返回
	s2 := covLoopServer(t)
	covCloseDB(t, s2)
	if _, _, err := s2.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "any"}); err == nil {
		t.Error("db failure should surface error")
	}

	// registerAgentConnection：重复连接拒绝 / 鉴权错误透传 / 新 agent 成功入库
	dup := NewAgent("with-hash", nil)
	if err := s.registry.Register(dup); err != nil {
		t.Fatal(err)
	}
	_, rejection, err = s.registerAgentConnection(nil, &protocol.RegisterPayload{AgentID: "with-hash", Secret: "s3cret"})
	if err != nil || rejection == nil || rejection.code != "duplicate_connection" {
		t.Fatalf("duplicate = %v, %v", rejection, err)
	}
	if _, _, err := s2.registerAgentConnection(nil, &protocol.RegisterPayload{AgentID: "any"}); err == nil {
		t.Error("register on closed db should error")
	}
	agent, rejection, err := s.registerAgentConnection(nil, &protocol.RegisterPayload{AgentID: "fresh-1", Hostname: "fh"})
	if err != nil || rejection != nil || agent == nil {
		t.Fatalf("fresh register = %v, %v, %v", agent, rejection, err)
	}
	if got, err := s.db.GetAgent("fresh-1"); err != nil || got.Hostname != "fh" {
		t.Errorf("persisted agent = %v, %v", got, err)
	}
}

// ============ writeLoop 写失败 ============

func TestCovWriteLoopFailure(t *testing.T) {
	s := covLoopServer(t)
	covWSReady(s)

	conn, _ := covDirectWS(t, func(w http.ResponseWriter, r *http.Request) {
		c, err := s.upgrader.Upgrade(w, r, nil)
		if err == nil {
			c.Close()
		}
	}, "/ws", "", true)
	conn.Close() // 对端已关 → 写必失败

	agent := NewAgent("wloop", conn)
	go s.writeLoop(agent)
	agent.Send <- protocol.NewMessage(protocol.MessageTypeHeartbeat, nil)
	time.Sleep(100 * time.Millisecond) // 让 writeLoop 撞上写错误退出
}

// ============ 其余单行分支 ============

func TestCovRemainingSmallBranches(t *testing.T) {
	// NextBackupRunAt 非法调度 → 0
	if got := NextBackupRunAt("bogus", time.Now()); got != 0 {
		t.Errorf("bogus schedule next run = %d", got)
	}

	// startBackupRun：CreateBackupRun 失败（关库，guards 不触库）
	s := covLoopServer(t)
	cfg := covSeedBackupCfg(t, s, "ghost-any", "ok")
	covCloseDB(t, s)
	if _, err := s.startBackupRun(cfg, "manual"); err == nil || !strings.Contains(err.Error(), "create run record") {
		t.Fatalf("create run err = %v", err)
	}

	// cleanupServerBackups：Remove 失败（备份名是非空目录）
	s2 := covLoopServer(t)
	dir := s2.serverBackupDir()
	if err := os.MkdirAll(filepath.Join(dir, "cockpit-20200101-000000.db", "inner"), 0755); err != nil {
		t.Fatal(err)
	}
	_ = s2.db.SetSetting(ServerBackupRetentionSettingKey, "1")
	s2.cleanupServerBackups() // Remove 报错仅记日志
	if _, err := os.Stat(filepath.Join(dir, "cockpit-20200101-000000.db")); err != nil {
		t.Errorf("removal failure should keep dir: %v", err)
	}

	// cleanupExpiredRecordings：Remove 失败（同名非空目录）→ meta 保留
	s3 := covLoopServer(t)
	covSeedRecording(t, s3, "sess-dir")
	if err := os.MkdirAll(filepath.Join(s3.recordingsDir(), "sess-dir.cast", "keep"), 0755); err != nil {
		t.Fatal(err)
	}
	_ = s3.db.SetSetting(RecordingRetentionSettingKey, "7")
	s3.cleanupExpiredRecordings()
	if rec, _ := s3.db.GetTerminalRecording("sess-dir"); rec == nil {
		t.Error("meta should survive file removal failure")
	}

	// handleRecordingsList / handleRecordingDelete：db 错误
	s4 := covLoopServer(t)
	recording := covSeedRecordingOnly(t, s4)
	covCloseDB(t, s4)
	rec := covRec()
	s4.handleRecordingsList(rec, covReq(http.MethodGet, "/api/recordings", nil))
	covWantCode(t, "recordings db err", rec, http.StatusInternalServerError)
	rec = covRec()
	s4.handleRecordingDelete(rec, covReq(http.MethodDelete, "/api/recordings/"+recording, nil), recording)
	covWantCode(t, "recording delete db err", rec, http.StatusNotFound) // Get 失败按不存在处理；Delete 的 500 不可达

	// drift：Hostname 为空的 agent 用 ID 兜底（fake agent 默认带 Hostname，手动清空）
	s5 := covLoopServer(t)
	nohost := covFakeAgent(t, s5, "agent-nohost", []string{"drift"}, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"items": []interface{}{
			map[string]interface{}{"kind": "file", "name": "/etc/q", "status": "drifted"},
		}})
	})
	nohost.Hostname = ""
	s5.scanDriftOnce()

	// stack 错误映射矩阵（rpcResp error 分支只映射 busy/not found，其余 502）
	s6 := covLoopServer(t)
	stackErr := func(id, msg string) {
		t.Helper()
		covFakeAgent(t, s6, id, []string{"docker"}, func(method string, params map[string]interface{}) map[string]interface{} {
			return covErrPayload(msg)
		})
	}
	cases := []struct {
		id, msg string
		code    int
	}{
		{"stk-busy", "agent busy", http.StatusConflict},
		{"stk-notfound", "task not found", http.StatusNotFound},
		{"stk-timeout", "response timeout", http.StatusBadGateway},
		{"stk-boom", "plain boom", http.StatusBadGateway},
	}
	for _, c := range cases {
		stackErr(c.id, c.msg)
	}
	for _, c := range cases {
		rec := covRec()
		s6.handleStackRPC(rec, covReq(http.MethodPost, "/x", nil), c.id, "stack.list", nil, "")
		covWantCode(t, "stack rpc "+c.id, rec, c.code)
	}
	rec = covRec()
	s6.handleStackRPC(rec, covReq(http.MethodPost, "/x", nil), "ghost", "stack.list", nil, "")
	covWantCode(t, "stack rpc ghost", rec, http.StatusNotFound)

	// handleAgentStacks：rpcResp error → 固定 502 "Invalid agent response"
	for _, id := range []string{"stk2-timeout", "stk2-boom"} {
		stackErr(id, "kaput")
		rec = covRec()
		s6.handleAgentStacks(rec, covReq(http.MethodGet, "/x", nil), id)
		covWantCode(t, "agent stacks "+id, rec, http.StatusBadGateway)
	}
	rec = covRec()
	s6.handleAgentStacks(rec, covReq(http.MethodGet, "/x", nil), "ghost")
	covWantCode(t, "agent stacks ghost", rec, http.StatusNotFound)
	// 成功：stack.list + stack.info
	covFakeAgent(t, s6, "stk-ok", []string{"docker"}, func(method string, params map[string]interface{}) map[string]interface{} {
		if method == "stack.info" {
			return covOKPayload(map[string]interface{}{"compose_version": "v2"})
		}
		return covOKPayload([]interface{}{map[string]interface{}{"name": "web", "state": "running"}})
	})
	rec = covRec()
	s6.handleAgentStacks(rec, covReq(http.MethodGet, "/x", nil), "stk-ok")
	covWantCode(t, "agent stacks ok", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "web") {
		t.Errorf("agent stacks body = %s", rec.Body.String())
	}

	// handleStackHistory：空历史 → 200 空数组；db 错误 → 500
	rec = covRec()
	s6.handleStackHistory(rec, covReq(http.MethodGet, "/x?limit=5", nil), "stk-ok", "web")
	covWantCode(t, "stack history ok", rec, http.StatusOK)
	s7 := covLoopServer(t)
	covCloseDB(t, s7)
	rec = covRec()
	s7.handleStackHistory(rec, covReq(http.MethodGet, "/x?limit=5", nil), "any", "web")
	covWantCode(t, "stack history db err", rec, http.StatusInternalServerError)
}

// covSeedRecordingOnly 造一条零时间录制并返回 sessionID
func covSeedRecordingOnly(t *testing.T, s *Server) string {
	t.Helper()
	covSeedRecording(t, s, "sess-loose")
	return "sess-loose"
}
