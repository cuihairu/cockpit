package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
