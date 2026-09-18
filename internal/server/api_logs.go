package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// 远程日志查询 API（设计见 docs/guide/logs-design.md）。
// 挂在 /api/agents/{id}/logs/... 下（serveAPI 的 /agents/ 分支转发到这里）。
//
//	GET  /api/agents/{id}/logs/status   两类日志源可用性
//	GET  /api/agents/{id}/logs/sources  运行中 systemd 服务 / docker 容器
//	POST /api/agents/{id}/logs/query    查询日志（tail/since/grep）
//	POST /api/agents/{id}/logs/follow   实时尾随（NDJSON 流，见 api_logs_follow.go）
//
// server 纯转发不落库；查询类操作不记审计（D9，与文件浏览同纪律）；
// 参数校验与 agent 同规则（双端防御）；agent 侧错误原样透传。

// logsSourcePattern 日志源白名单（与 agent logsSourceRe 同款）
var logsSourcePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.@+-]{0,127}$`)

// logsQueryPayload 查询请求体（与 agent LogsQuery 同规则）
type logsQueryPayload struct {
	Type         string `json:"type"`
	Source       string `json:"source"`
	Tail         int    `json:"tail"`
	SinceMinutes int    `json:"since_minutes"`
	Grep         string `json:"grep"`
}

const (
	logsMaxTail         = 2000
	logsMaxSinceMinutes = 1440
	logsMaxGrep         = 256
)

// validateLogsQuery 与 agent LogsQuery.validate 同规则
func validateLogsQuery(q *logsQueryPayload) error {
	if q.Type != "systemd" && q.Type != "docker" {
		return fmt.Errorf("type must be systemd or docker, got %q", q.Type)
	}
	if !logsSourcePattern.MatchString(q.Source) {
		return fmt.Errorf("invalid source name %q", q.Source)
	}
	if q.Tail == 0 {
		q.Tail = 200
	}
	if q.Tail < 1 || q.Tail > logsMaxTail {
		return fmt.Errorf("tail out of range [1, %d]", logsMaxTail)
	}
	if q.SinceMinutes < 0 || q.SinceMinutes > logsMaxSinceMinutes {
		return fmt.Errorf("since_minutes out of range [0, %d]", logsMaxSinceMinutes)
	}
	if len(q.Grep) > logsMaxGrep {
		return fmt.Errorf("grep too long (max %d bytes)", logsMaxGrep)
	}
	if strings.ContainsAny(q.Grep, "\r\n") {
		return fmt.Errorf("grep must not contain newlines")
	}
	return nil
}

// handleAgentLogsAPI 分发 /api/agents/{id}/logs/{sub}
// rest 是 "/agents/" 之后的部分（形如 "{agentID}/logs/query"）
func (s *Server) handleAgentLogsAPI(w http.ResponseWriter, r *http.Request, rest string) {
	const suffix = "/logs/"
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
		s.forwardLogsRPC(w, r, agentID, "logs.status", nil)
	case sub == "sources" && r.Method == http.MethodGet:
		s.forwardLogsRPC(w, r, agentID, "logs.sources", nil)
	case sub == "query" && r.Method == http.MethodPost:
		var payload logsQueryPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
		if err := validateLogsQuery(&payload); err != nil {
			s.handleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		s.forwardLogsRPC(w, r, agentID, "logs.query",
			map[string]interface{}{
				"query": map[string]interface{}{
					"type":          payload.Type,
					"source":        payload.Source,
					"tail":          payload.Tail,
					"since_minutes": payload.SinceMinutes,
					"grep":          payload.Grep,
				},
			})
	case sub == "follow" && r.Method == http.MethodPost:
		s.handleAgentLogsFollow(w, r, agentID)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// forwardLogsRPC 转发 RPC 并透传结果
func (s *Server) forwardLogsRPC(w http.ResponseWriter, r *http.Request, agentID, method string,
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
		// agent 错误原样透传（如 journalctl 权限问题详情）
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
