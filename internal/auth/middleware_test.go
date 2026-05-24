package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContextWithUser(t *testing.T) {
	ctx := context.Background()
	userID := "user-123"
	username := "testuser"
	role := "admin"

	newCtx := contextWithUser(ctx, userID, username, role)

	user, ok := newCtx.Value(userKey).(UserInfo)
	if !ok {
		t.Fatal("User should be in context")
	}

	if user.UserID != userID {
		t.Errorf("UserID = %v, want %v", user.UserID, userID)
	}

	if user.Username != username {
		t.Errorf("Username = %v, want %v", user.Username, username)
	}

	if user.Role != role {
		t.Errorf("Role = %v, want %v", user.Role, role)
	}
}

func TestGetUserFromContext(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *http.Request
		wantOK  bool
		wantID  string
	}{
		{
			name: "user in context",
			setup: func() *http.Request {
				req := httptest.NewRequest("GET", "/", nil)
				ctx := contextWithUser(req.Context(), "user-1", "alice", "admin")
				return req.WithContext(ctx)
			},
			wantOK: true,
			wantID: "user-1",
		},
		{
			name: "no user in context",
			setup: func() *http.Request {
				return httptest.NewRequest("GET", "/", nil)
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setup()

			user, ok := GetUserFromContext(req)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}

			if tt.wantOK && user.UserID != tt.wantID {
				t.Errorf("UserID = %v, want %v", user.UserID, tt.wantID)
			}
		})
	}
}

func TestUserInfo(t *testing.T) {
	user := UserInfo{
		UserID:   "123",
		Username: "bob",
		Role:     "user",
	}

	if user.UserID != "123" {
		t.Errorf("UserID = %v, want 123", user.UserID)
	}

	if user.Username != "bob" {
		t.Errorf("Username = %v, want bob", user.Username)
	}

	if user.Role != "user" {
		t.Errorf("Role = %v, want user", user.Role)
	}
}

func TestMiddlewareNoAuthHeader(t *testing.T) {
	nextCalled := false
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}

	middleware := Middleware(next)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	middleware(w, req)

	if nextCalled {
		t.Error("Next handler should not be called without auth header")
	}

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Authorization header required") {
		t.Errorf("Body should contain error message, got: %s", body)
	}
}

func TestMiddlewareInvalidAuthFormat(t *testing.T) {
	nextCalled := false
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}

	middleware := Middleware(next)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "InvalidFormat")
	w := httptest.NewRecorder()

	middleware(w, req)

	if nextCalled {
		t.Error("Next handler should not be called with invalid auth format")
	}

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Invalid authorization format") {
		t.Errorf("Body should contain error message, got: %s", body)
	}
}

func TestMiddlewareInvalidToken(t *testing.T) {
	nextCalled := false
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}

	middleware := Middleware(next)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	middleware(w, req)

	if nextCalled {
		t.Error("Next handler should not be called with invalid token")
	}

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestMiddlewareValidToken(t *testing.T) {
	SetSecret("test-secret")
	token, _ := GenerateToken("user-456", "charlie", "admin")

	nextCalled := false
	var receivedUser UserInfo
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		user, _ := GetUserFromContext(r)
		receivedUser = user
	}

	middleware := Middleware(next)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	middleware(w, req)

	if !nextCalled {
		t.Error("Next handler should be called with valid token")
	}

	if receivedUser.UserID != "user-456" {
		t.Errorf("UserID = %v, want user-456", receivedUser.UserID)
	}

	if receivedUser.Username != "charlie" {
		t.Errorf("Username = %v, want charlie", receivedUser.Username)
	}

	if receivedUser.Role != "admin" {
		t.Errorf("Role = %v, want admin", receivedUser.Role)
	}
}

func TestOptionalMiddlewareNoAuthHeader(t *testing.T) {
	nextCalled := false
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}

	middleware := OptionalMiddleware(next)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	middleware(w, req)

	if !nextCalled {
		t.Error("Next handler should be called without auth header")
	}

	// No user should be in context
	user, ok := GetUserFromContext(req)
	if ok {
		t.Errorf("User should not be in context, got: %+v", user)
	}
}

func TestOptionalMiddlewareInvalidToken(t *testing.T) {
	nextCalled := false
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}

	middleware := OptionalMiddleware(next)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	middleware(w, req)

	if !nextCalled {
		t.Error("Next handler should be called even with invalid token")
	}

	// No user should be in context when token is invalid
	user, ok := GetUserFromContext(req)
	if ok {
		t.Errorf("User should not be in context with invalid token, got: %+v", user)
	}
}

func TestOptionalMiddlewareValidToken(t *testing.T) {
	SetSecret("test-secret")
	token, _ := GenerateToken("user-789", "dave", "user")

	nextCalled := false
	var receivedUser UserInfo
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		user, _ := GetUserFromContext(r)
		receivedUser = user
	}

	middleware := OptionalMiddleware(next)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	middleware(w, req)

	if !nextCalled {
		t.Error("Next handler should be called with valid token")
	}

	if receivedUser.UserID != "user-789" {
		t.Errorf("UserID = %v, want user-789", receivedUser.UserID)
	}
}

func TestOptionalMiddlewareInvalidFormat(t *testing.T) {
	nextCalled := false
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}

	middleware := OptionalMiddleware(next)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "InvalidFormat token")
	w := httptest.NewRecorder()

	middleware(w, req)

	if !nextCalled {
		t.Error("Next handler should be called with invalid format")
	}

	// No user should be in context
	user, ok := GetUserFromContext(req)
	if ok {
		t.Errorf("User should not be in context with invalid format, got: %+v", user)
	}
}
