package backoff

import (
	"context"
	"testing"
	"time"
)

// TestIteratorSequence 退避时序正反用例：指数推进、封顶恒定、恒定间隔退化。
func TestIteratorSequence(t *testing.T) {
	cases := []struct {
		name   string
		policy Policy
		want   []time.Duration
	}{
		{
			// agent.go 重连的两段固定计时（5s 首等 + 10s 恒定重试）
			// 的同语义映射
			name:   "two-phase fixed",
			policy: Policy{Initial: 5 * time.Second, Max: 10 * time.Second},
			want:   []time.Duration{5 * time.Second, 10 * time.Second, 10 * time.Second, 10 * time.Second},
		},
		{name: "exponential capped", policy: Policy{Initial: time.Millisecond, Max: 100 * time.Millisecond},
			want: []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond, 8 * time.Millisecond,
				16 * time.Millisecond, 32 * time.Millisecond, 64 * time.Millisecond, 100 * time.Millisecond,
				100 * time.Millisecond, 100 * time.Millisecond}},
		{name: "constant interval", policy: Policy{Initial: 3 * time.Second, Max: 3 * time.Second},
			want: []time.Duration{3 * time.Second, 3 * time.Second, 3 * time.Second}},
		{name: "max below initial", policy: Policy{Initial: 4 * time.Second, Max: 2 * time.Second},
			want: []time.Duration{4 * time.Second, 4 * time.Second}},
		{name: "zero initial falls back", policy: Policy{Max: time.Minute},
			want: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			it := Start(tc.policy)
			for i, want := range tc.want {
				got := it.Next()
				if got != want {
					t.Fatalf("Next()#%d = %v, want %v (policy %+v)", i+1, got, want, tc.policy)
				}
			}
		})
	}
}

// TestIteratorIndependent 每次 Start 生成独立序列，不共享游标。
func TestIteratorIndependent(t *testing.T) {
	it1 := Start(Policy{Initial: time.Millisecond, Max: 4 * time.Millisecond})
	it2 := Start(Policy{Initial: time.Millisecond, Max: 4 * time.Millisecond})
	if it1.Next() != time.Millisecond {
		t.Fatal("it1 first should be initial")
	}
	if got := it2.Next(); got != time.Millisecond {
		t.Fatalf("it2 first = %v, want 1ms (it1 advance must not leak)", got)
	}
}

// TestWaitFull 等满返回 true。
func TestWaitFull(t *testing.T) {
	ctx := context.Background()
	start := time.Now()
	if !Wait(ctx, 5*time.Millisecond) {
		t.Fatal("Wait should return true when elapsed")
	}
	if time.Since(start) < 5*time.Millisecond {
		t.Fatal("Wait returned before delay")
	}
}

// TestWaitCancel ctx 取消立即返回 false（不空等）。
func TestWaitCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(2 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	if Wait(ctx, time.Second) {
		t.Fatal("Wait should return false on cancel")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("Wait did not return promptly on cancel")
	}
}

// TestWaitZeroDelay 零/负时长不做 timer：活 ctx 即 true，已取消即 false。
func TestWaitZeroDelay(t *testing.T) {
	if !Wait(context.Background(), 0) {
		t.Fatal("zero delay with live ctx should be true")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if Wait(ctx, -time.Second) {
		t.Fatal("negative delay with cancelled ctx should be false")
	}
}
