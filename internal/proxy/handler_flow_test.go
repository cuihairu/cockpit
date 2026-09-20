package proxy

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
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

