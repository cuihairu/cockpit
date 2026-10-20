package rpc

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/oob/liveness"
	"github.com/cuihairu/cockpit/internal/oob/wol"
)

// netInterfaces / ifaceAddrs 覆盖率注入点（同 agent.localip.go interfaceAddrs
// 范式）：未导出包级 var，默认值即生产取值；同包测试注入假网卡与假地址，
// 既覆盖默认构造分支，也不依赖 CI 机器的网卡布局与真实广播地址
// （Go 1.27 起 net.Interface 为 struct，地址直接取自系统状态，无法按实例伪造）。
var (
	netInterfaces = func() ([]net.Interface, error) { return net.Interfaces() }
	ifaceAddrs    = func(ifc net.Interface) ([]net.Addr, error) { return ifc.Addrs() }
)

// ============ Oob Provider（带外兜底动作面） ============

// OobProvider 带外管理动作面：Wake-on-LAN 唤醒。WOL 是局域网内无认证动作，
// 本 provider 不需要凭据；发送由 LAN 内的 agent 直接发 UDP 广播，动作幂等
// （重复发送无害）。设计：docs/design/oob-openwrt-bootstrap.md 片 A。
type OobProvider struct {
	sender *wol.Sender                                                     // 测试注入假 dialer
	ifaces func() ([]net.Interface, error)                                 // 测试注入网卡枚举
	probe  func(context.Context, string, liveness.Options) liveness.Report // 测试注入探活桩
}

// NewOobProvider 创建带外 provider；任一注入点为 nil 时走真实 UDP、本机网卡
// 与真实三源探活。
func NewOobProvider(sender *wol.Sender, ifaces func() ([]net.Interface, error),
	probe func(context.Context, string, liveness.Options) liveness.Report) *OobProvider {
	if sender == nil {
		sender = wol.New(wol.Options{})
	}
	if ifaces == nil {
		ifaces = netInterfaces
	}
	if probe == nil {
		probe = liveness.Probe
	}
	return &OobProvider{sender: sender, ifaces: ifaces, probe: probe}
}

// Type 返回 provider 类型（RPC 方法前缀 oob.<action>）。
func (p *OobProvider) Type() string { return "oob" }

// Call 分发动作。
func (p *OobProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "wol":
		return p.WakeOnLan(params)
	case "probe":
		return p.ProbeHost(params)
	}
	return nil, fmt.Errorf("unknown action: %s", action)
}

// wolParams 唤醒参数。
type wolParams struct {
	mac    string
	target string // 显式目标 ip 或 ip:port（优先于网卡推导）
	iface  string // 网卡名（缺省选首个可广播的非回环 IPv4）
	port   int
}

// parseWolParams 解析 WOL 参数：mac 必需，target/iface/port 可选。
func parseWolParams(params map[string]interface{}) (wolParams, error) {
	out := wolParams{port: wol.DefaultPort}
	if params == nil {
		return out, fmt.Errorf("oob.wol: params required")
	}
	mac, ok := params["mac"].(string)
	if !ok || strings.TrimSpace(mac) == "" {
		return out, fmt.Errorf("oob.wol: mac required")
	}
	out.mac = strings.TrimSpace(mac)
	if v, ok := params["target"].(string); ok {
		out.target = strings.TrimSpace(v)
	}
	if v, ok := params["iface"].(string); ok {
		out.iface = strings.TrimSpace(v)
	}
	if v, ok := params["port"]; ok {
		port := paramInt(v)
		if port < 0 {
			return out, fmt.Errorf("oob.wol: invalid port")
		}
		if port > 0 {
			out.port = port
		}
	}
	return out, nil
}

// paramInt 取端口参数（JSON 数字为 float64）；非法或缺失返回 -1。
func paramInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		i := int(n)
		if float64(i) != n || i < 0 || i > 65535 {
			return -1
		}
		return i
	case int:
		if n < 0 || n > 65535 {
			return -1
		}
		return n
	case string:
		i, err := strconv.Atoi(n)
		if err != nil || i < 0 || i > 65535 {
			return -1
		}
		return i
	}
	return -1
}

// resolveTarget 计算发送目标（ip:port）：显式 target 优先，否则按网卡网段
// 算广播地址。
func (p *OobProvider) resolveTarget(w wolParams) (string, error) {
	if w.target != "" {
		host, _, err := net.SplitHostPort(w.target)
		if err == nil {
			if net.ParseIP(host) == nil {
				return "", fmt.Errorf("oob.wol: invalid target host %q", host)
			}
			return w.target, nil
		}
		if net.ParseIP(w.target) == nil {
			return "", fmt.Errorf("oob.wol: invalid target %q (want ip:port or ip)", w.target)
		}
		return fmt.Sprintf("%s:%d", w.target, w.port), nil
	}
	ifaces, err := p.ifaces()
	if err != nil {
		return "", fmt.Errorf("oob.wol: list interfaces: %w", err)
	}
	for _, ifc := range ifaces {
		if w.iface != "" && ifc.Name != w.iface {
			continue
		}
		if ifc.Flags&net.FlagLoopback != 0 || ifc.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := ifaceAddrs(ifc)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil || ip4.IsLoopback() {
				continue
			}
			ones, _ := ipNet.Mask.Size()
			broadcast, err := wol.SubnetBroadcast(ip4.String(), strconv.Itoa(ones))
			if err != nil {
				continue
			}
			return fmt.Sprintf("%s:%d", broadcast, w.port), nil
		}
	}
	return "", fmt.Errorf("oob.wol: no broadcastable ipv4 interface found")
}

// WakeOnLan 发送魔术包（action "wol"）。返回发送字节数与目标地址，供审计留痕。
func (p *OobProvider) WakeOnLan(params map[string]interface{}) (interface{}, error) {
	w, err := parseWolParams(params)
	if err != nil {
		return nil, err
	}
	target, err := p.resolveTarget(w)
	if err != nil {
		return nil, err
	}
	sent, err := p.sender.Send(context.Background(), target, w.mac)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"sent":   sent,
		"target": target,
		"mac":    w.mac,
	}, nil
}

// probeParams 探活参数。
type probeParams struct {
	ip      string
	ports   []int
	timeout time.Duration // 0 → liveness.DefaultTimeout
}

// parseProbeParams 解析探活参数：ip 必需；ports/timeout（秒）可选。
func parseProbeParams(params map[string]interface{}) (probeParams, error) {
	out := probeParams{}
	if params == nil {
		return out, fmt.Errorf("oob.probe: params required")
	}
	ip, ok := params["ip"].(string)
	if !ok || strings.TrimSpace(ip) == "" {
		return out, fmt.Errorf("oob.probe: ip required")
	}
	out.ip = strings.TrimSpace(ip)
	if net.ParseIP(out.ip) == nil {
		return out, fmt.Errorf("oob.probe: invalid ip %q", out.ip)
	}
	if v, ok := params["ports"].([]interface{}); ok {
		for _, item := range v {
			port := paramInt(item)
			if port <= 0 {
				return out, fmt.Errorf("oob.probe: invalid port %v", item)
			}
			out.ports = append(out.ports, port)
		}
	}
	if v, ok := params["timeout"]; ok {
		secs := paramInt(v)
		if secs <= 0 || secs > 60 {
			return out, fmt.Errorf("oob.probe: invalid timeout")
		}
		out.timeout = time.Duration(secs) * time.Second
	}
	return out, nil
}

// ProbeHost 三源探活（action "probe"）：liveness.Report 原样返回（三态判定
// 在 agent 侧产生，server 只转发与审计）。
func (p *OobProvider) ProbeHost(params map[string]interface{}) (interface{}, error) {
	q, err := parseProbeParams(params)
	if err != nil {
		return nil, err
	}
	return p.probe(context.Background(), q.ip, liveness.Options{Timeout: q.timeout, Ports: q.ports}), nil
}
