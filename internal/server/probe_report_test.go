package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// probeReportFixture 组一份 probe_report 负载（两目标 + 一窗口；pg 为
// 冷区目标 since/last_checked=0，覆盖零时刻换算）。
func probeReportFixture(startedAt int64) map[string]interface{} {
	return map[string]interface{}{
		"targets": []map[string]interface{}{
			{"name": "blog", "state": "faulty", "since": startedAt, "last_checked": startedAt + 30, "last_error": "conn refused"},
			{"name": "pg", "state": "unknown", "since": int64(0), "last_checked": int64(0)},
		},
		"windows": []map[string]interface{}{
			{"target": "blog", "started_at": startedAt, "ended_at": int64(0), "last_error": "conn refused"},
		},
	}
}

// TestHandleProbeReportUpserts 全链：负载 → 快照/窗口落库 → 关窗重报刷新
// 同一行（幂等）。
func TestHandleProbeReportUpserts(t *testing.T) {
	s := &Server{db: testServerDB(t)}
	agent := NewAgent("probe-x", nil)

	startedAt := time.Now().Add(-time.Hour).Unix()
	msg := &protocol.Message{Type: protocol.MessageTypeProbeReport, Payload: probeReportFixture(startedAt)}
	s.handleProbeReport(agent, msg)

	targets, err := s.db.ListProbeTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(targets))
	}
	for _, r := range targets {
		if r.AgentID != "probe-x" {
			t.Fatalf("agent = %s, want probe-x", r.AgentID)
		}
		if r.Target == "blog" && (r.State != "faulty" || r.LastError != "conn refused") {
			t.Fatalf("blog = %+v", r)
		}
		if r.Target == "pg" && !r.Since.IsZero() {
			t.Fatalf("cold pg since = %v, want zero", r.Since)
		}
	}

	windows, err := s.db.ListProbeWindows("probe-x", "blog", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 1 || windows[0].EndedAt != nil {
		t.Fatalf("windows = %+v, want 1 open", windows)
	}

	// 关窗重报：同 started_at 幂等覆盖
	fixture := probeReportFixture(startedAt)
	closedAt := startedAt + 1200
	fixture["windows"] = []map[string]interface{}{
		{"target": "blog", "started_at": startedAt, "ended_at": closedAt, "last_error": "conn refused"},
	}
	s.handleProbeReport(agent, &protocol.Message{Type: protocol.MessageTypeProbeReport, Payload: fixture})

	windows, err = s.db.ListProbeWindows("probe-x", "blog", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 1 || windows[0].EndedAt == nil || windows[0].EndedAt.Unix() != closedAt {
		t.Fatalf("windows after close = %+v, want 1 closed at %d", windows, closedAt)
	}
}

// TestHandleProbeReportNilDBAndPayload nil-db 守卫与空负载不 panic 不落行。
func TestHandleProbeReportNilDBAndPayload(t *testing.T) {
	s := &Server{}
	agent := NewAgent("probe-x", nil)
	s.handleProbeReport(agent, &protocol.Message{Type: protocol.MessageTypeProbeReport}) // nil db

	s2 := &Server{db: testServerDB(t)}
	s2.handleProbeReport(agent, &protocol.Message{Type: protocol.MessageTypeProbeReport}) // nil payload

	targets, err := s2.db.ListProbeTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Fatalf("targets = %d, want 0", len(targets))
	}
}

// TestHandleProbeReportBadPayload 非法负载类型被拒不落库。
func TestHandleProbeReportBadPayload(t *testing.T) {
	s := &Server{db: testServerDB(t)}
	agent := NewAgent("probe-x", nil)
	s.handleProbeReport(agent, &protocol.Message{
		Type:    protocol.MessageTypeProbeReport,
		Payload: map[string]interface{}{"targets": "not-an-array"},
	})

	targets, err := s.db.ListProbeTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Fatalf("targets = %d, want 0", len(targets))
	}
}

// TestHandleProbeReportDBClosed 落库失败（库已关）走错误分支不 panic。
func TestHandleProbeReportDBClosed(t *testing.T) {
	db := testServerDB(t)
	s := &Server{db: db}
	agent := NewAgent("probe-x", nil)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s.handleProbeReport(agent, &protocol.Message{
		Type:    protocol.MessageTypeProbeReport,
		Payload: probeReportFixture(time.Now().Unix()),
	})
}

// TestProbeTargetsAPI GET /api/probe/targets 全量回查（经 handleProbeAPI
// 路由层：method 分派与 405 同测）。
func TestProbeTargetsAPI(t *testing.T) {
	s := newProbeTestServer(t)
	if err := s.db.UpsertProbeTargets("probe-x", []storage.ProbeTargetSnapshot{
		{Target: "blog", State: "healthy"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/probe/targets", nil)
	rec := httptest.NewRecorder()
	s.handleProbeAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Targets []storage.ProbeTargetSnapshot `json:"targets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Targets) != 1 || resp.Targets[0].Target != "blog" {
		t.Fatalf("resp = %+v", resp)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/probe/targets", nil)
	rec = httptest.NewRecorder()
	s.handleProbeAPI(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post code = %d, want 405", rec.Code)
	}
}

// TestProbeWindowsAPI GET /api/probe/windows 过滤与 limit 校验（经路由层）。
func TestProbeWindowsAPI(t *testing.T) {
	s := newProbeTestServer(t)
	started := time.Now().Add(-2 * time.Hour)
	if err := s.db.UpsertProbeWindows("probe-x", []storage.ProbeWindow{
		{Target: "blog", StartedAt: started, LastError: "boom"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/probe/windows?target=blog&limit=10", nil)
	rec := httptest.NewRecorder()
	s.handleProbeAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Windows []storage.ProbeWindow `json:"windows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Windows) != 1 || resp.Windows[0].Target != "blog" || resp.Windows[0].EndedAt != nil {
		t.Fatalf("resp = %+v", resp)
	}

	for _, q := range []string{"limit=abc", "limit=0", "limit=500"} {
		req := httptest.NewRequest(http.MethodGet, "/api/probe/windows?"+q, nil)
		rec := httptest.NewRecorder()
		s.handleProbeAPI(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s code = %d, want 400", q, rec.Code)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/probe/windows?target=absent", nil)
	rec = httptest.NewRecorder()
	s.handleProbeAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("absent target code = %d, want 200", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Windows) != 0 {
		t.Fatalf("absent target windows = %+v, want empty", resp.Windows)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/probe/windows", nil)
	rec = httptest.NewRecorder()
	s.handleProbeAPI(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post windows code = %d, want 405", rec.Code)
	}
}

// TestProbeAPIListDBError 库关闭时两查询端点 500（列表错误分支）。
func TestProbeAPIListDBError(t *testing.T) {
	db := testServerDB(t)
	s := &Server{db: db}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/probe/targets", nil)
	rec := httptest.NewRecorder()
	s.handleProbeAPI(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("targets code = %d, want 500", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/probe/windows", nil)
	rec = httptest.NewRecorder()
	s.handleProbeAPI(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("windows code = %d, want 500", rec.Code)
	}
}
