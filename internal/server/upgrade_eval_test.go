package server

import (
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

// TestValidUpgradeVersion 版本可解析（semver-lite）。
func TestValidUpgradeVersion(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"1.2.0", true}, {"1.0", true}, {"1", true}, {"v1.2.3", true},
		{"", false}, {"1..2", false}, {"1.2.x", false}, {"a.b", false},
		{"1.2.0-alpha", false},
	}
	for _, c := range cases {
		if got := ValidUpgradeVersion(c.v); got != c.want {
			t.Errorf("ValidUpgradeVersion(%q) = %v, want %v", c.v, got, c.want)
		}
	}
}

// TestValidateArtifactMeta 产物校验表：版本/min-version/size/sha256。
func TestValidateArtifactMeta(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cases := []struct {
		name string
		ver  string
		min  string
		sha  string
		size int64
		max  int64
		ok   bool
	}{
		{"valid", "1.2.0", "", sha, 100, 1000, true},
		{"valid with min satisfied", "1.2.0", "1.0.0", sha, 100, 1000, true},
		{"bad version", "x.y", "", sha, 100, 1000, false},
		{"empty version", "", "", sha, 100, 1000, false},
		{"min not satisfied", "1.0.0", "1.2.0", sha, 100, 1000, false},
		{"bad min format", "1.2.0", "x", sha, 100, 1000, false},
		{"empty size", "1.2.0", "", sha, 0, 1000, false},
		{"over size bound", "1.2.0", "", sha, 2000, 1000, false},
		{"short sha", "1.2.0", "", "abc", 100, 1000, false},
		{"non-hex sha", "1.2.0", "", "zz" + sha[2:], 100, 1000, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateArtifactMeta(c.ver, c.min, c.sha, c.size, c.max)
			if (err == nil) != c.ok {
				t.Fatalf("validateArtifactMeta = %v, want ok=%v", err, c.ok)
			}
		})
	}
}

// TestUpgradeNextStatusTable 状态机推进（设计 §3.2/§3.3）。
func TestUpgradeNextStatusTable(t *testing.T) {
	now := time.Now()
	window := time.Minute
	// at = 时间点（DeployedAt/VerifiedAt）。old 在窗口外，recent 在窗口内。
	old := now.Add(-2 * time.Minute)
	recent := now.Add(-10 * time.Second)
	mk := func(status string, at time.Time) *storage.AgentUpgrade {
		return &storage.AgentUpgrade{
			ID: "u", AgentID: "a", FromVersion: "1.0.0", ToVersion: "1.2.0",
			Status: status, DeployedAt: &at, VerifiedAt: &at,
		}
	}
	cases := []struct {
		name   string
		u      *storage.AgentUpgrade
		agentV string
		window time.Duration
		now    time.Time
		capOK  bool
		want   string // "" = 保持
	}{
		{"planned arms", mk(storage.UpgradeStatusPlanned, recent), "1.0.0", window, now, true, storage.UpgradeStatusDeploying},
		{"deploying sees target → verifying", mk(storage.UpgradeStatusDeploying, recent), "1.2.0", window, now, true, storage.UpgradeStatusVerifying},
		{"deploying within window holds", mk(storage.UpgradeStatusDeploying, recent), "1.0.0", window, now, true, ""},
		{"deploying window expired still from → rolled_back", mk(storage.UpgradeStatusDeploying, old), "1.0.0", window, now, true, storage.UpgradeStatusRolledBack},
		{"deploying window expired agent gone → failed", mk(storage.UpgradeStatusDeploying, old), "", window, now, true, storage.UpgradeStatusFailed},
		{"deploying window expired unexpected version → failed", mk(storage.UpgradeStatusDeploying, old), "2.0.0", window, now, true, storage.UpgradeStatusFailed},
		{"verifying stable target full window → succeeded", mk(storage.UpgradeStatusVerifying, old), "1.2.0", window, now, true, storage.UpgradeStatusSucceeded},
		{"verifying within window holds", mk(storage.UpgradeStatusVerifying, recent), "1.2.0", window, now, true, ""},
		{"verifying reverted → rolled_back", mk(storage.UpgradeStatusVerifying, recent), "1.0.0", window, now, true, storage.UpgradeStatusRolledBack},
		{"verifying capability fail → rolled_back", mk(storage.UpgradeStatusVerifying, recent), "1.2.0", window, now, false, storage.UpgradeStatusRolledBack},
		{"verifying agent gone → failed", mk(storage.UpgradeStatusVerifying, old), "", window, now, true, storage.UpgradeStatusFailed},
		{"verifying unexpected → failed", mk(storage.UpgradeStatusVerifying, recent), "2.0.0", window, now, true, storage.UpgradeStatusFailed},
		{"terminal holds", mk(storage.UpgradeStatusSucceeded, recent), "1.2.0", window, now, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := upgradeNextStatus(c.u, c.agentV, c.window, c.now, c.capOK)
			if got != c.want {
				t.Fatalf("upgradeNextStatus(%s) = %q, want %q", c.u.Status, got, c.want)
			}
		})
	}
}

// TestUpgradeCapabilityOK 能力契约满足判定（空要求恒真）。
func TestUpgradeCapabilityOK(t *testing.T) {
	reported := []storage.Capability{{Type: "backup"}, {Type: "probe"}}
	if !upgradeCapabilityOK(reported, nil) {
		t.Fatal("nil requirement should pass")
	}
	if !upgradeCapabilityOK(reported, []string{"backup"}) {
		t.Fatal("backup required, declared → pass")
	}
	if upgradeCapabilityOK(reported, []string{"backup", "vms"}) {
		t.Fatal("vms required but missing → fail")
	}
}
