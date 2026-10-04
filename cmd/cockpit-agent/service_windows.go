//go:build windows

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cuihairu/cockpit/internal/agent"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// 与 install.ps1 的既有注册口径完全一致（同名/同描述/同失败重启策略），
// setup.exe 与脚本两条安装路径可互相识别、升级时互相覆盖。
const (
	svcName        = "CockpitAgent"
	svcDisplayName = "Cockpit Infrastructure Monitoring Agent"
	svcDescription = "Cockpit Agent - Connects to Cockpit Server for infrastructure monitoring"
)

// handleService `cockpit-agent service <verb>`：安装器（Inno Setup [Run]）与
// 运维手动注册共用这一入口
func handleService(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		printServiceUsage(stdout)
		return 1
	}
	switch args[0] {
	case "run":
		return svcRunDispatcher()
	case "install":
		return svcInstall(args[1:], stdout)
	case "uninstall":
		return svcUninstall(stdout)
	case "start":
		return svcControl(stdout, true)
	case "stop":
		return svcControl(stdout, false)
	case "status":
		return svcStatus(stdout)
	default:
		fmt.Fprintf(stdout, "未知子命令: %s\n\n", args[0])
		printServiceUsage(stdout)
		return 1
	}
}

func printServiceUsage(w io.Writer) {
	fmt.Fprintln(w, "用法: cockpit-agent service <install|uninstall|start|stop|status|run> [start 同款参数]")
	fmt.Fprintln(w, "  install   注册 Windows 服务（开机自启 + 失败自动重启）并启动，参数同 start")
	fmt.Fprintln(w, "  uninstall 停止并注销服务")
	fmt.Fprintln(w, "  start/stop/status 启动/停止/查询服务")
	fmt.Fprintln(w, "  run       服务入口（SCM 调用，勿手动执行）")
}

// svcInstall 注册（或升级已注册的）服务：Automatic 开机自启 + 崩溃自动重启
// 5s/10s/20s（24h 计数重置，同 install.ps1 的 sc failure 策略），随后启动
// （svcBind/svcArgsFrom 见 svcargs.go：平台无关的部分下沉，故能在 linux CI 覆盖）
func svcInstall(args []string, stdout io.Writer) int {
	cmd, err := svcBind(args)
	if err != nil {
		fmt.Fprintln(stdout, "参数错误:", err)
		return 1
	}
	if err := cmd.Validate(); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stdout, "无法确定自身路径:", err)
		return 1
	}
	exe, _ = filepath.Abs(exe)

	m, err := mgr.Connect()
	if err != nil {
		fmt.Fprintln(stdout, "需要管理员权限连接服务管理器:", err)
		return 1
	}
	defer m.Disconnect()

	recovery := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 20 * time.Second},
	}
	svcArgs := svcArgsFrom(cmd)

	if s, serr := m.OpenService(svcName); serr == nil {
		// 已注册：升级路径——停服务、改二进制参数（连接信息更新）、重启
		svcWaitStopped(s)
		cfg, cerr := s.Config()
		if cerr != nil {
			fmt.Fprintln(stdout, "读取服务配置失败:", cerr)
			s.Close()
			return 1
		}
		cfg.StartType = mgr.StartAutomatic
		cfg.BinaryPathName = binPath(exe, svcArgs)
		cfg.DisplayName = svcDisplayName
		cfg.Description = svcDescription
		if uerr := s.UpdateConfig(cfg); uerr != nil {
			fmt.Fprintln(stdout, "更新服务配置失败:", uerr)
			s.Close()
			return 1
		}
		if rerr := s.SetRecoveryActions(recovery, 86400); rerr != nil {
			fmt.Fprintln(stdout, "设置失败重启策略未成功（不影响使用）:", rerr)
		}
		if serr := s.Start(); serr != nil {
			fmt.Fprintln(stdout, "服务已升级但启动失败，请检查参数:", serr)
			s.Close()
			return 1
		}
		s.Close()
		fmt.Fprintf(stdout, "服务 %s 已升级并启动（参数: %s）\n", svcName, strings.Join(svcArgs, " "))
		return 0
	}

	s, cerr := m.CreateService(svcName, exe, mgr.Config{
		StartType:   mgr.StartAutomatic,
		DisplayName: svcDisplayName,
		Description: svcDescription,
	}, svcArgs...)
	if cerr != nil {
		fmt.Fprintln(stdout, "创建服务失败:", cerr)
		return 1
	}
	defer s.Close()
	if rerr := s.SetRecoveryActions(recovery, 86400); rerr != nil {
		fmt.Fprintln(stdout, "设置失败重启策略未成功（不影响使用）:", rerr)
	}
	if serr := s.Start(); serr != nil {
		fmt.Fprintln(stdout, "服务已注册但启动失败，请检查参数:", serr)
		return 1
	}
	fmt.Fprintf(stdout, "服务 %s 已注册（开机自启）并启动\n", svcName)
	return 0
}

// svcUninstall 停止并注销服务
func svcUninstall(stdout io.Writer) int {
	m, err := mgr.Connect()
	if err != nil {
		fmt.Fprintln(stdout, "需要管理员权限连接服务管理器:", err)
		return 1
	}
	defer m.Disconnect()

	s, err := m.OpenService(svcName)
	if err != nil {
		fmt.Fprintf(stdout, "服务 %s 未注册\n", svcName)
		return 1
	}
	defer s.Close()

	svcWaitStopped(s)
	if err := s.Delete(); err != nil {
		fmt.Fprintln(stdout, "注销服务失败:", err)
		return 1
	}
	fmt.Fprintf(stdout, "服务 %s 已停止并注销\n", svcName)
	return 0
}

// svcControl start/stop
func svcControl(stdout io.Writer, start bool) int {
	m, err := mgr.Connect()
	if err != nil {
		fmt.Fprintln(stdout, "需要管理员权限连接服务管理器:", err)
		return 1
	}
	defer m.Disconnect()

	s, err := m.OpenService(svcName)
	if err != nil {
		fmt.Fprintf(stdout, "服务 %s 未注册\n", svcName)
		return 1
	}
	defer s.Close()

	if start {
		if err := s.Start(); err != nil {
			fmt.Fprintln(stdout, "启动失败:", err)
			return 1
		}
		fmt.Fprintf(stdout, "服务 %s 已启动\n", svcName)
		return 0
	}
	if _, err := s.Control(svc.Stop); err != nil {
		fmt.Fprintln(stdout, "停止失败:", err)
		return 1
	}
	svcWaitStopped(s)
	fmt.Fprintf(stdout, "服务 %s 已停止\n", svcName)
	return 0
}

// svcStatus 查询运行状态与启动类型
func svcStatus(stdout io.Writer) int {
	m, err := mgr.Connect()
	if err != nil {
		fmt.Fprintln(stdout, "需要管理员权限连接服务管理器:", err)
		return 1
	}
	defer m.Disconnect()

	s, err := m.OpenService(svcName)
	if err != nil {
		fmt.Fprintf(stdout, "服务 %s 未注册\n", svcName)
		return 1
	}
	defer s.Close()

	q, err := s.Query()
	if err != nil {
		fmt.Fprintln(stdout, "查询失败:", err)
		return 1
	}
	cfg, err := s.Config()
	if err != nil {
		fmt.Fprintln(stdout, "读取配置失败:", err)
		return 1
	}
	auto := "手动"
	if cfg.StartType == mgr.StartAutomatic {
		auto = "开机自启"
	} else if cfg.StartType == mgr.StartDisabled {
		auto = "已禁用"
	}
	fmt.Fprintf(stdout, "服务 %s：%s（%s）\n", svcName, stateText(q.State), auto)
	return 0
}

func stateText(s svc.State) string {
	switch s {
	case svc.Stopped:
		return "已停止"
	case svc.StartPending:
		return "启动中"
	case svc.StopPending:
		return "停止中"
	case svc.Running:
		return "运行中"
	case svc.ContinuePending:
		return "恢复中"
	case svc.PausePending:
		return "暂停中"
	case svc.Paused:
		return "已暂停"
	default:
		return fmt.Sprintf("未知状态(%d)", s)
	}
}

// svcWaitStopped 发 Stop 后轮询至 Stopped（上限 30s，超时不再阻塞）
func svcWaitStopped(s *mgr.Service) {
	if st, err := s.Control(svc.Stop); err == nil && st.State != svc.Stopped {
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			q, err := s.Query()
			if err != nil || q.State == svc.Stopped {
				return
			}
			time.Sleep(300 * time.Millisecond)
		}
	}
}

// redirectServiceStart SCM 上下文识别：install.ps1 存量 New-Service 直接把
// `start -server ...` 注册为服务——此时必须转入 svc.Run 应答控制码，否则
// 30 秒后被 SCM 判定无响应杀死（本函数即对该存量路径的修复）
func redirectServiceStart(args []string, stdout io.Writer) bool {
	_ = args
	_ = stdout
	inSCM, err := svc.IsWindowsService()
	if err != nil || !inSCM {
		return false
	}
	return svcRunDispatcher() == 0
}

// svcRunDispatcher SCM 派发入口：日志重定向到 ProgramData（服务无控制台，
// 默认 stderr 不可见）
func svcRunDispatcher() int {
	if f, err := svcLogFile(); err == nil {
		defer f.Close()
		log.SetOutput(f)
	} else {
		log.SetOutput(io.Discard)
	}
	if err := svc.Run(svcName, &agentService{}); err != nil {
		log.Printf("service dispatch error: %v", err)
		return 1
	}
	return 0
}

// svcLogFile %ProgramData%\CockpitAgent\agent.log（目录与 install.ps1 的
// 配置留档目录同源，卸载时统一清理）
func svcLogFile() (*os.File, error) {
	dir := filepath.Join(os.Getenv("ProgramData"), "CockpitAgent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "agent.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

// agentService svc.Handler：包装 StartCmd 阻塞循环，Stop/Shutdown 触发
// agent 优雅退出（连接断开、注册表残留清理由 agent 内部完成）
type agentService struct{}

func (a *agentService) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}

	// 参数即进程命令行去掉 exe（`service run -server ...` 或存量直挂的
	// `start -server ...`），剥掉动词后按 start 同款 flag 解析
	flagArgs := svcStripVerbs(os.Args[1:])
	cmd := &agent.StartCmd{Version: version}
	fs := flag.NewFlagSet("service run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cmd.Bind(fs)
	if err := fs.Parse(flagArgs); err != nil {
		log.Printf("service flag parse error: %v", err)
		return false, 1
	}
	if err := cmd.Validate(); err != nil {
		log.Printf("service config error: %v", err)
		return false, 1
	}

	stop := make(chan struct{})
	errCh := make(chan error, 1)
	go func() { errCh <- runAgentSupervised(cmd, stop) }()

	status <- svc.Status{State: svc.Running, Accepts: accepts}
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			status <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending, WaitHint: 15000}
			close(stop)
			select {
			case <-errCh: // 正常优雅退出
			case <-time.After(12 * time.Second): // 兜底：不让 SCM 等过 WaitHint
			}
			return false, 0
		}
	}
	select {
	case <-errCh:
	case <-time.After(12 * time.Second):
	}
	return false, 0
}

// runAgentSupervised agent 进程内看护：Server 不可达（开机时网络未就绪、
// DNS 尚未生效、服务端临时下线）时按间隔重连，而不是让进程退出——进程退出
// 只剩 SCM failure actions 的 3 次重启额度（5s/10s/20s），耗尽后服务就
// 永久停了；崩溃/硬杀的兜底仍由 failure actions 负责。
// 返回 nil 表示收到停止信号后的正常收尾。
func runAgentSupervised(cmd *agent.StartCmd, stop <-chan struct{}) error {
	const retryDelay = 15 * time.Second
	for attempt := 1; ; attempt++ {
		err := cmd.RunService(stop)
		if err == nil {
			// RunService 在 stop 关闭时返回 nil（Stop 触发的优雅退出）
			return nil
		}
		log.Printf("agent run failed (attempt %d), retry in %s: %v", attempt, retryDelay, err)
		select {
		case <-stop:
			return nil
		case <-time.After(retryDelay):
		}
	}
}

// binPath BinaryPathName = 转义后的 exe 绝对路径 + 参数（UpdateConfig 用；
// CreateService 内部同样按 syscall.EscapeArg 逐段转义，两条路径口径一致）
func binPath(exe string, args []string) string {
	s := syscall.EscapeArg(exe)
	for _, a := range args {
		s += " " + syscall.EscapeArg(a)
	}
	return s
}
