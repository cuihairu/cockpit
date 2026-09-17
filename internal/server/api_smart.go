package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// SMART 磁盘健康 API（设计见 docs/guide/disk-health-design.md D7）。
//
//	GET /api/agents/{id}/smart/status   单机磁盘健康快照（纯转发不落库）
//	GET /api/smart/config               巡检配置（全局，非 per-agent）
//	PUT /api/smart/config               写巡检间隔（0 = 关闭）
//
// 浏览类端点不记审计；agent 侧错误原样透传，输出由 provider 白名单构造。

// handleSmartConfig 全局巡检配置：GET 返回当前间隔与范围；PUT 校验后写入
// Setting（与 handleDriftConfig 同构，见 drift-design.md M2/D17）
func (s *Server) handleSmartConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"scan_interval_seconds": s.GetSmartScanInterval(),
			"min":                   smartMinIntervalSeconds,
			"max":                   smartMaxIntervalSeconds,
			"default":               smartDefaultInterval,
		})
	case http.MethodPut:
		var req struct {
			ScanIntervalSeconds int `json:"scan_interval_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
		if err := s.SetSmartScanInterval(req.ScanIntervalSeconds); err != nil {
			s.handleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"scan_interval_seconds": req.ScanIntervalSeconds,
		})
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleAgentSmartAPI 分发 /api/agents/{id}/smart/{sub}
// rest 是 "/agents/" 之后的部分（形如 "{agentID}/smart/status"）
func (s *Server) handleAgentSmartAPI(w http.ResponseWriter, r *http.Request, rest string) {
	const suffix = "/smart/"
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

	resp, err := s.CallAgent(agentID, "hardware-monitor.status", nil)
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
