package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

func TestStatusCmd_EmptyDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cockpit.db")

	cmd := &StatusCmd{DBPath: dbPath}
	if err := cmd.Run(); err != nil {
		t.Fatalf("StatusCmd.Run on empty db failed: %v", err)
	}
}

func TestStatusCmd_DefaultDBPath(t *testing.T) {
	// When DBPath is empty the command uses ./data/cockpit.db. Run it from a
	// temp cwd so we don't touch the real workspace.
	dir := t.TempDir()

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cmd := &StatusCmd{DBPath: ""}
	if err := cmd.Run(); err != nil {
		t.Fatalf("StatusCmd.Run with default db path failed: %v", err)
	}

	// db should have been created under ./data/cockpit.db
	if _, err := os.Stat(filepath.Join("data", "cockpit.db")); err != nil {
		t.Fatalf("default db not created: %v", err)
	}
}

func TestStatusCmd_PopulatedDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cockpit.db")

	db, err := storage.Open(storage.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Seed an agent + a domain so status has something to summarize.
	if err := db.UpsertAgent(&storage.Agent{
		ID:        "agent-status",
		Hostname:  "status-host",
		Status:    "online",
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}); err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}

	if err := db.UpsertDomain(&storage.Domain{
		ID:       "domain-status",
		Domain:   "status.example.com",
		Provider: "manual",
	}); err != nil {
		t.Fatalf("UpsertDomain: %v", err)
	}

	cmd := &StatusCmd{DBPath: dbPath}
	if err := cmd.Run(); err != nil {
		t.Fatalf("StatusCmd.Run on populated db failed: %v", err)
	}
}

func TestStatusCmd_MissingDBErrors(t *testing.T) {
	dir := t.TempDir()
	// Make the would-be parent directory a regular file so storage.Open's
	// MkdirAll cannot create it. This is the most portable way to force Open
	// to fail across SQLite driver implementations.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("seed blocker file: %v", err)
	}
	cmd := &StatusCmd{DBPath: filepath.Join(blocker, "cockpit.db")}
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected error when db cannot be opened")
	}
}

func TestIsRecentlyActive(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"zero", time.Time{}, false},
		{"ancient", now.Add(-24 * time.Hour), false},
		{"one minute ago", now.Add(-1 * time.Minute), true},
		{"just now", now.Add(-5 * time.Second), true},
		{"boundary under 2m", now.Add(-(2*time.Minute - time.Second)), true},
		{"boundary over 2m", now.Add(-(2*time.Minute + time.Second)), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRecentlyActive(tc.t); got != tc.want {
				t.Fatalf("isRecentlyActive(%v) = %v, want %v", tc.t, got, tc.want)
			}
		})
	}
}
