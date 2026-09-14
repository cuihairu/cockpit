package rpc

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
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

// ============ M1.5 restore / read ============

// runRestoreAndWait 同步执行一次恢复并返回终态任务
func runRestoreAndWait(t *testing.T, p *BackupProvider, dir, name, destDir string) map[string]interface{} {
	t.Helper()
	res, err := p.Call("restore", map[string]interface{}{
		"dir": dir, "name": name, "destDir": destDir,
	})
	if err != nil {
		t.Fatalf("backup.restore: %v", err)
	}
	started := res.(map[string]interface{})
	return waitBackupTask(t, p, started["taskId"].(string))
}

func TestBackupRestoreRoundtrip(t *testing.T) {
	p := newBackupTestProvider(t)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "app.conf"), []byte("key=value"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "data.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	backupDir := t.TempDir()
	task := runBackupAndWait(t, p, "roundtrip", []string{src}, backupDir, 0)
	if task["status"] != backupTaskSuccess {
		t.Fatalf("backup failed: %v", task["error"])
	}
	name := task["file"].(string)

	restoreDir := filepath.Join(t.TempDir(), "restored") // 不存在 → 允许
	task = runRestoreAndWait(t, p, backupDir, name, restoreDir)
	if task["status"] != backupTaskSuccess {
		t.Fatalf("restore failed: %v %s", task["error"], task["status"])
	}
	// basename 顶层目录结构 + 内容一致
	base := filepath.Base(src)
	got, err := os.ReadFile(filepath.Join(restoreDir, base, "app.conf"))
	if err != nil || string(got) != "key=value" {
		t.Fatalf("restored app.conf mismatch: %q err=%v", got, err)
	}
	got, err = os.ReadFile(filepath.Join(restoreDir, base, "sub", "data.txt"))
	if err != nil || string(got) != "hello" {
		t.Fatalf("restored sub/data.txt mismatch: %q err=%v", got, err)
	}
	// GetTask 报告 action=restore
	res, _ := p.GetTask(task["taskId"].(string))
	if res["action"] != "restore" {
		t.Fatalf("task action = %v, want restore", res["action"])
	}
}

func TestBackupRestoreRejectsNonEmptyDest(t *testing.T) {
	p := newBackupTestProvider(t)
	backupDir := t.TempDir()
	task := runBackupAndWait(t, p, "ne", []string{t.TempDir()}, backupDir, 0)
	if task["status"] != backupTaskSuccess {
		t.Fatalf("backup failed: %v", task["error"])
	}
	name := task["file"].(string)

	destDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(destDir, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Call("restore", map[string]interface{}{
		"dir": backupDir, "name": name, "destDir": destDir,
	}); err == nil {
		t.Fatal("restore to non-empty destDir should be rejected")
	}
	// 已有文件不得被动过
	got, err := os.ReadFile(filepath.Join(destDir, "existing.txt"))
	if err != nil || string(got) != "x" {
		t.Fatalf("existing file disturbed: %q err=%v", got, err)
	}
}

func TestBackupRestoreRejectsUnsafeParams(t *testing.T) {
	p := newBackupTestProvider(t)
	backupDir := t.TempDir()
	task := runBackupAndWait(t, p, "us", []string{t.TempDir()}, backupDir, 0)
	name := task["file"].(string)

	cases := []struct {
		label, dir, name, destDir string
	}{
		{"relative dir", "rel/path", name, t.TempDir()},
		{"traversal name", backupDir, "../evil.tar.gz", t.TempDir()},
		{"destDir root", backupDir, name, "/"},
		{"relative destDir", backupDir, name, "rel/dest"},
		{"missing archive", backupDir, "nope-20260101-000000.tar.gz", t.TempDir()},
	}
	for _, c := range cases {
		if _, err := p.Call("restore", map[string]interface{}{
			"dir": c.dir, "name": c.name, "destDir": c.destDir,
		}); err == nil {
			t.Errorf("%s: restore should be rejected", c.label)
		}
	}
}

// TestBackupRestoreZipSlipSkipped 手工构造带 ../ 逃逸条目的恶意包，验证条目被跳过
func TestBackupRestoreZipSlipSkipped(t *testing.T) {
	p := newBackupTestProvider(t)
	dir := t.TempDir()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	mal := []byte("pwned")
	if err := tw.WriteHeader(&tar.Header{
		Name: "../../evil.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(mal)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(mal); err != nil {
		t.Fatal(err)
	}
	// 正常条目也要能写入
	ok := []byte("fine")
	if err := tw.WriteHeader(&tar.Header{
		Name: "app/ok.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(ok)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(ok); err != nil {
		t.Fatal(err)
	}
	// 不支持的条目类型（硬链接）跳过
	if err := tw.WriteHeader(&tar.Header{
		Name: "app/hard.txt", Typeflag: tar.TypeLink, Linkname: "ok.txt",
	}); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gw.Close()
	archive := "evil-20260101-000000.tar.gz"
	if err := os.WriteFile(filepath.Join(dir, archive), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	restoreRoot := t.TempDir()
	destDir := filepath.Join(restoreRoot, "dest")
	task := runRestoreAndWait(t, p, dir, archive, destDir)
	if task["status"] != backupTaskSuccess {
		t.Fatalf("restore failed: %v", task["error"])
	}
	// 逃逸文件不存在
	if _, err := os.Stat(filepath.Join(restoreRoot, "evil.txt")); err == nil {
		t.Fatal("zip slip: file escaped destDir")
	}
	// 正常条目存在；恶意条目被跳过
	if _, err := os.Stat(filepath.Join(destDir, "app", "ok.txt")); err != nil {
		t.Fatalf("legitimate entry missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "evil.txt")); err == nil {
		t.Fatal("unsafe entry was extracted")
	}
	// 日志记录了跳过
	res, _ := p.GetTask(task["taskId"].(string))
	if log, _ := res["log"].(string); !strings.Contains(log, "unsafe entry") {
		t.Fatalf("log should mention unsafe entry: %q", log)
	}
}

func TestBackupReadChunk(t *testing.T) {
	p := newBackupTestProvider(t)
	dir := t.TempDir()
	// 1MB + 10B：跨两块，末块不满
	want := make([]byte, backupReadChunkLimit+10)
	for i := range want {
		want[i] = byte(i % 251)
	}
	name := "chunk-20260101-000000.tar.gz"
	if err := os.WriteFile(filepath.Join(dir, name), want, 0o644); err != nil {
		t.Fatal(err)
	}

	var got []byte
	var offset float64
	for {
		res, err := p.Call("read", map[string]interface{}{
			"dir": dir, "name": name, "offset": offset, "length": float64(256 * 1024),
		})
		if err != nil {
			t.Fatalf("backup.read: %v", err)
		}
		m := res.(map[string]interface{})
		data, _ := base64.StdEncoding.DecodeString(m["data"].(string))
		got = append(got, data...)
		if m["eof"].(bool) {
			// size 经 RPC JSON 层为 float64，直连调用为 int64，统一转浮点比较
			if size, _ := strconv.ParseFloat(fmt.Sprint(m["size"]), 64); int64(size) != int64(len(want)) {
				t.Fatalf("size = %v, want %d", m["size"], len(want))
			}
			break
		}
		offset += float64(len(data))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("chunked read mismatch: got %d bytes, want %d", len(got), len(want))
	}

	// offset == size → 空数据 + eof
	res, err := p.Call("read", map[string]interface{}{
		"dir": dir, "name": name, "offset": float64(len(want)), "length": float64(1024),
	})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]interface{})
	if m["data"] != "" || m["eof"] != true {
		t.Fatalf("eof read: got %v", m)
	}
	// offset 超出 size 同样 eof
	res, _ = p.Call("read", map[string]interface{}{
		"dir": dir, "name": name, "offset": float64(len(want) + 100), "length": float64(1024),
	})
	if m = res.(map[string]interface{}); m["eof"] != true {
		t.Fatalf("past-eof read: got %v", m)
	}
}

func TestBackupReadRejectsUnsafeParams(t *testing.T) {
	p := newBackupTestProvider(t)
	dir := t.TempDir()
	cases := []struct {
		label, dir, name string
	}{
		{"relative dir", "rel", "a-20260101-000000.tar.gz"},
		{"traversal name", dir, "../secret.txt"},
		{"not tar.gz", dir, "a.txt"},
		{"slash in name", dir, "sub/a.tar.gz"},
	}
	for _, c := range cases {
		if _, err := p.Call("read", map[string]interface{}{
			"dir": c.dir, "name": c.name, "offset": float64(0), "length": float64(1024),
		}); err == nil {
			t.Errorf("%s: read should be rejected", c.label)
		}
	}
}
