# Cockpit Todo

需求清单：`- [ ]` 待办、`- [x]` 已完成（一行证据：commit + 测试/验收规模）。过程细节查 git log 与 docs/。

执行纪律：

- 单任务单 commit；commit 信息英文简短，不提 AI；push 前 `git pull --rebase`。
- 先落设计文档再动码；测试与构建绿才 push。
- 不打版本号 tag、不发 Release；版本号由用户手动给出。
- 密钥不进仓库；解锁口令与明文凭据不落日志、不进错误消息；yaml 密钥字段只用 `${VAR}`。
- 真机验收不代测：等硬件/凭据到位按 `docs/guide/acceptance-checklist.md` 推进。

## 待办

> 2026-10-19 复核（FIRE1019R2）：19 条解锁条件逐条复核仍全部成立，无仓内可开工项；已裁决项不重开。本轮另修正本表 11 处 `checklist:` 行号漂移（清单整体后移 3 行，原指针落到已完成项或章节标题行）与 `docs/guide/acceptance-checklist.md:127` 内联指针。每条尾注〔阻塞｜解锁〕。

### 真机验收（等硬件/真实环境/凭据）

- [ ] service-health 生产接入：docker VM agent 已装但停用，改 SERVER_URL 指生产后 enable〔阻塞=宿主 docker VM 的配置动作，非仓内改动｜解锁=用户在生产 VM 改 SERVER_URL 并 enable〕
- [ ] Windows SCM 服务管理：列表/启停/自启/无权限透传〔阻塞=连接层四动词需提权 admin 会话（校验层 4 项已两台真机双证）｜解锁=提权会话按 checklist:249 复验〕
  - 2026-10-19 Windows 主机到位部分实测（探针 `scripts/acceptance/services/scm-probe/main.go`，commit f4a8ae50；`go build ./...` 绿）：**校验层 4 项真机 PASS**（validate 在 mgr.Connect 之前 service_windows_scm.go:104，无需提权）——①reload 被动词白名单拒绝（`unsupported action "reload" (allowed: start stop restart enable disable)`，service_windows_model.go:40，即 checklist:249「reload 按钮确认隐藏」后端半；前端半 index.tsx:88 isWindows + :265 仅 isSystemd 渲染重载，`index.test.tsx:302`「Windows 主机」测试过）②非法 unit 名 `../etc/passwd` 被拒（invalid windows service name，路径穿越面）③未知动词 nuke 被拒 ④**无权限报错透传**：SCM connect 返回 `scm connect: Access is denied.` 原文经 %w 透传到 provider 未被吞（scm.go:63）。**仍未验（需提权）**：list/启停/自启切换/restart 30s 超时路径——`mgr.Connect()` 申请 SC_MANAGER_ALL_ACCESS（scm.go:61），本会话非提权（UAC EnableLUA=1，无密码不可 runas）直接 Access is denied，故 checklist:249 连接层四动词实测**待提权 admin agent 会话复验**，不勾此行、不改验收标准。
  - 2026-10-19 补编译门禁（本 commit）：该探针带 `//go:build windows`，linux 上 `go build ./...` 恒跳过（上条「绿」不含此包），且此前所有 CI 腿只编 cmd 两包——探针零编译检查，坏了只会在真机验收现场炸；已实测 `GOOS=windows go build ./...` 全模块绿，并在 Build workflow windows 腿加整模块编译门禁。
  - 2026-10-20 win-worker 真机复证（hermes peer run，run_1420605fa72a4b92904a96a91a30ff04）：第二台独立 Windows 主机（Windows 11 26300.9711，go1.27.2 windows/amd64；blobless partial clone + sparse checkout 取探针及 internal/core 依赖，checkout 5a9512d）——**校验层 4 项再全过**（T1 reload 白名单拒 / T2 `../etc/passwd` invalid windows service name / T3 未知动词拒 / T4 `scm connect: Access is denied.` 经 %w 透传未吞），`=== RESULT 4 PASS / 0 FAIL / 1 BLOCKED ===` 退出码 0；T5 连接层仍 BLOCK——peer 会话同为非提权（Medium Mandatory Level S-1-16-8192，Administrators 仅拒绝访问，`net session` 失败），按红线未试 runas/UAC、未创建/未碰任何系统服务。连接层四动词缺口不变：等提权 admin agent 会话。
- [ ] macOS launchd 服务管理：列表/启停/自启/kickstart 回退〔阻塞=缺 macOS 主机｜解锁=真机到位按 checklist:251 验收〕
  - 2026-10-19 仓内预备半边（本 commit）：新增免提权验收探针 `scripts/acceptance/services/launchd-probe/main.go`（`//go:build darwin`，镜像 scm-probe 双层口径）。校验层 T1-T4 无需提权即可实测（validate 在 launchctl 前 service_launchd_darwin.go:125）——T1 非法 label `../etc/passwd` 被 label 正则拒（不含 `/`，service_launchd_model.go:35，路径穿越面）、T2 未知动词被白名单拒、T3 reload 被白名单拒（launchd 无 reload，D10.3；前端半 Services/index.tsx:265 仅 isSystemd 渲染重载按钮）、T4 合法动词打不存在 label 时 `launchctl kickstart` 错误原文带上下文透传不被吞（kickstart 失败且无 plist 时保留原错误 service_launchd_darwin.go:149）；`GOOS=darwin go build ./...` 实测绿。**仍未验（需真机）**：launchctl 执行层——list 观测（需 root 读 system 域）、启停/自启切换与 kickstart→bootstrap 回退（需 root + 临时 LaunchDaemon，不碰系统服务），按 checklist:251 待 macOS 真机复验，不勾此行、不改验收标准。darwin-only 文件此前零编译检查，已同步加 Build darwin 腿整模块编译门禁（与 windows 腿同缺口）。
- [ ] NAS：DSM 6/7、TrueNAS CORE/SCALE、OMV 真机 API；mdadm 降级演练；ZFS/SMB/NFS 主机〔阻塞=缺 NAS 设备与账号｜解锁=真机到位按 checklist:223-227 验收〕
- [ ] 备份云远端：真实 S3/B2、限速、GB 级完整链（server 侧 rclone 需宿主安装）〔阻塞=缺真实 S3/B2 凭据（checklist:51,52 明标不可本机验证）｜解锁=凭据与宿主 rclone 到位〕
- [ ] 录制异地：rclone 真远端推送〔阻塞=缺 rclone 真远端（checklist:61,62,68,69 标阻塞）｜解锁=远端配置到位〕
- [ ] ACME：staging/production 签发、自动续期、签发→部署→浏览器信任链〔阻塞=缺真实 ACME 账号与 DNS 凭据｜解锁=凭据到位按 checklist:111-117 验收〕
- [ ] DDNS：真 Cloudflare token、家庭侧 agent 出口 IP 多源〔阻塞=缺真实 Cloudflare token｜解锁=token 到位按 checklist:123-126 验收〕
- [ ] overlay：真 ZT/TS token 授权/除名、join/leave、systemctl 单元态、第二台主机合并观测〔阻塞=缺真实 ZT/TS token 与第二台主机｜解锁=token+主机到位按 checklist:166-168 验收〕
- [ ] SMART：带 attr 表主机读数、FAILED 端到端翻转〔阻塞=缺带 SMART 属性表的真实磁盘｜解锁=真机到位按 checklist:180,184 验收〕
- [ ] DNS 真账号：DNSPod/阿里云 SRV 与 TTL 下限、台账联动全链、M4 跨 provider 迁移演练〔阻塞=缺真实 DNSPod/阿里云账号｜解锁=账号到位按 checklist:133-135 验收〕
- [ ] Guacamole 音频真机：RDP 音频 mimetype 与 L8/L16 编解码确认（D4，静音按钮已实现）〔阻塞=缺 Windows RDP 主机｜解锁=真机确认编解码〕
- [ ] 移动端：Android release 安装/splash/R8/签名升级、生物识别、ntfy 送达、SSH 终端〔阻塞=缺 Android 真机与签名/ntfy 凭据｜解锁=真机+凭据到位按 checklist 移动端一节（:355 起）验收〕
- [ ] web 移动端触屏与 Workbench 触屏交互〔阻塞=缺真机触屏设备；地基已随 web 移动适配交付（1076 用例），仓内无代码缺口｜解锁=触屏真机验收〕
- [ ] iOS 签名与装机（低优先级；编译验证已在 CI）〔阻塞=缺 Apple 开发者账号与 Mac｜解锁=凭据到位装机验收〕

### 功能

- [ ] 防火墙写操作（规则增删）：未立项，等需求〔已裁决搁置：decision-log D-2026-10-07-3 否决直接做写，前置=只读真机验收闭环（未完成）｜解锁=只读真机验收后重开写操作评估〕
- [ ] workflow 并行分支执行：等真实需求再立项〔已裁决搁置：workflow-design W4/G3 压后｜解锁=真实并行需求出现再立项〕
- [ ] vault 硬验收：真 persona 服务端实测 provider 切换（单测已证切换不破坏 local 存量，cc51840）〔外部阻塞=缺 persona 服务端地址与 token｜解锁=用户提供地址+凭据〕
- [ ] jenkins/external endpoint 凭据 vault 引用：随链接功能立项〔待立项=无独立开工条件｜解锁=链接功能立项〕

### 多云扩展（2026-10-20 评审导入，未立项待拍板）

> 外部评审导入：Cockpit 定位从「Agent 驱动的基础设施管理平台」扩展为「Cloud API 驱动的多云资源
> 控制与成本管理平台」，两能力域（Personal / Cloud Infrastructure Operations）共用 Inventory，
> 不拆两套产品。**全文与逐条代码核验见 `docs/review/chatgpt-2026-10-20-multicloud-positioning.md`**
> （CloudAccount/Billing 现为 0 个 go 文件 = 全新缺口；ComputeInstance 无云原生 ID/计费/配额模型
> 已对照字段证实）。落地跟踪清单挂在该评审文档尾部，立项拍板后按 P0→P1→P2 从那边逐项勾。
> 边界注意：Reconciler 用普通 Go 小控制器，不触碰「不做」清单的 K8s CRD 全量模型。

- [ ] P0 云账号接入 / 资源自动发现 / Provider Framework / 计算生命周期闭环 / 成本地基〔2026-10-20 用户拍板开工（首批厂商已代决：阿里云+腾讯云）；**Provider Framework 片1已落**：设计 docs/design/cloud-provider.md(+en)、`internal/cloud`（拆分接口/能力声明+注册表 fail-closed 校验/Operation 状态机 pending→running→succeeded|failed/资源成本类型），go build ./... 绿 + 包覆盖率 100%；剩余=片2 表与存储、片3/4 厂商适配器〔阻塞=真实 AccessKey 由用户提供经 vault｜解锁=凭据到位〕〕
- [ ] P1 资源池与容量 / Go Reconciler / 云网络 / 预算策略引擎 / 双重控制边界裁定〔待拍板=P0 闭环后；其中「谁对同一资源有写入权（OpenTofu vs Reconciler）」需先裁定〕
- [ ] P2 智能调度 / 成本优化 / 控制面高可用〔待拍板=按真实需求〕

### 文档

- [x] archive 5 篇是否双语：历史归档（plans 2 / specs 2 / 归档说明 1）当前仅中文〔已定 2026-10-16（R1025PATROL 代拍板，用户事后可推翻）：归档是历史记录，英文版无读者价值，只保留中文不补英文版；决策记录 docs/guide/decision-log.md D-2026-10-16-1〕
- [x] docs 双语配对第二批：子目录 guide 37、design 5、operations 1、research 1 篇 + 首页英文版精修（d2bc3cd；guide 37/37、design 5/5、operations 1/1、research 1/1 全配对；英文页跨页链接全量对账补齐残留 36 处 / 12 文件——此前声称「43 处、残留中文清零」只覆盖部分页面，含无扩展名相对链接在 VitePress 下解析回中文页；现存 zh 指向仅 2 处预期项（index.en 语言切换、architecture.en→archive 无英文孪生）、死链 0、vitepress build 113 页 exit 0；archive 部分拆出为上方待办条目）

## 已完成

### 架构与 agent

- [x] Phase 0-7 收口：CLI 三命令、inventory 六类资源同步、server/agent 拆包、协议类型化 `DecodePayload[T]`、远控权限审计、前端拆页、CI 收口
- [x] agent-core 三层架构 P1-P7：core 八包 + platform 五 GOOS 契约 + 插件宿主收编（2667a12 起 10+ commit，agent 树覆盖 100%）
- [x] grdp 旧桌面路线删除，桌面统一走 Guacamole（D-2026-10-07-1，40 包绿）
- [x] 服务检测独立 agent：简档/Guard 状态机/调度器/薄壳/上行落库/web 面板（20b406f 至 569e5f8，20+ 用例）
- [x] execlog 执行留痕：协议面 + 审计归集 + 脱敏环 + rpc 单点埋点（ed35cb6/126b84e）
- [x] healthprobe 三语义 + 故障窗口判定上收（71bfb7c/73fd220）
- [x] 巡检循环启动等待改 ctx 可中断，六条同构循环收口（bd3fef4 五条基线 + d152c85 补 metricsCleanupLoop 第六条，其裸 Sleep 最长近 24h 会拖住 Shutdown 到次日凌晨 3 点）

### 远控

- [x] 三协议统一 Guacamole：SSH 接入、web 分流、尺寸同步、剪贴板双向（探针 16 场景）
- [x] guacd 经 agent 中继转发、不直拨目标（34d606c/0767b8b，16/16 两轮）
- [x] 真机验收 6 项全勾（2026-09-30，证据 `.acceptance/guac/`）
- [x] 会话台账：terminal/guacamole 双链路登记、DELETE 终止收口
- [x] egress 多策略并集判定（9a563b9）
- [x] WS 空 Origin 放行修复、SSH 默认密钥劫持修复（显式凭据优先五分支单测）
- [x] 外部新建远程会话契约（memex）：docs/design/external-session-api.md（3ee30fe）

### 密码箱

- [x] vault 抽象 + local provider + 接入点第一波（93473a0/47367b4/8fd31e8，54 包绿）
- [x] vault 第二波：配置 `vault:` 引用十字段 + ACME 私钥与 capability config 落库加密（6bf4f35/2904035）
- [x] persona provider：只读映射、fail-closed、严格配置解码、写删 API 映射 501（cc51840，29 测试 + 8 子测试）

### 资源与运维面

- [x] Docker 容器生命周期与日志查看（P0）
- [x] 健康探测 runner（P0）
- [x] 拨测增强：间隔可配、四渠道通知、心跳条、阈值配置、告警去重、`probe.interval` yaml 字段
- [x] Compose Stack：M1/M1.5 + 上传 + 真机 R1-R17 + P2 三条（Git-to-deploy、webhook 触发、inventory 端口联动，本地验收 68/68）
- [x] 备份：agent 备份/恢复/分块下载 + server SQLite 热备 + 异地保留 + 前置钩子（真机 K0-K11 44/44、B0-B3、R0-R3）
- [x] 反向代理：nginx + Traefik 双后端（真机 N0-N8、T1-T10）
- [x] 文件管理器 M1-M5：浏览/搜索/图片预览/分块续传/chmod-chown/跨刷新续传
- [x] 日志 M1-M3：查询、实时尾随、跨机联邦检索（真机 6/6）
- [x] 定时任务 M1-M4：cron 管理、下次触发、systemd timer、多用户
- [x] 服务管理三后端 systemd/Windows SCM/macOS launchd + mask/journal/daemon-reload/unit 文件（真机 17/17）
- [x] 服务健康探针自愈 M1/M2（真机场景 A/B）
- [x] drift M1-M6：基线/巡检/diff/以当前为准/双栏对照/CMDB 一致性
- [x] 录制 M1-M3：cast/guac 收集回放 + 异地归档
- [x] DNS M1-M4：Cloudflare/DNSPod/阿里云、台账写联动、批量导入导出
- [x] overlay M1-M3：运行态观测、云管理面、join/leave + frp/WireGuard 真机（frps serverinfo、wg dump 五列布局两修复）
- [x] SMART 磁盘健康 + 真机 S0-S4
- [x] DDNS M1 + 巡检语义真机 D1-D5
- [x] ACME：lego DNS-01 三 provider + 签发后多目标部署 + 证书联动
- [x] 防火墙 M1 只读观测（探针 F/H/W 全 PASS）
- [x] NAS M1/M2：本地 + DSM/TrueNAS/OMV
- [x] workflow：M1 + 变量传递、扇出、取消信号、审批门、DAG 依赖（真机 13/13）

### web 与移动端

- [x] web 前端存量收口：region/zone 地域字段对齐、tsc 清零（8f928baf/72adcbf8）
- [x] Flutter M1-M9 全线：认证/仪表盘/容器/告警/资源/备份/cron/文件/终端/生物锁/反代/Stacks/SMART/NAS/overlay 三页（158 测试）
- [x] Android release：签名/R8/splash/权限最小化
- [x] web 移动适配：全站表格兜底 + MobileTabBar 底部导航（1076 用例）
- [x] README 瘦身 236→55 行 + 文档站收口
- [x] 运维开源调研 docs/research/ops-oss-survey.md（0422d3f）

### 工程与门禁

- [x] 覆盖率三端收口：Go 台账制 100%、web lcov 有效 100%、mobile 99.83% 登记（`tool/coverage_check.sh` 三端，含产物新鲜度守卫）
- [x] race 收口：auth/proxy/agent 三处数据竞态 + TempDir 清理竞态根治（审计写序重排 5578f79）
- [x] TempDir 清理竞态第二轮：Shutdown 先等后台清扫 goroutine 退出再关 DB（d152c85，bgWG 计数 + Wait 前置 db.Close，approvals/dns-cmdb 清扫打到 database is closed 与 ENOTEMPTY 连带根治，仓库级 -race -short 全绿）
- [x] CI flake 根治：testlog EFBIG（fsizeWindowLimit）、Settings 镜像 flake、覆盖门禁假绿修复
- [x] agent 过期自动判定与列表隐藏（dc6163b/a871306）
- [x] region/zone unknown 语义（D-2026-10-08-1）、proxyType 拒收 udp（D-2026-10-08-2）
- [x] 文档一致性审计四轮（docs/审计-文档一致性.md；第四轮全量六束并行审计，90 条差异全修，P0=0/P1=18/P2=72）
- [x] e2e 冒烟 + `scripts/acceptance/` 十二功能域
- [x] install.sh/install.ps1 一键安装 + 服务注册 + nightly 矩阵
- [x] Docker 镜像发布链路：GHCR workflow + 部署 compose + deploy-docker.md
- [x] JWT 惰性初始化（import 零副作用）

## 不做（边界决策，重开需再拍板）

- Kubernetes 风格 CRD 全量模型（除非先定 v2 inventory 迁移方案）
- 替换前端 UI 框架
- SQLite 换 PostgreSQL
- 扩展更多远控协议（现 RDP/VNC/SSH + telnet 已收稳）
- workflow SSE
- P8 configsync（server 配置源面不立项）
- 移动端 VNC/桌面观看（D7；触发条件见 docs/guide/mobile-design.md）
- 大规模目录重命名
- [x] 审核整改（2026-10-18 独立审核，见 docs/review/luna-review-2026-10-18.md）：
      按审核报告的三张表逐项处理，**先核对再动手**，每项给 file:line 证据。
      回执：docs/review/luna-review-2026-10-18-response.md
      完成证据：表一#1-5、表二#1-4、表三评估（只评估不实现）全部落地（子项见下）；
      门禁全绿：全量 `go test ./... -count=1`（非 short，exit 0）+ Go 覆盖率门禁
      （有效覆盖 100%，新增 websocket.go:31-33 / guac_relay.go:73-74 覆盖，登记表漂移同步），
      `go test -race -short ./... -timeout 1500s`，web 覆盖率门禁（TZ=UTC，exit 0），
      `scripts/e2e-smoke.sh`（enrollment 准入链路，exit 0），docs VitePress build（exit 0）。
      全量 ESLint 仍报 5 个未改文件的既存规则错误（改动前端文件定向 lint 通过）。
      1) 表一「缺陷」：按风险排序修，高风险优先
      2) 表二「文档与实现不一致」：以代码为准改文档，或明确标注为规划中
         **禁止为对齐文档去改业务代码**
      3) 表三「值得补的能力」：只做评估与分期建议，**不实现**（需用户拍板）
      每完成一项勾掉并写证据；全部测试通过后 commit+push；不 bump 版本号。
      - [x] 表一#1 Agent 注册默认拒绝：`internal/server/websocket.go:144-191,188-264`；bcrypt 持久化失败回滚在线注册；测试 `internal/server/websocket_registration_policy_test.go:19-218` + e2e 通过。准入配置/引导：`config/cockpit.yaml:12-15`、`deployments/docker/config.yaml:5-8`、`deployments/docker/.env.example:9`、`deployments/install-server.sh:72-81`、`deployments/install-server.ps1:68-75`；Agent CLI/GUI 安装器示例：`docs/guide/getting-started.md:107-189`、`packaging/windows/agent.iss:69-97,134-141`。
      - [x] 表一#2 登录限流：`internal/server/login_limiter.go:1-149`、`internal/server/server.go:563-630`；IP/账号双桶、429/Retry-After；`internal/server/login_limiter_test.go:1-120`（定向测试通过）。
      - [x] 表一#3 ticket 日志：`internal/server/ticket.go:41-66`；`internal/server/ticket_test.go:58-77`（断言日志无明文 ticket，定向测试通过）。
      - [x] 表一#4 guacd 中继：`internal/server/guac_relay.go:14-135`、`internal/server/api_guacamole.go:362-399,511-530`；`internal/server/guac_integration_test.go:46-113,344-408`（Docker guacd→Server relay→fake Agent→fake VNC 通过）。
      - [x] 表一#5 密码重置枚举：`internal/server/password_reset_handlers.go:11-71`、`web/src/pages/ForgotPassword/index.tsx:19-46,128-164`；`internal/server/password_reset_handlers_test.go:84-119`（三态统一响应断言）、web 全量 1123 测试通过。
      - [x] 表二#1 ticket 通道文档：`docs/guide/architecture.md:121-127`、`docs/guide/protocol.md:167-175` 及英文配对；VitePress build 通过。
      - [x] 表二#2 Job 异步口径：`docs/guide/jobs-design.md:10-26,127-134` 及英文配对；VitePress build 通过。
      - [x] 表二#3 guacd 拓扑：`docs/guacd-deploy.md:49-75,127-142`、`docs/guacd-deploy.en.md:49-78,128-145`、`deployments/docker/docker-compose.yml:32-42`；双容器回拨验收通过。
      - [x] 表二#4 JWT 例外：`docs/guide/architecture.md:100-117` 及英文配对；说明 webhook 的 `X-Cockpit-Token` 例外，VitePress build 通过。
      - [x] 表三 5 项能力仅评估与 P0/P1/P2 建议：`docs/review/luna-review-2026-10-18-response.md:67-75`；无能力实现。
- [x] 文档时效性清理（2026-10-18 用户令：很多文档已过时）：
      在本轮审核整改完成之后执行，避免与整改冲突。
      完成证据（2026-10-19，30 个文件 / 15 组中英成对）：
      - 数值/常量对齐代码：nginx reload 超时 5s→10s（`nginxTestTimeout`）、备份跟踪总时限
        10 分钟→30 分钟（`backupTrackTimeout`）、web 端点数 173→196、launchd 行数
        207→183、openwrt 工件名补 `cockpit-agent-` 前缀（对照 nightly.yml）。
      - API 面补齐/纠错：通知状态真实路径 `/api/notification/status`（文档误写
        `/api/probe/notification/status`）、备份表补 `GET /api/backups/runs`、
        `file.write` 补可选 `mode`、`/api/probe/windows` 补可选 `agent_id`、
        RPC 前缀注明实施为 `nginx.*`/`traefik.*`。
      - 锚点/引用修正：`config/cockpit.yaml` 行号 85→94、`services/api.ts` 行号 124→138、
        失效章节引用 E8→E7、persona 上游 `EXTERNAL_INTEGRATION.md` 标明不在本仓。
      - 已废弃/已落地不留假象：guacamole 设计文档补「实现修正」注记（中继绑定路由本地 IP、
        `connect` 走中继地址、`GUACD_ADDR`/`GUAC_RELAY_BIND_ADDR`/`GUAC_RELAY_HOST`）、
        镜像名 `apache/guacamole`→`guacamole/guacd`、VNCModal/DesktopModal 标注已按
        D-2026-10-07-1 删除、reference-projects 已落地借鉴项划掉并指向落地位置、
        rbac-design 删重标题、acceptance-checklist W10/W10b/W10c 从「验收后回填约定」
        误挂位移回 Workflow 小节。
      - 命令示例实跑：`go build ./cmd/cockpit ./cmd/cockpit-agent` + `cockpit-agent version`、
        `cockpit-agent start -h`、`cockpit version` 全通过（安装类脚本会装系统服务，未代跑，
        其参数与配置路径已逐项对照脚本源码核对）。
      - 门禁：`npx vitepress build` exit 0（113 页）；未改任何业务代码。
      - 剩余已知：`docs/guide/审计-文档一致性.md`（2026-10-08 审计快照）保留为历史记录，
        其「35 篇」统计已不成立，仅作历史参考。
