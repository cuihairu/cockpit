package healthprobe

import (
	"errors"
	"testing"
	"time"
)

var base = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

// step 确定性推进：n 次结果（err 为每次的探测错误），时间步进 1s。
func step(t *testing.T, g *Guard, n int, err error) {
	t.Helper()
	for i := 0; i < n; i++ {
		g.Record(err, base.Add(time.Duration(i+1)*time.Second))
	}
}

func events() (*[]Transition, func(Transition)) {
	var got []Transition
	return &got, func(tr Transition) { got = append(got, tr) }
}

// TestGuardUnknownToHealthy 启动冷区：连续成功达恢复阈值才定性 Healthy。
func TestGuardUnknownToHealthy(t *testing.T) {
	g := NewGuard("t", 3, 2, nil)
	g.Record(nil, base)
	if s := g.Snapshot(); s.State != StateUnknown {
		t.Fatalf("one ok = %s, want unknown (below recovery)", s.State)
	}
	g.Record(nil, base.Add(time.Second))
	if s := g.Snapshot(); s.State != StateHealthy {
		t.Fatalf("two oks = %s, want healthy", s.State)
	}
}

// TestNewGuardClampsThresholds 阈值 0/负钳位按 1：首轮失败即定性 Faulty、
// 首轮成功即定性 Healthy（钳位的直接行为证据）。
func TestNewGuardClampsThresholds(t *testing.T) {
	g := NewGuard("t", 0, -1, nil)
	if g.faultThreshold != 1 || g.recoverThreshold != 1 {
		t.Fatalf("thresholds = %d/%d, want clamped to 1/1", g.faultThreshold, g.recoverThreshold)
	}
	g.Record(errors.New("x"), base)
	if s := g.Snapshot(); s.State != StateFaulty || s.Open == nil {
		t.Fatalf("state=%s open=%v, want faulty on first fail (clamp)", s.State, s.Open)
	}
	g.Record(nil, base.Add(time.Second))
	if s := g.Snapshot(); s.State != StateHealthy {
		t.Fatalf("state=%s, want healthy on first ok (clamp)", s.State)
	}
}

// TestGuardFirstFailThresholdOne 阈值 1 首轮失败即 Faulty 且开窗口。
func TestGuardFirstFailThresholdOne(t *testing.T) {
	onEv, cb := events()
	g := NewGuard("t", 1, 1, cb)
	g.Record(errors.New("refused"), base)

	s := g.Snapshot()
	if s.State != StateFaulty || s.Open == nil {
		t.Fatalf("state=%s open=%v, want faulty with open window", s.State, s.Open)
	}
	if !s.Open.StartedAt.Equal(base) || !s.Open.EndedAt.IsZero() {
		t.Fatalf("open window = %+v, want startedAt=base endedAt=zero", s.Open)
	}
	if len(*onEv) != 1 || (*onEv)[0].From != StateUnknown || (*onEv)[0].To != StateFaulty {
		t.Fatalf("events = %+v", *onEv)
	}
}

// TestGuardDebounce 防抖主路径：阈值内失败不定性；跨相反结果清零重计。
func TestGuardDebounce(t *testing.T) {
	onEv, cb := events()
	g := NewGuard("t", 3, 2, cb)

	boom := errors.New("boom")
	step(t, g, 2, boom)
	if g.Snapshot().State != StateUnknown {
		t.Fatal("2 fails below threshold=3 must stay unknown")
	}
	g.Record(nil, base.Add(3*time.Second)) // 相异结果清零
	step(t, g, 2, boom)
	if g.Snapshot().State != StateUnknown {
		t.Fatal("counter must reset on opposite result")
	}
	step(t, g, 1, boom)
	s := g.Snapshot()
	if s.State != StateFaulty {
		t.Fatal("3 consecutive fails must trip faulty")
	}
	if len(*onEv) != 1 {
		t.Fatalf("events = %+v, want single transition", *onEv)
	}
}

// TestGuardFaultToHealthy 恢复防抖：单次成功不关窗，达恢复阈值才关窗
// 并发事件；窗口 EndedAt 落在恢复时刻。
func TestGuardFaultToHealthy(t *testing.T) {
	onEv, cb := events()
	g := NewGuard("t", 1, 2, cb)
	g.Record(errors.New("down"), base) // → Faulty

	g.Record(nil, base.Add(time.Second))
	if s := g.Snapshot(); s.State != StateFaulty || s.Open == nil {
		t.Fatal("one ok below recovery=2 must stay faulty with open window")
	}
	g.Record(nil, base.Add(2*time.Second))

	s := g.Snapshot()
	if s.State != StateHealthy || s.Open != nil {
		t.Fatalf("state=%s open=%v, want healthy closed-window", s.State, s.Open)
	}
	if len(s.Recent) != 1 || !s.Recent[0].EndedAt.Equal(base.Add(2*time.Second)) {
		t.Fatalf("recent = %+v, want window closed at +2s", s.Recent)
	}
	if s.LastError != "down" {
		t.Fatalf("lastError = %q, want preserved", s.LastError)
	}
	if len(*onEv) != 2 || (*onEv)[1].To != StateHealthy {
		t.Fatalf("events = %+v, want faulty then healthy", *onEv)
	}
}

// TestGuardFaultyUpdatesLastError 故障持续期再失败：只刷新进行中窗口的
// last_error，不重复发事件、不新开窗口。
func TestGuardFaultyUpdatesLastError(t *testing.T) {
	onEv, cb := events()
	g := NewGuard("t", 1, 1, cb)
	g.Record(errors.New("down"), base)
	g.Record(errors.New("still down"), base.Add(time.Second))

	s := g.Snapshot()
	if len(*onEv) != 1 {
		t.Fatalf("events = %+v, want no duplicate", *onEv)
	}
	if s.Open.LastError != "still down" || len(s.Recent) != 0 {
		t.Fatalf("open=%+v recent=%v, want updated open only", s.Open, s.Recent)
	}
}

// TestGuardFailsAcrossTransition 连败计数跨迁移累计：故障持续期继续涨
// （上行读数要真实连败次数），任一次成功即清零。
func TestGuardFailsAcrossTransition(t *testing.T) {
	g := NewGuard("t", 3, 1, nil)
	step(t, g, 3, errors.New("x")) // 判 Faulty
	if f := g.Snapshot().Fails; f != 3 {
		t.Fatalf("fails at trip = %d, want 3", f)
	}
	step(t, g, 2, errors.New("x"))
	if f := g.Snapshot().Fails; f != 5 {
		t.Fatalf("fails during fault = %d, want 5 (kept counting)", f)
	}
	g.Record(nil, base.Add(99*time.Second))
	if f := g.Snapshot().Fails; f != 0 {
		t.Fatalf("fails after success = %d, want 0", f)
	}
}

// TestGuardWindowRing 环形容量：超限丢最旧，保留最近 windowCapacity 个。
func TestGuardWindowRing(t *testing.T) {
	g := NewGuard("t", 1, 1, nil)
	for i := 0; i < windowCapacity+6; i++ {
		at := base.Add(time.Duration(i) * time.Minute)
		g.Record(errors.New("x"), at)      // 开窗
		g.Record(nil, at.Add(time.Second)) // 关窗
	}
	if got := len(g.Snapshot().Recent); got != windowCapacity {
		t.Fatalf("recent len = %d, want %d", got, windowCapacity)
	}
}

// TestGuardSnapshotDefensive Snapshot 防御性拷贝：外部改动不回灌内部。
func TestGuardSnapshotDefensive(t *testing.T) {
	g := NewGuard("t", 1, 1, nil)
	g.Record(errors.New("x"), base)
	g.Record(nil, base.Add(time.Second))

	s := g.Snapshot()
	s.Recent[0].LastError = "tampered"
	s.Open = &Window{Target: "tampered"}
	if again := g.Snapshot(); again.Recent[0].LastError == "tampered" || again.Open != nil {
		t.Fatalf("snapshot not defensive: %+v", again)
	}
}

// TestNewGuardDelegatesLiveness NewGuard 即 Liveness 语义：快照与开窗都带
// liveness 标记（判「受监控系统活没活」的默认口径）。
func TestNewGuardDelegatesLiveness(t *testing.T) {
	g := NewGuard("t", 1, 1, nil)
	g.Record(errors.New("x"), base)
	g.Record(nil, base.Add(time.Second))

	s := g.Snapshot()
	if s.Semantics != SemLiveness {
		t.Fatalf("semantics = %q, want liveness", s.Semantics)
	}
	if len(s.Recent) != 1 || s.Recent[0].Semantics != SemLiveness {
		t.Fatalf("recent = %+v, want window tagged liveness", s.Recent)
	}
	if s.LastOK.IsZero() || !s.LastOK.Equal(base.Add(time.Second)) {
		t.Fatalf("lastOK = %v, want %v", s.LastOK, base.Add(time.Second))
	}
}

// TestGuardReadinessTagged Readiness 与 Liveness 判定一致，区别只在窗口
// 留痕的语义标记（后果归调用方的口径证据）。
func TestGuardReadinessTagged(t *testing.T) {
	g := NewGuardSem("t", SemReadiness, 2, 1, nil)
	g.Record(errors.New("dep down"), base)
	g.Record(errors.New("dep down"), base.Add(time.Second))
	g.Record(nil, base.Add(2*time.Second))

	s := g.Snapshot()
	if s.State != StateHealthy || len(s.Recent) != 1 {
		t.Fatalf("state=%s recent=%d, want healthy with one closed window", s.State, len(s.Recent))
	}
	if s.Recent[0].Semantics != SemReadiness || s.Open != nil {
		t.Fatalf("window = %+v, want readiness-tagged closed window", s.Recent[0])
	}
}

// TestGuardHeartbeatStampsOnly 心跳语义只打点：连续失败不迁移不开窗不发
// 事件，成功只刷 LastOK（「最近活体时刻」），失败只刷 LastError。
func TestGuardHeartbeatStampsOnly(t *testing.T) {
	onEv, cb := events()
	g := NewGuardSem("t", SemHeartbeat, 1, 1, cb)

	g.Record(errors.New("timeout"), base)
	s := g.Snapshot()
	if s.State != StateUnknown || s.Open != nil || !s.LastOK.IsZero() {
		t.Fatalf("state=%s open=%v lastOK=%v, want untouched by failure", s.State, s.Open, s.LastOK)
	}
	if s.LastError != "timeout" || !s.LastChecked.Equal(base) {
		t.Fatalf("lastError=%q lastChecked=%v, want failure reason and check stamp", s.LastError, s.LastChecked)
	}

	g.Record(nil, base.Add(time.Second))
	s = g.Snapshot()
	if s.State != StateUnknown || !s.LastOK.Equal(base.Add(time.Second)) {
		t.Fatalf("state=%s lastOK=%v, want heartbeat stays unknown with lastOK stamped", s.State, s.LastOK)
	}

	step(t, g, 5, errors.New("down"))
	s = g.Snapshot()
	if s.State != StateUnknown || s.Open != nil || len(s.Recent) != 0 || len(*onEv) != 0 {
		t.Fatalf("state=%s open=%v recent=%d events=%d, want no judgment at all",
			s.State, s.Open, len(s.Recent), len(*onEv))
	}
	if s.LastError != "down" {
		t.Fatalf("lastError = %q, want latest failure reason", s.LastError)
	}
	if !s.LastOK.Equal(base.Add(time.Second)) {
		t.Fatalf("lastOK = %v, want preserved across later failures", s.LastOK)
	}
}
