package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// NAS 存储观测 API（设计见 docs/guide/nas-design.md D5）。
//
//	GET /api/agents/{id}/nas/status   单机存储快照（纯转发不落库不审计）
//	GET /api/nas/config               巡检配置（间隔 + 容量阈值，全局）
//	PUT /api/nas/config               保存（巡检配置类不记审计，smart 同构）
//
// 浏览类端点不记审计；agent 侧错误原样透传。

// handleNASConfig 全局巡检配置：GET 返回间隔与阈值；PUT 校验后写 Setting
func (s *Server) handleNASConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"scan_interval_seconds": s.GetNASScanInterval(),
			"min":                   nasMinIntervalSeconds,
			"max":                   nasMaxIntervalSeconds,
			"default":               nasDefaultInterval,
			"usage_warn_percent":    s.GetNASUsageWarnPercent(),
			"usageMin":              nasMinUsageWarnPercent,
			"usageMax":              nasMaxUsageWarnPercent,
		})
	case http.MethodPut:
		var req struct {
			ScanIntervalSeconds int  `json:"scan_interval_seconds"`
			UsageWarnPercent    *int `json:"usage_warn_percent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
		if err := s.SetNASScanInterval(req.ScanIntervalSeconds); err != nil {
			s.handleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		if req.UsageWarnPercent != nil {
			if err := s.SetNASUsageWarnPercent(*req.UsageWarnPercent); err != nil {
				s.handleError(w, r, http.StatusBadRequest, err.Error())
				return
			}
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"scan_interval_seconds": req.ScanIntervalSeconds,
			"usage_warn_percent":    s.GetNASUsageWarnPercent(),
		})
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleAgentNASAPI 分发 /api/agents/{id}/nas/{sub}
// rest 是 "/agents/" 之后的部分（形如 "{agentID}/nas/status"）
func (s *Server) handleAgentNASAPI(w http.ResponseWriter, r *http.Request, rest string) {
	const suffix = "/nas/"
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

	resp, err := s.CallAgent(agentID, "nas.status", nil)
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
