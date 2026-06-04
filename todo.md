# Cockpit 架构收口与推进 Todo

本文档用于交给后续模型或开发者逐项执行。优先目标不是继续堆新功能，而是让 Cockpit 的核心定位闭环：Git-first CMDB + Agent 主动连接 + 运行态监控 + Web 控制台。

## 执行原则

- 每个任务单独提交，避免把架构调整、功能补齐、测试修复混在一个提交里。
- 不做大爆炸重构。先补闭环和边界，再拆包。
- 保持现有技术栈：Go 后端、SQLite/GORM、Gorilla WebSocket、React/Vite/Ant Design。
- 对外 API 和前端页面优先保持兼容，除非当前接口明显不存在或错误。
- 每完成一个任务至少运行对应包测试；跨模块任务运行 `go test ./...` 和 `pnpm run build`。
- 不回滚用户已有改动，不删除未确认的功能模块。

## 当前判断

当前架构方向合理，但实现重心偏移：

- 合理部分：`Server + Agent 主动 WebSocket + SQLite 运行态 + Web UI` 适合个人混合基础设施控制台。
- 最大缺口：README/文档强调 Git-first CMDB，但 `inventory -> SQLite -> API -> Web UI` 只覆盖 Agent、Domain、Certificate，Compute/Service/Gateway/Storage 没有闭环。
- 最大维护风险：`internal/server` 和 `internal/agent/rpc` 已经变成大聚合模块，职责开始混杂。
- 最大产品不一致：文档写了 `cockpit init/sync/status`，但 CLI 未实现；`cockpit agent` 是占位，真实 Agent 在 `cmd/cockpit-agent`。

## Phase 0: 建立可验证基线

### 0.1 记录并修复依赖下载问题

涉及文件：

- `go.mod`
- `go.sum`
- `.github/workflows/*.yml`

执行步骤：

- 运行 `go env GOPROXY`，确认本地依赖下载失败是否只由 `goproxy.cn` 引起。
- 运行 `GOPROXY=https://proxy.golang.org,direct go mod download`。
- 如果 `github.com/nakagami/grdp v0.8.6` 仍不可下载，确认是否需要替换 RDP 依赖或临时构建标签隔离 RDP。
- 不要随意升级大版本依赖；先以恢复测试为目标。

验收标准：

- `go mod download` 成功。
- `go test ./internal/protocol/... ./internal/storage/... ./internal/inventory/...` 成功。
- 若 `go test ./...` 仍失败，失败原因必须记录在本文件对应任务下。

### 0.2 建立基础验证命令

执行步骤：

- 后端：`go test ./...`
- 前端：`cd web && pnpm install && pnpm run build`
- 静态检查：`go vet ./...`

验收标准：

- 每个后续 Phase 完成时都更新本节测试结果。
- 如果某个测试因为环境依赖不能跑，记录具体依赖和替代验证方式。

## Phase 1: CLI 与文档收口

### 1.1 实现或修正文档里的 CLI 命令

当前问题：

- README 和 docs 写了 `./cockpit init`、`./cockpit sync`、`./cockpit status`。
- `cmd/cockpit/main.go` 只实现 `server`、`agent`、`version`。

涉及文件：

- `cmd/cockpit/main.go`
- `README.md`
- `docs/guide/getting-started.md`
- `internal/config/config.go`
- `internal/inventory/*`
- `internal/storage/*`

建议执行路径：

- 实现 `cockpit init`：创建默认配置、数据目录、inventory 示例文件。
- 实现 `cockpit sync -inventory examples/inventory.yaml`：解析 inventory 并同步到 SQLite。
- 实现 `cockpit status`：读取 SQLite 输出 agent/resource 汇总。
- 如果短期不实现某个命令，必须从 README/docs 删除对应命令，避免使用者按文档失败。

验收标准：

- `go build -o /tmp/cockpit ./cmd/cockpit` 成功。
- `/tmp/cockpit version` 成功。
- `/tmp/cockpit init -dir /tmp/cockpit-test` 能生成配置和 inventory。
- `/tmp/cockpit sync -config /tmp/cockpit-test/config.yaml` 能写入 SQLite。
- `/tmp/cockpit status -config /tmp/cockpit-test/config.yaml` 能输出资源摘要。

### 1.2 收口 Agent 启动入口

当前问题：

- `cockpit agent` 是占位提示。
- `cockpit-agent start` 才是真实实现。

涉及文件：

- `cmd/cockpit/main.go`
- `cmd/cockpit-agent/main.go`
- `internal/agent/agent.go`

建议执行路径：

- 方案 A：保留两个二进制，让 `cockpit agent` 调用与 `cockpit-agent start` 相同逻辑。
- 方案 B：移除 `cockpit agent` 子命令，只保留 `cockpit-agent`，同步更新 README/docs。
- 推荐方案 A，因为用户只记一个主命令更方便，发布两个二进制也不冲突。

验收标准：

- `go build ./cmd/cockpit ./cmd/cockpit-agent` 成功。
- `cockpit agent -server ws://127.0.0.1:9000/ws` 不再输出“正在开发中”。
- `cockpit-agent start -server ws://127.0.0.1:9000/ws` 行为保持兼容。

## Phase 2: Git-first Inventory 闭环

### 2.1 确定 inventory v1 规范

当前问题：

- `examples/inventory.yaml` 使用当前聚合式 `version: v1` 格式。
- `docs/guide/getting-started.md` 使用类似 Kubernetes manifest 的 `apiVersion/kind/metadata/spec` 格式。
- `internal/inventory/schema.go` 定义了 Compute/Service/Gateway/Storage，但 `Inventory` 顶层没有对应 map，导致无法同步。

涉及文件：

- `internal/inventory/schema.go`
- `examples/inventory.yaml`
- `docs/guide/getting-started.md`
- `docs/guide/concepts.md`

建议执行路径：

- 短期采用现有 `version: v1` 聚合式格式，先让代码闭环。
- 后续如需 manifest 目录格式，再单独做 v2 或 multi-file loader。
- 在 `Inventory` 顶层增加：
  - `ComputeInstances map[string]*ComputeInstance`
  - `Services map[string]*Service`
  - `Gateways map[string]*Gateway`
  - `Storages map[string]*Storage`
- 给 Compute/Service/Gateway/Storage 增加必要的 `Region`、`Zone` 字段，避免只能通过嵌套推断。
- 更新 examples 和 docs，保证文档示例能被当前 parser 解析。

验收标准：

- `inventory.ParseFile("examples/inventory.yaml")` 成功。
- 文档中的最小 inventory 示例可以直接复制后被解析。
- `go test ./internal/inventory/...` 成功。

### 2.2 补齐 inventory 同步到 SQLite

当前问题：

- `internal/inventory/sync.go` 只同步 Agent、Domain、Certificate。
- Storage 层已有 `UpsertComputeInstance`、`UpsertService`、`UpsertGateway`、`UpsertStorage` 等能力。

涉及文件：

- `internal/inventory/sync.go`
- `internal/inventory/sync_test.go`
- `internal/storage/storage.go`
- `internal/storage/models.go`

执行步骤：

- 新增 `syncComputeInstances`。
- 新增 `syncServices`。
- 新增 `syncGateways`。
- 新增 `syncStorages`。
- `SyncResult` 增加对应结果字段。
- 修正现有 Created/Updated 统计逻辑。当前 `Upsert` 后再读 `FirstSeen` 判断不可靠。
- 对 agent 引用做校验：资源引用不存在的 agent 时返回明确错误或记录 result.Errors。
- 对 region/zone 做基本校验：为空时允许但标记 unknown，或直接校验失败，二选一并写进文档。

验收标准：

- 同步 examples 后，`/api/resources/compute-instances`、`services`、`gateways`、`storages` 都能返回数据。
- `go test ./internal/inventory/... ./internal/storage/...` 成功。
- 新增测试覆盖 create/update、缺失 agent、空 inventory、重复 ID。

### 2.3 增加 inventory watch/sync 启动路径

当前代码里已有 `internal/sync/watcher.go`，但 Server 启动未明显接入 inventory 同步主线。

涉及文件：

- `internal/sync/watcher.go`
- `internal/server/server.go`
- `internal/config/config.go`
- `config/cockpit.yaml`

执行步骤：

- 在配置中增加 `inventory.path` 和 `inventory.watch`。
- Server 启动时如配置启用 watch，则启动 watcher。
- CLI `sync` 使用同一套 Syncer，避免两套同步逻辑。
- watcher 出错只记录，不应导致 Server 整体退出，除非配置要求 strict。

验收标准：

- 修改 inventory 文件后 SQLite 会更新。
- watcher 停止时能随 Server Shutdown 退出。
- `go test ./internal/sync/...` 成功。

## Phase 3: Server 模块边界收口

### 3.1 拆出 HTTP 路由注册

当前问题：

- `Server.Start` 同时做认证初始化、管理员初始化、路由注册、后台 goroutine 启动、HTTP listen。

涉及文件：

- `internal/server/server.go`
- `internal/server/api.go`
- `internal/server/api_*`

执行步骤：

- 新增 `func (s *Server) routes() http.Handler` 或 `registerRoutes(mux *http.ServeMux)`。
- 将 auth routes、resource routes、remote routes、metrics routes、static routes 分组。
- 保持 URL 不变。
- 保持测试里的 handler 调用尽量兼容。

验收标准：

- `Server.Start` 主要只负责初始化服务、启动后台任务、启动 HTTP server。
- `go test ./internal/server/...` 成功。

### 3.2 拆出 Agent Gateway / Agent Hub

当前问题：

- `Server` 直接处理 agent 注册、认证、readLoop/writeLoop、消息分发。

涉及文件：

- `internal/server/server.go`
- `internal/server/agent.go`
- `internal/server/registry.go`
- `internal/protocol/*`

建议目标结构：

- `AgentHub`：负责 registry、注册、注销、SendToAgent。
- `AgentAuthenticator`：负责 agent secret 验证。
- `MessageDispatcher`：负责 heartbeat/rpc/proxy/desktop/vnc 分发。

执行步骤：

- 先抽接口，不迁移行为。
- 保持 `/ws` 入口不变。
- 把 `handleWebSocket` 中的认证和注册拆成小函数。
- 把 `handleMessage` 中的 switch 移到 dispatcher 或至少独立文件。

验收标准：

- WebSocket 注册、心跳、RPC response、Proxy、Desktop、VNC 测试仍通过。
- `server.go` 行数明显下降，目标小于 500 行。

### 3.3 修复 Server 中已发现的行为问题

涉及文件：

- `internal/server/server.go`
- `internal/server/api.go`
- `internal/server/api_remote.go`
- `internal/server/api_desktop.go`
- `internal/server/api_proxy.go`
- `internal/proxy/manager.go`

待修复项：

- `proxyMgr` 创建后没有在 Server 启动时调用 `Start()`，导致数据库中 enabled proxy 不会自动监听。
- `handleTicketCreate` 使用 `string(rune(req.Port))` 保存端口，应该改为 `strconv.Itoa(req.Port)`；width/height 同理。
- `handleAgentSecret` 已有实现和测试辅助，但主路由没有正确挂 `/api/agents/{id}/secret`。
- duplicate agent 判断使用 `activeAgent.IsOnline(0)` 基本不会成立，应改为明确检查连接状态或使用心跳超时。
- `terminalKeepaliveLoop` 的 lastActive 没有被输入更新，可能固定按 CreatedAt 超时。
- `JWT` 配置文件里的 secret/expiration 没有接入 `auth.SetSecret` 和 token 过期策略，当前主要依赖环境变量或随机 secret。

验收标准：

- 新增或修复对应单元测试。
- `go test ./internal/server/... ./internal/proxy/...` 成功。

## Phase 4: Agent 模块边界与可靠性

### 4.1 拆分 Agent RPC Provider

当前问题：

- `internal/agent/rpc/handlers.go` 同时包含 Router、SystemProvider、PVEProvider、DockerProvider、OpenWrtProvider，文件接近 1000 行。

涉及文件：

- `internal/agent/rpc/handlers.go`
- `internal/agent/provider/provider.go`
- `internal/pve/*`
- `internal/docker/*`
- `internal/openwrt/*`

建议目标结构：

- `internal/agent/rpc/router.go`
- `internal/agent/rpc/system_provider.go`
- `internal/agent/rpc/pve_provider.go`
- `internal/agent/rpc/docker_provider.go`
- `internal/agent/rpc/openwrt_provider.go`

执行步骤：

- 先纯移动代码，不改行为。
- 为每个 provider 保留现有测试。
- 如果测试太依赖内部函数，先调整测试包结构或保留兼容 wrapper。

验收标准：

- `go test ./internal/agent/rpc/...` 成功。
- 单个 provider 文件职责清晰。

### 4.2 按能力检测注册 Provider

当前问题：

- Agent 检测了 capabilities，但 RPC provider 注册和能力检测之间的关系不清晰。

涉及文件：

- `internal/agent/agent.go`
- `internal/agent/detector/*`
- `internal/agent/rpc/*`

执行步骤：

- SystemProvider 始终注册。
- 检测到 Docker socket 时注册 DockerProvider。
- 检测到 PVE 配置或环境变量时注册 PVEProvider。
- 检测到 OpenWrt 时注册 OpenWrtProvider。
- Provider 初始化失败时记录日志，但不要阻止 Agent 基础心跳。

验收标准：

- Agent 注册 payload 中的 capabilities 与可调用 RPC provider 基本一致。
- 未安装 Docker/PVE/OpenWrt 的机器仍能启动 Agent 并上报 system。

### 4.3 修复 Agent 重连与子处理器状态

当前问题：

- `proxy.Handler.Start` 使用 `CompareAndSwap(false, true)`，重连后可能不会更新 websocket conn。
- `desktopHandler` send func 使用当前 conn 闭包，重连后需要确认更新。
- reconnect 可能并发触发多个 goroutine。

涉及文件：

- `internal/agent/agent.go`
- `internal/proxy/handler.go`
- `internal/agent/rdp/handler.go`

执行步骤：

- 给 Agent 增加 reconnect mutex 或 atomic 状态，避免并发重连。
- 让 proxy handler 支持 `AttachConn` 或重连时重建 handler。
- 重连成功后确保 heartbeatLoop 不重复启动，messageLoop 不泄漏。
- 增加重连单元测试或使用 mock websocket/codec 做行为测试。

验收标准：

- 断开连接后 Agent 只产生一个重连循环。
- 重连后 proxy/desktop 消息仍发送到新连接。
- `go test ./internal/agent/... ./internal/proxy/...` 成功。

## Phase 5: 协议类型化与安全边界

### 5.1 减少 map[string]interface{} 穿透

当前问题：

- `protocol.Message.Payload` 是 `map[string]interface{}`，各处手动取字段和转换类型。
- Proxy/Desktop/Remote payload 频繁做 base64/string/array 转换，容易出错。

涉及文件：

- `internal/protocol/message.go`
- `internal/protocol/remote.go`
- `internal/protocol/desktop.go`
- `internal/server/api_remote.go`
- `internal/server/api_desktop.go`
- `internal/proxy/*`
- `internal/agent/*`

执行步骤：

- 保留 wire format 不变。
- 在 protocol 包增加 typed decode helpers，例如 `DecodePayload[T any](msg *Message) (T, error)`。
- 为 Register、Heartbeat、RPCRequest、RPCResponse、ProxyNew、ProxyData、ProxyClose、DesktopNew、DesktopData 增加专用解析函数。
- 逐步替换 server/agent 中的手动 map 读取。

验收标准：

- 行为不变。
- 错误请求返回明确错误，不 panic。
- `go test ./internal/protocol/... ./internal/server/... ./internal/agent/...` 成功。

### 5.2 明确远控权限和审计

当前能力：

- Terminal、Desktop、VNC 可以通过 ticket 建立连接。
- API 有 JWT，但远控目标 host/port 目前主要由请求传入。

执行步骤：

- ticket 创建时校验用户权限。
- 限制可连接目标：默认只允许 inventory 中声明的 resource/access endpoint，或配置明确允许 `allow_arbitrary_target`。
- 对每次远控会话写 audit log：用户、agent、protocol、target、开始时间、结束时间、结果。
- 对敏感字段 password 不写日志。

验收标准：

- 未授权用户不能创建远控 ticket。
- 任意 host/port 连接需要显式配置启用。
- AuditLogs 页面能看到远控会话记录。

## Phase 6: 前端模块整理

### 6.1 对齐 API 能力与页面导航

当前问题：

- 前端 `ApiService` 有 `/me/profile`、`/settings` 等调用，但后端主路由未看到对应实现。
- Resources 页面展示多个资源类型，但后端数据来源目前不完整。

涉及文件：

- `web/src/services/api.ts`
- `web/src/pages/Resources/index.tsx`
- `web/src/pages/Settings/index.tsx`
- `web/src/pages/Profile/index.tsx`
- `internal/server/api.go`

执行步骤：

- 列出前端所有 API 调用，和后端路由逐一对表。
- 删除未使用/未实现接口，或补后端实现。
- Resources 页面空数据时说明“inventory 未同步”或展示同步入口，而不是看起来像系统无资源。

验收标准：

- 浏览器控制台不出现 404 API 调用。
- `cd web && pnpm run build` 成功。

### 6.2 拆分胖页面

涉及文件：

- `web/src/pages/Agents/index.tsx`
- `web/src/pages/Resources/index.tsx`
- `web/src/pages/Settings/index.tsx`

执行步骤：

- Agents 拆成列表、详情、secret 管理、连接状态组件。
- Resources 拆成 resource type tabs/table 和 data hooks。
- Settings 拆成 account/security/system/audit 子模块。
- 不改变视觉风格，优先降低维护成本。

验收标准：

- 单个页面文件目标小于 250 行。
- UI 行为不变。
- `pnpm run build` 成功。

## Phase 7: 测试与 CI 收口

### 7.1 调整 CI 覆盖阈值和测试范围

当前问题：

- 总覆盖阈值只有 5%，意义较弱。
- workflow 有重复：`test.yml`、`go.yml`、`agent-test.yml`、`build.yml` 部分重叠。

涉及文件：

- `.github/workflows/test.yml`
- `.github/workflows/go.yml`
- `.github/workflows/agent-test.yml`
- `.github/workflows/build.yml`

执行步骤：

- 保留一个主 Go CI：vet、test、build。
- Agent 专项 CI 只在 agent 路径变更时跑更细测试。
- 为 `internal/inventory`、`internal/server`、`internal/agent` 设置更有意义的包级覆盖目标。

验收标准：

- PR 上不会重复跑三套相同 Go 测试。
- CI 时间下降或保持不变。
- 失败信息更聚焦。

### 7.2 增加端到端最小验证

执行步骤：

- 启动 server 使用临时 SQLite。
- 启动 agent 连接 server。
- 验证 `/api/agents` 能看到在线 agent。
- 执行一次 inventory sync。
- 验证 `/api/resources/*` 返回同步资源。

验收标准：

- 能在本地通过一个脚本执行最小闭环验证。
- 脚本放到 `scripts/` 或 `dev.sh` 中，并在 README 中记录。

## 推荐执行顺序

1. Phase 0：先让测试和依赖稳定。
2. Phase 1：CLI 与文档收口，减少产品入口混乱。
3. Phase 2：补齐 Git-first inventory 闭环，这是项目核心。
4. Phase 3.3：修复已知 Server 行为 bug。
5. Phase 4.3：修复 Agent 重连可靠性。
6. Phase 3.1、3.2、4.1：做小步拆分，降低后续维护压力。
7. Phase 5：协议类型化和远控安全边界。
8. Phase 6：前端整理。
9. Phase 7：CI 和端到端验证。

## 暂不建议做的事

- 暂不引入 Kubernetes 风格 CRD 全量模型，除非先明确 v2 inventory 迁移方案。
- 暂不替换前端 UI 框架。
- 暂不把 SQLite 换成 PostgreSQL，个人 homelab 场景 SQLite 足够。
- 暂不继续扩展更多远控协议，先把现有 Terminal/VNC/Desktop 权限和会话生命周期收稳。
- 暂不做大规模目录重命名，先通过小模块抽离建立边界。
