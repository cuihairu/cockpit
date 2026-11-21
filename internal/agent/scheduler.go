package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RPC 调度资源隔离（P0 Agent 第二片，2026-11-21，见
// docs/design/agent-scheduling-isolation.md）：agent 侧对入站 RPC 做准入
// 控制——双优先级车道 + 有界并发 + 准入超时预算。
//
// 边界：provider.Provider.Call 无 ctx（internal/agent/provider/provider.go:9），
// 故预算只作用于准入等待，不作用于 in-flight Provider 调用。

// rpcLane 调度车道
type rpcLane int

const (
	// laneHigh 控制面车道：准入短等待、独立小槽位，不被普通任务挤占
	laneHigh rpcLane = iota
	// laneNormal 普通车道：批量/资源类动作
	laneNormal
)

// 调度默认值（Config 零值时使用；钳制见 NewScheduler）
const (
	// DefaultRPCMaxConcurrent 普通车道并发上限
	DefaultRPCMaxConcurrent = 8
	// DefaultRPCMaxHighConcurrent 控制面车道并发上限
	DefaultRPCMaxHighConcurrent = 2
	// DefaultRPCAdmissionTimeout 普通车道准入等待预算
	DefaultRPCAdmissionTimeout = 5 * time.Second
	// DefaultRPCHighAdmissionTimeout 控制面车道准入等待预算
	DefaultRPCHighAdmissionTimeout = 500 * time.Millisecond
	// rpcMaxConcurrentCap 并发上限钳制（防误配把 agent 压死）
	rpcMaxConcurrentCap = 64
)

// Scheduler RPC 准入调度器：两条独立车道的有界槽位 + 准入超时。
type Scheduler struct {
	high      chan struct{}
	normal    chan struct{}
	highWait  time.Duration
	normalWait time.Duration
}

// NewScheduler 创建调度器；零值/非法上限用默认值并钳制到 [1, rpcMaxConcurrentCap]
func NewScheduler(maxNormal, maxHigh int, normalWait, highWait time.Duration) *Scheduler {
	return &Scheduler{
		high:       make(chan struct{}, clampLaneSize(maxHigh, DefaultRPCMaxHighConcurrent)),
		normal:     make(chan struct{}, clampLaneSize(maxNormal, DefaultRPCMaxConcurrent)),
		highWait:   defaultDuration(highWait, DefaultRPCHighAdmissionTimeout),
		normalWait: defaultDuration(normalWait, DefaultRPCAdmissionTimeout),
	}
}

// clampLaneSize 槽位钳制：<=0 用 def，>cap 用 cap
func clampLaneSize(n, def int) int {
	if n <= 0 {
		return def
	}
	if n > rpcMaxConcurrentCap {
		return rpcMaxConcurrentCap
	}
	return n
}

// defaultDuration 零值/负值用默认
func defaultDuration(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}

// classifyMethod 按方法名分类车道：控制面方法走 high，其余 normal。
// 控制面＝必须低延迟可达的方法（system.* 状态类、job.cancel 取消、probe.* 探针）；
// 无点号的方法回退 system provider（见 rpc.parseMethod），同样归 high。
func classifyMethod(method string) rpcLane {
	switch {
	case method == "":
		return laneNormal
	case method == "job.cancel", strings.HasPrefix(method, "job.cancel."):
		return laneHigh
	case strings.HasPrefix(method, "system"), strings.HasPrefix(method, "probe."):
		return laneHigh
	case !strings.Contains(method, "."):
		// 单段方法整体回退 system provider（rpc.parseMethod:125），归控制面
		return laneHigh
	default:
		return laneNormal
	}
}

// Acquire 取一条车道的槽位；成功返回 release 回调，准入超时/ctx 取消返回错误。
// method 为空视为普通车道（未知方法 fail-normal，不误升级控制面）。
func (s *Scheduler) Acquire(ctx context.Context, method string) (func(), error) {
	var slots chan struct{}
	var wait time.Duration
	if classifyMethod(method) == laneHigh {
		slots, wait = s.high, s.highWait
	} else {
		slots, wait = s.normal, s.normalWait
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-timer.C:
		return nil, fmt.Errorf("rpc admission timeout (%s)", method)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Lane 车道槽位容量（诊断用；测试可断言钳制结果）
func (s *Scheduler) Lane(lane rpcLane) int {
	if lane == laneHigh {
		return cap(s.high)
	}
	return cap(s.normal)
}