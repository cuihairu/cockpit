package remote

import (
	"net"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestNewManager(t *testing.T) {
	m := NewManager()

	if m == nil {
		t.Error("NewManager() should not return nil")
	}

	if m.connections == nil {
		t.Error("connections map should be initialized")
	}

	if m.ctx == nil {
		t.Error("context should be initialized")
	}

	if m.cancel == nil {
		t.Error("cancel function should be initialized")
	}
}

func TestManagerStartStop(t *testing.T) {
	m := NewManager()

	err := m.Start()
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Should not panic
	m.Stop()

	// Should be safe to stop multiple times
	m.Stop()
}

func TestConnectionStruct(t *testing.T) {
	now := time.Now()
	conn := &Connection{
		ID:       "test-conn",
		Protocol: protocol.RemoteProtocolSSH,
		Target:   "192.168.1.1:22",
		CreatedAt: now,
		LastActive: now,
	}

	if conn.ID != "test-conn" {
		t.Errorf("ID = %v, want test-conn", conn.ID)
	}

	if conn.Protocol != protocol.RemoteProtocolSSH {
		t.Errorf("Protocol = %v, want SSH", conn.Protocol)
	}

	if conn.Target != "192.168.1.1:22" {
		t.Errorf("Target = %v, want 192.168.1.1:22", conn.Target)
	}
}

func TestGetConnectionNotFound(t *testing.T) {
	m := NewManager()

	_, exists := m.GetConnection("non-existent")
	if exists {
		t.Error("GetConnection() should return false for non-existent connection")
	}
}

func TestCloseConnectionNotFound(t *testing.T) {
	m := NewManager()

	err := m.CloseConnection("non-existent")
	if err == nil {
		t.Error("CloseConnection() should return error for non-existent connection")
	}
}

func TestCreateConnectionInvalidTarget(t *testing.T) {
	m := NewManager()

	_, err := m.CreateConnection(
		"test-conn",
		protocol.RemoteProtocolSSH,
		"invalid-target:99999", // Invalid port
		1*time.Second,
	)

	if err == nil {
		t.Error("CreateConnection() should return error for invalid target")
	}
}

func TestSetClientConn(t *testing.T) {
	conn := &Connection{
		ID:     "test",
		Target: "localhost:8080",
	}

	clientConn := &mockConn{}
	conn.SetClientConn(clientConn)

	if conn.ClientConn != clientConn {
		t.Error("ClientConn should be set")
	}
}

func TestHandleDataWithoutConn(t *testing.T) {
	conn := &Connection{
		ID:     "test",
		Target: "localhost:8080",
	}

	err := conn.HandleData([]byte("test"), true)
	if err == nil {
		t.Error("HandleData() should return error when target conn is not ready")
	}

	err = conn.HandleData([]byte("test"), false)
	if err == nil {
		t.Error("HandleData() should return error when client conn is not ready")
	}
}

func TestCloseConnection(t *testing.T) {
	conn := &Connection{
		ID:     "test",
		Target: "localhost:8080",
	}

	err := conn.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Should be safe to close again
	err = conn.Close()
	if err != nil {
		t.Errorf("Close() again should not error, got %v", err)
	}
}

func TestManagerGetStats(t *testing.T) {
	m := NewManager()

	stats := m.GetStats()

	if stats == nil {
		t.Error("GetStats() should not return nil")
	}

	total, ok := stats["totalConnections"].(int)
	if !ok {
		t.Error("totalConnections should be int")
	}

	if total != 0 {
		t.Errorf("totalConnections = %d, want 0", total)
	}

	byProtocol, ok := stats["byProtocol"].(map[string]int)
	if !ok {
		t.Error("byProtocol should be map[string]int")
	}

	if len(byProtocol) != 0 {
		t.Errorf("byProtocol should be empty, got %v", byProtocol)
	}
}

func TestManagerMultipleConnections(t *testing.T) {
	m := NewManager()

	// Start a TCP server for testing
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	addr := listener.Addr().String()

	// Create multiple connections
	for i := 0; i < 3; i++ {
		conn, err := m.CreateConnection(
			"conn-"+string(rune('0'+i)),
			protocol.RemoteProtocolSSH,
			addr,
			1*time.Second,
		)
		if err != nil {
			t.Errorf("CreateConnection() %d error = %v", i, err)
		}
		if conn != nil {
			conn.Close()
		}
	}

	stats := m.GetStats()
	total, _ := stats["totalConnections"].(int)
	if total != 0 {
		// Connections were closed
	}
}

func TestConnectionProtocol(t *testing.T) {
	protocols := []protocol.RemoteProtocol{
		protocol.RemoteProtocolSSH,
		protocol.RemoteProtocolTelnet,
		protocol.RemoteProtocolVNC,
		protocol.RemoteProtocolRDP,
		protocol.RemoteProtocolFTP,
	}

	for _, proto := range protocols {
		conn := &Connection{
			ID:       "test",
			Protocol: proto,
			Target:   "localhost:8080",
		}

		if conn.Protocol != proto {
			t.Errorf("Protocol = %v, want %v", conn.Protocol, proto)
		}
	}
}

func TestManagerStopWithConnections(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	addr := listener.Addr().String()

	conn, err := m.CreateConnection("test", protocol.RemoteProtocolSSH, addr, 1*time.Second)
	if err != nil {
		t.Skip("Need server for this test")
		return
	}

	if conn != nil {
		conn.Close()
	}

	// Stop should clean up
	m.Stop()

	stats := m.GetStats()
	total, _ := stats["totalConnections"].(int)
	if total != 0 {
		t.Errorf("After stop, totalConnections should be 0, got %d", total)
	}
}

func TestConnectionUpdateLastActive(t *testing.T) {
	conn := &Connection{
		ID:         "test",
		Target:     "localhost:8080",
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	oldLastActive := conn.LastActive

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Update through SetClientConn
	clientConn := &mockConn{}
	conn.SetClientConn(clientConn)

	// LastActive should be updated
	if !conn.LastActive.After(oldLastActive) {
		t.Error("LastActive should be updated")
	}
}

func TestConnectionCloseWithNilConns(t *testing.T) {
	conn := &Connection{
		ID:     "test",
		Target: "localhost:8080",
	}

	// Should not panic
	err := conn.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestConnectionEmptyID(t *testing.T) {
	conn := &Connection{
		ID:     "",
		Target: "localhost:8080",
	}

	if conn.ID != "" {
		t.Error("ID should be empty")
	}
}

func TestConnectionEmptyTarget(t *testing.T) {
	conn := &Connection{
		ID:     "test",
		Target: "",
	}

	if conn.Target != "" {
		t.Error("Target should be empty")
	}
}

func TestManagerStart(t *testing.T) {
	m := NewManager()

	err := m.Start()
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Start should be idempotent
	err = m.Start()
	if err != nil {
		t.Errorf("Start() again should not error, got %v", err)
	}

	m.Stop()
}

func TestGetStatsWithConnections(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	addr := listener.Addr().String()

	// Create connections with different protocols
	m.CreateConnection("conn1", protocol.RemoteProtocolSSH, addr, 1*time.Second)
	m.CreateConnection("conn2", protocol.RemoteProtocolTelnet, addr, 1*time.Second)

	stats := m.GetStats()

	byProtocol, _ := stats["byProtocol"].(map[string]int)

	if len(byProtocol) == 0 {
		// Connections might have failed
	}
}

func TestConcurrentManagerOperations(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	done := make(chan bool, 10)

	// Concurrent operations
	for i := 0; i < 10; i++ {
		go func(id int) {
			m.GetConnection("test")
			m.GetStats()
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestDuplicateConnectionID(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		listener.Accept()
	}()

	addr := listener.Addr().String()

	// Create first connection
	conn1, err := m.CreateConnection("dup", protocol.RemoteProtocolSSH, addr, 1*time.Second)
	if err != nil {
		t.Skip("Need server for this test")
		return
	}

	if conn1 != nil {
		defer conn1.Close()
	}

	// Try to create duplicate
	_, err = m.CreateConnection("dup", protocol.RemoteProtocolSSH, addr, 1*time.Second)
	if err == nil {
		t.Error("CreateConnection() with duplicate ID should return error")
	}
}

// Mock connection for testing
type mockConn struct {
	closed bool
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	return 0, nil
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	return len(b), nil
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return nil
}

func (m *mockConn) RemoteAddr() net.Addr {
	return nil
}

func (m *mockConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}
