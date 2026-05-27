package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func newTestServer() *Server {
	return &Server{
		registry: NewRegistry(),
	}
}

// ============ shouldAudit Tests ============

func TestShouldAudit(t *testing.T) {
	s := newTestServer()

	tests := []struct {
		method string
		path   string
		want   bool
	}{
		// Health and status should not be audited
		{"GET", "/health", false},
		{"GET", "/api/status", false},

		// GET requests to non-admin API should not be audited
		{"GET", "/api/agents", false},
		{"GET", "/api/agents/agent-1", false},

		// GET to admin API should be audited
		{"GET", "/api/admin/users", true},
		{"GET", "/api/admin/settings", true},

		// Non-GET API requests should be audited
		{"POST", "/api/agents", true},
		{"PUT", "/api/agents/agent-1", true},
		{"PATCH", "/api/agents/agent-1", true},
		{"DELETE", "/api/agents/agent-1", true},

		// Non-API paths should not be audited
		{"GET", "/", false},
		{"GET", "/static/app.js", false},
		{"POST", "/login", false},
	}

	for _, tt := range tests {
		got := s.shouldAudit(tt.method, tt.path)
		if got != tt.want {
			t.Errorf("shouldAudit(%q, %q) = %v, want %v", tt.method, tt.path, got, tt.want)
		}
	}
}

// ============ getActionFromMethod Tests ============

func TestGetActionFromMethod(t *testing.T) {
	s := newTestServer()

	tests := []struct {
		method string
		want   string
	}{
		{"GET", "view"},
		{"POST", "create"},
		{"PUT", "update"},
		{"PATCH", "update"},
		{"DELETE", "delete"},
		{"OPTIONS", "unknown"},
		{"HEAD", "unknown"},
	}

	for _, tt := range tests {
		got := s.getActionFromMethod(tt.method)
		if got != tt.want {
			t.Errorf("getActionFromMethod(%q) = %q, want %q", tt.method, got, tt.want)
		}
	}
}

// ============ getResourceFromPath Tests ============

func TestGetResourceFromPath(t *testing.T) {
	s := newTestServer()

	tests := []struct {
		path string
		want string
	}{
		{"/api/agents", "agents"},
		{"/api/agents/agent-1", "agents"},
		{"/api/admin/users", "admin"},
		{"/api/admin/users/123", "admin"},
		{"/api/proxy/sessions", "proxy"},
		{"/non-api/path", "unknown"},
		{"/health", "unknown"},
		{"/", "unknown"},
	}

	for _, tt := range tests {
		got := s.getResourceFromPath(tt.path)
		if got != tt.want {
			t.Errorf("getResourceFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// ============ getResourceIDFromPath Tests ============

func TestGetResourceIDFromPath(t *testing.T) {
	s := newTestServer()

	tests := []struct {
		path string
		want string
	}{
		{"/api/agents/agent-1", "agent-1"},
		{"/api/agents", "agents"},
		{"/api/admin/users/123", "123"},
		{"/", ""},
		{"/health", "health"},
	}

	for _, tt := range tests {
		got := s.getResourceIDFromPath(tt.path)
		if got != tt.want {
			t.Errorf("getResourceIDFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// ============ getClientIP Tests ============

func TestGetClientIP(t *testing.T) {
	s := newTestServer()

	// X-Forwarded-For header
	r := httptest.NewRequest("GET", "/test", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	ip := s.getClientIP(r)
	if ip != "1.2.3.4" {
		t.Errorf("X-Forwarded-For: got %q, want %q", ip, "1.2.3.4")
	}

	// X-Forwarded-For single IP
	r = httptest.NewRequest("GET", "/test", nil)
	r.Header.Set("X-Forwarded-For", "10.0.0.1")
	ip = s.getClientIP(r)
	if ip != "10.0.0.1" {
		t.Errorf("X-Forwarded-For single: got %q, want %q", ip, "10.0.0.1")
	}

	// X-Real-IP header
	r = httptest.NewRequest("GET", "/test", nil)
	r.Header.Set("X-Real-IP", "192.168.1.1")
	ip = s.getClientIP(r)
	if ip != "192.168.1.1" {
		t.Errorf("X-Real-IP: got %q, want %q", ip, "192.168.1.1")
	}

	// RemoteAddr fallback
	r = httptest.NewRequest("GET", "/test", nil)
	r.RemoteAddr = "172.16.0.1:12345"
	ip = s.getClientIP(r)
	if ip != "172.16.0.1:12345" {
		t.Errorf("RemoteAddr: got %q, want %q", ip, "172.16.0.1:12345")
	}

	// X-Forwarded-For takes priority over X-Real-IP
	r = httptest.NewRequest("GET", "/test", nil)
	r.Header.Set("X-Forwarded-For", "1.1.1.1")
	r.Header.Set("X-Real-IP", "2.2.2.2")
	ip = s.getClientIP(r)
	if ip != "1.1.1.1" {
		t.Errorf("Priority: got %q, want %q", ip, "1.1.1.1")
	}
}

// ============ getStatusFromStatusCode Tests ============

func TestGetStatusFromStatusCode(t *testing.T) {
	s := newTestServer()

	tests := []struct {
		code int
		want string
	}{
		{200, "success"},
		{201, "success"},
		{204, "success"},
		{299, "success"},
		{300, "failure"},
		{400, "failure"},
		{404, "failure"},
		{500, "failure"},
	}

	for _, tt := range tests {
		got := s.getStatusFromStatusCode(tt.code)
		if got != tt.want {
			t.Errorf("getStatusFromStatusCode(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

// ============ responseWriter Tests ============

func TestResponseWriterWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusNotFound)
	}
	if !rw.written {
		t.Error("written should be true")
	}

	// Second WriteHeader should be ignored
	rw.WriteHeader(http.StatusOK)
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("statusCode should not change, got %d", rw.statusCode)
	}
}

func TestResponseWriterWriteAutoStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.Write([]byte("hello"))
	if rw.statusCode != http.StatusOK {
		t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusOK)
	}
	if !rw.written {
		t.Error("written should be true after Write")
	}
}

// ============ isOriginAllowed Tests ============

func TestIsOriginAllowedNoConfig(t *testing.T) {
	// Without ALLOWED_ORIGINS set, all origins should be allowed
	r := httptest.NewRequest("GET", "/test", nil)
	r.Header.Set("Origin", "http://evil.com")
	if !isOriginAllowed(r) {
		t.Error("should allow all origins when ALLOWED_ORIGINS is not set")
	}
}

// ============ getEnv Tests ============

func TestGetEnv(t *testing.T) {
	key := "COCKPIT_TEST_ENV_VAR_12345"

	// Not set → default
	if v := getEnv(key, "default"); v != "default" {
		t.Errorf("getEnv unset = %q, want %q", v, "default")
	}

	// Set → value
	t.Setenv(key, "value")
	if v := getEnv(key, "default"); v != "value" {
		t.Errorf("getEnv set = %q, want %q", v, "value")
	}
}

// ============ handleHealth Tests ============

func TestHandleHealth(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	s.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

// ============ toStorageAgent Tests ============

func TestToStorageAgent(t *testing.T) {
	agent := NewAgent("agent-1", nil)
	agent.Hostname = "test-host"
	agent.IP = "10.0.0.1"
	agent.Location = protocol.Location{Region: "us-east", Zone: "us-east-1a"}
	agent.Capabilities = []protocol.Capability{
		{Type: "proxy", Version: "1.0", Endpoint: "0.0.0.0:8080"},
		{Type: "docker", Version: "2.0", Metadata: map[string]interface{}{"runtime": "containerd"}},
	}
	agent.Virtualization = &protocol.VirtualizationInfo{Type: "kvm", Role: "guest"}
	agent.Labels = map[string]interface{}{"env": "production"}

	sa := toStorageAgent(agent)
	if sa.ID != "agent-1" {
		t.Errorf("ID = %q", sa.ID)
	}
	if sa.Hostname != "test-host" {
		t.Errorf("Hostname = %q", sa.Hostname)
	}
	if sa.IP != "10.0.0.1" {
		t.Errorf("IP = %q", sa.IP)
	}
	if sa.Region != "us-east" {
		t.Errorf("Region = %q", sa.Region)
	}
	if sa.Zone != "us-east-1a" {
		t.Errorf("Zone = %q", sa.Zone)
	}
	if sa.Status != "online" {
		t.Errorf("Status = %q", sa.Status)
	}
	if sa.VirtType != "kvm" {
		t.Errorf("VirtType = %q", sa.VirtType)
	}
	if sa.VirtRole != "guest" {
		t.Errorf("VirtRole = %q", sa.VirtRole)
	}
	if len(sa.Capabilities) != 2 {
		t.Fatalf("Capabilities count = %d, want 2", len(sa.Capabilities))
	}
	if sa.Capabilities[0].Type != "proxy" {
		t.Errorf("Capability[0].Type = %q", sa.Capabilities[0].Type)
	}
	if sa.Capabilities[0].Config["endpoint"] != "0.0.0.0:8080" {
		t.Errorf("Capability[0] endpoint = %v", sa.Capabilities[0].Config["endpoint"])
	}
}

func TestToStorageAgentNoVirtualization(t *testing.T) {
	agent := NewAgent("a1", nil)
	sa := toStorageAgent(agent)
	if sa.VirtType != "" {
		t.Errorf("VirtType = %q, want empty", sa.VirtType)
	}
	if sa.VirtRole != "" {
		t.Errorf("VirtRole = %q, want empty", sa.VirtRole)
	}
}
