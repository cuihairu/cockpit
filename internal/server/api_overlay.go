package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// Overlay 组网观测与管理 API（设计见 docs/guide/overlay-design.md）。
// 挂在 /api/agents/{id}/overlay/... 下（serveAPI 的 /agents/ 分支转发到这里）。
//
//	GET  /api/agents/{id}/overlay/status               组网工具快照（M1，浏览不审计）
//	GET  /api/agents/{id}/overlay/daemon               daemon/服务状态（M3，浏览不审计）
//	POST /api/agents/{id}/overlay/networks/{netId}/join  加入网络（overlay:admin，审计 overlay_join）
//	POST /api/agents/{id}/overlay/networks/{netId}/leave 离开网络（overlay:admin，审计 overlay_leave）
//	POST /api/agents/{id}/overlay/service               服务启停/自启（overlay:admin，审计 service_toggle）
//
// server 纯转发不落库（D9）：CLI 即事实源。join/leave/service 为写操作：
// netId 与参数双端校验（D23/D29，agent 端同规则再挡一道）；成功后响应
// 携带 agent 端刷新的 status 快照与 identity（D26），server 就地更新
// registry 内该 agent 的 metadata.identity——身份经响应载荷上报，不动
// WebSocket 协议面。agent 侧错误原样透传，输出已由 provider 白名单构造。

// M3 写操作参数白名单（与 agent rpc/overlay_write.go 同规则）
var (
	// overlayTailnetNameRe Tailscale tailnet 名：`-` 或完整 tailnet DNS
	// 后缀（≥2 级标签；整体锚定，见 agent 端同规则的防退化注释）（D24/D29）
	overlayTailnetNameRe = regexp.MustCompile(`^(-|[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)+)$`)
	// overlayServiceActionRe 服务动词白名单（D27）
	overlayServiceActionRe = regexp.MustCompile(`^(start|stop|enable|disable)$`)
)

// overlayJoinableTools join/leave 支持的工具（wireguard/frp 无加入语义，D25）
var overlayJoinableTools = map[string]bool{"zerotier": true, "tailscale": true}

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

	switch {
	case sub == "status" && r.Method == http.MethodGet:
		data, ok := s.overlayForward(w, r, agentID, "overlay.status", nil)
		if !ok {
			return
		}
		s.writeJSON(w, http.StatusOK, data)
	case sub == "daemon" && r.Method == http.MethodGet:
		data, ok := s.overlayForward(w, r, agentID, "overlay.daemon", nil)
		if !ok {
			return
		}
		s.writeJSON(w, http.StatusOK, data)
	case sub == "service" && r.Method == http.MethodPost:
		s.handleOverlayService(w, r, agentID)
	case strings.HasPrefix(sub, "networks/"):
		s.handleOverlayNetworkChange(w, r, agentID, strings.TrimPrefix(sub, "networks/"))
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// overlayForward RPC 转发共同路径：CallAgent → 解码 → 状态检查 → data。
// 失败已写应答，返回 ok=false。
func (s *Server) overlayForward(w http.ResponseWriter, r *http.Request, agentID, method string, params map[string]interface{}) (map[string]interface{}, bool) {
	resp, err := s.CallAgent(agentID, method, params)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return nil, false
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "agent returned invalid response")
		return nil, false
	}
	if rpcResp.Status == "error" {
		msg := rpcResp.Error
		if msg == "" {
			msg = "agent rejected the operation"
		}
		s.handleError(w, r, http.StatusBadGateway, msg)
		return nil, false
	}
	data, _ := rpcResp.Data.(map[string]interface{})
	if data == nil {
		data = map[string]interface{}{}
	}
	return data, true
}

// handleOverlayNetworkChange /networks/{netId}/join|leave（D29）。
// netId 按 body 里 tool 的形态校验（路径里两段联合定工具与动作）。
func (s *Server) handleOverlayNetworkChange(w http.ResponseWriter, r *http.Request, agentID, sub string) {
	parts := strings.Split(strings.Trim(sub, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || (parts[1] != "join" && parts[1] != "leave") {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	netID, verb := parts[0], parts[1]

	var body struct {
		Tool string `json:"tool"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "body must be {\"tool\": \"zerotier|tailscale\"}")
		return
	}
	if !overlayJoinableTools[body.Tool] {
		s.handleError(w, r, http.StatusBadRequest, "tool must be zerotier or tailscale (wireguard/frp have no join/leave semantics)")
		return
	}
	if body.Tool == "zerotier" && !ztNetworkIDRe.MatchString(netID) {
		s.handleError(w, r, http.StatusBadRequest, "invalid zerotier network id (expect 16 hex)")
		return
	}
	if body.Tool == "tailscale" && !overlayTailnetNameRe.MatchString(netID) {
		s.handleError(w, r, http.StatusBadRequest, "invalid tailnet name (expect - or tailnet domain)")
		return
	}

	method := "overlay.join"
	if verb == "leave" {
		method = "overlay.leave"
	}
	data, ok := s.overlayForward(w, r, agentID, method, map[string]interface{}{
		"tool":      body.Tool,
		"networkId": netID,
	})
	if !ok {
		return
	}
	// 身份上报（D26）：agent 响应载荷带刷新后的 identity → 就地更新
	// registry，CMDB 对照即时生效；载荷缺席 identity 时不动旧数据
	if id, ok := data["identity"].(map[string]interface{}); ok && len(id) > 0 {
		if agent, ok := s.registry.Get(agentID); ok {
			agent.UpdateOverlayIdentity(id)
		}
	}
	action := "overlay_join"
	if verb == "leave" {
		action = "overlay_leave"
	}
	s.auditOverlay(r, action, fmt.Sprintf("%s/%s/%s", agentID, body.Tool, netID),
		map[string]interface{}{"tool": body.Tool, "networkId": netID})
	s.writeJSON(w, http.StatusOK, data)
}

// handleOverlayService POST /overlay/service（D27/D29）：服务启停/自启。
func (s *Server) handleOverlayService(w http.ResponseWriter, r *http.Request, agentID string) {
	var body struct {
		Tool   string `json:"tool"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "body must be {\"tool\": ..., \"action\": \"start|stop|enable|disable\"}")
		return
	}
	if !overlayJoinableTools[body.Tool] {
		s.handleError(w, r, http.StatusBadRequest, "tool must be zerotier or tailscale")
		return
	}
	if !overlayServiceActionRe.MatchString(body.Action) {
		s.handleError(w, r, http.StatusBadRequest, "action must be start, stop, enable or disable")
		return
	}
	data, ok := s.overlayForward(w, r, agentID, "overlay.service", map[string]interface{}{
		"tool":   body.Tool,
		"action": body.Action,
	})
	if !ok {
		return
	}
	// 审计 resourceID 用 agent 回带的 unit 名（白名单映射的单一事实源在
	// agent 端）；异常缺席时退化工具名
	unit, _ := data["unit"].(string)
	if unit == "" {
		unit = body.Tool
	}
	s.auditOverlay(r, "service_toggle", fmt.Sprintf("%s/%s", agentID, unit),
		map[string]interface{}{"tool": body.Tool, "unit": unit, "action": body.Action})
	s.writeJSON(w, http.StatusOK, data)
}

// auditOverlay M3 本机侧变更审计（D29；风格对齐 auditOverlayCloud）。
func (s *Server) auditOverlay(r *http.Request, action, resourceID string, details interface{}) {
	username := ""
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, action, audit.ResourceOverlayNetwork,
		resourceID, details, s.getClientIP(r), r.UserAgent())
}
