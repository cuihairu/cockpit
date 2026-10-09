//go:build !linux && !darwin && !windows

// Package other 其余平台兜底适配（agent-core ②）：无系统级机器标识的
// 平台恒返回空串，调用方回退 hostname+random（openwrt 走 linux 包）。
package other

// Host 兜底平台事实实现。
type Host struct{}

// MachineID 不支持的平台返回空字符串。
func (Host) MachineID() string { return "" }
