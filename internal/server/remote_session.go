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

func (m *RemoteSessionManager) Create(userID, username string, req RemoteSessionRequest) *RemoteSession {
	now := time.Now()
	session := &RemoteSession{
		ID:        uuid.New().String(),
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
	m.sessions[session.ID] = session
	m.mu.Unlock()

	return session
}

func (m *RemoteSessionManager) List() []*RemoteSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*RemoteSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		result = append(result, session)
	}
	return result
}

func (m *RemoteSessionManager) Get(id string) (*RemoteSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[id]
	return session, ok
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
