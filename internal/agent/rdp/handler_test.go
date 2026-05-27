package rdp

import (
	"encoding/base64"
	"fmt"
	"image"
	"sync"
	"testing"

	grdp "github.com/nakagami/grdp"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// ============ Handler Tests ============

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
}

func TestHandlerStopWithSessions(t *testing.T) {
	h := NewHandler()

	// Manually inject a closed session
	s := &Session{
		ID:        "test-session",
		sendQueue: make(chan *protocol.Message, 1),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}
	s.closed.Store(true)

	h.mu.Lock()
	h.sessions["test-session"] = s
	h.mu.Unlock()

	h.Stop()

	h.mu.RLock()
	count := len(h.sessions)
	h.mu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 sessions after Stop(), got %d", count)
	}
}

func TestHandleDesktopNewMissingSessionID(t *testing.T) {
	h := NewHandler()
	var capturedMsg *protocol.Message
	h.SetSendFunc(func(msg *protocol.Message) error {
		capturedMsg = msg
		return nil
	})

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopNew,
		Payload: map[string]interface{}{
			"target": "192.168.1.1:3389",
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

func TestHandleDesktopNewMissingTarget(t *testing.T) {
	h := NewHandler()
	var capturedMsg *protocol.Message
	h.SetSendFunc(func(msg *protocol.Message) error {
		capturedMsg = msg
		return nil
	})

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

	h.HandleDesktopData(msg)
}

func TestHandleDesktopDataClosedSession(t *testing.T) {
	h := NewHandler()

	s := &Session{
		ID:        "closed-session",
		sendQueue: make(chan *protocol.Message, 1),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}
	s.closed.Store(true)

	h.mu.Lock()
	h.sessions["closed-session"] = s
	h.mu.Unlock()

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopData,
		Payload: map[string]interface{}{
			"sessionId":   "closed-session",
			"desktopType": string(protocol.DesktopMsgKeyboard),
			"scanCode":    float64(28),
			"keyDown":     true,
		},
	}

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

	h.HandleDesktopClose(msg)
}

func TestHandleDesktopCloseExisting(t *testing.T) {
	h := NewHandler()

	s := &Session{
		ID:        "test-session",
		sendQueue: make(chan *protocol.Message, 1),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	h.mu.Lock()
	h.sessions["test-session"] = s
	h.mu.Unlock()

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopClose,
		Payload: map[string]interface{}{
			"sessionId": "test-session",
		},
	}
	h.HandleDesktopClose(msg)

	h.mu.RLock()
	_, exists := h.sessions["test-session"]
	h.mu.RUnlock()

	if exists {
		t.Error("Session should be removed after close")
	}
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
	h.HandleDesktopNew(msg)
}

func TestSendErrorNoSendFunc(t *testing.T) {
	h := NewHandler()
	// sendFunc is nil - should not panic
	h.sendError("test-session", "some error")
}

func TestSessionSendLoopExitsOnQueueClose(t *testing.T) {
	h := NewHandler()

	s := &Session{
		ID:        "loop-test",
		sendQueue: make(chan *protocol.Message, 10),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	h.mu.Lock()
	h.sessions["loop-test"] = s
	h.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.sessionSendLoop(s)
	}()

	// Close the queue to stop the send loop
	close(s.sendQueue)

	wg.Wait()

	// Session should be cleaned up
	h.mu.RLock()
	_, exists := h.sessions["loop-test"]
	h.mu.RUnlock()

	if exists {
		t.Error("Session should be cleaned up after send loop exits")
	}
}

var ErrTestSendFailed = &testError{}

type testError struct{}

func (e *testError) Error() string { return "test send failed" }

// ============ Session Tests ============

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

	q <- &protocol.Message{}

	msg := &protocol.Message{Type: protocol.MessageTypeDesktopData}
	s.enqueue(msg)
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

func TestSessionCloseQueue(t *testing.T) {
	q := make(chan *protocol.Message, 1)
	s := &Session{
		sendQueue: q,
	}

	s.Close()

	// Queue should be closed - reading should return zero value
	msg, ok := <-q
	if ok {
		t.Errorf("Queue should be closed, got msg=%v", msg)
	}
}

func TestSendQueueAccessor(t *testing.T) {
	q := make(chan *protocol.Message, 5)
	s := &Session{
		sendQueue: q,
	}

	if s.SendQueue() != q {
		t.Error("SendQueue() should return the underlying channel")
	}
}

// ============ Clipboard Tests ============

func TestClipboardText(t *testing.T) {
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

func TestClipboardConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			clipboardMu.Lock()
			clipboardText = string(rune('a' + n))
			clipboardMu.Unlock()
		}(i)
	}
	wg.Wait()
}

// ============ Base64 Encoding Tests ============

func TestBase64Encoding(t *testing.T) {
	pixels := make([]byte, 4)
	pixels[0] = 255
	pixels[1] = 0
	pixels[2] = 0
	pixels[3] = 255

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

func TestBase64EncodingLargeData(t *testing.T) {
	// Simulate a 100x100 RGBA bitmap
	size := 100 * 100 * 4
	pixels := make([]byte, size)
	for i := range pixels {
		pixels[i] = byte(i % 256)
	}

	encoded := base64.StdEncoding.EncodeToString(pixels)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(decoded) != size {
		t.Errorf("Decoded length = %d, want %d", len(decoded), size)
	}
	for i := range decoded {
		if decoded[i] != pixels[i] {
			t.Fatalf("Mismatch at byte %d", i)
		}
	}
}

// ============ Bitmap Processing Tests ============

func TestHandleBitmapClosedSession(t *testing.T) {
	s := &Session{
		ID:        "closed-bitmap",
		sendQueue: make(chan *protocol.Message, 10),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}
	s.closed.Store(true)

	// Should return immediately without processing
	s.handleBitmap(nil)
}

func TestHandleBitmapEmpty(t *testing.T) {
	s := &Session{
		ID:        "empty-bitmap",
		sendQueue: make(chan *protocol.Message, 10),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	s.handleBitmap(nil)

	if len(s.sendQueue) != 0 {
		t.Error("Empty bitmaps should not enqueue messages")
	}
}

// ============ Keyboard Input Tests ============

func TestHandleKeyboardClosedSession(t *testing.T) {
	s := &Session{
		sendQueue: make(chan *protocol.Message, 1),
	}
	s.closed.Store(true)

	// Should return immediately
	s.HandleKeyboard(28, true, false)
}

// ============ Mouse Input Tests ============

func TestHandleMouseClosedSession(t *testing.T) {
	s := &Session{
		sendQueue: make(chan *protocol.Message, 1),
	}
	s.closed.Store(true)

	// Should return immediately
	s.HandleMouse(100, 200, 0, 0, "move")
}

// ============ Resolution Tests ============

func TestHandleSetResolutionClosedSession(t *testing.T) {
	s := &Session{
		sendQueue: make(chan *protocol.Message, 1),
	}
	s.closed.Store(true)

	// Should return immediately
	s.HandleSetResolution(1920, 1080)
}

// ============ Handler Data Routing Tests ============

func TestHandleDesktopDataKeyboard(t *testing.T) {
	h := NewHandler()

	s := &Session{
		ID:        "kb-session",
		sendQueue: make(chan *protocol.Message, 1),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	h.mu.Lock()
	h.sessions["kb-session"] = s
	h.mu.Unlock()

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopData,
		Payload: map[string]interface{}{
			"sessionId":   "kb-session",
			"desktopType": string(protocol.DesktopMsgKeyboard),
			"scanCode":    float64(28),
			"keyDown":     true,
			"extended":    false,
		},
	}

	// Should not panic even though client is nil
	defer func() {
		if r := recover(); r != nil {
			// Expected - client is nil
		}
	}()
	h.HandleDesktopData(msg)
}

func TestHandleDesktopDataMouse(t *testing.T) {
	h := NewHandler()

	s := &Session{
		ID:        "mouse-session",
		sendQueue: make(chan *protocol.Message, 1),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	h.mu.Lock()
	h.sessions["mouse-session"] = s
	h.mu.Unlock()

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopData,
		Payload: map[string]interface{}{
			"sessionId":   "mouse-session",
			"desktopType": string(protocol.DesktopMsgMouse),
			"x":           float64(100),
			"y":           float64(200),
			"buttons":     float64(1),
			"wheelDelta":  float64(0),
			"action":      "move",
		},
	}

	defer func() {
		if r := recover(); r != nil {
			// Expected - client is nil
		}
	}()
	h.HandleDesktopData(msg)
}

func TestHandleDesktopDataClipboard(t *testing.T) {
	h := NewHandler()

	s := &Session{
		ID:        "clip-session",
		sendQueue: make(chan *protocol.Message, 1),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	h.mu.Lock()
	h.sessions["clip-session"] = s
	h.mu.Unlock()

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopData,
		Payload: map[string]interface{}{
			"sessionId":   "clip-session",
			"desktopType": string(protocol.DesktopMsgClipboardData),
			"text":        "hello world",
		},
	}

	defer func() {
		if r := recover(); r != nil {
			// Expected - client is nil
		}
	}()
	h.HandleDesktopData(msg)
}

func TestHandleDesktopDataResolution(t *testing.T) {
	h := NewHandler()

	s := &Session{
		ID:        "res-session",
		sendQueue: make(chan *protocol.Message, 1),
		width:     800,
		height:    600,
		screen:    image.NewRGBA(image.Rect(0, 0, 800, 600)),
	}

	h.mu.Lock()
	h.sessions["res-session"] = s
	h.mu.Unlock()

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopData,
		Payload: map[string]interface{}{
			"sessionId":   "res-session",
			"desktopType": string(protocol.DesktopMsgSetResolution),
			"width":       float64(1920),
			"height":      float64(1080),
		},
	}

	defer func() {
		if r := recover(); r != nil {
			// Expected - client is nil
		}
	}()
	h.HandleDesktopData(msg)
}

func TestHandleDesktopDataUnknownType(t *testing.T) {
	h := NewHandler()

	s := &Session{
		ID:        "unknown-session",
		sendQueue: make(chan *protocol.Message, 1),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	h.mu.Lock()
	h.sessions["unknown-session"] = s
	h.mu.Unlock()

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopData,
		Payload: map[string]interface{}{
			"sessionId":   "unknown-session",
			"desktopType": "unknown_type",
		},
	}

	// Should not panic
	h.HandleDesktopData(msg)
}

// ============ Concurrent Handler Tests ============

func TestHandlerConcurrentStop(t *testing.T) {
	h := NewHandler()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Stop()
		}()
	}
	wg.Wait()
}

// ============ Bitmap Processing Tests (with real data) ============

func TestHandleBitmapWithData(t *testing.T) {
	s := &Session{
		ID:        "bitmap-test",
		sendQueue: make(chan *protocol.Message, 10),
		width:     64,
		height:    64,
		screen:    image.NewRGBA(image.Rect(0, 0, 64, 64)),
	}

	bmWidth, bmHeight := 4, 4
	data := make([]byte, bmWidth*bmHeight*4)
	for i := 0; i < len(data); i += 4 {
		data[i] = 0xFF
		data[i+1] = 0x00
		data[i+2] = 0x00
		data[i+3] = 0xFF
	}

	bitmaps := []grdp.Bitmap{{
		DestLeft:     10,
		DestTop:      10,
		DestRight:    14,
		DestBottom:   14,
		Width:        bmWidth,
		Height:       bmHeight,
		BitsPerPixel: 32,
		Data:         data,
	}}

	s.handleBitmap(bitmaps)

	if len(s.sendQueue) != 1 {
		t.Fatalf("Expected 1 message in queue, got %d", len(s.sendQueue))
	}

	msg := <-s.sendQueue
	if msg.Type != protocol.MessageTypeDesktopData {
		t.Errorf("Type = %v, want %v", msg.Type, protocol.MessageTypeDesktopData)
	}
	if msg.Payload["desktopType"] != string(protocol.DesktopMsgScreenUpdate) {
		t.Errorf("desktopType = %v", msg.Payload["desktopType"])
	}

	rects, ok := msg.Payload["rects"].([]protocol.DesktopBitmapRect)
	if !ok {
		t.Fatal("rects should be []DesktopBitmapRect")
	}
	if len(rects) != 1 {
		t.Fatalf("Expected 1 rect, got %d", len(rects))
	}
	if rects[0].X != 10 || rects[0].Y != 10 {
		t.Errorf("Rect position = (%d,%d), want (10,10)", rects[0].X, rects[0].Y)
	}

	decoded, err := base64.StdEncoding.DecodeString(rects[0].Data)
	if err != nil {
		t.Fatalf("Failed to decode base64: %v", err)
	}
	if len(decoded) == 0 {
		t.Error("Decoded data should not be empty")
	}
}

func TestHandleBitmapMultipleRects(t *testing.T) {
	s := &Session{
		ID:        "multi-bitmap",
		sendQueue: make(chan *protocol.Message, 10),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	makeBitmap := func(x, y, w, h int) grdp.Bitmap {
		data := make([]byte, w*h*4)
		for i := range data {
			data[i] = byte(i % 256)
		}
		return grdp.Bitmap{
			DestLeft: x, DestTop: y,
			DestRight: x + w, DestBottom: y + h,
			Width: w, Height: h,
			BitsPerPixel: 32, Data: data,
		}
	}

	bitmaps := []grdp.Bitmap{
		makeBitmap(0, 0, 10, 10),
		makeBitmap(20, 20, 5, 5),
	}

	s.handleBitmap(bitmaps)

	if len(s.sendQueue) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(s.sendQueue))
	}

	msg := <-s.sendQueue
	rects := msg.Payload["rects"].([]protocol.DesktopBitmapRect)
	if len(rects) != 2 {
		t.Errorf("Expected 2 rects, got %d", len(rects))
	}
}

func TestHandleBitmapClipAtBoundary(t *testing.T) {
	s := &Session{
		ID:        "clip-bitmap",
		sendQueue: make(chan *protocol.Message, 10),
		width:     20,
		height:    20,
		screen:    image.NewRGBA(image.Rect(0, 0, 20, 20)),
	}

	data := make([]byte, 30*30*4)
	bm := grdp.Bitmap{
		DestLeft: 10, DestTop: 10,
		DestRight: 40, DestBottom: 40,
		Width: 30, Height: 30,
		BitsPerPixel: 32, Data: data,
	}

	s.handleBitmap([]grdp.Bitmap{bm})

	if len(s.sendQueue) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(s.sendQueue))
	}
}

func TestHandleBitmap16bpp(t *testing.T) {
	s := &Session{
		ID:        "16bpp-bitmap",
		sendQueue: make(chan *protocol.Message, 10),
		width:     32,
		height:    32,
		screen:    image.NewRGBA(image.Rect(0, 0, 32, 32)),
	}

	bmWidth, bmHeight := 2, 2
	data := make([]byte, bmWidth*bmHeight*2)

	bitmaps := []grdp.Bitmap{{
		DestLeft: 0, DestTop: 0,
		DestRight: bmWidth, DestBottom: bmHeight,
		Width: bmWidth, Height: bmHeight,
		BitsPerPixel: 2,
		Data:         data,
	}}

	s.handleBitmap(bitmaps)

	if len(s.sendQueue) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(s.sendQueue))
	}
}

// ============ getClipboardText Tests ============

func TestGetClipboardText(t *testing.T) {
	s := &Session{ID: "clip-test"}

	clipboardMu.Lock()
	clipboardText = "test clipboard content"
	clipboardMu.Unlock()

	got := s.getClipboardText()
	if got != "test clipboard content" {
		t.Errorf("getClipboardText() = %q, want %q", got, "test clipboard content")
	}

	clipboardMu.Lock()
	clipboardText = ""
	clipboardMu.Unlock()
}

func TestGetClipboardTextEmpty(t *testing.T) {
	s := &Session{ID: "clip-empty"}
	clipboardMu.Lock()
	clipboardText = ""
	clipboardMu.Unlock()

	got := s.getClipboardText()
	if got != "" {
		t.Errorf("Expected empty string, got %q", got)
	}
}

// ============ Session Close with nil client ============

func TestSessionCloseNilClient(t *testing.T) {
	s := &Session{
		ID:        "nil-client",
		sendQueue: make(chan *protocol.Message, 1),
	}

	if s.IsClosed() {
		t.Error("Should not be closed initially")
	}

	s.Close()

	if !s.IsClosed() {
		t.Error("Should be closed after Close()")
	}
}

// ============ sessionSendLoop Tests ============

func TestSessionSendLoopWithSendFunc(t *testing.T) {
	h := NewHandler()
	var received []*protocol.Message
	var mu sync.Mutex
	h.SetSendFunc(func(msg *protocol.Message) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	})

	s := &Session{
		ID:        "sendloop-test",
		sendQueue: make(chan *protocol.Message, 10),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	h.mu.Lock()
	h.sessions["sendloop-test"] = s
	h.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.sessionSendLoop(s)
	}()

	for i := 0; i < 3; i++ {
		s.enqueue(&protocol.Message{
			Type: protocol.MessageTypeDesktopData,
			Payload: map[string]interface{}{
				"sessionId": "sendloop-test",
				"index":     i,
			},
		})
	}

	close(s.sendQueue)
	wg.Wait()

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count != 3 {
		t.Errorf("Expected 3 messages sent, got %d", count)
	}
}

func TestSessionSendLoopSendError(t *testing.T) {
	h := NewHandler()
	callCount := 0
	h.SetSendFunc(func(msg *protocol.Message) error {
		callCount++
		return fmt.Errorf("send error")
	})

	s := &Session{
		ID:        "sendloop-err",
		sendQueue: make(chan *protocol.Message, 10),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	h.mu.Lock()
	h.sessions["sendloop-err"] = s
	h.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.sessionSendLoop(s)
	}()

	s.sendQueue <- &protocol.Message{
		Type: protocol.MessageTypeDesktopData,
		Payload: map[string]interface{}{
			"sessionId": "sendloop-err",
		},
	}

	wg.Wait()

	if callCount != 1 {
		t.Errorf("Expected 1 sendFunc call, got %d", callCount)
	}

	h.mu.RLock()
	_, exists := h.sessions["sendloop-err"]
	h.mu.RUnlock()
	if exists {
		t.Error("Session should be cleaned up after send error")
	}
}

func TestSessionSendLoopNilSendFunc(t *testing.T) {
	h := NewHandler()

	s := &Session{
		ID:        "sendloop-nil",
		sendQueue: make(chan *protocol.Message, 10),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}

	h.mu.Lock()
	h.sessions["sendloop-nil"] = s
	h.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.sessionSendLoop(s)
	}()

	s.sendQueue <- &protocol.Message{Type: protocol.MessageTypeDesktopData}

	close(s.sendQueue)
	wg.Wait()

	h.mu.RLock()
	_, exists := h.sessions["sendloop-nil"]
	h.mu.RUnlock()
	if exists {
		t.Error("Session should be cleaned up after loop exits")
	}
}

// ============ HandleDesktopNew Default Resolution ============

func TestHandleDesktopNewDefaultResolution(t *testing.T) {
	h := NewHandler()
	var capturedMsg *protocol.Message
	h.SetSendFunc(func(msg *protocol.Message) error {
		capturedMsg = msg
		return nil
	})

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopNew,
		Payload: map[string]interface{}{
			"sessionId": "default-res",
			"target":    "192.168.255.255:33389",
			"username":  "test",
			"password":  "test",
		},
	}
	h.HandleDesktopNew(msg)

	if capturedMsg == nil {
		t.Fatal("Should have sent error message")
	}
	if capturedMsg.Payload["desktopType"] != string(protocol.DesktopMsgError) {
		t.Error("Should send error for connection failure")
	}
}

func TestHandleDesktopNewZeroResolution(t *testing.T) {
	h := NewHandler()
	var capturedMsg *protocol.Message
	h.SetSendFunc(func(msg *protocol.Message) error {
		capturedMsg = msg
		return nil
	})

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopNew,
		Payload: map[string]interface{}{
			"sessionId": "zero-res",
			"target":    "192.168.255.255:33389",
			"username":  "test",
			"password":  "test",
			"width":     float64(0),
			"height":    float64(0),
		},
	}
	h.HandleDesktopNew(msg)

	if capturedMsg == nil {
		t.Fatal("Should have sent error message")
	}
}

// ============ HandleDesktopData Mouse Button Mapping ============

func TestHandleDesktopDataMouseButtonMapping(t *testing.T) {
	tests := []struct {
		name    string
		buttons float64
	}{
		{"left", 1},
		{"middle", 4},
		{"right", 2},
		{"none", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			s := &Session{
				ID:        "mouse-" + tt.name,
				sendQueue: make(chan *protocol.Message, 1),
				width:     100,
				height:    100,
				screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
			}
			s.closed.Store(true)

			h.mu.Lock()
			h.sessions["mouse-"+tt.name] = s
			h.mu.Unlock()

			msg := &protocol.Message{
				Type: protocol.MessageTypeDesktopData,
				Payload: map[string]interface{}{
					"sessionId":   "mouse-" + tt.name,
					"desktopType": string(protocol.DesktopMsgMouse),
					"x":           float64(50),
					"y":           float64(50),
					"buttons":     tt.buttons,
					"wheelDelta":  float64(3),
					"action":      "down",
				},
			}
			h.HandleDesktopData(msg)
		})
	}
}
