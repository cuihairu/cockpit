package probe

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/storage"
)

// newTestRunner creates a Runner backed by an isolated test database
func newTestRunner(t *testing.T) (*Runner, *storage.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := storage.Open(storage.Config{Path: path})
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	// interval 0 lets NewRunner apply its default
	return NewRunner(db, 0, nil), db
}

func TestNewRunnerDefaultsInterval(t *testing.T) {
	r, _ := newTestRunner(t)
	if r.Interval() != 5*time.Minute {
		t.Errorf("interval = %v, want 5m", r.Interval())
	}
}

func TestNewRunnerKeepsPositiveInterval(t *testing.T) {
	r, _ := newTestRunner(t)
	r2 := NewRunner(r.db, 30*time.Second, nil)
	if r2.Interval() != 30*time.Second {
		t.Errorf("interval = %v, want 30s", r2.Interval())
	}
	if r2.healthChecker == nil {
		t.Error("healthChecker should be initialized")
	}
	if r2.certMonitor == nil {
		t.Error("certMonitor should be initialized")
	}
}

func TestSetIntervalClampsToBounds(t *testing.T) {
	r, _ := newTestRunner(t)

	r.SetInterval(0)
	r.SetInterval(-5 * time.Second)
	if got := r.Interval(); got != 5*time.Minute {
		t.Errorf("non-positive SetInterval should be ignored, got %v", got)
	}

	r.SetInterval(5 * time.Second) // 低于下限 → 夹紧到 30s
	if got := r.Interval(); got != time.Duration(probeMinIntervalForTest)*time.Second {
		t.Errorf("interval = %v, want clamped %ds", got, probeMinIntervalForTest)
	}

	r.SetInterval(48 * time.Hour) // 高于上限 → 夹紧到 1h
	if got := r.Interval(); got != time.Duration(probeMaxIntervalForTest)*time.Second {
		t.Errorf("interval = %v, want clapped %ds", got, probeMaxIntervalForTest)
	}

	r.SetInterval(2 * time.Minute)
	if got := r.Interval(); got != 2*time.Minute {
		t.Errorf("interval = %v, want 2m", got)
	}
}

// probeMinIntervalForTest / probeMaxIntervalForTest 复述包级常量，便于错误信息可读
const (
	probeMinIntervalForTest = MinIntervalSeconds
	probeMaxIntervalForTest = MaxIntervalSeconds
)

// newEdgeTestRunner 带指向本地 httptest 的 webhook 通知服务的 Runner
func newEdgeTestRunner(t *testing.T, events map[string]*config.EventConfig) (*Runner, *storage.DB, chan []byte) {
	t.Helper()
	received := make(chan []byte, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		select {
		case received <- body:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := &config.NotificationConfig{
		Enabled: true,
		Webhook: []*config.WebhookConfig{{URL: srv.URL}},
		Events:  events,
	}
	r, db := newTestRunner(t)
	r.notifier = notification.NewService(cfg)
	return r, db, received
}

func TestTrackServiceEdgeNotifications(t *testing.T) {
	r, _, received := newEdgeTestRunner(t, map[string]*config.EventConfig{
		"down": {Type: notification.ServiceDown, Enabled: true},
		"up":   {Type: notification.ServiceUp, Enabled: true},
	})

	down := ProbeResult{ResourceType: "service", ResourceID: "svc-1", Name: "web", Status: "down", Error: "conn refused", Message: "conn refused"}
	up := ProbeResult{ResourceType: "service", ResourceID: "svc-1", Name: "web", Status: "up", Message: "HTTP 200"}

	// 第 1 次失败：未达阈值，不通知
	r.trackServiceEdge(down)
	select {
	case b := <-received:
		t.Fatalf("unexpected notification after 1st failure: %s", b)
	case <-time.After(300 * time.Millisecond):
	}

	// 第 2 次失败：达到阈值 → service.down
	r.trackServiceEdge(down)
	expectEvent(t, received, notification.ServiceDown)

	// 第 3 次失败：已通知过，不重复
	r.trackServiceEdge(down)
	select {
	case b := <-received:
		t.Fatalf("duplicate down notification: %s", b)
	case <-time.After(300 * time.Millisecond):
	}

	// 恢复：补发 service.up
	r.trackServiceEdge(up)
	expectEvent(t, received, notification.ServiceUp)

	// 持续正常：不再发
	r.trackServiceEdge(up)
	select {
	case b := <-received:
		t.Fatalf("unexpected up notification: %s", b)
	case <-time.After(300 * time.Millisecond):
	}
}

// expectEvent 等待并断言下一个 webhook 通知的 event_type
func expectEvent(t *testing.T, received chan []byte, eventType string) {
	t.Helper()
	select {
	case b := <-received:
		var payload map[string]interface{}
		if err := json.Unmarshal(b, &payload); err != nil {
			t.Fatalf("decode notification: %v (%s)", err, b)
		}
		if payload["event_type"] != eventType {
			t.Fatalf("event_type = %v, want %s", payload["event_type"], eventType)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s notification", eventType)
	}
}

func TestTrackServiceEdgeRespectsWhitelist(t *testing.T) {
	// 白名单未启用 service.down：边沿检测照常计数但不投递
	r, _, received := newEdgeTestRunner(t, map[string]*config.EventConfig{})
	down := ProbeResult{ResourceType: "service", ResourceID: "svc-1", Name: "web", Status: "down", Error: "x", Message: "x"}
	r.trackServiceEdge(down)
	r.trackServiceEdge(down)
	select {
	case b := <-received:
		t.Fatalf("filtered event should not be delivered: %s", b)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestRunnerStartStop(t *testing.T) {
	r, _ := newTestRunner(t)
	r.Start()
	// Stop while the runner is still in its initial 10s wait; must return promptly.
	done := make(chan struct{})
	go func() {
		r.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return in time")
	}
	// Stop must be idempotent
	r.Stop()
}

func TestRunAllChecksEmpty(t *testing.T) {
	r, _ := newTestRunner(t)
	results := r.RunAllChecks()
	if results == nil {
		t.Fatal("RunAllChecks() should not return nil")
	}
	if len(results.Results) != 0 {
		t.Errorf("results length = %d, want 0", len(results.Results))
	}
	if results.Domains != 0 || results.Services != 0 || results.Certificates != 0 {
		t.Errorf("counts should be zero, got %+v", results)
	}
	if results.Healthy != 0 || results.Unhealthy != 0 {
		t.Errorf("healthy/unhealthy should be zero, got %+v", results)
	}
}

func TestRunAllChecksHTTPServices(t *testing.T) {
	r, db := newTestRunner(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	up := &storage.Service{ID: "svc-up", Name: "api", Type: "http", URL: srv.URL, Status: "unknown"}
	down := &storage.Service{ID: "svc-down", Name: "dead", Type: "http", URL: "http://127.0.0.1:1", Status: "unknown"}
	if err := db.UpsertService(up); err != nil {
		t.Fatalf("UpsertService() error = %v", err)
	}
	if err := db.UpsertService(down); err != nil {
		t.Fatalf("UpsertService() error = %v", err)
	}

	results := r.RunAllChecks()
	if results.Services != 2 {
		t.Fatalf("Services = %d, want 2", results.Services)
	}
	// Note: "up" is not counted as healthy by RunAllChecks statistics,
	// but the failed service is counted as unhealthy.
	if results.Unhealthy != 1 {
		t.Errorf("Unhealthy = %d, want 1", results.Unhealthy)
	}

	// Status should be persisted back to storage
	gotUp, err := db.GetService("svc-up")
	if err != nil {
		t.Fatalf("GetService(svc-up) error = %v", err)
	}
	if gotUp.Status != "up" {
		t.Errorf("svc-up status = %q, want up", gotUp.Status)
	}
	gotDown, err := db.GetService("svc-down")
	if err != nil {
		t.Fatalf("GetService(svc-down) error = %v", err)
	}
	if gotDown.Status != "down" {
		t.Errorf("svc-down status = %q, want down", gotDown.Status)
	}
}

func TestRunAllChecksTCPService(t *testing.T) {
	r, db := newTestRunner(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	svc := &storage.Service{ID: "svc-tcp", Name: "tcp-svc", Type: "tcp", URL: "tcp://" + ln.Addr().String(), Status: "unknown"}
	if err := db.UpsertService(svc); err != nil {
		t.Fatalf("UpsertService() error = %v", err)
	}

	results := r.RunAllChecks()
	if results.Services != 1 {
		t.Fatalf("Services = %d, want 1", results.Services)
	}
	if results.Healthy != 0 || results.Unhealthy != 0 {
		t.Errorf("up services are not counted as healthy/unhealthy, got healthy=%d unhealthy=%d", results.Healthy, results.Unhealthy)
	}

	got, err := db.GetService("svc-tcp")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if got.Status != "up" {
		t.Errorf("status = %q, want up", got.Status)
	}
}

func TestProbeServiceTCPFallbackToName(t *testing.T) {
	r, db := newTestRunner(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	// Empty URL: target falls back to service name, which has no port info
	svc := &storage.Service{ID: "svc-name", Name: ln.Addr().String(), Type: "tcp", URL: "", Status: "unknown"}
	if err := db.UpsertService(svc); err != nil {
		t.Fatalf("UpsertService() error = %v", err)
	}

	results := r.RunAllChecks()
	if results.Services != 1 {
		t.Fatalf("Services = %d, want 1", results.Services)
	}

	got, err := db.GetService("svc-name")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if got.Status != "up" {
		t.Errorf("status = %q, want up", got.Status)
	}
}

func TestRunAllChecksDomainDNSFailure(t *testing.T) {
	r, db := newTestRunner(t)

	// .invalid TLD is guaranteed to never resolve
	domain := &storage.Domain{ID: "d-1", Domain: "nonexistent-domain.invalid", Status: "active"}
	if err := db.UpsertDomain(domain); err != nil {
		t.Fatalf("UpsertDomain() error = %v", err)
	}

	results := r.RunAllChecks()
	if results.Domains != 1 {
		t.Fatalf("Domains = %d, want 1", results.Domains)
	}
	if results.Unhealthy != 1 {
		t.Errorf("Unhealthy = %d, want 1", results.Unhealthy)
	}

	got, err := db.GetDomain("d-1")
	if err != nil {
		t.Fatalf("GetDomain() error = %v", err)
	}
	if got.Status != "down" {
		t.Errorf("domain status = %q, want down", got.Status)
	}
}

func TestRunAllChecksDomainResolvable(t *testing.T) {
	r, db := newTestRunner(t)

	// localhost always resolves via /etc/hosts; HTTP(S) on port 80/443 is
	// environment dependent, so accept both reachable and degraded outcomes.
	domain := &storage.Domain{ID: "d-2", Domain: "localhost", Status: "active"}
	if err := db.UpsertDomain(domain); err != nil {
		t.Fatalf("UpsertDomain() error = %v", err)
	}

	results := r.RunAllChecks()
	if results.Domains != 1 {
		t.Fatalf("Domains = %d, want 1", results.Domains)
	}

	var pr *ProbeResult
	for i := range results.Results {
		if results.Results[i].ResourceType == "domain" {
			pr = &results.Results[i]
		}
	}
	if pr == nil {
		t.Fatal("domain probe result missing")
	}
	switch pr.Status {
	case "degraded":
		if pr.Message != "DNS resolves but HTTP failed" {
			t.Errorf("degraded message = %q", pr.Message)
		}
	case "active":
		if pr.LatencyMs < 0 {
			t.Errorf("latency should not be negative, got %d", pr.LatencyMs)
		}
	default:
		t.Errorf("unexpected status %q for resolvable domain", pr.Status)
	}
}

func TestRunAllChecksCertificate(t *testing.T) {
	r, db := newTestRunner(t)

	cert := &storage.Certificate{
		ID:         "cert-1",
		DomainName: "127.0.0.1",
		Status:     "valid",
		ExpiresAt:  time.Now().UTC().Add(90 * 24 * time.Hour),
	}
	if err := db.UpsertCertificate(cert); err != nil {
		t.Fatalf("UpsertCertificate() error = %v", err)
	}

	results := r.RunAllChecks()
	if results.Certificates != 1 {
		t.Fatalf("Certificates = %d, want 1", results.Certificates)
	}

	// Whatever the probe outcome, status must be written back to storage
	got, err := db.GetCertificate("cert-1")
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}
	switch got.Status {
	case "error", "expired", "expiring", "valid":
	default:
		t.Errorf("unexpected certificate status %q", got.Status)
	}
}

func TestParseHostPort(t *testing.T) {
	tests := []struct {
		name   string
		target string
		host   string
		port   string
	}{
		{"host:port", "example.com:8080", "example.com", "8080"},
		{"http prefix with port", "http://example.com:9000", "example.com", "9000"},
		{"https prefix with port", "https://example.com:8443", "example.com", "8443"},
		{"tcp prefix with port", "tcp://example.com:27017", "example.com", "27017"},
		{"http prefix with path", "http://example.com/path", "example.com", "80"},
		{"host only", "example.com", "example.com", "80"},
		{"ipv6", "[::1]:5432", "::1", "5432"},
		{"ipv6 no port", "[::1]", "[::1]", "80"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port := parseHostPort(tt.target)
			if host != tt.host {
				t.Errorf("parseHostPort(%q) host = %q, want %q", tt.target, host, tt.host)
			}
			if port != tt.port {
				t.Errorf("parseHostPort(%q) port = %q, want %q", tt.target, port, tt.port)
			}
		})
	}
}
