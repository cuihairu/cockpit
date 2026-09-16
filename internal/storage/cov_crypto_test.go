package storage

import (
	"crypto/rand"
	"errors"
	"strings"
	"testing"
)

// covFailingRandReader 一个恒定失败的 io.Reader 替身
type covFailingRandReader struct{}

func (covFailingRandReader) Read([]byte) (int, error) {
	return 0, errors.New("cov: entropy exhausted")
}

// covWithFailingRand 在回调期间把 crypto/rand.Reader 换成失败实现，结束后恢复。
// crypto.go 通过 io.ReadFull(rand.Reader, ...) 取随机数（走该变量），
// 可覆盖其失败分支；注意 rand.Read 函数（Go 1.24+）不走此变量。
func covWithFailingRand(t *testing.T, fn func()) {
	t.Helper()
	oldReader := rand.Reader
	rand.Reader = covFailingRandReader{}
	defer func() { rand.Reader = oldReader }()
	fn()
}

// ============ 密钥状态 ============

func TestCovIsUsingDefaultKey(t *testing.T) {
	// 测试环境未设置 TOTP_ENCRYPTION_KEY，init 走默认密钥分支
	if !IsUsingDefaultKey() {
		t.Error("IsUsingDefaultKey() = false, want true (env unset at process start)")
	}
}

func TestCovValidateKey(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		set     bool
		wantErr bool
	}{
		{name: "unset", set: false, wantErr: true},
		{name: "empty", env: "", set: true, wantErr: true},
		{name: "too short", env: "short-key", set: true, wantErr: true},
		{name: "default prefix", env: "change-this-totp-encryption-key-and-more", set: true, wantErr: true},
		{name: "valid", env: "0123456789abcdef0123456789abcdef", set: true, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("TOTP_ENCRYPTION_KEY", tt.env)
			} else {
				t.Setenv("TOTP_ENCRYPTION_KEY", "")
			}
			err := ValidateKey()
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateKey() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "TOTP_ENCRYPTION_KEY") {
				t.Errorf("error = %v, want it to mention TOTP_ENCRYPTION_KEY", err)
			}
		})
	}
}

// ============ Encrypt / Decrypt 错误路径 ============

func TestCovEncryptInvalidKeySize(t *testing.T) {
	originalKey := encryptionKey
	encryptionKey = []byte("15-byte-key-xx") // 非 16/24/32 字节，AES 拒绝
	defer func() { encryptionKey = originalKey }()

	if _, err := Encrypt("plain"); err == nil {
		t.Error("Encrypt() should fail with invalid key size")
	}
	if _, err := Decrypt("YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXo="); err == nil {
		t.Error("Decrypt() should fail with invalid key size")
	}
}

func TestCovEncryptRandFailure(t *testing.T) {
	covWithFailingRand(t, func() {
		if _, err := Encrypt("plain"); err == nil {
			t.Error("Encrypt() should fail when rand fails")
		}
	})
}

func TestCovDecryptTamperedCiphertext(t *testing.T) {
	originalKey := encryptionKey
	encryptionKey = []byte("exactly32byteslongtestencryptkey")
	defer func() { encryptionKey = originalKey }()

	encrypted, err := Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// 替换首个字符为另一个合法 base64 字符：密文首字节（nonce）变化 →
	// GCM 认证失败（翻转末字符会命中 '=' 填充，只测到 base64 解码错误）
	tampered := []byte(encrypted)
	if tampered[0] == 'A' {
		tampered[0] = 'B'
	} else {
		tampered[0] = 'A'
	}
	if _, err := Decrypt(string(tampered)); err == nil {
		t.Error("Decrypt() should fail for tampered ciphertext")
	}
}

func TestCovGenerateBackupCodesRandFailure(t *testing.T) {
	covWithFailingRand(t, func() {
		if _, err := GenerateBackupCodes(); err == nil {
			t.Error("GenerateBackupCodes() should fail when rand fails")
		}
	})
}

// ============ Agent 密钥（password.go） ============

func TestCovGenerateAgentSecret(t *testing.T) {
	secret, err := GenerateAgentSecret()
	if err != nil {
		t.Fatalf("GenerateAgentSecret() error = %v", err)
	}
	if len(secret) != 64 {
		t.Errorf("length = %d, want 64", len(secret))
	}
	again, _ := GenerateAgentSecret()
	if again == secret {
		t.Error("GenerateAgentSecret() should be random")
	}
}

func TestCovHashAndVerifyAgentSecret(t *testing.T) {
	hash, err := HashAgentSecret("my-agent-secret")
	if err != nil {
		t.Fatalf("HashAgentSecret() error = %v", err)
	}
	if hash == "my-agent-secret" {
		t.Error("HashAgentSecret() should not return plaintext")
	}
	if !VerifyAgentSecret(hash, "my-agent-secret") {
		t.Error("VerifyAgentSecret() should accept correct secret")
	}
	if VerifyAgentSecret(hash, "wrong-secret") {
		t.Error("VerifyAgentSecret() should reject wrong secret")
	}
	if VerifyAgentSecret("not-a-hash", "my-agent-secret") {
		t.Error("VerifyAgentSecret() should reject malformed hash")
	}
}
