package report

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// wsPair 起一对内存内 websocket 连接（服务端收到的消息进 chan）。
func wsPair(t *testing.T) (client *websocket.Conn, received chan *protocol.Message) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	rcv := make(chan *protocol.Message, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var msg protocol.Message
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			rcv <- &msg
		}
	}))
	t.Cleanup(srv.Close)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c, rcv
}

func newUpstream(ctx context.Context, conn *websocket.Conn, mu *sync.Mutex, registered func() bool, onWriteErr func()) *Upstream {
	queue := make(chan *protocol.Message, 4)
	codec := protocol.NewCodec()
	var c *websocket.Conn
	if conn != nil {
		c = conn
	}
	return New(ctx, queue, codec,
		func() *websocket.Conn { return c },
		mu, registered, onWriteErr)
}

// TestEnqueueAccepted 入队成功。
func TestEnqueueAccepted(t *testing.T) {
	u := newUpstream(context.Background(), nil, &sync.Mutex{}, func() bool { return true }, nil)
	if err := u.Enqueue(protocol.NewMessage(protocol.MessageTypePing, nil)); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
}

// TestEnqueueQueueFull 队满即报错不阻塞。
func TestEnqueueQueueFull(t *testing.T) {
	u := newUpstream(context.Background(), nil, &sync.Mutex{}, func() bool { return true }, nil)
	for i := 0; i < 4; i++ {
		if err := u.Enqueue(protocol.NewMessage(protocol.MessageTypePing, nil)); err != nil {
			t.Fatalf("Enqueue #%d: %v", i, err)
		}
	}
	if err := u.Enqueue(protocol.NewMessage(protocol.MessageTypePing, nil)); err == nil ||
		err.Error() != "agent outbound queue full" {
		t.Fatalf("full queue err = %v, want queue full", err)
	}
}

// TestEnqueueStopped 队满 + ctx 取消：入队报停机（send 阻塞后 ctx.Done
// 唯一就绪——空队 + 已取消 ctx 时 send 与 Done 同就绪，select 伪随机，
// 原sendMessage 同语义，不做非确定断言）。
func TestEnqueueStopped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := newUpstream(ctx, nil, &sync.Mutex{}, func() bool { return true }, nil)
	for i := 0; i < 4; i++ {
		_ = u.Enqueue(protocol.NewMessage(protocol.MessageTypePing, nil))
	}
	cancel()
	if err := u.Enqueue(protocol.NewMessage(protocol.MessageTypePing, nil)); err == nil ||
		err.Error() != "agent stopped" {
		t.Fatalf("stopped err = %v, want agent stopped", err)
	}
}

// TestWriteNowNilConn 无连接即报错。
func TestWriteNowNilConn(t *testing.T) {
	u := newUpstream(context.Background(), nil, &sync.Mutex{}, func() bool { return true }, nil)
	if err := u.WriteNow(protocol.NewMessage(protocol.MessageTypePing, nil)); err == nil ||
		err.Error() != "agent not connected" {
		t.Fatalf("err = %v, want agent not connected", err)
	}
}

// TestWriteNowSuccess 直写落线，服务端可解。
func TestWriteNowSuccess(t *testing.T) {
	conn, rcv := wsPair(t)
	u := newUpstream(context.Background(), conn, &sync.Mutex{}, func() bool { return true }, nil)
	msg := protocol.NewMessage(protocol.MessageTypePing, map[string]interface{}{"k": "v"})
	if err := u.WriteNow(msg); err != nil {
		t.Fatalf("WriteNow: %v", err)
	}
	select {
	case got := <-rcv:
		if got.Type != protocol.MessageTypePing {
			t.Fatalf("type = %s", got.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not receive message")
	}
}

// TestRunSerialDrain 注册放行后按序消费队列。
func TestRunSerialDrain(t *testing.T) {
	conn, rcv := wsPair(t)
	var reg atomic.Bool
	u := newUpstream(context.Background(), conn, &sync.Mutex{}, func() bool { return reg.Load() }, nil)
	go u.Run()
	for i := 0; i < 3; i++ {
		if err := u.Enqueue(protocol.NewMessage(protocol.MessageTypePing,
			map[string]interface{}{"i": i})); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	// 未注册：一条都不出
	time.Sleep(150 * time.Millisecond)
	if len(rcv) != 0 {
		t.Fatalf("unregistered drain leaked %d messages", len(rcv))
	}
	reg.Store(true)
	n := 0
	deadline := time.After(3 * time.Second)
	for n < 3 {
		select {
		case <-rcv:
			n++
		case <-deadline:
			t.Fatalf("only %d/3 messages drained", n)
		}
	}
}

// TestRunWriteFailureContinues 写失败回调处置后循环继续（原 writeLoop
// 语义：失败不 return，下一条消息卡放行门等重注册）。
func TestRunWriteFailureContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mu := &sync.Mutex{}
	var c *websocket.Conn // 恒 nil：每条消息写必失败
	var errs atomic.Int32
	u := New(ctx, make(chan *protocol.Message, 4), protocol.NewCodec(),
		func() *websocket.Conn { return c }, mu,
		func() bool { return true },
		func() { errs.Add(1) })
	go u.Run()
	for i := 0; i < 2; i++ {
		if err := u.Enqueue(protocol.NewMessage(protocol.MessageTypePing, nil)); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	deadline := time.After(2 * time.Second)
	for errs.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("onWriteErr fired %d/2", errs.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}
	// 循环仍存活：ctx 取消能正常退出
	cancel()
}

// TestRunExitOnCancelUnregistered 未注册等待中 ctx 取消整体退出。
func TestRunExitOnCancelUnregistered(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := newUpstream(ctx, nil, &sync.Mutex{}, func() bool { return false }, nil)
	u.Enqueue(protocol.NewMessage(protocol.MessageTypePing, nil))
	done := make(chan struct{})
	go func() { u.Run(); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after cancel while unregistered")
	}
}

// TestWaitRegisteredImmediate 已注册即刻放行。
func TestWaitRegisteredImmediate(t *testing.T) {
	if err := WaitRegistered(context.Background(), func() bool { return true }); err != nil {
		t.Fatalf("WaitRegistered: %v", err)
	}
}

// TestWaitRegisteredAfterPoll 轮询若干 tick 后放行。
func TestWaitRegisteredAfterPoll(t *testing.T) {
	var reg atomic.Bool
	errCh := make(chan error, 1)
	go func() { errCh <- WaitRegistered(context.Background(), func() bool { return reg.Load() }) }()
	// 至少跨一个 pollInterval tick 再放行
	time.Sleep(250 * time.Millisecond)
	reg.Store(true)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("WaitRegistered = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not return after registration")
	}
}

// TestWaitRegisteredCancelled ctx 取消报 agent stopped。
func TestWaitRegisteredCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- WaitRegistered(ctx, func() bool { return false }) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if err == nil || err.Error() != "agent stopped" {
			t.Fatalf("WaitRegistered = %v, want agent stopped", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not return after cancel")
	}
}
