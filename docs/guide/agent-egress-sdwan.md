# Agent 出口与 SD-WAN 能力边界

本文记录 Cockpit 当前 Agent 作为内网访问出口的实际能力，以及与完整 SD-WAN 的差距。结论以当前代码实现为准。

## 结论

当前 Agent 已具备“通过某个在线 Agent 访问其网络可达目标”的基础能力，但还不是 SD-WAN。它更接近应用层远控、端口代理和堡垒机通道。

如果目标是“选一个 Agent 作为出点，访问内网某台机器的 SSH/RDP/VNC”，现有架构可以支撑一部分。如果目标是“一个网段都通过某个 Agent 透明路由，像 SD-WAN 一样接入”，当前还不具备。

## 已具备能力

### Agent 连接内网 TCP 目标

Server 可以向 Agent 发送 `proxy_new` 消息，Agent 收到后连接目标 `host:port`，并通过 `proxy_data` 双向转发数据。只要 Agent 所在网络能访问目标地址，Server 就可以经由该 Agent 建立应用层通道。

关键实现：

- `internal/proxy/handler.go`：Agent 端通过 `net.DialTimeout("tcp", target, ...)` 连接目标。
- `internal/proxy/manager.go`：Server 端可监听本地端口，将客户端连接转发到指定 Agent。
- `internal/server/api_remote.go`：远程终端入口通过 ticket 创建到 Agent 的代理连接。

### 桌面远控（RDP/VNC/SSH，Guacamole）

RDP/VNC/SSH 桌面统一走 Guacamole 隧道：浏览器 `GuacamoleModal` 连 `/api/remote/guacamole`，Server 把 guacamole-common-js 指令转成 `guac:` 前缀的 `proxy_new/proxy_data` 发给 Agent，Agent 只做到 guacd 的 TCP 中继，协议由 guacd 终结（RDP 走 FreeRDP、VNC 走 libvncclient）。无特殊构建标签要求；自研 grdp 与 noVNC 直连路径已删除（D-2026-10-07-1）。

关键实现：

- `internal/server/api_guacamole.go`：ticket 校验、`guac:` 指令编解码与字节中继。
- `web/src/components/GuacamoleModal/index.tsx`：guacamole-common-js 渲染与输入。

### 固定端口代理

ProxyManager 支持在 Server 侧监听端口，并通过某个 Agent 转发到目标服务。当前实际监听地址固定为 `127.0.0.1:<remotePort>`，这是安全默认，避免无意公网暴露。

这适合本机工具或前置反向代理接入，不等于自动对公网发布内网服务。

## 当前缺口

### 不是完整 SD-WAN

当前没有实现以下能力：

- TUN/TAP 虚拟网卡
- L3 路由转发
- 子网路由表下发
- NAT、iptables 或 nftables 管理
- CIDR 级访问控制
- UDP 代理真实转发
- 多 Agent 路径选择、链路质量探测和故障切换

仓库中有 WireGuard、Cloudflare Tunnel 和路由信息探测，但这些只是能力检测，不是隧道建立或路由管理。

### 不支持完整的“网段以 Agent 为出口”策略模型

远控白名单现在已经支持主机名、IP 和 CIDR，并且 `remote_control.egress` 已支持按 `agent_id + allowed_targets + allowed_ports` 做最小出口限制。但它仍不会从 inventory 自动派生 allow-list，也不具备多 Agent 路由选择、优先级、故障切换和动态路径探测。

关键实现：

- `internal/server/remote_audit.go`
- `config/cockpit.yaml` 的 `remote_control.allowed_targets`

### Web SSH 还不是完整 SSH 客户端

当前 TerminalModal 是浏览器 xterm WebSocket，Server 和 Agent 侧主要做 TCP 字节转发。SSH 协议本身需要客户端完成握手、认证、加密和 PTY 分配，当前代码中没有完整 SSH client 实现。

因此需要区分两个目标：

- 本地 SSH 客户端经 Server 代理端口访问内网 SSH：架构可支持。
- 浏览器内直接 Web SSH 登录：需要补 Agent 端或 Server 端 SSH client。

### UDP 代理未真正落地

**已拒收（D-2026-10-08-2）**：API 显式拒绝 `proxyType=udp`（返回 400），仅接受 `tcp`。存储层仍保留字段但写入固定为 `tcp`。UDP 代理需单独实现连接建模、会话超时、包边界与返回路径，未纳入当前规划。

## 建议路线

### 第一阶段：Agent Egress Gateway

优先实现“某个 Agent 可作为某些主机或网段的远控出口”，不要一开始自行开发完整 SD-WAN。

建议配置模型：

```yaml
remote_control:
  egress:
    - agent_id: office-agent
      allowed_cidrs:
        - 192.168.10.0/24
      allowed_ports:
        - 22
        - 3389
        - 5900
```

当前已落地：

- `remote_control.allowed_targets` 支持 CIDR。
- `remote_control.egress` 支持按 Agent 约束目标和端口。
- ticket 创建时会校验 `agent_id + host + port` 是否匹配策略。

后续建议：

- 审计日志记录命中的出口 Agent、目标 IP、端口和协议。
- 明确多策略命中规则和优先级。
- 如果需要透明组网，再引入专用数据平面。

该阶段能满足大部分“通过 Agent 访问内网 SSH/RDP/VNC”的需求，同时保持实现简单。

### 第二阶段：补齐协议级远控

协议级远控已由 Guacamole 栈统一落地：RDP/VNC/SSH 桌面经 guacd 终结，浏览器不自行实现协议栈；Agent 不做协议终结、只做 TCP 中继，构建产物无特殊标签要求。

SSH 另有内置终端兜底路径：Agent 端用 `x/crypto/ssh`（`internal/agent/rpc/ssh_provider.go`）完成握手、认证和 PTY 转发，Server 只负责编排、审计和 WebSocket 中转。

剩余工作是 ACL、错误反馈和审计在两条路径上的统一，而不是再造协议客户端。

### 第三阶段：端口发布

如果需要本地工具直接访问，例如 `ssh user@server -p 2222` 进入 Agent 后面的机器，应在 ProxyManager 中显式建模监听地址和暴露策略。

建议新增字段：

- `bindAddress`：默认 `127.0.0.1`。
- `allowedSources`：来源 IP/CIDR allow-list。
- `publicBind`：只有管理员显式允许时才可监听 `0.0.0.0`。

不要把当前固定 `127.0.0.1` 直接改成 `0.0.0.0`，否则会扩大暴露面。

### 第四阶段：真 SD-WAN 集成

如果目标是真正透明三层组网，建议集成成熟方案，而不是在 Cockpit 内自行开发完整数据平面。

可选方向：

- WireGuard：Cockpit 负责配置分发、节点注册、状态监控和审计。
- Headscale/Tailscale：Cockpit 作为控制台集成节点、ACL 和服务入口。
- Cloudflare Tunnel：适合应用层服务发布，不适合完整三层网段互通。

Cockpit 更适合作为控制面和运维入口，数据平面交给成熟隧道或 VPN 实现。

## 能力矩阵

| 目标 | 当前状态 | 说明 |
| --- | --- | --- |
| Agent 访问其网络可达的单个 TCP 目标 | 基础具备 | 通过 `proxy_new` 和 `proxy_data` 转发 |
| VNC 浏览器直连 | 基本具备 | 经 Guacamole（guacd 终结 VNC），Agent 仅 `guac:` TCP 中继 |
| RDP 浏览器直连 | 基本具备 | 经 Guacamole（guacd 终结 RDP），无特殊构建标签 |
| 浏览器真正 SSH 登录 | 基本具备 | Guacamole SSH 桌面；Agent 内置 `x/crypto/ssh` 终端兜底 |
| 本地 SSH 客户端经 Server 端口转发 | 架构可支持 | 需要补监听策略和产品化入口 |
| 一个网段以某个 Agent 为出口 | 部分支持 | 已支持 CIDR allow-list 和 per-agent egress policy，但仍不是透明路由 |
| 真 SD-WAN / 透明三层组网 | 当前不支持 | 建议集成 WireGuard 或 Headscale/Tailscale |

## 设计原则

- KISS：先做 Agent Egress Gateway，解决远控和运维访问，不直接自行开发 L3 数据平面。
- YAGNI：只有明确需要透明组网时，再引入 WireGuard/Headscale 等数据平面。
- DRY：远控、VNC、端口代理应共享同一套目标校验和审计逻辑。
- SOLID：Server 负责策略、ticket、审计和编排；Agent 负责连接目标和协议执行；不要让 Agent 直接承担全局策略判断。
