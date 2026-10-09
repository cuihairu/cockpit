package storage

import (
	"testing"
	"time"
)

// workflow_test.go Workflow/WorkflowRun 存储层（workflow-design.md W6）

func TestWorkflowCRUD(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	wf := &Workflow{ID: "wf-1", Name: "upgrade chain", Description: "d",
		Steps: `[{"type":"agent.exec","target":"a1","parameters":{"command":"uptime"}}]`,
		CreatedBy: "u", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.CreateWorkflow(wf); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := db.GetWorkflow("wf-1")
	if err != nil || got.Name != "upgrade chain" {
		t.Fatalf("get = %+v, %v", got, err)
	}

	wf2 := &Workflow{ID: "wf-2", Name: "b", CreatedAt: time.Now().Add(-time.Hour), UpdatedAt: time.Now()}
	if err := db.CreateWorkflow(wf2); err != nil {
		t.Fatalf("create2: %v", err)
	}
	list, err := db.ListWorkflows()
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %d, %v", len(list), err)
	}
	// 新在前（wf-1 创建时间晚）
	if list[0].ID != "wf-1" {
		t.Fatalf("order = %+v", list)
	}

	got.Name = "renamed"
	got.UpdatedAt = time.Now()
	if err := db.UpdateWorkflow(got); err != nil {
		t.Fatalf("update: %v", err)
	}
	again, _ := db.GetWorkflow("wf-1")
	if again.Name != "renamed" {
		t.Fatalf("update lost: %+v", again)
	}

	if err := db.DeleteWorkflow("wf-2"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := db.GetWorkflow("wf-2"); err == nil {
		t.Fatal("deleted workflow still readable")
	}
}

func TestWorkflowRunLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	run := &WorkflowRun{ID: "run-1", WorkflowID: "wf-1", WorkflowName: "n",
		Status: WorkflowRunStatusRunning, Actor: "u",
		Steps: `[{"name":"s1","status":"pending","attempts":0}]`, CreatedAt: time.Now()}
	if err := db.CreateWorkflowRun(run); err != nil {
		t.Fatalf("create: %v", err)
	}

	// active 检查：running 可见
	active, err := db.ActiveWorkflowRun("wf-1")
	if err != nil || active == nil || active.ID != "run-1" {
		t.Fatalf("active = %+v, %v", active, err)
	}

	// 落终态后 active 消失
	run.Status = WorkflowRunStatusSuccess
	fin := time.Now()
	run.FinishedAt = &fin
	if err := db.UpdateWorkflowRun(run); err != nil {
		t.Fatalf("update: %v", err)
	}
	active, _ = db.ActiveWorkflowRun("wf-1")
	if active != nil {
		t.Fatalf("terminal run still active: %+v", active)
	}

	// 台账（新在前）
	run2 := &WorkflowRun{ID: "run-2", WorkflowID: "wf-1", WorkflowName: "n",
		Status: WorkflowRunStatusRunning, CreatedAt: time.Now().Add(time.Hour)}
	if err := db.CreateWorkflowRun(run2); err != nil {
		t.Fatalf("create2: %v", err)
	}
	runs, err := db.ListWorkflowRuns("wf-1")
	if err != nil || len(runs) != 2 || runs[0].ID != "run-2" {
		t.Fatalf("runs = %+v, %v", runs, err)
	}
	if _, err := db.GetWorkflowRun("run-1"); err != nil {
		t.Fatalf("get run: %v", err)
	}
	// 空台账
	empty, err := db.ListWorkflowRuns("wf-none")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty = %+v, %v", empty, err)
	}
}

func TestListJobsFiltered(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now()
	seed := []Job{
		{ID: "j1", Type: "agent.exec", Target: "a1", Status: JobStatusSuccess, WorkflowRunID: "r1", CreatedAt: now},
		{ID: "j2", Type: "agent.exec", Target: "a2", Status: JobStatusFailed, CreatedAt: now.Add(time.Second)},
		{ID: "j3", Type: "agent.exec", Target: "a1", Status: JobStatusPending, WorkflowRunID: "r1", CreatedAt: now.Add(2 * time.Second)},
	}
	for i := range seed {
		if err := db.CreateJob(&seed[i]); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	if got, _ := db.ListJobsFiltered("pending", "", "", "", 0); len(got) != 1 || got[0].ID != "j3" {
		t.Fatalf("status filter = %+v", got)
	}
	if got, _ := db.ListJobsFiltered("", "a1", "", "", 0); len(got) != 2 {
		t.Fatalf("target filter = %+v", got)
	}
	if got, _ := db.ListJobsFiltered("", "", "agent.exec", "", 0); len(got) != 3 {
		t.Fatalf("type filter = %+v", got)
	}
	if got, _ := db.ListJobsFiltered("", "", "", "r1", 0); len(got) != 2 {
		t.Fatalf("run filter = %+v", got)
	}
	if got, _ := db.ListJobsFiltered("", "a1", "", "r1", 0); len(got) != 2 {
		t.Fatalf("combined filter = %+v", got)
	}
	// limit 生效
	if got, _ := db.ListJobsFiltered("", "", "", "", 2); len(got) != 2 {
		t.Fatalf("limit = %+v", got)
	}
	// 无命中
	if got, _ := db.ListJobsFiltered("cancelled", "", "", "", 0); len(got) != 0 {
		t.Fatalf("no match = %+v", got)
	}
}

func TestCancelJob(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if err := db.CreateJob(&Job{ID: "jc", Type: "agent.exec", Target: "a1",
		Status: JobStatusPending, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fin := time.Now()
	if err := db.CancelJob("jc", fin); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got, _ := db.GetJob("jc")
	if got.Status != JobStatusCancelled || got.FinishedAt == nil {
		t.Fatalf("cancelled = %+v", got)
	}
}

// TestWorkflowReadErrorBranches closed db 读错误分支（Find/First 查询失败
// 面唯一可确定性构造的形态，同 agent_expiry_test Pluck 口径）。
func TestWorkflowReadErrorBranches(t *testing.T) {
	db := testDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := db.ListWorkflows(); err == nil {
		t.Error("closed db ListWorkflows should fail")
	}
	if _, err := db.GetWorkflowRun("r1"); err == nil {
		t.Error("closed db GetWorkflowRun should fail")
	}
	if _, err := db.ListWorkflowRuns("wf-1"); err == nil {
		t.Error("closed db ListWorkflowRuns should fail")
	}
	if _, err := db.ActiveWorkflowRun("wf-1"); err == nil {
		t.Error("closed db ActiveWorkflowRun should fail")
	}
	if _, err := db.ListJobsFiltered("pending", "", "", "", 0); err == nil {
		t.Error("closed db ListJobsFiltered should fail")
	}
	if _, err := db.ListJobs(10); err == nil {
		t.Error("closed db ListJobs should fail")
	}
}
