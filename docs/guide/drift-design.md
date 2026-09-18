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
| D22 | Web diff 渲染 | drifted 行加「差异」按钮 → Modal：自写 LCS 行级 diff（双方先按行截断 1000 行并标注「内容过长，仅对比前 1000 行」）；统一 diff 着色（del 红=基线有当前无，add 绿=当前新增，same 灰）；页脚提示「核对后到对应管理页重新保存可消除漂移」 | 自写 LCS（滚动 DP + 回溯，1000 行 O(n²)≤10⁶ 可接受）避免引入依赖；不做双栏对照（行级统一 diff 更适合配置段落场景） |

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

## 不做（后续版本）

- `no_baseline` 对象的手动登记（「以当前为准」）；
- nginx 非 cockpit 片段、外部 crontab 条目、docker 卷内容检测；
- inventory 声明字段（hostname/IP/状态）与 agent 实报的一致性高亮；
- 双栏对照式 diff / diff 语法高亮（行级统一 diff 先覆盖配置段落场景）。

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
