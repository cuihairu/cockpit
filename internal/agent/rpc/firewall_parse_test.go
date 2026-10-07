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

func TestParseNftRulesetRenderBranches(t *testing.T) {
	// 渲染分支族（一次性打满）：meta/ct 左值、空 match/空 expr 兜底、
	// 缺省 op、counter 字符串/缺值/nil body、immediate 标量/nil/verdict/
	// 映射、value string/interval/bool/映射兜底、verdict jump/兜底/nil、
	// 坏规则对象跳过不炸整包
	sample := `{"nftables":[
	 {"table":{"family":"inet","name":"t1"}},
	 {"rule":{"family":"inet","table":"t1","chain":"c1","handle":1,
	   "expr":[ {"match":{"left":{"meta":{"key":"iifname"}},"right":"eth0"}},
	     {"match":null},
	     {"match":{}},
	     {"match":{"left":{"ct":{"key":"state"}},"op":"==","right":"established"}},
	     {"match":{"left":{"payload":{"protocol":"udp","field":"dport"}},"right":{"interval":["1000","2000"]}}},
	     {"match":{"left":{"payload":{"protocol":"icmp","field":"type"}},"right":true}},
	     {"counter":{"packets":1}},
	     {"counter":null},
	     {"counter":{"packets":"123","bytes":"4567"}},
	     {"immediate":{"data":42}},
	     {"immediate":null},
	     {"immediate":{"data":{"verdict":{"target":"web"}}}},
	     {"immediate":{"data":{"foo":"bar"}}}
	   ]}},
	 {"rule":123},
	 {"rule":{"family":"inet","table":"t1","chain":"c1","handle":2,
	   "expr":[{"verdict":{"target":"web2"}},{"verdict":{}},{"verdict":null}]}}
	]}`
	tables, total, err := parseNftRuleset([]byte(sample))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if total != 2 || len(tables) != 1 || len(tables[0].Chains) != 1 {
		t.Fatalf("tables=%+v total=%d, want 1 table/1 chain/2 rules", tables, total)
	}
	// 坏规则对象（{"rule":123}）跳过 → 只剩 handle 1/2 两条
	rules := tables[0].Chains[0].Rules
	if len(rules) != 2 {
		t.Fatalf("rules = %d, want 2 (bad rule skipped)", len(rules))
	}
	r1 := rules[0]
	// counter 三分支依次覆盖（末个字符串 counter 收尾）
	if r1.Packets != 123 || r1.Bytes != 4567 {
		t.Errorf("counters = %d/%d, want 123/4567 (string counter wins)", r1.Packets, r1.Bytes)
	}
	for _, want := range []string{
		"iifname == eth0", // meta 左值 + 缺省 op + string 右值
		"ct state == established",
		"udp dport == {1000, 2000}", // interval 右值
		"icmp type == true",         // bool 右值走默认 Sprintf
		"42",                        // immediate 标量
		"jump web",                  // immediate data.verdict → target
		"set",                       // immediate data 映射无 verdict → 映射兜底
	} {
		if !strings.Contains(r1.Text, want) {
			t.Errorf("rule1 text %q missing %q", r1.Text, want)
		}
	}
	r2 := rules[1]
	for _, want := range []string{"jump web2", "verdict"} {
		if !strings.Contains(r2.Text, want) {
			t.Errorf("rule2 text %q missing %q", r2.Text, want)
		}
	}
}

func TestParseIptablesSaveImplicitChain(t *testing.T) {
	// -A 指向尚未声明的链：隐式建链（规则行先于 :CHAIN 行）
	sample := "*filter\n-A NEWCHAIN -j DROP\n:INPUT ACCEPT [0:0]\nCOMMIT\n"
	tables, total := parseIptablesSave([]byte(sample), "ipv4")
	if total != 1 || len(tables) != 1 || len(tables[0].Chains) != 2 {
		t.Fatalf("tables=%+v total=%d, want implicit NEWCHAIN + INPUT", tables, total)
	}
	ch := tables[0].Chains[0]
	if ch.Name != "NEWCHAIN" || len(ch.Rules) != 1 || ch.Rules[0].Text != "-j DROP" {
		t.Errorf("implicit chain = %+v, want NEWCHAIN with 1 rule", ch)
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
	// string 部件：去空白 + 取首行
	if got := firewallErrSummary("  first line\nsecond"); got != "first line" {
		t.Errorf("string part = %q, want first line", got)
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
