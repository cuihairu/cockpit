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
