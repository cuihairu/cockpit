package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ 纯函数 ============

func TestValidBackupSchedule(t *testing.T) {
	valid := []string{"manual", "daily@00:00", "daily@03:30", "daily@23:59", "every:1h", "every:6h", "every:168h"}
	for _, s := range valid {
		if !ValidBackupSchedule(s) {
			t.Errorf("ValidBackupSchedule(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "hourly", "daily", "daily@24:00", "daily@12:60", "daily@3", "every:0h", "every:169h", "every:h", "weekly@mon"}
	for _, s := range invalid {
		if ValidBackupSchedule(s) {
			t.Errorf("ValidBackupSchedule(%q) = true, want false", s)
		}
	}
}

func TestNextBackupRunAt(t *testing.T) {
	now := time.Date(2026, 9, 14, 15, 0, 0, 0, time.Local)

	if got := NextBackupRunAt("manual", now); got != 0 {
		t.Errorf("manual next = %d, want 0", got)
	}
	// 当天时间已过 → 明天
	got := time.Unix(NextBackupRunAt("daily@03:00", now), 0)
	if got.Day() != 15 || got.Hour() != 3 || got.Minute() != 0 {
		t.Errorf("daily@03:00 next = %v, want Sep 15 03:00", got)
	}
	// 当天时间未到 → 今天
	got = time.Unix(NextBackupRunAt("daily@20:00", now), 0)
	if got.Day() != 14 || got.Hour() != 20 {
		t.Errorf("daily@20:00 next = %v, want Sep 14 20:00", got)
	}
	// every:Nh
	got = time.Unix(NextBackupRunAt("every:6h", now), 0)
	if got.Sub(now) != 6*time.Hour {
		t.Errorf("every:6h next = %v, want +6h", got)
	}
}

func TestParseBackupSources(t *testing.T) {
	if got := ParseBackupSources(""); len(got) != 0 {
		t.Errorf("empty = %v, want empty", got)
	}
	got := ParseBackupSources(`["/etc/nginx","/var/lib/db"]`)
	if len(got) != 2 || got[0] != "/etc/nginx" {
		t.Errorf("got = %v", got)
	}
}

func TestValidBackupSources(t *testing.T) {
	if ValidBackupSources(nil) || ValidBackupSources([]string{}) {
		t.Error("empty sources should be invalid")
	}
	if !ValidBackupSources([]string{"/a"}) {
		t.Error("absolute source should be valid")
	}
	if ValidBackupSources([]string{"/a", "relative"}) {
		t.Error("mixed relative source should be invalid")
	}
}

// ============ API ============

func newBackupTestServer(t *testing.T) *Server {
	t.Helper()
	db := testServerDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Server{
		registry: NewRegistry(),
		db:       db,
		audit:    audit.NewLogger(db),
		notifier: notification.NewService(nil),
		ctx:      ctx,
	}
}

func newBackupCfg(agentID, name, schedule string, enabled bool) *storage.BackupConfig {
	return &storage.BackupConfig{
		AgentID: agentID, Name: name,
		Sources: `["/etc/nginx"]`, DestDir: "/mnt/bak",
		Schedule: schedule, Retention: 7, Enabled: enabled,
	}
}

// withFakeBackupAgent 注册一个应答 backup.* RPC 的假 agent。
// handler 拿到 method/params，返回 (data, rpcError)。
func withFakeBackupAgent(t *testing.T, s *Server, agentID string, handler func(method string, params map[string]interface{}) (interface{}, string)) {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Capabilities = []protocol.Capability{{Type: "backup"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	go func() {
		for reqMsg := range agent.Send {
			if reqMsg.Type != protocol.MessageTypeRPCRequest {
				continue
			}
			method, _ := reqMsg.Payload["method"].(string)
			params, _ := reqMsg.Payload["params"].(map[string]interface{})
			var data interface{}
			var rpcErr string
			if handler != nil {
				data, rpcErr = handler(method, params)
			}
			payload := map[string]interface{}{"status": "success", "data": data}
			if rpcErr != "" {
				payload = map[string]interface{}{"status": "error", "error": rpcErr}
			}
			resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, payload)
			resp.ID = reqMsg.ID
			s.handleRPCResponse(resp)
		}
	}()
	t.Cleanup(func() { close(agent.Send) })
}

func validBackupReq() string {
	return `{"agent_id":"a1","name":"etc","sources":["/etc/nginx"],"dest_dir":"/mnt/bak","schedule":"daily@03:00","retention":7}`
}

func TestBackupConfigCreateListUpdateDelete(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)

	// 创建
	req := httptest.NewRequest(http.MethodPost, "/api/backups/configs", strings.NewReader(validBackupReq()))
	rec := httptest.NewRecorder()
	s.handleBackupConfigCreate(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var created backupConfigView
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == 0 || created.Name != "etc" || len(created.Sources) != 1 || created.NextRunAt == 0 {
		t.Fatalf("created = %+v", created)
	}
	if created.Sources[0] != "/etc/nginx" {
		t.Errorf("sources echo = %v", created.Sources)
	}

	// 列表
	rec = httptest.NewRecorder()
	s.handleBackupConfigsList(rec, httptest.NewRequest(http.MethodGet, "/api/backups/configs", nil))
	var list struct {
		Configs []backupConfigView `json:"configs"`
	}
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Configs) != 1 || list.Configs[0].Name != "etc" {
		t.Fatalf("list = %+v", list)
	}

	// 更新
	update := `{"agent_id":"a1","name":"etc","sources":["/etc/nginx","/var/lib/db"],"dest_dir":"/mnt/bak","schedule":"every:6h","retention":3,"enabled":false}`
	rec = httptest.NewRecorder()
	s.handleBackupConfigUpdate(rec, httptest.NewRequest(http.MethodPut, "/api/backups/configs/1", strings.NewReader(update)), 1)
	if rec.Code != http.StatusOK {
		t.Fatalf("update code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var updated backupConfigView
	json.Unmarshal(rec.Body.Bytes(), &updated)
	if len(updated.Sources) != 2 || updated.Enabled || updated.Retention != 3 {
		t.Errorf("updated = %+v", updated)
	}

	// 删除
	rec = httptest.NewRecorder()
	s.handleBackupConfigDelete(rec, httptest.NewRequest(http.MethodDelete, "/api/backups/configs/1", nil), 1)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete code = %d", rec.Code)
	}
	if _, err := s.db.GetBackupConfig(1); err == nil {
		t.Error("config should be gone after delete")
	}
}

func TestBackupConfigCreateValidation(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)

	cases := []struct {
		desc, body string
	}{
		{"bad schedule", `{"agent_id":"a1","name":"ok","sources":["/a"],"dest_dir":"/b","schedule":"hourly"}`},
		{"bad name", `{"agent_id":"a1","name":"Bad Name","sources":["/a"],"dest_dir":"/b","schedule":"manual"}`},
		{"relative source", `{"agent_id":"a1","name":"ok","sources":["etc/passwd"],"dest_dir":"/b","schedule":"manual"}`},
		{"relative dest", `{"agent_id":"a1","name":"ok","sources":["/a"],"dest_dir":"b","schedule":"manual"}`},
		{"empty sources", `{"agent_id":"a1","name":"ok","sources":[],"dest_dir":"/b","schedule":"manual"}`},
		{"retention over", `{"agent_id":"a1","name":"ok","sources":["/a"],"dest_dir":"/b","schedule":"manual","retention":400}`},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		s.handleBackupConfigCreate(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs", strings.NewReader(c.body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, want 400 (body: %s)", c.desc, rec.Code, rec.Body.String())
		}
	}

	// agent 不存在
	rec := httptest.NewRecorder()
	s.handleBackupConfigCreate(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs",
		strings.NewReader(`{"agent_id":"ghost","name":"ok","sources":["/a"],"dest_dir":"/b","schedule":"manual"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown agent: code = %d, want 400", rec.Code)
	}
}

func TestBackupRunDispatchesAndTracks(t *testing.T) {
	s := newBackupTestServer(t)
	var gotTaskGet int
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		switch method {
		case "backup.run":
			if params["name"] != "etc" {
				return nil, "unexpected name"
			}
			return map[string]interface{}{"taskId": "bt-1", "status": "started"}, ""
		case "backup.task.get":
			gotTaskGet++
			if gotTaskGet >= 2 { // 第二次轮询即终态
				return map[string]interface{}{
					"taskId": "bt-1", "status": "success",
					"file": "etc-20260914-030000.tar.gz", "size": float64(4096),
				}, ""
			}
			return map[string]interface{}{"taskId": "bt-1", "status": "running"}, ""
		default:
			return nil, "unexpected method " + method
		}
	})

	cfg := newBackupCfg("a1", "etc", "daily@03:00", true)
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// 手动运行
	rec := httptest.NewRecorder()
	s.handleBackupRun(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs/1/run", nil), cfg)
	if rec.Code != http.StatusOK {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}

	// 等待 track goroutine 回填终态
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if run, err := s.db.GetBackupRun(1); err == nil && run.Status != "running" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	run, err := s.db.GetBackupRun(1)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != "success" || run.File != "etc-20260914-030000.tar.gz" || run.Size != 4096 {
		t.Errorf("run = %+v", run)
	}
	if run.TaskID != "bt-1" {
		t.Errorf("taskId = %q, want bt-1", run.TaskID)
	}
	got, _ := s.db.GetBackupConfig(cfg.ID)
	if got.LastStatus != "success" {
		t.Errorf("config last status = %q", got.LastStatus)
	}

	// 运行历史
	rec = httptest.NewRecorder()
	s.handleBackupConfigRuns(rec, httptest.NewRequest(http.MethodGet, "/api/backups/configs/1/runs", nil), got)
	var list struct {
		Runs []*struct {
			Status string `json:"status"`
		} `json:"runs"`
	}
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Runs) != 1 || list.Runs[0].Status != "success" {
		t.Errorf("runs = %+v", list)
	}
}

func TestBackupRunConflictsWhileRunning(t *testing.T) {
	s := newBackupTestServer(t)
	cfg := newBackupCfg("a1", "busy", "manual", true)
	cfg.LastStatus = "running"
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.handleBackupRun(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs/1/run", nil), cfg)
	if rec.Code != http.StatusConflict {
		t.Errorf("code = %d, want 409", rec.Code)
	}
}

func TestBackupRunAgentOffline(t *testing.T) {
	s := newBackupTestServer(t)
	cfg := newBackupCfg("ghost-agent", "off", "manual", true)
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.handleBackupRun(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs/1/run", nil), cfg)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503", rec.Code)
	}
}

func TestBackupFilesDeleteRejectsBadName(t *testing.T) {
	s := newBackupTestServer(t)
	cfg := newBackupCfg("ghost-agent", "off", "manual", true)
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.handleBackupFileDelete(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs/1/files/delete",
		strings.NewReader(`{"name":"../../etc/passwd"}`)), cfg)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

// ============ 孤儿恢复与调度 ============

func TestRecoverOrphanBackupRuns(t *testing.T) {
	s := newBackupTestServer(t)
	cfg := newBackupCfg("a1", "orphan", "daily@03:00", true)
	cfg.LastStatus = "running"
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	run := &storage.BackupRun{ConfigID: cfg.ID, TaskID: "bt-x", Status: "running", StartedAt: 100}
	if err := s.db.CreateBackupRun(run); err != nil {
		t.Fatal(err)
	}

	s.recoverOrphanBackupRuns()

	gotRun, err := s.db.GetBackupRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotRun.Status != "failed" || gotRun.FinishedAt == 0 {
		t.Errorf("orphan run = %+v, want failed with finishedAt", gotRun)
	}
	gotCfg, _ := s.db.GetBackupConfig(cfg.ID)
	if gotCfg.LastStatus != "failed" {
		t.Errorf("config last status = %q, want failed", gotCfg.LastStatus)
	}

	// 再次执行应无副作用
	s.recoverOrphanBackupRuns()
}

func TestDispatchDueBackupsAdvancesNextRun(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method == "backup.run" {
			return map[string]interface{}{"taskId": "bt-due", "status": "started"}, ""
		}
		// task.get 永远 running，track goroutine 靠超时收尾（测试结束即取消）
		return map[string]interface{}{"taskId": "bt-due", "status": "running"}, ""
	})

	cfg := newBackupCfg("a1", "due", "every:24h", true)
	cfg.NextRunAt = time.Now().Add(-time.Minute).Unix() // 已到期
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}

	s.dispatchDueBackups()

	got, _ := s.db.GetBackupConfig(cfg.ID)
	if got.NextRunAt <= time.Now().Unix() {
		t.Errorf("next_run_at = %d, should be advanced into the future", got.NextRunAt)
	}
	if got.LastStatus != "running" {
		t.Errorf("last status = %q, want running", got.LastStatus)
	}
	// 再跑一次调度不应重复下发（NextRunAt 已推进 + running 防并发）
	s.dispatchDueBackups()
	runs, _ := s.db.ListBackupRuns(cfg.ID, 10)
	if len(runs) != 1 {
		t.Errorf("runs = %d, want 1 (no double dispatch)", len(runs))
	}
}
