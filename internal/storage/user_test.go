package storage

import (
	"testing"
	"time"
)

func TestUserTOTPFields(t *testing.T) {
	setupTime := time.Now()
	user := &User{
		Username:    "testuser",
		Password:    "hashedpassword",
		TOTPSecret:  "encrypted_secret",
		TOTPEnabled: true,
		BackupCodes: `["hash1","hash2"]`,
		TOTPSetupAt: &setupTime,
	}

	if user.TOTPSecret != "encrypted_secret" {
		t.Errorf("TOTPSecret mismatch: got %s, want %s", user.TOTPSecret, "encrypted_secret")
	}
	if !user.TOTPEnabled {
		t.Error("TOTPEnabled should be true")
	}
	if user.BackupCodes != `["hash1","hash2"]` {
		t.Errorf("BackupCodes mismatch: got %s, want %s", user.BackupCodes, `["hash1","hash2"]`)
	}
	if user.TOTPSetupAt == nil {
		t.Error("TOTPSetupAt should not be nil")
	}
}
