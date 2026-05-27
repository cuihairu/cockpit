package server

import (
	"fmt"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// Agent 表示已连接的 Agent
type Agent struct {
	ID             string
	Conn           *websocket.Conn
	Location       protocol.Location
	Capabilities   []protocol.Capability
	Hostname       string
	IP             string
	Virtualization *protocol.VirtualizationInfo
	Labels         map[string]interface{}
	Send           chan *protocol.Message
	mu             sync.RWMutex
	LastSeen       time.Time
}

// NewAgent 创建新的 Agent 实例
func NewAgent(id string, conn *websocket.Conn) *Agent {
	return &Agent{
		ID:       id,
		Conn:     conn,
		Send:     make(chan *protocol.Message, 256),
		LastSeen: time.Now(),
	}
}

// Update 更新 Agent 信息
func (a *Agent) Update(info *protocol.RegisterPayload) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.Location = info.Location
	a.Capabilities = info.Capabilities
	a.Hostname = info.Hostname
	a.IP = info.IP
	a.Virtualization = info.Virtualization
	a.Labels = info.Labels
	a.LastSeen = time.Now()
}

// GetLocation 获取位置信息
func (a *Agent) GetLocation() protocol.Location {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Location
}

// GetCapabilities 获取能力列表
func (a *Agent) GetCapabilities() []protocol.Capability {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Capabilities
}

// HasCapability 检查是否有指定能力
func (a *Agent) HasCapability(capType string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, cap := range a.Capabilities {
		if cap.Type == capType {
			return true
		}
	}
	return false
}

// GetCapability 获取指定能力的详细信息
func (a *Agent) GetCapability(capType string) *protocol.Capability {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, cap := range a.Capabilities {
		if cap.Type == capType {
			return &cap
		}
	}
	return nil
}

// Heartbeat 更新心跳时间
func (a *Agent) Heartbeat() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.LastSeen = time.Now()
}

// IsOnline 检查是否在线（根据心跳）
func (a *Agent) IsOnline(timeout time.Duration) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return time.Since(a.LastSeen) < timeout
}

// Close 关闭连接
func (a *Agent) Close() {
	close(a.Send)
	if a.Conn != nil {
		a.Conn.Close()
	}
}

// AgentID 实现 proxy.AgentConn 接口
func (a *Agent) AgentID() string {
	return a.ID
}

//SendMessage 发送消息给 Agent
func (a *Agent) SendMessage(msg *protocol.Message) error {
	select {
	case a.Send <- msg:
		return nil
	default:
		return fmt.Errorf("agent %s send channel full", a.ID)
	}
}
