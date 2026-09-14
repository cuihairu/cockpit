package remote

import (
	"net"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// startTarget starts a TCP echo server and returns its address.
func startTarget(t *testing.T) string {
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

func TestConnectionStartForwarding(t *testing.T) {
	m := NewManager()
	target := startTarget(t)

	conn, err := m.CreateConnection("conn-fwd", protocol.RemoteProtocolSSH, target, 3*time.Second)
	if err != nil {
		t.Fatalf("CreateConnection() error = %v", err)
	}

	client, srv := net.Pipe()
	defer client.Close()
	conn.SetClientConn(srv)

	conn.StartForwarding()

	// Client -> target -> client echo round trip through forward()
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	client.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 16)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != "ping" {
		t.Errorf("echo = %q, want ping", buf[:n])
	}

	// Closing the client ends forwarding and marks the connection closed
	client.Close()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !conn.closed.Load() {
		time.Sleep(10 * time.Millisecond)
	}
	if !conn.closed.Load() {
		t.Error("connection should be closed after client disconnect")
	}
}

func TestConnectionHandleDataDirections(t *testing.T) {
	m := NewManager()
	target := startTarget(t)

	conn, err := m.CreateConnection("conn-data", protocol.RemoteProtocolSSH, target, 3*time.Second)
	if err != nil {
		t.Fatalf("CreateConnection() error = %v", err)
	}

	// No client conn yet: data from target cannot be delivered
	if err := conn.HandleData([]byte("x"), false); err == nil {
		t.Error("HandleData(from target) should fail without client conn")
	}

	// Client -> target direction works (echo target receives the bytes)
	client, srv := net.Pipe()
	defer client.Close()
	defer srv.Close()
	conn.SetClientConn(srv)

	if err := conn.HandleData([]byte("to-target"), true); err != nil {
		t.Fatalf("HandleData(client->target) error = %v", err)
	}
}

func TestConnectionHandleDataWriteError(t *testing.T) {
	m := NewManager()
	target := startTarget(t)

	conn, err := m.CreateConnection("conn-werr", protocol.RemoteProtocolSSH, target, 3*time.Second)
	if err != nil {
		t.Fatalf("CreateConnection() error = %v", err)
	}
	conn.TargetConn.Close() // force write failure

	if err := conn.HandleData([]byte("boom"), true); err == nil {
		t.Error("HandleData should fail when the target conn is closed")
	}
	if !conn.closed.Load() {
		t.Error("connection should be marked closed after write failure")
	}
}

func TestManagerCleanupIdle(t *testing.T) {
	m := NewManager()
	target := startTarget(t)

	fresh, err := m.CreateConnection("conn-fresh", protocol.RemoteProtocolSSH, target, 3*time.Second)
	if err != nil {
		t.Fatalf("CreateConnection(fresh) error = %v", err)
	}

	idle, err := m.CreateConnection("conn-idle", protocol.RemoteProtocolSSH, target, 3*time.Second)
	if err != nil {
		t.Fatalf("CreateConnection(idle) error = %v", err)
	}
	// Simulate an old LastActive timestamp
	idle.mu.Lock()
	idle.LastActive = time.Now().Add(-6 * time.Minute)
	idle.mu.Unlock()

	m.cleanupIdle()

	if _, ok := m.GetConnection("conn-fresh"); !ok {
		t.Error("fresh connection must survive cleanupIdle")
	}
	if _, ok := m.GetConnection("conn-idle"); ok {
		t.Error("idle connection must be removed by cleanupIdle")
	}
	if !idle.closed.Load() {
		t.Error("idle connection should be closed")
	}
	_ = fresh
}

func TestCloseConnectionDoubleClose(t *testing.T) {
	m := NewManager()
	target := startTarget(t)

	if _, err := m.CreateConnection("conn-2x", protocol.RemoteProtocolSSH, target, 3*time.Second); err != nil {
		t.Fatalf("CreateConnection() error = %v", err)
	}
	if err := m.CloseConnection("conn-2x"); err != nil {
		t.Fatalf("CloseConnection() error = %v", err)
	}
	if err := m.CloseConnection("conn-2x"); err == nil {
		t.Error("second CloseConnection should fail for missing connection")
	}
}

func TestCreateConnectionDuplicate(t *testing.T) {
	m := NewManager()
	target := startTarget(t)

	if _, err := m.CreateConnection("conn-dup", protocol.RemoteProtocolSSH, target, 3*time.Second); err != nil {
		t.Fatalf("CreateConnection() error = %v", err)
	}
	if _, err := m.CreateConnection("conn-dup", protocol.RemoteProtocolSSH, target, 3*time.Second); err == nil {
		t.Error("duplicate CreateConnection should fail")
	}
}
