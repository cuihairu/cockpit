package server

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// Guacamole 目标转发（guac_relay.go）：guacd 拨回环中继，字节流经既有
// proxy_new/proxy_data 管线到 agent，agent 裸拨目标。测试用假 agent 捕获
// 出站消息并回注（走真实分发路径 handleProxyData/Close/Error）。

// relayCapture 假 agent：记录出站消息
type relayCapture struct {
	mu     sync.Mutex
	news   []protocol.ProxyNewPayload
	datas  [][]byte
	closes []protocol.ProxyClosePayload
}

func (c *relayCapture) send(_ string, msg *protocol.Message) error {
	switch msg.Type {
	case protocol.MessageTypeProxyNew:
		p, _ := protocol.DecodeProxyNew(msg)
		c.mu.Lock()
		c.news = append(c.news, p)
		c.mu.Unlock()
	case protocol.MessageTypeProxyData:
		p, _ := protocol.DecodeProxyData(msg)
		c.mu.Lock()
		c.datas = append(c.datas, p.Data)
		c.mu.Unlock()
	case protocol.MessageTypeProxyClose:
		p, _ := protocol.DecodeProxyClose(msg)
		c.mu.Lock()
		c.closes = append(c.closes, p)
		c.mu.Unlock()
	}
	return nil
}

func (c *relayCapture) newsCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.news)
}

func (c *relayCapture) datasCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.datas)
}

func (c *relayCapture) closesCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.closes)
}

func (c *relayCapture) lastNew() protocol.ProxyNewPayload {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.news[len(c.news)-1]
}

func waitForCond(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within 2s")
}

// 全链路：guacd 侧拨入 → proxy_new/proxy_data 出站 → 假 agent 回注数据 →
// guacd 侧收到 → guacd 侧断开 → proxy_close 出站
func TestGuacRelayRoundTrip(t *testing.T) {
	cap := &relayCapture{}
	relay, addr, err := startGuacRelay("agent-1", "guac:sess-rt", "10.0.0.5:22", cap.send)
	if err != nil {
		t.Fatalf("startGuacRelay: %v", err)
	}
	defer relay.Close()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial relay: %v", err)
	}
	if _, err := conn.Write([]byte("hello-agent")); err != nil {
		t.Fatalf("write: %v", err)
	}

	// agent 收到 proxy_new：目标就是票据里的 host:port，connId 归属本 relay
	waitForCond(t, func() bool { return cap.newsCount() > 0 })
	n := cap.lastNew()
	if n.Target != "10.0.0.5:22" || n.ProxyID != "guac:sess-rt" {
		t.Fatalf("proxy_new payload: %+v", n)
	}
	if len(n.ConnID) == 0 || !hasPrefix(n.ConnID, "guac:sess-rt-") {
		t.Fatalf("connId not namespaced: %q", n.ConnID)
	}

	// agent 收到 guacd 方向数据
	waitForCond(t, func() bool { return cap.datasCount() == 1 })
	if !bytes.Equal(cap.datas[0], []byte("hello-agent")) {
		t.Fatalf("relay data garbled: %q", cap.datas[0])
	}

	// 假 agent 回注（走真实分发路径）→ guacd 侧收到
	echo := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": n.ProxyID,
		"connId":  n.ConnID,
		"data":    []byte("hello-guacd"),
	})
	s := &Server{}
	s.handleProxyData(&Agent{ID: "agent-1"}, echo)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	read, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read echoed data: %v", err)
	}
	if !bytes.Equal(buf[:read], []byte("hello-guacd")) {
		t.Fatalf("echoed data garbled: %q", buf[:read])
	}

	// guacd 侧断开 → agent 收到 proxy_close
	conn.Close()
	waitForCond(t, func() bool { return cap.closesCount() > 0 })
	if got := cap.closes[0].ConnID; got != n.ConnID {
		t.Fatalf("proxy_close connId mismatch: %q vs %q", got, n.ConnID)
	}
}

// agent 侧目标断开（proxy_close）→ guacd 侧连接被关
func TestGuacRelayAgentCloseClosesGuacdSide(t *testing.T) {
	cap := &relayCapture{}
	relay, addr, err := startGuacRelay("agent-1", "guac:sess-ac", "10.0.0.5:22", cap.send)
	if err != nil {
		t.Fatalf("startGuacRelay: %v", err)
	}
	defer relay.Close()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial relay: %v", err)
	}
	_, _ = conn.Write([]byte("x")) // 触发 handleConn：注册 + proxy_new
	waitForCond(t, func() bool { return cap.newsCount() > 0 })
	connID := cap.lastNew().ConnID

	s := &Server{}
	s.handleProxyClose(&Agent{ID: "agent-1"}, protocol.NewMessage(
		protocol.MessageTypeProxyClose, map[string]interface{}{
			"proxyId": "guac:sess-ac",
			"connId":  connID,
			"reason":  "target closed",
		}))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(make([]byte, 16)); err == nil {
		t.Fatalf("guacd-side conn should be closed after agent close")
	}
}

// agent 拨目标失败（proxy_error）：带 connId 只拆那条；不带 connId 拆整条
// 中继（listener 停收、注册表摘除）
func TestGuacRelayDialError(t *testing.T) {
	// 带 connId：只拆对应连接
	cap := &relayCapture{}
	relay, addr, err := startGuacRelay("agent-1", "guac:sess-de1", "10.0.0.5:22", cap.send)
	if err != nil {
		t.Fatalf("startGuacRelay: %v", err)
	}
	defer relay.Close()
	conn, _ := net.Dial("tcp", addr)
	_, _ = conn.Write([]byte("x"))
	waitForCond(t, func() bool { return cap.newsCount() > 0 })
	connID := cap.lastNew().ConnID

	s := &Server{}
	s.handleProxyError(&Agent{ID: "agent-1"}, protocol.NewMessage(
		protocol.MessageTypeProxyError, map[string]interface{}{
			"proxyId": "guac:sess-de1",
			"connId":  connID,
			"error":   "dial tcp 10.0.0.5:22: refused",
		}))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(make([]byte, 16)); err == nil {
		t.Fatalf("conn should be closed after dial error")
	}

	// 不带 connId：整条中继拆掉
	cap2 := &relayCapture{}
	relay2, addr2, err := startGuacRelay("agent-1", "guac:sess-de2", "10.0.0.5:22", cap2.send)
	if err != nil {
		t.Fatalf("startGuacRelay: %v", err)
	}
	defer relay2.Close()
	conn2, _ := net.Dial("tcp", addr2)
	_, _ = conn2.Write([]byte("x"))
	waitForCond(t, func() bool { return cap2.newsCount() > 0 })

	s.handleProxyError(&Agent{ID: "agent-1"}, protocol.NewMessage(
		protocol.MessageTypeProxyError, map[string]interface{}{
			"proxyId": "guac:sess-de2",
			"error":   "agent offline",
		}))
	waitForCond(t, func() bool {
		return guacRelayLookup("guac:sess-de2") == nil
	})
	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn2.Read(make([]byte, 16)); err == nil {
		t.Fatalf("conn should be closed after relay-wide error")
	}
}

// relay.Close：listener 停收、在册连接逐条通知 agent 拆链、注册表摘除、幂等
func TestGuacRelayClose(t *testing.T) {
	cap := &relayCapture{}
	relay, addr, err := startGuacRelay("agent-1", "guac:sess-cl", "10.0.0.5:22", cap.send)
	if err != nil {
		t.Fatalf("startGuacRelay: %v", err)
	}
	conn, _ := net.Dial("tcp", addr)
	_, _ = conn.Write([]byte("x"))
	waitForCond(t, func() bool { return cap.newsCount() > 0 })

	relay.Close()
	waitForCond(t, func() bool { return cap.closesCount() > 0 })
	if got := cap.closes[0].Reason; got != "session closed" {
		t.Fatalf("close reason: %q", got)
	}
	if guacRelayLookup("guac:sess-cl") != nil {
		t.Fatalf("relay not removed from registry")
	}
	// listener 已关：新拨入被拒
	if c, err := net.Dial("tcp", addr); err == nil {
		c.Close()
		t.Fatalf("listener should be closed")
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(make([]byte, 16)); err == nil {
		t.Fatalf("open conn should be closed on relay close")
	}
	relay.Close() // 幂等，不得 panic/重复发 close
	closes := cap.closesCount()
	time.Sleep(50 * time.Millisecond)
	if cap.closesCount() != closes {
		t.Fatalf("duplicate proxy_close after idempotent Close")
	}
}

// 未知 relay/conn 的回程消息：静默拒绝（错误分支不 panic）
func TestGuacRelayDeliverUnknown(t *testing.T) {
	if err := guacRelayDeliver("guac:nope", "conn-x", []byte("d")); err == nil {
		t.Fatalf("deliver to unknown relay should error")
	}
	cap := &relayCapture{}
	relay, _, err := startGuacRelay("a", "guac:unknown-conn", "t:1", cap.send)
	if err != nil {
		t.Fatalf("startGuacRelay: %v", err)
	}
	defer relay.Close()
	if err := guacRelayDeliver("guac:unknown-conn", "no-such-conn", []byte("d")); err == nil {
		t.Fatalf("deliver to unknown conn should error")
	}
}
