package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

// 收敛后的 crypto core（V3/V4）：vault 域密文唯一出处。
// KDF：env key 走单轮 sha256——与存量 storage/crypto.go 同一派生口径，
// 存量密文（remote_credentials/TOTP）零迁移可解。
// 格式：新写入 v1:<base64(nonce||gcm(ct))>；读兼容无前缀 v0 存量格式，
// 版本前缀为将来轮换/KDF 升级的 rewrap 路径预留。

// 密钥来源（V5）：COCKPIT_VAULT_KEY 优先，回退 TOTP_ENCRYPTION_KEY（同口径），
// 都未设走弱默认密钥（开发环境；PRODUCTION=true 由 server 强校验拒绝）。
const (
	vaultKeyEnv  = "COCKPIT_VAULT_KEY"
	fallbackEnv  = "TOTP_ENCRYPTION_KEY"
	defaultKey   = "change-this-totp-encryption-key-in-prod!"
	// 与 storage/crypto.go 的弱密钥前缀完全一致（存量校验口径不变）。
	defaultKeyPrefix = "change-this-totp-encryption-key"
	keyVersionV1     = "v1:"
)

var (
	encryptionKey   []byte
	usingDefaultKey = false

	// 底层原语注入点：生产实现永不失败，错误分支仅供测试注入覆盖。
	newCipher  = aes.NewCipher
	newGCM     = cipher.NewGCM
	cryptoRand = rand.Reader
)

func init() {
	ApplyKey(resolveKeySecret())
}

// resolveKeySecret 按来源链取原始密钥串（V5）：COCKPIT_VAULT_KEY 优先，
// 回退 TOTP_ENCRYPTION_KEY；都未设返回弱默认密钥。
func resolveKeySecret() string {
	if v := os.Getenv(vaultKeyEnv); v != "" {
		return v
	}
	if v := os.Getenv(fallbackEnv); v != "" {
		return v
	}
	return defaultKey
}

// ApplyKey 依密钥派生 AES 密钥并更新弱密钥标志；返回是否落入弱密钥。
// 拆出仅为测试可注入，init 行为不变（与 storage/crypto.go 同构）。
func ApplyKey(secret string) bool {
	weak := secret == "" || len(secret) < 32 || strings.HasPrefix(secret, defaultKeyPrefix)
	encryptionKey = DeriveKey(secret)
	usingDefaultKey = weak
	return weak
}

// DeriveKey KDF 唯一定义点（V3）：单轮 sha256。
func DeriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// IsUsingDefaultKey 是否使用弱默认密钥（server 启动警告用）。
func IsUsingDefaultKey() bool { return usingDefaultKey }

// ValidateKey 校验 env 密钥强度（PRODUCTION 强校验沿用此口径）。
func ValidateKey() error {
	secret := resolveKeySecret()
	if secret == defaultKey {
		return fmt.Errorf("%s environment variable is not set", fallbackEnv)
	}
	if len(secret) < 32 {
		return fmt.Errorf("vault key must be at least 32 characters long")
	}
	if strings.HasPrefix(secret, defaultKeyPrefix) {
		return fmt.Errorf("vault key cannot use default/weak key")
	}
	return nil
}

// Encrypt 以进程密钥加密（v1 带版本前缀）。
func Encrypt(plaintext string) (string, error) {
	return encryptWith(encryptionKey, plaintext, true)
}

// Decrypt 解密 v1 前缀或 v0 存量格式密文。
func Decrypt(ciphertext string) (string, error) {
	return decryptWith(encryptionKey, ciphertext)
}

func encryptWith(key []byte, plaintext string, prefixed bool) (string, error) {
	block, err := newCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(cryptoRand, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	b64 := base64.StdEncoding.EncodeToString(out)
	if prefixed {
		return keyVersionV1 + b64, nil
	}
	return b64, nil
}

func decryptWith(key []byte, ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, keyVersionV1))
	if err != nil {
		return "", fmt.Errorf("vault: ciphertext not base64")
	}
	block, err := newCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("vault: ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("vault: ciphertext auth failed")
	}
	return string(plaintext), nil
}
