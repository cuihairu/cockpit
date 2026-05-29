package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ErrNotFound 记录不存在错误
var ErrNotFound = errors.New("record not found")

// DB 数据库封装
type DB struct {
	db *gorm.DB
	mu sync.RWMutex
}

// Config 数据库配置
type Config struct {
	Path   string        // 数据库文件路径
	LogLevel logger.LogLevel // 日志级别
}

// Open 打开数据库连接
func Open(cfg Config) (*DB, error) {
	if cfg.Path == "" {
		cfg.Path = "cockpit.db"
	}

	// 确保目录存在
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	// 配置 GORM
	logLevel := cfg.LogLevel
	if logLevel == 0 {
		logLevel = logger.Silent // 默认静默
	}

	db, err := gorm.Open(sqlite.Open(cfg.Path), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// 配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1) // SQLite
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	d := &DB{db: db}

	// 自动迁移
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return d, nil
}

// Close 关闭数据库
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	sqlDB, err := d.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// migrate 执行数据库迁移
func (d *DB) migrate() error {
	return d.db.AutoMigrate(
		&User{},
		&Agent{},
		&Alert{},
		&ComputeInstance{},
		&Domain{},
		&Certificate{},
		&Service{},
		&Gateway{},
		&Storage{},
		&AuditLog{},
		&Proxy{},
		&SystemMetric{},
		&SystemInfoSnapshot{},
	)
}

// Session 创建新的会话
func (d *DB) Session() *gorm.DB {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.db.Session(&gorm.Session{})
}

// ============ Agent 操作 ============

// UpsertAgent 插入或更新 Agent
func (d *DB) UpsertAgent(agent *Agent) error {
	return d.db.Where("id = ?", agent.ID).
		Assign(agent).
		FirstOrCreate(agent).Error
}

// GetAgent 获取单个 Agent
func (d *DB) GetAgent(id string) (*Agent, error) {
	var agent Agent
	err := d.db.First(&agent, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &agent, err
}

// ListAgents 获取 Agent 列表
func (d *DB) ListAgents() ([]*Agent, error) {
	var agents []*Agent
	err := d.db.Order("hostname").Find(&agents).Error
	return agents, err
}

// ListAgentsByRegion 按地域获取 Agent
func (d *DB) ListAgentsByRegion(region string) ([]*Agent, error) {
	var agents []*Agent
	err := d.db.Where("region = ?", region).Order("zone, hostname").Find(&agents).Error
	return agents, err
}

// DeleteAgent 删除 Agent
func (d *DB) DeleteAgent(id string) error {
	return d.db.Delete(&Agent{}, "id = ?", id).Error
}

// UpdateAgentStatus 更新 Agent 状态
func (d *DB) UpdateAgentStatus(id string, status string, lastSeen time.Time) error {
	return d.db.Model(&Agent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":    status,
			"last_seen": lastSeen,
		}).Error
}

// CleanupOfflineAgents 清理离线 Agent
func (d *DB) CleanupOfflineAgents(timeout time.Duration) ([]string, error) {
	cutoff := time.Now().Add(-timeout)

	var agents []*Agent
	err := d.db.Where("status = ? AND last_seen < ?", "offline", cutoff).
		Or("last_seen < ?", cutoff).
		Find(&agents).Error
	if err != nil {
		return nil, err
	}

	var removed []string
	for _, agent := range agents {
		if err := d.db.Delete(agent).Error; err == nil {
			removed = append(removed, agent.ID)
		}
	}

	return removed, nil
}

// UpdateAgentSecret 更新 Agent 密钥哈希
func (d *DB) UpdateAgentSecret(agentID, secretHash string) error {
	return d.db.Model(&Agent{}).
		Where("id = ?", agentID).
		Update("secret_hash", secretHash).Error
}

// RegenerateAgentSecret 重新生成 Agent 密钥（返回新明文密钥和哈希）
func (d *DB) RegenerateAgentSecret(agentID string) (string, error) {
	secret, err := GenerateAgentSecret()
	if err != nil {
		return "", err
	}
	hash, err := HashAgentSecret(secret)
	if err != nil {
		return "", err
	}
	if err := d.UpdateAgentSecret(agentID, hash); err != nil {
		return "", err
	}
	return secret, nil
}

// ============ ComputeInstance 操作 ============

// UpsertComputeInstance 插入或更新计算实例
func (d *DB) UpsertComputeInstance(inst *ComputeInstance) error {
	return d.db.Where("id = ?", inst.ID).
		Assign(inst).
		FirstOrCreate(inst).Error
}

// GetComputeInstance 获取单个计算实例
func (d *DB) GetComputeInstance(id string) (*ComputeInstance, error) {
	var inst ComputeInstance
	err := d.db.Preload("Agent").First(&inst, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &inst, err
}

// ListComputeInstances 获取计算实例列表
func (d *DB) ListComputeInstances(filter *ComputeInstanceFilter) ([]*ComputeInstance, error) {
	query := d.db.Model(&ComputeInstance{})

	if filter != nil {
		if filter.Region != "" {
			query = query.Where("region = ?", filter.Region)
		}
		if filter.Zone != "" {
			query = query.Where("zone = ?", filter.Zone)
		}
		if filter.Type != "" {
			query = query.Where("type = ?", filter.Type)
		}
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
	}

	var instances []*ComputeInstance
	err := query.Preload("Agent").Order("name").Find(&instances).Error
	return instances, err
}

// DeleteComputeInstance 删除计算实例
func (d *DB) DeleteComputeInstance(id string) error {
	return d.db.Delete(&ComputeInstance{}, "id = ?", id).Error
}

// ============ Domain 操作 ============

// UpsertDomain 插入或更新域名
func (d *DB) UpsertDomain(domain *Domain) error {
	return d.db.Where("id = ?", domain.ID).
		Assign(domain).
		FirstOrCreate(domain).Error
}

// GetDomain 获取单个域名
func (d *DB) GetDomain(id string) (*Domain, error) {
	var domain Domain
	err := d.db.Preload("Agent").Preload("Certificates").First(&domain, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &domain, err
}

// GetDomainByName 按域名获取
func (d *DB) GetDomainByName(domain string) (*Domain, error) {
	var dom Domain
	err := d.db.Preload("Agent").First(&dom, "domain = ?", domain).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &dom, err
}

// ListDomains 获取域名列表
func (d *DB) ListDomains() ([]*Domain, error) {
	var domains []*Domain
	err := d.db.Preload("Certificates").Order("domain").Find(&domains).Error
	return domains, err
}

// DeleteDomain 删除域名
func (d *DB) DeleteDomain(id string) error {
	return d.db.Delete(&Domain{}, "id = ?", id).Error
}

// ============ Certificate 操作 ============

// UpsertCertificate 插入或更新证书
func (d *DB) UpsertCertificate(cert *Certificate) error {
	return d.db.Where("id = ?", cert.ID).
		Assign(cert).
		FirstOrCreate(cert).Error
}

// GetCertificate 获取单个证书
func (d *DB) GetCertificate(id string) (*Certificate, error) {
	var cert Certificate
	err := d.db.Preload("Domain").Preload("Agent").First(&cert, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &cert, err
}

// ListCertificates 获取证书列表
func (d *DB) ListCertificates() ([]*Certificate, error) {
	var certs []*Certificate
	err := d.db.Preload("Domain").Order("expires_at").Find(&certs).Error
	return certs, err
}

// ListExpiringCertificates 获取即将过期的证书
func (d *DB) ListExpiringCertificates(within time.Duration) ([]*Certificate, error) {
	var certs []*Certificate
	cutoff := time.Now().Add(within)
	err := d.db.Where("status = ? AND expires_at <= ?", "valid", cutoff).
		Preload("Domain").
		Order("expires_at").
		Find(&certs).Error
	return certs, err
}

// DeleteCertificate 删除证书
func (d *DB) DeleteCertificate(id string) error {
	return d.db.Delete(&Certificate{}, "id = ?", id).Error
}

// ============ Service 操作 ============

// UpsertService 插入或更新服务
func (d *DB) UpsertService(service *Service) error {
	return d.db.Where("id = ?", service.ID).
		Assign(service).
		FirstOrCreate(service).Error
}

// GetService 获取单个服务
func (d *DB) GetService(id string) (*Service, error) {
	var service Service
	err := d.db.Preload("Agent").First(&service, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &service, err
}

// ListServices 获取服务列表
func (d *DB) ListServices() ([]*Service, error) {
	var services []*Service
	err := d.db.Preload("Agent").Order("name").Find(&services).Error
	return services, err
}

// DeleteService 删除服务
func (d *DB) DeleteService(id string) error {
	return d.db.Delete(&Service{}, "id = ?", id).Error
}

// ============ Gateway 操作 ============

// UpsertGateway 插入或更新网关
func (d *DB) UpsertGateway(gateway *Gateway) error {
	return d.db.Where("id = ?", gateway.ID).
		Assign(gateway).
		FirstOrCreate(gateway).Error
}

// GetGateway 获取单个网关
func (d *DB) GetGateway(id string) (*Gateway, error) {
	var gateway Gateway
	err := d.db.Preload("Agent").First(&gateway, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &gateway, err
}

// ListGateways 获取网关列表
func (d *DB) ListGateways() ([]*Gateway, error) {
	var gateways []*Gateway
	err := d.db.Preload("Agent").Order("name").Find(&gateways).Error
	return gateways, err
}

// DeleteGateway 删除网关
func (d *DB) DeleteGateway(id string) error {
	return d.db.Delete(&Gateway{}, "id = ?", id).Error
}

// ============ Storage 操作 ============

// UpsertStorage 插入或更新存储
func (d *DB) UpsertStorage(storage *Storage) error {
	return d.db.Where("id = ?", storage.ID).
		Assign(storage).
		FirstOrCreate(storage).Error
}

// GetStorage 获取单个存储
func (d *DB) GetStorage(id string) (*Storage, error) {
	var storage Storage
	err := d.db.Preload("Agent").First(&storage, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &storage, err
}

// ListStorages 获取存储列表
func (d *DB) ListStorages() ([]*Storage, error) {
	var storages []*Storage
	err := d.db.Preload("Agent").Order("name").Find(&storages).Error
	return storages, err
}

// DeleteStorage 删除存储
func (d *DB) DeleteStorage(id string) error {
	return d.db.Delete(&Storage{}, "id = ?", id).Error
}

// ============ 统计操作 ============

// Stats 统计信息
type Stats struct {
	// Agents
	AgentsOnline int64 `json:"agentsOnline"`
	AgentsTotal  int64 `json:"agentsTotal"`

	// Compute Instances
	ComputeInstancesRunning int64 `json:"computeInstancesRunning"`
	ComputeInstancesTotal   int64 `json:"computeInstancesTotal"`

	// Domains
	DomainsActive int64 `json:"domainsActive"`

	// Certificates
	CertificatesValid    int64 `json:"certificatesValid"`
	CertificatesExpiring int64 `json:"certificatesExpiring"`

	// Services
	ServicesUp   int64 `json:"servicesUp"`
	ServicesDown int64 `json:"servicesDown"`
}

// GetStats 获取统计信息
func (d *DB) GetStats() (*Stats, error) {
	stats := &Stats{}

	// Agent 统计
	d.db.Model(&Agent{}).Where("status = ?", "online").Count(&stats.AgentsOnline)
	d.db.Model(&Agent{}).Count(&stats.AgentsTotal)

	// 计算实例统计
	d.db.Model(&ComputeInstance{}).Where("status = ?", "running").Count(&stats.ComputeInstancesRunning)
	d.db.Model(&ComputeInstance{}).Count(&stats.ComputeInstancesTotal)

	// 域名统计
	d.db.Model(&Domain{}).Where("status = ?", "active").Count(&stats.DomainsActive)

	// 证书统计
	d.db.Model(&Certificate{}).Where("status = ? AND expires_at > ?", "valid", time.Now().Add(30*24*time.Hour)).Count(&stats.CertificatesValid)
	d.db.Model(&Certificate{}).Where("expires_at <= ? AND expires_at > ?", time.Now().Add(30*24*time.Hour), time.Now()).Count(&stats.CertificatesExpiring)

	// 服务统计
	d.db.Model(&Service{}).Where("status = ?", "up").Count(&stats.ServicesUp)
	d.db.Model(&Service{}).Where("status = ?", "down").Count(&stats.ServicesDown)

	return stats, nil
}

// ============ 系统指标操作 ============

// SaveSystemMetric 保存系统指标
func (d *DB) SaveSystemMetric(metric *SystemMetric) error {
	return d.db.Create(metric).Error
}

// UpdateSystemInfoSnapshot 更新系统信息快照
func (d *DB) UpdateSystemInfoSnapshot(snapshot *SystemInfoSnapshot) error {
	return d.db.Where("agent_id = ?", snapshot.AgentID).
		Assign(snapshot).
		FirstOrCreate(snapshot).Error
}

// GetSystemInfoSnapshot 获取系统信息快照
func (d *DB) GetSystemInfoSnapshot(agentID string) (*SystemInfoSnapshot, error) {
	var snapshot SystemInfoSnapshot
	err := d.db.Where("agent_id = ?", agentID).First(&snapshot).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &snapshot, err
}

// ListSystemInfoSnapshots 获取所有系统信息快照
func (d *DB) ListSystemInfoSnapshots() ([]*SystemInfoSnapshot, error) {
	var snapshots []*SystemInfoSnapshot
	err := d.db.Find(&snapshots).Error
	return snapshots, err
}

// GetSystemMetrics 获取系统指标历史
func (d *DB) GetSystemMetrics(agentID string, limit int, offset int) ([]*SystemMetric, error) {
	var metrics []*SystemMetric
	query := d.db.Where("agent_id = ?", agentID).Order("timestamp DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Find(&metrics).Error
	return metrics, err
}

// GetSystemMetricsByTimeRange 获取指定时间范围的系统指标
func (d *DB) GetSystemMetricsByTimeRange(agentID string, start, end time.Time) ([]*SystemMetric, error) {
	var metrics []*SystemMetric
	err := d.db.Where("agent_id = ? AND timestamp BETWEEN ? AND ?", agentID, start, end).
		Order("timestamp ASC").
		Find(&metrics).Error
	return metrics, err
}

// CleanupOldMetrics 清理旧的系统指标记录
func (d *DB) CleanupOldMetrics(retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)
	result := d.db.Where("timestamp < ?", cutoff).Delete(&SystemMetric{})
	return result.RowsAffected, result.Error
}

// ============ 事务支持 ============

// Transaction 执行事务
func (d *DB) Transaction(fn func(*DB) error) error {
	return d.db.Transaction(func(tx *gorm.DB) error {
		return fn(&DB{db: tx})
	})
}
