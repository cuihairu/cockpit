package storage

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// TestS4CreateWorkflowRunConflict 验证 S4：同一 workflow 已有 running 实例时，
// CreateWorkflowRun 必须原子拒绝（返回 ErrWorkflowRunConflict），不插入第二份。
func TestS4CreateWorkflowRunConflict(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	run1 := &WorkflowRun{ID: "s4-r1", WorkflowID: "s4-wf", WorkflowName: "n",
		Status: WorkflowRunStatusRunning, CreatedAt: time.Now()}
	if err := db.CreateWorkflowRun(run1); err != nil {
		t.Fatalf("create first: %v", err)
	}

	run2 := &WorkflowRun{ID: "s4-r2", WorkflowID: "s4-wf", WorkflowName: "n",
		Status: WorkflowRunStatusRunning, CreatedAt: time.Now()}
	err := db.CreateWorkflowRun(run2)
	if !errors.Is(err, ErrWorkflowRunConflict) {
		t.Fatalf("create second: err = %v, want ErrWorkflowRunConflict", err)
	}

	// 终态后允许再建。
	if _, err := db.FinishWorkflowRun("s4-r1", WorkflowRunStatusSuccess, time.Now()); err != nil {
		t.Fatalf("finish: %v", err)
	}
	run3 := &WorkflowRun{ID: "s4-r3", WorkflowID: "s4-wf", WorkflowName: "n",
		Status: WorkflowRunStatusRunning, CreatedAt: time.Now()}
	if err := db.CreateWorkflowRun(run3); err != nil {
		t.Fatalf("create after finish: %v", err)
	}
}

// TestS4CreateWorkflowRunConcurrent 验证 S4：并发创建时仅一个成功（原子
// check-then-act，无双跑）。
func TestS4CreateWorkflowRunConcurrent(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			run := &WorkflowRun{ID: "s4-c" + string(rune('a'+i)), WorkflowID: "s4-wf2",
				WorkflowName: "n", Status: WorkflowRunStatusRunning, CreatedAt: time.Now()}
			errs[i] = db.CreateWorkflowRun(run)
		}(i)
	}
	wg.Wait()

	success, conflict := 0, 0
	for _, err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, ErrWorkflowRunConflict) {
			conflict++
		} else {
			t.Fatalf("unexpected err: %v", err)
		}
	}
	if success != 1 || conflict != n-1 {
		t.Fatalf("success=%d conflict=%d, want 1/%d", success, conflict, n-1)
	}
}
