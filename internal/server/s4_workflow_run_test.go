package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

// TestS4WorkflowRunStorageConflictMaps409 验证 S4：当 ActiveWorkflowRun 预检
// 与 CreateWorkflowRun 之间出现并发 running run（或预检窗口错过）时，
// 存储层原子冲突必须映射 409，不得双跑。
func TestS4WorkflowRunStorageConflictMaps409(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})

	created := wfCreate(t, s, wfStepsTwo)

	// 预置一个 running run。为了稳定命中 CreateWorkflowRun 的冲突分支，
	// 先手动放入（绕过 handler 的 ActiveWorkflowRun 预检）。
	existing := &storage.WorkflowRun{
		ID:           "s4-active",
		WorkflowID:   created.ID,
		WorkflowName: created.Name,
		Status:       storage.WorkflowRunStatusRunning,
		Actor:        "test",
		CreatedAt:    time.Now(),
	}
	if err := s.db.CreateWorkflowRun(existing); err != nil {
		t.Fatalf("seed running run: %v", err)
	}

	rec := httptest.NewRecorder()
	s.handleWorkflowAPI(rec, httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", nil),
		"/workflows/"+created.ID+"/run")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (storage conflict), body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] != "workflow is already running" {
		t.Fatalf("error = %v, want workflow already running", resp["error"])
	}
}
