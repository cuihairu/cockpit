# P1 方案设计：应用部署（Compose Stack）

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
- compose CLI 不可用则不注册 → server 对该 agent 返回 404 语义 → UI 灰显「此 Agent 不支持应用部署」，容器管理不受影响

### 文件布局（compose-file-first）

```
/var/lib/cockpit/stacks/        # COCKPIT_STACKS_DIR，可覆盖
└── <stack-name>/
    ├── compose.yml             # 真相源，UI 可编辑
    ├── .env                    # 可选，UI 可编辑（敏感）
    └── .cockpit/
        └── last-deploy.log     # 最近一次任务的合并输出（stdout+stderr）
```

约束：

- `<stack-name>` 必须匹配 `^[a-z0-9][a-z0-9_-]{0,63}$`（同时满足 compose project name 规则），所有文件操作 jail 在 `COCKPIT_STACKS_DIR` 内，杜绝路径穿越
- 单文件上限 512KB；`.cockpit/` 为保留目录，list 时跳过
- 目录即 stack：外部手动放入的合法目录同样被识别（Dockge 行为），server 端不建「先注册后使用」的流程

### RPC 动作表

方法名遵循既有 `<provider>.<action>` 格式（`internal/agent/rpc/router.go`）：

| 方法 | 参数 | 返回 | 同步/异步 |
|------|------|------|-----------|
| `stack.list` | — | `[{name, services: [{name, image, state, status}], running, total}]` | 同步 |
| `stack.status` | `name` | 单个 stack 的服务/容器明细（含容器 ID，可跳转 `/docker`） | 同步 |
| `stack.file.get` | `name` | `{compose, env, modifiedAt}` | 同步 |
| `stack.file.save` | `name, compose, env?` | 保存前执行 `docker compose config -q` 校验，失败返回错误不落盘 | 同步 |
| `stack.up` | `name` | `{task_id}`，等价 `docker compose up -d --remove-orphans` | 异步 |
| `stack.down` | `name` | `{task_id}`，等价 `docker compose down` | 异步 |
| `stack.logs` | `name, service?, tail?` | compose logs 文本 | 同步 |
| `stack.task.get` | `task_id` | `{stack, action, status: running/success/failed, log, startedAt, finishedAt}` | 同步 |
| `stack.remove` | `name` | 先 `down`（已停则跳过），成功后删目录 | 异步 |

实现要点：

- **任务模型**：provider 内存维护 `map[taskID]*Task` + 每 stack 一把互斥锁；同 stack 并发 `up/down` 时第二个请求直接报 `stack busy` 错误（server 转 409）。任务日志同时写入内存环形缓冲（取最近 ~200KB）与 `last-deploy.log`
- **状态查询零改动**：复用 `docker.Client.ListContainers(true)`（`internal/docker/client.go`），按 `Labels["com.docker.compose.project"] == name` 客户端侧过滤，不动 DockerAPI 接口签名；规模大后再加 Filters 参数
- **`compose.yml` 与 `compose.yaml`**：读写优先 `compose.yml`，兼容识别 `compose.yaml`/`docker-compose.yml`（只读兼容，保存时统一规范为 `compose.yml`）

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
| `POST /api/stacks/agents/{agentId}/{name}/{action}` | `stack.up` / `stack.down` | action ∈ {up, down}，返回 `{task_id}` |
| `GET /api/stacks/agents/{agentId}/{name}/logs` | `stack.logs` | `?service=&tail=` |
| `GET /api/stacks/agents/{agentId}/tasks/{taskId}` | `stack.task.get` | 任务轮询 |
| `DELETE /api/stacks/agents/{agentId}/{name}` | `stack.remove` | 删 stack（body 可选 `{force}`） |

约定：404 = agent 不在线/无 stack provider；409 = stack busy；502/504 沿用 `api_docker.go` 的 RPC 错误映射。

### 存储

`storage` 新增 `stacks` 表（索引缓存，非真相源）：

```
Stack{ ID, AgentID, Name, RunningCount, TotalCount, LastDeployedAt, LastTaskStatus, UpdatedAt }
```

- agent 上报 list 时 upsert；agent 离线时 `GET /api/stacks` 用它渲染灰态
- 部署历史表（每次 up/down 一条记录）推迟到 M1.5，P1 只靠审计日志追溯

### 审计

复用 `audit.Logger` 新增事件：`stack.create`（首次 save）、`stack.update`（save）、`stack.up`、`stack.down`、`stack.remove`，detail 记 name + agent + 任务结果。**遵守既有约束：`.env` 内容与 deploy 输出一律不入审计日志**，只记事件与元数据。

## Web 侧设计

新增页面 `web/src/pages/Stacks/`，导航「应用部署」，路由 `/stacks`；`/docker` 页保持单容器视角不动，stack 详情中容器行提供跳转链接。

- **列表页**：按 agent 分组的卡片/表格（名称、running/total、最后部署时间、状态 Tag）；顶部「新建 Stack」按钮（选 agent + 名称 + 模板：空/Nginx 示例）
- **详情页**（抽屉或子路由）三个 Tab：
  1. **服务**：服务/容器状态表 + 「启动/停止」主按钮（触发 up/down → 轮询 task → 完成刷新）
  2. **compose.yml**：等宽 textarea 编辑 + 保存（后端校验失败时展示 `docker compose config` 错误输出）；`.env` 单独折叠区，值默认掩码显示
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
- [ ] 验收：在一台测试机上完成「新建 → 编辑 compose → up → 改配置 → up（重建）→ down → 删除」全流程；同名并发 up 返回 409；agent 断连后列表显示缓存灰态

### M1.5 —— 体验补齐

- [ ] 部署历史表 + 详情页时间线
- [ ] 模板库扩充（常见个人服务）+ import（粘贴已有 compose.yml）
- [ ] `restart` / `pull` 动作；多文件 compose（`include:`）支持评估
- [ ] stacks 目录权限/位置的自检提示

### P2 —— Git-to-deploy（Komodo 式升级）

- [ ] stacks 目录升级为 Git worktree：远端仓库拉取 → drift 提示（本地改动 vs 仓库）
- [ ] webhook 触发部署；部署日志流式推送（复用 proxy data 通道，替代轮询）
- [ ] inventory 联动：stack 与 CMDB 服务条目关联，probe 直接探测 stack 暴露的端口

## 风险与权衡

| 风险 | 应对 |
|------|------|
| compose CLI 行为随 Docker 版本漂移 | 只依赖 `up -d --remove-orphans` / `down` / `logs` / `config -q` 四个稳定子命令；agent 启动时探测版本并上报 |
| 轮询延迟（部署完成感知最多滞后 2s） | 可接受；P2 流式化。不为此提前扩协议 |
| agent 进程重启丢失内存任务态 | 任务状态落 `last-deploy.log`，重启后 task.get 返回 `unknown task`，UI 引导查看文件日志 |
| 用户手改文件与 UI 编辑冲突 | compose-file-first 原则：保存即覆盖（带 modifiedAt 乐观提示，不强做 merge） |
| Windows agent | P1 不支持，provider 注册时检测 `runtime.GOOS`，非 linux/darwin 直接跳过并记日志 |

## 参考

- [Komodo Git-to-deploy](https://komo.do/docs/intro)——Core/Periphery 同构、自动化程序（可对照本方案 P2）
- [Dockge](https://github.com/louislam/dockge)——compose-file-first、目录即栈
- 内部：[参考项目对比与借鉴](./reference-projects.md)、[协议定义](./protocol.md)、`todo.md` P1 条目
