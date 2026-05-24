package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// Manager 代理管理器
type Manager struct {
	server   ServerInterface       // Server 接口，用于发送消息给 Agent
	db       *storage.DB           // 数据库
	proxies  map[string]*Proxy     // proxyID -> Proxy
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	running  atomic.Bool
}

// ServerInterface Server 接口，用于代理管理器与 Server 通信
type ServerInterface interface {
	SendToAgent(agentID string, msg *protocol.Message) error
	GetAgentConn(agentID string) (AgentConn, bool)
}

// AgentConn Agent 连接接口
type AgentConn interface {
	SendMessage(msg *protocol.Message) error
	AgentID() string
}

// Proxy 代理实例
type Proxy struct {
	config    *storage.Proxy
	listener  net.Listener
	conns     map[string]*ProxyConn // connID -> ProxyConn
	connSeq   atomic.Uint64
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// ProxyConn 代理连接
type ProxyConn struct {
	ID       string
	ProxyID  string
	Conn     net.Conn
	AgentID  string
	Created  time.Time
	LastRead time.Time
	mu       sync.RWMutex
	closed   atomic.Bool
}

// NewManager 创建代理管理器
func NewManager(server ServerInterface, db *storage.DB) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		server:  server,
		db:      db,
		proxies: make(map[string]*Proxy),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动代理管理器
func (m *Manager) Start() error {
	if !m.running.CompareAndSwap(false, true) {
		return fmt.Errorf("manager already running")
	}

	// 加载启用的代理配置
	proxies, err := m.db.ListEnabledProxies()
	if err != nil {
		return fmt.Errorf("load proxies: %w", err)
	}

	// 启动每个代理
	for _, proxy := range proxies {
		if err := m.StartProxy(proxy); err != nil {
			log.Printf("Failed to start proxy %s: %v", proxy.ID, err)
		}
	}

	// 启动清理协程
	go m.cleanupLoop()

	log.Printf("Proxy manager started with %d proxies", len(proxies))
	return nil
}

// Stop 停止代理管理器
func (m *Manager) Stop() {
	if !m.running.CompareAndSwap(true, false) {
		return
	}

	m.cancel()

	// 停止所有代理
	m.mu.Lock()
	for _, proxy := range m.proxies {
		proxy.Stop()
	}
	m.mu.Unlock()

	log.Println("Proxy manager stopped")
}

// StartProxy 启动单个代理
func (m *Manager) StartProxy(config *storage.Proxy) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已存在
	if _, exists := m.proxies[config.ID]; exists {
		return fmt.Errorf("proxy %s already running", config.ID)
	}

	// 创建代理实例
	proxy := &Proxy{
		config: config,
		conns:  make(map[string]*ProxyConn),
	}

	ctx, cancel := context.WithCancel(m.ctx)
	proxy.ctx = ctx
	proxy.cancel = cancel

	// 启动监听
	addr := fmt.Sprintf(":%d", config.RemotePort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		cancel()
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	proxy.listener = listener

	m.proxies[config.ID] = proxy

	// 启动接受连接的协程
	go m.acceptConnections(proxy)

	log.Printf("Proxy %s (%s) started on port %d, target %s",
		config.ID, config.Name, config.RemotePort, config.Target)
	return nil
}

// StopProxy 停止单个代理
func (m *Manager) StopProxy(proxyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	proxy, exists := m.proxies[proxyID]
	if !exists {
		return fmt.Errorf("proxy %s not found", proxyID)
	}

	proxy.Stop()
	delete(m.proxies, proxyID)

	log.Printf("Proxy %s stopped", proxyID)
	return nil
}

// ReloadProxy 重新加载代理配置
func (m *Manager) ReloadProxy(config *storage.Proxy) error {
	// 先停止旧的
	m.StopProxy(config.ID)

	// 如果是启用状态，启动新的
	if config.Enabled {
		return m.StartProxy(config)
	}

	return nil
}

// acceptConnections 接受连接
func (m *Manager) acceptConnections(proxy *Proxy) {
	for {
		conn, err := proxy.listener.Accept()
		if err != nil {
			select {
			case <-proxy.ctx.Done():
				return
			default:
				log.Printf("Accept error on proxy %s: %v", proxy.config.ID, err)
				continue
			}
		}

		// 处理新连接
		go m.handleConnection(proxy, conn)
	}
}

// handleConnection 处理新连接
func (m *Manager) handleConnection(proxy *Proxy, clientConn net.Conn) {
	// 生成连接ID
	connID := fmt.Sprintf("%s-%d", proxy.config.ID, proxy.connSeq.Add(1))

	proxyConn := &ProxyConn{
		ID:      connID,
		ProxyID: proxy.config.ID,
		Conn:    clientConn,
		AgentID: proxy.config.AgentID,
		Created: time.Now(),
	}

	proxy.mu.Lock()
	proxy.conns[connID] = proxyConn
	proxy.mu.Unlock()

	log.Printf("New connection %s on proxy %s from %s",
		connID, proxy.config.ID, clientConn.RemoteAddr())

	// 向 Agent 发起新连接请求
	newConnMsg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"proxyId":   proxy.config.ID,
		"proxyType": proxy.config.ProxyType,
		"target":    proxy.config.Target,
	})
	newConnMsg.Payload["connId"] = connID
	newConnMsg.Payload["newConn"] = true

	if err := m.server.SendToAgent(proxy.config.AgentID, newConnMsg); err != nil {
		log.Printf("Failed to send new conn message to agent %s: %v", proxy.config.AgentID, err)
		clientConn.Close()
		proxy.mu.Lock()
		delete(proxy.conns, connID)
		proxy.mu.Unlock()
		return
	}

	// 启动数据读取协程
	go m.readFromClient(proxy, proxyConn)

	// 设置连接超时
	go func() {
		select {
		case <-time.After(30 * time.Second):
			if !proxyConn.closed.Load() {
				log.Printf("Connection %s timeout waiting for agent", connID)
				proxyConn.Close()
			}
		case <-proxy.ctx.Done():
		}
	}()
}

// readFromClient 从客户端读取数据并转发给 Agent
func (m *Manager) readFromClient(proxy *Proxy, proxyConn *ProxyConn) {
	defer proxyConn.Close()

	buf := make([]byte, 32*1024) // 32KB buffer

	for {
		n, err := proxyConn.Conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Read error from client %s: %v", proxyConn.ID, err)
			}
			break
		}

		proxyConn.mu.Lock()
		proxyConn.LastRead = time.Now()
		proxyConn.mu.Unlock()

		// 转发数据给 Agent
		dataMsg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
			"proxyId": proxy.config.ID,
			"connId":  proxyConn.ID,
			"data":    buf[:n],
		})

		if err := m.server.SendToAgent(proxy.config.AgentID, dataMsg); err != nil {
			log.Printf("Failed to send data to agent %s: %v", proxy.config.AgentID, err)
			break
		}
	}

	// 通知 Agent 关闭连接
	closeMsg := protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
		"proxyId": proxy.config.ID,
		"connId":  proxyConn.ID,
		"reason":  "client closed",
	})
	m.server.SendToAgent(proxy.config.AgentID, closeMsg)
}

// HandleProxyData 处理来自 Agent 的数据
func (m *Manager) HandleProxyData(proxyID, connID string, data []byte) error {
	m.mu.RLock()
	proxy, exists := m.proxies[proxyID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("proxy %s not found", proxyID)
	}

	proxy.mu.RLock()
	conn, exists := proxy.conns[connID]
	proxy.mu.RUnlock()

	if !exists {
		return fmt.Errorf("connection %s not found", connID)
	}

	if conn.closed.Load() {
		return fmt.Errorf("connection %s already closed", connID)
	}

	// 写入数据到客户端
	_, err := conn.Conn.Write(data)
	if err != nil {
		log.Printf("Write error to client %s: %v", connID, err)
		conn.Close()
		return err
	}

	return nil
}

// HandleProxyClose 处理来自 Agent 的关闭连接请求
func (m *Manager) HandleProxyClose(proxyID, connID, reason string) {
	m.mu.RLock()
	proxy, exists := m.proxies[proxyID]
	m.mu.RUnlock()

	if !exists {
		return
	}

	proxy.mu.Lock()
	conn, exists := proxy.conns[connID]
	if exists {
		delete(proxy.conns, connID)
	}
	proxy.mu.Unlock()

	if exists {
		log.Printf("Connection %s closed by agent: %s", connID, reason)
		conn.Close()
	}
}

// GetProxyStatus 获取代理状态
func (m *Manager) GetProxyStatus(proxyID string) (map[string]interface{}, error) {
	m.mu.RLock()
	proxy, exists := m.proxies[proxyID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("proxy %s not found", proxyID)
	}

	proxy.mu.RLock()
	defer proxy.mu.RUnlock()

	conns := make([]map[string]interface{}, 0, len(proxy.conns))
	for _, conn := range proxy.conns {
		conns = append(conns, map[string]interface{}{
			"id":        conn.ID,
			"remote":    conn.Conn.RemoteAddr().String(),
			"created":   conn.Created,
			"lastRead":  conn.LastRead,
			"closed":    conn.closed.Load(),
		})
	}

	return map[string]interface{}{
		"id":         proxy.config.ID,
		"name":       proxy.config.Name,
		"status":     "running",
		"connections": conns,
		"connCount":  len(conns),
	}, nil
}

// GetAllStatus 获取所有代理状态
func (m *Manager) GetAllStatus() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make([]map[string]interface{}, 0, len(m.proxies))
	for _, proxy := range m.proxies {
		proxy.mu.RLock()
		conns := make([]map[string]interface{}, 0, len(proxy.conns))
		for _, conn := range proxy.conns {
			conns = append(conns, map[string]interface{}{
				"id":       conn.ID,
				"remote":   conn.Conn.RemoteAddr().String(),
				"created":  conn.Created,
				"lastRead": conn.LastRead,
			})
		}

		status = append(status, map[string]interface{}{
			"id":         proxy.config.ID,
			"name":       proxy.config.Name,
			"agentId":    proxy.config.AgentID,
			"remotePort": proxy.config.RemotePort,
			"target":     proxy.config.Target,
			"status":     "running",
			"connCount":  len(conns),
		})
		proxy.mu.RUnlock()
	}

	return status
}

// cleanupLoop 清理过期连接
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanupIdleConnections()
		case <-m.ctx.Done():
			return
		}
	}
}

// cleanupIdleConnections 清理空闲连接
func (m *Manager) cleanupIdleConnections() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	for _, proxy := range m.proxies {
		proxy.mu.Lock()
		for connID, conn := range proxy.conns {
			// 清理超过 5 分钟没有读活动的连接
			if now.Sub(conn.LastRead) > 5*time.Minute {
				log.Printf("Closing idle connection %s", connID)
				conn.Close()
				delete(proxy.conns, connID)
			}
		}
		proxy.mu.Unlock()
	}
}

// Stop 停止代理
func (p *Proxy) Stop() {
	p.cancel()

	if p.listener != nil {
		p.listener.Close()
	}

	// 关闭所有连接
	p.mu.Lock()
	for _, conn := range p.conns {
		conn.Close()
	}
	p.conns = make(map[string]*ProxyConn)
	p.mu.Unlock()
}

// Close 关闭代理连接
func (pc *ProxyConn) Close() error {
	if !pc.closed.CompareAndSwap(false, true) {
		return nil // 已经关闭
	}
	return pc.Conn.Close()
}

// SendToAgent 实现 ServerInterface
func (m *Manager) SendToAgent(agentID string, msg *protocol.Message) error {
	return m.server.SendToAgent(agentID, msg)
}
