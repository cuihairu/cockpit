package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
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
	Name            string                  `json:"name"`
	Type            string                  `json:"type"`
	Target          string                  `json:"target"`
	Targets         []workflowStepTargetRun `json:"targets,omitempty"` // 扇出步逐台结果（M2b F6，单目标步恒空）
	Status          string                  `json:"status"`
	JobID           string                  `json:"jobId,omitempty"`
	Attempts        int                     `json:"attempts"`
	StopReason      string                  `json:"stopReason,omitempty"`
	ContinueOnError bool                    `json:"continueOnError,omitempty"`
}

// workflowStepTargetRun 扇出步单台结果（M2b F6）：Status 用 JobStatus* 同款
// 词表（pending=未跑到，cancelled=run 取消时未跑到的目标）
type workflowStepTargetRun struct {
	AgentID    string `json:"agentId"`
	Status     string `json:"status"`
	JobID      string `json:"jobId,omitempty"`
	Attempts   int    `json:"attempts"`
	StopReason string `json:"stopReason,omitempty"`
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

// stepOutputRefRe 步骤输出引用 {{steps.NAME.output}}（M2a V1）。NAME 为
// 步骤 name；仅此一种 token 形态参与替换，其余 {{...}} 原样保留（V6）。
var stepOutputRefRe = regexp.MustCompile(`\{\{steps\.([A-Za-z0-9_-]+)\.output\}\}`)

// collectStepRefs 递归收集参数树 string 值里的输出引用名
func collectStepRefs(v interface{}, into map[string]bool) {
	switch t := v.(type) {
	case string:
		for _, m := range stepOutputRefRe.FindAllStringSubmatch(t, -1) {
			into[m[1]] = true
		}
	case map[string]interface{}:
		for _, vv := range t {
			collectStepRefs(vv, into)
		}
	case []interface{}:
		for _, vv := range t {
			collectStepRefs(vv, into)
		}
	}
}

// validateWorkflowPayload 定义校验：name ≤64、步骤 1-20 且逐项过 Job 校验
// （type 白名单/target/command/timeout 沿用 P1 各限，W12）；step.name 同
// workflow 内唯一；输出引用必须指向更靠前的步骤（M2a V2——顺序链语义下
// 前向/自引用运行期必拿空值，保存期拒绝）
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
		if err := validateStepTargets(step); err != nil {
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
		refs := map[string]bool{}
		collectStepRefs(step.Parameters, refs)
		for ref := range refs {
			if !seen[ref] {
				return fmt.Errorf("steps[%d]: unknown or forward step reference %q", i, ref)
			}
		}
		seen[name] = true
	}
	return nil
}

// stepOptionsFromParams 提取步骤可选参数（timeout_s/continue_on_error/retry/name）
// workflowMaxTargets 扇出目标数上限（M2b F1）：个人 infra 规模 sanity 闸，
// 与 WorkflowMaxSteps 同量级的防误用边界
const workflowMaxTargets = 20

// validateStepTargets 扇出目标数组校验（M2b F1）：与 target 互斥、条目非空、
// 去重、≤20 台。单目标步（无 targets）恒过。
func validateStepTargets(step *jobCreatePayload) error {
	if len(step.Targets) == 0 {
		return nil
	}
	if strings.TrimSpace(step.Target) != "" {
		return fmt.Errorf("target and targets are mutually exclusive")
	}
	if len(step.Targets) > workflowMaxTargets {
		return fmt.Errorf("too many targets (max %d)", workflowMaxTargets)
	}
	seen := make(map[string]bool, len(step.Targets))
	for i := range step.Targets {
		step.Targets[i] = strings.TrimSpace(step.Targets[i])
		if step.Targets[i] == "" {
			return fmt.Errorf("targets[%d] is empty", i)
		}
		if seen[step.Targets[i]] {
			return fmt.Errorf("duplicate target %q", step.Targets[i])
		}
		seen[step.Targets[i]] = true
	}
	return nil
}

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
	// 步骤目标 agent 全部在线才建 run（与单条 Job 同口径：不建幽灵；扇出步
	// 逐台校验，M2b F8）
	stepPayloads := parseWorkflowSteps(wf)
	for i := range stepPayloads {
		step := &stepPayloads[i]
		for _, tgt := range fanoutTargetsOf(step) {
			if _, ok := s.registry.Get(tgt); !ok {
				s.handleError(w, r, http.StatusServiceUnavailable,
					fmt.Sprintf("agent offline for steps[%d]: %s", i, tgt))
				return
			}
		}
	}

	username := actorName(r)
	var steps []workflowStepRun
	for _, step := range stepPayloads {
		opts, name := stepOptionsFromParams(&step)
		sr := workflowStepRun{
			Name:            name,
			Type:            step.Type,
			Target:          step.Target,
			Status:          storage.JobStatusPending,
			ContinueOnError: opts.ContinueOnError,
		}
		if len(step.Targets) > 0 {
			// 扇出步：步级 Target 留空（聚合在 Targets），逐台 pending 行
			sr.Targets = make([]workflowStepTargetRun, 0, len(step.Targets))
			for _, tgt := range step.Targets {
				sr.Targets = append(sr.Targets, workflowStepTargetRun{AgentID: tgt, Status: storage.JobStatusPending})
			}
		}
		steps = append(steps, sr)
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

// stepOutputOf 取步骤最终 attempt 的输出（M2a V4）；Job 行未建成
// （存储错误）视为空输出，与失败步同语义
func stepOutputOf(job *storage.Job) string {
	if job == nil {
		return ""
	}
	return job.Output
}

// advanceWorkflow 编排器：逐步建 Job → 派发 → 按重试/继续语义推进（W4/W5）。
// 每个 attempt 都是真实 Job（台账可查）；重试固定间隔 5s。
func (s *Server) advanceWorkflow(wf *storage.Workflow, run *storage.WorkflowRun, actor string) {
	steps := parseWorkflowSteps(wf)
	outputs := make(map[string]string, len(steps)) // 步骤 name → 最终 attempt 输出（M2a V3/V4）
	for i := range steps {
		step := &steps[i]
		opts, name := stepOptionsFromParams(step)

		// 取消检查：cancelWorkflowRun 已把 run.Steps 停在第 i 步
		current, err := s.db.GetWorkflowRun(run.ID)
		if err != nil {
			log.Printf("workflow: load run %s failed: %v", run.ID, err)
			return
		}
		if current.Status != storage.WorkflowRunStatusRunning {
			return // 已被取消，不再推进
		}

		var job *storage.Job
		var attempts int
		var finalStatus, reason string
		if len(step.Targets) > 0 {
			// 扇出步（M2b F2-F5）：逐台跑、快照整步回写；不提供步骤输出
			// （F7，{{steps.NAME.output}} 解析空串）
			var results []workflowStepTargetRun
			finalStatus, reason, results = s.runWorkflowStepFanout(step, opts, run.ID, actor, outputs)
			s.recordStepTargets(run, i, results, finalStatus, reason)
			outputs[name] = ""
			if finalStatus == storage.JobStatusCancelled {
				return // run 已被取消，终态由 cancel 流程落
			}
		} else {
			job, finalStatus, attempts, reason = s.runWorkflowStep(step, opts, run.ID, actor, outputs)
			outputs[name] = stepOutputOf(job)
			s.recordStepResult(run, i, job, finalStatus, attempts, reason)
		}

		if finalStatus != storage.JobStatusSuccess && !opts.ContinueOnError {
			if reason == "" {
				reason = fmt.Sprintf("step %d (%s) failed", i, step.Type)
			}
			s.finishWorkflowRun(run, storage.WorkflowRunStatusFailed, i, reason, actor)
			return
		}
	}
	s.finishWorkflowRun(run, storage.WorkflowRunStatusSuccess, len(steps)-1, "", actor)
}

// fanoutTargetsOf 步骤目标列表（M2b）：targets 扇出形态原样返回，单目标
// 形态包成单元素——定义校验后二者必居其一
func fanoutTargetsOf(step *jobCreatePayload) []string {
	if len(step.Targets) > 0 {
		return step.Targets
	}
	return []string{step.Target}
}

// runWorkflowStep 执行单步（含重试）；返回终态 Job、归纳状态、实跑尝试数
// 与失败原因（"" = 无补充说明，走默认 "failed"）。每次尝试建独立 Job 行
// （尝试历史进台账）。进入重试循环前先做变量解析与复检（M2a V3/V5）：
// 注入可能把命令撑过上限，复检不过则不派发、该步直接 failed。
func (s *Server) runWorkflowStep(step *jobCreatePayload, opts stepOptions, runID, actor string, outputs map[string]string) (*storage.Job, string, int, string) {
	params := resolveStepParams(sanitizeStepParams(step.Parameters), outputs)
	if err := validateJobPayload(&jobCreatePayload{Type: step.Type, Target: step.Target, Parameters: params}); err != nil {
		return nil, storage.JobStatusFailed, 0, fmt.Sprintf("step rejected after variable resolution: %v", err)
	}
	maxAttempts := opts.Retry + 1
	var job *storage.Job
	status := storage.JobStatusFailed
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(workflowRetryInterval)
		}
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

		// 下发解析后的参数（与落库一致）。M1 传定义原参数：meta 键漏到
		// agent 且模板不解析——M2a 变量传递下 agent 会执行 {{...}} 原文，
		// 属必须修的遗留缺陷
		resolved := *step
		resolved.Parameters = params
		success, jobErr := s.dispatchJob(job, resolved)

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
			return job, storage.JobStatusSuccess, attempt + 1, ""
		}
		status = storage.JobStatusFailed
	}
	return job, status, maxAttempts, ""
}

// resolveStepParams 递归替换参数树 string 值里的步骤输出引用（M2a V1/V4）：
// {{steps.NAME.output}} → outputs[NAME]。缺名取空串——定义校验（V2）已拦
// 未知/前向引用，运行期缺名唯一可达来源是被引步骤 Job 行未建成（存储
// 错误），输出为空与失败步同语义。其余 {{...}} 形态原样保留（V6）。
func resolveStepParams(params map[string]interface{}, outputs map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(params))
	for k, v := range params {
		out[k] = resolveStepValue(v, outputs)
	}
	return out
}

func resolveStepValue(v interface{}, outputs map[string]string) interface{} {
	switch t := v.(type) {
	case string:
		return stepOutputRefRe.ReplaceAllStringFunc(t, func(tok string) string {
			return outputs[stepOutputRefRe.FindStringSubmatch(tok)[1]]
		})
	case map[string]interface{}:
		m := make(map[string]interface{}, len(t))
		for k, vv := range t {
			m[k] = resolveStepValue(vv, outputs)
		}
		return m
	case []interface{}:
		s := make([]interface{}, len(t))
		for i, vv := range t {
			s[i] = resolveStepValue(vv, outputs)
		}
		return s
	default:
		return v
	}
}

// recordStepResult 回写步骤快照（状态/attempts/jobId/stopReason）
func (s *Server) recordStepResult(run *storage.WorkflowRun, idx int, job *storage.Job, status string, attempts int, stopReason string) {
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
			if stopReason == "" {
				stopReason = "failed"
			}
			steps[idx].StopReason = stopReason
		}
		if b, err := json.Marshal(steps); err == nil {
			run.Steps = string(b)
			_ = s.db.UpdateWorkflowRun(run)
		}
	}
}

// runWorkflowStepFanout 顺序扇出（M2b F2-F5）：逐台完整重试周期（每台复用
// runWorkflowStep，仅覆写 Target——agent 零改动）；目标边界查取消（F5）；
// 失败策略同 W5：无 continue_on_error 首台失败即停，带则跑完全部。返回聚合
// 状态（全成功才 success，任一失败 failed，取消 cancelled）、失败原因与逐台
// 结果（未跑目标记 cancelled/pending）。
func (s *Server) runWorkflowStepFanout(step *jobCreatePayload, opts stepOptions, runID, actor string, outputs map[string]string) (string, string, []workflowStepTargetRun) {
	status := storage.JobStatusSuccess
	reason := ""
	results := make([]workflowStepTargetRun, 0, len(step.Targets))
	for t := range step.Targets {
		// 目标边界取消检查（F5，与步骤边界同机制）
		current, err := s.db.GetWorkflowRun(runID)
		if err != nil {
			log.Printf("workflow: load run %s failed: %v", runID, err)
			break // 同 advanceWorkflow 加载失败口径：不再推进
		}
		if current.Status != storage.WorkflowRunStatusRunning {
			for ; t < len(step.Targets); t++ {
				results = append(results, workflowStepTargetRun{
					AgentID:    step.Targets[t],
					Status:     storage.JobStatusCancelled,
					StopReason: "run cancelled",
				})
			}
			return storage.JobStatusCancelled, "", results
		}
		single := *step
		single.Target = step.Targets[t]
		job, finalStatus, attempts, r := s.runWorkflowStep(&single, opts, runID, actor, outputs)
		if finalStatus != storage.JobStatusSuccess && r == "" {
			r = "failed" // 与 recordStepResult 同款默认，单台失败可读
		}
		results = append(results, workflowStepTargetRun{AgentID: step.Targets[t], Status: finalStatus, JobID: jobIDOf(job), Attempts: attempts, StopReason: r})
		if finalStatus != storage.JobStatusSuccess {
			status = storage.JobStatusFailed
			if reason == "" {
				reason = r
			}
			if !opts.ContinueOnError {
				break // F3：后续目标不跑（快照保持 pending 行）
			}
		}
	}
	return status, reason, results
}

// jobIDOf 取终态 Job 的 ID（Job 行未建成返回空）
func jobIDOf(job *storage.Job) string {
	if job == nil {
		return ""
	}
	return job.ID
}

// recordStepTargets 扇出步快照整步回写（M2b F6）：按索引合并逐台结果
// （未跑目标保留 init 的 pending 行），步级 Status=聚合 + StopReason
func (s *Server) recordStepTargets(run *storage.WorkflowRun, idx int, results []workflowStepTargetRun, status, stopReason string) {
	run, err := s.db.GetWorkflowRun(run.ID)
	if err != nil {
		log.Printf("workflow: load run for step targets failed: %v", err)
		return
	}
	var steps []workflowStepRun
	if json.Unmarshal([]byte(run.Steps), &steps) == nil && idx < len(steps) {
		steps[idx].Status = status
		for t := range results {
			if t < len(steps[idx].Targets) {
				steps[idx].Targets[t] = results[t]
			}
		}
		if status == storage.JobStatusFailed {
			// cancelled 不落步级 StopReason（未跑原因在逐台条目里）；failed 的
			// reason 由扇出 runner 聚合时保证非空（单台失败默认 "failed"）
			steps[idx].StopReason = stopReason
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
	// 审计写先于终态 UPDATE：UPDATE 必须是编排 goroutine 的最后一个 DB 写。
	// 原顺序（UPDATE 后审计）下，测试轮询观察到终态时 goroutine 还有一次
	// 不可观测的审计插入，TempDir 清理与之并发即「directory not empty」
	// （CI 实锤 TestWorkflowFanoutOutputEmpty；TestCovTerminalAgentClose 同类）。
	// 观察终态 ⟹ 无后续写，对成功/失败停/取消各路径确定成立。
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
	run.Status = status
	finished := time.Now()
	run.FinishedAt = &finished
	if err := s.db.UpdateWorkflowRun(run); err != nil {
		log.Printf("workflow: update run final state failed: %v", err)
		return
	}
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
