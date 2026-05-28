package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestGenerateTOTPSecret(t *testing.T) {
	secret, err := GenerateTOTPSecret("testuser", "Cockpit")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}

	if secret == "" {
		t.Error("Secret should not be empty")
	}

	// 验证是有效的 Base32
	if len(secret) < 16 {
		t.Error("Secret too short")
	}
}

func TestValidateTOTP(t *testing.T) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Cockpit",
		AccountName: "test@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 生成有效代码
	code, err := totp.GenerateCode(key.Secret(), time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// 验证应该成功
	if !ValidateTOTP(key.Secret(), code) {
		t.Error("Valid TOTP code should pass")
	}

	// 无效代码应该失败
	if ValidateTOTP(key.Secret(), "000000") {
		t.Error("Invalid TOTP code should fail")
	}
}

func TestGenerateQRCode(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	url, err := GenerateTOTPURL(secret, "testuser", "Cockpit")
	if err != nil {
		t.Fatalf("GenerateTOTPURL: %v", err)
	}

	if !strings.Contains(url, "otpauth://totp") {
		t.Error("URL should be otpauth format")
	}
}

func TestGenerateQRCodeData(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	data, err := GenerateQRCodeData(secret, "testuser", "Cockpit")
	if err != nil {
		t.Fatalf("GenerateQRCodeData() error = %v", err)
	}
	if data == "" {
		t.Error("QR code data should not be empty")
	}
	if !strings.Contains(data, "otpauth://") {
		t.Error("QR code data should contain otpauth URL")
	}
}

func TestFormatBackupCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abcdefghijkl", "ABCD-EFGH-IJKL"},
		{"short", "SHORT"},
		{"", ""},
		{"123456789012", "1234-5678-9012"},
	}
	for _, tt := range tests {
		got := FormatBackupCode(tt.input)
		if got != tt.want {
			t.Errorf("FormatBackupCode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidateTOTPInvalid(t *testing.T) {
	// Random secret, random code — should fail
	if ValidateTOTP("INVALIDSECRET", "000000") {
		t.Error("ValidateTOTP should return false for invalid code")
	}
}
