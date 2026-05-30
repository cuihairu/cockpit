package storage

import (
	"time"
)

// AuditLog 审计日志
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    string    `gorm:"index;size:50" json:"user_id"`
	Username  string    `gorm:"size:100;index" json:"username"`
	Action    string    `gorm:"size:50;index" json:"action"`         // login, logout, create, update, delete, view
	Resource  string    `gorm:"size:100;index" json:"resource"`      // user, agent, domain, certificate, service, etc.
	ResourceID string   `gorm:"size:100" json:"resource_id"`         // 资源ID
	Details   string    `gorm:"type:text" json:"details"`            // JSON格式的详细信息
	IP        string    `gorm:"size:50" json:"ip"`
	UserAgent string    `gorm:"size:500" json:"user_agent"`
	Status    string    `gorm:"size:20;index" json:"status"`         // success, failure
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// TableName 指定表名
func (AuditLog) TableName() string {
	return "audit_logs"
}

// CreateAuditLog 创建审计日志
func (d *DB) CreateAuditLog(log *AuditLog) error {
	return d.db.Create(log).Error
}

// GetAuditLogs 获取审计日志列表
func (d *DB) GetAuditLogs(offset, limit int, filters map[string]interface{}) ([]AuditLog, int64, error) {
	var logs []AuditLog
	var total int64

	query := d.db.Model(&AuditLog{})

	// 应用过滤条件
	if action, ok := filters["action"]; ok {
		query = query.Where("action = ?", action)
	}
	if resource, ok := filters["resource"]; ok {
		query = query.Where("resource = ?", resource)
	}
	if username, ok := filters["username"]; ok {
		query = query.Where("username LIKE ?", "%"+username.(string)+"%")
	}
	if status, ok := filters["status"]; ok {
		query = query.Where("status = ?", status)
	}
	if startTime, ok := filters["start_time"]; ok {
		query = query.Where("created_at >= ?", startTime)
	}
	if endTime, ok := filters["end_time"]; ok {
		query = query.Where("created_at <= ?", endTime)
	}

	// 计数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询，按时间倒序
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// GetAuditLogByID 获取单条审计日志
func (d *DB) GetAuditLogByID(id uint) (*AuditLog, error) {
	var log AuditLog
	err := d.db.First(&log, id).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

// DeleteAuditLogsBefore 删除指定时间之前的日志（用于清理）
func (d *DB) DeleteAuditLogsBefore(before time.Time) (int64, error) {
	result := d.db.Where("created_at < ?", before).Delete(&AuditLog{})
	return result.RowsAffected, result.Error
}

// GetAuditLogStats 获取审计日志统计
func (d *DB) GetAuditLogStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// 查询总数
	var total int64
	if err := d.db.Model(&AuditLog{}).Count(&total).Error; err == nil {
		stats["total_logs"] = total
	}

	// 查询今天的日志
	var todayCount int64
	today := time.Now().Truncate(24 * time.Hour)
	if err := d.db.Model(&AuditLog{}).Where("created_at >= ?", today).Count(&todayCount).Error; err == nil {
		stats["today_logs"] = todayCount
	}

	// 查询失败的日志
	var failedCount int64
	if err := d.db.Model(&AuditLog{}).Where("status = ?", "failure").Count(&failedCount).Error; err == nil {
		stats["failed_logs"] = failedCount
	}

	return stats, nil
}
