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
	AgentOffline        = "agent.offline"
	DomainExpired       = "domain.expired"
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

	// 检查事件是否启用
	for _, eventCfg := range cfg.Events {
		if eventCfg.Type == eventType && eventCfg.Enabled {
			return &Event{
				Type:   eventType,
				Labels: buildEventLabels(alert),
			}
		}
	}

	return nil
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
