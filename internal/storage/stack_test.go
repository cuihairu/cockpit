package storage

import (
	"testing"
)

func TestStackUpsertAndList(t *testing.T) {
	db := testDB(t)

	st := &Stack{AgentID: "a1", Name: "web", Running: 2, Total: 3, LastAction: "up", LastStatus: "success", LastDeployedAt: 1700000000}
	if err := db.UpsertStack(st); err != nil {
		t.Fatalf("upsert error = %v", err)
	}
	if st.ID == 0 {
		t.Fatal("ID should be assigned")
	}

	// 再次 upsert 同一 (agent, name) 应更新而非新增
	st2 := &Stack{AgentID: "a1", Name: "web", Running: 3, Total: 3, LastAction: "down", LastStatus: "failed", LastDeployedAt: 1700000001}
	if err := db.UpsertStack(st2); err != nil {
		t.Fatalf("second upsert error = %v", err)
	}

	all, err := db.ListStacks()
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("list count = %d, want 1", len(all))
	}
	if all[0].Running != 3 || all[0].LastAction != "down" || all[0].LastStatus != "failed" {
		t.Errorf("stack = %+v", all[0])
	}

	// 不同 name 各一行
	if err := db.UpsertStack(&Stack{AgentID: "a1", Name: "db", Running: 1, Total: 1}); err != nil {
		t.Fatalf("upsert db error = %v", err)
	}
	byAgent, err := db.ListStacksByAgent("a1")
	if err != nil || len(byAgent) != 2 {
		t.Fatalf("byAgent = %v, %v", byAgent, err)
	}

	if err := db.DeleteStacksByAgent("a1"); err != nil {
		t.Fatalf("delete error = %v", err)
	}
	all, _ = db.ListStacks()
	if len(all) != 0 {
		t.Errorf("after delete count = %d, want 0", len(all))
	}
}
