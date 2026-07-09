package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/proxy"
	"github.com/cuihairu/cockpit/internal/storage"
	inventorysync "github.com/cuihairu/cockpit/internal/sync"
	"github.com/gorilla/websocket"
)

// Server WebSocket 服务器
type Server struct {
	addr           string
	registry       *Registry
	codec          *protocol.Codec
	db             *storage.DB
	audit          *audit.Logger
	proxyMgr       *proxy.Manager
	notification   *notification.Client
	remoteSessions *RemoteSessionManager
	ticketMgr      *TicketManager
	inventorySync  *inventorysync.Manager
	cfg            *config.Config
	upgrader       websocket.Upgrader

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewServer 创建新服务器
func NewServer(cfg *config.Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	// 打开数据库
	dbPath := cfg.Database.Path
	if dbPath == "" {
		dbPath = "./data/cockpit.db"
	}
	db, err := storage.Open(storage.Config{Path: dbPath})
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	// 配置 JWT
	if cfg.JWT != nil {
		if cfg.JWT.Secret != "" {
			auth.SetSecret(cfg.JWT.Secret)
		}
		if cfg.JWT.Expiration > 0 {
			auth.SetExpiration(cfg.JWT.Expiration)
		}
	}

	// 初始化通知客户端
	var notificationClient *notification.Client
	if cfg.Notification != nil && cfg.Notification.Enabled {
		notificationClient = notification.NewClient(cfg.Notification)
	}

	// 检查 TOTP 加密密钥
	if storage.IsUsingDefaultKey() {
		log.Println("WARNING: Using default TOTP encryption key. This is insecure for production!")
		log.Println("Please set TOTP_ENCRYPTION_KEY environment variable with a strong random key.")
	}
	// 在生产模式下强制验证密钥（可以通过环境变量 PRODUCTION=true 启用）
	if os.Getenv("PRODUCTION") == "true" {
		if err := storage.ValidateKey(); err != nil {
			log.Fatalf("SECURITY ERROR: %v", err)
		}
	}

	// 构造服务器地址
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	return &Server{
		addr:           addr,
		registry:       NewRegistry(),
		codec:          protocol.NewCodec(),
		db:             db,
		audit:          audit.NewLogger(db),
		proxyMgr:       proxy.NewManager(nil, db), // 将在 Start 中设置 ServerInterface
		notification:   notificationClient,
		remoteSessions: NewRemoteSessionManager(),
		ticketMgr:      NewTicketManager(),
		cfg:            cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin:     isOriginAllowed,
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	// 设置邮件配置
	auth.SetEmailConfig(s.cfg.Email)

	// 初始化认证（设置数据库）
	auth.InitDB(s.db)

	// 初始化管理员用户
	adminUser := getEnv("ADMIN_USERNAME", "admin")
	adminPass := os.Getenv("ADMIN_PASSWORD")

	// 强制要求设置密码
	if adminPass == "" {
		log.Fatal("SECURITY ERROR: ADMIN_PASSWORD environment variable is required for production use. Please set a strong password and restart.")
	}

	// 验证密码强度
	if len(adminPass) < 8 {
		log.Fatal("SECURITY ERROR: ADMIN_PASSWORD must be at least 8 characters long")
	}

	// 检查是否是常见的弱密码
	weakPasswords := []string{"password", "12345678", "admin123", "qwerty123", "abcdef12"}
	for _, weak := range weakPasswords {
		if adminPass == weak {
			log.Fatalf("SECURITY ERROR: ADMIN_PASSWORD is too weak (cannot use common password '%s')", weak)
		}
	}

	if err := auth.InitAdmin(s.db, adminUser, adminPass); err != nil {
		log.Printf("Warning: Failed to init admin user: %v", err)
	} else {
		log.Printf("Admin user initialized: %s", adminUser)
	}

	mux := http.NewServeMux()

	// 设置代理管理器的 ServerInterface
	s.proxyMgr = proxy.NewManager(s, s.db)

	// 启动代理管理器（启动已启用的代理）
	if err := s.proxyMgr.Start(); err != nil {
		log.Printf("Failed to start proxy manager: %v", err)
	}

	if err := s.startInventorySync(); err != nil {
		return fmt.Errorf("start inventory sync: %w", err)
	}

	// 注册所有路由
	s.registerRoutes(mux)

	// 应用审计中间件
	handler := s.AuditMiddleware(mux)

	server := &http.Server{
		Addr:    s.addr,
		Handler: handler,
	}

	log.Printf("Server starting on %s", s.addr)
	log.Printf("Web UI: http://%s", s.addr)

	// 启动清理协程
	go s.cleanupLoop()

	// 启动警告检查协程
	go s.alertCheckLoop()
	// 启动系统指标清理协程
	go s.metricsCleanupLoop()

	return server.ListenAndServe()
}

// registerRoutes 注册所有 HTTP 路由
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// 注册审计日志 API
	s.registerAuditAPI(mux)

	// 注册代理 API
	s.registerProxyAPI(mux)

	// 注册系统指标 API
	s.registerMetricsAPI(mux)
	s.registerDockerAPI(mux)

	// 注册远程连接 API
	s.registerRemoteAPI(mux)

	// 注册桌面连接 API
	s.registerDesktopAPI(mux)

	// 注册 VNC 连接 API
	s.registerVNCAPI(mux)

	// 公开路由
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/auth/login", s.handleLoginWithAudit)
	mux.HandleFunc("/api/auth/refresh", auth.HandleRefresh)
	mux.HandleFunc("/api/auth/totp/verify", s.handleTOTPVerify) // TOTP 验证不需要 JWT（使用临时令牌）
	mux.HandleFunc("/api/auth/forgot-password", s.handleForgotPassword)
	mux.HandleFunc("/api/auth/reset-password", s.handleResetPassword)
	mux.HandleFunc("/api/auth/verify-reset-code", s.handleVerifyResetCode)

	// 需要认证的 API 路由
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		// 登录相关接口不需要认证
		if strings.HasPrefix(r.URL.Path, "/api/auth/") {
			if r.URL.Path == "/api/auth/login" {
				s.handleLoginWithAudit(w, r)
			} else if r.URL.Path == "/api/auth/refresh" {
				auth.HandleRefresh(w, r)
			} else if r.URL.Path == "/api/auth/totp/verify" {
				s.handleTOTPVerify(w, r)
			}
			return
		}
		// TOTP 设置路由需要认证
		if r.URL.Path == "/api/auth/totp/generate" {
			auth.Middleware(s.handleTOTPGenerate)(w, r)
			return
		}
		if r.URL.Path == "/api/auth/totp/enable" {
			auth.Middleware(s.handleTOTPEnable)(w, r)
			return
		}
		if r.URL.Path == "/api/auth/totp/disable" {
			auth.Middleware(s.handleTOTPDisable)(w, r)
			return
		}
		// 其他 API 需要认证
		auth.Middleware(s.serveAPI)(w, r)
	})

	// Web UI (SPA) - 必须放在最后作为 fallback
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.spaHandler().ServeHTTP(w, r)
	})
}

// Shutdown 关闭服务器
func (s *Server) Shutdown() {
	s.cancel()
	if s.inventorySync != nil {
		s.inventorySync.Stop()
	}
	if s.proxyMgr != nil {
		s.proxyMgr.Stop()
	}
	if s.db != nil {
		s.db.Close()
	}
}

func (s *Server) startInventorySync() error {
	if s.cfg == nil || s.cfg.Inventory == nil || !s.cfg.Inventory.Watch {
		return nil
	}
	if s.cfg.Inventory.Path == "" {
		err := fmt.Errorf("inventory.watch enabled but inventory.path is empty")
		if s.cfg.Inventory.Strict {
			return err
		}
		log.Printf("Inventory sync disabled: %v", err)
		return nil
	}

	manager, err := inventorysync.NewManagerWithConfig(inventorysync.Config{
		InventoryPath: s.cfg.Inventory.Path,
		DB:            s.db,
		Strict:        s.cfg.Inventory.Strict,
	})
	if err != nil {
		if !s.cfg.Inventory.Strict {
			log.Printf("Inventory sync disabled: %v", err)
			return nil
		}
		return err
	}
	if err := manager.Start(); err != nil {
		if !s.cfg.Inventory.Strict {
			log.Printf("Inventory initial sync failed; watcher not started: %v", err)
			return nil
		}
		return err
	}
	s.inventorySync = manager
	return nil
}

// handleHealth 健康检查
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"agents": len(s.registry.List()),
	})
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

	storageAgent := &storage.Agent{
		ID:           agent.ID,
		Hostname:     agent.Hostname,
		IP:           agent.IP,
		Region:       agent.Location.Region,
		Zone:         agent.Location.Zone,
		Version:      "", // Agent 当前没有版本字段
		Capabilities: capabilities,
		Status:       "online",
		LastSeen:     agent.LastSeen,
		Labels:       agent.Labels,
	}

	// 添加虚拟化信息
	if agent.Virtualization != nil {
		storageAgent.VirtType = agent.Virtualization.Type
		storageAgent.VirtRole = agent.Virtualization.Role
	}

	return storageAgent
}

// handleLoginWithAudit 处理登录并记录审计日志
func (s *Server) handleLoginWithAudit(w http.ResponseWriter, r *http.Request) {
	// 在消费 body 之前先读取用户名用于审计
	var username string

	// 读取 body 用于解析
	body, err := io.ReadAll(r.Body)
	if err == nil {
		// 创建一个新的 reader 供后续使用
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		// 尝试解析用户名
		var loginReq auth.LoginRequest
		if json.Unmarshal(body, &loginReq) == nil {
			username = loginReq.Username
		}
	}

	// 创建一个 ResponseRecorder 来捕获响应状态码
	recorder := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	// 调用原始的登录处理函数
	auth.HandleLogin(recorder, r)

	// 根据响应状态码记录审计日志
	if username == "" {
		username = "unknown"
	}

	success := recorder.statusCode == http.StatusOK
	s.audit.LogLogin(
		username,
		success,
		s.getClientIP(r),
		r.UserAgent(),
	)
}

// ========== 代理管理相关方法 ==========

// SendToAgent 发送消息给指定 Agent
func (s *Server) SendToAgent(agentID string, msg *protocol.Message) error {
	agent, exists := s.registry.Get(agentID)
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}
	return agent.SendMessage(msg)
}

// GetAgentConn 获取 Agent 连接
func (s *Server) GetAgentConn(agentID string) (proxy.AgentConn, bool) {
	agent, exists := s.registry.Get(agentID)
	if !exists {
		return nil, false
	}
	return agent, true
}

// handleProxyData 处理代理数据消息
func (s *Server) handleProxyData(agent *Agent, msg *protocol.Message) {
	if s.proxyMgr == nil {
		return
	}

	p, err := protocol.DecodeProxyData(msg)
	if err != nil {
		log.Printf("decode proxy data from agent %s: %v", agent.ID, err)
		return
	}

	// 终端连接：显式标记或 proxyId 以 "terminal" 前缀
	if p.Terminal || hasPrefix(p.ProxyID, "terminal") {
		if err := s.HandleTerminalData(p.ConnID, p.Data); err != nil {
			log.Printf("HandleTerminalData error: %v", err)
		}
		return
	}

	// VNC 连接：proxyId 以 "vnc" 前缀
	if hasPrefix(p.ProxyID, "vnc") {
		if err := s.HandleVNCData(p.ConnID, p.Data); err != nil {
			log.Printf("HandleVNCData error: %v", err)
		}
		return
	}

	if err := s.proxyMgr.HandleProxyData(p.ProxyID, p.ConnID, p.Data); err != nil {
		log.Printf("HandleProxyData error: %v", err)
	}
}

// hasPrefix 安全的字符串前缀检查（避免多处重复 len+slice 模式）
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// handleProxyClose 处理代理关闭消息
func (s *Server) handleProxyClose(agent *Agent, msg *protocol.Message) {
	if s.proxyMgr == nil {
		return
	}

	p, err := protocol.DecodeProxyClose(msg)
	if err != nil {
		log.Printf("decode proxy close from agent %s: %v", agent.ID, err)
		return
	}

	// 检查是否是终端连接
	if p.Terminal || hasPrefix(p.ProxyID, "terminal") {
		s.HandleTerminalClose(p.ConnID, p.Reason)
		return
	}

	// 检查是否是 VNC 连接
	if hasPrefix(p.ProxyID, "vnc") {
		s.HandleVNCClose(p.ConnID, p.Reason)
		return
	}

	s.proxyMgr.HandleProxyClose(p.ProxyID, p.ConnID, p.Reason)
}

// handleProxyError 处理代理错误消息
func (s *Server) handleProxyError(agent *Agent, msg *protocol.Message) {
	p, err := protocol.DecodeProxyError(msg)
	if err != nil {
		log.Printf("decode proxy error from agent %s: %v", agent.ID, err)
		return
	}
	log.Printf("Proxy error from agent %s, proxy %s: %s", agent.ID, p.ProxyID, p.Error)
}
