---
title: P0 云厂商发现/生命周期接线缺口清单（项 20 已执行盘点）
status: inventory (2026-10-30, 已代决 docs/review/2026-10-30-decisions.md 项 20)
---

# P0 云厂商发现/生命周期 API 与界面接线缺口清单（项 20）

> 依据：决策项 20 立即执行动作「再盘点厂商发现、生命周期 API 与界面仍缺的接线」。
> 状态：**盘点完成**（无需凭据，本仓只读核对，commit 73c0a0b）。真实账号契约核对与生命周期闭环需用户提供 AccessKey（待用户）。
> 前置：`docs/design/cloud-provider.md`（P0 Slice 1 设计，接口/能力声明/Operation 状态机已定）；`internal/server/api_cloud.go`（当前实例路由只读数据库台账）。

## 1. 现状盘点（已核实）

| 层 | 现状 | 缺口 |
|---|---|---|
| 厂商适配器 | `internal/cloud/provider.go:19-24` 已定义 `ListInstances/CreateInstance/StartInstance/StopInstance/DeleteInstance/GetOperation`；aliyun `aliyun.go:241/295`、tencent `tencent.go:258/326` 均已实现 Compute 全链 | 无（适配器契约齐备） |
| server 实例路由 | `internal/server/api_cloud.go:121` `handleCloudInstances` 只读数据库台账，**不调用厂商发现 API** | ① 实例发现：未接厂商 `ListInstances` 全量同步 |
| server 账号路由 | `api_cloud.go:165/227` 账号 CRUD＋secret 写入（`writeCloudSecret:282`，经 vault） | ② 生命周期：server 未暴露 create/start/stop/delete HTTP 路由 |
| 生命周期接口 | `internal/cloud/operation.go:52` `MarkSucceeded`（create 场景）被 provider 内部消费 | `operation.go` 仅 provider 内部用，server 无路由 |
| UI | `web/src/pages/` 无 cloud/instance/provider 页面；`web/src/api/` 无客户端封装 | ③ 界面：无实例列表/生命周期/成本页面 |

## 2. 缺口明细（接线待实现，需真实账号验证后）

1. **实例发现**：server 增厂商发现同步（调 `ComputeProvider.ListInstances` 拉全量→`cloud_instances` 台账），替代「只读数据库列表」——不把只读台账当自动发现完成（决策明确不做）。
2. **生命周期路由**：server 暴露 `POST /api/cloud/instances/{id}/{start|stop|delete}` 与创建，接 `Operation` 状态机（`pending→running→succeeded|failed`），写动作经 `ComputeProvider`（单写入负责人，见 `p1-resource-ownership.md`）。
3. **UI**：web 增云实例列表＋生命周期操作＋成本展示（接 `CostRecord`/`BillingProvider` 类型）。
4. **Quota 契约**：`QuotaProvider.QueryQuotas`（`cloud-provider.md:43-45`）未接真实账号核对——**不凭空实现**（决策明确），随真实账号验收解锁。

## 3. 待用户项

- **两家测试子账号、区域、规格、费用硬上限、清理负责人**：用户提供（决策项 20 立即执行动作）。
- **AccessKey 经 vault 配置**：用户提供，`${VAR}` 引用形态（cloud-provider.md §6 红线，绝不明文）。
- **Quota 契约核对**：真实账号响应验证。

## 4. 决策对照

本清单落实决策项 20 的「盘点厂商发现、生命周期 API 与界面仍缺的接线」；接线实现与真实契约核对均待用户提供凭据后推进（决策「凭据和付费边界明确后逐家运行最小生命周期」）。不扩厂商、不把只读台账当自动发现完成、不提前进 P1/P2。
