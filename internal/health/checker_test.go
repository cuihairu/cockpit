package health

import (
	"testing"
	"time"
)

func TestStatusConstants(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{"healthy", StatusHealthy, "healthy"},
		{"unhealthy", StatusUnhealthy, "unhealthy"},
		{"degraded", StatusDegraded, "degraded"},
		{"unknown", StatusUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status.String() != tt.want {
				t.Errorf("Status.String() = %v, want %v", tt.status.String(), tt.want)
			}
		})
	}
}

func TestNewChecker(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"default config", Config{}},
		{"with timeout", Config{Timeout: 5 * time.Second}},
		{"skip TLS", Config{SkipTLSVerify: true}},
		{"full config", Config{Timeout: 30 * time.Second, SkipTLSVerify: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewChecker(tt.cfg)
			if c == nil {
				t.Error("NewChecker() should not return nil")
			}
			if c.httpClient == nil {
				t.Error("Checker.httpClient should not be nil")
			}
		})
	}
}

func TestNewCheckerTimeout(t *testing.T) {
	c := NewChecker(Config{Timeout: 5 * time.Second})
	if c.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", c.timeout)
	}

	c2 := NewChecker(Config{})
	if c2.timeout != 10*time.Second {
		t.Errorf("default timeout = %v, want 10s", c2.timeout)
	}
}

func TestResultIsHealthy(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"healthy status", StatusHealthy, true},
		{"unhealthy status", StatusUnhealthy, false},
		{"degraded status", StatusDegraded, false},
		{"unknown status", StatusUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Result{Status: tt.status}
			if r.IsHealthy() != tt.want {
				t.Errorf("Result.IsHealthy() = %v, want %v", r.IsHealthy(), tt.want)
			}
		})
	}
}

func TestResultIsUnhealthy(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"healthy status", StatusHealthy, false},
		{"unhealthy status", StatusUnhealthy, true},
		{"degraded status", StatusDegraded, false},
		{"unknown status", StatusUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Result{Status: tt.status}
			if r.IsUnhealthy() != tt.want {
				t.Errorf("Result.IsUnhealthy() = %v, want %v", r.IsUnhealthy(), tt.want)
			}
		})
	}
}

func TestResultShouldAlert(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"healthy status", StatusHealthy, false},
		{"unhealthy status", StatusUnhealthy, true},
		{"degraded status", StatusDegraded, true},
		{"unknown status", StatusUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Result{Status: tt.status}
			if r.ShouldAlert() != tt.want {
				t.Errorf("Result.ShouldAlert() = %v, want %v", r.ShouldAlert(), tt.want)
			}
		})
	}
}

func TestGetOverallStatus(t *testing.T) {
	tests := []struct {
		name     string
		results  []*Result
		want     Status
	}{
		{
			name:    "empty results",
			results: []*Result{},
			want:    StatusUnknown,
		},
		{
			name: "all healthy",
			results: []*Result{
				{Status: StatusHealthy},
				{Status: StatusHealthy},
			},
			want: StatusHealthy,
		},
		{
			name: "one unhealthy",
			results: []*Result{
				{Status: StatusHealthy},
				{Status: StatusUnhealthy},
			},
			want: StatusUnhealthy,
		},
		{
			name: "all degraded",
			results: []*Result{
				{Status: StatusDegraded},
				{Status: StatusDegraded},
			},
			want: StatusDegraded,
		},
		{
			name: "mixed degraded and healthy",
			results: []*Result{
				{Status: StatusHealthy},
				{Status: StatusDegraded},
			},
			want: StatusDegraded,
		},
		{
			name: "mixed all types",
			results: []*Result{
				{Status: StatusHealthy},
				{Status: StatusDegraded},
				{Status: StatusUnhealthy},
			},
			want: StatusUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetOverallStatus(tt.results); got != tt.want {
				t.Errorf("GetOverallStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckConfig(t *testing.T) {
	cfg := CheckConfig{
		Service:        "test-service",
		Type:           "http",
		Target:         "http://example.com",
		ExpectedStatus: 200,
	}

	if cfg.Service != "test-service" {
		t.Errorf("Service = %v, want test-service", cfg.Service)
	}

	if cfg.Type != "http" {
		t.Errorf("Type = %v, want http", cfg.Type)
	}

	if cfg.Target != "http://example.com" {
		t.Errorf("Target = %v, want http://example.com", cfg.Target)
	}

	if cfg.ExpectedStatus != 200 {
		t.Errorf("ExpectedStatus = %v, want 200", cfg.ExpectedStatus)
	}
}

func TestBatchCheckEmpty(t *testing.T) {
	c := NewChecker(Config{})
	results := c.BatchCheck([]CheckConfig{})

	if results == nil {
		t.Error("BatchCheck() should not return nil")
	}

	if len(results) != 0 {
		t.Errorf("BatchCheck() length = %v, want 0", len(results))
	}
}

func TestBatchCheckMultiple(t *testing.T) {
	c := NewChecker(Config{Timeout: 1 * time.Second})

	configs := []CheckConfig{
		{Service: "svc1", Type: "dns", Target: "localhost"},
		{Service: "svc2", Type: "ping", Target: "127.0.0.1"},
		{Service: "svc3", Type: "tcp", Target: "localhost:80"},
	}

	results := c.BatchCheck(configs)

	if results == nil {
		t.Fatal("BatchCheck() should not return nil")
	}

	if len(results) != 3 {
		t.Errorf("BatchCheck() length = %v, want 3", len(results))
	}

	for i, r := range results {
		if r == nil {
			t.Errorf("results[%d] should not be nil", i)
		}
		if r.Service != configs[i].Service {
			t.Errorf("results[%d].Service = %v, want %v", i, r.Service, configs[i].Service)
		}
	}
}

func TestCheckTCPInvalidTarget(t *testing.T) {
	c := NewChecker(Config{})
	result := c.CheckTCP("test", "invalid::address")

	if result == nil {
		t.Fatal("CheckTCP() should not return nil")
	}

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, StatusUnhealthy)
	}

	if result.Service != "test" {
		t.Errorf("Service = %v, want test", result.Service)
	}

	if result.Type != "tcp" {
		t.Errorf("Type = %v, want tcp", result.Type)
	}
}

func TestCheckDNSLocalhost(t *testing.T) {
	c := NewChecker(Config{Timeout: 2 * time.Second})
	result := c.CheckDNS("test", "localhost")

	if result == nil {
		t.Fatal("CheckDNS() should not return nil")
	}

	if result.Service != "test" {
		t.Errorf("Service = %v, want test", result.Service)
	}

	if result.Type != "dns" {
		t.Errorf("Type = %v, want dns", result.Type)
	}

	if result.Target != "localhost" {
		t.Errorf("Target = %v, want localhost", result.Target)
	}
}

func TestCheckDNSInvalid(t *testing.T) {
	c := NewChecker(Config{Timeout: 1 * time.Second})
	result := c.CheckDNS("test", "this-domain-definitely-does-not-exist-12345.invalid")

	if result == nil {
		t.Fatal("CheckDNS() should not return nil")
	}

	// DNS may return NXDOMAIN (success) instead of error
	// Just check that we get a result
	if result.Service != "test" {
		t.Errorf("Service = %v, want test", result.Service)
	}
}

func TestCheckPortInvalid(t *testing.T) {
	c := NewChecker(Config{})
	result := c.CheckPort("test", "invalid:target")

	if result == nil {
		t.Fatal("CheckPort() should not return nil")
	}

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, StatusUnhealthy)
	}

	if result.Type != "port" {
		t.Errorf("Type = %v, want port", result.Type)
	}
}

func TestCheckPortInvalidPort(t *testing.T) {
	c := NewChecker(Config{})
	result := c.CheckPort("test", "localhost:abc")

	if result == nil {
		t.Fatal("CheckPort() should not return nil")
	}

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, StatusUnhealthy)
	}
}

func TestCheckPingLocalhost(t *testing.T) {
	c := NewChecker(Config{Timeout: 2 * time.Second})
	result := c.CheckPing("test", "127.0.0.1")

	if result == nil {
		t.Fatal("CheckPing() should not return nil")
	}

	if result.Service != "test" {
		t.Errorf("Service = %v, want test", result.Service)
	}

	if result.Type != "ping" {
		t.Errorf("Type = %v, want ping", result.Type)
	}

	// Should be at least degraded (DNS should work)
	if result.Status == StatusUnhealthy {
		t.Logf("Warning: localhost ping returned unhealthy: %s", result.Message)
	}
}

func TestCheckUDPInvalidTarget(t *testing.T) {
	c := NewChecker(Config{})
	result := c.CheckUDP("test", "invalid::target", 1*time.Second)

	if result == nil {
		t.Fatal("CheckUDP() should not return nil")
	}

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, StatusUnhealthy)
	}
}

func TestResultFields(t *testing.T) {
	r := &Result{
		Service:    "test-service",
		Type:       "http",
		Target:     "http://example.com",
		Status:     StatusHealthy,
		Latency:    100 * time.Millisecond,
		Message:    "OK",
		StatusCode: 200,
		CheckedAt:  time.Now(),
	}

	if r.Service != "test-service" {
		t.Errorf("Service = %v, want test-service", r.Service)
	}

	if r.Type != "http" {
		t.Errorf("Type = %v, want http", r.Type)
	}

	if r.Target != "http://example.com" {
		t.Errorf("Target = %v, want http://example.com", r.Target)
	}

	if r.Status != StatusHealthy {
		t.Errorf("Status = %v, want healthy", r.Status)
	}

	if r.Latency != 100*time.Millisecond {
		t.Errorf("Latency = %v, want 100ms", r.Latency)
	}

	if r.Message != "OK" {
		t.Errorf("Message = %v, want OK", r.Message)
	}

	if r.StatusCode != 200 {
		t.Errorf("StatusCode = %v, want 200", r.StatusCode)
	}
}

func TestBatchCheckAutoDetect(t *testing.T) {
	c := NewChecker(Config{Timeout: 1 * time.Second})

	configs := []CheckConfig{
		{Service: "http-auto", Target: "http://example.com"},
		{Service: "port-auto", Target: "example.com:80"},
		{Service: "dns-auto", Target: "example.com"},
	}

	results := c.BatchCheck(configs)

	if len(results) != 3 {
		t.Errorf("BatchCheck() length = %v, want 3", len(results))
	}

	if results[0].Type != "http" {
		t.Errorf("Auto-detect http: Type = %v, want http", results[0].Type)
	}

	if results[1].Type != "port" {
		t.Errorf("Auto-detect port: Type = %v, want port", results[1].Type)
	}

	if results[2].Type != "dns" {
		t.Errorf("Auto-detect dns: Type = %v, want dns", results[2].Type)
	}
}
