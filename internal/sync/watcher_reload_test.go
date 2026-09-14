package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/inventory"
)

func TestStartFailsWhenDirectoryMissing(t *testing.T) {
	w, err := NewWatcher(Config{
		InventoryPath: filepath.Join(t.TempDir(), "missing-dir", "inventory.yaml"),
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	defer w.watcher.Close()

	if err := w.Start(); err == nil {
		t.Error("Start() should fail when watch directory does not exist")
	}
}

func TestWatchLoopReloadsOnWrite(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeEmptyInventory(t, invPath)

	db := testSyncDB(t)
	reloaded := make(chan struct{}, 4)
	w, err := NewWatcher(Config{
		InventoryPath: invPath,
		DB:            db,
		OnReload: func(inv *inventory.Inventory) error {
			reloaded <- struct{}{}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	defer w.Stop()

	if err := w.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	// Drain the signal produced by the initial load
	select {
	case <-reloaded:
	case <-time.After(5 * time.Second):
		t.Fatal("initial load did not invoke reload handler")
	}
	time.Sleep(100 * time.Millisecond)

	// Unrelated files in the same directory must not trigger a reload
	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("noise"), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	// Rewriting the inventory file should trigger a debounced reload
	writeTestInventory(t, invPath)

	select {
	case <-reloaded:
	case <-time.After(5 * time.Second):
		t.Fatal("watchLoop did not reload inventory after file write")
	}

	// The reloaded inventory must have been applied to the database
	agent, err := db.GetAgent("agent-1")
	if err != nil {
		t.Fatalf("GetAgent(agent-1) error = %v", err)
	}
	if agent.Hostname != "web-server" {
		t.Errorf("Hostname = %q, want web-server", agent.Hostname)
	}
}

func TestLoadInventoryApplyErrorWithCanceledContext(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeTestInventory(t, invPath)

	db := testSyncDB(t)
	w, err := NewWatcher(Config{InventoryPath: invPath, DB: db})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	defer w.watcher.Close()

	// First load succeeds and records the mod time
	if err := w.loadInventory(); err != nil {
		t.Fatalf("initial loadInventory() error = %v", err)
	}

	// Cancel the context so the Syncer fails, then change the file
	w.cancel()
	newer := `version: v1
regions:
  r1:
    zones:
      z1:
        agents:
          a9:
            id: a9
            hostname: newer-host
`
	if err := os.WriteFile(invPath, []byte(newer), 0644); err != nil {
		t.Fatal(err)
	}
	// Ensure a distinct mod time
	newTime := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(invPath, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	// loadInventory must not fail even when applying to the DB fails
	if err := w.loadInventory(); err != nil {
		t.Fatalf("loadInventory() should tolerate apply errors, got %v", err)
	}
}

func TestLoadInventoryReloadHandlerError(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeTestInventory(t, invPath)

	w, err := NewWatcher(Config{
		InventoryPath: invPath,
		OnReload: func(inv *inventory.Inventory) error {
			return os.ErrPermission
		},
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	defer w.watcher.Close()

	// Handler errors are logged, not propagated
	if err := w.loadInventory(); err != nil {
		t.Fatalf("loadInventory() should tolerate reload handler errors, got %v", err)
	}
}

func TestCountResult(t *testing.T) {
	if got := countResult(nil); got != 0 {
		t.Errorf("countResult(nil) = %d, want 0", got)
	}
	if got := countResult(&inventory.ResourceResult{Created: 1, Updated: 2, Errors: 3}); got != 6 {
		t.Errorf("countResult() = %d, want 6", got)
	}
}

func TestManagerValidateFileMissing(t *testing.T) {
	m, err := NewManager(filepath.Join(t.TempDir(), "nope.yaml"), nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer m.Stop()

	if err := m.Validate(); err == nil {
		t.Error("Validate() should fail for a missing inventory file")
	}
}

func TestNewManagerWithConfigSetsDB(t *testing.T) {
	db := testSyncDB(t)
	m, err := NewManagerWithConfig(Config{InventoryPath: "test.yaml", DB: db})
	if err != nil {
		t.Fatalf("NewManagerWithConfig() error = %v", err)
	}
	defer m.Stop()
	if m.db != db {
		t.Error("Manager.db should carry the configured DB")
	}
}
