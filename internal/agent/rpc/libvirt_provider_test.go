package rpc

import (
	"os/exec"

	"context"
	"errors"
	"strings"
	"testing"
)

// fakeVirtRun routes a virsh invocation by the verb it sees and returns
// canned bytes or a controlled error. It records the exact argv so tests
// can assert the read-only `-r` flag is always present.
func fakeVirtRun(t *testing.T, script map[string]func(args []string, stderr *strings.Builder) (string, error)) func(context.Context, string, ...string) ([]byte, []byte, error) {
	t.Helper()
	return func(ctx context.Context, bin string, args ...string) ([]byte, []byte, error) {
		// Identify the subcommand: skip global options, and skip the value of
		// a `-c` connection option so `-r -c lxc:/// capabilities` resolves
		// the verb to `capabilities`, not `lxc:///`.
		verb := ""
		for i := 0; i < len(args); i++ {
			if args[i] == "-c" {
				i++ // skip connection URI
				continue
			}
			if !strings.HasPrefix(args[i], "-") {
				verb = args[i]
				break
			}
		}
		stderr := &strings.Builder{}
		body := script[verb]
		if body == nil {
			return nil, nil, errors.New("unexpected verb: " + verb)
		}
		out, err := body(args, stderr)
		return []byte(out), []byte(stderr.String()), err
	}
}

func TestLibvirtProviderType(t *testing.T) {
	if got := NewLibvirtProvider("", nil).Type(); got != "virt" {
		t.Fatalf("Type() = %q, want virt", got)
	}
}

func TestLibvirtCapabilitiesBothDrivers(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"capabilities": func(args []string, _ *strings.Builder) (string, error) {
			if !hasFlag(args, "-r") {
				t.Errorf("capabilities missing read-only -r: %v", args)
			}
			return `<capabilities><domain type='kvm'></domain><domain type='lxc'></domain></capabilities>`, nil
		},
		"uri": func(args []string, _ *strings.Builder) (string, error) {
			if !hasFlag(args, "-r") {
				t.Errorf("uri missing read-only -r: %v", args)
			}
			return "qemu:///system\n", nil
		},
	})
	out, err := NewLibvirtProvider("virsh", run).Capabilities(nil)
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if out["available"] != true {
		t.Fatalf("available = %v, want true", out["available"])
	}
	if out["driver"] != "qemu" {
		t.Fatalf("driver = %v, want qemu", out["driver"])
	}
	if out["hypervisor"] != true || out["lxc"] != true {
		t.Fatalf("driver presence = %+v, want hypervisor+lxc", out)
	}
}

func TestLibvirtCapabilitiesHypervisorOnly(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"capabilities": func(args []string, _ *strings.Builder) (string, error) {
			return `<capabilities><domain type='qemu'></domain></capabilities>`, nil
		},
		"uri": func(args []string, _ *strings.Builder) (string, error) { return "qemu:///system\n", nil },
	})
	out, err := NewLibvirtProvider("virsh", run).Capabilities(nil)
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if out["hypervisor"] != true || out["lxc"] != false {
		t.Fatalf("presence = %+v, want hypervisor only", out)
	}
}

func TestLibvirtCapabilitiesDaemonUnavailable(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"capabilities": func(args []string, stderr *strings.Builder) (string, error) {
			stderr.WriteString("failed to connect to hypervisor")
			return "", errors.New("exit status 1")
		},
	})
	out, err := NewLibvirtProvider("virsh", run).Capabilities(nil)
	if err != nil {
		t.Fatalf("daemon-down must not be a probe error: %v", err)
	}
	if out["available"] != false {
		t.Fatalf("available = %v, want false", out["available"])
	}
	if got := out["error"]; got != "failed to connect to hypervisor" {
		t.Fatalf("error detail = %v, want preserved stderr", got)
	}
}

func TestLibvirtCapabilitiesDaemonErrorNoStderr(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"capabilities": func(args []string, _ *strings.Builder) (string, error) {
			return "", errors.New("exit status 1")
		},
	})
	out, err := NewLibvirtProvider("virsh", run).Capabilities(nil)
	if err != nil {
		t.Fatalf("no-stderr daemon down must not error: %v", err)
	}
	if out["available"] != false {
		t.Fatalf("available = %v, want false", out["available"])
	}
	if got := out["error"]; got != "exit status 1" {
		t.Fatalf("error = %v, want fallback to exit status 1", got)
	}
}

func TestLibvirtCapabilitiesProbeErrorPreservesStderr(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"capabilities": func(args []string, _ *strings.Builder) (string, error) {
			return `<capabilities><domain type='kvm'></domain></capabilities>`, nil
		},
		"uri": func(args []string, _ *strings.Builder) (string, error) {
			return "", errors.New("exit status 1")
		},
	})
	out, err := NewLibvirtProvider("virsh", run).Capabilities(nil)
	if err != nil {
		t.Fatalf("uri failure must not fail capabilities: %v", err)
	}
	if _, ok := out["uri"]; ok {
		t.Fatalf("uri must be omitted on best-effort failure: %+v", out)
	}
	if out["hypervisor"] != true {
		t.Fatalf("hypervisor = %v, want true from capabilities", out["hypervisor"])
	}
}

func TestLibvirtListVMSAndContainers(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"list": func(args []string, _ *strings.Builder) (string, error) {
			if !hasFlag(args, "-r") {
				t.Errorf("list missing read-only -r: %v", args)
			}
			if hasFlag(args, "-c") {
				t.Errorf("vms.list must not pass -c (default qemu connection): %v", args)
			}
			return "web-01\ndb-02\n", nil
		},
	})
	p := NewLibvirtProvider("virsh", run)
	out, err := p.Call("vms.list", nil)
	if err != nil {
		t.Fatalf("vms.list: %v", err)
	}
	// Shape is a flat name list, not the PVE {vms: []} object (audit finding).
	vms, ok := out.([]string)
	if !ok {
		t.Fatalf("vms.list shape = %T, want []string", out)
	}
	if len(vms) != 2 || vms[0] != "web-01" || vms[1] != "db-02" {
		t.Fatalf("vms = %v", vms)
	}
}

func TestLibvirtContainersUseLXCConnection(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"list": func(args []string, _ *strings.Builder) (string, error) {
			if !hasFlag(args, "-r") {
				t.Errorf("list missing read-only -r: %v", args)
			}
			if !hasFlag(args, "-c") || !hasFlag(args, "lxc:///") {
				t.Errorf("containers.list must target lxc:/// connection: %v", args)
			}
			return "ct-1\n", nil
		},
	})
	out, err := NewLibvirtProvider("virsh", run).Call("containers.list", nil)
	if err != nil {
		t.Fatalf("containers.list: %v", err)
	}
	cts, ok := out.([]string)
	if !ok || len(cts) != 1 || cts[0] != "ct-1" {
		t.Fatalf("containers = %#v, want [ct-1]", out)
	}
}

func TestLibvirtCapabilitiesProbesLXCConnection(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"capabilities": func(args []string, _ *strings.Builder) (string, error) {
			// Default connection: qemu only, no lxc driver advertised.
			if hasFlag(args, "-c") {
				return `<capabilities><domain type='lxc'></domain></capabilities>`, nil
			}
			return `<capabilities><domain type='qemu'></domain></capabilities>`, nil
		},
		"uri": func(args []string, _ *strings.Builder) (string, error) { return "qemu:///system\n", nil },
	})
	out, err := NewLibvirtProvider("virsh", run).Capabilities(nil)
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	// lxc must come from the dedicated connection probe, not the default.
	if out["lxc"] != true {
		t.Fatalf("lxc = %v, want true from lxc:/// probe", out["lxc"])
	}
	if out["hypervisor"] != true {
		t.Fatalf("hypervisor = %v, want true from default probe", out["hypervisor"])
	}
}

func TestLibvirtCapabilitiesLXCProbeFailsHonest(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"capabilities": func(args []string, _ *strings.Builder) (string, error) {
			if hasFlag(args, "-c") {
				return "", errors.New("exit status 1")
			}
			return `<capabilities><domain type='qemu'></domain></capabilities>`, nil
		},
		"uri": func(args []string, _ *strings.Builder) (string, error) { return "qemu:///system\n", nil },
	})
	out, err := NewLibvirtProvider("virsh", run).Capabilities(nil)
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if out["available"] != true {
		t.Fatalf("available = %v, want true (default probe OK)", out["available"])
	}
	if out["lxc"] != false {
		t.Fatalf("lxc = %v, want false when lxc:/// probe fails", out["lxc"])
	}
}

func TestLibvirtListEmptyIsNotNil(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"list": func(args []string, _ *strings.Builder) (string, error) { return "\n", nil },
	})
	out, err := NewLibvirtProvider("virsh", run).VMS(nil)
	if err != nil {
		t.Fatalf("VMS: %v", err)
	}
	vms, ok := out.([]string)
	if !ok {
		t.Fatalf("VMS shape = %T, want []string", out)
	}
	if vms == nil || len(vms) != 0 {
		t.Fatalf("vms = %#v, want empty non-nil slice", vms)
	}
}

func TestLibvirtListErrorKeepsDetail(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"list": func(args []string, stderr *strings.Builder) (string, error) {
			stderr.WriteString("error: failed to get domain list")
			return "", errors.New("exit status 1")
		},
	})
	_, err := NewLibvirtProvider("virsh", run).VMS(nil)
	if err == nil || !strings.Contains(err.Error(), "failed to get domain list") {
		t.Fatalf("err = %v, want stderr detail", err)
	}
}

func TestLibvirtCallUnknownAction(t *testing.T) {
	_, err := NewLibvirtProvider("virsh", nil).Call("vms.start", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown virt action") {
		t.Fatalf("err = %v, want unknown action (no write verbs in slice 1)", err)
	}
}

func TestLibvirtDriverFromURI(t *testing.T) {
	cases := map[string]string{
		"qemu:///system": "qemu",
		"lxc:///system":  "lxc",
		"":               "",
		"bare":           "bare",
	}
	for in, want := range cases {
		if got := libvirtDriverFromURI(in); got != want {
			t.Errorf("libvirtDriverFromURI(%q) = %q, want %q", in, got, want)
		}
	}
}

// hasFlag reports whether args contains value (a plain flag token).
func hasFlag(args []string, value string) bool {
	for _, a := range args {
		if a == value {
			return true
		}
	}
	return false
}

func TestLibvirtAvailableSeams(t *testing.T) {
	old, oldGoos := libvirtLookPath, libvirtGOOS
	cleanup := func() { libvirtLookPath, libvirtGOOS = old, oldGoos }
	t.Cleanup(cleanup)

	// 1. 非 Linux：LookPath 存在也不认。
	libvirtGOOS = "windows"
	if LibvirtAvailable() {
		t.Fatal("non-Linux must not report virsh available")
	}

	// 2. Linux + LookPath 命中：认入口存在。
	libvirtGOOS = "linux"
	libvirtLookPath = func(string) (string, error) { return "/usr/bin/virsh", nil }
	if !LibvirtAvailable() {
		t.Fatal("Linux + virsh in PATH must report available")
	}

	// 3. Linux + LookPath 未命中：不认（不猜 daemon）。
	libvirtLookPath = func(string) (string, error) { return "", errors.New("not found") }
	if LibvirtAvailable() {
		t.Fatal("Linux without virsh must not report available")
	}
}

// TestLibvirtDefaultRunClosure covers the production process seam body by
// swapping execCommandContext (package var) to a trivial command; the same
// injection pattern is used elsewhere in this package.
func TestLibvirtDefaultRunClosure(t *testing.T) {
	old := execCommandContext
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "printf", "qemu:///system")
	}
	t.Cleanup(func() { execCommandContext = old })

	p := NewLibvirtProvider("", nil)
	out, err := p.Capabilities(nil)
	if err != nil {
		t.Fatalf("Capabilities via default closure: %v", err)
	}
	if out["available"] != true {
		t.Fatalf("available = %v, want true", out["available"])
	}
}

func TestLibvirtCallCapabilitiesDispatch(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"version": func(args []string, _ *strings.Builder) (string, error) { return "qemu:///system\n", nil },
		"capabilities": func(args []string, _ *strings.Builder) (string, error) {
			return `<capabilities><domain type='kvm'></domain></capabilities>`, nil
		},
	})
	out, err := NewLibvirtProvider("virsh", run).Call("capabilities", nil)
	if err != nil {
		t.Fatalf("Call(capabilities): %v", err)
	}
	if m := out.(map[string]interface{}); m["available"] != true || m["hypervisor"] != true {
		t.Fatalf("Call(capabilities) = %+v", m)
	}
}

func TestLibvirtContainersListError(t *testing.T) {
	run := fakeVirtRun(t, map[string]func([]string, *strings.Builder) (string, error){
		"list": func(args []string, _ *strings.Builder) (string, error) {
			return "", errors.New("exit status 1")
		},
	})
	_, err := NewLibvirtProvider("virsh", run).Call("containers.list", nil)
	if err == nil || !strings.Contains(err.Error(), "list domains") {
		t.Fatalf("err = %v, want wrap without stderr detail", err)
	}
}
