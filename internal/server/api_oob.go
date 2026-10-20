package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// 带外兜底动作面 API（见 docs/design/oob-openwrt-bootstrap.md 片A/片B）。
//
//	POST /api/oob/wol    唤醒局域网目标机（agent 侧发 WOL 魔术包，审计留痕）
//	POST /api/oob/probe  三源探活（agent 侧 ARP/ICMP/TCP 独立三态判定，审计留痕）
//
// 边界：端点只做校验/转发/审计；MAC 解析、广播地址推导、UDP 发送与三源
// 探测全部在 agent 侧（internal/agent/rpc/oob_provider.go），server 不解析
// MAC、不推导网段、不发包——agent 返回的实际发送目标回填进审计与响应。
const oobAPIPrefix = "/api/oob/"

func (s *Server) registerOobAPI(mux *http.ServeMux) {
	mux.HandleFunc(oobAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
		s.authService().Middleware(s.handleOobAPI)(w, r)
	})
}

func (s *Server) handleOobAPI(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, oobAPIPrefix), "/")
	switch {
	case len(parts) == 1 && parts[0] == "wol" && r.Method == http.MethodPost:
		s.handleOobWake(w, r)
	case len(parts) == 1 && parts[0] == "probe" && r.Method == http.MethodPost:
		s.handleOobProbe(w, r)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// oobWakeRequest WOL 请求：agent_id 与 mac 必需；target/iface/port 可选
// （缺省时 agent 按其本机首个可广播非回环网段推导广播地址）。
type oobWakeRequest struct {
	AgentID string      `json:"agent_id"`
	MAC     string      `json:"mac"`
	Target  string      `json:"target"`
	Iface   string      `json:"iface"`
	Port    interface{} `json:"port"`
}

// handleOobWake 转发 WOL 到 agent（agent 侧 oob provider 执行发送）。
func (s *Server) handleOobWake(w http.ResponseWriter, r *http.Request) {
	var req oobWakeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AgentID == "" || req.MAC == "" {
		s.handleError(w, r, http.StatusBadRequest, "agent_id and mac required")
		return
	}
	agent, ok := s.registry.Get(req.AgentID)
	if !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}
	if !agent.HasCapability("oob") {
		s.handleError(w, r, http.StatusNotFound, "oob capability not available")
		return
	}

	params := map[string]interface{}{"mac": req.MAC}
	if req.Target != "" {
		params["target"] = req.Target
	}
	if req.Iface != "" {
		params["iface"] = req.Iface
	}
	if req.Port != nil {
		params["port"] = req.Port
	}
	resp, err := s.CallAgent(req.AgentID, "oob.wol", params)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status == "error" {
		msg := "failed to send wake packet"
		if rpcResp.Error != "" {
			msg = rpcResp.Error
		}
		s.handleError(w, r, http.StatusBadGateway, msg)
		return
	}

	data, _ := rpcResp.Data.(map[string]interface{})
	s.auditOobWake(r, req, data)
	s.writeJSON(w, http.StatusOK, rpcResp.Data)
}

// auditOobWake WOL 审计：目标 MAC 与实际发送目标/字节数（agent 回填）。
func (s *Server) auditOobWake(r *http.Request, req oobWakeRequest, data map[string]interface{}) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	details := map[string]interface{}{"mac": req.MAC}
	if target := data["target"]; target != nil {
		details["target"] = target
	}
	if sent := data["sent"]; sent != nil {
		details["sent"] = sent
	}
	s.audit.LogResource(username, audit.ActionOobWol, audit.ResourceOob, req.AgentID,
		details, s.getClientIP(r), r.UserAgent())
}

// oobProbeRequest 探活请求：agent_id 与 ip 必需；ports/timeout（秒）可选。
type oobProbeRequest struct {
	AgentID string        `json:"agent_id"`
	IP      string        `json:"ip"`
	Ports   []interface{} `json:"ports"`
	Timeout interface{}   `json:"timeout"`
}

// handleOobProbe 转发三源探活到 agent（三态判定在 agent 侧产生，server 只
// 校验/转发/审计）。
func (s *Server) handleOobProbe(w http.ResponseWriter, r *http.Request) {
	var req oobProbeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AgentID == "" || req.IP == "" {
		s.handleError(w, r, http.StatusBadRequest, "agent_id and ip required")
		return
	}
	agent, ok := s.registry.Get(req.AgentID)
	if !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}
	if !agent.HasCapability("oob") {
		s.handleError(w, r, http.StatusNotFound, "oob capability not available")
		return
	}

	params := map[string]interface{}{"ip": req.IP}
	if len(req.Ports) > 0 {
		params["ports"] = req.Ports
	}
	if req.Timeout != nil {
		params["timeout"] = req.Timeout
	}
	resp, err := s.CallAgent(req.AgentID, "oob.probe", params)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status == "error" {
		msg := "failed to probe target"
		if rpcResp.Error != "" {
			msg = rpcResp.Error
		}
		s.handleError(w, r, http.StatusBadGateway, msg)
		return
	}

	s.auditOobProbe(r, req)
	s.writeJSON(w, http.StatusOK, rpcResp.Data)
}

// auditOobProbe 探活审计：目标 IP 与端口清单；三态判定结果不落审计
// （以响应返回，探测动作本身可重复）。
func (s *Server) auditOobProbe(r *http.Request, req oobProbeRequest) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	details := map[string]interface{}{"ip": req.IP}
	if len(req.Ports) > 0 {
		details["ports"] = req.Ports
	}
	s.audit.LogResource(username, audit.ActionOobProbe, audit.ResourceOob, req.AgentID,
		details, s.getClientIP(r), r.UserAgent())
}
