package server

import (
	"testing"
	"time"
)

func TestNewTicketManager(t *testing.T) {
	tm := NewTicketManager()
	if tm == nil {
		t.Fatal("NewTicketManager() should not return nil")
	}

	if tm.tickets == nil {
		t.Error("tickets map should be initialized")
	}
}

func TestGenerateTicket(t *testing.T) {
	tm := NewTicketManager()

	params := map[string]string{
		"agent_id": "test-agent",
		"host":     "192.168.1.1",
		"port":     "3389",
	}

	ticket, err := tm.GenerateTicket("user-1", "testuser", params)
	if err != nil {
		t.Errorf("GenerateTicket() error = %v", err)
	}

	if ticket == nil {
		t.Fatal("GenerateTicket() should not return nil ticket")
	}

	if ticket.ID == "" {
		t.Error("Ticket ID should not be empty")
	}

	if ticket.UserID != "user-1" {
		t.Errorf("UserID = %v, want user-1", ticket.UserID)
	}

	if ticket.Username != "testuser" {
		t.Errorf("Username = %v, want testuser", ticket.Username)
	}

	if ticket.Consumed {
		t.Error("New ticket should not be consumed")
	}

	if time.Now().Add(4*time.Minute).After(ticket.ExpiresAt) {
		t.Error("Ticket should expire in about 5 minutes")
	}
}

func TestGenerateTicketWithEmptyParams(t *testing.T) {
	tm := NewTicketManager()

	ticket, err := tm.GenerateTicket("user-1", "testuser", nil)
	if err != nil {
		t.Errorf("GenerateTicket() with nil params error = %v", err)
	}

	if ticket == nil {
		t.Fatal("GenerateTicket() should not return nil ticket")
	}

	// Params should be nil when nil is passed (no automatic initialization)
	if ticket.Params != nil {
		t.Error("Params should be nil when nil is passed")
	}
}

func TestValidateTicketValid(t *testing.T) {
	tm := NewTicketManager()

	params := map[string]string{"host": "example.com"}
	ticket, _ := tm.GenerateTicket("user-1", "testuser", params)

	// Validate the ticket
	validTicket, valid := tm.ValidateTicket(ticket.ID)
	if !valid {
		t.Error("ValidateTicket() should return true for valid ticket")
	}

	if validTicket == nil {
		t.Fatal("ValidateTicket() should not return nil for valid ticket")
	}

	if validTicket.Username != "testuser" {
		t.Errorf("Username = %v, want testuser", validTicket.Username)
	}

	if validTicket.Params["host"] != "example.com" {
		t.Errorf("Params[host] = %v, want example.com", validTicket.Params["host"])
	}
}

func TestValidateTicketNonexistent(t *testing.T) {
	tm := NewTicketManager()

	_, valid := tm.ValidateTicket("nonexistent-ticket")
	if valid {
		t.Error("ValidateTicket() should return false for nonexistent ticket")
	}
}

func TestValidateTicketConsumed(t *testing.T) {
	tm := NewTicketManager()

	ticket, _ := tm.GenerateTicket("user-1", "testuser", nil)

	// First validation - should succeed
	_, valid := tm.ValidateTicket(ticket.ID)
	if !valid {
		t.Error("First validation should succeed")
	}

	// Second validation - should fail (already consumed)
	_, valid = tm.ValidateTicket(ticket.ID)
	if valid {
		t.Error("Second validation should fail (ticket already consumed)")
	}
}

func TestValidateTicketExpired(t *testing.T) {
	tm := NewTicketManager()

	params := map[string]string{"host": "example.com"}
	ticket, _ := tm.GenerateTicket("user-1", "testuser", params)

	// Manually expire the ticket
	ticket.mu.Lock()
	ticket.ExpiresAt = time.Now().Add(-1 * time.Minute)
	ticket.mu.Unlock()

	_, valid := tm.ValidateTicket(ticket.ID)
	if valid {
		t.Error("ValidateTicket() should return false for expired ticket")
	}
}

func TestTicketConcurrency(t *testing.T) {
	tm := NewTicketManager()

	done := make(chan bool)

	// Generate multiple tickets concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			params := map[string]string{"id": string(rune(id))}
			_, err := tm.GenerateTicket("user-1", "testuser", params)
			if err != nil {
				t.Errorf("Concurrent GenerateTicket() %d error = %v", id, err)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestTicketParams(t *testing.T) {
	tm := NewTicketManager()

	params := map[string]string{
		"agent_id":  "agent-123",
		"host":      "192.168.1.100",
		"port":      "22",
		"username":  "admin",
		"password":  "secret",
		"domain":    "EXAMPLE",
		"width":     "1920",
		"height":    "1080",
		"extra_key": "extra_value",
	}

	ticket, _ := tm.GenerateTicket("user-1", "testuser", params)

	// Verify all params are preserved
	validTicket, _ := tm.ValidateTicket(ticket.ID)
	if len(validTicket.Params) != len(params) {
		t.Errorf("Params length = %d, want %d", len(validTicket.Params), len(params))
	}

	for key, expectedValue := range params {
		if actualValue := validTicket.Params[key]; actualValue != expectedValue {
			t.Errorf("Params[%s] = %v, want %v", key, actualValue, expectedValue)
		}
	}
}

func TestTicketIDUniqueness(t *testing.T) {
	tm := NewTicketManager()

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		ticket, _ := tm.GenerateTicket("user-1", "testuser", nil)
		if ids[ticket.ID] {
			t.Errorf("Duplicate ticket ID generated: %s", ticket.ID)
		}
		ids[ticket.ID] = true
	}

	if len(ids) != 100 {
		t.Errorf("Generated %d unique IDs, want 100", len(ids))
	}
}

func TestTicketExpirationTime(t *testing.T) {
	tm := NewTicketManager()

	startTime := time.Now()
	ticket, _ := tm.GenerateTicket("user-1", "testuser", nil)

	// Check expiration is approximately 5 minutes from now
	diff := ticket.ExpiresAt.Sub(startTime)
	if diff < 4*time.Minute || diff > 6*time.Minute {
		t.Errorf("Expiration time = %v, want approximately 5 minutes from generation time", diff)
	}
}
