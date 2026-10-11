package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// cov_vault_test.go 覆盖 vault 配置节（vault-design.md V4/V5/V7）：
// yaml 解码、默认 provider=local。

func TestVaultConfigYamlDecode(t *testing.T) {
	var c Config
	src := `
vault:
  provider: local
  session_ttl: 5m
`
	if err := yaml.Unmarshal([]byte(src), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Vault == nil {
		t.Fatal("vault section must decode")
	}
	if c.Vault.Provider != "local" {
		t.Errorf("provider = %q", c.Vault.Provider)
	}
	if c.Vault.SessionTTL != 5*time.Minute {
		t.Errorf("session_ttl = %v", c.Vault.SessionTTL)
	}
}

func TestVaultConfigDefaultLocal(t *testing.T) {
	cfg := Normalize(&Config{})
	if cfg.Vault == nil {
		t.Fatal("Normalize must materialize vault config")
	}
	if cfg.Vault.Provider != "local" {
		t.Fatalf("default provider = %q, want local", cfg.Vault.Provider)
	}
	if cfg.Vault.SessionTTL != 0 {
		t.Fatalf("default ttl = %v, want 0", cfg.Vault.SessionTTL)
	}
}
