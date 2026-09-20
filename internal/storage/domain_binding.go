package storage

import (
	"time"
)

// DomainBinding 服务域名绑定（见 docs/guide/domain-binding-design.md D1/D2）：
// 「域名 → agent 上的目标服务」的唯一事实源登记。apply 时按三个 auto
// 开关各自独立联动：autoDNS 建 A 记录、autoProxy 下发反代站点、
// autoCert 纳入证书监控（inventory Domain）。
type DomainBinding struct {
	ID        uint   `gorm:"primarykey" json:"id"`
	Domain    string `gorm:"uniqueIndex;size:255" json:"domain"` // blog.example.com
	AgentID   string `gorm:"index;size:64" json:"agentId"`
	Target    string `gorm:"size:255" json:"target"` // host:port 或 docker://服务名
	Enabled   bool   `json:"enabled"`
	AutoDNS   bool   `json:"autoDns"`
	AutoProxy bool   `json:"autoProxy"`
	AutoCert  bool   `json:"autoCert"`
	// 最近一次 apply 的结果快照（列表页展示与排障依据，漂移检查另算）
	LastApplyStatus string    `gorm:"size:16" json:"lastApplyStatus"` // never / ok / failed
	LastError       string    `gorm:"size:512" json:"lastError"`
	AppliedAt       int64     `json:"appliedAt"` // Unix 秒，0 = 从未
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// ListDomainBindings 全部绑定（按域名排序）
func (d *DB) ListDomainBindings() ([]*DomainBinding, error) {
	var list []*DomainBinding
	if err := d.db.Order("domain").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListDomainBindingsByAgent 某 agent 的绑定（agent 侧域名清单的数据源）
func (d *DB) ListDomainBindingsByAgent(agentID string) ([]*DomainBinding, error) {
	var list []*DomainBinding
	if err := d.db.Where("agent_id = ?", agentID).Order("domain").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// GetDomainBinding 按域名取绑定（domain 唯一）
func (d *DB) GetDomainBinding(domain string) (*DomainBinding, error) {
	var b DomainBinding
	if err := d.db.Where("domain = ?", domain).First(&b).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

// SaveDomainBinding 登记/更新（POST 语义，domain 为唯一键 upsert）
func (d *DB) SaveDomainBinding(b *DomainBinding) error {
	existing, err := d.GetDomainBinding(b.Domain)
	if err != nil {
		return d.db.Create(b).Error
	}
	b.ID = existing.ID
	// 保留最近一次 apply 快照：登记/更新不改历史 apply 状态
	b.LastApplyStatus = existing.LastApplyStatus
	b.LastError = existing.LastError
	b.AppliedAt = existing.AppliedAt
	return d.db.Save(b).Error
}

// UpdateDomainBindingApply 回写 apply 结果快照
func (d *DB) UpdateDomainBindingApply(domain, status, lastErr string, appliedAt int64) error {
	return d.db.Model(&DomainBinding{}).Where("domain = ?", domain).
		Updates(map[string]interface{}{
			"last_apply_status": status,
			"last_error":        lastErr,
			"applied_at":        appliedAt,
		}).Error
}

// DeleteDomainBinding 删登记（D8：不动已下发的 DNS/站点/监控项）
func (d *DB) DeleteDomainBinding(domain string) error {
	return d.db.Where("domain = ?", domain).Delete(&DomainBinding{}).Error
}
