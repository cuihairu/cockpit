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

// TestAgentTCPCheckerAndNilAlerter tcp 分支（checkerFor case "tcp"）+
// SetAlerter(nil) 兜底：必败 tcp（threshold=1）一轮即定性 Faulty，
// nil 出口回落 NopAlerter 迁移照常入环不 panic。
func TestAgentTCPCheckerAndNilAlerter(t *testing.T) {
	a, err := New(&Config{Targets: []TargetConfig{{
		Name: "t", Type: "tcp", Addr: "127.0.0.1:1",
		Interval: Duration(20 * time.Millisecond), Timeout: Duration(500 * time.Millisecond),
		Threshold: 1, Recovery: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	a.SetAlerter(nil)
	a.ProbeAll(context.Background())

	snaps := a.Snapshots()
	if len(snaps) != 1 || snaps[0].State != healthprobe.StateFaulty || snaps[0].Open == nil {
		t.Fatalf("snaps = %+v, want faulty with open window", snaps)
	}
	if trs := a.DrainTransitions(); len(trs) != 1 || trs[0].To != healthprobe.StateFaulty {
		t.Fatalf("trs = %+v, want single faulty transition", trs)
	}
}

// TestTransitionRingTrim 事件环容量裁剪：翻转 70 轮（70 条迁移 > 64）后
// 环内恒为容量上限且保留最新（队尾与当前快照状态一致）。
func TestTransitionRingTrim(t *testing.T) {
	var healthy int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&healthy) == 1 {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(503)
	}))
	defer srv.Close()

	a, err := New(&Config{Targets: []TargetConfig{{
		Name: "h", Type: "http", URL: srv.URL, ExpectStatus: 200,
		Interval: Duration(time.Hour), Timeout: Duration(time.Second),
		Threshold: 1, Recovery: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}

	atomic.StoreInt32(&healthy, 1)
	a.ProbeAll(context.Background()) // 定性 healthy
	for i := 0; i < 70; i++ {
		atomic.StoreInt32(&healthy, int32(i%2))
		a.ProbeAll(context.Background()) // 每轮翻转产生 1 条迁移
	}

	trs := a.DrainTransitions()
	if len(trs) != transitionRing {
		t.Fatalf("transitions = %d, want trimmed to %d", len(trs), transitionRing)
	}
	want := healthprobe.StateHealthy
	if atomic.LoadInt32(&healthy) == 0 {
		want = healthprobe.StateFaulty
	}
	if trs[len(trs)-1].To != want {
		t.Fatalf("last transition to %s, want %s (newest kept)", trs[len(trs)-1].To, want)
	}
}

// TestProbeShutdownCancelSkipsRecord 停机取消的在途探测不计服务故障
// （probe ctx.Err() 短路分支）：请求挂住直至 agent 取消，Guard 应保持
// 未记录的 Unknown 且零迁移。
func TestProbeShutdownCancelSkipsRecord(t *testing.T) {
	entered := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-r.Context().Done() // 挂住直至 agent 侧取消传播进来
	}))
	defer srv.Close()

	a, err := New(&Config{Targets: []TargetConfig{{
		Name: "h", Type: "http", URL: srv.URL, ExpectStatus: 200,
		Interval: Duration(time.Hour), Timeout: Duration(10 * time.Second),
		Threshold: 1, Recovery: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.Run(ctx); close(done) }()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("probe never reached server")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return on cancel")
	}

	s := a.Snapshots()[0]
	if s.State != healthprobe.StateUnknown || !s.LastChecked.IsZero() {
		t.Fatalf("snapshot = %+v, want unrecorded unknown", s)
	}
	if trs := a.DrainTransitions(); len(trs) != 0 {
		t.Fatalf("transitions = %+v, want none", trs)
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
