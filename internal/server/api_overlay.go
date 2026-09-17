package server

import (
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// Overlay 组网观测 API（设计见 docs/guide/overlay-design.md）。
// 挂在 /api/agents/{id}/overlay/... 下（serveAPI 的 /agents/ 分支转发到这里）。
//
//	GET /api/agents/{id}/overlay/status   组网工具快照（ZeroTier/Tailscale/WireGuard/frp）
//
// server 纯转发不落库（D9）：CLI 即事实源。浏览类端点不记审计（D8）。
// agent 侧错误原样透传，输出已由 provider 白名单构造（不含 wg 私钥）。

// handleAgentOverlayAPI 分发 /api/agents/{id}/overlay/{sub}
// rest 是 "/agents/" 之后的部分（形如 "{agentID}/overlay/status"）
func (s *Server) handleAgentOverlayAPI(w http.ResponseWriter, r *http.Request, rest string) {
	const suffix = "/overlay/"
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

	resp, err := s.CallAgent(agentID, "overlay.status", nil)
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
