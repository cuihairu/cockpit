# luna-review-2026-10-18 整改回执

对应报告：[luna-review-2026-10-18.md](./luna-review-2026-10-18.md)（表一缺陷 / 表二文档不一致 / 表三能力评估）。
处理纪律（todo.md 审核整改条目）：先核对再动手，每项给 file:line 证据；表二以代码为准改文档；表三只评估不实现。

## 表一 缺陷整改

### 1. Agent 注册缺少默认身份验证 —— 已修

**核对**（先核对再动手）：

- `internal/server/websocket.go:167-236` `authenticateAgentRegistration`：未知 agent（`storage.ErrNotFound`）原样放行；存量无 `SecretHash` 的 agent 原样放行（注释「兼容历史数据」）。
- 入账路径核实：`internal/storage/storage.go:429` `UpdateAgentSecret` 对不存在的 ID 是 0 行更新无报错——无法为从未注册的 agent 预置 secret，「注册即建账」是唯一入账路径，任何能访问 `/ws` 的人都可冒充任意新 ID 入注册表。缺陷成立。

**修复**：

- 新增 `server.agent_registration` 配置（`internal/config/config.go:157-175,371-378`）：`allow_unknown`（开发环境自由注册）、`allow_secretless`（存量无 secret agent 显式放行）、`enrollment_secret`（新 agent 首注册口令）。三项默认全关 = fail-closed。
- `internal/server/websocket.go:188-236`：未知 agent 仅在 `allow_unknown` 或常量时间比较匹配 `enrollment_secret` 时放行，否则拒绝（`registration_forbidden`）；存量无 secret agent 默认拒绝（`authentication_required`）。
- enrollment 成功后把其携带 secret 的哈希落库（`internal/server/websocket.go:175-191,248-264`；注入点 `:24-26`），此后重连走既有 per-agent 校验；enrollment 口令轮换不影响已注册 agent。哈希或持久化失败会回滚 Registry 并拒绝注册，不再留下无 secret 的在线 Agent。
- 测试：`internal/server/websocket_registration_policy_test.go`（默认拒绝 / dev 放行 / enrollment 三态与重连闭环 / 哈希失败分支 / 并发注入点竞态 9 例）；测试舰队构造器（`newTestServerWithDB`、`covNewServer`、`covLoopServer`、`covROServer`）显式 dev-mode 策略，等价整改前自由注册语义，安全默认由新用例覆盖。
- e2e：`scripts/e2e-smoke.sh` 改走 enrollment 准入（config 置 `enrollment_secret`、agent `--secret` 携带），全链路 exit 0。
- 部署入口：`config/cockpit.yaml:12-15` 与镜像内 `deployments/docker/config.yaml:5-8` 只从 `${AGENT_ENROLLMENT_SECRET}` 读取；Compose `deployments/docker/docker-compose.yml:29-40` / `docker-compose.yml:23-34`、`.env.example:9`、Linux/Windows Server 安装配置（`deployments/install-server.sh:72-81`、`deployments/install-server.ps1:68-75`）均接线。Windows Agent 安装器支持交互输入和静默 `/SECRET`（`packaging/windows/agent.iss:69-97,135-142`）；Getting Started 与 README 已更新命令示例。
- 文档：`docs/guide/architecture.md` 安全边界一节更新；中英文 Getting Started 同步说明 enrollment 与后续 Agent secret。

**升级注意（破坏性变更）**：升级后未知 agent 与无 secret 存量 agent 默认无法注册/重连。已有部署需在 server 配置 `agent_registration.enrollment_secret` 并给 agent 加 `--secret`，或临时 `allow_secretless`。拒绝消息含处置指引。

### 2. 登录接口缺少尝试限流 —— 已修

**核对证据**：`internal/server/server.go:329-337` 将 `/api/auth/login` 注册为公开路由；`internal/auth/handler.go:60-75` 解码后直接调用 `VerifyPassword`，失败立即返回 401；当前 `Server`/`auth.Service` 未见失败计数或限速。登录审计在 `internal/server/server.go:560-595`。

**设计（先于实现）**：在 Server 登录入口增加进程内限流器，按来源 IP 与规范化账号双维度计数；失败达到阈值后短时返回 429 + `Retry-After`，成功清除该组合记录。来源使用 socket `RemoteAddr`，不信任可由客户端伪造的转发头。限定记录上限并清理过期桶；已有登录审计继续覆盖拒绝事件。属于节点级缓解，重启会清空，不能替代反向代理/WAF 或跨副本共享限流。

**修复与验收**：`internal/server/login_limiter.go:13-149` 实现 IP 20 次/账号 8 次每 15 分钟阈值、5 分钟冷却与 20,000 桶上限；`internal/server/server.go:563-630` 在登录前限流、失败计数并审计 429。`internal/server/login_limiter_test.go:13-120` 覆盖 IP/账号键、成功清理、窗口过期、容量上限、伪造 XFF 不生效及 Retry-After；定向测试通过。

### 3. 一次性远控 ticket 明文写入日志 —— 已修

**核对证据**：`internal/server/ticket.go:46-65` 生成 16 字节随机 ticket、缓存五分钟，并在第 63 行把完整 ID 写入日志；消费路径为 `internal/server/ticket.go:68-95`。日志副本会扩大有效凭据的暴露面。

**设计（先于实现）**：移除 ID 明文，不用可逆替代值；只记 ticket 创建事件与用户名。增加日志捕获回归测试，确认输出中不含返回给调用方的 ticket。

**修复与验收**：`internal/server/ticket.go:62-64` 只记录创建事件和用户名；`internal/server/ticket_test.go:58-77` 捕获日志并断言不含调用方 ticket ID；定向测试通过。

### 4. Docker guacd 无法回拨 Server 回环中继 —— 已修

**核对证据**：`internal/server/guac_relay.go:61-80` 固定监听 `127.0.0.1:0` 并把该地址交给 guacd；`internal/server/api_guacamole.go:362-384,511-524` 先连接 guacd、再将 relay host/port 放入 connect 指令；官方双容器部署 `deployments/docker/docker-compose.yml:16-18,47-48` 将 Server 与 guacd 放在 bridge 网络，不共享 loopback。现有 `internal/server/guac_integration_test.go:190-215` 只验证容器 guacd 握手，未覆盖其回拨中继；`217-271` 的完整 VNC 集成测试使用 host-gateway 作为目标地址，但未证明 relay 的目标地址不是容器自身 loopback。

**设计（先于实现）**：默认以 Server 到 guacd 已建立 TCP 连接所选中的本地 IP 作为 relay 的精确绑定地址与通告地址；避免 `0.0.0.0` 广泛监听。为分离网络/跨机部署保留显式 `GUAC_RELAY_BIND_ADDR`（本地绑定 IP）和 `GUAC_RELAY_HOST`（guacd 可达的通告主机）覆盖；拒绝 unspecified bind。

**修复与验收**：`internal/server/guac_relay.go:63-128` 按连接路由地址绑定，允许具体 IP/主机名覆盖但拒绝 wildcard；`internal/server/api_guacamole.go:362-399,511-530` 把选定的 relay 地址交给 guacd。强化 `internal/server/guac_integration_test.go:344-408`：本地 Docker guacd 必须通过 host-gateway 回拨 relay，并经 fake Agent 实际连接 fake VNC；测试通过。当前环境已验证 host-network gateway 的本机容器拓扑，不代表所有跨机防火墙/NAT 均实测。

### 5. 忘记密码响应可枚举账号并泄露邮箱 —— 已修

**核对证据**：`internal/server/password_reset_handlers.go:29-47` 对不存在用户与未绑定邮箱用户返回不同文案；`64-70` 对已绑定邮箱用户返回 `Email` 原文和脱敏邮箱；该路由公开注册于 `internal/server/server.go:332-337`。

**设计（先于实现）**：三个用户状态都返回同一 HTTP 200、同一通用 JSON 提示；成功时仍异步发送邮件，但响应不包含完整/脱敏邮箱。重置令牌生成异常也走统一提示，避免构造额外枚举分支。加三态响应相等断言。

**修复与验收**：`internal/server/password_reset_handlers.go:11-71` 对不存在、无邮箱、成功及令牌生成失败统一响应，`auth.ForgotPasswordResponse` 仅保留 message；`web/src/pages/ForgotPassword/index.tsx:19-46,128-164` 不再展示邮箱。`internal/server/password_reset_handlers_test.go:84-112` 断言三态响应完全一致；web 1123 项测试通过。

## 表二 文档与实现不一致

**核对及处理**：逐项依代码确认，表二只改中文/英文对应文档，不为对齐文档改业务代码。

- ticket 传递：`internal/server/api_remote.go:45-54` terminal 从 WebSocket 子协议读取；`internal/server/api_guacamole.go:306-312` Guacamole 先读 URL `?ticket=`，无 query 才回退子协议。更新 `docs/guide/architecture.md:121-127`、`docs/guide/protocol.md:167-175`（及英文配对 `architecture.en.md:121-127`、`protocol.en.md:168-175`），明确按通道区分。
- Job 异步：`internal/server/api_jobs.go:18-27,95-101` 创建 pending 后后台派发并立即返回；更新 `docs/guide/jobs-design.md:10-26,127-134`（及英文配对）陈旧的“不做异步/同步等终态”边界说明。
- guacd 拓扑：实现整改后按代码实际写清 relay bind/advertise 地址规则、默认同网络部署与显式覆盖变量；同步 `docs/guacd-deploy.md:49-75,127-142`、部署 Compose 注释和英文配对。检查 `deployments/guacd/docker-compose.yml:17-20` 仅映射 guacd 服务端口，不能据此误称 relay 已可跨机回拨。
- 公共 API 认证：`internal/server/server.go:339-362` 常规 API 走 JWT；`internal/server/api_stacks.go:47-53` 精确注册 webhook 路由，`internal/server/api_stack_webhook.go:13-25` 以 `X-Cockpit-Token` 认证。更新 `docs/guide/architecture.md:100-117`（及英文配对）中“其他 API 均由 JWT 保护”的绝对表述。

## 表三 能力评估

**仅评估，不实现**（依据报告表三及上述核对证据）：

1. **Agent 准入/secret 生命周期（高）**：P0 已由本次 #1 fail-closed + enrollment 准入处理；后续 P1 可补管理员预置 Agent、轮换/吊销、迁移盘点与审计、限时 enrollment token。先有生命周期设计与旧部署迁移方案，再排期。
2. **登录与密码重置限速（高）**：P0 本次只修登录入口限流；P1 单独为 forgot/reset/verify-code 做按 IP、账号及邮箱的限速与发送冷却，避免邮件轰炸和验证码猜测；多实例时需要共享存储，配合 CAPTCHA/反向代理策略评估。
3. **持久化任务恢复/重试（中高）**：`internal/server/orphan_run.go:10-20` 当前将重启遗留任务落为 failed。P1 先明确“至少一次/至多一次”语义、幂等键、目标执行去重、租约/心跳和人工重试审计；不应直接把可能已执行的命令自动重放。
4. **Agent/资源范围授权（中；团队场景偏高）**：`internal/server/rbac.go:29-59` 为角色/模块权限模型，`docs/guide/rbac-design.md:137-140` 明确暂不做行隔离。按真实多租户需求触发 P1，先建资源归属模型与跨 API 授权矩阵，再实现；个人自托管场景暂不排期。
5. **Job 实时输出/进度（中）**：`internal/agent/rpc/job_provider.go:99-108` 当前收集进程完成后的输出，`docs/guide/jobs-design.md:138-139` 明确实时流不在当前范围。P2 再与 Workflow/终端传输统一设计，需包含背压、断线重连、截断/保留策略与权限审计；不新增第二套任意命令数据通道。
