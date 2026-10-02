package server

// cov_guac_relay_gaps_test.go 补 guac_relay.go 的错误分支：proxy_new 发送
// 失败拆链、data 泵发送失败拆链、向已关闭连接回写失败拆链、未知 relay 的
// close/error 静默返回。

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// TestGuacRelayProxyNewSendFail 覆盖 handleConn 的 proxy_new 发送失败 →
// 拆链（guacd 侧连接被关）
func TestGuacRelayProxyNewSendFail(t *testing.T) {
	relay, addr, err := startGuacRelay("a", "guac:newfail", "t:1",
		func(string, *protocol.Message) error { return errors.New("agent gone") })
	if err != nil {
		t.Fatalf("startGuacRelay: %v", err)
	}
	defer relay.Close()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := conn.Write([]byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(make([]byte, 16)); err == nil {
		t.Fatal("conn should be closed when proxy_new send fails")
	}
}

// TestGuacRelayDataPumpSendFail 覆盖 pumpToAgent 的数据发送失败 → 泵退出
// 并拆链（proxy_new 成功、proxy_data 失败的条件发送器）
func TestGuacRelayDataPumpSendFail(t *testing.T) {
	var mu sync.Mutex
	news := 0
	relay, addr, err := startGuacRelay("a", "guac:datafail", "t:1",
		func(_ string, msg *protocol.Message) error {
			if msg.Type == protocol.MessageTypeProxyData {
				return errors.New("send buffer full")
			}
			mu.Lock()
			news++
			mu.Unlock()
			return nil
		})
	if err != nil {
		t.Fatalf("startGuacRelay: %v", err)
	}
	defer relay.Close()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(make([]byte, 16)); err == nil {
		t.Fatal("conn should be closed when data pump send fails")
	}
}

// TestGuacRelayDeliverWriteFail 覆盖 guacRelayDeliver 对已关闭连接的写失败
// → removeConn 拆链并通知 agent（proxy_close 出站）
func TestGuacRelayDeliverWriteFail(t *testing.T) {
	cap := &relayCapture{}
	relay, _, err := startGuacRelay("a", "guac:writefail", "t:1", cap.send)
	if err != nil {
		t.Fatalf("startGuacRelay: %v", err)
	}
	defer relay.Close()

	// 直接挂一条已关闭的管道连接：Write 立即 ErrClosedPipe
	c1, c2 := net.Pipe()
	c1.Close()
	c2.Close()
	relay.mu.Lock()
	relay.conns["c0"] = c2
	relay.mu.Unlock()

	if err := guacRelayDeliver("guac:writefail", "c0", []byte("x")); err == nil {
		t.Fatal("deliver to closed conn should fail")
	}
	waitForCond(t, func() bool { return cap.closesCount() > 0 })
	if got := cap.closes[0].ConnID; got != "c0" {
		t.Fatalf("proxy_close connId = %q, want c0", got)
	}
}

// TestGuacRelayHandleCloseAndErrorUnknownRelay 覆盖未知 relay 的静默返回
func TestGuacRelayHandleCloseAndErrorUnknownRelay(t *testing.T) {
	guacRelayHandleClose("guac:no-such", "c1", "r")
	guacRelayHandleError("guac:no-such", "c1", "boom")
	guacRelayHandleError("guac:no-such", "", "no connId")
}
