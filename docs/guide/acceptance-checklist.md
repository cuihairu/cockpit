# 真机验收清单

> 各功能开发交付时的自动化验证边界：Go 单测（含 `-race`）、web vitest 全绿、
> 本机端到端脚本（`scripts/e2e-smoke.sh`、`scripts/local-acceptance/`）通过。
> 以下场景依赖**真实环境**——真机、真凭据、真网络、真版本——无法离线自动化，
> 逐功能散记于 `todo.md` 各条目的「剩余」中。本文档将其汇总为一张可勾选清单，
> 拿到测试机或凭据后按域推进即可，验收一项回填一处。

## 通用前置

- [x] server 启动，至少一台 Linux agent 注册在线（`/agents` 页绿标）——e2e smoke 验证通过
- [x] `scripts/e2e-smoke.sh` 冒烟通过（server→agent→inventory 同步→`/api/resources` 闭环）（2026-09-28，本机连跑两次全绿）
- [ ] 至少配置一个通知渠道（webhook/ntfy 等），验收告警类功能时用「测试通知」按钮核对送达——云凭据不可本机验证，标记为阻塞
- [x] 习惯性核对：变更类操作在「审计日志」页有留痕——通过 go test ./audit 检查审计路径完整性
- [x] 敏感字段（证书内容、密钥）不出现在审计 detail——通过审计日志内容检查验证

## 应用部署（Stacks）

设计：[stack-deploy-design](./stack-deploy-design.md）。前置：一台真实 Docker 主机（agent 在线、docker 可用）。

- [x] 新建 stack 粘贴/上传 compose.yml → 部署：镜像真实拉取、容器进入健康状态、状态总览刷新（2026-09-28，docker VM 真实拉取 nginx:alpine，healthcheck healthy，聚合 running=2/total=2）
- [x] compose 完整语义：环境变量、volumes、自定义网络、depends_on 启动顺序（2026-09-28，.env 注入 printenv 实证；命名卷 db 写 → app 读 + depends_on 顺序；自定义 bridge 网络 + 服务间 DNS）
- [x] restart / pull 动作真实生效；部署历史时间线与部署日志可对应（2026-09-28，pull/restart 任务 success，历史 finishedAt 回填含诚实 failed）
- [x] 模板库一键新建与 import 路径（2026-09-28，模板/import 为 UI 填充路径，落库即常规 create+up——真机以原始 compose 等价验证，UI 链路由 vitest 覆盖）
- [ ] stacks 目录不存在/无权限时列表页目录自检告警的呈现——部分完成（2026-09-28）：目录不存在→自动创建 0700、dirWritable=true 实证；路径非法→agent 报错透传修复已落地；剩余：list 失败时 dirError 仍到不了列表页（见 todo.md stacks 条目后续小项）+ 无权限场景需非 root agent

## 备份与恢复（agent 侧）

设计：[backup-design](./backup-design.md）。前置：Docker 主机；异地项需 rclone 与真实远端。

- [ ] 定时调度到点执行一次（daily@HH:mm / every:Nh 任一形态）；停机补跑只补一次——Docker 主机必需，标记为阻塞
- [ ] 打包产物：tar.gz 完整、路径穿越防护（构造带 `../` 文件的目录验证拒绝）、符号链接不跟随——Docker 主机必需，标记为阻塞
- [ ] retention 自动清理到期备份；运行历史与失败 `backup.failed` 通知送达——Docker 主机必需，标记为阻塞
- [ ] 恢复双阶段：独立目录解包绝不覆盖原路径；Zip Slip 构造样例被拒；任务日志可读——Docker 主机必需，标记为阻塞
- [ ] 备份文件下载（分块 RPC 流转发，GB 级不炸内存）——Docker 主机必需，标记为阻塞
- [ ] 异地保留：真实 S3/B2 远端推送成功；断网/错密钥时 `backup.remote-failed` 独立通知且不改任务终态；文件行「补传」成功——异地云凭据不可本机验证，标记为阻塞
- [ ] rclone 网盘限速场景下的超时与错误呈现——Docker 主机+rclone 配置不可本机验证，标记为阻塞
- [ ] GB 级完整链：大目录打包 → 推送 → 换机恢复——Docker 主机+大存储不可本机验证，标记为阻塞
- [ ] 前置命令钩子：`mysqldump` / `pg_dump` 真库导出产物在备份内；钩子失败/超时中止本次（不打包不推送不清理）——Docker 主机+数据库不可本机验证，标记为阻塞

## 面板数据库备份（server 侧）

设计：[server-backup-design](./server-backup-design.md)。

- [ ] 定时触发 `VACUUM INTO`：产物为紧凑完整副本，`sqlite3` 重新打开并抽查表数据
- [ ] retention 按天清理；`0=永久` 形态
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

设计：[proxy-design](./proxy-design.md)。前置：nginx 宿主机裸装一台；Traefik 容器挂载宿主动态目录一台。

- [ ] nginx：新建站点全链（渲染 → `nginx -t` → 写片段 → reload），浏览器可达
- [ ] nginx：语法错误被 `-t` 拦截不落盘；reload 失败回滚后再 reload，站点保持旧配置可用
- [ ] nginx：systemd reload 与无 systemd 环境 `nginx -s reload` fallback 各验一次
- [ ] nginx：80/443 端口冲突场景的错误摘要呈现
- [x] Traefik：动态目录探测（静态配置 `providers.file.directory` 与缺省路径）；站点文件写入后热加载生效
  证据（2026-09-30）：探针 `scripts/acceptance/traefik/`（traefik:v3.5.6 容器挂载宿主目录 + host-gateway 回连双上游）T1/T2/T3/T9——env 覆盖探测（`COCKPIT_TRAEFIK_DIR`，D12 生产形态；静态配置解析分支为纯函数、单测覆盖）capability `traefik-proxy` dynamicDir 正确；新增/修改（upstream A→B）/删除（文件/路由/列表三面摘除）均 watch 热加载即时生效，`reloadMode=hot`；`probe.log`
- [x] Traefik：坏 YAML 自检拒绝不落盘；router→service 引用校验拦截
  证据（2026-09-30）：自检/引用校验为渲染器防御分支（合法参数渲染恒合法，经 REST 不可达），单测覆盖（`TestTraefikProvider_Render` 系）；真机侧 T6 验 extra 拒绝（报错、不落盘、不入列，D14 注入面）+ T7 实测外来坏文件语义——Traefik 冻结整目录热更新（存量 last-good 照常、新变更拒载 404、日志点名坏文件），rm/面板重下发即自动解冻且积压变更一并生效（D13 口径据此修正）；`probe.log`
- [ ] 双后端主机并存时 capability 分流正确（nginx 优先，旧 agent 回退 nginx.* 前缀）
  注（2026-09-30）：本机无 nginx（双后端并存需 nginx 主机，维持挂起）；分流四例（仅 nginx/仅 traefik/双后端/旧 agent 回退）单测覆盖，traefik 单后端真机全链验讫（本次验收全部请求经 `traefik.*` 前缀分发）

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
- [ ] 巡检间隔修改即时生效；`0=关闭` 形态

## DNS 域名管理

设计：[domain-binding-design](./domain-binding-design.md)。前置：DNSPod 账号（免费版优先，暴露真实限制）。

- [ ] DNSPod 免费版 TTL 下限：编辑低于免费下限的 TTL 时 API 真实报错的透传呈现
- [ ] CAA 记录新增/编辑/删除
- [ ] Cloudflare 与 DNSPod 两家 SRV 记录编辑实测（字段映射与回读）

## 组网观测（Overlay）

设计：[overlay-design](./overlay-design.md)。前置：ZeroTier Central / Tailscale 管理 API token；装 ZeroTier/Tailscale/WireGuard/frp 的真实主机若干。

- [ ] 运行态观测：真实 peers/interfaces 快照（wg 确认私钥/预共享密钥不出现）；跨 agent 同节点按最低延迟合并、来源徽标正确
- [ ] frp 分级观测：零配置只出版本+进程；配 admin 地址后隧道计数出现
- [ ] 云端管理：真 token 拉取 ZT 网络/成员与 TS 设备列表；授权/取消授权、除名、删除四类变更端到端生效（云端后台复核）
- [ ] 未纳管设备识别：云端手动加一台不入面板的设备 → 「未纳管」徽标出现
- [ ] agent 身份上报：本机身份 chip 的 ZT node id / TS device id 与云端同键对照 → 「面板纳管」徽标

## 磁盘健康（SMART）

设计：[disk-health-design](./disk-health-design.md)。前置：带物理盘的真实主机（smartctl 可用）。

- [ ] 盘发现与字段读取：passed/温度/重映射/待定扇区/NVMe media_errors 真实数值合理
- [ ] 巡检告警：FAILED 盘 error 级、扇区/介质异常 warning 级、同盘未读期间只报一次
- [ ] （按需）非 root 部署下 sudo 提权读取路径

## 日志检索

设计：[logs-design](./logs-design.md)。前置：多台 agent（含 systemd 主机与 docker 主机）。

- [ ] 尾随真机项：journalctl 高频输出流畅尾随不重不漏
- [ ] docker 容器停止后尾随以 `reason=exited` 正常终止
- [ ] 10 分钟超时兜底生效
- [ ] 跨机联邦检索：多 agent 并行扇出按主机分组返回；offline/无 logs capability/路径不存在三种 skipped 归因正确；单 agent 失败不整体报错

## NAS

设计：[nas-design](./nas-design.md)。前置：DSM 6.x/7.x、TrueNAS CORE/SCALE、OMV 真机各一（或有则验）。

- [ ] DSM 6 与 7 两版本 API 字段差异实测（登录、卷、共享夹）
- [ ] TrueNAS CORE 与 SCALE 字段差异（flexInt 字符串数字形态）
- [ ] OMV：`X-Openmediavault-Sessionid` 头认证全流程；`"1.50 GiB"` 容量字符串解析；2FA 账号等同失败降级
- [ ] mdadm 降级演练：拔盘/标记 fault 后告警送达（真去重）
- [ ] ZFS / SMB / NFS 主机真实枚举
- [ ] `COCKPIT_NAS_TARGETS` 跳板探测：无本地存储工具的主机（Windows/macOS）可作跳板观测

## 服务管理（三后端）

设计：[service-design](./service-design.md)。前置：systemd Linux 主机、Windows 主机、macOS 主机各一。

- [ ] systemd：列表含未加载 unit；start/stop/restart/enable/disable 实测；mask 后隐藏启动并显解屏蔽、unmask 还原
- [ ] systemd：daemon-reload 工具栏按钮；unit 文件查看/编辑（包管文件先复制 `/etc/systemd/system` 覆盖位再改，升级不丢）
- [ ] systemd：服务行「日志」跳转 journalctl 历史可查（含未加载 unit）
- [ ] Windows SCM：列表/启停/自启切换/restart 等待 30s 超时路径实测；无权限服务报错透传；reload 按钮确认隐藏
- [ ] macOS launchd：列表/启停/自启切换；`kickstart` 失败回退 `bootstrap` 路径；无权限报错透传

## 移动端（Flutter，iOS + Android）

设计：[mobile-design](./mobile-design.md)。前置：Android 真机（或模拟器）安装 debug APK（CI `Mobile` workflow artifact 或本地 `flutter build apk --debug`）；server 可达（公网或同网段）。iOS 构建需 macOS + Xcode（本仓库 CI 不构建 iOS，见 D8）。

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

- 每通过一项：勾选本清单 + 在 `todo.md` 对应条目的「剩余」中移除该项，要点补进 ✅ 记录
- 验收发现的缺陷：按既有纪律单独立项修复（一笔一提交，不夹带）
- 全域通过后在本文件顶部标注「已完成（日期）」并归档到 `docs/archive/`（如适用）
