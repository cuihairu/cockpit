package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ Additional Metrics Tests ============

func TestHandleSnapshotsEmpty(t *testing.T) {
	s := newTestServerWithDB(t)
	_, req := doAuthenticatedRequest(s, "GET", "/api/metrics/snapshots", nil)
	rec := callWithAuth(s, s.handleSnapshots, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	data, _ := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d items", len(data))
	}
}

func TestHandleMetricsHistoryWithTimeRange(t *testing.T) {
	s := newTestServerWithDB(t)
	_, req := doAuthenticatedRequest(s, "GET", "/api/metrics/history?agent_id=a1&start=1000000&end=2000000&limit=10", nil)
	rec := callWithAuth(s, s.handleMetricsHistory, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// ============ Login with Audit Tests ============

func TestHandleLoginWithAuditSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	s.audit = audit.NewLogger(s.db)
	auth.InitDB(s.db)
	s.db.InitAdminUser("admin", "password123")

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "password123"})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleLoginWithAudit(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestHandleLoginWithAuditFailure(t *testing.T) {
	s := newTestServerWithDB(t)
	s.audit = audit.NewLogger(s.db)
	auth.InitDB(s.db)
	s.db.InitAdminUser("admin", "password123")

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleLoginWithAudit(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// ============ Alert Tests ============

func TestHandleMarkAlertAsReadV2(t *testing.T) {
	s := newTestServerWithDB(t)
	alert := &storage.Alert{
		Type:    "warning",
		Title:   "Test Alert",
		Message: "test message",
	}
	s.db.CreateAlert(alert)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/alerts/"+alert.ID+"/read", nil)
	s.handleMarkAlertAsRead(w, r, alert.ID)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleAlertsListReadAll(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.CreateAlert(&storage.Alert{Type: "warning", Title: "A1", Message: "a1"})
	s.db.CreateAlert(&storage.Alert{Type: "info", Title: "A2", Message: "a2"})

	_, req := doAuthenticatedRequest(s, "PUT", "/api/alerts/read-all", nil)
	req.URL.Path = "/api/alerts/read-all"
	rec := callWithAuth(s, s.handleAlertsList, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
}

// ============ Register API Tests ============

func TestRegisterMetricsAPIRoutes(t *testing.T) {
	s := newTestServerWithDB(t)
	mux := http.NewServeMux()
	s.registerMetricsAPI(mux)

	routes := []string{"/api/metrics/snapshots", "/api/metrics/snapshot", "/api/metrics/history"}
	for _, route := range routes {
		req := httptest.NewRequest("GET", route, nil)
		_, pattern := mux.Handler(req)
		if pattern != route {
			t.Errorf("route %q not registered, got pattern %q", route, pattern)
		}
	}
}

func TestRegisterAuditAPIRoutes(t *testing.T) {
	s := newTestServerWithDB(t)
	mux := http.NewServeMux()
	s.registerAuditAPI(mux)

	routes := []string{"/api/admin/audit/logs", "/api/admin/audit/stats"}
	for _, route := range routes {
		req := httptest.NewRequest("GET", route, nil)
		_, pattern := mux.Handler(req)
		if pattern != route {
			t.Errorf("route %q not registered, got pattern %q", route, pattern)
		}
	}
}

// ============ NewServer & Shutdown Tests ============

func TestNewServerWithConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Server:       &config.ServerConfig{Host: "127.0.0.1", Port: 0},
		Database:     &config.DatabaseConfig{Path: dir + "/test.db"},
		JWT:          &config.JWTConfig{Secret: "test", Expiration: 24 * time.Hour},
		Email:        &config.EmailConfig{Enabled: false},
		Notification: &config.NotificationConfig{Enabled: false},
		Agent:        &config.AgentConfig{APIKeyHeader: "X-API-Key"},
	}

	s := NewServer(cfg)
	if s == nil {
		t.Fatal("NewServer() returned nil")
	}
	if s.db == nil {
		t.Error("Server.db should not be nil")
	}
	if s.registry == nil {
		t.Error("Server.registry should not be nil")
	}
	s.Shutdown()
}

func TestShutdownNilProxyMgrAndDB(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		registry: NewRegistry(),
		ctx:      ctx,
		cancel:   cancel,
	}
	s.Shutdown()
}
