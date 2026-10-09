// Package platform 平台适配层（agent-core ②）：唯一允许 GOOS 分支的地带
// （架构文档 §3）。具体实现位于 linux/windows/darwin/other 子目录（全量
// //go:build 标签），由 select_<goos>.go 编译期 import 装配——GOOS 决定装
// 哪个实现，运行时零分支。业务包方向恒为 包 → platform。
package platform

import (
	"os"
	"sync"
)

// Host 平台事实契约：机器标识/路径/信号。P4 只落 MachineID，P7a 补齐
// Paths/Signals（Service 服务集成挂约见 P7b，留位）。
type Host interface {
	// MachineID 系统级稳定机器标识（systemd machine-id / Windows
	// MachineGuid / macOS IOPlatformUUID），重启不变；不可得返回空串，
	// 调用方回退 hostname+random。
	MachineID() string

	// Paths 平台标准目录语义（服务形态为基准）：配置/数据/日志三目录。
	// linux 沿 install.sh 既有家族（/etc|/var/lib|/var/log/cockpit-agent，
	// ReadWritePaths 已含数据面）；windows 挂 ProgramData；darwin 挂
	// ~/Library。目录不自动创建——消费方按需 MkdirAll。不可得（如取
	// home 失败）返回空串字段，调用方自行兜底。
	Paths() Paths

	// Signals 优雅退出信号集：任一信号触发刷缓冲/断连接/退出码 0 的
	// 主动停路径；崩溃与硬杀退出码非 0，交平台监管方（systemd Restart /
	// SCM failure actions / launchd KeepAlive）按各自配置拉起。
	// windows 仅 os.Interrupt——SCM Stop 不产生信号量（svc handler 走
	// RunService stop 通道），控制台运行仍需 Ctrl+C 优雅停。
	Signals() []os.Signal
}

// Paths 平台标准目录三元组。
type Paths struct {
	ConfigDir string // 配置（env 文件/配置文件所在目录）
	DataDir   string // 运行数据（状态/缓存/留痕）
	LogDir    string // 日志（非 systemd/launchd 托管时的落盘面）
}

var (
	mu      sync.RWMutex
	current Host
)

// Register 平台实现注册（仅 select_<goos>.go 的 init 装配触发，业务代码
// 勿调）。
func Register(h Host) {
	mu.Lock()
	defer mu.Unlock()
	current = h
}

// Current 当前平台实现。select_* 四互斥全覆盖 GOOS，生产恒非 nil；
// nil 兜底由调用方处理（同 machineID 空串语义）。
func Current() Host {
	mu.RLock()
	defer mu.RUnlock()
	return current
}
