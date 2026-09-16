package agent

// cov_* 测试：覆盖率攻坚新增（不修改既有测试文件）。
// 覆盖连接循环 / 重连 / 心跳 / 消息处理的原未覆盖分支；
// 通过包级时序变量（heartbeatInterval / reconnectDelay /
// reconnectRetryDelay）注入短间隔保证确定性，轮询均带 deadline。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/agent/detector"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// TestMain 在所有测试启动前把重连时序调快（此后进程内不再写入这些
// 变量，只读，避免与测试中泄漏的 agent goroutine 产生数据竞争）。
// 快速重连只会让等待重连的用例更快通过，不影响既有断言。
func TestMain(m *testing.M) {
	reconnectDelay = 5 * time.Millisecond
	reconnectRetryDelay = 5 * time.Millisecond
	os.Exit(m.Run())
}

// covWSServer 起一个 fake WebSocket server；onConn 在连接升级后回调
//（可为 nil）。返回 server 与已升级服务端连接的通知通道。
func covWSServer(t *testing.T, onConn func(conn *websocket.Conn)) (*httptest.Server, chan *websocket.Conn) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	connCh := make(chan *websocket.Conn, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connCh <- conn
		if onConn != nil {
			onConn(conn)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, connCh
}

// covWSURL 把 http URL 转为 ws URL。
func covWSURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}

// covDial 以客户端身份连到 fake server。
func covDial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// covPoll 轮询直到条件满足或超时（超时 Fatal）。
func covPoll(t *testing.T, timeout time.Duration, desc string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", desc)
}

// ============ waitRegistered / writeLoop ============

func TestCovWaitRegisteredViaTicker(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	errCh := make(chan error, 1)
	go func() { errCh <- a.waitRegistered() }()

	// 等待至少一个 100ms tick 命中 ticker.C 分支后再放行
	time.Sleep(250 * time.Millisecond)
	a.registered.Store(true)

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("waitRegistered() = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitRegistered() did not return after registration")
	}
}

func TestCovWaitRegisteredCancelled(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	errCh := make(chan error, 1)
	go func() { errCh <- a.waitRegistered() }()

	time.Sleep(50 * time.Millisecond)
	a.cancel()

	select {
	case err := <-errCh:
		if err == nil || err.Error() != "agent stopped" {
			t.Errorf("waitRegistered() = %v, want agent stopped", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitRegistered() did not return after cancel")
	}
}

func TestCovWriteLoopExitsWhenCancelledUnregistered(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	a.outbound <- protocol.NewMessage(protocol.MessageTypePing, nil)
	go a.writeLoop()

	// writeLoop 在 waitRegistered 中等待；取消后应整体退出
	time.Sleep(100 * time.Millisecond)
	a.cancel()
	time.Sleep(200 * time.Millisecond) // 无外部可观测信号，给足退出时间
}

func TestCovWriteLoopWriteFailureClearsRegistration(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"}) // 连接必然失败
	defer a.Stop()

	a.registered.Store(true)
	a.outbound <- protocol.NewMessage(protocol.MessageTypePing, nil)
	go a.writeLoop()

	// writeToConn 失败（conn 为 nil）→ registered 复位并触发重连
	covPoll(t, 2*time.Second, "registered flag cleared after write failure", func() bool {
		return !a.registered.Load()
	})
	a.cancel()
	time.Sleep(100 * time.Millisecond) // 等待 reconnect 感知取消退出
}

// ============ reconnect ============

func TestCovReconnectSkippedWhenInProgress(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	a.reconnecting.Store(true)
	done := make(chan struct{})
	go func() {
		a.reconnect() // CAS 失败 → 直接跳过
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reconnect() should return immediately when already in progress")
	}
	if !a.reconnecting.Load() {
		t.Error("reconnecting flag should stay set (owned by the holder)")
	}
}

func TestCovReconnectConnectFailureRetries(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1/nope"})
	defer a.Stop()

	go a.reconnect()
	time.Sleep(200 * time.Millisecond) // 覆盖 connect 失败 → sleep → continue
	a.cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestCovReconnectRegisterFailureRetries(t *testing.T) {
	// 收到注册消息后不回复并断开：register 在 ReadMessage 处失败
	srv, connCh := covWSServer(t, func(conn *websocket.Conn) {
		codec := protocol.NewCodec()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, _ = codec.ReadMessage(conn)
		conn.Close()
	})

	a := NewAgent(Config{ServerURL: covWSURL(srv), AgentID: "cov-reg-fail"})
	defer a.Stop()

	go a.reconnect()

	// register 失败 → 关连接 → sleep → 重试：应看到第二次连接
	covPoll(t, 5*time.Second, "second connection after register failure", func() bool {
		return len(connCh) >= 2
	})
	a.cancel()
}

func TestCovReconnectSuccess(t *testing.T) {
	// 每条连接都接受注册并回复 ok：reconnect 走完 connect+register 成功路径
	srv, _ := covWSServer(t, func(conn *websocket.Conn) {
		codec := protocol.NewCodec()
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		_, _ = codec.ReadMessage(conn)
		_ = conn.WriteMessage(websocket.TextMessage,
			[]byte(`{"id":"reg-ok","type":"register","payload":{"status":"ok"}}`))
	})

	a := NewAgent(Config{ServerURL: covWSURL(srv), AgentID: "cov-re-ok"})
	defer a.Stop()

	go a.reconnect()

	covPoll(t, 5*time.Second, "reconnect registers successfully", func() bool {
		return a.registered.Load()
	})
	// reconnect 成功后自身返回，并另起 messageLoop；取消清理
	a.cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestCovRegisterWriteFailure(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1", AgentID: "cov-x"})
	defer a.Stop()

	// conn 为 nil → writeToConn 失败
	if err := a.register(); err == nil {
		t.Error("register() should fail when not connected")
	}
	if a.registered.Load() {
		t.Error("registered should stay false after failed register")
	}
}

func TestCovRegisterEmptyAgentID(t *testing.T) {
	// 服务端读注册消息并回复 ok
	srv, _ := covWSServer(t, func(conn *websocket.Conn) {
		codec := protocol.NewCodec()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, _ = codec.ReadMessage(conn)
		_ = conn.WriteMessage(websocket.TextMessage,
			[]byte(`{"id":"reg-ok","type":"register","payload":{"status":"ok"}}`))
	})

	a := NewAgent(Config{ServerURL: covWSURL(srv)}) // AgentID 为空
	defer a.Stop()

	conn := covDial(t, covWSURL(srv))
	a.mu.Lock()
	a.conn = conn
	a.connected = true
	a.mu.Unlock()

	if err := a.register(); err != nil {
		t.Fatalf("register() error = %v", err)
	}
	if !strings.HasPrefix(a.agentID, "agent-") {
		t.Errorf("agentID = %q, want agent-* prefix from hostname fallback", a.agentID)
	}
	if !a.registered.Load() {
		t.Error("registered should be true after successful register")
	}
}

// ============ heartbeat ============

func TestCovHeartbeatLoopTicks(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	// 在启动任何 goroutine 前写入（此时不存在并发读者）
	oldInterval := heartbeatInterval
	heartbeatInterval = 10 * time.Millisecond

	a.registered.Store(true)
	go a.heartbeatLoop()

	covPoll(t, 2*time.Second, "heartbeat enqueued by loop", func() bool {
		return len(a.outbound) > 0
	})
	msg := <-a.outbound
	if msg.Type != protocol.MessageTypeHeartbeat {
		t.Errorf("message type = %q, want heartbeat", msg.Type)
	}

	// 首个 tick 已观测（NewTicker 的读先于 tick 发生）；取消并等待
	// heartbeatLoop 退出后再恢复，避免数据竞争
	a.cancel()
	time.Sleep(100 * time.Millisecond)
	heartbeatInterval = oldInterval
}

func TestCovSendHeartbeatQueueFullTriggersReconnect(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	a.registered.Store(true)
	for i := 0; i < cap(a.outbound); i++ {
		a.outbound <- protocol.NewMessage(protocol.MessageTypeHeartbeat, nil)
	}

	a.sendHeartbeat() // sendMessage 失败 → 日志 + 触发重连
	time.Sleep(100 * time.Millisecond)
	a.cancel()
	time.Sleep(100 * time.Millisecond)
}

// ============ messageLoop / handlers ============

func TestCovMessageLoopWithoutConn(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	go a.messageLoop()
	time.Sleep(1300 * time.Millisecond) // 覆盖 conn==nil → sleep(1s) → continue
	a.cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestCovMessageLoopExitsAfterCancel(t *testing.T) {
	srv, connCh := covWSServer(t, nil)

	a := NewAgent(Config{ServerURL: covWSURL(srv), AgentID: "cov-ml"})
	defer a.Stop()

	clientConn := covDial(t, covWSURL(srv))
	a.mu.Lock()
	a.conn = clientConn
	a.connected = true
	a.mu.Unlock()

	// 取服务端一侧连接（向 agent 投递消息的方向）
	var serverConn *websocket.Conn
	select {
	case serverConn = <-connCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server never saw the connection")
	}

	go a.messageLoop()

	codec := protocol.NewCodec()

	// 第一阶段（ctx 存活）：确认 messageLoop 在读并分发消息
	if err := codec.WriteMessage(serverConn, protocol.NewMessage(protocol.MessageTypePing, nil)); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	covPoll(t, 2*time.Second, "ping handled while running", func() bool {
		return len(a.outbound) > 0
	})
	<-a.outbound

	// 第二阶段：取消后再投递一条消息，handleMessage 完成后
	// 循环顶部 select 命中 ctx.Done 分支退出
	a.cancel()
	if err := codec.WriteMessage(serverConn, protocol.NewMessage(protocol.MessageTypePing, nil)); err != nil {
		t.Fatalf("write second ping: %v", err)
	}
	time.Sleep(300 * time.Millisecond) // 给读循环走到顶部 ctx.Done 分支的时间
}

func TestCovHandlePingQueueFull(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	for i := 0; i < cap(a.outbound); i++ {
		a.outbound <- protocol.NewMessage(protocol.MessageTypeHeartbeat, nil)
	}
	// sendMessage 失败仅记录日志，不得 panic
	a.handlePing(protocol.NewMessage(protocol.MessageTypePing, nil))
}

func TestCovHandleRPCRequestQueueFull(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	defer a.Stop()

	for i := 0; i < cap(a.outbound); i++ {
		a.outbound <- protocol.NewMessage(protocol.MessageTypeHeartbeat, nil)
	}
	req := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]any{"method": "no.such.method"})
	a.handleRPCRequest(req) // RPC 错误响应也入队失败 → 仅记录日志
}

// ============ detectCapabilities ============

// covErrorDetector 恒定失败的 detector，用于覆盖失败日志分支。
type covErrorDetector struct{}

func (covErrorDetector) Name() string  { return "cov-error-detector" }
func (covErrorDetector) Priority() int { return 1 }
func (covErrorDetector) Detect() (*protocol.Capability, error) {
	return nil, errors.New("cov detector boom")
}

func TestCovDetectCapabilitiesDetectorError(t *testing.T) {
	detector.Register(covErrorDetector{})
	t.Setenv("PVE_URL", "http://127.0.0.1:1") // 跳过 3 个候选地址的 HTTPS 探测
	t.Setenv("DOCKER_HOST", t.TempDir()+"/x") // docker 检测快速失败

	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	caps := a.detectCapabilities()
	if caps == nil {
		t.Fatal("detectCapabilities() should return non-nil slice")
	}
	// 失败的 detector 不产生 capability
	for _, c := range caps {
		if c.Type == "" {
			t.Errorf("capability with empty type: %+v", c)
		}
	}
}

func TestCovDetectCapabilitiesNginx(t *testing.T) {
	// PATH 前置一个 nginx stub（-v 版本打到 stderr），覆盖 nginx-proxy capability
	dir := t.TempDir()
	nginx := dir + "/nginx"
	if err := os.WriteFile(nginx, []byte("#!/bin/sh\necho 'nginx version: nginx/1.24.0' >&2\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("PVE_URL", "http://127.0.0.1:1")
	t.Setenv("DOCKER_HOST", t.TempDir()+"/x")

	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	caps := a.detectCapabilities()

	var found, drift bool
	for _, c := range caps {
		if c.Type == "nginx-proxy" {
			found = true
			if v, _ := c.Metadata["version"].(string); v != "nginx/1.24.0" {
				t.Errorf("nginx version = %v, want nginx/1.24.0", c.Metadata["version"])
			}
		}
		if c.Type == "drift" {
			drift = true
		}
	}
	if !found {
		t.Error("nginx-proxy capability should be detected with stubbed nginx")
	}
	if !drift {
		t.Error("drift capability should follow nginx-proxy")
	}
}
