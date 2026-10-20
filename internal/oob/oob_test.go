package oob

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeProvider struct {
	name string
	info DeviceInfo
	err  error
}

func (f fakeProvider) Name() string { return f.name }
func (f fakeProvider) Probe(context.Context, Target) (DeviceInfo, error) {
	return f.info, f.err
}

func TestStateUsableRequiresAllThree(t *testing.T) {
	cases := []struct {
		state State
		want  bool
	}{
		{State{true, true, true}, true},
		{State{true, true, false}, false},
		{State{true, false, true}, false},
		{State{false, true, true}, false},
		{State{false, false, false}, false},
	}
	for _, tc := range cases {
		if got := tc.state.Usable(); got != tc.want {
			t.Fatalf("Usable(%+v)=%v want %v", tc.state, got, tc.want)
		}
	}
}

func TestRegistryValidation(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(fakeProvider{name: ""}); err == nil || !strings.Contains(err.Error(), "name must not be empty") {
		t.Fatalf("empty name: %v", err)
	}
	if err := r.Register(fakeProvider{name: "amt"}); err != nil {
		t.Fatalf("register amt: %v", err)
	}
	if err := r.Register(fakeProvider{name: "amt"}); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate: %v", err)
	}
}

func TestListSorted(t *testing.T) {
	r := NewRegistry()
	for _, n := range []string{"redfish", "amt"} {
		if err := r.Register(fakeProvider{name: n}); err != nil {
			t.Fatalf("register %s: %v", n, err)
		}
	}
	list := r.List()
	if len(list) != 2 || list[0].Name() != "amt" || list[1].Name() != "redfish" {
		t.Fatalf("want sorted [amt redfish], got %v", list)
	}
}

func TestProbeAllKeepsErrorsPerPath(t *testing.T) {
	r := NewRegistry()
	boom := errors.New("management engine unreachable")
	if err := r.Register(fakeProvider{name: "amt", err: boom}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(fakeProvider{
		name: "dash",
		info: DeviceInfo{Provider: "dash", Capabilities: map[Capability]State{CapabilityPower: {true, true, true}}},
	}); err != nil {
		t.Fatal(err)
	}
	results := r.ProbeAll(context.Background(), Target{Address: "192.168.1.10"})
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	var sawErr, sawOK bool
	for _, res := range results {
		if res.Provider == "amt" {
			if !errors.Is(res.Err, boom) {
				t.Fatalf("amt error lost: %v", res.Err)
			}
			sawErr = true
		}
		if res.Provider == "dash" && res.Err == nil {
			sawOK = true
		}
	}
	if !sawErr || !sawOK {
		t.Fatalf("per-path isolation broken: err=%v ok=%v", sawErr, sawOK)
	}
}

func TestMergeArbitratesByRank(t *testing.T) {
	results := []ProbeResult{
		{Provider: "amt", Info: DeviceInfo{
			Architecture: "amd64",
			Capabilities: map[Capability]State{
				CapabilityPower:   {Supported: true, Enabled: true, Reachable: false},
				CapabilityConsole: {Supported: true},
			},
		}},
		{Provider: "redfish", Info: DeviceInfo{
			Capabilities: map[Capability]State{
				CapabilityPower: {Supported: true, Enabled: true, Reachable: true},
			},
		}},
	}
	view := Merge(results)
	power := view.Capabilities[CapabilityPower]
	if !power.State.Usable() || power.Source != "redfish" {
		t.Fatalf("power should arbitrate to usable via redfish: %+v", power)
	}
	console := view.Capabilities[CapabilityConsole]
	if console.State != (State{Supported: true}) || console.Source != "amt" {
		t.Fatalf("console should stay amt/supported-only: %+v", console)
	}
	if view.Architecture != "amd64" {
		t.Fatalf("architecture: %q", view.Architecture)
	}
	if len(view.Providers) != 2 || view.Providers[0] != "amt" || view.Providers[1] != "redfish" {
		t.Fatalf("providers: %v", view.Providers)
	}
}

func TestMergeKeepsUnknownCapabilitiesAndFirstWinsOnTie(t *testing.T) {
	weird := Capability("vendor-secret-thing")
	results := []ProbeResult{
		{Provider: "a", Info: DeviceInfo{Capabilities: map[Capability]State{
			CapabilityBoot: {Supported: true},
			weird:          {Supported: true, Enabled: true},
		}}},
		{Provider: "b", Info: DeviceInfo{Capabilities: map[Capability]State{
			CapabilityBoot: {Supported: true}, // 同秩 → 先到（a）保持 source
			weird:          {Supported: true, Enabled: false},
		}}},
	}
	view := Merge(results)
	// 全 false 态也必须原样入账（来源可溯），不得被丢弃；
	// 两路同名项才会触发 rank 比较，逼出 default 分支。
	results = append(results,
		ProbeResult{Provider: "a", Info: DeviceInfo{Capabilities: map[Capability]State{"vendor-off": {}}}},
		ProbeResult{Provider: "b", Info: DeviceInfo{Capabilities: map[Capability]State{"vendor-off": {}}}},
	)
	view = Merge(results)
	if v, ok := view.Capabilities["vendor-off"]; !ok || v.Source != "a" || v.State != (State{}) {
		t.Fatalf("all-false state must be preserved with source: %+v ok=%v", v, ok)
	}
	if v, ok := view.Capabilities[weird]; !ok || v.Source != "a" || !v.State.Enabled {
		t.Fatalf("unknown capability lost or overwritten: %+v", v)
	}
	if v := view.Capabilities[CapabilityBoot]; v.Source != "a" {
		t.Fatalf("tie should keep first (stable name order): %+v", v)
	}
}

func TestMergeReachableOnlyOutranksEnabledOnly(t *testing.T) {
	// {enabled,不reachable} 秩 2 < {reachable,不支持/未启用} 秩 3：可达性优先暴露。
	results := []ProbeResult{
		{Provider: "dash", Info: DeviceInfo{Capabilities: map[Capability]State{
			CapabilityRecovery: {Supported: true, Enabled: true, Reachable: false},
		}}},
		{Provider: "ipkvm", Info: DeviceInfo{Capabilities: map[Capability]State{
			CapabilityRecovery: {Supported: false, Enabled: false, Reachable: true},
		}}},
	}
	view := Merge(results)
	got := view.Capabilities[CapabilityRecovery]
	if got.Source != "ipkvm" || got.State != (State{Reachable: true}) || got.State.Usable() {
		t.Fatalf("reachable-only must outrank enabled-only: %+v", got)
	}
}

func TestMergeSkipsFailedPaths(t *testing.T) {
	results := []ProbeResult{
		{Provider: "dead", Err: errors.New("timeout")},
		{Provider: "alive", Info: DeviceInfo{Provider: "alive", Capabilities: map[Capability]State{CapabilityInventory: {true, true, true}}}},
	}
	view := Merge(results)
	if len(view.Providers) != 1 || view.Providers[0] != "alive" {
		t.Fatalf("failed path must not enter view: %v", view.Providers)
	}
}

func TestProbeAllRespectsConcurrentCompletion(t *testing.T) {
	r := NewRegistry()
	slow := delayedProvider{name: "slow", delay: 20 * time.Millisecond}
	if err := r.Register(slow); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(fakeProvider{name: "fast"}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	results := r.ProbeAll(context.Background(), Target{})
	elapsed := time.Since(start)
	if len(results) != 2 {
		t.Fatalf("results: %d", len(results))
	}
	if elapsed >= 40*time.Millisecond {
		t.Fatalf("probes did not run concurrently: %v", elapsed)
	}
}

type delayedProvider struct {
	name  string
	delay time.Duration
}

func (d delayedProvider) Name() string { return d.name }
func (d delayedProvider) Probe(ctx context.Context, _ Target) (DeviceInfo, error) {
	select {
	case <-time.After(d.delay):
		return DeviceInfo{Provider: d.name}, nil
	case <-ctx.Done():
		return DeviceInfo{}, ctx.Err()
	}
}

func TestProbeAllHonorsContextCancel(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(delayedProvider{name: "slow", delay: time.Hour}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	var results []ProbeResult
	go func() {
		defer wg.Done()
		results = r.ProbeAll(ctx, Target{})
	}()
	wg.Wait()
	if len(results) != 1 || !errors.Is(results[0].Err, context.DeadlineExceeded) {
		t.Fatalf("cancel must surface as ctx error: %+v", results)
	}
}
