---
title: Agent 升级与回滚设计（P0 第四片）
status: implemented (2026-11-22, 依据 docs/review/2026-10-30-decisions.md 项 23；§2 边界与延后项照旧)
---

# Agent 升级与回滚（升级产物验证·健康检查·回退成功条件）

> 依据：`docs/review/2026-10-30-decisions.md` 项 23「升级回滚」片，立即执行动作
> 「升级产物验证、健康检查和回退成功条件」。边界：**不直接批量升级全部设备**
> （决策明确不做）；agent 二进制 OS 级换装需真机验收，超出本片，诚实延后。

## 1. 现状与可复用件

- agent 注册上报自身二进制版本：`protocol.RegisterPayload.Version`
  （`internal/protocol/message.go:112`，build 注入），落库 `storage.Agent.Version`
  （`internal/storage/models.go:17`）。
- 版本比较已具备：`internal/protocol/version.go` `CompareVersion`/`VersionSatisfies`
  （semver-lite，首片 af886dfd）——升级门控、回退成功判定、min-version 校验复用。
- **无**产物存储、无升级意图、无回滚机制（全仓零匹配）。

## 2. 边界（诚实清单）

- 本片实现**控制面**：产物登记与验证、单 agent 升级意图状态机、健康检查与
  回退成功条件判定、审计。**不做** agent 二进制 OS 级换装/下载通道（需 agent
  自更新协议 + 真机验收，超出本片）。
- **不做**批量升级全部设备（决策明确不做）：升级 API 只接受**单 agent** 显式
  指定，逐台推进，绝无「全部」开关。
- 产物上传只经服务器（管理员鉴权），不经 agent 侧任意拉取。

## 3. 设计

### 3.1 产物登记与验证（升级产物验证）

- `storage.AgentArtifact`：`version/arch/os/size/sha256/min_version/created_at`。
- 上传校验：sha256 与内容一致、size>0 且有上界（默认 256MB）、version 可解析
  （semver-lite 格式）、`VersionSatisfies(version, min_version)`（min_version 空=不门控）。
- 已存在同 `version+arch+os` 的登记 → 覆盖更新（幂等），记录审计。

### 3.2 升级意图状态机（单 agent 显式）

```
intent: planned → deploying → verifying → succeeded
                              └→ rolled_back  （健康检查不过/超窗）
```

- `POST /api/agents/{id}/upgrade`（body：version）→ 建 `AgentUpgrade`
  （agent_id/from_version/to_version/status=planned/created_at），**非 bulk**：
  仅单 agent，且校验 agent 在线、目标产物存在、to_version > from_version
  （CompareVersion>0，拒绝降级/同级）。
- 状态推进由 server 巡检（reconcile loop）驱动，不依赖前端轮询写状态。

### 3.3 健康检查与回退成功条件

- **健康窗口**：agent 在窗口（默认 120s）内重新注册（heartbeat）且上报
  `Version == to_version` → `succeeded`。
- **回退条件**（任一命中即回退意图）：窗口内未注册、上报版本仍为
  from_version（未升级成功）、或注册但能力契约校验失败
  （`CanWrite`/`CapabilitySatisfies` 对既有依赖不满足）。
- **回退成功条件**（判定函数，纯逻辑可单测）：回退意图后 agent 在窗口内
  重新注册且上报 `Version == from_version` → `rolled_back`；否则升级意图终态
  `failed`（人工介入）。
- 回退不自动重放（决策 31 口径）：人工确认后新升级意图才可再发。

### 3.4 可观测与审计

- 升级意图全生命周期审计（operator、agent、from/to version、终态）。
- `AgentUpgrade` 状态可查（GET），供前端展示与排障。

## 4. 实现位置

- `internal/storage/upgrade.go`（新）：`AgentArtifact`/`AgentUpgrade` 模型与 CRUD。
- `internal/server/api_upgrade.go`（新）：上传产物（multipart）、单 agent 升级、
  状态查询、回退判定。
- `internal/server/upgrade_reconcile.go`（新）：巡检循环推进意图状态机。
- `internal/server/upgrade_eval.go`（新）：健康/回退/回退成功判定纯函数。
- `internal/protocol/version.go`：复用（首片已落）。

## 5. 验收

- 单测：产物校验表（sha 错/空 size/超上界/版本不可解析/min-version 不满足）、
  状态机推进（planned→succeeded；超窗→回退；回退成功→rolled_back）、
  「拒绝批量/降级/离线 agent/未知产物」。
- 全量 `go test -short ./... -coverprofile` 新鲜 + `tool/coverage_check.sh` 绿；
  `-race` 绿。
- 行为不变性：未配置产物/未建升级意图时，现有 agent 生命周期不受影响。

## 6. 决策对照

落实项 23「升级产物验证、健康检查和回退成功条件」；「不直接批量升级全部设备」
明确不做；agent 二进制换装属真机验收项（延后，见 §2 边界）。

## 7. 实现注记（2026-11-22，实现时与 §3 的口径对齐项）

- **产物 sha256 由服务器对上传内容计算**，不接受客户端声明哈希（内容一致性
  由服务器自证，§3.1「sha256 与内容一致」即此口径）。超上界返回 413。
- **arch/os 缺省 = 任意已登记平台**：`storage.Agent` 无 arch/os 持久化列
  （agent 经系统信息上报，未落库），升级意图的产物匹配由请求显式声明目标
  平台；arch/os 都缺省时取同版本的任意已登记产物（最早登记优先）。
- **min_version 门控**在意图创建时校验（agent 当前版本须 >= 产物
  min_version，§3.1 的 min_version 语义落在升级入口而非仅上传）。
- **requiredCapabilities**：意图可选声明升级后 agent 仍须声明的能力契约；
  `verifying` 期校验失败即回退（§3.3「能力契约校验失败」回退条件的落点）。
- **巡检周期 5s**（`upgradeReconcileInterval`，包级变量可测试注入）；
  `upgradeReconcileLoop` 经 `goBg` 接线，`ctx.Done()` 退出。
- **判定纯函数** `upgradeNextStatus` 不触 DB（agent 版本/能力由调用方传入），
  状态机推进经条件 UPDATE `AdvanceUpgrade`（命中 0 = 已被并发推进则放弃）。
