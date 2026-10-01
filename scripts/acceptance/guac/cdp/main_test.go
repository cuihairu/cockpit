// guac/cdp 探针 helpers 单测（覆盖率收口：包 0% → helpers 全覆盖）。
//
// 场景编排 main()（C1-C7 真机 headless Chrome + Guacamole 登录链路）按验收
// 惯例走真机探针 + .acceptance 证据，不在此单测面内（同 darwin launchd /
// windows SCM 执行层的结构性边界，guac/probe 先例）。本文件覆盖手写 CDP
// 客户端全部可确定性验证的逻辑：证据层 / tab 生命周期 HTTP 面 / call 五态
// （成功·错误·超时·写失败·脏帧）/ eval 六形态 / 交互 helper（fill·click·
// 键盘·导航·截图五分支）——CDP 端点用 gorilla Upgrader 假服务回放。
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ---------- 测试基建 ----------

// resetGlobals 每用例隔离包级状态（flag 变量与注入点还原为默认语义）
func resetGlobals(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		evMu.Lock()
		passes, fails, evFile = 0, 0, nil
		evMu.Unlock()
		*cdpBase = "http://127.0.0.1:9224"
		*evDir = ""
		osExit = os.Exit
		callWait = 30 * time.Second
	})
	*evDir = t.TempDir()
}

// stubExit 记录 fatal 的退出码而不真退出
func stubExit(t *testing.T) *int {
	t.Helper()
	code := new(int)
	osExit = func(c int) { *code = c }
	return code
}

// evalReply 单次 Runtime.evaluate 的回放形态
type evalReply struct {
	value     interface{} // JSON 值（string/bool/number/nil）
	exception string      // 非空 → exceptionDetails
	rawResult string      // 非空 → 直接充当 result（覆盖非对象形态）
}

// cdpFake 假 CDP 端点：/json/new PUT、/json/list、/json/close/*、
// /devtools/page/* WS 回放（全部配置经互斥锁读写，-race 安全）
type cdpFake struct {
	mu         sync.Mutex
	onEval     func(expr string) evalReply
	screenshot string
	methodErr  map[string]string
	skip       map[string]bool
	extras     []string // 回复前先注入的帧（脏帧/事件/未知 ID）
	seen       []string // 收到的 eval 表达式（断言 JS 拼装）
	closed     []string // 被 close 的 tab id
}

var wsUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func (f *cdpFake) setOnEval(fn func(string) evalReply) {
	f.mu.Lock()
	f.onEval = fn
	f.mu.Unlock()
}

func (f *cdpFake) setScreenshot(s string) {
	f.mu.Lock()
	f.screenshot = s
	f.mu.Unlock()
}

func (f *cdpFake) setMethodErr(method, msg string) {
	f.mu.Lock()
	if f.methodErr == nil {
		f.methodErr = map[string]string{}
	}
	f.methodErr[method] = msg
	f.mu.Unlock()
}

func (f *cdpFake) setSkip(method string) {
	f.mu.Lock()
	if f.skip == nil {
		f.skip = map[string]bool{}
	}
	f.skip[method] = true
	f.mu.Unlock()
}

func (f *cdpFake) setExtras(frames ...string) {
	f.mu.Lock()
	f.extras = frames
	f.mu.Unlock()
}

func (f *cdpFake) seenExprs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

func (f *cdpFake) closedTabs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.closed...)
}

// resultFor 按方法生成 CDP result 字段
func (f *cdpFake) resultFor(m cdpMsg) json.RawMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch m.Method {
	case "Runtime.evaluate":
		var p struct {
			Expression string `json:"expression"`
		}
		_ = json.Unmarshal(m.Params, &p)
		f.seen = append(f.seen, p.Expression)
		r := evalReply{value: true}
		if f.onEval != nil {
			r = f.onEval(p.Expression)
		}
		if r.rawResult != "" {
			return json.RawMessage(r.rawResult)
		}
		if r.exception != "" {
			return json.RawMessage(fmt.Sprintf(`{"result":{},"exceptionDetails":{"text":%q}}`, r.exception))
		}
		v, _ := json.Marshal(r.value)
		return json.RawMessage(fmt.Sprintf(`{"result":{"type":"string","value":%s}}`, v))
	case "Page.captureScreenshot":
		// call 返回的就是 result 字段内容——screenshot 直接解 data 键
		if f.screenshot == "" {
			return json.RawMessage(`{}`)
		}
		return json.RawMessage(fmt.Sprintf(`{"data":%q}`, f.screenshot))
	default:
		return json.RawMessage(`{"result":{}}`)
	}
}

func (f *cdpFake) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var m cdpMsg
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		f.mu.Lock()
		extras := f.extras
		f.extras = nil
		skip := f.skip[m.Method]
		merr := f.methodErr[m.Method]
		f.mu.Unlock()
		for _, e := range extras {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(e))
		}
		if skip {
			continue // 不回复 → 调用方走 timeout 分支
		}
		if merr != "" {
			_ = conn.WriteJSON(cdpMsg{ID: m.ID, Error: &cdpErr{Code: -32000, Message: merr}})
			continue
		}
		_ = conn.WriteJSON(cdpMsg{ID: m.ID, Result: f.resultFor(m)})
	}
}

// newFakeCDP 起假 CDP 端点（HTTP 面 + WS 回放）
func newFakeCDP(t *testing.T, f *cdpFake) *httptest.Server {
	t.Helper()
	var addr string
	mux := http.NewServeMux()
	mux.HandleFunc("/json/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":                   "TAB1",
			"webSocketDebuggerUrl": "ws://" + addr + "/devtools/page/TAB1",
		})
	})
	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"TAB1"},{"id":"TAB2"}]`))
	})
	mux.HandleFunc("/json/close/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.closed = append(f.closed, strings.TrimPrefix(r.URL.Path, "/json/close/"))
		f.mu.Unlock()
		_, _ = w.Write([]byte("true"))
	})
	mux.HandleFunc("/devtools/page/", f.serveWS)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	addr = srv.Listener.Addr().String()
	return srv
}

// dial 直连假 CDP 的 WS（不走 newTab）
func (f *cdpFake) dial(t *testing.T, srv *httptest.Server) *cdp {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial(
		"ws://"+srv.Listener.Addr().String()+"/devtools/page/TAB1", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &cdp{ws: ws, pending: map[int]chan cdpMsg{}}
	go c.readLoop()
	t.Cleanup(func() { _ = ws.Close() })
	return c
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

// ---------- tab 生命周期（HTTP 面） ----------

func TestNewTabSuccessAndClose(t *testing.T) {
	resetGlobals(t)
	fake := &cdpFake{}
	srv := newFakeCDP(t, fake)
	*cdpBase = srv.URL

	c, tabID, err := newTab(*cdpBase)
	if err != nil || tabID != "TAB1" || c == nil {
		t.Fatalf("newTab: id=%q err=%v c=%v", tabID, err, c)
	}
	_ = c.ws.Close() // 后续 closeTab/closeAllTabs 走 HTTP 面

	closeTab(*cdpBase, "TAB1")
	closeAllTabs(*cdpBase)
	if got := fake.closedTabs(); len(got) != 3 {
		t.Fatalf("closed tabs = %v, want TAB1 + TAB1 + TAB2", got)
	}
}

func TestNewTabErrorBranches(t *testing.T) {
	resetGlobals(t)

	// 传输失败
	*cdpBase = "http://127.0.0.1:1"
	if _, _, err := newTab(*cdpBase); err == nil || !strings.Contains(err.Error(), "json/new") {
		t.Fatalf("transport: %v", err)
	}
	// 响应非 JSON
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(bad.Close)
	if _, _, err := newTab(bad.URL); err == nil || !strings.Contains(err.Error(), "bad response") {
		t.Fatalf("bad json: %v", err)
	}
	// ws 拨号失败（指向关闭端口）
	badWS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"T","webSocketDebuggerUrl":"ws://127.0.0.1:1/devtools/page/T"}`))
	}))
	t.Cleanup(badWS.Close)
	if _, _, err := newTab(badWS.URL); err == nil || !strings.Contains(err.Error(), "dial devtools ws") {
		t.Fatalf("ws dial: %v", err)
	}
	// Page.enable 报错 → 直接返回错误
	enableErr := &cdpFake{}
	enableErr.setMethodErr("Page.enable", "target closed")
	srv := newFakeCDP(t, enableErr)
	if _, _, err := newTab(srv.URL); err == nil || !strings.Contains(err.Error(), "target closed") {
		t.Fatalf("Page.enable err: %v", err)
	}
}

func TestCloseAllTabsErrorBranches(t *testing.T) {
	resetGlobals(t)
	// 传输失败 → 静默返回
	closeAllTabs("http://127.0.0.1:1")
	// 列表非 JSON → 静默返回
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(bad.Close)
	closeAllTabs(bad.URL)
	// closeTab 传输失败（不 panic）
	closeTab("http://127.0.0.1:1", "T")
	// httpPutJSON 传输失败
	if _, err := httpPutJSON("http://127.0.0.1:1/json/new"); err == nil {
		t.Fatal("httpPutJSON transport: want error")
	}
}

// ---------- call 五态 ----------

func TestCallBranches(t *testing.T) {
	resetGlobals(t)
	fake := &cdpFake{}
	srv := newFakeCDP(t, fake)
	c := fake.dial(t, srv)

	// 成功
	if _, err := c.call("Page.enable", nil); err != nil {
		t.Fatalf("call ok: %v", err)
	}
	if _, err := c.call("Foo.bar", map[string]interface{}{"x": 1}); err != nil {
		t.Fatalf("call with params: %v", err)
	}

	// 错误响应
	fake.setMethodErr("Bad.method", "boom")
	if _, err := c.call("Bad.method", nil); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error resp: %v", err)
	}

	// 超时（注入 callWait + 不回复）
	callWait = 60 * time.Millisecond
	fake.setSkip("Slow.method")
	if _, err := c.call("Slow.method", nil); err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("timeout: %v", err)
	}
	callWait = 30 * time.Second

	// 脏帧：非 JSON / 事件帧（ID=0）/ 未知 ID 回复 —— 均不打断主请求
	fake.setExtras(
		`not-json-frame`,
		`{"method":"Page.loadEventFired","params":{}}`,
		`{"id":9999,"result":{}}`,
	)
	if _, err := c.call("After.garbage", nil); err != nil {
		t.Fatalf("call after garbage frames: %v", err)
	}

	// 写失败：连接已关
	_ = c.ws.Close()
	if _, err := c.call("Page.enable", nil); err == nil {
		t.Fatal("write on closed ws: want error")
	}
}

// ---------- eval 六形态 ----------

func TestEvalForms(t *testing.T) {
	resetGlobals(t)
	fake := &cdpFake{}
	srv := newFakeCDP(t, fake)
	c := fake.dial(t, srv)

	// string
	fake.setOnEval(func(string) evalReply { return evalReply{value: "hello"} })
	if v, err := c.eval("x"); err != nil || v != "hello" {
		t.Fatalf("string: v=%q err=%v", v, err)
	}
	// bool → "true"（waitJS 判等口径）
	fake.setOnEval(func(string) evalReply { return evalReply{value: true} })
	if v, err := c.eval("x"); err != nil || v != "true" {
		t.Fatalf("bool: v=%q err=%v", v, err)
	}
	// 数字 → Sprint
	fake.setOnEval(func(string) evalReply { return evalReply{value: 42} })
	if v, err := c.eval("x"); err != nil || v != "42" {
		t.Fatalf("number: v=%q err=%v", v, err)
	}
	// nil 值 → 空串
	fake.setOnEval(func(string) evalReply { return evalReply{value: nil} })
	if v, err := c.eval("x"); err != nil || v != "" {
		t.Fatalf("nil value: v=%q err=%v", v, err)
	}
	// 异常 → error
	fake.setOnEval(func(string) evalReply { return evalReply{exception: "Uncaught Error: bad"} })
	if _, err := c.eval("x"); err == nil || !strings.Contains(err.Error(), "js exception") {
		t.Fatalf("exception: %v", err)
	}
	// result 非对象 → 反序列化失败
	fake.setOnEval(func(string) evalReply { return evalReply{rawResult: `"just-a-string"`} })
	if _, err := c.eval("x"); err == nil {
		t.Fatal("raw result: want error")
	}
	// call 层错误透传
	fake.setOnEval(func(string) evalReply { return evalReply{value: "v"} })
	fake.setMethodErr("Runtime.evaluate", "evaluate disabled")
	if _, err := c.eval("x"); err == nil || !strings.Contains(err.Error(), "evaluate disabled") {
		t.Fatalf("call err: %v", err)
	}
	fake.setMethodErr("Runtime.evaluate", "")

	// waitJS 真路径 / 假路径
	fake.setOnEval(func(string) evalReply { return evalReply{value: true} })
	if !c.waitJS("document.readyState==='complete'", time.Second) {
		t.Fatal("waitJS true: want true")
	}
	fake.setOnEval(func(string) evalReply { return evalReply{value: false} })
	if c.waitJS("document.readyState==='complete'", 120*time.Millisecond) {
		t.Fatal("waitJS false: want false")
	}
}

// ---------- 交互 helper ----------

// defaultEval 按表达式形态派发（假端点跑不了真 JS，用标记识别调用方）
func defaultEval(f *cdpFake) func(string) evalReply {
	return func(expr string) evalReply {
		switch {
		case strings.Contains(expr, "readyState"):
			return evalReply{value: true}
		case strings.Contains(expr, "el.click()"): // clickSel（含 .ant-modal-close 选择器）
			return evalReply{value: "OK"}
		case strings.Contains(expr, ".ant-modal"): // closeModal 的 waitJS
			return evalReply{value: true}
		case strings.Contains(expr, "querySelectorAll('button')"):
			return evalReply{value: "OK"}
		case strings.Contains(expr, "return 'NOEL'"):
			return evalReply{value: "OK"}
		default:
			return evalReply{value: true}
		}
	}
}

func TestFillAndClicks(t *testing.T) {
	resetGlobals(t)
	fake := &cdpFake{}
	fake.setOnEval(defaultEval(fake))
	srv := newFakeCDP(t, fake)
	c := fake.dial(t, srv)

	// fill 成功 + JS 拼装（选择器与值 JSON 编码嵌入）
	if !c.fill("#user", "ad`min\"") {
		t.Fatal("fill ok: want true")
	}
	exprs := fake.seenExprs()
	last := exprs[len(exprs)-1]
	if !strings.Contains(last, `"#user"`) || !strings.Contains(last, "ad`min\\\"") {
		t.Fatalf("fill expr = %s", last)
	}
	// fill NOEL → false
	fake.setOnEval(func(string) evalReply { return evalReply{value: "NOEL"} })
	if c.fill("#missing", "v") {
		t.Fatal("fill NOEL: want false")
	}
	// fill eval 报错 → false
	fake.setOnEval(func(string) evalReply { return evalReply{exception: "boom"} })
	if c.fill("#user", "v") {
		t.Fatal("fill err: want false")
	}

	// clickBtnByText 成功：两端去空白后的文本进 JS
	fake.setOnEval(defaultEval(fake))
	if !c.clickBtnByText("连 接") {
		t.Fatal("clickBtn ok: want true")
	}
	exprs = fake.seenExprs()
	last = exprs[len(exprs)-1]
	if !strings.Contains(last, `"连接"`) || strings.Contains(last, `"连 接"`) {
		t.Fatalf("clickBtn expr = %s", last)
	}
	// NOBTN → false
	fake.setOnEval(func(string) evalReply { return evalReply{value: "NOBTN"} })
	if c.clickBtnByText("不存在") {
		t.Fatal("clickBtn NOBTN: want false")
	}

	// clickSel 成功 / NOEL
	fake.setOnEval(defaultEval(fake))
	if !c.clickSel(".ant-modal-footer button") {
		t.Fatal("clickSel ok: want true")
	}
	fake.setOnEval(func(string) evalReply { return evalReply{value: "NOEL"} })
	if c.clickSel("#nope") {
		t.Fatal("clickSel NOEL: want false")
	}
}

func TestTypeTextAndPressEnter(t *testing.T) {
	resetGlobals(t)
	fake := &cdpFake{}
	fake.setOnEval(defaultEval(fake))
	srv := newFakeCDP(t, fake)
	c := fake.dial(t, srv)

	// 普通字符 / 空格 / 换行 三形态 + pressEnter
	c.typeText("a b\n")
	c.pressEnter()
	// Input 报错 → WARN 分支（keyDown/keyUp 各一条）
	fake.setMethodErr("Input.dispatchKeyEvent", "no focus")
	c.typeText("x")
}

func TestNavigateBranches(t *testing.T) {
	resetGlobals(t)
	fake := &cdpFake{}
	fake.setOnEval(defaultEval(fake))
	srv := newFakeCDP(t, fake)
	c := fake.dial(t, srv)

	// 成功（waitJS 真 + React 挂载等待）
	c.navigate("http://127.0.0.1:18180/")

	// navigate 报错 → fatal（注入桩），其后流程不中断地走完
	code := stubExit(t)
	fake.setMethodErr("Page.navigate", "net::ERR")
	c.navigate("http://nope/")
	if *code != 2 {
		t.Fatalf("navigate fatal exit = %d, want 2", *code)
	}
}

func TestScreenshotFiveBranches(t *testing.T) {
	resetGlobals(t)
	fake := &cdpFake{}
	srv := newFakeCDP(t, fake)
	c := fake.dial(t, srv)
	good := base64.StdEncoding.EncodeToString([]byte("PNGDATA"))

	// ① call 报错
	fake.setMethodErr("Page.captureScreenshot", "no surface")
	c.screenshot("a.png")
	// ② 结果为空
	fake.setMethodErr("Page.captureScreenshot", "")
	fake.setScreenshot("")
	c.screenshot("b.png")
	// ③ base64 解码失败
	fake.setScreenshot("!!!not-base64!!!")
	c.screenshot("c.png")
	// ④ 写入失败（目录不存在）
	fake.setScreenshot(good)
	*evDir = filepath.Join(t.TempDir(), "no-such-dir")
	c.screenshot("d.png")
	// ⑤ 成功
	*evDir = t.TempDir()
	c.screenshot("e.png")
	b, err := os.ReadFile(filepath.Join(*evDir, "e.png"))
	if err != nil || string(b) != "PNGDATA" {
		t.Fatalf("screenshot file: %v %q", err, b)
	}
}

func TestCloseModalAndWaitJS(t *testing.T) {
	resetGlobals(t)
	fake := &cdpFake{}
	fake.setOnEval(defaultEval(fake))
	srv := newFakeCDP(t, fake)
	c := fake.dial(t, srv)
	c.closeModal()
}
