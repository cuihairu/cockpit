package detector

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// overlay 虚拟网身份提取（见 docs/guide/overlay-design.md M2，D15/D16）。
// Register 时随 overlay capability metadata 上报，供 server 端 CMDB 对照
// （D17）与 web 身份 chip 使用。范围仅 ZeroTier / Tailscale——中心化虚拟网
// 才有云管理面对照语义（WireGuard 自建无控制面、frp 无成员概念）。
// 每命令 2s 超时；任一步失败该段身份静默缺席，绝不阻塞注册（D16）。

// overlayIdentityCmdTimeout 单条身份提取命令超时（D16）
const overlayIdentityCmdTimeout = 2 * time.Second

// identity 单命令执行，独立函数便于测试经由 PATH 桩注入（fakeBinDir）
func overlayIdentityRun(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), overlayIdentityCmdTimeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

// zerotierIdentity ZeroTier 侧身份：node id 即云管理面 member id（D17 匹配键）
func zerotierIdentity() (map[string]any, bool) {
	out, err := overlayIdentityRun(overlayZTCliBin, "-j", "info")
	if err != nil {
		return nil, false
	}
	var info struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(out, &info); err != nil || info.Address == "" {
		return nil, false
	}

	id := map[string]any{"nodeId": info.Address}
	// listnetworks 可选：daemon 在线但网络列表拉取失败只缺 networks 段
	if out, err := overlayIdentityRun(overlayZTCliBin, "-j", "listnetworks"); err == nil {
		var nets []struct {
			ID                string   `json:"id"`
			Name              string   `json:"name"`
			Status            string   `json:"status"`
			AssignedAddresses []string `json:"assignedAddresses"`
		}
		if json.Unmarshal(out, &nets) == nil && len(nets) > 0 {
			networks := make([]map[string]any, 0, len(nets))
			for _, n := range nets {
				networks = append(networks, map[string]any{
					"id":        n.ID,
					"name":      n.Name,
					"status":    n.Status,
					"addresses": stripCIDRSuffixes(n.AssignedAddresses),
				})
			}
			id["networks"] = networks
		}
	}
	return id, true
}

// tailscaleIdentity Tailscale 侧身份：Self.ID 即云管理面 device id（D17）。
// 未登录时 Self 为 null → 身份缺席。
func tailscaleIdentity() (map[string]any, bool) {
	out, err := overlayIdentityRun(overlayTailscaleBin, "status", "--json")
	if err != nil {
		return nil, false
	}
	var status struct {
		Self *struct {
			ID           string   `json:"ID"`
			HostName     string   `json:"HostName"`
			DNSName      string   `json:"DNSName"`
			TailscaleIPs []string `json:"TailscaleIPs"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(out, &status); err != nil || status.Self == nil || status.Self.ID == "" {
		return nil, false
	}
	id := map[string]any{"id": status.Self.ID}
	if status.Self.HostName != "" {
		id["hostName"] = status.Self.HostName
	}
	if status.Self.DNSName != "" {
		id["dnsName"] = status.Self.DNSName
	}
	if len(status.Self.TailscaleIPs) > 0 {
		id["addresses"] = status.Self.TailscaleIPs
	}
	return id, true
}

// stripCIDRSuffixes ZeroTier assignedAddresses 形如 "10.147.20.5/24"，
// 云端 ipAssignments 是裸地址——剥后缀对齐 D17 匹配口径
func stripCIDRSuffixes(addrs []string) []string {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, strings.SplitN(a, "/", 2)[0])
	}
	return out
}

// extractIdentity 按已探测到的工具提取身份；两工具都缺席时返回 nil
// （metadata 不带 identity 键）。
func extractIdentity(features map[string]any) map[string]any {
	identity := map[string]any{}
	if features["zerotier"] == true {
		if id, ok := zerotierIdentity(); ok {
			identity["zerotier"] = id
		}
	}
	if features["tailscale"] == true {
		if id, ok := tailscaleIdentity(); ok {
			identity["tailscale"] = id
		}
	}
	if len(identity) == 0 {
		return nil
	}
	return identity
}
