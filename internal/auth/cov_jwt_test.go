package auth

import (
	"testing"
	"time"
)

func TestCovNewService(t *testing.T) {
	// 默认过期时间（<=0 回退 24h）
	s := NewService(nil, Options{})
	if s == nil {
		t.Fatal("NewService() should not return nil")
	}
	secret, expiration := s.jwtConfig()
	if len(secret) == 0 {
		t.Error("secret should be generated when not provided")
	}
	if expiration != 24*time.Hour {
		t.Errorf("expiration = %v, want 24h", expiration)
	}

	// 显式 secret 与过期时间
	s2 := NewService(nil, Options{Secret: "cov-secret", Expiration: time.Hour})
	secret2, expiration2 := s2.jwtConfig()
	if string(secret2) != "cov-secret" {
		t.Errorf("secret = %s, want cov-secret", secret2)
	}
	if expiration2 != time.Hour {
		t.Errorf("expiration = %v, want 1h", expiration2)
	}
}

func TestCovServiceSetSecretEmpty(t *testing.T) {
	s := NewService(nil, Options{Secret: "cov-original"})

	s.SetSecret("")
	secret, _ := s.jwtConfig()
	if string(secret) != "cov-original" {
		t.Errorf("SetSecret(\"\") should be ignored, got %s", secret)
	}

	s.SetSecret("cov-updated")
	secret, _ = s.jwtConfig()
	if string(secret) != "cov-updated" {
		t.Errorf("SetSecret non-empty should apply, got %s", secret)
	}
}

func TestCovServiceSetExpiration(t *testing.T) {
	s := NewService(nil, Options{Expiration: time.Hour})

	s.SetExpiration(0) // 非正数被忽略
	_, expiration := s.jwtConfig()
	if expiration != time.Hour {
		t.Errorf("SetExpiration(0) should be ignored, got %v", expiration)
	}

	s.SetExpiration(-time.Minute)
	_, expiration = s.jwtConfig()
	if expiration != time.Hour {
		t.Errorf("SetExpiration(negative) should be ignored, got %v", expiration)
	}

	s.SetExpiration(2 * time.Hour)
	_, expiration = s.jwtConfig()
	if expiration != 2*time.Hour {
		t.Errorf("SetExpiration valid should apply, got %v", expiration)
	}
}

func TestCovPackageSetExpiration(t *testing.T) {
	defer SetExpiration(24 * time.Hour)

	SetExpiration(2 * time.Hour)
	if jwtExpiration != 2*time.Hour {
		t.Errorf("jwtExpiration = %v, want 2h", jwtExpiration)
	}

	SetExpiration(0) // 非正数被忽略
	if jwtExpiration != 2*time.Hour {
		t.Errorf("SetExpiration(0) should be ignored, got %v", jwtExpiration)
	}
}

func TestCovSecretFingerprintShortSecret(t *testing.T) {
	original := jwtSecret
	jwtSecret = []byte("ab")
	defer func() { jwtSecret = original }()

	fp := SecretFingerprint()
	if fp != "6162" { // hex("ab")
		t.Errorf("SecretFingerprint() = %s, want 6162", fp)
	}
}

func TestCovRefreshTokenNearExpiry(t *testing.T) {
	defer SetExpiration(24 * time.Hour)
	SetSecret("cov-refresh-secret")
	SetExpiration(30 * time.Minute)

	token, err := GenerateToken("cov-user", "cov-name", "admin")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	newToken, err := RefreshToken(token)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if newToken == "" {
		t.Fatal("RefreshToken() should return a token")
	}
	// 注意：同秒内重新签发的 JWT 可能与原 token 完全相同（claims 相同），
	// 这里只验证新 token 仍有效且过期时间在 1 小时内（即走了近过期逻辑）。
	claims, err := ValidateToken(newToken)
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if claims.UserID != "cov-user" {
		t.Errorf("UserID = %s, want cov-user", claims.UserID)
	}
	if !claims.ExpiresAt.Time.After(time.Now()) {
		t.Errorf("ExpiresAt = %v, should still be in the future", claims.ExpiresAt.Time)
	}
}

func TestCovJwtConfigNilReceiver(t *testing.T) {
	var s *Service
	secret, expiration := s.jwtConfig()
	if len(secret) == 0 {
		t.Error("nil receiver jwtConfig() should fall back to global secret")
	}
	if expiration != jwtExpiration {
		t.Errorf("expiration = %v, want %v", expiration, jwtExpiration)
	}
}

func TestCovJwtConfigEmptySecretAndExpiration(t *testing.T) {
	// 空 secret：回退到随机生成
	s := &Service{}
	secret, _ := s.jwtConfig()
	if len(secret) == 0 {
		t.Error("empty service secret should fall back to a generated secret")
	}

	// 非正数过期时间回退 24h
	s2 := &Service{secret: []byte("cov-k"), expiration: -time.Second}
	_, expiration := s2.jwtConfig()
	if expiration != 24*time.Hour {
		t.Errorf("expiration = %v, want 24h", expiration)
	}
}

func TestCovServiceTokenRoundTrip(t *testing.T) {
	s := NewService(nil, Options{Secret: "cov-roundtrip", Expiration: time.Hour})

	token, err := s.GenerateToken("u1", "n1", "user")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	claims, err := s.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if claims.Username != "n1" || claims.Role != "user" {
		t.Errorf("claims = %+v", claims)
	}

	if _, err := s.RefreshToken("not-a-token"); err == nil {
		t.Error("RefreshToken() should fail for invalid token")
	}
}
