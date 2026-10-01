// guac 探针 helpers 单测（覆盖率收口：包 0% → helpers 全覆盖）。
//
// 场景编排 main()（S1-S30，真机 guacd/sshd/VNC/容器栈）按验收惯例走真机
// 探针 + .acceptance 证据，不在此单测面内（同 darwin launchd / windows SCM
// 执行层的结构性边界）。本文件覆盖全部可确定性验证的协议/会话/侧信道逻辑：
//
//   - 指令编解码：guacEncode / parseInstr（含 0.,<uuid>; 内部首指令形态）
//   - REST 层：api / login / createTicket / auditSample（httptest 假 cockpit）
//   - WS 会话：dialGuac / readLoop / pingLoop / send / waitReady /
//     waitClipboardData / waitForOpcode / typeText / typeLine / count /
//     totalOps / close（本地假网关：gorilla Upgrader 回放 UUID/ready/error/
//     disconnect/blob/sync/clipboard 流）
//   - 客户端义务（blob→ack、sync→回声）与 clipboard 流旁路拼装（含坏 b64、
//     错 idx 忽略）
//   - 侧信道：dockerExec（注入）/ hostReadFile / fileReadCmd / markerPath
//   - 证据层：ev / check / fatal / sceneWanted / truncate / marshalShort
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
	t.Cleanup(func() {
		evMu.Lock() // evFile 会被泄漏窗口期的 readLoop 经 ev() 读，清空须同锁
		token, fails, passes, evFile = "", 0, 0, nil
		evMu.Unlock()
		*only, *apiBase, *container = "", "http://127.0.0.1:19990", "cockpit-acc-sshd"
		loginRetryWait = 500 * time.Millisecond
		pingInterval = 3 * time.Second
		dialFirstInstrWait = 10 * time.Second
		dockerExecCmd = func(args ...string) ([]byte, error) {
			return nil, fmt.Errorf("docker not available in unit test")
		}
	})
}

// fakeCockpit 假 cockpit 实例：REST（登录/票据/审计）与 WS 网关同一端口，
// 与真机拓扑一致（dialGuac 从 apiBase 推导 ws 地址）
type fakeCockpit struct {
	srv    *httptest.Server
	recv   chan string      // 客户端→服务端的原始指令（已编码）
	login  http.HandlerFunc // nil → 默认出 token
	ticket http.HandlerFunc // nil → 默认出票
	onWS   func(send func(opcode string, args ...string), closeFn func())
}

func newFakeCockpit(t *testing.T, f *fakeCockpit) *fakeCockpit {
	t.Helper()
	if f.recv == nil {
		f.recv = make(chan string, 512)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", orDefault(f.login, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "fake-token-1"})
	}))
	mux.HandleFunc("/api/remote/tickets", orDefault(f.ticket, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"ticket": fmt.Sprintf("tk-%d", time.Now().UnixNano())})
	}))
	mux.HandleFunc("/api/admin/audit/logs", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("A", 500)))
	})
	mux.HandleFunc("/api/remote/guacamole", func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		var wmu sync.Mutex
		send := func(opcode string, args ...string) {
			wmu.Lock()
			defer wmu.Unlock()
			_ = c.WriteMessage(websocket.TextMessage, []byte(guacEncode(opcode, args...)))
		}
		closeFn := func() {
			wmu.Lock()
			defer wmu.Unlock()
			_ = c.Close()
		}
		if f.onWS != nil {
			go f.onWS(send, closeFn)
		}
		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			select {
			case f.recv <- string(data):
			default:
			}
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	*apiBase = f.srv.URL
	return f
}

func orDefault(h, def http.HandlerFunc) http.HandlerFunc {
	if h != nil {
		return h
	}
	return def
}

// recvOne 带超时取一条客户端→服务端指令
func recvOne(t *testing.T, ch chan string) string {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(3 * time.Second):
		t.Fatalf("等待客户端指令超时")
		return ""
	}
}

// stdGateway 标准网关行为：内部 UUID → ready
func stdGateway(send func(opcode string, args ...string), _ func()) {
	send("", "11111111-2222-3333-4444-555555555555")
	send("ready")
	send("nop", "1") // 供 waitForOpcode 命中路径消费
}

// uuidOnlyGateway 只发内部 UUID（ping/静默类用例）
func uuidOnlyGateway(send func(opcode string, args ...string), _ func()) {
	send("", "uuid-x")
}

// ---------- 纯函数 ----------

func TestGuacEncodeAndParseInstr(t *testing.T) {
	resetGlobals(t)
	if got := guacEncode("key", "97", "1"); got != "3.key,2.97,1.1;" {
		t.Fatalf("guacEncode = %q", got)
	}
	// 内部首指令形态：空 opcode + 单元素（dialGuac 依赖该形态取会话 UUID）
	if got := guacEncode("", "uuid-42"); got != "0.,7.uuid-42;" {
		t.Fatalf("guacEncode empty-op = %q", got)
	}
	cases := []struct {
		raw  string
		op   string
		args []string
		ok   bool
	}{
		{"3.key,2.97,1.1;", "key", []string{"97", "1"}, true},
		{"0.,7.uuid-42;", "", []string{"uuid-42"}, true},
		{"4.sync;", "sync", []string{}, true},
		{"6.ready;", "ready", []string{}, true},
		{";", "", nil, false},        // 空
		{"noplain;", "", nil, false}, // 无长度前缀点号
	}
	for _, c := range cases {
		in, ok := parseInstr([]byte(c.raw))
		if ok != c.ok {
			t.Fatalf("parseInstr(%q) ok=%v want %v", c.raw, ok, c.ok)
		}
		if !ok {
			continue
		}
		if in.Opcode != c.op || len(in.Args) != len(c.args) {
			t.Fatalf("parseInstr(%q) = %+v want op=%q args=%v", c.raw, in, c.op, c.args)
		}
		for i := range c.args {
			if in.Args[i] != c.args[i] {
				t.Fatalf("parseInstr(%q) arg[%d]=%q want %q", c.raw, i, in.Args[i], c.args[i])
			}
		}
	}
}

func TestSceneWantedTruncateMarshal(t *testing.T) {
	resetGlobals(t)
	if !sceneWanted("S1 anything") || !sceneWanted("S2") {
		t.Fatal("空过滤应全跑")
	}
	*only = "S1"
	if !sceneWanted("S1 口令") || sceneWanted("S2 私钥") {
		t.Fatal("-s 子串过滤语义错误")
	}
	if truncate("abc", 5) != "abc" || truncate("abcdef", 5) != "abcde..." {
		t.Fatal("truncate 边界错误")
	}
	short := marshalShort(map[string]int{"a": 1})
	if !strings.Contains(short, `"a":1`) {
		t.Fatalf("marshalShort = %q", short)
	}
	if got := marshalShort(map[string]string{"k": strings.Repeat("x", 400)}); len(got) != 303 {
		t.Fatalf("marshalShort 未按 300 截断：len=%d", len(got))
	}
}

func TestWsBaseMarkerPathFileReadCmd(t *testing.T) {
	resetGlobals(t)
	*apiBase = "http://127.0.0.1:19990"
	if wsBase() != "ws://127.0.0.1:19990" {
		t.Fatalf("wsBase = %q", wsBase())
	}
	// 现状口径：https 前缀不转换（真机验收恒 http，wss 未用）
	*apiBase = "https://x.example"
	if wsBase() != "https://x.example" {
		t.Fatalf("wsBase(https) = %q", wsBase())
	}
	if markerPath("M") != "/tmp/M.txt" || fileReadCmd("/tmp/M.txt") != "cat /tmp/M.txt" {
		t.Fatal("markerPath/fileReadCmd 拼接错误")
	}
}

// ---------- 证据层 ----------

func TestCheckEvFatal(t *testing.T) {
	resetGlobals(t)
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "probe.log"))
	if err != nil {
		t.Fatal(err)
	}
	evFile = f
	t.Cleanup(func() { f.Close() })

	check("ok-场景", true, "detail-1")
	check("bad-场景", false, "detail-2")
	if passes != 1 || fails != 1 {
		t.Fatalf("计数 passes=%d fails=%d", passes, fails)
	}
	f.Close()
	b, _ := os.ReadFile(filepath.Join(dir, "probe.log"))
	body := string(b)
	for _, want := range []string{"[PASS] ok-场景", "[FAIL] bad-场景", "detail-1", "detail-2"} {
		if !strings.Contains(body, want) {
			t.Fatalf("证据缺 %q：%s", want, body)
		}
	}

	// fatal：注入 osExit（panic 捕获）验证退出码 2 且证据落盘
	osExit = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("fatal 未触发 osExit")
		} else if r.(int) != 2 {
			t.Fatalf("退出码 = %v want 2", r)
		}
	}()
	fatal("致命错误 %d", 42)
}

// ---------- REST 层 ----------

func TestAPIHeadersBodyAndUnreachable(t *testing.T) {
	resetGlobals(t)
	var gotAuth, gotCT, gotBody string
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		buf := make([]byte, 64)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	*apiBase = srv.URL
	st, body := api(http.MethodGet, "/x", nil)
	if st != 200 || string(body) != `{"ok":true}` || gotAuth != "" || gotMethod != http.MethodGet {
		t.Fatalf("GET st=%d body=%q auth=%q", st, body, gotAuth)
	}
	token = "tok-9"
	st, _ = api(http.MethodPost, "/x", map[string]string{"k": "v"})
	if st != 200 || gotAuth != "Bearer tok-9" || gotCT != "application/json" {
		t.Fatalf("POST st=%d auth=%q ct=%q", st, gotAuth, gotCT)
	}
	if !strings.Contains(gotBody, `"k":"v"`) {
		t.Fatalf("POST body=%q", gotBody)
	}
	// 不可达 → -1
	*apiBase = "http://127.0.0.1:1"
	if st, _ := api(http.MethodGet, "/x", nil); st != -1 {
		t.Fatalf("不可达应 -1，得 %d", st)
	}
}

func TestAuditSampleTruncation(t *testing.T) {
	resetGlobals(t)
	newFakeCockpit(t, &fakeCockpit{})
	if got := auditSample(); len(got) != 403 { // 500B 响应 → 400 + "..."
		t.Fatalf("auditSample len=%d", len(got))
	}
}

func TestLoginSuccessAndFailureExit(t *testing.T) {
	resetGlobals(t)
	newFakeCockpit(t, &fakeCockpit{})
	login()
	if token != "fake-token-1" {
		t.Fatalf("token=%q", token)
	}

	// 失败路径：20 次重试（间隔注入缩到 1ms）后 fatal 退出
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	*apiBase = srv.URL
	token = ""
	loginRetryWait = time.Millisecond
	osExit = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("登录失败未走 fatal")
		}
	}()
	login()
}

func TestCreateTicketBranches(t *testing.T) {
	resetGlobals(t)
	f := &fakeCockpit{ticket: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}}
	newFakeCockpit(t, f)
	if tk, st, body := createTicket(nil); tk != "" || st != 500 || body != "boom" {
		t.Fatalf("non-200: tk=%q st=%d body=%q", tk, st, body)
	}

	f2 := &fakeCockpit{ticket: func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`)) // 200 但无 ticket 字段
	}}
	newFakeCockpit(t, f2)
	tk, st, msg := createTicket(nil)
	if tk != "" || st != 200 || !strings.Contains(msg, "ticket 缺失") {
		t.Fatalf("missing-ticket: tk=%q st=%d msg=%q", tk, st, msg)
	}

	newFakeCockpit(t, &fakeCockpit{})
	if tk, st, msg := createTicket(map[string]interface{}{"protocol": "ssh"}); tk == "" || st != 200 || msg != "" {
		t.Fatalf("happy: tk=%q st=%d msg=%q", tk, st, msg)
	}
}

// ---------- WS 会话 ----------

func TestDialGuacHappyPathAndCounts(t *testing.T) {
	resetGlobals(t)
	newFakeCockpit(t, &fakeCockpit{onWS: stdGateway})
	gs, err := dialGuac(mustTicket(t))
	if err != nil {
		t.Fatalf("dialGuac: %v", err)
	}
	defer gs.close()
	if gs.sessionID != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("sessionID=%q", gs.sessionID)
	}
	if err := gs.waitReady(2 * time.Second); err != nil {
		t.Fatalf("waitReady: %v", err)
	}
	if got := gs.count("ready"); got != 1 {
		t.Fatalf("count(ready)=%d", got)
	}
	// waitForOpcode 命中路径：ready 已被 waitReady 消费，服务端补发目标指令
	if _, ok := gs.waitForOpcode("nop", 2*time.Second); !ok {
		t.Fatal("waitForOpcode(nop) 未命中")
	}
	if gs.totalOps() < 2 {
		t.Fatalf("totalOps=%d", gs.totalOps())
	}
}

func mustTicket(t *testing.T) string {
	t.Helper()
	tk, st, msg := createTicket(map[string]interface{}{"protocol": "ssh"})
	if st != 200 {
		t.Fatalf("建票据失败: %s", msg)
	}
	return tk
}

func TestDialGuacFailures(t *testing.T) {
	resetGlobals(t)

	// WS 端点不存在（升级失败）→ dial 错误
	mux := http.NewServeMux()
	mux.HandleFunc("/api/remote/tickets", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ticket":"tk"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	*apiBase = srv.URL
	if _, err := dialGuac("tk"); err == nil {
		t.Fatal("升级失败应报错")
	}

	// 首条指令非内部 UUID 形态
	newFakeCockpit(t, &fakeCockpit{onWS: func(send func(string, ...string), _ func()) {
		send("sync", "1")
	}})
	gs, err := dialGuac(mustTicket(t))
	gsClose(gs) // 错误返回的会话同样要收口（pingStop 不关 → pingLoop goroutine 泄漏）
	if err == nil || !strings.Contains(err.Error(), "非内部 UUID") {
		t.Fatalf("首指令非 UUID 应报错，得 %v", err)
	}

	// 服务端建连即断 → errCh 分支
	newFakeCockpit(t, &fakeCockpit{onWS: func(send func(string, ...string), closeFn func()) {
		closeFn()
	}})
	gs, err = dialGuac(mustTicket(t))
	gsClose(gs)
	if err == nil {
		t.Fatal("建连即断应报错")
	}

	// 首指令超时（等待窗口注入缩到 50ms，服务端静默）
	dialFirstInstrWait = 50 * time.Millisecond
	newFakeCockpit(t, &fakeCockpit{onWS: func(func(string, ...string), func()) {}})
	gs, err = dialGuac(mustTicket(t))
	gsClose(gs)
	if err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("首指令超时应报错，得 %v", err)
	}
}

func gsClose(gs *guacSession) {
	if gs != nil {
		gs.close()
	}
}

func TestWaitReadyErrorDisconnectAndDrop(t *testing.T) {
	resetGlobals(t)

	// error 指令
	newFakeCockpit(t, &fakeCockpit{onWS: func(send func(string, ...string), _ func()) {
		send("", "uuid-x")
		send("error", "7", "认证失败")
	}})
	gs, err := dialGuac(mustTicket(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.waitReady(2 * time.Second); err == nil || !strings.Contains(err.Error(), "guacd error") {
		gs.close()
		t.Fatalf("error 指令应报错，得 %v", err)
	}
	gs.close()

	// disconnect 指令
	newFakeCockpit(t, &fakeCockpit{onWS: func(send func(string, ...string), _ func()) {
		send("", "uuid-x")
		send("disconnect", "3", "bye")
	}})
	gs2, _ := dialGuac(mustTicket(t))
	if err := gs2.waitReady(2 * time.Second); err == nil || !strings.Contains(err.Error(), "disconnect") {
		gs2.close()
		t.Fatalf("disconnect 应报错，得 %v", err)
	}
	gs2.close()

	// 服务端中途断开 → ws 断开分支
	newFakeCockpit(t, &fakeCockpit{onWS: func(send func(string, ...string), closeFn func()) {
		send("", "uuid-x")
		time.Sleep(100 * time.Millisecond)
		closeFn()
	}})
	gs3, _ := dialGuac(mustTicket(t))
	if err := gs3.waitReady(3 * time.Second); err == nil || !strings.Contains(err.Error(), "ws 断开") {
		gs3.close()
		t.Fatalf("断开应报错，得 %v", err)
	}
	gs3.close()
}

// TestWaitHelpersErrChAndDeadline 等待族收尾分支：waitClipboardData 与
// waitForOpcode 各自的 errCh（ws 断开）与 deadline（超时）分支——errCh 只
// 缓冲读循环的一次错误，两条会话按相反顺序消费，四个分支全走到
func TestWaitHelpersErrChAndDeadline(t *testing.T) {
	resetGlobals(t)

	// 会话 A：先 waitClipboardData（errCh 分支），再 waitForOpcode（deadline 分支）
	newFakeCockpit(t, &fakeCockpit{onWS: func(send func(string, ...string), closeFn func()) {
		send("", "uuid-a")
		time.Sleep(100 * time.Millisecond)
		closeFn()
	}})
	gsA, err := dialGuac(mustTicket(t))
	if err != nil {
		t.Fatal(err)
	}
	defer gsA.close()
	if _, ok := gsA.waitClipboardData(2 * time.Second); ok {
		t.Fatal("ws 断开后 waitClipboardData 应 false（errCh）")
	}
	if _, ok := gsA.waitForOpcode("ready", 100*time.Millisecond); ok {
		t.Fatal("errCh 已消费，waitForOpcode 应走 deadline false")
	}

	// 会话 B：反序（waitForOpcode errCh 分支 → waitClipboardData deadline 分支）
	newFakeCockpit(t, &fakeCockpit{onWS: func(send func(string, ...string), closeFn func()) {
		send("", "uuid-b")
		time.Sleep(100 * time.Millisecond)
		closeFn()
	}})
	gsB, err := dialGuac(mustTicket(t))
	if err != nil {
		t.Fatal(err)
	}
	defer gsB.close()
	if _, ok := gsB.waitForOpcode("ready", 2*time.Second); ok {
		t.Fatal("ws 断开后 waitForOpcode 应 false（errCh）")
	}
	if _, ok := gsB.waitClipboardData(100 * time.Millisecond); ok {
		t.Fatal("errCh 已消费，waitClipboardData 应走 deadline false")
	}

	// waitReady 超时分支（UUID 后静默网关）
	newFakeCockpit(t, &fakeCockpit{onWS: uuidOnlyGateway})
	gsC, err := dialGuac(mustTicket(t))
	if err != nil {
		t.Fatal(err)
	}
	defer gsC.close()
	if err := gsC.waitReady(100 * time.Millisecond); err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("静默网关 waitReady 应超时报错，得 %v", err)
	}
}

// TestOpenSSHSessionWSFail 票据成功但 WS 隧道不可达 → check FAIL + nil
func TestOpenSSHSessionWSFail(t *testing.T) {
	resetGlobals(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/remote/tickets", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ticket":"tk"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	*apiBase = srv.URL
	failsBefore := fails
	if gs := openSSHSession("WS 失败场景", nil); gs != nil || fails != failsBefore+1 {
		t.Fatalf("WS 失败应 nil 且记 FAIL：gs=%v fails=%d→%d", gs, failsBefore, fails)
	}
}

func TestClientObligationsAckAndSyncEcho(t *testing.T) {
	resetGlobals(t)
	f := newFakeCockpit(t, &fakeCockpit{onWS: func(send func(string, ...string), _ func()) {
		send("", "uuid-x")
		send("ready")
		send("blob", "5", "aGVsbG8=") // img/blob 流 → 客户端必须 ack
		send("sync", "1699999999000")
	}})
	_ = f
	gs, err := dialGuac(mustTicket(t))
	if err != nil {
		t.Fatal(err)
	}
	defer gs.close()
	if err := gs.waitReady(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	// ack.<idx>,OK,0
	ack := recvOne(t, f.recv)
	if ack != "3.ack,1.5,2.OK,1.0;" {
		t.Fatalf("ack 指令 = %q", ack)
	}
	// sync 回声（同时间戳）
	if got := recvOne(t, f.recv); got != "4.sync,13.1699999999000;" {
		t.Fatalf("sync 回声 = %q", got)
	}
}

func TestClipboardStreamEndToEndAndEdge(t *testing.T) {
	resetGlobals(t)
	payload := "hello-clipboard-世界-42"
	b64 := base64.StdEncoding.EncodeToString([]byte(payload))
	half := b64[:len(b64)/2]
	rest := b64[len(b64)/2:]
	newFakeCockpit(t, &fakeCockpit{onWS: func(send func(string, ...string), _ func()) {
		send("", "uuid-x")
		send("ready")
		send("clipboard", "9") // 开流（idx=9）
		send("blob", "9", half)
		send("blob", "9", rest)
		send("end", "9")
	}})
	gs, err := dialGuac(mustTicket(t))
	if err != nil {
		t.Fatal(err)
	}
	defer gs.close()
	if err := gs.waitReady(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	got, ok := gs.waitClipboardData(3 * time.Second)
	if !ok || got != payload {
		t.Fatalf("clipboard 载荷 got=%q ok=%v want %q", got, ok, payload)
	}

	// 错 idx 的 blob/end 不拼装（旁路静默）+ 坏 b64 显性标记
	gs2 := &guacSession{clipCh: make(chan string, 1)}
	gs2.clipboardStream(instr{Opcode: "clipboard", Args: []string{"1"}})
	gs2.clipboardStream(instr{Opcode: "blob", Args: []string{"2", "AAAA"}}) // 非当前 idx
	gs2.clipboardStream(instr{Opcode: "end", Args: []string{"2"}})          // 非当前 idx
	gs2.clipboardStream(instr{Opcode: "blob", Args: []string{"1", "!!notb64!!"}})
	gs2.clipboardStream(instr{Opcode: "end", Args: []string{"1"}})
	got2, ok2 := gs2.waitClipboardData(time.Second)
	if !ok2 || !strings.HasPrefix(got2, "B64_ERR:") {
		t.Fatalf("坏 b64 应给 B64_ERR 标记，got=%q ok=%v", got2, ok2)
	}
	// clipboard 指令零 args（无 idx）不崩
	gs2.clipboardStream(instr{Opcode: "clipboard"})
	// blob/end 零 args 防御
	gs2.clipboardStream(instr{Opcode: "blob"})
	gs2.clipboardStream(instr{Opcode: "end"})
}

func TestTypeTextTypeLineKeysyms(t *testing.T) {
	resetGlobals(t)
	f := newFakeCockpit(t, &fakeCockpit{onWS: stdGateway})
	gs, err := dialGuac(mustTicket(t))
	if err != nil {
		t.Fatal(err)
	}
	defer gs.close()
	if err := gs.waitReady(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	gs.typeText("a")
	pressA := recvOne(t, f.recv)
	if pressA != "3.key,2.97,1.1;" {
		t.Fatalf("key press = %q", pressA)
	}
	if got := recvOne(t, f.recv); got != "3.key,2.97,1.0;" {
		t.Fatalf("key release = %q", got)
	}
	gs.typeLine("") // 仅回车（Enter keysym 0xFF0D=65293）
	if got := recvOne(t, f.recv); got != "3.key,5.65293,1.1;" {
		t.Fatalf("enter press = %q", got)
	}
	if got := recvOne(t, f.recv); got != "3.key,5.65293,1.0;" {
		t.Fatalf("enter release = %q", got)
	}
}

func TestPingLoopHeartbeatAndStop(t *testing.T) {
	resetGlobals(t)
	pingInterval = 30 * time.Millisecond
	var sawPing bool
	f := newFakeCockpit(t, &fakeCockpit{onWS: uuidOnlyGateway})
	gs, err := dialGuac(mustTicket(t))
	if err != nil {
		t.Fatal(err)
	}
	defer gs.close()
	for i := 0; i < 40 && !sawPing; i++ {
		select {
		case s := <-f.recv:
			if strings.Contains(s, "ping") {
				sawPing = true
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
	if !sawPing {
		t.Fatal("pingLoop 未发心跳")
	}
	gs.close() // pingStop → goroutine 退出（竞态器口径下无泄漏写）
	time.Sleep(60 * time.Millisecond)
}

func TestReadLoopCountsAndInstrGate(t *testing.T) {
	resetGlobals(t)
	// 300 条非优先指令：opCounts 全计；instrCh 仅缓冲前 200（读端不消费，
	// len>=200 后丢弃——正是 gate 分支）
	newFakeCockpit(t, &fakeCockpit{onWS: func(send func(string, ...string), _ func()) {
		send("", "uuid-x")
		for i := 0; i < 300; i++ {
			send("nop", fmt.Sprint(i))
		}
	}})
	gs, err := dialGuac(mustTicket(t))
	if err != nil {
		t.Fatal(err)
	}
	defer gs.close()
	deadline := time.Now().Add(3 * time.Second)
	for gs.count("nop") < 300 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := gs.count("nop"); got != 300 {
		t.Fatalf("opCounts(nop)=%d want 300", got)
	}
	if got := gs.totalOps(); got < 301 { // uuid + 300 nop
		t.Fatalf("totalOps=%d", got)
	}
}

func TestCloseIdempotentAndNilSafe(t *testing.T) {
	resetGlobals(t)
	var gs *guacSession
	gs.close() // nil 接收者安全
	newFakeCockpit(t, &fakeCockpit{onWS: stdGateway})
	gs2, err := dialGuac(mustTicket(t))
	if err != nil {
		t.Fatal(err)
	}
	gs2.close()
	gs2.close() // 幂等
}

// ---------- 侧信道与场景 ----------

func TestHostReadFile(t *testing.T) {
	resetGlobals(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "marker.txt")
	if err := os.WriteFile(p, []byte("  MARK-9 \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := hostReadFile(p); got != "MARK-9" {
		t.Fatalf("hostReadFile=%q", got)
	}
	if got := hostReadFile(filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("缺失文件应空串，得 %q", got)
	}
}

func TestDockerExecBranches(t *testing.T) {
	resetGlobals(t)
	*container = "ct-1"
	dockerExecCmd = func(args ...string) ([]byte, error) {
		want := []string{"sh", "-c", "cat /tmp/x"} // exec/container 前缀在注入的命令体内
		if fmt.Sprint(args) != fmt.Sprint(want) {
			return nil, fmt.Errorf("argv mismatch: %v", args)
		}
		return []byte("  out-1 \n"), nil
	}
	if got := dockerExec("sh", "-c", "cat /tmp/x"); got != "out-1" {
		t.Fatalf("dockerExec=%q", got)
	}
	dockerExecCmd = func(args ...string) ([]byte, error) {
		return []byte("no such container"), fmt.Errorf("exit 1")
	}
	if got := dockerExec("cat", "/x"); got != "" {
		t.Fatalf("失败应空串（错误串不得混入断言文本），得 %q", got)
	}
}

func TestOpenSSHSessionBranches(t *testing.T) {
	resetGlobals(t)

	// 票据非 200 → nil + check FAIL
	f := &fakeCockpit{ticket: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}}
	newFakeCockpit(t, f)
	failsBefore := fails
	if gs := openSSHSession("票据失败场景", nil); gs != nil || fails != failsBefore+1 {
		t.Fatalf("票据失败应 nil 且记 FAIL：gs=%v fails=%d→%d", gs, failsBefore, fails)
	}

	// ready 前 error 指令 → nil + check FAIL
	f2 := &fakeCockpit{onWS: func(send func(string, ...string), _ func()) {
		send("", "uuid-x")
		send("error", "7", "auth fail")
	}}
	newFakeCockpit(t, f2)
	failsBefore = fails
	if gs := openSSHSession("ready 失败场景", nil); gs != nil || fails != failsBefore+1 {
		t.Fatalf("ready 失败应 nil 且记 FAIL：gs=%v", gs)
	}

	// 成功路径：默认票据 + 标准网关
	newFakeCockpit(t, &fakeCockpit{onWS: stdGateway})
	gs := openSSHSession("成功场景", map[string]interface{}{"private_key": "K"})
	if gs == nil {
		t.Fatal("成功路径应返回会话")
	}
	gs.close()
}

func TestAuthScenarioFilterFailAndPass(t *testing.T) {
	resetGlobals(t)

	// -s 过滤：场景不跑（无 check 记账）
	*only = "ZZZ-不存在"
	checksBefore := passes + fails
	authScenario("过滤场景", "M", nil, func() string { return "M" })
	if passes+fails != checksBefore {
		t.Fatal("被过滤场景不应记账")
	}

	// 会话失败（票据 500）→ openSSHSession 记 FAIL 后返回
	*only = ""
	f := &fakeCockpit{ticket: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}}
	newFakeCockpit(t, f)
	authScenario("失败场景", "M", nil, func() string { return "M" })
	if fails == 0 {
		t.Fatal("会话失败场景应记 FAIL")
	}

	// 成功路径：输入标记命令 → 侧信道回读命中 → PASS
	newFakeCockpit(t, &fakeCockpit{onWS: stdGateway})
	fails, passes = 0, 0
	authScenario("成功场景", "AUTH_OK_1", map[string]interface{}{"password": "p"}, func() string { return "AUTH_OK_1" })
	if passes != 1 || fails != 0 {
		t.Fatalf("成功场景应 PASS：passes=%d fails=%d", passes, fails)
	}
}
