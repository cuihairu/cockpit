# 带外管理（OOB）统一方案评审（外部意见 + 用户裁决，2026-10-20）

> 来源：用户 2026-10-20 粘贴的 ChatGPT 评审（三条架构路线 + 统一 Provider），并入用户当场三条裁决。
> **性质：外部评审意见 + 用户决策记录，机型支持矩阵以「需逐型号确认固件」为准，不作承诺。**
> 相关：竞品全景 `competitor-landscape-2026-10-20.md`（MeshCentral AMT 借鉴项）、多云
> `chatgpt-2026-10-20-multicloud-positioning.md`（云主机电源并入 Provider 线）。仓内已有
> `docs/design/bmc-capabilities.md`(+en) 覆盖 BMC/Redfish 方向，本评审在此之上补 AMT/DASH 与
> 统一层，立项时以 bmc-capabilities 设计为基座合并，不另起炉灶。

## 一、三条技术路线（CPU 架构解耦，统一 Provider）

| 设备类型 | 带外方案 | Cockpit 方向 |
|---|---|---|
| Intel vPro PC | AMT | 首期 Provider |
| AMD Ryzen PRO PC | DASH / AIM-T | 首期 Provider |
| Intel/AMD 服务器 | BMC / Redfish | 首期 Provider（沿 bmc-capabilities 基座） |
| ARM 服务器（Ampere 等） | BMC / Redfish（SBMR 规范） | 同上，Redfish 覆盖 |
| Snapdragon PC | Snapdragon Guardian | 研究适配（逐型号确认 OEM/固件） |
| Apple Silicon Mac | 系统远控 + 厂商工具 | 软件层管理（无固件级 KVM 可依赖） |
| 普通 ARM SBC | SSH / Serial / WOL | 按硬件支持 |
| 任意物理设备 | 外部 IP-KVM | 可选兜底 |
| **任意局域网设备** | **OpenWrt 自举节点** | **用户裁决：第四条兜底线（见四）** |

统一操作面：Power · Inventory · BIOS · Console(带外 KVM) · Boot · Recovery。
能力模型用 **supported / enabled / reachable 三态分离**（硬件能力 ≠ 已启用 ≠ 当前可达），
布尔仅作接口示意，实际状态由检测与验证产生。

AMD 侧优先研究 **DASH CLI + AMD Manageability API** 集成而非自研底层协议
（官方入口 AMD Manageability Tools）；Intel 侧走 AMT 标准接口；服务器侧 Redfish。
参考项目：OpenBMC（ARM 服务器/Redfish 体系的开放软件栈）。

## 二、支持的服务器/电脑矩阵（用户要求列出；标注确认级别）

| 线路 | 可列范围 | 确认级别 |
|---|---|---|
| Intel AMT | 商用 vPro 机型（戴尔 OptiPlex/ThinkCentre/HP Elite 世代线等） | 型号+固件+Provisioning 三查；入支持表前必须实机验证 |
| AMD DASH/AIM-T | Ryzen PRO 商用平台（2026 PRO 平台含高分辨率 KVM/远程 Windows 恢复/远程擦除） | 同上；主板固件支持是前提 |
| 服务器 Redfish | 支持 Redfish 的 x86/ARM 服务器（含 Ampere Altra + BMC） | Redfish 语义版本与厂商扩展需逐家验证 |
| Snapdragon Guardian | Snapdragon X 系列商用平台 | 仅「研究适配」档，不得写进支持承诺 |
| Apple Silicon | 无 AMT/DASH 等价物 | 软件层管理，不列带外支持 |

**支持矩阵落地方式**：立项后建 `docs/reference/oob-support-matrix.md`（中英），
每行=机型/固件版本/Provider/实测状态（supported/enabled/reachable 三列），
**未实测不入表**（避免把「厂商宣称」当「已验证」）。

## 三、统一 Provider 架构（评审方案，采纳）

```
Cockpit — Out-of-Band Management
  设备发现 · 能力协商 · 权限控制 · 审计
  ├── Intel AMT Provider        （vPro 终端）
  ├── AMD DASH Provider         （Ryzen PRO 终端）
  ├── Redfish/IPMI Provider     （x86/ARM 服务器）
  ├── Vendor / IP-KVM Provider  （其他设备）
  └── OpenWrt Bootstrap Node    （局域网兜底，见四）
统一操作：Power · Inventory · BIOS · Console · Boot · Recovery
```

检测与 CPU 架构解耦：设备声明 `oob.providers[]` + `oob.capabilities{}`，值由检测产生。

## 四、OpenWrt 自举节点（用户裁决 2026-10-20，架构关键件）

> 用户原话要点：局域网中一台支持自启动的 OpenWrt 很关键——**不仅仅是网关，还是探活
> 和最后远程控制的一道手段。**

定位三层：

1. **网关**（既有）：入站口、端口转发、出口 IP。
2. **探活层**：对全网段做 ARP/ICMP/TCP 独立探测——即使目标机 agent 挂了、OS 死了，
   网络层可达性与 MAC 层在位判断仍成立（与 agent 心跳互为交叉验证）。
3. **最后兜底控制**：WOL 唤醒、端口转发到 IP-KVM/串口服务器、（有 AMT/DASH 的机型）
   转发带外管理流量——Server 不可达时局域网内仍保有一条控制路径。

「自启动」要求：节点断电恢复/看门狗超时后自动回到服务态（watchdog + 自动拉起配置，
不依赖人工登录恢复）。与既有 `internal/openwrt`（ubus RPC 客户端：系统信息/接口/路由/
防火墙区，client.go:245-381）衔接，新增的是探测调度、WOL 动作面与兜底转发编排。

## 核验结论（2026-10-20 实查）

| 论断 | 结果 |
|---|---|
| 仓内无 OOB 实现 | ✅ AMT/DASH/Redfish 无 go 代码；仅 docs/design/bmc-capabilities.md(+en) 与调研文档提及 IPMI/Redfish |
| internal/openwrt 已有客户端 | ✅ client.go（GetSystemInfo/ListInterfaces/ListRoutes/GetFirewallZones，ubus RPC）——OpenWrt 节点线可直接衔接 |
| OpenBMC 无涉猎 | ✅ 0 文件，仅本评审提及作参考 |

## 落地跟踪

> P0 已拍板开工（2026-10-20，todo.md:81）；落地即勾选 + 测试证据。

- [x] P0 OOB 统一 Provider 框架（能力三态 supported/enabled/reachable + 发现与协商；并入 internal/cloud 同款拆分思路）〔已落 42241d4：`internal/oob`（六能力/三态+Usable/秩仲裁/Registry+并发 ProbeAll/Merge 来源可溯），包测试 10 项绿 + 包覆盖率 100%〕
- [x] P0 Redfish Provider（服务器线，基于 bmc-capabilities 设计基座）〔已落：`internal/oob/redfish` 只读发现探测（service root 门禁 + System/Manager/Bios 三态核实，缺证据能力不猜、401 如实报「可达未定」），httptest 契约测试 13 项绿 + 包覆盖率 100%；边界=凭据面（vault→Basic/Session）与电源/传感器操作面随 bmc-capabilities §7 BMC-B3/B4 片落位，本片不自研 quirk〕
- [x] P0 Intel AMT Provider（vPro 商用机）〔已落（发现片）：`internal/oob/wsman`（WS-Management Identify 1.0 免认证共用层，DASH 同源）+ `internal/oob/amt`（无 scheme 默认 http、无 port 默认 :16992、产品匹配拒错认 DASH/通用 WBEM、401 如实报可达未定）；httptest 契约 21 用例绿 + 两包覆盖率 100%；Identify 只证固件在位+可达，**不报能力**——启用态（Provisioning）与 power/KVM/SOL 能力面需认证 wsman，随凭据片经 vault 落位〕
- [ ] P0 AMD DASH Provider（优先 DASH CLI + Manageability API 集成）
- [ ] P0 OpenWrt 自举节点：探活层 + WOL + 兜底转发编排 + 自启动可靠性（衔接 internal/openwrt）〔设计已落 docs/design/oob-openwrt-bootstrap.md(+en)：探活三源独立三态判定、WOL/兜底转发动作面（双确认+审计）、procd 自启动、凭据 vault 引用；分片 A-D 待排〕
- [ ] P1 支持机型矩阵文档（每行三态实测，未实测不入表）
- [ ] P1 Snapdragon Guardian 研究适配（仅研究档）
- [ ] IP-KVM 外置兜底 Provider（按需）
