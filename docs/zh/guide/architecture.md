# 架构与源码分层


本文以当前仓库实现为准，说明 Cockpit 的运行拓扑、模块职责和边界。它不是长期愿景文档；当代码行为变化时，应优先同步这里。

## 总览

Cockpit 当前由三个运行面组成：

```text
Browser Web UI
    |
    | HTTP JSON API / WebSocket
    v
Cockpit Server (cmd/cockpit)
    |  \
    |   \ SQLite runtime store
    |
    | WebSocket /ws
    v
Cockpit Agent (cmd/cockpit-agent)
    |
    | local API / socket / TCP
    v
Managed targets (host, Docker, PVE, OpenWrt, SSH/RDP/VNC service)
```

Server 是中心控制面，Agent 是节点侧执行面。Agent 主动连接 Server，因此 NAT 后节点不需要暴露入站端口。Web UI 只与 Server 通信，不直接访问 Agent 或内网目标。

## 进程边界

| 进程 | 入口 | 职责 | 不负责 |
| --- | --- | --- | --- |
| Cockpit Server | `cmd/cockpit` | HTTP API、Web UI 静态资源、Agent WebSocket 注册与路由、SQLite 持久化、认证、审计、告警、反向代理与远程会话编排 | 直接访问 NAT 后内网服务、直接采集 Agent 本机指标 |
| Cockpit Agent | `cmd/cockpit-agent` | 节点能力检测、心跳和系统指标上报、RPC/代理/远程桌面执行、连接本机或本网络可达目标 | 用户认证、全局状态持久化、跨 Agent 调度决策 |
| Web UI | `web/` | 登录、资源视图、工作台、监控、设置、远程连接交互 | 保存最终运行状态、绕过 Server 访问 Agent |
| CLI | `cockpit init/sync/status/server/agent` | 本地初始化、inventory 同步、状态查询、启动 Server；`cockpit agent` 兼容 `cockpit-agent start` 的启动参数 | 长期运行时的 Agent 职责仍在 Agent 进程内 |

## 数据边界

| 数据 | 位置 | 角色 | 生命周期 |
| --- | --- | --- | --- |
| 配置文件 | `config/cockpit.yaml` 或 `-config` 指定路径 | Server 启动配置，包括监听地址、数据库、JWT、邮件、通知、inventory | 启动读取；环境变量占位符会展开 |
| Inventory YAML | `inventory.path` 或 `cockpit sync -inventory` | 声明式资产清单 | 手动 `cockpit sync` 或 `inventory.watch` 热加载都会通过同一套 Syncer 写入数据库 |
| SQLite | `database.path` | 运行时查询和状态存储 | Server/CLI 打开时自动迁移 |
| Agent Registry | Server 内存 | 当前在线 Agent 连接池和待响应 RPC | 进程内状态，Server 重启后丢失 |
| Web UI 本地存储 | Browser `localStorage` | JWT、远控偏好、主题/站点名称/刷新间隔等 UI 本地偏好 | 浏览器侧状态，不作为系统真相源 |

当前的事实源不是单一的。Inventory 是声明式资产输入，SQLite 是 API 和 UI 的运行时查询面，Agent 心跳会更新在线状态和系统指标。需要区分“资产应当存在”和“节点当前在线/指标当前值”。

## 代码模块边界

| 模块 | 主要职责 |
| --- | --- |
| `internal/server` | HTTP 路由、Agent WebSocket、API 聚合、远程会话、代理、告警循环、指标清理 |
| `internal/agent` | Agent 生命周期、能力检测、心跳、消息循环、采集器、远程代理执行 |
| `internal/protocol` | Server 与 Agent 的 WebSocket 消息结构、消息类型与解码 helper |
| `internal/storage` | GORM/SQLite 模型、迁移、资源 CRUD、用户、审计、告警、指标、代理配置 |
| `internal/inventory` | Inventory YAML schema、解析、校验、同步到 storage |
| `internal/sync` | Server 运行期 inventory 文件监听；复用 `internal/inventory.Syncer` 将声明式资源同步到 storage |
| `internal/auth` | 登录、JWT、中间件、TOTP、密码重置 |
| `internal/proxy` | Server 侧监听代理端口、Agent 侧连接目标并转发数据 |
| `internal/pve` / `internal/docker` / `internal/openwrt` | 第三方平台客户端 |
| `web/src/services` | 前端 API 客户端；统一通过 `/api` 与 Server 通信 |

跨模块调用应尽量遵守这些边界。比如 API handler 不应直接解析 inventory 文件；应通过 storage 查询运行时结果。Agent 不应直接写 Server 数据库；应通过协议消息让 Server 持久化。

### 源码分层

这些层是职责边界，不是目录名。源码证据表见[术语表](/zh/guide/concepts#glossary)。

#### L0 外壳与进程引导

CLI 解析命令、探测默认配置路径、把 flags 映射为类型化启动配置，并启动 Server、Agent 或探针进程；不拥有运行期服务。`cmd/cockpit/main.go:22` 定义探测顺序，`cmd/cockpit/main.go:61` 分发命令，`cmd/cockpit-agent/main.go:75` 映射 `start` 参数，`cmd/cockpit-probe-agent/main.go:26` 启动独立探针进程。

#### L1 传输与契约

`/ws` 升级 Agent 连接，要求首帧 `register`，随后运行读写循环。`internal/server/websocket.go:70` 负责握手；`internal/protocol/message.go:8` 定义消息类型；`internal/protocol/codec.go:19` 负责 WebSocket JSON 编解码。`internal/agent/agent.go:355` 按 HTTP/1.1 和连接代数拨号，`internal/agent/agent.go:677` 分发下行控制帧。

#### L2 Server 组合与运行期控制

Server 持有进程级 context、后台任务计数、中间件顺序、子系统启停和 Agent RPC 关联。`internal/server/server.go:126` 打开 SQLite 并组装依赖；`internal/server/server.go:204` 启动子系统与监听；`internal/server/server.go:411` 按依赖顺序关停；`internal/server/server.go:503` 用 pending channel 关联 RPC 响应。

#### L3 控制 API 与会话策略

REST handler 把已认证请求转换成资源操作、Agent RPC 或流式会话控制。中央路由在 `internal/server/api.go:16`；公开与受保护路由在 `internal/server/server.go:354` 注册；JWT/RBAC/审计包装顺序在 `internal/server/server.go:283`；Web 菜单与路由声明在 `web/src/App.tsx:120`，渲染在 `web/src/App.tsx:533`。

#### L4 编排与调度

Jobs 和 Workflows 先持久化意图与步骤状态再派发；调度器和巡检按时间重放意图。Job 提交在 `internal/server/api_jobs.go:63`，派发在 `internal/server/api_jobs.go:401`；Workflow 创建在 `internal/server/api_workflow.go:484`，步骤状态机在 `internal/server/api_workflow.go:582`；探测循环在 `internal/probe/runner.go:185`，inventory 热加载在 `internal/sync/watcher.go:57`。

#### L5 持久化与秘密保管

SQLite 是业务持久态平面，Vault 是独立解锁的加密秘密平面。`internal/storage/storage.go:40` 打开并迁移 SQLite，`internal/storage/storage.go:102` 列出模型集合；Vault provider 在 `internal/server/server.go:140` 选择，本地加密提供者经 `internal/vault/local.go:122` 读写行。

#### L6 执行 Provider 与平台探测

Agent 把 `<provider>.<action>` RPC 映射到能力 Provider，在本地或本侧可达系统执行。Provider 契约在 `internal/agent/provider/provider.go:4`，方法路由在 `internal/agent/rpc/router.go:70`，探测契约在 `internal/agent/detector/detector.go:10`，按能力装配在 `internal/agent/providers.go:18`。

#### L7 集成与通知出口

Server 侧适配器把已存账号或 YAML 凭据转换成云、DNS、overlay 和通知调用。云 Registry 在 `internal/cloud/registry.go:23` 校验声明能力，契约在 `internal/cloud/provider.go:13`；通知渠道在 `internal/notification/service.go:68` 构建；DNS 与 overlay 适配器在 `internal/server/server.go:255` 装配。

#### L8 接口交付与客户端状态

Server 在同一进程交付 SPA 与 API；Web/mobile 只保留展示与会话本地状态。静态回退在 `internal/server/web.go:12`；Web API 网关在 `web/src/services/api.ts:122`；mobile 偏好与安全 token 边界在 `mobile/lib/state/settings.dart:10`；终端数据路径在 `web/src/components/TerminalModal/index.tsx:115` 重连。

#### 状态与通道矩阵

| 层 | 运行态 | 入口调用方 | 通道 |
| --- | --- | --- | --- |
| L0 | 进程环境与 CLI flags | 操作者 | CLI flags、YAML loader |
| L1 | Agent 连接对象、发送队列、pending RPC map | Agents | WebSocket `/ws`、JSON 帧 |
| L2 | context、WaitGroup、中间件链 | Server 内模块 | 进程内调用 |
| L3 | ticket/session 内存态；JWT 存浏览器/mobile 安全存储 | Web、mobile、脚本 | REST `/api`、浏览器 WebSocket `/api/remote/terminal` |
| L4 | Job/Workflow/ProbeResult 在 SQLite；timer 在内存 | REST、后台循环 | SQLite、Agent RPC、探测 HTTP/ICMP/TCP |
| L5 | 业务模型与加密 Vault 行 | L2-L7 | GORM/SQLite、加密密钥 |
| L6 | Agent Provider 会话与任务日志缓冲 | Server RPC | 本地进程/文件/socket、proxy 帧 |
| L7 | 云发现 TTL 缓存在内存；凭据来自配置/环境 | REST 与巡检 | HTTPS provider、email/webhook/Telegram |
| L8 | 浏览器/mobile UI 状态 | 用户 | 静态 HTTP、REST、会话 WebSocket |

### 拆分理由

1. 引导与执行分离，使多个命令目标可以共享同一配置探测路径，而不会把服务生命周期嵌进参数解析（`cmd/cockpit/main.go:22`）。
2. WS 契约独立于 handler，因为注册、上行上报和代理转发共用一个连接代数和串行写出器（`internal/agent/agent.go:355`；写出循环由 `internal/core/report/upstream.go` 提供）。
3. Server 组合独立于 REST，因为后台循环和 DB 关停需要一个长于请求生命周期的进程 context（`internal/server/server.go:203`；`internal/server/server.go:411`）。
4. 控制 API 独立于编排，因为请求 handler 先创建持久 Job 并返回，Agent 执行随后异步完成（`internal/server/api_jobs.go:63`；`internal/server/api_jobs.go:401`）。
5. Workflow 状态独立于 Job 状态，因为在途定义必须冻结，而步骤要生成独立可审计的 Job 尝试（`internal/server/api_workflow.go:426`；`internal/server/api_workflow.go:682`）。
6. 持久化与 Vault 分离，因为普通查询行和解锁门控的加密秘密有不同的生命周期与访问规则（`internal/storage/storage.go:40`；`internal/vault/local.go:122`）。
7. Agent 执行做成 Provider 可插拔，因为能力探测要跳过不可用集成，同时不破坏始终运行的心跳（`internal/agent/providers.go:18`；`internal/agent/agent.go:424`）。
8. Server 集成独立于 Agent Provider，因为云/DNS/overlay 用全局凭据控制资源，而节点执行由本地能力和环境门控（`internal/server/server.go:255`；`internal/agent/providers.go:18`）。
9. 接口交付独立于事实源状态，因为客户端可以本地缓存 token 与偏好，但 `/api` 仍是运行时权威路径（`web/src/services/api.ts:126`；`mobile/lib/state/settings.dart:10`）。

## 控制流

### Server 启动

1. `cmd/cockpit` 读取 `-config`，或依次尝试 `./config/cockpit.yaml`、`./cockpit.yaml`、`/etc/cockpit/config.yaml`。
2. Server 打开 SQLite 并自动迁移。
3. 初始化 JWT、邮件配置、通知客户端、审计、代理管理器、远程会话管理器和 ticket 管理器。
4. 必须通过 `ADMIN_PASSWORD` 初始化管理员密码；生产模式还会校验 `TOTP_ENCRYPTION_KEY`。
5. 注册 HTTP/API/WebSocket 路由并开始监听。

### Agent 注册与心跳

1. `cockpit-agent start -server ws://host:9000/ws` 连接 Server；也可使用兼容入口 `cockpit agent -server ws://host:9000/ws`。
2. Agent 运行能力检测器，形成 capability 列表。
3. Agent 首包必须是 `register`。
4. Server 校验重复连接和已配置的 Agent secret，接受后写入 SQLite 并加入内存 Registry。
5. Agent 每 30 秒发送 `heartbeat`，其中可包含系统信息；Server 写入历史指标和快照。

### Inventory 同步

`cockpit sync` 是当前完整同步路径。它解析 `version: v1` 的 YAML，并同步以下资源：

- Regions / Zones / Agents
- Domains / Certificates
- ComputeInstances
- Services
- Gateways
- Storages

Server 的 `inventory.watch: true` 会监听单个 inventory 文件并热加载，当前实现复用 `inventory.Syncer`，与 `cockpit sync` 走同一套资源同步逻辑。`inventory.strict: true` 会把 inventory 路径缺失、解析失败或初始同步失败视为启动错误；默认 `false` 时只记录日志并继续启动 Server。

### HTTP API 与 Web UI

Web UI 默认请求 `/api`。登录、Token 刷新、TOTP 验证、密码重置和 `/health` 是公开接口；`POST /api/stacks/webhook` 不走 JWT，而以 `X-Cockpit-Token` 验证。其他 `/api/` 资源接口通过 JWT 中间件保护。

主要 API 面：

- `/api/status`
- `/api/me`、`/api/me/profile`、`/api/me/password`
- `/api/settings`
- `/api/agents`、`/api/agents/{id}/secret`
- `/api/resources/{compute-instances|domains|certificates|services|gateways|storages}`
- `/api/users`（管理员管理用户）
- `/api/alerts`（告警列表与已读标记）
- `/api/metrics/*`
- `/api/proxies`
- `/api/remote/*`（tickets / sessions / terminal / guacamole / vault）
- `/api/admin/audit/*`
- `/api/auth/totp/*`（generate / enable / disable 需 JWT；verify 公开）

### 远程访问与代理

远程终端与桌面连接（Guacamole 隧道）使用短期 ticket：

1. 浏览器先携带 JWT 请求 `POST /api/remote/tickets`。
2. Server 生成 5 分钟有效、单次使用 ticket。
3. 浏览器建立 WebSocket：terminal 把 ticket 放在 `Sec-WebSocket-Protocol`；Guacamole 优先放在 URL 的 `?ticket=`，缺少 query 时兼容从 `Sec-WebSocket-Protocol` 读取。
4. Server 校验 ticket 后，把连接请求转成 `proxy_*` 消息发给对应 Agent（终端直连目标；桌面经 `guac:` 中继到 guacd）。
5. Agent 连接目标服务并把数据通过 Server 转发回浏览器。

这个边界保证浏览器不需要知道内网拓扑，也不需要直接连 Agent。
远控目标默认只允许 `remote_control.allowed_targets` 中显式配置的主机名、IP 或 CIDR。

## 协议边界

Server 与 Agent 的消息统一为：

```json
{
  "id": "message-id",
  "type": "heartbeat",
  "timestamp": 1710000000,
  "payload": {}
}
```

当前消息类型：

| 方向 | 类型 |
| --- | --- |
| Agent -> Server | `register`, `rpc_response`, `proxy_close`, `proxy_error`, `probe_report`, `exec_log` |
| Server -> Agent | `rpc_request`, `ping`, `proxy_new` |
| 双向 | `heartbeat`（Agent 上报、Server 回 ACK）、`error`, `proxy_data` |

`register` 必须是 Agent 建立 `/ws` 后的第一条消息。RPC 使用请求消息 ID 关联响应。代理数据使用 `proxyId`、`connId` 关联具体连接。

## 安全边界

- Server 管理员密码必须通过 `ADMIN_PASSWORD` 提供，长度至少 8 位。
- 生产环境应设置 `PRODUCTION=true` 和强随机 `TOTP_ENCRYPTION_KEY`。
- `ALLOWED_ORIGINS` 控制 WebSocket Origin；未设置时会接受所有来源，仅适合开发环境。
- Web UI 静态目录由 `STATIC_DIR` 或 `server.static_dir` 指定；未设置时根路径只返回 API 运行提示。
- Agent secret 是 Agent 认证边界。2026-10-18 起注册默认 fail-closed：未知 Agent 仅在携带 `server.agent_registration.enrollment_secret`（或开发环境显式 `allow_unknown`）时可注册，注册成功后其携带的 secret 即固化为该 Agent 的认证密钥；无 secret 的存量 Agent 重连默认拒绝，需显式 `allow_secretless`。已配置 secret 的 Agent 重连必须提供匹配密钥。
- 远程连接 ticket 是内存态、5 分钟有效、单次使用；Server 重启后失效。

## 当前实现边界

- Inventory 的目录拆分模型尚未成为主要实现路径；当前 CLI 示例和 parser 以单文件 `version: v1` YAML 为准。
- `internal/agent/rpc` 的 SystemProvider 始终注册；Docker/PVE/OpenWrt Provider 由 `Agent.setupProviders` 按检测到的 capability 和环境变量（如 `DOCKER_HOST`、`PVE_TOKEN_ID`、`OPENWRT_HOST` 等）条件注册，凭据缺失时仅记录日志跳过，不影响基础心跳。
- `/api/remote/sessions` 是内存会话台账（pending/connected/closed/failed），已接线到 terminal / guacamole 活会话生命周期（D-2026-10-12-4）；不是完整连接配置持久化，重启后台账清空。
- Web UI 中部分偏好配置保存在浏览器本地，不进入 Server 数据库；`/api/settings` 当前仅作为兼容入口，前端本地偏好以浏览器存储值为准。
- `docs/archive`（原 `docs/superpowers`）保存 TOTP、通知模块等历史设计稿和实施计划，仅供追溯，不代表当前运行架构；详见 [`docs/archive/README.md`](/zh/archive/README)。
