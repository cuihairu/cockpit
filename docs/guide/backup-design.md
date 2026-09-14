# P1 方案设计：备份管理（定时备份 + 保留策略 + 运行历史）

> 2026-09-14。P1「备份管理」首期设计。todo.md 原文：「数据库/配置/卷的快照与恢复，
> 定时备份策略 + 异地保留。这是个人云数据安全的核心缺口」。方案参考：Proxmox vzdump
> （tar 全量 + 保留 N 份）、restic（增量去重，本轮不引入）。本文只做设计决策与接口定义。

## 现状与痛点

| 痛点 | 现状 | 后果 |
|------|------|------|
| 无任何备份能力 | 全代码库无 backup 相关实现 | Docker 卷、配置目录、SQLite 文件散落各 agent 主机，磁盘故障即丢失 |
| 手工备份无记录 | 用户 SSH 手动 tar | 何时备过、备到哪、多大，无处可查 |
| 无保留策略 | 手动删除旧包 | 要么撑爆磁盘，要么不敢删 |
| 失败无感知 | 手动备份忘了校验 | 以为有备份，恢复时才发现是坏的 |

## 架构总览

Agent 主动出站 WebSocket 的连接模型决定**备份数据流不能经 server 中转**（RPC 不适合
传 GB 级文件）。因此备份文件在 **agent 主机本地生成、落在本地/挂载存储**（NAS 挂载点、
第二块盘、本机备份目录）；server 是纯控制面：存配置、算调度、下发指令、记录历史。

```
┌─ server ─────────────────────────┐        ┌─ agent ──────────────────────┐
│ BackupConfig 表（哪些目录、几点） │        │ backup provider               │
│ backupLoop（每分钟扫描到期配置）  │ ─RPC─▶ │ backup.run → 异步任务 tar.gz  │
│ BackupRun 表（运行历史/大小/状态）│ ◀─RPC─ │ backup.task.get → 轮询状态    │
│ 失败 → notification backup.failed│        │ retention 清理（保留 N 份）   │
│ REST /api/backups + Web 页面     │        │ 备份文件留在 agent 侧存储      │
└──────────────────────────────────┘        └───────────────────────────────┘
```

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | 备份执行位置 | Agent 侧本地 tar.gz，文件留在 agent 侧存储 | WebSocket 出站模型传不了大数据；NAS 挂载点即可实现「异地」；server 只做控制面 |
| D2 | 任务模型 | 复用 stack provider 异步任务模式（launchTask + task.get 轮询 + cappedBuffer 日志） | tar 数 GB 耗时分钟级，远超 CallAgent 30s 超时；该模式已验证 |
| D3 | 调度位置 | Server 侧循环（每 60s 扫描到期配置） | server 是唯一全局时钟；agent 无常驻调度职责；停机恢复后自动补跑一次 |
| D4 | schedule 形式 | `manual` / `daily@HH:mm` / `every:Nh`（1–168h） | 个人云不需要完整 cron；三种形式覆盖「每晚 3 点」「每 6 小时」「只手动」 |
| D5 | 保留策略 | retention N 份，agent 侧 backup.run 成功后按 `{name}-*.tar.gz` glob 清理最旧 | 清理发生在备份所在文件系统，server 无需文件访问；N=0 表示不清理 |
| D6 | 备份内容 | 源路径列表整体 tar.gz（配置目录 / docker volume 目录 / sqlite 文件） | 全量 + retention 简单可靠；数据库热备（dump/快照）记入 M1.5 |
| D7 | capability | 新增 `backup` capability，Linux agent 无条件注册 | 文件系统打包无外部依赖（系统自带 tar 由 Go 标准库替代，零依赖）；Windows 不注册 |
| D8 | 失败通知 | 集成 notification.Service，失败发 `backup.failed`（走 cfg.Events 白名单） | 成功不发（吵）；复用拨测增强已建好的多渠道通知 |

## Agent 侧设计

### capability 与注册

- detector 不动——`backup` capability 由 `agent.go` 在 GOOS==linux 时直接追加
  （类似 hardware-monitor 的无条件思路），metadata 带版本号；
- `providers.go` setupProviders 对 `cap.Type == "backup"` 调 `registerBackupProvider`，
  注册条件：GOOS==linux（openwrt/busybox 环境未验证，不阻断注册，见「不做」）。

### RPC 方法

| 方法 | 参数 | 返回 | 同步/异步 |
|------|------|------|-----------|
| `backup.run` | `{configId, name, sources[], destDir, retention}` | `{taskId}` | 异步启动 |
| `backup.task.get` | `{taskId}` | `{taskId, status, error, file, size, startedAt, finishedAt, log}` | 同步查询 |
| `backup.list` | `{dir}` | `{files: [{name, size, mtime}]}` | 同步 |
| `backup.delete` | `{dir, name}` | `{}` | 同步 |

- `configId` 仅供日志/任务记录关联，agent 不解析调度语义；
- `backup.list` 只列 `{dir}` 下匹配 `*-*.tar.gz` 的普通文件（浏览备份产物，不列任意目录内容）；
- `backup.delete` 只接受文件名（非路径），校验通过后才在 `{dir}` 内删除。

### 任务执行流程（backupTask）

1. 校验参数：`name` 匹配 `^[a-z0-9][a-z0-9_-]{0,63}$`；`sources` 非空且每个路径
   Clean 后仍为绝对路径；`destDir` 绝对路径；
2. `os.MkdirAll(destDir, 0o700)`；
3. 文件名 `{destDir}/{name}-{20060102-150405}.tar.gz`；若已存在（同秒重复）报错；
4. 逐 source 打包：每个源以 **basename 为顶层目录**（`etc-nginx/nginx.conf`），
   避免多源互相覆盖；walk 时符号链接记录为链接本身（archive/tar 默认行为，不跟随），
   目标不存在/无权限的源记 warning 跳过但继续其他源；
5. gzip BestSpeed（个人场景时间比体积敏感，后续可配）；完成后记录 size；
6. retention>0：按 mtime 降序列 `{destDir}/{name}-*.tar.gz`，删除第 N 份之后的最旧文件，
   清理结果记入任务日志（删除失败仅 warning，不影响任务成功）；
7. 终态 `success` / `failed`（含 error 摘要），日志进 cappedBuffer（复用 stack 的
   200KB 环形缓冲实现），任务内存 map + pruneTasksLocked 复用 stack 模式。

### 路径安全

- 所有入参路径 `filepath.Clean` + `filepath.IsAbs` 校验，拒绝相对路径；
- `backup.delete` 的 `name` 必须匹配 `^[A-Za-z0-9][A-Za-z0-9._-]*\.tar\.gz$`
  （不可能含 `/`，从根上杜绝穿越）；
- server 下发前同样校验（见 server 节），双端防御。

## Server 侧设计

### 数据模型（storage）

```go
// BackupConfig 备份任务配置
type BackupConfig struct {
    ID         uint
    AgentID    string `gorm:"index;size:64"`
    Name       string `gorm:"size:64"`   // 文件名前缀，^[a-z0-9][a-z0-9_-]{0,63}$
    Sources    string `gorm:"type:text"` // JSON 数组字符串 ["/etc/nginx", "/var/lib/postgres"]
    DestDir    string `gorm:"size:512"`
    Schedule   string `gorm:"size:32"`   // manual / daily@HH:mm / every:Nh
    Retention  int                       // 保留份数，0=不清理
    Enabled    bool
    LastRunAt  int64
    NextRunAt  int64                     // manual 恒为 0
    LastStatus string `gorm:"size:16"`  // "" / running / success / failed
    CreatedAt, UpdatedAt int64
}

// BackupRun 单次运行记录
type BackupRun struct {
    ID         uint
    ConfigID   uint `gorm:"index"`
    TaskID     string `gorm:"size:64"`
    Status     string `gorm:"size:16"` // running / success / failed
    File       string `gorm:"size:256"` // 备份文件名（不含目录）
    Size       int64
    Error      string `gorm:"size:512"`
    StartedAt, FinishedAt int64
}
```

- `Sources` 存 JSON 字符串，应用层 marshal/unmarshal（不引入 datatypes.JSON 依赖）；
- storage 提供 `CreateBackupConfig / UpdateBackupConfig / DeleteBackupConfig（级联删 runs）/
  ListBackupConfigs / GetBackupConfig / CreateBackupRun / UpdateBackupRun /
  ListBackupRuns(configID uint, limit int)`（configID=0 查全部）。

### 调度循环（server/backup_loop.go）

- `backupCheckLoop`（参考 alertCheckLoop）：每 60s tick，`SELECT ... WHERE enabled AND next_run_at <= now`；
- 到期且 `LastStatus != "running"`（同一 config 不并发）：
  1. 计算并回写 `NextRunAt`（**先推进再执行**，防执行失败后每分钟重试打爆 agent）；
  2. 调 `s.startBackupRun(config, "scheduled")`（与手动运行共用）；
- `startBackupRun`：校验 agent 在线 + HasCapability("backup") → 下发 `backup.run` →
  创建 BackupRun{running} → 置 Config.LastStatus=running → 起 goroutine
  `trackBackupTask`（复用 trackStackTask 模式：每 5s 轮询 `backup.task.get`，10 分钟无
  终态判超时失败）→ 终态回填 BackupRun（file/size/error/finishedAt）+ Config.LastStatus/LastRunAt；
- 失败终态 → `s.notifier.SendNonBlocking(&notification.Notification{EventType:
  notification.EventBackupFailed, Level: "error", ResourceType: "backup", ...})`；
- NextRunAt 计算：`daily@HH:mm` → 今天/明天该时刻（server 本地时区）；`every:Nh` →
  now + N*h；创建/更新配置时同步计算；
- server 启动时对 enabled 非 manual 的配置若 `NextRunAt == 0`（历史数据/首次启用）初始化下次时间。

### REST API（server/api_backups.go，registerBackupsAPI，JWT 保护）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/backups/configs` | 配置列表（Sources 解析为数组返回） |
| POST | `/api/backups/configs` | 创建；校验 agent 存在、name/schedule/retention 合法、sources 全绝对路径 |
| PUT | `/api/backups/configs/{id}` | 更新；重算 NextRunAt |
| DELETE | `/api/backups/configs/{id}` | 删除（级联删 runs） |
| POST | `/api/backups/configs/{id}/run` | 手动运行（running 状态拒绝 409） |
| GET | `/api/backups/configs/{id}/runs` | 该配置运行历史（limit 50） |
| GET | `/api/backups/configs/{id}/files` | 列 agent 备份目录文件（转发 backup.list，dir 取自配置） |
| POST | `/api/backups/configs/{id}/files/delete` | 删单个备份文件 `{name}`（校验文件名正则后转发 backup.delete） |

- 文件浏览/删除均以 config 的 destDir 为根，**不接受客户端传目录**；
- 审计：`ResourceBackup = "backup"`，动作 create/update/delete/run/delete_file；
  审计数据不含备份内容，sources 路径属配置信息可记。

### 通知与事件

- `notification/events.go` 加 `EventBackupFailed = "backup.failed"`；
- config.yaml `events.backup-failed: {type: backup.failed, enabled: true}` 显式启用才发
  （与 service.down 同一白名单机制）。

## Web UI

新页面 `web/src/pages/Backups/index.tsx` + 路由 `/backups` + 菜单「备份管理」：

- **配置列表** Table：名称 / agent / 源路径（Tag + Tooltip）/ 目标目录 / 计划
  （每日 02:00、每 6 小时、手动）/ 保留 / 状态 Tag（success 绿 / failed 红 / running 蓝）/
  上次运行时间 / 操作（立即运行、编辑、删除 Popconfirm）；
- **新建/编辑** Modal：agent 下拉（在线优先）、名称、源路径（Select mode="tags"，
  逐条回车）、目标目录 Input、计划类型 Radio（手动/每日/间隔）+ 时间选择/小时数、
  保留份数 InputNumber(0-365)、启用 Switch；
- **运行历史** Drawer：按配置查看（状态、文件名、大小、耗时、错误信息）；
- **文件浏览** Drawer：列 destDir 下备份文件（名称/大小/时间）+ 删除 Popconfirm；
- 空状态引导创建第一条备份配置。

## M1.5 设计：恢复（restore）与备份文件下载

### 关键决策（M1.5 增补）

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D9 | restore 目标 | destDir 必须**不存在或为空目录**，绝不覆盖现有数据 | 「独立目录解包」的强保证：恢复产物与在线数据物理隔离，迁移回原位是用户显式动作 |
| D10 | restore 双确认 | UI Modal 输入备份名 + API `confirmName` 字段必须与 `file` 一致 | GitHub 删除仓库模式：危险操作强制显式输入，误触不可能触发 |
| D11 | restore 执行 | 异步任务（复用 backup 任务模型）+ Zip Slip 防护 | 大包解包分钟级；tar 解包路径穿越是经典攻击面 |
| D12 | 下载通道 | 分块 RPC `backup.read {offset, length}`（base64 块）经 server 流转发 | 复用既有出站 WebSocket，无需协议扩展；server 不落盘直接流给浏览器 |

### Agent 侧新增 RPC

| 方法 | 参数 | 返回 | 同步/异步 |
|------|------|------|-----------|
| `backup.restore` | `{dir, name, destDir}` | `{taskId}` | 异步任务 |
| `backup.read` | `{dir, name, offset, length}` | `{data(base64), size, eof}` | 同步分块 |

- restore 校验：dir/name/destDir 路径安全同既有规则；destDir Clean 后不得为 `/`；
  destDir 已存在且非空 → 拒绝（D9）；备份文件必须存在；
- 解包安全：条目 `filepath.Join(destDir, hdr.Name)` 后强制前缀在 destDir 内（Zip Slip）；
  reg/dir/symlink 三类照常，其余 typeflag（设备/硬链接等）跳过 warning；
  单条目失败 warning 继续，日志记条目计数；
- `backup.read` 单块上限 1MB（server 用 256KB）；返回文件总大小供进度与 EOF 判断。

### Server 侧新增 REST

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/backups/configs/{id}/restore` | `{file, destDir, confirmName}`；confirmName ≠ file → 400；下发 backup.restore + 审计 `backup_restore` |
| GET | `/api/backups/configs/{id}/tasks/{taskId}` | 转发 `backup.task.get`（restore 任务状态/日志轮询，不落库） |
| GET | `/api/backups/configs/{id}/files/download?name=` | 循环 backup.read 分块拉取，流式写 ResponseWriter（attachment），客户端断开即停，总超时 15 分钟 |

- 下载不落 server 磁盘；name 走同一文件名正则校验。

### Web UI（备份文件 Drawer 增强）

- 每个备份文件行操作：**下载** / **恢复** / 删除；
- 下载：axios blob（带 JWT）+ `URL.createObjectURL` 触发保存；
- 恢复 Modal：展示文件名/大小/时间，填写目标目录（独立新目录），**输入备份名才能点确认**（D10）；
  提交后 Modal 切换为任务视图，轮询任务状态并展示解包日志尾部，终态显示成功/失败。

## 不做（后续项）

- S3/对象存储异地：agent 侧 rclone 集成或 server 中转，独立立项；
- server 自身 DB 备份（SQLite `VACUUM INTO`）：独立小功能；
- 数据库热备钩子（mysqldump / pg_dump / sqlite .backup 前置钩子）；
- 增量/去重（restic/borg 级别）与备份加密（age/gpg）：个人场景全量 + retention 先够用；
- openwrt/busybox 环境适配：未验证，不主动阻断但不在验收范围。

## 分期落地

### M1 —— 定时备份 + 保留策略 + 运行历史（2026-09-14 完成）

- [x] agent：`internal/agent/rpc/backup_provider.go`（run/task.get/list/delete +
      异步任务模型 + retention 清理 + 路径安全）
- [x] agent：backup capability 注册（GOOS==linux）+ providers.go 接线
- [x] storage：BackupConfig / BackupRun 表 + CRUD（含级联删除）
- [x] server：`backup_loop.go` 调度循环（NextRunAt 计算 + 到期下发 + track + 超时 + 失败通知）
- [x] server：`api_backups.go` REST 全量 + 审计（ResourceBackup）+ 路由注册
- [x] notification：BackupFailed 常量
- [x] web：备份管理页（配置 CRUD + 立即运行 + 运行历史 + 文件浏览/删除）+ 路由菜单
- [x] 测试：agent provider 单测（打包结构 / retention 清理 / 路径穿越拒绝 / 符号链接）、
      storage CRUD 与级联、调度 NextRunAt 计算各形态、server API 全分支
- [x] 文档收尾 + todo.md 同步

### M1.5 —— 恢复与备份文件下载（2026-09-14 完成）

- [x] agent：`backup.restore`（独立目录解包 + Zip Slip 防护 + 异步任务）与
      `backup.read`（分块 base64 读取）
- [x] server：restore REST（confirmName 双确认 + 审计 `backup_restore`）+
      任务轮询转发 + 文件下载流转发（Content-Length 透明透传，15 分钟总超时）
- [x] web：文件行「下载/恢复」操作（axios blob 下载）+ 恢复 Modal（输入备份名确认 +
      提交后任务视图轮询解包日志）
- [x] 测试：restore roundtrip / 非空目录拒绝 / Zip Slip 拒绝（恶意包构造）/
      分块读取边界（跨块 + EOF + 越界）/ server confirm 校验与下载流转发逐字节比对

## 参考

- [Proxmox VE vzdump](https://pve.proxmox.com/wiki/Backup_and_Restore)——tar 全量 +
  保留 N 份的资源调度模式
- [restic](https://restic.net/)——增量去重备份的标准设计（本轮不引入，后续异地化的候选）
- 内部：[stack-deploy-design.md](./stack-deploy-design.md)（异步任务模型来源）、
  [probe-enhance-design.md](./probe-enhance-design.md)（通知渠道集成）、`todo.md` P1 备份管理条目
