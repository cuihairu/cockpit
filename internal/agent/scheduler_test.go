package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// TestClassifyMethodTable 覆盖车道分类：控制面（system/job.cancel/probe/
// 单段回退）→ high，其余 → normal，空方法 fail-normal。
func TestClassifyMethodTable(t *testing.T) {
	cases := []struct {
		method string
		want   rpcLane
	}{
		{"system", laneHigh},
		{"system.status", laneHigh},
		{"job.cancel", laneHigh},
		{"probe.ping", laneHigh},
		{"status", laneHigh}, // 单段回退 system provider
		{"docker.list", laneNormal},
		{"pve.list_vms", laneNormal},
		{"backup.run", laneNormal},
		{"", laneNormal}, // 空方法不误升级控制面
	}
	for _, c := range cases {
		if got := classifyMethod(c.method); got != c.want {
			t.Errorf("classifyMethod(%q) = %v, want %v", c.method, got, c.want)
		}
	}
}

// TestSchedulerConcurrencyLimitAndRelease 覆盖并发上限与释放复用：
// 填满 normal 车道后第三条超时；释放后可再取。
func TestSchedulerConcurrencyLimitAndRelease(t *testing.T) {
	s := NewScheduler(2, 1, 50*time.Millisecond, 50*time.Millisecond)

	ctx := context.Background()
	rel1, err := s.Acquire(ctx, "docker.list")
	if err != nil {
		t.Fatalf("acquire 1: %v", err)
	}
	rel2, err := s.Acquire(ctx, "pve.list")
	if err != nil {
		t.Fatalf("acquire 2: %v", err)
	}

	// 第三条超时（无空槽）
	if _, err := s.Acquire(ctx, "backup.run"); err == nil {
		t.Fatal("third acquire should time out")
	}

	// 释放后复用
	rel1()
	rel3, err := s.Acquire(ctx, "docker.inspect")
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	rel3()
	rel2()
}

// TestSchedulerLaneIsolation 覆盖双车道隔离：normal 占满不影响 high；
// high 亦有独立上限。
func TestSchedulerLaneIsolation(t *testing.T) {
	s := NewScheduler(1, 1, 50*time.Millisecond, 50*time.Millisecond)
	ctx := context.Background()

	relN, err := s.Acquire(ctx, "docker.list")
	if err != nil {
		t.Fatalf("normal acquire: %v", err)
	}
	// normal 满：high 仍可取（独立槽位）
	relH, err := s.Acquire(ctx, "system.status")
	if err != nil {
		t.Fatalf("high acquire should succeed while normal full: %v", err)
	}
	// high 也满：第二条 high 超时
	if _, err := s.Acquire(ctx, "job.cancel"); err == nil {
		t.Fatal("second high acquire should time out")
	}
	// normal 再取应超时
	if _, err := s.Acquire(ctx, "docker.list"); err == nil {
		t.Fatal("normal full again should time out")
	}
	relH()
	relN()
}

// TestSchedulerAcquireCtxCancel 覆盖 ctx 取消：占满后 ctx 取消即返回。
func TestSchedulerAcquireCtxCancel(t *testing.T) {
	s := NewScheduler(1, 1, time.Second, time.Second)
	rel, err := s.Acquire(context.Background(), "docker.list")
	if err != nil {
		t.Fatalf("fill: %v", err)
	}
	defer rel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Acquire(ctx, "docker.list"); err == nil {
		t.Fatal("cancelled ctx acquire should fail")
	}
}

// TestSchedulerClamp 覆盖配置钳制：<=0 用默认；>64 钳到上限。
func TestSchedulerClamp(t *testing.T) {
	s := NewScheduler(0, 0, 0, 0)
	if got := s.Lane(laneNormal); got != DefaultRPCMaxConcurrent {
		t.Errorf("default normal lane = %d, want %d", got, DefaultRPCMaxConcurrent)
	}
	if got := s.Lane(laneHigh); got != DefaultRPCMaxHighConcurrent {
		t.Errorf("default high lane = %d, want %d", got, DefaultRPCMaxHighConcurrent)
	}

	s2 := NewScheduler(100, 100, 0, 0)
	if got := s2.Lane(laneNormal); got != rpcMaxConcurrentCap {
		t.Errorf("clamped normal lane = %d, want %d", got, rpcMaxConcurrentCap)
	}
	if got := s2.Lane(laneHigh); got != rpcMaxConcurrentCap {
		t.Errorf("clamped high lane = %d, want %d", got, rpcMaxConcurrentCap)
	}
}

// TestHandleRPCRequestAdmissionTimeout 覆盖准入接线：占满车道后
// handleRPCRequest 回 error 响应（rpc admission timeout），不 panic 不挂起。
func TestHandleRPCRequestAdmissionTimeout(t *testing.T) {
	a := NewAgent(Config{
		ServerURL:               "ws://127.0.0.1:1",
		RPCMaxConcurrent:        1,
		RPCAdmissionTimeoutS:    1,
		RPCHighAdmissionTimeoutS: 1,
	})
	defer a.Stop()

	// 占满普通车道唯一槽位
	block, err := a.sched.Acquire(a.ctx, "docker.list")
	if err != nil {
		t.Fatalf("fill lane: %v", err)
	}

	req := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]any{
		"method": "docker.list", "params": map[string]any{},
	})
	req.ID = "rpc-t"
	a.handleRPCRequest(req) // ~1s 准入超时后回错误响应
	block()

	select {
	case resp := <-a.outbound:
		raw, ok := resp.Payload["status"].(string)
		if !ok || raw != "error" {
			t.Fatalf("resp status = %v, want error", resp.Payload["status"])
		}
		er, _ := resp.Payload["error"].(string)
		if !strings.Contains(er, "rpc admission timeout") {
			t.Errorf("resp error = %q, want rpc admission timeout", er)
		}
		if resp.ID != "rpc-t" {
			t.Errorf("resp ID = %q, want rpc-t", resp.ID)
		}
	default:
		t.Fatal("expected admission-timeout error response on outbound")
	}
}