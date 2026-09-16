package rpc

// 覆盖率补充测试：backup_provider.go 错误分支与辅助函数。

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ============ 测试辅助 ============

// covTarEntry 自定义 tar 条目
type covTarEntry struct {
	name     string
	typeflag byte
	data     string
	linkname string
	mode     int64
}

// covWriteTarGz 按条目构造 tar.gz 文件
func covWriteTarGz(t *testing.T, path string, entries ...covTarEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Typeflag: e.typeflag, Mode: e.mode, Linkname: e.linkname, Size: int64(len(e.data))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if e.data != "" {
			if _, err := tw.Write([]byte(e.data)); err != nil {
				t.Fatal(err)
			}
		}
	}
	tw.Close()
	gz.Close()
}

// covCountWriter 记录总写入量（第一遍测量配额用）
type covCountWriter struct{ n int }

func (w *covCountWriter) Write(p []byte) (int, error) { w.n += len(p); return len(p), nil }

// covQuotaWriter 超过 quota 后报错
type covQuotaWriter struct{ quota, n int }

func (w *covQuotaWriter) Write(p []byte) (int, error) {
	if w.n+len(p) > w.quota {
		return 0, errors.New("quota exceeded")
	}
	w.n += len(p)
	return len(p), nil
}

// ============ 构造与任务查询 ============

func TestCovBackupTypeAndDefaultNewID(t *testing.T) {
	p := NewBackupProvider(BackupConfig{})
	if p.Type() != "backup" {
		t.Fatalf("type = %q", p.Type())
	}
	// 不注入 NewID：默认随机 ID（b+16hex）
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644)
	dest := t.TempDir()
	res, err := p.Call("run", map[string]interface{}{
		"name": "daily", "sources": toIfaceSlice([]string{src}), "destDir": dest,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	taskID := res.(map[string]interface{})["taskId"].(string)
	if !strings.HasPrefix(taskID, "b") || len(taskID) != 17 {
		t.Errorf("default task id = %q", taskID)
	}
	if task := waitBackupTask(t, p, taskID); task["status"] != backupTaskSuccess {
		t.Fatalf("task = %v", task)
	}
}

func TestCovBackupGetTaskAndUnknownAction(t *testing.T) {
	p := newBackupTestProvider(t)
	if _, err := p.Call("task.get", map[string]interface{}{}); err == nil ||
		!strings.Contains(err.Error(), "taskId required") {
		t.Errorf("empty taskId err = %v", err)
	}
	if _, err := p.Call("task.get", map[string]interface{}{"taskId": "b000"}); err == nil ||
		!strings.Contains(err.Error(), "task not found") {
		t.Errorf("missing task err = %v", err)
	}
	if _, err := p.Call("bogus", nil); err == nil || !strings.Contains(err.Error(), "unknown backup action") {
		t.Errorf("unknown action err = %v", err)
	}
}

// ============ run 校验与并发上限 ============

func TestCovBackupRunValidation(t *testing.T) {
	p := newBackupTestProvider(t)
	src := t.TempDir()

	if _, err := p.Call("run", map[string]interface{}{
		"name": "BAD", "sources": toIfaceSlice([]string{src}), "destDir": t.TempDir(),
	}); err == nil || !strings.Contains(err.Error(), "invalid backup name") {
		t.Errorf("invalid name err = %v", err)
	}
	if _, err := p.Call("run", map[string]interface{}{
		"name": "ok", "sources": toIfaceSlice([]string{src}), "destDir": "relative/dir",
	}); err == nil || !strings.Contains(err.Error(), "destDir must be absolute") {
		t.Errorf("relative destDir err = %v", err)
	}
	// sources 缺失 / 相对路径
	if _, err := p.Call("run", map[string]interface{}{"name": "ok", "destDir": t.TempDir()}); err == nil ||
		!strings.Contains(err.Error(), "sources required") {
		t.Errorf("missing sources err = %v", err)
	}
	if _, err := p.Call("run", map[string]interface{}{
		"name": "ok", "sources": toIfaceSlice([]string{"relative/src"}), "destDir": t.TempDir(),
	}); err == nil || !strings.Contains(err.Error(), "source must be absolute") {
		t.Errorf("relative source err = %v", err)
	}
	// 同名忙碌：预锁后立即拒绝
	lock, ok := p.nameLock("ok")
	if !ok {
		t.Fatal("pre-lock should succeed")
	}
	if _, err := p.Call("run", map[string]interface{}{
		"name": "ok", "sources": toIfaceSlice([]string{src}), "destDir": t.TempDir(),
	}); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Errorf("busy err = %v", err)
	}
	lock.Unlock()

	// 并发上限占满 → too many（直接占住信号量，确定性）
	p2 := NewBackupProvider(BackupConfig{MaxTasks: 1})
	p2.sem <- struct{}{}
	if _, err := p2.Call("run", map[string]interface{}{
		"name": "ok", "sources": toIfaceSlice([]string{src}), "destDir": t.TempDir(),
	}); err == nil || !strings.Contains(err.Error(), "too many concurrent") {
		t.Errorf("semaphore err = %v", err)
	}
	<-p2.sem
}

// ============ list / delete ============

func TestCovBackupListFiles(t *testing.T) {
	p := newBackupTestProvider(t)
	// 目录不存在
	if _, err := p.Call("list", map[string]interface{}{"dir": filepath.Join(t.TempDir(), "missing")}); err == nil ||
		!strings.Contains(err.Error(), "read dir") {
		t.Errorf("list missing err = %v", err)
	}
	if _, err := p.Call("list", map[string]interface{}{"dir": "rel"}); err == nil ||
		!strings.Contains(err.Error(), "dir must be absolute") {
		t.Errorf("relative dir err = %v", err)
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.tar.gz"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644)
	os.Mkdir(filepath.Join(dir, "adir.tar.gz"), 0o755)
	// fifo：非目录、后缀匹配、但非普通文件 → 跳过
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe.tar.gz"), 0o644); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	res, err := p.Call("list", map[string]interface{}{"dir": dir})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	files := res.(map[string]interface{})["files"].([]BackupFile)
	if len(files) != 1 || files[0].Name != "keep.tar.gz" {
		t.Fatalf("files = %+v", files)
	}
}

func TestCovBackupDeleteFile(t *testing.T) {
	p := newBackupTestProvider(t)
	dir := t.TempDir()
	if _, err := p.Call("delete", map[string]interface{}{"dir": dir, "name": "../evil"}); err == nil ||
		!strings.Contains(err.Error(), "invalid backup file name") {
		t.Errorf("invalid name err = %v", err)
	}
	// 文件不存在 → remove err
	if _, err := p.Call("delete", map[string]interface{}{"dir": dir, "name": "ghost.tar.gz"}); err == nil ||
		!strings.Contains(err.Error(), "remove ghost.tar.gz") {
		t.Errorf("remove missing err = %v", err)
	}
	os.WriteFile(filepath.Join(dir, "real.tar.gz"), []byte("x"), 0o644)
	if _, err := p.Call("delete", map[string]interface{}{"dir": dir, "name": "real.tar.gz"}); err != nil {
		t.Errorf("delete: %v", err)
	}
}

// ============ restore 校验 ============

func TestCovBackupRestoreValidation(t *testing.T) {
	p := newBackupTestProvider(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.tar.gz"), []byte("x"), 0o644)

	cases := []struct {
		desc   string
		params map[string]interface{}
		want   string
	}{
		{"relative dir", map[string]interface{}{"dir": "rel", "name": "a.tar.gz", "destDir": t.TempDir()}, "dir must be absolute"},
		{"bad name", map[string]interface{}{"dir": dir, "name": "x.txt", "destDir": t.TempDir()}, "invalid backup file name"},
		{"relative destDir", map[string]interface{}{"dir": dir, "name": "a.tar.gz", "destDir": "rel"}, "destDir must be an absolute path"},
		{"root destDir", map[string]interface{}{"dir": dir, "name": "a.tar.gz", "destDir": "/"}, "destDir must be an absolute path"},
		{"missing archive", map[string]interface{}{"dir": dir, "name": "b.tar.gz", "destDir": t.TempDir()}, "backup file not found"},
		{"nonempty destDir", map[string]interface{}{"dir": dir, "name": "a.tar.gz", "destDir": dir}, "not empty"},
	}
	for _, c := range cases {
		if _, err := p.Call("restore", c.params); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s err = %v", c.desc, err)
		}
	}

	// archive 是目录 → 非普通文件
	d2 := t.TempDir()
	os.Mkdir(filepath.Join(d2, "d.tar.gz"), 0o755)
	if _, err := p.Call("restore", map[string]interface{}{
		"dir": d2, "name": "d.tar.gz", "destDir": filepath.Join(d2, "out"),
	}); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("dir archive err = %v", err)
	}

	// 同名恢复忙碌：预锁 restore: 前缀
	lock, ok := p.nameLock("restore:a.tar.gz")
	if !ok {
		t.Fatal("pre-lock should succeed")
	}
	if _, err := p.Call("restore", map[string]interface{}{
		"dir": dir, "name": "a.tar.gz", "destDir": filepath.Join(t.TempDir(), "out"),
	}); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Errorf("busy err = %v", err)
	}
	lock.Unlock()

	// 信号量占满 → too many
	p2 := NewBackupProvider(BackupConfig{MaxTasks: 1})
	p2.sem <- struct{}{}
	if _, err := p2.Call("restore", map[string]interface{}{
		"dir": dir, "name": "a.tar.gz", "destDir": filepath.Join(t.TempDir(), "out"),
	}); err == nil || !strings.Contains(err.Error(), "too many concurrent") {
		t.Errorf("semaphore err = %v", err)
	}
	<-p2.sem
}

// ============ unpack 分支 ============

func TestCovBackupUnpackDirectErrors(t *testing.T) {
	p := newBackupTestProvider(t)
	task := &backupTask{Log: &cappedBuffer{limit: 4096}}
	dir := t.TempDir()
	dest := filepath.Join(t.TempDir(), "out")

	// 打不开归档
	if err := p.unpack(task, dir, "missing.tar.gz", dest); err == nil ||
		!strings.Contains(err.Error(), "open archive") {
		t.Errorf("open err = %v", err)
	}
	// 非 gzip 内容
	plain := filepath.Join(dir, "plain.tar.gz")
	os.WriteFile(plain, []byte("this is not gzip"), 0o644)
	if err := p.unpack(task, dir, "plain.tar.gz", dest); err == nil ||
		!strings.Contains(err.Error(), "gzip") {
		t.Errorf("gzip err = %v", err)
	}
	// destDir 路径中段是文件 → MkdirAll 失败
	blocker := filepath.Join(t.TempDir(), "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)
	good := filepath.Join(dir, "good.tar.gz")
	covWriteTarGz(t, good, covTarEntry{name: "a.txt", typeflag: tar.TypeReg, data: "x", mode: 0o644})
	if err := p.unpack(task, dir, "good.tar.gz", filepath.Join(blocker, "out")); err == nil ||
		!strings.Contains(err.Error(), "create dest dir") {
		t.Errorf("mkdir err = %v", err)
	}
	// gzip 合法但 tar 流损坏
	badTar := filepath.Join(dir, "badtar.tar.gz")
	f, _ := os.Create(badTar)
	gz := gzip.NewWriter(f)
	gz.Write([]byte("garbage that is not a tar stream at all"))
	gz.Close()
	f.Close()
	if err := p.unpack(task, dir, "badtar.tar.gz", dest); err == nil ||
		!strings.Contains(err.Error(), "read archive") {
		t.Errorf("tar err = %v", err)
	}
}

func TestCovBackupUnpackEntryTypes(t *testing.T) {
	p := newBackupTestProvider(t)
	task := &backupTask{Log: &cappedBuffer{limit: 8192}}
	dir := t.TempDir()
	dest := t.TempDir()

	// 条目组合：目录名与后续文件同名（OpenFile 目录 → 失败 warn）、
	// symlink、不支持类型、越界路径
	arch := filepath.Join(dir, "mixed.tar.gz")
	covWriteTarGz(t, arch,
		covTarEntry{name: "app", typeflag: tar.TypeDir, mode: 0o755},
		covTarEntry{name: "app", typeflag: tar.TypeReg, data: "clash", mode: 0o644},
		covTarEntry{name: "link", typeflag: tar.TypeSymlink, linkname: "app", mode: 0o777},
		covTarEntry{name: "dev", typeflag: tar.TypeChar, mode: 0o644},
		covTarEntry{name: "../evil", typeflag: tar.TypeReg, data: "x", mode: 0o644},
		covTarEntry{name: "ok.txt", typeflag: tar.TypeReg, data: "fine", mode: 0o644},
	)
	if err := p.unpack(task, dir, "mixed.tar.gz", dest); err != nil {
		t.Fatalf("unpack: %v", err)
	}
	log := task.Log.String()
	for _, want := range []string{"entry \"app\"", "unsupported entry type", "skip unsafe entry"} {
		if !strings.Contains(log, want) {
			t.Errorf("log missing %q:\n%s", want, log)
		}
	}
	// symlink 条目落地
	link, err := os.Readlink(filepath.Join(dest, "link"))
	if err != nil || link != "app" {
		t.Errorf("symlink = %q %v", link, err)
	}
	// 正常条目恢复
	b, err := os.ReadFile(filepath.Join(dest, "ok.txt"))
	if err != nil || string(b) != "fine" {
		t.Errorf("ok.txt = %q %v", b, err)
	}
}

func TestCovBackupRestoreCorruptArchive(t *testing.T) {
	p := newBackupTestProvider(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.tar.gz"), []byte("definitely not gzip"), 0o644)
	task := runRestoreAndWait(t, p, dir, "bad.tar.gz", filepath.Join(t.TempDir(), "out"))
	if task["status"] != backupTaskFailed || !strings.Contains(task["error"].(string), "gzip") {
		t.Fatalf("task = %v", task)
	}
}

// ============ read ============

func TestCovBackupReadChunkOpenError(t *testing.T) {
	p := newBackupTestProvider(t)
	if _, err := p.Call("read", map[string]interface{}{
		"dir": t.TempDir(), "name": "ghost.tar.gz", "offset": 0, "length": 10,
	}); err == nil || !strings.Contains(err.Error(), "open backup file") {
		t.Errorf("open err = %v", err)
	}
	// 参数校验
	if _, err := p.Call("read", map[string]interface{}{
		"dir": "rel", "name": "a.tar.gz",
	}); err == nil || !strings.Contains(err.Error(), "dir must be absolute") {
		t.Errorf("relative dir err = %v", err)
	}
	if _, err := p.Call("read", map[string]interface{}{
		"dir": t.TempDir(), "name": "a.txt",
	}); err == nil || !strings.Contains(err.Error(), "invalid backup file name") {
		t.Errorf("bad name err = %v", err)
	}
}

// ============ pack / writeArchive / addSource ============

func TestCovBackupPackFailures(t *testing.T) {
	// destDir 路径中段是文件 → 任务失败
	p := newBackupTestProvider(t)
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644)
	blocker := filepath.Join(t.TempDir(), "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)
	task := runBackupAndWait(t, p, "daily", []string{src}, filepath.Join(blocker, "out"), 0)
	if task["status"] != backupTaskFailed || !strings.Contains(task["error"].(string), "create dest dir") {
		t.Fatalf("task = %v", task)
	}

	// 固定时钟：二次同名备份 → already exists
	fixed := time.Unix(1700000000, 0)
	p2 := NewBackupProvider(BackupConfig{Now: func() time.Time { return fixed }})
	dest := t.TempDir()
	if task := runBackupAndWait(t, p2, "daily", []string{src}, dest, 0); task["status"] != backupTaskSuccess {
		t.Fatalf("first = %v", task)
	}
	task = runBackupAndWait(t, p2, "daily", []string{src}, dest, 0)
	if task["status"] != backupTaskFailed || !strings.Contains(task["error"].(string), "already exists") {
		t.Fatalf("second = %v", task)
	}

	// 全部源失败 → no source could be archived，且半成品被删除
	p3 := newBackupTestProvider(t)
	dest3 := t.TempDir()
	task = runBackupAndWait(t, p3, "daily", []string{filepath.Join(t.TempDir(), "missing")}, dest3, 0)
	if task["status"] != backupTaskFailed ||
		!strings.Contains(task["error"].(string), "no source could be archived") {
		t.Fatalf("task = %v", task)
	}
	leftover, _ := filepath.Glob(filepath.Join(dest3, "*.tar.gz"))
	if len(leftover) != 0 {
		t.Errorf("half-written archive must be removed: %v", leftover)
	}

	// destDir 只读 → Create 失败（非 root）
	if os.Geteuid() != 0 {
		p4 := newBackupTestProvider(t)
		ro := t.TempDir()
		os.Chmod(ro, 0o500)
		t.Cleanup(func() { os.Chmod(ro, 0o700) })
		task = runBackupAndWait(t, p4, "daily", []string{src}, ro, 0)
		if task["status"] != backupTaskFailed || !strings.Contains(task["error"].(string), "create file") {
			t.Fatalf("readonly dest task = %v", task)
		}
	}
}

func TestCovWriteArchiveNoSourceAndGzipClose(t *testing.T) {
	task := &backupTask{Log: &cappedBuffer{limit: 4096}}

	// 全部源失败 → no source
	if _, err := writeArchive(task, io.Discard, []string{"/nonexistent/source"}); err == nil ||
		!strings.Contains(err.Error(), "no source could be archived") {
		t.Errorf("no source err = %v", err)
	}

	// 配额 writer：先测量总量再卡掉最后一个字节 → gz.Close 失败
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("payload"), 0o644)
	measure := &covCountWriter{}
	if _, err := writeArchive(&backupTask{Log: &cappedBuffer{limit: 1024}}, measure, []string{src}); err != nil {
		t.Fatalf("measure: %v", err)
	}
	qw := &covQuotaWriter{quota: measure.n - 1}
	_, err := writeArchive(&backupTask{Log: &cappedBuffer{limit: 1024}}, qw, []string{src})
	if err == nil || (!strings.Contains(err.Error(), "finalize gzip") && !strings.Contains(err.Error(), "finalize tar")) {
		t.Errorf("quota err = %v (measured %d bytes)", err, measure.n)
	}
}

func TestCovAddSourceBranches(t *testing.T) {
	tw := tar.NewWriter(io.Discard)

	// 相对路径
	if _, err := addSource(tw, io.Discard, "relative"); err == nil ||
		!strings.Contains(err.Error(), "source must be absolute") {
		t.Errorf("relative err = %v", err)
	}
	// 顶层不存在
	if _, err := addSource(tw, io.Discard, filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing source should fail")
	}

	// 子目录不可读 → walk 子项报错但继续（非 root）
	if os.Geteuid() != 0 {
		src := t.TempDir()
		os.WriteFile(filepath.Join(src, "top.txt"), []byte("x"), 0o644)
		sub := filepath.Join(src, "sub")
		os.Mkdir(sub, 0o700)
		os.WriteFile(filepath.Join(sub, "in.txt"), []byte("y"), 0o644)
		os.Chmod(sub, 0o000)
		t.Cleanup(func() { os.Chmod(sub, 0o700) })
		n, err := addSource(tar.NewWriter(io.Discard), io.Discard, src)
		if err != nil {
			t.Fatalf("unreadable child: %v", err)
		}
		if n < 1 {
			t.Errorf("entries = %d, top entry should still be written", n)
		}

		// 普通文件无读权限 → open 失败仅告警，条目头仍写入
		src2 := t.TempDir()
		secret := filepath.Join(src2, "secret.txt")
		os.WriteFile(secret, []byte("y"), 0o644)
		os.Chmod(secret, 0o000)
		t.Cleanup(func() { os.Chmod(secret, 0o644) })
		if _, err := addSource(tar.NewWriter(io.Discard), io.Discard, src2); err != nil {
			t.Fatalf("unreadable file: %v", err)
		}
	}

	// tar writer 底层失败 → WriteHeader err 上抛
	failTw := tar.NewWriter(&covFailWriter{})
	src3 := t.TempDir()
	os.WriteFile(filepath.Join(src3, "a.txt"), []byte("x"), 0o644)
	if _, err := addSource(failTw, io.Discard, src3); err == nil {
		t.Error("failing tar writer should surface WriteHeader error")
	}
}

// covFailWriter 永远写失败
type covFailWriter struct{}

func (w *covFailWriter) Write(p []byte) (int, error) { return 0, errors.New("write failed") }

// ============ prune ============

func TestCovPruneOldBackups(t *testing.T) {
	p := newBackupTestProvider(t)
	task := &backupTask{Log: &cappedBuffer{limit: 4096}}
	dir := t.TempDir()

	// 坏 pattern → 直接返回
	p.pruneOldBackups(task, dir, "a[b", 1)

	// 数量不超 retention → 返回
	os.WriteFile(filepath.Join(dir, "one.tar.gz"), []byte("x"), 0o644)
	p.pruneOldBackups(task, dir, "one", 5)

	// 匹配到非空目录 → Remove 失败 → warn
	os.Mkdir(filepath.Join(dir, "daily-1.tar.gz"), 0o700)
	os.WriteFile(filepath.Join(dir, "daily-1.tar.gz", "inner"), []byte("x"), 0o644)
	p.pruneOldBackups(task, dir, "daily", 0)
	if !strings.Contains(task.Log.String(), "[warn] prune") {
		t.Errorf("prune warn missing: %s", task.Log.String())
	}

	// 正常清理：保留最新
	os.RemoveAll(filepath.Join(dir, "daily-1.tar.gz"))
	old := filepath.Join(dir, "daily-old.tar.gz")
	newF := filepath.Join(dir, "daily-new.tar.gz")
	os.WriteFile(old, []byte("x"), 0o644)
	os.WriteFile(newF, []byte("y"), 0o644)
	past := time.Now().Add(-2 * time.Hour)
	os.Chtimes(old, past, past)
	p.pruneOldBackups(task, dir, "daily", 1)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old backup should be pruned: %v", err)
	}
	if _, err := os.Stat(newF); err != nil {
		t.Errorf("new backup must survive: %v", err)
	}
}

// ============ helpers：nameLock / pruneTasksLocked ============

func TestCovBackupNameLockNilMap(t *testing.T) {
	p := newBackupTestProvider(t)
	p.locks = nil
	lock, ok := p.nameLock("x")
	if !ok || lock == nil {
		t.Fatalf("nameLock with nil map = %v %v", lock, ok)
	}
	if _, again := p.nameLock("x"); again {
		t.Error("second nameLock on same name should fail")
	}
	lock.Unlock()
}

func TestCovBackupPruneTasksLocked(t *testing.T) {
	p := newBackupTestProvider(t)
	running := &backupTask{ID: "running", Status: backupTaskRunning}
	p.tasks["running"] = running
	for i := 0; i < 55; i++ {
		id := fmt.Sprintf("t%02d", i)
		p.tasks[id] = &backupTask{ID: id, Status: backupTaskSuccess, FinishedAt: time.Unix(int64(i), 0)}
	}
	p.mu.Lock()
	p.pruneTasksLocked()
	p.mu.Unlock()
	if len(p.tasks) > 50 {
		t.Errorf("tasks = %d, want <= 50", len(p.tasks))
	}
	if _, ok := p.tasks["running"]; !ok {
		t.Error("running task must never be pruned")
	}
	// 任务数不足阈值 → 直接返回
	p2 := newBackupTestProvider(t)
	p2.mu.Lock()
	p2.pruneTasksLocked()
	p2.mu.Unlock()
}
