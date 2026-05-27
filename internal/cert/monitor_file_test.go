package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// generateTestCert creates a self-signed certificate for testing
func generateTestCert(t *testing.T, opts ...func(*x509.Certificate)) ([]byte, *x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "test.example.com",
			Organization: []string{"Test Org"},
		},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"test.example.com", "www.test.example.com"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		EmailAddresses:        []string{"admin@test.example.com"},
	}

	for _, opt := range opts {
		opt(template)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	return certDER, cert, key
}

// writePEMCert writes certificate DER in PEM format to temp file
func writePEMCert(t *testing.T, certDER []byte) string {
	t.Helper()
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	}), 0644)
	return certPath
}

// writeCertAndKey writes both cert and key for TLS server setup
func writeCertAndKey(t *testing.T, certDER []byte, key *ecdsa.PrivateKey) (certPath, keyPath string) {
	t.Helper()
	dir := t.TempDir()

	certPath = filepath.Join(dir, "cert.pem")
	keyBytes, _ := x509.MarshalECPrivateKey(key)
	keyPath = filepath.Join(dir, "key.pem")

	os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	}), 0644)

	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	}), 0644)

	return certPath, keyPath
}

func TestCheckFilePEM(t *testing.T) {
	certDER, _, _ := generateTestCert(t)
	certPath := writePEMCert(t, certDER)

	m := NewMonitor(Config{})
	info, err := m.CheckFile(certPath)
	if err != nil {
		t.Fatalf("CheckFile() error = %v", err)
	}
	if info.Domain != certPath {
		t.Errorf("Domain = %v, want %v", info.Domain, certPath)
	}
	if info.Subject != "test.example.com" {
		t.Errorf("Subject = %v, want test.example.com", info.Subject)
	}
	if info.Issuer != "test.example.com" {
		t.Errorf("Issuer = %v, want test.example.com (self-signed)", info.Issuer)
	}
	if info.IsExpired {
		t.Error("Cert should not be expired")
	}
	if info.DaysLeft <= 0 {
		t.Errorf("DaysLeft = %d, want > 0", info.DaysLeft)
	}
	if info.Fingerprint == "" {
		t.Error("Fingerprint should not be empty")
	}
	if info.SerialNumber == "" {
		t.Error("SerialNumber should not be empty")
	}
}

func TestCheckFileDER(t *testing.T) {
	certDER, _, _ := generateTestCert(t)

	dir := t.TempDir()
	derPath := filepath.Join(dir, "cert.der")
	os.WriteFile(derPath, certDER, 0644)

	m := NewMonitor(Config{})
	info, err := m.CheckFile(derPath)
	if err != nil {
		t.Fatalf("CheckFile() DER error = %v", err)
	}
	if info.Subject != "test.example.com" {
		t.Errorf("Subject = %v, want test.example.com", info.Subject)
	}
}

func TestCheckFileInvalidData(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.pem")
	os.WriteFile(badPath, []byte("not a certificate"), 0644)

	m := NewMonitor(Config{})
	_, err := m.CheckFile(badPath)
	if err == nil {
		t.Error("CheckFile() should return error for invalid data")
	}
}

func TestCheckFileEmptyPEM(t *testing.T) {
	dir := t.TempDir()
	emptyPath := filepath.Join(dir, "empty.pem")
	os.WriteFile(emptyPath, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte{},
	}), 0644)

	m := NewMonitor(Config{})
	_, err := m.CheckFile(emptyPath)
	if err == nil {
		t.Error("CheckFile() should return error for empty cert block")
	}
}

func TestParseCertInfo(t *testing.T) {
	_, cert, _ := generateTestCert(t)

	m := NewMonitor(Config{})
	info := m.parseCertInfo(cert, "test.example.com", "192.168.1.1:443")

	if info.Domain != "test.example.com" {
		t.Errorf("Domain = %v", info.Domain)
	}
	if info.IP != "192.168.1.1" {
		t.Errorf("IP = %v, want 192.168.1.1", info.IP)
	}
	if info.Subject != "test.example.com" {
		t.Errorf("Subject = %v", info.Subject)
	}
	if info.IsExpired {
		t.Error("Should not be expired")
	}
}

func TestParseCertInfoNoRemoteAddr(t *testing.T) {
	_, cert, _ := generateTestCert(t)

	m := NewMonitor(Config{})
	info := m.parseCertInfo(cert, "test.example.com", "")

	if info.IP != "" {
		t.Errorf("IP should be empty, got %v", info.IP)
	}
}

func TestParseCertInfoExpired(t *testing.T) {
	_, cert, _ := generateTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.NotAfter = time.Now().Add(-24 * time.Hour)
		tmpl.NotBefore = time.Now().Add(-48 * time.Hour)
	})

	m := NewMonitor(Config{})
	info := m.parseCertInfo(cert, "expired.com", "")

	if !info.IsExpired {
		t.Error("Should be expired")
	}
	if info.DaysLeft >= 0 {
		t.Errorf("DaysLeft = %d, want negative", info.DaysLeft)
	}
}

func TestGetSANs(t *testing.T) {
	_, cert, _ := generateTestCert(t)

	m := NewMonitor(Config{})
	sans := m.GetSANs(cert)

	if len(sans.DNSNames) != 2 {
		t.Errorf("DNSNames count = %d, want 2", len(sans.DNSNames))
	}
	if len(sans.IPs) != 1 {
		t.Errorf("IPs count = %d, want 1", len(sans.IPs))
	}
	if sans.IPs[0] != "127.0.0.1" {
		t.Errorf("IP = %v, want 127.0.0.1", sans.IPs[0])
	}
	if len(sans.Emails) != 1 {
		t.Errorf("Emails count = %d, want 1", len(sans.Emails))
	}
	if sans.Emails[0] != "admin@test.example.com" {
		t.Errorf("Email = %v", sans.Emails[0])
	}
}

func TestGetSANsEmpty(t *testing.T) {
	_, cert, _ := generateTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.DNSNames = nil
		tmpl.IPAddresses = nil
		tmpl.EmailAddresses = nil
	})

	m := NewMonitor(Config{})
	sans := m.GetSANs(cert)

	if len(sans.DNSNames) != 0 {
		t.Errorf("DNSNames should be empty, got %d", len(sans.DNSNames))
	}
	if len(sans.IPs) != 0 {
		t.Errorf("IPs should be empty, got %d", len(sans.IPs))
	}
	if len(sans.Emails) != 0 {
		t.Errorf("Emails should be empty, got %d", len(sans.Emails))
	}
}

func TestValidateForDomainMatchCN(t *testing.T) {
	_, cert, _ := generateTestCert(t)

	m := NewMonitor(Config{})
	result := m.ValidateForDomain(cert, "test.example.com")

	if !result.Valid {
		t.Error("Should be valid")
	}
	if !result.Matched {
		t.Error("CN should match")
	}
}

func TestValidateForDomainMatchSAN(t *testing.T) {
	_, cert, _ := generateTestCert(t)

	m := NewMonitor(Config{})
	result := m.ValidateForDomain(cert, "www.test.example.com")

	if !result.Matched {
		t.Error("SAN should match")
	}
	if len(result.SANsMatched) != 1 {
		t.Errorf("SANsMatched count = %d, want 1", len(result.SANsMatched))
	}
}

func TestValidateForDomainNoMatch(t *testing.T) {
	_, cert, _ := generateTestCert(t)

	m := NewMonitor(Config{})
	result := m.ValidateForDomain(cert, "other.com")

	if result.Matched {
		t.Error("Should not match other.com")
	}
}

func TestValidateForDomainExpired(t *testing.T) {
	_, cert, _ := generateTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.NotAfter = time.Now().Add(-24 * time.Hour)
		tmpl.NotBefore = time.Now().Add(-48 * time.Hour)
	})

	m := NewMonitor(Config{})
	result := m.ValidateForDomain(cert, "test.example.com")

	if result.Valid {
		t.Error("Expired cert should not be valid")
	}
	found := false
	for _, e := range result.Errors {
		if e == "certificate expired" {
			found = true
		}
	}
	if !found {
		t.Error("Should have 'certificate expired' error")
	}
}

func TestValidateForDomainNotYetValid(t *testing.T) {
	_, cert, _ := generateTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.NotBefore = time.Now().Add(24 * time.Hour)
		tmpl.NotAfter = time.Now().Add(365 * 24 * time.Hour)
	})

	m := NewMonitor(Config{})
	result := m.ValidateForDomain(cert, "test.example.com")

	if result.Valid {
		t.Error("Not-yet-valid cert should not be valid")
	}
}

func TestCheckDomainWithTLSServer(t *testing.T) {
	certDER, _, key := generateTestCert(t)
	certPath, keyPath := writeCertAndKey(t, certDER, key)

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadX509KeyPair: %v", err)
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	ts.StartTLS()
	defer ts.Close()

	// Extract host:port from ts.URL
	addr := ts.Listener.Addr().String()
	host, portStr, _ := net.SplitHostPort(addr)

	var port int
	fmt.Sscanf(portStr, "%d", &port)

	m := NewMonitor(Config{Timeout: 5 * time.Second})
	info, err := m.CheckDomain(host, port)
	if err != nil {
		t.Fatalf("CheckDomain() error = %v", err)
	}
	if info.Subject != "test.example.com" {
		t.Errorf("Subject = %v, want test.example.com", info.Subject)
	}
	if info.IP == "" {
		t.Error("IP should not be empty")
	}
	if info.IsExpired {
		t.Error("Should not be expired")
	}
}

func TestCheckDomainChainWithTLSServer(t *testing.T) {
	certDER, _, key := generateTestCert(t)
	certPath, keyPath := writeCertAndKey(t, certDER, key)

	cert, _ := tls.LoadX509KeyPair(certPath, keyPath)

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	ts.StartTLS()
	defer ts.Close()

	addr := ts.Listener.Addr().String()
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	m := NewMonitor(Config{Timeout: 5 * time.Second})
	chain, err := m.CheckDomainChain(host, port)
	if err != nil {
		t.Fatalf("CheckDomainChain() error = %v", err)
	}
	if len(chain) == 0 {
		t.Error("Chain should not be empty")
	}
	if chain[0].Subject != "test.example.com" {
		t.Errorf("Chain[0].Subject = %v", chain[0].Subject)
	}
}

func TestCheckDomainConnectionRefused(t *testing.T) {
	m := NewMonitor(Config{Timeout: 1 * time.Second})
	_, err := m.CheckDomain("127.0.0.1", 1)
	if err == nil {
		t.Error("CheckDomain() should fail on connection refused")
	}
}

func TestCheckMailServerConnectionRefused(t *testing.T) {
	m := NewMonitor(Config{Timeout: 1 * time.Second})
	_, err := m.CheckMailServer("127.0.0.1", 1)
	if err == nil {
		t.Error("CheckMailServer() should fail on connection refused")
	}
}

func TestCheckMailServerDefaultPort(t *testing.T) {
	m := NewMonitor(Config{Timeout: 1 * time.Second})
	// Just verify it doesn't crash with port 0
	_, _ = m.CheckMailServer("localhost", 0)
}
