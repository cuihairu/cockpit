---
title: BMC/iBMC 带外管理能力——Redfish 为主 IPMI 兜底（能力清单+厂商 quirk 表+安全红线）
---

# BMC/iBMC 带外管理能力设计

## 状态

- 状态: **Active（用户令 2026-10-10：封装常见 BMC/iBMS 系统能力，文档先行零代码）**。
- 定位：服务器**带外**（out-of-band）管理面——agent 经管理网直连 BMC 控制器（独立于主机 OS，
  关机/宕机仍可管），把「带内 agent + 带外 BMC」凑成完整服务器管理面。
- 与既有批次的关系：探测从 agent 侧发起（黑盒口径，同服务检测 agent）；电源破坏性操作的
  execlog 记录依赖 P5 execlog（**挂起待拍板**）——MVP 先落既有审计面，P5 落地后自动获得。
- 本批零代码，实施批次见 §7。

## 1. 协议层：标准优先

| | Redfish（主） | IPMI 2.0 LAN（兜底） |
| --- | --- | --- |
| 标准 | DMTF Redfish（REST+JSON over HTTPS） | IPMI 2.0 RMCP+（UDP 623） |
| 覆盖 | 2015 年后主流服务器（iDRAC 8/9/10、iLO 4/5/6、iBMC V3+…） | 更老机器、或 Redfish 被关闭的 BMC |
| 认证 | Basic 或 Session（/SessionService/Sessions，标准） | RMCP+ 用户名口令（cipher suite 协商，**禁 cipher 0**） |
| 传输 | HTTP/TLS 复用现有设施 | UDP 自管会话（库封装） |

**双栈策略**：per-target 配置 `protocol: redfish|ipmi`（显式指定不探测）；缺省 `auto`——先 GET
`/redfish/v1` service root（2s 超时），200 且 JSON 即 Redfish，否则回退 IPMI。两协议同一组
能力接口（§2），差异收敛在 provider 内部（§4）。

## 2. 能力清单（按常用排序）

| # | 能力 | Redfish 标准 | IPMI 兜底 | 批次 |
| --- | --- | --- | --- | --- |
| C1 | 电源状态 | `ComputerSystem.PowerState`（On/Off/PoweringOn…） | `chassis status` | B2 |
| C2 | 电源控制：开/关/优雅重启/强制重启 | `Actions/#ComputerSystem.Reset`，`ResetType` ∈ On/ForceOff/GracefulRestart/ForceRestart | `chassis power on\|off\|soft\|cycle` | B4（破坏性） |
| C3 | 传感器：温度/风扇/电压/功耗 | `Chassis` 下 `Thermal`/`Power`（旧）或 `ThermalSubsystem`/`PowerSubsystem`（Redfish 1.9+） | SDR + sensor reading 流式 | B3 |
| C4 | 硬件健康 rollup | `Status.Health`（System/Memory/Processor/Chassis） | sensor 阈值状态聚合 | B3 |
| C5 | SEL 事件日志（查询/按时间过滤/清空） | `Managers/{id}/LogServices/SEL/Entries` | `sel list/clear` | B5（清空破坏性） |
| C6 | BMC/固件/序列号信息 | `Manager`、`SoftwareInventory`、`SerialNumber/SKU` | `mc info`/device id | B2 |
| C7 | SOL 串口 | OEM 为主（SerialInterface 少有实现） | **IPMI SOL**（activate/deactivate 流） | B6 |
| C8 | KVM 入口/虚拟媒体 | **只留链接不做实现**：按厂商规则拼 HTTPS URL 返回前端跳转 | — | B7（链接） |

只读面（C1/C3/C4/C6）先行；破坏性面（C2、C5 清空）后置且同走 §5 红线。

## 3. 发起侧与落位

- **agent 侧 provider**（同 nginx/traefik/stacks 先例）：agent 经管理网可达 BMC；server 不直连。
  agent 配置 BMC 目标清单（地址/凭据引用/协议），capability `bmc-redfish`/`bmc-ipmi` 进注册
  metadata；server RPC 方法 `bmc.*` 转发（server REST `/api/agents/{id}/bmc/*` 纯转发不落库，
  同 proxy 先例——agent 侧为事实源）。
- provider 接口与通用 Redfish 同一套（能力键即方法面）：`bmc.power.status / bmc.power.set /
  bmc.sensors / bmc.health / bmc.sel.list / bmc.sel.clear / bmc.sol.open / bmc.info / bmc.console.link`。
- 三层纪律：协议客户端适配（gofish/go-ipmi 封装）进 core（如 `core/bmc`，纯客户端零业务）；
  provider 业务编排进 `internal/agent`（插件位）；quirk 表是数据不是代码（见 §4）。

## 4. 厂商 quirk 表（同能力不同属性路径）

运行时按 service root `Oem` 段厂商标识自动选 quirk（`Oem.Hpe`/`Oem.Dell`/`Oem.Huawei`）；
未知厂商走纯标准（quirk 空），quirk 缺失的能力报「不支持」不猜。

**quirk 层不自研**：顶层封装 bmclib/v2 内置多厂商 driver（探测型号自动回退，§6），quirk 落位
**优先复用其 driver**；下表是能力映射参考与 bmclib 未覆盖处的自补表兜底。

| 能力 | Redfish 标准（DMTF） | Dell iDRAC 9/10 | HPE iLO 5/6 | 华为 iBMC |
| --- | --- | --- | --- | --- |
| 温度/风扇 | `Chassis/{id}/Thermal`（旧）·`ThermalSubsystem/ThermalMetrics`（1.9+） | 标准 Thermal 可用；精确整机功耗走 OEM `/Oem/Dell/DellPower...` | OEM 扩展风扇/温度在 `/Oem/Hpe`（Thermal 部分字段缺） | 标准 Thermal 可用 |
| 功耗 | `Chassis/{id}/Power` 或 `PowerSubsystem/PowerMetrics` | OEM PowerMonitoring（瓦级精度） | `/Oem/Hpe` power 段 | 标准 Power + OEM 加速值 |
| SEL | `LogServices/SEL/Entries` | SEL + **LC 日志**（`LogServices/Lclog`，更全）并存 | IEL（Integrated Event Log，即标准 LogService，名不同） | 标准 SEL |
| 固件清单 | `UpdateService/SoftwareInventory`（FirmwareInventory 已废） | OEM FirmwareInventory 仍挂 UpdateService 下 | OEM `/Oem/Hpe/UpdateService` | 标准 SoftwareInventory |
| KVM 链接（C8 只拼链接） | 无标准 | `https://{bmc}/console`（HTML5） | `https://{bmc}/html5`（iLO5）/`/console`（iLO6） | `https://{bmc}/#ovm`（HTML5） |
| SOL | OEM 为主 | IPMI SOL | IPMI SOL（VirtualSerialPort OEM 另有） | IPMI SOL |
| 会话认证 | `/SessionService/Sessions` 标准注销头 `X-Auth-Token` | 标准 | 标准 | 标准 |

quirk 实现形态：**每能力一条覆盖记录**（URI 模板 / 字段名 / 结果投影函数键），provider 按
（能力键, 厂商）查表回退标准路径——quirk 表数据驱动，不写 per-厂商 if/else 链。

## 5. 安全红线

1. **凭据不落明文**：BMC 凭据存 server 凭据库（复用 remote credential vault 同款 AES-GCM
   落库模型，`protocol` 扩 `bmc` 类型）；每次操作时经加密 WS 下发到 agent（同 SSH 代理口令/
   私钥先例），agent 内存即用即弃，不落盘、不进日志与审计明文。
2. **TLS 默认校验**：Redfish 证书校验默认开；per-target 显式 `skip_verify: true` 仅限内网
   自签（web 配置处红字警示）；IPMI 强制 RMCP+（cipher suite ≥1，禁 cipher 0 明文）。
3. **破坏性操作双确认+审计**：C2 电源类与 C5 清空——web 二次确认（输入目标名 confirm，同
   stack 删除先例）+ 审计事件 `bmc.power.{on,off,restart}` / `bmc.sel.clear`（记 agent/target/
   动作/操作者，不含凭据）+ execlog 留痕（依赖 P5，MVP 审计先行）。
4. IPMI 账号最小权限（文档建议：探测专用角色仅 Operator；Admin 仅电源操作需要）。

## 6. 依赖选型（调研定案 2026-10-10：直接引用成熟库，不自研）

| 层 | 选型 | 理由 |
| --- | --- | --- |
| **顶层封装** | **`github.com/bmc-toolbox/bmclib/v2`** | 多厂商 driver 抽象（Redfish/IPMI/SSH/厂商 API），探测型号自动回退——**厂商 quirk 层已内置**（§4 落位优先复用，缺位再自补表）；power/boot device/inventory/SEL/firmware 等能力面现成 |
| Redfish 协议客户端 | **`github.com/stmcginnis/gofish`**（BSD-3） | DMTF 标准，45+ 下游；bmclib 底层同款，直连模式（无 bmclib driver 的 OEM 路径）用它 |
| 裸 IPMI（需要时） | **`github.com/bougou/go-ipmi`** | 纯 Go 免 ipmitool 二进制；remote(RMCP+)/in-band 双模，自带参考 BMC server 可 CI 起桩；SOL/SEL/sensor 全。bmclib 未覆盖的 SOL（C7）直用它 |

**注意**：bmclib 的 **SSH driver 默认关闭、按需启用**——它收的是主机 SSH 凭据（非 BMC 凭据），
凭据面另开口子，缺省不启用；确需厂商 SSH API 时单 target 显式开。
封装薄层 = provider 插件位 + 凭据/TLS/审计红线（§5）；quirk/多厂商回退不自研，升级跟随上游。
（备选留档：u-root ipmi 面向 in-band OpenIPMI 驱动不适合远端 LAN；shell 调 ipmitool 引入外部
二进制依赖——均弃。）

## 7. 批次计划（小步：实现+测试绿+英文 commit）

| 批 | 内容 | 门禁 |
| --- | --- | --- |
| BMC-B1 | 本简档 | docs |
| BMC-B2 | provider 骨架 + capability 探测 + C1/C6 只读（gofish/go-ipmi 引入 + 参考桩测试） | Go 门禁 |
| BMC-B3 | C3 传感器 + C4 健康（标准+quirk 路径表驱动） | Go 门禁 |
| BMC-B4 | C2 电源控制 + 双确认 + 审计 + 凭据库下发 | Go 门禁 + web 二次确认 |
| BMC-B5 | C5 SEL 查询/清空 | Go 门禁 |
| BMC-B6 | C7 SOL（IPMI 流桥接 web 终端） | Go 门禁 + 手工验收 |
| BMC-B7 | C8 KVM/虚拟媒体链接 | Go 门禁 |

## 8. 边界（诚实清单）

- 不绑厂商；未知 OEM 走纯标准，quirk 缺失能力明确报「不支持」。
- 虚拟媒体/KVM 只留链接不做实现；固件升级（推固件包）**不在清单**——高危另议。
- server 直连 BMC 的形态不做（agent 侧发起，管理网可达性由部署保证）；多探针点位归 croupier 口径另议。
- IPMI 老 BMC 的 SOL/SEL 功能面以 go-ipmi 实测覆盖为准（参考桩 ≠ 真机）；真机验收另批。
- execlog 依赖 P5（挂起待拍板）——MVP 审计面先行，P5 落地后电源操作自动补执行留痕。
- 与服务检测 agent（探活黑盒）语义不重叠：该面管「服务是否可达」，本面管「硬件与电源」。
