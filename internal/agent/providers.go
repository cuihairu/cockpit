package agent

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/cuihairu/cockpit/internal/agent/rpc"
	"github.com/cuihairu/cockpit/internal/docker"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// setupProviders 根据检测到的 capabilities 注册 RPC Provider。
//
// 注册规则：
//   - SystemProvider：始终注册（内置基础能力）
//   - DockerProvider：检测到 "docker-api" capability 时注册，host 取自 Capability.Endpoint
//   - StackProvider：跟随 docker-api capability，另需 linux/darwin + docker compose CLI 可用
//   - PVEProvider：检测到 "pve-api" capability 且环境变量 PVE_TOKEN_ID / PVE_TOKEN_SECRET 同时存在时注册
//   - OpenWrtProvider：检测到 "openwrt" capability 且 OPENWRT_HOST/OPENWRT_USER/OPENWRT_PASS 存在时注册
//   - NginxProvider：检测到 "nginx-proxy" capability（nginx 可执行存在）时注册
//   - TraefikProvider：检测到 "traefik-proxy" capability（动态目录存在）时注册
//   - LogsProvider：检测到 "logs" capability（journalctl/docker 至少一个存在）时注册
//   - DriftProvider：检测到 "drift" capability（nginx/cron/stack 任一存在）时注册，
//     并向三者注入同一个基线挂钩
//
// 单个 Provider 初始化失败仅记录日志，不影响 Agent 基础心跳。
func (a *Agent) setupProviders() {
	if a.rpc == nil {
		return
	}

	// 漂移基线存储：nginx/cron/stack 写路径挂钩共用一个实例（见 drift-design.md）
	baseline := rpc.NewDriftBaseline("")

	// 1. SystemProvider 始终注册
	a.rpc.RegisterProvider(rpc.NewSystemProvider())

	// 1.5 Backup Provider：Linux 文件打包零外部依赖，无条件注册（与
	// detectCapabilities 追加 backup capability 的条件一致）
	if runtime.GOOS == "linux" {
		a.rpc.RegisterProvider(rpc.NewBackupProvider(rpc.BackupConfig{}))
	}

	// 1.6 File Provider：远程文件管理，Go 标准库实现全平台可用，
	// 无条件注册（与 detectCapabilities 追加 file capability 的条件一致）
	a.rpc.RegisterProvider(rpc.NewFileProvider())

	// 2. 按检测到的能力注册
	for _, cap := range a.capabilities {
		switch cap.Type {
		case "docker-api":
			a.registerDockerProvider(cap)
			a.registerStackProvider(cap, baseline)
		case "pve-api":
			a.registerPVEProvider(cap)
		case "openwrt":
			a.registerOpenWrtProvider(cap)
		case "nginx-proxy":
			// Nginx 反代管理（见 docs/guide/proxy-design.md）
			np := rpc.NewNginxProvider(rpc.NginxConfig{})
			np.SetBaseline(baseline)
			a.rpc.RegisterProvider(np)
		case "traefik-proxy":
			// Traefik 反代管理（见 docs/guide/proxy-design.md M2）；
			// 动态目录取自探测阶段写入的 capability metadata
			dir, _ := cap.Metadata["dynamicDir"].(string)
			tp := rpc.NewTraefikProvider(dir, nil)
			tp.SetBaseline(baseline)
			a.rpc.RegisterProvider(tp)
		case "cron":
			// Crontab 任务管理（见 docs/guide/cron-design.md）
			cp := rpc.NewCronProvider(nil)
			cp.SetBaseline(baseline)
			a.rpc.RegisterProvider(cp)
		case "logs":
			// 远程日志查询（见 docs/guide/logs-design.md）；M2 实时尾随经
			// proxy 通道回推（proxyId="logs:<followId>"，与 terminal 前缀同构）
			lp := rpc.NewLogsProvider(nil)
			lp.SetSender(func(proxyID string, data []byte) {
				a.proxyHandler.SendMessage(protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
					"proxyId": proxyID,
					"connId":  proxyID, // follow 无对端连接，server 只按 proxyId 前缀分派
					"data":    data,
				}))
			})
			lp.SetCloser(func(proxyID, reason string) {
				a.proxyHandler.SendClose(proxyID, proxyID, reason)
			})
			a.rpc.RegisterProvider(lp)
		case "drift":
			// 防漂移检测（见 docs/guide/drift-design.md）；cron 段检查
			// 需要 crontab 命令，复用 cron capability 探测结果
			dp := rpc.NewDriftProvider(baseline, rpc.DriftConfig{})
			if a.hasCapability("cron") {
				dp.SetCronRunner()
			}
			a.rpc.RegisterProvider(dp)
		case "overlay":
			// 组网工具观测（见 docs/guide/overlay-design.md）
			a.rpc.RegisterProvider(rpc.NewOverlayProvider(nil))
		case "hardware-monitor":
			// SMART 磁盘健康观测（见 docs/guide/disk-health-design.md）；
			// 无 smartctl 的主机 provider 返回 available=false，不报错
			a.rpc.RegisterProvider(rpc.NewSmartProvider(nil))
		case "ddns":
			// 公网出口 IP 探测（见 docs/guide/ddns-design.md D4）
			a.rpc.RegisterProvider(rpc.NewDDNSProvider(nil))
		case "service":
			// 服务管理（见 docs/guide/service-design.md D9/D10）：按 backend
			// 区分 systemd / Windows SCM / macOS launchd
			switch cap.Metadata["backend"] {
			case "windows-scm":
				a.rpc.RegisterProvider(rpc.NewWindowsServiceProvider())
			case "launchd":
				a.rpc.RegisterProvider(rpc.NewLaunchdServiceProvider(nil))
			default:
				a.rpc.RegisterProvider(rpc.NewServiceProvider(nil))
			}
		case "nas":
			// NAS 存储观测（见 docs/guide/nas-design.md）
			a.rpc.RegisterProvider(rpc.NewNasProvider(nil))
		}
	}
}

// hasCapability 是否检测到指定能力
func (a *Agent) hasCapability(t string) bool {
	for _, c := range a.capabilities {
		if c.Type == t {
			return true
		}
	}
	return false
}

// registerDockerProvider 注册 Docker Provider
func (a *Agent) registerDockerProvider(cap protocol.Capability) {
	host := cap.Endpoint
	if host == "" {
		host = os.Getenv("DOCKER_HOST")
	}
	if host == "" {
		log.Printf("Skip docker provider: no endpoint detected")
		return
	}

	p, err := rpc.NewDockerProvider(host)
	if err != nil {
		log.Printf("Failed to create docker provider: %v", err)
		return
	}
	a.rpc.RegisterProvider(p)
}

// registerStackProvider 注册 Compose Stack Provider。
//
// 叠加条件：docker-api capability 带明确 endpoint（与 docker provider
// 同一纪律，endpoint 缺失时不兜底连默认 socket）+ linux/darwin +
// docker compose CLI 可用 + Docker daemon 可连。任一不满足则跳过，
// 不影响容器管理（方案见 docs/guide/stack-deploy-design.md）。
// stacks 根目录可用 COCKPIT_STACKS_DIR 覆盖，默认 /var/lib/cockpit/stacks。
func (a *Agent) registerStackProvider(cap protocol.Capability, baseline rpc.BaselineRecorder) {
	host := cap.Endpoint
	if host == "" {
		host = os.Getenv("DOCKER_HOST")
	}
	if host == "" {
		log.Printf("Skip stack provider: no endpoint detected")
		return
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		log.Printf("Skip stack provider: unsupported OS %s", runtime.GOOS)
		return
	}
	if !rpc.ComposeAvailable() {
		log.Printf("Skip stack provider: docker compose CLI not available")
		return
	}

	dockerClient, err := docker.NewClient(docker.Config{Host: host, Timeout: 30 * time.Second})
	if err != nil {
		log.Printf("Skip stack provider: connect docker daemon: %v", err)
		return
	}

	sp := rpc.NewStackProvider(rpc.StackConfig{
		Dir:    os.Getenv("COCKPIT_STACKS_DIR"),
		Docker: dockerClient,
	})
	sp.SetBaseline(baseline)
	a.rpc.RegisterProvider(sp)
}

// registerPVEProvider 注册 PVE Provider
//
// PVE API 需要 token 认证；endpoint 在 capability 中，但 token 必须从环境变量读取
// （capability payload 不应包含敏感凭据）。
func (a *Agent) registerPVEProvider(cap protocol.Capability) {
	endpoint := cap.Endpoint
	if endpoint == "" {
		endpoint = os.Getenv("PVE_URL")
	}
	tokenID := os.Getenv("PVE_TOKEN_ID")
	tokenSecret := os.Getenv("PVE_TOKEN_SECRET")

	if endpoint == "" || tokenID == "" || tokenSecret == "" {
		log.Printf("Skip pve provider: missing endpoint or token (PVE_URL/PVE_TOKEN_ID/PVE_TOKEN_SECRET)")
		return
	}

	a.rpc.RegisterProvider(rpc.NewPVEProvider(endpoint, tokenID, tokenSecret))
}

// registerOpenWrtProvider 注册 OpenWrt Provider
//
// OpenWrt detector 只检测本地特征，但 HTTP API 访问需要凭据。
// 凭据来源：OPENWRT_HOST / OPENWRT_PORT / OPENWRT_USER / OPENWRT_PASS 环境变量。
func (a *Agent) registerOpenWrtProvider(cap protocol.Capability) {
	host := os.Getenv("OPENWRT_HOST")
	user := os.Getenv("OPENWRT_USER")
	pass := os.Getenv("OPENWRT_PASS")
	if host == "" || user == "" || pass == "" {
		log.Printf("Skip openwrt provider: missing OPENWRT_HOST/OPENWRT_USER/OPENWRT_PASS")
		return
	}

	port := 443
	if pStr := os.Getenv("OPENWRT_PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			port = p
		}
	}

	a.rpc.RegisterProvider(rpc.NewOpenWrtProvider(host, port, user, pass))
}
