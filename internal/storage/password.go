package storage

import (
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
