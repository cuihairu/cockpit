package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// mockServerInterface 模拟 ServerInterface
type mockServerInterface struct {
	mu            sync.Mutex
	agents        map[string]*mockAgentConn
	sendToAgentFn func(agentID string, msg *protocol.Message) error
}

type mockAgentConn struct {
	agentID   string
	messages  []*protocol.Message
	mu        sync.Mutex
	closed    atomic.Bool
	closeChan chan struct{}
}

func newMockServerInterface() *mockServerInterface {
	return &mockServerInterface{
		agents: make(map[string]*mockAgentConn),
		sendToAgentFn: func(agentID string, msg *protocol.Message) error {
			return nil
		},
	}
}

func (m *mockServerInterface) SendToAgent(agentID string, msg *protocol.Message) error {
	if m.sendToAgentFn != nil {
		return m.sendToAgentFn(agentID, msg)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if agent, exists := m.agents[agentID]; exists {
		agent.mu.Lock()
		agent.messages = append(agent.messages, msg)
		agent.mu.Unlock()
		return nil
	}

	return errors.New("agent not found")
}

func (m *mockServerInterface) GetAgentConn(agentID string) (AgentConn, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if agent, exists := m.agents[agentID]; exists {
		return agent, true
	}
	return nil, false
}

func (m *mockServerInterface) addAgent(agentID string) *mockAgentConn {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent := &mockAgentConn{
		agentID:   agentID,
		closeChan: make(chan struct{}),
	}
	m.agents[agentID] = agent
	return agent
}

func (m *mockAgentConn) SendMessage(msg *protocol.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed.Load() {
		return io.EOF
	}

	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockAgentConn) AgentID() string {
	return m.agentID
}

func (m *mockAgentConn) Close() {
	m.closed.Store(true)
	close(m.closeChan)
}

// ============ Helper ============

func testDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(storage.Config{Path: dir + "/test.db"})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ============ Manager Tests ============

func TestNewManager(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)

	manager := NewManager(mockServer, db)

	if manager == nil {
		t.Fatal("NewManager() should not return nil")
	}
	if manager.proxies == nil {
		t.Error("proxies map should be initialized")
	}
	if manager.ctx == nil {
		t.Error("context should be initialized")
	}
	if manager.cancel == nil {
		t.Error("cancel function should be initialized")
	}
	if manager.running.Load() {
		t.Error("manager should not be running initially")
	}
}

func TestManagerStartStop(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	err := manager.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !manager.running.Load() {
		t.Error("manager should be running after Start()")
	}

	err = manager.Start()
	if err == nil {
		t.Error("Start() twice should return error")
	}

	manager.Stop()
	if manager.running.Load() {
		t.Error("manager should not be running after Stop()")
	}

	manager.Stop()
}

func TestManagerStopWithoutStart(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	manager.Stop()
	if manager.running.Load() {
		t.Error("manager should not be running")
	}
}

// ============ ProxyConn Tests ============

func TestProxyConnClose(t *testing.T) {
	conn1, conn2 := net.Pipe()

	proxyConn := &ProxyConn{
		ID:      "test-conn",
		ProxyID: "proxy-1",
		Conn:    conn1,
		AgentID: "agent-1",
		Created: time.Now(),
	}

	err := proxyConn.Close()
	if err != nil {
		t.Errorf("First Close() error = %v", err)
	}
	if !proxyConn.closed.Load() {
		t.Error("connection should be marked as closed")
	}

	err = proxyConn.Close()
	if err != nil {
		t.Errorf("Second Close() should not error, got %v", err)
	}

	conn2.Close()
}

func TestProxyConnFields(t *testing.T) {
	conn1, conn2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()

	now := time.Now()
	proxyConn := &ProxyConn{
		ID:       "conn-1",
		ProxyID:  "proxy-1",
		Conn:     conn1,
		AgentID:  "agent-1",
		Created:  now,
		LastRead: now,
	}

	if proxyConn.ID != "conn-1" {
		t.Errorf("ID = %v, want conn-1", proxyConn.ID)
	}
	if proxyConn.AgentID != "agent-1" {
		t.Errorf("AgentID = %v, want agent-1", proxyConn.AgentID)
	}
	if proxyConn.closed.Load() {
		t.Error("connection should not be closed initially")
	}
}

// ============ GetProxyStatus Tests ============

func TestGetProxyStatusNotFound(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	_, err := manager.GetProxyStatus("non-existent")
	if err == nil {
		t.Error("GetProxyStatus() should return error for non-existent proxy")
	}
}

func TestGetAllStatusEmpty(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	status := manager.GetAllStatus()
	if status == nil {
		t.Error("GetAllStatus() should not return nil")
	}
	if len(status) != 0 {
		t.Errorf("GetAllStatus() length = %d, want 0", len(status))
	}
}

// ============ HandleProxyData Tests ============

func TestHandleProxyDataProxyNotFound(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	err := manager.HandleProxyData("non-existent", "conn-1", []byte("test"))
	if err == nil {
		t.Error("HandleProxyData() should return error for non-existent proxy")
	}
}

func TestHandleProxyDataConnNotFound(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "test-proxy",
		Name:       "Test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}
	if err := manager.StartProxy(proxyConfig); err != nil {
		t.Fatal(err)
	}
	defer manager.StopProxy(proxyConfig.ID)

	err := manager.HandleProxyData("test-proxy", "non-existent-conn", []byte("test"))
	if err == nil {
		t.Error("HandleProxyData() should return error for non-existent connection")
	}
}

func TestHandleProxyDataWithConn(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "data-proxy",
		Name:       "Data Test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}
	if err := manager.StartProxy(proxyConfig); err != nil {
		t.Fatal(err)
	}
	defer manager.StopProxy(proxyConfig.ID)

	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	proxyConn := &ProxyConn{
		ID:      "data-conn",
		ProxyID: "data-proxy",
		Conn:    conn1,
		AgentID: "agent-1",
		Created: time.Now(),
	}

	manager.mu.RLock()
	proxy := manager.proxies["data-proxy"]
	manager.mu.RUnlock()

	proxy.mu.Lock()
	proxy.conns["data-conn"] = proxyConn
	proxy.mu.Unlock()

	// Start reader first to avoid pipe blocking
	testData := []byte("hello world")
	readCh := make(chan string, 1)
	go func() {
		buf := make([]byte, len(testData))
		conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn2.Read(buf)
		if err != nil {
			readCh <- ""
			return
		}
		readCh <- string(buf[:n])
	}()

	err := manager.HandleProxyData("data-proxy", "data-conn", testData)
	if err != nil {
		t.Errorf("HandleProxyData() error = %v", err)
	}

	select {
	case data := <-readCh:
		if data != string(testData) {
			t.Errorf("Data mismatch: got %q, want %q", data, testData)
		}
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for data")
	}
}

func TestManagerHandleProxyDataClosedConn(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "closed-proxy",
		Name:       "Closed Test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}
	if err := manager.StartProxy(proxyConfig); err != nil {
		t.Fatal(err)
	}
	defer manager.StopProxy(proxyConfig.ID)

	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	proxyConn := &ProxyConn{
		ID:      "closed-conn",
		ProxyID: "closed-proxy",
		Conn:    conn1,
		AgentID: "agent-1",
		Created: time.Now(),
	}
	proxyConn.closed.Store(true)

	manager.mu.RLock()
	proxy := manager.proxies["closed-proxy"]
	manager.mu.RUnlock()

	proxy.mu.Lock()
	proxy.conns["closed-conn"] = proxyConn
	proxy.mu.Unlock()

	err := manager.HandleProxyData("closed-proxy", "closed-conn", []byte("test"))
	if err == nil {
		t.Error("HandleProxyData() should return error for closed connection")
	}
}

// ============ HandleProxyClose Tests ============

func TestHandleProxyCloseProxyNotFound(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	manager.HandleProxyClose("non-existent", "conn-1", "test")
}

func TestManagerHandleProxyCloseExisting(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "close-proxy",
		Name:       "Close Test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}
	if err := manager.StartProxy(proxyConfig); err != nil {
		t.Fatal(err)
	}
	defer manager.StopProxy(proxyConfig.ID)

	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	proxyConn := &ProxyConn{
		ID:      "close-conn",
		ProxyID: "close-proxy",
		Conn:    conn1,
		AgentID: "agent-1",
		Created: time.Now(),
	}

	manager.mu.RLock()
	proxy := manager.proxies["close-proxy"]
	manager.mu.RUnlock()

	proxy.mu.Lock()
	proxy.conns["close-conn"] = proxyConn
	proxy.mu.Unlock()

	manager.HandleProxyClose("close-proxy", "close-conn", "test reason")

	proxy.mu.RLock()
	_, exists := proxy.conns["close-conn"]
	proxy.mu.RUnlock()

	if exists {
		t.Error("Connection should be removed after HandleProxyClose")
	}
	if !proxyConn.closed.Load() {
		t.Error("Connection should be closed after HandleProxyClose")
	}
}

// ============ StopProxy Tests ============

func TestStopProxyNotFound(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	err := manager.StopProxy("non-existent")
	if err == nil {
		t.Error("StopProxy() should return error for non-existent proxy")
	}
}

func TestStopProxyRunning(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "stop-proxy",
		Name:       "Stop Test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}
	if err := manager.StartProxy(proxyConfig); err != nil {
		t.Fatal(err)
	}

	err := manager.StopProxy("stop-proxy")
	if err != nil {
		t.Errorf("StopProxy() error = %v", err)
	}

	// Proxy should be removed
	manager.mu.RLock()
	_, exists := manager.proxies["stop-proxy"]
	manager.mu.RUnlock()
	if exists {
		t.Error("Proxy should be removed after StopProxy()")
	}
}

// ============ ReloadProxy Tests ============

func TestReloadProxyDisable(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "reload-proxy",
		Name:       "Reload Test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    false,
	}

	err := manager.ReloadProxy(proxyConfig)
	if err != nil {
		t.Errorf("ReloadProxy() with disabled proxy error = %v", err)
	}
}

func TestReloadProxyEnable(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "reload-enable",
		Name:       "Reload Enable",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}

	err := manager.ReloadProxy(proxyConfig)
	if err != nil {
		t.Errorf("ReloadProxy() with enabled proxy error = %v", err)
	}

	manager.StopProxy("reload-enable")
}

// ============ Proxy Stop Tests ============

func TestProxyStop(t *testing.T) {
	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())

	proxy := &Proxy{
		config: &storage.Proxy{
			ID:         "test-proxy",
			Name:       "Test",
			RemotePort: 8080,
		},
		listener: listener,
		conns:     make(map[string]*ProxyConn),
		ctx:       ctx,
		cancel:    cancel,
	}

	proxyConn := &ProxyConn{
		ID:      "conn-1",
		ProxyID: "test-proxy",
		Conn:    conn1,
		AgentID: "agent-1",
		Created: time.Now(),
	}
	proxy.conns["conn-1"] = proxyConn

	proxy.Stop()
}

func TestProxyStopWithNilListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	proxy := &Proxy{
		config:   &storage.Proxy{ID: "test-proxy"},
		listener: nil,
		conns:    make(map[string]*ProxyConn),
		ctx:      ctx,
		cancel:   cancel,
	}

	proxy.Stop()
}

func TestProxyStopWithConnections(t *testing.T) {
	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	ctx, cancel := context.WithCancel(context.Background())

	proxyConn := &ProxyConn{
		ID:      "conn-1",
		ProxyID: "proxy-1",
		Conn:    conn1,
		AgentID: "agent-1",
		Created: time.Now(),
	}

	proxy := &Proxy{
		config:   &storage.Proxy{ID: "test-proxy"},
		listener: nil,
		conns:    map[string]*ProxyConn{"conn-1": proxyConn},
		ctx:      ctx,
		cancel:   cancel,
	}

	proxy.Stop()

	if !proxyConn.closed.Load() {
		t.Error("Connection should be closed after proxy stop")
	}
	if len(proxy.conns) != 0 {
		t.Error("Connections should be cleared after proxy stop")
	}
}

// ============ StartProxy Tests ============

func TestStartProxyDuplicate(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "dup-proxy",
		Name:       "Dup Test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}

	err := manager.StartProxy(proxyConfig)
	if err != nil {
		t.Fatalf("StartProxy() first call error = %v", err)
	}
	defer manager.StopProxy(proxyConfig.ID)

	err = manager.StartProxy(proxyConfig)
	if err == nil {
		t.Error("StartProxy() second call should return error")
	}
}

func TestStartProxyInvalidPort(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "invalid-proxy",
		Name:       "Invalid",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: -1,
		Target:     "localhost:8080",
		Enabled:    true,
	}

	err := manager.StartProxy(proxyConfig)
	if err == nil {
		t.Error("StartProxy() with invalid port should return error")
	}
}

// ============ Status Tests ============

func TestGetProxyStatusRunningProxy(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "status-proxy",
		Name:       "Status Test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}
	if err := manager.StartProxy(proxyConfig); err != nil {
		t.Fatal(err)
	}
	defer manager.StopProxy(proxyConfig.ID)

	status, err := manager.GetProxyStatus(proxyConfig.ID)
	if err != nil {
		t.Fatalf("GetProxyStatus() error = %v", err)
	}
	if status["name"] != "Status Test" {
		t.Errorf("name = %v, want Status Test", status["name"])
	}
	if status["status"] != "running" {
		t.Errorf("status = %v, want running", status["status"])
	}
}

func TestGetAllStatusWithProxies(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	for i := 1; i <= 2; i++ {
		proxyConfig := &storage.Proxy{
			ID:         fmt.Sprintf("all-proxy-%d", i),
			Name:       fmt.Sprintf("Proxy %d", i),
			AgentID:    "agent-1",
			ProxyType:  "tcp",
			RemotePort: 0,
			Target:     "localhost:8080",
			Enabled:    true,
		}
		if err := manager.StartProxy(proxyConfig); err != nil {
			t.Fatal(err)
		}
		defer manager.StopProxy(proxyConfig.ID)
	}

	status := manager.GetAllStatus()
	if len(status) != 2 {
		t.Errorf("GetAllStatus() returned %d items, want 2", len(status))
	}
}

// ============ SendToAgent Tests ============

func TestManagerSendToAgent(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	var receivedMsg *protocol.Message
	mockServer.sendToAgentFn = func(agentID string, msg *protocol.Message) error {
		if agentID == "agent-1" {
			receivedMsg = msg
		}
		return nil
	}

	msg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"test": "data",
	})

	err := manager.SendToAgent("agent-1", msg)
	if err != nil {
		t.Errorf("SendToAgent() error = %v", err)
	}
	if receivedMsg == nil {
		t.Error("agent should receive message")
	}
}

// ============ AgentConn Interface Tests ============

func TestMockAgentConn(t *testing.T) {
	agent := &mockAgentConn{
		agentID:   "test-agent",
		closeChan: make(chan struct{}),
	}

	if agent.AgentID() != "test-agent" {
		t.Errorf("AgentID() = %v, want test-agent", agent.AgentID())
	}

	msg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"data": []byte("test"),
	})

	err := agent.SendMessage(msg)
	if err != nil {
		t.Errorf("SendMessage() error = %v", err)
	}

	agent.mu.Lock()
	if len(agent.messages) != 1 {
		t.Errorf("messages length = %d, want 1", len(agent.messages))
	}
	agent.mu.Unlock()

	agent.Close()
	if !agent.closed.Load() {
		t.Error("agent should be closed after Close()")
	}

	err = agent.SendMessage(msg)
	if err != io.EOF {
		t.Errorf("SendMessage() on closed agent should return EOF, got %v", err)
	}
}

// ============ Context Tests ============

func TestManagerContextCancellation(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	err := manager.Start()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-manager.ctx.Done():
		t.Error("context should not be cancelled yet")
	default:
	}

	manager.Stop()

	select {
	case <-manager.ctx.Done():
	default:
		t.Error("context should be cancelled after Stop()")
	}
}

// ============ Concurrent Access Tests ============

func TestManagerConcurrentStatus(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			manager.GetAllStatus()
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// ============ Cleanup Tests ============

func TestCleanupIdleConnections(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "cleanup-proxy",
		Name:       "Cleanup Test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}
	if err := manager.StartProxy(proxyConfig); err != nil {
		t.Fatal(err)
	}
	defer manager.StopProxy(proxyConfig.ID)

	manager.cleanupIdleConnections()
}

func TestCleanupIdleConnectionsRemovesStale(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "stale-proxy",
		Name:       "Stale Test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}
	if err := manager.StartProxy(proxyConfig); err != nil {
		t.Fatal(err)
	}
	defer manager.StopProxy(proxyConfig.ID)

	// Inject a stale connection
	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	staleConn := &ProxyConn{
		ID:       "stale-conn",
		ProxyID:  "stale-proxy",
		Conn:     conn1,
		AgentID:  "agent-1",
		Created:  time.Now().Add(-10 * time.Minute),
		LastRead: time.Now().Add(-10 * time.Minute),
	}

	manager.mu.RLock()
	proxy := manager.proxies["stale-proxy"]
	manager.mu.RUnlock()

	proxy.mu.Lock()
	proxy.conns["stale-conn"] = staleConn
	proxy.mu.Unlock()

	manager.cleanupIdleConnections()

	proxy.mu.RLock()
	_, exists := proxy.conns["stale-conn"]
	proxy.mu.RUnlock()

	if exists {
		t.Error("Stale connection should be removed by cleanup")
	}
}

// ============ Running State Tests ============

func TestManagerRunningState(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	if manager.running.Load() {
		t.Error("Manager should not be running initially")
	}

	err := manager.Start()
	if err != nil {
		t.Fatal(err)
	}
	if !manager.running.Load() {
		t.Error("Manager should be running after Start()")
	}

	err = manager.Start()
	if err == nil {
		t.Error("Second Start() should return error")
	}

	manager.Stop()
	if manager.running.Load() {
		t.Error("Manager should not be running after Stop()")
	}
}

func TestManagerStopProxiesWithoutStart(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "nostart-proxy",
		Name:       "NoStart",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}
	db.CreateProxy(proxyConfig)

	err := manager.StopProxy("nostart-proxy")
	if err == nil {
		t.Error("StopProxy() should return error for proxy that's not running")
	}
}
