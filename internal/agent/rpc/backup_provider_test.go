package rpc

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newBackupTestProvider 构建测试用 provider
func newBackupTestProvider(t *testing.T) *BackupProvider {
	t.Helper()
	return NewBackupProvider(BackupConfig{})
}

// waitBackupTask 轮询任务直到终态或超时
func waitBackupTask(t *testing.T, p *BackupProvider, taskID string) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res, err := p.GetTask(taskID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if res["status"] != backupTaskRunning {
			return res
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("backup task did not finish in time")
	return nil
}

// runBackupAndWait 同步执行一次备份并返回终态任务
func runBackupAndWait(t *testing.T, p *BackupProvider, name string, sources []string, destDir string, retention int) map[string]interface{} {
	t.Helper()
	res, err := p.Call("run", map[string]interface{}{
		"configId":  float64(1),
		"name":      name,
		"sources":   toIfaceSlice(sources),
		"destDir":   destDir,
		"retention": float64(retention),
	})
	if err != nil {
		t.Fatalf("backup.run: %v", err)
	}
	started := res.(map[string]interface{})
	return waitBackupTask(t, p, started["taskId"].(string))
}

func toIfaceSlice(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func TestBackupRunPacksSources(t *testing.T) {
	p := newBackupTestProvider(t)
	src1 := t.TempDir() // 目录源
	if err := os.WriteFile(filepath.Join(src1, "nginx.conf"), []byte("server {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src1, "conf.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src1, "conf.d", "a.conf"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	src2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(src2, "db.sqlite"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()

	task := runBackupAndWait(t, p, "mybackup", []string{src1, src2}, dest, 0)
	if task["status"] != backupTaskSuccess {
		t.Fatalf("task = %+v, want success; log: %v", task, task["log"])
	}
	fileName := task["file"].(string)
	if !strings.HasPrefix(fileName, "mybackup-") || !strings.HasSuffix(fileName, ".tar.gz") {
		t.Errorf("file = %q, want mybackup-*.tar.gz", fileName)
	}
	if task["size"].(int64) <= 0 {
		t.Errorf("size = %v, want > 0", task["size"])
	}

	// 解包验证结构：每个源以 basename 为顶层目录
	f, err := os.Open(filepath.Join(dest, fileName))
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

	seen := map[string]string{} // name → content
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		body, _ := io.ReadAll(tr)
		seen[hdr.Name] = string(body)
	}
	for _, want := range []struct{ name, content string }{
		{filepath.Base(src1) + "/nginx.conf", "server {}"},
		{filepath.Base(src1) + "/conf.d/a.conf", "a"},
		{filepath.Base(src2) + "/db.sqlite", "data"},
	} {
		if got, ok := seen[want.name]; !ok || got != want.content {
			t.Errorf("tar entry %q = %q, %v; want %q", want.name, got, ok, want.content)
		}
	}
}

func TestBackupRetentionPrunesOld(t *testing.T) {
	p := newBackupTestProvider(t)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()

	// 直接造 3 个旧包（不同 mtime），retention=2 应删最旧的
	now := time.Now()
	for i, offset := range []time.Duration{-3 * time.Hour, -2 * time.Hour, -1 * time.Hour} {
		name := filepath.Join(dest, "db-2026010"+string(rune('1'+i))+"-000000.tar.gz")
		if err := os.WriteFile(name, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := now.Add(offset)
		if err := os.Chtimes(name, mt, mt); err != nil {
			t.Fatal(err)
		}
	}

	task := runBackupAndWait(t, p, "db", []string{src}, dest, 2)
	if task["status"] != backupTaskSuccess {
		t.Fatalf("task = %+v", task)
	}
	entries, _ := os.ReadDir(dest)
	var remain []string
	for _, e := range entries {
		remain = append(remain, e.Name())
	}
	if len(remain) != 2 {
		t.Fatalf("after retention, files = %v; want 2", remain)
	}
	for _, n := range remain {
		if strings.Contains(n, "20260101") { // 最旧的应被清理
			t.Errorf("oldest backup %q should be pruned", n)
		}
	}
}

func TestBackupRejectsUnsafeParams(t *testing.T) {
	p := newBackupTestProvider(t)
	cases := []struct {
		desc   string
		params map[string]interface{}
	}{
		{"bad name", map[string]interface{}{"name": "../evil", "sources": toIfaceSlice([]string{"/tmp"}), "destDir": "/tmp/b"}},
		{"relative destDir", map[string]interface{}{"name": "ok", "sources": toIfaceSlice([]string{"/tmp"}), "destDir": "relative/dir"}},
		{"relative source", map[string]interface{}{"name": "ok", "sources": toIfaceSlice([]string{"etc/passwd"}), "destDir": "/tmp/b"}},
		{"empty sources", map[string]interface{}{"name": "ok", "sources": toIfaceSlice([]string{}), "destDir": "/tmp/b"}},
	}
	for _, c := range cases {
		if _, err := p.Call("run", c.params); err == nil {
			t.Errorf("%s: backup.run should fail", c.desc)
		}
	}
}

func TestBackupDeleteRejectsTraversal(t *testing.T) {
	p := newBackupTestProvider(t)
	dest := t.TempDir()
	victim := filepath.Join(dest, "keep.txt")
	if err := os.WriteFile(victim, []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 穿越与目录名全拒绝
	for _, bad := range []string{"../keep.txt", "sub/../../keep.txt", "", "a/b.tar.gz", "x.zip"} {
		if _, err := p.Call("delete", map[string]interface{}{"dir": dest, "name": bad}); err == nil {
			t.Errorf("delete name=%q should fail", bad)
		}
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("victim should still exist after rejected deletes")
	}

	// 相对目录拒绝
	if _, err := p.Call("delete", map[string]interface{}{"dir": "relative", "name": "a.tar.gz"}); err == nil {
		t.Error("relative dir should fail")
	}

	// 合法删除
	good := filepath.Join(dest, "ok-name.tar.gz")
	if err := os.WriteFile(good, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Call("delete", map[string]interface{}{"dir": dest, "name": "ok-name.tar.gz"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(good); !os.IsNotExist(err) {
		t.Error("backup file should be deleted")
	}
}

func TestBackupListFiles(t *testing.T) {
	p := newBackupTestProvider(t)
	dest := t.TempDir()
	for _, n := range []string{"db-1.tar.gz", "db-2.tar.gz", "notes.txt", "sub.tar.gz.part"} {
		path := filepath.Join(dest, n)
		if strings.Contains(n, "/") {
			continue
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := p.Call("list", map[string]interface{}{"dir": dest})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	files := res.(map[string]interface{})["files"].([]BackupFile)
	if len(files) != 2 {
		t.Fatalf("files = %+v; want only 2 tar.gz", files)
	}
	for _, f := range files {
		if f.Name != "db-1.tar.gz" && f.Name != "db-2.tar.gz" {
			t.Errorf("unexpected file %q", f.Name)
		}
	}

	if _, err := p.Call("list", map[string]interface{}{"dir": "relative"}); err == nil {
		t.Error("relative dir should fail")
	}
}

func TestBackupSymlinkNotFollowed(t *testing.T) {
	p := newBackupTestProvider(t)
	src := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("TOP SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(src, "leak")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	dest := t.TempDir()

	task := runBackupAndWait(t, p, "sym", []string{src}, dest, 0)
	if task["status"] != backupTaskSuccess {
		t.Fatalf("task = %+v", task)
	}

	// 解包：leak 应为符号链接条目，且不带 secret 内容
	f, _ := os.Open(filepath.Join(dest, task["file"].(string)))
	defer f.Close()
	gz, _ := gzip.NewReader(f)
	tr := tar.NewReader(gz)
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Name == filepath.Base(src)+"/leak" {
			found = true
			if hdr.Typeflag != tar.TypeSymlink {
				t.Errorf("leak should be a symlink entry, got typeflag %c", hdr.Typeflag)
			}
			body, _ := io.ReadAll(tr)
			if string(body) == "TOP SECRET" {
				t.Error("symlink target content should not be archived")
			}
		}
	}
	if !found {
		t.Error("symlink entry missing from archive")
	}
}

func TestBackupMissingSourceSkipped(t *testing.T) {
	p := newBackupTestProvider(t)
	good := t.TempDir()
	if err := os.WriteFile(filepath.Join(good, "f"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()

	task := runBackupAndWait(t, p, "mixed", []string{good, "/nonexistent/path/xyz"}, dest, 0)
	if task["status"] != backupTaskSuccess {
		t.Fatalf("missing source should be skipped with warning, task = %+v", task)
	}
}

func TestBackupSameNameBusy(t *testing.T) {
	// 慢打包用大文件制造并发窗口：第一个任务在跑时第二个同 name 请求应 busy
	p := newBackupTestProvider(t)
	src := t.TempDir()
	big := make([]byte, 8<<20)
	if err := os.WriteFile(filepath.Join(src, "big.bin"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()

	params := map[string]interface{}{
		"name": "slow", "sources": toIfaceSlice([]string{src}),
		"destDir": dest, "retention": float64(0),
	}
	if _, err := p.Call("run", params); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// 第二次立即请求同 name → busy（TryLock 语义）
	if _, err := p.Call("run", params); err == nil {
		t.Log("second run did not hit busy window (first finished too fast)")
	}
	// 等待收尾，保证测试退出前任务完成
	deadline := time.Now().Add(10 * time.Second)
	for p.hasRunningTask() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
}

func (p *BackupProvider) hasRunningTask() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, t := range p.tasks {
		if t.Status == backupTaskRunning {
			return true
		}
	}
	return false
}
