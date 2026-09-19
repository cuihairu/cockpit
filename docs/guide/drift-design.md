# 防漂移检测（Drift Detection）设计

> 状态：M1 方案定稿（2026-09-15）｜ 参考：todo.md「drift 漂移视图」条目
> （NetBox 期望态/实际态分离），主体实现为**配置漂移**：面板写路径产生
> 基线，与磁盘当前内容比对。

## 痛点与定位

自建云服务器上「面板改的配置被手改/误删」是高频翻车源：手改 nginx 片段后
下次面板保存静默覆盖；cockpit 名下 cron 任务被人为删掉无人知晓；stack 的
compose.yml 被直接编辑导致下次 up 行为与页面所见不符。

Cockpit 已有三大配置写路径（nginx 站点片段 / cron cockpit 段 / stack
compose 文件），**写入现场天然产生期望态**——基线在写成功那一刻记录，无
需用户额外声明。检测则是纯 pull：用户主动检查，实时读盘比对。

```
写路径（面板保存）                    检查（按需）
nginx ApplySite ──Record──┐          GET /drift/check
cron ApplyJob/DeleteJob ──┤ baseline │  当前内容 sha256
stack SaveCompose ────────┘  (JSON)  ←逐项对比→  vs 基线 sha256
                                     → ok / drifted / missing / no_baseline
```

## 决策

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D1 | M1 边界 | 按需检查（pull），基线只读不改 | 定时巡检 + 告警通知、手动登记基线、inventory 字段一致性（hostname/IP）留 M2 |
| D2 | 检测对象 | 仅 cockpit 名下对象：nginx `cockpit-site-*.conf` 片段 / cron cockpit 任务段 / stack `compose.yml` + `.env` | 与 nginx「用户已有配置零接触」纪律一致；外部条目不检测 |
| D3 | 基线存储 | agent 本地 JSON `/var/lib/cockpit/drift-baseline.json`（`COCKPIT_DRIFT_BASELINE` 覆盖），mutex + tmp+rename 原子写，0600 | 基线跟着 agent 走；server 不落库（转发纪律）；写路径与基线同进程无网络往返，无不一致窗口 |
| D4 | 条目 key | `nginx/<name>`、`cron/cockpit`、`stack/<name>/compose.yml`、`stack/<name>/.env`；每条目 `{sha256, updated_at}` | 文件粒度，问题定位直接 |
| D5 | 状态 | `ok`（hash 一致）/ `drifted`（基线在、当前在、内容变）/ `missing`（基线在、文件没了）/ `no_baseline`（文件在、无基线——面板外存量对象）；实现补充：`error`（读取失败，不影响其他类）、`none`（cron 空段且无基线，不产生噪音条目） | `no_baseline` 提示通过面板保存一次即自动登记；删除对象走 Forget 不产生 missing |
| D6 | check 算法 | 实时读当前内容算 sha256 与基线比对；nginx **对文件原始字节** hash（meta 注释被改坏同样算漂移）；cron 对 `splitCockpit` 解析出的任务列表 marshal hash（meta 行损坏会掉出任务列表 → hash 变 → 漂移）；stack 逐文件 | 不解析语义，字节级比对最诚实 |
| D7 | 挂钩注入 | `BaselineRecorder` 接口（`Record(kind, name, content)` / `Forget(kind, name)`）注入三个 provider，字段零值 nil 不记录 | provider 相互独立注册，接口注入改动最小；测试可注入 fake |
| D8 | 挂钩时机 | **写成功后** Record（nginx 在 reload 成功后；reload 失败回滚不记）；删除成功后 Forget（nginx DeleteSite / stack 删除目录 / cron DeleteJob） | 基线 = 线上实际生效的内容 |
| D9 | capability | `drift`：nginx-proxy / cron / docker-api（stack）**任一存在**即注册 | 三者全无则 check 无从谈起 |
| D10 | RPC | `drift.check` 单方法（POST），无用户输入参数 | 校验面为零；结果一次性全量返回 |
| D11 | server | `POST /api/agents/{id}/drift/check` 纯转发，不落库；浏览类不记审计 | 与 cron/logs 转发纪律一致 |
| D12 | Web | `/drift` 页面：agent 选择 + 「检查」按钮 + 汇总（漂移 N 项）+ 四态清单（类型 Tag / 名称 / 状态 Tag / 基线与当前 hash 短码） | 参照 /cron 页面结构 |

## RPC

| 方法 | 参数 | 返回 |
|------|------|------|
| `drift.check` | `{}` | `{items: [{kind, name, status, baseline_sha, current_sha}], checked_at}` |
| `drift.diff` | `{kind, name}` | `{expected, current}`（两侧全文；cron 侧为 MarshalIndent 美化后的 cockpit 段） |
| `drift.record` | `{kind, name}` | `{recorded: true, sha256}`（以当前内容登记基线，M4） |

status 语义见 D5；`baseline_sha` / `current_sha` 空串表示对应侧不存在。

## M2：定时巡检 + 漂移告警（2026-09-15）

把 M1 的「按需检查」升级为「自动发现」：server 侧定时对在线且带 drift
capability 的 agent 执行 drift.check，发现漂移即产生告警（复用 M2 拨测
的告警去重与通知管道），agent 侧零变更。

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D13 | 巡检循环 | server 侧 `driftScanLoop`（ctx+ticker 同 alert_loop 模式），每分钟醒来对比「距上次扫描 ≥ 间隔」再扫；间隔可动态改，故用固定 1min ticker + 时间戳对比 | probe runner 的 maybePrune/lastPrune 同款；lastScan 仅循环 goroutine 访问无锁 |
| D14 | 巡检目标 | `registry.ListByCapability("drift")` 全部在线 agent，逐个 `CallAgent(drift.check)`；失败记日志跳过下轮再试 | 单 agent 失败不影响其他；离线由既有 offline 告警覆盖 |
| D15 | 告警条件 | items 含 `drifted` 或 `missing` → 每 agent 一条汇总告警；`error` 只记日志（可能是权限问题，告警会噪音）；`no_baseline` 不告警（存量对象）；全 ok 无动作 | 汇总到 agent 级控制告警风暴；title 固定 → 同主机未读存在期间只报一次（D15 去重语义：用户标已读后再次漂移会重新告警） |
| D16 | 告警形态 | alert 包公开 `CheckDriftScan(agentID, hostname, driftedNames)`：非空则 `createAlertIfNotExists("warning", "配置漂移：主机 X", 明细列表, agentID, "agent")`；明细最多 10 行 + 截断提示 | 复用拨测 M2 的真去重（HasUnreadAlert）与非阻塞多渠道通知 |
| D17 | 配置 | Setting 键 `drift.scan_interval_seconds`，[0, 86400]，0=关闭巡检，默认 1800（30 分钟）；REST `GET/PUT /api/agents/.../drift/config`？——不，配置是全局非 per-agent：`GET/PUT /api/drift/config` | 与 probe interval 同一 Setting 模式；每次醒来读 Setting 免缓存（loadThresholds 同款） |
| D18 | Web | `/drift` 页面顶部加「自动巡检」设置行（开关 + 间隔分钟数，保存 PUT /api/drift/config）；巡检开启时页面提示自动发现可用 | 配置入口放使用场景里，不进全局 Settings |

不落库：巡检结果不存储（告警即记录），/drift 页面保持实时手动检查语义。

## M3：漂移 diff 视图（2026-09-18）

drifted 条目从「变没变 + hash 短码」升级为「哪里变了」：基线补存原文 +
`drift.diff` 取两侧全文 + web 行级 diff 渲染。

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D19 | 基线存原文 | `BaselineEntry` 加 `Content string`（omitempty；`Record` 时随 hash 一并写入，单条 >256KB 只存 hash 不存原文）；旧基线无原文时 `drift.diff` 明确报「基线无原文（旧版本记录），到对应管理页重新保存一次即可」 | Record 本来就拿到 content，零额外读；写路径下次保存自动补原文，零迁移；基线文件仍 0600 本地；.env 等内容经 diff 返回与文件管理器 read 同权限面（JWT + agent 在线），无新增泄露 |
| D20 | `drift.diff` | `{kind, name}`：kind 白名单 {nginx, cron, stack}；expected=基线原文，current=实时读（nginx 片段文件 / stack compose.yml 与 .env / cron 读 crontab 后 splitCockpit 取 jobs）；任一侧缺失或 >256KB 拒绝并报原因；**cron 两侧返回前 MarshalIndent 美化**（hash/基线层保持 compact `json.Marshal` 不动——否则升级后旧基线 hash 失配全量误报 drifted；美化只在 diff 展示层） | current 的取法与 check/Record 完全同源（同 marshal），保证 diff 与 hash 判定一致；JSON 美化让 cron 段可按行 diff |
| D21 | server 端点 | `POST /api/agents/{id}/drift/diff`，body `{kind, name}`：kind 白名单 + name 非空 ≤128 双端同规则校验后转发 `drift.diff`；不审计（浏览性质，与 check 一致） | forwardDriftRPC 复用 |
| D22 | Web diff 渲染 | drifted 行加「差异」按钮 → Modal：自写 LCS 行级 diff（双方先按行截断 1000 行并标注「内容过长，仅对比前 1000 行」）；统一 diff 着色（del 红=基线有当前无，add 绿=当前新增，same 灰）；页脚提示「核对后到对应管理页重新保存可消除漂移」 | 自写 LCS（整数键 + DP 全表 + 回溯，1000 行 O(n²)≤10⁶ 可接受）避免引入依赖；不做双栏对照（行级统一 diff 更适合配置段落场景；后被 M5/D26 推翻） |

### Agent 侧

- `drift.diff`：kind/name 校验 → 基线取条目（无 → 报错「无基线」；有但 Content
  空 → 报错「基线无原文」）→ 按 kind 读 current（读法与 check 同源）→ 两侧
  >256KB 报错 → cron 两侧 MarshalIndent → 返回；
- `Record` 存原文：`update()` 条目带 Content（截断逻辑在 Record 内，不动
  Forget/hash 语义）。

### Server 侧

- `handleAgentDriftAPI` 加 `sub == "diff"`：POST + body 解码 + kind 白名单
  {nginx,cron,stack} + name 非空 ≤128 → `forwardDriftRPC(..., "drift.diff", params)`。

### Web 侧

- `types` 加 `DriftDiffResult {expected, current}`；`api.driftDiff(agentId, kind, name)`；
- `web/src/utils/lineDiff.ts`：LCS 行级 diff（入参两侧字符串，先 split('\n') 截断
  1000 行 + truncated 标记，输出 `{type: 'same'|'del'|'add', text}[]`）；
- Drift 页：drifted 行「差异」按钮 → Modal（加载中 Spin / 失败 Alert 透出 agent
  原因 / diff 行渲染等宽 + 行号 + 着色 + 截断提示 + 页脚操作指引）。

**测试**：agent——Record 存原文与截断、diff 全流程（nginx 手改 / cron 美化与
同源 / stack 两文件）、旧基线无原文报错、kind/name 校验、两侧过大拒绝；
server——转发与 400 校验；web——tsc + build。

## M4：手动登记基线「以当前为准」（2026-09-18）

把「基线只来自面板写路径」补全为「也可来自用户确认」：面板外存量对象
（no_baseline）一键纳入检测；手改后的 drifted 对象确认无误后一键以当前
内容为新标准。agent 侧复用 M3 的 currentContent + Record，零新存储。

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D23 | `drift.record` | `{kind, name}`：kind/name 复用 M3 校验 → 实时读当前内容（与 check/diff 同源 `currentContent`，cron 为 splitCockpit→compact marshal）→ `baseline.Record` 登记原文；适用 no_baseline（纳入检测）与 drifted（确认手改为新标准）；missing/error 场景读不到当前内容直接报错（无需客户端状态白名单）；>256KB 照 Record 语义只存 hash | 全部复用 M3 积木（validDriftTarget + currentContent + Record）；「以当前为准」与面板重新保存互补——前者以磁盘为准（不重写文件），后者以面板为准（重写文件） |
| D24 | server 端点 + 审计 | `POST /api/agents/{id}/drift/record`：与 diff 同规则校验（kind 白名单 + name 非空 ≤128）后转发；**记审计** `drift_record` / `drift_baseline`，resourceID=`kind/name`，detail 记 agent | check/diff 是浏览不审计，record 是用户主动变更漂移判定标准——之后该对象「漂不漂」以此为准，必须可追溯（与文件变更审计同档） |
| D25 | Web 交互 | drifted 行「以当前为准」+ no_baseline 行「登记」按钮（Popconfirm：「之后的漂移检测将以当前磁盘内容为标准」）；成功后自动重查刷新清单；missing/error 行不出按钮 | 操作入口放结果行内就地闭环；两个文案同一 RPC，按行状态选择语义更准的动词 |

### Agent 侧

- `drift.record`：`validDriftTarget` → `currentContent`（读失败透传原因）→
  `baseline.Record` → 返回 `{recorded: true, sha256}`（登记后 hash，供前端提示）。

### Server 侧

- `handleAgentDriftAPI` 加 `sub == "record"`：与 diff 共用 body 校验 →
  `forwardDriftRPC(..., "drift.record", params)` → 成功后 `auditDriftRecord`
  （username/IP/UA 同 auditFile 模式，detail 记 agent）。

### Web 侧

- `api.driftRecord(agentId, kind, name)`；操作列按状态出「差异」（drifted）/
  「以当前为准」（drifted）/「登记」（no_baseline）；Popconfirm 确认 → 调用 →
  message 成功 → 自动重跑 check 刷新清单（失败 Alert 透出 agent 原因）。

**测试**：agent——no_baseline 登记→check 变 ok（nginx/cron/stack）、drifted
以当前为准→ok 且基线原文更新、读失败报错、校验拒绝；server——转发参数、
400 校验不达 agent、审计落库；web——tsc + build。

## M5：双栏对照 diff 与配置着色（2026-09-19）

推翻 D22「不做双栏对照」的范围决策（行级统一 diff 在增删交错时对照不
直观）。纯 web 升级，零 Go 改动。

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D26 | 双栏对照视图 | DiffModal 加 Segmented「对照 / 统一」切换（默认对照）；`pairDiffLines` 把统一 diff 转行对：same 直通双栏，连续 del/add 块内按序一一配对，多出的一侧独占行；缺位半边画底纹 | 配对在渲染层完成，lineDiff 的 LCS 输出不动；统一视图保留（窄窗口与整段复制场景） |
| D27 | 配置轻量着色 | `configHighlight.ts` 单行无状态词法：注释行 / 行首键（yaml `key:`、env `KEY=`、json `"key":` 带引号整体）/ nginx 首词指令（仅 `word …;` 与 `word … {` 形态才认）/ 引号串 / 独立数字（两侧不贴词字符或点）/ `{};` 标点；不做跨行解析状态、不猜行内 `#` 注释 | 宁缺毋错：覆盖 cockpit 下发四类内容（nginx conf、Traefik/compose YAML、cron JSON、.env）足矣；不引语法高亮依赖 |

### Web 侧

- `lineDiff.ts` 加 `pairDiffLines`（统一 diff → 双栏行对）；新增
  `configHighlight.ts`（着色词法，返回 span 序列）；
- DiffModal：Segmented 视图切换（默认对照）+ 两种视图共用着色与截断
  提示；Modal 宽度 960→1080（双栏需要更宽）。

**测试**：web——tsc + build + tsx 运行时校验（注释行 / yaml 键 / nginx 指令
与端口数字 / env 键值 / JSON 键与串值 / `hunter2` 值不误染 / 行内 `#` 不猜
注释；双栏配对 del/add 对齐、多出一侧独占行）。

## M6：CMDB 一致性——inventory 声明 vs agent 实报（2026-09-19）

「不做」清单的末项正式立项，也是 drift 主线（配置漂移）到 CMDB 主线
（期望态 vs 实际态）的收口：配置漂移管「面板下发的文件有没有被改」，
CMDB 一致性管「Git 里声明的机器还是不是那台机器」。数据事实决定形态——
声明态只活在 inventory YAML（watcher 热加载的内存副本），实况 hostname/IP
由注册覆写同一 DB 行（`persistRegisteredAgent` → `UpsertAgent`），声明值
并无持久化副本。因此**不建新表、不起巡检循环**，按需读模型比对。

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D28 | 按需读模型 | `GET /api/inventory/consistency`：`watcher.GetInventory()`（声明）× `db.ListAgents()`（实报）即时比对，零新持久化、零巡检循环、不进告警框架 | 一致性是 CMDB 卫生项不是事故，「UI 高亮」即诉求（P2 原文）；inventory watcher 已保证声明态新鲜度；起循环只会产出待读状态的滞后副本 |
| D29 | 比对语义 | 仅声明非空才比对；hostname **大小写不敏感**（DNS 语义）+ TrimSpace，IP 精确串匹配（不做 CIDR/多 IP 语义——inventory v1 就是单串）；四态：`ok` / `mismatch`（含 field 级明细）/ `unregistered`（声明了但库里没有——未上线或声明先于装机）/ `undeclared`（库里有但 YAML 没声明——CMDB 完整性缺口） | 大小写归一是唯一豁免：主机名在解析层本就不分大小写，报出来是噪声；其余宁可严格，误报比漏报有用于 CMDB |
| D30 | API + UI | server 侧 `handleInventoryConsistency`（JWT，浏览类不审计，同 drift check 口径；`inventorySync == nil` 时 503 指名引导 `inventory.path`）；比对纯函数落 `internal/inventory`（输入 `*Inventory` + `[]*storage.Agent`，产出报告，无 IO 可全表测）；web 漂移页加 Tabs「配置漂移 / CMDB 一致性」，一致性 Tab 出汇总条 + 四态表（mismatch 行展开声明 vs 实报对照） | 纯函数与端点解耦，watcher 无关即可测；UI 挂漂移页——两者同属「期望 vs 实际」主题，不为单一报表新建页面 |

### Server 侧

- `internal/inventory/consistency.go`：`AgentConsistency` 纯比对 +
  `CompareAgents(inv, agents)` 报告（summary 四态计数 + 按 id 排序明细）；
- `internal/sync` Manager 加 `Consistency(db)` 薄封装（GetInventory + 比对）；
- `api_inventory.go`：路由 `/inventory/consistency`。

### Web 侧

- `api.getInventoryConsistency()`；漂移页 Tabs 化：原内容为「配置漂移」，
  新「CMDB 一致性」Tab（useQuery 30s staleTime，四态 Tag + mismatch 明细
  Tooltip 对照声明/实报）。

**测试**：inventory——比对纯函数表驱动（ok/hostname 大小写/IP 不符/
多字段同报/unregistered/undeclared/声明空跳过）；server——端点 wiring
（200 形态、nil manager 503、JWT 外校验不涉及）；web——tsc 零新增 + build。

## M6 清单

- [x] inventory：`consistency.go` 比对纯函数（四态 + field 级 mismatch +
      summary 计数 + id 稳定排序）+ 表驱动单测
- [x] sync：`Manager.Consistency()`（GetInventory × ListAgents 即时比对）
- [x] server：`GET /api/inventory/consistency`（JWT，浏览类不审计；
      `inventorySync == nil` 503 指名引导 `inventory.path`）+ 端点测试
- [x] web：types + `api.getInventoryConsistency()` + 漂移页 Tabs 化 +
      `ConsistencyTab`（汇总条 + 四态表 + mismatch 行展开对照）
- [x] 测试：inventory 3 测试 + server 2 测试全绿（-race 全包过）；
      web tsc 零新增（基线 9 不变）+ build 过
- [x] 文档收尾（本清单勾选）+ todo.md 同步

✅ M6 完成（2026-09-19）。落地差异补记：① `Consistency()` 为无参方法
（db 由 `NewManager` 构造时持有），非设计行文的 `Consistency(db)`；②
mismatch 对照落地以**行展开**（字段级子表：字段/声明值/实报值）为准，
「Web 侧」行文的 Tooltip 只承载四态状态语义，不做 Tooltip 塞对照；③
Tabs 化随附卡片标题「漂移检测」→「漂移与一致性」（两 Tab 同属期望 vs
实际主题）；④ 未启用 inventory 的 503 文案在 web 侧以引导 Alert 透出
（含 `inventory.path` 指引），与 server 端点文案同口径。

## 不做（后续版本）

- nginx 非 cockpit 片段、外部 crontab 条目、docker 卷内容检测。

## M1 清单

- [x] agent：`drift_provider.go`（baseline 存储读写 + check）+ nginx/cron/stack
      三 provider 挂钩 + `drift` capability + providers.go 接线
- [x] server：`api_drift.go`（check 转发）+ serveAPI 接入
- [x] web：`/drift` 页面 + 路由与菜单
- [x] 测试：baseline 原子读写、四态判定（nginx/cron/stack）、挂钩注入、
      server 转发与离线 503
- [x] 文档收尾（本清单勾选）+ todo.md 同步

✅ M1 完成（2026-09-15）：agent 8 测试 + server 3 测试全绿；基线挂钩验证覆盖
「面板写入成功 → 基线登记」「校验失败 → 不登记」「外部条目变更 → 不算漂移」；
cron 基线只对 cockpit 任务段 hash（外部 crontab 条目零接触）。

## M2 清单

- [x] alert：`CheckDriftScan`（汇总告警 + 真去重复用）+ 单测
- [x] server：`drift_scan.go`（循环 + Setting 键 + interval 校验）+ config
      GET/PUT 端点 + server.go 启动
- [x] web：/drift 页面「自动巡检」设置行 + api/types
- [x] 测试：告警创建/去重/全 ok 无动作、config 校验、扫描失败跳过
- [x] 文档收尾（本清单勾选）+ todo.md 同步

✅ M2 完成（2026-09-15）：alert 2 测试（创建/字段校验、未读去重、已读后重新
告警、空列表 no-op、13 项截断 10 行 + 总数提示）+ server 5 测试（config 默认/
合法含 0 写入/越界 400、扫描产告警 + 二轮去重、全 ok 与 error 态不告警、agent
报错跳过不 panic、标题定位到主机名）全绿；巡检循环为固定 1min ticker +
lastScan 对比（间隔动态改生效），启动等 90s 让 agent 先上线。

## 参考

- 内部：[cron-design.md](./cron-design.md)（splitCockpit 与 server 转发模式）、
  [proxy-design.md](./proxy-design.md)（片段目录与零接触纪律）、
  [stack-deploy-design.md](./stack-deploy-design.md)（stacks 目录布局）
- 外部：NetBox desired/actual 模型、driftctl / Terraform drift detection 的
  状态比对呈现
