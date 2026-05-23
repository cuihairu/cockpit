package auth

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret = []byte(os.Getenv("JWT_SECRET"))

// SetSecret 设置 JWT 密钥
func SetSecret(secret string) {
	if secret != "" {
		jwtSecret = []byte(secret)
	} else {
		// 默认密钥（生产环境应该从环境变量设置）
		jwtSecret = []byte("change-this-secret-in-production")
	}
}

// Claims JWT 声明
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// GenerateToken 生成 JWT token
func GenerateToken(userID, username string) (string, error) {
	if len(jwtSecret) == 0 {
		SetSecret("")
	}

	claims := Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ValidateToken 验证 JWT token
func ValidateToken(tokenString string) (*Claims, error) {
	if len(jwtSecret) == 0 {
		SetSecret("")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// RefreshToken 刷新 token
func RefreshToken(tokenString string) (string, error) {
	claims, err := ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	// 如果 token 还没过期，直接返回
	if claims.ExpiresAt.Time.After(time.Now().Add(1 * time.Hour)) {
		return tokenString, nil
	}

	return GenerateToken(claims.UserID, claims.Username)
}
