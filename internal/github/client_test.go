package github

import (
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"default config", Config{}},
		{"with token", Config{Token: "test-token"}},
		{"with base URL", Config{BaseURL: "https://api.github.example.com"}},
		{"with timeout", Config{Timeout: 10 * time.Second}},
		{"full config", Config{
			Token:   "test-token",
			BaseURL: "https://api.github.example.com",
			Timeout: 60 * time.Second,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(tt.cfg)
			if c == nil {
				t.Error("NewClient() should not return nil")
			}

			expectedBaseURL := tt.cfg.BaseURL
			if expectedBaseURL == "" {
				expectedBaseURL = "https://api.github.com"
			}
			if c.baseURL != expectedBaseURL {
				t.Errorf("baseURL = %v, want %v", c.baseURL, expectedBaseURL)
			}

			expectedTimeout := tt.cfg.Timeout
			if expectedTimeout == 0 {
				expectedTimeout = 30 * time.Second
			}
			if c.timeout != expectedTimeout {
				t.Errorf("timeout = %v, want %v", c.timeout, expectedTimeout)
			}

			if c.token != tt.cfg.Token {
				t.Errorf("token = %v, want %v", c.token, tt.cfg.Token)
			}

			if c.client == nil {
				t.Error("HTTP client should not be nil")
			}
		})
	}
}

func TestNewClientDefaults(t *testing.T) {
	c := NewClient(Config{})

	if c.baseURL != "https://api.github.com" {
		t.Errorf("default baseURL = %v, want https://api.github.com", c.baseURL)
	}

	if c.timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", c.timeout)
	}

	if c.token != "" {
		t.Error("default token should be empty")
	}
}

func TestClientFields(t *testing.T) {
	cfg := Config{
		Token:   "test-token-123",
		BaseURL: "https://api.example.com",
		Timeout: 45 * time.Second,
	}

	c := NewClient(cfg)

	if c.token != "test-token-123" {
		t.Errorf("token = %v, want test-token-123", c.token)
	}

	if c.baseURL != "https://api.example.com" {
		t.Errorf("baseURL = %v, want https://api.example.com", c.baseURL)
	}

	if c.timeout != 45*time.Second {
		t.Errorf("timeout = %v, want 45s", c.timeout)
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}

	if cfg.Token != "" {
		t.Error("Token should be empty by default")
	}

	if cfg.BaseURL != "" {
		t.Error("BaseURL should be empty by default")
	}

	if cfg.Timeout != 0 {
		t.Error("Timeout should be 0 by default")
	}
}

func TestClientHTTPClient(t *testing.T) {
	c := NewClient(Config{Timeout: 10 * time.Second})

	if c.client == nil {
		t.Error("client should not be nil")
	}

	if c.client.Timeout != 10*time.Second {
		t.Errorf("HTTP client timeout = %v, want 10s", c.client.Timeout)
	}
}

func TestMultipleClients(t *testing.T) {
	cfg := Config{Token: "test"}

	for i := 0; i < 5; i++ {
		c := NewClient(cfg)
		if c == nil {
			t.Errorf("NewClient() iteration %d returned nil", i)
		}
		if c.token != "test" {
			t.Errorf("token = %v, want test", c.token)
		}
	}
}

func TestClientWithEmptyToken(t *testing.T) {
	c := NewClient(Config{Token: ""})

	if c.token != "" {
		t.Error("token should be empty")
	}

	// Client should still be valid
	if c.client == nil {
		t.Error("HTTP client should not be nil even without token")
	}
}

func TestClientTimeoutVariations(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		expected time.Duration
	}{
		{"1 second", 1 * time.Second, 1 * time.Second},
		{"30 seconds", 30 * time.Second, 30 * time.Second},
		{"1 minute", 1 * time.Minute, 1 * time.Minute},
		{"zero (defaults to 30s)", 0, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(Config{Timeout: tt.timeout})
			if c.timeout != tt.expected {
				t.Errorf("timeout = %v, want %v", c.timeout, tt.expected)
			}
		})
	}
}

func TestClientBaseURLVariations(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		expected string
	}{
		{"GitHub API", "", "https://api.github.com"},
		{"Custom URL", "https://api.example.com", "https://api.example.com"},
		{"Enterprise", "https://github.company.com/api/v3", "https://github.company.com/api/v3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(Config{BaseURL: tt.baseURL})
			if c.baseURL != tt.expected {
				t.Errorf("baseURL = %v, want %v", c.baseURL, tt.expected)
			}
		})
	}
}

func TestConcurrentClientCreation(t *testing.T) {
	done := make(chan *Client, 10)

	cfg := Config{Token: "test", Timeout: 10 * time.Second}

	for i := 0; i < 10; i++ {
		go func() {
			c := NewClient(cfg)
			done <- c
		}()
	}

	for i := 0; i < 10; i++ {
		c := <-done
		if c == nil {
			t.Error("NewClient() should not return nil")
		}
		if c.token != "test" {
			t.Errorf("token = %v, want test", c.token)
		}
	}
}

func TestClientImmutableConfig(t *testing.T) {
	cfg := Config{Token: "original"}
	c1 := NewClient(cfg)

	// Modify original config
	cfg.Token = "modified"

	// Create another client
	c2 := NewClient(cfg)

	// First client should not be affected
	if c1.token != "original" {
		t.Errorf("c1.token = %v, want original", c1.token)
	}

	// Second client should have new value
	if c2.token != "modified" {
		t.Errorf("c2.token = %v, want modified", c2.token)
	}
}
