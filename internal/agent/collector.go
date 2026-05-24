package agent

import (
	"runtime"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/load"
)

// Collector 系统信息采集器
type Collector struct {
	lastNetBytesSent uint64
	lastNetBytesRecv uint64
	lastNetTime      time.Time
}

// NewCollector 创建采集器
func NewCollector() *Collector {
	return &Collector{
		lastNetTime: time.Now(),
	}
}

// Collect 采集系统信息
func (c *Collector) Collect() *protocol.SystemInfoPayload {
	info := &protocol.SystemInfoPayload{}

	// 采集 CPU 信息
	if cpuPercent, err := cpu.Percent(time.Second, false); err == nil && len(cpuPercent) > 0 {
		info.CPUUsage = cpuPercent[0]
	}
	if cpuInfo, err := cpu.Counts(true); err == nil {
		info.CPUCores = cpuInfo
	}
	// CPU 频率需要从 /proc/cpuinfo 读取，gopsutil 暂不支持跨平台
	info.CPUFreqMHz = 0

	// 采集内存信息
	if memStat, err := mem.VirtualMemory(); err == nil {
		info.MemTotal = memStat.Total
		info.MemUsed = memStat.Used
		info.MemAvailable = memStat.Available
		info.MemUsagePercent = memStat.UsedPercent
	}

	// 采集磁盘信息（根分区）
	if diskStat, err := disk.Usage("/"); err == nil {
		info.DiskTotal = diskStat.Total
		info.DiskUsed = diskStat.Used
		info.DiskFree = diskStat.Free
		info.DiskUsagePercent = diskStat.UsedPercent
	}

	// 采集网络信息
	if netStats, err := net.IOCounters(false); err == nil && len(netStats) > 0 {
		totalSent := uint64(0)
		totalRecv := uint64(0)
		for _, stat := range netStats {
			totalSent += stat.BytesSent
			totalRecv += stat.BytesRecv
		}
		info.NetBytesSent = totalSent
		info.NetBytesRecv = totalRecv
		c.lastNetBytesSent = totalSent
		c.lastNetBytesRecv = totalRecv
		c.lastNetTime = time.Now()
	}

	// 采集主机信息
	if hostInfo, err := host.Info(); err == nil {
		info.OSName = hostInfo.OS
		info.OSVersion = hostInfo.PlatformVersion
		info.Arch = hostInfo.KernelArch
		info.Uptime = uint64(hostInfo.Uptime)
		info.Hostname = hostInfo.Hostname
	}

	// 采集负载信息（仅 Unix-like 系统）
	if loadStat, err := load.Avg(); err == nil {
		info.Load1 = loadStat.Load1
		info.Load5 = loadStat.Load5
		info.Load15 = loadStat.Load15
	}

	return info
}

// CollectBasic 采集基本信息（不阻塞）
func (c *Collector) CollectBasic() *protocol.SystemInfoPayload {
	info := &protocol.SystemInfoPayload{}

	// 采集 CPU 信息（快速采样）
	if cpuPercent, err := cpu.Percent(0, false); err == nil && len(cpuPercent) > 0 {
		info.CPUUsage = cpuPercent[0]
	}

	// 采集内存信息
	if memStat, err := mem.VirtualMemory(); err == nil {
		info.MemTotal = memStat.Total
		info.MemUsed = memStat.Used
		info.MemAvailable = memStat.Available
		info.MemUsagePercent = memStat.UsedPercent
	}

	// 采集磁盘信息
	if diskStat, err := disk.Usage("/"); err == nil {
		info.DiskTotal = diskStat.Total
		info.DiskUsed = diskStat.Used
		info.DiskFree = diskStat.Free
		info.DiskUsagePercent = diskStat.UsedPercent
	}

	// 主机信息通常不变，可以在初始化时缓存
	if hostInfo, err := host.Info(); err == nil {
		info.OSName = hostInfo.OS
		info.OSVersion = hostInfo.PlatformVersion
		info.Arch = hostInfo.KernelArch
		info.Uptime = uint64(hostInfo.Uptime)
		info.Hostname = hostInfo.Hostname
	}

	return info
}

// GetRuntimeInfo 获取运行时信息
func GetRuntimeInfo() map[string]interface{} {
	return map[string]interface{}{
		"goVersion": runtime.Version(),
		"goroutines": runtime.NumGoroutine(),
		"compiler": runtime.Compiler,
		"arch": runtime.GOARCH,
		"os": runtime.GOOS,
	}
}

// GetCPUInfo 获取详细 CPU 信息
func GetCPUInfo() ([]cpu.InfoStat, error) {
	return cpu.Info()
}

// GetDiskPartitions 获取磁盘分区信息
func GetDiskPartitions() ([]disk.PartitionStat, error) {
	return disk.Partitions(true)
}

// GetNetInterfaces 获取网络接口信息
func GetNetInterfaces() ([]net.InterfaceStat, error) {
	return net.Interfaces()
}
