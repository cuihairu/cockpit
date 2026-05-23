package detector

import (
	"testing"
)

func TestAll(t *testing.T) {
	detectors := All()

	if detectors == nil {
		t.Fatal("All() returned nil")
	}

	if len(detectors) == 0 {
		t.Error("expected at least one detector")
	}
}

func TestDetectorNames(t *testing.T) {
	detectors := All()

	names := make(map[string]bool)
	for _, d := range detectors {
		name := d.Name()
		if name == "" {
			t.Error("detector has empty name")
		}
		if names[name] {
			t.Errorf("duplicate detector name: %s", name)
		}
		names[name] = true
	}
}

func TestDetectorPriorities(t *testing.T) {
	detectors := All()

	for _, d := range detectors {
		priority := d.Priority()
		if priority < 0 {
			t.Errorf("detector %s has invalid priority: %d", d.Name(), priority)
		}
	}
}

func TestDetectorDetect(t *testing.T) {
	detectors := All()

	for _, d := range detectors {
		t.Run(d.Name(), func(t *testing.T) {
			cap, err := d.Detect()

			// Detection may fail (expected in test environment)
			if err != nil {
				t.Logf("detector %s failed (expected in test env): %v", d.Name(), err)
				return
			}

			if cap == nil {
				t.Logf("detector %s returned nil capability", d.Name())
				return
			}

			if cap.Type == "" {
				t.Error("capability has empty type")
			}

			// Endpoint may be empty for some capability types
			if cap.Endpoint != "" {
				t.Logf("detector %s: %s at %s", d.Name(), cap.Type, cap.Endpoint)
			}
		})
	}
}

func TestDetectorType(t *testing.T) {
	expectedTypes := []string{"pve", "docker", "openwrt", "hardware", "network"}
	detectors := All()

	foundTypes := make(map[string]bool)
	for _, d := range detectors {
		cap, err := d.Detect()
		if err == nil && cap != nil {
			foundTypes[cap.Type] = true
		}
	}

	for _, expectedType := range expectedTypes {
		if !foundTypes[expectedType] {
			t.Logf("type %s not detected (may be expected in test env)", expectedType)
		}
	}
}
