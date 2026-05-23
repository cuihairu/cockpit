package detector

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func init() {
	Register(&NetworkDetector{})
}

// NetworkDetector 网络监控检测器
type NetworkDetector struct{}

// Name 检测器名称
func (d *NetworkDetector) Name() string {
	return "network-monitor"
}

// Priority 检测优先级
func (d *NetworkDetector) Priority() int {
	return 15
}

// Detect 检测网络监控能力
func (d *NetworkDetector) Detect() (*protocol.Capability, error) {
	features := make(map[string]any)

	// 1. 检测 WireGuard
	if d.hasWireGuard() {
		features["wireguard"] = true
		interfaces := d.getWireGuardInterfaces()
		if len(interfaces) > 0 {
			features["wireguard_interfaces"] = interfaces
		}
	}

	// 2. 检测隧道配置
	tunnels := d.detectTunnels()
	if len(tunnels) > 0 {
		features["tunnels"] = tunnels
	}

	// 3. 检测路由信息
	if d.hasRouteInfo() {
		features["routes"] = true
	}

	if len(features) == 0 {
		return nil, nil
	}

	return &protocol.Capability{
		Type:     "network-monitor",
		Metadata: features,
	}, nil
}

// hasWireGuard 检测 WireGuard 支持
func (d *NetworkDetector) hasWireGuard() bool {
	// 检查 wg 工具
	_, err := exec.LookPath("wg")
	if err == nil {
		return true
	}

	// 检查 WireGuard 设备
	wgPath := "/proc/net/wireguard"
	if _, err := os.Stat(wgPath); err == nil {
		return true
	}

	// 检查 /sys/class/net 下的 wg* 接口
	netPath := "/sys/class/net"
	entries, _ := os.ReadDir(netPath)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "wg") {
			return true
		}
	}

	return false
}

// getWireGuardInterfaces 获取 WireGuard 接口列表
func (d *NetworkDetector) getWireGuardInterfaces() []string {
	var interfaces []string

	// 从 /proc/net/wireguard 读取
	if data, err := os.ReadFile("/proc/net/wireguard"); err == nil {
		lines := bytes.Split(data, []byte("\n"))
		for _, line := range lines {
			if len(line) > 0 && line[0] != '#' && line[0] != '\t' {
				interfaces = append(interfaces, strings.TrimSpace(string(line)))
			}
		}
	}

	// 从 /sys/class/net 扫描
	if len(interfaces) == 0 {
		entries, _ := os.ReadDir("/sys/class/net")
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "wg") {
				interfaces = append(interfaces, entry.Name())
			}
		}
	}

	return interfaces
}

// detectTunnels 检测隧道配置
func (d *NetworkDetector) detectTunnels() []map[string]any {
	var tunnels []map[string]any

	// 检测 WireGuard 隧道
	if d.hasWireGuard() {
		for _, iface := range d.getWireGuardInterfaces() {
			info := d.getWireGuardInfo(iface)
			if info != nil {
				tunnels = append(tunnels, info)
			}
		}
	}

	// 检测 Cloudflare Tunnel
	cfTunnels := d.detectCloudflareTunnels()
	tunnels = append(tunnels, cfTunnels...)

	return tunnels
}

// getWireGuardInfo 获取 WireGuard 接口信息
func (d *NetworkDetector) getWireGuardInfo(iface string) map[string]any {
	info := make(map[string]any)
	info["type"] = "wireguard"
	info["interface"] = iface

	// 尝试使用 wg show 命令
	cmd := exec.Command("wg", "show", iface, "dump")
	output, err := cmd.Output()
	if err != nil {
		return info
	}

	// 解析输出（简化版）
	lines := bytes.Split(output, []byte("\n"))
	if len(lines) > 0 {
		info["configured"] = true
		info["peer_count"] = len(lines) - 1
	}

	return info
}

// detectCloudflareTunnels 检测 Cloudflare Tunnel
func (d *NetworkDetector) detectCloudflareTunnels() []map[string]any {
	var tunnels []map[string]any

	// 检查 cloudflared 进程
	cmd := exec.Command("pgrep", "cloudflared")
	if err := cmd.Run(); err != nil {
		return tunnels
	}

	// 尝试获取 tunnel 信息
	cmd = exec.Command("cloudflared", "tunnel", "list")
	output, err := cmd.Output()
	if err != nil {
		// 进程存在但命令失败，仍然标记有隧道
		tunnels = append(tunnels, map[string]any{
			"type":  "cloudflare",
			"status": "detected",
		})
		return tunnels
	}

	// 解析输出
	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		if len(line) > 0 {
			fields := bytes.Fields(line)
			if len(fields) > 0 {
				tunnels = append(tunnels, map[string]any{
					"type":       "cloudflare",
					"id":         string(fields[0]),
					"status":     "running",
					"raw_output": string(line),
				})
			}
		}
	}

	return tunnels
}

// hasRouteInfo 检测路由信息可用性
func (d *NetworkDetector) hasRouteInfo() bool {
	// 检查 ip 命令
	_, err := exec.LookPath("ip")
	if err == nil {
		return true
	}

	// 检查 route 命令
	_, err = exec.LookPath("route")
	return err == nil
}

// GetNetworkInterfaces 获取网络接口列表
func GetNetworkInterfaces() ([]map[string]any, error) {
	var interfaces []map[string]any

	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		iface := entry.Name()

		// 跳过本地回环
		if iface == "lo" {
			continue
		}

		info := make(map[string]any)
		info["name"] = iface

		// 获取 MAC 地址
		addrPath := filepath.Join("/sys/class/net", iface, "address")
		if addr, err := os.ReadFile(addrPath); err == nil {
			info["mac"] = strings.TrimSpace(string(addr))
		}

		// 获取 IP 地址
		if ifaceObj, err := net.InterfaceByName(iface); err == nil {
			var ips []string
			addrs, _ := ifaceObj.Addrs()
			for _, addr := range addrs {
				ips = append(ips, addr.String())
			}
			info["ips"] = ips
		}

		interfaces = append(interfaces, info)
	}

	return interfaces, nil
}
