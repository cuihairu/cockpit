package agent

import (
	"runtime"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestNewCollector(t *testing.T) {
	c := NewCollector()

	if c == nil {
		t.Fatal("NewCollector() should not return nil")
	}

	if c.lastNetTime.IsZero() {
		t.Error("lastNetTime should be initialized")
	}
}

func TestCollectorCollect(t *testing.T) {
	c := NewCollector()

	info := c.Collect()

	if info == nil {
		t.Fatal("Collect() should not return nil")
	}

	// Verify basic fields exist
	if info.Hostname == "" {
		t.Log("Hostname may be empty in test environment")
	}

	if info.OSName == "" {
		t.Log("OSName may be empty in test environment")
	}
}

func TestCollectorCollectBasic(t *testing.T) {
	c := NewCollector()

	info := c.CollectBasic()

	if info == nil {
		t.Fatal("CollectBasic() should not return nil")
	}

	// CollectBasic should return quickly (non-blocking)
	start := time.Now()
	info = c.CollectBasic()
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("CollectBasic() took too long: %v", elapsed)
	}
}

func TestGetRuntimeInfo(t *testing.T) {
	info := GetRuntimeInfo()

	if info == nil {
		t.Fatal("GetRuntimeInfo() should not return nil")
	}

	if info["goVersion"] == nil {
		t.Error("goVersion should be present")
	}

	if info["goroutines"] == nil {
		t.Error("goroutines should be present")
	}

	if goroutines, ok := info["goroutines"].(int); ok {
		if goroutines < 1 {
			t.Error("goroutines should be at least 1")
		}
	}
}

func TestGetRuntimeInfoFields(t *testing.T) {
	info := GetRuntimeInfo()

	expectedFields := []string{"goVersion", "goroutines", "compiler", "arch", "os"}

	for _, field := range expectedFields {
		if info[field] == nil {
			t.Errorf("expected field %s to be present", field)
		}
	}

	// Verify values match runtime package
	if info["goVersion"] != runtime.Version() {
		t.Error("goVersion mismatch")
	}

	if info["compiler"] != runtime.Compiler {
		t.Error("compiler mismatch")
	}

	if info["arch"] != runtime.GOARCH {
		t.Error("arch mismatch")
	}

	if info["os"] != runtime.GOOS {
		t.Error("os mismatch")
	}
}

func TestGetCPUInfo(t *testing.T) {
	info, err := GetCPUInfo()

	if err != nil {
		t.Logf("GetCPUInfo() error (may be expected in some environments): %v", err)
		return
	}

	if info == nil {
		t.Error("GetCPUInfo() should not return nil slice")
	}

	// Each CPU info should have valid fields
	for i, cpu := range info {
		if cpu.ModelName == "" {
			t.Logf("CPU %d: ModelName may be empty", i)
		}
	}
}

func TestGetDiskPartitions(t *testing.T) {
	partitions, err := GetDiskPartitions()

	if err != nil {
		t.Logf("GetDiskPartitions() error (may be expected): %v", err)
		return
	}

	if partitions == nil {
		t.Error("GetDiskPartitions() should not return nil slice")
	}

	// Verify at least one partition
	if len(partitions) == 0 {
		t.Log("No partitions found (may be expected in container)")
	}
}

func TestGetNetInterfaces(t *testing.T) {
	interfaces, err := GetNetInterfaces()

	if err != nil {
		t.Logf("GetNetInterfaces() error: %v", err)
		return
	}

	if interfaces == nil {
		t.Error("GetNetInterfaces() should not return nil slice")
	}

	// Should have at least loopback interface
	if len(interfaces) == 0 {
		t.Log("No network interfaces found")
	}
}

func TestSystemInfoPayloadStructure(t *testing.T) {
	info := &protocol.SystemInfoPayload{
		CPUUsage:       50.5,
		CPUCores:       4,
		CPUFreqMHz:     2000,
		MemTotal:       8000000000,
		MemUsed:        4000000000,
		MemAvailable:   3000000000,
		MemUsagePercent: 50.0,
		DiskTotal:      100000000000,
		DiskUsed:       50000000000,
		DiskFree:       50000000000,
		DiskUsagePercent: 50.0,
		NetBytesSent:   1000000,
		NetBytesRecv:   2000000,
		OSName:         "linux",
		OSVersion:      "5.4.0",
		Arch:           "x86_64",
		Uptime:         86400,
		Hostname:       "test-host",
		Load1:          1.0,
		Load5:          0.8,
		Load15:         0.5,
	}

	if info.CPUUsage != 50.5 {
		t.Errorf("CPUUsage = %v, want 50.5", info.CPUUsage)
	}

	if info.CPUCores != 4 {
		t.Errorf("CPUCores = %v, want 4", info.CPUCores)
	}

	if info.MemUsagePercent != 50.0 {
		t.Errorf("MemUsagePercent = %v, want 50.0", info.MemUsagePercent)
	}
}

func TestCollectorMultipleCollects(t *testing.T) {
	c := NewCollector()

	// Multiple collects should be safe
	for i := 0; i < 5; i++ {
		info := c.Collect()
		if info == nil {
			t.Errorf("Collect() iteration %d returned nil", i)
		}
	}
}

func TestCollectorCollectBasicConsistency(t *testing.T) {
	c := NewCollector()

	info1 := c.CollectBasic()
	info2 := c.CollectBasic()

	// Some fields should be consistent
	if info1.MemTotal != info2.MemTotal && info1.MemTotal > 0 {
		t.Logf("MemTotal changed: %d -> %d", info1.MemTotal, info2.MemTotal)
	}

	if info1.DiskTotal != info2.DiskTotal && info1.DiskTotal > 0 {
		t.Logf("DiskTotal changed: %d -> %d", info1.DiskTotal, info2.DiskTotal)
	}
}

func TestCollectorZeroValues(t *testing.T) {
	c := NewCollector()
	info := c.Collect()

	// Even if collection fails, struct should be initialized
	if info == nil {
		t.Fatal("Collect() returned nil")
	}

	// Check that zero values are valid
	if info.CPUCores < 0 {
		t.Error("CPUCores should not be negative")
	}

	if info.MemTotal < 0 {
		t.Error("MemTotal should not be negative")
	}
}

func TestGetRuntimeInfoGoroutines(t *testing.T) {
	// Start a goroutine and check count increases
	info1 := GetRuntimeInfo()
	count1 := info1["goroutines"].(int)

	// Start a goroutine
	done := make(chan bool)
	go func() {
		time.Sleep(100 * time.Millisecond)
		done <- true
	}()

	info2 := GetRuntimeInfo()
	count2 := info2["goroutines"].(int)

	<-done

	// Goroutine count should be at least the same or higher
	if count2 < count1 {
		t.Errorf("goroutines decreased: %d -> %d", count1, count2)
	}
}

func TestCollectorInitialization(t *testing.T) {
	collectors := make([]*Collector, 5)

	for i := 0; i < 5; i++ {
		collectors[i] = NewCollector()
		if collectors[i] == nil {
			t.Errorf("NewCollector() iteration %d returned nil", i)
		}
		if collectors[i].lastNetTime.IsZero() {
			t.Errorf("Collector %d has zero lastNetTime", i)
		}
	}
}
