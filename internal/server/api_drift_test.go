package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/audit"
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

func TestDriftDiffForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	var gotParams map[string]interface{}
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		gotParams = params
		return map[string]interface{}{
			"expected":            "# meta\nserver block a",
			"current":             "# 被改\nserver block EVIL",
			"baseline_updated_at": float64(1730000000),
		}, ""
	})

	body := `{"kind":"nginx","name":"web"}`
	rec := httptest.NewRecorder()
	s.handleAgentDriftAPI(rec, logsReq(http.MethodPost, "a1", "diff", body), "a1/drift/diff")
	if rec.Code != http.StatusOK || gotMethod != "drift.diff" {
		t.Fatalf("code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	if gotParams["kind"] != "nginx" || gotParams["name"] != "web" {
		t.Fatalf("params = %+v", gotParams)
	}
	var resp struct {
		Expected string `json:"expected"`
		Current  string `json:"current"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Expected == "" || resp.Current == "" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestDriftDiffValidation(t *testing.T) {
	s := newBackupTestServer(t)
	dispatched := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		dispatched++
		return map[string]interface{}{}, ""
	})

	cases := []struct {
		desc, body, wantMsg string
		method              string
		wantCode            int
	}{
		{"unknown kind", `{"kind":"docker","name":"x"}`, "unknown kind", http.MethodPost, http.StatusBadRequest},
		{"empty name", `{"kind":"nginx","name":""}`, "name required", http.MethodPost, http.StatusBadRequest},
		{"long name", `{"kind":"nginx","name":"` + strings.Repeat("n", 129) + `"}`, "name too long", http.MethodPost, http.StatusBadRequest},
		{"bad body", `{invalid`, "Invalid request body", http.MethodPost, http.StatusBadRequest},
		{"GET not allowed", `{"kind":"nginx","name":"web"}`, "method not allowed", http.MethodGet, http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		s.handleAgentDriftAPI(rec, logsReq(tc.method, "a1", "diff", tc.body), "a1/drift/diff")
		if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantMsg) {
			t.Errorf("%s: code=%d body=%s, want %d with %q", tc.desc, rec.Code, rec.Body.String(), tc.wantCode, tc.wantMsg)
		}
	}
	if dispatched != 0 {
		t.Errorf("validation failures must not reach agent, dispatched = %d", dispatched)
	}

	// agent 报错透传（无基线 / 旧基线无原文等）
	withFakeBackupAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "no baseline for nginx/ghost (save it from the panel once)"
	})
	rec := httptest.NewRecorder()
	s.handleAgentDriftAPI(rec, logsReq(http.MethodPost, "a2", "diff", `{"kind":"nginx","name":"ghost"}`), "a2/drift/diff")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "no baseline") {
		t.Fatalf("passthrough: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDriftRecordForwardAndAudit(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method != "drift.record" {
			return nil, "unexpected method " + method
		}
		if params["kind"] != "nginx" || params["name"] != "legacy" {
			return nil, "bad params"
		}
		return map[string]interface{}{"recorded": true, "sha256": strings.Repeat("a", 64)}, ""
	})

	body := `{"kind":"nginx","name":"legacy"}`
	rec := httptest.NewRecorder()
	s.handleAgentDriftAPI(rec, logsReq(http.MethodPost, "a1", "record", body), "a1/drift/record")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}

	// 成功登记记审计（resourceID = kind/name，detail 记 agent）
	logs, total, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{
		"action": audit.ActionDriftRecord, "resource": audit.ResourceDriftBaseline,
	})
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("drift_record audit missing: total=%d err=%v", total, err)
	}
	if logs[0].ResourceID != "nginx/legacy" || !strings.Contains(logs[0].Details, "a1") {
		t.Fatalf("audit entry = %+v", logs[0])
	}

	// agent 报错 → 502 且不记审计
	withFakeBackupAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "no baseline for cron/cockpit"
	})
	rec = httptest.NewRecorder()
	s.handleAgentDriftAPI(rec, logsReq(http.MethodPost, "a2", "record", `{"kind":"cron","name":"cockpit"}`), "a2/drift/record")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("passthrough code = %d", rec.Code)
	}
	_, total, err = s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": audit.ActionDriftRecord})
	if err != nil || total != 1 {
		t.Fatalf("failed record should not audit: total=%d err=%v", total, err)
	}

	// 校验失败 400 不达 agent
	rec = httptest.NewRecorder()
	s.handleAgentDriftAPI(rec, logsReq(http.MethodPost, "a1", "record", `{"kind":"docker","name":"x"}`), "a1/drift/record")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown kind") {
		t.Fatalf("validation code = %d body=%s", rec.Code, rec.Body.String())
	}
}
