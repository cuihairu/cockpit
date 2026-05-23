package domain

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Info domain information
type Info struct {
	Domain       string    `json:"domain"`
	Registrar    string    `json:"registrar"`
	Registered   time.Time `json:"registered"`
	Expires      time.Time `json:"expires"`
	DaysLeft     int       `json:"days_left"`
	IsExpired    bool      `json:"is_expired"`
	NameServers  []string  `json:"name_servers"`
	Status       []string  `json:"status"`
	DNSSec       bool      `json:"dnssec"`
}

// Monitor domain monitor
type Monitor struct {
	whoisServer string
	whoisPath   string
	timeout     time.Duration
}

// Config monitor configuration
type Config struct {
	WhoisServer string
	WhoisPath   string
	Timeout     time.Duration
}

// NewMonitor creates domain monitor
func NewMonitor(cfg Config) *Monitor {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	whoisPath := cfg.WhoisPath
	if whoisPath == "" {
		// Try to find whois command
		if path, err := exec.LookPath("whois"); err == nil {
			whoisPath = path
		}
	}

	return &Monitor{
		whoisServer: cfg.WhoisServer,
		whoisPath:   whoisPath,
		timeout:     timeout,
	}
}

// Check checks domain expiration
func (m *Monitor) Check(domain string) (*Info, error) {
	// Normalize domain
	domain = strings.ToLower(strings.TrimSpace(domain))
	if !strings.Contains(domain, ".") {
		return nil, fmt.Errorf("invalid domain: %s", domain)
	}

	text, err := m.queryWhois(domain)
	if err != nil {
		return nil, fmt.Errorf("whois query: %w", err)
	}

	return m.parseWhois(domain, text)
}

// queryWhois queries whois server
func (m *Monitor) queryWhois(domain string) (string, error) {
	if m.whoisPath == "" {
		return "", fmt.Errorf("whois command not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	args := []string{domain}
	if m.whoisServer != "" {
		args = append(args, "-h", m.whoisServer)
	}

	cmd := exec.CommandContext(ctx, m.whoisPath, args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("execute whois: %w", err)
	}

	return string(output), nil
}

// parseWhois parses whois output
func (m *Monitor) parseWhois(domain, text string) (*Info, error) {
	info := &Info{
		Domain:     domain,
		NameServers: []string{},
		Status:     []string{},
	}

	lines := bufio.NewScanner(strings.NewReader(text))
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if line == "" {
			continue
		}

		// Split by first colon
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch key {
		case "registrar", "sponsoring registrar":
			info.Registrar = value
		case "creation date", "created", "registration time", "registered":
			info.Registered = m.parseDate(value)
		case "expiration date", "expires", "expiry date", "registry expiry date":
			info.Expires = m.parseDate(value)
		case "name server", "nserver", "nameserver":
			if value != "" {
				info.NameServers = append(info.NameServers, strings.ToLower(value))
			}
		case "domain status", "status", "domainstatus":
			if value != "" {
				info.Status = append(info.Status, value)
			}
		case "dnssec":
			info.DNSSec = strings.Contains(strings.ToLower(value), "signed")
		}
	}

	// Calculate days left
	if !info.Expires.IsZero() {
		info.DaysLeft = int(time.Until(info.Expires).Hours() / 24)
		info.IsExpired = time.Now().After(info.Expires)
	}

	return info, nil
}

// Date patterns for whois parsing
var datePatterns = []struct {
	pattern string
	layout  string
}{
	{`(\d{4}-\d{2}-\d{2})`, "2006-01-02"},
	{`(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)`, time.RFC3339},
	{`(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})`, "2006-01-02T15:04:05"},
	{`(\d{2}-\d{2}-\d{4})`, "02-01-2006"},
	{`(\d{2}/\d{2}/\d{4})`, "02/01/2006"},
	{`(\d{4}/\d{2}/\d{2})`, "2006/01/02"},
	{`(\d{2}-\w{3}-\d{4})`, "02-Jan-2006"},
	{`(\d{2}.\d{2}.\d{4})`, "02.01.2006"},
}

// parseDate parses date from various formats
func (m *Monitor) parseDate(s string) time.Time {
	s = strings.TrimSpace(s)

	// Try each pattern
	for _, p := range datePatterns {
		re := regexp.MustCompile(p.pattern)
		matches := re.FindStringSubmatch(s)
		if len(matches) > 1 {
			if t, err := time.Parse(p.layout, matches[1]); err == nil {
				return t
			}
		}
	}

	return time.Time{}
}

// CheckMultiple checks multiple domains
func (m *Monitor) CheckMultiple(domains []string) map[string]*Info {
	results := make(map[string]*Info)
	for _, domain := range domains {
		info, err := m.Check(domain)
		if err != nil {
			// Store error info
			results[domain] = &Info{
				Domain:   domain,
				DaysLeft: -1,
			}
		} else {
			results[domain] = info
		}
	}
	return results
}

// ShouldAlert checks if domain should trigger alert
func (i *Info) ShouldAlert(warnDays int) bool {
	if i.IsExpired {
		return true
	}
	return i.DaysLeft <= warnDays && i.DaysLeft >= 0
}

// GetStatus returns human-readable status
func (i *Info) GetStatus() string {
	if i.Expires.IsZero() {
		return "unknown"
	}
	if i.IsExpired {
		return "expired"
	}
	if i.DaysLeft <= 0 {
		return "expiring"
	}
	if i.DaysLeft <= 7 {
		return "critical"
	}
	if i.DaysLeft <= 30 {
		return "warning"
	}
	return "ok"
}

// GetTLD gets top-level domain
func (m *Monitor) GetTLD(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return ""
}

// GetWhoisServer returns whois server for TLD
func (m *Monitor) GetWhoisServer(tld string) string {
	// Common whois servers
	servers := map[string]string{
		"com": "whois.verisign-grs.com",
		"net": "whois.verisign-grs.com",
		"org": "whois.pir.org",
		"info": "whois.afilias.net",
		"biz": "whois.neulevel.biz",
		"name": "whois.nic.name",
		"io":   "whois.nic.io",
		"co":   "whois.nic.co",
		"ai":   "whois.nic.ai",
		"tv":   "whois.nic.tv",
		"me":   "whois.nic.me",
		"xyz":  "whois.nic.xyz",
		"online": "whois.nic.online",
		"site":   "whois.nic.site",
		"club":   "whois.nic.club",
		"cn":     "whois.cnnic.cn",
		"uk":     "whois.nic.uk",
		"de":     "whois.denic.de",
		"fr":     "whois.nic.fr",
		"jp":     "whois.jprs.jp",
		"kr":     "whois.kr",
		"ru":     "whois.tcinet.ru",
		"br":     "whois.registro.br",
		"au":     "whois.auda.org.au",
		"ca":     "whois.cira.ca",
		"in":     "whois.registry.in",
	}

	if server, ok := servers[tld]; ok {
		return server
	}
	return ""
}

// CheckWithServer checks domain using specific whois server
func (m *Monitor) CheckWithServer(domain, server string) (*Info, error) {
	oldServer := m.whoisServer
	m.whoisServer = server
	defer func() { m.whoisServer = oldServer }()

	return m.Check(domain)
}

// BatchCheckResult batch check result
type BatchCheckResult struct {
	Success []*Info  `json:"success"`
	Failed  []string `json:"failed"`
}

// BatchCheck checks multiple domains and returns separated results
func (m *Monitor) BatchCheck(domains []string) *BatchCheckResult {
	result := &BatchCheckResult{
		Success: make([]*Info, 0),
		Failed:  make([]string, 0),
	}

	for _, domain := range domains {
		info, err := m.Check(domain)
		if err != nil {
			result.Failed = append(result.Failed, domain)
		} else {
			result.Success = append(result.Success, info)
		}
	}

	return result
}

// ResolveDNS resolves DNS for domain
func (m *Monitor) ResolveDNS(domain string) ([]string, error) {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return nil, fmt.Errorf("dns lookup: %w", err)
	}

	result := make([]string, len(ips))
	for i, ip := range ips {
		result[i] = ip.String()
	}

	return result, nil
}

// CheckDNS checks DNS records for domain
type DNSInfo struct {
	Domain     string   `json:"domain"`
	ARecords   []string `json:"a_records"`
	AAAARecords []string `json:"aaaa_records"`
	MXRecords  []string `json:"mx_records"`
	TXTRecords []string `json:"txt_records"`
	NSRecords  []string `json:"ns_records"`
	CNAME      string   `json:"cname"`
}

// CheckDNS checks DNS records
func (m *Monitor) CheckDNS(domain string) (*DNSInfo, error) {
	info := &DNSInfo{
		Domain: domain,
	}

	// A records
	aRecs, err := net.LookupIP(domain)
	if err == nil {
		for _, ip := range aRecs {
			if ipv4 := ip.To4(); ipv4 != nil {
				info.ARecords = append(info.ARecords, ipv4.String())
			} else {
				info.AAAARecords = append(info.AAAARecords, ip.String())
			}
		}
	}

	// MX records
	mxRecs, err := net.LookupMX(domain)
	if err == nil {
		for _, mx := range mxRecs {
			info.MXRecords = append(info.MXRecords, mx.Host)
		}
	}

	// TXT records
	txtRecs, err := net.LookupTXT(domain)
	if err == nil {
		info.TXTRecords = txtRecs
	}

	// NS records
	nsRecs, err := net.LookupNS(domain)
	if err == nil {
		for _, ns := range nsRecs {
			info.NSRecords = append(info.NSRecords, ns.Host)
		}
	}

	// CNAME record
	cname, err := net.LookupCNAME(domain)
	if err == nil && cname != domain+"." {
		info.CNAME = cname
	}

	return info, nil
}

// IsAvailable checks if domain is available
func (m *Monitor) IsAvailable(domain string) (bool, error) {
	_, err := m.Check(domain)
	if err != nil {
		// If whois fails, domain might be available
		// Check DNS as fallback
		ips, err := net.LookupIP(domain)
		if err != nil {
			return true, nil // No DNS = likely available
		}
		return len(ips) == 0, nil
	}

	// Domain has whois info = registered
	return false, nil
}
