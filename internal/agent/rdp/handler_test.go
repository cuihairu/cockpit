package rdp

import (
	"encoding/base64"
	"image"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestNewHandler(t *testing.T) {
	h := NewHandler()
	if h == nil {
		t.Fatal("NewHandler() should not return nil")
	}
	if h.sessions == nil {
		t.Error("sessions map should be initialized")
	}
}

func TestHandlerStop(t *testing.T) {
	h := NewHandler()
	h.Stop()
	// Should not panic on empty handler
}

func TestHandlerStopWithSessions(t *testing.T) {
	h := NewHandler()
	h.Stop()
	// No sessions to stop, should not panic
}

func TestHandleDesktopDataNoSession(t *testing.T) {
	h := NewHandler()

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopData,
		Payload: map[string]interface{}{
			"sessionId":   "non-existent",
			"desktopType": string(protocol.DesktopMsgKeyboard),
			"scanCode":    float64(28),
			"keyDown":     true,
		},
	}

	// Should not panic for non-existent session
	h.HandleDesktopData(msg)
}

func TestHandleDesktopCloseNoSession(t *testing.T) {
	h := NewHandler()

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopClose,
		Payload: map[string]interface{}{
			"sessionId": "non-existent",
		},
	}

	// Should not panic
	h.HandleDesktopClose(msg)
}

func TestHandleDesktopNewMissingFields(t *testing.T) {
	h := NewHandler()
	var capturedMsg *protocol.Message
	h.SetSendFunc(func(msg *protocol.Message) error {
		capturedMsg = msg
		return nil
	})

	// Missing target
	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopNew,
		Payload: map[string]interface{}{
			"sessionId": "test-1",
		},
	}
	h.HandleDesktopNew(msg)

	if capturedMsg == nil {
		t.Fatal("Should have sent error message")
	}
	if capturedMsg.Payload["desktopType"] != string(protocol.DesktopMsgError) {
		t.Error("Should send error desktopType")
	}
}

func TestHandleDesktopNewInvalidTarget(t *testing.T) {
	// This test verifies error handling for unreachable RDP targets.
	// Skipped in short mode because it waits for a TCP timeout.
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	h := NewHandler()
	errCh := make(chan *protocol.Message, 1)
	h.SetSendFunc(func(msg *protocol.Message) error {
		select {
		case errCh <- msg:
		default:
		}
		return nil
	})

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopNew,
		Payload: map[string]interface{}{
			"sessionId": "test-1",
			"target":    "192.168.255.255:33389",
			"username":  "test",
			"password":  "test",
			"width":     float64(800),
			"height":    float64(600),
		},
	}
	h.HandleDesktopNew(msg)

	// Session creation will fail in a goroutine
	// We just verify it doesn't panic
}

func TestSendFuncError(t *testing.T) {
	h := NewHandler()
	h.SetSendFunc(func(msg *protocol.Message) error {
		return ErrTestSendFailed
	})

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopNew,
		Payload: map[string]interface{}{
			"sessionId": "test-err",
			"target":    "",
		},
	}
	// Should not panic even when sendFunc returns error
	h.HandleDesktopNew(msg)
}

var ErrTestSendFailed = &testError{}

type testError struct{}

func (e *testError) Error() string { return "test send failed" }

// ============ Session 辅助测试 ============

func TestSessionEnqueue(t *testing.T) {
	q := make(chan *protocol.Message, 2)
	s := &Session{
		ID:        "test",
		sendQueue: q,
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	msg := &protocol.Message{Type: protocol.MessageTypeDesktopData}
	s.enqueue(msg)

	if len(q) != 1 {
		t.Error("Message should be in queue")
	}
}

func TestSessionEnqueueFull(t *testing.T) {
	q := make(chan *protocol.Message, 1)
	s := &Session{
		ID:        "test",
		sendQueue: q,
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	// Fill queue
	q <- &protocol.Message{}

	// Enqueue should not block
	msg := &protocol.Message{Type: protocol.MessageTypeDesktopData}
	s.enqueue(msg)

	// Queue should have been drained and refilled
	// Just verify it doesn't deadlock
}

func TestClipboardText(t *testing.T) {
	// 测试剪贴板文本存取（不涉及 grdp client）
	clipboardMu.Lock()
	clipboardText = "hello"
	clipboardMu.Unlock()

	if clipboardText != "hello" {
		t.Error("Clipboard should be 'hello'")
	}

	clipboardMu.Lock()
	clipboardText = "world"
	clipboardMu.Unlock()

	if clipboardText != "world" {
		t.Error("Clipboard should be 'world'")
	}
}

func TestBase64Encoding(t *testing.T) {
	// Verify our base64 encoding is compatible with frontend atob decoding
	pixels := make([]byte, 4) // 1 pixel RGBA
	pixels[0] = 255 // R
	pixels[1] = 0   // G
	pixels[2] = 0   // B
	pixels[3] = 255 // A

	encoded := base64.StdEncoding.EncodeToString(pixels)
	if encoded == "" {
		t.Error("Encoding should not be empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(decoded) != 4 {
		t.Fatalf("Decoded length = %d, want 4", len(decoded))
	}
	if decoded[0] != 255 || decoded[3] != 255 {
		t.Error("Pixel data mismatch after roundtrip")
	}
}

func TestSessionClose(t *testing.T) {
	s := &Session{
		sendQueue: make(chan *protocol.Message, 1),
	}

	if s.IsClosed() {
		t.Error("Session should not be closed initially")
	}

	s.Close()

	if !s.IsClosed() {
		t.Error("Session should be closed after Close()")
	}

	// Double close should not panic
	s.Close()
}
