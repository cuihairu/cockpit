package alert

import (
	"fmt"
	"log"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/storage"
)

// Generator 警告生成器
type Generator struct {
	db              *storage.DB
	notification    *notification.Client
	notificationCfg *config.NotificationConfig
}

// NewGenerator 创建警告生成器
func NewGenerator(db *storage.DB, notif *notification.Client, notifCfg *config.NotificationConfig) *Generator {
	return &Generator{
		db:              db,
		notification:    notif,
		notificationCfg: notifCfg,
	}
}

// CheckAllChecks 检查所有警告条件
func (g *Generator) CheckAllChecks() {
	g.CheckExpiringCertificates()
	g.CheckDownServices()
	g.CheckOfflineAgents()
	g.CheckExpiredDomains()
}

// CheckExpiringCertificates 检查即将过期的证书
func (g *Generator) CheckExpiringCertificates() {
	certificates, err := g.db.ListCertificates()
	if err != nil {
		log.Printf("Failed to list certificates: %v", err)
		return
	}

	now := time.Now()
	for _, cert := range certificates {
		if cert.Status != "valid" {
			continue
		}

		daysUntilExpiry := int(cert.ExpiresAt.Sub(now).Hours() / 24)

		var alertType string
		var title string
		var shouldAlert bool

		switch {
		case daysUntilExpiry <= 0:
			alertType = "error"
			title = "证书已过期"
			shouldAlert = true
		case daysUntilExpiry <= 7:
			alertType = "error"
			title = "证书即将过期（7天内）"
			shouldAlert = true
		case daysUntilExpiry <= 30:
			alertType = "warning"
			title = "证书即将过期（30天内）"
			shouldAlert = true
		}

		if shouldAlert {
			message := fmt.Sprintf("域名 %s 的证书将在 %d 天后过期", cert.DomainName, daysUntilExpiry)
			g.createAlertIfNotExists(alertType, title, message, cert.ID, "certificate")
		}
	}
}

// CheckDownServices 检查宕机服务
func (g *Generator) CheckDownServices() {
	services, err := g.db.ListServices()
	if err != nil {
		log.Printf("Failed to list services: %v", err)
		return
	}

	for _, service := range services {
		if service.Status == "down" {
			title := "服务宕机"
			message := fmt.Sprintf("服务 %s (%s) 处于宕机状态", service.Name, service.Type)
			g.createAlertIfNotExists("error", title, message, service.ID, "service")
		}
	}
}

// CheckOfflineAgents 检查离线 Agent
func (g *Generator) CheckOfflineAgents() {
	agents, err := g.db.ListAgents()
	if err != nil {
		log.Printf("Failed to list agents: %v", err)
		return
	}

	for _, agent := range agents {
		if agent.Status == "offline" {
			title := "Agent 离线"
			message := fmt.Sprintf("Agent %s (%s) 已离线", agent.Hostname, agent.IP)
			g.createAlertIfNotExists("warning", title, message, agent.ID, "agent")
		}
	}
}

// CheckExpiredDomains 检查过期域名
func (g *Generator) CheckExpiredDomains() {
	domains, err := g.db.ListDomains()
	if err != nil {
		log.Printf("Failed to list domains: %v", err)
		return
	}

	for _, domain := range domains {
		if domain.Status == "expired" {
			title := "域名已过期"
			message := fmt.Sprintf("域名 %s 已过期", domain.Domain)
			g.createAlertIfNotExists("error", title, message, domain.ID, "domain")
		}
	}
}

// CheckDiskSpace 检查磁盘空间
func (g *Generator) CheckDiskSpace(thresholdPercent int) {
	// TODO: 实现磁盘空间检查
	// 需要从 Agent 获取磁盘使用情况
}

// CheckMemoryUsage 检查内存使用
func (g *Generator) CheckMemoryUsage(thresholdPercent int) {
	// TODO: 实现内存使用检查
	// 需要从 Agent 获取内存使用情况
}

// createAlertIfNotExists 如果不存在则创建警告
func (g *Generator) createAlertIfNotExists(alertType, title, message, resourceID, resourceType string) {
	// 检查是否已存在相同类型的未读警告
	// 这里简化处理，直接创建
	alert := &storage.Alert{
		Type:         alertType,
		Title:        title,
		Message:      message,
		ResourceID:   &resourceID,
		ResourceType: &resourceType,
		Read:         false,
	}

	if err := g.db.CreateAlert(alert); err != nil {
		log.Printf("Failed to create alert: %v", err)
	}

	// 发送外部通知（非阻塞）
	notification.SendAlertNonBlocking(g.notification, alert, g.notificationCfg)
}

// CleanupOldAlerts 清理旧警告
func (g *Generator) CleanupOldAlerts(olderThan time.Duration) {
	if err := g.db.DeleteOldAlerts(olderThan); err != nil {
		log.Printf("Failed to cleanup old alerts: %v", err)
	}
}
