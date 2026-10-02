package server

// cov_vault_gaps_test.go 补 api_vault.go 的错误/边界分支：verify 的 405/
// 坏 JSON/未认证/TOTP 分支、列表与保存删除的 DB 错误路径（closed DB）、
// 405/空 id、audit 为 nil 的静默分支，以及 vaultIssueToken 的 randRead
// 兜底与过期条目懒清理（randRead 注入点，见 ticket.go 注释）。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVaultVerifyMethodAndBodyEdges(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)

	// GET → 405
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodGet, "/api/remote/vault/verify", nil))
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultVerify)(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET verify = %d, want 405", rec.Code)
	}

	// 坏 JSON → 400
	req = authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPost, "/api/remote/vault/verify", strings.NewReader("{not-json")))
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultVerify)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json verify = %d, want 400", rec.Code)
	}

	// 未经 auth 中间件（无用户上下文）→ 401
	rec = httptest.NewRecorder()
	s.handleVaultVerify(rec, httptest.NewRequest(http.MethodPost, "/api/remote/vault/verify", strings.NewReader(`{"password":"x"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated verify = %d, want 401", rec.Code)
	}
}

// TestVaultVerifyTOTPCodeBranch 覆盖 totp_code 分支（admin 未开 TOTP →
// 校验失败 401，但分支体已执行）
func TestVaultVerifyTOTPCodeBranch(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)

	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPost, "/api/remote/vault/verify", strings.NewReader(`{"totp_code":"123456"}`)))
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultVerify)(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("totp verify (no TOTP enrolled) = %d, want 401", rec.Code)
	}
}

func TestVaultCredentialsEdgeBranches(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)

	// 未经 auth 中间件 → 401
	rec := httptest.NewRecorder()
	s.handleVaultCredentials(rec, httptest.NewRequest(http.MethodGet, "/api/remote/vault/credentials", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated list = %d, want 401", rec.Code)
	}

	// 未实现的方法（POST）→ 405
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPost, "/api/remote/vault/credentials", nil))
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultCredentials)(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST credentials = %d, want 405", rec.Code)
	}

	// 带 token 的坏 JSON upsert → 400
	token := vaultVerifyLogin(t, s)
	req = authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPut, "/api/remote/vault/credentials", strings.NewReader("{not-json")))
	req.Header.Set("X-Vault-Token", token)
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultCredentials)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json upsert = %d, want 400", rec.Code)
	}
}

// TestVaultCredentialsDBErrors closed DB 触发列表/保存/删除的错误路径
// （请求与 token 先于关库构造——authenticateAdminRequest 要读写 DB）
func TestVaultCredentialsDBErrors(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)
	token := vaultVerifyLogin(t, s)

	getReq := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodGet, "/api/remote/vault/credentials", nil))
	putReq := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPut, "/api/remote/vault/credentials",
		strings.NewReader(`{"agent_id":"a","host":"h","port":22,"protocol":"ssh","password":"p"}`)))
	putReq.Header.Set("X-Vault-Token", token)
	delReq := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodDelete, "/api/remote/vault/credentials/some-id", nil))
	delReq.Header.Set("X-Vault-Token", token)

	covCloseDB(t, s)

	// GET 列表 → 500
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultCredentials)(rec, getReq)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("GET list on closed DB = %d, want 500", rec.Code)
	}

	// PUT 保存 → 500
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultCredentials)(rec, putReq)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("PUT on closed DB = %d, want 500", rec.Code)
	}

	// DELETE → 500
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultCredentialDelete)(rec, delReq)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("DELETE on closed DB = %d, want 500", rec.Code)
	}
}

func TestVaultCredentialDeleteEdgeBranches(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)
	token := vaultVerifyLogin(t, s)

	// GET → 405
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodGet, "/api/remote/vault/credentials/x", nil))
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultCredentialDelete)(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET delete endpoint = %d, want 405", rec.Code)
	}

	// 未经 auth 中间件 → 401
	rec = httptest.NewRecorder()
	s.handleVaultCredentialDelete(rec, httptest.NewRequest(http.MethodDelete, "/api/remote/vault/credentials/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated delete = %d, want 401", rec.Code)
	}

	// 路径尾为空（/credentials/）→ 400
	req = authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodDelete, "/api/remote/vault/credentials/", nil))
	req.Header.Set("X-Vault-Token", token)
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultCredentialDelete)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty id delete = %d, want 400", rec.Code)
	}
}

// TestVaultIssueTokenRandFallbackAndExpiryCleanup 覆盖 randRead 兜底
// （时间熵填充）与签发时对过期条目的懒清理
func TestVaultIssueTokenRandFallbackAndExpiryCleanup(t *testing.T) {
	resetVaultTokens(t)

	// 预置一条已过期条目：签发时必须被清掉
	vaultTokensMu.Lock()
	vaultTokens["stale-token"] = &vaultTokenEntry{userID: "u-old", expiresAt: time.Now().Add(-time.Minute)}
	vaultTokensMu.Unlock()

	orig := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("boom rand") }
	t.Cleanup(func() { randRead = orig })

	token := vaultIssueToken("u-new")
	if len(token) != 32 { // 16 字节 hex
		t.Errorf("fallback token len = %d, want 32 hex chars", len(token))
	}

	vaultTokensMu.Lock()
	_, staleExists := vaultTokens["stale-token"]
	vaultTokensMu.Unlock()
	if staleExists {
		t.Error("expired token not cleaned up on issue")
	}
}

// TestVaultAuditActionNilAudit 覆盖 audit 为 nil 的静默分支（零值 Server）
func TestVaultAuditActionNilAudit(t *testing.T) {
	s := &Server{}
	s.auditVaultAction("vault.verify", "u", "n", "failure", "ip", "ua", "", nil)
}
