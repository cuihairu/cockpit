// traefik 探针 helpers 单测（覆盖率收口：包 0% → helpers 全覆盖）。
//
// 场景编排 main()（T1-T10 真机反代链路、热加载/冻结、TLS 证书加载）
// 按验收惯例走真机探针 + .acceptance 证据，不在此单测面内（同 darwin
// launchd / windows SCM 执行层的结构性边界，guac/probe 先例）。本文件覆盖
// 全部可确定性验证的逻辑：证据层 / cockpit REST（httptest 假实例）/
// Traefik admin API 轮询族（httptest 假 admin）/ 经反代的端到端请求
// （假 web entrypoint + 证书客户端三态）/ 站点操作。
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
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
		*apiBase = "http://127.0.0.1:19991"
		*traefikAPI = "http://127.0.0.1:18181"
		*webEP = "http://127.0.0.1:18180"
		*agentID = "traefik-acc-agent"
		*evDir = ""
		tlsC = nil
		osExit = os.Exit
	})
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
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "tk-tf-1"})
	})
	if h != nil {
		h(mux)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	*apiBase = srv.URL
	return srv
}

// fakeAdmin 假 Traefik admin API（/api/http/{routers,services}）
func fakeAdmin(t *testing.T, h func(mux *http.ServeMux)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	if h != nil {
		h(mux)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	*traefikAPI = srv.URL
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

// ---------- cockpit REST ----------

func TestRestLoginAndCapabilities(t *testing.T) {
	resetGlobals(t)
	var gotAuth string
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/echo", func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"ok":true}`))
		})
		mux.HandleFunc("/api/err", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(502)
			_, _ = w.Write([]byte(`{"error":"agent unavailable"}`))
		})
		mux.HandleFunc("/api/rawerr", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			_, _ = w.Write([]byte("not-json"))
		})
	})

	// login 成功
	login()
	evMu.Lock()
	tok := token
	evMu.Unlock()
	if tok != "tk-tf-1" {
		t.Fatalf("token = %q", tok)
	}

	// rest 200 → map；带 token 头
	m, err := rest(http.MethodGet, "/api/echo", nil)
	if err != nil || m["ok"] != true || gotAuth != "Bearer tk-tf-1" {
		t.Fatalf("rest ok: m=%v err=%v auth=%q", m, err, gotAuth)
	}
	// 非 200 + error 字段 → 摘要错误
	if _, err = rest(http.MethodGet, "/api/err", nil); err == nil ||
		!strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "agent unavailable") {
		t.Fatalf("rest error field: %v", err)
	}
	// 非 200 + 非 JSON 体 → 原文截断
	if _, err = rest(http.MethodGet, "/api/rawerr", nil); err == nil ||
		!strings.Contains(err.Error(), "not-json") {
		t.Fatalf("rest raw err: %v", err)
	}
	// 传输失败（rest 拼 *apiBase+path，故置坏基址）
	*apiBase = "http://127.0.0.1:1"
	if _, err = rest(http.MethodGet, "/api/x", nil); err == nil ||
		!strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("rest transport: %v", err)
	}
}

func TestLoginFatalPaths(t *testing.T) {
	resetGlobals(t)
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

func TestAgentCapabilities(t *testing.T) {
	resetGlobals(t)
	// 裸数组：命中 / 缺 id
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("[" +
				`{"id":"traefik-acc-agent","capabilities":[{"type":"nginx-proxy"},{"type":"proxy"}]},` +
				`{"id":"other","capabilities":[]}` +
				"]"))
		})
	})
	caps, err := agentCapabilities()
	if err != nil || len(caps) != 2 {
		t.Fatalf("caps: %v err=%v", caps, err)
	}
	// 缺 id
	*agentID = "ghost"
	if _, err = agentCapabilities(); err == nil {
		t.Fatal("missing id: want error")
	}
	*agentID = "traefik-acc-agent"

	// {agents:[..]} 包装形态
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"agents":[{"id":"traefik-acc-agent","capabilities":[{"type":"proxy"}]}]}`))
		})
	})
	if caps, err = agentCapabilities(); err != nil || len(caps) != 1 {
		t.Fatalf("wrapper: %v err=%v", caps, err)
	}

	// 坏响应
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		})
	})
	if _, err = agentCapabilities(); err == nil {
		t.Fatal("bad json: want error")
	}
}

func TestDriftStatus(t *testing.T) {
	resetGlobals(t)
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents/traefik-acc-agent/drift/check", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"items":[` +
				`{"kind":"nginx","name":"x","status":"clean"},` +
				`{"kind":"traefik","name":"s1","status":"dirty"},` +
				`{"kind":"traefik","name":"s2","status":"clean"}` +
				`]}`))
		})
	})
	if s, err := driftStatus("s1"); err != nil || s != "dirty" {
		t.Fatalf("drift s1: %q %v", s, err)
	}
	if s, err := driftStatus("s9"); err != nil || s != "" {
		t.Fatalf("drift absent: %q %v", s, err)
	}
	// rest 出错 → 透传
	*apiBase = "http://127.0.0.1:1"
	if _, err := driftStatus("s1"); err == nil {
		t.Fatal("drift transport: want error")
	}
}

// ---------- Traefik admin API ----------

func TestTraefikAPIGetAndRouters(t *testing.T) {
	resetGlobals(t)
	fakeAdmin(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/http/routers", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[` +
				`{"name":"s1@file","rule":"Host(` + "`" + `a.test` + "`" + `)"},` +
				`{"name":"s2@file","rule":"Host(` + "`" + `b.test` + "`" + `)"}` +
				`]`))
		})
		mux.HandleFunc("/api/http/services", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[` +
				`{"name":"s1@file","loadBalancer":{"servers":[{"url":"http://127.0.0.1:19001"}]}},` +
				`{"name":"bare@file"}` +
				`]`))
		})
		mux.HandleFunc("/api/bad", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		})
	})

	// traefikAPIGet 成功 / 坏 JSON
	var routers []map[string]interface{}
	if err := traefikAPIGet("/api/http/routers", &routers); err != nil || len(routers) != 2 {
		t.Fatalf("get: %v n=%d", err, len(routers))
	}
	if err := traefikAPIGet("/api/bad", &routers); err == nil {
		t.Fatal("bad json: want error")
	}

	// waitRouter 命中（rule 子串 + @ 前缀）
	if !waitRouter("s1", "a.test", time.Second) {
		t.Fatal("waitRouter hit: want true")
	}
	// rule 不匹配 → 超时 false
	if waitRouter("s1", "zzz", 40*time.Millisecond) {
		t.Fatal("waitRouter rule mismatch: want false")
	}
	// 名字前缀不匹配 → 超时 false
	if waitRouter("nomatch", "a.test", 40*time.Millisecond) {
		t.Fatal("waitRouter name mismatch: want false")
	}

	// waitRouterGone：s3 不存在 → 立即 true；s1 存在 → 超时 false
	if !waitRouterGone("s3", time.Second) {
		t.Fatal("waitRouterGone absent: want true")
	}
	if waitRouterGone("s1", 40*time.Millisecond) {
		t.Fatal("waitRouterGone present: want false")
	}

	// serviceServerURL：命中 / 无 loadBalancer / 名字不中
	if u := serviceServerURL("s1"); u != "http://127.0.0.1:19001" {
		t.Fatalf("serviceServerURL = %q", u)
	}
	if u := serviceServerURL("bare"); u != "" {
		t.Fatalf("no loadBalancer: %q", u)
	}
	if u := serviceServerURL("nomatch"); u != "" {
		t.Fatalf("name mismatch: %q", u)
	}

	// waitServiceURL 立即命中
	if u := waitServiceURL("s1", "19001", time.Second); u == "" {
		t.Fatal("waitServiceURL hit: want url")
	}
	// 超时路径 → 末次探测结果
	if u := waitServiceURL("s1", "nomatch-substr", 40*time.Millisecond); u != "http://127.0.0.1:19001" {
		t.Fatalf("waitServiceURL timeout = %q", u)
	}

	// admin 不可达 → 各自返回空/false
	*traefikAPI = "http://127.0.0.1:1"
	if err := traefikAPIGet("/api/http/routers", &routers); err == nil {
		t.Fatal("admin transport: want error")
	}
	if waitRouter("s1", "a.test", 40*time.Millisecond) {
		t.Fatal("waitRouter admin down: want false")
	}
	if waitRouterGone("s1", 40*time.Millisecond) {
		t.Fatal("waitRouterGone admin down: want false")
	}
	if u := serviceServerURL("s1"); u != "" {
		t.Fatalf("serviceServerURL admin down = %q", u)
	}
}

// ---------- 经 Traefik 的端到端请求 ----------

func TestViaTraefik(t *testing.T) {
	resetGlobals(t)
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		switch r.Host {
		case "miss.test":
			w.WriteHeader(404)
			_, _ = w.Write([]byte("no route"))
		case "redir.test":
			// 308：noRedir.CheckRedirect 返回 ErrUseLastResponse → 原样透传
			w.Header().Set("Location", "/moved")
			w.WriteHeader(http.StatusPermanentRedirect)
		default:
			_, _ = w.Write([]byte("hello from traefik"))
		}
		if r.Host == "redir.test" {
			// 308 + Location：验证 noRedir 的 CheckRedirect（ErrUseLastResponse）
			w.Header().Set("Location", "http://example.invalid/")
			w.WriteHeader(http.StatusPermanentRedirect)
			return
		}
		_, _ = w.Write([]byte("hello from traefik"))
	}))
	t.Cleanup(srv.Close)
	*webEP = srv.URL

	if code, body := viaTraefik("a.test"); code != 200 || !strings.Contains(body, "hello") {
		t.Fatalf("viaTraefik: %d %q", code, body)
	}
	if gotHost != "a.test" {
		t.Fatalf("Host header = %q", gotHost)
	}
	// 404 原样返回
	if code, body := viaTraefik("miss.test"); code != 404 || !strings.Contains(body, "no route") {
		t.Fatalf("viaTraefik 404: %d %q", code, body)
	}
	// 308 不跟随（CheckRedirect → ErrUseLastResponse），断言拿到 308 本体
	if code, _ := viaTraefik("redir.test"); code != http.StatusPermanentRedirect {
		t.Fatalf("viaTraefik 308: %d", code)
	}
	// 真 308+Location：CheckRedirect 返回 ErrUseLastResponse → 原 308 透传不跟随
	if code, _ := viaTraefik("redir.test"); code != http.StatusPermanentRedirect {
		t.Fatalf("viaTraefik redirect: want 308, got %d", code)
	}
	// 传输失败
	*webEP = "http://127.0.0.1:1"
	if code, body := viaTraefik("a.test"); code != -1 || body == "" {
		t.Fatalf("viaTraefik transport: %d %q", code, body)
	}
}

func TestViaTraefikTLSThreeStates(t *testing.T) {
	resetGlobals(t)
	// ① 未初始化 → 明确错误
	if _, _, err := viaTraefikTLS("s2.accept.test", "/"); err == nil ||
		!strings.Contains(err.Error(), "未初始化") {
		t.Fatalf("nil tlsC: %v", err)
	}

	// ② 成功：httptest TLS 证书 + ServerName 钉住 + 拨号固定到假服务
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tls ok"))
	}))
	t.Cleanup(srv.Close)
	tlsC = tlsClientFor(srv)

	if code, body, err := viaTraefikTLS("s2.accept.test", "/"); err != nil || code != 200 ||
		!strings.Contains(body, "tls ok") {
		t.Fatalf("tls success: code=%d body=%q err=%v", code, body, err)
	}

	// ③ 传输失败：拨号到关闭端口
	tlsC = tlsClientFor(nil)
	if _, _, err := viaTraefikTLS("s2.accept.test", "/"); err == nil {
		t.Fatal("tls transport: want error")
	}
}

// tlsClientFor 拨号固定到假 TLS 服务（srv=nil → 关闭端口），跳过证书校验：
// 单测只验 viaTraefikTLS 三态分支，真实证书加载由真机 T 用例覆盖
func tlsClientFor(srv *httptest.Server) *http.Client {
	addr := "127.0.0.1:1"
	if srv != nil {
		addr = srv.Listener.Addr().String()
	}
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "tcp", addr)
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

// ---------- 站点操作 ----------

func TestSiteOps(t *testing.T) {
	resetGlobals(t)
	var gotMethod, gotPath, gotBody string
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents/traefik-acc-agent/proxy/sites", func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			_, _ = w.Write([]byte(`{"sites":[` +
				`{"name":"s1","upstream":"127.0.0.1:3000"},` +
				`{"name":"s2","upstream":"127.0.0.1:3001"}` +
				`]}`))
		})
		mux.HandleFunc("/api/agents/traefik-acc-agent/proxy/sites/s1", func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			if r.Method == http.MethodDelete {
				w.WriteHeader(200)
				return
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		})
		mux.HandleFunc("/api/errsite", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		})
	})

	m, err := applySite("s1", map[string]interface{}{"upstream": "127.0.0.1:3000"})
	if err != nil || m["ok"] != true {
		t.Fatalf("applySite: %v %v", m, err)
	}
	if gotMethod != http.MethodPut || !strings.HasSuffix(gotPath, "/sites/s1") ||
		!strings.Contains(gotBody, `"name":"s1"`) {
		t.Fatalf("applySite req: %s %s body=%s", gotMethod, gotPath, gotBody)
	}

	sites, err := sitesList()
	if err != nil || len(sites) != 2 || sites["s1"]["upstream"] != "127.0.0.1:3000" {
		t.Fatalf("sitesList: %v %v", sites, err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("sitesList method = %s", gotMethod)
	}

	if err = deleteSite("s1"); err != nil {
		t.Fatalf("deleteSite: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("deleteSite method = %s", gotMethod)
	}

	// 错误透传
	*apiBase = "http://127.0.0.1:1"
	if _, err = applySite("s9", map[string]interface{}{}); err == nil {
		t.Fatal("applySite transport: want error")
	}
	if err = deleteSite("s9"); err == nil {
		t.Fatal("deleteSite transport: want error")
	}
	if _, err = sitesList(); err == nil {
		t.Fatal("sitesList transport: want error")
	}
	// sites 字段缺失 → 空 map 不报错
	fakeAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/agents/traefik-acc-agent/proxy/sites", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"other":1}`))
		})
	})
	if sites, err = sitesList(); err != nil || len(sites) != 0 {
		t.Fatalf("sitesList no sites: %v %v", sites, err)
	}
}

// ---------- poll 轮询 ----------

func TestPollViaTraefik(t *testing.T) {
	resetGlobals(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ready now"))
	}))
	t.Cleanup(srv.Close)
	*webEP = srv.URL
	if code, body := pollViaTraefik("a.test", 200, "ready", time.Second); code != 200 ||
		!strings.Contains(body, "ready") {
		t.Fatalf("poll hit: %d %q", code, body)
	}
	// 超时 → 末次结果（wantBody 不匹配）
	if code, _ := pollViaTraefik("a.test", 503, "", 100*time.Millisecond); code != 200 {
		t.Fatalf("poll timeout: %d", code)
	}
}
