package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

func testAuthDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(storage.Config{Path: dir + "/test.db"})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInitDB(t *testing.T) {
	db := testAuthDB(t)
	InitDB(db)
	if DB != db {
		t.Error("DB should be set")
	}
}

func TestInitAdmin(t *testing.T) {
	db := testAuthDB(t)
	if err := InitAdmin(db, "admin", "admin123"); err != nil {
		t.Fatalf("InitAdmin() error = %v", err)
	}

	user, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	if user.Role != "admin" {
		t.Errorf("Role = %v, want admin", user.Role)
	}
}

func TestHandleLoginSuccess(t *testing.T) {
	db := testAuthDB(t)
	InitDB(db)
	db.InitAdminUser("admin", "password123")

	body, _ := json.Marshal(LoginRequest{Username: "admin", Password: "password123"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()

	HandleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}

	var resp LoginResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Error("Token should not be empty")
	}
	if resp.Username != "admin" {
		t.Errorf("Username = %v, want admin", resp.Username)
	}
	if resp.Role != "admin" {
		t.Errorf("Role = %v, want admin", resp.Role)
	}
}

func TestHandleLoginWrongMethod(t *testing.T) {
	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()

	HandleLogin(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleLoginInvalidBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/login", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()

	HandleLogin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleLoginWrongPassword(t *testing.T) {
	db := testAuthDB(t)
	InitDB(db)
	db.InitAdminUser("admin", "password123")

	body, _ := json.Marshal(LoginRequest{Username: "admin", Password: "wrong"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()

	HandleLogin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestHandleLoginNonexistentUser(t *testing.T) {
	db := testAuthDB(t)
	InitDB(db)

	body, _ := json.Marshal(LoginRequest{Username: "nobody", Password: "pass"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()

	HandleLogin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestHandleRefreshSuccess(t *testing.T) {
	db := testAuthDB(t)
	InitDB(db)
	db.InitAdminUser("admin", "password123")

	// Get a valid token first
	token, _ := GenerateToken("user-1", "admin", "admin")

	req := httptest.NewRequest("POST", "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	HandleRefresh(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["token"] == "" {
		t.Error("New token should not be empty")
	}
}

func TestHandleRefreshWrongMethod(t *testing.T) {
	req := httptest.NewRequest("GET", "/refresh", nil)
	w := httptest.NewRecorder()

	HandleRefresh(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleRefreshNoAuth(t *testing.T) {
	req := httptest.NewRequest("POST", "/refresh", nil)
	w := httptest.NewRecorder()

	HandleRefresh(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestHandleRefreshInvalidToken(t *testing.T) {
	req := httptest.NewRequest("POST", "/refresh", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	HandleRefresh(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestHandleRefreshBearerValidation(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{"short header", "Bearer"},
		{"no bearer prefix", "Basic abc123"},
		{"just word", "token"},
		{"bearer lowercase", "bearer abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/refresh", nil)
			req.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()

			HandleRefresh(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401 for header %q", w.Code, tt.header)
			}
		})
	}
}

func TestGenerateTmpToken(t *testing.T) {
	tmpTokenStore = make(map[string]*TmpTokenData)

	token := generateTmpToken("user-1")
	if token == "" {
		t.Error("token should not be empty")
	}
	if len(token) != 64 {
		t.Errorf("token length = %d, want 64 (hex of 32 bytes)", len(token))
	}
}

func TestValidateTmpToken(t *testing.T) {
	tmpTokenStore = make(map[string]*TmpTokenData)

	token := generateTmpToken("user-1")

	userID, ok := ValidateTmpToken(token)
	if !ok {
		t.Error("ValidateTmpToken() should return true")
	}
	if userID != "user-1" {
		t.Errorf("UserID = %v, want user-1", userID)
	}
}

func TestValidateTmpTokenNotFound(t *testing.T) {
	tmpTokenStore = make(map[string]*TmpTokenData)

	_, ok := ValidateTmpToken("nonexistent")
	if ok {
		t.Error("ValidateTmpToken() should return false for nonexistent token")
	}
}

func TestValidateTmpTokenExpired(t *testing.T) {
	tmpTokenStore = make(map[string]*TmpTokenData)

	// Manually insert expired token
	tmpTokenStore["expired"] = &TmpTokenData{
		UserID:    "user-1",
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}

	_, ok := ValidateTmpToken("expired")
	if ok {
		t.Error("ValidateTmpToken() should return false for expired token")
	}
}

func TestConsumeTmpToken(t *testing.T) {
	tmpTokenStore = make(map[string]*TmpTokenData)

	token := generateTmpToken("user-1")

	if !ConsumeTmpToken(token) {
		t.Error("ConsumeTmpToken() should return true")
	}
	// Second consumption fails
	if ConsumeTmpToken(token) {
		t.Error("ConsumeTmpToken() should return false after consumption")
	}
}

func TestConsumeTmpTokenExpired(t *testing.T) {
	tmpTokenStore = make(map[string]*TmpTokenData)

	tmpTokenStore["expired"] = &TmpTokenData{
		UserID:    "user-1",
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}

	if ConsumeTmpToken("expired") {
		t.Error("ConsumeTmpToken() should return false for expired token")
	}
}

func TestConsumeTmpTokenNotFound(t *testing.T) {
	tmpTokenStore = make(map[string]*TmpTokenData)

	if ConsumeTmpToken("nonexistent") {
		t.Error("ConsumeTmpToken() should return false for nonexistent token")
	}
}

func TestHandleLoginWithTOTPEnabled(t *testing.T) {
	db := testAuthDB(t)
	InitDB(db)
	db.CreateUser(&storage.User{
		Username:    "totpuser",
		Password:    "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", // hash of "secret"
		Role:        "admin",
		TOTPEnabled: true,
		TOTPSecret:  "some-secret",
	})

	// We need a real bcrypt hash to test VerifyPassword
	hashed, _ := storage.HashPassword("secret123")
	user, _ := db.GetUserByUsername("totpuser")
	db.UpdatePassword(user.ID, hashed)
	user, _ = db.GetUserByUsername("totpuser")

	body, _ := json.Marshal(LoginRequest{Username: "totpuser", Password: "secret123"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()

	HandleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}

	var resp LoginResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.RequiresTOTP {
		t.Error("RequiresTOTP should be true")
	}
	if resp.TmpToken == "" {
		t.Error("TmpToken should not be empty when TOTP is enabled")
	}
	if resp.Token != "" {
		t.Error("Token should be empty when TOTP is required")
	}
}
