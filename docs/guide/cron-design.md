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

## 不做（后续项）

- systemd timer / service 列表展示：`systemctl list-timers` 输出解析脆弱，按需后补；
- 多用户 crontab（`-u`）：需要 agent 侧用户枚举与权限边界设计，等真实需求；
- 执行历史 / 失败告警：需包装器或日志采集，属日志聚合范畴；
- cron 表达式「下次触发时间」预览：需要完整 cron 解析器，M2 考虑引入成熟库；
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
