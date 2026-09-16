package config

import (
	"os"
	"path/filepath"
	"testing"
)

// covWriteConfig 写入临时配置文件并返回路径
func covWriteConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cov-config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestCovLoadInvalidYAML(t *testing.T) {
	path := covWriteConfig(t, "server: [unclosed\n\tnot: valid: yaml: here")

	cfg, err := Load(path)
	if err == nil {
		t.Fatal("Load() should return error for invalid YAML")
	}
	if cfg != nil {
		t.Errorf("Load() should return nil config on error, got %v", cfg)
	}
}

func TestCovLoadMissingFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing-cov.yaml"))
	if err == nil {
		t.Fatal("Load() should return error for missing file")
	}
	if cfg != nil {
		t.Errorf("Load() should return nil config on error, got %v", cfg)
	}
}

func TestCovExpandEnvUnsetVariable(t *testing.T) {
	t.Setenv("COV_CFG_SET_VAR", "hello")
	t.Setenv("COV_CFG_EMPTY_VAR", "")

	content := `
jwt:
  secret: ${COV_CFG_SET_VAR}
database:
  path: ${COV_CFG_UNSET_VAR_XYZ}
server:
  host: ${COV_CFG_EMPTY_VAR}
`
	path := covWriteConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.JWT.Secret != "hello" {
		t.Errorf("JWT.Secret = %q, want expanded %q", cfg.JWT.Secret, "hello")
	}
	// 展开为空串后 applyDefaults 补上默认路径
	if cfg.Database.Path != "./data/cockpit.db" {
		t.Errorf("Database.Path = %q, want default path after empty expansion", cfg.Database.Path)
	}
	// 未设置/为空的变量展开为空串后触发默认值
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q, want default 127.0.0.1", cfg.Server.Host)
	}
}

func TestCovExpandEnvInvalidVarName(t *testing.T) {
	// 非法变量名（以数字开头）与 $VAR 形式不应被展开；合法但未设置的变量展开为空串
	got := expandEnvInContent("a: ${1BAD_NAME} $NOT_BRACE ${OK_NAME_1}")
	want := "a: ${1BAD_NAME} $NOT_BRACE "
	if got != want {
		t.Errorf("expandEnvInContent() = %q, want %q", got, want)
	}
}

func TestCovNormalizeNilConfig(t *testing.T) {
	cfg := Normalize(nil)
	if cfg == nil {
		t.Fatal("Normalize(nil) should return non-nil config")
	}
	if cfg.Server == nil || cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Normalize(nil) should apply server defaults, got %+v", cfg.Server)
	}
	if cfg.JWT == nil || cfg.JWT.Secret != "change-me" {
		t.Errorf("Normalize(nil) should apply JWT defaults, got %+v", cfg.JWT)
	}
	if cfg.JWT.Expiration == 0 {
		t.Error("Normalize(nil) should set default JWT expiration")
	}
	if cfg.Database == nil || cfg.Database.Path == "" {
		t.Errorf("Normalize(nil) should apply database defaults, got %+v", cfg.Database)
	}
	if cfg.DNS == nil || cfg.DNS.Cloudflare == nil {
		t.Error("Normalize(nil) should apply DNS defaults")
	}
}

func TestCovNormalizeKeepsExisting(t *testing.T) {
	in := &Config{
		Server: &ServerConfig{Host: "0.0.0.0", Port: 8080},
		JWT:    &JWTConfig{Secret: "s3cret", Expiration: 7200000000000},
	}
	cfg := Normalize(in)
	if cfg != in {
		t.Fatal("Normalize() should return the same config instance")
	}
	if cfg.Server.Host != "0.0.0.0" || cfg.Server.Port != 8080 {
		t.Errorf("Normalize() should keep existing server values, got %+v", cfg.Server)
	}
	if cfg.JWT.Secret != "s3cret" {
		t.Errorf("Normalize() should keep JWT secret, got %q", cfg.JWT.Secret)
	}
}

func TestCovLoadOrDefaultInvalidYAML(t *testing.T) {
	path := covWriteConfig(t, "\t\tbad: [yaml")
	cfg := LoadOrDefault(path)
	if cfg == nil {
		t.Fatal("LoadOrDefault() should never return nil")
	}
	if cfg.Server == nil || cfg.Server.Port != 9000 {
		t.Errorf("LoadOrDefault() should fall back to defaults, got %+v", cfg.Server)
	}
}
