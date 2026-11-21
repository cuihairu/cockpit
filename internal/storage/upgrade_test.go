package storage

import (
	"testing"
	"time"
)

// TestSaveArtifactIdempotent 同 version+arch+os 覆盖更新（幂等，保留原 id）。
func TestSaveArtifactIdempotent(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	first := &AgentArtifact{ID: "art-1", Version: "1.2.0", Arch: "amd64", OS: "linux",
		Size: 100, SHA256: "aa", CreatedAt: time.Now()}
	if err := db.SaveArtifact(first); err != nil {
		t.Fatalf("save: %v", err)
	}
	second := &AgentArtifact{ID: "art-2", Version: "1.2.0", Arch: "amd64", OS: "linux",
		Size: 200, SHA256: "bb", CreatedAt: time.Now().Add(time.Second)}
	if err := db.SaveArtifact(second); err != nil {
		t.Fatalf("save 2: %v", err)
	}

	arts, err := db.ListArtifacts()
	if err != nil || len(arts) != 1 {
		t.Fatalf("artifacts = %d, err %v (want 1, idempotent)", len(arts), err)
	}
	if arts[0].ID != "art-1" || arts[0].Size != 200 {
		t.Fatalf("artifact = %+v, want id art-1 size 200 (overwrite)", arts[0])
	}
}

// TestGetArtifact 平台维度匹配；不存在返回 nil,nil。
func TestGetArtifact(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	if err := db.SaveArtifact(&AgentArtifact{ID: "a", Version: "1.0.0", Arch: "arm64", OS: "linux"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := db.GetArtifact("1.0.0", "arm64", "linux")
	if err != nil || got == nil {
		t.Fatalf("want hit, got %+v err %v", got, err)
	}
	got, err = db.GetArtifact("1.0.0", "arm64", "windows")
	if err != nil || got != nil {
		t.Fatalf("want miss, got %+v err %v", got, err)
	}
}

// TestAdvanceUpgradeConditional 条件推进：fromStatus 匹配才翻；重复推进命中 0。
func TestAdvanceUpgradeConditional(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	now := time.Now()
	u := &AgentUpgrade{ID: "u1", AgentID: "a1", FromVersion: "1.0.0", ToVersion: "1.1.0",
		Status: UpgradeStatusPlanned, CreatedAt: now}
	if err := db.CreateUpgrade(u); err != nil {
		t.Fatalf("create: %v", err)
	}
	// planned → deploying：命中 1，且记录 deployed_at
	hit, err := db.AdvanceUpgrade(u.ID, UpgradeStatusPlanned, UpgradeStatusDeploying, "armed", now)
	if err != nil || hit != 1 {
		t.Fatalf("advance planned→deploying: hit %d err %v (want 1)", hit, err)
	}
	got, _ := db.GetUpgrade(u.ID)
	if got.Status != UpgradeStatusDeploying || got.DeployedAt == nil {
		t.Fatalf("intent = %+v, want deploying with deployed_at", got)
	}
	// 同一意图再次用 planned 推进：命中 0（状态已变）
	hit, err = db.AdvanceUpgrade(u.ID, UpgradeStatusPlanned, UpgradeStatusVerifying, "stale", now)
	if err != nil || hit != 0 {
		t.Fatalf("stale advance: hit %d err %v (want 0)", hit, err)
	}
}

// TestListActiveUpgrades 终态不入巡检清单。
func TestListActiveUpgrades(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	now := time.Now()
	seeds := []AgentUpgrade{
		{ID: "u-act", AgentID: "a1", Status: UpgradeStatusDeploying, CreatedAt: now},
		{ID: "u-done", AgentID: "a2", Status: UpgradeStatusSucceeded, CreatedAt: now},
		{ID: "u-fail", AgentID: "a3", Status: UpgradeStatusFailed, CreatedAt: now},
	}
	for i := range seeds {
		if err := db.CreateUpgrade(&seeds[i]); err != nil {
			t.Fatalf("create %s: %v", seeds[i].ID, err)
		}
	}
	act, err := db.ListActiveUpgrades()
	if err != nil || len(act) != 1 || act[0].ID != "u-act" {
		t.Fatalf("active = %+v, err %v (want u-act only)", act, err)
	}
}

// TestGetUpgradeMissing 不存在返回 nil,nil。
func TestGetUpgradeMissing(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	got, err := db.GetUpgrade("nope")
	if err != nil || got != nil {
		t.Fatalf("want nil,nil, got %+v err %v", got, err)
	}
}

// TestListUpgradesByAgent agentID 过滤。
func TestListUpgradesByAgent(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	now := time.Now()
	for _, a := range []string{"a1", "a2"} {
		if err := db.CreateUpgrade(&AgentUpgrade{ID: "u-" + a, AgentID: a, Status: UpgradeStatusDeploying, CreatedAt: now}); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	all, err := db.ListUpgrades("", 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %d, err %v (want 2)", len(all), err)
	}
	one, err := db.ListUpgrades("a2", 0)
	if err != nil || len(one) != 1 || one[0].AgentID != "a2" {
		t.Fatalf("filtered = %+v, err %v (want a2 only)", one, err)
	}
}

// TestTerminalUpgradeStatus 终态集合判定。
func TestTerminalUpgradeStatus(t *testing.T) {
	terminal := []string{UpgradeStatusSucceeded, UpgradeStatusRolledBack, UpgradeStatusFailed}
	for _, s := range terminal {
		if !TerminalUpgradeStatus(s) {
			t.Errorf("TerminalUpgradeStatus(%q) = false, want true", s)
		}
	}
	for _, s := range []string{UpgradeStatusPlanned, UpgradeStatusDeploying, UpgradeStatusVerifying, ""} {
		if TerminalUpgradeStatus(s) {
			t.Errorf("TerminalUpgradeStatus(%q) = true, want false", s)
		}
	}
}

// TestSaveArtifactKeepsCreatedAt 覆盖更新时保留原 CreatedAt。
func TestSaveArtifactKeepsCreatedAt(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	orig := time.Now().Add(-time.Hour)
	if err := db.SaveArtifact(&AgentArtifact{ID: "art-1", Version: "1.2.0",
		Arch: "amd64", OS: "linux", Size: 10, CreatedAt: orig}); err != nil {
		t.Fatalf("save: %v", err)
	}
	// 第二次上传不填 CreatedAt（零值）→ 沿用原登记时间
	if err := db.SaveArtifact(&AgentArtifact{ID: "art-x", Version: "1.2.0",
		Arch: "amd64", OS: "linux", Size: 20}); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	got, err := db.GetArtifact("1.2.0", "amd64", "linux")
	if err != nil || got == nil {
		t.Fatalf("get: %+v err %v", got, err)
	}
	if !got.CreatedAt.Equal(orig) {
		t.Fatalf("created_at = %v, want %v (kept)", got.CreatedAt, orig)
	}
}

// TestAdvanceUpgradeTerminal 终态推进写 finished_at。
func TestAdvanceUpgradeTerminal(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	now := time.Now()
	if err := db.CreateUpgrade(&AgentUpgrade{ID: "u1", AgentID: "a1",
		FromVersion: "1.0.0", ToVersion: "1.2.0", Status: UpgradeStatusPlanned,
		CreatedAt: now}); err != nil {
		t.Fatalf("create: %v", err)
	}
	hit, err := db.AdvanceUpgrade("u1", UpgradeStatusPlanned, UpgradeStatusSucceeded,
		"health ok", now)
	if err != nil || hit != 1 {
		t.Fatalf("advance hit = %d err %v (want 1)", hit, err)
	}
	got, _ := db.GetUpgrade("u1")
	if got.FinishedAt == nil {
		t.Fatalf("finished_at not set for terminal advance")
	}
}

// TestUpgradeStorageClosedDB 库故障读路径返回 error（非 nil,nil 误报）。
func TestUpgradeStorageClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()
	if _, err := db.GetArtifact("1.0.0", "amd64", "linux"); err == nil {
		t.Fatal("GetArtifact on closed db: want error")
	}
	if _, err := db.GetUpgrade("u1"); err == nil {
		t.Fatal("GetUpgrade on closed db: want error")
	}
}
