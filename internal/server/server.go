package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/gorilla/websocket"
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Server WebSocket 服务器
type Server struct {
	addr     string
	registry *Registry
	codec    *protocol.Codec
	db       *storage.DB
	upgrader websocket.Upgrader

	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// Config 服务器配置
type Config struct {
	Addr    string // 监听地址，如 "0.0.0.0:8080"
	DataDir string // 数据目录
}

// NewServer 创建新服务器
func NewServer(cfg Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	// 打开数据库
	dbPath := filepath.Join(cfg.DataDir, "cockpit.db")
	db, err := storage.Open(storage.Config{Path: dbPath})
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	return &Server{
		addr:     cfg.Addr,
		registry: NewRegistry(),
		codec:    protocol.NewCodec(),
		db:       db,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// 生产环境应该验证 Origin
				return true
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	// 初始化认证（设置数据库）
	auth.InitDB(s.db)

	// 初始化管理员用户
	adminUser := getEnv("ADMIN_USERNAME", "admin")
	adminPass := getEnv("ADMIN_PASSWORD", "admin")
	if err := auth.InitAdmin(s.db, adminUser, adminPass); err != nil {
		log.Printf("Warning: Failed to init admin user: %v", err)
	} else {
		log.Printf("Admin user initialized: %s", adminUser)
	}

	mux := http.NewServeMux()

	// 公开路由
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/auth/login", auth.HandleLogin)
	mux.HandleFunc("/api/auth/refresh", auth.HandleRefresh)

	// 需要认证的 API 路由
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		// 登录相关接口不需要认证
		if strings.HasPrefix(r.URL.Path, "/api/auth/") {
			if r.URL.Path == "/api/auth/login" {
				auth.HandleLogin(w, r)
			} else if r.URL.Path == "/api/auth/refresh" {
				auth.HandleRefresh(w, r)
			}
			return
		}
		// 其他 API 需要认证
		auth.Middleware(s.serveAPI)(w, r)
	})

	// Web UI (SPA) - 必须放在最后作为 fallback
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.spaHandler().ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	log.Printf("Server starting on %s", s.addr)
	log.Printf("Web UI: http://%s", s.addr)

	// 启动清理协程
	go s.cleanupLoop()

	// 启动警告检查协程
	go s.alertCheckLoop()

	return server.ListenAndServe()
}

// Shutdown 关闭服务器
func (s *Server) Shutdown() {
	s.cancel()
	if s.db != nil {
		s.db.Close()
	}
}

// handleWebSocket 处理 WebSocket 连接
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// 等待注册消息
	msg, err := s.codec.ReadMessage(conn)
	if err != nil {
		log.Printf("Read register message failed: %v", err)
		conn.Close()
		return
	}

	if msg.Type != protocol.MessageTypeRegister {
		log.Printf("First message must be register, got: %s", msg.Type)
		conn.Close()
		return
	}

	// 解析注册信息
	var reg protocol.RegisterPayload
	payloadBytes, _ := json.Marshal(msg.Payload)
	if err := json.Unmarshal(payloadBytes, &reg); err != nil {
		log.Printf("Parse register payload failed: %v", err)
		conn.Close()
		return
	}

	agentID := reg.AgentID
	if agentID == "" {
		agentID = protocol.GenerateIDWithPrefix("agent")
	}

	// 创建 Agent
	agent := NewAgent(agentID, conn)
	agent.Update(&reg)

	// 注册到 Registry
	if err := s.registry.Register(agent); err != nil {
		log.Printf("Register agent failed: %v", err)
		// 如果已存在，先注销旧的
		s.registry.Unregister(agentID)
		s.registry.Register(agent)
	}

	// 持久化到数据库
	if err := s.db.UpsertAgent(toStorageAgent(agent)); err != nil {
		log.Printf("Failed to persist agent to database: %v", err)
	}

	log.Printf("Agent registered: %s at %s/%s", agentID, reg.Location.Region, reg.Location.Zone)

	// 发送注册响应
	resp := protocol.NewMessage(protocol.MessageTypeRegister, map[string]interface{}{
		"status":            "accepted",
		"serverTime":        time.Now().Unix(),
		"heartbeatInterval": int(30),
	})
	s.codec.WriteMessage(conn, resp)

	// 启动读写循环
	go s.readLoop(agent)
	go s.writeLoop(agent)
}

// readLoop 读取循环
func (s *Server) readLoop(agent *Agent) {
	defer s.registry.Unregister(agent.ID)

	for {
		msg, err := s.codec.ReadMessage(agent.Conn)
		if err != nil {
			log.Printf("Agent %s read error: %v", agent.ID, err)
			return
		}

		s.handleMessage(agent, msg)
	}
}

// writeLoop 写入循环
func (s *Server) writeLoop(agent *Agent) {
	defer agent.Conn.Close()

	for msg := range agent.Send {
		if err := s.codec.WriteMessage(agent.Conn, msg); err != nil {
			log.Printf("Agent %s write error: %v", agent.ID, err)
			return
		}
	}
}

// handleMessage 处理消息
func (s *Server) handleMessage(agent *Agent, msg *protocol.Message) {
	switch msg.Type {
	case protocol.MessageTypeHeartbeat:
		s.handleHeartbeat(agent, msg)
	case protocol.MessageTypeRPCResponse:
		s.handleRPCResponse(msg)
	default:
		log.Printf("Unknown message type: %s from agent %s", msg.Type, agent.ID)
	}
}

// handleHeartbeat 处理心跳
func (s *Server) handleHeartbeat(agent *Agent, msg *protocol.Message) {
	agent.Heartbeat()

	// 发送 ACK
	resp := protocol.NewMessage(protocol.MessageTypeHeartbeat, map[string]interface{}{
		"status":     "ack",
		"serverTime": time.Now().Unix(),
	})
	resp.ID = msg.ID // 关联请求ID

	select {
	case agent.Send <- resp:
	default:
		log.Printf("Agent %s send channel full", agent.ID)
	}
}

// handleRPCResponse 处理 RPC 响应
func (s *Server) handleRPCResponse(msg *protocol.Message) {
	if ch, exists := s.registry.GetPendingResponse(msg.ID); exists {
		select {
		case ch <- msg:
		default:
			log.Printf("Response channel full for message %s", msg.ID)
		}
		s.registry.UnregisterPendingResponse(msg.ID)
	}
}

// handleHealth 健康检查
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"agents": len(s.registry.List()),
	})
}

// cleanupLoop 定期清理离线 Agent
func (s *Server) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			removed := s.registry.CleanupOffline()
			if len(removed) > 0 {
				log.Printf("Cleaned up offline agents: %v", removed)
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// CallAgent 调用 Agent（RPC）
func (s *Server) CallAgent(agentID, method string, params map[string]interface{}) (*protocol.Message, error) {
	agent, exists := s.registry.Get(agentID)
	if !exists {
		return nil, ErrAgentNotFound
	}

	// 创建响应通道
	respCh := make(chan *protocol.Message, 1)
	msgID := protocol.GenerateID()
	s.registry.RegisterPendingResponse(msgID, respCh)
	defer s.registry.UnregisterPendingResponse(msgID)

	// 发送请求
	req := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
		"method": method,
		"params": params,
	})
	req.ID = msgID

	select {
	case agent.Send <- req:
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("send timeout")
	}

	// 等待响应
	select {
	case resp := <-respCh:
		return resp, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("response timeout")
	}
}

// toStorageAgent 将 Agent 转换为存储模型
func toStorageAgent(agent *Agent) *storage.Agent {
	capabilities := make([]storage.Capability, len(agent.Capabilities))
	for i, cap := range agent.Capabilities {
		// 将 Metadata 转换为 Config (map[string]interface{})
		config := make(map[string]interface{})
		for k, v := range cap.Metadata {
			config[k] = v
		}
		if cap.Endpoint != "" {
			config["endpoint"] = cap.Endpoint
		}

		capabilities[i] = storage.Capability{
			Type:    cap.Type,
			Version: cap.Version,
			Config:  config,
		}
	}

	return &storage.Agent{
		ID:           agent.ID,
		Hostname:     agent.Hostname,
		IP:           agent.IP,
		Region:       agent.Location.Region,
		Zone:         agent.Location.Zone,
		Version:      "",  // Agent 当前没有版本字段
		Capabilities: capabilities,
		Status:       "online",
		LastSeen:     agent.LastSeen,
	}
}

// alertCheckLoop 定期检查并生成警告
func (s *Server) alertCheckLoop() {
	// 启动时立即执行一次
	go func() {
		time.Sleep(5 * time.Second) // 等待服务完全启动
		s.runAlertChecks()
	}()

	// 每小时检查一次
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// 每天凌晨2点清理旧警告
	cleanupTicker := time.NewTicker(24 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runAlertChecks()
		case <-cleanupTicker.C:
			s.cleanupOldAlerts()
		case <-s.ctx.Done():
			return
		}
	}
}

// runAlertChecks 执行警告检查
func (s *Server) runAlertChecks() {
	generator := alert.NewGenerator(s.db)
	generator.CheckAllChecks()
	log.Println("Alert checks completed")
}

// cleanupOldAlerts 清理旧警告
func (s *Server) cleanupOldAlerts() {
	generator := alert.NewGenerator(s.db)
	generator.CleanupOldAlerts(30 * 24 * time.Hour) // 保留30天
	log.Println("Old alerts cleaned up")
}
