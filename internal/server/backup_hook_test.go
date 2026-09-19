package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// ============ M3 数据库热备钩子（pre-hook，见 backup-design.md D26-D32）============

const backupPreHook = `sqlite3 /data/app.db ".backup /tmp/app.db.bak"`

func TestBackupConfigPreHookRoundtrip(t *testing.T) {
	s := newBackupTestServer(t)
	var dispatchedPreHook interface{}
	withFakeBackupAgentCap(t, s, "a1",
		[]protocol.Capability{{Type: "backup"}}, func(method string, params map[string]interface{}) (interface{}, string) {
			if method == "backup.run" {
				dispatchedPreHook = params["preHook"]
				return map[string]interface{}{"taskId": "bt-h", "status": "started"}, ""
			}
			// task.get 永远 running，靠测试结束取消 track
			return map[string]interface{}{"taskId": "bt-h", "status": "running"}, ""
		})

	// 创建带 pre_hook
	bodyB, _ := json.Marshal(backupConfigRequest{
		AgentID: "a1", Name: "appdb", Sources: []string{"/tmp/app.db.bak"},
		DestDir: "/mnt/bak", Schedule: "manual", PreHook: backupPreHook,
	})
	rec := httptest.NewRecorder()
	s.handleBackupConfigCreate(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs", bytes.NewReader(bodyB)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created backupConfigView
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.PreHook != backupPreHook {
		t.Fatalf("created.PreHook = %q", created.PreHook)
	}

	// 立即运行 → 下发参数透传 preHook（D32）
	got, _ := s.db.GetBackupConfig(created.ID)
	rec = httptest.NewRecorder()
	s.handleBackupRun(rec, httptest.NewRequest(http.MethodPost, "/r", nil), got)
	if rec.Code != http.StatusOK {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}
	if dispatchedPreHook != backupPreHook {
		t.Fatalf("dispatched preHook = %v, want %q", dispatchedPreHook, backupPreHook)
	}

	// 更新清空 pre_hook（先解除 running 态：track goroutine 被 fake agent 挂在 running）
	got.LastStatus = "success"
	if err := s.db.UpdateBackupConfig(got); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	s.handleBackupConfigUpdate(rec, httptest.NewRequest(http.MethodPut, "/api/backups/configs/1",
		strings.NewReader(`{"agent_id":"a1","name":"appdb","sources":["/tmp/app.db.bak"],"dest_dir":"/mnt/bak","schedule":"manual","pre_hook":""}`)), created.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body.String())
	}
	gotCfg, _ := s.db.GetBackupConfig(created.ID)
	if gotCfg.PreHook != "" {
		t.Errorf("stored PreHook = %q, want empty", gotCfg.PreHook)
	}
}

func TestBackupConfigPreHookTooLong(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgentCap(t, s, "a1", []protocol.Capability{{Type: "backup"}}, nil)

	req := backupConfigRequest{
		AgentID: "a1", Name: "appdb", Sources: []string{"/a"},
		DestDir: "/b", Schedule: "manual",
		PreHook: strings.Repeat("x", 1025),
	}
	if msg, ok := req.validate(); ok {
		t.Errorf("validate passed, want rejection (msg=%q)", msg)
	}
	// HTTP 层同样 400
	body := `{"agent_id":"a1","name":"appdb","sources":["/a"],"dest_dir":"/b","schedule":"manual","pre_hook":"` + strings.Repeat("x", 1025) + `"}`
	rec := httptest.NewRecorder()
	s.handleBackupConfigCreate(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("create code = %d, want 400", rec.Code)
	}
}
