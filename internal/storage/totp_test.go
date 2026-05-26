package storage

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEnableTOTP(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := &User{
		Username:    "totpuser",
		Password:    "hash",
		TOTPSecret:  "encrypted_secret",
		TOTPEnabled: false,
	}
	if err := db.CreateUser(user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// 测试启用 TOTP
	backupCodes := []string{"hash1", "hash2"}
	if err := db.EnableTOTP(user.ID, "encrypted_secret", backupCodes); err != nil {
		t.Fatalf("EnableTOTP: %v", err)
	}

	// 验证已启用
	u, err := db.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if !u.TOTPEnabled {
		t.Error("TOTP should be enabled")
	}
	if u.TOTPSecret != "encrypted_secret" {
		t.Errorf("TOTPSecret = %s, want encrypted_secret", u.TOTPSecret)
	}
	if u.TOTPSetupAt == nil {
		t.Error("TOTPSetupAt should be set")
	}

	// 验证备份码
	var codes []string
	if err := json.Unmarshal([]byte(u.BackupCodes), &codes); err != nil {
		t.Fatalf("Unmarshal backup codes: %v", err)
	}
	if len(codes) != 2 {
		t.Errorf("backup codes length = %d, want 2", len(codes))
	}
}

func TestDisableTOTP(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	setupTime := time.Now()
	user := &User{
		Username:    "totpuser",
		Password:    "hash",
		TOTPSecret:  "encrypted_secret",
		TOTPEnabled: true,
		BackupCodes: `["hash1","hash2"]`,
		TOTPSetupAt: &setupTime,
	}
	if err := db.CreateUser(user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// 测试禁用 TOTP
	if err := db.DisableTOTP(user.ID); err != nil {
		t.Fatalf("DisableTOTP: %v", err)
	}

	// 验证已禁用
	u, err := db.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if u.TOTPEnabled {
		t.Error("TOTP should be disabled")
	}
	if u.TOTPSecret != "" {
		t.Error("TOTPSecret should be cleared")
	}
	if u.BackupCodes != "" {
		t.Error("BackupCodes should be cleared")
	}
	if u.TOTPSetupAt != nil {
		t.Error("TOTPSetupAt should be cleared")
	}
}

func TestUpdateTOTPSecret(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := &User{
		Username:    "totpuser",
		Password:    "hash",
		TOTPSecret:  "old_secret",
		TOTPEnabled: true,
	}
	if err := db.CreateUser(user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// 测试更新密钥
	if err := db.UpdateTOTPSecret(user.ID, "new_secret"); err != nil {
		t.Fatalf("UpdateTOTPSecret: %v", err)
	}

	// 验证已更新
	u, err := db.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if u.TOTPSecret != "new_secret" {
		t.Errorf("TOTPSecret = %s, want new_secret", u.TOTPSecret)
	}
}

func TestConsumeBackupCode(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := &User{
		Username:    "backupuser",
		Password:    "hash",
		TOTPEnabled: true,
		BackupCodes: `["111111111111111111111111111111111111111111111111111111111111","222222222222222222222222222222222222222222222222222222222222"]`,
	}
	if err := db.CreateUser(user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// 消费第一个备份码
	valid, err := db.ConsumeBackupCode(user.ID, "111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatalf("ConsumeBackupCode: %v", err)
	}
	if !valid {
		t.Error("Backup code should be valid")
	}

	// 验证已消费
	valid, err = db.ConsumeBackupCode(user.ID, "111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatalf("ConsumeBackupCode: %v", err)
	}
	if valid {
		t.Error("Backup code should be consumed")
	}

	// 验证第二个备份码仍然有效
	valid, err = db.ConsumeBackupCode(user.ID, "222222222222222222222222222222222222222222222222222222222222")
	if err != nil {
		t.Fatalf("ConsumeBackupCode second: %v", err)
	}
	if !valid {
		t.Error("Second backup code should be valid")
	}

	// 验证无效备份码返回 false
	valid, err = db.ConsumeBackupCode(user.ID, "invalid_code")
	if err != nil {
		t.Fatalf("ConsumeBackupCode invalid: %v", err)
	}
	if valid {
		t.Error("Invalid backup code should not be valid")
	}
}

func TestConsumeBackupCodeEmpty(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := &User{
		Username:    "nocodeuser",
		Password:    "hash",
		TOTPEnabled: true,
		BackupCodes: "",
	}
	if err := db.CreateUser(user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// 测试没有备份码的情况
	valid, err := db.ConsumeBackupCode(user.ID, "any_code")
	if err != nil {
		t.Fatalf("ConsumeBackupCode: %v", err)
	}
	if valid {
		t.Error("Should not be valid when no backup codes exist")
	}
}

func TestRegenerateBackupCodes(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := &User{
		Username:    "regenuser",
		Password:    "hash",
		TOTPEnabled: true,
		BackupCodes: `["old1","old2"]`,
	}
	if err := db.CreateUser(user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// 重新生成备份码
	newCodes := []string{"new1", "new2", "new3"}
	if err := db.RegenerateBackupCodes(user.ID, newCodes); err != nil {
		t.Fatalf("RegenerateBackupCodes: %v", err)
	}

	// 验证已更新
	u, err := db.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}

	var codes []string
	if err := json.Unmarshal([]byte(u.BackupCodes), &codes); err != nil {
		t.Fatalf("Unmarshal backup codes: %v", err)
	}
	if len(codes) != 3 {
		t.Errorf("backup codes length = %d, want 3", len(codes))
	}
	if codes[0] != "new1" {
		t.Errorf("First code = %s, want new1", codes[0])
	}
}
