package server

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"sync"
	"time"
)

// Ticket 一次性短期 WebSocket 连接凭证
type Ticket struct {
	ID         string
	UserID     string
	Username   string
	Params     map[string]string // 连接参数 (agent_id, host, port, etc.)
	ExpiresAt  time.Time
	Consumed   bool
	mu         sync.RWMutex
}

// TicketManager 票据管理器
type TicketManager struct {
	tickets map[string]*Ticket
	mu      sync.RWMutex
}

// NewTicketManager 创建票据管理器
func NewTicketManager() *TicketManager {
	tm := &TicketManager{
		tickets: make(map[string]*Ticket),
	}
	go tm.cleanupLoop()
	return tm
}

// GenerateTicket 生成新票据（有效期5分钟）
func (tm *TicketManager) GenerateTicket(userID, username string, params map[string]string) (*Ticket, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 生成随机票据ID（16字节）
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	ticketID := hex.EncodeToString(b)

	ticket := &Ticket{
		ID:        ticketID,
		UserID:    userID,
		Username:  username,
		Params:    params,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Consumed:  false,
	}

	tm.tickets[ticketID] = ticket
	log.Printf("Ticket generated: %s for user %s", ticketID, username)

	return ticket, nil
}

// ValidateTicket 验证并消费票据（单次使用）
func (tm *TicketManager) ValidateTicket(ticketID string) (*Ticket, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	ticket, exists := tm.tickets[ticketID]
	if !exists {
		return nil, false
	}

	ticket.mu.Lock()
	defer ticket.mu.Unlock()

	// 检查是否已消费
	if ticket.Consumed {
		return nil, false
	}

	// 检查是否过期
	if time.Now().After(ticket.ExpiresAt) {
		delete(tm.tickets, ticketID)
		return nil, false
	}

	// 标记为已消费
	ticket.Consumed = true

	return ticket, true
}

// cleanupLoop 定期清理过期票据
func (tm *TicketManager) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		tm.mu.Lock()
		now := time.Now()
		for id, ticket := range tm.tickets {
			if now.After(ticket.ExpiresAt) {
				delete(tm.tickets, id)
			}
		}
		tm.mu.Unlock()
	}
}
