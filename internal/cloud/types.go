// Package cloud 定义多云资源控制面的地基类型：能力声明、异步操作、资源与成本模型。
// 设计文档：docs/design/cloud-provider.md（评审导入：docs/review/chatgpt-2026-10-20-multicloud-positioning.md）。
package cloud

import "time"

// Capability 是 Provider 的声明式能力标识。
type Capability string

const (
	// CapabilityCompute 声明提供计算生命周期（列表/创建/启停/释放/操作轮询）。
	CapabilityCompute Capability = "compute"
	// CapabilityBilling 声明提供账单查询。
	CapabilityBilling Capability = "billing"
	// CapabilityQuota 声明提供配额查询。
	CapabilityQuota Capability = "quota"
)

// ProviderIdentity 是厂商的稳定标识。
type ProviderIdentity struct {
	Vendor      string   `json:"vendor"`
	DisplayName string   `json:"display_name"`
	Regions     []string `json:"regions,omitempty"`
}

// OperationKind 是异步云操作的类别。
type OperationKind string

const (
	OperationCreate OperationKind = "create"
	OperationStart  OperationKind = "start"
	OperationStop   OperationKind = "stop"
	OperationDelete OperationKind = "delete"
)

// OperationStatus 是异步云操作的状态。状态机：pending → running → succeeded|failed。
type OperationStatus string

const (
	OperationPending   OperationStatus = "pending"
	OperationRunning   OperationStatus = "running"
	OperationSucceeded OperationStatus = "succeeded"
	OperationFailed    OperationStatus = "failed"
)

// Operation 表示一次异步云操作。云 API 返回成功不等于资源到位（例：CVM 返回
// RequestId 后仍需轮询），统一以本模型追踪。VendorRequestID 原样保留作审计凭据。
type Operation struct {
	ID              string          `json:"id"`
	Kind            OperationKind   `json:"kind"`
	NativeID        string          `json:"native_id,omitempty"`
	VendorRequestID string          `json:"vendor_request_id"`
	Status          OperationStatus `json:"status"`
	Error           string          `json:"error,omitempty"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// InstanceStatus 是资源的运行态归一化值。
type InstanceStatus string

const (
	InstanceCreating InstanceStatus = "creating"
	InstanceRunning  InstanceStatus = "running"
	InstanceStopped  InstanceStatus = "stopped"
	InstanceDeleting InstanceStatus = "deleting"
	InstanceUnknown  InstanceStatus = "unknown"
)

// BillingMode 是计费模式。按量/包年包月/竞价不能套用同一回收策略。
type BillingMode string

const (
	BillingOnDemand BillingMode = "on-demand"
	BillingPrepaid  BillingMode = "prepaid"
	BillingSpot     BillingMode = "spot"
)

// Instance 是云实例的统一资源模型（新表，不动既有 ComputeInstance）。
type Instance struct {
	AccountID        string         `json:"account_id"`
	NativeID         string         `json:"native_id"`
	Name             string         `json:"name"`
	Region           string         `json:"region"`
	Zone             string         `json:"zone,omitempty"`
	Spec             string         `json:"spec,omitempty"`
	Status           InstanceStatus `json:"status"`
	BillingMode      BillingMode    `json:"billing_mode"`
	BillingContinues bool           `json:"billing_continues"` // 资源状态≠计费状态：由适配器按厂商事实填充，不做推断
	RawVendorStatus  string         `json:"raw_vendor_status,omitempty"`
}

// InstanceSpec 是创建实例的意图描述。
type InstanceSpec struct {
	Name        string      `json:"name"`
	Region      string      `json:"region"`
	Zone        string      `json:"zone,omitempty"`
	Spec        string      `json:"spec"`
	ImageID     string      `json:"image_id"`
	BillingMode BillingMode `json:"billing_mode"`
}

// CostKind 区分成本三层：预计（报价）、运行估算、实际账单。
type CostKind string

const (
	CostEstimated   CostKind = "estimated"
	CostRunEstimate CostKind = "run-estimate"
	CostActual      CostKind = "actual"
)

// CostQuery 是账单查询条件。
type CostQuery struct {
	AccountID string    `json:"account_id,omitempty"`
	Region    string    `json:"region,omitempty"`
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
}

// CostRecord 是一条成本记录。实际账单可能延迟（阿里云约一天），不能当实时数据用。
type CostRecord struct {
	AccountID string    `json:"account_id"`
	NativeID  string    `json:"native_id"`
	Kind      CostKind  `json:"kind"`
	Currency  string    `json:"currency"`
	Amount    float64   `json:"amount"`
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
}

// Quota 是一个地域的配额项。Total < 0 表示厂商未提供上限。
type Quota struct {
	Region string `json:"region"`
	Metric string `json:"metric"`
	Used   int64  `json:"used"`
	Total  int64  `json:"total"`
}
