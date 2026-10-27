package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/oob"
	"github.com/cuihairu/cockpit/internal/oob/amt"
	"github.com/cuihairu/cockpit/internal/oob/dash"
	"github.com/cuihairu/cockpit/internal/oob/redfish"
	"github.com/cuihairu/cockpit/internal/oob/wsman"
)

const oobDiscoverTimeout = 5 * time.Second

// OobDiscoverCmd runs the read-only multi-provider discovery pass.
// It intentionally has no action flags: operations requiring credentials stay
// in their future provider slices and are not guessed from discovery.
type OobDiscoverCmd struct {
	Address string
	Timeout time.Duration
	Output  io.Writer

	registry *oob.Registry
}

func (c *OobDiscoverCmd) Run(ctx context.Context) error {
	if strings.TrimSpace(c.Address) == "" {
		return fmt.Errorf("address required")
	}
	if c.Timeout <= 0 {
		c.Timeout = oobDiscoverTimeout
	}
	if c.registry == nil {
		c.registry = newOobDiscoverRegistry(c.Timeout)
	}
	if c.Output == nil {
		c.Output = os.Stdout
	}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	results := c.registry.ProbeAll(ctx, oob.Target{Address: strings.TrimSpace(c.Address)})
	view := oob.Merge(results)
	c.printResults(results, view)
	return nil
}

func newOobDiscoverRegistry(timeout time.Duration) *oob.Registry {
	client := &http.Client{Timeout: timeout}
	registry := oob.NewRegistry()
	wsmanOptions := wsman.Options{Client: client}
	registry.MustRegister(amt.New(amt.Options{Wsman: wsmanOptions}))
	registry.MustRegister(dash.New(dash.Options{Wsman: wsmanOptions}))
	registry.MustRegister(redfish.New(redfish.Options{Client: client}))
	return registry
}

func (c *OobDiscoverCmd) printResults(results []oob.ProbeResult, view oob.DeviceView) {
	fmt.Fprintf(c.Output, "OOB discovery for %s\n", strings.TrimSpace(c.Address))
	for _, result := range results {
		presence, evidence := oobPresence(result)
		fmt.Fprintf(c.Output, "Provider %s: %s\n  Evidence: %s\n", result.Provider, presence, evidence)
	}

	fmt.Fprintln(c.Output, "Capabilities (omitted capabilities remain unknown):")
	if len(view.Capabilities) == 0 {
		fmt.Fprintln(c.Output, "  none")
		return
	}
	for _, capability := range sortedCapabilities(view.Capabilities) {
		observed := view.Capabilities[capability]
		state := observed.State
		fmt.Fprintf(c.Output, "  %s: supported=%t enabled=%t reachable=%t source=%s\n",
			capability, state.Supported, state.Enabled, state.Reachable, observed.Source)
	}
}

func oobPresence(result oob.ProbeResult) (string, string) {
	if result.Err == nil {
		return "Present", "provider discovery response"
	}
	if providerRejectedResult(result.Err) {
		return "NotPresent", result.Err.Error()
	}
	return "Unknown", result.Err.Error()
}

func providerRejectedResult(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "not amt") ||
		strings.Contains(text, "not dash") ||
		strings.Contains(text, "not a redfish service") ||
		strings.Contains(text, "lacks redfishversion")
}

func sortedCapabilities(capabilities map[oob.Capability]oob.CapabilityView) []oob.Capability {
	names := make([]string, 0, len(capabilities))
	for capability := range capabilities {
		names = append(names, string(capability))
	}
	sort.Strings(names)
	out := make([]oob.Capability, 0, len(names))
	for _, name := range names {
		out = append(out, oob.Capability(name))
	}
	return out
}
