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

- **M1 已实现（2026-10-09）**：异步 Job（W1/W2/W3 单条语义）+ 线性 Workflow
  （W4-W9）+ 取消 + 台账过滤 + 权限/审计，全链 `internal/storage/workflow.go`、
  `internal/server/api_workflow.go`、`web/src/pages/Workflows/`、
  `web/src/services/workflows.ts`；POST /api/jobs 全异步化（W1）为破坏性语义
  变更（原同步等待删除，消费者仅 web 一处）。真机验收按「验收清单」Workflow
  节推进（探针四场景：成功链/失败停/重试/取消）。
- **M2a 已实现（2026-10-10）**：步骤变量传递（M2 候选清单第一项），设计见
  「M2 立项：步骤变量传递」节，824eb88 立项、97696ad 落地（含连带修复：
  dispatchJob 改传解析后参数，见 V3）。
- **M2b 已实现（2026-10-10）**：多目标扇出（M2 候选第二项），设计见「M2 立项：
  多目标扇出」节——server 侧顺序扇出、agent 零改动；web 步骤编辑器多选 +
  run 详情「N 台」聚合展开逐台明细同批落地；真机验收探针扩至 W10（双台
  扇出成功/失败停/ghost 混入，12/12 PASS）。剩余候选（取消信号/DAG/审批门/
  SSE）仍为候选，未排期。
- M2+ 其余各项在「边界（明确不做）」；未实现项以本文设计为准。
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
- **M2（候选，另行立项）**：步骤变量传递（见下节 M2a 立项）、多目标扇出
  （见 M2b 立项）、取消信号（见 M2c 立项）、DAG 依赖、审批门、SSE 推送。
  已开工 M2a/M2b/M2c；DAG 维持 W4 压后等真实并行需求，审批门因孤儿 run
  问题未解不开工，SSE 与全仓轮询模式冲突拍不做。
- **不做**：定时触发器（W11 约定）、输出实时流（P1 边界节留远控域统一设计）、
  Workflow 嵌套 Workflow（递归执行面先不碰）。

## M2 立项：步骤变量传递（M2a，2026-10-10）

线性链最真实的缺口：后步要用前步产物（扫描输出喂给处理命令、备份路径
喂给校验命令），现在只能落盘再读或整段复制粘贴。变量传递把输出在前步
Job 与后步命令之间接起来——纯 server 侧编排器增强，agent 零改动，无
schema 变更，是 M2 候选里最小的高价值增量。

### 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| V1 | 语法 | <span v-pre>`{{steps.NAME.output}}`</span>，NAME 为步骤 name（`[A-Za-z0-9_-]+`）；仅此一种形态 | 定义校验已保证 name 唯一；按名引用比按序号（原 M2 草案 `steps.N`）抗步骤重排，报错可读。`exit_code` 等其他字段暂不做，需要再加 |
| V2 | 校验时机 | 定义保存时（`validateWorkflowPayload`）：引用的 name 必须存在于**更靠前**的步骤；未知名/前向引用 400 | 顺序链语义下前向引用必然拿到空值，保存期拒绝比运行期失败诚实；自引用同理 |
| V3 | 替换时机 | 步骤开始前（进入重试循环前）解析一次，作用于 `sanitizeStepParams` 之后的参数树（递归 map/slice，仅 string 值；编排元键 name/retry/continue_on_error 不参与） | 重试各 attempt 用同一解析结果，语义稳定；目标字段 target 是资源 ID 不做替换。**连带修复 M1 遗留缺陷**：dispatchJob 原先传定义原参数——meta 键漏到 agent 且模板不解析（落库的是 sanitized 版，两处不一致）；现 dispatch 与落库同用解析后参数 |
| V4 | 取值 | 前步最终 attempt 的 Job.Output 原文（≤64KB，尾部截断存储值）；源步骤失败（continue_on_error 放行）时用其已存输出（可能为空或错误输出）——与 W4 组合语义一致 | 64KB 是 agent 侧既有截断面，不新增上限；「引用失败步」是 continue_on_error 的自然延伸 |
| V5 | 替换后再校验 | 解析结果过一遍 `validateJobPayload`，不过则该步直接 failed（快照记 stopReason），不派发 | 变量注入可能把命令撑过 16KB 上限；本地拒绝比发到 agent 再失败省一轮 RPC，行为确定 |
| V6 | 字面量边界 | 仅精确匹配 <span v-pre>`{{steps.NAME.output}}`</span> 的 token 参与替换；其余 <span v-pre>`{{...}}`</span> 形态原样保留 | shell 里的 `{`、JSON 模板等不受影响；想写引用但名字拼错会在 V2 定义期被拦（unknown name），不会静默变字面量 |

### 不变式

- 定义文件（workflows 表 steps 列）保存**模板原文**；解析结果只落在该步
  创建的 Job 行 Parameters 里（「快照即事实」W6 同款）。
- run 步骤快照不重复存解析结果——Job 台账可查（W5）。
- agent 侧零改动：`job.exec` 收到的就是已解析命令。

### 前端

零改动。步骤编辑器编辑的就是模板原文，替换对 UI 透明；提示文案后续有
真实使用反馈再加。

### 测试

- server（`cov_workflow` 系）：定义期未知引用/前向引用 400；两步链解析
  （假 provider 断言第二步 Job.Parameters 含第一步输出）；嵌套参数替换；
  非 <span v-pre>`{{steps.*.output}}`</span> 的 <span v-pre>`{{...}}`</span> 原样保留；替换后超 16KB 步骤直接
  failed 且无 Job 派发；引用失败步（continue_on_error）取错误输出。
- 全部可达，无新增 known_uncoverable 登记（预计）。

## M2 立项：多目标扇出（M2b，2026-10-10）

单步只能打一台 agent，N 台同构操作（全机升级/批量重启/全量巡检）只能复制
步骤或裸跑 N 次。扇出让一个步骤对 N 台 agent 各建真实 Job——server 侧编排
增强，agent 零改动、协议零新增（与 M2a 同款边界）。选中它而非取消信号/
DAG/审批门/SSE 的依据：logs M3 已验证过「server 并行扇出 + 降级归因」模式
（agent 零改动），本批把它带进变更面编排；取消信号已另立 M2c（C1-C9）
推进——RPC 协议面无结构新增（复用 rpc_request 帧 + router 点号方法），
需重构 agent 读循环为并发派发，但 agent/server 同仓同发无兼容包袱；DAG
被 W4 明确压后等真实并行需求；审批门有 server 重启孤儿 run 问题（等待态
goroutine 不可复活）未解不开工；SSE 与全仓轮询模式冲突拍不做。

### 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| F1 | 语法 | 步骤级可选 `targets: []string`（仅 workflow steps；standalone Job API 的单 `target` 不动）；`target` 与 `targets` 互斥（同现 400）；条目 trim 后非空、去重、≤20 台 | 单目标仍是绝大多数场景，保持主形态；数组直白；20 台=个人 infra 规模上限（WorkflowMaxSteps 同量级的 sanity 闸） |
| F2 | 执行序 | **顺序扇出**：逐台完整跑（含各自重试周期），非并行 | 编排 goroutine 同步模型不变、零并发协调；变更面（exec 写操作）与 logs M3 查询面不同，并行扇出放大器在写路径不可接受；N 台最坏 N×(retry+1)×300s 可控 |
| F3 | 失败策略 | 无 continue_on_error：首台失败即停步（后续目标不跑）；带 continue_on_error：跑完全部目标，步状态=失败但链继续 | 与 W5 失败停/继续语义同构，target 维度复用同一开关，无新概念 |
| F4 | 重试 | 按目标独立完整重试周期；每次 attempt 仍是独立 Job 行 | W4「尝试历史进台账」逐台保留 |
| F5 | 取消 | 目标边界检查取消（与既有步骤边界同机制，查 run.Status）；未跑目标记 `cancelled` + stopReason=`run cancelled` | W3「停止推进后续步骤」在更细粒度如实兑现；cancelled 是既有真实态 |
| F6 | 快照 | 步骤行加 `targets[]`（agentId/status/jobId/attempts/stopReason，omitempty）；步级 Status=聚合（全成功才 success，否则 failed）；单目标步快照形状不变（Target/JobID 仍在步级） | 存量 run JSON 向前兼容（omitempty）；聚合态让时间线不改也自洽；run 详情展开按目标明细 |
| F7 | 输出 | 扇出步**不提供**步骤输出：`{{steps.NAME.output}}` 解析为空串 | 多台输出拼接是伪精度；按目标取输出（per-target refs）等真实需求再立项；空串与 M2a「缺名取空串」同语义 |
| F8 | 在线校验 | 建 run 前逐台校验 targets 全在线（既有幽灵防线同口径），缺一 503 并指明 steps[i] 的缺席者 | 与单目标 `registry.Get` 同款，扇出只是 N 次 |

### 不变式

- 定义文件存 `targets` 原文；每台 attempt 落独立 Job 行（快照即事实 W6）。
- 单目标路径（`target` 字段）行为逐字节不变——既有 run/探针场景零感知。
- agent 零改动：每台收到的仍是自己的 `job.exec`。

### 前端

- 步骤编辑器：目标主机 Select 加多选切换（选多台即扇出形态，提交折成
  `targets`）；单选仍提交 `target`。
- run 详情时间线：扇出步行内「目标」列显示聚合（如 `3 台`），展开行按
  目标列明细（agentId/状态/尝试/输出入口）。

### 测试

- server（`cov_workflow` 系）：targets 校验矩阵（互斥/空/重复/超 20/空条目）；
  两台顺序扇出成功聚合 success；首台失败停步（第二台无 Job）；continue_on_error
  跑完全部且步 failed 链继续；按目标重试（首台 retry 后成功）；目标边界取消
  （第二台记 cancelled）；建 run 在线校验缺一 503；扇出步输出引用解析空串；
  单目标步快照形状回归（无 targets 键）。
- web：编辑器双形态折返（target/targets）、时间线扇出步展开渲染。

## M2 立项：取消信号（M2c，2026-10-10）

W3 的取消只停推进：run 置 cancelled 后编排器在下一步边界停，**在途步骤
的 Job 不被触碰**——失控命令要跑满 agent 侧超时（上限 300s）才自然结束。
M2c 把取消做成真信号：server 下发 `job.cancel` RPC，agent 击杀在途进程
（进程组 SIGKILL，孙进程一并死），Job 落 `cancelled` 终态。选中它而非
DAG/审批门/SSE 的依据（2026-10-10 巡检拍板）：DAG 维持 W4 压后（等真实
并行需求）；审批门有 server 重启孤儿 run 问题（等待态 goroutine 不可复活）
未解不开工；SSE 与全仓轮询模式冲突拍不做。取消信号是 M2 候选里唯一
「用户可感知的失控止血」增量。

### 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| C1 | 信号载体 | `job.cancel` RPC 方法（params `{id}`），复用既有 `rpc_request` 帧，**不新增消息类型** | probe_report（B5）先例是 agent→server 主动上报、无既有通道才加消息类型；取消是 server→agent 指令，`rpc_request` 就是该方向标准载体，router `parseMethod` 已支持点号方法（`job.cancel` → provider=job, action=cancel），零协议结构新增 |
| C2 | agent 读循环并发化 | `handleRPCRequest` 改 goroutine 派发（`go a.handleRPCRequest(msg)`） | **根本前提**：`messageLoop` 单 goroutine 串行，`job.exec` 阻塞期间（最长 300s）任何后续帧——含 cancel——都读不到。RPC 响应按消息 ID 相关（pending response map 互不干扰），`sendMessage`→`upstream.Enqueue` 已 goroutine 安全（writeLoop 串行写出），并发派发不破坏既有契约 |
| C3 | agent 句柄表 | `JobProvider` 加 `map[jobID]*jobSession`（ctx cancel + mutex），镜像 logs follow 的 `followState.follows` 模式 | logs_follow 已验证「注册句柄 + 按 ID 查找 + 幂等关闭」；exec 的 ctx cancel 触发 `newJobCmd` 既有 `cmd.Cancel`（进程组 SIGKILL），**复用超时同款击杀路径**，不引入第二套 kill 机制 |
| C4 | 击杀语义 | unix 进程组 SIGKILL（`Kill(-pid)`）；Windows 直接子进程 | `newJobCmd` 已实现（Setpgid + 组杀），超时路径同款；`sh -c` 派生的孙进程一并死，无孤儿。Windows 无进程组语义，维持既有直接子进程击杀 |
| C5 | 幂等语义 | 未知/已结束 ID 的 cancel 返回成功空操作（`{cancelled:false}`），镜像 `FollowStop` | cancel 与 exec 完成存在天然竞态（job 恰在 cancel 到达前结束）；报错会让 server 无法区分「已停」与「早已结束」，幂等成功使 server 侧语义单一 |
| C6 | server 单 Job 取消 | running 可撤：`CallAgent(job.cancel)` → 成功后**条件 UPDATE**（`WHERE status='running'`）落 cancelled；agent 不可达 → 409 如实拒绝。pending 维持 W3（DB-only），终态一律 409 | 「观察终态 ⟹ 状态可信」：只有 agent 确认击杀才落 cancelled，绝不假称已停；条件 UPDATE 防 check-then-update 竞态（GetJob 与 CancelJob 之间 pending→running 翻转窗口） |
| C7 | 终态回写不覆盖取消 | `dispatchJobAsync` / `runWorkflowStep` 回写前**重读** job，若已 cancelled 则保留 cancelled（仅补 output/exitCode/error） | cancel 写与 exec 回写并发：exec 被击杀后返回 exit -1 / "signal: killed"，若无守卫会把 cancelled 覆盖成 failed；重读收敛竞态，无需加锁串行化 |
| C8 | run 级取消联动 | `handleWorkflowRunCancel` 除置 run cancelled 外，查在途 step Job（`ListJobsFiltered(status=running, runID)`）逐个走 C6 击杀路径；在途步回写命中 C7 守卫，步骤快照记 cancelled + stopReason=`run cancelled`（F5 同口径） | W3 只停推进、失控命令跑满超时——M2c 的价值就在「真正停掉在途命令」；run 取消是用户最高频的「停下来」入口 |
| C9 | 步骤 cancelled ⟹ run cancelled | `advanceWorkflow` 单步分支补 cancelled 守卫（镜像扇出分支 F5 的 return）；若 run 仍 running（单 Job 取消场景）则 `finishWorkflowRun(cancelled)` | 步骤 Job 死则链断，run 不得僵尸悬挂（永不落终态）；单 Job 取消传播到 run 是直觉语义——用户杀了某步，期望整条链停 |

### 不变式

- agent 句柄表只在 exec 生命周期内持有（注册→执行→注销）；cancel 不新建
  状态机，ctx cancel 即全部。注销顺序 **cancel-then-unregister**（LIFO
  defer），防「已注销未击杀」微秒窗口漏杀。
- `cancelled` 是 Job 既有真实终态（storage W3 已立）；cancel 写与 exec
  回写的竞态由条件 UPDATE + 重读收敛，终态以库内为准。
- run 终态：cancelled run 不再被 `finishWorkflowRun` 改写——advanceWorkflow
  步骤边界检查 + C9 cancelled 守卫双保险；run 取消处理器自身 audit-then-
  update（与 finishWorkflowRun 写序不变式同款：观察终态 ⟹ 无后续写）。
- 审计动作不变（`job_cancel` / `workflow_cancel` 既有）；run 取消联动击杀
  的 step Job 不另发 cancel 审计（其 exec 回写已有 `job_run` 审计）。

### 前端

- Jobs 页取消按钮对 running job 现真生效（原仅 pending 可点）；按钮/文案
  零改动——行为升级对 UI 透明。
- run 时间线：在途步被击杀后显示 cancelled + stopReason `run cancelled`
  （复用 F5 渲染路径，零新增组件）。

### 测试

- agent（`rpc` 包）：exec 注册句柄 → cancel 击杀在途命令（真 `sleep` +
  断言提前返回）；未知 ID 幂等成功；exec 自然结束后 cancel 幂等；并发
  exec+cancel（`-race`）；`job.exec` 无 id 参数（旧调用形态）不注册、
  cancel 空操作。
- server（`cov_jobs` / `cov_workflow` 系）：running job cancel → 假 agent
  收 `job.cancel` → job cancelled + 输出仍回写；agent 不可达 → 409 不落
  cancelled；exec 回写不覆盖 cancelled（C7 守卫）；pending→running 翻转
  竞态走条件 UPDATE；run cancel 联动击杀在途 step（C8）；单步 Job 被杀
  → run 落 cancelled（C9）；扇出步目标 Job 被杀 → 步 cancelled。
- 全部确定性可达（假 agent handler 按 method 分流 + channel 编排时序），
  预计无新增 known_uncoverable 登记。

## 边界自检（战略评审两问）

- 是否重造底层？否——编排器是 server 侧一个 goroutine 状态机，调度底层
  仍是 OS 进程 + 既有 RPC 通道；没有引入队列/调度器中间件。
- 走统一 Job 模型还是又开一条执行通道？统一——步骤即 Job，进同一台账、
  同一权限、同一审计；Workflow API 只是编排层的 CRUD + run 控制。
