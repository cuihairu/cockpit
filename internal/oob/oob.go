// Package oob 定义带外管理（Out-of-Band）的统一能力模型与 Provider 抽象。
// 设计依据：docs/review/oob-management-2026-10-20.md（三路线统一 + 三态分离）。
// 边界：本包只做模型/注册/发现聚合，不含任何厂商协议实现（AMT/DASH/Redfish 片再接）。
package oob

import "context"

// Capability 是统一操作面的带外能力项。
type Capability string

const (
	// CapabilityPower 远程电源（开机/关机/重启/强制断电）。
	CapabilityPower Capability = "power"
	// CapabilityInventory 硬件清单查询（带外通道读取）。
	CapabilityInventory Capability = "inventory"
	// CapabilityBIOS BIOS 级远程管理。
	CapabilityBIOS Capability = "bios"
	// CapabilityConsole 带外图形 KVM/串口控制台。
	CapabilityConsole Capability = "console"
	// CapabilityBoot 远程启动介质/启动顺序控制。
	CapabilityBoot Capability = "boot"
	// CapabilityRecovery 崩溃后恢复（远程恢复/擦除）。
	CapabilityRecovery Capability = "recovery"
)

// State 是一项能力的三态分离结论：硬件支持 ≠ 已启用 ≠ 当前可达。
// 布尔仅为存储形态，语义必须由检测与验证产生，禁止厂商宣称直填。
type State struct {
	Supported bool `json:"supported"` // 固件/硬件支持
	Enabled   bool `json:"enabled"`   // 已启用（BIOS/Provisioning 后）
	Reachable bool `json:"reachable"` // 当前可达（管理引擎在线、网络通）
}

// Usable 三态齐备才算可用。
func (s State) Usable() bool {
	return s.Supported && s.Enabled && s.Reachable
}

// rank 给三态一个全序用于合并仲裁：usable > 只到 reachable > 只到 enabled > 只到 supported > 无。
func (s State) rank() int {
	switch {
	case s.Usable():
		return 4
	case s.Reachable:
		return 3
	case s.Enabled:
		return 2
	case s.Supported:
		return 1
	default:
		return 0
	}
}

// DeviceInfo 是一次探测产出的设备带外画像。
type DeviceInfo struct {
	// Architecture 与 CPU 架构解耦探测（amd64/arm64 等），不由架构推断能力。
	Architecture string               `json:"architecture,omitempty"`
	Provider     string               `json:"provider"`
	Capabilities map[Capability]State `json:"capabilities"`
}

// Target 是探测目标（地址 + 可选按 Provider 的定位信息）。
type Target struct {
	Address string            `json:"address"`
	Hints   map[string]string `json:"hints,omitempty"`
}

// Provider 是一条带外路线的最小公共面（AMT/DASH/Redfish/IP-KVM/OpenWrt 各为一个实现）。
// Probe 只做发现与状态核实；电源等操作面在各自实现片中扩展，不在本包预设。
type Provider interface {
	Name() string
	Probe(ctx context.Context, target Target) (DeviceInfo, error)
}
