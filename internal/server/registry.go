package server

import (
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// Registry 管理 Agent 连接池
type Registry struct {
	agents   map[string]*Agent
	mu       sync.RWMutex
	timeouts struct {
		Heartbeat time.Duration
	}
	pendingResponse map[string]chan *protocol.Message
	responseMu      sync.RWMutex
}

// NewRegistry 创建新的 Registry
func NewRegistry() *Registry {
	r := &Registry{
		agents:          make(map[string]*Agent),
		pendingResponse: make(map[string]chan *protocol.Message),
	}
	r.timeouts.Heartbeat = 60 * time.Second
	return r
}

// Register 注册新 Agent
func (r *Registry) Register(agent *Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[agent.ID]; exists {
		return ErrAgentAlreadyExists
	}

	r.agents[agent.ID] = agent
	return nil
}

// Unregister 注销 Agent
func (r *Registry) Unregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if agent, exists := r.agents[agentID]; exists {
		delete(r.agents, agentID)
		agent.Close()
	}
}

// Get 获取 Agent
func (r *Registry) Get(agentID string) (*Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, exists := r.agents[agentID]
	return agent, exists
}

// List 列出所有 Agent
func (r *Registry) List() []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		agents = append(agents, agent)
	}
	return agents
}

// ListByLocation 按位置列出 Agent
func (r *Registry) ListByLocation(region, zone string) []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*Agent, 0)
	for _, agent := range r.agents {
		loc := agent.GetLocation()
		if (region == "" || loc.Region == region) &&
			(zone == "" || loc.Zone == zone) {
			agents = append(agents, agent)
		}
	}
	return agents
}

// ListByCapability 按能力列出 Agent
func (r *Registry) ListByCapability(capType string) []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*Agent, 0)
	for _, agent := range r.agents {
		if agent.HasCapability(capType) {
			agents = append(agents, agent)
		}
	}
	return agents
}

// UpdateHeartbeat 更新心跳
func (r *Registry) UpdateHeartbeat(agentID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return false
	}
	agent.Heartbeat()
	return true
}

// CleanupOffline 清理离线 Agent
func (r *Registry) CleanupOffline() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var removed []string
	for id, agent := range r.agents {
		if !agent.IsOnline(r.timeouts.Heartbeat) {
			delete(r.agents, id)
			agent.Close()
			removed = append(removed, id)
		}
	}
	return removed
}

// RegisterPendingResponse 注册等待响应的请求
func (r *Registry) RegisterPendingResponse(msgID string, ch chan *protocol.Message) {
	r.responseMu.Lock()
	defer r.responseMu.Unlock()
	r.pendingResponse[msgID] = ch
}

// UnregisterPendingResponse 注销等待响应的请求
func (r *Registry) UnregisterPendingResponse(msgID string) {
	r.responseMu.Lock()
	defer r.responseMu.Unlock()
	delete(r.pendingResponse, msgID)
}

// GetPendingResponse 获取等待响应的通道
func (r *Registry) GetPendingResponse(msgID string) (chan *protocol.Message, bool) {
	r.responseMu.RLock()
	defer r.responseMu.RUnlock()
	ch, exists := r.pendingResponse[msgID]
	return ch, exists
}

// 错误定义
var (
	ErrAgentAlreadyExists = &Error{Code: "agent_exists", Message: "agent already exists"}
	ErrAgentNotFound     = &Error{Code: "agent_not_found", Message: "agent not found"}
)

// Error 自定义错误类型
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}
