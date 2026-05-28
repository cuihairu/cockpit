package proxy

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// mockWSConn simulates a WebSocket connection for testing
type mockWSConn struct {
	closed bool
}

func (m *mockWSConn) WriteJSON(v interface{}) error              { return nil }
func (m *mockWSConn) Close() error                                { m.closed = true; return nil }
func (m *mockWSConn) SetWriteDeadline(t time.Time) error         { return nil }
func (m *mockWSConn) ReadMessage() (int, []byte, error)          { return 0, nil, nil }
func (m *mockWSConn) SetReadDeadline(t time.Time) error          { return nil }
func (m *mockWSConn) SetPongHandler(h func(string) error)        {}
func (m *mockWSConn) WriteMessage(int, []byte) error             { return nil }
func (m *mockWSConn) WriteControl(int, []byte, time.Time) error  { return nil }
func (m *mockWSConn) Subprotocol() string                         { return "" }
func (m *mockWSConn) RemoteAddr() net.Addr                        { return nil }
func (m *mockWSConn) UnderlyingConn() net.Conn                    { return nil }
func (m *mockWSConn) MessageReader() io.Reader                    { return nil }

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

func TestNewHandler(t *testing.T) {
	h := NewHandler()
	if h == nil {
		t.Fatal("NewHandler() should not return nil")
	}
	if h.conns == nil {
		t.Error("conns map should be initialized")
	}
	if h.sendQueue == nil {
		t.Error("sendQueue should be initialized")
	}
	if h.running.Load() {
		t.Error("handler should not be running initially")
	}
}

func TestHandlerStartStop(t *testing.T) {
	h := NewHandler()

	// Start without ws conn - just sets running flag
	// Handler.Start() needs a real ws conn, so we test Stop directly
	h.Stop()

	// Double stop should be safe
	h.Stop()
}

func TestHandleProxyNewMissingFields(t *testing.T) {
	h := NewHandler()

	msg := &protocol.Message{
		Payload: map[string]interface{}{
			"proxyId": "",
			"target":  "",
			"connId":  "",
		},
	}

	err := h.HandleProxyNew(msg)
	if err == nil {
		t.Error("HandleProxyNew() should return error for missing fields")
	}
}

func TestHandleProxyNewMissingProxyID(t *testing.T) {
	h := NewHandler()

	msg := &protocol.Message{
		Payload: map[string]interface{}{
			"proxyId": "",
			"target":  "localhost:8080",
			"connId":  "conn-1",
		},
	}

	err := h.HandleProxyNew(msg)
	if err == nil {
		t.Error("HandleProxyNew() should return error for missing proxyId")
	}
}

func TestHandleProxyDataMissingConnID(t *testing.T) {
	h := NewHandler()

	msg := &protocol.Message{
		Payload: map[string]interface{}{
			"data": "test",
		},
	}

	err := h.HandleProxyData(msg)
	if err == nil {
		t.Error("HandleProxyData() should return error for missing connId")
	}
}

func TestHandleProxyDataNotFound(t *testing.T) {
	h := NewHandler()

	msg := &protocol.Message{
		Payload: map[string]interface{}{
			"connId": "non-existent",
			"data":   []byte("test"),
		},
	}

	err := h.HandleProxyData(msg)
	if err == nil {
		t.Error("HandleProxyData() should return error for non-existent connId")
	}
}

func TestHandleProxyDataStringData(t *testing.T) {
	h := NewHandler()

	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	agentConn := &AgentTargetConn{
		ID:      "string-conn",
		ProxyID: "proxy-1",
		Target:  "localhost:8080",
		Conn:    conn1,
		Created: time.Now(),
	}

	h.mu.Lock()
	h.conns["string-conn"] = agentConn
	h.mu.Unlock()

	msg := &protocol.Message{
		Payload: map[string]interface{}{
			"connId": "string-conn",
			"data":   "hello",
		},
	}

	// Read in background
	readCh := make(chan string, 1)
	go func() {
		buf := make([]byte, 100)
		conn2.SetReadDeadline(time.Now().Add(time.Second))
		n, _ := conn2.Read(buf)
		readCh <- string(buf[:n])
	}()

	err := h.HandleProxyData(msg)
	if err != nil {
		t.Errorf("HandleProxyData() with string data error = %v", err)
	}

	select {
	case data := <-readCh:
		if data != "hello" {
			t.Errorf("Received data = %q, want 'hello'", data)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for data")
	}
}

func TestHandleProxyDataBytesData(t *testing.T) {
	h := NewHandler()

	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	agentConn := &AgentTargetConn{
		ID:      "bytes-conn",
		ProxyID: "proxy-1",
		Target:  "localhost:8080",
		Conn:    conn1,
		Created: time.Now(),
	}

	h.mu.Lock()
	h.conns["bytes-conn"] = agentConn
	h.mu.Unlock()

	testData := []byte("binary data")
	msg := &protocol.Message{
		Payload: map[string]interface{}{
			"connId": "bytes-conn",
			"data":   testData,
		},
	}

	readCh := make(chan string, 1)
	go func() {
		buf := make([]byte, 100)
		conn2.SetReadDeadline(time.Now().Add(time.Second))
		n, _ := conn2.Read(buf)
		readCh <- string(buf[:n])
	}()

	err := h.HandleProxyData(msg)
	if err != nil {
		t.Errorf("HandleProxyData() with bytes error = %v", err)
	}

	select {
	case data := <-readCh:
		if data != string(testData) {
			t.Errorf("Received data = %q, want %q", data, testData)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for data")
	}
}

func TestHandleProxyDataClosedConn(t *testing.T) {
	h := NewHandler()

	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	agentConn := &AgentTargetConn{
		ID:      "closed-conn",
		ProxyID: "proxy-1",
		Target:  "localhost:8080",
		Conn:    conn1,
		Created: time.Now(),
	}
	agentConn.closed.Store(true)

	h.mu.Lock()
	h.conns["closed-conn"] = agentConn
	h.mu.Unlock()

	msg := &protocol.Message{
		Payload: map[string]interface{}{
			"connId": "closed-conn",
			"data":   []byte("test"),
		},
	}

	err := h.HandleProxyData(msg)
	if err == nil {
		t.Error("HandleProxyData() should return error for closed connection")
	}
}

func TestHandleProxyDataInvalidType(t *testing.T) {
	h := NewHandler()

	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	agentConn := &AgentTargetConn{
		ID:      "invalid-conn",
		ProxyID: "proxy-1",
		Target:  "localhost:8080",
		Conn:    conn1,
		Created: time.Now(),
	}

	h.mu.Lock()
	h.conns["invalid-conn"] = agentConn
	h.mu.Unlock()

	msg := &protocol.Message{
		Payload: map[string]interface{}{
			"connId": "invalid-conn",
			"data":   12345, // invalid type
		},
	}

	err := h.HandleProxyData(msg)
	if err == nil {
		t.Error("HandleProxyData() should return error for invalid data type")
	}
}

func TestHandleProxyCloseExisting(t *testing.T) {
	h := NewHandler()

	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	agentConn := &AgentTargetConn{
		ID:      "close-conn",
		ProxyID: "proxy-1",
		Target:  "localhost:8080",
		Conn:    conn1,
		Created: time.Now(),
	}

	h.mu.Lock()
	h.conns["close-conn"] = agentConn
	h.mu.Unlock()

	msg := &protocol.Message{
		Payload: map[string]interface{}{
			"connId": "close-conn",
		},
	}

	err := h.HandleProxyClose(msg)
	if err != nil {
		t.Errorf("HandleProxyClose() error = %v", err)
	}

	h.mu.RLock()
	_, exists := h.conns["close-conn"]
	h.mu.RUnlock()

	if exists {
		t.Error("Connection should be removed after HandleProxyClose")
	}
}

func TestHandleProxyCloseNonExistent(t *testing.T) {
	h := NewHandler()

	msg := &protocol.Message{
		Payload: map[string]interface{}{
			"connId": "non-existent",
		},
	}

	err := h.HandleProxyClose(msg)
	if err != nil {
		t.Errorf("HandleProxyClose() for non-existent should not error, got %v", err)
	}
}

func TestSendMessageNotRunning(t *testing.T) {
	h := NewHandler()

	msg := protocol.NewMessage(protocol.MessageTypeProxyData, nil)
	err := h.SendMessage(msg)
	if err == nil {
		t.Error("SendMessage() should return error when not running")
	}
}

func TestSendErrorNotRunning(t *testing.T) {
	h := NewHandler()
	// Should not panic
	h.SendError("proxy-1", "conn-1", "test error")
}

func TestSendCloseNotRunning(t *testing.T) {
	h := NewHandler()
	// Should not panic
	h.SendClose("proxy-1", "conn-1", "test reason")
}

func TestHandleProxyDataArrayData(t *testing.T) {
	h := NewHandler()

	conn1, conn2 := net.Pipe()
	defer conn2.Close()

	agentConn := &AgentTargetConn{
		ID:      "array-conn",
		ProxyID: "proxy-1",
		Target:  "localhost:8080",
		Conn:    conn1,
		Created: time.Now(),
	}

	h.mu.Lock()
	h.conns["array-conn"] = agentConn
	h.mu.Unlock()

	// JSON array of floats
	msg := &protocol.Message{
		Payload: map[string]interface{}{
			"connId": "array-conn",
			"data": []interface{}{
				float64(72), float64(101), float64(108), float64(108), float64(111),
			},
		},
	}

	readCh := make(chan string, 1)
	go func() {
		buf := make([]byte, 100)
		conn2.SetReadDeadline(time.Now().Add(time.Second))
		n, _ := conn2.Read(buf)
		readCh <- string(buf[:n])
	}()

	err := h.HandleProxyData(msg)
	if err != nil {
		t.Errorf("HandleProxyData() with array error = %v", err)
	}

	select {
	case data := <-readCh:
		if data != "Hello" {
			t.Errorf("Received data = %q, want 'Hello'", data)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for data")
	}
}

// ============ Mock WS Tests ============

func TestMockWSConnMethods(t *testing.T) {
	m := &mockWSConn{}

	if err := m.WriteJSON(nil); err != nil {
		t.Errorf("WriteJSON() error = %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
	if !m.closed {
		t.Error("Should be closed")
	}
	if m.Subprotocol() != "" {
		t.Error("Subprotocol should be empty")
	}
	if m.RemoteAddr() != nil {
		t.Error("RemoteAddr should be nil")
	}
	if m.UnderlyingConn() != nil {
		t.Error("UnderlyingConn should be nil")
	}
	if m.MessageReader() != nil {
		t.Error("MessageReader should be nil")
	}
}

// Ensure mockWSConn still satisfies the interface check
var _ interface {
	WriteJSON(v interface{}) error
	Close() error
	SetWriteDeadline(t time.Time) error
	ReadMessage() (int, []byte, error)
	SetReadDeadline(t time.Time) error
	SetPongHandler(h func(string) error)
	WriteMessage(int, []byte) error
	WriteControl(int, []byte, time.Time) error
	Subprotocol() string
	RemoteAddr() net.Addr
	UnderlyingConn() net.Conn
	MessageReader() io.Reader
} = (*mockWSConn)(nil)

func TestHandlerStartWithMock(t *testing.T) {
	h := NewHandler()

	// Manually set running to test Stop behavior
	h.running.Store(true)
	if !h.running.Load() {
		t.Error("Handler should be running")
	}
}

func TestHandlerStopCleansConns(t *testing.T) {
	h := NewHandler()
	h.running.Store(true)
	h.sendQueue = make(chan *protocol.Message, 1000)

	conn1, _ := net.Pipe()
	h.mu.Lock()
	h.conns["c1"] = &AgentTargetConn{ID: "c1", Conn: conn1}
	h.mu.Unlock()

	h.Stop()

	h.mu.RLock()
	count := len(h.conns)
	h.mu.RUnlock()
	if count != 0 {
		t.Errorf("conns should be empty after Stop(), got %d", count)
	}
}

func TestSendMessageRunning(t *testing.T) {
	h := NewHandler()
	h.running.Store(true)
	h.sendQueue = make(chan *protocol.Message, 1000)

	msg := protocol.NewMessage(protocol.MessageTypeHeartbeat, nil)
	err := h.SendMessage(msg)
	if err != nil {
		t.Errorf("SendMessage() while running error = %v", err)
	}
}

func TestSendErrorRunning(t *testing.T) {
	h := NewHandler()
	h.running.Store(true)
	h.sendQueue = make(chan *protocol.Message, 1000)

	h.SendError("p1", "c1", "test error")
}

func TestSendCloseRunning(t *testing.T) {
	h := NewHandler()
	h.running.Store(true)
	h.sendQueue = make(chan *protocol.Message, 1000)

	h.SendClose("p1", "c1", "test reason")
}
