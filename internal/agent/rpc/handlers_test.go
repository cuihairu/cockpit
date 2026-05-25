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
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

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
		{
			name:     "with empty node param",
			params:   map[string]interface{}{"node": ""},
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

func TestPVEProviderUnknownAction(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.Call("unknown_action", nil)
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestPVEProviderListVMs(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.ListVMs(map[string]interface{}{"node": "pve1"})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderGetVM(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.GetVM(map[string]interface{}{"node": "pve1", "vmid": 100})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderGetVMInvalidID(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.GetVM(map[string]interface{}{"node": "pve1", "vmid": "invalid"})
	if err == nil {
		t.Error("expected error for invalid vmid")
	}
}

func TestPVEProviderStartVM(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.StartVM(map[string]interface{}{"node": "pve1", "vmid": 100})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderStopVM(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.StopVM(map[string]interface{}{"node": "pve1", "vmid": 100})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderRestartVM(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.RestartVM(map[string]interface{}{"node": "pve1", "vmid": 100})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderSuspendVM(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.SuspendVM(map[string]interface{}{"node": "pve1", "vmid": 100})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderResumeVM(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.ResumeVM(map[string]interface{}{"node": "pve1", "vmid": 100})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderListContainers(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.ListContainers(map[string]interface{}{"node": "pve1"})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderGetContainer(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.GetContainer(map[string]interface{}{"node": "pve1", "vmid": 100})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderStartContainer(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.StartContainer(map[string]interface{}{"node": "pve1", "vmid": 100})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderStopContainer(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.StopContainer(map[string]interface{}{"node": "pve1", "vmid": 100})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderRestartContainer(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.RestartContainer(map[string]interface{}{"node": "pve1", "vmid": 100})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderListNodes(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.ListNodes(nil)
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderListStorage(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.ListStorage(map[string]interface{}{"node": "pve1"})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderListSnapshots(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.ListSnapshots(map[string]interface{}{"node": "pve1", "vmid": 100})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderCreateSnapshot(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.CreateSnapshot(map[string]interface{}{
		"node": "pve1",
		"vmid": 100,
		"name": "test-snapshot",
	})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderDeleteSnapshot(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.DeleteSnapshot(map[string]interface{}{
		"node": "pve1",
		"vmid": 100,
		"name": "test-snapshot",
	})
	if err == nil {
		// May succeed if server is running
	}
}

func TestPVEProviderDeleteSnapshotMissingName(t *testing.T) {
	p := NewPVEProvider("http://localhost:8006", "token-id", "secret")

	_, err := p.DeleteSnapshot(map[string]interface{}{
		"node": "pve1",
		"vmid": 100,
	})
	if err == nil {
		t.Error("expected error for missing snapshot name")
	}
}

func TestOpenWrtProviderUnknownAction(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.Call("unknown_action", nil)
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestOpenWrtProviderGetSystemInfo(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.GetSystemInfo(nil)
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderListInterfaces(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.ListInterfaces(nil)
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderGetInterface(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.GetInterface(map[string]interface{}{"name": "lan"})
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderGetInterfaceMissingName(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.GetInterface(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing interface name")
	}
}

func TestOpenWrtProviderGetRoutes(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.GetRoutes(nil)
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderGetFirewallZones(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.GetFirewallZones(nil)
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderGetFirewallRules(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.GetFirewallRules(nil)
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderGetFirewallRedirects(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.GetFirewallRedirects(nil)
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderGetWirelessStatus(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.GetWirelessStatus(nil)
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderGetDHCPLoads(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.GetDHCPLoads(nil)
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderReadFile(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.ReadFile(map[string]interface{}{"path": "/etc/config/system"})
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderReadFileMissingPath(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.ReadFile(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing file path")
	}
}

func TestOpenWrtProviderWriteFile(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.WriteFile(map[string]interface{}{
		"path": "/tmp/test",
		"data": "test content",
	})
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderWriteFileMissingPath(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.WriteFile(map[string]interface{}{"data": "test"})
	if err == nil {
		t.Error("expected error for missing file path")
	}
}

func TestOpenWrtProviderWriteFileMissingData(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.WriteFile(map[string]interface{}{"path": "/tmp/test"})
	if err == nil {
		t.Error("expected error for missing file data")
	}
}

func TestOpenWrtProviderReboot(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.Reboot(nil)
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderGetLEDState(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.GetLEDState(map[string]interface{}{"name": "tp-link:blue:status"})
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderGetLEDStateMissingName(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.GetLEDState(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing LED name")
	}
}

func TestOpenWrtProviderSetLEDState(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.SetLEDState(map[string]interface{}{
		"name":  "tp-link:blue:status",
		"state": "on",
	})
	if err == nil {
		// May succeed if server is running
	}
}

func TestOpenWrtProviderSetLEDStateMissingName(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.SetLEDState(map[string]interface{}{"state": "on"})
	if err == nil {
		t.Error("expected error for missing LED name")
	}
}

func TestOpenWrtProviderSetLEDStateMissingState(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 443, "root", "password")

	_, err := p.SetLEDState(map[string]interface{}{"name": "tp-link:blue:status"})
	if err == nil {
		t.Error("expected error for missing LED state")
	}
}

func TestDockerProviderUnknownAction(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.Call("unknown_action", nil)
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestDockerProviderListContainers(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.ListContainers(map[string]interface{}{"all": true})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderGetContainer(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.GetContainer(map[string]interface{}{"id": "test-container"})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderGetContainerMissingID(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.GetContainer(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing container id")
	}
}

func TestDockerProviderStartContainer(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.StartContainer(map[string]interface{}{"id": "test-container"})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderStartContainerMissingID(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.StartContainer(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing container id")
	}
}

func TestDockerProviderStopContainer(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.StopContainer(map[string]interface{}{"id": "test-container"})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderStopContainerMissingID(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.StopContainer(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing container id")
	}
}

func TestDockerProviderRestartContainer(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.RestartContainer(map[string]interface{}{"id": "test-container"})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderRestartContainerMissingID(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.RestartContainer(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing container id")
	}
}

func TestDockerProviderRemoveContainer(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.RemoveContainer(map[string]interface{}{"id": "test-container"})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderRemoveContainerMissingID(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.RemoveContainer(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing container id")
	}
}

func TestDockerProviderPauseContainer(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.PauseContainer(map[string]interface{}{"id": "test-container"})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderPauseContainerMissingID(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.PauseContainer(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing container id")
	}
}

func TestDockerProviderUnpauseContainer(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.UnpauseContainer(map[string]interface{}{"id": "test-container"})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderUnpauseContainerMissingID(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.UnpauseContainer(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing container id")
	}
}

func TestDockerProviderGetLogs(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.GetLogs(map[string]interface{}{
		"id":     "test-container",
		"tail":   "100",
		"stdout": true,
	})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderGetLogsMissingID(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.GetLogs(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing container id")
	}
}

func TestDockerProviderGetStats(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.GetStats(map[string]interface{}{"id": "test-container"})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderGetStatsMissingID(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.GetStats(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing container id")
	}
}

func TestDockerProviderListImages(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.ListImages(map[string]interface{}{"all": true})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderRemoveImage(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.RemoveImage(map[string]interface{}{"id": "test-image"})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderRemoveImageMissingID(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.RemoveImage(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing image id")
	}
}

func TestDockerProviderPullImage(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.PullImage(map[string]interface{}{"ref": "nginx:latest"})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderPullImageMissingRef(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.PullImage(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing image ref")
	}
}

func TestDockerProviderListVolumes(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.ListVolumes(nil)
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderRemoveVolume(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.RemoveVolume(map[string]interface{}{"name": "test-volume"})
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderRemoveVolumeMissingName(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.RemoveVolume(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing volume name")
	}
}

func TestDockerProviderListNetworks(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.ListNetworks(nil)
	if err == nil {
		// May succeed if docker is running
	}
}

func TestDockerProviderGetSystemInfo(t *testing.T) {
	p, err := NewDockerProvider("unix:///var/run/docker.sock")
	if err != nil {
		t.Skipf("Cannot create Docker provider: %v", err)
	}

	_, err = p.GetSystemInfo(nil)
	if err == nil {
		// May succeed if docker is running
	}
}
