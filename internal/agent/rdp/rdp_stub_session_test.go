//go:build !rdp || darwin

package rdp

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestStubHandlerNoopMethods(t *testing.T) {
	h := NewHandler()

	// Must not panic regardless of payload shape
	h.HandleDesktopData(protocol.NewMessage(protocol.MessageTypeDesktopData, map[string]interface{}{
		"sessionId": "s1",
		"data":      []byte{1, 2},
	}))
	h.HandleDesktopData(nil)
	h.HandleDesktopClose(protocol.NewMessage(protocol.MessageTypeDesktopClose, map[string]interface{}{
		"sessionId": "s1",
	}))
	h.HandleDesktopClose(nil)
	h.Stop()
}

func TestStubHandleDesktopNewWithoutSendFunc(t *testing.T) {
	h := NewHandler()
	// sendFunc is nil: sendError must return silently
	h.HandleDesktopNew(protocol.NewMessage(protocol.MessageTypeDesktopNew, map[string]interface{}{
		"sessionId": "s1",
	}))
}

func TestStubHandleDesktopNewNilPayload(t *testing.T) {
	h := NewHandler()
	var captured *protocol.Message
	h.SetSendFunc(func(msg *protocol.Message) error {
		captured = msg
		return nil
	})

	h.HandleDesktopNew(nil)
	if captured == nil {
		t.Fatal("expected error message even for nil message")
	}
	if captured.Payload["sessionId"] != "" {
		t.Errorf("sessionId = %v, want empty", captured.Payload["sessionId"])
	}
}

func TestStubNewSession(t *testing.T) {
	s, err := NewSession("sess-1", "10.0.0.1:3389", "DOM", "user", "pass", 1920, 1080)
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if s.ID != "sess-1" {
		t.Errorf("ID = %q, want sess-1", s.ID)
	}
	if s.IsClosed() {
		t.Error("new session must not be closed")
	}

	// Input handlers are no-ops
	s.HandleKeyboard(0x1E, true, false)
	s.HandleMouse(100, 200, 0, 0, "move")
	s.HandleClipboard("copied")
	s.HandleSetResolution(1280, 720)

	if s.SendQueue() == nil {
		t.Error("SendQueue() should not be nil")
	}
}

func TestStubSessionClose(t *testing.T) {
	s, _ := NewSession("sess-2", "target", "", "", "", 800, 600)

	// Queue is open and empty before close
	select {
	case _, open := <-s.SendQueue():
		if open {
			t.Fatal("queue should be empty before close")
		}
	default:
	}

	s.Close()
	if !s.IsClosed() {
		t.Error("session should be closed after Close()")
	}
	if _, open := <-s.SendQueue(); open {
		t.Error("SendQueue should be closed after Close()")
	}

	// Double close must not panic (channel close guard)
	s.Close()
}

func TestStubClosedBool(t *testing.T) {
	var c closedBool

	if c.Load() {
		t.Error("initial value should be false")
	}

	if !c.CompareAndSwap(false, true) {
		t.Error("CAS(false->true) should succeed")
	}
	if c.CompareAndSwap(false, true) {
		t.Error("second CAS(false->true) should fail")
	}
	if !c.Load() {
		t.Error("value should be true after CAS")
	}

	c.Store(false)
	if c.Load() {
		t.Error("Store(false) should reset value")
	}
}
