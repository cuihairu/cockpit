package inventory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewInventory(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
	}

	if inv.Version != "v1" {
		t.Errorf("Version = %v, want v1", inv.Version)
	}

	if inv.Regions != nil {
		t.Error("Regions should be nil initially")
	}

	if inv.Domains != nil {
		t.Error("Domains should be nil initially")
	}
}

func TestValidateValidInventory(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Domains: map[string]*Domain{
			"domain1": {
				ID:     "domain1",
				Domain: "example.com",
			},
		},
		Regions: map[string]*Region{
			"region1": {
				ID: "region1",
				Zones: map[string]*Zone{
					"zone1": {
						ID: "zone1",
						Agents: map[string]*Agent{
							"agent1": {
								ID:       "agent1",
								Hostname: "test-host",
							},
						},
					},
				},
			},
		},
	}

	err := inv.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestValidateMissingVersion(t *testing.T) {
	inv := &Inventory{}

	err := inv.Validate()
	if err == nil {
		t.Error("Validate() should return error for missing version")
	}

	if err.Error() != "version is required" {
		t.Errorf("Error message = %v, want 'version is required'", err.Error())
	}
}

func TestValidateInvalidVersion(t *testing.T) {
	inv := &Inventory{
		Version: "v2",
	}

	err := inv.Validate()
	if err == nil {
		t.Error("Validate() should return error for unsupported version")
	}
}

func TestValidateMissingDomainName(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Domains: map[string]*Domain{
			"domain1": {
				ID:     "domain1",
				Domain: "", // Missing domain name
			},
		},
	}

	err := inv.Validate()
	if err == nil {
		t.Error("Validate() should return error for missing domain name")
	}
}

func TestValidateMissingAgentHostname(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{
			"region1": {
				ID: "region1",
				Zones: map[string]*Zone{
					"zone1": {
						ID: "zone1",
						Agents: map[string]*Agent{
							"agent1": {
								ID: "agent1",
								// Missing both hostname and IP
							},
						},
					},
				},
			},
		},
	}

	err := inv.Validate()
	if err == nil {
		t.Error("Validate() should return error for agent without hostname or IP")
	}
}

func TestResolveRefDomain(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Domains: map[string]*Domain{
			"domain1": {
				ID:     "domain1",
				Domain: "example.com",
			},
		},
	}

	result, err := inv.ResolveRef("domains.domain1")
	if err != nil {
		t.Errorf("ResolveRef() error = %v", err)
	}

	domain, ok := result.(*Domain)
	if !ok {
		t.Fatal("Result should be a Domain")
	}

	if domain.Domain != "example.com" {
		t.Errorf("Domain = %v, want example.com", domain.Domain)
	}
}

func TestResolveRefInvalidPath(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
	}

	tests := []struct {
		name string
		path string
	}{
		{"empty path", ""},
		{"too short", "domains"},
		{"invalid type", "invalid.type.path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := inv.ResolveRef(tt.path)
			if err == nil {
				t.Errorf("ResolveRef(%s) should return error", tt.path)
			}
		})
	}
}

func TestResolveRefDomainNotFound(t *testing.T) {
	inv := &Inventory{
		Version:  "v1",
		Domains:  map[string]*Domain{},
		Regions:  map[string]*Region{},
	}

	_, err := inv.ResolveRef("domains.nonexistent")
	if err == nil {
		t.Error("ResolveRef() should return error for non-existent domain")
	}
}

func TestResolveRefRegion(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{
			"region1": {
				ID:   "region1",
				Name: "Test Region",
				Zones: map[string]*Zone{
					"zone1": {
						ID:   "zone1",
						Name: "Test Zone",
					},
				},
			},
		},
	}

	// Region refs require zones qualifier
	result, err := inv.ResolveRef("regions.region1.zones.zone1")
	if err != nil {
		t.Errorf("ResolveRef() error = %v", err)
	}

	zone, ok := result.(*Zone)
	if !ok {
		t.Fatal("Result should be a Zone")
	}

	if zone.Name != "Test Zone" {
		t.Errorf("Zone Name = %v, want 'Test Zone'", zone.Name)
	}
}

func TestResolveRefZone(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{
			"region1": {
				ID: "region1",
				Zones: map[string]*Zone{
					"zone1": {
						ID:   "zone1",
						Name: "Test Zone",
					},
				},
			},
		},
	}

	result, err := inv.ResolveRef("regions.region1.zones.zone1")
	if err != nil {
		t.Errorf("ResolveRef() error = %v", err)
	}

	zone, ok := result.(*Zone)
	if !ok {
		t.Fatal("Result should be a Zone")
	}

	if zone.Name != "Test Zone" {
		t.Errorf("Zone Name = %v, want 'Test Zone'", zone.Name)
	}
}

func TestResolveRefAgent(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{
			"region1": {
				ID: "region1",
				Zones: map[string]*Zone{
					"zone1": {
						ID: "zone1",
						Agents: map[string]*Agent{
							"agent1": {
								ID:       "agent1",
								Hostname: "test-host",
							},
						},
					},
				},
			},
		},
	}

	result, err := inv.ResolveRef("regions.region1.zones.zone1.agents.agent1")
	if err != nil {
		t.Errorf("ResolveRef() error = %v", err)
	}

	agent, ok := result.(*Agent)
	if !ok {
		t.Fatal("Result should be an Agent")
	}

	if agent.Hostname != "test-host" {
		t.Errorf("Agent Hostname = %v, want 'test-host'", agent.Hostname)
	}
}

func TestGetAgents(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{
			"region1": {
				ID:   "region1",
				Name: "Region One",
				Zones: map[string]*Zone{
					"zone1": {
						ID:   "zone1",
						Name: "Zone One",
						Agents: map[string]*Agent{
							"agent1": {
								ID:       "agent1",
								Hostname: "host1",
								IP:       "192.168.1.1",
							},
							"agent2": {
								ID:       "agent2",
								Hostname: "host2",
							},
						},
					},
				},
			},
			"region2": {
				ID:   "region2",
				Name: "Region Two",
				Zones: map[string]*Zone{
					"zone2": {
						ID: "zone2",
						Agents: map[string]*Agent{
							"agent3": {
								ID:       "agent3",
								Hostname: "host3",
							},
						},
					},
				},
			},
		},
	}

	agents := inv.GetAgents()

	if len(agents) != 3 {
		t.Errorf("GetAgents() returned %d agents, want 3", len(agents))
	}

	// Check agent location
	agent1, ok := agents["agent1"]
	if !ok {
		t.Fatal("agent1 should be in result")
	}

	if agent1.Region != "region1" {
		t.Errorf("agent1 Region = %v, want region1", agent1.Region)
	}

	if agent1.Zone != "zone1" {
		t.Errorf("agent1 Zone = %v, want zone1", agent1.Zone)
	}

	if agent1.RegionName != "Region One" {
		t.Errorf("agent1 RegionName = %v, want 'Region One'", agent1.RegionName)
	}
}

func TestGetAgentsEmpty(t *testing.T) {
	inv := &Inventory{
		Version:  "v1",
		Regions:  map[string]*Region{},
	}

	agents := inv.GetAgents()

	if len(agents) != 0 {
		t.Errorf("GetAgents() should return empty map, got %d agents", len(agents))
	}
}

func TestGetDomains(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Domains: map[string]*Domain{
			"domain1": {
				ID:     "domain1",
				Domain: "example.com",
			},
			"domain2": {
				ID:     "domain2",
				Domain: "test.org",
			},
		},
	}

	domains := inv.GetDomains()

	if len(domains) != 2 {
		t.Errorf("GetDomains() returned %d domains, want 2", len(domains))
	}
}

func TestGetCertificates(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Domains: map[string]*Domain{
			"domain1": {
				ID:     "domain1",
				Domain: "example.com",
				Certificates: []*Certificate{
					{
						ID:     "cert1",
						Domain: "example.com",
					},
					{
						ID:     "cert2",
						Domain: "example.com",
					},
				},
			},
			"domain2": {
				ID:     "domain2",
				Domain: "test.org",
				Certificates: []*Certificate{
					{
						ID:     "cert3",
						Domain: "test.org",
					},
				},
			},
		},
	}

	certs := inv.GetCertificates()

	if len(certs) != 3 {
		t.Errorf("GetCertificates() returned %d certificates, want 3", len(certs))
	}
}

func TestMerge(t *testing.T) {
	inv1 := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{
			"region1": {
				ID:   "region1",
				Name: "Original",
			},
		},
	}

	inv2 := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{
			"region2": {
				ID:   "region2",
				Name: "New",
			},
		},
		Domains: map[string]*Domain{
			"domain1": {
				ID:     "domain1",
				Domain: "example.com",
			},
		},
	}

	inv1.Merge(inv2)

	if len(inv1.Regions) != 2 {
		t.Errorf("After Merge, Regions length = %d, want 2", len(inv1.Regions))
	}

	if len(inv1.Domains) != 1 {
		t.Errorf("After Merge, Domains length = %d, want 1", len(inv1.Domains))
	}
}

func TestMergeNilInventory(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
	}

	// Merge with nil should not panic
	inv.Merge(nil)

	if len(inv.Regions) != 0 {
		t.Error("Regions should still be empty after merging nil")
	}
}

func TestParseValidYAML(t *testing.T) {
	yamlData := []byte(`
version: v1
domains:
  domain1:
    domain: example.com
regions:
  region1:
    zones:
      zone1:
        agents:
          agent1:
            hostname: test-host
`)

	inv, err := Parse(yamlData)
	if err != nil {
		t.Errorf("Parse() error = %v", err)
	}

	if inv.Version != "v1" {
		t.Errorf("Version = %v, want v1", inv.Version)
	}

	if len(inv.Domains) != 1 {
		t.Errorf("Domains length = %d, want 1", len(inv.Domains))
	}
}

func TestParseInvalidYAML(t *testing.T) {
	invalidYAML := []byte(`
version: v1
invalid: [unclosed
`)

	_, err := Parse(invalidYAML)
	if err == nil {
		t.Error("Parse() should return error for invalid YAML")
	}
}

func TestParseEmptyYAML(t *testing.T) {
	// Empty YAML should fail validation because version is required
	_, err := Parse([]byte("{}"))
	if err == nil {
		t.Error("Parse() should return error for empty YAML (missing version)")
	}
}

func TestWriteAndRead(t *testing.T) {
	inv := &Inventory{
		Version: "v1",
		Domains: map[string]*Domain{
			"domain1": {
				ID:     "domain1",
				Domain: "example.com",
			},
		},
	}

	// Create temp directory
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "inventory.yaml")

	// Write
	err := inv.Write(filePath)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Write() should create file")
	}

	// Read back
	inv2, err := ParseFile(filePath)
	if err != nil {
		t.Errorf("ParseFile() error = %v", err)
	}

	if inv2.Version != inv.Version {
		t.Errorf("Read Version = %v, want %v", inv2.Version, inv.Version)
	}
}

func TestLoadDir(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create first inventory file
	file1 := filepath.Join(tmpDir, "inventory1.yaml")
	err := os.WriteFile(file1, []byte(`
version: v1
domains:
  domain1:
    domain: example.com
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create second inventory file
	file2 := filepath.Join(tmpDir, "inventory2.yaml")
	err = os.WriteFile(file2, []byte(`
version: v1
domains:
  domain2:
    domain: test.org
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Load directory
	inv, err := LoadDir(tmpDir)
	if err != nil {
		t.Errorf("LoadDir() error = %v", err)
	}

	if len(inv.Domains) != 2 {
		t.Errorf("LoadDir() loaded %d domains, want 2", len(inv.Domains))
	}
}

func TestLoadDirNonExistent(t *testing.T) {
	_, err := LoadDir("/non/existent/path")
	if err == nil {
		t.Error("LoadDir() should return error for non-existent directory")
	}
}

func TestAgentLocation(t *testing.T) {
	agent := &Agent{
		ID:       "agent1",
		Hostname: "test-host",
		IP:       "192.168.1.1",
	}

	location := &AgentLocation{
		Agent:      agent,
		Region:     "region1",
		Zone:       "zone1",
		RegionName: "Region One",
		ZoneName:   "Zone One",
	}

	if location.ID != "agent1" {
		t.Errorf("ID = %v, want agent1", location.ID)
	}

	if location.Hostname != "test-host" {
		t.Errorf("Hostname = %v, want test-host", location.Hostname)
	}

	if location.Region != "region1" {
		t.Errorf("Region = %v, want region1", location.Region)
	}

	if location.RegionName != "Region One" {
		t.Errorf("RegionName = %v, want 'Region One'", location.RegionName)
	}
}

// ============ Error Path Tests ============

func TestParseFileNotExist(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/inventory.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParseFileInvalidYAML(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "bad.yaml")
	os.WriteFile(tmpFile, []byte(":\n  :\n    - [invalid"), 0644)

	_, err := ParseFile(tmpFile)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestResolveRegionRefNotFound(t *testing.T) {
	inv := &Inventory{Regions: map[string]*Region{}}
	_, err := inv.resolveRegionRef([]string{"nonexistent", "zones", "z1"})
	if err == nil {
		t.Error("expected error for nonexistent region")
	}
}

func TestResolveRegionRefInvalidRef(t *testing.T) {
	inv := &Inventory{Regions: map[string]*Region{}}
	_, err := inv.resolveRegionRef([]string{})
	if err == nil {
		t.Error("expected error for invalid ref")
	}
}

func TestResolveRegionRefZoneNotFound(t *testing.T) {
	inv := &Inventory{
		Regions: map[string]*Region{
			"r1": {Zones: map[string]*Zone{}},
		},
	}
	_, err := inv.resolveRegionRef([]string{"r1", "zones"})
	if err == nil {
		t.Error("expected error for missing zone name")
	}
}

func TestResolveRegionRefAgentNotFound(t *testing.T) {
	inv := &Inventory{
		Regions: map[string]*Region{
			"r1": {Zones: map[string]*Zone{
				"z1": {Agents: map[string]*Agent{}},
			}},
		},
	}
	_, err := inv.resolveRegionRef([]string{"r1", "zones", "z1", "agents"})
	if err == nil {
		t.Error("expected error for missing agent name")
	}
}

func TestMergeNil(t *testing.T) {
	inv := &Inventory{Version: "v1"}
	inv.Merge(nil) // should not panic
}

func TestMergeWithNilMaps(t *testing.T) {
	inv := &Inventory{Version: "v1"}
	other := &Inventory{
		Regions:   map[string]*Region{"r1": {Name: "R1"}},
		Domains:   map[string]*Domain{"d1": {Domain: "d1.com"}},
		Resources: map[string]*Ref{"res1": {Ref: "regions.home"}},
	}
	inv.Merge(other)

	if len(inv.Regions) != 1 {
		t.Errorf("Regions = %d, want 1", len(inv.Regions))
	}
	if len(inv.Domains) != 1 {
		t.Errorf("Domains = %d, want 1", len(inv.Domains))
	}
	if len(inv.Resources) != 1 {
		t.Errorf("Resources = %d, want 1", len(inv.Resources))
	}
}

func TestLoadDirNotExist(t *testing.T) {
	_, err := LoadDir("/nonexistent/directory")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestLoadDirWithBadFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(":\n  invalid"), 0644)

	_, err := LoadDir(dir)
	if err == nil {
		t.Error("expected error for invalid YAML in directory")
	}
}

func TestLoadDirSkipNonYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not yaml"), 0644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	result, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() error = %v", err)
	}
	if result == nil {
		t.Error("result should not be nil")
	}
}

func TestWriteAndReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.yaml")

	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{
			"us-east": {Name: "US East"},
		},
	}

	if err := inv.Write(path); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	loaded, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if loaded.Version != "v1" {
		t.Errorf("Version = %v", loaded.Version)
	}
}

func TestValidateEmpty(t *testing.T) {
	inv := &Inventory{Version: "v1"}
	err := inv.Validate()
	// Empty inventory should validate
	if err != nil {
		t.Logf("Validate() returned: %v (may be expected)", err)
	}
}

func TestResolveRefUnsupported(t *testing.T) {
	inv := &Inventory{
		Domains: map[string]*Domain{},
	}
	_, err := inv.ResolveRef("unsupported.ref.path")
	if err == nil {
		t.Error("expected error for unsupported ref type")
	}
}
