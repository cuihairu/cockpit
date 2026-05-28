package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
)

var (
	// ErrResetTokenInvalid 重置令牌无效
	ErrResetTokenInvalid = errors.New("invalid or expired reset token")
	// ErrEmailNotConfigured 邮件未配置
	ErrEmailNotConfigured = errors.New("email service not configured")
)

// ResetTokenData 重置令牌数据
type ResetTokenData struct {
	UserID    string
	Email     string
	Code      string // 6位验证码
	ExpiresAt time.Time
}

// 重置令牌存储（生产环境应使用 Redis）
var resetTokenStore = make(map[string]*ResetTokenData)
var resetTokenStoreMutex = make(map[string]*time.Time)

// emailConfig 全局邮件配置
var emailConfig *config.EmailConfig
var emailConfigMutex sync.RWMutex

// SetEmailConfig 设置邮件配置
func SetEmailConfig(cfg *config.EmailConfig) {
	emailConfigMutex.Lock()
	defer emailConfigMutex.Unlock()
	emailConfig = cfg
}

// 生成6位数字验证码
func generateVerificationCode() string {
	b := make([]byte, 3)
	rand.Read(b)
	return fmt.Sprintf("%06d", (int(b[0])<<16|int(b[1])<<8|int(b[2]))%1000000)
}

// GenerateResetToken 生成密码重置令牌和验证码
func GenerateResetToken(userID, email string) (string, string, error) {
	// 生成32字节随机令牌
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(tokenBytes)

	// 生成6位验证码
	code := generateVerificationCode()

	// 存储（30分钟有效）
	data := &ResetTokenData{
		UserID:    userID,
		Email:     email,
		Code:      code,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	resetTokenStore[token] = data
	resetTokenStoreMutex[token] = &data.ExpiresAt

	// 清理过期令牌
	go cleanupExpiredTokens()

	return token, code, nil
}

// ValidateResetToken 验证重置令牌
func ValidateResetToken(token string) (*ResetTokenData, error) {
	data, exists := resetTokenStore[token]
	if !exists {
		return nil, ErrResetTokenInvalid
	}

	if time.Now().After(data.ExpiresAt) {
		delete(resetTokenStore, token)
		delete(resetTokenStoreMutex, token)
		return nil, ErrResetTokenInvalid
	}

	return data, nil
}

// ValidateResetCode 验证重置验证码
func ValidateResetCode(token, code string) (*ResetTokenData, error) {
	data, err := ValidateResetToken(token)
	if err != nil {
		return nil, err
	}

	if data.Code != code {
		return nil, ErrResetTokenInvalid
	}

	return data, nil
}

// ConsumeResetToken 消费重置令牌（验证后删除）
func ConsumeResetToken(token string) bool {
	data, exists := resetTokenStore[token]
	if !exists {
		return false
	}

	if time.Now().After(data.ExpiresAt) {
		delete(resetTokenStore, token)
		delete(resetTokenStoreMutex, token)
		return false
	}

	delete(resetTokenStore, token)
	delete(resetTokenStoreMutex, token)
	_ = data
	return true
}

// cleanupExpiredTokens 清理过期令牌
func cleanupExpiredTokens() {
	for token, expiry := range resetTokenStoreMutex {
		if expiry != nil && time.Now().After(*expiry) {
			delete(resetTokenStore, token)
			delete(resetTokenStoreMutex, token)
		}
	}
}

// GetEmailConfig 获取邮件配置（优先使用配置文件，否则回退到环境变量）
func GetEmailConfig() *config.EmailConfig {
	emailConfigMutex.RLock()
	defer emailConfigMutex.RUnlock()

	// 如果已通过配置文件设置，直接返回
	if emailConfig != nil && emailConfig.Enabled {
		return emailConfig
	}

	// 回退到环境变量（保持向后兼容）
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	if smtpUser == "" || smtpPass == "" {
		return nil // 邮件未配置
	}

	return &config.EmailConfig{
		Enabled: true,
		SMTP: &config.SMTPConfig{
			Host:     getEnvOrDefault("SMTP_HOST", "smtp.gmail.com"),
			Port:     parseInt(getEnvOrDefault("SMTP_PORT", "587")),
			Username: smtpUser,
			Password: smtpPass,
			From:     getEnvOrDefault("SMTP_FROM", smtpUser),
			FromName: getEnvOrDefault("SMTP_FROM_NAME", "Cockpit"),
		},
		BaseURL: getEnvOrDefault("BASE_URL", "http://localhost:9000"),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

func parseInt(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}

// SendPasswordResetEmail 发送密码重置邮件
func SendPasswordResetEmail(email, username, code, token string) error {
	cfg := GetEmailConfig()

	// 检查邮件配置
	if cfg == nil || !cfg.Enabled || cfg.SMTP == nil || cfg.SMTP.Username == "" || cfg.SMTP.Password == "" {
		return ErrEmailNotConfigured
	}

	// 构建邮件内容
	subject := "重置您的 Cockpit 密码"
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = getBaseURL()
	}
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)

	body := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<style>
		body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
		.container { max-width: 600px; margin: 0 auto; padding: 20px; }
		.button { display: inline-block; padding: 12px 24px; background: #165DFF; color: white; text-decoration: none; border-radius: 4px; }
		.code { font-size: 24px; font-weight: bold; letter-spacing: 4px; background: #f5f5f5; padding: 16px; text-align: center; border-radius: 4px; }
	</style>
</head>
<body>
	<div class="container">
		<h2>密码重置请求</h2>
		<p>您好，<strong>%s</strong>：</p>
		<p>我们收到了您的密码重置请求。您的验证码是：</p>
		<div class="code">%s</div>
		<p>验证码有效期为 30 分钟。如果这不是您的操作，请忽略此邮件。</p>
		<p>或者点击以下链接直接重置密码：</p>
		<p><a href="%s" class="button">重置密码</a></p>
		<p>如果您无法点击上方按钮，请将以下链接复制到浏览器地址栏：</p>
		<p style="word-break: break-all; color: #666; font-size: 12px;">%s</p>
		<hr style="margin: 20px 0; border: none; border-top: 1px solid #eee;">
		<p style="font-size: 12px; color: #999;">此邮件由系统自动发送，请勿回复。</p>
	</div>
</body>
</html>
`, username, code, resetURL, resetURL)

	// 发送邮件
	return sendEmail(cfg, []string{email}, subject, body)
}

// sendEmail 发送邮件
func sendEmail(cfg *config.EmailConfig, to []string, subject, htmlBody string) error {
	smtpCfg := cfg.SMTP
	auth := smtp.PlainAuth("", smtpCfg.Username, smtpCfg.Password, smtpCfg.Host)

	addr := fmt.Sprintf("%s:%d", smtpCfg.Host, smtpCfg.Port)

	// 构建邮件内容
	var content bytes.Buffer
	content.WriteString(fmt.Sprintf("From: %s <%s>\r\n", smtpCfg.FromName, smtpCfg.From))
	content.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
	content.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	content.WriteString("MIME-Version: 1.0\r\n")
	content.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	content.WriteString(htmlBody)

	return smtp.SendMail(addr, auth, smtpCfg.From, to, content.Bytes())
}

// getBaseURL 获取基础 URL（用于生成重置链接）
func getBaseURL() string {
	if baseURL := os.Getenv("BASE_URL"); baseURL != "" {
		return strings.TrimSuffix(baseURL, "/")
	}
	return "http://localhost:9000" // 默认本地地址
}

// MaskEmail 脱敏邮箱地址
func MaskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}
	username := parts[0]
	domain := parts[1]

	if len(username) <= 3 {
		return email
	}

	// 只显示首字母和最后一位，中间用***代替
	maskedUsername := string(username[0]) + "***" + string(username[len(username)-1])
	return maskedUsername + "@" + domain
}

// ForgotPasswordRequest 忘记密码请求
type ForgotPasswordRequest struct {
	Username string `json:"username"`
}

// ForgotPasswordResponse 忘记密码响应
type ForgotPasswordResponse struct {
	Email     string `json:"email"`      // 脱敏邮箱
	MaskedEmail string `json:"masked_email"` // 脱敏邮箱
	Message   string `json:"message"`
}

// ResetPasswordRequest 重置密码请求
type ResetPasswordRequest struct {
	Token       string `json:"token"`
	Code        string `json:"code"`        // 可选，用于验证码验证
	NewPassword string `json:"new_password"`
}
