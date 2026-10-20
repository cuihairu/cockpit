# 多云 Provider 框架设计（P0 第一片）

> 状态：设计已定，实现随片推进。来源评审：`docs/review/chatgpt-2026-10-20-multicloud-positioning.md`。
> 边界：不使用 Kubernetes CRD 全量模型（「不做」清单）；本设计只定义 Go 接口与内存注册表。

## 1. 目标与非目标

**目标**：给 Cockpit 增加一层「云 API 驱动」的资源控制面地基——可插拔 Provider 接口、能力声明、
异步操作语义、统一资源与成本模型。先落框架与类型，厂商适配器随后按片接入。

**非目标**（本片不做，防扩散）：

- 不接任何真实云 SDK（适配器片再做，需要用户提供 AccessKey）。
- 不做资源池/Reconciler/调度（P1）；不做预算策略引擎（P1）。
- 不迁移 Inventory（现有 `ComputeInstance` 不动；云资源以新模型并行入库，走「云资源发现」再关联）。

## 2. 首批厂商（已代决 2026-10-20，用户可推翻）

**阿里云 + 腾讯云**。理由：国内网络直达；仓内已有阿里云 DNS（`internal/dns/alidns.go`，含现成
签名实现 `sign.go`）与 DNSPod 集成经验；账号体系为用户现成所有。后续按需扩 AWS/Azure/GCP。

## 3. 接口拆分（能力声明 + 小接口，拒绝巨型接口）

```go
// internal/cloud
type Provider interface {
    Identity() ProviderIdentity           // 厂商标识、显示名、区域列表来源
    Capabilities() []Capability           // 声明式能力清单
}

type ComputeProvider interface {
    ListInstances(ctx context.Context) ([]Instance, error)
    CreateInstance(ctx context.Context, spec InstanceSpec) (Operation, error)
    StartInstance(ctx context.Context, id string) (Operation, error)
    StopInstance(ctx context.Context, id string) (Operation, error)
    DeleteInstance(ctx context.Context, id string) (Operation, error)
    GetOperation(ctx context.Context, op Operation) (Operation, error) // 异步轮询
}

type BillingProvider interface {
    QueryCosts(ctx context.Context, q CostQuery) ([]CostRecord, error)
}

type QuotaProvider interface {
    QueryQuotas(ctx context.Context, region string) ([]Quota, error)
}
```

**能力声明纪律**：注册表校验「声明的能力必须有对应接口实现」——声明 `CapabilityCompute` 却未实现
`ComputeProvider` 直接注册失败（fail-closed，防上游误判可用）。

## 4. 异步操作语义（Operation）

云 API 的成功返回 ≠ 资源到位（例：腾讯云 CVM 返回 RequestId 后仍需轮询）。统一模型：

```
Operation{ ID, Kind(create/start/stop/delete), NativeID(资源 ID 或厂商句柄),
           VendorRequestID, Status(pending|running|succeeded|failed), Error, UpdatedAt }
```

状态机：`pending → running → succeeded|failed`；`succeeded/failed` 为终态，终态再驱动即报错。
`VendorRequestID` 必须原样保留（审计与厂商工单凭据），失败错误带厂商原文（不吞错，仓内既有纪律）。

## 5. 资源与成本模型（新表，不动 ComputeInstance）

- `cloud_account`：厂商、别名、凭据引用（**只存 vault 引用 `${VAR}`，不落明文**——仓内既有纪律）、
  权限范围、启用态。
- `cloud_instance`：账号、原生资源 ID、名称、地域/可用区、规格、运行状态、计费模式
  （on-demand/prepaid/spot）、厂商原始状态串。
- 资源状态与计费状态**分开**：`stopped` 不等于停止计费（评审第五节语义），模型层显式留
  `BillingContinues bool` 由厂商适配器按事实填充，不做推断。

成本三层（预计/估算/实际）本片只落 `CostRecord` 类型与 `BillingProvider` 接口；账单采集器在适配器片。

## 6. 凭据与安全边界

- 凭据一律走既有 vault（`internal/...` vault provider），配置只写引用；密钥不进仓、不落日志。
- 单一写入方纪律：**同一云资源的写操作只允许经 Cockpit ComputeProvider 发起**；若未来引入
  OpenTofu，须先裁定写入权归属（P1 决策点），本片在文档中立牌不做机制。

## 7. 分片计划

| 片 | 内容 | 门禁 |
|---|---|---|
| **1（本片）** | `internal/cloud`：接口、能力声明+注册表校验、Operation 状态机、资源/成本类型 | `go build ./...`、`go test ./internal/cloud/...`、包覆盖率 100%（登记制口径） |
| **2（已落 2026-10-20）** | `cloud_accounts`/`cloud_instances` 表（GORM AutoMigrate 随 Open 自动迁移，无独立迁移脚本）+ 存储层 `internal/storage/cloud.go`（账号 upsert/list/delete、实例 upsert/list/delete，均按业务唯一键幂等）+ 凭据红线落地：表内只存 `${VAR}` 引用（形态校验拒绝明文与拼接），运行时经 env 解析、未设值 fail-closed；接线/web 双确认片已落（`/api/cloud/accounts` + `cloud/{vendor}/{alias}` vault secret + `cloud:read/write`） | 同上 + `go test ./internal/server/... ./internal/storage/...` |
| 3（Compute 已落 2026-10-20，Quota 子片待排） | 阿里云 ECS 适配器：**Compute 已落**（`internal/cloud/aliyun`：RPC V1 签名复用抽出的 `internal/alisign` 中立包（原 `internal/dns/sign.go` 迁移，dns 同步改用）；DescribeInstances 分页列表 + 状态/计费归一（过渡态归 unknown 且原文保留、BillingContinues 只按计费模式事实填充）；RunInstances 创建（`InstanceSpec` 增 `SecurityGroupID`，多数厂商创建必需）；启停删同构动作；GetOperation 以目标实例状态推断完成态，ECS 失败同步发生在请求时不制造 failed 态）+ fake server 契约测试（桩侧复算签名）包覆盖 100%；**Quota 子片待排**〔统一配额服务 API 契约未经真实账号核对，不硬造，随片 4 验收解锁〕 | 同上 + 厂商契约测试 |
| 4 | 腾讯云适配器 + 双厂商闭环验收（需用户 AccessKey） | 真机验收 checklist |

**外部阻塞（片 4 与片 3 的 Quota 子片）**：真实云账号 AccessKey 由用户提供（凭据经 vault 配置，不进聊天不进仓）。片 3 的 Compute 部分已按厂商契约测试（fake server）落地；真实账号连通性验证并入片 4 双厂商闭环验收。
