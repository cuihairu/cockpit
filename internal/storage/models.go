package storage

import (
	"time"

	"gorm.io/gorm"
)

// Agent Agent 数据模型
type Agent struct {
	ID           string       `gorm:"primaryKey" json:"id"`
	Hostname     string       `gorm:"index" json:"hostname"`
	IP           string       `json:"ip"`
	Region       string       `gorm:"index" json:"region"`
	Zone         string       `gorm:"index" json:"zone"`
	Version      string       `json:"version"`
	Capabilities []Capability `gorm:"serializer:json" json:"capabilities"`
	Status       string       `gorm:"index;default:offline" json:"status"` // online, offline
	LastSeen     time.Time    `json:"lastSeen"`
	FirstSeen    time.Time    `json:"firstSeen"`
	CreatedAt    time.Time    `json:"createdAt"`
	UpdatedAt    time.Time    `json:"updatedAt"`

	// 虚拟化信息
	VirtType string `gorm:"index" json:"virtType"` // kvm, vmware, docker, none
	VirtRole string `json:"virtRole"`              // guest, host

	// 标签（支持复杂类型）
	Labels map[string]interface{} `gorm:"serializer:json" json:"labels"`

	// 认证：SecretHash 存储 Agent 认证密钥的哈希值
	SecretHash string `gorm:"column:secret_hash" json:"-"`

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
	Provider  string            `json:"provider"`          // pve, docker, etc
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

	Agent        *Agent        `gorm:"foreignKey:AgentID" json:"-"`
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

// Stack Compose Stack 索引缓存（真相源是 Agent 主机上的 stack 目录，
// 见 docs/guide/stack-deploy-design.md；此表仅供跨 Agent 聚合视图与
// 离线时的灰态展示）
type Stack struct {
	ID             uint      `gorm:"primarykey" json:"id"`
	AgentID        string    `gorm:"index;uniqueIndex:idx_stack_agent_name" json:"agentId"`
	Name           string    `gorm:"uniqueIndex:idx_stack_agent_name" json:"name"`
	Running        int       `json:"running"`
	Total          int       `json:"total"`
	LastAction     string    `json:"lastAction"`
	LastStatus     string    `json:"lastStatus"`
	LastDeployedAt int64     `json:"lastDeployedAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// StackDeployment Compose Stack 部署历史（server 侧记录，M1.5）。
// 启动类动作（up/down/restart/pull/remove）下发时插入 running 记录，
// server 后台轮询任务终态后回填。
type StackDeployment struct {
	ID         uint   `gorm:"primarykey" json:"id"`
	AgentID    string `gorm:"index:idx_stack_deploy_agent_stack;size:64" json:"agentId"`
	StackName  string `gorm:"index:idx_stack_deploy_agent_stack;size:64" json:"stackName"`
	Action     string `gorm:"size:16" json:"action"`
	Status     string `gorm:"size:16" json:"status"` // running / success / failed
	TaskID     string `gorm:"index;size:64" json:"taskId"`
	StartedAt  int64  `json:"startedAt"`
	FinishedAt int64  `json:"finishedAt"` // 0 = 尚未结束
}

// BackupConfig 备份任务配置（server 调度数据源；备份文件在 Agent 侧本地生成，
// 见 docs/guide/backup-design.md）
type BackupConfig struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	AgentID    string    `gorm:"index;size:64" json:"agentId"`
	Name       string    `gorm:"size:64" json:"name"`      // 备份文件名前缀
	Sources    string    `gorm:"type:text" json:"sources"` // JSON 数组字符串，源路径列表
	DestDir    string    `gorm:"size:512" json:"destDir"`
	Schedule   string    `gorm:"size:32" json:"schedule"`            // manual / daily@HH:mm / every:Nh
	Retention  int       `json:"retention"`                          // 保留份数，0=不清理
	RemoteDest string    `gorm:"size:512" json:"remoteDest"`         // rclone 远端目标 remote:path，空=不启用异地（M2 D20）
	PreHook    string    `gorm:"size:1024" json:"preHook,omitempty"` // 打包前执行的数据库热备命令，空=不执行（M3 D26）
	Enabled    bool      `json:"enabled"`
	LastRunAt  int64     `json:"lastRunAt"`
	NextRunAt  int64     `json:"nextRunAt"`                 // manual 恒为 0
	LastStatus string    `gorm:"size:16" json:"lastStatus"` // "" / running / success / failed
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// BackupRun 备份单次运行记录。下发 backup.run 时插入 running，
// server 轮询任务终态后回填（复用 StackDeployment 模式）。
type BackupRun struct {
	ID       uint   `gorm:"primarykey" json:"id"`
	ConfigID uint   `gorm:"index" json:"configId"`
	TaskID   string `gorm:"size:64" json:"taskId"`
	Status   string `gorm:"size:16" json:"status"` // running / success / failed / timeout
	File     string `gorm:"size:256" json:"file"`  // 备份文件名（不含目录）
	Size     int64  `json:"size"`
	Error    string `gorm:"size:512" json:"error,omitempty"`
	// M2 D20：本地打包成功即 success，rclone 推送失败只记在此（D21）
	RemoteStatus string `gorm:"size:16" json:"remoteStatus,omitempty"` // ""/ok/failed
	RemoteError  string `gorm:"size:512" json:"remoteError,omitempty"`
	StartedAt    int64  `json:"startedAt"`
	FinishedAt   int64  `json:"finishedAt"` // 0 = 尚未结束
}
