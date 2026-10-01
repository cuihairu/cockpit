// systemd 服务管理真机验收探针（service-design.md M1 真机项，
// acceptance-checklist「服务管理」节 systemd 首项部分覆盖）。
// 以面板用户身份走真实链路（server :19993 + root/noroot 双 agent 同机）：
//
//	T0  前置：登录；双 agent 在线且 service capability 口径正确
//	    （version=2、metadata.backend=systemd）
//	T1  列表含未加载 unit：root 侧 GET services——自建测试 unit 在列；
//	    至少一项 files-only 未加载项（LoadState 空 + ActiveState inactive，
//	    mergeServiceUnits 只出现在 list-unit-files 的分支）；列表不止运行项
//	    （total > active）；列表快照存 evidence/services-list.json
//	T2  restart/enable 实测（root 侧，测试 unit 专属）：restart → 200 且
//	    active/running；enable → 200 且 UnitFileState=enabled
//	T3  非 root agent 报错透传：noroot 侧 restart/enable 同一测试 unit →
//	    502 且体含 "systemctl" 与 unit 名（Access denied 原文透传）；对照组
//	    noroot 侧 GET 列表 200——证明是纯权限失败非连通性问题
//	T4  收尾断言（测试 unit 还原）：stop → inactive；disable → disabled；
//	    文件删除与 daemon-reload 由 teardown-env.sh 兜底
//
// 证据落 .acceptance/services/evidence/（probe.log + services-list.json）；
// FAIL → exit 1。只动 cockpit-acc-svc.service，绝不动用户业务 unit。
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
	apiBase  = flag.String("api", "http://127.0.0.1:19993", "cockpit server 基址")
	adminU   = flag.String("user", "admin", "管理员用户名")
	adminP   = flag.String("pass", "e2e-strong-pass-1", "管理员口令")
	evDir    = flag.String("ev", "", "证据目录（默认 .acceptance/services/evidence）")
	rootID   = flag.String("root", "svc-acc-root", "root agent（动词成功样本）")
	norootID = flag.String("noroot", "svc-acc-noroot", "非 root agent（报错透传样本）")
	unit     = flag.String("unit", "cockpit-acc-svc.service", "测试 unit（专属）")
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

func req(method, url string) (int, []byte) {
	r, _ := http.NewRequest(method, url, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpC.Do(r)
	if err != nil {
		return -1, []byte(err.Error())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func post(url string) (int, []byte) { return req(http.MethodPost, url) }

func login() {
	body, _ := json.Marshal(map[string]string{"username": *adminU, "password": *adminP})
	r, _ := http.NewRequest(http.MethodPost, *apiBase+"/api/auth/login", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	resp, err := httpC.Do(r)
	if err != nil {
		fatal("登录请求失败: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fatal("登录失败 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var t struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(raw, &t) != nil || t.Token == "" {
		fatal("登录响应无 token: %s", truncate(string(raw), 200))
	}
	token = t.Token
}

// serviceBackend 读某 agent 的 service capability backend（裸数组与
// {agents:[..]} 包装两形态兼容，logs 探针同款）
func serviceBackend(id string) (string, error) {
	_, raw := req(http.MethodGet, *apiBase+"/api/agents")
	type cap struct {
		Type     string                 `json:"type"`
		Version  string                 `json:"version"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	type entry struct {
		ID           string `json:"id"`
		Capabilities []cap  `json:"capabilities"`
	}
	find := func(list []entry) (string, error) {
		for _, a := range list {
			if a.ID != id {
				continue
			}
			for _, c := range a.Capabilities {
				if c.Type == "service" {
					be, _ := c.Metadata["backend"].(string)
					return be + "/v" + c.Version, nil
				}
			}
			return "", fmt.Errorf("agent %s 无 service capability", id)
		}
		return "", fmt.Errorf("agent %s 不在 /api/agents 列表", id)
	}
	var list []entry
	if json.Unmarshal(raw, &list) == nil {
		return find(list)
	}
	var w struct {
		Agents []entry `json:"agents"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		return "", fmt.Errorf("bad /api/agents: %v", err)
	}
	return find(w.Agents)
}

// ---------- services 面 ----------

type serviceUnit struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	LoadState     string `json:"loadState"`
	ActiveState   string `json:"activeState"`
	SubState      string `json:"subState"`
	UnitFileState string `json:"unitFileState"`
	Preset        string `json:"preset"`
}

func getList(agent string) (int, []serviceUnit, []byte) {
	code, raw := req(http.MethodGet, *apiBase+"/api/agents/"+agent+"/services")
	if code != 200 {
		return code, nil, raw
	}
	var r struct {
		Services []serviceUnit `json:"services"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return -1, nil, []byte(err.Error())
	}
	return code, r.Services, raw
}

func findUnit(units []serviceUnit, name string) *serviceUnit {
	for i := range units {
		if units[i].Name == name {
			return &units[i]
		}
	}
	return nil
}

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

	ev("=== systemd 服务管理真机验收 %s ===", time.Now().Format(time.RFC3339))
	ev("api=%s root=%s noroot=%s unit=%s", *apiBase, *rootID, *norootID, *unit)

	login()
	ev("登录成功（%s）", *adminU)

	// ---- T0 前置：双 agent service capability 口径 ----
	t0OK := func() bool {
		r, errR := serviceBackend(*rootID)
		n, errN := serviceBackend(*norootID)
		if errR != nil || errN != nil {
			check("T0 service capability 口径", false,
				fmt.Sprintf("root=%s(%v) noroot=%s(%v)", r, errR, n, errN))
			return false
		}
		ok := r == "systemd/v2" && n == "systemd/v2"
		check("T0 service capability 口径（双 agent systemd/v2）", ok,
			fmt.Sprintf("root=%s noroot=%s（uid 差异不影响探测）", r, n))
		return ok
	}()
	if !t0OK {
		fatal("capability 口径不符——先查 run-server 双 agent 日志")
	}

	// ---- T1 列表含未加载 unit（root 侧）----
	var listed *serviceUnit
	func() {
		name := "T1 服务列表含未加载 unit（total>active + files-only 项 + 测试 unit 在列）"
		code, units, raw := getList(*rootID)
		if code != 200 {
			check(name, false, fmt.Sprintf("GET services HTTP %d: %s", code, truncate(string(raw), 200)))
			return
		}
		_ = os.WriteFile(filepath.Join(*evDir, "services-list.json"), raw, 0o644)
		active, notloadedName := 0, ""
		for i := range units {
			if units[i].ActiveState == "active" {
				active++
			}
			// files-only 未加载项：merge 时无 LoadState/Description，
			// ActiveState 兜底 inactive（service_provider.go mergeServiceUnits）
			if notloadedName == "" && units[i].ActiveState == "inactive" &&
				units[i].LoadState == "" && units[i].Description == "" {
				notloadedName = units[i].Name
			}
		}
		listed = findUnit(units, *unit)
		ok := len(units) > active && notloadedName != "" && listed != nil
		ev("      total=%d active=%d 未加载样本=%q 测试 unit=%v",
			len(units), active, notloadedName, listed != nil)
		check(name, ok, fmt.Sprintf("total=%d>active=%d 未加载=%q 测试 unit 在列=%v",
			len(units), active, notloadedName, listed != nil))
	}()

	// ---- T2 restart/enable 实测（root 侧，测试 unit 专属）----
	func() {
		name := "T2a restart 实测（非运行测试 unit → active/running）"
		code, raw := post(*apiBase + "/api/agents/" + *rootID + "/services/" + *unit + "/restart")
		if code != 200 {
			check(name, false, fmt.Sprintf("HTTP %d: %s", code, truncate(string(raw), 200)))
			return
		}
		_, units, _ := getList(*rootID)
		u := findUnit(units, *unit)
		ok := u != nil && u.ActiveState == "active" && u.SubState == "running"
		state := "?"
		if u != nil {
			state = u.ActiveState + "/" + u.SubState
		}
		check(name, ok, fmt.Sprintf("restart 200，现态=%s", state))
	}()
	func() {
		name := "T2b enable 实测（自启态 → enabled）"
		code, raw := post(*apiBase + "/api/agents/" + *rootID + "/services/" + *unit + "/enable")
		if code != 200 {
			check(name, false, fmt.Sprintf("HTTP %d: %s", code, truncate(string(raw), 200)))
			return
		}
		_, units, _ := getList(*rootID)
		u := findUnit(units, *unit)
		ok := u != nil && u.UnitFileState == "enabled"
		state := "?"
		if u != nil {
			state = u.UnitFileState
		}
		check(name, ok, fmt.Sprintf("enable 200，自启态=%s", state))
	}()

	// ---- T3 非 root agent 报错透传 ----
	func() {
		name := "T3a 非 root restart 报错透传（502 + systemctl 原文）"
		code, raw := post(*apiBase + "/api/agents/" + *norootID + "/services/" + *unit + "/restart")
		body := string(raw)
		ok := code == http.StatusBadGateway &&
			strings.Contains(body, "systemctl") && strings.Contains(body, *unit)
		ev("      noroot restart: HTTP %d body=%q", code, truncate(body, 160))
		check(name, ok, fmt.Sprintf("HTTP %d（want 502）含 systemctl 原文=%v 含 unit 名=%v",
			code, strings.Contains(body, "systemctl"), strings.Contains(body, *unit)))
	}()
	func() {
		name := "T3b 非 root enable 报错透传（502 + systemctl 原文）"
		code, raw := post(*apiBase + "/api/agents/" + *norootID + "/services/" + *unit + "/enable")
		body := string(raw)
		ok := code == http.StatusBadGateway &&
			strings.Contains(body, "systemctl") && strings.Contains(body, *unit)
		ev("      noroot enable: HTTP %d body=%q", code, truncate(body, 160))
		check(name, ok, fmt.Sprintf("HTTP %d（want 502）含 systemctl 原文=%v 含 unit 名=%v",
			code, strings.Contains(body, "systemctl"), strings.Contains(body, *unit)))
	}()
	func() {
		name := "T3c 对照组：非 root 读列表正常（纯权限失败非连通问题）"
		code, units, raw := getList(*norootID)
		ok := code == 200 && len(units) > 0 && findUnit(units, *unit) != nil
		check(name, ok, fmt.Sprintf("noroot GET services HTTP %d unit 数=%d（want 200 且非空）",
			code, len(units)))
		_ = raw
	}()

	// ---- T4 收尾断言：测试 unit 还原（stop → inactive；disable → disabled）----
	func() {
		name := "T4a stop 收尾（→ inactive）"
		code, raw := post(*apiBase + "/api/agents/" + *rootID + "/services/" + *unit + "/stop")
		if code != 200 {
			check(name, false, fmt.Sprintf("HTTP %d: %s", code, truncate(string(raw), 200)))
			return
		}
		_, units, _ := getList(*rootID)
		u := findUnit(units, *unit)
		ok := u != nil && u.ActiveState == "inactive"
		state := "?"
		if u != nil {
			state = u.ActiveState + "/" + u.SubState
		}
		check(name, ok, fmt.Sprintf("stop 200，现态=%s", state))
	}()
	func() {
		name := "T4b disable 收尾（→ disabled）"
		code, raw := post(*apiBase + "/api/agents/" + *rootID + "/services/" + *unit + "/disable")
		if code != 200 {
			check(name, false, fmt.Sprintf("HTTP %d: %s", code, truncate(string(raw), 200)))
			return
		}
		_, units, _ := getList(*rootID)
		u := findUnit(units, *unit)
		ok := u != nil && u.UnitFileState == "disabled"
		state := "?"
		if u != nil {
			state = u.UnitFileState
		}
		check(name, ok, fmt.Sprintf("disable 200，自启态=%s", state))
	}()

	ev("=== 汇总：PASS=%d FAIL=%d（%s） ===", passes, fails, time.Now().Format(time.RFC3339))
	if fails > 0 {
		os.Exit(1)
	}
}
