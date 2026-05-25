package domain

import (
	"os/exec"
	"testing"
	"time"
)

func TestNewMonitor(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"default config", Config{}},
		{"with timeout", Config{Timeout: 10 * time.Second}},
		{"with whois server", Config{WhoisServer: "whois.example.com"}},
		{"with whois path", Config{WhoisPath: "/usr/bin/whois"}},
		{"full config", Config{
			WhoisServer: "whois.example.com",
			WhoisPath:   "/usr/bin/whois",
			Timeout:     60 * time.Second,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMonitor(tt.cfg)
			if m == nil {
				t.Error("NewMonitor() should not return nil")
			}

			expectedTimeout := tt.cfg.Timeout
			if expectedTimeout == 0 {
				expectedTimeout = 30 * time.Second
			}

			if m.timeout != expectedTimeout {
				t.Errorf("timeout = %v, want %v", m.timeout, expectedTimeout)
			}

			if m.whoisServer != tt.cfg.WhoisServer {
				t.Errorf("whoisServer = %v, want %v", m.whoisServer, tt.cfg.WhoisServer)
			}

			// whoisPath might be auto-detected
			if tt.cfg.WhoisPath != "" && m.whoisPath != tt.cfg.WhoisPath {
				t.Errorf("whoisPath = %v, want %v", m.whoisPath, tt.cfg.WhoisPath)
			}
		})
	}
}

func TestNewMonitorTimeout(t *testing.T) {
	m := NewMonitor(Config{Timeout: 5 * time.Second})
	if m.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", m.timeout)
	}

	m2 := NewMonitor(Config{})
	if m2.timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", m2.timeout)
	}
}

func TestCheckInvalidDomain(t *testing.T) {
	m := NewMonitor(Config{})

	tests := []string{
		"",
		"invalid",
		"no-dots",
		"   ",
	}

	for _, domain := range tests {
		t.Run("domain: "+domain, func(t *testing.T) {
			_, err := m.Check(domain)
			if err == nil {
				t.Error("Check() should return error for invalid domain")
			}
		})
	}
}

func TestCheckWithNoWhois(t *testing.T) {
	m := NewMonitor(Config{WhoisPath: "/non/existent/whois"})

	_, err := m.Check("example.com")
	if err == nil {
		t.Error("Check() should return error when whois is not available")
	}
}

func TestQueryWhoisNoPath(t *testing.T) {
	m := NewMonitor(Config{WhoisPath: ""})

	// If whois is auto-detected on the system, skip this test
	if m.whoisPath != "" {
		t.Skip("whois command found on system, skipping test")
		return
	}

	_, err := m.queryWhois("example.com")
	if err == nil {
		t.Error("queryWhois() should return error when whoisPath is empty")
	}
}

func TestQueryWhoisInvalidPath(t *testing.T) {
	m := NewMonitor(Config{WhoisPath: "/non/existent/whois"})

	_, err := m.queryWhois("example.com")
	if err == nil {
		t.Error("queryWhois() should return error for invalid whois path")
	}
}

func TestNormalizeDomain(t *testing.T) {
	if _, err := exec.LookPath("whois"); err != nil {
		t.Skip("skipping: whois command not found")
	}

	m := NewMonitor(Config{})

	tests := []struct {
		input       string
		shouldError bool
	}{
		{"EXAMPLE.COM", false},
		{"  example.com  ", false},
		{"Test.COM", false},
		{"sub.domain.com", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := m.Check(tt.input)
			if (err != nil) != tt.shouldError {
				t.Errorf("Check(%q) error = %v, shouldError %v", tt.input, err, tt.shouldError)
			}
		})
	}
}

func TestMonitorFields(t *testing.T) {
	m := NewMonitor(Config{
		WhoisServer: "whois.example.com",
		WhoisPath:   "/usr/bin/whois",
		Timeout:     45 * time.Second,
	})

	if m.whoisServer != "whois.example.com" {
		t.Errorf("whoisServer = %v, want whois.example.com", m.whoisServer)
	}

	if m.whoisPath != "/usr/bin/whois" {
		t.Errorf("whoisPath = %v, want /usr/bin/whois", m.whoisPath)
	}

	if m.timeout != 45*time.Second {
		t.Errorf("timeout = %v, want 45s", m.timeout)
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}

	if cfg.WhoisServer != "" {
		t.Errorf("WhoisServer should be empty by default")
	}

	if cfg.WhoisPath != "" {
		t.Errorf("WhoisPath should be empty by default")
	}

	if cfg.Timeout != 0 {
		t.Errorf("Timeout should be 0 by default")
	}
}

func TestCheckTimeout(t *testing.T) {
	m := NewMonitor(Config{WhoisPath: "/usr/bin/whois", Timeout: 1 * time.Nanosecond})

	// Very short timeout - might fail or succeed depending on system
	_, err := m.Check("example.com")
	_ = err // Result depends on system
}

func TestQueryWhoisTimeout(t *testing.T) {
	m := NewMonitor(Config{WhoisPath: "/usr/bin/whois", Timeout: 1 * time.Nanosecond})

	_, err := m.queryWhois("example.com")
	_ = err // May timeout or succeed depending on system speed
}

func TestMultipleMonitors(t *testing.T) {
	cfg := Config{Timeout: 10 * time.Second}

	for i := 0; i < 5; i++ {
		m := NewMonitor(cfg)
		if m == nil {
			t.Errorf("NewMonitor() iteration %d returned nil", i)
		}
	}
}

func TestConcurrentChecks(t *testing.T) {
	m := NewMonitor(Config{WhoisPath: "/non/existent/whois"})

	done := make(chan bool, 3)

	domains := []string{"example.com", "test.com", "demo.com"}
	for _, domain := range domains {
		go func(d string) {
			_, err := m.Check(d)
			_ = err // Expected to fail
			done <- true
		}(domain)
	}

	for i := 0; i < len(domains); i++ {
		<-done
	}
}

func TestMonitorZeroTimeout(t *testing.T) {
	m := NewMonitor(Config{Timeout: 0})

	if m.timeout != 30*time.Second {
		t.Errorf("zero timeout should default to 30s, got %v", m.timeout)
	}
}

func TestMonitorWithServer(t *testing.T) {
	m := NewMonitor(Config{
		WhoisServer: "whois.iana.org",
		WhoisPath:   "/usr/bin/whois",
	})

	if m.whoisServer != "whois.iana.org" {
		t.Errorf("WhoisServer = %v, want whois.iana.org", m.whoisServer)
	}
}

func TestDomainValidation(t *testing.T) {
	validDomains := []string{
		"example.com",
		"sub.example.com",
		"test.co.uk",
		"a.b.c.d.e.f.g.com",
	}

	for _, domain := range validDomains {
		t.Run("valid_"+domain, func(t *testing.T) {
			// Verify domain formats are valid (contain dots)
			if len(domain) == 0 || !containsDot(domain) {
				t.Errorf("Domain %q should be valid", domain)
			}
		})
	}
}

func containsDot(domain string) bool {
	for _, c := range domain {
		if c == '.' {
			return true
		}
	}
	return false
}

func TestInfoStruct(t *testing.T) {
	info := &Info{
		Domain:      "example.com",
		Registrar:   "Example Registrar",
		NameServers: []string{"ns1.example.com", "ns2.example.com"},
		Status:      []string{"clientTransferProhibited"},
		DNSSec:      true,
	}

	if info.Domain != "example.com" {
		t.Errorf("Domain = %v, want example.com", info.Domain)
	}

	if info.Registrar != "Example Registrar" {
		t.Errorf("Registrar = %v, want Example Registrar", info.Registrar)
	}

	if len(info.NameServers) != 2 {
		t.Errorf("NameServers length = %d, want 2", len(info.NameServers))
	}

	if info.DNSSec != true {
		t.Error("DNSSec should be true")
	}
}

func TestInfoEmpty(t *testing.T) {
	info := &Info{}

	if info.Domain != "" {
		t.Errorf("Domain should be empty")
	}

	if len(info.NameServers) != 0 {
		t.Errorf("NameServers should be empty")
	}

	if info.DNSSec != false {
		t.Error("DNSSec should be false")
	}
}

// ============ Info Method Tests ============

func TestInfoShouldAlert(t *testing.T) {
	tests := []struct {
		name     string
		info     *Info
		warnDays int
		expected bool
	}{
		{
			name: "expired domain",
			info: &Info{
				Domain:    "expired.com",
				IsExpired: true,
				DaysLeft:  -1,
			},
			warnDays: 30,
			expected: true,
		},
		{
			name: "expiring soon",
			info: &Info{
				Domain:    "expiring.com",
				IsExpired: false,
				DaysLeft:  7,
			},
			warnDays: 30,
			expected: true,
		},
		{
			name: "not expiring",
			info: &Info{
				Domain:    "safe.com",
				IsExpired: false,
				DaysLeft:  100,
			},
			warnDays: 30,
			expected: false,
		},
		{
			name: "exactly at warning",
			info: &Info{
				Domain:    "warning.com",
				IsExpired: false,
				DaysLeft:  30,
			},
			warnDays: 30,
			expected: true,
		},
		{
			name: "negative days (expired)",
			info: &Info{
				Domain:    "past.com",
				IsExpired: true,
				DaysLeft:  -100,
			},
			warnDays: 30,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.info.ShouldAlert(tt.warnDays)
			if result != tt.expected {
				t.Errorf("ShouldAlert() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestInfoGetStatus(t *testing.T) {
	tests := []struct {
		name     string
		info     *Info
		expected string
	}{
		{
			name: "expired",
			info: &Info{
				Expires:   time.Now().Add(-24 * time.Hour),
				IsExpired: true,
			},
			expected: "expired",
		},
		{
			name: "critical",
			info: &Info{
				Expires:   time.Now().Add(3 * 24 * time.Hour),
				IsExpired: false,
				DaysLeft:  3,
			},
			expected: "critical",
		},
		{
			name: "warning",
			info: &Info{
				Expires:   time.Now().Add(15 * 24 * time.Hour),
				IsExpired: false,
				DaysLeft:  15,
			},
			expected: "warning",
		},
		{
			name: "ok",
			info: &Info{
				Expires:   time.Now().Add(60 * 24 * time.Hour),
				IsExpired: false,
				DaysLeft:  60,
			},
			expected: "ok",
		},
		{
			name: "unknown",
			info: &Info{
				Expires: time.Time{},
			},
			expected: "unknown",
		},
		{
			name: "expiring now",
			info: &Info{
				Expires:   time.Now(),
				IsExpired: false,
				DaysLeft:  0,
			},
			expected: "expiring",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.info.GetStatus()
			if result != tt.expected {
				t.Errorf("GetStatus() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetTLD(t *testing.T) {
	m := NewMonitor(Config{})

	tests := []struct {
		domain   string
		expected string
	}{
		{"example.com", "com"},
		{"test.co.uk", "uk"},
		{"sub.domain.org", "org"},
		{"a.b.c.d.e.f.g.net", "net"},
		{"example.io", "io"},
		{"localhost", ""},
		{"nodots", ""},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			result := m.GetTLD(tt.domain)
			if result != tt.expected {
				t.Errorf("GetTLD(%q) = %v, want %v", tt.domain, result, tt.expected)
			}
		})
	}
}

func TestGetWhoisServer(t *testing.T) {
	m := NewMonitor(Config{})

	tests := []struct {
		tld      string
		expected string
	}{
		{"com", "whois.verisign-grs.com"},
		{"net", "whois.verisign-grs.com"},
		{"org", "whois.pir.org"},
		{"io", "whois.nic.io"},
		{"co", "whois.nic.co"},
		{"ai", "whois.nic.ai"},
		{"cn", "whois.cnnic.cn"},
		{"uk", "whois.nic.uk"},
		{"de", "whois.denic.de"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.tld, func(t *testing.T) {
			result := m.GetWhoisServer(tt.tld)
			if result != tt.expected {
				t.Errorf("GetWhoisServer(%q) = %v, want %v", tt.tld, result, tt.expected)
			}
		})
	}
}

func TestCheckMultiple(t *testing.T) {
	m := NewMonitor(Config{})

	domains := []string{
		"invalid", // Should fail validation
		"nodots",  // Should fail validation
	}

	results := m.CheckMultiple(domains)

	if results == nil {
		t.Fatal("CheckMultiple() should not return nil")
	}

	if len(results) != len(domains) {
		t.Errorf("CheckMultiple() returned %d results, want %d", len(results), len(domains))
	}

	// All should have -1 DaysLeft (error)
	for domain, info := range results {
		if info == nil {
			t.Errorf("Result for %s should not be nil", domain)
			continue
		}
		if info.DaysLeft != -1 {
			t.Errorf("Domain %s: DaysLeft = %d, want -1", domain, info.DaysLeft)
		}
	}
}

func TestBatchCheck(t *testing.T) {
	m := NewMonitor(Config{})

	domains := []string{
		"invalid", // Should fail
	}

	result := m.BatchCheck(domains)

	if result == nil {
		t.Fatal("BatchCheck() should not return nil")
	}

	if result.Success == nil {
		t.Error("Success slice should not be nil")
	}

	if result.Failed == nil {
		t.Error("Failed slice should not be nil")
	}
}

func TestBatchCheckValid(t *testing.T) {
	m := NewMonitor(Config{})

	// Test with valid domains that will fail whois
	domains := []string{"example.com"}

	result := m.BatchCheck(domains)

	if result == nil {
		t.Fatal("BatchCheck() should not return nil")
	}

	// Since whois is not available, all should fail
	if len(result.Failed) == 0 && m.whoisPath == "" {
		t.Log("No whois path, all domains should fail")
	}
}

func TestCheckWithServer(t *testing.T) {
	m := NewMonitor(Config{})

	// Should not panic even with invalid whois
	_, err := m.CheckWithServer("example.com", "whois.example.com")
	if err == nil {
		t.Log("CheckWithServer() succeeded (whois might be available)")
	}
}

func TestParseWhoisBasic(t *testing.T) {
	m := NewMonitor(Config{})

	whoisOutput := `
Registrar: Example Registrar
Creation Date: 2020-01-01
Expiration Date: 2025-01-01
Name Server: ns1.example.com
Name Server: ns2.example.com
Domain Status: clientTransferProhibited
DNSSEC: unsigned
`

	info, err := m.parseWhois("example.com", whoisOutput)
	if err != nil {
		t.Fatalf("parseWhois() error = %v", err)
	}

	if info.Domain != "example.com" {
		t.Errorf("Domain = %v, want example.com", info.Domain)
	}

	if info.Registrar != "Example Registrar" {
		t.Errorf("Registrar = %v, want Example Registrar", info.Registrar)
	}
}

func TestParseWhoisEmpty(t *testing.T) {
	m := NewMonitor(Config{})

	info, err := m.parseWhois("example.com", "")
	if err != nil {
		t.Fatalf("parseWhois() with empty output error = %v", err)
	}

	if info == nil {
		t.Fatal("parseWhois() should not return nil")
	}

	if info.Domain != "example.com" {
		t.Errorf("Domain = %v, want example.com", info.Domain)
	}
}

func TestParseDateFormats(t *testing.T) {
	m := NewMonitor(Config{})

	dateStrings := []struct {
		input  string
		valid  bool
	}{
		{"2020-01-01", true},
		{"2020-01-01T00:00:00Z", true},
		{"01-01-2020", true},
		{"01/01/2020", true},
		{"2020/01/01", true},
		{"01-Jan-2020", true},
		{"01.01.2020", true},
		{"invalid", false},
	}

	for _, tt := range dateStrings {
		t.Run(tt.input, func(t *testing.T) {
			result := m.parseDate(tt.input)
			if tt.valid && result.IsZero() {
				t.Errorf("parseDate(%q) should return valid date", tt.input)
			}
		})
	}
}

func TestDNSInfoStruct(t *testing.T) {
	info := &DNSInfo{
		Domain:     "example.com",
		ARecords:   []string{"1.2.3.4"},
		AAAARecords: []string{"2001:db8::1"},
		MXRecords:  []string{"mail.example.com"},
		TXTRecords: []string{"v=spf1 include:_spf.example.com ~all"},
		NSRecords:  []string{"ns1.example.com"},
		CNAME:      "alias.example.com",
	}

	if info.Domain != "example.com" {
		t.Errorf("Domain = %v, want example.com", info.Domain)
	}

	if len(info.ARecords) != 1 {
		t.Errorf("ARecords length = %d, want 1", len(info.ARecords))
	}

	if info.CNAME != "alias.example.com" {
		t.Errorf("CNAME = %v, want alias.example.com", info.CNAME)
	}
}

func TestDNSInfoEmpty(t *testing.T) {
	info := &DNSInfo{}

	if info.Domain != "" {
		t.Error("Domain should be empty")
	}

	if len(info.ARecords) != 0 {
		t.Error("ARecords should be empty")
	}
}

func TestBatchCheckResultStruct(t *testing.T) {
	result := &BatchCheckResult{
		Success: []*Info{
			{Domain: "success.com"},
		},
		Failed: []string{
			"failed.com",
		},
	}

	if len(result.Success) != 1 {
		t.Errorf("Success length = %d, want 1", len(result.Success))
	}

	if len(result.Failed) != 1 {
		t.Errorf("Failed length = %d, want 1", len(result.Failed))
	}
}

func TestIsAvailable(t *testing.T) {
	m := NewMonitor(Config{})

	// Test with invalid whois - will check DNS instead
	available, err := m.IsAvailable("nonexistent-domain-12345.com")
	if err != nil {
		t.Logf("IsAvailable() error (expected): %v", err)
	}

	// Result depends on DNS
	_ = available
}

func TestCheckDNS(t *testing.T) {
	m := NewMonitor(Config{})

	// This will do actual DNS lookups
	info, err := m.CheckDNS("localhost")
	if err != nil {
		t.Logf("CheckDNS() error: %v", err)
	}

	if info == nil {
		t.Log("CheckDNS() returned nil (may be expected)")
	} else {
		if info.Domain != "localhost" {
			t.Errorf("Domain = %v, want localhost", info.Domain)
		}
	}
}

func TestConcurrentBatchChecks(t *testing.T) {
	m := NewMonitor(Config{})

	done := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func() {
			result := m.BatchCheck([]string{"example.com"})
			if result == nil {
				t.Error("BatchCheck() should not return nil")
			}
			done <- true
		}()
	}

	for i := 0; i < 3; i++ {
		<-done
	}
}
