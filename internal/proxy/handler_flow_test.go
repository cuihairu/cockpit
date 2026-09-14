package proxy

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ Handler (agent side) tests ============

// echoServer starts a TCP server that echoes received bytes back to the client.
func echoServer(t *testing.T) (addr string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					if _, err := c.Write(buf[:n]); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

// collectSendFunc returns a send function capturing messages into a channel.
func collectSendFunc(msgs chan *protocol.Message) func(*protocol.Message) error {
	return func(m *protocol.Message) error {
		select {
		case msgs <- m:
		default:
		}
		return nil
	}
}

func waitForMessage(t *testing.T, msgs chan *protocol.Message, msgType protocol.MessageType) *protocol.Message {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case m := <-msgs:
			if m.Type == msgType {
				return m
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s message", msgType)
		}
	}
}

func TestHandlerStartAndAttachConn(t *testing.T) {
	h := NewHandler()
	h.Start(nil)
	if !h.running.Load() {
		t.Error("handler should be running after Start")
	}
	h.AttachConn(nil) // reconnect scenario must be safe
	h.Stop()
}

func TestHandlerSendMessageNotRunning(t *testing.T) {
	h := NewHandler()
	if err := h.SendMessage(protocol.NewMessage(protocol.MessageTypePing, nil)); err == nil {
		t.Error("SendMessage should fail when handler not running")
	}
}

func TestHandlerSendMessageNoSendFunc(t *testing.T) {
	h := NewHandler()
	h.Start(nil)
	defer h.Stop()
	if err := h.SendMessage(protocol.NewMessage(protocol.MessageTypePing, nil)); err == nil {
		t.Error("SendMessage should fail without configured send func")
	}
}

func TestHandlerProxyNewMissingFields(t *testing.T) {
	h := NewHandler()
	msg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"proxyId": "px-1",
		// target/connId missing
	})
	if err := h.HandleProxyNew(msg); err == nil {
		t.Error("HandleProxyNew should fail when required fields are missing")
	}
}

func TestHandlerProxyNewInvalidPayload(t *testing.T) {
	h := NewHandler()
	msg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"proxyId": 123, // wrong type
	})
	if err := h.HandleProxyNew(msg); err == nil {
		t.Error("HandleProxyNew should fail on invalid payload")
	}
}

func TestHandlerProxyNewUnreachableTarget(t *testing.T) {
	h := NewHandler()
	msgs := make(chan *protocol.Message, 8)
	h.SetSendFunc(collectSendFunc(msgs))
	h.Start(nil)
	defer h.Stop()

	msg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"proxyId": "px-1",
		"connId":  "conn-1",
		"target":  "127.0.0.1:1", // nothing listens here
	})
	if err := h.HandleProxyNew(msg); err == nil {
		t.Error("HandleProxyNew should fail for unreachable target")
	}
	errMsg := waitForMessage(t, msgs, protocol.MessageTypeProxyError)
	if errMsg.Payload["connId"] != "conn-1" {
		t.Errorf("proxy error connId = %v, want conn-1", errMsg.Payload["connId"])
	}
}

func TestHandlerProxyDataRoundTrip(t *testing.T) {
	target := echoServer(t)

	h := NewHandler()
	msgs := make(chan *protocol.Message, 16)
	h.SetSendFunc(collectSendFunc(msgs))
	h.Start(nil)
	defer h.Stop()

	// Open a proxied connection to the echo server
	newMsg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"proxyId": "px-1",
		"connId":  "conn-1",
		"target":  target,
	})
	if err := h.HandleProxyNew(newMsg); err != nil {
		t.Fatalf("HandleProxyNew() error = %v", err)
	}

	// Send data through the tunnel; the echo server returns it via readFromTarget
	dataMsg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "px-1",
		"connId":  "conn-1",
		"data":    []byte("hello proxy"),
	})
	if err := h.HandleProxyData(dataMsg); err != nil {
		t.Fatalf("HandleProxyData() error = %v", err)
	}

	echoed := waitForMessage(t, msgs, protocol.MessageTypeProxyData)
	data, _ := json.Marshal(echoed.Payload["data"])
	if want := `"aGVsbG8gcHJveHk="`; string(data) != want {
		t.Errorf("echoed data = %s, want %s", data, want)
	}

	// Unknown connection
	unknown := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "px-1",
		"connId":  "no-such-conn",
		"data":    []byte("x"),
	})
	if err := h.HandleProxyData(unknown); err == nil {
		t.Error("HandleProxyData should fail for unknown connection")
	}
}

func TestHandlerProxyDataClosedConn(t *testing.T) {
	target := echoServer(t)
	h := NewHandler()
	msgs := make(chan *protocol.Message, 8)
	h.SetSendFunc(collectSendFunc(msgs))
	h.Start(nil)
	defer h.Stop()

	newMsg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"proxyId": "px-1",
		"connId":  "conn-1",
		"target":  target,
	})
	if err := h.HandleProxyNew(newMsg); err != nil {
		t.Fatalf("HandleProxyNew() error = %v", err)
	}

	// Close the agent-side connection, then data must be rejected
	closeMsg := protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
		"proxyId": "px-1",
		"connId":  "conn-1",
	})
	if err := h.HandleProxyClose(closeMsg); err != nil {
		t.Fatalf("HandleProxyClose() error = %v", err)
	}

	dataMsg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "px-1",
		"connId":  "conn-1",
		"data":    []byte("late"),
	})
	if err := h.HandleProxyData(dataMsg); err == nil {
		t.Error("HandleProxyData should fail for closed connection")
	}
}

// ============ Manager (server side) end-to-end tests ============

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestManagerStartLoadsEnabledProxiesFromDB(t *testing.T) {
	mockServer := newMockServerInterface()
	mockServer.addAgent("agent-1")
	db := testDB(t)

	if err := db.CreateProxy(&storage.Proxy{
		ID:        "px-db",
		Name:      "db-proxy",
		AgentID:   "agent-1",
		ProxyType: "tcp",
		RemotePort: freePort(t),
		Target:    "127.0.0.1:12345",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("CreateProxy: %v", err)
	}
	// Disabled proxies must not start.
	// Note: created enabled first, then updated, because GORM applies the
	// `default:true` tag to the zero value false on insert.
	offCfg := &storage.Proxy{
		ID:        "px-off",
		Name:      "off",
		AgentID:   "agent-1",
		ProxyType: "tcp",
		RemotePort: freePort(t),
		Target:    "127.0.0.1:12345",
		Enabled:   true,
	}
	if err := db.CreateProxy(offCfg); err != nil {
		t.Fatalf("CreateProxy: %v", err)
	}
	offCfg.Enabled = false
	if err := db.UpdateProxy(offCfg); err != nil {
		t.Fatalf("UpdateProxy: %v", err)
	}

	m := NewManager(mockServer, db)
	if err := m.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer m.Stop()

	if _, err := m.GetProxyStatus("px-db"); err != nil {
		t.Errorf("enabled proxy should be running: %v", err)
	}
	if _, err := m.GetProxyStatus("px-off"); err == nil {
		t.Error("disabled proxy should not be running")
	}
}

func TestManagerClientDataFlow(t *testing.T) {
	mockServer := newMockServerInterface()
	// Route SendToAgent through the agents map so messages are recorded
	mockServer.sendToAgentFn = nil
	agent := mockServer.addAgent("agent-1")
	db := testDB(t)

	port := freePort(t)
	m := NewManager(mockServer, db)
	if err := m.StartProxy(&storage.Proxy{
		ID:        "px-1",
		Name:      "test",
		AgentID:   "agent-1",
		ProxyType: "tcp",
		RemotePort: port,
		Target:    "10.0.0.1:80",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("StartProxy() error = %v", err)
	}
	defer m.Stop()

	// A client connects to the proxy port
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()

	// The agent (mock) receives a proxy_new message
	var newMsg *protocol.Message
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, msg := range agent.messages {
			if msg.Type == protocol.MessageTypeProxyNew {
				newMsg = msg
			}
		}
		if newMsg != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if newMsg == nil {
		t.Fatal("agent never received proxy_new")
	}
	connID, _ := newMsg.Payload["connId"].(string)
	if connID == "" {
		t.Fatal("proxy_new missing connId")
	}

	// Client sends data; readFromClient forwards it to the agent
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	foundData := false
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !foundData {
		for _, msg := range agent.messages {
			if msg.Type == protocol.MessageTypeProxyData {
				foundData = true
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !foundData {
		t.Fatal("agent never received forwarded data")
	}

	// Agent responds through HandleProxyData; the client receives the bytes
	if err := m.HandleProxyData("px-1", connID, []byte("pong")); err != nil {
		t.Fatalf("HandleProxyData() error = %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != "pong" {
		t.Errorf("client received %q, want pong", buf[:n])
	}

	// Closing on the agent side tears down the client connection
	m.HandleProxyClose("px-1", connID, "agent closed")
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(buf); err == nil {
		t.Error("client connection should be closed after agent close")
	}
}

func TestManagerHandleProxyDataErrors(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	m := NewManager(mockServer, db)
	defer m.Stop()

	if err := m.HandleProxyData("no-proxy", "c1", []byte("x")); err == nil {
		t.Error("HandleProxyData should fail for unknown proxy")
	}

	port := freePort(t)
	if err := m.StartProxy(&storage.Proxy{
		ID:        "px-1",
		Name:      "test",
		AgentID:   "agent-1",
		ProxyType: "tcp",
		RemotePort: port,
		Target:    "10.0.0.1:80",
	}); err != nil {
		t.Fatalf("StartProxy() error = %v", err)
	}

	if err := m.HandleProxyData("px-1", "no-conn", []byte("x")); err == nil {
		t.Error("HandleProxyData should fail for unknown connection")
	}
}

func TestManagerStartProxyDuplicate(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	m := NewManager(mockServer, db)
	defer m.Stop()

	cfg := &storage.Proxy{ID: "px-dup", Name: "d", AgentID: "a1", ProxyType: "tcp", RemotePort: freePort(t), Target: "10.0.0.1:80"}
	if err := m.StartProxy(cfg); err != nil {
		t.Fatalf("StartProxy() error = %v", err)
	}
	if err := m.StartProxy(cfg); err == nil {
		t.Error("StartProxy should fail when proxy already running")
	}
}

func TestManagerStartProxyPortInUse(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	busyPort := ln.Addr().(*net.TCPAddr).Port

	m := NewManager(mockServer, db)
	defer m.Stop()
	err = m.StartProxy(&storage.Proxy{ID: "px-busy", Name: "b", AgentID: "a1", ProxyType: "tcp", RemotePort: busyPort, Target: "10.0.0.1:80"})
	if err == nil {
		t.Error("StartProxy should fail when the port is already in use")
	}
}

func TestManagerReloadProxy(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	m := NewManager(mockServer, db)
	if err := m.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer m.Stop()

	cfg := &storage.Proxy{ID: "px-reload", Name: "r", AgentID: "a1", ProxyType: "tcp", RemotePort: freePort(t), Target: "10.0.0.1:80", Enabled: true}
	if err := m.ReloadProxy(cfg); err != nil {
		t.Fatalf("ReloadProxy() error = %v", err)
	}
	if _, err := m.GetProxyStatus("px-reload"); err != nil {
		t.Errorf("proxy should run after reload: %v", err)
	}

	// Disabled reload stops the proxy
	cfg.Enabled = false
	if err := m.ReloadProxy(cfg); err != nil {
		t.Fatalf("ReloadProxy(disabled) error = %v", err)
	}
	if _, err := m.GetProxyStatus("px-reload"); err == nil {
		t.Error("proxy should be stopped after disabled reload")
	}
}

func TestManagerStopProxyNotFound(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	m := NewManager(mockServer, db)
	if err := m.StopProxy("ghost"); err == nil {
		t.Error("StopProxy should fail for unknown proxy")
	}
}

func TestManagerStartTwiceFails(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	m := NewManager(mockServer, db)
	if err := m.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer m.Stop()
	if err := m.Start(); err == nil {
		t.Error("second Start() should fail")
	}
}
