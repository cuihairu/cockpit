package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

func newInventorySyncTestServer(t *testing.T, invCfg *config.InventoryConfig) *Server {
	t.Helper()

	dir := t.TempDir()
	db, err := storage.Open(storage.Config{Path: filepath.Join(dir, "test.db")})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return &Server{
		db: db,
		cfg: &config.Config{
			Server:    &config.ServerConfig{Host: "127.0.0.1", Port: 0},
			Database:  &config.DatabaseConfig{Path: filepath.Join(dir, "test.db")},
			JWT:       &config.JWTConfig{Secret: "test", Expiration: 24 * time.Hour},
			Inventory: invCfg,
		},
	}
}

func TestStartInventorySyncStrictRequiresPath(t *testing.T) {
	s := newInventorySyncTestServer(t, &config.InventoryConfig{
		Watch:  true,
		Strict: true,
	})

	if err := s.startInventorySync(); err == nil {
		t.Fatal("startInventorySync() expected error when strict mode is enabled and path is empty")
	}
}

func TestStartInventorySyncNonStrictIgnoresMissingPath(t *testing.T) {
	s := newInventorySyncTestServer(t, &config.InventoryConfig{
		Watch: true,
	})

	if err := s.startInventorySync(); err != nil {
		t.Fatalf("startInventorySync() unexpected error: %v", err)
	}
	if s.inventorySync != nil {
		t.Fatal("inventorySync should stay nil when startup validation fails in non-strict mode")
	}
}

func TestStartInventorySyncStrictPropagatesInvalidInventory(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	if err := os.WriteFile(invPath, []byte("not: [valid"), 0644); err != nil {
		t.Fatal(err)
	}

	s := newInventorySyncTestServer(t, &config.InventoryConfig{
		Path:   invPath,
		Watch:  true,
		Strict: true,
	})

	if err := s.startInventorySync(); err == nil {
		t.Fatal("startInventorySync() expected error for invalid inventory in strict mode")
	}
}

func TestStartInventorySyncNonStrictKeepsWatcherRunningAfterInvalidInitialLoad(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	if err := os.WriteFile(invPath, []byte("not: [valid"), 0644); err != nil {
		t.Fatal(err)
	}

	s := newInventorySyncTestServer(t, &config.InventoryConfig{
		Path:  invPath,
		Watch: true,
	})
	t.Cleanup(func() {
		if s.inventorySync != nil {
			s.inventorySync.Stop()
		}
	})

	if err := s.startInventorySync(); err != nil {
		t.Fatalf("startInventorySync() unexpected error: %v", err)
	}
	if s.inventorySync == nil {
		t.Fatal("inventorySync should be initialized in non-strict mode even if initial load fails")
	}
}
