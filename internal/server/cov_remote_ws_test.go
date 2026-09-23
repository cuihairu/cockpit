package server

// cov_remote_ws_test.go 覆盖 api_remote.go / api_desktop.go / api_vnc.go 的
// 票据校验、会话生命周期、WebSocket 数据转发与保活循环（done 分支）。
//
// 说明：产品代码用 r.Header.Values("Sec-WebSocket-Protocol")（canonical-safe）读取票据
// 子协议头，而 net/http 会把请求头规范化为 "Sec-Websocket-Protocol"，真实
// 网络请求永远取不到票据（见报告中的疑似产品 bug）。因此这里在进程内直接
// 调用 handler，用 net.Pipe + 可 Hijack 的 ResponseWriter 构造完整
// WebSocket 会话，以覆盖票据校验之后的全部代码。

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// covRemoteSetup 构造远控测试服务（DB/录制目录均在临时目录）
func covRemoteSetup(t *testing.T) *Server {
	t.Helper()
	s := covNewServer(t)
	covWSReady(s)
	s.cfg = &config.Config{
		Database:      &config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "cockpit.db")},
		RemoteControl: &config.RemoteControlConfig{AllowArbitraryTarget: true},
	}
	return s
}

// covRegisterBareAgent 注册一个不自动应答的 agent（Send 由测试同步读取）
func covRegisterBareAgent(t *testing.T, s *Server, agentID string) *Agent {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Hostname = "host-" + agentID
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	t.Cleanup(func() { s.registry.Unregister(agentID) })
	return agent
}

// covTicket 生成指定参数的票据
func covTicket(t *testing.T, s *Server, params map[string]string) string {
	t.Helper()
	ticket, err := s.ticketMgr.GenerateTicket("u1", "covuser", params)
	if err != nil {
		t.Fatalf("generate ticket: %v", err)
	}
	return ticket.ID
}

func covTerminalTicket(t *testing.T, s *Server, agentID string) string {
	return covTicket(t, s, map[string]string{
		"agent_id": agentID, "host": "127.0.0.1", "port": "22", "protocol": "ssh",
	})
}

// covHijackRec 可 Hijack 的 ResponseWriter（进程内驱动 gorilla Upgrade）
type covHijackRec struct {
	hdr  http.Header
	conn net.Conn
	code int
}

func (r *covHijackRec) Header() http.Header { return r.hdr }
func (r *covHijackRec) WriteHeader(c int) {
	if r.code == 0 {
		r.code = c
	}
}
func (r *covHijackRec) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return len(b), nil
}
func (r *covHijackRec) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return r.conn, bufio.NewReadWriter(bufio.NewReader(r.conn), bufio.NewWriter(r.conn)), nil
}

// covExtractHeader 从原始请求字节里大小写不敏感地取一个头部的值
func covExtractHeader(raw []byte, name string) string {
	for _, line := range strings.Split(string(raw), "\r\n") {
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		if strings.EqualFold(line[:idx], name) {
			return strings.TrimSpace(line[idx+1:])
		}
	}
	return ""
}

// covDirectWS 在进程内直接调用 WS handler。wantUpgrade=false 时预期在升级前
// 返回（返回 recorder 状态码）；否则返回完成握手的客户端连接。
//
// net.Pipe 同步无缓冲，时序：客户端 goroutine 写握手请求 → 主 goroutine 逐
// 字节消费请求头并解析出客户端的 Sec-WebSocket-Key → 把 key 注入到直调请求
// （客户端会用它自生成的 key 校验 101 应答，两边必须一致）→ 启动 handler。
func covDirectWS(t *testing.T, handler http.HandlerFunc, path, ticket string, wantUpgrade bool) (*websocket.Conn, *covHijackRec) {
	t.Helper()
	conn, rec, _ := covDirectWSChan(t, handler, path, ticket, wantUpgrade, false)
	return conn, rec
}

// covDirectWSJoined 与 covDirectWS（wantUpgrade=true）相同，另返回 handler
// goroutine 的退出信号。远控 handler 同步跑 keepaliveLoop，其返回即代表
// 会话关闭路径（审计 / DB 回填）已执行完——需要在测试返回前排干后台
// db 写的用例（TempDir RemoveAll 与之竞态会报 directory not empty）应等待它。
func covDirectWSJoined(t *testing.T, handler http.HandlerFunc, path, ticket string) (*websocket.Conn, *covHijackRec, <-chan struct{}) {
	t.Helper()
	return covDirectWSChan(t, handler, path, ticket, true, true)
}

// covWaitHandlerExit 等 handler goroutine 退出（超时 fatal）
func covWaitHandlerExit(t *testing.T, desc string, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("handler did not exit: %s", desc)
	}
}

func covDirectWSChan(t *testing.T, handler http.HandlerFunc, path, ticket string, wantUpgrade, wantDone bool) (*websocket.Conn, *covHijackRec, <-chan struct{}) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	deadline := time.Now().Add(10 * time.Second)
	clientConn.SetDeadline(deadline)
	serverConn.SetDeadline(deadline)

	rec := &covHijackRec{hdr: make(http.Header), conn: serverConn}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-Websocket-Version", "13")
	if ticket != "" {
		// Header.Add 走 canonical 存储（Sec-Websocket-Protocol），与真实
		// HTTP server 的 ReadMIMEHeader 行为一致；产品代码经 Header.Values
		// 读取（同样 canonical-safe）
		req.Header.Add("Sec-WebSocket-Protocol", ticket)
	}

	if !wantUpgrade {
		clientConn.Close()
		serverConn.Close()
		if wantDone {
			done := make(chan struct{})
			go func() { defer close(done); handler(rec, req) }()
			<-done
			return nil, rec, done
		}
		handler(rec, req)
		return nil, rec, nil
	}

	u := &url.URL{Scheme: "ws", Host: "cov.internal", Path: path}
	type covWSResult struct {
		conn *websocket.Conn
		resp *http.Response
		err  error
	}
	resCh := make(chan covWSResult, 1)
	go func() {
		conn, resp, err := websocket.NewClient(clientConn, u,
			http.Header{"Sec-Websocket-Protocol": {ticket}}, 4096, 4096)
		resCh <- covWSResult{conn, resp, err}
	}()

	// 消费客户端握手请求头
	var reqBytes []byte
	buf := make([]byte, 1)
	for !bytes.HasSuffix(reqBytes, []byte("\r\n\r\n")) {
		n, err := serverConn.Read(buf)
		if err != nil {
			t.Fatalf("read client handshake: %v", err)
		}
		reqBytes = append(reqBytes, buf[:n]...)
	}
	key := covExtractHeader(reqBytes, "Sec-WebSocket-Key")
	if key == "" {
		t.Fatalf("client handshake missing key:\n%s", reqBytes)
	}
	req.Header.Set("Sec-Websocket-Key", key)

	var handlerDone chan struct{}
	if wantDone {
		handlerDone = make(chan struct{})
		go func() { defer close(handlerDone); handler(rec, req) }()
	} else {
		go handler(rec, req)
	}

	select {
	case res := <-resCh:
		if res.err != nil {
			status, body := 0, ""
			if res.resp != nil {
				status = res.resp.StatusCode
				bodyBytes, _ := io.ReadAll(res.resp.Body)
				body = string(bodyBytes)
			}
			t.Fatalf("in-process websocket handshake: %v (status %d, body %q)", res.err, status, body)
		}
		return res.conn, rec, handlerDone
	case <-time.After(10 * time.Second):
		t.Fatal("in-process websocket handshake timeout")
		return nil, rec, handlerDone
	}
}

func covWantHTTP(t *testing.T, desc string, handler http.HandlerFunc, path, ticket string, want int) {
	t.Helper()
	_, rec := covDirectWS(t, handler, path, ticket, false)
	if rec.code != want {
		t.Errorf("%s: code = %d, want %d", desc, rec.code, want)
	}
}

// covWaitGone 轮询等待条件成立
func covWaitGone(t *testing.T, desc string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting: %s", desc)
}

func covTerminalSessionByConn(connID string) bool {
	terminalSessionsMu.Lock()
	defer terminalSessionsMu.Unlock()
	_, ok := terminalByConn[connID]
	return ok
}

func covDesktopSessionExists(sessionID string) bool {
	desktopSessionsMu.Lock()
	defer desktopSessionsMu.Unlock()
	_, ok := desktopSessions[sessionID]
	return ok
}

func covVNCSessionByConn(connID string) bool {
	vncSessionsMu.Lock()
	defer vncSessionsMu.Unlock()
	for _, s := range vncSessions {
		if s.ConnID == connID {
			return true
		}
	}
	return false
}

// covServerPush 在 goroutine 里发起服务端推送（net.Pipe 同步无缓冲，推送会
// 阻塞到客户端读完），随后客户端读消息，再用 covPushResult 回收推送结果
func covServerPush(push func() error) chan error {
	errCh := make(chan error, 1)
	go func() { errCh <- push() }()
	return errCh
}

func covPushResult(t *testing.T, desc string, errCh chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("%s: %v", desc, err)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("%s: push did not complete", desc)
	}
}

// covClearSessions 兜底清理全局会话表，避免用例间串扰
func covClearSessions() {
	terminalSessionsMu.Lock()
	terminalSessions = make(map[string]*TerminalSession)
	terminalByConn = make(map[string]*TerminalSession)
	terminalSessionsMu.Unlock()
	desktopSessionsMu.Lock()
	desktopSessions = make(map[string]*DesktopSession)
	desktopSessionsMu.Unlock()
	vncSessionsMu.Lock()
	vncSessions = make(map[string]*VNCSession)
	vncSessionsMu.Unlock()
}

// ============ 终端 ============

func TestCovTerminalWebSocketFullFlow(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	agent := covRegisterBareAgent(t, s, "agent-term")

	conn, _, tDone := covDirectWSJoined(t, s.handleTerminalWebSocket, "/api/remote/terminal",
		covTerminalTicket(t, s, "agent-term"))
	t.Cleanup(func() { conn.Close() })

	var connect map[string]interface{}
	covWSReadJSON(t, conn, &connect)
	if connect["type"] != "connect" {
		t.Fatalf("first client message = %v, want connect", connect)
	}

	// agent 收到 proxy_new
	newMsg := covAgentRecv(t, agent, "proxy_new")
	if newMsg.Type != protocol.MessageTypeProxyNew {
		t.Fatalf("agent msg type = %s", newMsg.Type)
	}
	connID, _ := newMsg.Payload["connId"].(string)
	if newMsg.Payload["terminal"] != true || !strings.HasPrefix(newMsg.Payload["proxyId"].(string), "terminal-") {
		t.Fatalf("proxy_new payload = %+v", newMsg.Payload)
	}

	// input / resize / 非 JSON 输入三种转发
	covWSWriteJSON(t, conn, map[string]interface{}{"type": "input", "data": "ls -la\n"})
	inputMsg := covAgentRecv(t, agent, "input proxy_data")
	if inputMsg.Type != protocol.MessageTypeProxyData || inputMsg.Payload["terminal"] != true {
		t.Fatalf("input forward = %+v", inputMsg.Payload)
	}
	covWSWriteJSON(t, conn, map[string]interface{}{"type": "resize", "rows": 24, "cols": 80})
	resizeMsg := covAgentRecv(t, agent, "resize proxy_data")
	if resizeMsg.Payload["resize"] != true || resizeMsg.Payload["rows"] != 24 {
		t.Fatalf("resize forward = %+v", resizeMsg.Payload)
	}
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("raw-bytes-not-json")); err != nil {
		t.Fatal(err)
	}
	rawMsg := covAgentRecv(t, agent, "raw proxy_data")
	if rawMsg.Payload["data"] != "raw-bytes-not-json" {
		t.Fatalf("raw forward = %+v", rawMsg.Payload)
	}
	// 未知 JSON 类型：不转发，仅更新活跃时间
	covWSWriteJSON(t, conn, map[string]interface{}{"type": "whatever"})

	// agent → 浏览器方向数据 + 录制
	pushErr := covServerPush(func() error { return s.HandleTerminalData(connID, []byte("hello")) })
	var out map[string]interface{}
	covWSReadJSON(t, conn, &out)
	covPushResult(t, "HandleTerminalData", pushErr)
	if out["type"] != "data" || out["data"] != "hello" {
		t.Fatalf("data message = %v", out)
	}
	if err := s.HandleTerminalData("no-such-conn", []byte("x")); err != nil {
		t.Fatalf("unknown conn should be no-op, got %v", err)
	}

	// 客户端异常关闭（非常规 close code）→ sendLoop 退出 → closeTerminalSession
	if err := conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(3000, "cov-abnormal"), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	// agent 收到 proxy_close
	closeMsg := covAgentRecv(t, agent, "proxy_close")
	if closeMsg.Type != protocol.MessageTypeProxyClose || closeMsg.Payload["terminal"] != true {
		t.Fatalf("close forward = %+v", closeMsg.Payload)
	}
	covWaitGone(t, "terminal session cleanup", func() bool { return !covTerminalSessionByConn(connID) })
	// done 分支的 closeTerminalSession 在后台 goroutine：audit 写在会话表
	// 删除之后，join handler 确保它在 TempDir 清理前落库完成
	covWaitHandlerExit(t, "terminal handler after client close", tDone)

	// 录制文件已生成并包含输出事件（recordingEnabled 默认开）
	recs, err := s.db.ListTerminalRecordings(10)
	if err != nil || len(recs) != 1 {
		t.Fatalf("recordings = %v, err = %v", recs, err)
	}
	recDir := s.recordingsDir()
	entries, err := os.ReadDir(recDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("recording dir %s: %v (entries=%d)", recDir, err, len(entries))
	}
	data, err := os.ReadFile(filepath.Join(recDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Errorf("cast file missing output event:\n%s", data)
	}
}

func TestCovTerminalAgentClose(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	agent := covRegisterBareAgent(t, s, "agent-term2")

	conn, _ := covDirectWS(t, s.handleTerminalWebSocket, "/api/remote/terminal",
		covTerminalTicket(t, s, "agent-term2"), true)
	defer conn.Close()

	var connect map[string]interface{}
	covWSReadJSON(t, conn, &connect)
	newMsg := covAgentRecv(t, agent, "proxy_new")
	connID, _ := newMsg.Payload["connId"].(string)

	// 发一条 input 并等 agent 收到：sendLoop 在首条 connect 写完成后才启动，
	// 以此确认 handler goroutine 已退出写路径，避免与测试侧推送并发写
	covWSWriteJSON(t, conn, map[string]interface{}{"type": "input", "data": "sync\n"})
	syncMsg := covAgentRecv(t, agent, "sync input")
	if syncMsg.Type != protocol.MessageTypeProxyData {
		t.Fatalf("sync input type = %s", syncMsg.Type)
	}

	// agent 侧关闭：浏览器收到 close 消息且连接被关
	pushErr := covServerPush(func() error {
		s.HandleTerminalClose(connID, "agent closed")
		return nil
	})
	var closeOut map[string]interface{}
	covWSReadJSON(t, conn, &closeOut)
	covPushResult(t, "HandleTerminalClose", pushErr)
	if closeOut["type"] != "close" || closeOut["message"] != "agent closed" {
		t.Fatalf("close message = %v", closeOut)
	}
	covWaitGone(t, "terminal session cleanup after agent close", func() bool { return !covTerminalSessionByConn(connID) })

	// 未知 connID：no-op
	s.HandleTerminalClose("nope", "x")
}

func TestCovTerminalWebSocketBadRequests(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	covRegisterBareAgent(t, s, "agent-term3")

	covWantHTTP(t, "missing ticket header", s.handleTerminalWebSocket, "/api/remote/terminal", "", http.StatusBadRequest)
	covWantHTTP(t, "invalid ticket", s.handleTerminalWebSocket, "/api/remote/terminal", "invalid-ticket", http.StatusUnauthorized)
	covWantHTTP(t, "missing params", s.handleTerminalWebSocket, "/api/remote/terminal",
		covTicket(t, s, map[string]string{"agent_id": "agent-term3"}), http.StatusBadRequest)
	covWantHTTP(t, "bad port", s.handleTerminalWebSocket, "/api/remote/terminal",
		covTicket(t, s, map[string]string{"agent_id": "agent-term3", "host": "127.0.0.1", "port": "abc", "protocol": "ssh"}),
		http.StatusBadRequest)
	covWantHTTP(t, "unsupported protocol", s.handleTerminalWebSocket, "/api/remote/terminal",
		covTicket(t, s, map[string]string{"agent_id": "agent-term3", "host": "127.0.0.1", "port": "22", "protocol": "rdp"}),
		http.StatusBadRequest)
	covWantHTTP(t, "agent offline", s.handleTerminalWebSocket, "/api/remote/terminal",
		covTicket(t, s, map[string]string{"agent_id": "ghost-agent", "host": "127.0.0.1", "port": "22", "protocol": "ssh"}),
		http.StatusNotFound)

	// agent 已关闭 → SendMessage 失败分支：错误 JSON + 会话回滚
	closed := covRegisterBareAgent(t, s, "agent-term-closed")
	closed.Close()
	conn, _ := covDirectWS(t, s.handleTerminalWebSocket, "/api/remote/terminal",
		covTicket(t, s, map[string]string{"agent_id": "agent-term-closed", "host": "127.0.0.1", "port": "22", "protocol": "ssh"}), true)
	defer conn.Close()
	var errOut map[string]interface{}
	covWSReadJSON(t, conn, &errOut)
	if errOut["type"] != "error" {
		t.Fatalf("error message = %v", errOut)
	}
	covWaitGone(t, "failed terminal session cleanup", func() bool {
		terminalSessionsMu.Lock()
		defer terminalSessionsMu.Unlock()
		return len(terminalSessions) == 0
	})
}

// ============ 桌面 ============

func TestCovDesktopWebSocketFullFlow(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	agent := covRegisterBareAgent(t, s, "agent-desk")

	ticket := covTicket(t, s, map[string]string{
		"agent_id": "agent-desk", "host": "127.0.0.1", "port": "3389", "protocol": "rdp",
		"username": "u", "password": "p", "domain": "d", "width": "1920", "height": "1080",
	})
	conn, _ := covDirectWS(t, s.handleDesktopWebSocket, "/api/remote/desktop", ticket, true)
	t.Cleanup(func() { conn.Close() })

	var connecting map[string]interface{}
	covWSReadJSON(t, conn, &connecting)
	if connecting["type"] != "connecting" {
		t.Fatalf("first message = %v", connecting)
	}

	newMsg := covAgentRecv(t, agent, "desktop_new")
	if newMsg.Type != protocol.MessageTypeDesktopNew {
		t.Fatalf("agent msg type = %s", newMsg.Type)
	}
	if newMsg.Payload["width"] != 1920 || newMsg.Payload["username"] != "u" || newMsg.Payload["password"] != "p" {
		t.Fatalf("desktop_new payload = %+v", newMsg.Payload)
	}
	sessionID, _ := newMsg.Payload["sessionId"].(string)

	// keyboard / mouse / clipboard / set_resolution / unknown / 非JSON
	covWSWriteJSON(t, conn, map[string]interface{}{"type": "keyboard", "scanCode": 28, "keyDown": true})
	kb := covAgentRecv(t, agent, "keyboard")
	if kb.Type != protocol.MessageTypeDesktopData || kb.Payload["desktopType"] != string(protocol.DesktopMsgKeyboard) || kb.Payload["scanCode"] != float64(28) {
		t.Fatalf("keyboard = %+v", kb.Payload)
	}
	covWSWriteJSON(t, conn, map[string]interface{}{"type": "mouse", "x": 1, "y": 2, "buttons": 1, "wheelDelta": 0, "action": "move"})
	mouse := covAgentRecv(t, agent, "mouse")
	if mouse.Payload["desktopType"] != string(protocol.DesktopMsgMouse) || mouse.Payload["x"] != float64(1) {
		t.Fatalf("mouse = %+v", mouse.Payload)
	}
	covWSWriteJSON(t, conn, map[string]interface{}{"type": "clipboard", "text": "hi"})
	clip := covAgentRecv(t, agent, "clipboard")
	if clip.Payload["desktopType"] != string(protocol.DesktopMsgClipboardData) || clip.Payload["text"] != "hi" {
		t.Fatalf("clipboard = %+v", clip.Payload)
	}
	covWSWriteJSON(t, conn, map[string]interface{}{"type": "set_resolution", "width": 800, "height": 600})
	res := covAgentRecv(t, agent, "set_resolution")
	if res.Payload["desktopType"] != string(protocol.DesktopMsgSetResolution) || res.Payload["width"] != float64(800) {
		t.Fatalf("set_resolution = %+v", res.Payload)
	}
	covWSWriteJSON(t, conn, map[string]interface{}{"type": "unknown"}) // 忽略
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("not-json")); err != nil { // 忽略
		t.Fatal(err)
	}

	// agent → 浏览器数据
	pushErr := covServerPush(func() error {
		s.HandleDesktopData(protocol.NewMessage(protocol.MessageTypeDesktopData, map[string]interface{}{
			"sessionId": sessionID, "desktopType": string(protocol.DesktopMsgScreenUpdate), "frame": "zzz",
		}))
		return nil
	})
	var screen map[string]interface{}
	covWSReadJSON(t, conn, &screen)
	covPushResult(t, "HandleDesktopData", pushErr)
	if screen["type"] != string(protocol.DesktopMsgScreenUpdate) || screen["frame"] != "zzz" {
		t.Fatalf("screen update = %v", screen)
	}

	// 解析失败 / 未知会话：no-op
	s.HandleDesktopData(protocol.NewMessage(protocol.MessageTypeDesktopData, nil))
	s.HandleDesktopData(protocol.NewMessage(protocol.MessageTypeDesktopData, map[string]interface{}{
		"sessionId": "ghost", "desktopType": "x",
	}))

	// 浏览器断开 → desktop_close 给 agent；keepalive done 分支退出
	conn.Close()
	closeMsg := covAgentRecv(t, agent, "desktop_close")
	if closeMsg.Type != protocol.MessageTypeDesktopClose {
		t.Fatalf("agent msg type = %s", closeMsg.Type)
	}

	// done 分支不清理 map（桌面会话依赖 agent 回包清理）。
	// 手动制造写失败路径触发 closeDesktopSession：关服务端连接后再转发数据。
	desktopSessionsMu.Lock()
	session := desktopSessions[sessionID]
	desktopSessionsMu.Unlock()
	if session == nil {
		t.Fatal("desktop session should still exist after client close")
	}
	session.ClientWS.Close()
	s.HandleDesktopData(protocol.NewMessage(protocol.MessageTypeDesktopData, map[string]interface{}{
		"sessionId": sessionID, "desktopType": string(protocol.DesktopMsgScreenUpdate),
	}))
	covWaitGone(t, "desktop session cleanup on write failure", func() bool { return !covDesktopSessionExists(sessionID) })
}

func TestCovDesktopAgentCloseAndDefaults(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	agent := covRegisterBareAgent(t, s, "agent-desk2")

	// 缺 height + 非法 width → 默认 1280x800
	ticket := covTicket(t, s, map[string]string{
		"agent_id": "agent-desk2", "host": "127.0.0.1", "port": "3389", "protocol": "rdp", "width": "abc",
	})
	conn, _ := covDirectWS(t, s.handleDesktopWebSocket, "/api/remote/desktop", ticket, true)

	var connecting map[string]interface{}
	covWSReadJSON(t, conn, &connecting)
	newMsg := covAgentRecv(t, agent, "desktop_new")
	if newMsg.Payload["width"] != 1280 || newMsg.Payload["height"] != 800 {
		t.Fatalf("default resolution payload = %+v", newMsg.Payload)
	}
	sessionID, _ := newMsg.Payload["sessionId"].(string)

	// 先发一条 keyboard 并等 agent 收到（desktopSendLoop 在 connecting 写完成后
	// 才启动），确认 handler 已退出写路径
	covWSWriteJSON(t, conn, map[string]interface{}{"type": "keyboard", "scanCode": 1, "keyDown": true})
	if kb := covAgentRecv(t, agent, "sync keyboard"); kb.Type != protocol.MessageTypeDesktopData {
		t.Fatalf("sync keyboard type = %s", kb.Type)
	}

	// agent 侧断开 → 会话清理，浏览器收到 disconnected
	pushErr := covServerPush(func() error {
		s.HandleDesktopClose(protocol.NewMessage(protocol.MessageTypeDesktopClose, map[string]interface{}{
			"sessionId": sessionID, "reason": "cov reason",
		}))
		return nil
	})
	var disc map[string]interface{}
	covWSReadJSON(t, conn, &disc)
	covPushResult(t, "HandleDesktopClose", pushErr)
	if disc["type"] != "disconnected" || disc["reason"] != "cov reason" {
		t.Fatalf("disconnected = %v", disc)
	}
	covWaitGone(t, "desktop session cleanup by agent close", func() bool { return !covDesktopSessionExists(sessionID) })
	conn.Close()

	// 坏消息 / 未知会话的 HandleDesktopClose：no-op
	s.HandleDesktopClose(protocol.NewMessage(protocol.MessageTypeDesktopClose, nil))
	s.HandleDesktopClose(protocol.NewMessage(protocol.MessageTypeDesktopClose, map[string]interface{}{"sessionId": "ghost"}))
}

func TestCovDesktopWebSocketBadRequests(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	covRegisterBareAgent(t, s, "agent-desk3")

	covWantHTTP(t, "missing ticket", s.handleDesktopWebSocket, "/api/remote/desktop", "", http.StatusBadRequest)
	covWantHTTP(t, "bad ticket", s.handleDesktopWebSocket, "/api/remote/desktop", "bad-ticket", http.StatusUnauthorized)
	covWantHTTP(t, "missing params", s.handleDesktopWebSocket, "/api/remote/desktop",
		covTicket(t, s, map[string]string{"agent_id": "agent-desk3"}), http.StatusBadRequest)
	covWantHTTP(t, "agent offline", s.handleDesktopWebSocket, "/api/remote/desktop",
		covTicket(t, s, map[string]string{"agent_id": "ghost", "host": "h", "port": "3389"}), http.StatusNotFound)

	// SendMessage 失败分支：错误 JSON
	closed := covRegisterBareAgent(t, s, "agent-desk-closed")
	closed.Close()
	conn, _ := covDirectWS(t, s.handleDesktopWebSocket, "/api/remote/desktop",
		covTicket(t, s, map[string]string{"agent_id": "agent-desk-closed", "host": "h", "port": "3389"}), true)
	defer conn.Close()
	var errOut map[string]interface{}
	covWSReadJSON(t, conn, &errOut)
	if errOut["type"] != "error" {
		t.Fatalf("error message = %v", errOut)
	}
}

// ============ VNC ============

func TestCovVNCWebSocketFullFlow(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	agent := covRegisterBareAgent(t, s, "agent-vnc")

	ticket := covTicket(t, s, map[string]string{
		"agent_id": "agent-vnc", "host": "127.0.0.1", "port": "5900", "protocol": "vnc", "password": "pw",
	})
	conn, _ := covDirectWS(t, s.handleVNCWebSocket, "/api/remote/vnc", ticket, true)
	t.Cleanup(func() { conn.Close() })

	newMsg := covAgentRecv(t, agent, "proxy_new")
	if newMsg.Type != protocol.MessageTypeProxyNew {
		t.Fatalf("agent msg type = %s", newMsg.Type)
	}
	connID, _ := newMsg.Payload["connId"].(string)
	if !strings.HasPrefix(newMsg.Payload["proxyId"].(string), "vnc-") ||
		newMsg.Payload["protocol"] != "vnc" || newMsg.Payload["password"] != "pw" {
		t.Fatalf("proxy_new payload = %+v", newMsg.Payload)
	}

	// 二进制帧转发；文本帧忽略
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	bin := covAgentRecv(t, agent, "binary proxy_data")
	if bin.Type != protocol.MessageTypeProxyData {
		t.Fatalf("binary forward type = %s", bin.Type)
	}
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("ignore-me")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // 文本帧不产生转发

	// agent → 浏览器二进制
	pushErr := covServerPush(func() error { return s.HandleVNCData(connID, []byte{9, 9}) })
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	mt, data, err := conn.ReadMessage()
	covPushResult(t, "HandleVNCData", pushErr)
	if err != nil || mt != websocket.BinaryMessage || !bytes.Equal(data, []byte{9, 9}) {
		t.Fatalf("vnc data = %v %v %v", mt, data, err)
	}
	if err := s.HandleVNCData("no-such", []byte{1}); err != nil {
		t.Fatalf("unknown conn should be no-op, got %v", err)
	}

	// agent 侧关闭
	s.HandleVNCClose(connID, "cov reason")
	covWaitGone(t, "vnc session cleanup by agent close", func() bool { return !covVNCSessionByConn(connID) })
	s.HandleVNCClose("no-such", "x") // no-op
}

func TestCovVNCWebSocketClientCloseAndBadRequests(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	agent := covRegisterBareAgent(t, s, "agent-vnc2")

	// 客户端断开 → proxy_close → closeVNCSession（含审计）
	conn, _, vDone := covDirectWSJoined(t, s.handleVNCWebSocket, "/api/remote/vnc",
		covTicket(t, s, map[string]string{"agent_id": "agent-vnc2", "host": "127.0.0.1", "port": "5900", "protocol": "vnc"}))
	newMsg := covAgentRecv(t, agent, "proxy_new")
	connID, _ := newMsg.Payload["connId"].(string)
	conn.Close()
	closeMsg := covAgentRecv(t, agent, "proxy_close")
	if closeMsg.Type != protocol.MessageTypeProxyClose {
		t.Fatalf("close type = %s", closeMsg.Type)
	}
	covWaitGone(t, "vnc session cleanup on client close", func() bool { return !covVNCSessionByConn(connID) })
	// 审计在 keepaliveLoop（handler goroutine）里、会话表删除之后写库，join 排干
	covWaitHandlerExit(t, "vnc handler after client close", vDone)

	covWantHTTP(t, "missing ticket", s.handleVNCWebSocket, "/api/remote/vnc", "", http.StatusBadRequest)
	covWantHTTP(t, "bad ticket", s.handleVNCWebSocket, "/api/remote/vnc", "bad", http.StatusUnauthorized)
	covWantHTTP(t, "missing params", s.handleVNCWebSocket, "/api/remote/vnc",
		covTicket(t, s, map[string]string{"agent_id": "a"}), http.StatusBadRequest)
	covWantHTTP(t, "agent offline", s.handleVNCWebSocket, "/api/remote/vnc",
		covTicket(t, s, map[string]string{"agent_id": "ghost", "host": "h", "port": "5900"}), http.StatusNotFound)

	// SendMessage 失败分支：直接关连接
	closed := covRegisterBareAgent(t, s, "agent-vnc-closed")
	closed.Close()
	conn2, _ := covDirectWS(t, s.handleVNCWebSocket, "/api/remote/vnc",
		covTicket(t, s, map[string]string{"agent_id": "agent-vnc-closed", "host": "h", "port": "5900"}), true)
	defer conn2.Close()
	// 服务端 SendMessage 失败后会立即关管道；SetReadDeadline 可能已赶不上，
	// 两种错误都证明连接被关闭
	if err := conn2.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		if !strings.Contains(err.Error(), "closed pipe") {
			t.Fatalf("set read deadline: %v", err)
		}
	} else if _, _, err := conn2.ReadMessage(); err == nil {
		t.Error("expected connection close after proxy_new failure")
	}
	covWaitGone(t, "failed vnc session cleanup", func() bool {
		vncSessionsMu.Lock()
		defer vncSessionsMu.Unlock()
		return len(vncSessions) == 0
	})
}

// ============ 票据与会话管理 API ============

func TestCovHandleTicketCreateBranches(t *testing.T) {
	s := covRemoteSetup(t)
	covRegisterBareAgent(t, s, "agent-tk")

	call := func(method, target string, body string, withUser bool) *httptest.ResponseRecorder {
		var req *http.Request
		if body == "" {
			req = covReq(method, target, nil)
		} else {
			req = covReq(method, target, strings.NewReader(body))
		}
		if withUser {
			req = covAuthReq(method, target, strings.NewReader(body), "1", "covuser", "admin")
		}
		return covCallAuth(s, s.handleTicketCreate, req)
	}

	covWantCode(t, "wrong method", call(http.MethodGet, "/api/remote/tickets", "", true), http.StatusMethodNotAllowed)
	covWantCode(t, "bad body", call(http.MethodPost, "/api/remote/tickets", "{bad", true), http.StatusBadRequest)
	covWantCode(t, "missing fields", call(http.MethodPost, "/api/remote/tickets", `{"agent_id":"agent-tk"}`, true), http.StatusBadRequest)
	covWantCode(t, "agent offline", call(http.MethodPost, "/api/remote/tickets", `{"agent_id":"ghost","host":"h","port":22,"protocol":"ssh"}`, true), http.StatusNotFound)
	// 无用户上下文：直调 handler（走中间件会被提前 401，盖不到该分支）
	rec := covRec()
	s.handleTicketCreate(rec, covReq(http.MethodPost, "/api/remote/tickets",
		strings.NewReader(`{"agent_id":"agent-tk","host":"127.0.0.1","port":22,"protocol":"ssh"}`)))
	covWantCode(t, "no user", rec, http.StatusUnauthorized)

	rec = call(http.MethodPost, "/api/remote/tickets",
		`{"agent_id":"agent-tk","host":"127.0.0.1","port":2222,"protocol":"telnet","username":"u","password":"p","domain":"d","width":10,"height":20}`, true)
	covWantCode(t, "success", rec, http.StatusOK)
	var resp struct {
		Ticket    string `json:"ticket"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Ticket == "" || resp.ExpiresAt == "" {
		t.Fatalf("ticket response = %+v", resp)
	}
}

func TestCovHandleTicketCreateEgressDenied(t *testing.T) {
	s := covRemoteSetup(t)
	covRegisterBareAgent(t, s, "agent-eg")
	s.cfg.RemoteControl = &config.RemoteControlConfig{
		AllowedTargets: []string{"10.0.0.0/8", "trusted.host"},
	}

	post := func(body string) *httptest.ResponseRecorder {
		return covCallAuth(s, s.handleTicketCreate,
			covAuthReq(http.MethodPost, "/api/remote/tickets", strings.NewReader(body), "1", "covuser", "admin"))
	}

	// 不在白名单 → 403
	covWantCode(t, "egress denied", post(`{"agent_id":"agent-eg","host":"192.168.1.1","port":22,"protocol":"ssh"}`), http.StatusForbidden)
	// 白名单 CIDR → 放行
	covWantCode(t, "cidr allowed", post(`{"agent_id":"agent-eg","host":"10.1.2.3","port":22,"protocol":"ssh"}`), http.StatusOK)

	// 按 agent 出口策略：端口不允许
	s.cfg.RemoteControl.EgressPolicies = []*config.RemoteEgressPolicy{{
		AgentID: "agent-eg", AllowedTargets: []string{"10.1.2.3"}, AllowedPorts: []int{2222},
	}}
	covWantCode(t, "port denied", post(`{"agent_id":"agent-eg","host":"10.1.2.3","port":22,"protocol":"ssh"}`), http.StatusForbidden)
	covWantCode(t, "port allowed", post(`{"agent_id":"agent-eg","host":"10.1.2.3","port":2222,"protocol":"ssh"}`), http.StatusOK)
}

func TestCovRemoteSessionsAPI(t *testing.T) {
	s := covRemoteSetup(t)
	covRegisterBareAgent(t, s, "agent-sess")

	// GET 列表
	rec := covCallAuth(s, s.handleRemoteSessions, covAuthReq(http.MethodGet, "/api/remote/sessions", nil, "1", "u", "admin"))
	covWantCode(t, "list", rec, http.StatusOK)

	// POST 分支
	covWantCode(t, "bad body", covCallAuth(s, s.handleRemoteSessions,
		covAuthReq(http.MethodPost, "/api/remote/sessions", strings.NewReader("{bad"), "1", "u", "admin")), http.StatusBadRequest)
	covWantCode(t, "missing fields", covCallAuth(s, s.handleRemoteSessions,
		covAuthReq(http.MethodPost, "/api/remote/sessions", strings.NewReader(`{"agentId":"a"}`), "1", "u", "admin")), http.StatusBadRequest)
	covWantCode(t, "bad protocol", covCallAuth(s, s.handleRemoteSessions,
		covAuthReq(http.MethodPost, "/api/remote/sessions", strings.NewReader(`{"agentId":"agent-sess","protocol":"ftp","host":"h","port":22}`), "1", "u", "admin")), http.StatusBadRequest)
	covWantCode(t, "agent offline", covCallAuth(s, s.handleRemoteSessions,
		covAuthReq(http.MethodPost, "/api/remote/sessions", strings.NewReader(`{"agentId":"ghost","protocol":"ssh","host":"h","port":22}`), "1", "u", "admin")), http.StatusNotFound)
	rec = covCallAuth(s, s.handleRemoteSessions,
		covAuthReq(http.MethodPost, "/api/remote/sessions", strings.NewReader(`{"agentId":"agent-sess","protocol":"ssh","host":"h","port":22}`), "1", "u", "admin"))
	covWantCode(t, "create", rec, http.StatusCreated)
	var session RemoteSession
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	covWantCode(t, "method not allowed", covCallAuth(s, s.handleRemoteSessions,
		covAuthReq(http.MethodPut, "/api/remote/sessions", nil, "1", "u", "admin")), http.StatusMethodNotAllowed)

	// 状态更新（先于删除）
	if err := s.remoteSessions.UpdateStatus(session.ID, RemoteSessionStatusFailed, "boom"); err != nil {
		t.Errorf("UpdateStatus: %v", err)
	}
	got, _ := s.remoteSessions.Get(session.ID)
	if got.Status != RemoteSessionStatusFailed || got.ClosedAt == nil || got.Error != "boom" {
		t.Errorf("updated session = %+v", got)
	}

	// /api/remote/sessions/{id}
	covWantCode(t, "empty id", covCallAuth(s, s.handleRemoteSession,
		covAuthReq(http.MethodGet, "/api/remote/sessions/", nil, "1", "u", "admin")), http.StatusBadRequest)
	covWantCode(t, "get missing", covCallAuth(s, s.handleRemoteSession,
		covAuthReq(http.MethodGet, "/api/remote/sessions/ghost", nil, "1", "u", "admin")), http.StatusNotFound)
	rec = covCallAuth(s, s.handleRemoteSession,
		covAuthReq(http.MethodGet, "/api/remote/sessions/"+session.ID, nil, "1", "u", "admin"))
	covWantCode(t, "get found", rec, http.StatusOK)
	covWantCode(t, "delete missing", covCallAuth(s, s.handleRemoteSession,
		covAuthReq(http.MethodDelete, "/api/remote/sessions/ghost", nil, "1", "u", "admin")), http.StatusNotFound)
	covWantCode(t, "delete found", covCallAuth(s, s.handleRemoteSession,
		covAuthReq(http.MethodDelete, "/api/remote/sessions/"+session.ID, nil, "1", "u", "admin")), http.StatusNoContent)
	covWantCode(t, "method not allowed", covCallAuth(s, s.handleRemoteSession,
		covAuthReq(http.MethodPatch, "/api/remote/sessions/"+session.ID, nil, "1", "u", "admin")), http.StatusMethodNotAllowed)

	// RemoteSessionManager 错误分支
	if err := s.remoteSessions.UpdateStatus("ghost", RemoteSessionStatusClosed, ""); err == nil {
		t.Error("UpdateStatus on missing session should fail")
	}
	if s.remoteSessions.Delete("ghost") {
		t.Error("Delete on missing session should return false")
	}
}

// ============ SSH 凭据下发 ============

func TestCovTerminalSSHForwardsCredentials(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	agent := covRegisterBareAgent(t, s, "agent-ssh-cred")

	conn, _, tDone := covDirectWSJoined(t, s.handleTerminalWebSocket, "/api/remote/terminal",
		covTicket(t, s, map[string]string{
			"agent_id": "agent-ssh-cred", "host": "127.0.0.1", "port": "22", "protocol": "ssh",
			"username": "root", "password": "s3cret", "private_key": "PEM",
		}))
	t.Cleanup(func() { conn.Close() })

	var connect map[string]interface{}
	covWSReadJSON(t, conn, &connect)

	newMsg := covAgentRecv(t, agent, "proxy_new")
	if newMsg.Type != protocol.MessageTypeProxyNew {
		t.Fatalf("agent msg type = %s", newMsg.Type)
	}
	// SSH 凭据必须随 proxy_new 下发到 agent（agent 侧终结 SSH 协议）
	if newMsg.Payload["username"] != "root" || newMsg.Payload["password"] != "s3cret" {
		t.Fatalf("SSH credentials not forwarded: %+v", newMsg.Payload)
	}
	if newMsg.Payload["privateKey"] != "PEM" {
		t.Fatalf("privateKey not forwarded: %+v", newMsg.Payload)
	}

	// telnet 不带凭据（裸 TCP 协议无认证）
	covClearSessions()
	agent2 := covRegisterBareAgent(t, s, "agent-telnet-noauth")
	conn2, _, tDone2 := covDirectWSJoined(t, s.handleTerminalWebSocket, "/api/remote/terminal",
		covTicket(t, s, map[string]string{
			"agent_id": "agent-telnet-noauth", "host": "127.0.0.1", "port": "23", "protocol": "telnet",
			"username": "u", "password": "p",
		}))
	t.Cleanup(func() { conn2.Close() })
	var connect2 map[string]interface{}
	covWSReadJSON(t, conn2, &connect2)
	newMsg2 := covAgentRecv(t, agent2, "proxy_new")
	if _, ok := newMsg2.Payload["username"]; ok {
		t.Fatalf("telnet should not forward SSH credentials: %+v", newMsg2.Payload)
	}

	conn.Close()
	conn2.Close()
	<-tDone
	<-tDone2
}
