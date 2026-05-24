package proxy

import (
	"io"
	"net"
	"testing"
	"time"
)

// mockWSConn 模拟 WebSocket 连接
type mockWSConn struct {
	closed bool
}

func (m *mockWSConn) WriteJSON(v interface{}) error {
	return nil
}

func (m *mockWSConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockWSConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (m *mockWSConn) ReadMessage() (int, []byte, error) {
	return 0, nil, nil
}

func (m *mockWSConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockWSConn) SetPongHandler(h func(string) error) {}

func (m *mockWSConn) WriteMessage(int, []byte) error {
	return nil
}

func (m *mockWSConn) WriteControl(int, []byte, time.Time) error {
	return nil
}

func (m *mockWSConn) Subprotocol() string {
	return ""
}

func (m *mockWSConn) RemoteAddr() net.Addr {
	return nil
}

func (m *mockWSConn) UnderlyingConn() net.Conn {
	return nil
}

func (m *mockWSConn) MessageReader() io.Reader {
	return nil
}

func TestAgentTargetConnClose(t *testing.T) {
	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	agentConn := &AgentTargetConn{
		ID:      "conn-1",
		ProxyID: "proxy-1",
		Target:  "localhost:8080",
		Conn:    conn1,
		Created: time.Now(),
	}

	err := agentConn.Close()
	if err != nil {
		t.Errorf("First Close() error = %v", err)
	}

	if !agentConn.closed.Load() {
		t.Error("connection should be marked as closed")
	}

	err = agentConn.Close()
	if err != nil {
		t.Errorf("Second Close() should not error, got %v", err)
	}
}

func TestAgentTargetConnFields(t *testing.T) {
	now := time.Now()
	agentConn := &AgentTargetConn{
		ID:      "conn-1",
		ProxyID: "proxy-1",
		Target:  "localhost:8080",
		Conn:    nil,
		Created: now,
	}

	if agentConn.ID != "conn-1" {
		t.Errorf("ID = %v, want conn-1", agentConn.ID)
	}

	if agentConn.ProxyID != "proxy-1" {
		t.Errorf("ProxyID = %v, want proxy-1", agentConn.ProxyID)
	}

	if agentConn.Target != "localhost:8080" {
		t.Errorf("Target = %v, want localhost:8080", agentConn.Target)
	}

	if agentConn.Created.IsZero() {
		t.Error("Created should be set")
	}

	if agentConn.closed.Load() {
		t.Error("connection should not be closed initially")
	}
}
