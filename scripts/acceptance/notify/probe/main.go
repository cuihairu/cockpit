// 通知渠道真机验收探针（acceptance-checklist「通知渠道」行：至少配置一个
// 渠道，用「测试通知」按钮核对送达）。以面板用户身份走真实链路（REST +
// 接收器实收 JSONL + 审计面）：
//
//	N0  状态面：GET /api/notification/status → enabled=true、两渠道在列
//	    （webhook@127.0.0.1:9700 活样本 / @127.0.0.1:9799 死端口样本）
//	N1  测试通知：POST /api/notification/test（TestAll 绕过事件白名单）
//	    → 200；逐渠道结果 2 条：9700 ok=true；9799 ok=false 且 error 非空
//	    （投递失败逐渠道呈现，不 500 不掩盖）；响应不含渠道 secret
//	    （凭据不出结果面）
//	N2  接收器实收：JSONL 恰 1 条，secret_ok=true，body 含
//	    event_type=test / title「Cockpit 测试通知」/ level=info
//	    （送达内容与载荷契约一致；死端口渠道不应产生收包）
//	N3  审计：test 落 resource=notification / action=test /
//	    resource_id=channels 恰一条，details 含 "sent":1 与 "failed":1
//	N4  方法面：GET test=405、无 token status=401、未知子路径=404
//
// ntfy/telegram/herald 云端渠道不在本机验证面内（行内注明阻塞项）。
// 证据落 .acceptance/notify/evidence/（probe.log + status.json +
// test.json + audit.json）；FAIL → exit 1。
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
	apiBase   = flag.String("api", "http://127.0.0.1:19997", "cockpit server 基址")
	adminUser = flag.String("user", "admin", "管理员用户名")
	adminPass = flag.String("pass", "e2e-strong-pass-1", "管理员口令")
	evDir     = flag.String("ev", "", "证据目录（默认 .acceptance/notify/evidence）")
	hookLog   = flag.String("hooks", ".acceptance/notify/evidence/webhooks.jsonl", "接收器收包 JSONL")
	liveTgt   = flag.String("live-target", "127.0.0.1:9700", "活渠道 target（接收器）")
	deadTgt   = flag.String("dead-target", "127.0.0.1:9799", "死渠道 target（失败样本）")
	secret    = flag.String("secret", "notify-accept-secret", "活渠道 X-Cockpit-Secret（泄露断言用）")
)

var (
	evFile = (*os.File)(nil)
	evMu   sync.Mutex
	passes int
	fails  int
	token  string
	httpC  = &http.Client{Timeout: 30 * time.Second}
)

// osExit 注入点：fatal 退出在单测经 stubExit 覆盖，默认值即原行为
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

// loginAs 登录取 token
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

// ---------- 状态 / 测试结果解析 ----------

// statusView GET /api/notification/status 响应（渠道摘要不含凭据）
type statusView struct {
	Enabled  bool           `json:"enabled"`
	Channels []channelEntry `json:"channels"`
}

type channelEntry struct {
	Channel string `json:"channel"`
	Target  string `json:"target"`
}

func parseStatus(raw []byte) (*statusView, error) {
	var v statusView
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("bad status: %s", truncate(string(raw), 200))
	}
	return &v, nil
}

// channelOn 列表中是否存在 channel@target（target 子串匹配）
func channelOn(v *statusView, channel, targetSub string) bool {
	for _, c := range v.Channels {
		if c.Channel == channel && strings.Contains(c.Target, targetSub) {
			return true
		}
	}
	return false
}

// sendResult POST /api/notification/test 逐渠道结果
type sendResult struct {
	Channel string `json:"channel"`
	Target  string `json:"target"`
	OK      bool   `json:"ok"`
	Error   string `json:"error"`
}

func parseTestResults(raw []byte) ([]sendResult, error) {
	var r struct {
		Results []sendResult `json:"results"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("bad test results: %s", truncate(string(raw), 200))
	}
	return r.Results, nil
}

// resultForTarget 按 target 子串取单渠道结果
func resultForTarget(results []sendResult, targetSub string) *sendResult {
	for i := range results {
		if strings.Contains(results[i].Target, targetSub) {
			return &results[i]
		}
	}
	return nil
}

// ---------- 接收器收包解析 ----------

// hookEntry webhook_receiver.py JSONL 单条（时间戳/鉴权结果/正文）
type hookEntry struct {
	TS       string `json:"ts"`
	Path     string `json:"path"`
	SecretOK bool   `json:"secret_ok"`
	Body     string `json:"body"`
}

// readJSONL 逐行解析收包记录（空文件 = 零收包，非错误）
func readJSONL(path string) ([]hookEntry, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []hookEntry
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e hookEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("bad jsonl line %q: %v", truncate(line, 80), err)
		}
		out = append(out, e)
	}
	return out, nil
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

// countAudit 满足 action+resource_id 的审计条目
func countAudit(entries []auditEntry, action, resourceID string) []auditEntry {
	var out []auditEntry
	for _, e := range entries {
		if e.Action == action && e.ResourceID == resourceID {
			out = append(out, e)
		}
	}
	return out
}

// ---------- 场景编排 ----------

func main() {
	flag.Parse()
	if *evDir == "" {
		*evDir = ".acceptance/notify/evidence"
	}
	if err := os.MkdirAll(*evDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "evidence dir:", err)
		osExit(2)
	}
	f, err := os.OpenFile(filepath.Join(*evDir, "probe.log"),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe.log:", err)
		osExit(2)
	}
	evFile = f
	defer f.Close()

	start := time.Now()
	ev("== 通知渠道真机验收 %s ==", start.Format(time.RFC3339))

	tk, err := loginAs(*adminUser, *adminPass)
	if err != nil {
		fatal("登录失败: %v", err)
	}
	evMu.Lock()
	token = tk
	evMu.Unlock()

	// ---------- N0 状态面 ----------
	code, raw := reqJSON(http.MethodGet, *apiBase+"/api/notification/status", nil)
	if code != 200 {
		fatal("N0 status: HTTP %d %s", code, truncate(string(raw), 200))
	}
	saveEV("status.json", raw)
	st, err := parseStatus(raw)
	if err != nil {
		fatal("N0 status 解析: %v", err)
	}
	check("N0 通知服务 enabled", st.Enabled, fmt.Sprintf("enabled=%v channels=%d", st.Enabled, len(st.Channels)))
	check("N0 活渠道在列（webhook@9700）", channelOn(st, "webhook", *liveTgt),
		fmt.Sprintf("channels=%v", st.Channels))
	check("N0 死渠道在列（webhook@9799）", channelOn(st, "webhook", *deadTgt), "")

	// ---------- N1 测试通知（「测试通知」按钮同链路） ----------
	code, raw = reqJSON(http.MethodPost, *apiBase+"/api/notification/test", nil)
	if code != 200 {
		fatal("N1 test: HTTP %d %s", code, truncate(string(raw), 200))
	}
	saveEV("test.json", raw)
	results, err := parseTestResults(raw)
	if err != nil {
		fatal("N1 test 解析: %v", err)
	}
	live := resultForTarget(results, *liveTgt)
	dead := resultForTarget(results, *deadTgt)
	check("N1 逐渠道结果恰 2 条", len(results) == 2,
		fmt.Sprintf("results=%v", results))
	check("N1 活渠道送达 ok", live != nil && live.OK,
		fmt.Sprintf("live=%+v", live))
	check("N1 死渠道失败呈现（ok=false 且 error 非空，不 500 不掩盖）",
		dead != nil && !dead.OK && dead.Error != "",
		fmt.Sprintf("dead=%+v", dead))
	check("N1 响应不含渠道 secret", !strings.Contains(string(raw), *secret),
		"凭据不出结果面")

	// ---------- N2 接收器实收 ----------
	hooks, err := readJSONL(*hookLog)
	if err != nil {
		fatal("N2 收包读取: %v", err)
	}
	check("N2 收包恰 1 条（死端口渠道不产生收包）", len(hooks) == 1,
		fmt.Sprintf("entries=%d", len(hooks)))
	if len(hooks) == 1 {
		h := hooks[0]
		check("N2 secret 鉴权通过（X-Cockpit-Secret）", h.SecretOK && h.Path == "/hook",
			fmt.Sprintf("path=%s secret_ok=%v", h.Path, h.SecretOK))
		check("N2 载荷契约（event_type=test / title / level=info）",
			strings.Contains(h.Body, `"event_type":"test"`) &&
				strings.Contains(h.Body, "Cockpit 测试通知") &&
				strings.Contains(h.Body, `"level":"info"`),
			truncate(h.Body, 160))
	}

	// ---------- N3 审计 ----------
	code, raw = reqJSON(http.MethodGet, *apiBase+"/api/admin/audit/logs?resource=notification&page_size=20", nil)
	if code != 200 {
		fatal("N3 审计查询: HTTP %d", code)
	}
	saveEV("audit.json", raw)
	entries, err := parseAuditEntries(raw)
	if err != nil {
		fatal("N3 审计解析: %v", err)
	}
	tests := countAudit(entries, "test", "channels")
	check("N3 test 审计恰一条（notification/channels）",
		len(tests) == 1 && tests[0].Username == *adminUser,
		fmt.Sprintf("命中 %d 条", len(tests)))
	check("N3 details 计数（sent=1 failed=1）",
		len(tests) == 1 && strings.Contains(tests[0].Details, `"sent":1`) &&
			strings.Contains(tests[0].Details, `"failed":1`),
		truncate(tests[0].Details, 160))

	// ---------- N4 方法面 ----------
	code, _ = reqJSON(http.MethodGet, *apiBase+"/api/notification/test", nil)
	check("N4 GET test=405", code == 405, fmt.Sprintf("HTTP %d", code))
	code, _ = reqJSON(http.MethodGet, *apiBase+"/api/notification/nope", nil)
	check("N4 未知子路径=404", code == 404, fmt.Sprintf("HTTP %d", code))
	code, _ = func() (int, []byte) {
		evMu.Lock()
		saved := token
		token = ""
		evMu.Unlock()
		defer func() {
			evMu.Lock()
			token = saved
			evMu.Unlock()
		}()
		return reqJSON(http.MethodGet, *apiBase+"/api/notification/status", nil)
	}()
	check("N4 无 token status=401", code == 401, fmt.Sprintf("HTTP %d", code))

	ev("== 汇总：PASS %d / FAIL %d / 用时 %.1fs ==", passes, fails, time.Since(start).Seconds())
	if f := evFile; f != nil {
		_ = f.Sync()
	}
	if fails > 0 {
		osExit(1)
	}
}
