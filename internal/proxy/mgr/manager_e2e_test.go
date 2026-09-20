package mgr

// manager_e2e_test.go Manager（server 侧）端到端流程：从 handler_flow_test.go
// 随包边界拆分归位（原与 handler 测试同文件混放）。

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

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
		ID:         "px-db",
		Name:       "db-proxy",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: freePort(t),
		Target:     "127.0.0.1:12345",
		Enabled:    true,
	}); err != nil {
		t.Fatalf("CreateProxy: %v", err)
	}
	// Disabled proxies must not start.
	// Note: created enabled first, then updated, because GORM applies the
	// `default:true` tag to the zero value false on insert.
	offCfg := &storage.Proxy{
		ID:         "px-off",
		Name:       "off",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: freePort(t),
		Target:     "127.0.0.1:12345",
		Enabled:    true,
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
		ID:         "px-1",
		Name:       "test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: port,
		Target:     "10.0.0.1:80",
		Enabled:    true,
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
		for _, msg := range agent.snapshotMessages() {
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
		for _, msg := range agent.snapshotMessages() {
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
		ID:         "px-1",
		Name:       "test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: port,
		Target:     "10.0.0.1:80",
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
