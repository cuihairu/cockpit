package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/config"
)

func TestCovHandleLoginNoDB(t *testing.T) {
	// Service 未注入数据库时返回 500
	s := &Service{}
	body, _ := json.Marshal(LoginRequest{Username: "u", Password: "p"})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.HandleLogin(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestCovDBNilReceiver(t *testing.T) {
	var s *Service
	if got := s.DB(); got != DB {
		t.Error("nil receiver DB() should return package-level DB")
	}

	s2 := &Service{}
	if s2.DB() != nil {
		t.Error("empty service DB() should be nil")
	}
}

func TestCovSendPasswordResetEmailSendFailure(t *testing.T) {
	// 完整但指向本机未监听端口的 SMTP 配置：走到 sendEmail 拨号失败
	SetEmailConfig(&config.EmailConfig{
		Enabled: true,
		SMTP: &config.SMTPConfig{
			Host:     "127.0.0.1",
			Port:     1, // 本机 1 端口基本不可能监听，拨号立即被拒
			Username: "cov-user",
			Password: "cov-pass",
			From:     "cov@example.com",
			FromName: "CovTest",
		},
		// BaseURL 留空：走 getBaseURL() 回退分支
	})
	defer SetEmailConfig(nil)

	err := SendPasswordResetEmail("dest@example.com", "covname", "123456", "cov-token")
	if err == nil {
		t.Fatal("SendPasswordResetEmail() should fail when SMTP is unreachable")
	}
	t.Logf("send error (expected): %v", err)
}

func TestCovSendPasswordResetEmailDisabled(t *testing.T) {
	SetEmailConfig(nil)
	defer SetEmailConfig(nil)

	if err := SendPasswordResetEmail("a@b.c", "u", "123456", "t"); err != ErrEmailNotConfigured {
		t.Errorf("error = %v, want ErrEmailNotConfigured", err)
	}

	// Enabled=false 同样视为未配置
	SetEmailConfig(&config.EmailConfig{Enabled: false, SMTP: &config.SMTPConfig{}})
	if err := SendPasswordResetEmail("a@b.c", "u", "123456", "t"); err != ErrEmailNotConfigured {
		t.Errorf("disabled config error = %v, want ErrEmailNotConfigured", err)
	}
}

// TestCovHandleLoginTokenSignFail 注入签名失败，覆盖登录处理器
// 生成 token 失败的 500 分支（HS256 + []byte 密钥下 SignedString 不会失败）。
func TestCovHandleLoginTokenSignFail(t *testing.T) {
	db := testAuthDB(t)
	InitDB(db)
	if err := db.InitAdminUser("admin", "password123"); err != nil {
		t.Fatal(err)
	}

	orig := signLoginToken
	t.Cleanup(func() { signLoginToken = orig })
	signLoginToken = func(*Service, string, string, string) (string, error) {
		return "", errors.New("boom sign")
	}

	body, _ := json.Marshal(LoginRequest{Username: "admin", Password: "password123"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()

	HandleLogin(w, req)
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "Failed to generate token") {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

// TestCovGenerateTmpTokenRandFail 注入随机数失败，覆盖临时令牌的
// 时间戳后备分支。
func TestCovGenerateTmpTokenRandFail(t *testing.T) {
	orig := randRead
	t.Cleanup(func() { randRead = orig })
	randRead = func([]byte) (int, error) { return 0, errors.New("boom rand") }

	token := generateTmpToken("u1")
	if !strings.HasPrefix(token, "tmp_") {
		t.Errorf("fallback token = %q, want tmp_ prefix", token)
	}
}

// TestCovGenerateResetTokenRandFail 注入随机数失败，覆盖重置令牌
// 生成的错误分支。
func TestCovGenerateResetTokenRandFail(t *testing.T) {
	orig := randRead
	t.Cleanup(func() { randRead = orig })
	randRead = func([]byte) (int, error) { return 0, errors.New("boom rand") }

	if _, _, err := GenerateResetToken("u1", "a@b.c"); err == nil {
		t.Error("GenerateResetToken with failing rand should fail")
	}
}

// TestCovResolveSecretRandFail 注入随机数失败，覆盖随机 JWT secret
// 的降级分支（回退固定弱密钥）。
func TestCovResolveSecretRandFail(t *testing.T) {
	orig := randRead
	t.Cleanup(func() { randRead = orig })
	randRead = func([]byte) (int, error) { return 0, errors.New("boom rand") }

	if got := resolveSecret(""); string(got) != "change-me" {
		t.Errorf("resolveSecret fallback = %q, want change-me", got)
	}
}
