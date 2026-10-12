# 架构决策记录（Decision Log）

> 2026-10-07 起设立。拍板授权后，凡此前标注「挂起待拍板 / 有明确需求时推进」的项，
> 由维护代理按建议方案直接定案并在此登记：**定了什么 / 为什么 / 备选是什么 / 何时重开**。
> 用户后续审核发现问题照常改，改动追加记录不删旧案。

## D-2026-10-07-1 grdp/旧桌面路线删除（去留拍板）

**定了什么**：删除自研 grdp 兜底路线全量面——`internal/agent/rdp/`（grdp 客户端 + handler）、
`cap_rdp*.go`（`-tags rdp` 构建分流与 `rdp-client` capability 上报）、agent 侧
`desktop_new/desktop_data/desktop_close` 消息路由、server 侧 `api_desktop.go`/`api_vnc.go`
（旧桌面/VNC WS 入口）与 desktop 协议消息族、web 侧孤儿组件 `DesktopModal`/`VNCModal`
（零页面挂载）及其测试、四个 workflow 的 `-tags rdp` 步骤。**RDP/VNC 功能不受影响**——
入口统一走 GuacamoleModal（guacd + guacamole-common-js，2026-09-30 真机验收 16 场景全过），
agent 仅作 TCP 转发（proxy_* 消息族），与 grdp 链路无交集。

**为什么**：
1. 2026-09-24「保留兜底」的拍板自带重开条件「等 Guacamole 方案真要落地时再单独评估」
   ——该条件已触发（三协议真机验收闭环、部署文档/acceptance 全勾），重评结论倾向删除；
2. `DesktopModal`/`VNCModal` 已零挂载（孤儿），`hasRdpClient` 零消费方——UI 面早已只剩
   Guacamole 单入口，「并存」实际只剩后端死代码；
3. `-tags rdp` 构建矩阵（test/agent-test/build/nightly 四 workflow）+ grdp 回调闭包覆盖
   债（需真实 RDP 服务器）是永久维护税，换来的兜底场景只有「guacd 容器挂了」——而 guacd
   是文档化的一等部署组件，为单容器故障保留第二套完整 RDP 协议栈不成比例；
4. 「不重造底层」边界纪律（2026-10-04 战略评审）：grdp/noVNC 自研路线正是重造底层。

**备选**：保留并存（原案）——否决理由如上；部分删除（只删 UI 留 agent）——否决：
半删状态链路不通且更难理解。**恢复路径**：git 历史完整保留，`git revert` 单笔可回。

## D-2026-10-07-2 鸿蒙端暂缓（立项拍板）

**定了什么**：鸿蒙（HarmonyOS/OpenHarmory）端**暂缓立项**，非永久不做。不在仓库创建
鸿蒙工程，不在无验证路径下盲写代码进 main。

**为什么**：2026-10-05 第三版口径给鸿蒙的定义是「只写代码、编译过即可」，但现实核查：
① Flutter 官方不含 OHOS 目标，需第三方 fork（openharmony-sig/flutter_flutter_ohos）；
② 本机无工具链、GitHub hosted runner 无 OHOS 镜像——**「编译过」在任何可用路径上都
不成立**；③ 仓库无既有工程，从零盲写一个不可编译验证的应用，违反全绿纪律（未验证的
代码规模越大越是负资产）。iOS 侧同口径已闭环（build-ios job 首跑绿），鸿蒙半边缺的
正是验证路径本身。

**备选**：按口径盲写代码——否决（不可验证）；搭 OHOS SDK 的 CI 实验路径——列为
重开后的第一步而非现在做（主线价值存疑，易烧时间）。**重开触发条件**（满足任一）：
出现真实鸿蒙设备使用需求；openharmony-sig fork 成熟到可在 GitHub Actions 稳定出
HAP（届时先搭 CI 验证路径再写码）；OHOS 进 Flutter stable。

## D-2026-10-07-3 防火墙管理启动（推进拍板）

**定了什么**：路线图 P2 唯一未启动项「防火墙管理（iptables/nftables 规则可视化）」
正式推进，按既有 per-agent 模式做：设计文档（`docs/guide/firewall-design.md`，决策
D1-Dn）→ M1 只读观测（agent capability + 规则快照 RPC + Web 页只读三态）→ 后续批次
再评估写操作。

**为什么**：用户拍板授权推进 todo 全量；防火墙是安全面最后一块只读观测空白，与
SMART/NAS/Overlay 同族（观测→告警→（远期）操作），能力复用现成（Commander argv
直调、capability 探测、per-agent 端点、动作单入口）。原「有明确需求时推进」的需求
判定收敛为：个人 homelab 控制台看一圈防火墙状态是高频巡检动作，且与网络资源面板
（域名/证书/反代）互补。

**备选**：继续搁置——否决（授权令明确推进全量）；直接做写操作（规则增删）——否决：
防火墙写操作风险等级最高（锁死远程访问），必须只读先行、真机验收后再议，与 Cron
「外部条目只读」既有纪律同构。**边界**：只观测 iptables/nftables 现状与 cockpit 名下
标注，不碰用户自有规则；云安全组（AWS SG 等）不在范围。

## D-2026-10-08-1 region/zone 空值语义（todo Phase 2.2 拍板）

**定了什么**：inventory 资源与 agent 注册的 region/zone 校验定为**为空允许 + 标记
`unknown`**——不拒绝同步/注册：显式字段（非空白）优先保留；省略或空白统一落库
`unknown`；用户在 labels 里显式给的 `region`/`zone` 键不被空字段覆盖。落在
`internal/inventory/sync.go`（`locationOrUnknown` 列 + `applyLocationLabels` labels）
与 `internal/server/server.go` `toStorageAgent`（注册兜底）。

**为什么**：个人 homelab 场景地域常未知，校验失败会逼用户编造值；`unknown` 显式标记
让「未填」与「留空展示」可区分，与 agent 侧 `detectLocation` 默认 `unknown` 的既有
行为对齐（此前资源侧留空、agent 侧 unknown 两套口径不一致）。

**备选**：直接校验失败（拒绝同步）——否决：单文件 inventory 手工维护，硬失败伤易用性
且与「同步容错、Errors 计数不中断」的既有 sync 纪律冲突。**重开条件**：多租户/CMDB
严格口径需求出现时再议。

## D-2026-10-08-2 代理 proxyType 拒收 udp（文档一致性审计遗留观察拍板）

**定了什么**：`POST/PUT/PATCH /api/proxies` 的 `proxyType` 校验收紧为**只收
`tcp`（空值默认 tcp），其余一律 400**（错误体说明数据面仅 TCP）。落在
`internal/server/api_proxy.go` create/update 两处；protocol 与 storage 注释同步
标注「udp 预留未实现」。

**为什么**：文档一致性审计（docs/审计-文档一致性.md A4）发现 API 此前接受
`proxyType: "udp"`（校验+落库均通过），但 Agent 侧转发实现只按 TCP 拨号
（`proxy/handler.go` `DialTimeout` 硬编码 `"tcp"`）——用户会拿到一个名为 UDP
实际走 TCP 的静默错转代理。web/mobile 均无 udp 入口（零 UI 暴露、无存量用户），
诚实失败优于静默错转。

**备选**：真实现 UDP 数据面转发——否决：无 UI 需求、无消费场景，无连接语义的
数据报转发要走独立设计立项，不值当；仅文档注记不改行为——否决：陷阱永续存在，
每轮审计重复发现。**重开条件**：出现真实 UDP 代理需求（DNS 隧道等）时走设计
立项实现数据面，届时放开校验。

## D-2026-10-08-3 过期 agent 自动判定与列表隐藏（用户「清理过期 agent」闭环）

**定了什么**：三件套——①**心跳回写 DB `last_seen`**（`dispatcher.go`
handleHeartbeat 每 30s 心跳调 `TouchAgentLastSeen`，此前只在注册/断连落库）；
②**服务端自动过期判定**（`alert_loop.go` cleanupLoop 每 30s tick 跑
`expireStaleAgents`：`last_seen` 超阈值仍标 `online` 的假在线行批量标 `offline`
并释放其注册表连接，清理动作带日志）；③**阈值可配可见**：`AGENT_EXPIRE_MINUTES`
env 优先，`yaml agent.expire_minutes` 兜底，缺省 5 分钟，负值关闭；`/api/status`
下发 `agentExpireMinutes`，web Agents 页顶部展示「过期阈值：X」。列表侧默认**隐藏**
过期行（`last_seen` 超阈值即隐藏，不看 status），Alert 提示「已隐藏 N 台」+ 显示
切换；**DB 行不删**——密钥/标签/历史档案保留，物理清除走既有手动「清理离线 agent」
钮（thresholdMinutes 口径不变）。

**为什么（此前为何屡提不实现——三处断链）**：077b10f（10-04）只做了**手动清理**
这一层（`POST /api/agents/cleanup` + CleanupOfflineButton），而自动判定链整条缺失：
(a) 心跳**从不写 DB**，`last_seen` 冻结在注册时刻，服务端重启后全部 DB 行永远
`status=online`（假在线幽灵，DB 侧无任何可判定的时间证据）；(b) cleanupLoop 只清
**内存注册表**（60s 超时关连接），DB 幽灵行无自动判定、无人清理；(c) 阈值只是弹窗
里自由填的入参，无服务端配置、UI 不展示，「过期」概念在产品面不存在。本批把
(a) 证据、(b) 判定、(c) 配置可见三处补齐。

**过期 ≠ 删除的边界**：自动删行会顺带删掉 `secret_hash`（agent 密钥配置）、标签
关联与档案——agent 短暂离线即被摘除会造成重新注册时密钥失效、清单丢失；且
`authenticateAgentRegistration` 对无密钥哈希行放行（历史兼容），自动删行等于开了
ID 冒名注册的口子。故过期只做「判定 + 标离线 + 列表隐藏 + 资源释放」，删除权留给
人工按钮（删除时不可恢复，弹窗已有二次确认）。

**备选**：过期即自动删 DB 行——否决（密钥/档案丢失 + 兼容放行口子，见上）；只改
web 不动服务端——否决：DB 侧假在线行无从判定，隐藏口径没有数据基础。**重开条件**：
若需要「离线 N 天自动摘除清单」的资产轮转策略，应加**独立的保留期配置**（默认远长
于 5 分钟，如 30 天）而非复用过期阈值，走 todo 立项。

## D-2026-10-12-1 密码箱第二波开工（巡检授权自拍板）

**定了什么**：按 `vault-design.md` V6 第二波登记直接开工（用户 2026-10-12 巡检授权自拍板，
硬验收①与硬件真机项维持等待）：①**服务端 API token 加 vault 源**——DNS（cloudflare
api_token / dnspod login_token / alidns access_key+secret_key）、overlay（zerotier /
tailscale api_token）、notification（ntfy[].token / webhook[].secret / telegram[].bot_token）、
SMTP（password），yaml 字段支持 `vault:<path>` 显式引用，server 启动 initVault 后一次性解析
进内存 config；**优先级 env > yaml > vault**（vault 只经显式引用生效，不做约定路径隐式查找
——隐式查找会让「vault 里有没有条目」悄悄改变配置语义）；引用解析失败（ErrNotFound/读失败）
**启动即 logFatal 不降级**（V7 同口径，错误消息只含 path 不含内容）；条目口径 `value`=凭据。
②**ACME 私钥列加密落库**（`acme_certificates` 账户/证书私钥明文列 → vault core v1 密文，
读兼容 v0/明文存量行，写全走加密）。③**inventory capability config 加密落库**
（`agents.capabilities` JSON 内 `config.*` 现明文且无任何消费方 → 落库加密而非改 ref）。
jenkins external ref 维持「随链接功能立项」不变。

**为什么**：第二波三项都是 M1 盘点表上的已知欠账——API token 明文 yaml/用户口令同权限面
暴露、ACME 私钥是域信任根（库文件拖走即私钥泄露）、capability config 随库落明文；M1 已把
加密与解锁口径收敛进 vault 包，此三项都是纯消费侧接线，无新抽象。授权依据：仓内决策
`vault-design.md` V6「第二波（登记后续）」+ 用户 2026-10-12 巡检令自拍板。

**备选**：①约定路径隐式回填（字段空即查 vault）——否决：排查「token 到底从哪来」三源
叠加不可解释，且无法区分「未配置」与「配置了但 vault 缺条目」；②capability config 改
vault ref——否决：该字段当前零消费方，改 ref 是无消费者的间接层，先加密落库、出现消费方
再议 ref；③ACME 私钥挪进 vault 条目（per-cert path）——否决：迁移面大且部署脚本
（acme.sh 对接）依赖行内取数，列加密保持表结构零迁移。

**重开条件**：外部密码箱 provider 落地后，`vault:` 引用语法天然跨 provider（path 语义由
provider 解释）；若出现「同一 secret 多处共享」，再议条目别名/继承。

## D-2026-10-12-2 远控 egress 多策略命中规则明确

**定了什么**：`remote_control.egress[]` 同一 Agent 配置多条策略时，命中规则为
「任一策略同时放行目标与端口即放行」的**并集**判定，与配置顺序无关；不跨策略拼凑
（A 策略放行的目标 × B 策略放行的端口不构成放行）。Agent 未出现在任何策略中 → 拒绝文案
指向 `remote_control.egress`（Agent 级）；目标出现在某策略但 (目标, 端口) 组合不被任何
策略覆盖 → 拒绝文案指向 `allowed_ports`（端口级）。实现在 `internal/server/remote_audit.go`
`matchRemoteEgress`，用 `agentListed`/`targetAllowed` 两标志位区分三态拒绝。

**为什么**：原实现是「首条匹配 Agent 的策略就判定」——单策略场景无差异，但多策略场景
顺序决定结果（A→目标对端口错拒、B→目标对端口对但到不了 B），语义不可解释；并集判定
让「每条策略是独立的放行声明」这一配置语义成立。Agent 级与端口级拒绝文案分流，避免
「Agent 配了策略却被当成没配」的误导。

**备选**：①首策略优先（顺序敏感）——否决：yaml 顺序不应改变安全判定；②跨策略拼凑
（任一策略放目标 × 任一策略放端口即放行）——否决：策略本意是「(目标, 端口) 配对授权」，
拼凑会放行无人声明过的组合，扩大暴露面。

**重开条件**：若未来需要策略优先级（如 deny 策略压过 allow），再引入显式 priority 字段；
当前只有 allow 语义，并集已足够。

## D-2026-10-12-3 ACME 证书多目标部署模型

**定了什么**：证书部署绑定从 `AcmeCert` 行内五列（一对一）升级为新表 `AcmeDeployTarget`
（`(cert_id, agent_id)` 唯一，含 CertPath/KeyPath/LastDeployAt/LastDeployError），单个证书
可部署到最多 8 个 agent。API 入参 `deployTargets[]`（agentId 必填、绝对路径、同 agent 去重），
视图输出 `deployTargets[]`；旧五列弃用（列保留只为回滚安全，json 不再输出），migrate 一次性
幂等 backfill 生成同内容 target 行并清空旧列。部署语义：逐目标 `file.write` 并逐目标回写状态；
手动部署端点 无目标 400 / 无任何在线目标 503 / **仅全部目标失败才 502**（部分成功 = 200）；
审计 `acme_deploy` 逐目标记录（不含 PEM）。实现 `internal/storage/acme_deploy.go` +
`internal/server/api_acme.go`，web 表单改 Form.List 多行目标。

**为什么**：原一对一绑定只覆盖「一张证书 → 一个网关」，实际场景一张泛域名证书要铺到
多台反代/网关（proxy-design.md D7 每个 https 站点各自引用路径），逐台手动搬证书正是 D14
要消除的环节；行内列扩展到 N 列不可行，独立表是唯一干净形态。部分成功不算失败：全部
失败才报错，避免「8 台里 1 台网络抖动」把整体标红误导排查；逐目标状态在 target 行与
响应里可见，故障定位不丢信息。

**备选**：①行内加 `DeployAgentIDs []string` JSON 列——否决：无法承载逐目标的路径与部署
状态，查询/更新都要整列读写；②全部目标失败才 502 vs 任一失败即 502——后者否决：续期
自动部署场景下，部分成功是常态（某台离线），整体报错会让巡检告警与实际状态脱节；
③旧列直接删除——否决：保留一版可回滚（迁移首日发现问题可直接回退二进制），下一版再删。

**重开条件**：回滚窗口过后（确认无回退需求）删除五列；若单证书目标数超 8 出现真实需求，
再议上限放宽或改为 target 组（group）引用。

## D-2026-10-12-4 远控会话生命周期登记收口

**定了什么**：将 `RemoteSessionManager` 从孤立的 REST 登记入口接线到 terminal /
guacamole 两条活会话链路。terminal 链路：`handleTerminalWebSocket` 在票据校验后
`Register(pending)`，proxy_new 下发成功后 `UpdateStatus(connected)`，
`closeTerminalSession` 与 `HandleTerminalClose` 各 `UpdateStatus(closed)`。guacamole
链路：`handleGuacamoleWebSocket` 在会话对象创建后 `Register(pending)` + 插入
`guacSessions` 全局索引，握手完成 + 审计开始后 `UpdateStatus(connected)`，5 条早失败
路径各 `UpdateStatus(failed)`，`closeGuacamoleSession` 内 `UpdateStatus(closed)` +
移出 `guacSessions`。`DELETE /api/remote/sessions/{id}` 语义从「删登记」收口为
「先终止后删登记」：先查 `terminalSessions` / `guacSessions` 两个数据面索引，命中
则调对应 close 函数（幂等 + 审计 + 台账收口），未命中走 `Delete` 软删。

**为什么**：原 `RemoteSessionManager` 只有 REST `/api/remote/sessions` POST/GET/DELETE
入口，web 前端从未调用（零引用），`UpdateStatus` 在生产代码中从未被调用——管理
员无法从 API 看到当前有哪些活跃远控会话，DELETE 也无法真正断开已建立的连接。
设计文档 D4 明确要求「`RemoteSessionManager` 会话登记（pending/connected/closed/
failed）」，本次接线使台账状态与活会话生命周期对齐。

**备选**：①只接线 terminal 不接线 guacamole——否决：guacamole 同样是远控会话，
管理语义应统一；②DELETE 只删登记不终止活会话——否决：管理员点「删除」期望断开
连接，留着活会话只删台账会造成「已删除但仍连接」的安全错觉；③引入独立会话 ID
而非复用会话自身 ID——否决：审计 `Session` 字段、录制 `SessionID`、`guacSessions`
索引均使用会话自身 ID，复用使台账与数据面同源，DELETE 命中同一 ID。

**重开条件**：若未来 web 前端接入会话列表展示，需补充 REST 查询权限点（当前
`terminal:write` 仅覆盖票据签发）；若 guacd 会话量增长导致 `guacSessions` 内存
压力，再议 LRU 淘汰策略。

## D-2026-10-12-5 Stack Git-to-deploy ①（Git worktree 化）口径

**定了什么**：stacks 根目录整体接入为一个 git 仓库（非 per-stack 仓库）；新增
`stack.git.status` / `stack.git.pull` 两个 RPC 与 `GET .../git`、
`POST .../git/pull` 两条 REST。接入流程 `init → symbolic-ref → remote add →
fetch → checkout -B`（要求远端分支已存在）；已接入再拉为 `fetch --prune →
merge --ff-only`，不自动 commit、不代做 rebase/merge。drift = 工作区相对
HEAD 的 porcelain 改动（`-uall` 逐文件展开），`.cockpit/` 与 `.git/` 在代码
里过滤而非依赖 `.gitignore`。安全侧：remote URL 拒绝 `::` 传输助手语法、
前导 `-` 与控制字符；分支名 `^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$` 且禁 `..`；
URL 内 userinfo 凭据回显前统一抹除。远端 URL 只存 agent 侧 `.git/config`，
server 不建表；写操作审计 `stack_git_pull`，状态查询不审计。

**为什么**：①整仓粒度——个人 stacks 场景通常就是单一仓库，per-stack 仓库
倍增配置成本且无真实需求；②不自动 commit——compose-file-first 原则下 UI
编辑就是 drift，替用户提交会让「本地改动」概念消失、与「保存即真相源」冲突；
③status 不联网——列表页每次加载都触发远端 fetch 会把远端故障传导成 UI 卡顿，
behind/ahead 反映上次 fetch 时点足够引导操作；④`::` 拒绝——git 传输助手
`ext::sh -c ...` 等价任意命令执行，URL 由 web 表单录入，必须入口侧封死；
⑤凭据抹除——git 报错原样回显带凭据的 URL，违反「凭据不落日志」红线。

**备选**：①per-stack git 仓库——否决：配置面翻倍，且 `.cockpit/` 保留目录
会出现在每个仓库；②服务端 webhook 自动 pull——否决：留给 P2 ②（webhook 触发
部署）统一做，且自动 pull + 自动重启的爆炸半径需要独立评审；③用 go-git 库
替代 shell out——否决：与 provider 既有 `docker compose` shell-out 惯例一致，
go-git 对 sparse/worktree/凭据助手支持不全；④`.gitignore` 过滤 `.cockpit/`
——否决：接入的是用户既有仓库，无权替用户改 `.gitignore`。

**重开条件**：出现多仓库真实需求（如多团队分区）再议 per-stack 仓库或多 remote；
P2 ② webhook 自动 pull 落地时需补 drift 保护策略评审（自动 stash？拒绝？）。

## D-2026-10-12-6 Stack webhook 触发部署 + 任务日志流式推送（P2 ②）口径

**定了什么**：新增 `POST /api/stacks/webhook`（forge 回调，token 即凭据：
头 `X-Cockpit-Token` 常量时间比对配置 `stacks.webhook_token`，令牌空 =
端点 404 关闭，默认关）。语义只做「拉取已接入 origin + 部署改动 stack」，
请求里的 `remote` 参数一律剥离（webhook 不能重定向部署源）。body 可选
`{agentId, branch}`，缺省扇出全部心跳在线 agent；响应 `{triggered:[...]}`
即返回、部署异步跑，成功落审计（username=webhook）+ 部署历史（映射 up），
RPC 失败只落 server 日志。agent 侧 `stack.git.deploy` 部署范围：接入前无
HEAD → 全部 stack；HEAD 前移 → diff 顶层目录；未动 → 不动；目标 stack
任务执行中记 skipped 不阻塞。drift 保护沿用 git 自身拒绝（`--ff-only`
失败即整体报错，不 stash 不代提交）——回应 D-2026-10-12-5 重开条件。
任务日志流式推送完全复用 logs-design.md F4 架构：agent `stack.task.follow`
经 proxy_data（`stacktask:` 前缀）回推增量 + proxy_close 回传终态；server
NDJSON 流配额全局 8 / 单 agent 4、帧缓冲 256 满则丢帧、断开补发 follow.stop；
web StackDetail 运行期 follow 替代轮询，打不开自动回退轮询。

**为什么**：①token 空即关——webhook 是无认证面的新增入口，默认开等于给
公网扫描器送「触发任意部署」按钮，同 notification 红线口径；②拒收 remote——
部署源必须以 agent 侧 `.git/config` 为准，接受请求里的 remote 等于让任何
拿到 token 的人把部署源重定向到任意仓库；③异步扇出——多 agent 串行拉取
会把 forge 回调拖到超时，回调方只关心「受理了没有」；④drift 不 stash——
自动 stash 会把用户未提交的改动藏起来再部署旧内容，爆炸半径不可见，
git 拒绝 + 报错原文落日志是唯一诚实的语义；⑤日志流复用 F4——follow 会话/
配额/背压/断开清理全套已在 logs 路径验证过，第二实现只会引入第二套 bug。

**备选**：①webhook 带 secret 签名（HMAC payload）——否决：个人场景 forge
多在局域网，token 头已够，HMAC 需要每个 forge 单独配 secret 文档成本高；
等真实需求出现再升级；②同步等部署完成再响应——否决：部署是分钟级任务，
回调方（forge）超时通常 10s 内；③UI 侧 SSE 替代 NDJSON fetch 流——否决：
EventSource 不能带 Authorization 头，与 followLogs 同款 fetch 流保持一致；
④部署范围按 changed files 精确到文件级重启——否决：compose 栈的部署单位
就是 stack 目录，文件级无意义。

**重开条件**：出现多 forge / 多 secret 需求（升级 HMAC 签名）；大仓库 fetch
常态化超时（考虑 agent 侧后台 fetch 队列替代同步 RPC）。
