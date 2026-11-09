// Workflow 验收视图/轮询层（自 main.go 按职责拆分）：Job/Workflow run/审计
// 的视图类型、解析、轮询与步骤构造（jobView/stepTargetRun/stepRun/runView/
// auditEntry + parse*/is*Terminal/poll*/await*/getRun/jobsByRun/step/stepFan/
// wfCreate/wfRun/wfRunsCount/countAuditByAction）。
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ---------- Job 视图与轮询 ----------

type jobView struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Target     string                 `json:"target"`
	Actor      string                 `json:"actor"`
	Status     string                 `json:"status"`
	Parameters map[string]interface{} `json:"parameters"`
	Output     string                 `json:"output"`
	ExitCode   int                    `json:"exitCode"`
	Error      string                 `json:"error"`
	CreatedAt  *string                `json:"createdAt"`
	StartedAt  *string                `json:"startedAt"`
	FinishedAt *string                `json:"finishedAt"`
}

func parseJobsList(raw []byte) ([]jobView, error) {
	var r struct {
		Jobs []jobView `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("bad jobs list: %v", err)
	}
	return r.Jobs, nil
}

func parseJob(raw []byte) (*jobView, error) {
	var v jobView
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("bad job view: %v", err)
	}
	return &v, nil
}

func isJobTerminal(status string) bool {
	return status == "success" || status == "failed" || status == "cancelled"
}

// pollJob 轮询单条 Job 直至条件成立；W1 异步化后创建响应恒 pending，
// 终态与在途态都经台账轮询取得
func pollJob(id string, cond func(*jobView) bool, timeout time.Duration) (*jobView, error) {
	deadline := time.Now().Add(timeout)
	var last *jobView
	for {
		code, raw := reqJSON(http.MethodGet, *apiBase+"/api/jobs/"+id, nil)
		if code == 200 {
			if v, err := parseJob(raw); err == nil {
				last = v
				if cond(v) {
					return v, nil
				}
			}
		}
		if time.Now().After(deadline) {
			if last != nil {
				return last, fmt.Errorf("job %s 未在 %s 内满足条件（最后 status=%s）", id, timeout, last.Status)
			}
			return nil, fmt.Errorf("job %s 未在 %s 内满足条件", id, timeout)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// awaitJob 轮询到终态
func awaitJob(id string, timeout time.Duration) (*jobView, error) {
	return pollJob(id, func(v *jobView) bool { return isJobTerminal(v.Status) }, timeout)
}

// jobsByRun run 关联 Job 台账（步骤即 Job：编排器每步建真实 Job 行）
func jobsByRun(runID string) ([]jobView, error) {
	code, raw := reqJSON(http.MethodGet,
		*apiBase+"/api/jobs?workflow_run_id="+runID, nil)
	if code != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", code, truncate(string(raw), 200))
	}
	return parseJobsList(raw)
}

// pollJobsByRun 轮询 run 关联 Job 台账直至条件成立（run 步骤快照只在步骤
// 终态回写，在途态经 Job 台账观察）
func pollJobsByRun(runID string, cond func([]jobView) bool, timeout time.Duration) ([]jobView, error) {
	deadline := time.Now().Add(timeout)
	var last []jobView
	for {
		jobs, err := jobsByRun(runID)
		if err == nil {
			last = jobs
			if cond(jobs) {
				return jobs, nil
			}
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("run %s 的 Job 台账未在 %s 内满足条件", runID, timeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// ---------- Workflow 视图与轮询 ----------

// stepTargetRun 扇出步逐台结果（M2b F6）：Status 同 JobStatus 词表
type stepTargetRun struct {
	AgentID    string `json:"agentId"`
	Status     string `json:"status"`
	JobID      string `json:"jobId"`
	Attempts   int    `json:"attempts"`
	StopReason string `json:"stopReason"`
}

type stepRun struct {
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Target     string          `json:"target"`
	Targets    []stepTargetRun `json:"targets"` // 扇出步逐台（单目标步恒空）
	Status     string          `json:"status"`
	JobID      string          `json:"jobId"`
	Attempts   int             `json:"attempts"`
	StopReason string          `json:"stopReason"`
}

type runView struct {
	ID           string    `json:"id"`
	WorkflowID   string    `json:"workflowId"`
	WorkflowName string    `json:"workflowName"`
	Status       string    `json:"status"`
	Actor        string    `json:"actor"`
	Steps        []stepRun `json:"steps"`
	CreatedAt    *string   `json:"createdAt"`
	FinishedAt   *string   `json:"finishedAt"`
}

func parseRun(raw []byte) (*runView, error) {
	var v runView
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("bad run view: %v", err)
	}
	return &v, nil
}

func isRunTerminal(status string) bool {
	return status == "success" || status == "failed" || status == "cancelled"
}

func getRun(runID string) (*runView, error) {
	code, raw := reqJSON(http.MethodGet, *apiBase+"/api/workflow-runs/"+runID, nil)
	if code != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", code, truncate(string(raw), 200))
	}
	return parseRun(raw)
}

// pollRun 轮询 run 直至条件成立
func pollRun(runID string, cond func(*runView) bool, timeout time.Duration) (*runView, error) {
	deadline := time.Now().Add(timeout)
	var last *runView
	for {
		rv, err := getRun(runID)
		if err == nil {
			last = rv
			if cond(rv) {
				return rv, nil
			}
		}
		if time.Now().After(deadline) {
			if last != nil {
				return last, fmt.Errorf("run %s 未在 %s 内满足条件（最后 status=%s）", runID, timeout, last.Status)
			}
			return nil, fmt.Errorf("run %s 未在 %s 内满足条件", runID, timeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// step 一步定义（type/target 恒 agent.exec/在线目标；extra 带 retry/
// timeout_s/continue_on_error，落 parameters——server 侧 stepOptionsFromParams
// 只认 parameters 内的编排元参数）
func step(name, target, command string, extra map[string]interface{}) map[string]interface{} {
	params := map[string]interface{}{"name": name, "command": command}
	for k, v := range extra {
		params[k] = v
	}
	return map[string]interface{}{
		"type": "agent.exec", "target": target, "parameters": params,
	}
}

// stepFan 扇出步定义（M2b：targets 多目标，与 target 互斥；其余同 step）
func stepFan(name string, targets []string, command string, extra map[string]interface{}) map[string]interface{} {
	params := map[string]interface{}{"name": name, "command": command}
	for k, v := range extra {
		params[k] = v
	}
	return map[string]interface{}{
		"type": "agent.exec", "targets": targets, "parameters": params,
	}
}

// wfCreate 建定义并返回 id（非 201 视为环境故障走 fatal——定义建不出来
// 后续场景全部失去基线）
func wfCreate(name string, steps []map[string]interface{}) string {
	code, raw := reqJSON(http.MethodPost, *apiBase+"/api/workflows", map[string]interface{}{
		"name": name, "steps": steps})
	if code != http.StatusCreated {
		fatal("建定义 %s 失败 HTTP %d: %s", name, code, truncate(string(raw), 200))
	}
	var v struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &v) != nil || v.ID == "" {
		fatal("建定义 %s 响应无 id: %s", name, truncate(string(raw), 200))
	}
	return v.ID
}

// wfRun 触发 run（非 201 fatal），返回 run 视图与原始响应（落证据）
func wfRun(wfID string) (*runView, []byte) {
	code, raw := reqJSON(http.MethodPost, *apiBase+"/api/workflows/"+wfID+"/run", nil)
	if code != http.StatusCreated {
		fatal("触发 run 失败 HTTP %d: %s", code, truncate(string(raw), 200))
	}
	rv, err := parseRun(raw)
	if err != nil || rv.ID == "" {
		fatal("run 响应无 id: %s", truncate(string(raw), 200))
	}
	return rv, raw
}

// wfRunsCount 该定义的 run 台账条数（W7 断言 503 不建 run）
func wfRunsCount(wfID string) (int, error) {
	code, raw := reqJSON(http.MethodGet, *apiBase+"/api/workflows/"+wfID+"/runs", nil)
	if code != 200 {
		return 0, fmt.Errorf("HTTP %d: %s", code, truncate(string(raw), 200))
	}
	var r struct {
		Runs []json.RawMessage `json:"runs"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return 0, fmt.Errorf("bad runs list: %v", err)
	}
	return len(r.Runs), nil
}

// ---------- 审计解析 ----------

type auditEntry struct {
	Username   string `json:"username"`
	Action     string `json:"action"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id"`
	Details    string `json:"details"`
}

func parseAuditEntries(raw []byte) ([]auditEntry, error) {
	var r struct {
		Data []auditEntry `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("bad audit list: %v", err)
	}
	return r.Data, nil
}

// countAuditByAction 按动作计数
func countAuditByAction(entries []auditEntry, action string) int {
	n := 0
	for _, e := range entries {
		if e.Action == action {
			n++
		}
	}
	return n
}
