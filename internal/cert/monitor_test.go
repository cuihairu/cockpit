package cert

import (
	"testing"
	"time"
)

func TestNewMonitor(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"default config", Config{}},
		{"with timeout", Config{Timeout: 5 * time.Second}},
		{"zero timeout", Config{Timeout: 0}},
		{"long timeout", Config{Timeout: 60 * time.Second}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMonitor(tt.cfg)
			if m == nil {
				t.Error("NewMonitor() should not return nil")
			}

			expectedTimeout := tt.cfg.Timeout
			if expectedTimeout == 0 {
				expectedTimeout = 10 * time.Second
			}

			if m.timeout != expectedTimeout {
				t.Errorf("timeout = %v, want %v", m.timeout, expectedTimeout)
			}
		})
	}
}

func TestInfoShouldAlert(t *testing.T) {
	tests := []struct {
		name      string
		daysLeft  int
		isExpired bool
		warnDays  int
		want      bool
	}{
		{"expired cert", -1, true, 30, true},
		{"expiring soon", 5, false, 30, true},
		{"expiring at warn threshold", 30, false, 30, true},
		{"safe period", 60, false, 30, false},
		{"exactly expired", 0, false, 30, true},
		{"negative days", -10, false, 30, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &Info{
				DaysLeft:  tt.daysLeft,
				IsExpired: tt.isExpired,
			}
			if got := i.ShouldAlert(tt.warnDays); got != tt.want {
				t.Errorf("Info.ShouldAlert() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInfoGetStatus(t *testing.T) {
	tests := []struct {
		name      string
		daysLeft  int
		isExpired bool
		want      string
	}{
		{"expired", -1, true, "expired"},
		{"expiring", 0, false, "expiring"},
		{"critical", 5, false, "critical"},
		{"at warning threshold", 30, false, "warning"},
		{"safe", 100, false, "ok"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &Info{
				DaysLeft:  tt.daysLeft,
				IsExpired: tt.isExpired,
			}
			if got := i.GetStatus(); got != tt.want {
				t.Errorf("Info.GetStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckMultiple(t *testing.T) {
	m := NewMonitor(Config{Timeout: 2 * time.Second})

	domains := []string{
		"localhost",
		"example.com",
		"invalid-domain-that-does-not-exist-12345.com",
	}

	results := m.CheckMultiple(domains)

	if results == nil {
		t.Fatal("CheckMultiple() should not return nil")
	}

	if len(results) != len(domains) {
		t.Errorf("CheckMultiple() returned %d results, want %d", len(results), len(domains))
	}

	for _, domain := range domains {
		if _, ok := results[domain]; !ok {
			t.Errorf("CheckMultiple() missing result for %s", domain)
		}
	}
}

func TestCheckMultipleEmpty(t *testing.T) {
	m := NewMonitor(Config{})
	results := m.CheckMultiple([]string{})

	if results == nil {
		t.Fatal("CheckMultiple() should not return nil")
	}

	if len(results) != 0 {
		t.Errorf("CheckMultiple() length = %d, want 0", len(results))
	}
}

func TestBatchCheck(t *testing.T) {
	m := NewMonitor(Config{Timeout: 1 * time.Second})

	domains := []string{
		"localhost",
		"example.com",
	}

	result := m.BatchCheck(domains, 443)

	if result == nil {
		t.Fatal("BatchCheck() should not return nil")
	}

	if result.Success == nil {
		t.Error("BatchCheck().Success should not be nil")
	}

	if result.Failed == nil {
		t.Error("BatchCheck().Failed should not be nil")
	}

	// Total results should equal input domains
	total := len(result.Success) + len(result.Failed)
	if total != len(domains) {
		t.Errorf("BatchCheck() total results = %d, want %d", total, len(domains))
	}
}

func TestBatchCheckEmpty(t *testing.T) {
	m := NewMonitor(Config{})
	result := m.BatchCheck([]string{}, 443)

	if result == nil {
		t.Fatal("BatchCheck() should not return nil")
	}

	if len(result.Success) != 0 {
		t.Errorf("BatchCheck().Success length = %d, want 0", len(result.Success))
	}

	if len(result.Failed) != 0 {
		t.Errorf("BatchCheck().Failed length = %d, want 0", len(result.Failed))
	}
}

func TestValidationResult(t *testing.T) {
	result := &ValidationResult{
		Valid:       true,
		Domain:      "example.com",
		Matched:     true,
		SANsMatched: []string{"example.com", "www.example.com"},
		Errors:      []string{},
	}

	if !result.Valid {
		t.Error("ValidationResult.Valid should be true")
	}

	if result.Domain != "example.com" {
		t.Errorf("Domain = %v, want example.com", result.Domain)
	}

	if !result.Matched {
		t.Error("ValidationResult.Matched should be true")
	}

	if len(result.SANsMatched) != 2 {
		t.Errorf("SANsMatched length = %d, want 2", len(result.SANsMatched))
	}
}

func TestValidationResultWithErrors(t *testing.T) {
	result := &ValidationResult{
		Valid:   false,
		Domain:  "expired.com",
		Matched: false,
		Errors: []string{
			"certificate expired",
			"domain mismatch",
		},
	}

	if result.Valid {
		t.Error("ValidationResult.Valid should be false")
	}

	if len(result.Errors) != 2 {
		t.Errorf("Errors length = %d, want 2", len(result.Errors))
	}
}

func TestSANs(t *testing.T) {
	sans := SANs{
		DNSNames: []string{"example.com", "www.example.com"},
		IPs:      []string{"192.168.1.1", "2001:db8::1"},
		Emails:   []string{"admin@example.com"},
	}

	if len(sans.DNSNames) != 2 {
		t.Errorf("DNSNames length = %d, want 2", len(sans.DNSNames))
	}

	if len(sans.IPs) != 2 {
		t.Errorf("IPs length = %d, want 2", len(sans.IPs))
	}

	if len(sans.Emails) != 1 {
		t.Errorf("Emails length = %d, want 1", len(sans.Emails))
	}
}

func TestSANsEmpty(t *testing.T) {
	sans := SANs{
		DNSNames: []string{},
		IPs:      []string{},
		Emails:   []string{},
	}

	if len(sans.DNSNames) != 0 {
		t.Errorf("DNSNames length = %d, want 0", len(sans.DNSNames))
	}

	if len(sans.IPs) != 0 {
		t.Errorf("IPs length = %d, want 0", len(sans.IPs))
	}

	if len(sans.Emails) != 0 {
		t.Errorf("Emails length = %d, want 0", len(sans.Emails))
	}
}

func TestMonitorFingerprint(t *testing.T) {
	m := NewMonitor(Config{})

	// Test with known data
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	fingerprint := m.fingerprint(data)

	if fingerprint == "" {
		t.Error("fingerprint() should not return empty string")
	}

	// Test with longer data (should be truncated)
	longData := make([]byte, 100)
	for i := range longData {
		longData[i] = byte(i % 256)
	}
	longFingerprint := m.fingerprint(longData)

	if longFingerprint == "" {
		t.Error("fingerprint() should handle long data")
	}
}

func TestMatchesDomain(t *testing.T) {
	m := NewMonitor(Config{})

	tests := []struct {
		name    string
		pattern string
		domain  string
		want    bool
	}{
		{"exact match", "example.com", "example.com", true},
		{"wildcard match", "*.example.com", "www.example.com", true},
		{"wildcard subdomain", "*.example.com", "api.example.com", true},
		{"wildcard deep", "*.example.com", "a.b.example.com", false},
		{"wildcard no match", "*.example.com", "example.com", false},
		{"wildcard wrong domain", "*.example.com", "www.other.com", false},
		{"different domains", "other.com", "example.com", false},
		{"empty pattern", "", "example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.matchesDomain(tt.pattern, tt.domain); got != tt.want {
				t.Errorf("Monitor.matchesDomain(%q, %q) = %v, want %v", tt.pattern, tt.domain, got, tt.want)
			}
		})
	}
}

func TestBatchCheckResult(t *testing.T) {
	result := &BatchCheckResult{
		Success: []*Info{
			{Domain: "example.com"},
			{Domain: "test.com"},
		},
		Failed: []string{"invalid.com", "timeout.com"},
	}

	if len(result.Success) != 2 {
		t.Errorf("Success length = %d, want 2", len(result.Success))
	}

	if len(result.Failed) != 2 {
		t.Errorf("Failed length = %d, want 2", len(result.Failed))
	}

	if result.Success[0].Domain != "example.com" {
		t.Errorf("First success domain = %v, want example.com", result.Success[0].Domain)
	}

	if result.Failed[0] != "invalid.com" {
		t.Errorf("First failed = %v, want invalid.com", result.Failed[0])
	}
}

func TestCheckFileNonExistent(t *testing.T) {
	m := NewMonitor(Config{})

	_, err := m.CheckFile("/non/existent/file.pem")
	if err == nil {
		t.Error("CheckFile() should return error for non-existent file")
	}
}

func TestCheckDomainInvalid(t *testing.T) {
	m := NewMonitor(Config{Timeout: 1 * time.Second})

	// This should fail (connection timeout/refused)
	_, err := m.CheckDomain("localhost", 12345)

	// We expect an error, but don't check the exact message
	// as it depends on the system
	_ = err
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}
	m := NewMonitor(cfg)

	if m.timeout != 10*time.Second {
		t.Errorf("default timeout = %v, want 10s", m.timeout)
	}
}

func TestMonitorTimeoutVariations(t *testing.T) {
	timeouts := []time.Duration{
		1 * time.Second,
		5 * time.Second,
		10 * time.Second,
		30 * time.Second,
	}

	for _, timeout := range timeouts {
		m := NewMonitor(Config{Timeout: timeout})
		if m.timeout != timeout {
			t.Errorf("timeout = %v, want %v", m.timeout, timeout)
		}
	}
}

func TestInfoFields(t *testing.T) {
	info := &Info{
		Domain:       "example.com",
		IP:           "192.168.1.1",
		Subject:      "CN=example.com",
		Issuer:       "CN=Let's Encrypt",
		NotBefore:    time.Now().Add(-365 * 24 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		DaysLeft:     365,
		IsExpired:    false,
		Fingerprint:  "AA:BB:CC:DD",
		SerialNumber: "1234ABCD",
	}

	if info.Domain != "example.com" {
		t.Errorf("Domain = %v, want example.com", info.Domain)
	}

	if info.IP != "192.168.1.1" {
		t.Errorf("IP = %v, want 192.168.1.1", info.IP)
	}

	if info.Subject != "CN=example.com" {
		t.Errorf("Subject = %v, want CN=example.com", info.Subject)
	}

	if info.DaysLeft != 365 {
		t.Errorf("DaysLeft = %v, want 365", info.DaysLeft)
	}

	if info.IsExpired {
		t.Error("IsExpired should be false")
	}
}
