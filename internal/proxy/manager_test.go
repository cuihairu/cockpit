package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
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

// ============ Manager Tests ============

func TestNewManager(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

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
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	// Start
	err = manager.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !manager.running.Load() {
		t.Error("manager should be running after Start()")
	}

	// Start again should fail
	err = manager.Start()
	if err == nil {
		t.Error("Start() twice should return error")
	}

	// Stop
	manager.Stop()

	if manager.running.Load() {
		t.Error("manager should not be running after Stop()")
	}

	// Stop again should be safe
	manager.Stop()
}

func TestManagerStopWithoutStart(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	// Stop without Start should be safe
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

	// First close
	err := proxyConn.Close()
	if err != nil {
		t.Errorf("First Close() error = %v", err)
	}

	if !proxyConn.closed.Load() {
		t.Error("connection should be marked as closed")
	}

	// Second close should be safe
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

	if proxyConn.ProxyID != "proxy-1" {
		t.Errorf("ProxyID = %v, want proxy-1", proxyConn.ProxyID)
	}

	if proxyConn.AgentID != "agent-1" {
		t.Errorf("AgentID = %v, want agent-1", proxyConn.AgentID)
	}

	if proxyConn.Created.IsZero() {
		t.Error("Created should be set")
	}

	if proxyConn.closed.Load() {
		t.Error("connection should not be closed initially")
	}
}

// ============ GetProxyStatus Tests ============

func TestGetProxyStatusNotFound(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	_, err = manager.GetProxyStatus("non-existent")
	if err == nil {
		t.Error("GetProxyStatus() should return error for non-existent proxy")
	}
}

func TestGetAllStatusEmpty(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

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
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	err = manager.HandleProxyData("non-existent", "conn-1", []byte("test"))
	if err == nil {
		t.Error("HandleProxyData() should return error for non-existent proxy")
	}
}

// ============ HandleProxyClose Tests ============

func TestHandleProxyCloseProxyNotFound(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	// Should not panic
	manager.HandleProxyClose("non-existent", "conn-1", "test")
}

// ============ StopProxy Tests ============

func TestStopProxyNotFound(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	err = manager.StopProxy("non-existent")
	if err == nil {
		t.Error("StopProxy() should return error for non-existent proxy")
	}
}

func TestStopProxyNotRunning(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	// Create a proxy config
	proxyConfig := &storage.Proxy{
		ID:         "test-proxy",
		Name:       "Test Proxy",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0, // Use random port
		Target:     "localhost:8080",
		Enabled:    true,
	}
	db.CreateProxy(proxyConfig)

	// Don't start the manager, just try to stop a proxy
	err = manager.StopProxy("test-proxy")
	if err == nil {
		t.Error("StopProxy() should return error for proxy that's not running")
	}
}

// ============ ReloadProxy Tests ============

func TestReloadProxyDisable(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	// Create a disabled proxy config
	proxyConfig := &storage.Proxy{
		ID:         "test-proxy",
		Name:       "Test Proxy",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    false,
	}
	db.CreateProxy(proxyConfig)

	// Reload with disabled proxy
	err = manager.ReloadProxy(proxyConfig)
	if err != nil {
		t.Errorf("ReloadProxy() with disabled proxy error = %v", err)
	}
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

	// Add a connection
	proxyConn := &ProxyConn{
		ID:      "conn-1",
		ProxyID: "test-proxy",
		Conn:    conn1,
		AgentID: "agent-1",
		Created: time.Now(),
	}
	proxy.conns["conn-1"] = proxyConn

	// Stop the proxy
	proxy.Stop()

	// Connection should be closed
	if proxyConn.closed.Load() {
		// Connection should be closed
	}
}

func TestProxyStopWithNilListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	proxy := &Proxy{
		config: &storage.Proxy{
			ID: "test-proxy",
		},
		listener: nil,
		conns:     make(map[string]*ProxyConn),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Should not panic
	proxy.Stop()
}

// ============ SendToAgent Tests ============

func TestManagerSendToAgent(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	// Set up the sendToAgentFn to actually add messages to agents
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

	err = manager.SendToAgent("agent-1", msg)
	if err != nil {
		t.Errorf("SendToAgent() error = %v", err)
	}

	if receivedMsg == nil {
		t.Error("agent should receive message")
	}

	if receivedMsg != nil && receivedMsg.Type != protocol.MessageTypeProxyNew {
		t.Errorf("received message type = %v, want ProxyNew", receivedMsg.Type)
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
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	if manager.ctx == nil {
		t.Fatal("context should be initialized")
	}

	// Start the manager
	err = manager.Start()
	if err != nil {
		t.Fatal(err)
	}

	// Check context is not cancelled
	select {
	case <-manager.ctx.Done():
		t.Error("context should not be cancelled yet")
	default:
	}

	// Stop the manager
	manager.Stop()

	// Context should be cancelled
	select {
	case <-manager.ctx.Done():
		// Expected
	default:
		t.Error("context should be cancelled after Stop()")
	}
}

// ============ Concurrent Access Tests ============

func TestManagerConcurrentStatus(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

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

// ============ StartProxy Tests ============

func TestStartProxyDuplicate(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "test-proxy",
		Name:       "Test Proxy",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0, // Let OS choose
		Target:     "localhost:8080",
		Enabled:    true,
	}

	// First start
	err = manager.StartProxy(proxyConfig)
	if err != nil {
		t.Fatalf("StartProxy() first call error = %v", err)
	}

	// Second start should fail
	err = manager.StartProxy(proxyConfig)
	if err == nil {
		t.Error("StartProxy() second call should return error")
	}

	// Cleanup
	manager.StopProxy(proxyConfig.ID)
}

func TestStartProxyInvalidPort(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "test-proxy",
		Name:       "Test Proxy",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: -1, // Invalid port
		Target:     "localhost:8080",
		Enabled:    true,
	}

	err = manager.StartProxy(proxyConfig)
	if err == nil {
		t.Error("StartProxy() with invalid port should return error")
	}
}

// ============ cleanupIdleConnections Tests ============

func TestCleanupIdleConnections(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "test-proxy",
		Name:       "Test Proxy",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}

	err = manager.StartProxy(proxyConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.StopProxy(proxyConfig.ID)

	// This test just verifies the method doesn't panic
	// Actual idle connection cleanup would require more setup
	manager.cleanupIdleConnections()
}

// ============ Additional Manager Tests ============

func TestManagerSendToAgentDelegation(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	// Set up mock to verify message was sent
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

	err = manager.SendToAgent("agent-1", msg)
	if err != nil {
		t.Errorf("SendToAgent() error = %v", err)
	}

	if receivedMsg == nil {
		t.Error("Message should be sent to agent")
	}
}

func TestGetProxyStatusRunningProxy(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	// Create a proxy config
	proxyConfig := &storage.Proxy{
		ID:         "test-proxy",
		Name:       "Test Proxy",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}
	db.CreateProxy(proxyConfig)

	err = manager.StartProxy(proxyConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.StopProxy(proxyConfig.ID)

	status, err := manager.GetProxyStatus(proxyConfig.ID)
	if err != nil {
		t.Fatalf("GetProxyStatus() error = %v", err)
	}

	if status["name"] != "Test Proxy" {
		t.Errorf("name = %v, want Test Proxy", status["name"])
	}

	if status["status"] != "running" {
		t.Errorf("status = %v, want running", status["status"])
	}
}

func TestGetAllStatusWithProxies(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	// Create multiple proxies
	for i := 1; i <= 2; i++ {
		proxyConfig := &storage.Proxy{
			ID:         fmt.Sprintf("proxy-%d", i),
			Name:       fmt.Sprintf("Proxy %d", i),
			AgentID:    "agent-1",
			ProxyType:  "tcp",
			RemotePort: 0,
			Target:     "localhost:8080",
			Enabled:    true,
		}
		db.CreateProxy(proxyConfig)

		err = manager.StartProxy(proxyConfig)
		if err != nil {
			t.Fatal(err)
		}
		defer manager.StopProxy(proxyConfig.ID)
	}

	status := manager.GetAllStatus()

	if len(status) != 2 {
		t.Errorf("GetAllStatus() returned %d items, want 2", len(status))
	}
}

func TestProxyStopNilListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	proxy := &Proxy{
		config: &storage.Proxy{
			ID: "test-proxy",
		},
		listener: nil,
		conns:     make(map[string]*ProxyConn),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Should not panic
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
		config: &storage.Proxy{
			ID: "test-proxy",
		},
		listener: nil,
		conns: map[string]*ProxyConn{
			"conn-1": proxyConn,
		},
		ctx:    ctx,
		cancel: cancel,
	}

	proxy.Stop()

	// Connection should be closed
	if !proxyConn.closed.Load() {
		t.Error("Connection should be closed after proxy stop")
	}

	if len(proxy.conns) != 0 {
		t.Error("Connections should be cleared after proxy stop")
	}
}

func TestManagerRunningState(t *testing.T) {
	mockServer := newMockServerInterface()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	manager := NewManager(mockServer, db)

	// Initially not running
	if manager.running.Load() {
		t.Error("Manager should not be running initially")
	}

	// Start
	err = manager.Start()
	if err != nil {
		t.Fatal(err)
	}

	if !manager.running.Load() {
		t.Error("Manager should be running after Start()")
	}

	// Try to start again (should fail)
	err = manager.Start()
	if err == nil {
		t.Error("Second Start() should return error")
	}

	// Stop
	manager.Stop()

	if manager.running.Load() {
		t.Error("Manager should not be running after Stop()")
	}
}
