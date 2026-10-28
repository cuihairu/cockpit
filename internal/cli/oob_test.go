package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/oob"
)

type probeFunc func(ctx context.Context, target oob.Target) (oob.DeviceInfo, error)

type probeProvider struct {
	name string
	fn   probeFunc
}

func (p probeProvider) Name() string { return p.name }
func (p probeProvider) Probe(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
	return p.fn(ctx, target)
}

func oobRegistry(providers ...oob.Provider) *oob.Registry {
	r := oob.NewRegistry()
	for _, p := range providers {
		r.MustRegister(p)
	}
	return r
}

func TestOobDiscoverRunRequiresAddress(t *testing.T) {
	cmd := &OobDiscoverCmd{Output: &bytes.Buffer{}}
	err := cmd.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "address required") {
		t.Fatalf("err = %v, want address required", err)
	}
}

func TestOobDiscoverRunPrintsPresentAndEvidence(t *testing.T) {
	registry := oobRegistry(probeProvider{name: "dash", fn: func(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
		return oob.DeviceInfo{Provider: "dash", Capabilities: map[oob.Capability]oob.State{}}, nil
	}})
	var buf bytes.Buffer
	cmd := &OobDiscoverCmd{Address: "192.168.1.10", Output: &buf, registry: registry}

	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"OOB discovery for 192.168.1.10", "Provider dash: Present", "Evidence: provider discovery response"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestOobDiscoverRunPrintsNotPresent(t *testing.T) {
	registry := oobRegistry(probeProvider{name: "amt", fn: func(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
		return oob.DeviceInfo{}, errors.New("amt: wsman endpoint is not AMT (product \"foo\")")
	}})
	var buf bytes.Buffer
	cmd := &OobDiscoverCmd{Address: "192.168.1.10", Output: &buf, registry: registry}

	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Provider amt: NotPresent") {
		t.Fatalf("output missing NotPresent:\n%s", out)
	}
}

func TestOobDiscoverRunPrintsUnknown(t *testing.T) {
	registry := oobRegistry(probeProvider{name: "redfish", fn: func(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
		return oob.DeviceInfo{}, errors.New("redfish: dial tcp: connection refused")
	}})
	var buf bytes.Buffer
	cmd := &OobDiscoverCmd{Address: "192.168.1.10", Output: &buf, registry: registry}

	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Provider redfish: Unknown") {
		t.Fatalf("output missing Unknown:\n%s", out)
	}
}

func TestOobDiscoverRunPrintsCapabilitiesAndOmitsMissing(t *testing.T) {
	registry := oobRegistry(
		probeProvider{name: "redfish", fn: func(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
			return oob.DeviceInfo{Provider: "redfish", Capabilities: map[oob.Capability]oob.State{
				oob.CapabilityPower: {Supported: true, Enabled: true, Reachable: true},
				oob.CapabilityBoot:  {Supported: true, Enabled: true, Reachable: false},
			}}, nil
		}},
		probeProvider{name: "dash", fn: func(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
			return oob.DeviceInfo{Provider: "dash", Capabilities: map[oob.Capability]oob.State{}}, nil
		}},
	)
	var buf bytes.Buffer
	cmd := &OobDiscoverCmd{Address: "192.168.1.10", Output: &buf, registry: registry}

	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Capabilities",
		"power: supported=true enabled=true reachable=true source=redfish",
		"boot: supported=true enabled=true reachable=false source=redfish",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestOobDiscoverRunNoCapabilities(t *testing.T) {
	var buf bytes.Buffer
	cmd := &OobDiscoverCmd{Address: "192.168.1.10", Output: &buf, registry: oobRegistry(
		probeProvider{name: "dash", fn: func(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
			return oob.DeviceInfo{Provider: "dash", Capabilities: map[oob.Capability]oob.State{}}, nil
		}},
	)}
	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), "none") {
		t.Fatalf("expected no capabilities line, got:\n%s", buf.String())
	}
}

func TestOobPresenceClassification(t *testing.T) {
	cases := []struct {
		name       string
		result     oob.ProbeResult
		wantState  string
		wantHasErr bool
	}{
		{"ok", oob.ProbeResult{Provider: "dash", Info: oob.DeviceInfo{Provider: "dash"}}, "Present", false},
		{"rejected", oob.ProbeResult{Provider: "dash", Err: errors.New("dash: wsman endpoint is not DASH")}, "NotPresent", true},
		{"not-redfish", oob.ProbeResult{Provider: "redfish", Err: errors.New("redfish: not a redfish service (root status 404)")}, "NotPresent", true},
		{"transport", oob.ProbeResult{Provider: "redfish", Err: errors.New("redfish: dial tcp: i/o timeout")}, "Unknown", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, evidence := oobPresence(tc.result)
			if state != tc.wantState {
				t.Fatalf("state = %q, want %q", state, tc.wantState)
			}
			if (evidence != "") != tc.wantHasErr && tc.wantHasErr {
				t.Fatalf("expected error evidence, got %q", evidence)
			}
		})
	}
}

func TestOobDashQueryRunRequiresAddress(t *testing.T) {
	cmd := &OobDashQueryCmd{Output: &bytes.Buffer{}}
	err := cmd.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "address required") {
		t.Fatalf("err = %v, want address required", err)
	}
}

func TestOobDashQueryRunReusesPresenceSemantics(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantState  string
		wantDetail string
	}{
		{
			name:       "present",
			err:        nil,
			wantState:  "Present",
			wantDetail: "Evidence: provider discovery response",
		},
		{
			name:       "not-present",
			err:        errors.New("dash: wsman endpoint is not DASH (product \"Intel AMT\")"),
			wantState:  "NotPresent",
			wantDetail: "Evidence: dash: wsman endpoint is not DASH",
		},
		{
			name:       "unknown",
			err:        errors.New("dash: dial tcp: i/o timeout"),
			wantState:  "Unknown",
			wantDetail: "Evidence: dash: dial tcp: i/o timeout",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registry := oobRegistry(probeProvider{name: "dash", fn: func(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
				if tc.err != nil {
					return oob.DeviceInfo{}, tc.err
				}
				return oob.DeviceInfo{Provider: "dash", Capabilities: map[oob.Capability]oob.State{}}, nil
			}})
			var buf bytes.Buffer
			cmd := &OobDashQueryCmd{Address: "192.168.1.10", Output: &buf, registry: registry}
			if err := cmd.Run(context.Background()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			out := buf.String()
			for _, want := range []string{
				"DASH direct query for 192.168.1.10",
				"Provider dash: " + tc.wantState,
				tc.wantDetail,
			} {
				if !strings.Contains(out, want) {
					t.Fatalf("output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestOobDashQueryRunUsesDefaultRegistryAndOutput(t *testing.T) {
	// 本机未听端口快速失败，生产注册表 + stdout 缺省路径都被覆盖。
	cmd := &OobDashQueryCmd{Address: "127.0.0.1:1", Timeout: time.Second}
	oldStdout := os.Stdout
	outputPath := filepath.Join(t.TempDir(), "stdout")
	output, err := os.Create(outputPath)
	if err != nil {
		t.Fatalf("create stdout: %v", err)
	}
	os.Stdout = output
	err = cmd.Run(context.Background())
	os.Stdout = oldStdout
	if closeErr := output.Close(); closeErr != nil {
		t.Fatalf("close stdout: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	raw, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	out := string(raw)
	if !strings.Contains(out, "Provider dash: Unknown") {
		t.Fatalf("expected DASH Unknown on closed port:\n%s", out)
	}
}

func TestOobDashQueryRunUnregisteredProvider(t *testing.T) {
	// 注入的注册表不含 dash（不应发生，但失败要诚实上账，不吞）。
	var buf bytes.Buffer
	cmd := &OobDashQueryCmd{Address: "192.168.1.10", Output: &buf, registry: oobRegistry()}
	err := cmd.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("err = %v, want not registered", err)
	}
}

func TestOobDiscoverDefaultTimeoutAndOutput(t *testing.T) {
	cmd := &OobDiscoverCmd{Address: "192.168.1.10"}
	if got := cmd.Timeout; got != 0 {
		t.Fatalf("zero timeout must default at Run, got %v", got)
	}
	var buf bytes.Buffer
	cmd = &OobDiscoverCmd{Address: "x", registry: oobRegistry(
		probeProvider{name: "dash", fn: func(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
			return oob.DeviceInfo{Provider: "dash"}, nil
		}},
	)}
	start := time.Now()

	outputPath := filepath.Join(t.TempDir(), "stdout")
	output, err := os.Create(outputPath)
	if err != nil {
		t.Fatalf("create stdout file: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = output
	err = cmd.Run(context.Background())
	os.Stdout = oldStdout
	if closeErr := output.Close(); closeErr != nil {
		t.Fatalf("close stdout file: %v", closeErr)
	}
	raw, readErr := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if readErr != nil {
		t.Fatalf("read stdout file: %v", readErr)
	}
	buf.Write(raw)
	if got := time.Since(start); got > 2*time.Second {
		t.Fatalf("run took too long with injected registry: %v", got)
	}
}

func TestOobDiscoverUsesDefaultRegistryAndOutput(t *testing.T) {
	// 未注入 registry/Output 时走生产注册表与 stdout 路径；目标选本机未听
	// 端口（连接拒绝快速失败），三路都应为 Unknown，不猜。
	var buf bytes.Buffer
	cmd := &OobDiscoverCmd{Address: "127.0.0.1:1", Timeout: time.Second, Output: &buf}
	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Provider amt: Unknown") ||
		!strings.Contains(out, "Provider dash: Unknown") ||
		!strings.Contains(out, "Provider redfish: Unknown") {
		t.Fatalf("expected all providers Unknown on closed port:\n%s", out)
	}
}

func TestNewOobDiscoverRegistryHasProviders(t *testing.T) {
	registry := newOobDiscoverRegistry(time.Second)
	providers := registry.List()
	got := make([]string, 0, len(providers))
	for _, p := range providers {
		got = append(got, p.Name())
	}
	want := "amt,dash,redfish"
	str := strings.Join(got, ",")
	if str != want {
		t.Fatalf("providers = %q, want %q", str, want)
	}
}
