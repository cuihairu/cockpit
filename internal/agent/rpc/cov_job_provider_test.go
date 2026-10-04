package rpc

import (
	"runtime"
	"strings"
	"testing"
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