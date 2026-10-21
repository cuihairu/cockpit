package oob

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Registry 是带外 Provider 的进程内注册表（与 internal/cloud.Registry 同款纪律）。
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry 返回空注册表。
func NewRegistry() *Registry {
	return &Registry{providers: map[string]Provider{}}
}

// Register 注册 Provider：名字非空且不重复。
func (r *Registry) Register(p Provider) error {
	if p.Name() == "" {
		return fmt.Errorf("oob: provider name must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.providers[p.Name()]; dup {
		return fmt.Errorf("oob: provider %q already registered", p.Name())
	}
	r.providers[p.Name()] = p
	return nil
}

// MustRegister registers Provider and panics on invalid input. It is for
// compile-time provider sets whose names are package constants.
func (r *Registry) MustRegister(p Provider) {
	if err := r.Register(p); err != nil {
		panic(err)
	}
}

// List 返回全部 Provider，按名排序（稳定遍历）。
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Provider, 0, len(names))
	for _, n := range names {
		out = append(out, r.providers[n])
	}
	return out
}

// ProbeResult 是单个 Provider 的探测产出。错误原样保留，不吞错（仓内纪律）。
type ProbeResult struct {
	Provider string
	Info     DeviceInfo
	Err      error
}

// ProbeAll 并发探测全部 Provider，逐路返回结果；一条路失败不影响其他路。
func (r *Registry) ProbeAll(ctx context.Context, target Target) []ProbeResult {
	providers := r.List()
	results := make([]ProbeResult, len(providers))
	var wg sync.WaitGroup
	for i, p := range providers {
		wg.Add(1)
		go func(i int, p Provider) {
			defer wg.Done()
			info, err := p.Probe(ctx, target)
			results[i] = ProbeResult{Provider: p.Name(), Info: info, Err: err}
		}(i, p)
	}
	wg.Wait()
	return results
}

// DeviceView 是多路线合并后的设备带外总画像。
type DeviceView struct {
	Architecture string   `json:"architecture,omitempty"`
	Providers    []string `json:"providers"`
	// Capabilities 每项能力取三态秩最高者，并记录来源 Provider（仲裁可追溯）。
	Capabilities map[Capability]CapabilityView `json:"capabilities"`
}

// CapabilityView 带来源标注的能力状态。
type CapabilityView struct {
	State  State  `json:"state"`
	Source string `json:"source"`
}

// Merge 把多路探测结果合并为总画像：
//   - 只有成功的路参与合并；失败路保留在 ProbeResult 里由调用方处置；
//   - 同一能力取 rank 高者，平秩先到先得（Provider 名序稳定，结果确定）；
//   - 未知能力项不丢弃（前向兼容）；Architecture 取第一个非空。
func Merge(results []ProbeResult) DeviceView {
	view := DeviceView{Capabilities: map[Capability]CapabilityView{}}
	seen := map[string]bool{}
	for _, res := range results {
		if res.Err != nil {
			continue
		}
		if !seen[res.Provider] {
			seen[res.Provider] = true
			view.Providers = append(view.Providers, res.Provider)
		}
		if view.Architecture == "" {
			view.Architecture = res.Info.Architecture
		}
		for capName, state := range res.Info.Capabilities {
			cur, exists := view.Capabilities[capName]
			if !exists || state.rank() > cur.State.rank() {
				view.Capabilities[capName] = CapabilityView{State: state, Source: res.Provider}
			}
		}
	}
	sort.Strings(view.Providers)
	return view
}
