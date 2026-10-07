package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ============ Route Registration Tests ============

func TestRegisterRemoteAPIRoutes(t *testing.T) {
	s := newTestServerWithDB(t)
	mux := http.NewServeMux()
	s.registerRemoteAPI(mux)

	req := httptest.NewRequest("GET", "/api/remote/terminal", nil)
	_, pattern := mux.Handler(req)
	if pattern != "/api/remote/terminal" {
		t.Errorf("terminal route not registered, got %q", pattern)
	}
}

func TestRegisterProxyAPIRoutes(t *testing.T) {
	s := newTestServerWithDB(t)
	mux := http.NewServeMux()
	s.registerProxyAPI(mux)

	routes := []string{"/api/proxies", "/api/proxies/status"}
	for _, route := range routes {
		req := httptest.NewRequest("GET", route, nil)
		_, pattern := mux.Handler(req)
		if pattern != route {
			t.Errorf("proxy route %q not registered, got %q", route, pattern)
		}
	}
}

// ============ Printf Test ============

func TestPrintf(t *testing.T) {
	printf("test message: %s", "hello")
	printf("")
}

// ============ Terminal Session Tests ============

func TestHandleTerminalDataNoSession(t *testing.T) {
	s := newTestServerWithDB(t)
	err := s.HandleTerminalData("nonexistent-conn", []byte("data"))
	if err != nil {
		t.Errorf("HandleTerminalData() with no session should return nil, got %v", err)
	}
}

func TestHandleTerminalCloseNoSession(t *testing.T) {
	s := newTestServerWithDB(t)
	s.HandleTerminalClose("nonexistent-conn", "test reason")
}
