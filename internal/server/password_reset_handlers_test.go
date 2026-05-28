package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/storage"
)

func TestHandleForgotPasswordWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	req := httptest.NewRequest("GET", "/api/auth/forgot-password", nil)
	rec := httptest.NewRecorder()
	s.handleForgotPassword(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleForgotPasswordInvalidBody(t *testing.T) {
	s := newTestServerWithDB(t)
	req := httptest.NewRequest("POST", "/api/auth/forgot-password", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()
	s.handleForgotPassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleForgotPasswordEmptyUsername(t *testing.T) {
	s := newTestServerWithDB(t)
	body, _ := json.Marshal(map[string]string{"username": ""})
	req := httptest.NewRequest("POST", "/api/auth/forgot-password", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleForgotPassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleForgotPasswordUserNotFound(t *testing.T) {
	s := newTestServerWithDB(t)
	body, _ := json.Marshal(map[string]string{"username": "nobody"})
	req := httptest.NewRequest("POST", "/api/auth/forgot-password", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleForgotPassword(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (safe response)", rec.Code)
	}
}

func TestHandleForgotPasswordNoEmail(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.CreateUser(&storage.User{Username: "bob", Role: "user"})

	body, _ := json.Marshal(map[string]string{"username": "bob"})
	req := httptest.NewRequest("POST", "/api/auth/forgot-password", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleForgotPassword(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestHandleForgotPasswordWithEmail(t *testing.T) {
	auth.SetEmailConfig(nil)
	s := newTestServerWithDB(t)
	s.db.CreateUser(&storage.User{Username: "alice", Email: "alice@test.com", Role: "user"})

	body, _ := json.Marshal(map[string]string{"username": "alice"})
	req := httptest.NewRequest("POST", "/api/auth/forgot-password", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleForgotPassword(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestHandleResetPasswordWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	req := httptest.NewRequest("GET", "/api/auth/reset-password", nil)
	rec := httptest.NewRecorder()
	s.handleResetPassword(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleResetPasswordInvalidBody(t *testing.T) {
	s := newTestServerWithDB(t)
	req := httptest.NewRequest("POST", "/api/auth/reset-password", bytes.NewReader([]byte("bad")))
	rec := httptest.NewRecorder()
	s.handleResetPassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleResetPasswordMissingFields(t *testing.T) {
	s := newTestServerWithDB(t)
	body, _ := json.Marshal(map[string]string{"token": "abc"})
	req := httptest.NewRequest("POST", "/api/auth/reset-password", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleResetPassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleResetPasswordShortPassword(t *testing.T) {
	s := newTestServerWithDB(t)
	body, _ := json.Marshal(map[string]string{"token": "abc", "new_password": "12"})
	req := httptest.NewRequest("POST", "/api/auth/reset-password", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleResetPassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleResetPasswordInvalidToken(t *testing.T) {
	s := newTestServerWithDB(t)
	body, _ := json.Marshal(map[string]string{"token": "bad-token", "new_password": "newpass123"})
	req := httptest.NewRequest("POST", "/api/auth/reset-password", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleResetPassword(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandleResetPasswordSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.CreateUser(&storage.User{Username: "alice", Email: "alice@test.com", Role: "user"})
	alice, _ := s.db.GetUserByUsername("alice")

	token, _, _ := auth.GenerateResetToken(alice.ID, "alice@test.com")
	body, _ := json.Marshal(map[string]string{"token": token, "new_password": "newpass123"})
	req := httptest.NewRequest("POST", "/api/auth/reset-password", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleResetPassword(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResetPasswordWithCode(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.CreateUser(&storage.User{Username: "alice", Email: "alice@test.com", Role: "user"})
	alice, _ := s.db.GetUserByUsername("alice")

	token, code, _ := auth.GenerateResetToken(alice.ID, "alice@test.com")
	body, _ := json.Marshal(map[string]string{"token": token, "code": code, "new_password": "newpass123"})
	req := httptest.NewRequest("POST", "/api/auth/reset-password", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleResetPassword(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResetPasswordWrongCode(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.CreateUser(&storage.User{Username: "alice", Email: "alice@test.com", Role: "user"})
	alice, _ := s.db.GetUserByUsername("alice")

	token, _, _ := auth.GenerateResetToken(alice.ID, "alice@test.com")
	body, _ := json.Marshal(map[string]string{"token": token, "code": "000000", "new_password": "newpass123"})
	req := httptest.NewRequest("POST", "/api/auth/reset-password", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleResetPassword(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandleVerifyResetCodeWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	req := httptest.NewRequest("GET", "/api/auth/verify-reset-code", nil)
	rec := httptest.NewRecorder()
	s.handleVerifyResetCode(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleVerifyResetCodeInvalidBody(t *testing.T) {
	s := newTestServerWithDB(t)
	req := httptest.NewRequest("POST", "/api/auth/verify-reset-code", bytes.NewReader([]byte("bad")))
	rec := httptest.NewRecorder()
	s.handleVerifyResetCode(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleVerifyResetCodeInvalid(t *testing.T) {
	s := newTestServerWithDB(t)
	body, _ := json.Marshal(map[string]string{"token": "bad", "code": "000000"})
	req := httptest.NewRequest("POST", "/api/auth/verify-reset-code", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleVerifyResetCode(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["valid"] != false {
		t.Error("valid should be false")
	}
}

func TestHandleVerifyResetCodeValid(t *testing.T) {
	s := newTestServerWithDB(t)
	token, code, _ := auth.GenerateResetToken("user-1", "test@test.com")
	body, _ := json.Marshal(map[string]string{"token": token, "code": code})
	req := httptest.NewRequest("POST", "/api/auth/verify-reset-code", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleVerifyResetCode(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["valid"] != true {
		t.Error("valid should be true")
	}
}
