package rpc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installFakeRclone 注入一个记录 argv 的 fake rclone，返回可执行路径与 argv 日志文件
func installFakeRclone(t *testing.T, exitCode int, stderr string) (bin, argvFile string) {
	t.Helper()
	dir := t.TempDir()
	argvFile = filepath.Join(dir, "argv.log")
	bin = filepath.Join(dir, "fake-rclone")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %s\necho %q >&2\nexit %d\n", argvFile, stderr, exitCode)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argvFile
}

// readRcloneArgv 读 fake rclone 记录的 argv（空文件视为从未执行）
func readRcloneArgv(t *testing.T, argvFile string) string {
	t.Helper()
	b, err := os.ReadFile(argvFile)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func TestBackupRemotePushOk(t *testing.T) {
	bin, argvFile := installFakeRclone(t, 0, "")
	p := newBackupTestProvider(t)
	p.rcloneBin = bin

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()

	res, err := p.Call("run", map[string]interface{}{
		"configId":   float64(1),
		"name":       "web",
		"sources":    toIfaceSlice([]string{src}),
		"destDir":    dest,
		"retention":  float64(0),
		"remoteDest": "my-s3:cockpit/backups",
	})
	if err != nil {
		t.Fatalf("backup.run: %v", err)
	}
	task := waitBackupTask(t, p, res.(map[string]interface{})["taskId"].(string))

	// D21：本地打包成功即任务 success，推送成功记 remoteStatus=ok
	if task["status"] != backupTaskSuccess {
		t.Fatalf("status = %v, want success; log: %v", task["status"], task["log"])
	}
	if task["remoteStatus"] != "ok" || task["remoteError"] != "" {
		t.Errorf("remote = %v / %q, want ok / empty", task["remoteStatus"], task["remoteError"])
	}

	// D18：rclone copy --transfers 2 <本地文件> <remote:path>
	want := fmt.Sprintf("copy --transfers 2 %s my-s3:cockpit/backups", filepath.Join(dest, task["file"].(string)))
	if got := readRcloneArgv(t, argvFile); got != want {
		t.Errorf("rclone argv = %q, want %q", got, want)
	}
}

func TestBackupRemotePushFailedKeepsSuccess(t *testing.T) {
	bin, argvFile := installFakeRclone(t, 1, "rclone boom: access denied")
	p := newBackupTestProvider(t)
	p.rcloneBin = bin

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()

	res, err := p.Call("run", map[string]interface{}{
		"configId":   float64(1),
		"name":       "web",
		"sources":    toIfaceSlice([]string{src}),
		"destDir":    dest,
		"retention":  float64(0),
		"remoteDest": "my-s3:cockpit/backups",
	})
	if err != nil {
		t.Fatalf("backup.run: %v", err)
	}
	task := waitBackupTask(t, p, res.(map[string]interface{})["taskId"].(string))

	// D21：推送失败不改任务终态，本地产物仍在；RemoteStatus/RemoteError 独立记录
	if task["status"] != backupTaskSuccess {
		t.Fatalf("status = %v, want success (remote failure must not flip terminal state); log: %v",
			task["status"], task["log"])
	}
	if task["remoteStatus"] != "failed" {
		t.Errorf("remoteStatus = %v, want failed", task["remoteStatus"])
	}
	if !strings.Contains(task["remoteError"].(string), "access denied") {
		t.Errorf("remoteError = %q, want stderr summary", task["remoteError"])
	}
	if _, err := os.Stat(filepath.Join(dest, task["file"].(string))); err != nil {
		t.Errorf("local archive missing after remote failure: %v", err)
	}
	if readRcloneArgv(t, argvFile) == "" {
		t.Error("fake rclone was never executed")
	}
}

func TestBackupRunWithoutRemoteDestSkipsPush(t *testing.T) {
	bin, argvFile := installFakeRclone(t, 0, "")
	p := newBackupTestProvider(t)
	p.rcloneBin = bin

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := runBackupAndWait(t, p, "web", []string{src}, t.TempDir(), 0)

	if task["status"] != backupTaskSuccess {
		t.Fatalf("status = %v, want success", task["status"])
	}
	if task["remoteStatus"] != "" {
		t.Errorf("remoteStatus = %q, want empty when remoteDest not set", task["remoteStatus"])
	}
	if got := readRcloneArgv(t, argvFile); got != "" {
		t.Errorf("rclone executed without remoteDest: %q", got)
	}
}

func TestBackupRemoteDestValidation(t *testing.T) {
	p := newBackupTestProvider(t)
	src := t.TempDir()
	dest := t.TempDir()
	base := map[string]interface{}{
		"configId":  float64(1),
		"name":      "web",
		"sources":   toIfaceSlice([]string{src}),
		"destDir":   dest,
		"retention": float64(0),
	}

	cases := []struct {
		remoteDest string
		ok         bool
	}{
		{"", true},                      // 空 = 不启用异地
		{"my-s3:cockpit/backups", true}, // 常规
		{"a.b_c-d:x", true},             // remote 名允许 . _ -
		{"0bucket:/abs/path", true},     // 数字开头 + 绝对路径
		{"-flag:dest", false},           // D22：remote 名不以 - 开头（根除 flag 混淆）
		{"no-colon", false},             // 缺 remote:path 形态
		{":path", false},                // remote 名为空
		{"ok:has space", false},         // path 段禁空白
		{"ok:tab\there", false},         // path 段禁控制字符
		{"ok:del\x7f", false},           // path 段禁 DEL
	}
	for i, c := range cases {
		params := map[string]interface{}{}
		for k, v := range base {
			params[k] = v
		}
		// 合法 case 会异步启动任务并持有按名互斥锁，逐 case 换名避免误撞 busy
		params["name"] = fmt.Sprintf("web%d", i)
		params["remoteDest"] = c.remoteDest
		res, err := p.Call("run", params)
		if c.ok != (err == nil) {
			t.Errorf("remoteDest %q: err = %v, want ok=%v", c.remoteDest, err, c.ok)
			continue
		}
		// 合法 case 已启动任务：等终态，释放并发名额与按名锁
		if err == nil {
			waitBackupTask(t, p, res.(map[string]interface{})["taskId"].(string))
		}
	}
}

func TestBackupSyncRemote(t *testing.T) {
	bin, argvFile := installFakeRclone(t, 0, "")
	p := newBackupTestProvider(t)
	p.rcloneBin = bin

	dir := t.TempDir()
	name := "web-20260919-120000.tar.gz"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("archive"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := p.Call("remote.sync", map[string]interface{}{
		"dir":        dir,
		"name":       name,
		"remoteDest": "my-s3:cockpit/backups",
	})
	if err != nil {
		t.Fatalf("remote.sync: %v", err)
	}
	if res.(map[string]interface{})["synced"] != true {
		t.Errorf("synced = %v, want true", res)
	}
	want := fmt.Sprintf("copy --transfers 2 %s my-s3:cockpit/backups", filepath.Join(dir, name))
	if got := readRcloneArgv(t, argvFile); got != want {
		t.Errorf("rclone argv = %q, want %q", got, want)
	}

	// 参数校验
	badCases := []struct {
		label string
		dir   string
		name  string
		dest  string
	}{
		{"missing file", dir, "web-20990101-000000.tar.gz", "my-s3:x"},
		{"relative dir", "rel/dir", name, "my-s3:x"},
		{"traversal name", dir, "../evil.tar.gz", "my-s3:x"},
		{"bad name", dir, "not-a-backup.txt", "my-s3:x"},
		{"bad remoteDest", dir, name, "-flag:x"},
		{"empty remoteDest", dir, name, ""},
	}
	for _, c := range badCases {
		if _, err := p.Call("remote.sync", map[string]interface{}{
			"dir": c.dir, "name": c.name, "remoteDest": c.dest,
		}); err == nil {
			t.Errorf("%s: want error, got nil", c.label)
		}
	}
}

func TestBackupSyncRemoteFailurePropagates(t *testing.T) {
	bin, _ := installFakeRclone(t, 3, "network unreachable")
	p := newBackupTestProvider(t)
	p.rcloneBin = bin

	dir := t.TempDir()
	name := "web-20260919-120000.tar.gz"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("archive"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := p.Call("remote.sync", map[string]interface{}{
		"dir": dir, "name": name, "remoteDest": "my-s3:cockpit/backups",
	})
	if err == nil || !strings.Contains(err.Error(), "network unreachable") {
		t.Fatalf("err = %v, want stderr summary propagated", err)
	}
}

func TestRcloneAvailable(t *testing.T) {
	// PATH 指向含 rclone 的目录 → true；空目录 → false
	withRclone := t.TempDir()
	if err := os.Symlink("/bin/true", filepath.Join(withRclone, "rclone")); err != nil {
		t.Skip("cannot create symlink:", err)
	}
	t.Setenv("PATH", withRclone)
	if !RcloneAvailable() {
		t.Error("RcloneAvailable = false with rclone on PATH")
	}
	t.Setenv("PATH", t.TempDir())
	if RcloneAvailable() {
		t.Error("RcloneAvailable = true with empty PATH")
	}
}
