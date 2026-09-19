package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// ============ M2 异地保留（agent 侧 rclone，见 backup-design.md D18-D25）============

func TestValidBackupRemoteDest(t *testing.T) {
	valid := []string{
		"my-s3:cockpit/backups",
		"a.b_c-d:x",
		"0bucket:/abs/path",
		"gdrive:照片", // path 段允许多字节 UTF-8
	}
	for _, s := range valid {
		if !ValidBackupRemoteDest(s) {
			t.Errorf("ValidBackupRemoteDest(%q) = false, want true", s)
		}
	}
	invalid := []string{
		"",             // 空 = 不启用，由调用方放行、此处判否
		"-flag:dest",   // D22：remote 名不以 - 开头（根除 flag 混淆）
		"no-colon",     // 缺 remote:path 形态
		":path",        // remote 名为空
		"ok:has space", // path 段禁空白
		"ok:tab\there", // path 段禁控制字符
		"ok:del\x7f",   // path 段禁 DEL
	}
	for _, s := range invalid {
		if ValidBackupRemoteDest(s) {
			t.Errorf("ValidBackupRemoteDest(%q) = true, want false", s)
		}
	}
}

// withFakeBackupAgentCap 同 withFakeBackupAgent，但 capability 列表由调用方给定
func withFakeBackupAgentCap(t *testing.T, s *Server, agentID string, caps []protocol.Capability, handler func(string, map[string]interface{}) (interface{}, string)) {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Capabilities = caps
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
	t.Cleanup(func() { agent.Close() })
}

const backupRemoteDest = "my-s3:cockpit/backups"

func TestBackupConfigRemoteDestRequiresRclone(t *testing.T) {
	s := newBackupTestServer(t)
	// D23：backup capability 无 rclone metadata → 配置期 400 拦截
	withFakeBackupAgentCap(t, s, "a1",
		[]protocol.Capability{{Type: "backup"}}, nil)

	body := `{"agent_id":"a1","name":"etc","sources":["/etc/nginx"],"dest_dir":"/mnt/bak","schedule":"manual","remote_dest":"my-s3:cockpit/backups"}`
	rec := httptest.NewRecorder()
	s.handleBackupConfigCreate(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create without rclone: code = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "rclone") {
		t.Errorf("error body = %s, want rclone hint", rec.Body.String())
	}
	if _, err := s.db.GetBackupConfig(1); err == nil {
		t.Error("config should not be created when rclone check fails")
	}

	// 不启用异地（无 remote_dest）不要求 rclone
	rec = httptest.NewRecorder()
	s.handleBackupConfigCreate(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs",
		strings.NewReader(`{"agent_id":"a1","name":"etc","sources":["/etc/nginx"],"dest_dir":"/mnt/bak","schedule":"manual"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create without remote_dest: code = %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestBackupConfigRemoteDestLifecycle(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgentCap(t, s, "a1",
		[]protocol.Capability{{Type: "backup", Metadata: map[string]interface{}{"rclone": true}}}, nil)

	// 创建带 remote_dest
	body := `{"agent_id":"a1","name":"etc","sources":["/etc/nginx"],"dest_dir":"/mnt/bak","schedule":"manual","remote_dest":"my-s3:cockpit/backups"}`
	rec := httptest.NewRecorder()
	s.handleBackupConfigCreate(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs", strings.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created backupConfigView
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.RemoteDest != backupRemoteDest {
		t.Fatalf("created.RemoteDest = %q, want %q", created.RemoteDest, backupRemoteDest)
	}

	// 非法 remote_dest 形态 → 400
	rec = httptest.NewRecorder()
	s.handleBackupConfigUpdate(rec, httptest.NewRequest(http.MethodPut, "/api/backups/configs/1",
		strings.NewReader(`{"agent_id":"a1","name":"etc","sources":["/etc/nginx"],"dest_dir":"/mnt/bak","schedule":"manual","remote_dest":"-flag:x"}`)), 1)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("update with bad remote_dest: code = %d, want 400", rec.Code)
	}

	// 清空 remote_dest（不需要 rclone 校验）→ 200 且落库
	rec = httptest.NewRecorder()
	s.handleBackupConfigUpdate(rec, httptest.NewRequest(http.MethodPut, "/api/backups/configs/1",
		strings.NewReader(`{"agent_id":"a1","name":"etc","sources":["/etc/nginx"],"dest_dir":"/mnt/bak","schedule":"manual","remote_dest":""}`)), 1)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear remote_dest: %d %s", rec.Code, rec.Body.String())
	}
	got, err := s.db.GetBackupConfig(1)
	if err != nil {
		t.Fatal(err)
	}
	if got.RemoteDest != "" {
		t.Errorf("stored RemoteDest = %q, want empty", got.RemoteDest)
	}
}

func TestBackupFileSyncRemote(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	var gotParams map[string]interface{}
	withFakeBackupAgentCap(t, s, "a1",
		[]protocol.Capability{{Type: "backup", Metadata: map[string]interface{}{"rclone": true}}},
		func(method string, params map[string]interface{}) (interface{}, string) {
			gotMethod, gotParams = method, params
			return map[string]interface{}{"synced": true}, ""
		})

	cfg := newBackupCfg("a1", "etc", "manual", true)
	cfg.RemoteDest = backupRemoteDest
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got, _ := s.db.GetBackupConfig(cfg.ID)

	sync := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.handleBackupFileSyncRemote(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs/1/files/sync-remote", strings.NewReader(body)), got)
		return rec
	}

	// 成功：下发 backup.remote.sync 并审计
	rec := sync(`{"name":"etc-20260919-030000.tar.gz"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync: %d %s", rec.Code, rec.Body.String())
	}
	if gotMethod != "backup.remote.sync" {
		t.Fatalf("dispatched method = %q", gotMethod)
	}
	if gotParams["dir"] != "/mnt/bak" || gotParams["name"] != "etc-20260919-030000.tar.gz" || gotParams["remoteDest"] != backupRemoteDest {
		t.Fatalf("params = %v", gotParams)
	}
	if body := rec.Body.String(); !strings.Contains(body, "synced") {
		t.Errorf("body = %s, want synced", body)
	}
	logs, _, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": audit.ActionBackupRemoteSync})
	if err != nil || len(logs) != 1 {
		t.Fatalf("audit logs = %d, %v; want 1", len(logs), err)
	}
	if !strings.Contains(logs[0].Details, backupRemoteDest) {
		t.Errorf("audit details = %s, want remoteDest echoed", logs[0].Details)
	}

	// 缺 name / 非法 name
	if rec := sync(`{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("missing name: code = %d, want 400", rec.Code)
	}
	if rec := sync(`{"name":"../evil.tar.gz"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("traversal name: code = %d, want 400", rec.Code)
	}

	// 未配置 remote_dest → 400
	cfgNoRemote := newBackupCfg("a1", "local", "manual", true)
	if err := s.db.CreateBackupConfig(cfgNoRemote); err != nil {
		t.Fatal(err)
	}
	noRemote, _ := s.db.GetBackupConfig(cfgNoRemote.ID)
	rec = httptest.NewRecorder()
	s.handleBackupFileSyncRemote(rec, httptest.NewRequest(http.MethodPost, "/r", strings.NewReader(`{"name":"x-20260919-030000.tar.gz"}`)), noRemote)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("no remote_dest: code = %d, want 400", rec.Code)
	}

	// agent 离线 → 503
	cfgOffline := newBackupCfg("ghost", "off", "manual", true)
	cfgOffline.RemoteDest = backupRemoteDest
	if err := s.db.CreateBackupConfig(cfgOffline); err != nil {
		t.Fatal(err)
	}
	offline, _ := s.db.GetBackupConfig(cfgOffline.ID)
	rec = httptest.NewRecorder()
	s.handleBackupFileSyncRemote(rec, httptest.NewRequest(http.MethodPost, "/r", strings.NewReader(`{"name":"x-20260919-030000.tar.gz"}`)), offline)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("agent offline: code = %d, want 503", rec.Code)
	}
}

func TestBackupFileSyncRemoteErrorPassthrough(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgentCap(t, s, "a1",
		[]protocol.Capability{{Type: "backup", Metadata: map[string]interface{}{"rclone": true}}},
		func(method string, params map[string]interface{}) (interface{}, string) {
			return nil, "rclone copy: exit status 1: directory not found"
		})
	cfg := newBackupCfg("a1", "etc", "manual", true)
	cfg.RemoteDest = backupRemoteDest
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got, _ := s.db.GetBackupConfig(cfg.ID)

	rec := httptest.NewRecorder()
	s.handleBackupFileSyncRemote(rec, httptest.NewRequest(http.MethodPost, "/r", strings.NewReader(`{"name":"etc-20260919-030000.tar.gz"}`)), got)
	// agent 侧错误含 rclone stderr 摘要 → 透传给用户（D24）
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "directory not found") {
		t.Errorf("body = %s, want agent error passed through", rec.Body.String())
	}
}

// TestTrackBackupTaskRemoteFailedNotifies 全链路：下发带 remoteDest → 任务成功
// 但 remoteStatus=failed → run 记录回填 Remote* → LastStatus 保持 success →
// backup.remote-failed 独立通知（D21）
func TestTrackBackupTaskRemoteFailedNotifies(t *testing.T) {
	s := newBackupTestServer(t)

	// webhook 渠道捕获通知
	hook := make(chan map[string]interface{}, 4)
	hookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]interface{}
		json.NewDecoder(r.Body).Decode(&m)
		hook <- m
	}))
	defer hookSrv.Close()
	s.notifier = notification.NewService(&config.NotificationConfig{
		Enabled: true,
		Webhook: []*config.WebhookConfig{{URL: hookSrv.URL}},
		Events: map[string]*config.EventConfig{
			notification.BackupRemoteFailed: {Type: notification.BackupRemoteFailed, Enabled: true},
		},
	})

	var dispatchedRemoteDest interface{}
	polls := 0
	withFakeBackupAgentCap(t, s, "a1",
		[]protocol.Capability{{Type: "backup", Metadata: map[string]interface{}{"rclone": true}}},
		func(method string, params map[string]interface{}) (interface{}, string) {
			switch method {
			case "backup.run":
				dispatchedRemoteDest = params["remoteDest"]
				return map[string]interface{}{"taskId": "bt-r", "status": "started"}, ""
			case "backup.task.get":
				polls++
				if polls >= 2 {
					return map[string]interface{}{
						"taskId": "bt-r", "status": "success",
						"file": "etc-20260919-030000.tar.gz", "size": float64(4096),
						"remoteStatus": "failed", "remoteError": "rclone boom: connection refused",
					}, ""
				}
				return map[string]interface{}{"taskId": "bt-r", "status": "running"}, ""
			default:
				return nil, "unexpected method " + method
			}
		})

	cfg := newBackupCfg("a1", "etc", "daily@03:00", true)
	cfg.RemoteDest = backupRemoteDest
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got, _ := s.db.GetBackupConfig(cfg.ID)

	rec := httptest.NewRecorder()
	s.handleBackupRun(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs/1/run", nil), got)
	if rec.Code != http.StatusOK {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}
	if dispatchedRemoteDest != backupRemoteDest {
		t.Fatalf("dispatch remoteDest = %v, want %q", dispatchedRemoteDest, backupRemoteDest)
	}

	// 等 track goroutine 回填终态
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if run, err := s.db.GetBackupRun(1); err == nil && run.Status != "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	run, err := s.db.GetBackupRun(1)
	if err != nil {
		t.Fatal(err)
	}
	// D21：本地成功即 success，Remote* 独立记录
	if run.Status != "success" || run.RemoteStatus != "failed" {
		t.Fatalf("run = %+v, want success + remote failed", run)
	}
	if !strings.Contains(run.RemoteError, "connection refused") {
		t.Errorf("remoteError = %q, want stderr summary", run.RemoteError)
	}
	gotCfg, _ := s.db.GetBackupConfig(cfg.ID)
	if gotCfg.LastStatus != "success" {
		t.Errorf("last status = %q, want success (remote failure must not flip)", gotCfg.LastStatus)
	}

	// backup.remote-failed 通知到达 webhook
	select {
	case m := <-hook:
		if m["event_type"] != notification.BackupRemoteFailed {
			t.Errorf("event = %v, want %s", m["event_type"], notification.BackupRemoteFailed)
		}
		if !strings.Contains(fmt.Sprint(m["message"]), "connection refused") {
			t.Errorf("message = %v, want stderr summary", m["message"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("backup.remote-failed notification never delivered")
	}
}

// TestTrackBackupTaskRemoteOkSilent 推送成功不发通知
func TestTrackBackupTaskRemoteOkSilent(t *testing.T) {
	s := newBackupTestServer(t)
	hook := make(chan map[string]interface{}, 4)
	hookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]interface{}
		json.NewDecoder(r.Body).Decode(&m)
		hook <- m
	}))
	defer hookSrv.Close()
	s.notifier = notification.NewService(&config.NotificationConfig{
		Enabled: true,
		Webhook: []*config.WebhookConfig{{URL: hookSrv.URL}},
		Events: map[string]*config.EventConfig{
			notification.BackupRemoteFailed: {Type: notification.BackupRemoteFailed, Enabled: true},
		},
	})

	polls := 0
	withFakeBackupAgentCap(t, s, "a1",
		[]protocol.Capability{{Type: "backup", Metadata: map[string]interface{}{"rclone": true}}},
		func(method string, params map[string]interface{}) (interface{}, string) {
			if method == "backup.run" {
				return map[string]interface{}{"taskId": "bt-ok", "status": "started"}, ""
			}
			polls++
			if method == "backup.task.get" && polls >= 2 {
				return map[string]interface{}{
					"taskId": "bt-ok", "status": "success",
					"file": "etc.tar.gz", "size": float64(10),
					"remoteStatus": "ok",
				}, ""
			}
			return map[string]interface{}{"taskId": "bt-ok", "status": "running"}, ""
		})

	cfg := newBackupCfg("a1", "etc", "daily@03:00", true)
	cfg.RemoteDest = backupRemoteDest
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got, _ := s.db.GetBackupConfig(cfg.ID)
	rec := httptest.NewRecorder()
	s.handleBackupRun(rec, httptest.NewRequest(http.MethodPost, "/api/backups/configs/1/run", nil), got)
	if rec.Code != http.StatusOK {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if run, err := s.db.GetBackupRun(1); err == nil && run.Status != "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	run, _ := s.db.GetBackupRun(1)
	if run.Status != "success" || run.RemoteStatus != "ok" {
		t.Fatalf("run = %+v, want success + remote ok", run)
	}
	select {
	case m := <-hook:
		t.Fatalf("unexpected notification: %v", m)
	case <-time.After(300 * time.Millisecond):
		// 无通知即符合预期
	}
}
