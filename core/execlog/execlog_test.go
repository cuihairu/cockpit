package execlog

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// TestRecordFlushRoundTrip 记录→攒批→上行全链：捕获桩拿到 exec_log 消息，
// DecodeExecLog 还原字段一致；批后缓冲清空，二次 Flush 空批 no-op。
func TestRecordFlushRoundTrip(t *testing.T) {
	var sent []*protocol.Message
	r := New(DefaultCapacity, func(m *protocol.Message) error {
		sent = append(sent, m)
		return nil
	})

	r.Record(protocol.ExecLogEntry{Provider: "service", Action: "restart", Status: protocol.ExecStatusSuccess})
	r.RecordCall("file", "delete", 12*time.Millisecond, errors.New("permission denied"),
		map[string]interface{}{"path": "/tmp/x", "password": "s3cret"})

	if err := r.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(sent))
	}
	p, err := protocol.DecodeExecLog(sent[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(p.Entries))
	}
	e1, e2 := p.Entries[0], p.Entries[1]
	if e1.ID == "" || e1.Time == 0 {
		t.Fatalf("entry defaults not filled: %+v", e1)
	}
	if e2.Status != protocol.ExecStatusFailure || e2.Error != "permission denied" || e2.DurationMs != 12 {
		t.Fatalf("call entry = %+v", e2)
	}
	if _, ok := e2.Params["password"]; ok {
		t.Fatalf("sensitive key leaked: %+v", e2.Params)
	}
	if e2.Params["path"] != "/tmp/x" {
		t.Fatalf("params = %+v", e2.Params)
	}

	// 缓冲已清空：二次 Flush 空批 no-op
	if err := r.Flush(); err != nil {
		t.Fatalf("second flush: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("empty flush sent = %d, want 1", len(sent))
	}
}

// TestNewDefaultCapacity capacity<=0 回落默认容量。
func TestNewDefaultCapacity(t *testing.T) {
	r := New(0, func(*protocol.Message) error { return nil })
	r.Record(protocol.ExecLogEntry{Provider: "job", Action: "run", Status: protocol.ExecStatusSuccess})
	if err := r.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
}

// TestRunFlushErrorLogs 两条失败日志分支确定性覆盖：周期 Flush 失败（tick
// 路径，等 send 被调即命中）与停机末次 Flush 失败（ctx.Done 路径，间隔取
// 小时级保证取消是唯一触发点，末批必带条目）。
func TestRunFlushErrorLogs(t *testing.T) {
	// tick 路径：10ms 间隔，首个 tick 必触发一次失败 Flush
	var ticked int32
	r1 := New(8, func(*protocol.Message) error {
		atomic.AddInt32(&ticked, 1)
		return errors.New("down")
	})
	ctx1, cancel1 := context.WithCancel(context.Background())
	done1 := make(chan struct{})
	go func() {
		r1.Run(ctx1, 10*time.Millisecond)
		close(done1)
	}()
	r1.Record(protocol.ExecLogEntry{Provider: "job", Action: "run", Status: protocol.ExecStatusSuccess})
	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt32(&ticked) == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if atomic.LoadInt32(&ticked) == 0 {
		t.Fatal("periodic flush never fired")
	}
	cancel1()
	<-done1

	// ctx.Done 路径：间隔小时级——取消是唯一触发点
	r2 := New(8, func(*protocol.Message) error { return errors.New("down") })
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	go func() {
		r2.Run(ctx2, time.Hour)
		close(done2)
	}()
	r2.Record(protocol.ExecLogEntry{Provider: "job", Action: "run", Status: protocol.ExecStatusSuccess})
	cancel2()
	<-done2
}

// TestCapacityEvictsOldest 满丢最旧：留痕观测面不反压业务。
func TestCapacityEvictsOldest(t *testing.T) {
	var sent []*protocol.Message
	r := New(2, func(m *protocol.Message) error { sent = append(sent, m); return nil })
	for _, id := range []string{"a", "b", "c"} {
		r.Record(protocol.ExecLogEntry{ID: id, Provider: "job", Action: "run", Status: protocol.ExecStatusSuccess})
	}
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	p, _ := protocol.DecodeExecLog(sent[0])
	if len(p.Entries) != 2 || p.Entries[0].ID != "b" || p.Entries[1].ID != "c" {
		t.Fatalf("entries = %+v, want [b c]", p.Entries)
	}
}

// TestFlushSendErrorDropsBatch 发送失败该批即弃（尽力而为契约）：失败批
// 不重试、不回灌缓冲，下次 Flush 从空批开始。
func TestFlushSendErrorDropsBatch(t *testing.T) {
	fail := true
	var calls int
	r := New(4, func(m *protocol.Message) error {
		calls++
		if fail {
			return errors.New("queue full")
		}
		return nil
	})
	r.Record(protocol.ExecLogEntry{Provider: "job", Action: "run", Status: protocol.ExecStatusSuccess})
	if err := r.Flush(); err == nil {
		t.Fatal("flush: want error")
	}
	fail = false
	// 批已弃：二次 Flush 空批 no-op，不再触达 send（无重试）
	if err := r.Flush(); err != nil {
		t.Fatalf("flush after failure: %v", err)
	}
	if calls != 1 {
		t.Fatalf("send calls = %d, want 1 (failed batch dropped, not retried)", calls)
	}
}

// TestSanitize 敏感键剔除/标量截断/复合值只记形状。
func TestSanitize(t *testing.T) {
	got := Sanitize(map[string]interface{}{
		"path":       strings.Repeat("x", 200),
		"count":      float64(3),
		"force":      true,
		"empty":      nil,
		"password":   "leak",
		"privateKey": "leak",
		"nested":     map[string]interface{}{"secret": "deep"},
	})
	if got["path"] != strings.Repeat("x", 128)+"…" {
		t.Fatalf("path clip = %q", got["path"])
	}
	if got["count"] != "3" || got["force"] != "true" || got["empty"] != "" {
		t.Fatalf("scalars = %+v", got)
	}
	if got["nested"] != "…" {
		t.Fatalf("nested = %q, want shape marker", got["nested"])
	}
	for _, k := range []string{"password", "privateKey"} {
		if _, ok := got[k]; ok {
			t.Fatalf("sensitive key %q not dropped", k)
		}
	}
	if Sanitize(nil) != nil {
		t.Fatal("sanitize nil: want nil")
	}
}

// TestRunFlushesAndFinalFlush 周期 Flush 落批 + ctx 取消触发末次 Flush。
// 长间隔（500ms）保证取消发生在下一个 tick 之前——第二批只能来自末次
// Flush，断言不靠运气。捕获桩互斥保护：Run goroutine 追加与测试轮询计数
// 并发（-race 下未同步读写即报 DATA RACE，CI 实锤）。
func TestRunFlushesAndFinalFlush(t *testing.T) {
	var mu sync.Mutex
	var sent []*protocol.Message
	r := New(8, func(m *protocol.Message) error {
		mu.Lock()
		sent = append(sent, m)
		mu.Unlock()
		return nil
	})
	sentCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(sent)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx, 500*time.Millisecond)
		close(done)
	}()

	r.Record(protocol.ExecLogEntry{ID: "e1", Provider: "job", Action: "run", Status: protocol.ExecStatusSuccess})
	deadline := time.Now().Add(3 * time.Second)
	for sentCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sentCount() == 0 {
		t.Fatal("periodic flush never fired")
	}

	// 第二条只能走 ctx 取消后的末次 Flush（下一 tick 在 500ms 外）；
	// <-done 后 Run goroutine 已退出，余下读取与写入有 happens-before
	r.Record(protocol.ExecLogEntry{ID: "e2", Provider: "job", Action: "run", Status: protocol.ExecStatusSuccess})
	cancel()
	<-done
	if sentCount() != 2 {
		t.Fatalf("sent = %d, want 2 (final flush)", sentCount())
	}
	mu.Lock()
	last := sent[1]
	mu.Unlock()
	p, _ := protocol.DecodeExecLog(last)
	if len(p.Entries) != 1 || p.Entries[0].ID != "e2" {
		t.Fatalf("final batch = %+v, want [e2]", p.Entries)
	}
}
