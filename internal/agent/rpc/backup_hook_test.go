package rpc

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============ M3 数据库热备钩子（pre-hook，见 backup-design.md D26-D32）============

// TestBackupPreHookRunsBeforePack hook 经 sh -c 执行（重定向可用）且先于打包：
// hook 生成快照文件，源路径指向该文件，打包成功即证明顺序与语义
func TestBackupPreHookRunsBeforePack(t *testing.T) {
	p := newBackupTestProvider(t)
	dest := t.TempDir()
	snapshot := filepath.Join(t.TempDir(), "app.db.bak")

	res, err := p.Call("run", map[string]interface{}{
		"configId":  float64(1),
		"name":      "appdb",
		"sources":   toIfaceSlice([]string{snapshot}),
		"destDir":   dest,
		"retention": float64(0),
		// shell 重定向验证 sh -c 语义（D27）
		"preHook": fmt.Sprintf(`echo "fake snapshot" > %q`, snapshot),
	})
	if err != nil {
		t.Fatalf("backup.run: %v", err)
	}
	task := waitBackupTask(t, p, res.(map[string]interface{})["taskId"].(string))

	if task["status"] != backupTaskSuccess {
		t.Fatalf("status = %v, want success; log: %v", task["status"], task["log"])
	}
	if log := task["log"].(string); !strings.Contains(log, "[hook] echo") {
		t.Errorf("log missing [hook] line: %q", log)
	}

	// 打包产物里应包含 hook 产出的快照内容
	f, err := os.Open(filepath.Join(dest, task["file"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var content string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(hdr.Name) == filepath.Base(snapshot) {
			body, _ := io.ReadAll(tr)
			content = string(body)
		}
	}
	if strings.TrimSpace(content) != "fake snapshot" {
		t.Errorf("archived snapshot = %q, want hook output", content)
	}
}

// TestBackupPreHookFailureAborts hook 非零退出 → 任务 failed、不打包、不推送（D28）
func TestBackupPreHookFailureAborts(t *testing.T) {
	bin, argvFile := installFakeRclone(t, 0, "")
	p := newBackupTestProvider(t)
	p.rcloneBin = bin
	dest := t.TempDir()

	res, err := p.Call("run", map[string]interface{}{
		"configId":   float64(1),
		"name":       "appdb",
		"sources":    toIfaceSlice([]string{t.TempDir()}),
		"destDir":    dest,
		"retention":  float64(0),
		"remoteDest": "my-s3:cockpit/backups",
		"preHook":    "echo dump-boom >&2; exit 3",
	})
	if err != nil {
		t.Fatalf("backup.run: %v", err)
	}
	task := waitBackupTask(t, p, res.(map[string]interface{})["taskId"].(string))

	if task["status"] != backupTaskFailed {
		t.Fatalf("status = %v, want failed; log: %v", task["status"], task["log"])
	}
	errMsg := task["error"].(string)
	if !strings.Contains(errMsg, "pre-hook failed") || !strings.Contains(errMsg, "dump-boom") {
		t.Errorf("error = %q, want exit reason + stderr summary", errMsg)
	}
	// 不打包：目标目录无产物
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Errorf("dest has %d entries, want 0 (must abort before pack)", len(entries))
	}
	// 不推送远端
	if readRcloneArgv(t, argvFile) != "" {
		t.Error("rclone executed despite hook failure")
	}
}

// TestBackupPreHookTimeout hook 卡死 → 超时整组 kill 并判 failed（D29）
func TestBackupPreHookTimeout(t *testing.T) {
	old := backupHookTimeout
	backupHookTimeout = 150 * time.Millisecond
	defer func() { backupHookTimeout = old }()

	p := newBackupTestProvider(t)
	res, err := p.Call("run", map[string]interface{}{
		"configId":  float64(1),
		"name":      "appdb",
		"sources":   toIfaceSlice([]string{t.TempDir()}),
		"destDir":   t.TempDir(),
		"retention": float64(0),
		"preHook":   "sleep 30",
	})
	if err != nil {
		t.Fatalf("backup.run: %v", err)
	}
	task := waitBackupTask(t, p, res.(map[string]interface{})["taskId"].(string))

	if task["status"] != backupTaskFailed {
		t.Fatalf("status = %v, want failed", task["status"])
	}
	if !strings.Contains(task["error"].(string), "timed out") {
		t.Errorf("error = %q, want timeout message", task["error"])
	}
}

// TestBackupPreHookTooLong 超长命令在 run 入口拒绝
func TestBackupPreHookTooLong(t *testing.T) {
	p := newBackupTestProvider(t)
	_, err := p.Call("run", map[string]interface{}{
		"configId":  float64(1),
		"name":      "appdb",
		"sources":   toIfaceSlice([]string{t.TempDir()}),
		"destDir":   t.TempDir(),
		"retention": float64(0),
		"preHook":   strings.Repeat("x", 1025),
	})
	if err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("err = %v, want length rejection", err)
	}
}

// TestBackupPreHookEmptySkips 空 preHook 不执行也不留日志
func TestBackupPreHookEmptySkips(t *testing.T) {
	p := newBackupTestProvider(t)
	task := runBackupAndWait(t, p, "appdb", []string{t.TempDir()}, t.TempDir(), 0)
	if task["status"] != backupTaskSuccess {
		t.Fatalf("status = %v, want success", task["status"])
	}
	if log := task["log"].(string); strings.Contains(log, "[hook]") {
		t.Errorf("log has [hook] line without preHook: %q", log)
	}
}
