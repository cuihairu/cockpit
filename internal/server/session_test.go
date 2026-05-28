package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// ============ Route Registration Tests ============

func TestRegisterDesktopAPIRoutes(t *testing.T) {
	s := newTestServerWithDB(t)
	mux := http.NewServeMux()
	s.registerDesktopAPI(mux)

	req := httptest.NewRequest("GET", "/api/remote/desktop", nil)
	_, pattern := mux.Handler(req)
	if pattern != "/api/remote/desktop" {
		t.Errorf("desktop route not registered, got %q", pattern)
	}
}

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

func TestRegisterVNCAPIRoutes(t *testing.T) {
	s := newTestServerWithDB(t)
	mux := http.NewServeMux()
	s.registerVNCAPI(mux)

	req := httptest.NewRequest("GET", "/api/remote/vnc", nil)
	_, pattern := mux.Handler(req)
	if pattern != "/api/remote/vnc" {
		t.Errorf("vnc route not registered, got %q", pattern)
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

// ============ Desktop Session Tests ============

func TestHandleDesktopDataNoSession(t *testing.T) {
	s := newTestServerWithDB(t)
	msg := protocol.NewMessage(protocol.MessageTypeDesktopData, map[string]interface{}{
		"sessionId": "nonexistent",
	})
	s.HandleDesktopData(msg)
}

func TestHandleDesktopDataNoSessionID(t *testing.T) {
	s := newTestServerWithDB(t)
	msg := protocol.NewMessage(protocol.MessageTypeDesktopData, map[string]interface{}{})
	s.HandleDesktopData(msg)
}

func TestHandleDesktopCloseNoSession(t *testing.T) {
	s := newTestServerWithDB(t)
	msg := protocol.NewMessage(protocol.MessageTypeDesktopClose, map[string]interface{}{
		"sessionId": "nonexistent",
	})
	s.HandleDesktopClose(msg)
}

func TestSendDesktopCloseToAgentNoAgent(t *testing.T) {
	s := newTestServerWithDB(t)
	session := &DesktopSession{
		ID:      "test-session",
		AgentID: "nonexistent-agent",
	}
	s.sendDesktopCloseToAgent(session)
}

// ============ VNC Session Tests ============

func TestHandleVNCDataNoSession(t *testing.T) {
	s := newTestServerWithDB(t)
	err := s.HandleVNCData("nonexistent-conn", []byte("data"))
	if err != nil {
		t.Errorf("HandleVNCData() with no session should return nil, got %v", err)
	}
}

func TestHandleVNCCloseNoSession(t *testing.T) {
	s := newTestServerWithDB(t)
	s.HandleVNCClose("nonexistent-conn", "test reason")
}
