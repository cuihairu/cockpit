package healthprobe

import (
	"sync"
	"time"
)

// State 探测状态：Unknown 启动冷区（首轮未定性）、Healthy、Faulty。
type State string

const (
	StateUnknown State = "unknown"
	StateHealthy State = "healthy"
	StateFaulty  State = "faulty"
)

// Transition 状态迁移事件（仅 Faulty↔Healthy↔Unknown 间定性变化，窗口
// 开合点）；上行与告警出口（Alerter）都挂它。
type Transition struct {
	Target    string
	From, To  State
	At        time.Time
	LastError string
}

// Snapshot 单目标当前观测面：语义、状态、状态起始时刻、最近一次探测结果
// 与故障窗口（进行中 + 近期已关闭）；LastOK 即「最近活体时刻」（心跳语义
// 的主读数，其余语义下也有用）。
type Snapshot struct {
	Target      string    `json:"target"`
	Semantics   Semantics `json:"semantics,omitempty"`
	State       State     `json:"state"`
	Since       time.Time `json:"since"`
	LastChecked time.Time `json:"lastChecked"`
	LastOK      time.Time `json:"lastOk,omitempty"`
	LastError   string    `json:"lastError,omitempty"`
	Open        *Window   `json:"open,omitempty"`
	Recent      []Window  `json:"recent,omitempty"`
}

// Guard 单目标防抖状态机：连续失败 ≥ faultThreshold 判故障、连续成功 ≥
// recoverThreshold 判恢复（结果与倾向相异即清零重计）。窗口开合与事件
// 回调只发生在状态定性迁移处。Record（单写者）与 Snapshot（report 面
// 并发读）经 mu 互斥；onEvent 在锁内回调——出口实现须避免同步回锁
// Guard。零业务零平台，可独立复用。
type Guard struct {
	mu sync.Mutex

	name             string
	semantics        Semantics
	faultThreshold   int
	recoverThreshold int

	state      State
	fails, oks int
	since      time.Time
	lastErr    string
	lastCheck  time.Time
	lastOK     time.Time
	open       *Window
	closed     []Window
	onEvent    func(Transition)
}

// NewGuard 建状态机（Liveness 语义：判定 + 窗口留痕）；阈值为 0/负按 1
// （首轮即定性）。onEvent 可为 nil。
func NewGuard(name string, faultThreshold, recoverThreshold int, onEvent func(Transition)) *Guard {
	return NewGuardSem(name, SemLiveness, faultThreshold, recoverThreshold, onEvent)
}

// NewGuardSem 指定语义建状态机。Liveness/Readiness 判定一致（后果归调用方，
// 区别只随窗口留痕）；Heartbeat 只打点（阈值不参与判定，仅钳位保留）。
func NewGuardSem(name string, sem Semantics, faultThreshold, recoverThreshold int, onEvent func(Transition)) *Guard {
	if faultThreshold < 1 {
		faultThreshold = 1
	}
	if recoverThreshold < 1 {
		recoverThreshold = 1
	}
	return &Guard{
		name:             name,
		semantics:        sem,
		faultThreshold:   faultThreshold,
		recoverThreshold: recoverThreshold,
		state:            StateUnknown,
		onEvent:          onEvent,
	}
}

// Record 记一次探测结果：err==nil 成功。now 由调用方注入（测试确定性，
// 生产传 time.Now()）。Heartbeat 语义只打点：成功刷新 LastOK、失败仅留
// 最近原因，不进状态机（无迁移无窗口）。
func (g *Guard) Record(err error, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastCheck = now
	if err == nil {
		g.lastOK = now
		if g.semantics.judging() {
			g.recordSuccess(now)
		}
		return
	}
	if !g.semantics.judging() {
		g.lastErr = err.Error()
		return
	}
	g.recordFailure(err.Error(), now)
}

func (g *Guard) recordSuccess(now time.Time) {
	g.fails = 0
	g.oks++
	if g.state == StateFaulty && g.oks >= g.recoverThreshold {
		g.open.EndedAt = now
		g.pushClosed(*g.open)
		g.open = nil
		g.transition(StateHealthy, now, "")
		return
	}
	if g.state == StateUnknown && g.oks >= g.recoverThreshold {
		g.transition(StateHealthy, now, "")
	}
}

func (g *Guard) recordFailure(msg string, now time.Time) {
	g.oks = 0
	g.fails++
	g.lastErr = msg
	if g.fails < g.faultThreshold {
		return
	}
	if g.state == StateFaulty {
		g.open.LastError = msg
		return
	}
	g.open = &Window{Target: g.name, Semantics: g.semantics, StartedAt: now, LastError: msg}
	g.transition(StateFaulty, now, msg)
}

// transition 定性迁移：换态、刷新 since、发事件。
func (g *Guard) transition(to State, now time.Time, lastErr string) {
	from := g.state
	g.state = to
	g.since = now
	g.fails, g.oks = 0, 0
	if g.onEvent != nil {
		g.onEvent(Transition{Target: g.name, From: from, To: to, At: now, LastError: lastErr})
	}
}

// Snapshot 当前观测面（防御性拷贝，调用方改动不影响内部状态）。
func (g *Guard) Snapshot() Snapshot {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := Snapshot{
		Target:      g.name,
		Semantics:   g.semantics,
		State:       g.state,
		Since:       g.since,
		LastChecked: g.lastCheck,
		LastOK:      g.lastOK,
		LastError:   g.lastErr,
	}
	if g.open != nil {
		w := *g.open
		s.Open = &w
	}
	if len(g.closed) > 0 {
		s.Recent = make([]Window, len(g.closed))
		copy(s.Recent, g.closed)
	}
	return s
}
