# 架构与边界

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
| CLI | `cockpit init/sync/status/server` | 本地初始化、inventory 同步、状态查询、启动 Server | Agent 启动，Agent 已拆到 `cockpit-agent` |

## 数据边界

| 数据 | 位置 | 角色 | 生命周期 |
| --- | --- | --- | --- |
| 配置文件 | `config/cockpit.yaml` 或 `-config` 指定路径 | Server 启动配置，包括监听地址、数据库、JWT、邮件、通知、inventory | 启动读取；环境变量占位符会展开 |
| Inventory YAML | `inventory.path` 或 `cockpit sync -inventory` | 声明式资产清单 | 手动 `cockpit sync` 或 `inventory.watch` 热加载都会通过同一套 Syncer 写入数据库 |
| SQLite | `database.path` | 运行时查询和状态存储 | Server/CLI 打开时自动迁移 |
| Agent Registry | Server 内存 | 当前在线 Agent 连接池和待响应 RPC | 进程内状态，Server 重启后丢失 |
| Web UI 本地存储 | Browser `localStorage` | JWT、部分远程桌面连接偏好 | 浏览器侧状态，不作为系统真相源 |

当前的事实源不是单一的。Inventory 是声明式资产输入，SQLite 是 API 和 UI 的运行时查询面，Agent 心跳会更新在线状态和系统指标。需要区分“资产应当存在”和“节点当前在线/指标当前值”。

## 代码模块边界

| 模块 | 主要职责 |
| --- | --- |
| `internal/server` | HTTP 路由、Agent WebSocket、API 聚合、远程会话、代理、告警循环、指标清理 |
| `internal/agent` | Agent 生命周期、能力检测、心跳、消息循环、采集器、远程代理执行 |
| `internal/protocol` | Server 与 Agent 的 WebSocket 消息结构、消息类型、远程桌面子协议 |
| `internal/storage` | GORM/SQLite 模型、迁移、资源 CRUD、用户、审计、告警、指标、代理配置 |
| `internal/inventory` | Inventory YAML schema、解析、校验、同步到 storage |
| `internal/sync` | Server 运行期 inventory 文件监听；复用 `internal/inventory.Syncer` 将声明式资源同步到 storage |
| `internal/auth` | 登录、JWT、中间件、TOTP、密码重置 |
| `internal/proxy` | Server 侧监听代理端口、Agent 侧连接目标并转发数据 |
| `internal/pve` / `internal/docker` / `internal/openwrt` | 第三方平台客户端 |
| `web/src/services` | 前端 API 客户端；统一通过 `/api` 与 Server 通信 |

跨模块调用应尽量遵守这些边界。比如 API handler 不应直接解析 inventory 文件；应通过 storage 查询运行时结果。Agent 不应直接写 Server 数据库；应通过协议消息让 Server 持久化。

## 控制流

### Server 启动

1. `cmd/cockpit` 读取 `-config`，或依次尝试 `./config/cockpit.yaml`、`./cockpit.yaml`、`/etc/cockpit/config.yaml`。
2. Server 打开 SQLite 并自动迁移。
3. 初始化 JWT、邮件配置、通知客户端、审计、代理管理器、远程会话管理器和 ticket 管理器。
4. 必须通过 `ADMIN_PASSWORD` 初始化管理员密码；生产模式还会校验 `TOTP_ENCRYPTION_KEY`。
5. 注册 HTTP/API/WebSocket 路由并开始监听。

### Agent 注册与心跳

1. `cockpit-agent start -server ws://host:9000/ws` 连接 Server。
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

Server 的 `inventory.watch: true` 会监听单个 inventory 文件并热加载，当前实现复用 `inventory.Syncer`，与 `cockpit sync` 走同一套资源同步逻辑。

### HTTP API 与 Web UI

Web UI 默认请求 `/api`。公开接口包括登录、Token 刷新、TOTP 验证、密码重置和 `/health`。其他 `/api/` 资源接口通过 JWT 中间件保护。

主要 API 面：

- `/api/status`
- `/api/agents`
- `/api/resources/{compute-instances|domains|certificates|services|gateways|storages}`
- `/api/metrics/*`
- `/api/proxies`
- `/api/remote/*`
- `/api/admin/audit/*`

### 远程访问与代理

远程终端、VNC、RDP/桌面连接使用短期 ticket：

1. 浏览器先携带 JWT 请求 `POST /api/remote/tickets`。
2. Server 生成 5 分钟有效、单次使用 ticket。
3. 浏览器连接 `/api/remote/terminal`、`/api/remote/vnc` 或 `/api/remote/desktop`，把 ticket 放在 `Sec-WebSocket-Protocol`。
4. Server 校验 ticket 后，把连接请求转成 `proxy_*` 或 `desktop_*` 消息发给对应 Agent。
5. Agent 连接目标服务并把数据通过 Server 转发回浏览器。

这个边界保证浏览器不需要知道内网拓扑，也不需要直接连 Agent。

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
| Agent -> Server | `register`, `heartbeat`, `rpc_response`, `proxy_close`, `proxy_error` |
| Server -> Agent | `rpc_request`, `ping`, `proxy_new` |
| 双向 | `error`, `proxy_data`, `desktop_data`, `desktop_close` |
| Server -> Agent | `desktop_new` |

`register` 必须是 Agent 建立 `/ws` 后的第一条消息。RPC 使用请求消息 ID 关联响应。代理和远程桌面数据使用 `proxyId`、`connId` 或 `sessionId` 关联具体连接。

## 安全边界

- Server 管理员密码必须通过 `ADMIN_PASSWORD` 提供，长度至少 8 位。
- 生产环境应设置 `PRODUCTION=true` 和强随机 `TOTP_ENCRYPTION_KEY`。
- `ALLOWED_ORIGINS` 控制 WebSocket Origin；未设置时会接受所有来源，仅适合开发环境。
- Web UI 静态目录由 `STATIC_DIR` 或 `server.static_dir` 指定；未设置时根路径只返回 API 运行提示。
- Agent secret 是可选但推荐的 Agent 认证边界；已配置 secret 的 Agent 重连必须提供匹配密钥。
- 远程连接 ticket 是内存态、5 分钟有效、单次使用；Server 重启后失效。

## 当前实现边界

- Inventory 的目录拆分模型尚未成为主要实现路径；当前 CLI 示例和 parser 以单文件 `version: v1` YAML 为准。
- `internal/agent/rpc` 的 SystemProvider 始终注册；Docker/PVE/OpenWrt Provider 由 `Agent.setupProviders` 按检测到的 capability 和环境变量（如 `DOCKER_HOST`、`PVE_TOKEN_ID`、`OPENWRT_HOST` 等）条件注册，凭据缺失时仅记录日志跳过，不影响基础心跳。
- `/api/remote/sessions` 目前是内存会话登记，不是完整连接配置持久化。
- Web UI 中部分偏好配置保存在浏览器本地，不进入 Server 数据库。
- docs 中 `docs/superpowers` 是历史设计和计划记录，不代表当前运行架构。
