package detector

import (
	"os/exec"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func init() {
	Register(&OverlayDetector{})
}

// overlay 探测的命令名做成包级变量仅为测试可注入，默认值即生产取值。
var (
	overlayZTCliBin     = "zerotier-cli"
	overlayTailscaleBin = "tailscale"
	overlayWGBin        = "wg"
	overlayFrpcBin      = "frpc"
	overlayFrpsBin      = "frps"
)

// OverlayDetector Overlay 组网工具检测（见 docs/guide/overlay-design.md）。
// ZeroTier / Tailscale / WireGuard / frp 任一可用 → overlay capability；
// 各工具粒度的可用性由 provider 内部 reader 自检（D4）。
type OverlayDetector struct{}

// Name 检测器名称
func (d *OverlayDetector) Name() string {
	return "overlay"
}

// Priority 检测优先级
func (d *OverlayDetector) Priority() int {
	return 16
}

// Detect 检测本机可用的组网工具
func (d *OverlayDetector) Detect() (*protocol.Capability, error) {
	features := make(map[string]any)

	if lookPath(overlayZTCliBin) {
		features["zerotier"] = true
	}
	if lookPath(overlayTailscaleBin) {
		features["tailscale"] = true
	}
	if lookPath(overlayWGBin) {
		features["wireguard"] = true
	}
	// frp 客户端或服务端任一存在即视为具备 frp 观测资格，
	// reader 端再按 frpc/frps 细分
	if lookPath(overlayFrpcBin) || lookPath(overlayFrpsBin) {
		features["frp"] = true
	}

	if len(features) == 0 {
		return nil, nil
	}

	// M2-B：虚拟网身份提取（D15/D16）——失败静默缺席，不阻塞注册
	if id := extractIdentity(features); id != nil {
		features["identity"] = id
	}

	return &protocol.Capability{
		Type:     "overlay",
		Metadata: features,
	}, nil
}

// lookPath 命令存在性检查
func lookPath(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}
