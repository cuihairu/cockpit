package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
)

func TestGenerateVerificationCode(t *testing.T) {
	code := generateVerificationCode()
	if len(code) != 6 {
		t.Errorf("code length = %d, want 6", len(code))
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			t.Errorf("code contains non-digit: %c", c)
		}
	}
}

func TestGenerateVerificationCodeUniqueness(t *testing.T) {
	codes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code := generateVerificationCode()
		codes[code] = true
	}
	// 100 codes should produce at least 90 unique ones
	if len(codes) < 90 {
		t.Errorf("expected at least 90 unique codes, got %d", len(codes))
	}
}

// resetStore 清空令牌存储。GenerateResetToken 会起后台清理协程，
// 上一测试的 goroutine 可能存活到下一测试，访问必须持锁。
func resetStore() {
	resetTokenMu.Lock()
	defer resetTokenMu.Unlock()
	resetTokenStore = make(map[string]*ResetTokenData)
}

func seedResetToken(token string, data *ResetTokenData) {
	resetTokenMu.Lock()
	defer resetTokenMu.Unlock()
	resetTokenStore[token] = data
}

func storedResetTokenExists(token string) bool {
	resetTokenMu.Lock()
	defer resetTokenMu.Unlock()
	_, ok := resetTokenStore[token]
	return ok
}

func TestGenerateResetToken(t *testing.T) {
	// Clear store
	resetStore()

	token, code, err := GenerateResetToken("user-1", "test@example.com")
	if err != nil {
		t.Fatalf("GenerateResetToken() error = %v", err)
	}
	if token == "" {
		t.Error("token should not be empty")
	}
	if len(token) != 64 {
		t.Errorf("token length = %d, want 64", len(token))
	}
	if len(code) != 6 {
		t.Errorf("code length = %d, want 6", len(code))
	}
}

func TestValidateResetToken(t *testing.T) {
	resetStore()

	token, _, _ := GenerateResetToken("user-1", "test@example.com")

	data, err := ValidateResetToken(token)
	if err != nil {
		t.Fatalf("ValidateResetToken() error = %v", err)
	}
	if data.UserID != "user-1" {
		t.Errorf("UserID = %v, want user-1", data.UserID)
	}
	if data.Email != "test@example.com" {
		t.Errorf("Email = %v, want test@example.com", data.Email)
	}
}

func TestValidateResetTokenNotFound(t *testing.T) {
	resetStore()

	_, err := ValidateResetToken("nonexistent")
	if err != ErrResetTokenInvalid {
		t.Errorf("error = %v, want ErrResetTokenInvalid", err)
	}
}

func TestValidateResetTokenExpired(t *testing.T) {
	resetStore()

	// Manually insert expired token
	expired := time.Now().Add(-1 * time.Hour)
	seedResetToken("expired-token", &ResetTokenData{
		UserID:    "user-1",
		Email:     "test@example.com",
		Code:      "123456",
		ExpiresAt: expired,
	})

	_, err := ValidateResetToken("expired-token")
	if err != ErrResetTokenInvalid {
		t.Errorf("error = %v, want ErrResetTokenInvalid", err)
	}
}

func TestValidateResetCode(t *testing.T) {
	resetStore()

	token, code, _ := GenerateResetToken("user-1", "test@example.com")

	data, err := ValidateResetCode(token, code)
	if err != nil {
		t.Fatalf("ValidateResetCode() error = %v", err)
	}
	if data.UserID != "user-1" {
		t.Errorf("UserID = %v, want user-1", data.UserID)
	}
}

func TestValidateResetCodeWrongCode(t *testing.T) {
	resetStore()

	token, _, _ := GenerateResetToken("user-1", "test@example.com")

	_, err := ValidateResetCode(token, "000000")
	if err != ErrResetTokenInvalid {
		t.Errorf("error = %v, want ErrResetTokenInvalid", err)
	}
}

func TestValidateResetCodeInvalidToken(t *testing.T) {
	resetStore()

	_, err := ValidateResetCode("bad-token", "123456")
	if err != ErrResetTokenInvalid {
		t.Errorf("error = %v, want ErrResetTokenInvalid", err)
	}
}

func TestConsumeResetToken(t *testing.T) {
	resetStore()

	token, _, _ := GenerateResetToken("user-1", "test@example.com")

	if !ConsumeResetToken(token) {
		t.Error("ConsumeResetToken() should return true")
	}
	// Second consumption should fail
	if ConsumeResetToken(token) {
		t.Error("ConsumeResetToken() should return false for consumed token")
	}
}

func TestConsumeResetTokenExpired(t *testing.T) {
	resetStore()

	expired := time.Now().Add(-1 * time.Hour)
	seedResetToken("expired", &ResetTokenData{
		UserID:    "user-1",
		ExpiresAt: expired,
	})

	if ConsumeResetToken("expired") {
		t.Error("ConsumeResetToken() should return false for expired token")
	}
}

func TestConsumeResetTokenNotFound(t *testing.T) {
	resetStore()

	if ConsumeResetToken("nonexistent") {
		t.Error("ConsumeResetToken() should return false for nonexistent token")
	}
}

func TestCleanupExpiredTokens(t *testing.T) {
	resetStore()

	expired := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	seedResetToken("expired", &ResetTokenData{ExpiresAt: expired})
	seedResetToken("valid", &ResetTokenData{ExpiresAt: future})

	cleanupExpiredTokens()

	if storedResetTokenExists("expired") {
		t.Error("expired token should be cleaned up")
	}
	if !storedResetTokenExists("valid") {
		t.Error("valid token should remain")
	}
}

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"test@example.com", "t***t@example.com"},
		{"admin@example.com", "a***n@example.com"},
		{"ab@x.com", "ab@x.com"},       // too short
		{"invalid", "invalid"},          // no @
		{"a@b@c.com", "a@b@c.com"},     // multiple @ -> not 2 parts
	}
	for _, tt := range tests {
		got := MaskEmail(tt.input)
		if got != tt.want {
			t.Errorf("MaskEmail(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetEmailConfigNotSet(t *testing.T) {
	emailConfig = nil
	cfg := GetEmailConfig()
	if cfg != nil {
		t.Error("GetEmailConfig() should return nil when not configured")
	}
}

func TestGetEmailConfigFromSet(t *testing.T) {
	SetEmailConfig(&config.EmailConfig{
		Enabled: true,
		SMTP: &config.SMTPConfig{
			Host:     "smtp.test.com",
			Port:     587,
			Username: "test@test.com",
			Password: "pass",
		},
	})
	defer SetEmailConfig(nil)

	cfg := GetEmailConfig()
	if cfg == nil {
		t.Fatal("GetEmailConfig() should return config")
	}
	if cfg.SMTP.Host != "smtp.test.com" {
		t.Errorf("Host = %v, want smtp.test.com", cfg.SMTP.Host)
	}
}

func TestGetEmailConfigFromSetNormalizesBaseURL(t *testing.T) {
	SetEmailConfig(&config.EmailConfig{
		Enabled: true,
		SMTP: &config.SMTPConfig{
			Host:     "smtp.test.com",
			Port:     587,
			Username: "test@test.com",
			Password: "pass",
		},
		BaseURL: "http://example.com/",
	})
	defer SetEmailConfig(nil)

	cfg := GetEmailConfig()
	if cfg == nil {
		t.Fatal("GetEmailConfig() should return config")
	}
	if cfg.BaseURL != "http://example.com" {
		t.Errorf("BaseURL = %v, want http://example.com", cfg.BaseURL)
	}
}

func TestGetEmailConfigFromEnv(t *testing.T) {
	emailConfig = nil
	t.Setenv("SMTP_USER", "env@test.com")
	t.Setenv("SMTP_PASS", "envpass")
	t.Setenv("SMTP_HOST", "smtp.env.com")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("SMTP_FROM", "env@test.com")
	t.Setenv("SMTP_FROM_NAME", "TestApp")
	t.Setenv("BASE_URL", "http://test.com")

	cfg := GetEmailConfig()
	if cfg == nil {
		t.Fatal("GetEmailConfig() should return config from env")
	}
	if cfg.SMTP.Host != "smtp.env.com" {
		t.Errorf("Host = %v, want smtp.env.com", cfg.SMTP.Host)
	}
	if cfg.SMTP.Port != 2525 {
		t.Errorf("Port = %d, want 2525", cfg.SMTP.Port)
	}
	if cfg.BaseURL != "http://test.com" {
		t.Errorf("BaseURL = %v, want http://test.com", cfg.BaseURL)
	}
}

func TestSendPasswordResetEmailNotConfigured(t *testing.T) {
	emailConfig = nil
	err := SendPasswordResetEmail("test@example.com", "user", "123456", "token")
	if err != ErrEmailNotConfigured {
		t.Errorf("error = %v, want ErrEmailNotConfigured", err)
	}
}

func TestSendPasswordResetEmailDisabled(t *testing.T) {
	SetEmailConfig(&config.EmailConfig{Enabled: false})
	defer SetEmailConfig(nil)

	err := SendPasswordResetEmail("test@example.com", "user", "123456", "token")
	if err != ErrEmailNotConfigured {
		t.Errorf("error = %v, want ErrEmailNotConfigured", err)
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	if v := getEnvOrDefault("NONEXISTENT_VAR_12345", "default"); v != "default" {
		t.Errorf("got %q, want default", v)
	}
	t.Setenv("TEST_GETENV_12345", "value")
	if v := getEnvOrDefault("TEST_GETENV_12345", "default"); v != "value" {
		t.Errorf("got %q, want value", v)
	}
}

func TestParseInt(t *testing.T) {
	if v := parseInt("587"); v != 587 {
		t.Errorf("parseInt(587) = %d, want 587", v)
	}
	if v := parseInt(""); v != 0 {
		t.Errorf("parseInt('') = %d, want 0", v)
	}
}

func TestGetBaseURL(t *testing.T) {
	t.Setenv("BASE_URL", "http://example.com/")
	if v := getBaseURL(); v != "http://example.com" {
		t.Errorf("getBaseURL() = %q, want http://example.com (trailing slash removed)", v)
	}
}

func TestGetBaseURLDefault(t *testing.T) {
	if v := getBaseURL(); !strings.Contains(v, "localhost") {
		t.Errorf("getBaseURL() default should contain localhost, got %q", v)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	if v := normalizeBaseURL("http://example.com/"); v != "http://example.com" {
		t.Errorf("normalizeBaseURL() = %q, want http://example.com", v)
	}
	if v := normalizeBaseURL("http://example.com"); v != "http://example.com" {
		t.Errorf("normalizeBaseURL() = %q, want http://example.com", v)
	}
}
