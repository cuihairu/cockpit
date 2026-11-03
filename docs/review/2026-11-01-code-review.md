# 代码深审（2026-11-01）

> 按 requesting-code-review 工作流执行全仓代码深审：① 安全静态扫描 → ② 隐藏缺陷猎捕
> （Go agent/core 侧 + Go server 侧 + web/mobile 前端三路并行）→ ③ 质量门禁 →
> ④ 报告 → ⑤ P0/P1 当场修（一问题一 commit，测试绿），P2/P3 记 todo。
> 行号均为审计时 HEAD（dc81806 之后、717ac3b 前后混合核对，2026-11-03 复核全部原位）。

## 方法与范围

- **安全静态扫描**：硬编码秘密 / shell 注入 / SQL 拼接 / eval / 反序列化 / 路径穿越全仓 grep。
  命中项逐条核为安全形态：
  - `enrollment_secret` 仅作配置键名（internal/config/config.go:172），比较走 `subtle.ConstantTimeCompare`（internal/server/websocket.go:205-206），落库哈希（websocket.go:254）；
  - job executor `sh -c`（internal/agent/rpc/job_provider_unix.go:17）是设计内的远端命令执行面，RBAC 由 `jobs:write` 门禁；
  - 路径面有 `cleanAbsPath`（file_provider.go:72）守卫、录制路径 DB 校验（api_recordings.go:92）、SSH 密钥 const 清单（ssh_keys.go:73）；
  - SQL 全参数化（`?` 占位），无字符串拼接查询。
  - **结论：0 条独立安全 findings**（下表 P0 符号链接逃逸属猎捕类发现）。
- **隐藏缺陷猎捕**：三路并行只读猎捕（agent/core 11 条、server 11 条、web/mobile 9 条），
  跨路重复 1 条（manager.go:460 LastRead 竞态两路同报）去重后 **30 条**。
- **质量门禁**：`go vet` 绿；Go 全量短测除 `internal/server` 既有环境红口径（ticket.go:104
  cleanupLoop goroutine 泄漏超时，非本轮引入）外全绿；web `tsc --noEmit` 0 error、
  全量 1142+ 用例绿、`npm run build` 绿；P0/P1 修复各带回归测试与块覆盖登记同步。

## 计数汇总

| 严重度 | 总数 | 已修 | 记 todo |
| --- | --- | --- | --- |
| P0 | 1 | 1（dc81806） | 0 |
| P1 | 1 | 1（717ac3b） | 0 |
| P2 | 17 | 0 | 17 |
| P3 | 11 | 0 | 11 |
| **合计** | **30** | **2** | **28** |

## 已修（P0/P1）

### P0 备份恢复符号链接逃逸（CWE-59）— 已修 dc81806

- 位置：`internal/agent/rpc/backup_provider.go:585`（`unpackEntry` symlink 分支）。
- 问题：`unpack` 只对每个条目自身 `target` 做 Zip-Slip 检查（:548），`hdr.Linkname` 未验证；
  恶意 tar 先写符号链接 `evil -> /etc/passwd`（路径在 destDir 内，过检），再写常规文件 `evil`，
  `os.OpenFile(target, O_CREATE|O_TRUNC|O_WRONLY)`（:571）跟随链接，以 agent 用户（常为 root）
  截断/改写恢复目录外任意文件。
- 修复：symlink 分支拒绝绝对 `Linkname`，相对目标解析（`filepath.Clean(filepath.Join(filepath.Dir(target), hdr.Linkname))`）
  必须仍落在 `destClean` 内（`resolved != destClean && !strings.HasPrefix(resolved, destClean+string(filepath.Separator))` 即拒），
  `os.Symlink` 前完成校验；回归测试 `TestBackupRestoreSymlinkEscapeRefused` 覆盖绝对逃逸、
  相对 `..` 逃逸、同目录名覆盖三形态 + 树内合法链接保留。

### P1 Guacamole 连接孤儿隧道竞态 — 已修 717ac3b

- 位置：`web/src/components/GuacamoleModal/index.tsx:142`。
- 问题：`connect` await `createRemoteTicket` 无取消标记，票据在途时关闭弹窗/卸载组件
  （Workbench 中父组件保挂载）后仍执行 `client.connect()`/`setState('connected')`——清理已跑，
  孤儿 guacamole/WebSocket 隧道存活，刚关闭的弹窗被重新拉回已连接视图。
- 修复：`connectGenRef` 连接代数 ref；票据返回后、建隧道前、catch 早退三处校验代数；
  `onTimeout`/`handleDisconnect`/`handleClose`/卸载 effect/`visible=false` effect 全部 bump 代数；
  3 条回归测试（票据未返回即关闭、保险箱保存在途即关闭、票据失败但已关闭）。
  web 块覆盖登记 10 条行号随插入同步（151→156、152→157、185→194、186→195、271→287、
  322→338、359→375、407→423、610→637、634→661），TZ=UTC 全量 lcov 门禁绿（310/310）。

## P2（17 条，记 todo）

### Go agent/core 侧

| ID | 类别 | 位置 | 问题 | 修复方向 |
| --- | --- | --- | --- | --- |
| G1 | race | internal/agent/agent.go:635 | `messageLoop` 捕获 `conn` 无连接代数标记：陈旧循环在 `codec.ReadMessage` 内被去调度（或在 :628 nil-conn 窗口睡眠），`reconnect()` 已建 conn2 并复位 `reconnecting=false` 后唤醒，其 `go a.reconnect()` CAS 成功 → `closeCurrentConn()`（:714）拆掉新连接；nil 窗口路径则与 ：737 新循环并发形成双 websocket 读协程（Gorilla 禁并发 ReadMessage，破坏帧）。 | 连接代数计数器；`messageLoop` 代数失配即退出（不重连），仅连接所有者可派生下一循环。 |
| G2 | race | internal/proxy/mgr/manager.go:460 | `ProxyConn.LastRead` 写在 `conn.mu` 下（:286），`cleanupIdleConnections`（:460）/`GetProxyStatus`（:388）/`GetAllStatus`（:415）只持 `proxy.mu` 读——两把不重叠的锁护同一字段，`time.Time` 结构撕裂读可误判活跃代理为空闲并中途关闭。 | 读 `LastRead` 时持 `conn.mu`，或改 `atomic.Int64` unix-nano。 |
| G3 | leak | internal/proxy/handler.go:275 | `readFromTarget`（:244）在目标侧关闭（:275）与 `HandleProxyData` 写错路径（:213）关闭 conn 但都不 `delete(h.conns, conn.ID)`；凡以目标侧关闭收场的代理会话永久残留 `h.conns` 条目，长跑 agent 上 RDP/SSH 会话累积泄漏 `AgentTargetConn`（缓冲/TLS 态）直至进程退出。 | `readFromTarget` 的 deferred 清理与写错返回前 `delete(h.conns, conn.ID)`。 |
| G4 | leak | internal/agent/rpc/stack_task_follow.go:88 | `FollowTask` 直接 `p.taskFollows[followID] = sess` 覆盖旧会话，不停旧 `pumpTaskLog` 协程（300ms 轮询到任务结束为止）；followId 复用时派生无上界重复泵协程与重复 `proxy_data` 流。 | 覆盖前关旧会话 `stop` channel 并等待其泵协程退出。 |
| G5 | leak | internal/server/api_guacamole.go:476 | guacd 握手 `hsReader.ReadString(';')` 无读期限（全文件无 `SetReadDeadline`）；guacd 接受 TCP 后停滞则 handler 协程永久阻塞在握手，泄漏协程/guacd 连接/relay 监听器与 `guacSessions`/`remoteSessions` 注册条目。 | 握手与后续泵设读期限，停滞即拆除会话。 |
| G6 | error-handling | internal/docker/client.go:268 | `GetLogs` 只读一次（`reader.Read(buf)` :268）进 1MB 缓冲即返回 `string(buf[:n])` 无读循环；日志流超过首次 Read 交付量即静默截断无错误无标记（500KB 日志返回开头几 KB 乱象）。 | 循环读到 EOF 或 1MB 上限（封顶时附截断标记）。 |

### Go server 侧

| ID | 类别 | 位置 | 问题 | 修复方向 |
| --- | --- | --- | --- | --- |
| S1 | nil | internal/server/api_backups.go:187 | `requireAgentRclone` 取 `cap := agent.GetCapability("backup")` 后直接 `cap.Metadata["rclone"]`；无 backup 能力的 agent 返回 nil，创建/更新带 `remote_dest` 的备份配置时 nil 解引用 panic（net/http 兜成断连 500）而非预期 400。 | `cap == nil` 守卫（或 `HasCapability`）后再触 `Metadata`。 |
| S2 | oob | internal/server/backup_loop.go:105 | `NextBackupRunAt` 对 `daily@` 前缀 `SplitN(...,":",2)` 后无长度检查直接取 `parts[1]`（`daily@2`/`daily@` 等历史/导入行）→ 索引越界 panic 在 `startBackupLoop` 协程内无 recover，**崩整服务进程**；`ValidBackupSchedule` 守了同一解析，校验器与调度器不一致。 | `len(parts) != 2` 返回 0（镜像 `ValidBackupSchedule`）。 |
| S3 | race | internal/server/backup_loop.go 同源 | `ProxyConn.LastRead` 在 `handleConnection` 零值初始化：新接受尚未发首字节的连接 `now.Sub(time.Time{})` 远超 5 分钟，首个清理 tick（30s 后）强关健康仍在握手的连接（服务端先说话的协议）。 | 创建时 `LastRead = time.Now()`，或 `LastRead.IsZero()` 跳过清理。 |
| S4 | race | internal/server/api_workflow.go:490 | `handleWorkflowRun` check-then-act：`ActiveWorkflowRun`（:490）与 `CreateWorkflowRun`（:541）两次独立 DB 操作，`(workflow_id, status=running)` 无唯一约束（Status 仅普通索引）；并发 POST /run 双双过检创建两个 running run，双 `advanceWorkflow` 编排在目标 agent 上**重复执行工作流步骤**。 | 条件创建（`WHERE NOT EXISTS running` 单语句）或 partial unique index。 |
| S5 | leak | internal/server/api_guacamole.go:447 | `guacSessions[sessionID] = session` 后，五条握手失败路径（:465/:477/:486/:514/:537）关 conn/guacd/relay、更新台账但从不 `delete(guacSessions, sessionID)`——只有成功路径（:588）的 `closeGuacamoleSession`（:735）移除；每次失败握手泄漏一个 `GuacamoleSession`，`terminateLiveSession`（api_remote.go:684）还可能事后"关闭"从未连上的会话。 | 各失败路径调 `closeGuacamoleSession(session)`（幂等）或 `delete`。 |
| S6 | leak | internal/server/guac_relay.go:138 | `Close()` 在 `r.mu` 下快照 `connIDs` 后放锁逐个 `removeConn`；监听器关闭前 `acceptLoop` 刚派生的 `handleConn` 可在快照后向 `r.conns` 插入新 conn——该 guacd 侧连接永不关闭、不向 agent 发 `proxy_close`，泄漏 guacd fd + agent 侧目标连接 + `pumpToAgent` 协程直至超时。 | 持 `r.mu` 关全部 conn，或快照循环后复查 `r.conns`。 |

### Web / mobile 侧

| ID | 类别 | 位置 | 问题 | 修复方向 |
| --- | --- | --- | --- | --- |
| W1 | race | web/src/components/TerminalModal/index.tsx:106 | `connectWebSocket` await 票据后无条件建 WebSocket；票据在途关闭/卸载留下孤儿开放 socket，其 `onmessage` 向已 dispose 的 xterm `terminal.write()` 抛未捕获错误并泄漏 PTY 会话。 | await 前捕获代数/closed ref，失配即关闭新建 ws 并退出。 |
| W2 | race | web/src/pages/Recordings/index.tsx:151 | `PlaybackModal.play` 每事件一个 setTimeout 无代数守卫：首次调用仍在 await `getRecordingCast` 时切换倍速，慢的先到后到清掉后到的 timers，播放停在陈旧倍速。 | 每次倍速变更 bump 代数计数器，非当代调用跳过/清理 timers。 |
| W3 | leak | web/src/pages/ForgotPassword/index.tsx:49 | `startCountdown` 的 setInterval 存局部变量卸载不清；倒计时中导航离开（返回登录）后最多 60s 继续对已卸载组件 setState。 | timer id 存 ref，卸载清理（归零时也清）。 |
| W4 | error-handling | web/src/components/BackupCodesDisplay/index.tsx:21 | `navigator.clipboard.writeText(...).then(...)` 无 `.catch()`：http 部署下 `navigator.clipboard` 为 undefined 同步 TypeError；任何拒绝都是未处理 promise 拒绝；2s 复位 setTimeout 卸载后仍触发。 | try/catch 或 `.catch(...)`（带用户提示），复位定时器卸载清理。 |
| W5 | error-handling | web/src/pages/Domains/index.tsx:435 | `navigator.clipboard.writeText(snippet ?? '')` 无 `.then`/`.catch`：拒绝（剪贴板拒权/非安全上下文）是未处理 promise 拒绝，且 `message.success('已复制')` 在复制真正发生前就报成功。 | `.then(() => message.success(...)).catch(() => message.error(...))`（守 undefined clipboard）。 |

## P3（11 条，记 todo）

| ID | 类别 | 位置 | 问题 | 修复方向 |
| --- | --- | --- | --- | --- |
| G7 | leak | internal/agent/rpc/job_provider.go:99 | `Exec` 建 `context.WithTimeout`（:99）后 `cmd.CombinedOutput()` 返回（:107）从不 `cancel()`；每个提前完成的 exec 都让计时器挂到整个 per-job 超时（至 300s），泄漏计时器并悬挂 context 树。 | `WithTimeout` 后立即 `defer cancel()`。 |
| G8 | error-handling | internal/docker/client.go:293 | `GetContainerStats` 的 `stats.Body.Read(buf)`（:293）忽略返回错误，部分/失败读产出空/畸形 stats map 无错误上抛；更广地所有 docker 方法跑 `context.Background()` 无 per-op 期限，daemon 挂起即永久挂住 RPC 协程。 | 检查 Read 错误并关 `stats.Body`；docker 方法加 per-call 超时。 |
| G9 | error-handling | internal/agent/rpc/logs_follow.go:115 | `FollowStart` 容量检查 `len(p.fs.follows) >= logsFollowMaxActive`（:115，上限 16）先于已存在 followID 判断：16 个活跃时重启既有 follow 被误拒"too many active log follows"，而 ：119-128 的替换逻辑本就支持同 ID 重启。 | 容量检查只计非替换项（先查既有 ID 再测容量）。 |
| G10 | race | cmd/cockpit-probe-agent/main.go:116 | 关停时 `flushStatus` 在 `close(transitions)` 后写 `transitions = nil`（:116），alerter 闭包（:86-96）并发读 `transitions` 并 `select { case transitions <- tr: ... }`；指针写是数据竞态（select-default 只防 send-on-closed panic，防不了 racy 读）。 | `transitions` 加 mutex 守护，或仅在属主协程内置 nil。 |
| S7 | leak | internal/proxy/handler.go:133 | `h.conns[p.ConnID] = agentConn`（及 SSH 分支 ：165）直接覆盖同 connID 旧条目不关旧 `AgentTargetConn`；服务端对已开放 connID 重发 `proxy_new`（丢 ack 重试/connID 复用）即泄漏旧目标连接与其 `readFromTarget` 协程。 | 覆盖前关闭并删除既有条目。 |
| S8 | concurrency | internal/notification/service.go:149 | `SendNonBlocking` 在 `context.Background()` 上派生无追踪协程，不挂服务生命周期/任何 WaitGroup；服务关停中发送，webhook/telegram/ntfy 客户端可在 DB 关闭后运行（仅受 15s 超时约束），协程对关停协调不可见。 | 穿生命周期 context 或入 WaitGroup 追踪。 |
| S9 | error-handling | internal/server/password_reset_handlers.go:62 | 异步邮件协程经 `printf` 吞掉 `SendPasswordResetEmail` 错误——本文件中 `printf` 即空操作（:166-168）；重置邮件发送失败无处记录而 API 仍答"如果该用户存在，重置邮件已发送"（用户以为邮件已发，实际没有）。 | 改 `log.Printf`（并追踪协程）。 |
| S10 | error-handling | internal/server/backup_loop.go:93 | `ParseBackupSources` 忽略 `json.Unmarshal` 错误：`sources` JSON 损坏的配置产出空源清单照发 `backup.run`——备份看似派发并完成实则零文件，不响亮失败。 | 返回错误，`startBackupRun` 拒绝/中止。 |
| W6 | leak | web/src/components/GuacPlayer/index.tsx:100 | effect 将 `rec.getDisplay().getElement()` append 到 `displayRef.current`，cleanup 只 pause/disconnect 不移除元素；effect 重跑（main.tsx 开启的 StrictMode 双调、blob 变更）时旧 canvas 与新 canvas 同留 DOM。 | cleanup 中先从 `displayRef.current` 移除旧显示元素。 |
| W7 | error-handling | mobile/lib/pages/dashboard_page.dart:19 | `RefreshIndicator(onRefresh: () async {})` 空操作：下拉刷新即收不重取，数据滞留；`Future.wait` 在 `build` 里创建，父组件每次重建重启全部三个请求。 | future 缓存为字段（initState），`onRefresh` 重赋值并 setState。 |
| W8 | race | mobile/lib/pages/terminal_page.dart:136 | 重连时旧 channel 的流订阅不取消，其 `onDone`/`onError` 可在新连接建立后触发；`_fail` 无条件 `_state = _ConnState.error`（suppressClose 只藏消息不改状态翻转），工作正常终端上弹出虚假错误横幅。 | 重连开始时取消 `_sub`（并关旧 channel）再起新连接。 |

## 修复提交

| commit | 内容 |
| --- | --- |
| dc81806 | fix: reject symlink escape in backup restore（P0，含回归测试） |
| 717ac3b | fix: invalidate in-flight guacamole connect on close (orphan tunnel)（P1，含 3 条回归测试 + 覆盖登记同步） |

## 剩余

28 条（P2 × 17 + P3 × 11）已记 todo.md「代码深审 P2/P3（2026-11-01）」一节，逐条待修；
均为仓内代码修复面（无需真机/凭据），按账本顺序推进。
