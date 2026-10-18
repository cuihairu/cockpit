[0m
[0m$ [0mpwd && git status --short && rg --files -g '!node_modules' -g '!vendor' | sed -n '1,240p'
/home/cui/workspaces/cockpit
examples/inventory.yaml
todo.md
go.mod
cmd/cockpit-probe-agent/main_test.go
cmd/cockpit-probe-agent/main.go
cmd/cockpit-agent/cov_svcargs_test.go
cmd/cockpit-agent/cov_service_test.go
cmd/cockpit-agent/svcargs.go
cmd/cockpit-agent/service_other.go
cmd/cockpit-agent/rsrc_windows_arm64.syso
cmd/cockpit-agent/rsrc_windows_amd64.syso
cmd/cockpit-agent/cov_runservice_test.go
cmd/cockpit-agent/cov_main_test.go
cmd/cockpit-agent/main.go
cmd/cockpit-agent/cov_main_subproc_test.go
cmd/cockpit-agent/service_windows.go
cmd/cockpit/cov_main_test.go
cmd/cockpit/main.go
cmd/cockpit/cov_main_subproc_test.go
install.ps1
docs/remote-access-integration-design.md
docs/agent-id.en.md
docs/audit-doc-consistency.en.md
docs/index.md
docs/remote-desktop-guacamole-design.en.md
dev.sh
docs/ui-theme-design.en.md
docs/agent-id.md
docs/pnpm-lock.yaml
docs/remote-desktop-guacamole-design.md
docs/审计-文档一致性.md
docs/guacd-deploy.md
docs/agent-config.md
packaging/windows/agent.iss
packaging/windows/agent.ico
LICENSE
Dockerfile
README.md
docker-compose.yml
tool/known_uncoverable.txt
tool/coverage_check.sh
docs/index.en.md
docs/pnpm-workspace.yaml
docs/websocket-token-auth.en.md
internal/audit/entry_test.go
internal/audit/logger_test.go
internal/audit/logger.go
internal/audit/cov_logger_test.go
core/backoff/backoff_test.go
core/backoff/backoff.go
docs/guide/nas-design.en.md
internal/overlay/zerotier.go
internal/overlay/cov_cloud_test.go
docs/guide/proxy-design.en.md
internal/overlay/overlay_test.go
docs/guide/overlay-design.md
internal/overlay/tailscale.go
docs/guide/cron-design.md
docs/guide/windows-agent-design.en.md
docs/guide/stack-deploy-design.en.md
docs/guide/stack-deploy-design.md
docs/guide/getting-started.md
core/execlog/execlog_test.go
docs/guide/architecture.md
docs/guide/service-health-design.en.md
docs/guide/server-backup-design.md
docs/guide/protocol.md
docs/guide/vault-design.en.md
core/execlog/execlog.go
docs/operations/deploy-docker.en.md
docs/operations/deploy-docker.md
docs/package.json
docs/guacd-deploy.en.md
docs/remote-access-integration-design.en.md
core/healthprobe/healthprobe.go
core/healthprobe/window.go
core/healthprobe/semantics.go
core/healthprobe/state.go
core/healthprobe/state_test.go
core/healthprobe/healthprobe_test.go
scripts/local-acceptance/fake-docker-daemon.go
core/metrics/collector.go
core/metrics/collector_test.go
scripts/local-acceptance/README.md
scripts/local-acceptance/run-acceptance.sh
scripts/local-acceptance/fake-docker
scripts/e2e-smoke.ps1
scripts/package-ipk.sh
docs/guide/agent-egress-sdwan.md
web/tsconfig.json
docs/guide/recording-design.md
docs/guide/service-design.en.md
docs/guide/acme-design.en.md
docs/guide/domain-binding-design.md
docs/guide/backup-design.md
docs/guide/logs-design.md
docs/guide/acme-design.md
docs/guide/service-health-design.md
docs/guide/probe-enhance-design.md
docs/guide/cron-design.en.md
docs/guide/acceptance-checklist.en.md
docs/guide/overlay-design.en.md
docs/guide/disk-health-design.en.md
docs/guide/drift-design.en.md
docs/guide/mobile-design.md
docs/guide/persona-vault-mapping.en.md
docs/guide/firewall-design.en.md
docs/guide/file-manager-design.md
docs/guide/jobs-design.en.md
docs/guide/ddns-design.en.md
docs/guide/dns-design.en.md
docs/guide/reference-projects.md
docs/guide/workflow-design.md
docs/guide/acceptance-checklist.md
docs/guide/disk-health-design.md
docs/guide/agent-egress-sdwan.en.md
docs/guide/architecture.en.md
docs/guide/decision-log.en.md
docs/guide/getting-started.en.md
docs/guide/openwrt-agent-design.md
docs/guide/concepts.en.md
docs/guide/drift-design.md
docs/guide/windows-agent-design.md
docs/archive/specs/2026-05-26-totp-2fa-design.md
docs/archive/specs/2026-05-27-notification-module-design.md
docs/guide/rbac-design.en.md
docs/archive/README.md
docs/research/ops-oss-survey.md
docs/research/ops-oss-survey.en.md
docs/guide/protocol.en.md
docs/guide/decision-log.md
docs/guide/service-design.md
docs/guide/concepts.md
docs/guide/backup-design.en.md
docs/guide/probe-enhance-design.en.md
docs/guide/openwrt-agent-design.en.md
docs/guide/logs-design.en.md
docs/guide/ddns-design.md
docs/guide/mobile-design.en.md
docs/guide/firewall-design.md
docs/guide/vault-design.md
docs/guide/dns-design.md
docs/guide/domain-binding-design.en.md
docs/guide/reference-projects.en.md
docs/guide/proxy-design.md
docs/guide/jobs-design.md
docs/guide/persona-vault-mapping.md
docs/guide/introduction.md
docs/guide/nas-design.md
docs/guide/introduction.en.md
docs/guide/server-backup-design.en.md
docs/guide/file-manager-design.en.md
docs/guide/recording-design.en.md
docs/guide/rbac-design.md
docs/guide/workflow-design.en.md
docs/research/assets/netdata-overview.png
docs/research/assets/prometheus-architecture.svg
docs/research/assets/grafana-dashboard.png
docs/archive/plans/2026-05-26-totp-2fa-implementation.md
docs/archive/plans/2026-05-27-notification-module.md
docs/websocket-token-auth.md
docs/ui-theme-design.md
docs/agent-config.en.md
README.zh.md
docs/research/assets/zabbix-network-map.png
docs/research/assets/1panel-overview.png
docs/research/assets/coolify-deploy-db.webp
package.json
config/cockpit.yaml
docs/research/assets/netdata-charts.png
core/platform/other/signals.go
core/platform/other/machineid.go
core/platform/other/paths.go
core/platform/host_test.go
web/tool/known_uncoverable.txt
web/tool/coverage_check.sh
web/tsconfig.node.json
docs/research/assets/coolify-deploy-app.webp
docs/research/assets/zabbix-dashboards.png
docs/research/assets/beszel-hub-system.png
docs/research/assets/beszel-dashboard.png
core/platform/darwin/launchd.go
core/platform/darwin/signals.go
core/platform/darwin/machineid.go
core/platform/darwin/paths.go
core/platform/service.go
core/platform/select_linux.go
core/platform/host.go
core/platform/select_windows.go
docs/public/logo.svg
scripts/acceptance/ddns/probe-scan.sh
scripts/acceptance/gap_report.py
docs/public/logo.png
scripts/acceptance/services/run-server.sh
scripts/acceptance/services/teardown-env.sh
web/public/logo.svg
core/platform/linux/machineid_test.go
core/platform/linux/signals.go
core/platform/linux/paths_test.go
core/platform/linux/machineid.go
core/platform/linux/paths.go
web/vite.config.ts
web/pnpm-workspace.yaml
web/vitest.config.ts
core/platform/select_darwin.go
web/pnpm-lock.yaml
scripts/acceptance/services/stop-server.sh
scripts/acceptance/services/setup-env.sh
docs/public/screenshots/dashboard.png
docs/public/screenshots/agents-cleanup.png
docs/public/screenshots/dns.png
docs/public/screenshots/workbench.png
docs/public/screenshots/audit.png
docs/public/README.md
docs/design/bmc-capabilities.md
docs/design/bmc-capabilities.en.md
docs/design/external-session-api.en.md
docs/design/service-detect-agent.md
docs/design/hypervisor-capabilities.en.md
docs/design/agent-core-architecture.en.md
docs/design/agent-core-architecture.md
docs/design/service-detect-agent.en.md
docs/design/external-session-api.md
docs/design/hypervisor-capabilities.md
scripts/acceptance/notify/teardown-env.sh
scripts/acceptance/notify/run-server.sh
scripts/acceptance/notify/setup-env.sh
core/platform/windows/signals.go
scripts/acceptance/notify/stop-server.sh
core/platform/windows/machineid.go
core/platform/windows/paths.go
core/platform/windows/scm.go
core/platform/select_other.go
scripts/acceptance/workflows/stop-server.sh
scripts/acceptance/workflows/setup-env.sh
scripts/acceptance/guac/start-rdp.sh
scripts/acceptance/services/probe/main_test.go
scripts/acceptance/services/probe/main.go
scripts/acceptance/workflows/teardown-env.sh
scripts/acceptance/workflows/run-server.sh
[0m$ [0mcat todo.md
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
[0m$ [0mcat go.mod && cat package.json && cat web/package.json && cat README.md
module github.com/cuihairu/cockpit

go 1.27

require (
	github.com/fsnotify/fsnotify v1.10.1
	github.com/go-acme/lego/v4 v4.35.2
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/moby/moby/api v1.56.1
	github.com/moby/moby/client v0.6.2
	github.com/pquerna/otp v1.5.0
	github.com/shirou/gopsutil/v3 v3.24.5
	golang.org/x/crypto v0.58.0
	golang.org/x/sys v0.49.0
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.2
)

require (
	github.com/Microsoft/go-winio v0.6.3-0.20251027160822-ad3df93bed29 // indirect
	github.com/alibabacloud-go/alibabacloud-gateway-spi v0.0.5 // indirect
	github.com/alibabacloud-go/darabonba-openapi/v2 v2.1.16 // indirect
	github.com/alibabacloud-go/debug v1.0.1 // indirect
	github.com/alibabacloud-go/tea v1.4.0 // indirect
	github.com/alibabacloud-go/tea-utils/v2 v2.0.9 // indirect
	github.com/aliyun/credentials-go v1.4.7 // indirect
	github.com/boombuler/barcode v1.0.1 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clbanning/mxj/v2 v2.7.0 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/go-connections v0.8.2 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/go-acme/alidns-20150109/v4 v4.7.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/json-iterator/go v1.1.13-0.20220915233716-71ac16282d12 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-sqlite3 v1.14.44 // indirect
	github.com/miekg/dns v1.1.72 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/nrdcg/dnspod-go v0.4.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.71.0 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/tools v0.51.0 // indirect
	gopkg.in/ini.v1 v1.67.1 // indirect
	pgregory.net/rapid v1.3.0 // indirect
)
{
	"devDependencies": {
		"@types/qrcode": "^1.5.6"
	}
}
{
  "name": "cockpit-dashboard",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview",
    "lint": "eslint src",
    "test": "vitest run",
    "test:coverage": "vitest run --coverage"
  },
  "dependencies": {
    "@ant-design/icons": "^6.3.4",
    "@ant-design/pro-components": "^2.8.10",
    "@ant-design/pro-layout": "^7.22.7",
    "@ant-design/v5-patch-for-react-19": "^1.0.3",
    "@novnc/novnc": "^1.7.0",
    "@tanstack/react-query": "^5.104.1",
    "@xterm/addon-fit": "^0.11.0",
    "@xterm/addon-web-links": "^0.12.0",
    "@xterm/xterm": "^6.0.0",
    "antd": "^5.29.3",
    "axios": "^1.20.0",
    "dayjs": "^1.11.23",
    "echarts": "^6.1.0",
    "guacamole-common-js": "1.5.0",
    "qrcode": "^1.5.4",
    "react": "^19.3.0",
    "react-dom": "^19.3.0",
    "react-router-dom": "^7.18.4"
  },
  "devDependencies": {
    "@testing-library/jest-dom": "^7.0.1",
    "@testing-library/react": "^16.3.3",
    "@testing-library/user-event": "^14.6.7",
    "@types/qrcode": "^1.5.6",
    "@types/react": "^19.3.0",
    "@types/react-dom": "^19.3.0",
    "@typescript-eslint/eslint-plugin": "^8.63.0",
    "@typescript-eslint/parser": "^8.71.1",
    "@vitejs/plugin-react": "^6.1.2",
    "@vitest/coverage-v8": "^5.0.1",
    "eslint": "^10.12.0",
    "eslint-plugin-react-hooks": "^7.1.1",
    "eslint-plugin-react-refresh": "^0.5.7",
    "jsdom": "^30.1.1",
    "less": "^4.9.1",
    "typescript": "~6.0.3",
    "vite": "^8.3.4",
    "vitest": "^5.0.1"
  }
}
[English](README.md) | [中文](README.zh.md)

<p align="center">
  <img src="web/public/logo.svg" alt="Cockpit Logo" width="64" height="64">
</p>

<h1 align="center">Cockpit</h1>

A personal hybrid infrastructure console that consolidates resources scattered across local machine rooms, cloud VPS instances, and nodes behind NAT into a single lightweight Server + Agent control plane.

[![Test](https://img.shields.io/github/actions/workflow/status/cuihairu/cockpit/test.yml?branch=main&logo=github&label=Test)](https://github.com/cuihairu/cockpit/actions/workflows/test.yml)
[![Docs](https://img.shields.io/github/actions/workflow/status/cuihairu/cockpit/docs.yml?branch=main&logo=github&label=Docs)](https://github.com/cuihairu/cockpit/actions/workflows/docs.yml)
[![codecov](https://codecov.io/gh/cuihairu/cockpit/branch/main/graph/badge.svg)](https://codecov.io/gh/cuihairu/cockpit)
[![Agent: Windows](https://img.shields.io/badge/platform-Windows-0078D4?logo=windows&logoColor=white)](https://cuihairu.github.io/cockpit/guide/getting-started)
[![Agent: Linux](https://img.shields.io/badge/platform-Linux-FCC624?logo=linux&logoColor=black)](https://cuihairu.github.io/cockpit/guide/getting-started)
[![Agent: macOS](https://img.shields.io/badge/platform-macOS-000000?logo=apple&logoColor=white)](https://cuihairu.github.io/cockpit/guide/getting-started)
[![Agent: x86_64](https://img.shields.io/badge/arch-x86__64-5B5B5B)](https://cuihairu.github.io/cockpit/guide/getting-started)
[![Agent: aarch64](https://img.shields.io/badge/arch-aarch64-5B5B5B)](https://cuihairu.github.io/cockpit/guide/getting-started)

## Features

The console brings resources, execution, and remote access together in one place. On the resource side, a unified inventory view is synced from Inventory YAML; probe heartbeats, expiration alerts, and configuration drift detection all run on this data, with alerts delivered through four channels: Herald, ntfy, webhook, and Telegram. On the execution side, a fleet-wide Job ledger dispatches commands to any online agent with trackable status, exit codes, and captured output; Docker containers get full lifecycle management, and applications deploy as Stacks from compose files. On the remote access side, terminal, VNC, and desktop sessions go through short-lived ticket forwarding with session recording; reverse proxy sites, ACME certificates, DNS/DDNS, backup and restore, and service/Cron/SMART/NAS/network observability are all operated from the same console.

Agents register by dialing out over WebSocket, so nodes behind NAT never need to expose any inbound ports.

## Quick Start

> Prerequisite: a running Cockpit Server first (a single Docker command; the deploy host needs no build toolchain):

```bash
cp deployments/docker/.env.example deployments/docker/.env && vi deployments/docker/.env
docker compose -f deployments/docker/docker-compose.yml up -d
```

### Linux

1. Download and install (architecture auto-detected, fetched anonymously from daily builds, verified automatically after install):

   ```bash
   curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/install.sh | bash
   ```

2. Connect to the server (the example uses this site's domain; replace it with your own Cockpit server address):

   ```bash
   cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
   ```

3. Verify the installation:

   ```bash
   cockpit-agent --version
   ```

   Expected output on success:

   ```text
   Cockpit Agent v20261001
   ```

   (The version number is the date of installation; `Registered as agent: ...` in the agent log means the server connection is up.)

### macOS

1. Download and install (same `install.sh` as Linux; detects Darwin and arm64/amd64 automatically):

   ```bash
   curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/install.sh | bash
   ```

2. Connect to the server:

   ```bash
   cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
   ```

3. Verify the installation:

   ```bash
   cockpit-agent --version
   ```

   Expected output on success:

   ```text
   Cockpit Agent v20261001
   ```

### Windows

**Graphical installer (recommended)**: download `cockpit-agent-setup-nightly.exe` from the [nightly release](https://github.com/cuihairu/cockpit/releases/latest) (the identically named `.sha256` file is for verification) and run it by double-click.
It installs to `%ProgramFiles%\Cockpit Agent`, creates Start menu and desktop shortcuts (with the agent icon), asks for the Server address in the wizard, offers an optional "Register Windows service with auto-start on boot" checkbox, and registers a standard uninstall entry in Control Panel. For silent installation:

```powershell
.\cockpit-agent-setup-nightly.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SERVER=wss://cockpit.cuihairu.site/ws
```

Service management: `cockpit-agent service install|uninstall|start|stop|status` (service name
`CockpitAgent`, log at `%ProgramData%\CockpitAgent\agent.log`).

**PowerShell script path** (no administrator required; installs into the current user's directory and adds it to the user PATH):

1. Download and install:

   ```powershell
   irm https://raw.githubusercontent.com/cuihairu/cockpit/main/install.ps1 | iex
   ```

2. Connect to the server (open a new terminal so the PATH takes effect):

   ```powershell
   cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
   ```

3. Verify the installation:

   ```powershell
   cockpit-agent --version
   ```

   Expected output on success:

   ```text
   Cockpit Agent v20261001
   ```


## Local Testing Conventions

CI and local development split into three paths based on what changed — a one-line docs edit should not trigger a full build.

1. **Docs changes** (`*.md`, `docs/**`, images): no build. The main CI workflow's
   `paths-ignore` already skips these paths.
2. **Code changes**: run only the changed subset, never the full suite.
   - `go test ./internal/agent/<changed-package>/...`
   - `go test ./cmd/cockpit-agent/...`
3. **Full suite** (`go test ./internal/agent/... ./cmd/cockpit-agent/...`): only triggered by go.mod / go.sum,
   `.github/**`, Docker/Compose config changes, or before a release.

## Documentation

Full installation details (one-line installer internals / platform matrix / building from source / OpenWrt ipk), configuration reference, architecture, and per-feature design docs live on the documentation site:

- [Introduction](https://cuihairu.github.io/cockpit/guide/introduction)
- [Quick Start](https://cuihairu.github.io/cockpit/guide/getting-started)
- [Architecture and Boundaries](https://cuihairu.github.io/cockpit/guide/architecture)
- [Protocol and API Boundaries](https://cuihairu.github.io/cockpit/guide/protocol)
- [Docker Deployment](https://cuihairu.github.io/cockpit/operations/deploy-docker)
- [Open-Source Ops Panels Survey](https://cuihairu.github.io/cockpit/research/ops-oss-survey)
- Agent manual deployment and service unit examples: [deployments/README.md](deployments/README.md)

## Foundations

This repository is built on open-source components; what is written here is the control plane wiring and the provider layers: Server and Agent are in Go, storage goes through GORM into SQLite, and the realtime channel runs on gorilla/websocket; the web frontend is React 19 + Ant Design with xterm.js for terminal rendering; desktop and VNC/SSH remote access are backed by Apache Guacamole (guacd server + guacamole-common-js 1.5.0 client), and the built-in terminal's SSH uses golang.org/x/crypto/ssh; certificate issuance uses go-acme/lego (DNS-01), the mobile app is Flutter, the docs site is VitePress, and the Windows installer is packaged with Inno Setup.

## License

Apache License 2.0
[0m$ [0mfind . -path './.git' -prune -o -path './node_modules' -prune -o -type f -print | wc -l && find web/src -type f | sort && find internal core cmd -type f | sort
9491
web/src/App.less
web/src/App.test.tsx
web/src/App.tsx
web/src/assets/.gitkeep
web/src/assets/logo.png
web/src/assets/logo.svg
web/src/components/BackupCodesDisplay/index.less
web/src/components/BackupCodesDisplay/index.test.tsx
web/src/components/BackupCodesDisplay/index.tsx
web/src/components/BrandLogo/index.tsx
web/src/components/CleanupOfflineButton.test.tsx
web/src/components/CleanupOfflineButton.tsx
web/src/components/ErrorBoundary/index.test.tsx
web/src/components/ErrorBoundary/index.tsx
web/src/components/FileBrowser/index.test.tsx
web/src/components/FileBrowser/index.tsx
web/src/components/GuacamoleModal/index.test.tsx
web/src/components/GuacamoleModal/index.tsx
web/src/components/GuacPlayer/index.test.tsx
web/src/components/GuacPlayer/index.tsx
web/src/components/HeartbeatBar/index.test.tsx
web/src/components/HeartbeatBar/index.tsx
web/src/components/HeartbeatBar/ProbeHeartbeatCell.test.tsx
web/src/components/HeartbeatBar/ProbeHeartbeatCell.tsx
web/src/components/MetricsChart/index.test.tsx
web/src/components/MetricsChart/index.tsx
web/src/components/MobileTabBar/index.less
web/src/components/MobileTabBar/index.test.tsx
web/src/components/MobileTabBar/index.tsx
web/src/components/Notifications/index.test.tsx
web/src/components/Notifications/index.tsx
web/src/components/PermGuard.test.tsx
web/src/components/PermGuard.tsx
web/src/components/QRCodeDisplay/index.less
web/src/components/QRCodeDisplay/index.test.tsx
web/src/components/QRCodeDisplay/index.tsx
web/src/components/RemoteServices/index.test.tsx
web/src/components/RemoteServices/index.tsx
web/src/components/RemoteToolbar/index.test.tsx
web/src/components/RemoteToolbar/index.tsx
web/src/components/SystemInfoCard/index.test.tsx
web/src/components/SystemInfoCard/index.tsx
web/src/components/TagManageModal.tsx
web/src/components/TerminalModal/index.test.tsx
web/src/components/TerminalModal/index.tsx
web/src/components/ThemeColorSwatches/index.test.tsx
web/src/components/ThemeColorSwatches/index.tsx
web/src/components/ThemeSkinCards/index.test.tsx
web/src/components/ThemeSkinCards/index.tsx
web/src/components/VaultVerifyModal/index.test.tsx
web/src/components/VaultVerifyModal/index.tsx
web/src/contexts/SettingsContext.test.tsx
web/src/contexts/SettingsContext.tsx
web/src/contexts/settingsContextValue.ts
web/src/contexts/settingsTypes.ts
web/src/contexts/UserContext.test.tsx
web/src/contexts/UserContext.tsx
web/src/contexts/userContextValue.ts
web/src/contexts/userTypes.ts
web/src/contexts/useSettingsContext.ts
web/src/contexts/useUser.ts
web/src/global.d.ts
web/src/hooks/useConnectionTimeout.test.tsx
web/src/hooks/useConnectionTimeout.ts
web/src/hooks/usePerm.ts
web/src/index.css
web/src/main.tsx
web/src/pages/Acme/index.test.tsx
web/src/pages/Acme/index.tsx
web/src/pages/Agents/AgentDetailModal.test.tsx
web/src/pages/Agents/AgentDetailModal.tsx
web/src/pages/Agents/columns.tsx
web/src/pages/Agents/helpers.test.tsx
web/src/pages/Agents/helpers.tsx
web/src/pages/Agents/index.test.tsx
web/src/pages/Agents/index.tsx
web/src/pages/Agents/LabelsDisplay.tsx
web/src/pages/Agents/useRemoteModals.test.tsx
web/src/pages/Agents/useRemoteModals.ts
web/src/pages/Approvals/index.test.tsx
web/src/pages/Approvals/index.tsx
web/src/pages/AuditLogs/index.less
web/src/pages/AuditLogs/index.test.tsx
web/src/pages/AuditLogs/index.tsx
web/src/pages/Backups/index.test.tsx
web/src/pages/Backups/index.tsx
web/src/pages/Backups/ServerBackupCard.tsx
web/src/pages/Cron/index.test.tsx
web/src/pages/Cron/index.tsx
web/src/pages/Dashboard/AgentTable.tsx
web/src/pages/Dashboard/HealthBar.tsx
web/src/pages/Dashboard/index.less
web/src/pages/Dashboard/index.test.tsx
web/src/pages/Dashboard/index.tsx
web/src/pages/Dashboard/StatCards.tsx
web/src/pages/Disk/index.test.tsx
web/src/pages/Disk/index.tsx
web/src/pages/DNS/DDNSPanel.test.tsx
web/src/pages/DNS/DDNSPanel.tsx
web/src/pages/DNS/ImportModal.tsx
web/src/pages/DNS/index.test.tsx
web/src/pages/DNS/index.tsx
web/src/pages/Docker/index.test.tsx
web/src/pages/Docker/index.tsx
web/src/pages/Domains/index.test.tsx
web/src/pages/Domains/index.tsx
web/src/pages/Drift/ConsistencyTab.test.tsx
web/src/pages/Drift/ConsistencyTab.tsx
web/src/pages/Drift/DiffModal.test.tsx
web/src/pages/Drift/DiffModal.tsx
web/src/pages/Drift/index.test.tsx
web/src/pages/Drift/index.tsx
web/src/pages/Drift/shared.ts
web/src/pages/Firewall/index.test.tsx
web/src/pages/Firewall/index.tsx
web/src/pages/ForgotPassword/index.less
web/src/pages/ForgotPassword/index.test.tsx
web/src/pages/ForgotPassword/index.tsx
web/src/pages/Jobs/index.test.tsx
web/src/pages/Jobs/index.tsx
web/src/pages/Login/index.less
web/src/pages/Login/index.test.tsx
web/src/pages/Login/index.tsx
web/src/pages/LogSearch/index.test.tsx
web/src/pages/LogSearch/index.tsx
web/src/pages/Monitor/index.test.tsx
web/src/pages/Monitor/index.tsx
web/src/pages/Nas/index.test.tsx
web/src/pages/Nas/index.tsx
web/src/pages/Network/index.test.tsx
web/src/pages/Network/index.tsx
web/src/pages/Profile/index.test.tsx
web/src/pages/Profile/index.tsx
web/src/pages/Proxy/index.test.tsx
web/src/pages/Proxy/index.tsx
web/src/pages/Recordings/index.test.tsx
web/src/pages/Recordings/index.tsx
web/src/pages/ResetPassword/index.less
web/src/pages/ResetPassword/index.test.tsx
web/src/pages/ResetPassword/index.tsx
web/src/pages/Resources/CertsPage.test.tsx
web/src/pages/Resources/CertsPage.tsx
web/src/pages/Resources/columns.tsx
web/src/pages/Resources/ComputeDetail.test.tsx
web/src/pages/Resources/ComputeDetail.tsx
web/src/pages/Resources/ComputePage.test.tsx
web/src/pages/Resources/ComputePage.tsx
web/src/pages/Resources/DomainsPage.test.tsx
web/src/pages/Resources/DomainsPage.tsx
web/src/pages/Resources/GatewaysPage.test.tsx
web/src/pages/Resources/GatewaysPage.tsx
web/src/pages/Resources/index.test.tsx
web/src/pages/Resources/index.tsx
web/src/pages/Resources/ServicesPage.test.tsx
web/src/pages/Resources/ServicesPage.tsx
web/src/pages/Resources/StoragesPage.test.tsx
web/src/pages/Resources/StoragesPage.tsx
web/src/pages/Resources/subpages.test.tsx
web/src/pages/Resources/useResources.ts
web/src/pages/Roles/index.test.tsx
web/src/pages/Roles/index.tsx
web/src/pages/Services/HealthPanel.test.tsx
web/src/pages/Services/HealthPanel.tsx
web/src/pages/Services/index.test.tsx
web/src/pages/Services/index.tsx
web/src/pages/Services/ProbePanel.test.tsx
web/src/pages/Services/ProbePanel.tsx
web/src/pages/Settings/AlertSettings.tsx
web/src/pages/Settings/authenticatorLinks.ts
web/src/pages/Settings/DisableTOTPModal.tsx
web/src/pages/Settings/GeneralSettings.tsx
web/src/pages/Settings/index.test.tsx
web/src/pages/Settings/index.tsx
web/src/pages/Settings/SecuritySettings.tsx
web/src/pages/Settings/SystemInfo.tsx
web/src/pages/Settings/useSettings.test.tsx
web/src/pages/Settings/useSettings.ts
web/src/pages/SetupTOTP/index.less
web/src/pages/SetupTOTP/index.test.tsx
web/src/pages/SetupTOTP/index.tsx
web/src/pages/Stacks/GitBar.test.tsx
web/src/pages/Stacks/GitBar.tsx
web/src/pages/Stacks/index.test.tsx
web/src/pages/Stacks/index.tsx
web/src/pages/Stacks/shared.test.tsx
web/src/pages/Stacks/shared.tsx
web/src/pages/Stacks/StackDetail.test.tsx
web/src/pages/Stacks/StackDetail.tsx
web/src/pages/Stacks/TerminalBlock.test.tsx
web/src/pages/Stacks/TerminalBlock.tsx
web/src/pages/TOTPVerify/index.less
web/src/pages/TOTPVerify/index.test.tsx
web/src/pages/TOTPVerify/index.tsx
web/src/pages/Users/index.test.tsx
web/src/pages/Users/index.tsx
web/src/pages/Workbench/index.test.tsx
web/src/pages/Workbench/index.tsx
web/src/pages/Workflows/index.test.tsx
web/src/pages/Workflows/index.tsx
web/src/services/api.test.ts
web/src/services/api.ts
web/src/services/approvals.test.ts
web/src/services/approvals.ts
web/src/services/desktop.test.ts
web/src/services/desktop.ts
web/src/services/jobs.test.ts
web/src/services/jobs.ts
web/src/services/metrics.test.ts
web/src/services/metrics.ts
web/src/services/remote.test.ts
web/src/services/remote.ts
web/src/services/workflows.test.ts
web/src/services/workflows.ts
web/src/test/setup.ts
web/src/theme/themePresets.ts
web/src/theme/themeSkins.test.ts
web/src/theme/themeSkins.ts
web/src/types/index.ts
web/src/utils/apiError.test.ts
web/src/utils/apiError.ts
web/src/utils/configHighlight.test.ts
web/src/utils/configHighlight.ts
web/src/utils/format.test.ts
web/src/utils/format.ts
web/src/utils/lineDiff.test.ts
web/src/utils/lineDiff.ts
web/src/utils/logger.test.ts
web/src/utils/logger.ts
web/src/utils/perm.test.ts
web/src/utils/perm.ts
web/src/utils/scancodes.test.ts
web/src/utils/scancodes.ts
web/src/vite-env.d.ts
web/src/workbench/AgentSidebar.test.tsx
web/src/workbench/AgentSidebar.tsx
web/src/workbench/ConnectionPanel.test.tsx
web/src/workbench/ConnectionPanel.tsx
web/src/workbench/LogsPanel.test.tsx
web/src/workbench/LogsPanel.tsx
web/src/workbench/OverviewPanel.tsx
web/src/workbench/services.test.ts
web/src/workbench/services.ts
web/src/workbench/types.ts
cmd/cockpit-agent/cov_main_subproc_test.go
cmd/cockpit-agent/cov_main_test.go
cmd/cockpit-agent/cov_runservice_test.go
cmd/cockpit-agent/cov_service_test.go
cmd/cockpit-agent/cov_svcargs_test.go
cmd/cockpit-agent/main.go
cmd/cockpit-agent/rsrc_windows_amd64.syso
cmd/cockpit-agent/rsrc_windows_arm64.syso
cmd/cockpit-agent/service_other.go
cmd/cockpit-agent/service_windows.go
cmd/cockpit-agent/svcargs.go
cmd/cockpit/cov_main_subproc_test.go
cmd/cockpit/cov_main_test.go
cmd/cockpit/main.go
cmd/cockpit-probe-agent/main.go
cmd/cockpit-probe-agent/main_test.go
core/backoff/backoff.go
core/backoff/backoff_test.go
core/crash/crash.go
core/execlog/execlog.go
core/execlog/execlog_test.go
core/healthprobe/healthprobe.go
core/healthprobe/healthprobe_test.go
core/healthprobe/semantics.go
core/healthprobe/state.go
core/healthprobe/state_test.go
core/healthprobe/window.go
core/heartbeat/heartbeat.go
core/heartbeat/heartbeat_test.go
core/logx/logx.go
core/logx/logx_test.go
core/metrics/collector.go
core/metrics/collector_test.go
core/platform/darwin/launchd.go
core/platform/darwin/machineid.go
core/platform/darwin/paths.go
core/platform/darwin/signals.go
core/platform/host.go
core/platform/host_test.go
core/platform/linux/machineid.go
core/platform/linux/machineid_test.go
core/platform/linux/paths.go
core/platform/linux/paths_test.go
core/platform/linux/signals.go
core/platform/other/machineid.go
core/platform/other/paths.go
core/platform/other/signals.go
core/platform/select_darwin.go
core/platform/select_linux.go
core/platform/select_other.go
core/platform/select_windows.go
core/platform/service.go
core/platform/windows/machineid.go
core/platform/windows/paths.go
core/platform/windows/scm.go
core/platform/windows/signals.go
core/register/register.go
core/register/register_test.go
core/report/upstream.go
core/report/upstream_test.go
internal/agent/agent.go
internal/agent/agent_handler_test.go
internal/agent/agent_test.go
internal/agent/agent_ws_test.go
internal/agent/cov_agent_test.go
internal/agent/cov_coverage_gaps_test.go
internal/agent/cov_gaps2_test.go
internal/agent/cov_pct_test.go
internal/agent/cov_providers_test.go
internal/agent/cov_round10_test.go
internal/agent/cov_round9_test.go
internal/agent/cov_runservice_test.go
internal/agent/cov_startcmd_test.go
internal/agent/cov_start_signal_test.go
internal/agent/cov_vault_nas_test.go
internal/agent/cov_virtualization_test.go
internal/agent/detector/cov_detector_test.go
internal/agent/detector/detector.go
internal/agent/detector/detector_test.go
internal/agent/detector/docker.go
internal/agent/detector/env_test.go
internal/agent/detector/firewall.go
internal/agent/detector/firewall_test.go
internal/agent/detector/hardware.go
internal/agent/detector/openwrt.go
internal/agent/detector/overlay.go
internal/agent/detector/overlay_identity.go
internal/agent/detector/overlay_identity_test.go
internal/agent/detector/overlay_test.go
internal/agent/detector/platform_test.go
internal/agent/detector/pve.go
internal/agent/detector/remote_auth_test.go
internal/agent/detector/remote.go
internal/agent/localip.go
internal/agent/provider/provider.go
internal/agent/provider/provider_test.go
internal/agent/providers.go
internal/agent/providers_test.go
internal/agent/remote/cov_io_gaps_test.go
internal/agent/remote/manager_forward_test.go
internal/agent/remote/manager.go
internal/agent/remote/manager_test.go
internal/agent/rpc/backup_hook_other.go
internal/agent/rpc/backup_hook_test.go
internal/agent/rpc/backup_hook_unix.go
internal/agent/rpc/backup_provider.go
internal/agent/rpc/backup_provider_test.go
internal/agent/rpc/backup_remote_test.go
internal/agent/rpc/cov_backup_provider_test.go
internal/agent/rpc/cov_cron_provider_test.go
internal/agent/rpc/cov_dispatch_test.go
internal/agent/rpc/cov_docker_provider_test.go
internal/agent/rpc/cov_drift_provider_test.go
internal/agent/rpc/cov_file_provider_test.go
internal/agent/rpc/cov_final_gaps_test.go
internal/agent/rpc/cov_io_errors_test.go
internal/agent/rpc/cov_job_provider_test.go
internal/agent/rpc/cov_job_provider_unix_test.go
internal/agent/rpc/cov_logs_provider_test.go
internal/agent/rpc/cov_nas_targets_test.go
internal/agent/rpc/cov_nginx_provider_test.go
internal/agent/rpc/cov_openwrt_provider_test.go
internal/agent/rpc/cov_pct2_test.go
internal/agent/rpc/cov_pct_test.go
internal/agent/rpc/cov_pve_provider_test.go
internal/agent/rpc/cov_round10_test.go
internal/agent/rpc/cov_round5_test.go
internal/agent/rpc/cov_round6_test.go
internal/agent/rpc/cov_round7_test.go
internal/agent/rpc/cov_round8_test.go
internal/agent/rpc/cov_router_system_test.go
internal/agent/rpc/cov_stack_provider_test.go
internal/agent/rpc/cron_next.go
internal/agent/rpc/cron_next_test.go
internal/agent/rpc/cron_provider.go
internal/agent/rpc/cron_provider_test.go
internal/agent/rpc/cron_timers.go
internal/agent/rpc/cron_timers_test.go
internal/agent/rpc/cron_users.go
internal/agent/rpc/cron_users_test.go
internal/agent/rpc/ddns_provider.go
internal/agent/rpc/ddns_provider_test.go
internal/agent/rpc/docker_provider.go
internal/agent/rpc/drift_provider.go
internal/agent/rpc/drift_provider_test.go
internal/agent/rpc/file_provider.go
internal/agent/rpc/file_provider_test.go
internal/agent/rpc/file_stat_unix.go
internal/agent/rpc/file_stat_windows.go
internal/agent/rpc/firewall_parse.go
internal/agent/rpc/firewall_parse_test.go
internal/agent/rpc/firewall_provider.go
internal/agent/rpc/firewall_provider_test.go
internal/agent/rpc/handlers_mock_test.go
internal/agent/rpc/handlers_test.go
internal/agent/rpc/job_provider.go
internal/agent/rpc/job_provider_unix.go
internal/agent/rpc/job_provider_windows.go
internal/agent/rpc/logs_follow.go
internal/agent/rpc/logs_follow_test.go
internal/agent/rpc/logs_provider.go
internal/agent/rpc/logs_provider_test.go
internal/agent/rpc/nas_dsm.go
internal/agent/rpc/nas_dsm_test.go
internal/agent/rpc/nas_omv.go
internal/agent/rpc/nas_omv_test.go
internal/agent/rpc/nas_provider.go
internal/agent/rpc/nas_provider_test.go
internal/agent/rpc/nas_truenas.go
internal/agent/rpc/nas_truenas_test.go
internal/agent/rpc/nginx_provider.go
internal/agent/rpc/nginx_provider_test.go
internal/agent/rpc/openwrt_provider.go
internal/agent/rpc/overlay_parse.go
internal/agent/rpc/overlay_parse_test.go
internal/agent/rpc/overlay_provider.go
internal/agent/rpc/overlay_provider_test.go
internal/agent/rpc/overlay_write.go
internal/agent/rpc/overlay_write_test.go
internal/agent/rpc/pve_provider.go
internal/agent/rpc/router_execlog_test.go
internal/agent/rpc/router.go
internal/agent/rpc/service_health.go
internal/agent/rpc/service_health_test.go
internal/agent/rpc/service_launchd_darwin.go
internal/agent/rpc/service_launchd_model.go
internal/agent/rpc/service_launchd_model_test.go
internal/agent/rpc/service_launchd_stub.go
internal/agent/rpc/service_launchd_stub_test.go
internal/agent/rpc/service_provider.go
internal/agent/rpc/service_provider_test.go
internal/agent/rpc/service_windows_model.go
internal/agent/rpc/service_windows_model_test.go
internal/agent/rpc/service_windows_scm.go
internal/agent/rpc/service_windows_scm_test.go
internal/agent/rpc/smart_parse.go
internal/agent/rpc/smart_parse_test.go
internal/agent/rpc/smart_provider.go
internal/agent/rpc/smart_provider_test.go
internal/agent/rpc/ssh_provider.go
internal/agent/rpc/ssh_provider_test.go
internal/agent/rpc/stack_deploy_follow_test.go
internal/agent/rpc/stack_git.go
internal/agent/rpc/stack_git_test.go
internal/agent/rpc/stack_provider.go
internal/agent/rpc/stack_provider_test.go
internal/agent/rpc/stack_task_follow.go
internal/agent/rpc/system_provider.go
internal/agent/rpc/traefik_provider.go
internal/agent/rpc/traefik_provider_test.go
internal/agent/startcmd.go
internal/agent/startcmd_test.go
internal/agent/virtualization_extra_test.go
internal/agent/virtualization.go
internal/agent/virtualization_test.go
internal/alert/cov_io_gaps_test.go
internal/alert/drift_scan_test.go
internal/alert/generator_dedup_test.go
internal/alert/generator.go
internal/alert/generator_test.go
internal/alert/generator_thresholds_test.go
internal/alert/inspection_alerts_test.go
internal/audit/cockpit.db
internal/audit/cov_logger_test.go
internal/audit/entry_test.go
internal/audit/logger.go
internal/audit/logger_test.go
internal/auth/cov_handler_test.go
internal/auth/cov_jwt_test.go
internal/auth/cov_totp_test.go
internal/auth/handler.go
internal/auth/handler_test.go
internal/auth/jwt.go
internal/auth/jwt_test.go
internal/auth/lazy_init_test.go
internal/auth/middleware.go
internal/auth/middleware_test.go
internal/auth/password_reset.go
internal/auth/password_reset_test.go
internal/auth/totp.go
internal/auth/totp_test.go
internal/cert/cov_monitor_test.go
internal/cert/monitor_file_test.go
internal/cert/monitor.go
internal/cert/monitor_test.go
internal/cli/cov_cli_test.go
internal/cli/cov_io_gaps_test.go
internal/cli/init.go
internal/cli/init_test.go
internal/cli/status.go
internal/cli/status_test.go
internal/cli/sync.go
internal/cli/sync_test.go
internal/config/config.go
internal/config/config_test.go
internal/config/config_vault_providers_test.go
internal/config/cov_config_test.go
internal/config/cov_env_test.go
internal/config/cov_vault_test.go
internal/dns/alidns.go
internal/dns/alidns_test.go
internal/dns/cloudflare.go
internal/dns/cloudflare_test.go
internal/dns/cov_cloudflare_test.go
internal/dns/cov_errors_test.go
internal/dns/dnspod.go
internal/dns/dnspod_test.go
internal/dns/sign.go
internal/docker/client.go
internal/docker/client_test.go
internal/docker/cov_client_test.go
internal/docker/docker_api_test.go
internal/domain/cov_monitor_test.go
internal/domain/monitor.go
internal/domain/monitor_test.go
internal/github/client.go
internal/github/client_test.go
internal/github/cov_client_test.go
internal/health/checker.go
internal/health/checker_http_test.go
internal/health/checker_test.go
internal/health/cov_checker_test.go
internal/inventory/consistency.go
internal/inventory/consistency_test.go
internal/inventory/cov_compute_online_test.go
internal/inventory/cov_schema_test.go
internal/inventory/cov_sync_test.go
internal/inventory/schema.go
internal/inventory/schema_resources_test.go
internal/inventory/schema_test.go
internal/inventory/sync.go
internal/inventory/sync_test.go
internal/links/cov_manager_test.go
internal/links/manager.go
internal/links/manager_test.go
internal/notification/channel_herald.go
internal/notification/channel_ntfy.go
internal/notification/channel_telegram.go
internal/notification/channel_webhook.go
internal/notification/client.go
internal/notification/client_test.go
internal/notification/cov_channels_test.go
internal/notification/cov_telegram_test.go
internal/notification/events.go
internal/notification/service.go
internal/notification/service_test.go
internal/openwrt/client_func_test.go
internal/openwrt/client.go
internal/openwrt/client_rpc_test.go
internal/openwrt/client_test.go
internal/openwrt/cov_client_test.go
internal/overlay/cov_cloud_test.go
internal/overlay/overlay_test.go
internal/overlay/tailscale.go
internal/overlay/zerotier.go
internal/probeagent/agent.go
internal/probeagent/agent_test.go
internal/probeagent/config.go
internal/probeagent/config_test.go
internal/probeagent/report.go
internal/probeagent/report_test.go
internal/probeagent/upstream.go
internal/probeagent/upstream_test.go
internal/probe/cov_binding_ref_test.go
internal/probe/cov_io_gaps_test.go
internal/probe/cov_runner_test.go
internal/probe/runner.go
internal/probe/runner_history_test.go
internal/probe/runner_test.go
internal/protocol/codec.go
internal/protocol/codec_test.go
internal/protocol/codec_ws_test.go
internal/protocol/cov_decode_test.go
internal/protocol/decode.go
internal/protocol/decode_test.go
internal/protocol/execlog.go
internal/protocol/id.go
internal/protocol/id_test.go
internal/protocol/message.go
internal/protocol/message_test.go
internal/protocol/remote.go
internal/protocol/remote_test.go
internal/proxy/cov_coverage_gaps_test.go
internal/proxy/cov_handler_test.go
internal/proxy/handler_flow_test.go
internal/proxy/handler.go
internal/proxy/handler_test.go
internal/proxy/mgr/cov_io_gaps_test.go
internal/proxy/mgr/cov_manager_test.go
internal/proxy/mgr/manager_e2e_test.go
internal/proxy/mgr/manager.go
internal/proxy/mgr/manager_test.go
internal/proxy/ssh_keys.go
internal/proxy/ssh_keys_test.go
internal/proxy/ssh_session_e2e_test.go
internal/proxy/ssh_session.go
internal/proxy/ssh_session_test.go
internal/pve/client_func_test.go
internal/pve/client.go
internal/pve/client_test.go
internal/pve/cov_client_test.go
internal/server/acme_issuer.go
internal/server/acme_scan.go
internal/server/acme_test.go
internal/server/agent_expiry_test.go
internal/server/agent.go
internal/server/alert_loop.go
internal/server/api_acme.go
internal/server/api_approvals.go
internal/server/api_approvals_test.go
internal/server/api_audit.go
internal/server/api_backups.go
internal/server/api_cron.go
internal/server/api_cron_test.go
internal/server/api_ddns.go
internal/server/api_dns_batch_test.go
internal/server/api_dns_cmdb.go
internal/server/api_dns_cmdb_test.go
internal/server/api_dns.go
internal/server/api_dns_providers_test.go
internal/server/api_dns_test.go
internal/server/api_docker.go
internal/server/api_docker_test.go
internal/server/api_domain_bindings.go
internal/server/api_drift.go
internal/server/api_drift_test.go
internal/server/api_files.go
internal/server/api_files_test.go
internal/server/api_firewall.go
internal/server/api_firewall_test.go
internal/server/api.go
internal/server/api_guacamole.go
internal/server/api_guacamole_test.go
internal/server/api_inventory.go
internal/server/api_inventory_test.go
internal/server/api_jobs.go
internal/server/api_logs_follow.go
internal/server/api_logs_follow_test.go
internal/server/api_logs.go
internal/server/api_logs_search.go
internal/server/api_logs_search_test.go
internal/server/api_logs_test.go
internal/server/api_metrics.go
internal/server/api_nas.go
internal/server/api_overlay_cloud.go
internal/server/api_overlay_cloud_test.go
internal/server/api_overlay.go
internal/server/api_overlay_test.go
internal/server/api_probe.go
internal/server/api_probe_test.go
internal/server/api_proxy.go
internal/server/api_proxy_sites.go
internal/server/api_proxy_sites_test.go
internal/server/api_recordings.go
internal/server/api_remote.go
internal/server/api_roles.go
internal/server/api_roles_test.go
internal/server/api_server_backup.go
internal/server/api_service.go
internal/server/api_service_health.go
internal/server/api_service_health_test.go
internal/server/api_service_test.go
internal/server/api_smart.go
internal/server/api_smart_test.go
internal/server/api_stack_inventory_test.go
internal/server/api_stacks.go
internal/server/api_stacks_test.go
internal/server/api_stack_task_follow.go
internal/server/api_stack_task_follow_test.go
internal/server/api_stack_webhook.go
internal/server/api_stack_webhook_test.go
internal/server/api_tags.go
internal/server/api_test.go
internal/server/api_vault_entries.go
internal/server/api_vault.go
internal/server/api_workflow.go
internal/server/backup_hook_test.go
internal/server/backup_loop.go
internal/server/backup_remote_test.go
internal/server/backup_test.go
internal/server/cov_agent_apis_test.go
internal/server/cov_agent_domains_test.go
internal/server/cov_api_backups_test.go
internal/server/cov_api_docker_test.go
internal/server/cov_api_final_test.go
internal/server/cov_api_gaps_test.go
internal/server/cov_api_probe_test.go
internal/server/cov_api_routes_gaps_test.go
internal/server/cov_api_stacks_test.go
internal/server/cov_api_tags_test.go
internal/server/cov_api_test.go
internal/server/cov_coverage_gaps_test.go
internal/server/cov_domain_bindings_test.go
internal/server/cov_domain_drift_test.go
internal/server/cov_final_gaps_test.go
internal/server/cov_final_test.go
internal/server/cov_guac_default_key_test.go
internal/server/cov_guac_hs_write_inject_test.go
internal/server/cov_guac_relay_gaps_test.go
internal/server/cov_guac_relay_test.go
internal/server/cov_heartbeat_presence_test.go
internal/server/cov_helpers_test.go
internal/server/cov_http_method_test.go
internal/server/cov_io_gaps_test.go
internal/server/cov_jobs_test.go
internal/server/cov_last_gaps_test.go
internal/server/cov_loops_test.go
internal/server/cov_misc_test.go
internal/server/cov_more_gaps_test.go
internal/server/cov_pct2_test.go
internal/server/cov_pct3_test.go
internal/server/cov_pct_test.go
internal/server/cov_remote_session_lifecycle_test.go
internal/server/cov_remote_ws_test.go
internal/server/cov_round11_test.go
internal/server/cov_round4_handlers_test.go
internal/server/cov_round4_test.go
internal/server/cov_round5_handlers_test.go
internal/server/cov_round5_scan_test.go
internal/server/cov_round6_test.go
internal/server/cov_server_core_test.go
internal/server/cov_ssh_default_lookup_test.go
internal/server/cov_stack_git_test.go
internal/server/cov_vault_gaps_test.go
internal/server/cov_vault_test.go
internal/server/cov_workflow_test.go
internal/server/cov_ws_flow_test.go
internal/server/data/cockpit.db
internal/server/ddns_scan.go
internal/server/ddns_scan_test.go
internal/server/dispatcher.go
internal/server/drift_scan.go
internal/server/drift_scan_test.go
internal/server/execlog_report.go
internal/server/execlog_report_test.go
internal/server/guac_integration_test.go
internal/server/guac_relay.go
internal/server/inventory_sync_test.go
internal/server/metrics_test.go
internal/server/middleware.go
internal/server/middleware_test.go
internal/server/nas_scan.go
internal/server/nas_scan_test.go
internal/server/orphan_run.go
internal/server/orphan_run_test.go
internal/server/password_reset_handlers.go
internal/server/password_reset_handlers_test.go
internal/server/probe_report.go
internal/server/probe_report_test.go
internal/server/rbac.go
internal/server/rbac_test.go
internal/server/recording.go
internal/server/recording_remote_test.go
internal/server/recording_test.go
internal/server/registry.go
internal/server/registry_test.go
internal/server/remote_audit.go
internal/server/remote_audit_test.go
internal/server/remote_session.go
internal/server/remote_session_test.go
internal/server/server_backup.go
internal/server/server_backup_remote_test.go
internal/server/server_backup_test.go
internal/server/server.go
internal/server/service_health_scan.go
internal/server/service_health_scan_test.go
internal/server/session_test.go
internal/server/smart_scan.go
internal/server/smart_scan_test.go
internal/server/system_info.go
internal/server/ticket.go
internal/server/ticket_test.go
internal/server/totp_handlers.go
internal/server/totp_handlers_test.go
internal/server/vault.go
internal/server/vault_nas.go
internal/server/vault_nas_test.go
internal/server/vault_persona_test.go
internal/server/vault_secrets.go
internal/server/vault_secrets_test.go
internal/server/vault_test.go
internal/server/web.go
internal/server/websocket.go
internal/server/websocket_test.go
internal/server/web_test.go
internal/storage/acme_crypto.go
internal/storage/acme_deploy.go
internal/storage/acme_deploy_test.go
internal/storage/acme.go
internal/storage/acme_test.go
internal/storage/agent_expiry_test.go
internal/storage/alert.go
internal/storage/alert_test.go
internal/storage/approval.go
internal/storage/approval_test.go
internal/storage/audit_log.go
internal/storage/audit_log_test.go
internal/storage/backup.go
internal/storage/backup_test.go
internal/storage/capability_crypto.go
internal/storage/cov_acme_crypto_test.go
internal/storage/cov_agent_test.go
internal/storage/cov_capability_crypto_test.go
internal/storage/cov_crypto_test.go
internal/storage/cov_inject_test.go
internal/storage/cov_misc_test.go
internal/storage/cov_paths_test.go
internal/storage/cov_presence_batch_test.go
internal/storage/cov_recording_test.go
internal/storage/cov_remote_credential_gaps_test.go
internal/storage/cov_remote_credential_vault_test.go
internal/storage/cov_round1_test.go
internal/storage/cov_serializer_scan_test.go
internal/storage/cov_totp_validate_test.go
internal/storage/cov_upsert_fix_test.go
internal/storage/crypto.go
internal/storage/crypto_test.go
internal/storage/ddns.go
internal/storage/ddns_test.go
internal/storage/domain_binding.go
internal/storage/domain_binding_test.go
internal/storage/job.go
internal/storage/job_test.go
internal/storage/metrics.go
internal/storage/models.go
internal/storage/orphan_sweep_test.go
internal/storage/password.go
internal/storage/password_test.go
internal/storage/probe_result.go
internal/storage/probe_result_test.go
internal/storage/probe_target.go
internal/storage/probe_target_test.go
internal/storage/proxy.go
internal/storage/proxy_test.go
internal/storage/recording.go
internal/storage/remote_credential.go
internal/storage/remote_credential_test.go
internal/storage/role.go
internal/storage/role_test.go
internal/storage/setting.go
internal/storage/setting_test.go
internal/storage/stack.go
internal/storage/stack_test.go
internal/storage/storage.go
internal/storage/storage_test.go
internal/storage/tag.go
internal/storage/tag_test.go
internal/storage/totp.go
internal/storage/totp_test.go
internal/storage/user.go
internal/storage/user_test.go
internal/storage/workflow.go
internal/storage/workflow_test.go
internal/sync/cov_consistency_test.go
internal/sync/cov_io_gaps2_test.go
internal/sync/cov_round1_test.go
internal/sync/cov_round2_test.go
internal/sync/cov_watcher_test.go
internal/sync/watcher.go
internal/sync/watcher_integration_test.go
internal/sync/watcher_reload_test.go
internal/sync/watcher_test.go
internal/vault/crypto.go
internal/vault/local.go
internal/vault/persona.go
internal/vault/persona_test.go
internal/vault/session.go
internal/vault/vault.go
internal/vault/vault_test.go
仓库规模约 9,500 个文件，主要实现集中在 Go 服务端/agent 与 React 前端；我会优先按安全边界、远程执行/会话、持久化和文档承诺核验，并把无法从仓内验证的真机事项标为「未核实」。
[0m$ [0mfind . -maxdepth 2 -type d | sort | sed -n '1,200p'; git status --short --ignored | sed -n '1,80p'
.
./.acceptance
./.acceptance/agent-backup
./.acceptance/agents-expiry
./.acceptance/baseline
./.acceptance/baseline-vault
./.acceptance/ddns
./.acceptance/firewall
./.acceptance/guac
./.acceptance/jobs
./.acceptance/logs
./.acceptance/nas
./.acceptance/nginx
./.acceptance/notify
./.acceptance/overlay
./.acceptance/server-backup
./.acceptance/services
./.acceptance/smart
./.acceptance/stacks
./.acceptance/traefik
./.acceptance/workflows
./bin
./.claude
./cmd
./cmd/cockpit
./cmd/cockpit-agent
./cmd/cockpit-probe-agent
./config
./core
./core/backoff
./core/crash
./core/execlog
./core/healthprobe
./core/heartbeat
./core/logx
./core/metrics
./core/platform
./core/register
./core/report
./deployments
./deployments/docker
./deployments/guacd
./deployments/nginx
./docs
./docs/archive
./docs/design
./docs/guide
./docs/node_modules
./docs/operations
./docs/public
./docs/research
./docs/.vitepress
./examples
./.git
./.git/hooks
./.github
./.github/workflows
./.git/info
./.git/logs
./.git/objects
./.git/refs
./.git/worktrees
./internal
./internal/agent
./internal/alert
./internal/audit
./internal/auth
./internal/cert
./internal/cli
./internal/config
./internal/dns
./internal/docker
./internal/domain
./internal/github
./internal/health
./internal/inventory
./internal/links
./internal/notification
./internal/openwrt
./internal/overlay
./internal/probe
./internal/probeagent
./internal/protocol
./internal/proxy
./internal/pve
./internal/server
./internal/storage
./internal/sync
./internal/vault
./mobile
./mobile/android
./mobile/coverage
./mobile/.dart_tool
./mobile/ios
./mobile/lib
./mobile/test
./mobile/tool
./packaging
./packaging/windows
./scripts
./scripts/acceptance
./scripts/.acceptance
./scripts/local-acceptance
./scripts/openwrt
./.spec-workflow
./.spec-workflow/templates
./.spec-workflow/user-templates
./tool
./web
./web/coverage
./web/public
./web/src
./web/tool
!! .acceptance/
!! bin/
!! cockpit
!! cockpit-agent
!! cockpit-agent.exe
!! coverage-rdp.out
!! coverage.out
!! docs/.vitepress/dist/
!! docs/node_modules/
!! gate_go13.log
!! gate_go14.log
!! internal/audit/cockpit.db
!! internal/server/data/
!! mobile/.dart_tool/
!! mobile/.flutter-plugins-dependencies
!! mobile/android/app/src/main/java/
!! mobile/android/local.properties
!! mobile/coverage/
!! mobile/ios/Flutter/Generated.xcconfig
!! mobile/ios/Flutter/ephemeral/
!! mobile/ios/Flutter/flutter_export_environment.sh
!! mobile/ios/Runner/GeneratedPluginRegistrant.h
!! mobile/ios/Runner/GeneratedPluginRegistrant.m
!! scripts/.acceptance/
!! web/coverage/
!! web/gate_web.log
[0m$ [0mfind .github -type f -maxdepth 4 -print 2>/dev/null; find deployments -maxdepth 3 -type f -print 2>/dev/null; find mobile -maxdepth 4 -type f -print 2>/dev/null | head -120
.github/dependabot.yml
.github/workflows/docker.yml
.github/workflows/test.yml
.github/workflows/agent-test.yml
.github/workflows/build.yml
.github/workflows/nightly.yml
.github/workflows/mobile.yml
.github/workflows/deploy-dev.yml
.github/workflows/docs.yml
deployments/cockpit-agent.service
deployments/docker/config.yaml
deployments/docker/docker-compose.yml
deployments/docker/README.md
deployments/docker/entrypoint.sh
deployments/docker/.env.example
deployments/uninstall-windows.ps1
deployments/cockpit-agent.env
deployments/setup-dev.sh
deployments/README.md
deployments/guacd/docker-compose.yml
deployments/guacd/README.md
deployments/install-server.sh
deployments/cockpit-server.service
deployments/cockpit-server.env
deployments/install-server.ps1
deployments/nginx/cockpit.cuihairu.site.conf
deployments/DEPLOY-DEV.md
mobile/pubspec.lock
mobile/.flutter-plugins-dependencies
mobile/test/app_test.dart
mobile/test/biometric_test.dart
mobile/test/auth_test.dart
mobile/test/client_test.dart
mobile/test/home_page_test.dart
mobile/test/resources_page_test.dart
mobile/test/server_setup_page_test.dart
mobile/test/lock_gate_widget_test.dart
mobile/test/dashboard_page_test.dart
mobile/test/cron_page_test.dart
mobile/test/lock_gate_test.dart
mobile/test/app_theme_test.dart
mobile/test/settings_state_test.dart
mobile/test/agents_page_test.dart
mobile/test/models_test.dart
mobile/test/stacks_page_test.dart
mobile/test/terminal_page_test.dart
mobile/test/smart_page_test.dart
mobile/test/nas_page_test.dart
mobile/test/main_smoke_test.dart
mobile/test/settings_page_test.dart
mobile/test/alerts_page_test.dart
mobile/test/fixtures/localhost-key.pem
mobile/test/fixtures/localhost-cert.pem
mobile/test/login_page_test.dart
mobile/test/overlay_page_test.dart
mobile/test/endpoints_test.dart
mobile/test/files_page_test.dart
mobile/test/audit_page_test.dart
mobile/test/proxy_page_test.dart
mobile/test/backups_tab_test.dart
mobile/android/gradle/wrapper/gradle-wrapper.properties
mobile/android/build.gradle.kts
mobile/android/app/build.gradle.kts
mobile/android/app/proguard-rules.pro
mobile/android/gradle.properties
mobile/android/local.properties
mobile/android/settings.gradle.kts
mobile/android/key.properties.example
mobile/android/.gitignore
mobile/.dart_tool/package_config.json
mobile/.dart_tool/flutter_build/dart_plugin_registrant.dart
mobile/.dart_tool/package_graph.json
mobile/.dart_tool/version
mobile/README.md
mobile/pubspec.yaml
mobile/ios/Runner.xcworkspace/contents.xcworkspacedata
mobile/ios/Runner.xcworkspace/xcshareddata/IDEWorkspaceChecks.plist
mobile/ios/Runner.xcworkspace/xcshareddata/WorkspaceSettings.xcsettings
mobile/ios/RunnerTests/RunnerTests.swift
mobile/ios/Runner.xcodeproj/project.pbxproj
mobile/ios/Runner.xcodeproj/project.xcworkspace/contents.xcworkspacedata
mobile/ios/Flutter/Generated.xcconfig
mobile/ios/Flutter/Release.xcconfig
mobile/ios/Flutter/ephemeral/.swift_pm.lock
mobile/ios/Flutter/ephemeral/flutter_lldb_helper.py
mobile/ios/Flutter/ephemeral/flutter_lldbinit
mobile/ios/Flutter/ephemeral/flutter_native_integration.env
mobile/ios/Flutter/AppFrameworkInfo.plist
mobile/ios/Flutter/flutter_export_environment.sh
mobile/ios/Flutter/Debug.xcconfig
mobile/ios/Runner/AppDelegate.swift
mobile/ios/Runner/Runner-Bridging-Header.h
mobile/ios/Runner/GeneratedPluginRegistrant.m
mobile/ios/Runner/SceneDelegate.swift
mobile/ios/Runner/Info.plist
mobile/ios/Runner/Base.lproj/Main.storyboard
mobile/ios/Runner/Base.lproj/LaunchScreen.storyboard
mobile/ios/Runner/GeneratedPluginRegistrant.h
mobile/ios/.gitignore
mobile/analysis_options.yaml
mobile/.metadata
mobile/tool/coverage_check.sh
mobile/tool/known_uncoverable.txt
mobile/.gitignore
mobile/lib/widgets/lock_gate.dart
mobile/lib/pages/terminal_page.dart
mobile/lib/pages/home_page.dart
mobile/lib/pages/login_page.dart
mobile/lib/pages/backups_tab.dart
mobile/lib/pages/smart_page.dart
mobile/lib/pages/audit_page.dart
mobile/lib/pages/server_setup_page.dart
mobile/lib/pages/settings_page.dart
mobile/lib/pages/cron_page.dart
mobile/lib/pages/resources_page.dart
mobile/lib/pages/nas_page.dart
mobile/lib/pages/overlay_page.dart
mobile/lib/pages/dashboard_page.dart
mobile/lib/pages/stacks_page.dart
mobile/lib/pages/proxy_page.dart
mobile/lib/pages/files_page.dart
mobile/lib/pages/agents_page.dart
mobile/lib/pages/proxy_editor.dart
mobile/lib/pages/alerts_page.dart
mobile/lib/main.dart
mobile/lib/models/models.dart
mobile/lib/api/endpoints.dart
mobile/lib/api/client.dart
mobile/lib/theme/app_theme.dart
mobile/lib/state/auth.dart
mobile/lib/state/biometric.dart
mobile/lib/state/settings.dart
mobile/coverage/lcov.info
[0m$ [0mrg -n "TODO|FIXME|XXX|BUG|未实现|未支持|暂不支持|panic\(" --glob '!**/*_test.go' --glob '!docs/pnpm-lock.yaml' --glob '!web/pnpm-lock.yaml'
deployments/DEPLOY-DEV.md:16:4. 复制 token（格式类似 `AXXXXXXXXXXXXXXXXXXXXXXX`）
docs/审计-文档一致性.md:13:- `docs/operations/deploy-docker.md`、`docs/public/README.md`、`README.md`、`BUGS.md`
docs/审计-文档一致性.md:55:- websocket-token-auth.md、docs/public/README.md（脚手架说明无技术主张）、BUGS.md——符
docs/审计-文档一致性.md:83:> 范围：guide 全目录状态类主张扫描（「立项设计，未实现」「仍为候选，未排期」
docs/审计-文档一致性.md:84:> 「未实现/待实现/未落地/暂未/待办」），与码对读。仅 workflow-design.md 两处，
docs/审计-文档一致性.md:89:| C1 | guide/workflow-design.md:3（原） | 顶部横幅「2026-10-09 立项设计，未实现」 | 同文「实现状态」节 M1/M2a/M2b/M2c 全部落地 | 横幅改为「实现进度以实现状态节为准」 |
scripts/local-acceptance/run-acceptance.sh:37:WORK="$(mktemp -d /tmp/cockpit-accept.XXXXXX)"
internal/protocol/message.go:183:	ProxyType string `json:"proxyType"`          // 仅 tcp（udp 预留未实现，API 拒收，见 D-2026-10-08-2）
docs/guide/agent-egress-sdwan.md:54:仓库中有 WireGuard / ZeroTier / Tailscale 能力检测（`internal/agent/detector/overlay.go`，任一可用即报 `overlay` capability），但这些只是能力检测，不是隧道建立或路由管理；Cloudflare Tunnel 与路由信息探测未实现。
BUGS.md:1:# BUGS.md — 用户 bug 登记簿
docs/guide/workflow-design.md:62:- M2+ 其余各项在「边界（明确不做）」；未实现项以本文设计为准。
core/metrics/collector.go:44:	// CPU 频率需要从 /proc/cpuinfo 读取，gopsutil 暂不支持跨平台
docs/archive/plans/2026-05-27-notification-module.md:1348:- [x] 无 TBD/TODO
docs/ui-theme-design.md:200:- **已知差异**：移动端暂不支持自定义强调色，强调色固定为 Web 默认主题色「蔷薇红」
docs/guide/windows-agent-design.md:7:> 需求来源：用户催办（B1，见仓库 `BUGS.md`）。
internal/storage/proxy.go:12:	ProxyType   string    `gorm:"size:10;not null" json:"proxyType"` // tcp（udp 未实现，API 拒收，见 D-2026-10-08-2）
docs/guide/decision-log.md:89:标注「udp 预留未实现」。
docs/audit-doc-consistency.en.md:16:- `docs/operations/deploy-docker.md`, `docs/public/README.md`, `README.md`, `BUGS.md`
docs/audit-doc-consistency.en.md:58:- websocket-token-auth.md, docs/public/README.md (scaffolding note with no technical claims), BUGS.md — match
docs/guide/windows-agent-design.en.md:6:> Requirement origin: user urging (B1, see the repo's `BUGS.md`).
docs/guide/jobs-design.md:19:- **未实现**（按本文「边界（明确不做）」节，立项时再设计，不写进当前能力）：
mobile/lib/theme/app_theme.dart:7:/// Web 端强调色可独立切换，移动端暂不支持自定义，两端口径差异见
docs/guide/nas-design.md:263:  丢弃；type 未实现的条目忽略
docs/design/external-session-api.md:5:> 状态：**契约设计，未实现**（本文件先落档，实现随 memex 对接排期）。
docs/design/external-session-api.md:25:**未实现的新设计**——两类状态混排会让「以文档为准审码」时无法区分
docs/design/external-session-api.md:26:幽灵功能与真实现。独立文件 + 状态头（未实现）保持口径干净；实现落地
web/src/App.test.tsx:269:    // handleLogout：logout() 后 window.location.href='/login'（jsdom 导航未实现，
mobile/ios/Runner.xcodeproj/project.pbxproj:351:				DEBUG_INFORMATION_FORMAT = "dwarf-with-dsym";
mobile/ios/Runner.xcodeproj/project.pbxproj:364:				MTL_ENABLE_DEBUG_INFO = NO;
mobile/ios/Runner.xcodeproj/project.pbxproj:404:				SWIFT_ACTIVE_COMPILATION_CONDITIONS = DEBUG;
mobile/ios/Runner.xcodeproj/project.pbxproj:472:				DEBUG_INFORMATION_FORMAT = dwarf;
mobile/ios/Runner.xcodeproj/project.pbxproj:481:					"DEBUG=1",
mobile/ios/Runner.xcodeproj/project.pbxproj:491:				MTL_ENABLE_DEBUG_INFO = YES;
mobile/ios/Runner.xcodeproj/project.pbxproj:530:				DEBUG_INFORMATION_FORMAT = "dwarf-with-dsym";
mobile/ios/Runner.xcodeproj/project.pbxproj:543:				MTL_ENABLE_DEBUG_INFO = NO;
web/src/components/ErrorBoundary/index.test.tsx:25:    // jsdom location.reload 未实现，stub 掉
web/src/pages/Resources/columns.tsx:94:          <Tooltip title="暂未实现">
web/src/pages/Resources/columns.tsx:103:          <Tooltip title="暂未实现">
web/src/pages/Resources/columns.tsx:112:// 通用资源行操作（暂未实现，禁用）
web/src/pages/Resources/columns.tsx:115:    <Tooltip title="暂未实现">
web/src/pages/Resources/columns.tsx:118:    <Tooltip title="暂未实现">
[0m$ [0mrg -n "InsecureSkipVerify|Access-Control-Allow-Origin|CheckOrigin|Parse.*Token|JWT|secret|password|exec\.Command|CommandContext|filepath\.Join|ServeFile|http\.Redirect|ReadAll|LimitReader|MaxBytesReader" internal cmd core --glob '*.go'
path.Join(dir, "subdir", "test.yaml")
internal/inventory/schema_test.go:794:	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
internal/inventory/schema_test.go:795:	inv, err := ParseFile(filepath.Join(repoRoot, "examples", "inventory.yaml"))
internal/agent/rpc/nas_truenas_test.go:48:func newTruenasTestServer(t *testing.T, username, password string, badAuth bool) *httptest.Server {
internal/agent/rpc/nas_truenas_test.go:52:		if !ok || user != username || pass != password || badAuth {
internal/agent/rpc/nas_truenas_test.go:79:	targets := `[{"name":"tn-home","type":"truenas","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":false}]`
internal/agent/rpc/nas_truenas_test.go:167:		t.Error("snapshot leaks password")
internal/agent/rpc/nas_truenas_test.go:176:	targets := `[{"name":"bad-tn","type":"truenas","addr":"` + srv.URL + `","username":"cockpit","password":"wrong","insecureTls":false},
internal/agent/rpc/nas_truenas_test.go:177:	             {"name":"good-tn","type":"truenas","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":false}]`
internal/dns/alidns_test.go:55:// TestAliSign 与独立实现的 HMAC-SHA1(secret+"&") 对拍
internal/dns/alidns_test.go:57:	got := aliSign("GET&%2F&abc", "testsecret")
internal/dns/alidns_test.go:58:	mac := hmac.New(sha1.New, []byte("testsecret&"))
internal/inventory/schema_resources_test.go:75:	path := filepath.Join(dir, "nested", "deep", "inventory.yaml")
internal/agent/rpc/overlay_parse_test.go:145:		t.Fatal("dump output leaked secret fields (D6 violation)")
internal/dns/dnspod_test.go:297:		t.Error("alidns missing secret should be nil")
internal/inventory/cov_schema_test.go:74:	blocker := filepath.Join(dir, "blocker")
internal/inventory/cov_schema_test.go:80:	err := inv.Write(filepath.Join(blocker, "sub", "inv.yaml"))
internal/inventory/cov_schema_test.go:114:	path := filepath.Join(t.TempDir(), "inv.yaml")
internal/agent/rpc/cov_job_provider_unix_test.go:11:// CommandContext 只在启动成功后才调 Cancel，该分支为纯守卫，直调覆盖
internal/agent/rpc/cov_cron_provider_test.go:56:	fake := filepath.Join(bin, "crontab")
internal/agent/rpc/cov_cron_provider_test.go:138:	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
internal/dns/dnspod.go:165:	raw, err := io.ReadAll(resp.Body)
internal/dns/sign.go:6:// HMAC-SHA1(key = secret + "&") 后 base64。
internal/dns/sign.go:41:// aliSign 对 StringToSign 做 HMAC-SHA1（key = secret + "&"）后 base64
internal/dns/sign.go:42:func aliSign(stringToSign, secret string) string {
internal/dns/sign.go:43:	mac := hmac.New(sha1.New, []byte(secret+"&"))
internal/dns/sign.go:49:func aliSignQuery(secret string, params url.Values) string {
internal/dns/sign.go:52:	return canonical + "&Signature=" + aliPercentEncode(aliSign(stringToSign, secret))
internal/agent/rpc/job_provider_unix.go:12:// CommandContext 默认只在超时时杀直接子进程（sh），sh 派生的孙进程
internal/agent/rpc/job_provider_unix.go:17:	cmd := exec.CommandContext(ctx, "sh", "-c", command)
internal/agent/rpc/ssh_provider.go:47:		keyPath := filepath.Join(dir, name)
internal/agent/rpc/ssh_provider.go:74:			return filepath.Join(filepath.Dir(filepath.Dir(home)), sudoUser, ".ssh")
internal/agent/rpc/ssh_provider.go:78:	return filepath.Join(home, ".ssh")
internal/agent/rpc/overlay_write_test.go:67:	bin := filepath.Join(t.TempDir(), "fake-overlay-cli")
internal/agent/rpc/cov_round7_test.go:414:	if v, err := p.SaveUnitFile("a.service", "[Unit]\n"); err != nil || v.(map[string]interface{})["path"] != filepath.Join(dir, "a.service") {
internal/agent/rpc/cov_round7_test.go:420:	pkgFile := filepath.Join(pkgDir, "b.service")
internal/agent/rpc/cov_round7_test.go:428:	if v, err := p.SaveUnitFile("b.service", "new"); err != nil || v.(map[string]interface{})["path"] != filepath.Join(dir, "b.service") {
internal/agent/rpc/cov_round7_test.go:431:	if data, err := os.ReadFile(filepath.Join(dir, "b.service")); err != nil || string(data) != "new" {
internal/agent/rpc/cov_round7_test.go:436:	if err := os.Mkdir(filepath.Join(dir, "c.service"), 0o755); err != nil {
internal/agent/rpc/cov_round7_test.go:439:	cFile := filepath.Join(pkgDir, "c.service")
internal/agent/rpc/cov_round7_test.go:479:	broken := NewTraefikProvider(filepath.Join(dir, "bad["), okTraefikRunner)
internal/agent/rpc/cov_round7_test.go:500:	if err := os.Mkdir(filepath.Join(dir2, "cockpit-site-block.yml"), 0o755); err != nil {
internal/agent/rpc/cov_round7_test.go:527:		if err := os.WriteFile(filepath.Join(dir3, name), []byte("x"), 0o644); err != nil {
internal/agent/rpc/cov_round7_test.go:558:	if err := os.WriteFile(filepath.Join(dir, "cockpit-site-.yml"), []byte("x"), 0o644); err != nil {
internal/agent/rpc/cov_round7_test.go:561:	p = NewDriftProvider(NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json")), DriftConfig{DynamicDir: dir})
internal/agent/rpc/nas_truenas.go:13:func basicAuth(username, password string) string {
internal/agent/rpc/nas_truenas.go:14:	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
internal/agent/rpc/cov_round5_test.go:59:	path := filepath.Join(dir, "unit.conf")
internal/agent/rpc/cov_round5_test.go:66:	blocked := filepath.Join(dir, "blocked")
internal/agent/rpc/cov_round5_test.go:167:	broken := NewTraefikProvider(filepath.Join(dir, "bad["), okTraefikRunner)
internal/agent/rpc/cov_round5_test.go:218:		path := filepath.Join(root, rel)
internal/agent/rpc/cov_round5_test.go:233:	if err := os.Symlink(filepath.Join(root, "hit.txt"), filepath.Join(root, "link.txt")); err != nil {
internal/agent/rpc/cov_round5_test.go:238:		rel = filepath.Join(rel, "d"+strconv.Itoa(i))
internal/agent/rpc/cov_round5_test.go:240:	write(filepath.Join(rel, "leaf.txt"), "findme deep\n") // 超深 → 整枝跳过
internal/agent/rpc/cov_round5_test.go:241:	if err := os.Chmod(filepath.Join(root, "noperm.txt"), 0o000); err != nil {
internal/agent/rpc/cov_round5_test.go:302:	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
internal/agent/rpc/job_provider_windows.go:11:// Windows 无进程组 SIGKILL 语义，沿用 CommandContext 默认杀直接进程；
internal/agent/rpc/job_provider_windows.go:14:	return exec.CommandContext(ctx, "cmd", "/C", command)
internal/agent/rpc/cov_round10_test.go:24:	if err := os.WriteFile(filepath.Join(src, "ok.txt"), []byte("data"), 0o644); err != nil {
internal/agent/rpc/cov_round10_test.go:27:	ln, err := net.Listen("unix", filepath.Join(src, "sock"))
internal/agent/rpc/cov_round10_test.go:56:	own := filepath.Join(t.TempDir(), "f.txt")
internal/agent/rpc/cov_round10_test.go:72:	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "cockpit-site-dead.yml")); err != nil {
internal/agent/rpc/cov_final_gaps_test.go:66:	dir := filepath.Join(p.dir, "stuck")
internal/agent/rpc/cov_final_gaps_test.go:68:	os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("x"), 0o644)
internal/agent/rpc/cov_final_gaps_test.go:89:	f := filepath.Join(t.TempDir(), "data.bin")
internal/agent/rpc/cov_final_gaps_test.go:110:	base := filepath.Join(t.TempDir(), "bl")
internal/agent/rpc/cov_final_gaps_test.go:112:	b := NewDriftBaseline(filepath.Join(base, "baseline.json"))
internal/agent/rpc/cov_final_gaps_test.go:132:	arch := filepath.Join(dir, "nested.tar.gz")
internal/agent/rpc/cov_final_gaps_test.go:158:	if err := os.WriteFile(filepath.Join(src, "blob.bin"), big, 0o644); err != nil {
internal/agent/detector/overlay_test.go:13:	fp := filepath.Join(dir, name)
internal/agent/rpc/cov_openwrt_provider_test.go:24:		body, _ := io.ReadAll(r.Body)
internal/agent/cov_providers_test.go:25:	sock := filepath.Join(t.TempDir(), "docker.sock")
internal/agent/cov_providers_test.go:139:		path := filepath.Join(dir, name)
internal/agent/rpc/nas_provider_test.go:272:	// 缺 password 的条目被 parseNasTargets 丢弃，不触发
internal/agent/rpc/nas_provider_test.go:279:	t.Setenv(nasTargetsEnv, `[{"name":"nas1","type":"dsm","addr":"https://192.168.1.10:5001","username":"u","password":"p"}]`)
internal/agent/rpc/ssh_provider_test.go:59:	if err := os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte("not a private key"), 0600); err != nil {
internal/agent/rpc/ssh_provider_test.go:63:	if err := os.WriteFile(filepath.Join(dir, "id_rsa"), genTestKeyPEM(t), 0600); err != nil {
internal/agent/rpc/ssh_provider_test.go:94:	if err := os.WriteFile(filepath.Join(dir, "id_ed25519"), genTestKeyPEM(t), 0600); err != nil {
internal/agent/rpc/ssh_provider_test.go:131:	if got := defaultSSHDir(); got != filepath.Join(home, ".ssh") {
internal/agent/rpc/ssh_provider_test.go:132:		t.Errorf("fallback dir = %q, want %q", got, filepath.Join(home, ".ssh"))
internal/agent/rpc/ssh_provider_test.go:158:	passwdPath = filepath.Join(t.TempDir(), "missing")
internal/agent/rpc/cov_round8_test.go:406:	md := filepath.Join(t.TempDir(), "mdstat")
internal/agent/rpc/cov_round8_test.go:470:	path := filepath.Join(dir, "secret.conf")
internal/agent/rpc/cov_round8_test.go:487:	big := filepath.Join(dir, "big.txt")
internal/agent/rpc/cov_round8_test.go:503:	res, err = p.Search(map[string]interface{}{"dir": filepath.Join(dir, "nope"), "query": "x"})
internal/agent/rpc/cov_round8_test.go:516:	if err := os.WriteFile(filepath.Join(dir, "chunk.tar.gz"), []byte("0123456789"), 0o644); err != nil {
internal/agent/rpc/router_execlog_test.go:40:		"params": map[string]interface{}{"unit": "nginx.service", "password": "s3cret"},
internal/agent/rpc/router_execlog_test.go:50:// 落字段，入参过脱敏（password 键剔除）。
internal/agent/rpc/router_execlog_test.go:77:	if _, ok := e.Params["password"]; ok {
internal/agent/rpc/file_provider.go:111:		full := filepath.Join(cleaned, e.Name())
internal/agent/rpc/file_provider.go:308:	target := filepath.Join(filepath.Dir(cleaned), name)
internal/agent/rpc/backup_hook_other.go:12:	return exec.Command("sh", "-c", command)
internal/agent/rpc/logs_follow.go:20:// execCommandContext 可注入（测试覆盖 StdoutPipe 失败的防御分支）
internal/agent/rpc/logs_follow.go:21:var execCommandContext = exec.CommandContext
internal/agent/rpc/logs_follow.go:36:// timeout → CommandContext kill → closeFn("exited") 路径（stdinPipeFn /
internal/agent/rpc/logs_follow.go:140:			cancel() // kill 进程（CommandContext）
internal/agent/rpc/logs_follow.go:163:		cmd = execCommandContext(ctx, "journalctl",
internal/agent/rpc/logs_follow.go:166:		cmd = execCommandContext(ctx, "docker",
internal/agent/rpc/stack_deploy_follow_test.go:37:	logPath := filepath.Join(t.TempDir(), "compose.log")
internal/agent/rpc/stack_deploy_follow_test.go:51:	if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
internal/agent/rpc/stack_deploy_follow_test.go:54:	if err := os.WriteFile(filepath.Join(dir, name, "compose.yml"), []byte("services: {}\n"), 0o600); err != nil {
internal/agent/rpc/traefik_provider_test.go:23:	p.SetBaseline(NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json")))
internal/agent/rpc/traefik_provider_test.go:103:	baseline := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/traefik_provider_test.go:112:	path := filepath.Join(dir, "cockpit-site-grafana.yml")
internal/agent/rpc/traefik_provider_test.go:159:	os.WriteFile(filepath.Join(dir, "user-own.yml"), []byte("http: {}\n"), 0o644)
internal/agent/rpc/traefik_provider_test.go:160:	os.WriteFile(filepath.Join(dir, "cockpit-site-broken.yml"), []byte("garbage"), 0o644)
internal/agent/rpc/traefik_provider_test.go:246:	yml := filepath.Join(t.TempDir(), "traefik.yml")
internal/agent/rpc/traefik_provider_test.go:253:	empty := filepath.Join(t.TempDir(), "traefik.yml")
internal/agent/rpc/traefik_provider_test.go:259:	if got := traefikDirFromStatic([]string{filepath.Join(t.TempDir(), "absent.yml")}); got != "" {
internal/agent/rpc/traefik_provider_test.go:264:	yamlAlt := filepath.Join(t.TempDir(), "traefik.yaml")
internal/agent/rpc/traefik_provider_test.go:266:	if got := traefikDirFromStatic([]string{filepath.Join(t.TempDir(), "absent.yml"), yamlAlt}); got != "/alt" {
internal/agent/rpc/traefik_provider_test.go:280:	t.Setenv("COCKPIT_TRAEFIK_DIR", filepath.Join(dir, "nope"))
internal/agent/rpc/traefik_provider_test.go:292:		if err := os.WriteFile(filepath.Join(dynDir, "cockpit-site-"+name+".yml"), []byte(content), 0o644); err != nil {
internal/agent/rpc/traefik_provider_test.go:299:	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/traefik_provider_test.go:361:	cfgPath := filepath.Join(root, "traefik.yml")
internal/agent/rpc/traefik_provider_test.go:362:	dynDir := filepath.Join(root, "dyn")
internal/agent/rpc/ddns_provider.go:120:	body, err := io.ReadAll(io.LimitReader(resp.Body, ddnsMaxBody))
internal/agent/rpc/cov_dispatch_test.go:72:	if err := atomicWriteFile(filepath.Join(t.TempDir(), "gone", "u.file"), []byte("x"), 0o644); err == nil {
internal/agent/rpc/cov_dispatch_test.go:79:	uid, gid := fileStatOwner(filepath.Join(t.TempDir(), "absent"))
internal/agent/rpc/cov_dispatch_test.go:88:	p := NewDriftProvider(NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json")), DriftConfig{ConfDir: dir})
internal/agent/rpc/nas_dsm_test.go:39:	password string // 期望密码；不匹配 → 登录失败
internal/agent/rpc/nas_dsm_test.go:54:		if q.Get("passwd") != f.password || q.Get("account") != "cockpit" || q.Get("format") != "sid" {
internal/agent/rpc/nas_dsm_test.go:103:	fake := &fakeDsm{password: fakeDsmPassword}
internal/agent/rpc/nas_dsm_test.go:107:	targets := `[{"name":"home-dsm","type":"dsm","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":false}]`
internal/agent/rpc/nas_dsm_test.go:166:		t.Error("snapshot leaks password")
internal/agent/rpc/nas_dsm_test.go:178:	fake := &fakeDsm{password: fakeDsmPassword}
internal/agent/rpc/nas_dsm_test.go:183:	targets := `[{"name":"bad","type":"dsm","addr":"` + srv.URL + `","username":"cockpit","password":"wrong","insecureTls":false},
internal/agent/rpc/nas_dsm_test.go:184:	             {"name":"future","type":"truenas","addr":"` + srv.URL + `","username":"u","password":"p","insecureTls":false},
internal/agent/rpc/nas_dsm_test.go:185:	             {"name":"good","type":"dsm","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":false}]`
internal/agent/rpc/nas_dsm_test.go:205:			t.Error("error detail leaks password")
internal/agent/rpc/nas_dsm_test.go:211:	fake := &fakeDsm{password: fakeDsmPassword}
internal/agent/rpc/nas_dsm_test.go:216:	targets := `[{"name":"tls-dsm","type":"dsm","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":true}]`
internal/agent/rpc/nas_dsm_test.go:259:		{"name":"a","type":"dsm","addr":"https://1.2.3.4:5001","username":"u","password":"p","insecureTls":true},
internal/agent/rpc/nas_dsm_test.go:261:		{"name":"c","type":"dsm","addr":"1.2.3.4:5001","username":"u","password":"p"},
internal/agent/rpc/nas_dsm_test.go:262:		{"name":"d","type":"truenas","addr":"https://5.6.7.8","username":"u","password":"p"}
internal/agent/rpc/backup_hook_test.go:22:	snapshot := filepath.Join(t.TempDir(), "app.db.bak")
internal/agent/rpc/backup_hook_test.go:46:	f, err := os.Open(filepath.Join(dest, task["file"].(string)))
internal/agent/rpc/backup_hook_test.go:67:			body, _ := io.ReadAll(tr)
internal/agent/rpc/cov_logs_provider_test.go:76:		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
internal/agent/rpc/nas_omv_test.go:41:func newOmvTestServer(t *testing.T, username, password string, badAuth bool) *httptest.Server {
internal/agent/rpc/nas_omv_test.go:54:		body, _ := io.ReadAll(r.Body)
internal/agent/rpc/nas_omv_test.go:67:				Password string `json:"password"`
internal/agent/rpc/nas_omv_test.go:70:			if badAuth || p.Username != username || p.Password != password {
internal/agent/rpc/nas_omv_test.go:72:				_, _ = w.Write([]byte(`{"response":null,"error":{"code":400,"message":"Incorrect username or password."}}`))
internal/agent/rpc/nas_omv_test.go:113:	targets := `[{"name":"omv-home","type":"omv","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":false}]`
internal/agent/rpc/nas_omv_test.go:176:		t.Error("snapshot leaks password")
internal/agent/rpc/nas_omv_test.go:184:	targets := `[{"name":"bad-omv","type":"omv","addr":"` + srv.URL + `","username":"cockpit","password":"wrong","insecureTls":false},
internal/agent/rpc/nas_omv_test.go:185:	             {"name":"good-omv","type":"omv","addr":"` + srv.URL + `","username":"cockpit","password":"` + fakeDsmPassword + `","insecureTls":false}]`
internal/agent/rpc/cov_nas_targets_test.go:18:			{"name": "box2", "type": "omv", "addr": "https://h2", "username": "u2", "password": "pw2"},
internal/agent/rpc/cov_nas_targets_test.go:31:			{"name": "box3", "type": "dsm", "addr": "https://h3", "username": "u3", "password": "pw3"},
internal/agent/rpc/cov_nas_targets_test.go:53:	ts := ParseNasTargetsJSON(`[{"name":"n1","type":"dsm","addr":"https://a:5001","username":"u","password":"p"}]`)
internal/agent/rpc/nginx_provider_test.go:182:	if _, err := os.Stat(filepath.Join(p.confDir, "cockpit-site-x.conf")); !os.IsNotExist(err) {
internal/agent/rpc/nginx_provider_test.go:192:	path := filepath.Join(p.confDir, "cockpit-site-y.conf")
internal/agent/rpc/nginx_provider_test.go:211:	if _, err := os.Stat(filepath.Join(p.confDir, "cockpit-site-z.conf")); !os.IsNotExist(err) {
internal/agent/rpc/nginx_provider_test.go:233:	content, err := os.ReadFile(filepath.Join(p.confDir, "cockpit-site-app.conf"))
internal/agent/rpc/nginx_provider_test.go:255:	path := filepath.Join(p.confDir, "cockpit-site-d.conf")
internal/agent/rpc/nginx_provider_test.go:272:	if err := os.WriteFile(filepath.Join(p.confDir, "cockpit-site-good.conf"),
internal/agent/rpc/nginx_provider_test.go:276:	if err := os.WriteFile(filepath.Join(p.confDir, "cockpit-site-broken.conf"), []byte("no meta here\n"), 0o644); err != nil {
internal/agent/rpc/nginx_provider_test.go:280:	if err := os.WriteFile(filepath.Join(p.confDir, "user-own.conf"), []byte("x"), 0o644); err != nil {
internal/agent/rpc/backup_remote_test.go:15:	argvFile = filepath.Join(dir, "argv.log")
internal/agent/rpc/backup_remote_test.go:16:	bin = filepath.Join(dir, "fake-rclone")
internal/agent/rpc/backup_remote_test.go:43:	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644); err != nil {
internal/agent/rpc/backup_remote_test.go:70:	want := fmt.Sprintf("copy --transfers 2 %s my-s3:cockpit/backups", filepath.Join(dest, task["file"].(string)))
internal/agent/rpc/backup_remote_test.go:82:	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644); err != nil {
internal/agent/rpc/backup_remote_test.go:111:	if _, err := os.Stat(filepath.Join(dest, task["file"].(string))); err != nil {
internal/agent/rpc/backup_remote_test.go:125:	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644); err != nil {
internal/agent/rpc/backup_remote_test.go:195:	if err := os.WriteFile(filepath.Join(dir, name), []byte("archive"), 0o644); err != nil {
internal/agent/rpc/backup_remote_test.go:210:	want := fmt.Sprintf("copy --transfers 2 %s my-s3:cockpit/backups", filepath.Join(dir, name))
internal/agent/rpc/backup_remote_test.go:245:	if err := os.WriteFile(filepath.Join(dir, name), []byte("archive"), 0o644); err != nil {
internal/agent/rpc/backup_remote_test.go:260:	if err := os.Symlink("/bin/true", filepath.Join(withRclone, "rclone")); err != nil {
internal/agent/rpc/overlay_provider.go:297:	body, err := io.ReadAll(io.LimitReader(resp.Body, overlayMaxOutput))
internal/agent/rpc/overlay_provider.go:329:	body, err := io.ReadAll(io.LimitReader(resp.Body, overlayMaxOutput))
internal/agent/rpc/nas_dsm.go:28:	Password    string `json:"password"`
internal/agent/rpc/nas_dsm.go:55:			log.Printf("nas target: drop incomplete entry (need name/addr/username/password)")
internal/agent/rpc/nas_dsm.go:71:		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // 家用 NAS 自签常态，D2 明示
internal/agent/rpc/nas_dsm.go:93:	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
internal/agent/rpc/cov_drift_provider_test.go:36:	path := filepath.Join(t.TempDir(), "baseline.json")
internal/agent/rpc/cov_drift_provider_test.go:60:	middle := filepath.Join(t.TempDir(), "blocker")
internal/agent/rpc/cov_drift_provider_test.go:62:	b := NewDriftBaseline(filepath.Join(middle, "sub", "baseline.json"))
internal/agent/rpc/cov_drift_provider_test.go:73:		b2 := NewDriftBaseline(filepath.Join(dir, "baseline.json"))
internal/agent/rpc/cov_drift_provider_test.go:82:	os.Mkdir(filepath.Join(base, "baseline.json"), 0o700)
internal/agent/rpc/cov_drift_provider_test.go:83:	b3 := NewDriftBaseline(filepath.Join(base, "baseline.json"))
internal/agent/rpc/cov_drift_provider_test.go:89:	if _, err := os.Stat(filepath.Join(base, "baseline.json.tmp")); !os.IsNotExist(err) {
internal/agent/rpc/cov_drift_provider_test.go:129:	base := filepath.Join(t.TempDir(), "bl")
internal/agent/rpc/cov_drift_provider_test.go:131:	b := NewDriftBaseline(filepath.Join(base, "baseline.json"))
internal/agent/rpc/cov_drift_provider_test.go:136:	os.Mkdir(filepath.Join(confDir, "cockpit-site-broken.conf"), 0o700)
internal/agent/rpc/cov_drift_provider_test.go:138:	os.MkdirAll(filepath.Join(stacksDir, "app", "compose.yml"), 0o700)
internal/agent/rpc/cov_drift_provider_test.go:139:	os.WriteFile(filepath.Join(stacksDir, "app", ".env"), []byte("K=1"), 0o644)
internal/agent/rpc/cov_drift_provider_test.go:141:	os.MkdirAll(filepath.Join(stacksDir, stackReservedDir), 0o700)
internal/agent/rpc/cov_drift_provider_test.go:142:	os.WriteFile(filepath.Join(stacksDir, "plainfile"), []byte("x"), 0o644)
internal/agent/rpc/cov_drift_provider_test.go:164:	base := filepath.Join(t.TempDir(), "bl")
internal/agent/rpc/cov_drift_provider_test.go:166:	b := NewDriftBaseline(filepath.Join(base, "baseline.json"))
internal/agent/rpc/backup_provider_test.go:70:	if err := os.WriteFile(filepath.Join(src1, "nginx.conf"), []byte("server {}"), 0o644); err != nil {
internal/agent/rpc/backup_provider_test.go:73:	if err := os.MkdirAll(filepath.Join(src1, "conf.d"), 0o755); err != nil {
internal/agent/rpc/backup_provider_test.go:76:	if err := os.WriteFile(filepath.Join(src1, "conf.d", "a.conf"), []byte("a"), 0o644); err != nil {
internal/agent/rpc/backup_provider_test.go:80:	if err := os.WriteFile(filepath.Join(src2, "db.sqlite"), []byte("data"), 0o644); err != nil {
internal/agent/rpc/backup_provider_test.go:98:	f, err := os.Open(filepath.Join(dest, fileName))
internal/agent/rpc/backup_provider_test.go:122:		body, _ := io.ReadAll(tr)
internal/agent/rpc/backup_provider_test.go:139:	if err := os.WriteFile(filepath.Join(src, "f"), []byte("x"), 0o644); err != nil {
internal/agent/rpc/backup_provider_test.go:147:		name := filepath.Join(dest, "db-2026010"+string(rune('1'+i))+"-000000.tar.gz")
internal/agent/rpc/backup_provider_test.go:197:	victim := filepath.Join(dest, "keep.txt")
internal/agent/rpc/backup_provider_test.go:218:	good := filepath.Join(dest, "ok-name.tar.gz")
internal/agent/rpc/backup_provider_test.go:234:		path := filepath.Join(dest, n)
internal/agent/rpc/backup_provider_test.go:265:	outside := filepath.Join(t.TempDir(), "secret.txt")
internal/agent/rpc/backup_provider_test.go:269:	if err := os.Symlink(outside, filepath.Join(src, "leak")); err != nil {
internal/agent/rpc/backup_provider_test.go:279:	// 解包：leak 应为符号链接条目，且不带 secret 内容
internal/agent/rpc/backup_provider_test.go:280:	f, _ := os.Open(filepath.Join(dest, task["file"].(string)))
internal/agent/rpc/backup_provider_test.go:298:			body, _ := io.ReadAll(tr)
internal/agent/rpc/backup_provider_test.go:312:	if err := os.WriteFile(filepath.Join(good, "f"), []byte("ok"), 0o644); err != nil {
internal/agent/rpc/backup_provider_test.go:328:	if err := os.WriteFile(filepath.Join(src, "big.bin"), big, 0o644); err != nil {
internal/agent/rpc/backup_provider_test.go:380:	if err := os.WriteFile(filepath.Join(src, "app.conf"), []byte("key=value"), 0o644); err != nil {
internal/agent/rpc/backup_provider_test.go:383:	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
internal/agent/rpc/backup_provider_test.go:386:	if err := os.WriteFile(filepath.Join(src, "sub", "data.txt"), []byte("hello"), 0o644); err != nil {
internal/agent/rpc/backup_provider_test.go:396:	restoreDir := filepath.Join(t.TempDir(), "restored") // 不存在 → 允许
internal/agent/rpc/backup_provider_test.go:403:	got, err := os.ReadFile(filepath.Join(restoreDir, base, "app.conf"))
internal/agent/rpc/backup_provider_test.go:407:	got, err = os.ReadFile(filepath.Join(restoreDir, base, "sub", "data.txt"))
internal/agent/rpc/backup_provider_test.go:428:	if err := os.WriteFile(filepath.Join(destDir, "existing.txt"), []byte("x"), 0o644); err != nil {
internal/agent/rpc/backup_provider_test.go:437:	got, err := os.ReadFile(filepath.Join(destDir, "existing.txt"))
internal/agent/rpc/backup_provider_test.go:502:	if err := os.WriteFile(filepath.Join(dir, archive), buf.Bytes(), 0o644); err != nil {
internal/agent/rpc/backup_provider_test.go:507:	destDir := filepath.Join(restoreRoot, "dest")
internal/agent/rpc/backup_provider_test.go:513:	if _, err := os.Stat(filepath.Join(restoreRoot, "evil.txt")); err == nil {
internal/agent/rpc/backup_provider_test.go:517:	if _, err := os.Stat(filepath.Join(destDir, "app", "ok.txt")); err != nil {
internal/agent/rpc/backup_provider_test.go:520:	if _, err := os.Stat(filepath.Join(destDir, "evil.txt")); err == nil {
internal/agent/rpc/backup_provider_test.go:539:	if err := os.WriteFile(filepath.Join(dir, name), want, 0o644); err != nil {
internal/agent/rpc/backup_provider_test.go:595:		{"traversal name", dir, "../secret.txt"},
internal/agent/rpc/traefik_provider.go:262:	return filepath.Join(p.dir, "cockpit-site-"+name+".yml")
internal/agent/rpc/traefik_provider.go:267:	matches, err := filepath.Glob(filepath.Join(p.dir, "cockpit-site-*.yml"))
internal/agent/rpc/stack_provider.go:54:	cmd := exec.CommandContext(ctx, "docker", "compose", "version")
internal/agent/rpc/stack_provider.go:272:	return filepath.Join(p.dir, name), nil
internal/agent/rpc/stack_provider.go:278:		if st, err := os.Stat(filepath.Join(dir, f)); err == nil && !st.IsDir() {
internal/agent/rpc/stack_provider.go:316:		dir := filepath.Join(p.dir, name)
internal/agent/rpc/stack_provider.go:388:	if lt := readLastTask(filepath.Join(p.dir, name, stackReservedDir)); lt != nil {
internal/agent/rpc/stack_provider.go:404:	data, err := os.ReadFile(filepath.Join(cockpitDir, "last-task.json"))
internal/agent/rpc/stack_provider.go:416:	stackDir := filepath.Join(p.dir, stack)
internal/agent/rpc/stack_provider.go:421:	dir := filepath.Join(stackDir, stackReservedDir)
internal/agent/rpc/stack_provider.go:427:	_ = os.WriteFile(filepath.Join(dir, "last-task.json"), data, 0o644)
internal/agent/rpc/stack_provider.go:444:	data, err := os.ReadFile(filepath.Join(dir, file))
internal/agent/rpc/stack_provider.go:450:	if st, err := os.Stat(filepath.Join(dir, file)); err == nil {
internal/agent/rpc/stack_provider.go:453:	if env, err := os.ReadFile(filepath.Join(dir, ".env")); err == nil {
internal/agent/rpc/stack_provider.go:487:	cockpitDir := filepath.Join(dir, stackReservedDir)
internal/agent/rpc/stack_provider.go:493:	tmp := filepath.Join(cockpitDir, "compose.yml.tmp")
internal/agent/rpc/stack_provider.go:505:			os.Remove(filepath.Join(dir, f))
internal/agent/rpc/stack_provider.go:508:	if err := os.Rename(tmp, filepath.Join(dir, "compose.yml")); err != nil {
internal/agent/rpc/stack_provider.go:516:		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(*envPtr), 0o600); err != nil {
internal/agent/rpc/stack_provider.go:532:	cmd := exec.CommandContext(ctx, p.composeBin[0],
internal/agent/rpc/stack_provider.go:650:	out, err := exec.Command(p.composeBin[0], args...).Output()
internal/agent/rpc/stack_provider.go:737:	cmd := exec.Command(p.composeBin[0], full...)
internal/agent/rpc/stack_provider.go:818:	cmd := exec.CommandContext(ctx, p.composeBin[0], args...)
internal/agent/rpc/openwrt_provider.go:16:func NewOpenWrtProvider(host string, port int, username, password string) *OpenWrtProvider {
internal/agent/rpc/openwrt_provider.go:21:		Password:    password,
internal/agent/rpc/drift_provider.go:244:	matches, _ := filepath.Glob(filepath.Join(p.confDir, "cockpit-site-*.conf"))
internal/agent/rpc/drift_provider.go:256:	tfMatches, _ := filepath.Glob(filepath.Join(p.dynamicDir, "cockpit-site-*.yml"))
internal/agent/rpc/drift_provider.go:290:				content, err := os.ReadFile(filepath.Join(p.stacksDir, e.Name(), f))
internal/agent/rpc/drift_provider.go:399:		return os.ReadFile(filepath.Join(p.confDir, "cockpit-site-"+name+".conf"))
internal/agent/rpc/drift_provider.go:401:		return os.ReadFile(filepath.Join(p.dynamicDir, "cockpit-site-"+name+".yml"))
internal/agent/rpc/drift_provider.go:404:		return os.ReadFile(filepath.Join(p.stacksDir, dirName, fileName))
internal/agent/rpc/stack_git_test.go:27:	remote := filepath.Join(t.TempDir(), "stacks-remote.git")
internal/agent/rpc/stack_git_test.go:29:		cmd := exec.Command("git", args...)
internal/agent/rpc/stack_git_test.go:43:		p := filepath.Join(work, name)
internal/agent/rpc/stack_git_test.go:60:	p := filepath.Join(dir, name)
internal/agent/rpc/stack_git_test.go:86:	data, err := os.ReadFile(filepath.Join(p.dir, "web", "compose.yml"))
internal/agent/rpc/stack_git_test.go:117:	data, _ = os.ReadFile(filepath.Join(p.dir, "web", "compose.yml"))
internal/agent/rpc/stack_git_test.go:128:		cmd := exec.Command("git", args...)
internal/agent/rpc/stack_git_test.go:195:	data, _ := os.ReadFile(filepath.Join(p.dir, "web", "compose.yml"))
internal/agent/rpc/stack_git_test.go:252:		cmd := exec.Command("git", args...)
internal/agent/rpc/stack_git_test.go:280:	if _, err := os.Stat(filepath.Join(p.dir, "b", "compose.yml")); err != nil {
internal/agent/rpc/stack_git_test.go:285:	if got := sanitizeGitSecret("https://user:secret@host/x.git"); strings.Contains(got, "secret") {
internal/agent/rpc/stack_git_test.go:314:	cmd := exec.Command("git", args...)
internal/agent/rpc/stack_git_test.go:328:	bin := filepath.Join(t.TempDir(), "git-wrap")
internal/agent/rpc/stack_git_test.go:345:	bin := filepath.Join(t.TempDir(), "git-fake-status")
internal/agent/rpc/stack_git_test.go:386:	if err := os.WriteFile(filepath.Join(p2.dir, ".git"), []byte("garbage"), 0o644); err != nil {
internal/agent/rpc/stack_git_test.go:483:	gitDir := filepath.Join(p.dir, ".git")
internal/agent/rpc/stack_git_test.go:497:	if err := os.WriteFile(filepath.Join(parent, "blocker"), []byte("x"), 0o644); err != nil {
internal/agent/rpc/stack_git_test.go:500:	p := NewStackProvider(StackConfig{Dir: filepath.Join(parent, "blocker", "stacks"), GitBin: []string{"git"}})
internal/agent/rpc/stack_git_test.go:546:	empty := filepath.Join(t.TempDir(), "empty.git")
internal/agent/rpc/cov_nginx_provider_test.go:93:	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
internal/agent/rpc/cov_nginx_provider_test.go:189:	p2 := NewNginxProvider(NginxConfig{ConfDir: filepath.Join(t.TempDir(), "missing"), Run: (&mockNginxRunner{}).run})
internal/agent/rpc/cov_nginx_provider_test.go:223:	os.WriteFile(filepath.Join(p.confDir, "cockpit-site-.conf"), []byte("x"), 0o644)
internal/agent/rpc/cov_nginx_provider_test.go:225:	os.Mkdir(filepath.Join(p.confDir, "cockpit-site-dir.conf"), 0o700)
internal/agent/rpc/drift_provider_test.go:52:	path := filepath.Join(t.TempDir(), "sub", "baseline.json")
internal/agent/rpc/drift_provider_test.go:84:		if err := os.WriteFile(filepath.Join(confDir, "cockpit-site-"+name+".conf"), []byte(content), 0o644); err != nil {
internal/agent/rpc/drift_provider_test.go:91:	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/drift_provider_test.go:122:	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/drift_provider_test.go:169:		path := filepath.Join(stacksDir, rel)
internal/agent/rpc/drift_provider_test.go:181:	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/drift_provider_test.go:265:	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/drift_provider_test.go:267:	p := NewDriftProvider(b, DriftConfig{ConfDir: t.TempDir(), StacksDir: filepath.Join(t.TempDir(), "absent")})
internal/agent/rpc/drift_provider_test.go:308:	path := filepath.Join(t.TempDir(), "baseline.json")
internal/agent/rpc/drift_provider_test.go:333:	sitePath := filepath.Join(confDir, "cockpit-site-web.conf")
internal/agent/rpc/drift_provider_test.go:334:	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/drift_provider_test.go:378:	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/drift_provider_test.go:429:		path := filepath.Join(stacksDir, rel)
internal/agent/rpc/drift_provider_test.go:437:	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/drift_provider_test.go:458:	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/drift_provider_test.go:474:	sitePath := filepath.Join(confDir, "cockpit-site-web.conf")
internal/agent/rpc/drift_provider_test.go:475:	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/drift_provider_test.go:487:	b2 := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/drift_provider_test.go:494:	bigPath := filepath.Join(confDir, "cockpit-site-big.conf")
internal/agent/rpc/drift_provider_test.go:506:	if err := os.MkdirAll(filepath.Join(stacksDir, "web"), 0o700); err != nil {
internal/agent/rpc/drift_provider_test.go:510:		NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json")),
internal/agent/rpc/drift_provider_test.go:543:	sitePath := filepath.Join(confDir, "cockpit-site-legacy.conf")
internal/agent/rpc/drift_provider_test.go:547:	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/drift_provider_test.go:591:	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
internal/agent/rpc/drift_provider_test.go:609:		path := filepath.Join(stacksDir, rel)
internal/agent/rpc/drift_provider_test.go:619:	p2 := NewDriftProvider(NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json")), DriftConfig{StacksDir: stacksDir})
internal/agent/rpc/drift_provider_test.go:632:		NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json")),
internal/agent/rpc/cov_round6_test.go:61:	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644); err != nil {
internal/agent/rpc/cov_round6_test.go:95:// TestCovStartFollowCmdStdoutPipeFail 注入 exec.CommandContext 返回已占用
internal/agent/rpc/cov_round6_test.go:98:	orig := execCommandContext
internal/agent/rpc/cov_round6_test.go:99:	t.Cleanup(func() { execCommandContext = orig })
internal/agent/rpc/cov_round6_test.go:100:	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
internal/agent/rpc/cov_round6_test.go:117:	path := filepath.Join(t.TempDir(), "svc.conf")
internal/agent/rpc/cov_round6_test.go:144:	if err := os.WriteFile(filepath.Join(dir, "app-20260101.tar.gz"), []byte("data"), 0o644); err != nil {
internal/agent/rpc/cov_round6_test.go:161:	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte("x"), 0o644); err != nil {
internal/agent/rpc/cov_round6_test.go:202:	blocker := filepath.Join(dir, "blocker")
internal/agent/rpc/cov_round6_test.go:206:	b := NewDriftBaseline(filepath.Join(blocker, "baseline.json"))
internal/agent/rpc/cov_round6_test.go:216:	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
internal/agent/rpc/cov_round6_test.go:222:	if _, err := os.Stat(filepath.Join(dir, "f", stackReservedDir, "last-task.json")); err == nil {
internal/agent/rpc/cov_round6_test.go:245:	path := filepath.Join(t.TempDir(), "f.txt")
internal/agent/rpc/cov_round6_test.go:261:	path := filepath.Join(t.TempDir(), "svc.conf")
internal/agent/rpc/cov_round6_test.go:278:	if err := os.Symlink(filepath.Join(src, "absent"), filepath.Join(src, "link")); err != nil {
internal/agent/rpc/nas_omv.go:48:	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
internal/agent/rpc/nas_omv.go:75:		map[string]string{"username": t.Username, "password": t.Password})
internal/agent/rpc/backup_provider.go:229:			remoteStatus, remoteErr = p.pushRemote(task, filepath.Join(destDir, file), remoteDest)
internal/agent/rpc/backup_provider.go:323:	out, err := exec.Command(p.rcloneBin, "copy", "--transfers", "2", localPath, remoteDest).CombinedOutput()
internal/agent/rpc/backup_provider.go:368:		killHookGroup(cmd) // 整组杀：CommandContext 只杀 sh，dump 孙进程会变孤儿
internal/agent/rpc/backup_provider.go:406:	local := filepath.Join(filepath.Clean(dir), name)
internal/agent/rpc/backup_provider.go:433:	path := filepath.Join(dir, name)
internal/agent/rpc/backup_provider.go:457:	archive := filepath.Join(dir, name)
internal/agent/rpc/backup_provider.go:520:	f, err := os.Open(filepath.Join(dir, name))
internal/agent/rpc/backup_provider.go:547:		target := filepath.Join(destClean, hdr.Name)
internal/agent/rpc/backup_provider.go:609:	f, err := os.Open(filepath.Join(dir, name))
internal/agent/rpc/backup_provider.go:649:	outPath := filepath.Join(destDir, fileName)
internal/agent/rpc/backup_provider.go:740:			name = filepath.ToSlash(filepath.Join(top, rel))
internal/agent/rpc/backup_provider.go:781:	matches, err := filepath.Glob(filepath.Join(destDir, name+"-*.tar.gz"))
internal/agent/rpc/cov_backup_provider_test.go:80:	os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:166:	if _, err := p.Call("list", map[string]interface{}{"dir": filepath.Join(t.TempDir(), "missing")}); err == nil ||
internal/agent/rpc/cov_backup_provider_test.go:176:	os.WriteFile(filepath.Join(dir, "keep.tar.gz"), []byte("x"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:177:	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:178:	os.Mkdir(filepath.Join(dir, "adir.tar.gz"), 0o755)
internal/agent/rpc/cov_backup_provider_test.go:180:	if err := syscall.Mkfifo(filepath.Join(dir, "pipe.tar.gz"), 0o644); err != nil {
internal/agent/rpc/cov_backup_provider_test.go:205:	os.WriteFile(filepath.Join(dir, "real.tar.gz"), []byte("x"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:216:	os.WriteFile(filepath.Join(dir, "a.tar.gz"), []byte("x"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:238:	os.Mkdir(filepath.Join(d2, "d.tar.gz"), 0o755)
internal/agent/rpc/cov_backup_provider_test.go:240:		"dir": d2, "name": "d.tar.gz", "destDir": filepath.Join(d2, "out"),
internal/agent/rpc/cov_backup_provider_test.go:251:		"dir": dir, "name": "a.tar.gz", "destDir": filepath.Join(t.TempDir(), "out"),
internal/agent/rpc/cov_backup_provider_test.go:261:		"dir": dir, "name": "a.tar.gz", "destDir": filepath.Join(t.TempDir(), "out"),
internal/agent/rpc/cov_backup_provider_test.go:274:	dest := filepath.Join(t.TempDir(), "out")
internal/agent/rpc/cov_backup_provider_test.go:282:	plain := filepath.Join(dir, "plain.tar.gz")
internal/agent/rpc/cov_backup_provider_test.go:289:	blocker := filepath.Join(t.TempDir(), "blocker")
internal/agent/rpc/cov_backup_provider_test.go:291:	good := filepath.Join(dir, "good.tar.gz")
internal/agent/rpc/cov_backup_provider_test.go:293:	if err := p.unpack(task, dir, "good.tar.gz", filepath.Join(blocker, "out")); err == nil ||
internal/agent/rpc/cov_backup_provider_test.go:298:	badTar := filepath.Join(dir, "badtar.tar.gz")
internal/agent/rpc/cov_backup_provider_test.go:318:	arch := filepath.Join(dir, "mixed.tar.gz")
internal/agent/rpc/cov_backup_provider_test.go:337:	link, err := os.Readlink(filepath.Join(dest, "link"))
internal/agent/rpc/cov_backup_provider_test.go:342:	b, err := os.ReadFile(filepath.Join(dest, "ok.txt"))
internal/agent/rpc/cov_backup_provider_test.go:351:	os.WriteFile(filepath.Join(dir, "bad.tar.gz"), []byte("definitely not gzip"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:352:	task := runRestoreAndWait(t, p, dir, "bad.tar.gz", filepath.Join(t.TempDir(), "out"))
internal/agent/rpc/cov_backup_provider_test.go:386:	os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:387:	blocker := filepath.Join(t.TempDir(), "blocker")
internal/agent/rpc/cov_backup_provider_test.go:389:	task := runBackupAndWait(t, p, "daily", []string{src}, filepath.Join(blocker, "out"), 0)
internal/agent/rpc/cov_backup_provider_test.go:409:	task = runBackupAndWait(t, p3, "daily", []string{filepath.Join(t.TempDir(), "missing")}, dest3, 0)
internal/agent/rpc/cov_backup_provider_test.go:414:	leftover, _ := filepath.Glob(filepath.Join(dest3, "*.tar.gz"))
internal/agent/rpc/cov_backup_provider_test.go:443:	os.WriteFile(filepath.Join(src, "a.txt"), []byte("payload"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:464:	if _, err := addSource(tw, io.Discard, filepath.Join(t.TempDir(), "missing")); err == nil {
internal/agent/rpc/cov_backup_provider_test.go:471:		os.WriteFile(filepath.Join(src, "top.txt"), []byte("x"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:472:		sub := filepath.Join(src, "sub")
internal/agent/rpc/cov_backup_provider_test.go:474:		os.WriteFile(filepath.Join(sub, "in.txt"), []byte("y"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:487:		secret := filepath.Join(src2, "secret.txt")
internal/agent/rpc/cov_backup_provider_test.go:488:		os.WriteFile(secret, []byte("y"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:489:		os.Chmod(secret, 0o000)
internal/agent/rpc/cov_backup_provider_test.go:490:		t.Cleanup(func() { os.Chmod(secret, 0o644) })
internal/agent/rpc/cov_backup_provider_test.go:499:	os.WriteFile(filepath.Join(src3, "a.txt"), []byte("x"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:521:	os.WriteFile(filepath.Join(dir, "one.tar.gz"), []byte("x"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:525:	os.Mkdir(filepath.Join(dir, "daily-1.tar.gz"), 0o700)
internal/agent/rpc/cov_backup_provider_test.go:526:	os.WriteFile(filepath.Join(dir, "daily-1.tar.gz", "inner"), []byte("x"), 0o644)
internal/agent/rpc/cov_backup_provider_test.go:533:	os.RemoveAll(filepath.Join(dir, "daily-1.tar.gz"))
internal/agent/rpc/cov_backup_provider_test.go:534:	old := filepath.Join(dir, "daily-old.tar.gz")
internal/agent/rpc/cov_backup_provider_test.go:535:	newF := filepath.Join(dir, "daily-new.tar.gz")
internal/agent/rpc/handlers_test.go:310:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:329:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:337:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:372:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:381:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:390:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:399:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:408:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:417:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:426:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:435:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:444:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:453:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:462:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:471:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:480:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:489:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:498:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:507:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:516:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:525:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:538:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:551:	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")
internal/agent/rpc/handlers_test.go:563:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:572:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:581:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:590:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:599:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:608:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:617:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:626:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:635:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:644:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:653:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:662:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:671:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:680:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:692:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:701:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:710:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:719:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:728:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:737:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:749:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/handlers_test.go:758:	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")
internal/agent/rpc/logs_follow_test.go:16:		cmd := exec.CommandContext(ctx, "sh", "-c", script)
internal/agent/rpc/file_provider_test.go:30:	mustWrite(t, filepath.Join(root, "b.txt"), "x")
internal/agent/rpc/file_provider_test.go:31:	mustWrite(t, filepath.Join(root, "a.txt"), "yy")
internal/agent/rpc/file_provider_test.go:32:	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
internal/agent/rpc/file_provider_test.go:35:	if err := os.Symlink("a.txt", filepath.Join(root, "z.link")); err != nil {
internal/agent/rpc/file_provider_test.go:77:	path := filepath.Join(t.TempDir(), "big.bin")
internal/agent/rpc/file_provider_test.go:122:	path := filepath.Join(t.TempDir(), "f.txt")
internal/agent/rpc/file_provider_test.go:158:		"path": filepath.Join(t.TempDir(), "missing.txt"), "data": "", "truncate": false,
internal/agent/rpc/file_provider_test.go:171:	nested := filepath.Join(t.TempDir(), "a", "b", "c.txt")
internal/agent/rpc/file_provider_test.go:188:	nested := filepath.Join(root, "a", "b", "c")
internal/agent/rpc/file_provider_test.go:201:	deep := filepath.Join(nested, "f.txt")
internal/agent/rpc/file_provider_test.go:203:	if _, err := callFile(t, p, "delete", map[string]interface{}{"path": filepath.Join(root, "a")}); err != nil {
internal/agent/rpc/file_provider_test.go:206:	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
internal/agent/rpc/file_provider_test.go:211:	real := filepath.Join(root, "real.txt")
internal/agent/rpc/file_provider_test.go:213:	link := filepath.Join(root, "link")
internal/agent/rpc/file_provider_test.go:236:	old := filepath.Join(root, "old.txt")
internal/agent/rpc/file_provider_test.go:253:	other := filepath.Join(root, "other.txt")
internal/agent/rpc/file_provider_test.go:270:	dir := filepath.Join(root, "dir")
internal/agent/rpc/file_provider_test.go:274:	link := filepath.Join(root, "link")
internal/agent/rpc/file_provider_test.go:303:		"path": "../secret", "offset": float64(0), "length": float64(10),
internal/agent/rpc/file_provider_test.go:319:	def := filepath.Join(dir, "d.txt")
internal/agent/rpc/file_provider_test.go:331:		path := filepath.Join(dir, fmt.Sprintf("m%o.txt", int(mode)))
internal/agent/rpc/file_provider_test.go:343:	over := filepath.Join(dir, "over.txt")
internal/agent/rpc/file_provider_test.go:358:			"path": filepath.Join(dir, "bad.txt"), "data": base64.StdEncoding.EncodeToString([]byte("x")), "truncate": true, "mode": bad,
internal/agent/rpc/file_provider_test.go:366:	zero := filepath.Join(dir, "z.txt")
internal/agent/rpc/file_provider_test.go:384:		p := filepath.Join(root, rel)
internal/agent/rpc/file_provider_test.go:427:	if !paths["nginx.conf"] || !paths[filepath.Join("app", "settings.py")] {
internal/agent/rpc/file_provider_test.go:489:	big := filepath.Join(t.TempDir(), "many")
internal/agent/rpc/file_provider_test.go:491:		fp := filepath.Join(big, fmt.Sprintf("f%03d.txt", i))
internal/agent/rpc/file_provider_test.go:515:	deep := filepath.Join(root, "a/b/c/d/e/f/g/h/i")
internal/agent/rpc/file_provider_test.go:519:	if err := os.WriteFile(filepath.Join(deep, "x.conf"), []byte("deep needle\n"), 0o644); err != nil {
internal/agent/rpc/file_provider_test.go:522:	if err := os.WriteFile(filepath.Join(root, "top.txt"), []byte("shallow needle\n"), 0o644); err != nil {
internal/agent/rpc/file_provider_test.go:543:	path := filepath.Join(t.TempDir(), "f.txt")
internal/agent/rpc/file_provider_test.go:581:	link := filepath.Join(t.TempDir(), "l.txt")
internal/domain/monitor_test.go:126:	script := filepath.Join(dir, "fake-whois")


[showing lines 1311-1769 of 1769; full output saved to /home/cui/.local/share/opencode/shell/2b220c9cad2a7053c583eada91fc7bca7b5b6399/sh_14f061111001DN7jHr9t92lN8G.out]
[0m$ [0mnl -ba internal/server/server.go | sed -n '1,300p'; nl -ba internal/server/middleware.go | sed -n '1,260p'; nl -ba internal/server/rbac.go | sed -n '1,240p'
1	package server
     2	
     3	import (
     4		"bytes"
     5		"context"
     6		"encoding/json"
     7		"fmt"
     8		"io"
     9		"log"
    10		"net/http"
    11		"os"
    12		"strconv"
    13		"strings"
    14		"sync"
    15		"time"
    16	
    17		"github.com/cuihairu/cockpit/internal/audit"
    18		"github.com/cuihairu/cockpit/internal/auth"
    19		"github.com/cuihairu/cockpit/internal/config"
    20		"github.com/cuihairu/cockpit/internal/dns"
    21		"github.com/cuihairu/cockpit/internal/notification"
    22		"github.com/cuihairu/cockpit/internal/overlay"
    23		"github.com/cuihairu/cockpit/internal/probe"
    24		"github.com/cuihairu/cockpit/internal/protocol"
    25		"github.com/cuihairu/cockpit/internal/proxy/mgr"
    26		"github.com/cuihairu/cockpit/internal/storage"
    27		inventorysync "github.com/cuihairu/cockpit/internal/sync"
    28		"github.com/cuihairu/cockpit/internal/vault"
    29		"github.com/gorilla/websocket"
    30	)
    31	
    32	// CallAgent 超时参数。包级变量仅为测试可注入（cov_* 测试缩短等待），
    33	// 默认值即生产取值，行为不变。
    34	var (
    35		callAgentSendTimeout = 5 * time.Second
    36		callAgentTimeout     = 30 * time.Second
    37	)
    38	
    39	// Server WebSocket 服务器
    40	type Server struct {
    41		addr           string
    42		registry       *Registry
    43		codec          *protocol.Codec
    44		db             *storage.DB
    45		auth           *auth.Service
    46		audit          *audit.Logger
    47		proxyMgr       *mgr.Manager
    48		notifier       *notification.Service
    49		remoteSessions *RemoteSessionManager
    50		ticketMgr      *TicketManager
    51		inventorySync  *inventorysync.Manager
    52		probeRunner    *probe.Runner
    53		dns            dns.Provider
    54		acme           AcmeIssuer
    55		acmeDNSConfig  func() AcmeDNSConfig
    56		overlayZT      *overlay.ZeroTierClient
    57		overlayTS      *overlay.TailscaleClient
    58		cfg            *config.Config
    59		upgrader       websocket.Upgrader
    60	
    61		mu     sync.RWMutex
    62		ctx    context.Context
    63		cancel context.CancelFunc
    64	
    65		// bgWG 跟踪后台清扫循环与在途后台任务（goBg 启动），Shutdown 在
    66		// cancel 之后先等它们全部退出再关 DB：循环只在自己 select 的
    67		// ctx.Done 分支退出，mid-sweep 时被 cancel 会继续跑完本轮，不等待
    68		// 就会打到已关闭的库（"database is closed"），测试侧还会与
    69		// t.TempDir 的 RemoveAll 竞态（清扫写的 journal/子目录在删除过程中
    70		// 出现 → "directory not empty"）。
    71		// bgMu 保护 bgWG.Add 与 bgClosed：关闭已开始后新任务不再计数
    72		// （避免 Add 与 Wait 并发 panic），任务照跑但不阻塞收尾。
    73		bgWG     sync.WaitGroup
    74		bgMu     sync.Mutex
    75		bgClosed bool
    76	
    77		// backupTrackWG 跟踪在途的备份任务跟踪 goroutine（测试注入
    78		// backupTrackInterval 前等待其退出，保证恢复默认值不构成数据竞争）
    79		backupTrackWG sync.WaitGroup
    80	
    81		// vaultProvider 密码箱 provider（vault-design.md V7；NAS/外部端点
    82		// 凭据接入点逐步改走此实例）
    83		vaultProvider vault.Provider
    84	}
    85	
    86	// logFatalf 注入点：NewServer 启动防御（坏库路径/弱密码/生产密钥校验）以
    87	// log.Fatal 终止进程，测试以 panic 哨兵截获后 recover 断言。
    88	var logFatalf = log.Fatalf
    89	
    90	// storageValidateKey 注入点：生产密钥校验依赖环境，测试可控其失败。
    91	var storageValidateKey = storage.ValidateKey
    92	
    93	// goBg 在 bgWG 里起一个后台任务。关闭流程（Shutdown）等它全部退出后才
    94	// 关 DB，所以凡写库/写文件的在途任务都从这里启动。关闭已开始后照常
    95	// 执行但不再计数（bgClosed），既不 panic 也不拖住收尾。
    96	func (s *Server) goBg(fn func()) {
    97		s.bgMu.Lock()
    98		track := !s.bgClosed
    99		if track {
   100			s.bgWG.Add(1)
   101		}
   102		s.bgMu.Unlock()
   103		go func() {
   104			if track {
   105				defer s.bgWG.Done()
   106			}
   107			fn()
   108		}()
   109	}
   110	
   111	// NewServer 创建新服务器
   112	func NewServer(cfg *config.Config) *Server {
   113		cfg = config.Normalize(cfg)
   114		applyAgentExpireEnv(cfg)
   115		ctx, cancel := context.WithCancel(context.Background())
   116	
   117		// 打开数据库（config.Normalize 已保证 Path 非空）
   118		db, err := storage.Open(storage.Config{Path: cfg.Database.Path})
   119		if err != nil {
   120			logFatalf("Failed to open database: %v", err)
   121		}
   122	
   123		// 审计器（密码箱 provider 钩子与 Server 共用同一实例）
   124		auditLogger := audit.NewLogger(db)
   125	
   126		// 密码箱 provider（vault-design.md V4/V7）：默认 local，未知 provider 启动即失败
   127		vaultProvider, err := initVault(cfg.Vault, db, auditLogger)
   128		if err != nil {
   129			logFatalf("Failed to init vault: %v", err)
   130		}
   131	
   132		// vault secret 引用解析（vault-design.md V6 第二波）：yaml 里的
   133		// vault:<path> 一次性换成明文（只进内存 config）；失败启动即终止
   134		if err := applyVaultSecretRefs(cfg, vaultProvider); err != nil {
   135			logFatalf("Failed to resolve vault secret refs: %v", err)
   136		}
   137	
   138		authService := auth.NewService(db, auth.Options{
   139			Secret:     cfg.JWT.Secret,
   140			Expiration: cfg.JWT.Expiration,
   141		})
   142	
   143		// 初始化多渠道通知服务（herald/ntfy/webhook/telegram；未启用时为空实现）
   144		notifier := notification.NewService(cfg.Notification)
   145	
   146		// 检查 TOTP 加密密钥
   147		if storage.IsUsingDefaultKey() {
   148			log.Println("WARNING: Using default TOTP encryption key. This is insecure for production!")
   149			log.Println("Please set TOTP_ENCRYPTION_KEY environment variable with a strong random key.")
   150		}
   151		// 在生产模式下强制验证密钥（可以通过环境变量 PRODUCTION=true 启用）
   152		if os.Getenv("PRODUCTION") == "true" {
   153			if err := storageValidateKey(); err != nil {
   154				logFatalf("SECURITY ERROR: %v", err)
   155			}
   156		}
   157	
   158		// 构造服务器地址
   159		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
   160	
   161		return &Server{
   162			addr:           addr,
   163			registry:       NewRegistry(),
   164			codec:          protocol.NewCodec(),
   165			db:             db,
   166			auth:           authService,
   167			audit:          auditLogger,
   168			vaultProvider:  vaultProvider,
   169			proxyMgr:       mgr.NewManager(nil, db), // 将在 Start 中设置 ServerInterface
   170			notifier:       notifier,
   171			remoteSessions: NewRemoteSessionManager(),
   172			ticketMgr:      NewTicketManager(),
   173			cfg:            cfg,
   174			upgrader: websocket.Upgrader{
   175				CheckOrigin:     isOriginAllowed,
   176				ReadBufferSize:  1024,
   177				WriteBufferSize: 1024,
   178			},
   179			ctx:    ctx,
   180			cancel: cancel,
   181		}
   182	}
   183	
   184	// Start 启动服务器
   185	func (s *Server) Start() error {
   186		// 设置邮件配置
   187		auth.SetEmailConfig(s.cfg.Email)
   188	
   189		// 初始化管理员用户
   190		adminUser := getEnv("ADMIN_USERNAME", "admin")
   191		adminPass := os.Getenv("ADMIN_PASSWORD")
   192	
   193		// 强制要求设置密码
   194		if adminPass == "" {
   195			logFatalf("%s", "SECURITY ERROR: ADMIN_PASSWORD environment variable is required for production use. Please set a strong password and restart.")
   196		}
   197	
   198		// 验证密码强度
   199		if len(adminPass) < 8 {
   200			logFatalf("%s", "SECURITY ERROR: ADMIN_PASSWORD must be at least 8 characters long")
   201		}
   202	
   203		// 检查是否是常见的弱密码
   204		weakPasswords := []string{"password", "12345678", "admin123", "qwerty123", "abcdef12"}
   205		for _, weak := range weakPasswords {
   206			if adminPass == weak {
   207				logFatalf("SECURITY ERROR: ADMIN_PASSWORD is too weak (cannot use common password '%s')", weak)
   208			}
   209		}
   210	
   211		if err := auth.InitAdmin(s.db, adminUser, adminPass); err != nil {
   212			log.Printf("Warning: Failed to init admin user: %v", err)
   213		} else {
   214			log.Printf("Admin user initialized: %s", adminUser)
   215		}
   216	
   217		mux := http.NewServeMux()
   218	
   219		// 设置代理管理器的 ServerInterface
   220		s.proxyMgr = mgr.NewManager(s, s.db)
   221	
   222		// 启动代理管理器（启动已启用的代理）
   223		if err := s.proxyMgr.Start(); err != nil {
   224			log.Printf("Failed to start proxy manager: %v", err)
   225		}
   226	
   227		if err := s.startInventorySync(); err != nil {
   228			return fmt.Errorf("start inventory sync: %w", err)
   229		}
   230	
   231		// 启动自动健康探测（5 分钟间隔）
   232		s.startProbeRunner()
   233	
   234		// DNS 管理 client（凭据未配置时为 nil，API 统一 503 引导；provider 按
   235		// dns.provider 分派，与 ACME 共用同一套键，见 dns-design.md D11）
   236		s.dns = dns.New(s.cfg.DNS.Provider,
   237			s.cfg.DNS.Cloudflare.APIToken, s.cfg.DNS.DNSPod.LoginToken,
   238			s.cfg.DNS.AliDNS.AccessKey, s.cfg.DNS.AliDNS.SecretKey)
   239		if s.dns != nil {
   240			log.Print("DNS provider enabled: ", s.dnsProviderName())
   241		}
   242	
   243		// ACME 签发器（lego DNS-01，provider 按 dns.provider 分派，见 acme-design.md D4/D13）
   244		s.acmeDNSConfig = func() AcmeDNSConfig { return newAcmeDNSConfig(s.cfg) }
   245		s.acme = NewLegoIssuer(s.db, s.acmeDNSConfig)
   246	
   247		// 组网云管理面 client（凭据未配置时为 nil，API 统一 503 引导，D11/D12）
   248		if s.cfg.Overlay != nil {
   249			if s.cfg.Overlay.ZeroTier != nil && s.cfg.Overlay.ZeroTier.APIToken != "" {
   250				s.overlayZT = overlay.NewZeroTier(s.cfg.Overlay.ZeroTier.APIToken)
   251				log.Print("Overlay cloud management enabled: zerotier")
   252			}
   253			if s.cfg.Overlay.Tailscale != nil && s.cfg.Overlay.Tailscale.APIToken != "" {
   254				s.overlayTS = overlay.NewTailscale(s.cfg.Overlay.Tailscale.APIToken, s.cfg.Overlay.Tailscale.Tailnet)
   255				log.Print("Overlay cloud management enabled: tailscale")
   256			}
   257		}
   258	
   259		// 注册所有路由
   260		s.registerRoutes(mux)
   261	
   262		// 预检请求必须先于认证处理，否则跨域 OPTIONS 会被 JWT 中间件拒绝。
   263		// RBAC 在 Audit 内层：403 也进审计链（rbac-design.md 笔 2）
   264		handler := s.CORSMiddleware(s.AuditMiddleware(s.RBACMiddleware(mux)))
   265	
   266		server := &http.Server{
   267			Addr:    s.addr,
   268			Handler: handler,
   269		}
   270	
   271		log.Printf("Server starting on %s", s.addr)
   272		log.Printf("Web UI: http://%s", s.addr)
   273	
   274		// 启动清理协程
   275		s.goBg(s.cleanupLoop)
   276	
   277		// 启动警告检查协程
   278		s.goBg(s.alertCheckLoop)
   279		s.goBg(s.driftScanLoop)         // 漂移定时巡检（见 drift-design.md M2）
   280		s.goBg(s.smartScanLoop)         // SMART 磁盘健康巡检（见 disk-health-design.md D8）
   281		s.goBg(s.ddnsScanLoop)          // DDNS 定时同步（见 ddns-design.md D6）
   282		s.goBg(s.acmeScanLoop)          // ACME 证书自动续期巡检（见 acme-design.md D7）
   283		s.goBg(s.serviceHealthScanLoop) // 服务健康探针归集（见 service-health-design.md D8）
   284		s.goBg(s.nasScanLoop)           // NAS 存储巡检（见 nas-design.md D5）
   285		// 启动系统指标清理协程
   286		s.goBg(s.metricsCleanupLoop)
   287		// server 重启孤儿清扫：无推进者的 pending/running Job 与 running run
   288		// 收敛为 failed（先于 HTTP 接流，见 orphan_run.go）
   289		s.recoverOrphanRuns()
   290		// 启动备份调度循环
   291		s.startBackupLoop()
   292		// 启动 server 自身数据库备份循环（见 server-backup-design.md）
   293		s.goBg(s.serverBackupLoop)
   294	
   295		return server.ListenAndServe()
   296	}
   297	
   298	// registerRoutes 注册所有 HTTP 路由
   299	func (s *Server) registerRoutes(mux *http.ServeMux) {
   300		// 注册审计日志 API
     1	package server
     2	
     3	import (
     4		"github.com/cuihairu/cockpit/internal/audit"
     5		"github.com/cuihairu/cockpit/internal/auth"
     6		"net/http"
     7		"os"
     8		"strings"
     9	)
    10	
    11	// responseWriter 响应写入器
    12	type responseWriter struct {
    13		http.ResponseWriter
    14		statusCode int
    15		written    bool
    16	}
    17	
    18	func (r *responseWriter) WriteHeader(code int) {
    19		if !r.written {
    20			r.statusCode = code
    21			r.written = true
    22			r.ResponseWriter.WriteHeader(code)
    23		}
    24	}
    25	
    26	func (r *responseWriter) Write(b []byte) (int, error) {
    27		if !r.written {
    28			r.WriteHeader(http.StatusOK)
    29		}
    30		return r.ResponseWriter.Write(b)
    31	}
    32	
    33	// Flush 透传底层 Flusher。日志尾随（NDJSON 逐帧推送）经审计链时若包装器
    34	// 丢掉 http.Flusher，handleAgentLogsFollow 的 w.(http.Flusher) 断言必失败，
    35	// 流式面整体 500「streaming unsupported」——与 /ws 的 Hijacker 教训同类，
    36	// 但这里用接口透传而非跳过审计（follow 起停仍应入审计链）
    37	func (r *responseWriter) Flush() {
    38		if f, ok := r.ResponseWriter.(http.Flusher); ok {
    39			f.Flush()
    40		}
    41	}
    42	
    43	// CORSMiddleware 统一处理 API 跨域响应和预检请求。
    44	func (s *Server) CORSMiddleware(next http.Handler) http.Handler {
    45		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    46			if !strings.HasPrefix(r.URL.Path, "/api/") {
    47				next.ServeHTTP(w, r)
    48				return
    49			}
    50	
    51			setCORSHeaders(w)
    52			if r.Method == http.MethodOptions {
    53				w.WriteHeader(http.StatusOK)
    54				return
    55			}
    56	
    57			next.ServeHTTP(w, r)
    58		})
    59	}
    60	
    61	func setCORSHeaders(w http.ResponseWriter) {
    62		allowed := os.Getenv("ALLOWED_ORIGINS")
    63		if allowed != "" {
    64			w.Header().Set("Access-Control-Allow-Origin", allowed)
    65		}
    66		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
    67		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
    68	}
    69	
    70	// AuditMiddleware 审计日志中间件
    71	func (s *Server) AuditMiddleware(next http.Handler) http.Handler {
    72		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    73			// WebSocket 升级需要 http.Hijacker 接口；包装器会破坏该接口，
    74			// 导致 gorilla/websocket 报 "response does not implement http.Hijacker"。
    75			// 因此 WebSocket 路径直接透传，跳过审计（WS 会话由专门的远控审计覆盖）。
    76			if r.URL.Path == "/ws" ||
    77				r.URL.Path == "/api/remote/terminal" ||
    78				r.URL.Path == "/api/remote/guacamole" {
    79				next.ServeHTTP(w, r)
    80				return
    81			}
    82	
    83			// 保存原始响应写入器
    84			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
    85	
    86			// D14：可选认证——有效 Bearer 先把用户塞进 ctx。RBAC 判定层
    87			// 403 短路时不经过内层 auth 挂点，这里保证审计能记到「谁」被拒；
    88			// 内层 auth 幂等（重新解析覆盖，语义不变）
    89			if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
    90				if claims, err := s.authService().ValidateToken(strings.TrimPrefix(header, "Bearer ")); err == nil {
    91					ctx := auth.ContextWithUser(r.Context(), claims.UserID, claims.Username, claims.Role)
    92					r = r.WithContext(ctx)
    93				}
    94			}
    95	
    96			// 处理请求（在处理过程中，auth 中间件会设置用户上下文）
    97			next.ServeHTTP(wrapped, r)
    98	
    99			// 处理完成后，尝试获取用户信息
   100			// 此时 auth 中间件已经执行，context 应该包含用户信息
   101			username := "anonymous"
   102			userID := ""
   103			if userInfo, ok := auth.GetUserFromContext(r); ok {
   104				username = userInfo.Username
   105				userID = userInfo.UserID
   106			}
   107	
   108			// 记录审计日志（只记录需要审计的路径）
   109			if s.shouldAudit(r.Method, r.URL.Path, wrapped.statusCode) {
   110				action := s.getActionFromMethod(r.Method)
   111				resource := s.getResourceFromPath(r.URL.Path)
   112	
   113				details := map[string]interface{}{
   114					"method":      r.Method,
   115					"path":        r.URL.Path,
   116					"query":       r.URL.RawQuery,
   117					"status_code": wrapped.statusCode,
   118				}
   119	
   120				s.audit.Log(&audit.LogEntry{
   121					UserID:     userID,
   122					Username:   username,
   123					Action:     action,
   124					Resource:   resource,
   125					ResourceID: s.getResourceIDFromPath(r.URL.Path),
   126					Details:    details,
   127					IP:         s.getClientIP(r),
   128					UserAgent:  r.UserAgent(),
   129					Status:     s.getStatusFromStatusCode(wrapped.statusCode),
   130				})
   131			}
   132		})
   133	}
   134	
   135	// shouldAudit 判断是否需要审计。statusCode 是刚写出的响应码：D14——
   136	// API 的 403（越权拒绝，含 RBAC 判定层出的）恒记录，GET 也记，用于
   137	// 发现权限配置不足与探测行为
   138	func (s *Server) shouldAudit(method, path string, statusCode int) bool {
   139		// 不记录健康检查、静态资源等
   140		if path == "/health" || path == "/api/status" {
   141			return false
   142		}
   143	
   144		// API 403 恒记录（越权拒绝）
   145		if statusCode == http.StatusForbidden && strings.HasPrefix(path, "/api/") {
   146			return true
   147		}
   148	
   149		// 不记录 GET 请求（查询操作）
   150		if method == "GET" && !strings.HasPrefix(path, "/api/admin") {
   151			return false
   152		}
   153	
   154		// 记录所有 API 请求
   155		return strings.HasPrefix(path, "/api/")
   156	}
   157	
   158	// getActionFromMethod 从 HTTP 方法获取操作类型
   159	func (s *Server) getActionFromMethod(method string) string {
   160		switch method {
   161		case "GET":
   162			return audit.ActionView
   163		case "POST":
   164			return audit.ActionCreate
   165		case "PUT", "PATCH":
   166			return audit.ActionUpdate
   167		case "DELETE":
   168			return audit.ActionDelete
   169		default:
   170			return "unknown"
   171		}
   172	}
   173	
   174	// getResourceFromPath 从路径获取资源类型
   175	func (s *Server) getResourceFromPath(path string) string {
   176		if !strings.HasPrefix(path, "/api/") {
   177			return "unknown"
   178		}
   179	
   180		// 移除 /api/ 前缀（strings.Split 结果至少含一个元素，无需判空）
   181		return strings.Split(strings.TrimPrefix(path, "/api/"), "/")[0]
   182	}
   183	
   184	// getResourceIDFromPath 从路径获取资源ID
   185	func (s *Server) getResourceIDFromPath(path string) string {
   186		parts := strings.Split(path, "/")
   187		return parts[len(parts)-1]
   188	}
   189	
   190	// getClientIP 获取客户端IP
   191	func (s *Server) getClientIP(r *http.Request) string {
   192		// 检查 X-Forwarded-For 头
   193		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
   194			// 取第一个IP
   195			if idx := strings.Index(xff, ","); idx != -1 {
   196				return strings.TrimSpace(xff[:idx])
   197			}
   198			return xff
   199		}
   200	
   201		// 检查 X-Real-IP 头
   202		if xri := r.Header.Get("X-Real-IP"); xri != "" {
   203			return xri
   204		}
   205	
   206		// 使用 RemoteAddr
   207		return r.RemoteAddr
   208	}
   209	
   210	// getStatusFromStatusCode 从状态码获取状态
   211	func (s *Server) getStatusFromStatusCode(statusCode int) string {
   212		if statusCode >= 200 && statusCode < 300 {
   213			return audit.StatusSuccess
   214		}
   215		return audit.StatusFailure
   216	}
     1	package server
     2	
     3	// rbac.go 路由权限判定层（rbac-design.md 笔 2）：全局中间件形态，挂在
     4	// AuditMiddleware 内层（403 也被审计记录）、各 auth 挂点外层——自身解析
     5	// Bearer token 取角色，无/坏 token 放行给内层 auth 出 401（认证优先于
     6	// 鉴权）。每请求查库（D6），角色缺失 fail-closed 403；隐含规则同 resource
     7	// 下 write ⊇ read、admin ⊇ write。
     8	
     9	import (
    10		"net/http"
    11		"strings"
    12	
    13		"github.com/cuihairu/cockpit/internal/auth"
    14	)
    15	
    16	// resourceRule path 前缀 → resource；fixed 非空时忽略 method 推导恒用该
    17	// action：audit/logs 的动作全是读（导出/检索也是 POST）；terminal 只有
    18	// write（GET 列表同样按 write 门禁，D11 语义）。
    19	type resourceRule struct {
    20		prefix string
    21		res    string
    22		fixed  string
    23	}
    24	
    25	// resourceRules 有序：长前缀在前，先命中先赢。子路径归并决策：
    26	// probe/notification → alerts（拨测配置含告警阈值）、metrics → inventory
    27	// （基础设施观测）、proxies → proxy、server-backups → backup、
    28	// remote/desktop/vnc → terminal。
    29	var resourceRules = []resourceRule{
    30		{"/api/docker", "docker", ""},
    31		{"/api/stacks", "stack", ""},
    32		{"/api/backups", "backup", ""},
    33		{"/api/server-backups", "backup", ""},
    34		{"/api/probe", "alerts", ""},
    35		{"/api/notification", "alerts", ""},
    36		{"/api/proxies", "proxy", ""},
    37		{"/api/metrics", "inventory", ""},
    38		{"/api/admin/audit", "audit", "read"},
    39		{"/api/remote", "terminal", "write"},
    40		{"/api/recordings", "recordings", ""},
    41		{"/api/drift/config", "drift", ""},
    42		{"/api/inventory/consistency", "inventory", ""},
    43		{"/api/smart/config", "smart", ""},
    44		{"/api/nas/config", "nas", ""},
    45		{"/api/logs/search", "logs", "read"},
    46		{"/api/dns", "dns", ""},
    47		{"/api/domains", "dns", ""},
    48		{"/api/overlay/cloud", "overlay", ""},
    49		{"/api/ddns", "ddns", ""},
    50		{"/api/acme", "acme", ""},
    51		{"/api/resources/", "inventory", ""},
    52		{"/api/alerts", "alerts", ""},
    53		{"/api/agent-tags", "inventory", ""},    // 标签挂服务器，与 /agents/{id}/tags 同归 inventory
    54		{"/api/jobs", "jobs", ""},               // 执行 Job 路由层校验（jobs-design D8：GET=台账 read、POST=执行 write）
    55		{"/api/approvals", "approvals", ""},     // 审批门（workflow-design M2d：GET=队列 read、approve/reject=write）
    56		{"/api/workflows", "workflows", ""},     // Workflow 编排（workflow-design W9：定义 CRUD/run/cancel；GET=read、写=write）
    57		{"/api/workflow-runs", "workflows", ""}, // run 详情/取消同归 workflows 面
    58		{"/api/agents", "inventory", ""},
    59	}
    60	
    61	// agentSubResources /api/agents/{id}/{sub}/... 的 sub → resource
    62	// （裸 /api/agents/{id} 详情走 inventory）
    63	var agentSubResources = map[string]string{
    64		"files/":    "files",
    65		"proxy/":    "proxy",
    66		"cron/":     "cron",
    67		"services/": "services",
    68		"nas/":      "nas",
    69		"drift/":    "drift",
    70		"domains":   "dns", // D7 只读清单/片段
    71		"overlay/":  "overlay",
    72		"smart/":    "smart",
    73		"firewall/": "firewall",
    74	}
    75	
    76	// requiredPerms 返回 path+method 所需权限点与是否归 RBAC 管：
    77	// governed=false（非 /api/ 或未登记路径）放行，未登记路径由 handler 404
    78	// 兜底；perms 空切片 = 登录即可（/me、/status、认证流程）。
    79	func requiredPerms(path, method string) (perms []string, governed bool) {
    80		if !strings.HasPrefix(path, "/api/") {
    81			return nil, false
    82		}
    83		switch {
    84		case path == "/api/status",
    85			path == "/api/me" || strings.HasPrefix(path, "/api/me/"),
    86			strings.HasPrefix(path, "/api/auth/"):
    87			return nil, true
    88		case path == "/api/settings":
    89			return []string{"settings:admin"}, true
    90		case path == "/api/users" || strings.HasPrefix(path, "/api/users/"):
    91			return []string{"users:admin"}, true
    92		case path == "/api/roles" || strings.HasPrefix(path, "/api/roles/"):
    93			// 角色管理面整体仅 roles:admin（含 GET——roles 只有 admin action）
    94			return []string{"roles:admin"}, true
    95		}
    96		// acme 签发/部署归 acme:admin（D3 约定：签发/吊销是高危动作）
    97		if method == http.MethodPost && strings.HasPrefix(path, "/api/acme/") &&
    98			(strings.HasSuffix(path, "/issue") || strings.HasSuffix(path, "/deploy")) {
    99			return []string{"acme:admin"}, true
   100		}
   101		// 域名绑定写操作同时要 dns 与 proxy 写权限（domain-binding D9：
   102		// 联动了谁就要谁的权限；读沿用 dns:read）
   103		if path == "/api/domains" || strings.HasPrefix(path, "/api/domains/") {
   104			if !isReadMethod(method) {
   105				return []string{"dns:write", "proxy:write"}, true
   106			}
   107		}
   108		if rest, ok := strings.CutPrefix(path, "/api/agents/"); ok && rest != "" {
   109			// agent 密钥重置是管理动作
   110			if strings.HasSuffix(rest, "/secret") {
   111				return []string{"inventory:write"}, true
   112			}
   113			// overlay 本机侧写操作（join/leave/service）归 overlay:admin
   114			// （M3 D28：能把主机踢出网络，破坏半径高于云管理面 write，
   115			// 需显式授予，operator 不含此档）；读（status/daemon）仍 overlay:read
   116			if method == http.MethodPost && strings.Contains(rest, "/overlay/") &&
   117				(strings.HasSuffix(rest, "/join") || strings.HasSuffix(rest, "/leave") ||
   118					strings.Contains(rest, "/overlay/service")) {
   119				return []string{"overlay:admin"}, true
   120			}
   121			// 服务健康探针 /agents/{id}/health[/...] 归 services 面（策略层
   122			// 不另立权限点，service-health-design.md D10；注意裸 /health 无
   123			// 尾斜杠，agentSubResources 的 "/health/" 前缀匹配不到）
   124			if strings.HasSuffix(rest, "/health") || strings.Contains(rest, "/health/") {
   125				return actionPerm("services", method, ""), true
   126			}
   127			// agent 侧日志查询/尾随是读操作——POST 也归 logs:read（文件头约定
   128			// 「audit/logs 的动作全是读（导出/检索也是 POST）」，与 /api/logs/search
   129			// 同口径；且 logs 资源只有 read 档，按 method 泛化推导会把
   130			// query/follow 卡成 logs:write——合法权限集合里不存在该点，任何角色
   131			// （含 admin）都过不了，端点整体不可达）
   132			if strings.Contains(rest, "/logs/") {
   133				return actionPerm("logs", method, "read"), true
   134			}
   135			for sub, res := range agentSubResources {
   136				if strings.Contains(rest, "/"+sub) {
   137					return actionPerm(res, method, ""), true
   138				}
   139			}
   140			return actionPerm("inventory", method, ""), true
   141		}
   142		for _, rule := range resourceRules {
   143			if strings.HasPrefix(path, rule.prefix) {
   144				return actionPerm(rule.res, method, rule.fixed), true
   145			}
   146		}
   147		return nil, false
   148	}
   149	
   150	func isReadMethod(method string) bool {
   151		return method == http.MethodGet || method == http.MethodHead
   152	}
   153	
   154	func actionPerm(res, method, fixed string) []string {
   155		action := fixed
   156		if action == "" {
   157			if isReadMethod(method) {
   158				action = "read"
   159			} else {
   160				action = "write"
   161			}
   162		}
   163		return []string{res + ":" + action}
   164	}
   165	
   166	// roleCovers 权限清单是否覆盖需求（隐含规则见文件头）
   167	func roleCovers(have []string, need string) bool {
   168		nRes, nAct, ok := strings.Cut(need, ":")
   169		if !ok {
   170			return false
   171		}
   172		rank := map[string]int{"read": 1, "write": 2, "admin": 3}
   173		want := rank[nAct]
   174		if want == 0 {
   175			return false
   176		}
   177		for _, p := range have {
   178			hRes, hAct, _ := strings.Cut(p, ":")
   179			if hRes == nRes && rank[hAct] >= want {
   180				return true
   181			}
   182		}
   183		return false
   184	}
   185	
   186	// userHasPerm 当前请求用户（context）是否持有权限点（每请求查库）。
   187	// RBAC 拦截之外的细粒度判定用（如改密码免验旧密码的条件）。
   188	func (s *Server) userHasPerm(r *http.Request, perm string) bool {
   189		user, ok := auth.GetUserFromContext(r)
   190		if !ok {
   191			return false
   192		}
   193		role, err := s.db.GetRole(user.Role)
   194		if err != nil {
   195			return false
   196		}
   197		return roleCovers(role.Permissions, perm)
   198	}
   199	
   200	// roleCoversPerm 角色名是否覆盖权限点（角色在角色表存在且权限足够）。
   201	// D13「有效 admin」判定用：角色缺失（幽灵角色 fail-closed）不算。
   202	func (s *Server) roleCoversPerm(roleName, perm string) bool {
   203		role, err := s.db.GetRole(roleName)
   204		if err != nil {
   205			return false
   206		}
   207		return roleCovers(role.Permissions, perm)
   208	}
   209	
   210	// countEffectiveAdmins 有效 admin 用户数：角色在角色表存在且覆盖
   211	// users:admin。skipUserID/skipRoleName 把目标用户/角色从计数中剔除——
   212	// D13 判断「这次变更之后是否还有 admin」。
   213	func (s *Server) countEffectiveAdmins(skipUserID, skipRoleName string) (int64, error) {
   214		roles, err := s.db.ListRoles()
   215		if err != nil {
   216			return 0, err
   217		}
   218		names := make([]string, 0, len(roles))
   219		for _, r := range roles {
   220			if r.Name == skipRoleName {
   221				continue
   222			}
   223			if roleCovers(r.Permissions, "users:admin") {
   224				names = append(names, r.Name)
   225			}
   226		}
   227		if len(names) == 0 {
   228			return 0, nil
   229		}
   230		return s.db.CountUsersByRoles(names, skipUserID)
   231	}
   232	
   233	// RBACMiddleware 全局权限判定（AuditMiddleware 内层：403 也进审计链）
   234	func (s *Server) RBACMiddleware(next http.Handler) http.Handler {
   235		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
   236			perms, governed := requiredPerms(r.URL.Path, r.Method)
   237			if governed && len(perms) > 0 && !s.permitted(w, r, perms) {
   238				return
   239			}
   240			next.ServeHTTP(w, r)
[0m$ [0mnl -ba internal/auth/jwt.go | sed -n '1,240p'; nl -ba internal/auth/middleware.go | sed -n '1,240p'; nl -ba internal/server/api.go | sed -n '1,340p'
1	package auth
     2	
     3	import (
     4		"crypto/rand"
     5		"encoding/hex"
     6		"log"
     7		"os"
     8		"sync"
     9		"time"
    10	
    11		"github.com/cuihairu/cockpit/internal/storage"
    12		"github.com/golang-jwt/jwt/v5"
    13	)
    14	
    15	// randRead 注入点：go1.26 crypto/rand.Read 不会失败，错误分支仅供测试覆盖。
    16	var randRead = rand.Read
    17	
    18	var (
    19		// defaultsOnce 保护包级默认密钥/服务的惰性初始化。
    20		// 旧实现用 init()：任何 import 本包的二进制（含根本不发 token 的
    21		// `cockpit sync` CLI）启动即跑 resolveSecret，缺 JWT_SECRET 就打在
    22		// 场景下纯噪音的 WARNING。现在 import 期零副作用，密钥解析与警告
    23		// 推迟到首次真正用到默认服务时发生。
    24		defaultsOnce   sync.Once
    25		jwtSecret      []byte
    26		jwtExpiration  time.Duration = 24 * time.Hour // 默认 24 小时
    27		defaultService *Service
    28	)
    29	
    30	// ensureDefaults 首次使用时解析 JWT_SECRET 并构建默认服务。
    31	// 语义与改前 init() 逐条一致：env 优先；缺失则随机密钥 + WARNING；
    32	// randRead 失败回退 "change-me"。sync.Once 保证进程内密钥恒定且并发
    33	// 首触发无竞态。注意本函数只影响包级默认服务；服务端实际用的
    34	// NewService 实例一直是独立构造的，行为不变。
    35	func ensureDefaults() {
    36		defaultsOnce.Do(func() {
    37			jwtSecret = resolveSecret(os.Getenv("JWT_SECRET"))
    38			defaultService = &Service{
    39				secret:     cloneBytes(jwtSecret),
    40				expiration: jwtExpiration,
    41			}
    42		})
    43	}
    44	
    45	// Options Auth 服务配置。
    46	type Options struct {
    47		Secret     string
    48		Expiration time.Duration
    49	}
    50	
    51	// Service 封装认证依赖，避免 Server 运行期依赖包级全局状态。
    52	type Service struct {
    53		db         *storage.DB
    54		mu         sync.RWMutex
    55		secret     []byte
    56		expiration time.Duration
    57	}
    58	
    59	// NewService 创建认证服务实例。
    60	func NewService(db *storage.DB, opts Options) *Service {
    61		expiration := opts.Expiration
    62		if expiration <= 0 {
    63			expiration = 24 * time.Hour
    64		}
    65		return &Service{
    66			db:         db,
    67			secret:     resolveSecret(opts.Secret),
    68			expiration: expiration,
    69		}
    70	}
    71	
    72	// SetDB 设置认证服务使用的数据库。
    73	func (s *Service) SetDB(db *storage.DB) {
    74		s.mu.Lock()
    75		defer s.mu.Unlock()
    76		s.db = db
    77	}
    78	
    79	// SetSecret 设置认证服务 JWT 密钥。
    80	func (s *Service) SetSecret(secret string) {
    81		if secret == "" {
    82			return
    83		}
    84		s.mu.Lock()
    85		defer s.mu.Unlock()
    86		s.secret = []byte(secret)
    87	}
    88	
    89	// SetExpiration 设置认证服务 JWT 过期时间。
    90	func (s *Service) SetExpiration(expiration time.Duration) {
    91		if expiration <= 0 {
    92			return
    93		}
    94		s.mu.Lock()
    95		defer s.mu.Unlock()
    96		s.expiration = expiration
    97	}
    98	
    99	// SetSecret 设置 JWT 密钥
   100	func SetSecret(secret string) {
   101		if secret != "" {
   102			ensureDefaults()
   103			jwtSecret = []byte(secret)
   104			defaultService.SetSecret(secret)
   105		}
   106	}
   107	
   108	// SetExpiration 设置 JWT token 过期时间
   109	func SetExpiration(expiration time.Duration) {
   110		if expiration > 0 {
   111			ensureDefaults()
   112			jwtExpiration = expiration
   113			defaultService.SetExpiration(expiration)
   114		}
   115	}
   116	
   117	// SecretFingerprint 返回当前 secret 的指纹（用于诊断）
   118	func SecretFingerprint() string {
   119		ensureDefaults()
   120		if len(jwtSecret) < 4 {
   121			return hex.EncodeToString(jwtSecret)
   122		}
   123		return hex.EncodeToString(jwtSecret[:4])
   124	}
   125	
   126	// Claims JWT 声明
   127	type Claims struct {
   128		UserID   string `json:"user_id"`
   129		Username string `json:"username"`
   130		Role     string `json:"role"`
   131		jwt.RegisteredClaims
   132	}
   133	
   134	// GenerateToken 生成 JWT token
   135	func GenerateToken(userID, username, role string) (string, error) {
   136		ensureDefaults()
   137		return defaultService.GenerateToken(userID, username, role)
   138	}
   139	
   140	// GenerateToken 生成 JWT token
   141	func (s *Service) GenerateToken(userID, username, role string) (string, error) {
   142		secret, expiration := s.jwtConfig()
   143		claims := Claims{
   144			UserID:   userID,
   145			Username: username,
   146			Role:     role,
   147			RegisteredClaims: jwt.RegisteredClaims{
   148				ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiration)),
   149				IssuedAt:  jwt.NewNumericDate(time.Now()),
   150				NotBefore: jwt.NewNumericDate(time.Now()),
   151			},
   152		}
   153	
   154		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
   155		return token.SignedString(secret)
   156	}
   157	
   158	// ValidateToken 验证 JWT token
   159	func ValidateToken(tokenString string) (*Claims, error) {
   160		ensureDefaults()
   161		return defaultService.ValidateToken(tokenString)
   162	}
   163	
   164	// ValidateToken 验证 JWT token
   165	func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
   166		secret, _ := s.jwtConfig()
   167		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
   168			return secret, nil
   169		})
   170	
   171		if err != nil {
   172			return nil, err
   173		}
   174	
   175		// ParseWithClaims 返回 nil 错误时令牌必已通过签名与有效期校验，
   176		// Claims 类型断言随之必然成功。
   177		return token.Claims.(*Claims), nil
   178	}
   179	
   180	// RefreshToken 刷新 token
   181	func RefreshToken(tokenString string) (string, error) {
   182		ensureDefaults()
   183		return defaultService.RefreshToken(tokenString)
   184	}
   185	
   186	// RefreshToken 刷新 token
   187	func (s *Service) RefreshToken(tokenString string) (string, error) {
   188		claims, err := s.ValidateToken(tokenString)
   189		if err != nil {
   190			return "", err
   191		}
   192	
   193		// 如果 token 还没过期超过 1 小时，直接返回
   194		if claims.ExpiresAt.Time.After(time.Now().Add(1 * time.Hour)) {
   195			return tokenString, nil
   196		}
   197	
   198		return s.GenerateToken(claims.UserID, claims.Username, claims.Role)
   199	}
   200	
   201	func (s *Service) jwtConfig() ([]byte, time.Duration) {
   202		if s == nil {
   203			// 双保险：即便未来新增入口忘了显式 ensure，nil 服务取密钥前
   204			// 也保证默认值已解析（空密钥签发是绝不可发生的安全底线）。
   205			ensureDefaults()
   206			return cloneBytes(jwtSecret), jwtExpiration
   207		}
   208	
   209		s.mu.RLock()
   210		defer s.mu.RUnlock()
   211	
   212		secret := cloneBytes(s.secret)
   213		if len(secret) == 0 {
   214			secret = resolveSecret("")
   215		}
   216		expiration := s.expiration
   217		if expiration <= 0 {
   218			expiration = 24 * time.Hour
   219		}
   220		return secret, expiration
   221	}
   222	
   223	func resolveSecret(secret string) []byte {
   224		if secret != "" {
   225			return []byte(secret)
   226		}
   227	
   228		// 生成随机 secret，并打印警告。
   229		b := make([]byte, 32)
   230		if _, err := randRead(b); err != nil {
   231			log.Printf("WARNING: failed to generate random JWT secret: %v", err)
   232			return []byte("change-me")
   233		}
   234		log.Println("WARNING: JWT_SECRET not set, using random secret. Tokens will not survive restarts.")
   235		return b
   236	}
   237	
   238	func cloneBytes(src []byte) []byte {
   239		dst := make([]byte, len(src))
   240		copy(dst, src)
     1	package auth
     2	
     3	import (
     4		"context"
     5		"net/http"
     6		"strings"
     7	)
     8	
     9	// Middleware 认证中间件
    10	func Middleware(next http.HandlerFunc) http.HandlerFunc {
    11		ensureDefaults()
    12		return defaultService.Middleware(next)
    13	}
    14	
    15	// Middleware 认证中间件
    16	func (s *Service) Middleware(next http.HandlerFunc) http.HandlerFunc {
    17		return func(w http.ResponseWriter, r *http.Request) {
    18			// 从 Authorization header 获取 token
    19			authHeader := r.Header.Get("Authorization")
    20			if authHeader == "" {
    21				http.Error(w, `{"error":"Authorization header required"}`, http.StatusUnauthorized)
    22				return
    23			}
    24	
    25			// 检查 Bearer 前缀
    26			if !strings.HasPrefix(authHeader, "Bearer ") {
    27				http.Error(w, `{"error":"Invalid authorization format"}`, http.StatusUnauthorized)
    28				return
    29			}
    30	
    31			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
    32	
    33			// 验证 token
    34			claims, err := s.ValidateToken(tokenString)
    35			if err != nil {
    36				http.Error(w, `{"error":"Invalid or expired token"}`, http.StatusUnauthorized)
    37				return
    38			}
    39	
    40			// 将用户信息存入 request context
    41			ctx := r.Context()
    42			ctx = contextWithUser(ctx, claims.UserID, claims.Username, claims.Role)
    43			r = r.WithContext(ctx)
    44	
    45			next(w, r)
    46		}
    47	}
    48	
    49	// OptionalMiddleware 可选认证中间件（不强制要求登录）
    50	func OptionalMiddleware(next http.HandlerFunc) http.HandlerFunc {
    51		ensureDefaults()
    52		return defaultService.OptionalMiddleware(next)
    53	}
    54	
    55	// OptionalMiddleware 可选认证中间件（不强制要求登录）
    56	func (s *Service) OptionalMiddleware(next http.HandlerFunc) http.HandlerFunc {
    57		return func(w http.ResponseWriter, r *http.Request) {
    58			authHeader := r.Header.Get("Authorization")
    59			if authHeader == "" {
    60				next(w, r)
    61				return
    62			}
    63	
    64			if !strings.HasPrefix(authHeader, "Bearer ") {
    65				next(w, r)
    66				return
    67			}
    68	
    69			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
    70			claims, err := s.ValidateToken(tokenString)
    71			if err != nil {
    72				next(w, r)
    73				return
    74			}
    75	
    76			ctx := r.Context()
    77			ctx = contextWithUser(ctx, claims.UserID, claims.Username, claims.Role)
    78			r = r.WithContext(ctx)
    79	
    80			next(w, r)
    81		}
    82	}
    83	
    84	// 用户上下文 key
    85	type contextKey string
    86	
    87	const userKey contextKey = "user"
    88	
    89	func contextWithUser(ctx context.Context, userID, username, role string) context.Context {
    90		return ContextWithUser(ctx, userID, username, role)
    91	}
    92	
    93	// ContextWithUser 把已验证的用户信息注入 context（导出版本：审计层在
    94	// auth 挂点之前解析 Bearer 记录「谁被拒」，见 server/middleware.go）
    95	func ContextWithUser(ctx context.Context, userID, username, role string) context.Context {
    96		return context.WithValue(ctx, userKey, UserInfo{UserID: userID, Username: username, Role: role})
    97	}
    98	
    99	// GetUserFromContext 从 context 获取用户信息
   100	func GetUserFromContext(r *http.Request) (UserInfo, bool) {
   101		user, ok := r.Context().Value(userKey).(UserInfo)
   102		return user, ok
   103	}
   104	
   105	// UserInfo 用户信息
   106	type UserInfo struct {
   107		UserID   string
   108		Username string
   109		Role     string
   110	}
     1	package server
     2	
     3	import (
     4		"encoding/json"
     5		"errors"
     6		"io"
     7		"net/http"
     8		"strings"
     9		"time"
    10	
    11		"github.com/cuihairu/cockpit/internal/auth"
    12		"github.com/cuihairu/cockpit/internal/storage"
    13	)
    14	
    15	// serveAPI 处理 API 请求
    16	func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
    17		// 设置 CORS
    18		setCORSHeaders(w)
    19	
    20		// 处理 OPTIONS 预检请求
    21		if r.Method == http.MethodOptions {
    22			w.WriteHeader(http.StatusOK)
    23			return
    24		}
    25	
    26		// 解析路径（归一化尾斜杠，/api/agents/ → /api/agents）
    27		path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api"), "/")
    28		if path == "" {
    29			path = "/"
    30		}
    31	
    32		// 路由分发
    33		switch {
    34		case path == "/status":
    35			s.handleStatus(w, r)
    36		case path == "/me":
    37			s.handleCurrentUser(w, r)
    38		case path == "/me/profile":
    39			s.handleCurrentUserProfile(w, r)
    40		case path == "/me/password":
    41			s.handleCurrentUserPassword(w, r)
    42		case path == "/settings":
    43			s.handleSettings(w, r)
    44		case path == "/agents":
    45			s.handleAgentsList(w, r)
    46		case path == "/agents/cleanup":
    47			s.handleAgentsCleanup(w, r)
    48		case path == "/drift/config":
    49			s.handleDriftConfig(w, r)
    50		case path == "/jobs" || strings.HasPrefix(path, "/jobs/"):
    51			// 执行 Job（见 api_jobs.go，jobs-design.md）
    52			s.handleJobsAPI(w, r, path)
    53		case path == "/workflows" || strings.HasPrefix(path, "/workflows/") ||
    54			path == "/workflow-runs" || strings.HasPrefix(path, "/workflow-runs/"):
    55			// Workflow 编排（见 api_workflow.go，workflow-design.md）
    56			s.handleWorkflowAPI(w, r, path)
    57		case path == "/approvals" || strings.HasPrefix(path, "/approvals/"):
    58			// 审批门（见 api_approvals.go，workflow-design.md M2d）
    59			s.handleApprovalsAPI(w, r, path)
    60		case path == "/inventory/consistency":
    61			// CMDB 一致性（见 api_inventory.go，drift-design.md M6 D28/D30）
    62			s.handleInventoryConsistency(w, r)
    63		case path == "/smart/config":
    64			s.handleSmartConfig(w, r)
    65		case path == "/nas/config":
    66			s.handleNASConfig(w, r)
    67		case path == "/agent-tags" || strings.HasPrefix(path, "/agent-tags/"):
    68			s.handleAgentTagsAPI(w, r)
    69		case path == "/recordings":
    70			s.handleRecordings(w, r)
    71		case strings.HasPrefix(path, "/recordings/"):
    72			s.handleRecordings(w, r)
    73		case path == "/server-backups" || strings.HasPrefix(path, "/server-backups/"):
    74			s.handleServerBackups(w, r)
    75		case path == "/dns" || strings.HasPrefix(path, "/dns/"):
    76			s.handleDNS(w, r)
    77		case path == "/domains" || strings.HasPrefix(path, "/domains/"):
    78			// 服务域名绑定（见 api_domain_bindings.go，domain-binding-design.md）
    79			s.handleDomainBindings(w, r)
    80		case path == "/overlay/cloud" || strings.HasPrefix(path, "/overlay/cloud/"):
    81			// 组网云管理面（见 api_overlay_cloud.go，M2 D11-D19）
    82			s.handleOverlayCloud(w, r)
    83		case path == "/ddns" || strings.HasPrefix(path, "/ddns/"):
    84			s.handleDDNS(w, r)
    85		case path == "/acme" || strings.HasPrefix(path, "/acme/"):
    86			s.handleACME(w, r)
    87		case path == "/logs/search":
    88			// 跨机日志联邦检索（全局端点，见 api_logs_search.go M3/D11）
    89			s.handleLogsSearch(w, r)
    90		case strings.HasPrefix(path, "/agents/"):
    91			agentID := strings.TrimPrefix(path, "/agents/")
    92			// agent 标签 GET/PUT /agents/{id}/tags（见 api_tags.go）
    93			if strings.HasSuffix(agentID, "/tags") {
    94				s.handleAgentTagAssignAPI(w, r, agentID)
    95				return
    96			}
    97			// 远程文件管理 /agents/{id}/files/...（见 api_files.go）
    98			if strings.Contains(agentID, "/files/") {
    99				s.handleAgentFilesAPI(w, r, agentID)
   100				return
   101			}
   102			// 反向代理管理 /agents/{id}/proxy/...（见 api_proxy_sites.go）
   103			if strings.Contains(agentID, "/proxy/") {
   104				s.handleAgentProxyAPI(w, r, agentID)
   105				return
   106			}
   107			// 定时任务管理 /agents/{id}/cron/...（见 api_cron.go）
   108			if strings.Contains(agentID, "/cron/") {
   109				s.handleAgentCronAPI(w, r, agentID)
   110				return
   111			}
   112			// systemd 服务管理 /agents/{id}/services/...（见 api_service.go；
   113			// 裸 GET /agents/{id}/services 是服务列表端点，HasSuffix 兜底——
   114			// 真机验收逮到：仅 Contains "/services/" 时裸列表掉进 handleAgentGet
   115			// 报 404 Agent not found，Web 服务页列表整体不可达）
   116			if strings.HasSuffix(agentID, "/services") || strings.Contains(agentID, "/services/") {
   117				s.handleAgentServiceAPI(w, r, agentID)
   118				return
   119			}
   120			// 服务健康探针 /agents/{id}/health[/probes/{id}/check]（见 api_service_health.go）
   121			if strings.HasSuffix(agentID, "/health") || strings.Contains(agentID, "/health/") {
   122				s.handleAgentHealthAPI(w, r, agentID)
   123				return
   124			}
   125			// NAS 存储观测 /agents/{id}/nas/...（见 api_nas.go）
   126			if strings.Contains(agentID, "/nas/") {
   127				s.handleAgentNASAPI(w, r, agentID)
   128				return
   129			}
   130			// 远程日志查询 /agents/{id}/logs/...（见 api_logs.go）
   131			if strings.Contains(agentID, "/logs/") {
   132				s.handleAgentLogsAPI(w, r, agentID)
   133				return
   134			}
   135			// 防漂移检测 /agents/{id}/drift/...（见 api_drift.go）
   136			if strings.Contains(agentID, "/drift/") {
   137				s.handleAgentDriftAPI(w, r, agentID)
   138				return
   139			}
   140			// agent 域名清单 /agents/{id}/domains[/snippet]（见 api_domain_bindings.go，D7）
   141			if strings.HasSuffix(agentID, "/domains") || strings.Contains(agentID, "/domains/") {
   142				s.handleAgentDomainsAPI(w, r, agentID)
   143				return
   144			}
   145			// 组网工具观测 /agents/{id}/overlay/...（见 api_overlay.go）
   146			if strings.Contains(agentID, "/overlay/") {
   147				s.handleAgentOverlayAPI(w, r, agentID)
   148				return
   149			}
   150			// SMART 磁盘健康 /agents/{id}/smart/...（见 api_smart.go）
   151			if strings.Contains(agentID, "/smart/") {
   152				s.handleAgentSmartAPI(w, r, agentID)
   153				return
   154			}
   155			// 防火墙规则集快照 /agents/{id}/firewall/...（见 api_firewall.go）
   156			if strings.Contains(agentID, "/firewall/") {
   157				s.handleAgentFirewallAPI(w, r, agentID)
   158				return
   159			}
   160			// Handle /agents/{id}/secret sub-path
   161			if strings.HasSuffix(agentID, "/secret") {
   162				s.handleAgentSecret(w, r, strings.TrimSuffix(agentID, "/secret"))
   163				return
   164			}
   165			s.handleAgentGet(w, r, agentID)
   166		case strings.HasPrefix(path, "/resources/"):
   167			s.handleResources(w, r, strings.TrimPrefix(path, "/resources/"))
   168		case path == "/users":
   169			s.handleUsers(w, r)
   170		case strings.HasPrefix(path, "/users/"):
   171			s.handleUserActions(w, r, strings.TrimPrefix(path, "/users/"))
   172		case path == "/roles":
   173			// 角色 CRUD（见 api_roles.go，rbac-design.md 笔 4）
   174			s.handleRoles(w, r)
   175		case strings.HasPrefix(path, "/roles/"):
   176			s.handleRoleActions(w, r, strings.TrimPrefix(path, "/roles/"))
   177		case path == "/alerts" || path == "/alerts/read-all":
   178			s.handleAlertsList(w, r)
   179		case strings.HasPrefix(path, "/alerts/"):
   180			s.handleAlertActions(w, r, strings.TrimPrefix(path, "/alerts/"))
   181		default:
   182			s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
   183		}
   184	}
   185	
   186	// handleStatus 获取系统状态
   187	func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
   188		if r.Method != http.MethodGet {
   189			s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
   190			return
   191		}
   192	
   193		stats := s.db.GetStats()
   194	
   195		status := map[string]interface{}{
   196			"services": map[string]interface{}{
   197				"running": stats.ServicesUp,
   198				"down":    stats.ServicesDown,
   199				"unknown": 0,
   200			},
   201			"domains": map[string]interface{}{
   202				"valid":    stats.DomainsActive,
   203				"expiring": 0,
   204			},
   205			"certificates": map[string]interface{}{
   206				"valid":    stats.CertificatesValid,
   207				"expiring": stats.CertificatesExpiring,
   208			},
   209			"infrastructure": map[string]interface{}{
   210				"total":  stats.AgentsTotal,
   211				"online": stats.AgentsOnline,
   212			},
   213			// Agent 过期阈值（分钟，0=自动过期已关闭）：web 列表据此隐藏过期
   214			// 行并在清理钮旁展示口径（D-2026-10-08-3）
   215			"agentExpireMinutes": int(s.agentExpireThreshold() / time.Minute),
   216		}
   217	
   218		s.writeJSON(w, http.StatusOK, status)
   219	}
   220	
   221	// handleCurrentUser 获取当前用户信息
   222	func (s *Server) handleCurrentUser(w http.ResponseWriter, r *http.Request) {
   223		if r.Method != http.MethodGet {
   224			s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
   225			return
   226		}
   227	
   228		authUser, ok := auth.GetUserFromContext(r)
   229		if !ok || authUser.UserID == "" {
   230			s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
   231			return
   232		}
   233	
   234		user, err := s.db.GetUserByID(authUser.UserID)
   235		if err != nil {
   236			s.handleError(w, r, http.StatusNotFound, "User not found")
   237			return
   238		}
   239	
   240		// 返回用户信息（不包含密码）。permissions 查角色表展开（P1 笔 5：
   241		// 前端菜单裁剪与按钮显隐的数据源）；幽灵角色/库故障 → 空清单，
   242		// 与判定层 fail-closed 语义一致（前端全裁剪，不会多显示出入口）
   243		permissions := make([]string, 0)
   244		if role, err := s.db.GetRole(user.Role); err == nil && len(role.Permissions) > 0 {
   245			permissions = role.Permissions
   246		}
   247	
   248		response := map[string]interface{}{
   249			"id":            user.ID,
   250			"username":      user.Username,
   251			"email":         user.Email,
   252			"phone":         user.Phone,
   253			"department":    user.Department,
   254			"role":          user.Role,
   255			"permissions":   permissions,
   256			"totp_enabled":  user.TOTPEnabled,
   257			"totp_setup_at": user.TOTPSetupAt,
   258			"created_at":    user.CreatedAt,
   259			"updated_at":    user.UpdatedAt,
   260		}
   261	
   262		s.writeJSON(w, http.StatusOK, response)
   263	}
   264	
   265	// handleCurrentUserPassword 修改当前用户密码
   266	func (s *Server) handleCurrentUserPassword(w http.ResponseWriter, r *http.Request) {
   267		if r.Method != http.MethodPost && r.Method != http.MethodPut {
   268			s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
   269			return
   270		}
   271	
   272		userInfo, ok := auth.GetUserFromContext(r)
   273		if !ok || userInfo.UserID == "" {
   274			s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
   275			return
   276		}
   277	
   278		user, err := s.db.GetUserByID(userInfo.UserID)
   279		if err != nil {
   280			s.handleError(w, r, http.StatusNotFound, "User not found")
   281			return
   282		}
   283	
   284		// 支持两种字段命名（前端使用 currentPassword/newPassword）
   285		type ChangePasswordRequestAlt struct {
   286			CurrentPassword string `json:"currentPassword"`
   287			NewPassword     string `json:"newPassword"`
   288			OldPassword     string `json:"old_password,omitempty"`
   289		}
   290	
   291		var req ChangePasswordRequestAlt
   292		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
   293			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
   294			return
   295		}
   296	
   297		oldPassword := req.OldPassword
   298		if oldPassword == "" {
   299			oldPassword = req.CurrentPassword
   300		}
   301	
   302		// 验证新密码
   303		if req.NewPassword == "" {
   304			s.handleError(w, r, http.StatusBadRequest, "New password is required")
   305			return
   306		}
   307	
   308		if len(req.NewPassword) < 6 {
   309			s.handleError(w, r, http.StatusBadRequest, "New password must be at least 6 characters")
   310			return
   311		}
   312	
   313		// 验证旧密码
   314		_, err = s.db.VerifyPassword(user.Username, oldPassword)
   315		if err != nil {
   316			s.handleError(w, r, http.StatusUnauthorized, "Invalid current password")
   317			return
   318		}
   319	
   320		// 哈希新密码
   321		hashedPassword, err := storage.HashPassword(req.NewPassword)
   322		if err != nil {
   323			s.handleError(w, r, http.StatusInternalServerError, "Failed to hash password")
   324			return
   325		}
   326	
   327		// 更新密码
   328		if err := s.db.UpdatePassword(userInfo.UserID, hashedPassword); err != nil {
   329			s.handleError(w, r, http.StatusInternalServerError, "Failed to update password")
   330			return
   331		}
   332	
   333		s.writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Password updated successfully"})
   334	}
   335	
   336	// handleCurrentUserProfile 更新当前用户信息（email/phone/department）
   337	//
   338	// 对应前端 PUT /api/me/profile。
   339	func (s *Server) handleCurrentUserProfile(w http.ResponseWriter, r *http.Request) {
   340		if r.Method != http.MethodPut && r.Method != http.MethodPost {
[0m$ [0mnl -ba docs/guide/acceptance-checklist.md | sed -n '1,390p'; nl -ba docs/guide/architecture.md | sed -n '1,240p'; nl -ba docs/guide/protocol.md | sed -n '1,240p'
��）。分流四例至此全部真机验讫（仅 nginx=本批 N1、仅 traefik=2026-09-30、双后端/旧 agent 回退=本批 N6/N7）
   106	
   107	## ACME 证书签发
   108	
   109	设计：[acme-design](./acme-design.md)。前置：DNS 凭据（Cloudflare / DNSPod / 阿里云任一）+ 公网可解析域名；先 staging 防限频。
   110	
   111	- [ ] staging 签发跑通（账号 ECDSA key 惰性注册持久化，二次签发不重复注册）
   112	- [ ] production 真证书签发；到期时间与续期阈值（7-90 天）展示正确
   113	- [ ] 三种 DNS provider 各签发一轮（cloudflare / dnspod / alidns，凭据判定文案指名缺哪个键）
   114	- [ ] 自动续期：调阈值到临期触发重签；人为失败场景 1h 节流重试不撞 CA 限频
   115	- [ ] 签发 → 自动部署推送 agent 指定路径（证书 0644 / 私钥 0600）→ nginx 站点引用 → reload → 浏览器信任链完整
   116	- [ ] 续期后 agent 侧文件同步更新；部署失败只记状态不回滚签发
   117	- [ ] 下载私钥强制记审计（download_key）；审计 detail 不含 PEM
   118	
   119	## DDNS 动态域名
   120	
   121	设计：[ddns-design](./ddns-design.md)。前置：Cloudflare token + 家庭宽带动态 IP 环境（最好含 IPv6）。
   122	
   123	- [ ] agent 出口 IP 多源探测真实生效（主源失败自动切备源；IPv4/IPv6 地址族各自正确）
   124	- [ ] 记录缺失自动创建（TTL auto、无代理）；IP 变化更新保留原 TTL/Proxied
   125	- [ ] 同 zone+type 多记录共享一轮 ListRecords（观察 Cloudflare 后台调用频次无放大）
   126	- [ ] 失败告警去重（同记录未读期间只报一次）；恢复后静默自愈清错
   127	- [x] 巡检间隔修改即时生效；`0=关闭` 形态——本机验收（2026-10-07，探针 `scripts/acceptance/ddns/probe-scan.sh` D1-D5 6/6 PASS，零外部依赖形态：配置指 ghost agent + 不配 CF token，检查在 provider/agent 前置校验快速失败但 CheckedAt/LastStatus/LastError 回写任何错误路径都走，用 CheckedAt 推进做扫描观测）：默认 300 回读、59/86401/非数字拒 400 合法值不污染；interval=60 到点扫描（failed 回写 + LastError 非空）→ 连续推进（实测 gap=120s——60s 门槛 × 60s tick 组合下 `Since(lastScan)<60` 偶真跳一拍，门槛语义预期非缺陷）→ PUT 0 后跨 ≥1 原扫描窗 CheckedAt 冻结 → 改回 60 恢复推进（间隔修改即时生效双向实证）。真 CF 记录比对/出口 IP 探测/告警去重维持挂起（120-123 需真 token 与动态 IP 环境）
   128	
   129	## DNS 域名管理
   130	
   131	设计：[domain-binding-design](./domain-binding-design.md)。前置：DNSPod 账号（免费版优先，暴露真实限制）。
   132	
   133	- [ ] DNSPod 免费版 TTL 下限：编辑低于免费下限的 TTL 时 API 真实报错的透传呈现
   134	- [ ] CAA 记录新增/编辑/删除
   135	- [ ] Cloudflare 与 DNSPod 两家 SRV 记录编辑实测（字段映射与回读）
   136	
   137	## 组网观测（Overlay）
   138	
   139	设计：[overlay-design](./overlay-design.md)。前置：ZeroTier Central / Tailscale 管理 API token；装 ZeroTier/Tailscale/WireGuard/frp 的真实主机若干。
   140	
   141	- [x] 运行态观测 wg 半：真实 interfaces 快照 + 私钥/预共享密钥不出现
   142	  （2026-10-07，scripts/acceptance/overlay/probe-wg.sh W1-W3 全 PASS——
   143	  NET_ADMIN 容器真 wg0（wireguard-tools 建接口，内核 wireguard 模块宿主侧
   144	  自动加载）：W1 真实快照 wg0/listenPort=51820/peerCount=1/endpoint 与
   145	  allowed-ips 原样；W2 私钥与预共享密钥 hex 不在响应（D6 白名单）；W3 peer
   146	  公钥（公开数据）原样。**验收逮到并修复真缺陷**：新版 wireguard-tools
   147	  （v1.0.20200513+）interface 行多一列 public-key（真机 1.0.20210914 实证
   148	  `<iface> <私钥> <公钥> <端口> <fwmark>` 五列），旧解析按固定下标取
   149	  fields[2] 把公钥当 listenPort 渲染——私钥未泄（fields[1] 恒弃），但端口
   150	  字段显示为一段 base64；修为取倒数第二列（fwmark 恒末列）两代布局兼容 +
   151	  新布局回归单测，包覆盖保 100.0%）
   152	- [ ] 运行态观测 ZT/TS 半 + 跨 agent 同节点合并：真实 peers 快照与合并徽标
   153	  （挂起：需真 zerotier 网络 / tailscale tailnet 登录与第二台主机，云端与
   154	  daemon 侧不可本机造，随外部资源补；解析/合并逻辑单测已盖）
   155	- [x] frp 分级观测：零配置只出版本+进程；配 admin 地址后隧道计数出现
   156	  （2026-10-07，scripts/acceptance/overlay/probe-frp.sh O1-O3 全 PASS——容器内
   157	  真实 frp 0.61.2（frps bind 7100 + admin 7400、frpc admin 7500 + 一条 tcp
   158	  proxy、loginFailExit=false 防首连撞未监听即永久退出）：O1 零配置 status=ok +
   159	  version + 双进程 running 且无 tunnels/proxies 键；O2 双 admin 地址 →
   160	  frpc.tunnels=1、frps.proxies=1；O3 frpc admin 死端口 → status=degraded +
   161	  adminError，frps 侧不受影响。**验收逮到并修复真缺陷**：frps 没有 frpc 的
   162	  `/api/status`（真机 404——原实现调它恒 adminError 404 + degraded，frps
   163	  admin 从未在真 frp 上工作过）；修为 frps 走 dashboard 稳定路由
   164	  `/api/serverinfo` 的 proxyTypeCount 求和（fetchFRPSAdminProxyCount）+ 单测
   165	  （含半截 body 读错分支），overlay-design.md D7 与数据源表措辞同步修正）
   166	- [ ] 云端管理：真 token 拉取 ZT 网络/成员与 TS 设备列表；授权/取消授权、除名、删除四类变更端到端生效（云端后台复核）
   167	- [ ] 未纳管设备识别：云端手动加一台不入面板的设备 → 「未纳管」徽标出现
   168	- [ ] agent 身份上报：本机身份 chip 的 ZT node id / TS device id 与云端同键对照 → 「面板纳管」徽标
   169	
   170	## 磁盘健康（SMART）
   171	
   172	设计：[disk-health-design](./disk-health-design.md)。前置：带物理盘的真实主机（smartctl 可用）。
   173	
   174	- [x] 盘发现与字段读取：passed/温度/重映射/待定扇区/NVMe media_errors 真实数值合理
   175	  （2026-10-07，scripts/acceptance/smart/probe.sh S1-S4 全 PASS：本机 /dev/sda 真盘、
   176	  agent 以 root 拉起（smartctl 非 root 只回 rc=0 的空壳 JSON）、S1 盘发现
   177	  available+/dev/sda+model+sizeBytes、S2 health 与 smartctl -H 直读交叉一致（passed）、
   178	  S3 温度通道一致（VM 盘无传感器 → 双侧同缺，omitempty 0 值不渲染）、S4 attr 缺失
   179	  不伪造（ground truth 无 attr 表时 realloc/pending 面板同缺，不造 0 值冒充真读）。
   180	  边界注记：attr 非零真值面（重映射/待定扇区/NVMe media_errors 真实数值）需带真
   181	  attr 表的主机——本机为 QEMU VM 虚拟盘（health=passed、temp=0、无 attr 表）；
   182	  解析映射已由 smart_parse 单测覆盖（ATA passed/failed、NVMe、无 smart_status、
   183	  坏 JSON、raw 数值），真值主机实测随 NAS/物理机资源补）
   184	- [ ] 巡检告警：FAILED 盘 error 级、扇区/介质异常 warning 级、同盘未读期间只报一次
   185	  （挂起：真坏盘不可本机造——FAILED/扇区异常分级与 agent 过滤由 smart_scan 单测
   186	  覆盖（TestScanSmartOnceCreatesAlert/WarningLevel/Filters），告警真去重走 alert
   187	  管道同 drift；盘健康面 passed → failed 的端到端翻转需物理坏盘，随外部资源补）
   188	- [ ] （按需）非 root 部署下 sudo 提权读取路径
   189	  （挂起：需改宿主 sudoers，不做系统级变更——读数链路已按 root 形态实证
   190	  （探针 S0 root agent + smart.status 全链），sudo -n 包装属部署形态选项，
   191	  随真实非 root 部署需求补）
   192	
   193	## 防火墙观测（M1）
   194	
   195	设计：[firewall-design](./firewall-design.md)。探针：`scripts/acceptance/firewall/probe.sh`（本机三形态 agent：root/无 nft PATH/非 root）与 `scripts/acceptance/firewall/probe-hosts.sh`（容器宿主形态：裸容器/iptables-legacy/超大规则集，H1-H4）。
   196	
   197	- [x] capability 上报：nft/iptables 元数据与真实安装一致；双后端选择（D2）真实回落（2026-10-07 本机探针 F1/F3：nft 在走 nftables、PATH 摘除 nft 后 iptables-save + variant 解析 nf_tables）
   198	- [x] 规则快照真读：nft -j JSON 与 iptables-save 双后端解析真实规则集，表/链/规则计数与 nft list ruleset 对照合理（2026-10-07 本机探针 F2/F3：5 表 22 链 114 规则，ipv4+ipv6 双族）
   199	- [x] 非 root 说明态：available=false + 真实权限错误透传（2026-10-07 本机探针 F4，"you must be root"）
   200	- [x] 权限位单一 read 档（D10）：无 firewall:read 自定义角色 403、内置 viewer 200（2026-10-07 本机探针 F5）
   201	- [x] 浏览器实测页面三态：总览/按主机面板/策略徽标/cockpit 标注在真实数据下的渲染（2026-10-07 probe-web W1-W6：headless Chrome 驱动真实 web/dist——空态 Empty 文案、总览行后端+variant 标注、input accept 徽标、不可读/部分读取状态列、面板规则明细 + cockpit Tag；断言走 DOM innerText，headless-shell 容器无 CJK 字体截图呈 tofu 属容器字形缺失）
   202	- [x] iptables-legacy 主机（variant=legacy）：真实主机读数与变体标注（2026-10-07 probe-hosts H2 数据面真读 + probe-web W3a/W4 页面 variant 标注与规则明细）
   203	- [x] 无防火墙工具的裸容器主机：页面空态（2026-10-07 probe-hosts H1 数据面 502 unknown provider + probe-web W2 空态文案渲染）
   204	- [x] 超大规则集（>4MB）truncated 告警与 meta-only/行界截断呈现（2026-10-07 probe-hosts H3 nft 7.7MB meta-only + H4 iptables 5.1MB 行界截断留 16362 条；probe-web W3d 页顶 4MB 告警 + W6 面板 meta-only 错误说明不误报「规则集为空」）
   205	
   206	## 日志检索
   207	
   208	设计：[logs-design](./logs-design.md)。前置：多台 agent（含 systemd 主机与 docker 主机）。
   209	
   210	- [x] 尾随真机项：journalctl 高频输出流畅尾随不重不漏
   211	  证据（2026-09-30）：探针 `scripts/acceptance/logs/`（系统域 transient unit 高频源，本机实验结论：`journalctl -u` 不匹配用户域瞬态单元须 sudo systemd-run 建系统域）T1——tail=2000 回填 500 行 + 600 行/s 实时突发 6000 行，6500 帧序列严格递增、零重复零缺口（序号带每轮唯一基址防 journal 跨轮累积干扰）；验收顺带修通两处流式链路缺陷：① agent 侧 logs POST（query/follow）被 RBAC 按 method 泛化推导成 `logs:write`——合法权限集合只有 `logs:read`，任何角色（含 admin）都 403，端点整体不可达（既有测试直调 handler 绕过中间件故未拦住），修为与 `/api/logs/search` 同口径 fixed read；② 审计中间件 `responseWriter` 包装器丢 `http.Flusher`，生产链路 `w.(http.Flusher)` 断言必失败 → 500「streaming unsupported」（/ws 的 Hijacker 教训同类，修为接口透传保审计）；`probe.log`
   212	- [x] docker 容器停止后尾随以 `reason=exited` 正常终止
   213	  证据（2026-09-30）：T2——alpine 循环输出容器 follow 收 ≥3 行后 `docker stop -t 2`，`docker logs -f` 随容器退出 → eof `reason="exited"`，DOCLINE 序列有序无重；`probe.log`
   214	- [x] 10 分钟超时兜底生效
   215	  证据（2026-09-30）：T3——空闲源（prewarm 后完全静默）follow 常驻（`journalctl -f` 对已结束 unit 不自退，本机实测常驻，超时是唯一出口），到点 eof `reason="exited"`、耗时精确 10m0s、沉淀期后零新增帧；单测面 `TestLogsFollowTimeoutKillsIdleSession`（`logsFollowTimeout` 行为中性 var 注入，生产恒 10min）；`probe.log`
   216	- [x] 跨机联邦检索：多 agent 并行扇出按主机分组返回；offline/无 logs capability/路径不存在三种 skipped 归因正确；单 agent 失败不整体报错
   217	  证据（2026-09-30）：T4a/T4b——3 agent 拓扑（全量 PATH / 空 PATH 无 logs capability / journalctl 失败 shim 遮蔽有 capability 但查询必败）+ ghost id 四样本齐：a1 ok=true 带 hostname 分组、a4 ok=false error 降级且 HTTP 200 不整体报错、a2 skipped `no-logs`、ghost skipped `offline`，结果按 agentId 排序；全量扇出 skipped 只列在线无 capability 者（离线者天然缺席不列）；口径注明：「路径不存在/not-found」归因已与 offline 合并（`api_logs_search.go` 注册表缺席即离线、不区分 not-found，ghost id 即该形态的现实验证）；原始请求/响应 `search-*.json`
   218	
   219	## NAS
   220	
   221	设计：[nas-design](./nas-design.md)。前置：DSM 6.x/7.x、TrueNAS CORE/SCALE、OMV 真机各一（或有则验）。
   222	
   223	- [ ] DSM 6 与 7 两版本 API 字段差异实测（登录、卷、共享夹）
   224	- [ ] TrueNAS CORE 与 SCALE 字段差异（flexInt 字符串数字形态）
   225	- [ ] OMV：`X-Openmediavault-Sessionid` 头认证全流程；`"1.50 GiB"` 容量字符串解析；2FA 账号等同失败降级
   226	- [ ] mdadm 降级演练：拔盘/标记 fault 后告警送达（真去重）
   227	- [ ] ZFS / SMB / NFS 主机真实枚举
   228	- [x] `COCKPIT_NAS_TARGETS` 跳板探测机制：targets 解析→拨号→失联降级→source
   229	  归一全链（2026-10-07，scripts/acceptance/nas/probe-jump.sh J1-J2 全 PASS——
   230	  debian:12-slim 裸容器静态 agent：J1 合法 targets（死端口 dsm）→ nas/status
   231	  200、source=linux 不掺 dsm、pools/shares 空、available 与 df mounts 自洽
   232	  （失联 target 只 log 跳过不崩不超时）；J2 非 JSON env 整体忽略不崩。
   233	  形态实证两则：docker bind mount 让宿主真实块设备在容器 df 可见（≥1GB
   234	  计入 mounts 是正确行为）；/proc 宿主共享 → mdstat 恒存在 → DetectNas
   235	  mdstat 分支在 Linux 恒真（nas capability 恒上报）——env 注册分支只在
   236	  Windows/macOS 跳板平台可见，属解析层（单测盖）。真 DSM/TrueNAS/OMV 远端
   237	  观测维持挂起随真实 NAS）
   238	
   239	## 服务管理（三后端）
   240	
   241	设计：[service-design](./service-design.md)。前置：systemd Linux 主机、Windows 主机、macOS 主机各一。
   242	
   243	- [x] systemd：列表含未加载 unit；start/stop/restart/enable/disable 实测；mask 后隐藏启动并显解屏蔽、unmask 还原
   244	  证据（2026-10-01）：探针 `scripts/acceptance/services/`（本机 :19993，root/cui 双 agent 对照，测试 unit `cockpit-acc-*` 专属）两轮 9/9 PASS——首轮：T1 列表 total=313>active=72，files-only 未加载项在列（`apport-coredump-hook@.service` 等模板/未加载类，`mergeServiceUnits` 分支）；T2 restart→active/running、enable→enabled；T3 非 root restart/enable 同一 unit 均 502 且体含 `systemctl` 原文（Access denied…interactive authentication）原样透传，对照组非 root 读列表 200（纯权限失败）；T4 stop→inactive、disable→disabled 收尾；验收逮到一处生产缺陷：`serveAPI` 分发器仅 `Contains "/services/"`（要求尾斜杠），裸 `GET /api/agents/{id}/services`（文档与 Web `api.ts:1043` 调的列表端点）掉进 `handleAgentGet` 报 404，Web 服务页列表整体不可达（既有测试直调 handler 绕过分发器故未拦住），修为与 `/health`、`/domains` 行同款 `HasSuffix || Contains` + 分发器回归 `TestServiceListDispatcherRoutesBarePath`（反向验证无修复必 FAIL）；补验轮：ghost 未加载样本（装 `/etc` 永不 enable/start → `loadState` 空 + `activeState=inactive` + `unitFileState=disabled`，运行表/安装表双对照）、start/stop 往返、disable/enable 往返、mask→`masked`（masked 上 restart 502、is-masked 原文透传）→ unmask 还原 `enabled`（列表态同步，Web masked 行隐藏启动/显解屏蔽即由该字段驱动）、审计 `service_action` 落账断言（unit/agent 齐全）；teardown 删 unit 文件 + daemon-reload 可复跑（run-server 归一旧实例 DB，防 `/api/agents` 旧 agent 行污染注册计数）；证据 `probe.log`（两轮）+ `setup.log` + `services-list.json`
   245	- [x] systemd：daemon-reload 工具栏按钮；unit 文件查看/编辑（包管文件先复制 `/etc/systemd/system` 覆盖位再改，升级不丢）
   246	  证据（2026-10-01，R3 轮）：同探针扩展至 17/17 PASS——T4 unit 文件全链：GET 有效视图（`systemctl cat` 全文 + `fragmentPath=/usr/lib/systemd/system/...` 包管位）；PUT 改 Description → 包管文件先复制 `/etc/systemd/system` 覆盖位再写（PUT 响应 `path=/etc/...` + `/etc` 落盘 + 复读 `fragmentPath` 换位三证）+ 保存捆绑 daemon-reload（`systemctl show -p Description` 即时反映新值——不 reload 不可能，端到端铁证）；超 256KB+1 内容 413（server `MaxBytesReader` 与 agent 同限双端防御）；拒绝面：白名单外 unit 名（含 `:`）400、`{unit}/file/x` 多余段 404（写哪由 FragmentPath 决定、只收 unit 名，无路径参数即无穿越面）、windows 形名（`zzz-notaservice`）过 server 双后端并集白名单后 agent systemd 兜底拒且 502 透传 `invalid unit name ... expect *.service`（D9.4 设计内行为）；非 root PUT 502 + `permission denied` 原文透传（与 root 同操作 200 对照）；审计 `details.action=unitfile-save` 落账。daemon-reload 端点：`POST /agents/{id}/services/daemon-reload` 200 `{"reloaded":true}` + 审计落账；Web 接线在位——工具栏「重载配置」Popconfirm（`Services/index.tsx:381-386`）与「文件」Modal（`:320-494`）走 `api.ts:1056/1063/1070` 同三端点。本轮无产品缺陷（首轮 c88a48a 已修 serveAPI 分发器 404）
   247	- [x] systemd：服务行「日志」跳转 journalctl 历史可查（含未加载 unit）
   248	  证据（2026-10-01，R3 轮）：T5（`cockpit-acc-jlog.service` 样本，装 `/usr/lib` 永不 enable）——PUT 换带唯一 marker 的 oneshot ExecStart（dogfood 覆盖位 + 捆绑 reload：start 执行的正是 PUT 后内容）→ start 产 journal 历史 → stop + daemon-reload 后成未加载安装项（三面证据：`systemctl list-units --all` 无、服务表 `loadState=""`+`unitFileState=disabled`、`/logs/sources` 不含）→ `POST /agents/{id}/logs/query {"type":"systemd","source":"cockpit-acc-jlog.service"}` 200 且 `lines` 含 marker（`journalctl -u` 不关心加载态）；Web 链路同款：服务行「日志」按钮开 Drawer 内嵌 LogsPanel（`Services/index.tsx:506-514`），`initialSource` 对 unit 名无条件优先派生不回落（`LogsPanel.tsx:124-126`，未加载服务不在 `logs.sources` 列表仍可查）；非 root daemon-reload 502 Access denied 透传（对照记录）
   249	- [ ] Windows SCM：列表/启停/自启切换/restart 等待 30s 超时路径实测；无权限服务报错透传；reload 按钮确认隐藏
   250	- [ ] macOS launchd：列表/启停/自启切换；`kickstart` 失败回退 `bootstrap` 路径；无权限报错透传
   251	
   252	## 统一 Job 执行（agent.exec）
   253	
   254	设计：[jobs-design](./jobs-design.md)。前置：一台在线 Linux agent（命令经 `sh -c` 执行）。
   255	真机验收（2026-10-05，探针 `scripts/acceptance/jobs/` 五件套 9/9 PASS，证据
   256	`.acceptance/jobs/evidence/`：probe.log + create-J1..J5.json + audit-job_run.json）。
   257	
   258	- [x] 创建 Job（在线 agent，`uptime`）→ 弹窗回显 success、输出非空、退出码 0；台账新增一条（15s 轮询内可见）
   259	  证据：J1 `echo J1-OUTPUT-MARK && uptime` 82ms 返回 success/exitCode 0/输出 80B 含标记，
   260	  actor/type/target/parameters 往返一致、startedAt/finishedAt 落定；台账 0→1 条且倒序首位
   261	  即该条、单条详情含同样输出。Web 接线在位：15s 轮询 `Jobs/index.tsx:51`
   262	  （refetchInterval 15000）、终态成功提示按 STATUS_META 回显（`:84`）、
   263	  菜单 perm `jobs:read`（`App.tsx:211`）
   264	- [x] 非零退出（`exit 3`）→ failed、退出码 3、error 无输出回退
   265	  证据：J2 `echo J2-BEFORE-OUT; exit 3` → status=failed、exitCode=3、exit 前输出仍带回
   266	  （D4 业务字段）；error 实际为 `exit status 3`（agent trimExecErr 归一，设计「error
   267	  为空」的表述按实现修正——非零退出与超时的错误讯息走同一字段）
   268	- [x] 超时（`sleep 30`，timeout_s=1）→ failed、error 含 timed out；agent 侧进程组 SIGKILL，`ps` 复核无孤儿 sleep 残留
   269	  证据：J3 `sleep 297` timeout_s=1 → failed、exitCode=-1、error=`timed out after 1s`，
   270	  墙钟 1.078s（按时返回非等满）；`pgrep -f "sleep 297"` 复核零命中（进程组杀净，
   271	  无孙进程持管道悬挂）
   272	- [x] 输出截断：>64KB 大输出 → truncated 标记，业务字段不炸 RPC
   273	  证据：J4 `seq 1 20000`（全量 ≈108KB）→ success、output 恰 65536B、尾行 20000 保留、
   274	  首行 1 已截（agent `jobExecMaxOutput` 与 server `storage.JobMaxOutput` 双端同限截尾）。
   275	  `truncated` 标记在 agent RPC data 内，jobView 视图不暴露（表意按实现修正）
   276	- [x] 离线 agent：创建弹窗下拉禁用该机；直连 `POST /api/jobs` → 503，不落幽灵 Job 记录
   277	  证据：J5 目标 `jobs-acc-ghost`（从未注册；与「曾在线后掉线」同一 registry.Get 分支，
   278	  logs T4 同口径）→ 503 `agent offline`，台账 4→4 条不变、ghost 目标记录 0。Web 下拉
   279	  `Jobs/index.tsx:65` `disabled: a.status === 'offline'` 且标签标「（离线）」
   280	- [x] 审计：CreateJob 与终态 `job_run`（resource=job、details 含 type/target/status/params）在审计日志页留痕
   281	  证据：J7 `GET /api/admin/audit/logs?resource=job&action=job_run` 命中 J1-J4 每 Job
   282	  恰一条（resource_id=Job ID、username=admin、details 含 type/target/status/params.command）；
   283	  创建前的 503/400 校验失败不产审计，审计页按 resource=job 可查
   284	- [x] 权限：无 `jobs:write` 不见「执行命令」入口；无 `jobs:read` 进不了 Jobs 页
   285	  证据：J8 viewer（jobs:read）GET 列表/详情 200、POST 403；自定义仅 `dns:read` 角色
   286	  GET/POST 全 403；web 入口 PermGuard `jobs:write`（`Jobs/index.tsx:169`）+ 菜单
   287	  `jobs:read`（`App.tsx:211`）。**验收逮到真缺陷**：`/api/jobs` 未登记 `resourceRules`
   288	  → RBAC governed=false 整体放行，viewer 也能执行（与 agent-tags 补规则前同类）；
   289	  修为 `rbac.go` 补 `{"/api/jobs", "jobs", ""}` + 三角色矩阵 7 例（rbac_test.go）
   290	
   291	## Workflow 编排（M1：异步 Job + 线性链）
   292	
   293	设计：[workflow-design](./workflow-design.md)。前置：一台在线 Linux agent（同 Job 节）；
   294	M2b 扇出验收需两台（run-server.sh 起 `workflows-acc-a1/a2` 双实例）。
   295	M1 已实现并验收（2026-10-09）：探针 `scripts/acceptance/workflows/`（W0-W9，四件套 +
   296	Go probe，对齐 jobs 探针口径）真机 9/9 PASS（端口 20010，agent `workflows-acc-a1`，
   297	证据 `.acceptance/workflows/evidence/` + `probe.log`）。
   298	M2b 多目标扇出已实现并验收（2026-10-10）：探针扩至 W10，真机 12/12 PASS
   299	（双在线 agent + ghost 样本）。
   300	M2c 取消信号已实现并验收（2026-10-11）：探针 W5/W9 语义翻新至 `job.cancel`
   301	真击杀口径、新增 W11 变量传递场景（M2a 端到端面）——本机双实例 harness
   302	（run-server.sh，端口 20010）13/13 PASS（W5 击杀=true/台账 cancelled、W9
   303	running 取消 200+击杀 cancelled/终态再取消 409、W11 解析+定义留原文全实证）；
   304	下方 W5/W9 两条为翻新前的历史证据，语义以翻新后探针为准。
   305	
   306	- [x] 异步创建：`POST /api/jobs` 立即 201 返回 pending，台账轮询内见终态；pending 行「取消」可见可点（W1/W9）
   307	  证据：W1 `uptime` → 201 status=pending（W1 契约，0d065a1 race fix 后确定性）→ 台账轮询
   308	  success/exit 0/输出标记/params 往返/起止落定；pending 行取消 Web 在位 `Jobs/index.tsx:159`
   309	  （pending 行「取消」按钮 → `cancelJob`），running 取消被拒 409 `only pending`（W9）
   310	  （M2c 翻新：running 取消改为 RPC 击杀 200 + `cancelled`——W9 已翻新并复核
   311	  通过 200+击杀 cancelled+终态再取消 409）
   312	- [x] 成功链：两步 Workflow run 全步 success，run 台账每步 jobId 可点开看输出/退出码（W2）
   313	  证据：W2 两步链（uptime → df -h）run success；步骤快照 success 落 run.steps；
   314	  Job 台账 2 条 actor=workflow、`/api/jobs?workflow_run_id=` 命中；Web run 详情
   315	  jobId「查看」按钮复用台账 Job 视角（`Workflows/index.tsx:538` openJob）
   316	- [x] 失败停：中间步失败（非零退出）→ run failed、第三步不执行、断点在快照可读（W3）
   317	  证据：W3 三步链第 2 步 `exit 3` → run failed；快照 [success/failed/pending] 断点可读；
   318	  Job 台账恰 2 条（第三步无 Job，未派发）
   319	- [x] 重试：首试失败重试成功（retry=1）→ 步骤 attempts=2、每次尝试独立 Job 行进台账（W4）
   320	  证据：W4 marker 文件首试 `cat` 失败（exit 1）、retry=1 重试成功 → steps[0].attempts=2、
   321	  尝试 2 success 含输出标记、尝试 1 failed exit=1 独立成行；两条 Job 间隔实测 ∈[4s,10s]
   322	  （workflowRetryInterval=5s 语义）
   323	- [x] 取消 run：在途步骤自然结束后 run cancelled、后续步骤不推进；再次取消 409（W5）
   324	  证据：W5 sleep 12 在途 → cancel 200 → run cancelled+finishedAt；12s 后步骤 2 仍无
   325	  Job（不推进）、台账 1 条；再次 cancel 409 `run is not running`；Web 取消按钮仅
   326	  running 可见（`Workflows/index.tsx:496`）
   327	  （M2c 翻新：在途步骤改为被 `job.cancel` 真击杀落 `cancelled`+stopReason=`run
   328	  cancelled`，不再自然跑满——W5 已翻新待复核）
   329	- [x] 重入与冻结：同定义重复 run 409；active run 期间定义更新/删除 409（W6）
   330	  证据：W6 active run 窗口内（8s/12s sleep 期间）触发第二次 run 409、PUT 定义 409、
   331	  DELETE 定义 409（三 409 冻结面）；cancel 置终态后 DELETE 200 可清（W7 尾段）
   332	- [x] 离线目标：步骤目标含离线 agent → 503 不建 run（与单条 Job 同口径）（W7）
   333	  证据：W7 目标 `workflows-acc-ghost`（从未注册，与「曾在线后掉线」同 registry.Get 分支）
   334	  → 503 含 `agent offline`、run 台账 0 条（不落幽灵）；同口径单条 Job 见 J5
   335	- [x] 权限与审计：无 `workflows:write` 不见运行/新建入口；`workflow_create/workflow_run/workflow_cancel` 审计留痕（W8）
   336	  证据：W8 viewer（workflows:read）列表 200/run 详情 200、建/触发/取消全 403；仅
   337	  `dns:read` 自定义角色全 403（rbac `{"/api/workflows","workflows",""}` 路由层）；
   338	  审计 create/run/cancel 三动作留痕 resource=workflow；Web 入口 PermGuard
   339	  `workflows:write`（`Workflows/index.tsx:307,315,334`）+ 菜单 `workflows:read`
   340	  （`App.tsx:222`）
   341	- [x] 真机编排一轮：对真实 agent 跑一条「uptime → df -h」两步链并核对台账/审计/run 快照（W2）
   342	  证据：本机实例（:20010）+ agent `workflows-acc-a1` 真机执行；run success、两步快照
   343	  均含输出、Job 台账 2 条 actor=workflow、审计 workflow_run resource_id=run id；
   344	  证据文件 run-W2/final-W2/jobs-W2/audit-workflow.json（`.acceptance/workflows/evidence/`）
   345	
   346	## 移动端（Flutter，iOS + Android）
   347	
   348	设计：[mobile-design](./mobile-design.md)。前置：Android 真机（或模拟器）安装 debug APK（CI `Mobile` workflow artifact 或本地 `flutter build apk --debug`）；server 可达（公网或同网段）。iOS 编译验证已闭环到 CI macOS runner（`Mobile` workflow build-ios job，无签名 debug 构建，见 D8）；iOS 真机安装与上架签名仍需自有 Mac/账号，下述清单 iOS 侧暂以 Android 真机为准。
   349	
   350	- [ ] 首启引导：输入 server 地址 → `/health` 通过 → 落盘进入登录页；错误地址的失败呈现可读
   351	- [ ] 登录：用户名密码 → 开启 TOTP 的账号进入验证码二步 → 成功后底部四 tab 可用
   352	- [ ] 仪表盘：主机在线/离线计数、未读/错误告警数与 web 面板一致
   353	- [ ] 主机列表：在线徽标、`ip · region · zone` 标签与后端一致；下拉刷新
   354	- [ ] 容器操作：docker 主机进容器列表，restart 真实生效（状态翻转）、失败 SnackBar 呈现、操作在 web 审计留痕
   355	- [ ] 告警：列表与 server 一致、未读加粗带点、「全部已读」后 web 侧同步已读
   356	- [ ] 审计：分页首屏加载、数据不足一屏自动预取、失败行红标
   357	- [ ] 会话恢复：杀进程重开直接进入主界面（本地 token）；调短 JWT expiration 后 token 过期自动续期无感；refresh 失效回登录页
   358	- [ ] 安全：未开自签开关时自签 server 连接被拒；开启后可用（红字风险提示存在）
   359	- [ ] 推送：server 配 ntfy 渠道，触发一条告警，ntfy 官方 app 手机送达（D3 路径）
   360	- [ ] 深色模式跟随系统切换正常
   361	- [ ] 资源页：域名/证书临期徽标（正常/临期/已过期）与 web 资源面板一致；证书剩余天数呈现
   362	- [ ] 备份：任务列表 last_status/下次运行时间与 web 一致；「立即运行」真实触发且 SnackBar 反馈
   363	- [ ] Cron：带 cron 能力的主机入口可见，任务列表（schedule/命令/启停）与 server crontab 一致；外部条目原文可读
   364	- [ ] 文件：目录导航进出流畅、点文件弹详情（路径/权限/大小）；自签 server 下文件列表可加载
   365	- [ ] 终端：主机动作单「SSH 终端」连接成功、键盘输入与回显即时、命令执行真实生效（如 touch 后文件页可见）、断线出现错误横幅与重连按钮
   366	- [ ] 生物识别锁：设置页开关仅在支持设备可开；开启后杀进程重开要求指纹/面容验证；验证失败停留蒙层可重试；关闭开关后重开直接进主页
   367	
   368	## 验收后回填约定
   369	
   370	- 每通过一项：勾选本清单 + 在 `todo.md` 对应条目的「剩余」中移除该项，要点补进完成记录
   371	- 验收发现的缺陷：按既有纪律单独立项修复（一笔一提交，不夹带）
   372	- 全域通过后在本文件顶部标注「已完成（日期）」并归档到 `docs/archive/`（如适用）
   373	- [x] 扇出步：`targets` 多目标顺序逐台执行 → run success、逐台快照独立 jobId、台账按台独立（W10，M2b）
   374	  证据：W10 双台 [a1,a2] `echo FAN-OK && uptime` → 逐台 success、jobId 独立成行、
   375	  台账 2 条；Web 步骤编辑器多选目标 + run 时间线「N 台」聚合展开逐台明细（vitest 22 用例）
   376	- [x] 扇出失败停：首台失败 run failed、次台 pending 无 Job（F3 同 W5 停推进口径）（W10b）
   377	  证据：W10b [a1,a2] `exit 3` → 逐台 [failed/failed, pending]、步级 StopReason=failed、
   378	  台账 1 条（次台从未派发）
   379	- [x] 扇出 ghost 混入：任一目标离线 503 不建 run（F8 逐台校验，延伸 W7）（W10c）
   380	  证据：W10c [a1,ghost] → 503 `agent offline for steps[0]`、run 台账 0 条（不落幽灵 run）
     1	# 架构与边界
     2	
     3	> 语言：**中文** ｜ [English](./architecture.en.md)
     4	
     5	本文以当前仓库实现为准，说明 Cockpit 的运行拓扑、模块职责和边界。它不是长期愿景文档；当代码行为变化时，应优先同步这里。
     6	
     7	## 总览
     8	
     9	Cockpit 当前由三个运行面组成：
    10	
    11	```text
    12	Browser Web UI
    13	    |
    14	    | HTTP JSON API / WebSocket
    15	    v
    16	Cockpit Server (cmd/cockpit)
    17	    |  \
    18	    |   \ SQLite runtime store
    19	    |
    20	    | WebSocket /ws
    21	    v
    22	Cockpit Agent (cmd/cockpit-agent)
    23	    |
    24	    | local API / socket / TCP
    25	    v
    26	Managed targets (host, Docker, PVE, OpenWrt, SSH/RDP/VNC service)
    27	```
    28	
    29	Server 是中心控制面，Agent 是节点侧执行面。Agent 主动连接 Server，因此 NAT 后节点不需要暴露入站端口。Web UI 只与 Server 通信，不直接访问 Agent 或内网目标。
    30	
    31	## 进程边界
    32	
    33	| 进程 | 入口 | 职责 | 不负责 |
    34	| --- | --- | --- | --- |
    35	| Cockpit Server | `cmd/cockpit` | HTTP API、Web UI 静态资源、Agent WebSocket 注册与路由、SQLite 持久化、认证、审计、告警、反向代理与远程会话编排 | 直接访问 NAT 后内网服务、直接采集 Agent 本机指标 |
    36	| Cockpit Agent | `cmd/cockpit-agent` | 节点能力检测、心跳和系统指标上报、RPC/代理/远程桌面执行、连接本机或本网络可达目标 | 用户认证、全局状态持久化、跨 Agent 调度决策 |
    37	| Web UI | `web/` | 登录、资源视图、工作台、监控、设置、远程连接交互 | 保存最终运行状态、绕过 Server 访问 Agent |
    38	| CLI | `cockpit init/sync/status/server/agent` | 本地初始化、inventory 同步、状态查询、启动 Server；`cockpit agent` 兼容 `cockpit-agent start` 的启动参数 | 长期运行时的 Agent 职责仍在 Agent 进程内 |
    39	
    40	## 数据边界
    41	
    42	| 数据 | 位置 | 角色 | 生命周期 |
    43	| --- | --- | --- | --- |
    44	| 配置文件 | `config/cockpit.yaml` 或 `-config` 指定路径 | Server 启动配置，包括监听地址、数据库、JWT、邮件、通知、inventory | 启动读取；环境变量占位符会展开 |
    45	| Inventory YAML | `inventory.path` 或 `cockpit sync -inventory` | 声明式资产清单 | 手动 `cockpit sync` 或 `inventory.watch` 热加载都会通过同一套 Syncer 写入数据库 |
    46	| SQLite | `database.path` | 运行时查询和状态存储 | Server/CLI 打开时自动迁移 |
    47	| Agent Registry | Server 内存 | 当前在线 Agent 连接池和待响应 RPC | 进程内状态，Server 重启后丢失 |
    48	| Web UI 本地存储 | Browser `localStorage` | JWT、远控偏好、主题/站点名称/刷新间隔等 UI 本地偏好 | 浏览器侧状态，不作为系统真相源 |
    49	
    50	当前的事实源不是单一的。Inventory 是声明式资产输入，SQLite 是 API 和 UI 的运行时查询面，Agent 心跳会更新在线状态和系统指标。需要区分“资产应当存在”和“节点当前在线/指标当前值”。
    51	
    52	## 代码模块边界
    53	
    54	| 模块 | 主要职责 |
    55	| --- | --- |
    56	| `internal/server` | HTTP 路由、Agent WebSocket、API 聚合、远程会话、代理、告警循环、指标清理 |
    57	| `internal/agent` | Agent 生命周期、能力检测、心跳、消息循环、采集器、远程代理执行 |
    58	| `internal/protocol` | Server 与 Agent 的 WebSocket 消息结构、消息类型与解码 helper |
    59	| `internal/storage` | GORM/SQLite 模型、迁移、资源 CRUD、用户、审计、告警、指标、代理配置 |
    60	| `internal/inventory` | Inventory YAML schema、解析、校验、同步到 storage |
    61	| `internal/sync` | Server 运行期 inventory 文件监听；复用 `internal/inventory.Syncer` 将声明式资源同步到 storage |
    62	| `internal/auth` | 登录、JWT、中间件、TOTP、密码重置 |
    63	| `internal/proxy` | Server 侧监听代理端口、Agent 侧连接目标并转发数据 |
    64	| `internal/pve` / `internal/docker` / `internal/openwrt` | 第三方平台客户端 |
    65	| `web/src/services` | 前端 API 客户端；统一通过 `/api` 与 Server 通信 |
    66	
    67	跨模块调用应尽量遵守这些边界。比如 API handler 不应直接解析 inventory 文件；应通过 storage 查询运行时结果。Agent 不应直接写 Server 数据库；应通过协议消息让 Server 持久化。
    68	
    69	## 控制流
    70	
    71	### Server 启动
    72	
    73	1. `cmd/cockpit` 读取 `-config`，或依次尝试 `./config/cockpit.yaml`、`./cockpit.yaml`、`/etc/cockpit/config.yaml`。
    74	2. Server 打开 SQLite 并自动迁移。
    75	3. 初始化 JWT、邮件配置、通知客户端、审计、代理管理器、远程会话管理器和 ticket 管理器。
    76	4. 必须通过 `ADMIN_PASSWORD` 初始化管理员密码；生产模式还会校验 `TOTP_ENCRYPTION_KEY`。
    77	5. 注册 HTTP/API/WebSocket 路由并开始监听。
    78	
    79	### Agent 注册与心跳
    80	
    81	1. `cockpit-agent start -server ws://host:9000/ws` 连接 Server；也可使用兼容入口 `cockpit agent -server ws://host:9000/ws`。
    82	2. Agent 运行能力检测器，形成 capability 列表。
    83	3. Agent 首包必须是 `register`。
    84	4. Server 校验重复连接和已配置的 Agent secret，接受后写入 SQLite 并加入内存 Registry。
    85	5. Agent 每 30 秒发送 `heartbeat`，其中可包含系统信息；Server 写入历史指标和快照。
    86	
    87	### Inventory 同步
    88	
    89	`cockpit sync` 是当前完整同步路径。它解析 `version: v1` 的 YAML，并同步以下资源：
    90	
    91	- Regions / Zones / Agents
    92	- Domains / Certificates
    93	- ComputeInstances
    94	- Services
    95	- Gateways
    96	- Storages
    97	
    98	Server 的 `inventory.watch: true` 会监听单个 inventory 文件并热加载，当前实现复用 `inventory.Syncer`，与 `cockpit sync` 走同一套资源同步逻辑。`inventory.strict: true` 会把 inventory 路径缺失、解析失败或初始同步失败视为启动错误；默认 `false` 时只记录日志并继续启动 Server。
    99	
   100	### HTTP API 与 Web UI
   101	
   102	Web UI 默认请求 `/api`。公开接口包括登录、Token 刷新、TOTP 验证、密码重置和 `/health`。其他 `/api/` 资源接口通过 JWT 中间件保护。
   103	
   104	主要 API 面：
   105	
   106	- `/api/status`
   107	- `/api/me`、`/api/me/profile`、`/api/me/password`
   108	- `/api/settings`
   109	- `/api/agents`、`/api/agents/{id}/secret`
   110	- `/api/resources/{compute-instances|domains|certificates|services|gateways|storages}`
   111	- `/api/users`（管理员管理用户）
   112	- `/api/alerts`（告警列表与已读标记）
   113	- `/api/metrics/*`
   114	- `/api/proxies`
   115	- `/api/remote/*`（tickets / sessions / terminal / guacamole / vault）
   116	- `/api/admin/audit/*`
   117	- `/api/auth/totp/*`（generate / enable / disable 需 JWT；verify 公开）
   118	
   119	### 远程访问与代理
   120	
   121	远程终端与桌面连接（Guacamole 隧道）使用短期 ticket：
   122	
   123	1. 浏览器先携带 JWT 请求 `POST /api/remote/tickets`。
   124	2. Server 生成 5 分钟有效、单次使用 ticket。
   125	3. 浏览器连接 `/api/remote/terminal` 或 `/api/remote/guacamole`，把 ticket 放在 `Sec-WebSocket-Protocol`。
   126	4. Server 校验 ticket 后，把连接请求转成 `proxy_*` 消息发给对应 Agent（终端直连目标；桌面经 `guac:` 中继到 guacd）。
   127	5. Agent 连接目标服务并把数据通过 Server 转发回浏览器。
   128	
   129	这个边界保证浏览器不需要知道内网拓扑，也不需要直接连 Agent。
   130	远控目标默认只允许 `remote_control.allowed_targets` 中显式配置的主机名、IP 或 CIDR。
   131	
   132	## 协议边界
   133	
   134	Server 与 Agent 的消息统一为：
   135	
   136	```json
   137	{
   138	  "id": "message-id",
   139	  "type": "heartbeat",
   140	  "timestamp": 1710000000,
   141	  "payload": {}
   142	}
   143	```
   144	
   145	当前消息类型：
   146	
   147	| 方向 | 类型 |
   148	| --- | --- |
   149	| Agent -> Server | `register`, `rpc_response`, `proxy_close`, `proxy_error`, `probe_report`, `exec_log` |
   150	| Server -> Agent | `rpc_request`, `ping`, `proxy_new` |
   151	| 双向 | `heartbeat`（Agent 上报、Server 回 ACK）、`error`, `proxy_data` |
   152	
   153	`register` 必须是 Agent 建立 `/ws` 后的第一条消息。RPC 使用请求消息 ID 关联响应。代理数据使用 `proxyId`、`connId` 关联具体连接。
   154	
   155	## 安全边界
   156	
   157	- Server 管理员密码必须通过 `ADMIN_PASSWORD` 提供，长度至少 8 位。
   158	- 生产环境应设置 `PRODUCTION=true` 和强随机 `TOTP_ENCRYPTION_KEY`。
   159	- `ALLOWED_ORIGINS` 控制 WebSocket Origin；未设置时会接受所有来源，仅适合开发环境。
   160	- Web UI 静态目录由 `STATIC_DIR` 或 `server.static_dir` 指定；未设置时根路径只返回 API 运行提示。
   161	- Agent secret 是可选但推荐的 Agent 认证边界；已配置 secret 的 Agent 重连必须提供匹配密钥。
   162	- 远程连接 ticket 是内存态、5 分钟有效、单次使用；Server 重启后失效。
   163	
   164	## 当前实现边界
   165	
   166	- Inventory 的目录拆分模型尚未成为主要实现路径；当前 CLI 示例和 parser 以单文件 `version: v1` YAML 为准。
   167	- `internal/agent/rpc` 的 SystemProvider 始终注册；Docker/PVE/OpenWrt Provider 由 `Agent.setupProviders` 按检测到的 capability 和环境变量（如 `DOCKER_HOST`、`PVE_TOKEN_ID`、`OPENWRT_HOST` 等）条件注册，凭据缺失时仅记录日志跳过，不影响基础心跳。
   168	- `/api/remote/sessions` 是内存会话台账（pending/connected/closed/failed），已接线到 terminal / guacamole 活会话生命周期（D-2026-10-12-4）；不是完整连接配置持久化，重启后台账清空。
   169	- Web UI 中部分偏好配置保存在浏览器本地，不进入 Server 数据库；`/api/settings` 当前仅作为兼容入口，前端本地偏好以浏览器存储值为准。
   170	- `docs/archive`（原 `docs/superpowers`）保存 TOTP、通知模块等历史设计稿和实施计划，仅供追溯，不代表当前运行架构；详见 [`docs/archive/README.md`](../archive/README.md)。
     1	# 协议与 API 边界
     2	
     3	> 语言：**中文** ｜ [English](./protocol.en.md)
     4	
     5	本文记录当前 Server、Agent、Web UI 之间的通信边界。
     6	
     7	## 通信面
     8	
     9	| 通信面 | 发起方 | 入口 | 认证 |
    10	| --- | --- | --- | --- |
    11	| Web UI HTTP API | Browser | `/api/*` | JWT Bearer；登录和密码重置等公开接口除外 |
    12	| Agent 控制通道 | Agent | `/ws` | 首条 `register` 消息；可选 Agent secret |
    13	| 远程终端 | Browser | `/api/remote/terminal` | 短期 ticket，经 `Sec-WebSocket-Protocol` 传递 |
    14	| Guacamole 桌面（RDP/VNC/SSH） | Browser | `/api/remote/guacamole` | 短期 ticket，经 `Sec-WebSocket-Protocol` 传递 |
    15	
    16	## Agent WebSocket
    17	
    18	Agent 使用 WebSocket 主动连接 Server：
    19	
    20	```text
    21	cockpit-agent start -server ws://server:9000/ws
    22	```
    23	
    24	兼容入口 `cockpit agent -server ws://server:9000/ws` 使用同一套启动参数。
    25	
    26	连接建立后，第一条消息必须是 `register`。如果第一条消息不是注册消息，Server 会关闭连接。
    27	
    28	消息 envelope：
    29	
    30	```json
    31	{
    32	  "id": "msg_...",
    33	  "type": "register",
    34	  "timestamp": 1710000000,
    35	  "payload": {}
    36	}
    37	```
    38	
    39	### register
    40	
    41	方向：Agent -> Server
    42	
    43	```json
    44	{
    45	  "type": "register",
    46	  "payload": {
    47	    "agentId": "agent-home-server01",
    48	    "secret": "optional-agent-secret",
    49	    "location": {
    50	      "region": "home",
    51	      "zone": "datacenter"
    52	    },
    53	    "hostname": "server01",
    54	    "capabilities": [
    55	      {
    56	        "type": "docker-api",
    57	        "endpoint": "unix:///var/run/docker.sock"
    58	      }
    59	    ],
    60	    "virtualization": {
    61	      "type": "kvm",
    62	      "role": "guest"
    63	    },
    64	    "labels": {
    65	      "env": "home"
    66	    }
    67	  }
    68	}
    69	```
    70	
    71	Server 接受后返回同类型消息：
    72	
    73	```json
    74	{
    75	  "type": "register",
    76	  "payload": {
    77	    "status": "accepted",
    78	    "serverTime": 1710000000,
    79	    "heartbeatInterval": 30
    80	  }
    81	}
    82	```
    83	
    84	注册约束：
    85	
    86	- `agentId` 为空时 Server 可生成 ID，但 Agent 当前也会基于主机名生成默认 ID。
    87	- 如果数据库中已有该 Agent 且配置了 secret，注册必须提供匹配 secret。
    88	- Registry 中已有同 ID 活跃连接时，新连接会被拒绝。
    89	
    90	### heartbeat
    91	
    92	方向：Agent -> Server
    93	
    94	Agent 每 30 秒上报一次心跳。心跳可包含系统信息，Server 会写入系统指标历史和快照。
    95	
    96	```json
    97	{
    98	  "type": "heartbeat",
    99	  "payload": {
   100	    "agentId": "agent-home-server01",
   101	    "status": "online",
   102	    "systemInfo": {
   103	      "hostname": "server01",
   104	      "cpuUsage": 12.3,
   105	      "memUsagePercent": 48.1,
   106	      "diskUsagePercent": 61.5,
   107	      "osName": "linux",
   108	      "arch": "amd64"
   109	    }
   110	  }
   111	}
   112	```
   113	
   114	Server 返回 `heartbeat` ACK，并复用请求消息 ID。
   115	
   116	### rpc_request / rpc_response
   117	
   118	方向：Server -> Agent -> Server
   119	
   120	RPC payload 使用 `method` 和 `params`。方法名按 `<provider>.<action>` 解析；没有 provider 前缀时默认 provider 是 `system`。
   121	
   122	```json
   123	{
   124	  "type": "rpc_request",
   125	  "payload": {
   126	    "method": "system.status",
   127	    "params": {}
   128	  }
   129	}
   130	```
   131	
   132	响应必须复用请求 ID：
   133	
   134	```json
   135	{
   136	  "type": "rpc_response",
   137	  "payload": {
   138	    "status": "success",
   139	    "data": {}
   140	  }
   141	}
   142	```
   143	
   144	注意：SystemProvider 在 Agent 启动时始终注册；PVE、Docker、OpenWrt Provider 由 `Agent.setupProviders` 按检测到的 capability 和环境变量（如 `PVE_URL`/`PVE_TOKEN_ID`/`PVE_TOKEN_SECRET`、`DOCKER_HOST`、`OPENWRT_HOST`/`OPENWRT_USER`/`OPENWRT_PASS`）条件注册。凭据缺失时该 Provider 仅记录日志跳过，不影响其他 Provider 和基础心跳。
   145	
   146	## 代理消息
   147	
   148	代理消息用于 Server 通过在线 Agent 访问 Agent 本机或其网络可达的 TCP 目标。
   149	
   150	> 注：代理仅支持 TCP 转发；`proxyType` 入参非 `tcp` 一律 400 拒绝（`udp`
   151	> 曾被接受但数据面仍按 TCP 拨号——静默错转，已改为诚实失败，见 D-2026-10-08-2）。
   152	
   153	| 类型 | 方向 | 用途 |
   154	| --- | --- | --- |
   155	| `proxy_new` | Server -> Agent | 要求 Agent 建立到目标的连接 |
   156	| `proxy_data` | 双向 | 转发连接数据 |
   157	| `proxy_close` | 双向 | 关闭连接 |
   158	| `proxy_error` | Agent -> Server | 报告代理错误 |
   159	
   160	关键字段：
   161	
   162	- `proxyId`：代理配置或会话 ID。
   163	- `connId`：单条连接 ID。
   164	- `target`：Agent 侧要连接的 `host:port`。
   165	- `data`：数据内容；JSON 编码时可能是 base64 字符串或字节数组。
   166	
   167	## 远程连接 ticket
   168	
   169	Browser 不直接把 JWT 放进远程 WebSocket URL。流程是：
   170	
   171	1. Browser 携带 JWT 调 `POST /api/remote/tickets`。
   172	2. Server 生成 5 分钟有效、单次使用 ticket。
   173	3. Browser 建立 WebSocket，把 ticket 作为 `Sec-WebSocket-Protocol` 的第一个值。
   174	4. Server 校验并消费 ticket，然后开始转发。
   175	
   176	创建 ticket：
   177	
   178	```http
   179	POST /api/remote/tickets
   180	Authorization: Bearer <jwt>
   181	Content-Type: application/json
   182	```
   183	
   184	```json
   185	{
   186	  "agent_id": "agent-home-server01",
   187	  "host": "127.0.0.1",
   188	  "port": 22,
   189	  "protocol": "ssh"
   190	}
   191	```
   192	
   193	响应：
   194	
   195	```json
   196	{
   197	  "ticket": "random-ticket-id",
   198	  "expires_at": "2026-06-11T12:00:00Z"
   199	}
   200	```
   201	
   202	随后连接：
   203	
   204	```javascript
   205	new WebSocket("ws://server:9000/api/remote/terminal", [ticket])
   206	```
   207	
   208	远程目标默认只允许连接 `remote_control.allowed_targets` 中显式配置的主机名、IP 或 CIDR；设置 `remote_control.allow_arbitrary_target: true` 后才允许任意目标。当前实现不会从 inventory 自动派生远控 allow-list。
   209	
   210	RDP/VNC/SSH 桌面走 Guacamole 隧道（`/api/remote/guacamole`）：guacd 终结协议，Agent 只做 `guac:` 前缀的 TCP 中继，无特殊构建要求。
   211	
   212	## HTTP API
   213	
   214	公开接口：
   215	
   216	- `GET /health`
   217	- `POST /api/auth/login`
   218	- `POST /api/auth/refresh`
   219	- `POST /api/auth/totp/verify`
   220	- `POST /api/auth/forgot-password`
   221	- `POST /api/auth/reset-password`
   222	- `POST /api/auth/verify-reset-code`
   223	
   224	JWT 保护接口：
   225	
   226	- `GET /api/status`
   227	- `GET /api/me`
   228	- `PUT /api/me/profile`
   229	- `POST|PUT /api/me/password`
   230	- `GET|PUT /api/settings`
   231	- `GET /api/agents`
   232	- `GET /api/agents/{id}`
   233	- `GET|POST|DELETE /api/agents/{id}/secret`
   234	- `GET /api/resources/{type}`
   235	- `GET /api/resources/{type}/{id}`
   236	- `GET|POST /api/users`（创建限管理员）
   237	- `PUT|DELETE /api/users/{id}`
   238	- `POST /api/users/{id}/password`
   239	- `GET /api/alerts`
   240	- `PUT /api/alerts/read-all`


[showing lines 105-790 of 790; full output saved to /home/cui/.local/share/opencode/shell/2b220c9cad2a7053c583eada91fc7bca7b5b6399/sh_14f062795002K6Rz04vw7MxWus.out]
[0m$ [0mnl -ba internal/agent/rpc/job_provider.go | sed -n '1,280p'; nl -ba internal/agent/rpc/job_provider_unix.go; nl -ba internal/agent/rpc/job_provider_windows.go; nl -ba internal/agent/rpc/file_provider.go | sed -n '1,420p'
1	package rpc
     2	
     3	import (
     4		"context"
     5		"fmt"
     6		"os/exec"
     7		"strings"
     8		"sync"
     9		"time"
    10	)
    11	
    12	// Job Provider：控制面的统一执行通道（设计见 docs/guide/jobs-design.md）。
    13	//
    14	// 当前仅含 exec：在 Agent 本机带超时执行一条命令，返回退出码与输出尾部
    15	// （截断上限与 server 侧 Job 行列对齐）。非零退出码与执行失败均以业务
    16	// 字段形式返回（exit_code / error），RPC 层不视为协议错误——server 据此
    17	// 落 job 状态（exit 0 → success，否则 failed，输出仍带回展示）。
    18	type JobProvider struct {
    19		mu       sync.Mutex             // guards sessions
    20		sessions map[string]jobSession  // jobId → ctx cancel；exec 期间持有，结束后注销
    21	}
    22	
    23	type jobSession struct {
    24		cancel context.CancelFunc
    25	}
    26	
    27	// NewJobProvider 创建 Job Provider（exec 平台无关，无条件注册）
    28	func NewJobProvider() *JobProvider { return &JobProvider{sessions: make(map[string]jobSession)} }
    29	
    30	func (p *JobProvider) Type() string { return "job" }
    31	
    32	// Call 分发 <job>.<action>；当前含 exec、cancel（M2c）
    33	func (p *JobProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
    34		switch action {
    35		case "exec":
    36			return p.Exec(params)
    37		case "cancel":
    38			return p.Cancel(params)
    39		default:
    40			return nil, fmt.Errorf("unknown action: %s", action)
    41		}
    42	}
    43	
    44	const (
    45		// jobExecMaxTimeout 命令超时上限（秒），防误配把 agent 挂死
    46		jobExecMaxTimeout = 300
    47		// jobExecMaxCommand 命令长度上限
    48		jobExecMaxCommand = 16 * 1024
    49		// jobExecMaxOutput 输出保留上限：只留尾部（与 storage.JobMaxOutput 对齐）
    50		jobExecMaxOutput = 64 * 1024
    51	)
    52	
    53	// ExecParams 入参：command（必填）、timeout_s（可选，缺省 60，上限
    54	// jobExecMaxTimeout）。其余字段忽略（保留给后续 job.* 动作）。
    55	type ExecParams struct {
    56		Command  string `json:"command"`
    57		TimeoutS int  `json:"timeout_s"`
    58		// ID 是 job 唯一标识（server 下发 job.exec 时必带，用于 cancel 定位句柄）。
    59		// 旧版本 server 可能不传此字段；无 id 则不注册句柄表，cancel 幂等空操作。
    60		ID string `json:"id,omitempty"`
    61	}
    62	
    63	// Exec 执行结果：exit_code（-1 = 启动失败/超时被杀）、output（截断尾部）、
    64	// truncated、error（异常说明，成功为空）、duration_ms
    65	func (p *JobProvider) Exec(params map[string]interface{}) (interface{}, error) {
    66		var ep ExecParams
    67		if v, ok := params["command"].(string); ok {
    68			ep.Command = v
    69		}
    70		// timeout_s 经 RPC JSON 解码为 float64；直调测试传 int。两者都收
    71		if v, ok := params["timeout_s"].(float64); ok {
    72			if v > 0 {
    73				ep.TimeoutS = int(v)
    74			}
    75		} else if v, ok := params["timeout_s"].(int); ok {
    76			if v > 0 {
    77				ep.TimeoutS = v
    78			}
    79		}
    80		// id 由 server 提供；存在性非硬依赖（旧调用不含 id 也合法）
    81		if v, ok := params["id"].(string); ok {
    82			ep.ID = v
    83		}
    84	
    85		ep.Command = strings.TrimSpace(ep.Command)
    86		if ep.Command == "" {
    87			return nil, fmt.Errorf("command is required")
    88		}
    89		if len(ep.Command) > jobExecMaxCommand {
    90			return nil, fmt.Errorf("command too large (max %d bytes)", jobExecMaxCommand)
    91		}
    92		if ep.TimeoutS > jobExecMaxTimeout {
    93			return nil, fmt.Errorf("timeout too large (max %ds)", jobExecMaxTimeout)
    94		}
    95		if ep.TimeoutS == 0 {
    96			ep.TimeoutS = 60
    97		}
    98	
    99		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ep.TimeoutS)*time.Second)
   100	
   101		// 按 id 注册句柄（取消信号查找用）；无 id 或注册冲突不阻塞主流程
   102		p.registerSession(ep.ID, cancel)
   103	
   104		cmd := newJobCmd(ctx, ep.Command)
   105	
   106		start := time.Now()
   107		out, err := cmd.CombinedOutput()
   108		duration := time.Since(start)
   109	
   110		// 注销句柄（无论成功/失败/超时）。注意：注销前不再调用 cancel，cmd 已
   111		// 终结上下文；注销顺序 LIFO（注册先 → 注销先）
   112		p.unregisterSession(ep.ID)
   113	
   114		// 截断到尾部（保留末尾 64KB——错误信息通常在尾部）
   115		truncated := false
   116		if len(out) > jobExecMaxOutput {
   117			out = out[len(out)-jobExecMaxOutput:]
   118			truncated = true
   119		}
   120	
   121		res := map[string]interface{}{
   122			"exit_code": -1,
   123			"output":    string(out),
   124			"truncated": truncated,
   125			"error":     "",
   126			"duration_ms": duration.Milliseconds(),
   127		}
   128		if err != nil {
   129			exitCode := -1
   130			if ee, ok := err.(*exec.ExitError); ok {
   131				exitCode = ee.ExitCode()
   132			}
   133			res["exit_code"] = exitCode
   134			switch {
   135			case ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled:
   136				res["error"] = fmt.Sprintf("timed out after %ds", ep.TimeoutS)
   137			default:
   138				res["error"] = trimExecErr(err.Error())
   139			}
   140			return res, nil
   141		}
   142		// 正常结束时 CombinedOutput 可能为空；退出码 0
   143		res["exit_code"] = 0
   144		return res, nil
   145	}
   146	
   147	// Cancel 终止在途 job（C1-C5）：按 id 找到 ctx cancel 并触发之（进程组 SIGKILL）；
   148	// 未知 id 返回成功空操作（幂等语义，C5）——让 server 无法区分「已停」与「早已结束」。
   149	func (p *JobProvider) Cancel(params map[string]interface{}) (interface{}, error) {
   150		id, _ := params["id"].(string)
   151		if id == "" {
   152			return map[string]interface{}{"cancelled": false}, nil
   153		}
   154		p.mu.Lock()
   155		sess, ok := p.sessions[id]
   156		p.mu.Unlock()
   157		if !ok {
   158			// 已注销（exec 自然结束）或未注册（无 id 的旧调用）；幂等成功
   159			return map[string]interface{}{"cancelled": false}, nil
   160		}
   161		sess.cancel()
   162		return map[string]interface{}{"cancelled": true}, nil
   163	}
   164	
   165	// registerSession 将 exec 的 ctx cancel 注入句柄表（线程安全）；id 为空或已存在即忽略。
   166	func (p *JobProvider) registerSession(id string, cancel context.CancelFunc) {
   167		if id == "" {
   168			return
   169		}
   170		p.mu.Lock()
   171		defer p.mu.Unlock()
   172		if _, exists := p.sessions[id]; !exists {
   173			p.sessions[id] = jobSession{cancel: cancel}
   174		}
   175	}
   176	
   177	// unregisterSession 移除句柄表条目（exec 结束后调用）。
   178	func (p *JobProvider) unregisterSession(id string) {
   179		if id == "" {
   180			return
   181		}
   182		p.mu.Lock()
   183		defer p.mu.Unlock()
   184		delete(p.sessions, id)
   185	}
   186	
   187	// trimExecErr 错误串瘦身（exec 错误可能带完整命令原文与路径噪音）
   188	func trimExecErr(s string) string {
   189		s = strings.TrimSpace(s)
   190		if len(s) > 512 {
   191			return s[:512] + "..."
   192		}
   193		return s
   194	}
     1	//go:build unix
     2	
     3	package rpc
     4	
     5	import (
     6		"context"
     7		"os/exec"
     8		"syscall"
     9	)
    10	
    11	// newJobCmd 构造 unix 侧执行命令：sh -c，且进程组隔离——
    12	// CommandContext 默认只在超时时杀直接子进程（sh），sh 派生的孙进程
    13	// 会继续运行并把输出管道握到结束，CombinedOutput 因此无法按时返回。
    14	// Setpgid 让 sh 成为进程组首领，Cancel 改为杀整组：sleep 这类孙进程
    15	// 一并被 SIGKILL，管道立刻 EOF，超时语义才是真的超时。
    16	func newJobCmd(ctx context.Context, command string) *exec.Cmd {
    17		cmd := exec.CommandContext(ctx, "sh", "-c", command)
    18		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
    19		cmd.Cancel = func() error {
    20			if cmd.Process == nil {
    21				return nil
    22			}
    23			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
    24		}
    25		return cmd
    26	}
     1	//go:build windows
     2	
     3	package rpc
     4	
     5	import (
     6		"context"
     7		"os/exec"
     8	)
     9	
    10	// newJobCmd 构造 windows 侧执行命令：cmd /C（无 /bin/sh）。
    11	// Windows 无进程组 SIGKILL 语义，沿用 CommandContext 默认杀直接进程；
    12	// cmd /C 通常直接 exec 目标命令，超时语义由父链保证。
    13	func newJobCmd(ctx context.Context, command string) *exec.Cmd {
    14		return exec.CommandContext(ctx, "cmd", "/C", command)
    15	}
     1	package rpc
     2	
     3	import (
     4		"bufio"
     5		"bytes"
     6		"encoding/base64"
     7		"fmt"
     8		"io/fs"
     9		"os"
    10		"path/filepath"
    11		"regexp"
    12		"sort"
    13		"strings"
    14		"time"
    15	)
    16	
    17	// ============ File Provider ============
    18	//
    19	// Agent 主机远程文件管理：浏览 / 分块读 / 写（覆盖与追加）/ 建目录 / 删除 /
    20	// 改名。全文件系统可见（Agent 本有终端，等价能力，见 file-manager-design.md D1），
    21	// 安全靠路径校验与 symlink 拒绝（D7）。全部同步 RPC——单块 ≤1MB 的分块模型
    22	// 把每次调用压在 30s 超时内，大文件由 server 循环分块拉取（下载流转发）。
    23	
    24	const (
    25		// fileReadChunkLimit file.read 单块字节上限（server 侧用 256KB 分块拉）
    26		fileReadChunkLimit = 1024 * 1024
    27		// fileWriteChunkLimit file.write 单块字节上限（base64 解码后）
    28		fileWriteChunkLimit = 1024 * 1024
    29	)
    30	
    31	// FileReadChunkLimit / FileWriteChunkLimit 供 capability metadata 与 server 校验复用
    32	func FileReadChunkLimit() int  { return fileReadChunkLimit }
    33	func FileWriteChunkLimit() int { return fileWriteChunkLimit }
    34	
    35	// fileNameRe rename 目标：纯文件名，不含路径分隔符
    36	var fileNameRe = regexp.MustCompile(`^[^/\x00]{1,255}$`)
    37	
    38	// FileProvider 远程文件管理 Provider（无状态，全平台注册）
    39	type FileProvider struct{}
    40	
    41	func NewFileProvider() *FileProvider { return &FileProvider{} }
    42	
    43	func (p *FileProvider) Type() string { return "file" }
    44	
    45	func (p *FileProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
    46		switch action {
    47		case "list":
    48			return p.List(paramString(params, "dir"))
    49		case "read":
    50			return p.Read(params)
    51		case "write":
    52			return p.Write(params)
    53		case "mkdir":
    54			return p.Mkdir(paramString(params, "path"))
    55		case "delete":
    56			return p.Delete(paramString(params, "path"))
    57		case "rename":
    58			return p.Rename(paramString(params, "path"), paramString(params, "name"))
    59		case "chmod":
    60			return p.Chmod(params)
    61		case "chown":
    62			return p.Chown(params)
    63		case "search":
    64			return p.Search(params)
    65		default:
    66			return nil, fmt.Errorf("unknown file action: %s", action)
    67		}
    68	}
    69	
    70	// cleanAbsPath 路径安全校验：Clean 后必须是绝对路径且不为根目录。
    71	// 拒绝相对路径与 `..` 形态（Clean 已消解内部 `..`，残留者必然非绝对）。
    72	func cleanAbsPath(path, what string) (string, error) {
    73		cleaned := filepath.Clean(path)
    74		if !filepath.IsAbs(cleaned) {
    75			return "", fmt.Errorf("%s must be an absolute path: %q", what, path)
    76		}
    77		if cleaned == string(filepath.Separator) {
    78			return "", fmt.Errorf("%s must not be the filesystem root", what)
    79		}
    80		return cleaned, nil
    81	}
    82	
    83	// lstatEntry 取条目信息，symlink 返回链接目标（不跟随）
    84	func lstatEntry(path string) (os.FileInfo, bool, string, error) {
    85		info, err := os.Lstat(path)
    86		if err != nil {
    87			return nil, false, "", err
    88		}
    89		isSymlink := info.Mode()&os.ModeSymlink != 0
    90		target := ""
    91		if isSymlink {
    92			if t, err := os.Readlink(path); err == nil {
    93				target = t
    94			}
    95		}
    96		return info, isSymlink, target, nil
    97	}
    98	
    99	// List 列目录直属条目（不递归），目录优先 + 名称排序
   100	func (p *FileProvider) List(dir string) (interface{}, error) {
   101		cleaned, err := cleanAbsPath(dir, "dir")
   102		if err != nil {
   103			return nil, err
   104		}
   105		entries, err := os.ReadDir(cleaned)
   106		if err != nil {
   107			return nil, fmt.Errorf("read dir: %w", err)
   108		}
   109		out := make([]map[string]interface{}, 0, len(entries))
   110		for _, e := range entries {
   111			full := filepath.Join(cleaned, e.Name())
   112			info, isSymlink, target, err := lstatEntry(full)
   113			if err != nil {
   114				continue // 竞态删除等瞬时错误跳过
   115			}
   116			isDir := info.IsDir()
   117			uid, gid := fileStatOwner(full)
   118			out = append(out, map[string]interface{}{
   119				"name":      e.Name(),
   120				"size":      info.Size(),
   121				"mode":      fmt.Sprintf("%04o", uint32(info.Mode().Perm())),
   122				"mtime":     info.ModTime().Unix(),
   123				"uid":       uid,
   124				"gid":       gid,
   125				"isDir":     isDir,
   126				"isSymlink": isSymlink,
   127				"target":    target,
   128			})
   129		}
   130		sort.Slice(out, func(i, j int) bool {
   131			if out[i]["isDir"] != out[j]["isDir"] {
   132				return out[i]["isDir"].(bool) // 目录在前
   133			}
   134			return out[i]["name"].(string) < out[j]["name"].(string)
   135		})
   136		return map[string]interface{}{
   137			"dir":     cleaned,
   138			"entries": out,
   139		}, nil
   140	}
   141	
   142	// Read 分块读普通文件（symlink 拒绝，D7）。与 backup.read 同构。
   143	func (p *FileProvider) Read(params map[string]interface{}) (interface{}, error) {
   144		path, err := cleanAbsPath(paramString(params, "path"), "path")
   145		if err != nil {
   146			return nil, err
   147		}
   148		offset := int64(paramFloat(params, "offset"))
   149		length := int64(paramFloat(params, "length"))
   150		if length <= 0 || length > fileReadChunkLimit {
   151			length = fileReadChunkLimit
   152		}
   153		info, isSymlink, _, err := lstatEntry(path)
   154		if err != nil {
   155			return nil, fmt.Errorf("stat: %w", err)
   156		}
   157		if isSymlink {
   158			return nil, fmt.Errorf("refusing to read symlink: %s", path)
   159		}
   160		if !info.Mode().IsRegular() {
   161			return nil, fmt.Errorf("not a regular file: %s", path)
   162		}
   163		total := info.Size()
   164		if offset >= total {
   165			return map[string]interface{}{"data": "", "size": total, "eof": true}, nil
   166		}
   167		f, err := os.Open(path)
   168		if err != nil {
   169			return nil, fmt.Errorf("open: %w", err)
   170		}
   171		defer f.Close()
   172		if _, err := f.Seek(offset, 0); err != nil {
   173			return nil, fmt.Errorf("seek: %w", err)
   174		}
   175		if remaining := total - offset; length > remaining {
   176			length = remaining
   177		}
   178		buf := make([]byte, length)
   179		n, err := f.Read(buf)
   180		if err != nil && n == 0 {
   181			return nil, fmt.Errorf("read: %w", err)
   182		}
   183		return map[string]interface{}{
   184			"data": base64.StdEncoding.EncodeToString(buf[:n]),
   185			"size": total,
   186			"eof":  offset+int64(n) >= total,
   187		}, nil
   188	}
   189	
   190	// Write 写文件：truncate=true 覆盖/新建，false 追加（上传分块续传，文件须已存在）。
   191	// symlink 拒绝（防经链接写穿）；返回写入后的文件总大小。
   192	func (p *FileProvider) Write(params map[string]interface{}) (interface{}, error) {
   193		path, err := cleanAbsPath(paramString(params, "path"), "path")
   194		if err != nil {
   195			return nil, err
   196		}
   197		data, err := base64.StdEncoding.DecodeString(paramString(params, "data"))
   198		if err != nil {
   199			return nil, fmt.Errorf("data must be base64")
   200		}
   201		if len(data) > fileWriteChunkLimit {
   202			return nil, fmt.Errorf("chunk too large: %d > %d bytes", len(data), fileWriteChunkLimit)
   203		}
   204		truncate, _ := params["truncate"].(bool)
   205	
   206		if info, isSymlink, _, err := lstatEntry(path); err == nil {
   207			if isSymlink {
   208				return nil, fmt.Errorf("refusing to write through symlink: %s", path)
   209			}
   210			if info.IsDir() {
   211				return nil, fmt.Errorf("path is a directory: %s", path)
   212			}
   213		} else if os.IsNotExist(err) {
   214			// 追加模式要求文件已存在：分块上传首块建文件，后续块若发现
   215			// 目标丢失（被删/被移走），继续追加会产出损坏的文件碎片
   216			if !truncate {
   217				return nil, fmt.Errorf("append target does not exist: %s", path)
   218			}
   219		} else {
   220			return nil, fmt.Errorf("stat: %w", err)
   221		}
   222	
   223		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
   224			return nil, fmt.Errorf("create parent dir: %w", err)
   225		}
   226		// mode 可选：0/缺省 = 0644（既有行为）；白名单只放私钥 0600 与常规 0644，
   227		// 防误设 0777 之类（ACME 部署用它写私钥，见 acme-design.md D14）
   228		perm := os.FileMode(0o644)
   229		explicit := false
   230		if raw, ok := params["mode"]; ok && raw != nil && raw != float64(0) {
   231			mode, ok := raw.(float64)
   232			if !ok {
   233				return nil, fmt.Errorf("mode must be a number")
   234			}
   235			switch os.FileMode(mode) {
   236			case 0o600, 0o644:
   237				perm = os.FileMode(mode)
   238				explicit = mode == 0o600
   239			default:
   240				return nil, fmt.Errorf("mode must be 0600 or 0644")
   241			}
   242		}
   243		flags := os.O_CREATE | os.O_WRONLY
   244		if truncate {
   245			flags |= os.O_TRUNC
   246		} else {
   247			flags |= os.O_APPEND
   248		}
   249		f, err := os.OpenFile(path, flags, perm)
   250		if err != nil {
   251			return nil, fmt.Errorf("open: %w", err)
   252		}
   253		// 已存在文件的权限不受 OpenFile perm 影响：显式 0600（私钥部署覆盖写）
   254		// 需要补 chmod 落准；缺省路径不 chmod，保持既有行为（设备文件等特殊目标）
   255		if explicit {
   256			if err := f.Chmod(perm); err != nil {
   257				f.Close()
   258				return nil, fmt.Errorf("chmod: %w", err)
   259			}
   260		}
   261		defer f.Close()
   262		if _, err := f.Write(data); err != nil {
   263			return nil, fmt.Errorf("write: %w", err)
   264		}
   265		info, err := fileStat(f)
   266		if err != nil {
   267			return nil, fmt.Errorf("stat: %w", err)
   268		}
   269		return map[string]interface{}{"size": info.Size()}, nil
   270	}
   271	
   272	// Mkdir 新建目录（支持多级）；已存在报错，防误把已有目录当新建
   273	func (p *FileProvider) Mkdir(path string) (interface{}, error) {
   274		cleaned, err := cleanAbsPath(path, "path")
   275		if err != nil {
   276			return nil, err
   277		}
   278		if _, err := os.Lstat(cleaned); err == nil {
   279			return nil, fmt.Errorf("path already exists: %s", cleaned)
   280		}
   281		if err := os.MkdirAll(cleaned, 0o755); err != nil {
   282			return nil, fmt.Errorf("mkdir: %w", err)
   283		}
   284		return map[string]interface{}{}, nil
   285	}
   286	
   287	// Delete 删除文件或递归删目录；symlink 只删链接本身（Remove 不跟随）
   288	func (p *FileProvider) Delete(path string) (interface{}, error) {
   289		cleaned, err := cleanAbsPath(path, "path")
   290		if err != nil {
   291			return nil, err
   292		}
   293		if err := os.RemoveAll(cleaned); err != nil {
   294			return nil, fmt.Errorf("remove: %w", err)
   295		}
   296		return map[string]interface{}{}, nil
   297	}
   298	
   299	// Rename 同目录内改名：name 只允许纯文件名，目标已存在拒绝（不静默覆盖）
   300	func (p *FileProvider) Rename(path, name string) (interface{}, error) {
   301		cleaned, err := cleanAbsPath(path, "path")
   302		if err != nil {
   303			return nil, err
   304		}
   305		if !fileNameRe.MatchString(name) || name == "." || name == ".." {
   306			return nil, fmt.Errorf("invalid name: %q", name)
   307		}
   308		target := filepath.Join(filepath.Dir(cleaned), name)
   309		if _, err := os.Lstat(target); err == nil {
   310			return nil, fmt.Errorf("target already exists: %s", name)
   311		}
   312		if err := os.Rename(cleaned, target); err != nil {
   313			return nil, fmt.Errorf("rename: %w", err)
   314		}
   315		return map[string]interface{}{"path": target}, nil
   316	}
   317	
   318	// ============ 权限编辑（见 file-manager-design.md M4，D20-D21） ============
   319	
   320	// fileMaxID uid/gid 上限（uint32 语义）
   321	const fileMaxID = 1<<32 - 1
   322	
   323	// chmodChownTarget 校验路径并拒绝 symlink（D7 延伸：对链接 chmod/chown
   324	// 会作用到目标，给「不跟随」纪律开例外）
   325	func chmodChownTarget(path string) (string, os.FileInfo, error) {
   326		cleaned, err := cleanAbsPath(path, "path")
   327		if err != nil {
   328			return "", nil, err
   329		}
   330		info, err := os.Lstat(cleaned)
   331		if err != nil {
   332			return "", nil, fmt.Errorf("path not found: %s", cleaned)
   333		}
   334		if info.Mode()&os.ModeSymlink != 0 {
   335			return "", nil, fmt.Errorf("refusing to operate on a symlink: %s", cleaned)
   336		}
   337		return cleaned, info, nil
   338	}
   339	
   340	// numParam 取数字参数（JSON number 必为 float64），要求非负整数
   341	func numParam(params map[string]interface{}, key string, max float64) (float64, error) {
   342		raw, ok := params[key]
   343		if !ok || raw == nil {
   344			return 0, fmt.Errorf("%s is required", key)
   345		}
   346		v, ok := raw.(float64)
   347		if !ok || v != float64(int64(v)) || v < 0 || v > max {
   348			return 0, fmt.Errorf("%s must be an integer in [0, %d]", key, int64(max))
   349		}
   350		return v, nil
   351	}
   352	
   353	// Chmod 修改权限位（0-0o777，不碰 setuid/sticky）
   354	func (p *FileProvider) Chmod(params map[string]interface{}) (interface{}, error) {
   355		cleaned, _, err := chmodChownTarget(paramString(params, "path"))
   356		if err != nil {
   357			return nil, err
   358		}
   359		mode, err := numParam(params, "mode", 0o777)
   360		if err != nil {
   361			return nil, err
   362		}
   363		if err := os.Chmod(cleaned, os.FileMode(mode)); err != nil {
   364			return nil, fmt.Errorf("chmod: %w", err)
   365		}
   366		return map[string]interface{}{"mode": fmt.Sprintf("%04o", int(mode))}, nil
   367	}
   368	
   369	// Chown 修改属主 uid/gid（不做用户名解析——跨机用户名空间不可靠）
   370	func (p *FileProvider) Chown(params map[string]interface{}) (interface{}, error) {
   371		cleaned, _, err := chmodChownTarget(paramString(params, "path"))
   372		if err != nil {
   373			return nil, err
   374		}
   375		uid, err := numParam(params, "uid", fileMaxID)
   376		if err != nil {
   377			return nil, err
   378		}
   379		gid, err := numParam(params, "gid", fileMaxID)
   380		if err != nil {
   381			return nil, err
   382		}
   383		if err := os.Chown(cleaned, int(uid), int(gid)); err != nil {
   384			return nil, fmt.Errorf("chown: %w", err)
   385		}
   386		return map[string]interface{}{"uid": int(uid), "gid": int(gid)}, nil
   387	}
   388	
   389	// ============ 文本搜索（见 file-manager-design.md M2，D10-D11） ============
   390	
   391	const (
   392		// fileSearchMaxDepth 递归深度上限
   393		fileSearchMaxDepth = 8
   394		// fileSearchMaxFiles 扫描文件总数上限（防大目录树）
   395		fileSearchMaxFiles = 5000
   396		// fileSearchMaxMatches 命中条数上限（达到即停）
   397		fileSearchMaxMatches = 200
   398		// fileSearchMaxFileSize 单文件参与搜索的大小上限（与编辑器 read 上限一致）
   399		fileSearchMaxFileSize = 1024 * 1024
   400		// fileSearchMaxQuery 关键词长度上限
   401		fileSearchMaxQuery = 256
   402		// fileSearchLineMaxChars 命中行文本截断长度
   403		fileSearchLineMaxChars = 200
   404	)
   405	
   406	// fileSearchTimeout 单次搜索总超时（var：测试注入短超时以覆盖 deadline 分支）
   407	var fileSearchTimeout = 15 * time.Second
   408	
   409	// Search 目录内递归文本搜索：纯文本 contains（D10，防 ReDoS 与 logs grep 同纪律）、
   410	// 默认大小写不敏感；跳过 symlink/二进制/大文件（D11），各上限置 truncated。
   411	// 命中即停返回已得结果——搜索是浏览性质操作，部分结果同样可用。
   412	func (p *FileProvider) Search(params map[string]interface{}) (interface{}, error) {
   413		dir, err := cleanAbsPath(paramString(params, "dir"), "dir")
   414		if err != nil {
   415			return nil, err
   416		}
   417		query := paramString(params, "query")
   418		if query == "" {
   419			return nil, fmt.Errorf("query required")
   420		}
[0m$ [0mnl -ba internal/server/server.go | sed -n '298,520p'; nl -ba internal/server/rbac.go | sed -n '230,360p'; nl -ba internal/server/api_jobs.go | sed -n '1,320p'; nl -ba internal/server/remote_session.go | sed -n '1,260p'
298	// registerRoutes 注册所有 HTTP 路由
   299	func (s *Server) registerRoutes(mux *http.ServeMux) {
   300		// 注册审计日志 API
   301		s.registerAuditAPI(mux)
   302	
   303		// 注册代理 API
   304		s.registerProxyAPI(mux)
   305	
   306		// 注册系统指标 API
   307		s.registerMetricsAPI(mux)
   308		s.registerDockerAPI(mux)
   309	
   310		// 注册 Compose Stack API
   311		s.registerStacksAPI(mux)
   312	
   313		// 注册拨测与通知 API
   314		s.registerProbeAPI(mux)
   315	
   316		// 注册备份管理 API
   317		s.registerBackupsAPI(mux)
   318	
   319		// 注册远程连接 API
   320		s.registerRemoteAPI(mux)
   321	
   322		// 注册远控凭据保险箱 API（见 api_vault.go）
   323		s.registerVaultAPI(mux)
   324	
   325		// 注册 Guacamole 桌面隧道 API（guacd + guacamole-common-js 路线，
   326		// 见 docs/remote-desktop-guacamole-design.md）
   327		s.registerGuacamoleAPI(mux)
   328	
   329		// 公开路由
   330		mux.HandleFunc("/ws", s.handleWebSocket)
   331		mux.HandleFunc("/health", s.handleHealth)
   332		mux.HandleFunc("/api/auth/login", s.handleLoginWithAudit)
   333		mux.HandleFunc("/api/auth/refresh", s.authService().HandleRefresh)
   334		mux.HandleFunc("/api/auth/totp/verify", s.handleTOTPVerify) // TOTP 验证不需要 JWT（使用临时令牌）
   335		mux.HandleFunc("/api/auth/forgot-password", s.handleForgotPassword)
   336		mux.HandleFunc("/api/auth/reset-password", s.handleResetPassword)
   337		mux.HandleFunc("/api/auth/verify-reset-code", s.handleVerifyResetCode)
   338	
   339		// 需要认证的 API 路由
   340		mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
   341			// 登录相关接口不需要认证；TOTP 设置路由需认证（曾因置于
   342			// /api/auth/ 前缀守卫 return 之后而不可达，现并入同一 switch）
   343			if strings.HasPrefix(r.URL.Path, "/api/auth/") {
   344				switch r.URL.Path {
   345				case "/api/auth/login":
   346					s.handleLoginWithAudit(w, r)
   347				case "/api/auth/refresh":
   348					s.authService().HandleRefresh(w, r)
   349				case "/api/auth/totp/verify":
   350					s.handleTOTPVerify(w, r)
   351				case "/api/auth/totp/generate":
   352					s.authService().Middleware(s.handleTOTPGenerate)(w, r)
   353				case "/api/auth/totp/enable":
   354					s.authService().Middleware(s.handleTOTPEnable)(w, r)
   355				case "/api/auth/totp/disable":
   356					s.authService().Middleware(s.handleTOTPDisable)(w, r)
   357				}
   358				return
   359			}
   360			// 其他 API 需要认证
   361			s.authService().Middleware(s.serveAPI)(w, r)
   362		})
   363	
   364		// Web UI (SPA) - 必须放在最后作为 fallback
   365		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
   366			s.spaHandler().ServeHTTP(w, r)
   367		})
   368	}
   369	
   370	func (s *Server) authService() *auth.Service {
   371		s.mu.Lock()
   372		defer s.mu.Unlock()
   373	
   374		if s.auth != nil {
   375			return s.auth
   376		}
   377		s.auth = auth.NewService(s.db, auth.Options{Secret: "test-secret", Expiration: 24 * time.Hour})
   378		return s.auth
   379	}
   380	
   381	// Shutdown 关闭服务器。先 cancel 再等后台清扫循环与在途后台任务全部
   382	// 退出，最后才关 DB（先停清扫再关库）：循环只在自身 select 的 ctx.Done
   383	// 分支退出，不等待就会 mid-sweep 打到已关闭的库，测试侧还会与
   384	// TempDir RemoveAll 竞态（CI 曾见 "Agent expiry sweep failed: sql:
   385	// database is closed" 与 "directory not empty"）。
   386	func (s *Server) Shutdown() {
   387		if s.cancel != nil {
   388			s.cancel()
   389		}
   390		// 关闭已开始：此后新起的后台任务照跑但不入计数（不拖住收尾）
   391		s.bgMu.Lock()
   392		s.bgClosed = true
   393		s.bgMu.Unlock()
   394		s.bgWG.Wait()
   395		s.backupTrackWG.Wait()
   396		if s.inventorySync != nil {
   397			s.inventorySync.Stop()
   398		}
   399		if s.probeRunner != nil {
   400			s.probeRunner.Stop()
   401		}
   402		if s.proxyMgr != nil {
   403			s.proxyMgr.Stop()
   404		}
   405		if s.db != nil {
   406			s.db.Close()
   407		}
   408	}
   409	
   410	func (s *Server) startInventorySync() error {
   411		if s.cfg == nil || s.cfg.Inventory == nil || !s.cfg.Inventory.Watch {
   412			return nil
   413		}
   414		if s.cfg.Inventory.Path == "" {
   415			err := fmt.Errorf("inventory.watch enabled but inventory.path is empty")
   416			if s.cfg.Inventory.Strict {
   417				return err
   418			}
   419			log.Printf("Inventory sync disabled: %v", err)
   420			return nil
   421		}
   422	
   423		manager, err := inventorysync.NewManagerWithConfig(inventorysync.Config{
   424			InventoryPath: s.cfg.Inventory.Path,
   425			DB:            s.db,
   426			Strict:        s.cfg.Inventory.Strict,
   427		})
   428		if err != nil {
   429			if !s.cfg.Inventory.Strict {
   430				log.Printf("Inventory sync disabled: %v", err)
   431				return nil
   432			}
   433			return err
   434		}
   435		if err := manager.Start(); err != nil {
   436			if !s.cfg.Inventory.Strict {
   437				log.Printf("Inventory initial sync failed; watcher not started: %v", err)
   438				return nil
   439			}
   440			return err
   441		}
   442		s.inventorySync = manager
   443		return nil
   444	}
   445	
   446	// startProbeRunner 启动自动健康探测。间隔取值优先级：DB 中 UI 保存值 >
   447	// 配置文件 probe.interval > 内置默认 5m（probe.DefaultInterval）。
   448	func (s *Server) startProbeRunner() {
   449		interval := probe.DefaultInterval
   450		if s.cfg != nil && s.cfg.Probe != nil && s.cfg.Probe.Interval > 0 {
   451			interval = s.cfg.Probe.Interval
   452		}
   453		if v, err := s.db.GetSetting(probe.IntervalSettingKey); err == nil && v != "" {
   454			if secs, convErr := strconv.Atoi(v); convErr == nil &&
   455				secs >= probe.MinIntervalSeconds && secs <= probe.MaxIntervalSeconds {
   456				interval = time.Duration(secs) * time.Second
   457			}
   458		}
   459		s.probeRunner = probe.NewRunner(s.db, interval, s.notifier)
   460		s.probeRunner.LoadFailThreshold()
   461		s.probeRunner.Start()
   462		log.Printf("Probe runner started (interval: %s)", interval)
   463	}
   464	
   465	// handleHealth 健康检查
   466	func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
   467		w.Header().Set("Content-Type", "application/json")
   468		json.NewEncoder(w).Encode(map[string]interface{}{
   469			"status": "ok",
   470			"agents": len(s.registry.List()),
   471		})
   472	}
   473	
   474	// CallAgent 调用 Agent（RPC）
   475	func (s *Server) CallAgent(agentID, method string, params map[string]interface{}) (*protocol.Message, error) {
   476		agent, exists := s.registry.Get(agentID)
   477		if !exists {
   478			return nil, ErrAgentNotFound
   479		}
   480	
   481		// 创建响应通道
   482		respCh := make(chan *protocol.Message, 1)
   483		msgID := protocol.GenerateID()
   484		s.registry.RegisterPendingResponse(msgID, respCh)
   485		defer s.registry.UnregisterPendingResponse(msgID)
   486	
   487		// 发送请求
   488		req := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
   489			"method": method,
   490			"params": params,
   491		})
   492		req.ID = msgID
   493	
   494		// 发送请求（与 agent.Close 并发安全；通道满时重试至超时）
   495		if err := agent.SendWithTimeout(req, callAgentSendTimeout); err != nil {
   496			return nil, err
   497		}
   498	
   499		// 等待响应
   500		select {
   501		case resp := <-respCh:
   502			return resp, nil
   503		case <-time.After(callAgentTimeout):
   504			return nil, fmt.Errorf("response timeout")
   505		}
   506	}
   507	
   508	// locationOrUnknown 注册落库地域语义（拍板 2026-10-08，todo Phase 2.2
   509	// region/zone 校验项）：为空允许——不拒绝注册，空地域统一标记 "unknown"
   510	// （agent 侧 detectLocation 默认即 unknown，此处兜底第三方显式空值；语义见
   511	// docs/guide/concepts.md「Inventory 文件」与 decision-log）。
   512	func locationOrUnknown(v string) string {
   513		if v = strings.TrimSpace(v); v != "" {
   514			return v
   515		}
   516		return "unknown"
   517	}
   518	
   519	// toStorageAgent 将 Agent 转换为存储模型
   520	func toStorageAgent(agent *Agent) *storage.Agent {
   230		return s.db.CountUsersByRoles(names, skipUserID)
   231	}
   232	
   233	// RBACMiddleware 全局权限判定（AuditMiddleware 内层：403 也进审计链）
   234	func (s *Server) RBACMiddleware(next http.Handler) http.Handler {
   235		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
   236			perms, governed := requiredPerms(r.URL.Path, r.Method)
   237			if governed && len(perms) > 0 && !s.permitted(w, r, perms) {
   238				return
   239			}
   240			next.ServeHTTP(w, r)
   241		})
   242	}
   243	
   244	// permitted 校验 Bearer 角色覆盖 perms。无/坏 token 放行（内层 auth 出
   245	// 401）；角色在角色表缺失 fail-closed 403；权限不足 403。
   246	func (s *Server) permitted(w http.ResponseWriter, r *http.Request, perms []string) bool {
   247		header := r.Header.Get("Authorization")
   248		if !strings.HasPrefix(header, "Bearer ") {
   249			return true
   250		}
   251		claims, err := s.authService().ValidateToken(strings.TrimPrefix(header, "Bearer "))
   252		if err != nil {
   253			return true
   254		}
   255		role, err := s.db.GetRole(claims.Role)
   256		if err != nil {
   257			http.Error(w, `{"error":"Unknown role"}`, http.StatusForbidden)
   258			return false
   259		}
   260		for _, need := range perms {
   261			if !roleCovers(role.Permissions, need) {
   262				http.Error(w, `{"error":"Insufficient permissions"}`, http.StatusForbidden)
   263				return false
   264			}
   265		}
   266		return true
   267	}
     1	package server
     2	
     3	import (
     4		"encoding/json"
     5		"errors"
     6		"fmt"
     7		"log"
     8		"net/http"
     9		"strings"
    10		"time"
    11	
    12		"github.com/cuihairu/cockpit/internal/audit"
    13		"github.com/cuihairu/cockpit/internal/auth"
    14		"github.com/cuihairu/cockpit/internal/protocol"
    15		"github.com/cuihairu/cockpit/internal/storage"
    16	)
    17	
    18	// 执行 Job API（设计见 docs/guide/jobs-design.md；异步化见 workflow-design.md W1）。
    19	//
    20	//	POST /api/jobs             创建 Job，后台执行，立即 201 返回 pending 视图
    21	//	GET  /api/jobs             最近 Job 列表（倒序，limit 50；支持过滤）
    22	//	GET  /api/jobs/{id}        单条详情（含输出）
    23	//	POST /api/jobs/{id}/cancel 取消 pending/running Job（W3 + M2c C6）
    24	//
    25	// 执行流：校验 → 建 Job(pending) → 后台 goroutine CallAgent(job.exec) →
    26	// 终态 success/failed 回写 + 审计 job_run。目标 agent 离线一律 503（先查
    27	// registry，与 cron 同口径）。
    28	type jobCreatePayload struct {
    29		Type       string                 `json:"type"`
    30		Target     string                 `json:"target"`
    31		Targets    []string               `json:"targets,omitempty"`    // 多目标扇出（M2b F1）：仅 workflow steps 合法
    32		DependsOn  []string               `json:"depends_on,omitempty"` // 依赖门控（M2e G1）：仅 workflow steps 合法
    33		Parameters map[string]interface{} `json:"parameters"`
    34	}
    35	
    36	// jobTypes 允许的 Job 类型 → agent RPC method（最小闭环仅 agent.exec；
    37	// 新类型在此登记并补文档/前端类型映射）
    38	var jobTypes = map[string]string{
    39		"agent.exec": "job.exec",
    40	}
    41	
    42	// jobCancelMethod 取消信号 RPC 方法（M2c C1）：params {id} 按 dispatchJob
    43	// 下发时带入的 Job ID 定位 agent 侧在途执行
    44	const jobCancelMethod = "job.cancel"
    45	
    46	// jobSubmitRequest standalone Job 提交体：payload + 审批门开关（M2d）。
    47	// Approval 仅 server 侧消费，不进 agent 参数、不进 workflow steps（步骤级
    48	// 审批会在 advanceWorkflow 造不可复活的等待点，明确不支持）。
    49	type jobSubmitRequest struct {
    50		jobCreatePayload
    51		Approval string `json:"approval,omitempty"` // "required" = 先落审批门，待批准后派发
    52	}
    53	
    54	// approvalRequired 审批开关唯一合法取值。
    55	const approvalRequired = "required"
    56	
    57	// submitFail 可写响应的提交失败面（submitJob 供直接创建与批准后派发共用）。
    58	type submitFail struct {
    59		code int
    60		msg  string
    61	}
    62	
    63	// submitJob 校验→建行(pending)→后台派发（handleJobCreate 与审批门批准后
    64	// 共用的收口，M2d）：目标必须在线（离线 503 不建幽灵记录）。
    65	func (s *Server) submitJob(p *jobCreatePayload, actor string) (*jobView, *submitFail) {
    66		if err := validateJobPayload(p); err != nil {
    67			return nil, &submitFail{http.StatusBadRequest, err.Error()}
    68		}
    69		// targets 扇出仅 workflow steps 合法（M2b F1）：standalone Job 单目标语义
    70		if len(p.Targets) > 0 {
    71			return nil, &submitFail{http.StatusBadRequest, "targets is only valid in workflow steps"}
    72		}
    73		// depends_on 依赖门控仅 workflow steps 合法（M2e G1）：standalone 无编排语义
    74		if len(p.DependsOn) > 0 {
    75			return nil, &submitFail{http.StatusBadRequest, "depends_on is only valid in workflow steps"}
    76		}
    77		if _, ok := s.registry.Get(p.Target); !ok {
    78			return nil, &submitFail{http.StatusServiceUnavailable, "agent offline"}
    79		}
    80	
    81		paramsJSON, _ := json.Marshal(p.Parameters)
    82		job := &storage.Job{
    83			ID:         protocol.GenerateID(),
    84			Type:       p.Type,
    85			Target:     p.Target,
    86			Actor:      actor,
    87			Parameters: string(paramsJSON),
    88			Status:     storage.JobStatusPending,
    89			CreatedAt:  time.Now(),
    90		}
    91		if err := s.db.CreateJob(job); err != nil {
    92			return nil, &submitFail{http.StatusInternalServerError, "failed to create job"}
    93		}
    94	
    95		// 视图在派发前取定：goroutine 会原地改 job 状态（pending→running→终态），
    96		// 先快照保证 201 响应体恒为 pending（W1 契约），并与后台写互不竞争
    97		view := toJobView(job)
    98	
    99		// 后台派发：终态回写 + 审计在 goroutine 内完成（HTTP 请求即时返回）
   100		s.goBg(func() { s.dispatchJobAsync(job, *p, actor) })
   101		return &view, nil
   102	}
   103	
   104	// handleJobsAPI 分发 /api/jobs[/{id}[/cancel]]
   105	func (s *Server) handleJobsAPI(w http.ResponseWriter, r *http.Request, path string) {
   106		switch {
   107		case path == "/jobs" && r.Method == http.MethodGet:
   108			s.handleJobsList(w, r)
   109		case path == "/jobs" && r.Method == http.MethodPost:
   110			s.handleJobCreate(w, r)
   111		case strings.HasPrefix(path, "/jobs/"):
   112			rest := strings.TrimPrefix(path, "/jobs/")
   113			if rest == "" {
   114				s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
   115				return
   116			}
   117			// POST /api/jobs/{id}/cancel：pending 可撤（W3）/ running 可撤（M2c C6）
   118			if id, ok := strings.CutSuffix(rest, "/cancel"); ok && id != "" {
   119				if r.Method != http.MethodPost {
   120					s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
   121					return
   122				}
   123				s.handleJobCancel(w, r, id)
   124				return
   125			}
   126			if r.Method == http.MethodGet {
   127				s.handleJobGet(w, r, rest)
   128				return
   129			}
   130			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
   131		default:
   132			s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
   133		}
   134	}
   135	
   136	// validateJobPayload 入参加校验（type/target/parameters 结构与长度）
   137	func validateJobPayload(p *jobCreatePayload) error {
   138		if p.Type == "" {
   139			return fmt.Errorf("type is required")
   140		}
   141		if _, ok := jobTypes[p.Type]; !ok {
   142			return fmt.Errorf("unsupported job type %q (allowed: agent.exec)", p.Type)
   143		}
   144		p.Target = strings.TrimSpace(p.Target)
   145		if p.Target == "" && len(p.Targets) == 0 {
   146			return fmt.Errorf("target (agent id) is required")
   147		}
   148		if p.Type == "agent.exec" {
   149			cmd, _ := p.Parameters["command"].(string)
   150			if strings.TrimSpace(cmd) == "" {
   151				return fmt.Errorf("parameters.command is required")
   152			}
   153			if len(cmd) > 16*1024 {
   154				return fmt.Errorf("command too large (max 16KB)")
   155			}
   156			if t, ok := p.Parameters["timeout_s"].(float64); ok && (t <= 0 || t > 300) {
   157				return fmt.Errorf("timeout_s out of range (1-300)")
   158			}
   159		}
   160		return nil
   161	}
   162	
   163	// handleJobsList 最近 Job 列表（倒序；status/target/type/workflow_run_id 过滤）
   164	func (s *Server) handleJobsList(w http.ResponseWriter, r *http.Request) {
   165		q := r.URL.Query()
   166		jobs, err := s.db.ListJobsFiltered(
   167			q.Get("status"), q.Get("target"), q.Get("type"), q.Get("workflow_run_id"), 50)
   168		if err != nil {
   169			s.handleError(w, r, http.StatusInternalServerError, "failed to list jobs")
   170			return
   171		}
   172		out := make([]jobView, 0, len(jobs))
   173		for i := range jobs {
   174			out = append(out, toJobView(&jobs[i]))
   175		}
   176		s.writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": out})
   177	}
   178	
   179	// jobView API 视图：Parameters 解析为对象返回（库内存 JSON 串）
   180	type jobView struct {
   181		ID         string                 `json:"id"`
   182		Type       string                 `json:"type"`
   183		Target     string                 `json:"target"`
   184		Actor      string                 `json:"actor"`
   185		Status     string                 `json:"status"`
   186		Parameters map[string]interface{} `json:"parameters,omitempty"`
   187		Output     string                 `json:"output,omitempty"`
   188		ExitCode   int                    `json:"exitCode,omitempty"`
   189		Error      string                 `json:"error,omitempty"`
   190		CreatedAt  time.Time              `json:"createdAt"`
   191		StartedAt  *time.Time             `json:"startedAt,omitempty"`
   192		FinishedAt *time.Time             `json:"finishedAt,omitempty"`
   193	}
   194	
   195	func toJobView(j *storage.Job) jobView {
   196		v := jobView{
   197			ID:         j.ID,
   198			Type:       j.Type,
   199			Target:     j.Target,
   200			Actor:      j.Actor,
   201			Status:     j.Status,
   202			Output:     j.Output,
   203			ExitCode:   j.ExitCode,
   204			Error:      j.Error,
   205			CreatedAt:  j.CreatedAt,
   206			StartedAt:  j.StartedAt,
   207			FinishedAt: j.FinishedAt,
   208		}
   209		if j.Parameters != "" {
   210			var m map[string]interface{}
   211			if json.Unmarshal([]byte(j.Parameters), &m) == nil {
   212				v.Parameters = m
   213			}
   214		}
   215		return v
   216	}
   217	
   218	// handleJobGet 单条详情
   219	func (s *Server) handleJobGet(w http.ResponseWriter, r *http.Request, id string) {
   220		job, err := s.db.GetJob(id)
   221		if err != nil {
   222			// 唯一主键查询：查不到即 404（无其它失败面）
   223			s.handleError(w, r, http.StatusNotFound, "job not found")
   224			return
   225		}
   226		s.writeJSON(w, http.StatusOK, toJobView(job))
   227	}
   228	
   229	// handleJobCreate 创建 Job，后台执行（workflow-design W1：创建即返回 pending，
   230	// dispatch 转 goroutine；消费方经台账/详情轮询终态）。approval="required" 时
   231	// 不建 Job，改落审批记录（M2d），202 返回审批视图。
   232	func (s *Server) handleJobCreate(w http.ResponseWriter, r *http.Request) {
   233		var req jobSubmitRequest
   234		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
   235			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
   236			return
   237		}
   238		if req.Approval != "" && req.Approval != approvalRequired {
   239			s.handleError(w, r, http.StatusBadRequest, "unsupported approval value (allowed: required)")
   240			return
   241		}
   242	
   243		username := "unknown"
   244		if userInfo, ok := auth.GetUserFromContext(r); ok {
   245			username = userInfo.Username
   246		}
   247	
   248		// 审批门：先校验（批准后原样派发，坏 payload 不能进审批），落记录即返
   249		if req.Approval == approvalRequired {
   250			if err := validateJobPayload(&req.jobCreatePayload); err != nil {
   251				s.handleError(w, r, http.StatusBadRequest, err.Error())
   252				return
   253			}
   254			if len(req.jobCreatePayload.Targets) > 0 {
   255				s.handleError(w, r, http.StatusBadRequest, "targets is only valid in workflow steps")
   256				return
   257			}
   258			appr, err := s.createApproval(&req.jobCreatePayload, username)
   259			if err != nil {
   260				s.handleError(w, r, http.StatusInternalServerError, "failed to create approval")
   261				return
   262			}
   263			s.audit.LogResource(username, audit.ActionApprovalSubmit, audit.ResourceApproval, appr.ID,
   264				map[string]interface{}{"target": appr.Target, "summary": appr.Summary},
   265				s.getClientIP(r), r.UserAgent())
   266			s.writeJSON(w, http.StatusAccepted, toApprovalView(appr))
   267			return
   268		}
   269	
   270		view, fail := s.submitJob(&req.jobCreatePayload, username)
   271		if fail != nil {
   272			s.handleError(w, r, fail.code, fail.msg)
   273			return
   274		}
   275		s.writeJSON(w, http.StatusCreated, view)
   276	}
   277	
   278	// dispatchJobAsync 后台派发单条 Job：pending → running → 终态回写 + 审计
   279	func (s *Server) dispatchJobAsync(job *storage.Job, payload jobCreatePayload, actor string) {
   280		now := time.Now()
   281		job.Status = storage.JobStatusRunning
   282		job.StartedAt = &now
   283		if err := s.db.UpdateJob(job); err != nil {
   284			// 状态无法回写不阻碍执行（下一次 UpdateJob 会覆盖）
   285			log.Printf("jobs: update running state failed: %v", err)
   286		}
   287	
   288		success, jobErr := s.dispatchJob(job, payload)
   289	
   290		finished := time.Now()
   291		job.FinishedAt = &finished
   292		job.Output, job.Error, job.ExitCode = jobErr.Output, jobErr.Message, jobErr.ExitCode
   293	
   294		// C7 取消守卫：回写前重读，若取消写已落 cancelled（exec 被 job.cancel
   295		// 击杀）则保留 cancelled——取消与执行回写的竞态由库内状态收敛，终态
   296		// 不被覆盖（output/exit_code/error 补进台账供查）
   297		if fresh, err := s.db.GetJob(job.ID); err == nil && fresh.Status == storage.JobStatusCancelled {
   298			job.Status = storage.JobStatusCancelled
   299			if err := s.db.MarkJobCancelledOutput(job.ID, job.Output, job.Error, job.ExitCode); err != nil {
   300				log.Printf("jobs: write cancelled output failed: %v", err)
   301			}
   302		} else {
   303			if success {
   304				job.Status = storage.JobStatusSuccess
   305			} else {
   306				job.Status = storage.JobStatusFailed
   307			}
   308			if err := s.db.UpdateJob(job); err != nil {
   309				log.Printf("jobs: update final state failed: %v", err)
   310			}
   311		}
   312	
   313		// 审计：命令入参进 details（与 cron_apply 同口径），不涉及凭据
   314		details := map[string]interface{}{
   315			"type":   job.Type,
   316			"target": job.Target,
   317			"status": job.Status,
   318			"params": payload.Parameters,
   319		}
   320		s.audit.LogResource(actor, audit.ActionJobRun, audit.ResourceJob, job.ID,
     1	package server
     2	
     3	import (
     4		"errors"
     5		"sync"
     6		"time"
     7	
     8		"github.com/cuihairu/cockpit/internal/protocol"
     9		"github.com/google/uuid"
    10	)
    11	
    12	type RemoteSessionStatus string
    13	
    14	const (
    15		RemoteSessionStatusPending   RemoteSessionStatus = "pending"
    16		RemoteSessionStatusConnected RemoteSessionStatus = "connected"
    17		RemoteSessionStatusClosed    RemoteSessionStatus = "closed"
    18		RemoteSessionStatusFailed    RemoteSessionStatus = "failed"
    19	)
    20	
    21	type RemoteSession struct {
    22		ID        string                  `json:"id"`
    23		UserID    string                  `json:"userId"`
    24		Username  string                  `json:"username"`
    25		AgentID   string                  `json:"agentId"`
    26		Protocol  protocol.RemoteProtocol `json:"protocol"`
    27		Host      string                  `json:"host"`
    28		Port      int                     `json:"port"`
    29		Status    RemoteSessionStatus     `json:"status"`
    30		Error     string                  `json:"error,omitempty"`
    31		CreatedAt time.Time               `json:"createdAt"`
    32		UpdatedAt time.Time               `json:"updatedAt"`
    33		ClosedAt  *time.Time              `json:"closedAt,omitempty"`
    34	}
    35	
    36	type RemoteSessionRequest struct {
    37		AgentID  string                  `json:"agentId"`
    38		Protocol protocol.RemoteProtocol `json:"protocol"`
    39		Host     string                  `json:"host"`
    40		Port     int                     `json:"port"`
    41	}
    42	
    43	type RemoteSessionManager struct {
    44		mu       sync.RWMutex
    45		sessions map[string]*RemoteSession
    46	}
    47	
    48	func NewRemoteSessionManager() *RemoteSessionManager {
    49		return &RemoteSessionManager{
    50			sessions: make(map[string]*RemoteSession),
    51		}
    52	}
    53	
    54	// Create 登记会话并自分配 UUID（REST 登记入口用）。
    55	func (m *RemoteSessionManager) Create(userID, username string, req RemoteSessionRequest) *RemoteSession {
    56		return m.Register("", userID, username, req)
    57	}
    58	
    59	// Register 登记一个活会话。id 为空则生成 UUID（Create 语义）；terminal /
    60	// guacamole 链路传入自身会话 ID，使台账与数据面索引同源（审计 Session 字段
    61	// 与控制面 DELETE 命中同一 ID，D-2026-10-12-4）。
    62	func (m *RemoteSessionManager) Register(id, userID, username string, req RemoteSessionRequest) *RemoteSession {
    63		if id == "" {
    64			id = uuid.New().String()
    65		}
    66		now := time.Now()
    67		session := &RemoteSession{
    68			ID:        id,
    69			UserID:    userID,
    70			Username:  username,
    71			AgentID:   req.AgentID,
    72			Protocol:  req.Protocol,
    73			Host:      req.Host,
    74			Port:      req.Port,
    75			Status:    RemoteSessionStatusPending,
    76			CreatedAt: now,
    77			UpdatedAt: now,
    78		}
    79	
    80		m.mu.Lock()
    81		m.sessions[id] = session
    82		m.mu.Unlock()
    83	
    84		return session
    85	}
    86	
    87	// List / Get 返回台账快照（浅拷贝）：WS 数据面 goroutine 会并发 UpdateStatus
    88	// 写同一会话字段，读侧若直透存储指针，序列化/断言读到写一半的状态（-race 实锤）。
    89	// ClosedAt 是指针替换非穿透写，浅拷贝即一致快照。
    90	
    91	func (m *RemoteSessionManager) List() []*RemoteSession {
    92		m.mu.RLock()
    93		defer m.mu.RUnlock()
    94	
    95		result := make([]*RemoteSession, 0, len(m.sessions))
    96		for _, session := range m.sessions {
    97			cp := *session
    98			result = append(result, &cp)
    99		}
   100		return result
   101	}
   102	
   103	func (m *RemoteSessionManager) Get(id string) (*RemoteSession, bool) {
   104		m.mu.RLock()
   105		defer m.mu.RUnlock()
   106		session, ok := m.sessions[id]
   107		if !ok {
   108			return nil, false
   109		}
   110		cp := *session
   111		return &cp, true
   112	}
   113	
   114	func (m *RemoteSessionManager) UpdateStatus(id string, status RemoteSessionStatus, errMsg string) error {
   115		m.mu.Lock()
   116		defer m.mu.Unlock()
   117	
   118		session, ok := m.sessions[id]
   119		if !ok {
   120			return errors.New("remote session not found")
   121		}
   122	
   123		now := time.Now()
   124		session.Status = status
   125		session.UpdatedAt = now
   126		session.Error = errMsg
   127		if status == RemoteSessionStatusClosed || status == RemoteSessionStatusFailed {
   128			session.ClosedAt = &now
   129		}
   130		return nil
   131	}
   132	
   133	func (m *RemoteSessionManager) Delete(id string) bool {
   134		m.mu.Lock()
   135		defer m.mu.Unlock()
   136	
   137		if _, ok := m.sessions[id]; !ok {
   138			return false
   139		}
   140		delete(m.sessions, id)
   141		return true
   142	}
[0m$ [0mnl -ba internal/server/ticket.go | sed -n '1,240p'; nl -ba internal/server/api_remote.go | sed -n '1,360p'; nl -ba internal/server/websocket.go | sed -n '1,280p'; nl -ba internal/server/guac_relay.go | sed -n '1,300p'
1	package server
     2	
     3	import (
     4		"crypto/rand"
     5		"encoding/hex"
     6		"log"
     7		"sync"
     8		"time"
     9	)
    10	
    11	// randRead 注入点：crypto/rand.Read 实际不会失败，票据生成的错误分支
    12	// 仅供测试覆盖（api_remote 的 GenerateTicket 失败路径随之可达）。
    13	var randRead = rand.Read
    14	
    15	// Ticket 一次性短期 WebSocket 连接凭证
    16	type Ticket struct {
    17		ID        string
    18		UserID    string
    19		Username  string
    20		Params    map[string]string // 连接参数 (agent_id, host, port, etc.)
    21		ExpiresAt time.Time
    22		Consumed  bool
    23		mu        sync.RWMutex
    24	}
    25	
    26	// TicketManager 票据管理器
    27	type TicketManager struct {
    28		tickets map[string]*Ticket
    29		mu      sync.RWMutex
    30	}
    31	
    32	// NewTicketManager 创建票据管理器
    33	func NewTicketManager() *TicketManager {
    34		tm := &TicketManager{
    35			tickets: make(map[string]*Ticket),
    36		}
    37		go tm.cleanupLoop()
    38		return tm
    39	}
    40	
    41	// GenerateTicket 生成新票据（有效期5分钟）
    42	func (tm *TicketManager) GenerateTicket(userID, username string, params map[string]string) (*Ticket, error) {
    43		tm.mu.Lock()
    44		defer tm.mu.Unlock()
    45	
    46		// 生成随机票据ID（16字节）
    47		b := make([]byte, 16)
    48		if _, err := randRead(b); err != nil {
    49			return nil, err
    50		}
    51		ticketID := hex.EncodeToString(b)
    52	
    53		ticket := &Ticket{
    54			ID:        ticketID,
    55			UserID:    userID,
    56			Username:  username,
    57			Params:    params,
    58			ExpiresAt: time.Now().Add(5 * time.Minute),
    59			Consumed:  false,
    60		}
    61	
    62		tm.tickets[ticketID] = ticket
    63		log.Printf("Ticket generated: %s for user %s", ticketID, username)
    64	
    65		return ticket, nil
    66	}
    67	
    68	// ValidateTicket 验证并消费票据（单次使用）
    69	func (tm *TicketManager) ValidateTicket(ticketID string) (*Ticket, bool) {
    70		tm.mu.Lock()
    71		defer tm.mu.Unlock()
    72	
    73		ticket, exists := tm.tickets[ticketID]
    74		if !exists {
    75			return nil, false
    76		}
    77	
    78		ticket.mu.Lock()
    79		defer ticket.mu.Unlock()
    80	
    81		// 检查是否已消费
    82		if ticket.Consumed {
    83			return nil, false
    84		}
    85	
    86		// 检查是否过期
    87		if time.Now().After(ticket.ExpiresAt) {
    88			delete(tm.tickets, ticketID)
    89			return nil, false
    90		}
    91	
    92		// 标记为已消费
    93		ticket.Consumed = true
    94	
    95		return ticket, true
    96	}
    97	
    98	// cleanupLoop 定期清理过期票据
    99	func (tm *TicketManager) cleanupLoop() {
   100		ticker := time.NewTicker(1 * time.Minute)
   101		defer ticker.Stop()
   102	
   103		for range ticker.C {
   104			tm.mu.Lock()
   105			now := time.Now()
   106			for id, ticket := range tm.tickets {
   107				if now.After(ticket.ExpiresAt) {
   108					delete(tm.tickets, id)
   109				}
   110			}
   111			tm.mu.Unlock()
   112		}
   113	}
     1	package server
     2	
     3	import (
     4		"encoding/json"
     5		"log"
     6		"net/http"
     7		"strconv"
     8		"strings"
     9		"sync"
    10		"time"
    11	
    12		"github.com/cuihairu/cockpit/internal/audit"
    13		"github.com/cuihairu/cockpit/internal/auth"
    14		"github.com/cuihairu/cockpit/internal/protocol"
    15		"github.com/google/uuid"
    16		"github.com/gorilla/websocket"
    17	)
    18	
    19	// TerminalSession 终端会话
    20	type TerminalSession struct {
    21		ID         string
    22		UserID     string
    23		Username   string
    24		AgentID    string
    25		Protocol   protocol.RemoteProtocol
    26		Host       string
    27		Port       int
    28		ClientWS   *websocket.Conn
    29		ConnID     string
    30		CreatedAt  time.Time
    31		LastActive time.Time
    32		recorder   *castRecorder // 输出流录制（recording-design.md；nil = 未开）
    33		mu         sync.Mutex
    34		done       chan struct{}
    35	}
    36	
    37	var (
    38		terminalSessions   = make(map[string]*TerminalSession) // sessionID -> session
    39		terminalByConn     = make(map[string]*TerminalSession) // connID -> session
    40		terminalSessionsMu sync.Mutex
    41	)
    42	
    43	// handleTerminalWebSocket 处理终端 WebSocket 连接
    44	func (s *Server) handleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
    45		// 从 Sec-WebSocket-Protocol 头获取票据
    46		protocols := r.Header.Values("Sec-WebSocket-Protocol")
    47		if len(protocols) == 0 {
    48			http.Error(w, "Missing ticket in Sec-WebSocket-Protocol header", http.StatusBadRequest)
    49			return
    50		}
    51		ticketID := protocols[0]
    52	
    53		// 验证票据
    54		ticket, valid := s.ticketMgr.ValidateTicket(ticketID)
    55		if !valid {
    56			http.Error(w, "Invalid or expired ticket", http.StatusUnauthorized)
    57			return
    58		}
    59	
    60		// 从票据参数获取连接信息
    61		agentID := ticket.Params["agent_id"]
    62		host := ticket.Params["host"]
    63		portStr := ticket.Params["port"]
    64		protocolStr := ticket.Params["protocol"]
    65	
    66		if agentID == "" || host == "" || portStr == "" || protocolStr == "" {
    67			http.Error(w, "Invalid ticket parameters", http.StatusBadRequest)
    68			return
    69		}
    70	
    71		var port int
    72		if err := json.Unmarshal([]byte(portStr), &port); err != nil {
    73			http.Error(w, "Invalid port", http.StatusBadRequest)
    74			return
    75		}
    76	
    77		remoteProtocol := protocol.RemoteProtocol(protocolStr)
    78		if remoteProtocol != protocol.RemoteProtocolSSH &&
    79			remoteProtocol != protocol.RemoteProtocolTelnet &&
    80			remoteProtocol != protocol.RemoteProtocolVNC {
    81			http.Error(w, "Unsupported protocol", http.StatusBadRequest)
    82			return
    83		}
    84	
    85		agent, exists := s.registry.Get(agentID)
    86		if !exists {
    87			http.Error(w, "Agent not found or offline", http.StatusNotFound)
    88			return
    89		}
    90	
    91		// 升级 WebSocket，接受票据协议
    92		upgrader := websocket.Upgrader{
    93			CheckOrigin:  isOriginAllowed,
    94			Subprotocols: []string{ticketID},
    95		}
    96		conn, err := upgrader.Upgrade(w, r, nil)
    97		if err != nil {
    98			log.Printf("WebSocket upgrade failed: %v", err)
    99			return
   100		}
   101	
   102		connID := uuid.New().String()[:8]
   103		sessionID := uuid.New().String()
   104	
   105		session := &TerminalSession{
   106			ID:         sessionID,
   107			UserID:     ticket.UserID,
   108			Username:   ticket.Username,
   109			AgentID:    agentID,
   110			Protocol:   remoteProtocol,
   111			Host:       host,
   112			Port:       port,
   113			ClientWS:   conn,
   114			ConnID:     connID,
   115			CreatedAt:  time.Now(),
   116			LastActive: time.Now(),
   117			done:       make(chan struct{}),
   118		}
   119	
   120		terminalSessionsMu.Lock()
   121		terminalSessions[sessionID] = session
   122		terminalByConn[connID] = session
   123		terminalSessionsMu.Unlock()
   124	
   125		// 会话台账登记（D-2026-10-12-4）：terminal 会话 ID 与控制面同源，
   126		// 状态随建立/成功/失败/关闭推进（pending→connected→closed/failed）。
   127		s.remoteSessions.Register(sessionID, ticket.UserID, ticket.Username, RemoteSessionRequest{
   128			AgentID:  agentID,
   129			Protocol: remoteProtocol,
   130			Host:     host,
   131			Port:     port,
   132		})
   133	
   134		// 输出流录制（开关默认开；失败仅记日志，不影响远控主链路）
   135		if s.recordingEnabled() {
   136			session.recorder = s.startRecording(session)
   137		}
   138	
   139		log.Printf("Terminal session created: %s for user %s", sessionID, ticket.Username)
   140	
   141		target := host + ":" + portStr
   142		proxyNew := map[string]interface{}{
   143			"proxyId":   "terminal-" + connID,
   144			"proxyType": "tcp",
   145			"target":    target,
   146			"terminal":  true,
   147			"connId":    connID,
   148			"protocol":  string(remoteProtocol),
   149		}
   150		// SSH 凭据随 proxy_new 下发到 agent（agent 侧终结 SSH 协议）。
   151		// 口令只经加密 WS 通道传输，不落盘、不进日志/审计。
   152		if remoteProtocol == protocol.RemoteProtocolSSH {
   153			if v := ticket.Params["username"]; v != "" {
   154				proxyNew["username"] = v
   155			}
   156			if v := ticket.Params["password"]; v != "" {
   157				proxyNew["password"] = v
   158			}
   159			if v := ticket.Params["private_key"]; v != "" {
   160				proxyNew["privateKey"] = v
   161			}
   162		}
   163		msg := protocol.NewMessage(protocol.MessageTypeProxyNew, proxyNew)
   164	
   165		if err := agent.SendMessage(msg); err != nil {
   166			log.Printf("Failed to send proxy start message: %v", err)
   167			session.recorder.Close() // 已开的录制文件与元数据行照常收尾（其余出口都走这条）
   168			conn.WriteJSON(map[string]interface{}{
   169				"type":    "error",
   170				"message": "Failed to establish connection to agent",
   171			})
   172			conn.Close()
   173			terminalSessionsMu.Lock()
   174			delete(terminalSessions, sessionID)
   175			delete(terminalByConn, connID)
   176			terminalSessionsMu.Unlock()
   177			s.remoteSessions.UpdateStatus(sessionID, RemoteSessionStatusFailed, "agent unreachable")
   178			return
   179		}
   180		// proxy_new 已下发：会话建立（浏览器 ↔ agent 数据面就绪）
   181		_ = s.remoteSessions.UpdateStatus(sessionID, RemoteSessionStatusConnected, "")
   182	
   183		conn.WriteJSON(map[string]interface{}{
   184			"type":    "connect",
   185			"message": "Connecting to " + target,
   186		})
   187	
   188		go s.terminalSendLoop(session)
   189		s.terminalKeepaliveLoop(session)
   190	}
   191	
   192	// terminalSendLoop 浏览器 → Agent 数据转发
   193	func (s *Server) terminalSendLoop(session *TerminalSession) {
   194		defer func() {
   195			if r := recover(); r != nil {
   196				log.Printf("terminalSendLoop panic: %v", r)
   197			}
   198			close(session.done)
   199		}()
   200	
   201		conn := session.ClientWS
   202		connID := session.ConnID
   203	
   204		for {
   205			_, data, err := conn.ReadMessage()
   206			if err != nil {
   207				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
   208					log.Printf("WebSocket read error: %v", err)
   209				}
   210				s.sendCloseToAgent(session)
   211				return
   212			}
   213	
   214			// 更新最后活跃时间
   215			session.mu.Lock()
   216			session.LastActive = time.Now()
   217			session.mu.Unlock()
   218	
   219			var msg map[string]interface{}
   220			if err := json.Unmarshal(data, &msg); err == nil {
   221				msgType, _ := msg["type"].(string)
   222	
   223				switch msgType {
   224				case "input":
   225					inputData, _ := msg["data"].(string)
   226					agent, exists := s.registry.Get(session.AgentID)
   227					if exists {
   228						payload := map[string]interface{}{
   229							"proxyId":  "terminal-" + connID,
   230							"connId":   connID,
   231							"data":     []byte(inputData),
   232							"terminal": true,
   233						}
   234						agent.SendMessage(protocol.NewMessage(protocol.MessageTypeProxyData, payload))
   235					}
   236	
   237				case "resize":
   238					rows, _ := msg["rows"].(float64)
   239					cols, _ := msg["cols"].(float64)
   240					agent, exists := s.registry.Get(session.AgentID)
   241					if exists {
   242						payload := map[string]interface{}{
   243							"proxyId":  "terminal-" + connID,
   244							"connId":   connID,
   245							"resize":   true,
   246							"rows":     int(rows),
   247							"cols":     int(cols),
   248							"terminal": true,
   249						}
   250						agent.SendMessage(protocol.NewMessage(protocol.MessageTypeProxyData, payload))
   251					}
   252				}
   253			} else {
   254				agent, exists := s.registry.Get(session.AgentID)
   255				if exists {
   256					payload := map[string]interface{}{
   257						"proxyId":  "terminal-" + connID,
   258						"connId":   connID,
   259						"data":     string(data),
   260						"terminal": true,
   261					}
   262					agent.SendMessage(protocol.NewMessage(protocol.MessageTypeProxyData, payload))
   263				}
   264			}
   265		}
   266	}
   267	
   268	// terminalKeepalive 间隔。包级变量仅为测试可注入，默认值即生产取值。
   269	var terminalKeepaliveInterval = 30 * time.Second
   270	
   271	// terminalKeepaliveLoop 超时管理
   272	func (s *Server) terminalKeepaliveLoop(session *TerminalSession) {
   273		ticker := time.NewTicker(terminalKeepaliveInterval)
   274		defer ticker.Stop()
   275	
   276		for {
   277			select {
   278			case <-session.done:
   279				s.closeTerminalSession(session)
   280				return
   281			case <-ticker.C:
   282				if err := session.ClientWS.WriteJSON(map[string]interface{}{
   283					"type": "ping",
   284				}); err != nil {
   285					s.closeTerminalSession(session)
   286					return
   287				}
   288	
   289				// 获取实际的活动时间
   290				session.mu.Lock()
   291				lastActive := session.LastActive
   292				session.mu.Unlock()
   293	
   294				if time.Since(lastActive) > 30*time.Minute {
   295					log.Printf("Terminal session timeout: %s", session.ID)
   296					s.closeTerminalSession(session)
   297					return
   298				}
   299			}
   300		}
   301	}
   302	
   303	// sendCloseToAgent 发送关闭消息给 Agent
   304	func (s *Server) sendCloseToAgent(session *TerminalSession) {
   305		agent, exists := s.registry.Get(session.AgentID)
   306		if exists {
   307			payload := map[string]interface{}{
   308				"proxyId":  "terminal-" + session.ConnID,
   309				"connId":   session.ConnID,
   310				"reason":   "client disconnected",
   311				"terminal": true,
   312			}
   313			agent.SendMessage(protocol.NewMessage(protocol.MessageTypeProxyClose, payload))
   314		}
   315	}
   316	
   317	// closeTerminalSession 关闭终端会话
   318	func (s *Server) closeTerminalSession(session *TerminalSession) {
   319		session.recorder.Close() // 幂等，与 HandleTerminalClose 双出口都可调
   320		session.ClientWS.Close()
   321		terminalSessionsMu.Lock()
   322		delete(terminalSessions, session.ID)
   323		delete(terminalByConn, session.ConnID)
   324		terminalSessionsMu.Unlock()
   325		log.Printf("Terminal session closed: %s", session.ID)
   326		// 台账收口：关闭即终态（与审计结束同刻回填）
   327		_ = s.remoteSessions.UpdateStatus(session.ID, RemoteSessionStatusClosed, "")
   328	
   329		s.auditRemoteEnd(session.UserID, session.Username, "", "",
   330			&audit.RemoteSessionDetails{
   331				Protocol: string(session.Protocol),
   332				AgentID:  session.AgentID,
   333				Host:     session.Host,
   334				Port:     session.Port,
   335				Session:  session.ID,
   336				Duration: time.Since(session.CreatedAt).String(),
   337			})
   338	}
   339	
   340	// HandleTerminalData 处理从 Agent 转发到浏览器终端的数据
   341	func (s *Server) HandleTerminalData(connID string, data []byte) error {
   342		terminalSessionsMu.Lock()
   343		session, exists := terminalByConn[connID]
   344		terminalSessionsMu.Unlock()
   345	
   346		if !exists {
   347			return nil
   348		}
   349	
   350		// 录制输出流（agent → 浏览器方向的唯一出口）
   351		session.recorder.WriteOutput(data)
   352	
   353		return session.ClientWS.WriteJSON(map[string]interface{}{
   354			"type": "data",
   355			"data": string(data),
   356		})
   357	}
   358	
   359	// HandleTerminalClose 处理终端连接关闭
   360	func (s *Server) HandleTerminalClose(connID string, reason string) {
     1	package server
     2	
     3	import (
     4		"errors"
     5		"fmt"
     6		"log"
     7		"net/http"
     8		"os"
     9		"strings"
    10		"time"
    11	
    12		"github.com/cuihairu/cockpit/internal/protocol"
    13		"github.com/cuihairu/cockpit/internal/storage"
    14		"github.com/gorilla/websocket"
    15	)
    16	
    17	type registrationRejection struct {
    18		code    string
    19		message string
    20	}
    21	
    22	func getEnv(key, defaultValue string) string {
    23		if value := os.Getenv(key); value != "" {
    24			return value
    25		}
    26		return defaultValue
    27	}
    28	
    29	// isOriginAllowed 检查 WebSocket 升级请求的 Origin 是否在白名单内
    30	func isOriginAllowed(r *http.Request) bool {
    31		allowed := os.Getenv("ALLOWED_ORIGINS")
    32		if allowed == "" {
    33			// 未配置白名单时允许所有来源（开发模式）
    34			log.Println("WARNING: ALLOWED_ORIGINS not set, accepting all WebSocket origins. Configure this in production!")
    35			return true
    36		}
    37		origin := r.Header.Get("Origin")
    38		// 空 Origin = 非浏览器客户端（cockpit-agent 等不出于浏览器，不发 Origin 头）。
    39		// 白名单是防浏览器侧跨站 WS 的；把这类客户端一并拒绝会让生产
    40		// （ALLOWED_ORIGINS 必配）下 agent 握手 403/bad handshake。浏览器永远带 Origin，
    41		// 仍走下方白名单。
    42		if origin == "" {
    43			return true
    44		}
    45		for _, a := range strings.Split(allowed, ",") {
    46			a = strings.TrimSpace(a)
    47			if a == "*" || a == origin {
    48				return true
    49			}
    50		}
    51		return false
    52	}
    53	
    54	// handleWebSocket 处理 WebSocket 连接
    55	func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
    56		conn, err := s.upgrader.Upgrade(w, r, nil)
    57		if err != nil {
    58			log.Printf("WebSocket upgrade failed: %v", err)
    59			return
    60		}
    61	
    62		// 等待注册消息
    63		msg, err := s.codec.ReadMessage(conn)
    64		if err != nil {
    65			log.Printf("Read register message failed: %v", err)
    66			conn.Close()
    67			return
    68		}
    69	
    70		if msg.Type != protocol.MessageTypeRegister {
    71			log.Printf("First message must be register, got: %s", msg.Type)
    72			conn.Close()
    73			return
    74		}
    75	
    76		// 解析注册信息
    77		reg, err := decodeRegisterForWebSocket(msg)
    78		if err != nil {
    79			log.Printf("Parse register payload failed: %v", err)
    80			conn.Close()
    81			return
    82		}
    83	
    84		agent, rejection, err := s.registerAgentConnection(conn, &reg)
    85		if err != nil {
    86			log.Printf("Agent %s registration failed: %v", reg.AgentID, err)
    87			s.sendRegisterError(conn, "registration_failed", "Registration failed")
    88			conn.Close()
    89			return
    90		}
    91		if rejection != nil {
    92			log.Printf("Agent %s registration rejected: %s", reg.AgentID, rejection.message)
    93			s.sendRegisterError(conn, rejection.code, rejection.message)
    94			conn.Close()
    95			return
    96		}
    97	
    98		log.Printf("Agent registered: %s at %s/%s", agent.ID, reg.Location.Region, reg.Location.Zone)
    99	
   100		// 发送注册响应
   101		resp := protocol.NewMessage(protocol.MessageTypeRegister, s.registerRespPayload(agent))
   102		s.codec.WriteMessage(conn, resp)
   103	
   104		// 启动读写循环
   105		go s.readLoop(agent)
   106		go s.writeLoop(agent)
   107	
   108		// 服务健康探针配置补推（service-health D3）：注册上线即对齐配置——
   109		// 保存时 agent 离线的配置在这里落到 agent 侧
   110		s.goBg(func() { s.pushServiceHealthConfig(agent.ID) })
   111	}
   112	
   113	func decodeRegisterForWebSocket(msg *protocol.Message) (protocol.RegisterPayload, error) {
   114		if msg.Payload == nil {
   115			return protocol.RegisterPayload{}, nil
   116		}
   117		return protocol.DecodeRegister(msg)
   118	}
   119	
   120	// wsRegistryLookup registry 活跃连接查询。包级变量仅为测试可注入：
   121	// 后注册者跳过 Get 检查即可确定性复现「检查时不存在、注册时已被
   122	// 抢先」的 TOCTOU 后半程——真实并发下该窗口极窄，高负载（CI race
   123	// 检测）后到者几乎总被 Get 先拦成 duplicate_connection，赌调度
   124	// 会 flaky。
   125	var wsRegistryLookup = func(r *Registry, agentID string) (*Agent, bool) {
   126		return r.Get(agentID)
   127	}
   128	
   129	func (s *Server) registerAgentConnection(conn *websocket.Conn, reg *protocol.RegisterPayload) (*Agent, *registrationRejection, error) {
   130		if reg.AgentID == "" {
   131			reg.AgentID = protocol.GenerateIDWithPrefix("agent")
   132		}
   133	
   134		existingAgent, rejection, err := s.authenticateAgentRegistration(reg)
   135		if err != nil {
   136			return nil, nil, err
   137		}
   138		if rejection != nil {
   139			return nil, rejection, nil
   140		}
   141	
   142		// Registry 中的 Agent 均为活跃连接，已有记录时拒绝重复连接。
   143		if _, exists := wsRegistryLookup(s.registry, reg.AgentID); exists {
   144			return nil, &registrationRejection{
   145				code:    "duplicate_connection",
   146				message: "Agent already connected",
   147			}, nil
   148		}
   149	
   150		agent := NewAgent(reg.AgentID, conn)
   151		agent.Update(reg)
   152	
   153		if err := s.registry.Register(agent); err != nil {
   154			return nil, &registrationRejection{
   155				code:    "registration_failed",
   156				message: err.Error(),
   157			}, nil
   158		}
   159	
   160		if err := s.persistRegisteredAgent(agent, existingAgent); err != nil {
   161			log.Printf("Failed to persist agent to database: %v", err)
   162		}
   163	
   164		return agent, nil, nil
   165	}
   166	
   167	func (s *Server) authenticateAgentRegistration(reg *protocol.RegisterPayload) (*storage.Agent, *registrationRejection, error) {
   168		existingAgent, err := s.db.GetAgent(reg.AgentID)
   169		if err != nil {
   170			if errors.Is(err, storage.ErrNotFound) {
   171				return nil, nil, nil
   172			}
   173			return nil, nil, fmt.Errorf("load existing agent %s: %w", reg.AgentID, err)
   174		}
   175	
   176		if existingAgent.SecretHash == "" {
   177			// 兼容历史数据：已有 Agent 但未配置密钥时仍允许注册。
   178			log.Printf("Agent %s exists but has no secret configured", reg.AgentID)
   179			return existingAgent, nil, nil
   180		}
   181	
   182		if reg.Secret == "" {
   183			return existingAgent, &registrationRejection{
   184				code:    "authentication_required",
   185				message: "Agent secret is required",
   186			}, nil
   187		}
   188		if !storage.VerifyAgentSecret(existingAgent.SecretHash, reg.Secret) {
   189			return existingAgent, &registrationRejection{
   190				code:    "authentication_failed",
   191				message: "Invalid agent secret",
   192			}, nil
   193		}
   194	
   195		return existingAgent, nil, nil
   196	}
   197	
   198	func (s *Server) persistRegisteredAgent(agent *Agent, existingAgent *storage.Agent) error {
   199		storageAgent := toStorageAgent(agent)
   200		if existingAgent != nil && existingAgent.SecretHash != "" {
   201			storageAgent.SecretHash = existingAgent.SecretHash
   202		}
   203		return s.db.UpsertAgent(storageAgent)
   204	}
   205	
   206	// sendRegisterError 发送注册错误响应
   207	func (s *Server) sendRegisterError(conn *websocket.Conn, code, message string) {
   208		resp := protocol.NewMessage(protocol.MessageTypeError, map[string]interface{}{
   209			"code":    code,
   210			"message": message,
   211		})
   212		s.codec.WriteMessage(conn, resp)
   213	}
   214	
   215	// readLoop 读取循环
   216	func (s *Server) readLoop(agent *Agent) {
   217		defer func() {
   218			s.registry.Unregister(agent.ID)
   219			// 更新数据库状态为离线
   220			if err := s.db.UpdateAgentStatus(agent.ID, "offline", time.Now()); err != nil {
   221				log.Printf("Agent %s failed to update offline status: %v", agent.ID, err)
   222			}
   223		}()
   224	
   225		for {
   226			msg, err := s.codec.ReadMessage(agent.Conn)
   227			if err != nil {
   228				log.Printf("Agent %s read error: %v", agent.ID, err)
   229				return
   230			}
   231	
   232			s.handleMessage(agent, msg)
   233		}
   234	}
   235	
   236	// writeLoop 写入循环
   237	func (s *Server) writeLoop(agent *Agent) {
   238		defer agent.Conn.Close()
   239	
   240		for msg := range agent.Send {
   241			if err := s.codec.WriteMessage(agent.Conn, msg); err != nil {
   242				log.Printf("Agent %s write error: %v", agent.ID, err)
   243				return
   244			}
   245		}
   246	}
     1	package server
     2	
     3	import (
     4		"fmt"
     5		"io"
     6		"log"
     7		"net"
     8		"sync"
     9		"sync/atomic"
    10	
    11		"github.com/cuihairu/cockpit/internal/protocol"
    12	)
    13	
    14	// Guacamole 目标转发（设计修正 2026-10-02）：guacd 的目标连接不再直拨
    15	// hostname——server 常与目标不同网段（guacd 在云上、目标在 agent 所在内网）。
    16	// 每条会话起一个回环中继 listener：guacd 拨中继，字节流经既有 agent WS 通道
    17	// （proxy_new/proxy_data/proxy_close，与端口转发 proxy/mgr、内置终端
    18	// api_remote 同一条代理管线，agent 侧裸 TCP 拨号分支见 proxy/handler.go
    19	// HandleProxyNew），agent 从自己网络位拨 host:port。hostname 语义随之变为
    20	// 「agent 侧可达的地址」。
    21	//
    22	// 网络拓扑约束：中继绑 server 本机回环，guacd 必须与 server 同机（或共享
    23	// 回环可见性）——与录制目录同路径双挂（GUACD_RECORDING_PATH）是同一前提，
    24	// 见 deployments/guacd/README.md 的部署拓扑。
    25	
    26	// guacRelayPrefix Guacamole 转发会话的 proxyId 前缀。server 侧 proxy_data/
    27	// proxy_close/proxy_error 按前缀路由（同 logs:/terminal/vnc 惯例），不进
    28	// proxyMgr（端口转发的 DB 配置面）。
    29	const guacRelayPrefix = "guac:"
    30	
    31	// guacRelay 一条 Guacamole 会话的目标转发器
    32	type guacRelay struct {
    33		agentID string
    34		proxyID string
    35		target  string // agent 侧视角的 host:port
    36		send    func(agentID string, msg *protocol.Message) error
    37	
    38		ln net.Listener
    39	
    40		mu    sync.Mutex
    41		conns map[string]net.Conn // connID → guacd 侧连接
    42		seq   atomic.Uint64
    43	
    44		closeOnce sync.Once
    45	}
    46	
    47	// guacRelayReg 会话级转发器注册表（proxyID → relay）。包级同 terminalSessions
    48	// 惯例；server.handleProxyData 按前缀路由到这里，不依赖 Server 字段（测试
    49	// 可用零值 Server 走真实分发路径）。
    50	var (
    51		guacRelayRegMu sync.Mutex
    52		guacRelayReg   = map[string]*guacRelay{}
    53	)
    54	
    55	func guacRelayLookup(proxyID string) *guacRelay {
    56		guacRelayRegMu.Lock()
    57		defer guacRelayRegMu.Unlock()
    58		return guacRelayReg[proxyID]
    59	}
    60	
    61	// startGuacRelay 起中继 listener。返回 relay 与「guacd 应拨的地址」
    62	// （127.0.0.1:<ephemeral>，进 connect 指令的 hostname/port）。
    63	func startGuacRelay(agentID, proxyID, target string, send func(string, *protocol.Message) error) (*guacRelay, string, error) {
    64		ln, err := net.Listen("tcp", "127.0.0.1:0")
    65		if err != nil {
    66			return nil, "", fmt.Errorf("listen relay: %w", err)
    67		}
    68		r := &guacRelay{
    69			agentID: agentID,
    70			proxyID: proxyID,
    71			target:  target,
    72			send:    send,
    73			ln:      ln,
    74			conns:   make(map[string]net.Conn),
    75		}
    76		guacRelayRegMu.Lock()
    77		guacRelayReg[proxyID] = r
    78		guacRelayRegMu.Unlock()
    79		go r.acceptLoop()
    80		return r, ln.Addr().String(), nil
    81	}
    82	
    83	// Close 幂等关闭：listener + 全部 guacd 侧连接（并逐一通知 agent 拆链）。
    84	func (r *guacRelay) Close() {
    85		r.closeOnce.Do(func() {
    86			guacRelayRegMu.Lock()
    87			delete(guacRelayReg, r.proxyID)
    88			guacRelayRegMu.Unlock()
    89			_ = r.ln.Close()
    90			r.mu.Lock()
    91			connIDs := make([]string, 0, len(r.conns))
    92			for id := range r.conns {
    93				connIDs = append(connIDs, id)
    94			}
    95			r.mu.Unlock()
    96			for _, id := range connIDs {
    97				r.removeConn(id, "session closed")
    98			}
    99		})
   100	}
   101	
   102	func (r *guacRelay) acceptLoop() {
   103		for {
   104			conn, err := r.ln.Accept()
   105			if err != nil {
   106				return // listener 已关（会话结束）
   107			}
   108			go r.handleConn(conn)
   109		}
   110	}
   111	
   112	// handleConn guacd 的一次拨入：注册连接 → 让 agent 拨目标 → 起 guacd→agent 泵。
   113	// 注册先于 proxy_new：agent 回程数据到达时 connID 必已可路由。
   114	func (r *guacRelay) handleConn(conn net.Conn) {
   115		connID := fmt.Sprintf("%s-%d", r.proxyID, r.seq.Add(1))
   116		r.mu.Lock()
   117		r.conns[connID] = conn
   118		r.mu.Unlock()
   119	
   120		// 裸 TCP 分支：agent 不解析协议内容，协议端点是 guacd（SSH/RDP/VNC 都是）
   121		newConnMsg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
   122			"proxyId":   r.proxyID,
   123			"proxyType": "tcp",
   124			"target":    r.target,
   125			"connId":    connID,
   126			"newConn":   true,
   127		})
   128		if err := r.send(r.agentID, newConnMsg); err != nil {
   129			log.Printf("Guacamole relay: proxy_new to agent %s failed: %v", r.agentID, err)
   130			r.removeConn(connID, "agent unavailable")
   131			return
   132		}
   133		r.pumpToAgent(conn, connID)
   134	}
   135	
   136	// pumpToAgent guacd → agent 方向泵。出口（EOF/错误）即拆链并通知 agent。
   137	func (r *guacRelay) pumpToAgent(conn net.Conn, connID string) {
   138		defer r.removeConn(connID, "guacd closed")
   139		buf := make([]byte, 32*1024)
   140		for {
   141			n, err := conn.Read(buf)
   142			if n > 0 {
   143				dataMsg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
   144					"proxyId": r.proxyID,
   145					"connId":  connID,
   146					"data":    append([]byte(nil), buf[:n]...),
   147				})
   148				if err := r.send(r.agentID, dataMsg); err != nil {
   149					log.Printf("Guacamole relay: send data to agent %s failed: %v", r.agentID, err)
   150					return
   151				}
   152			}
   153			if err != nil {
   154				if err != io.EOF {
   155					log.Printf("Guacamole relay: read from guacd side %s failed: %v", connID, err)
   156				}
   157				return
   158			}
   159		}
   160	}
   161	
   162	// removeConn 拆一条连接：关 guacd 侧 socket + 通知 agent（幂等——agent 侧
   163	// 先关时会收到对不存在 connID 的 proxy_close，静默忽略）。
   164	func (r *guacRelay) removeConn(connID, reason string) {
   165		r.mu.Lock()
   166		conn, ok := r.conns[connID]
   167		if ok {
   168			delete(r.conns, connID)
   169		}
   170		r.mu.Unlock()
   171		if !ok {
   172			return
   173		}
   174		log.Printf("Guacamole relay: tearing down %s (%s)", connID, reason)
   175		_ = conn.Close()
   176		closeMsg := protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
   177			"proxyId": r.proxyID,
   178			"connId":  connID,
   179			"reason":  reason,
   180		})
   181		_ = r.send(r.agentID, closeMsg)
   182	}
   183	
   184	// guacRelayDeliver agent → guacd 方向（server.handleProxyData 路由进）。
   185	func guacRelayDeliver(proxyID, connID string, data []byte) error {
   186		r := guacRelayLookup(proxyID)
   187		if r == nil {
   188			return fmt.Errorf("relay %s not found", proxyID)
   189		}
   190		r.mu.Lock()
   191		conn, ok := r.conns[connID]
   192		r.mu.Unlock()
   193		if !ok {
   194			return fmt.Errorf("relay conn %s not found", connID)
   195		}
   196		_, err := conn.Write(data)
   197		if err != nil {
   198			r.removeConn(connID, "write to guacd failed")
   199		}
   200		return err
   201	}
   202	
   203	// guacRelayHandleClose agent 侧关闭（目标断开/写错误）→ 关 guacd 侧连接，
   204	// guacd 随之向浏览器走协议层错误路径。
   205	func guacRelayHandleClose(proxyID, connID, reason string) {
   206		r := guacRelayLookup(proxyID)
   207		if r == nil {
   208			return
   209		}
   210		r.removeConn(connID, "target closed: "+reason)
   211	}
   212	
   213	// guacRelayHandleError agent 拨目标失败（SendError）→ 关 guacd 侧连接，
   214	// 让 guacd 的 connect 立即失败而非挂到超时。
   215	func guacRelayHandleError(proxyID, connID, errMsg string) {
   216		r := guacRelayLookup(proxyID)
   217		if r == nil {
   218			return
   219		}
   220		if connID == "" {
   221			// 无 connId 的错误无法定位连接，整条中继拆掉（guacd connect 必败）
   222			log.Printf("Guacamole relay: agent error (no connId), closing relay %s: %s", proxyID, errMsg)
   223			r.Close()
   224			return
   225		}
   226		log.Printf("Guacamole relay: agent dial failed for %s: %s", connID, errMsg)
   227		r.removeConn(connID, "agent dial failed: "+errMsg)
   228	}
[0m$ [0mnl -ba internal/agent/rpc/stack_provider.go | sed -n '220,570p'; nl -ba internal/agent/rpc/stack_git.go | sed -n '1,360p'; nl -ba internal/server/api_stack_webhook.go | sed -n '1,280p'
220	}
   221	
   222	// stackTask 异步部署任务
   223	type stackTask struct {
   224		ID         string
   225		Stack      string
   226		Action     string
   227		Status     string
   228		Log        *cappedBuffer
   229		StartedAt  time.Time
   230		FinishedAt time.Time
   231	}
   232	
   233	// stackLastTask 落盘的任务元数据（.cockpit/last-task.json），
   234	// agent 重启后任务内存态丢失，靠它维持列表里的最近部署信息
   235	type stackLastTask struct {
   236		Action     string `json:"action"`
   237		Status     string `json:"status"`
   238		FinishedAt int64  `json:"finishedAt"`
   239	}
   240	
   241	// cappedBuffer 只保留最后 limit 字节的环形缓冲。Write/String 会被不同
   242	// 协程并发调用（任务执行协程写日志、GetTask 轮询读），内部自持锁。
   243	type cappedBuffer struct {
   244		mu    sync.Mutex
   245		limit int
   246		buf   []byte
   247	}
   248	
   249	func (b *cappedBuffer) Write(p []byte) (int, error) {
   250		b.mu.Lock()
   251		defer b.mu.Unlock()
   252		b.buf = append(b.buf, p...)
   253		if len(b.buf) > b.limit {
   254			b.buf = b.buf[len(b.buf)-b.limit:]
   255		}
   256		return len(p), nil
   257	}
   258	
   259	func (b *cappedBuffer) String() string {
   260		b.mu.Lock()
   261		defer b.mu.Unlock()
   262		return string(b.buf)
   263	}
   264	
   265	// ============ 路径安全 ============
   266	
   267	// stackDir 校验名称并返回 stack 目录（名称正则排除分隔符，Join 后无需再校验前缀）
   268	func (p *StackProvider) stackDir(name string) (string, error) {
   269		if !stackNameRe.MatchString(name) {
   270			return "", fmt.Errorf("invalid stack name: %q", name)
   271		}
   272		return filepath.Join(p.dir, name), nil
   273	}
   274	
   275	// findComposeFile 返回 stack 目录内优先级最高的 compose 文件名，不存在返回 ""
   276	func findComposeFile(dir string) string {
   277		for _, f := range stackComposeFiles {
   278			if st, err := os.Stat(filepath.Join(dir, f)); err == nil && !st.IsDir() {
   279				return f
   280			}
   281		}
   282		return ""
   283	}
   284	
   285	// ============ 列表与状态 ============
   286	
   287	// ListStacks 列出所有 stack（目录即 stack，无 compose 文件的目录跳过）
   288	func (p *StackProvider) ListStacks() ([]StackSummary, error) {
   289		entries, err := os.ReadDir(p.dir)
   290		if err != nil {
   291			if os.IsNotExist(err) {
   292				return []StackSummary{}, nil
   293			}
   294			return nil, fmt.Errorf("read stacks dir: %w", err)
   295		}
   296	
   297		// 一次容器列表调用覆盖所有 stack（按 compose project label 分组）
   298		byProject := map[string][]docker.ContainerInfo{}
   299		if p.docker != nil {
   300			if containers, err := p.docker.ListContainers(true); err == nil {
   301				for _, cnt := range containers {
   302					project := cnt.Labels["com.docker.compose.project"]
   303					if project != "" {
   304						byProject[project] = append(byProject[project], cnt)
   305					}
   306				}
   307			}
   308		}
   309	
   310		result := make([]StackSummary, 0, len(entries))
   311		for _, e := range entries {
   312			name := e.Name()
   313			if !e.IsDir() || strings.HasPrefix(name, ".") {
   314				continue
   315			}
   316			dir := filepath.Join(p.dir, name)
   317			file := findComposeFile(dir)
   318			if file == "" {
   319				continue
   320			}
   321			result = append(result, p.buildSummary(name, file, byProject[name]))
   322		}
   323		sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
   324		return result, nil
   325	}
   326	
   327	// StatusStack 单个 stack 的状态明细
   328	func (p *StackProvider) StatusStack(name string) (*StackSummary, error) {
   329		dir, err := p.stackDir(name)
   330		if err != nil {
   331			return nil, err
   332		}
   333		file := findComposeFile(dir)
   334		if file == "" {
   335			return nil, fmt.Errorf("stack %s not found", name)
   336		}
   337	
   338		var containers []docker.ContainerInfo
   339		if p.docker != nil {
   340			all, err := p.docker.ListContainers(true)
   341			if err != nil {
   342				return nil, fmt.Errorf("list containers: %w", err)
   343			}
   344			for _, cnt := range all {
   345				if cnt.Labels["com.docker.compose.project"] == name {
   346					containers = append(containers, cnt)
   347				}
   348			}
   349		}
   350		summary := p.buildSummary(name, file, containers)
   351		return &summary, nil
   352	}
   353	
   354	func (p *StackProvider) buildSummary(name, composeFile string, containers []docker.ContainerInfo) StackSummary {
   355		summary := StackSummary{
   356			Name:        name,
   357			Services:    make([]StackService, 0, len(containers)),
   358			ComposeFile: composeFile,
   359		}
   360		for _, cnt := range containers {
   361			state := strings.ToLower(cnt.State)
   362			if state == "running" {
   363				summary.Running++
   364			}
   365			summary.Total++
   366			// 已发布端口（hostPort>0）随服务上报；未发布的端口仅容器网络内可达，
   367			// probe 无法触达，不进列表
   368			ports := make([]StackPort, 0, len(cnt.Ports))
   369			for _, p := range cnt.Ports {
   370				if p.HostPort > 0 {
   371					ports = append(ports, StackPort{
   372						HostPort:      p.HostPort,
   373						ContainerPort: p.ContainerPort,
   374						Proto:         p.Proto,
   375					})
   376				}
   377			}
   378			summary.Services = append(summary.Services, StackService{
   379				// 服务名取 compose service label，缺失时退化为容器名
   380				Name:        firstNonEmpty(cnt.Labels["com.docker.compose.service"], cnt.Name),
   381				Image:       cnt.Image,
   382				State:       state,
   383				Status:      cnt.Status,
   384				ContainerID: cnt.ID,
   385				Ports:       ports,
   386			})
   387		}
   388		if lt := readLastTask(filepath.Join(p.dir, name, stackReservedDir)); lt != nil {
   389			summary.LastAction = lt.Action
   390			summary.LastStatus = lt.Status
   391			summary.LastDeployedAt = lt.FinishedAt
   392		}
   393		return summary
   394	}
   395	
   396	func firstNonEmpty(a, b string) string {
   397		if a != "" {
   398			return a
   399		}
   400		return b
   401	}
   402	
   403	func readLastTask(cockpitDir string) *stackLastTask {
   404		data, err := os.ReadFile(filepath.Join(cockpitDir, "last-task.json"))
   405		if err != nil {
   406			return nil
   407		}
   408		var lt stackLastTask
   409		if json.Unmarshal(data, &lt) != nil {
   410			return nil
   411		}
   412		return &lt
   413	}
   414	
   415	func (p *StackProvider) persistLastTask(stack string, task *stackTask) {
   416		stackDir := filepath.Join(p.dir, stack)
   417		// stack 已被删除（remove 任务）时不重建目录
   418		if _, err := os.Stat(stackDir); err != nil {
   419			return
   420		}
   421		dir := filepath.Join(stackDir, stackReservedDir)
   422		if err := os.MkdirAll(dir, 0o700); err != nil {
   423			return
   424		}
   425		lt := stackLastTask{Action: task.Action, Status: task.Status, FinishedAt: task.FinishedAt.Unix()}
   426		data, _ := json.Marshal(lt) // 纯标量结构体，Marshal 不会失败
   427		_ = os.WriteFile(filepath.Join(dir, "last-task.json"), data, 0o644)
   428	}
   429	
   430	// ============ 文件读写 ============
   431	
   432	// GetStackFile 读取 compose/.env 内容
   433	func (p *StackProvider) GetStackFile(name string) (*StackFile, error) {
   434		dir, err := p.stackDir(name)
   435		if err != nil {
   436			return nil, err
   437		}
   438		file := findComposeFile(dir)
   439		if file == "" {
   440			return nil, fmt.Errorf("stack %s not found", name)
   441		}
   442	
   443		result := &StackFile{Name: name, ComposeFile: file}
   444		data, err := os.ReadFile(filepath.Join(dir, file))
   445		if err != nil {
   446			return nil, fmt.Errorf("read compose file: %w", err)
   447		}
   448		result.Compose = string(data)
   449	
   450		if st, err := os.Stat(filepath.Join(dir, file)); err == nil {
   451			result.ModifiedAt = st.ModTime().Unix()
   452		}
   453		if env, err := os.ReadFile(filepath.Join(dir, ".env")); err == nil {
   454			result.Env = string(env)
   455		}
   456		return result, nil
   457	}
   458	
   459	// SaveStackFile 保存 compose/.env，保存前先过 `docker compose config -q` 校验，
   460	// 失败不落盘。首次保存时返回 created=true 供 server 记审计。
   461	func (p *StackProvider) SaveStackFile(params map[string]interface{}) (map[string]interface{}, error) {
   462		name := stackParam(params)
   463		compose, _ := params["compose"].(string)
   464		envPtr := func() *string {
   465			if v, ok := params["env"].(string); ok {
   466				return &v
   467			}
   468			return nil
   469		}()
   470	
   471		dir, err := p.stackDir(name)
   472		if err != nil {
   473			return nil, err
   474		}
   475		if compose == "" {
   476			return nil, fmt.Errorf("compose content required")
   477		}
   478		if len(compose) > stackFileLimit {
   479			return nil, fmt.Errorf("compose file too large (max %d bytes)", stackFileLimit)
   480		}
   481		if envPtr != nil && len(*envPtr) > stackFileLimit {
   482			return nil, fmt.Errorf(".env file too large (max %d bytes)", stackFileLimit)
   483		}
   484	
   485		existed := findComposeFile(dir) != ""
   486	
   487		cockpitDir := filepath.Join(dir, stackReservedDir)
   488		if err := os.MkdirAll(cockpitDir, 0o700); err != nil {
   489			return nil, fmt.Errorf("create stack dir: %w", err)
   490		}
   491	
   492		// 先写临时文件再校验，避免校验失败留下损坏的 compose.yml
   493		tmp := filepath.Join(cockpitDir, "compose.yml.tmp")
   494		if err := os.WriteFile(tmp, []byte(compose), 0o644); err != nil {
   495			return nil, fmt.Errorf("write compose temp file: %w", err)
   496		}
   497		if err := p.validateCompose(dir, tmp); err != nil {
   498			os.Remove(tmp)
   499			return nil, err
   500		}
   501	
   502		// 统一规范为 compose.yml，清理旧命名
   503		for _, f := range stackComposeFiles {
   504			if f != "compose.yml" {
   505				os.Remove(filepath.Join(dir, f))
   506			}
   507		}
   508		if err := os.Rename(tmp, filepath.Join(dir, "compose.yml")); err != nil {
   509			return nil, fmt.Errorf("save compose file: %w", err)
   510		}
   511		if p.baseline != nil {
   512			p.baseline.Record("stack", name+"/compose.yml", []byte(compose))
   513		}
   514	
   515		if envPtr != nil {
   516			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(*envPtr), 0o600); err != nil {
   517				return nil, fmt.Errorf("save .env file: %w", err)
   518			}
   519			if p.baseline != nil {
   520				p.baseline.Record("stack", name+"/.env", []byte(*envPtr))
   521			}
   522		}
   523	
   524		return map[string]interface{}{"status": "saved", "created": !existed}, nil
   525	}
   526	
   527	// validateCompose 用 `docker compose config -q` 校验 YAML；
   528	// --project-directory 保证文件内的相对路径按 stack 目录解析
   529	func (p *StackProvider) validateCompose(dir, composeFile string) error {
   530		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
   531		defer cancel()
   532		cmd := exec.CommandContext(ctx, p.composeBin[0],
   533			p.composeArgs("--project-directory", dir, "-f", composeFile, "config", "-q")...)
   534		cmd.Dir = dir
   535		if out, err := cmd.CombinedOutput(); err != nil {
   536			return fmt.Errorf("compose validation failed: %s", strings.TrimSpace(string(out)))
   537		}
   538		return nil
   539	}
   540	
   541	// composeArgs 拼接 compose 子命令参数（composeBin[1:] 之后的部分）
   542	func (p *StackProvider) composeArgs(args ...string) []string {
   543		rest := append([]string{}, p.composeBin[1:]...)
   544		return append(rest, args...)
   545	}
   546	
   547	// ============ 异步任务 ============
   548	
   549	func (p *StackProvider) stackLock(name string) *sync.Mutex {
   550		p.mu.Lock()
   551		defer p.mu.Unlock()
   552		if p.stackLocks[name] == nil {
   553			p.stackLocks[name] = &sync.Mutex{}
   554		}
   555		return p.stackLocks[name]
   556	}
   557	
   558	// UpStack 异步执行 compose up -d
   559	func (p *StackProvider) UpStack(name string) (map[string]interface{}, error) {
   560		dir, err := p.stackDir(name)
   561		if err != nil {
   562			return nil, err
   563		}
   564		if findComposeFile(dir) == "" {
   565			return nil, fmt.Errorf("stack %s not found", name)
   566		}
   567		return p.launchTask(name, "up", func(task *stackTask) error {
   568			return p.runCompose(task, dir, "up", "-d", "--remove-orphans")
   569		})
   570	}
     1	package rpc
     2	
     3	import (
     4		"context"
     5		"fmt"
     6		"os"
     7		"os/exec"
     8		"path/filepath"
     9		"regexp"
    10		"strings"
    11		"time"
    12	)
    13	
    14	// ============ Stack Git 化（P2 ① Git-to-deploy） ============
    15	//
    16	// stacks 根目录整体作为一个 git 仓库接入远端（bare 仓/任意 git remote），
    17	// pull 用 fetch + --ff-only 快进（不自动 commit、不 merge 本地分叉），
    18	// drift = 工作区相对 HEAD 的改动（stack.file.save 的 UI 编辑即本地改动，
    19	// 不悄悄替用户提交——compose-file-first 原则的延伸）。
    20	// 设计决策见 docs/guide/stack-deploy-design.md P2 章节与 decision-log。
    21	
    22	var (
    23		// gitBranchRe 分支名约束：拒绝以 - 开头（防被当作 git 选项）、拒绝 ..（防
    24		// 跨 ref 路径）；git 自身对分支字符还有校验，这里是入口侧的最小防线
    25		gitBranchRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$`)
    26	
    27		// gitCredLikeRe URL 内 userinfo 凭据（user:pass@）——任何回显前抹除，
    28		// 凭据不进日志/审计/错误消息
    29		gitCredLikeRe = regexp.MustCompile(`(//)[^/@\s]+@`)
    30	)
    31	
    32	// gitOutputTimeout 单条 git 命令的墙钟上限；fetch 走网络给足余量
    33	const gitOutputTimeout = 60 * time.Second
    34	
    35	// StackGitChange 工作区相对仓库的一条改动（porcelain XY 码 + 路径）
    36	type StackGitChange struct {
    37		Code string `json:"code"`
    38		Path string `json:"path"`
    39	}
    40	
    41	// StackGitStatus stacks 目录的 git 接入状态（未接入不是错误，是常态）
    42	type StackGitStatus struct {
    43		Enabled bool   `json:"enabled"`
    44		Reason  string `json:"reason,omitempty"` // git-unavailable / not-a-repo
    45		Remote  string `json:"remote,omitempty"`
    46		Branch  string `json:"branch,omitempty"`
    47		Head    string `json:"head,omitempty"` // 本地 HEAD（短 SHA；未提交为空）
    48		// RemoteHead origin/<branch> 的短 SHA（从未 fetch 到为空）；
    49		// head != remoteHead 且 behind>0 → 有可拉取更新
    50		RemoteHead string           `json:"remoteHead,omitempty"`
    51		Ahead      int              `json:"ahead"`
    52		Behind     int              `json:"behind"`
    53		Changes    []StackGitChange `json:"changes,omitempty"` // drift：本地改动 vs 仓库
    54	}
    55	
    56	// gitBin 返回 git 可执行（StackConfig.GitBin 可注入；nil/空 = 系统 git）
    57	func (p *StackProvider) gitBin() []string {
    58		if len(p.gitCmd) > 0 {
    59			return p.gitCmd
    60		}
    61		return []string{"git"}
    62	}
    63	
    64	// gitAvailable git 二进制是否可执行
    65	func (p *StackProvider) gitAvailable() bool {
    66		bin := p.gitBin()
    67		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    68		defer cancel()
    69		return exec.CommandContext(ctx, bin[0], append(bin[1:], "--version")...).Run() == nil
    70	}
    71	
    72	// runGit 在 dir 下执行一条 git 命令，返回合并后的 stdout+stderr。
    73	// 错误文本统一过 sanitizeGitSecret（git 会把带凭据的 URL 原样回显）。
    74	func (p *StackProvider) runGit(dir string, args ...string) (string, error) {
    75		bin := p.gitBin()
    76		ctx, cancel := context.WithTimeout(context.Background(), gitOutputTimeout)
    77		defer cancel()
    78		cmd := exec.CommandContext(ctx, bin[0], append(bin[1:], args...)...)
    79		cmd.Dir = dir
    80		out, err := cmd.CombinedOutput()
    81		return sanitizeGitSecret(string(out)), err
    82	}
    83	
    84	// sanitizeGitSecret 抹除输出里的 userinfo 凭据
    85	func sanitizeGitSecret(s string) string {
    86		return gitCredLikeRe.ReplaceAllString(s, "$1***@")
    87	}
    88	
    89	// isGitRepo stacks 目录是否已是 git 仓库（worktree 根或子目录均算）
    90	func isGitRepo(dir string) bool {
    91		_, err := os.Stat(filepath.Join(dir, ".git"))
    92		return err == nil
    93	}
    94	
    95	// GitStatus 查 stacks 目录的 git 接入与 drift 状态
    96	func (p *StackProvider) GitStatus() *StackGitStatus {
    97		st := &StackGitStatus{}
    98		if !p.gitAvailable() {
    99			st.Reason = "git-unavailable"
   100			return st
   101		}
   102		if !isGitRepo(p.dir) {
   103			st.Reason = "not-a-repo"
   104			return st
   105		}
   106	
   107		out, err := p.runGit(p.dir, "config", "--get", "remote.origin.url")
   108		if err == nil {
   109			st.Remote = sanitizeGitSecret(strings.TrimSpace(out))
   110		}
   111		out, err = p.runGit(p.dir, "rev-parse", "--abbrev-ref", "HEAD")
   112		if err == nil {
   113			st.Branch = strings.TrimSpace(out)
   114		}
   115		if st.Branch == "" || st.Branch == "HEAD" {
   116			st.Branch = "main"
   117		}
   118	
   119		// 本地 HEAD：unborn（空仓库）时 rev-parse 失败，Head 留空
   120		out, err = p.runGit(p.dir, "rev-parse", "--short", "HEAD")
   121		if err == nil {
   122			st.Head = strings.TrimSpace(out)
   123		}
   124	
   125		// origin/<branch>：fetch 过才有
   126		remoteRef := "refs/remotes/origin/" + st.Branch
   127		out, err = p.runGit(p.dir, "rev-parse", "--short", remoteRef)
   128		if err == nil {
   129			st.RemoteHead = strings.TrimSpace(out)
   130		}
   131		if st.Head != "" && st.RemoteHead != "" {
   132			out, err = p.runGit(p.dir, "rev-list", "--left-right", "--count", "HEAD..."+remoteRef)
   133			if err == nil {
   134				fmt.Sscanf(strings.TrimSpace(out), "%d\t%d", &st.Ahead, &st.Behind)
   135			}
   136		}
   137	
   138		st.Changes = p.gitDriftChanges()
   139		st.Enabled = true
   140		return st
   141	}
   142	
   143	// gitDriftChanges 工作区相对 HEAD 的改动清单（porcelain v1，逐行 "XY path"）。
   144	// 过滤：.cockpit/ 保留目录（部署日志等机器产物，永不属于 drift）、.git/。
   145	// 重命名行取 -> 后的新路径。
   146	func (p *StackProvider) gitDriftChanges() []StackGitChange {
   147		// porcelain -uall：untracked 逐文件展开（drift 提示精确到文件而非目录）
   148		out, err := p.runGit(p.dir, "status", "--porcelain", "-uall")
   149		if err != nil {
   150			return nil
   151		}
   152		changes := make([]StackGitChange, 0, 8)
   153		for _, line := range strings.Split(out, "\n") {
   154			if len(line) < 4 {
   155				continue
   156			}
   157			code := strings.TrimSpace(line[:2])
   158			path := strings.TrimSpace(line[3:])
   159			if idx := strings.LastIndex(path, " -> "); idx >= 0 {
   160				path = path[idx+4:]
   161			}
   162			path = strings.Trim(path, `"`)
   163			if path == "" {
   164				continue
   165			}
   166			if gitPathSkipped(path) {
   167				continue
   168			}
   169			changes = append(changes, StackGitChange{Code: code, Path: path})
   170		}
   171		if len(changes) == 0 {
   172			return nil
   173		}
   174		return changes
   175	}
   176	
   177	// gitPathSkipped drift 过滤规则：路径任一段为 .cockpit / .git 即跳过
   178	func gitPathSkipped(path string) bool {
   179		for _, seg := range strings.Split(filepath.ToSlash(path), "/") {
   180			if seg == stackReservedDir || seg == ".git" {
   181				return true
   182			}
   183		}
   184		return false
   185	}
   186	
   187	// validateGitRemote 远端地址最小校验：拒绝 git 传输助手语法（ext:: 等可
   188	// 执行任意命令）、拒绝 - 开头（防被当 git 选项）、拒绝控制字符。
   189	// 允许：本地路径 / file: / http(s): / ssh: / git: / scp 语法（user@host:path）。
   190	func validateGitRemote(remote string) error {
   191		if remote == "" {
   192			return fmt.Errorf("remote required")
   193		}
   194		if strings.Contains(remote, "::") {
   195			return fmt.Errorf("remote transport helpers (::) not allowed")
   196		}
   197		if strings.HasPrefix(remote, "-") {
   198			return fmt.Errorf("remote must not start with '-'")
   199		}
   200		for _, r := range remote {
   201			if r < 0x20 || r == 0x7f {
   202				return fmt.Errorf("remote contains control characters")
   203			}
   204		}
   205		return nil
   206	}
   207	
   208	// GitPull 从远端拉取 stacks 仓库。
   209	//
   210	// 首次接入（目录还不是仓库）：init → remote add → fetch → checkout -B
   211	// 到 origin/<branch>（要求远端分支已存在——空仓库先在别处推首版）。
   212	// 已接入：fetch 后 --ff-only 快进（本地有提交分叉时明确报错，不代做
   213	// rebase/merge 决策；工作区 tracked 文件被改时 git 拒绝合并，错误原文
   214	// 含冲突文件清单，正好是 drift 提示的素材）。
   215	func (p *StackProvider) GitPull(params map[string]interface{}) (map[string]interface{}, error) {
   216		remote := paramString(params, "remote")
   217		branch := paramString(params, "branch")
   218		if branch == "" {
   219			branch = "main"
   220		}
   221		if !gitBranchRe.MatchString(branch) || strings.Contains(branch, "..") {
   222			return nil, fmt.Errorf("invalid branch name: %q", branch)
   223		}
   224		if !p.gitAvailable() {
   225			return nil, fmt.Errorf("git unavailable on agent host")
   226		}
   227		if err := os.MkdirAll(p.dir, 0o700); err != nil {
   228			return nil, fmt.Errorf("create stacks dir: %w", err)
   229		}
   230	
   231		if !isGitRepo(p.dir) {
   232			if err := validateGitRemote(remote); err != nil {
   233				return nil, err
   234			}
   235			return p.gitAttach(remote, branch)
   236		}
   237		if remote != "" {
   238			if err := validateGitRemote(remote); err != nil {
   239				return nil, err
   240			}
   241			if _, err := p.runGit(p.dir, "remote", "set-url", "origin", remote); err != nil {
   242				// origin 尚不存在（init 过但没配过远端）
   243				if _, err2 := p.runGit(p.dir, "remote", "add", "origin", remote); err2 != nil {
   244					return nil, fmt.Errorf("set remote: %s", err2)
   245				}
   246			}
   247		}
   248	
   249		if _, err := p.runGit(p.dir, "fetch", "origin", "--prune"); err != nil {
   250			return nil, fmt.Errorf("fetch: %s", err)
   251		}
   252	
   253		// 目标分支：显式指定则切换（脏树冲突时 git 自行拒绝），否则用当前分支
   254		out, err := p.runGit(p.dir, "rev-parse", "--abbrev-ref", "HEAD")
   255		if err != nil {
   256			return nil, fmt.Errorf("read branch: %s", err)
   257		}
   258		current := strings.TrimSpace(out)
   259		if current != branch {
   260			if _, err := p.runGit(p.dir, "checkout", branch); err != nil {
   261				return nil, fmt.Errorf("checkout %s: %s", branch, err)
   262			}
   263		}
   264	
   265		remoteRef := "refs/remotes/origin/" + branch
   266		if _, err := p.runGit(p.dir, "rev-parse", "--verify", "--quiet", remoteRef); err != nil {
   267			return nil, fmt.Errorf("remote branch %s not found (empty repository? push a commit first)", branch)
   268		}
   269		if _, err := p.runGit(p.dir, "merge", "--ff-only", remoteRef); err != nil {
   270			// 本地改动 / 本地提交分叉——git 的拒绝理由就是给用户的 drift 提示
   271			return nil, fmt.Errorf("merge conflict, resolve local changes first: %s", err)
   272		}
   273	
   274		st := p.GitStatus()
   275		return map[string]interface{}{
   276			"enabled":    st.Enabled,
   277			"remote":     st.Remote,
   278			"branch":     st.Branch,
   279			"head":       st.Head,
   280			"remoteHead": st.RemoteHead,
   281			"ahead":      st.Ahead,
   282			"behind":     st.Behind,
   283			"changes":    st.Changes,
   284		}, nil
   285	}
   286	
   287	// gitAttach 首次接入：init + remote + fetch + checkout 落地远端内容
   288	func (p *StackProvider) gitAttach(remote, branch string) (map[string]interface{}, error) {
   289		if _, err := p.runGit(p.dir, "init"); err != nil {
   290			return nil, fmt.Errorf("git init: %s", err)
   291		}
   292		// 显式把 HEAD 指到目标分支（git init -b 需 2.28+，symbolic-ref 全版本可用）
   293		if _, err := p.runGit(p.dir, "symbolic-ref", "HEAD", "refs/heads/"+branch); err != nil {
   294			return nil, fmt.Errorf("set branch %s: %s", branch, err)
   295		}
   296		if _, err := p.runGit(p.dir, "remote", "add", "origin", remote); err != nil {
   297			return nil, fmt.Errorf("remote add: %s", err)
   298		}
   299		if _, err := p.runGit(p.dir, "fetch", "origin", "--prune"); err != nil {
   300			return nil, fmt.Errorf("fetch: %s", err)
   301		}
   302		remoteRef := "refs/remotes/origin/" + branch
   303		if _, err := p.runGit(p.dir, "rev-parse", "--verify", "--quiet", remoteRef); err != nil {
   304			return nil, fmt.Errorf("remote branch %s not found (empty repository? push a commit first)", branch)
   305		}
   306		// -B：unborn HEAD 下等价 checkout；若远端文件与既有本地栈重名，git 拒绝
   307		// 覆盖 untracked 并报冲突清单（不静默覆盖本地文件）
   308		if _, err := p.runGit(p.dir, "checkout", "-B", branch, remoteRef); err != nil {
   309			return nil, fmt.Errorf("checkout: %s", err)
   310		}
   311		st := p.GitStatus()
   312		return map[string]interface{}{
   313			"enabled":    st.Enabled,
   314			"remote":     st.Remote,
   315			"branch":     st.Branch,
   316			"head":       st.Head,
   317			"remoteHead": st.RemoteHead,
   318			"ahead":      st.Ahead,
   319			"behind":     st.Behind,
   320			"changes":    st.Changes,
   321		}, nil
   322	}
   323	
   324	// GitDeploy webhook 触发部署（P2 ②）：git 拉取后按「本次拉取改动的文件」
   325	// 决定重新部署哪些 stack——HEAD 前移时 diff old→new 取顶层目录名（即 stack
   326	// 名）；首次接入（原先无 HEAD）部署仓库里的全部 stack；HEAD 未动（只有本地
   327	// drift 或无更新）不部署。up 是异步任务（busy 的 stack 记 skipped 不报错）。
   328	func (p *StackProvider) GitDeploy(params map[string]interface{}) (map[string]interface{}, error) {
   329		oldHead := p.gitHeadShort()
   330		pullRes, err := p.GitPull(params)
   331		if err != nil {
   332			return nil, err
   333		}
   334		newHead, _ := pullRes["head"].(string)
   335	
   336		var names []string
   337		switch {
   338		case oldHead == "":
   339			// 首次接入：部署仓库里现有全部 stack
   340			if entries, err := os.ReadDir(p.dir); err == nil {
   341				for _, e := range entries {
   342					if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
   343						names = append(names, e.Name())
   344					}
   345				}
   346			}
   347		case oldHead != "" && newHead != "" && oldHead != newHead:
   348			// HEAD 前移：本次拉取改动的顶层目录（即 stack 名）
   349			if out, err := p.runGit(p.dir, "diff", "--name-only", oldHead, newHead); err == nil {
   350				for _, line := range strings.Split(out, "\n") {
   351					line = strings.TrimSpace(line)
   352					if i := strings.Index(line, "/"); i > 0 {
   353						line = line[:i]
   354					}
   355					if line != "" {
   356						names = append(names, line)
   357					}
   358				}
   359			}
   360		}
     1	package server
     2	
     3	import (
     4		"crypto/subtle"
     5		"encoding/json"
     6		"fmt"
     7		"net/http"
     8	
     9		"github.com/cuihairu/cockpit/internal/audit"
    10		"github.com/cuihairu/cockpit/internal/protocol"
    11	)
    12	
    13	// stacks webhook 触发部署（stack-deploy-design.md P2 ②）：
    14	//
    15	//	POST /api/stacks/webhook   头 X-Cockpit-Token == 配置 stacks.webhook_token
    16	//
    17	// git 仓库推送后由 forge 回调，无法走登录态——独立精确路由（先于
    18	// /api/stacks/ 认证前缀注册，ServeMux 最长匹配胜出）。令牌空 = 端点关闭
    19	// （默认关，显式配置开启，同 notification 红线口径）。webhook 只触发
    20	// 「拉取已接入 origin + 部署改动 stack」，不接受 remote 参数（不重定向 origin）。
    21	// body 可选 {"agentId":"...","branch":"..."}：指定单 agent，缺省扇出全部在线
    22	// agent（未接入仓库的 agent RPC 报 remote required → 失败仅落日志）。
    23	// 执行异步：响应即返回受理结果，部署结果看部署历史与审计（username=webhook）。
    24	
    25	// handleStackWebhook webhook 入口（不走认证中间件，token 即凭据）
    26	func (s *Server) handleStackWebhook(w http.ResponseWriter, r *http.Request) {
    27		if r.Method != http.MethodPost {
    28			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
    29			return
    30		}
    31		token := ""
    32		if s.cfg != nil && s.cfg.Stacks != nil {
    33			token = s.cfg.Stacks.WebhookToken
    34		}
    35		if token == "" {
    36			s.handleError(w, r, http.StatusNotFound, "stack webhook disabled")
    37			return
    38		}
    39		got := r.Header.Get("X-Cockpit-Token")
    40		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
    41			fmt.Printf("stack webhook rejected: bad token from %s\n", s.getClientIP(r))
    42			s.handleError(w, r, http.StatusUnauthorized, "invalid token")
    43			return
    44		}
    45	
    46		// body 可选：空 body = 全量扇出 + 当前分支
    47		params := map[string]interface{}{}
    48		if r.Body != nil {
    49			var body map[string]interface{}
    50			if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body != nil {
    51				params = body
    52			}
    53		}
    54		// remote 一律不接受：origin 以 agent 侧 .git/config 为准，
    55		// webhook 不重定向拉取来源（防注入内容改写部署源）
    56		delete(params, "remote")
    57	
    58		rpcParams := map[string]interface{}{}
    59		if b := stackString(params, "branch"); b != "" {
    60			rpcParams["branch"] = b
    61		}
    62	
    63		var targets []string
    64		if agentID := stackString(params, "agentId"); agentID != "" {
    65			if _, ok := s.registry.Get(agentID); !ok {
    66				s.handleError(w, r, http.StatusNotFound, "Agent not found")
    67				return
    68			}
    69			targets = []string{agentID}
    70		} else {
    71			for _, a := range s.registry.List() {
    72				if a.IsOnline(s.registry.timeouts.Heartbeat) {
    73					targets = append(targets, a.ID)
    74				}
    75			}
    76		}
    77	
    78		triggered := make([]string, 0, len(targets))
    79		for _, agentID := range targets {
    80			triggered = append(triggered, agentID)
    81			s.goBg(func() { s.runStackWebhookDeploy(agentID, rpcParams) })
    82		}
    83		s.writeJSON(w, http.StatusOK, map[string]interface{}{"triggered": triggered})
    84	}
    85	
    86	// runStackWebhookDeploy 单 agent 的异步拉取+部署：结果落审计与部署历史
    87	func (s *Server) runStackWebhookDeploy(agentID string, params map[string]interface{}) {
    88		resp, err := s.CallAgent(agentID, "stack.git.deploy", params)
    89		if err != nil {
    90			fmt.Printf("stack webhook deploy %s: %v\n", agentID, err)
    91			return
    92		}
    93		rpcResp, err := protocol.DecodeRPCResponse(resp)
    94		if err != nil || rpcResp.Status == "error" {
    95			msg := "invalid agent response"
    96			if err == nil {
    97				msg = rpcResp.Error
    98			}
    99			fmt.Printf("stack webhook deploy %s: %s\n", agentID, msg)
   100			return
   101		}
   102		data, _ := rpcResp.Data.(map[string]interface{})
   103		s.audit.LogResource("webhook", audit.ActionStackWebhookDeploy, audit.ResourceStack, agentID,
   104			map[string]interface{}{"agentId": agentID, "result": data}, "", "")
   105		for _, e := range stackDeployedEntries(data) {
   106			s.startStackDeployment(agentID, e.name, audit.ActionStackWebhookDeploy, e.taskID)
   107		}
   108	}
   109	
   110	// stackDeployedEntry result.deployed 里的一条 (stack, taskId)
   111	type stackDeployedEntry struct {
   112		name   string
   113		taskID string
   114	}
   115	
   116	// stackDeployedEntries 取 result.deployed 的 (stack, taskId) 对
   117	func stackDeployedEntries(data map[string]interface{}) []stackDeployedEntry {
   118		out := []stackDeployedEntry{}
   119		raw, ok := data["deployed"].([]interface{})
   120		if !ok {
   121			return out
   122		}
   123		for _, item := range raw {
   124			m, ok := item.(map[string]interface{})
   125			if !ok {
   126				continue
   127			}
   128			name := stackString(m, "stack")
   129			taskID := stackString(m, "taskId")
   130			if name != "" && taskID != "" {
   131				out = append(out, stackDeployedEntry{name: name, taskID: taskID})
   132			}
   133		}
   134		return out
   135	}
[0m$ [0mnl -ba docs/guide/jobs-design.md | sed -n '1,260p'; nl -ba docs/guide/remote-access-integration-design.md | sed -n '1,280p'; nl -ba docs/websocket-token-auth.md | sed -n '1,260p'; nl -ba docs/guide/rbac-design.md | sed -n '1,260p'
1	# P1 方案设计：执行 Job 模型（最小闭环）
     2	
     3	> 语言：**中文** ｜ [English](./jobs-design.en.md)
     4	
     5	> 2026-10-04。P1「Job」首期落地设计。战略评审（见 reference-projects）把
     6	> Job/Action/Workflow/Event 列为控制面四个核心抽象，本文只做 Job 的最小
     7	> 闭环：一条持久化的执行记录 + 一个可执行的内置类型（agent.exec），
     8	> Action/Workflow/Event 留待后续在 Job 之上生长。
     9	
    10	## 实现状态（2026-10-04 对账）
    11	
    12	- **已实现**（22bd7fd → 4a0660f 六笔）：本文「最小闭环」范围全链落地——
    13	  `storage.Job` 模型 + AutoMigrate；Job 类型 `agent.exec` 走 agent 侧 `job.exec`
    14	  RPC provider（命令 ≤16KB、超时 ≤300s 缺省 60s、输出 64KB 截断、超时按
    15	  进程组 SIGKILL）；`GET|POST /api/jobs` + `GET /api/jobs/{id}`（同步等待终态
    16	  回写）；终态单条审计 `job_run`（details 含 type/target/status/params）；
    17	  Jobs 页（全机台账 5s 轮询 + 创建即执行弹窗 + 终态详情）；权限点
    18	  `jobs:read`/`jobs:write`（PermGuard 控制创建入口）。
    19	- **未实现**（按本文「边界（明确不做）」节，立项时再设计，不写进当前能力）：
    20	  输出流式、Action/Event 抽象与 AI 接线（P5）。
    21	- **已由 P2 落地（2026-10-09，[workflow-design](./workflow-design.md) M1）**：
    22	  异步执行/取消（`cancelled` 终态、pending 可撤、台账过滤）与 Workflow 编排；
    23	  `POST /api/jobs` 语义随之改为「创建即 201 返回 pending，后台派发」（原同步
    24	  等待删除），timeout 仍按设计保持 `failed`（不设独立终态）。取消面再由 M2c
    25	  扩至 running：`POST /api/jobs/{id}/cancel` 经 `job.cancel` RPC 击杀在途进程
    26	  （2026-10-11）。Workflow/定时触发中，定时部分仍不做（workflow-design W11 约定）。
    27	- **真机验收**（2026-10-05，探针 `scripts/acceptance/jobs/` 五件套 9/9 PASS，
    28	  acceptance-checklist「统一 Job 执行」节七项全勾）：执行链路（82ms 成功回执 /
    29	  exit 3 输出带回 / 超时 1.078s 按时返回且 `pgrep` 无孤儿 / 截尾恰 64KB 尾部保留）、
    30	  离线 503 不落幽灵、校验面 400×7、终态审计恰一条、权限双角色 403 全部实证。
    31	  **验收逮到 D8 路由面缺口**：`/api/jobs` 未登记 `resourceRules`，RBAC
    32	  governed=false 整体放行（viewer 可执行），修为补规则 + 三角色矩阵 7 例。
    33	
    34	## 现状与痛点
    35	
    36	| 痛点 | 现状 | 后果 |
    37	|------|------|------|
    38	| 「点一下执行」没有统一入口 | 备份跑一次、cron 手动触发、服务重启各是各的 API，互不复用 | 每加一种动作就要新增一条 RPC + 一个页面入口 |
    39	| 执行无台账 | 执行完即散，只能翻审计日志拼现场 | 想查「昨晚那台机器跑过什么、输出是什么」无从下手 |
    40	| 权限粒度粗 | 备份/代理/定时任务各有权限点，通用命令执行没有模型 | 无法回答「谁能在这台机器上跑命令」 |
    41	| AI/自动化没有可挂接的执行面 | 无统一 Job API，任何自动化都要绕业务接口 | 战略评审 P5「AI 接线」无处落地 |
    42	
    43	## 架构总览
    44	
    45	与定时任务同一通道模型：**同步 RPC + server 落库**。区别于 cron 的
    46	「crontab 即状态、server 纯转发」，Job 的价值恰在台账——server 是事实源。
    47	
    48	```
    49	┌─ server ───────────────────────┐        ┌─ agent（目标主机）─────────────┐
    50	│ POST /api/jobs                 │ ─RPC─▶ │ job provider（新）              │
    51	│ 校验 → 建 Job(pending)         │        │ job.exec：sh -c / cmd /C        │
    52	│   → CallAgent(job.exec)        │ ◀─RPC─ │ 带超时执行，返回 exit_code/输出  │
    53	│   → 终态回写 + 审计 job_run    │        │ （输出截尾 64KB，业务字段不炸RPC）│
    54	│ GET /api/jobs[/{id}] 台账查询  │        └─────────────────────────────────┘
    55	└────────────────────────────────┘
    56	```
    57	
    58	## 关键决策
    59	
    60	| # | 决策 | 选择 | 理由 |
    61	|---|------|------|------|
    62	| D1 | 状态机 | `pending → running → success \| failed`；cancelled/timeout 不设独立态——超时被杀即 failed（error=「timed out after Ns」，exit_code=-1），取消是后续异步化才有的能力（**P2 后更新**：`cancelled` 为真实终态——pending DB-only 取消（W3），running 经 RPC 击杀（M2c）；timeout 仍并入 failed 未变） | 同步 RPC 下 running 为瞬态，终态二分已覆盖当下全部路径；状态集合按需扩展 |
    63	| D2 | 首个类型 | `agent.exec`：目标 agent 本机执行一条 shell 命令（unix `sh -c` / windows `cmd /C`） | Control 域第一块；命令执行是其它一切动作的兜底语义 |
    64	| D3 | 通道 | 复用既有 RPC：server `CallAgent(target, "job.exec", params)`，agent 端 JobProvider 注册（平台无关，无条件注册） | 与 cron/备份同构，零新基建 |
    65	| D4 | 退出码语义 | 非零退出码是**业务字段**不是协议错误：RPC 层恒 success，`exit_code`/`output`/`error` 走 data；server 据此落 success/failed（输出仍带回） | 「命令失败」是 Job 的正常产物，不应混入传输层错误 |
    66	| D5 | 超时 | 双端同限：入参 `timeout_s` 1-300（缺省 60）；unix 侧进程组隔离（Setpgid）+ 超时杀整组，防止孙进程握住输出管道导致「假超时」 | 超时必须真的按时返回；`sh -c` 派生链是最常见悬挂源 |
    67	| D6 | 输出上限 | agent 截尾保留末 64KB（`jobExecMaxOutput`），server 落库同限（`storage.JobMaxOutput`）；命令长度上限 16KB | 错误信息通常在尾部；防长输出表膨胀 |
    68	| D7 | 离线拦截 | 创建前查 registry，目标不在线直接 503，**不建幽灵记录** | 与 cron 同口径；台账里不出现「从未执行」的记录 |
    69	| D8 | 权限 | 权限点 `jobs:read` / `jobs:write`（resourceActions + allPermissions 两处登记）；服务端校验与 cron 同口径（路由层 auth middleware） | 台账人人可看（read），执行是高危操作（write） |
    70	| D9 | 审计 | 终态一条 `job_run`（resource=job，id=Job ID），details 含 type/target/status/params | 命令入参进审计与 cron_apply 同口径，便于追溯「谁在哪台机器跑了什么」 |
    71	| D10 | 存储 | SQLite `jobs` 表（AutoMigrate），Parameters 存 JSON 串、API 层解析回对象返回 | 单机个人云规模够用；gorm 现有惯例 |
    72	
    73	## Server 侧设计
    74	
    75	### REST API
    76	
    77	| 方法 | 路径 | 说明 |
    78	|------|------|------|
    79	| `POST /api/jobs` | `{type, target, parameters}` | 创建并执行，201 返回 pending 视图，后台派发终态 |
    80	| `POST /api/jobs/{id}/cancel` | — | 取消 pending/running Job（终态 409） |
    81	| `GET /api/jobs` | — | 最近 50 条台账（倒序） |
    82	| `GET /api/jobs/{id}` | — | 单条详情（含输出） |
    83	
    84	校验（`validateJobPayload`）：type 必填且在白名单（当前仅 `agent.exec`）；
    85	target 非空；exec 需 `parameters.command` 非空且 ≤16KB；`timeout_s` 1-300。
    86	
    87	### Job 视图
    88	
    89	```json
    90	{
    91	  "id": "…", "type": "agent.exec", "target": "agent-id", "actor": "username",
    92	  "status": "success", "parameters": {"command": "uptime"},
    93	  "output": "…尾部 64KB…", "exitCode": 0, "error": "",
    94	  "createdAt": "…", "startedAt": "…", "finishedAt": "…"
    95	}
    96	```
    97	
    98	## Agent 侧设计
    99	
   100	### RPC 方法
   101	
   102	| 方法 | 参数 | 返回 | 说明 |
   103	|------|------|------|------|
   104	| `job.exec` | `{command, timeout_s?}` | `{exit_code, output, truncated, error, duration_ms}` | exit_code -1 = 启动失败/超时被杀 |
   105	
   106	- 跨平台：`sh -c`（unix）/ `cmd /C`（windows），`exec.CommandContext` + deadline；
   107	- unix 侧 `Setpgid` 进程组 + Cancel 杀整组：`sh -c` 派生的孙进程（如 sleep）
   108	  会握住输出管道，只杀直接子进程会让 `CombinedOutput` 等到孙进程自然结束
   109	  ——超时语义必须是「整组即刻终止」；
   110	- 非 agent.exec 类平台差异（容器内执行、幂等 Action 等）是后续类型的事，
   111	  provider 的 action 分发已留位。
   112	
   113	## 前端设计
   114	
   115	- `web/src/pages/Jobs/index.tsx`：全机台账表（5s 轮询 + 手动刷新），
   116	  创建弹窗（在线 agent 下拉 + 命令 + 超时），详情弹窗（状态/退出码/输出尾部）；
   117	- `web/src/services/jobs.ts`：listJobs/createJob/getJob/cancelJob/createJobWithApproval（axios，Bearer 注入）；
   118	- 路由 `/jobs`，菜单「Job 执行」，perm `jobs:read`，创建按钮 `jobs:write`。
   119	
   120	## 测试
   121	
   122	- storage：CRUD/倒序/limit（`internal/storage/job_test.go`）；
   123	- agent rpc：成功/非零退出/超时杀整组/截尾/越限/未知 action（`internal/agent/rpc/cov_job_provider_test.go`，windows 恒适用用例直跑）；
   124	- server：校验各 400 分支、离线 503、成功/失败 dispatch、列表/详情/405/404（`internal/server/cov_jobs_test.go`，假 agent 注入）；
   125	- 覆盖率：防御分支登记 `tool/known_uncoverable.txt`（SQLite 查询/写入不可致错、auth 上下文单测惯例、协议不可变形）。
   126	
   127	## 边界（明确不做）
   128	
   129	- **不做异步执行**（**已由 P2 兑现，见顶部「已由 P2 落地」注记**）：创建即同步
   130	  等待终态。异步化（排队/取消/进度）等 Workflow 立项时一并设计，届时
   131	  cancelled/timeout 独立状态、`GET /jobs?status=running` 过滤、SSE 推送才有
   132	  真实的消费者（落地口径：`cancelled` 终态 + 台账过滤已实现；SSE 仍拍不做）；
   133	- **不做 Workflow/Action**：Workflow 是「一组 Job 的编排」（依赖/并行/重试），
   134	  Action 是「带语义的幂等操作」，都在 Job 之上，本期只有 Job 本体；
   135	- **不做定时触发**：Job 与 cron 是两个平面——cron 是 agent 侧调度面（crontab
   136	  即状态），Job 是控制面执行台账；「定时跑 Job」= cron 写一条调用 `POST
   137	  /api/jobs` 的命令行，不引入新耦合；
   138	- **不做输出实时流**：终态一次性回传。流式输出（WebSocket attach）留待
   139	  远控/终端域统一设计。
   140	
   141	## 后续生长点
   142	
   143	| 优先级 | 方向 | 依赖 |
   144	|--------|------|------|
   145	| P2 | Workflow：Job 编排（线性链/重试/取消，DAG/审批留 M2）——已落地（P2 异步化 + M1 编排 2026-10-09；M2a 变量/M2b 扇出/M2c 取消信号 2026-10-11，见 [workflow-design](./workflow-design)） | 本模型 |
   146	| P0′ | 事件模型：Job 终态事件进统一事件总线 | Event 抽象立项 |
   147	| P5 | AI 接线：Job API 即 AI 的执行面（agent.exec 之外补结构化类型） | 审计与限额先行 |
nl: docs/guide/remote-access-integration-design.md: No such file or directory
     1	# WebSocket 认证边界
     2	
     3	> 语言：**中文** ｜ [English](./websocket-token-auth.en.md)
     4	
     5	当前实现已经避免在远程连接 WebSocket URL 中直接携带 JWT。远程终端、VNC、桌面连接统一使用短期 ticket：terminal 入口仅经 `Sec-WebSocket-Protocol` 传递，guacamole 入口 URL query `?ticket=` 与子协议双通道（query 优先、子协议兜底）。
     6	
     7	## 当前流程
     8	
     9	1. Web UI 使用 JWT 调用 `POST /api/remote/tickets`。
    10	2. Server 生成内存态 ticket：
    11	   - 有效期 5 分钟。
    12	   - 单次使用。
    13	   - 绑定用户和连接参数。
    14	3. Web UI 建立 WebSocket 连接，并把 ticket 作为子协议值。
    15	4. Server 校验并消费 ticket，随后才升级连接并转发数据。
    16	
    17	```typescript
    18	const ticket = await createRemoteTicket(params)
    19	const ws = new WebSocket('/api/remote/terminal', [ticket])
    20	```
    21	
    22	对应服务端入口：
    23	
    24	- `/api/remote/terminal`
    25	- `/api/remote/guacamole`
    26	
    27	terminal 入口从 `Sec-WebSocket-Protocol` 读取 ticket；guacamole 入口先读 URL query `?ticket=`、缺失再回退子协议。两者都不把认证 token 放进通用审计。
    28	
    29	## 设计边界
    30	
    31	- JWT 只用于创建 ticket 的 HTTP API，不直接进入远程 WebSocket URL。
    32	- ticket 存在 Server 内存中，Server 重启后失效。
    33	- ticket 被成功验证后会立即标记为已消费，不能复用。
    34	- ticket 参数中可以包含 `agent_id`、`host`、`port`、`protocol`、`username`、`password`、`domain`、`width`、`height` 等连接参数。
    35	
    36	## 安全注意
    37	
    38	生产环境还需要配合以下配置：
    39	
    40	- 设置 `ALLOWED_ORIGINS`，限制 WebSocket Origin。
    41	- 使用 HTTPS/WSS 终止 TLS。
    42	- 避免在日志中打印 ticket 或远程连接密码。
    43	- 对远程连接和代理创建操作保留审计记录。
    44	
    45	## 历史说明
    46	
    47	早期文档讨论过 URL query token、短期连接 token、Cookie 认证等方案。当前代码采用“短期 ticket + WebSocket 子协议”的组合方式，是本文档应遵循的实现基线。
     1	# 方案设计：多账号与细粒度权限控制（RBAC）
     2	
     3	> 语言：**中文** ｜ [English](./rbac-design.en.md)
     4	
     5	> 2026-09-19。多账号权限控制首期设计。需求原文：「账号权限控制，支持多账号」。
     6	> 本文只做设计决策与接口定义，实现按 P0/P1 分期。
     7	
     8	## 现状与痛点
     9	
    10	多账号基础已存在：`User` 表（用户名/密码/TOTP/部门）、用户 CRUD API、JWT 携带
    11	`Role`。缺的是权限模型——判定全靠散落的字符串比较：
    12	
    13	| 痛点 | 现状 | 后果 |
    14	|------|------|------|
    15	| 只有两级角色 | `Role` 字段硬编码 `admin` / `user` 两种取值 | 想给运维同事「能操作但不能管用户」做不到 |
    16	| 判定散落 | 10+ 处 handler 手写 `user.Role != "admin"`（`api.go` 394/703/802/844/850/893/911…） | 新增模块容易漏判；审计「谁能干什么」要人肉 grep |
    17	| 无权限语义 | 「admin」同时意味着用户管理、证书签发、文件读写、终端接入 | 无法回答「这个账号能碰生产文件吗」 |
    18	| user 角色无定义 | 非 admin 除改自己资料外几乎什么都做不了，但这是隐式约定 | 前端菜单与后端判定各自为政 |
    19	
    20	## 架构总览
    21	
    22	RBAC-lite：**用户 → 角色 → 权限点**。权限判定收敛为 server 侧一个中间件，
    23	agent 与 websocket 通道零改动（权限在 server 收口，agent 只接受 server 单向调用）。
    24	
    25	```
    26	┌─ web ─────────────────┐      ┌─ server ──────────────────────────────────┐
    27	│ 路由守卫 + v-perm 指令 │─JWT─▶│ auth middleware：验签 → UserInfo{Role}     │
    28	│                       │      │   ↓                                        │
    29	│ /api/me 返回          │      │ requirePermission(perm)：按 Role 查角色表   │
    30	│ permissions[]         │      │   ↓（SQLite 本地读，微秒级，不做缓存）      │
    31	│ 菜单/按钮按权限裁剪    │      │ handler：删除散落的 Role != "admin"         │
    32	└───────────────────────┘      └────────────────────────────────────────────┘
    33	```
    34	
    35	## 关键决策
    36	
    37	| # | 决策 | 选择 | 理由 |
    38	|---|------|------|------|
    39	| D1 | 权限模型 | RBAC（角色→权限点集合），不做 ABAC/ACL | 管理面用户量个位数，按岗位授权足够；ABAC 的属性条件无处取值 |
    40	| D2 | 权限点粒度 | `<resource>:<action>`，action ∈ `read` / `write` / `admin` | 模块级足够（如 `files:write`、`acme:admin`）；按钮级授权成本高收益低 |
    41	| D3 | resource 清单 | 与现有 API 模块一一对应：`inventory` `files` `logs` `terminal` `docker` `stack` `cron` `backup` `acme` `dns` `ddns` `proxy` `overlay` `drift` `nas` `alerts` `audit` `users` `roles` `settings`；对账已补全 `services` `smart` `recordings` `firewall`（firewall-design D10，单 read 档）`jobs`（jobs-design）`workflows`（workflow-design W9）；`overlay` 含 `admin` 档（M3 D28，join/leave/service 本机侧写操作）。审批门 `approvals`（workflow-design M2d）已随 2026-10-15 文档审计补齐登记（此前路由已注册但权限点缺失导致端点 403）。当前全集以 `internal/storage/role.go` `resourceActions` 为单一事实源 | 后端已有路由前缀就是天然 resource，不发明新分类 |
    42	| D4 | 角色存储 | 新增 `Role` 表（`name` 主键、`permissions` JSON、`builtin` 标记）；`User.role` 沿用字符串存角色名 | GORM AutoMigrate 一张表搞定；User 不需要外键约束，角色名即软引用 |
    43	| D5 | 权限校验位置 | auth middleware 之后加 `requirePermission(perm)`；`admin` 权限点隐含该 resource 的 read/write | 中间件链一处收口；「admin 隐含读写」让内置 admin 角色不用枚举全部权限点 |
    44	| D6 | 每请求查库 | 校验时直接查 `roles` 表，不做内存缓存 | 管理面 QPS 个位数；缓存失效逻辑（角色改了要踢在线用户）比省的那点查询贵得多 |
    45	| D7 | JWT 内容 | 沿用现状：token 只带角色名 | 角色定义变更对已发 token **立即生效**（好性质）；把权限点塞进 token 反而要处理「token 未过期但权限已收窄」 |
    46	| D8 | 内置角色 | `admin`（全量含 `users:admin`/`roles:admin`）、`operator`（全部模块 read+write，无 users/roles/settings）、`viewer`（全模块 read） | 覆盖 90% 场景；自定义角色兜剩余 10% |
    47	| D9 | 存量迁移 | `role=admin` → admin；`role=user` → viewer，首次启动 seed 内置角色 | 保持现状语义（现在 user 就没有写权限），要放宽管理员手工升 operator |
    48	| D10 | agent 侧 | 零改动 | agent 无用户概念；RPC 由 server 发起，权限在 REST 层拦截即等效拦截 RPC |
    49	| D11 | websocket/终端 | 连接建立时校验 `terminal:write`，会话期内不复查 | 终端会话中途断权易产生半截命令；重连即按新权限判定 |
    50	| D12 | 角色管理 API | 仅 `roles:admin`（内置 admin）：CRUD 自定义角色；内置角色不可改删 | 防止自锁；自定义角色权限点必须落在 D3 清单内（后端校验） |
    51	| D13 | 自我保护 | 不可删除/降级最后一个有效 admin；不可修改自己的角色 | 常规多租户系统事故教训，一条校验省一次数据卷救援 |
    52	| D14 | 审计 | 角色增改、授权变更、越权拒绝（403）全部入审计日志 | 「谁给谁提了权」必须可追溯；403 记录用于发现权限配置不足 |
    53	
    54	## 权限点清单（D3 展开）
    55	
    56	> 完整权限点集以 `internal/storage/role.go` `resourceActions` 为单一事实源
    57	> （下表为摘录，含 P0/P1 补全的 `services`/`smart`/`recordings`/`firewall`/`jobs`）。
    58	
    59	```
    60	inventory:read/write    files:read/write       logs:read           terminal:write
    61	docker:read/write       stack:read/write       cron:read/write     backup:read/write
    62	acme:read/write/admin   dns:read/write         ddns:read/write     proxy:read/write
    63	overlay:read/write/admin drift:read/write     nas:read/write     alerts:read/write
    64	services:read/write     smart:read/write       recordings:read/write
    65	firewall:read           jobs:read/write         workflows:read/write
    66	approvals:read/write
    67	audit:read              users:admin            roles:admin         settings:admin
    68	```
    69	
    70	约定：`<r>:write` 隐含 `<r>:read`；`<r>:admin` 隐含 write（如 acme 的签发/吊销/
    71	CA 目录切换归 `acme:admin`，查看证书列表 `acme:read`）。
    72	
    73	## 分期
    74	
    75	| 期 | 内容 | 验收 |
    76	|----|------|------|
    77	| P0（后端） | Role 表 + 内置角色 seed + `requirePermission` 中间件 + 存量 handler 判定收敛 + 权限点清单常量 + D13/D14 校验 | 全部 API 在三内置角色下的 200/403 矩阵有测试覆盖 |
    78	| P1（前端+管理） | 用户/角色管理页、`/api/me` 返回 permissions、路由守卫与菜单裁剪、v-perm 按钮指令 | viewer 登录看不到任何写操作入口 |
    79	| P2（按需） | 自定义角色 UI 完善、权限模板、部门级授权（Department 字段已有，暂不参与判定） | 按实际需求排期 |
    80	
    81	## 实现状态（P0 拆笔）
    82	
    83	| 笔 | 内容 | 状态 |
    84	|----|------|------|
    85	| 1 | storage：`Role` 表（name 主键 / permissions JSON / builtin）+ 权限点常量与内置角色定义（admin/operator/viewer）+ seed（幂等，内置角色 permissions 以代码为准覆盖更新）+ CRUD（自定义角色权限点白名单校验；删除时拒内置、拒被引用）+ D9 存量迁移 `role=user → viewer` | 已完成 |
    86	| 2 | server：路由权限表（path 前缀 × method → 权限点，支持 AND 语义，domain-binding 增删改要求 `dns:write`+`proxy:write`）+ `authorize` 判定层（auth 后、serveAPI 分发前；每请求查库，角色缺失 fail-closed 403）+ 隐含规则（write⊇read、admin⊇write）+ 三角色矩阵测试。**不删存量判定**——authorize 拦在前，散落 `Role != "admin"` 暂成双保险 | 已完成 |
    87	| 3 | 收敛：删 10 处散落 `Role != "admin"`（api.go ×7、api_proxy.go ×3），users/settings 端点改走权限表。落地时追加两项：创建/更新用户时校验角色名在角色表存在（幽灵角色 fail-closed 会锁号，`errors.Is(ErrNotFound)` 才 400、库故障放行 500）；创建用户默认角色 user→viewer（D9 迁移后 user 已不存在）。改密免验旧密码条件从 admin 字符串改为 `userHasPerm(users:admin)` | 已完成 |
    88	| 4 | 角色/用户管理 API：`/api/roles` CRUD（仅 `roles:admin`）+ D13 自我保护（最后一个有效 admin 不可删/降级、不可改自己角色）+ D14 403 入审计。落地时追加三项：**「有效 admin」= 角色在角色表存在且覆盖 `users:admin`**（幽灵角色不算）；D13 计数出错时同样拒绝（fail-safe，库故障不做保护性变更）；RBAC 403 短路不经过内层 auth 挂点，审计层在请求前做可选认证（有效 Bearer 先塞 ctx）保证能记到「谁」被拒——内层 auth 幂等重复解析无害 | 已完成 |
    89	| P1 | web：用户/角色管理页、`/api/me` permissions、菜单裁剪（拆笔见下表） | 已完成（笔 5–8b，验收达成：viewer 全站无写入口） |
    90	
    91	## 实现状态（P1 拆笔）
    92	
    93	验收标准（设计阶段 P1 行）：viewer 登录看不到任何写操作入口。
    94	
    95	| 笔 | 内容 | 状态 |
    96	|----|------|------|
    97	| 5 | server：`/api/me` 返回 `permissions`（查角色表展开；幽灵角色 → 空清单，与判定层 fail-closed 一致） | 已完成 |
    98	| 6 | web 权限基础设施：UserContext 启动拉 `/api/me` 存 permissions（D7 语义：动态拉取而非 login 固化，角色变更刷新即生效）+ `usePerm`/`PermGuard`（前端复刻 write⊇read、admin⊇write 隐含规则）+ 菜单按权限标注与裁剪 + 路由守卫（无 read 权限 → 403 页；后端 RBAC 仍是权威，守卫纯 UX）。落地决策：permissions 不落 localStorage（刷新即重拉，避免陈旧判定）；未加载/加载失败一律 fail-closed 全裁剪；菜单项 perm 与后端 requiredPerms 的 GET 语义一一对齐（monitor→inventory、disk→smart、域名绑定→dns:read）；设置菜单全员可见（含个人 TOTP），Tab 级裁剪笔 8 | 已完成 |
    99	| 7 | web 用户/角色管理页：「访问控制」菜单组（用户管理 users:admin、角色管理 roles:admin），用户 CRUD/改角色/改密，角色 CRUD/权限点矩阵编辑（内置角色只读展示）。落地决策：合法权限点全集从内置 admin 角色的 permissions 推导（seed 恒全量，零后端改动）；用户表内联 Select 改角色（自己那行禁用——D13 前端预拦）；D13 后端拦截的错误信息经 message 透出 | 已完成 |
   100	| 8a | web 核心运维模块写入口裁剪：DNS/Domains/Docker/Stacks/Backups/Proxy/Cron/Services 的写按钮与危险操作包 `PermGuard`（Docker 容器操作按 action 过滤——logs 是读）。落地决策：Services 与 Docker 同模式（组件顶层 `usePerm` + 条件渲染，journal 日志按钮保留）；Cron 启用开关用 PermGuard 禁用态兜底（Switch 无写权限显示 disabled 而非消失，保留状态可见性）；DDNS/ServerBackup 的巡检/定时配置行只包保存按钮（开关改动是本地暂存态，无保存入口即不落库，与读展示共存）；Proxy「配置」查看是读保留 | 已完成 |
   101	| 8b | web 其余模块写入口裁剪：Acme（签发/部署 acme:admin）/Nas/Drift/Network/Disk/Recordings/Workbench（terminal:write 远控入口、files:write 文件操作）+ Settings 页 Tab 级（通用/告警 settings:admin，安全 Tab 全员——个人 TOTP）。落地决策：Acme 签发/部署按钮按后端固定 acme:admin、编辑/删除/新建/账户邮箱按 acme:write 分开标注（有 acme:write 无 admin 的角色可管配置但不能签发，与后端判定一一对齐）；Workbench 顶部 ssh/rdp/vnc 按钮组与 ConnectionPanel 连接按钮双入口都包 terminal:write；FileBrowser 预览/下载/搜索保留、编辑/权限/重命名/删除/新建目录/上传按 canWrite 条件渲染；Settings 用 usePerm 过滤 items 数组（安全/系统信息恒在）；Network 的 ZT 授权 Switch 用禁用态兜底、除名/TS 授权/TS 删除按钮包 PermGuard | 已完成 |
   102	
   103	两个实现决策（设计阶段未写死，落地时定）：
   104	
   105	- **内置角色 seed 覆盖策略**：已存在的 builtin 角色 permissions 以代码为准覆盖更新
   106	  （内置角色用户不可改——D12，因此版本升级调整内置清单必须能生效）；自定义角色不动。
   107	  稳态（角色已一致、无存量 user）零写——只读库 reopen（server 测试的 DB 错误注入
   108	  手法）不触发写。
   109	- **自定义角色删除**：被任何用户引用时拒绝删除（软引用删除会让这些用户 fail-closed
   110	  全拒，等价于锁号）。
   111	- **D13 用户侧分支的可达性**：到达用户管理 handler 的操作者持 `users:admin`、自身即
   112	  有效 admin，「删/降最后一个有效 admin」排除目标后至少剩操作者——生产不可达。该分支
   113	  保留为判定层变化时的防御纵深（测试直调覆盖）。角色侧（削自定义角色的 `users:admin`）
   114	  真实可达：`roles:admin` 操作者可以不持 `users:admin`。
   115	
   116	笔 2 追加的实现决策：
   117	
   118	- **判定层形态**：全局 `RBACMiddleware`，挂在 AuditMiddleware 内层（403 也进
   119	  审计链）、各 auth 挂点外层，不逐 handler 包 requirePermission。自身解析 Bearer
   120	  取角色；无/坏 token 放行给内层 auth 出 401（认证优先于鉴权）。一处挂载，
   121	  serveAPI 与十个独立注册点（docker/stacks/backups/probe/proxies/metrics/
   122	  audit/remote/desktop/vnc）零改动，`/ws`（agent 通道，非用户请求）不在 `/api/`
   123	  前缀自动豁免。
   124	- **子路径归并**：probe/notification → `alerts`（拨测配置含告警阈值）、metrics →
   125	  `inventory`、proxies → `proxy`、server-backups → `backup`、remote/desktop/vnc
   126	  → `terminal`。
   127	- **固定 action**：audit/logs 的动作全是读（导出/检索也是 POST，恒映射 `:read`）；
   128	  terminal 只有 write（GET 列表同样按 `terminal:write` 门禁，D11 语义）。
   129	- **acme 签发/部署**（POST `.../issue`、`.../deploy`）恒 `acme:admin`。
   130	- **未登记 /api/ 路径放行**（handler 404 兜底）——缺省拒绝与缺省 404 无安全差，
   131	  新端点忘登记不会把 404 变 403 暴露路径存在性。
   132	
   133	## 不做的事
   134	
   135	## 不做的事
   136	
   137	- 不引入 casban/ory 类权限框架——依赖重量与本项目体量不匹配
   138	- 不做行级权限（某个 agent 只对某部门可见）——现有部署是个人/小团队自托管，
   139	  行级隔离等真实需求出现再议
   140	- 不改 agent 协议——权限是 server 职责，agent 保持「被调用即执行」的信任模型
[0m$ [0mnl -ba internal/server/api_stack_webhook.go | sed -n '1,110p'; nl -ba internal/server/api_stacks.go | sed -n '1,250p'; nl -ba internal/server/api.go | sed -n '1,130p'; nl -ba internal/server/web.go | sed -n '1,180p'
1	package server
     2	
     3	import (
     4		"crypto/subtle"
     5		"encoding/json"
     6		"fmt"
     7		"net/http"
     8	
     9		"github.com/cuihairu/cockpit/internal/audit"
    10		"github.com/cuihairu/cockpit/internal/protocol"
    11	)
    12	
    13	// stacks webhook 触发部署（stack-deploy-design.md P2 ②）：
    14	//
    15	//	POST /api/stacks/webhook   头 X-Cockpit-Token == 配置 stacks.webhook_token
    16	//
    17	// git 仓库推送后由 forge 回调，无法走登录态——独立精确路由（先于
    18	// /api/stacks/ 认证前缀注册，ServeMux 最长匹配胜出）。令牌空 = 端点关闭
    19	// （默认关，显式配置开启，同 notification 红线口径）。webhook 只触发
    20	// 「拉取已接入 origin + 部署改动 stack」，不接受 remote 参数（不重定向 origin）。
    21	// body 可选 {"agentId":"...","branch":"..."}：指定单 agent，缺省扇出全部在线
    22	// agent（未接入仓库的 agent RPC 报 remote required → 失败仅落日志）。
    23	// 执行异步：响应即返回受理结果，部署结果看部署历史与审计（username=webhook）。
    24	
    25	// handleStackWebhook webhook 入口（不走认证中间件，token 即凭据）
    26	func (s *Server) handleStackWebhook(w http.ResponseWriter, r *http.Request) {
    27		if r.Method != http.MethodPost {
    28			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
    29			return
    30		}
    31		token := ""
    32		if s.cfg != nil && s.cfg.Stacks != nil {
    33			token = s.cfg.Stacks.WebhookToken
    34		}
    35		if token == "" {
    36			s.handleError(w, r, http.StatusNotFound, "stack webhook disabled")
    37			return
    38		}
    39		got := r.Header.Get("X-Cockpit-Token")
    40		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
    41			fmt.Printf("stack webhook rejected: bad token from %s\n", s.getClientIP(r))
    42			s.handleError(w, r, http.StatusUnauthorized, "invalid token")
    43			return
    44		}
    45	
    46		// body 可选：空 body = 全量扇出 + 当前分支
    47		params := map[string]interface{}{}
    48		if r.Body != nil {
    49			var body map[string]interface{}
    50			if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body != nil {
    51				params = body
    52			}
    53		}
    54		// remote 一律不接受：origin 以 agent 侧 .git/config 为准，
    55		// webhook 不重定向拉取来源（防注入内容改写部署源）
    56		delete(params, "remote")
    57	
    58		rpcParams := map[string]interface{}{}
    59		if b := stackString(params, "branch"); b != "" {
    60			rpcParams["branch"] = b
    61		}
    62	
    63		var targets []string
    64		if agentID := stackString(params, "agentId"); agentID != "" {
    65			if _, ok := s.registry.Get(agentID); !ok {
    66				s.handleError(w, r, http.StatusNotFound, "Agent not found")
    67				return
    68			}
    69			targets = []string{agentID}
    70		} else {
    71			for _, a := range s.registry.List() {
    72				if a.IsOnline(s.registry.timeouts.Heartbeat) {
    73					targets = append(targets, a.ID)
    74				}
    75			}
    76		}
    77	
    78		triggered := make([]string, 0, len(targets))
    79		for _, agentID := range targets {
    80			triggered = append(triggered, agentID)
    81			s.goBg(func() { s.runStackWebhookDeploy(agentID, rpcParams) })
    82		}
    83		s.writeJSON(w, http.StatusOK, map[string]interface{}{"triggered": triggered})
    84	}
    85	
    86	// runStackWebhookDeploy 单 agent 的异步拉取+部署：结果落审计与部署历史
    87	func (s *Server) runStackWebhookDeploy(agentID string, params map[string]interface{}) {
    88		resp, err := s.CallAgent(agentID, "stack.git.deploy", params)
    89		if err != nil {
    90			fmt.Printf("stack webhook deploy %s: %v\n", agentID, err)
    91			return
    92		}
    93		rpcResp, err := protocol.DecodeRPCResponse(resp)
    94		if err != nil || rpcResp.Status == "error" {
    95			msg := "invalid agent response"
    96			if err == nil {
    97				msg = rpcResp.Error
    98			}
    99			fmt.Printf("stack webhook deploy %s: %s\n", agentID, msg)
   100			return
   101		}
   102		data, _ := rpcResp.Data.(map[string]interface{})
   103		s.audit.LogResource("webhook", audit.ActionStackWebhookDeploy, audit.ResourceStack, agentID,
   104			map[string]interface{}{"agentId": agentID, "result": data}, "", "")
   105		for _, e := range stackDeployedEntries(data) {
   106			s.startStackDeployment(agentID, e.name, audit.ActionStackWebhookDeploy, e.taskID)
   107		}
   108	}
   109	
   110	// stackDeployedEntry result.deployed 里的一条 (stack, taskId)
     1	package server
     2	
     3	import (
     4		"fmt"
     5		"log"
     6		"net/http"
     7		"strconv"
     8		"strings"
     9		"sync"
    10		"time"
    11	
    12		"github.com/cuihairu/cockpit/internal/audit"
    13		"github.com/cuihairu/cockpit/internal/auth"
    14		"github.com/cuihairu/cockpit/internal/protocol"
    15		"github.com/cuihairu/cockpit/internal/storage"
    16	)
    17	
    18	// Compose Stack API：REST 门面转发 Agent RPC（设计见 docs/guide/stack-deploy-design.md）。
    19	//
    20	//	GET    /api/stacks                                     聚合所有 agent 的 stack 列表（含离线缓存）
    21	//	GET    /api/stacks/agents/{agentId}                    单 agent 的 stack 列表
    22	//	GET    /api/stacks/agents/{agentId}/{name}             stack 状态明细
    23	//	GET    /api/stacks/agents/{agentId}/{name}/compose     读取 compose/.env
    24	//	PUT    /api/stacks/agents/{agentId}/{name}/compose     保存 compose/.env（agent 侧先校验）
    25	//	POST   /api/stacks/agents/{agentId}/{name}/up|down     部署/停止（返回 taskId，异步）
    26	//	GET    /api/stacks/agents/{agentId}/{name}/logs        compose logs
    27	//	GET    /api/stacks/agents/{agentId}/tasks/{taskId}     任务状态轮询
    28	//	DELETE /api/stacks/agents/{agentId}/{name}             删除 stack（异步：先 down 再删目录）
    29	const stacksAPIPrefix = "/api/stacks/"
    30	
    31	// stackTasksSegment 保留路由段：stack 名正则不匹配它，但解析时仍需先于 {name} 判断
    32	const (
    33		stackTasksSegment      = "tasks"
    34		stackTaskFollowSegment = "follow" // POST /tasks/{taskId}/follow（P2 ② 流式日志）
    35	)
    36	
    37	// stackGitSegment git 保留路由段（P2 ① Git-to-deploy）："git" 本身满足
    38	// stack 名正则，同样必须先于 {name} 判断。
    39	//
    40	//	GET  /api/stacks/agents/{agentId}/git          stacks 仓接入状态 + drift
    41	//	POST /api/stacks/agents/{agentId}/git/pull     拉取远端（首次=接入，body 可带 remote/branch）
    42	const stackGitSegment = "git"
    43	
    44	// stackAggregateTimeout 聚合视图的整体等待预算（单 agent 的 CallAgent 内部超时 30s）
    45	const stackAggregateTimeout = 10 * time.Second
    46	
    47	func (s *Server) registerStacksAPI(mux *http.ServeMux) {
    48		mux.HandleFunc(stacksAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
    49			s.authService().Middleware(s.handleStacks)(w, r)
    50		})
    51		// webhook：forge 回调无登录态，精确路由独立注册（ServeMux 最长匹配
    52		// 胜出，先于 /api/stacks/ 认证前缀），token 即凭据（P2 ②）
    53		mux.HandleFunc("POST /api/stacks/webhook", s.handleStackWebhook)
    54	}
    55	
    56	func (s *Server) handleStacks(w http.ResponseWriter, r *http.Request) {
    57		segments := splitStacksPath(r.URL.Path)
    58		if len(segments) == 0 {
    59			if r.Method == http.MethodGet {
    60				s.handleStacksAll(w, r)
    61				return
    62			}
    63			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
    64			return
    65		}
    66		if segments[0] != "agents" || len(segments) < 2 {
    67			s.handleError(w, r, http.StatusBadRequest, "expected /api/stacks/agents/{agentId}/...")
    68			return
    69		}
    70	
    71		agentID, err := pathSegment(segments[1])
    72		if err != nil || agentID == "" {
    73			s.handleError(w, r, http.StatusBadRequest, "invalid agent id")
    74			return
    75		}
    76	
    77		// /api/stacks/agents/{agentId}
    78		if len(segments) == 2 {
    79			if r.Method != http.MethodGet {
    80				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
    81				return
    82			}
    83			s.handleAgentStacks(w, r, agentID)
    84			return
    85		}
    86	
    87		// /api/stacks/agents/{agentId}/tasks/{taskId}[/follow]
    88		if segments[2] == stackTasksSegment && (len(segments) == 4 ||
    89			(len(segments) == 5 && segments[4] == stackTaskFollowSegment)) {
    90			if len(segments) == 5 {
    91				// 部署日志流式推送（P2 ②）：NDJSON，替代任务运行期轮询
    92				if r.Method != http.MethodPost {
    93					s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
    94					return
    95				}
    96				taskID, err := pathSegment(segments[3])
    97				if err != nil || taskID == "" {
    98					s.handleError(w, r, http.StatusBadRequest, "invalid task id")
    99					return
   100				}
   101				s.handleStackTaskFollow(w, r, agentID, taskID)
   102				return
   103			}
   104			if r.Method != http.MethodGet {
   105				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
   106				return
   107			}
   108			taskID, err := pathSegment(segments[3])
   109			if err != nil || taskID == "" {
   110				s.handleError(w, r, http.StatusBadRequest, "invalid task id")
   111				return
   112			}
   113			s.handleStackTask(w, r, agentID, taskID)
   114			return
   115		}
   116	
   117		// /api/stacks/agents/{agentId}/git[/pull]（stacks 根目录级，与单个 stack 无关）
   118		if segments[2] == stackGitSegment {
   119			switch {
   120			case len(segments) == 3 && r.Method == http.MethodGet:
   121				s.handleStackRPC(w, r, agentID, "stack.git.status", map[string]interface{}{}, "")
   122			case len(segments) == 4 && segments[3] == "pull" && r.Method == http.MethodPost:
   123				params, err := requestParams(r)
   124				if err != nil {
   125					s.handleError(w, r, http.StatusBadRequest, err.Error())
   126					return
   127				}
   128				s.handleStackRPC(w, r, agentID, "stack.git.pull", params, audit.ActionStackGitPull)
   129			default:
   130				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
   131			}
   132			return
   133		}
   134	
   135		name, err := pathSegment(segments[2])
   136		if err != nil || name == "" {
   137			s.handleError(w, r, http.StatusBadRequest, "invalid stack name")
   138			return
   139		}
   140	
   141		if len(segments) == 3 {
   142			switch r.Method {
   143			case http.MethodGet:
   144				s.handleStackRPC(w, r, agentID, "stack.status", map[string]interface{}{"name": name}, "")
   145			case http.MethodDelete:
   146				s.handleStackRPC(w, r, agentID, "stack.remove", map[string]interface{}{"name": name}, audit.ActionStackRemove)
   147			default:
   148				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
   149			}
   150			return
   151		}
   152		if len(segments) != 4 {
   153			s.handleError(w, r, http.StatusBadRequest, "unsupported stacks route")
   154			return
   155		}
   156	
   157		switch segments[3] {
   158		case "compose":
   159			if r.Method == http.MethodGet {
   160				s.handleStackRPC(w, r, agentID, "stack.file.get", map[string]interface{}{"name": name}, "")
   161			} else if r.Method == http.MethodPut {
   162				params, err := requestParams(r)
   163				if err != nil {
   164					s.handleError(w, r, http.StatusBadRequest, err.Error())
   165					return
   166				}
   167				params["name"] = name
   168				// 记 stack_update；agent 返回 created=true 时由 auditStackAction 升级为 stack_create
   169				s.handleStackRPC(w, r, agentID, "stack.file.save", params, audit.ActionStackUpdate)
   170			} else {
   171				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
   172			}
   173		case "up":
   174			if r.Method == http.MethodPost {
   175				s.handleStackRPC(w, r, agentID, "stack.up", map[string]interface{}{"name": name}, audit.ActionStackUp)
   176			} else {
   177				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
   178			}
   179		case "down":
   180			if r.Method == http.MethodPost {
   181				s.handleStackRPC(w, r, agentID, "stack.down", map[string]interface{}{"name": name}, audit.ActionStackDown)
   182			} else {
   183				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
   184			}
   185		case "restart":
   186			if r.Method == http.MethodPost {
   187				s.handleStackRPC(w, r, agentID, "stack.restart", map[string]interface{}{"name": name}, audit.ActionStackRestart)
   188			} else {
   189				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
   190			}
   191		case "pull":
   192			if r.Method == http.MethodPost {
   193				s.handleStackRPC(w, r, agentID, "stack.pull", map[string]interface{}{"name": name}, audit.ActionStackPull)
   194			} else {
   195				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
   196			}
   197		case "history":
   198			if r.Method == http.MethodGet {
   199				s.handleStackHistory(w, r, agentID, name)
   200			} else {
   201				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
   202			}
   203		case "logs":
   204			if r.Method == http.MethodGet {
   205				params := map[string]interface{}{"name": name}
   206				setQueryString(params, r, "service")
   207				setQueryString(params, r, "tail")
   208				s.handleStackRPC(w, r, agentID, "stack.logs", params, "")
   209			} else {
   210				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
   211			}
   212		default:
   213			s.handleError(w, r, http.StatusBadRequest, "unsupported stack action")
   214		}
   215	}
   216	
   217	// requireStackAgent 校验 agent 存在且具备 docker 能力
   218	func (s *Server) requireStackAgent(agentID string) bool {
   219		agent, ok := s.registry.Get(agentID)
   220		if !ok {
   221			return false
   222		}
   223		return agent.HasCapability("docker-api") || agent.HasCapability("docker")
   224	}
   225	
   226	// handleStackRPC 转发 RPC 并按 stack 语义映射错误码；
   227	// auditAction 非空时记录审计（.env 内容与部署输出不入审计）。
   228	func (s *Server) handleStackRPC(w http.ResponseWriter, r *http.Request, agentID, method string, params map[string]interface{}, auditAction string) {
   229		if !s.requireStackAgent(agentID) {
   230			s.handleError(w, r, http.StatusNotFound, "Agent not found or docker capability not available")
   231			return
   232		}
   233	
   234		resp, err := s.CallAgent(agentID, method, params)
   235		if err != nil {
   236			status := http.StatusBadGateway
   237			if err == ErrAgentNotFound {
   238				status = http.StatusNotFound
   239			} else if strings.Contains(err.Error(), "busy") {
   240				status = http.StatusConflict
   241			} else if strings.Contains(err.Error(), "conflict") {
   242				// git pull 遇本地改动/分叉拒绝合并（--ff-only），前端按 409 引导先处理 drift
   243				status = http.StatusConflict
   244			} else if strings.Contains(err.Error(), "not found") {
   245				status = http.StatusNotFound
   246			} else if strings.Contains(err.Error(), "timeout") {
   247				status = http.StatusGatewayTimeout
   248			}
   249			s.handleError(w, r, status, err.Error())
   250			return
     1	package server
     2	
     3	import (
     4		"encoding/json"
     5		"errors"
     6		"io"
     7		"net/http"
     8		"strings"
     9		"time"
    10	
    11		"github.com/cuihairu/cockpit/internal/auth"
    12		"github.com/cuihairu/cockpit/internal/storage"
    13	)
    14	
    15	// serveAPI 处理 API 请求
    16	func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
    17		// 设置 CORS
    18		setCORSHeaders(w)
    19	
    20		// 处理 OPTIONS 预检请求
    21		if r.Method == http.MethodOptions {
    22			w.WriteHeader(http.StatusOK)
    23			return
    24		}
    25	
    26		// 解析路径（归一化尾斜杠，/api/agents/ → /api/agents）
    27		path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api"), "/")
    28		if path == "" {
    29			path = "/"
    30		}
    31	
    32		// 路由分发
    33		switch {
    34		case path == "/status":
    35			s.handleStatus(w, r)
    36		case path == "/me":
    37			s.handleCurrentUser(w, r)
    38		case path == "/me/profile":
    39			s.handleCurrentUserProfile(w, r)
    40		case path == "/me/password":
    41			s.handleCurrentUserPassword(w, r)
    42		case path == "/settings":
    43			s.handleSettings(w, r)
    44		case path == "/agents":
    45			s.handleAgentsList(w, r)
    46		case path == "/agents/cleanup":
    47			s.handleAgentsCleanup(w, r)
    48		case path == "/drift/config":
    49			s.handleDriftConfig(w, r)
    50		case path == "/jobs" || strings.HasPrefix(path, "/jobs/"):
    51			// 执行 Job（见 api_jobs.go，jobs-design.md）
    52			s.handleJobsAPI(w, r, path)
    53		case path == "/workflows" || strings.HasPrefix(path, "/workflows/") ||
    54			path == "/workflow-runs" || strings.HasPrefix(path, "/workflow-runs/"):
    55			// Workflow 编排（见 api_workflow.go，workflow-design.md）
    56			s.handleWorkflowAPI(w, r, path)
    57		case path == "/approvals" || strings.HasPrefix(path, "/approvals/"):
    58			// 审批门（见 api_approvals.go，workflow-design.md M2d）
    59			s.handleApprovalsAPI(w, r, path)
    60		case path == "/inventory/consistency":
    61			// CMDB 一致性（见 api_inventory.go，drift-design.md M6 D28/D30）
    62			s.handleInventoryConsistency(w, r)
    63		case path == "/smart/config":
    64			s.handleSmartConfig(w, r)
    65		case path == "/nas/config":
    66			s.handleNASConfig(w, r)
    67		case path == "/agent-tags" || strings.HasPrefix(path, "/agent-tags/"):
    68			s.handleAgentTagsAPI(w, r)
    69		case path == "/recordings":
    70			s.handleRecordings(w, r)
    71		case strings.HasPrefix(path, "/recordings/"):
    72			s.handleRecordings(w, r)
    73		case path == "/server-backups" || strings.HasPrefix(path, "/server-backups/"):
    74			s.handleServerBackups(w, r)
    75		case path == "/dns" || strings.HasPrefix(path, "/dns/"):
    76			s.handleDNS(w, r)
    77		case path == "/domains" || strings.HasPrefix(path, "/domains/"):
    78			// 服务域名绑定（见 api_domain_bindings.go，domain-binding-design.md）
    79			s.handleDomainBindings(w, r)
    80		case path == "/overlay/cloud" || strings.HasPrefix(path, "/overlay/cloud/"):
    81			// 组网云管理面（见 api_overlay_cloud.go，M2 D11-D19）
    82			s.handleOverlayCloud(w, r)
    83		case path == "/ddns" || strings.HasPrefix(path, "/ddns/"):
    84			s.handleDDNS(w, r)
    85		case path == "/acme" || strings.HasPrefix(path, "/acme/"):
    86			s.handleACME(w, r)
    87		case path == "/logs/search":
    88			// 跨机日志联邦检索（全局端点，见 api_logs_search.go M3/D11）
    89			s.handleLogsSearch(w, r)
    90		case strings.HasPrefix(path, "/agents/"):
    91			agentID := strings.TrimPrefix(path, "/agents/")
    92			// agent 标签 GET/PUT /agents/{id}/tags（见 api_tags.go）
    93			if strings.HasSuffix(agentID, "/tags") {
    94				s.handleAgentTagAssignAPI(w, r, agentID)
    95				return
    96			}
    97			// 远程文件管理 /agents/{id}/files/...（见 api_files.go）
    98			if strings.Contains(agentID, "/files/") {
    99				s.handleAgentFilesAPI(w, r, agentID)
   100				return
   101			}
   102			// 反向代理管理 /agents/{id}/proxy/...（见 api_proxy_sites.go）
   103			if strings.Contains(agentID, "/proxy/") {
   104				s.handleAgentProxyAPI(w, r, agentID)
   105				return
   106			}
   107			// 定时任务管理 /agents/{id}/cron/...（见 api_cron.go）
   108			if strings.Contains(agentID, "/cron/") {
   109				s.handleAgentCronAPI(w, r, agentID)
   110				return
   111			}
   112			// systemd 服务管理 /agents/{id}/services/...（见 api_service.go；
   113			// 裸 GET /agents/{id}/services 是服务列表端点，HasSuffix 兜底——
   114			// 真机验收逮到：仅 Contains "/services/" 时裸列表掉进 handleAgentGet
   115			// 报 404 Agent not found，Web 服务页列表整体不可达）
   116			if strings.HasSuffix(agentID, "/services") || strings.Contains(agentID, "/services/") {
   117				s.handleAgentServiceAPI(w, r, agentID)
   118				return
   119			}
   120			// 服务健康探针 /agents/{id}/health[/probes/{id}/check]（见 api_service_health.go）
   121			if strings.HasSuffix(agentID, "/health") || strings.Contains(agentID, "/health/") {
   122				s.handleAgentHealthAPI(w, r, agentID)
   123				return
   124			}
   125			// NAS 存储观测 /agents/{id}/nas/...（见 api_nas.go）
   126			if strings.Contains(agentID, "/nas/") {
   127				s.handleAgentNASAPI(w, r, agentID)
   128				return
   129			}
   130			// 远程日志查询 /agents/{id}/logs/...（见 api_logs.go）
     1	package server
     2	
     3	import (
     4		"encoding/json"
     5		"net/http"
     6		"os"
     7		"path/filepath"
     8		"strings"
     9	)
    10	
    11	// spaHandler 返回静态文件 handler，未配置 StaticDir 时返回 API 提示
    12	func (s *Server) spaHandler() http.Handler {
    13		staticDir := s.staticDir()
    14		if staticDir == "" {
    15			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    16				w.Header().Set("Content-Type", "application/json")
    17				w.WriteHeader(http.StatusOK)
    18				json.NewEncoder(w).Encode(map[string]string{
    19					"message": "Cockpit API server is running. Set STATIC_DIR to serve the web UI.",
    20				})
    21			})
    22		}
    23	
    24		fileServer := http.FileServer(http.Dir(staticDir))
    25		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    26			path := r.URL.Path
    27			// 跳过 API 和 WebSocket 路径
    28			if strings.HasPrefix(path, "/api/") || path == "/ws" {
    29				http.NotFound(w, r)
    30				return
    31			}
    32	
    33			// 清理路径，防止目录遍历
    34			cleanPath := filepath.Clean(strings.TrimPrefix(path, "/"))
    35			if cleanPath == "." || cleanPath == ".." {
    36				cleanPath = "index.html"
    37			}
    38	
    39			fsPath := filepath.Join(staticDir, cleanPath)
    40			if info, err := os.Stat(fsPath); err == nil && !info.IsDir() {
    41				// index.html 不缓存（SPA 入口，hash 变了旧页面就废了）
    42				if cleanPath == "index.html" {
    43					w.Header().Set("Cache-Control", "no-cache")
    44				}
    45				fileServer.ServeHTTP(w, r)
    46				return
    47			}
    48	
    49			// SPA fallback: 文件不存在则返回 index.html（不缓存）
    50			w.Header().Set("Cache-Control", "no-cache")
    51			r.URL.Path = "/"
    52			fileServer.ServeHTTP(w, r)
    53		})
    54	}
    55	
    56	// staticDir 获取静态文件目录，优先环境变量，其次配置文件
    57	func (s *Server) staticDir() string {
    58		if dir := os.Getenv("STATIC_DIR"); dir != "" {
    59			return dir
    60		}
    61		if s.cfg != nil && s.cfg.Server != nil && s.cfg.Server.StaticDir != "" {
    62			return s.cfg.Server.StaticDir
    63		}
    64		return ""
    65	}
[0m$ [0mrg -n "Ticket generated|ticket|ALLOWED_ORIGINS|Agent secret|secret is optional|rate limit|限流|频率限制" docs README* deployments --glob '*.md' --glob '*.yaml' --glob '*.yml' --glob '*.env*'; nl -ba internal/server/api_remote.go | sed -n '1,220p'; nl -ba internal/server/api_guacamole.go | sed -n '1,240p'; nl -ba internal/server/api.go | sed -n '500,780p'
key on reconnect.
docs/guide/architecture.en.md:162:- Remote connection tickets are in-memory, 5-minute valid, single-use; they expire after a Server restart.
docs/guide/decision-log.en.md:286:`Register(pending)` after ticket validation; once proxy_new has been delivered successfully it calls
docs/guide/decision-log.en.md:315:point has to be added (currently `terminal:write` covers only ticket issuance); if growing guacd
docs/guide/getting-started.en.md:219:export ALLOWED_ORIGINS="https://cockpit.cuihairu.site"
docs/guide/concepts.en.md:198:The remote-connection Browser WebSocket uses short-lived ticket authentication. See [Protocols and API Boundary](/guide/protocol.en.md) for details.
docs/guide/concepts.en.md:208:| WebSocket Origin | Controlled by `ALLOWED_ORIGINS`; unset is fine for development but not for production |
docs/guide/concepts.en.md:209:| Remote connections | 5-minute, single-use ticket |
docs/index.en.md:28:    details: Admin bootstrap, JWT, TOTP, password reset, audit log and per-Agent secret
docs/index.en.md:31:    details: Terminal, VNC and desktop sessions use short-lived tickets, forwarded through Server and Agent to the target service
docs/remote-access-integration-design.en.md:47:session/ticket/audit/egress policy stays in the Go backend (the boundary from old design
docs/remote-access-integration-design.en.md:56:| Standalone terminal gateways such as **WebSSH / sshwifty / ttyd** | SSH only | One more service to deploy and one more account system; tickets/audit/egress policy would need a second integration pass. guacd is already in the deployment. |
docs/remote-access-integration-design.en.md:66:   │  WSS /api/remote/guacamole?ticket=…                     │                    │
docs/remote-access-integration-design.en.md:82:| ticket parameter | guacd connect parameter | Description |
docs/remote-access-integration-design.en.md:94:1. **Tickets**: the `private_key` field of `POST /api/remote/tickets` has long existed
docs/remote-access-integration-design.en.md:95:   (used for agent-channel SSH private keys) and the Guacamole route reuses it directly; tickets remain one-shot, in-memory, valid for 5 minutes,
docs/guide/reference-projects.en.md:111:- Cockpit's short-lived ticket + `Sec-WebSocket-Protocol` authentication chain follows the same thinking; **the design is validated as correct**
docs/guide/reference-projects.en.md:168:| Teleport | Remote-control security | Short-lived tickets (validated) | Approval flows, session-granularity auditing | Enterprise-grade complexity |
docs/guide/protocol.en.md:12:| Agent control channel | Agent | `/ws` | First `register` message; optional Agent secret |
docs/guide/protocol.en.md:13:| Remote terminal | Browser | `/api/remote/terminal` | Short-lived ticket, passed via `Sec-WebSocket-Protocol` |
docs/guide/protocol.en.md:14:| Guacamole desktop (RDP/VNC/SSH) | Browser | `/api/remote/guacamole` | Short-lived ticket, passed via `Sec-WebSocket-Protocol` |
docs/guide/protocol.en.md:172:1. The browser calls `POST /api/remote/tickets` with the JWT.
docs/guide/protocol.en.md:173:2. The Server generates a 5-minute, single-use ticket.
docs/guide/protocol.en.md:174:3. The browser opens a WebSocket, putting the ticket as the first `Sec-WebSocket-Protocol` value.
docs/guide/protocol.en.md:175:4. The Server validates and consumes the ticket, then starts forwarding.
docs/guide/protocol.en.md:177:Creating a ticket:
docs/guide/protocol.en.md:180:POST /api/remote/tickets
docs/guide/protocol.en.md:198:  "ticket": "random-ticket-id",
docs/guide/protocol.en.md:206:new WebSocket("ws://server:9000/api/remote/terminal", [ticket])
docs/guide/protocol.en.md:248:- `POST /api/remote/tickets`
docs/guide/persona-vault-mapping.md:72:| 429 `rate_limited` | 显式限流错误（`rate limited (120 req/min per token)`；当前不自动重试，调用方按需退避） |
docs/guide/workflow-design.en.md:111:| W8 | Concurrency constraint | At most one active run per workflow at a time (rejected with 409); different workflows may run in parallel | At personal scale this prevents re-entrance (double-clicking "Run"); no global rate limiting |
docs/guide/introduction.md:15:远程操作方面，终端 / VNC / 桌面走短期 ticket 转发，会话录制归档可回放；agent 上能浏览与传输文件；日志既能在单机 tail（journalctl / Docker），也能跨机联邦检索。应用与容器由 Docker 全量管理（容器、镜像、网络、卷），Stacks 按 compose 部署，带模板库与部署历史，restart/pull 就近操作。
docs/guide/concepts.md:198:远程连接的 Browser WebSocket 使用短期 ticket 认证。详见 [协议与 API 边界](/guide/protocol)。
docs/guide/concepts.md:208:| WebSocket Origin | `ALLOWED_ORIGINS` 控制；未设置时适合开发但不适合生产 |
docs/guide/concepts.md:209:| 远程连接 | 5 分钟、单次使用 ticket |
docs/guide/introduction.en.md:15:For remote operations, terminal / VNC / desktop go through short-lived ticket forwarding, with session recording archived and replayable; files can be browsed and transferred on agents; logs can be tailed on a single machine (journalctl / Docker) and searched federated across machines. Applications and containers are fully managed by Docker (containers, images, networks, volumes); Stacks deploy via compose with a template library and deployment history; restart/pull are operated locally. 
docs/guide/probe-enhance-design.en.md:151:> The three leftovers flagged on the P1 probing ticket are cleared in one go: M1's duplicate alerts "not fixed
docs/guide/mobile-design.en.md:31:- **D7 Terminal / desktop deferred**: terminal in M2 with xterm.dart + the existing remote ticket WebSocket; VNC/desktop viewing has a weak ecosystem on the Flutter side (no noVNC equivalent), listed as an exploration item without commitment (closed as "won't do" on 2026-10-07; evaluation basis in the M3 section).
docs/guide/mobile-design.en.md:52:**M2** (completed 2026-09-22): resources page with three tabs (domain/certificate expiry, backup task status and manual trigger), Cron read-only view (external entries shown verbatim, write operations left to desktop), file browsing (read-only: directory navigation + entry details), SSH terminal (tickets + WS subprotocol + xterm.dart), biometric lock (local_auth startup gate).
docs/index.md:28:    details: 支持管理员初始化、JWT、TOTP、密码重置、审计日志和 Agent secret
docs/index.md:31:    details: 终端、VNC 和桌面连接使用短期 ticket，经 Server 和 Agent 转发到目标服务
docs/audit-doc-consistency.en.md:37:| A3 | remote-desktop-guacamole-design.md:83 | The current-endpoint examples include `/api/remote/desktop` | Route deleted (grdp deletion surgery D-2026-10-07-1; the surviving ones are terminal/guacamole/tickets/sessions/vault) | Doc ahead of code | P1 | Change to `/api/remote/guacamole`; sync the pipeline description to "agent WS / guacd TCP" |
docs/audit-doc-consistency.en.md:53:- ~150 endpoint claims all check out against the code, with only the A3/A6 differences; the protocol.md HTTP API section, ticket 5-minute single-use (ticket.go:58), the six public auth endpoints, and the six resource types all match
docs/audit-doc-consistency.en.md:117:| D8 | docs/websocket-token-auth.md:1 | Tickets are uniformly passed via `Sec-WebSocket-Protocol` | terminal = subprotocol only; guacamole = `?ticket=` query takes priority, subprotocol is the fallback | Description mismatch | P2 | Change to the dual-channel semantics |
docs/audit-doc-consistency.en.md:145:| D31 | docs/guide/persona-vault-mapping.md:67 | 429 has no detailed documentation | `rate limited (120 req/min per token)`, no automatic retry | Code ahead of doc | P2 | Add the documentation |
docs/audit-doc-consistency.en.md:158:| D44 | docs/operations/deploy-docker.md:84 | `ALLOWED_ORIGINS` is required | When unset, the WS accepts any Origin (logging a warning), and startup is not blocked | Description mismatch | P2 | Change to optional |
docs/audit-doc-consistency.en.md:228:| D90 | docs/operations/deploy-docker.md | Port mapping/tag/ALLOWED_ORIGINS/nginx template | Description mismatch | P2 |
docs/websocket-token-auth.md:5:当前实现已经避免在远程连接 WebSocket URL 中直接携带 JWT。远程终端、VNC、桌面连接统一使用短期 ticket：terminal 入口仅经 `Sec-WebSocket-Protocol` 传递，guacamole 入口 URL query `?ticket=` 与子协议双通道（query 优先、子协议兜底）。
docs/websocket-token-auth.md:9:1. Web UI 使用 JWT 调用 `POST /api/remote/tickets`。
docs/websocket-token-auth.md:10:2. Server 生成内存态 ticket：
docs/websocket-token-auth.md:14:3. Web UI 建立 WebSocket 连接，并把 ticket 作为子协议值。
docs/websocket-token-auth.md:15:4. Server 校验并消费 ticket，随后才升级连接并转发数据。
docs/websocket-token-auth.md:18:const ticket = await createRemoteTicket(params)
docs/websocket-token-auth.md:19:const ws = new WebSocket('/api/remote/terminal', [ticket])
docs/websocket-token-auth.md:27:terminal 入口从 `Sec-WebSocket-Protocol` 读取 ticket；guacamole 入口先读 URL query `?ticket=`、缺失再回退子协议。两者都不把认证 token 放进通用审计。
docs/websocket-token-auth.md:31:- JWT 只用于创建 ticket 的 HTTP API，不直接进入远程 WebSocket URL。
docs/websocket-token-auth.md:32:- ticket 存在 Server 内存中，Server 重启后失效。
docs/websocket-token-auth.md:33:- ticket 被成功验证后会立即标记为已消费，不能复用。
docs/websocket-token-auth.md:34:- ticket 参数中可以包含 `agent_id`、`host`、`port`、`protocol`、`username`、`password`、`domain`、`width`、`height` 等连接参数。
docs/websocket-token-auth.md:40:- 设置 `ALLOWED_ORIGINS`，限制 WebSocket Origin。
docs/websocket-token-auth.md:42:- 避免在日志中打印 ticket 或远程连接密码。
docs/websocket-token-auth.md:47:早期文档讨论过 URL query token、短期连接 token、Cookie 认证等方案。当前代码采用“短期 ticket + WebSocket 子协议”的组合方式，是本文档应遵循的实现基线。
docs/remote-access-integration-design.md:60:   │  WSS /api/remote/guacamole?ticket=…                     │                    │
docs/remote-access-integration-design.md:80:| ticket 参数 | guacd connect 参数 | 说明 |
docs/remote-access-integration-design.md:92:1. **票据**：`POST /api/remote/tickets` 的 `private_key` 字段早已存在
docs/design/external-session-api.md:43:  "reason": "ops ticket #1042"
docs/design/external-session-api.md:65:    "ticket": "…",
docs/design/external-session-api.md:66:    "ws_url": "wss://<server>/api/remote/guacamole?ticket=…",
docs/design/external-session-api.md:67:    "ticket_ttl_seconds": 300
docs/design/external-session-api.md:72:- `ticket` 为既有一次性票据（5 分钟、内存态、用后即焚，
docs/design/external-session-api.md:73:  `internal/server/ticket.go`；`?ticket=` query 即
docs/design/external-session-api.md:79:  `protocols[0]` 即 ticketID，与 guacamole 通道的 `?ticket=` query 不同）。
docs/design/external-session-api.md:151:- 契约不承载终端内容：memex 拿 `ticket` + `ws_url` 后由其客户端直连 WS，
docs/design/external-session-api.md:169:（`:642-669`）、票据（`internal/server/ticket.go`）、guacamole WS
docs/remote-desktop-guacamole-design.en.md:33:- **guacamole-web**: Java servlet + JDBC, responsible for "who may connect to which machine." **Cockpit already has this layer**: RBAC permission points (`terminal:write` etc.) + the `matchRemoteEgress` egress policy + dynamic inventory (the agent-reported `remote-services` capability) + one-time tickets (`internal/server/ticket.go`). Bringing it in means bringing in a second system, and its "static connection config" model is the opposite of Cockpit's dynamic inventory philosophy (see the Guacamole downsides note in `reference-projects.md`).
docs/remote-desktop-guacamole-design.en.md:43:The reasons to put it in Go rather than in guacamole-web are the same as D1: what the gateway does here is more than byte forwarding — there is also **session lifecycle, ticket validation, permission checks, audit persistence** — all capabilities Cockpit already has. guacamole-web's `GuacamoleWebSocketTunnelEndpoint` does the same thing, but with Java sessions + its permission tables, which lands back in two systems.
docs/remote-desktop-guacamole-design.en.md:47:### D3 Reuse the existing ticket and 401 recursive-refresh chain
docs/remote-desktop-guacamole-design.en.md:49:Existing ticket semantics (`internal/server/ticket.go`): `POST /api/remote/tickets` → one-time ticket, valid for 5 minutes, `ValidateTicket` consumes on use (`Consumed` flag). This was designed as the "WS handshake credential" — invalidated after one handshake, with a 5-minute leak window.
docs/remote-desktop-guacamole-design.en.md:51:The Guacamole tunnel **reuses the same ticket-issuing endpoint and auditing**; the ticket remains the WS handshake credential (one-time semantics unchanged), and the only difference is that "after a successful handshake the server side holds the guacd TCP connection until the session ends" rather than "forwarding to the agent." This gives the frontend a consistent experience: `createRemoteTicket` → get ticket → connect WS; and gives backend auditing consistency: `auditRemoteStart` records `Protocol: "rdp"`/`"vnc"`, and at session end `auditRemoteEnd` records duration.
docs/remote-desktop-guacamole-design.en.md:53:**Why the ticket stays one-time**: although the Guacamole tunnel is a long-lived connection, the act of "establishing the tunnel" is itself a one-time handshake. If the connection drops and a reconnect is needed, apply for a new ticket — consistent with the existing terminal/desktop reconnect behavior (the "reconnect" button in `TerminalModal` just calls `createRemoteTicket` again). Making the ticket a session-level long-lived credential would only widen the leak window, for zero benefit.
docs/remote-desktop-guacamole-design.en.md:55:**Ticket transport over a dual channel** (alignment verified against common-js during implementation): guacamole-common-js's `WebSocketTunnel.connect(data)` is `new WebSocket(url + "?" + data, "guacamole")` — the **subprotocol is hard-coded as `"guacamole"`**, and `data` is appended into the URL query, so there is no way to carry the ticket via a custom subprotocol. Hence the gateway accepts the ticket on both paths:
docs/remote-desktop-guacamole-design.en.md:57:- URL query `?ticket=` (**primary**): the frontend's `client.connect("ticket=" + ticket)` lands it in the query; the ticket is still a one-time 5-minute ticket, leak window unchanged;
docs/remote-desktop-guacamole-design.en.md:60:The query-form ticket does not enter audit/logs (WS paths are skipped by `AuditMiddleware` and covered by the dedicated remote-access audit; `auditRemoteStart` records only ticket.ID, not the URL).
docs/remote-desktop-guacamole-design.en.md:67:- The Guacamole tunnel's ticket application goes through the same `remoteClient` (which already has the Bearer interceptor), with identical 401 behavior; the WS handshake itself carries no JWT (the ticket is the credential), so the WS path needs no refresh, and on reconnect `createRemoteTicket` naturally runs through the 401 refresh once.
docs/remote-desktop-guacamole-design.en.md:75:- **Go backend**: ticket issuance (including the `matchRemoteEgress` egress policy check), `terminal:write` permission point check, `auditRemoteStart/End/Failure` persistence, `RemoteSessionManager` session registry (pending/connected/closed/failed), session recording toggle and retention policy.
docs/remote-desktop-guacamole-design.en.md:160:// Responsibilities: ticket validation (at handshake) + egress policy + audit + bidirectional byte pipe.
docs/remote-desktop-guacamole-design.en.md:176:// The ticket goes over a dual channel: URL query ?ticket= (primary, since common-js hard-codes subprotocol
docs/remote-desktop-guacamole-design.en.md:182:	// 4. Upgrade WS (Subprotocols echo back the ticketID)
docs/remote-desktop-guacamole-design.en.md:185:	//    (credentials come from ticket.Params, same as handleTerminalWebSocket's proxy_new dispatch)
docs/remote-desktop-guacamole-design.en.md:208: * this component only handles "apply for ticket → establish tunnel → lifecycle".
docs/remote-desktop-guacamole-design.en.md:215:    // 1. Apply for a ticket (reuse createRemoteTicket; the 401 refresh chain is already built into remoteClient)
docs/remote-desktop-guacamole-design.en.md:216:    const { ticket } = await createRemoteTicket({
docs/remote-desktop-guacamole-design.en.md:222:    // 2. Tunnel: the ticket goes in the URL query (common-js's WebSocketTunnel hard-codes
docs/remote-desktop-guacamole-design.en.md:223:    // subprotocol "guacamole" and appends connect(data) to the URL query, so the ticket
docs/remote-desktop-guacamole-design.en.md:224:    // cannot go via Sec-WebSocket-Protocol — an upstream implementation constraint, see D3 "ticket dual channel")
docs/remote-desktop-guacamole-design.en.md:257:    // The data of connect(data) lands in the WS URL query; the gateway exchanges the ticket
docs/remote-desktop-guacamole-design.en.md:260:    client.connect(`ticket=${encodeURIComponent(ticket)}`)
docs/remote-desktop-guacamole-design.en.md:268:**Why parameters (credentials) are placed into the `connect` instruction by the server rather than the frontend**: guacamole-common-js's `client.connect(data)` can take a parameter string, and the server can also carry parameters in the `connect` instruction. **Choose the server** because the credentials are already in ticket.Params (stored by `handleTicketCreate`), so the frontend never needs to handle them — this removes one exposure of credentials in browser memory/on the network, and it is the same shape as `handleTerminalWebSocket`'s `proxy_new` credential dispatch (the frontend passes the ticket; the agent receives resolved credentials).
docs/remote-desktop-guacamole-design.en.md:330:- **Default path (unchanged)**: passwords/private keys go only into `ticket.Params` → the guacd `connect` instruction / the agent's SSH dialing; never written to the DB, never into audit — the existing discipline of D4 and `remote_audit.go`'s "do not write password" continues to hold;
docs/remote-desktop-guacamole-design.en.md:365:2. Go WS gateway reverse proxy (`internal/server/api_guacamole.go`) + ticket/audit/egress policy;
docs/remote-desktop-guacamole-design.en.md:382:- In-project: `docs/guide/recording-design.md` (terminal recording, desktop recording gap), `docs/guide/reference-projects.md` (Guacamole reference notes), `internal/server/ticket.go` (ticket semantics), `internal/server/remote_audit.go` (remote-access audit), `web/src/services/api.ts` (401 refresh)
docs/design/external-session-api.en.md:19:| Session initiation, tickets, egress policy, recording, audit | **This repo** (all reuse existing chains, zero new infrastructure) |
docs/design/external-session-api.en.md:41:  "reason": "ops ticket #1042"
docs/design/external-session-api.en.md:59:    "ticket": "…",
docs/design/external-session-api.en.md:60:    "ws_url": "wss://<server>/api/remote/guacamole?ticket=…",
docs/design/external-session-api.en.md:61:    "ticket_ttl_seconds": 300
docs/design/external-session-api.en.md:66:- `ticket` is the existing one-time ticket (5 minutes, in-memory, burn-after-use, `internal/server/ticket.go`; the `?ticket=` query is the current input to `handleGuacamoleWebSocket`, `internal/server/api_guacamole.go:308`).
docs/design/external-session-api.en.md:67:- `channel` selection per E6: `guacamole` (rdp/vnc/ssh) or `terminal` (`telnet` must take this path; when the memex client cannot speak the guacamole protocol, ssh can also downgrade to `terminal`, i.e. the same chain as D6 mobile — `wss://…/api/remote/terminal`, where the ticket is carried over the **WebSocket subprotocol** — `internal/server/api_remote.go:51-54` `protocols[0]` is the ticketID, which differs from the guacamole channel's `?ticket=` query).
docs/design/external-session-api.en.md:101:- The 5-minute one-time ticket is unchanged; a session is not terminated because memex disconnects (the connection belongs to the memex client, the session belongs to the ledger).
docs/design/external-session-api.en.md:121:- The contract does not carry terminal content: once memex has the `ticket` + `ws_url` its client connects to the WS directly, and this repo neither forwards nor multiplexes the data stream to memex.
docs/design/external-session-api.en.md:125:1. **No credential delivery**: no field of this contract accepts or returns credentials; `credential_ref` only names an entry, and at runtime the server injects the ticket (`use_saved` semantics, `internal/server/api_remote.go:484` onward). The memex side never obtains a password/key.
docs/design/external-session-api.en.md:127:3. No session proxying/multiplexing, no new protocols, and no memex-side UI embedding specification (the ticket is the embedding gate, and its 5-minute one-time nature is the security boundary).
docs/design/external-session-api.en.md:131:The `handleTicketCreate` request shape (`api_remote.go:399-411`), the egress check and vault injection order (`:460-500`: egress first, `use_saved` filled after), the session ledger (`:642-669`), the ticket (`internal/server/ticket.go`), the guacamole WS (`api_guacamole.go:306-308`), and the terminal WS (`api_remote.go:387`). New code is expected only in: the external route + caller auth middleware + `credential_ref` resolution + the ledger `initiator` field.
     1	package server
     2	
     3	import (
     4		"encoding/json"
     5		"log"
     6		"net/http"
     7		"strconv"
     8		"strings"
     9		"sync"
    10		"time"
    11	
    12		"github.com/cuihairu/cockpit/internal/audit"
    13		"github.com/cuihairu/cockpit/internal/auth"
    14		"github.com/cuihairu/cockpit/internal/protocol"
    15		"github.com/google/uuid"
    16		"github.com/gorilla/websocket"
    17	)
    18	
    19	// TerminalSession 终端会话
    20	type TerminalSession struct {
    21		ID         string
    22		UserID     string
    23		Username   string
    24		AgentID    string
    25		Protocol   protocol.RemoteProtocol
    26		Host       string
    27		Port       int
    28		ClientWS   *websocket.Conn
    29		ConnID     string
    30		CreatedAt  time.Time
    31		LastActive time.Time
    32		recorder   *castRecorder // 输出流录制（recording-design.md；nil = 未开）
    33		mu         sync.Mutex
    34		done       chan struct{}
    35	}
    36	
    37	var (
    38		terminalSessions   = make(map[string]*TerminalSession) // sessionID -> session
    39		terminalByConn     = make(map[string]*TerminalSession) // connID -> session
    40		terminalSessionsMu sync.Mutex
    41	)
    42	
    43	// handleTerminalWebSocket 处理终端 WebSocket 连接
    44	func (s *Server) handleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
    45		// 从 Sec-WebSocket-Protocol 头获取票据
    46		protocols := r.Header.Values("Sec-WebSocket-Protocol")
    47		if len(protocols) == 0 {
    48			http.Error(w, "Missing ticket in Sec-WebSocket-Protocol header", http.StatusBadRequest)
    49			return
    50		}
    51		ticketID := protocols[0]
    52	
    53		// 验证票据
    54		ticket, valid := s.ticketMgr.ValidateTicket(ticketID)
    55		if !valid {
    56			http.Error(w, "Invalid or expired ticket", http.StatusUnauthorized)
    57			return
    58		}
    59	
    60		// 从票据参数获取连接信息
    61		agentID := ticket.Params["agent_id"]
    62		host := ticket.Params["host"]
    63		portStr := ticket.Params["port"]
    64		protocolStr := ticket.Params["protocol"]
    65	
    66		if agentID == "" || host == "" || portStr == "" || protocolStr == "" {
    67			http.Error(w, "Invalid ticket parameters", http.StatusBadRequest)
    68			return
    69		}
    70	
    71		var port int
    72		if err := json.Unmarshal([]byte(portStr), &port); err != nil {
    73			http.Error(w, "Invalid port", http.StatusBadRequest)
    74			return
    75		}
    76	
    77		remoteProtocol := protocol.RemoteProtocol(protocolStr)
    78		if remoteProtocol != protocol.RemoteProtocolSSH &&
    79			remoteProtocol != protocol.RemoteProtocolTelnet &&
    80			remoteProtocol != protocol.RemoteProtocolVNC {
    81			http.Error(w, "Unsupported protocol", http.StatusBadRequest)
    82			return
    83		}
    84	
    85		agent, exists := s.registry.Get(agentID)
    86		if !exists {
    87			http.Error(w, "Agent not found or offline", http.StatusNotFound)
    88			return
    89		}
    90	
    91		// 升级 WebSocket，接受票据协议
    92		upgrader := websocket.Upgrader{
    93			CheckOrigin:  isOriginAllowed,
    94			Subprotocols: []string{ticketID},
    95		}
    96		conn, err := upgrader.Upgrade(w, r, nil)
    97		if err != nil {
    98			log.Printf("WebSocket upgrade failed: %v", err)
    99			return
   100		}
   101	
   102		connID := uuid.New().String()[:8]
   103		sessionID := uuid.New().String()
   104	
   105		session := &TerminalSession{
   106			ID:         sessionID,
   107			UserID:     ticket.UserID,
   108			Username:   ticket.Username,
   109			AgentID:    agentID,
   110			Protocol:   remoteProtocol,
   111			Host:       host,
   112			Port:       port,
   113			ClientWS:   conn,
   114			ConnID:     connID,
   115			CreatedAt:  time.Now(),
   116			LastActive: time.Now(),
   117			done:       make(chan struct{}),
   118		}
   119	
   120		terminalSessionsMu.Lock()
   121		terminalSessions[sessionID] = session
   122		terminalByConn[connID] = session
   123		terminalSessionsMu.Unlock()
   124	
   125		// 会话台账登记（D-2026-10-12-4）：terminal 会话 ID 与控制面同源，
   126		// 状态随建立/成功/失败/关闭推进（pending→connected→closed/failed）。
   127		s.remoteSessions.Register(sessionID, ticket.UserID, ticket.Username, RemoteSessionRequest{
   128			AgentID:  agentID,
   129			Protocol: remoteProtocol,
   130			Host:     host,
   131			Port:     port,
   132		})
   133	
   134		// 输出流录制（开关默认开；失败仅记日志，不影响远控主链路）
   135		if s.recordingEnabled() {
   136			session.recorder = s.startRecording(session)
   137		}
   138	
   139		log.Printf("Terminal session created: %s for user %s", sessionID, ticket.Username)
   140	
   141		target := host + ":" + portStr
   142		proxyNew := map[string]interface{}{
   143			"proxyId":   "terminal-" + connID,
   144			"proxyType": "tcp",
   145			"target":    target,
   146			"terminal":  true,
   147			"connId":    connID,
   148			"protocol":  string(remoteProtocol),
   149		}
   150		// SSH 凭据随 proxy_new 下发到 agent（agent 侧终结 SSH 协议）。
   151		// 口令只经加密 WS 通道传输，不落盘、不进日志/审计。
   152		if remoteProtocol == protocol.RemoteProtocolSSH {
   153			if v := ticket.Params["username"]; v != "" {
   154				proxyNew["username"] = v
   155			}
   156			if v := ticket.Params["password"]; v != "" {
   157				proxyNew["password"] = v
   158			}
   159			if v := ticket.Params["private_key"]; v != "" {
   160				proxyNew["privateKey"] = v
   161			}
   162		}
   163		msg := protocol.NewMessage(protocol.MessageTypeProxyNew, proxyNew)
   164	
   165		if err := agent.SendMessage(msg); err != nil {
   166			log.Printf("Failed to send proxy start message: %v", err)
   167			session.recorder.Close() // 已开的录制文件与元数据行照常收尾（其余出口都走这条）
   168			conn.WriteJSON(map[string]interface{}{
   169				"type":    "error",
   170				"message": "Failed to establish connection to agent",
   171			})
   172			conn.Close()
   173			terminalSessionsMu.Lock()
   174			delete(terminalSessions, sessionID)
   175			delete(terminalByConn, connID)
   176			terminalSessionsMu.Unlock()
   177			s.remoteSessions.UpdateStatus(sessionID, RemoteSessionStatusFailed, "agent unreachable")
   178			return
   179		}
   180		// proxy_new 已下发：会话建立（浏览器 ↔ agent 数据面就绪）
   181		_ = s.remoteSessions.UpdateStatus(sessionID, RemoteSessionStatusConnected, "")
   182	
   183		conn.WriteJSON(map[string]interface{}{
   184			"type":    "connect",
   185			"message": "Connecting to " + target,
   186		})
   187	
   188		go s.terminalSendLoop(session)
   189		s.terminalKeepaliveLoop(session)
   190	}
   191	
   192	// terminalSendLoop 浏览器 → Agent 数据转发
   193	func (s *Server) terminalSendLoop(session *TerminalSession) {
   194		defer func() {
   195			if r := recover(); r != nil {
   196				log.Printf("terminalSendLoop panic: %v", r)
   197			}
   198			close(session.done)
   199		}()
   200	
   201		conn := session.ClientWS
   202		connID := session.ConnID
   203	
   204		for {
   205			_, data, err := conn.ReadMessage()
   206			if err != nil {
   207				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
   208					log.Printf("WebSocket read error: %v", err)
   209				}
   210				s.sendCloseToAgent(session)
   211				return
   212			}
   213	
   214			// 更新最后活跃时间
   215			session.mu.Lock()
   216			session.LastActive = time.Now()
   217			session.mu.Unlock()
   218	
   219			var msg map[string]interface{}
   220			if err := json.Unmarshal(data, &msg); err == nil {
     1	package server
     2	
     3	import (
     4		"bufio"
     5		"fmt"
     6		"io"
     7		"log"
     8		"net"
     9		"net/http"
    10		"os"
    11		"path/filepath"
    12		"strconv"
    13		"strings"
    14		"sync"
    15		"time"
    16	
    17		"github.com/cuihairu/cockpit/internal/audit"
    18		"github.com/cuihairu/cockpit/internal/protocol"
    19		"github.com/cuihairu/cockpit/internal/storage"
    20		"github.com/google/uuid"
    21		"github.com/gorilla/websocket"
    22	)
    23	
    24	// Guacamole 网关（见 docs/remote-desktop-guacamole-design.md D2/D3/D4）：
    25	// 浏览器 ↔ server 段是 WebSocket（文本帧即 Guacamole 指令流），server ↔
    26	// guacd 段是 TCP（Guacamole 原生协议）。网关只做双向字节管道——**不解析
    27	// 指令内容**（握手指令构造除外），协议演进由 guacamole-common-js 与 guacd
    28	// 自己对齐（见设计风险章节「chunk/base64 规则照抄官方 Tunnel」）。
    29	//
    30	// 复用既有远控基础设施：一次性票据（ticket.go）、出口策略
    31	// （matchRemoteEgress）、审计（auditRemoteStart/End）。
    32	
    33	// guacdAddrEnv guacd 地址（Go 网关反代目标）。默认同机回环，跨机部署时
    34	// 指向 guacd 主机内网地址（见 deployments/guacd/README.md）。
    35	const guacdAddrEnv = "GUACD_ADDR"
    36	
    37	// guacdDefaultAddr guacd 默认地址
    38	const guacdDefaultAddr = "127.0.0.1:4822"
    39	
    40	// guacdDialTimeout 拨 guacd 超时
    41	const guacdDialTimeout = 10 * time.Second
    42	
    43	// dialGuacd 拨 guacd TCP 调用点。var 化（非内联）仅为测试注入 mock conn——
    44	// select write 失败分支需 guacdConn.Write 立即 error，而 TCP RST 时序
    45	// （loopback 写缓冲 + goroutine 调度）本质不确定；生产行为不变
    46	// （恒调 net.DialTimeout）。
    47	var dialGuacd = func(addr string, timeout time.Duration) (net.Conn, error) {
    48		return net.DialTimeout("tcp", addr, timeout)
    49	}
    50	
    51	// guacamoleReadBuf guacd→WS 段读缓冲（Guacamole 指令是文本行，含 base64
    52	// blob，单条可较大）。upgrader 按请求构造（子协议回显因票据而异），
    53	// 见 handleGuacamoleWebSocket 第 4 步。
    54	const guacamoleReadBuf = 256 * 1024
    55	
    56	// guacdAddr 返回 guacd 地址（可经 GUACD_ADDR 覆盖）。
    57	func guacdAddr() string {
    58		if v := os.Getenv(guacdAddrEnv); v != "" {
    59			return v
    60		}
    61		return guacdDefaultAddr
    62	}
    63	
    64	// guacSessions 活会话索引（sessionID -> 会话），与 terminal 的
    65	// terminalSessions 对称——此前 guacamole 会话是 handler 局部变量，
    66	// 控制面（会话台账 / DELETE 终止）不可达（D-2026-10-12-4 补）。
    67	var (
    68		guacSessions   = make(map[string]*GuacamoleSession)
    69		guacSessionsMu sync.Mutex
    70	)
    71	
    72	// GuacamoleSession 一条 Guacamole 桌面会话：浏览器 WS ↔ guacd TCP 双跳管道。
    73	// 职责：票据校验（握手时）+ 出口策略 + 审计 + 双向字节管道。
    74	type GuacamoleSession struct {
    75		ID       string
    76		UserID   string
    77		Username string
    78		Protocol string // rdp / vnc / ssh
    79		AgentID  string
    80		Host     string
    81		Port     int
    82		// Recording 本会话是否开了 guacd 录制（connect 带 recording-*）——
    83		// collect 用它做零延迟跳过（未录制会话不必等 guacd finalize 轮询）
    84		Recording bool
    85		// relay 目标转发器（guac_relay.go）：guacd 拨中继、agent 拨真实目标。
    86		// Host/Port 保留用户视角（审计/录制元数据/出口策略语义），connect 指令
    87		// 里放的是中继地址
    88		relay    *guacRelay
    89		ClientWS *websocket.Conn
    90		guacd    net.Conn
    91		guacdRd  *bufio.Reader // 握手阶段建立，guacdToWS 复用（select 响应不丢）
    92		Created  time.Time
    93	
    94		writeMu sync.Mutex // gorilla WS 单写者
    95		done    chan struct{}
    96		once    sync.Once
    97	}
    98	
    99	// guacEncode 编码一条 Guacamole 指令：长度前缀 + 逗号分隔参数 + 分号结尾。
   100	// 格式照抄 guacamole-common-js 的 Tunnel.sendMessage（见设计风险章节
   101	// 「chunk/base64 规则照抄官方 Tunnel 实现，别自由发挥」）。
   102	// 例：guacEncode("size", "1024", "768", "96") → "4.size,4.1024,3.768,2.96;"
   103	func guacEncode(opcode string, args ...string) string {
   104		var b strings.Builder
   105		fmt.Fprintf(&b, "%d.%s", len(opcode), opcode)
   106		for _, a := range args {
   107			fmt.Fprintf(&b, ",%d.%s", len(a), a)
   108		}
   109		b.WriteByte(';')
   110		return b.String()
   111	}
   112	
   113	// guacConnectArgs 组装 connect 指令参数：与 guacd args 指令声明的参数名
   114	// 逐位对齐的值序列（未设置的参数留空占位，首元素回显协议版本位）。
   115	// Guacamole 协议的 connect 是位置参数——稀疏 name=value 形态会被真 guacd
   116	// 以「Client did not return the expected number of arguments」拒绝
   117	// （真机验收 guacamole/guacd:1.5.5 发现，2026-09-30）。
   118	// 凭据来自票据 params（handleTicketCreate 存储），不经浏览器二次经手
   119	// （见设计「为什么参数由服务端放进 connect 指令」）。
   120	func guacConnectArgs(argNames []string, protocol, host string, port int, params map[string]string, width, height int, record bool, recordingName string) []string {
   121		values := map[string]string{
   122			"hostname": host,
   123			"port":     strconv.Itoa(port),
   124		}
   125		if v := params["username"]; v != "" {
   126			values["username"] = v
   127		}
   128		if v := params["password"]; v != "" {
   129			values["password"] = v
   130		}
   131		if v := params["domain"]; v != "" {
   132			values["domain"] = v
   133		}
   134		switch protocol {
   135		case "rdp":
   136			// 办公场景验收（设计风险章节）：色深 32、忽略证书（自签/内网常见）。
   137			// 音频：不传 disable-audio（guacd 默认启用），音频流由服务端主动推
   138			// audio 指令 + blob 流，自动经本网关字节管道透传（见 todo.md M4 D1）
   139			values["security"] = "any"
   140			values["ignore-cert"] = "true"
   141			values["color-depth"] = "32"
   142			values["create-drive-path"] = "true"
   143			if width > 0 && height > 0 {
   144				values["width"] = strconv.Itoa(width)
   145				values["height"] = strconv.Itoa(height)
   146			}
   147		case "vnc":
   148			// VNC 的 password 即同名参数，上面已统一映射
   149			values["color-depth"] = "32"
   150		case "ssh":
   151			// guacd ssh 插件（libssh2，docs/remote-access-integration-design.md D2）：
   152			// username/password 上面已统一传；私钥走 private-key 参数。
   153			// 必须传 PEM 原文，不能 base64——settings.h 注释写 "encoded as
   154			// base64" 是误导，实现链 guac_user_parse_args_string →
   155			// guac_common_ssh_key_alloc 是 strlen+memcpy 零解码，base64 文本
   156			// 直接进 libssh2 PEM_read 必失败（真机 A/B 实测 2026-09-30：
   157			// base64 → "Unsupported private key file format"，原文 → 两格式
   158			// 认证全过、终端输出正常）。
   159			// width/height 不进 connect——字符终端，尺寸经隧道层 size 指令由
   160			// guacd 按字体度量换算列/行；domain 是 RDP 专属概念不传。
   161			if v := params["private_key"]; v != "" {
   162				values["private-key"] = v
   163			}
   164		}
   165		// 桌面会话录制（设计「guacd session recording 白捡」）：recording-path/
   166		// recording-name 指向 guacd 容器卷（deployments/guacd 的
   167		// guacd-recordings 卷挂载点）；recording.enabled 关闭时不设置
   168		if record {
   169			values["recording-path"] = guacamoleRecordingPath()
   170			values["recording-name"] = recordingName
   171			values["create-recording-path"] = "true"
   172		}
   173		out := make([]string, 0, len(argNames))
   174		for _, name := range argNames {
   175			if strings.HasPrefix(name, "VERSION_") {
   176				// args 列表首位的协议版本标记（1.5.0 握手协商），原样回显
   177				out = append(out, name)
   178				continue
   179			}
   180			out = append(out, values[name]) // 未配置参数留空占位
   181		}
   182		return out
   183	}
   184	
   185	// guacParseArgNames 解析 guacd 对 select 的响应（args 指令）：返回去 opcode
   186	// 后的参数名列表；畸形输入返回 nil（调用方按握手失败处理）。
   187	func guacParseArgNames(line string) []string {
   188		s := strings.TrimSuffix(line, ";")
   189		parts := strings.Split(s, ",")
   190		if len(parts) < 2 {
   191			return nil
   192		}
   193		names := make([]string, 0, len(parts)-1)
   194		for _, p := range parts[1:] {
   195			dot := strings.IndexByte(p, '.')
   196			if dot < 0 {
   197				return nil
   198			}
   199			names = append(names, p[dot+1:])
   200		}
   201		return names
   202	}
   203	
   204	// guacdRecordingPathEnv guacd 录制目录（M3 D3）：同一路径同时作 guacd 的
   205	// recording-path 参数（guacd 内路径）与 server 的收集源路径——同机部署即
   206	// 同目录；Docker 部署把同一卷挂到两侧同路径（deployments/guacd 的
   207	// guacd-recordings 卷挂载点）。两侧视角合一避免双配置漂移。
   208	const guacdRecordingPathEnv = "GUACD_RECORDING_PATH"
   209	
   210	// guacamoleRecordingPath guacd 侧录制目录（可经 GUACD_RECORDING_PATH 覆盖）。
   211	func guacamoleRecordingPath() string {
   212		if v := os.Getenv(guacdRecordingPathEnv); v != "" {
   213			return v
   214		}
   215		return "/var/lib/guacamole"
   216	}
   217	
   218	// collectGuacRecording 会话结束时把 guacd 卷里的 <sid>.guac 收到
   219	// recordingsDir（M3 D2）并回填元数据。跨分区用 copy+remove（非 rename）；
   220	// 源文件缺失静默跳过（guacd 未写或已被外部清理），失败只记日志不阻塞
   221	// 会话出口路径（与 .cast 的「写失败停录不影响转发」同纪律）。
   222	func (s *Server) collectGuacRecording(gs *GuacamoleSession) {
   223		// 未开录制（connect 指令未带 recording-*）零延迟跳过
   224		if !gs.Recording {
   225			return
   226		}
   227		src := filepath.Join(guacamoleRecordingPath(), gs.ID+".guac")
   228		// guacd 在 TCP 关闭后才 finalize 录制文件（client 析构时 flush）：
   229		// 先等文件出现，再等 size 稳定（两次一致），防拷到截断文件
   230		var size int64
   231		stable := false
   232		for i := 0; i < 10; i++ {
   233			info, err := os.Stat(src)
   234			if err == nil && info.Size() == size && size > 0 {
   235				stable = true
   236				break
   237			}
   238			if err == nil {
   239				size = info.Size()
   240			}
   500			s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
   501			return
   502		}
   503	
   504		// 可选 body：{"thresholdMinutes": N}；空体/缺字段 = 0（全部 offline）。
   505		// 负数无意义，直接 400 防误清。
   506		var body struct {
   507			ThresholdMinutes int `json:"thresholdMinutes"`
   508		}
   509		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
   510			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
   511			return
   512		}
   513		if body.ThresholdMinutes < 0 {
   514			s.handleError(w, r, http.StatusBadRequest, "thresholdMinutes must be >= 0")
   515			return
   516		}
   517	
   518		// 1. 将 DB 中标记 online 但实际已断开的 agent 标记为 offline
   519		dbAgents, err := s.db.ListAgents()
   520		if err != nil {
   521			s.handleError(w, r, http.StatusInternalServerError, "Failed to list agents")
   522			return
   523		}
   524		for _, a := range dbAgents {
   525			if a.Status == "online" {
   526				if _, active := s.registry.Get(a.ID); !active {
   527					_ = s.db.UpdateAgentStatus(a.ID, "offline", a.LastSeen)
   528				}
   529			}
   530		}
   531	
   532		// 2. 按阈值清理（0 = 全部 offline）
   533		removed, err := s.db.CleanupOfflineAgents(time.Duration(body.ThresholdMinutes) * time.Minute)
   534		if err != nil {
   535			s.handleError(w, r, http.StatusInternalServerError, "Failed to cleanup agents")
   536			return
   537		}
   538	
   539		s.writeJSON(w, http.StatusOK, map[string]interface{}{
   540			"status":  "ok",
   541			"removed": removed,
   542			"count":   len(removed),
   543		})
   544	}
   545	
   546	// handleAgentSecret 管理 Agent 密钥（RBAC：inventory:write）
   547	func (s *Server) handleAgentSecret(w http.ResponseWriter, r *http.Request, id string) {
   548		agent, err := s.db.GetAgent(id)
   549		if err != nil {
   550			s.handleError(w, r, http.StatusNotFound, "Agent not found")
   551			return
   552		}
   553	
   554		switch r.Method {
   555		case http.MethodPost:
   556			// 重新生成密钥
   557			newSecret, err := s.db.RegenerateAgentSecret(id)
   558			if err != nil {
   559				s.handleError(w, r, http.StatusInternalServerError, "Failed to regenerate secret")
   560				return
   561			}
   562			s.writeJSON(w, http.StatusOK, map[string]interface{}{
   563				"agentId": id,
   564				"secret":  newSecret,
   565				"message": "Agent secret regenerated. Save this secret, it won't be shown again.",
   566			})
   567		case http.MethodGet:
   568			// 检查密钥状态
   569			hasSecret := agent.SecretHash != ""
   570			s.writeJSON(w, http.StatusOK, map[string]interface{}{
   571				"agentId":    id,
   572				"hasSecret":  hasSecret,
   573				"configured": hasSecret,
   574			})
   575		case http.MethodDelete:
   576			// 删除密钥（将允许无认证连接，不推荐）
   577			if err := s.db.UpdateAgentSecret(id, ""); err != nil {
   578				s.handleError(w, r, http.StatusInternalServerError, "Failed to remove secret")
   579				return
   580			}
   581			s.writeJSON(w, http.StatusOK, map[string]interface{}{
   582				"message": "Agent secret removed (authentication disabled)",
   583			})
   584		default:
   585			s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
   586		}
   587	}
   588	
   589	// handleResources 处理资源请求
   590	func (s *Server) handleResources(w http.ResponseWriter, r *http.Request, resourcePath string) {
   591		if r.Method != http.MethodGet {
   592			s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
   593			return
   594		}
   595	
   596		// 解析资源类型和可能的 ID
   597		parts := strings.Split(resourcePath, "/")
   598		resourceType := parts[0]
   599	
   600		switch resourceType {
   601		case "compute-instances":
   602			if len(parts) > 1 {
   603				s.handleComputeInstanceGet(w, r, parts[1])
   604			} else {
   605				s.handleComputeInstancesList(w, r)
   606			}
   607		case "domains":
   608			if len(parts) > 1 {
   609				s.handleDomainGet(w, r, parts[1])
   610			} else {
   611				s.handleDomainsList(w, r)
   612			}
   613		case "certificates":
   614			if len(parts) > 1 {
   615				s.handleCertificateGet(w, r, parts[1])
   616			} else {
   617				s.handleCertificatesList(w, r)
   618			}
   619		case "services":
   620			if len(parts) > 1 {
   621				s.handleServiceGet(w, r, parts[1])
   622			} else {
   623				s.handleServicesList(w, r)
   624			}
   625		case "gateways":
   626			if len(parts) > 1 {
   627				s.handleGatewayGet(w, r, parts[1])
   628			} else {
   629				s.handleGatewaysList(w, r)
   630			}
   631		case "storages":
   632			if len(parts) > 1 {
   633				s.handleStorageGet(w, r, parts[1])
   634			} else {
   635				s.handleStoragesList(w, r)
   636			}
   637		default:
   638			s.handleError(w, r, http.StatusNotFound, "Resource type not found")
   639		}
   640	}
   641	
   642	// handleComputeInstancesList 计算实例列表
   643	func (s *Server) handleComputeInstancesList(w http.ResponseWriter, r *http.Request) {
   644		instances, err := s.db.ListComputeInstances(nil)
   645		if err != nil {
   646			s.handleError(w, r, http.StatusInternalServerError, "Failed to list compute instances")
   647			return
   648		}
   649		s.writeList(w, instances, len(instances))
   650	}
   651	
   652	// handleComputeInstanceGet 获取单个计算实例
   653	func (s *Server) handleComputeInstanceGet(w http.ResponseWriter, r *http.Request, id string) {
   654		instance, err := s.db.GetComputeInstance(id)
   655		if err != nil {
   656			s.handleError(w, r, http.StatusNotFound, "Compute instance not found")
   657			return
   658		}
   659		s.writeJSON(w, http.StatusOK, instance)
   660	}
   661	
   662	// handleDomainsList 域名列表
   663	func (s *Server) handleDomainsList(w http.ResponseWriter, r *http.Request) {
   664		domains, err := s.db.ListDomains()
   665		if err != nil {
   666			s.handleError(w, r, http.StatusInternalServerError, "Failed to list domains")
   667			return
   668		}
   669		s.writeList(w, domains, len(domains))
   670	}
   671	
   672	// handleDomainGet 获取单个域名
   673	func (s *Server) handleDomainGet(w http.ResponseWriter, r *http.Request, id string) {
   674		domain, err := s.db.GetDomain(id)
   675		if err != nil {
   676			s.handleError(w, r, http.StatusNotFound, "Domain not found")
   677			return
   678		}
   679		s.writeJSON(w, http.StatusOK, domain)
   680	}
   681	
   682	// handleCertificatesList 证书列表
   683	func (s *Server) handleCertificatesList(w http.ResponseWriter, r *http.Request) {
   684		certificates, err := s.db.ListCertificates()
   685		if err != nil {
   686			s.handleError(w, r, http.StatusInternalServerError, "Failed to list certificates")
   687			return
   688		}
   689		s.writeList(w, certificates, len(certificates))
   690	}
   691	
   692	// handleCertificateGet 获取单个证书
   693	func (s *Server) handleCertificateGet(w http.ResponseWriter, r *http.Request, id string) {
   694		cert, err := s.db.GetCertificate(id)
   695		if err != nil {
   696			s.handleError(w, r, http.StatusNotFound, "Certificate not found")
   697			return
   698		}
   699		s.writeJSON(w, http.StatusOK, cert)
   700	}
   701	
   702	// handleServicesList 服务列表
   703	func (s *Server) handleServicesList(w http.ResponseWriter, r *http.Request) {
   704		services, err := s.db.ListServices()
   705		if err != nil {
   706			s.handleError(w, r, http.StatusInternalServerError, "Failed to list services")
   707			return
   708		}
   709		s.writeList(w, services, len(services))
   710	}
   711	
   712	// handleServiceGet 获取单个服务
   713	func (s *Server) handleServiceGet(w http.ResponseWriter, r *http.Request, id string) {
   714		svc, err := s.db.GetService(id)
   715		if err != nil {
   716			s.handleError(w, r, http.StatusNotFound, "Service not found")
   717			return
   718		}
   719		s.writeJSON(w, http.StatusOK, svc)
   720	}
   721	
   722	// handleGatewaysList 网关列表
   723	func (s *Server) handleGatewaysList(w http.ResponseWriter, r *http.Request) {
   724		gateways, err := s.db.ListGateways()
   725		if err != nil {
   726			s.handleError(w, r, http.StatusInternalServerError, "Failed to list gateways")
   727			return
   728		}
   729		s.writeList(w, gateways, len(gateways))
   730	}
   731	
   732	// handleGatewayGet 获取单个网关
   733	func (s *Server) handleGatewayGet(w http.ResponseWriter, r *http.Request, id string) {
   734		gw, err := s.db.GetGateway(id)
   735		if err != nil {
   736			s.handleError(w, r, http.StatusNotFound, "Gateway not found")
   737			return
   738		}
   739		s.writeJSON(w, http.StatusOK, gw)
   740	}
   741	
   742	// handleStoragesList 存储列表
   743	func (s *Server) handleStoragesList(w http.ResponseWriter, r *http.Request) {
   744		storages, err := s.db.ListStorages()
   745		if err != nil {
   746			s.handleError(w, r, http.StatusInternalServerError, "Failed to list storages")
   747			return
   748		}
   749		s.writeList(w, storages, len(storages))
   750	}
   751	
   752	// handleStorageGet 获取单个存储
   753	func (s *Server) handleStorageGet(w http.ResponseWriter, r *http.Request, id string) {
   754		st, err := s.db.GetStorage(id)
   755		if err != nil {
   756			s.handleError(w, r, http.StatusNotFound, "Storage not found")
   757			return
   758		}
   759		s.writeJSON(w, http.StatusOK, st)
   760	}
   761	
   762	// storageAgentToResponse 将存储 Agent 转换为 API 响应格式
   763	func storageAgentToResponse(agent *storage.Agent) map[string]interface{} {
   764		// capabilities 保持对象数组（type/version/endpoint/metadata）——web/mobile 的
   765		// Capability.fromJson 与 workbench getRemoteServices 都读 cap.type/cap.metadata，
   766		// 此前扁平化成 []string 导致全部前端能力判断失效（真机验收发现，2026-09-30）。
   767		capabilities := agent.Capabilities
   768		if capabilities == nil {
   769			capabilities = []storage.Capability{}
   770		}
   771	
   772		return map[string]interface{}{
   773			"id":           agent.ID,
   774			"hostname":     agent.Hostname,
   775			"ip":           agent.IP,
   776			"localIps":     agent.LocalIPs,
   777			"region":       agent.Region,
   778			"zone":         agent.Zone,
   779			"capabilities": capabilities,
   780			"status":       agent.Status,


[showing lines 104-958 of 958; full output saved to /home/cui/.local/share/opencode/shell/2b220c9cad2a7053c583eada91fc7bca7b5b6399/sh_14f069d2b0014SBnewXX2dJ0ey.out]
[0m$ [0mnl -ba internal/storage/role.go | sed -n '1,280p'; nl -ba internal/audit/logger.go | sed -n '1,300p'; nl -ba internal/server/api_jobs.go | sed -n '278,380p'; nl -ba internal/server/remote_audit.go | sed -n '1,280p'
1	package storage
     2	
     3	import (
     4		"encoding/json"
     5		"errors"
     6		"strings"
     7		"time"
     8	
     9		"gorm.io/gorm"
    10	)
    11	
    12	// 角色与权限点（见 docs/guide/rbac-design.md）：用户 → 角色 → 权限点。
    13	// 本文件是权限点合法集合的单一事实源（resourceActions 表），内置角色
    14	// 的权限清单与角色写入校验都从它派生；server 侧路由权限表按字符串引用。
    15	
    16	var (
    17		// ErrBuiltinRole 内置角色不可修改/删除（D12：定义以代码为准）
    18		ErrBuiltinRole = errors.New("builtin role cannot be modified or deleted")
    19		// ErrRoleInUse 角色仍被用户引用（软引用删除会让这些用户 fail-closed 全拒）
    20		ErrRoleInUse = errors.New("role is still assigned to users")
    21		// ErrInvalidPermission 权限点不在合法集合内（自定义角色白名单校验）
    22		ErrInvalidPermission = errors.New("permission not in the allowed set")
    23		// ErrEmptyRoleName 角色名为空
    24		ErrEmptyRoleName = errors.New("role name is required")
    25	)
    26	
    27	// resourceActions D3 权限点清单：resource → 允许的 action 集。
    28	// action ∈ read/write/admin；约定 write ⊇ read、admin ⊇ write（判定在 server 侧）。
    29	var resourceActions = map[string][]string{
    30		"inventory": {"read", "write"},
    31		"files":     {"read", "write"},
    32		"logs":      {"read"},
    33		"terminal":  {"write"},
    34		"docker":    {"read", "write"},
    35		"stack":     {"read", "write"},
    36		"cron":      {"read", "write"},
    37		"backup":    {"read", "write"},
    38		"acme":      {"read", "write", "admin"},
    39		"dns":       {"read", "write"},
    40		"ddns":      {"read", "write"},
    41		"proxy":     {"read", "write"},
    42		"overlay":   {"read", "write", "admin"},
    43		"drift":     {"read", "write"},
    44		"nas":       {"read", "write"},
    45		"alerts":    {"read", "write"},
    46		"audit":     {"read"},
    47		"users":     {"admin"},
    48		"roles":     {"admin"},
    49		"settings":  {"admin"},
    50		// 笔 2 补齐：systemd 服务/SMART 磁盘/会话录制（D3 清单实现时对账补全）
    51		"services":   {"read", "write"},
    52		"smart":      {"read", "write"},
    53		"recordings": {"read", "write"},
    54		// 防火墙观测（firewall-design.md D10）：M1 只读快照，单 read 档
    55		"firewall": {"read"},
    56		// 执行 Job（jobs-design.md）：读写均为执行态操作（write=创建执行，
    57		// read=查看列表/详情）
    58		"jobs": {"read", "write"},
    59		// Workflow 编排（workflow-design.md W9）：read=定义/run 台账，write=CRUD/run/cancel
    60		"workflows": {"read", "write"},
    61		// 审批门（workflow-design.md M2d；rbac.go /api/approvals 门禁）：
    62		// read=审批队列，write=approve/reject。漏登记会让全端点 403、菜单永不渲染。
    63		"approvals": {"read", "write"},
    64	}
    65	
    66	// PermissionValid 权限点是否落在 D3 清单内
    67	func PermissionValid(perm string) bool {
    68		i := strings.LastIndex(perm, ":")
    69		if i <= 0 {
    70			return false
    71		}
    72		for _, a := range resourceActions[perm[:i]] {
    73			if perm[i+1:] == a {
    74				return true
    75			}
    76		}
    77		return false
    78	}
    79	
    80	// allPermissions 全部合法权限点（admin 角色即此清单）
    81	func allPermissions() []string {
    82		var out []string
    83		// 固定 resource 顺序使 seed 结果稳定（map 遍历乱序）
    84		for _, res := range []string{
    85			"inventory", "files", "logs", "terminal", "docker", "stack", "cron",
    86			"backup", "acme", "dns", "ddns", "proxy", "overlay", "drift", "nas",
    87			"alerts", "audit", "users", "roles", "settings",
    88			"services", "smart", "recordings", "jobs", "workflows", "firewall",
    89			"approvals",
    90		} {
    91			for _, a := range resourceActions[res] {
    92				out = append(out, res+":"+a)
    93			}
    94		}
    95		return out
    96	}
    97	
    98	// operatorPermissions 全部模块 read+write（含 terminal:write、acme:admin），
    99	// 无 users/roles/settings（D8）；无 overlay:admin（M3 D28：本机 join/leave
   100	// 能把主机踢出网络，破坏半径高于 operator 日常运维，需显式授予）
   101	func operatorPermissions() []string {
   102		out := make([]string, 0, len(allPermissions()))
   103		for _, p := range allPermissions() {
   104			switch p {
   105			case "users:admin", "roles:admin", "settings:admin", "overlay:admin":
   106			default:
   107				out = append(out, p)
   108			}
   109		}
   110		return out
   111	}
   112	
   113	// viewerPermissions 全模块只读，无 terminal（D8）
   114	func viewerPermissions() []string {
   115		var out []string
   116		for _, p := range allPermissions() {
   117			if strings.HasSuffix(p, ":read") {
   118				out = append(out, p)
   119			}
   120		}
   121		return out
   122	}
   123	
   124	// builtinRoles 内置角色定义（D8）；seed 时 builtin 角色 permissions 以此为准
   125	var builtinRoles = []struct {
   126		name        string
   127		permissions []string
   128	}{
   129		{"admin", allPermissions()},
   130		{"operator", operatorPermissions()},
   131		{"viewer", viewerPermissions()},
   132	}
   133	
   134	// Role 角色表（D4）：name 主键、permissions JSON、builtin 标记。
   135	// User.role 沿用字符串存角色名（软引用，无外键）。
   136	type Role struct {
   137		Name        string    `gorm:"primaryKey;size:64" json:"name"`
   138		Permissions []string  `gorm:"serializer:json" json:"permissions"`
   139		Builtin     bool      `json:"builtin"`
   140		CreatedAt   time.Time `json:"created_at"`
   141		UpdatedAt   time.Time `json:"updated_at"`
   142	}
   143	
   144	// ListRoles 全部角色（按名字排序）
   145	func (d *DB) ListRoles() ([]*Role, error) {
   146		var list []*Role
   147		if err := d.db.Order("name").Find(&list).Error; err != nil {
   148			return nil, err
   149		}
   150		return list, nil
   151	}
   152	
   153	// GetRole 按名字取角色
   154	func (d *DB) GetRole(name string) (*Role, error) {
   155		var r Role
   156		if err := d.db.First(&r, "name = ?", name).Error; err != nil {
   157			if errors.Is(err, gorm.ErrRecordNotFound) {
   158				return nil, ErrNotFound
   159			}
   160			return nil, err
   161		}
   162		return &r, nil
   163	}
   164	
   165	// CreateRole 新建自定义角色：名字非空且不撞内置角色、权限点全部合法
   166	func (d *DB) CreateRole(role *Role) error {
   167		if err := validateCustomRole(role); err != nil {
   168			return err
   169		}
   170		return d.db.Create(role).Error
   171	}
   172	
   173	// UpdateRole 更新自定义角色的权限清单（内置角色拒绝）
   174	func (d *DB) UpdateRole(role *Role) error {
   175		existing, err := d.GetRole(role.Name)
   176		if err != nil {
   177			return err
   178		}
   179		if existing.Builtin {
   180			return ErrBuiltinRole
   181		}
   182		if err := validatePermissions(role.Permissions); err != nil {
   183			return err
   184		}
   185		// serializer:json 字段不走 Updates(map) 的 schema 序列化，手动 JSON
   186		// （同 agentUpdateFields 先例）；[]string 序列化必成功，忽略 err
   187		b, _ := json.Marshal(role.Permissions)
   188		return d.db.Model(existing).Updates(map[string]interface{}{
   189			"permissions": string(b),
   190		}).Error
   191	}
   192	
   193	// DeleteRole 删除自定义角色（内置拒绝；仍被用户引用拒绝——删了会让
   194	// 这些用户查不到角色而 fail-closed 全拒，等价锁号）。引用计数在角色
   195	// 读取之前（顺序不影响语义，且关库路径先落 Count 的错误分支）
   196	func (d *DB) DeleteRole(name string) error {
   197		var count int64
   198		if err := d.db.Model(&User{}).Where("role = ?", name).Count(&count).Error; err != nil {
   199			return err
   200		}
   201		if count > 0 {
   202			return ErrRoleInUse
   203		}
   204		existing, err := d.GetRole(name)
   205		if err != nil {
   206			return err
   207		}
   208		if existing.Builtin {
   209			return ErrBuiltinRole
   210		}
   211		return d.db.Delete(&Role{}, "name = ?", name).Error
   212	}
   213	
   214	// CountUsersByRoles 统计角色名在集合内的用户数（D13 有效 admin 计数的
   215	// 查询半边；判定半边在 server 侧用 roleCovers 筛角色名）。skipUserID
   216	// 排除假想变更的目标用户本身。
   217	func (d *DB) CountUsersByRoles(roleNames []string, skipUserID string) (int64, error) {
   218		q := d.db.Model(&User{}).Where("role IN ?", roleNames)
   219		if skipUserID != "" {
   220			q = q.Where("id != ?", skipUserID)
   221		}
   222		var count int64
   223		if err := q.Count(&count).Error; err != nil {
   224			return 0, err
   225		}
   226		return count, nil
   227	}
   228	
   229	// SeedRoles 幂等 seed 内置角色（Open 时自动执行）：不存在的创建；与代码
   230	// 定义不一致的按代码覆盖 permissions（内置角色用户不可改——D12，版本升级
   231	// 调整内置清单必须能生效；撞名的自定义角色同样收编为 builtin）。
   232	// 稳态（角色已一致、无存量 user）零写——只读库 reopen 场景（server 测试
   233	// 的 DB 错误注入手法）不触发写。随后执行 D9 存量迁移：role=user → viewer
   234	// （现状 user 即无写权限，语义保持）。
   235	func (d *DB) SeedRoles() error {
   236		for _, b := range builtinRoles {
   237			var existing Role
   238			err := d.db.First(&existing, "name = ?", b.name).Error
   239			switch {
   240			case errors.Is(err, gorm.ErrRecordNotFound):
   241				role := &Role{Name: b.name, Permissions: b.permissions, Builtin: true}
   242				if err := d.db.Create(role).Error; err != nil {
   243					return err
   244				}
   245			case err != nil:
   246				return err
   247			default:
   248				if existing.Builtin && samePermissions(existing.Permissions, b.permissions) {
   249					continue
   250				}
   251				existing.Permissions = b.permissions
   252				existing.Builtin = true
   253				if err := d.db.Save(&existing).Error; err != nil {
   254					return err
   255				}
   256			}
   257		}
   258		var legacy int64
   259		if err := d.db.Model(&User{}).Where("role = ?", "user").Count(&legacy).Error; err != nil {
   260			return err
   261		}
   262		if legacy == 0 {
   263			return nil
   264		}
   265		return d.db.Model(&User{}).Where("role = ?", "user").Update("role", "viewer").Error
   266	}
   267	
   268	// samePermissions 权限清单无序相等
   269	func samePermissions(a, b []string) bool {
   270		if len(a) != len(b) {
   271			return false
   272		}
   273		set := make(map[string]bool, len(a))
   274		for _, p := range a {
   275			set[p] = true
   276		}
   277		for _, p := range b {
   278			if !set[p] {
   279				return false
   280			}
     1	package audit
     2	
     3	import (
     4		"encoding/json"
     5		"time"
     6	
     7		"github.com/cuihairu/cockpit/internal/storage"
     8	)
     9	
    10	// Action 操作类型
    11	const (
    12		ActionLogin       = "login"
    13		ActionLogout      = "logout"
    14		ActionCreate      = "create"
    15		ActionUpdate      = "update"
    16		ActionDelete      = "delete"
    17		ActionView        = "view"
    18		ActionExport      = "export"
    19		ActionImport      = "import"
    20		ActionStart       = "start"
    21		ActionStop        = "stop"
    22		ActionRestart     = "restart"
    23		ActionTOTPEnable  = "totp_enable"
    24		ActionTOTPDisable = "totp_disable"
    25		ActionTOTPVerify  = "totp_verify"
    26		// ActionRemoteStart / ActionRemoteEnd 远控会话开始/结束
    27		ActionRemoteStart = "remote_start"
    28		ActionRemoteEnd   = "remote_end"
    29		// ActionVaultVerify 远控凭据保险箱二次验证（阿里云密码箱模式：管理凭据
    30		// 前重验身份，签发短期多次使用的 vault token）
    31		ActionVaultVerify = "vault_verify"
    32		// Compose Stack 部署（见 docs/guide/stack-deploy-design.md）
    33		ActionStackCreate   = "stack_create"
    34		ActionStackUpdate   = "stack_update"
    35		ActionStackUp       = "stack_up"
    36		ActionStackDown     = "stack_down"
    37		ActionStackRemove   = "stack_remove"
    38		ActionStackRestart  = "stack_restart"
    39		ActionStackPull     = "stack_pull"
    40		ActionStackGitPull  = "stack_git_pull" // P2 ① Git-to-deploy：stacks 仓库拉取/接入
    41		ActionStackWebhookDeploy = "stack_webhook_deploy" // P2 ② webhook 触发拉取+部署（username=webhook）
    42		// 服务健康探针与自愈（见 docs/guide/service-health-design.md D9）
    43		ActionServiceHealthConfig      = "service_health_config"       // 探针/白名单配置保存
    44		ActionServiceHealthHeal        = "service_health_heal"         // 自愈重启成功（agent 事件归集，username=cockpit-agent）
    45		ActionServiceHealthHealFailed  = "service_health_heal_failed"  // 自愈重启失败
    46		ActionServiceHealthHealBlocked = "service_health_heal_blocked" // 非白名单拦截（只告警不动手）
    47		// 备份管理（见 docs/guide/backup-design.md）
    48		ActionBackupRun        = "backup_run"
    49		ActionBackupRestore    = "backup_restore"
    50		ActionBackupDeleteFile = "backup_delete_file"
    51		ActionBackupRemoteSync = "backup_remote_sync" // 手动补传备份文件到 rclone 远端（M2 D24）
    52		// 反向代理管理（见 docs/guide/proxy-design.md）
    53		ActionProxyApply  = "proxy_apply"
    54		ActionProxyDelete = "proxy_delete"
    55		// 定时任务管理（见 docs/guide/cron-design.md）
    56		ActionCronApply  = "cron_apply"
    57		ActionCronDelete = "cron_delete"
    58		// systemd 服务管理（见 docs/guide/service-design.md）
    59		ActionServiceAction = "service_action"
    60		// 执行 Job（见 docs/guide/jobs-design.md）：创建即执行，终态回写同记一条
    61		ActionJobRun = "job_run"
    62		// Job 取消（workflow-design W2/W3：pending 未派发可撤）
    63		ActionJobCancel = "job_cancel"
    64		// Workflow 定义变更与运行（见 docs/guide/workflow-design.md）
    65		ActionWorkflowCreate = "workflow_create"
    66		ActionWorkflowUpdate = "workflow_update"
    67		ActionWorkflowDelete = "workflow_delete"
    68		ActionWorkflowRun    = "workflow_run"
    69		ActionWorkflowCancel = "workflow_cancel"
    70		// 审批门（workflow-design M2d）：高危 Job 提交待批与批准/驳回。提交与
    71		// 执行（job_run）分账——「谁提交」「谁批准」与「谁执行」在审计上可分。
    72		ActionApprovalSubmit  = "approval_submit"
    73		ActionApprovalApprove = "approval_approve"
    74		ActionApprovalReject  = "approval_reject"
    75		// 防漂移检测：手动登记基线「以当前为准」（见 docs/guide/drift-design.md M4）
    76		ActionDriftRecord = "drift_record"
    77		// agent 执行留痕归集（agent-core P5 exec_log；username=cockpit-agent，
    78		// resource=执行面 provider）。与 service_health 自愈归集同口径：审计
    79		// 由 server 落库，来源标记 cockpit-agent。
    80		ActionExec = "exec"
    81	)
    82	
    83	// ResourceStack Compose Stack 资源类型
    84	const ResourceStack = "stack"
    85	
    86	// ResourceBackup 备份配置资源类型
    87	const ResourceBackup = "backup"
    88	
    89	// ResourceRemoteSession 远控会话资源类型
    90	const ResourceRemoteSession = "remote_session"
    91	
    92	// ResourceRemoteCredential 远控凭据保险箱资源类型（凭据的保存/删除/列表）
    93	const ResourceRemoteCredential = "remote_credential"
    94	
    95	// ResourceProbe 拨测配置资源类型
    96	const ResourceProbe = "probe"
    97	
    98	// ResourceNotification 通知渠道资源类型
    99	const ResourceNotification = "notification"
   100	
   101	// 文件管理动作（只记变更类操作，浏览/读/下载不记）
   102	const (
   103		ActionFileWrite  = "file_write"
   104		ActionFileMkdir  = "file_mkdir"
   105		ActionFileDelete = "file_delete"
   106		ActionFileRename = "file_rename"
   107		ActionFileChmod  = "file_chmod"
   108		ActionFileChown  = "file_chown"
   109	)
   110	
   111	// ResourceFile 远程文件资源类型
   112	const ResourceFile = "file"
   113	
   114	// ResourceProxySite 反代站点资源类型（见 docs/guide/proxy-design.md）
   115	const ResourceProxySite = "proxy_site"
   116	
   117	// ResourceCronJob 定时任务资源类型（见 docs/guide/cron-design.md）
   118	const ResourceCronJob = "cron_job"
   119	
   120	// ResourceJob 执行 Job 资源类型（见 docs/guide/jobs-design.md）
   121	const ResourceJob = "job"
   122	
   123	// ResourceApproval 审批门资源类型（见 docs/guide/workflow-design.md M2d）
   124	const ResourceApproval = "approval"
   125	
   126	// ResourceWorkflow Workflow 编排资源类型（定义与 run 共用，见 workflow-design.md）
   127	const ResourceWorkflow = "workflow"
   128	
   129	// ResourceService systemd 服务资源类型（见 docs/guide/service-design.md）
   130	const ResourceService = "service"
   131	
   132	// ResourceRecording 会话录制资源类型（见 recording-design.md）
   133	const ResourceRecording = "recording"
   134	
   135	// ResourceServerBackup server 自身数据库备份资源类型（见 server-backup-design.md）
   136	const ResourceServerBackup = "server_backup"
   137	
   138	// ResourceDriftBaseline 漂移基线资源类型（手动登记以当前为准，drift-design M4）
   139	const ResourceDriftBaseline = "drift_baseline"
   140	
   141	// ResourceDNSRecord DNS 记录资源类型（见 dns-design.md）
   142	const ResourceDNSRecord = "dns_record"
   143	
   144	// ResourceDomain Domain 台账资源类型（DNS 写联动的登记/移除，dns-design M3）
   145	const ResourceDomain = "domain"
   146	
   147	// ResourceAcmeCert ACME 签发证书资源类型（见 acme-design.md）
   148	const ResourceAcmeCert = "acme_cert"
   149	
   150	// ResourceOverlayMember 组网云管理面成员/设备资源类型（见 overlay-design.md M2 D14）
   151	const ResourceOverlayMember = "overlay_member"
   152	
   153	// ResourceOverlayNetwork 组网本机侧网络加入/离开资源类型（见 overlay-design.md M3 D29）
   154	const ResourceOverlayNetwork = "overlay_network"
   155	
   156	// ResourceDomainBinding 服务域名绑定资源类型（见 domain-binding-design.md）
   157	const ResourceDomainBinding = "domain_binding"
   158	
   159	// Status 状态
   160	const (
   161		StatusSuccess = "success"
   162		StatusFailure = "failure"
   163	)
   164	
   165	// LogEntry 日志条目
   166	type LogEntry struct {
   167		UserID     string
   168		Username   string
   169		Action     string
   170		Resource   string
   171		ResourceID string
   172		Details    interface{}
   173		IP         string
   174		UserAgent  string
   175		Status     string
   176	}
   177	
   178	// Logger 审计日志记录器
   179	type Logger struct {
   180		db *storage.DB
   181	}
   182	
   183	// NewLogger 创建新的日志记录器
   184	func NewLogger(db *storage.DB) *Logger {
   185		return &Logger{db: db}
   186	}
   187	
   188	// Log 记录审计日志
   189	func (l *Logger) Log(entry *LogEntry) error {
   190		detailsJSON := ""
   191		if entry.Details != nil {
   192			bytes, err := json.Marshal(entry.Details)
   193			if err == nil {
   194				detailsJSON = string(bytes)
   195			}
   196		}
   197	
   198		log := &storage.AuditLog{
   199			UserID:     entry.UserID,
   200			Username:   entry.Username,
   201			Action:     entry.Action,
   202			Resource:   entry.Resource,
   203			ResourceID: entry.ResourceID,
   204			Details:    detailsJSON,
   205			IP:         entry.IP,
   206			UserAgent:  entry.UserAgent,
   207			Status:     entry.Status,
   208			CreatedAt:  time.Now(),
   209		}
   210	
   211		return l.db.CreateAuditLog(log)
   212	}
   213	
   214	// LogSuccess 记录成功操作
   215	func (l *Logger) LogSuccess(username, action, resource, resourceID string, details interface{}, ip, userAgent string) error {
   216		return l.Log(&LogEntry{
   217			Username:   username,
   218			Action:     action,
   219			Resource:   resource,
   220			ResourceID: resourceID,
   221			Details:    details,
   222			IP:         ip,
   223			UserAgent:  userAgent,
   224			Status:     StatusSuccess,
   225		})
   226	}
   227	
   228	// LogFailure 记录失败操作
   229	func (l *Logger) LogFailure(username, action, resource, resourceID string, details interface{}, ip, userAgent string) error {
   230		return l.Log(&LogEntry{
   231			Username:   username,
   232			Action:     action,
   233			Resource:   resource,
   234			ResourceID: resourceID,
   235			Details:    details,
   236			IP:         ip,
   237			UserAgent:  userAgent,
   238			Status:     StatusFailure,
   239		})
   240	}
   241	
   242	// LogLogin 记录登录操作
   243	func (l *Logger) LogLogin(username string, success bool, ip, userAgent string) error {
   244		status := StatusSuccess
   245		if !success {
   246			status = StatusFailure
   247		}
   248		return l.Log(&LogEntry{
   249			Username:  username,
   250			Action:    ActionLogin,
   251			Resource:  "user",
   252			Details:   map[string]interface{}{"login_type": "password"},
   253			IP:        ip,
   254			UserAgent: userAgent,
   255			Status:    status,
   256		})
   257	}
   258	
   259	// LogLogout 记录登出操作
   260	func (l *Logger) LogLogout(username string, ip, userAgent string) error {
   261		return l.Log(&LogEntry{
   262			Username:  username,
   263			Action:    ActionLogout,
   264			Resource:  "user",
   265			IP:        ip,
   266			UserAgent: userAgent,
   267			Status:    StatusSuccess,
   268		})
   269	}
   270	
   271	// LogResource 记录资源操作
   272	func (l *Logger) LogResource(username, action, resource, resourceID string, details interface{}, ip, userAgent string) error {
   273		return l.Log(&LogEntry{
   274			Username:   username,
   275			Action:     action,
   276			Resource:   resource,
   277			ResourceID: resourceID,
   278			Details:    details,
   279			IP:         ip,
   280			UserAgent:  userAgent,
   281			Status:     StatusSuccess,
   282		})
   283	}
   284	
   285	// LogTOTPEnabled 记录 TOTP 启用操作
   286	func (l *Logger) LogTOTPEnabled(userID, ip, userAgent string) error {
   287		return l.Log(&LogEntry{
   288			Username:  userID,
   289			Action:    ActionTOTPEnable,
   290			Resource:  "totp",
   291			IP:        ip,
   292			UserAgent: userAgent,
   293			Status:    StatusSuccess,
   294		})
   295	}
   296	
   297	// LogTOTPDisabled 记录 TOTP 禁用操作
   298	func (l *Logger) LogTOTPDisabled(userID, ip, userAgent string) error {
   299		return l.Log(&LogEntry{
   300			Username:  userID,
   278	// dispatchJobAsync 后台派发单条 Job：pending → running → 终态回写 + 审计
   279	func (s *Server) dispatchJobAsync(job *storage.Job, payload jobCreatePayload, actor string) {
   280		now := time.Now()
   281		job.Status = storage.JobStatusRunning
   282		job.StartedAt = &now
   283		if err := s.db.UpdateJob(job); err != nil {
   284			// 状态无法回写不阻碍执行（下一次 UpdateJob 会覆盖）
   285			log.Printf("jobs: update running state failed: %v", err)
   286		}
   287	
   288		success, jobErr := s.dispatchJob(job, payload)
   289	
   290		finished := time.Now()
   291		job.FinishedAt = &finished
   292		job.Output, job.Error, job.ExitCode = jobErr.Output, jobErr.Message, jobErr.ExitCode
   293	
   294		// C7 取消守卫：回写前重读，若取消写已落 cancelled（exec 被 job.cancel
   295		// 击杀）则保留 cancelled——取消与执行回写的竞态由库内状态收敛，终态
   296		// 不被覆盖（output/exit_code/error 补进台账供查）
   297		if fresh, err := s.db.GetJob(job.ID); err == nil && fresh.Status == storage.JobStatusCancelled {
   298			job.Status = storage.JobStatusCancelled
   299			if err := s.db.MarkJobCancelledOutput(job.ID, job.Output, job.Error, job.ExitCode); err != nil {
   300				log.Printf("jobs: write cancelled output failed: %v", err)
   301			}
   302		} else {
   303			if success {
   304				job.Status = storage.JobStatusSuccess
   305			} else {
   306				job.Status = storage.JobStatusFailed
   307			}
   308			if err := s.db.UpdateJob(job); err != nil {
   309				log.Printf("jobs: update final state failed: %v", err)
   310			}
   311		}
   312	
   313		// 审计：命令入参进 details（与 cron_apply 同口径），不涉及凭据
   314		details := map[string]interface{}{
   315			"type":   job.Type,
   316			"target": job.Target,
   317			"status": job.Status,
   318			"params": payload.Parameters,
   319		}
   320		s.audit.LogResource(actor, audit.ActionJobRun, audit.ResourceJob, job.ID,
   321			details, "", "")
   322	}
   323	
   324	// handleJobCancel 取消 Job（W3 pending 可撤；M2c C6 running 可撤：RPC
   325	// job.cancel 击杀在途进程组，agent 不可达则如实拒绝——绝不假称已停）。
   326	// 终态一律 409；pending→running 翻转竞态由 cancelJob 内部条件 UPDATE 收敛。
   327	func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request, id string) {
   328		job, err := s.db.GetJob(id)
   329		if err != nil {
   330			s.handleError(w, r, http.StatusNotFound, "job not found")
   331			return
   332		}
   333		if err := s.cancelJob(job); err != nil {
   334			s.handleError(w, r, http.StatusConflict, err.Error())
   335			return
   336		}
   337	
   338		username := "unknown"
   339		if userInfo, ok := auth.GetUserFromContext(r); ok {
   340			username = userInfo.Username
   341		}
   342		s.audit.LogResource(username, audit.ActionJobCancel, audit.ResourceJob, id,
   343			map[string]interface{}{"type": job.Type, "target": job.Target},
   344			s.getClientIP(r), r.UserAgent())
   345	
   346		updated, err := s.db.GetJob(id)
   347		if err != nil {
   348			s.handleError(w, r, http.StatusInternalServerError, "failed to load job")
   349			return
   350		}
   351		s.writeJSON(w, http.StatusOK, toJobView(updated))
   352	}
   353	
   354	// cancelJob 按 Job 状态落取消（C6）：pending → 条件 UPDATE（DB-only，未派发
   355	// 无进程可杀）；running → RPC job.cancel 击杀 + 条件 UPDATE；终态 → 拒绝。
   356	// pending 条件更新 0 行 = 状态已翻转（dispatch 启动）——重读库内新状态重走
   357	// 一次（状态只前进，递归至多一层不死循环）。返回非 nil 即 409 文案。
   358	func (s *Server) cancelJob(job *storage.Job) error {
   359		switch job.Status {
   360		case storage.JobStatusPending:
   361			hit, err := s.db.CancelJob(job.ID, storage.JobStatusPending, time.Now())
   362			if err != nil {
   363				return errors.New("failed to cancel job")
   364			}
   365			if hit > 0 {
   366				return nil
   367			}
   368			fresh, err := s.db.GetJob(job.ID)
   369			if err != nil {
   370				return errors.New("job not found")
   371			}
   372			return s.cancelJob(fresh)
   373		case storage.JobStatusRunning:
   374			if _, err := s.CallAgent(job.Target, jobCancelMethod,
   375				map[string]interface{}{"id": job.ID}); err != nil {
   376				return fmt.Errorf("cannot cancel running job: %v", err)
   377			}
   378			hit, err := s.db.CancelJob(job.ID, storage.JobStatusRunning, time.Now())
   379			if err != nil {
   380				return errors.New("failed to cancel job")
     1	package server
     2	
     3	import (
     4		"fmt"
     5		"net"
     6		"net/http"
     7		"strings"
     8	
     9		"github.com/cuihairu/cockpit/internal/audit"
    10		"github.com/cuihairu/cockpit/internal/config"
    11	)
    12	
    13	// validateRemoteTarget 校验目标 host 是否被允许用于远控连接。
    14	//
    15	// 规则（按顺序短路）：
    16	//  1. 配置显式启用 AllowArbitraryTarget -> 全部放行
    17	//  2. host 命中 AllowedTargets 列表中的显式 host/IP 或 CIDR（端口不限） -> 放行
    18	//  3. 否则拒绝
    19	//
    20	// 返回值：
    21	//   - allow=true 时通过；allow=false 时拒绝，errMsg 给出可向客户端展示的原因
    22	func (s *Server) validateRemoteTarget(host string) (allow bool, errMsg string) {
    23		if host == "" {
    24			return false, "missing target host"
    25		}
    26	
    27		cfg := s.remoteControlConfig()
    28		if cfg != nil && cfg.AllowArbitraryTarget {
    29			return true, ""
    30		}
    31	
    32		host = strings.TrimSpace(host)
    33		allowed := s.collectAllowedTargets()
    34		for _, a := range allowed {
    35			if targetMatchesAllowEntry(host, a) {
    36				return true, ""
    37			}
    38		}
    39		return false, fmt.Sprintf("target host %q is not allowed; add it to remote_control.allowed_targets (host/IP/CIDR) or enable remote_control.allow_arbitrary_target", host)
    40	}
    41	
    42	func (s *Server) validateRemoteTargetForAgent(agentID, host string, port int) (allow bool, errMsg string) {
    43		match, errMsg := s.matchRemoteEgress(agentID, host, port)
    44		return match != nil, errMsg
    45	}
    46	
    47	func targetMatchesAllowEntry(host, entry string) bool {
    48		entry = strings.TrimSpace(entry)
    49		if entry == "" {
    50			return false
    51		}
    52	
    53		if entry == host {
    54			return true
    55		}
    56	
    57		_, network, err := net.ParseCIDR(entry)
    58		if err != nil {
    59			return false
    60		}
    61	
    62		ip := net.ParseIP(host)
    63		if ip == nil {
    64			return false
    65		}
    66	
    67		return network.Contains(ip)
    68	}
    69	
    70	// remoteControlConfig 安全读取远控配置（容忍 nil）
    71	func (s *Server) remoteControlConfig() *remoteControlConfigView {
    72		if s.cfg == nil || s.cfg.RemoteControl == nil {
    73			return nil
    74		}
    75		return &remoteControlConfigView{
    76			AllowArbitraryTarget: s.cfg.RemoteControl.AllowArbitraryTarget,
    77			AllowedTargets:       s.cfg.RemoteControl.AllowedTargets,
    78			EgressPolicies:       buildRemoteEgressPolicyViews(s.cfg.RemoteControl.EgressPolicies),
    79		}
    80	}
    81	
    82	// remoteControlConfigView 内部只读视图，避免直接暴露 *config.RemoteControlConfig
    83	type remoteControlConfigView struct {
    84		AllowArbitraryTarget bool
    85		AllowedTargets       []string
    86		EgressPolicies       []*remoteEgressPolicyView
    87	}
    88	
    89	type remoteEgressPolicyView struct {
    90		AgentID        string
    91		AllowedTargets []string
    92		AllowedPorts   map[int]struct{}
    93	}
    94	
    95	type remoteEgressMatch struct {
    96		Mode    string
    97		AgentID string
    98		Port    int
    99	}
   100	
   101	// collectAllowedTargets 汇总配置白名单
   102	func (s *Server) collectAllowedTargets() []string {
   103		cfg := s.remoteControlConfig()
   104		if cfg == nil {
   105			return nil
   106		}
   107		return cfg.AllowedTargets
   108	}
   109	
   110	func (s *Server) matchRemoteEgress(agentID, host string, port int) (*remoteEgressMatch, string) {
   111		if allow, errMsg := s.validateRemoteTarget(host); !allow {
   112			return nil, errMsg
   113		}
   114	
   115		cfg := s.remoteControlConfig()
   116		if cfg == nil || len(cfg.EgressPolicies) == 0 {
   117			return &remoteEgressMatch{Mode: "global-allow-list"}, ""
   118		}
   119	
   120		// 多策略命中规则（D-2026-10-12-2）：同一 agent 的多条 egress 策略按
   121		// 「任一策略同时放行目标与端口即放行」合并判定，与配置顺序无关；
   122		// 不跨策略拼凑（A 策略的目标 × B 策略的端口不构成放行）。
   123		targetAllowed := false
   124		agentListed := false
   125		for _, policy := range cfg.EgressPolicies {
   126			if policy == nil || strings.TrimSpace(policy.AgentID) != strings.TrimSpace(agentID) {
   127				continue
   128			}
   129			agentListed = true
   130			if !policy.allowsTarget(host) {
   131				continue
   132			}
   133			targetAllowed = true
   134			if policy.allowsPort(port) {
   135				return &remoteEgressMatch{
   136					Mode:    "agent-egress",
   137					AgentID: policy.AgentID,
   138					Port:    port,
   139				}, ""
   140			}
   141		}
   142	
   143		if !agentListed {
   144			return nil, fmt.Sprintf("agent %q is not allowed for remote egress; add it to remote_control.egress", agentID)
   145		}
   146		if !targetAllowed {
   147			return nil, fmt.Sprintf("target host %q is not allowed for agent %q; add it to remote_control.egress[].allowed_targets", strings.TrimSpace(host), agentID)
   148		}
   149		return nil, fmt.Sprintf("target port %d is not allowed for agent %q; add it to remote_control.egress[].allowed_ports", port, agentID)
   150	}
   151	
   152	func buildRemoteEgressPolicyViews(policies []*config.RemoteEgressPolicy) []*remoteEgressPolicyView {
   153		if len(policies) == 0 {
   154			return nil
   155		}
   156	
   157		result := make([]*remoteEgressPolicyView, 0, len(policies))
   158		for _, policy := range policies {
   159			if policy == nil {
   160				continue
   161			}
   162			view := &remoteEgressPolicyView{
   163				AgentID:        strings.TrimSpace(policy.AgentID),
   164				AllowedTargets: policy.AllowedTargets,
   165				AllowedPorts:   make(map[int]struct{}, len(policy.AllowedPorts)),
   166			}
   167			for _, port := range policy.AllowedPorts {
   168				if port > 0 {
   169					view.AllowedPorts[port] = struct{}{}
   170				}
   171			}
   172			result = append(result, view)
   173		}
   174		return result
   175	}
   176	
   177	func (p *remoteEgressPolicyView) allowsTarget(host string) bool {
   178		if p == nil || len(p.AllowedTargets) == 0 {
   179			return false
   180		}
   181		for _, allowed := range p.AllowedTargets {
   182			if targetMatchesAllowEntry(host, allowed) {
   183				return true
   184			}
   185		}
   186		return false
   187	}
   188	
   189	func (p *remoteEgressPolicyView) allowsPort(port int) bool {
   190		if p == nil || port <= 0 || len(p.AllowedPorts) == 0 {
   191			return false
   192		}
   193		_, ok := p.AllowedPorts[port]
   194		return ok
   195	}
   196	
   197	func (m *remoteEgressMatch) summary() string {
   198		if m == nil {
   199			return ""
   200		}
   201		if m.Mode == "global-allow-list" {
   202			return "global-allow-list"
   203		}
   204		if m.Mode == "agent-egress" {
   205			return fmt.Sprintf("agent:%s port:%d", m.AgentID, m.Port)
   206		}
   207		return ""
   208	}
   209	
   210	// auditRemoteStart 记录远控会话开始审计日志
   211	func (s *Server) auditRemoteStart(action, userID, username, ip, userAgent string, details *audit.RemoteSessionDetails) {
   212		if s.audit == nil || details == nil {
   213			return
   214		}
   215		if err := s.audit.LogRemoteSession(action, userID, username, audit.StatusSuccess, ip, userAgent, details); err != nil {
   216			fmt.Printf("audit remote start failed: %v\n", err)
   217		}
   218	}
   219	
   220	func (s *Server) auditRemoteFailure(userID, username, ip, userAgent string, details *audit.RemoteSessionDetails) {
   221		if s.audit == nil || details == nil {
   222			return
   223		}
   224		if err := s.audit.LogRemoteSession(audit.ActionRemoteStart, userID, username, audit.StatusFailure, ip, userAgent, details); err != nil {
   225			fmt.Printf("audit remote failure failed: %v\n", err)
   226		}
   227	}
   228	
   229	// auditRemoteEnd 记录远控会话结束审计日志
   230	func (s *Server) auditRemoteEnd(userID, username, ip, userAgent string, details *audit.RemoteSessionDetails) {
   231		if s.audit == nil || details == nil {
   232			return
   233		}
   234		if err := s.audit.LogRemoteSession(audit.ActionRemoteEnd, userID, username, audit.StatusSuccess, ip, userAgent, details); err != nil {
   235			fmt.Printf("audit remote end failed: %v\n", err)
   236		}
   237	}
   238	
   239	// rejectTargetDenied 统一构造"目标未授权"的 HTTP 响应
   240	func rejectTargetDenied(w http.ResponseWriter, reason string) {
   241		http.Error(w, reason, http.StatusForbidden)
   242	}
[0m$ [0mrg -n "rate.?limit|RateLimit|failed.*login|login.*attempt|Lockout|Retry-After|captcha|forgot.password|reset.code" internal/auth internal/server internal/storage --glob '*.go'; nl -ba internal/auth/handler.go | sed -n '1,340p'; nl -ba internal/auth/password_reset.go | sed -n '1,320p'
internal/server/password_reset_handlers_test.go:16:	req := httptest.NewRequest("GET", "/api/auth/forgot-password", nil)
internal/server/password_reset_handlers_test.go:26:	req := httptest.NewRequest("POST", "/api/auth/forgot-password", bytes.NewReader([]byte("not json")))
internal/server/password_reset_handlers_test.go:37:	req := httptest.NewRequest("POST", "/api/auth/forgot-password", bytes.NewReader(body))
internal/server/password_reset_handlers_test.go:48:	req := httptest.NewRequest("POST", "/api/auth/forgot-password", bytes.NewReader(body))
internal/server/password_reset_handlers_test.go:61:	req := httptest.NewRequest("POST", "/api/auth/forgot-password", bytes.NewReader(body))
internal/server/password_reset_handlers_test.go:75:	req := httptest.NewRequest("POST", "/api/auth/forgot-password", bytes.NewReader(body))
internal/server/password_reset_handlers_test.go:183:	req := httptest.NewRequest("GET", "/api/auth/verify-reset-code", nil)
internal/server/password_reset_handlers_test.go:193:	req := httptest.NewRequest("POST", "/api/auth/verify-reset-code", bytes.NewReader([]byte("bad")))
internal/server/password_reset_handlers_test.go:204:	req := httptest.NewRequest("POST", "/api/auth/verify-reset-code", bytes.NewReader(body))
internal/server/password_reset_handlers_test.go:221:	req := httptest.NewRequest("POST", "/api/auth/verify-reset-code", bytes.NewReader(body))
internal/server/cov_server_core_test.go:147:		t.Fatalf("failed login = %d, want 401", resp.StatusCode)
internal/server/server.go:335:	mux.HandleFunc("/api/auth/forgot-password", s.handleForgotPassword)
internal/server/server.go:337:	mux.HandleFunc("/api/auth/verify-reset-code", s.handleVerifyResetCode)
internal/server/cov_final_test.go:295:	s.handleForgotPassword(rec, covReq(http.MethodPost, "/api/auth/forgot-password",
internal/server/acme_test.go:338:	f.issueErr = errors.New("rate limited")
     1	package auth
     2	
     3	import (
     4		"encoding/hex"
     5		"encoding/json"
     6		"fmt"
     7		"net/http"
     8		"strings"
     9		"sync"
    10		"time"
    11	
    12		"github.com/cuihairu/cockpit/internal/storage"
    13	)
    14	
    15	// LoginRequest 登录请求
    16	type LoginRequest struct {
    17		Username string `json:"username"`
    18		Password string `json:"password"`
    19	}
    20	
    21	// LoginResponse 登录响应
    22	type LoginResponse struct {
    23		Token        string `json:"token,omitempty"`
    24		ExpiresAt    int64  `json:"expires_at,omitempty"`
    25		UserID       string `json:"user_id"`
    26		Username     string `json:"username"`
    27		Role         string `json:"role,omitempty"`
    28		RequiresTOTP bool   `json:"requires_totp"`
    29		TmpToken     string `json:"tmp_token,omitempty"`
    30	}
    31	
    32	// DB 存储接口（在运行时注入）
    33	var DB *storage.DB
    34	
    35	// InitDB 初始化数据库连接
    36	func InitDB(db *storage.DB) {
    37		DB = db
    38		ensureDefaults()
    39		defaultService.SetDB(db)
    40	}
    41	
    42	// InitAdmin 初始化管理员用户
    43	func InitAdmin(db *storage.DB, username, password string) error {
    44		return db.InitAdminUser(username, password)
    45	}
    46	
    47	// HandleLogin 处理登录
    48	func HandleLogin(w http.ResponseWriter, r *http.Request) {
    49		ensureDefaults()
    50		defaultService.HandleLogin(w, r)
    51	}
    52	
    53	// HandleLogin 处理登录
    54	func (s *Service) HandleLogin(w http.ResponseWriter, r *http.Request) {
    55		if r.Method != http.MethodPost {
    56			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
    57			return
    58		}
    59	
    60		var req LoginRequest
    61		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    62			http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
    63			return
    64		}
    65	
    66		// 验证用户名密码
    67		db := s.DB()
    68		if db == nil {
    69			http.Error(w, `{"error":"Authentication service unavailable"}`, http.StatusInternalServerError)
    70			return
    71		}
    72		user, err := db.VerifyPassword(req.Username, req.Password)
    73		if err != nil {
    74			http.Error(w, `{"error":"Invalid username or password"}`, http.StatusUnauthorized)
    75			return
    76		}
    77	
    78		// 检查是否启用了 TOTP
    79		if user.TOTPEnabled {
    80			tmpToken := generateTmpToken(user.ID)
    81			response := LoginResponse{
    82				UserID:       user.ID,
    83				Username:     user.Username,
    84				RequiresTOTP: true,
    85				TmpToken:     tmpToken,
    86			}
    87			w.Header().Set("Content-Type", "application/json")
    88			json.NewEncoder(w).Encode(response)
    89			return
    90		}
    91	
    92		// 生成 token（signLoginToken 注入点：HS256 + []byte 密钥下
    93		// SignedString 不会失败，错误分支仅供测试覆盖）
    94		token, err := signLoginToken(s, user.ID, user.Username, user.Role)
    95		if err != nil {
    96			http.Error(w, `{"error":"Failed to generate token"}`, http.StatusInternalServerError)
    97			return
    98		}
    99	
   100		// 返回 token
   101		response := LoginResponse{
   102			Token:     token,
   103			ExpiresAt: 0,
   104			UserID:    user.ID,
   105			Username:  user.Username,
   106			Role:      user.Role,
   107		}
   108	
   109		w.Header().Set("Content-Type", "application/json")
   110		json.NewEncoder(w).Encode(response)
   111	}
   112	
   113	// HandleRefresh 处理 token 刷新
   114	func HandleRefresh(w http.ResponseWriter, r *http.Request) {
   115		ensureDefaults()
   116		defaultService.HandleRefresh(w, r)
   117	}
   118	
   119	// HandleRefresh 处理 token 刷新
   120	func (s *Service) HandleRefresh(w http.ResponseWriter, r *http.Request) {
   121		if r.Method != http.MethodPost {
   122			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
   123			return
   124		}
   125	
   126		authHeader := r.Header.Get("Authorization")
   127		if authHeader == "" {
   128			http.Error(w, `{"error":"Authorization header required"}`, http.StatusUnauthorized)
   129			return
   130		}
   131	
   132		if !strings.HasPrefix(authHeader, "Bearer ") {
   133			http.Error(w, `{"error":"Invalid authorization format"}`, http.StatusUnauthorized)
   134			return
   135		}
   136		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
   137	
   138		newToken, err := s.RefreshToken(tokenString)
   139		if err != nil {
   140			http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
   141			return
   142		}
   143	
   144		w.Header().Set("Content-Type", "application/json")
   145		json.NewEncoder(w).Encode(map[string]string{"token": newToken})
   146	}
   147	
   148	// DB 返回认证服务当前数据库。
   149	func (s *Service) DB() *storage.DB {
   150		if s == nil {
   151			return DB
   152		}
   153	
   154		s.mu.RLock()
   155		defer s.mu.RUnlock()
   156		return s.db
   157	}
   158	
   159	// TmpTokenData 临时令牌数据
   160	type TmpTokenData struct {
   161		UserID    string
   162		ExpiresAt time.Time
   163	}
   164	
   165	// 临时令牌存储 (生产环境应使用 Redis)
   166	var tmpTokenStore = make(map[string]*TmpTokenData)
   167	var tmpTokenStoreMutex sync.RWMutex
   168	
   169	// signLoginToken 注入点：生产即 Service.GenerateToken。
   170	var signLoginToken = (*Service).GenerateToken
   171	
   172	// generateTmpToken 生成临时令牌
   173	func generateTmpToken(userID string) string {
   174		// 生成 32 字节随机数
   175		b := make([]byte, 32)
   176		if _, err := randRead(b); err != nil {
   177			// 如果随机数生成失败，使用时间戳作为后备
   178			return fmt.Sprintf("tmp_%d_%s", time.Now().UnixNano(), userID)
   179		}
   180		token := hex.EncodeToString(b)
   181	
   182		tmpTokenStoreMutex.Lock()
   183		defer tmpTokenStoreMutex.Unlock()
   184	
   185		tmpTokenStore[token] = &TmpTokenData{
   186			UserID:    userID,
   187			ExpiresAt: time.Now().Add(5 * time.Minute),
   188		}
   189	
   190		return token
   191	}
   192	
   193	// ValidateTmpToken 验证临时令牌（导出供外部使用）
   194	func ValidateTmpToken(token string) (string, bool) {
   195		tmpTokenStoreMutex.RLock()
   196		defer tmpTokenStoreMutex.RUnlock()
   197	
   198		data, exists := tmpTokenStore[token]
   199		if !exists {
   200			return "", false
   201		}
   202	
   203		// 检查是否过期
   204		if time.Now().After(data.ExpiresAt) {
   205			return "", false
   206		}
   207	
   208		return data.UserID, true
   209	}
   210	
   211	// ConsumeTmpToken 消耗临时令牌（验证后删除，导出供外部使用）
   212	func ConsumeTmpToken(token string) bool {
   213		tmpTokenStoreMutex.Lock()
   214		defer tmpTokenStoreMutex.Unlock()
   215	
   216		data, exists := tmpTokenStore[token]
   217		if !exists {
   218			return false
   219		}
   220	
   221		// 检查是否过期
   222		if time.Now().After(data.ExpiresAt) {
   223			delete(tmpTokenStore, token)
   224			return false
   225		}
   226	
   227		delete(tmpTokenStore, token)
   228		return true
   229	}
     1	package auth
     2	
     3	import (
     4		"bytes"
     5		"crypto/rand"
     6		"encoding/hex"
     7		"errors"
     8		"fmt"
     9		"net/smtp"
    10		"os"
    11		"strings"
    12		"sync"
    13		"time"
    14	
    15		"github.com/cuihairu/cockpit/internal/config"
    16	)
    17	
    18	var (
    19		// ErrResetTokenInvalid 重置令牌无效
    20		ErrResetTokenInvalid = errors.New("invalid or expired reset token")
    21		// ErrEmailNotConfigured 邮件未配置
    22		ErrEmailNotConfigured = errors.New("email service not configured")
    23	)
    24	
    25	// ResetTokenData 重置令牌数据
    26	type ResetTokenData struct {
    27		UserID    string
    28		Email     string
    29		Code      string // 6位验证码
    30		ExpiresAt time.Time
    31	}
    32	
    33	// 重置令牌存储（生产环境应使用 Redis）。
    34	// GenerateResetToken 会起后台清理协程，且 HTTP handler 并发调用，
    35	// 所有读写必须持 resetTokenMu。
    36	var (
    37		resetTokenMu    sync.Mutex
    38		resetTokenStore = make(map[string]*ResetTokenData)
    39	)
    40	
    41	// emailConfig 全局邮件配置
    42	var emailConfig *config.EmailConfig
    43	var emailConfigMutex sync.RWMutex
    44	
    45	// SetEmailConfig 设置邮件配置
    46	func SetEmailConfig(cfg *config.EmailConfig) {
    47		emailConfigMutex.Lock()
    48		defer emailConfigMutex.Unlock()
    49		emailConfig = cfg
    50	}
    51	
    52	// 生成6位数字验证码
    53	func generateVerificationCode() string {
    54		b := make([]byte, 3)
    55		rand.Read(b)
    56		return fmt.Sprintf("%06d", (int(b[0])<<16|int(b[1])<<8|int(b[2]))%1000000)
    57	}
    58	
    59	// GenerateResetToken 生成密码重置令牌和验证码
    60	func GenerateResetToken(userID, email string) (string, string, error) {
    61		// 生成32字节随机令牌
    62		tokenBytes := make([]byte, 32)
    63		if _, err := randRead(tokenBytes); err != nil {
    64			return "", "", err
    65		}
    66		token := hex.EncodeToString(tokenBytes)
    67	
    68		// 生成6位验证码
    69		code := generateVerificationCode()
    70	
    71		// 存储（30分钟有效）
    72		data := &ResetTokenData{
    73			UserID:    userID,
    74			Email:     email,
    75			Code:      code,
    76			ExpiresAt: time.Now().Add(30 * time.Minute),
    77		}
    78		resetTokenMu.Lock()
    79		resetTokenStore[token] = data
    80		resetTokenMu.Unlock()
    81	
    82		// 清理过期令牌
    83		go cleanupExpiredTokens()
    84	
    85		return token, code, nil
    86	}
    87	
    88	// ValidateResetToken 验证重置令牌
    89	func ValidateResetToken(token string) (*ResetTokenData, error) {
    90		resetTokenMu.Lock()
    91		defer resetTokenMu.Unlock()
    92	
    93		data, exists := resetTokenStore[token]
    94		if !exists {
    95			return nil, ErrResetTokenInvalid
    96		}
    97	
    98		if time.Now().After(data.ExpiresAt) {
    99			delete(resetTokenStore, token)
   100			return nil, ErrResetTokenInvalid
   101		}
   102	
   103		return data, nil
   104	}
   105	
   106	// ValidateResetCode 验证重置验证码
   107	func ValidateResetCode(token, code string) (*ResetTokenData, error) {
   108		data, err := ValidateResetToken(token)
   109		if err != nil {
   110			return nil, err
   111		}
   112	
   113		if data.Code != code {
   114			return nil, ErrResetTokenInvalid
   115		}
   116	
   117		return data, nil
   118	}
   119	
   120	// ConsumeResetToken 消费重置令牌（验证后删除）
   121	func ConsumeResetToken(token string) bool {
   122		resetTokenMu.Lock()
   123		defer resetTokenMu.Unlock()
   124	
   125		data, exists := resetTokenStore[token]
   126		if !exists {
   127			return false
   128		}
   129	
   130		if time.Now().After(data.ExpiresAt) {
   131			delete(resetTokenStore, token)
   132			return false
   133		}
   134	
   135		delete(resetTokenStore, token)
   136		_ = data
   137		return true
   138	}
   139	
   140	// cleanupExpiredTokens 清理过期令牌
   141	func cleanupExpiredTokens() {
   142		now := time.Now()
   143	
   144		resetTokenMu.Lock()
   145		defer resetTokenMu.Unlock()
   146		for token, data := range resetTokenStore {
   147			if now.After(data.ExpiresAt) {
   148				delete(resetTokenStore, token)
   149			}
   150		}
   151	}
   152	
   153	// GetEmailConfig 获取邮件配置（优先使用配置文件，否则回退到环境变量）
   154	func GetEmailConfig() *config.EmailConfig {
   155		emailConfigMutex.RLock()
   156		defer emailConfigMutex.RUnlock()
   157	
   158		// 如果已通过配置文件设置，直接返回
   159		if emailConfig != nil && emailConfig.Enabled {
   160			cfg := *emailConfig
   161			cfg.BaseURL = normalizeBaseURL(cfg.BaseURL)
   162			return &cfg
   163		}
   164	
   165		// 回退到环境变量（保持向后兼容）
   166		smtpUser := os.Getenv("SMTP_USER")
   167		smtpPass := os.Getenv("SMTP_PASS")
   168		if smtpUser == "" || smtpPass == "" {
   169			return nil // 邮件未配置
   170		}
   171	
   172		return &config.EmailConfig{
   173			Enabled: true,
   174			SMTP: &config.SMTPConfig{
   175				Host:     getEnvOrDefault("SMTP_HOST", "smtp.gmail.com"),
   176				Port:     parseInt(getEnvOrDefault("SMTP_PORT", "587")),
   177				Username: smtpUser,
   178				Password: smtpPass,
   179				From:     getEnvOrDefault("SMTP_FROM", smtpUser),
   180				FromName: getEnvOrDefault("SMTP_FROM_NAME", "Cockpit"),
   181			},
   182			BaseURL: normalizeBaseURL(getEnvOrDefault("BASE_URL", "http://localhost:9000")),
   183		}
   184	}
   185	
   186	func getEnvOrDefault(key, defaultValue string) string {
   187		if val := os.Getenv(key); val != "" {
   188			return val
   189		}
   190		return defaultValue
   191	}
   192	
   193	func parseInt(s string) int {
   194		var i int
   195		fmt.Sscanf(s, "%d", &i)
   196		return i
   197	}
   198	
   199	// SendPasswordResetEmail 发送密码重置邮件
   200	func SendPasswordResetEmail(email, username, code, token string) error {
   201		cfg := GetEmailConfig()
   202	
   203		// 检查邮件配置
   204		if cfg == nil || !cfg.Enabled || cfg.SMTP == nil || cfg.SMTP.Username == "" || cfg.SMTP.Password == "" {
   205			return ErrEmailNotConfigured
   206		}
   207	
   208		// 构建邮件内容
   209		subject := "重置您的 Cockpit 密码"
   210		baseURL := normalizeBaseURL(cfg.BaseURL)
   211		if baseURL == "" {
   212			baseURL = getBaseURL()
   213		}
   214		resetURL := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)
   215	
   216		body := fmt.Sprintf(`
   217	<!DOCTYPE html>
   218	<html>
   219	<head>
   220		<meta charset="UTF-8">
   221		<style>
   222			body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
   223			.container { max-width: 600px; margin: 0 auto; padding: 20px; }
   224			.button { display: inline-block; padding: 12px 24px; background: #165DFF; color: white; text-decoration: none; border-radius: 4px; }
   225			.code { font-size: 24px; font-weight: bold; letter-spacing: 4px; background: #f5f5f5; padding: 16px; text-align: center; border-radius: 4px; }
   226		</style>
   227	</head>
   228	<body>
   229		<div class="container">
   230			<h2>密码重置请求</h2>
   231			<p>您好，<strong>%s</strong>：</p>
   232			<p>我们收到了您的密码重置请求。您的验证码是：</p>
   233			<div class="code">%s</div>
   234			<p>验证码有效期为 30 分钟。如果这不是您的操作，请忽略此邮件。</p>
   235			<p>或者点击以下链接直接重置密码：</p>
   236			<p><a href="%s" class="button">重置密码</a></p>
   237			<p>如果您无法点击上方按钮，请将以下链接复制到浏览器地址栏：</p>
   238			<p style="word-break: break-all; color: #666; font-size: 12px;">%s</p>
   239			<hr style="margin: 20px 0; border: none; border-top: 1px solid #eee;">
   240			<p style="font-size: 12px; color: #999;">此邮件由系统自动发送，请勿回复。</p>
   241		</div>
   242	</body>
   243	</html>
   244	`, username, code, resetURL, resetURL)
   245	
   246		// 发送邮件
   247		return sendEmail(cfg, []string{email}, subject, body)
   248	}
   249	
   250	// sendEmail 发送邮件
   251	func sendEmail(cfg *config.EmailConfig, to []string, subject, htmlBody string) error {
   252		smtpCfg := cfg.SMTP
   253		auth := smtp.PlainAuth("", smtpCfg.Username, smtpCfg.Password, smtpCfg.Host)
   254	
   255		addr := fmt.Sprintf("%s:%d", smtpCfg.Host, smtpCfg.Port)
   256	
   257		// 构建邮件内容
   258		var content bytes.Buffer
   259		content.WriteString(fmt.Sprintf("From: %s <%s>\r\n", smtpCfg.FromName, smtpCfg.From))
   260		content.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
   261		content.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
   262		content.WriteString("MIME-Version: 1.0\r\n")
   263		content.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
   264		content.WriteString(htmlBody)
   265	
   266		return smtp.SendMail(addr, auth, smtpCfg.From, to, content.Bytes())
   267	}
   268	
   269	// getBaseURL 获取基础 URL（用于生成重置链接）
   270	func getBaseURL() string {
   271		if baseURL := os.Getenv("BASE_URL"); baseURL != "" {
   272			return normalizeBaseURL(baseURL)
   273		}
   274		return "http://localhost:9000" // 默认本地地址
   275	}
   276	
   277	func normalizeBaseURL(baseURL string) string {
   278		return strings.TrimSuffix(baseURL, "/")
   279	}
   280	
   281	// MaskEmail 脱敏邮箱地址
   282	func MaskEmail(email string) string {
   283		parts := strings.Split(email, "@")
   284		if len(parts) != 2 {
   285			return email
   286		}
   287		username := parts[0]
   288		domain := parts[1]
   289	
   290		if len(username) <= 3 {
   291			return email
   292		}
   293	
   294		// 只显示首字母和最后一位，中间用***代替
   295		maskedUsername := string(username[0]) + "***" + string(username[len(username)-1])
   296		return maskedUsername + "@" + domain
   297	}
   298	
   299	// ForgotPasswordRequest 忘记密码请求
   300	type ForgotPasswordRequest struct {
   301		Username string `json:"username"`
   302	}
   303	
   304	// ForgotPasswordResponse 忘记密码响应
   305	type ForgotPasswordResponse struct {
   306		Email       string `json:"email"`        // 脱敏邮箱
   307		MaskedEmail string `json:"masked_email"` // 脱敏邮箱
   308		Message     string `json:"message"`
   309	}
   310	
   311	// ResetPasswordRequest 重置密码请求
   312	type ResetPasswordRequest struct {
   313		Token       string `json:"token"`
   314		Code        string `json:"code"` // 可选，用于验证码验证
   315		NewPassword string `json:"new_password"`
   316	}
[0m$ [0mnl -ba internal/auth/password_reset.go | sed -n '299,500p'; nl -ba internal/server/password_reset_handlers.go | sed -n '1,300p'; nl -ba internal/server/api_remote.go | sed -n '375,540p'; nl -ba internal/server/api_guacamole.go | sed -n '300,380p'; nl -ba internal/server/ticket.go | sed -n '55,120p'; nl -ba internal/server/websocket.go | sed -n '125,205p'
299	// ForgotPasswordRequest 忘记密码请求
   300	type ForgotPasswordRequest struct {
   301		Username string `json:"username"`
   302	}
   303	
   304	// ForgotPasswordResponse 忘记密码响应
   305	type ForgotPasswordResponse struct {
   306		Email       string `json:"email"`        // 脱敏邮箱
   307		MaskedEmail string `json:"masked_email"` // 脱敏邮箱
   308		Message     string `json:"message"`
   309	}
   310	
   311	// ResetPasswordRequest 重置密码请求
   312	type ResetPasswordRequest struct {
   313		Token       string `json:"token"`
   314		Code        string `json:"code"` // 可选，用于验证码验证
   315		NewPassword string `json:"new_password"`
   316	}
     1	package server
     2	
     3	import (
     4		"encoding/json"
     5		"net/http"
     6	
     7		"github.com/cuihairu/cockpit/internal/auth"
     8		"github.com/cuihairu/cockpit/internal/storage"
     9	)
    10	
    11	// handleForgotPassword 处理忘记密码请求
    12	func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
    13		if r.Method != http.MethodPost {
    14			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
    15			return
    16		}
    17	
    18		var req auth.ForgotPasswordRequest
    19		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    20			http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
    21			return
    22		}
    23	
    24		if req.Username == "" {
    25			http.Error(w, `{"error":"Username is required"}`, http.StatusBadRequest)
    26			return
    27		}
    28	
    29		// 查找用户
    30		user, err := s.db.GetUserByUsername(req.Username)
    31		if err != nil {
    32			// 为了安全，即使用户不存在也返回成功消息
    33			w.Header().Set("Content-Type", "application/json")
    34			json.NewEncoder(w).Encode(map[string]string{
    35				"message": "如果该用户存在，重置邮件已发送",
    36			})
    37			return
    38		}
    39	
    40		// 检查用户是否有邮箱
    41		if user.Email == "" {
    42			w.Header().Set("Content-Type", "application/json")
    43			json.NewEncoder(w).Encode(map[string]string{
    44				"message": "该账户未绑定邮箱，请联系管理员重置密码",
    45			})
    46			return
    47		}
    48	
    49		// 生成重置令牌和验证码
    50		token, code, err := authGenerateResetToken(user.ID, user.Email)
    51		if err != nil {
    52			http.Error(w, `{"error":"Failed to generate reset token"}`, http.StatusInternalServerError)
    53			return
    54		}
    55	
    56		// 发送邮件（异步，不阻塞响应）
    57		go func() {
    58			if err := auth.SendPasswordResetEmail(user.Email, user.Username, code, token); err != nil {
    59				// 记录错误但不暴露给用户
    60				printf("Failed to send reset email: %v", err)
    61			}
    62		}()
    63	
    64		// 返回脱敏邮箱
    65		w.Header().Set("Content-Type", "application/json")
    66		json.NewEncoder(w).Encode(auth.ForgotPasswordResponse{
    67			Email:       user.Email,
    68			MaskedEmail: auth.MaskEmail(user.Email),
    69			Message:     "重置邮件已发送到您的邮箱",
    70		})
    71	}
    72	
    73	// handleResetPassword 处理密码重置请求
    74	func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
    75		if r.Method != http.MethodPost {
    76			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
    77			return
    78		}
    79	
    80		var req auth.ResetPasswordRequest
    81		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    82			http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
    83			return
    84		}
    85	
    86		if req.Token == "" || req.NewPassword == "" {
    87			http.Error(w, `{"error":"Token and new password are required"}`, http.StatusBadRequest)
    88			return
    89		}
    90	
    91		if len(req.NewPassword) < 6 {
    92			http.Error(w, `{"error":"Password must be at least 6 characters"}`, http.StatusBadRequest)
    93			return
    94		}
    95	
    96		// 验证令牌
    97		data, err := auth.ValidateResetToken(req.Token)
    98		if err != nil {
    99			http.Error(w, `{"error":"Invalid or expired reset token"}`, http.StatusUnauthorized)
   100			return
   101		}
   102	
   103		// 如果提供了验证码，也需要验证
   104		if req.Code != "" {
   105			_, err = auth.ValidateResetCode(req.Token, req.Code)
   106			if err != nil {
   107				http.Error(w, `{"error":"Invalid verification code"}`, http.StatusUnauthorized)
   108				return
   109			}
   110		}
   111	
   112		// 哈希新密码
   113		hashedPassword, err := storage.HashPassword(req.NewPassword)
   114		if err != nil {
   115			http.Error(w, `{"error":"Failed to process password"}`, http.StatusInternalServerError)
   116			return
   117		}
   118	
   119		// 更新密码
   120		if err := s.db.UpdatePassword(data.UserID, hashedPassword); err != nil {
   121			http.Error(w, `{"error":"Failed to update password"}`, http.StatusInternalServerError)
   122			return
   123		}
   124	
   125		// 消费令牌（一次性使用）
   126		auth.ConsumeResetToken(req.Token)
   127	
   128		w.Header().Set("Content-Type", "application/json")
   129		json.NewEncoder(w).Encode(map[string]string{
   130			"message": "密码已成功重置，请使用新密码登录",
   131		})
   132	}
   133	
   134	// handleVerifyResetCode 验证重置验证码
   135	func (s *Server) handleVerifyResetCode(w http.ResponseWriter, r *http.Request) {
   136		if r.Method != http.MethodPost {
   137			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
   138			return
   139		}
   140	
   141		var req struct {
   142			Token string `json:"token"`
   143			Code  string `json:"code"`
   144		}
   145		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
   146			http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
   147			return
   148		}
   149	
   150		// 验证令牌和验证码
   151		_, err := auth.ValidateResetCode(req.Token, req.Code)
   152		if err != nil {
   153			w.Header().Set("Content-Type", "application/json")
   154			json.NewEncoder(w).Encode(map[string]interface{}{
   155				"valid": false,
   156				"error": "Invalid or expired code",
   157			})
   158			return
   159		}
   160	
   161		w.Header().Set("Content-Type", "application/json")
   162		json.NewEncoder(w).Encode(map[string]interface{}{
   163			"valid":   true,
   164			"message": "验证码正确",
   165		})
   166	}
   167	
   168	// printf 打印日志（替代 log.Printf）
   169	func printf(format string, args ...interface{}) {
   170		// 在实际实现中应该使用日志系统
   171	}
   375			"type":    "close",
   376			"message": reason,
   377		})
   378		session.ClientWS.Close()
   379		log.Printf("Terminal connection closed: %s, reason: %s", connID, reason)
   380		// 台账收口（agent 先断时立即回填，不等 keepalive 兜底）
   381		_ = s.remoteSessions.UpdateStatus(session.ID, RemoteSessionStatusClosed, "")
   382	}
   383	
   384	// registerRemoteAPI 注册远程连接 API
   385	func (s *Server) registerRemoteAPI(mux *http.ServeMux) {
   386		mux.HandleFunc("/api/remote/tickets", s.authService().Middleware(s.handleTicketCreate))
   387		mux.HandleFunc("/api/remote/terminal", s.handleTerminalWebSocket)
   388		mux.HandleFunc("/api/remote/sessions", s.authService().Middleware(s.handleRemoteSessions))
   389		mux.HandleFunc("/api/remote/sessions/", s.authService().Middleware(s.handleRemoteSession))
   390	}
   391	
   392	// handleTicketCreate 创建短期 WebSocket 连接票据
   393	func (s *Server) handleTicketCreate(w http.ResponseWriter, r *http.Request) {
   394		if r.Method != http.MethodPost {
   395			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
   396			return
   397		}
   398	
   399		var req struct {
   400			AgentID    string `json:"agent_id"`
   401			Host       string `json:"host"`
   402			Port       int    `json:"port"`
   403			Protocol   string `json:"protocol"` // ssh, telnet, vnc, rdp
   404			Username   string `json:"username,omitempty"`
   405			Password   string `json:"password,omitempty"`    // SSH 口令 / VNC 密码等
   406			PrivateKey string `json:"private_key,omitempty"` // SSH PEM 私钥（优先于 Password）
   407			Domain     string `json:"domain,omitempty"`
   408			Width      int    `json:"width,omitempty"`
   409			Height     int    `json:"height,omitempty"`
   410			UseSaved   bool   `json:"use_saved,omitempty"` // 用保险箱已存凭据填充缺失项（egress 校验照旧先行）
   411		}
   412	
   413		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
   414			http.Error(w, "Invalid request body", http.StatusBadRequest)
   415			return
   416		}
   417	
   418		userInfo, _ := auth.GetUserFromContext(r)
   419		clientIP := s.getClientIP(r)
   420		userAgent := r.UserAgent()
   421	
   422		if req.AgentID == "" || req.Host == "" || req.Port <= 0 || req.Protocol == "" {
   423			s.auditRemoteFailure(
   424				userInfo.UserID,
   425				userInfo.Username,
   426				clientIP,
   427				userAgent,
   428				&audit.RemoteSessionDetails{
   429					Protocol: req.Protocol,
   430					AgentID:  req.AgentID,
   431					Host:     req.Host,
   432					Port:     req.Port,
   433					Reason:   "missing required fields",
   434				},
   435			)
   436			http.Error(w, "Missing required fields", http.StatusBadRequest)
   437			return
   438		}
   439	
   440		// 检查 Agent 是否在线
   441		if _, exists := s.registry.Get(req.AgentID); !exists {
   442			s.auditRemoteFailure(
   443				userInfo.UserID,
   444				userInfo.Username,
   445				clientIP,
   446				userAgent,
   447				&audit.RemoteSessionDetails{
   448					Protocol: req.Protocol,
   449					AgentID:  req.AgentID,
   450					Host:     req.Host,
   451					Port:     req.Port,
   452					Reason:   "agent not found or offline",
   453				},
   454			)
   455			http.Error(w, "Agent not found or offline", http.StatusNotFound)
   456			return
   457		}
   458	
   459		// 校验目标是否在 allow-list 和 Agent 出口策略内（除非显式允许任意目标）
   460		egressMatch, reason := s.matchRemoteEgress(req.AgentID, req.Host, req.Port)
   461		if egressMatch == nil {
   462			s.auditRemoteFailure(
   463				userInfo.UserID,
   464				userInfo.Username,
   465				clientIP,
   466				userAgent,
   467				&audit.RemoteSessionDetails{
   468					Protocol: req.Protocol,
   469					AgentID:  req.AgentID,
   470					Host:     req.Host,
   471					Port:     req.Port,
   472					Reason:   reason,
   473				},
   474			)
   475			rejectTargetDenied(w, reason)
   476			return
   477		}
   478	
   479		if userInfo.UserID == "" {
   480			http.Error(w, "Unauthorized", http.StatusUnauthorized)
   481			return
   482		}
   483	
   484		// 保险箱注入（use_saved）：egress 校验已先行通过，凭据只填充请求里
   485		// 缺失的字段——现场显式传入的值优先。解密结果不进日志/审计。
   486		authSource := ""
   487		if req.UseSaved {
   488			cred, err := s.db.GetRemoteCredential(userInfo.UserID, req.AgentID, req.Host, req.Protocol, req.Port)
   489			if err != nil {
   490				s.auditRemoteFailure(
   491					userInfo.UserID,
   492					userInfo.Username,
   493					clientIP,
   494					userAgent,
   495					&audit.RemoteSessionDetails{
   496						Protocol: req.Protocol,
   497						AgentID:  req.AgentID,
   498						Host:     req.Host,
   499						Port:     req.Port,
   500						Egress:   egressMatch.summary(),
   501						Reason:   "no saved credential for this target",
   502					},
   503				)
   504				http.Error(w, "No saved credential for this target", http.StatusBadRequest)
   505				return
   506			}
   507			if req.Username == "" {
   508				req.Username = cred.Username
   509			}
   510			if req.Password == "" {
   511				req.Password = cred.Password
   512			}
   513			if req.PrivateKey == "" {
   514				req.PrivateKey = cred.PrivateKey
   515			}
   516			if req.Domain == "" {
   517				req.Domain = cred.Domain
   518			}
   519			authSource = "saved"
   520		} else if req.Password != "" || req.PrivateKey != "" {
   521			authSource = "user"
   522		}
   523	
   524		// 构建票据参数
   525		params := map[string]string{
   526			"agent_id": req.AgentID,
   527			"host":     req.Host,
   528			"port":     strconv.Itoa(req.Port),
   529			"protocol": req.Protocol,
   530		}
   531		if req.Username != "" {
   532			params["username"] = req.Username
   533		}
   534		if req.Password != "" {
   535			params["password"] = req.Password
   536		}
   537		if req.PrivateKey != "" {
   538			params["private_key"] = req.PrivateKey
   539		}
   540		if req.Domain != "" {
   300	//     subprotocol 传票据——这是上游实现约束，不是自由发挥；
   301	//   - Sec-WebSocket-Protocol[0]（兼容位）：自定义 Tunnel 或不走 common-js 时
   302	//     可沿用 terminal/desktop 的同款形状。
   303	//
   304	// 票据仍是一次性 5 分钟票据，泄漏窗口不变；WS 路径不进通用审计
   305	// （AuditMiddleware 跳过），由专门的远控审计覆盖。
   306	func (s *Server) handleGuacamoleWebSocket(w http.ResponseWriter, r *http.Request) {
   307		// 1. 票据：query 优先，子协议兜底（复用 ticket.go，ValidateTicket 即消费）
   308		ticketID := r.URL.Query().Get("ticket")
   309		if ticketID == "" {
   310			if protocols := r.Header.Values("Sec-WebSocket-Protocol"); len(protocols) > 0 {
   311				ticketID = protocols[0]
   312			}
   313		}
   314		if ticketID == "" {
   315			http.Error(w, `{"error":"Missing ticket"}`, http.StatusUnauthorized)
   316			return
   317		}
   318		ticket, ok := s.ticketMgr.ValidateTicket(ticketID)
   319		if !ok {
   320			http.Error(w, `{"error":"Invalid or expired ticket"}`, http.StatusUnauthorized)
   321			return
   322		}
   323	
   324		agentID := ticket.Params["agent_id"]
   325		host := ticket.Params["host"]
   326		portStr := ticket.Params["port"]
   327		protocolStr := ticket.Params["protocol"]
   328		width, _ := strconv.Atoi(ticket.Params["width"])
   329		height, _ := strconv.Atoi(ticket.Params["height"])
   330	
   331		if agentID == "" || host == "" || portStr == "" {
   332			http.Error(w, `{"error":"Missing connection parameters"}`, http.StatusBadRequest)
   333			return
   334		}
   335		port, err := strconv.Atoi(portStr)
   336		if err != nil || port <= 0 || port > 65535 {
   337			http.Error(w, `{"error":"Invalid port"}`, http.StatusBadRequest)
   338			return
   339		}
   340		// Guacamole 承接 rdp/vnc/ssh 三协议（docs/remote-access-integration-design.md
   341		// D1）；telnet 走 TerminalModal（xterm.js）
   342		if protocolStr != "rdp" && protocolStr != "vnc" && protocolStr != "ssh" {
   343			http.Error(w, `{"error":"Unsupported protocol for Guacamole tunnel"}`, http.StatusBadRequest)
   344			return
   345		}
   346	
   347		// 2. 出口策略（复用 matchRemoteEgress：allow-list / CIDR / per-agent）
   348		egressMatch, reason := s.matchRemoteEgress(agentID, host, port)
   349		if egressMatch == nil {
   350			s.auditRemoteFailure(ticket.UserID, ticket.Username, r.RemoteAddr, r.UserAgent(),
   351				&audit.RemoteSessionDetails{
   352					Protocol: protocolStr,
   353					AgentID:  agentID,
   354					Host:     host,
   355					Port:     port,
   356					Reason:   reason,
   357				})
   358			rejectTargetDenied(w, reason)
   359			return
   360		}
   361	
   362		// 3. 拨 guacd TCP（不暴露 guacd 端口给浏览器——网关反代，见设计 D2）
   363		sessionID := uuid.New().String()
   364		guacdConn, err := dialGuacd(guacdAddr(), guacdDialTimeout)
   365		if err != nil {
   366			log.Printf("Guacamole: dial guacd %s failed: %v", guacdAddr(), err)
   367			s.auditRemoteFailure(ticket.UserID, ticket.Username, r.RemoteAddr, r.UserAgent(),
   368				&audit.RemoteSessionDetails{
   369					Protocol: protocolStr,
   370					AgentID:  agentID,
   371					Host:     host,
   372					Port:     port,
   373					Egress:   egressMatch.summary(),
   374					Reason:   "guacd unreachable",
   375				})
   376			http.Error(w, `{"error":"Guacamole daemon unavailable"}`, http.StatusBadGateway)
   377			return
   378		}
   379	
   380		// 3.5 目标转发器：guacd 拨回环中继，字节流经 agent WS 通道到 agent、
    55			UserID:    userID,
    56			Username:  username,
    57			Params:    params,
    58			ExpiresAt: time.Now().Add(5 * time.Minute),
    59			Consumed:  false,
    60		}
    61	
    62		tm.tickets[ticketID] = ticket
    63		log.Printf("Ticket generated: %s for user %s", ticketID, username)
    64	
    65		return ticket, nil
    66	}
    67	
    68	// ValidateTicket 验证并消费票据（单次使用）
    69	func (tm *TicketManager) ValidateTicket(ticketID string) (*Ticket, bool) {
    70		tm.mu.Lock()
    71		defer tm.mu.Unlock()
    72	
    73		ticket, exists := tm.tickets[ticketID]
    74		if !exists {
    75			return nil, false
    76		}
    77	
    78		ticket.mu.Lock()
    79		defer ticket.mu.Unlock()
    80	
    81		// 检查是否已消费
    82		if ticket.Consumed {
    83			return nil, false
    84		}
    85	
    86		// 检查是否过期
    87		if time.Now().After(ticket.ExpiresAt) {
    88			delete(tm.tickets, ticketID)
    89			return nil, false
    90		}
    91	
    92		// 标记为已消费
    93		ticket.Consumed = true
    94	
    95		return ticket, true
    96	}
    97	
    98	// cleanupLoop 定期清理过期票据
    99	func (tm *TicketManager) cleanupLoop() {
   100		ticker := time.NewTicker(1 * time.Minute)
   101		defer ticker.Stop()
   102	
   103		for range ticker.C {
   104			tm.mu.Lock()
   105			now := time.Now()
   106			for id, ticket := range tm.tickets {
   107				if now.After(ticket.ExpiresAt) {
   108					delete(tm.tickets, id)
   109				}
   110			}
   111			tm.mu.Unlock()
   112		}
   113	}
   125	var wsRegistryLookup = func(r *Registry, agentID string) (*Agent, bool) {
   126		return r.Get(agentID)
   127	}
   128	
   129	func (s *Server) registerAgentConnection(conn *websocket.Conn, reg *protocol.RegisterPayload) (*Agent, *registrationRejection, error) {
   130		if reg.AgentID == "" {
   131			reg.AgentID = protocol.GenerateIDWithPrefix("agent")
   132		}
   133	
   134		existingAgent, rejection, err := s.authenticateAgentRegistration(reg)
   135		if err != nil {
   136			return nil, nil, err
   137		}
   138		if rejection != nil {
   139			return nil, rejection, nil
   140		}
   141	
   142		// Registry 中的 Agent 均为活跃连接，已有记录时拒绝重复连接。
   143		if _, exists := wsRegistryLookup(s.registry, reg.AgentID); exists {
   144			return nil, &registrationRejection{
   145				code:    "duplicate_connection",
   146				message: "Agent already connected",
   147			}, nil
   148		}
   149	
   150		agent := NewAgent(reg.AgentID, conn)
   151		agent.Update(reg)
   152	
   153		if err := s.registry.Register(agent); err != nil {
   154			return nil, &registrationRejection{
   155				code:    "registration_failed",
   156				message: err.Error(),
   157			}, nil
   158		}
   159	
   160		if err := s.persistRegisteredAgent(agent, existingAgent); err != nil {
   161			log.Printf("Failed to persist agent to database: %v", err)
   162		}
   163	
   164		return agent, nil, nil
   165	}
   166	
   167	func (s *Server) authenticateAgentRegistration(reg *protocol.RegisterPayload) (*storage.Agent, *registrationRejection, error) {
   168		existingAgent, err := s.db.GetAgent(reg.AgentID)
   169		if err != nil {
   170			if errors.Is(err, storage.ErrNotFound) {
   171				return nil, nil, nil
   172			}
   173			return nil, nil, fmt.Errorf("load existing agent %s: %w", reg.AgentID, err)
   174		}
   175	
   176		if existingAgent.SecretHash == "" {
   177			// 兼容历史数据：已有 Agent 但未配置密钥时仍允许注册。
   178			log.Printf("Agent %s exists but has no secret configured", reg.AgentID)
   179			return existingAgent, nil, nil
   180		}
   181	
   182		if reg.Secret == "" {
   183			return existingAgent, &registrationRejection{
   184				code:    "authentication_required",
   185				message: "Agent secret is required",
   186			}, nil
   187		}
   188		if !storage.VerifyAgentSecret(existingAgent.SecretHash, reg.Secret) {
   189			return existingAgent, &registrationRejection{
   190				code:    "authentication_failed",
   191				message: "Invalid agent secret",
   192			}, nil
   193		}
   194	
   195		return existingAgent, nil, nil
   196	}
   197	
   198	func (s *Server) persistRegisteredAgent(agent *Agent, existingAgent *storage.Agent) error {
   199		storageAgent := toStorageAgent(agent)
   200		if existingAgent != nil && existingAgent.SecretHash != "" {
   201			storageAgent.SecretHash = existingAgent.SecretHash
   202		}
   203		return s.db.UpsertAgent(storageAgent)
   204	}
   205
[0m$ [0mnl -ba docs/审计-文档一致性.md | sed -n '1,190p'; nl -ba docs/audit-doc-consistency.en.md | sed -n '1,190p'; nl -ba docs/guide/recording-design.md | sed -n '1,220p'
由 agent 拨出，agent 在数据路径上 | 描述不符 | P1 | 改 guacd 中继架构 |
   114	| D10 | docs/remote-access-integration-design.md:91 | RBAC `remote` 资源域 | `terminal` 资源域（`/api/remote/*` 统一归并） | 描述不符 | P2 | 改资源域 |
   115	| D11 | docs/remote-access-integration-design.md:180 | 真机验收挂起待验 | 2026-09-30 已通过 | 码超前于文档 | P2 | 改验收状态 |
   116	
   117	### 束二：后端核心（agent-core / service-detect / external-session / agent-config / guacd / openwrt / reference / vault / persona / nas / file-manager / egress / recording / workflow / deploy-docker）
   118	
   119	| ID | 文档页 | 主张 | 源码证据 | 类别 | 严重度 | 处置 |
   120	| --- | --- | --- | --- | --- | --- | --- |
   121	| D12 | design/agent-core-architecture.md:6 | 状态 Proposed | P1-P5 已落地 | 码超前于文档 | P1 | 改状态为已落地 |
   122	| D13 | design/agent-core-architecture.md:48 | execlog 新建 | `core/execlog` 已有（环 + 攒批上行） | 码超前于文档 | P2 | 改已有 |
   123	| D14 | design/agent-core-architecture.md:113 | build-openwrt 3 架 | 5 架（mipsel/mips/aarch64_generic/arm_cortex-a7_neon-vfpv4/x86_64） | 码超前于文档 | P2 | 改 5 架 |
   124	| D15 | design/agent-core-architecture.md:134 | 批次表无状态列 | P1-P5 全部已落地 | 码超前于文档 | P2 | 补状态列 |
   125	| D16 | design/agent-core-architecture.md:16 | agent.go 778 行 | 696 行 | 码超前于文档 | P2 | 改行数 |
   126	| D17 | design/agent-core-architecture.md:104 | main 112 行 | 116 行 | 码超前于文档 | P2 | 改行数 |
   127	| D18 | design/service-detect-agent.md:64 | Window 结构无 Semantics 字段 | `core/healthprobe/window.go` 含 `Semantics` 字段 | 码超前于文档 | P2 | 补字段 |
   128	| D19 | design/service-detect-agent.md:77 | `cockpit-probe-agent status` 子命令 | `-status-file` 落 JSON（无 `status` 子命令） | 描述不符 | P2 | 改 flag 语义 |
   129	| D20 | design/external-session-api.md:42 | 白名单 ref `api_remote.go:403` | `handleRemoteSessionCreate` 白名单在 `api_remote.go:654-660` | 描述不符 | P2 | 改行号 |
   130	| D21 | docs/agent-config.md:30 | `BIAS`/`COCKPIT_AGENT_OPTS` 存在 | 全仓无此二变量 | 文档超前于码 | P2 | 删行 |
   131	| D22 | docs/agent-config.md:30 | `SERVER_URL` 有默认值 | 必填，无代码默认 | 描述不符 | P2 | 改必填 |
   132	| D23 | docs/agent-config.md:30 | REGION/ZONE 无说明 | 二进制直读 `COCKPIT_REGION`/`COCKPIT_ZONE`（`core/register/register.go:97-104`） | 码超前于文档 | P2 | 补说明 |
   133	| D24 | docs/agent-config.md:61 | Agent 以 root 运行 | systemd 以 `cockpit` 用户运行（`ProtectHome=true`） | 描述不符 | P2 | 改运行用户 |
   134	| D25 | docs/guacd-deploy.md:72 | Server 总是调用 `ssh.getDefaultKey` | 仅当既未填私钥也未填口令时才调用 | 描述不符 | P2 | 改条件 |
   135	| D26 | docs/guide/openwrt-agent-design.md:27 | 依赖闭包缺 core/ 十包 | `internal/agent/rdp` 已删；core/ 十包已加 | 码超前于文档 | P2 | 补依赖闭包 |
   136	| D27 | docs/guide/reference-projects.md:134 | uptimekuma.co 官方链接 | 非官方粉丝站，GitHub 为唯一官方来源 | 描述不符 | P2 | 改 GitHub only |
   137	| D28 | docs/guide/vault-design.md:15 | ACME 私钥 DB 明文 | DB 加密（`serializer:acmeprivkey`，V6 第二波已收敛） | 码超前于文档 | P1 | 改加密 |
   138	| D29 | docs/guide/vault-design.md:15 | inventory capability token 明文 | DB 加密（`serializer:capconf`） | 码超前于文档 | P1 | 改加密 |
   139	| D30 | docs/guide/vault-design.md:131 | V6 第二波未落地 | 已落地 | 码超前于文档 | P2 | 改已落地 |
   140	| D31 | docs/guide/persona-vault-mapping.md:67 | 429 无详细说明 | `rate limited (120 req/min per token)`，不自动重试 | 码超前于文档 | P2 | 补说明 |
   141	| D32 | docs/guide/nas-design.md:71 | 凭据只存 agent 环境变量 | 第一来源 = server vault `nas/<name>`（加密落库），env 仅回退 | 码超前于文档 | P1 | 改凭据来源 |
   142	| D33 | docs/guide/nas-design.md:82 | 仅 `dsm` 类型 | 支持 `dsm`/`truenas`/`omv` | 码超前于文档 | P2 | 补类型 |
   143	| D34 | docs/guide/nas-design.md:218 | 告警恢复语义 | `createAlertIfNotExists` 只新增不清理 | 描述不符 | P2 | 改恢复语义 |
   144	| D35 | docs/guide/file-manager-design.md:50 | `maxWrite`/`maxUpload` | `maxReadChunk`/`maxWriteChunk`（10MB 上传上限是 web 侧） | 描述不符 | P2 | 改字段名 |
   145	| D36 | docs/guide/agent-egress-sdwan.md:49 | overlay 探测含 Cloudflare Tunnel | 仅 zerotier/tailscale/wireguard，无 cloudflare | 描述不符 | P2 | 改探测范围 |
   146	| D37 | docs/guide/agent-egress-sdwan.md:60 | Web SSH 不是完整 SSH 客户端 | `internal/proxy/ssh_session.go` 实现完整 SSH client | 码超前于文档 | P1 | 改完整客户端 |
   147	| D38 | docs/guide/agent-egress-sdwan.md:111 | SSH 用 `ssh_provider.go` | `ssh_session.go`（`ssh_provider.go` 仅提供默认密钥） | 描述不符 | P2 | 改文件引用 |
   148	| D39 | docs/guide/recording-design.md:25 | HandleTerminalData 行号 302 | 341 | 描述不符 | P2 | 改行号 |
   149	| D40 | docs/guide/recording-design.md:34 | D1 判定 VNC/RDP 不录 | M3 已落地 guacd 全协议录制 | 码超前于文档 | P1 | 注 M3 推翻 + 新增 M3 节 |
   150	| D41 | docs/guide/workflow-design.md:46 | approvals 权限点已登记 | 路由已挂 `approvals` 但 `resourceActions` 无该键，全角色 403 | 码超前于文档 | P1 | 补登记 + 回归测试 |
   151	| D42 | docs/operations/deploy-docker.md:11 | compose 无额外端口映射 | 根 compose 额外映射 `9090:9000` | 码超前于文档 | P2 | 补映射 |
   152	| D43 | docs/operations/deploy-docker.md:53 | tag 表含 `v1.2.3` | `docker/metadata-action` 剥 `v` 前缀 | 描述不符 | P2 | 删 `v` 前缀 |
   153	| D44 | docs/operations/deploy-docker.md:84 | `ALLOWED_ORIGINS` 必填 | 未设置时 WS 接受所有 Origin（打 warning），不阻止启动 | 描述不符 | P2 | 改可选 |
   154	| D45 | docs/operations/deploy-docker.md:95 | `AGENT_EXPIRE_MINUTES` 已接线 | compose 未接线该变量 | 描述不符 | P2 | 注未接线 |
   155	| D46 | docs/operations/deploy-docker.md:150 | nginx conf 完整可用 | 模板含已删除的 desktop/vnc 端点 | 描述不符 | P2 | 改模板说明 |
   156	
   157	### 束三：协议 / jobs / cron
   158	
   159	| ID | 文档页 | 主张 | 源码证据 | 类别 | 严重度 | 处置 |
   160	| --- | --- | --- | --- | --- | --- | --- |
   161	| D47 | guide/jobs-design.md:15 | Jobs 页 15s 轮询 | `refetchInterval: 5000`（5s） | 描述不符 | **P1** | 改 5s |
   162	| D48 | guide/jobs-design.md:77 | POST /api/jobs 同步返回终态 | 201 返回 pending，后台派发终态 | 描述不符 | P2 | 改异步语义 |
   163	| D49 | guide/jobs-design.md:75 | 无 cancel 端点 | `POST /api/jobs/{id}/cancel` 存在 | 码超前于文档 | P2 | 补端点 |
   164	| D50 | guide/jobs-design.md:114 | jobs.ts 无 cancelJob | `cancelJob` 存在 | 码超前于文档 | P2 | 补方法 |
   165	| D51 | guide/cron-design.md:36 | @ 白名单 6 个 | 7 个（含 `@annually`） | 码超前于文档 | P2 | 补 @annually |
   166	| D52 | guide/cron-design.md:120 | 预设「每周一 03:00」 | `每周一 04:00`（`0 4 * * 1`） | 描述不符 | P2 | 改 04:00 |
   167	| D53 | guide/cron-design.md:120 | 预设 5 项 | 7 项（含每 5 分钟/每月 1 日 05:00/开机时） | 码超前于文档 | P2 | 补 3 项 |
   168	| D54 | guide/cron-design.md:161 | `Call` case `timers` 在 cron_timers.go | 在 `cron_provider.go:168`；方法名 `ListTimers` | 描述不符 | P2 | 改归属 + 方法名 |
   169	| D55 | guide/cron-design.md:191 | `Call` case `users` 在 cron_users.go | 在 `cron_provider.go:166` | 描述不符 | P2 | 改归属 |
   170	| D56 | guide/cron-design.md:32 | `crontab -` 写回（stdin） | `crontab <临时文件>`（argv 传路径，无 stdin） | 描述不符 | P2 | 改写回方式 |
   171	
   172	### 束四：stack-deploy / drift
   173	
   174	| ID | 文档页 | 主张 | 源码证据 | 类别 | 严重度 | 处置 |
   175	| --- | --- | --- | --- | --- | --- | --- |
   176	| D57 | guide/stack-deploy-design.md:60 | `last-deploy.log` 存任务输出 | 实际落盘为 `.cockpit/last-task.json`（仅元数据）；日志仅内存环形缓冲 | 文档超前于码 | **P1** | 改 last-task.json |
   177	| D58 | guide/stack-deploy-design.md:138 | 审计事件名 `stack.create` 等 | `stack_create`/`stack_update`/`stack_up`/`stack_down`/`stack_remove`（下划线） | 描述不符 | **P1** | 改下划线 |
   178	| D59 | guide/stack-deploy-design.md:129 | Stack 字段 `RunningCount`/`TotalCount`/`LastTaskStatus` | `Running`/`Total`/`LastAction`/`LastStatus` | 描述不符 | **P1** | 改字段名 |
   179	| D60 | guide/stack-deploy-design.md:122 | 404 = agent 不在线/无 stack provider | 404 = agent 不存在/无 docker 能力；502 = 有 docker 能力但 stack provider 未注册 | 描述不符 | **P1** | 改 404/502 映射 |
   180	| D61 | guide/stack-deploy-design.md:120 | DELETE body 可选 `{force}` | 路由只传 `{name}`，无 force 语义 | 文档超前于码 | **P1** | 删 force |
   181	| D62 | guide/stack-deploy-design.md:147 | `.env` 值默认掩码显示 | 明文渲染进 TextArea（`value={envValue}`） | 文档超前于码 | **P1** | 改明文 |
   182	| D63 | guide/drift-design.md:30 | 检测对象仅 nginx/cron/stack | 源码完整支持 traefik（`cockpit-site-*.yml` 动态片段） | 码超前于文档 | **P1** | 补 traefik |
   183	| D64 | guide/drift-design.md:38 | `drift.check` 单方法 | 3 个方法 check/diff/record | 描述不符 | P2 | 注 M3/M4 追加 |
   184	| D65 | guide/stack-deploy-design.md:314 | 无 token 401（默认关） | 未配令牌 → 404；已配但缺/错 → 401 | 描述不符 | P2 | 改 401/404 措辞 |
   185	| D66 | guide/stack-deploy-design.md:97 | 兼容 `compose.yaml`/`docker-compose.yml` | 还兼容 `docker-compose.yaml` | 码超前于文档 | P2 | 补 yaml |
   186	
   187	### 束五：service / service-health
   188	
   189	| ID | 文档页 | 主张 | 源码证据 | 类别 | 严重度 | 处置 |
   190	| --- | --- | --- | --- | --- | --- | --- |
     1	# Documentation-Source Consistency Audit (2026-10-08)
     2	
     3	> Language: [中文](./审计-文档一致性.md) ｜ **English**
     4	
     5	> Executed per the "Documentation-Source Consistency Audit Specification": enumerate the
     6	> document surface → take verifiable technical claims and check them against the code
     7	> (file:line) → enter the three categories of differences into a table → fix the docs
     8	> with the source as the truth (modifying code in reverse is forbidden).
     9	> This table records all differences found in the first audit round and their disposition;
    10	> the "current state" column is the post-fix verification conclusion.
    11	
    12	## Audit Scope
    13	
    14	- `docs/guide/` (35 documents: concepts/architecture/protocol/getting-started/designs for each feature domain/decision-log, etc.)
    15	- `docs/` root (agent-config / agent-id / guacd-deploy / websocket-token-auth / remote-desktop-guacamole-design / remote-access-integration-design / ui-theme-design)
    16	- `docs/operations/deploy-docker.md`, `docs/public/README.md`, `README.md`, `BUGS.md`
    17	- the 17 sidebar links of `docs/.vitepress/config.ts` (all reachable)
    18	- Spot-check method: first build the code-side ground truth (routing table / full capability set / config keys / protocol messages / permission-point list), then grep each document claim and check it against the code one by one
    19	
    20	## Ground-Truth Anchors
    21	
    22	| Fact | Evidence |
    23	| --- | --- |
    24	| Route registration (mux direct mount + `/api/` fallback switch) | `internal/server/server.go:165-310`, `internal/server/api.go:34-172` |
    25	| Full capability set of 17 (detector 7 + code direct-report 10) | `internal/agent/detector/` (7 files), `internal/agent/agent.go:341-477` |
    26	| Permission points' single source of truth | `internal/storage/role.go:29-60` `resourceActions`, `internal/server/rbac.go:23-88` |
    27	| Proxy data-plane dialing | `internal/proxy/handler.go:116` (`net.DialTimeout("tcp",…)`), `internal/proxy/ssh_session.go:125` (`ssh.Dial("tcp",…)`) |
    28	| guacd recording archival | `internal/server/api_guacamole.go:209-250` `collectGuacRecording` |
    29	| GHCR tag field-verified (anonymous token + registry API) | Only `latest`/`main`/`sha-*` exist in practice; the mechanism is in `.github/workflows/docker.yml:67-82` (a `v*` semver has never been triggered) |
    30	
    31	## Difference Table
    32	
    33	| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
    34	| --- | --- | --- | --- | --- | --- | --- |
    35	| A1 | guide/concepts.md:141 (original) | capability table contains `network-monitor` | Deleted (overlay-design D18; zero matches repo-wide) | Doc ahead of code | **P0** | Delete the row; rewrite the table as "detector 7 + code direct-report 10" with a historical note |
    36	| A2 | guide/concepts.md:133-141 (original) | "current detectors include" lists only 5 | True full set is 17 (7 in detector/ + 10 direct-report at agent.go:341-477) | Code ahead of doc | P1 | Add `firewall` `overlay` `remote-services` + the 10 direct-reports (with each one's reporting condition, verified line by line) |
    37	| A3 | remote-desktop-guacamole-design.md:83 | The current-endpoint examples include `/api/remote/desktop` | Route deleted (grdp deletion surgery D-2026-10-07-1; the surviving ones are terminal/guacamole/tickets/sessions/vault) | Doc ahead of code | P1 | Change to `/api/remote/guacamole`; sync the pipeline description to "agent WS / guacd TCP" |
    38	| A4 | guide/protocol.md:146 | Proxy messages can reach "TCP/UDP targets" | Data plane dials TCP only (handler.go:116 and ssh_session.go:125 both hard-code `"tcp"`; `proxyType=udp` is only accepted by API validation/persistence, api_proxy.go:99) | Description mismatch | P1 | Change to "TCP targets" + note the status of the udp parameter |
    39	| A5 | guide/getting-started.md:5 | "Go 1.26.3 or a compatible version" | `go.mod` says `go 1.27` | Description mismatch | P1 | Change to 1.27 |
    40	| A6 | guide/mobile-design.md:73 | Audit list endpoint `/api/audit` | Actually `/api/admin/audit/logs` (mobile/lib/api/endpoints.dart:76; api_audit.go routing) | Description mismatch | P1 | Change to the current endpoint |
    41	| A7 | guide/nas-design.md:65 | OMV provider "M2 queued" | Already implemented and wired (`internal/agent/rpc/nas_dsm.go:290` `case "omv"`) | Code ahead of doc | P1 | Change to "M2 completed" |
    42	| A8 | guide/rbac-design.md D3 | resource list stops at 20; `services`/`smart`/`recordings` read as future completion | `role.go` `resourceActions` actually has 25 (completion done, additionally `firewall` `jobs`) | Code ahead of doc | P2 | Change to "reconciliation completed" + list `firewall`/`jobs`, pointing at role.go as the single source of truth |
    43	| A9 | guide/getting-started.md:252 | Image tag table unconditionally lists `v1.2.3` | GHCR field-verified: only latest/main/sha-*; the semver series is a workflow capability (docker.yml:73-75), and a `v*` tag has never been pushed | Description mismatch | P2 | Change to the conditional wording "when a `v*` tag is pushed there is an additional semver series" |
    44	| A10 | guacd-deploy.md:96 | Recording files "automatically collected into the database" | The file is archived to `/data/recordings/` and only the metadata is back-filled into the DB (api_guacamole.go:209-250; deploy-docker.md:124 says the same) | Description mismatch | P2 | Change to "collected and archived to /data/recordings/ (metadata back-filled into the database)" |
    45	| A11 | guide/overlay-design.md:139, 163 | "network-monitor convergence → cleanup in a later version" reads as a todo | Already done and the same doc records it at D18 (:198) | Code ahead of doc | P2 | Add "done, see D18" in both places |
    46	
    47	**Totals**: doc-ahead-of-code 0; code-ahead-of-doc 0; description mismatch 0. All differences were fixed in the prior batch; see the tracking records in the "Disposition" column below.
    48	
    49	**P0 list**: none — the P0 item A1 (the concepts.md phantom capability `network-monitor`) had that row deleted in the prior batch, retaining only the historical note, so it makes no runtime claim.
    50	
    51	## Sampled Surfaces Verified Consistent (not in the difference table)
    52	
    53	- ~150 endpoint claims all check out against the code, with only the A3/A6 differences; the protocol.md HTTP API section, ticket 5-minute single-use (ticket.go:58), the six public auth endpoints, and the six resource types all match
    54	- agent-config.md: 8 startup params / 9 env vars / ID default format `agent-{hostname}-{machineId[:8]}` (agent.go:503) all match
    55	- agent-id.md: the three platforms' machine-identity sources and the bias suffix rules (agent.go:490-509) match
    56	- deploy-docker.md: PR builds do not push to the registry (`push:` condition in docker.yml), pnpm@11.19.0 (Dockerfile:16), entrypoint.sh weak-password hard check, nginx reverse-proxying three WS paths, the same-volume/same-path recording argument — all match
    57	- guacd-deploy.md: the compose file, guacd:1.5.5, GUACD_BIND/GUACD_PORT, TCP healthcheck, the `ssh.getDefaultKey` RPC (ssh_provider.go:31), automatic Ed25519 generation at agent startup (agent.go:167 → ssh_keys.go) — all match except A10
    58	- websocket-token-auth.md, docs/public/README.md (scaffolding note with no technical claims), BUGS.md — match
    59	- rbac.go's route→resource merge rules match the rbac-design.md D-entry notes (special conventions such as audit fixed to read, terminal fixed to write, and logs fixed to read on the agent side all have corresponding D-notes)
    60	
    61	## Leftover Observations (outside the scope of this doc-fix round)
    62	
    63	- `proxyType=udp`: the API accepts and persists it but the data plane does not implement it (the A4 note already says so). If real UDP forwarding is ever wanted it should be filed as a todo project; the docs should not be vague about it. → **Resolved (2026-10-08 follow-up decision D-2026-10-08-2)**: the API now rejects udp (400, tcp only), and the protocol/storage comments are annotated accordingly; the protocol.md note has been updated to the new behavior.
    64	- GHCR semver tag series: the mechanism exists but has never been triggered. After the first `v*` tag is pushed, the tag tables in getting-started/deploy-docker automatically become accurate; no code change needed.
    65	
    66	## Second Round (2026-10-11, after the M2c cancel-signal landing)
    67	
    68	> Scope: design documents newly added or changed after the 2026-10-08 first-round audit
    69	> (workflow/service/reference-projects/acceptance-checklist/protocol/jobs — six documents)
    70	> + a repo-wide check of the "natural end / only pending" class of cancel-semantics claims
    71	> against the code. The M2c landing code is the truth (`internal/server/api_jobs.go`
    72	> cancelJob, `internal/server/api_workflow.go` cancelRunJobs/FinishWorkflowRun).
    73	
    74	| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
    75	| --- | --- | --- | --- | --- | --- | --- |
    76	| B1 | guide/protocol.md:253 (original) | `POST /api/jobs/{id}/cancel` "cancels a pending Job; dispatched/final states get 409" | M2c C6: running can be cancelled — the `job.cancel` RPC kills the in-flight process and a conditional UPDATE lands `cancelled`; pending is a DB-only conditional cancel; final states are uniformly 409 (api_jobs.go cancelJob) | Code ahead of doc | P1 | Change to the current three-branch semantics |
    77	| B2 | guide/protocol.md:259 (original) | run cancel "in-flight steps end naturally" | M2c C8: run cancel kills in-flight step Jobs via `cancelRunJobs`, with the pending window being a DB-only conditional cancel (api_workflow.go) | Code ahead of doc | P1 | Change to the kill wording |
    78	| B3 | guide/jobs-design.md (top P2 note/D1/"boundary: no async execution"/P2 growth point) | "pending can be cancelled", "cancel is a capability only future async brings", "no async execution" read as current | P2 async (2026-10-09) + M2c (2026-10-11) have landed: `cancelled` is a real terminal state, running can be cancelled, and ledger filtering is in place | Code ahead of doc | P2 | Update by annotation (the historical decision keeps the original text + a "post-P2 update" parenthetical; the boundary section is marked fulfilled) |
    79	| B4 | guide/acceptance-checklist.md:301, 313 (original) | The W5/W9 evidence rows describe "natural end / only pending 409" | The semantics have changed (same as B1/B2); the W5/W9 probes have been refreshed to the kill wording + a new W11 was added (e72fb76); real-machine re-verification awaits the next real session | Code ahead of doc | P2 | Keep the historical evidence + a M2c refresh note; the semantics are governed by the refreshed probes |
    80	
    81	**Totals**: code-ahead-of-doc 4 (B1-B4), all semantic drift after the M2c landing, no doc-ahead-of-code.
    82	The remaining ~150 endpoint claims in protocol.md, the jobs-design.md decision table D2-D10, and workflow-design.md (maintained in the same batch) show no new differences.
    83	
    84	## Third Round (2026-10-11, alongside the coverage-gate hardening batch)
    85	
    86	> Scope: a scan of status-type claims across the whole guide directory ("projected, not
    87	> implemented", "still a candidate, not scheduled", "unimplemented / to be implemented /
    88	> not landed / not yet / todo"), read against the code. Only workflow-design.md has two,
    89	> fixed in the same batch.
    90	
    91	| ID | Doc page | Claim | Fact | Disposition |
    92	| --- | --- | --- | --- | --- |
    93	| C1 | guide/workflow-design.md:3 (original) | Top banner "2026-10-09 projected design, not implemented" | The same doc's "implementation status" section has M1/M2a/M2b/M2c all landed | Change the banner to "implementation progress is governed by the implementation-status section" |
    94	| C2 | guide/workflow-design.md:38 (original) | M2b item "remaining candidates (cancel signal/DAG/approval gate/SSE) are still candidates, not scheduled" | The M2c cancel signal landed on 10-11 (the following M2c bullet says so) | Keep the historical sentence + a parenthetical of the landing |
    95	
    96	**Totals**: code-ahead-of-doc 2 (C1-C2), both cases of banners/candidate lists not kept current after the M2c landing, not introduced by this round's semantic changes.
    97	
    98	## Fourth Round (2026-10-15, full-coverage six-bundle parallel audit)
    99	
   100	> Scope: the entire document surface (guide/ 35 + design/ 4 + root 7 + operations/ 1),
   101	> six parallel Explore agents checking each claim against the code and taking evidence, with
   102	> P0/P1 candidates re-verified in person. The three categories of differences go into the
   103	> table, and docs are fixed with the source as the truth (modifying code in reverse is
   104	> forbidden).
   105	
   106	### Bundle 1: Facade (architecture / getting-started / rbac / ui-theme / websocket-token-auth / remote-access)
   107	
   108	| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
   109	| --- | --- | --- | --- | --- | --- | --- |
   110	| D1 | guide/architecture.md:144 | Agent→Server messages are missing `probe_report`/`exec_log`; `heartbeat` is listed as Agent→Server | `internal/protocol/message.go:8-26` contains `probe_report`/`exec_log`; heartbeat is bidirectional (the Server replies with an ACK) | Code ahead of doc | P2 | Complete the message-type table |
   111	| D2 | guide/getting-started.md:24 | static_dir missing → the root path returns an API hint | The API hint is returned only when both `server.static_dir` and `STATIC_DIR` are unset; a configured-but-missing directory gives 404 | Description mismatch | P2 | Change to the dual-condition semantics |
   112	| D3 | guide/getting-started.md:32 | The init tree contains `cockpit.db` | The `data/` directory is created and `cockpit.db` is auto-created on first database open | Description mismatch | P2 | Change the init-tree explanation |
   113	| D4 | guide/getting-started.md:213 | vault cross-reference "section 5" | V5 is in section 2 | Description mismatch | P2 | Fix the reference |
   114	| D5 | guide/rbac-design.md:36 | resource list is missing `workflows`/`approvals`/`overlay admin` | `internal/storage/role.go:60,63,42` contains the three resources; `approvals` was additionally registered on 2026-10-15 | Code ahead of doc | P1 | Complete the resource list + the permission matrix |
   115	| D6 | docs/ui-theme-design.md:151 | `bodyBg` takes `chrome` | `bodyBg` takes `bg` (the page background color) | Description mismatch | P2 | Fix the field mapping |
   116	| D7 | docs/ui-theme-design.md:191 | `inputBg` has the same name on both ends | `inputBg` is web-only (the mobile SkinPalette has no such field) | Description mismatch | P2 | Note web-only |
   117	| D8 | docs/websocket-token-auth.md:1 | Tickets are uniformly passed via `Sec-WebSocket-Protocol` | terminal = subprotocol only; guacamole = `?ticket=` query takes priority, subprotocol is the fallback | Description mismatch | P2 | Change to the dual-channel semantics |
   118	| D9 | docs/remote-access-integration-design.md:91 | guacd dials the target from its own network position; the agent is not on the data path | guacd only dials the local loopback relay (`internal/server/guac_relay.go`); the real target is dialed out by the agent, which is on the data path | Description mismatch | P1 | Change to the guacd-relay architecture |
   119	| D10 | docs/remote-access-integration-design.md:91 | RBAC `remote` resource domain | The `terminal` resource domain (`/api/remote/*` is merged uniformly) | Description mismatch | P2 | Change the resource domain |
   120	| D11 | docs/remote-access-integration-design.md:180 | Real-machine acceptance is suspended pending verification | Already passed on 2026-09-30 | Code ahead of doc | P2 | Update the acceptance status |
   121	
   122	### Bundle 2: Backend Core (agent-core / service-detect / external-session / agent-config / guacd / openwrt / reference / vault / persona / nas / file-manager / egress / recording / workflow / deploy-docker)
   123	
   124	| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
   125	| --- | --- | --- | --- | --- | --- | --- |
   126	| D12 | design/agent-core-architecture.md:6 | Status Proposed | P1-P5 have landed | Code ahead of doc | P1 | Change the status to landed |
   127	| D13 | design/agent-core-architecture.md:48 | execlog is new | `core/execlog` already exists (ring buffer + batched uplink) | Code ahead of doc | P2 | Change to already-exists |
   128	| D14 | design/agent-core-architecture.md:113 | 3 build-openwrt chains | 5 chains (mipsel/mips/aarch64_generic/arm_cortex-a7_neon-vfpv4/x86_64) | Code ahead of doc | P2 | Change to 5 chains |
   129	| D15 | design/agent-core-architecture.md:113 (batch table) | The batch table has no status column | P1-P5 are all landed | Code ahead of doc | P2 | Add a status column |
   130	| D16 | design/agent-core-architecture.md:16 | agent.go is 778 lines | 696 lines | Code ahead of doc | P2 | Fix the line count |
   131	| D17 | design/agent-core-architecture.md:104 | main is 112 lines | 116 lines | Code ahead of doc | P2 | Fix the line count |
   132	| D18 | design/service-detect-agent.md:64 | The Window struct has no Semantics field | `core/healthprobe/window.go` contains a `Semantics` field | Code ahead of doc | P2 | Add the field |
   133	| D19 | design/service-detect-agent.md:77 | `cockpit-probe-agent status` subcommand | `-status-file` writes JSON (there is no `status` subcommand) | Description mismatch | P2 | Fix the flag semantics |
   134	| D20 | design/external-session-api.md:42 | Whitelist ref `api_remote.go:403` | `handleRemoteSessionCreate`'s whitelist is at `api_remote.go:654-660` | Description mismatch | P2 | Fix the line number |
   135	| D21 | docs/agent-config.md:30 | `BIAS`/`COCKPIT_AGENT_OPTS` exist | Neither variable exists anywhere in the repo | Doc ahead of code | P2 | Delete the row |
   136	| D22 | docs/agent-config.md:30 | `SERVER_URL` has a default | Required, no code default | Description mismatch | P2 | Change to required |
   137	| D23 | docs/agent-config.md:30 | REGION/ZONE undocumented | The binary reads `COCKPIT_REGION`/`COCKPIT_ZONE` directly (`core/register/register.go:97-104`) | Code ahead of doc | P2 | Add the documentation |
   138	| D24 | docs/agent-config.md:61 | The Agent runs as root | systemd runs it as the `cockpit` user (`ProtectHome=true`) | Description mismatch | P2 | Change the run user |
   139	| D25 | docs/guacd-deploy.md:72 | The Server always calls `ssh.getDefaultKey` | Only when neither a private key nor a password is filled in | Description mismatch | P2 | Change the condition |
   140	| D26 | docs/guide/openwrt-agent-design.md:27 | The dependency closure is missing the core/ ten packages | `internal/agent/rdp` is deleted; the core/ ten packages are added | Code ahead of doc | P2 | Complete the dependency closure |
   141	| D27 | docs/guide/reference-projects.md:134 | uptimekuma.co official link | It is an unofficial fan site; GitHub is the only official source | Description mismatch | P2 | Change to GitHub only |
   142	| D28 | docs/guide/vault-design.md:15 | The ACME private key is plaintext in the DB | It is encrypted in the DB (`serializer:acmeprivkey`, converged in the V6 second wave) | Code ahead of doc | P1 | Change to encrypted |
   143	| D29 | docs/guide/vault-design.md:15 | The inventory capability token is plaintext | It is encrypted in the DB (`serializer:capconf`) | Code ahead of doc | P1 | Change to encrypted |
   144	| D30 | docs/guide/vault-design.md:131 | The V6 second wave is not landed | Landed | Code ahead of doc | P2 | Change to landed |
   145	| D31 | docs/guide/persona-vault-mapping.md:67 | 429 has no detailed documentation | `rate limited (120 req/min per token)`, no automatic retry | Code ahead of doc | P2 | Add the documentation |
   146	| D32 | docs/guide/nas-design.md:71 | Credentials are stored only in the agent's environment variables | The first source is the server vault `nas/<name>` (encrypted, persisted); env is only the fallback | Code ahead of doc | P1 | Change the credential source |
   147	| D33 | docs/guide/nas-design.md:82 | Only the `dsm` type | Supports `dsm`/`truenas`/`omv` | Code ahead of doc | P2 | Add the types |
   148	| D34 | docs/guide/nas-design.md:218 | Alert-recovery semantics | `createAlertIfNotExists` only adds, never cleans up | Description mismatch | P2 | Fix the recovery semantics |
   149	| D35 | docs/guide/file-manager-design.md:50 | `maxWrite`/`maxUpload` | `maxReadChunk`/`maxWriteChunk` (the 10MB upload cap is on the web side) | Description mismatch | P2 | Fix the field names |
   150	| D36 | docs/guide/agent-egress-sdwan.md:49 | Overlay probing includes Cloudflare Tunnel | Only zerotier/tailscale/wireguard, no cloudflare | Description mismatch | P2 | Fix the probing scope |
   151	| D37 | docs/guide/agent-egress-sdwan.md:60 | The Web SSH is not a full SSH client | `internal/proxy/ssh_session.go` implements a full SSH client | Code ahead of doc | P1 | Change to full client |
   152	| D38 | docs/guide/agent-egress-sdwan.md:111 | SSH uses `ssh_provider.go` | `ssh_session.go` (`ssh_provider.go` only provides the default key) | Description mismatch | P2 | Fix the file reference |
   153	| D39 | docs/guide/recording-design.md:25 | HandleTerminalData line number 302 | 341 | Description mismatch | P2 | Fix the line number |
   154	| D40 | docs/guide/recording-design.md:34 | D1 decides VNC/RDP are not recorded | M3 has landed guacd all-protocol recording | Code ahead of doc | P1 | Note M3 overrules + add an M3 section |
   155	| D41 | docs/guide/workflow-design.md:46 | The approvals permission point is registered | The route has `approvals` mounted but `resourceActions` has no such key, so all roles get 403 | Code ahead of doc | P1 | Register it + a regression test |
   156	| D42 | docs/operations/deploy-docker.md:11 | The compose has no extra port mapping | The root compose maps an extra `9090:9000` | Code ahead of doc | P2 | Add the mapping |
   157	| D43 | docs/operations/deploy-docker.md:53 | The tag table contains `v1.2.3` | `docker/metadata-action` strips the `v` prefix | Description mismatch | P2 | Delete the `v` prefix |
   158	| D44 | docs/operations/deploy-docker.md:84 | `ALLOWED_ORIGINS` is required | When unset, the WS accepts any Origin (logging a warning), and startup is not blocked | Description mismatch | P2 | Change to optional |
   159	| D45 | docs/operations/deploy-docker.md:95 | `AGENT_EXPIRE_MINUTES` is wired | The compose does not wire that variable | Description mismatch | P2 | Note it is not wired |
   160	| D46 | docs/operations/deploy-docker.md:150 | The nginx conf is fully usable | The template contains the deleted desktop/vnc endpoints | Description mismatch | P2 | Fix the template explanation |
   161	
   162	### Bundle 3: Protocol / jobs / cron
   163	
   164	| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
   165	| --- | --- | --- | --- | --- | --- | --- |
   166	| D47 | guide/jobs-design.md:15 | The Jobs page polls every 15s | `refetchInterval: 5000` (5s) | Description mismatch | **P1** | Change to 5s |
   167	| D48 | guide/jobs-design.md:77 | POST /api/jobs returns a terminal state synchronously | The 201 response returns pending; the dispatched terminal state happens in the background | Description mismatch | P2 | Change to the async semantics |
   168	| D49 | guide/jobs-design.md:75 | No cancel endpoint | `POST /api/jobs/{id}/cancel` exists | Code ahead of doc | P2 | Add the endpoint |
   169	| D50 | guide/jobs-design.md:114 | jobs.ts has no cancelJob | `cancelJob` exists | Code ahead of doc | P2 | Add the method |
   170	| D51 | guide/cron-design.md:36 | The @ whitelist has 6 entries | 7 entries (including `@annually`) | Code ahead of doc | P2 | Add @annually |
   171	| D52 | guide/cron-design.md:120 | The preset "every Monday 03:00" | The preset is "every Monday 04:00" (`0 4 * * 1`) | Description mismatch | P2 | Change to 04:00 |
   172	| D53 | guide/cron-design.md:120 | 5 presets | 7 presets (adding every 5 minutes / the 1st of each month 05:00 / on boot) | Code ahead of doc | P2 | Add the 3 items |
   173	| D54 | guide/cron-design.md:161 | The `Call` case `timers` is in cron_timers.go | It is in `cron_provider.go:168`; the method name is `ListTimers` | Description mismatch | P2 | Fix the attribution + method name |
   174	| D55 | guide/cron-design.md:191 | The `Call` case `users` is in cron_users.go | It is in `cron_provider.go:166` | Description mismatch | P2 | Fix the attribution |
   175	| D56 | guide/cron-design.md:32 | `crontab -` writes back (stdin) | `crontab <temp file>` (the path is passed via argv, no stdin) | Description mismatch | P2 | Fix the write-back method |
   176	
   177	### Bundle 4: stack-deploy / drift
   178	
   179	| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
   180	| --- | --- | --- | --- | --- | --- | --- |
   181	| D57 | guide/stack-deploy-design.md:60 | `last-deploy.log` stores the task output | Actually persisted as `.cockpit/last-task.json` (metadata only); the log is an in-memory ring buffer only | Doc ahead of code | **P1** | Change to last-task.json |
   182	| D58 | guide/stack-deploy-design.md:138 | Audit event names like `stack.create` | `stack_create`/`stack_update`/`stack_up`/`stack_down`/`stack_remove` (underscores) | Description mismatch | **P1** | Change to underscores |
   183	| D59 | guide/stack-deploy-design.md:129 | Stack fields `RunningCount`/`TotalCount`/`LastTaskStatus` | `Running`/`Total`/`LastAction`/`LastStatus` | Description mismatch | **P1** | Fix the field names |
   184	| D60 | guide/stack-deploy-design.md:122 | 404 = agent offline / no stack provider | 404 = agent does not exist / no docker capability; 502 = has docker capability but the stack provider is not registered | Description mismatch | **P1** | Fix the 404/502 mapping |
   185	| D61 | guide/stack-deploy-design.md:120 | DELETE body optionally `{force}` | The route only passes `{name}`, with no force semantics | Doc ahead of code | **P1** | Delete force |
   186	| D62 | guide/stack-deploy-design.md:147 | `.env` values are masked by default | They are rendered in plaintext into the TextArea (`value={envValue}`) | Doc ahead of code | **P1** | Change to plaintext |
   187	| D63 | guide/drift-design.md:30 | The detection targets are only nginx/cron/stack | The source fully supports traefik (`cockpit-site-*.yml` dynamic fragments) | Code ahead of doc | **P1** | Add traefik |
   188	| D64 | guide/drift-design.md:38 | `drift.check` is a single method | 3 methods: check/diff/record | Description mismatch | P2 | Note the M3/M4 additions |
   189	| D65 | guide/stack-deploy-design.md:314 | No token → 401 (disabled by default) | Token not configured → 404; configured but missing/wrong → 401 | Description mismatch | P2 | Fix the 401/404 wording |
   190	| D66 | guide/stack-deploy-design.md:97 | Compatible with `compose.yaml`/`docker-compose.yml` | Also compatible with `docker-compose.yaml` | Code ahead of doc | P2 | Add yaml |
     1	# 远控会话录制设计（M1：终端输出录制 + 回放）
     2	
     3	> 语言：**中文** ｜ [English](./recording-design.en.md)
     4	
     5	> 2026-09-15 立项。对应 todo「远控会话录制：终端输出录制，补强远控审计
     6	> （当前审计只有会话开始/结束）。方案参考：Guacamole 会话录制、Teleport
     7	> 会话粒度审计。」
     8	
     9	## 痛点
    10	
    11	远控（SSH/telnet 终端）的审计目前只有**会话开始/结束**两条记录
    12	（`auditRemoteStart` / `auditRemoteEnd`）：知道谁在什么时间连了哪台主机，
    13	但**会话里做了什么完全不可追溯**。个人云场景下，所有管理员的终端操作
    14	集中在这一个入口，出问题（误删文件、改错配置）后无法还原现场。
    15	
    16	## 架构
    17	
    18	终端数据本来就全部流经 server（Agent 主动出站 WebSocket，浏览器 ←
    19	server ← agent 双向转发），**录制挂在 server 侧的转发管道上**，agent
    20	零变更：
    21	
    22	```
    23	浏览器 ──input──▶ terminalSendLoop ──▶ agent
    24	浏览器 ◀─data─── HandleTerminalData ◀── agent
    25	                      │
    26	                      └─▶ castRecorder.Write() ──▶ data/recordings/<sid>.cast
    27	                                                     + SQLite 元数据
    28	```
    29	
    30	- `api_remote.go:341 HandleTerminalData`：agent → 浏览器的唯一出口，
    31	  录制挂钩点；
    32	- `closeTerminalSession` / `HandleTerminalClose`：会话终态，finalize 回填
    33	  时长与字节数。
    34	
    35	## 决策
    36	
    37	| # | 决策 | 内容 | 理由 / 备注 |
    38	|---|------|------|------------|
    39	| D1 | M1 范围 | 仅终端（SSH/telnet 经 `/api/remote/terminal` 的会话）；VNC/RDP 像素流不录（此判断被 M3 推翻，见下） | 终端流天然是字符事件序列，asciinema 格式即为其行业标准；像素流录制体积与回放复杂度高，价值低 |
    40	| D2 | 录制位置 | server 侧转发管道，agent 零变更 | 数据已流经 server；agent 侧录会引入文件管理与上传链路 |
    41	| D3 | 格式 | asciinema v2（`.cast`）：首行 header JSON，之后每事件一行 `[相对秒,"o","数据"]` | 行业开放格式，可脱离 Cockpit 用 `asciinema play` 回放/转换；行式追加写天然适合流式 |
    42	| D4 | 只录输出 | 输入流不录（不写 `"i"` 事件） | 输入含密码（SSH/sudo）明文，录制即泄漏源；Guacamole 同样只录输出侧。审计要的是「终端上出现了什么」而非「键盘按了什么」 |
    43	| D5 | 存储 | 文件 `<db目录>/recordings/<sessionID>.cast` + 新表 `TerminalRecording`（session_id 唯一、username、agent_id、host、port、protocol、started_at、duration_ms、bytes） | 文件放内容、DB 放索引（与备份文件/配置分离同思路）；结束时回填 duration/bytes |
    44	| D6 | 开关与保留 | Setting `recording.enabled`（"true"/"false"，默认 true）+ `recording.retention_days`（0-365，默认 7，0=永久）；清理挂 cleanupLoop 每 30s 醒、按小时节流执行 | 默认开（审计闭环是卖点）+ 有限保留防磁盘膨胀；与 drift 巡检同款「循环里读 Setting 免缓存」 |
    45	| D7 | 写入策略 | `os.File` 直接写（不 fsync），每事件一行；写失败记一次日志后停录，**绝不影响转发主链路** | 终端输出小块低频，崩溃丢尾部可接受；远控可用性优先于录制完整性 |
    46	| D8 | finalize 幂等 | recorder 内部 `sync.Once`；`closeTerminalSession` 与 `HandleTerminalClose` 两处都调 Close（后者路径经 keepalive ping 失败兜底会晚 ≤30s 到达，两处直调即时回填） | 会话有两个结束入口，Once 防双写回填 |
    47	| D9 | REST | `GET /api/recordings`（倒序列表）`GET /api/recordings/{id}/cast`（文件流，即回放数据源，兼下载）`DELETE /api/recordings/{id}`（文件+记录同删） | 回放与下载同一端点取全量内容，统一记审计 |
    48	| D10 | 审计 | `recording_read`（取 cast 内容）与 `recording_delete` 记审计；列表浏览不记 | 录制内容含历史输出（可能有敏感信息），谁取走了内容应可追溯；列表是索引不算内容 |
    49	| D11 | 回放器 | Web 自行开发轻量：fetch .cast → 逐行解析 → 按时间差调度 `term.write()`，倍速 1/2/4/8x + 拖动跳放 | 不引 asciinema-player 依赖；项目已有 @xterm/xterm |
    50	| D12 | 尺寸 | cast header 固定 80x24；回放 xterm 按容器自适应不锁列宽 | server 侧不追踪 resize（session 未存 rows/cols）；asciinema v2 header 亦为单一尺寸，错位可接受 |
    51	| D13 | Web 入口 | 新页面 `/recordings`「会话录制」，菜单列于「审计日志」旁 | 录制是审计的延伸，入口放一起 |
    52	
    53	## 不做（后续版本）
    54	
    55	- VNC/RDP 桌面流录制（WebM/图片序列，体积大回放重）；
    56	- 输入事件录制（密码泄漏风险，见 D4）；
    57	- 实时旁观（live tail 进行中的会话）；
    58	- ~~录制文件的导出归档（S3/rclone，与备份异地同路线，等备份 M2 一起做）~~
    59	  → M2 立项（2026-09-19，前置路线 backup-design.md M2 与
    60	  server-backup-design.md M2 的 rclone 模式均已落地）。
    61	
    62	## M2：录制归档异地 + 配置入口（2026-09-19 设计）
    63	
    64	### 痛点
    65	
    66	录制是远控审计的唯一留痕，但本地 `.cast` 文件默认 7 天即被 retention
    67	删除，且与 cockpit.db 同盘——磁盘故障时审计记录与库一起消失。备份 M2
    68	（agent rclone）与 server 备份 M2 已建立完整的 remote:path 推送模式，
    69	录制归档复用同一条路线：录制结束即推异地，retention 只删本地，远端成为
    70	长期审计归档。
    71	
    72	顺手补 M1 缺口：`recording.enabled` / `recording.retention_days` 两个
    73	Setting 至今没有 REST/UI 写入口（只能直改库）——M2 一并补配置端点。
    74	
    75	### 决策
    76	
    77	| # | 决策 | 内容 | 理由 / 备注 |
    78	|---|------|------|------------|
    79	| D14 | 配置 | Setting `recording.remote_dest`（空=关闭），写时校验与 server 备份 M2 D13 同源（remote:path 正则 + 512 上限），读时脏值视为未配置 | 三备份/归档模块同一规则；凭据只在 server 主机 rclone.conf，零入库 |
    80	| D15 | 共用执行器 | 抽 `rcloneCopyLocalFile(local, remoteDest)`（exec copy --transfers 2 + 5min 超时 + WaitDelay 强断管道 + stderr 摘要），server 备份推送与录制归档共用；包级 var 改名 `serverRcloneBin` | 两处逐字同构，抽取消除漂移；WaitDelay 语义见 server-backup M2 差异补记 |
    81	| D16 | 时机 | 录制 `Close` 回填元数据的 finish 回调里**异步**推送（goroutine）：不拖会话出口路径；`.cast` 是已关闭的完整文件，单文件 copy 秒级 | 会话关闭路径同步 exec 会延迟 ws 清理；录制文件无需「推送失败中止」语义（D28 反例——审计留痕迟到可接受，丢失才致命） |
    82	| D17 | 失败语义 | 记 `[remote]` 日志 + 新事件 `recording.remote-failed`（白名单显式启用）：本地档仍在，retention 窗口内可手动补推；推送成功不通知 | 对称 backup.remote-failed / server_backup.remote-failed；retention 删除后失败档即永久丢失，通知是唯一可感知面 |
    83	| D18 | 手动补推 | `POST /api/recordings/{sid}/sync-remote`：按当前 remote_dest 推送，记审计 `recording sync_remote`（action=update） | 三模块对称；sid 是元数据主键天然无穿越面，文件不存在 404 |
    84	| D19 | 配置端点 | `GET/PUT /api/recordings/config`：enabled（bool）/ retention_days（0-365）/ remote_dest（正则校验）+ rclone_available 实时探测；web /recordings 页顶部加配置行 | 补 M1 缺口（Setting 无写入口）；配置放使用场景不进全局 Settings（drift D18 同款） |
    85	
    86	retention 只删本地文件与记录（远端归档不清理——rclone copy 幂等不删远端，
    87	清理远端有误删唯一归档的风险，文档说明即可）。
    88	
    89	### M2 清单
    90	
    91	- [x] notification：`RecordingRemoteFailed = "recording.remote-failed"` 常量
    92	- [x] server：`rcloneCopyLocalFile` 抽取（server 备份改调，行为不变）+
    93	      recording 远端推送（Close 异步）+ `recording.remote_dest` Setting
    94	- [x] REST：`/api/recordings/config` GET/PUT + `/{sid}/sync-remote`（审计）
    95	- [x] web：/recordings 页配置行（开关/保留/异地目标/rclone 提示）+
    96	      列表「补推」按钮 + api/types
    97	- [x] 测试：归档推送 ok/失败通知/未配置跳过/config 校验全形态/sync-remote
    98	      全路径/Close 异步不阻塞
    99	- [x] 文档收尾（本清单勾选）+ todo.md 同步
   100	
   101	M2 完成（2026-09-19）：server 6 测试全绿（fake rclone argv 注入同
   102	server 备份 M2 模式）。落地差异两处：① D18 设计认为「sid 是元数据主键
   103	天然无穿越面」，实施仍加了 UUID 严格正则（`^[0-9a-f]{8}-…`）前置 400——
   104	防御纵深零成本，非 UUID 形态根本到不了文件寻址；测试同时覆盖深穿越形态
   105	被路由分段检查挡在 404 的兜底。② D19 的 Setting 键名在清单草稿里写作
   106	`recording_remote_dest`，落地统一为点分风格 `recording.remote_dest`（与
   107	`recording.enabled` / `recording.retention_days` / `server_backup.remote_dest`
   108	同构）。真机剩余：rclone 真远端（如 gdrive/S3）推送验证（本地 fake rclone
   109	已覆盖 argv 与失败语义）。
   110	
   111	## M1 清单
   112	
   113	- [x] storage：`TerminalRecording` 模型 + 迁移 + Create/Finish/List/Get/
   114	      Delete/DeleteExpiredUntil
   115	- [x] server：`recording.go`（castRecorder 读写 + Setting 读写 + 过期清理
   116	      挂 cleanupLoop）+ 终端管道接线（创建/写输出/finalize 幂等）
   117	- [x] REST：`api_recordings.go` 列表/取内容/删除 + 审计 + serveAPI 接入
   118	- [x] web：`/recordings` 页面（列表 + xterm 回放 Modal 含倍速 + 下载/删除）
   119	      + 路由菜单
   120	- [x] 测试：cast 格式与 finalize 幂等、API CRUD 与审计、过期清理、
   121	      HandleTerminalData 落盘链路
   122	- [x] 文档收尾（本清单勾选）+ todo.md 同步
   123	
   124	M1 完成（2026-09-15）：server 5 测试全绿——cast 格式（header version=2 +
   125	`[dt,"o",data]` 行、Close 幂等回填一次、nil receiver 安全、文件权限 0600）、
   126	startRecording 元数据登记与管道落盘、开关语义（默认开/"false" 关，保留天数
   127	合法/0/非法回默认）、过期清理（文件+记录同删、0=永久）、API（列表/取内容
   128	Content-Type/404/405/删除/审计 view+delete 落库）。回放器自行开发：按事件时间差
   129	setTimeout 调度 term.write，1/2/4/8x 倍速切换即重放，进度展示。
   130	
   131	## M3：Guacamole 桌面会话录制（2026-09-30 落地，推翻 M1 D1）
   132	
   133	M1 D1 判定「VNC/RDP 像素流不录」，但 guacd 自带 session recording（`recording-path` /
   134	`recording-name` 两个 connect 参数，`Format=guac`），成本近乎白捡，M3 一并落地：
   135	
   136	- **所有 guacd 协议通录**：`internal/server/api_guacamole.go:165-171` 对 RDP/VNC/SSH 一律设置
   137	  `recording-path` / `recording-name` / `create-recording-path=true`，仅在 `recording.enabled=false` 时关闭；
   138	  格式为 `.guac`（指令流，非像素帧），回放走 web 端 GuacPlayer（绕开 guacamole-common-js 1.5.0
   139	  `SessionRecording(Blob)` 上游缺陷，duck tunnel 注入指令流）。
   140	- **台账统一**：录制元数据会话开始即登记（`Format=guac`，`recording.go` / `storage/recording.go`），
   141	  终端 asciinema 录制 `Format=cast`（空值兼容旧数据），二者共用 `/recordings` 列表与 `/api/recordings/{sid}/cast` 取内容。
   142	- **收集链路**：`collectGuacRecording` 从 guacd 容器卷搬到 server 录制目录，与终端录制同落
   143	  `<db 目录>/recordings`，M2 异地归档（`recording.remote_dest`）两者共享。
   144	- 验收证据：探针 S10 + 浏览器 CDP C4（2026-09-30，`.acceptance/guac/evidence/`），
   145	  真机清单见 `docs/guide/acceptance-checklist.md`「远控三协议」节。
   146	
   147	## 参考
   148	
   149	- 内部：[logs-design.md](./logs-design.md)（server 侧转发不落盘的反例——
   150	  本功能正是「落盘」侧的审计场景）、architecture.md 审计章节
   151	- 外部：asciinema v2 格式规范（<https://github.com/asciinema/asciinema/blob/develop/doc/asciicast-v2.md>）、
   152	  Guacamole session recording、Teleport session recording


[showing lines 113-532 of 532; full output saved to /home/cui/.local/share/opencode/shell/2b220c9cad2a7053c583eada91fc7bca7b5b6399/sh_14f070eef001N6Kze3QsRWZIhH.out]
[0m$ [0mnl -ba deployments/docker/docker-compose.yml | sed -n '1,220p'; nl -ba deployments/docker/.env.example | sed -n '1,200p'; nl -ba deployments/docker/README.md | sed -n '1,220p'; nl -ba deployments/guacd/README.md | sed -n '1,200p'; nl -ba README.zh.md | sed -n '1,140p'
1	# 生产部署 compose：拉 ghcr.io 发布的镜像，不在部署机本地构建。
     2	#
     3	# 与仓库根 docker-compose.yml 的分工：
     4	#   - 根目录那份：给「要改代码的人」用（build: . + image: cockpit:local，
     5	#     改完前端直接重建）
     6	#   - 本文件：给「部署者」用（image: ghcr.io/cuihairu/cockpit:<tag>，
     7	#     部署机只要有 Docker）
     8	#
     9	# 配套文档：docs/operations/deploy-docker.md（完整部署与排障）
    10	# 变量清单：deployments/docker/.env.example
    11	#
    12	# 部署：docker compose up -d
    13	# 交叉验证：docker compose config
    14	
    15	services:
    16	  cockpit-server:
    17	    # 锁 tag 部署（推荐）：COCKPIT_IMAGE_TAG=0.1.0；留空跟 latest
    18	    image: ghcr.io/cuihairu/cockpit:${COCKPIT_IMAGE_TAG:-latest}
    19	    container_name: cockpit-server
    20	    restart: unless-stopped
    21	    ports:
    22	      - "${COCKPIT_HTTP_PORT:-9000}:9000"
    23	    environment:
    24	      ADMIN_USERNAME: "${ADMIN_USERNAME:-admin}"
    25	      ADMIN_PASSWORD: "${ADMIN_PASSWORD:?set ADMIN_PASSWORD in .env}"
    26	      JWT_SECRET: "${JWT_SECRET:?set JWT_SECRET in .env}"
    27	      TOTP_ENCRYPTION_KEY: "${TOTP_ENCRYPTION_KEY:-}"
    28	      PRODUCTION: "${PRODUCTION:-true}"
    29	      ALLOWED_ORIGINS: "${ALLOWED_ORIGINS:-}"
    30	      BASE_URL: "${BASE_URL:-}"
    31	      TZ: "${TZ:-Asia/Shanghai}"
    32	      # 远程桌面/终端网关（guacd）地址：启用 guacd profile 时设为 guacd:4822
    33	      # （同网络 DNS 名）；留空 = 不启用 Guacamole 远控入口
    34	      # 见 docs/remote-desktop-guacamole-design.md 与
    35	      # docs/remote-access-integration-design.md
    36	      GUACD_ADDR: "${GUACD_ADDR:-}"
    37	      # guacd 录制目录。必须与 guacd 容器里的挂载点**同一个卷同一个路径**：
    38	      # guacd 按 connect 指令的 recording-path 往这儿写 <session>.guac，
    39	      # Go 网关会话结束时从同一路径收走归档到 /data/recordings
    40	      # （collectGuacRecording）。两侧路径不一致 = 录制文件永远收不到。
    41	      GUACD_RECORDING_PATH: "${GUACD_RECORDING_PATH:-/var/lib/guacamole}"
    42	    volumes:
    43	      # 数据卷：SQLite 库 /data/cockpit.db、清单、备份、录制归档
    44	      - cockpit-data:/data
    45	      # guacd 录制卷（与下方 guacd 服务共用，见 GUACD_RECORDING_PATH 注释）
    46	      - guacd-recordings:/var/lib/guacamole
    47	    networks:
    48	      - cockpit-net
    49	
    50	  # guacd：Apache Guacamole 协议守护进程，把 RDP/VNC/SSH 翻译成 Guacamole
    51	  # 指令流（跳过 guacamole-web 的 Java 层）。无用户概念、无状态——会话/权限/
    52	  # 审计都在 Go 网关侧。
    53	  # 启用：docker compose --profile guacd up -d（并设 GUACD_ADDR=guacd:4822）
    54	  # 要点：端口不对外暴露（Go 网关同网络反代，浏览器走
    55	  # /api/remote/guacamole 的 WebSocket）；GUACD_LOG_LEVEL 别开 debug
    56	  # （debug 会打印 connect 指令参数，含口令/私钥）。
    57	  guacd:
    58	    profiles: ["guacd"]
    59	    image: guacamole/guacd:1.5.5
    60	    container_name: cockpit-guacd
    61	    restart: unless-stopped
    62	    networks:
    63	      - cockpit-net
    64	    volumes:
    65	      # 与 cockpit-server 同一个卷：guacd 写 .guac，Go 网关收走归档
    66	      - guacd-recordings:/var/lib/guacamole
    67	    environment:
    68	      GUACD_LOG_LEVEL: "${GUACD_LOG_LEVEL:-info}"
    69	      TZ: "${TZ:-Asia/Shanghai}"
    70	
    71	networks:
    72	  cockpit-net:
    73	    driver: bridge
    74	
    75	volumes:
    76	  cockpit-data:
    77	  guacd-recordings:
     1	# 镜像 tag（仅 deployments/docker/docker-compose.yml 用；留空 = latest）。
     2	# 生产建议锁不可变 tag（语义化版本或构建短 sha），别长期跟 latest。
     3	COCKPIT_IMAGE_TAG=
     4	COCKPIT_HTTP_PORT=9000
     5	ADMIN_USERNAME=admin
     6	ADMIN_PASSWORD=replace-with-a-strong-password
     7	JWT_SECRET=replace-with-a-long-random-secret
     8	TOTP_ENCRYPTION_KEY=replace-with-a-32-byte-random-key
     9	PRODUCTION=true
    10	ALLOWED_ORIGINS=http://localhost:9000
    11	TZ=Asia/Shanghai
    12	# 远控栈（guacd）：--profile guacd 启用时设 guacd:4822（同网络 DNS 名，
    13	# 不能写 127.0.0.1——那是 guacd 容器自己的回环）。留空 = 前端隐藏 RDP/VNC/SSH 入口。
    14	GUACD_ADDR=
    15	# guacd 录制目录：须与 guacd 容器挂载点同卷同路径，否则 .guac 归档收不到
    16	GUACD_RECORDING_PATH=/var/lib/guacamole
    17	# 别开 debug：会打印 connect 指令参数（含口令/私钥）
    18	GUACD_LOG_LEVEL=info
     1	# Docker 部署
     2	
     3	适合在服务器上直接运行 Cockpit Server。
     4	
     5	本目录有两套部署姿势：
     6	
     7	| 方式 | 镜像来源 | 适用 |
     8	| --- | --- | --- |
     9	| 本目录 [`docker-compose.yml`](./docker-compose.yml) | 拉 `ghcr.io/cuihairu/cockpit` | 部署机只要有 Docker，不在部署机上编译 |
    10	| 仓库根 [`docker-compose.yml`](../../docker-compose.yml) | 本地 `build: .` | 改代码自测 |
    11	
    12	完整部署步骤（镜像 tag 策略、远控 guacd 启用、反向代理、备份回滚、排障）见
    13	**[docs/operations/deploy-docker.md](../../docs/operations/deploy-docker.md)**。
    14	
    15	## 快速开始
    16	
    17	```bash
    18	git clone https://github.com/cuihairu/cockpit.git
    19	cd cockpit
    20	
    21	# 方式一：本地构建（改代码用）
    22	cp deployments/docker/.env.example .env
    23	vi .env
    24	docker compose up -d --build
    25	
    26	# 方式二：拉官方镜像（部署用）
    27	cp deployments/docker/.env.example deployments/docker/.env
    28	vi deployments/docker/.env
    29	docker compose -f deployments/docker/docker-compose.yml up -d
    30	```
    31	
    32	访问：
    33	
    34	```text
    35	http://<server-ip>:9000
    36	```
    37	
    38	## 必改配置
    39	
    40	- `ADMIN_PASSWORD`：管理员初始密码，不能使用示例值。
    41	- `JWT_SECRET`：JWT 签名密钥，必须使用长随机字符串。
    42	- `TOTP_ENCRYPTION_KEY`：`PRODUCTION=true` 时必须设置，至少 32 个字符；启用 TOTP 后必须妥善保存，丢失后无法解密已有 TOTP 密钥。
    43	- `ALLOWED_ORIGINS`：对外域名，例如 `https://cockpit.example.com`。
    44	
    45	生成随机值示例：
    46	
    47	```bash
    48	openssl rand -base64 32
    49	```
    50	
    51	如果直接使用 `.env.example` 中的占位值，容器会拒绝启动。
    52	
    53	## 常用命令
    54	
    55	```bash
    56	docker compose ps
    57	docker compose logs -f cockpit-server
    58	docker compose restart cockpit-server
    59	docker compose pull
    60	docker compose up -d --build
    61	```
    62	
    63	数据保存在 Docker volume `cockpit-data` 中，包含 SQLite 数据库 `/data/cockpit.db`。
    64	
    65	升级镜像前建议备份该 volume 或导出 `/data/cockpit.db`。
    66	
    67	## 远控（guacd，可选）
    68	
    69	RDP / VNC / SSH 远控入口需要同网络的 guacd（`--profile guacd` 启用）：
    70	
    71	```bash
    72	GUACD_ADDR=guacd:4822 docker compose --profile guacd up -d
    73	```
    74	
    75	要点：guacd 端口不对外暴露（浏览器走 `/api/remote/guacamole`，Go 网关同网络
    76	反代）；`guacd-recordings` 卷必须 server 与 guacd **同卷同路径**（guacd 写
    77	`.guac`，Go 网关收走归档到 `/data/recordings`）；`GUACD_LOG_LEVEL` 别开 debug。
    78	详见 [deployments/guacd/README.md](../guacd/README.md) 与
    79	[docs/remote-access-integration-design.md](../../docs/remote-access-integration-design.md)。
    80	
    81	## Agent 连接
    82	
    83	Agent 连接地址：
    84	
    85	```text
    86	ws://<server-ip>:9000/ws
    87	```
    88	
    89	如果你在反向代理后提供 HTTPS，请使用：
    90	
    91	```text
    92	wss://cockpit.example.com/ws
    93	```
    94	
    95	## 反向代理
    96	
    97	如果使用 Nginx/Caddy/Traefik，请确保 WebSocket `/ws` 能正常升级转发。
     1	# guacd（Apache Guacamole 协议守护进程）
     2	
     3	Guacamole 集成的协议引擎侧。见
     4	[远程桌面设计](../../docs/remote-desktop-guacamole-design.md)。
     5	
     6	## 它做什么（和不做什么）
     7	
     8	`guacd` 是 Guacamole 的**协议翻译守护进程**（C 实现，Apache 2.0）：给它一条
     9	`RDP → host:3389` 的指令，它去连并把 RDP/VNC 协议翻译成 Guacamole 指令流。
    10	
    11	- **做**：RDP/VNC 协议终结（FreeRDP/VNC 客户端库）、指令流翻译、会话录制
    12	  落盘（`.guac` 格式）
    13	- **不做**：用户/权限/连接管理（那是 guacamole-web 的职责，我们跳过了——
    14	  会话/权限/审计在 Cockpit server 的 Go 网关里）
    15	
    16	## 部署
    17	
    18	```bash
    19	cd deployments/guacd
    20	docker compose up -d
    21	docker compose ps        # 应显示 healthy
    22	```
    23	
    24	端口默认绑 `127.0.0.1:4822`——**只给本机 Go 网关反代**，不对外暴露。
    25	guacd 不是 HTTP 服务，浏览器永远走 Cockpit 的 `/api/remote/guacamole`
    26	WebSocket 端点（由 Go 网关反代到此端口）。
    27	
    28	跨机部署（guacd 与 Cockpit server 不同机）：
    29	
    30	```bash
    31	# .env
    32	GUACD_BIND=192.168.5.20    # guacd 主机内网地址
    33	GUACD_PORT=4822
    34	```
    35	
    36	并靠防火墙收口——只允许 Cockpit server 的 IP 访问 4822。
    37	
    38	## 配置项（.env）
    39	
    40	| 变量 | 默认 | 说明 |
    41	|---|---|---|
    42	| `GUACD_BIND` | `127.0.0.1` | 端口绑定地址（同机反代用回环；跨机用内网地址） |
    43	| `GUACD_PORT` | `4822` | 端口（guacd 默认 4822，Go 网关侧 `GUACD_ADDR` 要对应） |
    44	| `GUACD_LOG_LEVEL` | `info` | **别开 `debug`**——debug 会打印 `connect` 指令参数（含口令） |
    45	| `TZ` | `Asia/Shanghai` | 时区 |
    46	
    47	## 持久化卷
    48	
    49	| 卷 | 路径 | 用途 |
    50	|---|---|---|
    51	| `guacd-recordings` | `/var/lib/guacamole` | **会话录制**（`.guac`）落盘目录。Go 网关在 `connect` 指令里传 `recording-path` + `recording-name`，guacd 自动录制——桌面审计回放的「白捡」红利，见设计风险章节 |
    52	| `guacd-drive` | `/var/lib/guacamole/drive` | 阶段二文件传输（RDP 驱动器重定向）预留 |
    53	
    54	guacd 本身无状态（连接参数每条 `connect` 指令传入，不落配置），容器可
    55	随意重建；recording 卷是唯一持久化面。
    56	
    57	## 版本对齐
    58	
    59	`guacd` 版本要与前端 `guacamole-common-js` 版本匹配到**同一 Guacamole
    60	发行版**（1.5.x 配 1.5.x）。Guacamole 协议是私有协议（无 schema），跨版本
    61	不保证兼容——见设计风险章节「隧道二进制帧 chunk/base64 规则照抄官方
    62	Tunnel 实现」。
    63	
    64	升级只动本文件的 `image` tag，同时升级前端 `guacamole-common-js` 到对应版本。
    65	
    66	## 验证连通
    67	
    68	```bash
    69	# 端口探活
    70	nc -z 127.0.0.1 4822 && echo ok
    71	
    72	# guacd 日志（应看到 "Listening on host 0.0.0.0, port 4822"）
    73	docker compose logs guacd | tail
    74	```
    75	
    76	真实连接验证见设计文档「阶段 1 验收范围」（浏览器 RDP 连 Windows、
    77	VNC 连 Linux、iPad Safari 触控；Win11 Home 无 RDP server 走 VNC 兜底）。
     1	[English](README.md) | [中文](README.zh.md)
     2	
     3	<p align="center">
     4	  <img src="web/public/logo.svg" alt="Cockpit Logo" width="64" height="64">
     5	</p>
     6	
     7	<h1 align="center">Cockpit</h1>
     8	
     9	个人混合基础设施控制台，用于把分散在本地机房、云 VPS、NAT 后节点上的资源收敛到一个轻量 Server + Agent 控制面。
    10	
    11	[![Test](https://img.shields.io/github/actions/workflow/status/cuihairu/cockpit/test.yml?branch=main&logo=github&label=Test)](https://github.com/cuihairu/cockpit/actions/workflows/test.yml)
    12	[![Docs](https://img.shields.io/github/actions/workflow/status/cuihairu/cockpit/docs.yml?branch=main&logo=github&label=Docs)](https://github.com/cuihairu/cockpit/actions/workflows/docs.yml)
    13	[![codecov](https://codecov.io/gh/cuihairu/cockpit/branch/main/graph/badge.svg)](https://codecov.io/gh/cuihairu/cockpit)
    14	[![Agent: Windows](https://img.shields.io/badge/platform-Windows-0078D4?logo=windows&logoColor=white)](https://cuihairu.github.io/cockpit/guide/getting-started)
    15	[![Agent: Linux](https://img.shields.io/badge/platform-Linux-FCC624?logo=linux&logoColor=black)](https://cuihairu.github.io/cockpit/guide/getting-started)
    16	[![Agent: macOS](https://img.shields.io/badge/platform-macOS-000000?logo=apple&logoColor=white)](https://cuihairu.github.io/cockpit/guide/getting-started)
    17	[![Agent: x86_64](https://img.shields.io/badge/arch-x86__64-5B5B5B)](https://cuihairu.github.io/cockpit/guide/getting-started)
    18	[![Agent: aarch64](https://img.shields.io/badge/arch-aarch64-5B5B5B)](https://cuihairu.github.io/cockpit/guide/getting-started)
    19	
    20	## 核心特性
    21	
    22	面板把资源、执行、远控三条线收在一处。资源侧从 Inventory YAML 同步出统一资源视图，拨测心跳、到期告警、配置漂移检测都挂这份数据，告警发 Herald / ntfy / webhook / Telegram 四个渠道。执行侧有一本全机 Job 台账，对任意在线 agent 下发命令，状态、退出码、输出回写可查；Docker 容器全量管理，应用按 compose 文件以 Stacks 部署。远控侧终端、VNC、桌面走短期 ticket 转发并支持会话录制；反向代理站点、ACME 证书、DNS/DDNS、备份恢复、服务/Cron/SMART/NAS/组网观测也在同一面板操作。
    23	
    24	Agent 经 WebSocket 主动连出注册，NAT 后的节点不用暴露任何入站端口。
    25	
    26	## 快速开始
    27	
    28	> 前置：先有一台 Cockpit Server（Docker 一条起，部署机无需编译环境）：
    29	
    30	```bash
    31	cp deployments/docker/.env.example deployments/docker/.env && vi deployments/docker/.env
    32	docker compose -f deployments/docker/docker-compose.yml up -d
    33	```
    34	
    35	### Linux
    36	
    37	1. 下载并安装（自动检测架构，从每日构建匿名直链下载，装完自动验证）：
    38	
    39	   ```bash
    40	   curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/install.sh | bash
    41	   ```
    42	
    43	2. 连接服务器（示例用本站域名，替换为你的 Cockpit 服务端地址）：
    44	
    45	   ```bash
    46	   cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
    47	   ```
    48	
    49	3. 验证安装：
    50	
    51	   ```bash
    52	   cockpit-agent --version
    53	   ```
    54	
    55	   验证成功预期输出：
    56	
    57	   ```text
    58	   Cockpit Agent v20261001
    59	   ```
    60	
    61	   （版本号为安装当日日期；agent 日志出现 `Registered as agent: ...` 即已连上服务器）
    62	
    63	### macOS
    64	
    65	1. 下载并安装（与 Linux 共用 `install.sh`，自动识别 Darwin 与 arm64/amd64）：
    66	
    67	   ```bash
    68	   curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/install.sh | bash
    69	   ```
    70	
    71	2. 连接服务器：
    72	
    73	   ```bash
    74	   cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
    75	   ```
    76	
    77	3. 验证安装：
    78	
    79	   ```bash
    80	   cockpit-agent --version
    81	   ```
    82	
    83	   验证成功预期输出：
    84	
    85	   ```text
    86	   Cockpit Agent v20261001
    87	   ```
    88	
    89	### Windows
    90	
    91	**图形化安装器（推荐）**：从 [nightly release](https://github.com/cuihairu/cockpit/releases/latest)
    92	下载 `cockpit-agent-setup-nightly.exe`（同名 `.sha256` 为校验文件），双击运行：
    93	装到 `%ProgramFiles%\Cockpit Agent`、开始菜单 + 桌面快捷方式（带 agent 图标）、
    94	向导内填写 Server 地址、可勾选「注册 Windows 服务并开机自启」、控制面板标准
    95	卸载条目。静默装机：
    96	
    97	```powershell
    98	.\cockpit-agent-setup-nightly.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SERVER=wss://cockpit.cuihairu.site/ws
    99	```
   100	
   101	服务管理：`cockpit-agent service install|uninstall|start|stop|status`（服务名
   102	`CockpitAgent`，日志 `%ProgramData%\CockpitAgent\agent.log`）。
   103	
   104	**PowerShell 脚本路径**（免管理员，装到当前用户目录并写入用户 PATH）：
   105	
   106	1. 下载并安装：
   107	
   108	   ```powershell
   109	   irm https://raw.githubusercontent.com/cuihairu/cockpit/main/install.ps1 | iex
   110	   ```
   111	
   112	2. 连接服务器（新开一个终端使 PATH 生效）：
   113	
   114	   ```powershell
   115	   cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
   116	   ```
   117	
   118	3. 验证安装：
   119	
   120	   ```powershell
   121	   cockpit-agent --version
   122	   ```
   123	
   124	   验证成功预期输出：
   125	
   126	   ```text
   127	   Cockpit Agent v20261001
   128	   ```
   129	
   130	## 本地测试口径
   131	
   132	CI 与本地开发按改动面三路分流——改一行文档不该全量编译。
   133	
   134	1. **文档改动**（`*.md`、`docs/**`、图片）：不编译。主 CI workflow 的
   135	   `paths-ignore` 已跳过这些路径。
   136	2. **代码改动**：只跑改动子集，不全量。
   137	   - `go test ./internal/agent/<改动包>/...`
   138	   - `go test ./cmd/cockpit-agent/...`
   139	3. **全量**（`go test ./internal/agent/... ./cmd/cockpit-agent/...`）：仅以下情形触发——go.mod / go.sum、
   140	   `.github/**`、Docker/Compose 配置、发布前。
[0m$ [0mnl -ba docs/guide/workflow-design.md | sed -n '1,150p'; nl -ba docs/guide/jobs-design.md | sed -n '10,35p;120,150p'; nl -ba docs/guide/architecture.md | sed -n '95,175p'; nl -ba docs/guide/protocol.md | sed -n '208,270p'
1	# P2 方案设计：Workflow 与异步 Job
     2	
     3	> 语言：**中文** ｜ [English](./workflow-design.en.md)
     4	
     5	
     6	> 2026-10-09 立项；实现进度以本文「实现状态」节为准（M1 异步 Job + 线性
     7	> Workflow 当日落地，M2a 变量传递 / M2b 多目标扇出 / M2c 取消信号相继实现）。
     8	> P1 执行 Job 模型（[jobs-design](./jobs-design)）已落地 `agent.exec` 最小闭环；
     9	> 本设计是它的下一生长点：Job 异步化 + 线性
    10	> Workflow 编排。设计边界遵守战略评审定调：Workflow 是「一组 Job 的编排」，
    11	> 只做控制面接线，不重造底层调度器。
    12	
    13	## 为什么现在做
    14	
    15	P1 的同步 RPC 模型（POST 即等待终态）有两个天花板：
    16	
    17	1. **最长 300s**：`agent.exec` 的 `timeout_s` 上限 300，浏览器 HTTP 请求
    18	   等不了更长；超过即 HTTP 层先断，台账里还可能出现「HTTP 断了但 Job 落了
    19	   终态」的双头现象。
    20	2. **编排无处挂接**：停机 → 备份 → 升级 → 起机 → 健康检查这类多步动作，
    21	   现在只能人手一步步点，或各自写脚本调 API。步骤失败后的处置（停 or 继续、
    22	   重试几次）没有落点。
    23	
    24	异步化不是独立功能——它是 Workflow 的前置：编排器必然在后台推进步骤，
    25	不可能同步等待。两者一并设计，一次把状态机扩到位。
    26	
    27	## 实现状态
    28	
    29	- **M1 已实现（2026-10-09）**：异步 Job（W1/W2/W3 单条语义）+ 线性 Workflow
    30	  （W4-W9）+ 取消 + 台账过滤 + 权限/审计，全链 `internal/storage/workflow.go`、
    31	  `internal/server/api_workflow.go`、`web/src/pages/Workflows/`、
    32	  `web/src/services/workflows.ts`；POST /api/jobs 全异步化（W1）为破坏性语义
    33	  变更（原同步等待删除，消费者仅 web 一处）。真机验收按「验收清单」Workflow
    34	  节推进（探针四场景：成功链/失败停/重试/取消）。
    35	- **M2a 已实现（2026-10-10）**：步骤变量传递（M2 候选清单第一项），设计见
    36	  「M2 立项：步骤变量传递」节，824eb88 立项、97696ad 落地（含连带修复：
    37	  dispatchJob 改传解析后参数，见 V3）。
    38	- **M2b 已实现（2026-10-10）**：多目标扇出（M2 候选第二项），设计见「M2 立项：
    39	  多目标扇出」节——server 侧顺序扇出、agent 零改动；web 步骤编辑器多选 +
    40	  run 详情「N 台」聚合展开逐台明细同批落地；真机验收探针扩至 W10（双台
    41	  扇出成功/失败停/ghost 混入，12/12 PASS）。余下候选当时均未排期（取消信号
    42	  已于 10-11 作为 M2c 落地，见下条）。
    43	- **M2c 已实现（2026-10-11）**：取消信号（M2 候选第三项），设计见「M2 立项：
    44	  取消信号」节——`job.cancel` RPC 真击杀在途进程（C1-C10，含 pending 窗口
    45	  DB-only 条件取消与 fanout 聚合口径修复）；web Jobs 页 running/pending 行
    46	  取消入口同批落地。真机验收探针 W5/W9 语义翻新至击杀口径、新增 W11 变量
    47	  传递场景（M2a 端到端面），本机双实例 harness 13/13 PASS。DAG 维持 W4 压后、
    48	  审批门因孤儿 run 未解不开工、SSE 拍不做。
    49	- **M2d 已实现（2026-10-11）**：审批门（M2 候选第四项），设计见「M2 立项：
    50	  审批门」节——前置阻塞点（server 重启孤儿 run）已由 9079b52 重启清扫解掉，
    51	  本批把等待态落库不落 goroutine（D1），高危提交 202 进审批、批准才派发。
    52	  权限点 `approvals:read/write` 的注册当时漏登记（路由已挂 `approvals` 资源但
    53	  `resourceActions` 无该键），全角色对 `/api/approvals` 403、web「审批门」菜单
    54	  永不渲染——2026-10-15 文档审计发现并补齐（含回归测试）。
    55	  审批台账页 + Jobs 页「需审批执行」开关同批落地。DAG 列 M2 剩余序列末位、
    56	  SSE 拍不做。
    57	- **M2e 已实现（2026-10-12）**：DAG 依赖（M2 候选第五项），设计见「M2 立项：
    58	  DAG 依赖」节——步骤级 `depends_on` 依赖门控（G1-G8）：数组序即拓扑序，
    59	  依赖失败带 continue_on_error 时下游如实 skipped（步级新态），不引入并行
    60	  执行（同 F2 写路径决策）；web 依赖多选编辑器 + 时间线「已跳过」灰标 +
    61	  停因列。
    62	- M2+ 其余各项在「边界（明确不做）」；未实现项以本文设计为准。
    63	- P1 已实现部分不受影响：`agent.exec` 类型、job provider、权限点原样保留，
    64	  本设计只改「server 侧何时返回」与「谁在等待」。
    65	
    66	## 架构总览
    67	
    68	```
    69	┌─ server ────────────────────────────────────────────────┐
    70	│ POST /api/jobs          POST /api/workflows/{id}/run    │
    71	│   建 Job(pending) ──┐     建 Run + 逐步建 Job(pending)   │
    72	│   立即返回 201      │            │                        │
    73	│                     ▼            ▼                        │
    74	│           dispatcher goroutine 编排器 goroutine          │
    75	│           CallAgent(job.exec)    逐步：建 Job → 等终态    │
    76	│           终态回写               失败按 continue_on_error  │
    77	│                                  决定停/续，run 落终态     │
    78	│ GET /api/jobs?status=&run=   GET /api/workflows/{id}/runs │
    79	└─────────────────────────────────────────────────────────┘
    80	```
    81	
    82	要点：**agent 侧零改动**。`job.exec` RPC 本就是「带超时同步执行、返回
    83	exit_code/输出」，编排器和单步 dispatcher 都只是 server 侧的 goroutine，
    84	等的是同一个 RPC。取消能力受 RPC 无取消帧的约束（D3），如实声明。
    85	
    86	## 关键决策
    87	
    88	| # | 决策 | 选择 | 理由 |
    89	|---|------|------|------|
    90	| W1 | POST /api/jobs 语义 | **全异步**：创建后立即 201 返回 pending Job 视图，后台 dispatch；原同步等待删除 | 消费者仅 web 一处（mobile 未接 jobs API），无兼容负担；「创建即返回 id」是台账模型的本义，P1 的同步返回是权宜 |
    91	| W2 | 状态机 | `pending → running → success \| failed \| cancelled`；**timeout 不设独立态**（保持 `failed` + error=`timed out after Ns`） | error 字段已可判定超时，独立态只增加过滤面；cancelled 是异步化后才有的真实能力，必须独立（running 被取消的 Job 不该计作 failed） |
    92	| W3 | 取消语义 | 只允许取消 **pending**（未派发）Job；running 拒绝（409，说明 job.exec 上限 300s 会自然结束）。Workflow run 取消 = 停止推进后续步骤 + 当前步骤同规则 | RPC 协议无取消帧，假装能取消 running 是撒谎；诚实边界是「派发前可撤，派发后等它回来」。agent 侧信号注入取消留 M2 |
    93	| W4 | 编排形态 | **线性步骤链**（steps 有序数组，逐步执行），非 DAG；步骤级 `continue_on_error` 与 `retry`（0-3 次，固定间隔 5s） | 个人基础设施最高频就是串行链（停机→备份→升级→起机→检查）；DAG 的 edges 表达在 UI 和存储里成本翻倍，等真实并行需求出现再扩。重试覆盖最常见韧性需求 |
    94	| W5 | 步骤与 Job 的关系 | 每步创建**真实 Job**（`jobs` 表加可空列 `workflow_run_id`），run 台账里每步可点开看输出/退出码 | 步骤不是影子记录——它就是一次执行，进统一台账、进既有审计 `job_run`；不新造「步骤日志」平行体系 |
    95	| W6 | 存储 | `workflows`（定义）+ `workflow_runs`（实例）两表；run 内嵌 steps 运行态 JSON 快照（含每步状态/尝试次数/关联 Job id）；AutoMigrate | 个人规模读多写少，步骤子表是过度设计；快照即事实（当时怎么跑的、每步结果） |
    96	| W7 | run 状态机 | `running → success \| failed \| cancelled`（无 pending——建 run 即开始推进）；部分失败 = failed，details 记明断点 | run 是进程性实体，建了就跑；「排队中」的 run 没有消费者 |
    97	| W8 | 并发约束 | 同一 workflow 同时至多一个 active run（409 拒绝）；不同 workflow 可并行 | 个人规模防重入（连点两次「运行」）；不做全局限流 |
    98	| W9 | 权限 | `workflows:read` / `workflows:write`；run/cancel 归 write；步骤 Job 沿用 `jobs:*` 既有面 | 与 cron/Job 同口径：台账可读（read），执行高危（write） |
    99	| W10 | 审计 | 定义类操作 `workflow_create/update/delete`；run 终态一条 `workflow_run`（resourceID=run id，details 含 workflow 名/steps 摘要/断点）；步骤 Job 自带 `job_run` | 定义是变更、run 是执行，两类分开；步骤已有审计不翻倍 |
   100	| W11 | 触发 | 手动 `POST /api/workflows/{id}/run`。「定时跑 Workflow」= cron 写一条调该 API 的命令（P1 边界节既有约定） | cron 是 agent 侧调度面、Job/Workflow 是控制面执行台账，两个平面不耦合；server 侧定时器是另一个立项 |
   101	| W12 | 步骤参数 | `{"name","type","target","parameters","timeout_s"?,"continue_on_error"?,"retry"?}`；type 白名单沿用 `jobTypes`（M1 仅 `agent.exec`） | 与 Job payload 同构，校验复用 `validateJobPayload`；新类型（容器操作、备份触发）注册进白名单即自动可用于编排 |
   102	
   103	## REST API
   104	
   105	| 方法 | 路径 | 说明 |
   106	|------|------|------|
   107	| `GET /api/workflows` | — | 定义列表 |
   108	| `POST /api/workflows` | `{name, description?, steps[]}` | 创建定义（≤20 步） |
   109	| `GET /api/workflows/{id}` | — | 定义详情 |
   110	| `PUT /api/workflows/{id}` | 同创建 | 更新（active run 存在时 409） |
   111	| `DELETE /api/workflows/{id}` | — | 删除定义（历史 run 保留） |
   112	| `POST /api/workflows/{id}/run` | — | 创建 run 并后台推进，201 返回 run 视图 |
   113	| `GET /api/workflows/{id}/runs` | `?status=` | 该定义的 run 台账 |
   114	| `GET /api/workflow-runs/{rid}` | — | run 详情（含 steps 快照） |
   115	| `POST /api/workflow-runs/{rid}/cancel` | — | 取消 run（W3 语义） |
   116	| `GET /api/jobs` | `?status=&target=&type=&workflow_run_id=` | P1 台账加过滤（run 详情页列步骤用） |
   117	| `POST /api/jobs/{id}/cancel` | — | 取消 pending Job（W3） |
   118	| `POST /api/jobs`（M2d） | `{..., approval:"required"}` | 高危提交：202 返回审批视图，不建 Job；批准后原样派发 |
   119	| `GET /api/approvals`（M2d） | `?status=` | 审批台账（惰性过期清扫） |
   120	| `GET /api/approvals/{id}`（M2d） | — | 单条详情（payload 解析为 parameters） |
   121	| `POST /api/approvals/{id}/approve`（M2d） | — | 批准并派发（重复/过期 409；agent 离线 503） |
   122	| `POST /api/approvals/{id}/reject`（M2d） | — | 驳回（终态，不建 Job） |
   123	
   124	校验：name 非空 ≤64 字符；steps 非空 ≤20；step.name 同一 workflow 内唯一
   125	≤64；`timeout_s`/`command` 等沿用 P1 各限；`retry` 0-3。M2d：`approval`
   126	只收 `required`；进审批的 payload 必须先过 validateJobPayload（批准后原样
   127	派发，校验不可后置）。
   128	
   129	## 数据模型
   130	
   131	```go
   132	type Workflow struct {
   133	    ID          string    `gorm:"primaryKey" json:"id"`
   134	    Name        string    `json:"name"`
   135	    Description string    `json:"description,omitempty"`
   136	    Steps       string    `json:"-"` // JSON 串，API 层解析
   137	    CreatedBy   string    `json:"createdBy"`
   138	    CreatedAt   time.Time `json:"createdAt"`
   139	    UpdatedAt   time.Time `json:"updatedAt"`
   140	}
   141	
   142	type WorkflowRun struct {
   143	    ID           string     `gorm:"primaryKey" json:"id"`
   144	    WorkflowID   string     `gorm:"index" json:"workflowId"`
   145	    WorkflowName string     `json:"workflowName"` // 冗余，定义删除后台账可读
   146	    Status       string     `gorm:"index" json:"status"`
   147	    Actor        string     `json:"actor"`
   148	    Steps        string     `json:"-"` // 运行态快照 JSON（每步状态/尝试/JobID）
   149	    CreatedAt    time.Time  `json:"createdAt"`
   150	    FinishedAt   *time.Time `json:"finishedAt,omitempty"`
    10	## 实现状态（2026-10-04 对账）
    11	
    12	- **已实现**（22bd7fd → 4a0660f 六笔）：本文「最小闭环」范围全链落地——
    13	  `storage.Job` 模型 + AutoMigrate；Job 类型 `agent.exec` 走 agent 侧 `job.exec`
    14	  RPC provider（命令 ≤16KB、超时 ≤300s 缺省 60s、输出 64KB 截断、超时按
    15	  进程组 SIGKILL）；`GET|POST /api/jobs` + `GET /api/jobs/{id}`（同步等待终态
    16	  回写）；终态单条审计 `job_run`（details 含 type/target/status/params）；
    17	  Jobs 页（全机台账 5s 轮询 + 创建即执行弹窗 + 终态详情）；权限点
    18	  `jobs:read`/`jobs:write`（PermGuard 控制创建入口）。
    19	- **未实现**（按本文「边界（明确不做）」节，立项时再设计，不写进当前能力）：
    20	  输出流式、Action/Event 抽象与 AI 接线（P5）。
    21	- **已由 P2 落地（2026-10-09，[workflow-design](./workflow-design.md) M1）**：
    22	  异步执行/取消（`cancelled` 终态、pending 可撤、台账过滤）与 Workflow 编排；
    23	  `POST /api/jobs` 语义随之改为「创建即 201 返回 pending，后台派发」（原同步
    24	  等待删除），timeout 仍按设计保持 `failed`（不设独立终态）。取消面再由 M2c
    25	  扩至 running：`POST /api/jobs/{id}/cancel` 经 `job.cancel` RPC 击杀在途进程
    26	  （2026-10-11）。Workflow/定时触发中，定时部分仍不做（workflow-design W11 约定）。
    27	- **真机验收**（2026-10-05，探针 `scripts/acceptance/jobs/` 五件套 9/9 PASS，
    28	  acceptance-checklist「统一 Job 执行」节七项全勾）：执行链路（82ms 成功回执 /
    29	  exit 3 输出带回 / 超时 1.078s 按时返回且 `pgrep` 无孤儿 / 截尾恰 64KB 尾部保留）、
    30	  离线 503 不落幽灵、校验面 400×7、终态审计恰一条、权限双角色 403 全部实证。
    31	  **验收逮到 D8 路由面缺口**：`/api/jobs` 未登记 `resourceRules`，RBAC
    32	  governed=false 整体放行（viewer 可执行），修为补规则 + 三角色矩阵 7 例。
    33	
    34	## 现状与痛点
    35	
   120	## 测试
   121	
   122	- storage：CRUD/倒序/limit（`internal/storage/job_test.go`）；
   123	- agent rpc：成功/非零退出/超时杀整组/截尾/越限/未知 action（`internal/agent/rpc/cov_job_provider_test.go`，windows 恒适用用例直跑）；
   124	- server：校验各 400 分支、离线 503、成功/失败 dispatch、列表/详情/405/404（`internal/server/cov_jobs_test.go`，假 agent 注入）；
   125	- 覆盖率：防御分支登记 `tool/known_uncoverable.txt`（SQLite 查询/写入不可致错、auth 上下文单测惯例、协议不可变形）。
   126	
   127	## 边界（明确不做）
   128	
   129	- **不做异步执行**（**已由 P2 兑现，见顶部「已由 P2 落地」注记**）：创建即同步
   130	  等待终态。异步化（排队/取消/进度）等 Workflow 立项时一并设计，届时
   131	  cancelled/timeout 独立状态、`GET /jobs?status=running` 过滤、SSE 推送才有
   132	  真实的消费者（落地口径：`cancelled` 终态 + 台账过滤已实现；SSE 仍拍不做）；
   133	- **不做 Workflow/Action**：Workflow 是「一组 Job 的编排」（依赖/并行/重试），
   134	  Action 是「带语义的幂等操作」，都在 Job 之上，本期只有 Job 本体；
   135	- **不做定时触发**：Job 与 cron 是两个平面——cron 是 agent 侧调度面（crontab
   136	  即状态），Job 是控制面执行台账；「定时跑 Job」= cron 写一条调用 `POST
   137	  /api/jobs` 的命令行，不引入新耦合；
   138	- **不做输出实时流**：终态一次性回传。流式输出（WebSocket attach）留待
   139	  远控/终端域统一设计。
   140	
   141	## 后续生长点
   142	
   143	| 优先级 | 方向 | 依赖 |
   144	|--------|------|------|
   145	| P2 | Workflow：Job 编排（线性链/重试/取消，DAG/审批留 M2）——已落地（P2 异步化 + M1 编排 2026-10-09；M2a 变量/M2b 扇出/M2c 取消信号 2026-10-11，见 [workflow-design](./workflow-design)） | 本模型 |
   146	| P0′ | 事件模型：Job 终态事件进统一事件总线 | Event 抽象立项 |
   147	| P5 | AI 接线：Job API 即 AI 的执行面（agent.exec 之外补结构化类型） | 审计与限额先行 |
    95	- Gateways
    96	- Storages
    97	
    98	Server 的 `inventory.watch: true` 会监听单个 inventory 文件并热加载，当前实现复用 `inventory.Syncer`，与 `cockpit sync` 走同一套资源同步逻辑。`inventory.strict: true` 会把 inventory 路径缺失、解析失败或初始同步失败视为启动错误；默认 `false` 时只记录日志并继续启动 Server。
    99	
   100	### HTTP API 与 Web UI
   101	
   102	Web UI 默认请求 `/api`。公开接口包括登录、Token 刷新、TOTP 验证、密码重置和 `/health`。其他 `/api/` 资源接口通过 JWT 中间件保护。
   103	
   104	主要 API 面：
   105	
   106	- `/api/status`
   107	- `/api/me`、`/api/me/profile`、`/api/me/password`
   108	- `/api/settings`
   109	- `/api/agents`、`/api/agents/{id}/secret`
   110	- `/api/resources/{compute-instances|domains|certificates|services|gateways|storages}`
   111	- `/api/users`（管理员管理用户）
   112	- `/api/alerts`（告警列表与已读标记）
   113	- `/api/metrics/*`
   114	- `/api/proxies`
   115	- `/api/remote/*`（tickets / sessions / terminal / guacamole / vault）
   116	- `/api/admin/audit/*`
   117	- `/api/auth/totp/*`（generate / enable / disable 需 JWT；verify 公开）
   118	
   119	### 远程访问与代理
   120	
   121	远程终端与桌面连接（Guacamole 隧道）使用短期 ticket：
   122	
   123	1. 浏览器先携带 JWT 请求 `POST /api/remote/tickets`。
   124	2. Server 生成 5 分钟有效、单次使用 ticket。
   125	3. 浏览器连接 `/api/remote/terminal` 或 `/api/remote/guacamole`，把 ticket 放在 `Sec-WebSocket-Protocol`。
   126	4. Server 校验 ticket 后，把连接请求转成 `proxy_*` 消息发给对应 Agent（终端直连目标；桌面经 `guac:` 中继到 guacd）。
   127	5. Agent 连接目标服务并把数据通过 Server 转发回浏览器。
   128	
   129	这个边界保证浏览器不需要知道内网拓扑，也不需要直接连 Agent。
   130	远控目标默认只允许 `remote_control.allowed_targets` 中显式配置的主机名、IP 或 CIDR。
   131	
   132	## 协议边界
   133	
   134	Server 与 Agent 的消息统一为：
   135	
   136	```json
   137	{
   138	  "id": "message-id",
   139	  "type": "heartbeat",
   140	  "timestamp": 1710000000,
   141	  "payload": {}
   142	}
   143	```
   144	
   145	当前消息类型：
   146	
   147	| 方向 | 类型 |
   148	| --- | --- |
   149	| Agent -> Server | `register`, `rpc_response`, `proxy_close`, `proxy_error`, `probe_report`, `exec_log` |
   150	| Server -> Agent | `rpc_request`, `ping`, `proxy_new` |
   151	| 双向 | `heartbeat`（Agent 上报、Server 回 ACK）、`error`, `proxy_data` |
   152	
   153	`register` 必须是 Agent 建立 `/ws` 后的第一条消息。RPC 使用请求消息 ID 关联响应。代理数据使用 `proxyId`、`connId` 关联具体连接。
   154	
   155	## 安全边界
   156	
   157	- Server 管理员密码必须通过 `ADMIN_PASSWORD` 提供，长度至少 8 位。
   158	- 生产环境应设置 `PRODUCTION=true` 和强随机 `TOTP_ENCRYPTION_KEY`。
   159	- `ALLOWED_ORIGINS` 控制 WebSocket Origin；未设置时会接受所有来源，仅适合开发环境。
   160	- Web UI 静态目录由 `STATIC_DIR` 或 `server.static_dir` 指定；未设置时根路径只返回 API 运行提示。
   161	- Agent secret 是可选但推荐的 Agent 认证边界；已配置 secret 的 Agent 重连必须提供匹配密钥。
   162	- 远程连接 ticket 是内存态、5 分钟有效、单次使用；Server 重启后失效。
   163	
   164	## 当前实现边界
   165	
   166	- Inventory 的目录拆分模型尚未成为主要实现路径；当前 CLI 示例和 parser 以单文件 `version: v1` YAML 为准。
   167	- `internal/agent/rpc` 的 SystemProvider 始终注册；Docker/PVE/OpenWrt Provider 由 `Agent.setupProviders` 按检测到的 capability 和环境变量（如 `DOCKER_HOST`、`PVE_TOKEN_ID`、`OPENWRT_HOST` 等）条件注册，凭据缺失时仅记录日志跳过，不影响基础心跳。
   168	- `/api/remote/sessions` 是内存会话台账（pending/connected/closed/failed），已接线到 terminal / guacamole 活会话生命周期（D-2026-10-12-4）；不是完整连接配置持久化，重启后台账清空。
   169	- Web UI 中部分偏好配置保存在浏览器本地，不进入 Server 数据库；`/api/settings` 当前仅作为兼容入口，前端本地偏好以浏览器存储值为准。
   170	- `docs/archive`（原 `docs/superpowers`）保存 TOTP、通知模块等历史设计稿和实施计划，仅供追溯，不代表当前运行架构；详见 [`docs/archive/README.md`](../archive/README.md)。
   208	远程目标默认只允许连接 `remote_control.allowed_targets` 中显式配置的主机名、IP 或 CIDR；设置 `remote_control.allow_arbitrary_target: true` 后才允许任意目标。当前实现不会从 inventory 自动派生远控 allow-list。
   209	
   210	RDP/VNC/SSH 桌面走 Guacamole 隧道（`/api/remote/guacamole`）：guacd 终结协议，Agent 只做 `guac:` 前缀的 TCP 中继，无特殊构建要求。
   211	
   212	## HTTP API
   213	
   214	公开接口：
   215	
   216	- `GET /health`
   217	- `POST /api/auth/login`
   218	- `POST /api/auth/refresh`
   219	- `POST /api/auth/totp/verify`
   220	- `POST /api/auth/forgot-password`
   221	- `POST /api/auth/reset-password`
   222	- `POST /api/auth/verify-reset-code`
   223	
   224	JWT 保护接口：
   225	
   226	- `GET /api/status`
   227	- `GET /api/me`
   228	- `PUT /api/me/profile`
   229	- `POST|PUT /api/me/password`
   230	- `GET|PUT /api/settings`
   231	- `GET /api/agents`
   232	- `GET /api/agents/{id}`
   233	- `GET|POST|DELETE /api/agents/{id}/secret`
   234	- `GET /api/resources/{type}`
   235	- `GET /api/resources/{type}/{id}`
   236	- `GET|POST /api/users`（创建限管理员）
   237	- `PUT|DELETE /api/users/{id}`
   238	- `POST /api/users/{id}/password`
   239	- `GET /api/alerts`
   240	- `PUT /api/alerts/read-all`
   241	- `GET|PUT|DELETE /api/alerts/{id}`（`PUT /api/alerts/{id}/read` 标记已读）
   242	- `GET /api/metrics/snapshots`
   243	- `GET /api/metrics/snapshot?agent_id=...`
   244	- `GET /api/metrics/history?agent_id=...`
   245	- `GET|POST|PUT|PATCH|DELETE /api/proxies`
   246	- `GET /api/proxies/status`
   247	- `POST /api/remote/tickets`
   248	- `GET|POST /api/remote/sessions`
   249	- `GET|DELETE /api/remote/sessions/{id}`
   250	- `POST /api/remote/vault/verify`（密码/TOTP 二次验证，签发 10 分钟 vault token）
   251	- `GET|PUT /api/remote/vault/credentials`（凭据元数据列表 / 加密落库保存，明文不出库；写操作需 `X-Vault-Token`）
   252	- `DELETE /api/remote/vault/credentials/{id}`（同上）
   253	- `GET|POST /api/jobs`（统一 Job 台账（支持 `status/target/type/workflow_run_id` 过滤）/ 创建 Job，仅 `agent.exec`；创建异步：201 返回 pending，后台派发，终态经台账轮询）
   254	- `GET /api/jobs/{id}`
   255	- `POST /api/jobs/{id}/cancel`（取消 Job：pending 直接条件取消（W3），running 经 `job.cancel` RPC 击杀在途进程后落 `cancelled`（M2c C6）；终态一律 409）
   256	- `GET|POST /api/workflows`（Workflow 编排定义列表 / 创建，线性步骤链 ≤20 步，每步即一条 Job）
   257	- `GET|PUT|DELETE /api/workflows/{id}`（定义详情 / 更新 / 删除；active run 在途时变更 409）
   258	- `POST /api/workflows/{id}/run`（创建 run 并后台推进，201 返回 run 视图；同定义重入 409）
   259	- `GET /api/workflows/{id}/runs`（该定义的 run 台账）
   260	- `GET /api/workflow-runs/{rid}`（run 详情，含步骤快照）
   261	- `POST /api/workflow-runs/{rid}/cancel`（取消 run：置 `cancelled` 并经 `job.cancel` 击杀在途步骤 Job、pending 窗口 DB-only 条件取消（M2c C8），后续步骤不推进；已终态 409）
   262	- `GET /api/admin/audit/logs`
   263	- `GET /api/admin/audit/stats`
   264	
   265	> 本节为代表性契约面，非全量端点清单：各功能域（如远控凭据保险箱、服务管理、
   266	> 备份、日志检索）的端点以对应设计文档的 API 段与 server 路由注册为准。
   267	
   268	TOTP 设置接口（同样需要 JWT，与上面的公开 `totp/verify` 区分）：
   269	
   270	- `POST /api/auth/totp/generate`
[0m$ [0mrg -n "HandleFunc\(|Handle\(" internal/server --glob '*.go' | grep -v _test | sort
internal/server/api_audit.go:165:	mux.HandleFunc("/api/admin/audit/logs", func(w http.ResponseWriter, r *http.Request) {
internal/server/api_audit.go:169:	mux.HandleFunc("/api/admin/audit/export", func(w http.ResponseWriter, r *http.Request) {
internal/server/api_audit.go:172:	mux.HandleFunc("/api/admin/audit/stats", func(w http.ResponseWriter, r *http.Request) {
internal/server/api_backups.go:35:	mux.HandleFunc(backupsAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
internal/server/api_docker.go:18:	mux.HandleFunc(dockerAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
internal/server/api_guacamole.go:290:	mux.HandleFunc("/api/remote/guacamole", s.handleGuacamoleWebSocket)
internal/server/api_metrics.go:13:	mux.HandleFunc("/api/metrics/snapshots", func(w http.ResponseWriter, r *http.Request) {
internal/server/api_metrics.go:18:	mux.HandleFunc("/api/metrics/snapshot", func(w http.ResponseWriter, r *http.Request) {
internal/server/api_metrics.go:23:	mux.HandleFunc("/api/metrics/history", func(w http.ResponseWriter, r *http.Request) {
internal/server/api_probe.go:37:	mux.HandleFunc(probeAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
internal/server/api_probe.go:40:	mux.HandleFunc(notificationAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
internal/server/api_proxy.go:341:	mux.HandleFunc("/api/proxies", func(w http.ResponseWriter, r *http.Request) {
internal/server/api_proxy.go:356:	mux.HandleFunc("/api/proxies/status", func(w http.ResponseWriter, r *http.Request) {
internal/server/api_remote.go:386:	mux.HandleFunc("/api/remote/tickets", s.authService().Middleware(s.handleTicketCreate))
internal/server/api_remote.go:387:	mux.HandleFunc("/api/remote/terminal", s.handleTerminalWebSocket)
internal/server/api_remote.go:388:	mux.HandleFunc("/api/remote/sessions", s.authService().Middleware(s.handleRemoteSessions))
internal/server/api_remote.go:389:	mux.HandleFunc("/api/remote/sessions/", s.authService().Middleware(s.handleRemoteSession))
internal/server/api_stacks.go:48:	mux.HandleFunc(stacksAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
internal/server/api_stacks.go:53:	mux.HandleFunc("POST /api/stacks/webhook", s.handleStackWebhook)
internal/server/api_vault.go:82:	mux.HandleFunc("/api/remote/vault/verify", s.authService().Middleware(s.handleVaultVerify))
internal/server/api_vault.go:83:	mux.HandleFunc("/api/remote/vault/credentials", s.authService().Middleware(s.handleVaultCredentials))
internal/server/api_vault.go:84:	mux.HandleFunc("/api/remote/vault/credentials/", s.authService().Middleware(s.handleVaultCredentialDelete))
internal/server/api_vault.go:87:	mux.HandleFunc("/api/remote/vault/entries", s.authService().Middleware(s.handleVaultEntries))
internal/server/api_vault.go:88:	mux.HandleFunc("/api/remote/vault/entries/", s.authService().Middleware(s.handleVaultEntry))
internal/server/server.go:330:	mux.HandleFunc("/ws", s.handleWebSocket)
internal/server/server.go:331:	mux.HandleFunc("/health", s.handleHealth)
internal/server/server.go:332:	mux.HandleFunc("/api/auth/login", s.handleLoginWithAudit)
internal/server/server.go:333:	mux.HandleFunc("/api/auth/refresh", s.authService().HandleRefresh)
internal/server/server.go:334:	mux.HandleFunc("/api/auth/totp/verify", s.handleTOTPVerify) // TOTP 验证不需要 JWT（使用临时令牌）
internal/server/server.go:335:	mux.HandleFunc("/api/auth/forgot-password", s.handleForgotPassword)
internal/server/server.go:336:	mux.HandleFunc("/api/auth/reset-password", s.handleResetPassword)
internal/server/server.go:337:	mux.HandleFunc("/api/auth/verify-reset-code", s.handleVerifyResetCode)
internal/server/server.go:340:	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
internal/server/server.go:365:	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
[0m$ [0mnl -ba internal/server/api_guacamole.go | sed -n '380,530p'; nl -ba deployments/guacd/docker-compose.yml | sed -n '1,130p'; nl -ba docs/guacd-deploy.md | sed -n '1,120p'; nl -ba docs/remote-access-integration-design.md | sed -n '50,115p'; nl -ba docs/guide/architecture.en.md | sed -n '118,135p'; nl -ba docs/guide/protocol.en.md | sed -n '165,185p'
380		// 3.5 目标转发器：guacd 拨回环中继，字节流经 agent WS 通道到 agent、
   381		// 由 agent 拨真实目标（架构：agent 是目标内网的唯一落点，guacd 不直拨
   382		// ——设计修正 2026-10-02）。中继起不来按 agent 不可达处理。
   383		relay, relayAddr, err := startGuacRelay(agentID, guacRelayPrefix+sessionID,
   384			net.JoinHostPort(host, portStr), s.SendToAgent)
   385		if err != nil {
   386			log.Printf("Guacamole: start target relay failed: %v", err)
   387			s.auditRemoteFailure(ticket.UserID, ticket.Username, r.RemoteAddr, r.UserAgent(),
   388				&audit.RemoteSessionDetails{
   389					Protocol: protocolStr,
   390					AgentID:  agentID,
   391					Host:     host,
   392					Port:     port,
   393					Egress:   egressMatch.summary(),
   394					Reason:   "target relay unavailable",
   395				})
   396			_ = guacdConn.Close()
   397			http.Error(w, `{"error":"Agent relay unavailable"}`, http.StatusBadGateway)
   398			return
   399		}
   400	
   401		// 4. 升级 WS。子协议必须回显其一：浏览器按 RFC 6455，服务端未选中任何
   402		// 所 offering 的子协议时直接判握手失败（1006 断开）。common-js 的
   403		// WebSocketTunnel 硬编码 offering "guacamole"；自定义 Tunnel 用票据作
   404		// 子协议传票据时回显票据本身（真机验收发现缺省不回显，2026-09-30）。
   405		up := websocket.Upgrader{
   406			CheckOrigin:     isOriginAllowed,
   407			ReadBufferSize:  guacamoleReadBuf,
   408			WriteBufferSize: guacamoleReadBuf,
   409			Subprotocols:    []string{"guacamole"},
   410		}
   411		if ticketID != "guacamole" {
   412			up.Subprotocols = append(up.Subprotocols, ticketID)
   413		}
   414		conn, err := up.Upgrade(w, r, nil)
   415		if err != nil {
   416			log.Printf("Guacamole WebSocket upgrade failed: %v", err)
   417			_ = guacdConn.Close()
   418			relay.Close()
   419			return
   420		}
   421	
   422		session := &GuacamoleSession{
   423			ID:       sessionID,
   424			UserID:   ticket.UserID,
   425			Username: ticket.Username,
   426			Protocol: protocolStr,
   427			AgentID:  agentID,
   428			Host:     host,
   429			Port:     port,
   430			relay:    relay,
   431			ClientWS: conn,
   432			guacd:    guacdConn,
   433			Created:  time.Now(),
   434			done:     make(chan struct{}),
   435		}
   436	
   437		// 会话台账登记（D-2026-10-12-4）：guacamole 会话 ID 与控制面同源，
   438		// 使台账状态（pending→connected→closed/failed）与活会话生命周期对齐。
   439		s.remoteSessions.Register(sessionID, ticket.UserID, ticket.Username, RemoteSessionRequest{
   440			AgentID:  agentID,
   441			Protocol: protocol.RemoteProtocol(protocolStr),
   442			Host:     host,
   443			Port:     port,
   444		})
   445		guacSessionsMu.Lock()
   446		guacSessions[sessionID] = session
   447		guacSessionsMu.Unlock()
   448	
   449		// 5a. tunnel UUID 先发浏览器（INTERNAL_DATA 单元素指令，即
   450		// Guacamole.Tunnel.INTERNAL_DATA_OPCODE 空 opcode）：common-js 首条指令
   451		// 若为内部指令单元素则 setUUID，否则仅置 OPEN（uuid 留 null）。
   452		// 这是 tunnel 层（浏览器↔网关），不进 guacd。
   453		session.writeWS([]byte(guacEncode("", sessionID)))
   454	
   455		// 5b. 握手（Guacamole 协议，设计 line 91 的 select/size/connect 序列）：
   456		// select → guacd 回参数列表 → size + 媒体能力声明 + connect。
   457		// audio/video 空声明 = 不启用（音频放阶段二，见设计风险章节）；image 必须声明：
   458		// 官方架构里 guacd 握手由服务端（Java webapp）代客户端完成，always 声明
   459		// image/png——若不声明，guacd 侧 user->info.image_mimetypes 为 NULL，渲染
   460		// 走 guac_user_supports_webp 遍历该数组时空指针解引用，guacd 子进程直接
   461		// segfault（首帧能渲染、一旦有输入触发新字形 img 流即断，VNC/RDP 同理）。
   462		// guacamole/guacd 1.5.5 与 1.6.0 均复现（真机验收定位，2026-09-30）。
   463		hsReader := bufio.NewReaderSize(guacdConn, guacamoleReadBuf)
   464		if _, err := guacdConn.Write([]byte(guacEncode("select", protocolStr))); err != nil {
   465			log.Printf("Guacamole: select write failed: %v", err)
   466			conn.Close()
   467			_ = guacdConn.Close()
   468			relay.Close()
   469			s.remoteSessions.UpdateStatus(sessionID, RemoteSessionStatusFailed, "select write failed")
   470			return
   471		}
   472		// 读 guacd 的 select 响应：args 参数名列表。connect 必须与这份列表逐位
   473		// 对齐（含 VERSION 版本位）——解析出名字再按位组装值
   474		// （真机验收发现稀疏形态被真 guacd 拒绝，2026-09-30）
   475		argsLine, err := hsReader.ReadString(';')
   476		if err != nil {
   477			log.Printf("Guacamole: read select response failed: %v", err)
   478			conn.Close()
   479			_ = guacdConn.Close()
   480			relay.Close()
   481			s.remoteSessions.UpdateStatus(sessionID, RemoteSessionStatusFailed, "read select response failed")
   482			return
   483		}
   484		argNames := guacParseArgNames(argsLine)
   485		if len(argNames) == 0 {
   486			log.Printf("Guacamole: guacd args response malformed: %q", argsLine)
   487			conn.Close()
   488			_ = guacdConn.Close()
   489			relay.Close()
   490			s.remoteSessions.UpdateStatus(sessionID, RemoteSessionStatusFailed, "malformed args response")
   491			return
   492		}
   493		if width <= 0 {
   494			width = 1280
   495		}
   496		if height <= 0 {
   497			height = 800
   498		}
   499		// recording-name 用会话 ID（可追溯到审计的 Session 字段）+ .guac 后缀：
   500		// guacd 把 recording-name 原样作文件名（不补后缀），不带后缀则
   501		// collectGuacRecording 的 <sid>.guac 永远 Stat 不到（真机验收发现
   502		// 2026-09-30：rec/ 里全是无后缀孤儿文件、录制页恒空）
   503		recording := s.recordingEnabled()
   504		session.Recording = recording
   505		// SSH 密钥自动获取：票据既没传 private_key 也没传 password 时，向 agent
   506		// 要默认密钥兜底（门控细则见 applySSHDefaultKey）
   507		if protocolStr == "ssh" {
   508			s.applySSHDefaultKey(ticket.Params, agentID, host)
   509		}
   510	
   511		// connect 的 hostname/port 用中继地址（guacd 拨回环，真实目标由 agent 拨）；
   512		// username/password/private-key 等凭据参数语义不变
   513		relayHost, relayPortStr, err := net.SplitHostPort(relayAddr)
   514		if err != nil {
   515			log.Printf("Guacamole: relay addr malformed %q: %v", relayAddr, err)
   516			conn.Close()
   517			_ = guacdConn.Close()
   518			relay.Close()
   519			s.remoteSessions.UpdateStatus(sessionID, RemoteSessionStatusFailed, "relay addr malformed")
   520			return
   521		}
   522		relayPort, _ := strconv.Atoi(relayPortStr)
   523		connectArgs := guacConnectArgs(argNames, protocolStr, relayHost, relayPort, ticket.Params, width, height,
   524			recording, sessionID+".guac")
   525		handshake := guacEncode("size", strconv.Itoa(width), strconv.Itoa(height), "96") +
   526			guacEncode("audio") + // 空 = 不启用音频（阶段二）
   527			guacEncode("video") + // 空 = 不启用视频
   528			guacEncode("image", "image/png") + // 必须声明，见上方 5b 注释（guacd segfault 根因）
   529			guacEncode("connect", connectArgs...)
   530		if _, err := guacdConn.Write([]byte(handshake)); err != nil {
     1	# guacd：Apache Guacamole 协议守护进程（RDP/VNC → Guacamole 指令流翻译）
     2	#
     3	# 见 docs/remote-desktop-guacamole-design.md（D1/D5）：
     4	# - guacd 无用户概念、无状态——会话/权限/审计在 Cockpit server（Go 网关）
     5	# - Go 网关反代此端口（浏览器不直连 guacd），端口只绑回环/内网
     6	# - GUACD_LOG_LEVEL 别开 debug：debug 会打印 connect 指令参数（含口令）
     7	#
     8	# 部署：docker compose up -d
     9	# 交叉验证：docker compose config
    10	
    11	services:
    12	  guacd:
    13	    image: guacamole/guacd:1.5.5
    14	    container_name: cockpit-guacd
    15	    restart: unless-stopped
    16	    ports:
    17	      # Go 网关同机反代：绑回环，不对外暴露（guacd 不是 HTTP 服务，
    18	      # 也不是浏览器可达的端点——浏览器走 /api/remote/guacamole WS）
    19	      # 跨机部署时改绑内网地址（如 "192.168.x.x:4822:4822"）并靠防火墙收口
    20	      - "${GUACD_BIND:-127.0.0.1}:${GUACD_PORT:-4822}:4822"
    21	    volumes:
    22	      # guacd session recording 落盘目录（connect 指令的 recording-path 指向这里，
    23	      # 白捡桌面会话审计回放，见设计「guacd session recording」节）
    24	      - guacd-recordings:/var/lib/guacamole:rw
    25	      # 阶段二文件传输（RDP 驱动器重定向）预留
    26	      - guacd-drive:/var/lib/guacamole/drive:rw
    27	    environment:
    28	      # info 即可；debug 会打出 connect 指令参数（含口令），是泄漏面
    29	      GUACD_LOG_LEVEL: ${GUACD_LOG_LEVEL:-info}
    30	      TZ: ${TZ:-Asia/Shanghai}
    31	    healthcheck:
    32	      # guacd 无 HTTP 健康端点；用 TCP 探活 4822
    33	      test: ["CMD-SHELL", "nc -z 127.0.0.1 4822 || exit 1"]
    34	      interval: 30s
    35	      timeout: 3s
    36	      retries: 3
    37	
    38	volumes:
    39	  guacd-recordings:
    40	  guacd-drive:
     1	# guacd 部署指南
     2	
     3	> 语言：**中文** ｜ [English](./guacd-deploy.en.md)
     4	
     5	## 概述
     6	
     7	guacd（Apache Guacamole Daemon）是 Cockpit 远程桌面网关的协议翻译层。浏览器通过 Cockpit Server（Go 网关）与 guacd 通信，guacd 负责 SSH/VNC/RDP 协议终结。
     8	
     9	```
    10	浏览器 ↔ Cockpit Server (Go) ↔ guacd (TCP 4822) ↔ 目标机器
    11	```
    12	
    13	## 快速部署
    14	
    15	### Docker Compose（推荐）
    16	
    17	```bash
    18	# 创建目录
    19	mkdir -p /opt/cockpit/guacd
    20	
    21	# 下载 compose 文件
    22	curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/deployments/guacd/docker-compose.yml \
    23	  -o /opt/cockpit/guacd/docker-compose.yml
    24	
    25	# 启动
    26	cd /opt/cockpit/guacd
    27	docker compose up -d
    28	
    29	# 验证
    30	docker compose ps        # 状态应为 healthy
    31	nc -z 127.0.0.1 4822     # 端口应可达
    32	```
    33	
    34	### Docker 命令
    35	
    36	```bash
    37	docker run -d \
    38	  --name cockpit-guacd \
    39	  --restart unless-stopped \
    40	  -p 127.0.0.1:4822:4822 \
    41	  -e GUACD_LOG_LEVEL=info \
    42	  -e TZ=Asia/Shanghai \
    43	  -v guacd-recordings:/var/lib/guacamole \
    44	  guacamole/guacd:1.5.5
    45	```
    46	
    47	## 配置
    48	
    49	### 环境变量
    50	
    51	| 变量 | 说明 | 默认值 |
    52	|------|------|--------|
    53	| `GUACD_LOG_LEVEL` | 日志级别（别开 debug，会打印密码） | `info` |
    54	| `GUACD_BIND` | 绑定地址（compose 文件用） | `127.0.0.1` |
    55	| `GUACD_PORT` | 端口（compose 文件用） | `4822` |
    56	| `TZ` | 时区 | `Asia/Shanghai` |
    57	
    58	### Cockpit Server 端
    59	
    60	Cockpit Server 通过 `GUACD_ADDR` 环境变量连接 guacd：
    61	
    62	```bash
    63	# 默认值（同机部署）
    64	GUACD_ADDR=127.0.0.1:4822
    65	
    66	# 跨机部署时指向 guacd 主机内网地址
    67	GUACD_ADDR=192.168.5.10:4822
    68	```
    69	
    70	在 `/etc/cockpit/config.yaml` 同机部署不需要额外配置。
    71	
    72	## SSH 密钥认证
    73	
    74	guacd 本身不存储密钥。SSH 密钥由 **Cockpit Agent** 管理：
    75	
    76	1. Agent 启动时自动检查 `~/.ssh/`，无密钥则生成 Ed25519
    77	2. 用户在 Web 发起 SSH 连接且**既未填私钥也未填口令**时，Server 才调用 Agent 的 `ssh.getDefaultKey` RPC 获取默认私钥（显式凭据一律原样透传）
    78	3. Server 将私钥 PEM 传给 guacd 的 `private-key` 参数
    79	4. guacd 用该密钥认证连接目标机器
    80	
    81	用户只需将 Agent 的公钥添加到目标机器的 `authorized_keys`：
    82	
    83	```bash
    84	# 查看 Agent 公钥
    85	cat /home/cui/.ssh/id_ed25519.pub
    86	
    87	# 添加到目标机器
    88	ssh-copy-id -i /home/cui/.ssh/id_ed25519.pub user@target-host
    89	```
    90	
    91	## 数据卷
    92	
    93	| 卷 | 用途 |
    94	|------|------|
    95	| `guacd-recordings` | 桌面会话录制文件（`.guac`） |
    96	| `guacd-drive` | RDP 驱动器重定向（阶段二预留） |
    97	
    98	录制文件由 Cockpit Server 在会话结束后自动收走归档到数据卷的
    99	`/data/recordings/` 目录（元数据回填数据库；`collectGuacRecording`）。
   100	
   101	## 健康检查
   102	
   103	```bash
   104	# 容器健康状态
   105	docker inspect cockpit-guacd --format='{{.State.Health.Status}}'
   106	
   107	# TCP 探活
   108	nc -z 127.0.0.1 4822
   109	
   110	# 日志
   111	docker logs cockpit-guacd --tail 20
   112	```
   113	
   114	## 升级
   115	
   116	```bash
   117	cd /opt/cockpit/guacd
   118	docker compose pull
   119	docker compose up -d
   120	```
    50	| **WebSSH / sshwifty / ttyd** 等独立终端网关 | 仅 SSH | 多部署一个服务、多一套账号体系；票据/审计/出口策略要二次打通。guacd 已在部署内。 |
    51	| **server 直连 crypto/ssh 自行开发网关** | 仅 SSH | 等于把 agent 通道平移到 server，仍要自行开发录制与协议维护；与「集成第三方方案」方向相反。 |
    52	| **Teleport** | SSH/K8s/DB | 企业级全家桶（自带用户库/证书体系/Proxy），个人 homelab 场景过重。 |
    53	| **Apache Guacamole 完整版**（含 Java web 层） | RDP/VNC/SSH/Telnet | 旧设计 D1 已否决：Java 层与 Cockpit 的 Go 后端职责重叠（认证/会话/审计都在 Go 侧）。只取 guacd + common-js。 |
    54	| **Myrtille / SparkView** | RDP 为主 | 商业许可或活跃度不足，协议覆盖不全。 |
    55	
    56	## 协议与数据流（SSH 新增段）
    57	
    58	```
    59	浏览器                    Go 网关                         guacd                  目标
    60	   │  WSS /api/remote/guacamole?ticket=…                     │                    │
    61	   │  (Sec-WebSocket-Protocol 兜底)                          │                    │
    62	   │ ────────────────────────────────────────────────────────>│  TCP select "ssh"  │
    63	   │                          参数名列表响应 <──────────────── │                    │
    64	   │                          size 1280x800 + connect ──────> │ ── libssh2 ──────> │ :22
    65	   │                          (username/password/private-key) │    认证 + PTY      │
    66	   │ <────────────── 指令流（终端图块/剪贴板/sync）─────────── │ <───────────────── │
    67	   │ ─────────────── 键盘/鼠标/剪贴板指令（key/mouse/clip）──> │ ────────────────> │
    68	```
    69	
    70	- **浏览器 ↔ server 段**：WebSocket 文本帧承载 Guacamole 指令流，与
    71	  RDP/VNC 完全同一管道；网关不解析指令内容（握手指令构造除外）。
    72	- **server ↔ guacd 段**：`select ssh` → 参数列表 → `size` + `connect`。
    73	  `size` 的像素宽高由 guacd 的 ssh 插件按默认字体度量换算成终端列/行
    74	  （connect 参数不传 width/height——官方 ssh 参数集无此项）。
    75	- **guacd ↔ 目标段**：libssh2 拨 `hostname:port`，认证用 connect 指令
    76	  携带的凭据；终端输出编码为 Guacamole 指令流（图块）推回。
    77	
    78	### connect 参数映射（D2）
    79	
    80	| ticket 参数 | guacd connect 参数 | 说明 |
    81	| --- | --- | --- |
    82	| `username` | `username` | 直传（上游已统一处理） |
    83	| `password` | `password` | 直传；口令认证 |
    84	| `private_key` | `private-key` | **PEM 原文直传**（不 base64）；私钥认证，优先于口令 |
    85	| `domain` | 不传 | SSH 无域概念（RDP 专用） |
    86	| `width`/`height` | 不传 | 字符终端；尺寸经隧道层 `size` 指令（guacd 自行换算列/行） |
    87	
    88	## 与现有远控链路衔接
    89	
    90	全部复用，零新增基础设施：
    91	
    92	1. **票据**：`POST /api/remote/tickets` 的 `private_key` 字段早已存在
    93	   （agent 通道 SSH 私钥用），Guacamole 路线直接复用；票据仍为 5 分钟
    94	   一次性、内存态，凭据不进 URL/日志/审计。
    95	2. **出口策略**：`matchRemoteEgress`（allow-list / CIDR / per-agent），
    96	   agent_id 作为策略锚点与审计归属——与 RDP/VNC 语义一致。guacd 只拨本机回环中继
    97	   （`internal/server/guac_relay.go`），真实目标由 agent 从自己网络位拨出，
    98	   agent 在数据路径上（2026-10-02 落地，修正旧设计 D4「guacd 直拨」的假设）。
    99	3. **审计**：`auditRemoteStart/End` 全协议同款；SSH 经 guacd 的会话同样
   100	   记 `Protocol: ssh`，凭据（口令/私钥）绝进审计详情。
   101	4. **录制**：guacd 的 ssh 插件同样支持 `recording-path/recording-name`
   102	   → `.guac` 收集链路（`collectGuacRecording`，Format=guac）原样生效，
   103	   SSH 会话录制与 RDP/VNC 同一 `/recordings` 页回放。
   104	5. **权限**：`terminal:write`（RBAC `terminal` 资源域，`/api/remote/*` 统一归并，见 `internal/server/rbac.go`），入口按钮的
   105	   PermGuard 不因通道切换而变。
   106	
   107	## 决策
   108	
   109	- **D1 协议白名单扩 `ssh`**：`handleGuacamoleWebSocket` 放行 rdp/vnc/ssh；
   110	  **telnet 不进**（方向只提三协议；telnet 无加密，维持 agent 通道现状）。
   111	- **D2 connect 参数映射**：见上表；私钥 PEM 原文直传（真机验收修正
   112	  2026-09-30：settings.h 注释 "encoded as base64" 与实现矛盾——
   113	  guac_user_parse_args_string → key_alloc 零解码，base64 文本进 libssh2
   114	  PEM_read 必失败；A/B 实测原文两格式认证全过、base64 全败）。
   115	- **D3 前端入口切换**：web 默认 SSH 入口从 TerminalModal 切到
   118	
   119	### Remote Access and Proxy
   120	
   121	Remote terminal and desktop connections (Guacamole tunnels) use short-lived tickets:
   122	
   123	1. The browser first carries its JWT in a `POST /api/remote/tickets` request.
   124	2. The Server generates a 5-minute-valid, single-use ticket.
   125	3. The browser connects to `/api/remote/terminal` or `/api/remote/guacamole`, placing the ticket in `Sec-WebSocket-Protocol`.
   126	4. After validating the ticket, the Server converts the connection request into a `proxy_*` message to the corresponding Agent (terminal connects directly to the target; desktop is relayed via `guac:` to guacd).
   127	5. The Agent connects to the target service and forwards data back to the browser through the Server.
   128	
   129	This boundary ensures the browser needs neither to know the intranet topology nor to connect directly to Agents.
   130	Remote targets by default only permit hostnames, IPs, or CIDRs explicitly configured in `remote_control.allowed_targets`.
   131	
   132	## Protocol Boundaries
   133	
   134	Server–Agent messages are unified as:
   135	
   165	- `target`: the `host:port` the Agent should connect to.
   166	- `data`: data content; when JSON-encoded, possibly a base64 string or byte array.
   167	
   168	## Remote Connection Ticket
   169	
   170	The browser does not put the JWT directly into the remote WebSocket URL. The flow is:
   171	
   172	1. The browser calls `POST /api/remote/tickets` with the JWT.
   173	2. The Server generates a 5-minute, single-use ticket.
   174	3. The browser opens a WebSocket, putting the ticket as the first `Sec-WebSocket-Protocol` value.
   175	4. The Server validates and consumes the ticket, then starts forwarding.
   176	
   177	Creating a ticket:
   178	
   179	```http
   180	POST /api/remote/tickets
   181	Authorization: Bearer <jwt>
   182	Content-Type: application/json
   183	```
   184	
   185	```json
已经确认两项高风险边界：票据 ID 被写入日志；未配置 secret 的 Agent 注册路径允许未认证注册。另发现远程 guacd 部署文档与中继绑定地址冲突，以及 Job/远控文档存在仍未消除的实现口径陈旧。接下来核对相关调用与测试边界，避免把设计取舍误报成缺陷。
[0m$ [0mnl -ba docs/guacd-deploy.md | sed -n '58,70p'; nl -ba deployments/guacd/docker-compose.yml | sed -n '15,22p'; nl -ba internal/server/guac_relay.go | sed -n '61,81p'; nl -ba docs/guide/architecture.md | sed -n '119,127p'; nl -ba docs/guide/protocol.md | sed -n '167,175p'; nl -ba docs/guide/jobs-design.md | sed -n '127,139p'; nl -ba internal/server/api_jobs.go | sed -n '18,27p;63,101p'
58	### Cockpit Server 端
    59	
    60	Cockpit Server 通过 `GUACD_ADDR` 环境变量连接 guacd：
    61	
    62	```bash
    63	# 默认值（同机部署）
    64	GUACD_ADDR=127.0.0.1:4822
    65	
    66	# 跨机部署时指向 guacd 主机内网地址
    67	GUACD_ADDR=192.168.5.10:4822
    68	```
    69	
    70	在 `/etc/cockpit/config.yaml` 同机部署不需要额外配置。
    15	    restart: unless-stopped
    16	    ports:
    17	      # Go 网关同机反代：绑回环，不对外暴露（guacd 不是 HTTP 服务，
    18	      # 也不是浏览器可达的端点——浏览器走 /api/remote/guacamole WS）
    19	      # 跨机部署时改绑内网地址（如 "192.168.x.x:4822:4822"）并靠防火墙收口
    20	      - "${GUACD_BIND:-127.0.0.1}:${GUACD_PORT:-4822}:4822"
    21	    volumes:
    22	      # guacd session recording 落盘目录（connect 指令的 recording-path 指向这里，
    61	// startGuacRelay 起中继 listener。返回 relay 与「guacd 应拨的地址」
    62	// （127.0.0.1:<ephemeral>，进 connect 指令的 hostname/port）。
    63	func startGuacRelay(agentID, proxyID, target string, send func(string, *protocol.Message) error) (*guacRelay, string, error) {
    64		ln, err := net.Listen("tcp", "127.0.0.1:0")
    65		if err != nil {
    66			return nil, "", fmt.Errorf("listen relay: %w", err)
    67		}
    68		r := &guacRelay{
    69			agentID: agentID,
    70			proxyID: proxyID,
    71			target:  target,
    72			send:    send,
    73			ln:      ln,
    74			conns:   make(map[string]net.Conn),
    75		}
    76		guacRelayRegMu.Lock()
    77		guacRelayReg[proxyID] = r
    78		guacRelayRegMu.Unlock()
    79		go r.acceptLoop()
    80		return r, ln.Addr().String(), nil
    81	}
   119	### 远程访问与代理
   120	
   121	远程终端与桌面连接（Guacamole 隧道）使用短期 ticket：
   122	
   123	1. 浏览器先携带 JWT 请求 `POST /api/remote/tickets`。
   124	2. Server 生成 5 分钟有效、单次使用 ticket。
   125	3. 浏览器连接 `/api/remote/terminal` 或 `/api/remote/guacamole`，把 ticket 放在 `Sec-WebSocket-Protocol`。
   126	4. Server 校验 ticket 后，把连接请求转成 `proxy_*` 消息发给对应 Agent（终端直连目标；桌面经 `guac:` 中继到 guacd）。
   127	5. Agent 连接目标服务并把数据通过 Server 转发回浏览器。
   167	## 远程连接 ticket
   168	
   169	Browser 不直接把 JWT 放进远程 WebSocket URL。流程是：
   170	
   171	1. Browser 携带 JWT 调 `POST /api/remote/tickets`。
   172	2. Server 生成 5 分钟有效、单次使用 ticket。
   173	3. Browser 建立 WebSocket，把 ticket 作为 `Sec-WebSocket-Protocol` 的第一个值。
   174	4. Server 校验并消费 ticket，然后开始转发。
   175	
   127	## 边界（明确不做）
   128	
   129	- **不做异步执行**（**已由 P2 兑现，见顶部「已由 P2 落地」注记**）：创建即同步
   130	  等待终态。异步化（排队/取消/进度）等 Workflow 立项时一并设计，届时
   131	  cancelled/timeout 独立状态、`GET /jobs?status=running` 过滤、SSE 推送才有
   132	  真实的消费者（落地口径：`cancelled` 终态 + 台账过滤已实现；SSE 仍拍不做）；
   133	- **不做 Workflow/Action**：Workflow 是「一组 Job 的编排」（依赖/并行/重试），
   134	  Action 是「带语义的幂等操作」，都在 Job 之上，本期只有 Job 本体；
   135	- **不做定时触发**：Job 与 cron 是两个平面——cron 是 agent 侧调度面（crontab
   136	  即状态），Job 是控制面执行台账；「定时跑 Job」= cron 写一条调用 `POST
   137	  /api/jobs` 的命令行，不引入新耦合；
   138	- **不做输出实时流**：终态一次性回传。流式输出（WebSocket attach）留待
   139	  远控/终端域统一设计。
    18	// 执行 Job API（设计见 docs/guide/jobs-design.md；异步化见 workflow-design.md W1）。
    19	//
    20	//	POST /api/jobs             创建 Job，后台执行，立即 201 返回 pending 视图
    21	//	GET  /api/jobs             最近 Job 列表（倒序，limit 50；支持过滤）
    22	//	GET  /api/jobs/{id}        单条详情（含输出）
    23	//	POST /api/jobs/{id}/cancel 取消 pending/running Job（W3 + M2c C6）
    24	//
    25	// 执行流：校验 → 建 Job(pending) → 后台 goroutine CallAgent(job.exec) →
    26	// 终态 success/failed 回写 + 审计 job_run。目标 agent 离线一律 503（先查
    27	// registry，与 cron 同口径）。
    63	// submitJob 校验→建行(pending)→后台派发（handleJobCreate 与审批门批准后
    64	// 共用的收口，M2d）：目标必须在线（离线 503 不建幽灵记录）。
    65	func (s *Server) submitJob(p *jobCreatePayload, actor string) (*jobView, *submitFail) {
    66		if err := validateJobPayload(p); err != nil {
    67			return nil, &submitFail{http.StatusBadRequest, err.Error()}
    68		}
    69		// targets 扇出仅 workflow steps 合法（M2b F1）：standalone Job 单目标语义
    70		if len(p.Targets) > 0 {
    71			return nil, &submitFail{http.StatusBadRequest, "targets is only valid in workflow steps"}
    72		}
    73		// depends_on 依赖门控仅 workflow steps 合法（M2e G1）：standalone 无编排语义
    74		if len(p.DependsOn) > 0 {
    75			return nil, &submitFail{http.StatusBadRequest, "depends_on is only valid in workflow steps"}
    76		}
    77		if _, ok := s.registry.Get(p.Target); !ok {
    78			return nil, &submitFail{http.StatusServiceUnavailable, "agent offline"}
    79		}
    80	
    81		paramsJSON, _ := json.Marshal(p.Parameters)
    82		job := &storage.Job{
    83			ID:         protocol.GenerateID(),
    84			Type:       p.Type,
    85			Target:     p.Target,
    86			Actor:      actor,
    87			Parameters: string(paramsJSON),
    88			Status:     storage.JobStatusPending,
    89			CreatedAt:  time.Now(),
    90		}
    91		if err := s.db.CreateJob(job); err != nil {
    92			return nil, &submitFail{http.StatusInternalServerError, "failed to create job"}
    93		}
    94	
    95		// 视图在派发前取定：goroutine 会原地改 job 状态（pending→running→终态），
    96		// 先快照保证 201 响应体恒为 pending（W1 契约），并与后台写互不竞争
    97		view := toJobView(job)
    98	
    99		// 后台派发：终态回写 + 审计在 goroutine 内完成（HTTP 请求即时返回）
   100		s.goBg(func() { s.dispatchJobAsync(job, *p, actor) })
   101		return &view, nil
[0m$ [0mnl -ba internal/auth/handler.go | sed -n '47,112p'; nl -ba internal/server/server.go | sed -n '329,365p'; rg -n "RateLimit|rate.?limit|rate limiter|login.*limit|failed.*attempt" internal --glob '*.go' --glob '!**/*_test.go' || true; nl -ba internal/server/api_remote.go | sed -n '540,700p'
47	// HandleLogin 处理登录
    48	func HandleLogin(w http.ResponseWriter, r *http.Request) {
    49		ensureDefaults()
    50		defaultService.HandleLogin(w, r)
    51	}
    52	
    53	// HandleLogin 处理登录
    54	func (s *Service) HandleLogin(w http.ResponseWriter, r *http.Request) {
    55		if r.Method != http.MethodPost {
    56			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
    57			return
    58		}
    59	
    60		var req LoginRequest
    61		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    62			http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
    63			return
    64		}
    65	
    66		// 验证用户名密码
    67		db := s.DB()
    68		if db == nil {
    69			http.Error(w, `{"error":"Authentication service unavailable"}`, http.StatusInternalServerError)
    70			return
    71		}
    72		user, err := db.VerifyPassword(req.Username, req.Password)
    73		if err != nil {
    74			http.Error(w, `{"error":"Invalid username or password"}`, http.StatusUnauthorized)
    75			return
    76		}
    77	
    78		// 检查是否启用了 TOTP
    79		if user.TOTPEnabled {
    80			tmpToken := generateTmpToken(user.ID)
    81			response := LoginResponse{
    82				UserID:       user.ID,
    83				Username:     user.Username,
    84				RequiresTOTP: true,
    85				TmpToken:     tmpToken,
    86			}
    87			w.Header().Set("Content-Type", "application/json")
    88			json.NewEncoder(w).Encode(response)
    89			return
    90		}
    91	
    92		// 生成 token（signLoginToken 注入点：HS256 + []byte 密钥下
    93		// SignedString 不会失败，错误分支仅供测试覆盖）
    94		token, err := signLoginToken(s, user.ID, user.Username, user.Role)
    95		if err != nil {
    96			http.Error(w, `{"error":"Failed to generate token"}`, http.StatusInternalServerError)
    97			return
    98		}
    99	
   100		// 返回 token
   101		response := LoginResponse{
   102			Token:     token,
   103			ExpiresAt: 0,
   104			UserID:    user.ID,
   105			Username:  user.Username,
   106			Role:      user.Role,
   107		}
   108	
   109		w.Header().Set("Content-Type", "application/json")
   110		json.NewEncoder(w).Encode(response)
   111	}
   112	
   329		// 公开路由
   330		mux.HandleFunc("/ws", s.handleWebSocket)
   331		mux.HandleFunc("/health", s.handleHealth)
   332		mux.HandleFunc("/api/auth/login", s.handleLoginWithAudit)
   333		mux.HandleFunc("/api/auth/refresh", s.authService().HandleRefresh)
   334		mux.HandleFunc("/api/auth/totp/verify", s.handleTOTPVerify) // TOTP 验证不需要 JWT（使用临时令牌）
   335		mux.HandleFunc("/api/auth/forgot-password", s.handleForgotPassword)
   336		mux.HandleFunc("/api/auth/reset-password", s.handleResetPassword)
   337		mux.HandleFunc("/api/auth/verify-reset-code", s.handleVerifyResetCode)
   338	
   339		// 需要认证的 API 路由
   340		mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
   341			// 登录相关接口不需要认证；TOTP 设置路由需认证（曾因置于
   342			// /api/auth/ 前缀守卫 return 之后而不可达，现并入同一 switch）
   343			if strings.HasPrefix(r.URL.Path, "/api/auth/") {
   344				switch r.URL.Path {
   345				case "/api/auth/login":
   346					s.handleLoginWithAudit(w, r)
   347				case "/api/auth/refresh":
   348					s.authService().HandleRefresh(w, r)
   349				case "/api/auth/totp/verify":
   350					s.handleTOTPVerify(w, r)
   351				case "/api/auth/totp/generate":
   352					s.authService().Middleware(s.handleTOTPGenerate)(w, r)
   353				case "/api/auth/totp/enable":
   354					s.authService().Middleware(s.handleTOTPEnable)(w, r)
   355				case "/api/auth/totp/disable":
   356					s.authService().Middleware(s.handleTOTPDisable)(w, r)
   357				}
   358				return
   359			}
   360			// 其他 API 需要认证
   361			s.authService().Middleware(s.serveAPI)(w, r)
   362		})
   363	
   364		// Web UI (SPA) - 必须放在最后作为 fallback
   365		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
internal/vault/persona.go:289:	case status == http.StatusTooManyRequests || code == "rate_limited":
internal/vault/persona.go:290:		return fmt.Errorf("vault: persona: rate limited (120 req/min per token)")
internal/github/client.go:430:// GetRateLimit gets rate limit status
internal/github/client.go:431:func (c *Client) GetRateLimit(ctx context.Context) (*RateLimit, error) {
internal/github/client.go:432:	path := "/rate_limit"
internal/github/client.go:436:		return nil, fmt.Errorf("get rate limit: %w", err)
internal/github/client.go:441:			Core    RateLimit `json:"core"`
internal/github/client.go:442:			GraphQL RateLimit `json:"graphql"`
internal/github/client.go:443:			Search  RateLimit `json:"search"`
internal/github/client.go:453:// RateLimit rate limit information
internal/github/client.go:454:type RateLimit struct {
   540		if req.Domain != "" {
   541			params["domain"] = req.Domain
   542		}
   543		if req.Width > 0 {
   544			params["width"] = strconv.Itoa(req.Width)
   545		}
   546		if req.Height > 0 {
   547			params["height"] = strconv.Itoa(req.Height)
   548		}
   549	
   550		// 生成票据
   551		ticket, err := s.ticketMgr.GenerateTicket(userInfo.UserID, userInfo.Username, params)
   552		if err != nil {
   553			s.auditRemoteFailure(
   554				userInfo.UserID,
   555				userInfo.Username,
   556				clientIP,
   557				userAgent,
   558				&audit.RemoteSessionDetails{
   559					Protocol: req.Protocol,
   560					AgentID:  req.AgentID,
   561					Host:     req.Host,
   562					Port:     req.Port,
   563					Egress:   egressMatch.summary(),
   564					Reason:   "failed to generate ticket",
   565				},
   566			)
   567			http.Error(w, "Failed to generate ticket", http.StatusInternalServerError)
   568			return
   569		}
   570	
   571		// 远控会话开始审计（不写 password）
   572		s.auditRemoteStart(
   573			audit.ActionRemoteStart,
   574			userInfo.UserID,
   575			userInfo.Username,
   576			clientIP,
   577			userAgent,
   578			&audit.RemoteSessionDetails{
   579				Protocol:   req.Protocol,
   580				AgentID:    req.AgentID,
   581				Host:       req.Host,
   582				Port:       req.Port,
   583				Session:    ticket.ID,
   584				Egress:     egressMatch.summary(),
   585				AuthSource: authSource,
   586			},
   587		)
   588	
   589		w.Header().Set("Content-Type", "application/json")
   590		json.NewEncoder(w).Encode(map[string]string{
   591			"ticket":     ticket.ID,
   592			"expires_at": ticket.ExpiresAt.Format(time.RFC3339),
   593		})
   594	}
   595	
   596	func (s *Server) handleRemoteSessions(w http.ResponseWriter, r *http.Request) {
   597		switch r.Method {
   598		case http.MethodGet:
   599			s.writeJSON(w, http.StatusOK, map[string]interface{}{
   600				"data": s.remoteSessions.List(),
   601			})
   602		case http.MethodPost:
   603			s.handleRemoteSessionCreate(w, r)
   604		default:
   605			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
   606		}
   607	}
   608	
   609	func (s *Server) handleRemoteSession(w http.ResponseWriter, r *http.Request) {
   610		id := strings.TrimPrefix(r.URL.Path, "/api/remote/sessions/")
   611		id = strings.TrimSpace(id)
   612		if id == "" {
   613			http.Error(w, "Missing session id", http.StatusBadRequest)
   614			return
   615		}
   616	
   617		switch r.Method {
   618		case http.MethodGet:
   619			session, ok := s.remoteSessions.Get(id)
   620			if !ok {
   621				http.Error(w, "Remote session not found", http.StatusNotFound)
   622				return
   623			}
   624			s.writeJSON(w, http.StatusOK, session)
   625		case http.MethodDelete:
   626			// 活会话优先真正终止（断开数据面，状态落 closed）；仅台账记录才
   627			// 走软删（D-2026-10-12-4：DELETE 语义从「删登记」收口为「先终止后删登记」）
   628			if s.terminateLiveSession(id) {
   629				w.WriteHeader(http.StatusNoContent)
   630				return
   631			}
   632			if ok := s.remoteSessions.Delete(id); !ok {
   633				http.Error(w, "Remote session not found", http.StatusNotFound)
   634				return
   635			}
   636			w.WriteHeader(http.StatusNoContent)
   637		default:
   638			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
   639		}
   640	}
   641	
   642	func (s *Server) handleRemoteSessionCreate(w http.ResponseWriter, r *http.Request) {
   643		var req RemoteSessionRequest
   644		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
   645			http.Error(w, "Invalid request body", http.StatusBadRequest)
   646			return
   647		}
   648	
   649		if req.AgentID == "" || req.Host == "" || req.Port <= 0 {
   650			http.Error(w, "Missing required fields", http.StatusBadRequest)
   651			return
   652		}
   653	
   654		if req.Protocol != protocol.RemoteProtocolSSH &&
   655			req.Protocol != protocol.RemoteProtocolTelnet &&
   656			req.Protocol != protocol.RemoteProtocolRDP &&
   657			req.Protocol != protocol.RemoteProtocolVNC {
   658			http.Error(w, "Unsupported protocol", http.StatusBadRequest)
   659			return
   660		}
   661	
   662		if _, exists := s.registry.Get(req.AgentID); !exists {
   663			http.Error(w, "Agent not found or offline", http.StatusNotFound)
   664			return
   665		}
   666	
   667		userInfo, _ := auth.GetUserFromContext(r)
   668		session := s.remoteSessions.Create(userInfo.UserID, userInfo.Username, req)
   669		s.writeJSON(w, http.StatusCreated, session)
   670	}
   671	
   672	// terminateLiveSession 终止一个活会话（terminal 或 guacamole），返回是否命中。
   673	// 关闭动作本身收口在两条链路各自的 close 函数（幂等 + 审计 + 台账状态），
   674	// 这里只做定位——按会话 ID 在两个数据面索引里查。
   675	func (s *Server) terminateLiveSession(id string) bool {
   676		terminalSessionsMu.Lock()
   677		ts, ok := terminalSessions[id]
   678		terminalSessionsMu.Unlock()
   679		if ok {
   680			s.closeTerminalSession(ts)
   681			return true
   682		}
   683		guacSessionsMu.Lock()
   684		gs, ok := guacSessions[id]
   685		guacSessionsMu.Unlock()
   686		if ok {
   687			s.closeGuacamoleSession(gs)
   688			return true
   689		}
   690		return false
   691	}
[0m$ [0mrg -n "Email: *user\.Email|MaskedEmail|GenerateResetToken|rate|attempt" internal/server/password_reset_handlers.go internal/auth/password_reset.go; nl -ba internal/server/password_reset_handlers.go | sed -n '24,71p'; nl -ba internal/server/ticket.go | sed -n '41,66p'; nl -ba internal/server/websocket.go | sed -n '167,196p'; nl -ba internal/server/middleware.go | sed -n '43,68p'
internal/auth/password_reset.go:34:// GenerateResetToken 会起后台清理协程，且 HTTP handler 并发调用，
internal/auth/password_reset.go:53:func generateVerificationCode() string {
internal/auth/password_reset.go:59:// GenerateResetToken 生成密码重置令牌和验证码
internal/auth/password_reset.go:60:func GenerateResetToken(userID, email string) (string, string, error) {
internal/auth/password_reset.go:69:	code := generateVerificationCode()
internal/auth/password_reset.go:307:	MaskedEmail string `json:"masked_email"` // 脱敏邮箱
internal/server/password_reset_handlers.go:50:	token, code, err := authGenerateResetToken(user.ID, user.Email)
internal/server/password_reset_handlers.go:52:		http.Error(w, `{"error":"Failed to generate reset token"}`, http.StatusInternalServerError)
internal/server/password_reset_handlers.go:67:		Email:       user.Email,
internal/server/password_reset_handlers.go:68:		MaskedEmail: auth.MaskEmail(user.Email),
    24		if req.Username == "" {
    25			http.Error(w, `{"error":"Username is required"}`, http.StatusBadRequest)
    26			return
    27		}
    28	
    29		// 查找用户
    30		user, err := s.db.GetUserByUsername(req.Username)
    31		if err != nil {
    32			// 为了安全，即使用户不存在也返回成功消息
    33			w.Header().Set("Content-Type", "application/json")
    34			json.NewEncoder(w).Encode(map[string]string{
    35				"message": "如果该用户存在，重置邮件已发送",
    36			})
    37			return
    38		}
    39	
    40		// 检查用户是否有邮箱
    41		if user.Email == "" {
    42			w.Header().Set("Content-Type", "application/json")
    43			json.NewEncoder(w).Encode(map[string]string{
    44				"message": "该账户未绑定邮箱，请联系管理员重置密码",
    45			})
    46			return
    47		}
    48	
    49		// 生成重置令牌和验证码
    50		token, code, err := authGenerateResetToken(user.ID, user.Email)
    51		if err != nil {
    52			http.Error(w, `{"error":"Failed to generate reset token"}`, http.StatusInternalServerError)
    53			return
    54		}
    55	
    56		// 发送邮件（异步，不阻塞响应）
    57		go func() {
    58			if err := auth.SendPasswordResetEmail(user.Email, user.Username, code, token); err != nil {
    59				// 记录错误但不暴露给用户
    60				printf("Failed to send reset email: %v", err)
    61			}
    62		}()
    63	
    64		// 返回脱敏邮箱
    65		w.Header().Set("Content-Type", "application/json")
    66		json.NewEncoder(w).Encode(auth.ForgotPasswordResponse{
    67			Email:       user.Email,
    68			MaskedEmail: auth.MaskEmail(user.Email),
    69			Message:     "重置邮件已发送到您的邮箱",
    70		})
    71	}
    41	// GenerateTicket 生成新票据（有效期5分钟）
    42	func (tm *TicketManager) GenerateTicket(userID, username string, params map[string]string) (*Ticket, error) {
    43		tm.mu.Lock()
    44		defer tm.mu.Unlock()
    45	
    46		// 生成随机票据ID（16字节）
    47		b := make([]byte, 16)
    48		if _, err := randRead(b); err != nil {
    49			return nil, err
    50		}
    51		ticketID := hex.EncodeToString(b)
    52	
    53		ticket := &Ticket{
    54			ID:        ticketID,
    55			UserID:    userID,
    56			Username:  username,
    57			Params:    params,
    58			ExpiresAt: time.Now().Add(5 * time.Minute),
    59			Consumed:  false,
    60		}
    61	
    62		tm.tickets[ticketID] = ticket
    63		log.Printf("Ticket generated: %s for user %s", ticketID, username)
    64	
    65		return ticket, nil
    66	}
   167	func (s *Server) authenticateAgentRegistration(reg *protocol.RegisterPayload) (*storage.Agent, *registrationRejection, error) {
   168		existingAgent, err := s.db.GetAgent(reg.AgentID)
   169		if err != nil {
   170			if errors.Is(err, storage.ErrNotFound) {
   171				return nil, nil, nil
   172			}
   173			return nil, nil, fmt.Errorf("load existing agent %s: %w", reg.AgentID, err)
   174		}
   175	
   176		if existingAgent.SecretHash == "" {
   177			// 兼容历史数据：已有 Agent 但未配置密钥时仍允许注册。
   178			log.Printf("Agent %s exists but has no secret configured", reg.AgentID)
   179			return existingAgent, nil, nil
   180		}
   181	
   182		if reg.Secret == "" {
   183			return existingAgent, &registrationRejection{
   184				code:    "authentication_required",
   185				message: "Agent secret is required",
   186			}, nil
   187		}
   188		if !storage.VerifyAgentSecret(existingAgent.SecretHash, reg.Secret) {
   189			return existingAgent, &registrationRejection{
   190				code:    "authentication_failed",
   191				message: "Invalid agent secret",
   192			}, nil
   193		}
   194	
   195		return existingAgent, nil, nil
   196	}
    43	// CORSMiddleware 统一处理 API 跨域响应和预检请求。
    44	func (s *Server) CORSMiddleware(next http.Handler) http.Handler {
    45		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    46			if !strings.HasPrefix(r.URL.Path, "/api/") {
    47				next.ServeHTTP(w, r)
    48				return
    49			}
    50	
    51			setCORSHeaders(w)
    52			if r.Method == http.MethodOptions {
    53				w.WriteHeader(http.StatusOK)
    54				return
    55			}
    56	
    57			next.ServeHTTP(w, r)
    58		})
    59	}
    60	
    61	func setCORSHeaders(w http.ResponseWriter) {
    62		allowed := os.Getenv("ALLOWED_ORIGINS")
    63		if allowed != "" {
    64			w.Header().Set("Access-Control-Allow-Origin", allowed)
    65		}
    66		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
    67		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
    68	}
[0m$ [0mgo test ./...
ok  	github.com/cuihairu/cockpit/cmd/cockpit	18.210s
ok  	github.com/cuihairu/cockpit/cmd/cockpit-agent	13.346s
ok  	github.com/cuihairu/cockpit/cmd/cockpit-probe-agent	0.064s
ok  	github.com/cuihairu/cockpit/core/backoff	0.044s
?   	github.com/cuihairu/cockpit/core/crash	[no test files]
ok  	github.com/cuihairu/cockpit/core/execlog	0.525s
ok  	github.com/cuihairu/cockpit/core/healthprobe	0.019s
ok  	github.com/cuihairu/cockpit/core/heartbeat	0.060s
ok  	github.com/cuihairu/cockpit/core/logx	0.019s
ok  	github.com/cuihairu/cockpit/core/metrics	7.197s
ok  	github.com/cuihairu/cockpit/core/platform	0.023s
ok  	github.com/cuihairu/cockpit/core/platform/linux	0.018s
ok  	github.com/cuihairu/cockpit/core/register	0.017s
ok  	github.com/cuihairu/cockpit/core/report	0.623s


Command exceeded timeout of 120000 ms. Retry with a larger timeout if the command is expected to take longer.
[0m$ [0mnl -ba internal/server/orphan_run.go | sed -n '1,180p'; nl -ba deployments/docker/docker-compose.yml | sed -n '21,55p'; nl -ba docs/guide/rbac-design.md | sed -n '133,141p'; nl -ba docs/guide/jobs-design.md | sed -n '127,139p'; nl -ba internal/server/server.go | sed -n '260,270p'; nl -ba internal/server/middleware.go | sed -n '61,69p'
1	package server
     2	
     3	import (
     4		"log"
     5		"time"
     6	
     7		"github.com/cuihairu/cockpit/internal/storage"
     8	)
     9	
    10	// server 重启孤儿清扫（审批门前置）：Job 派发（dispatchJobAsync）与 workflow
    11	// 推进（advanceWorkflow）都是进程内 goroutine，重启即丢——遗留的 pending/
    12	// running Job 行与 running WorkflowRun 行不再有推进者，继续等待只会永远悬挂。
    13	// 启动期（HTTP 未开始接流）一次性收敛为 failed 终态，与 backup 的
    14	// recoverOrphanBackupRuns 同款口径：逐行条件 UPDATE，绝不改写已落终态的行。
    15	
    16	// orphanErrMsg 孤儿记录落账错误摘要（进 Job.error；run 无错误列只落状态）。
    17	const orphanErrMsg = "server restarted during execution"
    18	
    19	// recoverOrphanRuns 把重启遗留的非终态 Job（pending/running）与 running
    20	// WorkflowRun 收敛为 failed。Job 面与 run 面相互独立：一面查询失败只记日志
    21	// 继续，另一面照常清扫（一面不可用不该拖住另一面的孤儿收敛）。
    22	func (s *Server) recoverOrphanRuns() {
    23		swept := 0
    24		// 先 running 后 pending：pending 行含「建行后派发 goroutine 未及翻转」
    25		// 的窗口态，重启后一律无推进者
    26		for _, status := range []string{storage.JobStatusRunning, storage.JobStatusPending} {
    27			jobs, err := s.db.ListJobsFiltered(status, "", "", "", 10000)
    28			if err != nil {
    29				log.Printf("recover orphan jobs (%s): %v", status, err)
    30				continue
    31			}
    32			for _, job := range jobs {
    33				if _, err := s.db.FailJob(job.ID, status, time.Now(), orphanErrMsg); err != nil {
    34					log.Printf("recover job %s: %v", job.ID, err)
    35					continue
    36				}
    37				swept++
    38			}
    39		}
    40	
    41		runs, err := s.db.RunningWorkflowRuns()
    42		if err != nil {
    43			log.Printf("recover orphan workflow runs: %v", err)
    44			return
    45		}
    46		for _, run := range runs {
    47			if _, err := s.db.FinishWorkflowRun(run.ID, storage.WorkflowRunStatusFailed, time.Now()); err != nil {
    48				log.Printf("recover workflow run %s: %v", run.ID, err)
    49				continue
    50			}
    51			swept++
    52		}
    53	
    54		if swept > 0 {
    55			log.Printf("Recovered %d orphan job/run records", swept)
    56		}
    57	}
    21	    ports:
    22	      - "${COCKPIT_HTTP_PORT:-9000}:9000"
    23	    environment:
    24	      ADMIN_USERNAME: "${ADMIN_USERNAME:-admin}"
    25	      ADMIN_PASSWORD: "${ADMIN_PASSWORD:?set ADMIN_PASSWORD in .env}"
    26	      JWT_SECRET: "${JWT_SECRET:?set JWT_SECRET in .env}"
    27	      TOTP_ENCRYPTION_KEY: "${TOTP_ENCRYPTION_KEY:-}"
    28	      PRODUCTION: "${PRODUCTION:-true}"
    29	      ALLOWED_ORIGINS: "${ALLOWED_ORIGINS:-}"
    30	      BASE_URL: "${BASE_URL:-}"
    31	      TZ: "${TZ:-Asia/Shanghai}"
    32	      # 远程桌面/终端网关（guacd）地址：启用 guacd profile 时设为 guacd:4822
    33	      # （同网络 DNS 名）；留空 = 不启用 Guacamole 远控入口
    34	      # 见 docs/remote-desktop-guacamole-design.md 与
    35	      # docs/remote-access-integration-design.md
    36	      GUACD_ADDR: "${GUACD_ADDR:-}"
    37	      # guacd 录制目录。必须与 guacd 容器里的挂载点**同一个卷同一个路径**：
    38	      # guacd 按 connect 指令的 recording-path 往这儿写 <session>.guac，
    39	      # Go 网关会话结束时从同一路径收走归档到 /data/recordings
    40	      # （collectGuacRecording）。两侧路径不一致 = 录制文件永远收不到。
    41	      GUACD_RECORDING_PATH: "${GUACD_RECORDING_PATH:-/var/lib/guacamole}"
    42	    volumes:
    43	      # 数据卷：SQLite 库 /data/cockpit.db、清单、备份、录制归档
    44	      - cockpit-data:/data
    45	      # guacd 录制卷（与下方 guacd 服务共用，见 GUACD_RECORDING_PATH 注释）
    46	      - guacd-recordings:/var/lib/guacamole
    47	    networks:
    48	      - cockpit-net
    49	
    50	  # guacd：Apache Guacamole 协议守护进程，把 RDP/VNC/SSH 翻译成 Guacamole
    51	  # 指令流（跳过 guacamole-web 的 Java 层）。无用户概念、无状态——会话/权限/
    52	  # 审计都在 Go 网关侧。
    53	  # 启用：docker compose --profile guacd up -d（并设 GUACD_ADDR=guacd:4822）
    54	  # 要点：端口不对外暴露（Go 网关同网络反代，浏览器走
    55	  # /api/remote/guacamole 的 WebSocket）；GUACD_LOG_LEVEL 别开 debug
   133	## 不做的事
   134	
   135	## 不做的事
   136	
   137	- 不引入 casban/ory 类权限框架——依赖重量与本项目体量不匹配
   138	- 不做行级权限（某个 agent 只对某部门可见）——现有部署是个人/小团队自托管，
   139	  行级隔离等真实需求出现再议
   140	- 不改 agent 协议——权限是 server 职责，agent 保持「被调用即执行」的信任模型
   127	## 边界（明确不做）
   128	
   129	- **不做异步执行**（**已由 P2 兑现，见顶部「已由 P2 落地」注记**）：创建即同步
   130	  等待终态。异步化（排队/取消/进度）等 Workflow 立项时一并设计，届时
   131	  cancelled/timeout 独立状态、`GET /jobs?status=running` 过滤、SSE 推送才有
   132	  真实的消费者（落地口径：`cancelled` 终态 + 台账过滤已实现；SSE 仍拍不做）；
   133	- **不做 Workflow/Action**：Workflow 是「一组 Job 的编排」（依赖/并行/重试），
   134	  Action 是「带语义的幂等操作」，都在 Job 之上，本期只有 Job 本体；
   135	- **不做定时触发**：Job 与 cron 是两个平面——cron 是 agent 侧调度面（crontab
   136	  即状态），Job 是控制面执行台账；「定时跑 Job」= cron 写一条调用 `POST
   137	  /api/jobs` 的命令行，不引入新耦合；
   138	- **不做输出实时流**：终态一次性回传。流式输出（WebSocket attach）留待
   139	  远控/终端域统一设计。
   260		s.registerRoutes(mux)
   261	
   262		// 预检请求必须先于认证处理，否则跨域 OPTIONS 会被 JWT 中间件拒绝。
   263		// RBAC 在 Audit 内层：403 也进审计链（rbac-design.md 笔 2）
   264		handler := s.CORSMiddleware(s.AuditMiddleware(s.RBACMiddleware(mux)))
   265	
   266		server := &http.Server{
   267			Addr:    s.addr,
   268			Handler: handler,
   269		}
   270	
    61	func setCORSHeaders(w http.ResponseWriter) {
    62		allowed := os.Getenv("ALLOWED_ORIGINS")
    63		if allowed != "" {
    64			w.Header().Set("Access-Control-Allow-Origin", allowed)
    65		}
    66		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
    67		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
    68	}
    69
[0m$ [0mnl -ba .github/workflows/test.yml | sed -n '1,100p'; nl -ba docs/guide/architecture.md | sed -n '100,104p'; nl -ba internal/server/api_stacks.go | sed -n '47,55p'; nl -ba internal/server/api_stack_webhook.go | sed -n '13,25p'
1	name: Test
     2	
     3	on:
     4	  push:
     5	    branches: [main]
     6	    paths-ignore:
     7	      - '**.md'
     8	      - 'docs/**'
     9	      - 'LICENSE'
    10	      - '.gitignore'
    11	      - '*.svg'
    12	      - '.editorconfig'
    13	  pull_request:
    14	    branches: [main]
    15	    paths-ignore:
    16	      - '**.md'
    17	      - 'docs/**'
    18	      - 'LICENSE'
    19	      - '.gitignore'
    20	      - '*.svg'
    21	      - '.editorconfig'
    22	  workflow_dispatch:
    23	
    24	permissions:
    25	  contents: read
    26	
    27	jobs:
    28	  test:
    29	    name: Test
    30	    runs-on: ubuntu-latest
    31	    env:
    32	      FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: true
    33	    steps:
    34	      - name: Checkout code
    35	        uses: actions/checkout@v7
    36	        with:
    37	          persist-credentials: false
    38	
    39	      - name: Set up Go
    40	        uses: actions/setup-go@v7
    41	        with:
    42	          go-version: '1.27'
    43	
    44	      - name: Download dependencies
    45	        run: go mod download
    46	
    47	      - name: Run go vet
    48	        run: go vet ./...
    49	
    50	      - name: Run tests with coverage
    51	        run: |
    52	          # server 包 -short 插桩峰值贴 600s（CI 慢机已两次撞线 panic，
    53	          # 本地负载下亦实测 600.7s——见 todo 2026-09-27 运维坑），对齐
    54	          # 下方 race 步骤放宽到 900s
    55	          go test -short ./... -coverprofile=coverage.out -covermode=atomic -timeout 900s
    56	
    57	      - name: Check coverage threshold
    58	        run: |
    59	          # 块级校验：未覆盖语句块必须登记 tool/known_uncoverable.txt 并附理由，
    60	          # 有效覆盖 100%。取代原「total 恒 100%」grep——探针 main() 场景编排、
    61	          # 真机命令默认闭包、死防御等无法进程内确定性覆盖，属结构性边界须登记豁免
    62	          bash tool/coverage_check.sh coverage.out
    63	
    64	      - name: Run tests with race detector
    65	        run: |
    66	          # race 插桩比覆盖步更慢：server 包本地实测 1004s（2026-10-13，
    67	          # P2 ② stack webhook/follow 用例入账后），900s 已不够，放宽到 1500s
    68	          go test -race -short ./... -timeout 1500s
    69	
    70	      - name: Upload coverage to Codecov
    71	        uses: codecov/codecov-action@v7
    72	        with:
    73	          files: ./coverage.out
    74	          token: ${{ secrets.CODECOV_TOKEN }}
    75	          fail_ci_if_error: false
    76	
    77	      - name: Set up pnpm
    78	        uses: pnpm/action-setup@v6
    79	        with:
    80	          version: 12
    81	
    82	      - name: Set up Node
    83	        uses: actions/setup-node@v7
    84	        with:
    85	          node-version: '24'
    86	          cache: 'pnpm'
    87	          cache-dependency-path: web/pnpm-lock.yaml
    88	
    89	      - name: Run web unit tests with coverage gate
    90	        run: |
    91	          cd web
    92	          pnpm install --frozen-lockfile
    93	          bash tool/coverage_check.sh
   100	### HTTP API 与 Web UI
   101	
   102	Web UI 默认请求 `/api`。公开接口包括登录、Token 刷新、TOTP 验证、密码重置和 `/health`。其他 `/api/` 资源接口通过 JWT 中间件保护。
   103	
   104	主要 API 面：
    47	func (s *Server) registerStacksAPI(mux *http.ServeMux) {
    48		mux.HandleFunc(stacksAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
    49			s.authService().Middleware(s.handleStacks)(w, r)
    50		})
    51		// webhook：forge 回调无登录态，精确路由独立注册（ServeMux 最长匹配
    52		// 胜出，先于 /api/stacks/ 认证前缀），token 即凭据（P2 ②）
    53		mux.HandleFunc("POST /api/stacks/webhook", s.handleStackWebhook)
    54	}
    55	
    13	// stacks webhook 触发部署（stack-deploy-design.md P2 ②）：
    14	//
    15	//	POST /api/stacks/webhook   头 X-Cockpit-Token == 配置 stacks.webhook_token
    16	//
    17	// git 仓库推送后由 forge 回调，无法走登录态——独立精确路由（先于
    18	// /api/stacks/ 认证前缀注册，ServeMux 最长匹配胜出）。令牌空 = 端点关闭
    19	// （默认关，显式配置开启，同 notification 红线口径）。webhook 只触发
    20	// 「拉取已接入 origin + 部署改动 stack」，不接受 remote 参数（不重定向 origin）。
    21	// body 可选 {"agentId":"...","branch":"..."}：指定单 agent，缺省扇出全部在线
    22	// agent（未接入仓库的 agent RPC 报 remote required → 失败仅落日志）。
    23	// 执行异步：响应即返回受理结果，部署结果看部署历史与审计（username=webhook）。
    24	
    25	// handleStackWebhook webhook 入口（不走认证中间件，token 即凭据）
### 表一 缺陷（按风险排序）

| 问题 | file:line | 为什么是问题 | 建议动作 |
|---|---|---|---|
| Agent 注册缺少默认身份验证 | `internal/server/websocket.go:167-179`、`internal/server/websocket.go:129-164` | 数据库中没有该 Agent，或已有记录没有 secret 时，注册会被放行并加入在线 Registry。能访问 `/ws` 的人可冒充无 secret 的 Agent，接收发往该 ID 的 RPC 并伪造响应。 | 新 Agent 采用预置 secret 或一次性注册令牌；未配置凭据时默认拒绝，仅在显式开发模式允许匿名注册。 |
| 登录接口没有尝试限流或退避 | `internal/server/server.go:329-337`、`internal/auth/handler.go:54-74` | 登录路由公开，每次请求都直接校验密码；仓内未发现登录失败计数或限速逻辑，在线密码猜测没有应用层阻力。 | 按 IP 和账号限速，增加失败退避/临时锁定，并审计异常尝试；反向代理限流作为补充。 |
| 一次性远控 ticket 明文写入日志 | `internal/server/ticket.go:46-63` | 日志记录完整 ticket ID；ticket 在消费前最多有效 5 分钟，持有者可用它建立远控连接。与“不要在日志打印 ticket”的安全要求冲突。 | 删除 ticket 原文日志，仅记录会话 ID 或不可逆指纹；增加日志回归断言。 |
| 随附 Docker guacd 部署的回环中继地址不可达 | `internal/server/guac_relay.go:61-80`、`internal/server/api_guacamole.go:511-524`、`deployments/docker/docker-compose.yml:47-55` | Server 在自身容器的 `127.0.0.1` 开中继，并把该地址交给 guacd；两者虽在同一 bridge 网络，仍有独立 loopback，guacd 会拨自己的回环而非 Server。该部署形态下远控链路预计无法建立；容器实测未核实。 | 中继绑定到容器网络可达地址，并把该地址提供给 guacd；限制监听范围与防火墙，再补双容器端到端验收。 |
| 忘记密码接口泄露账号邮箱及账号状态 | `internal/server/password_reset_handlers.go:29-46`、`internal/server/password_reset_handlers.go:64-70` | 用户存在且绑定邮箱时，响应包含完整邮箱；不存在和未绑定邮箱又返回不同消息，可枚举账号并收集邮箱。 | 所有分支返回相同消息和相近时延；响应仅返回统一脱敏提示，不返回完整邮箱。 |

### 表二 文档与实现不一致

| 文档说法 | 文档位置 | 代码实际 | 代码位置 |
|---|---|---|---|
| 远控 WebSocket 的 ticket 都放在 `Sec-WebSocket-Protocol` | `docs/guide/architecture.md:121-126`、`docs/guide/protocol.md:169-175` | terminal 从子协议读取；Guacamole 优先从 URL `?ticket=` 读取，缺失时才回退子协议。 | `internal/server/api_remote.go:45-54`、`internal/server/api_guacamole.go:306-312` |
| Job“不做异步执行”，创建后同步等终态 | `docs/guide/jobs-design.md:127-132` | `POST /api/jobs` 建立 pending 记录后启动后台派发，立即返回 pending 视图。 | `internal/server/api_jobs.go:18-27`、`internal/server/api_jobs.go:95-101` |
| guacd 可通过 `GUACD_ADDR` 配置为跨机器地址 | `docs/guacd-deploy.md:60-68`、`deployments/guacd/docker-compose.yml:17-20` | guacd 收到的目标中继地址固定绑定在 Server 的 `127.0.0.1`；远端 guacd 的 loopback 不指向 Server。 | `internal/server/guac_relay.go:61-80`、`internal/server/api_guacamole.go:511-524` |
| 其他 `/api/` 资源接口都由 JWT 保护 | `docs/guide/architecture.md:100-102` | `POST /api/stacks/webhook` 是独立注册路由，不走 JWT 中间件，以 `X-Cockpit-Token` 验证。 | `internal/server/api_stacks.go:47-53`、`internal/server/api_stack_webhook.go:13-25` |

### 表三 值得补的能力

| 能力 | 为什么现在没有 | 价值高低 |
|---|---|---|
| Agent 准入策略与注册凭据生命周期 | 新 ID 可无 secret 注册，已有无 secret 的 Agent 也允许重连；当前文档将 secret 定义为可选。`internal/server/websocket.go:167-195`、`docs/guide/architecture.md:155-162` | 高 |
| 登录与密码重置限速、失败防护 | 登录是公开路由并直接校验密码；忘记密码也可公开触发邮件发送，代码未见限速实现。`internal/server/server.go:332-337`、`internal/auth/handler.go:54-74`、`internal/server/password_reset_handlers.go:49-62` | 高 |
| 可恢复的持久化任务派发与重试 | 派发依赖进程内 goroutine；重启后 pending/running 记录被标记失败，而不是重新入队或恢复执行。`internal/server/api_jobs.go:95-101`、`internal/server/orphan_run.go:10-20` | 中高 |
| 按 Agent 或资源范围授权 | 当前 RBAC 以角色和模块权限点为主；设计明确暂不做 Agent/部门级行隔离。`internal/server/rbac.go:29-59`、`docs/guide/rbac-design.md:137-140` | 中（团队部署时偏高） |
| Job 执行过程的实时输出与进度 | Job agent 端使用 `CombinedOutput` 等待进程结束后才返回；设计明确不做实时流。`internal/agent/rpc/job_provider.go:99-108`、`docs/guide/jobs-design.md:138-139` | 中
