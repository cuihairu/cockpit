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

// TerminalSession 终端会话
type TerminalSession struct {
	ID        string
	AgentID   string
	Protocol  protocol.RemoteProtocol
	Host      string
	Port      int
	ClientWS  *websocket.Conn
	ConnID    string
	CreatedAt time.Time
	done      chan struct{}
}

var (
	terminalSessions   = make(map[string]*TerminalSession) // sessionID -> session
	terminalByConn     = make(map[string]*TerminalSession) // connID -> session
	terminalSessionsMu sync.Mutex
)

// handleTerminalWebSocket 处理终端 WebSocket 连接
func (s *Server) handleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	agentID := query.Get("agent_id")
	host := query.Get("host")
	portStr := query.Get("port")
	protocolStr := query.Get("protocol")
	token := query.Get("token")

	if agentID == "" || host == "" || portStr == "" || protocolStr == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	username, err := auth.ValidateToken(token)
	if err != nil {
		log.Printf("Token validation failed: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var port int
	if err := json.Unmarshal([]byte(portStr), &port); err != nil {
		http.Error(w, "Invalid port", http.StatusBadRequest)
		return
	}

	remoteProtocol := protocol.RemoteProtocol(protocolStr)
	if remoteProtocol != protocol.RemoteProtocolSSH &&
		remoteProtocol != protocol.RemoteProtocolTelnet &&
		remoteProtocol != protocol.RemoteProtocolVNC {
		http.Error(w, "Unsupported protocol", http.StatusBadRequest)
		return
	}

	agent, exists := s.registry.Get(agentID)
	if !exists {
		http.Error(w, "Agent not found or offline", http.StatusNotFound)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	connID := uuid.New().String()[:8]
	sessionID := uuid.New().String()

	session := &TerminalSession{
		ID:        sessionID,
		AgentID:   agentID,
		Protocol:  remoteProtocol,
		Host:      host,
		Port:      port,
		ClientWS:  conn,
		ConnID:    connID,
		CreatedAt: time.Now(),
		done:      make(chan struct{}),
	}

	terminalSessionsMu.Lock()
	terminalSessions[sessionID] = session
	terminalByConn[connID] = session
	terminalSessionsMu.Unlock()

	log.Printf("Terminal session created: %s for user %s", sessionID, username)

	target := host + ":" + portStr
	msg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"proxyId":   "terminal-" + connID,
		"proxyType": "tcp",
		"target":    target,
		"terminal":  true,
		"connId":    connID,
		"protocol":  string(remoteProtocol),
	})

	if err := agent.SendMessage(msg); err != nil {
		log.Printf("Failed to send proxy start message: %v", err)
		conn.WriteJSON(map[string]interface{}{
			"type":    "error",
			"message": "Failed to establish connection to agent",
		})
		conn.Close()
		terminalSessionsMu.Lock()
		delete(terminalSessions, sessionID)
		delete(terminalByConn, connID)
		terminalSessionsMu.Unlock()
		return
	}

	conn.WriteJSON(map[string]interface{}{
		"type":    "connect",
		"message": "Connecting to " + target,
	})

	go s.terminalSendLoop(session)
	s.terminalKeepaliveLoop(session)
}

// terminalSendLoop 浏览器 → Agent 数据转发
func (s *Server) terminalSendLoop(session *TerminalSession) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("terminalSendLoop panic: %v", r)
		}
		close(session.done)
	}()

	conn := session.ClientWS
	connID := session.ConnID

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			s.sendCloseToAgent(session)
			return
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err == nil {
			msgType, _ := msg["type"].(string)

			switch msgType {
			case "input":
				inputData, _ := msg["data"].(string)
				agent, exists := s.registry.Get(session.AgentID)
				if exists {
					payload := map[string]interface{}{
						"proxyId":  "terminal-" + connID,
						"connId":   connID,
						"data":     []byte(inputData),
						"terminal": true,
					}
					agent.SendMessage(protocol.NewMessage(protocol.MessageTypeProxyData, payload))
				}

			case "resize":
				rows, _ := msg["rows"].(float64)
				cols, _ := msg["cols"].(float64)
				agent, exists := s.registry.Get(session.AgentID)
				if exists {
					payload := map[string]interface{}{
						"proxyId":  "terminal-" + connID,
						"connId":   connID,
						"resize":   true,
						"rows":     int(rows),
						"cols":     int(cols),
						"terminal": true,
					}
					agent.SendMessage(protocol.NewMessage(protocol.MessageTypeProxyData, payload))
				}
			}
		} else {
			agent, exists := s.registry.Get(session.AgentID)
			if exists {
				payload := map[string]interface{}{
					"proxyId":  "terminal-" + connID,
					"connId":   connID,
					"data":     string(data),
					"terminal": true,
				}
				agent.SendMessage(protocol.NewMessage(protocol.MessageTypeProxyData, payload))
			}
		}
	}
}

// terminalKeepaliveLoop 超时管理
func (s *Server) terminalKeepaliveLoop(session *TerminalSession) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	lastActive := session.CreatedAt

	for {
		select {
		case <-session.done:
			s.closeTerminalSession(session)
			return
		case <-ticker.C:
			if err := session.ClientWS.WriteJSON(map[string]interface{}{
				"type": "ping",
			}); err != nil {
				s.closeTerminalSession(session)
				return
			}

			terminalSessionsMu.Lock()
			if s, ok := terminalSessions[session.ID]; ok {
				lastActive = s.CreatedAt // 用创建时间作为 fallback
			}
			terminalSessionsMu.Unlock()

			if time.Since(lastActive) > 30*time.Minute {
				log.Printf("Terminal session timeout: %s", session.ID)
				s.closeTerminalSession(session)
				return
			}
		}
	}
}

// sendCloseToAgent 发送关闭消息给 Agent
func (s *Server) sendCloseToAgent(session *TerminalSession) {
	agent, exists := s.registry.Get(session.AgentID)
	if exists {
		payload := map[string]interface{}{
			"proxyId":  "terminal-" + session.ConnID,
			"connId":   session.ConnID,
			"reason":   "client disconnected",
			"terminal": true,
		}
		agent.SendMessage(protocol.NewMessage(protocol.MessageTypeProxyClose, payload))
	}
}

// closeTerminalSession 关闭终端会话
func (s *Server) closeTerminalSession(session *TerminalSession) {
	session.ClientWS.Close()
	terminalSessionsMu.Lock()
	delete(terminalSessions, session.ID)
	delete(terminalByConn, session.ConnID)
	terminalSessionsMu.Unlock()
	log.Printf("Terminal session closed: %s", session.ID)
}

// HandleTerminalData 处理从 Agent 转发到浏览器终端的数据
func (s *Server) HandleTerminalData(connID string, data []byte) error {
	terminalSessionsMu.Lock()
	session, exists := terminalByConn[connID]
	terminalSessionsMu.Unlock()

	if !exists {
		return nil
	}

	return session.ClientWS.WriteJSON(map[string]interface{}{
		"type": "data",
		"data": string(data),
	})
}

// HandleTerminalClose 处理终端连接关闭
func (s *Server) HandleTerminalClose(connID string, reason string) {
	terminalSessionsMu.Lock()
	session, exists := terminalByConn[connID]
	if exists {
		delete(terminalSessions, session.ID)
		delete(terminalByConn, connID)
	}
	terminalSessionsMu.Unlock()

	if !exists {
		return
	}

	session.ClientWS.WriteJSON(map[string]interface{}{
		"type":    "close",
		"message": reason,
	})
	session.ClientWS.Close()
	log.Printf("Terminal connection closed: %s, reason: %s", connID, reason)
}

// registerRemoteAPI 注册远程连接 API
func (s *Server) registerRemoteAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/terminal", s.handleTerminalWebSocket)
}
