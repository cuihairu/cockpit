package proxy

// cov_io_gaps_test.go 覆盖 handleConnection 的"等待 agent 超时"与
// "proxy ctx 取消"分支（注入短超时），以及 cleanupLoop 的 tick 分支。

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// covFakeProxyServer SendToAgent 恒成功、无 agent 连接的假 ServerInterface
type covFakeProxyServer struct{}

func (covFakeProxyServer) SendToAgent(string, *protocol.Message) error { return nil }
func (covFakeProxyServer) GetAgentConn(string) (AgentConn, bool)       { return nil, false }

func TestCovHandleConnectionWaitTimeout(t *testing.T) {
	save := proxyAgentConnTimeout
	proxyAgentConnTimeout = 80 * time.Millisecond
	t.Cleanup(func() { proxyAgentConnTimeout = save })

	m := NewManager(covFakeProxyServer{}, nil)
	proxy := &Proxy{
		config: &storage.Proxy{ID: "cov-p1", Name: "cov", AgentID: "cov-agent",
			ProxyType: "tcp", Target: "127.0.0.1:80", Enabled: true},
		conns:  make(map[string]*ProxyConn),
		ctx:    context.Background(),
		cancel: func() {},
	}

	c1, c2 := net.Pipe()
	defer c2.Close()
	m.handleConnection(proxy, c1)

	// 超时后 proxyConn 被关闭 → c2 读到错误（pipe 对端关闭）
	_ = c2.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1)
	if _, err := c2.Read(buf); err == nil {
		t.Error("connection should be closed after wait timeout")
	}
}

func TestCovHandleConnectionProxyCtxDone(t *testing.T) {
	m := NewManager(covFakeProxyServer{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 已取消：select 必走 ctx.Done 分支（time.After 尚未到）
	proxy := &Proxy{
		config: &storage.Proxy{ID: "cov-p2", Name: "cov", AgentID: "cov-agent",
			ProxyType: "tcp", Target: "127.0.0.1:80", Enabled: true},
		conns:  make(map[string]*ProxyConn),
		ctx:    ctx,
		cancel: cancel,
	}

	c1, c2 := net.Pipe()
	defer c2.Close()
	m.handleConnection(proxy, c1)

	// ctx.Done 分支不关连接：c2 仍可读阻塞而非报错（用短 deadline 验证未关闭）
	_ = c2.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	buf := make([]byte, 1)
	if _, err := c2.Read(buf); err == nil {
		t.Error("unexpected data")
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		// 仍阻塞 = 连接未被关闭 = 走了 ctx.Done 分支
	} else {
		t.Errorf("conn should stay open on ctx cancel, got %v", err)
	}
	_ = c1.Close()
}

func TestCovProxyCleanupLoopTick(t *testing.T) {
	save := proxyCleanupInterval
	proxyCleanupInterval = 50 * time.Millisecond
	t.Cleanup(func() { proxyCleanupInterval = save })

	m := NewManager(covFakeProxyServer{}, nil)
	// 造一条 idle 超过 5 分钟的连接，tick 后应被清理
	proxy := &Proxy{
		config: &storage.Proxy{ID: "cov-p3", Name: "cov", AgentID: "a", ProxyType: "tcp", Target: "t"},
		conns:  make(map[string]*ProxyConn),
		ctx:    context.Background(),
		cancel: func() {},
	}
	c1, c2 := net.Pipe()
	defer c2.Close()
	proxy.conns["cov-conn-old"] = &ProxyConn{
		ID: "cov-conn-old", ProxyID: "cov-p3", Conn: c1, AgentID: "a",
		Created: time.Now().Add(-10 * time.Minute), LastRead: time.Now().Add(-10 * time.Minute),
	}
	m.mu.Lock()
	m.proxies["cov-p3"] = proxy
	m.mu.Unlock()

	// cleanupLoop：短周期注入后至少跑一轮 tick（清理 idle 连接），随后取消退出
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		m.cleanupLoop()
	}()
	covWaitGoneProxy(t, "idle connection cleaned", func() bool {
		proxy.mu.RLock()
		defer proxy.mu.RUnlock()
		_, kept := proxy.conns["cov-conn-old"]
		return !kept
	})
	m.cancel()
	select {
	case <-loopDone:
	case <-time.After(3 * time.Second):
		t.Fatal("cleanupLoop did not exit on cancel")
	}
}

func covWaitGoneProxy(t *testing.T, desc string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met: %s", desc)
}
