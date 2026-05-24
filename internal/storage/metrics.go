package storage

import (
	"time"
)

// SystemMetric 系统指标历史记录
type SystemMetric struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AgentID   string    `gorm:"index;not null" json:"agentId"`
	Timestamp time.Time `gorm:"index" json:"timestamp"`

	// CPU 信息
	CPUUsage      float64 `json:"cpuUsage"`       // CPU 使用率 (0-100)
	CPUCores      int     `json:"cpuCores"`       // CPU 核心数
	CPUFreqMHz    float64 `json:"cpuFreqMhz"`     // CPU 频率

	// 内存信息
	MemTotal      uint64  `json:"memTotal"`       // 总内存 (bytes)
	MemUsed       uint64  `json:"memUsed"`        // 已用内存 (bytes)
	MemAvailable  uint64  `json:"memAvailable"`   // 可用内存 (bytes)
	MemUsagePercent float64 `json:"memUsagePercent"` // 内存使用率

	// 磁盘信息
	DiskTotal     uint64  `json:"diskTotal"`      // 总磁盘空间 (bytes)
	DiskUsed      uint64  `json:"diskUsed"`       // 已用磁盘空间 (bytes)
	DiskFree      uint64  `json:"diskFree"`       // 可用磁盘空间 (bytes)
	DiskUsagePercent float64 `json:"diskUsagePercent"` // 磁盘使用率

	// 网络信息
	NetBytesSent  uint64  `json:"netBytesSent"`   // 发送字节数
	NetBytesRecv  uint64  `json:"netBytesRecv"`   // 接收字节数

	// 系统信息
	OSName        string  `json:"osName"`         // 操作系统名称
	OSVersion     string  `json:"osVersion"`      // 操作系统版本
	Arch          string  `json:"arch"`           // 架构 (amd64, arm64等)
	Uptime        uint64  `json:"uptime"`         // 系统运行时间 (seconds)

	// 负载信息 (Unix-like)
	Load1         float64 `json:"load1"`          // 1分钟负载
	Load5         float64 `json:"load5"`          // 5分钟负载
	Load15        float64 `json:"load15"`         // 15分钟负载

	CreatedAt     time.Time `json:"createdAt"`
}

// SystemInfoSnapshot 系统信息快照（最新状态）
type SystemInfoSnapshot struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	AgentID       string    `gorm:"uniqueIndex;not null" json:"agentId"`

	// CPU 信息
	CPUUsage      float64   `json:"cpuUsage"`
	CPUCores      int       `json:"cpuCores"`
	CPUFreqMHz    float64   `json:"cpuFreqMhz"`

	// 内存信息
	MemTotal      uint64    `json:"memTotal"`
	MemUsed       uint64    `json:"memUsed"`
	MemAvailable  uint64    `json:"memAvailable"`
	MemUsagePercent float64 `json:"memUsagePercent"`

	// 磁盘信息
	DiskTotal     uint64    `json:"diskTotal"`
	DiskUsed      uint64    `json:"diskUsed"`
	DiskFree      uint64    `json:"diskFree"`
	DiskUsagePercent float64 `json:"diskUsagePercent"`

	// 网络信息
	NetBytesSent  uint64    `json:"netBytesSent"`
	NetBytesRecv  uint64    `json:"netBytesRecv"`

	// 系统信息
	OSName        string    `json:"osName"`
	OSVersion     string    `json:"osVersion"`
	Arch          string    `json:"arch"`
	Uptime        uint64    `json:"uptime"`
	Hostname      string    `json:"hostname"`

	// 负载信息
	Load1         float64   `json:"load1"`
	Load5         float64   `json:"load5"`
	Load15        float64   `json:"load15"`

	UpdatedAt     time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (SystemMetric) TableName() string {
	return "system_metrics"
}

// TableName 指定表名
func (SystemInfoSnapshot) TableName() string {
	return "system_info_snapshots"
}
