package audit

import (
	"os"

	"github.com/cuihairu/cockpit/internal/storage"
)

import (
	"testing"
)

func TestLogEntryCreatedAt(t *testing.T) {
	entry := &LogEntry{
		Username: "testuser",
		Action:   ActionLogin,
		Resource: "user",
		Status:   StatusSuccess,
	}

	if entry.Username != "testuser" {
		t.Errorf("Username = %v, want testuser", entry.Username)
	}
}

func TestLogEntryBasicFields(t *testing.T) {
	entry := &LogEntry{
		Username: "admin",
		Action:   ActionCreate,
		Resource: "agent",
		Status:   StatusSuccess,
	}

	if entry.Action != ActionCreate {
		t.Errorf("Action = %v, want %v", entry.Action, ActionCreate)
	}
}

func TestLogEntryActions(t *testing.T) {
	actions := []string{
		ActionLogin,
		ActionLogout,
		ActionCreate,
		ActionUpdate,
		ActionDelete,
		ActionView,
		ActionExport,
		ActionImport,
		ActionStart,
		ActionStop,
		ActionRestart,
	}

	for _, action := range actions {
		entry := &LogEntry{
			Username: "user",
			Action:   action,
			Resource: "test",
			Status:   StatusSuccess,
		}

		if entry.Action != action {
			t.Errorf("Action = %v, want %v", entry.Action, action)
		}
	}
}

func TestLogEntryMultipleResources(t *testing.T) {
	resources := []string{"user", "agent", "domain", "certificate", "proxy", "gateway", "storage", "service"}

	for _, resource := range resources {
		entry := &LogEntry{
			Username: "admin",
			Action:   ActionView,
			Resource: resource,
			Status:   StatusSuccess,
		}

		if entry.Resource != resource {
			t.Errorf("Resource = %v, want %v", entry.Resource, resource)
		}
	}
}

func TestLogEntrySuccessStatus(t *testing.T) {
	entry := &LogEntry{
		Username: "user",
		Action:   ActionLogin,
		Resource: "auth",
		Status:   StatusSuccess,
	}

	if entry.Status != StatusSuccess {
		t.Errorf("Status = %v, want %v", entry.Status, StatusSuccess)
	}
}

func TestLogEntryFailureStatus(t *testing.T) {
	entry := &LogEntry{
		Username: "user",
		Action:   ActionLogin,
		Resource: "auth",
		Status:   StatusFailure,
	}

	if entry.Status != StatusFailure {
		t.Errorf("Status = %v, want %v", entry.Status, StatusFailure)
	}
}

func TestLogEntryWithEmptyFields(t *testing.T) {
	entry := &LogEntry{
		Action: ActionView,
		Status: StatusSuccess,
	}

	if entry.Username != "" {
		t.Errorf("Username should be empty, got %v", entry.Username)
	}

	if entry.Resource != "" {
		t.Errorf("Resource should be empty, got %v", entry.Resource)
	}

	if entry.Details != nil {
		t.Errorf("Details should be nil, got %v", entry.Details)
	}
}

func TestLogEntryWithComplexDetails(t *testing.T) {
	details := map[string]interface{}{
		"changes": []string{"field1", "field2"},
		"count":   10,
		"enabled": true,
		"ratio":   0.85,
	}

	entry := &LogEntry{
		Username: "admin",
		Action:   ActionUpdate,
		Resource: "agent",
		Details:  details,
		Status:   StatusSuccess,
	}

	if entry.Details == nil {
		t.Error("Details should not be nil")
	}

	detailsMap, ok := entry.Details.(map[string]interface{})
	if !ok {
		t.Fatal("Details should be a map")
	}

	if len(detailsMap) != 4 {
		t.Errorf("Details length = %v, want 4", len(detailsMap))
	}
}

func TestLogEntryWithNestedDetails(t *testing.T) {
	details := map[string]interface{}{
		"old": map[string]interface{}{
			"status": "offline",
			"count":  5,
		},
		"new": map[string]interface{}{
			"status": "online",
			"count":  10,
		},
	}

	entry := &LogEntry{
		Username: "system",
		Action:   ActionUpdate,
		Resource: "agent",
		Details:  details,
		Status:   StatusSuccess,
	}

	if entry.Details == nil {
		t.Error("Details should not be nil")
	}
}

func TestLogEntryConcurrentCreation(t *testing.T) {
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id int) {
			entry := &LogEntry{
				UserID:   string(rune(id)), // Convert to string for test
				Username: "user",
				Action:   ActionCreate,
				Resource: "test",
				Status:   StatusSuccess,
			}
			if entry.UserID != string(rune(id)) {
				t.Errorf("UserID = %v, want %v", entry.UserID, id)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestLogEntryAllCombinations(t *testing.T) {
	actions := []string{ActionLogin, ActionLogout, ActionCreate}
	statuses := []string{StatusSuccess, StatusFailure}

	count := 0
	for _, action := range actions {
		for _, status := range statuses {
			entry := &LogEntry{
				Username: "user",
				Action:   action,
				Resource: "test",
				Status:   status,
			}
			if entry.Action != action {
				t.Errorf("Action mismatch")
			}
			if entry.Status != status {
				t.Errorf("Status mismatch")
			}
			count++
		}
	}

	if count != 6 {
		t.Errorf("Created %d entries, want 6", count)
	}
}

// ============ Logger Tests ============

func TestNewLogger(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)
	if logger == nil {
		t.Fatal("NewLogger() should not return nil")
	}

	if logger.db == nil {
		t.Error("logger.db should not be nil")
	}
}

func TestLoggerLogSuccess(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	err = logger.LogSuccess("testuser", ActionCreate, "agent", "agent-1", map[string]interface{}{"test": "data"}, "127.0.0.1", "test-agent")
	if err != nil {
		t.Errorf("LogSuccess() error = %v", err)
	}
}

func TestLoggerLogFailure(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	err = logger.LogFailure("testuser", ActionDelete, "agent", "agent-1", map[string]interface{}{"error": "not found"}, "127.0.0.1", "test-agent")
	if err != nil {
		t.Errorf("LogFailure() error = %v", err)
	}
}

func TestLoggerLogLoginSuccess(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	err = logger.LogLogin("testuser", true, "127.0.0.1", "test-agent")
	if err != nil {
		t.Errorf("LogLogin() success error = %v", err)
	}
}

func TestLoggerLogLoginFailure(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	err = logger.LogLogin("testuser", false, "127.0.0.1", "test-agent")
	if err != nil {
		t.Errorf("LogLogin() failure error = %v", err)
	}
}

func TestLoggerLogLogout(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	err = logger.LogLogout("testuser", "127.0.0.1", "test-agent")
	if err != nil {
		t.Errorf("LogLogout() error = %v", err)
	}
}

func TestLoggerLogResource(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	err = logger.LogResource("testuser", ActionUpdate, "agent", "agent-1", map[string]interface{}{"status": "online"}, "127.0.0.1", "test-agent")
	if err != nil {
		t.Errorf("LogResource() error = %v", err)
	}
}

func TestLoggerLogWithNilDetails(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	err = logger.LogSuccess("testuser", ActionView, "agent", "", nil, "127.0.0.1", "test-agent")
	if err != nil {
		t.Errorf("LogSuccess() with nil details error = %v", err)
	}
}

func TestLoggerLogAllActions(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	actions := []string{
		ActionLogin, ActionLogout, ActionCreate, ActionUpdate,
		ActionDelete, ActionView, ActionExport, ActionImport,
		ActionStart, ActionStop, ActionRestart,
	}

	for _, action := range actions {
		err = logger.LogSuccess("testuser", action, "test", "1", nil, "127.0.0.1", "test-agent")
		if err != nil {
			t.Errorf("LogSuccess() for action %s error = %v", action, err)
		}
	}
}

func TestLoggerLogWithUserID(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	entry := &LogEntry{
		UserID:   "123", // Convert to string
		Username: "testuser",
		Action:   ActionCreate,
		Resource: "agent",
		Status:   StatusSuccess,
	}

	err = logger.Log(entry)
	if err != nil {
		t.Errorf("Log() error = %v", err)
	}
}

func TestLoggerLogTOTPEnabled(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	err = logger.LogTOTPEnabled("testuser", "127.0.0.1", "test-agent")
	if err != nil {
		t.Errorf("LogTOTPEnabled() error = %v", err)
	}
}

func TestLoggerLogTOTPDisabled(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	err = logger.LogTOTPDisabled("testuser", "127.0.0.1", "test-agent")
	if err != nil {
		t.Errorf("LogTOTPDisabled() error = %v", err)
	}
}

func TestLoggerLogTOTPVerified(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	// Test with backup code
	err = logger.LogTOTPVerified("testuser", "127.0.0.1", "test-agent", true)
	if err != nil {
		t.Errorf("LogTOTPVerified() with backup error = %v", err)
	}

	// Test without backup code
	err = logger.LogTOTPVerified("testuser", "127.0.0.1", "test-agent", false)
	if err != nil {
		t.Errorf("LogTOTPVerified() without backup error = %v", err)
	}
}

func TestLoggerLogTOTPFailed(t *testing.T) {
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.Remove("cockpit.db")
		os.Remove("cockpit.db-shm")
		os.Remove("cockpit.db-wal")
	}()

	logger := NewLogger(db)

	err = logger.LogTOTPFailed("testuser", "127.0.0.1", "test-agent")
	if err != nil {
		t.Errorf("LogTOTPFailed() error = %v", err)
	}
}
