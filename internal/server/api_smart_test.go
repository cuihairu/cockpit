package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSmartStatusForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		return map[string]interface{}{
			"available": true,
			"devices": []interface{}{
				map[string]interface{}{"name": "/dev/sda", "health": "passed", "temperatureC": 34},
				map[string]interface{}{"name": "/dev/sdb", "health": "failed"},
			},
		}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentSmartAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/smart/status", nil), "a1/smart/status")
	if rec.Code != http.StatusOK || gotMethod != "hardware-monitor.status" {
		t.Fatalf("status: code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	var resp struct {
		Available bool `json:"available"`
		Devices   []struct {
			Name   string `json:"name"`
			Health string `json:"health"`
		} `json:"devices"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.Available || len(resp.Devices) != 2 || resp.Devices[0].Name != "/dev/sda" || resp.Devices[1].Health != "failed" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestSmartRouting(t *testing.T) {
	s := newBackupTestServer(t)
	dispatched := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		dispatched++
		return map[string]interface{}{}, ""
	})

	// 非 GET 拒绝
	rec := httptest.NewRecorder()
	s.handleAgentSmartAPI(rec, httptest.NewRequest(http.MethodPost, "/api/agents/a1/smart/status", nil), "a1/smart/status")
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST status: code = %d, want 404", rec.Code)
	}
	// 未知子路径
	rec = httptest.NewRecorder()
	s.handleAgentSmartAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/smart/other", nil), "a1/smart/other")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown sub: code = %d, want 404", rec.Code)
	}
	// 畸形 rest
	rec = httptest.NewRecorder()
	s.handleAgentSmartAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/smart", nil), "a1/smart")
	if rec.Code != http.StatusNotFound {
		t.Errorf("malformed rest: code = %d, want 404", rec.Code)
	}
	// agent 离线
	rec = httptest.NewRecorder()
	s.handleAgentSmartAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/ghost/smart/status", nil), "ghost/smart/status")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("offline agent: code = %d, want 503", rec.Code)
	}
	if dispatched != 0 {
		t.Errorf("dispatched = %d, want 0", dispatched)
	}
}

func TestSmartAgentErrorPassthrough(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "smartctl not found"
	})
	rec := httptest.NewRecorder()
	s.handleAgentSmartAPI(rec, httptest.NewRequest(http.MethodGet, "/api/agents/a1/smart/status", nil), "a1/smart/status")
	if rec.Code != http.StatusBadGateway || rec.Body.String() == "" {
		t.Fatalf("error passthrough: code=%d body=%s", rec.Code, rec.Body.String())
	}
}
