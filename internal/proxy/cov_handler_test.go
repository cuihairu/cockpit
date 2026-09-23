package proxy

import (
	"net"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// covPutAgentConn 向 Handler 注册一个目标连接（测试辅助）。
func covPutAgentConn(h *Handler, ac *AgentTargetConn) {
	h.mu.Lock()
	h.conns[ac.ID] = ac
	h.mu.Unlock()
}

// TestCovHandlerProxyDataWriteError 覆盖 Agent 端 HandleProxyData
// 写目标失败后的 log/Close/SendClose/返回错误分支。
func TestCovHandlerProxyDataWriteError(t *testing.T) {
	h := NewHandler()
	defer h.Stop()

	// 对端关闭后写 pipe 端必然失败
	c1, c2 := net.Pipe()
	c1.Close()
	ac := &AgentTargetConn{ID: "cov-conn", ProxyID: "cov-p", Target: "127.0.0.1:80", Conn: c2}
	covPutAgentConn(h, ac)

	msg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "cov-p",
		"connId":  "cov-conn",
		"data":    []byte("x"),
	})

	if err := h.HandleProxyData(msg); err == nil {
		t.Fatal("HandleProxyData() should fail when target conn is closed")
	}
	if !ac.closed.Load() {
		t.Error("target connection should be closed after write failure")
	}
}

// TestCovHandlerProxyCloseDecodeError 覆盖 Agent 端 HandleProxyClose
// 的消息解码失败分支。
func TestCovHandlerProxyCloseDecodeError(t *testing.T) {
	h := NewHandler()
	defer h.Stop()

	// connId 类型错误（数字）无法解码进 string 字段
	msg := protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
		"proxyId": "cov-p",
		"connId":  12345,
	})

	if err := h.HandleProxyClose(msg); err == nil {
		t.Fatal("HandleProxyClose() should fail for undecodable payload")
	}
}

// TestCovHandlerReadFromTargetSendError 覆盖 Agent 端 readFromTarget
// 中 SendMessage 失败后的 break 分支。
func TestCovHandlerReadFromTargetSendError(t *testing.T) {
	h := NewHandler() // 未 Start：SendMessage 返回 "handler not running"

	clientEnd, targetEnd := net.Pipe()
	ac := &AgentTargetConn{ID: "cov-conn", ProxyID: "cov-p", Target: "127.0.0.1:80", Conn: targetEnd}
	covPutAgentConn(h, ac)

	done := make(chan struct{})
	go func() {
		h.readFromTarget(ac)
		close(done)
	}()

	// 目标有数据可读 → SendMessage 失败 → break 退出
	if _, err := clientEnd.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("readFromTarget did not exit after send failure")
	}
	clientEnd.Close()
}

// ============ resize 分支（PTY WindowChange） ============

// covResizableConn 实现 io.ReadWriteCloser + windowResizer
type covResizableConn struct {
	fakeRWC
	changes [][2]int
}

func (c *covResizableConn) WindowChange(rows, cols int) error {
	c.changes = append(c.changes, [2]int{rows, cols})
	return nil
}

func TestHandleProxyDataResizeBareTCP(t *testing.T) {
	h := NewHandler()
	defer h.Stop()

	c1, c2 := net.Pipe()
	defer c2.Close()
	ac := &AgentTargetConn{ID: "rz-1", ProxyID: "p", Target: "h:23", Conn: c1}
	covPutAgentConn(h, ac)

	// 裸 TCP 目标无 PTY 概念 → 静默忽略，不写数据、不报错
	msg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "p", "connId": "rz-1", "resize": true, "rows": 30, "cols": 100,
	})
	if err := h.HandleProxyData(msg); err != nil {
		t.Fatalf("HandleProxyData(resize) = %v, want nil", err)
	}
}

func TestHandleProxyDataResizeCallsWindowChange(t *testing.T) {
	h := NewHandler()
	defer h.Stop()

	rc := &covResizableConn{}
	ac := &AgentTargetConn{ID: "rz-2", ProxyID: "p", Target: "h:22", Conn: rc}
	covPutAgentConn(h, ac)

	msg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "p", "connId": "rz-2", "resize": true, "rows": 40, "cols": 120,
	})
	if err := h.HandleProxyData(msg); err != nil {
		t.Fatalf("HandleProxyData(resize) = %v, want nil", err)
	}
	if len(rc.changes) != 1 || rc.changes[0] != [2]int{40, 120} {
		t.Errorf("WindowChange calls = %v, want [[40 120]]", rc.changes)
	}
	// resize 不写数据
	if len(rc.writes) != 0 {
		t.Errorf("writes = %v, want none", rc.writes)
	}
}

func TestHandleProxyDataResizeClosedConn(t *testing.T) {
	h := NewHandler()
	defer h.Stop()

	rc := &covResizableConn{}
	ac := &AgentTargetConn{ID: "rz-3", ProxyID: "p", Target: "h:22", Conn: rc}
	covPutAgentConn(h, ac)
	ac.Close()

	msg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "p", "connId": "rz-3", "resize": true, "rows": 1, "cols": 1,
	})
	if err := h.HandleProxyData(msg); err == nil {
		t.Error("closed connection should reject data")
	}
}

func TestAgentTargetConnCloseWithSSHSession(t *testing.T) {
	// sshSess 非空时走 SSH 关闭路径（幂等）
	rc := &covResizableConn{}
	ac := &AgentTargetConn{ID: "ssh-1", ProxyID: "p", Target: "h:22", Conn: rc, sshSess: &SSHSession{}}
	if err := ac.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	if !ac.closed.Load() {
		t.Error("should be marked closed")
	}
	if err := ac.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}

func TestHandleProxyDataResizeSSHSession(t *testing.T) {
	h := NewHandler()
	defer h.Stop()

	// SSH 会话：Stream 不实现 windowResizer，resize 必须经 sshSess 转发
	rc := &fakeRWC{}
	ac := &AgentTargetConn{
		ID: "rz-ssh", ProxyID: "p", Target: "h:22",
		Conn:    rc,
		sshSess: &SSHSession{},
	}
	covPutAgentConn(h, ac)

	msg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "p", "connId": "rz-ssh", "resize": true, "rows": 33, "cols": 111,
	})
	// SSHSession.sess 为 nil 时 WindowChange 会 panic？——已 nil 安全则返回 closed/错误
	if err := h.HandleProxyData(msg); err != nil {
		t.Fatalf("HandleProxyData(resize) = %v", err)
	}
	if len(rc.writes) != 0 {
		t.Errorf("resize should not write data: %v", rc.writes)
	}
}
