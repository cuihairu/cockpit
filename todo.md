# Cockpit 架构收口与推进 Todo

本文档用于交给后续模型或开发者逐项执行。优先目标不是继续堆新功能，而是让 Cockpit 的核心定位闭环：Git-first CMDB + Agent 主动连接 + 运行态监控 + Web 控制台。

## 执行原则

- 每个任务单独提交，避免把架构调整、功能补齐、测试修复混在一个提交里。
- 不做大爆炸重构。先补闭环和边界，再拆包。
- 保持现有技术栈：Go 后端、SQLite/GORM、Gorilla WebSocket、React/Vite/Ant Design。
- 对外 API 和前端页面优先保持兼容，除非当前接口明显不存在或错误。
- 每完成一个任务至少运行对应包测试；跨模块任务运行 `go test ./...` 和 `pnpm run build`。
- 不回滚用户已有改动，不删除未确认的功能模块。
- **文档优先**：动代码之前，先把改动落到本文档或 `docs/guide/architecture.md`，让设计可审阅。

## 进度状态（2026-06-13 更新）

| Phase | 状态 | 备注 |
| --- | --- | --- |
| 0 建立可验证基线 | ✅ 已完成 | `go test ./...` 全绿、`go vet ./...` 干净 |
| 1.1 CLI 命令 | ✅ 已完成 | `internal/cli/{init,sync,status}.go` 已实现 |
| 1.2 Agent 入口收口 | ✅ 已完成 | 采用方案 A：`cockpit agent` 与 `cockpit-agent start` 共用 `internal/cli.AgentStartCmd`，参数保持兼容 |
| 2.1 inventory v1 规范 | ✅ 已完成 | `Inventory` 顶层已加 ComputeInstances/Services/Gateways/Storages |
| 2.2 inventory 同步 SQLite | ✅ 已完成 | 6 类资源同步 + Created/Updated 准确区分（GetXxx 判存在） |
| 2.3 inventory watch | ✅ 已完成 | watcher 复用 `inventory.Syncer`，消除两套并行同步逻辑 |
| 3.1 拆 HTTP 路由 | ✅ 已完成 | `Server.registerRoutes` 已抽出，按域分组 |
| 3.2 拆 AgentHub/Dispatcher | ✅ 已完成 | server.go 从 965 行降至 524 行；拆出 `websocket.go`/`dispatcher.go`/`system_info.go`/`alert_loop.go` |
| 3.3 Server 行为 bug | ✅ 已完成 | proxyMgr.Start/duplicate agent 判断/handleTicketCreate strconv/JWT 接入均已修复（todo 原描述已过时） |
| 4.1 拆 Agent RPC Provider | ✅ 已完成 | `handlers.go`(910 行)拆为 `router.go`(107) / `system_provider.go`(57) / `pve_provider.go`(291) / `docker_provider.go`(269) / `openwrt_provider.go`(199) |
| 4.2 按能力注册 Provider | ✅ 已完成 | `internal/agent/providers.go` 新增 `setupProviders()`，按 capability 类型 + 环境变量条件注册 4 类 Provider |
| 4.3 Agent 重连可靠性 | ✅ 已完成 | proxy.Handler.AttachConn、reconnecting atomic、desktopHandler 动态 conn |
| 5.1 协议类型化 | ✅ 已完成 | `internal/protocol/decode.go` 提供 `DecodePayload[T]` 泛型 + Register/Heartbeat/RPCRequest/RPCResponse/ProxyNew/ProxyData/ProxyClose/ProxyError/DesktopDataHeader/DesktopDisconnected 专用 helper。`ProxyData` 三 wire 格式兼容。已完成实际替换：`server.go` handleProxyData/Close/Error、`proxy/handler.go` 三方法、`dispatcher.go` heartbeat、`system_info.go` 改为 `*SystemInfoPayload`、`api_desktop.go` DesktopData/Close、`rdp/handler.go` DesktopClose、`rpc/router.go` RPCRequest。同时补齐协议定义（`ProxyDataPayload.Terminal`、`ProxyClosePayload.Terminal`、`ProxyNewPayload.{ConnID,Terminal,Protocol}`、新增 `DesktopDataHeaderPayload`）。`go test ./...` + `go vet ./...` 全绿 |
| 5.2 远控权限和审计 | ✅ 已完成 | `config.RemoteControl{AllowArbitraryTarget, AllowedTargets}`；`audit` 新增 `ActionRemoteStart/End` + `LogRemoteSession` + `RemoteSessionDetails`（不含 password）；`server/remote_audit.go` 集中目标校验；Terminal/Desktop/VNC 三类 Session 新增 UserID/Username，关闭时自动写 end 审计 |
| 6.1 前端 API 对齐 | ✅ 已完成 | 前端 `PUT /api/me/profile` + `PUT /api/settings` 后端补齐；`User` 表增加 Phone/Department 字段（GORM auto-migrate）；`/me` 响应附带 phone/department；前端 `pnpm run build` 通过 |
| 6.2 拆胖页面 | ✅ 已完成 | 三大页面已按职责拆分：Agents(536→197)→`columns.tsx`/`helpers.tsx`/`AgentDetailModal.tsx`/`useRemoteModals.ts`；Resources(387→80)→`columns.tsx`/`useResources.ts`；Settings(395→66)→`useSettings.ts`/`GeneralSettings.tsx`/`SecuritySettings.tsx`/`AlertSettings.tsx`/`SystemInfo.tsx`/`DisableTOTPModal.tsx`/`authenticatorLinks.ts`。主入口全部 ≤200 行，`pnpm run build` 通过 |
| 6.3 Profile/Settings 闭环 | ✅ 已完成 | `phone`/`department` 已落库并回填；Settings 采用方案 A（浏览器 localStorage 偏好），`siteName`/`theme`/`refreshInterval`/`compactMode`/`showResourceCount` 已持久化并驱动 UI |
| 7.1 CI 收口 | ✅ 已完成 | 删除重复的 `go.yml`；保留 `test.yml`（主 CI，阈值从 5% 提到 25%，实测 62%+）/`agent-test.yml`（路径过滤）/`build.yml`（跨平台构建）/`docs.yml`/`nightly.yml` |
| 7.2 端到端验证 | ✅ 已完成 | 新建 `scripts/e2e-smoke.sh`，本地一键验证 server→agent→inventory sync→/api/resources 闭环；修复副作用 bug：`AuditMiddleware` 包装破坏 WebSocket `Hijacker` 接口（对 `/ws`、`/api/remote/{terminal,desktop,vnc}` 路径跳过包装）；README 增加脚本说明 |

## 当前未完成功能（2026-06-23 复核）

下面不是历史计划，而是基于当前代码重新核实后的真实 backlog。原则：只记录“代码里确实没做完”或“文档/UI 已承诺但链路未闭环”的项。

### P0：前后端协议和主路径不一致

#### 1. 远程终端 / 桌面 / VNC 前端仍走旧的 query-token WebSocket，未切到 ticket + `Sec-WebSocket-Protocol` ✅ 已完成（2026-06-26）

代码证据：

- `internal/server/api_remote.go`、`internal/server/api_desktop.go`、`internal/server/api_vnc.go` 的 WebSocket 入口都要求先通过 `POST /api/remote/tickets` 获取 ticket，再从 `Sec-WebSocket-Protocol` 头读取票据。
- `web/src/components/TerminalModal/index.tsx`
- `web/src/components/DesktopModal/useDesktopWS.ts`
- `web/src/components/VNCModal/index.tsx`

完成情况：

- 前端已新增 `createRemoteTicket()`，统一命中 `POST /api/remote/tickets`。
- Terminal/Desktop/VNC 三个连接器都已改为先取 ticket，再通过 `Sec-WebSocket-Protocol` 建立连接。
- WebSocket URL 已不再携带 JWT、password、host/port 等敏感参数。

#### 2. 前端残留一套不存在的 `/api/remote/connections` / `/api/remote/terminal/start` API ✅ 已完成（2026-06-26）

代码证据：

- `web/src/services/remote.ts`
- 当前后端只实现了 `/api/remote/tickets`、`/api/remote/sessions`、`/api/remote/{terminal,desktop,vnc}`，未实现 `connections` 和 `terminal/start`。

完成情况：

- `web/src/services/remote.ts` 已删除不存在的 `connections` / `terminal/start` 假接口。
- 远控服务层现仅保留真实后端支持的 ticket API。
- 前端协议类型已收紧到后端真实支持集合，`ftp` 已从前端可选集合移除。

### P1：表面可用，但实际未闭环

#### 3. 个人资料里的 `phone` / `department` 目前不会持久化 ✅ 已完成（2026-06-26）

代码证据：

- `internal/server/api.go` 的 `handleCurrentUserProfile` 会接收并设置 `user.Phone`、`user.Department`。
- 但 `internal/storage/user.go` 的 `UpdateUser()` 目前只更新 `email` 和 `role`，没有写 `phone`、`department`。

完成情况：

- `storage.UpdateUser()` 已写入 `phone`、`department`。
- `storage/user_test.go`、`internal/server/api_test.go` 已补覆盖。
- Profile 页已从 `/api/me` 回填并在保存后同步上下文。

#### 4. `/api/settings` 只是兼容空实现，设置页并没有真正持久化或影响全局 UI ✅ 已完成（2026-06-26，方案 A）

代码证据：

- `internal/server/api.go` 的 `handleSettings()` 只校验 JSON 格式并返回 `"Settings saved"`。
- `web/src/pages/Settings/GeneralSettings.tsx` 使用硬编码 `initialValues`。
- `web/src/App.tsx` 顶层 theme/layout 也是硬编码 state，不读取设置页保存结果。

完成情况：

- 采用方案 A：前端 UI 偏好保存在浏览器 `localStorage`。
- Settings 页已加载真实当前值，不再使用硬编码初始值。
- `siteName`、`theme`、`refreshInterval`、`compactMode`、`showResourceCount` 已实际影响 UI/轮询行为。

#### 5. RDP 功能默认构建产物并不真正可用 ✅ 已完成（2026-06-28，方案 B）

代码证据：

- `internal/agent/rdp/handler.go` 只在 `//go:build rdp && !darwin` 条件下启用。
- 默认构建会落到 `internal/agent/rdp/rdp_stub.go`，其中 `HandleDesktopNew/Data/Close` 都是 no-op。
- `.github/workflows/build.yml` 和 README 的默认 `go build` 都没有加 `-tags rdp`。

完成情况：

- 采用方案 B：RDP 明确标记为可选构建能力。
- 默认 `rdp_stub` 收到 `desktop_new` 后返回标准 `desktop_data/error`，不再静默 no-op。
- 前端桌面 WebSocket 收到 `error` 后切回 disconnected，避免一直停留在连接态。
- `internal/agent/rdp/rdp_stub_test.go` 已覆盖默认构建下的显式错误。
- `docs/guide/{concepts,protocol,architecture}.md` 已说明 RDP 需要 `go build -tags rdp`。

### P2：产品边界和文档仍不一致

#### 6. `remote_control.allowed_targets` 文档声称支持 inventory endpoint，但当前实现只读取配置白名单 ✅ 已完成（2026-06-28）

代码证据：

- `internal/config/config.go` 注释写的是“host 必须命中 AllowedTargets 或 inventory 中声明的 endpoint”。
- `internal/server/remote_audit.go` 的 `collectAllowedTargets()` 当前只返回配置中的 `AllowedTargets`，没有从 inventory / resources 派生目标。

完成情况：

- 已收紧 `internal/config/config.go` 注释和 `internal/server/remote_audit.go` 错误提示，明确当前只支持显式 `remote_control.allowed_targets` 或 `allow_arbitrary_target`。
- `config/cockpit.yaml` 增加 `remote_control` 示例配置。
- `docs/guide/{concepts,protocol,architecture}.md` 已同步当前 allow-list 语义。
- `internal/server/remote_audit_test.go` 已覆盖默认拒绝、白名单放行和任意目标开关。

#### 7. `inventory.watch` 文档仍有历史表述，需要按代码事实改写 ✅ 已完成（2026-06-28）

代码证据：

- `internal/sync/watcher.go` 当前已经复用 `inventory.Syncer`。
- 但 `docs/guide/concepts.md` 仍写着“热加载当前只覆盖基础 Agent 同步，不等同完整 sync”。

完成情况：

- `docs/guide/concepts.md` 已改为说明 `inventory.watch` 复用同一套 Syncer，可同步 Agent、Domain、Certificate、ComputeInstance、Service、Gateway、Storage 等资源。

#### 8. 登录页仍展示 `admin / admin` 默认账号，和当前强制 `ADMIN_PASSWORD` 的启动逻辑矛盾 ✅ 已完成（2026-06-28）

代码证据：

- `internal/server/server.go` 启动时强制要求 `ADMIN_PASSWORD`，且长度至少 8 位。
- `web/src/pages/Login/index.tsx` 仍把默认表单值写成 `admin / admin`，并显示“默认账号: admin / admin”。

完成情况：

- `web/src/pages/Login/index.tsx` 已删除 `admin/admin` 默认填充值。
- 登录页页脚已改为提示管理员账号由 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 初始化。

#### 9. 前端仍把 `ftp` 暴露为可选远控协议，但主远控后端并未支持 ✅ 已完成（2026-06-26）

代码证据：

- `web/src/services/remote.ts`、`web/src/components/RemoteServices/index.tsx`、`web/src/components/TerminalModal/index.tsx` 都把 `ftp` 作为可选协议。
- `internal/server/api_remote.go` 的终端主链路只接受 `ssh` / `telnet` / `vnc`，`handleRemoteSessionCreate` 也未真正打通 FTP 语义。

完成情况：

- `web/src/services/remote.ts`、`web/src/components/RemoteServices/index.tsx`、`web/src/components/TerminalModal/index.tsx` 已移除 `ftp` 协议类型和入口。

## 建议优先级（2026-06-23）

1. 先修远控主链路协议一致性：ticket + `Sec-WebSocket-Protocol`。
2. 再修“保存了但其实没落库”的资料与设置问题。
3. 然后处理 RDP 默认构建策略和前端残留假接口。
4. 最后收文档/文案不一致项。

## 当前判断

当前架构方向合理，但实现重心偏移：

- 合理部分：`Server + Agent 主动 WebSocket + SQLite 运行态 + Web UI` 适合个人混合基础设施控制台。
- 最大缺口：README/文档强调 Git-first CMDB，但 `inventory -> SQLite -> API -> Web UI` 只覆盖 Agent、Domain、Certificate，Compute/Service/Gateway/Storage 没有闭环。
- 最大维护风险：`internal/server` 和 `internal/agent/rpc` 已经变成大聚合模块，职责开始混杂。
- 最大产品不一致：文档写了 `cockpit init/sync/status`，但 CLI 未实现；`cockpit agent` 是占位，真实 Agent 在 `cmd/cockpit-agent`。（历史快照；当前 CLI 和 Agent 入口已收口，见上方进度表。）

> 以上"当前判断"为初始状态描述，**作为历史快照保留**；实际进度以"进度状态"表为准。

## Phase 0: 建立可验证基线 ✅

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

- `cockpit agent` 曾是占位提示。
- `cockpit-agent start` 曾是真实实现。

完成情况：

- 采用方案 A：`cockpit agent` 和 `cockpit-agent start` 均复用 `internal/cli.AgentStartCmd`。
- 共享 `-server`、`-id`、`-secret`、`-region`、`-zone`、`-labels` 参数解析和 label 解析逻辑。
- `cockpit agent start ...` 作为兼容形式也可用。

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

**实施设计（2026-06-13 补充）**：采用务实分文件方案，先满足"独立文件"底线，server.go 从 965 行降到 ~400 行：

| 新文件 | 内容 | 预估行数 |
| --- | --- | --- |
| `internal/server/websocket.go` | `handleWebSocket`、`sendRegisterError`、`readLoop`、`writeLoop`、`isOriginAllowed`、`getEnv` | ~150 |
| `internal/server/dispatcher.go` | `handleMessage`（switch）、`handleHeartbeat`、`handleRPCResponse` | ~80 |
| `internal/server/system_info.go` | `handleSystemInfo`（心跳系统信息解析与持久化） | ~120 |
| `internal/server/alert_loop.go` | `alertCheckLoop`、`runAlertChecks`、`cleanupOldAlerts`、`cleanupLoop`、`metricsCleanupLoop` | ~80 |
| `internal/server/server.go` | Server struct、NewServer、Start、Shutdown、registerRoutes、startInventorySync、handleHealth、CallAgent、toStorageAgent、handleLoginWithAudit、SendToAgent/GetAgentConn、handleProxyData/Close/Error、decodeProxyData、hasPrefix | ~400 |

抽 AgentHub/AgentAuthenticator/MessageDispatcher **接口**留作后续小步迭代，先保证文件边界清晰。

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

## 当前待办（2026-09-14）

路线图 P0 三项已实现，工作区存在约 1682 行未提交改动（`go build` / `go test ./...` 全绿）。按「每个任务单独提交」原则收口：

1. `internal/probe/` 健康探测 runner + Storage 状态更新方法（含 `runner_test.go`）。
2. `web/src/pages/Docker/` 前端 Docker 页面 + `api.ts`/`types` 配套改动。
3. 测试补充（`agent_ws_test`、`codec_ws_test`、`handler_flow_test`、`watcher_reload_test` 等 10 个新测试文件）+ `docs/guide/agent-egress-sdwan.md` + sidebar/参考对比文档。

收口后进入路线图 P1，首选「应用部署（Compose Stack）」，其次「拨测增强」。与同类项目的完整对比分析见 `docs/guide/reference-projects.md`。

## 数据竞态收口（2026-09-17）

覆盖率推至 98.6% 后主线转向 `go test -race` 收口，本轮修复三处数据竞态：

1. ✅ `internal/proxy`：测试 mock 的 `messages` 切片被后台协程持锁追加、测试裸读（dbe1e8b0）。
2. ✅ `internal/auth`：密码重置令牌全局 map **生产代码无锁**——HTTP handler 并发读写 + `GenerateResetToken` 起后台清理协程遍历删除；加 `resetTokenMu`，顺带删除名字像锁实为冗余映射的 `resetTokenStoreMutex`（a849ada8）。
3. ✅ `internal/agent`：`connect()` 直接写全局单例 `websocket.DefaultDialer.HandshakeTimeout`，与并发 Dial 构成竞态；改为值拷贝（d9f28dcc）。

配套：

- ✅ CI `test.yml` 新增 `go test -race -short ./...` 兜底步骤（此前只有普通 `go test`，这类竞态 CI 抓不到）。
- ✅ 全仓 `-race -short` 终验通过。

## web 前端存量收口（2026-09-22）

全页面测试系列收官（33/33 页面有测试，Drift 页收尾）后，两笔清掉 web 侧全部存量：

1. ✅ **Agent/资源地域字段对齐后端 API**（8f928baf）：后端 `storage.Agent`/`ComputeInstance` 顶层序列化 `region`/`zone`，从无 `location` 对象；前端类型错位导致 Agents 地域列与筛选、Dashboard 地域列、Docker 面包屑、Workbench 搜索与概览、Network 面板标签在真实环境**恒空**（页面测试 mock 沿用错位 shape 所以从未暴露——mock 数据应以后端真实序列化为准，不能互相抄）。`Agent`/`ComputeInstance` 类型改顶层可选 `region`/`zone`；`Gateway`/`Storage` 后端无地域字段，删除 `location`；`Location` interface 删除；10 处 reader（含 `src/workbench/`）、19 个测试文件 mock 同步迁移；Network 面板标签断言更新为「主机名 · 地域」（修复后地域首次真实显示）。
2. ✅ **tsc 存量错误清零**（72adcbf8）：`tsc --noEmit` 全仓 0 错误（此前 7 个存量）。navTheme 映射 realDark、ErrorBoundary 未用 React 导入、ServerBackupCard enabled 收敛 Boolean、useSettings 剔除 null refreshInterval、新增 `vite-env.d.ts`（vite/client）。

验收：`pnpm build` 通过；全量 vitest 68 文件 / 473 用例全绿、退出码 0 无 Errors。

## 未来路线图（个人云场景功能扩展）

> 2026-07-15 复核，2026-09-14 更新（打勾状态核对 + 按参考项目对比标注方案来源）。针对「个人云基础设施控制台」定位，盘点当前架构已支撑但前端/自动化未覆盖的常见场景，按优先级规划。后端能力储备较充分，多数条目是前端页面 + 自动化逻辑的补齐。

### P0 — 近期（核心场景补全）✅ 全部完成

- ✅ **Docker 容器生命周期管理**：已实现。后端 `docker_provider.go` 支持 start/stop/restart/remove/pause/unpause，前端 `/docker` 页面操作列含全部按钮（停止/重启/暂停/启动/恢复/删除，带二次确认 Modal）。路由设计：`POST /api/docker/agents/{id}/containers/{cid}/{action}`。前端通过 `useMutation` 操作后自动刷新容器列表。
- ✅ **Docker 容器日志实时查看**：已实现。后端 `containers.logs` 支持 tail/timestamps，前端「日志」按钮打开 Modal（深色终端风格背景，支持 50/100/200/500/2000 行选择，可手动刷新，自动剥离 Docker 流多路复用头控制字符）。路由设计：`GET /api/docker/agents/{id}/containers/{cid}/logs?tail=N&timestamps=true`。
- ✅ **自动健康探测**：已实现。新增 `internal/probe/runner.go`，每 5 分钟一轮完整探测。域名走 DNS + HTTP HEAD；服务按 type 自动选探测方式；证书走 TLS 握手。通过 `UpdateServiceStatus/UpdateDomainStatus/UpdateCertificateStatus` 回写数据库。Storage 层新增 3 个状态更新方法。（遗留增强项已列入 P1「拨测增强」。）

### P1 — 中期（运维自动化）

- **应用部署（Compose Stack）**：Docker Compose Stack 管理（上传/编辑 compose.yml、up/down、状态总览），类似 Portainer 的轻量版。→ 方案参考：Komodo 的 Git-to-deploy（栈文件存 Git 仓库 + 触发部署）+ Dockge 的 compose-file-first；Cockpit 已有 `docker_provider` RPC 链路，投入产出比最高。**建议 P1 首个启动。** ✅ 方案设计完成（2026-09-14）：`docs/guide/stack-deploy-design.md`；✅ M1 开发完成（2026-09-14）：Agent stack provider + `/api/stacks` REST + storage 缓存 + 审计 + Web `/stacks` 页面（列表/详情/compose 编辑/异步任务轮询/部署日志）；✅ M1.5 体验补齐完成（2026-09-14）：restart/pull 动作、部署历史表+详情页时间线、模板库（5 个常见服务模板）+import、stacks 目录自检提示；✅ 本机端到端验收通过（2026-09-14，`scripts/local-acceptance/`，42 项断言全过，覆盖 M1+M1.5；顺带修复 detector 不支持 `DOCKER_HOST=unix://` 惯例格式），剩余：真实 Docker 测试机验收（镜像拉取/容器健康/compose 完整语义/目录权限）。
- **拨测增强**：探测间隔可配置（当前硬编码 5 分钟）、通知渠道集成（ntfy/webhook/Telegram，告警框架已有）、心跳条式状态历史 UI。→ 方案参考：Uptime Kuma；Cockpit 差异化优势是 probe 天然联动 inventory 资源 + 经 Agent 所在网络分布式探测。✅ M1 完成（2026-09-14）：探测间隔 UI 可配（30-3600s，DB 持久化 + 立即生效）、多渠道通知（herald/ntfy/webhook/telegram，config.yaml 配置）、服务宕机/恢复即时通知（连续 2 次失败防抖）、测试通知按钮。✅ M2 完成（2026-09-15，P1 三大遗留项全部清零）：`docs/guide/probe-enhance-design.md` M2 章节（决策 D7-D16）——心跳条式状态历史（新 `probe_results` 表每轮批量落库、30 天保留每 24h 清理、`GET /api/probe/history`、Resources 服务列表 Uptime Kuma 式心跳条 30 格带 Tooltip、`HeartbeatBar`/`ProbeHeartbeatCell` 组件）；告警阈值配置化（5 个 Setting 键：失败判定阈值 probe.fail_threshold 1-10、磁盘/内存 50-99%、证书警告/提醒天数，`PUT /api/probe/config` 全量保存 + probe 原子值即时生效 + alert 每轮从库刷新）；重复告警去重（`createAlertIfNotExists` 改为按 resource+title 未读告警真去重——持续故障只建一条只通知一次，用户标已读后再现重新告警，6 项检查全部受益）。✅ domain/certificate 列表心跳条复用完成（2026-09-17）：Resources 页两列表新增「最近状态」列，复用 `ProbeHeartbeatCell`（`resource_type=domain/certificate`），与 service 列同款。
- **备份管理**：数据库/配置/卷的快照与恢复，定时备份策略 + 异地保留。这是个人云数据安全的核心缺口。✅ 方案设计 + M1 完成（2026-09-14）：`docs/guide/backup-design.md`；Agent 本地 tar.gz 打包（复用 stack 异步任务模型，路径穿越防护 + 符号链接不跟随）+ retention 自动清理、server 每分钟调度（daily@HH:mm / every:Nh / manual，停机补跑一次）+ 运行历史 + 失败 backup.failed 通知、REST `/api/backups` + 审计、Web `/backups` 页面（配置 CRUD/立即运行/历史/文件浏览删除）。✅ M1.5 完成（2026-09-14）：恢复 restore（独立目录解包绝不覆盖 + Zip Slip 防护 + 双确认：UI 输入备份名 + API confirm_name 校验 + 审计）、备份文件下载（分块 RPC backup.read base64 经 server 流转发，server 不落盘，256KB 分块 + 15 分钟总超时）、Web 文件行下载/恢复 + 恢复 Modal 任务日志轮询。✅ server 自身 SQLite 备份完成（2026-09-15）：`docs/guide/server-backup-design.md`；SQLite `VACUUM INTO` 在线热备（不停机，产物为紧凑完整副本，测试用 GORM 重开验证数据完整）到 `<db目录>/server-backups/cockpit-YYYYMMDD-HHMMSS.db`（0600，目录即事实源不建表）；独立调度循环（1h ticker + 目录最新文件名时间戳对比，Setting `server_backup.interval_hours` 0-168 默认 24、0=关闭；`retention_days` 0-365 默认 7、0=永久，小时节流清理）；REST `/api/server-backups` 列表/配置/立即备份/下载/删除（文件名严格模式防穿越；下载/删除/手动备份/配置变更记审计）；Web /backups「面板数据库」卡片（定时开关+间隔/保留配置、立即备份、列表下载删除、恢复=停服替换说明）。剩余：真实 Docker 测试机验收、数据库热备钩子。✅ M2 异地保留完成（2026-09-19）：`backup-design.md` D18-D25；agent 侧 rclone copy 推送（exec argv 不走 shell，remote:path 正则校验 D22，RemoteDest 空=不启用；推送失败只记 RemoteStatus/RemoteError 不改任务终态 D21）+ `backup.remote.sync` 手动补传 + capability metadata rclone 探测（D23）；server 配置期校验 + requireAgentRclone 400 拦截 + track 回填 Remote* + `backup.remote-failed` 独立通知 + `POST /configs/{id}/files/sync-remote` 端点（backup_remote_sync 审计）；Web 配置 Modal 异地目标输入 / 运行历史异地 Tag / 文件行补传按钮（D25）。真机验收剩余：真实 S3/B2 远端、rclone 网盘限速、GB 级完整链。✅ M3 数据库热备钩子完成（2026-09-19）：`backup-design.md` D26-D32；BackupConfig.PreHook（≤1024，AutoMigrate 零迁移），agent 打包前经 sh -c 执行（unix 独立进程组超时 5min 整组 kill），失败/超时即中止本次备份（不打包不推送不清理 retention，D28）+ `[hook]` 任务日志；Web 配置 Modal「前置命令（可选）」TextArea（sqlite3 .backup 占位示例）。剩余：真实 Docker 测试机验收、数据库热备钩子真实验证（mysqldump/pg_dump 真库）。✅ M2 异地传输完成（2026-09-19）：`server-backup-design.md` D11-D18——Setting `server_backup.remote_dest`（空=关闭，写时同 agent D22 正则校验+512 上限，读时脏值视为未配置不阻塞本地备份）；本地 VACUUM INTO 成功后同步 `rclone copy --transfers 2` 推送（exec 5 分钟超时 + **WaitDelay 强断管道**——测试暴露 CombinedOutput 管道写端被子进程继承时 Wait 被孤儿拖住、超时形同虚设），失败只记 `[remote]` 日志 + `server_backup.remote-failed` 事件白名单通知，不动本地成果与 API 返回（D21 语义对称）；config GET 带 `rclone_available` 实时 LookPath 探测（后装免重启）；`POST /server-backups/{name}/sync-remote` 手动补推（D7 严格文件名校验 + 审计）；web 面板数据库卡片配置行加「异地目标」输入 + rclone 未检测到提示，文件列加「补推」按钮。server 7 新测试全绿，web tsc 零新增（9 存量）+ build 过。剩余：真机 rclone 远端（gdrive/S3）实测。
- **反向代理/路由管理**：Nginx/Caddy/Traefik 配置可视化与下发，把「网关」资源从记录升级为可管理对象。✅ 方案设计 + M1 完成（2026-09-15）：`docs/guide/proxy-design.md`；M1 仅 Nginx（宿主机裸装/systemd），只管理 `/etc/nginx/conf.d/cockpit-site-*.conf` 自己名下片段（用户配置零接触），站点元数据内嵌文件头 `# cockpit:meta {json}`（回读自己渲染的产物，无需解析 nginx 语法）；应用流程「渲染 → nginx -t → 写 → reload，-t 失败不落盘、reload 失败回滚再 reload」（apply 失败站点照旧的强保证）；reload 优先 systemctl、无 systemd fallback `nginx -s reload`；Agent nginx provider（status/sites/site.get/site.apply/site.delete，Commander 抽象可测）+ `nginx-proxy` capability 探测（LookPath + `nginx -v`）；server `/api/agents/{id}/proxy/*` 5 端点纯转发不落库（agent 侧文件为唯一事实源）+ 双端参数校验 + proxy_apply/proxy_delete 审计（不含证书内容）；Web `/proxy` 页面（agent 选择标注 nginx 版本/状态卡/站点列表/新建编辑 Modal 含 nginx -t 错误摘要展示/配置预览）。✅ M2 Traefik 后端设计完成（2026-09-18）：`proxy-design.md` M2 章节（D11-D16，未实施）——仅 Traefik file provider 动态目录（与 nginx conf.d 片段同构，D2 零接触原样成立；Caddy 因 Caddyfile 整文件语义需改用户主文件加 import、破坏零接触而暂缓）；动态目录探测取静态配置 `providers.file.directory` → 缺省 `/etc/traefik/dynamic`，capability `traefik-proxy` = 目录存在可写（不依赖 LookPath——Traefik 多容器化宿主无二进制）；无 `-t`/reload 等价物，渲染后 yaml.Unmarshal 自检 + router→service 引用校验失败不落盘，file provider 热加载坏文件局部隔离（弱在无全局预检、强在无回滚分支）；渲染映射 Host rule/loadBalancer/双 router（80 redirectScheme 跳转 + 443 tls + 文件级 tls.certificates 直引 ACME 推送证书）、websocket 字段 no-op（Traefik 原生透传）、extra 拒绝（防任意 YAML 注入）；meta 首行 YAML 注释同构 + drift kind 白名单扩 traefik。✅ M2 实施完成（2026-09-18）：agent `traefik_provider.go`（DetectTraefik 探测 + `COCKPIT_TRAEFIK_DIR` 覆盖、渲染 yaml.Marshal 结构体、checkTraefikYAML 自检、临时文件+rename 原子写、baseline Record/Forget、Type()="traefik"）+ `traefik-proxy` capability（dynamicDir 入 metadata）+ providers.go 注册 + drift DynamicDir 分支（Check 枚举/missing 补全/currentContent/validDriftTarget 放行）；server 一处偏差修正——设计 D11 写「RPC 方法名 proxy.* 不变」但代码现实前缀是 nginx.*，实施按现实走：`proxyRPCPrefix` 按 agent capability 分流 nginx.*/traefik.*（双后端 nginx 优先，旧 agent 回退 nginx.*），转发逻辑仍不感知后端差异；web 状态卡加「后端」行（Traefik 青色 Tag）+ 生效方式热加载文案 + 片段后缀/提示语按后端切换 + extra 控件 traefik 禁用，drift KIND_LABEL/COLOR 加 traefik；测试渲染两形态/YAML 自检拒绝/extra 拒绝不落盘/生命周期含基线/目录探测（env 覆盖+静态配置解析）/drift 四态+diff/server 前缀分流四例+traefik 审计。剩余：Traefik 真机验收（容器挂载宿主目录热加载）、nginx 真实主机验收（systemd reload 与 80/443 冲突场景）。证书签发联动已完成（见 ACME 项 D14：签发/续期自动推送到 agent 指定路径，站点直接引用）。
- **文件管理器**：通过 Agent 远程文件浏览/上传/下载（Workbench 目前只有终端和桌面），支撑配置编辑与日志文件查看。→ 可同时参考 Guacamole 的远控文件传输通道设计。✅ 方案设计 + M1 完成（2026-09-15）：`docs/guide/file-manager-design.md`；Agent file provider（list/read/write/mkdir/delete/rename，分块 base64 RPC 与 backup.read 同构，全平台注册）、双端路径校验 + symlink 拒绝（不跟随，删除只删链接本身）、追加模式要求文件已存在（防分块上传碎片）、server `/api/agents/{id}/files/*` 转发 + 下载流式转发（server 不落盘）+ 变更类操作审计（file_write/mkdir/delete/rename）、Workbench「文件」Tab（面包屑/手输跳转/编辑 ≤1MB/上传 ≤10MB/下载不限大小/目录删除输入名字确认）。剩余：断点续传大文件上传、chmod/chown。✅ M2 文本搜索完成（2026-09-18）：`file-manager-design.md` D10-D13——agent `file.search` 纯文本 contains（防 ReDoS 与 logs grep 同纪律）默认大小写不敏感、递归深度 ≤8/排除 .git 与 node_modules/跳过 symlink+二进制+>1MB 文件/文件数 5000/命中 200 即停/15s 超时（truncated+skipped 计数可解释，命中即停返回部分结果）；server `POST .../files/search` 双端同规则校验不审计；FileBrowser「搜索」Modal（当前目录为根、命中路径点击跳转、行号+高亮、扫描/跳过计数）。✅ M2 图片预览完成（2026-09-18）：`file-manager-design.md` D14——纯 web 零 Go 改动，复用 `files/read` 分块端点（offset 按已读字节数推进，`size` 是总大小不是游标）；扩展名白名单 jpg/jpeg/png/gif/webp/bmp/svg/ico/avif + >20MB 拒绝引导下载；Modal 关闭置 token 中止在途分块；Blob 按扩展名映射 MIME 不信任内容嗅探；`<img>` 渲染（SVG secure static 模式脚本不执行，绝不 innerHTML）；透明底纹深色底 + 自然分辨率页脚 + 下载按钮 + URL revoke 三时点。✅ M3 分块上传/断点续传完成（2026-09-18）：`file-manager-design.md` D15-D19——web 循环既有 `files/write`（agent 零改动，server 端点零改动）：≤10MB 单块不变、更大分块（单块 1MB = agent 上限满额，首块 `truncate=true` 建文件、续块 `truncate=false` append——agent 既有防碎片设计正式启用），每块以返回的写入后总大小自证连续性（不一致即终止防交错损坏）；失败自动探测服务器文件实际大小续传 ≤3 次（断点续传/响应丢失跳过已落盘块/目标丢失首块重建/并发写拒绝覆盖）；上限 2GB（更大引导 scp/终端）；server write 审计按 truncate 分流（仅新写入会话记 `file_write`，append 续块不记——500MB 上传不再 500 条刷屏）；工具栏 Progress 进度 + 取消（token 递增中止 + 删除半成品静默）+ antd onChange fileList 累积按 uid 去重。server 1 新测试（truncate=true 审计/false 不审计/缺省照记）全绿；web tsc 零新增错误 + build 过。✅ M4 chmod/chown 权限编辑完成（2026-09-19）：`file-manager-design.md` D20-D22——agent `file.chmod {path,mode}`（0-0o777 不碰 setuid/sticky）/`file.chown {path,uid,gid}`（数字 0-uint32，不做用户名解析——跨机用户名空间不可靠），symlink 一律拒绝（D7 不跟随延伸），无平台门禁（Windows 诚实报错）；list 条目增补 uid/gid（`fileStatOwner` 按 GOOS 拆文件 file_stat_unix/windows，unix 取 Stat_t、windows 恒 -1；darwin/windows 交叉编译验证）；server `/files/chmod`、`/files/chown` 双端同规则校验 + 审计 file_chmod/file_chown（detail 记八进制 mode 与 uid/gid）；web 行操作「权限」按钮（symlink 禁用）→ Modal（rwx 九宫格 + uid/gid 输入 + 合成八进制实时显示），应用按变化分流（位变 chmod、uid/gid 变 chown 需同填）。agent 测试（chmod roundtrip/chown 同值/list 归属/symlink 越界缺参拒绝）+ server 测试（转发/校验 7 形态/审计/失败不记）全绿；web tsc 零新增错误 + build 过。剩余：跨刷新断点续传（服务端上传会话状态）。✅ M5 跨刷新断点续传完成（2026-09-19）：`file-manager-design.md` D23-D25——关键观察：续传游标（远端文件实际 size）agent 侧 M3 D16 已有探测机制，刷新丢的只是「上传坐标记忆」，localStorage 单浏览器语义恰好覆盖，**推翻原设想的「服务端上传会话状态」，纯 web 零 Go 改动**；分块上传每块成功 upsert `{agentId,dir,name,size,lastModified,uploaded,ts}` 进 localStorage（容量 10 FIFO + 7 天 TTL，读写 try/catch 隐私窗口静默缺席），成功/取消移除、重试耗尽的最终失败保留（断网关页面正是续传场景），≤10MB 单块不登记；FileBrowser 挂起 Alert（上传中隐藏）每行「继续」重选本地文件（File 句柄不可恢复）→ 校验矩阵：size 不一致报错保留、mtime 不符 Modal 确认混合内容、远端 null/0 从头、1MB 整块对齐从游标续、已完整清记录、非对齐/超出 Modal 确认覆盖重传；uploadChunked 参数化（dir 显式 + resume 起始偏移）复用 size-mismatch/并发写保护；「放弃」只清本地记录不删远端半成品。web tsc 零新增（9 存量）+ build 过；server/agent 零改动零回归。

### P2 — 远期（生态扩展）

- **drift 漂移视图**：inventory 声明与 Agent 实报不一致时在 API/UI 高亮。→ 方案参考：NetBox 的期望态（desired）/实际态（actual）分离；Git-first CMDB 的差异化卖点。✅ M1 配置漂移完成（2026-09-15）：`docs/guide/drift-design.md`（主体实现为配置漂移：面板写路径产生基线 vs 磁盘当前内容）；Agent drift provider（本地基线 JSON `/var/lib/cockpit/drift-baseline.json` 原子写 + drift.check 实时比对，ok/drifted/missing/no_baseline 四态）、BaselineRecorder 挂钩注入 nginx（ApplySite 后/DeleteSite 后）/cron（写回后，只对 cockpit 段 hash）/stack（SaveStackFile compose+.env / RemoveStack 后）三 provider、`drift` capability（nginx/cron/docker-api 任一）；server `/api/agents/{id}/drift/check` 纯转发不落库不审计；Web `/drift` 页面（agent 选择按 capability 过滤 + 检查 + 汇总告警条 + 四态清单含 hash 短码）。✅ M2 定时巡检+漂移告警完成（2026-09-15）：server `drift_scan.go` 巡检循环（1min ticker + lastScan 对比，间隔动态可改；对全部带 drift capability 的在线 agent 逐个 drift.check）+ alert `CheckDriftScan`（每 agent 一条汇总告警，明细最多 10 行 + 截断提示，复用真去重：同主机未读期间只报一次、已读后再漂移重新告警；error 态只记日志不告警）+ Setting `drift.scan_interval_seconds`（[0,86400]，0=关闭，默认 1800）+ `GET/PUT /api/drift/config` + Web /drift「自动巡检」设置行（开关+间隔分钟，保存即生效）。agent 侧零变更。✅ M3 漂移 diff 视图完成（2026-09-18）：`drift-design.md` D19-D22；BaselineEntry 加 `Content`（Record 时随 hash 存原文，>256KB 只存 hash；旧基线无原文 diff 明确报错引导重新保存，零迁移）；agent `drift.diff` RPC（expected=基线原文，current=实时读盘与 check 同源；cron 两侧展示层 json.Indent 美化，hash/基线层保持 compact 不动——升级不误报；kind 白名单 + name 形态校验防穿越，stack 文件名白名单 compose.yml/.env）+ server `POST /api/agents/{id}/drift/diff` 双端同规则校验转发不审计 + Web drifted 行「差异」按钮 → Modal（自写 LCS 行级统一 diff `utils/lineDiff.ts`，两侧各截 1000 行提示，del/add 着色 + 双侧行号，失败透出 agent 原因，页脚基线记录时间 + 消漂移指引）。agent 8 新测试（Record 存原文/截断边界、nginx 手改与删除、cron 美化与同源、stack 两文件、旧基线报错、两侧过大、校验 13 形态）+ server 2 新测试（转发参数、校验 5 形态与不达 agent、错误透传）全绿；web tsc 零新增错误 + build 过。✅ M4 手动登记「以当前为准」完成（2026-09-18）：`drift-design.md` D23-D25；agent `drift.record`（validDriftTarget 校验 → currentContent 实时读与 check/diff 同源 → baseline.Record 登记原文，missing/error 读不到在此报错，返回 recorded+sha256）；server `POST /api/agents/{id}/drift/record`（与 diff 共用校验转发，仅成功后记审计 `drift_record`/`drift_baseline`，resourceID=kind/name——用户主动变更漂移判定标准可追溯，forwardDriftRPC 返回成功与否供分流）；Web drifted 行「以当前为准」/no_baseline 行「登记」（Popconfirm 确认，成功自动重查刷新）。agent 3 新测试（no_baseline 登记变 ok 含 diff 原文可用、cron/stack 登记、drifted 重登记、读失败与校验拒绝）+ server 1 新测试（转发参数校验、审计落库、agent 失败不记审计、400 不达 agent）全绿；web tsc 零新增错误 + build 过。✅ M5 双栏对照 diff 与配置着色完成（2026-09-19）：`drift-design.md` D26/D27——纯 web 零 Go 改动，DiffModal 加 Segmented「对照/统一」视图切换（默认对照，Modal 960→1080）；`pairDiffLines` 统一 diff 转行对（same 直通双栏、连续 del/add 块按序配对、多出一侧独占行、缺位半边底纹），LCS 输出不动；`configHighlight.ts` 单行无状态轻量着色（注释行/行首键 yaml·env·JSON 带引号键/nginx 首词指令仅 word…; 与 word…{ 形态/引号串/独立数字两侧不贴词与点/{}; 标点；不做跨行状态、不猜行内 # 注释，覆盖下发四类内容不引高亮依赖）；tsx 运行时校验（含 hunter2 不误染、行内 # 不猜）+ tsc 零新增错误 + build 过。✅ M6 CMDB 一致性完成（2026-09-19）：`drift-design.md` D28-D30——「不做」末项立项，配置漂移主线到 CMDB 主线的收口（配置漂移管「面板下发的文件有没有被改」，CMDB 一致性管「Git 里声明的机器还是不是那台机器」）。数据事实决定形态：声明态只活在 inventory YAML（watcher 热加载），实报 hostname/IP 由注册覆写同一 DB 行、无持久化声明副本，故**不建新表、不起巡检循环、不进告警框架**，按需读模型比对。inventory `CompareAgents` 纯函数（无 IO 可全表测；四态 ok/mismatch/unregistered/undeclared + field 级 mismatch 明细 + summary 计数；仅声明非空才比对，hostname 大小写不敏感 + TrimSpace 即 DNS 语义豁免，IP 精确串匹配）+ `Manager.Consistency()` 薄封装 + `GET /api/inventory/consistency`（JWT，浏览类不审计同 drift check 口径，未启用 inventory 503 指名引导 `inventory.path`）；Web 漂移页 Tabs 化「配置漂移 / CMDB 一致性」（卡片标题随改「漂移与一致性」），一致性 Tab useQuery 30s staleTime + 手动刷新，汇总条（共 N 台 + 四态计数 Tag）+ 四态表（状态 Tag 语义 Tooltip + 声明/实报双列事实快照）+ mismatch 行展开字段级对照，503 与空声明均出引导。inventory 3 测试 + server 2 测试全绿（-race 全包过）；web tsc 零新增错误 + build 过。
- **远控会话录制**：终端输出录制，补强远控审计（当前审计只有会话开始/结束）。→ 方案参考：Guacamole 会话录制、Teleport 会话粒度审计。✅ M1 完成（2026-09-15）：`docs/guide/recording-design.md`；server 侧挂在 agent→浏览器转发管道（HandleTerminalData）输出流录制，agent 零变更；asciinema v2 `.cast` 格式落盘（`data/recordings/<sessionID>.cast` 0600）+ `TerminalRecording` 元数据表（结束时回填时长/字节，Close 幂等覆盖双结束出口）；只录输出不录输入（输入含密码）；Setting `recording.enabled`（默认开）+ `recording.retention_days`（默认 7，0=永久，挂 cleanupLoop 小时节流清理）；REST `/api/recordings` 列表/取内容/删除（取内容与删除记审计，列表不记）；Web `/recordings` 页面（列表含进行中标记 + 自研 xterm 回放器 1/2/4/8x 倍速 + 下载/删除）。剩余（后续版本）：VNC/RDP 桌面流录制、实时旁观。✅ M2 完成（2026-09-19）：录制归档异地 + 配置入口（`recording-design.md` D14-D19）——Setting `recording.remote_dest`（写时校验同 server 备份 M2 D13，读时脏值视为未配置）；复用抽出的 `rcloneCopyLocalFile` 共用执行器（server 备份与录制同源，含 WaitDelay 强断管道）；录制 Close 回填元数据后异步推送（不拖会话出口路径），失败发 `recording.remote-failed` 通知（本地档在，retention 窗口内可补推）；`POST /api/recordings/{sid}/sync-remote` 手动补推（sid UUID 严格校验 + 审计）；`GET/PUT /api/recordings/config` 补 M1 缺口（enabled/retention_days 此前无写入口）；web /recordings 页配置行 + 列表补推按钮。剩余：rclone 真远端推送真机验证。
- **DNS 管理**：Cloudflare API 集成，域名资源联动记录增删改。✅ M1 完成（2026-09-15）：`docs/guide/dns-design.md`；server 直连 Cloudflare API v4（Agent 不参与，与 probe/通知同款出站路径）；`internal/dns` Provider 接口（ListZones/ListRecords/Create/Update/Delete）+ Cloudflare client（Bearer token、超时 15s、per_page=50 分页、`{success,errors}` 信封解包、发出前归一化 type 大写/TTL=0→auto）；token 走 config.yaml `dns.cloudflare.api_token` 或 env `CLOUDFLARE_API_TOKEN` 优先（secret 不落 yaml），未配置统一 503 + 引导文案（token 绝不进响应/错误/审计）；REST `/api/dns/status|zones|zones/{zid}/records`（GET/POST/PUT/DELETE，不落库），zones 对照 Domain 表标注 `in_cmdb`（只读联动）；type 白名单 A/AAAA/CNAME/MX/TXT/NS/SRV/CAA 双端同规则；dns_create/dns_update/dns_delete 记审计（resourceID=zoneID/记录名或ID）；Web `/dns` 页面（zone 下拉含未登记 CMDB 标注 + 类型过滤 + 记录表 + CRUD Modal 含 proxied 仅 A/AAAA/CNAME + 翻页 + 未配置引导卡片）。✅ M2 设计完成（2026-09-19）：`dns-design.md` M2 章节（D11-D17）——provider 统一（复用 ACME D13 全局 `dns.provider` 与凭据，消除 D13 语义分叉：一个键管 ACME 签发与记录管理两处）；DNSPod 手写 form client（dnsapi.cn，status.code==1 判成功）+ 阿里云手写 RPC V1 签名 client（alidns.aliyuncs.com 2015-01-09，HMAC-SHA1 percentEncode）——两者与 cloudflare.go 同构 httptest 全可测，不引 SDK（依赖树里的 alidns-20150109 系 lego 带入且 tea 栈难注入测试端点）；概念映射（Zone.ID=域名、记录名子域↔全名互转、分页归一）+ 记录字段差异吸收（proxied 非 CF 忽略、MX 首词优先级拆装、SRV 四段拆装、DNSPod TTL auto 省参）；API 面/审计/校验全复用，/api/dns/status 扩 {provider,configured}；web 引导卡三选一 + proxied 列分流；智能线路固定「默认」明示。✅ M2 实施完成（2026-09-19）：`internal/dns/dnspod.go|alidns.go|sign.go` + `dns.New` 工厂按 provider 分流（server 构造点改造，requireDNS 503 各报各的键，status 回显真实 provider，MX/SRV 拆装错进 400）+ web 引导卡三选一与 proxied 列仅 cloudflare；dns 包 21 测试（公共参数/分页/MX·SRV 拆装/错误码/签名表驱动+桩侧复算）+ server 3 测试（三 provider 503 分流/端到端/审计不回归）全绿（c850d662、5bdbdf8d）。剩余（后续版本）：Domain 写联动、批量导入导出、经 Agent 分布式解析检查、真机验收（DNSPod 免费版 TTL 下限/CAA、两家 SRV 编辑实测）。
- **定时任务/Cron 管理**：Agent 侧 crontab 可视化与调度，支撑自动化运维。→ 可与 Komodo「自动化程序」编排理念结合。✅ 方案设计 + M1 完成（2026-09-15）：`docs/guide/cron-design.md`；Agent cron provider（crontab -l 读 / 临时文件 crontab <file> 写回，Commander 复用 nginx 抽象），cockpit 只管理自己名下任务对（`# cockpit:job {meta-json}` 注释行 + 命令行，停用渲染为注释行不丢失），写回 = 读全量 → 只替换 cockpit 段 → **写回前自检非 cockpit 行与旧内容逐行一致**（绝不误伤用户手写条目），provider 级 mutex 串行化读改写；cron 表达式双端同规则校验（5 字段范围 + `*/0` 步长拒绝 + @ 白名单）；`cron` capability（LookPath crontab）；server `/api/agents/{id}/cron/*` 4 端点纯转发不落库 + cron_apply/cron_delete 审计；Web `/cron` 页面（agent 选择/状态卡/任务列表含启停 Switch/常用表达式预设/外部条目只读折叠面板）。✅ M2 下次触发预览完成（2026-09-18）：`cron-design.md` D14-D17；agent `cron_next.go` 自写解析器（语法域与 validateCronExpr 同源：cronFieldRe + cronFieldRanges + cronAtExtensions；@ 简写等价展开、@reboot 返回 0；dow 7 归一 0 不修改循环变量以免连累步长；Vixie dom/dow 联合语义——双受限 OR、单受限 AND），服务器本地时区逐分钟扫描（从下一整分钟起、AddDate(5) 上限，2/30 等 5 年无触发返回 0 不致命）；Jobs() 每 job 加 `next_run`（disabled 不计算恒 0）；Web 任务表加「下次触发」列（dayjs.unix 按浏览器时区展示，已禁用/@reboot 特殊文案）。agent 7 新测试（基本字段组合、@ 简写、dom/dow 语义含 dow 0/7、闰年 2/29、整分钟边界与非法 8 形态、Jobs 集成 shape）全绿；web tsc 零新增错误 + build 过。✅ M3 systemd timer 只读列表完成（2026-09-18）：`cron-design.md` D18-D20；弃 `systemctl list-timers` 表格式输出（列随版本变，此前「不做」根因），两段式稳定 API——`list-unit-files --type=timer --no-legend` 首列取 unit 名 + 一条 `show` 全量属性查询（Id= 归属、空行分段、错误段跳过，模板单元 x@.timer show 失败仅列名不致命）；日程取原文不做语义解析（TimersCalendar 的 OnCalendar= 优先，monotonic 逐行 OnBootUSec/OnUnitActiveUSec 拼接）；时间剥尾时区缩写按本机本地时区转 unix 秒（同机同 TZ 墙钟一致，@epoch 微秒直解，n/a/垃圾归 0）；agent cron provider 新 action `timers`（纯读，capability 不动）+ server `GET /api/agents/{id}/cron/timers` 纯转发不审计 + web Cron 页「systemd 定时器」折叠面板（关键字过滤，空/失败不渲染，默认收起）。agent 3 新测试（时间解析 6 异常形态、Id 归属/错误段/多余段、monotonic 拼接不泄漏 next_elapse、无 systemd 报错）+ server 1 新测试（转发与不审计、POST 404）全绿；web tsc 零新增错误 + build 过。✅ M4 多用户 crontab（-u）完成（2026-09-19）：`cron-design.md` D21-D25——「不做」首项立项：服务用户（www-data/postgres 等）的定时任务挂在其本人 crontab 里，-u 是 crontab 原生语义，agent 经 argv 转发纳管，权限边界 = crontab 命令自身（agent 以谁跑就受谁限，非 root 诚实透出 "must be privileged"）。agent：crontab 相关 4 action 加可选 user 参数（白名单 `^[a-z_][a-z0-9_-]{0,31}$` 双端同规则，非法入口即拒不触达 crontab；argv 拼 `-u` 绝不拼 shell），readCrontab/writeViaFile 加 user 维度，cockpit 段按 (user, name) 二元组独立成立、自检/段落逻辑零改动（D25），公共方法保留无参版本零破坏；新 action `users`（getent passwd 优先 NSS 兼容，缺失/失败 fallback /etc/passwd，name/uid/shell 全量不做 nologin 过滤——服务用户恰是 crontab 主人，D22）。server：`GET /cron/users` 纯转发不审计（同 timers 口径），status/jobs/PUT/DELETE 加 `?user=` 校验透传（非法 400 不达 agent），cron_apply/cron_delete 审计 details 记 user（仅显式指定时，缺省留空保历史语义，D23）。web：api 方法加可选 user + getCronUsers，Cron 页「目标用户」AutoComplete（默认当前用户，枚举下拉 name+shell 提示、可手输任意合法名、枚举失败不阻塞），切主机重置、切用户重查，写路径透传 + 成功提示指名用户，timers 不受影响（D24 落地为 AutoComplete 而非 Select——满足手输要求）。agent 8 新测试（校验矩阵/-u argv/Call 层拒入/apply-delete 全链路含外部条目保持/getent//etc/passwd/passwd 解析）+ server 3 新测试（透传与 400/users 不审计/审计带 user 缺省不带）全绿；agent+server 全包 -race 过；web tsc 零新增错误 + build 过。
- **日志聚合查看**：Agent 推送容器/系统日志，WebUI 统一检索（可接 Loki/轻量自研）。✅ M1 完成（2026-09-15）：`docs/guide/logs-design.md`；查询式 pull 路线（agent 不采集不存储，查询时实时执行 journalctl/docker logs，不落库），logs.status/sources/query 三个 RPC + `logs` capability（journalctl 或 docker 至少一个可用）、source 白名单正则 + 1MB 输出截断到行边界 + 30s 超时 + argv 直传不经 shell、grep 统一内存 contains 过滤（双端行为一致）、docker 停止容器有输出不当错误；server `/api/agents/{id}/logs/*` 纯转发 + 双端同规则校验（浏览类不记审计）；Workbench「日志」Tab（类型 Segmented 按可用性禁用 + 源 Select 联动 + tail/since/grep 过滤栏 + 深色等宽视图 ERROR 红/WARN 橙行着色 + grep 命中高亮 + 自动滚底 + 截断提示）。✅ M2 实时尾随完成（2026-09-18）：`docs/guide/logs-design.md` F1-F8——控制面 RPC logs.follow.start/stop（平铺参数 {followId,type,source,tail,grep}）+ 数据面复用 proxy_data/proxy_close（proxyId="logs:<followId>"，与 terminal 前缀同构，协议零新增）；agent journalctl -f / docker logs -f 跟随进程（回填 tail 由命令自带），bufio 行扫描 + grep 同款过滤，会话上限（单会话 4MB/超时 10min/并发 16/同 followId 覆盖杀旧）；server `POST /api/agents/{id}/logs/follow` NDJSON 流（JWT 头认证 fetch streaming，注册表全局 8/每 agent 2，注册先于 CallAgent 防丢回填帧，eof 前排干缓冲防 select 随机丢尾，客户端断开补发 follow.stop）；LogsPanel「实时尾随」开关（AbortController、逐帧追加渲染、尾随中锁定参数、eof reason 状态展示）。剩余：尾随真机验收（journalctl 高频输出/docker 容器停止 reason=exited/10min 超时）。✅ M3 跨机联邦检索完成（2026-09-19）：`logs-design.md` D11-D16——路线决策：联邦扇出检索（server 并行向在线 agent 转发**既有** logs.query 按主机分组返回，agent 零改动零存储，保持 D1/D8 纯转发哲学）替代推送式采集作为 M3 本体，原「跨机检索依赖采集管道先行」假设被打破（排障高频动作是全机 grep 同一条错误而非全量历史索引；离线机器日志本就无法访问；3-10 台日志量撑不起索引收益）；推送采集维持不做（仅当历史趋势/合规留存需求出现再立项），结构化解析维持不做（docker 无对应语义，双轨解析不值）。关键决策：全局端点 POST /api/logs/search 不挂 /agents/ 前缀、tail 收窄 ≤500/机（扇出放大器）、目标=在线且带 logs capability（agents 指定取交集 + skipped 三因归因 offline/no-logs/not-found）、扇出并发 4 + 端点在途 ≤2 超 429 双闸门、单 agent 失败降级不整体 5xx、按 agent 分组不做跨机时间归并（时钟不可信伪精度）、server 镜像 M1 校验 + 浏览不审计（D9 纪律）、web 独立「日志检索」页（source 手填 AutoComplete 回避 N 次 sources 枚举扇出，Workbench 单机 Tab 保留深查/尾随动线）。✅ 实施完成（2026-09-19）：server `api_logs_search.go`（校验/筛选/扇出/降级/双闸门，6 测试）+ web「日志检索」页（tsc 零新增 + build 过）+ todo 收尾（0fb5bb40、157109bd、ca3edb0a）。
- **移动端适配**：当前 Ant Design 响应式基础可用但不保证，关键页面（Dashboard/Monitor）做移动端优化。✅ M1 完成（2026-09-15）：`docs/guide/mobile-design.md`；`Grid.useBreakpoint` 判定 < 768px，mix 顶部菜单窄屏溢出 → 强制切 side + ProLayout 内建抽屉菜单；全局 768px media 块扩充——Modal `max-width: calc(100vw - 16px)` 兜底（一行覆盖全部现存与未来 Modal）、header 文档按钮只留图标、容器/卡片间距收紧、表格 cell padding 8px；Dashboard 统计卡窄屏 2 列 + Agent 表 `scroll.x` + 页头 wrap；Monitor Agent 下拉窄屏自适应宽。Go 侧零改动。剩余（M2 按需）：全站各页 Table 补 scroll.x/列 responsive 隐藏、底部 Tab Bar 独立移动导航、Workbench 触屏交互。
- **组网观测（Overlay）**：ZeroTier/Tailscale/WireGuard/frp 运行态只读观测，跨地域设备一屏可见。✅ 方案设计 + M1 完成（2026-09-17）：`docs/guide/overlay-design.md`（决策 D1-D10）；agent `overlay` capability（四类任一可用）+ `overlay.status` 单 RPC 全量快照（单工具失败各自降级、CLI 5s 超时、peers 200 截断）、**wg dump 私钥/预共享密钥白名单剥离**、frp 分级观测（零配置版本+进程，`COCKPIT_FRPC_ADMIN`/`COCKPIT_FRPS_ADMIN` 配置后追加 admin API 隧道计数）；server `GET /api/agents/{id}/overlay/status` 纯转发不落库不审计；Web `/network` 页（全网总览：前端逐 agent 聚合跨地域节点表，同节点按延迟最低合并；按主机分组详情）。剩余（M2）：ZeroTier Central/Tailscale 云 API 管理面（成员授权/CMDB 对照）、Agent 上报自身虚拟网身份（连接路径语义）、network-monitor 旧 capability 收敛。✅ M2 完成（2026-09-19）：`overlay-design.md`（决策 D11-D20）；**server 直连云管理面**（`internal/overlay` 新包：ZeroTier Central/Tailscale 两 client，Bearer + 15s 超时 + 白名单构造 + `NewXxxWithBase` httptest 注入；凭据 `overlay.zerotier.api_token`/`overlay.tailscale.api_token` + env 优先 ZEROTIER_API_TOKEN/TAILSCALE_API_TOKEN 不落 yaml；TS `tailnet` 可选默认 `-`）；REST `/api/overlay/cloud`（GET 段级语义：未配置段 `configured:false`、失败段段内 `error` 不整体 5xx；ZT 授权/除名、TS 授权/删除四变更端点，ID 白名单正则校验不过不入云、未配置 503 指名引导键、上游错误统一 502 脱 token 摘要；变更记 overlay_authz/overlay_remove 审计，浏览不记）；**managed 对照 server 算**（registry 里 agent `overlay` capability `metadata.identity` 的 ZT node id / TS device id 与云端 member/device id 同键对照，前端零推导）；**agent 身份上报**（detector `extractIdentity`：`zerotier-cli -j info/listnetworks` 取 nodeId+网络地址（CIDR 剥离）、`tailscale status --json` Self 取 id/hostName/dnsName/地址；全失败静默不上报 identity，注册时快照重连刷新）；**network-monitor 旧 capability 删除**（`detector/network.go` 零消费方 grep 验证）；Web `/network` 页 Segmented「运行态观测/云端管理」双视图（云端：ZT 按网络折叠成员表授权 Switch+除名 Popconfirm、TS 设备表授权按钮+删除输设备名 Modal 确认、未纳管徽标+汇总 Alert、未配置 provider 引导卡；观测：agent 面板顶部本机身份 chip）。剩余：真实环境验收（真 token 列表/授权/除名实测、未纳管设备识别）。
- **磁盘健康（SMART）**：物理盘 SMART 只读观测 + 定时巡检告警，数据安全第一道防线。✅ 方案设计 + M1 完成（2026-09-17）：`docs/guide/disk-health-design.md`（决策 D1-D10）；复用 `hardware-monitor` capability（metadata.smart 已探测 smartctl，不新建 detector）+ `hardware-monitor.status` 单 RPC（`lsblk --json` 盘发现过滤 TYPE=disk + 逐盘 `smartctl --json -H -A` **字段白名单**：passed/温度/重映射5/待定197/通电9/NVMe media_errors+percentage_used，绝不透传完整 JSON）；健康三态 passed/failed/unknown（unknown=权限或不支持，不告警），单盘失败不影响其余，raw.value 数字/字符串两形态兼容；server `GET /api/agents/{id}/smart/status` 纯转发不落库不审计 + `GET/PUT /api/smart/config`；smartScanLoop 复刻 drift 巡检（`smart.scan_interval_seconds` 默认 3600，min 300 max 86400，0=关闭），`CheckDiskHealth` 真去重（盘 FAILED → error 级，扇区/介质异常 → warning 级，title 按级别分两种允许升级叠加）；Web `/disk` 页（跨 agent 总览：异常盘置顶+告警条+健康徽标+扇区高亮，按主机 Collapse 详情，自动巡检设置与 Drift 页同模式）。剩余（按需）：NVMe 深度属性、盘序列号关联 Storage 资源、非 root 部署下 sudo 提权读取。
- **DDNS 动态域名解析**：家庭宽带动态 IP 绑定域名，server 巡检比对并写 Cloudflare。✅ 方案设计 + M1 完成（2026-09-17）：`docs/guide/ddns-design.md`（决策 D1-D12）；架构分工：agent 只答出口 IP（`ddns` capability 恒上报，照 file 先例标准库实现；`ddns.ip` RPC 多源兜底——IPv4 api.ipify.org→api-ipv4.ip.sb→ipv4.icanhazip.com→4.ipw.cn，IPv6 同理；`net.ParseIP` + 地址族匹配校验，5s 单源超时），server 持配置与写 DNS（不采连接对端地址——代理/overlay 下不可靠）；`DDNSConfig` 表（agent 绑定/zone/记录名/类型 A|AAAA/LastIP/LastStatus never|ok|failed）；ddnsScanLoop 复刻 drift 巡检（`ddns.scan_interval_seconds` 默认 300，min 60 max 86400，0=关闭），配额保护——同 zone+type 每轮共享一次 ListRecords 分页缓存，仅记录缺失（自动创建，TTL auto 无代理）或 IP 变化（更新保留原 TTL/Proxied）才写 Cloudflare；失败 `CheckDDNS` 真去重告警（warning 级，同记录未读期间只报一次）+ 成功静默自愈清错；REST `/api/ddns` CRUD（ddns_create/update/delete 记审计，巡检自动变更不记）+ `/api/ddns/{id}/check` 立即检查 + `/api/ddns/config` 巡检设置；Web `/dns` 页加 Tabs（记录管理 + DDNS：配置表含状态徽标 Tooltip 错误/当前 IP/绑定主机/最后检查，新建编辑 Modal 含 agent 下拉在线优先，立即检查按钮，自动巡检设置与 Drift/Disk 页同模式；未配置 token 引导卡片两 Tab 共用）。剩余：真实环境验收（Cloudflare token + 家庭侧 agent 多源探测）。
- **ACME 证书自动签发**：Let's Encrypt 证书签发+存储+自动续期，补上反向代理「证书签发联动」等待的缺口。✅ 方案设计 + M1 完成（2026-09-18）：`docs/guide/acme-design.md`（决策 D1-D12）；选型 **lego v4**（`go-acme/lego/v4` MIT，否决 autocert 绑定 http.Server / certmagic 过重）；只做 **DNS-01**（家宽 80/443 不可靠 + 唯一支持泛域名，复用 `dns.cloudflare.api_token` 同一 token，Zone.DNS 权限即可）；server 侧直连（agent 不参与）；`AcmeAccount` 单行表（ECDSA P-256 key 惰性注册持久化——同 key 在 CA 幂等，不触发账户限频）+ `AcmeCert` 表（domains/CA 目录 staging|production 默认 staging 防触限频/三段 PEM 持久化随 server-backup 备份/到期/续期阈值 7-90 默认 30）；`AcmeIssuer` 接口（生产 lego、测试 fake——真流程需真实 CA）；REST `/api/acme`（certs CRUD 白名单视图 PEM 绝不出响应 + issue 同步签发 + download part=key 强制记审计 + account email + config 巡检设置；acme_create/update/delete/issue/download_key 审计，删除不吊销）；acmeScanLoop 复刻巡检（`acme.scan_interval_seconds` 默认 3600，0=关闭；临期 issued 重签、failed 1h 节流重试防撞 CA 失败限频、pending 手动首签）+ `CheckACME` 真去重告警；签发成功联动证书观测表（确定性 ID `acme-{id}`，Labels source=acme，与 probe TLS 探测两层互补）；Web `/acme` 页「证书签发」（导航挂资源分组；配置表/签发/下载/编辑/Modal/巡检设置/账户邮箱/未配置 token 引导）。剩余：真实环境验收（staging 签发跑通 + production 真证书 + 自动续期观察）；M2：签发后推送 agent（proxy 站点联动）。✅ M2 扩 DNSPod/阿里云 provider 完成（2026-09-18）：`acme-design.md` D13——`dns.provider` 三选一（空=cloudflare 向后兼容/dnspod/alidns），**ACME 凭据判定独立于 DNS 管理页**（后者仍 Cloudflare 专属，dns.status 不动，语义分叉避免破坏性变更）；config 加 `dns.provider/dnspod.login_token/alidns.access_key/secret_key`，env 优先注入 DNSPOD_LOGIN_TOKEN/ALIYUN_ACCESS_KEY(_SECRET)；server `AcmeDNSConfig.Ready()` 按 provider 校验并报缺哪个键 + `dnsProvider` 工厂构造 lego provider（alidns 引阿里云 SDK 依赖树）；issue 503 预检按 provider 判定、巡检续期经 Issue 内部自然携带 provider 感知文案；`GET /api/acme/config` 加 `dns: {provider, configured}`（ACME 视角）；web 引导卡片改读该字段按当前 provider 列配置方法，不再依赖 /api/dns/status。✅ M2 扩签发后部署推送到 agent 完成（2026-09-18）：`acme-design.md` D14（proxy 站点联动）——`AcmeCert` 一对一部署目标绑定（DeployAgentID/CertPath/KeyPath + LastDeployAt/LastDeployError）；`runACMEIssue` 成功路径统一触发自动推送（手动 issue 与巡检续期共用；失败只记 LastDeployError 不回滚签发状态、不产告警避免巡检风暴）；通道复用 `file.write`（agent 加可选 mode 参数白名单 {0600,0644}，证书 0644/私钥 0600 覆盖写补 chmod）；`POST /api/acme/certs/{id}/deploy` 手动立即部署 + acme_deploy 审计（不含 PEM）；web 配置 Modal 部署区（在线 agent 下拉 + 按主域名填默认 `/etc/cockpit/certs/` 路径）+ 部署状态列 + /proxy https 表单路径约定提示。剩余：真实环境验收（签发→自动部署→nginx reload 后浏览器信任链、续期后 agent 侧文件更新）。

- **systemd 服务管理**：systemd 主机上 `*.service` unit 观测 + 运行时启停/自启管理，NAS 场景前置能力。✅ 方案设计 + M1 完成（2026-09-18）：`docs/guide/service-design.md`（决策 D1-D8）；Agent service provider 复用 Commander 抽象 argv 直调 systemctl 不经 shell；`systemd` capability 探测 = LookPath systemctl + `/run/systemd/system` 存在（排除容器）；`service.list` = list-units（运行态）与 list-unit-files（安装项+自启态）合并（transient unit 保留、未加载项 activeState 兜底 inactive），`service.status` = is-system-running（degraded 退出码非 0 但 stdout 有值，以 stdout 为准）+ active/failed/enabled 计数，`service.action` = 六动词白名单 start/stop/restart/reload/enable/disable + unit 名白名单 `^[A-Za-z0-9@._+-]+\.service$`（双端同规则校验，非法请求 server 直接 400 不消耗 agent 往返）；server `/api/agents/{id}/services`（GET status/list 纯转发浏览不审计 + POST `/{unit}/{action}` 记 service_action 审计）纯转发不落库；Web `/services` 页「服务管理」（agent 按 systemd capability 过滤/系统状态卡 degraded 橙徽标/服务表 failed→active 置顶排序/启停重载自启按钮按状态显隐/行级 loading/30s 轮询/失败 Alert 展示 systemctl 原始错误）。剩余：真实环境验收（systemd 主机列表含未加载 unit、restart/enable 实测、非 root agent 报错透传）；M2 候选：mask/unmask、journal 日志跳转（logs provider 已有底层）、unit 文件编辑。✅ M2 跨平台统一 + Windows SCM 后端完成（2026-09-18）：`service-design.md` D9（决策 D9.1-D9.8）——capability `systemd`→`service`（version 2，metadata.backend 区分 systemd/windows-scm，agent/server/前端同仓同发无兼容包袱）；Windows 侧 SCM 内置恒可用直接上报 capability，providers.go 按 backend 构造对应 provider；实现三层组织（`service_windows_model.go` 无 tag 纯函数层：WinService DTO→ServiceUnit 映射 + Windows 服务名白名单 `^[A-Za-z0-9_.\- ]{1,256}$` 拒 `.`/`..`，Linux CI 可测；`service_windows_scm.go` //go:build windows 真采集：x/sys/windows/svc/mgr 直连 SCM 本地 RPC——`ListServices()` 枚举 + 逐个 Query/Config，单服务读取失败降级跳过全失败才报错；`service_windows_stub.go` 非 Windows 防御层显式报错）；字段映射（Running→active/Stopped→inactive/Start|Continue Pending→activating/Stop Pending→deactivating/Paused→paused 不扭曲语义 + StartType Automatic 含 Delayed→enabled/Manual|Disabled→disabled，LoadState 恒 loaded、Preset 空）；动作映射（start/stop 幂等化先查询、restart=stop+等待 Stopped 30s 超时+start、enable/disable=UpdateConfig 改 StartType 其余字段原值回写、reload 不支持显式报错）；server 端 unit 名校验放宽为双后端并集（仍纯白名单无 `/` `\` 空字节 `..`，后端专属严格校验 agent 侧兜底）；Web 服务页按 `service` capability 过滤 + windows-scm 主机隐藏重载按钮与系统状态项。三平台 build 全过（GOOS=windows/darwin/linux）。剩余：Windows 真机验收（列表/启停/自启切换/restart 等待实测、无权限服务报错透传、reload 按钮确认隐藏）；macOS launchd 留 M3 单独设计（LaunchAgent/LaunchDaemon 双域语义差异大）。✅ M3 macOS launchd 后端完成（2026-09-18）：`service-design.md` D10——只管 system 域 LaunchDaemons（`/Library/LaunchDaemons` + `/System/Library/LaunchDaemons`；LaunchAgents 用户域不做），capability 按 GOOS=darwin 恒注册 backend=launchd；观测 = `launchctl list`（PID/Status/Label 运行态）+ plist 目录扫描（安装项 + RunAtLoad/Disabled）按 Label 合并（transient 内置服务保留、坏 plist 跳过），**轻量 encoding/xml 解析只取 Label/RunAtLoad/Disabled 三键零第三方依赖**（array/dict 复杂值整树 Skip）；映射（PID 在→active、退出码非 0→failed、RunAtLoad&&!Disabled→enabled、Description=plist 路径）；动作（`system/<label>` 目标：start=kickstart 失败回退 bootstrap、stop=bootout（launchd 无 stop，卸载即停 plist 保留）、restart=kickstart -k、enable/disable 写 disabled DB（不卸载运行实例）、reload 不支持）；label 独立白名单 `^[A-Za-z0-9_][A-Za-z0-9._-]{0,254}$`；三层组织同 Windows（无 tag 模型层 Linux CI 可测 + darwin 真执行 + stub 防御）；Web backend 显隐泛化（仅 systemd 显示重载与系统状态项，launchd 文案说明卸载语义）。服务管理三后端（systemd/Windows SCM/macOS launchd）全部交付。剩余：macOS 真机验收（列表/启停/自启切换、kickstart 回退 bootstrap、无权限报错透传）。✅ M2 扩 mask/unmask 完成（2026-09-18）：systemd 白名单扩到 8 动词（mask 链接 /dev/null 强封禁防误启/禁用伪装服务，不影响正在运行的服务；unmask 还原）——agent serviceActions + server validateServiceAction 双端同规则放行；Web 操作列 systemd 后端显屏蔽/解屏蔽切换（masked 行隐藏启动并显解屏蔽带语义提示，自启列 masked 红标已有）。仅 systemd 后端支持（Windows SCM/launchd 无此概念）。✅ M2 扩 journal 日志跳转完成（2026-09-18）：`service-design.md` D11——服务表操作列「日志」按钮（仅 systemd 后端）开 Drawer 内嵌 Workbench 同款 LogsPanel，新 prop initialSource 锁定当前 unit 为日志源（未加载服务不在 logs.sources 列表但 journalctl -u 仍可查历史日志，派生逻辑对该值无条件优先不回落）+ status 就绪后自动首查；零新端点零 Go 改动（复用 /logs/query 转发，浏览类不记审计）。✅ M2 扩 daemon-reload 完成（2026-09-18）：`service-design.md` D12——agent `service.daemon-reload` RPC（systemctl daemon-reload 全局操作不针对 unit，仅 systemd provider）；server `POST /agents/{id}/services/daemon-reload`（一段 sub 在两段 unit/action 解析前特判，记 service_action 审计）；web 工具栏「重载配置」按钮（仅 isSystemd，Popconfirm 确认后刷新列表）。M2 剩余：unit 文件编辑（改动面大待单独设计）。✅ M2 扩 unit 文件查看与编辑完成（2026-09-18）：`service-design.md` D13——读 `systemctl cat` 有效全文 + FragmentPath；写路径只收 unit 名（写哪由 FragmentPath 决定，无穿越面），语义对齐 `systemctl edit --full`（包管文件 /usr/lib 先复制 /etc/systemd/system 覆盖位再改，升级不丢），临时文件+rename 原子写 0644、content 双端同限 256KB、保存捆绑 daemon-reload；server `GET/PUT /agents/{id}/services/{unit}/file`（GET 浏览不审计、PUT 记 service_action）；web「文件」按钮（仅 isSystemd）Modal 等宽编辑器。**服务管理 M2 全部候选（mask/unmask、journal 日志跳转、daemon-reload、unit 文件编辑）交付完毕**，剩余均为真机验收项。

- **防火墙管理**：iptables/nftables 规则可视化（仅在有明确需求时推进）。

- **NAS 系统对接**：存储池/挂载/共享观测与异常告警。✅ 方案设计 + M1 完成（2026-09-18）：`docs/guide/nas-design.md`（决策 D1-D9）；**多 Provider 统一快照架构**（NasSnapshot = available/source/pools/mounts/shares，M1 linux 源 + M2 dsm/truenas/omv 网络 API 预留——agent 内网 HTTP，凭据 `COCKPIT_NAS_TARGETS` 环境变量 JSON 不落库）；Agent nas provider 纯读不写：mdadm（/proc/mdstat，[U_] 缺 U→degraded、resync 进度行）、ZFS（zpool list -H -p）、LVM（vgs）、df（真实块设备 ≥1GB、bind mount 去重）、SMB（testparm -s）/NFS（exportfs -v）；单数据源失败只降级该段不整体报错，`nas` capability = mdstat 存在或任一工具可用；server nasScanLoop（smart 同构：`nas.scan_interval_seconds` 300-86400 默认 1800、0=关闭；`nas.usage_warn_percent` 50-99 默认 80）消费 nas.status——池 failed→error/degraded/resync→warning 告警（unknown 观测缺失不告警）、挂载超阈值→warning，createAlertIfNotExists 真去重、恢复=不再新增（静默自愈），不落库（NAS 本机为唯一事实源）；`GET /api/agents/{id}/nas/status` 纯转发 + `GET/PUT /api/nas/config`（巡检配置类不记审计）；Web `/nas` 页「存储池」（跨 agent 池/挂载/共享总览，异常置顶告警条、挂载超阈值红高亮、按主机 Collapse 三段详情、巡检设置行）。剩余：真实环境验收（mdadm 降级演练、ZFS/SMB/NFS 主机实测）；M2：truenas/omv provider。✅ M2-TrueNAS 完成（2026-09-18）：REST v2.0 Basic Auth + flexInt 字符串数字兼容 + 顶层 dataset 防噪声（见上方 D3c 细节）。✅ M2-OMV 完成（2026-09-18）：`nas-design.md` D3d——JSON-RPC over `POST rpc.php`（body `{service,method,params}`、响应 `{response,error}` 包装），认证走 **`X-Openmediavault-Sessionid` 请求头**（源码核实：非 Cookie 非 body）；`session.login` params 对象 → `response.sessionid`（2FA 账号 challengeRequired 等同失败降级）、`session.logout` best-effort；`filesystemmgmt.enumerateFilesystems` → Mounts（mounted+mountpoint 过滤 swap/未挂载；**容量是 binary_format 字符串** `"1.50 GiB"`，parseBinarySize 按 IEC 单位换算十进制 GB，非法/负值→0）、`smb/nfs.getShareList`（{total,data} 包装）→ Shares（SMB 仅 enable=true、Hosts=hostsallow/client；**Path 留空**——sharedfolder 位置需两次关联查询且导出根版本间有差异，不伪造）；**Pools 置空**（OMV 无统一存储池概念，底层 mdadm 由 OS 层 agent 本地覆盖）；凭据/Host/降级纪律同 DSM/TrueNAS，消费端零改动；假 OMV httptest（sessionid 头校验 + service/method 分发 + logout 计数）全流程/降级/解析表驱动。NAS M2 三 provider（DSM/TrueNAS/OMV）全部交付。✅ 跳板探测边界修复（2026-09-18）：`DetectNas()` 增加「`COCKPIT_NAS_TARGETS` 解析出有效条目即注册」分支（无本地存储工具的 Windows/macOS 主机也可当跳板观测网络 NAS；非法 JSON/缺字段条目不触发，type 未实现也注册——快照阶段才忽略，provider 实现随 agent 升级自动生效）。剩余：真实环境验收（mdadm 降级演练、ZFS/SMB/NFS 主机实测、DSM 6/7 与 TrueNAS CORE/SCALE 与 OMV 各版本真机 API 验收）。

✅ M2-DSM、M2-TrueNAS 完成（2026-09-18）：`docs/guide/nas-design.md` D3b；统一模型 NasPool/NasMount/NasShare 加 `Host` 来源设备字段（本地观测空、网络 NAS 填 target 名，Source 逗号 join 如 `linux,dsm`）；Agent DSM provider（`COCKPIT_NAS_TARGETS` 环境变量 JSON：name/type/addr/username/password/insecureTls，缺字段/缺协议条目丢弃、未实现 type 保留由快照阶段忽略；DSM webapi 会话 = SYNO.API.Auth v6 login/logout + SYNO.Storage.CGI.Storage load_info（pools+volumes）+ SYNO.FileStation.List list_share，url.Values 编码凭据（密码含特殊字符安全）、单请求 5s、insecureTls 跳过自签校验、登出 best-effort）；池状态映射 Normal→healthy/Degrade→degraded/Crash→failed/Resync,Migrat→resync（Kind=dsm）、volumes→Mounts（容量告警语义复用）、shares 统一 smb（DSM 共享协议粒度内部 API 拿不到）；单 target 失败降级不拖垮快照，凭据绝不进快照/日志/错误消息；server 告警 title「主机」位 Host 优先显示设备名（跨设备去重天然隔离）；Web 总览主机列「agent · 设备名」。测试：httptest 假 DSM 全流程/坏 target 降级/自签 TLS/状态映射表/targets 解析规则/凭据不泄露。剩余：DSM 真机验收（DSM 6/7 版本差异字段）。

✅ M2-TrueNAS 完成（2026-09-18）：`docs/guide/nas-design.md` D3c；TrueNAS REST v2.0 + **Basic Auth**（无会话状态，比 DSM sid 流程简单）；端点 = `GET /pool`（ZFS 状态原词 ONLINE/DEGRADED/FAULTED/OFFLINE/UNAVAIL/REMOVED + `scan.state=SCANNING`（resilver/scrub）→resync 优先 + topology.data 成员盘）、`GET /dataset?limit=0` 顶层 FILESYSTEM 过滤（每池一个挂载 `/mnt/tank`，子数据集配额防噪声，used+available=总量）、`GET /sharing/smb`、`GET /sharing/nfs`（paths 逐条展开、networks+hosts join）；`flexInt` 兼容容量字段 number/字符串数字（TrueNAS 版本差异）；通用化重构：`nasHTTPClientFactory` + `nasHTTPGet`（headers 参数）DSM/TrueNAS 共用，`snapshotFromTargets` switch 分发，Source 记 target 类型（`linux,truenas`）；Kind=truenas，凭据/降级/Host 纪律与 DSM 同，server/web 消费端仅加 kind 标签零逻辑改动。测试：假 REST 全流程（五态池/字符串数字容量/顶层过滤/NFS 展开/凭据不泄露）/坏 target 401 降级/状态映射表/flexInt。剩余：TrueNAS 真机验收（CORE/SCALE 字段差异）；M2：omv provider。

## 暂不建议做的事

- 暂不引入 Kubernetes 风格 CRD 全量模型，除非先明确 v2 inventory 迁移方案。
- 暂不替换前端 UI 框架。
- 暂不把 SQLite 换成 PostgreSQL，个人 homelab 场景 SQLite 足够。
- 暂不继续扩展更多远控协议，先把现有 Terminal/VNC/Desktop 权限和会话生命周期收稳。
- 暂不做大规模目录重命名，先通过小模块抽离建立边界。
