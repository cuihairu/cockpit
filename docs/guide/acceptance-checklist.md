# 真机验收清单

> 各功能开发交付时的自动化验证边界：Go 单测（含 `-race`）、web vitest 全绿、
> 本机端到端脚本（`scripts/e2e-smoke.sh`、`scripts/local-acceptance/`）通过。
> 以下场景依赖**真实环境**——真机、真凭据、真网络、真版本——无法离线自动化，
> 逐功能散记于 `todo.md` 各条目的「剩余」中。本文档将其汇总为一张可勾选清单，
> 拿到测试机或凭据后按域推进即可，验收一项回填一处。

## 通用前置

- [ ] server 启动，至少一台 Linux agent 注册在线（`/agents` 页绿标）
- [ ] `scripts/e2e-smoke.sh` 冒烟通过（server→agent→inventory 同步→`/api/resources` 闭环）
- [ ] 至少配置一个通知渠道（webhook/ntfy 等），验收告警类功能时用「测试通知」按钮核对送达
- [ ] 习惯性核对：变更类操作在「审计日志」页有留痕、敏感字段（证书内容、密钥）不出现在审计 detail

## 应用部署（Stacks）

设计：[stack-deploy-design](./stack-deploy-design.md)。前置：一台真实 Docker 主机（agent 在线、docker 可用）。

- [ ] 新建 stack 粘贴/上传 compose.yml → 部署：镜像真实拉取、容器进入健康状态、状态总览刷新
- [ ] compose 完整语义：环境变量、volumes、自定义网络、depends_on 启动顺序
- [ ] restart / pull 动作真实生效；部署历史时间线与部署日志可对应
- [ ] 模板库一键新建与 import 路径
- [ ] stacks 目录不存在/无权限时列表页目录自检告警的呈现

## 备份与恢复（agent 侧）

设计：[backup-design](./backup-design.md)。前置：Docker 主机；异地项需 rclone 与真实远端。

- [ ] 定时调度到点执行一次（daily@HH:mm / every:Nh 任一形态）；停机补跑只补一次
- [ ] 打包产物：tar.gz 完整、路径穿越防护（构造带 `../` 文件的目录验证拒绝）、符号链接不跟随
- [ ] retention 自动清理到期备份；运行历史与失败 `backup.failed` 通知送达
- [ ] 恢复双阶段：独立目录解包绝不覆盖原路径；Zip Slip 构造样例被拒；任务日志可读
- [ ] 备份文件下载（分块 RPC 流转发，GB 级不炸内存）
- [ ] 异地保留：真实 S3/B2 远端推送成功；断网/错密钥时 `backup.remote-failed` 独立通知且不改任务终态；文件行「补传」成功
- [ ] rclone 网盘限速场景下的超时与错误呈现
- [ ] GB 级完整链：大目录打包 → 推送 → 换机恢复
- [ ] 前置命令钩子：`mysqldump` / `pg_dump` 真库导出产物在备份内；钩子失败/超时中止本次（不打包不推送不清理）

## 面板数据库备份（server 侧）

设计：[server-backup-design](./server-backup-design.md)。

- [ ] 定时触发 `VACUUM INTO`：产物为紧凑完整副本，`sqlite3` 重新打开并抽查表数据
- [ ] retention 按天清理；`0=永久` 形态
- [ ] rclone 真远端（gdrive/S3）推送；失败只记 `[remote]` 日志 + `server_backup.remote-failed` 通知，本地备份不受影响
- [ ] 「补推」按钮对历史文件生效

## 远程会话录制

设计：[recording-design](./recording-design.md)。

- [ ] 终端/桌面会话录制归档后异步推送 rclone 真远端成功
- [ ] 推送失败发 `recording.remote-failed` 通知，本地档保留、retention 窗口内「补推」成功

## 反向代理

设计：[proxy-design](./proxy-design.md)。前置：nginx 宿主机裸装一台；Traefik 容器挂载宿主动态目录一台。

- [ ] nginx：新建站点全链（渲染 → `nginx -t` → 写片段 → reload），浏览器可达
- [ ] nginx：语法错误被 `-t` 拦截不落盘；reload 失败回滚后再 reload，站点保持旧配置可用
- [ ] nginx：systemd reload 与无 systemd 环境 `nginx -s reload` fallback 各验一次
- [ ] nginx：80/443 端口冲突场景的错误摘要呈现
- [ ] Traefik：动态目录探测（静态配置 `providers.file.directory` 与缺省路径）；站点文件写入后热加载生效
- [ ] Traefik：坏 YAML 自检拒绝不落盘；router→service 引用校验拦截
- [ ] 双后端主机并存时 capability 分流正确（nginx 优先，旧 agent 回退 nginx.* 前缀）

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

## 验收后回填约定

- 每通过一项：勾选本清单 + 在 `todo.md` 对应条目的「剩余」中移除该项，要点补进 ✅ 记录
- 验收发现的缺陷：按既有纪律单独立项修复（一笔一提交，不夹带）
- 全域通过后在本文件顶部标注「已完成（日期）」并归档到 `docs/archive/`（如适用）
