package storage

import (
	"time"
)

// DDNSConfig 一条 DDNS 记录配置（见 docs/guide/ddns-design.md D5）：
// server 巡检时让绑定的 agent 探测公网 IP，与 Cloudflare 上
// ZoneName 中的 RecordName 记录比对，缺失则建、变化则改。
type DDNSConfig struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	AgentID    string    `gorm:"index;size:64" json:"agentId"`
	ZoneID     string    `gorm:"size:64" json:"zoneId"`
	ZoneName   string    `gorm:"size:255" json:"zoneName"`
	RecordName string    `gorm:"size:255" json:"recordName"` // 完整记录名，如 home.example.com
	Type       string    `gorm:"size:8" json:"type"`         // A / AAAA
	Enabled    bool      `json:"enabled"`
	LastIP     string    `gorm:"size:64" json:"lastIP"`     // 最近一次同步到 DNS 的 IP
	LastStatus string    `gorm:"size:16" json:"lastStatus"` // never / ok / failed
	LastError  string    `gorm:"size:512" json:"lastError"` // failed 时的原因
	CheckedAt  int64     `json:"checkedAt"`                 // 最近一次检查的 Unix 秒，0 = 从未
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// CreateDDNSConfig 新建 DDNS 配置
func (d *DB) CreateDDNSConfig(cfg *DDNSConfig) error {
	return d.db.Create(cfg).Error
}

// UpdateDDNSConfig 按主键更新配置全部字段
func (d *DB) UpdateDDNSConfig(cfg *DDNSConfig) error {
	return d.db.Save(cfg).Error
}

// GetDDNSConfig 按主键取配置
func (d *DB) GetDDNSConfig(id uint) (*DDNSConfig, error) {
	var cfg DDNSConfig
	if err := d.db.First(&cfg, id).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ListDDNSConfigs 全部 DDNS 配置（按创建顺序）
func (d *DB) ListDDNSConfigs() ([]*DDNSConfig, error) {
	var list []*DDNSConfig
	if err := d.db.Order("id").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteDDNSConfig 按主键删除配置
func (d *DB) DeleteDDNSConfig(id uint) error {
	return d.db.Delete(&DDNSConfig{}, id).Error
}
