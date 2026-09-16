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
