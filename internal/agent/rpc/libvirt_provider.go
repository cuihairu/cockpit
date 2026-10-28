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
// visible in `virsh capabilities`. Only daemon-verified evidence is
// reported: no driver is inferred from the virsh binary being installed.
func (p *LibvirtProvider) Capabilities(params map[string]interface{}) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), libvirtProbeTimeout)
	defer cancel()

	// `virsh capabilities` is the single authoritative read probe: it
	// connects to the daemon AND returns the full capability XML. A failed
	// connection (daemon down, no driver, socket missing) is reported as
	// available=false with the transport detail preserved.
	capsXML, stderr, err := p.run(ctx, p.bin, "-r", "capabilities")
	if err != nil {
		return map[string]interface{}{"available": false}, p.wrapProbe(err, stderr)
	}

	out := map[string]interface{}{"available": true}
	hypervisor, lxc := libvirtDriversFromCapabilities(string(capsXML))
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

// wrapProbe preserves transport errors verbatim so callers can tell
// "daemon present but XML parse/capabilities call failed" apart from
// "daemon unreachable" — same honesty rule as oobPresence.
func (p *LibvirtProvider) wrapProbe(err error, stderr []byte) error {
	detail := strings.TrimSpace(string(stderr))
	if detail == "" {
		return fmt.Errorf("libvirt probe: %w", err)
	}
	return fmt.Errorf("libvirt probe: %v: %s", err, detail)
}

// VMS lists libvirt-managed domains (action "vms.list"). Shape follows the
// PVE list contract ({vms: []}) so server consumers can share rendering.
func (p *LibvirtProvider) VMS(params map[string]interface{}) (interface{}, error) {
	names, err := p.listNames("--all")
	if err != nil {
		return nil, fmt.Errorf("virt.vms.list: %w", err)
	}
	return map[string]interface{}{"vms": names}, nil
}

// Containers lists libvirt-managed LXC domains (action "containers.list").
// `virsh -r list --all` returns both QEMU and LXC domains on the same
// daemon; the distinction is surfaced via capabilities in this read-only
// slice rather than guessed per row.
func (p *LibvirtProvider) Containers(params map[string]interface{}) (interface{}, error) {
	names, err := p.listNames("--all")
	if err != nil {
		return nil, fmt.Errorf("virt.containers.list: %w", err)
	}
	return map[string]interface{}{"containers": names}, nil
}

// listNames runs `virsh -r list --all --name` and returns one name per
// line, preserving blank-line honesty (an empty list is empty, not nil).
func (p *LibvirtProvider) listNames(extra ...string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), libvirtListTimeout)
	defer cancel()
	args := append([]string{"-r", "list", "--all", "--name"}, extra...)
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
