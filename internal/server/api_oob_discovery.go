package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/oob"
	"github.com/cuihairu/cockpit/internal/oob/amt"
	"github.com/cuihairu/cockpit/internal/oob/redfish"
)

// OOB multi-provider discovery is a read-only negotiation pass. Providers
// decide what they may claim; this endpoint records the attempted paths and
// preserves per-path failures instead of turning one miss into a 502.
const defaultOOBProbeTimeout = 5 * time.Second

type oobDiscoverRequest struct {
	Address string            `json:"address"`
	Hints   map[string]string `json:"hints,omitempty"`
	Timeout *float64          `json:"timeout_seconds,omitempty"`
}

type oobDiscoveryError struct {
	Provider string `json:"provider"`
	Error    string `json:"error_text"`
}

type oobDiscoveryResult struct {
	Target                string                        `json:"target"`
	Architecture          string                        `json:"architecture,omitempty"`
	Providers             []string                      `json:"providers"`
	Capabilities          map[string]oob.CapabilityView `json:"capabilities"`
	ReachableUndetermined bool                          `json:"reachable_undetermined"`
	Errors                []oobDiscoveryError           `json:"errors,omitempty"`
	Attempted             []string                      `json:"attempted"`
}

func newOOBRegistry() *oob.Registry {
	registry := oob.NewRegistry()
	registry.MustRegister(amt.New(amt.Options{}))
	registry.MustRegister(redfish.New(redfish.Options{}))
	return registry
}

func (req *oobDiscoverRequest) timeout() time.Duration {
	if req.Timeout == nil {
		return defaultOOBProbeTimeout
	}
	seconds := *req.Timeout
	if seconds <= 0 || seconds > 30 {
		return defaultOOBProbeTimeout
	}
	return time.Duration(seconds * float64(time.Second))
}

func (req *oobDiscoverRequest) validate() (string, bool) {
	if strings.TrimSpace(req.Address) == "" {
		return "address required", false
	}
	return "", true
}

func discoveryErrors(results []oob.ProbeResult) []oobDiscoveryError {
	failures := []oobDiscoveryError{}
	for _, result := range results {
		if result.Err == nil {
			continue
		}
		text := discoveryErrorText(result.Err)
		if text == "" {
			text = result.Err.Error()
		}
		failures = append(failures, oobDiscoveryError{
			Provider: result.Provider,
			Error:    text,
		})
	}
	return failures
}

func discoveryErrorText(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "context canceled"
	}
	return ""
}

func reachableUndetermined(results []oob.ProbeResult) bool {
	for _, result := range results {
		if result.Err == nil && len(result.Info.Capabilities) == 0 {
			return true
		}
	}
	return false
}

func oobDiscoveryToResult(results []oob.ProbeResult, view oob.DeviceView, attempted []string,
	failures []oobDiscoveryError) oobDiscoveryResult {
	capabilities := make(map[string]oob.CapabilityView, len(view.Capabilities))
	for capability, state := range view.Capabilities {
		capabilities[string(capability)] = state
	}
	if view.Providers == nil {
		view.Providers = []string{}
	}
	if attempted == nil {
		attempted = []string{}
	}
	return oobDiscoveryResult{
		Target:                view.Architecture,
		Architecture:          view.Architecture,
		Providers:             view.Providers,
		Capabilities:          capabilities,
		ReachableUndetermined: reachableUndetermined(results),
		Errors:                failures,
		Attempted:             attempted,
	}
}

func (s *Server) handleOobDiscover(w http.ResponseWriter, r *http.Request) {
	var req oobDiscoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if reason, ok := req.validate(); !ok {
		s.handleError(w, r, http.StatusBadRequest, reason)
		return
	}

	registry := s.oobProviders
	address := strings.TrimSpace(req.Address)
	target := oob.Target{Address: address, Hints: req.Hints}
	ctx, cancel := context.WithTimeout(r.Context(), req.timeout())
	defer cancel()
	results := registry.ProbeAll(ctx, target)
	failures := discoveryErrors(results)
	view := oob.Merge(results)

	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, audit.ActionOobDiscover, audit.ResourceOob, address,
		map[string]interface{}{"attempted": providerNames(registry)}, s.getClientIP(r), r.UserAgent())

	s.writeJSON(w, http.StatusOK, oobDiscoveryToResult(results, view, providerNames(registry), failures))
}

func providerNames(registry *oob.Registry) []string {
	providers := registry.List()
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, provider.Name())
	}
	return names
}
