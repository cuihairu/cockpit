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

- [ ] service-health 生产接入：docker VM agent 已装但停用，改 SERVER_URL 指生产后 enable〔阻塞=生产 server 迁至 dev 仅绑 127.0.0.1，VM 原 SERVER_URL 指向已停用入口；配置新出口需用户提供/授权暴露 SERVER_URL｜解锁=可用生产 WS 地址或批准 LAN 暴露后改 SERVER_URL 并 enable〕〔BLOCKED：缺生产 WS 地址或 LAN 暴露授权｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
  - 2026-10-22 复核：VM 192.168.5.5 直连确认 `cockpit-agent.service` 处于 disabled/inactive，`/etc/default/cockpit-agent` 的 SERVER_URL 仍指向本机已停用的 :9000；可达的生产 server 位于 dev（cockpit.cuihairu.site → 106.14.216.177），但生产 `cockpit.service` 仅绑 `127.0.0.1:9000`。未获新 WS 地址/暴露授权前不改生产入口。
- [ ] Windows SCM 服务管理：列表/启停/自启/无权限透传〔阻塞=连接层四动词需提权 admin 会话（校验层 4 项已两台真机双证）｜解锁=提权会话按 checklist:249 复验〕〔BLOCKED：缺提权 admin agent 会话｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
  - 2026-10-19 Windows 主机到位部分实测（探针 `scripts/acceptance/services/scm-probe/main.go`，commit f4a8ae50；`go build ./...` 绿）：**校验层 4 项真机 PASS**（validate 在 mgr.Connect 之前 service_windows_scm.go:104，无需提权）——①reload 被动词白名单拒绝（`unsupported action "reload" (allowed: start stop restart enable disable)`，service_windows_model.go:40，即 checklist:249「reload 按钮确认隐藏」后端半；前端半 index.tsx:88 isWindows + :265 仅 isSystemd 渲染重载，`index.test.tsx:302`「Windows 主机」测试过）②非法 unit 名 `../etc/passwd` 被拒（invalid windows service name，路径穿越面）③未知动词 nuke 被拒 ④**无权限报错透传**：SCM connect 返回 `scm connect: Access is denied.` 原文经 %w 透传到 provider 未被吞（scm.go:63）。**仍未验（需提权）**：list/启停/自启切换/restart 30s 超时路径——`mgr.Connect()` 申请 SC_MANAGER_ALL_ACCESS（scm.go:61），本会话非提权（UAC EnableLUA=1，无密码不可 runas）直接 Access is denied，故 checklist:249 连接层四动词实测**待提权 admin agent 会话复验**，不勾此行、不改验收标准。
  - 2026-10-19 补编译门禁（本 commit）：该探针带 `//go:build windows`，linux 上 `go build ./...` 恒跳过（上条「绿」不含此包），且此前所有 CI 腿只编 cmd 两包——探针零编译检查，坏了只会在真机验收现场炸；已实测 `GOOS=windows go build ./...` 全模块绿，并在 Build workflow windows 腿加整模块编译门禁。
  - 2026-10-20 win-worker 真机复证（hermes peer run，run_1420605fa72a4b92904a96a91a30ff04）：第二台独立 Windows 主机（Windows 11 26300.9711，go1.27.2 windows/amd64；blobless partial clone + sparse checkout 取探针及 internal/core 依赖，checkout 5a9512d）——**校验层 4 项再全过**（T1 reload 白名单拒 / T2 `../etc/passwd` invalid windows service name / T3 未知动词拒 / T4 `scm connect: Access is denied.` 经 %w 透传未吞），`=== RESULT 4 PASS / 0 FAIL / 1 BLOCKED ===` 退出码 0；T5 连接层仍 BLOCK——peer 会话同为非提权（Medium Mandatory Level S-1-16-8192，Administrators 仅拒绝访问，`net session` 失败），按红线未试 runas/UAC、未创建/未碰任何系统服务。连接层四动词缺口不变：等提权 admin agent 会话。
- [ ] macOS launchd 服务管理：列表/启停/自启/kickstart 回退〔阻塞=缺 macOS 主机｜解锁=真机到位按 checklist:251 验收〕〔BLOCKED：缺 macOS 主机｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
  - 2026-10-19 仓内预备半边（本 commit）：新增免提权验收探针 `scripts/acceptance/services/launchd-probe/main.go`（`//go:build darwin`，镜像 scm-probe 双层口径）。校验层 T1-T4 无需提权即可实测（validate 在 launchctl 前 service_launchd_darwin.go:125）——T1 非法 label `../etc/passwd` 被 label 正则拒（不含 `/`，service_launchd_model.go:35，路径穿越面）、T2 未知动词被白名单拒、T3 reload 被白名单拒（launchd 无 reload，D10.3；前端半 Services/index.tsx:265 仅 isSystemd 渲染重载按钮）、T4 合法动词打不存在 label 时 `launchctl kickstart` 错误原文带上下文透传不被吞（kickstart 失败且无 plist 时保留原错误 service_launchd_darwin.go:149）；`GOOS=darwin go build ./...` 实测绿。**仍未验（需真机）**：launchctl 执行层——list 观测（需 root 读 system 域）、启停/自启切换与 kickstart→bootstrap 回退（需 root + 临时 LaunchDaemon，不碰系统服务），按 checklist:251 待 macOS 真机复验，不勾此行、不改验收标准。darwin-only 文件此前零编译检查，已同步加 Build darwin 腿整模块编译门禁（与 windows 腿同缺口）。
- [ ] NAS：DSM 6/7、TrueNAS CORE/SCALE、OMV 真机 API；mdadm 降级演练；ZFS/SMB/NFS 主机〔阻塞=缺 NAS 设备与账号｜解锁=真机到位按 checklist:223-227 验收〕〔BLOCKED：缺 NAS 设备与账号｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] 备份云远端：真实 S3/B2、限速、GB 级完整链（server 侧 rclone 需宿主安装）〔阻塞=缺真实 S3/B2 凭据（checklist:51,52 明标不可本机验证）｜解锁=凭据与宿主 rclone 到位〕〔BLOCKED：缺真实 S3/B2 凭据与宿主 rclone｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] 录制异地：rclone 真远端推送〔阻塞=缺 rclone 真远端（checklist:61,62,68,69 标阻塞）｜解锁=远端配置到位〕〔BLOCKED：缺 rclone 真远端｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] ACME：staging/production 签发、自动续期、签发→部署→浏览器信任链〔阻塞=缺真实 ACME 账号与 DNS 凭据｜解锁=凭据到位按 checklist:111-117 验收〕〔BLOCKED：缺真实 ACME 账号与 DNS 凭据｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] DDNS：真 Cloudflare token、家庭侧 agent 出口 IP 多源〔阻塞=缺真实 Cloudflare token｜解锁=token 到位按 checklist:123-126 验收〕〔BLOCKED：缺真实 Cloudflare token｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] overlay：真 ZT/TS token 授权/除名、join/leave、systemctl 单元态、第二台主机合并观测〔阻塞=缺真实 ZT/TS token 与第二台主机｜解锁=token+主机到位按 checklist:166-168 验收〕〔BLOCKED：缺真实 ZT/TS token 与第二台主机｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] SMART：带 attr 表主机读数、FAILED 端到端翻转〔阻塞=缺带 SMART 属性表的真实磁盘｜解锁=真机到位按 checklist:180,184 验收〕〔BLOCKED：缺带 SMART 属性表的真实磁盘｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] DNS 真账号：DNSPod/阿里云 SRV 与 TTL 下限、台账联动全链、M4 跨 provider 迁移演练〔阻塞=缺真实 DNSPod/阿里云账号｜解锁=账号到位按 checklist:133-135 验收〕〔BLOCKED：缺真实 DNSPod/阿里云账号｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [x] Guacamole 音频真机：RDP 音频 mimetype 与 L8/L16 编解码确认（D4，静音按钮已实现）〔2026-11-01 openclaw 真机验收完成：协商 mimetype=audio/L16（16-bit PCM 44100Hz 2ch），L8 仅浏览器 RawAudioPlayer 支持/guacd 1.5.5 对 16-bit 源不产 L8；5,013,504 字节 PCM 经网关到达；发现并修复网关空 audio 声明（api_guacamole.go:533），单测同步；静音按钮 suspend/resume 由单测覆盖；详见 docs/review/2026-11-01-guacamole-audio-acceptance.md〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] 移动端：Android release 安装/splash/R8/签名升级、生物识别、ntfy 送达、SSH 终端〔阻塞=缺 Android 真机与签名/ntfy 凭据｜解锁=真机+凭据到位按 checklist 移动端一节（:355 起）验收〕〔BLOCKED：缺 Android 真机与签名/ntfy 凭据｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] web 移动端触屏与 Workbench 触屏交互〔阻塞=缺真机触屏设备；地基已随 web 移动适配交付（1076 用例），仓内无代码缺口｜解锁=触屏真机验收〕〔BLOCKED：缺真机触屏设备｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] iOS 签名与装机（低优先级；编译验证已在 CI）〔阻塞=缺 Apple 开发者账号与 Mac｜解锁=凭据到位装机验收〕〔BLOCKED：缺 Apple 开发者账号与 Mac｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)

### 功能

- [ ] 防火墙写操作（规则增删）：未立项，等需求〔已裁决搁置：decision-log D-2026-10-07-3 否决直接做写，前置=只读真机验收闭环（未完成）｜解锁=只读真机验收后重开写操作评估〕〔BLOCKED：缺用户裁决（只读真机验收闭环前置未完成）｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] workflow 并行分支执行：等真实需求再立项〔已裁决搁置：workflow-design W4/G3 压后｜解锁=真实并行需求出现再立项〕〔BLOCKED：缺真实并行需求出现｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] vault 硬验收：真 persona 服务端实测 provider 切换（单测已证切换不破坏 local 存量，cc51840）〔外部阻塞=缺 persona 服务端地址与 token｜解锁=用户提供地址+凭据〕〔BLOCKED：缺 persona 服务端地址与 token｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
- [ ] jenkins/external endpoint 凭据 vault 引用：随链接功能立项〔待立项=无独立开工条件｜解锁=链接功能立项〕〔BLOCKED：缺链接功能立项｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)

### 多云扩展（2026-10-20 评审导入，未立项待拍板）

> 外部评审导入：Cockpit 定位从「Agent 驱动的基础设施管理平台」扩展为「Cloud API 驱动的多云资源
> 控制与成本管理平台」，两能力域（Personal / Cloud Infrastructure Operations）共用 Inventory，
> 不拆两套产品。**全文与逐条代码核验见 `docs/review/chatgpt-2026-10-20-multicloud-positioning.md`**
> （CloudAccount/Billing 原为 0 个 go 文件 = 全新缺口；ComputeInstance 无云原生 ID/计费/配额模型
> 已对照字段证实）。落地跟踪清单挂在该评审文档尾部，立项拍板后按 P0→P1→P2 从那边逐项勾。
> 边界注意：Reconciler 用普通 Go 小控制器，不触碰「不做」清单的 K8s CRD 全量模型。

- [ ] P0 云账号接入 / 资源自动发现 / Provider Framework / 计算生命周期闭环 / 成本地基〔2026-10-20 用户拍板开工（首批厂商已代决：阿里云+腾讯云）；**片1已落 cfeee64**（设计 docs/design/cloud-provider.md(+en)、`internal/cloud`：拆分接口/能力声明+注册表 fail-closed 校验/Operation 状态机 pending→running→succeeded|failed/资源成本类型，go build ./... 绿 + 包覆盖率 100%）；**片2已落**（`internal/storage/cloud.go`：cloud_accounts/cloud_instances 表 AutoMigrate、账号/实例 upsert/list/delete、`${VAR}` 凭据引用形态校验+运行时解析 fail-closed，storage 包测试绿）；**片3 Compute 部分已落 8bf385c**（签名器抽中立包 `internal/alisign`（原 dns/sign.go，dns 同步改用，e862a4f）+ `internal/cloud/aliyun` ECS 适配器：DescribeInstances 分页列表+状态/计费归一（过渡态归 unknown 原文保留）、RunInstances 创建（InstanceSpec 增 SecurityGroupID）、启停删、GetOperation 以实例状态推断完成态，fake server 契约测试（桩侧复算签名）绿 + 包覆盖 100% + 全量门禁绿）；**片4 适配器已落 df558ff**（TC3-HMAC-SHA256 签名抽中立包 `internal/tcsign`（纯函数，结构/敏感性/确定性测试，100% 覆盖）+ `internal/cloud/tencent` CVM 适配器：DescribeInstances 分页列表+状态/计费归一（过渡态归 unknown 原文保留）、RunInstances 创建、启停删、GetOperation 以实例状态推断完成态，fake server 契约测试（桩侧复算 TC3 签名）绿 + 包覆盖 100%）；剩余=片3 Quota 子片〔统一配额服务 API 契约未经真实账号核对，不硬造，随片4 验收解锁〕与双厂商闭环验收〔阻塞=真实 AccessKey 由用户提供经 vault｜解锁=凭据到位；2026-10-27 修订口径：适配器契约先行，真 AccessKey 到位前不做闭环验收〕〕〔BLOCKED：缺真实云 AccessKey（Quota 子片与双厂商闭环验收）｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)〔立即动作已落：docs/design/cloud-lifecycle-wiring.md〕
  - 进度：**账号接线/web 双确认片已落**（`/api/cloud/accounts` 只回 `${VAR}` 引用，secret 经既有 vault `cloud/{vendor}/{alias}` 入库；`cloud:read/write` RBAC、审计、web 云账号页保存/删除逐字双确认；契约/页面测试绿），全量门禁受环境干扰未过；腾讯适配器已完成（上条片4 已落 df558ff）；后续=发现/生命周期接线。
- [ ] P1 资源池与容量 / Go Reconciler / 云网络 / 预算策略引擎 / 双重控制边界裁定〔待拍板=P0 闭环后；其中「谁对同一资源有写入权（OpenTofu vs Reconciler）」需先裁定〕〔BLOCKED：缺用户裁决（P0 闭环后）｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)〔立即动作已落：docs/design/p1-resource-ownership.md〕
- [ ] P2 智能调度 / 成本优化 / 控制面高可用〔待拍板=按真实需求〕〔BLOCKED：缺真实需求｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)

### 竞品功能强化（2026-10-20 评审导入，未立项待拍板）

> 8 竞品（Tactical RMM/Zabbix Agent 2/Teleport/Beszel/Portainer/Tailscale/RustDesk/MeshCentral）
> 全景分析导入：核心结论「补 Agent 可靠性/安全边界/跨平台一致性/资源管理体系，不横向堆功能」。
> 全文+逐项核验表+落地跟踪见 `docs/review/competitor-landscape-2026-10-20.md`。
> 核验要点：12 项建议里 4 项已覆盖大半（隧道/服务三后端/定时任务/审计留痕）、
> 6 项部分强化、3 项全新缺口（Capability 协议/离线缓冲/轻量模式）；
> **Provider Adapter 独立层归并入上方多云工作流，不单开线**。

- [ ] P0 Agent 侧四件：Capability 协议 / 调度资源隔离（限流·优先级·超时）/ 升级回滚 / 任务可靠性（重试·窗口·幂等）〔待拍板=评审核心建议：先把 Agent Core 做小、契约做稳；四件可拆四单按序排｜解锁=拍板即排单〕〔BLOCKED：缺用户拍板｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)〔立即动作已落：docs/design/agent-capability-contract.md〕
- [ ] P0 安全两件：短期凭据+资源级权限（Teleport 式，依托既有 vault）/ 轻量运行模式（最小构建+资源预算）〔待拍板〕〔BLOCKED：缺用户拍板｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)〔立即动作已落：docs/design/resource-permission-matrix.md〕
- [ ] P1 离线数据缓冲 / 远程协作（会话共享）/ 统一电源控制（WoL+PVE；云电源随多云片）/ 四套源码深读笔记（Zabbix Agent 2·Teleport·Portainer Edge·Tactical RMM）〔待拍板=P0 后〕〔BLOCKED：缺用户拍板（P0 后）｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)〔立即动作已落：docs/design/offline-buffer-design.md〕

### 带外管理 OOB（2026-10-20 评审+用户裁决导入，未立项待拍板）

> 三条路线统一 Provider：Intel AMT / AMD DASH(AIM-T) / Redfish(服务器含 ARM)，能力三态
> supported/enabled/reachable；**用户裁决第四线：局域网 OpenWrt 自举节点=网关+探活+
> 最后兜底控制（WOL/转发），必须自启动可靠**（衔接既有 internal/openwrt ubus 客户端）。
> 用户另要求：**支持机型矩阵逐型号实测入表，未实测不入表**。
> 全文/矩阵/落地跟踪见 `docs/review/oob-management-2026-10-20.md`；Redfish 线以既有
> `docs/design/bmc-capabilities.md`(+en) 为基座合并，不另起炉灶。

- [ ] P0 OOB Provider 框架（三态能力模型+发现协商）/ Redfish / AMT / DASH（优先 DASH CLI+API 集成）/ OpenWrt 自举节点（探活+WOL+兜底转发+自启动）〔2026-10-20 用户拍板开工；**框架片已落**：`internal/oob`（Capability 六项/State 三态+Usable/rank 仲裁/Registry+并发 ProbeAll/Merge 合并来源可溯），go build 绿 + 包覆盖率 100%；**Redfish 探测片已落**：`internal/oob/redfish` 只读发现探测（service root 门禁+System/Manager/Bios 三态核实、缺证据不猜、401 如实报可达未定），httptest 契约 13 测试绿+包覆盖率 100%，凭据面与操作面随 bmc-capabilities BMC-B3/B4 片；**AMT 探测片已落**：`internal/oob/wsman`（WS-Management Identify 免认证共用层，DASH 同源）+ `internal/oob/amt`（无 scheme/port 默认 :16992、产品匹配拒错认、401 如实报可达未定），21 用例绿+两包覆盖率 100%，启用态与能力面随凭据片经 vault；**DASH 首片已落**：`internal/oob/dash` 只读发现探测（`dash.go:59` 经 wsman Identify，`dash.go:66` 产品匹配拒错认，`dash.go:61` 401/403 如实报可达未定，`dash.go:74` 无 scheme/port 默认 http:16992；Identify 只证网络栈在位+可达，不报能力、缺证据不猜；已接 `internal/server/api_oob_discovery.go:47` Registry/Merge，attempted 含 dash），httptest 契约 13 测试绿（dash 包 11 顶层+2 子用例）+ 包覆盖率 100%，启用态与能力面随凭据片经 vault；**OpenWrt 自举节点设计已落**（docs/design/oob-openwrt-bootstrap.md(+en)：探活三源独立三态判定/WOL 与兜底转发动作面双确认+审计/procd 自启动/凭据 vault 引用，分片 A-D 待排）；**OpenWrt 片D Go 门禁已落 32bf697**：procd init 脚本显式 respawn 看门狗参数（threshold=3600s/cooldown=5s/retry=5，设计 §5，锁死默认策略防漂移）+ 工件接线契约测试（scripts/openwrt/init_test.go：respawn 显式、UCI 字段 init↔conf 对齐、package-ipk.sh 接线、e2e 真实 ipk ar 结构+tar.gz 载荷，linux 门禁，race 绿；shellcheck SC2034 为 procd rc.common 误报）；剩余=OpenWrt 片D 真机自启动验收（断电/看门狗恢复，需真机）；**OpenWrt 片A WOL 动作面已落 f9210ab**：internal/oob/wol 魔术包（102B）/MAC 解析/网段广播计算/UDP 发送注入点（包覆盖 100%，测试零真发报文）+ agent oob provider `oob.wol`（显式 ip:port 优先，缺省按首个可广播非回环网段推导广播地址，网卡枚举/地址读取注入点）+ server POST /api/oob/wol 转发端点（oob capability 门禁、RBAC 归 inventory 写、审计 oob_wol 记目标 MAC 与实际发送目标），Go 门禁全绿（全量短测+块覆盖登记门+全量 -race）；**OpenWrt 片B 探活动作面已落 62b7e6d**：internal/oob/liveness 三源独立三态判定（present/unknown；absent 保留——探测不到 ≠ 不存在；ARP 读邻居表注入点、ping 注入点、TCP dial 注入点，包覆盖 100%）+ agent `oob.probe`（ip 必需，ports/timeout 秒可选）+ server POST /api/oob/probe 转发端点（oob capability 门禁、RBAC 归 inventory 写、审计 oob_probe 记目标 IP 与端口清单，判定结果不落审计），Go 门禁全绿（全量短测+块覆盖登记门 87.98%/有效 100%+全量 -race 63 包）；**OpenWrt 片B-2 调度上报已落 6fd7a77**：internal/oob/liveness.Scheduler（周期串行不重叠、单周期并发上限、空清单 Run 即返回、Prober/Ticks 注入点，包覆盖 100%）+ agent 启动参数 `-liveness-targets/-liveness-ports/-liveness-interval` 与 liveness_report 组包上行（新消息类型，不混用 probe_report——网络层在位与服务面在位回答的问题不同，混流会让消费方误读在位语义）+ server 内存最新态 upsert 与 GET /api/oob/liveness（agent_id/ip 过滤、stale 新鲜度标记、不落库、纯读不审计、RBAC 归 inventory 读）+ known_uncoverable 登记 1 条（decodeLivenessReport Marshal 失败防御，同 probe_report 口径），Go 门禁全绿（全量短测+块覆盖登记门 88.04%/有效 100%+全量 -race 63 包）；**OpenWrt 片C 兜底转发编排已落 d545a93+d96d727**：internal/oob/redirect 纯逻辑层（规则名 cockpit_{label}_{过期Unix}，三纪律=前缀即归属/强制 TTL/过期时刻进名字；ubus 写契约未绑定实机，隔离在纯 payload 构造层，真机修契约只动一层）+ openwrt client add/del/list firewall redirect + agent oob.redirect/.remove/.list 三 RPC（凭据只经参数流，不进错误文案）+ server POST /api/oob/redirect、POST /api/oob/redirect/remove、GET /api/oob/redirect（双确认 confirm_label/confirm_name 逐字一致；凭据 vault oob/{name} 每请求解析，明文只在加密 WS 与内存态；审计 oob_redirect/oob_redirect_remove 记规格与 agent 回传的规则名+过期时刻，agent 拒绝也留痕），Go 门禁全绿（全量短测+块覆盖登记门 88.16%/有效 100%+全量 -race 64 包）；**OpenWrt 片C-3 过期清扫已落 ea8adde**：agent `oob.redirect.sweep`（列出→只拆 cockpit 前缀且名字过期时刻已过的规则，单条拆除失败即中止报错，凭据不进错误文案）+ server POST /api/oob/redirect/sweep（confirm_oob 双确认指认 vault 条目，拆什么由 agent 按规则名定）+ 审计新动作 oob_redirect_sweep（记拆除清单，拒绝同走 oob_redirect 留痕），受影响两包 702+1194 测试 -short 与 -race 全绿 + 块覆盖包级探测全登记；**DASH CLI 集成面已落**：`internal/cli/oob.go:33`（`OobDiscoverCmd` 走 `registry.ProbeAll`，`internal/cli/oob.go:55` 生产注册 amt/dash/redfish，`internal/cli/oob.go:85` per-provider 三态 Present/NotPresent/Unknown 只按探测结果归类——正命中=Present、明确协议拒认=NotPresent、其余含传输失败/超时=Unknown，缺证据不猜；`internal/cli/oob.go:68` 输出各 provider 状态与证据来源），入口 `cmd/cockpit/main.go:213`（`oob discover` 子命令，`cmd/cockpit/main.go:74` 接线），无新增能力面（power/console/KVM 随凭据片，不猜），Go 门禁全绿（受影响 `internal/cli`+`cmd/cockpit`+`internal/oob/...` 短测 100% 包覆盖+块覆盖登记门+全量 -race 绿；全量短测仅 `internal/server` 按既有环境红口径超时未绿，与本片无关）；**OpenWrt 片C web 双确认管理页已落 b285c8a**：`web/src/pages/Oob`（/oob 路由+菜单：列表/新建/拆除/清扫 + confirm_label/confirm_name/confirm_oob 逐字双确认、RBAC inventory 读/写、vault 条目名输入；api 方法+类型、13 页测 + App/api 测试更新），web 全量门禁绿（100 文件 1142 用例 + lcov 308/308 登记有效覆盖 100%）〕〔BLOCKED：缺真机（OpenWrt 片D 自启动验收）｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)
  - 2026-10-27：DASH 首片收尾完成（`go build ./...` 绿；`internal/oob/dash` 13 测试（11 顶层+2 子用例）+包覆盖 100% 绿；`go vet` 绿；server OOB discovery 定向绿；server 全量按既有环境红口径未绿，golangci-lint 因 go1.26 二进制 < go.mod 1.27 未跑成）。
  - 2026-10-28：DASH 直连查询片收尾完成（commit `b2a1300`；`internal/oob/registry.go:85` `ProbeOne` 单路探测、按名定位、探测错误原样保留不吞；`internal/cli/oob.go:87` `OobDashQueryCmd` 复用 `internal/cli/oob.go:124` `printOobResult` 三态 Present/NotPresent/Unknown、缺证据不猜、输出复用 `oobPresence`；入口 `cmd/cockpit/main.go:271` `oob dash` 子命令，`-address` 必需、`-timeout` 限 (0,30]，`-h` 文档化三态），无写路径；受影响 `internal/oob`+`internal/cli`+`cmd/cockpit` 短测 100% 语句覆盖+块覆盖登记门 0 新增豁免+`go vet` 绿；全量短测仅 `internal/server` 按既有环境红口径超时未绿，与本片无关）。
  - 2026-10-22：OOB 发现协商片收尾完成（oob/audit 两包全量 + server 定向 6 用例绿；server 全量按既有 600s 环境红口径未重跑）。
- [ ] P1 支持机型矩阵文档（中英，三态实测制）/ Snapdragon Guardian 研究档 / IP-KVM 外置兜底〔待拍板=P0 后〕〔BLOCKED：缺用户拍板（P0 后）｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)

### 虚拟化 Provider（2026-10-20 用户裁决，未立项）

> **用户裁决：agent 侧集成 libvirt**（KVM/QEMU 与 LXC 一处调用；libvirt 亦有 Hyper-V driver）。
> 口径：本地 hypervisor 由驻宿主 agent 经 libvirt socket 调用并经既有 RPC 上报，
> 不让 Server 直连宿主 libvirt；PVE 沿用既有 REST（internal/pve），Windows Hyper-V 备选
> hcsshim/HCS；Firecracker/cloud-hypervisor 留给弹性池片再议。

- [ ] agent libvirt 集成：能力探测（hypervisor/LXC）、VM 生命周期（列表/启停/快照/克隆只读先行）、经 agent RPC 上报〔**只读第一片已落 e36ea6e**：`internal/agent/rpc/libvirt_provider.go:93`（`Capabilities` 经 `virsh -r capabilities` 连 daemon 并解析 `<domain type='kvm'/lxc'>` 出 hypervisor/LXC，`libvirt_provider.go:138`（同函数解析）；daemon 不可达返回 available=false 并保留传输细节不猜驱动；URI/driver 经 `virsh -r uri` 最佳努力元数据，失败不降级判定）+ `libvirt_provider.go:164`/`libvirt_provider.go:176`（`vms.list`/`containers.list` 只读，`virsh -r list --all --name`，输出形状对齐 `pve.vms.list`/`pve.containers.list`）+ 注册 `internal/agent/providers.go:145`（`virt-libvirt` capability 且 virsh 在 PATH）+ capability 上报 `internal/agent/agent.go:572`（metadata read_only=true）；无任何写 action（Call 未知 verb 失败关闭）；验证二选一选**单测 mock**（`libvirt_provider_test.go` 与 `libvirt_wiring_test.go` 注入假 runner/PATH 桩，零 virsh 子进程；未用 PVE 192.168.5.10 真机，因本片是驻宿主 agent 经本地 libvirt socket，开发机无 virsh/libvirt socket，真机验收留写片后）；受影响 `internal/agent/...`（含 rpc）短测+块覆盖登记门（99.99%，1 语句既有豁免）+`-race`+`go vet` 全绿；剩余=写操作面（启停/快照/克隆）与真机/API 验收随生命周期片〕〔BLOCKED：缺 libvirt 绑定选型与生命周期片拍板｜2026-11-01〕〔复核 2026-11-04：所缺外部输入/真实条件仍未提供，不重试、不虚构〕
  - 已代决:按 docs/review/2026-10-30-decisions.md 执行(2026-10-30)〔立即动作已落：docs/design/libvirt-write-slice.md〕

### 文档

- [x] archive 5 篇是否双语：历史归档（plans 2 / specs 2 / 归档说明 1）当前仅中文〔已定 2026-10-16（R1025PATROL 代拍板，用户事后可推翻）：归档是历史记录，英文版无读者价值，只保留中文不补英文版；决策记录 docs/guide/decision-log.md D-2026-10-16-1〕
- [x] docs 双语配对第二批：子目录 guide 37、design 5、operations 1、research 1 篇 + 首页英文版精修（d2bc3cd；guide 37/37、design 5/5、operations 1/1、research 1/1 全配对；英文页跨页链接全量对账补齐残留 36 处 / 12 文件——此前声称「43 处、残留中文清零」只覆盖部分页面，含无扩展名相对链接在 VitePress 下解析回中文页；现存 zh 指向仅 2 处预期项（index.en 语言切换、architecture.en→archive 无英文孪生）、死链 0、vitepress build 113 页 exit 0；archive 部分拆出为上方待办条目）

### 代码深审 P2/P3（2026-11-01 深审导入，逐条待修；明细见 docs/review/2026-11-01-code-review.md）

- [x] G1 agent messageLoop 连接代数竞态：陈旧循环在 reconnect 后误拆新连接/并发双 websocket 读协程（internal/agent/agent.go:635）〔修复=连接代数计数器，messageLoop 代数失配即退出〕
  - 证据：`9f77bdd`（`internal/agent/agent.go` 加 connGen 连接代数，connect 时递增；messageLoop 启动快照代数，读前/读错后/读成后校验，失配即退出不触发重连；`internal/agent/g1_message_loop_test.go` 回归：连接换代后陈旧循环退出、不误触发 reconnect、不在新连接上双读。`go test ./internal/agent/` 100s 绿 + `-race` 104s 绿 + `go vet` 绿）
- [x] G2 proxy LastRead 竞态：conn.mu 写 / proxy.mu 读同一字段，time.Time 撕裂读误关活跃代理（internal/proxy/mgr/manager.go:460）〔修复=读持 conn.mu 或 atomic.Int64〕
  - 证据：`8af9c67`（`internal/proxy/mgr/manager.go` 新增 markRead/lastReadAt 方法统一经 conn.mu 读写 LastRead；读协程 markRead、cleanup/状态导出 lastReadAt；`g2_lastread_test.go` 并发读写 race 回归。`go test ./internal/proxy/mgr/` 39s 绿 + `-race` 83s 绿）
- [x] G3 proxy conns 泄漏：目标侧关闭与写错路径不 delete(h.conns)，长跑 agent 会话泄漏 AgentTargetConn（internal/proxy/handler.go:275）〔修复=deferred 清理与写错路径 delete〕
  - 证据：`c7307ab`（`internal/proxy/handler.go` 新增 removeConn（指针校验防同 ID 覆盖误删）；readFromTarget defer 摘表、HandleProxyData 写错路径摘表；`g3_conns_leak_test.go` 三回归。`go test ./internal/proxy/` 0.14s 绿 + `-race` 1.8s 绿）
- [x] G4 taskFollows 覆盖泄漏：followId 复用不停旧 pumpTaskLog 协程，无界重复泵与 proxy_data 流（internal/agent/rpc/stack_task_follow.go:88）〔修复=覆盖前关旧 stop channel 并等待〕
  - 证据：`148fadc`（`internal/agent/rpc/stack_task_follow.go`：taskFollowSession 加 done 通道；FollowTask 覆盖同 followId 时先摘表，锁外关旧 stop 并等待 done 再启新 pump；pump 终态/stop 均关 done；`g4_follow_overwrite_test.go` 两回归（覆盖停旧泵、无死锁）。`go test ./internal/agent/rpc/` 70s 绿 + `-race` 85s 绿）
- [x] G5 guacd 握手无读期限：guacd 停滞则 handler 协程永久阻塞，泄漏协程/连接/监听器/注册条目（internal/server/api_guacamole.go:476）〔修复=握手与泵设读期限〕
  - 证据：`efa11d8`（`internal/server/api_guacamole.go` 新增 guacdHandshakeTimeout（15s，var 可注入）；select 响应 ReadString 前设读期限，成功后清除（泵不设静默超时防误断）；`g5_guacd_handshake_timeout_test.go` 回归用不响应 guacd mock 验证超时收尾。`go test ./internal/server -run "Guacamole|Guac"` 相关子集绿，G5 测试 3/3 稳定绿）
- [x] G6 GetLogs 单次读截断：日志流超首 Read 交付量即静默截断无标记（internal/docker/client.go:268）〔修复=循环读到 EOF/1MB 上限并附标记〕
  - 证据：`3534731`（`internal/docker/client.go` GetLogs 循环读到 EOF，1MB 上限截断并附 `[logs truncated at 1MB]` 标记；`g6_getlogs_test.go` 两回归（多块读全、超限截断）。`go test ./internal/docker/` 0.15s 绿 + `-race` 2s 绿）
- [x] S1 requireAgentRclone nil 解引用：无 backup 能力 agent 的 cap.Metadata 访问 panic 成 500 而非 400（internal/server/api_backups.go:187）〔修复=cap==nil 守卫或 HasCapability〕
  - 证据：`a1f1c33`（`internal/server/api_backups.go` requireAgentRclone 加 cap==nil 守卫，按 rclone 未启用同判 400；`s1_require_rclone_nil_test.go` 回归。`go test ./internal/server -run "RequireAgentRclone|Backup"` 226s 绿）
- [x] S2 NextBackupRunAt 索引越界：daily@ 无冒号历史行 parts[1] 越界 panic，startBackupLoop 协程无 recover 崩进程（internal/server/backup_loop.go:105）〔修复=len(parts)!=2 返回 0〕
  - 证据：`d477687`（`internal/server/backup_loop.go` NextBackupRunAt 对 daily@ 校验 len(parts)==2，异常返回 0（manual 语义）；`backup_test.go` 加 TestNextBackupRunAtMalformedDailyNoColon。`go test ./internal/server -run "Backup|NextBackupRunAt"` 215s 绿）
- [x] S3 proxy LastRead 零值误关：新连接未发首字节即 now.Sub(zero) 超 5 分钟，首 tick 强关握手连接（internal/proxy/mgr/manager.go:460）〔修复=创建时置 time.Now() 或 IsZero 跳过〕
  - 证据：`2d2fd2d`（`internal/proxy/mgr/manager.go` handleConnection 创建 ProxyConn 时 LastRead 置 time.Now()；`s3_lastread_init_test.go` 回归走 handleConnection + cleanup 不误删。`go test ./internal/proxy/mgr/` 63s 绿 + `-race` 92s 绿）
- [x] S4 workflow 并发双跑：check-then-act 无 running 唯一约束，并发 POST /run 双编排重复执行步骤（internal/server/api_workflow.go:490）〔修复=条件创建或 partial unique index〕
  - 证据：`bcfb9d2`（`internal/storage/workflow.go` CreateWorkflowRun 事务内原子检查无 running 实例，冲突返回 ErrWorkflowRunConflict；`internal/server/api_workflow.go` 映射 409；storage 并发 8 路仅 1 成功回归 `s4_workflow_conflict_test.go` + handler 409 回归 `s4_workflow_run_test.go`；既有 orphan_sweep 适配新约束。storage 全包 450s 绿 + 相关 race 子集 17s 绿 + server Workflow 64s 绿）
- [x] S5 guacSessions 握手失败泄漏：五条失败路径从不 delete，泄漏 GuacamoleSession 并可误关未连会话（internal/server/api_guacamole.go:447）〔修复=各失败路径 closeGuacamoleSession/delete〕
  - 证据：`8844f2d`（`internal/server/api_guacamole.go` 新增 failGuacamoleSession（幂等关连接+摘 guacSessions，不改 failed 状态）；六条握手失败路径统一走它；`s5_guac_handshake_fail_test.go` 回归。既有 select-write/read-fail/malformed-args 注入测试绿）
- [x] S6 guacRelay.Close 竞态：快照后 acceptLoop 插入新 conn 永不关闭，泄漏 guacd fd/目标连接/泵协程（internal/server/guac_relay.go:138）〔修复=持 r.mu 关全部或快照后复查〕
  - 证据：`c6b8f96`（`internal/server/guac_relay.go` Close 持 r.mu 置 closed 并关全部+清 map；handleConn 在 closed 时拒绝新连接；`s6_guac_relay_close_test.go` 回归。`go test ./internal/server -run "GuacRelay|S6"` + `-race` 绿）
- [x] W1 TerminalModal 孤儿 WebSocket：票据在途关闭后无条件建 ws，向已 dispose xterm 写并泄漏 PTY（web/src/components/TerminalModal/index.tsx:106）〔修复=代数/closed ref，失配关闭退出〕
  - 证据：`bfced32`（`web/src/components/TerminalModal/index.tsx` 加 sessionGenRef 代数；票据在途返回时/回调/清理时校验代数，失配放弃建 WS 并忽略回调；`index.test.tsx` 加「票据在途卸载不建孤儿 WS」回归。TerminalModal 29 测试绿，web 全量 1152 测试绿）
- [x] W2 播放倍速竞态：setTimeout 无代数守卫，慢速先到清掉快速 timers 停错倍速（web/src/pages/Recordings/index.tsx:151）〔修复=倍速变更 bump 代数，非当代跳过/清理〕
  - 证据：`dfd13d9`（`web/src/pages/Recordings/index.tsx` 加 playGenRef 代数；play 内 await cast 后校验代数（旧代迟到不得清新代 timers）+ 每个 timer 回调校验代数跳过；`index.test.tsx` 加「倍速切换旧代迟到 cast 不清新代」回归。Recordings 20 测试绿）
- [x] W3 ForgotPassword 计时器泄漏：setInterval 卸载不清，最多 60s 对卸载组件 setState（web/src/pages/ForgotPassword/index.tsx:49）〔修复=timer ref + 卸载清理〕
  - 证据：`fe7a404`（`web/src/pages/ForgotPassword/index.tsx` 加 countdownTimerRef；startCountdown 先清旧 interval，useEffect 卸载时 clearInterval；`index.test.tsx` 加「卸载清理倒计时」回归。ForgotPassword 6 测试绿）
- [x] W4 BackupCodes 剪贴板无 catch：http 下 clipboard undefined 同步 TypeError，拒绝成未处理 promise（web/src/components/BackupCodesDisplay/index.tsx:21）〔修复=try/catch 或 .catch + 卸载清理〕〔commit 9b00c9d6：web vitest 备份码用例绿；go 包不受影响〕
- [x] W5 Domains 剪贴板无 catch：拒绝未处理且 message.success 先报成功（web/src/pages/Domains/index.tsx:435）〔修复=then/catch 链 + 守 undefined〕〔commit b40d682d：web vitest Domains 用例绿；go 包不受影响〕
- [x] G7 job Exec 超时泄漏：CombinedOutput 后不 cancel，计时器挂满 per-job 超时（internal/agent/rpc/job_provider.go:99）〔修复=WithTimeout 后 defer cancel〕〔commit ea4c5c55：internal/agent/rpc 全包 + -race 绿，含 TestJobExecReleasesSessionAndTimer〕
- [x] G8 GetContainerStats 读错忽略：部分/失败读产出空 stats 无错误，docker 方法无 per-op 期限（internal/docker/client.go:293）〔修复=查 Read 错误 + per-call 超时〕〔commit 0994fbf4：internal/docker 全包 + -race 绿，含 stats 读错/坏 JSON/空 payload 回归〕
- [x] G9 FollowStart 容量误拒：16 活跃时重启既有 follow 被误拒，替换逻辑本支持同 ID（internal/agent/rpc/logs_follow.go:115）〔修复=先查既有 ID 再测容量〕〔commit fcd2b16d：internal/agent/rpc 全包 + -race 绿（跳过 2 个既有基线 flaky：TestStackGitDeployBusySkipped、TestCovPctFileSearchScannedLimit），含 TestLogsFollowCapacityReplaceAtMax〕
- [x] G10 probe-agent transitions 竞态：close 后写 nil 与 alerter 并发读指针竞态（cmd/cockpit-probe-agent/main.go:116）〔修复=mutex 守护或属主协程置 nil〕
  - 证据：本 commit（`cmd/cockpit-probe-agent/main.go` 加 `stateMu` 守护 `transitions`/`written`；aleniter 回调锁内检查非阻塞发送，flushStatus 锁内捕获副本后置 nil 再锁外 close，幂等收尾；`go test ./cmd/cockpit-probe-agent/...` 全绿含 `-race`，`go vet` 绿，`go build ./...` 绿）
- [x] S7 proxy conns 覆盖泄漏：同 connID 重发 proxy_new 覆盖旧条目不关旧连接/协程（internal/proxy/handler.go:133）〔修复=覆盖前关闭删除既有条目〕
  - 证据：本 commit（`internal/proxy/handler.go` 加 `replaceConn`：锁内放新条目后锁外 `Close` 旧条目，`HandleProxyNew` 与 `openSSHProxy` 复用；`g3_conns_leak_test.go` 加 `TestS7ReplaceConnClosesOldSameID` 回归。`go test -race ./internal/proxy/...` 全绿含新回归，`go vet` 绿，`go build ./...` 绿）
- [x] S8 通知 goroutine 无追踪：context.Background 派生无 WaitGroup 协程，关停中可跑过 DB 关闭（internal/notification/service.go:149）〔修复=生命周期 context 或 WaitGroup〕
  - 证据：本 commit（`internal/notification/service.go` 加生命周期 ctx/cancel + WaitGroup + closed 标志，`SendNonBlocking` 从 s.ctx 派生并计数，`Close` 幂等取消并 Wait 在途投递，`Shutdown` 在 db.Close 前调用 `notifier.Close()`；`service_test.go` 加 `TestServiceCloseWaitsInflight`/`TestServiceCloseIdempotent` 回归。`go test -race ./internal/notification/...` 全绿含新回归，`go vet` 绿，`go build ./...` 绿）
- [x] S9 密码重置邮件吞错：printf 空操作吞 SendPasswordResetEmail 错误，API 假报成功（internal/server/password_reset_handlers.go:62）〔修复=log.Printf + 追踪〕
  - 证据：本 commit（`internal/server/password_reset_handlers.go` 将 `printf` 空实现改为 `log.Printf`，邮件发送失败真实落日志；`password_reset_handlers_test.go` 覆盖回归。`go test ./internal/server -run ForgotPassword -count=1` 绿，`go vet` 绿，`go build ./...` 绿）
- [x] S10 ParseBackupSources 忽略错误：sources JSON 损坏产出空清单照发 run 假完成（internal/server/backup_loop.go:93）〔修复=返回错误，startBackupRun 拒绝〕
  - 证据：`internal/server/backup_loop.go:89` `ParseBackupSources` 改为 `([]string, error)`，损坏 JSON 返回错误；`backup_loop.go:204` `startBackupRun` 在 `CreateBackupRun` 前校验 sources，损坏即拒绝不照发空清单；`backup_test.go:63` `TestParseBackupSources` 加损坏 JSON 断言 + `cov_loops_test.go:196` `TestCovStartBackupRunGuards` 加 malformed-sources 拒绝回归。`go test -short` 定向全绿 + `go vet` 绿。
- [x] W6 GuacPlayer canvas 残留：effect 重跑旧 canvas 同留 DOM（web/src/components/GuacPlayer/index.tsx:100）〔修复=cleanup 先移除旧显示元素〕
  - 证据：本 commit（`web/src/components/GuacPlayer/index.tsx:101` 挂载时记录 `displayElement`，`:108` cleanup 在 pause/disconnect 后 `displayElement.remove()` 移除旧 Display；`web/src/components/GuacPlayer/index.test.tsx:96` 新增 `effect 重跑时移除旧 Display` 回归——rerender 新 blob 后旧 `guac-display` 脱离 DOM 且仅剩 1 个。web vitest 全量 100 文件/1159 用例绿 + `npm run build` 绿；另注 `npx tsc --noEmit` 的 Recordings 未用变量报错为既有缺陷（dfd13d92b 引入，`git stash` 验证与本改动无关），按仓库 build 脚本口径不改）
- [x] W7 dashboard 下拉刷新空操作：onRefresh 空 async 即收不重取，Future.wait 在 build 重建重启（mobile/lib/pages/dashboard_page.dart:19）〔修复=future 缓存字段 + onRefresh 重赋值 setState〕
  - 证据：本 commit（`mobile/lib/pages/dashboard_page.dart:14` 改为 `ConsumerStatefulWidget`，`:22` `_load` 构建 `Future.wait` 并在 `_DashboardPageState` 缓存为 `_future` 字段（`:29` `_refresh` 重赋值 setState 触发重取，`:43` build 仅在 `_future == null || !identical(_futureApi, api)` 时重建，避免 build 重跑重启加载）；`mobile/test/dashboard_page_test.dart:135` 新增「下拉刷新真实重取并更新卡片」回归——fling 下拉后 adapter 第二轮返回 1 台并断言 `/ 2` → `/ 1`。`flutter analyze` 0 问题 + 全量 `flutter test --coverage` 187 用例全绿 + `tool/coverage_check.sh` 门禁通过（未覆盖行全登记））
- [x] W8 终端重连订阅泄漏：旧流订阅不取消，onDone/onError 在新连接后假报错横幅（mobile/lib/pages/terminal_page.dart:136）〔修复=重连先取消 _sub 并关旧 channel〕
  - 证据：本 commit（`mobile/lib/pages/terminal_page.dart:43` `_connect` 增 `_connectGeneration` 代际计数，`:48` 重连时 `unawaited(_sub?.cancel())` 取消旧订阅、`:62` 新通道就绪后关闭旧 channel，`:75` listen 的 onError/onDone 与 `_onServerMessage` 均带 generation 守卫，过期订阅回调被拦截不再 `_fail`；重连按钮 `:172` 直接 `onPressed: _connect` 不再同步关旧 channel；`mobile/test/terminal_page_test.dart:275` 新增「重连后旧连接 close 不触发错误横幅（W8 回归）」——两次真 zone refresh 建立两条真实 WS 会话后关闭 `sessions[0]`，断言旧订阅回调被代际守卫拦截、无错误横幅。`flutter analyze` 0 问题 + 全量 `flutter test --coverage` 188 用例全绿 + coverage 门禁通过（新 guard 行 63 已登记 KNOWN_UNCOVERABLE））

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
- 自研 mesh 组网数据面（2026-10-20 用户令：mesh 组网交给 meshlet；本机基础信息由
  cockpit-agent 只读供给，边界见 meshlet ADR-013，重开需再拍板）
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

## 审核发现

- [x] 审核发现：Capabilities 只在默认 libvirt 连接上执行 `virsh -r capabilities`，默认 qemu URI 时无法发现独立的 `lxc:///` 驱动，LXC 会被误报为不支持（internal/agent/rpc/libvirt_provider.go:101）。
  - 属实，已修：`internal/agent/rpc/libvirt_provider.go:128` 在默认连接之外追加 `virsh -r -c lxc:/// capabilities` 独立探测，LXC 驱动只在 lxc:/// 连接上可见时仍上报 `lxc=true`；`libvirt_provider.go:130` 取并。证据 commit `0cd7232`（test `libvirt_provider_test.go` `TestLibvirtCapabilitiesProbesLXCConnection`）。
- [x] 审核发现：containers.list 与 vms.list 复用同一条默认连接的 `virsh -r list --all --name`，未切到 `lxc:///`，会把 QEMU 域当作容器返回或漏掉全部 LXC（internal/agent/rpc/libvirt_provider.go:177）。
  - 属实，已修：`internal/agent/rpc/libvirt_provider.go:193` `Containers` 改走 `listNames(libvirtLXCURI, "--all")` 直连 `lxc:///`；`libvirt_provider.go:180` `VMS` 保持默认 qemu 连接。证据 commit `0cd7232`（test `TestLibvirtContainersUseLXCConnection` 断言 `-c lxc:///`）。
- [x] 审核发现：vms.list/containers.list 返回 `{vms: []string}`/`{containers: []string}`，而 PVE 既有契约返回 `[]VM`/`[]Container`，注释所称“对齐 PVE 输出形状”不成立（internal/agent/rpc/libvirt_provider.go:169）。
  - 属实，已修：`internal/agent/rpc/libvirt_provider.go:185,197` 返回扁平 `[]string` 名称列表（本只读片只有名称，与 PVE `[]VM` 对象形状本就不同），并改注释不再声称对齐 PVE。证据 commit `0cd7232`（test `TestLibvirtListVMSAndContainers` 断言 shape 为 `[]string`）。
- [x] 审核发现：Capabilities 失败时返回 `available=false` 与 error，但 rpc.Handler 在 error 时丢弃 result，远端只能收到错误、拿不到宣称的 `available=false`（internal/agent/rpc/libvirt_provider.go:103）。
  - 属实，已修：`internal/agent/rpc/libvirt_provider.go:119` daemon 不可达时返回 `{"available": false, "error": detail}` 且 error 为 nil，不再走 rpc.Handler 的 error 丢弃 result 路径（`router.go:96` `err != nil` 分支）。证据 commit `0cd7232`（test `TestLibvirtCapabilitiesDaemonUnavailable` / `TestLibvirtCapabilitiesDaemonErrorNoStderr` 断言 error=nil 且 available=false）。
- [x] 审核发现：`virt-libvirt` 已注册上报，但 server 没有任何 `virt.*` 转发或 `/api/agents/{id}/virt/*` 路由，与 hypervisor 设计契约不一致且产品侧无法调用（internal/agent/agent.go:574）。
  - 属实，已修：新增 `internal/server/api_virt.go:21` `handleAgentVirtAPI`（`/agents/{id}/virt/{sub}` 只读白名单 `virt.capabilities`/`virt.vms.list`/`virt.containers.list`，api_virt.go:13-16），`internal/server/api.go:156` 接线路由。证据 commit `0cd7232`（test `api_virt_test.go` `TestVirtCapabilitiesForward`/`TestVirtRouting`）。
- [x] 审核发现：libvirt 测试用同一份 list 输出同时断言 vms 与 containers，且未断言 `lxc:///` 连接或 PVE 返回形状，固化了上述错误（internal/agent/rpc/libvirt_provider_test.go:136）。
  - 属实，已修：`libvirt_provider_test.go` 拆分 `TestLibvirtListVMSAndContainers`（断言 vms 走默认连接、shape `[]string`）与新增 `TestLibvirtContainersUseLXCConnection`（断言 `-c lxc:///`）、`TestLibvirtCapabilitiesProbesLXCConnection`。证据 commit `0cd7232`。
- [x] 审核发现：Present 帮助称 DASH Identify 已返回发现响应，但 401/403 无响应时也会输出 Present 且 Evidence 固定为“provider discovery response”，会误导操作者（cmd/cockpit/main.go:285）。
  - 属实，已修：`internal/oob/oob.go:65` 新增 `Discovered` 字段（仅识别响应确认后置 true：dash.go:69/redfish.go:113/amt.go:73）；`internal/cli/oob.go:134` `oobPresence` 仅在 `Err==nil && Info.Discovered` 时判 Present，401/403 可达未定判 Unknown（oob.go:141 evidence「endpoint reachable but discovery response not confirmed」）。证据 commit `76f16d8`（test `internal/cli/oob_test.go` `TestOobPresenceClassification` 新增 reachable-undetermined 用例）。
- [x] 审核发现：`-timeout` 帮助写“1-30”秒，实际校验与错误文案允许 `(0,30]`（例如 0.1 秒），CLI 自述不一致（cmd/cockpit/main.go:274）。
  - 属实，已修：`cmd/cockpit/main.go:232,274` 帮助文案改 `(0-30]`，与校验 `*timeout <= 0 || *timeout > 30` 一致。证据 commit `76f16d8`。
- [x] 审核发现：拆除按钮对非 cockpit_ 前缀规则同样渲染，而后端仅允许拆除 cockpit 前缀规则（否则 400），UI 允许用户发起必败动作（web/src/pages/Oob/index.tsx:210）。
  - 属实，已修：`web/src/pages/Oob/index.tsx:46` 新增 `isOwned`（`name.startsWith('cockpit_')`），`web/src/pages/Oob/index.tsx:219` 仅 `isOwned(r.name)` 时渲染拆除按钮，与后端 `redirect.Owned` 同口径。证据 commit `60fbe06`（test `index.test.tsx:292`「非 cockpit 前缀规则不渲染拆除入口」）。
- [x] 审核发现：测试夹具含 user_forward/bare_rule 两条非 cockpit 规则，但未断言其不显示拆除入口、也未覆盖后端 400 拒绝路径，该契约不一致无回归保护（web/src/pages/Oob/index.test.tsx:38）。
  - 属实，已修：`web/src/pages/Oob/index.test.tsx:292` 新增用例断言仅 2 条 cockpit 规则有拆除按钮、user_forward/bare_rule 无。证据 commit `60fbe06`。
- [x] 审核发现：显式选中主机掉线后 effectiveAgentId 仍指向该机，列表查询继续启用并请求离线 agent（未重置/禁用），无对应测试（web/src/pages/Oob/index.tsx:92,105）。
  - 属实，已修：`web/src/pages/Oob/index.tsx:100` `effectiveAgentId = oobAgents.some((a) => a.id === agentId) ? agentId : oobAgents[0]?.id`，显式选中主机掉线（不在 oobAgents）时回退首个在线 oob 主机，列表不再请求离线 agent。证据 commit `60fbe06`（test `index.test.tsx:300`「显式选中的主机掉线」）。
- [x] 审核发现：新建表单强制 label 必填，但后端 Spec.Validate 允许空标签（按 proto/目标自动推导规则名），UI 未暴露后端合法路径（web/src/pages/Oob/index.tsx:324 vs internal/oob/redirect/redirect.go:77）。
  - 属实，已修：`web/src/pages/Oob/index.tsx:335` label 改为可选（占位「留空 = 按 proto/目标推导」，去 `required` 规则），`index.tsx:404` 确认校验与 `labelValue ?? ''` 对齐，`index.tsx:169` 提交 `label: v.label ?? ''`。证据 commit `60fbe06`（test `index.test.tsx:320`「label 可留空」）。
- [x] 审核发现：提交信息称 hostname||id 边遗漏已登记，实际该边被用例覆盖且未登记于 known_uncoverable.txt，提交声称与实现不符（web/src/pages/Oob/index.tsx:247）。
  - 属实（提交信息失实），已更正：`web/tool/known_uncoverable.txt` 补注释说明该边由 `index.test.tsx:84-85`「选择主机+输入条目」用例覆盖（ag-2 hostname 空走 id 回退），b285c8a 提交信息「hostname||id edge misses registered」不准确。证据 commit `b0799f6`。
- [x] 审核发现：`GET /api/cloud/accounts/{id}/instances/`（尾斜杠）走 handleCloudAccount 的 `strings.Trim` 归一化后被判定为 instances 请求并转发，但 handleCloudInstances 用 `strings.TrimSuffix(rest, "/instances")` 对带尾斜杠的 rest 不生效，id 被解析成 `{id}/instances` 命中 `Contains("/")` 返回 400 "cloud account id required"，与无尾斜杠路径返回 200 不一致（internal/server/api_cloud.go:90 vs :185）。
  - 属实，已修：`internal/server/api_cloud.go:129` 归一化改 `strings.Trim(strings.TrimSuffix(strings.TrimSuffix(rest, "/"), "/instances"), "/")`，尾斜杠与无尾斜杠等价；空/含斜杠 id 仍 400。证据：本 commit `api_cloud.go:129`（test `api_cloud_test.go` `TestCloudInstancesServeMuxRouting` 双路径均 200）。
- [x] 审核发现：instances 端点直接序列化原始 `storage.CloudInstance`（createdAt/updatedAt 为 time.Time，JSON 输出含亚秒 "2026-10-30T12:34:56.789+08:00"），而姊妹 ledger 端点 accounts 经 `cloudAccountToView` 输出格式化字符串 "2006-01-02T15:04:05Z07:00"，同一 API 族两个端点的同名字段时间格式不一致（internal/server/api_cloud.go:118 vs :60-73）。
  - 属实，已修：`internal/server/api_cloud.go:43` 新增 `cloudInstanceView`，`handleCloudInstances` 经 `cloudInstanceToView`（`api_cloud.go:95`）输出格式化字符串 "2006-01-02T15:04:05Z07:00"，与 accounts 端点同口径；`api_cloud_test.go` `TestCloudInstancesTimestampFormat` 断言无亚秒且匹配 RFC3339 形态。证据：本 commit `api_cloud.go:43,95,157`（test `api_cloud_test.go` `TestCloudInstancesTimestampFormat`）。
- [x] 审核发现：新增 `/api/cloud/accounts/{id}/instances` 路由从未经真实 ServeMux 验证——`TestCloudInstancesRegisterRoute` 只请求 `/api/cloud/accounts`（列表路由），其余 instances 用例均经 `callCloudAPI` 直接调 `handleCloudAccount`，绕过 `registerCloudAPI` 的 mux 接线，该端点的路由分发（含 `/instances` 转发判定）无 mux 层测试（internal/server/api_cloud_test.go:186）。
  - 属实，已修：`internal/server/api_cloud_test.go` 新增 `TestCloudInstancesServeMuxRouting`，经 `registerCloudAPI` 的真实 ServeMux 断言 `/instances` 与 `/instances/` 均 200 且含实例，`/api/cloud/accounts/` 走账号处理 405，覆盖 mux 层转发判定。证据：本 commit `api_cloud_test.go` `TestCloudInstancesServeMuxRouting`。
- [x] 审核发现：`DeleteCloudAccount` 只删账号行不删其 `cloud_instances`，而新端点对不存在账号硬回 404，账号删除后其实例成为无法经任何 API 列出/清理的孤儿数据且无级联或清理路径（internal/storage/cloud.go:140 vs internal/server/api_cloud.go:108）。
  - 属实，已修：`internal/storage/cloud.go:140` `DeleteCloudAccount` 改事务内先删 `cloud_instances` 再删账号行，杜绝孤儿实例；不存在账号仍 `ErrNotFound`。证据：本 commit `cloud.go:140`（test `internal/storage/cloud_test.go` `TestDeleteCloudAccountCascadesInstances`）。
- [x] 审核发现：本轮 diff 未见问题（c25f6b2e95434be0ac90f50e638f24c13bf230d3..HEAD）。
  - 属实，已复核：diff 即 commit a58ecb7，逐文件审毕——`api_cloud.go` 尾斜杠归一化（/instances 与 /instances/ 等价）、`cloudInstanceView` 时间戳口径与 accounts 端点一致、`DeleteCloudAccount` 事务内级联删 `cloud_instances`、`cov_agent_test.go` 注入本地不可达 publicIP 端点（不依赖真网）均无问题；受影响 `internal/server`+`internal/storage` 云测试与 `internal/agent` 重连测试全绿（含 -race），`go vet` 绿；`internal/server` 全量短测 600s 超时系既有环境红口径（ticket.go:104 cleanupLoop goroutine 泄漏，非本 diff 引入）。证据：本 commit `a58ecb7`。
- 已删除原「审核发现：本轮 diff 未见问题（a58ecb77..HEAD）」行。不属实：`git log --oneline a58ecb7..HEAD` 实测返回 `4aa14c1`+`73c0a0b` 两个提交，且 `73c0a0b` 存在行号漂移，不能宣称该范围无问题。

- [x] 审核发现：本 diff 同 commit 向 todo.md 插入 29 行「已代决」注释使既有条目行号整体下移，但新增 docs/review/2026-10-30-decisions.md 仍按插入前的旧行号引用 todo.md（如 01→19 正确，02→21 已漂移，实际在 22；覆盖与证据说明的 02→21/03→25/…/28→96 全部落后实际行），读者按引用定位将落到错误条目（docs/review/2026-10-30-decisions.md:609）。
  - 属实，已修：`docs/review/2026-10-30-decisions.md` 逐项改为 01→19、02→22、03→27、04→30、05→32、06→34、07→36、08→38、09→40、10→42、11→44、12→46、13→48、14→50、15→52、16→57、17→59、18→61、19→63、20→75、21→78、22→80、23→92、24→94、25→96、26→108、27→113、28→123，并把 29—33 源改为 `todo.md:241`。证据：本 commit `docs/review/2026-10-30-decisions.md:609`（核对命令 `grep -n 'todo.md:[0-9]' docs/review/2026-10-30-decisions.md`，来源/覆盖表与实际待办行一致）。
- [x] 审核发现：docs/review/2026-10-30-decisions.md:30 阅读纠偏称「todo.md:57 仍把腾讯适配器列为后续，但第 56 行已记录其完成」，但当前 todo.md:57 是防火墙写操作条目，todo.md:56 为腾讯适配器完成说明的 P0 云条目且未把腾讯列后续；该行号定位在插入后已错位（docs/review/2026-10-30-decisions.md:30）。
  - 属实，已修：阅读纠偏改为引用 `todo.md:75`，并澄清“进度注记仍列为后续”；同时删除该过时说法，改为“腾讯适配器已完成（片4 df558ff），后续=发现/生命周期接线”。证据：本 commit `docs/review/2026-10-30-decisions.md:30`、`todo.md:75,77`。
- [x] 审核发现：docs/review/2026-10-30-decisions.md:520/609/673-677 以 todo.md:202 指「表三 5 项能力仅评估」，但当前 todo.md:202 位于「已完成」区且为文档一致性审计行，实际「表三 5 项能力仅评估与 P0/P1/P2 建议」在 todo.md:241（插入后下移 28 行），引用失效（docs/review/2026-10-30-decisions.md:520）。
  - 属实，已修：520/609/673-677 全部改为 `todo.md:241`。证据：本 commit `grep -n 'todo.md:241' docs/review/2026-10-30-decisions.md` 命中 `520`、`609`、`673-677`。
- [x] 审核发现：docs/review/2026-10-30-decisions.md 覆盖表将 20→56/21→58/22→59/23→70/24→71/25→72/26→83/27→87/28→96，但插入后这些 BLOCKED 条目实际位于 75/78/80/92/94/96/108/113/123，全部漂移（docs/review/2026-10-30-decisions.md:669）。
  - 属实，已修：覆盖表 20→75、21→78、22→80、23→92、24→94、25→96、26→108、27→113、28→123，并同步修正 01—19 的漂移源行。证据：本 commit `docs/review/2026-10-30-decisions.md:609,625-667`。
- [x] 审核发现：todo.md:305 新加行「审核发现：本轮 diff 未见问题（a58ecb77..HEAD）」基线错误——a58ecb77..HEAD 实含两个 commit（4aa14c1+73c0a0b），非本轮审查范围（4aa14c14..HEAD 仅 73c0a0b），且同一 diff 已存在上述行号漂移问题，「未见问题」结论不成立（todo.md:305）。
  - 属实，已修：删除该错误结论并改为不属实记录，防止“无问题”误导。证据：本 commit `todo.md:305`；`git log --oneline a58ecb7..HEAD` 结果为 `73c0a0b`、`4aa14c1`（2 commits），`git rev-list --count` 分别为 2/1。

- [x] 审核发现：commit 6233f05 新增「立即执行动作执行结果（2026-10-30 续）」节的已执行/待用户两表仍按插入前旧行号引用 todo.md（20→56、21→58、23→70、24→71、25→72、28→96、29→202、02→21、03→25、04→27、16→42、17→43、26→83、27→87），但实际当前位于 75/78/92/94/96/123/241/22/27/30/57/59/108/113，正是 commit c0be14b 已修正的同源行号漂移在此新节被重新引入（docs/review/2026-10-30-decisions.md:709-755）。
  - 属实，已修：`docs/review/2026-10-30-decisions.md` 新节「已执行/待用户」两表逐项改为当前实际待办行号（已执行 20→75、21→78、23→92、24→94、25→96、28→123、29→241；待用户 01→19、02→22、03→27、04→30、05→32、06→34、07→36、08→38、09→40、10→42、11→44、12→46、13→48、14→50、15→52、16→57、17→59、18→61、19→63、20→75、21→78、22→80、23→92、24→94、25→96、26→108、27→113、28→123、29→241、30→241、31→241、33→241）。证据：本 commit `836f740`（docs VitePress build 绿；Go 全量短测除 `internal/server` 既有环境红口径（ticket.go:104 cleanupLoop 泄漏）外全绿，本次纯文档未触 Go）。
- [x] 审核发现：decisions.md:30 称「todo.md:75 的进度注记仍把腾讯适配器列为后续」，但 commit 6233f05 已把该进度注记（todo.md:77）改为「腾讯适配器已完成（上条片4 已落 df558ff）」，此陈述与当前内容不符（docs/review/2026-10-30-decisions.md:30）。
  - 属实，已修：`docs/review/2026-10-30-decisions.md:30` 改为引用 `todo.md:77`，并澄清“腾讯适配器已完成（片4 df558ff），后续=发现/生命周期接线”，删除过时的“仍列为后续”说法。证据：本 commit `836f740`（docs VitePress build 绿）。
- [x] 审核发现：libvirt-write-slice.md 引用 `internal/agent/rpc/libvirt_provider.go:34` 为 LookPath 检测、`:12-14` 为「-r 只读连接」注释，但实际 LookPath 检测在 `:75`（`libvirtLookPath("virsh")`）、「-r 只读连接」注释在 `:28-29`，两处行号与所指内容不符（docs/design/libvirt-write-slice.md:16-17）。
  - 属实，已修：`docs/design/libvirt-write-slice.md:16-17` 两处引用分别改为 `internal/agent/rpc/libvirt_provider.go:75`（LookPath 检测）与 `:28-29`（`-r` 只读连接注释）。证据：本 commit `836f740`（docs VitePress build 绿）。

- [x] 审核发现：capability 守卫失效——`selectedHasService` 以 `o.backend !== undefined` 判定能力，但 `agentOptions` 里 `backend: (cap?.metadata?.backend as string | undefined) ?? 'systemd'`（index.tsx:81）恒有 `?? 'systemd'` 兜底，`backend` 永不 undefined，故 `o.backend !== undefined` 恒真，守卫实际只验证「selectedAgent 是否在列表」，并未按 capability 过滤；无能力 agent（如 `none-01`）仍会被查询 → 后端 404 噪声依旧，T4 问题未真正解决（web/src/pages/Services/index.tsx:93-96 vs :81）。
  - 属实，已修：`selectedHasService` 改按新增的 `o.hasService`（= `!!cap`，独立于 backend 兜底值）判定，无能力 agent 不再触发 status/list 查询。证据：本 commit（web 101 文件/1149 测试绿，tsc 清零，build 绿）。
- [x] 审核发现：T4 测试未真正走到守卫路径，属假阳性——测试 `selectAgent('none-01（未检测到服务管理）')` 点击的是 disabled 选项；rc-select OptionList.js 的 onClick 有 `if (!mergedDisabled) { onSelectValue(value); }` 守卫，点击 disabled 选项不会触发 onChange，`selectedAgent` 保持 undefined，查询本就不发起，断言 `getAgentServices`/`getServiceStatus` 未调用只是「未选中任何主机」的结果，与 capability 守卫无关（web/src/pages/Services/index.test.tsx:130-139；rc-select OptionList.js:362-365）。要真正测守卫，需直接设置选中一个无能力 agent 值，而非点击 disabled 选项。
  - 属实，已修：测试改为经 React fiber 直达 antd Select 的 `onChange`，直接写入无能力 agent 值 `ag-none`（绕开 rc-select 对 disabled 选项的点击守卫），断言 status/list 均未调用。反证：临时把守卫回退为 `backend !== undefined` 后本用例红（getAgentServices 被调用），确认测试真正踩到守卫路径。证据：本 commit（Services 19 测试绿）。
- [x] 审核发现：暗色模式对比度回退——`#86909c`→`#667085` 改善亮色（3.24→4.97 达标），但四个皮肤暗色 surface（default #141620、midnight #0e1729、eyecare #241f19、graphite #1c1c1e）上由旧值 5.04-5.56:1 降到 3.28-3.62:1，跌破 WCAG AA 4.5:1；且这些硬编码色绕过已接入的 `colorTextSecondary: skin.textMuted` 主题 token（App.tsx:592，暗色下 textMuted 为浅色可过 AA）。受影响：Dashboard/index.less:44,60、AuditLogs/index.tsx:265、Roles/index.tsx:159、workbench/ConnectionPanel.tsx:43、workbench/OverviewPanel.tsx:6。（Login 页背景恒为浅色白盒，不受影响）
  - 属实，已修：5 处硬编码 `#667085` 改为走主题 token——Dashboard/index.less:44,60 改 `var(--cockpit-text-muted, #667085)`，AuditLogs/index.tsx:265、Roles/index.tsx:159 改 `Typography.Text type="secondary"`，workbench/ConnectionPanel.tsx:43、OverviewPanel.tsx:6 改 `var(--cockpit-text-muted, #667085)`；暗色下各皮肤 textMuted 均 ≥5.46:1 过 AA。新增回归守卫 themeSkins.test.ts「皮肤色板对比度（WCAG AA）」（textMuted/text 在每皮肤每档 surface ≥4.5:1），临时把暗色 textMuted 改回 `#667085` 本守卫红。证据：本 commit（web 101 文件/1149 测试绿，tsc 清零，build 绿）。
- [x] 审核待办 T7 收编：审核时仓内无 DESIGN.md（无设计基准可比对），已补 `docs/DESIGN.md`——UI 设计基准（色板/次要文字色/占位色/语义色规范 + WCAG 与布局不变量），供后续审核比对；其内联 `scroll={{ x: 'max-content' }}` 被 Vue 当作插值致 VitePress build 破，改为「`scroll` prop 设 `x: 'max-content'`」。已在 `docs/index.md` 收编链接。证据：本 commit（docs VitePress build 绿）。
- [x] 审核发现：审核报告引用「头部搜索」证据截图与 dashboard-after 字节相同——`docs/review/2026-10-31-ui-audit-shots/header-search-after.png` 与 `dashboard-after.png` md5 均为 `e393325c1e637602b9b6992390ac4c6f`，即 F6 头部搜索修复的「after」截图实为仪表盘截图副本，非独立采集；报告还称「外观不变」，但加入 `enterButton` 后按钮由 icon-only 变为带边框 Button（App.tsx:461），外观确有变化（docs/review/2026-10-31-ui-audit-impeccable.md:39-46）。
  - 属实，已修：`header-search-after.png` 已替换为独立截图（1440×757，当前 md5 `f69f74d32f3a3c5527db4dd00933d1e9`，与 `dashboard-after.png` 的 `e393325c1e637602b9b6992390ac4c6f` 不同）；报告「外观不变」改为「外观变为带边框 Button」。证据：`md5sum docs/review/2026-10-31-ui-audit-shots/{header-search-after,dashboard-after}.png`。
- [x] 审核发现：capability 守卫修复不完整——无能力残留选中值下 `HealthPanel` 的 `getAgentHealth` 查询及 systemd 专属 UI（重载/健康面板）仍会触发，因 `isSystemd` 由 `backend ?? 'systemd'` 兜底派生（无能力时恒为 systemd），未与新增的 `hasService` 守卫同步（web/src/pages/Services/index.tsx:84,90,477；web/src/pages/Services/HealthPanel.tsx:130-133）。
  - 属实，已修：`isSystemd`/`isWindows` 均加 `selectedHasService` 守卫（web/src/pages/Services/index.tsx:97-103），无能力残留值不再渲染 systemd 专属 UI 或触发 `getAgentHealth`。证据：本 commit（Services/index.test.tsx「选中主机后失去服务能力」用例断言健康面板/重载配置/系统状态均收起）。
- [x] 审核发现：T4 测试仅断言 `getAgentServices`/`getServiceStatus` 未调用，未断言 `getAgentHealth` 也未调用，故上述 HealthPanel 查询泄漏未被测试覆盖（web/src/pages/Services/index.test.tsx:157-158）。
  - 属实，已修：T4 测试改为「选中主机后失去服务能力」场景，断言 `getAgentHealth`/`getAgentServices`/`getServiceStatus` 调用次数均不新增（web/src/pages/Services/index.test.tsx:136-167）。证据：本 commit（web 100 文件/1148 测试绿，tsc/build 绿）。
- [x] 审核发现：`setAgentValue` 经 React fiber 内部属性 `__reactFiber`/`__reactInternalInstance` 直达 `onChange`，耦合 React 实现细节，版本升级易碎（web/src/pages/Services/index.test.tsx:99-109）。
  - 属实，已修：删除 `setAgentValue` 及 fiber 内部属性遍历；测试改为正常选中有能力主机后模拟 agent 数据能力撤销并 invalidate `agents` 查询，不再触碰 React 内部实现。证据：`rg '__reactFiber|__reactInternalInstance' web/src/pages/Services` 返回空（其余测试文件不受影响）。
- [x] 审核发现：同一 CSS 变量 `--cockpit-text-muted` 的 fallback 值不一致——本次改动用 `#667085`，而 App.less:23 与 MobileTabBar 用 `#64748b`，同变量两套兜底（web/src/pages/Dashboard/index.less:44,60 vs web/src/App.less:23）。
  - 属实，已修：`Dashboard/index.less:44,60`、`ConnectionPanel.tsx:43`、`OverviewPanel.tsx:6` 的 fallback 统一为 `#64748b`，与 `App.less:23`/`MobileTabBar` 一致。证据：`rg '#667085' web/src/pages/Dashboard web/src/workbench` 返回空（Login 页仍用 `#667085`，非本次改动范围）。
- [x] 审核发现：无能力残留选中值下 `isSystemd`/`isWindows` 均被守卫为 false，导致 Alert 走 `else` 分支渲染「所有启停/自启操作直接作用于 launchd…」误导文案（用户并未选择 macOS/launchd 主机），而该状态实际应展示「未检测到服务管理能力」之类的空态说明（web/src/pages/Services/index.tsx:453-456）。
  - 依据：本次 diff 把 `isSystemd`/`isWindows` 改为 `selectedHasService && …`，但 `Alert` 的 `isWindows ? … : isSystemd ? … : launchd` 三态未同步无能力分支；`selectedHasService=false` 时误显示 launchd 文案。建议为无能力残留值单独分支或复用空态文案（同文件 :404-412 的「仅列出检测到服务管理 capability 的主机…」）。测试 T4（index.test.tsx:136-167）只断言 systemd 专属 UI 收起与三查询不新增，未断言该 Alert 文案，故未覆盖。
  - 属实，已修：`Alert` 增加 `!selectedHasService` 前置分支，无能力残留选中值时显示「未检测到服务管理能力：仅列出检测到 service capability 的主机，请重新选择。」，不再误落 launchd 文案。T4 用例补充断言：该空态文案出现且 launchd 文案不出现。证据：本 commit（web 100 文件/1148 测试绿，tsc/build 绿）。
- [x] 审核发现：本轮 diff 未见问题（01a412d6..HEAD）。证据：commit 0702cb7 已推送；`web/src/App.tsx` 新增 `colorTextDescription: skin.textMuted`；`tsc --noEmit` 与 `npm run build` 均通过。
- [x] 审核发现：本轮 diff 未见问题（2152b362..HEAD）。证据：commit 0702cb7 仅向 `web/src/App.tsx:595` 新增 antd 合法 token `colorTextDescription: skin.textMuted`，与相邻 `colorTextSecondary/Tertiary/Placeholder` 同源一致；token 已在 antd 5.29 alias 接口定义（web/node_modules/antd/es/theme/interface/alias.d.ts:94），无类型/样式风险。；验证：`git diff --name-status 2152b362..HEAD` 仅涉 App 主题样式/审查报告/截图/todo；`App.tsx` ESLint 0 error、`App.test.tsx` 22/22、`Services/index.test.tsx` 19/19、`npm run build` 绿（全量 lint 的 5 个既有 error 均在未改动的 GuacamoleModal/TagManageModal/TerminalModal）。
- [x] 审核发现：README.md 与 README.zh.md 徽章行回归——平台/架构徽章（Windows/Linux/macOS/x86_64/aarch64，5 枚）被删除，改为同一 docs 徽章重复 6 枚，且嵌套两层 `<p align="center">`（开 2 闭 1）形成非法/冗余 HTML；基线 0702cb7e 的 README.md 有 5 枚平台徽章，HEAD 只剩 6 枚相同 docs 徽章（README.md:11-13、README.zh.md:11-13）。
  - 属实，已修：README.md:11 与 README.zh.md:11 恢复 5 枚平台/架构徽章（Windows/Linux/macOS/x86_64/aarch64）并保留 1 枚 docs 徽章，删除 5 枚重复 docs 徽章；嵌套 `<p align="center">` 改为单层，HTML 合法。证据：本 commit（docs VitePress build 绿，build complete in 79.92s；`grep -c 'shields.io/badge/platform' README.md`=1 且 `grep -c 'shields.io/badge/arch'`=1，README.zh.md 同；`grep -c 'img.shields.io/badge/docs-blue' README.md`=1、`img.shields.io/badge/%E6%96%87%E6%A1%A3-blue` README.zh.md=1）。
- [x] 审核发现：docs capability 计数失实——concepts.md 称「共 19 个（恒上报 18 个 + 条件上报 virt-libvirt）」，但代码 `detectCapabilities` 中真正无条件上报仅 file/ddns/oob（约 3 个），其余（openwrt/pve-api/docker-api/hardware-monitor/firewall/overlay/remote-services/backup/nginx-proxy/traefik-proxy/cron/logs/drift/service/nas/virt-libvirt）均为条件上报，「恒上报 18 个」与代码不符（docs/guide/concepts.md:134、docs/zh/guide/concepts.md:134）。
  - 属实，已修：中英文 concepts.md 均改为「共 19 个，仅 `file`/`ddns`/`oob` 恒上报，其余 16 个条件上报」。证据：本 commit（`docs/guide/concepts.md:134`、`docs/zh/guide/concepts.md:134`；代码口径核对 `internal/agent/agent.go:419-578`，恒上报仅 ：459 file、:519 ddns、:565 oob；docs VitePress build 绿，build complete in 86.13s）。
- [x] 审核发现：验收报告行号漂移——2026-11-01 音频验收报告声称单测覆盖在 `index.test.tsx:332`（静音 suspend/resume）与 `:474`（AudioContext 不可用），但后续 717ac3b 在文件前部插入 73 行，当前实际在 :405 与 :547，报告引用已失效（docs/review/2026-11-01-guacamole-audio-acceptance.md:21、:122-123；docs/zh/remote-desktop-guacamole-design.md:429）。
  - 属实，已修：`2026-11-01-guacamole-audio-acceptance.md` 两处与中英文设计文档各一处统一改为 `:405`；报告第 122 行下一行同步改为 `:547`。证据：本 commit（`docs/review/2026-11-01-guacamole-audio-acceptance.md:21`、`:122-123`；`web/src/components/GuacamoleModal/index.test.tsx:405`、`:547`；另修正英文设计文档同源 `:332` → `:405` 于 `docs/remote-desktop-guacamole-design.md:286`；docs VitePress build 绿，build complete in 56.15s）。
- [x] 审核发现：本轮 diff 未见问题（7da10b1f1c66..HEAD）。证据：该区间仅含 commit 1bdf57b「docs: restore readme platform badges」（父提交已确认 = 7da10b1f1c66），纯文档变更。HEAD 的 README.md:12 与 README.zh.md:12 徽章行为单层 `<p align="center">…</p>`（每文件 open_p=close_p=2，含 logo 行，无嵌套），平台/架构徽章（Windows/Linux/macOS/x86_64/aarch64 共 5 枚）已逐字节恢复为与基线 0702cb7e 完全一致，并保留 1 枚 docs 徽章、删除 5 枚重复 docs 徽章；`arch-x86__64` URL 与基线相同，非本区间引入。无代码改动，无注入/越界/泄漏/竞态/反序列化风险面，无测试缺口（docs-only）。区间内 todo.md 仅追加前轮已复核的审计记录。
  - 属实（复核结论成立，无需改动）：`git rev-parse 1bdf57b^`=`7da10b1f1c665f8ef17cf2039f34b2272e6b3358`；区间仅 1 commit，`git diff --name-status` 仅 README.md/README.zh.md/todo.md；两 README `open_p=close_p=2`，平台/架构 5 枚徽章 URL 与基线 `0702cb7e` 逐字节一致，docs 徽章各 1 枚。证据：本 commit（`README.md:12`、`README.zh.md:12`）。
- [x] 审核发现：本轮 diff 未见问题（1bdf57be..HEAD）。
  - 覆盖范围与证据：区间 4 commits（4422537/186a249/eb56b3d/094c31d），`git diff --stat` 全量 9 文件均逐文件复核（docs/audit-doc-consistency.md、docs/guide/acceptance-checklist.md、docs/guide/concepts.md、docs/remote-desktop-guacamole-design.md、docs/review/2026-11-01-guacamole-audio-acceptance.md、docs/zh/guide/concepts.md、docs/zh/remote-desktop-guacamole-design.md、docs/zh/审计-文档一致性.md、todo.md），纯文档无代码改动，无注入/反序列化/越界/泄漏/竞态风险面。修正后的文档↔代码映射逐项核验属实：`guacEncode("audio","audio/L8","audio/L16")` 确在 `internal/server/api_guacamole.go:533`；静音 suspend/resume 单测确在 `web/src/components/GuacamoleModal/index.test.tsx:405`、AudioContext 不可用单测确在 `:547`；capability 口径「共 19 个、仅 file/ddns/oob 恒上报、余 16 条件上报」与 `internal/agent/agent.go` 一致（无条件 append 仅 :459 file/:519 ddns/:565 oob；7 个 detector 均有条件返回 nil 路径 + agent.go 内 9 个条件 capability，7+12=19）；增量文档引用的 8 个 commit 锚（b1410c7/24e0e2e/094c31d/eb56b3d/186a249/dc81806/717ac3b/e3023c2）全部存在且主题相符；acceptance-checklist.md:370、concepts.md:134、design.md:286 行号锚点与改动后位置一致。docs-only 变更无测试缺口。

## 漏洞扫描遗留（2026-11-03 全舰队扫描，源 scratch/vuln-scan-*.md）

- [x] VULN-COCKPIT-001 [非安全·过时依赖] go get -u ./... 因 proxy EOF 部分失败（仅安全项保留）：补跑非安全升级；验收：go list -m -u all 无网络错误 + go build ./... && go test ./... 绿（另：govulncheck ./... 已于 2026-11-03 晚实测 No vulnerabilities found，原 ./internal/rpc/... setup failed 系仓内无此路径——实际为 internal/agent/rpc 等，已闭合）。证据：`go get -u ./...` 无网络错误，`golang.org/x/crypto` 升至 v0.58.1-0.20261009181718-2f1f834330b2、`go 1.27` → `go 1.27.3`（std lib 漏洞 GO-2026-6603/6605/6607-6613/6809 修于 1.27.3）；`go list -m -u all` 无网络错误（exit=0）；`go build ./...` 绿 + `go vet ./...` 绿；`go test -short ./...` 全绿（server 包 -short 单独绿，CI 同口径；非-short 全量 server 包基线同撞 900s 为既有环境红口径）；`govulncheck ./...` → No vulnerabilities found。本 commit

## 审核发现

- [x] 审核发现：BackupCodesDisplay 复制单个恢复码改用共享单一 resetTimerRef，2s 内再复制第二个码会 clearTimeout 掉第一个码的重置定时器，使第一个码的「已复制」高亮永久残留（原实现每码独立 setTimeout 各自复位）（web/src/components/BackupCodesDisplay/index.tsx:47-55）
  - 属实，已修：`web/src/components/BackupCodesDisplay/index.tsx:20` 改为 `resetTimersRef` 每码独立 `setTimeout`，`:50` 仅清同 index 旧定时器；`:56` 复位用函数式更新只删本码，避免过期快照误清其它码；`:72` 卸载统一清理。证据 commit `2e6557d1`（`web/src/components/BackupCodesDisplay/index.test.tsx:55` 新增 1.9s 后复制第二码、两个高亮分别重置回归；web vitest 全绿 100 files/1158 tests + build 绿）。
- [x] 审核发现：G4 的 FollowTask 覆盖停旧泵逻辑被容量检查先行拒绝——taskFollows 满 8 个时 :73 容量判断先于 :81 的 followID 替换判断返回 too-many，覆盖能力在满员时不可达，与 G9 在 logs_follow.go 的「替换不计容量」修复不一致（internal/agent/rpc/stack_task_follow.go:73-76）
  - 属实，已修：`internal/agent/rpc/stack_task_follow.go:79` 改为先取旧会话再判容量（`old == nil && len>=max` 才拒绝），同 followId 覆盖在满员时仍可替换（与 logs_follow.go:117 G9 一致）；新增 `internal/agent/rpc/g4_follow_overwrite_test.go:121` `TestG4FollowReplaceAtCapacity` 回归（满 8 后替换 f0 成功、新增 over 仍拒）。证据：`internal/agent/rpc/stack_task_follow.go:79`、`internal/agent/rpc/g4_follow_overwrite_test.go:121`（`go test -short ./internal/agent/rpc` 54.6s 全绿 + `go vet` 绿）。
- [x] 审核发现：新增 failGuacamoleSession 后原「closeGuacamoleSession 幂等关闭：WS + guacd TCP + 审计结束」doc 注释被留在其上方误挂到 failGuacamoleSession，而真正的 closeGuacamoleSession（:746）反而无注释（internal/server/api_guacamole.go:727-731）
  - 属实，已修：`internal/server/api_guacamole.go:727` 移除误挂到 failGuacamoleSession 的 closeGuacamoleSession 注释，`:745` 在真正的 closeGuacamoleSession 上方补回「幂等关闭：WS + guacd TCP + 审计结束」。docs-only 注释修正，S5 握手失败与正常关闭路径定向测试全绿（`TestS5GuacHandshakeFailureCleansSession`/`TestG5GuacdHandshakeReadTimeout`/`TestCovGuacamoleHandshakeWriteInject`）。证据：`internal/server/api_guacamole.go:745`。
- [x] 审核发现：TestNextBackupRunAtMalformedDailyNoColon 内「空冒号段（daily@:00）同样返回 0」注释与下方断言矛盾（断言期望今日/明日 00:00 而非 0/epoch），且与下一行注释重复（internal/server/backup_test.go:618）
  - 属实，已修：`internal/server/backup_test.go:626` 删除与断言矛盾的「同样返回 0」注释，仅保留下方与断言一致的「解析为 h=0,m=0 合法形态，仅确保不 panic 且落在今日/明日 00:00」注释。证据：`internal/server/backup_test.go:626`（`TestNextBackupRunAtMalformedDailyNoColon` 定向绿）。
- [x] 审核发现：GetLogs 改为循环读到 EOF/1MB 后，follow=true 路径由「返回首个数据块」变为「阻塞至 EOF 或 1MB」，而 GET /docker/agents/{a}/containers/{c}/logs 仍暴露 ?follow=true（api_docker.go:159），活动容器上该请求将挂起到 1MB 累积或容器停止；前端未使用 follow，影响有限（internal/docker/client.go:275-293）
  - 属实，已修：`internal/server/api_docker.go:158` 在同步容器 logs 路由拒绝 `follow=true`（含 `follow=1` 等 ParseBool 真值），不透传给 GetLogs 阻塞；新增 `internal/server/api_docker_test.go:164` `TestParseDockerContainerLogsRejectsFollow` 回归（`follow=true` 与 `follow=1` 均 400）。证据：`internal/server/api_docker.go:158`、`internal/server/api_docker_test.go:164`（`go test -short ./internal/docker` 全绿 + 定向 server 绿）。

- [x] 审核发现：W8 重连改动把「关闭旧 channel」延迟到新连接成功之后（terminal_page.dart:69），但新连接失败（如 createRemoteTicket 抛错）时 catch（:87-89）只置错误态、从不关闭仍存活的原 `_channel`，且 `_terminal.onOutput`（:71）未重绑仍写向旧 channel——重连失败后旧连接成僵尸：订阅已取消但 socket 未关、终端输入仍发往无人读取的旧流，直至下次成功重连才释放；旧实现的重连按钮在 connect 前先 `_channel?.sink.close()` 无此泄漏。测试缺口：新增 W8 回归仅覆盖「成功重连+旧订阅关闭」，未覆盖「新连接失败而旧连接仍活」路径（mobile/lib/pages/terminal_page.dart:67-69,87-89）
  - 属实，已修：`mobile/lib/pages/terminal_page.dart:87` catch 分支新增 `unawaited(_channel?.sink.close())` + `_channel = null` + `_terminal.onOutput = null` + `_terminal.onResize = null`（重连失败即关闭旧僵尸 socket、解绑旧 xterm 回调，终端输入不再发往无人读取的旧流）；新增 `mobile/test/terminal_page_test.dart:373` `终端页：重连失败 → 旧连接被关闭且终端输出断开（W8 回归）`——先建真实 WS 旧连接，再让 ticket 500，断言服务端观测旧 socket 断连且错误横幅出现。证据：`mobile/lib/pages/terminal_page.dart:87`、`mobile/test/terminal_page_test.dart:373`（`flutter test test/terminal_page_test.dart` 9 用例全绿 + `flutter analyze` 0 问题）。本 commit
- [x] 审核发现：S10 修复后 `ParseBackupSources` 返回错误，但备份配置视图 `backupConfigToView` 用 `sources, _ :=` 丢弃该错误（api_backups.go:122）——损坏的 sources JSON 在视图静默渲染为空清单，与本 commit 将解析错误视为硬失败（run 路径 `startBackupRun` 拒绝）的口径不一致；虽非本 diff 引入的行为回归（旧实现同样忽略解析错误），仍建议视图侧对损坏记录显式暴露而非伪装成「无来源」（internal/server/api_backups.go:122）
  - 属实，已修：`internal/server/api_backups.go:109` `backupConfigView` 新增 `SourcesOK`/`SourcesErr`，`backupConfigToView`（`:124`）不再丢弃 `ParseBackupSources` 错误——损坏 sources JSON 视图显式暴露 `sources_ok=false` + 错误文案，不再伪装成「无来源」；`internal/server/backup_test.go:243` `TestBackupConfigViewSurfacesMalformedSources` 回归断言损坏配置被标记且有效配置正常；`web/src/pages/Backups/index.tsx:236` 渲染 `sources_ok===false` 时显式「源路径损坏」Tag + Tooltip 错误，`web/src/pages/Backups/index.test.tsx:215` 新增回归；`web/src/types/index.ts:770` 加 `sources_ok?/sources_err?`。证据：`internal/server/api_backups.go:109`、`internal/server/backup_test.go:243`（`go test -short ./internal/server` 绿）；`web/src/pages/Backups/index.tsx:236`、`web/src/pages/Backups/index.test.tsx:215`（web vitest 全量 100 files/1160 tests 绿 + build 绿）。本 commit

- [x] 审核发现：W8 修复的 catch 分支（mobile/lib/pages/terminal_page.dart:90-93）在 `unawaited(_channel?.sink.close())`、`_channel = null`、`_terminal.onOutput = null`、`_terminal.onResize = null` 前未校验 generation；当一次陈旧连接（gen 已落后）的 `createRemoteTicket`/`_openChannel` 异步失败发生在新一代连接已成功建立之后，catch 会误关新一代连接的 channel 并解绑其终端回调，页面仍显示 connected 但终端失效（mobile/lib/pages/terminal_page.dart:90）
  - 不属实：`_connect` 的 try 起点同步置 `_state=connecting`（terminal_page.dart:66-69）并取消旧订阅，refresh 按钮仅当 `_state != connecting` 渲染（terminal_page.dart:139）；gen 卡在 `createRemoteTicket`/`_openChannel` 异步期间无法经 UI 触发新一代，且 `_openChannel` 返回后成功路径同步执行到底、函数即结束，该代际的 catch 不可能在新一代已建立后触发，场景在真实 UI 不可达。
