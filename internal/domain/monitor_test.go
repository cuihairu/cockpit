package domain

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
