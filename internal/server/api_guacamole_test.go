package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/cuihairu/cockpit/internal/config"
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
				g.mu.Lock()
				g.handled = append(g.handled, string(buf[:n]))
				g.mu.Unlock()
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

	// 等 guacd 收到握手：size + connect（含凭据与分辨率，不解析参数内容）
	covWaitGone(t, "handshake", func() bool {
		return strings.Contains(g.handshakeText(), "7.connect")
	})
	hs := g.handshakeText()
	if !strings.HasPrefix(hs, "4.size,4.1024,3.768") {
		t.Errorf("handshake should start with size, got %q", hs)
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
