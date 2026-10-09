package probeagent

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/core/platform"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// probeFakeServer 探针上行链集成桩：升级连接、读注册、回注册响应，之后
// 把处置交给 afterRegister（读循环/发 ping/断连由用例定制）。
type probeFakeServer struct {
	upgrader      websocket.Upgrader
	connCh        chan *websocket.Conn
	regCh         chan *protocol.Message
	afterRegister func(conn *websocket.Conn, codec *protocol.Codec)
}

func newProbeFakeServer() *probeFakeServer {
	return &probeFakeServer{
		upgrader: websocket.Upgrader{},
		connCh:   make(chan *websocket.Conn, 4),
		regCh:    make(chan *protocol.Message, 8),
	}
}

func (f *probeFakeServer) handler(w http.ResponseWriter, r *http.Request) {
	conn, err := f.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	f.connCh <- conn
	codec := protocol.NewCodec()
	msg, err := codec.ReadMessage(conn)
	if err != nil {
		return
	}
	if msg.Type != protocol.MessageTypeRegister {
		return
	}
	f.regCh <- msg
	_ = codec.WriteMessage(conn, protocol.NewMessage(protocol.MessageTypeRegister, map[string]interface{}{
		"status":     "accepted",
		"serverTime": time.Now().Unix(),
	}))
	if f.afterRegister != nil {
		f.afterRegister(conn, codec)
	}
}

func probeWsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}

// readProbeMsg 带截止地读一条消息（gorilla 读失败即永久禁读，整段
// deadline 一次给满——同主 agent readUntil 注释）。
func readProbeMsg(t *testing.T, conn *websocket.Conn, timeout time.Duration) *protocol.Message {
	t.Helper()
	codec := protocol.NewCodec()
	deadline := time.Now().Add(timeout)
	conn.SetReadDeadline(deadline)
	msg, err := codec.ReadMessage(conn)
	if err != nil {
		if time.Now().After(deadline) {
			t.Fatalf("timed out reading message: %v", err)
		}
		return nil
	}
	return msg
}

// waitRegister 轮询注册门放行。
func waitRegister(t *testing.T, u *Upstream, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if u.Registered() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("probe agent never registered")
}

// newProbeTestAgent 健康目标探针（interval 无关紧要，ProbeAll 手动驱动）。
func newProbeTestAgent(t *testing.T) (*Agent, *httptest.Server) {
	t.Helper()
	var up int32 = 1
	srv := flipServer(t, &up)
	a, err := New(&Config{Targets: []TargetConfig{{
		Name: "blog", Type: "http", URL: srv.URL, ExpectStatus: 200,
		Interval: Duration(time.Hour), Timeout: Duration(time.Second),
		Threshold: 1, Recovery: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	a.ProbeAll(context.Background())
	return a, srv
}

// TestDeriveProbeID 显式优先；派生 = probe-<host>-<machine-id 前 8>；
// 无 machine-id 回退纯 hostname。与主机 agent 的 agent- 前缀区分。
func TestDeriveProbeID(t *testing.T) {
	if got := DeriveProbeID("explicit", "web1", "abcdef123456"); got != "explicit" {
		t.Fatalf("got %q", got)
	}
	if got := DeriveProbeID("", "web1", "0123456789abcdef"); got != "probe-web1-01234567" {
		t.Fatalf("got %q", got)
	}
	if got := DeriveProbeID("", "web1", ""); got != "probe-web1" {
		t.Fatalf("got %q", got)
	}
	if got := DeriveProbeID("", "web1", "short"); got != "probe-web1-short" {
		t.Fatalf("got %q", got)
	}
}

// TestClampFallbackInterval 兜底周期夹在 [30s, 10m]。
func TestClampFallbackInterval(t *testing.T) {
	if got := clampFallbackInterval(time.Second); got != upstreamFallbackFloor {
		t.Fatalf("got %v", got)
	}
	if got := clampFallbackInterval(0); got != upstreamFallbackFloor {
		t.Fatalf("got %v", got)
	}
	if got := clampFallbackInterval(20 * time.Minute); got != upstreamFallbackCeiling {
		t.Fatalf("got %v", got)
	}
	if got := clampFallbackInterval(5 * time.Minute); got != 5*time.Minute {
		t.Fatalf("got %v", got)
	}
}

// TestUpstreamRegisterReportAndPong 全链：拨号→注册→ping→pong→RPC 拒答
// →未知类型静默→probe_report 全量上行→Stop 优雅退出。
func TestUpstreamRegisterReportAndPong(t *testing.T) {
	a, _ := newProbeTestAgent(t)

	msgs := make(chan *protocol.Message, 8)
	fs := newProbeFakeServer()
	fs.afterRegister = func(conn *websocket.Conn, codec *protocol.Codec) {
		ping := protocol.NewMessage(protocol.MessageTypePing, nil)
		ping.ID = "probe-ping-1"
		_ = codec.WriteMessage(conn, ping)
		rpc := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
			"method": "noop",
		})
		rpc.ID = "probe-rpc-1"
		_ = codec.WriteMessage(conn, rpc)
		// 未知类型走 handleMessage default 静默
		_ = codec.WriteMessage(conn, protocol.NewMessage(protocol.MessageType("probe_unknown"), nil))
		for {
			m := readProbeMsg(t, conn, 5*time.Second)
			if m == nil {
				return
			}
			select {
			case msgs <- m:
			default:
			}
		}
	}
	wsrv := httptest.NewServer(http.HandlerFunc(fs.handler))
	defer wsrv.Close()

	u := NewUpstream(a, &Config{
		Server:   probeWsURL(wsrv),
		Interval: Duration(time.Second),
	}, "test")
	if u.ID() == "" || !strings.HasPrefix(u.ID(), "probe-") {
		t.Fatalf("id = %q", u.ID())
	}

	done := make(chan struct{})
	go func() { u.Run(); close(done) }()

	select {
	case <-fs.regCh:
	case <-time.After(5 * time.Second):
		t.Fatal("register never arrived")
	}
	waitRegister(t, u, 5*time.Second)

	// 迁移即发信号 → 全量 probe_report 上行（ping 的 pong 也走队列注册门）
	u.Notify()
	deadline := time.Now().Add(5 * time.Second)
	var pong, report, rpcResp *protocol.Message
	for time.Now().Before(deadline) && (pong == nil || report == nil || rpcResp == nil) {
		select {
		case m := <-msgs:
			switch m.Type {
			case protocol.MessageTypeHeartbeat:
				if pong == nil {
					pong = m
				}
			case protocol.MessageTypeProbeReport:
				if report == nil {
					report = m
				}
			case protocol.MessageTypeRPCResponse:
				if rpcResp == nil {
					rpcResp = m
				}
			}
		case <-time.After(500 * time.Millisecond):
		}
	}
	if pong == nil {
		t.Fatal("pong never arrived")
	}
	if report == nil {
		t.Fatal("probe_report never arrived")
	}
	if rpcResp == nil {
		t.Fatal("rpc_response never arrived")
	}
	if rpcResp.ID != "probe-rpc-1" {
		t.Fatalf("rpc id = %q, want probe-rpc-1", rpcResp.ID)
	}
	if rpcResp.Payload["status"] != "error" {
		t.Fatalf("rpc payload = %+v, want error status", rpcResp.Payload)
	}
	targets, ok := report.Payload["targets"].([]interface{})
	if !ok || len(targets) != 1 {
		t.Fatalf("report targets = %#v", report.Payload["targets"])
	}
	t0 := targets[0].(map[string]interface{})
	if t0["name"] != "blog" || t0["state"] != "healthy" {
		t.Fatalf("target = %+v", t0)
	}

	u.Stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Stop")
	}
}

// TestUpstreamReconnectAfterWriteFailure 服务端断连 → 写失败触发重连 →
// 二次注册（退避间隔测试注入为毫秒级）。
func TestUpstreamReconnectAfterWriteFailure(t *testing.T) {
	oldDelay, oldMax := upstreamReconnectDelay, upstreamReconnectMaxDelay
	upstreamReconnectDelay, upstreamReconnectMaxDelay = 10*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { upstreamReconnectDelay, upstreamReconnectMaxDelay = oldDelay, oldMax })

	a, _ := newProbeTestAgent(t)

	fs := newProbeFakeServer()
	fs.afterRegister = func(conn *websocket.Conn, codec *protocol.Codec) {
		_ = conn.Close() // 注册即断 → agent 侧重连
	}
	wsrv := httptest.NewServer(http.HandlerFunc(fs.handler))
	defer wsrv.Close()

	u := NewUpstream(a, &Config{Server: probeWsURL(wsrv), Interval: Duration(time.Second)}, "test")
	go u.Run()
	defer u.Stop()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(fs.regCh) >= 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("registers = %d, want >= 2 (reconnect re-register)", len(fs.regCh))
}

// TestUpstreamConnectRetryThenServe 初始拨号失败 → 退避重试 → 端口起来
// 后注册到达（覆盖 Run 初连失败分支与重试成功）。
func TestUpstreamConnectRetryThenServe(t *testing.T) {
	oldDelay, oldMax := upstreamReconnectDelay, upstreamReconnectMaxDelay
	upstreamReconnectDelay, upstreamReconnectMaxDelay = 10*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { upstreamReconnectDelay, upstreamReconnectMaxDelay = oldDelay, oldMax })

	a, _ := newProbeTestAgent(t)

	// 占一个端口再放开：拨号必败（端口暂无监听）
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	u := NewUpstream(a, &Config{
		Server:   fmt.Sprintf("ws://127.0.0.1:%d/ws", port),
		Interval: Duration(time.Second),
	}, "test")
	go u.Run()
	defer u.Stop()

	// 休眠一拍让 Run 的首个拨号先落败（go 语句不保证先于重监听被调度，
	// 不等会竞态成「首拨即中」——既测不了重试也盖不住失败分支）
	time.Sleep(50 * time.Millisecond)

	// 端口重新监听：重试拨号应成功并注册
	fs := newProbeFakeServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", fs.handler)
	srv := &http.Server{Handler: mux}
	ln2, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Skipf("port %d was taken between close and re-listen: %v", port, err)
	}
	defer func() { _ = srv.Close() }()
	go func() { _ = srv.Serve(ln2) }()

	select {
	case <-fs.regCh:
	case <-time.After(10 * time.Second):
		t.Fatal("register never arrived after retry")
	}
}

// TestUpstreamNoServerIdleLoop 服务端永不监听：Run 首拨失败走重连退避，
// 读循环在无连接分支空转（1s 兜底定时器 + Stop 时 ctx.Done 出口）。
func TestUpstreamNoServerIdleLoop(t *testing.T) {
	oldDelay, oldMax := upstreamReconnectDelay, upstreamReconnectMaxDelay
	upstreamReconnectDelay, upstreamReconnectMaxDelay = 10*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { upstreamReconnectDelay, upstreamReconnectMaxDelay = oldDelay, oldMax })

	a, _ := newProbeTestAgent(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close() // 只借端口不重监听：拨号恒败

	u := NewUpstream(a, &Config{
		Server:   fmt.Sprintf("ws://127.0.0.1:%d/ws", port),
		Interval: Duration(time.Second),
	}, "test")
	done := make(chan struct{})
	go func() { u.Run(); close(done) }()

	// 读循环无连接空转至少一轮 1s 兜底定时器
	time.Sleep(1300 * time.Millisecond)

	u.Stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Stop")
	}
}

// TestUpstreamRegisterNeverAccepted 服务端收注册即断、不回包：注册应答读
// 失败 → Run 注册失败分支 + 重连循环再注册失败分支（探针反复重试）。
func TestUpstreamRegisterNeverAccepted(t *testing.T) {
	oldDelay, oldMax := upstreamReconnectDelay, upstreamReconnectMaxDelay
	upstreamReconnectDelay, upstreamReconnectMaxDelay = 10*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { upstreamReconnectDelay, upstreamReconnectMaxDelay = oldDelay, oldMax })

	a, _ := newProbeTestAgent(t)

	attempts := make(chan struct{}, 16)
	upgrader := websocket.Upgrader{}
	wsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		codec := protocol.NewCodec()
		msg, err := codec.ReadMessage(conn)
		if err != nil || msg.Type != protocol.MessageTypeRegister {
			return
		}
		attempts <- struct{}{}
		_ = conn.Close() // 收注册即断：注册应答读必败
	}))
	defer wsrv.Close()

	u := NewUpstream(a, &Config{Server: probeWsURL(wsrv), Interval: Duration(time.Second)}, "test")
	go u.Run()
	defer u.Stop()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(attempts) >= 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("register attempts = %d, want >= 2 (initial + reconnect re-register)", len(attempts))
}

// TestUpstreamNilHostAndNotifyDrop 无平台宿主回退（machineID 空串 → ID 无
// machine 段）+ 预启动双 Notify：信号缓冲 1，第二发走丢弃分支。
func TestUpstreamNilHostAndNotifyDrop(t *testing.T) {
	orig := platform.Current()
	defer platform.Register(orig)
	platform.Register(nil)

	a, _ := newProbeTestAgent(t)
	u := NewUpstream(a, &Config{
		Server:   "ws://127.0.0.1:1/ws",
		Interval: Duration(time.Second),
	}, "test")

	hostname, _ := os.Hostname()
	if u.ID() != "probe-"+hostname {
		t.Fatalf("id = %q, want probe-%s (no machine segment)", u.ID(), hostname)
	}

	// 消费循环未启动前双发：缓冲 1 落一、default 丢一
	u.Notify()
	u.Notify()
}

// TestUpstreamWriteFailQueueFullClosures 断连后队满错误面白盒补盖：统一
// 上行写失败→onWriteErr、兜底 ticker 灌满队列→Enqueue 失败（reportWorker
// 日志分支、心跳 Send 失败→OnSendFail、pong 与 RPC 拒答入队失败）、重连
// CAS 防并发守卫。这些分支正常流转仅在竞态窗口可达（注册门放行与连接
// 断开同时成立），以注册门/队列直操纵确定性触达。
func TestUpstreamWriteFailQueueFullClosures(t *testing.T) {
	oldDelay, oldMax := upstreamReconnectDelay, upstreamReconnectMaxDelay
	upstreamReconnectDelay, upstreamReconnectMaxDelay = 10*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { upstreamReconnectDelay, upstreamReconnectMaxDelay = oldDelay, oldMax })

	a, _ := newProbeTestAgent(t)

	fs := newProbeFakeServer()
	fs.afterRegister = func(conn *websocket.Conn, codec *protocol.Codec) {
		_ = conn.Close() // 注册即断
	}
	wsrv := httptest.NewServer(http.HandlerFunc(fs.handler))

	u := NewUpstream(a, &Config{Server: probeWsURL(wsrv), Interval: Duration(time.Second)}, "test")
	u.fallbackInterval = time.Millisecond // 兜底 ticker 加速灌队
	go u.Run()
	defer u.Stop()

	waitRegister(t, u, 5*time.Second)
	wsrv.Close() // 服务端下线：重连拨号恒败，注册门此后只受测试操纵

	// 等断连被感知、注册门落 false（读循环读错 → 重连 → closeConn）
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && u.Registered() {
		time.Sleep(10 * time.Millisecond)
	}
	if u.Registered() {
		t.Fatal("registered flag never dropped after disconnect")
	}

	// 白盒补盖 onWriteErr：放行注册门 + 连接恒断 → 统一上行写失败分支
	// （正常流转此窗口为微秒级竞态，直操纵确定性触达）
	u.registered.Store(true)
	go u.upstream.Run() // 第二消费循环：注册门放行即写失败
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && u.registered.Load() {
		time.Sleep(5 * time.Millisecond)
	}
	if u.registered.Load() {
		t.Fatal("onWriteErr never fired (registered still set)")
	}

	// 兜底 ticker 灌满队列（1ms × 128 容量，消费循环卡放行门只进不出）
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && len(u.outbound) < cap(u.outbound) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(u.outbound) < cap(u.outbound) {
		t.Fatalf("outbound queue = %d, never filled to %d", len(u.outbound), cap(u.outbound))
	}

	// 队满错误面（注册门关着，消费循环不醒，队列保持满）：pong 与 RPC
	// 拒答的入队失败分支
	u.handleMessage(protocol.NewMessage(protocol.MessageTypePing, nil))
	u.handleMessage(protocol.NewMessage(protocol.MessageTypeRPCRequest, nil))

	// 心跳 Send 失败 → OnSendFail：放行注册门后同 goroutine 立即 Beat，
	// 消费循环要等放行门轮询才醒，队满在窗口内必然成立
	u.registered.Store(true)
	u.heartbeats.Beat()

	// 重连 CAS 防并发守卫：占位后直调第二发必走直返分支
	u.reconnecting.Store(true)
	u.reconnect()
	u.reconnecting.Store(false)
}

// TestUpstreamRegisterReadRespNilConn 注册应答读在连接缺失时报错不读：
// Stop/reconnect 可并发把 conn 置 nil，快照守卫防 nil 进 ReadMessage。
func TestUpstreamRegisterReadRespNilConn(t *testing.T) {
	a, _ := newProbeTestAgent(t)

	u := NewUpstream(a, &Config{
		Server:   "ws://127.0.0.1:1/ws",
		Interval: Duration(time.Second),
	}, "test")

	if msg, err := u.readRegisterResp(); err == nil || msg != nil {
		t.Fatalf("readRegisterResp = %v, %v; want error with nil conn", msg, err)
	}
}

// TestUpstreamStopDuringRegister 注册应答挂起时 Stop：register 失败且 ctx
// 已取消 → Run 静默返回（时序窗口守卫分支）。
func TestUpstreamStopDuringRegister(t *testing.T) {
	a, _ := newProbeTestAgent(t)

	// 收注册后不回包、保连接：register 卡在应答读直至 agent 侧关闭
	upgrader := websocket.Upgrader{}
	wsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		codec := protocol.NewCodec()
		if _, err := codec.ReadMessage(conn); err != nil {
			return
		}
		for {
			if _, err := codec.ReadMessage(conn); err != nil {
				return
			}
		}
	}))
	defer wsrv.Close()

	u := NewUpstream(a, &Config{Server: probeWsURL(wsrv), Interval: Duration(time.Second)}, "test")
	done := make(chan struct{})
	go func() { u.Run(); close(done) }()

	time.Sleep(150 * time.Millisecond) // 拨号 + 首包直写 + 应答读挂起落定
	u.Stop()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Stop during register")
	}
}
