package protocol

import (
	"testing"
	"time"
)

func TestNewMessage(t *testing.T) {
	payload := map[string]interface{}{
		"key": "value",
		"num": 123,
	}

	msg := NewMessage(MessageTypeRegister, payload)

	if msg == nil {
		t.Fatal("NewMessage() should not return nil")
	}

	if msg.ID == "" {
		t.Error("NewMessage() should generate ID")
	}

	if msg.Type != MessageTypeRegister {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeRegister)
	}

	if msg.Timestamp == 0 {
		t.Error("NewMessage() should set timestamp")
	}

	if len(msg.Payload) != 2 {
		t.Errorf("Payload length = %d, want 2", len(msg.Payload))
	}
}

func TestNewMessageWithEmptyPayload(t *testing.T) {
	msg := NewMessage(MessageTypeHeartbeat, nil)

	// nil payload is stored as-is, not initialized
	if msg.Payload != nil {
		t.Error("NewMessage() with nil payload should have nil Payload field")
	}

	// Accessing nil map length is 0
	if len(msg.Payload) != 0 {
		t.Errorf("Payload length = %d, want 0", len(msg.Payload))
	}
}

func TestMessageTypes(t *testing.T) {
	types := []MessageType{
		MessageTypeRegister,
		MessageTypeHeartbeat,
		MessageTypeRPCResponse,
		MessageTypeProxyClose,
		MessageTypeProxyError,
		MessageTypeRPCRequest,
		MessageTypePing,
		MessageTypeProxyNew,
		MessageTypeError,
		MessageTypeProxyData,
	}

	for _, typ := range types {
		if typ == "" {
			t.Errorf("MessageType %v should not be empty", typ)
		}
	}
}

func TestLocation(t *testing.T) {
	tests := []struct {
		name   string
		loc    Location
		wantRegion string
		wantZone   string
	}{
		{
			name:   "full location",
			loc:    Location{Region: "us-west", Zone: "zone-a"},
			wantRegion: "us-west",
			wantZone:   "zone-a",
		},
		{
			name:   "empty location",
			loc:    Location{},
			wantRegion: "",
			wantZone:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.loc.Region != tt.wantRegion {
				t.Errorf("Region = %v, want %v", tt.loc.Region, tt.wantRegion)
			}
			if tt.loc.Zone != tt.wantZone {
				t.Errorf("Zone = %v, want %v", tt.loc.Zone, tt.wantZone)
			}
		})
	}
}

func TestCapability(t *testing.T) {
	cap := Capability{
		Type:     "docker",
		Endpoint: "unix:///var/run/docker.sock",
		Version:  "1.0",
		Metadata: map[string]interface{}{
			"api_version": "1.42",
		},
	}

	if cap.Type != "docker" {
		t.Errorf("Type = %v, want docker", cap.Type)
	}

	if cap.Endpoint != "unix:///var/run/docker.sock" {
		t.Errorf("Endpoint = %v, want unix:///var/run/docker.sock", cap.Endpoint)
	}

	if cap.Metadata["api_version"] != "1.42" {
		t.Error("Metadata not preserved correctly")
	}
}

func TestCapabilityWithEmptyFields(t *testing.T) {
	cap := Capability{
		Type: "test",
	}

	if cap.Endpoint != "" {
		t.Errorf("Endpoint should be empty, got %v", cap.Endpoint)
	}

	if cap.Version != "" {
		t.Errorf("Version should be empty, got %v", cap.Version)
	}

	if cap.Metadata != nil {
		t.Errorf("Metadata should be nil, got %v", cap.Metadata)
	}
}

func TestVirtualizationInfo(t *testing.T) {
	tests := []struct {
		name  string
		info  VirtualizationInfo
	}{
		{
			name: "KVM guest",
			info: VirtualizationInfo{
				Type:     "kvm",
				Role:     "guest",
				Platform: "qemu",
			},
		},
		{
			name: "physical host",
			info: VirtualizationInfo{
				Type: "none",
				Role: "host",
			},
		},
		{
			name: "Docker container",
			info: VirtualizationInfo{
				Type: "docker",
				Role: "guest",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.info.Type == "" {
				t.Error("Type should not be empty")
			}
			if tt.info.Role == "" {
				t.Error("Role should not be empty")
			}
		})
	}
}

func TestRegisterPayload(t *testing.T) {
	payload := RegisterPayload{
		AgentID:  "agent-123",
		Location: Location{Region: "eu", Zone: "zone-1"},
		Capabilities: []Capability{
			{Type: "docker", Version: "1.0"},
		},
		Hostname: "test-host",
		IP:       "192.168.1.1",
		Virtualization: &VirtualizationInfo{
			Type: "kvm",
			Role: "guest",
		},
		Labels: map[string]interface{}{
			"env": "production",
		},
	}

	if payload.AgentID != "agent-123" {
		t.Errorf("AgentID = %v, want agent-123", payload.AgentID)
	}

	if len(payload.Capabilities) != 1 {
		t.Errorf("Capabilities length = %d, want 1", len(payload.Capabilities))
	}

	if payload.Virtualization == nil {
		t.Error("Virtualization should not be nil")
	}

	if len(payload.Labels) != 1 {
		t.Errorf("Labels length = %d, want 1", len(payload.Labels))
	}
}

func TestHeartbeatPayload(t *testing.T) {
	payload := HeartbeatPayload{
		AgentID: "agent-456",
		Status:  "online",
		Metrics: map[string]interface{}{
			"cpu": 50.0,
			"mem": 60.0,
		},
		SystemInfo: &SystemInfoPayload{
			CPUUsage: 45.5,
			CPUCores: 4,
		},
	}

	if payload.AgentID != "agent-456" {
		t.Errorf("AgentID = %v, want agent-456", payload.AgentID)
	}

	if payload.Status != "online" {
		t.Errorf("Status = %v, want online", payload.Status)
	}

	if len(payload.Metrics) != 2 {
		t.Errorf("Metrics length = %d, want 2", len(payload.Metrics))
	}

	if payload.SystemInfo == nil {
		t.Error("SystemInfo should not be nil")
	}
}

func TestSystemInfoPayload(t *testing.T) {
	info := SystemInfoPayload{
		CPUUsage:         75.5,
		CPUCores:         8,
		CPUFreqMHz:       2400.0,
		MemTotal:         16 * 1024 * 1024 * 1024,
		MemUsed:          8 * 1024 * 1024 * 1024,
		MemAvailable:     8 * 1024 * 1024 * 1024,
		MemUsagePercent:  50.0,
		DiskTotal:        500 * 1024 * 1024 * 1024,
		DiskUsed:         250 * 1024 * 1024 * 1024,
		DiskFree:         250 * 1024 * 1024 * 1024,
		DiskUsagePercent: 50.0,
		NetBytesSent:     1024 * 1024,
		NetBytesRecv:     2048 * 1024,
		OSName:           "Linux",
		OSVersion:        "5.15.0",
		Arch:             "amd64",
		Uptime:           86400,
		Hostname:         "test-host",
		Load1:            1.5,
		Load5:            1.2,
		Load15:           1.0,
	}

	if info.CPUUsage != 75.5 {
		t.Errorf("CPUUsage = %v, want 75.5", info.CPUUsage)
	}

	if info.CPUCores != 8 {
		t.Errorf("CPUCores = %v, want 8", info.CPUCores)
	}

	if info.MemUsagePercent != 50.0 {
		t.Errorf("MemUsagePercent = %v, want 50.0", info.MemUsagePercent)
	}

	if info.OSName != "Linux" {
		t.Errorf("OSName = %v, want Linux", info.OSName)
	}

	if info.Load1 != 1.5 {
		t.Errorf("Load1 = %v, want 1.5", info.Load1)
	}
}

func TestRPCRequestPayload(t *testing.T) {
	payload := RPCRequestPayload{
		Method: "containers.list",
		Params: map[string]interface{}{
			"all": true,
		},
	}

	if payload.Method != "containers.list" {
		t.Errorf("Method = %v, want containers.list", payload.Method)
	}

	if len(payload.Params) != 1 {
		t.Errorf("Params length = %d, want 1", len(payload.Params))
	}
}

func TestRPCResponsePayload(t *testing.T) {
	tests := []struct {
		name   string
		payload RPCResponsePayload
	}{
		{
			name: "success response",
			payload: RPCResponsePayload{
				Status: "success",
				Data:   []string{"item1", "item2"},
			},
		},
		{
			name: "error response",
			payload: RPCResponsePayload{
				Status: "error",
				Error:  "something went wrong",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.payload.Status == "" {
				t.Error("Status should not be empty")
			}
		})
	}
}

func TestRegisterResponse(t *testing.T) {
	resp := RegisterResponse{
		Status:            "ok",
		ServerTime:        1234567890,
		HeartbeatInterval: 30,
	}

	if resp.Status != "ok" {
		t.Errorf("Status = %v, want ok", resp.Status)
	}

	if resp.HeartbeatInterval != 30 {
		t.Errorf("HeartbeatInterval = %v, want 30", resp.HeartbeatInterval)
	}
}

func TestProxyNewPayload(t *testing.T) {
	payload := ProxyNewPayload{
		ProxyID:   "proxy-1",
		ProxyType: "tcp",
		Target:    "192.168.1.100:22",
	}

	if payload.ProxyID != "proxy-1" {
		t.Errorf("ProxyID = %v, want proxy-1", payload.ProxyID)
	}

	if payload.ProxyType != "tcp" {
		t.Errorf("ProxyType = %v, want tcp", payload.ProxyType)
	}

	if payload.Target != "192.168.1.100:22" {
		t.Errorf("Target = %v, want 192.168.1.100:22", payload.Target)
	}
}

func TestProxyDataPayload(t *testing.T) {
	payload := ProxyDataPayload{
		ProxyID: "proxy-1",
		ConnID:  "conn-1",
		Data:    []byte("test data"),
		NewConn: true,
	}

	if payload.ProxyID != "proxy-1" {
		t.Errorf("ProxyID = %v, want proxy-1", payload.ProxyID)
	}

	if len(payload.Data) != 9 {
		t.Errorf("Data length = %d, want 9", len(payload.Data))
	}

	if !payload.NewConn {
		t.Error("NewConn should be true")
	}
}

func TestProxyClosePayload(t *testing.T) {
	payload := ProxyClosePayload{
		ProxyID: "proxy-1",
		ConnID:  "conn-1",
		Reason:  "client closed",
	}

	if payload.ProxyID != "proxy-1" {
		t.Errorf("ProxyID = %v, want proxy-1", payload.ProxyID)
	}

	if payload.Reason != "client closed" {
		t.Errorf("Reason = %v, want client closed", payload.Reason)
	}
}

func TestProxyErrorPayload(t *testing.T) {
	payload := ProxyErrorPayload{
		ProxyID: "proxy-1",
		ConnID:  "conn-1",
		Error:   "connection refused",
	}

	if payload.ProxyID != "proxy-1" {
		t.Errorf("ProxyID = %v, want proxy-1", payload.ProxyID)
	}

	if payload.Error != "connection refused" {
		t.Errorf("Error = %v, want connection refused", payload.Error)
	}
}

func TestMessageWithAllPayloadTypes(t *testing.T) {
	// Test creating messages with different payload types
	msg1 := NewMessage(MessageTypeRegister, map[string]interface{}{
		"agentId": "test",
	})

	msg2 := NewMessage(MessageTypeHeartbeat, map[string]interface{}{
		"status": "online",
	})

	if msg1.Type != MessageTypeRegister {
		t.Error("Message type not preserved")
	}

	if msg2.Type != MessageTypeHeartbeat {
		t.Error("Message type not preserved")
	}
}

func TestMessageTimestamp(t *testing.T) {
	before := time.Now().Unix()
	msg := NewMessage(MessageTypePing, nil)
	after := time.Now().Unix()

	if msg.Timestamp < before || msg.Timestamp > after {
		t.Errorf("Timestamp = %v, want between %d and %d", msg.Timestamp, before, after)
	}
}
