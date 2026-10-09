---
title: 虚拟化平台能力——hypervisor provider 层（PVE/vSphere/本机 QEMU-KVM 统一虚机生命周期）
---

# 虚拟化平台能力设计

## 状态

- 状态: **Active（用户令 2026-10-10 设计补单2：与 BMC 层同构进插件机制）**。
- 定位：hypervisor provider 层——把 PVE / vSphere / 本机 QEMU-KVM 统一成**同一组虚机生命周期
  接口**，与 [BMC 带外管理](bmc-capabilities.md) 共用同一套 provider 机制（capability 探测 +
  RPC 转发 + 凭据下发 + 审计红线），差异只在平台客户端适配。
- 与 BMC 层的关系：**带内 vs 带外互补**——BMC 管硬件/电源（主机关机也能管），本层管虚机生命周期
  （平台在线时）；面板上分区呈现，语义不重叠（同服务检测 agent 的「服务是否可达」也不重叠）。
- 本批零代码，实施批次见 §5。

## 1. 平台与选型（调研定案 2026-10-10：直接引用成熟库，不自研）

| 平台 | 选型 | 说明 |
| --- | --- | --- |
| PVE | **`github.com/bpg/proxmox-api`**（MPL-2.0，活跃） | bpg/terraform-provider-proxmox 2.3k★ 家族抽的独立客户端，REST+SSH；备选 Telmate/proxmox-api-go（老牌，wiki 官列） |
| vSphere | **`github.com/vmware/govmomi`**（Apache-2，~2.6k★ 官方级） | 带 govc CLI + **vcsim 模拟器——CI 用 vcsim 免真机** |
| 本机 QEMU/KVM | **预留 libvirt Go bindings 位** | 不选型定案，只留位（agent 与 hypervisor 同机时 in-band 直连） |

## 2. 统一虚机生命周期接口（平台无关）

| 能力 | 接口键 | 破坏性 |
| --- | --- | --- |
| 列表 | `vm.list` | 否 |
| 详情 | `vm.get` | 否 |
| 创建 | `vm.create` | 否（占资源，审计） |
| 启停 | `vm.start` / `vm.stop` / `vm.reboot` | stop/reboot 破坏性（审计） |
| 克隆 | `vm.clone` | 破坏性（审计） |
| 删除 | `vm.delete` | **高危（双确认+审计）** |
| 控制台入口 | `vm.console` | 否（链接/代理面，本批留位） |
| 资源用量 | `vm.metrics` | 否（接现有 metrics 面） |
| 平台/节点 | `node.list` / `platform.info` | 否 |

## 3. 落位（与 BMC 同构的 provider 机制）

- **agent 侧 provider**（同 nginx/traefik/stacks/BMC 先例）：capability `virt-pve`/`virt-vsphere`/
  `virt-libvirt` 进注册 metadata；server RPC 方法 `virt.*` 转发，REST `/api/agents/{id}/virt/*`
  纯转发不落库（agent 侧为事实源，同 proxy 先例）。
- 平台客户端封在 core（如 `core/virt`，纯客户端零业务）；provider 编排进 `internal/agent`
  （插件位）；三平台同一 provider 接口，平台差异在客户端适配层。
- 凭据：PVE 推荐 **API token**（PVEVMUser 角色最小权限，优于口令）；vSphere service account；
  经加密 WS 下发即用即弃（同 BMC §5 红线）。
- CI 策略：vSphere=vcsim 免真机；PVE=接口桩（bpg/proxmox-api 无模拟器）；真机验收另批。

## 4. 安全红线（与 BMC 同款）

1. **凭据不落明文**：存 server 凭据库（remote credential vault 同款 AES-GCM 模型，扩 `virt` 类型）；
   每次操作经加密 WS 下发，不落 agent 盘、不进日志/审计明文。
2. **TLS 默认校验**：证书校验默认开；PVE 自签常见，per-target 显式 `skip_verify` 仅限内网自签
   （web 红字警示）。
3. **破坏性操作双确认+审计**：`vm.delete`/`vm.stop`(强制)/`vm.clone`——web 二次确认（输入目标名
   confirm，同 stack 删除先例）+ 审计事件 `virt.vm.{delete,stop,clone,create}`（记 agent/平台/目标/
   动作/操作者，不含凭据）+ execlog 留痕（依赖 P5 execlog 挂起项——MVP 先落既有审计面）。
4. **最小权限分权**：PVE API token 用 PVEVMUser 角色；vSphere 只读角色与虚机管理角色分开。

## 5. 批次计划（小步：实现+测试绿+英文 commit）

| 批 | 内容 | 门禁 |
| --- | --- | --- |
| H-B1 | 本简档 | docs |
| H-B2 | PVE provider（list/get/start-stop + 接口桩测试） | Go 门禁 |
| H-B3 | vSphere provider（vcsim CI 全链） | Go 门禁 |
| H-B4 | 创建/克隆/删除 + 双确认 + 审计 + 凭据库下发 | Go 门禁 + web 二次确认 |
| H-B5 | vm.metrics 接现有资源面板 | web 全量门禁 TZ=UTC |
| H-B6 | vm.console 控制台入口 | Go 门禁 + 手工验收 |
| H-B7 | libvirt 预留位接线 | Go 门禁 |

## 6. 边界（诚实清单）

- 不做存储/网络编排（PVE SDN、vSphere DVS）——虚机生命周期为界。
- 快照管理、迁移（HA/DRS）留位；固件/硬件面归 BMC 层。
- KVM 直连形态仅 libvirt 位预留，不选型不定案。
- 与 stack-deploy（docker compose 容器栈）不重叠：那管容器，本层管虚机。
- 真机验收（PVE 集群/vCenter）另批；CI 以 vcsim + 接口桩为准。
