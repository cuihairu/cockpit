package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
