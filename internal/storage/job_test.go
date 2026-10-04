package storage

import (
	"strings"
	"testing"
	"time"
)

func TestJobCRUD(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now()
	j := &Job{
		ID:         "job-1",
		Type:       "agent.exec",
		Target:     "agent-a",
		Actor:      "cui",
		Parameters: `{"command":"uptime"}`,
		Status:     JobStatusPending,
		CreatedAt:  now,
	}
	if err := db.CreateJob(j); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	got, err := db.GetJob("job-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Type != "agent.exec" || got.Target != "agent-a" || got.Status != JobStatusPending {
		t.Fatalf("GetJob = %+v", got)
	}
	if !strings.Contains(got.Parameters, "uptime") {
		t.Fatalf("parameters not persisted: %q", got.Parameters)
	}

	// 转换为 running 回写
	got.Status = JobStatusRunning
	if err := db.UpdateJob(got); err != nil {
		t.Fatalf("UpdateJob(running): %v", err)
	}
	got2, _ := db.GetJob("job-1")
	if got2.Status != JobStatusRunning {
		t.Fatalf("status after update = %q", got2.Status)
	}

	// 终态回写（含输出/退出码/错误/完成时间）
	finished := time.Now()
	got2.Status = JobStatusSuccess
	got2.ExitCode = 0
	got2.Output = "ok"
	got2.FinishedAt = &finished
	if err := db.UpdateJob(got2); err != nil {
		t.Fatalf("UpdateJob(finished): %v", err)
	}
	got3, _ := db.GetJob("job-1")
	if got3.Status != JobStatusSuccess || got3.ExitCode != 0 || got3.FinishedAt == nil {
		t.Fatalf("final job = %+v", got3)
	}

	// 不存在的 ID
	if _, err := db.GetJob("nope"); err == nil {
		t.Fatalf("GetJob(nope) should error")
	}
}

func TestListJobsOrderAndLimit(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	for i := 1; i <= 3; i++ {
		ts := time.Now().Add(time.Duration(i) * time.Minute)
		if err := db.CreateJob(&Job{
			ID:        "job-" + string(rune('a'+i-1)),
			Type:      "agent.exec",
			Target:    "agent-a",
			Actor:     "cui",
			Status:    JobStatusPending,
			CreatedAt: ts,
		}); err != nil {
			t.Fatalf("CreateJob %d: %v", i, err)
		}
	}

	jobs, err := db.ListJobs(0)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	// 新在前：job-c > job-b > job-a
	if len(jobs) < 3 || jobs[0].ID != "job-c" || jobs[2].ID != "job-a" {
		got := make([]string, 0, len(jobs))
		for i := range jobs {
			got = append(got, jobs[i].ID)
		}
		t.Fatalf("order = %v", got)
	}

	limited, err := db.ListJobs(2)
	if err != nil {
		t.Fatalf("ListJobs(2): %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("limit not applied: %d", len(limited))
	}
}