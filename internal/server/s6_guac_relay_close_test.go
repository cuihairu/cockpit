package server

import (
	"net"
	"testing"
	"time"
)

// TestS6GuacRelayCloseRejectsNewConn 验证 S6：Close 后 acceptLoop 已 accept
// 的新连接必须被拒绝（closed 标志），不得插入 map 逃逸泄漏。
func TestS6GuacRelayCloseRejectsNewConn(t *testing.T) {
	cap := &relayCapture{}
	relay, addr, err := startGuacRelay("agent-1", "guac:sess-s6", "10.0.0.5:22", cap.send)
	if err != nil {
		t.Fatalf("startGuacRelay: %v", err)
	}
	defer relay.Close()

	// 建立一条已注册的连接。
	c1, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	waitForCond(t, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		return len(relay.conns) == 1
	})

	// Close：应清空 map、关闭已注册连接、置 closed。
	relay.Close()

	relay.mu.Lock()
	n := len(relay.conns)
	closed := relay.closed
	relay.mu.Unlock()
	if n != 0 {
		t.Fatalf("conns = %d, want 0 after Close", n)
	}
	if !closed {
		t.Fatal("closed flag not set after Close")
	}

	// 再拨一条：acceptLoop 可能在 listener 关闭前已 accept；handleConn 必须
	// 拒绝（closed）并关闭，不得插入。等待 listener 关闭后拨号失败即视为
	// 已被拒绝（S6）。
	c2, err := net.Dial("tcp", addr)
	if err != nil {
		return // listener 已关：正常
	}
	_ = c2
	time.Sleep(200 * time.Millisecond)
	relay.mu.Lock()
	n = len(relay.conns)
	relay.mu.Unlock()
	if n != 0 {
		t.Fatalf("conns = %d after Close+re-dial, want 0 (S6 leak)", n)
	}
	_ = c1.Close()
}
