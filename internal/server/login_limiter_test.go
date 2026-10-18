package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRemoteAddrIP(t *testing.T) {
	tests := []struct {
		remote string
		want   string
	}{
		{"192.0.2.1:1234", "192.0.2.1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"198.51.100.4", "198.51.100.4"},
		{"not-an-address", "not-an-address"},
		{"  ", "unknown"},
	}
	for _, tt := range tests {
		if got := remoteAddrIP(tt.remote); got != tt.want {
			t.Errorf("remoteAddrIP(%q) = %q, want %q", tt.remote, got, tt.want)
		}
	}
}

func TestLoginLimiterAccountAndIPBuckets(t *testing.T) {
	limiter := newLoginAttemptLimiter()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	if got := limiter.blocked("192.0.2.1", " Alice ", now); got != 0 {
		t.Fatalf("initial blocked duration = %v, want 0", got)
	}
	for i := 0; i < loginAttemptsPerUser; i++ {
		limiter.failed("192.0.2.1", "Alice", now)
	}
	if got := limiter.blocked("198.51.100.2", "alice", now); got != loginAttemptLock {
		t.Fatalf("account block duration = %v, want %v", got, loginAttemptLock)
	}
	limiter.succeeded("192.0.2.1", "ALICE")
	if got := limiter.blocked("198.51.100.2", "alice", now); got != 0 {
		t.Fatalf("successful login did not clear account bucket: %v", got)
	}
	if got := limiter.blocked("192.0.2.1", "someone-else", now); got != 0 {
		t.Fatalf("successful login did not clear source IP bucket: %v", got)
	}
}

func TestLoginLimiterIPBucketAndWindowExpiry(t *testing.T) {
	limiter := newLoginAttemptLimiter()
	now := time.Now()
	for i := 0; i < loginAttemptsPerIP; i++ {
		limiter.failed("192.0.2.9", "user-"+string(rune('a'+i)), now)
	}
	if got := limiter.blocked("192.0.2.9", "another-user", now); got != loginAttemptLock {
		t.Fatalf("IP block duration = %v, want %v", got, loginAttemptLock)
	}
	if got := limiter.blocked("192.0.2.9", "another-user", now.Add(loginAttemptWindow+time.Second)); got != 0 {
		t.Fatalf("expired IP bucket still blocks: %v", got)
	}
	if len(limiter.buckets) != 0 {
		t.Fatalf("expired buckets remain: %d", len(limiter.buckets))
	}
}

func TestLoginLimiterBoundsMemoryAndNilReceiver(t *testing.T) {
	var nilLimiter *loginAttemptLimiter
	if nilLimiter.blocked("ip", "user", time.Now()) != 0 {
		t.Fatal("nil limiter must not block")
	}
	nilLimiter.failed("ip", "user", time.Now())
	nilLimiter.succeeded("ip", "user")

	limiter := newLoginAttemptLimiter()
	now := time.Now()
	for i := 0; i < loginLimiterMaxKeys; i++ {
		limiter.buckets["old:"+string(rune(i+1))] = loginAttemptBucket{
			windowUntil: now.Add(time.Hour), lastSeen: now,
		}
	}
	limiter.failed("192.0.2.10", "new-user", now)
	if len(limiter.buckets) > loginLimiterMaxKeys {
		t.Fatalf("bucket cap exceeded: %d", len(limiter.buckets))
	}
}

func TestHandleLoginAttemptLimitUsesSocketIPAndAudits(t *testing.T) {
	s := newTestServerWithDB(t)
	if err := s.db.InitAdminUser("alice", "password123"); err != nil {
		t.Fatal(err)
	}

	login := func() *httptest.ResponseRecorder {
		body := strings.NewReader(`{"username":"alice","password":"wrong"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
		req.RemoteAddr = "192.0.2.44:9876"
		req.Header.Set("X-Forwarded-For", "203.0.113.7") // must not bypass IP bucket
		rec := httptest.NewRecorder()
		s.handleLoginWithAudit(rec, req)
		return rec
	}
	for i := 0; i < loginAttemptsPerUser; i++ {
		if rec := login(); rec.Code != http.StatusUnauthorized {
			t.Fatalf("failed attempt %d status = %d, want 401", i+1, rec.Code)
		}
	}
	rec := login()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("limited status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 response missing Retry-After")
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
		t.Fatalf("invalid 429 response: %s (%v)", rec.Body.String(), err)
	}
}
