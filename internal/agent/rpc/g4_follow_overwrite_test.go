package rpc

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestG4FollowSameIDStopsOldPump 验证 G4：同 followId 覆盖后旧 pump 必须
// 退出（stop 通道被关、done 关闭），不得与新的 pump 并发向同一 proxyId
// 推流（无界重复泵与重复 proxy_data）。
func TestG4FollowSameIDStopsOldPump(t *testing.T) {
	dir := t.TempDir()
	seedStackDir(t, dir, "web")
	p := gitDeployProvider(t, dir, true) // up 睡 1s：留出旧 pump 运行窗口

	var mu sync.Mutex
	senderCount := 0
	p.SetSender(func(proxyID string, data []byte) {
		mu.Lock()
		senderCount++
		mu.Unlock()
	})
	closed := make(chan string, 8)
	p.SetCloser(func(proxyID, reason string) { closed <- reason })

	res, err := p.UpStack("web")
	if err != nil {
		t.Fatal(err)
	}
	taskID := res["taskId"].(string)

	// 起第一个 follow。
	if _, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": "f1"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // 让旧 pump 至少跑一拍

	// 同 followId 覆盖：旧 pump 应被停止。
	if _, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": "f1"}); err != nil {
		t.Fatal(err)
	}

	// 覆盖触发旧 pump 的 stop close（stopped），随后新 pump 在任务终态
	// 收尾（success）。收集两个 reason。
	sawStopped, sawSuccess := false, false
	deadline := time.After(5 * time.Second)
	for !sawSuccess || !sawStopped {
		select {
		case reason := <-closed:
			switch reason {
			case "stopped":
				sawStopped = true
			case stackTaskSuccess:
				sawSuccess = true
			}
		case <-deadline:
			t.Fatalf("follow close missing: stopped=%v success=%v", sawStopped, sawSuccess)
		}
	}

	// 稍候再确认没有并发重复推流：senderCount 不应异常暴增（若无界
	// 重复泵，覆盖后旧 pump 每 300ms 持续推，计数远高于单 pump）。
	time.Sleep(500 * time.Millisecond)
	mu.Lock()
	n := senderCount
	mu.Unlock()
	if n > 50 {
		t.Fatalf("sender invoked %d times: unbounded duplicate pump suspected", n)
	}

	// 会话表最终只保留一份且可再 stop（new 会话已随终态清理）。
	p.mu.Lock()
	_, exists := p.taskFollows["f1"]
	p.mu.Unlock()
	if exists {
		t.Error("follow session should be cleaned after terminal close")
	}

	waitForTask(t, p, taskID)
}

// TestG4FollowOverwriteImmediately 覆盖后旧 pump 的 stop 分支应立即关 done
// 并释放，不卡死 FollowTask（回归死锁风险：锁内等待旧 pump）。
func TestG4FollowOverwriteImmediately(t *testing.T) {
	dir := t.TempDir()
	seedStackDir(t, dir, "web")
	p := gitDeployProvider(t, dir, false)
	p.SetSender(func(string, []byte) {})
	closed := make(chan string, 8)
	p.SetCloser(func(proxyID, reason string) { closed <- reason })

	res, err := p.UpStack("web")
	if err != nil {
		t.Fatal(err)
	}
	taskID := res["taskId"].(string)

	if _, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": "f1"}); err != nil {
		t.Fatal(err)
	}
	// 立即覆盖（旧 pump 可能正持 p.mu）：不应死锁。
	done := make(chan error, 1)
	go func() {
		_, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": "f1"})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("overwrite follow: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("FollowTask overwrite deadlocked")
	}

	waitForTask(t, p, taskID)
}

// TestG4FollowReplaceAtCapacity 验证 G4 修复后的容量语义：taskFollows 满
// 8 个时，同 followId 覆盖（不新增活跃计数）仍被允许——与 logs_follow.go
// G9「替换不计容量」一致；不同 followId 依旧被拒绝。
func TestG4FollowReplaceAtCapacity(t *testing.T) {
	dir := t.TempDir()
	seedStackDir(t, dir, "web")
	p := gitDeployProvider(t, dir, true)
	p.SetSender(func(string, []byte) {})
	closed := make(chan string, 16)
	p.SetCloser(func(_ string, reason string) { closed <- reason })

	res, err := p.UpStack("web")
	if err != nil {
		t.Fatal(err)
	}
	taskID := res["taskId"].(string)

	// 填满 8 个不同 followId。
	for i := 0; i < stackTaskFollowMaxActive; i++ {
		if _, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": fmt.Sprintf("f%d", i)}); err != nil {
			t.Fatalf("follow %d: %v", i, err)
		}
	}

	// 满员时替换既有 followId 必须成功（旧 pump 被停）。
	if _, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": "f0"}); err != nil {
		t.Fatalf("replace at capacity should succeed, got: %v", err)
	}

	// 满员时新增不同 followId 仍被拒。
	if _, err := p.FollowTask(map[string]interface{}{"taskId": taskID, "followId": "over"}); err == nil ||
		!strings.Contains(err.Error(), "too many") {
		t.Fatalf("over-limit err = %v", err)
	}

	waitForTask(t, p, taskID)
}
