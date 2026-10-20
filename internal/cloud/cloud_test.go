package cloud

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fullMock 实现全部接口，用于正例。
type fullMock struct {
	vendor string
	caps   []Capability
}

func (m fullMock) Identity() ProviderIdentity {
	return ProviderIdentity{Vendor: m.vendor, DisplayName: m.vendor}
}
func (m fullMock) Capabilities() []Capability { return m.caps }
func (m fullMock) ListInstances(context.Context) ([]Instance, error) {
	return nil, nil
}
func (m fullMock) CreateInstance(context.Context, InstanceSpec) (Operation, error) {
	return Operation{}, nil
}
func (m fullMock) StartInstance(context.Context, string) (Operation, error) {
	return Operation{}, nil
}
func (m fullMock) StopInstance(context.Context, string) (Operation, error) {
	return Operation{}, nil
}
func (m fullMock) DeleteInstance(context.Context, string) (Operation, error) {
	return Operation{}, nil
}
func (m fullMock) GetOperation(context.Context, Operation) (Operation, error) {
	return Operation{}, nil
}
func (m fullMock) QueryCosts(context.Context, CostQuery) ([]CostRecord, error) {
	return nil, nil
}
func (m fullMock) QueryQuotas(context.Context, string) ([]Quota, error) {
	return nil, nil
}

// baseOnly 只实现 Provider，用于「声明了能力但没实现」的反例。
type baseOnly struct {
	vendor string
	caps   []Capability
}

func (m baseOnly) Identity() ProviderIdentity {
	return ProviderIdentity{Vendor: m.vendor}
}
func (m baseOnly) Capabilities() []Capability { return m.caps }

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	p := fullMock{vendor: "aliyun", caps: []Capability{CapabilityCompute, CapabilityBilling, CapabilityQuota}}
	if err := r.Register(p); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, ok := r.Get("aliyun")
	if !ok || got.Identity().Vendor != "aliyun" {
		t.Fatalf("get aliyun: ok=%v provider=%v", ok, got)
	}
	if _, ok := r.Get("tencentcloud"); ok {
		t.Fatal("get unknown vendor should miss")
	}
}

func TestRegisterRejectsEmptyVendor(t *testing.T) {
	r := NewRegistry()
	err := r.Register(fullMock{vendor: "", caps: nil})
	if err == nil || !strings.Contains(err.Error(), "vendor must not be empty") {
		t.Fatalf("want empty-vendor error, got %v", err)
	}
}

func TestRegisterRejectsDuplicateVendor(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(fullMock{vendor: "aliyun"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := r.Register(fullMock{vendor: "aliyun"})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("want duplicate error, got %v", err)
	}
}

func TestRegisterRejectsDeclarationWithoutImplementation(t *testing.T) {
	cases := []struct {
		capability Capability
		wantText   string
	}{
		{CapabilityCompute, "without implementing ComputeProvider"},
		{CapabilityBilling, "without implementing BillingProvider"},
		{CapabilityQuota, "without implementing QuotaProvider"},
	}
	for _, tc := range cases {
		r := NewRegistry()
		err := r.Register(baseOnly{vendor: "mock", caps: []Capability{tc.capability}})
		if err == nil || !strings.Contains(err.Error(), tc.wantText) {
			t.Fatalf("capability %s: want %q error, got %v", tc.capability, tc.wantText, err)
		}
	}
}

func TestRegisterRejectsUnknownCapability(t *testing.T) {
	r := NewRegistry()
	err := r.Register(baseOnly{vendor: "mock", caps: []Capability{"ml"}})
	if err == nil || !strings.Contains(err.Error(), "unknown capability") {
		t.Fatalf("want unknown-capability error, got %v", err)
	}
}

func TestListSortedByVendor(t *testing.T) {
	r := NewRegistry()
	for _, v := range []string{"tencentcloud", "aliyun"} {
		if err := r.Register(fullMock{vendor: v}); err != nil {
			t.Fatalf("register %s: %v", v, err)
		}
	}
	list := r.List()
	if len(list) != 2 || list[0].Identity().Vendor != "aliyun" || list[1].Identity().Vendor != "tencentcloud" {
		t.Fatalf("want [aliyun tencentcloud], got %v", list)
	}
}

func TestSupports(t *testing.T) {
	p := fullMock{vendor: "aliyun", caps: []Capability{CapabilityCompute}}
	if !Supports(p, CapabilityCompute) {
		t.Fatal("declared capability should be supported")
	}
	if Supports(p, CapabilityBilling) {
		t.Fatal("undeclared capability should not be supported")
	}
}

func TestErrNotSupportedSentinel(t *testing.T) {
	wrapped := fmt.Errorf("aliyun eip: %w", ErrNotSupported)
	if !errors.Is(wrapped, ErrNotSupported) {
		t.Fatal("ErrNotSupported must remain identifiable through wrapping")
	}
}

func TestNewOperation(t *testing.T) {
	now := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	op := NewOperation(OperationCreate, "req-1", now)
	if op.ID != "create-req-1" {
		t.Fatalf("id: got %q", op.ID)
	}
	if op.Status != OperationPending || op.Kind != OperationCreate || op.VendorRequestID != "req-1" {
		t.Fatalf("unexpected op: %+v", op)
	}
	if !op.UpdatedAt.Equal(now) || op.IsTerminal() {
		t.Fatalf("timestamp/terminal wrong: %+v", op)
	}
}

func TestOperationHappyPath(t *testing.T) {
	t0 := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Minute)
	t2 := t1.Add(time.Minute)
	op := NewOperation(OperationCreate, "req-2", t0)
	running, err := op.WithStatus(OperationRunning, t1)
	if err != nil || running.Status != OperationRunning || !running.UpdatedAt.Equal(t1) {
		t.Fatalf("pending→running: op=%+v err=%v", running, err)
	}
	done, err := running.MarkSucceeded("i-abc", t2)
	if err != nil || done.Status != OperationSucceeded || done.NativeID != "i-abc" {
		t.Fatalf("running→succeeded: op=%+v err=%v", done, err)
	}
	if !done.IsTerminal() {
		t.Fatal("succeeded must be terminal")
	}
}

func TestOperationFailureKeepsVendorError(t *testing.T) {
	t0 := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	op := NewOperation(OperationStop, "req-3", t0)
	running, err := op.WithStatus(OperationRunning, t0.Add(time.Second))
	if err != nil {
		t.Fatalf("to running: %v", err)
	}
	failed, err := running.MarkFailed("OperationDenied.QuotaRunningInstances", t0.Add(2*time.Second))
	if err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if failed.Status != OperationFailed || failed.Error != "OperationDenied.QuotaRunningInstances" {
		t.Fatalf("vendor error lost: %+v", failed)
	}
	if failed.VendorRequestID != "req-3" {
		t.Fatalf("vendor request id lost: %+v", failed)
	}
	if !failed.IsTerminal() {
		t.Fatal("failed must be terminal")
	}
}

func TestOperationRejectsTerminalDrive(t *testing.T) {
	now := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	succeeded := Operation{ID: "op", Status: OperationSucceeded, UpdatedAt: now}
	if _, err := succeeded.WithStatus(OperationRunning, now); err == nil || !strings.Contains(err.Error(), "terminal state") {
		t.Fatalf("terminal drive must fail, got %v", err)
	}
	failed := Operation{ID: "op2", Status: OperationFailed, UpdatedAt: now}
	if _, err := failed.MarkFailed("again", now); err == nil {
		t.Fatal("failed.MarkFailed on terminal must fail")
	}
	if _, err := succeeded.MarkSucceeded("x", now); err == nil {
		t.Fatal("succeeded.MarkSucceeded on terminal must fail")
	}
}

func TestOperationRejectsInvalidTransitions(t *testing.T) {
	now := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	pending := NewOperation(OperationStart, "req-4", now)
	if _, err := pending.MarkSucceeded("i-1", now); err == nil || !strings.Contains(err.Error(), "cannot transition") {
		t.Fatalf("pending→succeeded must fail, got %v", err)
	}
	if _, err := pending.MarkFailed("boom", now); err == nil {
		t.Fatal("pending→failed must fail")
	}
	running, _ := pending.WithStatus(OperationRunning, now)
	if _, err := running.WithStatus(OperationRunning, now); err == nil {
		t.Fatal("running→running must fail")
	}
	weird := Operation{ID: "op5", Status: "weird", UpdatedAt: now}
	if _, err := weird.WithStatus(OperationRunning, now); err == nil {
		t.Fatal("unknown source status must fail (default branch)")
	}
}
