//go:build windows
// +build windows

package main

import (
	"bytes"
	"flag"
	"runtime"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/agent"
)

// TestRunServiceDispatch 覆盖 run() 的 service 分支与 handleService 的各子命令。
// 注意：涉及 SCM 实际读写的用例（uninstall/start/stop/status 未注册、
// run 非 SCM）只断言退出码——输出文案随权限上下文而变（管理员输出
// 「未注册」，非管理员输出「需要管理员权限」），且两者行为都正确。
// install 的完整注册链由 nightly 的 Build Agent Installer job 真机走查。
func TestRunServiceDispatch(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    int
		wantOut string
	}{
		{"service no args", []string{"cockpit-agent", "service"}, 1, "用法:"},
		{"service unknown", []string{"cockpit-agent", "service", "foobar"}, 1, "未知子命令"},
		// install 无 -server：svcBind 后 Validate 先于任何 SCM 访问失败，
		// 输出与权限无关（参数解析失败才走「参数错误」分支）
		{"service install missing server", []string{"cockpit-agent", "service", "install"}, 1, "missing required -server flag"},
		{"service uninstall not exist", []string{"cockpit-agent", "service", "uninstall"}, 1, ""},
		{"service start not exist", []string{"cockpit-agent", "service", "start"}, 1, ""},
		{"service stop not exist", []string{"cockpit-agent", "service", "stop"}, 1, ""},
		{"service status not exist", []string{"cockpit-agent", "service", "status"}, 1, ""},
		// run 非 SCM：StartServiceCtrlDispatcher 必失败 → 退出 1；
		// 「service dispatch error」经 log 输出到 %ProgramData% 日志文件而非 stdout
		{"service run (not in SCM)", []string{"cockpit-agent", "service", "run", "-server", "ws://localhost:9000/ws"}, 1, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			code := run(tt.args, &buf)
			if code != tt.want {
				t.Errorf("exit code: got %d want %d\noutput: %s", code, tt.want, buf.String())
			}
			if tt.wantOut != "" && !strings.Contains(buf.String(), tt.wantOut) {
				t.Errorf("output missing %q: %s", tt.wantOut, buf.String())
			}
		})
	}
}

// TestHandleStartRedirectServiceStart 覆盖 handleStart -> redirectServiceStart
// 的非 SCM 分支（控制台运行恒为 false，流程继续到 Validate 失败）。
// 注意：故意不带 -server——带合法地址会真正启动 agent（Run 阻塞重连），
// 测试会挂死；这里断言「非 SCM 上下文未拦截 + 参数校验照常生效」。
func TestHandleStartRedirectServiceStart(t *testing.T) {
	var buf bytes.Buffer
	args := []string{"cockpit-agent", "start"}
	code := run(args, &buf)
	if code != 1 {
		t.Errorf("expected exit 1 (validate fail), got %d: %s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "错误") {
		t.Errorf("expected validate error output, got: %s", buf.String())
	}
}

// TestSvcBindAndArgs 覆盖 svcBind/svcArgsFrom 逻辑
func TestSvcBindAndArgs(t *testing.T) {
	cmd, _, err := svcBind([]string{"-server", "ws://test:9000/ws", "-id", "agent-1", "-secret", "sec", "-region", "cn", "-zone", "z1", "-labels", "k=v", "-ssh-keys", "/ssh"})
	if err != nil {
		t.Fatalf("svcBind error: %v", err)
	}
	if cmd.Server != "ws://test:9000/ws" {
		t.Errorf("Server: got %q", cmd.Server)
	}
	if cmd.ID != "agent-1" {
		t.Errorf("ID: got %q", cmd.ID)
	}
	if cmd.Secret != "sec" {
		t.Errorf("Secret: got %q", cmd.Secret)
	}
	if cmd.Region != "cn" {
		t.Errorf("Region: got %q", cmd.Region)
	}
	if cmd.Zone != "z1" {
		t.Errorf("Zone: got %q", cmd.Zone)
	}
	if cmd.Labels != "k=v" {
		t.Errorf("Labels: got %q", cmd.Labels)
	}
	if cmd.SSHKeys != "/ssh" {
		t.Errorf("SSHKeys: got %q", cmd.SSHKeys)
	}

	args := svcArgsFrom(cmd)
	expected := []string{"service", "run", "-server", "ws://test:9000/ws", "-id", "agent-1", "-secret", "sec", "-region", "cn", "-zone", "z1", "-labels", "k=v", "-ssh-keys", "/ssh"}
	for i, a := range expected {
		if i >= len(args) || args[i] != a {
			t.Errorf("args[%d]: got %q want %q (full: %v)", i, args[i], a, args)
			break
		}
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

// TestBinPath 覆盖 binPath 转义拼接（含空格路径的引号包裹）
func TestBinPath(t *testing.T) {
	exe := `C:\Program Files\CockpitAgent\cockpit-agent.exe`
	args := []string{"service", "run", "-server", "ws://test:9000/ws"}
	bp := binPath(exe, args)
	if !strings.HasPrefix(bp, `"C:\Program Files\CockpitAgent\cockpit-agent.exe"`) {
		t.Errorf("binPath missing quoted exe: %s", bp)
	}
	if !strings.Contains(bp, `-server`) || !strings.Contains(bp, `ws://test:9000/ws`) {
		t.Errorf("binPath missing args: %s", bp)
	}
}

// TestSvcRunDispatcherNotInSCM 覆盖 svcRunDispatcher 非 SCM 路径（svc.Run
// -> StartServiceCtrlDispatcher 在非服务进程必失败，应返回 1）
func TestSvcRunDispatcherNotInSCM(t *testing.T) {
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

// TestVersionFlag 覆盖 version 子命令
func TestVersionFlag(t *testing.T) {
	var buf bytes.Buffer
	code := run([]string{"cockpit-agent", "version"}, &buf)
	if code != 0 {
		t.Errorf("version exit: want 0 got %d", code)
	}
	if !strings.Contains(buf.String(), "Cockpit Agent v") {
		t.Errorf("version output: %s", buf.String())
	}
}

// TestRunUsage 覆盖 run() 无参数/未知命令分支
func TestRunUsage(t *testing.T) {
	var buf bytes.Buffer
	code := run([]string{"cockpit-agent"}, &buf)
	if code != 1 {
		t.Errorf("no args: want 1 got %d", code)
	}
	if !strings.Contains(buf.String(), "用法:") {
		t.Errorf("usage output: %s", buf.String())
	}

	buf.Reset()
	code = run([]string{"cockpit-agent", "unknowncmd"}, &buf)
	if code != 1 {
		t.Errorf("unknown cmd: want 1 got %d", code)
	}
	if !strings.Contains(buf.String(), "Unknown command") {
		t.Errorf("unknown cmd output: %s", buf.String())
	}
}

// TestStartCmdBind 覆盖 startCmd.BindWithUsage（通过 run -> handleStart 的
// flag 面与 StartCmd 字段绑定）
func TestStartCmdBind(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd := &agent.StartCmd{Version: "test"}
	cmd.BindWithUsage(fs, agent.StartUsage{
		Server:  "Server WebSocket 地址 (必需)",
		ID:      "Agent ID",
		Secret:  "Secret",
		Region:  "Region",
		Zone:    "Zone",
		Labels:  "Labels",
		Bias:    "Bias",
		SSHKeys: "SSHKeys",
	})
	_ = fs.Parse([]string{"-server", "ws://localhost:9000/ws", "-id", "test-1"})
	if cmd.Server != "ws://localhost:9000/ws" {
		t.Errorf("Server not bound: %s", cmd.Server)
	}
	if cmd.ID != "test-1" {
		t.Errorf("ID not bound: %s", cmd.ID)
	}
}

// TestMainOSArch 确保测试只在 windows/amd64 跑（CI matrix 已保证）
func TestMainOSArch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	if runtime.GOARCH != "amd64" {
		t.Skip("amd64-only test (CI matrix)")
	}
}
