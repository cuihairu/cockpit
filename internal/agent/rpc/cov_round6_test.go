package rpc

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// TestCovRandReadFailFallbackID 注入 randRead 失败，覆盖 backup/stack 默认
// newID 回退 UnixNano 的防御分支（go1.26 crypto/rand 实际不会失败）。
func TestCovRandReadFailFallbackID(t *testing.T) {
	orig := randRead
	t.Cleanup(func() { randRead = orig })
	randRead = func([]byte) (int, error) { return 0, errors.New("no rand") }

	bid := NewBackupProvider(BackupConfig{}).newID
	if id := bid(); !strings.HasPrefix(id, "b") {
		t.Errorf("backup fallback id = %q, want b-prefixed", id)
	} else if _, err := strconv.ParseInt(id[1:], 10, 64); err != nil {
		t.Errorf("backup fallback id = %q, want numeric tail", id)
	}

	sid := NewStackProvider(StackConfig{}).newID
	if id := sid(); !strings.HasPrefix(id, "t") {
		t.Errorf("stack fallback id = %q, want t-prefixed", id)
	} else if _, err := strconv.ParseInt(id[1:], 10, 64); err != nil {
		t.Errorf("stack fallback id = %q, want numeric tail", id)
	}
}

// TestCovCronUsersReadFileFail getent 不可用且 /etc/passwd 不可读：
// 覆盖 Users 的错误透传与 listCronUsers 兜底失败分支。
func TestCovCronUsersReadFileFail(t *testing.T) {
	origLook, origRead := cronLookPath, osReadFile
	t.Cleanup(func() { cronLookPath, osReadFile = origLook, origRead })
	cronLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	osReadFile = func(string) ([]byte, error) { return nil, errors.New("passwd unreadable") }

	p := &CronProvider{}
	if _, err := p.Users(); err == nil || !strings.Contains(err.Error(), "enumerate users") {
		t.Errorf("Users() err = %v, want enumerate users failure", err)
	}
}

// TestCovFileSearchDeadline 注入超短超时，覆盖搜索 deadline 截断分支。
func TestCovFileSearchDeadline(t *testing.T) {
	orig := fileSearchTimeout
	t.Cleanup(func() { fileSearchTimeout = orig })
	fileSearchTimeout = time.Nanosecond

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := NewFileProvider().Search(map[string]interface{}{"dir": dir, "query": "hello"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.(map[string]interface{})["truncated"] != true {
		t.Errorf("truncated = %v, want true under 1ns deadline", res)
	}
}

// TestCovFileSearchMissingRoot 根目录不存在：WalkDir 首个回调带 err，
// 覆盖 walkFn 的瞬时错误跳过分支（整体不报错、结果为空）。
func TestCovFileSearchMissingRoot(t *testing.T) {
	res, err := NewFileProvider().Search(map[string]interface{}{
		"dir": "/nonexistent-cov-search-root", "query": "x",
	})
	if err != nil {
		t.Fatalf("Search with missing root should not error: %v", err)
	}
	if matches := res.(map[string]interface{})["matches"]; len(matches.([]map[string]interface{})) != 0 {
		t.Errorf("matches = %v, want empty", matches)
	}
}

// TestCovFileStatOwnerMissing Lstat 失败返回 (-1,-1)。
func TestCovFileStatOwnerMissing(t *testing.T) {
	uid, gid := fileStatOwner("/nonexistent-cov-path")
	if uid != -1 || gid != -1 {
		t.Errorf("uid,gid = %d,%d, want -1,-1", uid, gid)
	}
}

// TestCovStartFollowCmdStdoutPipeFail 注入 exec.CommandContext 返回已占用
// Stdout 的 Cmd，覆盖 StdoutPipe 失败防御分支。
func TestCovStartFollowCmdStdoutPipeFail(t *testing.T) {
	orig := execCommandContext
	t.Cleanup(func() { execCommandContext = orig })
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		c := &exec.Cmd{}
		c.Stdout = os.Stdout // 预置 Stdout → StdoutPipe 报 "exec: Stdout already set"
		return c
	}

	_, _, err := startFollowCmd(context.Background(), &LogsQuery{Type: "systemd", Tail: 10, Source: "nginx"})
	if err == nil || !strings.Contains(err.Error(), "stdout pipe") {
		t.Errorf("err = %v, want stdout pipe failure", err)
	}
}

// TestCovAtomicWriteFileWriteAndChmodFail 注入文件写/权限原语失败，
// 覆盖原子写的两个防御分支（目标不得落盘）。
func TestCovAtomicWriteFileWriteAndChmodFail(t *testing.T) {
	origW, origC := fileWrite, fileChmod
	t.Cleanup(func() { fileWrite, fileChmod = origW, origC })
	path := filepath.Join(t.TempDir(), "svc.conf")

	fileWrite = func(*os.File, []byte) (int, error) { return 0, errors.New("write boom") }
	if err := atomicWriteFile(path, []byte("x"), 0o600); err == nil || !strings.Contains(err.Error(), "write boom") {
		t.Errorf("write fail err = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("target should not exist after write failure: %v", err)
	}

	fileWrite = origW // 写恢复，仅注入 chmod 失败
	fileChmod = func(*os.File, os.FileMode) error { return errors.New("chmod boom") }
	if err := atomicWriteFile(path, []byte("x"), 0o600); err == nil || !strings.Contains(err.Error(), "chmod boom") {
		t.Errorf("chmod fail err = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("target should not exist after chmod failure: %v", err)
	}
}

// TestCovReadChunkStatFail 注入 fileStat 失败，覆盖分块读取的 stat 防御分支。
func TestCovReadChunkStatFail(t *testing.T) {
	orig := fileStat
	t.Cleanup(func() { fileStat = orig })
	fileStat = func(*os.File) (os.FileInfo, error) { return nil, errors.New("stat boom") }

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app-20260101.tar.gz"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewBackupProvider(BackupConfig{}).ReadChunk(map[string]interface{}{
		"dir": dir, "name": "app-20260101.tar.gz", "offset": float64(0), "length": float64(10),
	})
	if err == nil || !strings.Contains(err.Error(), "stat backup file") {
		t.Errorf("err = %v, want stat backup file failure", err)
	}
}

// TestCovPackStatAndCloseFail 注入 fileStat/fileClose 失败，覆盖打包收尾
// 的两个防御分支（任务失败、半成品清理）。
func TestCovPackStatAndCloseFail(t *testing.T) {
	origS, origC := fileStat, fileClose
	t.Cleanup(func() { fileStat, fileClose = origS, origC })
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fileStat = func(*os.File) (os.FileInfo, error) { return nil, errors.New("stat boom") }
	task := runBackupAndWait(t, NewBackupProvider(BackupConfig{}), "covstat", []string{src}, t.TempDir(), 0)
	if task["status"] != backupTaskFailed {
		t.Fatalf("status = %v, want failed; log: %v", task["status"], task["log"])
	}
	if log := task["log"].(string); !strings.Contains(log, "stat file") {
		t.Errorf("log = %s, want stat file failure", log)
	}

	fileStat, fileClose = origS, func(*os.File) error { return errors.New("close boom") }
	task2 := runBackupAndWait(t, NewBackupProvider(BackupConfig{}), "covclose", []string{src}, t.TempDir(), 0)
	if task2["status"] != backupTaskFailed {
		t.Fatalf("status = %v, want failed; log: %v", task2["status"], task2["log"])
	}
	if log := task2["log"].(string); !strings.Contains(log, "close file") {
		t.Errorf("log = %s, want close file failure", log)
	}
}

// TestCovTraefikApplySiteSelfCheckFail 注入自检失败，覆盖渲染后自检防御分支。
func TestCovTraefikApplySiteSelfCheckFail(t *testing.T) {
	orig := traefikSelfCheck
	t.Cleanup(func() { traefikSelfCheck = orig })
	traefikSelfCheck = func([]byte) error { return errors.New("check boom") }

	p := &TraefikProvider{}
	_, err := p.ApplySite(&ProxySite{Name: "blog", ServerNames: []string{"blog.example.com"},
		Upstream: "127.0.0.1:3000", Scheme: "http"})
	if err == nil || !strings.Contains(err.Error(), "self-check") {
		t.Errorf("err = %v, want self-check failure", err)
	}
}

// TestCovDriftBaselineSaveMkdirFail 基线路径落在普通文件之下，
// 覆盖 save 的建目录失败分支。
func TestCovDriftBaselineSaveMkdirFail(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := NewDriftBaseline(filepath.Join(blocker, "baseline.json"))
	if err := b.save(baselineFile{}); err == nil || !strings.Contains(err.Error(), "create baseline dir") {
		t.Errorf("save err = %v, want create baseline dir failure", err)
	}
}

// TestCovStackPersistLastTaskMkdirFail stack 目录被同名文件占位：
// Stat 通过但 MkdirAll 失败，覆盖 last-task 落盘的防御分支。
func TestCovStackPersistLastTaskMkdirFail(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &StackProvider{dir: dir}
	p.persistLastTask("f", &stackTask{Action: "up", Status: "success", FinishedAt: time.Now()})
	// ENOTDIR 路径下 Stat 报 not a directory，同样说明未落盘
	if _, err := os.Stat(filepath.Join(dir, "f", stackReservedDir, "last-task.json")); err == nil {
		t.Error("last-task.json should not exist under file path")
	}
}

// TestCovHandleDotMethod "." 拆不出任何段：覆盖 parseMethod 的 ok=false
// 分支（修复前 parts[0] 会越界 panic）。
func TestCovHandleDotMethod(t *testing.T) {
	h := NewHandler()
	msg := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
		"method": ".",
	})
	if _, err := h.Handle(msg); err == nil || err.Error() != "invalid method" {
		t.Errorf("err = %v, want invalid method", err)
	}
}

// TestCovFileWriteStatFail 注入 fileStat 失败，覆盖写入收尾的 stat 防御分支。
func TestCovFileWriteStatFail(t *testing.T) {
	orig := fileStat
	t.Cleanup(func() { fileStat = orig })
	fileStat = func(*os.File) (os.FileInfo, error) { return nil, errors.New("stat boom") }

	path := filepath.Join(t.TempDir(), "f.txt")
	_, err := NewFileProvider().Write(map[string]interface{}{
		"path": path, "data": base64.StdEncoding.EncodeToString([]byte("x")), "truncate": true,
	})
	if err == nil || !strings.Contains(err.Error(), "stat: stat boom") {
		t.Errorf("err = %v, want stat: stat boom", err)
	}
}

// TestCovAtomicWriteFileCloseFail 注入 fileClose 失败，覆盖原子写收尾
// 的 close 防御分支（目标不得落盘）。
func TestCovAtomicWriteFileCloseFail(t *testing.T) {
	orig := fileClose
	t.Cleanup(func() { fileClose = orig })
	fileClose = func(*os.File) error { return errors.New("close boom") }

	path := filepath.Join(t.TempDir(), "svc.conf")
	if err := atomicWriteFile(path, []byte("x"), 0o600); err == nil || err.Error() != "close boom" {
		t.Errorf("err = %v, want close boom", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("target should not exist after close failure: %v", err)
	}
}

// TestCovBackupSymlinkReadlinkFail 注入 Readlink 失败，覆盖打包遍历中
// 断链 symlink 的跳过分支（任务本身仍成功）。
func TestCovBackupSymlinkReadlinkFail(t *testing.T) {
	orig := osReadlink
	t.Cleanup(func() { osReadlink = orig })
	osReadlink = func(string) (string, error) { return "", errors.New("dangling") }

	src := t.TempDir()
	if err := os.Symlink(filepath.Join(src, "absent"), filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	task := runBackupAndWait(t, NewBackupProvider(BackupConfig{}), "covlink", []string{src}, t.TempDir(), 0)
	if task["status"] != backupTaskSuccess {
		t.Fatalf("status = %v, want success; log: %v", task["status"], task["log"])
	}
}
