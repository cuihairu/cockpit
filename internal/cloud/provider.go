package cloud

import (
	"context"
	"errors"
)

// ErrNotSupported 表示厂商未提供该能力（适配器在显式未支持时返回，而不是伪装成功）。
var ErrNotSupported = errors.New("cloud: capability not supported by provider")

// Provider 是所有厂商适配器的最小公共面：标识 + 能力声明。
// 不设计巨型接口：计算/账单/配额各自独立，见 docs/design/cloud-provider.md 第 3 节。
type Provider interface {
	Identity() ProviderIdentity
	Capabilities() []Capability
}

// ComputeProvider 是计算生命周期能力。云 API 的异步性由 Operation 表达：
// Create/Start/Stop/Delete 返回已受理的 Operation，完成态经 GetOperation 轮询。
type ComputeProvider interface {
	ListInstances(ctx context.Context) ([]Instance, error)
	CreateInstance(ctx context.Context, spec InstanceSpec) (Operation, error)
	StartInstance(ctx context.Context, id string) (Operation, error)
	StopInstance(ctx context.Context, id string) (Operation, error)
	DeleteInstance(ctx context.Context, id string) (Operation, error)
	GetOperation(ctx context.Context, op Operation) (Operation, error)
}

// BillingProvider 是账单查询能力。
type BillingProvider interface {
	QueryCosts(ctx context.Context, q CostQuery) ([]CostRecord, error)
}

// QuotaProvider 是配额查询能力。
type QuotaProvider interface {
	QueryQuotas(ctx context.Context, region string) ([]Quota, error)
}

// Supports 判断 provider 是否声明了某能力。
func Supports(p Provider, c Capability) bool {
	for _, have := range p.Capabilities() {
		if have == c {
			return true
		}
	}
	return false
}
