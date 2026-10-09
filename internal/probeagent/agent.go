package probeagent

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/core/healthprobe"
)

// transitionRing 迁移事件环容量（report 面消费前的缓冲上限）。
const transitionRing = 64

// Agent 探测调度器：每 target 独立 ticker + 超时探测 + Guard 防抖记录；
// 状态定性迁移进事件环并转发 Alerter（herald 留位，简档 §4.4）。
type Agent struct {
	targets  []TargetConfig
	checkers []healthprobe.Checker
	guards   []*healthprobe.Guard

	mu          sync.Mutex
	transitions []healthprobe.Transition
	alerter     Alerter
}

// New 按已回填缺省的配置构建调度器（配置须先经 LoadConfig resolve）。
func New(cfg *Config) (*Agent, error) {
	if cfg == nil || len(cfg.Targets) == 0 {
		return nil, errors.New("no probe targets")
	}
	a := &Agent{alerter: NopAlerter{}}
	for i := range cfg.Targets {
		t := cfg.Targets[i]
		chk, err := checkerFor(t)
		if err != nil {
			return nil, err
		}
		a.targets = append(a.targets, t)
		a.checkers = append(a.checkers, chk)
		a.guards = append(a.guards, healthprobe.NewGuard(t.Name, t.Threshold, t.Recovery, a.onTransition))
	}
	return a, nil
}

// SetAlerter 换告警出口（缺省 NopAlerter；herald 接线点）。
func (a *Agent) SetAlerter(al Alerter) {
	if al == nil {
		al = NopAlerter{}
	}
	a.mu.Lock()
	a.alerter = al
	a.mu.Unlock()
}

// checkerFor target → core/healthprobe Checker；超时经 Client/Dialer 注入
// （Checker 自身不持有超时旋钮）。
func checkerFor(t TargetConfig) (healthprobe.Checker, error) {
	timeout := t.Timeout.D()
	switch t.Type {
	case "http":
		return &healthprobe.HTTPChecker{
			URL:          t.URL,
			ExpectStatus: t.ExpectStatus,
			Client:       &http.Client{Timeout: timeout},
		}, nil
	case "tcp":
		return &healthprobe.TCPChecker{Addr: t.Addr, Dialer: &net.Dialer{Timeout: timeout}}, nil
	default:
		return nil, errors.New("unsupported type: " + t.Type)
	}
}

// onTransition 迁移回调：入环（容量裁剪）+ 转发出口（锁外调用，出口可
// 回读 Agent 不死锁）。
func (a *Agent) onTransition(tr healthprobe.Transition) {
	a.mu.Lock()
	a.transitions = append(a.transitions, tr)
	if len(a.transitions) > transitionRing {
		a.transitions = a.transitions[len(a.transitions)-transitionRing:]
	}
	al := a.alerter
	a.mu.Unlock()
	al.Alert(tr)
}

// Run 阻塞运行全部 target 循环至 ctx 取消；每 target 首探立即执行，
// 之后按各自 interval 轮询。
func (a *Agent) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := range a.targets {
		wg.Add(1)
		go a.loop(ctx, &wg, i)
	}
	wg.Wait()
}

func (a *Agent) loop(ctx context.Context, wg *sync.WaitGroup, i int) {
	defer wg.Done()
	a.probe(ctx, i)
	ticker := time.NewTicker(a.targets[i].Interval.D())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.probe(ctx, i)
		}
	}
}

func (a *Agent) probe(ctx context.Context, i int) {
	pctx, cancel := context.WithTimeout(ctx, a.targets[i].Timeout.D())
	defer cancel()
	err := a.checkers[i].Check(pctx)
	if ctx.Err() != nil {
		// 停机取消的在途探测不计服务故障——这是 agent 自身关闭的观测
		// 伪影，不是目标状态变化（TestRunGracefulShutdown 语义锚点）。
		return
	}
	a.guards[i].Record(err, time.Now())
}

// ProbeAll 同步探测全部 target 各一次（-once 模式与测试用）。
func (a *Agent) ProbeAll(ctx context.Context) {
	for i := range a.targets {
		a.probe(ctx, i)
	}
}

// Snapshots 全部 target 当前观测面（上行/状态文件消费）。
func (a *Agent) Snapshots() []healthprobe.Snapshot {
	out := make([]healthprobe.Snapshot, 0, len(a.guards))
	for _, g := range a.guards {
		out = append(out, g.Snapshot())
	}
	return out
}

// DrainTransitions 取走累积迁移事件（report 面上行后清空）。
func (a *Agent) DrainTransitions() []healthprobe.Transition {
	a.mu.Lock()
	defer a.mu.Unlock()
	trs := a.transitions
	a.transitions = nil
	return trs
}
