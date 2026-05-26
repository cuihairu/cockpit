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
}

func TestExpandEnv(t *testing.T) {
    os.Setenv("TEST_VAR", "expanded")
    defer os.Unsetenv("TEST_VAR")

    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"simple expansion", "${TEST_VAR}", "expanded"},
        {"multiple vars", "${TEST_VAR}-${TEST_VAR}", "expanded-expanded"},
        {"no vars", "plain-text", "plain-text"},
        {"missing var", "${MISSING_VAR}", ""},
        {"mixed", "prefix-${TEST_VAR}-suffix", "prefix-expanded-suffix"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := expandEnv(tt.input)
            if result != tt.expected {
                t.Errorf("expandEnv(%q) = %q, want %q", tt.input, result, tt.expected)
            }
        })
    }
}

func TestLoadDefaults(t *testing.T) {
    // 测试默认值
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "minimal.yaml")

    content := []byte(`
server:
  port: 9000
`)
    if err := os.WriteFile(configPath, content, 0644); err != nil {
        t.Fatalf("Failed to write config: %v", err)
    }

    cfg, err := Load(configPath)
    if err != nil {
        t.Fatalf("Failed to load config: %v", err)
    }

    // 检查默认值
    if cfg.Server.Host != "" {
        t.Errorf("Expected empty host as default, got '%s'", cfg.Server.Host)
    }
}
