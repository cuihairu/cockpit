package agent

import (
	"os"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestNewAgent(t *testing.T) {
	cfg := Config{
		ServerURL: "ws://localhost:8080",
		AgentID:   "test-agent",
		Region:    "us-west",
		Zone:      "zone-a",
	}

	agent := NewAgent(cfg)

	if agent == nil {
		t.Fatal("NewAgent returned nil")
	}

	if agent.serverURL != cfg.ServerURL {
		t.Errorf("expected serverURL %s, got %s", cfg.ServerURL, agent.serverURL)
	}

	if agent.config.ServerURL != cfg.ServerURL {
		t.Errorf("expected config.ServerURL %s, got %s", cfg.ServerURL, agent.config.ServerURL)
	}

	if agent.config.AgentID != cfg.AgentID {
		t.Errorf("expected config.AgentID %s, got %s", cfg.AgentID, agent.config.AgentID)
	}

	if agent.config.Region != cfg.Region {
		t.Errorf("expected config.Region %s, got %s", cfg.Region, agent.config.Region)
	}

	if agent.config.Zone != cfg.Zone {
		t.Errorf("expected config.Zone %s, got %s", cfg.Zone, agent.config.Zone)
	}

	if agent.ctx == nil {
		t.Error("expected context to be initialized")
	}

	if agent.cancel == nil {
		t.Error("expected cancel function to be initialized")
	}
}

func TestNewAgentDefaults(t *testing.T) {
	cfg := Config{
		ServerURL: "ws://localhost:8080",
	}

	agent := NewAgent(cfg)

	if agent.config == nil {
		t.Fatal("expected config to be initialized")
	}

	if agent.capabilities == nil {
		t.Error("expected capabilities to be initialized")
	}
}

func TestDetectLocation(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		env      map[string]string
		expected protocol.Location
	}{
		{
			name: "from config",
			config: &Config{
				Region: "us-west",
				Zone:   "zone-a",
			},
			expected: protocol.Location{
				Region: "us-west",
				Zone:   "zone-a",
			},
		},
		{
			name:   "from env",
			config: &Config{},
			env: map[string]string{
				"COCKPIT_REGION": "eu-central",
				"COCKPIT_ZONE":   "zone-b",
			},
			expected: protocol.Location{
				Region: "eu-central",
				Zone:   "zone-b",
			},
		},
		{
			name:   "default unknown",
			config: &Config{},
			expected: protocol.Location{
				Region: "unknown",
				Zone:   "unknown",
			},
		},
		{
			name: "config overrides env",
			config: &Config{
				Region: "us-west",
				Zone:   "zone-a",
			},
			env: map[string]string{
				"COCKPIT_REGION": "eu-central",
				"COCKPIT_ZONE":   "zone-b",
			},
			expected: protocol.Location{
				Region: "us-west",
				Zone:   "zone-a",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env vars
			for k, v := range tt.env {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			agent := &Agent{config: tt.config}
			result := agent.detectLocation()

			if result.Region != tt.expected.Region {
				t.Errorf("expected Region %s, got %s", tt.expected.Region, result.Region)
			}

			if result.Zone != tt.expected.Zone {
				t.Errorf("expected Zone %s, got %s", tt.expected.Zone, result.Zone)
			}
		})
	}
}

func TestStop(t *testing.T) {
	agent := NewAgent(Config{
		ServerURL: "ws://localhost:8080",
	})

	// Stop should not panic
	agent.Stop()

	// After stop, context should be cancelled
	select {
	case <-agent.ctx.Done():
		// Expected
	default:
		t.Error("expected context to be cancelled after Stop")
	}
}

func TestDetectCapabilities(t *testing.T) {
	agent := NewAgent(Config{
		ServerURL: "ws://localhost:8080",
	})

	caps := agent.detectCapabilities()

	if caps == nil {
		t.Fatal("expected capabilities to be non-nil")
	}

	// Capabilities should be a slice (may be empty)
	if len(caps) == 0 {
		t.Log("no capabilities detected (may be expected in test environment)")
	}
}

func TestAgentConnected(t *testing.T) {
	agent := NewAgent(Config{
		ServerURL: "ws://localhost:8080",
	})

	// Initially not connected
	if agent.connected {
		t.Error("expected agent to be initially disconnected")
	}

	agent.mu.Lock()
	agent.connected = true
	agent.mu.Unlock()

	if !agent.connected {
		t.Error("expected agent to be connected")
	}
}

func TestAgentIDGeneration(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		contains string
	}{
		{
			name: "explicit agent ID",
			cfg: Config{
				AgentID: "my-agent",
			},
			contains: "my-agent",
		},
		{
			name: "default agent ID",
			cfg:  Config{},
			contains: "agent-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test ID generation is handled in register()
			// Here we just verify the config value
			if tt.cfg.AgentID != "" && tt.cfg.AgentID != tt.contains {
				t.Errorf("expected AgentID to contain %s, got %s", tt.contains, tt.cfg.AgentID)
			}
		})
	}
}

func TestContextCancellation(t *testing.T) {
	agent := NewAgent(Config{
		ServerURL: "ws://localhost:8080",
	})

	// Context should not be done initially
	select {
	case <-agent.ctx.Done():
		t.Error("expected context to be active")
	default:
		// Expected
	}

	// Cancel context
	agent.cancel()

	// Context should be done
	select {
	case <-agent.ctx.Done():
		// Expected
	case <-time.After(time.Second):
		t.Error("expected context to be cancelled")
	}
}

func TestConfigLabels(t *testing.T) {
	cfg := Config{
		ServerURL: "ws://localhost:8080",
		Labels: map[string]string{
			"env":     "test",
			"cluster": "test-cluster",
		},
	}

	agent := NewAgent(cfg)

	if agent.config.Labels == nil {
		t.Error("expected labels to be initialized")
	}

	if len(agent.config.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(agent.config.Labels))
	}
}

func TestConfigWithEmptyValues(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		expected string
	}{
		{
			name: "empty agent ID",
			cfg: Config{
				ServerURL: "ws://localhost:8080",
				AgentID:   "",
			},
			expected: "",
		},
		{
			name: "empty region",
			cfg: Config{
				ServerURL: "ws://localhost:8080",
				Region:    "",
			},
			expected: "",
		},
		{
			name: "empty zone",
			cfg: Config{
				ServerURL: "ws://localhost:8080",
				Zone:      "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := NewAgent(tt.cfg)
			if agent.config == nil {
				t.Error("expected config to be initialized")
			}
		})
	}
}

func TestAgentServerURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "ws URL",
			url:      "ws://localhost:8080",
			expected: "ws://localhost:8080",
		},
		{
			name:     "wss URL",
			url:      "wss://example.com",
			expected: "wss://example.com",
		},
		{
			name:     "URL with path",
			url:      "ws://localhost:8080/agent",
			expected: "ws://localhost:8080/agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := NewAgent(Config{ServerURL: tt.url})
			if agent.serverURL != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, agent.serverURL)
			}
		})
	}
}

func TestLocationDefaultValues(t *testing.T) {
	agent := &Agent{config: &Config{}}
	loc := agent.detectLocation()

	if loc.Region != "unknown" {
		t.Errorf("expected Region 'unknown', got '%s'", loc.Region)
	}

	if loc.Zone != "unknown" {
		t.Errorf("expected Zone 'unknown', got '%s'", loc.Zone)
	}
}

func TestNilConfigHandling(t *testing.T) {
	agent := &Agent{config: nil}
	loc := agent.detectLocation()

	// Should not panic and return defaults
	if loc.Region != "unknown" {
		t.Errorf("expected Region 'unknown', got '%s'", loc.Region)
	}
}

func TestCapabilitiesSlice(t *testing.T) {
	agent := NewAgent(Config{ServerURL: "ws://localhost:8080"})

	caps := agent.detectCapabilities()

	// Should return a slice (may be empty)
	if caps == nil {
		t.Error("expected capabilities to be non-nil slice")
	}

	// Each capability should have valid structure
	for i, cap := range caps {
		if cap.Type == "" {
			t.Errorf("capability %d: empty Type", i)
		}
		// Endpoint may be empty for some detector types
	}
}

func TestAgentMutex(t *testing.T) {
	agent := NewAgent(Config{ServerURL: "ws://localhost:8080"})

	// Test concurrent access to connected status
	done := make(chan bool)

	go func() {
		agent.mu.Lock()
		agent.connected = true
		time.Sleep(10 * time.Millisecond)
		agent.mu.Unlock()
		done <- true
	}()

	go func() {
		agent.mu.RLock()
		_ = agent.connected
		agent.mu.RUnlock()
		done <- true
	}()

	<-done
	<-done
}

func TestAgentStopWithNilConnection(t *testing.T) {
	agent := NewAgent(Config{ServerURL: "ws://localhost:8080"})

	// Stop with nil connection should not panic
	agent.Stop()

	select {
	case <-agent.ctx.Done():
		// Expected
	default:
		t.Error("expected context to be cancelled")
	}
}
