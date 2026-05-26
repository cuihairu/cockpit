package storage

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

var (
	// ErrBackupCodeUsed 备份码已使用错误
	ErrBackupCodeUsed = errors.New("backup code already used")
)

// EnableTOTP 启用 TOTP 验证
func (d *DB) EnableTOTP(userID, encryptedSecret string, hashedBackupCodes []string) error {
	backupJSON, err := json.Marshal(hashedBackupCodes)
	if err != nil {
		return err
	}
	now := time.Now()
	return d.db.Model(&User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"totp_secret":   encryptedSecret,
			"totp_enabled":  true,
			"backup_codes":  string(backupJSON),
			"totp_setup_at": now,
		}).Error
}

// DisableTOTP 禁用 TOTP 验证
func (d *DB) DisableTOTP(userID string) error {
	return d.db.Model(&User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"totp_secret":   "",
			"totp_enabled":  false,
			"backup_codes":  "",
			"totp_setup_at": nil,
		}).Error
}

// UpdateTOTPSecret 更新 TOTP 密钥
func (d *DB) UpdateTOTPSecret(userID, encryptedSecret string) error {
	return d.db.Model(&User{}).
		Where("id = ?", userID).
		Update("totp_secret", encryptedSecret).Error
}

// ConsumeBackupCode 验证并消费备份码
// 返回 (true, nil) 表示备份码有效并被消费
// 返回 (false, nil) 表示备份码无效或已消费
// 返回 (false, err) 表示数据库或其他错误
func (d *DB) ConsumeBackupCode(userID, codeHash string) (bool, error) {
	var user User
	err := d.db.Where("id = ?", userID).First(&user).Error
	if err != nil {
		return false, err
	}

	// 如果没有备份码，返回无效但不返回错误
	if user.BackupCodes == "" {
		return false, nil
	}

	var codes []string
	if err := json.Unmarshal([]byte(user.BackupCodes), &codes); err != nil {
		return false, nil
	}

	// 如果备份码列表为空，返回无效但不返回错误
	if len(codes) == 0 {
		return false, nil
	}

	// 查找匹配的备份码
	found := -1
	for i, code := range codes {
		if code == codeHash {
			found = i
			break
		}
	}

	// 未找到匹配的备份码，返回无效但不返回错误
	if found == -1 {
		return false, nil
	}

	// 移除已使用的备份码
	codes = append(codes[:found], codes[found+1:]...)

	// 更新数据库
	backupJSON, _ := json.Marshal(codes)
	if err := d.db.Model(&User{}).
		Where("id = ?", userID).
		Update("backup_codes", string(backupJSON)).Error; err != nil {
		return false, err
	}

	return true, nil
}

// RegenerateBackupCodes 重新生成备份码
func (d *DB) RegenerateBackupCodes(userID string, hashedBackupCodes []string) error {
	backupJSON, err := json.Marshal(hashedBackupCodes)
	if err != nil {
		return err
	}
	return d.db.Model(&User{}).
		Where("id = ?", userID).
		Update("backup_codes", string(backupJSON)).Error
}

// ValidateTOTPCode 验证 TOTP 代码或备份码
// 返回 (isValid, isBackup, error)
func (d *DB) ValidateTOTPCode(userID, code string) (bool, bool, error) {
	user, err := d.GetUserByID(userID)
	if err != nil {
		return false, false, err
	}

	if !user.TOTPEnabled {
		return false, false, nil
	}

	// 尝试验证 TOTP 代码
	// 需要解密 secret
	secret, err := Decrypt(user.TOTPSecret)
	if err == nil && secret != "" {
		// 直接使用 totp 包验证
		valid, _ := totp.ValidateCustom(
			code,
			secret,
			time.Now(),
			totp.ValidateOpts{
				Period:    30,
				Skew:      1,
				Digits:    otp.DigitsSix,
				Algorithm: otp.AlgorithmSHA1,
			},
		)
		if valid {
			return true, false, nil
		}
	}

	// 尝试验证备份码
	codeHash := HashSingleBackupCode(code)
	valid, err := d.ConsumeBackupCode(userID, codeHash)
	if err != nil {
		return false, false, err
	}
	if valid {
		return true, true, nil
	}

	return false, false, nil
}
