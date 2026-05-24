package agent

import (
	"bufio"
	"os"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v3/host"
)

// VirtualizationType 虚拟化类型
type VirtualizationType string

const (
	VirtTypeNone      VirtualizationType = "none"       // 物理机
	VirtTypeKVM       VirtualizationType = "kvm"        // KVM
	VirtTypeVMware    VirtualizationType = "vmware"     // VMware
	VirtTypeVirtualBox VirtualizationType = "virtualbox" // VirtualBox
	VirtTypeQEMU      VirtualizationType = "qemu"       // QEMU
	VirtTypeXen       VirtualizationType = "xen"        // Xen
	VirtTypeHyperV    VirtualizationType = "hyperv"     // Hyper-V
	VirtTypeParallels VirtualizationType = "parallels"  // Parallels
	VirtTypeContainer VirtualizationType = "container"  // 容器 (Docker/LXC)
	VirtTypeOpenVZ    VirtualizationType = "openvz"     // OpenVZ
	VirtTypeLXC       VirtualizationType = "lxc"        // LXC
	VirtTypeDocker    VirtualizationType = "docker"     // Docker
)

// SystemRole 系统角色
type SystemRole string

const (
	RoleGuest SystemRole = "guest" // 虚拟机/容器
	RoleHost  SystemRole = "host"  // 物理机/宿主机
)

// VirtualizationInfo 虚拟化信息
type VirtualizationInfo struct {
	Type     VirtualizationType `json:"type"`
	Role     SystemRole         `json:"role"`
	Platform string             `json:"platform,omitempty"` // 具体平台信息
}

var (
	cachedVirtInfo *VirtualizationInfo
	virtOnce       sync.Once
)

// DetectVirtualization 检测虚拟化信息（带缓存）
func DetectVirtualization() *VirtualizationInfo {
	virtOnce.Do(func() {
		cachedVirtInfo = detectVirtualizationImpl()
	})
	return cachedVirtInfo
}

// detectVirtualizationImpl 实际检测逻辑
func detectVirtualizationImpl() *VirtualizationInfo {
	info := &VirtualizationInfo{
		Type: VirtTypeNone,
		Role: RoleHost,
	}

	// 首先尝试使用 gopsutil
	if virtType, role, err := host.Virtualization(); err == nil {
		if virtType != "" {
			info.Type = VirtualizationType(virtType)
			info.Role = SystemRole(role)
			return info
		}
	}

	// gopsutil 检测失败，尝试手动检测
	info = detectVirtualizationManual()
	return info
}

// detectVirtualizationManual 手动检测虚拟化（fallback）
func detectVirtualizationManual() *VirtualizationInfo {
	info := &VirtualizationInfo{
		Type: VirtTypeNone,
		Role: RoleHost,
	}

	// 检测容器环境
	if isContainer() {
		info.Role = RoleGuest
		info.Type = detectContainerType()
		return info
	}

	// 检测 /sys/class/dmi/id/product_name
	if productName, err := readSysFile("/sys/class/dmi/id/product_name"); err == nil {
		productName = strings.ToLower(productName)
		if strings.Contains(productName, "vmware") {
			info.Type = VirtTypeVMware
			info.Role = RoleGuest
			return info
		}
		if strings.Contains(productName, "virtualbox") {
			info.Type = VirtTypeVirtualBox
			info.Role = RoleGuest
			return info
		}
		if strings.Contains(productName, "qemu") || strings.Contains(productName, "standard pc") {
			info.Type = VirtTypeQEMU
			info.Role = RoleGuest
			return info
		}
		if strings.Contains(productName, "kvm") {
			info.Type = VirtTypeKVM
			info.Role = RoleGuest
			return info
		}
		if strings.Contains(productName, "parallels") {
			info.Type = VirtTypeParallels
			info.Role = RoleGuest
			return info
		}
	}

	// 检测 /proc/cpuinfo 中的 model name
	if cpuInfo, err := readProcCpuInfo(); err == nil {
		cpuInfo = strings.ToLower(cpuInfo)
		if strings.Contains(cpuInfo, "qemu") {
			info.Type = VirtTypeQEMU
			info.Role = RoleGuest
			return info
		}
		if strings.Contains(cpuInfo, "vmware") {
			info.Type = VirtTypeVMware
			info.Role = RoleGuest
			return info
		}
		if strings.Contains(cpuInfo, "xen") {
			info.Type = VirtTypeXen
			info.Role = RoleGuest
			return info
		}
		if strings.Contains(cpuInfo, "kvm") {
			info.Type = VirtTypeKVM
			info.Role = RoleGuest
			return info
		}
	}

	// 检测 /sys/hypervisor/type (Xen)
	if hypervisorType, err := readSysFile("/sys/hypervisor/type"); err == nil {
		info.Type = VirtTypeXen
		info.Role = RoleGuest
		info.Platform = hypervisorType
		return info
	}

	// 检测 systemd-detect-virt（如果可用）
	if systemdVirt, err := runCommand("systemd-detect-virt", "--vm"); err == nil && systemdVirt != "" {
		info.Type = VirtualizationType(systemdVirt)
		info.Role = RoleGuest
		return info
	}

	// 默认为物理机
	return info
}

// isContainer 检测是否在容器中运行
func isContainer() bool {
	// 检测 /.dockerenv
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// 检测 /proc/1/cgroup
	if cgroup, err := readSysFile("/proc/1/cgroup"); err == nil {
		cgroup = strings.ToLower(cgroup)
		if strings.Contains(cgroup, "docker") || strings.Contains(cgroup, "lxc") || strings.Contains(cgroup, "kubepods") {
			return true
		}
	}

	return false
}

// detectContainerType 检测容器类型
func detectContainerType() VirtualizationType {
	// 检测 /proc/1/cgroup
	if cgroup, err := readSysFile("/proc/1/cgroup"); err == nil {
		cgroup = strings.ToLower(cgroup)
		if strings.Contains(cgroup, "docker") || strings.Contains(cgroup, "containerd") {
			return VirtTypeDocker
		}
		if strings.Contains(cgroup, "lxc") {
			return VirtTypeLXC
		}
		if strings.Contains(cgroup, "kubepods") {
			return VirtTypeContainer // Kubernetes
		}
	}

	return VirtTypeContainer
}

// readSysFile 读取系统文件
func readSysFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// readProcCpuInfo 读取 /proc/cpuinfo
func readProcCpuInfo() (string, error) {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "", err
	}
	defer file.Close()

	var modelName string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				modelName = strings.TrimSpace(parts[1])
				break
			}
		}
	}
	return modelName, scanner.Err()
}

// runCommand 执行命令（简单实现，仅用于检测）
func runCommand(name string, args ...string) (string, error) {
	// 简化实现，实际可以调用 exec.Command
	// 这里返回空，让 gopsutil 和其他检测方法优先
	return "", os.ErrNotExist
}

// IsVirtualMachine 判断是否为虚拟机
func IsVirtualMachine() bool {
	info := DetectVirtualization()
	return info.Role == RoleGuest && info.Type != VirtTypeContainer
}

// IsContainer 判断是否为容器
func IsContainer() bool {
	info := DetectVirtualization()
	return info.Type == VirtTypeDocker || info.Type == VirtTypeLXC || info.Type == VirtTypeContainer
}

// IsPhysicalMachine 判断是否为物理机
func IsPhysicalMachine() bool {
	info := DetectVirtualization()
	return info.Role == RoleHost
}
