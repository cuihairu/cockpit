---
title: Agent RPC 调度资源隔离设计（P0 第二片）
status: implemented (2026-11-21, 依据 docs/review/2026-10-30-decisions.md 项 23)
---

# Agent RPC 调度资源隔离（并发上限·优先级·超时预算）

> 依据：`docs/review/2026-10-30-decisions.md` 项 23「并发/超时隔离」片，首片
> Capability 契约已落（`af886dfd`）。边界：不推翻 Agent Core 三层结构、不另造
> 插件大框架（决策明确不做）。

## 1. 现状与缺口

- agent 对每条 `MessageTypeRPCRequest` 现为 `go a.handleRPCRequest(msg)` 无界
  goroutine（`internal/agent/agent.go:697`）；`handleRPCRequest` 同步调
  `a.rpc.Handle`（`agent.go:730`）。**无并发上限**：请求洪峰 → 无界 goroutine
  + 无界并发 Provider 调用（资源耗尽面）。
- **无优先级**：所有 RPC 同等；控制面（`system.*`、`job.cancel`、`probe.*`）可被
  批量普通任务挤占。
- **无准入超时预算**：任务排队无上界等待（当前根本无排队，直接全并发）。

## 2. 边界（诚实清单）

- `provider.Provider.Call(action, params)` **无 ctx 参数**（`internal/agent/provider/provider.go:9`）。
  因此本片预算只作用于**准入等待**，**不作用于 in-flight Provider 调用**——挂死的
  Provider 调用不可被本片超时取消（那是 Provider 接口改造，超出本片；job provider
  已有自身 300s 超时与 cancel 通道）。本片不假装能取消在飞调用。
- 并发上限是「准入」上限：超过上限的请求排队（有界），不无界堆积，也不无条件丢弃
  普通任务。

## 3. 设计

### 3.1 准入模型（有界队列 + 双优先级 + 准入超时）

```
handleRPCRequest(msg)
  └─ classify(method) → lane: high | normal
      └─ sched.Acquire(lane, admissionTimeout)
           ├─ 得槽 → 跑 provider.Call → 释放槽 → 发响应
           └─ 超时/队列满 → 发 error 响应（status:"error", error="rpc admission timeout"）
```

- **双车道**：
  - `high`：控制面方法（`system.*`、`job.cancel`、`probe.*`），独立小槽位
    （默认 2），准入等待短（默认 500ms）——防控制面被普通任务挤死。
  - `normal`：其余方法，主槽位（默认 8），准入等待默认 5s。
- **并发上限**：normal 车道默认 8（`RPCMaxConcurrent`，可配，1..64 钳制）；
  high 车道默认 2（`RPCMaxHighConcurrent`）。
- **超时预算**：普通车道准入默认 5s（`RPCAdmissionTimeoutS`）；high 车道
  500ms（`RPCHighAdmissionTimeoutS`）。超时 → 错误响应，**不挂起**（挂起的是
  已准入的调用，非队列）。
- **失败处置**：准入失败只影响该请求（错误响应），不杀连接、不影响其他 RPC
  （与 Provider 返回业务错误同语义——RPC 层不视为协议错误）。

### 3.2 可观测

- 每车道槽位用 buffered chan 实现（`chan struct{}`），`Acquire` 用
  `select`（slot / ctx.Done / timeout）——确定性可测，无竞态。
- 不新增全局计数；日志在准入超时/满时打一条（`log.Printf`），带 method。

## 4. 实现位置

- `internal/agent/scheduler.go`（新）：`Scheduler`（双车道 + Acquire/Release）
  + `classifyMethod`。纯逻辑、无平台分支，可单测。
- `internal/agent/agent.go`：`handleMessage` 的 RPC 分支改为经 `Scheduler` 准入；
  `Agent` 持有 `sched`（`NewAgent` 装配，从 Config 读上限/超时）。
- `internal/agent/agent.go` Config：新增 `RPCMaxConcurrent`、`RPCMaxHighConcurrent`、
  `RPCAdmissionTimeoutS`、`RPCHighAdmissionTimeoutS`（0 用默认）。

## 5. 验收

- 单测：`scheduler_test.go`——并发上限（>N 请求只放行 N）、优先级（high 在
  normal 之前）、准入超时（无槽位时 timeout → error）、释放后复用、配置钳制。
- `handleRPCRequest` 接线测试：超时路径返回 error 响应。
- 全量 `go test -short ./... -coverprofile` 新鲜 + `tool/coverage_check.sh` 绿；
  `-race` 绿。
- 行为不变性：默认配置下并发 ≥ 普通单任务（上限 ≥1），不改变单请求语义。

## 6. 决策对照

落实项 23「并发上限、取消与超时预算」；「维护窗口」属第三片（幂等与执行窗口），
「升级产物验证/健康检查/回退」属第四片（升级回滚）。本片不实现优先级抢占
（preempt），只做准入优先级——抢占在飞调用需 Provider 接口 ctx，超出边界。
