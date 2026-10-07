package rpc

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ============ Firewall Provider ============
//
// 防火墙规则集只读快照（见 docs/guide/firewall-design.md）：
// 优先 nft -j list ruleset，nft 不可用回落 iptables-save /
// ip6tables-save（D2/D4）；无工具或读数失败返回 available=false +
// 原因（D8），不报错；字段白名单扁平结构（D5）。

const (
	// firewallReadTimeout 读数命令超时（D4）
	firewallReadTimeout = 10 * time.Second
	// firewallTotalTimeout 整次 RPC 整体上限
	firewallTotalTimeout = 30 * time.Second
	// firewallMaxOutput 输出体积上限（D4）：nft 超限只回 meta 不解析
	// （超大 JSON 无法安全半解析），iptables 按行界截断
	firewallMaxOutput = 4 << 20
)

// 命令名/探测做成变量仅为测试可注入
var (
	firewallNftBin       = "nft"
	firewallIptablesBin  = "iptables"
	firewallIptablesSave = "iptables-save"
	firewallIp6Save      = "ip6tables-save"
	firewallLookPathFn   = exec.LookPath
	// iptablesVersionRe 提取 variant：iptables v1.8.9 (nf_tables)
	iptablesVersionRe = regexp.MustCompile(`\((nf_tables|legacy)\)`)
)

// FirewallProvider 防火墙规则集观测，挂在 firewall capability 下（D1）
type FirewallProvider struct {
	run Commander
}

// NewFirewallProvider 创建 provider；run 为 nil 时使用真实命令执行
func NewFirewallProvider(run Commander) *FirewallProvider {
	if run == nil {
		run = defaultCommander
	}
	return &FirewallProvider{run: run}
}

// Type RPC provider 类型（firewall capability）
func (p *FirewallProvider) Type() string { return "firewall" }

// Call RPC 分发
func (p *FirewallProvider) Call(action string, _ map[string]interface{}) (interface{}, error) {
	if action != "status" {
		return nil, fmt.Errorf("unsupported action %q", action)
	}
	return p.readFirewall()
}

// readFirewall 双后端选择 + 读数编排（D2）
func (p *FirewallProvider) readFirewall() (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), firewallTotalTimeout)
	defer cancel()

	hasNft := p.hasBin(firewallNftBin)
	hasIpt := p.hasBin(firewallIptablesBin)
	if !hasNft && !hasIpt {
		return map[string]interface{}{
			"available": false, "backend": "",
			"tables": []firewallTable{}, "totalRules": 0, "truncated": false,
		}, nil
	}

	if hasNft {
		if resp, ok := p.readNft(ctx); ok {
			return resp, nil
		}
		// nft 在但读数失败：回落 iptables（可能是 nftables 内核组件缺、
		// 权限差异等），两端全败才 available=false（D2）
	}

	resp := p.readIptables(ctx, hasIpt)
	return resp, nil
}

// readNft nftables 读数；失败返回 ok=false 走回落
func (p *FirewallProvider) readNft(ctx context.Context) (map[string]interface{}, bool) {
	cmdCtx, cancel := context.WithTimeout(ctx, firewallReadTimeout)
	defer cancel()

	out, stderr, err := p.run(cmdCtx, firewallNftBin, "-j", "list", "ruleset")
	if err != nil {
		return nil, false
	}
	resp := map[string]interface{}{
		"available": true, "backend": "nftables",
		"backendVersion": p.binVersion(firewallNftBin),
		"tables":         []firewallTable{}, "totalRules": 0, "truncated": false,
	}
	if len(out) > firewallMaxOutput {
		// 超大 ruleset 不半解析：只回 meta（D4），错误说明带原样 stderr
		resp["truncated"] = true
		resp["error"] = fmt.Sprintf("ruleset output %d bytes exceeds %d limit, summary only",
			len(out), firewallMaxOutput)
		_ = stderr
		return resp, true
	}
	tables, total, perr := parseNftRuleset(out)
	if perr != nil {
		resp["available"] = false
		resp["error"] = perr.Error()
		return resp, true
	}
	resp["tables"] = tables
	resp["totalRules"] = total
	return resp, true
}

// readIptables iptables-save（v4 + 可选 v6）读数
func (p *FirewallProvider) readIptables(ctx context.Context, hasIpt bool) map[string]interface{} {
	resp := map[string]interface{}{
		"available": true, "backend": "iptables",
		"iptablesVariant": p.iptablesVariant(),
		"tables":          []firewallTable{}, "totalRules": 0, "truncated": false,
	}
	if !hasIpt {
		resp["available"] = false
		resp["error"] = "no firewall read tool available"
		return resp
	}

	var errs []string
	total := 0
	tables := []firewallTable{}
	truncated := false

	// v4 必须，失败则整包不可用；v6 尽力而为，缺/失败静默省略
	tables4, total4, trunc, err := p.readSave(ctx, firewallIptablesSave, "ipv4")
	if err != nil {
		resp["available"] = false
		resp["error"] = firewallErrSummary(err)
		return resp
	}
	tables = append(tables, tables4...)
	total += total4
	truncated = truncated || trunc

	if _, err := firewallLookPathFn(firewallIp6Save); err == nil {
		tables6, total6, trunc6, err6 := p.readSave(ctx, firewallIp6Save, "ipv6")
		if err6 != nil {
			errs = append(errs, "ip6tables: "+firewallErrSummary(err6))
		} else {
			tables = append(tables, tables6...)
			total += total6
			truncated = truncated || trunc6
		}
	}
	if len(errs) > 0 {
		resp["error"] = strings.Join(errs, "; ")
	}
	resp["tables"] = tables
	resp["totalRules"] = total
	resp["truncated"] = truncated
	return resp
}

// readSave 单次 save 命令读数 + 截断语义（D4：按行界截断）
func (p *FirewallProvider) readSave(ctx context.Context, bin, family string) ([]firewallTable, int, bool, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, firewallReadTimeout)
	defer cancel()

	out, stderr, err := p.run(cmdCtx, bin)
	if err != nil {
		return nil, 0, false, fmt.Errorf("%s: %s", bin, firewallErrSummary(stderr, err))
	}
	truncated := false
	if len(out) > firewallMaxOutput {
		// 行界截断：丢掉最后半行，保证不解析到坏行
		cut := out[:firewallMaxOutput]
		if nl := strings.LastIndexByte(string(cut), '\n'); nl > 0 {
			cut = cut[:nl]
		}
		out = cut
		truncated = true
	}
	tables, total := parseIptablesSave(out, family)
	return tables, total, truncated, nil
}

// iptablesVariant iptables --version 里的 (nf_tables)/(legacy)，取不到为空
func (p *FirewallProvider) iptablesVariant() string {
	cmdCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, _, err := p.run(cmdCtx, firewallIptablesBin, "--version")
	if err != nil {
		return ""
	}
	if m := iptablesVersionRe.FindSubmatch(out); len(m) > 1 {
		return string(m[1])
	}
	return ""
}

// binVersion 首行版本串（展示用，解析失败为空）
func (p *FirewallProvider) binVersion(bin string) string {
	cmdCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, _, err := p.run(cmdCtx, bin, "--version")
	if err != nil {
		return ""
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	if len(line) > 80 {
		line = line[:80]
	}
	return line
}

// hasBin LookPath 探测
func (p *FirewallProvider) hasBin(bin string) bool {
	_, err := firewallLookPathFn(bin)
	return err == nil
}

// firewallErrSummary 错误摘要（stderr 首行优先，控制长度）
func firewallErrSummary(parts ...any) string {
	for _, part := range parts {
		switch v := part.(type) {
		case []byte:
			if s := strings.TrimSpace(string(v)); s != "" {
				return truncateFirewallErr(s)
			}
		case error:
			if v != nil {
				return truncateFirewallErr(v.Error())
			}
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return truncateFirewallErr(s)
			}
		}
	}
	return "unknown firewall error"
}

func truncateFirewallErr(s string) string {
	s = strings.SplitN(s, "\n", 2)[0]
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
