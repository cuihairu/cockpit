package detector

import (
	"errors"
	"os/exec"
	"testing"
)

// withFirewallDetectorStubs 注入 LookPath/--version 探测（只有 present 中的
// 命令放行），返回注入的 runVersion 供用例自定义行为
func withFirewallDetectorStubs(t *testing.T, present ...string) {
	t.Helper()
	oldLook, oldRun := firewallLookPath, firewallRunVersion
	firewallLookPath = func(bin string) (string, error) {
		for _, p := range present {
			if p == bin {
				return "/usr/sbin/" + bin, nil
			}
		}
		return "", exec.ErrNotFound
	}
	firewallRunVersion = func(bin string) error { return nil }
	t.Cleanup(func() { firewallLookPath, firewallRunVersion = oldLook, oldRun })
}

func TestFirewallDetectorBothTools(t *testing.T) {
	withFirewallDetectorStubs(t, "nft", "iptables")
	cap, err := (&FirewallDetector{}).Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil || cap.Type != "firewall" {
		t.Fatalf("cap = %+v, want firewall capability", cap)
	}
	if cap.Metadata["nft"] != true || cap.Metadata["iptables"] != true {
		t.Errorf("metadata = %v, want nft+iptables", cap.Metadata)
	}
}

func TestFirewallDetectorOnlyNft(t *testing.T) {
	withFirewallDetectorStubs(t, "nft")
	cap, err := (&FirewallDetector{}).Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil || cap.Metadata["nft"] != true || cap.Metadata["iptables"] == true {
		t.Errorf("cap = %+v, want nft-only", cap)
	}
}

func TestFirewallDetectorNone(t *testing.T) {
	// 双缺（Windows/裸容器）→ nil capability，不报错（D1）
	withFirewallDetectorStubs(t)
	cap, err := (&FirewallDetector{}).Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap != nil {
		t.Errorf("cap = %+v, want nil", cap)
	}
}

func TestFirewallDetectorPriorityAndName(t *testing.T) {
	// 名称/优先级元数据（Priority 参与多检测器排序）
	d := &FirewallDetector{}
	if d.Name() != "firewall" {
		t.Errorf("Name() = %q, want firewall", d.Name())
	}
	if d.Priority() != 30 {
		t.Errorf("Priority() = %d, want 30", d.Priority())
	}
}

func TestFirewallDetectorVersionRunFails(t *testing.T) {
	// LookPath 命中但 --version 跑不动（坏安装）→ 视为不可用
	withFirewallDetectorStubs(t, "nft")
	firewallRunVersion = func(bin string) error { return errors.New("exec format error") }
	cap, err := (&FirewallDetector{}).Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap != nil {
		t.Errorf("cap = %+v, want nil (version check failed)", cap)
	}
}
