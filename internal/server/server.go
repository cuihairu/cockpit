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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/dns"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/overlay"
	"github.com/cuihairu/cockpit/internal/probe"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/proxy/mgr"
	"github.com/cuihairu/cockpit/internal/storage"
	inventorysync "github.com/cuihairu/cockpit/internal/sync"
	"github.com/gorilla/websocket"
)

// CallAgent 超时参数。包级变量仅为测试可注入（cov_* 测试缩短等待），
// 默认值即生产取值，行为不变。
var (
	callAgentSendTimeout = 5 * time.Second
	callAgentTimeout     = 30 * time.Second
)

// Server WebSocket 服务器
type Server struct {
	addr           string
	registry       *Registry
	codec          *protocol.Codec
	db             *storage.DB
	auth           *auth.Service
	audit          *audit.Logger
	proxyMgr       *mgr.Manager
	notifier       *notification.Service
	remoteSessions *RemoteSessionManager
	ticketMgr      *TicketManager
	inventorySync  *inventorysync.Manager
	probeRunner    *probe.Runner
	dns            dns.Provider
	acme           AcmeIssuer
	acmeDNSConfig  func() AcmeDNSConfig
	overlayZT      *overlay.ZeroTierClient
	overlayTS      *overlay.TailscaleClient
	cfg            *config.Config
	upgrader       websocket.Upgrader

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc

	// backupTrackWG 跟踪在途的备份任务跟踪 goroutine（测试注入
	// backupTrackInterval 前等待其退出，保证恢复默认值不构成数据竞争）
	backupTrackWG sync.WaitGroup
}

// logFatalf 注入点：NewServer 启动防御（坏库路径/弱密码/生产密钥校验）以
// log.Fatal 终止进程，测试以 panic 哨兵截获后 recover 断言。
var logFatalf = log.Fatalf

// storageValidateKey 注入点：生产密钥校验依赖环境，测试可控其失败。
var storageValidateKey = storage.ValidateKey

// NewServer 创建新服务器
func NewServer(cfg *config.Config) *Server {
	cfg = config.Normalize(cfg)
	ctx, cancel := context.WithCancel(context.Background())

	// 打开数据库（config.Normalize 已保证 Path 非空）
	db, err := storage.Open(storage.Config{Path: cfg.Database.Path})
	if err != nil {
		logFatalf("Failed to open database: %v", err)
	}

	authService := auth.NewService(db, auth.Options{
		Secret:     cfg.JWT.Secret,
		Expiration: cfg.JWT.Expiration,
	})

	// 初始化多渠道通知服务（herald/ntfy/webhook/telegram；未启用时为空实现）
	notifier := notification.NewService(cfg.Notification)

	// 检查 TOTP 加密密钥
	if storage.IsUsingDefaultKey() {
		log.Println("WARNING: Using default TOTP encryption key. This is insecure for production!")
		log.Println("Please set TOTP_ENCRYPTION_KEY environment variable with a strong random key.")
	}
	// 在生产模式下强制验证密钥（可以通过环境变量 PRODUCTION=true 启用）
	if os.Getenv("PRODUCTION") == "true" {
		if err := storageValidateKey(); err != nil {
			logFatalf("SECURITY ERROR: %v", err)
		}
	}

	// 构造服务器地址
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	return &Server{
		addr:           addr,
		registry:       NewRegistry(),
		codec:          protocol.NewCodec(),
		db:             db,
		auth:           authService,
		audit:          audit.NewLogger(db),
		proxyMgr:       mgr.NewManager(nil, db), // 将在 Start 中设置 ServerInterface
		notifier:       notifier,
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

	// 初始化管理员用户
	adminUser := getEnv("ADMIN_USERNAME", "admin")
	adminPass := os.Getenv("ADMIN_PASSWORD")

	// 强制要求设置密码
	if adminPass == "" {
		logFatalf("%s", "SECURITY ERROR: ADMIN_PASSWORD environment variable is required for production use. Please set a strong password and restart.")
	}

	// 验证密码强度
	if len(adminPass) < 8 {
		logFatalf("%s", "SECURITY ERROR: ADMIN_PASSWORD must be at least 8 characters long")
	}

	// 检查是否是常见的弱密码
	weakPasswords := []string{"password", "12345678", "admin123", "qwerty123", "abcdef12"}
	for _, weak := range weakPasswords {
		if adminPass == weak {
			logFatalf("SECURITY ERROR: ADMIN_PASSWORD is too weak (cannot use common password '%s')", weak)
		}
	}

	if err := auth.InitAdmin(s.db, adminUser, adminPass); err != nil {
		log.Printf("Warning: Failed to init admin user: %v", err)
	} else {
		log.Printf("Admin user initialized: %s", adminUser)
	}

	mux := http.NewServeMux()

	// 设置代理管理器的 ServerInterface
	s.proxyMgr = mgr.NewManager(s, s.db)

	// 启动代理管理器（启动已启用的代理）
	if err := s.proxyMgr.Start(); err != nil {
		log.Printf("Failed to start proxy manager: %v", err)
	}

	if err := s.startInventorySync(); err != nil {
		return fmt.Errorf("start inventory sync: %w", err)
	}

	// 启动自动健康探测（5 分钟间隔）
	s.startProbeRunner()

	// DNS 管理 client（凭据未配置时为 nil，API 统一 503 引导；provider 按
	// dns.provider 分派，与 ACME 共用同一套键，见 dns-design.md D11）
	s.dns = dns.New(s.cfg.DNS.Provider,
		s.cfg.DNS.Cloudflare.APIToken, s.cfg.DNS.DNSPod.LoginToken,
		s.cfg.DNS.AliDNS.AccessKey, s.cfg.DNS.AliDNS.SecretKey)
	if s.dns != nil {
		log.Print("DNS provider enabled: ", s.dnsProviderName())
	}

	// ACME 签发器（lego DNS-01，provider 按 dns.provider 分派，见 acme-design.md D4/D13）
	s.acmeDNSConfig = func() AcmeDNSConfig { return newAcmeDNSConfig(s.cfg) }
	s.acme = NewLegoIssuer(s.db, s.acmeDNSConfig)

	// 组网云管理面 client（凭据未配置时为 nil，API 统一 503 引导，D11/D12）
	if s.cfg.Overlay != nil {
		if s.cfg.Overlay.ZeroTier != nil && s.cfg.Overlay.ZeroTier.APIToken != "" {
			s.overlayZT = overlay.NewZeroTier(s.cfg.Overlay.ZeroTier.APIToken)
			log.Print("Overlay cloud management enabled: zerotier")
		}
		if s.cfg.Overlay.Tailscale != nil && s.cfg.Overlay.Tailscale.APIToken != "" {
			s.overlayTS = overlay.NewTailscale(s.cfg.Overlay.Tailscale.APIToken, s.cfg.Overlay.Tailscale.Tailnet)
			log.Print("Overlay cloud management enabled: tailscale")
		}
	}

	// 注册所有路由
	s.registerRoutes(mux)

	// 预检请求必须先于认证处理，否则跨域 OPTIONS 会被 JWT 中间件拒绝。
	// RBAC 在 Audit 内层：403 也进审计链（rbac-design.md 笔 2）
	handler := s.CORSMiddleware(s.AuditMiddleware(s.RBACMiddleware(mux)))

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
	go s.driftScanLoop() // 漂移定时巡检（见 drift-design.md M2）
	go s.smartScanLoop() // SMART 磁盘健康巡检（见 disk-health-design.md D8）
	go s.ddnsScanLoop()  // DDNS 定时同步（见 ddns-design.md D6）
	go s.acmeScanLoop()  // ACME 证书自动续期巡检（见 acme-design.md D7）
	go s.nasScanLoop()   // NAS 存储巡检（见 nas-design.md D5）
	// 启动系统指标清理协程
	go s.metricsCleanupLoop()
	// 启动备份调度循环
	s.startBackupLoop()
	// 启动 server 自身数据库备份循环（见 server-backup-design.md）
	go s.serverBackupLoop()

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

	// 注册 Compose Stack API
	s.registerStacksAPI(mux)

	// 注册拨测与通知 API
	s.registerProbeAPI(mux)

	// 注册备份管理 API
	s.registerBackupsAPI(mux)

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
	mux.HandleFunc("/api/auth/refresh", s.authService().HandleRefresh)
	mux.HandleFunc("/api/auth/totp/verify", s.handleTOTPVerify) // TOTP 验证不需要 JWT（使用临时令牌）
	mux.HandleFunc("/api/auth/forgot-password", s.handleForgotPassword)
	mux.HandleFunc("/api/auth/reset-password", s.handleResetPassword)
	mux.HandleFunc("/api/auth/verify-reset-code", s.handleVerifyResetCode)

	// 需要认证的 API 路由
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		// 登录相关接口不需要认证；TOTP 设置路由需认证（曾因置于
		// /api/auth/ 前缀守卫 return 之后而不可达，现并入同一 switch）
		if strings.HasPrefix(r.URL.Path, "/api/auth/") {
			switch r.URL.Path {
			case "/api/auth/login":
				s.handleLoginWithAudit(w, r)
			case "/api/auth/refresh":
				s.authService().HandleRefresh(w, r)
			case "/api/auth/totp/verify":
				s.handleTOTPVerify(w, r)
			case "/api/auth/totp/generate":
				s.authService().Middleware(s.handleTOTPGenerate)(w, r)
			case "/api/auth/totp/enable":
				s.authService().Middleware(s.handleTOTPEnable)(w, r)
			case "/api/auth/totp/disable":
				s.authService().Middleware(s.handleTOTPDisable)(w, r)
			}
			return
		}
		// 其他 API 需要认证
		s.authService().Middleware(s.serveAPI)(w, r)
	})

	// Web UI (SPA) - 必须放在最后作为 fallback
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.spaHandler().ServeHTTP(w, r)
	})
}

func (s *Server) authService() *auth.Service {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.auth != nil {
		return s.auth
	}
	s.auth = auth.NewService(s.db, auth.Options{Secret: "test-secret", Expiration: 24 * time.Hour})
	return s.auth
}

// Shutdown 关闭服务器
func (s *Server) Shutdown() {
	s.cancel()
	if s.inventorySync != nil {
		s.inventorySync.Stop()
	}
	if s.probeRunner != nil {
		s.probeRunner.Stop()
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

// startProbeRunner 启动自动健康探测；DB 中保存过间隔则覆盖默认值
func (s *Server) startProbeRunner() {
	interval := probe.DefaultInterval
	if v, err := s.db.GetSetting(probe.IntervalSettingKey); err == nil && v != "" {
		if secs, convErr := strconv.Atoi(v); convErr == nil &&
			secs >= probe.MinIntervalSeconds && secs <= probe.MaxIntervalSeconds {
			interval = time.Duration(secs) * time.Second
		}
	}
	s.probeRunner = probe.NewRunner(s.db, interval, s.notifier)
	s.probeRunner.LoadFailThreshold()
	s.probeRunner.Start()
	log.Printf("Probe runner started (interval: %s)", interval)
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

	// 发送请求（与 agent.Close 并发安全；通道满时重试至超时）
	if err := agent.SendWithTimeout(req, callAgentSendTimeout); err != nil {
		return nil, err
	}

	// 等待响应
	select {
	case resp := <-respCh:
		return resp, nil
	case <-time.After(callAgentTimeout):
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
	s.authService().HandleLogin(recorder, r)

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
func (s *Server) GetAgentConn(agentID string) (mgr.AgentConn, bool) {
	agent, exists := s.registry.Get(agentID)
	if !exists {
		return nil, false
	}
	return agent, true
}

// handleProxyData 处理代理数据消息
func (s *Server) handleProxyData(agent *Agent, msg *protocol.Message) {
	p, err := protocol.DecodeProxyData(msg)
	if err != nil {
		log.Printf("decode proxy data from agent %s: %v", agent.ID, err)
		return
	}

	// 日志实时尾随：proxyId 以 "logs:" 前缀（不依赖 proxyMgr，见 api_logs_follow.go）
	if hasPrefix(p.ProxyID, "logs:") {
		s.HandleLogsFollowData(p.ProxyID, p.Data)
		return
	}

	if s.proxyMgr == nil {
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
	p, err := protocol.DecodeProxyClose(msg)
	if err != nil {
		log.Printf("decode proxy close from agent %s: %v", agent.ID, err)
		return
	}

	// 日志实时尾随终止：转 eof 帧收尾（不依赖 proxyMgr）
	if hasPrefix(p.ProxyID, "logs:") {
		s.HandleLogsFollowClose(p.ProxyID, p.Reason)
		return
	}

	if s.proxyMgr == nil {
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
