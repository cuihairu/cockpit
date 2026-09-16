package proxy

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// covFailAcceptsListener 的 Accept 每次阻塞等待 trigger 中的错误，
// 用于确定性地驱动 acceptConnections 的 Accept 错误分支（无忙转）。
type covFailAcceptsListener struct {
	trigger chan error
	closed  atomic.Bool
}

func (l *covFailAcceptsListener) Accept() (net.Conn, error) {
	return nil, <-l.trigger
}

func (l *covFailAcceptsListener) Close() error {
	l.closed.Store(true)
	return nil
}

type covDummyAddr struct{}

func (covDummyAddr) Network() string { return "tcp" }
func (covDummyAddr) String() string  { return "127.0.0.1:0" }

func (l *covFailAcceptsListener) Addr() net.Addr { return covDummyAddr{} }

// TestCovManagerStartLoadError 覆盖 Start 中 ListEnabledProxies 失败分支。
func TestCovManagerStartLoadError(t *testing.T) {
	db := testDB(t)
	db.Close() // 此后所有查询失败

	m := NewManager(newMockServerInterface(), db)
	defer m.Stop()

	if err := m.Start(); err == nil {
		t.Fatal("Start() should fail when proxy list cannot be loaded")
	}
}

// TestCovManagerStartProxyListenError 覆盖 Start 循环中 StartProxy 失败仅记录日志的分支。
func TestCovManagerStartProxyListenError(t *testing.T) {
	db := testDB(t)

	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	occupied := blocker.Addr().(*net.TCPAddr).Port

	if err := db.CreateProxy(&storage.Proxy{
		ID:         "cov-busy",
		Name:       "busy",
		AgentID:    "cov-agent",
		ProxyType:  "tcp",
		RemotePort: occupied,
		Target:     "127.0.0.1:80",
		Enabled:    true,
	}); err != nil {
		t.Fatalf("CreateProxy: %v", err)
	}

	m := NewManager(newMockServerInterface(), db)
	defer m.Stop()

	// StartProxy 失败被记录但 Start 仍成功
	if err := m.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := m.GetProxyStatus("cov-busy"); err == nil {
		t.Error("failed proxy should not be registered")
	}
}

// TestCovManagerConnectionSendFailure 覆盖 handleConnection 中
// SendToAgent 失败后关闭客户端连接并从 map 移除的分支。
func TestCovManagerConnectionSendFailure(t *testing.T) {
	mockServer := newMockServerInterface()
	mockServer.sendToAgentFn = func(agentID string, msg *protocol.Message) error {
		return errors.New("cov send failure")
	}

	m := NewManager(mockServer, testDB(t))
	defer m.Stop()

	port := freePort(t)
	if err := m.StartProxy(&storage.Proxy{
		ID:         "cov-p1",
		Name:       "cov",
		AgentID:    "cov-agent",
		ProxyType:  "tcp",
		RemotePort: port,
		Target:     "127.0.0.1:80",
		Enabled:    true,
	}); err != nil {
		t.Fatalf("StartProxy() error = %v", err)
	}

	m.mu.RLock()
	proxy := m.proxies["cov-p1"]
	m.mu.RUnlock()

	client, err := net.DialTimeout("tcp", proxy.listener.Addr().String(), 3*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer client.Close()

	// SendToAgent 失败后连接应被关闭并移除
	deadline := time.Now().Add(3 * time.Second)
	removed := false
	for time.Now().Before(deadline) {
		proxy.mu.RLock()
		n := len(proxy.conns)
		proxy.mu.RUnlock()
		if n == 0 {
			removed = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !removed {
		t.Error("connection should be removed after SendToAgent failure")
	}
}

// TestCovAcceptConnectionsAcceptError 覆盖 acceptConnections 的
// Accept 错误但 ctx 未取消时的 log+continue 分支，以及 ctx 取消后的退出。
func TestCovAcceptConnectionsAcceptError(t *testing.T) {
	m := NewManager(newMockServerInterface(), testDB(t))
	defer m.Stop()

	trigger := make(chan error, 2)
	p := &Proxy{
		config:   &storage.Proxy{ID: "cov-acc", Name: "cov", AgentID: "a", ProxyType: "tcp"},
		conns:    make(map[string]*ProxyConn),
		listener: &covFailAcceptsListener{trigger: trigger},
	}
	ctx, cancel := context.WithCancel(m.ctx)
	p.ctx = ctx
	p.cancel = cancel

	done := make(chan struct{})
	go func() {
		m.acceptConnections(p)
		close(done)
	}()

	// 第一次错误：ctx 未取消 → log + continue → 再次阻塞在 Accept
	trigger <- errors.New("cov accept failure #1")
	time.Sleep(50 * time.Millisecond)

	// 取消 ctx 后的第二次错误 → return
	cancel()
	trigger <- errors.New("cov accept failure #2")

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("acceptConnections did not exit after ctx cancellation")
	}
}

// TestCovManagerReadFromClientSendError 覆盖 readFromClient 中 SendToAgent 失败分支。
func TestCovManagerReadFromClientSendError(t *testing.T) {
	mockServer := newMockServerInterface()
	mockServer.sendToAgentFn = func(agentID string, msg *protocol.Message) error {
		return errors.New("cov send failure")
	}

	m := NewManager(mockServer, testDB(t))
	defer m.Stop()

	if err := m.StartProxy(&storage.Proxy{
		ID: "cov-p5", Name: "cov", AgentID: "a", ProxyType: "tcp",
		RemotePort: 0, Target: "127.0.0.1:80", Enabled: true,
	}); err != nil {
		t.Fatalf("StartProxy() error = %v", err)
	}
	m.mu.RLock()
	proxy := m.proxies["cov-p5"]
	m.mu.RUnlock()

	client, serverEnd := net.Pipe()
	conn := &ProxyConn{ID: "cov-conn", ProxyID: "cov-p5", Conn: serverEnd, AgentID: "a"}

	done := make(chan struct{})
	go func() {
		m.readFromClient(proxy, conn)
		close(done)
	}()

	// 写入数据触发 SendToAgent 失败 → readFromClient 退出
	if _, err := client.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("readFromClient did not exit after send failure")
	}
	client.Close()
}

// TestCovManagerHandleProxyDataWriteError 覆盖写客户端失败分支。
func TestCovManagerHandleProxyDataWriteError(t *testing.T) {
	m := NewManager(newMockServerInterface(), testDB(t))
	defer m.Stop()

	if err := m.StartProxy(&storage.Proxy{
		ID: "cov-p2", Name: "cov", AgentID: "a", ProxyType: "tcp",
		RemotePort: 0, Target: "127.0.0.1:80", Enabled: true,
	}); err != nil {
		t.Fatalf("StartProxy() error = %v", err)
	}
	m.mu.RLock()
	proxy := m.proxies["cov-p2"]
	m.mu.RUnlock()

	// 对端关闭后写 pipe 端必然失败
	c1, c2 := net.Pipe()
	c1.Close()
	conn := &ProxyConn{ID: "cov-conn", ProxyID: "cov-p2", Conn: c2}

	proxy.mu.Lock()
	proxy.conns["cov-conn"] = conn
	proxy.mu.Unlock()

	if err := m.HandleProxyData("cov-p2", "cov-conn", []byte("x")); err == nil {
		t.Fatal("HandleProxyData() should fail when client conn is closed")
	}
	if !conn.closed.Load() {
		t.Error("connection should be closed after write failure")
	}
}

// TestCovManagerStatusWithConns 覆盖 GetProxyStatus/GetAllStatus 的连接列表构建循环。
func TestCovManagerStatusWithConns(t *testing.T) {
	m := NewManager(newMockServerInterface(), testDB(t))
	defer m.Stop()

	if err := m.StartProxy(&storage.Proxy{
		ID: "cov-p4", Name: "cov-name", AgentID: "cov-agent", ProxyType: "tcp",
		RemotePort: 0, Target: "127.0.0.1:8080", Enabled: true,
	}); err != nil {
		t.Fatalf("StartProxy() error = %v", err)
	}
	m.mu.RLock()
	proxy := m.proxies["cov-p4"]
	m.mu.RUnlock()

	c1, c2 := net.Pipe()
	defer c1.Close()
	proxy.mu.Lock()
	proxy.conns["cov-conn"] = &ProxyConn{
		ID:       "cov-conn",
		ProxyID:  "cov-p4",
		Conn:     c2,
		AgentID:  "cov-agent",
		Created:  time.Now(),
		LastRead: time.Now(),
	}
	proxy.mu.Unlock()

	status, err := m.GetProxyStatus("cov-p4")
	if err != nil {
		t.Fatalf("GetProxyStatus() error = %v", err)
	}
	if status["connCount"] != 1 {
		t.Errorf("connCount = %v, want 1", status["connCount"])
	}
	conns := status["connections"].([]map[string]interface{})
	if len(conns) != 1 || conns[0]["id"] != "cov-conn" {
		t.Errorf("connections = %v, want one cov-conn entry", conns)
	}

	all := m.GetAllStatus()
	if len(all) != 1 {
		t.Fatalf("GetAllStatus() = %v, want 1 entry", all)
	}
	if all[0]["connCount"] != 1 {
		t.Errorf("connCount = %v, want 1", all[0]["connCount"])
	}
}
