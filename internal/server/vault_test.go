package server

import (
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/cuihairu/cockpit/internal/vault"
)

const vaultTestKey = "vault-server-test-key-0123456789abcdef!"

func TestInitVaultDefaultLocal(t *testing.T) {
	vault.ApplyKey(vaultTestKey)
	db, err := storage.Open(storage.Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	p, err := initVault(nil, db, nil)
	if err != nil {
		t.Fatalf("initVault(nil): %v", err)
	}
	if p.Name() != "local" {
		t.Fatalf("name = %q", p.Name())
	}
	// provider 可用：读写回环
	if err := p.Write("nas/t1", &vault.Entry{Kind: "pair", Username: "u", Value: "v"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := p.Read("nas/t1")
	if err != nil || got.Value != "v" || got.Username != "u" {
		t.Fatalf("roundtrip: %v %+v", err, got)
	}
}

func TestInitVaultExplicitConfig(t *testing.T) {
	vault.ApplyKey(vaultTestKey)
	db, err := storage.Open(storage.Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	p, err := initVault(&config.VaultConfig{Provider: "local", SessionTTL: 5 * time.Minute}, db, nil)
	if err != nil || p.Name() != "local" {
		t.Fatalf("initVault(local): %v %v", err, p)
	}
}

func TestInitVaultUnknownProviderFails(t *testing.T) {
	db, err := storage.Open(storage.Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = initVault(&config.VaultConfig{Provider: "persona-cloud"}, db, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("want unknown-provider error, got %v", err)
	}
}

func TestInitVaultAuditHookWritesEntries(t *testing.T) {
	vault.ApplyKey(vaultTestKey)
	db, err := storage.Open(storage.Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	logger := audit.NewLogger(db)

	p, err := initVault(&config.VaultConfig{}, db, logger)
	if err != nil {
		t.Fatalf("initVault: %v", err)
	}
	const secret = "hook-plain-XYZ"
	if err := p.Write("nas/a", &vault.Entry{Kind: "pair", Username: "hu", Value: secret}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := p.Read("nas/missing"); err != vault.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	var successCount, failCount int64
	if err := db.Gorm().Table("audit_logs").Where("action = ? AND status = ?", "vault.write", "success").Count(&successCount).Error; err != nil {
		t.Fatalf("count success: %v", err)
	}
	if err := db.Gorm().Table("audit_logs").Where("action = ? AND status = ?", "vault.read", "failure").Count(&failCount).Error; err != nil {
		t.Fatalf("count failure: %v", err)
	}
	if successCount != 1 || failCount != 1 {
		t.Fatalf("audit rows: success=%d fail=%d", successCount, failCount)
	}
	var details string
	db.Gorm().Table("audit_logs").Where("action = ?", "vault.write").Select("details").Scan(&details)
	if !strings.Contains(details, "nas/a") || strings.Contains(details, secret) {
		t.Fatalf("audit details must carry path but not plaintext: %q", details)
	}
}

func TestNewServerWiresVaultProvider(t *testing.T) {
	s := NewServer(&config.Config{})
	if s.vaultProvider == nil {
		t.Fatal("NewServer must wire vault provider")
	}
	if s.vaultProvider.Name() != "local" {
		t.Fatalf("provider = %q", s.vaultProvider.Name())
	}
}

func TestNewServerUnknownVaultProviderFatals(t *testing.T) {
	t.Chdir(t.TempDir())
	covFinalInterceptFatal(t)
	covFinalWantFatal(t, "Failed to init vault", func() {
		NewServer(&config.Config{
			Database: &config.DatabaseConfig{Path: "./vault-fatal-test.db"},
			Vault:    &config.VaultConfig{Provider: "no-such-provider"},
		})
	})
}
