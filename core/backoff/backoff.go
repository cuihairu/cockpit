// Package backoff 统一重试退避旋钮（agent-core ① 通用能力层，~60 行）。
// 收敛散落的重试计时写法：固定间隔起步、逐次加倍封顶；Initial>=Max
// 即退化为恒定间隔（与既有「两段固定计时」同语义）。等待经 Wait 可被
// ctx 中断。零业务、零平台分支、零三方依赖。
package backoff

import (
	"context"
	"time"
)

const defaultInitial = time.Second

// Policy 退避参数：Initial 首次等待，Max 封顶间隔。Max<=Initial 时
// 序列恒为 Initial（恒定间隔）。零值 Initial 回退 defaultInitial。
type Policy struct {
	Initial time.Duration
	Max     time.Duration
}

// Iterator 退避序列发生器：Next 依次返回 Initial、2*Initial、
// 4*Initial…封顶 Max，其后恒为 Max。
type Iterator struct {
	policy  Policy
	current time.Duration
}

// Start 生成退避序列发生器。
func Start(p Policy) *Iterator {
	it := &Iterator{policy: p, current: p.Initial}
	if it.current <= 0 {
		it.current = defaultInitial
	}
	return it
}

// Next 返回下一次等待时长，并把序列推进一档（封顶后不再变化）。
func (it *Iterator) Next() time.Duration {
	d := it.current
	if max := it.policy.Max; max > d {
		if next := d * 2; next > max {
			it.current = max
		} else {
			it.current = next
		}
	}
	return d
}

// Wait 阻塞 d 或 ctx 取消；返回 false 表示 ctx 已取消（未等满）。
func Wait(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
