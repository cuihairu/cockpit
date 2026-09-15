package storage

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProbeResult 拨测历史记录（见 docs/guide/probe-enhance-design.md M2 / D7）
//
// 每轮探测每目标一条；心跳条按 (resource_type, resource_id) 取最近 N 条。
// name 冗余一份，服务被删后历史仍可读。
type ProbeResult struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	ResourceType string    `gorm:"index:idx_probe_target,priority:1" json:"resourceType"`
	ResourceID   string    `gorm:"index:idx_probe_target,priority:2" json:"resourceId"`
	Name         string    `json:"name"`
	Status       string    `gorm:"index" json:"status"` // up, down, degraded
	LatencyMs    int       `json:"latencyMs"`
	Message      string    `json:"message"`
	CheckedAt    time.Time `gorm:"index:idx_probe_target,priority:3" json:"checkedAt"`
}

// BeforeCreate GORM hook
func (p *ProbeResult) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

// CreateProbeResults 批量写入一轮探测结果
func (d *DB) CreateProbeResults(results []*ProbeResult) error {
	if len(results) == 0 {
		return nil
	}
	return d.db.Create(&results).Error
}

// ListProbeResults 按目标取最近 limit 条历史（checked_at 倒序）
func (d *DB) ListProbeResults(resourceType, resourceID string, limit int) ([]*ProbeResult, error) {
	if limit <= 0 {
		limit = 50
	}
	var results []*ProbeResult
	err := d.db.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Order("checked_at DESC").Limit(limit).Find(&results).Error
	return results, err
}

// DeleteProbeResultsOlderThan 清理保留期外历史（D10：30 天，每 24h 一次）
func (d *DB) DeleteProbeResultsOlderThan(cutoff time.Time) (int64, error) {
	res := d.db.Where("checked_at < ?", cutoff).Delete(&ProbeResult{})
	return res.RowsAffected, res.Error
}
