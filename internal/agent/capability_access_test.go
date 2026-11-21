package agent

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// TestCapabilityAccessClassification 覆盖写面/只读面的类型分类，
// 并验证未知类型 fail-closed → readonly。
func TestCapabilityAccessClassification(t *testing.T) {
	writable := []string{
		"backup", "file", "nginx-proxy", "traefik-proxy", "cron",
		"drift", "docker-api", "docker", "pve-api", "openwrt", "oob", "job",
	}
	readonly := []string{
		"nas", "logs", "virt-libvirt", "probe", "unknown-capability", "",
	}

	for _, typ := range writable {
		if got := capabilityAccess(typ); got != protocol.CapabilityWritable {
			t.Errorf("capabilityAccess(%q) = %q, want %q", typ, got, protocol.CapabilityWritable)
		}
	}
	for _, typ := range readonly {
		if got := capabilityAccess(typ); got != protocol.CapabilityReadonly {
			t.Errorf("capabilityAccess(%q) = %q, want %q", typ, got, protocol.CapabilityReadonly)
		}
	}
}

// TestWithCapabilityAccessDefault 覆盖默认补值：未声明 Access 的条目按
// 类型分类补默认；已声明的条目保持原值（detector 显式声明优先）。
func TestWithCapabilityAccessDefault(t *testing.T) {
	in := []protocol.Capability{
		{Type: "docker"},          // 写面 → 默认 writable
		{Type: "nas"},             // 观察面 → 默认 readonly
		{Type: "unknown"},         // 未知 → fail-closed readonly
		{Type: "virt-libvirt"},    // 只读片 → 默认 readonly
		{Type: "oob", Access: protocol.CapabilityReadonly}, // 显式声明覆盖默认
	}

	out := withCapabilityAccess(in)

	wantAccess := map[string]protocol.CapabilityAccess{
		"docker":       protocol.CapabilityWritable,
		"nas":          protocol.CapabilityReadonly,
		"unknown":      protocol.CapabilityReadonly,
		"virt-libvirt": protocol.CapabilityReadonly,
		"oob":          protocol.CapabilityReadonly,
	}
	for _, cap := range out {
		if got, want := cap.Access, wantAccess[cap.Type]; got != want {
			t.Errorf("cap %q Access = %q, want %q", cap.Type, got, want)
		}
	}

	// 入参不被改写（返回新切片，共享 backing array 安全）
	if in[0].Access != "" {
		t.Errorf("input mutated: in[0].Access = %q, want empty", in[0].Access)
	}
}