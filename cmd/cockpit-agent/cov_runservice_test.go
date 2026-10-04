//go:build windows
// +build windows

package main

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/sys/windows/svc/mgr"
)

// adminCtx 探测管理员上下文：mgr.Connect 成功即管理员（svcInstall/
// svcUninstall/svcControl/svcStatus 依赖同一权限判定，非管理员
// 一律「需要管理员权限」）。
func adminCtx() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	m.Disconnect()
	return true
}

// TestRunServiceDispatch 覆盖 run() 的 service 分支与 handleService 的各子命令。
// 管理员/非管理员上下文文案不同（未注册 vs 需要管理员权限），wantOuts 任一命中即可。
func TestRunServiceDispatch(t *testing.T) {
	admin := adminCtx()
	tests := []struct {
		name        string
		args        []string
		want        int
		wantOuts    []string
		skipIfAdmin bool
	}{
		{"service no args", []string{"cockpit-agent", "service"}, 1, []string{"用法:"}, false},
		{"service unknown", []string{"cockpit-agent", "service", "foobar"}, 1, []string{"未知子命令"}, false},
		// 缺 -server：Validate 在 mgr.Connect 之前失败，两种上下文同文案
		{"service install missing server", []string{"cockpit-agent", "service", "install"}, 1, []string{"missing required -server flag"}, false},
		// 非管理员：mgr.Connect 失败即退出；管理员会真实注册服务（副作用），
		// 成功路径由 nightly 走查在真 Windows 上覆盖
		{"service install ok", []string{"cockpit-agent", "service", "install", "-server", "ws://localhost:9000/ws"}, 1, []string{"需要管理员权限"}, true},
		// 未注册服务：非管理员停在 Connect（需要管理员权限），管理员走到 OpenService（未注册）
		{"service uninstall not exist", []string{"cockpit-agent", "service", "uninstall"}, 1, []string{"未注册", "需要管理员权限"}, false},
		{"service start not exist", []string{"cockpit-agent", "service", "start"}, 1, []string{"未注册", "需要管理员权限"}, false},
		{"service stop not exist", []string{"cockpit-agent", "service", "stop"}, 1, []string{"未注册", "需要管理员权限"}, false},
		{"service status not exist", []string{"cockpit-agent", "service", "status"}, 1, []string{"未注册", "需要管理员权限"}, false},
		// svc.Run 非 SCM 上下文报错退出；文案进 ProgramData 日志（服务无 stdout），只断言退出码
		{"service run (not in SCM)", []string{"cockpit-agent", "service", "run", "-server", "ws://localhost:9000/ws"}, 1, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipIfAdmin && admin {
				t.Skip("管理员上下文会真实注册服务；成功路径由 nightly 走查覆盖")
			}
			var buf bytes.Buffer
			code := run(tt.args, &buf)
			if code != tt.want {
				t.Errorf("exit code: got %d want %d\noutput: %s", code, tt.want, buf.String())
			}
			if len(tt.wantOuts) > 0 {
				hit := false
				for _, want := range tt.wantOuts {
					if strings.Contains(buf.String(), want) {
						hit = true
						break
					}
				}
				if !hit {
					t.Errorf("output missing any of %v: %s", tt.wantOuts, buf.String())
				}
			}
		})
	}
}

// TestHandleStartRedirectServiceStart 覆盖 handleStart -> redirectServiceStart 的分支
// （非 SCM 上下文返回 false，不进入 svc.Run）
func TestHandleStartRedirectServiceStart(t *testing.T) {
	// 非 SCM 环境下 redirectServiceStart 应返回 false；handleStart 继续走
	// flag.Parse + Validate——缺 -server 确定性失败（不给真实服务器地址，
	// 避免 Validate 通过后 Run() 真的去连）
	var buf bytes.Buffer
	args := []string{"cockpit-agent", "start"}
	code := run(args, &buf)
	if code != 1 {
		t.Errorf("expected exit 1 (validate fail), got %d: %s", code, buf.String())
	}
}

// TestPrintServiceUsage 覆盖 printServiceUsage
func TestPrintServiceUsage(t *testing.T) {
	var buf bytes.Buffer
	printServiceUsage(&buf)
	out := buf.String()
	if !strings.Contains(out, "install") || !strings.Contains(out, "uninstall") || !strings.Contains(out, "run") {
		t.Errorf("usage missing keywords: %s", out)
	}
}

// TestBinPath 覆盖 binPath 转义拼接
func TestBinPath(t *testing.T) {
	exe := `C:\Program Files\CockpitAgent\cockpit-agent.exe`
	args := []string{"service", "run", "-server", `ws://test:9000/ws`}
	bp := binPath(exe, args)
	if !strings.HasPrefix(bp, `"C:\Program Files\CockpitAgent\cockpit-agent.exe"`) {
		t.Errorf("binPath missing quoted exe: %s", bp)
	}
	if !strings.Contains(bp, `-server`) || !strings.Contains(bp, `ws://test:9000/ws`) {
		t.Errorf("binPath missing args: %s", bp)
	}
}

// TestSvcRunDispatcherNotInSCM 覆盖 svcRunDispatcher 非 SCM 路径（svc.Run 返回错误）
func TestSvcRunDispatcherNotInSCM(t *testing.T) {
	// 非 SCM 上下文调用 svc.Run 会返回错误，svcRunDispatcher 应返回 1
	code := svcRunDispatcher()
	if code != 1 {
		t.Errorf("svcRunDispatcher outside SCM: want 1 got %d", code)
	}
}

// TestRedirectServiceStartNotInSCM 覆盖 redirectServiceStart 非 SCM 分支
func TestRedirectServiceStartNotInSCM(t *testing.T) {
	var buf bytes.Buffer
	ok := redirectServiceStart([]string{"-server", "ws://x"}, &buf)
	if ok {
		t.Errorf("redirectServiceStart outside SCM should return false")
	}
}
