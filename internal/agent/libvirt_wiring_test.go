package agent

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/agent/rpc"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// TestLibvirtAvailableFalseWithoutPath probes the fallback branch. The CI
// image has no virsh binary, so restoring PATH afterwards is safe.
func TestLibvirtAvailableFalseWithoutPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if rpc.LibvirtAvailable() {
		t.Fatal("LibvirtAvailable() = true with no virsh on PATH")
	}
}

// TestSetupProvidersLibvirtRegistered covers the capability gate and the
// provider registration. A PATH stub makes LibvirtAvailable deterministic;
// no process is spawned during setupProviders.
func TestSetupProvidersLibvirtRegistered(t *testing.T) {
	fakeBinDirCov(t, map[string]string{"virsh": "exit 0\n"})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "virt-libvirt"}}
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), baseTypes("virt"))
}

// TestSetupProvidersLibvirtSkippedWithoutBinary covers the honest
// no-capability branch: an installed-but-missing virsh must not register a
// provider that cannot answer.
func TestSetupProvidersLibvirtSkippedWithoutBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "virt-libvirt"}}
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), baseTypes())
}

// TestDetectCapabilitiesLibvirtCapability covers the capability append. It
// reuses the same PATH stub and asserts the metadata contract only.
func TestDetectCapabilitiesLibvirtCapability(t *testing.T) {
	fakeBinDirCov(t, map[string]string{"virsh": "exit 0\n"})

	a := NewAgent(Config{ServerURL: "ws://test"})
	var found bool
	for _, cap := range a.detectCapabilities() {
		if cap.Type == "virt-libvirt" {
			found = true
			if v, _ := cap.Metadata["read_only"].(bool); !v {
				t.Fatalf("virt-libvirt metadata = %+v, want read_only=true", cap.Metadata)
			}
		}
	}
	if !found {
		t.Fatal("virt-libvirt capability missing")
	}
}
