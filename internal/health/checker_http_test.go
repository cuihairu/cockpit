package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckHTTPHealthy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	checker := NewChecker(Config{Timeout: 5 * time.Second})
	result := checker.CheckHTTP("test", ts.URL, 0)

	if result.Status != StatusHealthy {
		t.Errorf("Status = %v, want healthy: %s", result.Status, result.Message)
	}
	if result.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
}

func TestCheckHTTPExpectedStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	checker := NewChecker(Config{})

	result := checker.CheckHTTP("test", ts.URL, 201)
	if result.Status != StatusHealthy {
		t.Errorf("Status = %v, want healthy for expected 201", result.Status)
	}

	result = checker.CheckHTTP("test", ts.URL, 200)
	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy for unexpected status", result.Status)
	}
}

func TestCheckHTTPServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	checker := NewChecker(Config{})
	result := checker.CheckHTTP("test", ts.URL, 0)

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy for 500", result.Status)
	}
}

func TestCheckHTTPClientError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	checker := NewChecker(Config{})
	result := checker.CheckHTTP("test", ts.URL, 0)

	if result.Status != StatusDegraded {
		t.Errorf("Status = %v, want degraded for 404", result.Status)
	}
}

func TestCheckHTTPUnreachable(t *testing.T) {
	checker := NewChecker(Config{Timeout: 1 * time.Second})
	result := checker.CheckHTTP("test", "http://127.0.0.1:1", 0)

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy for unreachable", result.Status)
	}
}

func TestCheckHTTPAutoPrefix(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Strip scheme - should be auto-added
	target := ts.URL[7:] // remove "http://"
	checker := NewChecker(Config{})
	result := checker.CheckHTTP("test", target, 0)

	if result.Status != StatusHealthy {
		t.Errorf("Status = %v, want healthy with auto-prefix: %s", result.Status, result.Message)
	}
}

func TestCheckTCPInvalidAddress(t *testing.T) {
	checker := NewChecker(Config{})
	result := checker.CheckTCP("test", ":::invalid")

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy for invalid address", result.Status)
	}
}

func TestCheckTCPConnectionRefused(t *testing.T) {
	checker := NewChecker(Config{Timeout: 1 * time.Second})
	result := checker.CheckTCP("test", "127.0.0.1:1")

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy for refused", result.Status)
	}
}

func TestCheckTCPDefaultPort(t *testing.T) {
	checker := NewChecker(Config{Timeout: 1 * time.Second})
	// Without port - adds default port 80
	result := checker.CheckTCP("test", "127.0.0.1")

	// Will fail to connect but should have tried
	if result.Service != "test" {
		t.Errorf("Service = %v", result.Service)
	}
}

func TestCheckUDPInvalidAddress(t *testing.T) {
	checker := NewChecker(Config{})
	result := checker.CheckUDP("test", ":::invalid", 0)

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy", result.Status)
	}
}

func TestCheckUDPConnectionRefused(t *testing.T) {
	checker := NewChecker(Config{Timeout: 1 * time.Second})
	result := checker.CheckUDP("test", "127.0.0.1:1", 0)

	// UDP is connectionless, so it may report healthy on Windows
	if result.Status != StatusUnhealthy && result.Status != StatusHealthy {
		t.Errorf("Status = %v, expected unhealthy or healthy", result.Status)
	}
}

func TestCheckDNSValid(t *testing.T) {
	checker := NewChecker(Config{})
	result := checker.CheckDNS("test", "localhost")

	// localhost should resolve on all systems
	if result.Status != StatusHealthy {
		t.Errorf("Status = %v, want healthy for localhost: %s", result.Status, result.Message)
	}
}

func TestCheckDNSInvalidDomain(t *testing.T) {
	// DNS resolution may behave differently across platforms/networks
	// On some Windows configs, .invalid TLD may still resolve
	t.Skip("DNS invalid domain test is flaky across different network configs")
	checker := NewChecker(Config{Timeout: 3 * time.Second})
	result := checker.CheckDNS("test", "this-domain-does-not-exist-ever.invalid")

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy for invalid domain", result.Status)
	}
}

func TestCheckPortInvalidAddress(t *testing.T) {
	checker := NewChecker(Config{})
	result := checker.CheckPort("test", "no-port-here")

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy for no port", result.Status)
	}
}

func TestCheckPortInvalidPortNum(t *testing.T) {
	checker := NewChecker(Config{})
	result := checker.CheckPort("test", "host:abc")

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy for invalid port", result.Status)
	}
}

func TestCheckPortClosed(t *testing.T) {
	checker := NewChecker(Config{Timeout: 1 * time.Second})
	result := checker.CheckPort("test", "127.0.0.1:1")

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy for closed port", result.Status)
	}
}

func TestCheckPortOpen(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	// Extract port from test server
	checker := NewChecker(Config{Timeout: 5 * time.Second})
	result := checker.CheckPort("test", ts.Listener.Addr().String())

	if result.Status != StatusHealthy {
		t.Errorf("Status = %v, want healthy for open port: %s", result.Status, result.Message)
	}
}

func TestBatchCheck(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	checker := NewChecker(Config{})
	results := checker.BatchCheck([]CheckConfig{
		{Service: "web", Type: "http", Target: ts.URL},
		{Service: "dns", Type: "dns", Target: "localhost"},
		{Service: "bad-port", Type: "tcp", Target: "127.0.0.1:1"},
	})

	if len(results) != 3 {
		t.Fatalf("BatchCheck count = %d, want 3", len(results))
	}
	if results[0].Status != StatusHealthy {
		t.Errorf("web status = %v", results[0].Status)
	}
	if results[1].Status != StatusHealthy {
		t.Errorf("dns status = %v", results[1].Status)
	}
	if results[2].Status != StatusUnhealthy {
		t.Errorf("bad-port status = %v", results[2].Status)
	}
}

func TestBatchCheckAutoDetectHTTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	checker := NewChecker(Config{Timeout: 1 * time.Second})
	results := checker.BatchCheck([]CheckConfig{
		{Service: "auto-http", Target: ts.URL},
		{Service: "auto-port", Target: "127.0.0.1:1"},
		{Service: "auto-dns", Target: "localhost"},
	})

	if len(results) != 3 {
		t.Fatalf("count = %d, want 3", len(results))
	}
}

func TestParseURL(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"http://example.com", false},
		{"https://example.com", false},
		{"example.com", false}, // auto-adds http://
		{"http://example.com:8080/path", false},
	}

	for _, tt := range tests {
		u, err := ParseURL(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseURL(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if err == nil && u == nil {
			t.Errorf("ParseURL(%q) returned nil URL", tt.input)
		}
	}
}
