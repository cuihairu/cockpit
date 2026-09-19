package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// ============ M2 录制归档异地（rclone，见 recording-design.md D14-D19）============

// runRecordingSession 走完整链路：startRecording → WriteOutput → Close
// （finish 回调内异步归档）
func runRecordingSession(t *testing.T, s *Server, sessionID string) {
	t.Helper()
	session := &TerminalSession{
		ID: sessionID, Username: "alice", AgentID: "a1",
		Protocol: protocol.RemoteProtocolSSH, Host: "10.0.0.5", Port: 22,
		CreatedAt: time.Now(),
	}
	rec := s.startRecording(session)
	if rec == nil {
		t.Fatal("startRecording returned nil")
	}
	rec.WriteOutput([]byte("$ ls\n"))
	rec.Close()
}

func waitForArgv(t *testing.T, argvFile string) []string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(argvFile); err == nil {
			return strings.Fields(strings.TrimSpace(string(data)))
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("recording archive push never invoked")
	return nil
}

// TestRecordingArchivePushOK 归档成功：argv 同款，本地文件不受影响
func TestRecordingArchivePushOK(t *testing.T) {
	s := newRecordingTestServer(t)
	argvFile := installServerFakeRclone(t, 0, "")
	if err := s.db.SetSetting(RecordingRemoteDestSettingKey, "my-s3:recordings"); err != nil {
		t.Fatal(err)
	}

	runRecordingSession(t, s, "11111111-2222-3333-4444-555555555555")
	argv := waitForArgv(t, argvFile)
	want := []string{"copy", "--transfers", "2",
		filepath.Join(s.recordingsDir(), "11111111-2222-3333-4444-555555555555.cast"), "my-s3:recordings"}
	if len(argv) != len(want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv = %v, want %v", argv, want)
		}
	}
	// 本地档仍在（retention 未到期不删）
	if _, err := os.Stat(filepath.Join(s.recordingsDir(), "11111111-2222-3333-4444-555555555555.cast")); err != nil {
		t.Fatalf("local cast missing: %v", err)
	}
}

// TestRecordingArchiveFailedNotifies D17：失败通知，本地档不受影响
func TestRecordingArchiveFailedNotifies(t *testing.T) {
	s := newRecordingTestServer(t)
	installServerFakeRclone(t, 1, "rclone boom: quota exceeded")
	if err := s.db.SetSetting(RecordingRemoteDestSettingKey, "my-s3:recordings"); err != nil {
		t.Fatal(err)
	}

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
			notification.RecordingRemoteFailed: {Type: notification.RecordingRemoteFailed, Enabled: true},
		},
	})

	sid := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	runRecordingSession(t, s, sid)

	// 本地成果与元数据回填不受推送失败影响
	meta, err := s.db.GetTerminalRecording(sid)
	if err != nil || meta.DurationMs < 0 {
		t.Fatalf("meta = %+v err=%v", meta, err)
	}

	select {
	case m := <-hook:
		if m["event_type"] != notification.RecordingRemoteFailed {
			t.Errorf("event = %v, want %s", m["event_type"], notification.RecordingRemoteFailed)
		}
		if !strings.Contains(fmt.Sprint(m["message"]), "quota exceeded") {
			t.Errorf("message = %v, want stderr summary", m["message"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("recording.remote-failed notification never delivered")
	}
}

// TestRecordingArchiveNotConfiguredSkips 未配置 remote_dest 不调用 rclone
func TestRecordingArchiveNotConfiguredSkips(t *testing.T) {
	s := newRecordingTestServer(t)
	argvFile := installServerFakeRclone(t, 0, "")

	runRecordingSession(t, s, "99999999-8888-7777-6666-555555555555")
	time.Sleep(300 * time.Millisecond) // 给异步 goroutine 窗口
	if _, err := os.ReadFile(argvFile); !os.IsNotExist(err) {
		t.Fatal("rclone should not be invoked without remote_dest")
	}
}

// TestRecordingArchiveAsyncDoesNotBlockClose D16：Close 不等推送（slow fake）
func TestRecordingArchiveAsyncDoesNotBlockClose(t *testing.T) {
	s := newRecordingTestServer(t)
	argvFile := filepath.Join(t.TempDir(), "argv.log")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %s\nsleep 30\n", argvFile)
	bin := filepath.Join(t.TempDir(), "fake-rclone-slow")
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	setServerRclone(t, bin, 200*time.Millisecond)
	if err := s.db.SetSetting(RecordingRemoteDestSettingKey, "my-s3:recordings"); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	runRecordingSession(t, s, "77777777-6666-5555-4444-333333333333")
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Close blocked %v on archive push", elapsed)
	}
	waitForArgv(t, argvFile) // 推送确实发生
}

// TestRecordingsConfigAPI D19：配置 GET/PUT 全形态 + rclone_available
func TestRecordingsConfigAPI(t *testing.T) {
	s := newRecordingTestServer(t)
	installServerFakeRclone(t, 0, "")

	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.handleRecordings(rec, httptest.NewRequest(http.MethodPut, "/recordings/config", strings.NewReader(body)))
		return rec
	}

	// 越界 retention 与非法 remote_dest → 400
	if rec := put(`{"retention_days":999}`); rec.Code != http.StatusBadRequest {
		t.Errorf("retention 999: code = %d, want 400", rec.Code)
	}
	if rec := put(`{"remote_dest":"-flag:x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad dest: code = %d, want 400", rec.Code)
	}
	if got := s.recordingRemoteDest(); got != "" {
		t.Fatalf("invalid dest stored = %q", got)
	}

	// 合法写入（开关+保留+异地）→ 回显
	rec := put(`{"enabled":false,"retention_days":30,"remote_dest":"my-s3:recordings"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT valid: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"enabled":false`) || !strings.Contains(rec.Body.String(), "my-s3:recordings") {
		t.Fatalf("resp = %s", rec.Body.String())
	}
	if got := s.recordingRetentionDays(); got != 30 {
		t.Errorf("retention = %d, want 30", got)
	}

	// 清空异地 → 200
	if rec := put(`{"remote_dest":""}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT empty dest: %d %s", rec.Code, rec.Body.String())
	}

	// GET 全字段 + rclone_available
	rec = httptest.NewRecorder()
	s.handleRecordings(rec, httptest.NewRequest(http.MethodGet, "/recordings/config", nil))
	var cfg struct {
		Enabled         bool   `json:"enabled"`
		RetentionDays   int    `json:"retention_days"`
		RemoteDest      string `json:"remote_dest"`
		RcloneAvailable bool   `json:"rclone_available"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.RetentionDays != 30 || cfg.RemoteDest != "" || !cfg.RcloneAvailable {
		t.Fatalf("cfg = %+v", cfg)
	}
}

// TestRecordingSyncRemoteAPI D18：非法 sid/未配置/404/成功+审计
func TestRecordingSyncRemoteAPI(t *testing.T) {
	s := newRecordingTestServer(t)
	argvFile := installServerFakeRclone(t, 0, "")
	sid := "abcdefab-1234-5678-9abc-def012345678"

	sync := func(id string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.handleRecordings(rec, httptest.NewRequest(http.MethodPost, "/recordings/"+id+"/sync-remote", nil))
		return rec
	}
	setDest := func(dest string) {
		if err := s.db.SetSetting(RecordingRemoteDestSettingKey, dest); err != nil {
			t.Fatal(err)
		}
	}

	// 非法 sid（非 uuid 形态）→ 400
	rec := sync("evil")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad sid: code = %d, want 400", rec.Code)
	}
	// 深穿越被路由分段的段数挡在 404（到不了文件寻址，安全兜底）
	rec = httptest.NewRecorder()
	s.handleRecordings(rec, httptest.NewRequest(http.MethodPost, "/recordings/..%2F..%2Fetc%2Fpasswd%2Fsid/sync-remote", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("traversal sid: code = %d, want 404", rec.Code)
	}
	// 未配置 remote_dest → 400
	if rec := sync(sid); rec.Code != http.StatusBadRequest {
		t.Errorf("no dest: code = %d, want 400", rec.Code)
	}

	setDest("my-s3:recordings")
	// 元数据不存在 → 404
	if rec := sync(sid); rec.Code != http.StatusNotFound {
		t.Errorf("no meta: code = %d, want 404", rec.Code)
	}

	// 造完整录制（临时撤掉 remote_dest：录制链路自身不推，隔离 sync 的 argv）
	setDest("")
	runRecordingSession(t, s, sid)
	waitDeadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(waitDeadline) {
		time.Sleep(20 * time.Millisecond)
	}
	os.Remove(argvFile) // 清掉链路可能产生的残留，sync 的 argv 单独归账

	setDest("my-s3:recordings")
	rec = sync(sid)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync: %d %s", rec.Code, rec.Body.String())
	}
	argv := readServerArgv(t, argvFile)
	if len(argv) != 5 || argv[0] != "copy" || argv[4] != "my-s3:recordings" {
		t.Fatalf("argv = %v", argv)
	}
	logs, _, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"resource": "recording"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if strings.Contains(l.Details, "sync_remote") {
			found = true
		}
	}
	if !found {
		t.Fatalf("sync_remote audit missing in %d logs", len(logs))
	}
}
