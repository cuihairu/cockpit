// cockpit-probe-agent 服务检测 agent（docs/design/service-detect-agent.md）：
// 探测目标清单轮询 + 防抖状态机 + 故障窗口；standalone 本地模式，配置
// server 后注册/心跳/probe_report 全链上行（B5）。main 薄壳，逻辑在
// probeagent 包。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/cuihairu/cockpit/core/healthprobe"
	"github.com/cuihairu/cockpit/core/platform"
	"github.com/cuihairu/cockpit/internal/probeagent"
)

// version 本地开发缺省值；构建注入 -ldflags "-X main.version=..."。
var version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run 命令主体，返回进程退出码（可测入口）。
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cockpit-probe-agent", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath(), "探测目标清单 YAML 路径")
	statusFile := fs.String("status-file", "", "观测快照 JSON 落盘路径（可选）")
	once := fs.Bool("once", false, "探测全部目标各一次后退出（cron/验收用）")
	showVersion := fs.Bool("version", false, "显示版本")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "cockpit-probe-agent v%s\n", version)
		return 0
	}

	cfg, err := probeagent.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, "load config:", err)
		return 1
	}

	a, err := probeagent.New(cfg)
	if err != nil {
		fmt.Fprintln(stderr, "init agent:", err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 优雅退出：平台信号集（core/platform Signals 契约，同 internal/agent
	// startcmd 先例）；0=主动停，交平台监管方区分。注册尽早——先于一切
	// 可观察副作用（状态文件）。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, gracefulSignals()...)
	go func() {
		<-sigCh
		cancel()
	}()

	// 上行链（B5）：server 配置即注册/心跳/probe_report 全链；-once 本地
	// 快照模式不装配。迁移即发经 Notify 交 worker（Guard 锁内回调契约：
	// 只入队信号不回读，兜底 10×interval 由 worker 自转）。
	var upstream *probeagent.Upstream
	if cfg.Server != "" && !*once {
		upstream = probeagent.NewUpstream(a, cfg, version)
		go func() { _ = upstream.Run() }()
		defer upstream.Stop()
	}

	// 状态文件：启动写一次 + 每次定性迁移重写 + 退出终态。落盘走异步
	// writer——Guard 在锁内回调 onEvent（core/healthprobe/state.go 契约），
	// 出口同步回读 Agent 状态会自死锁，故迁移事件经 channel 交写盘 goroutine。
	// 双出口（写盘+上行）合入同一 Alerter：迁移事件均非阻塞分发，写盘
	// 终态与上行兜底各自兜底丢失窗口。
	var transitions chan healthprobe.Transition
	var written chan struct{}
	a.SetAlerter(alerterFunc(func(tr healthprobe.Transition) {
		if transitions != nil {
			select {
			case transitions <- tr:
			default:
			}
		}
		if upstream != nil {
			upstream.Notify()
		}
	}))
	if *statusFile != "" {
		transitions = make(chan healthprobe.Transition, 8)
		written = make(chan struct{})
		go func() {
			defer close(written)
			for range transitions {
				_ = probeagent.WriteStatusFile(*statusFile, a.Snapshots())
			}
		}()
		_ = probeagent.WriteStatusFile(*statusFile, a.Snapshots())
	}

	// flushStatus 停写盘 goroutine 并落终态（收尾幂等）。
	flushStatus := func() {
		if transitions == nil {
			return
		}
		close(transitions)
		<-written
		transitions = nil
		_ = probeagent.WriteStatusFile(*statusFile, a.Snapshots())
	}

	if *once {
		a.ProbeAll(ctx)
		flushStatus()
		return 0
	}

	a.Run(ctx)
	signal.Stop(sigCh)
	flushStatus()
	return 0
}

// defaultConfigPath 缺省配置路径 = platform Paths 契约 ConfigDir/probe.yaml
// （Paths 契约首个消费方，简档 §5）。
func defaultConfigPath() string {
	if h := platform.Current(); h != nil {
		if p := h.Paths(); p.ConfigDir != "" {
			return filepath.Join(p.ConfigDir, "probe.yaml")
		}
	}
	return "probe.yaml"
}

// gracefulSignals 平台优雅退出信号集（Current 兜底 unix 全集）。
func gracefulSignals() []os.Signal {
	if h := platform.Current(); h != nil {
		if s := h.Signals(); len(s) > 0 {
			return s
		}
	}
	return []os.Signal{syscall.SIGTERM, os.Interrupt}
}

// alerterFunc 函数适配 Alerter 接口。
type alerterFunc func(healthprobe.Transition)

// Alert 实现 probeagent.Alerter。
func (f alerterFunc) Alert(tr healthprobe.Transition) { f(tr) }
