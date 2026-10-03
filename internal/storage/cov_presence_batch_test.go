package storage

import (
	"strings"
	"testing"
	"time"
)

// cov_presence_batch_test.go：心跳元信息（StartedAt/Services）落库与
// 「离线 agent 一键清理」批量删除（DeleteAgents/CleanupOfflineAgents）的
// 核心逻辑与错误分支覆盖（bc1098c 引入）。

// ============ agentUpdateFields：started_at / services ============

func TestCovAgentUpdateFieldsPresence(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	st := time.Unix(1700000000, 0).UTC()
	// 单测两个字段块：StartedAt 非零写入、Services 非 nil 写入（含空切片）
	u := agentUpdateFields(&Agent{StartedAt: st, Services: []AgentService{}})
	if v, ok := u["started_at"]; !ok || v != st {
		t.Errorf("started_at = %v, want %v", v, st)
	}
	if v, ok := u["services"]; !ok || v != "[]" {
		t.Errorf("services = %#v, want []", v)
	}
	// 零值：两字段都不进 updates（老 agent 不带该字段）
	u = agentUpdateFields(&Agent{Hostname: "h"})
	if _, ok := u["started_at"]; ok {
		t.Error("zero StartedAt must be skipped")
	}
	if _, ok := u["services"]; ok {
		t.Error("nil Services must be skipped")
	}

	// 落库往返（UpsertAgent 路径）
	db.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"})
	db.UpsertAgent(&Agent{ID: "a1", Hostname: "h1", StartedAt: st,
		Services: []AgentService{{Protocol: "tcp", Host: "10.0.0.1", Port: 22, Name: "ssh"}}})
	got, err := db.GetAgent("a1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !got.StartedAt.Equal(st) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, st)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "ssh" || got.Services[0].Port != 22 {
		t.Errorf("Services = %+v", got.Services)
	}
	// 空切片语义：服务全关 → 清空库里的旧项
	db.UpsertAgent(&Agent{ID: "a1", Hostname: "h1", Services: []AgentService{}})
	got, _ = db.GetAgent("a1")
	if got.Services == nil || len(got.Services) != 0 {
		t.Errorf("Services after clear = %#v, want empty non-nil", got.Services)
	}
}

// ============ UpdateAgentPresence（心跳路径） ============

func TestCovUpdateAgentPresence(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"})
	st := time.Unix(1700000001, 0).UTC()

	// 零值/nil → 全部跳过，空 updates 短路不发查询
	if err := db.UpdateAgentPresence("a1", time.Time{}, nil); err != nil {
		t.Fatalf("presence skip: %v", err)
	}
	got, _ := db.GetAgent("a1")
	if !got.StartedAt.IsZero() {
		t.Errorf("StartedAt = %v, want zero", got.StartedAt)
	}

	// 只带 startedAt
	if err := db.UpdateAgentPresence("a1", st, nil); err != nil {
		t.Fatalf("presence startedAt: %v", err)
	}
	got, _ = db.GetAgent("a1")
	if !got.StartedAt.Equal(st) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, st)
	}

	// 只带 services：写入
	if err := db.UpdateAgentPresence("a1", time.Time{},
		[]AgentService{{Protocol: "tcp", Port: 3389, Name: "rdp"}}); err != nil {
		t.Fatalf("presence services: %v", err)
	}
	got, _ = db.GetAgent("a1")
	if len(got.Services) != 1 || got.Services[0].Name != "rdp" {
		t.Errorf("Services = %+v", got.Services)
	}
	// 两者同时 + 空切片清空
	if err := db.UpdateAgentPresence("a1", st, []AgentService{}); err != nil {
		t.Fatalf("presence both: %v", err)
	}
	got, _ = db.GetAgent("a1")
	if len(got.Services) != 0 || !got.StartedAt.Equal(st) {
		t.Errorf("after both: services=%#v startedAt=%v", got.Services, got.StartedAt)
	}
	// DB 关闭 → 更新失败
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := db.UpdateAgentPresence("a1", st, nil); err == nil {
		t.Error("closed db should error")
	}
}

// ============ ListSystemInfoSnapshotsByAgent ============

func TestCovListSystemInfoSnapshotsByAgent(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// 空入参短路（不发查询，关闭后也返回空 map）
	m, err := db.ListSystemInfoSnapshotsByAgent(nil)
	if err != nil || len(m) != 0 {
		t.Fatalf("empty = %v, %v", m, err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if m, err := db.ListSystemInfoSnapshotsByAgent(nil); err != nil || len(m) != 0 {
		t.Fatalf("closed empty = %v, %v", m, err)
	}
}

func TestCovListSystemInfoSnapshotsByAgentQuery(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if err := db.UpdateSystemInfoSnapshot(&SystemInfoSnapshot{AgentID: "a1", CPUCores: 8}); err != nil {
		t.Fatalf("seed a1: %v", err)
	}
	if err := db.UpdateSystemInfoSnapshot(&SystemInfoSnapshot{AgentID: "a2", CPUCores: 4}); err != nil {
		t.Fatalf("seed a2: %v", err)
	}
	// 命中 + 未命中 agent 混合
	m, err := db.ListSystemInfoSnapshotsByAgent([]string{"a1", "ghost"})
	if err != nil || len(m) != 1 {
		t.Fatalf("list = %v, %v", m, err)
	}
	if m["a1"] == nil || m["a1"].CPUCores != 8 {
		t.Errorf("a1 snapshot = %+v", m["a1"])
	}
	// 查询失败（DB 关闭、非空入参）
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := db.ListSystemInfoSnapshotsByAgent([]string{"a1"}); err == nil {
		t.Error("closed db should error")
	}
}

// ============ DeleteAgent：标签清理失败早退 ============

func TestCovDeleteAgentTagClearError(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"})
	// 关闭 DB → ClearAgentTags 失败 → DeleteAgent 在清理步早退，不删 agent
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := db.DeleteAgent("a1"); err == nil {
		t.Error("closed db should error")
	}
}

// ============ DeleteAgents 批量删除（一键清理核心） ============

func TestCovDeleteAgentsFlow(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// a1 带标签、a2 无标签、ghost 不存在（逐条跳过）
	db.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"})
	db.UpsertAgent(&Agent{ID: "a2", Hostname: "h2"})
	tag, err := db.CreateTag("prod", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := db.SetAgentTags("a1", []string{tag.ID}); err != nil {
		t.Fatalf("SetAgentTags: %v", err)
	}

	removed, err := db.DeleteAgents([]string{"a1", "ghost", "a2"})
	if err != nil {
		t.Fatalf("DeleteAgents: %v", err)
	}
	if len(removed) != 2 || removed[0] != "a1" || removed[1] != "a2" {
		t.Errorf("removed = %v, want [a1 a2]", removed)
	}
	if _, err := db.GetAgent("a1"); err == nil {
		t.Error("a1 should be deleted")
	}
	// 标签关联行随批删摘除，计数不虚高
	counts, err := db.TagCounts()
	if err != nil {
		t.Fatalf("TagCounts: %v", err)
	}
	if counts[tag.ID] != 0 {
		t.Errorf("tag count = %d, want 0", counts[tag.ID])
	}

	// 空入参短路
	removed, err = db.DeleteAgents(nil)
	if err != nil || removed != nil {
		t.Errorf("empty = %v, %v", removed, err)
	}
}

func TestCovDeleteAgentsErrors(t *testing.T) {
	// 内层查询失败（agents 表被删，非 NotFound）→ 事务中止错误透传
	db := testDB(t)
	if err := db.db.Exec("DROP TABLE agents").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := db.DeleteAgents([]string{"a1"}); err == nil {
		t.Error("dropped table should error")
	}
	db.Close()

	// 事务开启失败（DB 关闭）→ 错误透传
	db2 := testDB(t)
	db2.Close()
	if _, err := db2.DeleteAgents([]string{"a1"}); err == nil {
		t.Error("closed db should error")
	}
}

func TestCovDeleteAgentsTxTriggerErrors(t *testing.T) {
	// 关联行删除失败（事务内第一步）→ 事务回滚错误透传
	db := testDB(t)
	db.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"})
	tag, err := db.CreateTag("prod", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	// trigger 逐行触发，需先有真实关联行
	if err := db.SetAgentTags("a1", []string{tag.ID}); err != nil {
		t.Fatalf("SetAgentTags: %v", err)
	}
	if err := db.db.Exec(`CREATE TRIGGER assign_boom BEFORE DELETE ON agent_tag_assignments
		BEGIN SELECT RAISE(ABORT, 'assign delete boom'); END`).Error; err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if _, err := db.DeleteAgents([]string{"a1"}); err == nil ||
		!strings.Contains(err.Error(), "assign delete boom") {
		t.Errorf("assignment delete = %v, want trigger error", err)
	}
	// 回滚生效：agent 还在
	if _, err := db.GetAgent("a1"); err != nil {
		t.Errorf("agent must survive rollback: %v", err)
	}
	db.Close()

	// agent 本体删除失败（前置 First/关联删都成功）
	db2 := testDB(t)
	db2.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"})
	if err := db2.db.Exec(`CREATE TRIGGER agent_boom BEFORE DELETE ON agents
		BEGIN SELECT RAISE(ABORT, 'agent delete boom'); END`).Error; err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if _, err := db2.DeleteAgents([]string{"a1"}); err == nil ||
		!strings.Contains(err.Error(), "agent delete boom") {
		t.Errorf("agent delete = %v, want trigger error", err)
	}
	db2.Close()
}

// ============ CleanupOfflineAgents：清理步错误早退 ============

func TestCovCleanupOfflineAgentsClearError(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// 离线 agent + 已挂标签；关联删除注入失败 → 清理循环早退
	db.UpsertAgent(&Agent{ID: "a1", Hostname: "h1", Status: "offline",
		LastSeen: time.Now().Add(-48 * time.Hour)})
	tag, err := db.CreateTag("prod", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := db.SetAgentTags("a1", []string{tag.ID}); err != nil {
		t.Fatalf("SetAgentTags: %v", err)
	}
	if err := db.db.Exec(`CREATE TRIGGER assign_boom BEFORE DELETE ON agent_tag_assignments
		BEGIN SELECT RAISE(ABORT, 'assign delete boom'); END`).Error; err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if _, err := db.CleanupOfflineAgents(0); err == nil ||
		!strings.Contains(err.Error(), "assign delete boom") {
		t.Errorf("cleanup = %v, want trigger error", err)
	}
}

func TestCovCleanupOfflineAgentsThreshold(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now()
	seed := []struct {
		id       string
		status   string
		lastSeen time.Time
	}{
		{"old-off", "offline", now.Add(-96 * time.Hour)},  // 4 天前，三档全命中
		{"fresh-off", "offline", now.Add(-2 * time.Hour)}, // 2 小时前：24h 档不清
		{"stale-on", "online", now.Add(-96 * time.Hour)},  // 7 天未见但 status 仍 online
	}
	for _, s := range seed {
		if err := db.UpsertAgent(&Agent{ID: s.id, Hostname: s.id, Status: s.status}); err != nil {
			t.Fatalf("seed %s: %v", s.id, err)
		}
		// BeforeCreate 钩子会把 LastSeen 覆盖为 now，落库后单独回拨
		if err := db.db.Model(&Agent{}).Where("id = ?", s.id).
			Update("last_seen", s.lastSeen).Error; err != nil {
			t.Fatalf("set last_seen %s: %v", s.id, err)
		}
	}

	// timeout > 0：只按 last_seen cutoff（不叠加 status 条件）
	removed, err := db.CleanupOfflineAgents(72 * time.Hour)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed = %v, want old-off + stale-on", removed)
	}
	removedSet := map[string]bool{removed[0]: true, removed[1]: true}
	if !removedSet["old-off"] || !removedSet["stale-on"] {
		t.Errorf("removed = %v", removed)
	}
	if _, err := db.GetAgent("fresh-off"); err != nil {
		t.Errorf("fresh-off must survive 72h cutoff: %v", err)
	}
}
