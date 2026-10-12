package rpc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============ P2 ② 测试：git.deploy 拉取+部署、任务日志 follow 流 ============

// gitDeployProvider 真实 git + 记账 compose 假脚本（up 调用追加进 $CD_LOG）
func gitDeployProvider(t *testing.T, dir string, slow bool) *StackProvider {
	t.Helper()
	// 回显进任务日志（follow 流断言用）+ 落盘给 deploy 用例断言
	script := `echo "stack-cmd $@"
echo "$@" >> "$CD_LOG"`
	if slow {
		script += `
case "$*" in
  *" up "*) sleep 1 ;;
esac`
	}
	script += "\nexit 0"
	return NewStackProvider(StackConfig{
		Dir:        dir,
		ComposeBin: []string{"/bin/sh", "-c", script},
		GitBin:     []string{"git"},
	})
}

func composeLog(t *testing.T) (string, func() string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "compose.log")
	t.Setenv("CD_LOG", logPath)
	return logPath, func() string {
		b, err := os.ReadFile(logPath)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// seedStackDir 手工铺一个 stack 目录（不走 git 拉取的用例）
func seedStackDir(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, "compose.yml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// HEAD 前移：只部署本次拉取改动的 stack；非 stack 目录跳过
func TestStackGitDeployChangedOnly(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{
		"web/compose.yml": "services:\n  web:\n    image: nginx\n",
		"db/compose.yml":  "services:\n  db:\n    image: pg\n",
	})
	_, readLog := composeLog(t)
	p := gitDeployProvider(t, t.TempDir(), false)

	// 已接入（plain pull，不部署）
	if _, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"}); err != nil {
		t.Fatal(err)
	}
	if got := readLog(); strings.Contains(got, "up") {
		t.Fatalf("plain pull should not deploy: %s", got)
	}

	// 只改 web：deploy 只 up web
	pushNewCommit(t, remote, "web/compose.yml", "services:\n  web:\n    image: nginx:1.27\n")
	res, err := p.GitDeploy(map[string]interface{}{})
	if err != nil {
		t.Fatalf("git deploy: %v", err)
	}
	dep := res["deployed"].([]map[string]interface{})
	if len(dep) != 1 || dep[0]["stack"] != "web" || dep[0]["taskId"] == "" {
		t.Fatalf("deployed = %v", dep)
	}
	if len(res["skipped"].([]map[string]interface{})) != 0 {
		t.Fatalf("skipped = %v", res["skipped"])
	}
	waitForTask(t, p, dep[0]["taskId"].(string))
	log := readLog()
	if !strings.Contains(log, "web") || !strings.Contains(log, "up") {
		t.Fatalf("compose up not invoked for web: %s", log)
	}
	if strings.Contains(log, "db") {
		t.Fatalf("untouched stack deployed: %s", log)
	}

	// 只加非 stack 目录（README）：HEAD 前移但无 stack 变更 → 不部署
	pushNewCommit(t, remote, "docs/readme.md", "# notes\n")
	res, err = p.GitDeploy(map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res["deployed"].([]map[string]interface{})) != 0 {
		t.Fatalf("non-stack change deployed: %v", res["deployed"])
	}

	// HEAD 未动：不部署（走 Call 分派覆盖 git.deploy case）
	callRes, err := p.Call("git.deploy", map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	res = callRes.(map[string]interface{})
	if len(res["deployed"].([]map[string]interface{})) != 0 {
		t.Fatalf("no-change deploy: %v", res["deployed"])
	}
}

// 首次接入：部署仓库里的全部 stack
func TestStackGitDeployAttachAll(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{
		"web/compose.yml": "services:\n  web:\n    image: nginx\n",
		"db/compose.yml":  "services:\n  db:\n    image: pg\n",
	})
	_, readLog := composeLog(t)
	p := gitDeployProvider(t, t.TempDir(), false)
	res, err := p.GitDeploy(map[string]interface{}{"remote": remote, "branch": "main"})
	if err != nil {
		t.Fatalf("attach deploy: %v", err)
	}
	got := map[string]bool{}
	for _, d := range res["deployed"].([]map[string]interface{}) {
		got[d["stack"].(string)] = true
		waitForTask(t, p, d["taskId"].(string))
	}
	if !got["web"] || !got["db"] {
		t.Fatalf("attach deployed = %v", got)
	}
	if !strings.Contains(readLog(), "up") {
		t.Fatal("compose up not invoked")
	}
}

// 目标 stack 任务执行中（锁占用）：记 skipped 不报错
func TestStackGitDeployBusySkipped(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{"web/compose.yml": "services: {}\n"})
	composeLog(t)
	p := gitDeployProvider(t, t.TempDir(), true)
	if _, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"}); err != nil {
		t.Fatal(err)
	}
	// 占住 web 的任务锁（compose 假脚本对 up 睡 1s，留足 GitDeploy 竞速窗口）
	busyRes, err := p.UpStack("web")
	if err != nil {
		t.Fatal(err)
	}
	busyTask := busyRes["taskId"].(string)
	pushNewCommit(t, remote, "web/compose.yml", "services:\n  web:\n    image: v2\n")
	res, err := p.GitDeploy(map[string]interface{}{})
	if err != nil {
		t.Fatalf("deploy while busy: %v", err)
	}
	skip := res["skipped"].([]map[string]interface{})
	if len(skip) != 1 || skip[0]["stack"] != "web" {
		t.Fatalf("skipped = %v", skip)
	}
	if len(res["deployed"].([]map[string]interface{})) != 0 {
		t.Fatalf("deployed = %v", res["deployed"])
	}
	// 等占位任务结束再收尾，避免测试泄漏运行中任务
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, err := p.GetTask(busyTask)
		if err == nil && st["status"] != stackTaskRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("busy task never finished")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// follow 参数校验 + Call 分派（task.follow / task.follow.stop）
func TestStackTaskFollowValidationAndDispatch(t *testing.T) {
	composeLog(t)
	dir := t.TempDir()
	seedStackDir(t, dir, "web")
	p := gitDeployProvider(t, dir, true)

	// sender 未注入：Call 分派进 FollowTask 报不可用
	if _, err := p.Call("task.follow", map[string]interface{}{"taskId": "x", "followId": "f"}); err == nil ||
		!strings.Contains(err.Error(), "not available") {
		t.Fatalf("dispatch without sender err = %v", err)
	}

	var closed chan string
	p.SetSender(func(string, []byte) {})
	p.SetCloser(func(_ string, reason string) { closed <- reason })

	// 缺 taskId/followId
	if _, err := p.Call("task.follow", map[string]interface{}{"followId": "f1"}); err == nil ||
		!strings.Contains(err.Error(), "required") {
		t.Fatalf("missing params err = %v", err)
	}
	// stop 缺 followId（经 Call 分派）
	if _, err := p.Call("task.follow.stop", map[string]interface{}{}); err == nil ||
		!strings.Contains(err.Error(), "required") {
		t.Fatalf("stop missing followId err = %v", err)
	}
}

// agent 侧同活 follow 上限：8 个之后拒绝，stop 全部释放
func TestStackTaskFollowMaxActive(t *testing.T) {
	composeLog(t)
	dir := t.TempDir()
	seedStackDir(t, dir, "web")
	p := gitDeployProvider(t, dir, true)
	p.SetSender(func(string, []byte) {})
	closed := make(chan string, 16)
	p.SetCloser(func(_ string, reason string) { closed <- reason })

	res, err := p.UpStack("web")
	if err != nil {
		t.Fatal(err)
	}
	taskID := res["taskId"].(string)
	for i := 0; i < stackTaskFollowMaxActive; i++ {
		if _, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": fmt.Sprintf("f%d", i)}); err != nil {
			t.Fatalf("follow %d: %v", i, err)
		}
	}
	if _, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": "over"}); err == nil ||
		!strings.Contains(err.Error(), "too many") {
		t.Fatalf("over-limit err = %v", err)
	}
	// 逐个 stop：pump 的 stop 分支退出 + 名额释放
	for i := 0; i < stackTaskFollowMaxActive; i++ {
		if _, err := p.StopTaskFollow(map[string]interface{}{"followId": fmt.Sprintf("f%d", i)}); err != nil {
			t.Fatalf("stop %d: %v", i, err)
		}
	}
	for i := 0; i < stackTaskFollowMaxActive; i++ {
		select {
		case reason := <-closed:
			if reason != "stopped" {
				t.Fatalf("close reason = %q", reason)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("stop close missing")
		}
	}
	// 名额已全部释放：第 9 个现在能建
	if _, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": "now-ok"}); err != nil {
		t.Fatalf("follow after release: %v", err)
	}
	p.StopTaskFollow(map[string]interface{}{"followId": "now-ok"})
	waitForTask(t, p, taskID)
}

// 未出生 HEAD（git init 无提交）下 deploy：gitHeadShort 回空 + pull 报错透传
func TestStackGitDeployUnbornPullFail(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{"web/compose.yml": "services: {}\n"})
	composeLog(t)
	dir := t.TempDir()
	gitRun(t, dir, "init")
	p := gitDeployProvider(t, dir, false)
	if _, err := p.GitDeploy(map[string]interface{}{"remote": remote, "branch": "main"}); err == nil ||
		!strings.Contains(err.Error(), "read branch") {
		t.Fatalf("unborn deploy err = %v", err)
	}
}

// 同一 stack 在 diff 里出现多行：去重只部署一次；非 stack 命名顶层目录跳过
func TestStackGitDeployDedupeAndNameFilter(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{"web/compose.yml": "services: {}\n"})
	_, readLog := composeLog(t)
	p := gitDeployProvider(t, t.TempDir(), false)
	if _, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"}); err != nil {
		t.Fatal(err)
	}
	// web 目录两处改动 + 大写开头目录（stack 名正则外）
	pushNewCommit(t, remote, "web/x.txt", "1\n")
	pushNewCommit(t, remote, "web/y.txt", "2\n")
	pushNewCommit(t, remote, "Scripts/run.sh", "#!/bin/sh\n")
	res, err := p.GitDeploy(map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	dep := res["deployed"].([]map[string]interface{})
	if len(dep) != 1 || dep[0]["stack"] != "web" {
		t.Fatalf("deployed = %v, want web once", dep)
	}
	waitForTask(t, p, dep[0]["taskId"].(string))
	log := readLog()
	if strings.Contains(log, "Scripts") {
		t.Fatalf("non-stack dir deployed: %s", log)
	}
}

// follow 流：回填 + 增量推送 + 终态 close；stop 收尾；已结束任务快照即终
func TestStackTaskFollow(t *testing.T) {
	dir := t.TempDir()
	composeLog(t)
	seedStackDir(t, dir, "web")
	p := gitDeployProvider(t, dir, true)

	// sender 未注入：报错
	if _, err := p.FollowTask(map[string]interface{}{"taskId": "x", "followId": "f1"}); err == nil {
		t.Fatal("follow without sender should fail")
	}

	var mu sync.Mutex
	var got []string
	closed := make(chan string, 4)
	p.SetSender(func(proxyID string, data []byte) {
		if !strings.HasPrefix(proxyID, "stacktask:") {
			t.Errorf("proxyId prefix missing: %q", proxyID)
		}
		mu.Lock()
		got = append(got, string(data))
		mu.Unlock()
	})
	p.SetCloser(func(proxyID, reason string) {
		closed <- reason
	})

	// 未知任务
	if _, err := p.FollowTask(map[string]interface{}{"taskId": "nope", "followId": "f0"}); err == nil ||
		!strings.Contains(err.Error(), "task not found") {
		t.Fatalf("unknown task err = %v", err)
	}

	// 运行中任务：回填 + 终态收尾
	res, err := p.UpStack("web")
	if err != nil {
		t.Fatal(err)
	}
	taskID := res["taskId"].(string)
	if _, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": "f1"}); err != nil {
		t.Fatalf("follow start: %v", err)
	}
	select {
	case reason := <-closed:
		if reason != stackTaskSuccess {
			t.Fatalf("close reason = %q", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follow never closed")
	}
	mu.Lock()
	all := strings.Join(got, "")
	mu.Unlock()
	if !strings.Contains(all, "up") {
		t.Fatalf("follow content missing compose output: %q", all)
	}

	// 已结束任务：快照 + 立即终态
	got = nil
	if _, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": "f2"}); err != nil {
		t.Fatal(err)
	}
	select {
	case reason := <-closed:
		if reason != stackTaskSuccess {
			t.Fatalf("finished follow close = %q", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("finished follow never closed")
	}

	// stop：运行中任务 follow 后 stop → reason=stopped
	res, err = p.UpStack("web")
	if err != nil {
		t.Fatal(err)
	}
	taskID = res["taskId"].(string)
	if _, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": "f3"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.StopTaskFollow(map[string]interface{}{"followId": "f3"}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case reason := <-closed:
		if reason != "stopped" {
			t.Fatalf("stop close = %q", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stop never closed")
	}
	// 重复 stop：会话已注销，报 not found
	if _, err := p.StopTaskFollow(map[string]interface{}{"followId": "f3"}); err == nil {
		t.Fatal("double stop should fail")
	}
}
