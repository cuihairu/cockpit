package rpc

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// libvirtProbeTimeout / libvirtListTimeout bounds each virsh invocation.
// Capability discovery parses an XML document that stays resident on the
// same host, so a short default is enough; listing domains can be slower
// on loaded hosts but is still bounded to keep agent RPC responsive.
const (
	libvirtProbeTimeout = 5 * time.Second
	libvirtListTimeout  = 10 * time.Second
	// libvirtLXCURI is the dedicated LXC connection URI. `virsh capabilities`
	// on the default qemu:///system connection reports only the qemu/kvm
	// domain driver; the independent LXC driver is only visible when the
	// probe targets its own connection (audit finding: default URI misses
	// the lxc:/// driver and misreports LXC as unsupported).
	libvirtLXCURI = "lxc:///"
)

// libvirtRun is the process execution seam. Production runs `virsh -r`
// (read-only connection); tests inject fake output without spawning a
// process.
var libvirtRun = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := execCommandContext(ctx, name, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return []byte(stdout.String()), []byte(stderr.String()), err
}

// libvirtLookPath / libvirtGOOS are coverage seams (same pattern as the
// firewall detector): production values are the real LookPath and GOOS, so
// neither a virsh binary nor a non-Linux host is required in CI.
var (
	libvirtLookPath = exec.LookPath
	libvirtGOOS     = runtime.GOOS
)

// LibvirtProvider is the read-only slice of the agent-side libvirt slot.
// Every virsh invocation carries `-r` so the connection itself cannot
// mutate state even if a future maintainer adds a query verb; there is no
// write action in this slice by design (todo agent libvirt slice 1).
type LibvirtProvider struct {
	bin string
	run func(context.Context, string, ...string) ([]byte, []byte, error)
}

// NewLibvirtProvider creates the read-only provider. nil run falls back to
// the process seam; bin defaults to "virsh".
func NewLibvirtProvider(bin string, run func(context.Context, string, ...string) ([]byte, []byte, error)) *LibvirtProvider {
	if bin == "" {
		bin = "virsh"
	}
	if run == nil {
		run = libvirtRun
	}
	return &LibvirtProvider{bin: bin, run: run}
}

// LibvirtAvailable reports whether the read-only virsh entry point exists
// on this host. Non-Linux hosts never report available.
func LibvirtAvailable() bool {
	if libvirtGOOS != "linux" {
		return false
	}
	_, err := libvirtLookPath("virsh")
	return err == nil
}

// Type returns the RPC provider prefix (virt.<action>).
func (p *LibvirtProvider) Type() string { return "virt" }

// Call dispatches read-only actions. Unknown verbs fail closed; the
// registry only forwards capability and list reads in this slice.
func (p *LibvirtProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "capabilities":
		return p.Capabilities(params)
	case "vms.list":
		return p.VMS(params)
	case "containers.list":
		return p.Containers(params)
	}
	return nil, fmt.Errorf("unknown virt action: %s", action)
}

// Capabilities probes the local libvirt daemon and classifies drivers
// visible in `virsh capabilities`. The default connection (qemu:///system)
// is probed first, then the dedicated LXC connection so an independent
// lxc:/// driver is never misreported as unsupported. Only daemon-verified
// evidence is reported: no driver is inferred from the virsh binary being
// installed.
func (p *LibvirtProvider) Capabilities(params map[string]interface{}) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), libvirtProbeTimeout)
	defer cancel()

	// `virsh capabilities` is the single authoritative read probe: it
	// connects to the daemon AND returns the full capability XML. A failed
	// connection (daemon down, no driver, socket missing) is reported as
	// available=false with the transport detail preserved.
	capsXML, stderr, err := p.run(ctx, p.bin, "-r", "capabilities")
	if err != nil {
		// available=false is data, not a transport failure: return it with the
		// preserved detail so the RPC handler does not drop it on the error
		// path (audit finding: Handler discards result when err != nil).
		detail := strings.TrimSpace(string(stderr))
		if detail == "" {
			detail = err.Error()
		}
		return map[string]interface{}{"available": false, "error": detail}, nil
	}

	out := map[string]interface{}{"available": true}
	hypervisor, lxc := libvirtDriversFromCapabilities(string(capsXML))
	// Probe the dedicated LXC connection: the default qemu URI does not list
	// the lxc driver, so without this a host with only an lxc:/// driver
	// would be misreported as no-LXC. A failed lxc probe is honest evidence
	// that the driver is absent (lxc stays false), not a probe failure.
	if lxcXML, _, lxcErr := p.run(ctx, p.bin, "-r", "-c", libvirtLXCURI, "capabilities"); lxcErr == nil {
		_, lxcSeen := libvirtDriversFromCapabilities(string(lxcXML))
		lxc = lxc || lxcSeen
	}
	out["hypervisor"] = hypervisor
	out["lxc"] = lxc

	// URI/driver are best-effort metadata: if `uri` cannot be resolved the
	// capability verdict above stands; we never infer a driver from it.
	if uriOut, _, uriErr := p.run(ctx, p.bin, "-r", "uri"); uriErr == nil {
		uri := strings.TrimSpace(string(uriOut))
		out["uri"] = uri
		out["driver"] = libvirtDriverFromURI(uri)
	}
	return out, nil
}

// libvirtDriverFromURI maps the connection URI scheme to a short driver
// label. Unknown schemes are echoed verbatim without guessing.
func libvirtDriverFromURI(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}
	if i := strings.Index(uri, "://"); i > 0 {
		return uri[:i]
	}
	return uri
}

// libvirtDriversFromCapabilities extracts domain driver types from the
// capabilities XML without pulling in a libvirt binding dependency.
var libvirtDomainType = regexp.MustCompile(`<domain\s+type=['"]([^'"]+)['"]`)

func libvirtDriversFromCapabilities(xml string) (bool, bool) {
	var hypervisor, lxc bool
	for _, m := range libvirtDomainType.FindAllStringSubmatch(xml, -1) {
		switch strings.ToLower(m[1]) {
		case "kvm", "qemu", "xen", "test":
			hypervisor = true
		case "lxc":
			lxc = true
		}
	}
	return hypervisor, lxc
}

// VMS lists libvirt-managed QEMU/KVM domains (action "vms.list"). Uses the
// default qemu:///system connection; the LXC driver is never mixed in. The
// shape is a flat name list (the read-only slice only has names), distinct
// from the PVE []VM object shape — the prior comment falsely claimed PVE
// alignment (audit finding).
func (p *LibvirtProvider) VMS(params map[string]interface{}) (interface{}, error) {
	names, err := p.listNames("", "--all")
	if err != nil {
		return nil, fmt.Errorf("virt.vms.list: %w", err)
	}
	return names, nil
}

// Containers lists libvirt-managed LXC domains (action "containers.list").
// Targets the dedicated lxc:/// connection so QEMU domains are never
// returned as containers (audit finding: reusing the default connection
// both leaked QEMU domains and dropped all LXC).
func (p *LibvirtProvider) Containers(params map[string]interface{}) (interface{}, error) {
	names, err := p.listNames(libvirtLXCURI, "--all")
	if err != nil {
		return nil, fmt.Errorf("virt.containers.list: %w", err)
	}
	return names, nil
}

// listNames runs `virsh -r [-c uri] list --all --name` and returns one name
// per line, preserving blank-line honesty (an empty list is empty, not nil).
// conn empty means the default (qemu) connection; otherwise it is passed via
// the `-c` global option before the subcommand.
func (p *LibvirtProvider) listNames(conn string, extra ...string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), libvirtListTimeout)
	defer cancel()
	args := []string{"-r"}
	if conn != "" {
		args = append(args, "-c", conn)
	}
	args = append(args, "list", "--all", "--name")
	args = append(args, extra...)
	stdout, stderr, err := p.run(ctx, p.bin, args...)
	if err != nil {
		detail := strings.TrimSpace(string(stderr))
		if detail == "" {
			return nil, fmt.Errorf("list domains: %w", err)
		}
		return nil, fmt.Errorf("list domains: %v: %s", err, detail)
	}
	lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}, nil
	}
	return lines, nil
}
