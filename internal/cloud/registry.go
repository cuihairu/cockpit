package cloud

import (
	"fmt"
	"sort"
	"sync"
)

// Registry 是厂商适配器的进程内注册表。
// 注册即校验：声明的能力必须有对应接口实现（fail-closed），
// 防止上游把「声明了但没实现」的厂商误判为可用。
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry 返回空注册表。
func NewRegistry() *Registry {
	return &Registry{providers: map[string]Provider{}}
}

// Register 注册厂商适配器。校验顺序：vendor 非空 → 每项声明能力有实现 → 不重复。
func (r *Registry) Register(p Provider) error {
	id := p.Identity()
	if id.Vendor == "" {
		return fmt.Errorf("cloud: provider identity vendor must not be empty")
	}
	for _, c := range p.Capabilities() {
		if err := checkImplementation(p, c); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.providers[id.Vendor]; dup {
		return fmt.Errorf("cloud: provider %q already registered", id.Vendor)
	}
	r.providers[id.Vendor] = p
	return nil
}

// checkImplementation 校验单条能力声明与接口实现一致。
func checkImplementation(p Provider, c Capability) error {
	vendor := p.Identity().Vendor
	switch c {
	case CapabilityCompute:
		if _, ok := p.(ComputeProvider); !ok {
			return fmt.Errorf("cloud: provider %q declares %s without implementing ComputeProvider", vendor, c)
		}
	case CapabilityBilling:
		if _, ok := p.(BillingProvider); !ok {
			return fmt.Errorf("cloud: provider %q declares %s without implementing BillingProvider", vendor, c)
		}
	case CapabilityQuota:
		if _, ok := p.(QuotaProvider); !ok {
			return fmt.Errorf("cloud: provider %q declares %s without implementing QuotaProvider", vendor, c)
		}
	default:
		return fmt.Errorf("cloud: provider %q declares unknown capability %q", vendor, c)
	}
	return nil
}

// Get 按厂商标识取适配器。
func (r *Registry) Get(vendor string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[vendor]
	return p, ok
}

// List 返回全部已注册适配器，按 vendor 排序（稳定遍历，便于 UI 与测试）。
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	vendors := make([]string, 0, len(r.providers))
	for v := range r.providers {
		vendors = append(vendors, v)
	}
	sort.Strings(vendors)
	out := make([]Provider, 0, len(vendors))
	for _, v := range vendors {
		out = append(out, r.providers[v])
	}
	return out
}
