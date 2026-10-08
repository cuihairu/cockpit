# P2 方案设计：Workflow 与异步 Job

> 2026-10-09 立项设计，未实现。P1 执行 Job 模型（[jobs-design](./jobs-design)）
> 已落地 `agent.exec` 最小闭环；本设计是它的下一生长点：Job 异步化 + 线性
> Workflow 编排。设计边界遵守战略评审定调：Workflow 是「一组 Job 的编排」，
> 只做控制面接线，不重造底层调度器。

## 为什么现在做

P1 的同步 RPC 模型（POST 即等待终态）有两个天花板：

1. **最长 300s**：`agent.exec` 的 `timeout_s` 上限 300，浏览器 HTTP 请求
   等不了更长；超过即 HTTP 层先断，台账里还可能出现「HTTP 断了但 Job 落了
   终态」的双头现象。
2. **编排无处挂接**：停机 → 备份 → 升级 → 起机 → 健康检查这类多步动作，
   现在只能人手一步步点，或各自写脚本调 API。步骤失败后的处置（停 or 继续、
   重试几次）没有落点。

异步化不是独立功能——它是 Workflow 的前置：编排器必然在后台推进步骤，
不可能同步等待。两者一并设计，一次把状态机扩到位。

## 实现状态

- **未实现**。本文为立项设计，按里程碑推进：M1（异步 Job + 线性 Workflow）
  待拍板后实现；M2+ 各项在「边界（明确不做）」。
- P1 已实现部分不受影响：`agent.exec` 类型、job provider、权限点原样保留，
  本设计只改「server 侧何时返回」与「谁在等待」。

## 架构总览

```
┌─ server ────────────────────────────────────────────────┐
│ POST /api/jobs          POST /api/workflows/{id}/run    │
│   建 Job(pending) ──┐     建 Run + 逐步建 Job(pending)   │
│   立即返回 201      │            │                        │
│                     ▼            ▼                        │
│           dispatcher goroutine 编排器 goroutine          │
│           CallAgent(job.exec)    逐步：建 Job → 等终态    │
│           终态回写               失败按 continue_on_error  │
│                                  决定停/续，run 落终态     │
│ GET /api/jobs?status=&run=   GET /api/workflows/{id}/runs │
└─────────────────────────────────────────────────────────┘
```

要点：**agent 侧零改动**。`job.exec` RPC 本就是「带超时同步执行、返回
exit_code/输出」，编排器和单步 dispatcher 都只是 server 侧的 goroutine，
等的是同一个 RPC。取消能力受 RPC 无取消帧的约束（D3），如实声明。

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| W1 | POST /api/jobs 语义 | **全异步**：创建后立即 201 返回 pending Job 视图，后台 dispatch；原同步等待删除 | 消费者仅 web 一处（mobile 未接 jobs API），无兼容负担；「创建即返回 id」是台账模型的本义，P1 的同步返回是权宜 |
| W2 | 状态机 | `pending → running → success \| failed \| cancelled`；**timeout 不设独立态**（保持 `failed` + error=`timed out after Ns`） | error 字段已可判定超时，独立态只增加过滤面；cancelled 是异步化后才有的真实能力，必须独立（running 被取消的 Job 不该计作 failed） |
| W3 | 取消语义 | 只允许取消 **pending**（未派发）Job；running 拒绝（409，说明 job.exec 上限 300s 会自然结束）。Workflow run 取消 = 停止推进后续步骤 + 当前步骤同规则 | RPC 协议无取消帧，假装能取消 running 是撒谎；诚实边界是「派发前可撤，派发后等它回来」。agent 侧信号注入取消留 M2 |
| W4 | 编排形态 | **线性步骤链**（steps 有序数组，逐步执行），非 DAG；步骤级 `continue_on_error` 与 `retry`（0-3 次，固定间隔 5s） | 个人基础设施最高频就是串行链（停机→备份→升级→起机→检查）；DAG 的 edges 表达在 UI 和存储里成本翻倍，等真实并行需求出现再扩。重试覆盖最常见韧性需求 |
| W5 | 步骤与 Job 的关系 | 每步创建**真实 Job**（`jobs` 表加可空列 `workflow_run_id`），run 台账里每步可点开看输出/退出码 | 步骤不是影子记录——它就是一次执行，进统一台账、进既有审计 `job_run`；不新造「步骤日志」平行体系 |
| W6 | 存储 | `workflows`（定义）+ `workflow_runs`（实例）两表；run 内嵌 steps 运行态 JSON 快照（含每步状态/尝试次数/关联 Job id）；AutoMigrate | 个人规模读多写少，步骤子表是过度设计；快照即事实（当时怎么跑的、每步结果） |
| W7 | run 状态机 | `running → success \| failed \| cancelled`（无 pending——建 run 即开始推进）；部分失败 = failed，details 记明断点 | run 是进程性实体，建了就跑；「排队中」的 run 没有消费者 |
| W8 | 并发约束 | 同一 workflow 同时至多一个 active run（409 拒绝）；不同 workflow 可并行 | 个人规模防重入（连点两次「运行」）；不做全局限流 |
| W9 | 权限 | `workflows:read` / `workflows:write`；run/cancel 归 write；步骤 Job 沿用 `jobs:*` 既有面 | 与 cron/Job 同口径：台账可读（read），执行高危（write） |
| W10 | 审计 | 定义类操作 `workflow_create/update/delete`；run 终态一条 `workflow_run`（resourceID=run id，details 含 workflow 名/steps 摘要/断点）；步骤 Job 自带 `job_run` | 定义是变更、run 是执行，两类分开；步骤已有审计不翻倍 |
| W11 | 触发 | 手动 `POST /api/workflows/{id}/run`。「定时跑 Workflow」= cron 写一条调该 API 的命令（P1 边界节既有约定） | cron 是 agent 侧调度面、Job/Workflow 是控制面执行台账，两个平面不耦合；server 侧定时器是另一个立项 |
| W12 | 步骤参数 | `{"name","type","target","parameters","timeout_s"?,"continue_on_error"?,"retry"?}`；type 白名单沿用 `jobTypes`（M1 仅 `agent.exec`） | 与 Job payload 同构，校验复用 `validateJobPayload`；新类型（容器操作、备份触发）注册进白名单即自动可用于编排 |

## REST API

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET /api/workflows` | — | 定义列表 |
| `POST /api/workflows` | `{name, description?, steps[]}` | 创建定义（≤20 步） |
| `GET /api/workflows/{id}` | — | 定义详情 |
| `PUT /api/workflows/{id}` | 同创建 | 更新（active run 存在时 409） |
| `DELETE /api/workflows/{id}` | — | 删除定义（历史 run 保留） |
| `POST /api/workflows/{id}/run` | — | 创建 run 并后台推进，201 返回 run 视图 |
| `GET /api/workflows/{id}/runs` | `?status=` | 该定义的 run 台账 |
| `GET /api/workflow-runs/{rid}` | — | run 详情（含 steps 快照） |
| `POST /api/workflow-runs/{rid}/cancel` | — | 取消 run（W3 语义） |
| `GET /api/jobs` | `?status=&target=&type=&workflow_run_id=` | P1 台账加过滤（run 详情页列步骤用） |
| `POST /api/jobs/{id}/cancel` | — | 取消 pending Job（W3） |

校验：name 非空 ≤64 字符；steps 非空 ≤20；step.name 同一 workflow 内唯一
≤64；`timeout_s`/`command` 等沿用 P1 各限；`retry` 0-3。

## 数据模型

```go
type Workflow struct {
    ID          string    `gorm:"primaryKey" json:"id"`
    Name        string    `json:"name"`
    Description string    `json:"description,omitempty"`
    Steps       string    `json:"-"` // JSON 串，API 层解析
    CreatedBy   string    `json:"createdBy"`
    CreatedAt   time.Time `json:"createdAt"`
    UpdatedAt   time.Time `json:"updatedAt"`
}

type WorkflowRun struct {
    ID           string     `gorm:"primaryKey" json:"id"`
    WorkflowID   string     `gorm:"index" json:"workflowId"`
    WorkflowName string     `json:"workflowName"` // 冗余，定义删除后台账可读
    Status       string     `gorm:"index" json:"status"`
    Actor        string     `json:"actor"`
    Steps        string     `json:"-"` // 运行态快照 JSON（每步状态/尝试/JobID）
    CreatedAt    time.Time  `json:"createdAt"`
    FinishedAt   *time.Time `json:"finishedAt,omitempty"`
}
// storage.Job 增列：WorkflowRunID string `gorm:"index;default:''"`
```

步骤快照形态：

```json
[
  {"name":"backup","type":"agent.exec","target":"a1","status":"success",
   "jobId":"j-1","attempts":1},
  {"name":"upgrade","type":"agent.exec","target":"a1","status":"failed",
   "jobId":"j-2","attempts":2,"stopReason":"retries exhausted"}
]
```

## 前端

- `web/src/pages/Workflows/index.tsx`：定义列表（名称/步数/最近 run 状态/
  运行按钮）+ 新建/编辑抽屉（步骤编辑器：逐行 name/target/命令/超时/
  continue_on_error 开关/retry 下拉，上下移排序）；
- run 详情：步骤时间线（每步状态徽标 + attempts + 跳转 Job 详情弹窗），
  进行中 5s 轮询、终态停轮询；
- Jobs 页：创建弹窗改异步语义（提交即关 + toast「已提交，可在台账查看」），
  列表加状态过滤；pending 行出现「取消」按钮（`jobs:write`）；
- 菜单「Workflow 编排」（`workflows:read`），perm `workflows:write` 控写入口。

## 测试

- storage：两表 CRUD/快照回读/WorkflowRunID 关联（`internal/storage/workflow_test.go`）；
- server：编排器推进（成功链/中间失败停/continue_on_error 跳过/retry 第二次成功/
  run 取消停推进/pending Job 取消/running Job 取消 409/同 workflow 重入 409/
  定义更新 active run 409/校验各 400）——agent 用假 provider 注入（`cov_jobs_test`
  先例）；jobs 过滤参数（`cov_jobs_test.go` 扩）；
- web：Workflows 页三态 + 步骤编辑器校验 + run 时间线轮询；Jobs 页异步语义与取消；
- 覆盖率：新增分支按 100% 纪律收口，不可达防御按既有格式登记
  `tool/known_uncoverable.txt`。

## 里程碑切分

- **M1（本设计）**：异步 Job + 线性 Workflow + 取消（pending 语义）+
  台账过滤 + 权限/审计。验收：探针跑通成功链/失败停/重试/取消四场景 +
  真机连一台 agent 编排一轮。
- **M2（候选，另行立项）**：步骤变量传递（`{{steps.N.output}}` 注入命令）、
  agent 侧取消信号（RPC 加 cancel 帧 + provider 监听）、多目标扇出（同步骤
  跑 N 台 agent）、DAG 依赖、审批门、SSE 推送。
- **不做**：定时触发器（W11 约定）、输出实时流（P1 边界节留远控域统一设计）、
  Workflow 嵌套 Workflow（递归执行面先不碰）。

## 边界自检（战略评审两问）

- 是否重造底层？否——编排器是 server 侧一个 goroutine 状态机，调度底层
  仍是 OS 进程 + 既有 RPC 通道；没有引入队列/调度器中间件。
- 走统一 Job 模型还是又开一条执行通道？统一——步骤即 Job，进同一台账、
  同一权限、同一审计；Workflow API 只是编排层的 CRUD + run 控制。
