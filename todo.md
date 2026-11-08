# Cockpit Todo

需求清单：`- [ ]` 待办、`- [x]` 已完成（一行证据：commit + 测试/验收规模）。过程细节查 git log 与 docs/。

执行纪律：

- 单任务单 commit；commit 信息英文简短，不提 AI；push 前 `git pull --rebase`。
- 先落设计文档再动码；测试与构建绿才 push。
- 不打版本号 tag、不发 Release；版本号由用户手动给出。
- 密钥不进仓库；解锁口令与明文凭据不落日志、不进错误消息；yaml 密钥字段只用 `${VAR}`。
- 真机验收不代测：等硬件/凭据到位按 `docs/guide/acceptance-checklist.md` 推进。

## 待办

> 2026-11-05 整编：一行一条需求，完成项只留一行证据；过程细节查 git log、docs/review/ 与各条目引用的验收清单小节。

### 真机验收（等硬件/真实环境/凭据）

- [ ] service-health 生产接入：docker VM agent 改 SERVER_URL 指生产后 enable〔阻塞=缺生产 enrollment 凭据/授权改 SERVER_URL 并 enable｜解锁=凭据到位后改 SERVER_URL 并 enable〕
  - 2026-11-06 复核：生产入口已可达（`wss://cockpit.cuihairu.site/ws` 实测 HTTP/1.1 升级 101 握手成功 + `/health` 200 `{"agents":1}`），原「缺生产 WS 地址」阻塞解除；但 docker VM 192.168.5.5 的 `/etc/default/cockpit-agent` 的 `SERVER_URL` 仍指向已停用 dev `ws://192.168.5.188:9000/ws`（agent `bad handshake`、重启计数 77297），未改生产入口前不勾；本地无生产 `AGENT_ENROLLMENT_SECRET`/admin 凭据，不擅自改生产 agent（真机验收不代测）。
- [ ] Windows SCM 服务管理：连接层 list/启停/自启/restart 30s 超时路径提权实测（校验层 4 项已两台真机双证）〔阻塞=缺提权 admin agent 会话〕
  - 2026-11-06 复核：Windows 主机 10.240.0.5:22 可达（`OpenSSH_for_Windows_9.5`，known_hosts 有记录），但 SSH 认证失败（publickey/password 均 Permission denied，含 `~/.ssh/id_ed25519_win`），无法进入提权会话实测；阻塞=缺提权 admin agent 会话仍成立，不勾。
- [ ] macOS launchd 服务管理：launchctl 执行层 list/启停/自启/kickstart 回退（校验层探针已备，checklist:251）〔三台真机 openclaw/imac/.5 均为 Windows，无 macOS 主机（macOS 项保持跳过）〕
- [ ] NAS 真机 API：DSM 6/7、TrueNAS CORE/SCALE、OMV；mdadm 降级演练；ZFS/SMB/NFS 主机（checklist:223-227）〔阻塞=缺 NAS 设备与账号〕
- [ ] 备份云远端：真实 S3/B2、限速、GB 级完整链（server 侧 rclone 需宿主安装，checklist:51-52）〔阻塞=缺真实 S3/B2 凭据与宿主 rclone〕
- [ ] 录制异地：rclone 真远端推送（checklist:61,62,68,69）〔阻塞=缺 rclone 真远端〕
- [ ] ACME：staging/production 签发、自动续期、签发→部署→浏览器信任链（checklist:111-117）〔阻塞=缺真实 ACME 账号与 DNS 凭据〕
- [ ] DDNS：真 Cloudflare token、家庭侧 agent 出口 IP 多源（checklist:123-126）〔阻塞=缺真实 Cloudflare token〕
- [ ] overlay：真 ZT/TS token 授权/除名、join/leave、systemctl 单元态、第二台主机合并观测（checklist:166-168）〔阻塞=缺真实 token 与第二台主机〕
- [ ] SMART：带 attr 表主机读数、FAILED 端到端翻转（checklist:180,184）〔阻塞=缺带 SMART 属性表的真实磁盘〕
- [ ] DNS 真账号：DNSPod/阿里云 SRV 与 TTL 下限、台账联动全链、M4 跨 provider 迁移演练（checklist:133-135）〔阻塞=缺真实 DNSPod/阿里云账号〕
- [ ] 移动端真机：Android release 安装/splash/R8/签名升级、生物识别、ntfy 送达、SSH 终端（checklist 移动端一节 :355 起）〔阻塞=缺 Android 真机与签名/ntfy 凭据〕
- [ ] web 移动端触屏与 Workbench 触屏交互真机验收〔阻塞=缺触屏真机（代码地基已随 web 移动适配交付，仓内无缺口）〕
- [ ] iOS 签名与装机（低优先级）〔阻塞=缺 Apple 开发者账号与 Mac（编译验证已在 CI）〕

### 功能

- [ ] 防火墙写操作（规则增删）〔搁置=D-2026-10-07-3 否决直接写；解锁=只读真机验收闭环后重开评估〕
- [ ] workflow 并行分支执行〔搁置=workflow-design W4/G3 压后；解锁=真实并行需求出现再立项〕
- [ ] vault 硬验收：真 persona 服务端实测 provider 切换〔阻塞=缺 persona 服务端地址与 token〕
- [ ] jenkins/external endpoint 凭据 vault 引用〔阻塞=缺链接功能立项〕

### 多云扩展（2026-10-20 评审导入；P0 已拍板开工，P1/P2 未立项待拍板；全文核验见 docs/review/chatgpt-2026-10-20-multicloud-positioning.md）

- [x] P0 云账号接入/资源自动发现/Provider Framework/计算生命周期闭环/成本地基：适配器与接线片已落（cfeee64/8bf385c/df558ff，`internal/cloud` aliyun/tencent、账号 web 双确认）〔2026-10-20 用户拍板开工（首批厂商已代决：阿里云+腾讯云）〕；剩余=厂商发现/生命周期接线（仓内可开工，缺口清单见 docs/design/cloud-lifecycle-wiring.md）、Quota 子片与双厂商闭环验收〔阻塞=缺真实云 AccessKey（仅 Quota 子片与闭环验收依赖凭据；发现/生命周期接线不阻塞）｜解锁=凭据经 vault 到位〕
- [ ] P1 资源池与容量/Go Reconciler/云网络/预算策略引擎/双重控制边界裁定（「OpenTofu vs Reconciler 写入权」先裁定，见 docs/design/p1-resource-ownership.md）〔阻塞=缺用户裁决（P0 闭环后）〕
- [ ] P2 智能调度/成本优化/控制面高可用〔阻塞=缺真实需求〕

### 竞品功能强化（未立项待拍板；全文+核验表见 docs/review/competitor-landscape-2026-10-20.md）

- [ ] P0 Agent 侧四件：Capability 协议/调度资源隔离（限流·优先级·超时）/升级回滚/任务可靠性（重试·窗口·幂等）〔阻塞=缺用户拍板（已落设计 docs/design/agent-capability-contract.md）〕
- [ ] P0 安全两件：短期凭据+资源级权限（依托既有 vault）/轻量运行模式〔阻塞=缺用户拍板（已落设计 docs/design/resource-permission-matrix.md）〕
- [ ] P1 离线数据缓冲/远程协作（会话共享）/统一电源控制（WoL+PVE）/四套源码深读笔记〔阻塞=缺用户拍板（P0 后；已落设计 docs/design/offline-buffer-design.md）〕

### 带外管理 OOB（2026-10-20 评审+用户裁决导入；P0 已拍板开工，P1 未立项待拍板；全文/矩阵/跟踪见 docs/review/oob-management-2026-10-20.md）

- [ ] OpenWrt 片D 真机自启动验收：断电/看门狗恢复（框架/Redfish/AMT/DASH/WOL f9210ab/探活 62b7e6d/procd 看门狗 32bf697 片已落）〔2026-10-20 用户裁决第四线：自启动可靠〕〔OpenWrt 硬件不在三台真机 openclaw/imac/.5 之列（均为 Windows），断电/看门狗恢复需 OpenWrt 设备（待用户提供 OpenWrt 设备）〕
- [ ] P1 支持机型矩阵文档（中英，三态实测制，未实测不入表）/Snapdragon Guardian 研究档/IP-KVM 外置兜底〔阻塞=缺用户拍板（P0 后）〕

### 虚拟化 Provider（只读片已落 e36ea6e；写片设计见 docs/design/libvirt-write-slice.md）

- [ ] agent libvirt 写操作面（启停/快照/克隆）与真机/API 验收〔阻塞=缺 libvirt 绑定选型与生命周期片拍板〕

### 代码深审 P2/P3（明细见 docs/review/2026-11-01-code-review.md）

- [x] W8 残余竞态：重连按钮同帧双击可启动重叠 `_connect`（按钮要下一帧重建才消失，terminal_page.dart:169），旧代请求失败时 catch 无 generation 校验会误关新连接（mobile/lib/pages/terminal_page.dart:90）〔2026-11-05 文档审计证实可达；修法=catch 分支补代际校验〕〔证据：本 commit，mobile 全量 190 用例+analyze 0 问题；回归用例「重连同帧双击 → 旧代失败不误关新连接」实测无修复时失败（旧代失败误关新连接+置错误态）、有修复时绿〕

## 已完成

### 架构与 agent

- [x] Phase 0-7 收口：CLI 三命令、inventory 六类同步、server/agent 拆包、协议类型化、远控权限审计、前端拆页、CI 收口
- [x] agent-core 三层架构 P1-P7（2667a12 起 10+ commit，agent 树覆盖 100%）
- [x] grdp 旧桌面路线删除，桌面统一走 Guacamole（D-2026-10-07-1，40 包绿）
- [x] 服务检测独立 agent：简档/Guard 状态机/调度器/web 面板（20b406f 至 569e5f8，20+ 用例）
- [x] execlog 执行留痕：协议面+审计归集+脱敏环+rpc 单点埋点（ed35cb6/126b84e）
- [x] healthprobe 三语义+故障窗口判定上收（71bfb7c/73fd220）
- [x] 巡检循环 ctx 可中断收口六条同构循环（bd3fef4+d152c85）
- [x] agent 测试注入点数据竞争修复（CI main race 腿红）：injectMu 统一守卫 14 个注入 var（injectSwap 写/injectRead 读、锁外做 I/O），三处「无数据竞争」失实注释改写（3d607919；-race 全包+30× 压测+coverage_check 全绿，PR #108 十 checks 绿，合并后 main CI 四 workflow 绿）

### 远控

- [x] 三协议统一 Guacamole：SSH 接入、web 分流、尺寸同步、剪贴板双向（探针 16 场景）
- [x] guacd 经 agent 中继转发、不直拨目标（34d606c/0767b8b，16/16 两轮）
- [x] 真机验收 6 项全勾（2026-09-30，证据 `.acceptance/guac/`）
- [x] 会话台账：terminal/guacamole 双链路登记、DELETE 终止收口
- [x] egress 多策略并集判定（9a563b9）
- [x] WS 空 Origin 放行修复、SSH 默认密钥劫持修复（显式凭据优先五分支单测）
- [x] 外部新建远程会话契约（memex）：docs/design/external-session-api.md（3ee30fe）

### 密码箱

- [x] vault 抽象+local provider+接入点第一波（93473a0/47367b4/8fd31e8，54 包绿）
- [x] vault 第二波：配置 `vault:` 引用十字段+ACME 私钥与 capability config 落库加密（6bf4f35/2904035）
- [x] persona provider：只读映射、fail-closed、写删 API 映射 501（cc51840，29 测试+8 子测试）
- [x] Guacamole 音频真机：RDP 音频 mimetype 与 L8/L16 编解码确认（D4，静音按钮已实现）〔证据：openclaw 真机验收 2026-11-01，协商 audio/L16 44.1kHz 2ch、网关空 audio 声明修复；docs/review/2026-11-01-guacamole-audio-acceptance.md〕

### 资源与运维面

- [x] Docker 容器生命周期与日志查看（P0）
- [x] 健康探测 runner（P0）
- [x] 拨测增强：间隔可配、四渠道通知、心跳条、阈值配置、告警去重、`probe.interval`
- [x] Compose Stack M1/M1.5+上传+真机 R1-R17+P2 三条（本地验收 68/68）
- [x] 备份：agent 备份/恢复/分块下载+server SQLite 热备+异地保留+前置钩子（真机 K0-K11 44/44、B0-B3、R0-R3）
- [x] 反向代理：nginx+Traefik 双后端（真机 N0-N8、T1-T10）
- [x] 文件管理器 M1-M5：浏览/搜索/预览/分块续传/chmod-chown/跨刷新续传
- [x] 日志 M1-M3：查询、实时尾随、跨机联邦检索（真机 6/6）
- [x] 定时任务 M1-M4：cron 管理、下次触发、systemd timer、多用户
- [x] 服务管理三后端 systemd/Windows SCM/macOS launchd+mask/journal/daemon-reload/unit 文件（真机 17/17）
- [x] 服务健康探针自愈 M1/M2（真机场景 A/B）
- [x] drift M1-M6：基线/巡检/diff/以当前为准/双栏对照/CMDB 一致性
- [x] 录制 M1-M3：cast/guac 收集回放+异地归档
- [x] DNS M1-M4：Cloudflare/DNSPod/阿里云、台账写联动、批量导入导出
- [x] overlay M1-M3：运行态观测、云管理面、join/leave+frp/WireGuard 真机
- [x] SMART 磁盘健康+真机 S0-S4
- [x] DDNS M1+巡检语义真机 D1-D5
- [x] ACME：lego DNS-01 三 provider+签发后多目标部署+证书联动
- [x] 防火墙 M1 只读观测（探针 F/H/W 全 PASS）
- [x] NAS M1/M2：本地+DSM/TrueNAS/OMV
- [x] workflow：M1+变量传递、扇出、取消信号、审批门、DAG 依赖（真机 13/13）

### web 与移动端

- [x] web 前端存量收口：region/zone 对齐、tsc 清零（8f928baf/72adcbf8）
- [x] Flutter M1-M9 全线（158 测试）
- [x] Android release：签名/R8/splash/权限最小化
- [x] web 移动适配：全站表格兜底+MobileTabBar 底部导航（1076 用例）
- [x] README 瘦身 236→55 行+文档站收口
- [x] 运维开源调研 docs/research/ops-oss-survey.md（0422d3f）

### 工程与门禁

- [x] 覆盖率三端收口：Go 台账制 100%、web lcov 有效 100%、mobile 99.83% 登记（tool/coverage_check.sh 三端）
- [x] race 收口：auth/proxy/agent 三处数据竞态+TempDir 清理竞态根治（5578f79+d152c85）
- [x] CI flake 根治：testlog EFBIG、Settings 镜像 flake、覆盖门禁假绿修复
- [x] agent 过期自动判定与列表隐藏（dc6163b/a871306）
- [x] region/zone unknown 语义（D-2026-10-08-1）、proxyType 拒收 udp（D-2026-10-08-2）
- [x] 文档一致性审计五轮（docs/audit-doc-consistency.md；第五轮 2026-11-05：1/0/4，P0=0）
- [x] e2e 冒烟+`scripts/acceptance/` 十二功能域
- [x] install.sh/install.ps1 一键安装+服务注册+nightly 矩阵
- [x] Docker 镜像发布链路：GHCR workflow+部署 compose+deploy-docker.md
- [x] JWT 惰性初始化（import 零副作用）

### 文档

- [x] archive 5 篇只保留中文不补英文版〔证据：D-2026-10-16-1，docs/guide/decision-log.md〕
- [x] docs 双语配对第二批：guide 37/design 5/operations 1/research 1 全配对〔证据：d2bc3cd，vitepress build 113 页绿〕
- [x] 审核整改（luna review 三表：缺陷/文档不一致/能力评估）〔证据：docs/review/luna-review-2026-10-18-response.md；全量 go test+race+web/docs 门禁绿〕
- [x] 文档时效性清理：30 文件/15 组中英成对，数值/API/锚点/废弃口径对齐代码〔证据：vitepress build 113 页 exit 0，2026-10-19〕

### 代码深审修复（G/W/S 系列，明细 docs/review/2026-11-01-code-review.md）

- [x] G1 agent messageLoop 连接代数竞态〔证据：9f77bdd，internal/agent 短测+race 绿〕
- [x] G2 proxy LastRead 撕裂读误关活跃代理〔证据：8af9c67，mgr 短测+race 绿〕
- [x] G3 proxy conns 泄漏：目标侧关闭/写错路径不摘表〔证据：c7307ab，三回归〕
- [x] G4 taskFollows 覆盖泄漏：覆盖前停旧泵〔证据：148fadc，rpc 短测+race 绿〕
- [x] G5 guacd 握手无读期限：handler 永久阻塞〔证据：efa11d8，Guacamole 子集 3/3 绿〕
- [x] G6 GetLogs 单次读截断：循环读到 EOF/1MB 上限附标记〔证据：3534731，docker 包两回归〕
- [x] G7 job Exec 超时泄漏：WithTimeout 后 defer cancel〔证据：ea4c5c55，rpc 全包+race 绿〕
- [x] G8 GetContainerStats 读错忽略：Read 错误+per-call 超时〔证据：0994fbf4，docker 包+race 绿〕
- [x] G9 FollowStart 容量误拒：先查既有 ID 再测容量〔证据：fcd2b16d，rpc 全包+race 绿〕
- [x] G10 probe-agent transitions 竞态：mutex 守护+属主收尾〔证据：本 commit，probe-agent 包+race 绿〕
- [x] S1 requireAgentRclone nil 解引用：cap==nil 守卫〔证据：a1f1c33，Backup 子集 226s 绿〕
- [x] S2 NextBackupRunAt 索引越界：len(parts)!=2 返回 0〔证据：d477687，Backup 子集绿〕
- [x] S3 proxy LastRead 零值误关：创建时置 time.Now()〔证据：2d2fd2d，mgr 回归+race 绿〕
- [x] S4 workflow 并发双跑：事务内原子检查+409〔证据：bcfb9d2，storage 并发回归+handler 409 回归〕
- [x] S5 guacSessions 握手失败泄漏：failGuacamoleSession 六路径统一〔证据：8844f2d，回归绿〕
- [x] S6 guacRelay.Close 竞态：持锁关全部+拒绝新连接〔证据：c6b8f96，GuacRelay+race 绿〕
- [x] S7 proxy conns 覆盖泄漏：replaceConn 锁外关旧〔证据：本 commit，proxy race 全绿〕
- [x] S8 通知 goroutine 无追踪：生命周期 ctx+WaitGroup〔证据：本 commit，notification race 全绿〕
- [x] S9 密码重置邮件吞错：log.Printf 落日志〔证据：本 commit，ForgotPassword 定向绿〕
- [x] S10 ParseBackupSources 忽略错误：损坏 JSON 拒绝发 run〔证据：本 commit，定向绿〕
- [x] W1 TerminalModal 孤儿 WebSocket：sessionGenRef 代数守卫〔证据：bfced32，TerminalModal 29 测试+web 全量绿〕
- [x] W2 播放倍速竞态：playGenRef 代数守卫〔证据：dfd13d，Recordings 20 测试绿〕
- [x] W3 ForgotPassword 计时器泄漏：timer ref+卸载清理〔证据：fe7a404，6 测试绿〕
- [x] W4 BackupCodes 剪贴板无 catch〔证据：9b00c9d，vitest 绿〕
- [x] W5 Domains 剪贴板无 catch〔证据：b40d682，vitest 绿〕
- [x] W6 GuacPlayer canvas 残留：cleanup 移除旧 Display〔证据：本 commit，web 全量 100 文件/1159 用例+build 绿〕
- [x] W7 dashboard 下拉刷新空操作：future 缓存+重赋值〔证据：本 commit，flutter 187 用例绿〕
- [x] W8 终端重连订阅泄漏：代际计数+旧订阅取消+失败关僵尸 socket（mobile/lib/pages/terminal_page.dart:43-95）〔证据：本 commit，terminal_page 9 用例+analyze 0 问题；残余同帧双击竞态见待办〕

### 审核发现（逐条裁决记录；过程细节查 docs/review/ 与 git log）

- [x] libvirt capabilities 漏 lxc:/// 独立驱动——属实，已修〔证据：0cd7232〕
- [x] libvirt containers.list 未切 lxc:/// 会混报——属实，已修〔证据：0cd7232〕
- [x] libvirt list 返回形状注释失实——属实，已修〔证据：0cd7232〕
- [x] libvirt available=false 被 rpc.Handler error 路径丢弃——属实，已修〔证据：0cd7232〕
- [x] virt-libvirt 无 server 转发路由——属实，已补 /api/agents/{id}/virt/*〔证据：0cd7232〕
- [x] libvirt 测试固化上述错误——属实，已拆分回归〔证据：0cd7232〕
- [x] DASH Present 在 401/403 时误导——属实，加 Discovered 三态判定〔证据：76f16d8〕
- [x] oob dash -timeout 帮助「1-30」与校验 (0,30] 不一致——属实，文案已改〔证据：76f16d8〕
- [x] OOB 拆除按钮对非 cockpit_ 前缀规则渲染——属实，加 isOwned〔证据：60fbe06〕
- [x] OOB 测试未断言非 owned 规则无拆除入口——属实，补用例〔证据：60fbe06〕
- [x] OOB 显式选中主机掉线不回退——属实，回退首个在线 oob 主机〔证据：60fbe06〕
- [x] OOB label 必填而后端允许推导——属实，改可选〔证据：60fbe06〕
- [x] b285c8a 提交信息「hostname||id 边遗漏」失实——属实，登记表更正〔证据：b0799f6〕
- [x] cloud instances 尾斜杠 400 与无尾斜杠不一致——属实，归一化已修〔证据：本 commit〕
- [x] cloud instances 时间戳格式与 accounts 端点不一致——属实，统一 RFC3339〔证据：本 commit〕
- [x] cloud instances 路由缺 mux 层测试——属实，补 TestCloudInstancesServeMuxRouting〔证据：本 commit〕
- [x] DeleteCloudAccount 不级联删 cloud_instances——属实，事务内级联〔证据：本 commit〕
- [x] 「本轮 diff 未见问题（a58ecb77..HEAD）」基线错误——属实，结论撤销〔证据：区间实含 2 commits〕
- [x] decisions.md 引用 todo.md 行号漂移 29 处——属实，已逐项改〔证据：本 commit〕
- [x] decisions.md:30 腾讯适配器「仍列为后续」过时——属实，已改〔证据：本 commit〕
- [x] decisions.md 以 todo.md:202 指表三条目失效——属实，改指实际行〔证据：本 commit〕
- [x] decisions.md 覆盖表 20-28 行号漂移——属实，已改〔证据：本 commit〕
- [x] todo.md:305「未见问题」基线错误——属实，撤销并记录不属实〔证据：本 commit〕
- [x] 6233f05 新节重新引入行号漂移——属实，已逐项改〔证据：836f740〕
- [x] decisions.md:30 腾讯适配器陈述再次过时——属实，已改〔证据：836f740〕
- [x] libvirt-write-slice.md 引用行号与所指内容不符——属实，已改〔证据：836f740〕
- [x] Services capability 守卫恒真（backend 兜底）——属实，改 hasService 判定〔证据：本 commit，web 101 文件/1149 测试绿〕
- [x] T4 测试假阳性（点击 disabled 选项）——属实，改经 onChange 直测守卫〔证据：本 commit，反证回退守卫则红〕
- [x] 暗色模式对比度跌破 WCAG AA——属实，5 处改主题 token+对比度守卫测试〔证据：本 commit，web 全量绿〕
- [x] 审核无 DESIGN.md 基准——属实，补 docs/DESIGN.md 并收编〔证据：本 commit，vitepress build 绿〕
- [x] header-search-after 截图系仪表盘截图副本——属实，已换独立截图并改「外观不变」表述〔证据：md5 双图不同〕
- [x] isSystemd/isWindows 未与 hasService 守卫同步——属实，均加守卫〔证据：本 commit〕
- [x] T4 未断言 getAgentHealth 不触发——属实，断言三查询均不新增〔证据：本 commit〕
- [x] setAgentValue 耦合 React fiber 内部属性——属实，删除并改正规选路〔证据：rg 返回空〕
- [x] --cockpit-text-muted fallback 两套值——属实，统一 #64748b〔证据：rg 返回空〕
- [x] 无能力残留选中值渲染 launchd 误导文案——属实，加空态分支〔证据：本 commit〕
- [x] 未见问题（01a412d6..HEAD）——复核成立，无需改动〔证据：0702cb7 纯 token 新增〕
- [x] 未见问题（2152b362..HEAD）——复核成立，无需改动〔证据：0702cb7 仅 App.tsx token〕
- [x] README 平台徽章回归为重复 docs 徽章+嵌套 p——属实，已恢复 5 枚并修 HTML〔证据：1bdf57b，vitepress build 绿〕
- [x] concepts.md「恒上报 18 个」失实——属实，改「仅 file/ddns/oob 恒上报」〔证据：094c31d，agent.go 口径核对〕
- [x] 音频验收报告行号漂移（:332→:405、:474→:547）——属实，四处已改〔证据：本 commit，vitepress build 绿〕
- [x] 未见问题（7da10b1f..HEAD）——复核成立〔证据：1bdf57b 纯文档，徽章与基线逐字节一致〕
- [x] 未见问题（1bdf57be..HEAD）——复核成立〔证据：4 commits 逐文件复核，锚点全对上〕
- [x] BackupCodesDisplay 复制共享单 timer 致高亮残留——属实，改每码独立 timer〔证据：2e6557d，web 100 文件/1158 测试+build 绿〕
- [x] G4 覆盖停旧泵在满员时不可达——属实，先取旧会话再判容量〔证据：本 commit，rpc 短测绿〕
- [x] failGuacamoleSession 误挂 closeGuacamoleSession 注释——属实，注释归位〔证据：本 commit，S5/G5 定向绿〕
- [x] backup_test 注释与断言矛盾——属实，删矛盾注释〔证据：本 commit，定向绿〕
- [x] GetLogs 循环读后 ?follow=true 同步端点挂起——属实，该路由拒 follow〔证据：本 commit，docker 短测+回归绿〕
- [x] W8 catch 分支无代际校验可能误关新一代——初判不属实（其依据行号 :66-69/:139 系漂移，实际 :50-53/:169）；2026-11-05 审计改判：同帧双击可启动重叠连接，场景可达，登记为待办代码修复〔证据：本 commit〕
- [x] W8 复核说明引用 terminal_page.dart:66-69/:139 无法定位——属实，已改 :50-53/:169〔证据：本 commit〕
- [x] 文档一致性审计波次 2：external-session-api 台账基座锚漂移、审计锚表 role.go 区间漂移、todo W8 复核行号漂移、W8「不可达」结论被同帧双击路径推翻、luna-review 4 处历史锚越界保留原样〔证据：docs/audit-doc-consistency.md 第五轮，1/0/4，P0=0〕

### 漏洞扫描遗留（2026-11-03 全舰队扫描）

- [x] VULN-COCKPIT-001 非安全过时依赖补升级〔证据：golang.org/x/crypto v0.58.1+、go 1.27.3；go build/vet/test -short 绿；govulncheck 无漏洞〕

## 不做（边界决策，重开需再拍板）

- Kubernetes 风格 CRD 全量模型（除非先定 v2 inventory 迁移方案）
- 替换前端 UI 框架
- SQLite 换 PostgreSQL
- 扩展更多远控协议（现 RDP/VNC/SSH+telnet 已收稳）
- workflow SSE
- P8 configsync（server 配置源面不立项）
- 移动端 VNC/桌面观看（D7；触发条件见 docs/guide/mobile-design.md）
- 大规模目录重命名
- 自研 mesh 组网数据面（2026-10-20 用户令：mesh 组网交给 meshlet；本机基础信息由 cockpit-agent 只读供给，边界见 meshlet ADR-013，重开需再拍板）

## 审核发现

- [x] 审核发现：多云 P0 待办将剩余工作缩成 Quota 与验收，漏掉尚未实现的厂商发现和生命周期接线，易误判凭据是唯一阻塞（todo.md:43）——属实，已修〔证据：本 commit；todo.md:43 剩余口径改为「厂商发现/生命周期接线（仓内可开工，缺口清单 docs/design/cloud-lifecycle-wiring.md）+Quota 子片+双厂商闭环验收」；核实 `internal/server/api_cloud.go` 仅账号+只读实例台账，`cloudProviders` 注册表从未 `Register`（provider.go/registry.go）〕
- [x] 审核发现：整编删除了多云与 OOB P0 已获批准的标记，却保留整节「未立项待拍板」，会误导后续工作授权判断（todo.md:41）——属实，已修〔证据：本 commit；todo.md:41/53 标题改「P0 已拍板开工，P1/P2 未立项待拍板」，条目补 2026-10-20 拍板/裁决；对齐 docs/review/2026-10-30-decisions.md:30「标题未拍板不能推翻条目内已批准事实」〕
- [x] 审核发现：中英文 OpenWrt 设计及 OOB 评审的新 todo 行号指向竞品功能而非 OOB 条目，英文片 D 还指向竞品章节标题（docs/design/oob-openwrt-bootstrap.md:10）——属实，已修〔证据：本 commit；docs/design/oob-openwrt-bootstrap.md 与 zh 版 todo.md:49/47 全部改 :55；核对当前 todo.md:55=OpenWrt 片D 条目（39e958ce 后移 2 行，现行 :57，见审核发现 :276）〕
- [x] 审核发现：多云评审的新 todo.md:34 引用指向功能章节而非多云 P0，且同处仍将已落地的腾讯适配器列为待做（docs/review/chatgpt-2026-10-20-multicloud-positioning.md:214）——属实，已修〔证据：本 commit；:214/:216/:219 改 todo.md:43，:219 腾讯云 CVM 片4 已落 df558ff 并移除「待做」（39e958ce 后移 2 行，现行 :45，见审核发现 :277）〕
- [x] 审核发现：决策报告的 28 项待办行号对照仍使用整编前行号，与同报告更新后的来源及当前待办不一致（docs/review/2026-10-30-decisions.md:609）——属实，已修〔证据：本 commit；:609 映射改 01→19…12→93…26→55/56→60，对齐正文 :625-751 及当前 todo.md〕
- [x] 审核发现：本轮 diff 未见问题（09043f37..HEAD）——复核成立，无需改动〔证据：本 commit；区间仅 a55eb2f9 纯 docs CSS（.vitepress/theme/custom.css 5 行），无代码/测试改动〕
- [ ] 审核发现：listCloudOperations 的账号过滤恒为真——cloudOperationKey(accountID, op.ID) 直接用入参 accountID 拼键再 HasPrefix(accountID+"/")，等于对自身前缀判真，返回全账号操作，跨账号泄露（含实例原生 ID/厂商请求 ID）（internal/server/api_cloud.go:407）
- [ ] 审核发现：handleCloudInstances 发现同步失败与 provider 构造失败均被静默吞掉（serr/perr 空 if 块），无任何日志或审计，厂商 API 故障与凭据未配置对运维不可见（internal/server/api_cloud.go:483-487）
- [ ] 审核发现：GET /accounts/{id}/instances 每次读取都同步调用厂商 ListInstances（含 15s 超时预算）且无缓存/TTL，读路径被网络写放大，页面刷新可长时间阻塞（internal/server/api_cloud.go:483）
- [ ] 审核发现：handleCloudOperation 无论列操作（OperationID 为空）还是取终态，都先构造 provider（读 vault，可 503），而这两条路径根本不需 provider，构造结果在列表分支被丢弃（internal/server/api_cloud.go:349-369）
- [ ] 审核发现：创建实例的前端/后端均把 security_group_id 标为可选，但 aliyun/tencent 适配器都强制要求非空，用户留空会拿到无明细的 502「Failed to call cloud provider」（web/src/pages/Resources/CloudInstancesPage.tsx:166、internal/server/api_cloud.go:220）
- [ ] 审核发现：账号创建不校验 region（validate() 只查 vendor/alias/secret），但适配器要求 region 非空（aliyun.New 空 region 返回 nil），缺省账号的生命周期操作走默认分支回 500「Failed to load cloud provider」，报错误导（internal/server/api_cloud.go:74-86、156）
- [ ] 审核发现：op.ID 被覆写为「厂商ID-时间戳」，违背 cloud.NewOperation 声明的确定性 ID 契约（同一次云操作保持同一 ID），且时间戳与厂商请求 ID 无关，仅靠纳秒唯一（internal/server/api_cloud.go:307、335）
- [ ] 审核发现：callCloudLifecycleAPI 先在一次性的丢弃 mux 上注册 handler（s.registerCloudAPI(http.NewServeMux())），属死代码，随后才在真正 mux 上二次注册（internal/server/api_cloud_lifecycle_test.go:101）
- [ ] 审核发现：TestCloudDiscoverySync 依赖 instances[0]/[1] 顺序，而 ListCloudInstances 按 created_at 排序，同批写入若时间戳相同则顺序未定义，存在偶发断言失败风险（internal/server/api_cloud_lifecycle_test.go:128）
- [ ] 审核发现：操作列表过滤使用请求账号重新拼接键而非检查实际存储键，导致查询任一账号时返回所有账号的操作记录（internal/server/api_cloud.go:407）。
- [ ] 审核发现：创建请求显式选择按量计费时未被 switch 接受，页面选择「按量」会收到 400，只有不传计费模式才能创建按量实例（internal/server/api_cloud.go:315）。
- [ ] 审核发现：创建允许指定不同于账号默认地域的地域，但发现、生命周期操作和轮询仍使用账号默认地域，导致跨地域创建的实例无法被发现或正确跟踪（internal/server/api_cloud.go:325）。
- [ ] 审核发现：同步保留云端已不存在的实例且不更新状态，释放成功后页面仍将其显示为原状态和计费状态并允许继续操作（internal/server/api_cloud.go:180）。
- [ ] 审核发现：页面收到异步操作后只立即刷新一次且从不轮询操作，云端稍后完成或失败时用户看不到最终结果和更新后的状态（web/src/pages/Resources/CloudInstancesPage.tsx:43）。
- [ ] 审核发现：云操作仅保存在进程内存中，服务重启后已受理操作的查询返回 404，无法继续跟踪云端仍在执行的创建或释放（internal/server/api_cloud.go:386）。
- [ ] 审核发现：构造云适配器未检查账号的 enabled 状态，已禁用的云账号仍会被发现并可发起创建、启停和释放操作（internal/server/api_cloud.go:140）。

- [x] 审核发现：OOB 设计（中英）`todo.md:55` 引用在 HEAD 失效——现指向「带外管理 OOB」章节标题，OpenWrt 片D 条目实际在 todo.md:57；根因=本区间 39e958ce 在 todo.md:20/:22 插入两行复核子注，使 :23 起行号整体下移 2，把 638e6c4b 刚对齐的引用与 todo.md:255 自查「todo.md:55=OpenWrt 片D 条目」双双推歪（docs/design/oob-openwrt-bootstrap.md:10，同 :67-70；docs/zh/design/oob-openwrt-bootstrap.md:10、:57-59）——属实，已修〔证据：本 commit；两份设计文档 7 处 `todo.md:55` 全改 :57，逐行核对 7 处均指 OpenWrt 片D；todo.md:255 历史证据补「现行 :57」防误读〕
- [x] 审核发现：多云评审 `todo.md:43` 引用在 HEAD 失效——现指向「多云扩展」章节标题，P0 云账号条目实际在 todo.md:45（同因 39e958ce 行号下移 2），todo.md:256「改 todo.md:43」结论在 HEAD 不成立（docs/review/chatgpt-2026-10-20-multicloud-positioning.md:214，同 :216、:219）——属实，已修〔证据：本 commit；评审 :214/:216/:219 三处 `todo.md:43` 全改 :45，逐行核对均指多云 P0 条目；todo.md:256 历史证据补「现行 :45」防误读〕
- [x] 审核发现：决策报告 :609 映射「20→43…26→55/56→60」在 HEAD 差 2 行——20→:43 现指多云章节标题（P0 条目 :45）、26→:55 现指 OOB 章节标题（片D 条目 :57）、28→:60 现指虚拟化章节标题（libvirt 条目 :62），正文 :363/:380/:397/:416/:433/:450/:469/:486/:503 与 :649-667/:709-714/:743-751 各表对 items 20-28 同步失效；「对齐正文 :625-751 及当前 todo.md」自查在 HEAD 不成立（docs/review/2026-10-30-decisions.md:609）——属实，已修〔证据：本 commit；:609 映射全量重算为 02→21…28→62（01→19 不变），正文 9 个「来源」行与 28 处表格 `todo.md:` 引用同步 +2，逐项核对均指目标条目〕
- [x] 审核发现：OOB 评审文档 `todo.md:49` 引用残余失效——仍指向「竞品功能强化」章节标题而非 OOB 条目（片D 实际在 todo.md:57）；本轮声称修复「OOB 评审」行号但未达该文件（docs/review/oob-management-2026-10-20.md:88）——属实，已修〔证据：本 commit；评审 :88 `todo.md:49` 改 :51，逐行核对 :51=OOB P0 条目〕

## 源码分层分析 2026-11-07

### 抽象层清单

- L0 进程外壳：解析命令与配置探测，薄壳启动 Server/Agent/探针，不拥有运行期服务。（cmd/cockpit/main.go:22）
- L1 传输契约：统一 Server-Agent WebSocket 帧类型、注册握手、JSON 编解码和 ID/proxy 关联。（internal/protocol/message.go:8）
- L2 Server 运行时组合：组装依赖，管理 context、后台任务、中间件链、子系统启停和 RPC 关联。（internal/server/server.go:126）
- L3 控制 API 与会话策略：认证 REST 请求，分配远控票据/会话，转发资源操作或流式数据面控制。（internal/server/api.go:31）
- L4 编排与调度：先落 Job/Workflow/探测意图，再后台派发、重试、推进和周期扫描。（internal/server/api_workflow.go:484）
- L5 持久化与秘密保管：SQLite 拥有业务模型和迁移；Vault 独立解锁并加密凭据行。（internal/storage/storage.go:40）
- L6 执行 Provider：Agent 按探测能力装配本地/本侧可达执行器，并把 RPC 动作路由到 Provider。（internal/agent/rpc/router.go:70）
- L7 集成与通知出口：Server 用全局凭据/账号适配云、DNS/ACME、overlay、第三方系统和多渠道通知。（internal/notification/service.go:68）
- L8 接口交付与客户端状态：同进程交付 SPA/API/WebSocket；Web/mobile 只保留本地偏好、token 和会话 UI 状态。（web/src/App.tsx:533）

### 超大文件候选

- [ ] LAY-001 [P1] 拆分 docs/guide/concepts.md（245 行）——新增 glossary 后概念、状态、能力与差异说明混在单页，可按“运行态/声明源/安全”拆分。（docs/guide/concepts.md:1）
- [ ] LAY-002 [P1] 拆分 web/src/types/index.ts（1712 行）——云、远控、备份、Workflow、设置等全部领域类型集中，页面局部修改会被迫触碰共享巨型文件。（web/src/types/index.ts:1）
- [ ] LAY-003 [P1] 拆分 web/src/services/api.ts（1570 行）——单个 ApiService 汇聚全部 REST 资源与两种流式协议，领域 API 可按资源边界拆分。（web/src/services/api.ts:122）
- [ ] LAY-004 [P1] 拆分 web/src/components/FileBrowser/index.tsx（1239 行）——远端浏览、操作、传输和 UI 布局同文件，组件接口与副作用耦合。（web/src/components/FileBrowser/index.tsx:1）
- [ ] LAY-005 [P1] 拆分 scripts/acceptance/guac/probe/main.go（1171 行）——验收主流程、协议构造、会话断言和输出聚合集中，场景扩展会继续膨胀。（scripts/acceptance/guac/probe/main.go:1）
- [ ] LAY-006 [P1] 拆分 internal/server/api.go（1166 行）——中央分发器混合认证辅助、用户/资源 handler 和大量私有 view 类型。（internal/server/api.go:31）
- [ ] LAY-007 [P1] 拆分 scripts/acceptance/agent-backup/probe/main.go（1157 行）——备份验收的状态机、远端探测与报告输出耦合在 main 包单文件。（scripts/acceptance/agent-backup/probe/main.go:1）
- [ ] LAY-008 [P1] 拆分 internal/server/api_workflow.go（1090 行）——HTTP 解码、定义 CRUD、编排状态机、变量替换和结果快照同文件。（internal/server/api_workflow.go:484）
- [ ] LAY-009 [P1] 拆分 scripts/acceptance/workflows/probe/main.go（1022 行）——Workflow 验收的步骤、审批、取消和报告逻辑集中，新增用例会进一步扩张入口。（scripts/acceptance/workflows/probe/main.go:1）

测试文件与验收 probe 未列入拆分候选之外的“生产路径优先级”，但上表只保留非 `_test.go`、非生成文件。

### 目录/分类问题

- `docs/guide/concepts.md` 新增源码术语表后，既是入门概念页又是架构证据索引；可后续拆出 `docs/guide/source-layers.md`，两语言页互链（docs/guide/concepts.md:225）。
- `internal/server/api.go` 名义上是 API 聚合入口，但 `handleAPI` 中央 switch、用户/资源 CRUD、审计辅助和 view 类型混合，按资源拆分后可让 `api.go` 只保留路由分发（internal/server/api.go:31）。
- `web/src/types/index.ts` 与 `web/src/services/api.ts` 都是按“通用前端类型/API”归类，不是按领域；建议拆 `types/remote`、`types/cloud`、`services/workflows` 等已有模式（web/src/types/index.ts:1）。
- `internal/server/api_workflow.go` 承载 REST 与编排引擎两种职责，目录分类只反映 API 面；状态机可独立成 `workflow/engine.go`（internal/server/api_workflow.go:582）。
