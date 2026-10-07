package detector

import (
	"os/exec"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func init() {
	Register(&FirewallDetector{})
}

// 探测函数做成包级变量仅为测试可注入（真 LookPath 过不了假路径）
var (
	firewallLookPath = exec.LookPath
	// firewallRunVersion 运行 `<bin> --version` 验证可执行
	firewallRunVersion = func(bin string) error {
		return exec.Command(bin, "--version").Run()
	}
)

// FirewallDetector 防火墙工具检测器（见 docs/guide/firewall-design.md D1）
type FirewallDetector struct{}

// Name 检测器名称
func (d *FirewallDetector) Name() string { return "firewall" }

// Priority 检测优先级
func (d *FirewallDetector) Priority() int { return 30 }

// Detect nft/iptables 任一可用即上报 firewall capability；两者皆缺返回
// nil（页面走「无防火墙工具」空态，不报错，smart D1 同则）
func (d *FirewallDetector) Detect() (*protocol.Capability, error) {
	features := make(map[string]any)
	if d.hasBin("nft") {
		features["nft"] = true
	}
	if d.hasBin("iptables") {
		features["iptables"] = true
	}
	if len(features) == 0 {
		return nil, nil
	}
	return &protocol.Capability{
		Type:     "firewall",
		Metadata: features,
	}, nil
}

// hasBin LookPath + 一次 --version 验证（nft/iptables 的 --version 均
// 无需特权，不会误报权限问题）
func (d *FirewallDetector) hasBin(bin string) bool {
	if _, err := firewallLookPath(bin); err != nil {
		return false
	}
	return firewallRunVersion(bin) == nil
}
