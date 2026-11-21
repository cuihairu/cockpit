package storage

import (
	"testing"
	"time"
)

// TestGetActiveJobByDedupKey 在途去重查询：同键 pending/running 命中（新在前），
// 终态不算在途（同键完成后再提交应无在途），空键不命中（无去重语义）。
func TestGetActiveJobByDedupKey(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	key := "dedup-key-1"
	now := time.Now()
	// 终态（failed）先落，再落 pending：去重应命中 pending，忽略 failed
	seeds := []*Job{
		{ID: "j-done", Type: "agent.exec", Target: "a", Status: JobStatusFailed,
			DedupKey: key, CreatedAt: now},
		{ID: "j-active", Type: "agent.exec", Target: "a", Status: JobStatusPending,
			DedupKey: key, CreatedAt: now.Add(time.Second)},
	}
	for _, j := range seeds {
		if err := db.CreateJob(j); err != nil {
			t.Fatalf("CreateJob %s: %v", j.ID, err)
		}
	}

	got, err := db.GetActiveJobByDedupKey(key)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if got == nil || got.ID != "j-active" {
		t.Fatalf("active = %+v, want j-active", got)
	}

	// 终态同键不再在途：把 j-active 落终态后再查 → 无
	if _, err := db.FailJob("j-active", JobStatusPending, now, "done"); err != nil {
		t.Fatalf("fail active: %v", err)
	}
	got, err = db.GetActiveJobByDedupKey(key)
	if err != nil {
		t.Fatalf("query after terminal: %v", err)
	}
	if got != nil {
		t.Fatalf("terminal dedup should be empty, got %+v", got)
	}

	// 空键不命中（feature 前行无去重语义）
	got, err = db.GetActiveJobByDedupKey("")
	if err != nil {
		t.Fatalf("empty key query: %v", err)
	}
	if got != nil {
		t.Fatalf("empty key should not match, got %+v", got)
	}
}

// TestGetActiveJobByDedupKeyDBClosed 闭库确定性构造查询失败分支。
func TestGetActiveJobByDedupKeyDBClosed(t *testing.T) {
	db := testDB(t)
	db.Close()
	if _, err := db.GetActiveJobByDedupKey("k"); err == nil {
		t.Fatal("closed db: want error")
	}
}
