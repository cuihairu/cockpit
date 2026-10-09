// workflows 探针 helpers 单测（覆盖率收口：helpers 全覆盖）。
//
// 场景编排 main()（W0-W9 真机 REST/agent/审计/权限面）按验收惯例走真机
// 探针 + .acceptance 证据，不在此单测面内（jobs/probe 先例）。本文件覆盖
// 全部可确定性验证的逻辑：证据层 / REST 层（httptest 假 cockpit）/
// agent 在线判定双形态 / Job 与 run 与审计响应解析 / 轮询器（假 API 态
// 迁移与超时）/ 定义与 run 触发装配。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
		*apiBase = "http://127.0.0.1:20010"
		*evDir = ""
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
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "tk-wf-1"})
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
	if b, err := os.ReadFile(*evDir + "/sample.json"); err != nil || string(b) != `{"k":1}` {
		t.Fatalf("saveEV: %v %q", err, b)
	}
	*evDir = ""
	saveEV("skip.json", []byte("x")) // 不 panic 即可

	// mustJSON：合法值序列化；不可序列化值回退 {}
	if b := mustJSON(map[string]int{"a": 1}); !strings.Contains(string(b), `"a": 1`) {
		t.Fatalf("mustJSON = %q", b)
	}
	if b := mustJSON(make(chan int)); string(b) != "{}" {
		t.Fatalf("mustJSON fallback = %q", b)
	}
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
	setToken("tk-1")
	code, _ = reqJSON(http.MethodPut, srv.URL+"/api/echo", map[string]string{"k": "v"})
	if code != 200 || gotMethod != http.MethodPut {
		t.Fatalf("put: code=%d method=%q", code, gotMethod)
	}
	if gotBody != `{"k":"v"}` || gotAuth != "Bearer tk-1" {
		t.Fatalf("body=%q auth=%q", gotBody, gotAuth)
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
	if err != nil || tk != "tk-wf-1" {
		t.Fatalf("loginAs: tk=%q err=%v", tk, err)
	}
	login()
	evMu.Lock()
	tok := token
	evMu.Unlock()
	if tok != "tk-wf-1" {
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

// ---------- Job 解析与轮询 ----------

func TestParseJobHelpers(t *testing.T) {
	resetGlobals(t)
	raw := []byte(`{"jobs":[
		{"id":"j2","target":"a1","status":"success","actor":"workflow","createdAt":"2026-10-09T00:00:05Z"},
		{"id":"j1","target":"a1","status":"failed","actor":"workflow","exitCode":1,
		 "createdAt":"2026-10-09T00:00:00Z"}
	]}`)
	jobs, err := parseJobsList(raw)
	if err != nil || len(jobs) != 2 || jobs[0].ID != "j2" || jobs[1].ExitCode != 1 {
		t.Fatalf("parseJobsList: jobs=%+v err=%v", jobs, err)
	}
	if _, err := parseJobsList([]byte("not-json")); err == nil {
		t.Fatal("bad json: want error")
	}
	v, err := parseJob([]byte(`{"id":"j1","status":"pending","createdAt":"2026-10-09T00:00:00Z"}`))
	if err != nil || v.Status != "pending" || v.CreatedAt == nil {
		t.Fatalf("parseJob: %+v err=%v", v, err)
	}
	if _, err := parseJob([]byte("bad")); err == nil {
		t.Fatal("bad json: want error")
	}
	for st, want := range map[string]bool{"pending": false, "running": false,
		"success": true, "failed": true, "cancelled": true} {
		if isJobTerminal(st) != want {
			t.Fatalf("isJobTerminal(%q) = %v", st, !want)
		}
	}
}

func TestPollJobTransitionsAndTimeout(t *testing.T) {
	resetGlobals(t)
	// pending → running → success 三态迁移（cond=terminal 在第三拍命中）
	calls := 0
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/jobs/j1", func(w http.ResponseWriter, r *http.Request) {
			calls++
			st := "pending"
			if calls == 2 {
				st = "running"
			} else if calls >= 3 {
				st = "success"
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"j1","status":%q}`, st)))
		})
	})
	v, err := awaitJob("j1", 3*time.Second)
	if err != nil || v.Status != "success" || calls < 3 {
		t.Fatalf("awaitJob: v=%+v err=%v calls=%d", v, err, calls)
	}

	// 条件恒不满足 → 超时带回最后视图
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/jobs/j2", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":"j2","status":"pending"}`))
		})
	})
	v, err = pollJob("j2", func(j *jobView) bool { return false }, 400*time.Millisecond)
	if err == nil || v == nil || v.Status != "pending" {
		t.Fatalf("timeout: v=%+v err=%v", v, err)
	}

	// 请求全程失败 → 超时无视图
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/jobs/j3", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
		})
	})
	if v, err = pollJob("j3", func(j *jobView) bool { return true }, 400*time.Millisecond); err == nil || v != nil {
		t.Fatalf("all-fail: v=%+v err=%v", v, err)
	}
}

// ---------- run 解析与轮询 ----------

func TestParseRunAndPoll(t *testing.T) {
	resetGlobals(t)
	raw := []byte(`{"id":"r1","workflowId":"wf1","workflowName":"chain","status":"running",
		"actor":"admin","finishedAt":null,
		"steps":[{"name":"s1","type":"agent.exec","target":"a1","status":"success",
			"jobId":"j1","attempts":1},{"name":"s2","status":"pending"}]}`)
	rv, err := parseRun(raw)
	if err != nil || rv.ID != "r1" || len(rv.Steps) != 2 ||
		rv.Steps[0].JobID != "j1" || rv.Steps[1].Status != "pending" {
		t.Fatalf("parseRun: %+v err=%v", rv, err)
	}
	if _, err := parseRun([]byte("bad")); err == nil {
		t.Fatal("bad json: want error")
	}
	for st, want := range map[string]bool{"running": false,
		"success": true, "failed": true, "cancelled": true} {
		if isRunTerminal(st) != want {
			t.Fatalf("isRunTerminal(%q) = %v", st, !want)
		}
	}

	// pollRun：running → success 迁移
	calls := 0
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/workflow-runs/r1", func(w http.ResponseWriter, r *http.Request) {
			calls++
			st := "running"
			if calls >= 2 {
				st = "success"
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"r1","status":%q,"steps":[]}`, st)))
		})
	})
	v, err := pollRun("r1", func(x *runView) bool { return isRunTerminal(x.Status) }, 3*time.Second)
	if err != nil || v.Status != "success" || calls < 2 {
		t.Fatalf("pollRun: v=%+v err=%v calls=%d", v, err, calls)
	}

	// 超时带回最后视图 / 非 200 无视图
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/workflow-runs/r2", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":"r2","status":"running","steps":[]}`))
		})
	})
	if v, err = pollRun("r2", func(x *runView) bool { return false }, 400*time.Millisecond); err == nil || v == nil {
		t.Fatalf("timeout: v=%+v err=%v", v, err)
	}
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/workflow-runs/r3", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
		})
	})
	if v, err = pollRun("r3", func(x *runView) bool { return true }, 400*time.Millisecond); err == nil || v != nil {
		t.Fatalf("404: v=%+v err=%v", v, err)
	}
}

// ---------- run 关联 Job 台账 ----------

func TestJobsByRunAndPoll(t *testing.T) {
	resetGlobals(t)
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("workflow_run_id"); got != "r1" {
				t.Fatalf("workflow_run_id = %q", got)
			}
			_, _ = w.Write([]byte(`{"jobs":[{"id":"j1","status":"running"},{"id":"j0","status":"success"}]}`))
		})
	})
	jobs, err := jobsByRun("r1")
	if err != nil || len(jobs) != 2 {
		t.Fatalf("jobsByRun: jobs=%+v err=%v", jobs, err)
	}
	// 条件首拍命中
	got, err := pollJobsByRun("r1", func(js []jobView) bool { return len(js) == 2 }, 2*time.Second)
	if err != nil || len(got) != 2 {
		t.Fatalf("pollJobsByRun hit: %+v err=%v", got, err)
	}
	// 恒不满足 → 超时
	if _, err = pollJobsByRun("r1", func(js []jobView) bool { return false }, 400*time.Millisecond); err == nil {
		t.Fatal("timeout: want error")
	}
	// 非 200 → 超时错误路径
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	t.Cleanup(srv.Close)
	*apiBase = srv.URL
	if _, err = pollJobsByRun("r1", func(js []jobView) bool { return true }, 400*time.Millisecond); err == nil {
		t.Fatal("non-200: want error")
	}
}

// ---------- 定义与 run 装配 ----------

func TestStepBuilder(t *testing.T) {
	resetGlobals(t)
	s := step("s1", "a1", "uptime", nil)
	if s["type"] != "agent.exec" || s["target"] != "a1" {
		t.Fatalf("step base: %+v", s)
	}
	p := s["parameters"].(map[string]interface{})
	if p["name"] != "s1" || p["command"] != "uptime" {
		t.Fatalf("step params: %+v", p)
	}
	// extra 合并进 parameters（retry/timeout_s 条件携带——server 只认 parameters 内元参数）
	s2 := step("s2", "a1", "true", map[string]interface{}{"retry": 1, "timeout_s": 60})
	if s2["retry"] != nil {
		t.Fatalf("extra leaked to top level: %+v", s2)
	}
	p2 := s2["parameters"].(map[string]interface{})
	if p2["retry"] != 1 || p2["timeout_s"] != 60 {
		t.Fatalf("step extra: %+v", p2)
	}
}

func TestWfCreateAndRun(t *testing.T) {
	resetGlobals(t)
	// 创建：201 → id
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/workflows", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"id":"wf1","name":"n"}`))
		})
		mux.HandleFunc("/api/workflows/wf1/run", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"id":"r1","status":"running","steps":[]}`))
		})
		mux.HandleFunc("/api/workflows/wf1/runs", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"runs":[{"id":"r1"},{"id":"r0"}]}`))
		})
	})
	if id := wfCreate("n", []map[string]interface{}{step("s1", "a1", "true", nil)}); id != "wf1" {
		t.Fatalf("wfCreate id = %q", id)
	}
	rv, raw := wfRun("wf1")
	if rv.ID != "r1" || rv.Status != "running" || len(raw) == 0 {
		t.Fatalf("wfRun: %+v", rv)
	}
	n, err := wfRunsCount("wf1")
	if err != nil || n != 2 {
		t.Fatalf("wfRunsCount: n=%d err=%v", n, err)
	}

	// 创建非 201 → fatal(2)
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"name is required"}`))
	}))
	t.Cleanup(srv2.Close)
	*apiBase = srv2.URL
	code := stubExit(t)
	wfCreate("n", nil)
	if *code != 2 {
		t.Fatal("wfCreate non-201 should fatal")
	}
	// 创建 201 无 id → fatal(2)
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv3.Close)
	*apiBase = srv3.URL
	code = stubExit(t)
	wfCreate("n", nil)
	if *code != 2 {
		t.Fatal("wfCreate no id should fatal")
	}
	// 触发非 201 → fatal(2)
	srv4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":"workflow is already running"}`))
	}))
	t.Cleanup(srv4.Close)
	*apiBase = srv4.URL
	code = stubExit(t)
	wfRun("wf1")
	if *code != 2 {
		t.Fatal("wfRun non-201 should fatal")
	}
	// 触发 201 无 id → fatal(2)
	srv5 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"status":"running"}`))
	}))
	t.Cleanup(srv5.Close)
	*apiBase = srv5.URL
	code = stubExit(t)
	wfRun("wf1")
	if *code != 2 {
		t.Fatal("wfRun no id should fatal")
	}
	// run 台账非 200 → error
	if _, err = wfRunsCount("wf1"); err == nil {
		t.Fatal("wfRunsCount non-200: want error")
	}
	// run 台账 200 但 body 非 JSON → error
	srv6 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv6.Close()
	*apiBase = srv6.URL
	if _, err = wfRunsCount("wf1"); err == nil {
		t.Fatal("wfRunsCount bad json: want error")
	}
}

// ---------- 审计解析 ----------

func TestAuditHelpers(t *testing.T) {
	resetGlobals(t)
	raw := []byte(`{"data":[
		{"username":"admin","action":"workflow_create","resource":"workflow","resource_id":"wf1","details":"{}"},
		{"username":"admin","action":"workflow_run","resource":"workflow","resource_id":"r1","details":"{}"},
		{"username":"admin","action":"workflow_run","resource":"workflow","resource_id":"r2","details":"{}"}
	],"pagination":{"total":3}}`)
	entries, err := parseAuditEntries(raw)
	if err != nil || len(entries) != 3 {
		t.Fatalf("parseAuditEntries: %+v err=%v", entries, err)
	}
	if _, err := parseAuditEntries([]byte("not-json")); err == nil {
		t.Fatal("bad json: want error")
	}
	if n := countAuditByAction(entries, "workflow_run"); n != 2 {
		t.Fatalf("count workflow_run = %d, want 2", n)
	}
	if n := countAuditByAction(entries, "workflow_cancel"); n != 0 {
		t.Fatalf("count workflow_cancel = %d, want 0", n)
	}
}
