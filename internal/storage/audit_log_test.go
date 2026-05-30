package storage

import (
	"testing"
	"time"
)

func TestCreateAuditLog(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	log := &AuditLog{
		UserID:   "1",
		Username: "admin",
		Action:   "login",
		Resource: "session",
		Status:   "success",
		IP:       "192.168.1.1",
	}
	if err := db.CreateAuditLog(log); err != nil {
		t.Fatalf("CreateAuditLog() error = %v", err)
	}
	if log.ID == 0 {
		t.Error("AuditLog.ID should be set after creation")
	}
}

func TestGetAuditLogs(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	for i := 0; i < 5; i++ {
		db.CreateAuditLog(&AuditLog{
			Username: "admin",
			Action:   "login",
			Resource: "session",
			Status:   "success",
		})
	}

	logs, total, err := db.GetAuditLogs(0, 10, nil)
	if err != nil {
		t.Fatalf("GetAuditLogs() error = %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(logs) != 5 {
		t.Errorf("logs count = %d, want 5", len(logs))
	}
}

func TestGetAuditLogsWithFilters(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateAuditLog(&AuditLog{Username: "admin", Action: "login", Resource: "session", Status: "success"})
	db.CreateAuditLog(&AuditLog{Username: "admin", Action: "delete", Resource: "agent", Status: "failure"})
	db.CreateAuditLog(&AuditLog{Username: "bob", Action: "login", Resource: "session", Status: "success"})

	// Filter by action
	_, total, _ := db.GetAuditLogs(0, 10, map[string]interface{}{"action": "login"})
	if total != 2 {
		t.Errorf("filter by action: total = %d, want 2", total)
	}

	// Filter by status
	_, total, _ = db.GetAuditLogs(0, 10, map[string]interface{}{"status": "failure"})
	if total != 1 {
		t.Errorf("filter by status: total = %d, want 1", total)
	}

	// Filter by resource
	_, total, _ = db.GetAuditLogs(0, 10, map[string]interface{}{"resource": "agent"})
	if total != 1 {
		t.Errorf("filter by resource: total = %d, want 1", total)
	}

	// Filter by username (LIKE)
	_, total, _ = db.GetAuditLogs(0, 10, map[string]interface{}{"username": "bob"})
	if total != 1 {
		t.Errorf("filter by username: total = %d, want 1", total)
	}
}

func TestGetAuditLogsWithTimeFilter(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateAuditLog(&AuditLog{Username: "admin", Action: "login", Status: "success"})

	// Use wide time range to avoid precision issues with SQLite
	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(24 * time.Hour)
	_, total, _ := db.GetAuditLogs(0, 10, map[string]interface{}{
		"start_time": past,
		"end_time":   future,
	})
	if total != 1 {
		t.Errorf("time filter: total = %d, want 1", total)
	}

	_, total, _ = db.GetAuditLogs(0, 10, map[string]interface{}{
		"start_time": future,
	})
	if total != 0 {
		t.Errorf("future start_time: total = %d, want 0", total)
	}
}

func TestGetAuditLogsPagination(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	for i := 0; i < 10; i++ {
		db.CreateAuditLog(&AuditLog{Username: "admin", Action: "login", Status: "success"})
	}

	logs, total, _ := db.GetAuditLogs(5, 3, nil)
	if total != 10 {
		t.Errorf("total = %d, want 10", total)
	}
	if len(logs) != 3 {
		t.Errorf("page size = %d, want 3", len(logs))
	}
}

func TestGetAuditLogByID(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	log := &AuditLog{Username: "admin", Action: "login", Status: "success"}
	db.CreateAuditLog(log)

	got, err := db.GetAuditLogByID(log.ID)
	if err != nil {
		t.Fatalf("GetAuditLogByID() error = %v", err)
	}
	if got.Username != "admin" {
		t.Errorf("Username = %v, want admin", got.Username)
	}
}

func TestGetAuditLogByIDNotFound(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	_, err := db.GetAuditLogByID(9999)
	if err == nil {
		t.Error("GetAuditLogByID(9999) should return error")
	}
}

func TestDeleteAuditLogsBefore(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// Create and backdate
	old := &AuditLog{Username: "admin", Action: "login", Status: "success"}
	db.CreateAuditLog(old)
	db.db.Model(&AuditLog{}).Where("id = ?", old.ID).Update("created_at", time.Now().Add(-48*time.Hour))

	// Create recent
	db.CreateAuditLog(&AuditLog{Username: "admin", Action: "login", Status: "success"})

	deleted, err := db.DeleteAuditLogsBefore(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("DeleteAuditLogsBefore() error = %v", err)
	}
	if deleted != 1 {
		t.Errorf("RowsAffected = %d, want 1", deleted)
	}
}

func TestGetAuditLogStats(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateAuditLog(&AuditLog{Username: "admin", Action: "login", Status: "success"})
	db.CreateAuditLog(&AuditLog{Username: "admin", Action: "delete", Status: "failure"})
	db.CreateAuditLog(&AuditLog{Username: "bob", Action: "login", Status: "success"})

	stats, err := db.GetAuditLogStats()
	if err != nil {
		t.Fatalf("GetAuditLogStats() error = %v", err)
	}
	if stats["total_logs"].(int64) != 3 {
		t.Errorf("total_logs = %v, want 3", stats["total_logs"])
	}
	if stats["failed_logs"].(int64) != 1 {
		t.Errorf("failed_logs = %v, want 1", stats["failed_logs"])
	}
}

func TestAuditLogTableName(t *testing.T) {
	l := AuditLog{}
	if l.TableName() != "audit_logs" {
		t.Errorf("TableName() = %v, want audit_logs", l.TableName())
	}
}
