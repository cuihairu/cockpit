package cert

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestCovCheckDomainChainDefaultPort 覆盖 port==0 时默认 443 与连接失败分支。
// 本机 443 通常无服务：连接拒绝走 err 分支；若恰有服务则宽松接受成功结果。
func TestCovCheckDomainChainDefaultPort(t *testing.T) {
	m := NewMonitor(Config{Timeout: 1 * time.Second})

	chain, err := m.CheckDomainChain("127.0.0.1", 0)
	if err != nil {
		if len(chain) != 0 {
			t.Errorf("chain should be nil on error, got %v", chain)
		}
		return // 连接拒绝（预期常见路径）
	}
	// 极端情况：本机 443 有 TLS 服务，则验证链非空即可
	if len(chain) == 0 {
		t.Error("CheckDomainChain() returned empty chain without error")
	}
}

// TestCovCheckMailServerSuccess 通过本地 TLS 服务器覆盖邮件服务器证书检查的成功路径。
func TestCovCheckMailServerSuccess(t *testing.T) {
	certDER, _, key := generateTestCert(t)
	certPath, keyPath := writeCertAndKey(t, certDER, key)

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadX509KeyPair: %v", err)
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	ts.StartTLS()
	defer ts.Close()

	addr := ts.Listener.Addr().String()
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	m := NewMonitor(Config{Timeout: 5 * time.Second})
	info, err := m.CheckMailServer(host, port)
	if err != nil {
		t.Fatalf("CheckMailServer() error = %v", err)
	}
	if info.Subject != "test.example.com" {
		t.Errorf("Subject = %q, want test.example.com", info.Subject)
	}
	if info.IP == "" {
		t.Error("IP should be populated from remote addr")
	}
	if info.IsExpired {
		t.Error("freshly generated cert should not be expired")
	}
}
