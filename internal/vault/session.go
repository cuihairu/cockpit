package vault

import (
	"sync"
	"time"
)

// Session provider 级解锁会话（V3）：TTL 到期自动上锁；TTL<=0 表示进程生命周期。
// 会话口径只在此处定义，各实现共用，不散落。
type Session struct {
	mu       sync.Mutex
	unlocked bool
	deadline time.Time
	ttl      time.Duration
	nowFn    func() time.Time // 测试注入
}

// NewSession 创建会话；ttl<=0 = 进程生命周期（自托管无人值守重启默认）。
func NewSession(ttl time.Duration) *Session {
	return &Session{ttl: ttl, nowFn: time.Now}
}

// Unlock 标记解锁并刷新时限。已解锁时幂等（重验由 provider 层完成）。
func (s *Session) Unlock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unlocked = true
	if s.ttl > 0 {
		s.deadline = s.nowFn().Add(s.ttl)
	}
}

// Lock 立即上锁。
func (s *Session) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unlocked = false
}

// Check 会话有效返回 nil；未解锁或 TTL 到期（自动上锁）返回 ErrLocked。
func (s *Session) Check() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.unlocked {
		return ErrLocked
	}
	if s.ttl > 0 && s.nowFn().After(s.deadline) {
		s.unlocked = false
		return ErrLocked
	}
	return nil
}
