package server

import (
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// libvirtReadRPCActions is the whitelist of read-only virt RPC methods that
// may be forwarded from the server API. Write/lifecycle actions are not in
// this slice; the libvirt provider itself also rejects unknown verbs.
var libvirtReadRPCActions = map[string]bool{
	"virt.capabilities":   true,
	"virt.vms.list":       true,
	"virt.containers.list": true,
}

// handleAgentVirtAPI dispatches /api/agents/{id}/virt/{sub}.
// rest is the part after "/agents/" (e.g. "a1/virt/capabilities").
func (s *Server) handleAgentVirtAPI(w http.ResponseWriter, r *http.Request, rest string) {
	const suffix = "/virt/"
	idx := strings.Index(rest, suffix)
	if idx <= 0 {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	agentID := rest[:idx]
	action := rest[idx+len(suffix):]
	if agentID == "" || action == "" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	method := "virt." + action
	if !libvirtReadRPCActions[method] {
		s.handleError(w, r, http.StatusNotFound, "virt action not available")
		return
	}
	if _, ok := s.registry.Get(agentID); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}
	resp, err := s.CallAgent(agentID, method, nil)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "agent returned invalid response")
		return
	}
	if rpcResp.Status == "error" {
		msg := rpcResp.Error
		if msg == "" {
			msg = "agent rejected the operation"
		}
		s.handleError(w, r, http.StatusBadGateway, msg)
		return
	}
	s.writeJSON(w, http.StatusOK, rpcResp.Data)
}

