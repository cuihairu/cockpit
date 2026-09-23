package agent

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// 覆盖 detectCapabilities 的 rdp-client append 分支（agent.go:407-412）。
// rdpClientAvailable 是 build-tag 分流的常量（stub 恒 false / rdp 恒 true），
// 默认构建下 if 体不执行；此处注入 true 覆盖分支。rdpClientAvailable 由
// func 改 var（行为不变——生产仍按 build tag 恒定返回），仅开放测试注入点。
func TestDetectCapabilitiesRDPClientBranch(t *testing.T) {
	old := rdpClientAvailable
	rdpClientAvailable = func() bool { return true }
	defer func() { rdpClientAvailable = old }()

	agent := NewAgent(Config{ServerURL: "ws://localhost:8080"})
	caps := agent.detectCapabilities()

	found := false
	for _, c := range caps {
		if c.Type == "rdp-client" {
			found = true
			if c.Version != "1" {
				t.Errorf("rdp-client version = %q, want 1", c.Version)
			}
		}
	}
	if !found {
		t.Error("detectCapabilities should include rdp-client when rdpClientAvailable() == true")
	}

	// 反向：false 时不上报
	rdpClientAvailable = func() bool { return false }
	caps = agent.detectCapabilities()
	for _, c := range caps {
		if c.Type == "rdp-client" {
			t.Error("detectCapabilities should not include rdp-client when rdpClientAvailable() == false")
		}
	}
	_ = protocol.Capability{} // 保持 protocol import 语义清晰
}
