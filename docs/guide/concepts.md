# 核心概念

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

当前 capability 共 17 个，分两类来源。

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

终端与桌面连接都使用 `proxy_*` 消息做 TCP 数据转发：终端直连目标，RDP/VNC/SSH 桌面经 Guacamole（`guac:` 前缀中继到 guacd，见 [远控桌面 Guacamole 设计](/remote-desktop-guacamole-design)）。

远程目标默认必须命中 `remote_control.allowed_targets` 中显式配置的主机名、IP 或 CIDR；开发/调试环境可以设置 `remote_control.allow_arbitrary_target: true` 跳过该限制。当前实现不会自动从 inventory 推导可远控目标。

RDP/VNC/SSH 桌面协议由 guacd 终结，Agent 只做 TCP 转发——Agent 无特殊构建标签要求，任意平台默认构建均可用。

远程连接的 Browser WebSocket 使用短期 ticket 认证。详见 [协议与 API 边界](/guide/protocol)。

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

仓库中有一些设计文档描述了更完整的 Git-first、目录拆分 inventory 和第三方集成愿景。当前实现已经具备 inventory 同步、Server/Agent 控制面、资源查询、监控、认证、审计、告警和远程连接，但仍应以本文和 [架构与边界](/guide/architecture) 中描述的代码事实为准。
