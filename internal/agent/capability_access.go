package agent

import "github.com/cuihairu/cockpit/internal/protocol"

// capabilityAccess 按能力类型返回契约权限状态（P0 Agent 契约首片，
// 2026-11-21，见 docs/design/agent-capability-contract.md §4）。
//
// 分类依据 internal/agent/rpc 各 Provider 的实际写面：
//   - 写面（创建/修改/删除/下发配置/执行命令/产生外部副作用）→ writable；
//   - 纯观察/只读查询 → readonly。
//
// fail-closed：未列入写面的类型（含未知能力、detector 新类型）一律默认
// readonly——server 侧据此拒绝写动作，能力升级须在实现时显式补入写面
// （如 libvirt 只读片 → 写片上线时把 virt-libvirt 移入写面）。
func capabilityAccess(capType string) protocol.CapabilityAccess {
	switch capType {
	case // backup：打包装档写本地/远端存储
		"backup",
		// file：远程文件读写
		"file",
		// 代理配置面：改配置并 reload
		"nginx-proxy",
		"traefik-proxy",
		// cron：写 crontab
		"cron",
		// drift：检测漂移并回写基线配置
		"drift",
		// 容器/虚拟机/网关管理：生命周期与配置写
		"docker-api",
		"docker",
		"pve-api",
		"openwrt",
		// oob：WOL 魔术包下发，外部副作用动作
		"oob",
		// job：统一执行通道（job.exec 执行命令）
		"job":
		return protocol.CapabilityWritable
	default:
		// 默认 readonly：nas/logs/virt-libvirt/probe 等观察面与未分类能力
		return protocol.CapabilityReadonly
	}
}

// withCapabilityAccess 为能力列表补默认权限状态，返回新切片（不改动入参
// backing array，避免与注册载荷共享底层数组）。已有 Access 的条目保持原值
// （detector 显式声明优先）。
func withCapabilityAccess(caps []protocol.Capability) []protocol.Capability {
	out := make([]protocol.Capability, len(caps))
	copy(out, caps)
	for i := range out {
		if out[i].Access == "" {
			out[i].Access = capabilityAccess(out[i].Type)
		}
	}
	return out
}