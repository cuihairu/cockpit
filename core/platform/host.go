// Package platform 平台适配层（agent-core ②）：唯一允许 GOOS 分支的地带
// （架构文档 §3）。具体实现位于 linux/windows/darwin/other 子目录（全量
// //go:build 标签），由 select_<goos>.go 编译期 import 装配——GOOS 决定装
// 哪个实现，运行时零分支。业务包方向恒为 包 → platform。
package platform

import "sync"

// Host 平台事实契约：机器标识/路径/服务集成/信号。P4 分批收编——本批
// 只落 MachineID，Paths/Service/Signals 留位接口随批次补齐。
type Host interface {
	// MachineID 系统级稳定机器标识（systemd machine-id / Windows
	// MachineGuid / macOS IOPlatformUUID），重启不变；不可得返回空串，
	// 调用方回退 hostname+random。
	MachineID() string
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
