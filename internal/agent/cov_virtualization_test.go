package agent

// cov_* 测试：覆盖率攻坚新增（不修改既有测试文件）。
// 通过包级路径/依赖变量的注入（默认值即生产路径）覆盖原本依赖真实
// /proc、/sys 环境的分支。只直接调用 detectVirtualizationImpl /
// detectVirtualizationManual 等内部函数，不触碰 DetectVirtualization 的
// sync.Once 缓存，不影响其他测试。

import (
	"os"
	"path/filepath"
	"testing"
)

// covSetStr 测试期内替换字符串型包级变量，结束时恢复。
func covSetStr(t *testing.T, dst *string, val string) {
	t.Helper()
	old := *dst
	*dst = val
	t.Cleanup(func() { *dst = old })
}

// covWriteFile 在临时目录写文件并返回路径。
func covWriteFile(t *testing.T, name, content string) string {
	t.Helper()
	fp := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(fp, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", fp, err)
	}
	return fp
}

// covMissingPath 返回一个确定不存在的路径。
func covMissingPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "definitely-missing")
}

// covVirtNoContainer 把容器探测路径全部指向不存在的位置。
func covVirtNoContainer(t *testing.T) {
	t.Helper()
	covSetStr(t, &dockerEnvPath, covMissingPath(t))
	covSetStr(t, &proc1CgroupPath, covMissingPath(t))
}

// covVirtResetPaths 把手动探测的所有数据源指向不存在的位置
//（runCommand 默认实现恒返回错误，等价于 systemd-detect-virt 缺失）。
func covVirtResetPaths(t *testing.T) {
	t.Helper()
	covVirtNoContainer(t)
	covSetStr(t, &sysProductNamePath, covMissingPath(t))
	covSetStr(t, &sysHypervisorTypePath, covMissingPath(t))
	covSetStr(t, &procCpuInfoPath, covMissingPath(t))
}

func TestCovDetectVirtualizationImplGopsutilEmpty(t *testing.T) {
	// gopsutil 返回空虚拟化类型 → 回退手动检测
	covVirtResetPaths(t) // 手动探测数据源全部置空 → 结果确定
	old := hostVirtualization
	hostVirtualization = func() (string, string, error) { return "", "", nil }
	t.Cleanup(func() { hostVirtualization = old })

	info := detectVirtualizationImpl()
	if info == nil {
		t.Fatal("detectVirtualizationImpl() should never return nil")
	}
	if info.Type != VirtTypeNone || info.Role != RoleHost {
		t.Errorf("info = %+v, want none/host (no source matches)", info)
	}
}

func TestCovDetectVirtualizationImplGopsutilError(t *testing.T) {
	old := hostVirtualization
	hostVirtualization = func() (string, string, error) { return "", "", os.ErrNotExist }
	t.Cleanup(func() { hostVirtualization = old })

	if info := detectVirtualizationImpl(); info == nil {
		t.Fatal("detectVirtualizationImpl() should never return nil")
	}
}

func TestCovDetectVirtualizationManualContainer(t *testing.T) {
	// /.dockerenv 存在 → 容器环境
	covSetStr(t, &dockerEnvPath, covWriteFile(t, "dockerenv", ""))
	covSetStr(t, &proc1CgroupPath, covWriteFile(t, "cgroup", "0::/docker/abc123\n"))

	info := detectVirtualizationManual()
	if info.Role != RoleGuest || info.Type != VirtTypeDocker {
		t.Errorf("info = %+v, want guest/docker", info)
	}
}

func TestCovIsContainerAndTypeVariants(t *testing.T) {
	tests := []struct {
		cgroup    string
		container bool // isContainer 是否判定为容器
		want      VirtualizationType
	}{
		{"0::/docker/abc", true, VirtTypeDocker},
		// containerd 是 detectContainerType 的关键字，但不含 isContainer 的关键字
		{"0::/system.slice/containerd-xyz", false, VirtTypeDocker},
		{"0::/lxc/container1", true, VirtTypeLXC},
		{"0::/kubepods/pod88", true, VirtTypeContainer},
		{"0::/system.slice/sshd", false, VirtTypeContainer}, // 无匹配 → 默认 container
	}
	covSetStr(t, &dockerEnvPath, covMissingPath(t))
	for _, tt := range tests {
		covSetStr(t, &proc1CgroupPath, covWriteFile(t, "cgroup", tt.cgroup))
		if got := isContainer(); got != tt.container {
			t.Errorf("isContainer(%q) = %v, want %v", tt.cgroup, got, tt.container)
		}
		if got := detectContainerType(); got != tt.want {
			t.Errorf("detectContainerType(%q) = %s, want %s", tt.cgroup, got, tt.want)
		}
	}

	// 两个来源都不存在 → 非容器，类型为默认 container
	covSetStr(t, &proc1CgroupPath, covMissingPath(t))
	if isContainer() {
		t.Error("isContainer() should be false when no marker exists")
	}
	if got := detectContainerType(); got != VirtTypeContainer {
		t.Errorf("detectContainerType() = %s, want container default", got)
	}
}

func TestCovDetectVirtualizationManualProductNames(t *testing.T) {
	tests := []struct {
		productName string
		want        VirtualizationType
	}{
		{"VMware Virtual Platform", VirtTypeVMware},
		{"VirtualBox Box", VirtTypeVirtualBox},
		{"QEMU Standard PC", VirtTypeQEMU},
		{"Standard PC (QEMU)", VirtTypeQEMU}, // "standard pc" 关键字
		{"KVM Virtual Machine", VirtTypeKVM},
		{"Parallels Virtual Machine", VirtTypeParallels},
	}
	for _, tt := range tests {
		covVirtNoContainer(t)
		covSetStr(t, &sysProductNamePath, covWriteFile(t, "product_name", tt.productName+"\n"))
		covSetStr(t, &procCpuInfoPath, covMissingPath(t))
		covSetStr(t, &sysHypervisorTypePath, covMissingPath(t))

		info := detectVirtualizationManual()
		if info.Type != tt.want || info.Role != RoleGuest {
			t.Errorf("productName %q: info = %+v, want type %s guest", tt.productName, info, tt.want)
		}
	}
}

func TestCovDetectVirtualizationManualCpuInfo(t *testing.T) {
	tests := []struct {
		modelName string
		want      VirtualizationType
	}{
		{"QEMU Virtual CPU v2", VirtTypeQEMU},
		{"Intel(R) Xeon(R) VMware CPU", VirtTypeVMware},
		{"AMD Xen Processor", VirtTypeXen},
		{"AMD KVM Host CPU", VirtTypeKVM},
	}
	for _, tt := range tests {
		covVirtResetPaths(t)
		covSetStr(t, &procCpuInfoPath, covWriteFile(t, "cpuinfo",
			"processor\t: 0\nmodel name\t: "+tt.modelName+"\n"))

		info := detectVirtualizationManual()
		if info.Type != tt.want || info.Role != RoleGuest {
			t.Errorf("modelName %q: info = %+v, want type %s guest", tt.modelName, info, tt.want)
		}
	}
}

func TestCovDetectVirtualizationManualXenHypervisor(t *testing.T) {
	covVirtResetPaths(t)
	covSetStr(t, &sysHypervisorTypePath, covWriteFile(t, "type", "xen\n"))

	info := detectVirtualizationManual()
	if info.Type != VirtTypeXen || info.Role != RoleGuest || info.Platform != "xen" {
		t.Errorf("info = %+v, want xen guest platform=xen", info)
	}
}

func TestCovDetectVirtualizationManualSystemdVirt(t *testing.T) {
	covVirtResetPaths(t)
	old := runCommand
	runCommand = func(name string, args ...string) (string, error) {
		return "kvm", nil
	}
	t.Cleanup(func() { runCommand = old })

	info := detectVirtualizationManual()
	if info.Type != VirtTypeKVM || info.Role != RoleGuest {
		t.Errorf("info = %+v, want kvm guest", info)
	}
}

func TestCovDetectVirtualizationManualPhysicalDefault(t *testing.T) {
	// 所有数据源缺失 + runCommand 失败 → 物理机默认值
	covVirtResetPaths(t)

	info := detectVirtualizationManual()
	if info.Type != VirtTypeNone || info.Role != RoleHost {
		t.Errorf("info = %+v, want none/host", info)
	}
}

func TestCovReadProcCpuInfoOpenError(t *testing.T) {
	covSetStr(t, &procCpuInfoPath, covMissingPath(t))
	if _, err := readProcCpuInfo(); err == nil {
		t.Error("readProcCpuInfo() should fail for missing path")
	}
}
