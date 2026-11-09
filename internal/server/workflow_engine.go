// Workflow 编排引擎（自 api_workflow.go 按职责拆分）：run 创建、后台推进
// 状态机、单步/扇出执行、变量替换（{{steps.NAME.output}}）与步骤快照回写。
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

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
		// 并发 POST /run 的 check-then-act 竞态由存储层原子条件插入兜底
		// （S4）：落入冲突分支返回 409，不再双跑。
		if errors.Is(err, storage.ErrWorkflowRunConflict) {
			s.handleError(w, r, http.StatusConflict, "workflow is already running")
			return
		}
		s.handleError(w, r, http.StatusInternalServerError, "failed to create run")
		return
	}

	s.goBg(func() { s.advanceWorkflow(wf, run, username) })

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
	outputs := make(map[string]string, len(steps))  // 步骤 name → 最终 attempt 输出（M2a V3/V4）
	outcomes := make(map[string]string, len(steps)) // 步骤 name → success/failed/skipped（M2e G4 依赖门控）
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

		// 依赖门控（M2e G4/G5）：任一依赖终态非 success 即跳过本步（不建
		// Job 行）；skipped 依赖同样传递。取消优先于跳过（上面已查）。
		if dep, state, gated := unmetDependency(step, outcomes); gated {
			reason := fmt.Sprintf("skipped: dependency %s %s", dep, state)
			s.recordStepSkipped(run, i, step, reason)
			outcomes[name] = workflowStepStatusSkipped
			outputs[name] = "" // M2a V4：{{steps.NAME.output}} 解析空串
			continue
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
			if finalStatus == storage.JobStatusCancelled {
				// C9：步骤 Job 被取消（run cancel 联动击杀，或单 Job 取消）。
				// 扇出分支同款 return；run 仍 running（单 Job 取消场景）时
				// 落 cancelled 终态——run 不得僵尸悬挂
				current, err := s.db.GetWorkflowRun(run.ID)
				if err != nil {
					log.Printf("workflow: load run %s for cancelled step failed: %v", run.ID, err)
					return
				}
				if current.Status == storage.WorkflowRunStatusRunning {
					if reason == "" {
						reason = fmt.Sprintf("step %d (%s) cancelled", i, step.Type)
					}
					s.finishWorkflowRun(run, storage.WorkflowRunStatusCancelled, i, reason, actor)
				}
				return
			}
		}
		outcomes[name] = finalStatus // M2e G4：依赖门控取这里的状态

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

// unmetDependency 依赖门控判定（M2e G4）：返回第一个终态非 success 的依赖
// 及其状态。G2 校验保证依赖都在 outcomes 里有终态（前序步必已执行或跳过），
// 缺项视为未满足（防御：不应发生）。
func unmetDependency(step *jobCreatePayload, outcomes map[string]string) (string, string, bool) {
	for _, dep := range step.DependsOn {
		if state := outcomes[dep]; state != storage.JobStatusSuccess {
			return dep, state, true
		}
	}
	return "", "", false
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

		// C7 取消守卫：回写前重读——取消写（run cancel 联动击杀 / 单 Job
		// 取消）可能已落 cancelled，exec 的 killed 结果不得覆盖终态
		finalStatus := storage.JobStatusFailed
		if success {
			finalStatus = storage.JobStatusSuccess
		}
		if fresh, err := s.db.GetJob(job.ID); err == nil && fresh.Status == storage.JobStatusCancelled {
			finalStatus = storage.JobStatusCancelled
			job.Status = storage.JobStatusCancelled
			if err := s.db.MarkJobCancelledOutput(job.ID, job.Output, job.Error, job.ExitCode); err != nil {
				log.Printf("workflow: write cancelled step output failed: %v", err)
			}
		} else {
			job.Status = finalStatus
			_ = s.db.UpdateJob(job)
		}

		s.audit.LogResource(actor, audit.ActionJobRun, audit.ResourceJob, job.ID,
			map[string]interface{}{
				"type": job.Type, "target": job.Target, "status": job.Status,
				"params": params, "workflow_run_id": runID,
			}, "", "")

		if finalStatus == storage.JobStatusCancelled {
			// C9：Job 被取消则重试无意义且 run 终将 cancelled——立即返回，
			// 不再进入下一 attempt
			return job, finalStatus, attempt + 1, "run cancelled"
		}
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

// recordStepSkipped 跳过步快照回写（M2e G4）：无 Job 行、attempts 0，
// status=skipped + 停因。db 失败面与 recordStepResult 同口径静默降级
// （编排不因台账回写失败而中断）。
func (s *Server) recordStepSkipped(run *storage.WorkflowRun, idx int, step *jobCreatePayload, stopReason string) {
	current, err := s.db.GetWorkflowRun(run.ID)
	if err != nil {
		log.Printf("workflow: load run for step skip failed: %v", err)
		return
	}
	var steps []workflowStepRun
	if json.Unmarshal([]byte(current.Steps), &steps) == nil && idx < len(steps) {
		steps[idx].Status = workflowStepStatusSkipped
		steps[idx].StopReason = stopReason
		if b, err := json.Marshal(steps); err == nil {
			current.Steps = string(b)
			_ = s.db.UpdateWorkflowRun(current)
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
			// M2c C9 同款：目标被取消（run cancel 联动击杀在途步）整步聚合
			// cancelled，不按 failed 谎报
			if finalStatus == storage.JobStatusCancelled {
				status = storage.JobStatusCancelled
			} else {
				status = storage.JobStatusFailed
			}
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
	// 终态条件写（M2c，同 Job CancelJob 口径）：仅当仍 running 生效。取消
	// 与步骤自然终态竞态可达（击杀输给 exec 收尾，步骤以 failed 收场），
	// 先落终态者胜——cancelled 不被翻成 failed/success
	hit, err := s.db.FinishWorkflowRun(run.ID, status, finished)
	if err != nil {
		log.Printf("workflow: update run final state failed: %v", err)
		return
	}
	if hit == 0 {
		log.Printf("workflow: run %s already terminal, skip finish as %s", run.ID, status)
	}
}
