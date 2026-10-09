package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func covRun(args ...string) (int, string) {
	var buf bytes.Buffer
	code := run(append([]string{"cockpit-agent"}, args...), &buf)
	return code, buf.String()
}

func TestCovRunNoArgs(t *testing.T) {
	code, out := covRun()
	if code != 1 || !strings.Contains(out, "Cockpit Agent") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestCovRunVersionVariants(t *testing.T) {
	for _, arg := range []string{"version", "-v", "--version"} {
		code, out := covRun(arg)
		if code != 0 || !strings.Contains(out, "Cockpit Agent v") {
			t.Fatalf("run %s: code=%d out=%q", arg, code, out)
		}
	}
}

func TestCovRunUnknownCommand(t *testing.T) {
	code, out := covRun("bogus")
	if code != 1 || !strings.Contains(out, "Unknown command: bogus") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestCovHandleStartHelp(t *testing.T) {
	code, out := covRun("start", "-h")
	if code != 0 || !strings.Contains(out, "启动 Cockpit Agent") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestCovHandleStartValidateFail(t *testing.T) {
	code, out := covRun("start")
	if code != 1 || !strings.Contains(out, "missing required -server flag") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

// 指向必然拒绝连接的地址：agent.StartCmd.Run 返回 connect error → 退出码 1
func TestCovHandleStartConnectRefused(t *testing.T) {
	code, _ := covRun("start", "-server", "ws://127.0.0.1:1/ws")
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

// SCM 拦截命中（Windows 服务直挂形态，linux stub 恒 false，注入 true 覆盖
// 命中分支；注入点见 main.go serviceRedirect 注释）
func TestCovHandleStartServiceRedirect(t *testing.T) {
	saved := serviceRedirect
	serviceRedirect = func(args []string, stdout io.Writer) bool { return true }
	t.Cleanup(func() { serviceRedirect = saved })

	var buf bytes.Buffer
	if code := handleStart([]string{"start"}, &buf); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
}
