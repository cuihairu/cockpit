package redirect

import (
	"testing"
	"time"
)

func validSpec() Spec {
	return Spec{
		Label:   "amtbmc",
		Proto:   "tcp",
		SrcZone: "wan",
		DstZone: "lan",
		DstIP:   "192.168.1.10",
		DstPort: 16992,
	}
}

// TestValidate 逐项校验与空标签/可选字段放行。
func TestValidate(t *testing.T) {
	if err := validSpec().Validate(); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}

	optional := validSpec()
	optional.Label = ""
	if err := optional.Validate(); err != nil {
		t.Fatalf("empty label must be allowed: %v", err)
	}

	bad := map[string]Spec{
		"bad label":    {Label: "Bad Label!", Proto: "tcp", SrcZone: "wan", DstZone: "lan", DstIP: "1.2.3.4", DstPort: 1},
		"long label":   {Label: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Proto: "tcp", SrcZone: "wan", DstZone: "lan", DstIP: "1.2.3.4", DstPort: 1},
		"bad proto":    {Label: "a", Proto: "icmp", SrcZone: "wan", DstZone: "lan", DstIP: "1.2.3.4", DstPort: 1},
		"no src zone":  {Label: "a", Proto: "tcp", SrcZone: "  ", DstZone: "lan", DstIP: "1.2.3.4", DstPort: 1},
		"no dst zone":  {Label: "a", Proto: "tcp", SrcZone: "wan", DstZone: "", DstIP: "1.2.3.4", DstPort: 1},
		"bad dst ip":   {Label: "a", Proto: "tcp", SrcZone: "wan", DstZone: "lan", DstIP: "not-an-ip", DstPort: 1},
		"bad src ip":   {Label: "a", Proto: "tcp", SrcZone: "wan", DstZone: "lan", SrcIP: "1.2.3.999", DstIP: "1.2.3.4", DstPort: 1},
		"port 0":       {Label: "a", Proto: "tcp", SrcZone: "wan", DstZone: "lan", DstIP: "1.2.3.4", DstPort: 0},
		"port high":    {Label: "a", Proto: "tcp", SrcZone: "wan", DstZone: "lan", DstIP: "1.2.3.4", DstPort: 65536},
		"src port neg": {Label: "a", Proto: "tcp", SrcZone: "wan", DstZone: "lan", SrcIP: "1.2.3.4", SrcPort: -1, DstIP: "1.2.3.4", DstPort: 1},
		"src port hi":  {Label: "a", Proto: "tcp", SrcZone: "wan", DstZone: "lan", SrcIP: "1.2.3.4", SrcPort: 70000, DstIP: "1.2.3.4", DstPort: 1},
		"ttl neg":      {Label: "a", Proto: "tcp", SrcZone: "wan", DstZone: "lan", DstIP: "1.2.3.4", DstPort: 1, TTL: -time.Second},
	}
	for name, spec := range bad {
		if err := spec.Validate(); err == nil {
			t.Fatalf("%s: want error, got nil", name)
		}
	}
}

// TestTTL 缺省与上限收敛。
func TestTTL(t *testing.T) {
	cases := map[string]struct {
		in   time.Duration
		want time.Duration
	}{
		"zero uses default":     {0, DefaultTTL},
		"negative uses default": {-time.Second, DefaultTTL},
		"explicit":              {time.Minute, time.Minute},
		"clamped to max":        {MaxTTL + time.Hour, MaxTTL},
	}
	for name, c := range cases {
		if got := (Spec{TTL: c.in}).ttl(); got != c.want {
			t.Fatalf("%s: ttl = %v, want %v", name, got, c.want)
		}
	}
}

// TestRuleName 显式标签与推导标签，过期时刻进名字。
func TestRuleName(t *testing.T) {
	at := time.Unix(1_800_000_000, 0)

	name := RuleName(validSpec(), at)
	if name != RulePrefix+"amtbmc_1800001800" {
		t.Fatalf("name = %s", name)
	}

	derived := validSpec()
	derived.Label = ""
	if got := RuleName(derived, at); got != RulePrefix+"tcp.192.168.1.10.16992_1800001800" {
		t.Fatalf("derived name = %s", got)
	}

	ttled := validSpec()
	ttled.TTL = time.Minute
	if got := RuleName(ttled, at); got != RulePrefix+"amtbmc_1800000060" {
		t.Fatalf("ttled name = %s", got)
	}

	if !Owned(RuleName(validSpec(), at)) {
		t.Fatal("rule name must be owned")
	}
}

// TestSplitRuleName 反解、非本包名与非数字过期段。
func TestSplitRuleName(t *testing.T) {
	at := time.Unix(1_800_000_000, 0)
	label, expiry, ok := SplitRuleName(RuleName(validSpec(), at))
	if !ok || label != "amtbmc" || !expiry.Equal(at.Add(DefaultTTL)) {
		t.Fatalf("split = %q %v %v", label, expiry, ok)
	}

	for _, name := range []string{"user_rule", "", "cockpit_", "cockpit_no_expiry", "cockpit_a_notanumber", "cockpit_a_0x1f"} {
		if _, _, ok := SplitRuleName(name); ok {
			t.Fatalf("%q must not split", name)
		}
	}
}

// TestIsExpired 过期判定与非本包规则恒为 false。
func TestIsExpired(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	at := now.Add(-time.Hour)
	fresh := RuleName(Spec{Label: "a", TTL: 2 * time.Hour}, at)
	if IsExpired(fresh, now) {
		t.Fatalf("%s must not be expired", fresh)
	}
	if !IsExpired(RuleName(Spec{Label: "a", TTL: time.Hour}, at), now) {
		t.Fatal("expired rule not detected")
	}
	if IsExpired("user_rule", now) {
		t.Fatal("foreign rule must never be treated as expired")
	}
}

// TestApplyPayload 必填字段恒定、可选字段按空值省略。
func TestApplyPayload(t *testing.T) {
	spec := validSpec()
	payload := ApplyPayload(spec, "cockpit_amtbmc_1")
	if payload["name"] != "cockpit_amtbmc_1" || payload["proto"] != "tcp" ||
		payload["src"] != "wan" || payload["dest"] != "lan" ||
		payload["dest_ip"] != "192.168.1.10" || payload["dest_port"] != "16992" {
		t.Fatalf("payload = %v", payload)
	}
	if _, has := payload["src_ip"]; has {
		t.Fatalf("empty src ip must be omitted: %v", payload)
	}
	if _, has := payload["src_port"]; has {
		t.Fatalf("zero src port must be omitted: %v", payload)
	}

	full := validSpec()
	full.SrcIP = "10.0.0.5"
	full.SrcPort = 51000
	payload = ApplyPayload(full, "cockpit_x")
	if payload["src_ip"] != "10.0.0.5" || payload["src_port"] != "51000" {
		t.Fatalf("optional fields = %v", payload)
	}
}

// TestRemovePayload 拆除只带规则名。
func TestRemovePayload(t *testing.T) {
	payload := RemovePayload("cockpit_amtbmc_1")
	if len(payload) != 1 || payload["name"] != "cockpit_amtbmc_1" {
		t.Fatalf("payload = %v", payload)
	}
}
