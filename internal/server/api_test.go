package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// authenticateAdminRequest sets up admin user and injects auth context via middleware.
// Returns (recorder, request) ready for handler invocation.
func doAuthenticatedRequest(s *Server, method, path string, body []byte) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	token, _ := auth.GenerateToken("1", "admin", "admin")
	req.Header.Set("Authorization", "Bearer "+token)
	return httptest.NewRecorder(), req
}

// setupAdmin creates admin user in DB
func setupAdmin(s *Server) {
	auth.InitAdmin(s.db, "admin", "admin123")
}

// callWithAuth wraps handler with auth.Middleware and calls it
func callWithAuth(s *Server, handler http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	auth.Middleware(handler)(rec, r)
	return rec
}

// newTestServerWithDB creates a Server with a real in-memory SQLite DB
func newTestServerWithDB(t *testing.T) *Server {
	t.Helper()

	db, err := storage.Open(storage.Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to open test DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	auth.InitDB(db)

	return &Server{
		registry: NewRegistry(),
		db:       db,
	}
}

// authenticateAdmin creates an admin user and returns an authenticated request
func authenticateAdminRequest(t *testing.T, s *Server, r *http.Request) *http.Request {
	t.Helper()

	err := auth.InitAdmin(s.db, "admin", "admin123")
	if err != nil {
		t.Fatalf("Failed to init admin: %v", err)
	}

	token, err := auth.GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

// ============ serveAPI Tests ============

func TestServeAPIOptions(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("OPTIONS", "/api/status", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("OPTIONS status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestServeAPINotFound(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/nonexistent", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ============ handleStatus Tests ============

func TestHandleStatus(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var result map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&result)
	if _, ok := result["services"]; !ok {
		t.Error("response should contain 'services'")
	}
	if _, ok := result["infrastructure"]; !ok {
		t.Error("response should contain 'infrastructure'")
	}
}

func TestHandleStatusWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("POST", "/api/status", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// ============ handleAgentsList Tests ============

func TestHandleAgentsList(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/agents", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleAgentsListWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("POST", "/api/agents", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// ============ handleAgentGet Tests ============

func TestHandleAgentGetNotFound(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/agents/nonexistent", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ============ handleResources Tests ============

func TestHandleResourcesWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("POST", "/api/resources/compute-instances", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleResourcesUnknownType(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/unknown-type", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleComputeInstancesList(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/compute-instances", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var result map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&result)
	if _, ok := result["data"]; !ok {
		t.Error("response should contain 'data'")
	}
}

func TestHandleDomainsList(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/domains", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleCertificatesList(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/certificates", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleServicesList(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/services", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleGatewaysList(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/gateways", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleStoragesList(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/storages", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ============ handleHealth Tests (in server context) ============

func TestHandleHealthWithAgents(t *testing.T) {
	s := newTestServerWithDB(t)
	s.registry.Register(NewAgent("a1", nil))
	s.registry.Register(NewAgent("a2", nil))

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	s.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var result map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&result)
	if agents, ok := result["agents"].(float64); !ok || int(agents) != 2 {
		t.Errorf("agents = %v, want 2", result["agents"])
	}
}

// ============ writeJSON / handleError Tests ============

func TestWriteJSON(t *testing.T) {
	s := newTestServerWithDB(t)
	rec := httptest.NewRecorder()

	s.writeJSON(rec, http.StatusCreated, map[string]string{"key": "value"})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestHandleErrorJSON(t *testing.T) {
	s := newTestServerWithDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)

	s.handleError(rec, req, http.StatusBadRequest, "bad request")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rec.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&result)
	if result["error"] != "bad request" {
		t.Errorf("error = %v", result["error"])
	}
}

// ============ storageAgentToResponse Tests ============

func TestStorageAgentToResponse(t *testing.T) {
	agent := &storage.Agent{
		ID:       "a1",
		Hostname: "test",
		IP:       "10.0.0.1",
		Region:   "us-east",
		Zone:     "us-east-1a",
		Status:   "online",
		Capabilities: []storage.Capability{
			{Type: "proxy"},
			{Type: "docker"},
		},
	}

	resp := storageAgentToResponse(agent)
	if resp["id"] != "a1" {
		t.Errorf("id = %v", resp["id"])
	}
	if resp["hostname"] != "test" {
		t.Errorf("hostname = %v", resp["hostname"])
	}
	caps, ok := resp["capabilities"].([]string)
	if !ok || len(caps) != 2 {
		t.Errorf("capabilities = %v", resp["capabilities"])
	}
	loc, ok := resp["location"].(map[string]string)
	if !ok || loc["region"] != "us-east" {
		t.Errorf("location = %v", resp["location"])
	}
}

// ============ User API Tests ============

func TestHandleUsersList(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/users", nil)
	result := callWithAuth(s, s.handleUsers, req)

	if result.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", result.Code, http.StatusOK)
	}
}

func TestHandleUsersWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("DELETE", "/api/users", nil)
	rec := httptest.NewRecorder()

	s.handleUsers(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleUserCreateUnauthorized(t *testing.T) {
	s := newTestServerWithDB(t)

	body, _ := json.Marshal(CreateUserRequest{
		Username: "newuser",
		Password: "pass123",
	})
	req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	s.handleUserCreate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleUserCreateSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	body, _ := json.Marshal(CreateUserRequest{
		Username: "newuser",
		Password: "pass123",
		Email:    "new@test.com",
		Role:     "user",
	})
	_, req := doAuthenticatedRequest(s, "POST", "/api/users", body)
	result := callWithAuth(s, s.handleUserCreate, req)

	if result.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", result.Code, http.StatusCreated, result.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(result.Body).Decode(&resp)
	if resp["username"] != "newuser" {
		t.Errorf("username = %v", resp["username"])
	}
	if resp["id"] == nil {
		t.Error("id should be set")
	}
}

func TestHandleUserCreateMissingFields(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	body, _ := json.Marshal(CreateUserRequest{Username: "no-password"})
	_, req := doAuthenticatedRequest(s, "POST", "/api/users", body)
	result := callWithAuth(s, s.handleUserCreate, req)

	if result.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", result.Code, http.StatusBadRequest)
	}
}

func TestHandleUserCreateDuplicate(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	// Create first user
	body, _ := json.Marshal(CreateUserRequest{Username: "dup", Password: "pass123"})
	_, req := doAuthenticatedRequest(s, "POST", "/api/users", body)
	callWithAuth(s, s.handleUserCreate, req)

	// Try to create duplicate
	body, _ = json.Marshal(CreateUserRequest{Username: "dup", Password: "pass456"})
	_, req = doAuthenticatedRequest(s, "POST", "/api/users", body)
	result := callWithAuth(s, s.handleUserCreate, req)

	if result.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", result.Code, http.StatusConflict)
	}
}

func TestHandleUserCreateInvalidJSON(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "POST", "/api/users", []byte("not json"))
	result := callWithAuth(s, s.handleUserCreate, req)

	if result.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", result.Code, http.StatusBadRequest)
	}
}

// ============ CORS Tests ============

func TestServeAPICORS(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("OPTIONS", "/api/status", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	origin := rec.Header().Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("CORS origin = %q, want %q", origin, "*")
	}
	methods := rec.Header().Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("CORS methods should be set")
	}
}

func TestServeAPICORSWithAllowedOrigins(t *testing.T) {
	s := newTestServerWithDB(t)
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:3000")

	req := httptest.NewRequest("OPTIONS", "/api/status", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	origin := rec.Header().Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:3000" {
		t.Errorf("CORS origin = %q, want %q", origin, "http://localhost:3000")
	}
}

// ============ SPA Handler Tests ============

func TestIsOriginAllowedWithConfig(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173")

	r := httptest.NewRequest("GET", "/test", nil)
	r.Header.Set("Origin", "http://localhost:3000")
	if !isOriginAllowed(r) {
		t.Error("should allow whitelisted origin")
	}

	r.Header.Set("Origin", "http://evil.com")
	if isOriginAllowed(r) {
		t.Error("should reject non-whitelisted origin")
	}

	// Wildcard
	t.Setenv("ALLOWED_ORIGINS", "*")
	r.Header.Set("Origin", "http://anything.com")
	if !isOriginAllowed(r) {
		t.Error("should allow wildcard origin")
	}
}

// ============ SendToAgent / GetAgentConn Tests ============

func TestSendToAgentNotFound(t *testing.T) {
	s := newTestServerWithDB(t)

	err := s.SendToAgent("nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestSendToAgentSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	agent := NewAgent("a1", nil)
	s.registry.Register(agent)

	msg := protocol.NewMessage("test", nil)
	err := s.SendToAgent("a1", msg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify message was sent
	select {
	case m := <-agent.Send:
		if m.Type != "test" {
			t.Errorf("message type = %v", m.Type)
		}
	default:
		t.Error("message should be in channel")
	}
}

func TestGetAgentConn(t *testing.T) {
	s := newTestServerWithDB(t)
	s.registry.Register(NewAgent("a1", nil))

	conn, ok := s.GetAgentConn("a1")
	if !ok {
		t.Error("should find agent")
	}
	if conn == nil {
		t.Error("conn should not be nil")
	}

	_, ok = s.GetAgentConn("nonexistent")
	if ok {
		t.Error("should not find nonexistent agent")
	}
}

// ============ Metrics API Tests ============

func TestHandleSnapshots(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/metrics/snapshots", nil)
	result := callWithAuth(s, s.handleSnapshots, req)

	if result.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", result.Code, http.StatusOK)
	}
}

func TestHandleSnapshotsWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "POST", "/api/metrics/snapshots", nil)
	result := callWithAuth(s, s.handleSnapshots, req)

	if result.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", result.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleSnapshotMissingAgentID(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/metrics/snapshot", nil)
	result := callWithAuth(s, s.handleSnapshot, req)

	if result.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", result.Code, http.StatusBadRequest)
	}
}

func TestHandleSnapshotNotFound(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/metrics/snapshot?agent_id=nonexistent", nil)
	result := callWithAuth(s, s.handleSnapshot, req)

	if result.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", result.Code, http.StatusNotFound)
	}
}

func TestHandleMetricsHistoryMissingAgentID(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/metrics/history", nil)
	result := callWithAuth(s, s.handleMetricsHistory, req)

	if result.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", result.Code, http.StatusBadRequest)
	}
}

func TestHandleMetricsHistoryWithParams(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/metrics/history?agent_id=a1&limit=100", nil)
	result := callWithAuth(s, s.handleMetricsHistory, req)

	if result.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", result.Code, http.StatusOK)
	}
}

func TestHandleMetricsHistoryWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "POST", "/api/metrics/history", nil)
	result := callWithAuth(s, s.handleMetricsHistory, req)

	if result.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", result.Code, http.StatusMethodNotAllowed)
	}
}

// ============ Audit API Tests ============

func TestHandleAuditLogs(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/admin/audit/logs", nil)
	result := callWithAuth(s, s.handleAuditLogs, req)

	if result.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", result.Code, http.StatusOK)
	}
}

func TestHandleAuditLogsWithFilters(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/admin/audit/logs?page=2&page_size=10&action=login&username=admin&status=success", nil)
	result := callWithAuth(s, s.handleAuditLogs, req)

	if result.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", result.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(result.Body).Decode(&resp)
	if _, ok := resp["pagination"]; !ok {
		t.Error("response should contain pagination")
	}
}

func TestHandleAuditLogsWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "POST", "/api/admin/audit/logs", nil)
	result := callWithAuth(s, s.handleAuditLogs, req)

	if result.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", result.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAuditLogStats(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/admin/audit/stats", nil)
	result := callWithAuth(s, s.handleAuditLogStats, req)

	if result.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", result.Code, http.StatusOK)
	}
}

func TestHandleAuditLogStatsWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "POST", "/api/admin/audit/stats", nil)
	result := callWithAuth(s, s.handleAuditLogStats, req)

	if result.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", result.Code, http.StatusMethodNotAllowed)
	}
}

// ============ Proxy API Tests ============

func TestHandleProxies(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/proxies", nil)
	result := callWithAuth(s, s.handleProxies, req)

	if result.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", result.Code, http.StatusOK)
	}
}

func TestHandleProxiesWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "DELETE", "/api/proxies", nil)
	result := callWithAuth(s, s.handleProxies, req)

	if result.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", result.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleProxyCreateInvalidJSON(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "POST", "/api/proxies", []byte("bad json"))
	result := callWithAuth(s, s.handleProxyCreate, req)

	if result.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", result.Code, http.StatusBadRequest)
	}
}

func TestHandleProxyCreateMissingFields(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	body, _ := json.Marshal(map[string]string{"name": "test"})
	_, req := doAuthenticatedRequest(s, "POST", "/api/proxies", body)
	result := callWithAuth(s, s.handleProxyCreate, req)

	if result.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", result.Code, http.StatusBadRequest)
	}
}

func TestHandleProxyCreateAgentNotFound(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "test-proxy", "agentId": "nonexistent", "remotePort": 8080, "target": "localhost:3000",
	})
	_, req := doAuthenticatedRequest(s, "POST", "/api/proxies", body)
	result := callWithAuth(s, s.handleProxyCreate, req)

	if result.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", result.Code, http.StatusNotFound)
	}
}

func TestHandleProxyUpdateInvalidJSON(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "PUT", "/api/proxies", []byte("bad json"))
	result := callWithAuth(s, s.handleProxyUpdate, req)

	if result.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", result.Code, http.StatusBadRequest)
	}
}

func TestHandleProxyUpdateMissingID(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	body, _ := json.Marshal(map[string]string{"name": "test"})
	_, req := doAuthenticatedRequest(s, "PUT", "/api/proxies", body)
	result := callWithAuth(s, s.handleProxyUpdate, req)

	if result.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", result.Code, http.StatusBadRequest)
	}
}

func TestHandleProxyUpdateNotFound(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	body, _ := json.Marshal(map[string]string{"id": "nonexistent", "name": "test"})
	_, req := doAuthenticatedRequest(s, "PUT", "/api/proxies", body)
	result := callWithAuth(s, s.handleProxyUpdate, req)

	if result.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", result.Code, http.StatusNotFound)
	}
}

func TestHandleProxyDeleteMissingID(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	body, _ := json.Marshal(map[string]string{})
	_, req := doAuthenticatedRequest(s, "DELETE", "/api/proxies", body)
	result := callWithAuth(s, s.handleProxyDelete, req)

	if result.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", result.Code, http.StatusBadRequest)
	}
}

func TestHandleProxyStatusNoMgr(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/proxies/status", nil)
	result := callWithAuth(s, s.handleProxyStatus, req)

	if result.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", result.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleProxyStatusWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "POST", "/api/proxies/status", nil)
	result := callWithAuth(s, s.handleProxyStatus, req)

	if result.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", result.Code, http.StatusMethodNotAllowed)
	}
}

// ============ Alert API Tests ============

func TestHandleAlertsListUnauthorized(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/alerts", nil)
	rec := httptest.NewRecorder()

	s.handleAlertsList(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleAlertsListSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/alerts", nil)
	result := callWithAuth(s, s.handleAlertsList, req)

	if result.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", result.Code, http.StatusOK)
	}
}

func TestHandleAlertsListWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "DELETE", "/api/alerts", nil)
	result := callWithAuth(s, s.handleAlertsList, req)

	if result.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", result.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAlertActionsUnauthorized(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("PUT", "/api/alerts/1/read", nil)
	rec := httptest.NewRecorder()

	s.handleAlertActions(rec, req, "1/read")

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleAlertActionsNoAction(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/alerts/", nil)
	result := callWithAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleAlertActions(w, r, "")
	}, req)

	if result.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", result.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAlertActionsWrongMethod(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/alerts/1/read", nil)
	result := callWithAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleAlertActions(w, r, "1/read")
	}, req)

	if result.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", result.Code, http.StatusMethodNotAllowed)
	}
}

// ============ User CRUD Tests ============

func TestHandleUserDeleteUnauthorized(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("DELETE", "/api/users/1", nil)
	rec := httptest.NewRecorder()

	s.handleUserDelete(rec, req, "1")

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleUserUpdateUnauthorized(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("PUT", "/api/users/1", nil)
	rec := httptest.NewRecorder()

	s.handleUserUpdate(rec, req, "1")

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleUserChangePasswordUnauthorized(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("POST", "/api/users/1/password", nil)
	rec := httptest.NewRecorder()

	s.handleUserChangePassword(rec, req, "1")

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleUserActionsNoPath(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/users/", nil)
	rec := httptest.NewRecorder()

	s.handleUserActions(rec, req, "")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleUserActionsDefaultMethod(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/users/1", nil)
	rec := httptest.NewRecorder()

	s.handleUserActions(rec, req, "1")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// ============ Resource Get Tests ============

func TestHandleComputeInstanceGetNotFound(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/compute-instances/nonexistent", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDomainGetNotFound(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/domains/nonexistent", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCertificateGetNotFound(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/certificates/nonexistent", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleServiceGetNotFound(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/services/nonexistent", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGatewayGetNotFound(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/gateways/nonexistent", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleStorageGetNotFound(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest("GET", "/api/resources/storages/nonexistent", nil)
	rec := httptest.NewRecorder()

	s.serveAPI(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ============ AuditMiddleware Tests ============

func TestAuditMiddlewareRecordsPost(t *testing.T) {
	db, err := storage.Open(storage.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := &Server{
		registry: NewRegistry(),
		db:       db,
		audit:    audit.NewLogger(db),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	middleware := s.AuditMiddleware(handler)

	req := httptest.NewRequest("POST", "/api/agents", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestAuditMiddlewareSkipsHealthCheck(t *testing.T) {
	db, _ := storage.Open(storage.Config{Path: ":memory:"})
	defer db.Close()

	s := &Server{registry: NewRegistry(), db: db}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := s.AuditMiddleware(handler)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestAuditMiddlewareSkipsGetRequests(t *testing.T) {
	db, _ := storage.Open(storage.Config{Path: ":memory:"})
	defer db.Close()

	s := &Server{registry: NewRegistry(), db: db}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := s.AuditMiddleware(handler)

	req := httptest.NewRequest("GET", "/api/agents", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}

// ============ handleMarkAlertAsRead Tests ============

func TestHandleMarkAlertAsRead(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	req := httptest.NewRequest("PUT", "/api/alerts/some-id/read", nil)
	rec := httptest.NewRecorder()

	s.handleMarkAlertAsRead(rec, req, "some-id")

	// SQLite won't error on UPDATE with no matching rows
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ============ handleRPCResponse Tests ============

func TestHandleRPCResponseNoPending(t *testing.T) {
	s := newTestServerWithDB(t)

	msg := protocol.NewMessage("rpc_response", nil)
	msg.ID = "nonexistent"

	// Should not panic
	s.handleRPCResponse(msg)
}

func TestHandleRPCResponseWithPending(t *testing.T) {
	s := newTestServerWithDB(t)

	ch := make(chan *protocol.Message, 1)
	s.registry.RegisterPendingResponse("msg-1", ch)

	msg := protocol.NewMessage("rpc_response", map[string]interface{}{"result": "ok"})
	msg.ID = "msg-1"

	s.handleRPCResponse(msg)

	select {
	case received := <-ch:
		if received.ID != "msg-1" {
			t.Errorf("ID = %v", received.ID)
		}
	default:
		t.Error("message should be in channel")
	}
}

// ============ handleHeartbeat Tests ============

func TestHandleHeartbeat(t *testing.T) {
	s := newTestServerWithDB(t)
	agent := NewAgent("a1", nil)
	s.registry.Register(agent)

	msg := protocol.NewMessage("heartbeat", nil)
	msg.ID = "hb-1"

	s.handleHeartbeat(agent, msg)

	// Verify heartbeat was updated
	found, _ := s.registry.Get("a1")
	if time.Since(found.LastSeen) > time.Second {
		t.Error("heartbeat should be updated")
	}

	// Verify ACK was sent
	select {
	case resp := <-agent.Send:
		if resp.ID != "hb-1" {
			t.Errorf("ACK ID = %v, want hb-1", resp.ID)
		}
	default:
		t.Error("ACK should be sent")
	}
}

// ============ handleSystemInfo Tests ============

func TestHandleSystemInfo(t *testing.T) {
	s := newTestServerWithDB(t)
	// Register agent in DB first
	s.db.UpsertAgent(&storage.Agent{ID: "a1", Status: "online"})

	systemInfo := map[string]interface{}{
		"cpuUsage":         45.5,
		"cpuCores":         float64(4),
		"cpuFreqMhz":       3200.0,
		"memTotal":         float64(8589934592),
		"memUsed":          float64(4294967296),
		"memAvailable":     float64(4294967296),
		"memUsagePercent":  50.0,
		"diskTotal":        float64(107374182400),
		"diskUsed":         float64(53687091200),
		"diskFree":         float64(53687091200),
		"diskUsagePercent": 50.0,
		"netBytesSent":     float64(1000000),
		"netBytesRecv":     float64(2000000),
		"osName":           "Linux",
		"osVersion":        "5.15.0",
		"arch":             "amd64",
		"uptime":           float64(86400),
		"load1":            1.5,
		"load5":            1.2,
		"load15":           1.0,
		"hostname":         "test-host",
	}

	// Should not panic
	s.handleSystemInfo("a1", systemInfo)
}

// ============ handleProxyError Tests ============

func TestHandleProxyError(t *testing.T) {
	s := newTestServerWithDB(t)
	agent := NewAgent("a1", nil)

	msg := protocol.NewMessage("proxy_error", map[string]interface{}{
		"proxyId": "proxy-1",
		"error":   "connection refused",
	})

	// Should not panic
	s.handleProxyError(agent, msg)
}

// ============ User Delete Tests ============

func TestHandleUserDeleteSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	// Create a user to delete
	s.db.CreateUser(&storage.User{
		Username: "deleteme",
		Password: "hashed",
		Role:     "user",
	})

	users, _ := s.db.ListUsers()
	var targetID string
	for _, u := range users {
		if u.Username == "deleteme" {
			targetID = u.ID
			break
		}
	}

	_, req := doAuthenticatedRequest(s, http.MethodDelete, "/api/users/"+targetID, nil)
	rr := callWithAuth(s, s.makeUserHandler(targetID), req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleUserDeleteSelf(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	users, _ := s.db.ListUsers()
	var adminID string
	for _, u := range users {
		if u.Username == "admin" {
			adminID = u.ID
			break
		}
	}

	_, req := doAuthenticatedRequest(s, http.MethodDelete, "/api/users/"+adminID, nil)
	rr := callWithAuth(s, s.makeUserHandler(adminID), req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d for deleting self", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleUserDeleteNotFound(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, http.MethodDelete, "/api/users/999", nil)
	rr := callWithAuth(s, s.makeUserHandler("999"), req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ============ User Update Tests ============

func TestHandleUserUpdateSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	// Create a user to update
	s.db.CreateUser(&storage.User{
		Username: "updateme",
		Password: "hashed",
		Role:     "user",
		Email:    "old@test.com",
	})

	users, _ := s.db.ListUsers()
	var targetID string
	for _, u := range users {
		if u.Username == "updateme" {
			targetID = u.ID
			break
		}
	}

	body, _ := json.Marshal(map[string]string{
		"email": "new@test.com",
		"role":  "admin",
	})

	_, req := doAuthenticatedRequest(s, http.MethodPut, "/api/users/"+targetID, body)
	rr := callWithAuth(s, s.makeUserHandler(targetID), req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleUserUpdateNotFound(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	body, _ := json.Marshal(map[string]string{"email": "x@x.com"})
	_, req := doAuthenticatedRequest(s, http.MethodPut, "/api/users/999", body)
	rr := callWithAuth(s, s.makeUserHandler("999"), req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHandleUserUpdateInvalidJSON(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	s.db.CreateUser(&storage.User{Username: "jsonuser", Password: "h", Role: "user"})
	users, _ := s.db.ListUsers()
	var targetID string
	for _, u := range users {
		if u.Username == "jsonuser" {
			targetID = u.ID
			break
		}
	}

	_, req := doAuthenticatedRequest(s, http.MethodPut, "/api/users/"+targetID, []byte("invalid"))
	rr := callWithAuth(s, s.makeUserHandler(targetID), req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ============ User Change Password Tests ============

func TestHandleUserChangePasswordAdminSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	// Create a regular user
	s.db.CreateUser(&storage.User{Username: "pwuser", Password: "old", Role: "user"})
	users, _ := s.db.ListUsers()
	var targetID string
	for _, u := range users {
		if u.Username == "pwuser" {
			targetID = u.ID
			break
		}
	}

	body, _ := json.Marshal(map[string]string{
		"new_password": "newpass123",
	})

	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/users/"+targetID+"/password", body)
	handler := s.makePasswordHandler(targetID)
	rr := callWithAuth(s, handler, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleUserChangePasswordEmptyNew(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	s.db.CreateUser(&storage.User{Username: "pwempty", Password: "old", Role: "user"})
	users, _ := s.db.ListUsers()
	var targetID string
	for _, u := range users {
		if u.Username == "pwempty" {
			targetID = u.ID
			break
		}
	}

	body, _ := json.Marshal(map[string]string{
		"new_password": "",
	})

	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/users/"+targetID+"/password", body)
	rr := callWithAuth(s, s.makePasswordHandler(targetID), req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleUserChangePasswordNotFound(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	body, _ := json.Marshal(map[string]string{"new_password": "x"})
	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/users/999/password", body)
	rr := callWithAuth(s, s.makePasswordHandler("999"), req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHandleUserChangePasswordInvalidJSON(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	s.db.CreateUser(&storage.User{Username: "pwjson", Password: "old", Role: "user"})
	users, _ := s.db.ListUsers()
	var targetID string
	for _, u := range users {
		if u.Username == "pwjson" {
			targetID = u.ID
			break
		}
	}

	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/users/"+targetID+"/password", []byte("bad"))
	rr := callWithAuth(s, s.makePasswordHandler(targetID), req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ============ User Actions Routing Tests ============

func TestHandleUserActionsDelete(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	s.db.CreateUser(&storage.User{Username: "delroute", Password: "h", Role: "user"})
	users, _ := s.db.ListUsers()
	var targetID string
	for _, u := range users {
		if u.Username == "delroute" {
			targetID = u.ID
			break
		}
	}

	_, req := doAuthenticatedRequest(s, http.MethodDelete, "/api/users/"+targetID, nil)
	rr := callWithAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserActions(w, r, targetID)
	}, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleUserActionsPassword(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	s.db.CreateUser(&storage.User{Username: "pwroute", Password: "h", Role: "user"})
	users, _ := s.db.ListUsers()
	var targetID string
	for _, u := range users {
		if u.Username == "pwroute" {
			targetID = u.ID
			break
		}
	}

	body, _ := json.Marshal(map[string]string{"new_password": "newpw"})
	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/users/"+targetID+"/password", body)
	rr := callWithAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserActions(w, r, targetID+"/password")
	}, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleUserActionsUpdate(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	s.db.CreateUser(&storage.User{Username: "updroute", Password: "h", Role: "user"})
	users, _ := s.db.ListUsers()
	var targetID string
	for _, u := range users {
		if u.Username == "updroute" {
			targetID = u.ID
			break
		}
	}

	body, _ := json.Marshal(map[string]string{"email": "upd@test.com"})
	_, req := doAuthenticatedRequest(s, http.MethodPut, "/api/users/"+targetID, body)
	rr := callWithAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserActions(w, r, targetID)
	}, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

// ============ Proxy CRUD Success Tests ============

func TestHandleProxyCreateSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	// Register agent
	s.db.UpsertAgent(&storage.Agent{ID: "agent-1", Status: "online"})

	body, _ := json.Marshal(map[string]interface{}{
		"name":       "Test Proxy",
		"agentId":    "agent-1",
		"proxyType":  "tcp",
		"remotePort": 8080,
		"target":     "localhost:3000",
	})

	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/proxies", body)
	rr := callWithAuth(s, s.handleProxyCreate, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
}

func TestHandleProxyCreateUDP(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	s.db.UpsertAgent(&storage.Agent{ID: "agent-1", Status: "online"})

	body, _ := json.Marshal(map[string]interface{}{
		"name":       "UDP Proxy",
		"agentId":    "agent-1",
		"proxyType":  "udp",
		"remotePort": 9090,
		"target":     "localhost:5000",
	})

	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/proxies", body)
	rr := callWithAuth(s, s.handleProxyCreate, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
}

func TestHandleProxyUpdateSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	s.db.UpsertAgent(&storage.Agent{ID: "agent-1", Status: "online"})

	// Create proxy first
	proxy := &storage.Proxy{
		ID:         "proxy-1",
		Name:       "Old",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 8080,
		Target:     "localhost:3000",
		Enabled:    true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	s.db.CreateProxy(proxy)

	body, _ := json.Marshal(map[string]interface{}{
		"id":          "proxy-1",
		"name":        "Updated",
		"target":      "localhost:4000",
		"description": "updated desc",
	})

	_, req := doAuthenticatedRequest(s, http.MethodPut, "/api/proxies", body)
	rr := callWithAuth(s, s.handleProxyUpdate, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleProxyUpdatePortConflict(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	s.db.UpsertAgent(&storage.Agent{ID: "agent-1", Status: "online"})

	// Create two proxies
	s.db.CreateProxy(&storage.Proxy{
		ID: "p1", Name: "P1", AgentID: "agent-1", ProxyType: "tcp",
		RemotePort: 8080, Target: "t1", Enabled: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	s.db.CreateProxy(&storage.Proxy{
		ID: "p2", Name: "P2", AgentID: "agent-1", ProxyType: "tcp",
		RemotePort: 9090, Target: "t2", Enabled: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	body, _ := json.Marshal(map[string]interface{}{
		"id":         "p1",
		"remotePort": 9090, // Conflict with p2
	})

	_, req := doAuthenticatedRequest(s, http.MethodPut, "/api/proxies", body)
	rr := callWithAuth(s, s.handleProxyUpdate, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusConflict)
	}
}

func TestHandleProxyDeleteSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	s.db.CreateProxy(&storage.Proxy{
		ID: "del-proxy", Name: "Del", AgentID: "a1", ProxyType: "tcp",
		RemotePort: 8080, Target: "t", Enabled: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	body, _ := json.Marshal(map[string]string{"id": "del-proxy"})
	_, req := doAuthenticatedRequest(s, http.MethodDelete, "/api/proxies", body)
	rr := callWithAuth(s, s.handleProxyDelete, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusNoContent)
	}
}

func TestHandleProxyDeleteByQueryParam(t *testing.T) {
	s := newTestServerWithDB(t)

	s.db.CreateProxy(&storage.Proxy{
		ID: "qp-proxy", Name: "QP", AgentID: "a1", ProxyType: "tcp",
		RemotePort: 8081, Target: "t", Enabled: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	// Invalid JSON body, should fall back to query param
	req := httptest.NewRequest(http.MethodDelete, "/api/proxies?id=qp-proxy", bytes.NewReader([]byte("invalid")))
	rec := httptest.NewRecorder()
	s.handleProxyDelete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestHandleProxiesWithAgentID(t *testing.T) {
	s := newTestServerWithDB(t)

	s.db.UpsertAgent(&storage.Agent{ID: "a1", Status: "online"})
	s.db.CreateProxy(&storage.Proxy{
		ID: "p1", Name: "P1", AgentID: "a1", ProxyType: "tcp",
		RemotePort: 8080, Target: "t", Enabled: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/proxies?agent_id=a1", nil)
	rec := httptest.NewRecorder()
	s.handleProxies(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ============ Alerts List Success Tests ============

func TestHandleAlertsListWithData(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	// Create alert
	s.db.CreateAlert(&storage.Alert{
		Type:    "warning",
		Title:   "Test",
		Message: "test alert",
	})

	_, req := doAuthenticatedRequest(s, http.MethodGet, "/api/alerts", nil)
	rr := callWithAuth(s, s.handleAlertsList, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleAlertsReadAll(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	s.db.CreateAlert(&storage.Alert{
		Type:    "warning",
		Title:   "Test",
		Message: "test alert",
	})

	_, req := doAuthenticatedRequest(s, http.MethodPut, "/api/alerts/read-all", nil)
	req.URL, _ = url.Parse("/api/alerts/read-all")
	rr := callWithAuth(s, s.handleAlertsList, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleMarkAlertAsReadSuccess(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	s.db.CreateAlert(&storage.Alert{
		Type:    "info",
		Title:   "MarkRead",
		Message: "test",
	})

	alerts, _ := s.db.ListAlerts(50)
	if len(alerts) == 0 {
		t.Fatal("No alerts created")
	}

	_, req := doAuthenticatedRequest(s, http.MethodPut, "/api/alerts/"+alerts[0].ID+"/read", nil)
	rr := callWithAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleMarkAlertAsRead(w, r, alerts[0].ID)
	}, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

// ============ Resource List/Get Success Tests ============

func TestHandleStatusSuccess(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	s.handleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleAgentsListSuccess(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()
	s.handleAgentsList(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleAgentGetSuccess(t *testing.T) {
	s := newTestServerWithDB(t)

	s.db.UpsertAgent(&storage.Agent{ID: "a1", Hostname: "test", Status: "online"})

	req := httptest.NewRequest(http.MethodGet, "/api/agents/a1", nil)
	rec := httptest.NewRecorder()
	s.handleAgentGet(rec, req, "a1")

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleResourcesRouting(t *testing.T) {
	s := newTestServerWithDB(t)

	tests := []struct {
		path string
	}{
		{"compute-instances"},
		{"compute-instances/id1"},
		{"domains"},
		{"domains/id1"},
		{"certificates"},
		{"certificates/id1"},
		{"services"},
		{"services/id1"},
		{"gateways"},
		{"gateways/id1"},
		{"storages"},
		{"storages/id1"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/resources/"+tt.path, nil)
			rec := httptest.NewRecorder()
			s.handleResources(rec, req, tt.path)

			if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
				t.Errorf("Status = %d for %s", rec.Code, tt.path)
			}
		})
	}
}

func TestHandleResourcesWrongMethodV2(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest(http.MethodPost, "/api/resources/domains", nil)
	rec := httptest.NewRecorder()
	s.handleResources(rec, req, "domains")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleResourcesUnknownTypeV2(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/resources/unknown", nil)
	rec := httptest.NewRecorder()
	s.handleResources(rec, req, "unknown")

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ============ serveAPI Routing Tests ============

func TestServeAPIUsersRoute(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	token, _ := auth.GenerateToken("1", "admin", "admin")
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestServeAPINotFoundV2(t *testing.T) {
	s := newTestServerWithDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestServeAPICORSOriginEnv(t *testing.T) {
	s := newTestServerWithDB(t)
	t.Setenv("ALLOWED_ORIGINS", "http://test.com")

	req := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "http://test.com" {
		t.Errorf("CORS origin = %q, want %q", rec.Header().Get("Access-Control-Allow-Origin"), "http://test.com")
	}
}

// ============ Helper functions for user action routing ============

// makeUserHandler creates a handler that routes to handleUserDelete/Update based on method
func (s *Server) makeUserHandler(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			s.handleUserDelete(w, r, id)
		case http.MethodPut:
			s.handleUserUpdate(w, r, id)
		}
	}
}

func (s *Server) makePasswordHandler(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.handleUserChangePassword(w, r, id)
	}
}
