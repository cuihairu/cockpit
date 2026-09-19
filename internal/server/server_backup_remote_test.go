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
)

// ============ M2 异地传输（server 侧 rclone，见 server-backup-design.md D11-D18）============

// installServerFakeRclone 注入 fake rclone：argv 记录到文件，可配置退出码
// 与 stderr（同 agent 侧 installFakeRclone 模式）。返回 argv 文件路径。
func installServerFakeRclone(t *testing.T, exitCode int, stderr string) string {
	t.Helper()
	argvFile := filepath.Join(t.TempDir(), "argv.log")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %s\necho %q >&2\nexit %d\n", argvFile, stderr, exitCode)
	bin := filepath.Join(t.TempDir(), "fake-rclone")
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	old := serverRcloneBin
	serverRcloneBin = bin
	t.Cleanup(func() { serverRcloneBin = old })
	return argvFile
}

// setServerRemoteDest 直写 Setting（绕过 API 层校验，模拟已配置状态）
func setServerRemoteDest(t *testing.T, s *Server, dest string) {
	t.Helper()
	if err := s.db.SetSetting(ServerBackupRemoteDestSettingKey, dest); err != nil {
		t.Fatal(err)
	}
}

func readServerArgv(t *testing.T, argvFile string) []string {
	t.Helper()
	data, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("fake rclone not invoked: %v", err)
	}
	return strings.Fields(strings.TrimSpace(string(data)))
}

// TestServerBackupRemoteDestSetting 读写语义：空=关闭；脏值读时视为未配置（D13）
func TestServerBackupRemoteDestSetting(t *testing.T) {
	s := newServerBackupTestServer(t)
	if got := s.serverBackupRemoteDest(); got != "" {
		t.Fatalf("default remote_dest = %q, want empty", got)
	}
	setServerRemoteDest(t, s, "my-s3:cockpit")
	if got := s.serverBackupRemoteDest(); got != "my-s3:cockpit" {
		t.Fatalf("remote_dest = %q", got)
	}
	// 写入口之后被手改的脏值：读宽松，视为未配置且不 panic
	setServerRemoteDest(t, s, "-flag:x")
	if got := s.serverBackupRemoteDest(); got != "" {
		t.Fatalf("dirty remote_dest = %q, want empty", got)
	}
	setServerRemoteDest(t, s, "ok:has space")
	if got := s.serverBackupRemoteDest(); got != "" {
		t.Fatalf("dirty remote_dest = %q, want empty", got)
	}
}

// TestServerBackupConfigRemoteDestAPI config API：写时严格校验 + 回显 + rclone_available
func TestServerBackupConfigRemoteDestAPI(t *testing.T) {
	s := newServerBackupTestServer(t)
	installServerFakeRclone(t, 0, "")

	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.handleServerBackups(rec, httptest.NewRequest(http.MethodPut, "/server-backups/config", strings.NewReader(body)))
		return rec
	}

	// 非法形态 400（flag 混淆/空白/缺冒号/超长）
	longDest := "r:" + strings.Repeat("x", 600)
	for _, body := range []string{
		`{"remote_dest":"-flag:x"}`,
		`{"remote_dest":"ok:has space"}`,
		`{"remote_dest":"no-colon"}`,
		fmt.Sprintf(`{"remote_dest":%q}`, longDest),
	} {
		if rec := put(body); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %s: code = %d, want 400", body, rec.Code)
		}
	}
	// 非法值未落库
	if got := s.serverBackupRemoteDest(); got != "" {
		t.Fatalf("invalid dest stored = %q", got)
	}

	// 合法写入 → 回显
	rec := put(`{"remote_dest":"my-s3:cockpit"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "my-s3:cockpit") {
		t.Fatalf("PUT valid: %d %s", rec.Code, rec.Body.String())
	}

	// 清空（空串 = 关闭）→ 200
	if rec := put(`{"remote_dest":""}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT empty: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.serverBackupRemoteDest(); got != "" {
		t.Fatalf("after clear = %q", got)
	}

	// GET 含 remote_dest 与 rclone_available（fake 已注入 = true）
	rec = httptest.NewRecorder()
	s.handleServerBackups(rec, httptest.NewRequest(http.MethodGet, "/server-backups/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d", rec.Code)
	}
	var cfg struct {
		RemoteDest      string `json:"remote_dest"`
		RcloneAvailable bool   `json:"rclone_available"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.RcloneAvailable {
		t.Error("rclone_available = false, want true (fake installed)")
	}
}

// TestServerBackupRemotePushOK 推送成功：argv 与 agent D18 同款，本地成果不变
func TestServerBackupRemotePushOK(t *testing.T) {
	s := newServerBackupTestServer(t)
	argvFile := installServerFakeRclone(t, 0, "")
	setServerRemoteDest(t, s, "my-s3:cockpit")

	name, err := s.runServerBackup()
	if err != nil {
		t.Fatal(err)
	}
	argv := readServerArgv(t, argvFile)
	want := []string{"copy", "--transfers", "2", filepath.Join(s.serverBackupDir(), name), "my-s3:cockpit"}
	if len(argv) != len(want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv = %v, want %v", argv, want)
		}
	}
}

// TestServerBackupRemoteNotConfiguredSkips 未配置 remote_dest 不调用 rclone
func TestServerBackupRemoteNotConfiguredSkips(t *testing.T) {
	s := newServerBackupTestServer(t)
	argvFile := installServerFakeRclone(t, 0, "")

	if _, err := s.runServerBackup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(argvFile); !os.IsNotExist(err) {
		t.Fatal("rclone should not be invoked without remote_dest")
	}
}

// TestServerBackupRemoteFailedNotifies D14/D16：推送失败不影响本地成果，
// server_backup.remote-failed 走独立事件白名单到达 webhook
func TestServerBackupRemoteFailedNotifies(t *testing.T) {
	s := newServerBackupTestServer(t)
	installServerFakeRclone(t, 1, "rclone boom: connection refused")
	setServerRemoteDest(t, s, "my-s3:cockpit")

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
			notification.ServerBackupRemoteFailed: {Type: notification.ServerBackupRemoteFailed, Enabled: true},
		},
	})

	name, err := s.runServerBackup()
	if err != nil {
		t.Fatalf("local backup must succeed regardless: %v", err)
	}
	if info, err := os.Stat(filepath.Join(s.serverBackupDir(), name)); err != nil || info.Size() == 0 {
		t.Fatalf("local backup missing: %v", err)
	}

	select {
	case m := <-hook:
		if m["event_type"] != notification.ServerBackupRemoteFailed {
			t.Errorf("event = %v, want %s", m["event_type"], notification.ServerBackupRemoteFailed)
		}
		if !strings.Contains(fmt.Sprint(m["message"]), "connection refused") {
			t.Errorf("message = %v, want stderr summary", m["message"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server_backup.remote-failed notification never delivered")
	}
}

// TestServerBackupRemoteTimeout D15：超时整杀 rclone，本地成果不受影响
func TestServerBackupRemoteTimeout(t *testing.T) {
	s := newServerBackupTestServer(t)
	argvFile := filepath.Join(t.TempDir(), "argv.log")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %s\nsleep 30\n", argvFile)
	bin := filepath.Join(t.TempDir(), "fake-rclone-slow")
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	oldBin, oldWait := serverRcloneBin, serverBackupRemoteMaxWait
	serverRcloneBin, serverBackupRemoteMaxWait = bin, 150*time.Millisecond
	t.Cleanup(func() { serverRcloneBin, serverBackupRemoteMaxWait = oldBin, oldWait })
	setServerRemoteDest(t, s, "my-s3:cockpit")

	start := time.Now()
	if _, err := s.runServerBackup(); err != nil {
		t.Fatalf("local backup must succeed despite timeout: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("push took %v, timeout not applied", elapsed)
	}
	readServerArgv(t, argvFile) // 确实调用过
}

// TestServerBackupSyncRemoteAPI D17 手动补推全路径：未配置 400 / 文件不存在
// 404 / 成功（argv + 审计）/ 穿越名 400
func TestServerBackupSyncRemoteAPI(t *testing.T) {
	s := newServerBackupTestServer(t)
	argvFile := installServerFakeRclone(t, 0, "")

	sync := func(name string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.handleServerBackups(rec, httptest.NewRequest(http.MethodPost, "/server-backups/"+name+"/sync-remote", nil))
		return rec
	}

	// 未配置 remote_dest → 400
	if rec := sync("cockpit-20260919-030000.db"); rec.Code != http.StatusBadRequest {
		t.Fatalf("no remote_dest: code = %d, want 400", rec.Code)
	}

	setServerRemoteDest(t, s, "my-s3:cockpit")

	// 穿越与畸形名 → 400
	if rec := sync("..%2Fcockpit.db"); rec.Code != http.StatusBadRequest {
		t.Fatalf("traversal: code = %d, want 400", rec.Code)
	}

	// 文件不存在 → 404
	if rec := sync("cockpit-20260919-030000.db"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing file: code = %d, want 404", rec.Code)
	}

	// 造一个合法备份文件 → 成功且 argv 正确 + 审计
	dir := s.serverBackupDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	name := "cockpit-20260919-030000.db"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	rec := sync(name)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync: %d %s", rec.Code, rec.Body.String())
	}
	argv := readServerArgv(t, argvFile)
	if len(argv) != 5 || argv[0] != "copy" || argv[4] != "my-s3:cockpit" || !strings.HasSuffix(argv[3], name) {
		t.Fatalf("argv = %v", argv)
	}
	logs, _, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"resource": "server_backup"})
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
