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

func TestStackDeploymentLifecycle(t *testing.T) {
	db := testDB(t)

	rec := &StackDeployment{AgentID: "a1", StackName: "web", Action: "up", Status: "running", TaskID: "t1", StartedAt: 100}
	if err := db.CreateStackDeployment(rec); err != nil {
		t.Fatalf("create error = %v", err)
	}
	if rec.ID == 0 {
		t.Fatal("ID should be assigned")
	}

	// 回填终态
	if err := db.FinishStackDeployment("t1", "success", 200); err != nil {
		t.Fatalf("finish error = %v", err)
	}

	// 再插两条：一条 failed、一条别的 stack
	if err := db.CreateStackDeployment(&StackDeployment{AgentID: "a1", StackName: "web", Action: "down", Status: "running", TaskID: "t2", StartedAt: 300}); err != nil {
		t.Fatalf("create t2 error = %v", err)
	}
	if err := db.FinishStackDeployment("t2", "failed", 400); err != nil {
		t.Fatalf("finish t2 error = %v", err)
	}
	if err := db.CreateStackDeployment(&StackDeployment{AgentID: "a1", StackName: "other", Action: "up", Status: "running", TaskID: "t3", StartedAt: 500}); err != nil {
		t.Fatalf("create t3 error = %v", err)
	}

	list, err := db.ListStackDeployments("a1", "web", 10)
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list count = %d, want 2", len(list))
	}
	// 倒序：t2 在前
	if list[0].TaskID != "t2" || list[0].Status != "failed" || list[0].FinishedAt != 400 {
		t.Errorf("list[0] = %+v", list[0])
	}
	if list[1].TaskID != "t1" || list[1].Status != "success" {
		t.Errorf("list[1] = %+v", list[1])
	}

	// limit 生效
	short, _ := db.ListStackDeployments("a1", "web", 1)
	if len(short) != 1 {
		t.Errorf("limit 1 got %d", len(short))
	}

	if err := db.DeleteStackDeploymentsByAgent("a1"); err != nil {
		t.Fatalf("delete error = %v", err)
	}
	list, _ = db.ListStackDeployments("a1", "web", 10)
	if len(list) != 0 {
		t.Errorf("after delete count = %d, want 0", len(list))
	}
}
