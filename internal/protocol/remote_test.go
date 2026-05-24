package protocol

import (
	"testing"
)

func TestRemoteProtocolConstants(t *testing.T) {
	protocols := []RemoteProtocol{
		RemoteProtocolSSH,
		RemoteProtocolRDP,
		RemoteProtocolVNC,
		RemoteProtocolTelnet,
		RemoteProtocolFTP,
	}

	for _, protocol := range protocols {
		if protocol == "" {
			t.Errorf("RemoteProtocol constant should not be empty")
		}
	}

	// Check specific values
	if RemoteProtocolSSH != "ssh" {
		t.Errorf("RemoteProtocolSSH = %v, want ssh", RemoteProtocolSSH)
	}
	if RemoteProtocolRDP != "rdp" {
		t.Errorf("RemoteProtocolRDP = %v, want rdp", RemoteProtocolRDP)
	}
	if RemoteProtocolVNC != "vnc" {
		t.Errorf("RemoteProtocolVNC = %v, want vnc", RemoteProtocolVNC)
	}
	if RemoteProtocolTelnet != "telnet" {
		t.Errorf("RemoteProtocolTelnet = %v, want telnet", RemoteProtocolTelnet)
	}
	if RemoteProtocolFTP != "ftp" {
		t.Errorf("RemoteProtocolFTP = %v, want ftp", RemoteProtocolFTP)
	}
}

func TestRemoteConnectionInfo(t *testing.T) {
	info := RemoteConnectionInfo{
		Protocol: RemoteProtocolSSH,
		Host:     "192.168.1.1",
		Port:     22,
		Username: "admin",
		Password: "password",
		AuthType: "password",
		Name:     "Test Server",
	}

	if info.Protocol != RemoteProtocolSSH {
		t.Errorf("Protocol = %v, want %v", info.Protocol, RemoteProtocolSSH)
	}

	if info.Host != "192.168.1.1" {
		t.Errorf("Host = %v, want 192.168.1.1", info.Host)
	}

	if info.Port != 22 {
		t.Errorf("Port = %v, want 22", info.Port)
	}

	if info.Username != "admin" {
		t.Errorf("Username = %v, want admin", info.Username)
	}

	if info.AuthType != "password" {
		t.Errorf("AuthType = %v, want password", info.AuthType)
	}

	if info.Name != "Test Server" {
		t.Errorf("Name = %v, want Test Server", info.Name)
	}
}

func TestRemoteConnectionInfoWithEmptyFields(t *testing.T) {
	info := RemoteConnectionInfo{
		Protocol: RemoteProtocolVNC,
		Host:     "localhost",
		Port:     5900,
	}

	if info.Username != "" {
		t.Errorf("Username should be empty, got %v", info.Username)
	}

	if info.Password != "" {
		t.Errorf("Password should be empty, got %v", info.Password)
	}

	if info.AuthType != "" {
		t.Errorf("AuthType should be empty, got %v", info.AuthType)
	}

	if info.Name != "" {
		t.Errorf("Name should be empty, got %v", info.Name)
	}
}

func TestRemoteConnectionInfoAllProtocols(t *testing.T) {
	tests := []struct {
		protocol RemoteProtocol
		host     string
		port     int
	}{
		{RemoteProtocolSSH, "ssh.example.com", 22},
		{RemoteProtocolRDP, "rdp.example.com", 3389},
		{RemoteProtocolVNC, "vnc.example.com", 5900},
		{RemoteProtocolTelnet, "telnet.example.com", 23},
		{RemoteProtocolFTP, "ftp.example.com", 21},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			info := RemoteConnectionInfo{
				Protocol: tt.protocol,
				Host:     tt.host,
				Port:     tt.port,
			}

			if info.Protocol != tt.protocol {
				t.Errorf("Protocol = %v, want %v", info.Protocol, tt.protocol)
			}

			if info.Host != tt.host {
				t.Errorf("Host = %v, want %v", info.Host, tt.host)
			}

			if info.Port != tt.port {
				t.Errorf("Port = %v, want %v", info.Port, tt.port)
			}
		})
	}
}

func TestRemoteConnectionInfoAuthTypes(t *testing.T) {
	authTypes := []string{"password", "key", "none"}

	for _, authType := range authTypes {
		info := RemoteConnectionInfo{
			Protocol: RemoteProtocolSSH,
			Host:     "example.com",
			Port:     22,
			AuthType: authType,
		}

		if info.AuthType != authType {
			t.Errorf("AuthType = %v, want %v", info.AuthType, authType)
		}
	}
}

func TestRemoteServiceInfo(t *testing.T) {
	info := RemoteServiceInfo{
		Protocol: RemoteProtocolSSH,
		Host:     "0.0.0.0",
		Port:     22,
		Name:     "SSH Server",
		Running:  true,
	}

	if info.Protocol != RemoteProtocolSSH {
		t.Errorf("Protocol = %v, want %v", info.Protocol, RemoteProtocolSSH)
	}

	if info.Host != "0.0.0.0" {
		t.Errorf("Host = %v, want 0.0.0.0", info.Host)
	}

	if info.Port != 22 {
		t.Errorf("Port = %v, want 22", info.Port)
	}

	if info.Name != "SSH Server" {
		t.Errorf("Name = %v, want SSH Server", info.Name)
	}

	if !info.Running {
		t.Error("Running should be true")
	}
}

func TestRemoteServiceInfoNotRunning(t *testing.T) {
	info := RemoteServiceInfo{
		Protocol: RemoteProtocolTelnet,
		Host:     "127.0.0.1",
		Port:     23,
		Name:     "Telnet",
		Running:  false,
	}

	if info.Running {
		t.Error("Running should be false")
	}
}

func TestRemoteServiceInfoAllHosts(t *testing.T) {
	hosts := []string{"0.0.0.0", "127.0.0.1", "::", "localhost"}

	for _, host := range hosts {
		info := RemoteServiceInfo{
			Protocol: RemoteProtocolSSH,
			Host:     host,
			Port:     22,
			Running:  true,
		}

		if info.Host != host {
			t.Errorf("Host = %v, want %v", info.Host, host)
		}
	}
}

func TestRemoteProxyStartPayload(t *testing.T) {
	payload := RemoteProxyStartPayload{
		ConnectionID: "conn-123",
		Protocol:     RemoteProtocolSSH,
		Target:       "192.168.1.100:22",
		Timeout:      30,
	}

	if payload.ConnectionID != "conn-123" {
		t.Errorf("ConnectionID = %v, want conn-123", payload.ConnectionID)
	}

	if payload.Protocol != RemoteProtocolSSH {
		t.Errorf("Protocol = %v, want %v", payload.Protocol, RemoteProtocolSSH)
	}

	if payload.Target != "192.168.1.100:22" {
		t.Errorf("Target = %v, want 192.168.1.100:22", payload.Target)
	}

	if payload.Timeout != 30 {
		t.Errorf("Timeout = %v, want 30", payload.Timeout)
	}
}

func TestRemoteProxyDataPayload(t *testing.T) {
	data := []byte("test data")

	payload := RemoteProxyDataPayload{
		ConnectionID: "conn-456",
		Data:         data,
	}

	if payload.ConnectionID != "conn-456" {
		t.Errorf("ConnectionID = %v, want conn-456", payload.ConnectionID)
	}

	if len(payload.Data) != len(data) {
		t.Errorf("Data length = %v, want %v", len(payload.Data), len(data))
	}

	if string(payload.Data) != string(data) {
		t.Error("Data content mismatch")
	}
}

func TestRemoteProxyDataPayloadEmpty(t *testing.T) {
	payload := RemoteProxyDataPayload{
		ConnectionID: "conn-789",
		Data:         []byte{},
	}

	if len(payload.Data) != 0 {
		t.Errorf("Data should be empty, got length %v", len(payload.Data))
	}
}

func TestRemoteProxyClosePayload(t *testing.T) {
	payload := RemoteProxyClosePayload{
		ConnectionID: "conn-999",
		Reason:       "client disconnected",
	}

	if payload.ConnectionID != "conn-999" {
		t.Errorf("ConnectionID = %v, want conn-999", payload.ConnectionID)
	}

	if payload.Reason != "client disconnected" {
		t.Errorf("Reason = %v, want client disconnected", payload.Reason)
	}
}

func TestRemoteProxyClosePayloadEmptyReason(t *testing.T) {
	payload := RemoteProxyClosePayload{
		ConnectionID: "conn-888",
		Reason:       "",
	}

	if payload.Reason != "" {
		t.Errorf("Reason should be empty, got %v", payload.Reason)
	}
}

func TestRemoteProtocolString(t *testing.T) {
	protocols := []struct {
		value    RemoteProtocol
		expected string
	}{
		{RemoteProtocolSSH, "ssh"},
		{RemoteProtocolRDP, "rdp"},
		{RemoteProtocolVNC, "vnc"},
		{RemoteProtocolTelnet, "telnet"},
		{RemoteProtocolFTP, "ftp"},
	}

	for _, tt := range protocols {
		if string(tt.value) != tt.expected {
			t.Errorf("RemoteProtocol string = %v, want %v", tt.value, tt.expected)
		}
	}
}
