package agent

// cov_pct_test.go 覆盖率攻坚（agent.go L351/L359/L438 与 providers.go
// L82/L89/L174 的原未覆盖语句）：
//   - GOOS 编译期分支：经 agent.go 的包级 goos 快照注入平台值触发；
//   - register 的 conn==nil 快照分支：占住 writeMu 制造确定性窗口；
//   - logs provider 的 sender/closer 闭包体：PATH 注入假 journalctl，
//     经 rpc.Handle 驱动 follow.start/follow.stop，回推经 proxyHandler
//     汇入 outbound 供轮询断言。
// 复用 cov_agent_test.go / cov_providers_test.go 的 helper（covWSServer、
// covPoll、covRegistered 等）；无 flaky 睡眠，等待一律轮询 + deadline。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/agent/rpc"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// withGoos 临时替换包级平台快照 goos（默认 runtime.GOOS，见 agent.go
// 注释），用 defer 恢复。同包测试串行执行且读方均为本 goroutine 同步
// 调用的函数，无并发读取窗口。
func withGoos(t *testing.T, v string) {
	t.Helper()
	old := goos
	goos = v
	t.Cleanup(func() { goos = old })
}

// ============ GOOS 分支（agent.go windows/darwin + providers.go stack）============

// TestCovDetectCapabilitiesServiceWindows 覆盖 detectCapabilities 的
// windows 分支（service capability，backend=windows-scm）。手段：goos
// 注入 "windows"；PVE_URL/DOCKER_HOST 与既有 cov 测试同则，避免慢探测。
func TestCovDetectCapabilitiesServiceWindows(t *testing.T) {
	withGoos(t, "windows")
	t.Setenv("PVE_URL", "http://127.0.0.1:1")
	t.Setenv("DOCKER_HOST", t.TempDir()+"/x")

	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	caps := a.detectCapabilities()

	var found bool
	for _, c := range caps {
		if c.Type == "service" {
			found = true
			if backend, _ := c.Metadata["backend"].(string); backend != "windows-scm" {
				t.Errorf("service backend = %v, want windows-scm", c.Metadata["backend"])
			}
		}
	}
	if !found {
		t.Errorf("service capability missing under goos=windows: %+v", caps)
	}
}

// TestCovDetectCapabilitiesServiceDarwin 覆盖 detectCapabilities 的
// darwin 分支（service capability，backend=launchd）。
func TestCovDetectCapabilitiesServiceDarwin(t *testing.T) {
	withGoos(t, "darwin")
	t.Setenv("PVE_URL", "http://127.0.0.1:1")
	t.Setenv("DOCKER_HOST", t.TempDir()+"/x")

	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	caps := a.detectCapabilities()

	var found bool
	for _, c := range caps {
		if c.Type == "service" {
			found = true
			if backend, _ := c.Metadata["backend"].(string); backend != "launchd" {
				t.Errorf("service backend = %v, want launchd", c.Metadata["backend"])
			}
		}
	}
	if !found {
		t.Errorf("service capability missing under goos=darwin: %+v", caps)
	}
}

// TestCovRegisterStackProviderUnsupportedOS 覆盖 registerStackProvider
// 的非 linux/darwin 跳过分支。手段：goos 注入 "windows" 并带明确
// endpoint（先通过 host 检查，在 ComposeAvailable 之前即返回，无网络），
// 断言 stack provider 未注册。
func TestCovRegisterStackProviderUnsupportedOS(t *testing.T) {
	withGoos(t, "windows")

	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1"})
	before := len(a.rpc.RegisteredTypes())
	a.registerStackProvider(protocol.Capability{
		Type:     "docker-api",
		Endpoint: "tcp://127.0.0.1:1",
	}, rpc.NewDriftBaseline(""))

	if got := len(a.rpc.RegisteredTypes()); got != before {
		t.Errorf("stack provider should be skipped on unsupported OS, registered: %v", a.rpc.RegisteredTypes())
	}
	if covRegistered(a, "stack") {
		t.Error("stack provider must not register when goos is not linux/darwin")
	}
}

// ============ register 的 conn==nil 快照分支（agent.go L438）============

// TestCovRegisterConnClearedBeforeSnapshot 覆盖 register 在 writeToConn
// 成功之后、第二次 conn 快照时发现连接已被置 nil 的分支。
//
// 确定性时序：测试先占住 writeMu，register 的 writeToConn 在取锁前完成
// conn 快照（必为非 nil）后阻塞在 writeMu；随后测试置 a.conn = nil 并放
// 锁，WriteMessage 用旧连接成功写出（服务端读到注册消息即证明），register
// 的第二次快照读到 nil → 返回 "agent not connected"。
func TestCovRegisterConnClearedBeforeSnapshot(t *testing.T) {
	// 服务端：读到注册消息即通知（WriteMessage 成功的可观测证据），随后
	// 保持连接打开、不回复注册响应
	regArrived := make(chan struct{}, 1)
	srv, _ := covWSServer(t, func(conn *websocket.Conn) {
		codec := protocol.NewCodec()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := codec.ReadMessage(conn); err == nil {
			regArrived <- struct{}{}
		}
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		_, _ = codec.ReadMessage(conn)
	})

	a := NewAgent(Config{ServerURL: covWSURL(srv), AgentID: "cov-nil", Region: "r", Zone: "z"})
	defer a.Stop()

	DetectVirtualization() // 预热 sync.Once 缓存，避免探测耗时挤占下方时序窗口

	conn := covDial(t, covWSURL(srv))
	a.mu.Lock()
	a.conn = conn
	a.connected = true
	a.mu.Unlock()

	a.writeMu.Lock() // 占住写锁：register 必阻塞在 writeToConn 的 writeMu 上
	errCh := make(chan error, 1)
	go func() { errCh <- a.register() }()

	// 给 register 足够时间走到 writeMu 阻塞点（此时它已完成非 nil conn
	// 快照——快照在取锁之前）；纯等待裕量，正确性由下方 regArrived 断言兜底
	time.Sleep(500 * time.Millisecond)

	a.mu.Lock()
	a.conn = nil
	a.mu.Unlock()
	a.writeMu.Unlock()

	// 注册消息必须已真实写出，否则说明 writeToConn 提前失败、时序前提不成立
	select {
	case <-regArrived:
	case <-time.After(2 * time.Second):
		t.Fatal("register message never reached the server; timing assumption broken")
	}

	select {
	case err := <-errCh:
		if err == nil || err.Error() != "agent not connected" {
			t.Errorf("register() = %v, want %q from nil-conn snapshot", err, "agent not connected")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("register() did not return after conn cleared")
	}
	if a.registered.Load() {
		t.Error("registered must stay false when conn snapshot is nil")
	}
}

// ============ logs provider 的 sender/closer 闭包体（providers.go）============

// TestCovLogsProviderSenderCloserWired 覆盖 setupProviders 注册给
// LogsProvider 的 sender/closer 两个闭包体。手段：PATH 前置假 journalctl
// （输出两行后短暂挂起保持管道），setupProviders 装配 logs capability，
// 经 a.rpc.Handle 驱动 follow.start / follow.stop；闭包回推经
// proxyHandler（sendFunc = a.sendMessage）落入 outbound，轮询可观测。
func TestCovLogsProviderSenderCloserWired(t *testing.T) {
	// 假 journalctl：跟随命令输出两行后挂起（保持 stdout 打开）；
	// follow.stop 由 CommandContext kill，孤儿 sleep 最长 5s 自行退出
	dir := t.TempDir()
	script := "#!/bin/sh\necho log-line-1\necho log-line-2\nsleep 5\n"
	if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	srv, _ := covWSServer(t, nil)

	a := NewAgent(Config{ServerURL: covWSURL(srv)})
	defer a.Stop()

	// 仅带 logs capability 装配，触发 providers.go 的 sender/closer 注册
	a.capabilities = []protocol.Capability{{Type: "logs"}}
	a.setupProviders()
	if !covRegistered(a, "logs") {
		t.Fatalf("logs provider not registered: %v", a.rpc.RegisteredTypes())
	}

	// 回推链路：闭包 → proxyHandler.SendMessage → a.sendMessage → outbound
	a.proxyHandler.SetSendFunc(a.sendMessage)
	a.proxyHandler.Start(covDial(t, covWSURL(srv)))
	defer a.proxyHandler.Stop()

	rpcCall := func(method string, params map[string]any) error {
		_, err := a.rpc.Handle(protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]any{
			"method": method,
			"params": params,
		}))
		return err
	}

	// follow.start：pumpFollow 扫到两行 → sender 闭包执行（ProxyData 入队）
	if err := rpcCall("logs.follow.start", map[string]any{
		"followId": "f1",
		"type":     "systemd",
		"source":   "nginx.service",
	}); err != nil {
		t.Fatalf("follow.start: %v", err)
	}
	covPoll(t, 5*time.Second, "two proxy_data messages from sender closure", func() bool {
		return len(a.outbound) >= 2
	})

	// follow.stop：closeFn 同步执行 → closer 闭包执行（ProxyClose 入队）；
	// Handle 返回即代表 closer 已跑完
	if err := rpcCall("logs.follow.stop", map[string]any{"followId": "f1"}); err != nil {
		t.Fatalf("follow.stop: %v", err)
	}
	covPoll(t, 2*time.Second, "proxy_close message from closer closure", func() bool {
		return len(a.outbound) >= 3
	})

	// 断言出队内容：两行数据 + 一条关闭通知
	var sawData, sawClose int
	for len(a.outbound) > 0 {
		msg := <-a.outbound
		switch msg.Type {
		case protocol.MessageTypeProxyData:
			sawData++
			if id, _ := msg.Payload["proxyId"].(string); id != "logs:f1" {
				t.Errorf("proxy data proxyId = %v, want logs:f1", msg.Payload["proxyId"])
			}
			data, _ := msg.Payload["data"].([]byte)
			if !strings.Contains(string(data), "log-line") {
				t.Errorf("proxy data payload = %q, want log-line-*", data)
			}
		case protocol.MessageTypeProxyClose:
			sawClose++
			if id, _ := msg.Payload["proxyId"].(string); id != "logs:f1" {
				t.Errorf("proxy close proxyId = %v, want logs:f1", msg.Payload["proxyId"])
			}
			if reason, _ := msg.Payload["reason"].(string); reason != "stopped" {
				t.Errorf("proxy close reason = %v, want stopped", msg.Payload["reason"])
			}
		default:
			t.Errorf("unexpected outbound message type: %s", msg.Type)
		}
	}
	if sawData != 2 {
		t.Errorf("proxy_data count = %d, want 2", sawData)
	}
	if sawClose != 1 {
		t.Errorf("proxy_close count = %d, want 1", sawClose)
	}

	// 收尾：给 pumpFollow（进程被杀后 Scan 返回、cmd.Wait）留退出时间，
	// 无断言依赖，仅防测试结束后 goroutine 仍在访问 provider 状态
	time.Sleep(100 * time.Millisecond)
}
