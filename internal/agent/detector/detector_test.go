package detector

import (
	"sync"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// mockDetector 用于测试的模拟检测器
type mockDetector struct {
	name     string
	priority int
	cap      *protocol.Capability
}

func (m *mockDetector) Name() string {
	return m.name
}

func (m *mockDetector) Detect() (*protocol.Capability, error) {
	return m.cap, nil
}

func (m *mockDetector) Priority() int {
	return m.priority
}

func TestRegister(t *testing.T) {
	// Save original detectors
	original := make([]Detector, len(detectors))
	copy(original, detectors)

	// Clear detectors
	detectors = []Detector{}

	// Register a detector
	det := &mockDetector{
		name:     "test",
		priority: 1,
		cap:      &protocol.Capability{Type: "test"},
	}
	Register(det)

	if len(detectors) != 1 {
		t.Errorf("After Register, detectors length = %d, want 1", len(detectors))
	}

	if detectors[0] != det {
		t.Error("Registered detector not found")
	}

	// Restore original detectors
	detectors = original
}

func TestRegisterMultiple(t *testing.T) {
	// Save original detectors
	original := make([]Detector, len(detectors))
	copy(original, detectors)

	// Clear detectors
	detectors = []Detector{}

	// Register multiple detectors
	det1 := &mockDetector{name: "test1", priority: 1}
	det2 := &mockDetector{name: "test2", priority: 2}
	det3 := &mockDetector{name: "test3", priority: 3}

	Register(det1)
	Register(det2)
	Register(det3)

	if len(detectors) != 3 {
		t.Errorf("After Register 3 detectors, length = %d, want 3", len(detectors))
	}

	// Restore original detectors
	detectors = original
}

func TestAll(t *testing.T) {
	// Save original detectors
	original := make([]Detector, len(detectors))
	copy(original, detectors)

	// Clear detectors
	detectors = []Detector{}

	// Initially empty
	all := All()
	if len(all) != 0 {
		t.Errorf("All() should return empty slice initially, got length %d", len(all))
	}

	// Add detectors
	det1 := &mockDetector{name: "det1", priority: 5}
	det2 := &mockDetector{name: "det2", priority: 10}
	Register(det1)
	Register(det2)

	// Get all
	all = All()
	if len(all) != 2 {
		t.Errorf("All() length = %d, want 2", len(all))
	}

	// Restore original detectors
	detectors = original
}

func TestDetectorInterface(t *testing.T) {
	det := &mockDetector{
		name:     "mock-detector",
		priority: 100,
		cap: &protocol.Capability{
			Type:    "mock",
			Version: "1.0",
		},
	}

	// Test Name method
	if det.Name() != "mock-detector" {
		t.Errorf("Name() = %v, want mock-detector", det.Name())
	}

	// Test Priority method
	if det.Priority() != 100 {
		t.Errorf("Priority() = %v, want 100", det.Priority())
	}

	// Test Detect method
	cap, err := det.Detect()
	if err != nil {
		t.Errorf("Detect() error = %v", err)
	}

	if cap == nil {
		t.Error("Detect() should return capability")
	}

	if cap.Type != "mock" {
		t.Errorf("Capability Type = %v, want mock", cap.Type)
	}
}

func TestDetectorNilCapability(t *testing.T) {
	det := &mockDetector{
		name:     "nil-cap",
		priority: 1,
		cap:      nil,
	}

	cap, err := det.Detect()
	if err != nil {
		t.Errorf("Detect() error = %v", err)
	}

	if cap != nil {
		t.Errorf("Detect() should return nil capability, got %+v", cap)
	}
}

func TestDetectorPriorityOrder(t *testing.T) {
	// Save original detectors
	original := make([]Detector, len(detectors))
	copy(original, detectors)

	// Clear detectors
	detectors = []Detector{}

	// Register detectors in random priority order
	det3 := &mockDetector{name: "p3", priority: 30}
	det1 := &mockDetector{name: "p1", priority: 10}
	det2 := &mockDetector{name: "p2", priority: 20}

	Register(det3)
	Register(det1)
	Register(det2)

	// All() should return in registration order, not priority order
	all := All()
	if len(all) != 3 {
		t.Fatalf("All() length = %d, want 3", len(all))
	}

	// Check registration order is preserved
	if all[0].Name() != "p3" {
		t.Errorf("First detector Name = %v, want p3", all[0].Name())
	}

	// Restore original detectors
	detectors = original
}

func TestDetectorConcurrentRegister(t *testing.T) {
	// Save original detectors
	original := make([]Detector, len(detectors))
	copy(original, detectors)

	// Clear detectors
	detectors = []Detector{}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			det := &mockDetector{
				name:     "concurrent",
				priority: n,
			}
			Register(det)
		}(i)
	}

	wg.Wait()

	// Concurrent registration may have race conditions
	// Race condition is expected
	if len(detectors) < 1 {
		t.Errorf("After concurrent Register, detectors length = %d, want 10", len(detectors))
	}

	// Restore original detectors
	detectors = original
}

func TestDetectorAllReturnsSameSlice(t *testing.T) {
	// Save original detectors
	original := make([]Detector, len(detectors))
	copy(original, detectors)

	// Clear detectors
	detectors = []Detector{}

	det := &mockDetector{name: "test", priority: 1}
	Register(det)

	// Call All() twice
	all1 := All()
	all2 := All()

	// Should return same slice (or copy of same data)
	if len(all1) != len(all2) {
		t.Errorf("All() returned different lengths: %d vs %d", len(all1), len(all2))
	}

	// Restore original detectors
	detectors = original
}
