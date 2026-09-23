package server

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/gorilla/websocket"
)

// ============ Guacamole 指令编码（照抄官方 Tunnel 的长度前缀格式） ============

func TestGuacEncodeLengthPrefix(t *testing.T) {
	// 格式：长度.opcode,长度.arg...;（见设计风险章节「照抄官方 Tunnel」）
	got := guacEncode("size", "1024", "768", "96")
	want := "4.size,4.1024,3.768,2.96;"
	if got != want {
		t.Errorf("guacEncode = %q, want %q", got, want)
	}

	// 无参数指令
	if got := guacEncode("sync"); got != "4.sync;" {
		t.Errorf("guacEncode(no args) = %q", got)
	}

	// 多字节内容按字节长度计（UTF-8 安全：协议是字节流）
	got = guacEncode("clipboard", "text/plain", "héllo")
	if !strings.HasPrefix(got, "9.clipboard,10.text/plain,") {
		t.Errorf("UTF-8 length prefix = %q", got)
	}
}

func TestGuacConnectArgsRDP(t *testing.T) {
	params := map[string]string{
		"username": "admin",
		"password": "s3cret",
		"domain":   "CORP",
	}
	args := guacConnectArgs("rdp", "10.0.0.9", 3389, params, 1280, 800, false, "sid-1")

	joined := strings.Join(args, ";")
	for _, want := range []string{
		"hostname=10.0.0.9",
		"port=3389",
		"username=admin",
		"password=s3cret",
		"domain=CORP",
		"security=any",
		"ignore-cert=true",
		"color-depth=32",
		"width=1280",
		"height=800",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("rdp args missing %q in %q", want, joined)
		}
	}
	// 录制关闭时不传 recording-*（音频同理不传 audio 参数）
	if strings.Contains(joined, "recording-") {
		t.Errorf("record=false should not carry recording-*: %q", joined)
	}
}

func TestGuacConnectArgsVNCAndRecording(t *testing.T) {
	params := map[string]string{"password": "vncpw"}
	args := guacConnectArgs("vnc", "10.0.0.5", 5900, params, 0, 0, true, "sid-2")
	joined := strings.Join(args, ";")

	if !strings.Contains(joined, "port=5900") {
		t.Errorf("vnc args missing port: %q", joined)
	}
	// VNC 无 width/height 参数（0 值不传）
	if strings.Contains(joined, "width=") {
		t.Errorf("vnc should not carry width: %q", joined)
	}
	// 录制开启：recording-path 指向 guacd 卷、recording-name 用会话 ID
	if !strings.Contains(joined, "recording-path=/var/lib/guacamole") {
		t.Errorf("record=true missing recording-path: %q", joined)
	}
	if !strings.Contains(joined, "recording-name=sid-2") {
		t.Errorf("record=true missing recording-name: %q", joined)
	}
}

func TestGuacConnectArgsNoCredentials(t *testing.T) {
	args := guacConnectArgs("rdp", "h", 3389, map[string]string{}, 0, 0, false, "s")
	joined := strings.Join(args, ";")
	for _, bad := range []string{"username=", "password=", "domain="} {
		if strings.Contains(joined, bad) {
			t.Errorf("empty params should not emit %q: %q", bad, joined)
		}
	}
}

// ============ HTTP 失败分支（升级前） ============

func TestGuacamoleWebSocketBadRequests(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)

	// 缺票据（无 Sec-WebSocket-Protocol）
	rec := covRec()
	s.handleGuacamoleWebSocket(rec, covReq(http.MethodGet, "/api/remote/guacamole", nil))
	covWantCode(t, "missing ticket", rec, http.StatusUnauthorized)

	// 无效票据
	rec = covRec()
	s.handleGuacamoleWebSocket(rec, guacReqWithTicket("bogus-ticket"))
	covWantCode(t, "invalid ticket", rec, http.StatusUnauthorized)

	// 缺连接参数（票据 params 为空）
	rec = covRec()
	s.handleGuacamoleWebSocket(rec, guacReqWithTicket(covTicket(t, s, map[string]string{})))
	covWantCode(t, "missing params", rec, http.StatusBadRequest)

	// 端口非法
	rec = covRec()
	s.handleGuacamoleWebSocket(rec, guacReqWithTicket(covTicket(t, s, map[string]string{
		"agent_id": "a", "host": "h", "port": "not-a-port", "protocol": "rdp",
	})))
	covWantCode(t, "invalid port", rec, http.StatusBadRequest)

	// 端口越界
	rec = covRec()
	s.handleGuacamoleWebSocket(rec, guacReqWithTicket(covTicket(t, s, map[string]string{
		"agent_id": "a", "host": "h", "port": "70000", "protocol": "rdp",
	})))
	covWantCode(t, "port out of range", rec, http.StatusBadRequest)

	// 协议不支持（ssh/telnet 走 TerminalModal，见设计「不做」）
	for _, proto := range []string{"ssh", "telnet"} {
		rec = covRec()
		s.handleGuacamoleWebSocket(rec, guacReqWithTicket(covTicket(t, s, map[string]string{
			"agent_id": "a", "host": "h", "port": "22", "protocol": proto,
		})))
		covWantCode(t, "protocol "+proto, rec, http.StatusBadRequest)
	}
}

func TestGuacamoleEgressDenied(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	// 收紧出口策略：不放行任意目标、allow-list 为空 → 全拒
	s.cfg.RemoteControl = &config.RemoteControlConfig{AllowArbitraryTarget: false}

	rec := covRec()
	s.handleGuacamoleWebSocket(rec, guacReqWithTicket(covTicket(t, s, map[string]string{
		"agent_id": "a", "host": "forbidden.example", "port": "3389", "protocol": "rdp",
	})))
	covWantCode(t, "egress denied", rec, http.StatusForbidden)
}

func TestGuacamoleGuacdUnreachable(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	t.Setenv("GUACD_ADDR", "127.0.0.1:1") // 拒绝连接的端口

	rec := covRec()
	s.handleGuacamoleWebSocket(rec, guacReqWithTicket(covTicket(t, s, map[string]string{
		"agent_id": "a", "host": "10.0.0.9", "port": "3389", "protocol": "rdp",
	})))
	covWantCode(t, "guacd unreachable", rec, http.StatusBadGateway)
}

// ============ 全链路：mock guacd TCP ↔ 浏览器 WS 双向管道 ============

// covGuacdServer 内存 guacd：收握手指令、可双向推送字节
type covGuacdServer struct {
	ln      net.Listener
	mu      sync.Mutex
	handled []string // 收到的原始字节（断言握手指令用）
	conn    net.Conn
}

func covStartGuacd(t *testing.T) *covGuacdServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := &covGuacdServer{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		g.mu.Lock()
		g.conn = c
		g.mu.Unlock()
		buf := make([]byte, 64*1024)
		for {
			n, err := c.Read(buf)
			if n > 0 {
				got := string(buf[:n])
				g.mu.Lock()
				g.handled = append(g.handled, got)
				g.mu.Unlock()
				// Guacamole 协议握手：收到 select 后回参数列表指令
				//（真 guacd 行为；网关读此响应后才发 size+connect）
				if strings.Contains(got, "6.select,") {
					_, _ = c.Write([]byte("6.select,8.hostname,4.port;"))
				}
			}
			if err != nil {
				return
			}
		}
	}()
	t.Setenv("GUACD_ADDR", ln.Addr().String())
	return g
}

func (g *covGuacdServer) handshakeText() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return strings.Join(g.handled, "")
}

func (g *covGuacdServer) sendToGateway(t *testing.T, payload string) {
	t.Helper()
	g.mu.Lock()
	c := g.conn
	g.mu.Unlock()
	if c == nil {
		t.Fatal("guacd has no connection yet")
	}
	if _, err := c.Write([]byte(payload)); err != nil {
		t.Fatalf("guacd write: %v", err)
	}
}

func TestGuacamoleTunnelFullFlow(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	g := covStartGuacd(t)

	ticket := covTicket(t, s, map[string]string{
		"agent_id": "agent-g", "host": "10.0.0.9", "port": "3389", "protocol": "rdp",
		"username": "admin", "password": "pw", "width": "1024", "height": "768",
	})

	conn, _, tDone := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole", ticket)
	t.Cleanup(func() { conn.Close() })

	// 首条帧 = tunnel UUID（INTERNAL_DATA 单元素指令，common-js 首条指令
	// setUUID；opcode 长度 0）。net.Pipe 同步无缓冲——writeWS 会阻塞到客户端
	// 读完，必须先消费掉才能放行后续 guacd handshake 写入。
	_, uuidFrame, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	if !strings.HasPrefix(string(uuidFrame), "0.,") {
		t.Errorf("tunnel uuid frame should be INTERNAL_DATA (empty opcode), got %q", uuidFrame)
	}
	if !strings.Contains(string(uuidFrame), ";") {
		t.Errorf("uuid frame should be a complete instruction, got %q", uuidFrame)
	}

	// 等 guacd 收到握手：size + connect（含凭据与分辨率，不解析参数内容）
	covWaitGone(t, "handshake", func() bool {
		return strings.Contains(g.handshakeText(), "7.connect")
	})
	hs := g.handshakeText()
	if !strings.HasPrefix(hs, "6.select,3.rdp;") {
		t.Errorf("handshake should start with select, got %q", hs)
	}
	if !strings.Contains(hs, "4.size,4.1024,3.768") {
		t.Errorf("handshake missing size, got %q", hs)
	}
	for _, want := range []string{"username=admin", "password=pw", "hostname=10.0.0.9"} {
		if !strings.Contains(hs, want) {
			t.Errorf("handshake missing %q in %q", want, hs)
		}
	}
	// 音频/视频不传（阶段二，见设计风险章节）
	if strings.Contains(hs, "5.audio") || strings.Contains(hs, "5.video") {
		t.Errorf("audio/video should not be negotiated in phase 1: %q", hs)
	}

	// guacd → 浏览器：按指令边界（分号）切分下发
	g.sendToGateway(t, "4.sync,1.0;")
	g.sendToGateway(t, "5.error,4.test;") // 两条连发 → 两帧
	_, frame1, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read frame1: %v", err)
	}
	_, frame2, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read frame2: %v", err)
	}
	if string(frame1) != "4.sync,1.0;" || string(frame2) != "5.error,4.test;" {
		t.Errorf("frames = %q / %q", frame1, frame2)
	}

	// 浏览器 → guacd：WS 文本帧直写 TCP（不解析内容）
	if err := conn.WriteMessage(websocket.TextMessage, []byte("4.move,3.100,2.50;")); err != nil {
		t.Fatalf("write: %v", err)
	}
	covWaitGone(t, "client->guacd", func() bool {
		return strings.Contains(g.handshakeText(), "4.move,3.100,2.50;")
	})

	// 关闭：WS 断开 → guacd conn 关闭 + 审计结束（handler 返回即完成）
	conn.Close()
	covWaitHandlerExit(t, "guacamole session close", tDone)
}

// guacReqWithTicket 构造带票据子协议的 GET 请求（同 terminal/desktop 测试）
func guacReqWithTicket(ticket string) *http.Request {
	r := covReq(http.MethodGet, "/api/remote/guacamole", nil)
	r.Header.Set("Sec-WebSocket-Protocol", ticket)
	return r
}

// ============ 隧道内部控制指令（INTERNAL_DATA_OPCODE） ============

func TestGuacIsInternal(t *testing.T) {
	// opcode 长度 0 = Guacamole.Tunnel.INTERNAL_DATA_OPCODE
	if !guacIsInternal([]byte("0.,4.ping,13.12345;")) {
		t.Error("empty-opcode instruction should be internal")
	}
	if !guacIsInternal([]byte("0.,36.3f8f0a9b-c1d2-4e5f-8a9b-0c1d2e3f4a5b;")) {
		t.Error("tunnel uuid (internal single-element) should be internal")
	}
	// 普通指令 opcode 非空
	for _, frame := range []string{"4.sync,1.0;", "4.size,4.1024,3.768;", "4.move,3.100,2.50;", "10.clipboard,x;"} {
		if guacIsInternal([]byte(frame)) {
			t.Errorf("%q should not be internal", frame)
		}
	}
	// 非法帧（无长度前缀/非数字前缀）不判为 internal（走 guacd，交上游裁决）
	for _, frame := range []string{"", ".", "a.b;", "..x;"} {
		if guacIsInternal([]byte(frame)) {
			t.Errorf("%q should not be internal", frame)
		}
	}
}

func TestGuacamoleInternalPingEchoedNotForwarded(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	g := covStartGuacd(t)

	ticket := covTicket(t, s, map[string]string{
		"agent_id": "agent-p", "host": "10.0.0.9", "port": "5900", "protocol": "vnc",
	})
	conn, _, tDone := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole", ticket)
	t.Cleanup(func() { conn.Close() })

	// 消费 tunnel UUID 帧（见 TestGuacamoleTunnelFullFlow 时序说明）
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	covWaitGone(t, "handshake", func() bool {
		return strings.Contains(g.handshakeText(), "7.connect")
	})
	before := g.handshakeText()

	// ping（INTERNAL_DATA）→ 原样回显给浏览器，绝不进 guacd
	ping := "0.,4.ping,13.175000000000;"
	if err := conn.WriteMessage(websocket.TextMessage, []byte(ping)); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	_, echo, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ping echo: %v", err)
	}
	if string(echo) != ping {
		t.Errorf("ping echo = %q, want identical %q", echo, ping)
	}
	if got := g.handshakeText(); got != before {
		t.Errorf("internal instruction must not reach guacd: %q", got[len(before):])
	}

	// 同理 tunnel uuid / 其他 internal 也回显不转发
	other := "0.,5.uuid1;"
	if err := conn.WriteMessage(websocket.TextMessage, []byte(other)); err != nil {
		t.Fatalf("write internal: %v", err)
	}
	_, echo2, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read internal echo: %v", err)
	}
	if string(echo2) != other {
		t.Errorf("internal echo = %q, want %q", echo2, other)
	}

	// 普通指令仍转发 guacd
	if err := conn.WriteMessage(websocket.TextMessage, []byte("4.sync,9.175000000;")); err != nil {
		t.Fatalf("write sync: %v", err)
	}
	covWaitGone(t, "sync forwarded", func() bool {
		return strings.Contains(g.handshakeText(), "4.sync,9.175000000;")
	})

	conn.Close()
	covWaitHandlerExit(t, "guacamole ping session close", tDone)
}

func TestGuacamoleTicketViaQuery(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	g := covStartGuacd(t)

	// common-js WebSocketTunnel 硬编码 subprotocol "guacamole"，
	// 票据只能走 URL query（client.connect(data) → new WebSocket(url+"?"+data)）
	ticket := covTicket(t, s, map[string]string{
		"agent_id": "agent-q", "host": "10.0.0.9", "port": "5900", "protocol": "vnc",
	})
	conn, _, tDone := covDirectWSJoined(t, s.handleGuacamoleWebSocket,
		"/api/remote/guacamole?ticket="+ticket, ticket)
	t.Cleanup(func() { conn.Close() })

	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	covWaitGone(t, "handshake via query ticket", func() bool {
		return strings.Contains(g.handshakeText(), "7.connect")
	})
	conn.Close()
	covWaitHandlerExit(t, "query ticket session close", tDone)
}

// ============ M3：.guac 录制收集与 Format 分流 ============

func TestRecordingExt(t *testing.T) {
	// cast=asciinema 流（含空值，兼容 M1/M2 旧数据）；guac=Guacamole 会话流
	for _, c := range []struct {
		format string
		want   string
	}{
		{"guac", ".guac"},
		{"cast", ".cast"},
		{"", ".cast"},
		{"other", ".cast"},
	} {
		if got := recordingExt(c.format); got != c.want {
			t.Errorf("recordingExt(%q) = %q, want %q", c.format, got, c.want)
		}
	}
}

func TestGuacRecordingCollectAndFinish(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	g := covStartGuacd(t)
	// guacd 录制目录与 server 收集源同一路径（M3 D3：两侧视角合一）
	t.Setenv("GUACD_RECORDING_PATH", t.TempDir())

	ticket := covTicket(t, s, map[string]string{
		"agent_id": "agent-rec", "host": "10.0.0.9", "port": "3389", "protocol": "rdp",
	})
	conn, _, tDone := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole", ticket)
	t.Cleanup(func() { conn.Close() })

	// tunnel UUID 帧（net.Pipe 同步语义下必须先消费）
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	covWaitGone(t, "handshake", func() bool {
		return strings.Contains(g.handshakeText(), "7.connect")
	})

	// 会话开始登记元数据（Format=guac，进行中也在列）
	var sid string
	covWaitGone(t, "recording meta", func() bool {
		list, err := s.db.ListTerminalRecordings(10)
		if err != nil || len(list) == 0 {
			return false
		}
		rec := list[0]
		if rec.Format != "guac" || rec.Protocol != "rdp" {
			t.Fatalf("recording meta = %+v, want format=guac protocol=rdp", rec)
		}
		if rec.DurationMs != 0 {
			t.Errorf("in-progress duration = %d, want 0", rec.DurationMs)
		}
		sid = rec.SessionID
		return true
	})

	// 模拟 guacd 落盘 <sid>.guac（真实 guacd 写该文件；测试直接造）
	guacDir := guacamoleRecordingPath()
	if err := os.MkdirAll(guacDir, 0700); err != nil {
		t.Fatal(err)
	}
	guacSrc := filepath.Join(guacDir, sid+".guac")
	if err := os.WriteFile(guacSrc, []byte("fake-guac-bytes"), 0600); err != nil {
		t.Fatal(err)
	}

	// 关闭会话 → 收集到 recordingsDir + 回填 duration/bytes
	conn.Close()
	covWaitHandlerExit(t, "guac recording session close", tDone)

	covWaitGone(t, "collected .guac", func() bool {
		dst := filepath.Join(s.recordingsDir(), sid+".guac")
		info, err := os.Stat(dst)
		return err == nil && info.Size() == int64(len("fake-guac-bytes"))
	})
	// 源文件已清（防 guacd 卷膨胀）
	if _, err := os.Stat(guacSrc); !os.IsNotExist(err) {
		t.Errorf("source .guac should be removed after collect, stat err = %v", err)
	}
	// 元数据回填
	covWaitGone(t, "recording meta finished", func() bool {
		rec, err := s.db.GetTerminalRecording(sid)
		return err == nil && rec.DurationMs > 0 && rec.Bytes == int64(len("fake-guac-bytes"))
	})
}

func TestGuacRecordingDisabledNotRegistered(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	g := covStartGuacd(t)
	t.Setenv("GUACD_RECORDING_PATH", t.TempDir())
	// 关闭录制开关（M3 D6：复用 recording.enabled）
	if err := s.db.SetSetting(RecordingEnabledSettingKey, "false"); err != nil {
		t.Fatal(err)
	}

	ticket := covTicket(t, s, map[string]string{
		"agent_id": "agent-norec", "host": "10.0.0.9", "port": "5900", "protocol": "vnc",
	})
	conn, _, tDone := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole", ticket)
	t.Cleanup(func() { conn.Close() })
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	covWaitGone(t, "handshake", func() bool {
		return strings.Contains(g.handshakeText(), "7.connect")
	})
	// 关闭录制 → 不登记元数据、connect 指令不带 recording-*
	if strings.Contains(g.handshakeText(), "recording-") {
		t.Errorf("recording disabled should not carry recording-*: %q", g.handshakeText())
	}
	conn.Close()
	covWaitHandlerExit(t, "no-recording session close", tDone)
	list, err := s.db.ListTerminalRecordings(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("recording disabled should not register meta: %+v", list)
	}
}

func TestRecordingFormatSuffixInREST(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)

	// guac 形态：/{sid}/cast 按 Format 分流读 .guac（URL 不变，语义是取录制内容）
	sid := "11111111-2222-4333-8444-555555555555"
	if err := s.db.CreateTerminalRecording(&storage.TerminalRecording{
		SessionID: sid, Username: "u", AgentID: "a", Host: "h", Port: 3389,
		Protocol: "rdp", Format: "guac", StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.recordingsDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.recordingsDir(), sid+".guac"), []byte("guac-bytes"), 0600); err != nil {
		t.Fatal(err)
	}

	rec := covRec()
	s.handleRecordingCast(rec, covReq(http.MethodGet, "/api/recordings/"+sid+"/cast", nil), sid)
	if rec.Code != http.StatusOK {
		t.Fatalf("guac content code = %d", rec.Code)
	}
	if rec.Body.String() != "guac-bytes" {
		t.Errorf("guac content = %q, want guac-bytes", rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, ".guac") {
		t.Errorf("Content-Disposition = %q, want .guac", cd)
	}

	// cast 形态（空 Format 兼容旧数据）走 .cast
	sid2 := "22222222-3333-4444-8555-666666666666"
	if err := s.db.CreateTerminalRecording(&storage.TerminalRecording{
		SessionID: sid2, Username: "u", AgentID: "a", Host: "h", Port: 22,
		Protocol: "ssh", StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.recordingsDir(), sid2+".cast"), []byte("cast-bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	rec2 := covRec()
	s.handleRecordingCast(rec2, covReq(http.MethodGet, "/api/recordings/"+sid2+"/cast", nil), sid2)
	if rec2.Body.String() != "cast-bytes" {
		t.Errorf("cast content = %q, want cast-bytes", rec2.Body.String())
	}
	if cd := rec2.Header().Get("Content-Disposition"); !strings.Contains(cd, ".cast") {
		t.Errorf("Content-Disposition = %q, want .cast", cd)
	}
}
