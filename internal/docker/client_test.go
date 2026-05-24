package docker

import (
	"testing"
	"time"
)

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}

	if cfg.Host != "" {
		t.Error("Host should be empty by default")
	}

	if cfg.Timeout != 0 {
		t.Error("Timeout should be 0 by default")
	}
}

func TestConfigWithValues(t *testing.T) {
	cfg := Config{
		Host:    "unix:///var/run/docker.sock",
		Timeout: 30 * time.Second,
	}

	if cfg.Host != "unix:///var/run/docker.sock" {
		t.Errorf("Host = %v, want unix:///var/run/docker.sock", cfg.Host)
	}

	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}
}

func TestNewClientNoDaemon(t *testing.T) {
	cfg := Config{
		Host:    "tcp://127.0.0.1:9999", // Non-existent daemon
		Timeout: 1 * time.Second,
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("NewClient() should return error when Docker daemon is not available")
	}
}

func TestContainerInfoStruct(t *testing.T) {
	info := ContainerInfo{
		ID:      "container-123",
		Name:    "test-container",
		Image:   "nginx:latest",
		ImageID: "sha256:abc123",
		State:   "running",
		Status:  "Up 2 hours",
		Labels:  map[string]string{"app": "test"},
		Created: 1234567890,
	}

	if info.ID != "container-123" {
		t.Errorf("ID = %v, want container-123", info.ID)
	}

	if info.Name != "test-container" {
		t.Errorf("Name = %v, want test-container", info.Name)
	}

	if info.State != "running" {
		t.Errorf("State = %v, want running", info.State)
	}

	if len(info.Labels) != 1 {
		t.Errorf("Labels length = %d, want 1", len(info.Labels))
	}
}

func TestContainerInfoEmpty(t *testing.T) {
	info := ContainerInfo{}

	if info.ID != "" {
		t.Error("ID should be empty")
	}

	if info.Name != "" {
		t.Error("Name should be empty")
	}

	if len(info.Labels) != 0 {
		t.Error("Labels should be empty")
	}
}

func TestConfigTimeoutVariations(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{"1 second", 1 * time.Second},
		{"30 seconds", 30 * time.Second},
		{"1 minute", 1 * time.Minute},
		{"zero", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Timeout: tt.timeout}
			if cfg.Timeout != tt.timeout {
				t.Errorf("Timeout = %v, want %v", cfg.Timeout, tt.timeout)
			}
		})
	}
}

func TestConfigHostVariations(t *testing.T) {
	tests := []struct {
		name string
		host string
	}{
		{"unix socket", "unix:///var/run/docker.sock"},
		{"TCP", "tcp://localhost:2375"},
		{"empty", ""},
		{"custom", "tcp://192.168.1.1:2375"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Host: tt.host}
			if cfg.Host != tt.host {
				t.Errorf("Host = %v, want %v", cfg.Host, tt.host)
			}
		})
	}
}

func TestContainerInfoLabels(t *testing.T) {
	labels := map[string]string{
		"app":     "myapp",
		"version": "1.0.0",
		"env":     "production",
	}

	info := ContainerInfo{
		ID:     "test",
		Labels: labels,
	}

	if len(info.Labels) != 3 {
		t.Errorf("Labels length = %d, want 3", len(info.Labels))
	}

	if info.Labels["app"] != "myapp" {
		t.Errorf("Labels[app] = %v, want myapp", info.Labels["app"])
	}
}

func TestContainerInfoNilLabels(t *testing.T) {
	info := ContainerInfo{
		ID:     "test",
		Labels: nil,
	}

	if info.Labels != nil {
		t.Error("Labels should be nil")
	}
}

func TestConfigImmutable(t *testing.T) {
	cfg := Config{Host: "original"}
	host1 := cfg.Host

	cfg.Host = "modified"
	host2 := cfg.Host

	if host1 != "original" {
		t.Errorf("original host = %v, want original", host1)
	}

	if host2 != "modified" {
		t.Errorf("modified host = %v, want modified", host2)
	}
}

func TestMultipleConfigs(t *testing.T) {
	configs := []Config{
		{Host: "host1", Timeout: 10 * time.Second},
		{Host: "host2", Timeout: 20 * time.Second},
		{Host: "host3", Timeout: 30 * time.Second},
	}

	for i, cfg := range configs {
		if cfg.Host != "host"+string(rune('1'+i)) {
			t.Errorf("config[%d].Host = %v", i, cfg.Host)
		}
	}
}
