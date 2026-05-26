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

func TestLoadDefaults(t *testing.T) {
    // 测试 LoadOrDefault 在文件不存在时返回默认配置
    tmpDir := t.TempDir()
    nonExistentPath := filepath.Join(tmpDir, "does-not-exist.yaml")

    cfg := LoadOrDefault(nonExistentPath)

    // 验证默认配置的具体值
    if cfg.Server == nil {
        t.Fatal("Expected Server config to be non-nil")
    }
    if cfg.Server.Host != "0.0.0.0" {
        t.Errorf("Expected default host '0.0.0.0', got '%s'", cfg.Server.Host)
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
}
