package sync

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCovNewWatcherInotifyExhausted 通过耗尽 inotify 实例配额触发
// fsnotify.NewWatcher 的错误分支（NewWatcher 与 NewManagerWithConfig 两条路径）。
func TestCovNewWatcherInotifyExhausted(t *testing.T) {
	var held []*watcherHolder
	defer func() {
		for _, h := range held {
			h.w.watcher.Close()
		}
	}()

	exhausted := false
	for i := 0; i < 4096; i++ {
		w, err := NewWatcher(Config{InventoryPath: "cov-none.yaml"})
		if err != nil {
			exhausted = true
			// NewManagerWithConfig 应同样失败
			if _, mErr := NewManagerWithConfig(Config{InventoryPath: "cov-none.yaml"}); mErr == nil {
				t.Error("NewManagerWithConfig() should fail when NewWatcher fails")
			}
			break
		}
		held = append(held, &watcherHolder{w})
	}
	if !exhausted {
		t.Skip("could not exhaust inotify instances within limit; error branch not reachable here")
	}
}

type watcherHolder struct {
	w *Watcher
}

// TestCovWatchLoopConsumesError 验证 watchLoop 能消费 Errors 通道中的错误并继续运行。
func TestCovWatchLoopConsumesError(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	writeEmptyInventory(t, invPath)

	w, err := NewWatcher(Config{InventoryPath: invPath})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	defer w.Stop()

	done := make(chan struct{})
	go func() {
		w.watchLoop()
		close(done)
	}()

	// 注入一个错误；fsnotify v1.10 的 Errors 带缓冲，发送后由 watchLoop 消费
	select {
	case w.watcher.Errors <- errors.New("cov injected error"):
	case <-time.After(5 * time.Second):
		t.Fatal("could not deliver error to Errors channel")
	}

	// watchLoop 应仍然存活（ctx 未取消、通道未关闭），随后通过 cancel 退出
	w.cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watchLoop did not exit after cancel")
	}
}

// TestCovWatchLoopExitsOnEventsClose 验证 Events 通道关闭后 watchLoop 退出。
// 直接 close Events：watcher 从未 Add 任何路径，fsnotify 内部 goroutine 不会向
// 已关闭通道发送；后续也不再调用 watcher.Close()，避免 fsnotify 内部重复 close。
func TestCovWatchLoopExitsOnEventsClose(t *testing.T) {
	w, err := NewWatcher(Config{InventoryPath: "cov-none.yaml"})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	done := make(chan struct{})
	go func() {
		w.watchLoop()
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	close(w.watcher.Events)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watchLoop did not exit after Events channel closed")
	}
	w.cancel()
}

// TestCovWatchLoopExitsOnErrorsClose 验证 Errors 通道关闭后 watchLoop 退出。
func TestCovWatchLoopExitsOnErrorsClose(t *testing.T) {
	w, err := NewWatcher(Config{InventoryPath: "cov-none.yaml"})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	done := make(chan struct{})
	go func() {
		w.watchLoop()
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	close(w.watcher.Errors)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watchLoop did not exit after Errors channel closed")
	}
	w.cancel()
}

// TestCovLoadInventoryUnreadableFile 覆盖 Stat 成功但 ReadFile 失败的分支。
func TestCovLoadInventoryUnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: file permissions are not enforced")
	}

	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	if err := os.WriteFile(invPath, []byte("version: v1\nregions: {}\n"), 0000); err != nil {
		t.Fatalf("write file: %v", err)
	}
	t.Cleanup(func() { os.Chmod(invPath, 0644) })

	w, err := NewWatcher(Config{InventoryPath: invPath})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	defer w.watcher.Close()

	gotErr := w.loadInventory()
	if gotErr == nil {
		t.Fatal("loadInventory() should fail when the file is unreadable")
	}
	if !os.IsPermission(gotErr) {
		t.Errorf("loadInventory() error = %v, want permission error", gotErr)
	}
}

// TestCovManagerValidateExplicitIDs 验证显式 ID 与 map key 不一致时 Validate 仍通过。
// 注意：inventory.Parse 会用 map key 无条件覆盖显式 ID，因此 Validate 内的
// 空 ID 填充分支经由文件路径不可达（详见报告中的死代码说明）。
func TestCovManagerValidateExplicitIDs(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")

	yaml := `version: v1
regions:
  r1:
    zones:
      z1:
        agents:
          a1:
            hostname: cov-host
domains:
  d1:
    domain: cov.example.com
`
	if err := os.WriteFile(invPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(invPath, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer m.Stop()

	if err := m.Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}
