package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestVirtCapabilitiesForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		return map[string]interface{}{"available": true, "hypervisor": true, "lxc": false}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentVirtAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/virt/capabilities", nil), "a1/virt/capabilities")
	if rec.Code != http.StatusOK || gotMethod != "virt.capabilities" {
		t.Fatalf("capabilities: code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	if got := rec.Body.String(); got == "" || !strings.Contains(got, `"available":true`) {
		t.Fatalf("body = %s", got)
	}
}

func TestVirtRouting(t *testing.T) {
	s := newBackupTestServer(t)
	dispatched := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		dispatched++
		return map[string]interface{}{}, ""
	})

	cases := []struct {
		name     string
		method   string
		url      string
		rest     string
		wantCode int
	}{
		{"POST rejected", http.MethodPost, "/api/agents/a1/virt/capabilities", "a1/virt/capabilities", http.StatusMethodNotAllowed},
		{"unknown action", http.MethodGet, "/api/agents/a1/virt/other", "a1/virt/other", http.StatusNotFound},
		{"malformed rest", http.MethodGet, "/api/agents/a1/virt", "a1/virt", http.StatusNotFound},
		{"offline agent", http.MethodGet, "/api/agents/ghost/virt/capabilities", "ghost/virt/capabilities", http.StatusServiceUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.handleAgentVirtAPI(rec, httptest.NewRequest(c.method, c.url, nil), c.rest)
			if rec.Code != c.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, c.wantCode)
			}
		})
	}
	if dispatched != 0 {
		t.Fatalf("dispatched = %d, want 0", dispatched)
	}
}

func TestVirtEmptyAction(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentVirtAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/virt/", nil), "a1/virt/")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestVirtTopLevelRouteDispatch(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"available": true}, ""
	})

	rec := httptest.NewRecorder()
	s.serveAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/virt/capabilities", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestVirtCallAgentError(t *testing.T) {
	s := newBackupTestServer(t)
	// 注册后立即关闭：CallAgent 经 SendWithTimeout 报传输错误 → 502
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{{Type: "backup"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	agent.Close()

	rec := httptest.NewRecorder()
	s.handleAgentVirtAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/virt/capabilities", nil), "a1/virt/capabilities")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

// withVirtRawResponder 注册一个按自定义 payload 应答 RPC 的假 agent，
// 用于构造合法 helper 覆盖不到的坏帧 / error 帧分支。
func withVirtRawResponder(t *testing.T, s *Server, agentID string, payload map[string]interface{}) {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Capabilities = []protocol.Capability{{Type: "backup"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	go func() {
		for reqMsg := range agent.Send {
			if reqMsg.Type != protocol.MessageTypeRPCRequest {
				continue
			}
			resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, payload)
			resp.ID = reqMsg.ID
			s.handleRPCResponse(resp)
		}
	}()
	t.Cleanup(func() { agent.Close() })
}

func TestVirtBadRPCPayload(t *testing.T) {
	s := newBackupTestServer(t)
	// status 非字符串：DecodeRPCResponse 必然失败 → 502 invalid response
	withVirtRawResponder(t, s, "a1", map[string]interface{}{"status": 123})

	rec := httptest.NewRecorder()
	s.handleAgentVirtAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/virt/capabilities", nil), "a1/virt/capabilities")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestVirtRPCErrorWithMessage(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return nil, "operation rejected by agent"
	})

	rec := httptest.NewRecorder()
	s.handleAgentVirtAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/virt/capabilities", nil), "a1/virt/capabilities")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if got := rec.Body.String(); !strings.Contains(got, "operation rejected by agent") {
		t.Fatalf("body = %s", got)
	}
}

func TestVirtRPCErrorWithoutMessage(t *testing.T) {
	s := newBackupTestServer(t)
	// status=error 且缺 error 字段：走默认文案兜底
	withVirtRawResponder(t, s, "a1", map[string]interface{}{"status": "error"})

	rec := httptest.NewRecorder()
	s.handleAgentVirtAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/virt/capabilities", nil), "a1/virt/capabilities")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if got := rec.Body.String(); !strings.Contains(got, "agent rejected the operation") {
		t.Fatalf("body = %s", got)
	}
}
