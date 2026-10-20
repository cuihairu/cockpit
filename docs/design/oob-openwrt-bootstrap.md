---
title: OOB 第四线——OpenWrt 自举节点（探活层+WOL+兜底转发+自启动可靠性）
---

# OpenWrt 自举节点设计

> 语言：**中文** ｜ [English](./oob-openwrt-bootstrap.en.md)

## 状态

- 状态: **Active（设计落地，实现分片待排；P0，todo.md:81 OOB 第四线，用户裁决 2026-10-20）**。
- 来源: `docs/review/oob-management-2026-10-20.md` §四（用户裁决：局域网自启动 OpenWrt =
  网关 + 探活 + 最后兜底控制的一道手段，**必须自启动可靠**）。
- 衔接: 既有 `internal/openwrt`（ubus RPC 客户端：系统信息/接口/路由/防火墙区与转发读，client.go:245-497）。

## 1. 定位三层（与既有面的边界）

| 层 | 内容 | 与既有面的边界 |
| --- | --- | --- |
| 网关（既有） | 入站口/端口转发/出口 IP 观测 | 已由 overlay 与 openwrt 只读观测覆盖，本设计不动 |
| 探活层（新） | 对配置网段做 ARP/ICMP/TCP **独立**探测：目标机 agent 挂了、OS 死了，网络层可达与 MAC 在位判断仍成立；与 agent 心跳互为交叉验证 | healthprobe（服务检测 agent）管「服务是否可达」，本面管「网络层在位」——语义不重叠，报文与判定独立实现，不共用探测器 |
| 最后兜底控制（新） | WOL 唤醒、（OOB 机型）带外流量转发、IP-KVM/串口转发；Server 不可达时局域网内仍保有一条控制路径 | 转发写操作只管 OOB 兜底场景的 redirect 增删；**主机防火墙规则写不在本设计**（todo 已裁决搁置，先只读验收闭环） |

「自启动」要求：节点断电恢复/看门狗超时后自动回到服务态（watchdog + 自动拉起，不依赖人工登录恢复）。

## 2. 落位与形态

- **探活层与 WOL/编排动作跑在 agent 侧**（部署于 OpenWrt 节点的 cockpit-agent，
  对应 nightly 既有 openwrt 工件线）：本地直连网段，探测与动作不绕 server；
  server 经既有 agent RPC 面下发动作、回收观测（server 不直连 OpenWrt，同 BMC 面纪律）。
- 探测器三源各自独立失败：ARP（邻居表/主动解析）、ICMP、TCP（可配端口列表）；
  判定输出「在位/不在位/未知」三态而非布尔（与 OOB 三态同纪律：探测不到 ≠ 不存在）。

## 3. 动作面

| 动作 | 实现 | 纪律 |
| --- | --- | --- |
| WOL | 魔术包构造（纯函数）+ UDP 广播发送（conn 注入点，测试零真发） | 审计事件记目标 MAC/网段，动作幂等 |
| 兜底转发编排 | ubus firewall redirect 写扩展（既有客户端只有读；写经 uci commit 等效 ubus 调用） | **双确认 + 审计**（同 bmc-capabilities §5 电源类口径）：server 侧二次确认，agent 侧只执行带目标名的指令；规则带 TTL/手动拆除，不得静默常驻 |
| 带外流量转发 | 同上（AMT/DASH 机型的 16992/16993 端口转发属 redirect 子集） | 同上 |

## 4. 凭据面

OpenWrt ubus RPC 需登录：用户名/口令经 vault `${VAR}` 引用配置，运行时解析后由
server 经加密 WS 下发 agent（同 SSH 代理口令先例），agent 内存即用即弃，不落盘、
不进日志与审计明文（同 bmc-capabilities §5.1 口径）。

## 5. 自启动可靠性

- OpenWrt 侧 cockpit-agent 以 **procd init 脚本**随 boot 自启（/etc/init.d/），
  `respawn` 看门狗参数显式配置（阈值/重试/冷却），断电恢复不依赖人工登录。
- 安装接线进 packaging/openwrt 与 nightly 工件线（init 脚本随工件下发）。

## 6. 分片计划（小步：实现+测试绿+英文 commit）

| 片 | 内容 | 门禁 |
| --- | --- | --- |
| 片A（**已落 2026-10-20**，hash 见 todo.md:81） | WOL 动作面：魔术包纯函数 + UDP 发送注入点 + agent RPC 方法 + server 转发端点〔已落 f9210ab：internal/oob/wol 魔术包/MAC 解析/网段广播/发送注入点 + agent `oob.wol` RPC（显式 ip:port 优先，缺省按首个可广播非回环网段推导广播地址）+ POST /api/oob/wol（RBAC 归 inventory 写门禁，审计记目标 MAC 与实际发送目标）；剩余=片B 探活层〕 | Go 门禁 ✅（全量短测+块覆盖门+全量 -race 绿） |
| 片B（探活动作面**已落 2026-10-20**，hash 见 todo.md:81；剩余=片B-2 调度+推送上报） | 探活层：ARP/ICMP/TCP 三源探测器（各自注入点）+ 调度 + 三态判定上报〔探活动作面已落：internal/oob/liveness 三源独立三态判定（present/unknown；absent 保留——探测不到 ≠ 不存在；ARP 读邻居表注入点、ping 注入点、TCP dial 注入点，包覆盖 100%，测试零真发报文，ICMP 回环打点容忍机器环境）+ agent `oob.probe` RPC（ip 必需，ports/timeout 秒可选）+ POST /api/oob/probe 转发端点（oob capability 门禁、RBAC 归 inventory 写、审计 oob_probe 记目标 IP 与端口清单，判定结果不落审计）；剩余=调度循环 + 推送上报（循 protocol 消息先例）〕 | Go 门禁 ✅（探活动作面） |
| 片C | 兜底转发编排：ubus firewall 写扩展 + 双确认 + 审计 | Go 门禁 + web 确认 |
| 片D | 自启动：procd init 脚本 + respawn 配置 + 工件接线 | Go 门禁 + 真机验收 |

真机验收（OpenWrt 设备到位后）：断电恢复自启动、agent 全挂时探活仍报在位、
WOL 唤醒目标机、转发兜底路径可达；未实测不入支持矩阵。

## 7. 边界（诚实清单）

- 不做 mesh 组网数据面（meshlet 边界，ADR-013）；本节点只读供给本机基础信息。
- 不做主机防火墙规则写（另一条目已裁决搁置）。
- 探活不宣称「服务健康」：只有「网络层在位」，服务面归 healthprobe。
- 转发编排不承诺持久 NAT 质量（高并发吞吐归网关本体）；TTL 内兜底可用即达标。
