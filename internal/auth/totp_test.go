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
