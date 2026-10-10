package rpc

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// 平台无关的内置命令（sh -c / cmd /C 都直接支持）
const (
	testExecHello = "echo hello"
	// 非零退出：sh 用 exit 3；cmd 用 exit 3（ERRORLEVEL 即 3）
	testExecExit3 = "exit 3"
	// 长睡眠：超时测试用（仅 unix 侧跑）
	testExecSleep = "sleep 30"
)

func TestJobExecSuccess(t *testing.T) {
	p := NewJobProvider()
	if p.Type() != "job" {
		t.Fatalf("Type = %q", p.Type())
	}
	res, err := p.Call("exec", map[string]interface{}{"command": testExecHello})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m := res.(map[string]interface{})
	if code := intField(m, "exit_code"); code != 0 {
		t.Fatalf("exit_code = %d", code)
	}
	if out := stringField(m, "output"); !strings.Contains(out, "hello") {
		t.Fatalf("output = %q", out)
	}
	if e := stringField(m, "error"); e != "" {
		t.Fatalf("error = %q", e)
	}
	if d, ok := m["duration_ms"].(int64); !ok || d < 0 {
		t.Fatalf("duration_ms missing/bad: %v", m["duration_ms"])
	}

	// 缺省 timeout_s 也应执行成功
	if _, err := p.Call("exec", map[string]interface{}{"command": testExecHello, "timeout_s": 0}); err != nil {
		t.Fatalf("default timeout exec: %v", err)
	}
	// RPC JSON 路径 timeout_s 是 float64（直调测试传 int）——两型都收
	if _, err := p.Call("exec", map[string]interface{}{"command": testExecHello, "timeout_s": float64(1)}); err != nil {
		t.Fatalf("float64 timeout exec: %v", err)
	}
}

func TestJobExecNonZeroExit(t *testing.T) {
	p := NewJobProvider()
	res, err := p.Call("exec", map[string]interface{}{"command": testExecExit3})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m := res.(map[string]interface{})
	if code := intField(m, "exit_code"); code != 3 {
		t.Fatalf("exit_code = %d, want 3", code)
	}
}

func TestJobExecMissingCommand(t *testing.T) {
	p := NewJobProvider()
	if _, err := p.Call("exec", map[string]interface{}{}); err == nil ||
		!strings.Contains(err.Error(), "command is required") {
		t.Fatalf("missing command: err = %v", err)
	}
	if _, err := p.Call("exec", map[string]interface{}{"command": "   "}); err == nil {
		t.Fatalf("blank command should fail")
	}
}

func TestJobExecOverlimits(t *testing.T) {
	p := NewJobProvider()
	big := strings.Repeat("a", 20*1024)
	if _, err := p.Call("exec", map[string]interface{}{"command": big}); err == nil ||
		!strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized command: err = %v", err)
	}
	if _, err := p.Call("exec", map[string]interface{}{"command": "echo x", "timeout_s": 301}); err == nil ||
		!strings.Contains(err.Error(), "timeout too large") {
		t.Fatalf("oversized timeout: err = %v", err)
	}
}

func TestJobExecTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		// cmd /C 无类 sleep 的可靠内置；超时语义由 unix 侧覆盖
		t.Skip("windows: no reliable sleep builtin")
	}
	p := NewJobProvider()
	res, err := p.Call("exec", map[string]interface{}{"command": testExecSleep, "timeout_s": 1})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m := res.(map[string]interface{})
	if code := intField(m, "exit_code"); code != -1 {
		t.Fatalf("timeout exit_code = %d, want -1", code)
	}
	if e := stringField(m, "error"); !strings.Contains(e, "timed out") {
		t.Fatalf("timeout error = %q", e)
	}
}

func TestJobExecOutputTruncated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows: no head/tr pipe builtins")
	}
	p := NewJobProvider()
	res, err := p.Call("exec", map[string]interface{}{
		"command": "head -c 70000 /dev/zero | tr '\\0' 'a'",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m := res.(map[string]interface{})
	if trunc, _ := m["truncated"].(bool); !trunc {
		t.Fatalf("truncated flag = %v", m["truncated"])
	}
	if out := stringField(m, "output"); len(out) != jobExecMaxOutput {
		t.Fatalf("output len = %d, want %d", len(out), jobExecMaxOutput)
	}
}

func TestJobUnknownAction(t *testing.T) {
	p := NewJobProvider()
	if _, err := p.Call("nope", nil); err == nil ||
		!strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("unknown action: err = %v", err)
	}
}

// trimExecErr 纯函数：>512 截断保尾 + 短串原样 + 首尾空白清理
func TestJobTrimExecErr(t *testing.T) {
	long := strings.Repeat("x", 600)
	got := trimExecErr(long)
	if len(got) != 512+3 || !strings.HasSuffix(got, "...") {
		t.Fatalf("trimmed len = %d, want %d with ... suffix", len(got), 512+3)
	}
	if trimExecErr("short") != "short" {
		t.Fatalf("short string altered")
	}
	if trimExecErr("  pad  ") != "pad" {
		t.Fatalf("trim whitespace failed")
	}
}

// —— 测内小工具（避免与主包重名冲突）——

func intField(m map[string]interface{}, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func stringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// —— M2c 取消信号测试（C1-C5）——

// TestJobCancelKillsRunningExec：exec 在途时 cancel 触发 ctx 取消（进程组
// SIGKILL），CombinedOutput 提前返回；error 带 "timed out"（ctx 取消走
// DeadlineExceeded/Canceled 同分支）
func TestJobCancelKillsRunningExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows: no reliable sleep builtin for cancel test")
	}
	p := NewJobProvider()
	cmd := map[string]interface{}{"command": testExecSleep, "timeout_s": 60, "id": "job-cancel-1"}

	done := make(chan interface{}, 1)
	go func() {
		res, err := p.Call("exec", cmd)
		if err != nil {
			t.Errorf("exec error: %v", err)
			return
		}
		done <- res
	}()

	// 等待 exec 启动（sleep 30 会阻塞 CombinedOutput）
	time.Sleep(200 * time.Millisecond)

	// 下发取消
	res, err := p.Call("cancel", map[string]interface{}{"id": "job-cancel-1"})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	m := res.(map[string]interface{})
	if c, _ := m["cancelled"].(bool); !c {
		t.Fatalf("cancel should report cancelled=true, got %v", m)
	}

	// exec 应提前返回（被击杀）
	select {
	case execRes := <-done:
		em := execRes.(map[string]interface{})
		if code := intField(em, "exit_code"); code == 0 {
			t.Fatalf("killed exec should have non-zero exit_code, got %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("exec did not return after cancel")
	}
}

// TestJobCancelUnknownId：未知 ID 幂等成功（不报错，cancelled=false）
func TestJobCancelUnknownId(t *testing.T) {
	p := NewJobProvider()
	res, err := p.Call("cancel", map[string]interface{}{"id": "nonexistent"})
	if err != nil {
		t.Fatalf("cancel unknown id: %v", err)
	}
	m := res.(map[string]interface{})
	if c, _ := m["cancelled"].(bool); c {
		t.Fatalf("cancel of unknown id should report cancelled=false, got %v", m)
	}
}

// TestJobCancelEmptyId：无 ID 的 cancel 参数幂等成功
func TestJobCancelEmptyId(t *testing.T) {
	p := NewJobProvider()
	res, err := p.Call("cancel", map[string]interface{}{"id": ""})
	if err != nil {
		t.Fatalf("cancel empty id: %v", err)
	}
	m := res.(map[string]interface{})
	if c, _ := m["cancelled"].(bool); c {
		t.Fatalf("cancel of empty id should report cancelled=false, got %v", m)
	}
}

// TestJobExecWithoutId：无 ID 的 exec 不注册句柄，cancel 幂等空操作
func TestJobExecWithoutId(t *testing.T) {
	p := NewJobProvider()
	// 无 id 的 exec 应正常执行
	res, err := p.Call("exec", map[string]interface{}{"command": testExecHello})
	if err != nil {
		t.Fatalf("exec without id: %v", err)
	}
	m := res.(map[string]interface{})
	if code := intField(m, "exit_code"); code != 0 {
		t.Fatalf("exec without id exit_code = %d", code)
	}
	// 对任意 ID cancel 都应幂等成功（句柄表中无此条目）
	res2, err := p.Call("cancel", map[string]interface{}{"id": "any-id"})
	if err != nil {
		t.Fatalf("cancel after exec without id: %v", err)
	}
	m2 := res2.(map[string]interface{})
	if c, _ := m2["cancelled"].(bool); c {
		t.Fatalf("cancel after id-less exec should report cancelled=false, got %v", m2)
	}
}

// TestJobCancelAfterExecDone：exec 自然结束后 cancel 幂等（句柄已注销）
func TestJobCancelAfterExecDone(t *testing.T) {
	p := NewJobProvider()
	_, err := p.Call("exec", map[string]interface{}{"command": testExecHello, "id": "job-done-1"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	res, err := p.Call("cancel", map[string]interface{}{"id": "job-done-1"})
	if err != nil {
		t.Fatalf("cancel after done: %v", err)
	}
	m := res.(map[string]interface{})
	if c, _ := m["cancelled"].(bool); c {
		t.Fatalf("cancel after exec done should report cancelled=false, got %v", m)
	}
}