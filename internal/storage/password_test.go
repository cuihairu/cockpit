package storage

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name    string
		password string
		wantErr bool
	}{
		{
			name:    "simple password",
			password: "password123",
			wantErr: false,
		},
		{
			name:    "complex password",
			password: "C0mplex!P@ssw0rd#2024",
			wantErr: false,
		},
		{
			name:    "empty password",
			password: "",
			wantErr: false,
		},
		{
			name:    "long password",
			password: strings.Repeat("a", 72),
			wantErr: false,
		},
		{
			name:    "unicode password",
			password: "密码123",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashPassword() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if hash == "" {
					t.Error("HashPassword() should return non-empty hash")
				}
				// Hash should not equal the password
				if hash == tt.password {
					t.Error("HashPassword() should not return plain password")
				}
				// bcrypt hash always starts with $2a$, $2b$, or $2y$
				if len(hash) < 3 || hash[0:3] != "$2a" && hash[0:3] != "$2b" && hash[0:3] != "$2y" {
					t.Errorf("HashPassword() should return bcrypt hash, got %s", hash[:min(10, len(hash))])
				}
			}
		})
	}
}

func TestVerifyPassword(t *testing.T) {
	password := "test-password-123"
	hash, _ := HashPassword(password)

	tests := []struct {
		name           string
		hashedPassword string
		password       string
		want           bool
	}{
		{
			name:           "correct password",
			hashedPassword: hash,
			password:       password,
			want:           true,
		},
		{
			name:           "incorrect password",
			hashedPassword: hash,
			password:       "wrong-password",
			want:           false,
		},
		{
			name:           "empty password",
			hashedPassword: hash,
			password:       "",
			want:           false,
		},
		{
			name:           "invalid hash",
			hashedPassword: "invalid-hash",
			password:       password,
			want:           false,
		},
		{
			name:           "empty hash",
			hashedPassword: "",
			password:       password,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := verifyPassword(tt.hashedPassword, tt.password); got != tt.want {
				t.Errorf("verifyPassword() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHashPasswordUniqueness(t *testing.T) {
	password := "same-password"

	hash1, _ := HashPassword(password)
	hash2, _ := HashPassword(password)

	// Each hash should be unique due to bcrypt salt
	if hash1 == hash2 {
		t.Error("HashPassword() should generate different hashes for same password")
	}
}

func TestHashPasswordLength(t *testing.T) {
	password := "test"
	hash, _ := HashPassword(password)

	// bcrypt hashes are always 60 characters
	if len(hash) != 60 {
		t.Errorf("HashPassword() length = %d, want 60", len(hash))
	}
}

func TestVerifyPasswordConsistency(t *testing.T) {
	passwords := []string{
		"short",
		"password123",
		"P@ssw0rd!2024",
		"密码测试",
		strings.Repeat("a", 60), // bcrypt has 72 byte limit
	}

	for _, password := range passwords {
		t.Run(password, func(t *testing.T) {
			hash, _ := HashPassword(password)
			if !verifyPassword(hash, password) {
				t.Error("verifyPassword() should return true for correct password")
			}
			if verifyPassword(hash, password+"x") {
				t.Error("verifyPassword() should return false for incorrect password")
			}
		})
	}
}

func TestHashPasswordInternal(t *testing.T) {
	password := "test-internal"

	// Test internal function (which calls exported function)
	hash, err := hashPassword(password)
	if err != nil {
		t.Errorf("hashPassword() error = %v", err)
	}
	if hash == "" {
		t.Error("hashPassword() should return non-empty hash")
	}

	// Verify it works with verifyPassword
	if !verifyPassword(hash, password) {
		t.Error("hashPassword() result should be verifiable")
	}
}

func BenchmarkHashPassword(b *testing.B) {
	password := "benchmark-password"

	b.ReportAllocs()
	for b.Loop() {
		HashPassword(password)
	}
}

func BenchmarkVerifyPassword(b *testing.B) {
	password := "benchmark-password"
	hash, _ := HashPassword(password)

	b.ReportAllocs()
	for b.Loop() {
		verifyPassword(hash, password)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
