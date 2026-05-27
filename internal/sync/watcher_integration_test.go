package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/inventory"
	"github.com/cuihairu/cockpit/internal/storage"
)

func testSyncDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(storage.Config{Path: dir + "/test.db"})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func writeTestInventory(t *testing.T, path string) {
	t.Helper()
	yaml := `version: v1
metadata:
  name: test
regions:
  us-east:
    id: us-east
    name: US East
    zones:
      us-east-1a:
        id: us-east-1a
        name: Zone 1A
        agents:
          agent-1:
            id: agent-1
            hostname: web-server
            ip: 10.0.0.1
            capabilities:
              - proxy
              - remote-services
            config:
              port: 8080
          agent-2:
            id: agent-2
            hostname: db-server
            ip: 10.0.0.2
domains:
  d1:
    id: d1
    domain: example.com
    provider: cloudflare
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeEmptyInventory(t *testing.T, path string) {
	t.Helper()
	yaml := `version: v1
metadata:
  name: empty
regions: {}
domains: {}
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadInventoryWithFile(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeTestInventory(t, invPath)

	w, _ := NewWatcher(Config{InventoryPath: invPath})
	defer w.watcher.Close()

	err := w.loadInventory()
	if err != nil {
		t.Fatalf("loadInventory() error = %v", err)
	}

	if w.GetLastModTime().IsZero() {
		t.Error("lastModTime should be set after successful load")
	}
}

func TestLoadInventoryWithDB(t *testing.T) {
	db := testSyncDB(t)
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeTestInventory(t, invPath)

	w, _ := NewWatcher(Config{
		InventoryPath: invPath,
		DB:           db,
	})
	defer w.watcher.Close()

	err := w.loadInventory()
	if err != nil {
		t.Fatalf("loadInventory() error = %v", err)
	}

	// Verify agents were synced to DB
	agent1, err := db.GetAgent("agent-1")
	if err != nil {
		t.Fatalf("GetAgent(agent-1) error = %v", err)
	}
	if agent1.Hostname != "web-server" {
		t.Errorf("agent-1.Hostname = %v", agent1.Hostname)
	}

	agent2, err := db.GetAgent("agent-2")
	if err != nil {
		t.Fatalf("GetAgent(agent-2) error = %v", err)
	}
	if agent2.Hostname != "db-server" {
		t.Errorf("agent-2.Hostname = %v", agent2.Hostname)
	}
}

func TestLoadInventorySkipsSameModTime(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeTestInventory(t, invPath)

	w, _ := NewWatcher(Config{InventoryPath: invPath})
	defer w.watcher.Close()

	// First load
	w.loadInventory()
	modTime := w.GetLastModTime()

	// Second load without file change - should skip
	w.loadInventory()
	if !w.GetLastModTime().Equal(modTime) {
		t.Error("Second load with same mod time should be a no-op")
	}
}

func TestLoadInventoryWithReloadCallback(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeTestInventory(t, invPath)

	var callbackInv *inventory.Inventory
	w, _ := NewWatcher(Config{
		InventoryPath: invPath,
		OnReload: func(inv *inventory.Inventory) error {
			callbackInv = inv
			return nil
		},
	})
	defer w.watcher.Close()

	w.loadInventory()

	if callbackInv == nil {
		t.Fatal("OnReload callback should have been called")
	}
	if len(callbackInv.Regions) == 0 {
		t.Error("Callback inventory should have regions")
	}
}

func TestApplyInventoryWithRealDB(t *testing.T) {
	db := testSyncDB(t)
	w, _ := NewWatcher(Config{
		InventoryPath: "test.yaml",
		DB:           db,
	})
	defer w.watcher.Close()

	inv := &inventory.Inventory{
		Version: "v1",
		Regions: map[string]*inventory.Region{
			"local": {
				ID:   "local",
				Name: "Local",
				Zones: map[string]*inventory.Zone{
					"z1": {
						ID:   "z1",
						Name: "Zone 1",
						Agents: map[string]*inventory.Agent{
							"a1": {
								ID:           "a1",
								Hostname:     "host1",
								IP:           "192.168.1.1",
								Capabilities: []string{"proxy"},
							},
						},
					},
				},
			},
		},
	}

	err := w.applyInventory(inv)
	if err != nil {
		t.Fatalf("applyInventory() error = %v", err)
	}

	agent, err := db.GetAgent("a1")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if agent.Hostname != "host1" {
		t.Errorf("Hostname = %v, want host1", agent.Hostname)
	}
	if len(agent.Capabilities) == 0 || agent.Capabilities[0].Type != "proxy" {
		t.Errorf("Capabilities = %v, want [{proxy}]", agent.Capabilities)
	}
}

func TestApplyInventoryAutoDetectCapabilities(t *testing.T) {
	db := testSyncDB(t)
	w, _ := NewWatcher(Config{
		InventoryPath: "test.yaml",
		DB:           db,
	})
	defer w.watcher.Close()

	inv := &inventory.Inventory{
		Version: "v1",
		Regions: map[string]*inventory.Region{
			"r1": {
				Zones: map[string]*inventory.Zone{
					"z1": {
						Agents: map[string]*inventory.Agent{
							"a1": {
								ID:       "a1",
								Hostname: "auto",
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

	w.applyInventory(inv)

	agent, _ := db.GetAgent("a1")
	capTypes := make(map[string]bool)
	for _, c := range agent.Capabilities {
		capTypes[c.Type] = true
	}
	if !capTypes["pve"] {
		t.Error("Should auto-detect pve capability from config")
	}
	if !capTypes["docker"] {
		t.Error("Should auto-detect docker capability from config")
	}
}

func TestCountAgentsWithRealInventory(t *testing.T) {
	inv := &inventory.Inventory{
		Version: "v1",
		Regions: map[string]*inventory.Region{
			"r1": {
				Zones: map[string]*inventory.Zone{
					"z1": {
						Agents: map[string]*inventory.Agent{
							"a1": {ID: "a1"},
							"a2": {ID: "a2"},
						},
					},
					"z2": {
						Agents: map[string]*inventory.Agent{
							"a3": {ID: "a3"},
						},
					},
				},
			},
			"r2": {
				Zones: map[string]*inventory.Zone{
					"z3": {
						Agents: map[string]*inventory.Agent{
							"a4": {ID: "a4"},
						},
					},
				},
			},
		},
	}

	count := countAgents(inv)
	if count != 4 {
		t.Errorf("countAgents() = %d, want 4", count)
	}
}

func TestCountAgentsEmpty(t *testing.T) {
	inv := &inventory.Inventory{
		Version: "v1",
		Regions: map[string]*inventory.Region{
			"r1": {Zones: map[string]*inventory.Zone{}},
		},
	}

	count := countAgents(inv)
	if count != 0 {
		t.Errorf("countAgents() = %d, want 0", count)
	}
}

func TestGetInventoryWithValidFile(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeTestInventory(t, invPath)

	w, _ := NewWatcher(Config{InventoryPath: invPath})
	defer w.watcher.Close()

	inv, err := w.GetInventory()
	if err != nil {
		t.Fatalf("GetInventory() error = %v", err)
	}
	if len(inv.Regions) == 0 {
		t.Error("Inventory should have regions")
	}
}

func TestStartAndStop(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeEmptyInventory(t, invPath)

	w, _ := NewWatcher(Config{InventoryPath: invPath})
	defer w.watcher.Close()

	if err := w.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give watcher time to settle
	time.Sleep(100 * time.Millisecond)

	w.Stop()
}

func TestManagerStartStop(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeEmptyInventory(t, invPath)

	m, err := NewManager(invPath, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("Manager.Start() error = %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	m.Stop()
}

func TestManagerReloadWithFile(t *testing.T) {
	db := testSyncDB(t)
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeTestInventory(t, invPath)

	m, err := NewManager(invPath, db)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer m.Stop()

	err = m.Reload()
	if err != nil {
		t.Fatalf("Manager.Reload() error = %v", err)
	}

	agent, err := db.GetAgent("agent-1")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if agent.Hostname != "web-server" {
		t.Errorf("Hostname = %v, want web-server", agent.Hostname)
	}
}

func TestManagerValidateWithAgents(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")

	yaml := `version: v1
regions:
  r1:
    zones:
      z1:
        agents:
          agent1:
            hostname: test
`
	os.WriteFile(invPath, []byte(yaml), 0644)

	m, _ := NewManager(invPath, nil)
	defer m.Stop()

	err := m.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}
