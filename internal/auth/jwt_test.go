package auth

import (
	"testing"
	"time"
)

func TestSetSecret(t *testing.T) {
	tests := []struct {
		name  string
		secret string
	}{
		{
			name:  "custom secret",
			secret: "my-custom-secret",
		},
		{
			name:  "empty secret uses default",
			secret: "",
		},
		{
			name:  "long secret",
			secret: "this-is-a-very-long-secret-key-for-testing-purposes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetSecret(tt.secret)
			if len(jwtSecret) == 0 {
				t.Error("jwtSecret should not be empty after SetSecret")
			}
		})
	}
}

func TestGenerateToken(t *testing.T) {
	SetSecret("test-secret")

	tests := []struct {
		name    string
		userID  string
		username string
		role    string
		wantErr bool
	}{
		{
			name:    "valid token",
			userID:  "user-123",
			username: "testuser",
			role:    "admin",
			wantErr: false,
		},
		{
			name:    "empty fields",
			userID:  "",
			username: "",
			role:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := GenerateToken(tt.userID, tt.username, tt.role)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateToken() error = %v, wantErr %v", err, tt.wantErr)
			}
			if token == "" && !tt.wantErr {
				t.Error("GenerateToken() should return non-empty token")
			}
		})
	}
}

func TestValidateToken(t *testing.T) {
	SetSecret("test-secret")

	// Generate a valid token for testing
	validToken, _ := GenerateToken("user-123", "testuser", "admin")

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "valid token",
			token:   validToken,
			wantErr: false,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: true,
		},
		{
			name:    "invalid token",
			token:   "invalid.token.string",
			wantErr: true,
		},
		{
			name:    "malformed token",
			token:   "not-a-jwt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := ValidateToken(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateToken() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if claims == nil {
					t.Error("ValidateToken() should return claims for valid token")
				}
				if claims.UserID != "user-123" {
					t.Errorf("UserID = %v, want user-123", claims.UserID)
				}
				if claims.Username != "testuser" {
					t.Errorf("Username = %v, want testuser", claims.Username)
				}
				if claims.Role != "admin" {
					t.Errorf("Role = %v, want admin", claims.Role)
				}
			}
		})
	}
}

func TestValidateTokenClaims(t *testing.T) {
	SetSecret("test-secret")

	token, _ := GenerateToken("user-456", "john", "user")
	claims, err := ValidateToken(token)

	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}

	if claims.UserID != "user-456" {
		t.Errorf("UserID = %v, want user-456", claims.UserID)
	}

	if claims.Username != "john" {
		t.Errorf("Username = %v, want john", claims.Username)
	}

	if claims.Role != "user" {
		t.Errorf("Role = %v, want user", claims.Role)
	}

	// Check expiration is set
	if claims.ExpiresAt == nil {
		t.Error("ExpiresAt should be set")
	} else {
		expectedExpiry := time.Now().Add(24 * time.Hour)
		diff := expectedExpiry.Sub(claims.ExpiresAt.Time)
		if diff < 0 {
			diff = -diff
		}
		if diff > time.Second {
			t.Errorf("ExpiresAt diff = %v, want < 1s", diff)
		}
	}
}

func TestRefreshToken(t *testing.T) {
	SetSecret("test-secret")

	t.Run("refresh valid token", func(t *testing.T) {
		token, _ := GenerateToken("user-789", "alice", "admin")

		newToken, err := RefreshToken(token)
		if err != nil {
			t.Errorf("RefreshToken() error = %v", err)
		}
		if newToken == "" {
			t.Error("RefreshToken() should return non-empty token")
		}

		// Verify new token is valid
		claims, err := ValidateToken(newToken)
		if err != nil {
			t.Errorf("ValidateToken() on refreshed token error = %v", err)
		}
		if claims.UserID != "user-789" {
			t.Errorf("UserID = %v, want user-789", claims.UserID)
		}
	})

	t.Run("refresh invalid token", func(t *testing.T) {
		_, err := RefreshToken("invalid-token")
		if err == nil {
			t.Error("RefreshToken() should return error for invalid token")
		}
	})
}

func TestTokenExpiry(t *testing.T) {
	SetSecret("test-secret")

	token, _ := GenerateToken("user-expiry", "bob", "user")
	claims, _ := ValidateToken(token)

	// Token should expire in approximately 24 hours
	expectedExpiry := time.Now().Add(24 * time.Hour)
	timeDiff := claims.ExpiresAt.Time.Sub(expectedExpiry)

	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	// Allow 1 second tolerance
	if timeDiff > time.Second {
		t.Errorf("token expiry time diff = %v, want < 1s", timeDiff)
	}
}

func TestTokenIssuedAt(t *testing.T) {
	SetSecret("test-secret")

	before := time.Now()
	token, _ := GenerateToken("user-issued", "charlie", "user")
	after := time.Now()

	claims, _ := ValidateToken(token)

	if claims.IssuedAt == nil {
		t.Fatal("IssuedAt should be set")
	}

	issuedTime := claims.IssuedAt.Time
	if issuedTime.Before(before.Add(-time.Second)) || issuedTime.After(after.Add(time.Second)) {
		t.Errorf("IssuedAt = %v, want between %v and %v", issuedTime, before, after)
	}
}

func TestTokenNotBefore(t *testing.T) {
	SetSecret("test-secret")

	token, _ := GenerateToken("user-notbefore", "dave", "user")
	claims, _ := ValidateToken(token)

	if claims.NotBefore == nil {
		t.Fatal("NotBefore should be set")
	}

	// NotBefore should be approximately now
	now := time.Now()
	diff := now.Sub(claims.NotBefore.Time)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("NotBefore diff = %v, want < 1s", diff)
	}
}

func TestDifferentSecrets(t *testing.T) {
	// Generate token with one secret
	SetSecret("secret-one")
	token, _ := GenerateToken("user-1", "user", "admin")

	// Change secret
	SetSecret("secret-two")

	// Token should be invalid with new secret
	_, err := ValidateToken(token)
	if err == nil {
		t.Error("ValidateToken() should fail with different secret")
	}
}

func TestEmptySecretInit(t *testing.T) {
	// Reset jwtSecret to empty
	jwtSecret = []byte{}

	// GenerateToken should handle empty secret
	token, err := GenerateToken("user-empty", "user", "admin")
	if err != nil {
		t.Errorf("GenerateToken() with empty secret error = %v", err)
	}
	if token == "" {
		t.Error("GenerateToken() should work with empty secret (uses default)")
	}

	// Validate should also work
	claims, err := ValidateToken(token)
	if err != nil {
		t.Errorf("ValidateToken() error = %v", err)
	}
	if claims == nil {
		t.Error("ValidateToken() should return claims")
	}
}

func TestSecretFingerprint(t *testing.T) {
	SetSecret("test-secret-key")
	fp := SecretFingerprint()
	if fp == "" {
		t.Error("SecretFingerprint() should not be empty")
	}
	if len(fp) != 8 {
		t.Errorf("fingerprint length = %d, want 8 (4 bytes hex)", len(fp))
	}
}

func TestRefreshTokenReturnsSameWhenNotNearExpiry(t *testing.T) {
	SetSecret("test-secret")

	token, _ := GenerateToken("user-1", "admin", "admin")
	newToken, err := RefreshToken(token)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	// Token not near expiry, should return same token
	if newToken != token {
		t.Error("RefreshToken() should return same token when not near expiry")
	}
}
