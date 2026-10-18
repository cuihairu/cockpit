package server

import (
	"net"
	"strings"
	"sync"
	"time"
)

const (
	loginAttemptWindow   = 15 * time.Minute
	loginAttemptLock     = 5 * time.Minute
	loginAttemptsPerIP   = 20
	loginAttemptsPerUser = 8
	loginLimiterMaxKeys  = 20000
)

type loginAttemptBucket struct {
	failures    int
	windowUntil time.Time
	blockedTill time.Time
	lastSeen    time.Time
}

// loginAttemptLimiter is process-local by design: it slows online guessing on
// one server instance, but does not replace a shared limiter in multi-replica
// deployments or edge-proxy controls.
type loginAttemptLimiter struct {
	mu      sync.Mutex
	buckets map[string]loginAttemptBucket
}

func newLoginAttemptLimiter() *loginAttemptLimiter {
	return &loginAttemptLimiter{buckets: make(map[string]loginAttemptBucket)}
}

// remoteAddrIP deliberately ignores X-Forwarded-For/X-Real-IP: those headers
// are client-controlled unless a trusted-proxy boundary is configured.
func remoteAddrIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(remoteAddr); ip != nil {
		return ip.String()
	}
	if strings.TrimSpace(remoteAddr) == "" {
		return "unknown"
	}
	return strings.TrimSpace(remoteAddr)
}

func loginUserKey(username string) string {
	return "user:" + strings.ToLower(strings.TrimSpace(username))
}

func loginIPKey(ip string) string {
	return "ip:" + strings.TrimSpace(ip)
}

func (l *loginAttemptLimiter) blocked(ip, username string, now time.Time) time.Duration {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked(now)

	var retry time.Duration
	for _, key := range loginKeys(ip, username) {
		bucket, ok := l.buckets[key]
		if !ok {
			continue
		}
		if now.Before(bucket.blockedTill) && bucket.blockedTill.Sub(now) > retry {
			retry = bucket.blockedTill.Sub(now)
		}
	}
	return retry
}

func (l *loginAttemptLimiter) failed(ip, username string, now time.Time) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked(now)
	keys := loginKeys(ip, username)
	for _, key := range keys {
		bucket := l.buckets[key]
		if bucket.windowUntil.IsZero() || !now.Before(bucket.windowUntil) {
			bucket = loginAttemptBucket{windowUntil: now.Add(loginAttemptWindow)}
		}
		bucket.failures++
		bucket.lastSeen = now
		threshold := loginAttemptsPerIP
		if strings.HasPrefix(key, "user:") {
			threshold = loginAttemptsPerUser
		}
		if bucket.failures >= threshold {
			bucket.blockedTill = now.Add(loginAttemptLock)
		}
		l.buckets[key] = bucket
	}
	l.enforceLimitLocked()
}

func (l *loginAttemptLimiter) succeeded(ip, username string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range loginKeys(ip, username) {
		delete(l.buckets, key)
	}
}

func loginKeys(ip, username string) []string {
	keys := []string{loginIPKey(ip)}
	if user := strings.TrimSpace(username); user != "" {
		keys = append(keys, loginUserKey(user))
	}
	return keys
}

func (l *loginAttemptLimiter) cleanupLocked(now time.Time) {
	for key, bucket := range l.buckets {
		if !now.Before(bucket.windowUntil) && !now.Before(bucket.blockedTill) {
			delete(l.buckets, key)
		}
	}
}

func (l *loginAttemptLimiter) enforceLimitLocked() {
	for len(l.buckets) > loginLimiterMaxKeys {
		var oldestKey string
		var oldest time.Time
		for key, bucket := range l.buckets {
			if oldestKey == "" || bucket.lastSeen.Before(oldest) {
				oldestKey, oldest = key, bucket.lastSeen
			}
		}
		delete(l.buckets, oldestKey)
	}
}
