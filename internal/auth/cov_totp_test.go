package auth

import (
	"strings"
	"testing"
)

func TestCovGenerateTOTPSecretError(t *testing.T) {
	// pquerna/otp 要求 AccountName 非空
	_, err := GenerateTOTPSecret("", "cockpit")
	if err == nil {
		t.Error("GenerateTOTPSecret() should fail with empty account name")
	}

	secret, err := GenerateTOTPSecret("cov-user", "cockpit")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret() error = %v", err)
	}
	if secret == "" {
		t.Error("GenerateTOTPSecret() should return non-empty secret")
	}
}

func TestCovGenerateTOTPURLError(t *testing.T) {
	// issuer 里的控制字符令 otpauth URL 解析失败（url.Parse 拒绝控制字符）
	if _, err := GenerateTOTPURL("JBSWY3DPEHPK3PXP", "cov-user", "bad\x00issuer"); err == nil {
		t.Error("GenerateTOTPURL() should fail with control character in issuer")
	}
	if _, err := GenerateTOTPURL("JBSWY3DPEHPK3PXP", "cov\x7fuser", "cockpit"); err == nil {
		t.Error("GenerateTOTPURL() should fail with control character in account")
	}

	url, err := GenerateTOTPURL("JBSWY3DPEHPK3PXP", "cov-user", "cockpit")
	if err != nil {
		t.Fatalf("GenerateTOTPURL() error = %v", err)
	}
	if !strings.HasPrefix(url, "otpauth://totp/") {
		t.Errorf("url = %s, want otpauth URL", url)
	}
}

func TestCovGenerateQRCodeDataError(t *testing.T) {
	if _, err := GenerateQRCodeData("JBSWY3DPEHPK3PXP", "cov-user", "bad\x00issuer"); err == nil {
		t.Error("GenerateQRCodeData() should fail with control character in issuer")
	}

	data, err := GenerateQRCodeData("JBSWY3DPEHPK3PXP", "cov-user", "cockpit")
	if err != nil {
		t.Fatalf("GenerateQRCodeData() error = %v", err)
	}
	if data == "" {
		t.Error("GenerateQRCodeData() should return URL")
	}
}
