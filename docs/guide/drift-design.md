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

status 语义见 D5；`baseline_sha` / `current_sha` 空串表示对应侧不存在。

## 不做（M2 及以后）

- 定时巡检 + 漂移告警通知（复用 alert 管道）；
- `no_baseline` 对象的手动登记（「以当前为准」）；
- nginx 非 cockpit 片段、外部 crontab 条目、docker 卷内容检测；
- inventory 声明字段（hostname/IP/状态）与 agent 实报的一致性高亮；
- 漂移内容 diff 视图（M1 只报「变没变 + hash 短码」，看内容去各管理页）。

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

## 参考

- 内部：[cron-design.md](./cron-design.md)（splitCockpit 与 server 转发模式）、
  [proxy-design.md](./proxy-design.md)（片段目录与零接触纪律）、
  [stack-deploy-design.md](./stack-deploy-design.md)（stacks 目录布局）
- 外部：NetBox desired/actual 模型、driftctl / Terraform drift detection 的
  状态比对呈现
