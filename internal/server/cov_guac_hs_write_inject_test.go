package server

// handshake write 失败分支的确定性覆盖（api_guacamole.go 506-511）。
//
// 背景：TestGuacamoleHandshakeWriteFail 用 TCP stub（SetLinger(0) 发
// RST）触发该分支，但「Write 是否撞上 RST」取决于内核时序——select
// 响应写入内核缓冲后，握手 Write 可能先于 RST 被处理而成功，错误
// 推迟到后续操作才暴露。CI 高负载下抖动，曾致 coverage_check 红腿
// （未登记缺口 506,511）。
//
// 与 failWriteConn / failReadConn 同性质：dialGuacd 注入点 + 行为
// 中性 mock，把落点从内核时序手里收回来——第 1 次 Write（select）
// 成功、Read 回 select 响应、第 2 次 Write（handshake）必失败。

import (
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// handshakeFailConn select write 成功、select 响应可读、handshake
// write 必失败（failed 置位供测试断言——单测内仅 handler goroutine
// 触达 Write/Read，无需锁）
type handshakeFailConn struct {
	net.Conn
	respLeft []byte
	writes   int
	failed   atomic.Bool
}

func (c *handshakeFailConn) Write(p []byte) (int, error) {
	c.writes++
	if c.writes == 1 {
		return len(p), nil // select write 成功
	}
	c.failed.Store(true) // handshake write 撞已死 guacd
	return 0, errors.New("injected handshake write failure")
}

func (c *handshakeFailConn) Read(p []byte) (int, error) {
	if len(c.respLeft) == 0 {
		return 0, io.EOF // 分支命中即 return，后续无读
	}
	n := copy(p, c.respLeft)
	c.respLeft = c.respLeft[n:]
	return n, nil
}

func TestGuacamoleHandshakeWriteFailInjected(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)

	c1, c2 := net.Pipe()
	t.Cleanup(func() { _ = c1.Close(); _ = c2.Close() })
	mock := &handshakeFailConn{
		Conn:     c1,
		respLeft: []byte("6.select,8.hostname,4.port;"),
	}
	old := dialGuacd
	dialGuacd = func(addr string, timeout time.Duration) (net.Conn, error) {
		return mock, nil
	}
	t.Cleanup(func() { dialGuacd = old })

	conn, _, _ := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole",
		covTicket(t, s, map[string]string{
			"agent_id": "a", "host": "10.0.0.9", "port": "3389", "protocol": "rdp",
		}))
	// net.Pipe 同步无缓冲：writeWS(uuid) 阻塞到读完，必须先消费首帧放行
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	// 确定性断言 handshake write 失败分支执行（RST stub 形态只能盲等
	// 500ms，无法断言；注入形态下 failed 置位即分支已走到）
	covWaitGone(t, "handshake write fail", mock.failed.Load)
	conn.Close()
}
