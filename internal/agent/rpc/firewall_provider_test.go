package rpc

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// firewallFakeRun 可编程 Commander：按命令名返回预置输出；
// `--version` 调用单独走 versions（读数与探测同名不同参）
func firewallFakeRun(outputs map[string][]byte, errs map[string]error) Commander {
	return func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) > 0 && args[len(args)-1] == "--version" {
			if out, ok := outputs[name+"--version"]; ok {
				return out, nil, nil
			}
		}
		if e, ok := errs[name]; ok {
			return nil, []byte("Permission denied (you must be root)"), e
		}
		if out, ok := outputs[name]; ok {
			return out, nil, nil
		}
		return nil, nil, errors.New("unexpected command: " + name)
	}
}

// withFirewallBins 临时替换命令名与 LookPath 探测（只有出现的命令放行）
func withFirewallBins(t *testing.T, present ...string) {
	t.Helper()
	oldNft, oldIpt, oldSave, oldSave6, oldLook :=
		firewallNftBin, firewallIptablesBin, firewallIptablesSave, firewallIp6Save, firewallLookPathFn
	firewallNftBin, firewallIptablesBin = "nft", "iptables"
	firewallIptablesSave, firewallIp6Save = "iptables-save", "ip6tables-save"
	firewallLookPathFn = func(bin string) (string, error) {
		for _, p := range present {
			if p == bin {
				return "/usr/sbin/" + bin, nil
			}
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() {
		firewallNftBin, firewallIptablesBin, firewallIptablesSave, firewallIp6Save, firewallLookPathFn =
			oldNft, oldIpt, oldSave, oldSave6, oldLook
	})
}

const firewallNftSample = `{"nftables":[
 {"metainfo":{"version":"1.0.9"}},
 {"table":{"family":"inet","name":"filter","handle":1}},
 {"chain":{"family":"inet","table":"filter","name":"input","handle":2,"policy":"accept"}},
 {"rule":{"family":"inet","table":"filter","chain":"input","handle":12,
   "expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":9000}},
           {"counter":{"packets":123,"bytes":4567}},{"verdict":{"code":"accept"}}]}},
 {"rule":{"family":"inet","table":"filter","chain":"input","handle":13,
   "comment":"cockpit:allow-agent","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":22}},
           {"verdict":{"code":"accept"}}]}},
 {"table":{"family":"ip","name":"nat","handle":3}},
 {"chain":{"family":"ip","table":"nat","name":"prerouting","handle":4,"policy":"accept"}},
 {"rule":{"family":"ip","table":"nat","chain":"prerouting","handle":14,
   "expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":8080}},
           {"masquerade":null}]}}
]}`

const firewallIptablesSaveSample = `*filter
:INPUT ACCEPT [100:200]
:FORWARD DROP [0:0]
:COCKPIT-CUSTOM - [0:0]
-A INPUT -p tcp -m tcp --dport 9000 -j ACCEPT
-A INPUT -m comment --comment "cockpit:allow-ssh" -p tcp --dport 22 -j ACCEPT
-A FORWARD -j COCKPIT-CUSTOM
COMMIT
*nat
:PREROUTING ACCEPT [0:0]
-A PREROUTING -d 1.2.3.4 -j MASQUERADE
COMMIT
`

func TestFirewallStatusNoTools(t *testing.T) {
	// nft/iptables 全缺 → available=false 不报错（D1/D8）
	withFirewallBins(t)
	p := NewFirewallProvider(firewallFakeRun(nil, nil))
	resp, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	m := resp.(map[string]interface{})
	if m["available"] != false {
		t.Errorf("available = %v, want false", m["available"])
	}
	if m["backend"] != "" {
		t.Errorf("backend = %v, want empty", m["backend"])
	}
}

func TestFirewallStatusNftables(t *testing.T) {
	// nft 路径：白名单解析 + cockpit: 注释识别（D5/D7）
	withFirewallBins(t, "nft")
	p := NewFirewallProvider(firewallFakeRun(map[string][]byte{
		"nft":          []byte(firewallNftSample),
		"nft--version": []byte("nft 1.0.9 (Leniel ~ librnl 3.9)"),
	}, nil))
	resp, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	m := resp.(map[string]interface{})
	if m["available"] != true || m["backend"] != "nftables" {
		t.Fatalf("available/backend = %v/%v, want true/nftables", m["available"], m["backend"])
	}
	if m["totalRules"] != 3 {
		t.Errorf("totalRules = %v, want 3", m["totalRules"])
	}
	if got, ok := m["backendVersion"].(string); !ok || !strings.Contains(got, "1.0.9") {
		t.Errorf("backendVersion = %v, want contains 1.0.9", m["backendVersion"])
	}
	tables := m["tables"].([]firewallTable)
	var inet *firewallTable
	for i := range tables {
		if tables[i].Family == "inet" {
			inet = &tables[i]
		}
	}
	if inet == nil {
		t.Fatalf("inet table missing: %+v", tables)
	}
	var input *firewallChain
	for i := range inet.Chains {
		if inet.Chains[i].Name == "input" {
			input = &inet.Chains[i]
		}
	}
	if input == nil || input.Policy != "accept" {
		t.Fatalf("input chain = %+v, want policy accept", inet)
	}
	if len(input.Rules) != 2 {
		t.Fatalf("input rules = %d, want 2", len(input.Rules))
	}
	if !strings.Contains(input.Rules[0].Text, "tcp dport") ||
		!strings.Contains(input.Rules[0].Text, "accept") {
		t.Errorf("rule text = %q, want match+verdict rendering", input.Rules[0].Text)
	}
	if input.Rules[0].Packets != 123 || input.Rules[0].Bytes != 4567 {
		t.Errorf("counters = %d/%d, want 123/4567", input.Rules[0].Packets, input.Rules[0].Bytes)
	}
	if !input.Rules[1].OwnedByCockpit {
		t.Errorf("cockpit comment rule not flagged: %+v", input.Rules[1])
	}
	if input.Rules[0].OwnedByCockpit {
		t.Errorf("plain rule wrongly flagged: %+v", input.Rules[0])
	}
	// nat 表 masquerade 规则（未知 kind → kind token 文本）
	var nat *firewallTable
	for i := range tables {
		if tables[i].Name == "nat" {
			nat = &tables[i]
		}
	}
	if nat == nil || len(nat.Chains) != 1 || !strings.Contains(nat.Chains[0].Rules[0].Text, "masquerade") {
		t.Errorf("nat table = %+v", nat)
	}
}

func TestFirewallStatusNftPermissionFailsFallbackToIptables(t *testing.T) {
	// nft 读数失败（非 root）→ 回落 iptables（D2/D8）
	withFirewallBins(t, "nft", "iptables", "iptables-save")
	runs := firewallFakeRun(map[string][]byte{
		"iptables-save": []byte(firewallIptablesSaveSample),
		"iptables":      []byte("iptables v1.8.9 (nf_tables)"),
	}, map[string]error{"nft": errors.New("exit status 4")})
	p := NewFirewallProvider(runs)
	resp, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	m := resp.(map[string]interface{})
	if m["available"] != true || m["backend"] != "iptables" {
		t.Fatalf("available/backend = %v/%v, want true/iptables", m["available"], m["backend"])
	}
	if m["iptablesVariant"] != "nf_tables" {
		t.Errorf("iptablesVariant = %v, want nf_tables", m["iptablesVariant"])
	}
	if m["totalRules"] != 4 {
		t.Errorf("totalRules = %v, want 4 (3 v4 + 1 nat)", m["totalRules"])
	}
	tables := m["tables"].([]firewallTable)
	var filter *firewallTable
	for i := range tables {
		if tables[i].Name == "filter" {
			filter = &tables[i]
		}
	}
	if filter == nil || filter.Family != "ipv4" {
		t.Fatalf("filter table = %+v", filter)
	}
	var input *firewallChain
	for i := range filter.Chains {
		if filter.Chains[i].Name == "INPUT" {
			input = &filter.Chains[i]
		}
	}
	if input == nil || input.Policy != "accept" || len(input.Rules) != 2 {
		t.Fatalf("INPUT chain = %+v, want 2 rules", input)
	}
	if !input.Rules[1].OwnedByCockpit {
		t.Errorf("cockpit comment rule not flagged: %+v", input.Rules[1])
	}
	// 自定义链无策略：policy 置空
	var custom *firewallChain
	for i := range filter.Chains {
		if filter.Chains[i].Name == "COCKPIT-CUSTOM" {
			custom = &filter.Chains[i]
		}
	}
	if custom == nil || custom.Policy != "" {
		t.Errorf("COCKPIT-CUSTOM = %+v, want empty policy", custom)
	}
}

func TestFirewallStatusBothBackendsFail(t *testing.T) {
	// nft 与 iptables-save 全败（典型非 root）→ available=false + 原因（D8）
	withFirewallBins(t, "nft", "iptables", "iptables-save")
	runs := firewallFakeRun(map[string][]byte{
		"iptables": []byte("iptables v1.8.9 (legacy)"),
	}, map[string]error{"nft": errors.New("exit 1"), "iptables-save": errors.New("exit 4")})
	p := NewFirewallProvider(runs)
	resp, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	m := resp.(map[string]interface{})
	if m["available"] != false {
		t.Errorf("available = %v, want false", m["available"])
	}
	if e, _ := m["error"].(string); !strings.Contains(e, "Permission denied") {
		t.Errorf("error = %q, want stderr summary", e)
	}
}

func TestFirewallStatusIPv6BestEffort(t *testing.T) {
	// ip6tables-save 在：v6 表并入（family=ipv6）；ip6tables-save 失败不影响 v4（D4）
	withFirewallBins(t, "nft", "iptables", "iptables-save", "ip6tables-save")
	runs := firewallFakeRun(map[string][]byte{
		"nft":           []byte(firewallNftSample),
		"iptables-save": []byte(firewallIptablesSaveSample),
	}, map[string]error{"nft": errors.New("exit 1"), "ip6tables-save": errors.New("exit 4")})
	// nft 失败走 iptables；ip6tables-save 在 LookPath 中但命令失败 → 记 error 省 v6
	p := NewFirewallProvider(runs)
	resp, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	m := resp.(map[string]interface{})
	if m["available"] != true {
		t.Fatalf("available = %v, want true", m["available"])
	}
	if e, _ := m["error"].(string); !strings.Contains(e, "ip6tables") {
		t.Errorf("error = %q, want ip6tables failure note", e)
	}
	if m["totalRules"] != 4 {
		t.Errorf("totalRules = %v, want 4 (v6 failed, omitted)", m["totalRules"])
	}
}

func TestFirewallStatusIptablesOversizeTruncated(t *testing.T) {
	// 超限按行界截断 + truncated 标记（D4）
	withFirewallBins(t, "iptables", "iptables-save")
	big := &strings.Builder{}
	big.WriteString("*filter\n:INPUT ACCEPT [0:0]\n")
	for big.Len() < firewallMaxOutput+100 {
		big.WriteString("-A INPUT -s 10.0.0.1 -j ACCEPT\n")
	}
	big.WriteString("COMMIT\n")
	p := NewFirewallProvider(firewallFakeRun(map[string][]byte{
		"iptables":      []byte("iptables v1.8.7 (legacy)"),
		"iptables-save": []byte(big.String()),
	}, nil))
	resp, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	m := resp.(map[string]interface{})
	if m["truncated"] != true {
		t.Errorf("truncated = %v, want true", m["truncated"])
	}
	if m["available"] != true {
		t.Errorf("available = %v, want true (截断仍可用)", m["available"])
	}
	// 截断边界必须是完整行（COMMIT 之前全是 -A 行）
	tables := m["tables"].([]firewallTable)
	if len(tables) != 1 || tables[0].Name != "filter" {
		t.Fatalf("tables = %+v", tables)
	}
	for _, ch := range tables[0].Chains {
		for _, r := range ch.Rules {
			if !strings.HasPrefix(r.Text, "-s 10.0.0.1") && r.Text != "" {
				t.Errorf("partial line parsed: %q", r.Text)
			}
		}
	}
}

func TestFirewallStatusNftOversizeMetaOnly(t *testing.T) {
	// nft 超限：不解析只回 meta（D4）
	withFirewallBins(t, "nft")
	big := make([]byte, firewallMaxOutput+10)
	for i := range big {
		big[i] = 'x'
	}
	p := NewFirewallProvider(firewallFakeRun(map[string][]byte{"nft": big}, nil))
	resp, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	m := resp.(map[string]interface{})
	if m["truncated"] != true || m["available"] != true {
		t.Errorf("truncated/available = %v/%v, want true/true", m["truncated"], m["available"])
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("error missing for oversize nft output")
	}
}

func TestFirewallCallUnknownAction(t *testing.T) {
	p := NewFirewallProvider(nil)
	if _, err := p.Call("restart", nil); err == nil {
		t.Error("unsupported action should error")
	}
}
