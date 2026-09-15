package alert

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 告警阈值 Setting 键（见 docs/guide/probe-enhance-design.md M2 / D12）
const (
	DiskThresholdSettingKey   = "alert.disk_percent"
	MemoryThresholdSettingKey = "alert.memory_percent"
	CertWarnDaysSettingKey    = "alert.cert_warn_days"
	CertInfoDaysSettingKey    = "alert.cert_info_days"
)

// 阈值边界与默认值（API 层校验同规则）
const (
	MinPercentThreshold    = 50
	MaxPercentThreshold    = 99
	DefaultDiskThreshold   = 80
	DefaultMemoryThreshold = 85

	MinCertWarnDays     = 1
	MaxCertWarnDays     = 90
	DefaultCertWarnDays = 7

	MinCertInfoDays     = 1
	MaxCertInfoDays     = 365
	DefaultCertInfoDays = 30
)

// Generator 警告生成器
type Generator struct {
	db              *storage.DB
	notifier        *notification.Service
	notificationCfg *config.NotificationConfig
	// 阈值（M2 可配置）：CheckAllChecks 每轮从 Setting 表刷新，
	// 单测直接调用单项 Check 时用初始默认值
	diskThreshold int
	memThreshold  int
	certWarnDays  int
	certInfoDays  int
}

// NewGenerator 创建警告生成器
func NewGenerator(db *storage.DB, notifier *notification.Service, notifCfg *config.NotificationConfig) *Generator {
	return &Generator{
		db:              db,
		notifier:        notifier,
		notificationCfg: notifCfg,
		diskThreshold:   DefaultDiskThreshold,
		memThreshold:    DefaultMemoryThreshold,
		certWarnDays:    DefaultCertWarnDays,
		certInfoDays:    DefaultCertInfoDays,
	}
}

// CheckAllChecks 检查所有警告条件
func (g *Generator) CheckAllChecks() {
	g.loadThresholds()
	g.CheckExpiringCertificates()
	g.CheckDownServices()
	g.CheckOfflineAgents()
	g.CheckExpiredDomains()
	g.CheckDiskSpace(g.diskThreshold)
	g.CheckMemoryUsage(g.memThreshold)
}

// loadThresholds 从 Setting 表读取告警阈值（M2/D13：小时级轮询每轮读一次，
// 无缓存必要）；未配置或非法时保持当前值
func (g *Generator) loadThresholds() {
	applyInt := func(key string, minV, maxV int, dst *int) {
		v, err := g.db.GetSetting(key)
		if err != nil || v == "" {
			return
		}
		if n, convErr := strconv.Atoi(v); convErr == nil && n >= minV && n <= maxV {
			*dst = n
		}
	}
	applyInt(DiskThresholdSettingKey, MinPercentThreshold, MaxPercentThreshold, &g.diskThreshold)
	applyInt(MemoryThresholdSettingKey, MinPercentThreshold, MaxPercentThreshold, &g.memThreshold)
	applyInt(CertWarnDaysSettingKey, MinCertWarnDays, MaxCertWarnDays, &g.certWarnDays)
	applyInt(CertInfoDaysSettingKey, MinCertInfoDays, MaxCertInfoDays, &g.certInfoDays)
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
		case daysUntilExpiry <= g.certWarnDays:
			alertType = "error"
			title = fmt.Sprintf("证书即将过期（%d天内）", g.certWarnDays)
			shouldAlert = true
		case daysUntilExpiry <= g.certInfoDays:
			alertType = "warning"
			title = fmt.Sprintf("证书即将过期（%d天内）", g.certInfoDays)
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
	if g == nil || g.db == nil {
		return
	}
	// 获取最新的系统信息快照
	snapshots, err := g.db.ListSystemInfoSnapshots()
	if err != nil {
		log.Printf("Failed to get system snapshots: %v", err)
		return
	}

	for _, snapshot := range snapshots {
		// 检查磁盘使用率
		if snapshot.DiskUsagePercent >= float64(thresholdPercent) {
			title := "磁盘空间不足"
			message := fmt.Sprintf("Agent %s (%s) 磁盘使用率已达 %.1f%%，超过阈值 %d%%",
				snapshot.Hostname, snapshot.AgentID, snapshot.DiskUsagePercent, thresholdPercent)
			g.createAlertIfNotExists("warning", title, message, snapshot.AgentID, "agent")
		}
	}
}

// CheckMemoryUsage 检查内存使用
func (g *Generator) CheckMemoryUsage(thresholdPercent int) {
	if g == nil || g.db == nil {
		return
	}
	// 获取最新的系统信息快照
	snapshots, err := g.db.ListSystemInfoSnapshots()
	if err != nil {
		log.Printf("Failed to get system snapshots: %v", err)
		return
	}

	for _, snapshot := range snapshots {
		// 检查内存使用率
		if snapshot.MemUsagePercent >= float64(thresholdPercent) {
			title := "内存使用率过高"
			message := fmt.Sprintf("Agent %s (%s) 内存使用率已达 %.1f%%，超过阈值 %d%%",
				snapshot.Hostname, snapshot.AgentID, snapshot.MemUsagePercent, thresholdPercent)
			g.createAlertIfNotExists("warning", title, message, snapshot.AgentID, "agent")
		}
	}
}

// createAlertIfNotExists 同资源同标题不存在未读告警时才创建（M2/D15 真去重）。
// 轮询期间问题持续存在只产生一条告警、只通知一次；用户标已读后同一问题
// 再现会重新创建并通知（已读=已知晓，再现值得提醒）。查重失败按无重复
// 处理，宁多勿漏。
func (g *Generator) createAlertIfNotExists(alertType, title, message, resourceID, resourceType string) {
	exists, err := g.db.HasUnreadAlert(resourceType, resourceID, title)
	if err != nil {
		log.Printf("Failed to check existing alerts: %v", err)
	} else if exists {
		return
	}

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
		return
	}

	// 发送外部通知（非阻塞，多渠道扇出；事件白名单在 Service 内过滤）。
	// 仅在真正创建新告警时通知，去重命中的轮次保持安静。
	g.notifier.SendAlertNonBlocking(alert)
}

// CleanupOldAlerts 清理旧警告
func (g *Generator) CleanupOldAlerts(olderThan time.Duration) {
	if err := g.db.DeleteOldAlerts(olderThan); err != nil {
		log.Printf("Failed to cleanup old alerts: %v", err)
	}
}

// DriftAlertMaxItems 漂移告警明细最多列出的对象数（超出截断提示）
const DriftAlertMaxItems = 10

// CheckDriftScan 漂移巡检结果入告警（M2/D16）：driftedNames 非空时每
// agent 一条汇总告警，title 固定 → 同主机未读存在期间只报一次（真去重）；
// 空列表（全部一致）无动作，不自动 resolve——用户处理后标已读，再次漂移
// 会重新创建并通知。
func (g *Generator) CheckDriftScan(agentID, hostname string, driftedNames []string) {
	if len(driftedNames) == 0 {
		return
	}
	title := "配置漂移：主机 " + hostname
	detail := driftedNames
	suffix := ""
	if len(detail) > DriftAlertMaxItems {
		detail = detail[:DriftAlertMaxItems]
		suffix = fmt.Sprintf("（等共 %d 项）", len(driftedNames))
	}
	message := "以下对象与面板最后一次保存的内容不一致：\n" +
		strings.Join(detail, "\n")
	if suffix != "" {
		message += "\n" + suffix
	}
	g.createAlertIfNotExists("warning", title, message, agentID, "agent")
}
