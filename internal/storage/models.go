package storage

import (
	"time"

	"gorm.io/gorm"
)

// Agent Agent 数据模型
type Agent struct {
	ID           string        `gorm:"primaryKey" json:"id"`
	Hostname     string        `gorm:"index" json:"hostname"`
	IP           string        `json:"ip"`
	Region       string        `gorm:"index" json:"region"`
	Zone         string        `gorm:"index" json:"zone"`
	Version      string        `json:"version"`
	Capabilities []Capability  `gorm:"serializer:json" json:"capabilities"`
	Status       string        `gorm:"index;default:offline" json:"status"` // online, offline
	LastSeen     time.Time     `json:"lastSeen"`
	FirstSeen    time.Time     `json:"firstSeen"`
	CreatedAt    time.Time     `json:"createdAt"`
	UpdatedAt    time.Time     `json:"updatedAt"`

	// 虚拟化信息
	VirtType     string `gorm:"index" json:"virtType"`     // kvm, vmware, docker, none
	VirtRole     string `json:"virtRole"`                 // guest, host

	// 标签（支持复杂类型）
	Labels       map[string]interface{} `gorm:"serializer:json" json:"labels"`

	// 认证：SecretHash 存储 Agent 认证密钥的哈希值
	SecretHash   string `gorm:"column" json:"-"`

	// 关联资源
	ComputeInstances []ComputeInstance `gorm:"foreignKey:AgentID" json:"-"`
	Domains          []Domain          `gorm:"foreignKey:AgentID" json:"-"`
	Certificates     []Certificate     `gorm:"foreignKey:AgentID" json:"-"`
	Services         []Service         `gorm:"foreignKey:AgentID" json:"-"`
	Gateways         []Gateway         `gorm:"foreignKey:AgentID" json:"-"`
	Storages         []Storage         `gorm:"foreignKey:AgentID" json:"-"`
}

// Capability 能力定义
type Capability struct {
	Type    string                 `json:"type"`
	Version string                 `json:"version"`
	Config  map[string]interface{} `json:"config"`
}

// ComputeInstance 计算实例
type ComputeInstance struct {
	ID        string            `gorm:"primaryKey" json:"id"`
	Name      string            `gorm:"index" json:"name"`
	AgentID   string            `gorm:"index;not null" json:"agentId"`
	Type      string            `gorm:"index" json:"type"` // vm, container, baremetal
	Provider  string            `json:"provider"`         // pve, docker, etc
	Region    string            `gorm:"index" json:"region"`
	Zone      string            `gorm:"index" json:"zone"`
	Status    string            `gorm:"index" json:"status"` // running, stopped, error
	CPUCores  int               `json:"cpuCores"`
	MemoryMB  int               `json:"memoryMb"`
	DiskGB    int               `json:"diskGb"`
	IPv4      string            `json:"ipv4"`
	IPv6      string            `json:"ipv6"`
	Tags      []string          `gorm:"serializer:json" json:"tags"`
	Labels    map[string]string `gorm:"serializer:json" json:"labels"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`

	Agent *Agent `gorm:"foreignKey:AgentID" json:"-"`
}

// Domain 域名
type Domain struct {
	ID        string            `gorm:"primaryKey" json:"id"`
	Domain    string            `gorm:"uniqueIndex;not null" json:"domain"`
	AgentID   *string           `gorm:"index" json:"agentId"`
	Provider  string            `json:"provider"`
	Status    string            `gorm:"index" json:"status"` // active, expired, pending
	ExpiresAt *time.Time        `json:"expiresAt"`
	AutoRenew bool              `gorm:"default:false" json:"autoRenew"`
	Tags      []string          `gorm:"serializer:json" json:"tags"`
	Labels    map[string]string `gorm:"serializer:json" json:"labels"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`

	Agent       *Agent        `gorm:"foreignKey:AgentID" json:"-"`
	Certificates []Certificate `gorm:"foreignKey:DomainID" json:"-"`
}

// Certificate SSL 证书
type Certificate struct {
	ID              string            `gorm:"primaryKey" json:"id"`
	DomainID        *string           `gorm:"index" json:"domainId"`
	AgentID         *string           `gorm:"index" json:"agentId"`
	DomainName      string            `gorm:"not null;index" json:"domainName"`
	Issuer          string            `json:"issuer"`
	Status          string            `gorm:"index" json:"status"` // valid, expiring, expired
	ExpiresAt       time.Time         `gorm:"index" json:"expiresAt"`
	AutoRenew       bool              `gorm:"default:false" json:"autoRenew"`
	RenewBeforeDays int               `gorm:"default:30" json:"renewBeforeDays"`
	Tags            []string          `gorm:"serializer:json" json:"tags"`
	Labels          map[string]string `gorm:"serializer:json" json:"labels"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`

	Domain *Domain `gorm:"foreignKey:DomainID" json:"-"`
	Agent  *Agent  `gorm:"foreignKey:AgentID" json:"-"`
}

// Service 服务
type Service struct {
	ID             string            `gorm:"primaryKey" json:"id"`
	Name           string            `gorm:"index" json:"name"`
	AgentID        *string           `gorm:"index" json:"agentId"`
	Type           string            `json:"type"` // http, tcp, database
	URL            string            `json:"url"`
	Status         string            `gorm:"index" json:"status"` // up, down, degraded
	ResponseTimeMs int               `json:"responseTimeMs"`
	LastCheck      *time.Time        `json:"lastCheck"`
	Tags           []string          `gorm:"serializer:json" json:"tags"`
	Labels         map[string]string `gorm:"serializer:json" json:"labels"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`

	Agent *Agent `gorm:"foreignKey:AgentID" json:"-"`
}

// Gateway 网关
type Gateway struct {
	ID        string            `gorm:"primaryKey" json:"id"`
	Name      string            `gorm:"index" json:"name"`
	AgentID   string            `gorm:"index;not null" json:"agentId"`
	Type      string            `json:"type"` // openwrt, pfsense, etc
	IPv4      string            `json:"ipv4"`
	IPv6      string            `json:"ipv6"`
	Upstream  string            `json:"upstream"`
	Status    string            `gorm:"index" json:"status"`
	Tags      []string          `gorm:"serializer:json" json:"tags"`
	Labels    map[string]string `gorm:"serializer:json" json:"labels"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`

	Agent *Agent `gorm:"foreignKey:AgentID" json:"-"`
}

// Storage 存储
type Storage struct {
	ID          string            `gorm:"primaryKey" json:"id"`
	Name        string            `gorm:"index" json:"name"`
	AgentID     string            `gorm:"index;not null" json:"agentId"`
	Type        string            `json:"type"` // nfs, iscsi, local, ceph
	Path        string            `json:"path"`
	TotalGB     int               `json:"totalGb"`
	UsedGB      int               `json:"usedGb"`
	AvailableGB int               `json:"availableGb"`
	Status      string            `gorm:"index" json:"status"`
	Tags        []string          `gorm:"serializer:json" json:"tags"`
	Labels      map[string]string `gorm:"serializer:json" json:"labels"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`

	Agent *Agent `gorm:"foreignKey:AgentID" json:"-"`
}

// BeforeCreate GORM hook
func (a *Agent) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	a.FirstSeen = now
	a.LastSeen = now
	if a.Status == "" {
		a.Status = "offline"
	}
	return nil
}

// ComputeInstanceFilter 计算实例过滤条件
type ComputeInstanceFilter struct {
	Region string
	Zone   string
	Type   string
	Status string
}
