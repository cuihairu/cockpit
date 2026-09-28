package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ============ 服务健康探针与自愈（服务管理面·策略层）============
//
// 见 docs/guide/service-health-design.md：挂在 service provider 上的策略层，
// agent 侧周期探测（http/tcp/systemd 三类，D4），连续 failThreshold 次失败
// 判 down 并触发自愈（D5）。自愈目标必须在白名单内（D6，双端校验的第二道），
// 重启动作复用 ServiceProvider.DoAction 的执行层路径（D1，不另立 systemctl
// 通道）；unit 级滑动窗口台账防重启风暴（D7）。事件写环形缓冲，由 server
// 定时轮询 service.health.status 拉取归集（D8，协议零改动）。
//
// 本文件只做「判断与节流」：什么算不健康、要不要动手、多久动一次。
// 一切执行原语（restart/ is-active）都来自执行层。

const (
	// healthEventRingCap 事件环形缓冲上限（D8）：server 30s 轮询归集，
	// 正常节奏远用不满；server 长时间失联时丢最旧保新
	healthEventRingCap = 256
	// healthMaxProbes 单机探针数上限（配置爆炸防线）
	healthMaxProbes = 32

	// 数值范围与默认值（D12；server 侧同规则校验）
	healthMinInterval      = 5
	healthMaxInterval      = 3600
	healthDefaultInterval  = 30
	healthMinTimeout       = 1
	healthMaxTimeout       = 30
	healthDefaultTimeout   = 5
	healthMaxThreshold     = 60
	healthDefaultThreshold = 3
	healthMinBackoffWindow = 60
	healthMaxBackoffWindow = 86400
	healthMaxRestarts      = 10

	healthDefaultBackoffWindow    = 600
	healthDefaultMaxRestartsInWin = 3

	// healthBodyDrain http 探针读完少量 body 再关（连接复用 + 防 RST 噪音）
	healthBodyDrain = 4 << 10

	// healthDetailClip 事件/错误 detail 截断（防长 stderr 撑爆事件环）
	healthDetailClip = 200
)

// 事件 kind（server 侧按 kind 归集为审计/告警/通知，D9）
const (
	healthEventDown       = "probe_down"
	healthEventRecovered  = "probe_recovered"
	healthEventHealed     = "heal_restarted"
	healthEventHealFailed = "heal_failed"
	healthEventBlocked    = "heal_blocked"
	healthEventBackoff    = "heal_backoff"
)

// HealthProbe 单条探针定义（server 存储 = 下发载荷，D12 字段语义见设计文档）
type HealthProbe struct {
	ID            string `json:"id"`
	Type          string `json:"type"` // http / tcp / systemd
	Target        string `json:"target"`
	ExpectStatus  int    `json:"expectStatus"` // http 专用，0 视为 200
	IntervalSec   int    `json:"intervalSec"`
	TimeoutSec    int    `json:"timeoutSec"`
	FailThreshold int    `json:"failThreshold"`
	Heal          bool   `json:"heal"`
	Unit          string `json:"unit"` // 自愈目标；systemd 型缺省取 target
	BackoffWindowSec int `json:"backoffWindowSec"`
	MaxRestartsInWindow int `json:"maxRestartsInWindow"`
}

// HealthConfig 单机全量配置（全量替换语义，D3）
type HealthConfig struct {
	Probes    []HealthProbe `json:"probes"`
	Whitelist []string      `json:"whitelist"`
}

// healthProbeState 单探针运行态（快照进 server 缓存供离线灰态，D8）
type healthProbeState struct {
	Status           string         `json:"status"` // ok / fail / unknown
	LastCheck        int64          `json:"lastCheck"`
	ConsecutiveFails int            `json:"consecutiveFails"`
	LastError        string         `json:"lastError,omitempty"`
	LastHeal         *healthHealRec `json:"lastHeal,omitempty"`
}

// healthHealRec 最近一次自愈决策记录（含「没动手」的原因，仪表盘可见）
type healthHealRec struct {
	Time       int64  `json:"time"`
	Unit       string `json:"unit"`
	Result     string `json:"result"` // restarted / restart_failed / blocked_whitelist / backoff_skipped
	Detail     string `json:"detail,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
}

// healthEvent 归集事件（drain 语义：server 取回即清空）
type healthEvent struct {
	Time    int64  `json:"time"`
	ProbeID string `json:"probeId"`
	Kind    string `json:"kind"`
	Unit    string `json:"unit,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// healthProbeIDRe 探针 id：小写字母数字开头，可含连字符（D12）
var healthProbeIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// validateHealthConfig 装载前校验（D12）：缺省值补齐 + 形状/范围白名单。
// 整包拒绝——配置是原子的，不做部分应用。server 侧保存时另有一份同规则
// 校验（双端同规则纪律，规则简单不值得抽公共包）。
func validateHealthConfig(cfg *HealthConfig) error {
	if len(cfg.Probes) == 0 {
		return fmt.Errorf("probes must not be empty")
	}
	if len(cfg.Probes) > healthMaxProbes {
		return fmt.Errorf("too many probes (%d > %d)", len(cfg.Probes), healthMaxProbes)
	}
	seen := map[string]bool{}
	for i := range cfg.Probes {
		p := &cfg.Probes[i]
		if !healthProbeIDRe.MatchString(p.ID) {
			return fmt.Errorf("probe[%d]: invalid id %q (expect lowercase alnum/hyphen, <=64)", i, p.ID)
		}
		if seen[p.ID] {
			return fmt.Errorf("probe[%d]: duplicate id %q", i, p.ID)
		}
		seen[p.ID] = true

		switch p.Type {
		case "http":
			u, err := url.Parse(p.Target)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return fmt.Errorf("probe %q: invalid http target %q (expect absolute http(s) URL)", p.ID, p.Target)
			}
			if p.ExpectStatus == 0 {
				p.ExpectStatus = http.StatusOK
			}
			if p.ExpectStatus < 100 || p.ExpectStatus > 599 {
				return fmt.Errorf("probe %q: expectStatus %d out of range [100,599]", p.ID, p.ExpectStatus)
			}
		case "tcp":
			host, portStr, err := net.SplitHostPort(p.Target)
			if err != nil || host == "" {
				return fmt.Errorf("probe %q: invalid tcp target %q (expect host:port)", p.ID, p.Target)
			}
			port, err := strconv.Atoi(portStr)
			if err != nil || port < 1 || port > 65535 {
				return fmt.Errorf("probe %q: invalid tcp port %q", p.ID, portStr)
			}
		case "systemd":
			if !serviceUnitNameRe.MatchString(p.Target) {
				return fmt.Errorf("probe %q: systemd target %q must be a *.service unit", p.ID, p.Target)
			}
		default:
			return fmt.Errorf("probe %q: unsupported type %q (allowed: http tcp systemd)", p.ID, p.Type)
		}

		if p.IntervalSec == 0 {
			p.IntervalSec = healthDefaultInterval
		}
		if p.IntervalSec < healthMinInterval || p.IntervalSec > healthMaxInterval {
			return fmt.Errorf("probe %q: intervalSec %d out of range [%d,%d]", p.ID, p.IntervalSec, healthMinInterval, healthMaxInterval)
		}
		if p.TimeoutSec == 0 {
			p.TimeoutSec = healthDefaultTimeout
		}
		if p.TimeoutSec < healthMinTimeout || p.TimeoutSec > healthMaxTimeout {
			return fmt.Errorf("probe %q: timeoutSec %d out of range [%d,%d]", p.ID, p.TimeoutSec, healthMinTimeout, healthMaxTimeout)
		}
		if p.FailThreshold == 0 {
			p.FailThreshold = healthDefaultThreshold
		}
		if p.FailThreshold > healthMaxThreshold {
			return fmt.Errorf("probe %q: failThreshold %d out of range [1,%d]", p.ID, p.FailThreshold, healthMaxThreshold)
		}
		// 自愈字段：heal=false 时 unit 缺省取 target（systemd 型），其余清零
		if !p.Heal {
			if p.Unit == "" && p.Type == "systemd" {
				p.Unit = p.Target
			}
			p.BackoffWindowSec, p.MaxRestartsInWindow = 0, 0
			continue
		}
		if p.Unit == "" && p.Type == "systemd" {
			p.Unit = p.Target
		}
		if !serviceUnitNameRe.MatchString(p.Unit) {
			return fmt.Errorf("probe %q: heal unit %q must be a *.service unit", p.ID, p.Unit)
		}
		if p.BackoffWindowSec == 0 {
			p.BackoffWindowSec = healthDefaultBackoffWindow
		}
		if p.BackoffWindowSec < healthMinBackoffWindow || p.BackoffWindowSec > healthMaxBackoffWindow {
			return fmt.Errorf("probe %q: backoffWindowSec %d out of range [%d,%d]", p.ID, p.BackoffWindowSec, healthMinBackoffWindow, healthMaxBackoffWindow)
		}
		if p.MaxRestartsInWindow == 0 {
			p.MaxRestartsInWindow = healthDefaultMaxRestartsInWin
		}
		if p.MaxRestartsInWindow > healthMaxRestarts {
			return fmt.Errorf("probe %q: maxRestartsInWindow %d out of range [1,%d]", p.ID, p.MaxRestartsInWindow, healthMaxRestarts)
		}
	}
	// 白名单：每个条目都是合法 *.service 名，去重
	wl := cfg.Whitelist[:0:0]
	seenUnit := map[string]bool{}
	for _, u := range cfg.Whitelist {
		if !serviceUnitNameRe.MatchString(u) {
			return fmt.Errorf("whitelist entry %q must be a *.service unit", u)
		}
		if seenUnit[u] {
			continue
		}
		seenUnit[u] = true
		wl = append(wl, u)
	}
	cfg.Whitelist = wl
	return nil
}

// serviceHealthEngine 探针 + 自愈引擎（挂在 ServiceProvider 上，D1/D2）。
// 状态全部内存态（D13：agent 重启即清零，server 侧未读告警去重兜底）。
type serviceHealthEngine struct {
	mu     sync.Mutex
	cfg    HealthConfig
	states map[string]*healthProbeState
	events []healthEvent
	// ledger unit -> 窗口内 restart 时刻（D7：两个探针指向同一 unit 共享额度）
	ledger map[string][]time.Time
	// blockedEmitted probeID -> 本轮 down 期已发过 heal_blocked（每期至多一条）
	blockedEmitted map[string]bool
	// backoffEmitted unit -> 上次 heal_backoff 事件时刻（每窗口至多一条）
	backoffEmitted map[string]time.Time
	// epoch SetConfig 递增；loop 用于丢弃配置变更前收集的过期轮次
	epoch int

	// 注入点（测试替身）；nil 时用默认实现
	restart func(unit string) error
	probe   func(ctx context.Context, p HealthProbe) error
	systemd func() bool

	wake    chan struct{}
	started bool
}

// newServiceHealthEngine 构造引擎并接好默认执行器：restart 走执行层
// DoAction("restart")（D1），probe 按类型分发（D4），systemd 平台探测。
func newServiceHealthEngine(sp *ServiceProvider) *serviceHealthEngine {
	e := &serviceHealthEngine{
		states:         map[string]*healthProbeState{},
		ledger:         map[string][]time.Time{},
		blockedEmitted: map[string]bool{},
		backoffEmitted: map[string]time.Time{},
		wake:           make(chan struct{}, 1),
	}
	e.restart = func(unit string) error {
		_, err := sp.DoAction(unit, "restart")
		return err
	}
	e.probe = e.defaultProbe(sp)
	e.systemd = DetectSystemd
	return e
}

// healthHTTPClient http 探针共用 client（超时由每探针 ctx 控制，不设整体
// 超时避免双重约束）；var 仅为测试可注入
var healthHTTPClient = &http.Client{}

// defaultProbe 默认探针执行器（D4，全部只读）：
//   - http：GET target，状态码精确等于 expectStatus（僵死探测的关键——
//     cloudflared /ready 隧道连通才回 200，进程活着但隧道死时非 200）
//   - tcp：DialContext 连通即活（「进程监听还在」类）
//   - systemd：is-active 输出 active（systemd 自己能看见的挂，兜底用）
func (e *serviceHealthEngine) defaultProbe(sp *ServiceProvider) func(context.Context, HealthProbe) error {
	return func(ctx context.Context, p HealthProbe) error {
		switch p.Type {
		case "http":
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Target, nil)
			if err != nil {
				return err
			}
			resp, err := healthHTTPClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, healthBodyDrain))
			if resp.StatusCode != p.ExpectStatus {
				return fmt.Errorf("HTTP %d (expect %d)", resp.StatusCode, p.ExpectStatus)
			}
			return nil
		case "tcp":
			var d net.Dialer
			conn, err := d.DialContext(ctx, "tcp", p.Target)
			if err != nil {
				return err
			}
			return conn.Close()
		case "systemd":
			out, err := sp.systemctlRun("is-active", p.Target)
			if err != nil {
				return err
			}
			if state := strings.TrimSpace(string(out)); state != "active" {
				return fmt.Errorf("systemd reports %q", state)
			}
			return nil
		}
		return fmt.Errorf("unsupported probe type %q", p.Type)
	}
}

// SetConfig 全量替换配置（D3）：状态清零（D13，重新攒阈值）、事件环保留
// （待 server 归集）、退避台账保留（unit 重启史是真实历史，不因改配置清零）。
// 首次调用驱动常驻循环。
func (e *serviceHealthEngine) SetConfig(cfg HealthConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg = cfg
	e.states = map[string]*healthProbeState{}
	e.blockedEmitted = map[string]bool{}
	e.epoch++
	if !e.started {
		e.started = true
		go e.loop()
	}
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// snapshot 状态全量 + 事件 drain（D8）。深拷贝出锁后序列化，避免与
// 引擎循环竞争（LastHeal 指针一并复制）。
func (e *serviceHealthEngine) snapshot() (map[string]healthProbeState, []healthEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	states := make(map[string]healthProbeState, len(e.states))
	for id, st := range e.states {
		cp := *st
		if st.LastHeal != nil {
			h := *st.LastHeal
			cp.LastHeal = &h
		}
		states[id] = cp
	}
	events := e.events
	e.events = nil
	return states, events
}

// emit 追加事件（调用方持锁）；满丢最旧（D8）
func (e *serviceHealthEngine) emit(kind, probeID, unit, detail string) {
	e.events = append(e.events, healthEvent{
		Time:    time.Now().Unix(),
		ProbeID: probeID,
		Kind:    kind,
		Unit:    unit,
		Detail:  clipLine(detail, healthDetailClip),
	})
	if len(e.events) > healthEventRingCap {
		e.events = e.events[len(e.events)-healthEventRingCap:]
	}
}

// whitelisted 自愈目标是否在白名单（调用方持锁）
func (e *serviceHealthEngine) whitelisted(unit string) bool {
	for _, u := range e.cfg.Whitelist {
		if u == unit {
			return true
		}
	}
	return false
}

// backoffAllows D7：unit 窗口台账剪枝后仍有额度则放行（调用方持锁）。
// 剪枝窗口取执行探针的配置（同 unit 多探针共享台账，窗口以当前动作为准）。
func (e *serviceHealthEngine) backoffAllows(p *HealthProbe, now time.Time) bool {
	win := time.Duration(p.BackoffWindowSec) * time.Second
	kept := e.ledger[p.Unit][:0]
	for _, t := range e.ledger[p.Unit] {
		if now.Sub(t) < win {
			kept = append(kept, t)
		}
	}
	e.ledger[p.Unit] = kept
	return len(kept) < p.MaxRestartsInWindow
}

// healthAction 需要执行的自愈动作（applyResult 在锁内决定，锁外执行）
type healthAction struct {
	probe HealthProbe
}

// applyResult 探针结果入状态机（D5 边沿判定 + D6/D7 自愈决策）：
//   - 成功且此前在失败 → recovered 事件、计数清零、blocked 期标志复位
//   - 失败：计数 ++；到达阈值发 down；heal 探针过白名单/退避两道闸后
//     返回动作（锁外执行 restart，再回 applyHealOutcome 落账）
func (e *serviceHealthEngine) applyResult(p HealthProbe, probeErr error) *healthAction {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.states[p.ID]
	if st == nil {
		st = &healthProbeState{Status: "unknown"}
		e.states[p.ID] = st
	}
	st.LastCheck = time.Now().Unix()
	if probeErr == nil {
		if st.ConsecutiveFails > 0 {
			e.emit(healthEventRecovered, p.ID, p.Unit,
				fmt.Sprintf("recovered after %d consecutive fails", st.ConsecutiveFails))
		}
		st.Status = "ok"
		st.ConsecutiveFails = 0
		st.LastError = ""
		delete(e.blockedEmitted, p.ID)
		return nil
	}

	st.Status = "fail"
	st.LastError = clipLine(probeErr.Error(), healthDetailClip)
	st.ConsecutiveFails++
	if st.ConsecutiveFails == p.FailThreshold {
		e.emit(healthEventDown, p.ID, p.Unit,
			fmt.Sprintf("%d consecutive fails: %s", st.ConsecutiveFails, st.LastError))
	}
	if st.ConsecutiveFails < p.FailThreshold || !p.Heal {
		return nil
	}

	// 闸一：白名单（D6，agent 侧第二道；server 保存时已挡第一道）
	if !e.whitelisted(p.Unit) {
		if !e.blockedEmitted[p.ID] {
			e.emit(healthEventBlocked, p.ID, p.Unit,
				"unit not in self-heal whitelist; alert only")
			e.blockedEmitted[p.ID] = true
			st.LastHeal = &healthHealRec{Time: st.LastCheck, Unit: p.Unit,
				Result: "blocked_whitelist", Detail: st.LastError}
		}
		return nil
	}
	// 闸二：退避台账（D7；窗口耗尽每窗口至多一条事件）
	now := time.Now()
	if !e.backoffAllows(&p, now) {
		win := time.Duration(p.BackoffWindowSec) * time.Second
		if last, ok := e.backoffEmitted[p.Unit]; !ok || now.Sub(last) >= win {
			e.emit(healthEventBackoff, p.ID, p.Unit,
				fmt.Sprintf("restart budget exhausted (%d in %ds window); alert only",
					p.MaxRestartsInWindow, p.BackoffWindowSec))
			e.backoffEmitted[p.Unit] = now
			st.LastHeal = &healthHealRec{Time: st.LastCheck, Unit: p.Unit,
				Result: "backoff_skipped", Detail: st.LastError}
		}
		return nil
	}
	return &healthAction{probe: p}
}

// applyHealOutcome 自愈执行结果落账（D7 台账 + D9 事件）：成功失败都占
// 窗口额度——防对坏 unit 反复硬敲。台账与事件无条件落（探针被重配置
// 移除的竞态窗口里审计不能丢），LastHeal 仅在状态在时更新。
func (e *serviceHealthEngine) applyHealOutcome(p HealthProbe, dur time.Duration, restartErr error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	e.ledger[p.Unit] = append(e.ledger[p.Unit], now)
	st := e.states[p.ID]
	rec := &healthHealRec{Time: now.Unix(), Unit: p.Unit, DurationMs: dur.Milliseconds()}
	if restartErr == nil {
		rec.Result = "restarted"
		rec.Detail = "systemctl restart ok"
		if st != nil {
			rec.Detail = fmt.Sprintf("systemctl restart ok (fails=%d)", st.ConsecutiveFails)
		}
		e.emit(healthEventHealed, p.ID, p.Unit, rec.Detail)
	} else {
		rec.Result = "restart_failed"
		rec.Detail = clipLine(restartErr.Error(), healthDetailClip)
		e.emit(healthEventHealFailed, p.ID, p.Unit, rec.Detail)
	}
	if st != nil {
		st.LastHeal = rec
	}
}

// probeDue 探针下次应跑时刻（调用方持锁）；无状态 = 立即到期
func (e *serviceHealthEngine) probeDue(p *HealthProbe) time.Time {
	st := e.states[p.ID]
	if st == nil || st.LastCheck == 0 {
		return time.Time{}
	}
	return time.Unix(st.LastCheck, 0).Add(time.Duration(p.IntervalSec) * time.Second)
}

// loop 常驻调度循环：睡到最近到期探针 → 收集到期项 → 锁外执行（探针与
// restart 都可能慢，绝不持锁等待）→ 结果回状态机。wake/epoch 保证配置
// 变更即时生效并丢弃过期轮次（D3）。probeDue 零值 = 从未跑过 = 立即到期。
func (e *serviceHealthEngine) loop() {
	for {
		e.mu.Lock()
		epoch := e.epoch
		readyNow := false
		var next time.Time
		for i := range e.cfg.Probes {
			d := e.probeDue(&e.cfg.Probes[i])
			if d.IsZero() {
				readyNow = true
				break
			}
			if next.IsZero() || d.Before(next) {
				next = d
			}
		}
		e.mu.Unlock()

		if !readyNow {
			if next.IsZero() {
				<-e.wake
				continue
			}
			d := time.Until(next)
			if d < 0 {
				d = 0
			}
			timer := time.NewTimer(d)
			select {
			case <-timer.C:
			case <-e.wake:
				timer.Stop()
				continue
			}
			timer.Stop()
		}

		now := time.Now()
		e.mu.Lock()
		if e.epoch != epoch {
			e.mu.Unlock()
			continue
		}
		var due []HealthProbe
		for i := range e.cfg.Probes {
			p := e.cfg.Probes[i]
			if dueAt := e.probeDue(&p); dueAt.IsZero() || !dueAt.After(now) {
				// 跑点记在发起时（慢探针不打乱后续节奏，D4 超时兜底）
				st := e.states[p.ID]
				if st == nil {
					st = &healthProbeState{Status: "unknown"}
					e.states[p.ID] = st
				}
				st.LastCheck = now.Unix()
				due = append(due, p)
			}
		}
		e.mu.Unlock()

		for _, p := range due {
			e.runOne(p)
		}
	}
}

// runOne 执行单探针并处理结果/自愈（锁外慢操作 + 锁内状态机）
func (e *serviceHealthEngine) runOne(p HealthProbe) {
	probeErr := e.exec(p)
	if act := e.applyResult(p, probeErr); act != nil {
		start := time.Now()
		rerr := e.restart(p.Unit)
		e.applyHealOutcome(p, time.Since(start), rerr)
	}
}

// exec 带单探针超时执行（D4：timeoutSec 是探针各自的）
func (e *serviceHealthEngine) exec(p HealthProbe) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.TimeoutSec)*time.Second)
	defer cancel()
	return e.probe(ctx, p)
}

// ============ provider 动作（service.health.*）============

// decodeHealthParams 参数载荷重解析（params 已是 JSON 反序列化产物，
// 重 marshaling 到强类型是包内既有惯例的最稳路径）
func decodeHealthParams(params map[string]interface{}) (HealthConfig, error) {
	var cfg HealthConfig
	raw, err := json.Marshal(params)
	if err != nil {
		return cfg, fmt.Errorf("invalid health config payload: %w", err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("invalid health config payload: %w", err)
	}
	return cfg, nil
}

// HealthConfigAction service.health.config：全量替换（D3/D12）。非 systemd
// 平台拒绝任何 heal 探针（D11）——server 侧校验先行，这里兜底。
func (p *ServiceProvider) HealthConfigAction(params map[string]interface{}) (interface{}, error) {
	cfg, err := decodeHealthParams(params)
	if err != nil {
		return nil, err
	}
	if err := validateHealthConfig(&cfg); err != nil {
		return nil, err
	}
	for i := range cfg.Probes {
		if cfg.Probes[i].Heal && !p.health.systemd() {
			return nil, fmt.Errorf("probe %q: heal requires systemd (not available on this platform)", cfg.Probes[i].ID)
		}
	}
	p.health.SetConfig(cfg)
	return map[string]interface{}{"applied": len(cfg.Probes)}, nil
}

// HealthStatusAction service.health.status：运行态快照 + 事件 drain（D8）
func (p *ServiceProvider) HealthStatusAction() (interface{}, error) {
	states, events := p.health.snapshot()
	if events == nil {
		events = []healthEvent{}
	}
	return map[string]interface{}{"states": states, "events": events}, nil
}

// HealthNowAction service.health.now：立即执行一次指定探针（诊断按钮，
// 等同调度触发——计入连续失败与边沿判定，D5）
func (p *ServiceProvider) HealthNowAction(id string) (interface{}, error) {
	if id == "" {
		return nil, fmt.Errorf("probe id is required")
	}
	e := p.health
	e.mu.Lock()
	var found *HealthProbe
	for i := range e.cfg.Probes {
		if e.cfg.Probes[i].ID == id {
			found = &e.cfg.Probes[i]
			break
		}
	}
	var probe HealthProbe
	if found != nil {
		probe = *found
	}
	e.mu.Unlock()
	if found == nil {
		return nil, fmt.Errorf("unknown probe id %q", id)
	}

	probeErr := e.exec(probe)
	if act := e.applyResult(probe, probeErr); act != nil {
		start := time.Now()
		rerr := e.restart(probe.Unit)
		e.applyHealOutcome(probe, time.Since(start), rerr)
	}
	states, _ := e.snapshot()
	out := map[string]interface{}{"probe": id}
	if st, ok := states[id]; ok {
		out["status"] = st.Status
		out["consecutiveFails"] = st.ConsecutiveFails
		if st.LastError != "" {
			out["lastError"] = st.LastError
		}
	}
	return out, nil
}
