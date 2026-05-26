package auth

import (
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// GenerateTOTPSecret 生成 TOTP 密钥
func GenerateTOTPSecret(username, issuer string) (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: username,
		SecretSize:  20,
	})
	if err != nil {
		return "", err
	}
	return key.Secret(), nil
}

// GenerateTOTPURL 生成 TOTP URL（用于 QR 码）
func GenerateTOTPURL(secret, username, issuer string) (string, error) {
	// 直接使用 otp.Key 从 URL 构建
	key, err := otp.NewKeyFromURL(fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s",
		issuer, username, secret, issuer,
	))
	if err != nil {
		return "", err
	}
	return key.String(), nil
}

// ValidateTOTP 验证 TOTP 代码
func ValidateTOTP(secret, code string) bool {
	// 允许 ±1 个时间步长的容错
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
	return valid
}

// GenerateQRCodeData 生成 QR 码数据
// 注意：实际 QR 码图片生成在前端进行，这里只返回 URL
func GenerateQRCodeData(secret, username, issuer string) (string, error) {
	url, err := GenerateTOTPURL(secret, username, issuer)
	if err != nil {
		return "", err
	}
	return url, nil
}

// FormatBackupCode 格式化备份码显示
func FormatBackupCode(code string) string {
	// xxxx-xxxx-xxxx 格式
	if len(code) == 12 {
		return strings.ToUpper(fmt.Sprintf("%s-%s-%s", code[0:4], code[4:8], code[8:12]))
	}
	return strings.ToUpper(code)
}
