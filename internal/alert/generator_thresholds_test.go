package alert

import (
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

func TestChecksTolerateClosedDB(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)
	db.Close()

	// All list queries fail; each check must log and return without panic.
	g.CheckExpiringCertificates()
	g.CheckDownServices()
	g.CheckOfflineAgents()
	g.CheckExpiredDomains()
	g.CheckDiskSpace(80)
	g.CheckMemoryUsage(85)
	g.CleanupOldAlerts(time.Hour)
}

func TestCheckDiskSpaceThresholds(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	high := &storage.SystemInfoSnapshot{
		AgentID:           "agent-high",
		Hostname:          "high-host",
		DiskUsagePercent:  91.5,
	}
	if err := db.UpdateSystemInfoSnapshot(high); err != nil {
		t.Fatalf("UpdateSystemInfoSnapshot: %v", err)
	}

	low := &storage.SystemInfoSnapshot{
		AgentID:           "agent-low",
		Hostname:          "low-host",
		DiskUsagePercent:  40,
	}
	if err := db.UpdateSystemInfoSnapshot(low); err != nil {
		t.Fatalf("UpdateSystemInfoSnapshot: %v", err)
	}

	g.CheckDiskSpace(80)

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("Expected 1 disk alert, got %d", len(alerts))
	}
	if alerts[0].Title != "磁盘空间不足" {
		t.Errorf("Title = %q, want 磁盘空间不足", alerts[0].Title)
	}
	if alerts[0].Type != "warning" {
		t.Errorf("Type = %q, want warning", alerts[0].Type)
	}
}

func TestCheckMemoryUsageThresholds(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	high := &storage.SystemInfoSnapshot{
		AgentID:           "agent-high",
		Hostname:          "high-host",
		MemUsagePercent:   90,
	}
	if err := db.UpdateSystemInfoSnapshot(high); err != nil {
		t.Fatalf("UpdateSystemInfoSnapshot: %v", err)
	}

	low := &storage.SystemInfoSnapshot{
		AgentID:           "agent-low",
		Hostname:          "low-host",
		MemUsagePercent:   50,
	}
	if err := db.UpdateSystemInfoSnapshot(low); err != nil {
		t.Fatalf("UpdateSystemInfoSnapshot: %v", err)
	}

	g.CheckMemoryUsage(85)

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("Expected 1 memory alert, got %d", len(alerts))
	}
	if alerts[0].Title != "内存使用率过高" {
		t.Errorf("Title = %q, want 内存使用率过高", alerts[0].Title)
	}
	if alerts[0].Type != "warning" {
		t.Errorf("Type = %q, want warning", alerts[0].Type)
	}
}

func TestCleanupOldAlertsDeletesOld(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	alert := &storage.Alert{
		Type:    "info",
		Title:   "stale",
		Message: "old message",
		Read:    true,
	}
	if err := db.CreateAlert(alert); err != nil {
		t.Fatalf("CreateAlert: %v", err)
	}

	g.CleanupOldAlerts(1 * time.Nanosecond)

	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("Expected old alerts to be cleaned, got %d", len(alerts))
	}
}
