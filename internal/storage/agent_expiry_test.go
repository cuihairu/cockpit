package storage

import (
	"strings"
	"testing"
	"time"
)

// agent_expiry_test.go 过期判定存储层（D-2026-10-08-3）：心跳回写 last_seen、
// 假在线行批量标离线（先查后改，返回被改 ID）。

func TestTouchAgentLastSeen(t *testing.T) {
	db := testDB(t)
	old := time.Now().Add(-30 * time.Minute)
	if err := db.UpsertAgent(&Agent{ID: "ag-touch", Hostname: "h", Status: "online", LastSeen: old}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ts := time.Now()
	if err := db.TouchAgentLastSeen("ag-touch", ts); err != nil {
		t.Fatalf("touch: %v", err)
	}

	got, err := db.GetAgent("ag-touch")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.LastSeen.Unix() != ts.Unix() {
		t.Errorf("last_seen = %v, want %v", got.LastSeen, ts)
	}
	// 只动 last_seen，状态不受影响
	if got.Status != "online" {
		t.Errorf("status = %q, want online (touch must not alter status)", got.Status)
	}
}

func TestMarkStaleAgentsOffline(t *testing.T) {
	db := testDB(t)
	stale := time.Now().Add(-10 * time.Minute)
	fresh := time.Now()
	// 注意：Agent.BeforeCreate 会就地改写入参的 LastSeen/FirstSeen 为 now，
	// 播种后必须用捕获的时间变量回拨，不能读 struct 字段
	seed := []struct {
		id     string
		status string
		ts     time.Time
	}{
		{"ag-stale-online", "online", stale},
		{"ag-fresh-online", "online", fresh},
		{"ag-stale-offline", "offline", stale},
	}
	for _, r := range seed {
		if err := db.UpsertAgent(&Agent{ID: r.id, Hostname: r.id, Status: r.status}); err != nil {
			t.Fatalf("seed %s: %v", r.id, err)
		}
		// 回拨到测试时刻（BeforeCreate 把新建行 last_seen 强制为 now）
		if err := db.UpdateAgentStatus(r.id, r.status, r.ts); err != nil {
			t.Fatalf("backdate %s: %v", r.id, err)
		}
	}

	removed, err := db.MarkStaleAgentsOffline(time.Now().Add(-5 * time.Minute))
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(removed) != 1 || removed[0] != "ag-stale-online" {
		t.Fatalf("removed = %v, want [ag-stale-online]", removed)
	}

	got, _ := db.GetAgent("ag-stale-online")
	if got.Status != "offline" {
		t.Errorf("stale-online status = %q, want offline", got.Status)
	}
	// last_seen 保留原值（过期判定证据，不覆盖成 sweep 时刻）
	if got.LastSeen.Unix() != stale.Unix() {
		t.Errorf("last_seen overwritten: %v, want %v", got.LastSeen, stale)
	}

	freshRow, _ := db.GetAgent("ag-fresh-online")
	if freshRow.Status != "online" {
		t.Errorf("fresh-online status = %q, want online", freshRow.Status)
	}

	// 已离线行不动（LastSeen 保持）
	gone, _ := db.GetAgent("ag-stale-offline")
	if gone.Status != "offline" || gone.LastSeen.Unix() != stale.Unix() {
		t.Errorf("offline row mutated: %+v", gone)
	}

	// 无过期行时返回空
	again, err := db.MarkStaleAgentsOffline(time.Now().Add(-5 * time.Minute))
	if err != nil || len(again) != 0 {
		t.Errorf("second sweep = %v, %v; want empty", again, err)
	}
}

// TestMarkStaleAgentsOfflineErrorBranches 错误分支收口（100% 覆盖纪律）：
// closed db 覆盖 Pluck 失败；BEFORE UPDATE 触发器阻断覆盖 Update 失败
// （须先落一行 stale-online 让 Pluck 拿到非空 ids，否则 len==0 提前返回
// 走不到 Update）——对齐 tag_test 的 RAISE(ABORT) 注入先例
func TestMarkStaleAgentsOfflineErrorBranches(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// Update 失败：有 stale 行（Pluck 命中）+ 触发器阻断 UPDATE
	if err := db.UpsertAgent(&Agent{ID: "ag-block", Hostname: "h", Status: "online"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.UpdateAgentStatus("ag-block", "online", time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if err := db.db.Exec(`CREATE TRIGGER agent_status_boom BEFORE UPDATE ON agents
		BEGIN SELECT RAISE(ABORT, 'agent update boom'); END`).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if _, err := db.MarkStaleAgentsOffline(time.Now()); err == nil ||
		!strings.Contains(err.Error(), "agent update boom") {
		t.Errorf("blocked update = %v, want boom error", err)
	}

	// Pluck 失败：closed db（触发器不影响 SELECT，须关库注入）
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := db.MarkStaleAgentsOffline(time.Now()); err == nil {
		t.Error("closed db should fail pluck, got nil")
	}
}
