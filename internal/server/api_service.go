package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// systemd 服务管理 API（设计见 docs/guide/service-design.md）。
// 挂在 /api/agents/{id}/services/... 下（serveAPI 的 /agents/ 分支转发到这里）。
//
//	GET  /api/agents/{id}/services/status          概览（浏览，不审计）
//	GET  /api/agents/{id}/services                 服务列表（浏览，不审计）
//	POST /api/agents/{id}/services/{unit}/{action} 执行动作（审计 service_action）
//
// server 纯转发不落库（cron D9 同款纪律）：systemd 为唯一事实源。unit 名与
// 动作双端同规则校验（白名单 + .service 后缀），非法请求 server 直接 400，
// 不消耗 agent 往返；agent 侧错误原样透传。

// 与 agent 侧 validateServiceUnit 完全一致的校验规则（双端同规则）
var serviceUnitNamePattern = regexp.MustCompile(`^[A-Za-z0-9@._+-]+\.service$`)

// validateServiceAction server 侧 unit 名与动作校验（与 agent 同规则）
func validateServiceAction(unit, action string) error {
	if !serviceUnitNamePattern.MatchString(unit) {
		return fmt.Errorf("invalid unit name %q (expect *.service)", unit)
	}
	switch action {
	case "start", "stop", "restart", "reload", "enable", "disable":
		return nil
	}
	return fmt.Errorf("unsupported action %q", action)
}

// handleAgentServiceAPI 分发 /api/agents/{id}/services/{sub}
// rest 是 "/agents/" 之后的部分（形如 "{agentID}/services" 或 "{agentID}/services/nginx.service/restart"）
func (s *Server) handleAgentServiceAPI(w http.ResponseWriter, r *http.Request, rest string) {
	const suffix = "/services"
	idx := strings.Index(rest, suffix)
	if idx <= 0 {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	agentID := rest[:idx]
	sub := rest[idx+len(suffix):]
	// sub 形如 ""、"/status"、"/nginx.service/restart"
	sub = strings.TrimPrefix(sub, "/")
	if agentID == "" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	if _, ok := s.registry.Get(agentID); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}

	switch {
	case sub == "status" && r.Method == http.MethodGet:
		s.forwardServiceRPC(w, r, agentID, "service.status", nil, "", "")
	case sub == "" && r.Method == http.MethodGet:
		s.forwardServiceRPC(w, r, agentID, "service.list", nil, "", "")
	case sub != "" && r.Method == http.MethodPost:
		parts := strings.Split(sub, "/")
		if len(parts) != 2 {
			s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
			return
		}
		unit, action := parts[0], parts[1]
		if err := validateServiceAction(unit, action); err != nil {
			s.handleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		s.forwardServiceRPC(w, r, agentID, "service.action",
			map[string]interface{}{"name": unit, "action": action}, unit, action)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// forwardServiceRPC 转发 RPC 并透传结果；action 非空时记审计
func (s *Server) forwardServiceRPC(w http.ResponseWriter, r *http.Request, agentID, method string,
	params map[string]interface{}, unit, action string) {

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
		// agent 错误原样透传（如 systemctl 的 stderr 摘要）
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

	if action != "" {
		username := "unknown"
		if userInfo, ok := auth.GetUserFromContext(r); ok {
			username = userInfo.Username
		}
		s.audit.LogResource(username, audit.ActionServiceAction, audit.ResourceService, unit,
			map[string]interface{}{"action": action, "agent": agentID},
			s.getClientIP(r), r.UserAgent())
	}
	s.writeJSON(w, http.StatusOK, data)
}
