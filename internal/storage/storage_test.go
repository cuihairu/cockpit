package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenWithDefaultPath(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	if db == nil {
		t.Error("Open() should not return nil")
	}
}

func TestOpenWithCustomPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if db == nil {
		t.Error("Open() should not return nil")
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file should exist")
	}
}

func TestOpenCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "subdir", "test.db")

	db, err := Open(Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file should exist in created directory")
	}
}

func TestClose(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer os.Remove("cockpit.db")

	err = db.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Should be safe to close again
	err = db.Close()
	if err != nil {
		t.Errorf("Close() again should not error, got %v", err)
	}
}

func TestDBSession(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	session := db.Session()
	if session == nil {
		t.Error("Session() should not return nil")
	}
}

// ============ Agent Tests ============

func TestUpsertAgent(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{
		ID:       "agent-1",
		Hostname: "test-host",
		IP:       "192.168.1.1",
		Region:   "us-east",
		Zone:     "zone-1",
		Status:   "online",
		Labels:   map[string]interface{}{"env": "test"},
		Capabilities: []Capability{
			{Type: "docker", Version: "1.0"},
		},
	}

	err = db.UpsertAgent(agent)
	if err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	if agent.ID == "" {
		t.Error("agent.ID should be set")
	}

	// Update
	agent.Hostname = "updated-host"
	err = db.UpsertAgent(agent)
	if err != nil {
		t.Fatalf("UpsertAgent() update error = %v", err)
	}
}

func TestGetAgent(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{
		ID:       "agent-1",
		Hostname: "test-host",
		Status:   "online",
	}
	db.UpsertAgent(agent)

	// Found
	found, err := db.GetAgent("agent-1")
	if err != nil {
		t.Errorf("GetAgent() error = %v", err)
	}
	if found.Hostname != "test-host" {
		t.Errorf("Hostname = %v, want test-host", found.Hostname)
	}

	// Not found
	_, err = db.GetAgent("non-existent")
	if err != ErrNotFound {
		t.Errorf("GetAgent() non-existent error = %v, want ErrNotFound", err)
	}
}

func TestListAgents(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	// Add agents
	agents := []*Agent{
		{ID: "agent-1", Hostname: "zebra", Status: "online"},
		{ID: "agent-2", Hostname: "alpha", Status: "online"},
		{ID: "agent-3", Hostname: "beta", Status: "offline"},
	}
	for _, a := range agents {
		db.UpsertAgent(a)
	}

	list, err := db.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("ListAgents() length = %d, want 3", len(list))
	}

	// Should be ordered by hostname
	if list[0].Hostname != "alpha" {
		t.Errorf("first agent = %v, want alpha", list[0].Hostname)
	}
}

func TestListAgentsByRegion(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agents := []*Agent{
		{ID: "agent-1", Hostname: "host1", Region: "us-east", Zone: "zone-1"},
		{ID: "agent-2", Hostname: "host2", Region: "us-east", Zone: "zone-2"},
		{ID: "agent-3", Hostname: "host3", Region: "us-west", Zone: "zone-1"},
	}
	for _, a := range agents {
		db.UpsertAgent(a)
	}

	list, err := db.ListAgentsByRegion("us-east")
	if err != nil {
		t.Fatalf("ListAgentsByRegion() error = %v", err)
	}

	if len(list) != 2 {
		t.Errorf("ListAgentsByRegion() length = %d, want 2", len(list))
	}

	// Should be ordered by zone, then hostname
	if list[0].Zone != "zone-1" {
		t.Errorf("first agent zone = %v, want zone-1", list[0].Zone)
	}
}

func TestDeleteAgent(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "test"}
	db.UpsertAgent(agent)

	err = db.DeleteAgent("agent-1")
	if err != nil {
		t.Errorf("DeleteAgent() error = %v", err)
	}

	_, err = db.GetAgent("agent-1")
	if err != ErrNotFound {
		t.Error("agent should be deleted")
	}
}

func TestUpdateAgentStatus(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "test", Status: "online"}
	db.UpsertAgent(agent)

	lastSeen := time.Now().UTC()
	err = db.UpdateAgentStatus("agent-1", "offline", lastSeen)
	if err != nil {
		t.Fatalf("UpdateAgentStatus() error = %v", err)
	}

	updated, err := db.GetAgent("agent-1")
	if err != nil {
		t.Fatal(err)
	}

	if updated.Status != "offline" {
		t.Errorf("Status = %v, want offline", updated.Status)
	}
}

func TestCleanupOfflineAgents(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	// Use local time to match what CleanupOfflineAgents uses (time.Now())
	oldTime := time.Now().Add(-2 * time.Hour)
	recentTime := time.Now().Add(-30 * time.Minute)

	// Use raw SQL to insert with specific timestamps
	db.db.Exec("INSERT INTO agents (id, hostname, status, last_seen, first_seen, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"old-offline", "old", "offline", oldTime, oldTime, oldTime, oldTime)
	db.db.Exec("INSERT INTO agents (id, hostname, status, last_seen, first_seen, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"old-online", "old2", "online", oldTime, oldTime, oldTime, oldTime)
	db.db.Exec("INSERT INTO agents (id, hostname, status, last_seen, first_seen, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"recent", "recent", "offline", recentTime, recentTime, recentTime, recentTime)

	removed, err := db.CleanupOfflineAgents(1 * time.Hour)
	if err != nil {
		t.Fatalf("CleanupOfflineAgents() error = %v", err)
	}

	// Should remove agents with last_seen older than 1 hour
	// old-offline: status=offline AND last_seen < cutoff ✓
	// old-online: last_seen < cutoff ✓
	// recent: last_seen > cutoff ✗
	if len(removed) != 2 {
		t.Errorf("removed = %d, want 2", len(removed))
	}

	// Verify the recent agent still exists
	_, err = db.GetAgent("recent")
	if err != nil {
		t.Error("recent agent should still exist")
	}
}

// ============ ComputeInstance Tests ============

func TestUpsertComputeInstance(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	inst := &ComputeInstance{
		ID:      "inst-1",
		Name:    "test-vm",
		AgentID: "agent-1",
		Type:    "vm",
		Status:  "running",
	}

	err = db.UpsertComputeInstance(inst)
	if err != nil {
		t.Fatalf("UpsertComputeInstance() error = %v", err)
	}

	// Update
	inst.Status = "stopped"
	err = db.UpsertComputeInstance(inst)
	if err != nil {
		t.Fatalf("UpsertComputeInstance() update error = %v", err)
	}
}

func TestGetComputeInstance(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	inst := &ComputeInstance{
		ID:      "inst-1",
		Name:    "test",
		AgentID: "agent-1",
		Type:    "vm",
	}
	db.UpsertComputeInstance(inst)

	found, err := db.GetComputeInstance("inst-1")
	if err != nil {
		t.Fatalf("GetComputeInstance() error = %v", err)
	}

	if found.Name != "test" {
		t.Errorf("Name = %v, want test", found.Name)
	}

	if found.Agent == nil {
		t.Error("Agent should be preloaded")
	}
}

func TestListComputeInstances(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	instances := []*ComputeInstance{
		{ID: "inst-1", Name: "aaa", AgentID: "agent-1", Type: "vm", Region: "us-east", Status: "running"},
		{ID: "inst-2", Name: "bbb", AgentID: "agent-1", Type: "container", Region: "us-west", Status: "stopped"},
		{ID: "inst-3", Name: "ccc", AgentID: "agent-1", Type: "vm", Region: "us-east", Status: "running"},
	}
	for _, i := range instances {
		db.UpsertComputeInstance(i)
	}

	// No filter
	all, err := db.ListComputeInstances(nil)
	if err != nil {
		t.Fatalf("ListComputeInstances() error = %v", err)
	}
	if len(all) != 3 {
		t.Errorf("length = %d, want 3", len(all))
	}

	// Filter by region
	filtered, err := db.ListComputeInstances(&ComputeInstanceFilter{Region: "us-east"})
	if err != nil {
		t.Fatalf("ListComputeInstances(filter) error = %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("filtered length = %d, want 2", len(filtered))
	}

	// Filter by status
	running, err := db.ListComputeInstances(&ComputeInstanceFilter{Status: "running"})
	if err != nil {
		t.Fatalf("ListComputeInstances(status) error = %v", err)
	}
	if len(running) != 2 {
		t.Errorf("running length = %d, want 2", len(running))
	}

	// Combined filter
	combined, err := db.ListComputeInstances(&ComputeInstanceFilter{
		Region: "us-east",
		Status: "running",
		Type:   "vm",
	})
	if err != nil {
		t.Fatalf("ListComputeInstances(combined) error = %v", err)
	}
	if len(combined) != 2 {
		t.Errorf("combined length = %d, want 2", len(combined))
	}
}

func TestDeleteComputeInstance(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	inst := &ComputeInstance{ID: "inst-1", Name: "test", AgentID: "agent-1"}
	db.UpsertComputeInstance(inst)

	err = db.DeleteComputeInstance("inst-1")
	if err != nil {
		t.Errorf("DeleteComputeInstance() error = %v", err)
	}

	_, err = db.GetComputeInstance("inst-1")
	if err != ErrNotFound {
		t.Error("instance should be deleted")
	}
}

// ============ Domain Tests ============

func TestUpsertDomain(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	domain := &Domain{
		ID:      "domain-1",
		Domain:  "example.com",
		Status:  "active",
		AutoRenew: true,
		Labels:  map[string]string{"env": "prod"},
	}

	err = db.UpsertDomain(domain)
	if err != nil {
		t.Fatalf("UpsertDomain() error = %v", err)
	}
}

func TestGetDomain(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	domain := &Domain{
		ID:     "domain-1",
		Domain: "example.com",
		Status: "active",
	}
	db.UpsertDomain(domain)

	found, err := db.GetDomain("domain-1")
	if err != nil {
		t.Fatalf("GetDomain() error = %v", err)
	}

	if found.Domain != "example.com" {
		t.Errorf("Domain = %v, want example.com", found.Domain)
	}
}

func TestGetDomainByName(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	domain := &Domain{
		ID:     "domain-1",
		Domain: "example.com",
		Status: "active",
	}
	db.UpsertDomain(domain)

	found, err := db.GetDomainByName("example.com")
	if err != nil {
		t.Fatalf("GetDomainByName() error = %v", err)
	}

	if found.ID != "domain-1" {
		t.Errorf("ID = %v, want domain-1", found.ID)
	}

	_, err = db.GetDomainByName("nonexistent.com")
	if err != ErrNotFound {
		t.Error("should return ErrNotFound for nonexistent domain")
	}
}

func TestListDomains(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	domains := []*Domain{
		{ID: "d-1", Domain: "aaa.com", Status: "active"},
		{ID: "d-2", Domain: "zzz.com", Status: "active"},
		{ID: "d-3", Domain: "mmm.com", Status: "expired"},
	}
	for _, d := range domains {
		db.UpsertDomain(d)
	}

	list, err := db.ListDomains()
	if err != nil {
		t.Fatalf("ListDomains() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("length = %d, want 3", len(list))
	}

	// Should be ordered by domain name
	if list[0].Domain != "aaa.com" {
		t.Errorf("first domain = %v, want aaa.com", list[0].Domain)
	}
}

func TestDeleteDomain(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	domain := &Domain{ID: "domain-1", Domain: "test.com"}
	db.UpsertDomain(domain)

	err = db.DeleteDomain("domain-1")
	if err != nil {
		t.Errorf("DeleteDomain() error = %v", err)
	}

	_, err = db.GetDomain("domain-1")
	if err != ErrNotFound {
		t.Error("domain should be deleted")
	}
}

// ============ Certificate Tests ============

func TestUpsertCertificate(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	domain := &Domain{ID: "domain-1", Domain: "example.com"}
	db.UpsertDomain(domain)

	domainID := "domain-1"
	cert := &Certificate{
		ID:         "cert-1",
		DomainID:   &domainID,
		DomainName: "example.com",
		Issuer:     "Let's Encrypt",
		Status:     "valid",
		ExpiresAt:  time.Now().UTC().Add(90 * 24 * time.Hour),
		AutoRenew:  true,
	}

	err = db.UpsertCertificate(cert)
	if err != nil {
		t.Fatalf("UpsertCertificate() error = %v", err)
	}
}

func TestGetCertificate(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	domain := &Domain{ID: "domain-1", Domain: "example.com"}
	db.UpsertDomain(domain)

	domainID := "domain-1"
	cert := &Certificate{
		ID:         "cert-1",
		DomainID:   &domainID,
		DomainName: "example.com",
		Status:     "valid",
		ExpiresAt:  time.Now().UTC().Add(90 * 24 * time.Hour),
	}
	db.UpsertCertificate(cert)

	found, err := db.GetCertificate("cert-1")
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}

	if found.DomainName != "example.com" {
		t.Errorf("DomainName = %v, want example.com", found.DomainName)
	}

	if found.Domain == nil {
		t.Error("Domain should be preloaded")
	}
}

func TestListCertificates(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	certs := []*Certificate{
		{ID: "cert-1", DomainName: "a.com", Status: "valid", ExpiresAt: time.Now().UTC().Add(100 * time.Hour)},
		{ID: "cert-2", DomainName: "b.com", Status: "valid", ExpiresAt: time.Now().UTC().Add(50 * time.Hour)},
		{ID: "cert-3", DomainName: "c.com", Status: "expired", ExpiresAt: time.Now().UTC().Add(-10 * time.Hour)},
	}
	for _, c := range certs {
		db.UpsertCertificate(c)
	}

	list, err := db.ListCertificates()
	if err != nil {
		t.Fatalf("ListCertificates() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("length = %d, want 3", len(list))
	}

	// Should be ordered by expires_at
	if list[0].ID != "cert-3" {
		t.Errorf("first cert should be expired, got %v", list[0].ID)
	}
}

func TestListExpiringCertificates(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	now := time.Now().UTC()

	certs := []*Certificate{
		{ID: "cert-1", DomainName: "expiring.com", Status: "valid", ExpiresAt: now.Add(15 * 24 * time.Hour)},
		{ID: "cert-2", DomainName: "safe.com", Status: "valid", ExpiresAt: now.Add(60 * 24 * time.Hour)},
		{ID: "cert-3", DomainName: "soon.com", Status: "valid", ExpiresAt: now.Add(5 * 24 * time.Hour)},
	}
	for _, c := range certs {
		db.UpsertCertificate(c)
	}

	expiring, err := db.ListExpiringCertificates(30 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("ListExpiringCertificates() error = %v", err)
	}

	if len(expiring) != 2 {
		t.Errorf("length = %d, want 2", len(expiring))
	}
}

func TestDeleteCertificate(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	cert := &Certificate{
		ID:         "cert-1",
		DomainName: "example.com",
		Status:     "valid",
		ExpiresAt:  time.Now().UTC().Add(90 * 24 * time.Hour),
	}
	db.UpsertCertificate(cert)

	err = db.DeleteCertificate("cert-1")
	if err != nil {
		t.Errorf("DeleteCertificate() error = %v", err)
	}

	_, err = db.GetCertificate("cert-1")
	if err != ErrNotFound {
		t.Error("certificate should be deleted")
	}
}

// ============ Service Tests ============

func TestUpsertService(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	agentID := "agent-1"
	service := &Service{
		ID:      "svc-1",
		Name:    "api",
		AgentID: &agentID,
		Type:    "http",
		URL:     "http://localhost:8080",
		Status:  "up",
	}

	err = db.UpsertService(service)
	if err != nil {
		t.Fatalf("UpsertService() error = %v", err)
	}
}

func TestGetService(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	agentID := "agent-1"
	service := &Service{
		ID:      "svc-1",
		Name:    "api",
		AgentID: &agentID,
		Type:    "http",
		Status:  "up",
	}
	db.UpsertService(service)

	found, err := db.GetService("svc-1")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}

	if found.Name != "api" {
		t.Errorf("Name = %v, want api", found.Name)
	}

	if found.Agent == nil {
		t.Error("Agent should be preloaded")
	}
}

func TestListServices(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	services := []*Service{
		{ID: "svc-1", Name: "zzz", Type: "http", Status: "up"},
		{ID: "svc-2", Name: "aaa", Type: "tcp", Status: "down"},
		{ID: "svc-3", Name: "mmm", Type: "database", Status: "degraded"},
	}
	for _, s := range services {
		db.UpsertService(s)
	}

	list, err := db.ListServices()
	if err != nil {
		t.Fatalf("ListServices() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("length = %d, want 3", len(list))
	}

	// Should be ordered by name
	if list[0].Name != "aaa" {
		t.Errorf("first service = %v, want aaa", list[0].Name)
	}
}

func TestDeleteService(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	service := &Service{ID: "svc-1", Name: "test", Type: "http"}
	db.UpsertService(service)

	err = db.DeleteService("svc-1")
	if err != nil {
		t.Errorf("DeleteService() error = %v", err)
	}

	_, err = db.GetService("svc-1")
	if err != ErrNotFound {
		t.Error("service should be deleted")
	}
}

// ============ Gateway Tests ============

func TestUpsertGateway(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	gateway := &Gateway{
		ID:      "gw-1",
		Name:    "main-gateway",
		AgentID: "agent-1",
		Type:    "openwrt",
		IPv4:    "192.168.1.1",
		Status:  "online",
	}

	err = db.UpsertGateway(gateway)
	if err != nil {
		t.Fatalf("UpsertGateway() error = %v", err)
	}
}

func TestGetGateway(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	gateway := &Gateway{
		ID:      "gw-1",
		Name:    "main",
		AgentID: "agent-1",
		Type:    "openwrt",
		Status:  "online",
	}
	db.UpsertGateway(gateway)

	found, err := db.GetGateway("gw-1")
	if err != nil {
		t.Fatalf("GetGateway() error = %v", err)
	}

	if found.Name != "main" {
		t.Errorf("Name = %v, want main", found.Name)
	}

	if found.Agent == nil {
		t.Error("Agent should be preloaded")
	}
}

func TestListGateways(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	gateways := []*Gateway{
		{ID: "gw-1", Name: "zzz", AgentID: "agent-1", Type: "openwrt"},
		{ID: "gw-2", Name: "aaa", AgentID: "agent-1", Type: "pfsense"},
	}
	for _, g := range gateways {
		db.UpsertGateway(g)
	}

	list, err := db.ListGateways()
	if err != nil {
		t.Fatalf("ListGateways() error = %v", err)
	}

	if len(list) != 2 {
		t.Errorf("length = %d, want 2", len(list))
	}

	if list[0].Name != "aaa" {
		t.Errorf("first gateway = %v, want aaa", list[0].Name)
	}
}

func TestDeleteGateway(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	gateway := &Gateway{ID: "gw-1", Name: "test", AgentID: "agent-1"}
	db.UpsertGateway(gateway)

	err = db.DeleteGateway("gw-1")
	if err != nil {
		t.Errorf("DeleteGateway() error = %v", err)
	}

	_, err = db.GetGateway("gw-1")
	if err != ErrNotFound {
		t.Error("gateway should be deleted")
	}
}

// ============ Storage Tests ============

func TestUpsertStorage(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	storage := &Storage{
		ID:          "storage-1",
		Name:        "main-storage",
		AgentID:     "agent-1",
		Type:        "nfs",
		Path:        "/mnt/data",
		TotalGB:     1000,
		UsedGB:      500,
		AvailableGB: 500,
		Status:      "online",
	}

	err = db.UpsertStorage(storage)
	if err != nil {
		t.Fatalf("UpsertStorage() error = %v", err)
	}
}

func TestGetStorage(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	storage := &Storage{
		ID:      "storage-1",
		Name:    "main",
		AgentID: "agent-1",
		Type:    "nfs",
		Status:  "online",
	}
	db.UpsertStorage(storage)

	found, err := db.GetStorage("storage-1")
	if err != nil {
		t.Fatalf("GetStorage() error = %v", err)
	}

	if found.Name != "main" {
		t.Errorf("Name = %v, want main", found.Name)
	}

	if found.Agent == nil {
		t.Error("Agent should be preloaded")
	}
}

func TestListStorages(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	storages := []*Storage{
		{ID: "s-1", Name: "zzz", AgentID: "agent-1", Type: "nfs"},
		{ID: "s-2", Name: "aaa", AgentID: "agent-1", Type: "local"},
	}
	for _, s := range storages {
		db.UpsertStorage(s)
	}

	list, err := db.ListStorages()
	if err != nil {
		t.Fatalf("ListStorages() error = %v", err)
	}

	if len(list) != 2 {
		t.Errorf("length = %d, want 2", len(list))
	}
}

func TestDeleteStorage(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	agent := &Agent{ID: "agent-1", Hostname: "host"}
	db.UpsertAgent(agent)

	storage := &Storage{ID: "s-1", Name: "test", AgentID: "agent-1"}
	db.UpsertStorage(storage)

	err = db.DeleteStorage("s-1")
	if err != nil {
		t.Errorf("DeleteStorage() error = %v", err)
	}

	_, err = db.GetStorage("s-1")
	if err != ErrNotFound {
		t.Error("storage should be deleted")
	}
}

// ============ Stats Tests ============

func TestGetStats(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	// Add test data
	agents := []*Agent{
		{ID: "agent-1", Hostname: "online-host", Status: "online"},
		{ID: "agent-2", Hostname: "offline-host", Status: "offline"},
	}
	for _, a := range agents {
		db.UpsertAgent(a)
	}

	instances := []*ComputeInstance{
		{ID: "inst-1", Name: "running", AgentID: "agent-1", Type: "vm", Status: "running"},
		{ID: "inst-2", Name: "stopped", AgentID: "agent-1", Type: "vm", Status: "stopped"},
	}
	for _, i := range instances {
		db.UpsertComputeInstance(i)
	}

	domains := []*Domain{
		{ID: "d-1", Domain: "active.com", Status: "active"},
		{ID: "d-2", Domain: "pending.com", Status: "pending"},
	}
	for _, d := range domains {
		db.UpsertDomain(d)
	}

	services := []*Service{
		{ID: "svc-1", Name: "up", Type: "http", Status: "up"},
		{ID: "svc-2", Name: "down", Type: "tcp", Status: "down"},
	}
	for _, s := range services {
		db.UpsertService(s)
	}

	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	if stats.AgentsOnline != 1 {
		t.Errorf("AgentsOnline = %d, want 1", stats.AgentsOnline)
	}

	if stats.AgentsTotal != 2 {
		t.Errorf("AgentsTotal = %d, want 2", stats.AgentsTotal)
	}

	if stats.ComputeInstancesRunning != 1 {
		t.Errorf("ComputeInstancesRunning = %d, want 1", stats.ComputeInstancesRunning)
	}

	if stats.ComputeInstancesTotal != 2 {
		t.Errorf("ComputeInstancesTotal = %d, want 2", stats.ComputeInstancesTotal)
	}

	if stats.DomainsActive != 1 {
		t.Errorf("DomainsActive = %d, want 1", stats.DomainsActive)
	}

	if stats.ServicesUp != 1 {
		t.Errorf("ServicesUp = %d, want 1", stats.ServicesUp)
	}

	if stats.ServicesDown != 1 {
		t.Errorf("ServicesDown = %d, want 1", stats.ServicesDown)
	}
}

// ============ SystemMetric Tests ============

func TestSaveSystemMetric(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	metric := &SystemMetric{
		AgentID:   "agent-1",
		Timestamp: time.Now().UTC(),
		CPUUsage:  50.5,
		MemUsed:   1024,
	}

	err = db.SaveSystemMetric(metric)
	if err != nil {
		t.Fatalf("SaveSystemMetric() error = %v", err)
	}
}

func TestUpdateSystemInfoSnapshot(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	snapshot := &SystemInfoSnapshot{
		AgentID:    "agent-1",
		OSName:     "Linux",
		OSVersion:  "5.15.0",
		Arch:       "x86_64",
		Hostname:   "test-host",
	}

	err = db.UpdateSystemInfoSnapshot(snapshot)
	if err != nil {
		t.Fatalf("UpdateSystemInfoSnapshot() error = %v", err)
	}

	// Update
	snapshot.OSVersion = "6.0.0"
	err = db.UpdateSystemInfoSnapshot(snapshot)
	if err != nil {
		t.Fatalf("UpdateSystemInfoSnapshot() update error = %v", err)
	}
}

func TestGetSystemInfoSnapshot(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	snapshot := &SystemInfoSnapshot{
		AgentID:  "agent-1",
		OSName:   "Linux",
		Hostname: "test-host",
	}
	db.UpdateSystemInfoSnapshot(snapshot)

	found, err := db.GetSystemInfoSnapshot("agent-1")
	if err != nil {
		t.Fatalf("GetSystemInfoSnapshot() error = %v", err)
	}

	if found.Hostname != "test-host" {
		t.Errorf("Hostname = %v, want test-host", found.Hostname)
	}

	_, err = db.GetSystemInfoSnapshot("non-existent")
	if err != ErrNotFound {
		t.Error("should return ErrNotFound for non-existent snapshot")
	}
}

func TestListSystemInfoSnapshots(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	snapshots := []*SystemInfoSnapshot{
		{AgentID: "agent-1", OSName: "Linux", Hostname: "host1"},
		{AgentID: "agent-2", OSName: "FreeBSD", Hostname: "host2"},
	}
	for _, s := range snapshots {
		db.UpdateSystemInfoSnapshot(s)
	}

	list, err := db.ListSystemInfoSnapshots()
	if err != nil {
		t.Fatalf("ListSystemInfoSnapshots() error = %v", err)
	}

	if len(list) != 2 {
		t.Errorf("length = %d, want 2", len(list))
	}
}

func TestGetSystemMetrics(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	now := time.Now().UTC()
	metrics := []*SystemMetric{
		{AgentID: "agent-1", Timestamp: now.Add(-3 * time.Hour), CPUUsage: 30},
		{AgentID: "agent-1", Timestamp: now.Add(-2 * time.Hour), CPUUsage: 40},
		{AgentID: "agent-1", Timestamp: now.Add(-1 * time.Hour), CPUUsage: 50},
		{AgentID: "agent-1", Timestamp: now, CPUUsage: 60},
		{AgentID: "agent-2", Timestamp: now, CPUUsage: 70},
	}
	for _, m := range metrics {
		db.SaveSystemMetric(m)
	}

	// Get with limit
	list, err := db.GetSystemMetrics("agent-1", 2, 0)
	if err != nil {
		t.Fatalf("GetSystemMetrics() error = %v", err)
	}

	if len(list) != 2 {
		t.Errorf("length = %d, want 2", len(list))
	}

	// Should be ordered by timestamp DESC (most recent first)
	if list[0].CPUUsage != 60 {
		t.Errorf("first metric CPUUsage = %v, want 60", list[0].CPUUsage)
	}
}

func TestGetSystemMetricsByTimeRange(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	now := time.Now().UTC()
	metrics := []*SystemMetric{
		{AgentID: "agent-1", Timestamp: now.Add(-5 * time.Hour), CPUUsage: 10},
		{AgentID: "agent-1", Timestamp: now.Add(-3 * time.Hour), CPUUsage: 30},
		{AgentID: "agent-1", Timestamp: now.Add(-1 * time.Hour), CPUUsage: 50},
	}
	for _, m := range metrics {
		db.SaveSystemMetric(m)
	}

	start := now.Add(-4 * time.Hour)
	end := now.Add(-2 * time.Hour)

	list, err := db.GetSystemMetricsByTimeRange("agent-1", start, end)
	if err != nil {
		t.Fatalf("GetSystemMetricsByTimeRange() error = %v", err)
	}

	if len(list) != 1 {
		t.Errorf("length = %d, want 1", len(list))
	}

	if list[0].CPUUsage != 30 {
		t.Errorf("CPUUsage = %v, want 30", list[0].CPUUsage)
	}
}

func TestCleanupOldMetrics(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	now := time.Now().UTC()
	metrics := []*SystemMetric{
		{AgentID: "agent-1", Timestamp: now.Add(-25 * time.Hour), CPUUsage: 10},
		{AgentID: "agent-1", Timestamp: now.Add(-20 * time.Hour), CPUUsage: 20},
		{AgentID: "agent-1", Timestamp: now.Add(-5 * time.Hour), CPUUsage: 50},
	}
	for _, m := range metrics {
		db.SaveSystemMetric(m)
	}

	deleted, err := db.CleanupOldMetrics(24 * time.Hour)
	if err != nil {
		t.Fatalf("CleanupOldMetrics() error = %v", err)
	}

	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}
}

// ============ Transaction Tests ============

func TestTransaction(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	err = db.Transaction(func(tx *DB) error {
		agent := &Agent{
			ID:       "agent-1",
			Hostname: "test",
			Status:   "online",
		}
		return tx.UpsertAgent(agent)
	})

	if err != nil {
		t.Fatalf("Transaction() error = %v", err)
	}

	agent, err := db.GetAgent("agent-1")
	if err != nil {
		t.Fatal("agent should exist after transaction")
	}

	if agent.Hostname != "test" {
		t.Errorf("Hostname = %v, want test", agent.Hostname)
	}
}

func TestTransactionRollback(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	defer os.Remove("cockpit.db")

	expectedErr := errors.New("rollback test")
	err = db.Transaction(func(tx *DB) error {
		agent := &Agent{
			ID:       "agent-1",
			Hostname: "test",
		}
		tx.UpsertAgent(agent)
		return expectedErr
	})

	if err != expectedErr {
		t.Fatalf("Transaction() error = %v, want %v", err, expectedErr)
	}

	_, err = db.GetAgent("agent-1")
	if err != ErrNotFound {
		t.Error("agent should not exist after rollback")
	}
}

// ============ Config Tests ============

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}

	if cfg.Path != "" {
		t.Error("Path should be empty by default")
	}

	if cfg.LogLevel != 0 {
		t.Error("LogLevel should be 0 by default")
	}
}

// ============ ErrNotFound Tests ============

func TestErrNotFound(t *testing.T) {
	if ErrNotFound == nil {
		t.Error("ErrNotFound should not be nil")
	}

	if ErrNotFound.Error() != "record not found" {
		t.Errorf("ErrNotFound.Error() = %v, want 'record not found'", ErrNotFound.Error())
	}
}

// ============ Stats Struct Tests ============

func TestStatsStruct(t *testing.T) {
	stats := &Stats{
		AgentsOnline:            5,
		AgentsTotal:             10,
		ComputeInstancesRunning: 20,
		ComputeInstancesTotal:   25,
		DomainsActive:           3,
		CertificatesValid:       15,
		CertificatesExpiring:    2,
		ServicesUp:              8,
		ServicesDown:            1,
	}

	if stats.AgentsOnline != 5 {
		t.Errorf("AgentsOnline = %d, want 5", stats.AgentsOnline)
	}

	if stats.AgentsTotal != 10 {
		t.Errorf("AgentsTotal = %d, want 10", stats.AgentsTotal)
	}
}

// ============ ComputeInstanceFilter Tests ============

func TestComputeInstanceFilterDefaults(t *testing.T) {
	filter := &ComputeInstanceFilter{}

	if filter.Region != "" {
		t.Error("Region should be empty by default")
	}

	if filter.Zone != "" {
		t.Error("Zone should be empty by default")
	}

	if filter.Type != "" {
		t.Error("Type should be empty by default")
	}

	if filter.Status != "" {
		t.Error("Status should be empty by default")
	}
}

// RemoveTestDB 删除测试数据库文件
func RemoveTestDB(db *DB) {
	if db != nil {
		sqlDB, _ := db.db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}
	os.Remove("cockpit.db")
	os.Remove("cockpit.db-shm")
	os.Remove("cockpit.db-wal")
}

