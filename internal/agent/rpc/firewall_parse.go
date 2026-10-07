package rpc

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ============ 防火墙规则集解析 ============
//
// docs/guide/firewall-design.md D5/D7：输出为白名单扁平规则列表，
// 不透传原始 JSON；`cockpit:` 注释前缀识别为 cockpit 名下规则（D7，
// 机制预留——cockpit 当前不创建任何规则）。

// cockpitCommentPrefix cockpit 名下规则的注释前缀（D7）
const cockpitCommentPrefix = "cockpit:"

type firewallRule struct {
	Handle         int    `json:"handle,omitempty"`
	Text           string `json:"text"`
	Packets        uint64 `json:"packets,omitempty"`
	Bytes          uint64 `json:"bytes,omitempty"`
	OwnedByCockpit bool   `json:"ownedByCockpit"`
}

type firewallChain struct {
	Name   string         `json:"name"`
	Policy string         `json:"policy,omitempty"`
	Rules  []firewallRule `json:"rules"`
}

type firewallTable struct {
	Family string          `json:"family"`
	Name   string          `json:"name"`
	Chains []firewallChain `json:"chains"`
}

// ---------- nftables（nft -j list ruleset） ----------

// parseNftRuleset 解析 nft -j JSON 输出为表/链/规则扁平结构。
// nftables 顶层是对象数组（metainfo/table/chain/rule 混排），
// chain/rule 通过 family+table(+chain) 名回挂。
func parseNftRuleset(out []byte) ([]firewallTable, int, error) {
	var doc struct {
		Nftables []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, 0, fmt.Errorf("invalid nft json: %w", err)
	}

	type tableKey struct{ family, name string }
	tables := make([]firewallTable, 0, 8)
	tableIdx := map[tableKey]int{}
	chains := map[tableKey]map[string]*firewallChain{}
	total := 0

	// chain 元数据先收全，rule 再挂（nft 输出 chain 不保证在 rule 前）
	type chainKey struct{ family, table, name string }
	chainMeta := map[chainKey]*firewallChain{}

	for _, obj := range doc.Nftables {
		if raw, ok := obj["table"]; ok {
			var t struct {
				Family string `json:"family"`
				Name   string `json:"name"`
			}
			if json.Unmarshal(raw, &t) == nil && t.Name != "" {
				key := tableKey{t.Family, t.Name}
				if _, dup := tableIdx[key]; !dup {
					tableIdx[key] = len(tables)
					tables = append(tables, firewallTable{Family: t.Family, Name: t.Name})
					chains[key] = map[string]*firewallChain{}
				}
			}
			continue
		}
		if raw, ok := obj["chain"]; ok {
			var c struct {
				Family string `json:"family"`
				Table  string `json:"table"`
				Name   string `json:"name"`
				Policy string `json:"policy"`
			}
			if json.Unmarshal(raw, &c) == nil && c.Name != "" {
				meta := &firewallChain{Name: c.Name, Policy: strings.ToLower(c.Policy)}
				chainMeta[chainKey{c.Family, c.Table, c.Name}] = meta
			}
			continue
		}
		if raw, ok := obj["rule"]; ok {
			r, err := parseNftRule(raw)
			if err != nil {
				continue // 单规则坏对象跳过，不炸整包（smart D5 同则）
			}
			key := chainKey{r.family, r.table, r.chain}
			meta := chainMeta[key]
			if meta == nil {
				// 无 chain 头的孤儿规则：补一个隐式链
				meta = &firewallChain{Name: r.chain}
				chainMeta[key] = meta
			}
			meta.Rules = append(meta.Rules, r.rule)
			total++
		}
	}

	// 元数据回挂到表结构，链名稳定排序保证输出可 diff
	for ck, meta := range chainMeta {
		tk := tableKey{ck.family, ck.table}
		idx, ok := tableIdx[tk]
		if !ok {
			idx = len(tables)
			tableIdx[tk] = idx
			tables = append(tables, firewallTable{Family: ck.family, Name: ck.table})
			chains[tk] = map[string]*firewallChain{}
		}
		chains[tk][ck.name] = meta
	}
	for key, idx := range tableIdx {
		names := make([]string, 0, len(chains[key]))
		for name := range chains[key] {
			names = append(names, name)
		}
		sort.Strings(names)
		tables[idx].Chains = make([]firewallChain, 0, len(names))
		for _, name := range names {
			tables[idx].Chains = append(tables[idx].Chains, *chains[key][name])
		}
	}
	return tables, total, nil
}

// nftParsedRule 解析期中间态（定位链 + 携带结果）
type nftParsedRule struct {
	family, table, chain string
	rule                 firewallRule
}

// parseNftRule 单条规则：expr 数组渲染为空格连接的摘要文本（D5）
func parseNftRule(raw json.RawMessage) (nftParsedRule, error) {
	var r struct {
		Family  string           `json:"family"`
		Table   string           `json:"table"`
		Chain   string           `json:"chain"`
		Handle  int              `json:"handle"`
		Comment string           `json:"comment"`
		Expr    []map[string]any `json:"expr"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nftParsedRule{}, err
	}

	var parts []string
	var pkts, bytes uint64
	for _, expr := range r.Expr {
		for kind, body := range expr {
			b, _ := body.(map[string]any)
			switch kind {
			case "counter":
				pkts, bytes = counterValues(b)
			case "match":
				if s := renderNftMatch(b); s != "" {
					parts = append(parts, s)
				}
			case "verdict":
				parts = append(parts, renderNftVerdict(b))
			case "immediate":
				parts = append(parts, renderNftImmediate(b))
			default:
				// limit/log/masquerade/reject/…：kind 即语义 token
				parts = append(parts, kind)
			}
		}
	}
	rule := firewallRule{
		Handle:  r.Handle,
		Text:    strings.Join(parts, " "),
		Packets: pkts,
		Bytes:   bytes,
	}
	if strings.HasPrefix(r.Comment, cockpitCommentPrefix) {
		rule.OwnedByCockpit = true
		if r.Comment != "" {
			rule.Text = strings.TrimSpace(rule.Text + " # " + r.Comment)
		}
	}
	return nftParsedRule{family: r.Family, table: r.Table, chain: r.Chain, rule: rule}, nil
}

// counterValues 提取 counter 表达式的包数/字节数
func counterValues(b map[string]any) (uint64, uint64) {
	toU64 := func(v any) uint64 {
		switch n := v.(type) {
		case float64:
			return uint64(n)
		case string:
			var out uint64
			fmt.Sscanf(n, "%d", &out)
			return out
		}
		return 0
	}
	if b == nil {
		return 0, 0
	}
	return toU64(b["packets"]), toU64(b["bytes"])
}

// renderNftMatch 渲染 match 表达式：payload/meta 左值 + op + 右值
func renderNftMatch(b map[string]any) string {
	if b == nil {
		return ""
	}
	op, _ := b["op"].(string)
	var left, right string
	if l, ok := b["left"].(map[string]any); ok {
		if p, ok := l["payload"].(map[string]any); ok {
			proto, _ := p["protocol"].(string)
			field, _ := p["field"].(string)
			left = strings.TrimSpace(proto + " " + field)
		} else if m, ok := l["meta"].(map[string]any); ok {
			key, _ := m["key"].(string)
			left = key
		} else if ct, ok := l["ct"].(map[string]any); ok {
			key, _ := ct["key"].(string)
			left = "ct " + key
		}
	}
	right = renderNftValue(b["right"])
	if left == "" && right == "" {
		return ""
	}
	if op == "" {
		op = "=="
	}
	return strings.TrimSpace(left + " " + op + " " + right)
}

// renderNftValue 右值：标量/数组/set 一律转可读文本
func renderNftValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strings.TrimSuffix(fmt.Sprintf("%v", t), ".0")
	case []any:
		items := make([]string, 0, len(t))
		for _, item := range t {
			items = append(items, renderNftValue(item))
		}
		return "{" + strings.Join(items, ", ") + "}"
	case map[string]any:
		if set, ok := t["set"].([]any); ok {
			return renderNftValue(set)
		}
		if interval, ok := t["interval"].([]any); ok {
			return renderNftValue(interval)
		}
		return "set"
	}
	return fmt.Sprintf("%v", v)
}

// renderNftVerdict verdict/jump/goto 表达式
func renderNftVerdict(b map[string]any) string {
	if b == nil {
		return ""
	}
	if code, ok := b["code"].(string); ok {
		return code
	}
	if target, ok := b["target"].(string); ok {
		return "jump " + target
	}
	return "verdict"
}

// renderNftImmediate immediate 表达式：data 内可能是 verdict 或标量
func renderNftImmediate(b map[string]any) string {
	if b == nil {
		return ""
	}
	data, ok := b["data"].(map[string]any)
	if !ok {
		return renderNftValue(b["data"])
	}
	if verdict, ok := data["verdict"].(map[string]any); ok {
		return renderNftVerdict(verdict)
	}
	return renderNftValue(data)
}

// ---------- iptables（iptables-save / ip6tables-save 文本） ----------

// parseIptablesSave 解析 iptables-save 文本；family 标记 v4/v6 由调用方
// 传（iptables-save="ipv4"、ip6tables-save="ipv6"）。行式格式：
//
//	*table / :CHAIN POLICY [pkts:bytes] / -A CHAIN 规则体 / COMMIT
func parseIptablesSave(out []byte, family string) ([]firewallTable, int) {
	tables := make([]firewallTable, 0, 4)
	var cur *firewallTable
	chainIdx := map[string]int{}
	total := 0

	flush := func() {
		cur = nil
		chainIdx = map[string]int{}
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "*"):
			tables = append(tables, firewallTable{Family: family, Name: line[1:]})
			cur = &tables[len(tables)-1]
			chainIdx = map[string]int{}
		case cur == nil:
			// COMMIT / 空行 / 未进入表前的杂项
		case strings.HasPrefix(line, ":"):
			rest := strings.TrimPrefix(line, ":")
			name := rest
			policy := ""
			if sp := strings.IndexByte(rest, ' '); sp > 0 {
				name = rest[:sp]
				policy = strings.TrimSpace(rest[sp+1:])
				if br := strings.IndexByte(policy, '['); br > 0 {
					policy = strings.TrimSpace(policy[:br])
				}
				policy = strings.ToLower(policy)
				if policy == "-" { // 自定义链无策略（iptables-save 记为 -）
					policy = ""
				}
			}
			chainIdx[name] = len(cur.Chains)
			cur.Chains = append(cur.Chains, firewallChain{Name: name, Policy: policy})
		case strings.HasPrefix(line, "-A "):
			rest := strings.TrimPrefix(line, "-A ")
			sp := strings.IndexByte(rest, ' ')
			if sp <= 0 {
				continue
			}
			chainName := rest[:sp]
			body := strings.TrimSpace(rest[sp+1:])
			rule := firewallRule{Text: body}
			if strings.Contains(body, `--comment "cockpit:`) ||
				strings.Contains(body, `--comment cockpit:`) {
				rule.OwnedByCockpit = true
			}
			idx, ok := chainIdx[chainName]
			if !ok {
				idx = len(cur.Chains)
				chainIdx[chainName] = idx
				cur.Chains = append(cur.Chains, firewallChain{Name: chainName})
			}
			cur.Chains[idx].Rules = append(cur.Chains[idx].Rules, rule)
			total++
		case strings.HasPrefix(line, "COMMIT"):
			flush()
		}
	}
	return tables, total
}
