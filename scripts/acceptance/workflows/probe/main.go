// Workflow 编排真机验收探针（workflow-design.md M1，acceptance-checklist
// 「Workflow 编排」节）。以面板用户身份走真实链路（REST + agent RPC +
// 审计/权限面）：
//
//	W0  前置：登录；目标 agent 在线；定义台账可读（基线计数）
//	W1  异步 Job：创建 201 恒 pending（W1 契约，fix 0d065a1 后确定性）→
//	    轮询 success/exit 0/输出非空；actor/参数往返；起止时间落定
//	W2  真机编排一轮：uptime → df -h | head -1 两步链 run success、步骤
//	    快照 success+jobId、attempts=1；run 关联 Job 台账 actor=workflow
//	W3  失败停：true / exit 3 / echo never → run failed；第三步保持
//	    pending 且无 Job（停推进）；run 台账恰 2 条 Job
//	W4  重试：marker 文件命令 retry=1 → attempts=2、两条 Job 行、间隔
//	    ≈5s（workflowRetryInterval，断言 [4s,10s] 带宽）
//	W5  取消：sleep 步骤在途取消 → 200 cancelled；在途步骤自然跑完不被
//	    打断、后续步骤不推进（保持 pending、无 Job）；再 cancel 409
//	W6  重入/冻结：active run 期间同定义 run 409 / PUT 409 / DELETE 409
//	W7  离线 ghost 目标 → 503 不建 run（与单条 Job 同口径，不落幽灵）；
//	    无 active run 后 DELETE 200
//	W8  权限与审计：viewer（workflows:read）读 200 写 403；自定义无权限
//	    角色全 403；审计 resource=workflow 的 create/run/cancel 齐
//	W9  Job cancel 语义（workflow-design W3）：running Job 取消 409
//	    （等自然结束，RPC 无取消帧）、ghost 404、GET /cancel 405
//
// 证据落 .acceptance/workflows/evidence/（probe.log + 场景原始响应）；
// FAIL → exit 1。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	apiBase   = flag.String("api", "http://127.0.0.1:20010", "cockpit server 基址")
	adminUser = flag.String("user", "admin", "管理员用户名")
	adminPass = flag.String("pass", "e2e-strong-pass-1", "管理员口令")
	evDir     = flag.String("ev", "", "证据目录（默认 .acceptance/workflows/evidence）")
	agentA1   = flag.String("a1", "workflows-acc-a1", "在线执行目标 agent")
	ghostID   = flag.String("ghost", "workflows-acc-ghost", "从未注册的 agent id（offline 样本）")
)

var (
	evFile = (*os.File)(nil)
	evMu   sync.Mutex
	passes int
	fails  int
	token  string
	httpC  = &http.Client{Timeout: 30 * time.Second}
)

// 行为中性注入点（先例：jobs/probe osExit）——fatal 退出在单测注入桩覆盖
// 分支，默认值即原行为
var osExit = os.Exit

// ---------- 证据 ----------

func ev(format string, args ...interface{}) {
	evMu.Lock()
	defer evMu.Unlock()
	line := fmt.Sprintf(format, args...)
	fmt.Println(line)
	if evFile != nil {
		fmt.Fprintf(evFile, "%s\n", line)
	}
}

func check(name string, ok bool, detail string) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		fails++
	} else {
		passes++
	}
	ev("[%s] %s — %s", status, name, detail)
}

func fatal(format string, args ...interface{}) {
	ev("[FATAL] "+format, args...)
	osExit(2)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// saveEV 场景原始响应落证据目录
func saveEV(name string, raw []byte) {
	if *evDir == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(*evDir, name), raw, 0o644)
}

// ---------- cockpit REST ----------

func reqJSON(method, url string, body interface{}) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
	req.Header.Set("Content-Type", "application/json")
	evMu.Lock()
	tk := token
	evMu.Unlock()
	if tk != "" {
		req.Header.Set("Authorization", "Bearer "+tk)
	}
	resp, err := httpC.Do(req)
	if err != nil {
		return -1, []byte(err.Error())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// loginAs 登录取 token（不改包级 token——W8 多用户场景用）
func loginAs(user, pass string) (string, error) {
	code, raw := reqJSON(http.MethodPost, *apiBase+"/api/auth/login",
		map[string]string{"username": user, "password": pass})
	if code != 200 {
		return "", fmt.Errorf("HTTP %d: %s", code, truncate(string(raw), 200))
	}
	var r struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Token == "" {
		return "", fmt.Errorf("响应无 token: %s", truncate(string(raw), 200))
	}
	return r.Token, nil
}

func login() {
	tk, err := loginAs(*adminUser, *adminPass)
	if err != nil {
		fatal("登录失败: %v", err)
	}
	evMu.Lock()
	token = tk
	evMu.Unlock()
}

// agentOnline /api/agents 为 DB 视图（裸数组与 {agents:[..]} 包装两形态
// 兼容），按 id 找 status=online
func agentOnline(id string) (bool, error) {
	_, raw := reqJSON(http.MethodGet, *apiBase+"/api/agents", nil)
	type agentEntry struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	match := func(list []agentEntry) (bool, error) {
		for _, a := range list {
			if a.ID == id {
				return a.Status == "online", nil
			}
		}
		return false, fmt.Errorf("agent %s 不在 /api/agents 列表", id)
	}
	var list []agentEntry
	if json.Unmarshal(raw, &list) == nil {
		return match(list)
	}
	var wrapper struct {
		Agents []agentEntry `json:"agents"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return false, fmt.Errorf("bad /api/agents: %v", err)
	}
	return match(wrapper.Agents)
}

// setToken 切换请求身份（场景内多用户切换的统一口子）
func setToken(tk string) {
	evMu.Lock()
	token = tk
	evMu.Unlock()
}

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

type stepRun struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Target     string `json:"target"`
	Status     string `json:"status"`
	JobID      string `json:"jobId"`
	Attempts   int    `json:"attempts"`
	StopReason string `json:"stopReason"`
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

// ---------- 场景 ----------

func main() {
	flag.Parse()
	if _, err := os.Stat("go.mod"); err != nil {
		fatal("请在仓库根目录运行（找不到 go.mod）")
	}
	repo, _ := os.Getwd()
	if *evDir == "" {
		*evDir = filepath.Join(repo, ".acceptance/workflows/evidence")
	}
	_ = os.MkdirAll(*evDir, 0o755)
	f, err := os.Create(filepath.Join(*evDir, "probe.log"))
	if err != nil {
		fatal("证据文件创建失败: %v", err)
	}
	evFile = f
	defer evFile.Close()

	ev("=== Workflow 编排真机验收 %s ===", time.Now().Format(time.RFC3339))
	ev("api=%s a1=%s ghost=%s", *apiBase, *agentA1, *ghostID)

	// ---- W0 前置 ----
	wfBaseline := 0
	func() {
		name := "W0 前置：登录 + 目标 agent 在线 + 定义台账可读"
		login()
		online, err := agentOnline(*agentA1)
		if err != nil {
			check(name, false, err.Error())
			fatal("目标 agent 不在线——先跑 run-server.sh")
		}
		code, raw := reqJSON(http.MethodGet, *apiBase+"/api/workflows", nil)
		if code != 200 {
			check(name, false, fmt.Sprintf("GET /api/workflows code=%d %s", code, truncate(string(raw), 120)))
			fatal("定义台账不可读")
		}
		var l struct {
			Workflows []json.RawMessage `json:"workflows"`
		}
		_ = json.Unmarshal(raw, &l)
		wfBaseline = len(l.Workflows)
		check(name, true, fmt.Sprintf("agent=%s online=%v 定义基线=%d", *agentA1, online, wfBaseline))
	}()

	// ---- W1 异步 Job 契约 ----
	func() {
		name := "W1 异步 Job：201 恒 pending → 轮询 success/exit 0/输出非空"
		cmd := "echo W1-OUTPUT-MARK && uptime"
		code, raw := reqJSON(http.MethodPost, *apiBase+"/api/jobs", map[string]interface{}{
			"type": "agent.exec", "target": *agentA1,
			"parameters": map[string]interface{}{"command": cmd, "timeout_s": 60},
		})
		saveEV("create-W1.json", raw)
		if code != http.StatusCreated {
			check(name, false, fmt.Sprintf("HTTP %d（want 201）: %s", code, truncate(string(raw), 200)))
			return
		}
		pending, err := parseJob(raw)
		if err != nil {
			check(name, false, err.Error())
			return
		}
		if pending.Status != "pending" || pending.Actor != *adminUser {
			check(name, false, fmt.Sprintf("创建契约: status=%q actor=%q（want pending/%s）",
				pending.Status, pending.Actor, *adminUser))
			return
		}
		v, err := awaitJob(pending.ID, 30*time.Second)
		if err != nil {
			check(name, false, "轮询终态: "+err.Error())
			return
		}
		saveEV("final-W1.json", mustJSON(v))
		cmdBack, _ := v.Parameters["command"].(string)
		check(name, v.Status == "success" && v.ExitCode == 0 &&
			strings.Contains(v.Output, "W1-OUTPUT-MARK") &&
			v.Actor == *adminUser && cmdBack == cmd &&
			v.StartedAt != nil && v.FinishedAt != nil,
			fmt.Sprintf("pending 契约+终态 success/exit0/输出标记/params 往返/起止落定 id=%s",
				truncate(v.ID, 12)))
	}()

	// ---- W2 真机编排一轮 ----
	w2WfID, w2RunID := "", ""
	func() {
		name := "W2 真机编排一轮：两步链 run success/步骤快照/Job 台账 actor=workflow"
		w2WfID = wfCreate("wf-acc-chain", []map[string]interface{}{
			step("s1-uptime", *agentA1, "uptime", nil),
			step("s2-disk", *agentA1, "df -h | head -1", nil),
		})
		run, raw := wfRun(w2WfID)
		w2RunID = run.ID
		saveEV("run-W2.json", raw)
		final, err := pollRun(run.ID, func(rv *runView) bool { return isRunTerminal(rv.Status) }, 60*time.Second)
		if err != nil {
			check(name, false, "轮询 run: "+err.Error())
			return
		}
		saveEV("final-W2.json", mustJSON(final))
		stepsOK := len(final.Steps) == 2 &&
			final.Steps[0].Status == "success" && final.Steps[0].JobID != "" && final.Steps[0].Attempts == 1 &&
			final.Steps[1].Status == "success" && final.Steps[1].JobID != "" && final.Steps[1].Attempts == 1
		headOK := final.Status == "success" && final.Actor == *adminUser && final.FinishedAt != nil
		jobs, jerr := jobsByRun(run.ID)
		jobsOK := jerr == nil && len(jobs) == 2
		for _, j := range jobs {
			if j.Actor != "workflow" || j.Status != "success" {
				jobsOK = false
			}
		}
		saveEV("jobs-W2.json", mustJSON(jobs))
		check(name, stepsOK && headOK && jobsOK,
			fmt.Sprintf("run=%s status=%s 步骤快照=%v 台账=%d 条 actor=workflow=%v",
				truncate(run.ID, 12), final.Status, stepsOK, len(jobs), jobsOK))
	}()

	// ---- W3 失败停 ----
	func() {
		name := "W3 失败停：exit 3 → run failed/第三步 pending 无 Job"
		wfID := wfCreate("wf-acc-failstop", []map[string]interface{}{
			step("s1-true", *agentA1, "true", nil),
			step("s2-fail", *agentA1, "exit 3", nil),
			step("s3-never", *agentA1, "echo never-reached", nil),
		})
		run, raw := wfRun(wfID)
		saveEV("run-W3.json", raw)
		final, err := pollRun(run.ID, func(rv *runView) bool { return isRunTerminal(rv.Status) }, 60*time.Second)
		if err != nil {
			check(name, false, "轮询 run: "+err.Error())
			return
		}
		saveEV("final-W3.json", mustJSON(final))
		snapshotOK := len(final.Steps) == 3 &&
			final.Steps[0].Status == "success" &&
			final.Steps[1].Status == "failed" && final.Steps[1].Attempts == 1 &&
			final.Steps[2].Status == "pending" && final.Steps[2].JobID == ""
		jobs, jerr := jobsByRun(run.ID)
		jobsOK := jerr == nil && len(jobs) == 2 // 第三步从未建 Job（停推进）
		check(name, final.Status == "failed" && snapshotOK && jobsOK,
			fmt.Sprintf("status=%s 快照[ok/%s/pending=%v] 台账=%d 条（第三步无 Job）",
				final.Status, final.Steps[1].Status, snapshotOK, len(jobs)))
	}()

	// ---- W4 重试 ----
	func() {
		name := "W4 重试 retry=1 → attempts=2/两条 Job 行/间隔≈5s"
		marker := fmt.Sprintf("/tmp/wf-acc-retry-%d", time.Now().UnixNano())
		cmd := fmt.Sprintf("if [ -f %s ]; then echo retry-pass; else touch %s; exit 1; fi", marker, marker)
		wfID := wfCreate("wf-acc-retry", []map[string]interface{}{
			step("s1-retry", *agentA1, cmd, map[string]interface{}{"retry": 1}),
		})
		run, raw := wfRun(wfID)
		saveEV("run-W4.json", raw)
		final, err := pollRun(run.ID, func(rv *runView) bool { return isRunTerminal(rv.Status) }, 90*time.Second)
		if err != nil {
			check(name, false, "轮询 run: "+err.Error())
			return
		}
		saveEV("final-W4.json", mustJSON(final))
		jobs, jerr := jobsByRun(run.ID)
		saveEV("jobs-W4.json", mustJSON(jobs))
		if jerr != nil || len(jobs) != 2 || len(final.Steps) != 1 {
			check(name, false, fmt.Sprintf("台账 %d 条（want 2）/步骤数 %d err=%v", len(jobs), len(final.Steps), jerr))
			return
		}
		// 台账倒序：jobs[0]=第 2 次尝试（success），jobs[1]=第 1 次（failed exit 1）
		attempt2, attempt1 := jobs[0], jobs[1]
		gapOK := false
		if attempt1.CreatedAt != nil && attempt2.CreatedAt != nil {
			t1, e1 := time.Parse(time.RFC3339, *attempt1.CreatedAt)
			t2, e2 := time.Parse(time.RFC3339, *attempt2.CreatedAt)
			if e1 == nil && e2 == nil {
				gap := t2.Sub(t1)
				gapOK = gap >= 4*time.Second && gap <= 10*time.Second
			}
		}
		check(name, final.Status == "success" && final.Steps[0].Attempts == 2 &&
			attempt2.Status == "success" && strings.Contains(attempt2.Output, "retry-pass") &&
			attempt1.Status == "failed" && attempt1.ExitCode == 1 && gapOK,
			fmt.Sprintf("attempts=%d 尝试2=%s(输出标记=%v) 尝试1=%s(exit=%d) 间隔∈[4s,10s]=%v",
				final.Steps[0].Attempts, attempt2.Status,
				strings.Contains(attempt2.Output, "retry-pass"), attempt1.Status, attempt1.ExitCode, gapOK))
	}()

	// ---- W5+W6 取消与重入/冻结 ----
	func() {
		name := "W5+W6 取消与重入/冻结：active run 409 面 + 在途取消不推进"
		steps := []map[string]interface{}{
			step("s1-sleep", *agentA1, "sleep 12", nil),
			step("s2-after", *agentA1, "echo after-cancel", nil),
		}
		wfID := wfCreate("wf-acc-cancel", steps)
		run, raw := wfRun(wfID)
		saveEV("run-W5.json", raw)

		// W6 重入/冻结：run 在途期间同定义 run/PUT/DELETE 全 409
		cRun, _ := reqJSON(http.MethodPost, *apiBase+"/api/workflows/"+wfID+"/run", nil)
		cPut, _ := reqJSON(http.MethodPut, *apiBase+"/api/workflows/"+wfID,
			map[string]interface{}{"name": "wf-acc-cancel", "steps": steps})
		cDel, _ := reqJSON(http.MethodDelete, *apiBase+"/api/workflows/"+wfID, nil)
		freezeOK := cRun == http.StatusConflict && cPut == http.StatusConflict && cDel == http.StatusConflict

		// 等 s1 的 Job 真正在途（run 步骤快照不追踪 running，经 Job 台账观察）
		_, perr := pollJobsByRun(run.ID, func(jobs []jobView) bool {
			return len(jobs) == 1 && jobs[0].Status == "running"
		}, 15*time.Second)
		if perr != nil {
			check(name, false, "等在途步骤: "+perr.Error())
			return
		}
		code, craw := reqJSON(http.MethodPost, *apiBase+"/api/workflow-runs/"+run.ID+"/cancel", nil)
		saveEV("cancel-W5.json", craw)
		if code != 200 {
			check(name, false, fmt.Sprintf("cancel HTTP %d: %s", code, truncate(string(craw), 160)))
			return
		}
		cv, _ := parseRun(craw)
		// 在途 sleep 自然跑完（不被打断），后续不推进
		final, ferr := pollRun(run.ID, func(rv *runView) bool {
			return len(rv.Steps) == 2 && isJobTerminal(rv.Steps[0].Status)
		}, 30*time.Second)
		if ferr != nil {
			check(name, false, "等在途步骤终态: "+ferr.Error())
			return
		}
		saveEV("final-W5.json", mustJSON(final))
		notAdvanced := final.Status == "cancelled" &&
			final.Steps[0].Status == "success" &&
			final.Steps[1].Status == "pending" && final.Steps[1].JobID == "" &&
			final.FinishedAt != nil
		jobs, _ := jobsByRun(run.ID)
		cAgain, rawAgain := reqJSON(http.MethodPost, *apiBase+"/api/workflow-runs/"+run.ID+"/cancel", nil)
		cAgainOK := cAgain == http.StatusConflict && strings.Contains(string(rawAgain), "not running")
		check(name, freezeOK && cv != nil && cv.Status == "cancelled" &&
			notAdvanced && len(jobs) == 1 && cAgainOK,
			fmt.Sprintf("冻结面 run/PUT/DELETE=%d/%d/%d cancel=cancelled+finishedAt 后续不推进=%v 台账=%d 条 再取消 409=%v",
				cRun, cPut, cDel, notAdvanced, len(jobs), cAgainOK))
	}()

	// ---- W7 离线 ghost 不建 run ----
	func() {
		name := "W7 离线 ghost 目标 → 503 不建 run/无 active run 后 DELETE 200"
		wfID := wfCreate("wf-acc-offline", []map[string]interface{}{
			step("s1-ghost", *ghostID, "echo should-not-run", nil),
		})
		code, raw := reqJSON(http.MethodPost, *apiBase+"/api/workflows/"+wfID+"/run", nil)
		saveEV("run503-W7.json", raw)
		runs503, rerr := wfRunsCount(wfID)
		codeDel, rawDel := reqJSON(http.MethodDelete, *apiBase+"/api/workflows/"+wfID, nil)
		delOK := codeDel == 200 && strings.Contains(string(rawDel), `"deleted":true`)
		check(name, code == http.StatusServiceUnavailable &&
			strings.Contains(string(raw), "agent offline") && rerr == nil && runs503 == 0 && delOK,
			fmt.Sprintf("HTTP=%d 含 agent offline=%v run 台账=%d 条（不落幽灵） DELETE=%d",
				code, strings.Contains(string(raw), "agent offline"), runs503, codeDel))
	}()

	// ---- W8 权限与审计 ----
	func() {
		name := "W8 权限与审计：viewer 读 200 写 403/无权限角色全 403/审计三动作齐"
		if w2RunID == "" || w2WfID == "" {
			check(name, false, "W2 未产出 run 样本——先修 W2")
			return
		}
		adminTok := ""
		evMu.Lock()
		adminTok = token
		evMu.Unlock()

		allOK := true
		var detail []string

		// viewer：workflows:read（角色种子自动含）→ 列表/run 详情 200；写面 403
		code, raw := reqJSON(http.MethodPost, *apiBase+"/api/users", map[string]interface{}{
			"username": "workflows-acc-viewer", "password": "acc-viewer-pass-1", "role": "viewer"})
		if code != http.StatusCreated {
			allOK = false
			detail = append(detail, fmt.Sprintf("建 viewer 用户=%d %s", code, truncate(string(raw), 100)))
		}
		vTok, err := loginAs("workflows-acc-viewer", "acc-viewer-pass-1")
		if err != nil {
			check(name, false, "viewer 登录: "+err.Error())
			return
		}
		setToken(vTok)
		cList, _ := reqJSON(http.MethodGet, *apiBase+"/api/workflows", nil)
		cRunGet, _ := reqJSON(http.MethodGet, *apiBase+"/api/workflow-runs/"+w2RunID, nil)
		cCreate, wraw := reqJSON(http.MethodPost, *apiBase+"/api/workflows", map[string]interface{}{
			"name": "should-not-create", "steps": []map[string]interface{}{
				step("s1", *agentA1, "true", nil)}})
		cRunPost, _ := reqJSON(http.MethodPost, *apiBase+"/api/workflows/"+w2WfID+"/run", nil)
		cCancel, _ := reqJSON(http.MethodPost, *apiBase+"/api/workflow-runs/"+w2RunID+"/cancel", nil)
		viewerOK := cList == 200 && cRunGet == 200 &&
			cCreate == 403 && cRunPost == 403 && cCancel == 403
		detail = append(detail, fmt.Sprintf("viewer 列表=%d run详情=%d 建=%d 触发=%d 取消=%d",
			cList, cRunGet, cCreate, cRunPost, cCancel))
		if !viewerOK {
			allOK = false
			detail = append(detail, truncate(string(wraw), 120))
		}

		// 建角色/用户是 roles/users 管理面——还原管理员再做（jobs 探针 J8 先例）
		setToken(adminTok)
		code, raw = reqJSON(http.MethodPost, *apiBase+"/api/roles", map[string]interface{}{
			"name": "wf-acc-norole", "permissions": []string{"dns:read"}})
		if code != http.StatusCreated {
			allOK = false
			detail = append(detail, fmt.Sprintf("建角色=%d %s", code, truncate(string(raw), 100)))
		}
		code, raw = reqJSON(http.MethodPost, *apiBase+"/api/users", map[string]interface{}{
			"username": "workflows-acc-norole", "password": "acc-norole-pass-1", "role": "wf-acc-norole"})
		if code != http.StatusCreated {
			allOK = false
			detail = append(detail, fmt.Sprintf("建 norole 用户=%d %s", code, truncate(string(raw), 100)))
		}
		nTok, err := loginAs("workflows-acc-norole", "acc-norole-pass-1")
		if err != nil {
			check(name, false, "norole 登录: "+err.Error())
			return
		}
		setToken(nTok)
		nList, _ := reqJSON(http.MethodGet, *apiBase+"/api/workflows", nil)
		nRun, wraw2 := reqJSON(http.MethodPost, *apiBase+"/api/workflows/"+w2WfID+"/run", nil)
		noroleOK := nList == 403 && nRun == 403
		detail = append(detail, fmt.Sprintf("norole 列表=%d 触发=%d", nList, nRun))
		if !noroleOK {
			allOK = false
			detail = append(detail, truncate(string(wraw2), 120))
		}

		// 审计：resource=workflow 的 create/run/cancel 三动作（本轮真实操作产生）
		setToken(adminTok)
		code, raw = reqJSON(http.MethodGet,
			*apiBase+"/api/admin/audit/logs?resource=workflow&page_size=100", nil)
		saveEV("audit-workflow.json", raw)
		entries, aerr := parseAuditEntries(raw)
		if aerr != nil || code != 200 {
			allOK = false
			detail = append(detail, fmt.Sprintf("审计读取=%d err=%v", code, aerr))
		} else {
			nCreate := countAuditByAction(entries, "workflow_create")
			nRun := countAuditByAction(entries, "workflow_run")
			nCancel := countAuditByAction(entries, "workflow_cancel")
			auditOK := nCreate >= 3 && nRun >= 2 && nCancel >= 1
			detail = append(detail, fmt.Sprintf("审计 create=%d run=%d cancel=%d", nCreate, nRun, nCancel))
			if !auditOK {
				allOK = false
			}
		}

		setToken(adminTok) // 还原（后续场景/汇总不再发请求，防御性还原）
		check(name, allOK, strings.Join(detail, "；"))
	}()

	// ---- W9 Job cancel 语义 ----
	func() {
		name := "W9 Job cancel：running 取消 409/ghost 404/GET cancel 405"
		code, raw := reqJSON(http.MethodPost, *apiBase+"/api/jobs", map[string]interface{}{
			"type": "agent.exec", "target": *agentA1,
			"parameters": map[string]interface{}{"command": "sleep 12", "timeout_s": 30},
		})
		if code != http.StatusCreated {
			check(name, false, fmt.Sprintf("建 Job HTTP %d（want 201）: %s", code, truncate(string(raw), 160)))
			return
		}
		pending, err := parseJob(raw)
		if err != nil {
			check(name, false, err.Error())
			return
		}
		// 等派发进 running（cancel 只对 pending 放行——running 已在途，如实拒绝）
		v, werr := pollJob(pending.ID, func(j *jobView) bool { return j.Status == "running" }, 10*time.Second)
		if werr != nil {
			check(name, false, "等 running: "+werr.Error())
			return
		}
		codeC, rawC := reqJSON(http.MethodPost, *apiBase+"/api/jobs/"+v.ID+"/cancel", nil)
		cancelOK := codeC == http.StatusConflict && strings.Contains(string(rawC), "only pending")
		codeGhost, _ := reqJSON(http.MethodPost, *apiBase+"/api/jobs/no-such-job/cancel", nil)
		code405, _ := reqJSON(http.MethodGet, *apiBase+"/api/jobs/"+v.ID+"/cancel", nil)
		check(name, cancelOK && codeGhost == 404 && code405 == 405,
			fmt.Sprintf("running 取消=%d（only pending） ghost=%d GET cancel=%d（job=%s 等 12s 自然结束）",
				codeC, codeGhost, code405, truncate(v.ID, 12)))
	}()

	ev("=== 汇总：PASS=%d FAIL=%d（%s） ===", passes, fails, time.Now().Format(time.RFC3339))
	if fails > 0 {
		os.Exit(1)
	}
}

// mustJSON 场景视图落证据的序列化（失败回退占位，不碍主流程）
func mustJSON(v interface{}) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return b
}
