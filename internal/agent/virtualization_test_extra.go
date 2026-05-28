package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectVirtualizationManualNoLinux(t *testing.T) {
	// On Windows, /sys and /proc don't exist, so it should return none
	info := detectVirtualizationManual()
	if info == nil {
		t.Fatal("detectVirtualizationManual() should not return nil")
	}
	// On non-Linux, fallback to default
	if info.Type != VirtTypeNone && info.Role != RoleHost {
		// Could be detected via gopsutil on some platforms
		t.Logf("Type=%s Role=%s (platform dependent)", info.Type, info.Role)
	}
}

func TestReadSysFileNonexistent(t *testing.T) {
	_, err := readSysFile("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("readSysFile() should return error for nonexistent file")
	}
}

func TestReadSysFileSuccess(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("hello world\n"), 0644)

	got, err := readSysFile(fp)
	if err != nil {
		t.Fatalf("readSysFile() error = %v", err)
	}
	if got != "hello world" {
		t.Errorf("readSysFile() = %q, want %q", got, "hello world")
	}
}

func TestReadProcCpuInfoNonexistent(t *testing.T) {
	_, err := readProcCpuInfo()
	if err == nil {
		// On Linux this might succeed
		t.Log("readProcCpuInfo() succeeded (platform dependent)")
	}
}

func TestIsContainerOnWindows(t *testing.T) {
	// On Windows, /.dockerenv and /proc/1/cgroup don't exist
	result := isContainer()
	// Just verify it doesn't panic
	_ = result
}

func TestDetectContainerType(t *testing.T) {
	// On Windows, /proc/1/cgroup doesn't exist, returns default
	vt := detectContainerType()
	_ = vt // just verify no panic
}

func TestRunCommand(t *testing.T) {
	result, err := runCommand("nonexistent-command")
	if err == nil {
		t.Error("runCommand() should return error for nonexistent command")
	}
	if result != "" {
		t.Errorf("runCommand() result = %q, want empty", result)
	}
}
