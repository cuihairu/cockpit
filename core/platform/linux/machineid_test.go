//go:build linux

package linux

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMachineIDBranches machineID 读取失败/空内容/正常裁剪三分支
// （machineIDPath 注入点，自 internal/agent TestCovMachineIDBranches 迁移）。
func TestMachineIDBranches(t *testing.T) {
	saved := machineIDPath
	t.Cleanup(func() { machineIDPath = saved })

	h := Host{}

	machineIDPath = filepath.Join(t.TempDir(), "missing")
	if got := h.MachineID(); got != "" {
		t.Errorf("missing file = %q, want empty", got)
	}

	empty := filepath.Join(t.TempDir(), "empty")
	os.WriteFile(empty, []byte("   \n"), 0644)
	machineIDPath = empty
	if got := h.MachineID(); got != "" {
		t.Errorf("blank file = %q, want empty", got)
	}

	normal := filepath.Join(t.TempDir(), "normal")
	os.WriteFile(normal, []byte("  0123456789abcdef  \n"), 0644)
	machineIDPath = normal
	if got := h.MachineID(); got != "0123456789abcdef" {
		t.Errorf("MachineID() = %q, want trimmed id", got)
	}
}
