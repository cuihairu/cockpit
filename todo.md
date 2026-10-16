# Cockpit Todo

需求清单：`- [ ]` 待办、`- [x]` 已完成（一行证据：commit + 测试/验收规模）。过程细节查 git log 与 docs/。

执行纪律：

- 单任务单 commit；commit 信息英文简短，不提 AI；push 前 `git pull --rebase`。
- 先落设计文档再动码；测试与构建绿才 push。
- 不打版本号 tag、不发 Release；版本号由用户手动给出。
- 密钥不进仓库；解锁口令与明文凭据不落日志、不进错误消息；yaml 密钥字段只用 `${VAR}`。
- 真机验收不代测：等硬件/凭据到位按 `docs/guide/acceptance-checklist.md` 推进。

## 待办

> 2026-10-16 复核（PATROL1016）：19 条解锁条件逐条复核仍全部成立，无仓内可开工项；已裁决项不重开。每条尾注〔阻塞｜解锁〕。

### 真机验收（等硬件/真实环境/凭据）

- [ ] service-health 生产接入：docker VM agent 已装但停用，改 SERVER_URL 指生产后 enable〔阻塞=宿主 docker VM 的配置动作，非仓内改动｜解锁=用户在生产 VM 改 SERVER_URL 并 enable〕
- [ ] Windows SCM 服务管理：列表/启停/自启/无权限透传〔阻塞=缺 Windows 主机｜解锁=真机到位按 checklist:246 验收〕
- [ ] macOS launchd 服务管理：列表/启停/自启/kickstart 回退〔阻塞=缺 macOS 主机｜解锁=真机到位按 checklist:247 验收〕
- [ ] NAS：DSM 6/7、TrueNAS CORE/SCALE、OMV 真机 API；mdadm 降级演练；ZFS/SMB/NFS 主机〔阻塞=缺 NAS 设备与账号｜解锁=真机到位按 checklist:220-224 验收〕
- [ ] 备份云远端：真实 S3/B2、限速、GB 级完整链（server 侧 rclone 需宿主安装）〔阻塞=缺真实 S3/B2 凭据（checklist:48,49 明标不可本机验证）｜解锁=凭据与宿主 rclone 到位〕
- [ ] 录制异地：rclone 真远端推送〔阻塞=缺 rclone 真远端（checklist:58,59,65,66 标阻塞）｜解锁=远端配置到位〕
- [ ] ACME：staging/production 签发、自动续期、签发→部署→浏览器信任链〔阻塞=缺真实 ACME 账号与 DNS 凭据｜解锁=凭据到位按 checklist:108-114 验收〕
- [ ] DDNS：真 Cloudflare token、家庭侧 agent 出口 IP 多源〔阻塞=缺真实 Cloudflare token｜解锁=token 到位按 checklist:120-123 验收〕
- [ ] overlay：真 ZT/TS token 授权/除名、join/leave、systemctl 单元态、第二台主机合并观测〔阻塞=缺真实 ZT/TS token 与第二台主机｜解锁=token+主机到位按 checklist:163-165 验收〕
- [ ] SMART：带 attr 表主机读数、FAILED 端到端翻转〔阻塞=缺带 SMART 属性表的真实磁盘｜解锁=真机到位按 checklist:181 验收〕
- [ ] DNS 真账号：DNSPod/阿里云 SRV 与 TTL 下限、台账联动全链、M4 跨 provider 迁移演练〔阻塞=缺真实 DNSPod/阿里云账号｜解锁=账号到位按 checklist:130-132 验收〕
- [ ] Guacamole 音频真机：RDP 音频 mimetype 与 L8/L16 编解码确认（D4，静音按钮已实现）〔阻塞=缺 Windows RDP 主机｜解锁=真机确认编解码〕
- [ ] 移动端：Android release 安装/splash/R8/签名升级、生物识别、ntfy 送达、SSH 终端〔阻塞=缺 Android 真机与签名/ntfy 凭据｜解锁=真机+凭据到位按 checklist 移动端一节（:343 起）验收〕
- [ ] web 移动端触屏与 Workbench 触屏交互〔阻塞=缺真机触屏设备；地基已随 web 移动适配交付（1076 用例），仓内无代码缺口｜解锁=触屏真机验收〕
- [ ] iOS 签名与装机（低优先级；编译验证已在 CI）〔阻塞=缺 Apple 开发者账号与 Mac｜解锁=凭据到位装机验收〕

### 功能

- [ ] 防火墙写操作（规则增删）：未立项，等需求〔已裁决搁置：decision-log D-2026-10-07-3 否决直接做写，前置=只读真机验收闭环（未完成）｜解锁=只读真机验收后重开写操作评估〕
- [ ] workflow 并行分支执行：等真实需求再立项〔已裁决搁置：workflow-design W4/G3 压后｜解锁=真实并行需求出现再立项〕
- [ ] vault 硬验收：真 persona 服务端实测 provider 切换（单测已证切换不破坏 local 存量，cc51840）〔外部阻塞=缺 persona 服务端地址与 token｜解锁=用户提供地址+凭据〕
- [ ] jenkins/external endpoint 凭据 vault 引用：随链接功能立项〔待立项=无独立开工条件｜解锁=链接功能立项〕

### 文档

- [ ] archive 5 篇是否双语：历史归档（plans 2 / specs 2 / 归档说明 1）当前仅中文，是否补英文版待用户拍板〔未决=需用户裁定归档页是否纳入双语口径｜解锁=用户拍板要/不要；不要则在本行注明口径已定并关闭〕
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
