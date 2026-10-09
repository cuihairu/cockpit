package probeagent

import (
	"path/filepath"
	"testing"

	"github.com/cuihairu/cockpit/core/healthprobe"
)

// TestWriteStatusFileBadPath 目标目录不存在 → tmp 写失败错误分支。
func TestWriteStatusFileBadPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "no-such-dir", "status.json")
	if err := WriteStatusFile(p, []healthprobe.Snapshot{{Target: "x"}}); err == nil {
		t.Fatal("want error for missing parent dir")
	}
}
