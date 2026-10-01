// logs 探针 helpers 单测（覆盖率收口：包 0% → helpers 全覆盖）。
//
// 场景编排 main()（T0-T4 真机三 agent/systemd/docker 突发源）按验收惯例走
// 真机探针 + .acceptance 证据，不在此单测面内（同 darwin launchd / windows
// SCM 执行层的结构性边界，guac/probe 先例）。本文件覆盖全部可确定性验证的
// 逻辑：证据层 / REST 层（httptest 假 cockpit）/ capability 双形态解析 /
// systemd-run 重试环与 journalctl 轮询（注入桩）/ NDJSON 尾随流四失败面与
// 帧扫描边界 / 序列分析（序号提取·有序·重复·缺口）。
package main

import (
	"encoding/json"
	"errors"
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
		*apiBase = "http://127.0.0.1:19992"
		*evDir = ""
		busyRetryWait = 500 * time.Millisecond
		systemdRunCmd = func(unit, script string) ([]byte, error) {
			return nil, fmt.Errorf("systemd-run not available in unit test")
		}
		journalCtlCmd = func(unit string) ([]byte, error) {
			return nil, fmt.Errorf("journalctl not available in unit test")
		}
		osExit = os.Exit
	})
	busyRetryWait = time.Millisecond
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
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "tk-logs-1"})
	})
	if h != nil {
		h(mux)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	*apiBase = srv.URL
	return srv
}

func agentListJSON(id string, withLogs bool) string {
	caps := "[]"
	if withLogs {
		caps = `[{"type":"logs"}]`
	}
	return fmt.Sprintf(`[{"id":%q,"hostname":"h","ip":"1.2.3.4","status":"online","capabilities":%s}]`, id, caps)
}

// ---------- 证据层 ----------

func TestEvCheckFatalTruncate(t *testing.T) {
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

func TestLoginPaths(t *testing.T) {
	resetGlobals(t)
	// 成功
	fakeAPI(t, nil)
	login()
	evMu.Lock()
	tok := token
	evMu.Unlock()
	if tok != "tk-logs-1" {
		t.Fatalf("token = %q", tok)
	}

	// HTTP 非 200 → fatal(2)
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"x"}`))
	}))
	t.Cleanup(srv2.Close)
	*apiBase = srv2.URL
	code := stubExit(t)
	login()
	if *code != 2 {
		t.Fatal("login HTTP!=200 should fatal")
	}

	// 200 无 token → fatal(2)
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv3.Close)
	*apiBase = srv3.URL
	code = stubExit(t)
	login()
	if *code != 2 {
		t.Fatal("login without token should fatal")
	}
}

func TestAgentHasLogsCapability(t *testing.T) {
	resetGlobals(t)
	// 裸数组三形态
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("[" +
				`{"id":"a1","capabilities":[{"type":"logs"}]},` +
				`{"id":"a2","capabilities":[]}` +
				"]"))
		})
	})
	if ok, err := agentHasLogsCapability("a1"); err != nil || !ok {
		t.Fatalf("a1: ok=%v err=%v", ok, err)
	}
	if ok, err := agentHasLogsCapability("a2"); err != nil || ok {
		t.Fatalf("a2: ok=%v err=%v", ok, err)
	}
	if _, err := agentHasLogsCapability("a9"); err == nil {
		t.Fatal("missing id: want error")
	}
	// {agents:[..]} 包装形态
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"agents":[{"id":"a1","capabilities":[{"type":"logs"}]}]}`))
		})
	})
	if ok, err := agentHasLogsCapability("a1"); err != nil || !ok {
		t.Fatalf("wrapper a1: ok=%v err=%v", ok, err)
	}
	// 坏响应
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		})
	})
	if _, err := agentHasLogsCapability("a1"); err == nil {
		t.Fatal("bad json: want error")
	}
}

// ---------- systemd 层（注入桩） ----------

func TestRunUnitRetryLoop(t *testing.T) {
	resetGlobals(t)
	// 一次成功
	systemdRunCmd = func(unit, script string) ([]byte, error) { return nil, nil }
	if err := runUnit("u", "echo"); err != nil {
		t.Fatalf("success: %v", err)
	}
	// 非 busy 错误立即返回（带原文截断）
	systemdRunCmd = func(unit, script string) ([]byte, error) {
		return []byte("boom happened"), errors.New("exit 1")
	}
	err := runUnit("u", "echo")
	if err == nil || !strings.Contains(err.Error(), "boom happened") || !strings.Contains(err.Error(), "u") {
		t.Fatalf("non-busy error: %v", err)
	}
	// busy（already exist）两轮后成功
	calls := 0
	systemdRunCmd = func(unit, script string) ([]byte, error) {
		calls++
		if calls < 3 {
			return []byte("Unit u.service already exists"), errors.New("exit 1")
		}
		return nil, nil
	}
	if err := runUnit("u", "echo"); err != nil || calls != 3 {
		t.Fatalf("busy-then-ok: err=%v calls=%d", err, calls)
	}
	// busy 20 轮耗尽 → unit name busy
	systemdRunCmd = func(unit, script string) ([]byte, error) {
		return []byte("already exists"), errors.New("exit 1")
	}
	err = runUnit("u", "echo")
	if err == nil || !strings.Contains(err.Error(), "unit name busy") {
		t.Fatalf("busy exhausted: %v", err)
	}
}

func TestWaitJournalPoll(t *testing.T) {
	resetGlobals(t)
	journalCtlCmd = func(unit string) ([]byte, error) {
		return []byte("-- Logs begin --\ncockpit marker-here\n"), nil
	}
	if !waitJournal("u", "marker-here", time.Second) {
		t.Fatal("marker present: want true")
	}
	journalCtlCmd = func(unit string) ([]byte, error) { return []byte("nothing"), nil }
	if waitJournal("u", "marker-here", 10*time.Millisecond) {
		t.Fatal("timeout: want false")
	}
}

// ---------- NDJSON 尾随流 ----------

func TestStartFollowAndRead(t *testing.T) {
	resetGlobals(t)
	evMu.Lock()
	token = "tk-1"
	evMu.Unlock()
	agentID := "a1"

	// 正常流：帧 / 空行跳过 / 坏 JSON 跳过 / 超初始缓冲的长行（4MB 上限内）
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents/a1/logs/follow", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tk-1" {
				w.WriteHeader(401)
				return
			}
			w.Header().Set("Content-Type", "application/x-ndjson")
			fl := w.(http.Flusher)
			fmt.Fprintln(w, `{"data":"HF-1"}`)
			fmt.Fprintln(w) // 空行
			fmt.Fprintln(w, `{bad json`)
			fmt.Fprintln(w, `{"data":"`+strings.Repeat("x", 100_000)+`"}`)
			fmt.Fprintln(w, `{"eof":true,"reason":"exited"}`)
			fl.Flush()
		})
	})
	fs, err := startFollow(agentID, map[string]interface{}{"tail": 5}, 5*time.Second)
	if err != nil {
		t.Fatalf("startFollow: %v", err)
	}
	f, ok := fs.read(3 * time.Second)
	if !ok || f.Data != "HF-1" {
		t.Fatalf("frame1: ok=%v f=%+v", ok, f)
	}
	f, ok = fs.read(3 * time.Second)
	if !ok || len(f.Data) != 100_000 {
		t.Fatalf("long frame: ok=%v len=%d", ok, len(f.Data))
	}
	f, ok = fs.read(3 * time.Second)
	if !ok || !f.EOF || f.Reason != "exited" {
		t.Fatalf("eof frame: ok=%v f=%+v", ok, f)
	}
	// 流尽后的读超时（timeout 内无帧）
	if _, ok := fs.read(50 * time.Millisecond); ok {
		t.Fatal("drained stream: want timeout")
	}
	fs.abort() // 客户端断开（不 panic、连接被关）

	// 非 200：错误带响应体摘要
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents/a1/logs/follow", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(502)
			_, _ = w.Write([]byte(`{"error":"agent unavailable"}`))
		})
	})
	if _, err := startFollow(agentID, nil, 5*time.Second); err == nil ||
		!strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "agent unavailable") {
		t.Fatalf("non-200: %v", err)
	}
	// 响应头超时（服务端握住连接不发头）
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	t.Cleanup(slow.Close)
	*apiBase = slow.URL
	if _, err := startFollow(agentID, nil, 50*time.Millisecond); err == nil ||
		!strings.Contains(err.Error(), "无响应") {
		t.Fatalf("header timeout: %v", err)
	}
	// 传输失败
	*apiBase = "http://127.0.0.1:1"
	if _, err := startFollow(agentID, nil, 5*time.Second); err == nil {
		t.Fatal("transport error: want error")
	}
}

// ---------- 序列分析 ----------

func TestSeqAnalysis(t *testing.T) {
	resetGlobals(t)
	var nums []int
	appendNums(&nums, hfRe, "line HF-42 tail", 0)
	appendNums(&nums, hfRe, "line HF-7 below-lo", 10) // 基址之下丢弃
	appendNums(&nums, hfRe, "no marker", 0)
	// docRe 单独提取另一族序号（DOCLINE），与 hfRe 互不串
	appendNums(&nums, docRe, "DOCLINE-9", 0)
	if len(nums) != 2 || nums[0] != 42 || nums[1] != 9 {
		t.Fatalf("appendNums: %v, want [42 9]", nums)
	}

	dups, gaps, ordered := analyzeSeq([]int{1, 2, 3, 5})
	if dups != 0 || gaps != 1 || !ordered {
		t.Fatalf("clean-with-gap: dups=%d gaps=%d ordered=%v", dups, gaps, ordered)
	}
	dups, gaps, ordered = analyzeSeq([]int{1, 2, 2, 3})
	if dups != 1 || gaps != -1 || ordered {
		t.Fatalf("dup: dups=%d gaps=%d ordered=%v", dups, gaps, ordered)
	}
	dups, _, ordered = analyzeSeq([]int{3, 1, 2})
	if dups != 0 || ordered {
		t.Fatalf("unordered: dups=%d ordered=%v", dups, ordered)
	}
	if _, gaps, _ := analyzeSeq(nil); gaps != -1 {
		t.Fatalf("empty: gaps=%d, want -1", gaps)
	}
	if !containsNum([]int{5, 7}, 7) || containsNum([]int{5, 7}, 6) {
		t.Fatal("containsNum")
	}
}

// ---------- T4 联邦检索 ----------

func TestRunSearchAndSkipped(t *testing.T) {
	resetGlobals(t)
	*evDir = t.TempDir()
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/logs/search", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"results":[{"agentId":"a1","hostname":"h1","ok":true,"lines":"HF-1"}],` +
				`"skipped":[{"agentId":"a2","reason":"no-logs"}]}`))
		})
	})
	resp, raw, err := runSearch("t", map[string]interface{}{"tail": 5})
	if err != nil {
		t.Fatalf("runSearch: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].AgentID != "a1" ||
		!resp.Results[0].OK || !strings.Contains(string(raw), "a1") {
		t.Fatalf("resp=%+v raw=%s", resp.Results, raw)
	}
	// 证据双写：请求体 + 响应体
	for _, n := range []string{"search-t.json", "search-t-resp.json"} {
		if _, err := os.Stat(filepath.Join(*evDir, n)); err != nil {
			t.Fatalf("evidence %s: %v", n, err)
		}
	}
	// skipped 查找
	if r, ok := resp.skippedReason("a2"); !ok || r != "no-logs" {
		t.Fatalf("skippedReason: r=%q ok=%v", r, ok)
	}
	if _, ok := resp.skippedReason("a9"); ok {
		t.Fatal("skippedReason absent: want !ok")
	}

	// 非 200 → 错误；坏 JSON → 错误
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/logs/search", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		})
	})
	if _, _, err := runSearch("e1", nil); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("non-200: %v", err)
	}
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/logs/search", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		})
	})
	if _, _, err := runSearch("e2", nil); err == nil || !strings.Contains(err.Error(), "bad response") {
		t.Fatalf("bad json: %v", err)
	}
}
