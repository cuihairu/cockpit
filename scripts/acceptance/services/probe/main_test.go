// services 探针 helpers 单测（覆盖率收口：包 0% → helpers 全覆盖）。
//
// 场景编排 main()（T0-T5 真机 systemd 动词面/unit 文件编辑/journal 跳转）
// 按验收惯例走真机探针 + .acceptance 证据，不在此单测面内（同 darwin
// launchd / windows SCM 执行层的结构性边界，guac/probe 先例）。本文件覆盖
// 全部可确定性验证的逻辑：证据层 / REST 层（httptest 假 cockpit）/ capability
// 双形态解析 / 动词与 unit 文件与日志端点薄封装 / 本机 systemctl 只读对照
// （注入桩，含生产默认体的真 echo 路径）/ 服务列表解析与查找。
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// ---------- 测试基建 ----------

// resetGlobals 每用例隔离包级状态（flag 变量与注入点还原为默认语义）
func resetGlobals(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		evMu.Lock()
		token, fails, passes, evFile = "", 0, 0, nil
		evMu.Unlock()
		*apiBase = "http://127.0.0.1:19993"
		*evDir = ""
		*unit = "cockpit-acc-svc.service"
		osExit = os.Exit
		shExec = func(name string, args ...string) ([]byte, error) {
			return nil, errNoShExec
		}
	})
}

// defaultShExec 捕获生产默认注入值（测试二进制 init 时求值）——真 echo 路径
// 覆盖 shExec 默认闭包体，验证 shOut「stdout 为准、去首尾空白」契约
var defaultShExec = shExec

// errNoShExec 单测默认桩：不再触碰本机 systemctl
var errNoShExec = errStr("shExec not available in unit test")

type errStr string

func (e errStr) Error() string { return string(e) }

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
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "tk-svc-1"})
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

// evFile 为 nil 时 ev/check 不 panic（main() 之外的零值路径）
func TestEvWithoutFile(t *testing.T) {
	resetGlobals(t)
	evMu.Lock()
	evFile = nil
	evMu.Unlock()
	ev("no file line")
	check("no file check", true, "ok")
	if passes != 1 {
		t.Fatalf("passes = %d", passes)
	}
}

// ---------- REST 层 ----------

func TestReqJSONAndLogin(t *testing.T) {
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
	if code != 200 || gotMethod != http.MethodPut || gotAuth != "Bearer tk-1" {
		t.Fatalf("put: code=%d method=%q auth=%q", code, gotMethod, gotAuth)
	}
	if gotBody != `{"k":"v"}` {
		t.Fatalf("body = %q, want {\"k\":\"v\"}", gotBody)
	}
	// 传输失败 → -1 + 错误文本
	code, raw = reqJSON(http.MethodGet, "http://127.0.0.1:1/api/x", nil)
	if code != -1 || !strings.Contains(string(raw), "connection refused") {
		t.Fatalf("transport error: code=%d raw=%q", code, raw)
	}

	// login 成功
	evMu.Lock()
	token = ""
	evMu.Unlock()
	login()
	evMu.Lock()
	tok := token
	evMu.Unlock()
	if tok != "tk-svc-1" {
		t.Fatalf("login token = %q", tok)
	}
}

func TestLoginFatalPaths(t *testing.T) {
	resetGlobals(t)
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

func TestAgentHasCapability(t *testing.T) {
	resetGlobals(t)
	// 裸数组：命中 / 类型不符 / 缺 id
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("[" +
				`{"id":"a1","capabilities":[{"type":"service"}]},` +
				`{"id":"a2","capabilities":[]}` +
				"]"))
		})
	})
	if ok, err := agentHasCapability("a1", "service"); err != nil || !ok {
		t.Fatalf("a1: ok=%v err=%v", ok, err)
	}
	if ok, err := agentHasCapability("a2", "service"); err != nil || ok {
		t.Fatalf("a2: ok=%v err=%v", ok, err)
	}
	if _, err := agentHasCapability("a9", "service"); err == nil {
		t.Fatal("missing id: want error")
	}
	// {agents:[..]} 包装形态
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"agents":[{"id":"a1","capabilities":[{"type":"service"}]}]}`))
		})
	})
	if ok, err := agentHasCapability("a1", "service"); err != nil || !ok {
		t.Fatalf("wrapper a1: ok=%v err=%v", ok, err)
	}
	// 坏响应
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		})
	})
	if _, err := agentHasCapability("a1", "service"); err == nil {
		t.Fatal("bad json: want error")
	}
}

// ---------- 动词 / unit 文件 / 日志端点薄封装 ----------

func TestActionAndUnitFileEndpoints(t *testing.T) {
	resetGlobals(t)
	var hits []string
	fakeAPI(t, func(mux *http.ServeMux) {
		// 子树挂载：services/{unit}/{act}、services/{unit}/file、daemon-reload 全走这
		mux.HandleFunc("/api/agents/a1/services/", func(w http.ResponseWriter, r *http.Request) {
			hits = append(hits, r.Method+" "+r.URL.Path)
			switch {
			case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/file"):
				_, _ = w.Write([]byte(`{"ok":true,"reloaded":true}`))
			case strings.HasSuffix(r.URL.Path, "/file"):
				_, _ = w.Write([]byte(`{"content":"[Unit]"}`))
			case strings.HasSuffix(r.URL.Path, "/daemon-reload"):
				_, _ = w.Write([]byte(`{"ok":true,"reloaded":true}`))
			default:
				_, _ = w.Write([]byte(`{"ok":true}`))
			}
		})
		mux.HandleFunc("/api/agents/a1/logs/query", func(w http.ResponseWriter, r *http.Request) {
			hits = append(hits, r.Method+" "+r.URL.Path)
			var b map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["type"] != "systemd" || b["source"] != "cockpit-acc-svc.service" {
				w.WriteHeader(400)
				return
			}
			_, _ = w.Write([]byte(`{"lines":"HF-1\\nHF-2"}`))
		})
		mux.HandleFunc("/api/agents/a1/logs/sources", func(w http.ResponseWriter, r *http.Request) {
			hits = append(hits, r.Method+" "+r.URL.Path)
			_, _ = w.Write([]byte(`{"sources":[{"id":"u1"}]}`))
		})
		mux.HandleFunc("/api/admin/audit/logs", func(w http.ResponseWriter, r *http.Request) {
			hits = append(hits, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
			_, _ = w.Write([]byte(`{"logs":[{"action":"service_action"}]}`))
		})
	})

	if code, raw := serviceAction("a1", "restart"); code != 200 ||
		!strings.Contains(string(raw), "ok") {
		t.Fatalf("serviceAction: %d %s", code, raw)
	}
	// serviceAction 的 URL 用 flag *unit 拼装——首击路径须带默认 unit 名
	if len(hits) == 0 || !strings.HasSuffix(hits[0], "/"+*unit+"/restart") {
		t.Fatalf("serviceAction url = %q, want suffix %q", hits[0], "/"+*unit+"/restart")
	}
	if code, raw := serviceActionUnit("a1", "jlog.service", "start"); code != 200 ||
		!strings.Contains(string(raw), "ok") {
		t.Fatalf("serviceActionUnit: %d %s", code, raw)
	}
	if code, raw := daemonReload("a1"); code != 200 ||
		!strings.Contains(string(raw), "reloaded") {
		t.Fatalf("daemonReload: %d %s", code, raw)
	}
	if code, raw := unitFileGet("a1", "x.service"); code != 200 || !strings.Contains(string(raw), "[Unit]") {
		t.Fatalf("unitFileGet: %d %s", code, raw)
	}
	if code, raw := unitFilePut("a1", "x.service", "[Unit]\nDescription=y"); code != 200 ||
		!strings.Contains(string(raw), "reloaded") {
		t.Fatalf("unitFilePut: %d %s", code, raw)
	}
	code, lines := logsQuery("a1", "cockpit-acc-svc.service", 50)
	if code != 200 || !strings.Contains(lines, "HF-1") {
		t.Fatalf("logsQuery: %d %q", code, lines)
	}
	if code, raw := logsSources("a1"); code != 200 || !strings.Contains(string(raw), "sources") {
		t.Fatalf("logsSources: %d %s", code, raw)
	}
	if got := auditBody(); !strings.Contains(got, "service_action") {
		t.Fatalf("auditBody: %s", got)
	}

	// 坏 JSON 的 logs/query → lines 空但 code 透传（放最后：换新假实例）
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents/a1/logs/query", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		})
	})
	if code, lines = logsQuery("a1", "u", 1); code != 200 || lines != "" {
		t.Fatalf("logsQuery bad json: %d %q", code, lines)
	}
}

// ---------- 本机只读对照（注入桩） ----------

func TestShOutReadOnly(t *testing.T) {
	resetGlobals(t)
	var gotName string
	var gotArgs []string
	shExec = func(name string, args ...string) ([]byte, error) {
		gotName, gotArgs = name, args
		return []byte("active\n"), nil
	}
	if s, err := shOut("systemctl", "is-active", *unit); err != nil || s != "active" {
		t.Fatalf("shOut: s=%q err=%v", s, err)
	}
	if gotName != "systemctl" || len(gotArgs) != 2 || gotArgs[0] != "is-active" {
		t.Fatalf("shExec args = %v %v", gotName, gotArgs)
	}
	if isActive() != "active" {
		t.Fatalf("isActive = %q", isActive())
	}
	shExec = func(name string, args ...string) ([]byte, error) {
		return []byte("enabled\n"), nil
	}
	if isEnabled() != "enabled" {
		t.Fatalf("isEnabled = %q", isEnabled())
	}
	shExec = func(name string, args ...string) ([]byte, error) {
		return []byte("1234\n"), nil
	}
	if mainPID() != "1234" {
		t.Fatalf("mainPID = %q", mainPID())
	}
	// 失败路径：错误与 stdout 原样透传（生产 main() 用 s, _ := shOut(...)
	// 忽略 err——is-active/is-enabled 对 inactive/disabled 退出码本就非 0）
	shExec = func(name string, args ...string) ([]byte, error) {
		return []byte("inactive\n"), errStr("exit 3")
	}
	if s, err := shOut("systemctl", "is-active", *unit); err == nil || s != "inactive" {
		t.Fatalf("shOut err path: s=%q err=%v", s, err)
	}
	// 空输出（TrimSpace）
	shExec = func(name string, args ...string) ([]byte, error) {
		return []byte("  \n"), nil
	}
	if s, _ := shOut("systemctl", "is-enabled", *unit); s != "" {
		t.Fatalf("shOut empty: s=%q", s)
	}

	// 生产默认注入体：真 echo 走一遍（stdout 为准 + TrimSpace；echo 恒 0 退出）
	shExec = defaultShExec
	if s, err := shOut("echo", "  active  "); err != nil || s != "active" {
		t.Fatalf("default shExec: s=%q err=%v", s, err)
	}
}

// ---------- 服务列表 ----------

func TestListServicesAndFind(t *testing.T) {
	resetGlobals(t)
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents/a1/services", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"services":[` +
				`{"name":"cockpit-acc-ghost.service","loadState":"loaded","activeState":"inactive","subState":"dead","unitFileState":"disabled"},` +
				`{"name":"cockpit-acc-svc.service","loadState":"loaded","activeState":"active","subState":"running","unitFileState":"enabled"}` +
				`]}`))
		})
	})
	list, code, msg := listServices("a1")
	if code != 200 || msg != "" || len(list) != 2 {
		t.Fatalf("list: code=%d msg=%q n=%d", code, msg, len(list))
	}
	g := findUnit(list, "cockpit-acc-ghost.service")
	if g == nil || g.ActiveState != "inactive" || g.UnitFileState != "disabled" {
		t.Fatalf("ghost = %+v", g)
	}
	if findUnit(list, "nope.service") != nil {
		t.Fatal("findUnit absent: want nil")
	}

	// 非 200 → (nil, code, 截断体：200 字 + "...")
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents/a1/services", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(502)
			_, _ = w.Write([]byte(strings.Repeat("x", 500)))
		})
	})
	list, code, msg = listServices("a1")
	if list != nil || code != 502 || len(msg) != 203 || !strings.HasSuffix(msg, "...") {
		t.Fatalf("non-200: list=%v code=%d msgLen=%d", list, code, len(msg))
	}

	// 坏 JSON → bad response
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents/a1/services", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		})
	})
	if _, code, msg = listServices("a1"); code != 200 || !strings.Contains(msg, "bad response") {
		t.Fatalf("bad json: code=%d msg=%q", code, msg)
	}
}
