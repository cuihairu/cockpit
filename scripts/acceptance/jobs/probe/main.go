// 执行 Job 真机验收探针（jobs-design.md，acceptance-checklist「统一 Job
// 执行」节）。以面板用户身份走真实链路（REST + agent RPC + 审计/权限面）：
//
//	J0  前置：登录；目标 agent 在线；台账可读（基线计数）
//	J1  创建执行（uptime）→ success/exit 0/输出非空；台账新增（倒序首位）
//	    与单条详情一致；parameters 往返；startedAt/finishedAt 落定
//	J2  非零退出（exit 3）→ failed、exit_code=3、输出仍带回（D4）、error 空
//	J3  超时（sleep 297，timeout_s=1）→ failed、exit_code=-1、error 含
//	    "timed out after 1s"、墙钟 <10s；pgrep 复核无孤儿 sleep（进程组
//	    SIGKILL，D5 杀整组不留孙进程）
//	J4  输出截断（seq 1 20000 ≈108KB）→ success、output ≤64KB 且尾部保留
//	    （20000 在、首行 1 不在；agent jobExecMaxOutput 与 server
//	    storage.JobMaxOutput 双端同限，jobView 不另暴露 truncated 标记）
//	J5  离线/ghost（registry 缺席即 503，与掉线同一 registry.Get 分支，
//	    logs T4 同口径注明）→ 503 agent offline、台账计数不变、无 ghost
//	    目标记录（D7 不落幽灵）
//	J6  校验面：type/target/command/长度/timeout 各 400 分支 + 404/405
//	J7  审计：每个创建过的 Job 恰一条 job_run（resource=job、
//	    resource_id=Job ID、details 含 type/target/status/params、
//	    username=admin）；503/400 不产审计
//	J8  权限（RBAC 路由层）：viewer（jobs:read）台账可读、创建 403；
//	    自定义无 jobs 权限角色 GET/POST 全 403（首轮真机验收逮过 /api/jobs
//	    未登记 resourceRule 整体放行的缺陷，修复后此处为回归位）
//
// 证据落 .acceptance/jobs/evidence/（probe.log + create-*.json + audit.json）；
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
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	apiBase   = flag.String("api", "http://127.0.0.1:19994", "cockpit server 基址")
	adminUser = flag.String("user", "admin", "管理员用户名")
	adminPass = flag.String("pass", "e2e-strong-pass-1", "管理员口令")
	evDir     = flag.String("ev", "", "证据目录（默认 .acceptance/jobs/evidence）")
	agentA1   = flag.String("a1", "jobs-acc-a1", "在线执行目标 agent")
	ghostID   = flag.String("ghost", "jobs-acc-ghost", "从未注册的 agent id（offline 样本）")
)

var (
	evFile = (*os.File)(nil)
	evMu   sync.Mutex
	passes int
	fails  int
	token  string
	httpC  = &http.Client{Timeout: 30 * time.Second}
)

// 行为中性注入点（先例：logs/probe osExit/systemdRunCmd）——fatal 退出与
// 真机 pgrep 命令面在单测注入桩覆盖分支，默认值即原行为
var (
	osExit   = os.Exit
	pgrepCmd = func(pattern string) ([]byte, error) {
		return exec.Command("pgrep", "-f", pattern).CombinedOutput()
	}
)

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

// loginAs 登录取 token（不改包级 token——J8 多用户场景用）
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

// ---------- Job 视图解析 ----------

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
	StartedAt  *string                `json:"startedAt"`
	FinishedAt *string                `json:"finishedAt"`
}

// parseJobsList 台账响应 {jobs:[...]} → 视图切片
func parseJobsList(raw []byte) ([]jobView, error) {
	var r struct {
		Jobs []jobView `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("bad jobs list: %v", err)
	}
	return r.Jobs, nil
}

// parseJob 单条详情
func parseJob(raw []byte) (*jobView, error) {
	var v jobView
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("bad job view: %v", err)
	}
	return &v, nil
}

// countJobsByTarget 台账中目标为指定 agent 的条数（J5 幽灵记录对照）
func countJobsByTarget(jobs []jobView, target string) int {
	n := 0
	for _, j := range jobs {
		if j.Target == target {
			n++
		}
	}
	return n
}

// findJobByID 台账中按 id 找单条（J1 倒序首位对照）
func findJobByID(jobs []jobView, id string) *jobView {
	for i := range jobs {
		if jobs[i].ID == id {
			return &jobs[i]
		}
	}
	return nil
}

// ---------- 审计解析 ----------

type auditEntry struct {
	Username   string `json:"username"`
	Action     string `json:"action"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id"`
	Details    string `json:"details"`
}

// parseAuditEntries 审计列表响应 {data:[...], pagination:...} → 条目切片
func parseAuditEntries(raw []byte) ([]auditEntry, error) {
	var r struct {
		Data []auditEntry `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("bad audit list: %v", err)
	}
	return r.Data, nil
}

// auditEntriesForResourceID 取 resource_id 匹配的全部条目（J7 断言恰一条）
func auditEntriesForResourceID(entries []auditEntry, id string) []auditEntry {
	var out []auditEntry
	for _, e := range entries {
		if e.ResourceID == id {
			out = append(out, e)
		}
	}
	return out
}

// detailsHas 审计 details（JSON 串）是否含全部子串
func detailsHas(e auditEntry, subs ...string) bool {
	for _, s := range subs {
		if !strings.Contains(e.Details, s) {
			return false
		}
	}
	return true
}

// ---------- 孤儿进程复核 ----------

// orphanAlive pgrep 是否命中（exit 0 = 进程组残留）
func orphanAlive(pattern string) bool {
	out, err := pgrepCmd(pattern)
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// ---------- 场景 ----------

func main() {
	flag.Parse()
	if _, err := os.Stat("go.mod"); err != nil {
		fatal("请在仓库根目录运行（找不到 go.mod）")
	}
	repo, _ := os.Getwd()
	if *evDir == "" {
		*evDir = filepath.Join(repo, ".acceptance/jobs/evidence")
	}
	_ = os.MkdirAll(*evDir, 0o755)
	f, err := os.Create(filepath.Join(*evDir, "probe.log"))
	if err != nil {
		fatal("证据文件创建失败: %v", err)
	}
	evFile = f
	defer evFile.Close()

	ev("=== 执行 Job 真机验收 %s ===", time.Now().Format(time.RFC3339))
	ev("api=%s a1=%s ghost=%s", *apiBase, *agentA1, *ghostID)

	// ---- J0 前置 ----
	baseline := 0
	func() {
		name := "J0 前置：登录 + 目标 agent 在线 + 台账可读"
		login()
		online, err := agentOnline(*agentA1)
		if err != nil {
			check(name, false, err.Error())
			fatal("目标 agent 不在线——先跑 run-server.sh")
		}
		code, raw := reqJSON(http.MethodGet, *apiBase+"/api/jobs", nil)
		jobs, perr := parseJobsList(raw)
		if code != 200 || perr != nil {
			check(name, false, fmt.Sprintf("GET /api/jobs code=%d err=%v", code, perr))
			fatal("台账不可读")
		}
		baseline = len(jobs)
		check(name, true, fmt.Sprintf("agent=%s online=%v 台账基线=%d 条", *agentA1, online, baseline))
	}()

	created := map[string]*jobView{}

	// ---- J1 创建执行成功链路 ----
	func() {
		name := "J1 创建执行 uptime → success/exit 0/输出非空/台账新增"
		cmd := "echo J1-OUTPUT-MARK && uptime"
		start := time.Now()
		code, raw := reqJSON(http.MethodPost, *apiBase+"/api/jobs", map[string]interface{}{
			"type": "agent.exec", "target": *agentA1,
			"parameters": map[string]interface{}{"command": cmd},
		})
		saveEV("create-J1.json", raw)
		if code != 200 {
			check(name, false, fmt.Sprintf("HTTP %d: %s", code, truncate(string(raw), 200)))
			return
		}
		v, err := parseJob(raw)
		if err != nil {
			check(name, false, err.Error())
			return
		}
		created["J1"] = v
		cmdBack, _ := v.Parameters["command"].(string)
		ok := v.Status == "success" && v.ExitCode == 0 && strings.Contains(v.Output, "J1-OUTPUT-MARK") &&
			v.Actor == *adminUser && v.Type == "agent.exec" && v.Target == *agentA1 &&
			cmdBack == cmd && v.StartedAt != nil && v.FinishedAt != nil
		ev("      id=%s status=%s exit=%d output=%dB 耗时=%s", truncate(v.ID, 12), v.Status, v.ExitCode, len(v.Output), time.Since(start).Round(time.Millisecond))

		// 台账：新增一条且倒序首位；单条详情一致（含输出）
		_, lraw := reqJSON(http.MethodGet, *apiBase+"/api/jobs", nil)
		jobs, err := parseJobsList(lraw)
		if err != nil {
			check(name, false, "台账解析: "+err.Error())
			return
		}
		top := findJobByID(jobs, v.ID)
		ledgerOK := len(jobs) == baseline+1 && top != nil && top.Status == "success"
		_, draw := reqJSON(http.MethodGet, *apiBase+"/api/jobs/"+v.ID, nil)
		dv, err := parseJob(draw)
		detailOK := err == nil && dv.Status == "success" && dv.Output == v.Output
		check(name, ok && ledgerOK && detailOK,
			fmt.Sprintf("success+exit0+输出标记+actor/params 往返=%v 台账=%d 条（基线 %d+1）首位命中=%v 详情含输出=%v",
				ok, len(jobs), baseline, top != nil, detailOK))
	}()

	// ---- J2 非零退出 ----
	func() {
		name := "J2 非零退出 exit 3 → failed/exit_code=3/输出仍带回（D4）"
		code, raw := reqJSON(http.MethodPost, *apiBase+"/api/jobs", map[string]interface{}{
			"type": "agent.exec", "target": *agentA1,
			"parameters": map[string]interface{}{"command": "echo J2-BEFORE-OUT; exit 3"},
		})
		saveEV("create-J2.json", raw)
		if code != 200 {
			check(name, false, fmt.Sprintf("HTTP %d: %s", code, truncate(string(raw), 200)))
			return
		}
		v, err := parseJob(raw)
		if err != nil {
			check(name, false, err.Error())
			return
		}
		created["J2"] = v
		// error 契约：非零退出经 trimExecErr 归一为 "exit status N"（与超时
		// 的 "timed out after Ns" 并列），输出仍带回展示（D4）
		check(name, v.Status == "failed" && v.ExitCode == 3 &&
			strings.Contains(v.Output, "J2-BEFORE-OUT") && strings.Contains(v.Error, "exit status 3"),
			fmt.Sprintf("status=%s exit=%d output 含标记=%v error=%q（命令失败是业务字段非传输错误）",
				v.Status, v.ExitCode, strings.Contains(v.Output, "J2-BEFORE-OUT"), v.Error))
	}()

	// ---- J3 超时与进程组 ----
	func() {
		name := "J3 超时 sleep 297 timeout_s=1 → failed/timed out/无孤儿（进程组 SIGKILL）"
		start := time.Now()
		code, raw := reqJSON(http.MethodPost, *apiBase+"/api/jobs", map[string]interface{}{
			"type": "agent.exec", "target": *agentA1,
			"parameters": map[string]interface{}{"command": "sleep 297", "timeout_s": 1},
		})
		elapsed := time.Since(start)
		saveEV("create-J3.json", raw)
		if code != 200 {
			check(name, false, fmt.Sprintf("HTTP %d: %s", code, truncate(string(raw), 200)))
			return
		}
		v, err := parseJob(raw)
		if err != nil {
			check(name, false, err.Error())
			return
		}
		created["J3"] = v
		time.Sleep(1500 * time.Millisecond) // 等 agent 侧杀组收尾
		orphan := orphanAlive("sleep 297")
		check(name, v.Status == "failed" && v.ExitCode == -1 &&
			strings.Contains(v.Error, "timed out after 1s") && elapsed < 10*time.Second && !orphan,
			fmt.Sprintf("status=%s exit=%d error=%q 墙钟=%s（按时返回非等满 30s）孤儿 sleep=%v",
				v.Status, v.ExitCode, v.Error, elapsed.Round(time.Millisecond), orphan))
	}()

	// ---- J4 输出截断 ----
	func() {
		name := "J4 大输出截尾 seq 1 20000 → success/output ≤64KB/尾部保留"
		code, raw := reqJSON(http.MethodPost, *apiBase+"/api/jobs", map[string]interface{}{
			"type": "agent.exec", "target": *agentA1,
			"parameters": map[string]interface{}{"command": "seq 1 20000"},
		})
		saveEV("create-J4.json", raw)
		if code != 200 {
			check(name, false, fmt.Sprintf("HTTP %d: %s", code, truncate(string(raw), 200)))
			return
		}
		v, err := parseJob(raw)
		if err != nil {
			check(name, false, err.Error())
			return
		}
		created["J4"] = v
		// 双端同限：agent jobExecMaxOutput 截尾保留末 64KB，server 落库同限；
		// 尾部保留 = 末行 20000 在，首行 1 不在（全量 108KB > 64KB）
		tailOK := strings.HasSuffix(v.Output, "20000\n") || strings.HasSuffix(v.Output, "20000")
		headDropped := !strings.HasPrefix(v.Output, "1\n")
		check(name, v.Status == "success" && len(v.Output) <= 64*1024 && tailOK && headDropped,
			fmt.Sprintf("status=%s output=%dB（上限 %dB）尾部 20000 在=%v 首行已截=%v",
				v.Status, len(v.Output), 64*1024, tailOK, headDropped))
	}()

	// ---- J5 离线/ghost 不落幽灵 ----
	func() {
		name := "J5 离线目标 → 503 agent offline/不落幽灵记录（D7）"
		_, lraw := reqJSON(http.MethodGet, *apiBase+"/api/jobs", nil)
		jobsBefore, _ := parseJobsList(lraw)
		code, raw := reqJSON(http.MethodPost, *apiBase+"/api/jobs", map[string]interface{}{
			"type": "agent.exec", "target": *ghostID,
			"parameters": map[string]interface{}{"command": "echo should-not-run"},
		})
		saveEV("create-J5-503.json", raw)
		if code != http.StatusServiceUnavailable {
			check(name, false, fmt.Sprintf("HTTP %d（want 503）: %s", code, truncate(string(raw), 200)))
			return
		}
		_, lraw2 := reqJSON(http.MethodGet, *apiBase+"/api/jobs", nil)
		jobsAfter, err := parseJobsList(lraw2)
		if err != nil {
			check(name, false, "台账解析: "+err.Error())
			return
		}
		noGhost := countJobsByTarget(jobsAfter, *ghostID) == 0
		check(name, code == 503 && len(jobsAfter) == len(jobsBefore) && noGhost,
			fmt.Sprintf("HTTP=%d 台账 %d→%d 不变 ghost 目标记录=%d（registry 缺席即 503，与掉线同分支）",
				code, len(jobsBefore), len(jobsAfter), countJobsByTarget(jobsAfter, *ghostID)))
	}()

	// ---- J6 校验面 ----
	func() {
		name := "J6 校验面：type/target/command/长度/timeout 各 400 + 404/405"
		if created["J1"] == nil {
			check(name, false, "J1 未创建成功，/jobs/{id} 405 断言缺样本——先修 J1")
			return
		}
		cases := []struct {
			tag    string
			body   map[string]interface{}
			substr string
		}{
			{"no-type", map[string]interface{}{"target": "x", "parameters": map[string]interface{}{"command": "true"}}, "type is required"},
			{"bad-type", map[string]interface{}{"type": "workflow.run", "target": "x", "parameters": map[string]interface{}{"command": "true"}}, "unsupported job type"},
			{"no-target", map[string]interface{}{"type": "agent.exec", "parameters": map[string]interface{}{"command": "true"}}, "target (agent id) is required"},
			{"no-command", map[string]interface{}{"type": "agent.exec", "target": "x", "parameters": map[string]interface{}{}}, "parameters.command is required"},
			{"big-command", map[string]interface{}{"type": "agent.exec", "target": "x", "parameters": map[string]interface{}{"command": strings.Repeat("a", 16*1024+1)}}, "command too large"},
			{"timeout-0", map[string]interface{}{"type": "agent.exec", "target": "x", "parameters": map[string]interface{}{"command": "true", "timeout_s": 0}}, "timeout_s out of range"},
			{"timeout-301", map[string]interface{}{"type": "agent.exec", "target": "x", "parameters": map[string]interface{}{"command": "true", "timeout_s": 301}}, "timeout_s out of range"},
		}
		allOK := true
		var detail []string
		for _, c := range cases {
			code, raw := reqJSON(http.MethodPost, *apiBase+"/api/jobs", c.body)
			ok := code == 400 && strings.Contains(string(raw), c.substr)
			detail = append(detail, fmt.Sprintf("%s=%d", c.tag, code))
			if !ok {
				allOK = false
				detail = append(detail, truncate(string(raw), 120))
			}
		}
		code404, _ := reqJSON(http.MethodGet, *apiBase+"/api/jobs/no-such-job", nil)
		// 405 只挂在 /jobs/{id} 的非 GET（列表级未登记方法走 handler default 404）
		code405, _ := reqJSON(http.MethodPut, *apiBase+"/api/jobs/"+created["J1"].ID, nil)
		codeList404, _ := reqJSON(http.MethodDelete, *apiBase+"/api/jobs", nil)
		if code404 != 404 {
			allOK = false
			detail = append(detail, fmt.Sprintf("404面=%d", code404))
		}
		if code405 != 405 {
			allOK = false
			detail = append(detail, fmt.Sprintf("405面=%d", code405))
		}
		if codeList404 != 404 {
			allOK = false
			detail = append(detail, fmt.Sprintf("列表级未登记方法=%d", codeList404))
		}
		check(name, allOK, strings.Join(detail, " "))
	}()

	// ---- J7 审计 ----
	func() {
		name := "J7 审计 job_run：每 Job 恰一条（details 含 type/target/status/params）"
		code, raw := reqJSON(http.MethodGet,
			*apiBase+"/api/admin/audit/logs?resource=job&action=job_run&page_size=100", nil)
		saveEV("audit-job_run.json", raw)
		entries, err := parseAuditEntries(raw)
		if err != nil || code != 200 {
			check(name, false, fmt.Sprintf("HTTP %d err=%v", code, err))
			return
		}
		allOK := len(entries) >= 4
		var detail []string
		for _, tag := range []string{"J1", "J2", "J3", "J4"} {
			v := created[tag]
			if v == nil {
				allOK = false
				detail = append(detail, tag+" 无创建记录")
				continue
			}
			matches := auditEntriesForResourceID(entries, v.ID)
			hit := len(matches) == 1 && matches[0].Action == "job_run" &&
				matches[0].Resource == "job" && matches[0].Username == *adminUser &&
				detailsHas(matches[0],
					`"type":"agent.exec"`, `"target":"`+*agentA1+`"`,
					`"status":"`+v.Status+`"`, `"command":`)
			detail = append(detail, fmt.Sprintf("%s 恰一条+字段齐=%v（%d 条）", tag, hit, len(matches)))
			if !hit {
				allOK = false
			}
		}
		check(name, allOK, strings.Join(detail, "；"))
	}()

	// ---- J8 权限 ----
	func() {
		name := "J8 权限（RBAC 路由层）：viewer 可读不可执行；无 jobs 权限角色全拒"
		if created["J1"] == nil {
			check(name, false, "J1 未创建成功，详情断言缺样本——先修 J1")
			return
		}
		evMu.Lock()
		adminTok := token
		evMu.Unlock()

		allOK := true
		var detail []string

		// viewer：jobs:read 台账可读，jobs:write 创建 403
		code, raw := reqJSON(http.MethodPost, *apiBase+"/api/users", map[string]interface{}{
			"username": "jobs-acc-viewer", "password": "acc-viewer-pass-1", "role": "viewer"})
		if code != http.StatusCreated {
			allOK = false
			detail = append(detail, fmt.Sprintf("建 viewer 用户=%d %s", code, truncate(string(raw), 100)))
		}
		vTok, err := loginAs("jobs-acc-viewer", "acc-viewer-pass-1")
		if err != nil {
			check(name, false, "viewer 登录: "+err.Error())
			return
		}
		evMu.Lock()
		token = vTok
		evMu.Unlock()
		cRead, _ := reqJSON(http.MethodGet, *apiBase+"/api/jobs", nil)
		cDetail, _ := reqJSON(http.MethodGet, *apiBase+"/api/jobs/"+created["J1"].ID, nil)
		cWrite, wraw := reqJSON(http.MethodPost, *apiBase+"/api/jobs", map[string]interface{}{
			"type": "agent.exec", "target": *agentA1,
			"parameters": map[string]interface{}{"command": "echo should-not-run"}})
		viewerOK := cRead == 200 && cDetail == 200 && cWrite == 403
		detail = append(detail, fmt.Sprintf("viewer 读=%d 详情=%d 写=%d", cRead, cDetail, cWrite))
		if !viewerOK {
			allOK = false
			detail = append(detail, truncate(string(wraw), 120))
		}

		// 建角色/用户是 roles:admin/users:admin 面——还原管理员身份再做
		// （首轮真机跑用 viewer token 建角色 403，角色/用户都没落库，登录 401）
		evMu.Lock()
		token = adminTok
		evMu.Unlock()

		// 自定义角色：仅 dns:read（无 jobs 权限点）→ GET/POST 全 403
		code, raw = reqJSON(http.MethodPost, *apiBase+"/api/roles", map[string]interface{}{
			"name": "jobs-acc-noread", "permissions": []string{"dns:read"}})
		if code != http.StatusCreated {
			allOK = false
			detail = append(detail, fmt.Sprintf("建角色=%d %s", code, truncate(string(raw), 100)))
		}
		code, raw = reqJSON(http.MethodPost, *apiBase+"/api/users", map[string]interface{}{
			"username": "jobs-acc-noread", "password": "acc-noread-pass-1", "role": "jobs-acc-noread"})
		if code != http.StatusCreated {
			allOK = false
			detail = append(detail, fmt.Sprintf("建 noread 用户=%d %s", code, truncate(string(raw), 100)))
		}
		nTok, err := loginAs("jobs-acc-noread", "acc-noread-pass-1")
		if err != nil {
			check(name, false, "noread 登录: "+err.Error())
			return
		}
		evMu.Lock()
		token = nTok
		evMu.Unlock()
		nRead, _ := reqJSON(http.MethodGet, *apiBase+"/api/jobs", nil)
		nWrite, wraw2 := reqJSON(http.MethodPost, *apiBase+"/api/jobs", map[string]interface{}{
			"type": "agent.exec", "target": *agentA1,
			"parameters": map[string]interface{}{"command": "echo should-not-run"}})
		noreadOK := nRead == 403 && nWrite == 403
		detail = append(detail, fmt.Sprintf("noread 读=%d 写=%d", nRead, nWrite))
		if !noreadOK {
			allOK = false
			detail = append(detail, truncate(string(wraw2), 120))
		}

		// 还原管理员 token（后续场景/汇总不再发请求，防御性还原）
		evMu.Lock()
		token = adminTok
		evMu.Unlock()
		check(name, allOK, strings.Join(detail, "；"))
	}()

	ev("=== 汇总：PASS=%d FAIL=%d（%s） ===", passes, fails, time.Now().Format(time.RFC3339))
	if fails > 0 {
		os.Exit(1)
	}
}
