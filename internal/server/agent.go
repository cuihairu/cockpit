package server

import (
	"fmt"
	"sync"
	"sync/atomic"
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
	closed         atomic.Bool
	// sendMu 保护 Send 通道的 close 与入队互斥，消除并发发送的
	// send-on-closed-channel panic（旧实现靠 recover 兜底，仍算数据竞争）
	sendMu   sync.Mutex
	LastSeen time.Time
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
	a.sendMu.Lock()
	defer a.sendMu.Unlock()

	if a.closed.CompareAndSwap(false, true) {
		close(a.Send)
	}

	if a.Conn != nil {
		a.Conn.Close()
	}
}

// AgentID 实现 proxy.AgentConn 接口
func (a *Agent) AgentID() string {
	return a.ID
}

// trySendLocked 尝试入队一条消息（须持有 sendMu）。
// 返回是否入队成功、通道是否已关闭。
func (a *Agent) trySendLocked(msg *protocol.Message) (queued, closed bool) {
	if a.closed.Load() {
		return false, true
	}
	select {
	case a.Send <- msg:
		return true, false
	default:
		return false, false
	}
}

//SendMessage 发送消息给 Agent
func (a *Agent) SendMessage(msg *protocol.Message) error {
	a.sendMu.Lock()
	defer a.sendMu.Unlock()

	queued, closed := a.trySendLocked(msg)
	switch {
	case queued:
		return nil
	case closed:
		return fmt.Errorf("agent %s is closed", a.ID)
	default:
		return fmt.Errorf("agent %s send channel full", a.ID)
	}
}

// SendWithTimeout 在 timeout 内持续尝试入队（通道满时短间隔重试）。
// 与 Close 并发安全；用于 RPC 请求等允许等待的发送方。
func (a *Agent) SendWithTimeout(msg *protocol.Message, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		a.sendMu.Lock()
		queued, closed := a.trySendLocked(msg)
		a.sendMu.Unlock()

		if queued {
			return nil
		}
		if closed {
			return fmt.Errorf("agent %s is closed", a.ID)
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("agent %s send timeout", a.ID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
