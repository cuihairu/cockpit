package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ============ Overlay M3：网络加入/离开 + daemon 服务管理 ============
//
// 独立部署路线（overlay-design.md M3，D21-D27）：daemon 归 systemd，
// agent 只管理不宿主——本文件只做 CLI/systemctl 的调用者与状态读取者，
// 不 spawn/托管任何 daemon 进程。命令 argv 直传不过 shell（D10 纪律），
// 写操作双端校验（server 先挡一道，这里再挡一道，D23）。

const (
	// overlayJoinTimeout join/leave/systemctl 单命令超时（D23/D27）
	overlayJoinTimeout = 5 * time.Second
)

// overlayZTNetIDRe ZeroTier 网络 ID：16 位 hex（与 server 端同规则，D23）
var overlayZTNetIDRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

// overlayTailnetNameRe Tailscale tailnet 名：`-` 或完整 tailnet DNS 后缀
// （≥2 级标签，小写字母数字、点/连字符，各级标签字母或数字开头）。
// 整体锚定——交替分支若不整体加括号，`^-|xxx` 的后半段会退化成无锚
// 子串匹配，任何含合法域名的字符串都能混过校验。server 端同规则（D24/D29）
var overlayTailnetNameRe = regexp.MustCompile(`^(-|[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)+)$`)

// overlayDaemonUnit 工具 → systemd 单元名硬白名单（D27）。
// 只允许这两个单元：overlay 页绝不能成为动任意 systemd 单元的口子
// （通用 service provider 是给 /services 页的，权限资源不同）。
var overlayDaemonUnit = map[string]string{
	"zerotier":  "zerotier-one.service",
	"tailscale": "tailscaled.service",
}

// overlayServiceActions systemctl 动词白名单（D27）
var overlayServiceActions = map[string]bool{
	"start": true, "stop": true, "enable": true, "disable": true,
}

// overlayDetectSystemd 平台 systemd 探测（var 仅为测试可注入，同上方
// bin 名先例——CI 容器无 /run/systemd/system，daemon/service 的 systemd
// 分支必须能脱离真机环境断言）
var overlayDetectSystemd = DetectSystemd

// overlayStatusSnapshot join/leave 成功后回带快照的取数点（var 仅为测试
// 可注入，同 overlayDetectSystemd 先例——Status 是全降级设计、从不返回
// 错误，下方 refresh-failure 分支是防御性兜底，只有注入桩才能覆盖）
var overlayStatusSnapshot = func(p *OverlayProvider) (interface{}, error) { return p.Status() }

// SetOverlayIdentityFn 注入 join/leave 成功后刷新身份用的提取函数
// （D26；detector.OverlayIdentity 在 providers.go 注入）。未注入时
// 响应不带 identity 键——server 跳过 registry 更新，其余语义不变。
func (p *OverlayProvider) SetOverlayIdentityFn(fn func() map[string]interface{}) {
	p.identityFn = fn
}

// Join 加入网络：zerotier-cli join <16hex> / tailscale up（D23/D24）
func (p *OverlayProvider) Join(tool, netID string) (interface{}, error) {
	return p.changeNetwork("join", tool, netID)
}

// Leave 离开网络：zerotier-cli leave <16hex> / tailscale down（D23/D24）
func (p *OverlayProvider) Leave(tool, netID string) (interface{}, error) {
	return p.changeNetwork("leave", tool, netID)
}

// changeNetwork join/leave 共同流程：校验 → 执行 → 回带最新快照 + 身份。
// 失败不拉快照（命令都没成，快照只会掩盖错误）；成功回快照使前端免
// 二次请求（D26）。
func (p *OverlayProvider) changeNetwork(verb, tool, netID string) (interface{}, error) {
	switch tool {
	case "zerotier":
		if !overlayZTNetIDRe.MatchString(netID) {
			return nil, fmt.Errorf("invalid zerotier network id %q (expect 16 hex)", netID)
		}
		out, stderr, err := p.overlayRun(overlayZTCliBin, verb, netID)
		if err != nil {
			return nil, fmt.Errorf("zerotier-cli %s %s: %s", verb, netID, commandErrSummary(stderr, err))
		}
		// zerotier-cli 应答码：200 成功；101 = 已在网（leave 侧为 101 not
		// joined），语义上都是幂等达成，不当失败（D23）
		if code := strings.TrimSpace(string(out)); !strings.HasPrefix(code, "200") && !strings.HasPrefix(code, "101") {
			return nil, fmt.Errorf("zerotier-cli %s %s: unexpected response %q", verb, netID, clipLine(code, 120))
		}
	case "tailscale":
		if !overlayTailnetNameRe.MatchString(netID) {
			return nil, fmt.Errorf("invalid tailnet name %q", netID)
		}
		if verb == "leave" {
			// down：断连但保留登录态，再 join 可回；绝不用 logout
			// （清凭据 → 交互重认证，破坏性翻倍，D24）
			if _, stderr, err := p.overlayRun(overlayTailscaleBin, "down"); err != nil {
				return nil, fmt.Errorf("tailscale down: %s", commandErrSummary(stderr, err))
			}
		} else {
			// up 未登录时是交互式认证流（打印 auth URL 等浏览器），
			// 非交互执行必挂——先验 BackendState，未登录拒绝并回引导（D24）
			state, dnsName, err := p.tailscaleBackend()
			if err != nil {
				return nil, err
			}
			if state != "Running" {
				return nil, fmt.Errorf("tailscale not logged in (state %s)；请先在该机执行 tailscale up 完成登录后再来面板管理", state)
			}
			if netID != "-" {
				if got := tailnetFromDNSName(dnsName); got != "" && !strings.EqualFold(got, netID) {
					return nil, fmt.Errorf("tailnet mismatch: device belongs to %q, not %q", got, netID)
				}
			}
			if _, stderr, err := p.overlayRun(overlayTailscaleBin, "up"); err != nil {
				return nil, fmt.Errorf("tailscale up: %s", commandErrSummary(stderr, err))
			}
		}
	default:
		// wireguard/frp 无 join/leave 语义（D25），未知工具一并拒绝
		return nil, fmt.Errorf("tool %s does not support join/leave (supported: zerotier, tailscale)", tool)
	}

	result, err := overlayStatusSnapshot(p)
	if err != nil {
		return nil, fmt.Errorf("command succeeded but status refresh failed: %w", err)
	}
	data := map[string]interface{}{"status": result}
	if p.identityFn != nil {
		if id := p.identityFn(); id != nil {
			data["identity"] = id
		}
	}
	return data, nil
}

// tailscaleBackend 读 BackendState 与 Self.DNSName（join 前置校验，D24）。
// status 命令失败原样报错——前置检查失败就是不能 join，无需二次包装。
func (p *OverlayProvider) tailscaleBackend() (state, dnsName string, err error) {
	out, stderr, err := p.overlayRun(overlayTailscaleBin, "status", "--json")
	if err != nil {
		return "", "", fmt.Errorf("tailscale status: %s", commandErrSummary(stderr, err))
	}
	var st struct {
		BackendState string `json:"BackendState"`
		Self         *struct {
			DNSName string `json:"DNSName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(clipOutput(out), &st); err != nil {
		return "", "", fmt.Errorf("invalid tailscale status json: %w", err)
	}
	if st.Self != nil {
		dnsName = st.Self.DNSName
	}
	return st.BackendState, dnsName, nil
}

// tailnetFromDNSName 从 "host.tailnet-name.ts.net." 提取 tailnet DNS
// 后缀（去掉主机名首段，剩 "tailnet-name.ts.net"，即 Tailscale 的 tailnet
// 标识形态，与 join 入参 networkId 同口径）。标签不足 3 段（主机名 +
// ≥2 段后缀都凑不齐）返回空串——调用方据此跳过一致性比较（旧版/自建
// coordination server 的 DNSName 形态不保证，宁可不比不可错杀）。
func tailnetFromDNSName(dnsName string) string {
	dnsName = strings.TrimSuffix(dnsName, ".")
	parts := strings.Split(dnsName, ".")
	if len(parts) < 3 {
		return ""
	}
	return strings.Join(parts[1:], ".")
}

// Daemon 读各工具 daemon/服务状态（D27）：CLI 存在性 + 版本 + systemd
// 单元存在性与运行态。非 systemd 平台 unit 字段缺席（降级而非报错）。
func (p *OverlayProvider) Daemon() (interface{}, error) {
	return map[string]interface{}{"tools": []map[string]interface{}{
		p.daemonZeroTier(),
		p.daemonTailscale(),
	}}, nil
}

func (p *OverlayProvider) daemonZeroTier() map[string]interface{} {
	m := p.daemonBase("zerotier", overlayZTCliBin, "zerotier-one")
	if m["installed"] == true {
		if out, _, err := p.overlayRun(overlayZTCliBin, "--version"); err == nil {
			if v := firstLine(out); v != "" {
				m["version"] = v
			}
		}
	}
	return m
}

func (p *OverlayProvider) daemonTailscale() map[string]interface{} {
	m := p.daemonBase("tailscale", overlayTailscaleBin, "tailscaled")
	if m["installed"] == true {
		if out, _, err := p.overlayRun(overlayTailscaleBin, "version"); err == nil {
			if v := firstLine(out); v != "" {
				m["version"] = v
			}
		}
	}
	return m
}

// daemonBase 安装态 + systemd 单元态 + 缺装引导。installed 只判 CLI
// （daemon 二进制与包同装，无独立探测价值——如 tailscale/tailscaled
// 是两个路径但同一包，zerotier-cli 与 zerotier-one 同包）。
func (p *OverlayProvider) daemonBase(tool, cliBin, daemonBin string) map[string]interface{} {
	m := map[string]interface{}{"tool": tool, "installed": false}
	if _, err := exec.LookPath(cliBin); err != nil {
		if _, dErr := exec.LookPath(daemonBin); dErr != nil {
			m["missingGuide"] = overlayInstallGuide(tool)
			return m
		}
	}
	m["installed"] = true
	if !overlayDetectSystemd() {
		return m
	}
	unit := overlayDaemonUnit[tool]
	// is-enabled 对不存在单元退出码 1；先 cat 判存在，再取两态
	if _, _, err := p.overlayRun("systemctl", "cat", unit); err == nil {
		m["unitExists"] = true
		m["active"] = p.systemdFlag("is-active", unit)
		m["enabled"] = p.systemdFlag("is-enabled", unit)
	} else {
		m["unitExists"] = false
	}
	return m
}

// systemdFlag is-active/is-enabled 退出码 0 = true（inactive/disabled/
// static 均非 0 退出，语义即 false，输出不参与判定）
func (p *OverlayProvider) systemdFlag(verb, unit string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), overlayJoinTimeout)
	defer cancel()
	_, _, err := p.run(ctx, "systemctl", verb, unit)
	return err == nil
}

// Service 启停/自启管理（D27）：工具 → 单元名硬白名单 + 动词白名单，
// argv 直传 systemctl。
func (p *OverlayProvider) Service(tool, action string) (interface{}, error) {
	unit, ok := overlayDaemonUnit[tool]
	if !ok {
		return nil, fmt.Errorf("tool %q has no manageable overlay daemon unit (supported: zerotier, tailscale)", tool)
	}
	if !overlayServiceActions[action] {
		return nil, fmt.Errorf("unsupported service action %q (allowed: start stop enable disable)", action)
	}
	if !overlayDetectSystemd() {
		return nil, fmt.Errorf("systemd not available on this platform")
	}
	if _, _, err := p.overlayRun("systemctl", "cat", unit); err != nil {
		return nil, fmt.Errorf("unit %s is not installed; install the %s package first", unit, tool)
	}
	if _, stderr, err := p.overlayRun("systemctl", action, unit); err != nil {
		return nil, fmt.Errorf("systemctl %s %s: %s", action, unit, commandErrSummary(stderr, err))
	}
	return map[string]interface{}{"tool": tool, "unit": unit, "action": action}, nil
}

// overlayInstallGuide 缺装引导文案（静态字符串，不执行探测——agent
// 包管理器形态各异，面板只做安装路径提示）
func overlayInstallGuide(tool string) string {
	switch tool {
	case "zerotier":
		return "未检测到 zerotier-cli。安装：curl -s https://install.zerotier.com | sudo bash（或发行版包 zerotier-one）"
	case "tailscale":
		return "未检测到 tailscale CLI。安装：curl -fsSL https://tailscale.com/install.sh | sh（或发行版包 tailscale）"
	}
	return ""
}

// overlayRun join/leave/service 的命令执行：argv 直传（D10），独立短超时。
// 与读观测同一 Commander 注入点——测试注入假执行器即可断言 argv。
func (p *OverlayProvider) overlayRun(name string, args ...string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), overlayJoinTimeout)
	defer cancel()
	return p.run(ctx, name, args...)
}

// firstLine 输出首行（TrimSpace 后），全空返回 ""
func firstLine(out []byte) string {
	s := strings.TrimSpace(string(out))
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// clipLine 短摘要截断（错误文本用，避免把整段响应塞进错误消息）
func clipLine(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
