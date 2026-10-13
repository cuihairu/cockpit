package config

// vault.providers 配置节测试（V7）：persona 子节解析、env 覆盖、未知字段
// 严格报错、local-only 部署不产生空节。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func writeVaultConfig(t *testing.T, content string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestLoadVaultProvidersPersona(t *testing.T) {
	t.Setenv("PERSONA_CONNECT_TOKEN", "env-pconn-token")
	cfg, err := writeVaultConfig(t, `
vault:
  provider: persona
  session_ttl: 30m
  providers:
    persona:
      endpoint: http://127.0.0.1:8747
      token: yaml-pconn-token
      timeout: 3s
      session_ttl: 10m
`)
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Vault.Providers.Persona
	if p == nil {
		t.Fatal("persona section missing")
	}
	if p.Endpoint != "http://127.0.0.1:8747" {
		t.Errorf("Endpoint = %q", p.Endpoint)
	}
	if p.Token != "env-pconn-token" {
		t.Errorf("Token = %q, want env override", p.Token)
	}
	if p.Timeout != 3*time.Second {
		t.Errorf("Timeout = %v", p.Timeout)
	}
	if p.SessionTTL != 10*time.Minute {
		t.Errorf("SessionTTL = %v", p.SessionTTL)
	}
	if cfg.Vault.Provider != "persona" || cfg.Vault.SessionTTL != 30*time.Minute {
		t.Errorf("vault section: provider=%q ttl=%v", cfg.Vault.Provider, cfg.Vault.SessionTTL)
	}
}

func TestLoadVaultProvidersUnknownField(t *testing.T) {
	_, err := writeVaultConfig(t, `
vault:
  provider: persona
  providers:
    persona:
      endpoint: http://127.0.0.1:8747
      endpooint: http://typo
`)
	if err == nil || !strings.Contains(err.Error(), "vault.providers") {
		t.Fatalf("want strict unknown-field error, got %v", err)
	}
}

func TestLoadVaultProvidersUnknownProviderKey(t *testing.T) {
	_, err := writeVaultConfig(t, `
vault:
  providers:
    keepass:
      path: /tmp/x.kdbx
`)
	if err == nil || !strings.Contains(err.Error(), "vault.providers") {
		t.Fatalf("want unknown provider key error, got %v", err)
	}
}

func TestLoadVaultProvidersAbsent(t *testing.T) {
	cfg, err := writeVaultConfig(t, `
server:
  port: 9001
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Vault == nil || cfg.Vault.Provider != "local" {
		t.Fatalf("default provider: %+v", cfg.Vault)
	}
	if cfg.Vault.Providers != nil {
		t.Fatalf("local-only config must not gain providers section: %+v", cfg.Vault.Providers)
	}
}

func TestLoadVaultProvidersNullSkip(t *testing.T) {
	// 空值 providers 节合法（严格解码对 null 是 no-op）
	cfg, err := writeVaultConfig(t, `
vault:
  providers:
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Vault.Provider != "local" {
		t.Fatalf("provider: %q", cfg.Vault.Provider)
	}
}

func TestPersonaTokenEnvLazyProviders(t *testing.T) {
	t.Setenv("PERSONA_CONNECT_TOKEN", "env-only")
	cfg := Normalize(&Config{Vault: &VaultConfig{}})
	if cfg.Vault.Providers == nil || cfg.Vault.Providers.Persona == nil {
		t.Fatal("env token must materialize providers.persona")
	}
	if cfg.Vault.Providers.Persona.Token != "env-only" {
		t.Fatalf("Token = %q", cfg.Vault.Providers.Persona.Token)
	}
	// 无 env、无节：不产生空节（yaml 回写不落 persona: {}）
	t.Setenv("PERSONA_CONNECT_TOKEN", "")
	cfg = Normalize(&Config{Vault: &VaultConfig{Provider: "local"}})
	if cfg.Vault.Providers != nil {
		t.Fatalf("providers section must stay nil without env/yaml: %+v", cfg.Vault.Providers)
	}
}

func TestLoadEmptyConfigSkipsStrict(t *testing.T) {
	cfg, err := writeVaultConfig(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Vault.Provider != "local" {
		t.Fatalf("empty file default provider: %q", cfg.Vault.Provider)
	}
}

func TestLoadVaultWithoutProvidersSkipsStrict(t *testing.T) {
	cfg, err := writeVaultConfig(t, "vault:\n  provider: local\n  session_ttl: 1m\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Vault.Providers != nil {
		t.Fatalf("providers must stay nil: %+v", cfg.Vault.Providers)
	}
	if cfg.Vault.SessionTTL != time.Minute {
		t.Fatalf("session_ttl: %v", cfg.Vault.SessionTTL)
	}
}

func TestStrictVaultProvidersBadYAML(t *testing.T) {
	// Load 前置的普通解码会先拦住坏 yaml；该分支为直调防御路径
	if err := strictVaultProviders([]byte("a: [b, ")); err == nil {
		t.Fatal("want yaml error")
	}
}

func TestMappingValue(t *testing.T) {
	var n yaml.Node
	if err := yaml.Unmarshal([]byte("a: 1\nb: 2\n"), &n); err != nil {
		t.Fatal(err)
	}
	root := n.Content[0]
	if mappingValue(root, "a") == nil {
		t.Fatal("existing key must resolve")
	}
	if mappingValue(root, "missing") != nil {
		t.Fatal("missing key must be nil")
	}
	if mappingValue(&yaml.Node{Kind: yaml.ScalarNode}, "a") != nil {
		t.Fatal("non-mapping node must be nil")
	}
	if mappingValue(nil, "a") != nil {
		t.Fatal("nil node must be nil")
	}
}
