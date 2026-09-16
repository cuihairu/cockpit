package remote

// cov_io_gaps_test.go 覆盖 forward 的目标端写失败分支与 cleanupLoop
// 的 tick 分支（注入短周期）。

import (
	"net"
	"testing"
	"time"
)

func TestCovForwardDstWriteError(t *testing.T) {
	// 源端 pipe 可读出数据；目标端已关闭 → Write 报错走 190 行日志分支
	s1, s2 := net.Pipe()
	defer s1.Close()
	d1, d2 := net.Pipe()
	d1.Close()
	d2.Close()

	go func() {
		_, _ = s2.Write([]byte("x"))
		s2.Close()
	}()

	c := &Connection{ID: "cov-fwd", Target: "h:1", ClientConn: s1, TargetConn: d1}
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.forward(s1, d1, true)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("forward did not exit on dst write error")
	}
}

func TestCovCleanupLoopTick(t *testing.T) {
	save := connCleanupInterval
	connCleanupInterval = 50 * time.Millisecond
	t.Cleanup(func() { connCleanupInterval = save })

	m := NewManager()
	// 一条 idle 超过 5 分钟的连接：tick 后应被清理
	c1, c2 := net.Pipe()
	defer c2.Close()
	conn := &Connection{ID: "cov-old", Target: "h:1",
		ClientConn: c1, TargetConn: c2,
		CreatedAt: time.Now().Add(-10 * time.Minute), LastActive: time.Now().Add(-10 * time.Minute)}
	m.mu.Lock()
	m.connections["cov-old"] = conn
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.cleanupLoop()
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.RLock()
		_, kept := m.connections["cov-old"]
		m.mu.RUnlock()
		if !kept {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.mu.RLock()
	_, kept := m.connections["cov-old"]
	m.mu.RUnlock()
	if kept {
		t.Fatal("idle connection should be cleaned up on tick")
	}

	m.cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cleanupLoop did not exit on cancel")
	}
}
