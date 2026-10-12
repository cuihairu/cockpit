package server

import (
	"errors"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/google/uuid"
)

type RemoteSessionStatus string

const (
	RemoteSessionStatusPending   RemoteSessionStatus = "pending"
	RemoteSessionStatusConnected RemoteSessionStatus = "connected"
	RemoteSessionStatusClosed    RemoteSessionStatus = "closed"
	RemoteSessionStatusFailed    RemoteSessionStatus = "failed"
)

type RemoteSession struct {
	ID        string                  `json:"id"`
	UserID    string                  `json:"userId"`
	Username  string                  `json:"username"`
	AgentID   string                  `json:"agentId"`
	Protocol  protocol.RemoteProtocol `json:"protocol"`
	Host      string                  `json:"host"`
	Port      int                     `json:"port"`
	Status    RemoteSessionStatus     `json:"status"`
	Error     string                  `json:"error,omitempty"`
	CreatedAt time.Time               `json:"createdAt"`
	UpdatedAt time.Time               `json:"updatedAt"`
	ClosedAt  *time.Time              `json:"closedAt,omitempty"`
}

type RemoteSessionRequest struct {
	AgentID  string                  `json:"agentId"`
	Protocol protocol.RemoteProtocol `json:"protocol"`
	Host     string                  `json:"host"`
	Port     int                     `json:"port"`
}

type RemoteSessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*RemoteSession
}

func NewRemoteSessionManager() *RemoteSessionManager {
	return &RemoteSessionManager{
		sessions: make(map[string]*RemoteSession),
	}
}

// Create 登记会话并自分配 UUID（REST 登记入口用）。
func (m *RemoteSessionManager) Create(userID, username string, req RemoteSessionRequest) *RemoteSession {
	return m.Register("", userID, username, req)
}

// Register 登记一个活会话。id 为空则生成 UUID（Create 语义）；terminal /
// guacamole 链路传入自身会话 ID，使台账与数据面索引同源（审计 Session 字段
// 与控制面 DELETE 命中同一 ID，D-2026-10-12-4）。
func (m *RemoteSessionManager) Register(id, userID, username string, req RemoteSessionRequest) *RemoteSession {
	if id == "" {
		id = uuid.New().String()
	}
	now := time.Now()
	session := &RemoteSession{
		ID:        id,
		UserID:    userID,
		Username:  username,
		AgentID:   req.AgentID,
		Protocol:  req.Protocol,
		Host:      req.Host,
		Port:      req.Port,
		Status:    RemoteSessionStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	return session
}

// List / Get 返回台账快照（浅拷贝）：WS 数据面 goroutine 会并发 UpdateStatus
// 写同一会话字段，读侧若直透存储指针，序列化/断言读到写一半的状态（-race 实锤）。
// ClosedAt 是指针替换非穿透写，浅拷贝即一致快照。

func (m *RemoteSessionManager) List() []*RemoteSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*RemoteSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		cp := *session
		result = append(result, &cp)
	}
	return result
}

func (m *RemoteSessionManager) Get(id string) (*RemoteSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	cp := *session
	return &cp, true
}

func (m *RemoteSessionManager) UpdateStatus(id string, status RemoteSessionStatus, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return errors.New("remote session not found")
	}

	now := time.Now()
	session.Status = status
	session.UpdatedAt = now
	session.Error = errMsg
	if status == RemoteSessionStatusClosed || status == RemoteSessionStatusFailed {
		session.ClosedAt = &now
	}
	return nil
}

func (m *RemoteSessionManager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[id]; !ok {
		return false
	}
	delete(m.sessions, id)
	return true
}
