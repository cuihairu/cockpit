package agent

import (
	"runtime"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// TestAgentMessageLoopStaleGenerationExits 验证 G1：连接被替换（连接代数
// 递增）后，陈旧 messageLoop 必须退出——不得继续读新连接（并发双读）、
// 不得误触发重连。回归：旧实现无代数守卫，重连后陈旧循环读失败会
// 误触发 reconnect，且可能对新连接并发读。
func TestAgentMessageLoopStaleGenerationExits(t *testing.T) {
	srv, connCh := covWSServer(t, nil)

	a := NewAgent(Config{ServerURL: covWSURL(srv), AgentID: "g1"})
	defer a.Stop()

	// 代 1：客户端连接 conn1（server 侧 sconn1）
	conn1 := covDial(t, covWSURL(srv))
	var sconn1 *websocket.Conn
	select {
	case sconn1 = <-connCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server never saw conn1")
	}

	a.mu.Lock()
	a.conn = conn1
	a.connected = true
	a.connGen = 1
	a.mu.Unlock()

	loopDone := make(chan struct{})
	go func() {
		a.messageLoop()
		close(loopDone)
	}()
	t.Logf("DBGML-test loop started conn1=%p", conn1) // TMP-DEBUG

	// 确认代 1 循环活着并处理消息。
	codec := protocol.NewCodec()
	if err := codec.WriteMessage(sconn1, protocol.NewMessage(protocol.MessageTypePing, nil)); err != nil {
		t.Fatalf("write ping on conn1: %v", err)
	}
	covPoll(t, 2*time.Second, "generation-1 loop processes message", func() bool {
		return len(a.outbound) > 0
	})
	<-a.outbound
	t.Logf("DBGML-test pong consumed") // TMP-DEBUG

	// 代 2：建立新连接 conn2，并模拟 reconnect 的 connect() 写回：递增
	// 连接代数后把新连接挂到 a.conn。
	conn2 := covDial(t, covWSURL(srv))
	var sconn2 *websocket.Conn
	select {
	case sconn2 = <-connCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server never saw conn2")
	}

	a.mu.Lock()
	a.conn = conn2
	a.connected = true
	a.connGen = 2
	a.mu.Unlock()
	t.Logf("DBGML-test swap2 done conn2=%p", conn2) // TMP-DEBUG

	// 关闭旧连接：代 1 的 ReadMessage 返回错误，但代数已失配 → 应退出。
	conn1.Close()
	t.Logf("DBGML-test conn1 closed") // TMP-DEBUG

	select {
	case <-loopDone:
	case <-time.After(3 * time.Second):
		t.Fatal("stale messageLoop did not exit after connection generation change")
	}
	// sconn1 在 ping 写入后不再被引用，Go 活跃性分析判定其可回收：
	// netFD finalizer 关 fd 会向 conn1 发 FIN，恰在 swap 前触发会让
	// 陈旧循环读到 1006 EOF 并误走重连（CI 实测，见 894b8470 后的
	// Agent Test 红腿）。KeepAlive 钉住到断言完成，杜绝 finalizer 抢跑。
	runtime.KeepAlive(sconn1)

	// 未误触发重连：reconnecting 未置位，且代数未被再次递增。
	if a.reconnecting.Load() {
		t.Error("stale loop must not trigger reconnect after generation change")
	}
	a.mu.RLock()
	gen := a.connGen
	a.mu.RUnlock()
	if gen != 2 {
		t.Errorf("connGen = %d, want 2 (no extra reconnect)", gen)
	}

	// 无代 2 循环在跑：若陈旧循环误读 conn2（并发双读），它会把消息
	// 送入 outbound。此断言证明陈旧循环未继续在新连接上读。
	if err := codec.WriteMessage(sconn2, protocol.NewMessage(protocol.MessageTypePing, nil)); err != nil {
		t.Fatalf("write ping on conn2: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if len(a.outbound) > 0 {
		t.Error("stale loop read on the new connection (double-read risk)")
	}
}
