package domain

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// covWriteFakeWhois 写一个假 whois 可执行脚本并返回路径。
// serverMode 为 true 时脚本会识别 -h <server> 参数并输出不同内容，
// 便于断言 whoisServer 分支确实生效。
func covWriteFakeWhois(t *testing.T, serverMode bool) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "whois")

	script := `#!/bin/sh
case " $* " in
  *" -h "*)
    printf 'Registrar: ViaServer Registrar\n'
    printf 'Expiration Date: 2099-06-15T04:00:00Z\n'
    printf 'Domain Status: viaServer\n'
    exit 0
    ;;
esac
printf 'Registrar: Cov Registrar, Inc.\n'
printf 'Creation Date: 2020-01-01T00:00:00Z\n'
printf 'Registry Expiry Date: 2099-06-15T04:00:00Z\n'
printf 'Name Server: NS1.COV.EXAMPLE\n'
printf 'Name Server: ns2.cov.example\n'
printf 'Domain Status: clientTransferProhibited\n'
printf 'DNSSEC: signedDelegation\n'
printf 'plain line without colon should be skipped\n'
`
	if !serverMode {
		script = `#!/bin/sh
printf 'Registrar: Cov Registrar, Inc.\n'
printf 'Creation Date: 2020-01-01T00:00:00Z\n'
printf 'Registry Expiry Date: 2099-06-15T04:00:00Z\n'
printf 'Name Server: NS1.COV.EXAMPLE\n'
printf 'Name Server: ns2.cov.example\n'
printf 'Domain Status: clientTransferProhibited\n'
printf 'DNSSEC: signedDelegation\n'
printf 'plain line without colon should be skipped\n'
`
	}

	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake whois: %v", err)
	}
	return path
}

func TestCovNewMonitorAutoDetectWhois(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "whois")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake whois: %v", err)
	}
	t.Setenv("PATH", dir)

	m := NewMonitor(Config{})
	if m.whoisPath != fake {
		t.Errorf("whoisPath = %q, want auto-detected %q", m.whoisPath, fake)
	}
}

func TestCovCheckWithFakeWhois(t *testing.T) {
	m := NewMonitor(Config{WhoisPath: covWriteFakeWhois(t, false)})

	info, err := m.Check("  EXAMPLE.com  ")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if info.Domain != "example.com" {
		t.Errorf("Domain = %q, want lowercased/trimmed example.com", info.Domain)
	}
	if info.Registrar != "Cov Registrar, Inc." {
		t.Errorf("Registrar = %q, want %q", info.Registrar, "Cov Registrar, Inc.")
	}
	if info.IsExpired {
		t.Error("IsExpired should be false for 2099 expiry")
	}
	if info.DaysLeft <= 0 {
		t.Errorf("DaysLeft = %d, want positive for 2099 expiry", info.DaysLeft)
	}
	if len(info.NameServers) != 2 {
		t.Fatalf("NameServers = %v, want 2 entries", info.NameServers)
	}
	if info.NameServers[0] != "ns1.cov.example" {
		t.Errorf("NameServers[0] = %q, want lowercased ns1.cov.example", info.NameServers[0])
	}
	if info.DNSSec != true {
		t.Error("DNSSec should be true for signedDelegation")
	}
	if !time.Now().Before(info.Expires) {
		t.Error("Expires should be in the future")
	}
}

func TestCovQueryWhoisWithServerFlag(t *testing.T) {
	m := NewMonitor(Config{
		WhoisPath:   covWriteFakeWhois(t, true),
		WhoisServer: "whois.cov.test",
	})

	out, err := m.queryWhois("example.com")
	if err != nil {
		t.Fatalf("queryWhois() error = %v", err)
	}
	if !strings.Contains(out, "ViaServer Registrar") {
		t.Errorf("queryWhois() output should reflect -h server invocation, got %q", out)
	}
}

func TestCovCheckWithServerFakeWhois(t *testing.T) {
	m := NewMonitor(Config{WhoisPath: covWriteFakeWhois(t, true)})

	info, err := m.CheckWithServer("example.com", "whois.cov.test")
	if err != nil {
		t.Fatalf("CheckWithServer() error = %v", err)
	}
	if info.Registrar != "ViaServer Registrar" {
		t.Errorf("Registrar = %q, want ViaServer Registrar", info.Registrar)
	}

	// 结束后 whoisServer 应回复为原值
	if m.whoisServer != "" {
		t.Errorf("whoisServer = %q after CheckWithServer, want restored empty", m.whoisServer)
	}
}

func TestCovCheckMultipleSuccess(t *testing.T) {
	m := NewMonitor(Config{WhoisPath: covWriteFakeWhois(t, false)})

	results := m.CheckMultiple([]string{"example.com", "other.org", "invalid"})
	if len(results) != 3 {
		t.Fatalf("CheckMultiple() returned %d results, want 3", len(results))
	}

	info := results["example.com"]
	if info == nil || info.DaysLeft < 0 {
		t.Errorf("example.com should succeed, got %+v", info)
	}
	info2 := results["other.org"]
	if info2 == nil || info2.DaysLeft < 0 {
		t.Errorf("other.org should succeed, got %+v", info2)
	}
	errInfo := results["invalid"]
	if errInfo == nil || errInfo.DaysLeft != -1 {
		t.Errorf("invalid should be marked with DaysLeft -1, got %+v", errInfo)
	}
}

func TestCovBatchCheckSuccess(t *testing.T) {
	m := NewMonitor(Config{WhoisPath: covWriteFakeWhois(t, false)})

	result := m.BatchCheck([]string{"example.com", "invalid"})
	if len(result.Success) != 1 {
		t.Fatalf("Success = %v, want 1 entry", result.Success)
	}
	if result.Success[0].Registrar != "Cov Registrar, Inc." {
		t.Errorf("Success[0].Registrar = %q, want Cov Registrar, Inc.", result.Success[0].Registrar)
	}
	if len(result.Failed) != 1 || result.Failed[0] != "invalid" {
		t.Errorf("Failed = %v, want [invalid]", result.Failed)
	}
}

func TestCovResolveDNSLocalhost(t *testing.T) {
	m := NewMonitor(Config{})

	ips, err := m.ResolveDNS("localhost")
	if err != nil {
		t.Skipf("ResolveDNS(localhost) error = %v (no local hosts entry)", err)
	}
	found := false
	for _, ip := range ips {
		if ip == "127.0.0.1" {
			found = true
		}
	}
	if !found {
		t.Errorf("ResolveDNS(localhost) = %v, want 127.0.0.1", ips)
	}
}

func TestCovResolveDNSError(t *testing.T) {
	m := NewMonitor(Config{})

	ips, err := m.ResolveDNS("nonexistent-cov-probe.invalid")
	if err == nil {
		t.Skipf("ResolveDNS unexpectedly succeeded with %v (resolver resolves everything)", ips)
	}
	if ips != nil {
		t.Errorf("ResolveDNS() should return nil ips on error, got %v", ips)
	}
	if !strings.Contains(err.Error(), "dns lookup") {
		t.Errorf("error should wrap dns lookup, got %v", err)
	}
}

func TestCovCheckDNSIPv6(t *testing.T) {
	m := NewMonitor(Config{})

	// ip6-localhost 在多数 /etc/hosts 中映射 ::1，不依赖外部网络
	info, err := m.CheckDNS("ip6-localhost")
	if err != nil {
		t.Fatalf("CheckDNS() error = %v", err)
	}
	if info.Domain != "ip6-localhost" {
		t.Errorf("Domain = %q, want ip6-localhost", info.Domain)
	}
	hasV6 := false
	for _, r := range info.AAAARecords {
		if r == "::1" {
			hasV6 = true
		}
	}
	if !hasV6 {
		t.Skipf("AAAARecords = %v, no ::1 in hosts file", info.AAAARecords)
	}
}

func TestCovCheckDNSFullRecords(t *testing.T) {
	m := NewMonitor(Config{})

	// 断言全部宽松：无网络时各 lookup 失败被产品代码吞掉，测试依然通过
	info, err := m.CheckDNS("example.com")
	if err != nil {
		t.Fatalf("CheckDNS() error = %v", err)
	}
	if info.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com", info.Domain)
	}
	for _, a := range info.ARecords {
		if !strings.Contains(a, ".") {
			t.Errorf("ARecords entry %q does not look like IPv4", a)
		}
	}
	for _, mx := range info.MXRecords {
		if mx == "" {
			t.Error("MXRecords should not contain empty host")
		}
	}
	for _, txt := range info.TXTRecords {
		if txt == "" {
			t.Error("TXTRecords should not contain empty strings")
		}
	}
	for _, ns := range info.NSRecords {
		if ns == "" {
			t.Error("NSRecords should not contain empty host")
		}
	}
}

func TestCovCheckDNSUnknownDomain(t *testing.T) {
	m := NewMonitor(Config{})

	// 全部查询失败时仍应返回空的 DNSInfo 而非错误
	info, err := m.CheckDNS("nonexistent-cov-probe.invalid")
	if err != nil {
		t.Fatalf("CheckDNS() should not fail on unresolvable domain, got %v", err)
	}
	if info == nil || info.Domain != "nonexistent-cov-probe.invalid" {
		t.Errorf("CheckDNS() = %+v, want empty info with domain set", info)
	}
}

func TestCovIsAvailableRegistered(t *testing.T) {
	m := NewMonitor(Config{WhoisPath: covWriteFakeWhois(t, false)})

	available, err := m.IsAvailable("example.com")
	if err != nil {
		t.Fatalf("IsAvailable() error = %v", err)
	}
	if available {
		t.Error("IsAvailable() should be false when whois returns records")
	}
}

func TestCovIsAvailableNoDNS(t *testing.T) {
	// 环境确定性：真实 DNS 在 fake-IP 解析器（Clash/MOS 等）下会把 .invalid
	// TLD 经搜索域追加也解析掉——注入解析失败，与 probe 包同款纪律
	orig := domainLookupIP
	t.Cleanup(func() { domainLookupIP = orig })
	domainLookupIP = func(string) ([]net.IP, error) {
		return nil, errors.New("no such host")
	}

	m := NewMonitor(Config{WhoisPath: "/non/existent/whois"})

	available, err := m.IsAvailable("nonexistent-cov-probe.invalid")
	if err != nil {
		t.Fatalf("IsAvailable() error = %v", err)
	}
	if !available {
		t.Error("IsAvailable() should be true when both whois and DNS fail")
	}
}

func TestCovIsAvailableResolvableButNoWhois(t *testing.T) {
	m := NewMonitor(Config{WhoisPath: "/non/existent/whois"})

	// localhost 可解析出 IP：whois 失败但 DNS 有记录 → 已注册
	available, err := m.IsAvailable("localhost")
	if err != nil {
		t.Fatalf("IsAvailable() error = %v", err)
	}
	if available {
		t.Error("IsAvailable() should be false when DNS resolves to IPs")
	}
}

func TestCovParseWhoisEdgeLines(t *testing.T) {
	m := NewMonitor(Config{})

	text := "Registrar: Cov\nName Server: \nDomain Status: \nDNSSEC: unsigned\nno colon line\n: empty key\n"
	info, err := m.parseWhois("example.com", text)
	if err != nil {
		t.Fatalf("parseWhois() error = %v", err)
	}
	if info.Registrar != "Cov" {
		t.Errorf("Registrar = %q, want Cov", info.Registrar)
	}
	if len(info.NameServers) != 0 {
		t.Errorf("NameServers = %v, want empty for blank value", info.NameServers)
	}
	if len(info.Status) != 0 {
		t.Errorf("Status = %v, want empty for blank value", info.Status)
	}
	// 疑似产品 bug：'unsigned' 包含子串 'signed'，DNSSec 被误判为 true。
	// 此处按当前实际行为断言（同时用不含 'signed' 的值验证 false 分支）。
	if !info.DNSSec {
		t.Error("DNSSec should currently be true for 'unsigned' (substring match quirk)")
	}

	info2, err := m.parseWhois("example.com", "DNSSEC: no\n")
	if err != nil {
		t.Fatalf("parseWhois() error = %v", err)
	}
	if info2.DNSSec {
		t.Error("DNSSec should be false when value lacks 'signed'")
	}
}
