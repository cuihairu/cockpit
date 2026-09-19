package rpc

// cov_round10_test.go 覆盖率零头：writeArchive 对 socket 等非常规文件的
// header 告警跳过；file Chmod/Chown 的系统调用失败；traefik loadSites 对
// glob 命中但读取失败的条目（悬空 symlink）跳过；omvSnapshot 对 smb 失败
// 与 nfs 坏数据的不致命继续。

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCovBackupArchiveSocketHeader unix socket 让 tar.FileInfoHeader 报
// 不支持类型 → 记 warn 后继续打包其余文件
func TestCovBackupArchiveSocketHeader(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "ok.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", filepath.Join(src, "sock"))
	if err != nil {
		t.Skipf("cannot create unix socket: %v", err)
	}
	defer ln.Close()

	var out strings.Builder
	task := &backupTask{Log: &cappedBuffer{limit: backupLogLimit}}
	if _, err := writeArchive(task, writerFunc(out.WriteString), []string{src}); err != nil {
		t.Fatalf("writeArchive: %v", err)
	}
}

// writerFunc 把 func(string) (int, error) 适配成 io.Writer
type writerFunc func(string) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(string(p)) }

// TestCovFileChmodChownSyscallFailures Chmod/Chown 的底层失败：procfs
// 不支持 chmod（root 也 EPERM）；非 root 改属主为他人 EPERM
func TestCovFileChmodChownSyscallFailures(t *testing.T) {
	p := NewFileProvider()

	// /proc/version 在 procfs 上，chmod 恒失败（Lstat 通过且非 symlink）
	if _, err := p.Chmod(map[string]interface{}{"path": "/proc/version", "mode": 0o755}); err == nil {
		t.Fatal("expect chmod failure on procfs")
	}

	// 非 root 进程把文件属主改成他人 → EPERM
	own := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(own, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root; chown EPERM path unreachable")
	}
	if _, err := p.Chown(map[string]interface{}{"path": own, "uid": 1, "gid": 1}); err == nil {
		t.Fatal("expect chown EPERM for non-root")
	}
}

// TestCovTraefikLoadSitesReadFailure glob 命中的片段是悬空 symlink →
// ReadFile 失败，跳过该条目不中断列表
func TestCovTraefikLoadSitesReadFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "cockpit-site-dead.yml")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	p := NewTraefikProvider(dir, nil)
	sites, err := p.loadSites()
	if err != nil {
		t.Fatalf("loadSites: %v", err)
	}
	if len(sites) != 0 {
		t.Fatalf("dead symlink should be skipped, got %+v", sites)
	}
}

// TestCovOMVShareListFailures omvSnapshot 的共享枚举：smb getShareList
// 报错不致命（continue），nfs 返回结构外数据 → decode 失败同样继续
func TestCovOMVShareListFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body["service"] {
		case "session":
			w.Write([]byte(`{"response":{"sessionid":"s"},"error":null}`))
		case "filesystemmgmt":
			w.Write([]byte(`{"response":[],"error":null}`))
		case "smb":
			w.Write([]byte(`{"response":{},"error":{"code":11,"message":"smb down"}}`))
		default: // nfs：数组无法解进 omvShareList 对象
			w.Write([]byte(`{"response":[1,2],"error":null}`))
		}
	}))
	defer srv.Close()

	target := NasTarget{Name: "omv-share", Type: "omv", Addr: srv.URL, Username: "u", Password: "p"}
	_, shares, err := omvSnapshot(context.Background(), target)
	if err != nil {
		t.Fatalf("share failures must not be fatal: %v", err)
	}
	if len(shares) != 0 {
		t.Fatalf("no shares expected, got %+v", shares)
	}
}
