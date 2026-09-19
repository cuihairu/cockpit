package notification

import (
	"strings"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// Event Herald 事件结构
type Event struct {
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels"`
}

// EventType 事件类型常量
const (
	CertificateExpired  = "certificate.expired"
	CertificateExpiring = "certificate.expiring"
	CertificateWarning  = "certificate.warning"
	ServiceDown         = "service.down"
	ServiceUp           = "service.up" // 恢复通知（probe 边沿检测，需在 events 白名单显式启用）
	AgentOffline        = "agent.offline"
	DomainExpired       = "domain.expired"
	BackupFailed        = "backup.failed"        // 备份任务失败（backup 调度循环，需在 events 白名单显式启用）
	BackupRemoteFailed  = "backup.remote-failed" // 本地成功但 rclone 推送失败（M2 D21，白名单显式启用）

	ServerBackupRemoteFailed = "server_backup.remote-failed" // 面板库本地备份成功但 rclone 推送失败（server-backup M2 D16，白名单显式启用）

	RecordingRemoteFailed = "recording.remote-failed" // 录制文件归档推送失败（本地档在，retention 窗口内可补推；recording M2 D17，白名单显式启用）
)

// getAlertEventType 根据 Alert 获取对应的事件类型
func getAlertEventType(alert *storage.Alert) string {
	if alert.ResourceType == nil {
		return ""
	}

	rt := strings.ToLower(*alert.ResourceType)

	switch rt {
	case "certificate":
		if strings.Contains(alert.Title, "已过期") {
			return CertificateExpired
		}
		if strings.Contains(alert.Title, "7天") {
			return CertificateExpiring
		}
		if strings.Contains(alert.Title, "30天") {
			return CertificateWarning
		}
	case "service":
		if alert.Type == "error" && strings.Contains(alert.Title, "宕机") {
			return ServiceDown
		}
	case "agent":
		if alert.Type == "warning" && strings.Contains(alert.Title, "离线") {
			return AgentOffline
		}
	case "domain":
		if alert.Type == "error" && strings.Contains(alert.Title, "过期") {
			return DomainExpired
		}
	}

	return ""
}

// AlertToEvent 将 Alert 转换为 Herald Event
func AlertToEvent(alert *storage.Alert, cfg *config.NotificationConfig) *Event {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	eventType := getAlertEventType(alert)
	if eventType == "" {
		return nil
	}

	// 检查事件是否启用。配置中的 map key 是业务名称，真正的事件类型在 eventCfg.Type。
	enabled := false
	for _, eventCfg := range cfg.Events {
		if eventCfg != nil && eventCfg.Type == eventType && eventCfg.Enabled {
			enabled = true
			break
		}
	}
	if !enabled {
		return nil
	}

	return &Event{
		Type:   eventType,
		Labels: buildEventLabels(alert),
	}
}

// buildEventLabels 从 Alert 构建事件标签
func buildEventLabels(alert *storage.Alert) map[string]string {
	labels := make(map[string]string)

	labels["level"] = alert.Type
	labels["title"] = alert.Title
	labels["message"] = alert.Message

	if alert.ResourceID != nil {
		labels["resource_id"] = *alert.ResourceID
	}
	if alert.ResourceType != nil {
		labels["resource_type"] = *alert.ResourceType
	}

	return labels
}
