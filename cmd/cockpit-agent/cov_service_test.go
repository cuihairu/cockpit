//go:build !windows

package main

import (
	"strings"
	"testing"
)

// service 子命令分发（Windows 实现在 service_windows.go，此处覆盖
// linux/macOS 的指引分支与 usage 文案）

func TestCovServiceUsageInHelp(t *testing.T) {
	code, out := covRun()
	if code != 1 {
		t.Fatalf("code=%d", code)
	}
	for _, want := range []string{"service", "Windows 服务管理"} {
		if !strings.Contains(out, want) {
			t.Fatalf("usage missing %q: %q", want, out)
		}
	}
}

func TestCovServiceNoVerb(t *testing.T) {
	// linux 侧 service 子命令统一落平台指引（含无动词；Windows 的 usage
	// 分支由 GOOS=windows 编译覆盖 + nightly 真机走查）
	code, out := covRun("service")
	if code != 1 || !strings.Contains(out, "service 命令仅支持 Windows") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestCovServiceUnsupportedOnLinux(t *testing.T) {
	// 任意动词（install/run/status…）在非 Windows 都落到同一条指引
	for _, verb := range []string{"install", "run", "uninstall", "start", "stop", "status", "bogus"} {
		code, out := covRun("service", verb)
		if code != 1 || !strings.Contains(out, "service 命令仅支持 Windows") {
			t.Fatalf("verb %s: code=%d out=%q", verb, code, out)
		}
	}
}
