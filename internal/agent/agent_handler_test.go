package agent

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestHandleMessagePing(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	msg := protocol.NewMessage(protocol.MessageTypePing, map[string]any{"status": "ping"})

	// Should not panic even without connection
	a.handleMessage(msg)
}

func TestHandleMessageRPCRequest(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	msg := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]any{
		"method": "list_containers",
	})

	// Should not panic even without connection
	a.handleMessage(msg)
}

func TestHandleMessageHeartbeat(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	msg := protocol.NewMessage(protocol.MessageTypeHeartbeat, nil)
	a.handleMessage(msg)
}

func TestHandleMessageProxyNew(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	msg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]any{
		"sessionId": "test",
		"target":    "127.0.0.1:22",
	})
	a.handleMessage(msg)
}

func TestHandleMessageProxyData(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	msg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]any{
		"sessionId": "test",
		"data":      []byte{1, 2, 3},
	})
	a.handleMessage(msg)
}

func TestHandleMessageProxyClose(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	msg := protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]any{
		"sessionId": "test",
	})
	a.handleMessage(msg)
}

func TestHandleMessageDesktopNew(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	msg := protocol.NewMessage(protocol.MessageTypeDesktopNew, map[string]any{
		"sessionId": "desktop-1",
		"target":    "192.168.1.100",
	})
	a.handleMessage(msg)
}

func TestHandleMessageDesktopData(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	msg := protocol.NewMessage(protocol.MessageTypeDesktopData, map[string]any{
		"sessionId": "desktop-1",
		"type":      "keyboard",
	})
	a.handleMessage(msg)
}

func TestHandleMessageDesktopClose(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	msg := protocol.NewMessage(protocol.MessageTypeDesktopClose, map[string]any{
		"sessionId": "desktop-1",
	})
	a.handleMessage(msg)
}

func TestHandleMessageUnknown(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	msg := protocol.NewMessage("unknown_type", nil)
	a.handleMessage(msg)
}

func TestHandlePingNoConnection(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	msg := protocol.NewMessage(protocol.MessageTypePing, nil)
	msg.ID = "ping-123"

	// Should not panic with nil conn
	a.handlePing(msg)
}

func TestHandleRPCRequestNoConnection(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	msg := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]any{
		"method":  "nonexistent",
		"params":  map[string]any{},
	})
	msg.ID = "rpc-123"

	// Should not panic with nil conn
	a.handleRPCRequest(msg)
}

func TestSendHeartbeatNoConnection(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})

	// Should return early without panicking (conn is nil)
	a.sendHeartbeat()
}

func TestAgentSubHandlersInitialized(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})

	if a.proxyHandler == nil {
		t.Error("proxyHandler should be initialized")
	}
	if a.desktopHandler == nil {
		t.Error("desktopHandler should be initialized")
	}
	if a.rpc == nil {
		t.Error("rpc handler should be initialized")
	}
	if a.collector == nil {
		t.Error("collector should be initialized")
	}
	if a.codec == nil {
		t.Error("codec should be initialized")
	}
}

func TestHandleMessageMultipleTypes(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})

	types := []protocol.MessageType{
		protocol.MessageTypePing,
		protocol.MessageTypeRPCRequest,
		protocol.MessageTypeHeartbeat,
		protocol.MessageTypeProxyNew,
		protocol.MessageTypeProxyData,
		protocol.MessageTypeProxyClose,
		protocol.MessageTypeDesktopNew,
		protocol.MessageTypeDesktopData,
		protocol.MessageTypeDesktopClose,
		"unknown_message_type",
	}

	for _, msgType := range types {
		msg := protocol.NewMessage(msgType, map[string]any{"test": true})
		a.handleMessage(msg) // Should not panic for any type
	}
}

func TestStopMultipleTimes(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://localhost:8080"})

	a.Stop()
	a.Stop() // Should not panic on second stop
	a.Stop() // Or third
}
