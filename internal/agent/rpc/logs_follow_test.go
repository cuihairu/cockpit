package rpc

import (
	"bufio"
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// follow 测试依赖 sh 起假跟随进程（journalctl/docker 不存在于 CI）
func fakeFollowCmd(script string) func(context.Context, *LogsQuery) (*exec.Cmd, *bufio.Reader, error) {
	return func(ctx context.Context, _ *LogsQuery) (*exec.Cmd, *bufio.Reader, error) {
		cmd := exec.CommandContext(ctx, "sh", "-c", script)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, nil, err
		}
		return cmd, bufio.NewReader(stdout), nil
	}
}

// followCollector 线程安全收集 sender/closer 回调（pumpFollow 在 goroutine 里跑）
type followCollector struct {
	mu     sync.Mutex
	lines  []string
	closes []string
}

func (c *followCollector) sender(_ string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, string(data))
}

func (c *followCollector) closer(_ string, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closes = append(c.closes, reason)
}

func (c *followCollector) gotLines() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}

func (c *followCollector) gotCloses() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.closes...)
}

// waitClose 轮询等待某 reason 的 close 出现（进程异步退出）
func waitClose(t *testing.T, c *followCollector, reason string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, r := range c.gotCloses() {
			if r == reason {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("close %q not received, got %v", reason, c.gotCloses())
}

func newFollowTestProvider() *LogsProvider {
	p := NewLogsProvider(nil)
	c := &followCollector{}
	p.SetSender(c.sender)
	p.SetCloser(c.closer)
	return p
}

func followParams(followID, grep string) map[string]interface{} {
	return map[string]interface{}{
		"followId": followID,
		"type":     "systemd",
		"source":   "nginx.service",
		"tail":     10,
		"grep":     grep,
	}
}

func TestLogsFollowPushAndGrep(t *testing.T) {
	p := newFollowTestProvider()
	c := &followCollector{}
	p.SetSender(c.sender)
	p.SetCloser(c.closer)
	p.followCmdFn = fakeFollowCmd(`printf 'first\nERROR oops\nlast\n'; exec sleep 30`)

	if _, err := p.FollowStart(followParams("f1", "ERROR")); err != nil {
		t.Fatalf("FollowStart: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for len(c.gotLines()) < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	lines := c.gotLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "ERROR oops") {
		t.Fatalf("expected only grep hit, got %q", lines)
	}

	// stop 杀进程并通知
	if _, err := p.FollowStop(map[string]interface{}{"followId": "f1"}); err != nil {
		t.Fatalf("FollowStop: %v", err)
	}
	waitClose(t, c, "stopped")
	// close 幂等：pumpFollow 退出路径不重复发
	if n := len(c.gotCloses()); n != 1 {
		t.Fatalf("expected exactly one close, got %v", c.gotCloses())
	}
}

func TestLogsFollowReplaceSameID(t *testing.T) {
	p := newFollowTestProvider()
	c := &followCollector{}
	p.SetSender(c.sender)
	p.SetCloser(c.closer)
	p.followCmdFn = fakeFollowCmd(`printf 'tick\n'; exec sleep 30`)

	if _, err := p.FollowStart(followParams("f1", "")); err != nil {
		t.Fatalf("FollowStart 1: %v", err)
	}
	// 同 followId 二次 start：旧会话被杀（closeFn 在锁外异步触发，先等其落定）
	if _, err := p.FollowStart(followParams("f1", "")); err != nil {
		t.Fatalf("FollowStart 2: %v", err)
	}
	waitClose(t, c, "replaced")

	// 覆盖后 map 只剩新会话
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		p.fs.mu.Lock()
		n := len(p.fs.follows)
		p.fs.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	p.fs.mu.Lock()
	n := len(p.fs.follows)
	p.fs.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 active follow after replace, got %d", n)
	}

	// 新会话仍在推数据（旧会话已被杀）
	if _, err := p.FollowStop(map[string]interface{}{"followId": "f1"}); err != nil {
		t.Fatalf("FollowStop: %v", err)
	}
}

func TestLogsFollowByteLimit(t *testing.T) {
	p := newFollowTestProvider()
	c := &followCollector{}
	p.SetSender(c.sender)
	p.SetCloser(c.closer)
	// 5MB 分行输出，越过 4MB 会话上限
	p.followCmdFn = fakeFollowCmd(`yes xxxxxxxxxx | head -c 5242880`)

	if _, err := p.FollowStart(followParams("f1", "")); err != nil {
		t.Fatalf("FollowStart: %v", err)
	}
	waitClose(t, c, "limit")

	p.fs.mu.Lock()
	n := len(p.fs.follows)
	p.fs.mu.Unlock()
	if n != 0 {
		t.Fatalf("follow session should be removed after limit, got %d", n)
	}
}

func TestLogsFollowValidationAndGuards(t *testing.T) {
	// 未注入 sender/closer：follow 不可用
	pNoHooks := NewLogsProvider(nil)
	if _, err := pNoHooks.FollowStart(followParams("f1", "")); err == nil {
		t.Fatal("expected error when sender/closer not injected")
	}

	p := newFollowTestProvider()
	p.followCmdFn = fakeFollowCmd(`exec sleep 30`)

	cases := []struct {
		name   string
		params map[string]interface{}
	}{
		{"missing followId", map[string]interface{}{"type": "systemd", "source": "a.service"}},
		{"blank followId", map[string]interface{}{"followId": "  ", "type": "systemd", "source": "a.service"}},
		{"empty params", map[string]interface{}{}},
		{"bad type", map[string]interface{}{"followId": "f1", "type": "foo", "source": "a"}},
		{"bad source", map[string]interface{}{"followId": "f1", "type": "systemd", "source": "a; rm -rf"}},
		{"bad tail", map[string]interface{}{"followId": "f1", "type": "systemd", "source": "a", "tail": 5000}},
	}
	for _, tc := range cases {
		if _, err := p.FollowStart(tc.params); err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}

	// 会话上限：塞满 16 个再 start 拒绝
	for i := 0; i < logsFollowMaxActive; i++ {
		p.fs.follows[strings.Repeat("x", i+1)] = &logsFollowSession{}
	}
	if _, err := p.FollowStart(followParams("f1", "")); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("expected too-many error, got %v", err)
	}

	// stop 不存在的 followId 幂等成功
	if _, err := p.FollowStop(map[string]interface{}{"followId": "nope"}); err != nil {
		t.Fatalf("FollowStop unknown id: %v", err)
	}
	// stop 缺 followId 报错
	if _, err := p.FollowStop(map[string]interface{}{}); err == nil {
		t.Fatal("expected error for missing followId")
	}
}

func TestLogsFollowProcessExitCloses(t *testing.T) {
	p := newFollowTestProvider()
	c := &followCollector{}
	p.SetSender(c.sender)
	p.SetCloser(c.closer)
	// 输出完即退出：容器停止等场景 → reason=exited
	p.followCmdFn = fakeFollowCmd(`printf 'bye\n'`)

	if _, err := p.FollowStart(followParams("f1", "")); err != nil {
		t.Fatalf("FollowStart: %v", err)
	}
	waitClose(t, c, "exited")
	// once 幂等：exit 路径已 close，后续 stop 不重复发
	if _, err := p.FollowStop(map[string]interface{}{"followId": "f1"}); err != nil {
		t.Fatalf("FollowStop after exit: %v", err)
	}
	if n := len(c.gotCloses()); n != 1 {
		t.Fatalf("expected exactly one close, got %v", c.gotCloses())
	}
}
