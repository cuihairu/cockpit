package server

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// newCapabilityTestAgent 构造带契约字段的能力集合，模拟三类 agent：
//   - 新 agent（docker/nas/virt-libvirt 带 Access）
//   - 旧 agent（oldcap 无 Access、nold 无 Access 无 Version）
func newCapabilityTestAgent() *Agent {
	return &Agent{
		ID: "a1",
		Capabilities: []protocol.Capability{
			{Type: "docker", Version: "1", Access: protocol.CapabilityWritable},
			{Type: "nas", Version: "1", Access: protocol.CapabilityReadonly},
			{Type: "virt-libvirt", Version: "1", Access: protocol.CapabilityReadonly},
			{Type: "oldcap", Version: "1"}, // 旧 agent：无 Access
			{Type: "nold"},                 // 旧 agent：无 Access 无 Version
		},
	}
}

// TestCanWriteFailClosed 覆盖写权限门控：writable 放行，readonly/缺失
// Access（旧 agent）/无能力一律拒绝（fail-closed）。
func TestCanWriteFailClosed(t *testing.T) {
	a := newCapabilityTestAgent()

	cases := []struct {
		cap  string
		want bool
	}{
		{"docker", true},       // writable → 放行
		{"nas", false},         // readonly → 拒绝
		{"virt-libvirt", false},
		{"oldcap", false},      // 旧 agent 缺 Access → 视为 readonly
		{"nold", false},
		{"missing", false},     // 无能力
	}
	for _, c := range cases {
		if got := a.CanWrite(c.cap); got != c.want {
			t.Errorf("CanWrite(%q) = %v, want %v", c.cap, got, c.want)
		}
	}
}

// TestCapabilityVersionGet 覆盖版本读取：有版本返回版本，无能力返回空串。
func TestCapabilityVersionGet(t *testing.T) {
	a := newCapabilityTestAgent()
	if got := a.CapabilityVersion("docker"); got != "1" {
		t.Errorf("CapabilityVersion(docker) = %q, want \"1\"", got)
	}
	if got := a.CapabilityVersion("missing"); got != "" {
		t.Errorf("CapabilityVersion(missing) = %q, want \"\"", got)
	}
}

// TestCapabilitySatisfies 覆盖全量门控（版本+权限+存在性）与 fail-closed：
// 版本缺失/过低、写权限缺失一律拒绝；无版本/无写要求时等价 HasCapability。
func TestCapabilitySatisfies(t *testing.T) {
	a := newCapabilityTestAgent()

	cases := []struct {
		cap, min     string
		requireWrite bool
		want         bool
	}{
		// 写动作：要求 writable + 版本
		{"docker", "1", true, true},
		{"docker", "2", true, false},  // 版本过低
		{"docker", "", true, true},    // 无版本要求仍要求 writable
		{"nas", "1", true, false},     // readonly 拒绝写
		{"oldcap", "1", true, false},  // 旧 agent 缺 Access 拒绝写
		{"nold", "1", true, false},
		{"missing", "1", true, false},
		// 只读动作：要求存在 + 版本
		{"docker", "1", false, true},
		{"docker", "2", false, false},
		{"nas", "1", false, true},
		{"virt-libvirt", "1", false, true},
		{"oldcap", "1", false, true},  // 旧 agent 只读动作放行（有版本）
		{"nold", "1", false, false},   // 版本缺失 → 拒绝（fail-closed）
		{"nold", "", false, true},     // 无版本要求 → 等价 HasCapability
		{"missing", "1", false, false},
	}
	for _, c := range cases {
		if got := a.CapabilitySatisfies(c.cap, c.min, c.requireWrite); got != c.want {
			t.Errorf("CapabilitySatisfies(%q, %q, %v) = %v, want %v",
				c.cap, c.min, c.requireWrite, got, c.want)
		}
	}
}