package liveness

import (
	"context"
	"testing"
	"time"
)

// TestSchedulerRunTicks 手控 tick 驱动：每 tick 一轮全目标探测，报告按目标
// 顺序回调；ctx 取消后 Run 返回。
func TestSchedulerRunTicks(t *testing.T) {
	ticks := make(chan time.Time, 2)
	var cycles [][]Report
	s := &Scheduler{
		Targets:  []Target{{IP: "10.0.0.5", Ports: []int{22}}, {IP: "10.0.0.6"}},
		Interval: 99 * time.Second,
		Timeout:  time.Second,
		Workers:  1,
		Ticks:    ticks,
		Prober: func(ctx context.Context, ip string, opts Options) Report {
			return Report{IP: ip, At: 1}
		},
		OnCycle: func(reports []Report) {
			cp := make([]Report, len(reports))
			copy(cp, reports)
			cycles = append(cycles, cp)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	ticks <- time.Now()
	ticks <- time.Now()
	time.Sleep(100 * time.Millisecond) // stub 无阻塞，等两轮回调落定
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run must return on ctx cancel")
	}
	if len(cycles) != 2 {
		t.Fatalf("cycles = %d, want 2", len(cycles))
	}
	for _, cycle := range cycles {
		if len(cycle) != 2 || cycle[0].IP != "10.0.0.5" || cycle[1].IP != "10.0.0.6" {
			t.Fatalf("cycle order = %+v", cycle)
		}
	}
}

// TestSchedulerRunEmptyTargets 空目标清单：tick 到达也不回调，取消后退出。
func TestSchedulerRunEmptyTargets(t *testing.T) {
	ticks := make(chan time.Time, 1)
	called := 0
	s := &Scheduler{Ticks: ticks, OnCycle: func([]Report) { called++ }}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	ticks <- time.Now()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run must return on ctx cancel")
	}
	if called != 0 {
		t.Fatalf("OnCycle called %d times for empty targets", called)
	}
}

// TestSchedulerRunCycle 单周期：结果按目标顺序；空清单不回调、返回 nil。
func TestSchedulerRunCycle(t *testing.T) {
	s := &Scheduler{
		Targets: []Target{{IP: "10.0.0.5"}, {IP: "10.0.0.6"}, {IP: "10.0.0.7"}},
		Workers: 2,
		Prober: func(ctx context.Context, ip string, opts Options) Report {
			return Report{IP: ip}
		},
	}
	reports := s.runCycle(context.Background())
	if len(reports) != 3 || reports[0].IP != "10.0.0.5" || reports[2].IP != "10.0.0.7" {
		t.Fatalf("reports = %+v", reports)
	}

	called := false
	empty := &Scheduler{OnCycle: func([]Report) { called = true }}
	if got := empty.runCycle(context.Background()); got != nil || called {
		t.Fatalf("empty cycle = %v, called=%v", got, called)
	}
}

// TestSchedulerRunCycleCancelledProbe 目标探测阻塞时由 ctx 取消退出（周期
// 不被单个目标挂死），并以部分结果回调。
func TestSchedulerRunCycleCancelledProbe(t *testing.T) {
	started := make(chan struct{})
	called := false
	s := &Scheduler{
		Targets: []Target{{IP: "10.0.0.5"}},
		Prober: func(ctx context.Context, ip string, opts Options) Report {
			close(started)
			<-ctx.Done()
			return Report{IP: ip}
		},
		OnCycle: func([]Report) { called = true },
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.runCycle(ctx); close(done) }()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cycle must return after ctx cancel")
	}
	if !called {
		t.Fatal("OnCycle must fire with partial results")
	}
}

// TestSchedulerDefaults 注入点缺省：Prober→Probe（对本机回环跑真实三源，
// 容忍机器环境，只断言 IP 回填与时刻非零），Workers/Timeout 走缺省常量。
func TestSchedulerDefaults(t *testing.T) {
	s := &Scheduler{Targets: []Target{{IP: "127.0.0.1"}}}
	reports := s.runCycle(context.Background())
	if len(reports) != 1 || reports[0].IP != "127.0.0.1" || reports[0].At <= 0 {
		t.Fatalf("default cycle = %+v", reports)
	}
}

// TestSchedulerAccessors 缺省常量与显式值透传。
func TestSchedulerAccessors(t *testing.T) {
	zero := &Scheduler{}
	if zero.interval() != DefaultInterval || zero.workers() != DefaultWorkers || zero.timeout() != DefaultTimeout {
		t.Fatalf("defaults = %v %v %v", zero.interval(), zero.workers(), zero.timeout())
	}
	set := &Scheduler{Interval: 7 * time.Second, Workers: 3, Timeout: 500 * time.Millisecond}
	if set.interval() != 7*time.Second || set.workers() != 3 || set.timeout() != 500*time.Millisecond {
		t.Fatalf("explicit = %v %v %v", set.interval(), set.workers(), set.timeout())
	}
}

// TestSchedulerTickSource Ticks 注入直出（stop 为 no-op）；未注入时建真实
// ticker，stop 可安全调用。
func TestSchedulerTickSource(t *testing.T) {
	src := make(chan time.Time)
	s := &Scheduler{Ticks: src}
	ch, stop := s.tickSource()
	if ch != src {
		t.Fatal("injected ticks must pass through")
	}
	stop()

	real := &Scheduler{Interval: time.Millisecond}
	rch, rstop := real.tickSource()
	if rch == nil {
		t.Fatal("real ticker must provide a channel")
	}
	rstop()
}

// TestSchedulerNoOnCycle 周期回调为 nil 时不 panic，仍返回报告。
func TestSchedulerNoOnCycle(t *testing.T) {
	probed := 0
	s := &Scheduler{
		Targets: []Target{{IP: "10.0.0.5"}},
		Prober: func(ctx context.Context, ip string, opts Options) Report {
			probed++
			return Report{IP: ip}
		},
	}
	reports := s.runCycle(context.Background())
	if len(reports) != 1 || probed != 1 {
		t.Fatalf("reports=%+v probed=%d", reports, probed)
	}
}
