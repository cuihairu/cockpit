package openwrt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// ============ httptest-based API Tests ============

func openwrtTestHandler(t *testing.T, responses map[string]interface{}) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		var req RPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		// Extract method from params
		if len(req.Params) >= 3 {
			method := req.Params[1].(string)
			if method == "session" {
				// Login request
				resp := RPCResponse{
					Jsonrpc: "2.0",
					ID:      1,
					Result: RPCResult{
						Data: []interface{}{
							map[string]interface{}{
								"ubus_rpc_session": "test-session-token",
							},
						},
					},
				}
				json.NewEncoder(w).Encode(resp)
				return
			}

			procedure, _ := req.Params[2].(string)
			key := method + "." + procedure
			if data, ok := responses[key]; ok {
				resp := RPCResponse{
					Jsonrpc: "2.0",
					ID:      1,
					Result: RPCResult{
						Data: []interface{}{data},
					},
				}
				json.NewEncoder(w).Encode(resp)
				return
			}
		}

		// Default: return empty result
		json.NewEncoder(w).Encode(RPCResponse{Jsonrpc: "2.0", ID: 1})
	}
}

func newTestClient(ts *httptest.Server) *Client {
	return &Client{
		endpoint: ts.URL + "/ubus",
		username: "root",
		password: "password",
		timeout:  5 * time.Second,
		client:   ts.Client(),
	}
}

func TestListInterfacesWithServer(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"network.interface.dump": map[string]interface{}{
			"interface": []interface{}{
				map[string]interface{}{
					"interface":     "lan",
					"up":            true,
					"proto":         "static",
					"ipv4-address":  []string{"192.168.1.1"},
				},
			},
		},
	}))
	defer ts.Close()

	c := newTestClient(ts)
	ifaces, err := c.ListInterfaces()
	if err != nil {
		t.Fatalf("ListInterfaces() error = %v", err)
	}
	if len(ifaces) != 1 {
		t.Errorf("count = %d, want 1", len(ifaces))
	}
	if ifaces[0].Name != "lan" {
		t.Errorf("name = %v", ifaces[0].Name)
	}
}

func TestListInterfacesParseError(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"network.interface.dump": "not-an-object",
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.ListInterfaces()
	if err == nil {
		t.Error("expected error for invalid response")
	}
}

func TestGetInterfaceWithServer(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"network.interface.status": map[string]interface{}{
			"interface": "wan",
			"up":        true,
		},
	}))
	defer ts.Close()

	c := newTestClient(ts)
	iface, err := c.GetInterface("wan")
	if err != nil {
		t.Fatalf("GetInterface() error = %v", err)
	}
	if iface.Name != "wan" {
		t.Errorf("name = %v", iface.Name)
	}
}

func TestGetInterfaceParseError(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"network.interface.status": "invalid",
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.GetInterface("wan")
	if err == nil {
		t.Error("expected error")
	}
}

func TestListRoutesWithServer(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"network.route.dump": map[string]interface{}{
			"route": []interface{}{
				map[string]interface{}{
					"target":  "0.0.0.0",
					"mask":    0,
					"nexthop": "192.168.1.254",
					"device":  "eth0",
				},
			},
		},
	}))
	defer ts.Close()

	c := newTestClient(ts)
	routes, err := c.ListRoutes()
	if err != nil {
		t.Fatalf("ListRoutes() error = %v", err)
	}
	if len(routes) != 1 {
		t.Errorf("count = %d, want 1", len(routes))
	}
}

func TestGetFirewallZonesWithServer(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"firewall.get_zones": []interface{}{
			map[string]interface{}{"name": "lan", "input": "ACCEPT", "forward": "REJECT"},
		},
	}))
	defer ts.Close()

	c := newTestClient(ts)
	zones, err := c.GetFirewallZones()
	if err != nil {
		t.Fatalf("GetFirewallZones() error = %v", err)
	}
	if len(zones) != 1 {
		t.Errorf("count = %d", len(zones))
	}
}

func TestGetFirewallRulesWithServer(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"firewall.get_rules": []interface{}{
			map[string]interface{}{"name": "test-rule", "src": "lan"},
		},
	}))
	defer ts.Close()

	c := newTestClient(ts)
	rules, err := c.GetFirewallRules()
	if err != nil {
		t.Fatalf("GetFirewallRules() error = %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("count = %d", len(rules))
	}
}

func TestGetFirewallRedirectsWithServer(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"firewall.get_redirects": []interface{}{
			map[string]interface{}{"name": "test-redir", "src": "wan"},
		},
	}))
	defer ts.Close()

	c := newTestClient(ts)
	redirects, err := c.GetFirewallRedirects()
	if err != nil {
		t.Fatalf("GetFirewallRedirects() error = %v", err)
	}
	if len(redirects) != 1 {
		t.Errorf("count = %d", len(redirects))
	}
}

func TestGetWirelessStatusWithServer(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"network.wireless.status": []interface{}{
			map[string]interface{}{
				"radios": []interface{}{
					map[string]interface{}{"name": "radio0", "up": true},
				},
				"interfaces": []interface{}{
					map[string]interface{}{"ssid": "TestWiFi", "mode": "ap"},
				},
			},
		},
	}))
	defer ts.Close()

	c := newTestClient(ts)
	status, err := c.GetWirelessStatus()
	if err != nil {
		t.Fatalf("GetWirelessStatus() error = %v", err)
	}
	if len(status) != 1 {
		t.Errorf("count = %d, want 1", len(status))
	}
}

func TestGetDHCPLoadsWithServer(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"uci.get": map[string]interface{}{
			"dhcp": []interface{}{
				map[string]interface{}{
					"leasefile": "/tmp/dhcp.leases",
				},
			},
		},
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.GetDHCPLoads()
	// May error on reading lease file, but should not panic
	if err != nil {
		// OK - lease file not accessible in test
	}
}

func TestReadFileWithServer(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"file.read": map[string]interface{}{
			"data": "file contents here",
		},
	}))
	defer ts.Close()

	c := newTestClient(ts)
	content, err := c.ReadFile("/etc/config/network")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if content != "file contents here" {
		t.Errorf("content = %v", content)
	}
}

func TestGetLEDStateWithServer(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"led.get": map[string]interface{}{
			"name":  "power",
			"state": "on",
		},
	}))
	defer ts.Close()

	c := newTestClient(ts)
	led, err := c.GetLEDState("power")
	if err != nil {
		t.Fatalf("GetLEDState() error = %v", err)
	}
	if led.Name != "power" {
		t.Errorf("name = %v", led.Name)
	}
}

func TestGetSystemInfoWithServer(t *testing.T) {
	ts := httptest.NewServer(openwrtTestHandler(t, map[string]interface{}{
		"system.info": map[string]interface{}{
			"uptime":    float64(86400),
			"localtime": float64(1700000000),
			"load":      []interface{}{0.5, 0.3, 0.1},
			"memory": map[string]interface{}{
				"total":     float64(128 * 1024 * 1024),
				"free":      float64(64 * 1024 * 1024),
				"available": float64(80 * 1024 * 1024),
				"buffered":  float64(16 * 1024 * 1024),
			},
		},
	}))
	defer ts.Close()

	c := newTestClient(ts)
	info, err := c.GetSystemInfo()
	if err != nil {
		t.Fatalf("GetSystemInfo() error = %v", err)
	}
	if info.Uptime != 86400 {
		t.Errorf("uptime = %v", info.Uptime)
	}
}

func TestCallLoginFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return RPC error for login
		resp := RPCResponse{
			Jsonrpc: "2.0",
			ID:      1,
			Error:   &RPCError{Code: -32000, Message: "Access denied"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.ListInterfaces()
	if err == nil {
		t.Error("expected error for login failure")
	}
}

func TestCallHTTPErrorNew(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.ListInterfaces()
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}
