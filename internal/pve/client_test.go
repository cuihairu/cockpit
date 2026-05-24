package pve

import (
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"minimal config", Config{
			Endpoint:    "https://pve.example.com:8006",
			TokenID:     "test@pve!test",
			TokenSecret: "secret",
		}},
		{"with node", Config{
			Endpoint:    "https://pve.example.com:8006",
			TokenID:     "test@pve!test",
			TokenSecret: "secret",
			Node:        "pve1",
		}},
		{"with insecure TLS", Config{
			Endpoint:    "https://pve.example.com:8006",
			TokenID:     "test@pve!test",
			TokenSecret: "secret",
			InsecureTLS: true,
		}},
		{"trailing slash in endpoint", Config{
			Endpoint:    "https://pve.example.com:8006/",
			TokenID:     "test@pve!test",
			TokenSecret: "secret",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(tt.cfg)
			if c == nil {
				t.Error("NewClient() should not return nil")
			}

			expectedEndpoint := tt.cfg.Endpoint
			if expectedEndpoint != "" {
				expectedEndpoint = strings.TrimSuffix(expectedEndpoint, "/")
			}
			if c.endpoint != expectedEndpoint {
				t.Errorf("endpoint = %v, want %v", c.endpoint, expectedEndpoint)
			}

			if c.tokenID != tt.cfg.TokenID {
				t.Errorf("tokenID = %v, want %v", c.tokenID, tt.cfg.TokenID)
			}

			if c.tokenSecret != tt.cfg.TokenSecret {
				t.Errorf("tokenSecret = %v, want %v", c.tokenSecret, tt.cfg.TokenSecret)
			}

			if c.node != tt.cfg.Node {
				t.Errorf("node = %v, want %v", c.node, tt.cfg.Node)
			}

			if c.httpClient == nil {
				t.Error("httpClient should not be nil")
			}

			if c.httpClient.Timeout != 30*time.Second {
				t.Errorf("default timeout should be 30s, got %v", c.httpClient.Timeout)
			}
		})
	}
}

func TestNewClientTrimsSlash(t *testing.T) {
	cfg := Config{
		Endpoint: "https://pve.example.com:8006/",
	}

	c := NewClient(cfg)

	if c.endpoint != "https://pve.example.com:8006" {
		t.Errorf("endpoint should be trimmed, got %v", c.endpoint)
	}
}

func TestClientFields(t *testing.T) {
	cfg := Config{
		Endpoint:    "https://pve.example.com:8006",
		TokenID:     "root@pam!test",
		TokenSecret: "abc123def456",
		Node:        "node1",
		InsecureTLS: true,
	}

	c := NewClient(cfg)

	if c.endpoint != "https://pve.example.com:8006" {
		t.Errorf("endpoint = %v", c.endpoint)
	}

	if c.tokenID != "root@pam!test" {
		t.Errorf("tokenID = %v", c.tokenID)
	}

	if c.tokenSecret != "abc123def456" {
		t.Errorf("tokenSecret = %v", c.tokenSecret)
	}

	if c.node != "node1" {
		t.Errorf("node = %v", c.node)
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}

	if cfg.Endpoint != "" {
		t.Error("Endpoint should be empty by default")
	}

	if cfg.TokenID != "" {
		t.Error("TokenID should be empty by default")
	}

	if cfg.TokenSecret != "" {
		t.Error("TokenSecret should be empty by default")
	}

	if cfg.Node != "" {
		t.Error("Node should be empty by default")
	}

	if cfg.InsecureTLS != false {
		t.Error("InsecureTLS should be false by default")
	}
}

func TestHTTPClientConfiguration(t *testing.T) {
	cfg := Config{
		Endpoint:    "https://pve.example.com:8006",
		TokenID:     "test",
		TokenSecret: "secret",
		InsecureTLS: true,
	}

	c := NewClient(cfg)

	if c.httpClient == nil {
		t.Fatal("httpClient should not be nil")
	}

	if c.httpClient.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", c.httpClient.Timeout)
	}
}

func TestMultipleClients(t *testing.T) {
	cfg := Config{
		Endpoint:    "https://pve.example.com:8006",
		TokenID:     "test",
		TokenSecret: "secret",
	}

	for i := 0; i < 5; i++ {
		c := NewClient(cfg)
		if c == nil {
			t.Errorf("NewClient() iteration %d returned nil", i)
		}
	}
}

func TestClientWithEmptyEndpoint(t *testing.T) {
	cfg := Config{
		Endpoint:    "",
		TokenID:     "test",
		TokenSecret: "secret",
	}

	c := NewClient(cfg)

	if c.endpoint != "" {
		t.Error("endpoint should be empty")
	}

	// Client should still be created
	if c == nil {
		t.Error("NewClient() should not return nil even with empty endpoint")
	}
}

func TestClientWithEmptyCredentials(t *testing.T) {
	cfg := Config{
		Endpoint: "https://pve.example.com:8006",
	}

	c := NewClient(cfg)

	if c.tokenID != "" {
		t.Error("tokenID should be empty")
	}

	if c.tokenSecret != "" {
		t.Error("tokenSecret should be empty")
	}

	// Client should still be created
	if c == nil {
		t.Error("NewClient() should not return nil even with empty credentials")
	}
}

func TestConcurrentClientCreation(t *testing.T) {
	cfg := Config{
		Endpoint:    "https://pve.example.com:8006",
		TokenID:     "test",
		TokenSecret: "secret",
	}

	done := make(chan *Client, 10)

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
	}
}

func TestClientImmutableConfig(t *testing.T) {
	cfg := Config{
		Endpoint: "https://original.example.com",
	}

	c1 := NewClient(cfg)

	// Modify config
	cfg.Endpoint = "https://modified.example.com"

	c2 := NewClient(cfg)

	if c1.endpoint != "https://original.example.com" {
		t.Errorf("c1.endpoint = %v, want original", c1.endpoint)
	}

	if c2.endpoint != "https://modified.example.com" {
		t.Errorf("c2.endpoint = %v, want modified", c2.endpoint)
	}
}

func TestEndpointVariations(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://pve.example.com:8006", "https://pve.example.com:8006"},
		{"https://pve.example.com:8006/", "https://pve.example.com:8006"},
		// TrimSuffix only removes one trailing slash
		{"https://pve.example.com:8006//", "https://pve.example.com:8006/"},
		{"http://pve.local", "http://pve.local"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			c := NewClient(Config{Endpoint: tt.input})
			if c.endpoint != tt.expected {
				t.Errorf("endpoint = %v, want %v", c.endpoint, tt.expected)
			}
		})
	}
}

func TestNodeConfiguration(t *testing.T) {
	tests := []struct {
		name string
		node string
	}{
		{"node1", "node1"},
		{"pve-01", "pve-01"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(Config{
				Endpoint: "https://pve.example.com:8006",
				Node:     tt.node,
			})

			if c.node != tt.node {
				t.Errorf("node = %v, want %v", c.node, tt.node)
			}
		})
	}
}

func TestInsecureTLSConfiguration(t *testing.T) {
	secureClient := NewClient(Config{
		Endpoint:    "https://pve.example.com:8006",
		InsecureTLS: false,
	})

	insecureClient := NewClient(Config{
		Endpoint:    "https://pve.example.com:8006",
		InsecureTLS: true,
	})

	// Both should have HTTP clients
	if secureClient.httpClient == nil {
		t.Error("secure client should have HTTP client")
	}

	if insecureClient.httpClient == nil {
		t.Error("insecure client should have HTTP client")
	}
}

func TestTokenIDFormats(t *testing.T) {
	tokenIDs := []string{
		"root@pve!token",
		"user@realm!secret",
		"test!value",
	}

	for _, tokenID := range tokenIDs {
		c := NewClient(Config{
			Endpoint:    "https://pve.example.com:8006",
			TokenID:     tokenID,
			TokenSecret: "secret",
		})

		if c.tokenID != tokenID {
			t.Errorf("tokenID = %v, want %v", c.tokenID, tokenID)
		}
	}
}
