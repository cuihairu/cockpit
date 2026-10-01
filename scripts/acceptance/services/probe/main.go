// 服务管理真实环境验收探针（acceptance-checklist「服务管理（三后端）」systemd
// 行、todo.md systemd 服务管理条目）。以面板用户身份走真实 REST→agent
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
//	T4  unit 文件查看/编辑（D13，a1 root）：GET 有效视图（fragmentPath=
//	    /usr/lib 包管位）→ PUT 改 Description → 包管文件先复制 /etc 覆盖位
//	    再写 + 捆绑 daemon-reload（systemctl show 即时反映新值）→ GET 复读
//	    fragmentPath 已是 /etc；超 256KB 拒 413；坏 unit 名 400 / 多余段
//	    404（路径穿越面只收 unit 名）；非 root PUT 502 permission denied
//	    透传；审计 details.action=unitfile-save 落账
//	T5  journal 日志跳转（D11，含未加载 unit，a1）：PUT jlog 换带唯一
//	    marker 的 ExecStart（dogfood 覆盖位 + 捆绑 reload——start 执行的
//	    是新内容即铁证）→ start 产 journal 历史 → stop + daemon-reload
//	    端点 → jlog 成未加载安装项（list-units --all 无 / 表 loadState 空 /
//	    logs.sources 不含）→ /logs/query 按 unit 派生 source 仍查得到
//	    历史 marker（LogsPanel initialSource 无条件优先派生同款路径）
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
	jlogUnit  = flag.String("jlog", "cockpit-acc-jlog.service", "journal 历史样本 unit")
)

var (
	evFile *os.File
	evMu   sync.Mutex
	passes int
	fails  int
	token  string
	httpC  = &http.Client{Timeout: 30 * time.Second}
)

// 行为中性注入点（先例：guac/probe osExit/dockerExecCmd、logs/probe
// systemdRunCmd）——fatal 退出与本机 systemctl 命令面在单测注入桩覆盖分支，
// 默认值即原行为
var (
	osExit = os.Exit
	shExec = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
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

// serviceActionUnit 对指定 unit 执行动词（jlog 的 start/stop 等）
func serviceActionUnit(agentID, name, act string) (int, []byte) {
	return reqJSON(http.MethodPost,
		*apiBase+"/api/agents/"+agentID+"/services/"+name+"/"+act, nil)
}

// daemonReload POST /services/daemon-reload（D12 工具栏「重载配置」按钮链路）
func daemonReload(agentID string) (int, []byte) {
	return reqJSON(http.MethodPost,
		*apiBase+"/api/agents/"+agentID+"/services/daemon-reload", nil)
}

// unitFileGet GET /services/{unit}/file（D13 有效视图）
func unitFileGet(agentID, name string) (int, []byte) {
	return reqJSON(http.MethodGet,
		*apiBase+"/api/agents/"+agentID+"/services/"+name+"/file", nil)
}

// unitFilePut PUT /services/{unit}/file（D13 保存：覆盖位复制 + 捆绑 reload）
func unitFilePut(agentID, name, content string) (int, []byte) {
	return reqJSON(http.MethodPut,
		*apiBase+"/api/agents/"+agentID+"/services/"+name+"/file",
		map[string]string{"content": content})
}

// logsQuery POST /logs/query（服务行「日志」跳转的查询端点，D11）
func logsQuery(agentID, source string, tail int) (int, string) {
	code, raw := reqJSON(http.MethodPost,
		*apiBase+"/api/agents/"+agentID+"/logs/query",
		map[string]interface{}{
			"type": "systemd", "source": source, "tail": tail,
			"since_minutes": 0, "grep": "",
		})
	var resp struct {
		Lines string `json:"lines"`
	}
	_ = json.Unmarshal(raw, &resp)
	return code, resp.Lines
}

// logsSources GET /logs/sources（只列 loaded/running——未加载 unit 不在表，
// LogsPanel initialSource 无条件优先派生正是为该场景）
func logsSources(agentID string) (int, string) {
	code, raw := reqJSON(http.MethodGet,
		*apiBase+"/api/agents/"+agentID+"/logs/sources", nil)
	return code, string(raw)
}

// auditBody 拉审计（action=service_action 面）原文
func auditBody() string {
	_, raw := reqJSON(http.MethodGet,
		*apiBase+"/api/admin/audit/logs?action=service_action&resource=service&page_size=50", nil)
	return string(raw)
}

// ---------- 本机只读对照（probe 以 cui 直跑）----------

// shOut 组合输出（忽略退出码——is-active/is-enabled 对 inactive/disabled 退出码
// 本就非 0，stdout 才是事实）
func shOut(args ...string) (string, error) {
	out, err := shExec(args[0], args[1:]...)
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
	ev("api=%s unit=%s ghost=%s jlog=%s a1(root)=%s a2(nonroot)=%s",
		*apiBase, *unit, *ghostUnit, *jlogUnit, *agentA1, *agentA2)

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

	// ---- T4 unit 文件查看/编辑（D13 全链，a1 root；拒绝面走 a2/直接构造）----
	func() {
		type unitFile struct {
			Name         string `json:"name"`
			FragmentPath string `json:"fragmentPath"`
			Content      string `json:"content"`
			Path         string `json:"path"`
			Reloaded     bool   `json:"reloaded"`
		}

		// T4a GET 有效视图：包管位 fragmentPath + systemctl cat 全文
		name := "T4a GET unit 文件有效视图（/usr/lib 包管位）"
		codeG, rawG := unitFileGet(*agentA1, *unit)
		var f unitFile
		_ = json.Unmarshal(rawG, &f)
		check(name, codeG == 200 && f.Name == *unit &&
			f.FragmentPath == "/usr/lib/systemd/system/"+*unit &&
			strings.Contains(f.Content, "cockpit acceptance test unit"),
			fmt.Sprintf("HTTP=%d name=%q fragmentPath=%q content=%dB",
				codeG, f.Name, f.FragmentPath, len(f.Content)))

		// T4b PUT 改 Description：包管 → /etc 覆盖位 + 捆绑 daemon-reload
		//（systemctl show 即时反映新值 = reload 真的发生了）+ GET 复读换位 + 审计
		name = "T4b PUT 保存（包管→/etc 覆盖位 + 捆绑 daemon-reload 即时生效）"
		newDesc := fmt.Sprintf("cockpit acceptance test unit R3 %d", time.Now().Unix())
		newContent := strings.Replace(f.Content,
			"cockpit acceptance test unit (throwaway)", newDesc, 1)
		codeP, rawP := unitFilePut(*agentA1, *unit, newContent)
		var r unitFile
		_ = json.Unmarshal(rawP, &r)
		etcPath := "/etc/systemd/system/" + *unit
		etcFile, _ := shOut("cat", etcPath)
		showDesc, _ := shOut("systemctl", "show", "-p", "Description", "--value", *unit)
		codeG2, rawG2 := unitFileGet(*agentA1, *unit)
		var f2 unitFile
		_ = json.Unmarshal(rawG2, &f2)
		auditHasSave := strings.Contains(auditBody(), "unitfile-save")
		check(name, codeP == 200 && r.Path == etcPath && r.Reloaded &&
			strings.Contains(etcFile, newDesc) && showDesc == newDesc &&
			codeG2 == 200 && f2.FragmentPath == etcPath &&
			strings.Contains(f2.Content, newDesc) && auditHasSave,
			fmt.Sprintf("put=%d path=%q reloaded=%v；/etc 落盘含新值=%v systemctl show Description=%q（捆绑 reload 铁证）；复读 fragmentPath=%q 含新值=%v；审计 unitfile-save=%v",
				codeP, r.Path, r.Reloaded, strings.Contains(etcFile, newDesc),
				showDesc, f2.FragmentPath, strings.Contains(f2.Content, newDesc), auditHasSave))

		// T4c 超限拒绝：256KB+1 → 413（server MaxBytesReader 门；agent 侧同限双端防御）
		name = "T4c 内容超限拒绝（256KB+1 → 413）"
		codeBig, _ := unitFilePut(*agentA1, *unit, strings.Repeat("#", 256*1024+1))
		check(name, codeBig == http.StatusRequestEntityTooLarge,
			fmt.Sprintf("HTTP=%d（期望 413）", codeBig))

		// T4d 拒绝面：并集白名单外 400；多余路径段 404（写哪由 FragmentPath
		// 决定、只收 unit 名，无路径参数即无穿越面）；windows 形名过 server
		// 并集、systemd 后端 agent 兜底拒并 502 透传（D9.4 设计内行为）
		name = "T4d 拒绝面（白名单外 400 / 多余段 404 / 后端兜底 502 透传）"
		codeBad, _ := unitFileGet(*agentA1, "zzz:notaservice")
		codeSeg, _ := unitFileGet(*agentA1, *unit+"/file/x")
		codeW, rawW := unitFileGet(*agentA1, "zzz-notaservice")
		var eW struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(rawW, &eW)
		check(name, codeBad == http.StatusBadRequest && codeSeg == http.StatusNotFound &&
			codeW == http.StatusBadGateway && strings.Contains(eW.Error, "invalid unit name"),
			fmt.Sprintf("白名单外(:)=%d（期望 400） extra-seg=%d（期望 404）windows形名=%d error=%q（agent systemd 兜底拒，透传）",
				codeBad, codeSeg, codeW, truncate(eW.Error, 100)))

		// T4e 非 root PUT 透传：/etc 写入 EACCES → 502 permission denied 原文
		name = "T4e 非 root PUT 报错透传（502 permission denied）"
		codeN, rawN := unitFilePut(*agentA2, *unit, newContent)
		var eN struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(rawN, &eN)
		check(name, codeN == http.StatusBadGateway &&
			strings.Contains(strings.ToLower(eN.Error), "permission denied"),
			fmt.Sprintf("HTTP=%d error=%q（a1 同操作 200 见 T4b——权限对照）",
				codeN, truncate(eN.Error, 160)))
	}()

	// ---- T5 journal 日志跳转（D11，含未加载 unit，a1）----
	func() {
		// T5a PUT jlog 换带唯一 marker 的 ExecStart（基线 /bin/true 无输出）→
		// start 执行新内容产 journal 历史——捆绑 daemon-reload 的端到端铁证
		name := "T5a PUT jlog（marker 版）+ start 产 journal 历史"
		marker := fmt.Sprintf("cockpit-jlog-marker-%d", time.Now().UnixNano())
		jlogContent := "[Unit]\nDescription=cockpit acceptance journal sample R3\n" +
			"[Service]\nType=oneshot\n" +
			"ExecStart=/bin/sh -c 'echo " + marker + "'\n" +
			"[Install]\nWantedBy=multi-user.target\n"
		codeP, rawP := unitFilePut(*agentA1, *jlogUnit, jlogContent)
		var r struct {
			Path     string `json:"path"`
			Reloaded bool   `json:"reloaded"`
		}
		_ = json.Unmarshal(rawP, &r)
		codeS, _ := serviceActionUnit(*agentA1, *jlogUnit, "start")
		found := false
		for i := 0; i < 20 && !found; i++ { // oneshot 即完，轮询吸收 journal 异步
			if _, lines := logsQuery(*agentA1, *jlogUnit, 50); strings.Contains(lines, marker) {
				found = true
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		check(name, codeP == 200 && r.Path == "/etc/systemd/system/"+*jlogUnit &&
			r.Reloaded && codeS == 200 && found,
			fmt.Sprintf("put=%d path=%q reloaded=%v start=%d marker 见于查询=%v（start 执行 PUT 后内容=捆绑 reload 生效）",
				codeP, r.Path, r.Reloaded, codeS, found))

		// T5b stop + daemon-reload 端点 → jlog 成未加载安装项（三面证据）
		name = "T5b stop + daemon-reload 端点 → jlog 成未加载安装项"
		codeStop, _ := serviceActionUnit(*agentA1, *jlogUnit, "stop")
		codeR, rawR := daemonReload(*agentA1)
		var rR struct {
			Reloaded bool `json:"reloaded"`
		}
		_ = json.Unmarshal(rawR, &rR)
		auditHasDR := strings.Contains(auditBody(), "daemon-reload")
		unitsAll, _ := shOut("systemctl", "list-units", "--all", "--type=service",
			"--no-legend", "--no-pager")
		list, _, _ := listServices(*agentA1)
		jlog := findUnit(list, *jlogUnit)
		codeSrc, srcBody := logsSources(*agentA1)
		check(name, codeStop == 200 && codeR == 200 && rR.Reloaded && auditHasDR &&
			jlog != nil && jlog.LoadState == "" && jlog.UnitFileState == "disabled" &&
			!strings.Contains(unitsAll, *jlogUnit) &&
			codeSrc == 200 && !strings.Contains(srcBody, *jlogUnit),
			fmt.Sprintf("stop=%d reload=%d reloaded=%v 审计=%v；list-units --all 含 jlog=%v；表 loadState=%q unitFileState=%q；logs.sources 含 jlog=%v",
				codeStop, codeR, rR.Reloaded, auditHasDR,
				strings.Contains(unitsAll, *jlogUnit),
				func() string {
					if jlog != nil {
						return jlog.LoadState
					}
					return "<不在表>"
				}(),
				func() string {
					if jlog != nil {
						return jlog.UnitFileState
					}
					return "-"
				}(),
				strings.Contains(srcBody, *jlogUnit)))

		// （对照记录，不判分）非 root daemon-reload 的透传口径
		codeDR2, rawDR2 := daemonReload(*agentA2)
		ev("      （对照记录）非 root daemon-reload：HTTP=%d %s",
			codeDR2, truncate(string(rawDR2), 120))

		// T5c 未加载 unit 历史日志可查（/logs/query 按 unit 派生 source——
		// LogsPanel initialSource 无条件优先派生同款路径）
		name = "T5c 未加载 unit 历史日志可查（marker 仍在）"
		codeQ, lines := logsQuery(*agentA1, *jlogUnit, 100)
		check(name, codeQ == 200 && strings.Contains(lines, marker),
			fmt.Sprintf("HTTP=%d marker 在结果=%v（返回 %dB）",
				codeQ, strings.Contains(lines, marker), len(lines)))
	}()

	ev("=== 汇总：PASS=%d FAIL=%d（%s） ===", passes, fails, time.Now().Format(time.RFC3339))
	if fails > 0 {
		os.Exit(1)
	}
}
