# 运行时状态

Cockpit 的运行时状态是 API 与 Web UI 查询的 SQLite 视图，加上 Server 上注册的在线 Agent 连接。向 SQLite 提供数据的声明面见 [Inventory 声明源](/zh/guide/inventory)；安全边界见 [安全](/zh/guide/security)。

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

终端与桌面连接都使用 `proxy_*` 消息做 TCP 数据转发：终端直连目标，RDP/VNC/SSH 桌面经 Guacamole（`guac:` 前缀中继到 guacd，见[远控桌面 Guacamole 设计](/zh/remote-desktop-guacamole-design)）。

远程目标默认必须命中 `remote_control.allowed_targets` 中显式配置的主机名、IP 或 CIDR；开发/调试环境可以设置 `remote_control.allow_arbitrary_target: true` 跳过该限制。当前实现不会自动从 inventory 推导可远控目标。

RDP/VNC/SSH 桌面协议由 guacd 终结，Agent 只做 TCP 转发——Agent 无特殊构建标签要求，任意平台默认构建均可用。

远程连接的 Browser WebSocket 使用短期 ticket 认证。详见[协议与 API 边界](/zh/guide/protocol)。
