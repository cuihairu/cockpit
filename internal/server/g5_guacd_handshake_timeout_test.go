package server

import (
	"net"
	"testing"
	"time"
)

// TestG5GuacdHandshakeReadTimeout 验证 G5：guacd 握手阶段停滞时，
// ReadString 必须受读期限约束并超时收尾，不得永久阻塞 handler。
func TestG5GuacdHandshakeReadTimeout(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)

	// 用真实 TCP 对（net.Pipe 不支持 SetReadDeadline，会走 set-deadline-failed
	// 分支而非读超时路径）。服务端 c2 不读也不写：guacd 停滞。
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		// 永不写 select 响应，保持连接僵住。
		<-time.After(5 * time.Second)
		_ = c.Close()
	}()

	oldDial := dialGuacd
	dialGuacd = func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.Dial("tcp", l.Addr().String())
	}
	t.Cleanup(func() { dialGuacd = oldDial })

	oldTimeout := guacdHandshakeTimeout
	guacdHandshakeTimeout = 50 * time.Millisecond
	t.Cleanup(func() { guacdHandshakeTimeout = oldTimeout })

	conn, _, _ := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole",
		covTicket(t, s, map[string]string{
			"agent_id": "a", "host": "10.0.0.9", "port": "3389", "protocol": "rdp",
		}))
	t.Cleanup(func() { _ = conn.Close() })

	// 先消费 tunnel UUID 帧，释放同步 pipe 写。
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}

	// guacd mock 永不写 select 响应；握手超时后 handler 应关闭 WS。
	deadline := time.Now().Add(2 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected websocket close after guacd handshake read timeout")
	}
	if time.Now().After(deadline) {
		t.Fatal("guacd handshake read timeout did not close within 2s")
	}
}
