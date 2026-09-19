package auth

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/golang-jwt/jwt/v5"
)

// randRead 注入点：go1.26 crypto/rand.Read 不会失败，错误分支仅供测试覆盖。
var randRead = rand.Read

var (
	jwtSecret      []byte
	jwtExpiration  time.Duration = 24 * time.Hour // 默认 24 小时
	defaultService *Service
)

func init() {
	jwtSecret = resolveSecret(os.Getenv("JWT_SECRET"))
	defaultService = &Service{
		secret:     cloneBytes(jwtSecret),
		expiration: jwtExpiration,
	}
}

// Options Auth 服务配置。
type Options struct {
	Secret     string
	Expiration time.Duration
}

// Service 封装认证依赖，避免 Server 运行期依赖包级全局状态。
type Service struct {
	db         *storage.DB
	mu         sync.RWMutex
	secret     []byte
	expiration time.Duration
}

// NewService 创建认证服务实例。
func NewService(db *storage.DB, opts Options) *Service {
	expiration := opts.Expiration
	if expiration <= 0 {
		expiration = 24 * time.Hour
	}
	return &Service{
		db:         db,
		secret:     resolveSecret(opts.Secret),
		expiration: expiration,
	}
}

// SetDB 设置认证服务使用的数据库。
func (s *Service) SetDB(db *storage.DB) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
}

// SetSecret 设置认证服务 JWT 密钥。
func (s *Service) SetSecret(secret string) {
	if secret == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secret = []byte(secret)
}

// SetExpiration 设置认证服务 JWT 过期时间。
func (s *Service) SetExpiration(expiration time.Duration) {
	if expiration <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expiration = expiration
}

// SetSecret 设置 JWT 密钥
func SetSecret(secret string) {
	if secret != "" {
		jwtSecret = []byte(secret)
		defaultService.SetSecret(secret)
	}
}

// SetExpiration 设置 JWT token 过期时间
func SetExpiration(expiration time.Duration) {
	if expiration > 0 {
		jwtExpiration = expiration
		defaultService.SetExpiration(expiration)
	}
}

// SecretFingerprint 返回当前 secret 的指纹（用于诊断）
func SecretFingerprint() string {
	if len(jwtSecret) < 4 {
		return hex.EncodeToString(jwtSecret)
	}
	return hex.EncodeToString(jwtSecret[:4])
}

// Claims JWT 声明
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken 生成 JWT token
func GenerateToken(userID, username, role string) (string, error) {
	return defaultService.GenerateToken(userID, username, role)
}

// GenerateToken 生成 JWT token
func (s *Service) GenerateToken(userID, username, role string) (string, error) {
	secret, expiration := s.jwtConfig()
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// ValidateToken 验证 JWT token
func ValidateToken(tokenString string) (*Claims, error) {
	return defaultService.ValidateToken(tokenString)
}

// ValidateToken 验证 JWT token
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	secret, _ := s.jwtConfig()
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return secret, nil
	})

	if err != nil {
		return nil, err
	}

	// ParseWithClaims 返回 nil 错误时令牌必已通过签名与有效期校验，
	// Claims 类型断言随之必然成功。
	return token.Claims.(*Claims), nil
}

// RefreshToken 刷新 token
func RefreshToken(tokenString string) (string, error) {
	return defaultService.RefreshToken(tokenString)
}

// RefreshToken 刷新 token
func (s *Service) RefreshToken(tokenString string) (string, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	// 如果 token 还没过期超过 1 小时，直接返回
	if claims.ExpiresAt.Time.After(time.Now().Add(1 * time.Hour)) {
		return tokenString, nil
	}

	return s.GenerateToken(claims.UserID, claims.Username, claims.Role)
}

func (s *Service) jwtConfig() ([]byte, time.Duration) {
	if s == nil {
		return cloneBytes(jwtSecret), jwtExpiration
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	secret := cloneBytes(s.secret)
	if len(secret) == 0 {
		secret = resolveSecret("")
	}
	expiration := s.expiration
	if expiration <= 0 {
		expiration = 24 * time.Hour
	}
	return secret, expiration
}

func resolveSecret(secret string) []byte {
	if secret != "" {
		return []byte(secret)
	}

	// 生成随机 secret，并打印警告。
	b := make([]byte, 32)
	if _, err := randRead(b); err != nil {
		log.Printf("WARNING: failed to generate random JWT secret: %v", err)
		return []byte("change-me")
	}
	log.Println("WARNING: JWT_SECRET not set, using random secret. Tokens will not survive restarts.")
	return b
}

func cloneBytes(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
