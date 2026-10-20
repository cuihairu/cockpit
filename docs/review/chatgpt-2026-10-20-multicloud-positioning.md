# Cockpit 多云扩展定位评审（外部意见导入稿）

> 来源：https://chatgpt.com/s/t_6ad6ae250e1c8191af8d5f935715ed6b
> 提取日期：2026-10-20（分享页 HTML flight-stream 恢复，8 段去重收敛）
> **性质声明：本文是外部评审意见，不是本仓事实的陈述。**每一条对现有实现的论断都已对照代码核验（见文末「核验结论」表）；未核验通过的部分一律按「待核验」处理，不得直接当作需求依据开工。

---

## 一、真正的产品定位差异

个人云运维与游戏基础设施运维，底层能力高度相似，但资源的获取方式、控制权、调度规模和成本模型完全不同。两者都不应该管理玩家、道具、GM、活动或者游戏逻辑。

| 维度 | 个人云基础设施运维 | 游戏基础设施运维 |
|--------|--------|--------|
| 核心目标 | 整合和管理已有资源 | 按需获取、组织和优化资源 |
| 资源来源 | 家庭设备、NAS、PVE、自有服务器、VPS | 阿里云、腾讯云、AWS、GCP、Azure、裸金属等 |
| 资源管理 | 发现、接入、维护 | 发现、采购、创建、调整、回收 |
| 计算能力 | 管理固定容量 | 根据需求动态申请容量 |
| 操作对象 | 主机、虚拟机、容器、网络、存储 | 云账号、地域、实例池、计算、网络、存储 |
| 生命周期 | 长期运行、人工维护为主 | 动态创建、启动、停止、释放 |
| 成本管理 | 资产成本、续费、到期提醒 | 实时成本、预算、账单、归集、优化 |
| 自动化 | 定时任务、健康检查、自恢复 | 资源编排、容量规划、批量调度、自动回收 |
| 多租户 | 以个人或少量用户为主 | 多项目、团队、环境的资源隔离 |
| 运维规模 | 通常少量至数十台设备 | 可以扩展至数百或数千实例 |

最本质的区别：

- **个人云：我已经拥有哪些资源，如何统一管理？**
- **游戏基础设施：我需要多少资源、去哪里获取、什么时候启动、如何协调、什么时候释放、花费多少？**

「游戏」只决定基础设施的规模、资源配置和成本分摊需求，并不需要进入游戏业务逻辑。

## 二、对 Cockpit 的判断

Cockpit 已有 `Region`、`Zone`、`Agent`、`ComputeInstance`、`Service`、`Storage` 等资源概念，也有 Docker、Compose、远程执行、DNS、备份和监控等能力。但现有 `ComputeInstance` 主要记录名称、CPU、内存、磁盘、IP、Agent 和标签，并没有形成完整的云账号、云厂商原生资源 ID、计费模式、价格、云 API 生命周期和配额模型。

所以 Cockpit 目前更像一个 **Agent 驱动的基础设施管理平台**，而希望它进一步具备的是 **Cloud API 驱动的多云资源控制与成本管理平台**。

两者之间的关键差距：Cockpit 现在擅长管理一台已经存在、能够连接 Agent 的机器；云基础设施平台必须能够管理**尚未创建、已经关机、没有安装 Agent、甚至已经释放但仍有账单记录的资源**。

## 三、应该新增的六个核心能力

### 1. 多云 Provider 层（最大架构缺口）

需要支持：云账号接入（阿里云、腾讯云、AWS、Azure、GCP）；云资源发现（云主机、磁盘、VPC、LB、公网 IP、对象存储、数据库）；生命周期（创建、启动、停止、调整规格、快照、释放）；容量查询（地域、可用区、规格、配额、余量）；计费查询（价格、历史账单、预付费、折扣、实际成本）。

不建议设计一个巨大、要求每家云厂商完全实现的 `CloudProvider` 接口，应拆为独立能力：

```go
type CloudProvider interface {
    Identity() ProviderIdentity
    Capabilities() []Capability
}
type ComputeProvider interface {
    ListInstances(ctx context.Context) ([]Instance, error)
    CreateInstance(ctx context.Context, spec InstanceSpec) (Operation, error)
    StartInstance(ctx context.Context, id string) (Operation, error)
    StopInstance(ctx context.Context, id string) (Operation, error)
    DeleteInstance(ctx context.Context, id string) (Operation, error)
}
type BillingProvider interface {
    QueryCosts(ctx context.Context, q CostQuery) ([]CostRecord, error)
}
type QuotaProvider interface {
    QueryQuotas(ctx context.Context, region string) ([]Quota, error)
}
```

`Operation` 代表异步云操作，必须有可查询状态和厂商请求 ID（例：腾讯云 CVM 启动接口返回 RequestId 不代表已启动完成，需继续查询状态）。统一云 API 的核心不是方法名，而是：**统一操作意图、异步执行结果、错误语义与能力发现**。

### 2. 资源池与容量协调

控制对象不应只是单台云服务器，而是可动态伸缩的资源池：

```yaml
pool: realtime-compute-asia
requirements:
  region: ap-southeast
  cpu: 8
  memoryGiB: 16
  os: linux
  capacity: { min: 5, desired: 20, max: 100 }
  purchase: { models: [on-demand, spot], maxHourlyBudget: 80 }
```

它表达的只是「在指定地域维持一定数量、一定规格、符合成本约束的计算资源」，无任何游戏业务逻辑。系统要做资源发现、采购、容量预留、扩缩容、故障替换、回收和账单关联。还要支持两种容量：**已持有容量**（包年包月、预留、自有裸金属）与**弹性容量**（按量、竞价、可动态创建释放）。

### 3. 资源协调控制器（Reconciler）

不要设计成「点一次按钮执行一次云 API」，而是声明式：期望（东京 20 台）→ 实际（17 运行/1 启动中/2 失败）→ 控制器（判缺口、查预算配额、重试或替换失败实例、协调到目标容量）。Cockpit 不必依赖 Kubernetes——一个小型 Go Reconciler 即可，让资源池具有期望状态、实际状态和协调记录。

### 4. 成本管理（FinOps，一级功能而非价格字段）

三种数据必须分开：

| 成本概念 | 数据来源 | 用途 |
|---|---|---|
| 预计成本 | 云产品报价、当前资源规模 | 采购前决策 |
| 运行估算成本 | 实例运行时间、价格与计费模型 | 实时预算控制 |
| 实际账单成本 | 云厂商账单 API、账单导出 | 财务核算与成本归集 |

实际计费远比 `CPU × 小时` 复杂（如 AWS EC2 停机后实例费停但 EBS/部分 IP 费用继续）。阿里云账单 API 数据可能延迟约一天，不能把实际账单当秒级实时成本。成本归属链至少：

`CloudProvider → Account → Region → Project → Environment → ResourcePool → Resource`

其中 Project / Environment 是**基础设施成本标签**，不表示 Cockpit 要管理游戏业务对象。共享负载均衡器/VPC/流量费用需可配置分摊规则。

### 5. 云厂商差异化的资源生命周期

不同云厂商对「关机」定义不同（如 Azure Deallocate ≠ OS Shutdown）。资源状态与计费状态必须分开：

| 资源状态 | 计算资源是否占用 | 是否继续计费 |
|---|---|---|
| Running | 通常是 | 通常是 |
| Stopped | 取决于厂商及操作方式 | 部分资源继续计费 |
| Deallocated | 计算容量已释放 | 存储、IP 等仍可能收费 |
| Terminated | 实例已释放 | 残留资源与历史费用仍需核对 |

调度器还必须考虑：Stop 与 Delete 的安全边界；按量与包年包月不能套用同一回收策略；Spot 可能被厂商回收；释放失败要持续重试并告警；已释放资源仍保留成本与操作历史。

### 6. 基础设施策略引擎

允许平台自动申请云服务器后，安全边界要从 RBAC 扩展为**资源与成本策略**：

```yaml
policy:
  project: project-a
  environment: production
  allowedProviders: [aliyun, tencentcloud, aws]
  allowedRegions: [ap-southeast-1, ap-northeast-1]
  maxInstances: 100
  maxMonthlyCost: 10000
  currency: USD
  requireApprovalFor: [delete-production-resource, budget-override, purchase-commitment]
```

预算达上限不应直接关闭生产服务器，更合理的是限制新资源创建、触发告警、请求审批，对允许回收的资源采取相应措施。

## 四、推荐架构：两个能力域，不拆两套产品

```
Cockpit 统一基础设施控制面：认证 · RBAC · Inventory · 审计 · API · UI
├── Infrastructure Agent（已有资源运维）
│   主机/系统 · Docker/Compose · NAS/PVE · 网络/存储 · 终端/备份/监控
└── Cloud Control（新增资源控制面）
    Cloud Provider SDK · Resource Reconciler · Capacity Scheduler
    Cloud Billing / FinOps · Quota/Budget/Policy
统一资源 Inventory：CloudAccount · Resource · Resource · Region · Pool · Ownership · Cost
```

两个系统做成 Cockpit 的两个能力域，因为资源发现、权限、任务、监控、审计、远程访问可以共用。产品层面两种功能组合：**Personal Infrastructure**（自有设备、VPS、NAS、PVE）与 **Cloud Infrastructure Operations**（云账号、生命周期、资源池、容量协调、FinOps），共用 Inventory 但不强制安装全部功能。

## 五、不要从零重复开发成熟能力（借鉴而非塞入）

| 开源项目 | 借鉴/集成能力 | 建议 |
|--------|--------|--------|
| OpenTofu | 跨云资源定义与生命周期 | 复杂整体创建走执行计划 |
| CloudQuery | 跨云配置数据同步 | 配置盘点参考 |
| OpenCost | 容器化成本归集 | Kubernetes 场景可考虑 |

技术分工建议：

- 日常云服务器启停、状态查询 → 官方 Cloud SDK
- VPC、子网、LB、复杂集群整体创建 → OpenTofu 执行计划
- 资源扫描 → 官方 API + 可扩展发现器
- 账单 → 官方 Billing API + FOCUS 规范化
- 调度 → Cockpit 自己实现资源池策略与协调逻辑

**要避免双重控制**：同一资源必须明确由 OpenTofu、Cockpit Reconciler 或外部系统中的哪一个负责写入；其他系统只能观察或通过资源拥有者提交操作，否则会产生状态覆盖。

## 六、开发优先级建议

| 优先级 | 建议模块 | 工作内容 |
|--------|--------|--------|
| P0 | Cloud Account | 多云账号、凭据、权限范围 |
| P0 | Cloud Resource Inventory | 自动扫描、资源去重、原生 ID 关联 |
| P0 | Provider Framework | 可插拔接口、能力声明、官方 SDK 适配 |
| P0 | Compute Lifecycle | 创建、启动、停止、释放、状态追踪 |
| P0 | Cost Foundation | 账单采集、估算、资源成本归属 |
| P1 | Resource Pool | 计算资源池、配额、容量管理 |
| P1 | Reconciler | 期望容量与实际资源协调 |
| P1 | Cloud Networking | VPC、子网、EIP、安全组、LB |
| P1 | Budget & Policy | 预算、审批、成本限制、防误删 |
| P2 | Smart Scheduling | 根据容量、价格、地域与约束选云 |
| P2 | Cost Optimization | 空闲资源、Spot、预留实例、节省建议 |
| P2 | High Availability | 多实例控制面、任务恢复、故障切换 |

优先选两家实际要使用的云厂商，把「账号接入 → 自动发现 → 创建实例 → 安装/关联 Agent → 停止/释放 → 成本归集」做成完整闭环，再扩大 Provider 数量。

## 七、最终判断

Cockpit 的扩展价值反而更大，因为它没有必要成为游戏专用平台。可以从 **Personal Hybrid Infrastructure Console** 逐步发展为 **Hybrid & Multi-Cloud Infrastructure Operations Platform**：个人用户用它管理自己的机器和云资源；游戏公司的基础设施团队用它管理多云计算资源、容量池和成本。两种用户共享控制面，只是关注的能力不同。

游戏业务系统（如 croupier）将来可以向 Cockpit 提交资源需求，但 Cockpit 不需要理解它们的业务内容，只按授权、预算、容量和基础设施策略提供并管理资源。

**结论：继续沿用 Cockpit，不为游戏基础设施另起炉灶；真正需要新增的是 Cloud Provider、Resource Pool、Reconciler 和 FinOps 四个核心子系统。**

---

## 核验结论（2026-10-20，对照 origin/main cab61bf 逐条实查）

| # | 评审论断 | 核验结果 | 证据 |
|---|---|---|---|
| 1 | 已有 Region/Zone/Agent/ComputeInstance/Service/Storage 概念 | ✅ 属实 | Region 36 个 go 文件、Zone 67、Agent 259；`ComputeInstance`（Name/AgentID/Type/Provider/Region/Zone/Status/…） |
| 2 | ComputeInstance 无云原生资源 ID/计费模式/价格/配额模型 | ✅ 属实 | 字段清单无 native ID、billing、price、quota；Quota 仅 3 个文件零星出现，Cost 1 个 |
| 3 | 无 CloudAccount 概念 | ✅ 属实 | `CloudAccount` 0 个 go 文件；`Billing` 0 个 |
| 4 | 「Cockpit 现在擅长管已存在、能连 Agent 的机器」 | ✅ 属实 | 与 todo 已完成清单（Docker/服务/备份/远控等均经 agent 链路）一致 |
| 5 | Reconciler「不必依赖 Kubernetes」 | ✅ 兼容本仓边界 | 「不做」清单已否决 K8s 风格 CRD 全量模型；评审自己也建议小型 Go 实现，不冲突 |

**未能核验（需用户补充决策史）**：评审中「此前设计/已有架构」类表述涉及的决策过程（如是否曾讨论过多云方向、被搁置的理由），文档层面查无记录——按惯例归类为「未落文档」而非「全新需求」，其立项优先级由用户拍板。

## 落地跟踪

> 每项落地后勾选并附 commit hash + 测试证据。P0 已拍板开工（2026-10-20，todo.md:55；首批厂商已代决：阿里云+腾讯云）。

- [ ] P0 云账号接入（CloudAccount 模型 + 凭据入 vault + 权限范围）〔片2 已落 CloudAccount 表 + `${VAR}` 引用形态校验（8825a29）；接线/web 双确认片已落（进度注记见 todo.md:55），剩余=发现/生命周期接线与腾讯云适配器〕
- [ ] P0 云资源自动发现与原生 ID 关联（Inventory 扩展）
- [x] P0 Provider Framework（拆分接口 + 能力声明 + 异步 Operation 语义）〔已落 cfeee64：`internal/cloud` 拆分接口/能力声明+注册表 fail-closed 校验/Operation 状态机，包测试绿 + 覆盖 100%〕
- [ ] P0 计算生命周期闭环（先选两家实际在用的云：接入→发现→创建→关联 Agent→停止/释放→成本归集）〔片3 Compute 适配器已落：`internal/cloud/aliyun` ECS ComputeProvider，fake server 契约测试绿 + 包覆盖 100%（hash 见 todo.md:55）；账号/实例接线与腾讯云片4 待做〕
- [ ] P0 成本地基（预计/估算/实际三层 + 资源状态与计费状态分离 + 归属链）
- [ ] P1 资源池与容量（已持有 + 弹性两种容量）
- [ ] P1 Go Reconciler（期望/实际/协调记录；不碰 CRD）
- [ ] P1 云网络（VPC/子网/EIP/安全组/LB）
- [ ] P1 预算与策略引擎（超预算=限新建+告警+审批，不直接关生产）
- [ ] P1 双重控制边界裁定（OpenTofu vs Cockpit Reconciler 写入权归属）
- [ ] P2 智能调度 / 成本优化 / 控制面高可用
