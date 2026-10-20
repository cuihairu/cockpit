package liveness

import (
	"context"
	"sync"
	"time"
)

// DefaultInterval 探活调度缺省周期：与 agent 心跳（30s）同一量级，用于
// 交叉验证——目标 agent 心跳缺失时，网络层在位判定仍可判出。
const DefaultInterval = 60 * time.Second

// DefaultWorkers 单周期并发探测上限：串行扫一个 /24 会被单源超时拖住，
// 并发后单周期成本约等于最慢一批目标。
const DefaultWorkers = 16

// Target 探活目标：IP + 该目标专属的 TCP 探测端口清单（空则 TCP 源报 unknown）。
type Target struct {
	IP    string
	Ports []int
}

// Scheduler 探活调度器：按周期对目标清单跑三源探测，每周期结束把该周期
// 全部报告交给 OnCycle 回调（agent 侧据此组装一条推送消息）。
//
// 调度纪律：周期串行不重叠——上一周期未结束前不启新周期，慢周期只把后续
// tick 顺延；单源超时已兜住单个目标的耗时。目标清单为空时 Run 立即返回。
type Scheduler struct {
	Targets  []Target
	Interval time.Duration    // 周期；<=0 用 DefaultInterval
	Timeout  time.Duration    // 单源超时；<=0 用 DefaultTimeout
	Workers  int              // 单周期并发上限；<=0 用 DefaultWorkers
	Prober   ProbeFunc        // 探测注入点；nil 用 Probe
	Ticks    <-chan time.Time // tick 源注入点（测试手控）；nil 用 Interval 建真实 ticker
	OnCycle  func([]Report)   // 周期回调；nil 丢弃报告
}

// ProbeFunc 单目标探测签名（与 Probe 一致）。
type ProbeFunc func(context.Context, string, Options) Report

// Run 周期循环：收到 tick 跑一轮，ctx 取消即返回。
func (s *Scheduler) Run(ctx context.Context) {
	ticks, stop := s.tickSource()
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			s.runCycle(ctx)
		}
	}
}

// tickSource 返回 tick 通道与停止函数（测试注入通道直出，生产建真实 ticker）。
func (s *Scheduler) tickSource() (<-chan time.Time, func()) {
	if s.Ticks != nil {
		return s.Ticks, func() {}
	}
	t := time.NewTicker(s.interval())
	return t.C, t.Stop
}

func (s *Scheduler) interval() time.Duration {
	if s.Interval <= 0 {
		return DefaultInterval
	}
	return s.Interval
}

func (s *Scheduler) workers() int {
	if s.Workers <= 0 {
		return DefaultWorkers
	}
	return s.Workers
}

func (s *Scheduler) prober() ProbeFunc {
	if s.Prober == nil {
		return Probe
	}
	return s.Prober
}

func (s *Scheduler) timeout() time.Duration {
	if s.Timeout <= 0 {
		return DefaultTimeout
	}
	return s.Timeout
}

// runCycle 单周期：并发探测全部目标（并发上限 Workers），结果按目标顺序回调。
// 空目标清单不回调。
func (s *Scheduler) runCycle(ctx context.Context) []Report {
	if len(s.Targets) == 0 {
		return nil
	}
	reports := make([]Report, len(s.Targets))
	prober := s.prober()
	sem := make(chan struct{}, s.workers())
	var wg sync.WaitGroup
	for i, tg := range s.Targets {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			reports[i] = prober(ctx, tg.IP, Options{Timeout: s.timeout(), Ports: tg.Ports})
		}(i)
	}
	wg.Wait()
	if s.OnCycle != nil {
		s.OnCycle(reports)
	}
	return reports
}
