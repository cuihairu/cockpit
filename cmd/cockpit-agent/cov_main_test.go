package main

import (
	"bytes"
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

// 指向必然拒绝连接的地址：AgentStartCmd.Run 返回 connect error → 退出码 1
func TestCovHandleStartConnectRefused(t *testing.T) {
	code, _ := covRun("start", "-server", "ws://127.0.0.1:1/ws")
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}
