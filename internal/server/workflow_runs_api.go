// Workflow run 台账/详情/取消 HTTP 面（自 api_workflow.go 按职责拆分）：
// 台账与详情读取、取消（终态条件写 + 在途 step Job 联动击杀）。
package server

import (
	"log"
	"net/http"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/storage"
)

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

// handleWorkflowRunCancel 取消 run（W3 停推进 + M2c C8 击杀在途）：置
// cancelled——编排器在下一步推进前检查该状态即停；在途 step Job 经
// job.cancel 联动击杀（失控命令不再跑满超时），被击杀 Job 的回写命中
// runWorkflowStep 的 C7 取消守卫，步骤快照记 cancelled + stopReason
// "run cancelled"（与 F5 未跑目标同口径）。
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
	username := actorName(r)
	// 审计写先于终态 UPDATE（写序不变式同 finishWorkflowRun：观察终态 ⟹
	// 无 run 行后续写——本 handler 不再碰 run 行）
	s.audit.LogResource(username, audit.ActionWorkflowCancel, audit.ResourceWorkflow, id,
		map[string]interface{}{"workflow": run.WorkflowName},
		s.getClientIP(r), r.UserAgent())
	run.Status = storage.WorkflowRunStatusCancelled
	finished := time.Now()
	run.FinishedAt = &finished
	// 条件终态写（M2c，同 finishWorkflowRun 口径）：仅当仍 running 生效——
	// 取消与步骤自然终态竞态下先落者胜，后到者如实 409
	hit, err := s.db.FinishWorkflowRun(id, storage.WorkflowRunStatusCancelled, finished)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to cancel run")
		return
	}
	if hit == 0 {
		s.handleError(w, r, http.StatusConflict, "run is not running")
		return
	}
	// C8：联动击杀在途 step Job（最佳努力——个别目标不可达仅记日志，
	// run 已落终态不回滚）
	s.cancelRunJobs(id)
	s.writeJSON(w, http.StatusOK, toWorkflowRunView(run))
}

// cancelRunJobs 击杀 run 的在途 step Job（M2c C8）：running 逐个 job.cancel +
// 条件 UPDATE 落 cancelled；pending（建行未派发的微秒窗口）DB-only 条件取消，
// 防取消后台账留无终态的孤儿 pending 行。不另发 cancel 审计（其 exec 回写
// 已有 job_run 审计）。
func (s *Server) cancelRunJobs(runID string) {
	s.cancelRunJobsStatus(runID, storage.JobStatusRunning)
	s.cancelRunJobsStatus(runID, storage.JobStatusPending)
}

// cancelRunJobsStatus 击杀 run 的指定在途态 step Job：running 走 job.cancel +
// 条件 UPDATE（C8 主体）；pending 直接条件取消即可，0 行命中说明恰好翻转为
// running（取消与 dispatch 竞态）——落到 running 击杀路径兜底（C6 同款
// fallthrough，agent 侧未知 ID 幂等，最坏 cancel 先于 exec 到达被无操作）。
func (s *Server) cancelRunJobsStatus(runID, status string) {
	jobs, err := s.db.ListJobsFiltered(status, "", "", runID, 50)
	if err != nil {
		log.Printf("workflow: list %s step jobs for cancel failed: %v", status, err)
		return
	}
	for i := range jobs {
		job := &jobs[i]
		if status == storage.JobStatusPending {
			hit, err := s.db.CancelJob(job.ID, storage.JobStatusPending, time.Now())
			if err != nil {
				log.Printf("workflow: mark step job %s cancelled failed: %v", job.ID, err)
				continue
			}
			if hit == 1 {
				continue
			}
			// 已翻转 running → RPC 击杀兜底
		}
		if _, err := s.CallAgent(job.Target, jobCancelMethod,
			map[string]interface{}{"id": job.ID}); err != nil {
			log.Printf("workflow: kill step job %s failed: %v", job.ID, err)
			continue
		}
		if _, err := s.db.CancelJob(job.ID, storage.JobStatusRunning, time.Now()); err != nil {
			log.Printf("workflow: mark step job %s cancelled failed: %v", job.ID, err)
		}
	}
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
