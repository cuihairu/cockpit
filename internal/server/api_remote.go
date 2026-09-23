package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// TerminalSession 终端会话
type TerminalSession struct {
	ID         string
	UserID     string
	Username   string
	AgentID    string
	Protocol   protocol.RemoteProtocol
	Host       string
	Port       int
	ClientWS   *websocket.Conn
	ConnID     string
	CreatedAt  time.Time
	LastActive time.Time
	recorder   *castRecorder // 输出流录制（recording-design.md；nil = 未开）
	mu         sync.Mutex
	done       chan struct{}
}

var (
	terminalSessions   = make(map[string]*TerminalSession) // sessionID -> session
	terminalByConn     = make(map[string]*TerminalSession) // connID -> session
	terminalSessionsMu sync.Mutex
)

// handleTerminalWebSocket 处理终端 WebSocket 连接
func (s *Server) handleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	// 从 Sec-WebSocket-Protocol 头获取票据
	protocols := r.Header.Values("Sec-WebSocket-Protocol")
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
	protocolStr := ticket.Params["protocol"]

	if agentID == "" || host == "" || portStr == "" || protocolStr == "" {
		http.Error(w, "Invalid ticket parameters", http.StatusBadRequest)
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

	// 升级 WebSocket，接受票据协议
	upgrader := websocket.Upgrader{
		CheckOrigin:  isOriginAllowed,
		Subprotocols: []string{ticketID},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	connID := uuid.New().String()[:8]
	sessionID := uuid.New().String()

	session := &TerminalSession{
		ID:         sessionID,
		UserID:     ticket.UserID,
		Username:   ticket.Username,
		AgentID:    agentID,
		Protocol:   remoteProtocol,
		Host:       host,
		Port:       port,
		ClientWS:   conn,
		ConnID:     connID,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
		done:       make(chan struct{}),
	}

	terminalSessionsMu.Lock()
	terminalSessions[sessionID] = session
	terminalByConn[connID] = session
	terminalSessionsMu.Unlock()

	// 输出流录制（开关默认开；失败仅记日志，不影响远控主链路）
	if s.recordingEnabled() {
		session.recorder = s.startRecording(session)
	}

	log.Printf("Terminal session created: %s for user %s", sessionID, ticket.Username)

	target := host + ":" + portStr
	proxyNew := map[string]interface{}{
		"proxyId":   "terminal-" + connID,
		"proxyType": "tcp",
		"target":    target,
		"terminal":  true,
		"connId":    connID,
		"protocol":  string(remoteProtocol),
	}
	// SSH 凭据随 proxy_new 下发到 agent（agent 侧终结 SSH 协议）。
	// 口令只经加密 WS 通道传输，不落盘、不进日志/审计。
	if remoteProtocol == protocol.RemoteProtocolSSH {
		if v := ticket.Params["username"]; v != "" {
			proxyNew["username"] = v
		}
		if v := ticket.Params["password"]; v != "" {
			proxyNew["password"] = v
		}
		if v := ticket.Params["private_key"]; v != "" {
			proxyNew["privateKey"] = v
		}
	}
	msg := protocol.NewMessage(protocol.MessageTypeProxyNew, proxyNew)

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

		// 更新最后活跃时间
		session.mu.Lock()
		session.LastActive = time.Now()
		session.mu.Unlock()

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

// terminalKeepalive 间隔。包级变量仅为测试可注入，默认值即生产取值。
var terminalKeepaliveInterval = 30 * time.Second

// terminalKeepaliveLoop 超时管理
func (s *Server) terminalKeepaliveLoop(session *TerminalSession) {
	ticker := time.NewTicker(terminalKeepaliveInterval)
	defer ticker.Stop()

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

			// 获取实际的活动时间
			session.mu.Lock()
			lastActive := session.LastActive
			session.mu.Unlock()

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
	session.recorder.Close() // 幂等，与 HandleTerminalClose 双出口都可调
	session.ClientWS.Close()
	terminalSessionsMu.Lock()
	delete(terminalSessions, session.ID)
	delete(terminalByConn, session.ConnID)
	terminalSessionsMu.Unlock()
	log.Printf("Terminal session closed: %s", session.ID)

	s.auditRemoteEnd(session.UserID, session.Username, "", "",
		&audit.RemoteSessionDetails{
			Protocol: string(session.Protocol),
			AgentID:  session.AgentID,
			Host:     session.Host,
			Port:     session.Port,
			Session:  session.ID,
			Duration: time.Since(session.CreatedAt).String(),
		})
}

// HandleTerminalData 处理从 Agent 转发到浏览器终端的数据
func (s *Server) HandleTerminalData(connID string, data []byte) error {
	terminalSessionsMu.Lock()
	session, exists := terminalByConn[connID]
	terminalSessionsMu.Unlock()

	if !exists {
		return nil
	}

	// 录制输出流（agent → 浏览器方向的唯一出口）
	session.recorder.WriteOutput(data)

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

	session.recorder.Close() // agent 侧关闭路径也即时回填（不等 keepalive 兜底）
	session.ClientWS.WriteJSON(map[string]interface{}{
		"type":    "close",
		"message": reason,
	})
	session.ClientWS.Close()
	log.Printf("Terminal connection closed: %s, reason: %s", connID, reason)
}

// registerRemoteAPI 注册远程连接 API
func (s *Server) registerRemoteAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/tickets", s.authService().Middleware(s.handleTicketCreate))
	mux.HandleFunc("/api/remote/terminal", s.handleTerminalWebSocket)
	mux.HandleFunc("/api/remote/sessions", s.authService().Middleware(s.handleRemoteSessions))
	mux.HandleFunc("/api/remote/sessions/", s.authService().Middleware(s.handleRemoteSession))
}

// handleTicketCreate 创建短期 WebSocket 连接票据
func (s *Server) handleTicketCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		AgentID    string `json:"agent_id"`
		Host       string `json:"host"`
		Port       int    `json:"port"`
		Protocol   string `json:"protocol"` // ssh, telnet, vnc, rdp
		Username   string `json:"username,omitempty"`
		Password   string `json:"password,omitempty"`    // SSH 口令 / VNC 密码等
		PrivateKey string `json:"private_key,omitempty"` // SSH PEM 私钥（优先于 Password）
		Domain     string `json:"domain,omitempty"`
		Width      int    `json:"width,omitempty"`
		Height     int    `json:"height,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	userInfo, _ := auth.GetUserFromContext(r)
	clientIP := s.getClientIP(r)
	userAgent := r.UserAgent()

	if req.AgentID == "" || req.Host == "" || req.Port <= 0 || req.Protocol == "" {
		s.auditRemoteFailure(
			userInfo.UserID,
			userInfo.Username,
			clientIP,
			userAgent,
			&audit.RemoteSessionDetails{
				Protocol: req.Protocol,
				AgentID:  req.AgentID,
				Host:     req.Host,
				Port:     req.Port,
				Reason:   "missing required fields",
			},
		)
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// 检查 Agent 是否在线
	if _, exists := s.registry.Get(req.AgentID); !exists {
		s.auditRemoteFailure(
			userInfo.UserID,
			userInfo.Username,
			clientIP,
			userAgent,
			&audit.RemoteSessionDetails{
				Protocol: req.Protocol,
				AgentID:  req.AgentID,
				Host:     req.Host,
				Port:     req.Port,
				Reason:   "agent not found or offline",
			},
		)
		http.Error(w, "Agent not found or offline", http.StatusNotFound)
		return
	}

	// 校验目标是否在 allow-list 和 Agent 出口策略内（除非显式允许任意目标）
	egressMatch, reason := s.matchRemoteEgress(req.AgentID, req.Host, req.Port)
	if egressMatch == nil {
		s.auditRemoteFailure(
			userInfo.UserID,
			userInfo.Username,
			clientIP,
			userAgent,
			&audit.RemoteSessionDetails{
				Protocol: req.Protocol,
				AgentID:  req.AgentID,
				Host:     req.Host,
				Port:     req.Port,
				Reason:   reason,
			},
		)
		rejectTargetDenied(w, reason)
		return
	}

	if userInfo.UserID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 构建票据参数
	params := map[string]string{
		"agent_id": req.AgentID,
		"host":     req.Host,
		"port":     strconv.Itoa(req.Port),
		"protocol": req.Protocol,
	}
	if req.Username != "" {
		params["username"] = req.Username
	}
	if req.Password != "" {
		params["password"] = req.Password
	}
	if req.PrivateKey != "" {
		params["private_key"] = req.PrivateKey
	}
	if req.Domain != "" {
		params["domain"] = req.Domain
	}
	if req.Width > 0 {
		params["width"] = strconv.Itoa(req.Width)
	}
	if req.Height > 0 {
		params["height"] = strconv.Itoa(req.Height)
	}

	// 生成票据
	ticket, err := s.ticketMgr.GenerateTicket(userInfo.UserID, userInfo.Username, params)
	if err != nil {
		s.auditRemoteFailure(
			userInfo.UserID,
			userInfo.Username,
			clientIP,
			userAgent,
			&audit.RemoteSessionDetails{
				Protocol: req.Protocol,
				AgentID:  req.AgentID,
				Host:     req.Host,
				Port:     req.Port,
				Egress:   egressMatch.summary(),
				Reason:   "failed to generate ticket",
			},
		)
		http.Error(w, "Failed to generate ticket", http.StatusInternalServerError)
		return
	}

	// 远控会话开始审计（不写 password）
	s.auditRemoteStart(
		audit.ActionRemoteStart,
		userInfo.UserID,
		userInfo.Username,
		clientIP,
		userAgent,
		&audit.RemoteSessionDetails{
			Protocol: req.Protocol,
			AgentID:  req.AgentID,
			Host:     req.Host,
			Port:     req.Port,
			Session:  ticket.ID,
			Egress:   egressMatch.summary(),
		},
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"ticket":     ticket.ID,
		"expires_at": ticket.ExpiresAt.Format(time.RFC3339),
	})
}

func (s *Server) handleRemoteSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": s.remoteSessions.List(),
		})
	case http.MethodPost:
		s.handleRemoteSessionCreate(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRemoteSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/remote/sessions/")
	id = strings.TrimSpace(id)
	if id == "" {
		http.Error(w, "Missing session id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		session, ok := s.remoteSessions.Get(id)
		if !ok {
			http.Error(w, "Remote session not found", http.StatusNotFound)
			return
		}
		s.writeJSON(w, http.StatusOK, session)
	case http.MethodDelete:
		if ok := s.remoteSessions.Delete(id); !ok {
			http.Error(w, "Remote session not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRemoteSessionCreate(w http.ResponseWriter, r *http.Request) {
	var req RemoteSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.AgentID == "" || req.Host == "" || req.Port <= 0 {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	if req.Protocol != protocol.RemoteProtocolSSH &&
		req.Protocol != protocol.RemoteProtocolTelnet &&
		req.Protocol != protocol.RemoteProtocolRDP &&
		req.Protocol != protocol.RemoteProtocolVNC {
		http.Error(w, "Unsupported protocol", http.StatusBadRequest)
		return
	}

	if _, exists := s.registry.Get(req.AgentID); !exists {
		http.Error(w, "Agent not found or offline", http.StatusNotFound)
		return
	}

	userInfo, _ := auth.GetUserFromContext(r)
	session := s.remoteSessions.Create(userInfo.UserID, userInfo.Username, req)
	s.writeJSON(w, http.StatusCreated, session)
}
