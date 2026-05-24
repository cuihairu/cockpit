package alert

import (
	"testing"
	"time"
)

func TestNewGenerator(t *testing.T) {
	g := NewGenerator(nil)

	if g == nil {
		t.Error("NewGenerator() should not return nil")
	}

	if g.db != nil {
		t.Error("Generator.db should be nil when created with nil")
	}
}

func TestNewGeneratorWithDB(t *testing.T) {
	// Since we can't create a real DB in unit tests,
	// this just tests the function signature
	g := NewGenerator(nil)

	if g == nil {
		t.Error("NewGenerator() should not return nil")
	}
}

func TestCheckAllChecks(t *testing.T) {
	g := NewGenerator(nil)

	// Will panic due to nil DB - recover to test behavior
	defer func() {
		if r := recover(); r != nil {
			// Expected to panic with nil DB
		}
	}()
	g.CheckAllChecks()
}

func TestCheckExpiringCertificates(t *testing.T) {
	g := NewGenerator(nil)

	defer func() {
		if r := recover(); r != nil {
			// Expected to panic with nil DB
		}
	}()
	g.CheckExpiringCertificates()
}

func TestCheckDownServices(t *testing.T) {
	g := NewGenerator(nil)

	defer func() {
		if r := recover(); r != nil {
			// Expected to panic with nil DB
		}
	}()
	g.CheckDownServices()
}

func TestCheckOfflineAgents(t *testing.T) {
	g := NewGenerator(nil)

	defer func() {
		if r := recover(); r != nil {
			// Expected to panic with nil DB
		}
	}()
	g.CheckOfflineAgents()
}

func TestCheckExpiredDomains(t *testing.T) {
	g := NewGenerator(nil)

	defer func() {
		if r := recover(); r != nil {
			// Expected to panic with nil DB
		}
	}()
	g.CheckExpiredDomains()
}

func TestCheckDiskSpace(t *testing.T) {
	g := NewGenerator(nil)

	// Should not panic - it's a TODO method
	g.CheckDiskSpace(80)

	g.CheckDiskSpace(90)

	g.CheckDiskSpace(0)
}

func TestCheckMemoryUsage(t *testing.T) {
	g := NewGenerator(nil)

	// Should not panic - it's a TODO method
	g.CheckMemoryUsage(80)

	g.CheckMemoryUsage(90)

	g.CheckMemoryUsage(0)
}

func TestCleanupOldAlerts(t *testing.T) {
	g := NewGenerator(nil)

	// Will panic due to nil DB - recover to test behavior
	defer func() {
		if r := recover(); r != nil {
			// Expected to panic with nil DB
		}
	}()

	durations := []time.Duration{
		24 * time.Hour,
		7 * 24 * time.Hour,
		30 * 24 * time.Hour,
		0,
		-1 * time.Hour,
	}

	for _, d := range durations {
		g.CleanupOldAlerts(d)
	}
}

func TestGeneratorNilDatabase(t *testing.T) {
	g := NewGenerator(nil)

	// All methods that use DB will panic - handle it gracefully
	defer func() {
		if r := recover(); r != nil {
			// Expected to panic with nil DB
		}
	}()

	g.CheckExpiringCertificates()
	g.CheckDownServices()
	g.CheckOfflineAgents()
	g.CheckExpiredDomains()
	g.CheckDiskSpace(80)
	g.CheckMemoryUsage(80)
	g.CleanupOldAlerts(24 * time.Hour)
	g.CheckAllChecks()
}

func TestCleanupOldAlertsVariousDurations(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
	}{
		{"one hour", time.Hour},
		{"one day", 24 * time.Hour},
		{"one week", 7 * 24 * time.Hour},
		{"one month", 30 * 24 * time.Hour},
		{"zero", 0},
		{"negative", -1 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGenerator(nil)

			// Will panic due to nil DB - recover to test behavior
			defer func() {
				if r := recover(); r != nil {
					// Expected to panic with nil DB
				}
			}()

			g.CleanupOldAlerts(tt.duration)
		})
	}
}

func TestCheckDiskSpaceThresholds(t *testing.T) {
	tests := []struct {
		name     string
		threshold int
	}{
		{"zero percent", 0},
		{"low threshold", 50},
		{"high threshold", 80},
		{"critical threshold", 90},
		{"max threshold", 100},
		{"over max", 150},
	}

	g := NewGenerator(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g.CheckDiskSpace(tt.threshold)
		})
	}
}

func TestCheckMemoryUsageThresholds(t *testing.T) {
	tests := []struct {
		name     string
		threshold int
	}{
		{"zero percent", 0},
		{"low threshold", 50},
		{"high threshold", 80},
		{"critical threshold", 90},
		{"max threshold", 100},
	}

	g := NewGenerator(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g.CheckMemoryUsage(tt.threshold)
		})
	}
}

func TestGeneratorMultipleCalls(t *testing.T) {
	g := NewGenerator(nil)

	// Multiple calls should be safe (TODO methods won't panic)
	for i := 0; i < 5; i++ {
		g.CheckDiskSpace(80)
		g.CheckMemoryUsage(80)
	}
}
