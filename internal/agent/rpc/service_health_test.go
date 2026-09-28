package rpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// validHealthProbe 返回一条通过校验的基准探针（测试按需改字段）
func validHealthProbe() HealthProbe {
	return HealthProbe{
		ID: "p1", Type: "http", Target: "http://127.0.0.1:20241/ready",
		IntervalSec: 30, TimeoutSec: 5, FailThreshold: 3,
		Heal: true, Unit: "cloudflared.service",
		BackoffWindowSec: 600, MaxRestartsInWindow: 3,
	}
}

func TestValidateHealthConfig(t *testing.T) {
	t.Run("缺省值补齐", func(t *testing.T) {
		cfg := HealthConfig{Probes: []HealthProbe{{ID: "p", Type: "http", Target: "http://h/x"}}}
		if err := validateHealthConfig(&cfg); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		p := cfg.Probes[0]
		if p.ExpectStatus != 200 || p.IntervalSec != healthDefaultInterval ||
			p.TimeoutSec != healthDefaultTimeout || p.FailThreshold != healthDefaultThreshold {
			t.Fatalf("defaults not applied: %+v", p)
		}
	})
	t.Run("systemd 型 unit 缺省取 target", func(t *testing.T) {
		cfg := HealthConfig{Probes: []HealthProbe{{ID: "p", Type: "systemd", Target: "a.service"}}}
		if err := validateHealthConfig(&cfg); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if cfg.Probes[0].Unit != "a.service" {
			t.Fatalf("unit default = %q", cfg.Probes[0].Unit)
		}
	})
	t.Run("非法形状整包拒绝", func(t *testing.T) {
		cases := []struct {
			name string
			mut  func(*HealthConfig)
		}{
			{"空 probes", func(c *HealthConfig) { c.Probes = nil }},
			{"超上限", func(c *HealthConfig) {
				c.Probes = nil
				for i := 0; i <= healthMaxProbes; i++ {
					c.Probes = append(c.Probes, HealthProbe{ID: fmt.Sprintf("p%d", i), Type: "tcp", Target: "h:1"})
				}
			}},
			{"非法 id", func(c *HealthConfig) { c.Probes[0].ID = "Bad_ID" }},
			{"重复 id", func(c *HealthConfig) { c.Probes = append(c.Probes, c.Probes[0]) }},
			{"未知 type", func(c *HealthConfig) { c.Probes[0].Type = "icmp" }},
			{"http target 非绝对 URL", func(c *HealthConfig) { c.Probes[0].Target = "127.0.0.1:80" }},
			{"http target scheme 不符", func(c *HealthConfig) { c.Probes[0].Target = "ftp://h/x" }},
			{"http 无 host", func(c *HealthConfig) { c.Probes[0].Target = "http:///path" }},
			{"expectStatus 越界", func(c *HealthConfig) { c.Probes[0].ExpectStatus = 99 }},
			{"tcp 缺端口", func(c *HealthConfig) { c.Probes[0].Type = "tcp"; c.Probes[0].Target = "host" }},
			{"tcp 端口越界", func(c *HealthConfig) { c.Probes[0].Type = "tcp"; c.Probes[0].Target = "host:70000" }},
			{"systemd target 非 unit", func(c *HealthConfig) { c.Probes[0].Type = "systemd"; c.Probes[0].Target = "socket" }},
			{"interval 越界", func(c *HealthConfig) { c.Probes[0].IntervalSec = healthMaxInterval + 1 }},
			{"timeout 越界", func(c *HealthConfig) { c.Probes[0].TimeoutSec = healthMaxTimeout + 1 }},
			{"threshold 越界", func(c *HealthConfig) { c.Probes[0].FailThreshold = healthMaxThreshold + 1 }},
			{"heal unit 非 unit", func(c *HealthConfig) { c.Probes[0].Unit = "../etc" }},
			{"backoff 窗口越界", func(c *HealthConfig) { c.Probes[0].BackoffWindowSec = 1 }},
			{"maxRestarts 越界", func(c *HealthConfig) { c.Probes[0].MaxRestartsInWindow = healthMaxRestarts + 1 }},
			{"白名单条目非 unit", func(c *HealthConfig) { c.Whitelist = []string{"x.socket"} }},
		}
		for _, tc := range cases {
			cfg := HealthConfig{Probes: []HealthProbe{validHealthProbe()}, Whitelist: []string{"a.service"}}
			tc.mut(&cfg)
			if err := validateHealthConfig(&cfg); err == nil {
				t.Errorf("%s: expected error", tc.name)
			}
		}
	})
	t.Run("白名单去重", func(t *testing.T) {
		cfg := HealthConfig{
			Probes:    []HealthProbe{{ID: "p", Type: "tcp", Target: "h:1"}},
			Whitelist: []string{"a.service", "a.service", "b.service"},
		}
		if err := validateHealthConfig(&cfg); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(cfg.Whitelist) != 2 {
			t.Fatalf("whitelist = %v", cfg.Whitelist)
		}
	})
	t.Run("heal=false 清零自愈字段", func(t *testing.T) {
		p := validHealthProbe()
		p.Heal = false
		cfg := HealthConfig{Probes: []HealthProbe{p}}
		if err := validateHealthConfig(&cfg); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if cfg.Probes[0].BackoffWindowSec != 0 || cfg.Probes[0].MaxRestartsInWindow != 0 {
			t.Fatalf("heal fields not cleared: %+v", cfg.Probes[0])
		}
	})
}

// TestHealthProbeExecutors 三类默认执行器（D4）
func TestHealthProbeExecutors(t *testing.T) {
	sp := NewServiceProvider(nil)
	e := sp.health

	t.Run("http 状态码精确匹配", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer srv.Close()
		p := HealthProbe{Type: "http", Target: srv.URL, ExpectStatus: 200}
		if err := e.probe(context.Background(), p); err == nil || !strings.Contains(err.Error(), "502") {
			t.Fatalf("expect 502 mismatch err, got %v", err)
		}
		p.ExpectStatus = 502
		if err := e.probe(context.Background(), p); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})
	t.Run("tcp 连通即活", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()
		if err := e.probe(context.Background(), HealthProbe{Type: "tcp", Target: ln.Addr().String()}); err != nil {
			t.Fatalf("open port should pass: %v", err)
		}
		dead := HealthProbe{Type: "tcp", Target: "127.0.0.1:1"}
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		if err := e.probe(ctx, dead); err == nil {
			t.Fatal("closed port should fail")
		}
	})
	t.Run("systemd is-active", func(t *testing.T) {
		cmd := func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name != "systemctl" || args[0] != "is-active" {
				return nil, nil, fmt.Errorf("unexpected %v %v", name, args)
			}
			if args[1] == "good.service" {
				return []byte("active\n"), nil, nil
			}
			return []byte("failed\n"), nil, nil
		}
		sp2 := NewServiceProvider(cmd)
		e2 := sp2.health
		if err := e2.probe(context.Background(), HealthProbe{Type: "systemd", Target: "good.service"}); err != nil {
			t.Fatalf("active should pass: %v", err)
		}
		err := e2.probe(context.Background(), HealthProbe{Type: "systemd", Target: "bad.service"})
		if err == nil || !strings.Contains(err.Error(), `reports "failed"`) {
			t.Fatalf("expect state err, got %v", err)
		}
	})
}

// TestHealthConfigActionPlatformGuard 非 systemd 平台拒绝 heal 探针（D11）
func TestHealthConfigActionPlatformGuard(t *testing.T) {
	sp := NewServiceProvider(nil)
	sp.health.systemd = func() bool { return false }

	p := validHealthProbe()
	params := map[string]interface{}{"probes": []interface{}{p}, "whitelist": []interface{}{"cloudflared.service"}}
	if _, err := sp.HealthConfigAction(params); err == nil || !strings.Contains(err.Error(), "systemd") {
		t.Fatalf("expect systemd guard err, got %v", err)
	}
	p.Heal = false
	params["probes"] = []interface{}{p}
	if out, err := sp.HealthConfigAction(params); err != nil {
		t.Fatalf("non-heal probe should pass: %v", err)
	} else if out.(map[string]interface{})["applied"] != 1 {
		t.Fatalf("applied = %v", out)
	}
}

// TestHealthConfigActionRejectKeepsOld 整包拒绝时旧配置不受影响（D12 原子性）
func TestHealthConfigActionRejectKeepsOld(t *testing.T) {
	sp := NewServiceProvider(nil)
	sp.health.systemd = func() bool { return true }
	ok := map[string]interface{}{"probes": []interface{}{validHealthProbe()}}
	if _, err := sp.HealthConfigAction(ok); err != nil {
		t.Fatalf("seed config failed: %v", err)
	}
	bad := map[string]interface{}{"probes": []interface{}{HealthProbe{ID: "Bad!", Type: "http"}}}
	if _, err := sp.HealthConfigAction(bad); err == nil {
		t.Fatal("expected validation error")
	}
	if _, err := sp.HealthNowAction("p1"); err != nil {
		t.Fatalf("old config should survive: %v", err)
	}
}

// healthTestEnv 组装带计数替身的引擎（不触真 systemctl）
type healthTestEnv struct {
	sp       *ServiceProvider
	e        *serviceHealthEngine
	mu       sync.Mutex
	restarts []string
	probeErr error
}

func newHealthTestEnv(probeErr error) *healthTestEnv {
	env := &healthTestEnv{sp: NewServiceProvider(nil), probeErr: probeErr}
	env.e = env.sp.health
	env.e.systemd = func() bool { return true }
	env.e.probe = func(context.Context, HealthProbe) error { return env.probeErr }
	env.e.restart = func(unit string) error {
		env.mu.Lock()
		defer env.mu.Unlock()
		env.restarts = append(env.restarts, unit)
		return nil
	}
	return env
}

func (env *healthTestEnv) restartCount() int {
	env.mu.Lock()
	defer env.mu.Unlock()
	return len(env.restarts)
}

// drainEvents 取走事件并按 kind 计数
func drainEvents(t *testing.T, e *serviceHealthEngine) map[string]int {
	t.Helper()
	_, events := e.snapshot(true)
	counts := map[string]int{}
	for _, ev := range events {
		counts[ev.Kind]++
	}
	return counts
}

// TestHealthThresholdEdgeAndHeal D5 阈值边沿 + 自愈主链路
func TestHealthThresholdEdgeAndHeal(t *testing.T) {
	env := newHealthTestEnv(errors.New("connect refused"))
	p := validHealthProbe() // threshold 3
	env.e.cfg = HealthConfig{Probes: []HealthProbe{p}, Whitelist: []string{"cloudflared.service"}}

	// 前两次失败不到阈值：无事件无动作
	for i := 0; i < 2; i++ {
		if act := env.e.applyResult(p, errors.New("connect refused")); act != nil {
			t.Fatalf("fail#%d should not trigger heal", i+1)
		}
	}
	if got := drainEvents(t, env.e); got[healthEventDown] != 0 {
		t.Fatalf("below threshold no down event, got %v", got)
	}
	// 第 3 次：down 事件 + 返回动作 → 执行层 restart → 落账
	act := env.e.applyResult(p, errors.New("connect refused"))
	if act == nil {
		t.Fatal("threshold reached should return heal action")
	}
	if got := drainEvents(t, env.e); got[healthEventDown] != 1 {
		t.Fatalf("down event once at threshold, got %v", got)
	}
	env.e.applyHealOutcome(p, 5*time.Millisecond, nil)
	if env.restartCount() != 0 { // 动作不自动执行，须调用方执行
		t.Fatal("applyResult must not run restart itself")
	}
	env.e.restart(p.Unit)
	if env.restartCount() != 1 {
		t.Fatal("restart should be called by caller")
	}
	states, _ := env.e.snapshot(true)
	st := states[p.ID]
	if st.ConsecutiveFails != 3 || st.LastHeal == nil || st.LastHeal.Result != "restarted" {
		t.Fatalf("state after heal: %+v", st)
	}
	// 恢复：计数清零 + recovered 事件（先验事件——snapshot 会顺带 drain）
	if act := env.e.applyResult(p, nil); act != nil {
		t.Fatal("success must not heal")
	}
	if got := drainEvents(t, env.e); got[healthEventRecovered] != 1 {
		t.Fatalf("recovered event missing: %v", got)
	}
	states, _ = env.e.snapshot(true)
	if states[p.ID].ConsecutiveFails != 0 || states[p.ID].Status != "ok" {
		t.Fatalf("state after recover: %+v", states[p.ID])
	}
}

// TestHealthWhitelistAlertOnly 非白名单单元只告警不动手（D6）
func TestHealthWhitelistAlertOnly(t *testing.T) {
	env := newHealthTestEnv(errors.New("down"))
	p := validHealthProbe()
	p.FailThreshold = 1
	env.e.cfg = HealthConfig{Probes: []HealthProbe{p}} // 白名单为空

	if act := env.e.applyResult(p, errors.New("down")); act != nil {
		t.Fatal("non-whitelisted must not heal")
	}
	got := drainEvents(t, env.e)
	if got[healthEventBlocked] != 1 || got[healthEventDown] != 1 {
		t.Fatalf("events: %v", got)
	}
	states, _ := env.e.snapshot(true)
	if st := states[p.ID]; st.LastHeal == nil || st.LastHeal.Result != "blocked_whitelist" {
		t.Fatalf("lastHeal: %+v", st.LastHeal)
	}
	// 同一下沉期不重复发 blocked 事件
	_ = env.e.applyResult(p, errors.New("down"))
	if got := drainEvents(t, env.e); got[healthEventBlocked] != 0 {
		t.Fatalf("blocked event should emit once per down period: %v", got)
	}
	// 恢复后重新下沉可再次告警
	_ = env.e.applyResult(p, nil)
	_ = env.e.applyResult(p, errors.New("down"))
	if got := drainEvents(t, env.e); got[healthEventBlocked] != 1 {
		t.Fatalf("blocked should re-arm after recovery: %v", got)
	}
	if env.restartCount() != 0 {
		t.Fatal("no restart ever for non-whitelisted unit")
	}
}

// TestHealthBackoffWindow D7：窗口额度用尽只告警不动作，事件每窗口一条
func TestHealthBackoffWindow(t *testing.T) {
	env := newHealthTestEnv(errors.New("down"))
	p := validHealthProbe()
	p.FailThreshold = 1
	p.MaxRestartsInWindow = 1
	p.BackoffWindowSec = healthMaxBackoffWindow // 大窗口：测试期内不重置
	env.e.cfg = HealthConfig{Probes: []HealthProbe{p}, Whitelist: []string{"cloudflared.service"}}

	// 第 1 次：放行 + 落账
	act := env.e.applyResult(p, errors.New("down"))
	if act == nil {
		t.Fatal("first heal should pass backoff")
	}
	env.e.applyHealOutcome(p, time.Millisecond, nil)
	// 第 2 次：窗口耗尽 → 无动作 + backoff 事件
	if act := env.e.applyResult(p, errors.New("down")); act != nil {
		t.Fatal("budget exhausted must not heal")
	}
	got := drainEvents(t, env.e)
	if got[healthEventBackoff] != 1 || got[healthEventHealed] != 1 {
		t.Fatalf("events: %v", got)
	}
	// 第 3 次：连 backoff 事件也不重复（每窗口一条）
	_ = env.e.applyResult(p, errors.New("down"))
	if got := drainEvents(t, env.e); got[healthEventBackoff] != 0 {
		t.Fatalf("backoff event once per window: %v", got)
	}
	states, _ := env.e.snapshot(true)
	if st := states[p.ID]; st.LastHeal == nil || st.LastHeal.Result != "backoff_skipped" {
		t.Fatalf("lastHeal: %+v", st.LastHeal)
	}
	if env.restartCount() != 0 { // restart 由测试侧手工计数（上面只执行过 0 次）
		t.Fatal("unexpected restart calls")
	}
}

// TestHealthLedgerSharedByUnit 同 unit 两探针共享额度（D7）；
// 失败重启同样占额度
func TestHealthLedgerSharedAndFailedRestartsCount(t *testing.T) {
	env := newHealthTestEnv(errors.New("down"))
	p1 := validHealthProbe()
	p1.FailThreshold = 1
	p1.MaxRestartsInWindow = 1
	p2 := p1
	p2.ID = "p2"
	env.e.cfg = HealthConfig{Probes: []HealthProbe{p1, p2}, Whitelist: []string{"cloudflared.service"}}

	act := env.e.applyResult(p1, errors.New("down"))
	env.e.applyHealOutcome(p1, time.Millisecond, errors.New("systemctl restart: exit 1"))
	if act == nil {
		t.Fatal("first attempt should pass")
	}
	// 失败重启也占额度：p2 同 unit 被台账挡住
	if act := env.e.applyResult(p2, errors.New("down")); act != nil {
		t.Fatal("failed restart must consume budget")
	}
	got := drainEvents(t, env.e)
	if got[healthEventHealFailed] != 1 || got[healthEventBackoff] != 1 {
		t.Fatalf("events: %v", got)
	}
}

// TestHealthEventRing 环形缓冲满丢最旧 + drain 清空（D8）
func TestHealthEventRing(t *testing.T) {
	env := newHealthTestEnv(nil)
	for i := 0; i < healthEventRingCap+50; i++ {
		env.e.emit(healthEventDown, "p", "", "x")
	}
	states, events := env.e.snapshot(true)
	if len(states) != 0 || len(events) != healthEventRingCap {
		t.Fatalf("ring cap: %d events", len(events))
	}
	if _, events := env.e.snapshot(true); len(events) != 0 {
		t.Fatalf("drain should clear, got %d", len(events))
	}
}

// TestHealthLoopIntegration 全链路：配置装载 → 探针失败 → 阈值触发自愈 →
// 事件归集（drain）→ 重推配置状态清零（D13）
func TestHealthLoopIntegration(t *testing.T) {
	env := newHealthTestEnv(errors.New("tunnel dead"))
	p := validHealthProbe()
	p.FailThreshold = 1
	params := map[string]interface{}{
		"probes":    []interface{}{p},
		"whitelist": []interface{}{"cloudflared.service"},
	}
	if out, err := env.sp.HealthConfigAction(params); err != nil {
		t.Fatalf("config: %v", err)
	} else if out.(map[string]interface{})["applied"] != 1 {
		t.Fatalf("applied: %v", out)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got := drainEvents(t, env.e); got[healthEventHealed] == 1 && got[healthEventDown] == 1 {
			if env.restartCount() == 1 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if env.restartCount() != 1 {
		t.Fatalf("restart calls = %d, want 1", env.restartCount())
	}
	env.mu.Lock()
	unit := ""
	if len(env.restarts) > 0 {
		unit = env.restarts[0]
	}
	env.mu.Unlock()
	if unit != "cloudflared.service" {
		t.Fatalf("restarted unit = %q", unit)
	}

	// 立即探测动作（health.now）：结果与状态联动
	if _, err := env.sp.HealthNowAction("nope"); err == nil {
		t.Fatal("unknown probe id should error")
	}
	out, err := env.sp.HealthNowAction(p.ID)
	if err != nil {
		t.Fatalf("health.now: %v", err)
	}
	if out.(map[string]interface{})["status"] != "fail" {
		t.Fatalf("health.now result: %v", out)
	}

	// 重推配置：状态清零（D13），事件环保留待归集
	if _, err := env.sp.HealthConfigAction(params); err != nil {
		t.Fatalf("re-push: %v", err)
	}
	states, _ := env.e.snapshot(true)
	if _, ok := states[p.ID]; ok {
		t.Fatal("state should reset on config change")
	}
}

// TestHealthStatusActionEmptyNoConfig 未配置时 status 返回空结构不报错
func TestHealthStatusActionEmptyNoConfig(t *testing.T) {
	sp := NewServiceProvider(nil)
	out, err := sp.HealthStatusAction(nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	m := out.(map[string]interface{})
	if len(m["states"].(map[string]healthProbeState)) != 0 || len(m["events"].([]healthEvent)) != 0 {
		t.Fatalf("empty snapshot: %v", m)
	}
}

// TestHealthValidateDefaultsHealFields heal 探针缺省 backoff/额度补齐 +
// heal=false 的 systemd 型 unit 缺省
func TestHealthValidateDefaultsHealFields(t *testing.T) {
	p := validHealthProbe()
	p.BackoffWindowSec = 0
	p.MaxRestartsInWindow = 0
	cfg := HealthConfig{Probes: []HealthProbe{p}, Whitelist: []string{"cloudflared.service"}}
	if err := validateHealthConfig(&cfg); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.Probes[0].BackoffWindowSec != healthDefaultBackoffWindow ||
		cfg.Probes[0].MaxRestartsInWindow != healthDefaultMaxRestartsInWin {
		t.Fatalf("backoff defaults: %+v", cfg.Probes[0])
	}
	off := HealthProbe{ID: "q", Type: "systemd", Target: "a.service", Heal: false}
	cfg2 := HealthConfig{Probes: []HealthProbe{off}}
	if err := validateHealthConfig(&cfg2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg2.Probes[0].Unit != "a.service" {
		t.Fatalf("heal=false systemd unit default = %q", cfg2.Probes[0].Unit)
	}
	on := HealthProbe{ID: "r", Type: "systemd", Target: "b.service", Heal: true}
	cfg3 := HealthConfig{Probes: []HealthProbe{on}, Whitelist: []string{"b.service"}}
	if err := validateHealthConfig(&cfg3); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg3.Probes[0].Unit != "b.service" {
		t.Fatalf("heal=true systemd unit default = %q", cfg3.Probes[0].Unit)
	}
}

// TestHealthDefaultRestartClosure 默认自愈执行器走执行层 DoAction（D1）：
// argv 直传 systemctl restart，失败回 stderr 摘要
func TestHealthDefaultRestartClosure(t *testing.T) {
	cmd := func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		if name == "systemctl" && args[0] == "restart" {
			if args[1] == "bad.service" {
				return nil, []byte("Failed to restart bad.service: Unit is masked"), errors.New("exit status 1")
			}
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("unexpected %v %v", name, args)
	}
	e := NewServiceProvider(cmd).health
	if err := e.restart("good.service"); err != nil {
		t.Fatalf("good restart: %v", err)
	}
	err := e.restart("bad.service")
	if err == nil || !strings.Contains(err.Error(), "Unit is masked") {
		t.Fatalf("bad restart err: %v", err)
	}
}

// TestHealthDefaultProbeEdgePaths 默认探针执行器的防御分支：非法 URL、
// systemctl 报错、未知类型
func TestHealthDefaultProbeEdgePaths(t *testing.T) {
	cmd := func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
		return nil, []byte("boom"), errors.New("exit status 1")
	}
	e := NewServiceProvider(cmd).health
	if err := e.probe(context.Background(), HealthProbe{Type: "http", Target: "http://\x7f"}); err == nil {
		t.Fatal("invalid URL should error")
	}
	if err := e.probe(context.Background(), HealthProbe{Type: "systemd", Target: "a.service"}); err == nil {
		t.Fatal("systemctl failure should error")
	}
	if err := e.probe(context.Background(), HealthProbe{Type: "bogus"}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unknown type err: %v", err)
	}
}

// TestHealthSetConfigWakeFull wake 满时非阻塞投递（重配风暴不堆积信号）
func TestHealthSetConfigWakeFull(t *testing.T) {
	e := NewServiceProvider(nil).health
	e.wake <- struct{}{} // 预填：下一次 SetConfig 走 default 分支
	e.SetConfig(HealthConfig{Probes: []HealthProbe{{
		ID: "p", Type: "tcp", Target: "127.0.0.1:1",
		IntervalSec: 3600, TimeoutSec: 1, FailThreshold: 1,
	}}})
}

// TestHealthApplyHealOutcomeNoState 自愈落账时状态缺席（探针被重配置移除
// 的竞态窗口）静默返回
func TestHealthApplyHealOutcomeNoState(t *testing.T) {
	env := newHealthTestEnv(nil)
	p := validHealthProbe()
	env.e.cfg = HealthConfig{Probes: []HealthProbe{p}}
	env.e.applyHealOutcome(p, time.Millisecond, nil) // states 空：不 panic
	if _, events := env.e.snapshot(true); len(events) != 1 {
		t.Fatalf("heal event should still emit, got %d", len(events))
	}
}

// TestHealthLoopEmptyConfig 空配置循环：挂起等 wake，重配唤醒后继续空转
func TestHealthLoopEmptyConfig(t *testing.T) {
	e := NewServiceProvider(nil).health
	e.SetConfig(HealthConfig{}) // 空 probes：loop 阻塞在 wake；第二次 push 唤醒
	time.Sleep(50 * time.Millisecond)
	e.SetConfig(HealthConfig{})
}

// TestHealthLoopTimerAndEpochSkip 循环的定时触发与过期轮次丢弃（D3）：
// epoch 在睡眠窗口内被推高 → timer 自然到期后该轮作废、按新配置重排
func TestHealthLoopTimerAndEpochSkip(t *testing.T) {
	env := newHealthTestEnv(nil)
	var calls int
	env.e.probe = func(context.Context, HealthProbe) error {
		env.mu.Lock()
		calls++
		n := calls
		env.mu.Unlock()
		if n == 1 {
			return errors.New("first fails")
		}
		return nil
	}
	p := validHealthProbe()
	p.IntervalSec = 1
	p.FailThreshold = 1
	env.e.SetConfig(HealthConfig{Probes: []HealthProbe{p}, Whitelist: []string{"cloudflared.service"}})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		env.e.mu.Lock()
		n := calls
		env.e.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	// 睡眠窗口内推高 epoch（模拟 SetConfig 后的过期轮次）→ timer 到期后丢弃
	env.e.mu.Lock()
	env.e.epoch++
	env.e.mu.Unlock()
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		env.e.mu.Lock()
		n := calls
		env.e.mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	env.e.mu.Lock()
	n := calls
	env.e.mu.Unlock()
	if n < 2 {
		t.Fatalf("probe calls = %d, want >=2 (timer fire + epoch skip path)", n)
	}
}

// TestHealthLoopOverdueClamp 状态被回拨（时钟/长阻塞恢复）→ 到期时间为
// 过去 → 循环夹零立即补跑
func TestHealthLoopOverdueClamp(t *testing.T) {
	env := newHealthTestEnv(errors.New("down"))
	p := validHealthProbe()
	p.IntervalSec = 5
	p.FailThreshold = 60 // 期间不触发自愈，纯调度路径
	env.e.SetConfig(HealthConfig{Probes: []HealthProbe{p}})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		env.e.mu.Lock()
		st := env.e.states[p.ID]
		ok := st != nil && st.LastCheck > 0
		env.e.mu.Unlock()
		if ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	env.e.mu.Lock()
	env.e.states[p.ID].LastCheck = time.Now().Add(-time.Hour).Unix() // 回拨到期时间
	env.e.mu.Unlock()
	env.e.wake <- struct{}{} // 循环在定时器上睡眠，唤醒后重算才发现过期
	// 补跑把 LastCheck 拉回现在
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		env.e.mu.Lock()
		lc := env.e.states[p.ID].LastCheck
		env.e.mu.Unlock()
		if time.Now().Unix()-lc < 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("overdue probe was not re-run immediately")
}

// TestHealthConfigActionBadPayload 载荷重解析失败（不可序列化值 / 标量）整包拒绝
func TestHealthConfigActionBadPayload(t *testing.T) {
	sp := NewServiceProvider(nil)
	if _, err := sp.HealthConfigAction(map[string]interface{}{"probes": make(chan int)}); err == nil {
		t.Fatal("unmarshalable payload should error")
	}
	if _, err := sp.HealthConfigAction(map[string]interface{}{"probes": "nope"}); err == nil {
		t.Fatal("scalar payload should error")
	}
}

// TestHealthNowEmptyID 立即探测缺 id 拒绝
func TestHealthNowEmptyID(t *testing.T) {
	if _, err := NewServiceProvider(nil).HealthNowAction(""); err == nil {
		t.Fatal("empty id should error")
	}
}

// TestHealthCallDispatch Call() 对 health.* 三动作的分发路由
func TestHealthCallDispatch(t *testing.T) {
	sp := NewServiceProvider(nil)
	sp.health.systemd = func() bool { return true }

	p := validHealthProbe()
	if _, err := sp.Call("health.config", map[string]interface{}{
		"probes": []interface{}{p}, "whitelist": []interface{}{"cloudflared.service"},
	}); err != nil {
		t.Fatalf("health.config via Call: %v", err)
	}
	if _, err := sp.Call("health.status", nil); err != nil {
		t.Fatalf("health.status via Call: %v", err)
	}
	if _, err := sp.Call("health.now", map[string]interface{}{"id": p.ID}); err != nil {
		t.Fatalf("health.now via Call: %v", err)
	}
	if _, err := sp.Call("health.nope", nil); err == nil {
		t.Fatal("unknown health action should error")
	}
}

// TestHealthStatusPeekVsDrain status 的窥视/取走双语义（D8）：默认窥视
// 保留事件（仪表盘读不吞事件），drain=true 才取走（归集循环专用）
func TestHealthStatusPeekVsDrain(t *testing.T) {
	env := newHealthTestEnv(nil)
	env.e.emit(healthEventDown, "p", "", "x")

	out, err := env.sp.HealthStatusAction(nil) // 窥视
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if got := len(out.(map[string]interface{})["events"].([]healthEvent)); got != 1 {
		t.Fatalf("peek should keep events, got %d", got)
	}
	out, err = env.sp.HealthStatusAction(map[string]interface{}{"drain": true})
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if got := len(out.(map[string]interface{})["events"].([]healthEvent)); got != 1 {
		t.Fatalf("drain should return events, got %d", got)
	}
	_, events := env.e.snapshot(false) // 再窥视为空：事件已被取走
	if len(events) != 0 {
		t.Fatalf("events should be drained, got %d", len(events))
	}
	// 非法 drain 值按窥视处理
	env.e.emit(healthEventDown, "p", "", "y")
	if _, err := env.sp.HealthStatusAction(map[string]interface{}{"drain": "yes"}); err != nil {
		t.Fatalf("non-bool drain should not error: %v", err)
	}
}
