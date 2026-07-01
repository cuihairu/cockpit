//go:build !rdp || darwin

package rdp

import "github.com/cuihairu/cockpit/internal/protocol"

// Handler reports an explicit error when RDP support is not built in.
type Handler struct {
	sessions map[string]*Session
	sendFunc func(msg *protocol.Message) error
}

func NewHandler() *Handler {
	return &Handler{
		sessions: make(map[string]*Session),
	}
}

func (h *Handler) SetSendFunc(fn func(msg *protocol.Message) error) {
	h.sendFunc = fn
}

func (h *Handler) HandleDesktopNew(msg *protocol.Message) {
	sessionID := ""
	if msg != nil && msg.Payload != nil {
		sessionID, _ = msg.Payload["sessionId"].(string)
	}
	h.sendError(sessionID, "RDP support is not enabled in this agent build; rebuild cockpit-agent with -tags rdp on a supported non-darwin platform")
}

func (h *Handler) HandleDesktopData(msg *protocol.Message) {}

func (h *Handler) HandleDesktopClose(msg *protocol.Message) {}

func (h *Handler) Stop() {}

func (h *Handler) sendError(sessionID, errMsg string) {
	if h.sendFunc == nil {
		return
	}
	_ = h.sendFunc(&protocol.Message{
		Type: protocol.MessageTypeDesktopData,
		Payload: map[string]interface{}{
			"sessionId":   sessionID,
			"desktopType": string(protocol.DesktopMsgError),
			"error":       errMsg,
		},
	})
}

// Session is a no-op RDP session used when RDP support is not built in.
type Session struct {
	ID        string
	sendQueue chan *protocol.Message
	width     int
	height    int
	closed    closedBool
}

type closedBool struct {
	value bool
}

func (c *closedBool) Load() bool {
	return c.value
}

func (c *closedBool) CompareAndSwap(old, new bool) bool {
	if c.value != old {
		return false
	}
	c.value = new
	return true
}

func (c *closedBool) Store(value bool) {
	c.value = value
}

func NewSession(sessionID, target, domain, username, password string, width, height int) (*Session, error) {
	return &Session{
		ID:        sessionID,
		sendQueue: make(chan *protocol.Message, 60),
		width:     width,
		height:    height,
	}, nil
}

func (s *Session) HandleKeyboard(scanCode uint16, keyDown bool, extended bool) {}

func (s *Session) HandleMouse(x, y, button, wheelDelta int, action string) {}

func (s *Session) HandleClipboard(text string) {}

func (s *Session) HandleSetResolution(width, height int) {}

func (s *Session) Close() {
	if s.closed.CompareAndSwap(false, true) {
		close(s.sendQueue)
	}
}

func (s *Session) SendQueue() <-chan *protocol.Message {
	return s.sendQueue
}

func (s *Session) IsClosed() bool {
	return s.closed.Load()
}
