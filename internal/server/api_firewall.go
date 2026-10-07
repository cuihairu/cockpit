package server

import (
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// 防火墙规则集快照 API（设计见 docs/guide/firewall-design.md D9）。
//
//	GET /api/agents/{id}/firewall/status   单机规则集快照（纯转发不落库）
//
// 浏览类端点不记审计；agent 侧错误原样透传，输出由 provider 白名单构造。

// handleAgentFirewallAPI 分发 /api/agents/{id}/firewall/{sub}
// rest 是 "/agents/" 之后的部分（形如 "{agentID}/firewall/status"）
func (s *Server) handleAgentFirewallAPI(w http.ResponseWriter, r *http.Request, rest string) {
	const suffix = "/firewall/"
	idx := strings.Index(rest, suffix)
	if idx <= 0 {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	agentID := rest[:idx]
	sub := rest[idx+len(suffix):]
	if agentID == "" || sub == "" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	if _, ok := s.registry.Get(agentID); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}

	if sub != "status" || r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}

	resp, err := s.CallAgent(agentID, "firewall.status", nil)
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
	data, _ := rpcResp.Data.(map[string]interface{})
	if data == nil {
		data = map[string]interface{}{}
	}
	s.writeJSON(w, http.StatusOK, data)
}
