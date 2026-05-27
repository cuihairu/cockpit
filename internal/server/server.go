package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/proxy"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/gorilla/websocket"
	"github.com/cuihairu/cockpit/internal/audit"
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// isOriginAllowed 检查 WebSocket 升级请求的 Origin 是否在白名单内
func isOriginAllowed(r *http.Request) bool {
	allowed := os.Getenv("ALLOWED_ORIGINS")
	if allowed == "" {
		// 未配置白名单时允许所有来源（开发模式）
		log.Println("WARNING: ALLOWED_ORIGINS not set, accepting all WebSocket origins. Configure this in production!")
		return true
	}
	origin := r.Header.Get("Origin")
	for _, a := range strings.Split(allowed, ",") {
		a = strings.TrimSpace(a)
		if a == "*" || a == origin {
			return true
		}
	}
	return false
}

// Server WebSocket 服务器
type Server struct {
	addr         string
	registry     *Registry
	codec        *protocol.Codec
	db           *storage.DB
	audit        *audit.Logger
	proxyMgr     *proxy.Manager
	notification *notification.Client
	cfg          *config.Config
	upgrader     websocket.Upgrader

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

	// 初始化通知客户端
	var notificationClient *notification.Client
	if cfg.Notification != nil && cfg.Notification.Enabled {
		notificationClient = notification.NewClient(cfg.Notification)
	}

	// 构造服务器地址
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	return &Server{
		addr:         addr,
		registry:     NewRegistry(),
		codec:        protocol.NewCodec(),
		db:           db,
		audit:        audit.NewLogger(db),
		proxyMgr:     proxy.NewManager(nil, db), // 将在 Start 中设置 ServerInterface
		notification: notificationClient,
		cfg:          cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: isOriginAllowed,
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
	adminPass := getEnv("ADMIN_PASSWORD", "admin")
	if adminPass == "admin" {
		log.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		log.Println("!! WARNING: Using DEFAULT admin password 'admin'!      !!")
		log.Println("!! Set ADMIN_PASSWORD environment variable immediately! !!")
		log.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	}
	if err := auth.InitAdmin(s.db, adminUser, adminPass); err != nil {
		log.Printf("Warning: Failed to init admin user: %v", err)
	} else {
		log.Printf("Admin user initialized: %s", adminUser)
	}

	mux := http.NewServeMux()

	// 设置代理管理器的 ServerInterface
	s.proxyMgr = proxy.NewManager(s, s.db)

	// 注册审计日志 API
	s.registerAuditAPI(mux)

	// 注册代理 API
	s.registerProxyAPI(mux)
		// 注册系统指标 API
		s.registerMetricsAPI(mux)

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

// Shutdown 关闭服务器
func (s *Server) Shutdown() {
	s.cancel()
	if s.proxyMgr != nil {
		s.proxyMgr.Stop()
	}
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
	case protocol.MessageTypeProxyData:
		s.handleProxyData(agent, msg)
	case protocol.MessageTypeProxyClose:
		s.handleProxyClose(agent, msg)
	case protocol.MessageTypeProxyError:
		s.handleProxyError(agent, msg)
	case protocol.MessageTypeDesktopData:
		s.HandleDesktopData(msg)
	case protocol.MessageTypeDesktopClose:
		s.HandleDesktopClose(msg)
	default:
		log.Printf("Unknown message type: %s from agent %s", msg.Type, agent.ID)
	}
}

// handleHeartbeat 处理心跳
func (s *Server) handleHeartbeat(agent *Agent, msg *protocol.Message) {
	agent.Heartbeat()

	// 解析心跳负载，检查是否包含系统信息
	if msg.Payload != nil {
		if systemInfoRaw, ok := msg.Payload["systemInfo"]; ok {
			// 尝试解析系统信息
			if systemInfoMap, ok := systemInfoRaw.(map[string]interface{}); ok {
				s.handleSystemInfo(agent.ID, systemInfoMap)
			}
		}
	}

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

// handleSystemInfo 处理系统信息
func (s *Server) handleSystemInfo(agentID string, systemInfo map[string]interface{}) {
	now := time.Now()

	// 保存历史指标
	metric := &storage.SystemMetric{
		AgentID:   agentID,
		Timestamp: now,
	}

	// 解析 CPU 信息
	if v, ok := systemInfo["cpuUsage"].(float64); ok {
		metric.CPUUsage = v
	}
	if v, ok := systemInfo["cpuCores"].(float64); ok {
		metric.CPUCores = int(v)
	}
	if v, ok := systemInfo["cpuFreqMhz"].(float64); ok {
		metric.CPUFreqMHz = v
	}

	// 解析内存信息
	if v, ok := systemInfo["memTotal"].(float64); ok {
		metric.MemTotal = uint64(v)
	}
	if v, ok := systemInfo["memUsed"].(float64); ok {
		metric.MemUsed = uint64(v)
	}
	if v, ok := systemInfo["memAvailable"].(float64); ok {
		metric.MemAvailable = uint64(v)
	}
	if v, ok := systemInfo["memUsagePercent"].(float64); ok {
		metric.MemUsagePercent = v
	}

	// 解析磁盘信息
	if v, ok := systemInfo["diskTotal"].(float64); ok {
		metric.DiskTotal = uint64(v)
	}
	if v, ok := systemInfo["diskUsed"].(float64); ok {
		metric.DiskUsed = uint64(v)
	}
	if v, ok := systemInfo["diskFree"].(float64); ok {
		metric.DiskFree = uint64(v)
	}
	if v, ok := systemInfo["diskUsagePercent"].(float64); ok {
		metric.DiskUsagePercent = v
	}

	// 解析网络信息
	if v, ok := systemInfo["netBytesSent"].(float64); ok {
		metric.NetBytesSent = uint64(v)
	}
	if v, ok := systemInfo["netBytesRecv"].(float64); ok {
		metric.NetBytesRecv = uint64(v)
	}

	// 解析系统信息
	if v, ok := systemInfo["osName"].(string); ok {
		metric.OSName = v
	}
	if v, ok := systemInfo["osVersion"].(string); ok {
		metric.OSVersion = v
	}
	if v, ok := systemInfo["arch"].(string); ok {
		metric.Arch = v
	}
	if v, ok := systemInfo["uptime"].(float64); ok {
		metric.Uptime = uint64(v)
	}

	// 解析负载信息
	if v, ok := systemInfo["load1"].(float64); ok {
		metric.Load1 = v
	}
	if v, ok := systemInfo["load5"].(float64); ok {
		metric.Load5 = v
	}
	if v, ok := systemInfo["load15"].(float64); ok {
		metric.Load15 = v
	}

	metric.CreatedAt = now

	// 保存到数据库
	if err := s.db.SaveSystemMetric(metric); err != nil {
		log.Printf("Failed to save system metric for agent %s: %v", agentID, err)
	}

	// 更新快照
	snapshot := &storage.SystemInfoSnapshot{
		AgentID:         agentID,
		CPUUsage:        metric.CPUUsage,
		CPUCores:        metric.CPUCores,
		CPUFreqMHz:      metric.CPUFreqMHz,
		MemTotal:        metric.MemTotal,
		MemUsed:         metric.MemUsed,
		MemAvailable:    metric.MemAvailable,
		MemUsagePercent: metric.MemUsagePercent,
		DiskTotal:       metric.DiskTotal,
		DiskUsed:        metric.DiskUsed,
		DiskFree:        metric.DiskFree,
		DiskUsagePercent: metric.DiskUsagePercent,
		NetBytesSent:    metric.NetBytesSent,
		NetBytesRecv:    metric.NetBytesRecv,
		OSName:          metric.OSName,
		OSVersion:       metric.OSVersion,
		Arch:            metric.Arch,
		Uptime:          metric.Uptime,
		Load1:           metric.Load1,
		Load5:           metric.Load5,
		Load15:          metric.Load15,
		UpdatedAt:       now,
	}

	if v, ok := systemInfo["hostname"].(string); ok {
		snapshot.Hostname = v
	}

	if err := s.db.UpdateSystemInfoSnapshot(snapshot); err != nil {
		log.Printf("Failed to update system info snapshot for agent %s: %v", agentID, err)
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
	generator := alert.NewGenerator(s.db, s.notification, s.cfg.Notification)
	generator.CheckAllChecks()
	log.Println("Alert checks completed")
}

// cleanupOldAlerts 清理旧警告
func (s *Server) cleanupOldAlerts() {
	generator := alert.NewGenerator(s.db, s.notification, s.cfg.Notification)
	generator.CleanupOldAlerts(30 * 24 * time.Hour) // 保留30天
	log.Println("Old alerts cleaned up")
}

// handleLoginWithAudit 处理登录并记录审计日志
func (s *Server) handleLoginWithAudit(w http.ResponseWriter, r *http.Request) {
	// 创建一个 ResponseRecorder 来捕获响应状态码
	recorder := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	// 调用原始的登录处理函数
	auth.HandleLogin(recorder, r)

	// 根据响应状态码记录审计日志
	username := r.FormValue("username")
	if username == "" {
		// 尝试从 JSON body 读取
		if err := r.ParseForm(); err == nil {
			username = r.FormValue("username")
		}
	}
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

	proxyID, _ := msg.Payload["proxyId"].(string)
	connID, _ := msg.Payload["connId"].(string)

	// 检查是否是终端连接
	terminalFlag, _ := msg.Payload["terminal"].(bool)
	if terminalFlag || (len(proxyID) > 8 && proxyID[:8] == "terminal") {
		// 解析数据
		var data []byte
		switch v := msg.Payload["data"].(type) {
		case string:
			data = []byte(v)
		case []byte:
			data = v
		case []interface{}:
			data = make([]byte, len(v))
			for i, b := range v {
				if f, ok := b.(float64); ok {
					data[i] = byte(f)
				}
			}
		}
		// 转发给终端会话
		if err := s.HandleTerminalData(connID, data); err != nil {
			log.Printf("HandleTerminalData error: %v", err)
		}
		return
	}

	// 检查是否是 VNC 连接
	if len(proxyID) > 3 && proxyID[:3] == "vnc" {
		var data []byte
		switch v := msg.Payload["data"].(type) {
		case string:
			data = []byte(v)
		case []byte:
			data = v
		case []interface{}:
			data = make([]byte, len(v))
			for i, b := range v {
				if f, ok := b.(float64); ok {
					data[i] = byte(f)
				}
			}
		}
		if err := s.HandleVNCData(connID, data); err != nil {
			log.Printf("HandleVNCData error: %v", err)
		}
		return
	}

	// 解析数据
	var data []byte
	switch v := msg.Payload["data"].(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	case []interface{}:
		data = make([]byte, len(v))
		for i, b := range v {
			if f, ok := b.(float64); ok {
				data[i] = byte(f)
			}
		}
	}

	if err := s.proxyMgr.HandleProxyData(proxyID, connID, data); err != nil {
		log.Printf("HandleProxyData error: %v", err)
	}
}

// handleProxyClose 处理代理关闭消息
func (s *Server) handleProxyClose(agent *Agent, msg *protocol.Message) {
	if s.proxyMgr == nil {
		return
	}

	proxyID, _ := msg.Payload["proxyId"].(string)
	connID, _ := msg.Payload["connId"].(string)
	reason, _ := msg.Payload["reason"].(string)

	// 检查是否是终端连接
	terminalFlag, _ := msg.Payload["terminal"].(bool)
	if terminalFlag || (len(proxyID) > 8 && proxyID[:8] == "terminal") {
		s.HandleTerminalClose(connID, reason)
		return
	}

	// 检查是否是 VNC 连接
	if len(proxyID) > 3 && proxyID[:3] == "vnc" {
		s.HandleVNCClose(connID, reason)
		return
	}

	s.proxyMgr.HandleProxyClose(proxyID, connID, reason)
}

// handleProxyError 处理代理错误消息
func (s *Server) handleProxyError(agent *Agent, msg *protocol.Message) {
	proxyID, _ := msg.Payload["proxyId"].(string)
	errorMsg, _ := msg.Payload["error"].(string)

	log.Printf("Proxy error from agent %s, proxy %s: %s", agent.ID, proxyID, errorMsg)
}

// metricsCleanupLoop 清理旧的系统指标
func (s *Server) metricsCleanupLoop() {
	// 每天凌晨3点清理
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// 启动时先等待到下次清理时间
	now := time.Now()
	nextCleanup := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	if nextCleanup.Before(now) {
		nextCleanup = nextCleanup.Add(24 * time.Hour)
	}
	time.Sleep(time.Until(nextCleanup))

	for {
		// 清理30天前的数据
		count, err := s.db.CleanupOldMetrics(30 * 24 * time.Hour)
		if err != nil {
			log.Printf("Failed to cleanup old metrics: %v", err)
		} else {
			log.Printf("Cleaned up %d old metric records", count)
		}

		select {
		case <-ticker.C:
			// 继续下一次清理
		case <-s.ctx.Done():
			return
		}
	}
}
