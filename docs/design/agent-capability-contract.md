---
title: Agent capability 契约设计（P0 首片）
status: design-settled (2026-10-30, 已代决 docs/review/2026-10-30-decisions.md 项 23)
---

# Agent Capability 最小契约设计（P0 首片）

> 依据：`docs/review/2026-10-30-decisions.md` 项 23（推荐批准四个有依赖独立小范围，首片＝Capability 契约）。
> 状态：**设计定稿**。本片只落契约设计与盘点结论，不实现——按决策「这里是推荐新拍板，不表示本次授权了实现」，实现待用户拍板后单独立项。
> 盘点结果：现有 capability 上报与消费位置见 §2；契约缺口见 §3。

## 1. 目标与边界

- **目标**：为「能力/版本/权限状态」补最小契约，使 server 侧能判定「某 agent 是否支持某动作、以什么权限、什么版本」，而不靠消费方硬编码类型字符串。
- **边界**：不推翻现有 Agent Core 三层结构（core / internal/agent 插件宿主 / rpc Provider）；不另造插件大框架；不默认重试任意 shell 命令；不直接批量升级全部设备。
- **成功判据**：契约可表达「capability 类型 + 版本 + 权限状态」，消费方（server 的 `HasCapability` / 动作分派）能据此做能力门控，且旧 agent 缺字段时 fail-closed 不被误判支持。

## 2. 现有上报与消费位置（盘点结论）

**上报源（agent → server）**

| 位置 | 内容 |
|---|---|
| `internal/protocol/message.go:56-63` | `protocol.Capability{Type, Endpoint, Version, Metadata}`——注册负载携带 |
| `internal/protocol/message.go:84` | `RegisterPayload.Capabilities []Capability` |
| `internal/agent/agent.go:572` | capability 上报 metadata `read_only=true`（libvirt 只读片例） |
| `internal/probeagent/upstream.go:247` | 探针 agent 注册 `[]protocol.Capability{{Type:"probe"}}` |
| `internal/inventory/sync.go:105-111` | inventory 声明能力 → storage.Capability；`detectCapabilities` 从 config 键推断（`pve/docker/openwrt`） |
| `internal/agent/providers.go:42-105` | rpc Provider 注册（system/backup/file/ssh/job/oob/pve/…），与 capability 检测联动 |

**消费位置（server 侧）**

| 位置 | 内容 |
|---|---|
| `internal/server/agent.go:129` | `Agent.HasCapability(capType string)`——当前按类型字符串匹配 |
| `internal/server/api_stacks.go:223/230/524` | `HasCapability("docker-api"|"docker")` 硬编码门控 |
| `internal/server/api_overlay_cloud.go:209-232` | 从 capability metadata 提取 zerotier `nodeId/deviceId` |
| `internal/server/server.go:540-561` | 注册时对齐 `storage.Capability`（Endpoint/Metadata 直映射） |
| `internal/server/api.go:769-784` | REST 返回 capabilities 对象数组（web/mobile 读 `cap.type/cap.metadata`） |
| `internal/cloud/provider.go:40` / registry | 云 provider 声明能力 + fail-closed 校验（能力契约的既有先例） |

## 3. 契约缺口

1. **无权限状态字段**：`protocol.Capability` 只有 `Type/Endpoint/Version/Metadata`，无独立「权限状态」（如 `readonly` / `writable`）。`libvirt_provider.go` 把 `read_only=true` 塞进 `Metadata` 是特例，不是统一契约——消费方无法通用地判断「该能力可写吗」。
2. **版本无契约语义**：`Version` 字段存在但无约束（无版本比较、无兼容/能力版本协商），server 无法判断「能力版本是否满足某动作」。
3. **消费方硬编码**：`HasCapability("docker-api")` 这类字符串门控是契约缺失的直接后果；新增能力须改消费方代码。
4. **上报/声明双通道**：agent 运行时上报（register payload）与 inventory 声明（`inventory/sync.go`）并存，语义不同（`Metadata` vs `Config`），契约需明确二者关系。

## 4. 最小契约设计

### 4.1 契约结构（建议，实现待拍板）

```go
// protocol.Capability 扩展——新增字段，旧 agent 缺省 fail-closed
type Capability struct {
    Type     string                 `json:"type"`
    Endpoint string                 `json:"endpoint,omitempty"`
    Version  string                 `json:"version,omitempty"`
    Metadata map[string]interface{} `json:"metadata,omitempty"`
    // 新增：
    Access   CapabilityAccess       `json:"access,omitempty"`   // readonly|writable，缺省 readonly
}

// CapabilityAccess 权限状态
type CapabilityAccess string
const (
    CapabilityReadonly  CapabilityAccess = "readonly"
    CapabilityWritable  CapabilityAccess = "writable"
)
```

### 4.2 门控规则（fail-closed）

- **未知能力**：server 不因未识别类型而放行写动作；只读查询可放行。
- **权限状态**：写动作要求 `Access == writable`；`Access` 缺失（旧 agent）视为 `readonly`，写动作拒绝并记录——旧节点不被误判支持新动作。
- **版本**：动作带最低能力版本时，`Version` 缺失或低于要求 → 拒绝（兼容性不靠猜）。
- **metadata vs config**：`Metadata`（agent 运行时上报）与 `Config`（inventory 声明）语义不同、并存不互斥（`storage/models.go:57-64` 已注明）；契约不合并二者，消费方按需读取。

### 4.3 迁移路径（不批量升级）

- 新增字段向后兼容：旧 agent 不发送 → server 按 `readonly` 处理，不拒绝已有只读流程。
- 写能力（如 libvirt 写片、云生命周期）逐片升级时，该片 agent 升级后上报 `writable`，server 才放行对应写动作。
- 不默认重试任意 shell 命令、不直接批量升级全部设备（决策边界）。

## 5. 与本片的关系（后续三片）

首片契约定稿后，后续三片（并发/超时隔离 → 幂等与执行窗口 → 升级回滚）各自依赖此契约做能力门控与动作隔离；不改变本片契约内容，只在其上扩展动作语义。

## 6. 待用户拍板项

- 契约字段 `Access` 的具体枚举值（readonly/writable 是否够用，是否需 `admin` 级）——建议 `readonly|writable` 二值起步，够用即不扩。
- 版本号策略（semver vs 单调整数）——建议 semver，与 `cmd/cockpit-agent` 注入版本一致。
