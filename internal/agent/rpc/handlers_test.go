package rpc

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestNewHandler(t *testing.T) {
	h := NewHandler()

	if h == nil {
		t.Fatal("NewHandler returned nil")
	}

	if h.providers == nil {
		t.Error("expected providers map to be initialized")
	}
}

func TestSystemProvider(t *testing.T) {
	p := NewSystemProvider()

	if p.Type() != "system" {
		t.Errorf("expected type 'system', got '%s'", p.Type())
	}
}

func TestSystemProviderStatus(t *testing.T) {
	p := NewSystemProvider()

	result, err := p.Status(nil)
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("Status should return a map")
	}

	if resultMap["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%v'", resultMap["status"])
	}

	if _, ok := resultMap["uptime"]; !ok {
		t.Error("expected uptime to be present")
	}

	if _, ok := resultMap["go_version"]; !ok {
		t.Error("expected go_version to be present")
	}
}

func TestSystemProviderInfo(t *testing.T) {
	p := NewSystemProvider()

	result, err := p.Info(nil)
	if err != nil {
		t.Fatalf("Info returned error: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("Info should return a map")
	}

	if _, ok := resultMap["capabilities"]; !ok {
		t.Error("expected capabilities to be present")
	}

	if _, ok := resultMap["version"]; !ok {
		t.Error("expected version to be present")
	}
}

func TestSystemProviderVersion(t *testing.T) {
	p := NewSystemProvider()

	result, err := p.Call("version", nil)
	if err != nil {
		t.Fatalf("version returned error: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("version should return a map")
	}

	if _, ok := resultMap["version"]; !ok {
		t.Error("expected version to be present")
	}

	if _, ok := resultMap["build"]; !ok {
		t.Error("expected build to be present")
	}
}

func TestSystemProviderUnknownAction(t *testing.T) {
	p := NewSystemProvider()

	_, err := p.Call("unknown", nil)
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestSplitMethod(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple",
			input:    "test",
			expected: []string{"test"},
		},
		{
			name:     "with dots",
			input:    "docker.containers.list",
			expected: []string{"docker", "containers", "list"},
		},
		{
			name:     "consecutive dots",
			input:    "a..b",
			expected: []string{"a", "b"},
		},
		{
			name:     "leading dot",
			input:    ".test",
			expected: []string{"test"},
		},
		{
			name:     "trailing dot",
			input:    "test.",
			expected: []string{"test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitMethod(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d parts, got %d", len(tt.expected), len(result))
				return
			}

			for i, part := range result {
				if part != tt.expected[i] {
					t.Errorf("part %d: expected %s, got %s", i, tt.expected[i], part)
				}
			}
		})
	}
}

func TestJoinMethod(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "single",
			input:    []string{"test"},
			expected: "test",
		},
		{
			name:     "multiple",
			input:    []string{"docker", "containers", "list"},
			expected: "docker.containers.list",
		},
		{
			name:     "empty",
			input:    []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinMethod(tt.input)

			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestParseMethod(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedProv  string
		expectedAction string
		expectError   bool
	}{
		{
			name:          "simple action",
			input:         "status",
			expectedProv:  "system",
			expectedAction: "status",
		},
		{
			name:          "dotted action",
			input:         "docker.containers.list",
			expectedProv:  "docker",
			expectedAction: "containers.list",
		},
		{
			name:          "triple dotted",
			input:         "a.b.c",
			expectedProv:  "a",
			expectedAction: "b.c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov, action, err := parseMethod(tt.input)

			if tt.expectError && err == nil {
				t.Error("expected error")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if prov != tt.expectedProv {
				t.Errorf("expected provider %s, got %s", tt.expectedProv, prov)
			}

			if action != tt.expectedAction {
				t.Errorf("expected action %s, got %s", tt.expectedAction, action)
			}
		})
	}
}

func TestHandlerRegisterProvider(t *testing.T) {
	h := NewHandler()
	p := NewSystemProvider()

	h.RegisterProvider(p)

	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(h.providers))
	}

	if _, ok := h.providers["system"]; !ok {
		t.Error("expected system provider to be registered")
	}
}

func TestHandlerHandle(t *testing.T) {
	h := NewHandler()
	p := NewSystemProvider()
	h.RegisterProvider(p)

	tests := []struct {
		name        string
		method      string
		expectError bool
	}{
		{
			name:        "valid system call",
			method:      "status",
			expectError: false,
		},
		{
			name:        "unknown provider",
			method:      "unknown.status",
			expectError: true,
		},
		{
			name:        "unknown action",
			method:      "system.unknown",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
				"method": tt.method,
			})

			resp, err := h.Handle(msg)

			if tt.expectError && err == nil {
				t.Error("expected error")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && resp == nil {
				t.Error("expected response")
			}
		})
	}
}

func TestPVEProviderType(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	if p.Type() != "pve" {
		t.Errorf("expected type 'pve', got '%s'", p.Type())
	}
}

func TestDockerProviderType(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider (docker may not be running): %v", err)
	}

	if p.Type() != "docker" {
		t.Errorf("expected type 'docker', got '%s'", p.Type())
	}
}

func TestOpenWrtProviderType(t *testing.T) {
	p := NewOpenWrtProvider("192.168.1.1", 443, "root", "password")

	if p.Type() != "openwrt" {
		t.Errorf("expected type 'openwrt', got '%s'", p.Type())
	}
}

func TestPVEProviderGetNode(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	tests := []struct {
		name     string
		params   map[string]interface{}
		expected string
	}{
		{
			name:     "with node param",
			params:   map[string]interface{}{"node": "pve1"},
			expected: "pve1",
		},
		{
			name:     "without node param",
			params:   map[string]interface{}{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.getNode(tt.params)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}
