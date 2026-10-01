// 日志尾随与跨机联邦检索真机验收探针（logs-design.md M2/M3 真机项，
// acceptance-checklist「日志检索」节）。以面板用户身份走真实链路：
//
//	T0  前置：登录；三 agent 在线且 capability 口径正确
//	    （a1 logs / a2 无 logs——空 PATH 探测不命中 / a4 logs——shim 遮蔽）
//	T1  L141 journalctl 高频尾随：系统域 transient unit 预热 500 行 →
//	    follow tail=2000 回填 → 600 行/s 实时突发 6000 行 → 断言序列
//	    1..6500 严格递增不重不漏（client abort 收尾，server 补发 follow.stop）
//	T2  L142 docker 容器停止：alpine 循环输出容器 follow ≥3 行后
//	    docker stop → docker logs -f 退出 → eof reason=exited
//	T3  L143 10 分钟超时兜底：空闲源（预热后无任何输出）follow 常驻——
//	    journalctl -f 对已结束 unit 不自退（本机实测 exit=124），超时是
//	    唯一出口；T3 最先启动，等待窗与 T1/T2/T4 并行，探针总时长 ≈10min
//	T4  L144 跨机联邦检索：agents 指定扇出（a1 成功 / a4 失败降级 /
//	    a2 no-logs / ghost offline）+ 全量扇出（结果按 agentId 排序分组），
//	    单 agent 失败 HTTP 200 不整体报错
//
// 归因口径注明：设计原稿 skipped 三因 offline/no-logs/not-found，实现将
// not-found 合并为 offline（api_logs_search.go「缺席即离线」），ghost id
// 即「路径不存在」的现实验证形态。
//
// 证据落 .acceptance/logs/evidence/（probe.log + search-*.json）；FAIL → exit 1。
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	apiBase   = flag.String("api", "http://127.0.0.1:19992", "cockpit server 基址")
	adminUser = flag.String("user", "admin", "管理员用户名")
	adminPass = flag.String("pass", "e2e-strong-pass-1", "管理员口令")
	evDir     = flag.String("ev", "", "证据目录（默认 .acceptance/logs/evidence）")
	agentA1   = flag.String("a1", "logs-acc-a1", "全量 PATH agent（查询成功样本）")
	agentA2   = flag.String("a2", "logs-acc-a2", "空 PATH agent（no-logs 样本）")
	agentA4   = flag.String("a4", "logs-acc-a4", "journalctl 失败 shim agent（单机失败样本）")
	ghostID   = flag.String("ghost", "logs-acc-ghost", "从未注册的 agent id（offline 样本）")
	hfUnit    = flag.String("hf-unit", "cockpit-acc-hf", "T1 高频输出 systemd unit")
	idleUnit  = flag.String("idle-unit", "cockpit-acc-idle", "T3 空闲 systemd unit")
	dockerSrc = flag.String("docker-src", "cockpit-acc-log", "T2 docker 容器名")
	t3Wait    = flag.Duration("t3-wait", 780*time.Second, "T3 超时 eof 等待上限")
)

var (
	evFile *os.File
	evMu   sync.Mutex
	passes int
	fails  int
	token  string
	httpC  = &http.Client{Timeout: 15 * time.Second}
	// followC 无总超时——尾随流要挂 10 分钟（各阶段有自己的等待窗）
	followC = &http.Client{}
)

// 行为中性注入点（先例：guac/probe osExit/dockerExecCmd、生产码
// stdinPipeFn）——fatal 退出与真机 systemd-run/journalctl 命令面在单测
// 注入桩覆盖分支，默认值即原行为
var (
	osExit        = os.Exit
	busyRetryWait = 500 * time.Millisecond
	systemdRunCmd = func(unit, script string) ([]byte, error) {
		return exec.Command("sudo", "-n", "systemd-run",
			"--unit="+unit, "--collect", "/bin/bash", "-c", script).CombinedOutput()
	}
	journalCtlCmd = func(unit string) ([]byte, error) {
		return exec.Command("journalctl", "-u", unit, "--no-pager",
			"-q", "-n", "2000").CombinedOutput()
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
		rd = bytes.NewReader(b)
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

// agentHasLogsCapability /api/agents 为 DB 视图，capabilities 来自注册载荷
// （裸数组与 {agents:[..]} 包装两形态兼容）
func agentHasLogsCapability(id string) (bool, error) {
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
				if c.Type == "logs" {
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

// ---------- systemd 系统域 transient unit（免密 sudo）----------

// runUnit 系统域建 transient unit 跑一次性脚本（stdout 进 journal，
// --collect 退出即自清；同名 unit 上一次尚未 collect 时重试）
func runUnit(unit, script string) error {
	var last string
	for i := 0; i < 20; i++ {
		out, err := systemdRunCmd(unit, script)
		if err == nil {
			return nil
		}
		last = fmt.Sprintf("%v: %s", err, truncate(strings.TrimSpace(string(out)), 200))
		if strings.Contains(strings.ToLower(string(out)), "already exist") {
			time.Sleep(busyRetryWait)
			continue
		}
		return fmt.Errorf("systemd-run %s: %s", unit, last)
	}
	return fmt.Errorf("systemd-run %s: unit name busy — %s", unit, last)
}

// waitJournal 轮询本机 journal（普通用户可读）直到出现 marker
func waitJournal(unit, marker string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, _ := journalCtlCmd(unit)
		if strings.Contains(string(out), marker) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// ---------- NDJSON 尾随流 ----------

type followFrame struct {
	Data   string `json:"data"`
	EOF    bool   `json:"eof"`
	Reason string `json:"reason"`
}

// followStream 后台 goroutine 扫 NDJSON 帧入 chan（100k 帧缓冲足够 T1 全量）
type followStream struct {
	resp *http.Response
	ch   chan followFrame
}

// startFollow 发起尾随（响应头返回 = agent follow.start 已应答）；headerWait
// 独立于流读取——CallAgent 起随是同步往返，卡死也要能报错
func startFollow(agentID string, body map[string]interface{}, headerWait time.Duration) (*followStream, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost,
		*apiBase+"/api/agents/"+agentID+"/logs/follow", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	type res struct {
		resp *http.Response
		err  error
	}
	ch := make(chan res, 1)
	go func() { r, e := followC.Do(req); ch <- res{r, e} }()
	var resp *http.Response
	select {
	case c := <-ch:
		if c.err != nil {
			return nil, c.err
		}
		resp = c.resp
	case <-time.After(headerWait):
		return nil, fmt.Errorf("follow start 无响应（>%s）", headerWait)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	fs := &followStream{resp: resp, ch: make(chan followFrame, 100000)}
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
		for sc.Scan() {
			line := sc.Bytes()
			if len(line) == 0 {
				continue
			}
			var f followFrame
			if json.Unmarshal(line, &f) != nil {
				continue
			}
			fs.ch <- f
		}
	}()
	return fs, nil
}

// read 读一帧（timeout 内无帧返回 !ok）
func (fs *followStream) read(timeout time.Duration) (followFrame, bool) {
	select {
	case f := <-fs.ch:
		return f, true
	case <-time.After(timeout):
		return followFrame{}, false
	}
}

// abort 客户端断开：server 感知 r.Context().Done() 补发 logs.follow.stop
func (fs *followStream) abort() { _ = fs.resp.Body.Close() }

// ---------- 序列分析 ----------

var (
	hfRe  = regexp.MustCompile(`HF-(\d+)`)
	docRe = regexp.MustCompile(`DOCLINE-(\d+)`)
)

// appendNums 提取行内序号；lo 之下丢弃——T1 序号带每轮唯一基址
// （时间戳 ×10 万），前几轮探针留在同一 unit journal 里的同名旧行天然出局
// （传输层本无重帧，旧帧是复跑累积的口径噪声）
func appendNums(nums *[]int, re *regexp.Regexp, data string, lo int) {
	if m := re.FindStringSubmatch(data); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= lo {
			*nums = append(*nums, n)
		}
	}
}

// analyzeSeq 有序性/重复/缺口计数（gaps 仅在有序无重复时有意义，否则 -1）
func analyzeSeq(nums []int) (dups, gaps int, ordered bool) {
	ordered = true
	seen := map[int]bool{}
	for i, n := range nums {
		if seen[n] {
			dups++
		}
		seen[n] = true
		if i > 0 && nums[i-1] >= n {
			ordered = false
		}
	}
	gaps = -1
	if ordered && dups == 0 && len(nums) > 0 {
		gaps = nums[len(nums)-1] - nums[0] + 1 - len(nums)
	}
	return dups, gaps, ordered
}

func containsNum(nums []int, v int) bool {
	for _, n := range nums {
		if n == v {
			return true
		}
	}
	return false
}

// ---------- T4 联邦检索 ----------

type searchResult struct {
	AgentID   string `json:"agentId"`
	Hostname  string `json:"hostname"`
	OK        bool   `json:"ok"`
	Truncated bool   `json:"truncated"`
	Lines     string `json:"lines"`
	Error     string `json:"error"`
}

type searchResponse struct {
	Results []searchResult `json:"results"`
	Skipped []struct {
		AgentID string `json:"agentId"`
		Reason  string `json:"reason"`
	} `json:"skipped"`
}

func runSearch(name string, body map[string]interface{}) (*searchResponse, []byte, error) {
	b, _ := json.Marshal(body)
	_ = os.WriteFile(filepath.Join(*evDir, "search-"+name+".json"), b, 0o644)
	code, raw := reqJSON(http.MethodPost, *apiBase+"/api/logs/search", body)
	if code != http.StatusOK {
		return nil, raw, fmt.Errorf("HTTP %d: %s", code, truncate(string(raw), 200))
	}
	var resp searchResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, raw, fmt.Errorf("bad response: %v", err)
	}
	_ = os.WriteFile(filepath.Join(*evDir, "search-"+name+"-resp.json"), raw, 0o644)
	return &resp, raw, nil
}

// skippedReason 找 (agentId, reason) 是否在 skipped 里
func (r *searchResponse) skippedReason(agentID string) (string, bool) {
	for _, s := range r.Skipped {
		if s.AgentID == agentID {
			return s.Reason, true
		}
	}
	return "", false
}

// ---------- 场景 ----------

func main() {
	flag.Parse()
	if _, err := os.Stat("go.mod"); err != nil {
		fatal("请在仓库根目录运行（找不到 go.mod）")
	}
	repo, _ := os.Getwd()
	if *evDir == "" {
		*evDir = filepath.Join(repo, ".acceptance/logs/evidence")
	}
	_ = os.MkdirAll(*evDir, 0o755)
	f, err := os.Create(filepath.Join(*evDir, "probe.log"))
	if err != nil {
		fatal("证据文件创建失败: %v", err)
	}
	evFile = f
	defer evFile.Close()

	ev("=== 日志尾随与联邦检索真机验收 %s ===", time.Now().Format(time.RFC3339))
	ev("api=%s hf=%s idle=%s docker=%s t3-wait=%s", *apiBase, *hfUnit, *idleUnit, *dockerSrc, *t3Wait)

	// T1 序号基址：每轮唯一（同一 unit 的 journal 跨轮累积，旧行不得混入
	// 本轮序列断言），base+n 保证旧帧序号永远小于本轮下界
	seqBase := int(time.Now().Unix()) * 100000

	login()
	ev("登录成功（%s）", *adminUser)

	// ---- T0 前置：三 agent capability 口径 ----
	t0OK := func() bool {
		c1, err1 := agentHasLogsCapability(*agentA1)
		c2, err2 := agentHasLogsCapability(*agentA2)
		c4, err4 := agentHasLogsCapability(*agentA4)
		if err1 != nil || err2 != nil || err4 != nil {
			check("T0 agent capability 口径", false,
				fmt.Sprintf("a1=%v(%v) a2=%v(%v) a4=%v(%v)", c1, err1, c2, err2, c4, err4))
			return false
		}
		ok := c1 && !c2 && c4
		check("T0 agent capability 口径（a1 logs / a2 无 / a4 logs）", ok,
			fmt.Sprintf("a1.logs=%v a2.logs=%v(空 PATH 探测不命中) a4.logs=%v(shim 遮蔽仍命中)",
				c1, c2, c4))
		return ok
	}()
	if !t0OK {
		fatal("capability 口径不符，联邦检索归因断言将全部失真——先查 run-server 三 agent 日志")
	}

	// ---- T3 先启动（10 分钟计时器最早开跑，等待窗与后续场景并行）----
	var t3 *followStream
	var t3Start time.Time
	var t3Backfill int
	t3Ready := func() bool {
		if err := runUnit(*idleUnit, "echo IDLE-PREWARM"); err != nil {
			check("T3 空闲源预热", false, err.Error())
			return false
		}
		if !waitJournal(*idleUnit, "IDLE-PREWARM", 10*time.Second) {
			check("T3 空闲源预热", false, "journal 未见 IDLE-PREWARM")
			return false
		}
		fs, err := startFollow(*agentA1, map[string]interface{}{
			"type": "systemd", "source": *idleUnit, "tail": 5,
		}, 30*time.Second)
		if err != nil {
			check("T3 空闲源 follow 启动", false, err.Error())
			return false
		}
		t3, t3Start = fs, time.Now()
		// 沉淀期：吃掉 tail 回填（含前几轮 prewarm 的旧行——journal 跨轮
		// 累积，数量不定），连续 2s 无帧视为回填完毕；之后的 10min 等待窗
		// 内必须完全静默，静默才证明「空闲不自退、超时是唯一出口」
		for miss := 0; miss < 2; {
			if f, ok := t3.read(2 * time.Second); ok {
				if !f.EOF {
					t3Backfill++
					miss = 0
				}
				continue
			}
			miss++
		}
		ev("T3 尾随已启动（%s @ %s），回填 %d 帧后静默——唯一出口是 10min 超时",
			*idleUnit, t3Start.Format("15:04:05"), t3Backfill)
		return true
	}()
	if t3Ready {
		defer t3.abort()
	}

	// ---- T1 journalctl 高频尾随不重不漏 ----
	func() {
		name := "T1 journalctl 高频尾随不重不漏（回填 + 600 行/s × 10s）"
		if err := runUnit(*hfUnit, fmt.Sprintf(
			`for i in $(seq 1 500); do echo "HF-$((%d+i))"; done`, seqBase)); err != nil {
			check(name, false, "预热 runA: "+err.Error())
			return
		}
		if !waitJournal(*hfUnit, fmt.Sprintf("HF-%d", seqBase+500), 15*time.Second) {
			check(name, false, "journal 未见本轮预热末行（预热未完成）")
			return
		}
		fs, err := startFollow(*agentA1, map[string]interface{}{
			"type": "systemd", "source": *hfUnit, "tail": 2000,
		}, 30*time.Second)
		if err != nil {
			check(name, false, "follow: "+err.Error())
			return
		}
		defer fs.abort()

		// 阶段1：tail=2000 回填本轮 base+1..base+500（前几轮旧行被 lo 过滤）
		var nums []int
		p1 := time.Now().Add(25 * time.Second)
		for !containsNum(nums, seqBase+500) && time.Now().Before(p1) {
			f, ok := fs.read(3 * time.Second)
			if !ok {
				break
			}
			if f.EOF {
				check(name, false, fmt.Sprintf("回填阶段意外 eof（reason=%q）", f.Reason))
				return
			}
			appendNums(&nums, hfRe, f.Data, seqBase)
		}
		backfill := containsNum(nums, seqBase+500) && len(nums) >= 500
		ev("      回填：收到 HF-%d 帧（含本轮 500 行=%v）", len(nums), backfill)

		// 阶段2：实时突发 base+501..base+6500（60 行/100ms ≈ 600 行/s，
		// 突发块 60 < server 256 帧缓冲）
		liveStart := time.Now()
		if err := runUnit(*hfUnit, fmt.Sprintf(
			`i=%d; while [ "$i" -lt %d ]; do for j in $(seq 1 60); do i=$((i+1)); [ "$i" -gt %d ] && break; echo "HF-$i"; done; sleep 0.1; done`,
			seqBase+500, seqBase+6500, seqBase+6500)); err != nil {
			check(name, false, "突发 runB: "+err.Error())
			return
		}
		p2 := time.Now().Add(90 * time.Second)
		for !containsNum(nums, seqBase+6500) && time.Now().Before(p2) {
			f, ok := fs.read(5 * time.Second)
			if !ok {
				break
			}
			if f.EOF {
				ev("      [WARN] 实时阶段 eof（reason=%q）", f.Reason)
				break
			}
			appendNums(&nums, hfRe, f.Data, seqBase)
		}
		liveDur := time.Since(liveStart)

		dups, gaps, ordered := analyzeSeq(nums)
		var first, last int
		if len(nums) > 0 {
			first, last = nums[0], nums[len(nums)-1]
		}
		ok := backfill && len(nums) == 6500 && ordered && dups == 0 &&
			first == seqBase+1 && last == seqBase+6500
		rate := float64(len(nums)-500) / liveDur.Seconds()
		check(name, ok, fmt.Sprintf(
			"帧=%d 序列=%d..%d（base=%d）有序=%v 重复=%d 缺口=%d 实时速率=%.0f 行/s（60 行/100ms 突发 ×10s）",
			len(nums), first, last, seqBase, ordered, dups, gaps, rate))
	}()

	// ---- T2 docker 容器停止 → eof reason=exited ----
	func() {
		name := "T2 docker 容器停止后尾随 reason=exited 正常终止"
		if out, err := exec.Command("docker", "rm", "-f", *dockerSrc).CombinedOutput(); err != nil {
			ev("      [WARN] 清理旧容器: %v: %s", err, truncate(string(out), 80))
		}
		if out, err := exec.Command("docker", "run", "-d", "--name", *dockerSrc,
			"alpine:latest", "sh", "-c",
			`i=0; while true; do i=$((i+1)); echo DOCLINE-$i; sleep 1; done`).CombinedOutput(); err != nil {
			check(name, false, "docker run: "+truncate(string(out), 200))
			return
		}
		fs, err := startFollow(*agentA1, map[string]interface{}{
			"type": "docker", "source": *dockerSrc, "tail": 20,
		}, 30*time.Second)
		if err != nil {
			check(name, false, "follow: "+err.Error())
			return
		}
		defer fs.abort()

		// 收 ≥3 行再停（回填 + 每秒 1 行实时）
		var nums []int
		p1 := time.Now().Add(30 * time.Second)
		for len(nums) < 3 && time.Now().Before(p1) {
			f, ok := fs.read(3 * time.Second)
			if !ok {
				break
			}
			if f.EOF {
				check(name, false, fmt.Sprintf("停止前意外 eof（reason=%q）", f.Reason))
				return
			}
			appendNums(&nums, docRe, f.Data, 0)
		}
		if len(nums) < 3 {
			check(name, false, fmt.Sprintf("仅收到 %d 行 DOCLINE（不足 3）", len(nums)))
			return
		}
		if out, err := exec.Command("docker", "stop", "-t", "2", *dockerSrc).CombinedOutput(); err != nil {
			check(name, false, "docker stop: "+truncate(string(out), 200))
			return
		}
		// docker logs -f 随容器退出 → agent closeFn("exited") → eof 帧
		eofReason, gotEOF := "", false
		p2 := time.Now().Add(30 * time.Second)
		for !gotEOF && time.Now().Before(p2) {
			f, ok := fs.read(5 * time.Second)
			if !ok {
				continue
			}
			if f.EOF {
				eofReason, gotEOF = f.Reason, true
				break
			}
			appendNums(&nums, docRe, f.Data, 0)
		}
		dups, _, ordered := analyzeSeq(nums)
		check(name, gotEOF && eofReason == "exited" && ordered && dups == 0,
			fmt.Sprintf("eof=%v reason=%q（want exited）帧=%d 有序=%v 重复=%d",
				gotEOF, eofReason, len(nums), ordered, dups))
	}()

	// ---- T4 跨机联邦检索 ----
	func() {
		name1 := "T4a agents 指定扇出（成功/失败降级/no-logs/offline 四样本齐）"
		resp, _, err := runSearch("filtered", map[string]interface{}{
			"type": "systemd", "source": *hfUnit, "tail": 50,
			"agents": []string{*agentA1, *agentA2, *agentA4, *ghostID},
		})
		if err != nil {
			check(name1, false, err.Error())
			return
		}
		if len(resp.Results) != 2 {
			check(name1, false, fmt.Sprintf("results=%d（want 2：a1 成功 + a4 失败降级），skipped=%v",
				len(resp.Results), resp.Skipped))
			return
		}
		r1, r4 := resp.Results[0], resp.Results[1]
		sorted := r1.AgentID == *agentA1 && r4.AgentID == *agentA4
		r1OK := r1.AgentID == *agentA1 && r1.OK && strings.Contains(r1.Lines, "HF-")
		r4Fail := r4.AgentID == *agentA4 && !r4.OK && r4.Error != ""
		skA2, a2Skipped := resp.skippedReason(*agentA2)
		skGhost, ghostSkipped := resp.skippedReason(*ghostID)
		grouped := r1.Hostname != "" && r4.Hostname != ""
		ev("      a1: ok=%v hostname=%q lines前缀=%q", r1.OK, r1.Hostname, truncate(r1.Lines, 60))
		ev("      a4: ok=%v error=%q（单机失败，HTTP 200 不整体报错）", r4.OK, truncate(r4.Error, 80))
		ev("      skipped: a2=%q ghost=%q（not-found 已合并 offline 口径）", skA2, skGhost)
		check(name1, sorted && r1OK && r4Fail && a2Skipped && skA2 == "no-logs" &&
			ghostSkipped && skGhost == "offline" && grouped,
			fmt.Sprintf("results=[%s,%s]（按 agentId 排序）a1.ok=%v a4.ok=%v skipped(a2)=%q skipped(ghost)=%q",
				r1.AgentID, r4.AgentID, r1.OK, r4.OK, skA2, skGhost))

		name2 := "T4b 全量扇出（在线+logs 为目标集，no-logs 入 skipped）"
		resp2, _, err := runSearch("all", map[string]interface{}{
			"type": "systemd", "source": *hfUnit, "tail": 50,
		})
		if err != nil {
			check(name2, false, err.Error())
			return
		}
		var a1r, a4r *searchResult
		for i := range resp2.Results {
			switch resp2.Results[i].AgentID {
			case *agentA1:
				a1r = &resp2.Results[i]
			case *agentA4:
				a4r = &resp2.Results[i]
			}
		}
		skA2b, a2SkippedB := resp2.skippedReason(*agentA2)
		// 全量模式 skipped 只列在线但无 capability 的（离线 agent 不在注册表，天然缺席）
		sorted2 := len(resp2.Results) == 2 && a1r != nil && a4r != nil &&
			resp2.Results[0].AgentID < resp2.Results[1].AgentID
		ok2 := sorted2 && a1r != nil && a1r.OK && a4r != nil && !a4r.OK &&
			a2SkippedB && skA2b == "no-logs" && len(resp2.Skipped) == 1
		ev("      全量 results=%d（a1 ok=%v / a4 ok=%v）skipped=%v（离线者缺席不列）",
			len(resp2.Results), a1r != nil && a1r.OK, a4r != nil && a4r.OK, resp2.Skipped)
		check(name2, ok2, fmt.Sprintf("results=%d 按 agentId 排序=%v a1.ok=%v a4.ok=%v skipped(a2)=%q",
			len(resp2.Results), sorted2, a1r != nil && a1r.OK, a4r != nil && a4r.OK, skA2b))
	}()

	// ---- T3 收官：等待 10min 超时 eof ----
	if t3Ready {
		name := "T3 10 分钟超时兜底（空闲源 follow 到点 eof reason=exited）"
		dataAfter, gotEOF, eofReason := 0, false, ""
		deadline := t3Start.Add(*t3Wait)
		for !gotEOF && time.Now().Before(deadline) {
			f, ok := t3.read(30 * time.Second)
			if !ok {
				continue
			}
			if f.EOF {
				eofReason, gotEOF = f.Reason, true
				break
			}
			dataAfter++
		}
		elapsed := time.Since(t3Start)
		// 期望 ~600s（10min 超时 + kill/EOF/传输余量）；沉淀期后到 eof 前必须
		// 完全静默（dataAfter==0 证明空闲源不自退、超时是唯一出口）
		check(name, gotEOF && eofReason == "exited" && dataAfter == 0 &&
			elapsed >= 540*time.Second && elapsed <= 700*time.Second,
			fmt.Sprintf("eof=%v reason=%q 耗时=%s 回填=%d 帧 沉淀后新增=%d（want 0）",
				gotEOF, eofReason, elapsed.Round(time.Second), t3Backfill, dataAfter))
	} else {
		check("T3 10 分钟超时兜底", false, "前置失败（空闲源预热/follow 启动未成）")
	}

	ev("=== 汇总：PASS=%d FAIL=%d（%s） ===", passes, fails, time.Now().Format(time.RFC3339))
	if fails > 0 {
		os.Exit(1)
	}
}
