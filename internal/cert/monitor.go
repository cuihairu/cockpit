package cert

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// Info certificate information
type Info struct {
	Domain      string    `json:"domain"`
	IP          string    `json:"ip,omitempty"`
	Subject     string    `json:"subject"`
	Issuer      string    `json:"issuer"`
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
	DaysLeft    int       `json:"days_left"`
	IsExpired   bool      `json:"is_expired"`
	Fingerprint string    `json:"fingerprint"`
	SerialNumber string   `json:"serial_number"`
}

// Monitor certificate monitor
type Monitor struct {
	timeout time.Duration
}

// Config monitor configuration
type Config struct {
	Timeout time.Duration
}

// NewMonitor creates certificate monitor
func NewMonitor(cfg Config) *Monitor {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &Monitor{
		timeout: timeout,
	}
}

// CheckDomain checks certificate for domain
func (m *Monitor) CheckDomain(domain string, port int) (*Info, error) {
	if port == 0 {
		port = 443
	}

	address := fmt.Sprintf("%s:%d", domain, port)

	// Create connection with timeout
	dialer := &net.Dialer{
		Timeout: m.timeout,
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", address, err)
	}
	defer conn.Close()

	// Get peer certificates
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, fmt.Errorf("no certificates found")
	}

	cert := state.PeerCertificates[0]
	return m.parseCertInfo(cert, domain, conn.RemoteAddr().String()), nil
}

// CheckFile checks certificate from file
func (m *Monitor) CheckFile(path string) (*Info, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Try PEM format first
	block, _ := pem.Decode(data)
	if block != nil && block.Type == "CERTIFICATE" {
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse certificate: %w", err)
		}
		return m.parseCertInfo(cert, path, ""), nil
	}

	// Try DER format
	cert, err := x509.ParseCertificate(data)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	return m.parseCertInfo(cert, path, ""), nil
}

// CheckDomainChain checks full certificate chain for domain
func (m *Monitor) CheckDomainChain(domain string, port int) ([]*Info, error) {
	if port == 0 {
		port = 443
	}

	address := fmt.Sprintf("%s:%d", domain, port)

	dialer := &net.Dialer{
		Timeout: m.timeout,
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", address, err)
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, fmt.Errorf("no certificates found")
	}

	result := make([]*Info, len(state.PeerCertificates))
	for i, cert := range state.PeerCertificates {
		result[i] = m.parseCertInfo(cert, domain, conn.RemoteAddr().String())
	}

	return result, nil
}

// parseCertInfo parses certificate to Info
func (m *Monitor) parseCertInfo(cert *x509.Certificate, domain, remoteAddr string) *Info {
	now := time.Now()
	daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)

	// Extract IP from remote address if available
	ip := ""
	if remoteAddr != "" {
		ip, _, _ = net.SplitHostPort(remoteAddr)
	}

	return &Info{
		Domain:       domain,
		IP:           ip,
		Subject:      cert.Subject.CommonName,
		Issuer:       cert.Issuer.CommonName,
		NotBefore:    cert.NotBefore,
		NotAfter:     cert.NotAfter,
		DaysLeft:     daysLeft,
		IsExpired:    now.After(cert.NotAfter),
		Fingerprint:  m.fingerprint(cert.Raw),
		SerialNumber: fmt.Sprintf("%X", cert.SerialNumber),
	}
}

// fingerprint calculates certificate fingerprint
func (m *Monitor) fingerprint(data []byte) string {
	// Simple hex representation
	hex := "0123456789ABCDEF"
	if len(data) > 20 {
		data = data[:20]
	}
	result := make([]byte, len(data)*3)
	for i, b := range data {
		result[i*3] = hex[b>>4]
		result[i*3+1] = hex[b&0xF]
		if i < len(data)-1 {
			result[i*3+2] = ':'
		}
	}
	return strings.TrimRight(string(result), ":")
}

// CheckMultiple checks multiple domains
func (m *Monitor) CheckMultiple(domains []string) map[string]*Info {
	results := make(map[string]*Info)
	for _, domain := range domains {
		info, err := m.CheckDomain(domain, 0)
		if err != nil {
			// Store error info
			results[domain] = &Info{
				Domain: domain,
				NotAfter: time.Time{},
				DaysLeft: -1,
			}
		} else {
			results[domain] = info
		}
	}
	return results
}

// ShouldAlert checks if certificate should trigger alert
func (i *Info) ShouldAlert(warnDays int) bool {
	if i.IsExpired {
		return true
	}
	return i.DaysLeft <= warnDays && i.DaysLeft >= 0
}

// GetStatus returns human-readable status
func (i *Info) GetStatus() string {
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

// SANs represents Subject Alternative Names
type SANs struct {
	DNSNames []string
	IPs      []string
	Emails   []string
}

// GetSANs gets alternative names from certificate
func (m *Monitor) GetSANs(cert *x509.Certificate) SANs {
	sans := SANs{
		DNSNames: cert.DNSNames,
	}

	for _, ip := range cert.IPAddresses {
		sans.IPs = append(sans.IPs, ip.String())
	}

	for _, email := range cert.EmailAddresses {
		sans.Emails = append(sans.Emails, email)
	}

	return sans
}

// ValidationResult validation result
type ValidationResult struct {
	Valid       bool     `json:"valid"`
	Domain      string   `json:"domain"`
	Matched     bool     `json:"matched"`
	SANsMatched []string `json:"sans_matched"`
	Errors      []string `json:"errors"`
}

// ValidateForDomain validates certificate for specific domain
func (m *Monitor) ValidateForDomain(cert *x509.Certificate, domain string) *ValidationResult {
	result := &ValidationResult{
		Domain: domain,
		Valid:  true,
	}

	// Check if common name matches
	cnMatch := strings.EqualFold(cert.Subject.CommonName, domain)
	result.Matched = cnMatch

	// Check SANs
	sans := m.GetSANs(cert)
	for _, dns := range sans.DNSNames {
		if m.matchesDomain(dns, domain) {
			result.Matched = true
			result.SANsMatched = append(result.SANsMatched, dns)
		}
	}

	// Check if expired
	if time.Now().After(cert.NotAfter) {
		result.Valid = false
		result.Errors = append(result.Errors, "certificate expired")
	}

	// Check if not yet valid
	if time.Now().Before(cert.NotBefore) {
		result.Valid = false
		result.Errors = append(result.Errors, "certificate not yet valid")
	}

	return result
}

// matchesDomain checks if certificate pattern matches domain
func (m *Monitor) matchesDomain(pattern, domain string) bool {
	if pattern == domain {
		return true
	}

	// Handle wildcard certificates
	if strings.HasPrefix(pattern, "*.") {
		wildcardDomain := pattern[2:]
		domainParts := strings.Split(domain, ".")
		if len(domainParts) >= 2 {
			domainBase := strings.Join(domainParts[1:], ".")
			return wildcardDomain == domainBase
		}
	}

	return false
}

// CheckMailServer checks mail server certificate
func (m *Monitor) CheckMailServer(domain string, port int) (*Info, error) {
	if port == 0 {
		port = 25 // SMTP
	}

	address := fmt.Sprintf("%s:%d", domain, port)

	dialer := &net.Dialer{
		Timeout: m.timeout,
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", address, err)
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, fmt.Errorf("no certificates found")
	}

	cert := state.PeerCertificates[0]
	return m.parseCertInfo(cert, domain, conn.RemoteAddr().String()), nil
}

// BatchCheckResult batch check result
type BatchCheckResult struct {
	Success []*Info  `json:"success"`
	Failed  []string `json:"failed"`
}

// BatchCheck checks multiple domains and returns separated results
func (m *Monitor) BatchCheck(domains []string, port int) *BatchCheckResult {
	result := &BatchCheckResult{
		Success: make([]*Info, 0),
		Failed:  make([]string, 0),
	}

	for _, domain := range domains {
		info, err := m.CheckDomain(domain, port)
		if err != nil {
			result.Failed = append(result.Failed, domain)
		} else {
			result.Success = append(result.Success, info)
		}
	}

	return result
}
