package storage

import (
	"testing"
	"time"
)

func TestCreateAlert(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	alert := &Alert{
		Type:    "warning",
		Title:   "Test Alert",
		Message: "Something happened",
	}
	if err := db.CreateAlert(alert); err != nil {
		t.Fatalf("CreateAlert() error = %v", err)
	}
	if alert.ID == "" {
		t.Error("Alert.ID should be set by BeforeCreate hook")
	}
}

func TestGetAlert(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	alert := &Alert{Type: "info", Title: "T", Message: "M"}
	db.CreateAlert(alert)

	got, err := db.GetAlert(alert.ID)
	if err != nil {
		t.Fatalf("GetAlert() error = %v", err)
	}
	if got.Title != "T" {
		t.Errorf("Title = %v, want T", got.Title)
	}
}

func TestGetAlertNotFound(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	_, err := db.GetAlert("nonexistent")
	if err != ErrNotFound {
		t.Errorf("GetAlert(nonexistent) error = %v, want ErrNotFound", err)
	}
}

func TestListAlerts(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	for i := 0; i < 5; i++ {
		db.CreateAlert(&Alert{Type: "info", Title: "Alert", Message: "msg"})
	}

	alerts, err := db.ListAlerts(3)
	if err != nil {
		t.Fatalf("ListAlerts() error = %v", err)
	}
	if len(alerts) != 3 {
		t.Errorf("ListAlerts(3) count = %d, want 3", len(alerts))
	}

	all, _ := db.ListAlerts(0)
	if len(all) != 5 {
		t.Errorf("ListAlerts(0) count = %d, want 5", len(all))
	}
}

func TestListUnreadAlerts(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateAlert(&Alert{Type: "info", Title: "Unread", Message: "m", Read: false})
	db.CreateAlert(&Alert{Type: "info", Title: "Read", Message: "m", Read: true})

	alerts, err := db.ListUnreadAlerts()
	if err != nil {
		t.Fatalf("ListUnreadAlerts() error = %v", err)
	}
	if len(alerts) != 1 {
		t.Errorf("ListUnreadAlerts() count = %d, want 1", len(alerts))
	}
}

func TestMarkAlertAsRead(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	alert := &Alert{Type: "info", Title: "T", Message: "M", Read: false}
	db.CreateAlert(alert)

	if err := db.MarkAlertAsRead(alert.ID); err != nil {
		t.Fatalf("MarkAlertAsRead() error = %v", err)
	}

	got, _ := db.GetAlert(alert.ID)
	if !got.Read {
		t.Error("Alert should be marked as read")
	}
}

func TestMarkAllAlertsAsRead(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateAlert(&Alert{Type: "info", Title: "A1", Message: "m"})
	db.CreateAlert(&Alert{Type: "info", Title: "A2", Message: "m"})

	if err := db.MarkAllAlertsAsRead(); err != nil {
		t.Fatalf("MarkAllAlertsAsRead() error = %v", err)
	}

	unread, _ := db.ListUnreadAlerts()
	if len(unread) != 0 {
		t.Errorf("Unread count = %d, want 0", len(unread))
	}
}

func TestDeleteAlert(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	alert := &Alert{Type: "info", Title: "T", Message: "M"}
	db.CreateAlert(alert)

	if err := db.DeleteAlert(alert.ID); err != nil {
		t.Fatalf("DeleteAlert() error = %v", err)
	}

	_, err := db.GetAlert(alert.ID)
	if err != ErrNotFound {
		t.Errorf("GetAlert() after delete error = %v, want ErrNotFound", err)
	}
}

func TestDeleteOldAlerts(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// Create alerts with different ages by directly manipulating created_at
	old := &Alert{Type: "info", Title: "Old", Message: "m"}
	db.CreateAlert(old)
	db.db.Model(&Alert{}).Where("id = ?", old.ID).Update("created_at", time.Now().Add(-48*time.Hour))

	recent := &Alert{Type: "info", Title: "Recent", Message: "m"}
	db.CreateAlert(recent)

	if err := db.DeleteOldAlerts(24 * time.Hour); err != nil {
		t.Fatalf("DeleteOldAlerts() error = %v", err)
	}

	alerts, _ := db.ListAlerts(0)
	if len(alerts) != 1 || alerts[0].Title != "Recent" {
		t.Errorf("After DeleteOldAlerts, got %d alerts, want 1 (Recent)", len(alerts))
	}
}

func TestCreateSystemAlert(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if err := db.CreateSystemAlert("error", "System Error", "Disk full"); err != nil {
		t.Fatalf("CreateSystemAlert() error = %v", err)
	}

	alerts, _ := db.ListAlerts(1)
	if len(alerts) != 1 {
		t.Fatalf("ListAlerts() count = %d, want 1", len(alerts))
	}
	if alerts[0].Type != "error" {
		t.Errorf("Type = %v, want error", alerts[0].Type)
	}
	if alerts[0].Read {
		t.Error("System alert should be unread by default")
	}
}

func TestGetUnreadCount(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateAlert(&Alert{Type: "info", Title: "A1", Message: "m", Read: false})
	db.CreateAlert(&Alert{Type: "info", Title: "A2", Message: "m", Read: false})
	db.CreateAlert(&Alert{Type: "info", Title: "A3", Message: "m", Read: true})

	count, err := db.GetUnreadCount()
	if err != nil {
		t.Fatalf("GetUnreadCount() error = %v", err)
	}
	if count != 2 {
		t.Errorf("UnreadCount = %d, want 2", count)
	}
}
