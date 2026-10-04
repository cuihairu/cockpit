// 通知渠道探针 helpers 单测（覆盖率收口：helpers 全覆盖）。
//
// 场景编排 main()（N0-N4 真机 server/webhook 接收器/审计面）按验收惯例走
// 真机探针 + .acceptance 证据，不在此单测面内（同 guac/probe、jobs/probe
// 先例）。本文件覆盖全部可确定性验证的逻辑：证据层 / REST 层（httptest 假
// cockpit）/ 状态与逐渠道结果解析 / 收包 JSONL 解析 / 审计解析。
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- 测试基建 ----------

// resetGlobals 每用例隔离包级状态（flag 变量还原为默认语义）
func resetGlobals(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		evMu.Lock()
		token, fails, passes, evFile = "", 0, 0, nil
		evMu.Unlock()
		*apiBase = "http://127.0.0.1:19997"
		*evDir = ""
		*hookLog = ".acceptance/notify/evidence/webhooks.jsonl"
		osExit = os.Exit
	})
	osExit = os.Exit
}

// stubExit 记录 fatal 的退出码而不真退出
func stubExit(t *testing.T) *int {
	t.Helper()
	code := new(int)
	osExit = func(c int) { *code = c }
	return code
}

// fakeAPI 假 cockpit：登录出 token，其余路径可注册
func fakeAPI(t *testing.T, h func(mux *http.ServeMux)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "tk-nt-1"})
	})
	if h != nil {
		h(mux)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	*apiBase = srv.URL
	return srv
}

// ---------- 证据层 ----------

func TestEvCheckFatalTruncateSave(t *testing.T) {
	resetGlobals(t)
	f, err := os.CreateTemp(t.TempDir(), "ev")
	if err != nil {
		t.Fatal(err)
	}
	evMu.Lock()
	evFile = f
	evMu.Unlock()

	ev("line %d", 1)
	check("pass-case", true, "ok")
	check("fail-case", false, "bad")
	if passes != 1 || fails != 1 {
		t.Fatalf("counters: passes=%d fails=%d, want 1/1", passes, fails)
	}
	body, _ := os.ReadFile(f.Name())
	for _, want := range []string{"line 1", "[PASS] pass-case", "[FAIL] fail-case"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("evidence log missing %q:\n%s", want, body)
		}
	}

	code := stubExit(t)
	fatal("boom %d", 42)
	if *code != 2 {
		t.Fatalf("fatal exit code = %d, want 2", *code)
	}

	if got := truncate("abcdef", 10); got != "abcdef" {
		t.Fatalf("truncate short = %q", got)
	}
	if got := truncate("abcdef", 3); got != "abc..." {
		t.Fatalf("truncate long = %q", got)
	}

	*evDir = t.TempDir()
	saveEV("sample.json", []byte(`{"k":1}`))
	if b, err := os.ReadFile(filepath.Join(*evDir, "sample.json")); err != nil || string(b) != `{"k":1}` {
		t.Fatalf("saveEV: %v %q", err, b)
	}
	*evDir = ""
	saveEV("skip.json", []byte("x")) // 不 panic 即可
}

// ---------- REST 层 ----------

func TestReqAndLogin(t *testing.T) {
	resetGlobals(t)

	var gotAuth string
	srv := fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/notification/status", func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"enabled":true,"channels":[]}`))
		})
	})

	code, raw := reqJSON(http.MethodGet, srv.URL+"/api/notification/status", nil)
	if code != 200 || gotAuth != "" {
		t.Fatalf("req no-token: %d auth=%q", code, gotAuth)
	}

	tk, err := loginAs("admin", "pw")
	if err != nil || tk != "tk-nt-1" {
		t.Fatalf("loginAs: %q %v", tk, err)
	}
	evMu.Lock()
	token = tk
	evMu.Unlock()
	code, raw = reqJSON(http.MethodGet, srv.URL+"/api/notification/status", nil)
	if code != 200 || gotAuth != "Bearer tk-nt-1" || !strings.Contains(string(raw), `"enabled":true`) {
		t.Fatalf("req with token: %d auth=%q raw=%s", code, gotAuth, raw)
	}

	// 非 200 登录端点（apiBase 指向不存在路径 → 404）
	savedBase := *apiBase
	*apiBase = savedBase + "/nope"
	if _, err := loginAs("admin", "pw"); err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("non-200 err = %v", err)
	}
	*apiBase = savedBase

	code, _ = reqJSON(http.MethodGet, "http://127.0.0.1:1/nope", nil)
	if code != -1 {
		t.Fatalf("conn error code = %d, want -1", code)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer bad.Close()
	*apiBase = bad.URL
	if _, err := loginAs("admin", "pw"); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("bad-json err = %v", err)
	}
}

// ---------- 状态 / 测试结果解析 ----------

func TestParseStatusAndChannelOn(t *testing.T) {
	resetGlobals(t)
	st, err := parseStatus([]byte(
		`{"enabled":true,"channels":[{"channel":"webhook","target":"127.0.0.1:9700"},{"channel":"ntfy","target":"127.0.0.1:9799"}]}`))
	if err != nil || !st.Enabled || len(st.Channels) != 2 {
		t.Fatalf("parseStatus: %+v %v", st, err)
	}
	if !channelOn(st, "webhook", "127.0.0.1:9700") {
		t.Fatal("channelOn live: want true")
	}
	if channelOn(st, "webhook", "127.0.0.1:9799") {
		t.Fatal("channelOn wrong channel: want false")
	}
	if channelOn(st, "telegram", "x") {
		t.Fatal("channelOn missing: want false")
	}
	if _, err := parseStatus([]byte("not-json")); err == nil {
		t.Fatal("parseStatus bad json: want error")
	}
}

func TestParseTestResults(t *testing.T) {
	resetGlobals(t)
	results, err := parseTestResults([]byte(
		`{"results":[{"channel":"webhook","target":"127.0.0.1:9700","ok":true},
		            {"channel":"webhook","target":"127.0.0.1:9799","ok":false,"error":"send request: conn refused"}]}`))
	if err != nil || len(results) != 2 {
		t.Fatalf("parseTestResults: %v len=%d", err, len(results))
	}
	live := resultForTarget(results, "127.0.0.1:9700")
	if live == nil || !live.OK {
		t.Fatalf("resultForTarget live: %+v", live)
	}
	dead := resultForTarget(results, "127.0.0.1:9799")
	if dead == nil || dead.OK || dead.Error == "" {
		t.Fatalf("resultForTarget dead: %+v", dead)
	}
	if resultForTarget(results, "10.0.0.1") != nil {
		t.Fatal("resultForTarget miss: want nil")
	}
	if _, err := parseTestResults([]byte("not-json")); err == nil {
		t.Fatal("parseTestResults bad json: want error")
	}
}

// ---------- 收包 JSONL 解析 ----------

func TestReadJSONL(t *testing.T) {
	resetGlobals(t)
	// 文件不存在 = 零收包
	hooks, err := readJSONL(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err != nil || len(hooks) != 0 {
		t.Fatalf("readJSONL missing: %v len=%d", err, len(hooks))
	}

	p := filepath.Join(t.TempDir(), "hooks.jsonl")
	line1 := `{"ts":"05/Oct/2026:02:00:00","path":"/hook","secret_ok":true,"body":"{\"event_type\":\"test\",\"title\":\"Cockpit 测试通知\",\"level\":\"info\"}"}`
	// 中段留一个空行 → 覆盖 continue 分支（首尾空白会被 TrimSpace 吃掉）
	if err := os.WriteFile(p, []byte(line1+"\n\n"+line1+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hooks, err = readJSONL(p)
	if err != nil || len(hooks) != 2 {
		t.Fatalf("readJSONL: %v len=%d", err, len(hooks))
	}
	h := hooks[0]
	if !h.SecretOK || h.Path != "/hook" ||
		!strings.Contains(h.Body, `"event_type":"test"`) ||
		!strings.Contains(h.Body, "Cockpit 测试通知") ||
		!strings.Contains(h.Body, `"level":"info"`) {
		t.Fatalf("hook entry: %+v", h)
	}

	// 坏行 → 解析错误
	if err := os.WriteFile(p, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readJSONL(p); err == nil {
		t.Fatal("readJSONL bad line: want error")
	}

	// 非 NotExist 的读错误（目录当文件读）→ 透传
	if _, err := readJSONL(t.TempDir()); err == nil {
		t.Fatal("readJSONL dir path: want error")
	}
}

// ---------- 审计解析 ----------

func TestParseAndCountAudit(t *testing.T) {
	resetGlobals(t)
	raw := []byte(`{"data":[
		{"username":"admin","action":"test","resource":"notification","resource_id":"channels","details":"{\"sent\":1,\"failed\":1}"},
		{"username":"admin","action":"update","resource":"server_backup","resource_id":"config","details":"{}"}],
		"pagination":{"page":1}}`)
	entries, err := parseAuditEntries(raw)
	if err != nil || len(entries) != 2 {
		t.Fatalf("parseAuditEntries: %v len=%d", err, len(entries))
	}
	got := countAudit(entries, "test", "channels")
	if len(got) != 1 || got[0].Username != "admin" ||
		!strings.Contains(got[0].Details, `"sent":1`) ||
		!strings.Contains(got[0].Details, `"failed":1`) {
		t.Fatalf("countAudit: %+v", got)
	}
	if len(countAudit(entries, "test", "nope")) != 0 {
		t.Fatal("countAudit miss: want 0")
	}
	if _, err := parseAuditEntries([]byte("not-json")); err == nil {
		t.Fatal("parseAuditEntries bad json: want error")
	}
}
