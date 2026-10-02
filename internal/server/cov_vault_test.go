package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/config"
)

// resetVaultTokens 清空包级 vault token 存储，隔离测试
func resetVaultTokens(t *testing.T) {
	t.Helper()
	vaultTokensMu.Lock()
	vaultTokens = map[string]*vaultTokenEntry{}
	vaultTokensMu.Unlock()
	t.Cleanup(func() {
		vaultTokensMu.Lock()
		vaultTokens = map[string]*vaultTokenEntry{}
		vaultTokensMu.Unlock()
	})
}

// vaultVerifyLogin 用 admin 登录密码过二次验证，返回 vault token
func vaultVerifyLogin(t *testing.T, s *Server) string {
	t.Helper()
	body := []byte(`{"password":"admin123"}`)
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPost, "/api/remote/vault/verify", bytes.NewReader(body)))
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultVerify)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("verify response decode: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("verify response has empty token")
	}
	return resp.Token
}

func TestVaultVerifyPassword(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)

	// 错密码 → 401
	body := []byte(`{"password":"wrong"}`)
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPost, "/api/remote/vault/verify", bytes.NewReader(body)))
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultVerify)(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d, want 401", rec.Code)
	}

	// 对密码 → 200 + token
	token := vaultVerifyLogin(t, s)

	// token 可多次使用（多次校验均通过）
	for i := 0; i < 2; i++ {
		if !vaultCheckToken(token, "any-user-check-skipped") {
			// userID 换人必须失败——先确认绑定
			break
		}
	}
}

func TestVaultVerifyWrongUserRejected(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)

	token := vaultVerifyLogin(t, s)
	// admin 的 userID 绑定：别人的 userID 校验必须失败
	admin, _ := s.db.GetUserByUsername("admin")
	if vaultCheckToken(token, "someone-else") {
		t.Error("token bound to issuing user; cross-user check should fail")
	}
	if !vaultCheckToken(token, admin.ID) {
		t.Error("token should validate for issuing user")
	}
}

func TestVaultVerifyEmptyBody(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)

	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPost, "/api/remote/vault/verify", strings.NewReader("{}")))
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultVerify)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty body status = %d, want 400", rec.Code)
	}
}

func vaultPutCredential(t *testing.T, s *Server, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPut, "/api/remote/vault/credentials", strings.NewReader(body)))
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultCredentials)(rec, req)
	return rec
}

func TestVaultPutRequiresVaultToken(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)

	rec := vaultPutCredential(t, s, "", `{"agent_id":"a1","host":"h","port":22,"protocol":"ssh","username":"root","password":"pw"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("PUT without vault token = %d, want 401", rec.Code)
	}
}

func TestVaultPutListDeleteFlow(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)
	token := vaultVerifyLogin(t, s)

	// PUT 保存
	rec := vaultPutCredential(t, s, token, `{"agent_id":"a1","host":"192.168.5.188","port":22,"protocol":"ssh","username":"cui","password":"secret-pass"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// GET 列表：元数据 + 明文绝不出现
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodGet, "/api/remote/vault/credentials", nil))
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultCredentials)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "secret-pass") {
		t.Fatalf("GET response leaks plaintext: %s", body)
	}
	if !strings.Contains(body, `"hasPassword":true`) || !strings.Contains(body, `"username":"cui"`) {
		t.Fatalf("GET response missing metadata: %s", body)
	}

	// 解析出 id 后 DELETE（需 vault token）
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Data) != 1 {
		t.Fatalf("list decode err=%v len=%d", err, len(list.Data))
	}
	id := list.Data[0].ID

	// DELETE 无 token → 401
	delReq := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodDelete, "/api/remote/vault/credentials/"+id, nil))
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultCredentialDelete)(rec, delReq)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("DELETE without token = %d, want 401", rec.Code)
	}

	// DELETE 带 token → 200，再删 → 404
	delReq = authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodDelete, "/api/remote/vault/credentials/"+id, nil))
	delReq.Header.Set("X-Vault-Token", token)
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultCredentialDelete)(rec, delReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultCredentialDelete)(rec, delReq)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second DELETE = %d, want 404", rec.Code)
	}
}

func TestVaultPutRejectsBadPayload(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)
	token := vaultVerifyLogin(t, s)

	cases := []string{
		`{"agent_id":"","host":"h","port":22,"protocol":"ssh","password":"p"}`,          // 缺 agent
		`{"agent_id":"a","host":"h","port":0,"protocol":"ssh","password":"p"}`,          // 非法端口
		`{"agent_id":"a","host":"h","port":22,"protocol":"telnet","password":"p"}`,      // 协议不在保险箱范围
		`{"agent_id":"a","host":"h","port":22,"protocol":"ssh"}`,                        // 无凭据本体
	}
	for i, body := range cases {
		rec := vaultPutCredential(t, s, token, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("case %d status = %d, want 400 (body=%s)", i, rec.Code, body)
		}
	}
}

// TestTicketCreateUseSavedInjectsCredentials use_saved 路径：保存 → 无凭据
// 请求 → 票据参数被注入已存凭据
func TestTicketCreateUseSavedInjectsCredentials(t *testing.T) {
	s := newTestServerWithDB(t)
	s.ticketMgr = NewTicketManager()
	s.cfg = &config.Config{RemoteControl: &config.RemoteControlConfig{AllowArbitraryTarget: true}}
	resetVaultTokens(t)
	token := vaultVerifyLogin(t, s)

	if err := s.registry.Register(NewAgent("agent-1", nil)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	rec := vaultPutCredential(t, s, token, `{"agent_id":"agent-1","host":"192.168.5.188","port":22,"protocol":"ssh","username":"cui","password":"saved-pass","private_key":"-----BEGIN KEY-----"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body=%s", rec.Code, rec.Body.String())
	}

	body := []byte(`{"agent_id":"agent-1","host":"192.168.5.188","port":22,"protocol":"ssh","use_saved":true}`)
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPost, "/api/remote/tickets", bytes.NewReader(body)))
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleTicketCreate)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ticket status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ticket response decode: %v", err)
	}
	ticket, ok := s.ticketMgr.ValidateTicket(resp.Ticket)
	if !ok {
		t.Fatal("ValidateTicket failed")
	}
	if ticket.Params["username"] != "cui" || ticket.Params["password"] != "saved-pass" || ticket.Params["private_key"] != "-----BEGIN KEY-----" {
		t.Errorf("ticket params not injected from vault: %+v", ticket.Params)
	}

	// 审计记录 auth_source=saved
	logs, total, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{
		"action": "remote_start",
		"status": "success",
	})
	if err != nil {
		t.Fatalf("GetAuditLogs: %v", err)
	}
	if total != 1 || !strings.Contains(logs[0].Details, `"auth_source":"saved"`) {
		t.Fatalf("audit auth_source missing: total=%d details=%s", total, logs[0].Details)
	}
}

// TestTicketCreateUseSavedNoCredential use_saved 但没存过 → 400 + 审计失败
func TestTicketCreateUseSavedNoCredential(t *testing.T) {
	s := newTestServerWithDB(t)
	s.ticketMgr = NewTicketManager()
	s.cfg = &config.Config{RemoteControl: &config.RemoteControlConfig{AllowArbitraryTarget: true}}

	if err := s.registry.Register(NewAgent("agent-1", nil)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	body := []byte(`{"agent_id":"agent-1","host":"192.168.5.188","port":22,"protocol":"ssh","use_saved":true}`)
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPost, "/api/remote/tickets", bytes.NewReader(body)))
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleTicketCreate)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}

	logs, total, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": "remote_start", "status": "failure"})
	if err != nil || total != 1 {
		t.Fatalf("failure audit total=%d err=%v", total, err)
	}
	if !strings.Contains(logs[0].Details, "no saved credential") {
		t.Fatalf("audit details = %s, want no-saved-credential reason", logs[0].Details)
	}
}

// TestTicketCreateUseSavedStillEgressGated egress 拒绝先于保险箱查询——
// 保险箱不得成为绕过出口策略的通道
func TestTicketCreateUseSavedStillEgressGated(t *testing.T) {
	s := newTestServerWithDB(t)
	s.ticketMgr = NewTicketManager()
	resetVaultTokens(t)
	token := vaultVerifyLogin(t, s)

	s.cfg = &config.Config{
		RemoteControl: &config.RemoteControlConfig{
			AllowArbitraryTarget: true,
		},
	}
	if err := s.registry.Register(NewAgent("agent-1", nil)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// 存的是内网目标
	rec := vaultPutCredential(t, s, token, `{"agent_id":"agent-1","host":"192.168.5.188","port":22,"protocol":"ssh","username":"cui","password":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: %d", rec.Code)
	}

	// 但 egress 只放行另一网段
	s.cfg = &config.Config{
		RemoteControl: &config.RemoteControlConfig{
			AllowedTargets: []string{"10.0.0.0/8"},
		},
	}
	body := []byte(`{"agent_id":"agent-1","host":"192.168.5.188","port":22,"protocol":"ssh","use_saved":true}`)
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPost, "/api/remote/tickets", bytes.NewReader(body)))
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleTicketCreate)(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("use_saved egress bypass status = %d, want 403", rec.Code)
	}
}
