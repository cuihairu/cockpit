package storage

import (
	"crypto/rand"
	"encoding/hex"
	"golang.org/x/crypto/bcrypt"
)

// HashPassword 哈希密码（导出）
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// hashPassword 哈希密码（内部使用）
func hashPassword(password string) (string, error) {
	return HashPassword(password)
}

// verifyPassword 验证密码
func verifyPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// randRead 注入点：go1.26 crypto/rand.Read 不会失败，错误分支仅供测试覆盖。
var randRead = rand.Read

// GenerateAgentSecret 生成随机 Agent 密钥（32字节，64位hex）
func GenerateAgentSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := randRead(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashAgentSecret 哈希 Agent 密钥
func HashAgentSecret(secret string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	return string(bytes), err
}

// VerifyAgentSecret 验证 Agent 密钥
func VerifyAgentSecret(hash, secret string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret))
	return err == nil
}
