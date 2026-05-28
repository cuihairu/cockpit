package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cuihairu/cockpit/internal/storage"
)

// ctxUserID sets user_id in context for handlers that use r.Context().Value("user_id")
func withUserID(r *http.Request, userID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), "user_id", userID))
}

func TestHandleTOTPGenerateWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	_, req := doAuthenticatedRequest(s, "GET", "/api/auth/totp/generate", nil)
	rec := callWithAuth(s, s.handleTOTPGenerate, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleTOTPGenerateSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.CreateUser(&storage.User{Username: "admin", Role: "admin"})
	u, _ := s.db.GetUserByUsername("admin")

	_, req := doAuthenticatedRequest(s, "POST", "/api/auth/totp/generate", nil)
	req = withUserID(req, u.ID)
	rec := callWithAuth(s, s.handleTOTPGenerate, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var resp TOTPGenerateResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Secret == "" {
		t.Error("Secret should not be empty")
	}
	if resp.QRCode == "" {
		t.Error("QRCode should not be empty")
	}
	if len(resp.BackupCodes) == 0 {
		t.Error("BackupCodes should not be empty")
	}
}

func TestHandleTOTPGenerateUserNotFound(t *testing.T) {
	s := newTestServerWithDB(t)
	_, req := doAuthenticatedRequest(s, "POST", "/api/auth/totp/generate", nil)
	req = withUserID(req, "nonexistent-id")
	rec := callWithAuth(s, s.handleTOTPGenerate, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestHandleTOTPEnableWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	_, req := doAuthenticatedRequest(s, "GET", "/api/auth/totp/enable", nil)
	rec := callWithAuth(s, s.handleTOTPEnable, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleTOTPEnableNoSetup(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.CreateUser(&storage.User{Username: "admin", Role: "admin"})
	u, _ := s.db.GetUserByUsername("admin")

	body, _ := json.Marshal(TOTPEnableRequest{Code: "123456"})
	_, req := doAuthenticatedRequest(s, "POST", "/api/auth/totp/enable", body)
	req = withUserID(req, u.ID)
	rec := callWithAuth(s, s.handleTOTPEnable, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (no TOTP setup initiated)", rec.Code)
	}
}

func TestHandleTOTPVerifyWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	req := httptest.NewRequest("GET", "/api/auth/totp/verify", nil)
	rec := httptest.NewRecorder()
	s.handleTOTPVerify(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleTOTPVerifyInvalidBody(t *testing.T) {
	s := newTestServerWithDB(t)
	req := httptest.NewRequest("POST", "/api/auth/totp/verify", bytes.NewReader([]byte("bad")))
	rec := httptest.NewRecorder()
	s.handleTOTPVerify(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleTOTPVerifyInvalidTmpToken(t *testing.T) {
	s := newTestServerWithDB(t)
	body, _ := json.Marshal(TOTPVerifyRequest{Code: "123456", TmpToken: "bad-token"})
	req := httptest.NewRequest("POST", "/api/auth/totp/verify", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleTOTPVerify(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandleTOTPDisableWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	_, req := doAuthenticatedRequest(s, "GET", "/api/auth/totp/disable", nil)
	rec := callWithAuth(s, s.handleTOTPDisable, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleTOTPDisableNotEnabled(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.CreateUser(&storage.User{Username: "admin", Role: "admin", TOTPEnabled: false})
	u, _ := s.db.GetUserByUsername("admin")

	body, _ := json.Marshal(TOTPDisableRequest{Code: "123456"})
	_, req := doAuthenticatedRequest(s, "POST", "/api/auth/totp/disable", body)
	req = withUserID(req, u.ID)
	rec := callWithAuth(s, s.handleTOTPDisable, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleCurrentUserWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	_, req := doAuthenticatedRequest(s, "POST", "/api/auth/me", nil)
	rec := callWithAuth(s, s.handleCurrentUser, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleCurrentUserSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.CreateUser(&storage.User{Username: "testuser", Role: "admin", Email: "test@test.com"})
	u, _ := s.db.GetUserByUsername("testuser")

	_, req := doAuthenticatedRequest(s, "GET", "/api/auth/me", nil)
	req = withUserID(req, u.ID)
	rec := callWithAuth(s, s.handleCurrentUser, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["username"] != "testuser" {
		t.Errorf("username = %v, want testuser", resp["username"])
	}
}

func TestHandleCurrentUserNotFound(t *testing.T) {
	s := newTestServerWithDB(t)
	_, req := doAuthenticatedRequest(s, "GET", "/api/auth/me", nil)
	req = withUserID(req, "nonexistent-id")
	rec := callWithAuth(s, s.handleCurrentUser, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestHandleCurrentUserPasswordWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	_, req := doAuthenticatedRequest(s, "GET", "/api/auth/me/password", nil)
	rec := callWithAuth(s, s.handleCurrentUserPassword, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleCurrentUserPasswordSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	hashed, _ := storage.HashPassword("oldpass")
	s.db.CreateUser(&storage.User{Username: "testuser", Password: hashed, Role: "admin"})
	u, _ := s.db.GetUserByUsername("testuser")

	body, _ := json.Marshal(map[string]string{"currentPassword": "oldpass", "newPassword": "newpass123"})
	_, req := doAuthenticatedRequest(s, "PUT", "/api/auth/me/password", body)
	req = withUserID(req, u.ID)
	rec := callWithAuth(s, s.handleCurrentUserPassword, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCurrentUserPasswordWrongOldPassword(t *testing.T) {
	s := newTestServerWithDB(t)
	hashed, _ := storage.HashPassword("oldpass")
	s.db.CreateUser(&storage.User{Username: "testuser", Password: hashed, Role: "admin"})
	u, _ := s.db.GetUserByUsername("testuser")

	body, _ := json.Marshal(map[string]string{"currentPassword": "wrongpass", "newPassword": "newpass123"})
	_, req := doAuthenticatedRequest(s, "PUT", "/api/auth/me/password", body)
	req = withUserID(req, u.ID)
	rec := callWithAuth(s, s.handleCurrentUserPassword, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandleCurrentUserPasswordEmptyNew(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.CreateUser(&storage.User{Username: "testuser", Password: "h", Role: "admin"})
	u, _ := s.db.GetUserByUsername("testuser")

	body, _ := json.Marshal(map[string]string{"currentPassword": "old", "newPassword": ""})
	_, req := doAuthenticatedRequest(s, "PUT", "/api/auth/me/password", body)
	req = withUserID(req, u.ID)
	rec := callWithAuth(s, s.handleCurrentUserPassword, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleCurrentUserPasswordShortNew(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.CreateUser(&storage.User{Username: "testuser", Password: "h", Role: "admin"})
	u, _ := s.db.GetUserByUsername("testuser")

	body, _ := json.Marshal(map[string]string{"currentPassword": "old", "newPassword": "ab"})
	_, req := doAuthenticatedRequest(s, "PUT", "/api/auth/me/password", body)
	req = withUserID(req, u.ID)
	rec := callWithAuth(s, s.handleCurrentUserPassword, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
