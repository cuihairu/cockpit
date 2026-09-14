package inventory

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGetComputeInstances(t *testing.T) {
	inv := &Inventory{
		ComputeInstances: map[string]*ComputeInstance{
			"vm-1": {ID: "vm-1", Name: "web", Type: "vm"},
			"vm-2": nil,
		},
	}
	instances := inv.GetComputeInstances()
	if len(instances) != 1 {
		t.Errorf("GetComputeInstances() length = %d, want 1", len(instances))
	}
	if instances[0].ID != "vm-1" {
		t.Errorf("GetComputeInstances()[0].ID = %q, want vm-1", instances[0].ID)
	}
}

func TestGetServices(t *testing.T) {
	inv := &Inventory{
		Services: map[string]*Service{
			"svc-1": {ID: "svc-1", Name: "api", Type: "http"},
			"svc-2": nil,
		},
	}
	services := inv.GetServices()
	if len(services) != 1 {
		t.Errorf("GetServices() length = %d, want 1", len(services))
	}
	if services[0].ID != "svc-1" {
		t.Errorf("GetServices()[0].ID = %q, want svc-1", services[0].ID)
	}
}

func TestGetGateways(t *testing.T) {
	inv := &Inventory{
		Gateways: map[string]*Gateway{
			"gw-1": {ID: "gw-1", Name: "edge", Type: "openwrt"},
			"gw-2": nil,
		},
	}
	gateways := inv.GetGateways()
	if len(gateways) != 1 {
		t.Errorf("GetGateways() length = %d, want 1", len(gateways))
	}
	if gateways[0].ID != "gw-1" {
		t.Errorf("GetGateways()[0].ID = %q, want gw-1", gateways[0].ID)
	}
}

func TestGetStorages(t *testing.T) {
	inv := &Inventory{
		Storages: map[string]*Storage{
			"st-1": {ID: "st-1", Name: "data", Type: "nfs"},
			"st-2": nil,
		},
	}
	storages := inv.GetStorages()
	if len(storages) != 1 {
		t.Errorf("GetStorages() length = %d, want 1", len(storages))
	}
	if storages[0].ID != "st-1" {
		t.Errorf("GetStorages()[0].ID = %q, want st-1", storages[0].ID)
	}
}

func TestWriteCreatesNestedDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "inventory.yaml")

	inv := &Inventory{Version: "v1"}
	if err := inv.Write(path); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	parsed, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if parsed.Version != "v1" {
		t.Errorf("round-trip Version = %q, want v1", parsed.Version)
	}
}

func TestSyncerNilResourceEntriesAreSkipped(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)

	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{
			"r1": {
				ID: "r1",
				Zones: map[string]*Zone{
					"z1": {
						ID: "z1",
						Agents: map[string]*Agent{
							"a1": nil,
						},
					},
				},
			},
		},
		Domains:          map[string]*Domain{"d1": nil},
		ComputeInstances: map[string]*ComputeInstance{"vm-1": nil},
		Services:         map[string]*Service{"svc-1": nil},
		Gateways:         map[string]*Gateway{"gw-1": nil},
		Storages:         map[string]*Storage{"st-1": nil},
	}

	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Agents.Created != 0 || result.Domains.Created != 0 ||
		result.ComputeInstances.Created != 0 || result.Services.Created != 0 ||
		result.Gateways.Created != 0 || result.Storages.Created != 0 {
		t.Errorf("nil entries must not create records, got %+v", result)
	}
}

func TestSyncerResourcesWithRegionZoneLabels(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)

	inv := &Inventory{
		Version: "v1",
		Services: map[string]*Service{
			"svc-1": {ID: "svc-1", Name: "api", Type: "http", Region: "us-east", Zone: "z1", URL: "http://10.0.0.1"},
		},
		Gateways: map[string]*Gateway{
			"gw-1": {ID: "gw-1", Name: "edge", Type: "openwrt", Agent: "a1", Region: "us-east", Zone: "z1"},
		},
		Storages: map[string]*Storage{
			"st-1": {ID: "st-1", Name: "data", Type: "nfs", Agent: "a1", Region: "us-east", Zone: "z1", Path: "/mnt"},
		},
		ComputeInstances: map[string]*ComputeInstance{
			"vm-1": {ID: "vm-1", Name: "web", Type: "vm", Agent: "a1", Region: "us-east", Zone: "z1", CPU: 4, Memory: 8192, Disk: 100, IPv4: "10.0.0.5"},
		},
	}

	result, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Services.Created != 1 || result.Gateways.Created != 1 || result.Storages.Created != 1 || result.ComputeInstances.Created != 1 {
		t.Fatalf("unexpected created counts: %+v", result)
	}

	svc, err := db.GetService("svc-1")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if svc.Labels["region"] != "us-east" || svc.Labels["zone"] != "z1" {
		t.Errorf("service labels = %v, want region/zone", svc.Labels)
	}

	gw, err := db.GetGateway("gw-1")
	if err != nil {
		t.Fatalf("GetGateway() error = %v", err)
	}
	if gw.Labels["region"] != "us-east" || gw.Labels["zone"] != "z1" {
		t.Errorf("gateway labels = %v, want region/zone", gw.Labels)
	}

	st, err := db.GetStorage("st-1")
	if err != nil {
		t.Fatalf("GetStorage() error = %v", err)
	}
	if st.Labels["region"] != "us-east" || st.Labels["zone"] != "z1" {
		t.Errorf("storage labels = %v, want region/zone", st.Labels)
	}

	inst, err := db.GetComputeInstance("vm-1")
	if err != nil {
		t.Fatalf("GetComputeInstance() error = %v", err)
	}
	if inst.Region != "us-east" || inst.CPUCores != 4 || inst.IPv4 != "10.0.0.5" {
		t.Errorf("compute instance fields wrong: %+v", inst)
	}

	// Second sync reports updates, not creates
	result2, err := s.Sync(context.Background(), inv)
	if err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}
	if result2.Services.Updated != 1 || result2.Gateways.Updated != 1 || result2.Storages.Updated != 1 || result2.ComputeInstances.Updated != 1 {
		t.Errorf("second sync should report updates, got %+v", result2)
	}
}

func TestDetectCapabilities(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		want   []string
	}{
		{"nil config", nil, nil},
		{"empty config", map[string]any{}, nil},
		{"pve only", map[string]any{"pve": map[string]any{"host": "1.1.1.1"}}, []string{"pve"}},
		{"all known keys", map[string]any{
			"docker":  map[string]any{"host": "unix:///x"},
			"openwrt": map[string]any{"host": "2.2.2.2"},
			"pve":     map[string]any{"host": "3.3.3.3"},
		}, []string{"pve", "docker", "openwrt"}},
		{"unknown keys only", map[string]any{"other": true}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectCapabilities(tt.config)
			if len(got) != len(tt.want) {
				t.Fatalf("detectCapabilities() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("detectCapabilities()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSyncerAgentAutoCapabilities(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)

	inv := &Inventory{
		Version: "v1",
		Regions: map[string]*Region{
			"r1": {
				ID: "r1",
				Zones: map[string]*Zone{
					"z1": {
						ID: "z1",
						Agents: map[string]*Agent{
							"a1": {
								ID:       "a1",
								Hostname: "auto-host",
								Config: map[string]any{
									"pve":    map[string]any{"host": "1.2.3.4"},
									"docker": map[string]any{"host": "unix:///var/run/docker.sock"},
								},
							},
						},
					},
				},
			},
		},
	}

	if _, err := s.Sync(context.Background(), inv); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	agent, err := db.GetAgent("a1")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	capTypes := map[string]bool{}
	for _, c := range agent.Capabilities {
		capTypes[c.Type] = true
	}
	if !capTypes["pve"] || !capTypes["docker"] {
		t.Errorf("expected auto-detected pve+docker capabilities, got %v", agent.Capabilities)
	}
}

func TestSyncerServiceWithAgentReference(t *testing.T) {
	db := testDB(t)
	s := NewSyncer(db)

	inv := &Inventory{
		Version: "v1",
		Services: map[string]*Service{
			"svc-1": {ID: "svc-1", Name: "db", Type: "tcp", Agent: "a1"},
		},
	}

	if _, err := s.Sync(context.Background(), inv); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	svc, err := db.GetService("svc-1")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if svc.AgentID == nil || *svc.AgentID != "a1" {
		t.Errorf("AgentID = %v, want a1", svc.AgentID)
	}
}
