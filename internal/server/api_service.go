package server

import (
	"encoding/json"
	"fmt"
	"io"
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
//	POST /api/agents/{id}/services/daemon-reload   刷新 manager 配置（审计 service_action，D12）
//	GET  /api/agents/{id}/services/{unit}/file     unit 文件有效视图（浏览，不审计，D13）
//	PUT  /api/agents/{id}/services/{unit}/file     保存 unit 文件（审计 service_action，D13）
//	POST /api/agents/{id}/services/{unit}/{action} 执行动作（审计 service_action）
//
// server 纯转发不落库（cron D9 同款纪律）：systemd 为唯一事实源。unit 名与
// 动作双端同规则校验（白名单 + .service 后缀），非法请求 server 直接 400，
// 不消耗 agent 往返；agent 侧错误原样透传。

// server 侧 unit 名校验为双后端白名单并集（D9.4）：systemd unit 名或
// Windows 服务名二者其一即放行转发——server 不解析 agent capability 做
// 精确匹配，后端专属严格校验在 agent 侧各自兜底；并集仍是纯白名单
// （无 / \ 空字节 ..），无注入面
var (
	serviceUnitNamePattern    = regexp.MustCompile(`^[A-Za-z0-9@._+-]+\.service$`)
	windowsServiceNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.\- ]{1,256}$`)
)

func validateServiceUnitName(unit string) error {
	if serviceUnitNamePattern.MatchString(unit) {
		return nil
	}
	if windowsServiceNamePattern.MatchString(unit) && unit != "." && unit != ".." {
		return nil
	}
	return fmt.Errorf("invalid unit name %q (expect *.service or windows service name)", unit)
}

// validateServiceAction server 侧 unit 名与动作校验（agent 侧按 backend 同规则校验）
func validateServiceAction(unit, action string) error {
	if err := validateServiceUnitName(unit); err != nil {
		return err
	}
	switch action {
	case "start", "stop", "restart", "reload", "enable", "disable", "mask", "unmask":
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
	case sub == "daemon-reload" && r.Method == http.MethodPost:
		// systemd manager 配置刷新（D12）：全局操作不针对 unit，sub 只有一段，
		// 在两段 unit/action 解析之前特判（unit 动作路由恒两段，无歧义）
		s.forwardServiceRPC(w, r, agentID, "service.daemon-reload", nil, "daemon-reload", "daemon-reload")
	case strings.HasSuffix(sub, "/file") && r.Method == http.MethodGet:
		// unit 文件有效视图（D13，浏览不审计）；GET 与 unit 动作的 POST 不重叠
		s.handleUnitFile(w, r, agentID, sub, false)
	case strings.HasSuffix(sub, "/file") && r.Method == http.MethodPut:
		// unit 文件保存（D13，审计 details.action=unitfile-save）
		s.handleUnitFile(w, r, agentID, sub, true)
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

// unitFileBodyLimit 保存内容上限（与 agent 侧 unitFileContentLimit 同值，双端防御）
const unitFileBodyLimit = 256 << 10

// handleUnitFile 分发 GET/PUT /{unit}/file（D13）
func (s *Server) handleUnitFile(w http.ResponseWriter, r *http.Request, agentID, sub string, saving bool) {
	unit := strings.TrimSuffix(sub, "/file")
	// 仅允许恰好两段（{unit}/file），多余段即 404
	if unit == "" || strings.Contains(unit, "/") {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	if err := validateServiceUnitName(unit); err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if !saving {
		s.forwardServiceRPC(w, r, agentID, "service.unitfile",
			map[string]interface{}{"name": unit}, "", "")
		return
	}
	var payload struct {
		Content string `json:"content"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, unitFileBodyLimit+1))
	if err != nil {
		s.handleError(w, r, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("unit file content exceeds limit (%d bytes)", unitFileBodyLimit))
		return
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(payload.Content) > unitFileBodyLimit {
		s.handleError(w, r, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("unit file content exceeds limit (%d bytes)", unitFileBodyLimit))
		return
	}
	s.forwardServiceRPC(w, r, agentID, "service.unitsave",
		map[string]interface{}{"name": unit, "content": payload.Content}, unit, "unitfile-save")
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
