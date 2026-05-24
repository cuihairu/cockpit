package remote

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
)

// Manager 远程连接管理器
type Manager struct {
	connections map[string]*Connection // connID -> Connection
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// Connection 远程连接会话
type Connection struct {
	ID         string
	Protocol   protocol.RemoteProtocol
	Target     string // host:port
	ClientConn net.Conn
	TargetConn net.Conn
	CreatedAt  time.Time
	LastActive time.Time
	mu         sync.RWMutex
	closed     atomic.Bool
}

// NewManager 创建远程连接管理器
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		connections: make(map[string]*Connection),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start 启动连接管理器
func (m *Manager) Start() error {
	// 启动清理协程
	go m.cleanupLoop()
	return nil
}

// Stop 停止连接管理器
func (m *Manager) Stop() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, conn := range m.connections {
		conn.Close()
	}
	m.connections = make(map[string]*Connection)
}

// CreateConnection 创建新连接
func (m *Manager) CreateConnection(connID string, protocol protocol.RemoteProtocol, target string, timeout time.Duration) (*Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.connections[connID]; exists {
		return nil, fmt.Errorf("connection %s already exists", connID)
	}

	// 连接到目标服务
	targetConn, err := net.DialTimeout("tcp", target, timeout)
	if err != nil {
		return nil, fmt.Errorf("connect to target %s failed: %w", target, err)
	}

	conn := &Connection{
		ID:         connID,
		Protocol:   protocol,
		Target:     target,
		TargetConn: targetConn,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	m.connections[connID] = conn

	log.Printf("Remote connection created: %s -> %s (%s)", connID, target, protocol)

	return conn, nil
}

// GetConnection 获取连接
func (m *Manager) GetConnection(connID string) (*Connection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[connID]
	return conn, exists
}

// CloseConnection 关闭连接
func (m *Manager) CloseConnection(connID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[connID]
	if !exists {
		return fmt.Errorf("connection %s not found", connID)
	}

	conn.Close()
	delete(m.connections, connID)

	log.Printf("Remote connection closed: %s", connID)

	return nil
}

// SetClientConn 设置客户端连接
func (c *Connection) SetClientConn(conn net.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ClientConn = conn
	c.LastActive = time.Now()
}

// HandleData 处理数据转发
func (c *Connection) HandleData(data []byte, fromClient bool) error {
	c.mu.Lock()
	c.LastActive = time.Now()
	c.mu.Unlock()

	var conn net.Conn
	if fromClient {
		conn = c.TargetConn
	} else {
		conn = c.ClientConn
	}

	if conn == nil {
		return fmt.Errorf("connection not ready")
	}

	_, err := conn.Write(data)
	if err != nil {
		c.Close()
		return fmt.Errorf("write failed: %w", err)
	}

	return nil
}

// StartForwarding 开始数据转发（双向）
func (c *Connection) StartForwarding() {
	// 客户端 -> 目标
	go c.forward(c.ClientConn, c.TargetConn, true)

	// 目标 -> 客户端
	go c.forward(c.TargetConn, c.ClientConn, false)
}

// forward 单向数据转发
func (c *Connection) forward(src, dst net.Conn, toTarget bool) {
	defer c.Close()

	buf := make([]byte, 32*1024)

	for {
		n, err := src.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Forward error on %s: %v", c.ID, err)
			}
			break
		}

		c.mu.Lock()
		c.LastActive = time.Now()
		c.mu.Unlock()

		_, err = dst.Write(buf[:n])
		if err != nil {
			log.Printf("Write error on %s: %v", c.ID, err)
			break
		}
	}
}

// Close 关闭连接
func (c *Connection) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil // 已关闭
	}

	if c.ClientConn != nil {
		c.ClientConn.Close()
	}
	if c.TargetConn != nil {
		c.TargetConn.Close()
	}

	return nil
}

// cleanupLoop 清理空闲连接
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanupIdle()
		case <-m.ctx.Done():
			return
		}
	}
}

// cleanupIdle 清理空闲超过 5 分钟的连接
func (m *Manager) cleanupIdle() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, conn := range m.connections {
		if now.Sub(conn.LastActive) > 5*time.Minute {
			log.Printf("Closing idle connection: %s", id)
			conn.Close()
			delete(m.connections, id)
		}
	}
}

// GetStats 获取统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]interface{}{
		"totalConnections": len(m.connections),
	}

	byProtocol := make(map[string]int)
	for _, conn := range m.connections {
		byProtocol[string(conn.Protocol)]++
	}
	stats["byProtocol"] = byProtocol

	return stats
}
