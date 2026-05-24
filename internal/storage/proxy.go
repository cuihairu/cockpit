package storage

import (
	"time"
)

// Proxy 代理配置
type Proxy struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:100;not null" json:"name"`
	AgentID     string    `gorm:"size:100;index;not null" json:"agentId"`
	ProxyType   string    `gorm:"size:10;not null" json:"proxyType"`   // tcp / udp
	RemotePort  int       `gorm:"not null" json:"remotePort"`           // Server 监听的端口
	Target      string    `gorm:"size:255;not null" json:"target"`      // 目标地址，如 192.168.31.1:80
	Description string    `gorm:"size:500" json:"description"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CreatedBy   string    `gorm:"size:100" json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (Proxy) TableName() string {
	return "proxies"
}

// CreateProxy 创建代理配置
func (d *DB) CreateProxy(proxy *Proxy) error {
	return d.db.Create(proxy).Error
}

// GetProxy 获取单个代理配置
func (d *DB) GetProxy(id string) (*Proxy, error) {
	var proxy Proxy
	err := d.db.First(&proxy, "id = ?", id).Error
	if err == nil {
		return &proxy, nil
	}
	return nil, err
}

// ListProxies 获取代理配置列表
func (d *DB) ListProxies(agentID string) ([]*Proxy, error) {
	var proxies []*Proxy
	query := d.db.Order("created_at DESC")
	if agentID != "" {
		query = query.Where("agent_id = ?", agentID)
	}
	err := query.Find(&proxies).Error
	return proxies, err
}

// ListEnabledProxies 获取启用的代理配置
func (d *DB) ListEnabledProxies() ([]*Proxy, error) {
	var proxies []*Proxy
	err := d.db.Where("enabled = ?", true).Find(&proxies).Error
	return proxies, err
}

// UpdateProxy 更新代理配置
func (d *DB) UpdateProxy(proxy *Proxy) error {
	return d.db.Save(proxy).Error
}

// DeleteProxy 删除代理配置
func (d *DB) DeleteProxy(id string) error {
	return d.db.Delete(&Proxy{}, "id = ?", id).Error
}

// GetProxyByRemotePort 根据远程端口获取代理配置
func (d *DB) GetProxyByRemotePort(port int) (*Proxy, error) {
	var proxy Proxy
	err := d.db.Where("remote_port = ? AND enabled = ?", port, true).First(&proxy).Error
	if err == nil {
		return &proxy, nil
	}
	return nil, err
}
