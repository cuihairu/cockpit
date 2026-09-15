package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDriftCheckForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		return map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"kind": "nginx", "name": "web", "status": "drifted"},
				map[string]interface{}{"kind": "cron", "name": "cockpit", "status": "ok"},
			},
			"checked_at": float64(1730000000),
		}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentDriftAPI(rec, logsReq(http.MethodPost, "a1", "check", ""), "a1/drift/check")
	if rec.Code != http.StatusOK || gotMethod != "drift.check" {
		t.Fatalf("check: code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	var resp struct {
		Items []struct {
			Kind   string `json:"kind"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"items"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 || resp.Items[0].Status != "drifted" || resp.Items[1].Status != "ok" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestDriftMethodAndRouting(t *testing.T) {
	s := newBackupTestServer(t)
	dispatched := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		dispatched++
		return map[string]interface{}{}, ""
	})

	// GET 不允许
	rec := httptest.NewRecorder()
	s.handleAgentDriftAPI(rec, logsReq(http.MethodGet, "a1", "check", ""), "a1/drift/check")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET check: code = %d, want 405", rec.Code)
	}
	// 未知子路径
	rec = httptest.NewRecorder()
	s.handleAgentDriftAPI(rec, logsReq(http.MethodPost, "a1", "baseline", ""), "a1/drift/baseline")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown sub: code = %d, want 404", rec.Code)
	}
	// 离线 agent 503
	rec = httptest.NewRecorder()
	s.handleAgentDriftAPI(rec, logsReq(http.MethodPost, "ghost", "check", ""), "ghost/drift/check")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("offline: code = %d, want 503", rec.Code)
	}
	if dispatched != 0 {
		t.Errorf("no request should reach agent, dispatched = %d", dispatched)
	}
}

func TestDriftErrorPassthrough(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "baseline unreadable"
	})
	rec := httptest.NewRecorder()
	s.handleAgentDriftAPI(rec, logsReq(http.MethodPost, "a1", "check", ""), "a1/drift/check")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "baseline unreadable") {
		t.Fatalf("code=%d body=%s, want 502 with agent error", rec.Code, rec.Body.String())
	}
}
