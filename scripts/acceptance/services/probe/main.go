// 服务管理真实环境验收探针（acceptance-checklist「服务管理（三后端）」systemd
// 行、todo.md systemd 服务管理条目剩余三件）。以面板用户身份走真实 REST→agent
// →systemctl 链路，本机 systemd 主机、测试 unit（cockpit-acc-*，不碰业务 unit）：
//
//	T0  前置：登录；双 agent service capability；a1 系统状态可读
//	T1  列表含未加载 unit：cockpit-acc-ghost（装 /etc 永不 enable/start，
//	    daemon-reload 后无引用不 load）→ 合并列表含它且 activeState 兜底
//	    inactive、unitFileState=disabled；probe 直跑运行表/安装表双对照
//	T2  root agent 动词面（a1，sudo 起）：restart（MainPID 周期更换）→
//	    stop/start → disable/enable → mask（is-enabled=masked、列表 masked）
//	    → masked 上 restart 被拒 502 原文透传 → unmask 还原 enabled；
//	    收尾审计 service_action 落账断言
//	T3  非 root agent 报错透传（a2，cui）：polkit 拒非交互授权（实测
//	    Access denied ... interactive authentication），restart/enable 均
//	    502 + systemctl 原文（含 unit 名）——与 a1 同操作 200 构成对照
//
// is-active/is-enabled 非 root 可读（stdout 为准、退出码非 0 忽略：inactive/
// disabled 的 is-* 退出码本就非 0）。证据落 .acceptance/services/evidence/
// （probe.log）；FAIL → exit 1。
package main

import (
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
	apiBase   = flag.String("api", "http://127.0.0.1:19993", "cockpit server 基址")
	adminUser = flag.String("user", "admin", "管理员用户名")
	adminPass = flag.String("pass", "e2e-strong-pass-1", "管理员口令")
	evDir     = flag.String("ev", "", "证据目录（默认 .acceptance/services/evidence）")
	agentA1   = flag.String("a1", "svc-acc-a1", "root agent（成功组）")
	agentA2   = flag.String("a2", "svc-acc-a2", "非 root agent（报错透传组）")
	unit      = flag.String("unit", "cockpit-acc-svc.service", "测试操作 unit")
	ghostUnit = flag.String("ghost", "cockpit-acc-ghost.service", "未加载样本 unit")
)

var (
	evFile *os.File
	evMu   sync.Mutex
	passes int
	fails  int
	token  string
	httpC  = &http.Client{Timeout: 30 * time.Second}
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
	os.Exit(2)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ---------- cockpit REST ----------

func reqJSON(method, url string, body interface{}) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, url, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpC.Do(req)
	if err != nil {
		return -1, []byte(err.Error())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func login() {
	code, raw := reqJSON(http.MethodPost, *apiBase+"/api/auth/login",
		map[string]string{"username": *adminUser, "password": *adminPass})
	if code != 200 {
		fatal("登录失败 HTTP %d: %s", code, truncate(string(raw), 200))
	}
	var r struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Token == "" {
		fatal("登录响应无 token: %s", truncate(string(raw), 200))
	}
	token = r.Token
}

// agentHasCapability /api/agents 为 DB 视图（裸数组与 {agents:[..]} 包装两形态兼容）
func agentHasCapability(id, capType string) (bool, error) {
	_, raw := reqJSON(http.MethodGet, *apiBase+"/api/agents", nil)
	type agentEntry struct {
		ID           string `json:"id"`
		Capabilities []struct {
			Type string `json:"type"`
		} `json:"capabilities"`
	}
	match := func(list []agentEntry) (bool, error) {
		for _, a := range list {
			if a.ID != id {
				continue
			}
			for _, c := range a.Capabilities {
				if c.Type == capType {
					return true, nil
				}
			}
			return false, nil
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

// serviceAction 执行动词；返回 (httpStatus, body)
func serviceAction(agentID, act string) (int, []byte) {
	return reqJSON(http.MethodPost,
		*apiBase+"/api/agents/"+agentID+"/services/"+*unit+"/"+act, nil)
}

// ---------- 本机只读对照（probe 以 cui 直跑）----------

// shOut 组合输出（忽略退出码——is-active/is-enabled 对 inactive/disabled 退出码
// 本就非 0，stdout 才是事实）
func shOut(args ...string) (string, error) {
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func isActive() string {
	s, _ := shOut("systemctl", "is-active", *unit)
	return s
}

func isEnabled() string {
	s, _ := shOut("systemctl", "is-enabled", *unit)
	return s
}

func mainPID() string {
	s, _ := shOut("systemctl", "show", "-p", "MainPID", "--value", *unit)
	return s
}

// ---------- 服务列表 ----------

type serviceUnit struct {
	Name          string `json:"name"`
	LoadState     string `json:"loadState"`
	ActiveState   string `json:"activeState"`
	SubState      string `json:"subState"`
	UnitFileState string `json:"unitFileState"`
}

func listServices(agentID string) ([]serviceUnit, int, string) {
	code, raw := reqJSON(http.MethodGet, *apiBase+"/api/agents/"+agentID+"/services", nil)
	if code != 200 {
		return nil, code, truncate(string(raw), 200)
	}
	var resp struct {
		Services []serviceUnit `json:"services"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, code, "bad response: " + err.Error()
	}
	return resp.Services, code, ""
}

func findUnit(list []serviceUnit, name string) *serviceUnit {
	for i := range list {
		if list[i].Name == name {
			return &list[i]
		}
	}
	return nil
}

// ---------- 场景 ----------

func main() {
	flag.Parse()
	if _, err := os.Stat("go.mod"); err != nil {
		fatal("请在仓库根目录运行（找不到 go.mod）")
	}
	repo, _ := os.Getwd()
	if *evDir == "" {
		*evDir = filepath.Join(repo, ".acceptance/services/evidence")
	}
	_ = os.MkdirAll(*evDir, 0o755)
	f, err := os.Create(filepath.Join(*evDir, "probe.log"))
	if err != nil {
		fatal("证据文件创建失败: %v", err)
	}
	evFile = f
	defer evFile.Close()

	ev("=== 服务管理真实环境验收 %s ===", time.Now().Format(time.RFC3339))
	ev("api=%s unit=%s ghost=%s a1(root)=%s a2(nonroot)=%s",
		*apiBase, *unit, *ghostUnit, *agentA1, *agentA2)

	login()
	ev("登录成功（%s）", *adminUser)

	// ---- T0 前置：双 agent service capability + 系统状态可读 ----
	c1, err1 := agentHasCapability(*agentA1, "service")
	c2, err2 := agentHasCapability(*agentA2, "service")
	codeSt, rawSt := reqJSON(http.MethodGet, *apiBase+"/api/agents/"+*agentA1+"/services/status", nil)
	var st struct {
		SystemState string `json:"systemState"`
	}
	_ = json.Unmarshal(rawSt, &st)
	t0 := c1 && err1 == nil && c2 && err2 == nil && codeSt == 200 && st.SystemState != ""
	check("T0 双 agent service capability + 系统状态可读", t0,
		fmt.Sprintf("a1.service=%v a2.service=%v status=%d systemState=%q",
			c1, c2, codeSt, st.SystemState))
	if !t0 {
		fatal("前置不符——先查 run-server 双 agent 日志（capability 探测=LookPath systemctl + /run/systemd/system）")
	}

	// ---- T1 列表含未加载 unit（a1 面）----
	func() {
		name := "T1 列表含未加载 unit（ghost 安装未加载，activeState 兜底 inactive）"
		list, code, why := listServices(*agentA1)
		if code != 200 {
			check(name, false, fmt.Sprintf("GET services=%d %s", code, why))
			return
		}
		ghost := findUnit(list, *ghostUnit)
		svc := findUnit(list, *unit)
		ghostOK := ghost != nil && ghost.ActiveState == "inactive" &&
			ghost.UnitFileState == "disabled"
		svcOK := svc != nil && svc.ActiveState == "active" &&
			svc.UnitFileState == "enabled"
		// 真机对照：运行表（list-units）确无 ghost、安装表（list-unit-files）有
		unitsOut, _ := shOut("systemctl", "list-units", "--type=service",
			"--no-legend", "--no-pager")
		filesOut, _ := shOut("systemctl", "list-unit-files", "--type=service",
			"--no-legend", "--no-pager")
		contrastOK := !strings.Contains(unitsOut, *ghostUnit) &&
			strings.Contains(filesOut, *ghostUnit)
		detail := "ghost: "
		if ghost != nil {
			detail += fmt.Sprintf("loadState=%q activeState=%q unitFileState=%q（loadState 空=未加载）",
				ghost.LoadState, ghost.ActiveState, ghost.UnitFileState)
		} else {
			detail += "不在列表"
		}
		if svc != nil {
			detail += fmt.Sprintf("；svc: activeState=%q unitFileState=%q", svc.ActiveState, svc.UnitFileState)
		}
		detail += fmt.Sprintf("；运行表含 ghost=%v 安装表含 ghost=%v（对照）",
			strings.Contains(unitsOut, *ghostUnit), strings.Contains(filesOut, *ghostUnit))
		check(name, ghostOK && svcOK && contrastOK, detail)
	}()

	// ---- T2 root agent 动词面（a1）----
	func() {
		// restart：MainPID 周期更换 + 仍 active
		name := "T2a restart 实测（root agent，MainPID 周期更换）"
		pid0 := mainPID()
		code, _ := serviceAction(*agentA1, "restart")
		pid1 := mainPID()
		act := isActive()
		check(name, code == 200 && pid0 != "" && pid1 != "" && pid0 != pid1 && act == "active",
			fmt.Sprintf("HTTP=%d MainPID %s→%s is-active=%s", code, pid0, pid1, act))

		// stop/start 往返
		name = "T2b stop/start 往返"
		codeS1, _ := serviceAction(*agentA1, "stop")
		act1 := isActive()
		codeS2, _ := serviceAction(*agentA1, "start")
		act2 := isActive()
		check(name, codeS1 == 200 && act1 == "inactive" && codeS2 == 200 && act2 == "active",
			fmt.Sprintf("stop=%d→%s start=%d→%s", codeS1, act1, codeS2, act2))

		// disable/enable 往返（非 root 同操作被拒——见 T3，权限对照）
		name = "T2c disable/enable 往返（自启态）"
		codeD, _ := serviceAction(*agentA1, "disable")
		en1 := isEnabled()
		codeE, _ := serviceAction(*agentA1, "enable")
		en2 := isEnabled()
		check(name, codeD == 200 && en1 == "disabled" && codeE == 200 && en2 == "enabled",
			fmt.Sprintf("disable=%d→%s enable=%d→%s", codeD, en1, codeE, en2))

		// mask → masked 拒启 → unmask 还原
		name = "T2d mask/解屏蔽（masked 拒启 502 透传 + unmask 还原 enabled）"
		codeM, _ := serviceAction(*agentA1, "mask")
		enM := isEnabled()
		listM, _, _ := listServices(*agentA1)
		var fileStateM string
		if u := findUnit(listM, *unit); u != nil {
			fileStateM = u.UnitFileState
		}
		codeR, rawR := serviceAction(*agentA1, "restart")
		var errR struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(rawR, &errR)
		codeU, _ := serviceAction(*agentA1, "unmask")
		enU := isEnabled()
		listU, _, _ := listServices(*agentA1)
		var fileStateU string
		if u := findUnit(listU, *unit); u != nil {
			fileStateU = u.UnitFileState
		}
		maskedRefused := codeR == 502 && strings.Contains(errR.Error, "is masked") &&
			strings.Contains(errR.Error, *unit)
		check(name, codeM == 200 && enM == "masked" && fileStateM == "masked" &&
			maskedRefused && codeU == 200 && enU == "enabled" && fileStateU == "enabled",
			fmt.Sprintf("mask=%d→is-enabled=%s 列表=%s；masked restart=%d 含 is-masked=%v；unmask=%d→is-enabled=%s 列表=%s",
				codeM, enM, fileStateM, codeR,
				strings.Contains(errR.Error, "is masked"), codeU, enU, fileStateU))

		// 审计 service_action 落账（值子串匹配防 schema 耦合）
		name = "T2e 审计 service_action 落账"
		_, rawA := reqJSON(http.MethodGet,
			*apiBase+"/api/admin/audit/logs?action=service_action&resource=service&page_size=50", nil)
		bodyA := string(rawA)
		auditOK := strings.Contains(bodyA, "service_action") &&
			strings.Contains(bodyA, *unit) &&
			strings.Contains(bodyA, *agentA1)
		check(name, auditOK, fmt.Sprintf("审计含 service_action=%v unit=%v agent=%v（响应 %dB）",
			strings.Contains(bodyA, "service_action"),
			strings.Contains(bodyA, *unit),
			strings.Contains(bodyA, *agentA1), len(bodyA)))
	}()

	// ---- T3 非 root agent 报错透传（a2）----
	func() {
		name := "T3a 非 root restart 报错透传（502 + systemctl 原文含 unit 名）"
		code, raw := serviceAction(*agentA2, "restart")
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &errBody)
		passed := code == 502 &&
			strings.Contains(errBody.Error, "Access denied") &&
			strings.Contains(errBody.Error, *unit)
		check(name, passed, fmt.Sprintf("HTTP=%d error=%q（a1 同操作 200 见 T2a——权限对照）",
			code, truncate(errBody.Error, 160)))

		name = "T3b 非 root enable 报错透传（502 + Access denied 原文）"
		code, raw = serviceAction(*agentA2, "enable")
		_ = json.Unmarshal(raw, &errBody)
		passed = code == 502 &&
			strings.Contains(errBody.Error, "Access denied") &&
			strings.Contains(errBody.Error, *unit)
		check(name, passed, fmt.Sprintf("HTTP=%d error=%q", code, truncate(errBody.Error, 160)))

		// 收尾：a1 把自启态复位 enabled（T2c 结束已是 enabled；unmask 后亦 enabled）
		// —— teardown 会彻底移除测试 unit，此处仅记录末态
		ev("      末态：is-active=%s is-enabled=%s", isActive(), isEnabled())
	}()

	ev("=== 汇总：PASS=%d FAIL=%d（%s） ===", passes, fails, time.Now().Format(time.RFC3339))
	if fails > 0 {
		os.Exit(1)
	}
}
