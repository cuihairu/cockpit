package rpc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/docker"
	"github.com/moby/moby/client"
)

// ============ 测试基建 ============

// newTestStackProvider 构造注入了 fake compose 命令与 fake DockerAPI 的 provider。
// script 是传给 /bin/sh -c 的脚本体，provider 追加的参数会作为脚本的位置参数（"$@"）。
func newTestStackProvider(t *testing.T, script string, mock docker.DockerAPI) *StackProvider {
	t.Helper()
	root := t.TempDir()
	p := NewStackProvider(StackConfig{
		Dir:        root,
		Docker:     mock,
		ComposeBin: []string{"/bin/sh", "-c", script},
		Now:        func() time.Time { return time.Unix(1700000000, 0) },
	})
	return p
}

// mockStackDocker 最小化 DockerAPI mock，只实现 ListContainers，其余空实现
type mockStackDocker struct {
	containers []docker.ContainerInfo
	err        error
}

func (m *mockStackDocker) ListContainers(all bool) ([]docker.ContainerInfo, error) {
	return m.containers, m.err
}

func (m *mockStackDocker) GetContainer(id string) (*docker.ContainerInfo, error) {
	return nil, errors.New("not implemented")
}
func (m *mockStackDocker) StartContainer(id string) error { return nil }
func (m *mockStackDocker) StopContainer(id string, timeout *int) error {
	return nil
}
func (m *mockStackDocker) RestartContainer(id string, timeout *int) error {
	return nil
}
func (m *mockStackDocker) RemoveContainer(id string, force, removeVolumes bool) error {
	return nil
}
func (m *mockStackDocker) PauseContainer(id string) error   { return nil }
func (m *mockStackDocker) UnpauseContainer(id string) error { return nil }
func (m *mockStackDocker) GetLogs(id string, tail, since string, follow, timestamps, stdout, stderr bool) (string, error) {
	return "", nil
}
func (m *mockStackDocker) GetContainerStats(id string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *mockStackDocker) ListImages(all bool) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (m *mockStackDocker) RemoveImage(id string, force, pruneChildren bool) ([]string, error) {
	return nil, nil
}
func (m *mockStackDocker) PullImage(ref string) (string, error) { return "", nil }
func (m *mockStackDocker) ListVolumes() ([]docker.VolumeInfo, error) {
	return nil, nil
}
func (m *mockStackDocker) RemoveVolume(name string, force bool) error { return nil }
func (m *mockStackDocker) ListNetworks() ([]docker.NetworkInfo, error) {
	return nil, nil
}
func (m *mockStackDocker) Info() (*docker.SystemInfo, error) { return nil, nil }
func (m *mockStackDocker) Version() (client.ServerVersionResult, error) {
	return client.ServerVersionResult{}, nil
}
func (m *mockStackDocker) Close() error { return nil }

// composeWebContainers 构造带 compose label 的容器（web 项目 2 个服务，1 运行 1 停止）
func composeWebContainers() []docker.ContainerInfo {
	return []docker.ContainerInfo{
		{
			ID: "c1", Name: "web-nginx-1", Image: "nginx:latest", State: "running", Status: "Up 2 hours",
			Labels: map[string]string{"com.docker.compose.project": "web", "com.docker.compose.service": "nginx"},
		},
		{
			ID: "c2", Name: "web-redis-1", Image: "redis:7", State: "exited", Status: "Exited (0)",
			Labels: map[string]string{"com.docker.compose.project": "web", "com.docker.compose.service": "redis"},
		},
		{
			ID: "c3", Name: "other", Image: "busybox", State: "running",
			Labels: map[string]string{}, // 无 compose label，不应计入任何 stack
		},
	}
}

// writeStack 写入一个 stack 目录（含 compose 文件与可选内容；composeFile 为空只建目录）
func writeStack(t *testing.T, root, name, composeFile, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if composeFile == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, composeFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func waitForTask(t *testing.T, p *StackProvider, taskID string) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := p.Call("task.get", map[string]interface{}{"taskId": taskID})
		if err != nil {
			t.Fatalf("task.get error = %v", err)
		}
		task := resp.(map[string]interface{})
		if task["status"] != stackTaskRunning {
			return task
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("task did not finish in time")
	return nil
}

// ============ 名称校验 ============

func TestStackProviderInvalidName(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	for _, name := range []string{"../evil", "a/b", "Upper", "has space", ".hidden", "", "a{b}"} {
		if _, err := p.Call("status", map[string]interface{}{"name": name}); err == nil {
			t.Errorf("status(%q) should fail", name)
		}
		if _, err := p.Call("file.save", map[string]interface{}{"name": name, "compose": "services: {}"}); err == nil {
			t.Errorf("file.save(%q) should fail", name)
		}
	}
}

// ============ 列表 ============

func TestStackProviderList(t *testing.T) {
	root := t.TempDir()
	writeStack(t, root, "web", "compose.yml", "services:\n  nginx:\n    image: nginx\n")
	writeStack(t, root, "db", "docker-compose.yml", "services:\n  pg:\n    image: postgres\n")
	writeStack(t, root, "empty-dir", "", "")                           // 无 compose 文件 → 跳过
	writeStack(t, root, ".hidden", "compose.yml", "services: {}\n")    // 隐藏目录 → 跳过
	os.WriteFile(filepath.Join(root, "afile.txt"), []byte("x"), 0o644) // 普通文件 → 跳过
	// web 有已完成的任务元数据
	os.MkdirAll(filepath.Join(root, "web", ".cockpit"), 0o700)
	os.WriteFile(filepath.Join(root, "web", ".cockpit", "last-task.json"),
		[]byte(`{"action":"up","status":"success","finishedAt":1700000000}`), 0o644)

	p := NewStackProvider(StackConfig{
		Dir:        root,
		Docker:     &mockStackDocker{containers: composeWebContainers()},
		ComposeBin: []string{"/bin/sh", "-c", "exit 0"},
	})

	resp, err := p.Call("list", nil)
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	stacks := resp.([]StackSummary)
	if len(stacks) != 2 {
		t.Fatalf("list count = %d, want 2 (web, db)", len(stacks))
	}
	// 按名称排序：db 在前，web 在后
	db := stacks[0]
	if db.Name != "db" || db.Total != 0 || db.ComposeFile != "docker-compose.yml" {
		t.Errorf("db summary = %+v", db)
	}
	web := stacks[1]
	if web.Name != "web" || web.Running != 1 || web.Total != 2 {
		t.Errorf("web summary = %+v", web)
	}
	if len(web.Services) != 2 || web.Services[0].Name != "nginx" || web.Services[0].ContainerID != "c1" {
		t.Errorf("web services = %+v", web.Services)
	}
	if web.LastAction != "up" || web.LastStatus != "success" || web.LastDeployedAt != 1700000000 {
		t.Errorf("web last task = %s/%s/%d", web.LastAction, web.LastStatus, web.LastDeployedAt)
	}
}

func TestStackProviderListMissingRoot(t *testing.T) {
	p := NewStackProvider(StackConfig{Dir: filepath.Join(t.TempDir(), "nonexistent")})
	resp, err := p.Call("list", nil)
	if err != nil {
		t.Fatalf("list on missing root should not error, got %v", err)
	}
	if len(resp.([]StackSummary)) != 0 {
		t.Error("list should be empty")
	}
}

func TestStackProviderListDockerErrorStillLists(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{err: errors.New("daemon down")})
	resp, err := p.Call("list", nil)
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	if len(resp.([]StackSummary)) != 0 {
		t.Error("docker error should yield zero services but no failure")
	}
}

// ============ 状态 ============

func TestStackProviderStatus(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{containers: composeWebContainers()})
	writeStack(t, p.dir, "web", "compose.yml", "services: {}\n")

	resp, err := p.Call("status", map[string]interface{}{"name": "web"})
	if err != nil {
		t.Fatalf("status error = %v", err)
	}
	summary := resp.(*StackSummary)
	if summary.Running != 1 || summary.Total != 2 || len(summary.Services) != 2 {
		t.Errorf("summary = %+v", summary)
	}
}

func TestStackProviderStatusNotFound(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	if _, err := p.Call("status", map[string]interface{}{"name": "ghost"}); err == nil {
		t.Error("status on missing stack should fail")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got %v", err)
	}
}

// ============ 文件读写 ============

func TestStackProviderFileGetSave(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})

	// 保存（首次 → created=true），同时带 .env
	resp, err := p.Call("file.save", map[string]interface{}{
		"name":    "web",
		"compose": "services:\n  nginx:\n    image: nginx\n",
		"env":     "KEY=value\n",
	})
	if err != nil {
		t.Fatalf("save error = %v", err)
	}
	if resp.(map[string]interface{})["created"] != true {
		t.Error("first save should report created=true")
	}

	// legacy 命名会被清理，统一为 compose.yml
	if _, err := os.Stat(filepath.Join(p.dir, "web", "compose.yml")); err != nil {
		t.Errorf("compose.yml should exist: %v", err)
	}

	// 读取
	gResp, err := p.Call("file.get", map[string]interface{}{"name": "web"})
	if err != nil {
		t.Fatalf("file.get error = %v", err)
	}
	file := gResp.(*StackFile)
	if file.ComposeFile != "compose.yml" || !strings.Contains(file.Compose, "nginx") {
		t.Errorf("file = %+v", file)
	}
	if file.Env != "KEY=value\n" {
		t.Errorf("env = %q", file.Env)
	}
	if file.ModifiedAt == 0 {
		t.Error("modifiedAt should be set")
	}

	// 二次保存（created=false）
	resp, err = p.Call("file.save", map[string]interface{}{"name": "web", "compose": "services: {}\n"})
	if err != nil {
		t.Fatalf("second save error = %v", err)
	}
	if resp.(map[string]interface{})["created"] != false {
		t.Error("second save should report created=false")
	}

	// 未传 env 时不覆盖已有 .env
	envContent, _ := os.ReadFile(filepath.Join(p.dir, "web", ".env"))
	if string(envContent) != "KEY=value\n" {
		t.Errorf(".env should be preserved, got %q", envContent)
	}
}

func TestStackProviderFileSaveLegacyCleanup(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	writeStack(t, p.dir, "web", "docker-compose.yml", "old\n")

	if _, err := p.Call("file.save", map[string]interface{}{"name": "web", "compose": "services: {}\n"}); err != nil {
		t.Fatalf("save error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.dir, "web", "docker-compose.yml")); !os.IsNotExist(err) {
		t.Error("legacy docker-compose.yml should be removed after save")
	}
}

func TestStackProviderFileSaveValidationFails(t *testing.T) {
	// fake compose 以 exit 1 + 错误输出模拟校验失败
	p := newTestStackProvider(t, `echo "invalid yaml" >&2; exit 1`, &mockStackDocker{})

	_, err := p.Call("file.save", map[string]interface{}{"name": "web", "compose": "services: [broken"})
	if err == nil {
		t.Fatal("save with invalid compose should fail")
	}
	if !strings.Contains(err.Error(), "compose validation failed") || !strings.Contains(err.Error(), "invalid yaml") {
		t.Errorf("error should carry compose output, got %v", err)
	}
	// 校验失败不落盘
	if _, err := os.Stat(filepath.Join(p.dir, "web", "compose.yml")); !os.IsNotExist(err) {
		t.Error("compose.yml must not be written when validation fails")
	}
}

func TestStackProviderFileSaveLimits(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	huge := strings.Repeat("x", stackFileLimit+1)
	if _, err := p.Call("file.save", map[string]interface{}{"name": "web", "compose": huge}); err == nil {
		t.Error("oversized compose should fail")
	}
	if _, err := p.Call("file.save", map[string]interface{}{"name": "web"}); err == nil {
		t.Error("empty compose should fail")
	}
}

func TestStackProviderFileGetNotFound(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	if _, err := p.Call("file.get", map[string]interface{}{"name": "ghost"}); err == nil {
		t.Error("file.get on missing stack should fail")
	}
}

// ============ 异步任务：up/down/remove/busy ============

func TestStackProviderUpTaskLifecycle(t *testing.T) {
	// fake compose：输出两行后成功
	p := newTestStackProvider(t, `echo "Pulling nginx"; echo "Started"; exit 0`, &mockStackDocker{})
	writeStack(t, p.dir, "web", "compose.yml", "services: {}\n")

	resp, err := p.Call("up", map[string]interface{}{"name": "web"})
	if err != nil {
		t.Fatalf("up error = %v", err)
	}
	taskID := resp.(map[string]interface{})["taskId"].(string)
	if taskID == "" {
		t.Fatal("taskId should not be empty")
	}

	task := waitForTask(t, p, taskID)
	if task["status"] != stackTaskSuccess {
		t.Errorf("status = %v, want success; log = %v", task["status"], task["log"])
	}
	if !strings.Contains(task["log"].(string), "Pulling nginx") {
		t.Errorf("log should contain compose output, got %v", task["log"])
	}
	if task["action"] != "up" || task["stack"] != "web" {
		t.Errorf("task meta = %v/%v", task["action"], task["stack"])
	}
	if task["finishedAt"].(int64) == 0 {
		t.Error("finishedAt should be set")
	}

	// last-task.json 落盘，list 可见
	list, err := p.Call("list", nil)
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	web := list.([]StackSummary)[0]
	if web.LastAction != "up" || web.LastStatus != stackTaskSuccess || web.LastDeployedAt == 0 {
		t.Errorf("list last task = %s/%s/%d", web.LastAction, web.LastStatus, web.LastDeployedAt)
	}
}

func TestStackProviderUpFailsOnComposeError(t *testing.T) {
	p := newTestStackProvider(t, `echo "pull failed" >&2; exit 17`, &mockStackDocker{})
	writeStack(t, p.dir, "web", "compose.yml", "services: {}\n")

	resp, _ := p.Call("up", map[string]interface{}{"name": "web"})
	task := waitForTask(t, p, resp.(map[string]interface{})["taskId"].(string))
	if task["status"] != stackTaskFailed {
		t.Errorf("status = %v, want failed", task["status"])
	}
	if !strings.Contains(task["log"].(string), "pull failed") {
		t.Errorf("log should carry stderr, got %v", task["log"])
	}
	// 失败也写 last-task.json
	if lt := readLastTask(filepath.Join(p.dir, "web", stackReservedDir)); lt == nil || lt.Status != stackTaskFailed {
		t.Errorf("last-task.json = %+v, want failed", lt)
	}
}

func TestStackProviderBusy(t *testing.T) {
	// fake compose 睡 300ms，制造可观察的运行窗口
	p := newTestStackProvider(t, `sleep 0.3; exit 0`, &mockStackDocker{})
	writeStack(t, p.dir, "web", "compose.yml", "services: {}\n")

	if _, err := p.Call("up", map[string]interface{}{"name": "web"}); err != nil {
		t.Fatalf("first up error = %v", err)
	}
	// launchTask 在同步段持锁，此刻必然 busy
	if _, err := p.Call("up", map[string]interface{}{"name": "web"}); err == nil {
		t.Fatal("second concurrent up should fail with busy")
	} else if !strings.Contains(err.Error(), "busy") {
		t.Errorf("error should mention busy, got %v", err)
	}
	// down 同样被拒
	if _, err := p.Call("down", map[string]interface{}{"name": "web"}); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Errorf("down during up should be busy, got %v", err)
	}
	// 等待结束后恢复可用
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := p.Call("up", map[string]interface{}{"name": "web"}); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("up should succeed again after task finishes")
}

func TestStackProviderDown(t *testing.T) {
	p := newTestStackProvider(t, `exit 0`, &mockStackDocker{})
	writeStack(t, p.dir, "web", "compose.yml", "services: {}\n")
	resp, err := p.Call("down", map[string]interface{}{"name": "web"})
	if err != nil {
		t.Fatalf("down error = %v", err)
	}
	task := waitForTask(t, p, resp.(map[string]interface{})["taskId"].(string))
	if task["status"] != stackTaskSuccess || task["action"] != "down" {
		t.Errorf("task = %v/%v", task["action"], task["status"])
	}
}

func TestStackProviderRemove(t *testing.T) {
	p := newTestStackProvider(t, `exit 0`, &mockStackDocker{})
	writeStack(t, p.dir, "web", "compose.yml", "services: {}\n")

	resp, err := p.Call("remove", map[string]interface{}{"name": "web"})
	if err != nil {
		t.Fatalf("remove error = %v", err)
	}
	taskID := resp.(map[string]interface{})["taskId"].(string)
	task := waitForTask(t, p, taskID)
	if task["status"] != stackTaskSuccess {
		t.Errorf("remove task = %v, log=%v", task["status"], task["log"])
	}
	if _, err := os.Stat(filepath.Join(p.dir, "web")); !os.IsNotExist(err) {
		t.Error("stack dir should be removed")
	}
	// 删除后任务记录仍在，task.get 可查
	if _, err := p.Call("task.get", map[string]interface{}{"taskId": taskID}); err != nil {
		t.Errorf("task.get after remove error = %v", err)
	}
}

func TestStackProviderUpNotFound(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	for _, action := range []string{"up", "down", "restart", "pull", "remove"} {
		if _, err := p.Call(action, map[string]interface{}{"name": "ghost"}); err == nil {
			t.Errorf("%s on missing stack should fail", action)
		}
	}
}

func TestStackProviderRestartAndPull(t *testing.T) {
	p := newTestStackProvider(t, `exit 0`, &mockStackDocker{})
	writeStack(t, p.dir, "web", "compose.yml", "services: {}\n")

	resp, err := p.Call("restart", map[string]interface{}{"name": "web"})
	if err != nil {
		t.Fatalf("restart error = %v", err)
	}
	task := waitForTask(t, p, resp.(map[string]interface{})["taskId"].(string))
	if task["status"] != stackTaskSuccess || task["action"] != "restart" {
		t.Errorf("restart task = %v/%v", task["action"], task["status"])
	}

	resp, err = p.Call("pull", map[string]interface{}{"name": "web"})
	if err != nil {
		t.Fatalf("pull error = %v", err)
	}
	task = waitForTask(t, p, resp.(map[string]interface{})["taskId"].(string))
	if task["status"] != stackTaskSuccess || task["action"] != "pull" {
		t.Errorf("pull task = %v/%v", task["action"], task["status"])
	}
}

func TestStackProviderInfo(t *testing.T) {
	p := newTestStackProvider(t, `echo "v2.39.0"`, &mockStackDocker{})
	raw, err := p.Call("info", map[string]interface{}{})
	if err != nil {
		t.Fatalf("info error = %v", err)
	}
	info, ok := raw.(*StackInfo)
	if !ok {
		t.Fatalf("info type = %T, want *StackInfo", raw)
	}
	if info.Dir != p.dir {
		t.Errorf("info dir = %v, want %v", info.Dir, p.dir)
	}
	if !info.DirWritable {
		t.Errorf("info dirWritable = false, want true (err=%v)", info.DirError)
	}

	// 目录不可写（非 root 下 chmod 后创建探针文件失败）
	if os.Geteuid() != 0 {
		readOnly := filepath.Join(t.TempDir(), "ro")
		if err := os.MkdirAll(readOnly, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		p2 := newTestStackProvider(t, `exit 0`, &mockStackDocker{})
		p2.dir = readOnly
		if err := os.Chmod(readOnly, 0o500); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })
		raw2, err := p2.Call("info", map[string]interface{}{})
		if err != nil {
			t.Fatalf("info error = %v", err)
		}
		info2 := raw2.(*StackInfo)
		if info2.DirWritable {
			t.Error("readonly dir dirWritable = true, want false")
		}
		if info2.DirError == "" {
			t.Error("readonly dir should carry dirError")
		}
	}
}

func TestStackProviderTaskNotFound(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	if _, err := p.Call("task.get", map[string]interface{}{"taskId": "t-nope"}); err == nil {
		t.Error("unknown task should fail")
	}
	if _, err := p.Call("task.get", map[string]interface{}{}); err == nil {
		t.Error("empty taskId should fail")
	}
}

// ============ 同步日志 ============

func TestStackProviderLogs(t *testing.T) {
	p := newTestStackProvider(t, `echo "web-1 | hello"; echo "web-1 | world"`, &mockStackDocker{})
	writeStack(t, p.dir, "web", "compose.yml", "services: {}\n")

	resp, err := p.Call("logs", map[string]interface{}{"name": "web", "tail": "100", "service": "nginx"})
	if err != nil {
		t.Fatalf("logs error = %v", err)
	}
	logs := resp.(map[string]interface{})["logs"].(string)
	if !strings.Contains(logs, "hello") || !strings.Contains(logs, "world") {
		t.Errorf("logs = %q", logs)
	}
}

func TestStackProviderLogsInvalidTail(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	writeStack(t, p.dir, "web", "compose.yml", "services: {}\n")
	if _, err := p.Call("logs", map[string]interface{}{"name": "web", "tail": "abc"}); err == nil {
		t.Error("non-numeric tail should fail")
	}
	if _, err := p.Call("logs", map[string]interface{}{"name": "web", "tail": "-5"}); err == nil {
		t.Error("negative tail should fail")
	}
}

func TestStackProviderLogsNotFound(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	if _, err := p.Call("logs", map[string]interface{}{"name": "ghost"}); err == nil {
		t.Error("logs on missing stack should fail")
	}
}

func TestStackProviderUnknownAction(t *testing.T) {
	p := newTestStackProvider(t, "exit 0", &mockStackDocker{})
	if _, err := p.Call("bogus", nil); err == nil || !strings.Contains(err.Error(), "unknown stack action") {
		t.Errorf("unknown action error = %v", err)
	}
}

// ============ 并发上限 ============

func TestStackProviderConcurrentLimit(t *testing.T) {
	// 4 个慢任务占满信号量后，第 5 个 stack 拒绝
	p := newTestStackProvider(t, `sleep 0.5; exit 0`, &mockStackDocker{})
	for _, name := range []string{"s1", "s2", "s3", "s4"} {
		writeStack(t, p.dir, name, "compose.yml", "services: {}\n")
		if _, err := p.Call("up", map[string]interface{}{"name": name}); err != nil {
			t.Fatalf("up %s error = %v", name, err)
		}
	}
	writeStack(t, p.dir, "s5", "compose.yml", "services: {}\n")
	if _, err := p.Call("up", map[string]interface{}{"name": "s5"}); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Errorf("5th concurrent task should be rejected, got %v", err)
	}
	// 等信号量清空（全部任务 goroutine 退出）再返回：persistLastTask
	// 写盘发生在 release 之前，原「s5 可受理即返回」只等到一个槽位
	// 释放，其余任务可能仍在写盘，与 TempDir 清理竞争（CI 慢机 flaky）
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(p.sem) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("stack tasks did not finish within deadline")
}

// 确保 pruneTasksLocked 不丢运行中的任务
func TestStackProviderPruneKeepsRunning(t *testing.T) {
	p := NewStackProvider(StackConfig{Dir: t.TempDir()})
	now := time.Now()
	for i := 0; i < 150; i++ {
		status := stackTaskSuccess
		st := now.Add(-time.Duration(i) * time.Minute)
		if i == 149 {
			status = stackTaskRunning
		}
		p.tasks[string(rune('a'+i%26))+string(rune('a'+i/26))] = &stackTask{
			ID:         "",
			Status:     status,
			FinishedAt: st,
		}
	}
	// 手动构造：运行中的任务无 FinishedAt，不应被清掉
	var runningID string
	p.mu.Lock()
	for id, task := range p.tasks {
		if task.Status == stackTaskRunning {
			task.ID = id
			runningID = id
		}
	}
	p.pruneTasksLocked()
	_, kept := p.tasks[runningID]
	p.mu.Unlock()
	if !kept {
		t.Error("running task must not be pruned")
	}
}

// 编译期检查：mock 实现 DockerAPI
var _ docker.DockerAPI = (*mockStackDocker)(nil)

// 防止 unused import 报错（sync 在并发测试中使用）
var _ = sync.Mutex{}
