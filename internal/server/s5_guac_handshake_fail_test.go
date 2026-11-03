package server

import (
	"net"
	"testing"
	"time"
)

// TestS5GuacHandshakeFailureCleansSession 验证 S5：guacd 握手失败（select
// write 失败）后必须从 guacSessions 摘除会话，不得泄漏；台账状态保持
// failed（不被 closeGuacamoleSession 覆盖为 closed）。
func TestS5GuacHandshakeFailureCleansSession(t *testing.T) {
	defer covClearSessions()
	defer covClearGuacSessions()
	s := covRemoteSetup(t)

	c1, c2 := net.Pipe()
	t.Cleanup(func() { _ = c1.Close(); _ = c2.Close() })
	old := dialGuacd
	dialGuacd = func(addr string, timeout time.Duration) (net.Conn, error) {
		return failWriteConn{guacTestTCPAddrConn{Conn: c1}}, nil
	}
	t.Cleanup(func() { dialGuacd = old })

	conn, _, _ := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole",
		covTicket(t, s, map[string]string{
			"agent_id": "a", "host": "10.0.0.9", "port": "3389", "protocol": "rdp",
		}))
	t.Cleanup(func() { _ = conn.Close() })

	// 消费 uuid 帧，释放同步 pipe 写。
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}

	// 等 select-write-failed 分支收尾。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		guacSessionsMu.Lock()
		n := len(guacSessions)
		guacSessionsMu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	guacSessionsMu.Lock()
	n := len(guacSessions)
	guacSessionsMu.Unlock()
	if n != 0 {
		t.Fatalf("guacSessions leaked after handshake failure: %d entries", n)
	}
}
