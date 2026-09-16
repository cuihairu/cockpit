package rpc

// 覆盖率补充测试：收尾零散未覆盖分支。

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCovNewDockerProviderSuccess(t *testing.T) {
	// 假 docker daemon：/_ping 返回 200 → client 建立成功
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.Header().Set("Api-Version", "1.43")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	p, err := NewDockerProvider(srv.URL)
	if err != nil {
		t.Fatalf("NewDockerProvider: %v", err)
	}
	if p.Type() != "docker" {
		t.Errorf("type = %q", p.Type())
	}
}

func TestCovSystemCallInfo(t *testing.T) {
	p := NewSystemProvider()
	res, err := p.Call("info", nil)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if res == nil {
		t.Error("info result should not be nil")
	}
}

func TestCovStackGetStackFileInvalidName(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	if _, err := p.GetStackFile("../bad"); err == nil {
		t.Error("invalid stack name should fail")
	}
	if _, err := p.Call("file.get", map[string]interface{}{"name": "../bad"}); err == nil {
		t.Error("file.get with invalid name should fail")
	}
}

func TestCovStackRemoveDirFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	// stack 目录只读且非空 → RemoveAll 失败 → remove 任务失败
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	dir := filepath.Join(p.dir, "stuck")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("x"), 0o644)
	os.Chmod(dir, 0o500)
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	resp, _ := p.Call("remove", map[string]interface{}{"name": "stuck"})
	task := waitForTask(t, p, resp.(map[string]interface{})["taskId"].(string))
	if task["status"] != stackTaskFailed || !strings.Contains(task["log"].(string), "remove stack dir") {
		t.Fatalf("task = %v log=%v", task["status"], task["log"])
	}
}

func TestCovFileRelativePathAndNegativeOffset(t *testing.T) {
	p := newFileTestProvider()
	// Write 相对路径
	if _, err := p.Write(map[string]interface{}{"path": "rel.txt", "data": "eA==", "truncate": true}); err == nil {
		t.Error("relative write should fail")
	}
	// Rename 相对路径
	if _, err := p.Rename("rel.txt", "new.txt"); err == nil {
		t.Error("relative rename should fail")
	}
	// Read 负 offset → Seek 报错
	f := filepath.Join(t.TempDir(), "data.bin")
	mustWrite(t, f, "hello")
	if _, err := p.Read(map[string]interface{}{"path": f, "offset": float64(-1)}); err == nil ||
		!strings.Contains(err.Error(), "seek:") {
		t.Errorf("negative offset err = %v", err)
	}
}

func TestCovFileWriteDevFull(t *testing.T) {
	// /dev/full：写入必得 ENOSPC → f.Write 错误分支
	if fi, err := os.Lstat("/dev/full"); err != nil || fi.Mode()&os.ModeDevice == 0 {
		t.Skip("/dev/full unavailable")
	}
	p := newFileTestProvider()
	if _, err := p.Write(map[string]interface{}{"path": "/dev/full", "data": "aGVsbG8=", "truncate": true}); err == nil ||
		!strings.Contains(err.Error(), "write:") {
		t.Errorf("/dev/full write err = %v", err)
	}
}

func TestCovDriftCallCheck(t *testing.T) {
	base := filepath.Join(t.TempDir(), "bl")
	os.MkdirAll(base, 0o700)
	b := NewDriftBaseline(filepath.Join(base, "baseline.json"))
	p := NewDriftProvider(b, DriftConfig{ConfDir: t.TempDir(), StacksDir: t.TempDir()})
	res, err := p.Call("check", nil)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if res == nil {
		t.Error("check result should not be nil")
	}
}

func TestCovBackupUnpackMkdirErrors(t *testing.T) {
	p := newBackupTestProvider(t)
	task := &backupTask{Log: &cappedBuffer{limit: 8192}}
	dir := t.TempDir()
	dest := t.TempDir()

	// 先落普通文件 f/g，再用子路径条目触发 MkdirAll ENOTDIR：
	// - TypeReg "f/inner.txt" → unpackEntry MkdirAll(Dir) 失败
	// - TypeSymlink "g/link" → symlink 分支 MkdirAll(Dir) 失败
	arch := filepath.Join(dir, "nested.tar.gz")
	covWriteTarGz(t, arch,
		covTarEntry{name: "f", typeflag: tar.TypeReg, data: "x", mode: 0o644},
		covTarEntry{name: "g", typeflag: tar.TypeReg, data: "y", mode: 0o644},
		covTarEntry{name: "f/inner.txt", typeflag: tar.TypeReg, data: "z", mode: 0o644},
		covTarEntry{name: "g/link", typeflag: tar.TypeSymlink, linkname: "f", mode: 0o777},
	)
	if err := p.unpack(task, dir, "nested.tar.gz", dest); err != nil {
		t.Fatalf("unpack: %v", err)
	}
	log := task.Log.String()
	if !strings.Contains(log, `entry "f/inner.txt"`) {
		t.Errorf("reg mkdir err missing:\n%s", log)
	}
	if !strings.Contains(log, `entry "g/link"`) {
		t.Errorf("symlink mkdir err missing:\n%s", log)
	}
}

func TestCovAddSourceCopyError(t *testing.T) {
	// 256KB 伪随机（不可压缩）源：flate 持续向底层写；配额卡在数据阶段 →
	// io.Copy(tw, f) 失败上抛
	src := t.TempDir()
	big := make([]byte, 256*1024)
	rng := rand.New(rand.NewSource(42)) //nolint:gosec
	rng.Read(big)
	if err := os.WriteFile(filepath.Join(src, "blob.bin"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	measure := &covCountWriter{}
	mgz := gzip.NewWriter(measure)
	mtw := tar.NewWriter(mgz)
	if _, err := addSource(mtw, io.Discard, src); err != nil {
		t.Fatalf("measure addSource: %v", err)
	}
	mtw.Close()
	mgz.Close()

	qw := &covQuotaWriter{quota: measure.n - 64*1024}
	qgz := gzip.NewWriter(qw)
	qtw := tar.NewWriter(qgz)
	if _, err := addSource(qtw, io.Discard, src); err == nil {
		t.Fatal("quota-limited addSource should fail during io.Copy")
	}
}
