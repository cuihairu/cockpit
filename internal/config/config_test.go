package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")

	content := []byte(`
server:
  host: "0.0.0.0"
  port: 9000

database:
  path: "./data/cockpit.db"

jwt:
  secret: "test-secret"
  expiration: 24h

email:
  enabled: true
  smtp:
    host: "smtp.test.com"
    port: 587
    username: "${SMTP_USER}"
    password: "${SMTP_PASS}"
    from: "test@example.com"
    from_name: "Test"
  base_url: "http://localhost:9000"

notification:
  enabled: true
  herald:
    base_url: "http://localhost:8080"
    timeout: 5s
  events:
    certificate_expired:
      type: "certificate.expired"
      enabled: true
    service_down:
      type: "service.down"
      enabled: true

agent:
  api_key_header: "X-API-Key"

inventory:
  path: "./examples/inventory.yaml"
  watch: true
  strict: true
`)
	if err := os.WriteFile(configPath, content, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// 设置环境变量用于测试
	os.Setenv("SMTP_USER", "test@example.com")
	os.Setenv("SMTP_PASS", "secret")
	defer os.Unsetenv("SMTP_USER")
	defer os.Unsetenv("SMTP_PASS")

	// 加载配置
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 验证基本配置
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Expected host '0.0.0.0', got '%s'", cfg.Server.Host)
	}
	if cfg.Server.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", cfg.Server.Port)
	}

	// 验证环境变量扩展
	if cfg.Email.SMTP.Username != "test@example.com" {
		t.Errorf("Expected username 'test@example.com', got '%s'", cfg.Email.SMTP.Username)
	}
	if cfg.Email.SMTP.Password != "secret" {
		t.Errorf("Expected password 'secret', got '%s'", cfg.Email.SMTP.Password)
	}

	// 验证通知配置
	if !cfg.Notification.Enabled {
		t.Error("Expected notification to be enabled")
	}
	if cfg.Notification.Herald.BaseURL != "http://localhost:8080" {
		t.Errorf("Expected herald base URL 'http://localhost:8080', got '%s'", cfg.Notification.Herald.BaseURL)
	}
	if cfg.Notification.Herald.Timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", cfg.Notification.Herald.Timeout)
	}

	eventCfg, ok := cfg.Notification.Events["certificate_expired"]
	if !ok {
		t.Fatal("Event 'certificate_expired' not found")
	}
	if eventCfg.Type != "certificate.expired" {
		t.Errorf("Expected type 'certificate.expired', got '%s'", eventCfg.Type)
	}

	if cfg.Inventory == nil {
		t.Fatal("Expected inventory config to be non-nil")
	}
	if !cfg.Inventory.Watch {
		t.Error("Expected inventory watch to be enabled")
	}
	if !cfg.Inventory.Strict {
		t.Error("Expected inventory strict to be enabled")
	}
}

func TestLoadDefaults(t *testing.T) {
	// 测试 LoadOrDefault 在文件不存在时返回默认配置
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "does-not-exist.yaml")

	cfg := LoadOrDefault(nonExistentPath)

	// 验证默认配置的具体值
	if cfg.Server == nil {
		t.Fatal("Expected Server config to be non-nil")
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Expected default host '127.0.0.1', got '%s'", cfg.Server.Host)
	}
	if cfg.Server.Port != 9000 {
		t.Errorf("Expected default port 9000, got %d", cfg.Server.Port)
	}

	if cfg.Database == nil {
		t.Fatal("Expected Database config to be non-nil")
	}
	if cfg.Database.Path != "./data/cockpit.db" {
		t.Errorf("Expected default database path './data/cockpit.db', got '%s'", cfg.Database.Path)
	}

	if cfg.JWT == nil {
		t.Fatal("Expected JWT config to be non-nil")
	}
	if cfg.JWT.Secret != "change-me" {
		t.Errorf("Expected default JWT secret 'change-me', got '%s'", cfg.JWT.Secret)
	}
	if cfg.JWT.Expiration != 24*time.Hour {
		t.Errorf("Expected default JWT expiration 24h, got %v", cfg.JWT.Expiration)
	}

	if cfg.Notification == nil {
		t.Fatal("Expected Notification config to be non-nil")
	}
	if cfg.Notification.Enabled {
		t.Error("Expected notification to be disabled by default")
	}

	if cfg.Inventory == nil {
		t.Fatal("Expected Inventory config to be non-nil")
	}
	if cfg.Inventory.Strict {
		t.Error("Expected inventory strict to be disabled by default")
	}
}

func TestLoadAppliesDefaultsToPartialConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "partial.yaml")

	content := []byte(`
server:
  host: "0.0.0.0"
`)
	if err := os.WriteFile(configPath, content, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Server == nil || cfg.Server.Host != "0.0.0.0" || cfg.Server.Port != 9000 {
		t.Fatalf("server defaults not applied correctly: %#v", cfg.Server)
	}
	if cfg.Database == nil || cfg.Database.Path != "./data/cockpit.db" {
		t.Fatalf("database defaults not applied correctly: %#v", cfg.Database)
	}
	if cfg.JWT == nil || cfg.JWT.Secret == "" || cfg.JWT.Expiration != 24*time.Hour {
		t.Fatalf("jwt defaults not applied correctly: %#v", cfg.JWT)
	}
	if cfg.Agent == nil || cfg.Agent.APIKeyHeader != "X-API-Key" {
		t.Fatalf("agent defaults not applied correctly: %#v", cfg.Agent)
	}
	if cfg.Inventory == nil {
		t.Fatal("Expected Inventory config to be non-nil")
	}
	if cfg.RemoteControl == nil {
		t.Fatal("Expected RemoteControl config to be non-nil")
	}
}

func TestLoadRemoteControlEgressPolicies(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "egress.yaml")

	content := []byte(`
remote_control:
  allow_arbitrary_target: false
  allowed_targets:
    - "10.0.0.10"
  egress:
    - agent_id: "office-agent"
      allowed_targets:
        - "192.168.10.0/24"
        - "db.internal"
      allowed_ports:
        - 22
        - 3389
`)
	if err := os.WriteFile(configPath, content, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.RemoteControl == nil {
		t.Fatal("Expected RemoteControl config to be non-nil")
	}
	if len(cfg.RemoteControl.EgressPolicies) != 1 {
		t.Fatalf("Expected 1 egress policy, got %d", len(cfg.RemoteControl.EgressPolicies))
	}

	policy := cfg.RemoteControl.EgressPolicies[0]
	if policy.AgentID != "office-agent" {
		t.Fatalf("policy.AgentID = %q, want office-agent", policy.AgentID)
	}
	if len(policy.AllowedTargets) != 2 {
		t.Fatalf("policy.AllowedTargets = %#v, want 2 entries", policy.AllowedTargets)
	}
	if len(policy.AllowedPorts) != 2 || policy.AllowedPorts[0] != 22 || policy.AllowedPorts[1] != 3389 {
		t.Fatalf("policy.AllowedPorts = %#v, want [22 3389]", policy.AllowedPorts)
	}
}

func TestDNSCloudflareEnvOverride(t *testing.T) {
	// yaml 里的 token 应被 env 覆盖
	t.Setenv("CLOUDFLARE_API_TOKEN", "env-token")
	cfg := Normalize(&Config{DNS: &DNSConfig{Cloudflare: &CloudflareDNSConfig{APIToken: "yaml-token"}}})
	if cfg.DNS.Cloudflare.APIToken != "env-token" {
		t.Fatalf("APIToken = %q, want env-token", cfg.DNS.Cloudflare.APIToken)
	}
	// env 未设置时保留 yaml 值
	t.Setenv("CLOUDFLARE_API_TOKEN", "")
	cfg = Normalize(&Config{DNS: &DNSConfig{Cloudflare: &CloudflareDNSConfig{APIToken: "yaml-token"}}})
	if cfg.DNS.Cloudflare.APIToken != "yaml-token" {
		t.Fatalf("APIToken = %q, want yaml-token", cfg.DNS.Cloudflare.APIToken)
	}
	// 完全未配置时子结构补齐为空（不 panic）
	cfg = Normalize(&Config{})
	if cfg.DNS == nil || cfg.DNS.Cloudflare == nil || cfg.DNS.Cloudflare.APIToken != "" {
		t.Fatalf("default DNS config = %+v", cfg.DNS)
	}
}

func TestDNSProvidersEnvOverride(t *testing.T) {
	// DNSPod/阿里云凭据 env 优先（ACME D13），与 Cloudflare 同一惯例
	t.Setenv("DNSPOD_LOGIN_TOKEN", "env-id,env-token")
	t.Setenv("ALIYUN_ACCESS_KEY", "env-ak")
	t.Setenv("ALIYUN_ACCESS_KEY_SECRET", "env-sk")
	cfg := Normalize(&Config{DNS: &DNSConfig{
		Provider: "dnspod",
		DNSPod:   &DNSPodConfig{LoginToken: "yaml-token"},
		AliDNS:   &AliDNSConfig{AccessKey: "yaml-ak", SecretKey: "yaml-sk"},
	}})
	if cfg.DNS.Provider != "dnspod" {
		t.Errorf("Provider = %q, want dnspod", cfg.DNS.Provider)
	}
	if cfg.DNS.DNSPod.LoginToken != "env-id,env-token" {
		t.Errorf("DNSPod.LoginToken = %q, want env value", cfg.DNS.DNSPod.LoginToken)
	}
	if cfg.DNS.AliDNS.AccessKey != "env-ak" || cfg.DNS.AliDNS.SecretKey != "env-sk" {
		t.Errorf("AliDNS = %+v, want env values", cfg.DNS.AliDNS)
	}

	// env 未设置时保留 yaml 值
	t.Setenv("DNSPOD_LOGIN_TOKEN", "")
	t.Setenv("ALIYUN_ACCESS_KEY", "")
	t.Setenv("ALIYUN_ACCESS_KEY_SECRET", "")
	cfg = Normalize(&Config{DNS: &DNSConfig{
		DNSPod: &DNSPodConfig{LoginToken: "yaml-token"},
		AliDNS: &AliDNSConfig{AccessKey: "yaml-ak", SecretKey: "yaml-sk"},
	}})
	if cfg.DNS.DNSPod.LoginToken != "yaml-token" ||
		cfg.DNS.AliDNS.AccessKey != "yaml-ak" || cfg.DNS.AliDNS.SecretKey != "yaml-sk" {
		t.Fatalf("yaml values not kept: %+v", cfg.DNS)
	}

	// 完全未配置时子结构补齐为空（不 panic）
	cfg = Normalize(&Config{})
	if cfg.DNS == nil || cfg.DNS.DNSPod == nil || cfg.DNS.AliDNS == nil || cfg.DNS.Provider != "" {
		t.Fatalf("default DNS config = %+v", cfg.DNS)
	}
}
