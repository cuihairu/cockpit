package openwrt

import (
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"default HTTPS", Config{
			Host:     "192.168.1.1",
			Port:     443,
			Username: "root",
		}},
		{"HTTP", Config{
			Host:     "192.168.1.1",
			Port:     80,
			Username: "root",
		}},
		{"with credentials", Config{
			Host:     "openwrt.local",
			Port:     443,
			Username: "admin",
			Password: "password",
		}},
		{"with timeout", Config{
			Host:    "192.168.1.1",
			Port:    443,
			Timeout: 60 * time.Second,
		}},
		{"with insecure TLS", Config{
			Host:        "192.168.1.1",
			Port:        443,
			InsecureTLS: true,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(tt.cfg)
			if c == nil {
				t.Error("NewClient() should not return nil")
			}

			if c.username != tt.cfg.Username {
				t.Errorf("username = %v, want %v", c.username, tt.cfg.Username)
			}

			if c.password != tt.cfg.Password {
				t.Errorf("password = %v, want %v", c.password, tt.cfg.Password)
			}

			expectedTimeout := tt.cfg.Timeout
			if expectedTimeout == 0 {
				expectedTimeout = 30 * time.Second
			}
			if c.timeout != expectedTimeout {
				t.Errorf("timeout = %v, want %v", c.timeout, expectedTimeout)
			}

			if c.client == nil {
				t.Error("HTTP client should not be nil")
			}
		})
	}
}

func TestNewClientHTTPEndpoint(t *testing.T) {
	c := NewClient(Config{
		Host: "192.168.1.1",
		Port: 80,
	})

	// The actual implementation includes the port
	expected := "http://192.168.1.1:80/ubus"
	if c.endpoint != expected {
		t.Errorf("endpoint = %v, want %v", c.endpoint, expected)
	}
}

func TestNewClientHTTPSEndpoint(t *testing.T) {
	c := NewClient(Config{
		Host: "192.168.1.1",
		Port: 443,
	})

	if c.endpoint != "https://192.168.1.1:443/ubus" {
		t.Errorf("endpoint = %v, want https://192.168.1.1:443/ubus", c.endpoint)
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}

	if cfg.Host != "" {
		t.Error("Host should be empty by default")
	}

	if cfg.Port != 0 {
		t.Error("Port should be 0 by default")
	}

	if cfg.Username != "" {
		t.Error("Username should be empty by default")
	}

	if cfg.Password != "" {
		t.Error("Password should be empty by default")
	}

	if cfg.Timeout != 0 {
		t.Error("Timeout should be 0 by default")
	}

	if cfg.InsecureTLS != false {
		t.Error("InsecureTLS should be false by default")
	}
}

func TestClientFields(t *testing.T) {
	cfg := Config{
		Host:        "router.local",
		Port:        8443,
		Username:    "admin",
		Password:    "secret123",
		Timeout:     45 * time.Second,
		InsecureTLS: true,
	}

	c := NewClient(cfg)

	if c.endpoint != "https://router.local:8443/ubus" {
		t.Errorf("endpoint = %v", c.endpoint)
	}

	if c.username != "admin" {
		t.Errorf("username = %v", c.username)
	}

	if c.password != "secret123" {
		t.Errorf("password = %v", c.password)
	}

	if c.timeout != 45*time.Second {
		t.Errorf("timeout = %v, want 45s", c.timeout)
	}
}

func TestClientTimeoutDefault(t *testing.T) {
	c := NewClient(Config{
		Host: "192.168.1.1",
		Port: 443,
	})

	if c.timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", c.timeout)
	}

	if c.client.Timeout != 30*time.Second {
		t.Errorf("HTTP client timeout = %v, want 30s", c.client.Timeout)
	}
}

func TestMultipleClients(t *testing.T) {
	cfg := Config{
		Host:     "192.168.1.1",
		Port:     443,
		Username: "root",
	}

	for i := 0; i < 5; i++ {
		c := NewClient(cfg)
		if c == nil {
			t.Errorf("NewClient() iteration %d returned nil", i)
		}
	}
}

func TestConcurrentClientCreation(t *testing.T) {
	cfg := Config{
		Host:     "192.168.1.1",
		Port:     443,
		Username: "root",
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
		Host:  "original.example.com",
		Port:  443,
		Username: "user1",
	}

	c1 := NewClient(cfg)

	// Modify config
	cfg.Host = "modified.example.com"
	cfg.Username = "user2"

	c2 := NewClient(cfg)

	if c1.endpoint[:len("https://original.example.com")] != "https://original.example.com" {
		t.Errorf("c1.endpoint should start with original endpoint")
	}

	if c1.username != "user1" {
		t.Errorf("c1.username = %v, want user1", c1.username)
	}

	if c2.username != "user2" {
		t.Errorf("c2.username = %v, want user2", c2.username)
	}
}

func TestEndpointFormats(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		expected string
	}{
		{"IPv4 HTTPS", "192.168.1.1", 443, "https://192.168.1.1:443/ubus"},
		{"IPv4 HTTP", "192.168.1.1", 80, "http://192.168.1.1:80/ubus"},
		{"hostname HTTPS", "router.local", 443, "https://router.local:443/ubus"},
		{"hostname HTTP", "router.local", 80, "http://router.local:80/ubus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(Config{Host: tt.host, Port: tt.port})
			if c.endpoint != tt.expected {
				t.Errorf("endpoint = %v, want %v", c.endpoint, tt.expected)
			}
		})
	}
}

func TestPortVariations(t *testing.T) {
	ports := []int{0, 80, 443, 8080, 8443}

	for _, port := range ports {
		c := NewClient(Config{
			Host: "192.168.1.1",
			Port: port,
		})

		// Client should be created for any port
		if c == nil {
			t.Errorf("NewClient() with port %d should not return nil", port)
		}
	}
}

func TestEmptyCredentials(t *testing.T) {
	c := NewClient(Config{
		Host: "192.168.1.1",
		Port: 443,
	})

	if c.username != "" {
		t.Error("username should be empty")
	}

	if c.password != "" {
		t.Error("password should be empty")
	}

	// Client should still be valid
	if c == nil {
		t.Error("NewClient() should not return nil without credentials")
	}
}

func TestTimeoutVariations(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		expected time.Duration
	}{
		{"5 seconds", 5 * time.Second, 5 * time.Second},
		{"30 seconds", 30 * time.Second, 30 * time.Second},
		{"1 minute", 1 * time.Minute, 1 * time.Minute},
		{"zero defaults to 30s", 0, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(Config{
				Host:    "192.168.1.1",
				Port:    443,
				Timeout: tt.timeout,
			})

			if c.timeout != tt.expected {
				t.Errorf("timeout = %v, want %v", c.timeout, tt.expected)
			}
		})
	}
}

func TestInsecureTLSConfiguration(t *testing.T) {
	secureClient := NewClient(Config{
		Host:        "192.168.1.1",
		Port:        443,
		InsecureTLS: false,
	})

	insecureClient := NewClient(Config{
		Host:        "192.168.1.1",
		Port:        443,
		InsecureTLS: true,
	})

	// Both should have HTTP clients
	if secureClient.client == nil {
		t.Error("secure client should have HTTP client")
	}

	if insecureClient.client == nil {
		t.Error("insecure client should have HTTP client")
	}
}

func TestRPCRequestStruct(t *testing.T) {
	req := RPCRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "call",
		Params:  []interface{}{"system", "info"},
	}

	if req.Jsonrpc != "2.0" {
		t.Errorf("Jsonrpc = %v, want 2.0", req.Jsonrpc)
	}

	if req.ID != 1 {
		t.Errorf("ID = %v, want 1", req.ID)
	}

	if req.Method != "call" {
		t.Errorf("Method = %v, want call", req.Method)
	}

	if len(req.Params) != 2 {
		t.Errorf("Params length = %d, want 2", len(req.Params))
	}
}

func TestRPCResponseStruct(t *testing.T) {
	resp := RPCResponse{
		Jsonrpc: "2.0",
		ID:      1,
	}

	if resp.Jsonrpc != "2.0" {
		t.Errorf("Jsonrpc = %v, want 2.0", resp.Jsonrpc)
	}

	if resp.ID != 1 {
		t.Errorf("ID = %v, want 1", resp.ID)
	}
}
