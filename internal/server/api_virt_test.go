package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
