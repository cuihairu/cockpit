package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOverlayStatusForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		return map[string]interface{}{
			"tools": []interface{}{
				map[string]interface{}{"tool": "zerotier", "status": "ok", "version": "1.14.2"},
				map[string]interface{}{"tool": "frp", "status": "unavailable"},
			},
		}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/overlay/status", nil), "a1/overlay/status")
	if rec.Code != http.StatusOK || gotMethod != "overlay.status" {
		t.Fatalf("status: code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	var resp struct {
		Tools []struct {
			Tool    string `json:"tool"`
			Status  string `json:"status"`
			Version string `json:"version"`
		} `json:"tools"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Tools) != 2 || resp.Tools[0].Tool != "zerotier" || resp.Tools[1].Status != "unavailable" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestOverlayRouting(t *testing.T) {
	s := newBackupTestServer(t)
	dispatched := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		dispatched++
		return map[string]interface{}{}, ""
	})

	// 非 GET 拒绝
	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodPost, "/api/agents/a1/overlay/status", nil), "a1/overlay/status")
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST status: code = %d, want 404", rec.Code)
	}
	// 未知子路径
	rec = httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/overlay/other", nil), "a1/overlay/other")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown sub: code = %d, want 404", rec.Code)
	}
	// 畸形 rest
	rec = httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/overlay", nil), "a1/overlay")
	if rec.Code != http.StatusNotFound {
		t.Errorf("malformed rest: code = %d, want 404", rec.Code)
	}
	// agent 离线
	rec = httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/ghost/overlay/status", nil), "ghost/overlay/status")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("offline agent: code = %d, want 503", rec.Code)
	}
	if dispatched != 0 {
		t.Errorf("dispatched = %d, want 0", dispatched)
	}
}

func TestOverlayAgentErrorPassthrough(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "agent side boom"
	})
	rec := httptest.NewRecorder()
	s.handleAgentOverlayAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/overlay/status", nil), "a1/overlay/status")
	if rec.Code != http.StatusBadGateway {
		t.Errorf("code = %d, want 502", rec.Code)
	}
	if body := rec.Body.String(); body == "" || (json.Unmarshal([]byte(body), &map[string]interface{}{}) != nil && true) {
		t.Errorf("body should be json error, got %q", body)
	}
}
