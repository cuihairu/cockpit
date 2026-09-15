# 远控会话录制设计（M1：终端输出录制 + 回放）

> 2026-09-15 立项。对应 todo「远控会话录制：终端输出录制，补强远控审计
> （当前审计只有会话开始/结束）。方案参考：Guacamole 会话录制、Teleport
> 会话粒度审计。」

## 痛点

远控（SSH/telnet 终端）的审计目前只有**会话开始/结束**两条记录
（`auditRemoteStart` / `auditRemoteEnd`）：知道谁在什么时间连了哪台主机，
但**会话里做了什么完全不可追溯**。个人云场景下，所有管理员的终端操作
集中在这一个入口，出问题（误删文件、改错配置）后无法还原现场。

## 架构

终端数据本来就全部流经 server（Agent 主动出站 WebSocket，浏览器 ←
server ← agent 双向转发），**录制挂在 server 侧的转发管道上**，agent
零变更：

```
浏览器 ──input──▶ terminalSendLoop ──▶ agent
浏览器 ◀─data─── HandleTerminalData ◀── agent
                      │
                      └─▶ castRecorder.Write() ──▶ data/recordings/<sid>.cast
                                                     + SQLite 元数据
```

- `api_remote.go:302 HandleTerminalData`：agent → 浏览器的唯一出口，
  录制挂钩点；
- `closeTerminalSession` / `HandleTerminalClose`：会话终态，finalize 回填
  时长与字节数。

## 决策

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D1 | M1 范围 | 仅终端（SSH/telnet 经 `/api/remote/terminal` 的会话）；VNC/RDP 像素流不录 | 终端流天然是字符事件序列，asciinema 格式即为其行业标准；像素流录制体积与回放复杂度高，价值低 |
| D2 | 录制位置 | server 侧转发管道，agent 零变更 | 数据已流经 server；agent 侧录会引入文件管理与上传链路 |
| D3 | 格式 | asciinema v2（`.cast`）：首行 header JSON，之后每事件一行 `[相对秒,"o","数据"]` | 行业开放格式，可脱离 Cockpit 用 `asciinema play` 回放/转换；行式追加写天然适合流式 |
| D4 | 只录输出 | 输入流不录（不写 `"i"` 事件） | 输入含密码（SSH/sudo）明文，录制即泄漏源；Guacamole 同样只录输出侧。审计要的是「终端上出现了什么」而非「键盘按了什么」 |
| D5 | 存储 | 文件 `<db目录>/recordings/<sessionID>.cast` + 新表 `TerminalRecording`（session_id 唯一、username、agent_id、host、port、protocol、started_at、duration_ms、bytes） | 文件放内容、DB 放索引（与备份文件/配置分离同思路）；结束时回填 duration/bytes |
| D6 | 开关与保留 | Setting `recording.enabled`（"true"/"false"，默认 true）+ `recording.retention_days`（0-365，默认 7，0=永久）；清理挂 cleanupLoop 每 30s 醒、按小时节流执行 | 默认开（审计闭环是卖点）+ 有限保留防磁盘膨胀；与 drift 巡检同款「循环里读 Setting 免缓存」 |
| D7 | 写入策略 | `os.File` 直接写（不 fsync），每事件一行；写失败记一次日志后停录，**绝不影响转发主链路** | 终端输出小块低频，崩溃丢尾部可接受；远控可用性优先于录制完整性 |
| D8 | finalize 幂等 | recorder 内部 `sync.Once`；`closeTerminalSession` 与 `HandleTerminalClose` 两处都调 Close（后者路径经 keepalive ping 失败兜底会晚 ≤30s 到达，两处直调即时回填） | 会话有两个结束入口，Once 防双写回填 |
| D9 | REST | `GET /api/recordings`（倒序列表）`GET /api/recordings/{id}/cast`（文件流，即回放数据源，兼下载）`DELETE /api/recordings/{id}`（文件+记录同删） | 回放与下载同一端点取全量内容，统一记审计 |
| D10 | 审计 | `recording_read`（取 cast 内容）与 `recording_delete` 记审计；列表浏览不记 | 录制内容含历史输出（可能有敏感信息），谁取走了内容应可追溯；列表是索引不算内容 |
| D11 | 回放器 | Web 自研轻量：fetch .cast → 逐行解析 → 按时间差调度 `term.write()`，倍速 1/2/4/8x + 拖动跳放 | 不引 asciinema-player 依赖；项目已有 @xterm/xterm |
| D12 | 尺寸 | cast header 固定 80x24；回放 xterm 按容器自适应不锁列宽 | server 侧不追踪 resize（session 未存 rows/cols）；asciinema v2 header 亦为单一尺寸，错位可接受 |
| D13 | Web 入口 | 新页面 `/recordings`「会话录制」，菜单列于「审计日志」旁 | 录制是审计的延伸，入口放一起 |

## 不做（后续版本）

- VNC/RDP 桌面流录制（WebM/图片序列，体积大回放重）；
- 输入事件录制（密码泄漏风险，见 D4）；
- 实时旁观（live tail 进行中的会话）；
- 录制文件的导出归档（S3/rclone，与备份异地同路线，等备份 M2 一起做）。

## M1 清单

- [x] storage：`TerminalRecording` 模型 + 迁移 + Create/Finish/List/Get/
      Delete/DeleteExpiredUntil
- [x] server：`recording.go`（castRecorder 读写 + Setting 读写 + 过期清理
      挂 cleanupLoop）+ 终端管道接线（创建/写输出/finalize 幂等）
- [x] REST：`api_recordings.go` 列表/取内容/删除 + 审计 + serveAPI 接入
- [x] web：`/recordings` 页面（列表 + xterm 回放 Modal 含倍速 + 下载/删除）
      + 路由菜单
- [x] 测试：cast 格式与 finalize 幂等、API CRUD 与审计、过期清理、
      HandleTerminalData 落盘链路
- [x] 文档收尾（本清单勾选）+ todo.md 同步

✅ M1 完成（2026-09-15）：server 5 测试全绿——cast 格式（header version=2 +
`[dt,"o",data]` 行、Close 幂等回填一次、nil receiver 安全、文件权限 0600）、
startRecording 元数据登记与管道落盘、开关语义（默认开/"false" 关，保留天数
合法/0/非法回默认）、过期清理（文件+记录同删、0=永久）、API（列表/取内容
Content-Type/404/405/删除/审计 view+delete 落库）。回放器自研：按事件时间差
setTimeout 调度 term.write，1/2/4/8x 倍速切换即重放，进度展示。

## 参考

- 内部：[logs-design.md](./logs-design.md)（server 侧转发不落盘的反例——
  本功能正是「落盘」侧的审计场景）、architecture.md 审计章节
- 外部：asciinema v2 格式规范（<https://github.com/asciinema/asciinema/blob/develop/doc/asciicast-v2.md>）、
  Guacamole session recording、Teleport session recording
