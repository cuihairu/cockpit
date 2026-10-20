package rpc

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/oob/liveness"
	"github.com/cuihairu/cockpit/internal/oob/redirect"
	"github.com/cuihairu/cockpit/internal/oob/wol"
	"github.com/cuihairu/cockpit/internal/openwrt"
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
	case "redirect":
		return p.ApplyRedirect(params)
	case "redirect.remove":
		return p.RemoveRedirect(params)
	case "redirect.list":
		return p.ListRedirects(params)
	case "redirect.sweep":
		return p.SweepRedirects(params)
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

// redirectExecutor 转发规则执行面（与 internal/openwrt 客户端同款能力，
// 注入点让测试不必起 ubus 端点）。
type redirectExecutor interface {
	AddFirewallRedirect(map[string]interface{}) error
	RemoveFirewallRedirect(string) error
	ListFirewallRedirects() ([]map[string]interface{}, error)
}

// newRedirectExecutor 客户端装配注入点：凭据按调用传入，客户端即用即弃
// （同 SSH 代理口令先例，内存即用即弃，不进日志与错误文案）。
var newRedirectExecutor = func(cfg openwrt.Config) redirectExecutor {
	return openwrt.NewClient(cfg)
}

// redirectConn 单次转发动作的连接参数（host 必需，其余可选）。
type redirectConn struct {
	host     string
	port     int
	username string
	password string
	insecure bool
}

// parseRedirectConn 从 RPC 参数取连接参数；端口取不到或非正则忽略（回落 http）。
func parseRedirectConn(params map[string]interface{}) (redirectConn, error) {
	var out redirectConn
	host, _ := params["host"].(string)
	out.host = strings.TrimSpace(host)
	if out.host == "" {
		return out, fmt.Errorf("host required")
	}
	if p := paramInt(params["port"]); p > 0 {
		out.port = p
	}
	out.username, _ = params["username"].(string)
	out.password, _ = params["password"].(string)
	out.insecure, _ = params["insecure_tls"].(bool)
	return out, nil
}

// redirectConnConfig 连接参数转客户端配置。
func (c redirectConn) config() openwrt.Config {
	return openwrt.Config{
		Host:        c.host,
		Port:        c.port,
		Username:    c.username,
		Password:    c.password,
		InsecureTLS: c.insecure,
	}
}

// redirectSpecParams 转发规格 + 连接参数（片C）。
type redirectSpecParams struct {
	spec redirect.Spec
	conn redirectConn
}

// parseRedirectSpecParams 解析下发规格：连接与规格逐字段校验；凭据只留内存。
func parseRedirectSpecParams(params map[string]interface{}) (redirectSpecParams, error) {
	var out redirectSpecParams
	if params == nil {
		return out, fmt.Errorf("oob.redirect: params required")
	}
	conn, err := parseRedirectConn(params)
	if err != nil {
		return out, fmt.Errorf("oob.redirect: %w", err)
	}
	out.conn = conn

	spec := redirect.Spec{}
	spec.Label, _ = params["label"].(string)
	spec.Proto, _ = params["proto"].(string)
	spec.SrcZone, _ = params["src_zone"].(string)
	spec.DstZone, _ = params["dst_zone"].(string)
	spec.SrcIP, _ = params["src_ip"].(string)
	spec.DstIP, _ = params["dst_ip"].(string)
	if p := paramInt(params["src_port"]); p >= 0 {
		spec.SrcPort = p
	}
	if p := paramInt(params["dst_port"]); p > 0 {
		spec.DstPort = p
	}
	if secs := paramInt(params["ttl_seconds"]); secs > 0 {
		spec.TTL = time.Duration(secs) * time.Second
	}
	if err := spec.Validate(); err != nil {
		return out, fmt.Errorf("oob.redirect: %w", err)
	}
	out.spec = spec
	return out, nil
}

// ApplyRedirect 下发一条兜底转发规则（action "redirect"）：规则名在 agent 侧
// 由规格与本机时钟生成（过期时刻进名字，重启不丢过期账），返回规则名与过期
// 时刻供 server 审计与展示。
func (p *OobProvider) ApplyRedirect(params map[string]interface{}) (interface{}, error) {
	q, err := parseRedirectSpecParams(params)
	if err != nil {
		return nil, err
	}
	name := redirect.RuleName(q.spec, time.Now())
	exec := newRedirectExecutor(q.conn.config())
	if err := exec.AddFirewallRedirect(redirect.ApplyPayload(q.spec, name)); err != nil {
		return nil, fmt.Errorf("oob.redirect: failed to apply redirect: %w", err)
	}
	_, expiry, _ := redirect.SplitRuleName(name)
	return map[string]interface{}{
		"name":        name,
		"expiry":      expiry.Unix(),
		"ttl_seconds": int64(q.spec.EffectiveTTL() / time.Second),
	}, nil
}

// RemoveRedirect 按规则名拆除（action "redirect.remove"）：只接受 cockpit 前缀
// 的规则名（命名即归属），不代为删除用户自建规则。
func (p *OobProvider) RemoveRedirect(params map[string]interface{}) (interface{}, error) {
	if params == nil {
		return nil, fmt.Errorf("oob.redirect.remove: params required")
	}
	name, _ := params["name"].(string)
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("oob.redirect.remove: name required")
	}
	if !redirect.Owned(name) {
		return nil, fmt.Errorf("oob.redirect.remove: %q is not a cockpit-owned rule", name)
	}
	conn, err := parseRedirectConn(params)
	if err != nil {
		return nil, fmt.Errorf("oob.redirect.remove: %w", err)
	}
	exec := newRedirectExecutor(conn.config())
	if err := exec.RemoveFirewallRedirect(name); err != nil {
		return nil, fmt.Errorf("oob.redirect.remove: failed to remove redirect: %w", err)
	}
	return map[string]interface{}{"name": name}, nil
}

// ListRedirects 读设备当前全部转发规则（action "redirect.list"）：
// 全量返回，展示侧再按 cockpit_ 前缀区分本包创建与用户自建。
func (p *OobProvider) ListRedirects(params map[string]interface{}) (interface{}, error) {
	if params == nil {
		return nil, fmt.Errorf("oob.redirect.list: params required")
	}
	conn, err := parseRedirectConn(params)
	if err != nil {
		return nil, fmt.Errorf("oob.redirect.list: %w", err)
	}
	exec := newRedirectExecutor(conn.config())
	body, err := exec.ListFirewallRedirects()
	if err != nil {
		return nil, fmt.Errorf("oob.redirect.list: failed to list redirects: %w", err)
	}
	return map[string]interface{}{"redirects": body}, nil
}

// SweepRedirects 清扫设备上已过期的兜底转发规则（action "redirect.sweep"）：
// 过期账在规则名里（cockpit_ 前缀 + 过期 Unix 时刻，见 internal/oob/redirect），
// 只拆前缀所属且时刻已过的规则——用户自建与未到期的都不碰；单条拆除失败
// 即中止并报错（不静默吞掉，宁可再生清扫一次）。返回实际拆除的规则名。
func (p *OobProvider) SweepRedirects(params map[string]interface{}) (interface{}, error) {
	if params == nil {
		return nil, fmt.Errorf("oob.redirect.sweep: params required")
	}
	conn, err := parseRedirectConn(params)
	if err != nil {
		return nil, fmt.Errorf("oob.redirect.sweep: %w", err)
	}
	exec := newRedirectExecutor(conn.config())
	rules, err := exec.ListFirewallRedirects()
	if err != nil {
		return nil, fmt.Errorf("oob.redirect.sweep: failed to list redirects: %w", err)
	}
	now := time.Now()
	removed := []string{}
	for _, rule := range rules {
		name, _ := rule["name"].(string)
		if !redirect.Owned(name) || !redirect.IsExpired(name, now) {
			continue
		}
		if err := exec.RemoveFirewallRedirect(name); err != nil {
			return nil, fmt.Errorf("oob.redirect.sweep: failed to remove %q: %w", name, err)
		}
		removed = append(removed, name)
	}
	return map[string]interface{}{"removed": removed, "count": len(removed)}, nil
}
