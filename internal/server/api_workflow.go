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

// Workflow 编排 API（设计见 docs/guide/workflow-design.md）。
//
//	GET|POST /api/workflows                 定义列表/创建
//	GET|PUT|DELETE /api/workflows/{id}      定义详情/更新/删除
//	POST /api/workflows/{id}/run            创建 run 并后台推进（W8 重入 409）
//	GET  /api/workflows/{id}/runs           该定义的 run 台账
//	GET  /api/workflow-runs/{rid}           run 详情（含步骤快照）
//	POST /api/workflow-runs/{rid}/cancel    取消 run（W3：停止推进后续步骤）
//
// 步骤即 Job（W5）：每步建真实 Job 进统一台账，编排器只是 server 侧
// goroutine 状态机，等的是与单条 Job 相同的 RPC。
type workflowPayload struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Steps       []jobCreatePayload `json:"steps"`
}

// workflowStepRun 步骤运行态快照（持久化在 run.Steps JSON 里）
type workflowStepRun struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Target          string `json:"target"`
	Status          string `json:"status"`
	JobID           string `json:"jobId,omitempty"`
	Attempts        int    `json:"attempts"`
	StopReason      string `json:"stopReason,omitempty"`
	ContinueOnError bool   `json:"continueOnError,omitempty"`
}

type stepOptions struct {
	TimeoutS        int
	ContinueOnError bool
	Retry           int
}

// handleWorkflowAPI 分发 /api/workflows[/{id}[/run|/runs]] 与 /api/workflow-runs/{rid}[/cancel]
// （挂在 api.go 中央分发器，认证由外层 /api/ 中间件统一承担）
func (s *Server) handleWorkflowAPI(w http.ResponseWriter, r *http.Request, path string) {
	if strings.HasPrefix(path, "/workflow-runs") {
		s.handleWorkflowRunsAPI(w, r, path)
		return
	}
	s.handleWorkflowsAPI(w, r, path)
}

// handleWorkflowsAPI 分发 /api/workflows[/{id}[/run|/runs]]
func (s *Server) handleWorkflowsAPI(w http.ResponseWriter, r *http.Request, path string) {
	switch {
	case path == "/workflows" && r.Method == http.MethodGet:
		s.handleWorkflowList(w, r)
	case path == "/workflows" && r.Method == http.MethodPost:
		s.handleWorkflowCreate(w, r)
	case strings.HasPrefix(path, "/workflows/"):
		rest := strings.TrimPrefix(path, "/workflows/")
		if rest == "" {
			s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
			return
		}
		if rest == "/run" || rest == "/runs" {
			s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
			return
		}
		switch {
		case strings.HasSuffix(rest, "/run") && r.Method == http.MethodPost:
			s.handleWorkflowRun(w, r, strings.TrimSuffix(rest, "/run"))
		case strings.HasSuffix(rest, "/runs") && r.Method == http.MethodGet:
			s.handleWorkflowRunsList(w, r, strings.TrimSuffix(rest, "/runs"))
		case strings.HasSuffix(rest, "/run") || strings.HasSuffix(rest, "/runs"):
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		case r.Method == http.MethodGet:
			s.handleWorkflowGet(w, r, rest)
		case r.Method == http.MethodPut:
			s.handleWorkflowUpdate(w, r, rest)
		case r.Method == http.MethodDelete:
			s.handleWorkflowDelete(w, r, rest)
		default:
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// handleWorkflowRunsAPI 分发 /api/workflow-runs/{rid}[/cancel]
func (s *Server) handleWorkflowRunsAPI(w http.ResponseWriter, r *http.Request, path string) {
	rest := strings.TrimPrefix(path, "/workflow-runs/")
	if rest == "" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	if id, ok := strings.CutSuffix(rest, "/cancel"); ok && id != "" {
		if r.Method != http.MethodPost {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleWorkflowRunCancel(w, r, id)
		return
	}
	if r.Method == http.MethodGet {
		s.handleWorkflowRunGet(w, r, rest)
		return
	}
	s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
}

// validateWorkflowPayload 定义校验：name ≤64、步骤 1-20 且逐项过 Job 校验
// （type 白名单/target/command/timeout 沿用 P1 各限，W12）；step.name 同
// workflow 内唯一
func validateWorkflowPayload(p *workflowPayload) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(p.Name) > 64 {
		return fmt.Errorf("name too long (max 64)")
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("steps must not be empty")
	}
	if len(p.Steps) > storage.WorkflowMaxSteps {
		return fmt.Errorf("too many steps (max %d)", storage.WorkflowMaxSteps)
	}
	seen := make(map[string]bool, len(p.Steps))
	for i := range p.Steps {
		step := &p.Steps[i]
		if err := validateJobPayload(step); err != nil {
			return fmt.Errorf("steps[%d]: %w", i, err)
		}
		// name 提取须类型断言——fmt.Sprint(缺失键) 得 "<nil>" 会漏校验
		_, name := stepOptionsFromParams(step)
		if name == "" {
			return fmt.Errorf("steps[%d]: parameters.name is required", i)
		}
		if seen[name] {
			return fmt.Errorf("steps[%d]: duplicate step name %q", i, name)
		}
		seen[name] = true
	}
	return nil
}

// stepOptionsFromParams 提取步骤可选参数（timeout_s/continue_on_error/retry/name）
func stepOptionsFromParams(p *jobCreatePayload) (stepOptions, string) {
	opts := stepOptions{}
	name := ""
	if p.Parameters != nil {
		if t, ok := p.Parameters["timeout_s"].(float64); ok {
			opts.TimeoutS = int(t)
		}
		if c, ok := p.Parameters["continue_on_error"].(bool); ok {
			opts.ContinueOnError = c
		}
		if rt, ok := p.Parameters["retry"].(float64); ok {
			opts.Retry = int(rt)
		}
		if n, ok := p.Parameters["name"].(string); ok {
			name = strings.TrimSpace(n)
		}
	}
	return opts, name
}

// sanitizeStepParams 剥离编排元参数（name/continue_on_error/retry），只留
// Job 类型自身参数下发 agent
func sanitizeStepParams(params map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(params))
	for k, v := range params {
		switch k {
		case "name", "continue_on_error", "retry":
		default:
			out[k] = v
		}
	}
	return out
}

// workflowView 定义视图：Steps 解析为对象返回
type workflowView struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Steps       []jobCreatePayload `json:"steps"`
	CreatedBy   string             `json:"createdBy"`
	CreatedAt   time.Time          `json:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt"`
}

func toWorkflowView(wf *storage.Workflow) workflowView {
	v := workflowView{
		ID:          wf.ID,
		Name:        wf.Name,
		Description: wf.Description,
		Steps:       []jobCreatePayload{},
		CreatedBy:   wf.CreatedBy,
		CreatedAt:   wf.CreatedAt,
		UpdatedAt:   wf.UpdatedAt,
	}
	if wf.Steps != "" {
		var steps []jobCreatePayload
		if json.Unmarshal([]byte(wf.Steps), &steps) == nil {
			v.Steps = steps
		}
	}
	return v
}

// workflowRunView run 视图：Steps 快照解析为对象返回
type workflowRunView struct {
	ID           string            `json:"id"`
	WorkflowID   string            `json:"workflowId"`
	WorkflowName string            `json:"workflowName"`
	Status       string            `json:"status"`
	Actor        string            `json:"actor"`
	Steps        []workflowStepRun `json:"steps"`
	CreatedAt    time.Time         `json:"createdAt"`
	FinishedAt   *time.Time        `json:"finishedAt,omitempty"`
}

func toWorkflowRunView(run *storage.WorkflowRun) workflowRunView {
	v := workflowRunView{
		ID:           run.ID,
		WorkflowID:   run.WorkflowID,
		WorkflowName: run.WorkflowName,
		Status:       run.Status,
		Actor:        run.Actor,
		Steps:        []workflowStepRun{},
		CreatedAt:    run.CreatedAt,
		FinishedAt:   run.FinishedAt,
	}
	if run.Steps != "" {
		var steps []workflowStepRun
		if json.Unmarshal([]byte(run.Steps), &steps) == nil {
			v.Steps = steps
		}
	}
	return v
}

func (s *Server) handleWorkflowList(w http.ResponseWriter, r *http.Request) {
	wfs, err := s.db.ListWorkflows()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to list workflows")
		return
	}
	out := make([]workflowView, 0, len(wfs))
	for i := range wfs {
		out = append(out, toWorkflowView(&wfs[i]))
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"workflows": out})
}

func (s *Server) handleWorkflowGet(w http.ResponseWriter, r *http.Request, id string) {
	wf, err := s.db.GetWorkflow(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "workflow not found")
		return
	}
	s.writeJSON(w, http.StatusOK, toWorkflowView(wf))
}

// decodeWorkflowPayload 公共入参解码（创建/更新共用）
func (s *Server) decodeWorkflowPayload(w http.ResponseWriter, r *http.Request) (*workflowPayload, bool) {
	var payload workflowPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return nil, false
	}
	if err := validateWorkflowPayload(&payload); err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return &payload, true
}

func (s *Server) handleWorkflowCreate(w http.ResponseWriter, r *http.Request) {
	payload, ok := s.decodeWorkflowPayload(w, r)
	if !ok {
		return
	}
	username := actorName(r)
	stepsJSON, _ := json.Marshal(payload.Steps)
	now := time.Now()
	wf := &storage.Workflow{
		ID:          protocol.GenerateID(),
		Name:        payload.Name,
		Description: payload.Description,
		Steps:       string(stepsJSON),
		CreatedBy:   username,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.db.CreateWorkflow(wf); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to create workflow")
		return
	}
	s.audit.LogResource(username, audit.ActionWorkflowCreate, audit.ResourceWorkflow, wf.ID,
		map[string]interface{}{"name": wf.Name, "steps": len(payload.Steps)},
		s.getClientIP(r), r.UserAgent())
	s.writeJSON(w, http.StatusCreated, toWorkflowView(wf))
}

func (s *Server) handleWorkflowUpdate(w http.ResponseWriter, r *http.Request, id string) {
	payload, ok := s.decodeWorkflowPayload(w, r)
	if !ok {
		return
	}
	wf, err := s.db.GetWorkflow(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "workflow not found")
		return
	}
	// active run 在途时定义不得变更（推进中的编排器读的是自己的快照，但
	// 台账一致性要求运行期间定义冻结）
	if active, err := s.db.ActiveWorkflowRun(id); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to check active run")
		return
	} else if active != nil {
		s.handleError(w, r, http.StatusConflict, "workflow has an active run; wait for it to finish")
		return
	}
	username := actorName(r)
	stepsJSON, _ := json.Marshal(payload.Steps)
	wf.Name = payload.Name
	wf.Description = payload.Description
	wf.Steps = string(stepsJSON)
	wf.UpdatedAt = time.Now()
	if err := s.db.UpdateWorkflow(wf); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to update workflow")
		return
	}
	s.audit.LogResource(username, audit.ActionWorkflowUpdate, audit.ResourceWorkflow, wf.ID,
		map[string]interface{}{"name": wf.Name, "steps": len(payload.Steps)},
		s.getClientIP(r), r.UserAgent())
	s.writeJSON(w, http.StatusOK, toWorkflowView(wf))
}

func (s *Server) handleWorkflowDelete(w http.ResponseWriter, r *http.Request, id string) {
	wf, err := s.db.GetWorkflow(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "workflow not found")
		return
	}
	if active, err := s.db.ActiveWorkflowRun(id); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to check active run")
		return
	} else if active != nil {
		s.handleError(w, r, http.StatusConflict, "workflow has an active run; wait for it to finish")
		return
	}
	if err := s.db.DeleteWorkflow(id); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to delete workflow")
		return
	}
	username := actorName(r)
	s.audit.LogResource(username, audit.ActionWorkflowDelete, audit.ResourceWorkflow, id,
		map[string]interface{}{"name": wf.Name}, s.getClientIP(r), r.UserAgent())
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

// handleWorkflowRun 创建 run 并后台推进（W8：同定义重入 409）
func (s *Server) handleWorkflowRun(w http.ResponseWriter, r *http.Request, id string) {
	wf, err := s.db.GetWorkflow(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "workflow not found")
		return
	}
	if active, err := s.db.ActiveWorkflowRun(id); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to check active run")
		return
	} else if active != nil {
		s.handleError(w, r, http.StatusConflict, "workflow is already running")
		return
	}
	// 步骤目标 agent 全部在线才建 run（与单条 Job 同口径：不建幽灵）
	stepPayloads := parseWorkflowSteps(wf)
	for i := range stepPayloads {
		step := &stepPayloads[i]
		if _, ok := s.registry.Get(step.Target); !ok {
			s.handleError(w, r, http.StatusServiceUnavailable,
				fmt.Sprintf("agent offline for steps[%d]: %s", i, step.Target))
			return
		}
	}

	username := actorName(r)
	var steps []workflowStepRun
	for _, step := range stepPayloads {
		opts, name := stepOptionsFromParams(&step)
		steps = append(steps, workflowStepRun{
			Name:            name,
			Type:            step.Type,
			Target:          step.Target,
			Status:          storage.JobStatusPending,
			ContinueOnError: opts.ContinueOnError,
		})
	}
	stepsJSON, _ := json.Marshal(steps)
	run := &storage.WorkflowRun{
		ID:           protocol.GenerateID(),
		WorkflowID:   wf.ID,
		WorkflowName: wf.Name,
		Status:       storage.WorkflowRunStatusRunning,
		Actor:        username,
		Steps:        string(stepsJSON),
		CreatedAt:    time.Now(),
	}
	if err := s.db.CreateWorkflowRun(run); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to create run")
		return
	}

	go s.advanceWorkflow(wf, run, username)

	s.writeJSON(w, http.StatusCreated, toWorkflowRunView(run))
}

// parseWorkflowSteps 解析定义的步骤数组（解析失败返回空——建行前已过校验）
func parseWorkflowSteps(wf *storage.Workflow) []jobCreatePayload {
	var steps []jobCreatePayload
	if wf.Steps != "" {
		_ = json.Unmarshal([]byte(wf.Steps), &steps)
	}
	return steps
}

// workflowRetryInterval 步骤重试间隔；var 便于测试缩短等待（同
// backupTrackInterval 惯例）
var workflowRetryInterval = 5 * time.Second

// advanceWorkflow 编排器：逐步建 Job → 派发 → 按重试/继续语义推进（W4/W5）。
// 每个 attempt 都是真实 Job（台账可查）；重试固定间隔 5s。
func (s *Server) advanceWorkflow(wf *storage.Workflow, run *storage.WorkflowRun, actor string) {
	steps := parseWorkflowSteps(wf)
	for i := range steps {
		step := &steps[i]
		opts, _ := stepOptionsFromParams(step)

		// 取消检查：cancelWorkflowRun 已把 run.Steps 停在第 i 步
		current, err := s.db.GetWorkflowRun(run.ID)
		if err != nil {
			log.Printf("workflow: load run %s failed: %v", run.ID, err)
			return
		}
		if current.Status != storage.WorkflowRunStatusRunning {
			return // 已被取消，不再推进
		}

		job, finalStatus, attempts := s.runWorkflowStep(step, opts, run.ID, actor)
		s.recordStepResult(run, i, job, finalStatus, attempts)

		if finalStatus != storage.JobStatusSuccess && !opts.ContinueOnError {
			s.finishWorkflowRun(run, storage.WorkflowRunStatusFailed, i,
				fmt.Sprintf("step %d (%s) failed", i, step.Type), actor)
			return
		}
	}
	s.finishWorkflowRun(run, storage.WorkflowRunStatusSuccess, len(steps)-1, "", actor)
}

// runWorkflowStep 执行单步（含重试）；返回终态 Job、归纳状态与实跑尝试数。
// 每次尝试建独立 Job 行（尝试历史进台账）。
func (s *Server) runWorkflowStep(step *jobCreatePayload, opts stepOptions, runID, actor string) (*storage.Job, string, int) {
	maxAttempts := opts.Retry + 1
	var job *storage.Job
	status := storage.JobStatusFailed
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(workflowRetryInterval)
		}
		params := sanitizeStepParams(step.Parameters)
		paramsJSON, _ := json.Marshal(params)
		now := time.Now()
		job = &storage.Job{
			ID:            protocol.GenerateID(),
			Type:          step.Type,
			Target:        step.Target,
			Actor:         "workflow",
			Parameters:    string(paramsJSON),
			Status:        storage.JobStatusPending,
			WorkflowRunID: runID,
			CreatedAt:     now,
		}
		if err := s.db.CreateJob(job); err != nil {
			log.Printf("workflow: create step job failed: %v", err)
			continue
		}
		job.Status = storage.JobStatusRunning
		started := time.Now()
		job.StartedAt = &started
		_ = s.db.UpdateJob(job)

		success, jobErr := s.dispatchJob(job, *step)

		finished := time.Now()
		job.FinishedAt = &finished
		job.Output, job.Error, job.ExitCode = jobErr.Output, jobErr.Message, jobErr.ExitCode
		if success {
			job.Status = storage.JobStatusSuccess
		} else {
			job.Status = storage.JobStatusFailed
		}
		_ = s.db.UpdateJob(job)

		s.audit.LogResource(actor, audit.ActionJobRun, audit.ResourceJob, job.ID,
			map[string]interface{}{
				"type": job.Type, "target": job.Target, "status": job.Status,
				"params": params, "workflow_run_id": runID,
			}, "", "")

		if success {
			return job, storage.JobStatusSuccess, attempt + 1
		}
		status = storage.JobStatusFailed
	}
	return job, status, maxAttempts
}

// recordStepResult 回写步骤快照（状态/attempts/jobId）
func (s *Server) recordStepResult(run *storage.WorkflowRun, idx int, job *storage.Job, status string, attempts int) {
	run, err := s.db.GetWorkflowRun(run.ID)
	if err != nil {
		log.Printf("workflow: load run for step result failed: %v", err)
		return
	}
	var steps []workflowStepRun
	if json.Unmarshal([]byte(run.Steps), &steps) == nil && idx < len(steps) {
		steps[idx].Status = status
		steps[idx].Attempts = attempts
		if job != nil {
			steps[idx].JobID = job.ID
		}
		if status != storage.JobStatusSuccess {
			steps[idx].StopReason = "failed"
		}
		if b, err := json.Marshal(steps); err == nil {
			run.Steps = string(b)
			_ = s.db.UpdateWorkflowRun(run)
		}
	}
}

// finishWorkflowRun run 落终态 + 审计（断点记 steps 摘要）
func (s *Server) finishWorkflowRun(run *storage.WorkflowRun, status string, failedIdx int, reason, actor string) {
	run, err := s.db.GetWorkflowRun(run.ID)
	if err != nil {
		log.Printf("workflow: load run for finish failed: %v", err)
		return
	}
	run.Status = status
	finished := time.Now()
	run.FinishedAt = &finished
	if err := s.db.UpdateWorkflowRun(run); err != nil {
		log.Printf("workflow: update run final state failed: %v", err)
		return
	}
	details := map[string]interface{}{
		"workflow":  run.WorkflowName,
		"status":    status,
		"steps_run": failedIdx + 1,
	}
	if reason != "" {
		details["reason"] = reason
	}
	s.audit.LogResource(actor, audit.ActionWorkflowRun, audit.ResourceWorkflow, run.ID,
		details, "", "")
}

// handleWorkflowRunsList 某定义的 run 台账
func (s *Server) handleWorkflowRunsList(w http.ResponseWriter, r *http.Request, id string) {
	if _, err := s.db.GetWorkflow(id); err != nil {
		s.handleError(w, r, http.StatusNotFound, "workflow not found")
		return
	}
	runs, err := s.db.ListWorkflowRuns(id)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to list runs")
		return
	}
	out := make([]workflowRunView, 0, len(runs))
	for i := range runs {
		out = append(out, toWorkflowRunView(&runs[i]))
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"runs": out})
}

func (s *Server) handleWorkflowRunGet(w http.ResponseWriter, r *http.Request, id string) {
	run, err := s.db.GetWorkflowRun(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "run not found")
		return
	}
	s.writeJSON(w, http.StatusOK, toWorkflowRunView(run))
}

// handleWorkflowRunCancel 取消 run（W3）：置 cancelled——编排器在下一步推进
// 前检查该状态即停；当前在途步骤同单条 Job 语义（等它自然结束，结果记
// cancelled 步骤状态不受影响）
func (s *Server) handleWorkflowRunCancel(w http.ResponseWriter, r *http.Request, id string) {
	run, err := s.db.GetWorkflowRun(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "run not found")
		return
	}
	if run.Status != storage.WorkflowRunStatusRunning {
		s.handleError(w, r, http.StatusConflict, "run is not running")
		return
	}
	run.Status = storage.WorkflowRunStatusCancelled
	finished := time.Now()
	run.FinishedAt = &finished
	if err := s.db.UpdateWorkflowRun(run); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to cancel run")
		return
	}
	username := actorName(r)
	s.audit.LogResource(username, audit.ActionWorkflowCancel, audit.ResourceWorkflow, id,
		map[string]interface{}{"workflow": run.WorkflowName},
		s.getClientIP(r), r.UserAgent())
	s.writeJSON(w, http.StatusOK, toWorkflowRunView(run))
}

// actorName 提取请求用户名（无 auth 上下文回退 unknown；后台 goroutine 传 nil）
func actorName(r *http.Request) string {
	if r == nil {
		return "workflow"
	}
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		return userInfo.Username
	}
	return "unknown"
}
