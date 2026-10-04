# P1 方案设计：执行 Job 模型（最小闭环）

> 2026-10-04。P1「Job」首期落地设计。战略评审（见 reference-projects）把
> Job/Action/Workflow/Event 列为控制面四个核心抽象，本文只做 Job 的最小
> 闭环：一条持久化的执行记录 + 一个可执行的内置类型（agent.exec），
> Action/Workflow/Event 留待后续在 Job 之上生长。

## 实现状态（2026-10-04 对账）

- **已实现**（22bd7fd → 4a0660f 六笔）：本文「最小闭环」范围全链落地——
  `storage.Job` 模型 + AutoMigrate；Job 类型 `agent.exec` 走 agent 侧 `job.exec`
  RPC provider（命令 ≤16KB、超时 ≤300s 缺省 60s、输出 64KB 截断、超时按
  进程组 SIGKILL）；`GET|POST /api/jobs` + `GET /api/jobs/{id}`（同步等待终态
  回写）；终态单条审计 `job_run`（details 含 type/target/status/params）；
  Jobs 页（全机台账 15s 轮询 + 创建即执行弹窗 + 终态详情）；权限点
  `jobs:read`/`jobs:write`（PermGuard 控制创建入口）。
- **未实现**（按本文「边界（明确不做）」节，立项时再设计，不写进当前能力）：
  异步执行/取消（`cancelled` 与独立 `timeout` 终态）、Workflow/定时触发、
  输出流式、Action/Event 抽象与 AI 接线（P5）。
- **真机验收**（2026-10-05，探针 `scripts/acceptance/jobs/` 五件套 9/9 PASS，
  acceptance-checklist「统一 Job 执行」节七项全勾）：执行链路（82ms 成功回执 /
  exit 3 输出带回 / 超时 1.078s 按时返回且 `pgrep` 无孤儿 / 截尾恰 64KB 尾部保留）、
  离线 503 不落幽灵、校验面 400×7、终态审计恰一条、权限双角色 403 全部实证。
  **验收逮到 D8 路由面缺口**：`/api/jobs` 未登记 `resourceRules`，RBAC
  governed=false 整体放行（viewer 可执行），修为补规则 + 三角色矩阵 7 例。

## 现状与痛点

| 痛点 | 现状 | 后果 |
|------|------|------|
| 「点一下执行」没有统一入口 | 备份跑一次、cron 手动触发、服务重启各是各的 API，互不复用 | 每加一种动作就要新增一条 RPC + 一个页面入口 |
| 执行无台账 | 执行完即散，只能翻审计日志拼现场 | 想查「昨晚那台机器跑过什么、输出是什么」无从下手 |
| 权限粒度粗 | 备份/代理/定时任务各有权限点，通用命令执行没有模型 | 无法回答「谁能在这台机器上跑命令」 |
| AI/自动化没有可挂接的执行面 | 无统一 Job API，任何自动化都要绕业务接口 | 战略评审 P5「AI 接线」无处落地 |

## 架构总览

与定时任务同一通道模型：**同步 RPC + server 落库**。区别于 cron 的
「crontab 即状态、server 纯转发」，Job 的价值恰在台账——server 是事实源。

```
┌─ server ───────────────────────┐        ┌─ agent（目标主机）─────────────┐
│ POST /api/jobs                 │ ─RPC─▶ │ job provider（新）              │
│ 校验 → 建 Job(pending)         │        │ job.exec：sh -c / cmd /C        │
│   → CallAgent(job.exec)        │ ◀─RPC─ │ 带超时执行，返回 exit_code/输出  │
│   → 终态回写 + 审计 job_run    │        │ （输出截尾 64KB，业务字段不炸RPC）│
│ GET /api/jobs[/{id}] 台账查询  │        └─────────────────────────────────┘
└────────────────────────────────┘
```

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | 状态机 | `pending → running → success \| failed`；cancelled/timeout 不设独立态——超时被杀即 failed（error=「timed out after Ns」，exit_code=-1），取消是后续异步化才有的能力 | 同步 RPC 下 running 为瞬态，终态二分已覆盖当下全部路径；状态集合按需扩展 |
| D2 | 首个类型 | `agent.exec`：目标 agent 本机执行一条 shell 命令（unix `sh -c` / windows `cmd /C`） | Control 域第一块；命令执行是其它一切动作的兜底语义 |
| D3 | 通道 | 复用既有 RPC：server `CallAgent(target, "job.exec", params)`，agent 端 JobProvider 注册（平台无关，无条件注册） | 与 cron/备份同构，零新基建 |
| D4 | 退出码语义 | 非零退出码是**业务字段**不是协议错误：RPC 层恒 success，`exit_code`/`output`/`error` 走 data；server 据此落 success/failed（输出仍带回） | 「命令失败」是 Job 的正常产物，不应混入传输层错误 |
| D5 | 超时 | 双端同限：入参 `timeout_s` 1-300（缺省 60）；unix 侧进程组隔离（Setpgid）+ 超时杀整组，防止孙进程握住输出管道导致「假超时」 | 超时必须真的按时返回；`sh -c` 派生链是最常见悬挂源 |
| D6 | 输出上限 | agent 截尾保留末 64KB（`jobExecMaxOutput`），server 落库同限（`storage.JobMaxOutput`）；命令长度上限 16KB | 错误信息通常在尾部；防长输出表膨胀 |
| D7 | 离线拦截 | 创建前查 registry，目标不在线直接 503，**不建幽灵记录** | 与 cron 同口径；台账里不出现「从未执行」的记录 |
| D8 | 权限 | 权限点 `jobs:read` / `jobs:write`（resourceActions + allPermissions 两处登记）；服务端校验与 cron 同口径（路由层 auth middleware） | 台账人人可看（read），执行是高危操作（write） |
| D9 | 审计 | 终态一条 `job_run`（resource=job，id=Job ID），details 含 type/target/status/params | 命令入参进审计与 cron_apply 同口径，便于追溯「谁在哪台机器跑了什么」 |
| D10 | 存储 | SQLite `jobs` 表（AutoMigrate），Parameters 存 JSON 串、API 层解析回对象返回 | 单机个人云规模够用；gorm 现有惯例 |

## Server 侧设计

### REST API

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST /api/jobs` | `{type, target, parameters}` | 创建并执行，同步返回终态视图 |
| `GET /api/jobs` | — | 最近 50 条台账（倒序） |
| `GET /api/jobs/{id}` | — | 单条详情（含输出） |

校验（`validateJobPayload`）：type 必填且在白名单（当前仅 `agent.exec`）；
target 非空；exec 需 `parameters.command` 非空且 ≤16KB；`timeout_s` 1-300。

### Job 视图

```json
{
  "id": "…", "type": "agent.exec", "target": "agent-id", "actor": "username",
  "status": "success", "parameters": {"command": "uptime"},
  "output": "…尾部 64KB…", "exitCode": 0, "error": "",
  "createdAt": "…", "startedAt": "…", "finishedAt": "…"
}
```

## Agent 侧设计

### RPC 方法

| 方法 | 参数 | 返回 | 说明 |
|------|------|------|------|
| `job.exec` | `{command, timeout_s?}` | `{exit_code, output, truncated, error, duration_ms}` | exit_code -1 = 启动失败/超时被杀 |

- 跨平台：`sh -c`（unix）/ `cmd /C`（windows），`exec.CommandContext` + deadline；
- unix 侧 `Setpgid` 进程组 + Cancel 杀整组：`sh -c` 派生的孙进程（如 sleep）
  会握住输出管道，只杀直接子进程会让 `CombinedOutput` 等到孙进程自然结束
  ——超时语义必须是「整组即刻终止」；
- 非 agent.exec 类平台差异（容器内执行、幂等 Action 等）是后续类型的事，
  provider 的 action 分发已留位。

## 前端设计

- `web/src/pages/Jobs/index.tsx`：全机台账表（15s 轮询 + 手动刷新），
  创建弹窗（在线 agent 下拉 + 命令 + 超时），详情弹窗（状态/退出码/输出尾部）；
- `web/src/services/jobs.ts`：listJobs/createJob/getJob（axios，Bearer 注入）；
- 路由 `/jobs`，菜单「Job 执行」，perm `jobs:read`，创建按钮 `jobs:write`。

## 测试

- storage：CRUD/倒序/limit（`internal/storage/job_test.go`）；
- agent rpc：成功/非零退出/超时杀整组/截尾/越限/未知 action（`internal/agent/rpc/cov_job_provider_test.go`，windows 恒适用用例直跑）；
- server：校验各 400 分支、离线 503、成功/失败 dispatch、列表/详情/405/404（`internal/server/cov_jobs_test.go`，假 agent 注入）；
- 覆盖率：防御分支登记 `tool/known_uncoverable.txt`（SQLite 查询/写入不可致错、auth 上下文单测惯例、协议不可变形）。

## 边界（明确不做）

- **不做异步执行**：创建即同步等待终态。异步化（排队/取消/进度）等 Workflow
  立项时一并设计，届时 cancelled/timeout 独立状态、`GET /jobs?status=running`
  过滤、SSE 推送才有真实的消费者；
- **不做 Workflow/Action**：Workflow 是「一组 Job 的编排」（依赖/并行/重试），
  Action 是「带语义的幂等操作」，都在 Job 之上，本期只有 Job 本体；
- **不做定时触发**：Job 与 cron 是两个平面——cron 是 agent 侧调度面（crontab
  即状态），Job 是控制面执行台账；「定时跑 Job」= cron 写一条调用 `POST
  /api/jobs` 的命令行，不引入新耦合；
- **不做输出实时流**：终态一次性回传。流式输出（WebSocket attach）留待
  远控/终端域统一设计。

## 后续生长点

| 优先级 | 方向 | 依赖 |
|--------|------|------|
| P2 | Workflow：Job 编排（DAG/重试/审批） | 本模型 |
| P0′ | 事件模型：Job 终态事件进统一事件总线 | Event 抽象立项 |
| P5 | AI 接线：Job API 即 AI 的执行面（agent.exec 之外补结构化类型） | 审计与限额先行 |
