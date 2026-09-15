package server

import (
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// 防漂移检测 API（设计见 docs/guide/drift-design.md）。
// 挂在 /api/agents/{id}/drift/... 下（serveAPI 的 /agents/ 分支转发到这里）：
//
//	POST /api/agents/{id}/drift/check  全量比对（nginx/cron/stack 基线 vs 当前）
//
// server 纯转发不落库（D11）；浏览类操作不记审计；check 无用户输入参数，
// 校验面为零；agent 侧错误原样透传。

// handleAgentDriftAPI 分发 /api/agents/{id}/drift/{sub}
// rest 是 "/agents/" 之后的部分（形如 "{agentID}/drift/check"）
func (s *Server) handleAgentDriftAPI(w http.ResponseWriter, r *http.Request, rest string) {
	const suffix = "/drift/"
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

	switch {
	case sub == "check":
		if r.Method != http.MethodPost {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.forwardDriftRPC(w, r, agentID, "drift.check", nil)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// forwardDriftRPC 转发 RPC 并透传结果（与 forwardLogsRPC 同构）
func (s *Server) forwardDriftRPC(w http.ResponseWriter, r *http.Request, agentID, method string,
	params map[string]interface{}) {

	resp, err := s.CallAgent(agentID, method, params)
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
