# P1 方案设计：应用部署（Compose Stack）

> 语言：**中文** ｜ [English](./stack-deploy-design.en.md)

> 2026-09-14。P1 首个功能的设计方案。参考来源与对比结论见[参考项目对比与借鉴](./reference-projects.md)（Komodo「Git-to-deploy」+ Dockge「compose-file-first」）。本文只做设计决策与接口定义，实现按「分期落地」推进。

## 背景与目标

Cockpit 当前已能管理单容器生命周期（`/docker` 页面：启停/日志/删除），但没有「应用」层——一个典型的个人服务（如 `nginx + certbot + redis`）是一个 Compose 项目，用户需要在主机上手动 `docker compose up`。

本功能在 Cockpit 中补齐这一层：

- 在 Web 上创建、编辑、部署 Compose Stack（up/down/查看服务状态/看部署日志）
- **compose 文件本身是真相源**（Dockge 的 compose-file-first），UI 是视图
- 为 P2 的 Git-to-deploy（Komodo 式：栈文件存 Git 仓库 + 触发部署）预留演进路径

**非目标（P1 不做）**：镜像构建流水线、Git 仓库同步、多机编排模板市场、Kubernetes。

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | Compose 执行方式 | Agent shell out 到 `docker compose` CLI | Komodo/Dockge 同款做法。Go 生态没有可嵌入的 compose 引擎库（compose-go 只是模型/校验库）；CLI 天然支持 build/env/profiles/healthcheck 全语义 |
| D2 | 文件真相源 | Agent 主机本地目录 `COCKPIT_STACKS_DIR`（默认 `/var/lib/cockpit/stacks`） | compose-file-first；server 数据库只存索引缓存。P2 接 Git 时该目录升级为 Git worktree |
| D3 | RPC Provider 归属 | 新建独立 `stack` provider，不扩展 `docker` provider | stack 需要文件系统写 + compose CLI，职责不同；provider 按 capability 独立注册降级，不影响纯容器管理 |
| D4 | 长耗时部署 | 异步任务模型：`up/down` 立即返回 `task_id`，日志/状态轮询获取 | server `CallAgent` 固定 30s 响应超时（`server.go`），镜像拉取必然超它；不改协议、不加推送通道的最稳妥解 |
| D5 | 前端编辑器 | P1 用等宽 textarea，不引入 Monaco/CodeMirror | 与此前「移除 echarts-for-react 优化分包」的包体策略一致；编辑器升级独立成项 |

## 总体架构

复用现有三段式链路，不引入新协议消息：

```
Web (Stacks 页面)
  │  REST /api/stacks/agents/{agentId}/...
  ▼
Server  api_stacks.go ── CallAgent RPC（既有 30s 通道）
  │                        │
  │  storage.stacks        ▼
  │  (索引缓存+审计)    Agent  rpc.Handler ── stack provider
  │                                        │ ├─ docker.Client（状态查询，label 过滤）
  │                                        └ ─ docker compose CLI（up/down/logs）
  ▼                                        ▼
审计日志                              <stacks_dir>/<name>/compose.yml  ← 真相源
```

## Agent 侧设计

### Provider 注册

- 挂在既有 `docker-api` capability 下：`setupProviders`（`internal/agent/providers.go`）检测到该 capability 且 `docker compose version` 可用时，注册 `stack` provider
- compose CLI 不可用则不注册 → server 对该 agent 返回 502（agent 回 `unknown provider: stack`）→ UI 报错提示，容器管理不受影响

### 文件布局（compose-file-first）

```
/var/lib/cockpit/stacks/        # COCKPIT_STACKS_DIR，可覆盖
└── <stack-name>/
    ├── compose.yml             # 真相源，UI 可编辑
    ├── .env                    # 可选，UI 可编辑（敏感）
    └── .cockpit/
        └── last-task.json      # 最近一次任务元数据（action/status/finishedAt）
```

约束：

- `<stack-name>` 必须匹配 `^[a-z0-9][a-z0-9_-]{0,63}$`（同时满足 compose project name 规则），所有文件操作 jail 在 `COCKPIT_STACKS_DIR` 内，杜绝路径穿越
- 单文件上限 512KB；`.cockpit/` 为保留目录，list 时跳过
- 目录即 stack：外部手动放入的合法目录同样被识别（Dockge 行为），server 端不建「先注册后使用」的流程

### RPC 动作表

方法名遵循既有 `<provider>.<action>` 格式（`internal/agent/rpc/router.go`）：

| 方法 | 参数 | 返回 | 同步/异步 |
|------|------|------|-----------|
| `stack.list` | — | `[{name, services: [{name, image, state, status, ports?: [{hostPort, containerPort, proto}]}], running, total}]`（ports 为已发布端口，P2 ③） | 同步 |
| `stack.status` | `name` | 单个 stack 的服务/容器明细（含容器 ID，可跳转 `/docker`） | 同步 |
| `stack.file.get` | `name` | `{compose, env, modifiedAt}` | 同步 |
| `stack.file.save` | `name, compose, env?` | 保存前执行 `docker compose config -q` 校验，失败返回错误不落盘 | 同步 |
| `stack.up` | `name` | `{task_id}`，等价 `docker compose up -d --remove-orphans` | 异步 |
| `stack.down` | `name` | `{task_id}`，等价 `docker compose down` | 异步 |
| `stack.logs` | `name, service?, tail?` | compose logs 文本 | 同步 |
| `stack.task.get` | `task_id` | `{stack, action, status: running/success/failed, log, startedAt, finishedAt}` | 同步 |
| `stack.remove` | `name` | 先 `down`（已停则跳过），成功后删目录 | 异步 |
| `stack.restart` | `name` | `{task_id}`，等价 `docker compose restart`（M1.5） | 异步 |
| `stack.pull` | `name` | `{task_id}`，等价 `docker compose pull`（M1.5） | 异步 |
| `stack.info` | — | `{dir, dirWritable, dirError?, composeVersion?}`，目录可写性探针 + compose 版本（M1.5） | 同步 |
| `stack.git.status` | — | `{enabled, reason?, remote?, branch?, head?, remoteHead?, ahead, behind, changes?: [{code, path}]}`，stacks 仓库接入状态 + drift（P2 ①） | 同步 |
| `stack.git.pull` | `remote?, branch?` | 同 `git.status` 的接入后快照；首次调用即接入（P2 ①） | 同步 |
| `stack.git.deploy` | `branch?` | `{head, deployed: [{stack, taskId}], skipped: [{stack, reason}]}`：拉取 + 部署改动 stack（P2 ②，webhook 专用；语义见[P2 ② 设计](#p2--设计2026-10-12)） | 同步（含 fetch，慢仓库受 CallAgent 30s 上限约束） |
| `stack.task.follow` | `taskId, followId` | `{followId}`：订阅任务日志增量，经 proxy_data（`proxyId="stacktask:<followId>"`）回推，终态经 proxy_close（P2 ②） | 异步会话 |
| `stack.task.follow.stop` | `followId` | 停止一个 follow 会话（P2 ②） | 同步 |

实现要点：

- **任务模型**：provider 内存维护 `map[taskID]*Task` + 每 stack 一把互斥锁；同 stack 并发 `up/down` 时第二个请求直接报 `stack busy` 错误（server 转 409）。任务日志只写入内存环形缓冲（取最近 ~200KB），不落盘
- **状态查询零改动**：复用 `docker.Client.ListContainers(true)`（`internal/docker/client.go`），按 `Labels["com.docker.compose.project"] == name` 客户端侧过滤，不动 DockerAPI 接口签名；规模大后再加 Filters 参数
- **`compose.yml` 与 `compose.yaml`**：读写优先 `compose.yml`，兼容识别 `compose.yaml`/`docker-compose.yml`/`docker-compose.yaml`（只读兼容，保存时统一规范为 `compose.yml`）

## Server 侧设计

### REST API

新文件 `internal/server/api_stacks.go`，模式复刻 `api_docker.go`（JWT 中间件 + 解析路径 + `CallAgent` + 错误码映射）：

| 路由 | RPC | 说明 |
|------|-----|------|
| `GET /api/stacks` | — | 聚合视图：遍历具备 docker-api 的在线 agent 并发拉取 `stack.list`，合并 DB 缓存（离线 agent 显示缓存状态） |
| `GET /api/stacks/agents/{agentId}` | `stack.list` | 单 agent 的 stack 列表 |
| `GET /api/stacks/agents/{agentId}/{name}` | `stack.status` | stack 详情 |
| `GET /api/stacks/agents/{agentId}/{name}/compose` | `stack.file.get` | 读文件 |
| `PUT /api/stacks/agents/{agentId}/{name}/compose` | `stack.file.save` | 写文件（body: `{compose, env}`） |
| `POST /api/stacks/agents/{agentId}/{name}/{action}` | `stack.up` / `stack.down` / `stack.restart` / `stack.pull` | action ∈ {up, down, restart, pull}，返回 `{task_id}` |
| `GET /api/stacks/agents/{agentId}/{name}/history` | — | 部署历史（server 侧 `stack_deployments` 表），`?limit=`（默认 50，上限 200）（M1.5） |
| `GET /api/stacks/agents/{agentId}/{name}/logs` | `stack.logs` | `?service=&tail=` |
| `GET /api/stacks/agents/{agentId}/tasks/{taskId}` | `stack.task.get` | 任务轮询 |
| `POST /api/stacks/agents/{agentId}/tasks/{taskId}/follow` | `stack.task.follow` | 任务日志 NDJSON 流（`{"data":...}` 增量 / `{"eof":true,"reason":终态}`）；上限全局 8 / 单 agent 4（P2 ②；浏览类不记审计，同日志尾随纪律） |
| `POST /api/stacks/webhook` | `stack.git.deploy` | forge 回调入口：头 `X-Cockpit-Token` 常量时间比对配置 `stacks.webhook_token`；body 可选 `{agentId, branch}`，缺省扇出全部在线 agent；响应 `{triggered:[...]}` 即返回、部署异步（审计 username=webhook）（P2 ②；不走认证中间件，令牌空 = 端点 404 关闭） |
| `GET /api/stacks/agents/{agentId}/git` | `stack.git.status` | stacks 仓库接入状态 + drift（P2 ①；不记审计） |
| `POST /api/stacks/agents/{agentId}/git/pull` | `stack.git.pull` | 拉取远端（首次即接入；body 可选 `{remote, branch}`；审计 stack_git_pull）（P2 ①） |
| `DELETE /api/stacks/agents/{agentId}/{name}` | `stack.remove` | 删 stack（body 仅 `{name}`） |

约定：404 = agent 不存在/无 docker 能力；502 = 有 docker 能力但 stack provider 未注册（compose CLI 不可用，agent 回 `unknown provider: stack`）；409 = stack busy；504 沿用 `api_docker.go` 的 RPC 错误映射。P2 ① 追加：409 亦用于 git pull 冲突（本地改动/分叉被 `--ff-only` 拒绝）；agent 报「remote branch not found」（空仓库/分支写错）→ 404。

### 存储

`storage` 新增 `stacks` 表（索引缓存，非真相源）：

```
Stack{ ID, AgentID, Name, Running, Total, LastAction, LastStatus, LastDeployedAt, UpdatedAt }
```

- agent 上报 list 时 upsert；agent 离线时 `GET /api/stacks` 用它渲染灰态
- 部署历史表（每次 up/down 一条记录）推迟到 M1.5，P1 只靠审计日志追溯
- inventory 联动（P2 ③）不新建表：stack 已发布端口收编为 `services` 表台账行（`labels.source=stack`，确定性 ID `stackport-*`），probe 复用既有服务探测

### 审计

复用 `audit.Logger` 新增事件：`stack_create`（首次 save）、`stack_update`（save）、`stack_up`、`stack_down`、`stack_remove`，detail 记 name + agent + 任务结果。**遵守既有约束：`.env` 内容与 deploy 输出一律不入审计日志**，只记事件与元数据。

## Web 侧设计

新增页面 `web/src/pages/Stacks/`，导航「应用部署」，路由 `/stacks`；`/docker` 页保持单容器视角不动，stack 详情中容器行提供跳转链接。

- **列表页**：按 agent 分组的卡片/表格（名称、running/total、最后部署时间、状态 Tag）；顶部「新建 Stack」按钮（选 agent + 名称 + 模板：空/Nginx 示例）
- **详情页**（抽屉或子路由）三个 Tab：
  1. **服务**：服务/容器状态表 + 「启动/停止」主按钮（触发 up/down → 轮询 task → 完成刷新）
  2. **compose.yml**：等宽 textarea 编辑 + 保存（后端校验失败时展示 `docker compose config` 错误输出）；`.env` 单独折叠区，明文展示（内容不落审计）
  3. **部署日志**：任务运行中 2s 轮询 `tasks/{id}`，终端风格深色块（复用 `/docker` 日志 Modal 的样式）
- agent 离线/compose 不可用：列表置灰，操作禁用并提示原因

## 安全设计

1. **路径安全**：stack 名称白名单正则；文件读写统一走 `filepath.Dir` 清洗 + 前缀校验， jail 在 stacks 根目录内
2. **能力最小化**：stack provider 仅在 docker-api + compose CLI 双条件满足时注册；无 `docker-api` capability 的 agent 完全无此攻击面
3. **审计闭环**：所有写操作（save/up/down/remove）过 JWT 中间件 + 审计事件；.env 内容不落审计
4. **资源限制**：compose 文件 512KB 上限；任务日志 200KB 环形缓冲；provider 级并发任务数上限（默认 4）
5. **文件权限**：stacks 根目录建议 `0700`，文档给出 systemd 单元示例（`getting-started` 后续补充）

## 分期落地

### M1 —— P1 本体（本次立项范围）

- [x] Agent：`internal/agent/rpc/stack_provider.go`（list/status/file/task/up/down/logs/remove + compose CLI 检测）
- [x] Agent：providers.go 注册逻辑 + 单测（fake 脚本模拟 compose CLI）
- [x] Server：`api_stacks.go` + storage stacks 表 + 审计事件
- [x] Web：Stacks 页面（列表/详情/编辑/部署/日志轮询）
- [x] 验收（本机端到端，2026-09-14）：真实 server/agent 二进制 + fake Docker Engine（`scripts/local-acceptance/`），32 项断言全部通过且可重复，覆盖「创建 → 校验失败不落盘 → up（并发 409）→ 状态联动 → logs → down → 删除 → 断连灰态 → 审计闭环 + .env 不泄漏」，记录见下文[验收记录](#验收记录)
- [x] 验收（真实 Docker 测试机，2026-09-28）：镜像真实拉取、容器健康/端口/重启策略、compose 完整语义（env/volumes/自定义网络/depends_on/pull/profiles/healthcheck）、`COCKPIT_STACKS_DIR` 0700 实践——见[验收记录](#验收记录)

### M1.5 —— 体验补齐（2026-09-14 完成）

- [x] 部署历史表 + 详情页时间线：`stack_deployments` 表（server 侧记录），启动类动作下发时插 running、后台 goroutine 2s 轮询任务终态回填（agent 离线不终止，重连后继续拿真实终态；15min 超时兜底标 failed）；详情页「历史」Tab 时间线
- [x] 模板库扩充（空/Nginx/WordPress/Nginx Proxy Manager/Uptime Kuma）+ import（新建弹窗选模板填充或直接粘贴 compose.yml，保存即建）
- [x] `restart` / `pull` 动作（异步任务，与 up/down 同一 launchTask 通道 + 审计 stack_restart/stack_pull）；多文件 compose（`include:`）评估结论：当前单文件语义够用，等 P2 Git 仓库场景出现真实多文件需求再议
- [x] stacks 目录权限/位置的自检提示：`stack.info` RPC（目录可写性探针 + compose 版本），列表接口透传 `agentInfo`，目录异常时列表页顶部 Alert

### P2 —— Git-to-deploy（Komodo 式升级）

- [x] stacks 目录升级为 Git worktree：远端仓库拉取 → drift 提示（本地改动 vs 仓库）（2026-10-12，见下文[Git-to-deploy ① 设计](#p2--git-to-deploy-设计)）
- [x] webhook 触发部署；部署日志流式推送（复用 proxy data 通道，替代轮询）（2026-10-12，见下文[P2 ② 设计](#p2--设计2026-10-12)）
- [x] inventory 联动：stack 与 CMDB 服务条目关联，probe 直接探测 stack 暴露的端口（2026-10-13，见下文[P2 ③ 设计](#p2--设计2026-10-13)）

#### P2 ① Git-to-deploy 设计（2026-10-12）

**模型**：stacks 根目录整体作为一个 git 仓库接入一个远端（不是每 stack 一个仓库）——个人服务场景 stacks 通常就是单一仓库（Komodo/Dockge 同款粒度），per-stack 仓库等真实需求出现再议。

**语义**（决策记录见 decision-log D-2026-10-12-5）：

- **接入**（目录还不是仓库）：`git init` → `symbolic-ref HEAD refs/heads/<branch>`（`init -b` 需 git≥2.28，symbolic-ref 全版本可用）→ `remote add origin` → `fetch` → `checkout -B <branch> origin/<branch>`。**要求远端分支已存在**（空仓库先在别处推首版）；远端文件与既有本地文件重名时 git 拒绝覆盖 untracked，不静默吞掉本地文件
- **已接入再拉**：`fetch --prune` → `merge --ff-only origin/<branch>`。不自动 commit、不代做 rebase/merge 决策；本地有提交分叉或 tracked 文件被改时 git 拒绝合并，错误原文透传给 UI（409 conflict）——那就是 drift 提示本身
- **drift = 工作区相对 HEAD 的改动**（`status --porcelain -uall`）：UI 编辑 compose（stack.file.save）天然是 drift——compose-file-first 原则下这是特性不是缺陷，UI 提示而不阻止；`.cockpit/`、`.git/` 路径在代码里过滤（不下发 `.gitignore`，避免污染用户仓库）
- **状态查询不联网**：`stack.git.status` 只读本地引用（`behind/ahead` 反映上次 fetch 时点），拉取动作本身承担 fetch——状态面永不因远端挂起而卡 UI

**安全**：

- remote URL 拒绝 `::`（git 传输助手语法，`ext::sh -c ...` 可执行任意命令）、前导 `-`（防被当 git 选项）、控制字符；分支名约束 `^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$` 且拒绝 `..`
- URL 内 userinfo 凭据（`user:pass@`）在任何回显（错误消息/审计/状态）前统一抹为 `***`；凭据本身由 agent 主机的 git 凭据助手管理，server 不存储
- 远端 URL 持久在 agent 侧 `.git/config`（server 不建表）；写操作（pull）记审计 `stack_git_pull`，状态查询不记

**UI（Stacks 页 GitBar）**：按 agent 一条告警行——未接入=低调 info +「接入」入口；无 git=warning；drift=warning 列改动文件（超出 5 条折叠）；behind>0=info +「拉取更新」。老 agent 不认识新 RPC（502）时静默跳过。

#### P2 ② 设计（2026-10-12）

**webhook 触发部署**（决策记录见 decision-log D-2026-10-12-6）：

- **入口**：`POST /api/stacks/webhook`（精确路由，先于 `/api/stacks/` 认证前缀注册，ServeMux 最长匹配胜出）。forge 推送回调没有登录态，token 即凭据：头 `X-Cockpit-Token` 与配置 `stacks.webhook_token` 常量时间比对；**令牌空 = 端点 404 关闭（默认关，显式配置开启，同 notification 红线口径）**
- **语义**：只触发「拉取已接入 origin + 部署改动 stack」，**不接受 `remote` 参数**（请求里的 remote 一律剥离——webhook 不能重定向部署源）；body 可选 `{agentId, branch}`：指定单 agent，缺省扇出全部心跳在线 agent（未接入仓库的 agent RPC 报错，仅落日志）
- **执行**：异步——响应即返回 `{triggered:[...]}`，每 agent 一个 goroutine 跑 `stack.git.deploy`；成功落审计（`username=webhook`，action `stack_webhook_deploy`）+ 逐 stack 写部署历史（映射为 up 跟踪终态）；RPC 失败只落 server 日志
- **部署范围**（`stack.git.deploy`）：接入前无 HEAD → 部署仓库全部 stack；HEAD 前移 → `git diff --name-only old new` 的顶层目录；HEAD 未动 → 不部署。目标 stack 任务执行中（锁占用）记 `skipped`（原因经凭据抹除），不阻塞其余 stack
- **drift 保护**：沿用 git 自身拒绝——本地改动/分叉使 `merge --ff-only` 失败时整个 deploy 报错（不自动 stash、不代提交，同 D-2026-10-12-5 重开条件的评审结论）

**部署日志流式推送**（架构同 logs-design.md F4 尾随）：

- agent 侧 `stack.task.follow`：复用日志尾随同款 proxy_data 通道，`proxyId="stacktask:<followId>"`；pump 每 300ms 取任务日志增量推送，任务到终态经 proxy_close 回传 reason；agent 侧活跃 follow 上限 8
- server 侧 `POST /api/stacks/agents/{agentId}/tasks/{taskId}/follow`：NDJSON 流（`{"data":...}` 增量 / `{"eof":true,"reason":终态}`），注册表配额全局 8 / 单 agent 4，帧缓冲 256 满则丢帧（不堵 agent WS 读循环）；客户端断开由 `r.Context().Done()` 感知并补发 `stack.task.follow.stop`；浏览类不记审计
- web 侧 StackDetail：任务运行期打开 follow 替代 2s 轮询，`eof` 后清实时区 + 拉一次终态；follow 打不开（老 agent 502 等）自动回退轮询并提示「轮询中」
- 已知限制：CallAgent 30s 超时约束 `stack.git.deploy` 的 fetch 时长（大仓库慢网络下 webhook 部署可能超时失败，重试即可）

#### P2 ③ 设计（2026-10-13）

**inventory 联动**（决策记录见 decision-log D-2026-10-13-1）：

- **事实源**：agent 的 `stack.list` / `stack.status` 服务条目自带已发布端口（docker list API 的 `Ports` 随容器列表返回，零额外 Engine 调用）；`ContainerInfo.Ports` → `StackService.Ports: [{hostPort, containerPort, proto}]`，只收 `hostPort > 0`——未发布端口仅容器网络内可达，probe 触达不到
- **收编**：server 在 `cacheStack`（列表同步点，单 agent 与聚合两条路径共用）调用 `syncStackServices`：每条 **tcp** 已发布端口 upsert 一条 CMDB `Service` 台账行，`Type=tcp`、`URL=tcp://{agentIP}:{hostPort}` → probe 走既有 `parseHostPort` + `CheckTCP`，**probe 侧零改动**；不新建表，复用 `services` 表
- **确定性 ID**：`stackport-{agentId}-{stack}-{hostPort}-tcp`（同 `dns-` 先例），端口消失时按 ID 删除，无需反查
- **来源标记**：`Labels{source: "stack", agent_id, stack}`（同 DNS `labels.source=dns` 先例）；inventory 声明或手工登记的行无此标签，对账永不触碰
- **host 取 agent IP**：registry 优先、离线退回 DB 档案；IP 为空不建行（没有探测目标）；IP 变更时刷新 URL 但探测状态保留——`storage.UpsertServiceTarget` 只动寻址字段（name/agent/type/url/labels），`status/responseTimeMs/lastCheck` 归 probe 独占
- **对账**：端口对账随每次 `syncStackServices`（该 stack 名下集合外的行删除）；stack 消失对账在全量列表成功后 `pruneStackServices`——agent 不可达 ≠ 已删除，探测失败的 agent 不对账
- **边界**：udp/sctp 端口只在 UI 展示不收编（probe 仅实现 TCP/HTTP）；不做手动映射 UI（CMDB Service 仍由 inventory.yaml 文件管理，没有手工关联的落点，等真实需求出现再议）；Dashboard 的 ServicesUp/Down 计数把 stack 行一并算入（可接受的耦合，台账行本就该被监控）
- **尽力而为**：联动失败只记日志，不影响 stack 缓存与 API 应答（同 DNS 台账联动 D20 口径）

## 风险与权衡

| 风险 | 应对 |
|------|------|
| compose CLI 行为随 Docker 版本漂移 | 只依赖 `up -d --remove-orphans` / `down` / `logs` / `config -q` 四个稳定子命令；agent 启动时探测版本并上报 |
| 轮询延迟（部署完成感知最多滞后 2s） | 可接受；P2 流式化。不为此提前扩协议 |
| agent 进程重启丢失内存任务态 | agent 重启丢内存任务态 → task.get 404；历史终态由 server `stack_deployments` 表兜底 |
| 用户手改文件与 UI 编辑冲突 | compose-file-first 原则：保存即覆盖（带 modifiedAt 乐观提示，不强做 merge） |
| Windows agent | P1 不支持，provider 注册时检测 `runtime.GOOS`，非 linux/darwin 直接跳过并记日志 |

## 验收记录

### 2026-09-14 本机端到端验收（无真实 Docker）

**方式**：`scripts/local-acceptance/run-acceptance.sh` 一键运行——真实
`cockpit` server 与 `cockpit-agent` 二进制，Docker 侧用替身（unix socket 上的
最小 Engine API + PATH 内 compose CLI 假体，`up` 后状态真实联动 `status` 查询）。
链路完整经过 REST → JWT → 审计 → WebSocket RPC → stack provider → compose 调用。

**结果**：32 项断言全部通过，重跑可重复；`go test ./...` 全绿。

**覆盖**：创建（created=true）→ compose/.env 回读一致 → 非法 compose 校验失败
不落盘 → 列表 → up 异步任务（taskId/轮询 success）→ 同名并发 up 409 →
status 联动（running=2/total=2）→ logs → down → remove 删目录 →
agent 断连缓存灰态（online=false）→ 审计事件（stack_create/up/down/remove）
齐全且 `.env` 值不泄漏。

**验收中发现并修复**：`DOCKER_HOST` 不支持 Docker 官方惯例格式
`unix:///path`——detector 直接 `stat` 带前缀字符串必然失败，导致真机上用户
设置标准格式时 agent 漏检 docker 能力；而裸路径形式虽能检出，endpoint 传给
docker 库（要求 scheme）又会解析失败。修复：`detector.normalizeUnixHost`
兼容两种形式并统一输出 `unix://` endpoint（含单测）。

**遗留（真实 Docker 测试机）**：镜像真实拉取、容器健康/端口/重启策略、
compose build/pull/profiles/healthcheck 语义、`COCKPIT_STACKS_DIR` 权限实践。

### 2026-09-14 本机端到端验收 · M1.5 增补

同一套脚本扩展后跑 M1.5 新功能，42 项断言全部通过，重跑可重复：

- **A15 restart/pull**：两动作均经 up 同款异步任务通道，任务轮询到 success
- **A16 部署历史**：`/history` 返回 server 侧记录，up/restart/pull 三动作齐全，
  终态由后台 goroutine 回填（`finishedAt > 0` 且 status=success）
- **A17 目录自检**：聚合接口 `agentInfo` 带 `dirWritable=true` 与 stacks 目录路径
- **A14 审计扩展**：六个事件齐全（create/up/down/remove/restart/pull），`.env` 值仍不泄漏

### 2026-09-28 真实 Docker 测试机验收

**环境**：Docker 29.8.1 + Compose v5.5.1 真机（Linux VM，agent 以 root systemd 服务运行，`COCKPIT_STACKS_DIR=/var/lib/cockpit/stacks`）；server 为本机验收二进制（`.acceptance/` 专用配置，不入库）；镜像经真实 Docker Hub 拉取（nginx:alpine、busybox）。

**通过（R1-R17 + 补充 S1-S3）**：

- **创建/回读/校验**：created=true、compose 回读 md5 一致；非法 compose 经真实 `docker compose config -q` 拦截（502）且原文件不落盘
- **部署全链**：up 真实拉镜像起容器（running=2/total=2，`profiles: [debug]` 服务正确排除）；端口跨主机可达（18080→HTTP 200）；healthcheck 进入 healthy；`unless-stopped` 与 `.env` 变量替换实证（容器内 printenv）
- **自愈/日志/动作**：kill 容器进程后 Docker 按重启策略自动拉起（约 7s，重启退避内）；logs 返回真实 nginx 输出；pull/restart 真实生效
- **历史/审计**：部署历史 finishedAt 由后台回填（含一次诚实 failed 记录）；审计 5 事件齐全且 `.env` 值不泄漏
- **compose 完整语义（S1/S3）**：命名卷跨容器共享 + depends_on 启动顺序（db 写卷 → app 读到 seed，卷带项目前缀创建）；自定义 bridge 网络创建 + 服务间 DNS 解析（b 容器 `ping a` 成功后落文件实证）
- **边界行为**：build 缺 Dockerfile 优雅失败（任务 failed 日志可读，不影响 server）；`down` 保留命名卷（compose 默认语义，非 `-v`，符合预期）；remove 删目录；目录自检 dirWritable=true + composeVersion 5.5.1
- **目录权限**：`/var/lib/cockpit/stacks` 0700 且可写探针通过；「目录不存在」场景自动创建（list 返回空 + info 报 dirWritable=true）

**环境杂音（非缺陷）**：首轮 up 因 Docker Hub auth 瞬断失败（任务日志如实记录原始错误），重试即通过。

**验收中发现并修复**：stacks 目录路径非法（如误配为普通文件）时 `stack.list` 报错，`GET /api/stacks/agents/{id}` 曾把 agent 原始错误（`read stacks dir: ... not a directory`）替换成笼统的 "Invalid agent response"，与 `handleStackRPC` 的透传行为不一致；已改为透传原始原因（含单测断言透传文案）。

**已知边界（记录为后续小项，不在本次修复）**：

- 路径非法场景下 `stack.info`（dirError）到不了列表页——server 仅在 list 成功后拉取自检信息；需要 server 侧「list 失败仍拉 info」+ web 呈现联动
- 「无权限」场景对 root agent 不可模拟（CAP_DAC_OVERRIDE 绕过 DAC），需非 root 部署真验
- 镜像 nginx:alpine/busybox 留作 VM 缓存未清理；模板库/import 为 UI 填充路径（落库即常规 create+up，UI 链路由 vitest 覆盖），真机以原始 compose 等价验证

### 2026-10-12 本机验收（P2 ① Git-to-deploy，A18）

**方式**：`run-acceptance.sh` A18 段——本地 bare 仓当远端（不依赖外网凭据），seed 工作仓推三版内容，走真实 REST → JWT → 审计 → WebSocket RPC → stack provider → git CLI 全链路。53 项断言全部通过，重跑可重复。

**覆盖（A18.1-A18.7）**：未接入 reason=not-a-repo → attach（enabled=true、远端文件落地、head=remoteHead）→ 远端新提交二次 pull 快进（磁盘内容更新）→ 本地改 tracked 文件 + 远端同文件新提交 → pull 409 → `stack.git.status` drift 列表（attach 前已存在的 UI 建 stack 记为 untracked `??`，预期语义）→ `ext::` 传输助手远端被拒（502）→ 审计含 `stack_git_pull`。agent 单测另以真实 git 覆盖 attach/untracked 冲突不覆盖本地文件、分支切换、凭据脱敏、`gitPathSkipped` 边界。

**验收中发现并修复**：agent 侧 execlog（router 单点埋点）把 `stack.file.save` 入参 `env`（.env 明文凭据负载）原样记入留痕并上行落审计表——违反「凭据不落审计」红线；此前因 execlog 30s 攒批晚于验收取审计而侥幸未暴露。修复：`core/execlog` 敏感键正则补 `env`（全仓唯一使用方即 stack.file.save），A14 回归断言转绿。

### 2026-10-12 本机验收（P2 ② webhook + 日志流，A19）

**方式**：`run-acceptance.sh` A19 段——同一本地 bare 远端，`stacks.webhook_token` 配置开启；链路 REST（无登录态，`X-Cockpit-Token`）→ webhook 扇出 → WebSocket RPC `stack.git.deploy` → git CLI → compose 假体 → 任务 follow NDJSON 流。62 项断言全部通过，重跑可重复。

**覆盖（A19.1-A19.7）**：未配令牌 404（默认关）→ 已配令牌但缺/错 `X-Cockpit-Token` → 401 → 带盘外 `remote` 的恶意 body 受理但参数被剥离（200 + triggered）→ 新 stack 落盘 + 已接入 stack 快进（nginx:webhook）→ 部署历史 action=up → follow 流含 `"eof":true` 帧 → follow 未知任务 404 → 审计含 `stack_webhook_deploy`（username=webhook）。A18.4-A18.7 drift 用例回归同绿。

**验收中发现并修复**：① `stack_provider.go` Call 分派漏 `git.deploy` case（方法已实现但 RPC 到不了，报 unknown stack action）——单测直调方法掩盖分派缺失，补 case 并将一条用例改走 `Call` 覆盖分发行；② A19.4/A19.5 断言与异步落库竞速（webhook 受理即 200，部署历史行要等 `startStackDeployment` 写入、`trackStackTask` 轮到终态才翻转 status）——改为轮询 `action=up && status!=running` 行后再断言；③ follow 收尾的 `stop` RPC 改即发即忘（goroutine）——同步等应答在 agent 无应答时被 CallAgent 30s 超时拖住 handler 返回（no-flusher 500 路径 race 步实测 30s）。race 步顺带实锤一个**存量**竞态：`RemoteSessionManager.Get/List` 直透存储指针，WS 数据面 goroutine 并发 `UpdateStatus` 写同一会话字段——读侧改锁内浅拷贝快照（独立 fix commit）；server 包 race 插桩本地实测 1004s，CI race 步 timeout 900s→1500s。

### 2026-10-13 本机验收（P2 ③ inventory 联动，A20）

**环境**：本地 bare 仓当远端（A18 已建）+ fake-docker-daemon（A20 起 `compose up` 解析 compose ports 段输出 `Ports` 公共/私有端口对）+ 真 agent 二进制；`cache-demo` 带 `8080:80 tcp` + `5353:53 udp` 双端口。

**验收中发现并修复**：A20 断言误用 `/api/services`（该路由不存在，CMDB 台账行挂在 `/api/resources/services` 资源派发下）——服务行实际已正确落库（DB 直查 + probe 巡检均见行），纯断言 URL 笔误；修正后 A20.1-A20.4 全绿（台账行收编/name 定型/URL 指向 agent 主机端口/udp 不入账/服务视图带端口列表）。

- [Komodo Git-to-deploy](https://komo.do/docs/intro)——Core/Periphery 同构、自动化程序（可对照本方案 P2）
- [Dockge](https://github.com/louislam/dockge)——compose-file-first、目录即栈
- 内部：[参考项目对比与借鉴](./reference-projects.md)、[协议定义](./protocol.md)、`todo.md` P1 条目

