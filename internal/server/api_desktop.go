package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// DesktopSession 桌面会话
type DesktopSession struct {
	ID         string
	AgentID    string
	Target     string
	ClientWS   *websocket.Conn
	ConnID     string
	Width      int
	Height     int
	CreatedAt  time.Time
	LastActive time.Time
	done       chan struct{}
}

var (
	desktopSessions   = make(map[string]*DesktopSession) // sessionID -> session
	desktopSessionsMu sync.Mutex
)

// desktopUpgrader 桌面连接专用 upgrader（更大的缓冲区）
var desktopUpgrader = websocket.Upgrader{
	CheckOrigin: isOriginAllowed,
	ReadBufferSize:  1 * 1024 * 1024, // 1MB
	WriteBufferSize: 1 * 1024 * 1024,
}

// handleDesktopWebSocket 处理桌面 WebSocket 连接
func (s *Server) handleDesktopWebSocket(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	agentID := query.Get("agent_id")
	host := query.Get("host")
	portStr := query.Get("port")
	token := query.Get("token")
	username := query.Get("password")
	password := query.Get("password")
	domain := query.Get("domain")
	widthStr := query.Get("width")
	heightStr := query.Get("height")

	// 修正：username 参数
	username = query.Get("username")

	if agentID == "" || host == "" || portStr == "" || token == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	// 验证 token
	_, err := auth.ValidateToken(token)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 检查 Agent 是否在线
	agent, exists := s.registry.Get(agentID)
	if !exists {
		http.Error(w, "Agent not found or offline", http.StatusNotFound)
		return
	}

	// 解析分辨率
	var width, height int
	if widthStr != "" {
		json.Unmarshal([]byte(widthStr), &width)
	}
	if heightStr != "" {
		json.Unmarshal([]byte(heightStr), &height)
	}
	if width <= 0 {
		width = 1280
	}
	if height <= 0 {
		height = 800
	}

	// 升级 WebSocket
	conn, err := desktopUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Desktop WebSocket upgrade failed: %v", err)
		return
	}

	connID := uuid.New().String()[:8]
	sessionID := uuid.New().String()

	target := host + ":" + portStr

	session := &DesktopSession{
		ID:         sessionID,
		AgentID:    agentID,
		Target:     target,
		ClientWS:   conn,
		ConnID:     connID,
		Width:      width,
		Height:     height,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
		done:       make(chan struct{}),
	}

	desktopSessionsMu.Lock()
	desktopSessions[sessionID] = session
	desktopSessionsMu.Unlock()

	log.Printf("Desktop session created: %s -> %s (%dx%d)", sessionID, target, width, height)

	// 发送 desktop_new 消息给 Agent
	msg := protocol.NewMessage(protocol.MessageTypeDesktopNew, map[string]interface{}{
		"sessionId": sessionID,
		"target":    target,
		"username":  username,
		"password":  password,
		"domain":    domain,
		"width":     width,
		"height":    height,
	})

	if err := agent.SendMessage(msg); err != nil {
		log.Printf("Failed to send desktop_new to agent: %v", err)
		conn.WriteJSON(map[string]interface{}{
			"type":  "error",
			"error": "Failed to establish connection to agent",
		})
		conn.Close()
		desktopSessionsMu.Lock()
		delete(desktopSessions, sessionID)
		desktopSessionsMu.Unlock()
		return
	}

	// 通知浏览器连接中
	conn.WriteJSON(map[string]interface{}{
		"type":    "connecting",
		"message": "Connecting to " + target,
	})

	// 启动浏览器→Agent 读取循环
	go s.desktopSendLoop(session)
	// 超时/保活管理（阻塞直到 done）
	s.desktopKeepaliveLoop(session)
}

// desktopSendLoop 浏览器 -> Agent 数据转发
func (s *Server) desktopSendLoop(session *DesktopSession) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("desktopSendLoop panic: %v", r)
		}
		close(session.done) // 通知 keepaliveLoop 退出
	}()

	for {
		_, data, err := session.ClientWS.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Desktop WebSocket read error: %v", err)
			}
			s.sendDesktopCloseToAgent(session)
			return
		}

		session.LastActive = time.Now()

		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		msgType, _ := msg["type"].(string)

		payload := map[string]interface{}{
			"sessionId": session.ID,
		}

		switch msgType {
		case "keyboard":
			payload["desktopType"] = string(protocol.DesktopMsgKeyboard)
			payload["scanCode"] = msg["scanCode"]
			payload["keyDown"] = msg["keyDown"]
			payload["extended"] = msg["extended"]

		case "mouse":
			payload["desktopType"] = string(protocol.DesktopMsgMouse)
			payload["x"] = msg["x"]
			payload["y"] = msg["y"]
			payload["buttons"] = msg["buttons"]
			payload["wheelDelta"] = msg["wheelDelta"]
			payload["action"] = msg["action"]

		case "clipboard":
			payload["desktopType"] = string(protocol.DesktopMsgClipboardData)
			payload["text"] = msg["text"]

		case "set_resolution":
			payload["desktopType"] = string(protocol.DesktopMsgSetResolution)
			payload["width"] = msg["width"]
			payload["height"] = msg["height"]

		default:
			continue
		}

		agent, exists := s.registry.Get(session.AgentID)
		if exists {
			proxyMsg := protocol.NewMessage(protocol.MessageTypeDesktopData, payload)
			agent.SendMessage(proxyMsg)
		}
	}
}

// desktopKeepaliveLoop 超时管理与保活
func (s *Server) desktopKeepaliveLoop(session *DesktopSession) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-session.done:
			// browser 断开，sendLoop 已退出
			return

		case <-ticker.C:
			if err := session.ClientWS.WriteJSON(map[string]interface{}{
				"type": "ping",
			}); err != nil {
				s.closeDesktopSession(session)
				return
			}

			if time.Since(session.LastActive) > 30*time.Minute {
				log.Printf("Desktop session timeout: %s", session.ID)
				s.closeDesktopSession(session)
				return
			}
		}
	}
}

// sendDesktopCloseToAgent 发送关闭消息给 Agent
func (s *Server) sendDesktopCloseToAgent(session *DesktopSession) {
	agent, exists := s.registry.Get(session.AgentID)
	if exists {
		msg := protocol.NewMessage(protocol.MessageTypeDesktopClose, map[string]interface{}{
			"sessionId": session.ID,
			"reason":    "client disconnected",
		})
		agent.SendMessage(msg)
	}
}

// closeDesktopSession 关闭桌面会话
func (s *Server) closeDesktopSession(session *DesktopSession) {
	session.ClientWS.Close()
	desktopSessionsMu.Lock()
	delete(desktopSessions, session.ID)
	desktopSessionsMu.Unlock()
	log.Printf("Desktop session closed: %s", session.ID)
}

// HandleDesktopData 处理 Agent -> 浏览器的桌面数据
func (s *Server) HandleDesktopData(msg *protocol.Message) {
	sessionID, _ := msg.Payload["sessionId"].(string)
	if sessionID == "" {
		return
	}

	desktopSessionsMu.Lock()
	session, exists := desktopSessions[sessionID]
	desktopSessionsMu.Unlock()

	if !exists {
		return
	}

	// 将 desktop_data 负载转发给浏览器，desktopType 映射为前端 type
	desktopType, _ := msg.Payload["desktopType"].(string)
	agentMsg := map[string]interface{}{
		"type": desktopType,
	}

	for k, v := range msg.Payload {
		agentMsg[k] = v
	}

	if err := session.ClientWS.WriteJSON(agentMsg); err != nil {
		log.Printf("Failed to write desktop data to browser: %v", err)
		s.closeDesktopSession(session)
	}
}

// HandleDesktopClose 处理 Agent 关闭桌面会话
func (s *Server) HandleDesktopClose(msg *protocol.Message) {
	sessionID, _ := msg.Payload["sessionId"].(string)
	reason, _ := msg.Payload["reason"].(string)
	if reason == "" {
		reason = "agent closed"
	}

	desktopSessionsMu.Lock()
	session, exists := desktopSessions[sessionID]
	if exists {
		delete(desktopSessions, sessionID)
	}
	desktopSessionsMu.Unlock()

	if !exists {
		return
	}

	session.ClientWS.WriteJSON(map[string]interface{}{
		"type":   "disconnected",
		"reason": reason,
	})
	session.ClientWS.Close()
	log.Printf("Desktop session closed by agent: %s, reason: %s", sessionID, reason)
}

// registerDesktopAPI 注册桌面 API
func (s *Server) registerDesktopAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/desktop", s.handleDesktopWebSocket)
}
