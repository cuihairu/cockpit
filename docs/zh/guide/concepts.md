# 核心概念与源码术语表


## 资源模型

Cockpit 使用 Region、Zone、Agent 和资源对象描述个人基础设施：

```text
Region
  Zone
    Agent
      ComputeInstance / Service / Gateway / Storage

Domain
  Certificate
```

| 概念 | 说明 |
| --- | --- |
| Region | 地域或逻辑分组，例如 `home`、`jiangsu-huaian` |
| Zone | Region 下的机房、网络区域或可用区 |
| Agent | 运行在节点上的执行代理，主动连接 Server |
| ComputeInstance | VM、容器、物理机等计算实例 |
| Service | HTTP、TCP、数据库等服务 |
| Domain | 域名及注册信息 |
| Certificate | TLS 证书，通常挂在 Domain 下 |
| Gateway | OpenWrt、pfSense 等网关 |
| Storage | NFS、iSCSI、本地盘、Ceph 等存储 |

## 配置与状态

Cockpit 当前有两个主要数据面：

```text
Inventory YAML
    |
    | cockpit sync
    v
SQLite
    ^
    | heartbeat / runtime update
    |
Agent
```

Inventory YAML 描述“希望系统知道哪些资产”。SQLite 保存 API 和 Web UI 查询所需的运行时视图，包括同步后的资源、用户、告警、审计、代理配置、系统指标和 Agent 在线状态。

需要注意：

- `cockpit sync` 是手动 inventory 同步入口。
- `inventory.watch: true` 的 Server 热加载复用同一套 Syncer，会同步 Agent、Domain、Certificate、ComputeInstance、Service、Gateway、Storage 等资源。
- `inventory.strict: true` 会在 Server 启动时严格校验 inventory；默认 `false` 时启动失败风险会降级为日志。
- Agent 心跳会上报在线状态和系统指标，但不会直接修改 inventory 文件。

## Inventory 文件

当前主路径是单文件 YAML：

```yaml
version: v1

metadata:
  name: "My Home Lab"
  description: "Personal hybrid infrastructure"

regions:
  home:
    name: "Home"
    zones:
      datacenter:
        name: "Data Center"
        agents:
          server01:
            name: "Server 01"
            hostname: "server01.home.local"
            ip: "192.168.1.10"
            capabilities:
              - docker
              - system

domains:
  example-local:
    domain: "example.local"
    provider: "internal"

computeInstances:
  vm-web01:
    name: "Web Server VM"
    type: "vm"
    agent: "server01"
    region: "home"
    zone: "datacenter"
    cpu: 2
    memory: 2048
    disk: 40

services:
  web-service:
    name: "Web Service"
    type: "http"
    agent: "server01"
    url: "http://192.168.1.10:8080"
    interval: 60
```

顶层字段当前包括：

- `version`
- `metadata`
- `regions`
- `domains`
- `computeInstances`
- `services`
- `gateways`
- `storages`
- `resources`
- `templates`

`resources` / `templates` 已在 schema 中保留，但同步到数据库的主路径是明确的资源字段，例如 `computeInstances`、`services`、`gateways`、`storages`。

资源上的 `region` / `zone` 字段可省略：为空不阻塞同步，落库统一标记 `unknown`（显式值优先；`services`/`gateways`/`storages` 的地域写进各自 labels，用户在 labels 里显式给的键不被空字段覆盖）。Agent 注册侧同语义——未提供时默认 `unknown`（D-2026-10-08-1）。

## Agent 能力

Agent 启动时会运行能力检测器，并在注册消息中上报 capability：

```json
{
  "type": "docker-api",
  "endpoint": "unix:///var/run/docker.sock"
}
```

当前 capability 共 19 个，分两类来源。其中仅 `file`、`ddns`、`oob` 恒上报，其余 16 个按平台/环境条件满足才上报。

探测式（`internal/agent/detector/`，满足条件才上报）：

| 能力 | 检测依据 |
| --- | --- |
| `openwrt` | `/etc/openwrt_release`、`ubus`、`opkg` |
| `pve-api` | `PVE_URL` 或常见 PVE API 地址 |
| `docker-api` | `DOCKER_HOST` 或常见 Docker socket |
| `hardware-monitor` | `smartctl`、温度传感器、UPS 工具 |
| `firewall` | `nft` 或 `iptables` 可用（LookPath + `--version` 验证） |
| `overlay` | `zerotier-cli` / `tailscale` / `wg` / `frp` 任一可用 |
| `remote-services` | 本机常见远控端口探测（SSH 22 / RDP 3389 / VNC 590x / Telnet 23 / FTP 21） |

代码直报（`agent.go` `detectCapabilities`，按平台/环境满足即上报）：

| 能力 | 上报条件 |
| --- | --- |
| `backup` | Linux（标准库打包，零外部依赖） |
| `file` | 恒上报（标准库全平台可用） |
| `nginx-proxy` | 探测到 nginx 可执行与版本 |
| `traefik-proxy` | 探测到 Traefik file provider 动态目录 |
| `cron` | `crontab` 命令可用 |
| `logs` | `journalctl` 或 `docker` 至少一个可用 |
| `drift` | `nginx-proxy` / `traefik-proxy` / `cron` / `docker-api` 任一存在 |
| `ddns` | 恒上报（标准库出站 HTTP 探公网 IP） |
| `service` | Windows（SCM）/ macOS（launchd）/ systemd 之一 |
| `nas` | 软件 RAID（`/proc/mdstat`）或任一存储工具存在 |
| `oob` | 恒上报（带外兜底动作面，WOL 魔术包，`agent.go` `detectCapabilities`） |
| `virt-libvirt` | 本地 `virsh` 入口存在（`rpc.LibvirtAvailable()`），只读；真实驱动探测由 `virt.capabilities` RPC 验证 |

> 历史注记：旧 `network-monitor` capability（WireGuard peer 数与 cloudflared
> 进程级探测）已在 overlay M1 落地时删除（overlay-design.md D18），server/web
> 无消费方。

Agent 还会采集基础系统指标并通过心跳上报，包括 CPU、内存、磁盘、网络、系统版本、架构、负载和 uptime。

## Server Registry

Server 内存中的 Registry 只表示当前在线连接：

- 注册成功的 Agent 会进入 Registry。
- 重复 Agent ID 的活跃连接会被拒绝。
- RPC、代理、远程终端和桌面会话都依赖 Registry 找到在线 Agent。
- Server 重启后 Registry 清空，Agent 需要重连注册。

SQLite 中的 Agent 记录表示历史和资源关联，不等同于“当前在线连接”。

## 远程访问

远程访问不是 Browser 直连内网目标，而是：

```text
Browser
  -> Server WebSocket
  -> Agent WebSocket control channel
  -> target host:port
```

终端与桌面连接都使用 `proxy_*` 消息做 TCP 数据转发：终端直连目标，RDP/VNC/SSH 桌面经 Guacamole（`guac:` 前缀中继到 guacd，见 远控桌面 Guacamole 设计）。

远程目标默认必须命中 `remote_control.allowed_targets` 中显式配置的主机名、IP 或 CIDR；开发/调试环境可以设置 `remote_control.allow_arbitrary_target: true` 跳过该限制。当前实现不会自动从 inventory 推导可远控目标。

RDP/VNC/SSH 桌面协议由 guacd 终结，Agent 只做 TCP 转发——Agent 无特殊构建标签要求，任意平台默认构建均可用。

远程连接的 Browser WebSocket 使用短期 ticket 认证。详见 协议与 API 边界。

## 安全概念

| 边界 | 当前实现 |
| --- | --- |
| 用户认证 | JWT Bearer token |
| 管理员初始化 | `ADMIN_USERNAME` 可选，`ADMIN_PASSWORD` 必需 |
| 二次验证 | TOTP，密钥通过 `TOTP_ENCRYPTION_KEY` 加密 |
| Agent 认证 | 已配置 secret 的 Agent 必须在注册时提供匹配 secret |
| WebSocket Origin | `ALLOWED_ORIGINS` 控制；未设置时适合开发但不适合生产 |
| 远程连接 | 5 分钟、单次使用 ticket |

## 与历史设计的差异

仓库中有一些设计文档描述了更完整的 Git-first、目录拆分 inventory 和第三方集成愿景。当前实现已经具备 inventory 同步、Server/Agent 控制面、资源查询、监控、认证、审计、告警和远程连接，但仍应以本文和 架构与边界 中描述的代码事实为准。

## 术语表

每条先定义抽象职责，再指向拥有该职责的源码事实。对应分层小节见[架构与边界](/zh/guide/architecture#源码分层)。

| 术语 | 英文 | 定义 | file:line |
| --- | --- | --- | --- |
| 进程外壳 | Process shell | 解析 CLI 命令、选择配置，并为 Server/Agent/探针进程映射启动参数。 | `cmd/cockpit/main.go:22`, `cmd/cockpit-agent/main.go:75` |
| 运行时组合 | Runtime composition | 持有进程 context、组装依赖、统计后台任务、启停子系统并按依赖顺序关闭 DB。 | `internal/server/server.go:126`, `internal/server/server.go:204`, `internal/server/server.go:411` |
| Agent 会话注册表 | Agent session registry | 保存当前连接的 Agent 对象、能力快照、发送队列、心跳在线态和 pending RPC channel；不是持久 inventory。 | `internal/server/registry.go:11`, `internal/server/agent.go:15` |
| 控制协议 | Control protocol | 定义 register、heartbeat、RPC、proxy 数据、probe/liveness 上报和执行留痕的 WebSocket 消息类型与编解码。 | `internal/protocol/message.go:8`, `internal/protocol/codec.go:19` |
| Agent RPC 路由 | Agent RPC router | 解析 `<provider>.<action>`，选择 Provider，执行动作，按消息 ID 返回响应并记录真实执行。 | `internal/agent/rpc/router.go:70`, `internal/server/server.go:503` |
| 控制 API | Control API | 认证 REST 调用方，并把请求转换为存储操作、Agent RPC、流式 ticket 或会话管理。 | `internal/server/api.go:10`, `internal/server/server.go:283` |
| 远控会话 | Remote-control session | 跟踪由浏览器中转的终端/桌面活路径，并转换为 Agent 控制通道上的 `proxy_*` 帧。 | `internal/server/remote_session.go:43`, `internal/server/server.go:668` |
| 远控票据 | Remote-control ticket | 把 JWT 认证的 REST 请求桥接到浏览器 WebSocket，5 分钟有效且单次使用。 | `internal/server/ticket.go:26`, `internal/server/ticket.go:41` |
| 代理数据面 | Proxy data plane | 用 `proxy_new/proxy_data/proxy_close` 把监听器或浏览器 socket 桥接到 Agent 侧 TCP；按协议前缀路由伪流。 | `internal/proxy/mgr/manager.go:26`, `internal/server/server.go:668` |
| 作业与编排 | Jobs and orchestration | 先持久化 Job/WorkflowRun，再异步生成可审计的 Agent 尝试、重试步骤、解析输出，并冻结活动定义。 | `internal/server/api_jobs.go:63`, `internal/server/workflow_engine.go:18`, `internal/server/workflow_engine.go:216` |
| 定时调度 | Scheduled execution | 用巡检循环或周期 Runner 重放已存意图，不保留请求 context。 | `internal/server/server.go:293`, `internal/probe/runner.go:185`, `internal/server/backup_loop.go:101` |
| Inventory 声明源 | Inventory declaration source | 解析声明式 YAML，经 CLI 与热加载共用的 Syncer reconcile 到 SQLite。 | `internal/inventory/schema.go:1`, `internal/sync/watcher.go:57`, `internal/sync/watcher.go:129` |
| 业务持久态 | Business persistence | 拥有 GORM 模型、自动迁移，以及资源、用户、作业、Workflow、指标和审计行的 SQLite CRUD。 | `internal/storage/storage.go:40`, `internal/storage/storage.go:102` |
| 秘密保管 | Secret vault | 独立解锁并加密凭据条目，明文不进入普通 API 响应和审计日志。 | `internal/server/server.go:140`, `internal/vault/local.go:122`, `internal/vault/local.go:145` |
| Agent 能力探测 | Agent capability detection | 注册 Detector，发现节点/平台/集成特性，上报能力 metadata，不绑定凭据。 | `internal/agent/detector/detector.go:10`, `internal/agent/providers.go:18` |
| 执行 Provider | Execution provider | 在 Agent 内执行本地或本侧可达的动作类型：文件、作业、Docker、PVE、服务、日志、Stack、NAS、防火墙、overlay。 | `internal/agent/provider/provider.go:4`, `internal/agent/rpc/router.go:70`, `internal/agent/providers.go:33` |
| 基础运行时 | Agent core runtime | 提供注册、心跳、上报、重连、指标、平台和执行留痕等可复用原语。 | `core/register/register.go:1`, `core/heartbeat/heartbeat.go:1`, `core/report/upstream.go:1`, `core/platform/host.go:1` |
| 云集成 | Cloud integration | 用已存账号构造厂商适配器，声明独立能力，并记录异步操作以支持发现和生命周期。 | `internal/cloud/registry.go:12`, `internal/cloud/provider.go:13`, `internal/server/api_cloud.go:1` |
| DNS/证书集成 | DNS and certificate integration | 用 provider 专属凭据管理 DNS 记录、ACME 签发、续期和部署目标。 | `internal/dns/alidns.go:1`, `internal/server/acme_issuer.go:1`, `internal/server/acme_scan.go:1` |
| Overlay 集成 | Overlay integration | 读取或更新 ZeroTier/Tailscale 网络，并 reconcile 本地 overlay 身份。 | `internal/overlay/zerotier.go:1`, `internal/overlay/tailscale.go:1`, `internal/agent/providers.go:214` |
| 通知扇出 | Notification fanout | 应用事件白名单，构建启用渠道，并短超时同步或异步投递。 | `internal/notification/service.go:33`, `internal/notification/service.go:68`, `internal/server/server.go:297` |
| SPA 交付 | SPA delivery | 在拥有 `/api` 与 `/ws` 的同一 Server 进程交付带哈希静态资源，并回退到 `index.html`。 | `internal/server/web.go:12`, `internal/server/server.go:389` |
| Web 客户端状态 | Web client state | 拥有路由、权限菜单、JWT、展示偏好和浏览器会话 socket；不是系统事实源。 | `web/src/App.tsx:120`, `web/src/App.tsx:583`, `web/src/services/api.ts:122` |
| Mobile 客户端状态 | Mobile client state | 非敏感设置存偏好存储，auth token 存安全存储，仍使用同一 `/api` 运行面。 | `mobile/lib/state/settings.dart:10`, `mobile/lib/state/settings.dart:117`, `mobile/lib/api/client.dart:38` |
