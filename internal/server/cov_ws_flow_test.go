package server

// cov_ws_flow_test.go 覆盖 websocket.go（handleWebSocket/readLoop/writeLoop/
// sendRegisterError/isOriginAllowed）、dispatcher.go、system_info.go 以及
// server.go 的 proxy 消息处理与 Agent RPC 辅助方法。全部走真实 WebSocket 拨号。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/proxy"
	"github.com/gorilla/websocket"
)

// covWSReady 给 covNewServer 补上 codec 与 upgrader（WebSocket 流程必需）
func covWSReady(s *Server) {
	if s.codec == nil {
		s.codec = protocol.NewCodec()
	}
	s.upgrader = websocket.Upgrader{
		CheckOrigin:     isOriginAllowed,
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
}

// covWSStrip 把 httptest 的 http URL 换成 ws 协议
func covWSStrip(url string) string { return "ws" + strings.TrimPrefix(url, "http") }

// covDialWS 拨号并返回客户端连接（失败即 Fatal）
func covDialWS(t *testing.T, url string, header http.Header) *websocket.Conn {
	t.Helper()
	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(url, header)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	return conn
}

// covWSWriteJSON 带截止期写 JSON
func covWSWriteJSON(t *testing.T, conn *websocket.Conn, v interface{}) {
	t.Helper()
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set write deadline: %v", err)
	}
	if err := conn.WriteJSON(v); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

// covWSReadJSON 带截止期读 JSON
func covWSReadJSON(t *testing.T, conn *websocket.Conn, v interface{}) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if err := conn.ReadJSON(v); err != nil {
		t.Fatalf("read json: %v", err)
	}
}

// covAgentRecv 从 agent.Send 限时取一条消息
func covAgentRecv(t *testing.T, agent *Agent, desc string) *protocol.Message {
	t.Helper()
	select {
	case msg := <-agent.Send:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting agent message: %s", desc)
		return nil
	}
}

// covRegisterPayload 构造注册 payload map（wire 格式）
func covRegisterPayload(agentID string) map[string]interface{} {
	return map[string]interface{}{
		"agentId":      agentID,
		"hostname":     "host-" + agentID,
		"ip":           "10.1.1.1",
		"location":     map[string]interface{}{"region": "r1", "zone": "z1"},
		"capabilities": []interface{}{map[string]interface{}{"type": "system", "version": "1"}},
	}
}

// ============ isOriginAllowed ============

func TestCovIsOriginAllowed(t *testing.T) {
	req := func(origin string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/ws", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}

	t.Setenv("ALLOWED_ORIGINS", "")
	if !isOriginAllowed(req("http://evil.com")) {
		t.Error("empty allowlist should accept all origins")
	}

	t.Setenv("ALLOWED_ORIGINS", "*")
	if !isOriginAllowed(req("http://any.com")) {
		t.Error("wildcard should accept")
	}

	t.Setenv("ALLOWED_ORIGINS", "http://a.com, http://b.com")
	if !isOriginAllowed(req("http://a.com")) {
		t.Error("listed origin should be accepted")
	}
	if isOriginAllowed(req("http://c.com")) {
		t.Error("unlisted origin should be rejected")
	}
	if isOriginAllowed(req("")) {
		t.Error("empty origin should not match explicit entries")
	}
}

// ============ /ws handleWebSocket ============

func TestCovHandleWebSocketFullFlow(t *testing.T) {
	s := covNewServer(t)
	covWSReady(s)
	httpSrv := httptest.NewServer(http.HandlerFunc(s.handleWebSocket))
	t.Cleanup(httpSrv.Close)

	conn := covDialWS(t, covWSStrip(httpSrv.URL)+"/ws", nil)
	t.Cleanup(func() { conn.Close() })

	covWSWriteJSON(t, conn, protocol.NewMessage(protocol.MessageTypeRegister, covRegisterPayload("agent-ws-1")))

	var resp protocol.Message
	covWSReadJSON(t, conn, &resp)
	if resp.Type != protocol.MessageTypeRegister {
		t.Fatalf("register response type = %s, want register", resp.Type)
	}
	if resp.Payload["status"] != "accepted" {
		t.Fatalf("register status = %v, want accepted", resp.Payload["status"])
	}

	// Agent 已进 registry，并持久化到 DB
	if _, ok := s.registry.Get("agent-ws-1"); !ok {
		t.Fatal("agent not registered in registry")
	}
	if _, err := s.db.GetAgent("agent-ws-1"); err != nil {
		t.Fatalf("agent not persisted: %v", err)
	}

	// 心跳携带 systemInfo → 落库 + ACK（同一 ID）
	hb := protocol.NewMessage(protocol.MessageTypeHeartbeat, map[string]interface{}{
		"agentId": "agent-ws-1",
		"status":  "ok",
		"systemInfo": map[string]interface{}{
			"cpuUsage": 12.5, "cpuCores": 4, "memTotal": 100, "hostname": "host-agent-ws-1",
		},
	})
	covWSWriteJSON(t, conn, hb)
	var ack protocol.Message
	covWSReadJSON(t, conn, &ack)
	if ack.Type != protocol.MessageTypeHeartbeat || ack.ID != hb.ID {
		t.Fatalf("heartbeat ack = %+v", ack)
	}

	snap, err := s.db.GetSystemInfoSnapshot("agent-ws-1")
	if err != nil {
		t.Fatalf("snapshot not saved: %v", err)
	}
	if snap.CPUUsage != 12.5 || snap.Hostname != "host-agent-ws-1" {
		t.Errorf("snapshot = %+v", snap)
	}

	// rpc_response（无 pending）与未知类型走默认分支，不崩
	covWSWriteJSON(t, conn, protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{"status": "success"}))
	covWSWriteJSON(t, conn, protocol.NewMessage(protocol.MessageType("bogus"), nil))

	// 客户端断开 → readLoop 出错 → Unregister
	conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := s.registry.Get("agent-ws-1"); !ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent still registered after client close")
}

func TestCovHandleWebSocketUpgradeFailure(t *testing.T) {
	s := covNewServer(t)
	covWSReady(s)
	httpSrv := httptest.NewServer(http.HandlerFunc(s.handleWebSocket))
	t.Cleanup(httpSrv.Close)

	// 普通 GET 缺少升级头 → Upgrade 失败 → 直接返回
	resp, err := http.Get(httpSrv.URL + "/ws")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("plain GET /ws status = %d, want 400", resp.StatusCode)
	}
}

func TestCovHandleWebSocketFirstMessageNotRegister(t *testing.T) {
	s := covNewServer(t)
	covWSReady(s)
	httpSrv := httptest.NewServer(http.HandlerFunc(s.handleWebSocket))
	t.Cleanup(httpSrv.Close)

	conn := covDialWS(t, covWSStrip(httpSrv.URL)+"/ws", nil)
	defer conn.Close()

	covWSWriteJSON(t, conn, protocol.NewMessage(protocol.MessageTypeHeartbeat, nil))

	// 服务端直接关闭连接：读到错误
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var m protocol.Message
	if err := conn.ReadJSON(&m); err == nil {
		t.Error("expected read error after non-register first message")
	}
}

func TestCovHandleWebSocketBadRegisterPayload(t *testing.T) {
	s := covNewServer(t)
	covWSReady(s)
	httpSrv := httptest.NewServer(http.HandlerFunc(s.handleWebSocket))
	t.Cleanup(httpSrv.Close)

	// location 为数字 → DecodeRegister unmarshal 失败
	conn := covDialWS(t, covWSStrip(httpSrv.URL)+"/ws", nil)
	defer conn.Close()
	covWSWriteJSON(t, conn, protocol.NewMessage(protocol.MessageTypeRegister, map[string]interface{}{"location": 123}))

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var m protocol.Message
	if err := conn.ReadJSON(&m); err == nil {
		t.Error("expected read error after bad register payload")
	}

	// 客户端先断开 → 服务端读注册消息失败分支
	conn2 := covDialWS(t, covWSStrip(httpSrv.URL)+"/ws", nil)
	conn2.Close()
	time.Sleep(100 * time.Millisecond)
}

func TestCovHandleWebSocketDuplicateRejected(t *testing.T) {
	s := covNewServer(t)
	covWSReady(s)
	if err := s.registry.Register(NewAgent("agent-dup", nil)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.registry.Unregister("agent-dup") })

	httpSrv := httptest.NewServer(http.HandlerFunc(s.handleWebSocket))
	t.Cleanup(httpSrv.Close)

	conn := covDialWS(t, covWSStrip(httpSrv.URL)+"/ws", nil)
	defer conn.Close()
	covWSWriteJSON(t, conn, protocol.NewMessage(protocol.MessageTypeRegister, covRegisterPayload("agent-dup")))

	var resp protocol.Message
	covWSReadJSON(t, conn, &resp)
	if resp.Type != protocol.MessageTypeError {
		t.Fatalf("response type = %s, want error", resp.Type)
	}
	if resp.Payload["code"] != "duplicate_connection" {
		t.Fatalf("error code = %v, want duplicate_connection", resp.Payload["code"])
	}
}

func TestCovHandleWebSocketDBErrorRejected(t *testing.T) {
	s := covNewServer(t)
	covWSReady(s)
	covCloseDB(t, s) // GetAgent 报错 → registration_failed

	httpSrv := httptest.NewServer(http.HandlerFunc(s.handleWebSocket))
	t.Cleanup(httpSrv.Close)

	conn := covDialWS(t, covWSStrip(httpSrv.URL)+"/ws", nil)
	defer conn.Close()
	covWSWriteJSON(t, conn, protocol.NewMessage(protocol.MessageTypeRegister, covRegisterPayload("agent-dberr")))

	var resp protocol.Message
	covWSReadJSON(t, conn, &resp)
	if resp.Type != protocol.MessageTypeError {
		t.Fatalf("response type = %s, want error", resp.Type)
	}
	if resp.Payload["code"] != "registration_failed" {
		t.Fatalf("error code = %v, want registration_failed", resp.Payload["code"])
	}
}

// ============ dispatcher.go ============

func TestCovHandleMessageDispatch(t *testing.T) {
	s := covNewServer(t)
	s.proxyMgr = proxy.NewManager(s, s.db)
	agent := NewAgent("agent-dispatch", nil)

	cases := []*protocol.Message{
		protocol.NewMessage(protocol.MessageTypeHeartbeat, nil),
		protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{"status": "success"}),
		protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{"proxyId": "p1", "connId": "c1", "data": "aGk="}),
		protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{"proxyId": "p1", "connId": "c1", "reason": "done"}),
		protocol.NewMessage(protocol.MessageTypeProxyError, map[string]interface{}{"proxyId": "p1", "error": "boom"}),
		protocol.NewMessage(protocol.MessageTypeDesktopData, map[string]interface{}{"bad": 1}),
		protocol.NewMessage(protocol.MessageTypeDesktopClose, nil),
		protocol.NewMessage(protocol.MessageType("unknown-type"), nil),
	}
	for i, msg := range cases {
		s.handleMessage(agent, msg) // 不 panic 即通过
		_ = i
	}
}

func TestCovHandleHeartbeatBranches(t *testing.T) {
	s := covNewServer(t)
	agent := NewAgent("agent-hb", nil)

	// 空 payload：DecodeHeartbeat 失败被忽略，仍 ACK
	msg := protocol.NewMessage(protocol.MessageTypeHeartbeat, nil)
	s.handleHeartbeat(agent, msg)
	select {
	case got := <-agent.Send:
		if got.ID != msg.ID || got.Type != protocol.MessageTypeHeartbeat {
			t.Fatalf("ack = %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no heartbeat ack")
	}

	// 发送通道满：default 分支
	full := &Agent{ID: "agent-hb-full", Send: make(chan *protocol.Message)} // 无缓冲且无消费者
	s.handleHeartbeat(full, protocol.NewMessage(protocol.MessageTypeHeartbeat, nil))
}

func TestCovHandleRPCResponseBranches(t *testing.T) {
	s := covNewServer(t)

	// 未知 ID：无操作
	s.handleRPCResponse(protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{"status": "success"}))

	// 正常投递
	ch := make(chan *protocol.Message, 1)
	s.registry.RegisterPendingResponse("mid-1", ch)
	respMsg := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{"status": "success"})
	respMsg.ID = "mid-1"
	s.handleRPCResponse(respMsg)
	select {
	case got := <-ch:
		if got.ID != "mid-1" {
			t.Fatalf("delivered = %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("response not delivered")
	}
	if _, exists := s.registry.GetPendingResponse("mid-1"); exists {
		t.Error("pending response should be unregistered after delivery")
	}

	// 通道满：default 分支
	fullCh := make(chan *protocol.Message, 1)
	fullCh <- protocol.NewMessage(protocol.MessageTypeRPCResponse, nil)
	s.registry.RegisterPendingResponse("mid-2", fullCh)
	respMsg2 := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{"status": "success"})
	respMsg2.ID = "mid-2"
	s.handleRPCResponse(respMsg2) // default 丢弃
	if len(fullCh) != 1 {
		t.Fatal("full channel should not accept second response")
	}
}

// ============ system_info.go ============

func TestCovHandleSystemInfo(t *testing.T) {
	s := covNewServer(t)

	s.handleSystemInfo("agent-si", nil) // nil 直接返回

	info := &protocol.SystemInfoPayload{
		CPUUsage: 33.3, CPUCores: 8, CPUFreqMHz: 2400,
		MemTotal: 1000, MemUsed: 500, MemAvailable: 500, MemUsagePercent: 50,
		DiskTotal: 2000, DiskUsed: 1000, DiskFree: 1000, DiskUsagePercent: 50,
		NetBytesSent: 10, NetBytesRecv: 20,
		OSName: "linux", OSVersion: "6", Arch: "amd64", Uptime: 99,
		Hostname: "h", Load1: 1, Load5: 2, Load15: 3,
	}
	s.handleSystemInfo("agent-si", info)

	snap, err := s.db.GetSystemInfoSnapshot("agent-si")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.CPUCores != 8 || snap.Arch != "amd64" || snap.Load15 != 3 {
		t.Errorf("snapshot = %+v", snap)
	}

	// DB 关闭后两个落库错误分支（仅记日志）
	covCloseDB(t, s)
	s.handleSystemInfo("agent-si", info)
}

// ============ server.go proxy 消息处理与 Agent 辅助 ============

func TestCovHandleProxyMessages(t *testing.T) {
	s := covNewServer(t)
	agent := NewAgent("agent-proxy", nil)

	// proxyMgr 为 nil：直接返回
	s.handleProxyData(agent, protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "p", "connId": "c", "data": "aGk=",
	}))
	s.handleProxyClose(agent, protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
		"proxyId": "p", "connId": "c",
	}))

	s.proxyMgr = proxy.NewManager(s, s.db)

	// 解码失败（payload 为 nil）
	s.handleProxyData(agent, protocol.NewMessage(protocol.MessageTypeProxyData, nil))
	s.handleProxyClose(agent, protocol.NewMessage(protocol.MessageTypeProxyClose, nil))
	// proxy_error 解码失败 + 正常日志
	s.handleProxyError(agent, protocol.NewMessage(protocol.MessageTypeProxyError, nil))
	s.handleProxyError(agent, protocol.NewMessage(protocol.MessageTypeProxyError, map[string]interface{}{
		"proxyId": "p1", "error": "conn refused",
	}))

	// terminal 显式标记 / 前缀 → HandleTerminalData（无会话，no-op）
	s.handleProxyData(agent, protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "other", "connId": "tc1", "data": "aGk=", "terminal": true,
	}))
	s.handleProxyData(agent, protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "terminal-xyz", "connId": "tc2", "data": "aGk=",
	}))
	// vnc 前缀 → HandleVNCData（无会话，no-op）
	s.handleProxyData(agent, protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "vnc-xyz", "connId": "vc1", "data": "aGk=",
	}))
	// 普通代理通道（proxyMgr 无该代理 → 错误日志）
	s.handleProxyData(agent, protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "plain-1", "connId": "cc", "data": "aGk=",
	}))

	// close：terminal / vnc / 普通三路
	s.handleProxyClose(agent, protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
		"proxyId": "other", "connId": "tc1", "terminal": true,
	}))
	s.handleProxyClose(agent, protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
		"proxyId": "vnc-xyz", "connId": "vc1",
	}))
	s.handleProxyClose(agent, protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
		"proxyId": "plain-1", "connId": "cc",
	}))
}

func TestCovAgentCloseAndSendMessage(t *testing.T) {
	s := covNewServer(t)
	covWSReady(s)

	// 真连接上的 Close（Conn.Close 分支）
	upgraded := make(chan *websocket.Conn, 1)
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := s.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		upgraded <- conn
	}))
	t.Cleanup(httpSrv.Close)
	client := covDialWS(t, covWSStrip(httpSrv.URL)+"/ws", nil)
	serverConn := <-upgraded
	client.Close()

	agent := NewAgent("agent-close", serverConn)
	agent.Close() // 覆盖 Conn.Close 分支 + closed 标记
	if err := agent.SendMessage(protocol.NewMessage(protocol.MessageTypePing, nil)); err == nil {
		t.Error("SendMessage on closed agent should fail")
	}
	agent.Close() // 幂等

	// 发送通道满
	full := NewAgent("agent-full", nil)
	for i := 0; i < 256; i++ {
		full.Send <- protocol.NewMessage(protocol.MessageTypePing, nil)
	}
	if err := full.SendMessage(protocol.NewMessage(protocol.MessageTypePing, nil)); err == nil {
		t.Error("SendMessage with full channel should fail")
	}

	// 并发 Close + SendMessage：sendMu 互斥下不再出现 send-on-closed-channel
	// panic（旧实现靠 recover 兜底，仍是数据竞争）
	var wg sync.WaitGroup
	wg.Add(2)
	concurrent := NewAgent("agent-concurrent", nil)
	go func() { defer wg.Done(); concurrent.Close() }()
	go func() {
		defer wg.Done()
		_ = concurrent.SendMessage(protocol.NewMessage(protocol.MessageTypePing, nil))
	}()
	wg.Wait()
	if err := concurrent.SendMessage(protocol.NewMessage(protocol.MessageTypePing, nil)); err == nil {
		t.Error("SendMessage after Close should fail")
	}
}

func TestCovCallAgentAndSendHelpers(t *testing.T) {
	s := covNewServer(t)

	// CallAgent：agent 不存在
	if _, err := s.CallAgent("ghost", "x", nil); err != ErrAgentNotFound {
		t.Fatalf("CallAgent missing agent err = %v", err)
	}
	_ = s

	// SendToAgent / GetAgentConn 两分支
	if err := s.SendToAgent("ghost", protocol.NewMessage(protocol.MessageTypePing, nil)); err == nil {
		t.Error("SendToAgent missing agent should fail")
	}
	if _, ok := s.GetAgentConn("ghost"); ok {
		t.Error("GetAgentConn missing agent should be false")
	}

	covFakeAgent(t, s, "agent-call", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"echo": method})
	})
	if err := s.SendToAgent("agent-call", protocol.NewMessage(protocol.MessageTypePing, nil)); err != nil {
		t.Errorf("SendToAgent: %v", err)
	}
	if conn, ok := s.GetAgentConn("agent-call"); !ok || conn == nil {
		t.Error("GetAgentConn should find agent")
	}

	resp, err := s.CallAgent("agent-call", "demo.run", map[string]interface{}{"a": 1})
	if err != nil {
		t.Fatalf("CallAgent: %v", err)
	}
	var payload protocol.RPCResponsePayload
	raw, _ := json.Marshal(resp.Payload)
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "success" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestCovCallAgentSendTimeout(t *testing.T) {
	// 注入短超时后 -short 也能跑（生产默认 5s）
	covShortIntervals(t)
	s := covNewServer(t)
	covStuckAgent(t, s, "agent-stuck") // Send 塞满 → 发送超时
	if _, err := s.CallAgent("agent-stuck", "x", nil); err == nil || !strings.Contains(err.Error(), "send timeout") {
		t.Fatalf("err = %v, want send timeout", err)
	}
}

func TestCovToStorageAgent(t *testing.T) {
	agent := NewAgent("agent-conv", nil)
	agent.Hostname = "h"
	agent.IP = "1.2.3.4"
	agent.Location = protocol.Location{Region: "r", Zone: "z"}
	agent.Labels = map[string]interface{}{"k": "v"}
	agent.Virtualization = &protocol.VirtualizationInfo{Type: "kvm", Role: "guest"}
	agent.Capabilities = []protocol.Capability{
		{Type: "docker", Version: "1", Endpoint: "unix:///var/run/docker.sock", Metadata: map[string]interface{}{"m": "n"}},
	}

	got := toStorageAgent(agent)
	if got.ID != "agent-conv" || got.Hostname != "h" || got.Status != "online" {
		t.Errorf("got = %+v", got)
	}
	if got.VirtType != "kvm" || got.VirtRole != "guest" {
		t.Errorf("virtualization not converted: %s/%s", got.VirtType, got.VirtRole)
	}
	if len(got.Capabilities) != 1 {
		t.Fatalf("capabilities = %+v", got.Capabilities)
	}
	cfg := got.Capabilities[0].Config
	if cfg["endpoint"] != "unix:///var/run/docker.sock" || cfg["m"] != "n" {
		t.Errorf("capability config = %+v", cfg)
	}
}
