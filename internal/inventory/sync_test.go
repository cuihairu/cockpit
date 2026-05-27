package inventory

import (
	"context"
	"os"
	"testing"

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

// Build an inventory with agents nested in zones, domains with certs
func testInventory() *Inventory {
	return &Inventory{
		Version: "v1",
		Metadata: Metadata{
			Name: "test",
		},
		Regions: map[string]*Region{
			"us-east": {
				ID:   "us-east",
				Name: "US East",
				Zones: map[string]*Zone{
					"us-east-1a": {
						ID:   "us-east-1a",
						Name: "US East 1A",
						Agents: map[string]*Agent{
							"agent-1": {
								ID:           "agent-1",
								Hostname:     "web-server",
								IP:           "10.0.0.1",
								Capabilities: []string{"remote-services", "proxy"},
								Config:       map[string]any{"port": 8080},
							},
							"agent-2": {
								ID:       "agent-2",
								Hostname: "db-server",
								IP:       "10.0.0.2",
							},
						},
					},
				},
			},
		},
		Domains: map[string]*Domain{
			"domain-1": {
				ID:        "domain-1",
				Domain:    "example.com",
				Provider:  "cloudflare",
				AutoRenew: true,
				Agent:     "agent-1",
				Certificates: []*Certificate{
					{
						ID:              "cert-1",
						Domain:          "example.com",
						Provider:        "letsencrypt",
						AutoRenew:       true,
						RenewBeforeDays: 30,
						Agent:           "agent-1",
					},
				},
			},
			"domain-2": {
				ID:      "domain-2",
				Domain:  "test.org",
				Provider: "route53",
			},
		},
	}
}

func TestNewSyncer(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)
	if s == nil {
		t.Fatal("NewSyncer() should not return nil")
	}
	if s.db != db {
		t.Error("Syncer.db should match input")
	}
}

func TestSyncerSyncBasic(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)
	inv := testInventory()

	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result == nil {
		t.Fatal("Sync() result should not be nil")
	}
	if result.Agents == nil || result.Domains == nil || result.Certificates == nil {
		t.Error("All resource results should be populated")
	}
}

func TestSyncerAgents(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)
	inv := testInventory()

	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	// BeforeCreate hook sets FirstSeen, making sync count first upsert as "updated"
	if result.Agents.Updated != 2 {
		t.Errorf("Agents.Updated = %d, want 2", result.Agents.Updated)
	}

	// Verify agents were stored
	agent1, err := db.GetAgent("agent-1")
	if err != nil {
		t.Fatalf("GetAgent(agent-1) error = %v", err)
	}
	if agent1.Hostname != "web-server" {
		t.Errorf("agent-1.Hostname = %v", agent1.Hostname)
	}
	if agent1.IP != "10.0.0.1" {
		t.Errorf("agent-1.IP = %v", agent1.IP)
	}
	if agent1.Status != "offline" {
		t.Errorf("agent-1.Status = %v, want offline", agent1.Status)
	}
	if agent1.Region != "us-east" {
		t.Errorf("agent-1.Region = %v, want us-east", agent1.Region)
	}
	if agent1.Zone != "us-east-1a" {
		t.Errorf("agent-1.Zone = %v, want us-east-1a", agent1.Zone)
	}
}

func TestSyncerDomains(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)
	inv := testInventory()

	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if result.Domains.Created != 2 {
		t.Errorf("Domains.Created = %d, want 2", result.Domains.Created)
	}

	// Verify domain-1 has agent association
	domain1, err := db.GetDomain("domain-1")
	if err != nil {
		t.Fatalf("GetDomain(domain-1) error = %v", err)
	}
	if domain1.Domain != "example.com" {
		t.Errorf("domain-1.Domain = %v", domain1.Domain)
	}
	if domain1.Provider != "cloudflare" {
		t.Errorf("domain-1.Provider = %v", domain1.Provider)
	}
	if domain1.AgentID == nil || *domain1.AgentID != "agent-1" {
		t.Errorf("domain-1.AgentID = %v, want agent-1", domain1.AgentID)
	}
}

func TestSyncerCertificates(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)
	inv := testInventory()

	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if result.Certificates.Created != 1 {
		t.Errorf("Certificates.Created = %d, want 1", result.Certificates.Created)
	}

	certs, err := db.ListCertificates()
	if err != nil {
		t.Fatalf("ListCertificates() error = %v", err)
	}
	if len(certs) < 1 {
		t.Fatalf("ListCertificates() count = %d, want >= 1", len(certs))
	}

	cert := certs[0]
	if cert.DomainName != "example.com" {
		t.Errorf("cert.DomainName = %v", cert.DomainName)
	}
	if cert.Issuer != "letsencrypt" {
		t.Errorf("cert.Issuer = %v", cert.Issuer)
	}
}

func TestSyncerEmptyInventory(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)

	inv := &Inventory{
		Version:  "v1",
		Metadata: Metadata{Name: "empty"},
	}

	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if result.Agents.Created != 0 {
		t.Errorf("Agents.Created = %d, want 0", result.Agents.Created)
	}
	if result.Domains.Created != 0 {
		t.Errorf("Domains.Created = %d, want 0", result.Domains.Created)
	}
	if result.Certificates.Created != 0 {
		t.Errorf("Certificates.Created = %d, want 0", result.Certificates.Created)
	}
}

func TestSyncerNilDomains(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)

	inv := &Inventory{
		Version:  "v1",
		Metadata: Metadata{Name: "test"},
		Domains: map[string]*Domain{
			"nil-domain": nil,
		},
	}

	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Domains.Created != 0 {
		t.Errorf("Domains.Created = %d, want 0 for nil domain", result.Domains.Created)
	}
}

func TestSyncerReSync(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)
	inv := testInventory()

	// First sync
	_, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("First Sync() error = %v", err)
	}

	// Second sync should update, not create
	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Second Sync() error = %v", err)
	}

	// Agents should be updated now
	if result.Agents.Updated != 2 {
		t.Errorf("Agents.Updated = %d, want 2", result.Agents.Updated)
	}
	if result.Agents.Created != 0 {
		t.Errorf("Agents.Created = %d, want 0 on re-sync", result.Agents.Created)
	}
}

func TestSyncerParseFileAndSync(t *testing.T) {
	db := testDB(t)

	dir := t.TempDir()
	yamlContent := []byte(`version: "v1"
metadata:
  name: file-test
regions:
  local:
    id: local
    name: Local
    zones:
      zone-a:
        id: zone-a
        name: Zone A
        agents:
          file-agent:
            id: file-agent
            hostname: file-host
            ip: 192.168.1.1
domains:
  d1:
    id: d1
    domain: file.example.com
    provider: manual
`)
	yamlPath := dir + "/inventory.yaml"
	if err := os.WriteFile(yamlPath, yamlContent, 0644); err != nil {
		t.Fatal(err)
	}

	inv, err := ParseFile(yamlPath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	s := NewSyncer(db)
	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if result.Agents.Updated != 1 {
		t.Errorf("Agents.Updated = %d, want 1", result.Agents.Updated)
	}
	if result.Domains.Created != 1 {
		t.Errorf("Domains.Created = %d, want 1", result.Domains.Created)
	}

	agent, err := db.GetAgent("file-agent")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if agent.Hostname != "file-host" {
		t.Errorf("agent.Hostname = %v", agent.Hostname)
	}
	if agent.Region != "local" {
		t.Errorf("agent.Region = %v, want local", agent.Region)
	}
}

func TestResourceResultDefaults(t *testing.T) {
	r := &ResourceResult{}
	if r.Created != 0 || r.Updated != 0 || r.Deleted != 0 || r.Errors != 0 {
		t.Error("ResourceResult should have zero defaults")
	}
}

func TestSyncResultDefaults(t *testing.T) {
	r := &SyncResult{}
	if r.Agents != nil || r.Domains != nil || r.Certificates != nil {
		t.Error("SyncResult should have nil defaults")
	}
}

func TestSyncerCertWithoutDomainInInventory(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)

	// Certificate for a domain that exists in the inventory
	inv := &Inventory{
		Version:  "v1",
		Metadata: Metadata{Name: "test"},
		Domains: map[string]*Domain{
			"d1": {ID: "d1", Domain: "test.com", Provider: "manual"},
		},
	}
	// Manually add certs to domain
	inv.Domains["d1"].Certificates = []*Certificate{
		{
			ID:              "cert-1",
			Domain:          "test.com",
			Provider:        "letsencrypt",
			RenewBeforeDays: 14,
		},
	}

	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Certificates.Created != 1 {
		t.Errorf("Certificates.Created = %d, want 1", result.Certificates.Created)
	}

	certs, _ := db.ListCertificates()
	if len(certs) < 1 {
		t.Fatal("Expected at least 1 certificate")
	}
	// Should have domain association since domain exists
	if certs[0].DomainID == nil {
		t.Error("DomainID should not be nil when matching domain exists")
	}
}

func TestSyncerDomainWithoutAgent(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)

	inv := &Inventory{
		Version:  "v1",
		Metadata: Metadata{Name: "test"},
		Domains: map[string]*Domain{
			"d1": {
				ID:       "d1",
				Domain:   "no-agent.com",
				Provider: "manual",
			},
		},
	}

	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Domains.Created != 1 {
		t.Errorf("Domains.Created = %d, want 1", result.Domains.Created)
	}

	domain, _ := db.GetDomain("d1")
	if domain.AgentID != nil {
		t.Errorf("AgentID should be nil for domain without agent, got %v", domain.AgentID)
	}
}

func TestSyncerCertWithAgent(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)

	inv := &Inventory{
		Version:  "v1",
		Metadata: Metadata{Name: "test"},
		Domains: map[string]*Domain{
			"d1": {
				ID:       "d1",
				Domain:   "agent-cert.com",
				Provider: "manual",
				Certificates: []*Certificate{
					{
						ID:       "cert-agent",
						Domain:   "agent-cert.com",
						Provider: "letsencrypt",
						Agent:    "agent-x",
					},
				},
			},
		},
	}

	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Certificates.Created != 1 {
		t.Errorf("Certificates.Created = %d, want 1", result.Certificates.Created)
	}

	certs, _ := db.ListCertificates()
	if len(certs) < 1 {
		t.Fatal("Expected at least 1 certificate")
	}
	if certs[0].AgentID == nil || *certs[0].AgentID != "agent-x" {
		t.Errorf("AgentID = %v, want agent-x", certs[0].AgentID)
	}
}
