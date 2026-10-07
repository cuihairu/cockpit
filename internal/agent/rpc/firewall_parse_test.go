package rpc

import (
	"strings"
	"testing"
)

func TestParseNftRulesetBadJSON(t *testing.T) {
	if _, _, err := parseNftRuleset([]byte("not json")); err == nil {
		t.Error("bad json should error")
	}
}

func TestParseNftRulesetEmpty(t *testing.T) {
	tables, total, err := parseNftRuleset([]byte(`{"nftables":[{"metainfo":{"version":"1.0.9"}}]}`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if total != 0 || len(tables) != 0 {
		t.Errorf("tables=%d total=%d, want 0/0", len(tables), total)
	}
}

func TestParseNftRulesetOrphanRule(t *testing.T) {
	// rule 先于 chain/table 出现（乱序输出）：隐式建链不丢规则
	sample := `{"nftables":[
	 {"rule":{"family":"inet","table":"filter","chain":"input","handle":9,
	   "expr":[{"verdict":{"code":"drop"}}]}}
	]}`
	tables, total, err := parseNftRuleset([]byte(sample))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if total != 1 || len(tables) != 1 || tables[0].Name != "filter" {
		t.Fatalf("tables=%+v total=%d", tables, total)
	}
	if len(tables[0].Chains) != 1 || tables[0].Chains[0].Name != "input" {
		t.Errorf("chains = %+v", tables[0].Chains)
	}
}

func TestParseNftRuleMalformedSkipped(t *testing.T) {
	// 单规则坏 expr 结构不炸整包：expr 缺失 → 空文本规则仍入列
	sample := `{"nftables":[
	 {"table":{"family":"ip","name":"mangle"}},
	 {"rule":{"family":"ip","table":"mangle","chain":"output","handle":1}}
	]}`
	tables, total, err := parseNftRuleset([]byte(sample))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if tables[0].Chains[0].Rules[0].Text != "" {
		t.Errorf("text = %q, want empty (no expr)", tables[0].Chains[0].Rules[0].Text)
	}
}

func TestRenderNftMatchSetRight(t *testing.T) {
	// 右值为 set/interval 集合
	got := renderNftMatch(map[string]any{
		"op": "!=",
		"left": map[string]any{"payload": map[string]any{
			"protocol": "tcp", "field": "dport"}},
		"right": map[string]any{"set": []any{float64(22), float64(80)}},
	})
	if got != "tcp dport != {22, 80}" {
		t.Errorf("renderNftMatch = %q", got)
	}
}

func TestRenderNftValueFloat(t *testing.T) {
	if got := renderNftValue(float64(9000)); got != "9000" {
		t.Errorf("renderNftValue = %q, want 9000", got)
	}
	if got := renderNftValue(float64(1.5)); got != "1.5" {
		t.Errorf("renderNftValue = %q, want 1.5", got)
	}
}

func TestParseIptablesSaveEmptyAndJunk(t *testing.T) {
	tables, total := parseIptablesSave([]byte(""), "ipv4")
	if len(tables) != 0 || total != 0 {
		t.Errorf("empty save → tables=%d total=%d", len(tables), total)
	}
	tables, total = parseIptablesSave([]byte("garbage\nlines\n"), "ipv4")
	if len(tables) != 0 || total != 0 {
		t.Errorf("no-table junk → tables=%d total=%d", len(tables), total)
	}
}

func TestParseIptablesSaveMalformedRuleSkipped(t *testing.T) {
	// -A 后无规则体的坏行跳过
	sample := "*filter\n:INPUT ACCEPT [0:0]\n-A INPUT\n-A INPUT -j DROP\nCOMMIT\n"
	tables, total := parseIptablesSave([]byte(sample), "ipv4")
	if total != 1 {
		t.Errorf("total = %d, want 1 (malformed skipped)", total)
	}
	if tables[0].Chains[0].Rules[0].Text != "-j DROP" {
		t.Errorf("text = %q", tables[0].Chains[0].Rules[0].Text)
	}
}

func TestParseIptablesSaveChainCountersAndDashPolicy(t *testing.T) {
	// :CHAIN - [0:0]（自定义链无策略）与带计数器的 policy 行
	sample := "*filter\n:INPUT ACCEPT [123:456]\n:FOO - [0:0]\nCOMMIT\n"
	tables, _ := parseIptablesSave([]byte(sample), "ipv4")
	if tables[0].Chains[0].Policy != "accept" {
		t.Errorf("INPUT policy = %q, want accept", tables[0].Chains[0].Policy)
	}
	if tables[0].Chains[1].Policy != "" {
		t.Errorf("FOO policy = %q, want empty", tables[0].Chains[1].Policy)
	}
}

func TestFirewallErrSummaryTruncated(t *testing.T) {
	long := strings.Repeat("x", 300)
	got := firewallErrSummary([]byte(long))
	// 200 字节 + "…"（3 字节 UTF-8）
	if len(got) > 203 || !strings.HasSuffix(got, "…") {
		t.Errorf("firewallErrSummary len = %d, want ≤203", len(got))
	}
	if got := firewallErrSummary(nil, nil, ""); got != "unknown firewall error" {
		t.Errorf("empty parts = %q", got)
	}
}

func TestIptablesVariantRegex(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    string
	}{
		{"iptables v1.8.9 (nf_tables)", "nf_tables"},
		{"iptables v1.8.7 (legacy)", "legacy"},
		{"iptables v1.8.9", ""},
	} {
		if got := iptablesVersionRe.FindStringSubmatch(tc.version); (len(got) > 1 && got[1] != tc.want) || (len(got) <= 1 && tc.want != "") {
			t.Errorf("iptablesVersionRe(%q) = %v, want %q", tc.version, got, tc.want)
		}
	}
}
