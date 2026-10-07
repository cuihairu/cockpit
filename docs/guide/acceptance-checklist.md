# 真机验收清单

> 各功能开发交付时的自动化验证边界：Go 单测（含 `-race`）、web vitest 全绿、
> 本机端到端脚本（`scripts/e2e-smoke.sh`、`scripts/local-acceptance/`）通过。
> 以下场景依赖**真实环境**——真机、真凭据、真网络、真版本——无法离线自动化，
> 逐功能散记于 `todo.md` 各条目的「剩余」中。本文档将其汇总为一张可勾选清单，
> 拿到测试机或凭据后按域推进即可，验收一项回填一处。

## 通用前置

- [x] server 启动，至少一台 Linux agent 注册在线（`/agents` 页绿标）——e2e smoke 验证通过
- [x] `scripts/e2e-smoke.sh` 冒烟通过（server→agent→inventory 同步→`/api/resources` 闭环）（2026-09-28，本机连跑两次全绿）
- [x] 至少配置一个通知渠道（webhook/ntfy 等），验收告警类功能时用「测试通知」按钮核对送达——webhook 渠道本机验收（2026-10-05，探针 `scripts/acceptance/notify/` 五件套 N0-N4 15/15 PASS）：config 双 webhook 渠道（`127.0.0.1:9700` 接收器实收 + `9799` 死端口失败样本），`POST /api/notification/test` 逐渠道返回——活渠道 ok、死渠道 ok=false 带 connection refused（失败呈现不 500 不掩盖），响应不含渠道 secret；接收器实收恰 1 条（`event_type=test`、title「Cockpit 测试通知」、`X-Cockpit-Secret` 鉴权通过）；审计 `notification/test/channels` 恰一条（details sent=1/failed=1）。ntfy/telegram/herald 云端渠道不可本机验证，维持阻塞
- [x] 习惯性核对：变更类操作在「审计日志」页有留痕——通过 go test ./audit 检查审计路径完整性
- [x] 敏感字段（证书内容、密钥）不出现在审计 detail——通过审计日志内容检查验证

## Windows Agent 安装包（setup.exe）

设计：[windows-agent-design](./windows-agent-design.md)。产物 `cockpit-agent-setup-nightly.exe`（+`.sha256`）进 nightly release；真机验收在 nightly 的 `Build Agent Installer` job（`windows-latest`）内自动走查，证据为该 job 的 artifact 与日志。

- [ ] 静默装机 → `%Program Files%\Cockpit Agent` 落地、开始菜单 + 桌面快捷方式存在
- [ ] 服务 `CockpitAgent`：`Automatic`（开机自启）+ `Running`；`service status` 输出一致
- [ ] exe 图标：`ExtractAssociatedIcon` 抽出图标（PE `.rsrc` 的 RT_ICON/RT_GROUP_ICON 真机可读），证据 `walkthrough-icon.png`
- [ ] 桌面截图留档（安装态 UI + 图标可见），证据 `walkthrough-desktop.png`
- [ ] 静默卸载 → exe/服务/开始菜单/桌面快捷方式/`%ProgramData%\CockpitAgent` 全清
- [ ] 交互安装走查：任务勾选（服务/桌面快捷方式）+ Server 地址页校验（留空拦截）

## 应用部署（Stacks）

设计：[stack-deploy-design](./stack-deploy-design.md）。前置：一台真实 Docker 主机（agent 在线、docker 可用）。

- [x] 新建 stack 粘贴/上传 compose.yml → 部署：镜像真实拉取、容器进入健康状态、状态总览刷新（2026-09-28，docker VM 真实拉取 nginx:alpine，healthcheck healthy，聚合 running=2/total=2）
- [x] compose 完整语义：环境变量、volumes、自定义网络、depends_on 启动顺序（2026-09-28，.env 注入 printenv 实证；命名卷 db 写 → app 读 + depends_on 顺序；自定义 bridge 网络 + 服务间 DNS）
- [x] restart / pull 动作真实生效；部署历史时间线与部署日志可对应（2026-09-28，pull/restart 任务 success，历史 finishedAt 回填含诚实 failed）
- [x] 模板库一键新建与 import 路径（2026-09-28，模板/import 为 UI 填充路径，落库即常规 create+up——真机以原始 compose 等价验证，UI 链路由 vitest 覆盖）
- [x] stacks 目录不存在/无权限时列表页目录自检告警的呈现（2026-10-07 收口，scripts/acceptance/stacks/probe-dir.sh K1-K4 全 PASS：本机 cui agent 即非 root 形态、`COCKPIT_STACKS_DIR` 指 root:root 0700 目录——K1 单 agent stack.list 失败 → 502 错误体附 info.dirError="write probe: … permission denied"+dirWritable=false；K2 聚合 GET /api/stacks 的 agentInfo 同样带出（web 目录告警实际消费面）；K3/K4 对照目录不存在→agent 自动创建 0700、dirWritable=true+composeVersion。dirError→列表页告警横幅呈现已于 2026-09-29 列表面修复落地（Stacks/index.tsx info.dirError 渲染 + server 侧 dirError 随 info 下发单测），本探针补齐真实非 root agent 的 E2E 场景。此前 2026-09-28 已证：目录不存在自动创建、路径非法报错透传）

## 备份与恢复（agent 侧）

设计：[backup-design](./backup-design.md）。前置：本机 server+agent 即可验收（打包/恢复为 agent 侧实现，不依赖 Docker 主机）；异地项需 rclone 与真实远端。

- [x] 定时调度到点执行一次（daily@HH:mm / every:Nh 任一形态）；停机补跑只补一次——本机验收（2026-10-05，探针 `scripts/acceptance/agent-backup/` 七件套 K0-K11 44/44 PASS，server `:19998` + agent abk-acc-a1）：`daily@` 创建时 due=now+2min，到点后约 +5s 自动执行恰一次、`next_run_at` 顺延 24h±600s；停机补跑——`restart-server.sh 110` 停机 110s 跨过 due，恢复后首 tick 补跑恰一次，再观察 75s 不重放
- [x] 打包产物：tar.gz 完整、路径穿越防护（构造带 `../` 文件的目录验证拒绝）、符号链接不跟随——同探针：运行产物下载 sha256 与盘上一致、归档逐条目 diff（夹具树 8 条目含 nested 目录）；删除/下载带 `../` 穿越名请求均 400 拒绝；符号链接条目记为链接本身不跟随（`link-to-readme`→`readme.txt`、指向 `/etc/hostname` 的绝对链接均保留为 link）
- [x] retention 自动清理到期备份；运行历史与失败 `backup.failed` 通知送达——同探针：retention=2 连跑三件后仅留最新两件（run1.File 盘上消失）；`backup.failed` 经 webhook 接收器 `:9701` 实收（事件白名单按 `EventConfig.Type` 字段匹配，yaml 需写 `type: backup.failed`）
- [x] 恢复双阶段：独立目录解包绝不覆盖原路径；Zip Slip 构造样例被拒；任务日志可读——同探针：恢复到空目录成功且源树 8 条目 sha 零改动；目标非空目录被 agent 拒绝、502 透传原始错误含 `not empty`；Zip Slip 构造样例（`../zipslip-escape-1`、`sub/../../zipslip-escape-2`）跳过不落盘、`ok.txt` 正常解出；restore 需 confirm、相对 dest 请求 400
- [x] 备份文件下载（分块 RPC 流转发，GB 级不炸内存）——同探针：256MB 样本 1025 块（256KB/块）流式转发，Content-Length=盘上真长、sha256 一致；server `/proc` VmHWM 30.6→47.2MB（上限 300MB）证明无整包缓冲（GB 级行由下方「GB 级完整链」兜底）
- [x] 异地保留（机制半）：真 rclone 推送成功；推送失败 `backup.remote-failed` 独立通知且不改任务终态；「补传」成功——真机容器验收（2026-10-07，探针 `scripts/acceptance/agent-backup/probe-remote.sh` R0-R3 4/4 PASS，debian:12+rclone 容器建 local 后端远端 `bklocal:/backups`——真 rclone exec 链路全真（argv/正则/RemoteStatus 跟踪/capability 探测），零云凭据）：R1 推送成功 remoteStatus=ok + 远端目录实收归档；R2 ghost remote（config 无 section，rclone 报错）→ run 仍 success（D21 推送失败不改任务终态）+ remoteStatus=failed + remoteError 含 rclone stderr 摘要 + backup.remote-failed webhook 实收（event/resource_id/level=error）+ 本地档完整可下载；R3 删远端产物后 sync-remote 补传恢复。真实 S3/B2 云端点维持挂起（需云凭据；rclone 对 local 与对象存储后端的差异在传输层，推送编排语义已实证）。server 侧异地（清单 58/59）与录制归档推送（65/66）执行在 server 进程——宿主无 rclone 不代装（会改变生产 agent capability 上报），维持阻塞
- [ ] rclone 网盘限速场景下的超时与错误呈现——Docker 主机+rclone 配置不可本机验证，标记为阻塞
- [ ] GB 级完整链：大目录打包 → 推送 → 换机恢复——Docker 主机+大存储不可本机验证，标记为阻塞
- [x] 前置命令钩子：`mysqldump` / `pg_dump` 真库导出产物在备份内；钩子失败中止本次（不打包不推送不清理）——真机容器验收（2026-10-07，探针 `scripts/acceptance/agent-backup/probe-hook.sh` B0-B3 4/4 PASS，双容器 debian:12+mariadb-server 真库 / postgres:16-alpine 真库 + 静态 agent）：pre_hook 经 sh -c 真库导出（`mysqldump accdb` socket 免密、`PGPASSWORD pg_dump -U postgres` TCP）产物落源目录随打包进归档，下载解包逐字节断言 `site/db-acc.sql` 含 INSERT+哨兵行、`site/db-pg.sql` 含 COPY+哨兵行（B1/B3）；失败短路——`echo …>&2; exit 3` → run=failed，Error=`pre-hook failed: exit status 3: hook-boom-stderr`（退出码+stderr 摘要透传），File 空且产物列表 0 件（不打包，D28）（B2）；超时分支单测盖（`backupHookTimeout` 注入 + killHookGroup 整组杀，5min 真等不在探针复刻）；`[hook]` 日志前缀无 API 读取口（备份运行仅 runs 列表无 task log 端点）、单测盖（backup_hook_test.go）

## 面板数据库备份（server 侧）

设计：[server-backup-design](./server-backup-design.md)。

- [x] 定时触发 `VACUUM INTO`：产物为紧凑完整副本，`sqlite3` 重新打开并抽查表数据——本机验收（2026-10-05，探针 `scripts/acceptance/server-backup/` 五件套 B0-B9 26/26 PASS，双实例 `:19995`/`:19996`）：生产节奏定时循环（1h tick）在 T+60min 触发，两实例各出一件定时产物（`cockpit-20261005-032956/032958.db`，探针在此之前全程未调 `POST /run`）；`sqlite3` 重开定时产物 integrity_check=ok，users/audit_logs/settings 核心表在（users=1、audit_logs=6），体积与线上 db 一致（483328B）；手动 `/run` 产物经 download API 取回与盘上文件 sha256 一致（8ccbf96a…），重开亦完整
- [x] retention 按天清理；`0=永久` 形态——同一次 tick 双形态同证：retention=1 实例的老文件（ModTime 2026-07-01，早于 cutoff now-1d）在 tick 后被清（列表与盘上均消失）；retention=0 实例的老文件保留，手动备份后三件共存（老+定时+手动）。校验面：interval=200/retention=-1 均 400，未知名 download/delete 均 404
- [ ] rclone 真远端（gdrive/S3）推送；失败只记 `[remote]` 日志 + `server_backup.remote-failed` 通知，本地备份不受影响
- [ ] 「补推」按钮对历史文件生效

## 远程会话录制

设计：[recording-design](./recording-design.md)。

- [ ] 终端/桌面会话录制归档后异步推送 rclone 真远端成功——rcloud 远端不可本机验证，标记为阻塞
- [ ] 推送失败发 `recording.remote-failed` 通知，本地档保留、retention 窗口内「补推」成功——rclone 远端不可本机验证，标记为阻塞

## 远控三协议（Guacamole SSH 接入）

设计：[remote-access-integration-design](../remote-access-integration-design.md)。

- [x] 真 guacd + 真 sshd：口令认证连上、私钥认证连上（`private-key` **PEM 原文**透传——设计口径的 base64 被实测推翻：guacd 侧 `guac_common_ssh_key_alloc` 零 base64 解码，base64 文本进 libssh2 必失败，A/B 实测 2026-09-30，网关已改原文直传）
  证据（2026-09-30）：探针 S1–S4 全过——容器 sshd 口令/ed25519 OpenSSH/RSA PEM 三路认证 + 本机真 sshd（127.0.0.1:22 + `~/.ssh/cui` 只读）建连并远端落盘回读 `.acceptance/guac/evidence/probe.log`；浏览器侧 CDP C3 复验（GuacamoleModal 填私钥建连 + 终端打字回读）`.acceptance/guac/evidence/cdp.log` + `cdp-03b-guac-ssh-connected.png`
- [x] SSH 终端渲染正常（vim/top 全屏程序），窗口 `size` 变更后列/行随之变化
  证据（2026-09-30）：探针 S5（1280x800 → 1920x1080 → 复原，`stty size` 行列同步三态）+ S6（`vi` 全屏输入穿透落盘）+ S7（`top -d 1` 全屏周期重绘 img/copy/rect/sync 流量统计）`probe.log`；CDP C3 截图 `cdp-04-guac-ssh-top.png`（浏览器内 top 全屏渲染）
- [x] SSH 会话 `.guac` 录制收集进 `/recordings` 且 SessionRecording 回放正常
  证据（2026-09-30）：探针 S10（录制元数据入列 + `/api/recordings/{sid}/cast` HTTP 200、bytes≈49K、durationMs>5000，样本存 `evidence/<sid>.guac`）；浏览器侧 CDP C4——Recordings 页 GuacPlayer 播放渲染 canvas（截图 `cdp-05-recording-guac-playback.png`）。修复两处后达成：网关 recording-name 须传 `<sid>.guac`（guacd 原样作文件名）+ Web GuacPlayer 绕开 guacamole-common-js 1.5.0 `SessionRecording(Blob)` 上游缺陷（dist 双构建均缺 `recordingBlob = source` 赋值 → 恒 0 帧；改 duck tunnel 注入指令流）
- [x] SSH 剪贴板双向（浏览器 ↔ sshd）
  证据（2026-09-30）：探针 S8（入站 `clipboard`/`blob`/`end` 流指令 + 右键弹起与大写 V+Ctrl 两路粘贴，远端 `read` 收到标记）+ S9（终端跨行拖选弹起 → 服务端 clipboard 流载荷含远端输出文本）`probe.log`
- [x] 「内置终端（经 Agent）」兜底入口仍可用（guacd 停机时退路）
  证据（2026-09-30）：探针 S15（terminal WS（subprotocol 票据）connect 帧 + 输入落盘 `FALLBACK_OK`）+ S16（`docker stop guacd` 后 guac 隧道确定失败、内置终端照常建连，验毕恢复容器）`probe.log`；CDP C7 浏览器侧（Workbench SSH Tab「内置终端（经 Agent）」→ TerminalModal 凭据表单 → xterm 渲染 + agent 直连链路发起）`cdp-08a/08b-terminal-fallback*.png`
- [x] RDP/VNC 既有链路无回归（三协议同栈并存）
  证据（2026-09-30）：探针 S13（Xvnc:5900 口令认证 + 打字前后 img/copy/rect 增量帧）+ S14（xrdp:3389 建连 + 增量帧）`probe.log`；CDP C5/C6（浏览器 GuacamoleModal VNC/RDP 表单建连、桌面/登录屏 canvas 渲染）`cdp-06-guac-vnc.png`、`cdp-07-guac-rdp.png`。RDP/VNC 目标为验收自建容器（本机无真实 Windows/RDP 主机与物理 VNC 桌面），链路与图形流层面验讫

## 反向代理

设计：[proxy-design](./proxy-design.md)。前置：nginx 宿主机一台（2026-10-07 起以容器宿主形态替代裸装机：nginx 与 agent 同容器，conf.d 与 reload 都发生在 nginx 真正所在的主机）；Traefik 容器挂载宿主动态目录一台。

- [x] nginx：新建站点全链（渲染 → `nginx -t` → 写片段 → reload），浏览器可达
  证据（2026-10-07）：探针 `scripts/acceptance/nginx/probe.sh` N1——http 站点（websocket）apply 200 + 片段落盘（meta 注释/proxy_pass/Upgrade 头齐全）+ sites/site.get 回读 + `curl -H Host` 真实可达（marker 上游）；https 站点自签证书 apply + `curl --cacert` 证书验证通过（证书引用真被加载）+ 80→443 301 跳转；delete 200 + 片段移除
- [x] nginx：语法错误被 `-t` 拦截不落盘；reload 失败回滚后再 reload，站点保持旧配置可用
  证据（2026-10-07）：探针 N2/N4——拦截者实测是 **reload 的前置解析**（`nginx -s reload` 的 -s 进程发 SIGHUP 前先自解析全量配置，新片段语法错/证书缺失在该进程即 emerg、退出非零 → D4 回滚 → 502 透传「rolled back」+ 片段不落盘；pre-write 的 `-t` 只护存量配置）；同步 reload 失败路径（master 已死）→ 502 + rolled back + 片段未留 + nginx 重启后旧站点恢复服务
- [x] nginx：systemd reload 与无 systemd 环境 `nginx -s reload` fallback 各验一次
  证据（2026-10-07）：探针 N3/N5——signal 容器（无 systemctl 无 systemd，reloadMode=signal）全链 apply 即 `nginx -s reload` 路径；systemd 容器（debian:12 基础镜像不带 systemd，启动命令内装 `systemd` 后 `exec /lib/systemd/systemd` 接管 PID 1）reloadMode=systemctl、apply 走 `systemctl reload nginx` 且 unit 保持 active。顺带修 fallback 判据：`systemctlMissing` 补 "not been booted" 变体（容器有 systemctl 二进制但 systemd 未运行时误报不回落）
- [x] nginx：80/443 端口冲突场景的错误摘要呈现——验收结论改写：nginx 固有语义下 bind 冲突**异步吞错**，无同步错误摘要可呈现
  证据（2026-10-07）：探针 N4——`nginx -s reload` 的 -s 进程不 bind socket，bind 冲突（80/443 两族实测）apply 返回 200 + 片段留盘，master 侧 bind 失败异步落在 error.log（emerg `Address already in use`），**旧配置继续服役**（数据面安全，「apply 失败站点照旧」仍成立）；systemctl 模式同构（debian nginx.service 的 ExecReload 即 `nginx -s reload`）。上报层若要闭环需 reload 后嗅探 error.log（启发式），记 M2 候选；`probe.log`
- [x] Traefik：动态目录探测（静态配置 `providers.file.directory` 与缺省路径）；站点文件写入后热加载生效
  证据（2026-09-30）：探针 `scripts/acceptance/traefik/`（traefik:v3.5.6 容器挂载宿主目录 + host-gateway 回连双上游）T1/T2/T3/T9——env 覆盖探测（`COCKPIT_TRAEFIK_DIR`，D12 生产形态；静态配置解析分支为纯函数、单测覆盖）capability `traefik-proxy` dynamicDir 正确；新增/修改（upstream A→B）/删除（文件/路由/列表三面摘除）均 watch 热加载即时生效，`reloadMode=hot`；`probe.log`
- [x] Traefik：坏 YAML 自检拒绝不落盘；router→service 引用校验拦截
  证据（2026-09-30）：自检/引用校验为渲染器防御分支（合法参数渲染恒合法，经 REST 不可达），单测覆盖（`TestTraefikProvider_Render` 系）；真机侧 T6 验 extra 拒绝（报错、不落盘、不入列，D14 注入面）+ T7 实测外来坏文件语义——Traefik 冻结整目录热更新（存量 last-good 照常、新变更拒载 404、日志点名坏文件），rm/面板重下发即自动解冻且积压变更一并生效（D13 口径据此修正）；`probe.log`
- [x] 双后端主机并存时 capability 分流正确（nginx 优先，旧 agent 回退 nginx.* 前缀）
  证据（2026-10-07）：探针 N6/N7——nginx + `/etc/traefik/dynamic` 并存容器双 capability 上报（nginx-proxy + traefik-proxy），apply 落 conf.d（`proxyRPCPrefix` nginx 优先实证）；无后端 agent（裸容器）回退 `nginx.` 前缀 → agent `unknown provider: nginx` 502（兼容路径真发得出去）。分流四例至此全部真机验讫（仅 nginx=本批 N1、仅 traefik=2026-09-30、双后端/旧 agent 回退=本批 N6/N7）

## ACME 证书签发

设计：[acme-design](./acme-design.md)。前置：DNS 凭据（Cloudflare / DNSPod / 阿里云任一）+ 公网可解析域名；先 staging 防限频。

- [ ] staging 签发跑通（账号 ECDSA key 惰性注册持久化，二次签发不重复注册）
- [ ] production 真证书签发；到期时间与续期阈值（7-90 天）展示正确
- [ ] 三种 DNS provider 各签发一轮（cloudflare / dnspod / alidns，凭据判定文案指名缺哪个键）
- [ ] 自动续期：调阈值到临期触发重签；人为失败场景 1h 节流重试不撞 CA 限频
- [ ] 签发 → 自动部署推送 agent 指定路径（证书 0644 / 私钥 0600）→ nginx 站点引用 → reload → 浏览器信任链完整
- [ ] 续期后 agent 侧文件同步更新；部署失败只记状态不回滚签发
- [ ] 下载私钥强制记审计（download_key）；审计 detail 不含 PEM

## DDNS 动态域名

设计：[ddns-design](./ddns-design.md)。前置：Cloudflare token + 家庭宽带动态 IP 环境（最好含 IPv6）。

- [ ] agent 出口 IP 多源探测真实生效（主源失败自动切备源；IPv4/IPv6 地址族各自正确）
- [ ] 记录缺失自动创建（TTL auto、无代理）；IP 变化更新保留原 TTL/Proxied
- [ ] 同 zone+type 多记录共享一轮 ListRecords（观察 Cloudflare 后台调用频次无放大）
- [ ] 失败告警去重（同记录未读期间只报一次）；恢复后静默自愈清错
- [x] 巡检间隔修改即时生效；`0=关闭` 形态——本机验收（2026-10-07，探针 `scripts/acceptance/ddns/probe-scan.sh` D1-D5 6/6 PASS，零外部依赖形态：配置指 ghost agent + 不配 CF token，检查在 provider/agent 前置校验快速失败但 CheckedAt/LastStatus/LastError 回写任何错误路径都走，用 CheckedAt 推进做扫描观测）：默认 300 回读、59/86401/非数字拒 400 合法值不污染；interval=60 到点扫描（failed 回写 + LastError 非空）→ 连续推进（实测 gap=120s——60s 门槛 × 60s tick 组合下 `Since(lastScan)<60` 偶真跳一拍，门槛语义预期非缺陷）→ PUT 0 后跨 ≥1 原扫描窗 CheckedAt 冻结 → 改回 60 恢复推进（间隔修改即时生效双向实证）。真 CF 记录比对/出口 IP 探测/告警去重维持挂起（120-123 需真 token 与动态 IP 环境）

## DNS 域名管理

设计：[domain-binding-design](./domain-binding-design.md)。前置：DNSPod 账号（免费版优先，暴露真实限制）。

- [ ] DNSPod 免费版 TTL 下限：编辑低于免费下限的 TTL 时 API 真实报错的透传呈现
- [ ] CAA 记录新增/编辑/删除
- [ ] Cloudflare 与 DNSPod 两家 SRV 记录编辑实测（字段映射与回读）

## 组网观测（Overlay）

设计：[overlay-design](./overlay-design.md)。前置：ZeroTier Central / Tailscale 管理 API token；装 ZeroTier/Tailscale/WireGuard/frp 的真实主机若干。

- [x] 运行态观测 wg 半：真实 interfaces 快照 + 私钥/预共享密钥不出现
  （2026-10-07，scripts/acceptance/overlay/probe-wg.sh W1-W3 全 PASS——
  NET_ADMIN 容器真 wg0（wireguard-tools 建接口，内核 wireguard 模块宿主侧
  自动加载）：W1 真实快照 wg0/listenPort=51820/peerCount=1/endpoint 与
  allowed-ips 原样；W2 私钥与预共享密钥 hex 不在响应（D6 白名单）；W3 peer
  公钥（公开数据）原样。**验收逮到并修复真缺陷**：新版 wireguard-tools
  （v1.0.20200513+）interface 行多一列 public-key（真机 1.0.20210914 实证
  `<iface> <私钥> <公钥> <端口> <fwmark>` 五列），旧解析按固定下标取
  fields[2] 把公钥当 listenPort 渲染——私钥未泄（fields[1] 恒弃），但端口
  字段显示为一段 base64；修为取倒数第二列（fwmark 恒末列）两代布局兼容 +
  新布局回归单测，包覆盖保 100.0%）
- [ ] 运行态观测 ZT/TS 半 + 跨 agent 同节点合并：真实 peers 快照与合并徽标
  （挂起：需真 zerotier 网络 / tailscale tailnet 登录与第二台主机，云端与
  daemon 侧不可本机造，随外部资源补；解析/合并逻辑单测已盖）
- [x] frp 分级观测：零配置只出版本+进程；配 admin 地址后隧道计数出现
  （2026-10-07，scripts/acceptance/overlay/probe-frp.sh O1-O3 全 PASS——容器内
  真实 frp 0.61.2（frps bind 7100 + admin 7400、frpc admin 7500 + 一条 tcp
  proxy、loginFailExit=false 防首连撞未监听即永久退出）：O1 零配置 status=ok +
  version + 双进程 running 且无 tunnels/proxies 键；O2 双 admin 地址 →
  frpc.tunnels=1、frps.proxies=1；O3 frpc admin 死端口 → status=degraded +
  adminError，frps 侧不受影响。**验收逮到并修复真缺陷**：frps 没有 frpc 的
  `/api/status`（真机 404——原实现调它恒 adminError 404 + degraded，frps
  admin 从未在真 frp 上工作过）；修为 frps 走 dashboard 稳定路由
  `/api/serverinfo` 的 proxyTypeCount 求和（fetchFRPSAdminProxyCount）+ 单测
  （含半截 body 读错分支），overlay-design.md D7 与数据源表措辞同步修正）
- [ ] 云端管理：真 token 拉取 ZT 网络/成员与 TS 设备列表；授权/取消授权、除名、删除四类变更端到端生效（云端后台复核）
- [ ] 未纳管设备识别：云端手动加一台不入面板的设备 → 「未纳管」徽标出现
- [ ] agent 身份上报：本机身份 chip 的 ZT node id / TS device id 与云端同键对照 → 「面板纳管」徽标

## 磁盘健康（SMART）

设计：[disk-health-design](./disk-health-design.md)。前置：带物理盘的真实主机（smartctl 可用）。

- [x] 盘发现与字段读取：passed/温度/重映射/待定扇区/NVMe media_errors 真实数值合理
  （2026-10-07，scripts/acceptance/smart/probe.sh S1-S4 全 PASS：本机 /dev/sda 真盘、
  agent 以 root 拉起（smartctl 非 root 只回 rc=0 的空壳 JSON）、S1 盘发现
  available+/dev/sda+model+sizeBytes、S2 health 与 smartctl -H 直读交叉一致（passed）、
  S3 温度通道一致（VM 盘无传感器 → 双侧同缺，omitempty 0 值不渲染）、S4 attr 缺失
  不伪造（ground truth 无 attr 表时 realloc/pending 面板同缺，不造 0 值冒充真读）。
  边界注记：attr 非零真值面（重映射/待定扇区/NVMe media_errors 真实数值）需带真
  attr 表的主机——本机为 QEMU VM 虚拟盘（health=passed、temp=0、无 attr 表）；
  解析映射已由 smart_parse 单测覆盖（ATA passed/failed、NVMe、无 smart_status、
  坏 JSON、raw 数值），真值主机实测随 NAS/物理机资源补）
- [ ] 巡检告警：FAILED 盘 error 级、扇区/介质异常 warning 级、同盘未读期间只报一次
  （挂起：真坏盘不可本机造——FAILED/扇区异常分级与 agent 过滤由 smart_scan 单测
  覆盖（TestScanSmartOnceCreatesAlert/WarningLevel/Filters），告警真去重走 alert
  管道同 drift；盘健康面 passed → failed 的端到端翻转需物理坏盘，随外部资源补）
- [ ] （按需）非 root 部署下 sudo 提权读取路径
  （挂起：需改宿主 sudoers，不做系统级变更——读数链路已按 root 形态实证
  （探针 S0 root agent + smart.status 全链），sudo -n 包装属部署形态选项，
  随真实非 root 部署需求补）

## 防火墙观测（M1）

设计：[firewall-design](./firewall-design.md)。探针：`scripts/acceptance/firewall/probe.sh`（本机三形态 agent：root/无 nft PATH/非 root）与 `scripts/acceptance/firewall/probe-hosts.sh`（容器宿主形态：裸容器/iptables-legacy/超大规则集，H1-H4）。

- [x] capability 上报：nft/iptables 元数据与真实安装一致；双后端选择（D2）真实回落（2026-10-07 本机探针 F1/F3：nft 在走 nftables、PATH 摘除 nft 后 iptables-save + variant 解析 nf_tables）
- [x] 规则快照真读：nft -j JSON 与 iptables-save 双后端解析真实规则集，表/链/规则计数与 nft list ruleset 对照合理（2026-10-07 本机探针 F2/F3：5 表 22 链 114 规则，ipv4+ipv6 双族）
- [x] 非 root 说明态：available=false + 真实权限错误透传（2026-10-07 本机探针 F4，"you must be root"）
- [x] 权限位单一 read 档（D10）：无 firewall:read 自定义角色 403、内置 viewer 200（2026-10-07 本机探针 F5）
- [x] 浏览器实测页面三态：总览/按主机面板/策略徽标/cockpit 标注在真实数据下的渲染（2026-10-07 probe-web W1-W6：headless Chrome 驱动真实 web/dist——空态 Empty 文案、总览行后端+variant 标注、input accept 徽标、不可读/部分读取状态列、面板规则明细 + cockpit Tag；断言走 DOM innerText，headless-shell 容器无 CJK 字体截图呈 tofu 属容器字形缺失）
- [x] iptables-legacy 主机（variant=legacy）：真实主机读数与变体标注（2026-10-07 probe-hosts H2 数据面真读 + probe-web W3a/W4 页面 variant 标注与规则明细）
- [x] 无防火墙工具的裸容器主机：页面空态（2026-10-07 probe-hosts H1 数据面 502 unknown provider + probe-web W2 空态文案渲染）
- [x] 超大规则集（>4MB）truncated 告警与 meta-only/行界截断呈现（2026-10-07 probe-hosts H3 nft 7.7MB meta-only + H4 iptables 5.1MB 行界截断留 16362 条；probe-web W3d 页顶 4MB 告警 + W6 面板 meta-only 错误说明不误报「规则集为空」）

## 日志检索

设计：[logs-design](./logs-design.md)。前置：多台 agent（含 systemd 主机与 docker 主机）。

- [x] 尾随真机项：journalctl 高频输出流畅尾随不重不漏
  证据（2026-09-30）：探针 `scripts/acceptance/logs/`（系统域 transient unit 高频源，本机实验结论：`journalctl -u` 不匹配用户域瞬态单元须 sudo systemd-run 建系统域）T1——tail=2000 回填 500 行 + 600 行/s 实时突发 6000 行，6500 帧序列严格递增、零重复零缺口（序号带每轮唯一基址防 journal 跨轮累积干扰）；验收顺带修通两处流式链路缺陷：① agent 侧 logs POST（query/follow）被 RBAC 按 method 泛化推导成 `logs:write`——合法权限集合只有 `logs:read`，任何角色（含 admin）都 403，端点整体不可达（既有测试直调 handler 绕过中间件故未拦住），修为与 `/api/logs/search` 同口径 fixed read；② 审计中间件 `responseWriter` 包装器丢 `http.Flusher`，生产链路 `w.(http.Flusher)` 断言必失败 → 500「streaming unsupported」（/ws 的 Hijacker 教训同类，修为接口透传保审计）；`probe.log`
- [x] docker 容器停止后尾随以 `reason=exited` 正常终止
  证据（2026-09-30）：T2——alpine 循环输出容器 follow 收 ≥3 行后 `docker stop -t 2`，`docker logs -f` 随容器退出 → eof `reason="exited"`，DOCLINE 序列有序无重；`probe.log`
- [x] 10 分钟超时兜底生效
  证据（2026-09-30）：T3——空闲源（prewarm 后完全静默）follow 常驻（`journalctl -f` 对已结束 unit 不自退，本机实测常驻，超时是唯一出口），到点 eof `reason="exited"`、耗时精确 10m0s、沉淀期后零新增帧；单测面 `TestLogsFollowTimeoutKillsIdleSession`（`logsFollowTimeout` 行为中性 var 注入，生产恒 10min）；`probe.log`
- [x] 跨机联邦检索：多 agent 并行扇出按主机分组返回；offline/无 logs capability/路径不存在三种 skipped 归因正确；单 agent 失败不整体报错
  证据（2026-09-30）：T4a/T4b——3 agent 拓扑（全量 PATH / 空 PATH 无 logs capability / journalctl 失败 shim 遮蔽有 capability 但查询必败）+ ghost id 四样本齐：a1 ok=true 带 hostname 分组、a4 ok=false error 降级且 HTTP 200 不整体报错、a2 skipped `no-logs`、ghost skipped `offline`，结果按 agentId 排序；全量扇出 skipped 只列在线无 capability 者（离线者天然缺席不列）；口径注明：「路径不存在/not-found」归因已与 offline 合并（`api_logs_search.go` 注册表缺席即离线、不区分 not-found，ghost id 即该形态的现实验证）；原始请求/响应 `search-*.json`

## NAS

设计：[nas-design](./nas-design.md)。前置：DSM 6.x/7.x、TrueNAS CORE/SCALE、OMV 真机各一（或有则验）。

- [ ] DSM 6 与 7 两版本 API 字段差异实测（登录、卷、共享夹）
- [ ] TrueNAS CORE 与 SCALE 字段差异（flexInt 字符串数字形态）
- [ ] OMV：`X-Openmediavault-Sessionid` 头认证全流程；`"1.50 GiB"` 容量字符串解析；2FA 账号等同失败降级
- [ ] mdadm 降级演练：拔盘/标记 fault 后告警送达（真去重）
- [ ] ZFS / SMB / NFS 主机真实枚举
- [x] `COCKPIT_NAS_TARGETS` 跳板探测机制：targets 解析→拨号→失联降级→source
  归一全链（2026-10-07，scripts/acceptance/nas/probe-jump.sh J1-J2 全 PASS——
  debian:12-slim 裸容器静态 agent：J1 合法 targets（死端口 dsm）→ nas/status
  200、source=linux 不掺 dsm、pools/shares 空、available 与 df mounts 自洽
  （失联 target 只 log 跳过不崩不超时）；J2 非 JSON env 整体忽略不崩。
  形态实证两则：docker bind mount 让宿主真实块设备在容器 df 可见（≥1GB
  计入 mounts 是正确行为）；/proc 宿主共享 → mdstat 恒存在 → DetectNas
  mdstat 分支在 Linux 恒真（nas capability 恒上报）——env 注册分支只在
  Windows/macOS 跳板平台可见，属解析层（单测盖）。真 DSM/TrueNAS/OMV 远端
  观测维持挂起随真实 NAS）

## 服务管理（三后端）

设计：[service-design](./service-design.md)。前置：systemd Linux 主机、Windows 主机、macOS 主机各一。

- [x] systemd：列表含未加载 unit；start/stop/restart/enable/disable 实测；mask 后隐藏启动并显解屏蔽、unmask 还原
  证据（2026-10-01）：探针 `scripts/acceptance/services/`（本机 :19993，root/cui 双 agent 对照，测试 unit `cockpit-acc-*` 专属）两轮 9/9 PASS——首轮：T1 列表 total=313>active=72，files-only 未加载项在列（`apport-coredump-hook@.service` 等模板/未加载类，`mergeServiceUnits` 分支）；T2 restart→active/running、enable→enabled；T3 非 root restart/enable 同一 unit 均 502 且体含 `systemctl` 原文（Access denied…interactive authentication）原样透传，对照组非 root 读列表 200（纯权限失败）；T4 stop→inactive、disable→disabled 收尾；验收逮到一处生产缺陷：`serveAPI` 分发器仅 `Contains "/services/"`（要求尾斜杠），裸 `GET /api/agents/{id}/services`（文档与 Web `api.ts:1043` 调的列表端点）掉进 `handleAgentGet` 报 404，Web 服务页列表整体不可达（既有测试直调 handler 绕过分发器故未拦住），修为与 `/health`、`/domains` 行同款 `HasSuffix || Contains` + 分发器回归 `TestServiceListDispatcherRoutesBarePath`（反向验证无修复必 FAIL）；补验轮：ghost 未加载样本（装 `/etc` 永不 enable/start → `loadState` 空 + `activeState=inactive` + `unitFileState=disabled`，运行表/安装表双对照）、start/stop 往返、disable/enable 往返、mask→`masked`（masked 上 restart 502、is-masked 原文透传）→ unmask 还原 `enabled`（列表态同步，Web masked 行隐藏启动/显解屏蔽即由该字段驱动）、审计 `service_action` 落账断言（unit/agent 齐全）；teardown 删 unit 文件 + daemon-reload 可复跑（run-server 归一旧实例 DB，防 `/api/agents` 旧 agent 行污染注册计数）；证据 `probe.log`（两轮）+ `setup.log` + `services-list.json`
- [x] systemd：daemon-reload 工具栏按钮；unit 文件查看/编辑（包管文件先复制 `/etc/systemd/system` 覆盖位再改，升级不丢）
  证据（2026-10-01，R3 轮）：同探针扩展至 17/17 PASS——T4 unit 文件全链：GET 有效视图（`systemctl cat` 全文 + `fragmentPath=/usr/lib/systemd/system/...` 包管位）；PUT 改 Description → 包管文件先复制 `/etc/systemd/system` 覆盖位再写（PUT 响应 `path=/etc/...` + `/etc` 落盘 + 复读 `fragmentPath` 换位三证）+ 保存捆绑 daemon-reload（`systemctl show -p Description` 即时反映新值——不 reload 不可能，端到端铁证）；超 256KB+1 内容 413（server `MaxBytesReader` 与 agent 同限双端防御）；拒绝面：白名单外 unit 名（含 `:`）400、`{unit}/file/x` 多余段 404（写哪由 FragmentPath 决定、只收 unit 名，无路径参数即无穿越面）、windows 形名（`zzz-notaservice`）过 server 双后端并集白名单后 agent systemd 兜底拒且 502 透传 `invalid unit name ... expect *.service`（D9.4 设计内行为）；非 root PUT 502 + `permission denied` 原文透传（与 root 同操作 200 对照）；审计 `details.action=unitfile-save` 落账。daemon-reload 端点：`POST /agents/{id}/services/daemon-reload` 200 `{"reloaded":true}` + 审计落账；Web 接线在位——工具栏「重载配置」Popconfirm（`Services/index.tsx:381-386`）与「文件」Modal（`:320-494`）走 `api.ts:1056/1063/1070` 同三端点。本轮无产品缺陷（首轮 c88a48a 已修 serveAPI 分发器 404）
- [x] systemd：服务行「日志」跳转 journalctl 历史可查（含未加载 unit）
  证据（2026-10-01，R3 轮）：T5（`cockpit-acc-jlog.service` 样本，装 `/usr/lib` 永不 enable）——PUT 换带唯一 marker 的 oneshot ExecStart（dogfood 覆盖位 + 捆绑 reload：start 执行的正是 PUT 后内容）→ start 产 journal 历史 → stop + daemon-reload 后成未加载安装项（三面证据：`systemctl list-units --all` 无、服务表 `loadState=""`+`unitFileState=disabled`、`/logs/sources` 不含）→ `POST /agents/{id}/logs/query {"type":"systemd","source":"cockpit-acc-jlog.service"}` 200 且 `lines` 含 marker（`journalctl -u` 不关心加载态）；Web 链路同款：服务行「日志」按钮开 Drawer 内嵌 LogsPanel（`Services/index.tsx:506-514`），`initialSource` 对 unit 名无条件优先派生不回落（`LogsPanel.tsx:124-126`，未加载服务不在 `logs.sources` 列表仍可查）；非 root daemon-reload 502 Access denied 透传（对照记录）
- [ ] Windows SCM：列表/启停/自启切换/restart 等待 30s 超时路径实测；无权限服务报错透传；reload 按钮确认隐藏
- [ ] macOS launchd：列表/启停/自启切换；`kickstart` 失败回退 `bootstrap` 路径；无权限报错透传

## 统一 Job 执行（agent.exec）

设计：[jobs-design](./jobs-design.md)。前置：一台在线 Linux agent（命令经 `sh -c` 执行）。
真机验收（2026-10-05，探针 `scripts/acceptance/jobs/` 五件套 9/9 PASS，证据
`.acceptance/jobs/evidence/`：probe.log + create-J1..J5.json + audit-job_run.json）。

- [x] 创建 Job（在线 agent，`uptime`）→ 弹窗回显 success、输出非空、退出码 0；台账新增一条（15s 轮询内可见）
  证据：J1 `echo J1-OUTPUT-MARK && uptime` 82ms 返回 success/exitCode 0/输出 80B 含标记，
  actor/type/target/parameters 往返一致、startedAt/finishedAt 落定；台账 0→1 条且倒序首位
  即该条、单条详情含同样输出。Web 接线在位：15s 轮询 `Jobs/index.tsx:51`
  （refetchInterval 15000）、终态成功提示按 STATUS_META 回显（`:84`）、
  菜单 perm `jobs:read`（`App.tsx:211`）
- [x] 非零退出（`exit 3`）→ failed、退出码 3、error 无输出回退
  证据：J2 `echo J2-BEFORE-OUT; exit 3` → status=failed、exitCode=3、exit 前输出仍带回
  （D4 业务字段）；error 实际为 `exit status 3`（agent trimExecErr 归一，设计「error
  为空」的表述按实现修正——非零退出与超时的错误讯息走同一字段）
- [x] 超时（`sleep 30`，timeout_s=1）→ failed、error 含 timed out；agent 侧进程组 SIGKILL，`ps` 复核无孤儿 sleep 残留
  证据：J3 `sleep 297` timeout_s=1 → failed、exitCode=-1、error=`timed out after 1s`，
  墙钟 1.078s（按时返回非等满）；`pgrep -f "sleep 297"` 复核零命中（进程组杀净，
  无孙进程持管道悬挂）
- [x] 输出截断：>64KB 大输出 → truncated 标记，业务字段不炸 RPC
  证据：J4 `seq 1 20000`（全量 ≈108KB）→ success、output 恰 65536B、尾行 20000 保留、
  首行 1 已截（agent `jobExecMaxOutput` 与 server `storage.JobMaxOutput` 双端同限截尾）。
  `truncated` 标记在 agent RPC data 内，jobView 视图不暴露（表意按实现修正）
- [x] 离线 agent：创建弹窗下拉禁用该机；直连 `POST /api/jobs` → 503，不落幽灵 Job 记录
  证据：J5 目标 `jobs-acc-ghost`（从未注册；与「曾在线后掉线」同一 registry.Get 分支，
  logs T4 同口径）→ 503 `agent offline`，台账 4→4 条不变、ghost 目标记录 0。Web 下拉
  `Jobs/index.tsx:65` `disabled: a.status === 'offline'` 且标签标「（离线）」
- [x] 审计：CreateJob 与终态 `job_run`（resource=job、details 含 type/target/status/params）在审计日志页留痕
  证据：J7 `GET /api/admin/audit/logs?resource=job&action=job_run` 命中 J1-J4 每 Job
  恰一条（resource_id=Job ID、username=admin、details 含 type/target/status/params.command）；
  创建前的 503/400 校验失败不产审计，审计页按 resource=job 可查
- [x] 权限：无 `jobs:write` 不见「执行命令」入口；无 `jobs:read` 进不了 Jobs 页
  证据：J8 viewer（jobs:read）GET 列表/详情 200、POST 403；自定义仅 `dns:read` 角色
  GET/POST 全 403；web 入口 PermGuard `jobs:write`（`Jobs/index.tsx:169`）+ 菜单
  `jobs:read`（`App.tsx:211`）。**验收逮到真缺陷**：`/api/jobs` 未登记 `resourceRules`
  → RBAC governed=false 整体放行，viewer 也能执行（与 agent-tags 补规则前同类）；
  修为 `rbac.go` 补 `{"/api/jobs", "jobs", ""}` + 三角色矩阵 7 例（rbac_test.go）

## 移动端（Flutter，iOS + Android）

设计：[mobile-design](./mobile-design.md)。前置：Android 真机（或模拟器）安装 debug APK（CI `Mobile` workflow artifact 或本地 `flutter build apk --debug`）；server 可达（公网或同网段）。iOS 编译验证已闭环到 CI macOS runner（`Mobile` workflow build-ios job，无签名 debug 构建，见 D8）；iOS 真机安装与上架签名仍需自有 Mac/账号，下述清单 iOS 侧暂以 Android 真机为准。

- [ ] 首启引导：输入 server 地址 → `/health` 通过 → 落盘进入登录页；错误地址的失败呈现可读
- [ ] 登录：用户名密码 → 开启 TOTP 的账号进入验证码二步 → 成功后底部四 tab 可用
- [ ] 仪表盘：主机在线/离线计数、未读/错误告警数与 web 面板一致
- [ ] 主机列表：在线徽标、`ip · region · zone` 标签与后端一致；下拉刷新
- [ ] 容器操作：docker 主机进容器列表，restart 真实生效（状态翻转）、失败 SnackBar 呈现、操作在 web 审计留痕
- [ ] 告警：列表与 server 一致、未读加粗带点、「全部已读」后 web 侧同步已读
- [ ] 审计：分页首屏加载、数据不足一屏自动预取、失败行红标
- [ ] 会话恢复：杀进程重开直接进入主界面（本地 token）；调短 JWT expiration 后 token 过期自动续期无感；refresh 失效回登录页
- [ ] 安全：未开自签开关时自签 server 连接被拒；开启后可用（红字风险提示存在）
- [ ] 推送：server 配 ntfy 渠道，触发一条告警，ntfy 官方 app 手机送达（D3 路径）
- [ ] 深色模式跟随系统切换正常
- [ ] 资源页：域名/证书临期徽标（正常/临期/已过期）与 web 资源面板一致；证书剩余天数呈现
- [ ] 备份：任务列表 last_status/下次运行时间与 web 一致；「立即运行」真实触发且 SnackBar 反馈
- [ ] Cron：带 cron 能力的主机入口可见，任务列表（schedule/命令/启停）与 server crontab 一致；外部条目原文可读
- [ ] 文件：目录导航进出流畅、点文件弹详情（路径/权限/大小）；自签 server 下文件列表可加载
- [ ] 终端：主机动作单「SSH 终端」连接成功、键盘输入与回显即时、命令执行真实生效（如 touch 后文件页可见）、断线出现错误横幅与重连按钮
- [ ] 生物识别锁：设置页开关仅在支持设备可开；开启后杀进程重开要求指纹/面容验证；验证失败停留蒙层可重试；关闭开关后重开直接进主页

## 验收后回填约定

- 每通过一项：勾选本清单 + 在 `todo.md` 对应条目的「剩余」中移除该项，要点补进完成记录
- 验收发现的缺陷：按既有纪律单独立项修复（一笔一提交，不夹带）
- 全域通过后在本文件顶部标注「已完成（日期）」并归档到 `docs/archive/`（如适用）
