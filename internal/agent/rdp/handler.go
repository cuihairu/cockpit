package rdp

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// Handler Agent 端桌面消息路由处理器
type Handler struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	sendFunc func(msg *protocol.Message) error
}

// NewHandler 创建桌面处理器
func NewHandler() *Handler {
	return &Handler{
		sessions: make(map[string]*Session),
	}
}

// SetSendFunc 设置消息发送函数（由 Agent 注入）
func (h *Handler) SetSendFunc(fn func(msg *protocol.Message) error) {
	h.sendFunc = fn
}

// HandleDesktopNew 处理新建桌面会话请求
func (h *Handler) HandleDesktopNew(msg *protocol.Message) {
	payload := msg.Payload

	sessionID, _ := payload["sessionId"].(string)
	target, _ := payload["target"].(string)
	username, _ := payload["username"].(string)
	password, _ := payload["password"].(string)
	domain, _ := payload["domain"].(string)

	var width, height int
	if w, ok := payload["width"].(float64); ok {
		width = int(w)
	}
	if hh, ok := payload["height"].(float64); ok {
		height = int(hh)
	}

	if sessionID == "" || target == "" {
		slog.Error("HandleDesktopNew: missing sessionId or target")
		h.sendError(sessionID, "missing sessionId or target")
		return
	}

	if width <= 0 {
		width = 1280
	}
	if height <= 0 {
		height = 800
	}

	slog.Info("Creating RDP session", "sessionID", sessionID, "target", target, "resolution", fmt.Sprintf("%dx%d", width, height))

	session, err := NewSession(sessionID, target, domain, username, password, width, height)
	if err != nil {
		slog.Error("Failed to create RDP session", "sessionID", sessionID, "error", err)
		h.sendError(sessionID, fmt.Sprintf("RDP connection failed: %v", err))
		return
	}

	h.mu.Lock()
	h.sessions[sessionID] = session
	h.mu.Unlock()

	// 启动发送协程，将 session 队列中的消息转发到 Agent 的 WebSocket
	go h.sessionSendLoop(session)
}

// HandleDesktopData 处理桌面数据消息（浏览器 -> Agent）
func (h *Handler) HandleDesktopData(msg *protocol.Message) {
	payload := msg.Payload

	sessionID, _ := payload["sessionId"].(string)
	desktopType, _ := payload["desktopType"].(string)

	h.mu.RLock()
	session, exists := h.sessions[sessionID]
	h.mu.RUnlock()

	if !exists || session.IsClosed() {
		return
	}

	switch protocol.DesktopMessageType(desktopType) {
	case protocol.DesktopMsgKeyboard:
		scanCode := uint16(0)
		if sc, ok := payload["scanCode"].(float64); ok {
			scanCode = uint16(sc)
		}
		keyDown, _ := payload["keyDown"].(bool)
		extended, _ := payload["extended"].(bool)
		session.HandleKeyboard(scanCode, keyDown, extended)

	case protocol.DesktopMsgMouse:
		var x, y, buttons, wheelDelta int
		if v, ok := payload["x"].(float64); ok {
			x = int(v)
		}
		if v, ok := payload["y"].(float64); ok {
			y = int(v)
		}
		if v, ok := payload["buttons"].(float64); ok {
			buttons = int(v)
		}
		if v, ok := payload["wheelDelta"].(float64); ok {
			wheelDelta = int(v)
		}
		action, _ := payload["action"].(string)
		// buttons 位标志: 1=left, 2=right, 4=middle -> grdp: 0=left, 1=middle, 2=right
		button := 0
		switch {
		case buttons&1 != 0:
			button = 0
		case buttons&4 != 0:
			button = 1
		case buttons&2 != 0:
			button = 2
		}
		session.HandleMouse(x, y, button, wheelDelta, action)

	case protocol.DesktopMsgClipboardData:
		text, _ := payload["text"].(string)
		session.HandleClipboard(text)

	case protocol.DesktopMsgSetResolution:
		var w, hh int
		if v, ok := payload["width"].(float64); ok {
			w = int(v)
		}
		if v, ok := payload["height"].(float64); ok {
			hh = int(v)
		}
		session.HandleSetResolution(w, hh)
	}
}

// HandleDesktopClose 处理关闭桌面会话
func (h *Handler) HandleDesktopClose(msg *protocol.Message) {
	sessionID, _ := msg.Payload["sessionId"].(string)

	h.mu.Lock()
	session, exists := h.sessions[sessionID]
	if exists {
		delete(h.sessions, sessionID)
	}
	h.mu.Unlock()

	if exists {
		session.Close()
		slog.Info("Desktop session closed", "sessionID", sessionID)
	}
}

// sessionSendLoop 从 session 发送队列读取消息并转发
func (h *Handler) sessionSendLoop(session *Session) {
	for msg := range session.SendQueue() {
		if h.sendFunc == nil {
			continue
		}
		if err := h.sendFunc(msg); err != nil {
			slog.Error("Failed to send desktop message", "sessionID", session.ID, "error", err)
			break
		}
	}

	// session 队列关闭后，清理会话
	h.mu.Lock()
	delete(h.sessions, session.ID)
	h.mu.Unlock()
}

// sendError 发送错误消息
func (h *Handler) sendError(sessionID, errMsg string) {
	if h.sendFunc == nil {
		return
	}
	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopData,
		Payload: map[string]interface{}{
			"sessionId":   sessionID,
			"desktopType": string(protocol.DesktopMsgError),
			"error":       errMsg,
		},
	}
	h.sendFunc(msg)
}

// Stop 停止所有桌面会话
func (h *Handler) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for id, session := range h.sessions {
		session.Close()
		delete(h.sessions, id)
	}
	slog.Info("All desktop sessions stopped")
}
