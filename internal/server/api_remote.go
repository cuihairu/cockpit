package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// TerminalSession 终端会话
type TerminalSession struct {
	ID         string
	AgentID    string
	Protocol   protocol.RemoteProtocol
	Host       string
	Port       int
	ClientWS   *websocket.Conn
	ConnID     string
	CreatedAt  time.Time
	LastActive time.Time
}

// terminalSessions 活跃的终端会话
var (
	terminalSessions = make(map[string]*TerminalSession)
	terminalSessionsMu = make(map[string]*websocket.Conn) // connID -> client ws for proxy data forwarding
)

// handleTerminalWebSocket 处理终端 WebSocket 连接
func (s *Server) handleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	// 从查询参数获取连接信息
	query := r.URL.Query()
	agentID := query.Get("agent_id")
	host := query.Get("host")
	portStr := query.Get("port")
	protocolStr := query.Get("protocol")
	token := query.Get("token")

	// 验证参数
	if agentID == "" || host == "" || portStr == "" || protocolStr == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	// 验证 token
	username, err := auth.ValidateToken(token)
	if err != nil {
		log.Printf("Token validation failed: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 验证端口
	var port int
	if err := json.Unmarshal([]byte(portStr), &port); err != nil {
		http.Error(w, "Invalid port", http.StatusBadRequest)
		return
	}

	// 验证协议
	remoteProtocol := protocol.RemoteProtocol(protocolStr)
	if remoteProtocol != protocol.RemoteProtocolSSH &&
		remoteProtocol != protocol.RemoteProtocolTelnet &&
		remoteProtocol != protocol.RemoteProtocolVNC {
		http.Error(w, "Unsupported protocol", http.StatusBadRequest)
		return
	}

	// 检查 Agent 是否在线
	agent, exists := s.registry.Get(agentID)
	if !exists {
		http.Error(w, "Agent not found or offline", http.StatusNotFound)
		return
	}

	// 升级到 WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// 生成连接 ID
	connID := uuid.New().String()[:8]
	sessionID := uuid.New().String()

	// 创建会话
	session := &TerminalSession{
		ID:         sessionID,
		AgentID:    agentID,
		Protocol:   remoteProtocol,
		Host:       host,
		Port:       port,
		ClientWS:   conn,
		ConnID:     connID,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}
	terminalSessions[sessionID] = session
	terminalSessionsMu[connID] = conn

	log.Printf("Terminal session created: %s for user %s", sessionID, username)

	// 发送启动远程代理消息给 Agent
	target := host + ":" + portStr
	msg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"proxyId": "terminal-" + connID,
		"proxyType": "tcp",
		"target": target,
		"terminal": true,
		"connId": connID,
		"protocol": string(remoteProtocol),
	})

	if err := agent.SendMessage(msg); err != nil {
		log.Printf("Failed to send proxy start message: %v", err)
		conn.WriteJSON(map[string]interface{}{
			"type":    "error",
			"message": "Failed to establish connection to agent",
		})
		conn.Close()
		delete(terminalSessions, sessionID)
		delete(terminalSessionsMu, connID)
		return
	}

	// 发送成功消息
	conn.WriteJSON(map[string]interface{}{
		"type":    "connect",
		"message": "Connecting to " + target,
	})

	// 启动消息处理循环
	go s.terminalSendLoop(session)
	s.terminalReceiveLoop(session)
}

// terminalSendLoop 处理从浏览器到 Agent 的数据转发
func (s *Server) terminalSendLoop(session *TerminalSession) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("terminalSendLoop panic: %v", r)
		}
	}()

	conn := session.ClientWS
	connID := session.ConnID

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			// 发送关闭消息给 Agent
			s.sendCloseToAgent(session)
			break
		}

		session.LastActive = time.Now()

		// 解析消息（可能是 JSON 或原始文本）
		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err == nil {
			// JSON 格式消息
			msgType, _ := msg["type"].(string)

			switch msgType {
			case "input":
				// 终端输入数据
				inputData, _ := msg["data"].(string)
				agent, exists := s.registry.Get(session.AgentID)
				if exists {
					payload := map[string]interface{}{
						"proxyId": "terminal-" + connID,
						"connId":  connID,
						"data":    []byte(inputData),
						"terminal": true,
					}
					proxyMsg := protocol.NewMessage(protocol.MessageTypeProxyData, payload)
					agent.SendMessage(proxyMsg)
				}

			case "resize":
				// 终端窗口大小调整
				rows, _ := msg["rows"].(float64)
				cols, _ := msg["cols"].(float64)
				agent, exists := s.registry.Get(session.AgentID)
				if exists {
					payload := map[string]interface{}{
						"proxyId": "terminal-" + connID,
						"connId":  connID,
						"resize": true,
						"rows": int(rows),
						"cols": int(cols),
					}
					proxyMsg := protocol.NewMessage(protocol.MessageTypeProxyData, payload)
					agent.SendMessage(proxyMsg)
				}
			}
		} else {
			// 原始数据直接转发
			agent, exists := s.registry.Get(session.AgentID)
			if exists {
				payload := map[string]interface{}{
					"proxyId": "terminal-" + connID,
					"connId":  connID,
					"data":    string(data),
					"terminal": true,
				}
				proxyMsg := protocol.NewMessage(protocol.MessageTypeProxyData, payload)
				agent.SendMessage(proxyMsg)
			}
		}
	}
}

// terminalReceiveLoop 处理超时和清理
func (s *Server) terminalReceiveLoop(session *TerminalSession) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 发送 ping
			if err := session.ClientWS.WriteJSON(map[string]interface{}{
				"type": "ping",
			}); err != nil {
				// 连接已断开
				s.closeTerminalSession(session)
				return
			}

			// 检查空闲超时（30 分钟）
			if time.Since(session.LastActive) > 30*time.Minute {
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
			"proxyId": "terminal-" + session.ConnID,
			"connId":  session.ConnID,
			"reason":  "client disconnected",
			"terminal": true,
		}
		msg := protocol.NewMessage(protocol.MessageTypeProxyClose, payload)
		agent.SendMessage(msg)
	}
}

// closeTerminalSession 关闭终端会话
func (s *Server) closeTerminalSession(session *TerminalSession) {
	session.ClientWS.Close()
	delete(terminalSessions, session.ID)
	delete(terminalSessionsMu, session.ConnID)
	log.Printf("Terminal session closed: %s", session.ID)
}

// HandleTerminalData 处理从 Agent 转发到浏览器终端的数据
// 这个方法由 proxy.Manager 调用
func (s *Server) HandleTerminalData(connID string, data []byte) error {
	conn, exists := terminalSessionsMu[connID]
	if !exists {
		return nil // 会话可能已关闭
	}

	msg := map[string]interface{}{
		"type": "data",
		"data": string(data),
	}

	return conn.WriteJSON(msg)
}

// HandleTerminalClose 处理终端连接关闭
func (s *Server) HandleTerminalClose(connID string, reason string) {
	conn, exists := terminalSessionsMu[connID]
	if !exists {
		return
	}

	msg := map[string]interface{}{
		"type":    "close",
		"message": reason,
	}
	conn.WriteJSON(msg)
	conn.Close()

	// 清理会话
	for sessionID, session := range terminalSessions {
		if session.ConnID == connID {
			delete(terminalSessions, sessionID)
			break
		}
	}
	delete(terminalSessionsMu, connID)
	log.Printf("Terminal connection closed: %s, reason: %s", connID, reason)
}

// registerRemoteAPI 注册远程连接 API
func (s *Server) registerRemoteAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/terminal", s.handleTerminalWebSocket)
}
