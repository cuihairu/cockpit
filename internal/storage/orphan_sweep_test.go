package storage

import (
	"testing"
	"time"
)

// TestFailJobConditional 条件 failed 回写：非终态命中（error/finished_at 落账），
// 已落终态的行不命中（恢复清扫绝不改写终态）。
func TestFailJobConditional(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now()
	for id, status := range map[string]string{
		"job-run":  JobStatusRunning,
		"job-pend": JobStatusPending,
		"job-done": JobStatusSuccess,
	} {
		if err := db.CreateJob(&Job{ID: id, Type: "agent.exec", Target: "a", Status: status, CreatedAt: now}); err != nil {
			t.Fatalf("CreateJob %s: %v", id, err)
		}
	}

	finished := time.Now()
	if n, err := db.FailJob("job-run", JobStatusRunning, finished, "server restarted"); err != nil || n != 1 {
		t.Fatalf("FailJob(running) = %d, %v; want 1 hit", n, err)
	}
	if n, err := db.FailJob("job-pend", JobStatusPending, finished, "server restarted"); err != nil || n != 1 {
		t.Fatalf("FailJob(pending) = %d, %v; want 1 hit", n, err)
	}
	// 终态行不命中：status 条件不匹配
	if n, err := db.FailJob("job-done", JobStatusRunning, finished, "server restarted"); err != nil || n != 0 {
		t.Fatalf("FailJob(success) = %d, %v; want 0 hit", n, err)
	}

	got, err := db.GetJob("job-run")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != JobStatusFailed || got.Error != "server restarted" || got.FinishedAt == nil {
		t.Fatalf("failed job = %+v", got)
	}
}

// TestRunningWorkflowRuns 只返回 running 的 run（含多定义混排时不过滤定义）。
func TestRunningWorkflowRuns(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now()
	runs := []*WorkflowRun{
		{ID: "r1", WorkflowID: "wf-a", Status: WorkflowRunStatusRunning, CreatedAt: now},
		{ID: "r2", WorkflowID: "wf-a", Status: WorkflowRunStatusFailed, CreatedAt: now},
		{ID: "r3", WorkflowID: "wf-b", Status: WorkflowRunStatusRunning, CreatedAt: now},
		{ID: "r4", WorkflowID: "wf-b", Status: WorkflowRunStatusSuccess, CreatedAt: now},
	}
	for _, r := range runs {
		if err := db.CreateWorkflowRun(r); err != nil {
			t.Fatalf("CreateWorkflowRun %s: %v", r.ID, err)
		}
	}

	got, err := db.RunningWorkflowRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("running runs = %d, want 2", len(got))
	}
	ids := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !ids["r1"] || !ids["r3"] {
		t.Fatalf("running runs = %+v, want r1/r3", got)
	}
}

// TestRunningWorkflowRunsDBClosed 闭库确定性构造查询失败分支。
func TestRunningWorkflowRunsDBClosed(t *testing.T) {
	db := testDB(t)
	db.Close()
	if _, err := db.RunningWorkflowRuns(); err == nil {
		t.Fatal("closed db: want error")
	}
}
