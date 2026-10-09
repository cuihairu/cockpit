package rpc

// cov_io_errors_test.go 覆盖剩余 I/O 错误分支：备份读块 seek/read 失败、
// 截断归档的条目写失败、tar 收尾粘性错误、不可读源目录、悬空链接的
// 保留清理比较器，以及 cron 临时文件写失败（RLIMIT_FSIZE）与 logs 非法
// 载荷（NaN 无法 JSON 序列化）。

import (
	"archive/tar"
	"flag"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// fsizeWindowLimit 返回 fsize 窗口可安全设置的 RLIMIT_FSIZE 软限与原始
// rlimit（cur, old, ok）。go test 框架以非 append 模式持有 testlog.txt 的
// 写偏移，对进程内每次文件操作即时追加一行；窗口软限必须压过该偏移，
// 否则窗口内任何一次刷盘（bufio 满 4KB）或部分越限的短写都以 EFBIG /
// ErrShortWrite 粘住缓冲，进程收尾以 "testing: can't write testlog.txt"
// 报错整包 FAIL——CI 第 5-8 次复发真因（run 37934963613 / 37950067121 /
// 37952757780 / 37997368667）。os.Truncate 截 0 不复位偏移（截后首笔写
// 仍在原偏移落笔、稀洞补零），shrinkTestLog 截 0 治理因此无效（第 8 次
// 复发实证）。软限取日志当前大小 + 1MB 余量：覆盖未刷缓冲（≤4KB）与
// 窗口内框架增量（数百字节级），固定小软限（1KB/1MB）随包日志体量增长
// 必然复发。载荷取软限 +1MB 保证必撞限；硬限放不下时 ok=false 调用方
// Skip。丢失的缓存键条目无碍正确性。
func fsizeWindowLimit(t *testing.T) (cur uint64, old syscall.Rlimit, ok bool) {
	t.Helper()
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &old); err != nil {
		return 0, old, false
	}
	var logSize uint64
	if f := flag.Lookup("test.testlogfile"); f != nil {
		if path := f.Value.String(); path != "" {
			if st, err := os.Stat(path); err == nil {
				logSize = uint64(st.Size())
			}
		}
	}
	cur = logSize + 1<<20
	if cur < 1<<20 { // testlogfile 为空（无框架日志）时按 0 处理
		cur = 1 << 20
	}
	if old.Max != ^uint64(0) && cur > old.Max {
		return 0, old, false
	}
	return cur, old, true
}

// ============ backup.read 的 seek / read 失败 ============

func TestCovBackupReadChunkSeekAndReadErrors(t *testing.T) {
	p := newBackupTestProvider(t)
	dir := t.TempDir()

	// 负 offset：offset < total 通过，但 Seek(-1) 返回 EINVAL
	os.WriteFile(filepath.Join(dir, "real.tar.gz"), []byte("12345"), 0o644)
	if _, err := p.Call("read", map[string]interface{}{
		"dir": dir, "name": "real.tar.gz", "offset": float64(-1), "length": 4,
	}); err == nil || !strings.Contains(err.Error(), "seek:") {
		t.Errorf("negative offset err = %v", err)
	}

	// 指向目录的符号链接：Open/Stat/Seek 全部成功，ReadFull 读目录 fd
	// 得到 EISDIR → "read:" 错误分支
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "d.tar.gz")); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Call("read", map[string]interface{}{
		"dir": dir, "name": "d.tar.gz", "offset": 0, "length": 4,
	}); err == nil || !strings.Contains(err.Error(), "read:") {
		t.Errorf("dir-read err = %v", err)
	}
}

// ============ 截断归档：条目数据不完整 → io.Copy 失败（单条目告警） ============

func TestCovBackupUnpackTruncatedArchive(t *testing.T) {
	p := newBackupTestProvider(t)
	task := &backupTask{Log: &cappedBuffer{limit: 8192}}
	dir := t.TempDir()
	dest := t.TempDir()

	// 8KB 伪随机（不可压缩）数据：保证 tar 头之后仍有充足压缩数据，
	// 截掉尾部 200 字节只伤到条目内容，Next() 仍可读出头部
	rnd := rand.New(rand.NewSource(7)) //nolint:gosec
	data := make([]byte, 8192)
	rnd.Read(data)
	arch := filepath.Join(dir, "trunc.tar.gz")
	covWriteTarGz(t, arch, covTarEntry{name: "big.bin", typeflag: tar.TypeReg, mode: 0o644, data: string(data)})
	info, err := os.Stat(arch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(arch, info.Size()-200); err != nil {
		t.Fatal(err)
	}

	// 条目数据被截断：io.Copy 失败记 [warn] entry；随后下一次 Next()
	// 撞到流尾 → read archive 错误上抛（两个分支同测）
	if err := p.unpack(task, dir, "trunc.tar.gz", dest); err == nil ||
		!strings.Contains(err.Error(), "read archive") {
		t.Fatalf("unpack err = %v, want read archive", err)
	}
	if !strings.Contains(task.Log.String(), `entry "big.bin"`) {
		t.Errorf("truncated entry warn missing:\n%s", task.Log.String())
	}
}

// ============ writeArchive：大源中途写失败 → tw.Close 粘性错误 ============

func TestCovWriteArchiveFinalizeTarError(t *testing.T) {
	small := t.TempDir()
	os.WriteFile(filepath.Join(small, "a.txt"), []byte("payload"), 0o644)
	big := t.TempDir()
	blob := make([]byte, 256*1024)
	rnd := rand.New(rand.NewSource(11)) //nolint:gosec
	rnd.Read(blob)
	if err := os.WriteFile(filepath.Join(big, "blob.bin"), blob, 0o644); err != nil {
		t.Fatal(err)
	}

	// 测量小源的压缩总量，配额 = 小源 + 64KB：小源完整写入（written=1），
	// 大源 io.Copy 中途触底 → tar.Writer 粘住错误 → tw.Close 返回
	// "finalize tar" 而非 "no source"
	measure := &covCountWriter{}
	if _, err := writeArchive(&backupTask{Log: &cappedBuffer{limit: 1024}}, measure, []string{small}); err != nil {
		t.Fatalf("measure: %v", err)
	}
	qw := &covQuotaWriter{quota: measure.n + 64*1024}
	_, err := writeArchive(&backupTask{Log: &cappedBuffer{limit: 1024}}, qw, []string{small, big})
	if err == nil || !strings.Contains(err.Error(), "finalize tar") {
		t.Fatalf("err = %v (want finalize tar, measured %d bytes)", err, measure.n)
	}
}

// ============ addSource：源目录自身不可读 → 顶层 walk 错误上抛 ============

func TestCovAddSourceUnreadableRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644)
	if err := os.Chmod(src, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(src, 0o700) })
	// Lstat(src) 成功（权限只挡 readDir），Walk 顶层 readDir 失败 → 中断
	if _, err := addSource(tar.NewWriter(io.Discard), io.Discard, src); err == nil {
		t.Error("unreadable source root should abort addSource")
	}
}

// ============ pruneOldBackups：悬空符号链接 → 比较器 Stat 失败 ============

func TestCovPruneOldBackupsDanglingLink(t *testing.T) {
	p := newBackupTestProvider(t)
	task := &backupTask{Log: &cappedBuffer{limit: 4096}}
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "daily-real.tar.gz"), []byte("x"), 0o644)
	if err := os.Symlink("/nonexistent/daily-gone.tar.gz", filepath.Join(dir, "daily-gone.tar.gz")); err != nil {
		t.Fatal(err)
	}
	p.pruneOldBackups(task, dir, "daily", 1)
	// 悬空链接 Stat 失败 → 比较器退化为字符串序：gone < real，
	// 保留序首 gone（Remove 链接本身会成功），删除 real
	if _, err := os.Lstat(filepath.Join(dir, "daily-gone.tar.gz")); err != nil {
		t.Errorf("dangling link should survive as kept slot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "daily-real.tar.gz")); !os.IsNotExist(err) {
		t.Errorf("string-ordered prune should remove real file: %v", err)
	}
}

// ============ logs.query：载荷含 NaN → json.Marshal 失败 ============

func TestCovLogsQueryBadPayload(t *testing.T) {
	p := NewLogsProvider(nil)
	if _, err := p.Call("query", map[string]interface{}{
		"query": map[string]interface{}{"sinceMinutes": math.NaN()},
	}); err == nil || !strings.Contains(err.Error(), "bad query payload") {
		t.Errorf("NaN payload err = %v", err)
	}
}

// ============ cron：临时文件写入超过 RLIMIT_FSIZE → EFBIG ============

func TestCovCronWriteViaFileFsize(t *testing.T) {
	p := NewCronProvider(nil)

	// 软限压过框架 testlog 偏移（见 helper 注释），载荷再 +1MB 必撞限；
	// Go 运行时默认丢弃 SIGXFSZ，write 以错误返回
	cur, old, ok := fsizeWindowLimit(t)
	if !ok {
		t.Skipf("fsize 软限需求超出硬限 %d，无法安全收窄", old.Max)
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: cur, Max: old.Max}); err != nil {
		t.Skipf("setrlimit: %v", err)
	}
	err := p.writeViaFile(strings.Repeat("# cockpit cov fsize\n", int(cur/20)+(1<<20)/20), "")
	_ = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &old)
	if err == nil || !strings.Contains(err.Error(), "write temp file") {
		t.Fatalf("fsize err = %v", err)
	}
}
