package sync

// cov_io_gaps2_test.go 覆盖 watcher 的 Start 完整路径（含 watchLoop 的
// 防抖定时器排空）与 onReload 返回错误的日志分支。

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/inventory"
)

// covWriteInv 写最小合法 inventory
func covWriteInv(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCovWatcherStartOnReloadError(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.yaml")
	covWriteInv(t, invPath, "version: v1\nregions: {}\ndomains: {}\n")

	w, err := NewWatcher(Config{
		InventoryPath: invPath,
		OnReload:      func(*inventory.Inventory) error { return errors.New("cov reload boom") },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// 防抖定时器（NewTimer(0)）在 watchLoop 启动时被 Stop+排空；
	// 给一点调度时间
	time.Sleep(50 * time.Millisecond)
}
