package rpc

// 覆盖率补充测试：stack_provider.go 错误分支与默认值路径。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/docker"
)

func TestCovStackTypeAndDefaults(t *testing.T) {
	p := NewStackProvider(StackConfig{})
	if p.Type() != "stack" {
		t.Fatalf("type = %q", p.Type())
	}
	if p.dir != "/var/lib/cockpit/stacks" {
		t.Errorf("default dir = %q", p.dir)
	}
	if len(p.composeBin) != 2 || p.composeBin[0] != "docker" {
		t.Errorf("default composeBin = %v", p.composeBin)
	}
}

func TestCovComposeAvailableNoDocker(t *testing.T) {
	// PATH 指向空目录 → 找不到 docker → false（确定性）
	t.Setenv("PATH", t.TempDir())
	if ComposeAvailable() {
		t.Error("ComposeAvailable should be false without docker in PATH")
	}
}

func TestCovStackDefaultNewID(t *testing.T) {
	// 不注入 NewID：launchTask 走默认随机 ID 生成
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "web"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "web", "compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewStackProvider(StackConfig{
		Dir:        root,
		ComposeBin: []string{"/bin/sh", "-c", "exit 0"},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	})
	resp, err := p.Call("up", map[string]interface{}{"name": "web"})
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	taskID := resp.(map[string]interface{})["taskId"].(string)
	if !strings.HasPrefix(taskID, "t") || len(taskID) != 17 { // "t" + 16 hex
		t.Errorf("default task id = %q, want t+16hex", taskID)
	}
	waitForTask(t, p, taskID)
}

func TestCovCappedBufferTruncation(t *testing.T) {
	b := &cappedBuffer{limit: 8}
	if n, _ := b.Write([]byte("12345678")); n != 8 {
		t.Fatalf("write = %d", n)
	}
	b.Write([]byte("9abcdefg"))
	if got := b.String(); got != "9abcdefg" {
		t.Errorf("after overflow buf = %q, want last 8 bytes", got)
	}
	// 一次性超限写入同样只留尾部
	b2 := &cappedBuffer{limit: 4}
	b2.Write([]byte("XXXXXXXXXX"))
	if b2.String() != "XXXX" {
		t.Errorf("b2 = %q", b2.String())
	}
}

func TestCovStackListDirIsFile(t *testing.T) {
	// stacks 根路径是普通文件 → ReadDir 报错（非 NotExist）
	f := filepath.Join(t.TempDir(), "notadir")
	os.WriteFile(f, []byte("x"), 0o644)
	p := NewStackProvider(StackConfig{Dir: f})
	if _, err := p.ListStacks(); err == nil {
		t.Error("list with file as root should fail")
	}
}

func TestCovStackStatusDockerError(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "web"), 0o700)
	os.WriteFile(filepath.Join(root, "web", "compose.yml"), []byte("services: {}\n"), 0o644)
	p := NewStackProvider(StackConfig{Dir: root, Docker: &mockStackDocker{err: errFakeDocker}})
	if _, err := p.StatusStack("web"); err == nil {
		t.Error("status with docker error should fail")
	}
}

// errFakeDocker 包级占位错误（mockStackDocker.err 用）
var errFakeDocker = &covFakeError{}

type covFakeError struct{}

func (e *covFakeError) Error() string { return "fake docker error" }

func TestCovStackServiceFallsBackToContainerName(t *testing.T) {
	// 无 compose service label → 服务名退化为容器名（firstNonEmpty else 分支）
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "web"), 0o700)
	os.WriteFile(filepath.Join(root, "web", "compose.yml"), []byte("services: {}\n"), 0o644)
	containers := []docker.ContainerInfo{
		{ID: "c1", Name: "web-container-1", Image: "nginx", State: "running",
			Labels: map[string]string{"com.docker.compose.project": "web"}},
	}
	p := NewStackProvider(StackConfig{Dir: root, Docker: &mockStackDocker{containers: containers}})
	resp, err := p.StatusStack("web")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	svc := resp.Services
	if len(svc) != 1 || svc[0].Name != "web-container-1" {
		t.Fatalf("service name fallback = %+v", svc)
	}
}

func TestCovStackReadLastTaskCorrupted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".cockpit")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "last-task.json"), []byte("{not json"), 0o644)
	if lt := readLastTask(dir); lt != nil {
		t.Errorf("corrupted last-task should return nil, got %+v", lt)
	}
}

func TestCovStackPersistLastTaskErrors(t *testing.T) {
	p := NewStackProvider(StackConfig{Dir: t.TempDir()})
	task := &stackTask{Action: "up", Status: stackTaskSuccess, FinishedAt: time.Unix(1, 0)}

	// stack 目录不存在 → 直接返回
	p.persistLastTask("ghost", task)

	// stack 目录存在但不可写（非 root 下 MkdirAll 失败）
	if os.Geteuid() != 0 {
		stackDir := filepath.Join(p.dir, "ro")
		os.MkdirAll(stackDir, 0o500)
		p.persistLastTask("ro", task)
		// 未落盘即视为走了失败分支
		if _, err := os.Stat(filepath.Join(stackDir, stackReservedDir, "last-task.json")); !os.IsNotExist(err) {
			t.Error("readonly stack dir should not persist last-task")
		}
	}

	// .cockpit 已存在但 last-task.json 路径是目录 → WriteFile 失败
	stackDir := filepath.Join(p.dir, "wd")
	cockpitDir := filepath.Join(stackDir, stackReservedDir)
	os.MkdirAll(cockpitDir, 0o700)
	os.MkdirAll(filepath.Join(cockpitDir, "last-task.json"), 0o700)
	p.persistLastTask("wd", task) // 不 panic 即可

	// 正常路径
	os.MkdirAll(filepath.Join(p.dir, "ok"), 0o700)
	p.persistLastTask("ok", task)
	if lt := readLastTask(filepath.Join(p.dir, "ok", stackReservedDir)); lt == nil || lt.Action != "up" {
		t.Errorf("persisted = %+v", lt)
	}
}

func TestCovStackGetStackFileReadError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "web"), 0o700)
	compose := filepath.Join(root, "web", "compose.yml")
	os.WriteFile(compose, []byte("services: {}\n"), 0o644)
	os.Chmod(compose, 0o000)
	t.Cleanup(func() { os.Chmod(compose, 0o644) })
	p := NewStackProvider(StackConfig{Dir: root})
	if _, err := p.GetStackFile("web"); err == nil {
		t.Error("unreadable compose file should fail")
	}
}

func TestCovStackSaveFileErrorBranches(t *testing.T) {
	root := t.TempDir()
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	p.dir = root

	// .env 超限
	if _, err := p.SaveStackFile(map[string]interface{}{
		"name": "web", "compose": "services: {}", "env": strings.Repeat("x", stackFileLimit+1),
	}); err == nil || !strings.Contains(err.Error(), ".env file too large") {
		t.Errorf("oversized env err = %v", err)
	}

	// stacks 根路径中一段是文件 → MkdirAll 失败
	os.WriteFile(filepath.Join(root, "blocker"), []byte("x"), 0o644)
	p2 := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	p2.dir = filepath.Join(root, "blocker", "stacks")
	if _, err := p2.SaveStackFile(map[string]interface{}{"name": "web", "compose": "services: {}"}); err == nil {
		t.Error("save under file path should fail")
	}

	// 临时文件路径被目录占用 → WriteFile 失败
	p3 := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	os.MkdirAll(filepath.Join(p3.dir, "web", stackReservedDir, "compose.yml.tmp"), 0o700)
	if _, err := p3.SaveStackFile(map[string]interface{}{"name": "web", "compose": "services: {}"}); err == nil {
		t.Error("save with dir-clogged temp file should fail")
	}

	// 目标 compose.yml 已是目录 → Rename 失败
	p4 := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	os.MkdirAll(filepath.Join(p4.dir, "web", "compose.yml"), 0o700)
	if _, err := p4.SaveStackFile(map[string]interface{}{"name": "web", "compose": "services: {}"}); err == nil {
		t.Error("save with dir-clogged compose.yml should fail")
	}

	// .env 路径已是目录 → WriteFile 失败
	p5 := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	os.MkdirAll(filepath.Join(p5.dir, "web", ".env"), 0o700)
	if _, err := p5.SaveStackFile(map[string]interface{}{"name": "web", "compose": "services: {}", "env": "K=1"}); err == nil {
		t.Error("save with dir-clogged .env should fail")
	}
}

func TestCovStackActionInvalidName(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	for _, action := range []string{"up", "down", "restart", "pull", "remove", "logs"} {
		if _, err := p.Call(action, map[string]interface{}{"name": "../evil"}); err == nil {
			t.Errorf("%s with invalid name should fail", action)
		}
	}
	// remove 对不存在的目录报 not found
	if _, err := p.Call("remove", map[string]interface{}{"name": "ghost"}); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Errorf("remove missing stack err = %v", err)
	}
}

func TestCovStackRemoveDownFails(t *testing.T) {
	// compose 存在 + down 失败 → remove 任务失败且目录保留
	p := newTestStackProvider(t, `echo "down failed" >&2; exit 1`, &mockStackDocker{})
	writeStack(t, p.dir, "web", "compose.yml", "services: {}\n")
	resp, _ := p.Call("remove", map[string]interface{}{"name": "web"})
	task := waitForTask(t, p, resp.(map[string]interface{})["taskId"].(string))
	if task["status"] != stackTaskFailed {
		t.Fatalf("remove with failed down = %v, log=%v", task["status"], task["log"])
	}
	if !strings.Contains(task["log"].(string), "down before remove") {
		t.Errorf("log should mention down-before-remove, got %v", task["log"])
	}
	if _, err := os.Stat(filepath.Join(p.dir, "web")); err != nil {
		t.Errorf("stack dir must be kept on failed down: %v", err)
	}
}

func TestCovStackRemoveForgetsBaseline(t *testing.T) {
	rec := newFakeRecorder()
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	p.SetBaseline(rec)
	writeStack(t, p.dir, "web", "compose.yml", "services: {}\n")
	resp, _ := p.Call("remove", map[string]interface{}{"name": "web"})
	waitForTask(t, p, resp.(map[string]interface{})["taskId"].(string))
	if len(rec.forgot) != 2 {
		t.Fatalf("forgot = %v, want compose+env", rec.forgot)
	}
}

func TestCovStackInfoMkdirFails(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "blocker"), []byte("x"), 0o644)
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	p.dir = filepath.Join(root, "blocker", "stacks")
	raw, err := p.Call("info", nil)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	info := raw.(*StackInfo)
	if info.DirWritable || info.DirError == "" {
		t.Errorf("info = %+v, want dirError", info)
	}
}

func TestCovStackProbeComposeVersionFails(t *testing.T) {
	p := NewStackProvider(StackConfig{ComposeBin: []string{"cov-no-such-cmd-xyz"}})
	if got := p.probeComposeVersion(); got != "" {
		t.Errorf("probe with missing binary = %q, want empty", got)
	}
	// InfoStack 走同一探测（version 为空但 info 成功）
	p2 := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	p2.composeBin = []string{"cov-no-such-cmd-xyz"}
	raw, err := p2.Call("info", nil)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if v := raw.(*StackInfo).ComposeVersion; v != "" {
		t.Errorf("composeVersion = %q, want empty", v)
	}
}

func TestCovStackLogsBranches(t *testing.T) {
	// 无 tail 参数 → 默认 200
	p := newTestStackProvider(t, `echo "default tail"`, &mockStackDocker{})
	writeStack(t, p.dir, "web", "compose.yml", "services: {}\n")
	resp, err := p.Call("logs", map[string]interface{}{"name": "web"})
	if err != nil {
		t.Fatalf("logs default tail: %v", err)
	}
	if !strings.Contains(resp.(map[string]interface{})["logs"].(string), "default tail") {
		t.Errorf("logs = %v", resp)
	}

	// tail 超上限拒绝
	if _, err := p.Call("logs", map[string]interface{}{"name": "web", "tail": "100001"}); err == nil {
		t.Error("tail > 100000 should fail")
	}

	// compose logs 命令失败 → 错误透传
	p2 := newTestStackProvider(t, `echo "boom" >&2; exit 3`, &mockStackDocker{})
	writeStack(t, p2.dir, "web", "compose.yml", "services: {}\n")
	if _, err := p2.Call("logs", map[string]interface{}{"name": "web"}); err == nil ||
		!strings.Contains(err.Error(), "compose logs") {
		t.Errorf("logs failure err = %v", err)
	}

	// 输出超 1MB → 截断到 stackLogTail
	p3 := newTestStackProvider(t, `head -c 1200000 /dev/zero | tr '\0' 'a'`, &mockStackDocker{})
	writeStack(t, p3.dir, "web", "compose.yml", "services: {}\n")
	resp, err = p3.Call("logs", map[string]interface{}{"name": "web"})
	if err != nil {
		t.Fatalf("logs big output: %v", err)
	}
	logs := resp.(map[string]interface{})["logs"].(string)
	if len(logs) > stackLogTail {
		t.Errorf("logs len = %d, want <= %d", len(logs), stackLogTail)
	}
}
