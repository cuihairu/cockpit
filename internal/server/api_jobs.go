package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 执行 Job API（设计见 docs/guide/jobs-design.md；异步化见 workflow-design.md W1）。
//
//	POST /api/jobs             创建 Job，后台执行，立即 201 返回 pending 视图
//	GET  /api/jobs             最近 Job 列表（倒序，limit 50；支持过滤）
//	GET  /api/jobs/{id}        单条详情（含输出）
//	POST /api/jobs/{id}/cancel 取消 pending Job（workflow-design W3）
//
// 执行流：校验 → 建 Job(pending) → 后台 goroutine CallAgent(job.exec) →
// 终态 success/failed 回写 + 审计 job_run。目标 agent 离线一律 503（先查
// registry，与 cron 同口径）。
type jobCreatePayload struct {
	Type       string                 `json:"type"`
	Target     string                 `json:"target"`
	Targets    []string               `json:"targets,omitempty"` // 多目标扇出（M2b F1）：仅 workflow steps 合法
	Parameters map[string]interface{} `json:"parameters"`
}

// jobTypes 允许的 Job 类型 → agent RPC method（最小闭环仅 agent.exec；
// 新类型在此登记并补文档/前端类型映射）
var jobTypes = map[string]string{
	"agent.exec": "job.exec",
}

// handleJobsAPI 分发 /api/jobs[/{id}[/cancel]]
func (s *Server) handleJobsAPI(w http.ResponseWriter, r *http.Request, path string) {
	switch {
	case path == "/jobs" && r.Method == http.MethodGet:
		s.handleJobsList(w, r)
	case path == "/jobs" && r.Method == http.MethodPost:
		s.handleJobCreate(w, r)
	case strings.HasPrefix(path, "/jobs/"):
		rest := strings.TrimPrefix(path, "/jobs/")
		if rest == "" {
			s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
			return
		}
		// POST /api/jobs/{id}/cancel：pending 未派发可撤（workflow-design W3）
		if id, ok := strings.CutSuffix(rest, "/cancel"); ok && id != "" {
			if r.Method != http.MethodPost {
				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			s.handleJobCancel(w, r, id)
			return
		}
		if r.Method == http.MethodGet {
			s.handleJobGet(w, r, rest)
			return
		}
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// validateJobPayload 入参加校验（type/target/parameters 结构与长度）
func validateJobPayload(p *jobCreatePayload) error {
	if p.Type == "" {
		return fmt.Errorf("type is required")
	}
	if _, ok := jobTypes[p.Type]; !ok {
		return fmt.Errorf("unsupported job type %q (allowed: agent.exec)", p.Type)
	}
	p.Target = strings.TrimSpace(p.Target)
	if p.Target == "" && len(p.Targets) == 0 {
		return fmt.Errorf("target (agent id) is required")
	}
	if p.Type == "agent.exec" {
		cmd, _ := p.Parameters["command"].(string)
		if strings.TrimSpace(cmd) == "" {
			return fmt.Errorf("parameters.command is required")
		}
		if len(cmd) > 16*1024 {
			return fmt.Errorf("command too large (max 16KB)")
		}
		if t, ok := p.Parameters["timeout_s"].(float64); ok && (t <= 0 || t > 300) {
			return fmt.Errorf("timeout_s out of range (1-300)")
		}
	}
	return nil
}

// handleJobsList 最近 Job 列表（倒序；status/target/type/workflow_run_id 过滤）
func (s *Server) handleJobsList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	jobs, err := s.db.ListJobsFiltered(
		q.Get("status"), q.Get("target"), q.Get("type"), q.Get("workflow_run_id"), 50)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to list jobs")
		return
	}
	out := make([]jobView, 0, len(jobs))
	for i := range jobs {
		out = append(out, toJobView(&jobs[i]))
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": out})
}

// jobView API 视图：Parameters 解析为对象返回（库内存 JSON 串）
type jobView struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Target     string                 `json:"target"`
	Actor      string                 `json:"actor"`
	Status     string                 `json:"status"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Output     string                 `json:"output,omitempty"`
	ExitCode   int                    `json:"exitCode,omitempty"`
	Error      string                 `json:"error,omitempty"`
	CreatedAt  time.Time              `json:"createdAt"`
	StartedAt  *time.Time             `json:"startedAt,omitempty"`
	FinishedAt *time.Time             `json:"finishedAt,omitempty"`
}

func toJobView(j *storage.Job) jobView {
	v := jobView{
		ID:         j.ID,
		Type:       j.Type,
		Target:     j.Target,
		Actor:      j.Actor,
		Status:     j.Status,
		Output:     j.Output,
		ExitCode:   j.ExitCode,
		Error:      j.Error,
		CreatedAt:  j.CreatedAt,
		StartedAt:  j.StartedAt,
		FinishedAt: j.FinishedAt,
	}
	if j.Parameters != "" {
		var m map[string]interface{}
		if json.Unmarshal([]byte(j.Parameters), &m) == nil {
			v.Parameters = m
		}
	}
	return v
}

// handleJobGet 单条详情
func (s *Server) handleJobGet(w http.ResponseWriter, r *http.Request, id string) {
	job, err := s.db.GetJob(id)
	if err != nil {
		// 唯一主键查询：查不到即 404（无其它失败面）
		s.handleError(w, r, http.StatusNotFound, "job not found")
		return
	}
	s.writeJSON(w, http.StatusOK, toJobView(job))
}

// handleJobCreate 创建 Job，后台执行（workflow-design W1：创建即返回 pending，
// dispatch 转 goroutine；消费方经台账/详情轮询终态）
func (s *Server) handleJobCreate(w http.ResponseWriter, r *http.Request) {
	var payload jobCreatePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := validateJobPayload(&payload); err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	// targets 扇出仅 workflow steps 合法（M2b F1）：standalone Job 单目标语义
	if len(payload.Targets) > 0 {
		s.handleError(w, r, http.StatusBadRequest, "targets is only valid in workflow steps")
		return
	}

	// 目标 agent 必须在线（与 cron 同口径：离线直接 503，不建幽灵记录）
	if _, ok := s.registry.Get(payload.Target); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}

	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}

	paramsJSON, _ := json.Marshal(payload.Parameters)
	job := &storage.Job{
		ID:         protocol.GenerateID(),
		Type:       payload.Type,
		Target:     payload.Target,
		Actor:      username,
		Parameters: string(paramsJSON),
		Status:     storage.JobStatusPending,
		CreatedAt:  time.Now(),
	}
	if err := s.db.CreateJob(job); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to create job")
		return
	}

	// 视图在派发前取定：goroutine 会原地改 job 状态（pending→running→终态），
	// 先快照保证 201 响应体恒为 pending（W1 契约），并与后台写互不竞争
	view := toJobView(job)

	// 后台派发：终态回写 + 审计在 goroutine 内完成（HTTP 请求即时返回）
	go s.dispatchJobAsync(job, payload, username)

	s.writeJSON(w, http.StatusCreated, view)
}

// dispatchJobAsync 后台派发单条 Job：pending → running → 终态回写 + 审计
func (s *Server) dispatchJobAsync(job *storage.Job, payload jobCreatePayload, actor string) {
	now := time.Now()
	job.Status = storage.JobStatusRunning
	job.StartedAt = &now
	if err := s.db.UpdateJob(job); err != nil {
		// 状态无法回写不阻碍执行（下一次 UpdateJob 会覆盖）
		log.Printf("jobs: update running state failed: %v", err)
	}

	success, jobErr := s.dispatchJob(job, payload)

	finished := time.Now()
	job.FinishedAt = &finished
	job.Output, job.Error, job.ExitCode = jobErr.Output, jobErr.Message, jobErr.ExitCode
	if success {
		job.Status = storage.JobStatusSuccess
	} else {
		job.Status = storage.JobStatusFailed
	}
	if err := s.db.UpdateJob(job); err != nil {
		log.Printf("jobs: update final state failed: %v", err)
	}

	// 审计：命令入参进 details（与 cron_apply 同口径），不涉及凭据
	details := map[string]interface{}{
		"type":   job.Type,
		"target": job.Target,
		"status": job.Status,
		"params": payload.Parameters,
	}
	s.audit.LogResource(actor, audit.ActionJobRun, audit.ResourceJob, job.ID,
		details, "", "")
}

// handleJobCancel 取消 pending Job（workflow-design W3：派发前可撤；
// running 已在途——RPC 无取消帧，job.exec 上限 300s 会自然结束，如实拒绝）
func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request, id string) {
	job, err := s.db.GetJob(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "job not found")
		return
	}
	if job.Status != storage.JobStatusPending {
		s.handleError(w, r, http.StatusConflict,
			"only pending jobs can be cancelled; running jobs finish on their own (max 300s)")
		return
	}
	if err := s.db.CancelJob(id, time.Now()); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to cancel job")
		return
	}

	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, audit.ActionJobCancel, audit.ResourceJob, id,
		map[string]interface{}{"type": job.Type, "target": job.Target},
		s.getClientIP(r), r.UserAgent())

	updated, err := s.db.GetJob(id)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to load job")
		return
	}
	s.writeJSON(w, http.StatusOK, toJobView(updated))
}

// jobExecError RPC 执行结果（成功性 + 输出/退出码/错误讯息）
type jobExecError struct {
	Output   string
	Message  string
	ExitCode int
}

// dispatchJob 经 CallAgent 下发 job.<type> 动作并归纳执行结果
func (s *Server) dispatchJob(job *storage.Job, payload jobCreatePayload) (bool, jobExecError) {
	method := jobTypes[job.Type]
	fail := func(msg string) (bool, jobExecError) {
		return false, jobExecError{Message: msg, ExitCode: -1}
	}

	resp, err := s.CallAgent(job.Target, method, payload.Parameters)
	if err != nil {
		return fail("failed to reach agent: " + err.Error())
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		return fail("agent returned invalid response")
	}
	if rpcResp.Status == "error" {
		msg := rpcResp.Error
		if msg == "" {
			msg = "agent rejected the operation"
		}
		return fail(msg)
	}
	data, _ := rpcResp.Data.(map[string]interface{})
	if data == nil {
		data = map[string]interface{}{}
	}
	je := jobExecError{
		Output:   stringField(data, "output"),
		Message:  stringField(data, "error"),
		ExitCode: intField(data, "exit_code"),
	}
	// 非零退出码/异常讯息 → failed（输出仍带回展示）
	if je.ExitCode != 0 || je.Message != "" {
		return false, je
	}
	return true, je
}

func stringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func intField(m map[string]interface{}, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}
