package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
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

// workflowStepStatusSkipped 跳过步快照状态（M2e G4）：步级词表新态——
// 如实陈述「没跑」，与 pending（还没轮到）区分；JobStatus 词表不加词
const workflowStepStatusSkipped = "skipped"

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
// workflow 内唯一；输出引用与 depends_on 都必须指向更靠前的步骤（M2a V2/
// M2e G2——顺序链语义下前向/自引用运行期必拿空值或无意义，保存期拒绝）
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
		if err := validateStepDependsOn(step, seen); err != nil {
			return fmt.Errorf("steps[%d]: %w", i, err)
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

// validateStepDependsOn 依赖声明校验（M2e G1/G2）：条目 trim 后非空、列表内
// 去重、只能指向更靠前已声明的 name（seen 随循环累积，自引用天然不在）——
// 数组序即拓扑序，编排器循环顺序零改动；前向/未知引用与 M2a V2 输出引用
// 同措辞拒绝，环在构造上不可能，无需运行期环检测。无 depends_on 恒过。
func validateStepDependsOn(step *jobCreatePayload, seen map[string]bool) error {
	if len(step.DependsOn) == 0 {
		return nil
	}
	deps := make(map[string]bool, len(step.DependsOn))
	for i := range step.DependsOn {
		step.DependsOn[i] = strings.TrimSpace(step.DependsOn[i])
		if step.DependsOn[i] == "" {
			return fmt.Errorf("depends_on[%d] is empty", i)
		}
		if deps[step.DependsOn[i]] {
			return fmt.Errorf("duplicate dependency %q", step.DependsOn[i])
		}
		if !seen[step.DependsOn[i]] {
			return fmt.Errorf("unknown or forward step reference %q", step.DependsOn[i])
		}
		deps[step.DependsOn[i]] = true
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
