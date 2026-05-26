package storage

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

var encryptionKey []byte

func init() {
	key := os.Getenv("TOTP_ENCRYPTION_KEY")
	if key == "" {
		// 开发环境默认密钥
		// ⚠️ 警告：生产环境必须设置 TOTP_ENCRYPTION_KEY 环境变量！
		// 使用默认密钥会导致所有实例使用相同密钥，严重破坏安全性。
		key = "change-this-totp-encryption-key-in-prod!"
	}
	hash := sha256.Sum256([]byte(key))
	encryptionKey = hash[:]
}

// Encrypt 使用 AES-256-GCM 加密明文
func Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密密文
func Decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// GenerateBackupCodes 生成 10 个备份恢复码
func GenerateBackupCodes() ([]string, error) {
	codes := make([]string, 10)
	for i := 0; i < 10; i++ {
		// 生成 12 位随机字符
		b := make([]byte, 6)
		if _, err := io.ReadFull(rand.Reader, b); err != nil {
			return nil, err
		}
		code := fmt.Sprintf("%04x-%04x-%04x", b[0:2], b[2:4], b[4:6])
		codes[i] = strings.ToUpper(code)
	}
	return codes, nil
}

// HashBackupCodes 对备份码进行 SHA256 哈希
func HashBackupCodes(codes []string) ([]string, error) {
	hashed := make([]string, len(codes))
	for i, code := range codes {
		hash := sha256.Sum256([]byte(code))
		hashed[i] = fmt.Sprintf("%x", hash)
	}
	return hashed, nil
}

// HashSingleBackupCode 单个备份码哈希（用于验证时对比）
func HashSingleBackupCode(code string) string {
	hash := sha256.Sum256([]byte(code))
	return fmt.Sprintf("%x", hash)
}
