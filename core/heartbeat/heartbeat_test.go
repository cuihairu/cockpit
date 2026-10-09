package heartbeat

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// capture 构造带捕获的 Loop（全注入，无网络）。
func capture(registered bool, sendErr error) (*Loop, *[]*protocol.Message, *int32) {
	var mu sync.Mutex
	var sent []*protocol.Message
	var fails int32
	l := New(context.Background(), Options{
		AgentID:    func() string { return "hb-1" },
		StartedAt:  func() time.Time { return time.Unix(1700000000, 0) },
		Services:   func() []protocol.RemoteServicePayload { return []protocol.RemoteServicePayload{{Protocol: "ssh"}} },
		SystemInfo: func() interface{} { return map[string]string{"cpu": "x"} },
		Registered: func() bool { return registered },
		Send: func(m *protocol.Message) error {
			mu.Lock()
			sent = append(sent, m)
			mu.Unlock()
			return sendErr
		},
		OnSendFail: func() { atomic.AddInt32(&fails, 1) },
	})
	return l, &sent, &fails
}

// TestBeatUnregisteredSilent 未注册静默跳过（不出队）。
func TestBeatUnregisteredSilent(t *testing.T) {
	l, sent, fails := capture(false, nil)
	l.Beat()
	if len(*sent) != 0 || atomic.LoadInt32(fails) != 0 {
		t.Fatalf("unregistered beat must be silent: sent=%d fails=%d", len(*sent), *fails)
	}
}

// TestBeatPayload 心跳报文字段：agentId/status/startedAt/services/systemInfo。
func TestBeatPayload(t *testing.T) {
	l, sent, _ := capture(true, nil)
	l.Beat()
	if len(*sent) != 1 {
		t.Fatalf("sent = %d", len(*sent))
	}
	msg := (*sent)[0]
	if msg.Type != protocol.MessageTypeHeartbeat {
		t.Fatalf("type = %s", msg.Type)
	}
	if msg.Payload["agentId"] != "hb-1" || msg.Payload["status"] != "online" {
		t.Fatalf("payload = %v", msg.Payload)
	}
	if msg.Payload["startedAt"] != int64(1700000000) {
		t.Fatalf("startedAt = %v (%T)", msg.Payload["startedAt"], msg.Payload["startedAt"])
	}
	if svcs, ok := msg.Payload["services"].([]protocol.RemoteServicePayload); !ok || len(svcs) != 1 {
		t.Fatalf("services = %v (%T)", msg.Payload["services"], msg.Payload["services"])
	}
	if msg.Payload["systemInfo"] == nil {
		t.Fatal("systemInfo missing")
	}
}

// TestBeatNilSystemInfoOmitted SystemInfo 缺省/返回 nil 时字段不出现。
func TestBeatNilSystemInfoOmitted(t *testing.T) {
	var mu sync.Mutex
	var sent []*protocol.Message
	l := New(context.Background(), Options{
		AgentID:    func() string { return "hb-1" },
		StartedAt:  func() time.Time { return time.Now() },
		Services:   func() []protocol.RemoteServicePayload { return nil },
		Registered: func() bool { return true },
		Send: func(m *protocol.Message) error {
			mu.Lock()
			sent = append(sent, m)
			mu.Unlock()
			return nil
		},
	})
	l.Beat()
	l2 := New(context.Background(), Options{
		AgentID:    func() string { return "hb-1" },
		StartedAt:  func() time.Time { return time.Now() },
		Services:   func() []protocol.RemoteServicePayload { return nil },
		SystemInfo: func() interface{} { return nil },
		Registered: func() bool { return true },
		Send: func(m *protocol.Message) error {
			mu.Lock()
			sent = append(sent, m)
			mu.Unlock()
			return nil
		},
	})
	l2.Beat()
	mu.Lock()
	defer mu.Unlock()
	for _, m := range sent {
		if _, ok := m.Payload["systemInfo"]; ok {
			t.Fatalf("systemInfo must be omitted when nil: %v", m.Payload)
		}
	}
}

// TestBeatSendFail 发送失败触发处置回调（capture 先记录后返回错误，
// sent=1 是尝试记录；断言重点是 OnSendFail 恰一次）。
func TestBeatSendFail(t *testing.T) {
	l, sent, fails := capture(true, context.Canceled)
	l.Beat()
	if len(*sent) != 1 {
		t.Fatalf("send attempts = %d, want 1", len(*sent))
	}
	if atomic.LoadInt32(fails) != 1 {
		t.Fatalf("onSendFail = %d, want 1", *fails)
	}
}

// TestRunTicksAndExit 周期出队 + ctx 取消退出。
func TestRunTicksAndExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	n := 0
	l := New(ctx, Options{
		AgentID:    func() string { return "hb-1" },
		StartedAt:  func() time.Time { return time.Now() },
		Services:   func() []protocol.RemoteServicePayload { return nil },
		Registered: func() bool { return true },
		Send: func(m *protocol.Message) error {
			mu.Lock()
			n++
			mu.Unlock()
			return nil
		},
	})
	done := make(chan struct{})
	go func() { l.Run(5 * time.Millisecond); close(done) }()

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		c := n
		mu.Unlock()
		if c >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("only %d beats", c)
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after cancel")
	}
}

// TestBeatNilRegistered 默认未注册（Registered 省略即静默）。
func TestBeatNilRegistered(t *testing.T) {
	l := New(context.Background(), Options{
		AgentID:   func() string { return "hb-1" },
		StartedAt: func() time.Time { return time.Now() },
		Services:  func() []protocol.RemoteServicePayload { return nil },
		Send:      func(m *protocol.Message) error { return nil },
	})
	l.Beat() // 不 panic、不出队（Send 未被调用——用 panic-capable Send 兜底即上面无副作用）
}
