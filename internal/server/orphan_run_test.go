package server

import (
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

// TestRecoverOrphanRuns 重启孤儿清扫全链：running/pending Job 与 running run
// 收敛 failed，终态行（success/cancelled/failed run）原样保留；步骤 Job
// （带 WorkflowRunID）同被扫到。
func TestRecoverOrphanRuns(t *testing.T) {
	db := testServerDB(t)
	s := &Server{db: db}

	now := time.Now()
	seed := []*storage.Job{
		{ID: "j-run", Type: "agent.exec", Target: "a", Status: storage.JobStatusRunning, CreatedAt: now},
		{ID: "j-pend", Type: "agent.exec", Target: "a", Status: storage.JobStatusPending, CreatedAt: now},
		// 步骤 Job：挂在 run 名下，同样无推进者
		{ID: "j-step", Type: "agent.exec", Target: "b", Status: storage.JobStatusRunning,
			WorkflowRunID: "r-live", CreatedAt: now},
		// 终态行：清扫不得触碰
		{ID: "j-done", Type: "agent.exec", Target: "a", Status: storage.JobStatusSuccess, CreatedAt: now},
		{ID: "j-cancel", Type: "agent.exec", Target: "a", Status: storage.JobStatusCancelled, CreatedAt: now},
	}
	for _, j := range seed {
		if err := db.CreateJob(j); err != nil {
			t.Fatalf("CreateJob %s: %v", j.ID, err)
		}
	}
	runs := []*storage.WorkflowRun{
		{ID: "r-live", WorkflowID: "wf-a", Status: storage.WorkflowRunStatusRunning, CreatedAt: now},
		// 终态 run：清扫不得触碰
		{ID: "r-done", WorkflowID: "wf-a", Status: storage.WorkflowRunStatusSuccess, CreatedAt: now},
	}
	for _, r := range runs {
		if err := db.CreateWorkflowRun(r); err != nil {
			t.Fatalf("CreateWorkflowRun %s: %v", r.ID, err)
		}
	}

	s.recoverOrphanRuns()

	for _, id := range []string{"j-run", "j-pend", "j-step"} {
		job, err := db.GetJob(id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status != storage.JobStatusFailed || job.Error != "server restarted during execution" {
			t.Fatalf("%s = %+v, want failed with restart error", id, job)
		}
		if job.FinishedAt == nil {
			t.Fatalf("%s finishedAt not set", id)
		}
	}
	for _, id := range []string{"j-done", "j-cancel"} {
		job, _ := db.GetJob(id)
		if job.Error != "" || job.FinishedAt != nil {
			t.Fatalf("%s touched by sweep: %+v", id, job)
		}
	}

	run, err := db.GetWorkflowRun("r-live")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != storage.WorkflowRunStatusFailed || run.FinishedAt == nil {
		t.Fatalf("r-live = %+v, want failed with finishedAt", run)
	}
	done, _ := db.GetWorkflowRun("r-done")
	if done.Status != storage.WorkflowRunStatusSuccess || done.FinishedAt != nil {
		t.Fatalf("r-done touched by sweep: %+v", done)
	}
}

// TestRecoverOrphanRunsEmpty 无遗留时 no-op（不报错）。
func TestRecoverOrphanRunsEmpty(t *testing.T) {
	db := testServerDB(t)
	s := &Server{db: db}
	s.recoverOrphanRuns() // 空库不 panic、不报错
}

// TestRecoverOrphanRunsDBClosed 闭库确定性构造两处查询失败分支：Job 面失败
// 只记日志继续，run 面照常走完（两面独立，互不拖住）。
func TestRecoverOrphanRunsDBClosed(t *testing.T) {
	db := testServerDB(t)
	s := &Server{db: db}
	db.Close()
	s.recoverOrphanRuns() // 不 panic、不报错
}
