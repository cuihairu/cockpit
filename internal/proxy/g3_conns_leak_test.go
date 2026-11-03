package proxy

import (
	"net"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// covGetAgentConn 读取活动连接表条目（测试辅助）。
func covGetAgentConn(h *Handler, connID string) (*AgentTargetConn, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, ok := h.conns[connID]
	return conn, ok
}

// TestG3ReadFromTargetRemovesConn 验证 G3：目标关闭后 readFromTarget 退出
// 时必须从活动连接表摘除条目（旧实现只 Close 不 delete，长跑泄漏）。
func TestG3ReadFromTargetRemovesConn(t *testing.T) {
	h := NewHandler()
	defer h.Stop()

	clientEnd, targetEnd := net.Pipe()
	ac := &AgentTargetConn{ID: "g3-conn", ProxyID: "g3-p", Target: "127.0.0.1:80", Conn: targetEnd}
	covPutAgentConn(h, ac)

	done := make(chan struct{})
	go func() {
		h.readFromTarget(ac)
		close(done)
	}()

	// 对端关闭 → target Read 返回错误 → readFromTarget 退出。
	clientEnd.Close()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("readFromTarget did not exit after target close")
	}

	if _, ok := covGetAgentConn(h, "g3-conn"); ok {
		t.Error("readFromTarget must delete its conn from the map on exit (G3)")
	}
}

// TestG3ProxyDataWriteErrorRemovesConn 验证 G3：HandleProxyData 写目标失败
// 后删除 map 条目并关闭。
func TestG3ProxyDataWriteErrorRemovesConn(t *testing.T) {
	h := NewHandler()
	defer h.Stop()

	c1, c2 := net.Pipe()
	c1.Close() // 对端已关，写必失败
	ac := &AgentTargetConn{ID: "g3-w", ProxyID: "g3-p", Target: "127.0.0.1:80", Conn: c2}
	covPutAgentConn(h, ac)

	msg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "g3-p",
		"connId":  "g3-w",
		"data":    []byte("x"),
	})

	if err := h.HandleProxyData(msg); err == nil {
		t.Fatal("HandleProxyData() should fail on write error")
	}
	if _, ok := covGetAgentConn(h, "g3-w"); ok {
		t.Error("write-error path must delete conn from map (G3)")
	}
}

// TestG3RemoveConnKeepsNewSameID 验证 removeConn 指针校验：同 connID 被
// 覆盖重建后，旧协程的 defer 不得误删新条目。
func TestG3RemoveConnKeepsNewSameID(t *testing.T) {
	h := NewHandler()
	defer h.Stop()

	c1, c2 := net.Pipe()
	_ = c1
	_ = c2
	oldConn := &AgentTargetConn{ID: "g3-same", ProxyID: "g3-p", Conn: c2}
	newConn := &AgentTargetConn{ID: "g3-same", ProxyID: "g3-p", Conn: c2}
	covPutAgentConn(h, oldConn)
	covPutAgentConn(h, newConn) // 覆盖

	if removed := h.removeConn("g3-same", oldConn); removed {
		t.Error("removeConn must not delete a replaced same-ID entry")
	}
	if _, ok := covGetAgentConn(h, "g3-same"); !ok {
		t.Error("new same-ID entry must survive old conn removal")
	}
}
