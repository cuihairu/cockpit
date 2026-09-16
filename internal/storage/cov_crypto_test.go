package storage

// cov_crypto_test.go 覆盖 applyEncryptionKey（从 init 拆出）的密钥
// 强度分支、ValidateKey 的各错误/成功分支、IsUsingDefaultKey，以及
// Decrypt 对篡改密文的 GCM 认证失败分支。结束后恢复进程初始的默认
// 密钥，避免影响同包其他用例的加解密。

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func TestCovApplyEncryptionKey(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want bool
	}{
		{"empty falls back to default", "", true},
		{"short key is weak", "short-key", true},
		{"default prefix is weak", defaultKeyPrefix + "-anything", true},
		{"strong key is fine", strings.Repeat("k", 40), false},
	}
	for _, c := range cases {
		if got := applyEncryptionKey(c.key); got != c.want {
			t.Errorf("applyEncryptionKey(%q) = %v, want %v (%s)", c.key, got, c.want, c.name)
		}
	}

	// 恢复进程初始密钥（测试进程通常未设置环境变量 → 默认密钥），
	// 保持同包后续 Encrypt/Decrypt 语义
	applyEncryptionKey(os.Getenv("TOTP_ENCRYPTION_KEY"))
}

func TestCovValidateKey(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		wantErr string // 期望错误信息片段；空串表示应成功
	}{
		{"unset", "", "TOTP_ENCRYPTION_KEY environment variable is not set"},
		{"too short", "short-key", "at least 32 characters"},
		{"default prefix", defaultKeyPrefix + "-anything", "default/weak key"},
		{"strong key ok", strings.Repeat("k", 40), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("TOTP_ENCRYPTION_KEY", c.key)
			err := ValidateKey()
			if c.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateKey() = %v, want nil", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("ValidateKey() = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}

func TestCovIsUsingDefaultKey(t *testing.T) {
	// 进程 init 时环境变量通常未设置 → 默认密钥；这里只断言与
	// applyEncryptionKey 返回值联动一致（不修改全局状态）
	want := applyEncryptionKey(os.Getenv("TOTP_ENCRYPTION_KEY"))
	if got := IsUsingDefaultKey(); got != want {
		t.Errorf("IsUsingDefaultKey() = %v, want %v", got, want)
	}
}

func TestCovDecryptTamperedCiphertext(t *testing.T) {
	enc, err := Encrypt("cov-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatal(err)
	}
	// 翻转末字节（认证标签）→ GCM 认证失败
	raw[len(raw)-1] ^= 0xFF
	if _, err := Decrypt(base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Error("Decrypt(tampered) should fail GCM authentication")
	}
}
