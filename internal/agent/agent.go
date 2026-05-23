package agent

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/agent/detector"
	"github.com/cuihairu/cockpit/internal/agent/rpc"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// Agent Cockpit Agent
type Agent struct {
	serverURL string
	conn      *websocket.Conn
	codec     *protocol.Codec
	rpc       *rpc.Handler

	// 注册信息
	agentID  string
	location protocol.Location
	capabilities []protocol.Capability

	// 状态
	mu        sync.RWMutex
	connected bool
	ctx       context.Context
	cancel    context.CancelFunc

	// 配置
	config *Config
}

// Config Agent 配置
type Config struct {
	ServerURL   string            `json:"server_url"`
	AgentID     string            `json:"agent_id,omitempty"`
	Region      string            `json:"region,omitempty"`
	Zone        string            `json:"zone,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// NewAgent 创建新 Agent
func NewAgent(cfg Config) *Agent {
	ctx, cancel := context.WithCancel(context.Background())

	return &Agent{
		serverURL:    cfg.ServerURL,
		codec:        protocol.NewCodec(),
		rpc:          rpc.NewHandler(),
		capabilities: []protocol.Capability{},
		ctx:          ctx,
		cancel:       cancel,
		config:       &cfg,
	}
}

// Start 启动 Agent
func (a *Agent) Start() error {
	log.Printf("Starting Cockpit Agent...")

	// 1. 运行能力检测
	log.Printf("Running capability detection...")
	a.capabilities = a.detectCapabilities()
	log.Printf("Detected %d capabilities", len(a.capabilities))
	for _, cap := range a.capabilities {
		log.Printf("  - %s: %s", cap.Type, cap.Endpoint)
	}

	// 2. 连接 Server
	if err := a.connect(); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	// 3. 注册
	if err := a.register(); err != nil {
		return fmt.Errorf("register failed: %w", err)
	}

	// 4. 启动心跳
	go a.heartbeatLoop()

	// 5. 启动消息循环
	go a.messageLoop()

	log.Printf("Agent started successfully")

	// 等待退出
	<-a.ctx.Done()
	return nil
}

// Stop 停止 Agent
func (a *Agent) Stop() {
	a.cancel()
	if a.conn != nil {
		a.conn.Close()
	}
}

// connect 连接到 Server
func (a *Agent) connect() error {
	log.Printf("Connecting to %s...", a.serverURL)

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(a.serverURL, nil)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.conn = conn
	a.connected = true
	a.mu.Unlock()

	log.Printf("Connected to server")
	return nil
}

// detectCapabilities 检测能力
func (a *Agent) detectCapabilities() []protocol.Capability {
	var capabilities []protocol.Capability

	detectors := detector.All()
	log.Printf("Running %d detectors...", len(detectors))

	// 按优先级排序
	sort.Slice(detectors, func(i, j int) bool {
		return detectors[i].Priority() < detectors[j].Priority()
	})

	for _, d := range detectors {
		log.Printf("Running detector: %s (priority: %d)", d.Name(), d.Priority())
		cap, err := d.Detect()
		if err != nil {
			log.Printf("  Detector %s failed: %v", d.Name(), err)
		} else if cap != nil {
			log.Printf("  Detected: %s", cap.Type)
			capabilities = append(capabilities, *cap)
		} else {
			log.Printf("  Not detected: %s", d.Name())
		}
	}

	return capabilities
}

// register 注册到 Server
func (a *Agent) register() error {
	// 确定 Agent ID
	if a.config.AgentID != "" {
		a.agentID = a.config.AgentID
	} else {
		// 获取主机名
		hostname, _ := os.Hostname()
		a.agentID = protocol.GenerateIDWithPrefix("agent-" + hostname)
	}

	// 确定位置
	a.location = protocol.Location{
		Region: a.config.Region,
		Zone:   a.config.Zone,
	}

	// 如果配置没有指定，尝试自动检测
	if a.location.Region == "" {
		a.location = a.detectLocation()
	}

	// 构建注册消息
	hostname, _ := os.Hostname()

	payload := map[string]any{
		"agentId":      a.agentID,
		"location":     a.location,
		"capabilities": a.capabilities,
		"hostname":     hostname,
	}

	// 发送注册消息
	msg := protocol.NewMessage(protocol.MessageTypeRegister, payload)

	if err := a.codec.WriteMessage(a.conn, msg); err != nil {
		return err
	}

	log.Printf("Registered as agent: %s at %s/%s", a.agentID, a.location.Region, a.location.Zone)

	// 等待响应
	resp, err := a.codec.ReadMessage(a.conn)
	if err != nil {
		return err
	}

	if resp.Type != protocol.MessageTypeRegister {
		return fmt.Errorf("expected register response, got: %s", resp.Type)
	}

	log.Printf("Registration accepted")
	return nil
}

// detectLocation 检测位置信息
func (a *Agent) detectLocation() protocol.Location {
	// 默认位置
	loc := protocol.Location{
		Region: "unknown",
		Zone:   "unknown",
	}

	// TODO: 实现位置检测逻辑
	// - 检查 IP 地理位置
	// - 检查配置文件
	// - 检查环境变量

	return loc
}

// heartbeatLoop 心跳循环
func (a *Agent) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.sendHeartbeat()
		case <-a.ctx.Done():
			return
		}
	}
}

// sendHeartbeat 发送心跳
func (a *Agent) sendHeartbeat() {
	a.mu.RLock()
	conn := a.conn
	a.mu.RUnlock()

	if conn == nil {
		return
	}

	msg := protocol.NewMessage(protocol.MessageTypeHeartbeat, map[string]any{
		"agentId": a.agentID,
		"status":  "online",
	})

	if err := a.codec.WriteMessage(conn, msg); err != nil {
		log.Printf("Send heartbeat failed: %v", err)
		// 尝试重连
		go a.reconnect()
	}
}

// messageLoop 消息循环
func (a *Agent) messageLoop() {
	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}

		a.mu.RLock()
		conn := a.conn
		a.mu.RUnlock()

		if conn == nil {
			time.Sleep(1 * time.Second)
			continue
		}

		msg, err := a.codec.ReadMessage(conn)
		if err != nil {
			log.Printf("Read message failed: %v", err)
			go a.reconnect()
			return
		}

		a.handleMessage(msg)
	}
}

// handleMessage 处理消息
func (a *Agent) handleMessage(msg *protocol.Message) {
	log.Printf("Received message: %s", msg.Type)

	switch msg.Type {
	case protocol.MessageTypePing:
		a.handlePing(msg)
	case protocol.MessageTypeRPCRequest:
		a.handleRPCRequest(msg)
	case protocol.MessageTypeHeartbeat:
		// 心跳响应
		log.Printf("Heartbeat acknowledged")
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

// handlePing 处理 Ping
func (a *Agent) handlePing(msg *protocol.Message) {
	resp := protocol.NewMessage(protocol.MessageTypeHeartbeat, map[string]any{
		"status":     "pong",
		"serverTime": time.Now().Unix(),
	})
	resp.ID = msg.ID

	a.mu.RLock()
	conn := a.conn
	a.mu.RUnlock()

	if conn != nil {
		a.codec.WriteMessage(conn, resp)
	}
}

// handleRPCRequest 处理 RPC 请求
func (a *Agent) handleRPCRequest(msg *protocol.Message) {
	log.Printf("RPC request: %v", msg.Payload)

	// 使用 RPC 处理器
	resp, err := a.rpc.Handle(msg)
	if err != nil {
		log.Printf("RPC error: %v", err)
		resp = protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]any{
			"status": "error",
			"error":  err.Error(),
		})
		resp.ID = msg.ID
	}

	a.mu.RLock()
	conn := a.conn
	a.mu.RUnlock()

	if conn != nil {
		if err := a.codec.WriteMessage(conn, resp); err != nil {
			log.Printf("Send RPC response failed: %v", err)
		}
	}
}

// reconnect 重连
func (a *Agent) reconnect() {
	log.Printf("Attempting to reconnect...")

	a.mu.Lock()
	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
	a.connected = false
	a.mu.Unlock()

	// 等待后重连
	time.Sleep(5 * time.Second)

	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}

		if err := a.connect(); err != nil {
			log.Printf("Reconnect failed: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		if err := a.register(); err != nil {
			log.Printf("Re-register failed: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		log.Printf("Reconnected successfully")
		go a.messageLoop()
		return
	}
}
