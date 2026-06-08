package alert

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/storage"
)

func testDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(storage.Config{Path: dir + "/test.db"})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestNewGenerator(t *testing.T) {
	g := NewGenerator(nil, nil, nil)
	if g == nil {
		t.Fatal("NewGenerator() should not return nil")
	}
}

func TestNewGeneratorWithDB(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)
	if g == nil {
		t.Fatal("NewGenerator() should not return nil")
	}
	if g.db != db {
		t.Error("Generator.db should match input")
	}
}

func TestCheckAllChecksEmptyDB(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)
	g.CheckAllChecks()
}

func TestCheckExpiringCertificates7Days(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	cert := &storage.Certificate{
		DomainName: "example.com",
		Status:     "valid",
		ExpiresAt:  time.Now().Add(5 * 24 * time.Hour),
	}
	if err := db.UpsertCertificate(cert); err != nil {
		t.Fatalf("UpsertCertificate: %v", err)
	}

	g.CheckExpiringCertificates()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) == 0 {
		t.Error("Expected alert for certificate expiring in 5 days")
	}
	if alerts[0].Type != "error" {
		t.Errorf("Alert type = %v, want error", alerts[0].Type)
	}
}

func TestCheckExpiringCertificates30Days(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	cert := &storage.Certificate{
		DomainName: "example20.com",
		Status:     "valid",
		ExpiresAt:  time.Now().Add(20 * 24 * time.Hour),
	}
	if err := db.UpsertCertificate(cert); err != nil {
		t.Fatalf("UpsertCertificate: %v", err)
	}

	g.CheckExpiringCertificates()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) == 0 {
		t.Error("Expected alert for certificate expiring in 20 days")
	}
	if alerts[0].Type != "warning" {
		t.Errorf("Alert type = %v, want warning", alerts[0].Type)
	}
}

func TestCheckExpiringCertificatesExpired(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	cert := &storage.Certificate{
		DomainName: "expired.com",
		Status:     "valid",
		ExpiresAt:  time.Now().Add(-1 * time.Hour),
	}
	if err := db.UpsertCertificate(cert); err != nil {
		t.Fatalf("UpsertCertificate: %v", err)
	}

	g.CheckExpiringCertificates()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) == 0 {
		t.Error("Expected alert for expired certificate")
	}
}

func TestCheckExpiringCertificatesSendsNotification(t *testing.T) {
	db := testDB(t)

	received := make(chan map[string]interface{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/events" {
			t.Errorf("expected path /api/v1/events, got %s", r.URL.Path)
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request body: %v", err)
		}

		select {
		case received <- payload:
		default:
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	notifCfg := &config.NotificationConfig{
		Enabled: true,
		Herald: &config.HeraldConfig{
			BaseURL: server.URL,
			Timeout: time.Second,
		},
		Events: map[string]*config.EventConfig{
			"certificate_expired": {
				Type:    notification.CertificateExpired,
				Enabled: true,
			},
		},
	}
	g := NewGenerator(db, notification.NewClient(notifCfg), notifCfg)

	cert := &storage.Certificate{
		DomainName: "expired.example.com",
		Status:     "valid",
		ExpiresAt:  time.Now().Add(-time.Hour),
	}
	if err := db.UpsertCertificate(cert); err != nil {
		t.Fatalf("UpsertCertificate: %v", err)
	}

	g.CheckExpiringCertificates()

	select {
	case payload := <-received:
		if payload["type"] != notification.CertificateExpired {
			t.Fatalf("event type = %v, want %s", payload["type"], notification.CertificateExpired)
		}

		labels, ok := payload["labels"].(map[string]interface{})
		if !ok {
			t.Fatal("labels should be an object")
		}
		if labels["resource_type"] != "certificate" {
			t.Fatalf("resource_type = %v, want certificate", labels["resource_type"])
		}
		if labels["title"] != "证书已过期" {
			t.Fatalf("title = %v, want 证书已过期", labels["title"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification event")
	}
}

func TestCheckExpiringCertificatesNotExpiring(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	cert := &storage.Certificate{
		DomainName: "safe.com",
		Status:     "valid",
		ExpiresAt:  time.Now().Add(90 * 24 * time.Hour),
	}
	if err := db.UpsertCertificate(cert); err != nil {
		t.Fatalf("UpsertCertificate: %v", err)
	}

	g.CheckExpiringCertificates()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("Expected 0 alerts, got %d", len(alerts))
	}
}

func TestCheckExpiringCertificatesNonValid(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	cert := &storage.Certificate{
		DomainName: "revoked.com",
		Status:     "revoked",
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}
	if err := db.UpsertCertificate(cert); err != nil {
		t.Fatalf("UpsertCertificate: %v", err)
	}

	g.CheckExpiringCertificates()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("Expected 0 alerts for non-valid cert, got %d", len(alerts))
	}
}

func TestCheckDownServices(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	svc := &storage.Service{
		Name:   "nginx",
		Type:   "web",
		Status: "down",
	}
	if err := db.UpsertService(svc); err != nil {
		t.Fatalf("UpsertService: %v", err)
	}

	g.CheckDownServices()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) == 0 {
		t.Error("Expected alert for down service")
	}
	if alerts[0].Type != "error" {
		t.Errorf("Alert type = %v, want error", alerts[0].Type)
	}
}

func TestCheckDownServicesRunning(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	svc := &storage.Service{
		Name:   "nginx",
		Type:   "web",
		Status: "running",
	}
	if err := db.UpsertService(svc); err != nil {
		t.Fatalf("UpsertService: %v", err)
	}

	g.CheckDownServices()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("Expected 0 alerts for running service, got %d", len(alerts))
	}
}

func TestCheckOfflineAgents(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	agent := &storage.Agent{
		Hostname: "test-host",
		IP:       "192.168.1.1",
		Status:   "offline",
	}
	if err := db.UpsertAgent(agent); err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}

	g.CheckOfflineAgents()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) == 0 {
		t.Error("Expected alert for offline agent")
	}
	if alerts[0].Type != "warning" {
		t.Errorf("Alert type = %v, want warning", alerts[0].Type)
	}
}

func TestCheckOfflineAgentsOnline(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	agent := &storage.Agent{
		Hostname: "online-host",
		IP:       "192.168.1.2",
		Status:   "online",
	}
	if err := db.UpsertAgent(agent); err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}

	g.CheckOfflineAgents()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("Expected 0 alerts for online agent, got %d", len(alerts))
	}
}

func TestCheckExpiredDomains(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	domain := &storage.Domain{
		Domain: "expired-domain.com",
		Status: "expired",
	}
	if err := db.UpsertDomain(domain); err != nil {
		t.Fatalf("UpsertDomain: %v", err)
	}

	g.CheckExpiredDomains()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) == 0 {
		t.Error("Expected alert for expired domain")
	}
	if alerts[0].Type != "error" {
		t.Errorf("Alert type = %v, want error", alerts[0].Type)
	}
}

func TestCheckExpiredDomainsActive(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	domain := &storage.Domain{
		Domain: "active-domain.com",
		Status: "active",
	}
	if err := db.UpsertDomain(domain); err != nil {
		t.Fatalf("UpsertDomain: %v", err)
	}

	g.CheckExpiredDomains()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("Expected 0 alerts for active domain, got %d", len(alerts))
	}
}

func TestCheckDiskSpaceNoPanic(t *testing.T) {
	g := NewGenerator(nil, nil, nil)
	g.CheckDiskSpace(80)
	g.CheckDiskSpace(0)
	g.CheckDiskSpace(100)
}

func TestCheckMemoryUsageNoPanic(t *testing.T) {
	g := NewGenerator(nil, nil, nil)
	g.CheckMemoryUsage(80)
	g.CheckMemoryUsage(0)
	g.CheckMemoryUsage(100)
}

func TestCleanupOldAlerts(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	oldAlert := &storage.Alert{
		Type:    "info",
		Title:   "Old alert",
		Message: "This is old",
		Read:    true,
	}
	if err := db.CreateAlert(oldAlert); err != nil {
		t.Fatalf("CreateAlert: %v", err)
	}

	g.CleanupOldAlerts(1 * time.Nanosecond)
	time.Sleep(100 * time.Millisecond)
}

func TestMultipleChecksIdempotent(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	svc := &storage.Service{Name: "api-server", Type: "api", Status: "down"}
	db.UpsertService(svc)

	g.CheckDownServices()
	g.CheckDownServices()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) < 1 {
		t.Error("Expected at least 1 alert")
	}
}

func TestCheckAllChecksMixed(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	svc := &storage.Service{Name: "web", Type: "http", Status: "down"}
	db.UpsertService(svc)

	agent := &storage.Agent{Hostname: "host1", IP: "10.0.0.1", Status: "offline"}
	db.UpsertAgent(agent)

	cert := &storage.Certificate{
		DomainName: "test.com",
		Status:     "valid",
		ExpiresAt:  time.Now().Add(3 * 24 * time.Hour),
	}
	db.UpsertCertificate(cert)

	domain := &storage.Domain{Domain: "test.com", Status: "expired"}
	db.UpsertDomain(domain)

	g.CheckAllChecks()

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) < 4 {
		t.Errorf("Expected at least 4 alerts, got %d", len(alerts))
	}
}
