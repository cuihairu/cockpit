package storage

import "testing"

func testBindingDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDomainBindingCRUD(t *testing.T) {
	db := testBindingDB(t)

	b := &DomainBinding{Domain: "blog.example.com", AgentID: "a1", Target: "127.0.0.1:8080", Enabled: true}
	if err := db.SaveDomainBinding(b); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if b.ID == 0 {
		t.Fatal("Save should assign ID")
	}

	got, err := db.GetDomainBinding("blog.example.com")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AgentID != "a1" || got.Target != "127.0.0.1:8080" {
		t.Errorf("got = %+v", got)
	}

	// upsert：同域名再存 → 更新字段，保留 apply 快照
	if err := db.UpdateDomainBindingApply("blog.example.com", "ok", "", 123); err != nil {
		t.Fatalf("UpdateApply: %v", err)
	}
	b2 := &DomainBinding{Domain: "blog.example.com", AgentID: "a1", Target: "docker://web:8080", Enabled: true}
	if err := db.SaveDomainBinding(b2); err != nil {
		t.Fatalf("Save upsert: %v", err)
	}
	got, _ = db.GetDomainBinding("blog.example.com")
	if got.Target != "docker://web:8080" {
		t.Errorf("upsert target = %q", got.Target)
	}
	if got.LastApplyStatus != "ok" || got.AppliedAt != 123 {
		t.Errorf("apply snapshot not preserved: %+v", got)
	}
	if got.ID != b.ID {
		t.Errorf("upsert should keep ID %d, got %d", b.ID, got.ID)
	}

	// 列表与按 agent 过滤
	if err := db.SaveDomainBinding(&DomainBinding{Domain: "api.example.com", AgentID: "a2", Target: "10.0.0.2:9000"}); err != nil {
		t.Fatalf("Save a2: %v", err)
	}
	all, err := db.ListDomainBindings()
	if err != nil || len(all) != 2 {
		t.Fatalf("List = %v, %v", all, err)
	}
	only, err := db.ListDomainBindingsByAgent("a2")
	if err != nil || len(only) != 1 || only[0].Domain != "api.example.com" {
		t.Fatalf("ListByAgent = %+v, %v", only, err)
	}

	// 删除；再删不存在的行是 no-op（GORM 惯例，与 DDNSConfig.Delete 一致）
	if err := db.DeleteDomainBinding("blog.example.com"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := db.GetDomainBinding("blog.example.com"); err == nil {
		t.Error("deleted binding should not be found")
	}
	if err := db.DeleteDomainBinding("blog.example.com"); err != nil {
		t.Errorf("delete nonexistent should be no-op, got %v", err)
	}
}

func TestUpdateDomainAgentID(t *testing.T) {
	db := testBindingDB(t)

	aid := "a1"
	if err := db.UpsertDomain(&Domain{ID: "blog.example.com", Domain: "blog.example.com", Status: "pending", AgentID: &aid}); err != nil {
		t.Fatalf("UpsertDomain: %v", err)
	}
	// Assign(struct) 对已存在记录不落库（见 UpdateDomainAgentID 注释），
	// 换绑必须走显式 Update
	if err := db.UpdateDomainAgentID("blog.example.com", "a2"); err != nil {
		t.Fatalf("UpdateDomainAgentID: %v", err)
	}
	got, err := db.GetDomainByName("blog.example.com")
	if err != nil || got.AgentID == nil || *got.AgentID != "a2" {
		t.Fatalf("after rebind = %+v (%v), want agentID a2", got, err)
	}

	// closed db：错误分支
	db.Close()
	if err := db.UpdateDomainAgentID("blog.example.com", "a3"); err == nil {
		t.Error("UpdateDomainAgentID on closed db should fail")
	}
}

func TestDomainBindingListClosedDB(t *testing.T) {
	db := testBindingDB(t)
	if err := db.SaveDomainBinding(&DomainBinding{Domain: "x.example.com", AgentID: "a1", Target: "127.0.0.1:80"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	db.Close()
	if _, err := db.ListDomainBindings(); err == nil {
		t.Error("ListDomainBindings on closed db should fail")
	}
	if _, err := db.ListDomainBindingsByAgent("a1"); err == nil {
		t.Error("ListDomainBindingsByAgent on closed db should fail")
	}
}
