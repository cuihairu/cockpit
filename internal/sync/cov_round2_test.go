package sync

// cov_round2_test.go 覆盖率：watchLoop 的去抖定时器初始化分支。
// ctx 已取消时直接同步调用：Timer(0) 先 fire → Stop 返回 false →
// 排干 channel → select 命中 ctx.Done() 返回，全程不阻塞。

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCovWatcherLoopDebounceTimer watchLoop 入口的 Timer(0)/Stop false
// 排干分支（fsnotify 事件路径由 Start 的常驻协程覆盖）
func TestCovWatcherLoopDebounceTimer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inv.yaml")
	if err := os.WriteFile(path, []byte("version: v1\ndomains: {}\nregions: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(path, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(m.Stop)

	m.watcher.cancel() // 让 watchLoop 的 select 立即走 ctx.Done()
	m.watcher.watchLoop()
}
