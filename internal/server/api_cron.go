package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// 定时任务管理 API（设计见 docs/guide/cron-design.md）。
// 挂在 /api/agents/{id}/cron/... 下（serveAPI 的 /agents/ 分支转发到这里）。
//
//	GET    /api/agents/{id}/cron/status        概览（运行用户 + 条目计数）
//	GET    /api/agents/{id}/cron/jobs          cockpit 名下任务 + 外部条目原文
//	GET    /api/agents/{id}/cron/timers        systemd timer 只读列表（cron-design M3）
//	PUT    /api/agents/{id}/cron/jobs/{name}   应用任务（审计 cron_apply）
//	DELETE /api/agents/{id}/cron/jobs/{name}   删除任务（审计 cron_delete）
//
// server 纯转发不落库（D9）：crontab 为唯一事实源。参数校验与 agent
// 同规则（双端防御）；agent 侧错误原样透传。

// 与 agent 侧 CronJob.validate 完全一致的校验规则（双端同规则）
var cronJobNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// cronPayload 应用任务的请求体
type cronPayload struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Enabled  bool   `json:"enabled"`
}

// validateCronExpr server 侧 cron 表达式校验（与 agent validateCronExpr 同规则）
func validateCronExpr(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("schedule is required")
	}
	if strings.HasPrefix(expr, "@") {
		switch expr {
		case "@reboot", "@hourly", "@daily", "@weekly", "@monthly", "@yearly", "@annually":
			return nil
		}
		return fmt.Errorf("unsupported @extension %q", expr)
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("schedule must have 5 fields (min hour dom mon dow), got %d", len(fields))
	}
	ranges := [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}
	for i, f := range fields {
		lo, hi := ranges[i][0], ranges[i][1]
		for _, part := range strings.Split(f, ",") {
			if !cronFieldPattern.MatchString(part) {
				return fmt.Errorf("invalid schedule field %d: %q", i+1, part)
			}
			base := part
			if idx := strings.Index(part, "/"); idx >= 0 {
				step, err := strconv.Atoi(part[idx+1:])
				if err != nil || step < 1 || step > hi {
					return fmt.Errorf("invalid step in schedule field %d", i+1)
				}
				base = part[:idx]
			}
			tok := base
			neg := ""
			if idx := strings.Index(base, "-"); idx > 0 {
				tok, neg = base[:idx], base[idx+1:]
			}
			for _, t := range []string{tok, neg} {
				if t == "" || t == "*" {
					continue
				}
				n, err := strconv.Atoi(t)
				if err != nil || n < lo || n > hi {
					return fmt.Errorf("schedule field %d value %s out of range [%d,%d]", i+1, t, lo, hi)
				}
			}
		}
	}
	return nil
}

// cronFieldPattern 单字段：数字/星号 + 可选范围与步长（与 agent cronFieldRe 同款）
var cronFieldPattern = regexp.MustCompile(`^(\*|[0-9]+)(-[0-9]+)?(/[0-9]+)?$`)

// validateCronJob 任务参数校验（与 agent 同规则）
func validateCronJob(j *cronPayload) error {
	if !cronJobNameRe.MatchString(j.Name) {
		return fmt.Errorf("invalid job name %q", j.Name)
	}
	if err := validateCronExpr(j.Schedule); err != nil {
		return err
	}
	if j.Command == "" {
		return fmt.Errorf("command is required")
	}
	if len(j.Command) > 4*1024 {
		return fmt.Errorf("command too large (max %d bytes)", 4*1024)
	}
	if strings.ContainsAny(j.Command, "\r\n") {
		return fmt.Errorf("command must not contain newlines (crontab is line-based)")
	}
	return nil
}

// handleAgentCronAPI 分发 /api/agents/{id}/cron/{sub}
// rest 是 "/agents/" 之后的部分（形如 "{agentID}/cron/jobs"）
func (s *Server) handleAgentCronAPI(w http.ResponseWriter, r *http.Request, rest string) {
	const suffix = "/cron/"
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
		s.forwardCronRPC(w, r, agentID, "cron.status", nil, "", nil)
	case sub == "jobs" && r.Method == http.MethodGet:
		s.forwardCronRPC(w, r, agentID, "cron.jobs", nil, "", nil)
	case sub == "timers" && r.Method == http.MethodGet:
		// systemd timer 只读列表（M3 D20）：纯转发，只读不审计
		s.forwardCronRPC(w, r, agentID, "cron.timers", nil, "", nil)
	case sub == "jobs/" || strings.HasPrefix(sub, "jobs/"):
		name := strings.TrimPrefix(sub, "jobs/")
		s.handleCronJob(w, r, agentID, name)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// handleCronJob 处理 jobs/{name} 两个端点
func (s *Server) handleCronJob(w http.ResponseWriter, r *http.Request, agentID, name string) {
	if !cronJobNameRe.MatchString(name) {
		s.handleError(w, r, http.StatusBadRequest, "invalid job name")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var payload cronPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
		// URL 与 body 的 name 必须一致
		if payload.Name != name {
			s.handleError(w, r, http.StatusBadRequest, "job name in URL and body must match")
			return
		}
		if err := validateCronJob(&payload); err != nil {
			s.handleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		details := map[string]interface{}{
			"schedule": payload.Schedule,
			"command":  payload.Command,
			"enabled":  payload.Enabled,
		}
		s.forwardCronRPC(w, r, agentID, "cron.job.apply",
			map[string]interface{}{"job": payload}, name, details)
	case http.MethodDelete:
		s.forwardCronRPC(w, r, agentID, "cron.job.delete",
			map[string]interface{}{"name": name}, name, map[string]interface{}{})
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// forwardCronRPC 转发 RPC 并透传结果
func (s *Server) forwardCronRPC(w http.ResponseWriter, r *http.Request, agentID, method string,
	params map[string]interface{}, jobName string, details map[string]interface{}) {

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
		// agent 错误原样透传（如外部条目自检失败的详情）
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

	if details != nil {
		username := "unknown"
		if userInfo, ok := auth.GetUserFromContext(r); ok {
			username = userInfo.Username
		}
		details["agent"] = agentID
		action := audit.ActionCronApply
		if r.Method == http.MethodDelete {
			action = audit.ActionCronDelete
		}
		s.audit.LogResource(username, action, audit.ResourceCronJob, jobName,
			details, s.getClientIP(r), r.UserAgent())
	}
	s.writeJSON(w, http.StatusOK, data)
}
