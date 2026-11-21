# 核心概念与源码术语表

Cockpit 的概念分布在四个页面：本页承载资源模型与源码术语表；运行时状态、Inventory 声明源和安全边界各自成页。

- [运行时状态](/zh/guide/runtime) — 数据面、Agent 能力模型、Server Registry 与远程访问路径。
- [Inventory 声明源](/zh/guide/inventory) — Inventory YAML schema、同步语义，以及与历史设计的差异。
- [安全](/zh/guide/security) — 认证、秘密保管与远程连接边界。

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
| Web 客户端状态 | Web client state | 拥有路由、权限菜单、JWT、展示偏好和浏览器会话 socket；不是系统事实源。 | `web/src/App.tsx:120`, `web/src/App.tsx:583`, `web/src/services/api.ts:35` |
| Mobile 客户端状态 | Mobile client state | 非敏感设置存偏好存储，auth token 存安全存储，仍使用同一 `/api` 运行面。 | `mobile/lib/state/settings.dart:10`, `mobile/lib/state/settings.dart:117`, `mobile/lib/api/client.dart:38` |
