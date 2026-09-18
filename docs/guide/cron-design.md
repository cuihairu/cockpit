# P2 方案设计：定时任务管理（Crontab 可视化）

> 2026-09-15。P2「定时任务/Cron 管理」首期设计。todo.md 原文：「Agent 侧 crontab
> 可视化与调度，支撑自动化运维」。本文只做设计决策与接口定义。

## 现状与痛点

| 痛点 | 现状 | 后果 |
|------|------|------|
| 加个定时任务要 SSH 手编 crontab | `crontab -e` 手写表达式，写完不知道对不对 | 表达式写错静默不跑，很久才发现 |
| 主机上有哪些定时任务无处可查 | 散落在各主机的 crontab / 脚本里 | 半年后不知道哪个 cron 在干什么、还在不在用 |
| 改 crontab 心惊胆战 | 全量替换写回，手滑删掉别人的条目 | 用户手写的条目被误伤且无备份 |
| Cockpit 无自动化入口 | 备份调度是 server 侧私有实现，agent 侧零能力 | 「自动化运维」无从谈起 |

## 架构总览

与反向代理同一通道模型：**同步 RPC + server 纯转发**。crontab 是 KB 级文本、
读写毫秒级，30s RPC 超时绰绰有余。

```
┌─ server ────────────────────┐        ┌─ agent（crontab 所在主机）────────┐
│ REST /api/agents/{id}/cron  │ ─RPC─▶ │ cron provider（新）                │
│ 任务 CRUD + 审计            │ ◀─RPC─ │ 读全量 → 替换自己名下段 → 写回     │
│ 不落库：crontab 即状态      │        │ 用户手写条目零接触（逐行保留）     │
└─────────────────────────────┘        └───────────────────────────────────┘
```

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | M1 范围 | 仅 **crontab**（agent 运行用户，`crontab -l` 读 / `crontab -` 写回）；systemd timer 只读列表后续 | 个人云定时任务绝大多数在 crontab；多用户 `-u` 与 timer 列表按需后补 |
| D2 | 管理边界 | cockpit 只动自己名下条目：每个任务两行（meta 注释行 + 命令行），写回 = 读全量 → 只替换 cockpit 段 → 其余行**逐行原样保留**；写回前自检非 cockpit 行与旧内容一致，不一致拒绝写 | crontab 是全量替换写回，没有片段文件可用；自检兜底绝不误伤用户手写条目 |
| D3 | 任务元数据 | meta 注释行 `# cockpit:job {json}` 内嵌（含 name/schedule/command/enabled），回读以 meta 为准 | 与 nginx `# cockpit:meta` 同构，roundtrip 靠解析自己渲染的产物 |
| D4 | 任务模型 | `{name, schedule, command, enabled}`；enabled=false 时命令行渲染为注释（停用不留痕丢失） | 覆盖个人场景；停用是高频需求（临时关掉而不是删掉） |
| D5 | 表达式 | 5 字段标准 cron + `@reboot/@hourly/@daily/@weekly/@monthly/@yearly` 白名单；双端同规则校验（字段范围 + 语法） | 前端即时反馈；agent 侧兜底防御 |
| D6 | 执行模型 | 全同步 RPC + provider 级 mutex 串行化读改写 | 操作频率极低，异步任务是过度设计；mutex 防 server 并发 apply 交叉写 |
| D7 | capability | `cron`，`exec.LookPath("crontab")` 探测；未安装的 agent 不出现假能力 | 同 nginx-proxy 模式 |
| D8 | REST 挂载 | `/api/agents/{id}/cron/...`，复用 serveAPI `/agents/` 分发模式 | 按 agent 作用域 |
| D9 | 持久化 | **不落库**——agent 侧 crontab 为唯一事实源，server 纯转发 + 审计 | 无状态同步问题；apply/delete 记审计可追溯 |
| D10 | 执行历史 | M1 不做（cron 无内建执行记录，采集需包装器脚本） | 属日志聚合范畴，独立立项 |

## Agent 侧设计

### 探测与注册

- `exec.LookPath("crontab")` 通过则追加 `cron` capability（providers.go 按能力注册）；
- provider 复用 nginx 的 `Commander` 抽象执行 `crontab -l` / `crontab -`，测试可注入。

### RPC 方法

| 方法 | 参数 | 返回 | 说明 |
|------|------|------|------|
| `cron.status` | `{}` | `{user, cockpitCount, externalCount}` | 概览：运行用户 + 条目计数 |
| `cron.jobs` | `{}` | `{jobs: [...], external: "..."}` | cockpit 名下任务列表 + 外部条目原文（只读展示） |
| `cron.job.apply` | `{job}` | `{name}` | 校验 → 读全量 → 替换 cockpit 段 → 自检 → 写回 |
| `cron.job.delete` | `{name}` | `{}` | 从 cockpit 段移除该任务 → 自检 → 写回 |

### 任务参数与校验（双端同规则）

| 字段 | 规则 |
|------|------|
| `name` | `^[a-z0-9][a-z0-9_-]{0,63}$`（proxyNameRe 同款） |
| `schedule` | 5 字段 cron（分 0-59 / 时 0-23 / 日 1-31 / 月 1-12 / 周 0-7，支持 `* , - /`）或 `@` 白名单 |
| `command` | 非空，≤4KB，**不含换行**（crontab 单行语义） |
| `enabled` | bool |

### 渲染与写回流程

```
渲染（每任务两行）：
  # cockpit:job {"name":"backup","schedule":"0 3 * * *","command":"/opt/bk.sh","enabled":true}
  0 3 * * * /opt/bk.sh
  （enabled=false 时命令行为 #0 3 * * * /opt/bk.sh）

apply/delete：
  crontab -l 读全量（空 crontab 视为空串）
    ↓
  替换 cockpit 段：扫描 meta 标记行定位任务对，整对替换/删除；
  新任务追加到段尾；非 cockpit 行保持原序原样
    ↓
  自检：新旧内容的非 cockpit 行逐行一致，否则拒绝写回（防御 bug）
    ↓
  crontab - 写回（stdin）
```

- 读改写临界区由 provider 级 `sync.Mutex` 串行化；
- 外部条目（非 cockpit 行）原样透传给 UI 只读展示——「可视化」的一部分。

### cron 表达式校验器

- `@` 开头走白名单（6 个），否则按空白切 5 段；
- 每段匹配 `^(\*|\d+)(-\d+)?(/\d+)?$` 的逗号组合，数字 token 按字段范围校验
  （dow 同时接受 0-7，7 视同 0）；
- 双端实现同规则（Go `validateCronExpr` / TS `validateCronExpr`）。

## Server 侧设计

### REST API（server/api_cron.go，挂 serveAPI `/agents/` 分支，JWT）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/api/agents/{id}/cron/status` | 转发 cron.status |
| GET  | `/api/agents/{id}/cron/jobs` | 转发 cron.jobs（含 external 原文） |
| PUT  | `/api/agents/{id}/cron/jobs/{name}` | body 为任务参数 → cron.job.apply + 审计 cron_apply |
| DELETE | `/api/agents/{id}/cron/jobs/{name}` | cron.job.delete + 审计 cron_delete |

- 参数校验双端同规则（D5）；agent 错误原样透传（502）；
- 审计：`ResourceCronJob = "cron_job"`，动作 cron_apply / cron_delete，
  detail 记 name/schedule/command 摘要（命令属用户自己写的运维内容，全量可记）。

## Web UI 设计

- 新页面 `/cron`「定时任务」（菜单，ClockCircleOutlined）：
  - **Agent 选择**：下拉，无 cron capability 的 agent 禁选并提示；
  - **状态卡**：运行用户 / cockpit 任务数 / 外部条目数；
  - **任务列表** Table：名称 / 表达式（等宽）/ 命令（省略号展开）/ 状态
    （启用 Tag / 停用 Tag）/ 操作（编辑、启停 Switch、删除 Popconfirm）；
  - **新建/编辑 Modal**：名称、表达式（常用预设下拉：每分钟/每小时/每天 HH:mm/
    每周一 03:00/自定义，选中自动填）、命令 TextArea（等宽，校验无换行）、启用开关；
  - **外部条目** 折叠面板：只读 pre 展示非 cockpit 名下的原始行（可见不可编辑）。

## M2：下次触发预览（2026-09-18）

任务列表从「何时配的」升级为「何时会跑」：agent 侧自写 cron 解析器算下次
触发，`cron.jobs` 响应顺带返回，server 零改动。

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D14 | 计算位置在 agent | `next_run` 由 agent 用服务器本地时区（`time.Now()`）计算，返回 unix 秒 | cron 触发语义绑定服务器时区——浏览器时区算必错；server 纯转发不动 |
| D15 | 解析器自写 | `cron_next.go`：复用 `validateCronExpr` 已定义的语法（`cronFieldRe` 字段形 + `cronFieldRanges` 范围 + @ 白名单）——白名单式校验保证表达式形态有界，不需要 Robfig 的秒/年字段超集；@ 简写展开等价 5 字段（@hourly=`0 * * * *`、@daily=`0 0 * * *`、@weekly=`0 0 * * 0`、@monthly=`0 0 1 * *`、@yearly/@annually=`0 0 1 1 *`）；@reboot 返回 0（无下次触发） | 不引第三方库：支持的表达式域是自身校验器的子集，解析器与校验器同源演进 |
| D16 | 触发判定语义 | 字段展开为位集合逐分钟扫描（上限 5 年，262 万次整数比较 <10ms，任务量级个位数到百）；dom/dow 联合遵循 Vixie cron 规则——**两者都受限时 OR**（`0 0 1 * 1` = 每月 1 号或每周一），**任一为 `*` 时 AND**；dow 7 归一为 0（周日） | Vixie 联合语义是 crontab(5) 事实标准，漏掉必算错；逐分钟扫描简单正确优先 |
| D17 | 呈现 | `Jobs()` 每个 job 加 `next_run`（unix 秒；disabled=0 不计算，解析失败=0 不致命——外部手改的非法表达式不影响列表）；web 任务列表加「下次触发」列：disabled→「已禁用」、@reboot→「开机时」、0→`-`、其余本地时间 | 列表 RPC 顺带返回零新增往返 |

### Agent 侧

- `cron_next.go`：`parseCronSchedule`（表达式 → 位集合）+ `nextCronRun`（从
  下一整分钟起逐分钟扫描）；
- `Jobs()` 集成：enabled 任务逐个计算 `next_run`。

### Web 侧

- Cron 页任务列表「下次触发」列（`dayjs.unix` 本地格式 YYYY-MM-DD HH:mm）。

**测试**：agent——每分钟/@daily/@reboot/步长范围/或语义/AND 语义/闰年 2·29/
dow=7/非法表达式 0/禁用不计算；web——tsc + build。

## M3：systemd timer 只读列表（2026-09-18）

crontab 之外的第二类定时机制。只读预览：列出系统 timer unit 的日程与触发
时间，不做启停/编辑（写路径涉及 root 边界，不做）。

| 决策 | 内容 | 理由 |
|------|------|------|
| D18 | 列举与查询走稳定 API：`systemctl list-unit-files --type=timer --no-legend` 取每行**首列**（unit 名，不依赖列数），再**一条命令** `systemctl show 全部 unit 名 -p Description -p ActiveState -p UnitFileState -p LastTriggerUSec -p NextElapseUSecRealtime -p TimersCalendar -p TimersMonotonic` 属性查询，输出按空行分段、每段 `Key=Value` | `list-timers` 表格式输出列随 systemd 版本变（此前「不做」的根因）；`show -p` 属性名是稳定 bus API，实测验证两段式各一次 exec |
| D19 | 日程取原文不做语义解析：`TimersCalendar` 的 `OnCalendar=` 原文优先，缺则拼 `TimersMonotonic` 各行（OnBootUSec/OnUnitActiveUSec）；`NextElapseUSecRealtime`/`LastTriggerUSec` 剥尾时区缩写后按**本机本地时区**解析为 unix 秒（`@` 前缀 = epoch 微秒直解；`n/a`/空/解析失败归 0）；show 失败的 unit（如模板 `@.timer`）仅列名、属性留空 | systemd 的 OnCalendar 语法超集太大不自写解析器；systemctl 与 agent 同机同 TZ，墙钟字符串与 `time.Local` 一致；模板单元本机实测 show 报错，兜底不致命 |
| D20 | 呈现与只读边界：agent cron provider 新 action `timers`（纯读，capability 不动——入口挂 Cron 页）；server `GET /api/agents/{id}/cron/timers` 纯转发不落库不审计；web Cron 页新增「systemd 定时器」折叠面板（unit/描述/日程/状态/上次触发/下次触发，关键字过滤，请求失败或空列表不显示面板，时间 `dayjs.unix` 本地格式） | 列表性质与外部条目折叠面板一致；只读不涉审计与 capability 语义变化 |

### Agent 侧

- `cron_timers.go`：`listTimers`（两次 exec + 分段解析 + 时间解析
  `parseSystemdTime`）；`Call` 加 case `timers`。

### Web 侧

- Cron 页「systemd 定时器」折叠面板（默认收起；空/失败不渲染）。

**测试**：agent——时间解析各形态（UTC/本地尾缀/`@`epoch/`n/a`/垃圾）、
分段解析（两 unit + 错误段跳过 + 模板单元仅列名）、schedule 原文提取
（calendar 与 monotonic 多行）；server——转发；web——tsc + build。

## 不做（后续项）

- 多用户 crontab（`-u`）：需要 agent 侧用户枚举与权限边界设计，等真实需求；
- 执行历史 / 失败告警：需包装器或日志采集，属日志聚合范畴；
- 分布式锁 / server 侧调度下发：agent 侧 crontab 已是事实源，无需中心化调度。

## M1 清单

- [x] agent：`internal/agent/rpc/cron_provider.go`（探测/渲染/读改写/自检/mutex）
      + capability 追加 + 按能力注册
- [x] server：`api_cron.go`（4 端点 + 双端参数校验 + 审计）+ serveAPI 接入
      + audit 常量
- [x] web：`/cron` 页面（agent 选择/状态卡/任务列表/编辑 Modal/外部条目）+ 路由菜单
- [x] 测试：渲染各形态（启用/停用）、meta roundtrip、表达式校验、外部行保留自检、
      并发 apply 串行化、server 转发与审计
- [x] 文档收尾 + todo.md 同步

## 参考

- 内部：[proxy-design.md](./proxy-design.md)（meta 注释 roundtrip 与 server 转发模式同构）、
  [backup-design.md](./backup-design.md)（nameLock 并发纪律）、`todo.md` P2 定时任务条目
- 外部：[crontab(5) 手册](https://man7.org/linux/man-pages/man5/crontab.5.html)
  （字段语义与 @extensions）、vixie-cron 源码（写回语义）
