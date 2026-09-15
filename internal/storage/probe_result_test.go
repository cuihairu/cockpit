package storage

import (
	"testing"
	"time"
)

func TestProbeResultCRUD(t *testing.T) {
	db := testDB(t)
	base := time.Now().Add(-time.Hour)

	rows := []*ProbeResult{
		{ResourceType: "service", ResourceID: "svc-1", Name: "api", Status: "up", LatencyMs: 42, Message: "ok", CheckedAt: base},
		{ResourceType: "service", ResourceID: "svc-1", Name: "api", Status: "down", LatencyMs: 0, Message: "refused", CheckedAt: base.Add(time.Minute)},
		{ResourceType: "service", ResourceID: "svc-2", Name: "db", Status: "up", LatencyMs: 7, CheckedAt: base.Add(2 * time.Minute)},
		{ResourceType: "domain", ResourceID: "dom-1", Name: "example.com", Status: "active", CheckedAt: base},
	}
	if err := db.CreateProbeResults(rows); err != nil {
		t.Fatalf("CreateProbeResults: %v", err)
	}
	// 空批量是 no-op 而非错误
	if err := db.CreateProbeResults(nil); err != nil {
		t.Fatalf("CreateProbeResults(nil): %v", err)
	}

	got, err := db.ListProbeResults("service", "svc-1", 10)
	if err != nil {
		t.Fatalf("ListProbeResults: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows for svc-1, want 2", len(got))
	}
	// 倒序：最新在前
	if got[0].Status != "down" || got[1].Status != "up" {
		t.Errorf("order = [%s, %s], want [down, up]", got[0].Status, got[1].Status)
	}
	// ID 由 BeforeCreate 生成
	if got[0].ID == "" {
		t.Error("ID should be auto-generated")
	}

	// limit 生效
	limited, err := db.ListProbeResults("service", "svc-1", 1)
	if err != nil || len(limited) != 1 {
		t.Fatalf("limit=1 got %d rows (err=%v), want 1", len(limited), err)
	}

	// 其他目标隔离
	others, _ := db.ListProbeResults("service", "svc-2", 10)
	if len(others) != 1 {
		t.Errorf("svc-2 got %d rows, want 1", len(others))
	}
}

func TestDeleteProbeResultsOlderThan(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	rows := []*ProbeResult{
		{ResourceType: "service", ResourceID: "a", Status: "up", CheckedAt: now.Add(-31 * 24 * time.Hour)},
		{ResourceType: "service", ResourceID: "a", Status: "up", CheckedAt: now.Add(-29 * 24 * time.Hour)},
		{ResourceType: "service", ResourceID: "a", Status: "up", CheckedAt: now},
	}
	if err := db.CreateProbeResults(rows); err != nil {
		t.Fatalf("CreateProbeResults: %v", err)
	}

	n, err := db.DeleteProbeResultsOlderThan(now.Add(-30 * 24 * time.Hour))
	if err != nil {
		t.Fatalf("DeleteProbeResultsOlderThan: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted %d rows, want 1", n)
	}
	rest, _ := db.ListProbeResults("service", "a", 10)
	if len(rest) != 2 {
		t.Errorf("remaining %d rows, want 2", len(rest))
	}
}

func TestHasUnreadAlert(t *testing.T) {
	db := testDB(t)

	rt, rid, title := "service", "svc-1", "服务宕机"
	exists, err := db.HasUnreadAlert(rt, rid, title)
	if err != nil || exists {
		t.Fatalf("empty db: exists=%v err=%v, want false", exists, err)
	}

	read := true
	if err := db.CreateAlert(&Alert{Type: "error", Title: title, Message: "m", ResourceID: &rid, ResourceType: &rt, Read: read}); err != nil {
		t.Fatalf("CreateAlert read: %v", err)
	}
	if exists, _ := db.HasUnreadAlert(rt, rid, title); exists {
		t.Error("read alert should not count as unread")
	}

	if err := db.CreateAlert(&Alert{Type: "error", Title: title, Message: "m", ResourceID: &rid, ResourceType: &rt, Read: false}); err != nil {
		t.Fatalf("CreateAlert unread: %v", err)
	}
	if exists, _ := db.HasUnreadAlert(rt, rid, title); !exists {
		t.Error("unread alert should be detected")
	}

	// 不同资源 / 不同标题 / 空资源匹配空
	if exists, _ := db.HasUnreadAlert(rt, "svc-2", title); exists {
		t.Error("different resource should not match")
	}
	if exists, _ := db.HasUnreadAlert(rt, rid, "其他标题"); exists {
		t.Error("different title should not match")
	}
	if exists, _ := db.HasUnreadAlert("", "", "其他标题"); exists {
		t.Error("unrelated empty-key query should not match")
	}
}
