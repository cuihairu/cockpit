package probeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/core/healthprobe"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// TestWriteStatusFileBadPath 目标目录不存在 → tmp 写失败错误分支。
func TestWriteStatusFileBadPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "no-such-dir", "status.json")
	if err := WriteStatusFile(p, []healthprobe.Snapshot{{Target: "x"}}); err == nil {
		t.Fatal("want error for missing parent dir")
	}
}

// TestAlerterFuncAdapter 函数适配器直通。
func TestAlerterFuncAdapter(t *testing.T) {
	var got int
	AlerterFunc(func(healthprobe.Transition) { got++ }).Alert(healthprobe.Transition{})
	if got != 1 {
		t.Fatalf("got = %d, want 1", got)
	}
}

// flipServer http 桩：flip 控制 200/503。
func flipServer(t *testing.T, flip *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(flip) == 1 {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(503)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// payloadOf 取消息负载。
func payloadOf(t *testing.T, m *protocol.Message) map[string]interface{} {
	t.Helper()
	if m.Type != protocol.MessageTypeProbeReport {
		t.Fatalf("type = %s, want probe_report", m.Type)
	}
	b, err := json.Marshal(m.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]interface{}
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// targetsOf / windowsOf 取负载字段。
func targetsOf(t *testing.T, p map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := p["targets"].([]interface{})
	if !ok {
		t.Fatalf("targets = %#v", p["targets"])
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, it := range raw {
		out = append(out, it.(map[string]interface{}))
	}
	return out
}

func windowsOf(t *testing.T, p map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := p["windows"].([]interface{})
	if !ok {
		t.Fatalf("windows = %#v", p["windows"])
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, it := range raw {
		out = append(out, it.(map[string]interface{}))
	}
	return out
}

// TestUpstreamMessageFullCycle 全链：冷区零时刻 → 故障开窗（ended_at=0）
// → 恢复关窗（ended_at>0）随报可回查。
func TestUpstreamMessageFullCycle(t *testing.T) {
	var up int32 = 1
	srv := flipServer(t, &up)

	a, err := New(&Config{Targets: []TargetConfig{{
		Name: "blog", Type: "http", URL: srv.URL, ExpectStatus: 200,
		Interval: Duration(time.Hour), Timeout: Duration(time.Second),
		Threshold: 1, Recovery: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}

	// 冷区：未探测时刻面为零
	m := a.UpstreamMessage()
	p := payloadOf(t, m)
	ts := targetsOf(t, p)
	if len(ts) != 1 || ts[0]["state"] != "unknown" || ts[0]["since"].(float64) != 0 {
		t.Fatalf("cold targets = %+v", ts)
	}

	// 定性 healthy：since/last_checked 均非零
	a.ProbeAll(context.Background())
	ts = targetsOf(t, payloadOf(t, a.UpstreamMessage()))
	if ts[0]["state"] != "healthy" || ts[0]["since"].(float64) == 0 || ts[0]["last_checked"].(float64) == 0 {
		t.Fatalf("healthy targets = %+v", ts)
	}
	if ws := windowsOf(t, payloadOf(t, a.UpstreamMessage())); len(ws) != 0 {
		t.Fatalf("healthy windows = %+v, want empty", ws)
	}

	// 故障：threshold=1 一轮即 Faulty，开窗（ended_at=0）
	atomic.StoreInt32(&up, 0)
	a.ProbeAll(context.Background())
	p = payloadOf(t, a.UpstreamMessage())
	if ts = targetsOf(t, p); ts[0]["state"] != "faulty" || ts[0]["last_error"] != "HTTP 503 (expect 200)" {
		t.Fatalf("faulty targets = %+v", ts)
	}
	ws := windowsOf(t, p)
	if len(ws) != 1 || ws[0]["target"] != "blog" || ws[0]["ended_at"].(float64) != 0 {
		t.Fatalf("open windows = %+v", ws)
	}
	if ws[0]["started_at"].(float64) == 0 {
		t.Fatalf("open window started_at = 0: %+v", ws[0])
	}

	// 恢复：关窗入 Recent，随报可回查（ended_at>0）
	atomic.StoreInt32(&up, 1)
	a.ProbeAll(context.Background())
	p = payloadOf(t, a.UpstreamMessage())
	if ts = targetsOf(t, p); ts[0]["state"] != "healthy" {
		t.Fatalf("recovered targets = %+v", ts)
	}
	ws = windowsOf(t, p)
	if len(ws) != 1 || ws[0]["ended_at"].(float64) == 0 {
		t.Fatalf("closed windows = %+v", ws)
	}
}

// TestUpstreamMessageRecentCap 已关窗口随报条数有界（upstreamRecentWindows，
// 保最新）——本地环保 64，上行只带近 8 条。
func TestUpstreamMessageRecentCap(t *testing.T) {
	var up int32
	srv := flipServer(t, &up)

	a, err := New(&Config{Targets: []TargetConfig{{
		Name: "blog", Type: "http", URL: srv.URL, ExpectStatus: 200,
		Interval: Duration(time.Hour), Timeout: Duration(time.Second),
		Threshold: 1, Recovery: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}

	// 20 轮翻转 → 10 条已关窗口；上行只带最新 8 条
	for i := 0; i < 20; i++ {
		atomic.StoreInt32(&up, int32(i%2))
		a.ProbeAll(context.Background())
	}
	ws := windowsOf(t, payloadOf(t, a.UpstreamMessage()))
	if len(ws) != upstreamRecentWindows {
		t.Fatalf("windows = %d, want capped to %d", len(ws), upstreamRecentWindows)
	}
	if ws[len(ws)-1]["ended_at"].(float64) == 0 {
		t.Fatalf("last window = %+v, want newest closed", ws[len(ws)-1])
	}
}
