package audit

import (
	"testing"
)

func TestConstants(t *testing.T) {
	// Test action constants
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
		if action == "" {
			t.Errorf("Action constant should not be empty")
		}
	}

	// Test status constants
	if StatusSuccess == "" {
		t.Error("StatusSuccess should not be empty")
	}
	if StatusFailure == "" {
		t.Error("StatusFailure should not be empty")
	}
}

func TestLogEntry(t *testing.T) {
	entry := &LogEntry{
		UserID:     "1",
		Username:   "testuser",
		Action:     ActionLogin,
		Resource:   "user",
		ResourceID: "1",
		Details:    map[string]interface{}{"key": "value"},
		IP:         "192.168.1.1",
		UserAgent:  "test-agent",
		Status:     StatusSuccess,
	}

	if entry.Username != "testuser" {
		t.Errorf("Username = %v, want testuser", entry.Username)
	}

	if entry.Action != ActionLogin {
		t.Errorf("Action = %v, want %v", entry.Action, ActionLogin)
	}

	if entry.Status != StatusSuccess {
		t.Errorf("Status = %v, want %v", entry.Status, StatusSuccess)
	}

	if entry.Details == nil {
		t.Error("Details should not be nil")
	}
}

func TestLogEntryWithNilDetails(t *testing.T) {
	entry := &LogEntry{
		Username: "testuser",
		Action:   ActionLogout,
		Resource: "user",
		Status:   StatusSuccess,
		// Details is nil
	}

	if entry.Details != nil {
		t.Errorf("Details = %v, want nil", entry.Details)
	}
}

func TestLogEntryAllActions(t *testing.T) {
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
			Username: "testuser",
			Action:   action,
			Resource: "test",
			Status:   StatusSuccess,
		}

		if entry.Action != action {
			t.Errorf("Action = %v, want %v", entry.Action, action)
		}
	}
}

func TestLogEntryAllStatuses(t *testing.T) {
	statuses := []string{StatusSuccess, StatusFailure}

	for _, status := range statuses {
		entry := &LogEntry{
			Username: "testuser",
			Action:   ActionView,
			Resource: "test",
			Status:   status,
		}

		if entry.Status != status {
			t.Errorf("Status = %v, want %v", entry.Status, status)
		}
	}
}

func TestLogEntryDetailsTypes(t *testing.T) {
	tests := []struct {
		name    string
		details interface{}
	}{
		{
			name:    "string details",
			details: "test details",
		},
		{
			name:    "map details",
			details: map[string]interface{}{"key": "value"},
		},
		{
			name:    "slice details",
			details: []string{"item1", "item2"},
		},
		{
			name:    "number details",
			details: 123,
		},
		{
			name:    "bool details",
			details: true,
		},
		{
			name:    "nil details",
			details: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &LogEntry{
				Username: "testuser",
				Action:   ActionCreate,
				Resource: "test",
				Details:  tt.details,
				Status:   StatusSuccess,
			}

			// Check Details is set (but don't compare directly for uncomparable types)
			if tt.details == nil {
				if entry.Details != nil {
					t.Errorf("Details = %v, want nil", entry.Details)
				}
			} else {
				if entry.Details == nil {
					t.Error("Details should not be nil")
				}
			}
		})
	}
}

func TestLogEntryWithUserID(t *testing.T) {
	tests := []struct {
		name   string
		userID string
	}{
		{"empty user ID", ""},
		{"numeric user ID", "123"},
		{"UUID user ID", "550e8400-e29b-41d4-a716-446655440000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &LogEntry{
				UserID:   tt.userID,
				Username: "testuser",
				Action:   ActionView,
				Resource: "test",
				Status:   StatusSuccess,
			}

			if entry.UserID != tt.userID {
				t.Errorf("UserID = %v, want %v", entry.UserID, tt.userID)
			}
		})
	}
}

func TestLogEntryWithResourceID(t *testing.T) {
	tests := []struct {
		name       string
		resourceID string
	}{
		{"empty resource ID", ""},
		{"numeric resource ID", "123"},
		{"string resource ID", "abc-123"},
		{"UUID resource ID", "550e8400-e29b-41d4-a716-446655440000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &LogEntry{
				Username:   "testuser",
				Action:     ActionUpdate,
				Resource:   "test",
				ResourceID: tt.resourceID,
				Status:     StatusSuccess,
			}

			if entry.ResourceID != tt.resourceID {
				t.Errorf("ResourceID = %v, want %v", entry.ResourceID, tt.resourceID)
			}
		})
	}
}

func TestLogEntryWithIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{"IPv4", "192.168.1.1"},
		{"IPv6", "2001:db8::1"},
		{"localhost", "127.0.0.1"},
		{"empty IP", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &LogEntry{
				Username: "testuser",
				Action:   ActionDelete,
				Resource: "test",
				IP:       tt.ip,
				Status:   StatusSuccess,
			}

			if entry.IP != tt.ip {
				t.Errorf("IP = %v, want %v", entry.IP, tt.ip)
			}
		})
	}
}

func TestLogEntryWithUserAgent(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
	}{
		{"Chrome", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"},
		{"Firefox", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0"},
		{"Safari", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15"},
		{"empty user agent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &LogEntry{
				Username:  "testuser",
				Action:    ActionExport,
				Resource:  "data",
				UserAgent: tt.userAgent,
				Status:    StatusSuccess,
			}

			if entry.UserAgent != tt.userAgent {
				t.Errorf("UserAgent = %v, want %v", entry.UserAgent, tt.userAgent)
			}
		})
	}
}

func TestLogEntryComplete(t *testing.T) {
	// Test a complete log entry with all fields set
	entry := &LogEntry{
		UserID:     "42",
		Username:   "admin",
		Action:     ActionUpdate,
		Resource:   "agent",
		ResourceID: "agent-123",
		Details: map[string]interface{}{
			"changes": []string{"status", "config"},
			"old":     map[string]interface{}{"status": "offline"},
			"new":     map[string]interface{}{"status": "online"},
		},
		IP:        "10.0.0.1",
		UserAgent: "TestClient/1.0",
		Status:    StatusSuccess,
	}

	if entry.UserID != "42" {
		t.Errorf("UserID = %v, want 42", entry.UserID)
	}

	if entry.Action != ActionUpdate {
		t.Errorf("Action = %v, want %v", entry.Action, ActionUpdate)
	}

	if entry.Resource != "agent" {
		t.Errorf("Resource = %v, want agent", entry.Resource)
	}

	if entry.ResourceID != "agent-123" {
		t.Errorf("ResourceID = %v, want agent-123", entry.ResourceID)
	}

	if entry.Status != StatusSuccess {
		t.Errorf("Status = %v, want %v", entry.Status, StatusSuccess)
	}

	details, ok := entry.Details.(map[string]interface{})
	if !ok {
		t.Fatal("Details should be a map")
	}

	if details["changes"] == nil {
		t.Error("Details.changes should not be nil")
	}
}
