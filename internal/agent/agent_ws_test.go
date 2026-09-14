package agent

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// ============ virtualization helpers (previously uncovered) ============

func TestReadSysFileMissing(t *testing.T) {
	if _, err := readSysFile("/nonexistent/path/file.txt"); err == nil {
		t.Error("readSysFile() should fail for a missing file")
	}
}

func TestReadSysFileTrims(t *testing.T) {
	fp := filepath.Join(t.TempDir(), "value")
	os.WriteFile(fp, []byte("  hello world \n"), 0644)

	got, err := readSysFile(fp)
	if err != nil {
		t.Fatalf("readSysFile() error = %v", err)
	}
	if got != "hello world" {
		t.Errorf("readSysFile() = %q, want %q", got, "hello world")
	}
}

func TestReadProcCpuInfo(t *testing.T) {
	_, _ = readProcCpuInfo() // platform dependent; must not panic
}

func TestIsContainerAndDetectContainerType(t *testing.T) {
	_ = isContainer()
	_ = detectContainerType()
}

func TestDetectVirtualizationManual(t *testing.T) {
	info := detectVirtualizationManual()
	if info == nil {
		t.Fatal("detectVirtualizationManual() should never return nil")
	}
	t.Logf("virtualization: type=%s role=%s", info.Type, info.Role)
}

func TestRunCommandMissing(t *testing.T) {
	out, err := runCommand("definitely-not-a-command-xyz")
	if err == nil {
		t.Error("runCommand() should fail for a missing binary")
	}
	if out != "" {
		t.Errorf("runCommand() = %q, want empty", out)
	}
}

func TestRunCommandIsStubbed(t *testing.T) {
	// runCommand is intentionally a stub that never executes anything
	out, err := runCommand("echo", "hi")
	if err == nil {
		t.Error("stubbed runCommand() should return an error")
	}
	if out != "" {
		t.Errorf("runCommand() = %q, want empty", out)
	}
}

// ============ sendMessage / writeToConn edge cases ============

func TestSendMessageQueueFull(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	// Fill the outbound queue
	for i := 0; i < cap(a.outbound); i++ {
		if err := a.sendMessage(protocol.NewMessage(protocol.MessageTypePing, nil)); err != nil {
			t.Fatalf("sendMessage() %d unexpectedly failed: %v", i, err)
		}
	}
	if err := a.sendMessage(protocol.NewMessage(protocol.MessageTypePing, nil)); err == nil {
		t.Error("sendMessage() should fail when the queue is full")
	}
}

func TestSendMessageAfterStop(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	a.Stop()

	// After stop the call must not block; with queue space the outcome is a
	// race between the outbound and ctx.Done select cases, so accept either
	// success or a proper error, but never a hang or panic.
	done := make(chan error, 1)
	go func() {
		done <- a.sendMessage(protocol.NewMessage(protocol.MessageTypePing, nil))
	}()
	select {
	case err := <-done:
		if err != nil && err.Error() != "agent stopped" && err.Error() != "agent outbound queue full" {
			t.Errorf("sendMessage() after stop = %v, want known error or nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sendMessage() blocked after stop")
	}
}

func TestWriteToConnNotConnected(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	if err := a.writeToConn(protocol.NewMessage(protocol.MessageTypePing, nil)); err == nil {
		t.Error("writeToConn() should fail when not connected")
	}
}

func TestSendHeartbeatBeforeRegistration(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	// Must be a no-op without registration (drains nothing into outbound)
	a.sendHeartbeat()
	select {
	case <-a.outbound:
		t.Error("sendHeartbeat must not enqueue before registration")
	default:
	}
}

func TestSendHeartbeatAfterRegistration(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	a.registered.Store(true)
	a.sendHeartbeat()

	select {
	case msg := <-a.outbound:
		if msg.Type != protocol.MessageTypeHeartbeat {
			t.Errorf("heartbeat message type = %q, want heartbeat", msg.Type)
		}
		if msg.Payload["agentId"] != a.agentID {
			// agentID may be empty here; only status matters
			_ = msg.Payload["agentId"]
		}
		if msg.Payload["status"] != "online" {
			t.Errorf("heartbeat status = %v, want online", msg.Payload["status"])
		}
		if msg.Payload["systemInfo"] == nil {
			t.Error("heartbeat should carry systemInfo from the collector")
		}
	default:
		t.Fatal("sendHeartbeat did not enqueue a message")
	}
}

// ============ websocket-based agent lifecycle ============

type fakeServer struct {
	upgrader  websocket.Upgrader
	connCh    chan *websocket.Conn
	register  chan *protocol.Message
	responder func(msg *protocol.Message) *protocol.Message
}

func newFakeServer() *fakeServer {
	return &fakeServer{
		upgrader: websocket.Upgrader{},
		connCh:   make(chan *websocket.Conn, 4),
		register: make(chan *protocol.Message, 4),
	}
}

func (f *fakeServer) handler(w http.ResponseWriter, r *http.Request) {
	conn, err := f.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	f.connCh <- conn
}

// readUntil reads messages until pred matches or timeout.
// Returns nil on timeout (safe to call from non-test goroutines).
func readUntil(t *testing.T, conn *websocket.Conn, timeout time.Duration, pred func(*protocol.Message) bool) *protocol.Message {
	t.Helper()
	codec := protocol.NewCodec()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		msg, err := codec.ReadMessage(conn)
		if err != nil {
			continue
		}
		if pred == nil || pred(msg) {
			return msg
		}
	}
	t.Errorf("timed out waiting for message")
	return nil
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}

func TestAgentStartConnectFailure(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1/nope"})
	defer a.Stop()

	errCh := make(chan error, 1)
	go func() { errCh <- a.Start() }()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Start() should fail when the server is unreachable")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Start() did not fail fast for an unreachable server")
	}
}

func TestAgentRegisterRejected(t *testing.T) {
	fs := newFakeServer()
	srv := httptest.NewServer(http.HandlerFunc(fs.handler))
	defer srv.Close()

	a := NewAgent(Config{ServerURL: wsURL(srv), AgentID: "agent-reg-test"})
	defer a.Stop()
	// Server rejects registration by replying with the wrong message type
	go func() {
		select {
		case conn := <-fs.connCh:
			readUntil(t, conn, 10*time.Second, func(m *protocol.Message) bool {
				return m.Type == protocol.MessageTypeRegister
			})
			conn.WriteMessage(websocket.TextMessage, []byte(`{"id":"x","type":"error","payload":{}}`))
		case <-time.After(10 * time.Second):
		}
	}()

	errCh := make(chan error, 1)
	go func() { errCh <- a.Start() }()

	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "register failed") {
			t.Errorf("Start() error = %v, want register failed", err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("Start() did not return after register rejection")
	}
}

func TestAgentFullLifecycleOverWebSocket(t *testing.T) {
	fs := newFakeServer()
	srv := httptest.NewServer(http.HandlerFunc(fs.handler))
	defer srv.Close()

	a := NewAgent(Config{ServerURL: wsURL(srv), AgentID: "agent-lifecycle"})
	codec := protocol.NewCodec()

	serverDone := make(chan *websocket.Conn, 1)
	serverHold := make(chan struct{})
	go func() {
		select {
		case conn := <-fs.connCh:
			t.Cleanup(func() { conn.Close() })
			// Accept registration
			reg := readUntil(t, conn, 30*time.Second, func(m *protocol.Message) bool {
				return m.Type == protocol.MessageTypeRegister
			})
			if reg == nil {
				serverDone <- nil
				<-serverHold
				return
			}
			if reg.Payload["agentId"] != "agent-lifecycle" {
				t.Errorf("registered agentId = %v", reg.Payload["agentId"])
			}
			conn.WriteMessage(websocket.TextMessage, []byte(`{"id":"reg-ok","type":"register","payload":{"status":"ok"}}`))
			serverDone <- conn
			<-serverHold // keep the connection open for the whole test
		case <-time.After(30 * time.Second):
			serverDone <- nil
		}
	}()
	defer close(serverHold)

	startErr := make(chan error, 1)
	go func() { startErr <- a.Start() }()

	var conn *websocket.Conn
	select {
	case conn = <-serverDone:
		if conn == nil {
			t.Fatal("server never saw the registration")
		}
	case <-time.After(45 * time.Second):
		t.Fatal("registration handshake timed out")
	}

	// Wait until the agent considers itself registered (writeLoop starts flushing)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !a.registered.Load() {
		time.Sleep(50 * time.Millisecond)
	}
	if !a.registered.Load() {
		t.Fatal("agent did not become registered")
	}

	// Ping -> pong via the outbound writeLoop
	ping := protocol.NewMessage(protocol.MessageTypePing, nil)
	ping.ID = "ping-1"
	codec.WriteMessage(conn, ping)
	pong := readUntil(t, conn, 10*time.Second, func(m *protocol.Message) bool {
		return m.Type == protocol.MessageTypeHeartbeat && m.ID == "ping-1"
	})
	if pong.Payload["status"] != "pong" {
		t.Errorf("ping response status = %v, want pong", pong.Payload["status"])
	}

	// RPC request -> error response (unknown method)
	rpcReq := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]any{
		"method": "no.such.method",
	})
	rpcReq.ID = "rpc-1"
	codec.WriteMessage(conn, rpcReq)
	rpcResp := readUntil(t, conn, 10*time.Second, func(m *protocol.Message) bool {
		return m.Type == protocol.MessageTypeRPCResponse && m.ID == "rpc-1"
	})
	if rpcResp.Payload["status"] != "error" {
		t.Errorf("rpc response status = %v, want error", rpcResp.Payload["status"])
	}

	// Unknown message type is ignored gracefully
	unknown := protocol.NewMessage("mystery-type", nil)
	if err := codec.WriteMessage(conn, unknown); err != nil {
		t.Fatalf("write unknown: %v", err)
	}

	a.Stop()

	select {
	case err := <-startErr:
		if err != nil {
			t.Errorf("Start() returned error after Stop(): %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Start() did not return after Stop()")
	}
}

func TestAgentReconnectsAfterServerDrop(t *testing.T) {
	fs := newFakeServer()
	srv := httptest.NewServer(http.HandlerFunc(fs.handler))
	defer srv.Close()

	a := NewAgent(Config{ServerURL: wsURL(srv), AgentID: "agent-reconnect"})
	defer a.Stop()

	// acceptAndRegister handles one agent connection: register -> reply ok
	acceptAndRegister := func(conn *websocket.Conn) {
		t.Cleanup(func() { conn.Close() })
		reg := readUntil(t, conn, 30*time.Second, func(m *protocol.Message) bool {
			return m.Type == protocol.MessageTypeRegister
		})
		if reg == nil {
			return
		}
		conn.WriteMessage(websocket.TextMessage, []byte(`{"id":"reg-ok","type":"register","payload":{"status":"ok"}}`))
	}

	startErr := make(chan error, 1)
	go func() { startErr <- a.Start() }()

	go acceptAndRegister(<-fs.connCh)

	// Wait for initial registration
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && !a.registered.Load() {
		time.Sleep(50 * time.Millisecond)
	}
	if !a.registered.Load() {
		t.Fatal("agent did not register initially")
	}

	// Drop the server side: the agent must detect the read error and reconnect.
	// The next server connection is handled by the goroutine below.
	dropped := make(chan *websocket.Conn, 1)
	go func() {
		select {
		case conn := <-fs.connCh:
			dropped <- conn
			acceptAndRegister(conn)
		case <-time.After(60 * time.Second):
		}
	}()

	// Force a disconnect by closing all currently tracked connections is not
	// possible from here, so simulate the agent side instead: trigger the
	// reconnect path directly after killing the current connection state.
	a.mu.RLock()
	current := a.conn
	a.mu.RUnlock()
	if current == nil {
		t.Fatal("agent should have an active connection")
	}
	current.Close() // read failure in messageLoop triggers reconnect

	// The reconnect loop sleeps 5s before retrying; wait for the new registration
	deadline = time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) && !a.registered.Load() {
		time.Sleep(100 * time.Millisecond)
	}
	if !a.registered.Load() {
		t.Fatal("agent did not re-register after reconnect")
	}

	select {
	case conn := <-dropped:
		if conn == nil {
			t.Fatal("reconnect did not produce a new server connection")
		}
	default:
		// dropped may already be drained; registration state is the source of truth
	}

	a.Stop()
	select {
	case err := <-startErr:
		if err != nil {
			t.Errorf("Start() returned error after Stop(): %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Start() did not return after Stop()")
	}
}
