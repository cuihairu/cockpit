package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cuihairu/cockpit/internal/inventory"
	"github.com/cuihairu/cockpit/internal/storage"
)

// minimalInventoryYAML returns a v1 inventory with one agent + one compute instance
// so that Sync actually counts resources.
func minimalInventoryYAML(t *testing.T, path string) {
	t.Helper()
	const yaml = `version: "v1"
metadata:
  name: cli-test
regions:
  home:
    name: Home
    zones:
      dc:
        name: DC
        agents:
          agent-1:
            hostname: agent-1.local
            ip: 127.0.0.1
computeInstances:
  vm-1:
    name: VM 1
    type: vm
    agent: agent-1
    region: home
    zone: dc
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatalf("write inventory: %v", err)
	}
}

func openTestDB(t *testing.T, dir string) *storage.DB {
	t.Helper()
	dbPath := filepath.Join(dir, "test.db")
	db, err := storage.Open(storage.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSyncCmd_SyncsInventoryToDB(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	minimalInventoryYAML(t, invPath)
	dbPath := filepath.Join(dir, "cockpit.db")

	cmd := &SyncCmd{Inventory: invPath, DBPath: dbPath}
	if err := cmd.Run(); err != nil {
		t.Fatalf("SyncCmd.Run failed: %v", err)
	}

	db, err := storage.Open(storage.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db.Close()

	agents, err := db.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent synced, got %d", len(agents))
	}

	compute, err := db.ListComputeInstances(nil)
	if err != nil {
		t.Fatalf("ListComputeInstances: %v", err)
	}
	if len(compute) != 1 {
		t.Fatalf("expected 1 compute instance synced, got %d", len(compute))
	}
}

func TestSyncCmd_MissingInventoryPathErrors(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cockpit.db")

	// No Inventory, no Config — sync has nothing to read.
	cmd := &SyncCmd{DBPath: dbPath}
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected error when inventory path is missing")
	}
}

func TestSyncCmd_BadInventoryErrors(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "bad.yaml")
	// Not valid YAML for our schema (missing required version).
	if err := os.WriteFile(invPath, []byte("::: not yaml ::: "), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	dbPath := filepath.Join(dir, "cockpit.db")

	cmd := &SyncCmd{Inventory: invPath, DBPath: dbPath}
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected error for unparseable inventory")
	}
}

func TestSyncCmd_MissingDBPathDefaults(t *testing.T) {
	// When DBPath is empty the command falls back to cfg.Database.Path, and
	// ultimately to ./data/cockpit.db. Drive this through an explicit config
	// so we can assert the file lands where we expect.
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	minimalInventoryYAML(t, invPath)

	dbPath := filepath.Join(dir, "data", "cockpit.db")
	cfgPath := filepath.Join(dir, "cockpit.yaml")
	cfg := []byte("database:\n  path: " + dbPath + "\n")
	if err := os.WriteFile(cfgPath, cfg, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := &SyncCmd{Config: cfgPath, Inventory: invPath}
	if err := cmd.Run(); err != nil {
		t.Fatalf("SyncCmd.Run failed: %v", err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("db not created at config-specified path %s: %v", dbPath, err)
	}
}

func TestLoadConfigOrDefault_LoadsFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cockpit.yaml")
	cfg := []byte("database:\n  path: ./somewhere.db\n")
	if err := os.WriteFile(cfgPath, cfg, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := loadConfigOrDefault(cfgPath)
	if got.Database == nil || got.Database.Path != "./somewhere.db" {
		t.Fatalf("config not loaded, got %+v", got.Database)
	}
}

func TestLoadConfigOrDefault_EmptyPathReturnsDefault(t *testing.T) {
	got := loadConfigOrDefault("")
	// Should never return nil — defaults are applied.
	if got == nil {
		t.Fatalf("expected non-nil default config")
	}
}

func TestLoadConfigOrDefault_BadFileFallsBack(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(badPath, []byte("::: not yaml :::"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := loadConfigOrDefault(badPath)
	if got == nil {
		t.Fatalf("expected fallback default config, got nil")
	}
}

func TestHasSyncErrors(t *testing.T) {
	cases := []struct {
		name string
		res  *inventory.SyncResult
		want bool
	}{
		{
			name: "nil result",
			res:  nil,
			want: false,
		},
		{
			name: "no errors",
			res: &inventory.SyncResult{
				Agents: &inventory.ResourceResult{Created: 1},
			},
			want: false,
		},
		{
			name: "agent errors",
			res: &inventory.SyncResult{
				Agents: &inventory.ResourceResult{Errors: 2},
			},
			want: true,
		},
		{
			name: "domain errors",
			res: &inventory.SyncResult{
				Domains: &inventory.ResourceResult{Errors: 1},
			},
			want: true,
		},
		{
			name: "compute errors",
			res: &inventory.SyncResult{
				ComputeInstances: &inventory.ResourceResult{Errors: 1},
			},
			want: true,
		},
		{
			name: "storage errors",
			res: &inventory.SyncResult{
				Storages: &inventory.ResourceResult{Errors: 1},
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasSyncErrors(tc.res); got != tc.want {
				t.Fatalf("hasSyncErrors(%+v) = %v, want %v", tc.res, got, tc.want)
			}
		})
	}
}
