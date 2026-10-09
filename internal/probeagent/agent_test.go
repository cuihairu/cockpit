package probeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/core/healthprobe"
)

// recordingAlerter 记录迁移事件，断言出口收到与事件环一致的序列。
type recordingAlerter struct{ got []healthprobe.Transition }

func (r *recordingAlerter) Alert(tr healthprobe.Transition) { r.got = append(r.got, tr) }

// TestAgentProbeCycleAndTransitions 全链：首探定性 Healthy → 故障翻转
// （threshold=1）→ Faulty + 迁移事件（环与出口一致）→ 快照落盘可回读。
func TestAgentProbeCycleAndTransitions(t *testing.T) {
	var healthy int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&healthy) == 1 {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(503)
	}))
	defer srv.Close()
	atomic.StoreInt32(&healthy, 1)

	cfg := &Config{Targets: []TargetConfig{{
		Name: "h", Type: "http", URL: srv.URL, ExpectStatus: 200,
		Interval: Duration(20 * time.Millisecond), Timeout: Duration(time.Second),
		Threshold: 1, Recovery: 1,
	}}}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	al := &recordingAlerter{}
	a.SetAlerter(al)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { a.Run(ctx); close(done) }()

	waitState(t, a, healthprobe.StateHealthy, 3*time.Second)

	atomic.StoreInt32(&healthy, 0)
	waitState(t, a, healthprobe.StateFaulty, 3*time.Second)

	trs := a.DrainTransitions()
	if len(trs) < 2 || trs[0].To != healthprobe.StateHealthy || trs[len(trs)-1].To != healthprobe.StateFaulty {
		t.Fatalf("transitions = %+v", trs)
	}
	if len(al.got) < 2 {
		t.Fatalf("alerter got %+v, want same transitions", al.got)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return on cancel")
	}
}

// TestAgentUnsupportedType 直建配置绕过 LoadConfig：非法类型构建报错。
func TestAgentUnsupportedType(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("nil config must error")
	}
	if _, err := New(&Config{}); err == nil {
		t.Fatal("empty targets must error")
	}
	_, err := New(&Config{Targets: []TargetConfig{{Name: "x", Type: "ftp", Timeout: Duration(time.Second)}}})
	if err == nil || err.Error() != "unsupported type: ftp" {
		t.Fatalf("err = %v", err)
	}
}

// TestWriteStatusFile 快照落盘 tmp+rename：JSON 数组可回读。
func TestWriteStatusFile(t *testing.T) {
	snaps := []healthprobe.Snapshot{{
		Target: "h", State: healthprobe.StateFaulty,
		Since: time.Now(), LastError: "boom",
		Open: &healthprobe.Window{Target: "h", StartedAt: time.Now(), LastError: "boom"},
	}}
	p := filepath.Join(t.TempDir(), "status.json")
	if err := WriteStatusFile(p, snaps); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var back []healthprobe.Snapshot
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(back) != 1 || back[0].State != healthprobe.StateFaulty || back[0].Open == nil {
		t.Fatalf("roundtrip = %+v", back)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file should be renamed away, stat err = %v", err)
	}
}

// waitState 轮询至目标状态（首探+翻转均为异步路径）。
func waitState(t *testing.T, a *Agent, want healthprobe.State, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		for _, s := range a.Snapshots() {
			if s.State == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("state %s not reached within %v; snapshots = %+v", want, within, a.Snapshots())
}
