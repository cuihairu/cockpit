// jobs 探针 helpers 单测（覆盖率收口：helpers 全覆盖）。
//
// 场景编排 main()（J0-J8 真机 REST/agent/审计/权限面）按验收惯例走真机
// 探针 + .acceptance 证据，不在此单测面内（同 darwin launchd / windows SCM
// 执行层的结构性边界，guac/probe 先例）。本文件覆盖全部可确定性验证的逻辑：
// 证据层 / REST 层（httptest 假 cockpit）/ agent 在线判定双形态 / Job 与
// 审计响应解析（含台账计数与按 id 查找）/ pgrep 孤儿复核（注入桩）。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- 测试基建 ----------

// resetGlobals 每用例隔离包级状态（flag 变量与注入点还原为默认语义）
func resetGlobals(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		evMu.Lock()
		token, fails, passes, evFile = "", 0, 0, nil
		evMu.Unlock()
		*apiBase = "http://127.0.0.1:19994"
		*evDir = ""
		osExit = os.Exit
		pgrepCmd = func(pattern string) ([]byte, error) {
			return nil, fmt.Errorf("pgrep not available in unit test")
		}
	})
	osExit = os.Exit
	pgrepCmd = func(pattern string) ([]byte, error) {
		return nil, fmt.Errorf("pgrep not available in unit test")
	}
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
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "tk-jobs-1"})
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

	// saveEV：有目录落盘、无目录静默跳过
	*evDir = t.TempDir()
	saveEV("sample.json", []byte(`{"k":1}`))
	if b, err := os.ReadFile(filepath.Join(*evDir, "sample.json")); err != nil || string(b) != `{"k":1}` {
		t.Fatalf("saveEV: %v %q", err, b)
	}
	*evDir = ""
	saveEV("skip.json", []byte("x")) // 不 panic 即可
}

// ---------- REST 层 ----------

func TestReqJSONShapes(t *testing.T) {
	resetGlobals(t)
	var gotMethod, gotBody, gotAuth string
	srv := fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/echo", func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotAuth = r.Header.Get("Authorization")
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"ok":true}`))
		})
	})

	// nil body + 无 token
	code, raw := reqJSON(http.MethodPost, srv.URL+"/api/echo", nil)
	if code != 200 || string(raw) != `{"ok":true}` || gotBody != "" || gotAuth != "" {
		t.Fatalf("nil body: code=%d raw=%q body=%q auth=%q", code, raw, gotBody, gotAuth)
	}
	// body + token
	evMu.Lock()
	token = "tk-1"
	evMu.Unlock()
	code, _ = reqJSON(http.MethodPut, srv.URL+"/api/echo", map[string]string{"k": "v"})
	if code != 200 || gotMethod != http.MethodPut {
		t.Fatalf("put: code=%d method=%q", code, gotMethod)
	}
	if gotBody != `{"k":"v"}` {
		t.Fatalf("body = %q, want {\"k\":\"v\"}", gotBody)
	}
	if gotAuth != "Bearer tk-1" {
		t.Fatalf("auth = %q", gotAuth)
	}
	// 传输失败 → -1 + 错误文本
	code, raw = reqJSON(http.MethodGet, "http://127.0.0.1:1/api/x", nil)
	if code != -1 || !strings.Contains(string(raw), "connection refused") {
		t.Fatalf("transport error: code=%d raw=%q", code, raw)
	}
}

func TestLoginAsAndLogin(t *testing.T) {
	resetGlobals(t)
	// 成功（fakeAPI 出 token）
	fakeAPI(t, nil)
	tk, err := loginAs("admin", "x")
	if err != nil || tk != "tk-jobs-1" {
		t.Fatalf("loginAs: tk=%q err=%v", tk, err)
	}
	login()
	evMu.Lock()
	tok := token
	evMu.Unlock()
	if tok != "tk-jobs-1" {
		t.Fatalf("login() token = %q", tok)
	}

	// HTTP 非 200 → error
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"bad credentials"}`))
	}))
	t.Cleanup(srv2.Close)
	*apiBase = srv2.URL
	if _, err := loginAs("u", "p"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("loginAs 401: err=%v", err)
	}
	// login() 包装成 fatal(2)
	code := stubExit(t)
	login()
	if *code != 2 {
		t.Fatal("login HTTP!=200 should fatal")
	}

	// 200 无 token → error / fatal
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv3.Close)
	*apiBase = srv3.URL
	if _, err := loginAs("u", "p"); err == nil || !strings.Contains(err.Error(), "无 token") {
		t.Fatalf("loginAs no token: err=%v", err)
	}
	code = stubExit(t)
	login()
	if *code != 2 {
		t.Fatal("login without token should fatal")
	}
}

func TestAgentOnline(t *testing.T) {
	resetGlobals(t)
	// 裸数组：online / offline / 缺席
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("[" +
				`{"id":"a1","status":"online"},` +
				`{"id":"a2","status":"offline"}` +
				"]"))
		})
	})
	if ok, err := agentOnline("a1"); err != nil || !ok {
		t.Fatalf("a1: ok=%v err=%v", ok, err)
	}
	if ok, err := agentOnline("a2"); err != nil || ok {
		t.Fatalf("a2 offline: ok=%v err=%v", ok, err)
	}
	if _, err := agentOnline("a9"); err == nil {
		t.Fatal("missing id: want error")
	}
	// {agents:[..]} 包装形态
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"agents":[{"id":"a1","status":"online"}]}`))
		})
	})
	if ok, err := agentOnline("a1"); err != nil || !ok {
		t.Fatalf("wrapper a1: ok=%v err=%v", ok, err)
	}
	// 坏响应
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		})
	})
	if _, err := agentOnline("a1"); err == nil {
		t.Fatal("bad json: want error")
	}
}

// ---------- Job / 审计解析 ----------

func TestParseJobsListAndHelpers(t *testing.T) {
	resetGlobals(t)
	raw := []byte(`{"jobs":[
		{"id":"j2","target":"a1","status":"success","actor":"admin"},
		{"id":"j1","target":"ghost","status":"failed","actor":"admin","exitCode":3,"error":""}
	]}`)
	jobs, err := parseJobsList(raw)
	if err != nil || len(jobs) != 2 || jobs[0].ID != "j2" {
		t.Fatalf("parseJobsList: jobs=%+v err=%v", jobs, err)
	}
	if _, err := parseJobsList([]byte("not-json")); err == nil {
		t.Fatal("bad json: want error")
	}

	if n := countJobsByTarget(jobs, "a1"); n != 1 {
		t.Fatalf("countJobsByTarget a1 = %d, want 1", n)
	}
	if n := countJobsByTarget(jobs, "ghost"); n != 1 {
		t.Fatalf("countJobsByTarget ghost = %d, want 1", n)
	}
	if n := countJobsByTarget(jobs, "none"); n != 0 {
		t.Fatalf("countJobsByTarget none = %d, want 0", n)
	}

	if j := findJobByID(jobs, "j1"); j == nil || j.ExitCode != 3 {
		t.Fatalf("findJobByID j1 = %+v", j)
	}
	if j := findJobByID(jobs, "j9"); j != nil {
		t.Fatalf("findJobByID absent = %+v, want nil", j)
	}

	v, err := parseJob([]byte(`{"id":"j1","status":"failed","exitCode":3,
		"startedAt":"2026-10-05T10:00:00Z","finishedAt":"2026-10-05T10:00:01Z"}`))
	if err != nil || v.ExitCode != 3 || v.StartedAt == nil || v.FinishedAt == nil {
		t.Fatalf("parseJob: %+v err=%v", v, err)
	}
	if _, err := parseJob([]byte("bad")); err == nil {
		t.Fatal("bad json: want error")
	}
}

func TestParseAuditAndHelpers(t *testing.T) {
	resetGlobals(t)
	raw := []byte(`{"data":[
		{"username":"admin","action":"job_run","resource":"job","resource_id":"j1",
		 "details":"{\"type\":\"agent.exec\",\"target\":\"a1\",\"status\":\"success\",\"params\":{\"command\":\"uptime\"}}"},
		{"username":"admin","action":"job_run","resource":"job","resource_id":"j2",
		 "details":"{\"status\":\"failed\"}"}
	],"pagination":{"total":2}}`)
	entries, err := parseAuditEntries(raw)
	if err != nil || len(entries) != 2 {
		t.Fatalf("parseAuditEntries: entries=%+v err=%v", entries, err)
	}
	if _, err := parseAuditEntries([]byte("not-json")); err == nil {
		t.Fatal("bad json: want error")
	}

	m := auditEntriesForResourceID(entries, "j1")
	if len(m) != 1 || m[0].ResourceID != "j1" {
		t.Fatalf("auditEntriesForResourceID j1 = %+v", m)
	}
	if m := auditEntriesForResourceID(entries, "j9"); m != nil {
		t.Fatalf("absent = %+v, want nil", m)
	}
	if !detailsHas(entries[0], `"type":"agent.exec"`, `"command":"uptime"`) {
		t.Fatal("detailsHas hit: want true")
	}
	if detailsHas(entries[1], `"command":`) {
		t.Fatal("detailsHas miss: want false")
	}
}

// ---------- 孤儿复核（注入桩） ----------

func TestOrphanAlive(t *testing.T) {
	resetGlobals(t)
	// pgrep 命中（exit 0 有输出）
	pgrepCmd = func(pattern string) ([]byte, error) {
		if pattern != "sleep 297" {
			t.Fatalf("pattern = %q", pattern)
		}
		return []byte("12345\n"), nil
	}
	if !orphanAlive("sleep 297") {
		t.Fatal("hit: want true")
	}
	// pgrep 无匹配（exit 1）
	pgrepCmd = func(pattern string) ([]byte, error) {
		return []byte(""), fmt.Errorf("exit status 1")
	}
	if orphanAlive("sleep 297") {
		t.Fatal("no match: want false")
	}
	// 默认闭包（真机 pgrep；单测恒注入桩）——闭包体不执行，仅确认变量非 nil
	if pgrepCmd == nil {
		t.Fatal("pgrepCmd nil")
	}
	_ = time.Now
}
