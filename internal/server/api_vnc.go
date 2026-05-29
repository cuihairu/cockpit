package server

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// VNCSession VNC 桌面会话（二进制透传）
type VNCSession struct {
	ID         string
	AgentID    string
	Target     string
	ClientWS   *websocket.Conn
	ConnID     string
	CreatedAt  time.Time
	LastActive time.Time
	done       chan struct{}
}

var (
	vncSessions   = make(map[string]*VNCSession)
	vncSessionsMu sync.Mutex
)

var vncUpgrader = websocket.Upgrader{
	CheckOrigin: isOriginAllowed,
	ReadBufferSize:  1 * 1024 * 1024,
	WriteBufferSize: 1 * 1024 * 1024,
}

// handleVNCWebSocket 处理 VNC WebSocket 连接（二进制透传）
func (s *Server) handleVNCWebSocket(w http.ResponseWriter, r *http.Request) {
	// 从 Sec-WebSocket-Protocol 头获取票据
	protocols := r.Header["Sec-WebSocket-Protocol"]
	if len(protocols) == 0 {
		http.Error(w, "Missing ticket in Sec-WebSocket-Protocol header", http.StatusBadRequest)
		return
	}
	ticketID := protocols[0]

	// 验证票据
	ticket, valid := s.ticketMgr.ValidateTicket(ticketID)
	if !valid {
		http.Error(w, "Invalid or expired ticket", http.StatusUnauthorized)
		return
	}

	// 从票据参数获取连接信息
	agentID := ticket.Params["agent_id"]
	host := ticket.Params["host"]
	portStr := ticket.Params["port"]
	password := ticket.Params["password"]

	if agentID == "" || host == "" || portStr == "" {
		http.Error(w, "Invalid ticket parameters", http.StatusBadRequest)
		return
	}

	agent, exists := s.registry.Get(agentID)
	if !exists {
		http.Error(w, "Agent not found or offline", http.StatusNotFound)
		return
	}

	// 升级 WebSocket，接受票据协议
	upgrader := websocket.Upgrader{
		CheckOrigin:   isOriginAllowed,
		Subprotocols:  []string{ticketID},
		ReadBufferSize:  1 * 1024 * 1024,
		WriteBufferSize: 1 * 1024 * 1024,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("VNC WebSocket upgrade failed: %v", err)
		return
	}

	connID := uuid.New().String()[:8]
	sessionID := uuid.New().String()
	target := host + ":" + portStr

	session := &VNCSession{
		ID:         sessionID,
		AgentID:    agentID,
		Target:     target,
		ClientWS:   conn,
		ConnID:     connID,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
		done:       make(chan struct{}),
	}

	vncSessionsMu.Lock()
	vncSessions[sessionID] = session
	vncSessionsMu.Unlock()

	log.Printf("VNC session created: %s -> %s", sessionID, target)

	// 通知 Agent 建立 TCP 代理到 VNC 目标
	proxyMsg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"proxyId":  "vnc-" + connID,
		"proxyType": "tcp",
		"target":   target,
		"connId":   connID,
		"protocol": string(protocol.RemoteProtocolVNC),
		"password": password,
	})

	if err := agent.SendMessage(proxyMsg); err != nil {
		log.Printf("Failed to send VNC proxy new to agent: %v", err)
		conn.Close()
		vncSessionsMu.Lock()
		delete(vncSessions, sessionID)
		vncSessionsMu.Unlock()
		return
	}

	go s.vncSendLoop(session)
	s.vncKeepaliveLoop(session)
}

// vncSendLoop 浏览器 → Agent 二进制数据转发
func (s *Server) vncSendLoop(session *VNCSession) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("vncSendLoop panic: %v", r)
		}
		close(session.done)
	}()

	for {
		msgType, data, err := session.ClientWS.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("VNC WebSocket read error: %v", err)
			}
			s.sendVNCCloseToAgent(session)
			return
		}

		session.LastActive = time.Now()

		// 二进制帧直接转发，文本帧忽略
		if msgType != websocket.BinaryMessage {
			continue
		}

		agent, exists := s.registry.Get(session.AgentID)
		if !exists {
			return
		}

		proxyMsg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
			"proxyId": "vnc-" + session.ConnID,
			"connId":  session.ConnID,
			"data":    data,
		})
		agent.SendMessage(proxyMsg)
	}
}

// vncKeepaliveLoop 超时管理
func (s *Server) vncKeepaliveLoop(session *VNCSession) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-session.done:
			s.closeVNCSession(session)
			return
		case <-ticker.C:
			if time.Since(session.LastActive) > 30*time.Minute {
				log.Printf("VNC session timeout: %s", session.ID)
				s.closeVNCSession(session)
				return
			}
		}
	}
}

// sendVNCCloseToAgent 发送关闭消息给 Agent
func (s *Server) sendVNCCloseToAgent(session *VNCSession) {
	agent, exists := s.registry.Get(session.AgentID)
	if exists {
		msg := protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
			"proxyId": "vnc-" + session.ConnID,
			"connId":  session.ConnID,
			"reason":  "client disconnected",
		})
		agent.SendMessage(msg)
	}
}

// closeVNCSession 关闭 VNC 会话
func (s *Server) closeVNCSession(session *VNCSession) {
	session.ClientWS.Close()
	vncSessionsMu.Lock()
	delete(vncSessions, session.ID)
	vncSessionsMu.Unlock()
	log.Printf("VNC session closed: %s", session.ID)
}

// HandleVNCData 处理 Agent → 浏览器的 VNC 二进制数据
func (s *Server) HandleVNCData(connID string, data []byte) error {
	vncSessionsMu.Lock()
	var session *VNCSession
	for _, s := range vncSessions {
		if s.ConnID == connID {
			session = s
			break
		}
	}
	vncSessionsMu.Unlock()

	if session == nil {
		return nil
	}

	return session.ClientWS.WriteMessage(websocket.BinaryMessage, data)
}

// HandleVNCClose 处理 VNC 连接关闭
func (s *Server) HandleVNCClose(connID string, reason string) {
	vncSessionsMu.Lock()
	var session *VNCSession
	for id, s := range vncSessions {
		if s.ConnID == connID {
			session = s
			delete(vncSessions, id)
			break
		}
	}
	vncSessionsMu.Unlock()

	if session == nil {
		return
	}

	session.ClientWS.Close()
	log.Printf("VNC session closed by agent: %s, reason: %s", session.ID, reason)
}

// registerVNCAPI 注册 VNC API
func (s *Server) registerVNCAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/vnc", s.handleVNCWebSocket)
}
