# Server 自身数据库备份设计（备份管理补齐项）

> 2026-09-15 立项。对应 todo 备份管理遗留项「server 自身 SQLite 备份
> （VACUUM INTO）」。前置：`backup-design.md` M1/M1.5（Agent 侧文件备份
> 已闭环）。

## 痛点

备份管理目前只覆盖 **Agent 侧文件**（配置/数据库/卷打包）。但面板自己的
SQLite 库——Git-first CMDB 的全部资源记录（Agent/Domain/Certificate/
Service/Gateway/Storage）、用户与权限、审计日志、告警、备份配置、探测
历史、**会话录制元数据**——没有任何备份。磁盘损坏或误操作 `cockpit.db`
一次，全部管理面数据归零，且 Agent 侧备份反而依赖这些配置才知道备什么。

## 方案

SQLite 原生 `VACUUM INTO 'path'`：在线（不停机不加读锁阻塞）产生当前
数据库的**紧凑完整副本**，产物是普通 `.db` 文件，可直接改名替换回恢复。
server 单侧改动，Agent 零感知。

## 决策

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D1 | 备份方式 | `VACUUM INTO`（storage 层暴露 `VacuumInto(path)`），非文件拷贝 | 文件拷贝在写入并发下可能产生撕裂页；VACUUM INTO 是 SQLite 官方推荐的在线备份路径，产物还顺带整理碎片 |
| D2 | 存储 | `<db目录>/server-backups/cockpit-YYYYMMDD-HHMMSS.db`（0600）；**目录即事实源，不建 DB 表** | 文件名自带时间戳，列表 = 扫目录；无 per-record 字段需求（对比 BackupRun 需要任务状态机） |
| D3 | 调度 | 独立 `serverBackupLoop`（1h ticker）：读 Setting 间隔，扫描目录最新文件时间戳，距今 ≥ 间隔才执行 | 与 drift 巡检同款「ticker + 时间戳对比，间隔动态可改」；不挂 agent 备份循环（那是 per-agent 配置驱动的 dispatchDue） |
| D4 | 配置 | Setting `server_backup.interval_hours`（0=关闭，1-168，默认 24）+ `server_backup.retention_days`（0=永久，1-365，默认 7）；`GET/PUT /api/server-backups/config` | 全局配置 Setting 模式（probe/drift 同款）；默认每天备一次 |
| D5 | 手动触发 | `POST /api/server-backups/run` 立即备份（同一路径复用），记审计 | 磁盘满/误删后想立刻留档的场景；与循环互不干扰（同秒冲突直接报错） |
| D6 | REST | `GET /api/server-backups`（列表：文件名/时间/大小）、`GET .../{name}/download`、`DELETE .../{name}`；下载/删除记审计，列表/手动备份不记 | 与备份文件下载同款纪律（下载落 JWT blob、变更记审计）；手动备份是保护动作不是敏感读取 |
| D7 | 文件名安全 | 只接受 `cockpit-\d{8}-\d{6}\.db` 严格模式；其余 400 | 下载/删除按名字寻址，必须防 `../` 穿越与任意文件删除 |
| D8 | 恢复 | M1 不做在线恢复：下载产物 → 停服 → 替换 `cockpit.db` → 启动（README/文档说明） | SQLite 热替换正在写入的库必然损坏；停机替换是唯一安全路径 |
| D9 | 失败语义 | VACUUM INTO 失败记日志，下一轮重试；不产生告警 | 磁盘满等环境问题，日志足够（与 drift error 态同级别处理） |
| D10 | Web 入口 | `/backups` 页面加「面板数据库」分区卡片：配置行（间隔/保留）+ 立即备份按钮 + 文件列表（下载/删除） | 同一使用场景（备份），不新开页面 |

## 不做（后续版本）

- 在线恢复/一键还原（D8）；
- 异地传输（S3/rclone，与 Agent 备份的异地同一条路线一起做）；
- 定制时刻调度（daily@HH:mm，interval 小时数够用）。

## M1 清单

- [x] storage：`VacuumInto(path)` + 单测（产物可打开、表存在）
- [x] server：`server_backup.go`（Setting 读写 + 目录工具 + 备份执行 +
      retention 清理 + 调度循环）+ server.go 启动接线
- [x] REST：`api_server_backup.go` 列表/下载/删除/手动备份/配置 + 审计 +
      serveAPI 接入
- [x] web：/backups 页「面板数据库」卡片（配置 + 立即备份 + 列表）
- [x] 测试：VacuumInto 产物合法性、文件名模式拒绝、API 全路径、retention
      清理、间隔语义
- [x] 文档收尾（本清单勾选）+ todo.md 同步

✅ M1 完成（2026-09-15）：server 6 测试全绿——VACUUM INTO 产物用真实 GORM
重开能读到备前写入的数据（完整性硬验证）且权限 0600；文件名严格模式拒绝
`../` 穿越与畸形名；配置语义（默认 24h/7d、0=关闭或永久、越界/非法回默认）；
retention 清理过期删永久留；API 全路径（配置 GET/PUT 含越界 400、立即备份、
列表、下载、删除、404/400、四类审计动作落库）。

## 参考

- 内部：[backup-design.md](./backup-design.md)（Agent 侧备份与任务模型）、
  [recording-design.md](./recording-design.md)（retention 清理与下载审计模式）
- 外部：SQLite 文档 Backup API / VACUUM INTO（3.27+）
