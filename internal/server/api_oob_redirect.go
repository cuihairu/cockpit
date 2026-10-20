package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/oob/redirect"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/vault"
)

// 兜底转发编排（片C，docs/design/oob-openwrt-bootstrap.md §3）。
//
// 纪律：
//   - 双确认：下发要求 confirm_label 等于规则标签，拆除要求 confirm_name
//     等于规则名（同 handleBackupRestore 的 confirmName 先例）。
//   - 凭据：只从 vault 的 oob/<name> 条目取（同 nas/<name> 先例），
//     明文只在内存态与加密 WS 通道流转，不落库、不进日志与审计明文。
//   - 规则名由 agent 侧生成（过期时刻进名字），server 只回传与留痕。
//   - RBAC 归 /api/oob 既有前缀规则：读走 inventory:read，写走 inventory:write。

// oobVaultPrefix vault 条目前缀：oob/<name>
const oobVaultPrefix = "oob/"

// oobRedirectSpec 转发规格（请求体片段）。
type oobRedirectSpec struct {
	Label      string `json:"label"`
	Proto      string `json:"proto"`
	SrcZone    string `json:"src_zone"`
	DstZone    string `json:"dst_zone"`
	SrcIP      string `json:"src_ip,omitempty"`
	DstIP      string `json:"dst_ip"`
	SrcPort    int    `json:"src_port,omitempty"`
	DstPort    int    `json:"dst_port"`
	TTLSeconds int    `json:"ttl_seconds"`
}

// toRedirectSpec 转纯逻辑层的规格（TTL 由秒转时长）。
func (q oobRedirectSpec) toRedirectSpec() redirect.Spec {
	return redirect.Spec{
		Label:   q.Label,
		Proto:   q.Proto,
		SrcZone: q.SrcZone,
		DstZone: q.DstZone,
		SrcIP:   q.SrcIP,
		DstIP:   q.DstIP,
		SrcPort: q.SrcPort,
		DstPort: q.DstPort,
		TTL:     ttlSeconds(q.TTLSeconds),
	}
}

// ttlSeconds 请求秒数转时长：<=0 表示用缺省 TTL（redirect 层收敛缺省与上限）。
func ttlSeconds(secs int) time.Duration {
	if secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

// vaultOobTarget 解析 oob/<name> 条目为下发连接参数（host/port/username/
// password/insecure_tls）。明文只留在返回值，不落日志。
func (s *Server) vaultOobTarget(name string) (map[string]interface{}, error) {
	if s.vaultProvider == nil {
		return nil, errors.New("vault unavailable")
	}
	e, err := s.vaultProvider.Read(oobVaultPrefix + name)
	if err != nil {
		if errors.Is(err, vault.ErrLocked) {
			return nil, errors.New("vault locked")
		}
		if errors.Is(err, vault.ErrNotFound) {
			return nil, errors.New("oob credential not found")
		}
		return nil, err
	}
	if e.Kind != "pair" && e.Kind != "password" {
		return nil, errors.New("oob credential is not a pair entry")
	}
	out := map[string]interface{}{}
	if addr := e.Meta["addr"]; addr != "" {
		host, port, err := net.SplitHostPort(addr)
		if err != nil || host == "" {
			// 无端口写法（仅主机名/IP）直接当 host 用
			host = strings.TrimSpace(addr)
		}
		out["host"] = host
		if port != "" {
			if p, err := strconv.Atoi(port); err == nil {
				out["port"] = p
			}
		}
	}
	// 空凭据不带键：agent 侧缺省即匿名 ubus，避免空串被当成有效值
	if e.Username != "" {
		out["username"] = e.Username
	}
	if e.Value != "" {
		out["password"] = e.Value
	}
	if e.Meta["insecure_tls"] == "true" {
		out["insecure_tls"] = true
	}
	if out["host"] == nil {
		return nil, errors.New("oob credential has no addr")
	}
	return out, nil
}

// oobRedirectRequest 下发转发规则请求。
type oobRedirectRequest struct {
	AgentID      string          `json:"agent_id"`
	Oob          string          `json:"oob"` // vault oob/<name> 条目名
	Spec         oobRedirectSpec `json:"spec"`
	ConfirmLabel string          `json:"confirm_label"`
}

// handleOobRedirect POST /api/oob/redirect：双确认后下发一条转发规则。
func (s *Server) handleOobRedirect(w http.ResponseWriter, r *http.Request) {
	var req oobRedirectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AgentID == "" || req.Oob == "" {
		s.handleError(w, r, http.StatusBadRequest, "agent_id and oob required")
		return
	}
	if err := redirect.Spec(req.Spec.toRedirectSpec()).Validate(); err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	// 双确认：标签必须逐字一致，误触无法触发
	if req.ConfirmLabel != req.Spec.Label {
		s.handleError(w, r, http.StatusBadRequest, "confirm_label must match spec.label")
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
	conn, err := s.vaultOobTarget(req.Oob)
	if err != nil {
		s.handleError(w, r, http.StatusServiceUnavailable, err.Error())
		return
	}
	params := map[string]interface{}{
		"label":       req.Spec.Label,
		"proto":       req.Spec.Proto,
		"src_zone":    req.Spec.SrcZone,
		"dst_zone":    req.Spec.DstZone,
		"dst_ip":      req.Spec.DstIP,
		"dst_port":    req.Spec.DstPort,
		"ttl_seconds": req.Spec.TTLSeconds,
	}
	if req.Spec.SrcIP != "" {
		params["src_ip"] = req.Spec.SrcIP
	}
	if req.Spec.SrcPort > 0 {
		params["src_port"] = req.Spec.SrcPort
	}
	for k, v := range conn {
		params[k] = v
	}

	resp, err := s.CallAgent(req.AgentID, "oob.redirect", params)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status == "error" {
		s.auditRedirectRefuse(r, req.AgentID, req.Spec.Label, rpcResp.Error)
		s.handleError(w, r, http.StatusBadGateway, "failed to apply redirect")
		return
	}
	s.auditOobRedirect(r, req, rpcResp.Data)
	s.writeJSON(w, http.StatusOK, rpcResp.Data)
}

// oobRedirectRemoveRequest 拆除转发规则请求。
type oobRedirectRemoveRequest struct {
	AgentID     string `json:"agent_id"`
	Oob         string `json:"oob"`
	Name        string `json:"name"`
	ConfirmName string `json:"confirm_name"`
}

// handleOobRedirectRemove POST /api/oob/redirect/remove：双确认后拆除规则。
func (s *Server) handleOobRedirectRemove(w http.ResponseWriter, r *http.Request) {
	var req oobRedirectRemoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AgentID == "" || req.Oob == "" || req.Name == "" {
		s.handleError(w, r, http.StatusBadRequest, "agent_id, oob and name required")
		return
	}
	if req.ConfirmName != req.Name {
		s.handleError(w, r, http.StatusBadRequest, "confirm_name must match name")
		return
	}
	if !redirect.Owned(req.Name) {
		s.handleError(w, r, http.StatusBadRequest, "name must be a cockpit-owned rule")
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
	conn, err := s.vaultOobTarget(req.Oob)
	if err != nil {
		s.handleError(w, r, http.StatusServiceUnavailable, err.Error())
		return
	}
	params := map[string]interface{}{"name": req.Name}
	for k, v := range conn {
		params[k] = v
	}
	resp, err := s.CallAgent(req.AgentID, "oob.redirect.remove", params)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status == "error" {
		s.auditRedirectRefuse(r, req.AgentID, "", rpcResp.Error)
		s.handleError(w, r, http.StatusBadGateway, "failed to remove redirect")
		return
	}
	s.auditOobRedirectRemove(r, req, rpcResp.Data)
	s.writeJSON(w, http.StatusOK, rpcResp.Data)
}

// handleOobRedirectList GET /api/oob/redirect?agent_id=&oob= 读设备当前全部规则。
func (s *Server) handleOobRedirectList(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	oob := r.URL.Query().Get("oob")
	if agentID == "" || oob == "" {
		s.handleError(w, r, http.StatusBadRequest, "agent_id and oob query required")
		return
	}
	agent, ok := s.registry.Get(agentID)
	if !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}
	if !agent.HasCapability("oob") {
		s.handleError(w, r, http.StatusNotFound, "oob capability not available")
		return
	}
	conn, err := s.vaultOobTarget(oob)
	if err != nil {
		s.handleError(w, r, http.StatusServiceUnavailable, err.Error())
		return
	}
	resp, err := s.CallAgent(agentID, "oob.redirect.list", conn)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status == "error" {
		s.handleError(w, r, http.StatusBadGateway, "failed to list redirects")
		return
	}
	s.writeJSON(w, http.StatusOK, rpcResp.Data)
}

// oobRedirectSweepRequest 清扫请求：确认字段落在本包凭据条目名上——
// 清扫的破坏面是「该设备上全部已过期规则」，指认到 vault 条目即指认到设备。
type oobRedirectSweepRequest struct {
	AgentID    string `json:"agent_id"`
	Oob        string `json:"oob"`
	ConfirmOob string `json:"confirm_oob"`
}

// handleOobRedirectSweep POST /api/oob/redirect/sweep：双确认后清扫该设备上
// 全部已过期的 cockpit 规则（拆什么由 agent 按规则名里的过期时刻定，
// server 只做指认与留痕）。
func (s *Server) handleOobRedirectSweep(w http.ResponseWriter, r *http.Request) {
	var req oobRedirectSweepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AgentID == "" || req.Oob == "" {
		s.handleError(w, r, http.StatusBadRequest, "agent_id and oob required")
		return
	}
	if req.ConfirmOob != req.Oob {
		s.handleError(w, r, http.StatusBadRequest, "confirm_oob must match oob")
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
	conn, err := s.vaultOobTarget(req.Oob)
	if err != nil {
		s.handleError(w, r, http.StatusServiceUnavailable, err.Error())
		return
	}
	resp, err := s.CallAgent(req.AgentID, "oob.redirect.sweep", conn)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status == "error" {
		// 拒绝同走 ActionOobRedirect 留痕（与下发/拆除的拒绝同动作）
		s.auditRedirectRefuse(r, req.AgentID, "", rpcResp.Error)
		s.handleError(w, r, http.StatusBadGateway, "failed to sweep redirects")
		return
	}
	s.auditOobRedirectSweep(r, req, rpcResp.Data)
	s.writeJSON(w, http.StatusOK, rpcResp.Data)
}

// auditOobRedirectSweep 清扫审计：对象 vault 条目 + agent 回传的拆除清单。
func (s *Server) auditOobRedirectSweep(r *http.Request, req oobRedirectSweepRequest, data interface{}) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	details := map[string]interface{}{"oob": req.Oob}
	if m, ok := data.(map[string]interface{}); ok {
		if names, ok := m["removed"].([]interface{}); ok {
			removed := make([]string, 0, len(names))
			for _, n := range names {
				if v, ok := n.(string); ok {
					removed = append(removed, v)
				}
			}
			details["removed"] = removed
			details["count"] = len(removed)
		}
	}
	s.audit.LogResource(username, audit.ActionOobRedirectSweep, audit.ResourceOob, req.AgentID,
		details, s.getClientIP(r), r.UserAgent())
}

// auditOobRedirect 下发审计：规格 + agent 生成的规则名与过期时刻。
func (s *Server) auditOobRedirect(r *http.Request, req oobRedirectRequest, data interface{}) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	details := map[string]interface{}{
		"label": req.Spec.Label,
		"proto": req.Spec.Proto,
		"src":   req.Spec.SrcZone,
		"dest":  req.Spec.DstZone,
		"dst":   req.Spec.DstIP + ":" + strconv.Itoa(req.Spec.DstPort),
		"oob":   req.Oob,
	}
	if req.Spec.SrcIP != "" {
		details["src_ip"] = req.Spec.SrcIP
	}
	if m, ok := data.(map[string]interface{}); ok {
		if name, ok := m["name"].(string); ok {
			details["name"] = name
		}
		if expiry, ok := m["expiry"].(float64); ok {
			details["expiry"] = int64(expiry)
		}
	}
	s.audit.LogResource(username, audit.ActionOobRedirect, audit.ResourceOob, req.AgentID,
		details, s.getClientIP(r), r.UserAgent())
}

// auditOobRedirectRemove 拆除审计：规则名与所属 vault 条目。
func (s *Server) auditOobRedirectRemove(r *http.Request, req oobRedirectRemoveRequest, data interface{}) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	details := map[string]interface{}{"name": req.Name, "oob": req.Oob}
	s.audit.LogResource(username, audit.ActionOobRedirectRemove, audit.ResourceOob, req.AgentID,
		details, s.getClientIP(r), r.UserAgent())
}

// auditRedirectRefuse agent 拒绝时留痕：失败也要可回溯是谁试了下发。
// agent 错误文案可含规格字段但不含凭据（agent 侧错误已按此纪律构造）。
func (s *Server) auditRedirectRefuse(r *http.Request, agentID, label, msg string) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	details := map[string]interface{}{"refused": true, "reason": msg}
	if label != "" {
		details["label"] = label
	}
	s.audit.LogResource(username, audit.ActionOobRedirect, audit.ResourceOob, agentID,
		details, s.getClientIP(r), r.UserAgent())
}
