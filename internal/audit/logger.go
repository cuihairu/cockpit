package audit

import (
	"encoding/json"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

// Action 操作类型
const (
	ActionLogin    = "login"
	ActionLogout   = "logout"
	ActionCreate   = "create"
	ActionUpdate   = "update"
	ActionDelete   = "delete"
	ActionView     = "view"
	ActionExport   = "export"
	ActionImport   = "import"
	ActionStart    = "start"
	ActionStop     = "stop"
	ActionRestart  = "restart"
	ActionTOTPEnable = "totp_enable"
	ActionTOTPDisable = "totp_disable"
	ActionTOTPVerify = "totp_verify"
	// ActionRemoteStart / ActionRemoteEnd 远控会话开始/结束
	ActionRemoteStart = "remote_start"
	ActionRemoteEnd   = "remote_end"
)

// ResourceRemoteSession 远控会话资源类型
const ResourceRemoteSession = "remote_session"

// Status 状态
const (
	StatusSuccess = "success"
	StatusFailure = "failure"
)

// LogEntry 日志条目
type LogEntry struct {
	UserID     string
	Username   string
	Action     string
	Resource   string
	ResourceID string
	Details    interface{}
	IP         string
	UserAgent  string
	Status     string
}

// Logger 审计日志记录器
type Logger struct {
	db *storage.DB
}

// NewLogger 创建新的日志记录器
func NewLogger(db *storage.DB) *Logger {
	return &Logger{db: db}
}

// Log 记录审计日志
func (l *Logger) Log(entry *LogEntry) error {
	detailsJSON := ""
	if entry.Details != nil {
		bytes, err := json.Marshal(entry.Details)
		if err == nil {
			detailsJSON = string(bytes)
		}
	}

	log := &storage.AuditLog{
		UserID:     entry.UserID,
		Username:   entry.Username,
		Action:     entry.Action,
		Resource:   entry.Resource,
		ResourceID: entry.ResourceID,
		Details:    detailsJSON,
		IP:         entry.IP,
		UserAgent:  entry.UserAgent,
		Status:     entry.Status,
		CreatedAt:  time.Now(),
	}

	return l.db.CreateAuditLog(log)
}

// LogSuccess 记录成功操作
func (l *Logger) LogSuccess(username, action, resource, resourceID string, details interface{}, ip, userAgent string) error {
	return l.Log(&LogEntry{
		Username:   username,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Details:    details,
		IP:         ip,
		UserAgent:  userAgent,
		Status:     StatusSuccess,
	})
}

// LogFailure 记录失败操作
func (l *Logger) LogFailure(username, action, resource, resourceID string, details interface{}, ip, userAgent string) error {
	return l.Log(&LogEntry{
		Username:   username,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Details:    details,
		IP:         ip,
		UserAgent:  userAgent,
		Status:     StatusFailure,
	})
}

// LogLogin 记录登录操作
func (l *Logger) LogLogin(username string, success bool, ip, userAgent string) error {
	status := StatusSuccess
	if !success {
		status = StatusFailure
	}
	return l.Log(&LogEntry{
		Username:  username,
		Action:    ActionLogin,
		Resource:  "user",
		Details:   map[string]interface{}{"login_type": "password"},
		IP:        ip,
		UserAgent: userAgent,
		Status:    status,
	})
}

// LogLogout 记录登出操作
func (l *Logger) LogLogout(username string, ip, userAgent string) error {
	return l.Log(&LogEntry{
		Username:  username,
		Action:    ActionLogout,
		Resource:  "user",
		IP:        ip,
		UserAgent: userAgent,
		Status:    StatusSuccess,
	})
}

// LogResource 记录资源操作
func (l *Logger) LogResource(username, action, resource, resourceID string, details interface{}, ip, userAgent string) error {
	return l.Log(&LogEntry{
		Username:   username,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Details:    details,
		IP:         ip,
		UserAgent:  userAgent,
		Status:     StatusSuccess,
	})
}

// LogTOTPEnabled 记录 TOTP 启用操作
func (l *Logger) LogTOTPEnabled(userID, ip, userAgent string) error {
	return l.Log(&LogEntry{
		Username:  userID,
		Action:    ActionTOTPEnable,
		Resource:  "totp",
		IP:        ip,
		UserAgent: userAgent,
		Status:    StatusSuccess,
	})
}

// LogTOTPDisabled 记录 TOTP 禁用操作
func (l *Logger) LogTOTPDisabled(userID, ip, userAgent string) error {
	return l.Log(&LogEntry{
		Username:  userID,
		Action:    ActionTOTPDisable,
		Resource:  "totp",
		IP:        ip,
		UserAgent: userAgent,
		Status:    StatusSuccess,
	})
}

// LogTOTPVerified 记录 TOTP 验证操作
func (l *Logger) LogTOTPVerified(userID, ip, userAgent string, usedBackup bool) error {
	details := map[string]interface{}{"used_backup": usedBackup}
	return l.Log(&LogEntry{
		Username:  userID,
		Action:    ActionTOTPVerify,
		Resource:  "totp",
		Details:   details,
		IP:        ip,
		UserAgent: userAgent,
		Status:    StatusSuccess,
	})
}

// LogTOTPFailed 记录 TOTP 验证失败操作
func (l *Logger) LogTOTPFailed(userID, ip, userAgent string) error {
	return l.Log(&LogEntry{
		Username:  userID,
		Action:    ActionTOTPVerify,
		Resource:  "totp",
		IP:        ip,
		UserAgent: userAgent,
		Status:    StatusFailure,
	})
}

// RemoteSessionDetails 远控会话审计详情
//
// 敏感字段（password/private key）由调用方保证不写入。
type RemoteSessionDetails struct {
	Protocol string `json:"protocol"`
	AgentID  string `json:"agent_id"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Session  string `json:"session_id"`
	Duration string `json:"duration,omitempty"` // 仅 end 事件
	Reason   string `json:"reason,omitempty"`   // 仅 end 事件
}

// LogRemoteSession 记录远控会话审计日志
//
// action 取 ActionRemoteStart 或 ActionRemoteEnd；status 取 StatusSuccess/StatusFailure。
func (l *Logger) LogRemoteSession(action, userID, username, status, ip, userAgent string, details *RemoteSessionDetails) error {
	return l.Log(&LogEntry{
		UserID:     userID,
		Username:   username,
		Action:     action,
		Resource:   ResourceRemoteSession,
		ResourceID: details.Session,
		Details:    details,
		IP:         ip,
		UserAgent:  userAgent,
		Status:     status,
	})
}
