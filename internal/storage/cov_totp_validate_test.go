package storage

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

// covCreateTOTPUser 建一个用户并返回其 ID
func covCreateTOTPUser(t *testing.T, db *DB, username string) string {
	t.Helper()
	user := &User{Username: username, Password: "hash"}
	if err := db.CreateUser(user); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	return user.ID
}

func TestCovValidateTOTPCodeDisabled(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	uid := covCreateTOTPUser(t, db, "cov-totp-off")
	valid, isBackup, err := db.ValidateTOTPCode(uid, "123456")
	if err != nil {
		t.Fatalf("ValidateTOTPCode() error = %v", err)
	}
	if valid || isBackup {
		t.Errorf("got (%v, %v), want (false, false) for disabled TOTP", valid, isBackup)
	}
}

func TestCovValidateTOTPCodeValidCode(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	uid := covCreateTOTPUser(t, db, "cov-totp-on")

	key, err := totp.Generate(totp.GenerateOpts{Issuer: "cockpit", AccountName: "cov-totp-on"})
	if err != nil {
		t.Fatalf("totp.Generate() error = %v", err)
	}
	secret := key.Secret()
	encrypted, err := Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if err := db.EnableTOTP(uid, encrypted, []string{HashSingleBackupCode("AAAA-BBBB-CCCC")}); err != nil {
		t.Fatalf("EnableTOTP() error = %v", err)
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode() error = %v", err)
	}
	valid, isBackup, err := db.ValidateTOTPCode(uid, code)
	if err != nil {
		t.Fatalf("ValidateTOTPCode() error = %v", err)
	}
	if !valid || isBackup {
		t.Errorf("got (%v, %v), want (true, false) for valid TOTP code", valid, isBackup)
	}
}

func TestCovValidateTOTPCodeBackupCode(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	uid := covCreateTOTPUser(t, db, "cov-totp-backup")
	// secret 留空：跳过 TOTP 校验，直接走备份码
	if err := db.EnableTOTP(uid, "", []string{
		HashSingleBackupCode("AAAA-BBBB-1111"),
		HashSingleBackupCode("AAAA-BBBB-2222"),
	}); err != nil {
		t.Fatalf("EnableTOTP() error = %v", err)
	}

	valid, isBackup, err := db.ValidateTOTPCode(uid, "AAAA-BBBB-1111")
	if err != nil {
		t.Fatalf("ValidateTOTPCode() error = %v", err)
	}
	if !valid || !isBackup {
		t.Errorf("got (%v, %v), want (true, true) for backup code", valid, isBackup)
	}

	// 已消费的备份码不能复用；未匹配的备份码也无效
	valid, isBackup, err = db.ValidateTOTPCode(uid, "AAAA-BBBB-1111")
	if err != nil {
		t.Fatalf("ValidateTOTPCode() reused error = %v", err)
	}
	if valid || isBackup {
		t.Errorf("reused backup code got (%v, %v), want (false, false)", valid, isBackup)
	}
	valid, _, err = db.ValidateTOTPCode(uid, "AAAA-BBBB-9999")
	if err != nil {
		t.Fatalf("ValidateTOTPCode() unknown error = %v", err)
	}
	if valid {
		t.Error("unknown backup code should be invalid")
	}
}

func TestCovValidateTOTPCodeUnknownUser(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if _, _, err := db.ValidateTOTPCode("no-such-user", "123456"); err == nil {
		t.Error("ValidateTOTPCode() should fail for unknown user")
	}
}

func TestCovConsumeBackupCodeMalformedJSON(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	uid := covCreateTOTPUser(t, db, "cov-totp-badjson")
	// 直接写入非法 JSON，覆盖 Unmarshal 失败分支
	if err := db.db.Exec("UPDATE users SET backup_codes = 'not-json' WHERE id = ?", uid).Error; err != nil {
		t.Fatalf("seed backup_codes: %v", err)
	}
	ok, err := db.ConsumeBackupCode(uid, "whatever")
	if err != nil {
		t.Fatalf("ConsumeBackupCode() error = %v", err)
	}
	if ok {
		t.Error("ConsumeBackupCode() should be false for malformed JSON")
	}
}

func TestCovConsumeBackupCodeEmptyList(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	uid := covCreateTOTPUser(t, db, "cov-totp-emptylist")
	if err := db.db.Exec("UPDATE users SET backup_codes = '[]' WHERE id = ?", uid).Error; err != nil {
		t.Fatalf("seed backup_codes: %v", err)
	}
	ok, err := db.ConsumeBackupCode(uid, "whatever")
	if err != nil {
		t.Fatalf("ConsumeBackupCode() error = %v", err)
	}
	if ok {
		t.Error("ConsumeBackupCode() should be false for empty list")
	}
}

func TestCovConsumeBackupCodeClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()

	if _, err := db.ConsumeBackupCode("u", "c"); err == nil {
		t.Error("ConsumeBackupCode() on closed DB should fail")
	}
}

// TestCovConsumeBackupCodeWriteConflict 覆盖写库失败分支：
// 另一个连接持有 BEGIN IMMEDIATE 保留锁时，本连接读成功、更新失败。
func TestCovConsumeBackupCodeWriteConflict(t *testing.T) {
	path := t.TempDir() + "/conflict.db"
	db1, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open(db1) error = %v", err)
	}
	defer db1.Close()
	db2, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open(db2) error = %v", err)
	}
	defer db2.Close()

	uid := covCreateTOTPUser(t, db1, "cov-totp-locked")
	if err := db1.EnableTOTP(uid, "", []string{HashSingleBackupCode("CCCC-DDDD-EEEE")}); err != nil {
		t.Fatalf("EnableTOTP() error = %v", err)
	}

	if err := db1.db.Exec("BEGIN IMMEDIATE").Error; err != nil {
		t.Fatalf("BEGIN IMMEDIATE error = %v", err)
	}
	defer db1.db.Exec("ROLLBACK")

	code := "CCCC-DDDD-EEEE"
	// 读不被保留锁阻塞，写更新报错（或等待后报错）
	_, _ = db2.ConsumeBackupCode(uid, HashSingleBackupCode(code))
	valid, _, err := db2.ValidateTOTPCode(uid, code)
	if err != nil {
		t.Logf("ValidateTOTPCode under lock returned error: %v", err)
	} else if valid {
		t.Error("ValidateTOTPCode() should not consume code while DB is locked")
	}
}

// TestCovRegenerateBackupCodesRoundTrip 正常重新生成并校验
func TestCovRegenerateBackupCodesRoundTrip(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	uid := covCreateTOTPUser(t, db, "cov-totp-regen")
	if err := db.RegenerateBackupCodes(uid, []string{"h1", "h2"}); err != nil {
		t.Fatalf("RegenerateBackupCodes() error = %v", err)
	}
	u, err := db.GetUserByID(uid)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	var codes []string
	if err := json.Unmarshal([]byte(u.BackupCodes), &codes); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(codes) != 2 || codes[0] != "h1" {
		t.Errorf("codes = %v, want [h1 h2]", codes)
	}
}

// TestCovEnableTOTPWithEmptyCodes 空备份码列表也能启用
func TestCovEnableTOTPWithEmptyCodes(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	uid := covCreateTOTPUser(t, db, "cov-totp-nocodes")
	if err := db.EnableTOTP(uid, "enc", nil); err != nil {
		t.Fatalf("EnableTOTP() error = %v", err)
	}
	ok, err := db.ConsumeBackupCode(uid, "x")
	if err != nil || ok {
		t.Errorf("ConsumeBackupCode() = (%v, %v), want (false, nil)", ok, err)
	}
}
