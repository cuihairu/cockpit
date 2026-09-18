package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// 防漂移检测 API（设计见 docs/guide/drift-design.md）。
// 挂在 /api/agents/{id}/drift/... 下（serveAPI 的 /agents/ 分支转发到这里）：
//
//	POST /api/agents/{id}/drift/check  全量比对（nginx/cron/stack 基线 vs 当前）
//	POST /api/agents/{id}/drift/diff   单对象两侧全文（M3，{kind, name}）
//	POST /api/agents/{id}/drift/record 手动登记基线「以当前为准」（M4，记审计）
//
// server 纯转发不落库（D11）；浏览类操作（check/diff）不记审计；agent 侧
// 错误原样透传。

// driftDiffKinds drift.diff 允许的对象类型（与 agent 侧白名单一致）
var driftDiffKinds = map[string]bool{"nginx": true, "traefik": true, "cron": true, "stack": true}

// driftDiffMaxNameLen diff 目标 name 长度上限（与 agent 侧同规则，双端防御）
const driftDiffMaxNameLen = 128

// handleDriftConfig 全局巡检配置：GET 返回当前间隔与范围；PUT 校验后写入
// Setting（见 drift-design.md M2/D17）。路径全局（/api/drift/config），
// 不挂在 /agents/{id} 下——巡检配置非 per-agent。
func (s *Server) handleDriftConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"scan_interval_seconds": s.GetDriftScanInterval(),
			"min":                   driftMinIntervalSeconds,
			"max":                   driftMaxIntervalSeconds,
			"default":               driftDefaultInterval,
		})
	case http.MethodPut:
		var req struct {
			ScanIntervalSeconds int `json:"scan_interval_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
		if err := s.SetDriftScanInterval(req.ScanIntervalSeconds); err != nil {
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
	case sub == "diff":
		s.handleDriftDiff(w, r, agentID)
	case sub == "record":
		s.handleDriftRecord(w, r, agentID)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// decodeDriftTarget 解析并校验 {kind, name}（diff 与 record 共用，双端同规则）
func decodeDriftTarget(r *http.Request) (kind, name string, errMsg string, code int) {
	var req struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return "", "", "Invalid request body", http.StatusBadRequest
	}
	if !driftDiffKinds[req.Kind] {
		return "", "", "unknown kind (want nginx, traefik, cron or stack)", http.StatusBadRequest
	}
	if req.Name == "" {
		return "", "", "name required", http.StatusBadRequest
	}
	if len(req.Name) > driftDiffMaxNameLen {
		return "", "", fmt.Sprintf("name too long (max %d bytes)", driftDiffMaxNameLen), http.StatusBadRequest
	}
	return req.Kind, req.Name, "", 0
}

// handleDriftDiff POST /drift/diff：{kind, name} 与 agent 同规则校验后转发
// （M3/D21）。浏览性质不审计，与 check 一致。
func (s *Server) handleDriftDiff(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	kind, name, errMsg, code := decodeDriftTarget(r)
	if errMsg != "" {
		s.handleError(w, r, code, errMsg)
		return
	}
	s.forwardDriftRPC(w, r, agentID, "drift.diff", map[string]interface{}{
		"kind": kind,
		"name": name,
	})
}

// handleDriftRecord POST /drift/record：以当前内容登记基线（M4/D24）。
// 用户主动变更漂移判定标准——仅成功转发后记审计（与 check/diff 浏览分流）。
func (s *Server) handleDriftRecord(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	kind, name, errMsg, code := decodeDriftTarget(r)
	if errMsg != "" {
		s.handleError(w, r, code, errMsg)
		return
	}
	if !s.forwardDriftRPC(w, r, agentID, "drift.record", map[string]interface{}{
		"kind": kind,
		"name": name,
	}) {
		return
	}
	s.auditDriftRecord(r, agentID, kind, name)
}

// auditDriftRecord 登记基线审计（resourceID = kind/name，detail 记 agent）
func (s *Server) auditDriftRecord(r *http.Request, agentID, kind, name string) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, audit.ActionDriftRecord, audit.ResourceDriftBaseline,
		kind+"/"+name,
		map[string]interface{}{"agent": agentID},
		s.getClientIP(r), r.UserAgent())
}

// forwardDriftRPC 转发 RPC 并透传结果（与 forwardLogsRPC 同构）；返回是否
// 成功（record 据此决定是否记审计——agent 失败不产生审计记录）
func (s *Server) forwardDriftRPC(w http.ResponseWriter, r *http.Request, agentID, method string,
	params map[string]interface{}) bool {

	resp, err := s.CallAgent(agentID, method, params)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return false
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "agent returned invalid response")
		return false
	}
	if rpcResp.Status == "error" {
		msg := rpcResp.Error
		if msg == "" {
			msg = "agent rejected the operation"
		}
		s.handleError(w, r, http.StatusBadGateway, msg)
		return false
	}
	data, _ := rpcResp.Data.(map[string]interface{})
	if data == nil {
		data = map[string]interface{}{}
	}
	s.writeJSON(w, http.StatusOK, data)
	return true
}
