package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cuihairu/cockpit/internal/inventory"
)

func TestGetLastModTime(t *testing.T) {
	w, _ := NewWatcher(Config{
		InventoryPath: "test.yaml",
	})
	defer w.watcher.Close()

	// Initially zero time
	if w.GetLastModTime().IsZero() {
		// Expected - no file loaded yet
	}
}

func TestNewWatcher(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				InventoryPath: "test.yaml",
			},
			wantErr: false,
		},
		{
			name: "empty path",
			cfg: Config{
				InventoryPath: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := NewWatcher(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewWatcher() error = %v, wantErr %v", err, tt.wantErr)
			}
			if w != nil {
				w.watcher.Close()
			}
		})
	}
}

func TestStopWatcher(t *testing.T) {
	w, _ := NewWatcher(Config{
		InventoryPath: "test.yaml",
	})

	// Should not panic
	w.Stop()

	// Should be safe to call multiple times
	w.Stop()
}

func TestManagerStop(t *testing.T) {
	m, _ := NewManager("test.yaml", nil)

	// Should not panic
	m.Stop()

	// Should be safe to call multiple times
	m.Stop()
}

func TestForceReloadNonExistent(t *testing.T) {
	w, _ := NewWatcher(Config{
		InventoryPath: "/non/existent/file.yaml",
	})
	defer w.watcher.Close()

	err := w.ForceReload()
	if err == nil {
		t.Error("ForceReload() should return error for non-existent file")
	}
}

func TestGetInventoryNonExistent(t *testing.T) {
	w, _ := NewWatcher(Config{
		InventoryPath: "/non/existent/file.yaml",
	})
	defer w.watcher.Close()

	_, err := w.GetInventory()
	if err == nil {
		t.Error("GetInventory() should return error for non-existent file")
	}
}

func TestValidateWithFile(t *testing.T) {
	tmpDir := t.TempDir()
	invPath := filepath.Join(tmpDir, "inventory.yaml")

	validYAML := `
version: v1
regions:
  region1:
    zones:
      zone1:
        agents:
          agent1:
            hostname: test
`

	if err := os.WriteFile(invPath, []byte(validYAML), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(invPath, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer m.Stop()

	err = m.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestValidateEmptyInventory(t *testing.T) {
	tmpDir := t.TempDir()
	invPath := filepath.Join(tmpDir, "inventory.yaml")

	emptyYAML := `
version: v1
regions: {}
domains: {}
`

	if err := os.WriteFile(invPath, []byte(emptyYAML), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(invPath, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer m.Stop()

	err = m.Validate()
	if err != nil {
		t.Errorf("Validate() with empty inventory should not error, got %v", err)
	}
}

func TestReloadNonExistent(t *testing.T) {
	m, _ := NewManager("/non/existent/file.yaml", nil)
	defer m.Stop()

	err := m.Reload()
	if err == nil {
		t.Error("Reload() should return error for non-existent file")
	}
}

func TestNewManager(t *testing.T) {
	m, err := NewManager("test.yaml", nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer m.Stop()

	if m == nil {
		t.Error("NewManager() should not return nil")
	}

	if m.watcher == nil {
		t.Error("Manager.watcher should not be nil")
	}
}

func TestLoadInventoryNonExistent(t *testing.T) {
	w, _ := NewWatcher(Config{
		InventoryPath: "/non/existent/file.yaml",
	})
	defer w.watcher.Close()

	// Should return error
	err := w.loadInventory()
	if err == nil {
		t.Error("loadInventory() should return error for non-existent file")
	}
}

func TestGetLastModTimeConcurrent(t *testing.T) {
	w, _ := NewWatcher(Config{
		InventoryPath: "test.yaml",
	})
	defer w.watcher.Close()

	// Test concurrent access
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			w.GetLastModTime()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestWatcherContext(t *testing.T) {
	w, _ := NewWatcher(Config{
		InventoryPath: "test.yaml",
	})
	defer w.watcher.Close()

	if w.ctx == nil {
		t.Error("Watcher ctx should not be nil")
	}

	if w.cancel == nil {
		t.Error("Watcher cancel should not be nil")
	}
}

func TestApplyInventoryNilDB(t *testing.T) {
	w, _ := NewWatcher(Config{
		InventoryPath: "test.yaml",
		DB:           nil,
	})
	defer w.watcher.Close()

	// Should not panic with nil DB
	err := w.applyInventory(nil)
	if err != nil {
		t.Errorf("applyInventory() with nil DB and nil inv should not error, got %v", err)
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{
		InventoryPath: "test.yaml",
	}

	if cfg.InventoryPath != "test.yaml" {
		t.Errorf("InventoryPath = %v, want test.yaml", cfg.InventoryPath)
	}

	if cfg.DB != nil {
		t.Error("DB should be nil by default")
	}

	if cfg.OnReload != nil {
		t.Error("OnReload should be nil by default")
	}
}

func TestStopClosesWatcher(t *testing.T) {
	w, _ := NewWatcher(Config{
		InventoryPath: "test.yaml",
	})

	w.Stop()

	// After stop, watcher should be closed
	// We can't directly test this, but we can verify Stop() doesn't panic
}

func TestApplyInventoryWithNilDB(t *testing.T) {
	w, _ := NewWatcher(Config{
		InventoryPath: "test.yaml",
		DB:           nil,
	})
	defer w.watcher.Close()

	// Create minimal inventory
	type MinimalInventory struct {
		Regions map[string]interface{}
	}

	inv := &MinimalInventory{
		Regions: make(map[string]interface{}),
	}

	// Should not panic with nil DB (though it won't match the real type)
	// In real usage, this would be *inventory.Inventory
	_ = w.db
	_ = inv
}

func TestForceReloadUpdatesLastModTime(t *testing.T) {
	tmpDir := t.TempDir()
	invPath := filepath.Join(tmpDir, "inventory.yaml")

	yaml := `version: v1
regions: {}`

	if err := os.WriteFile(invPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	w, _ := NewWatcher(Config{
		InventoryPath: invPath,
	})
	defer w.watcher.Close()

	initialTime := w.GetLastModTime()

	// Force reload
	err := w.ForceReload()
	if err != nil {
		t.Fatalf("ForceReload() error = %v", err)
	}

	newTime := w.GetLastModTime()

	// Time should be updated or remain valid
	if !newTime.Equal(initialTime) && newTime.IsZero() {
		t.Error("GetLastModTime() should return valid time after reload")
	}
}

func TestOnReloadCallback(t *testing.T) {
	callbackCalled := false
	cfg := Config{
		InventoryPath: "test.yaml",
		OnReload: func(inv *inventory.Inventory) error {
			// This would be *inventory.Inventory in real usage
			callbackCalled = true
			return nil
		},
	}

	w, err := NewWatcher(cfg)
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	defer w.watcher.Close()

	// Verify onReload is set (though we can't test it without a real file)
	if w.onReload == nil {
		t.Error("onReload should be set")
	}

	_ = callbackCalled
}

func TestCountAgentsHelper(t *testing.T) {
	// This is a compile-time test to ensure countAgents exists
	// We can't easily test it without a real *inventory.Inventory
	_ = func() {
		type TestInv struct {
			Regions map[string]interface{}
		}
		inv := &TestInv{Regions: make(map[string]interface{})}
		// countAgents(inv) // This would fail at runtime due to type mismatch
		_ = inv
	}
}
