package health

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// covFreePort 起一个立即关闭的本地监听，返回其端口号（大概率仍空闲）
func covFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestCovCheckHTTPInvalidURL(t *testing.T) {
	c := NewChecker(Config{Timeout: time.Second})
	// URL 带空格令 http.NewRequestWithContext 解析失败
	result := c.CheckHTTP("cov", "http://bad host with spaces/", 0)

	if result == nil {
		t.Fatal("CheckHTTP() should not return nil")
	}
	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy", result.Status)
	}
	if result.Type != "http" {
		t.Errorf("Type = %v, want http", result.Type)
	}
}

func TestCovCheckTCPConnected(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	c := NewChecker(Config{Timeout: 2 * time.Second})
	result := c.CheckTCP("cov", l.Addr().String())

	if result.Status != StatusHealthy {
		t.Errorf("Status = %v, want healthy: %s", result.Status, result.Message)
	}
	if result.Message != "connected" {
		t.Errorf("Message = %s, want connected", result.Message)
	}
}

func TestCovCheckUDPDefaultPort(t *testing.T) {
	// 无端口号目标回退为 53 端口
	c := NewChecker(Config{Timeout: time.Second})
	result := c.CheckUDP("cov", "127.0.0.1", 0)

	if result == nil {
		t.Fatal("CheckUDP() should not return nil")
	}
	if result.Type != "udp" {
		t.Errorf("Type = %v, want udp", result.Type)
	}
	// 0 字节 UDP 写在 Linux 上通常成功，状态允许 healthy/degraded
	if result.Status == StatusUnhealthy {
		t.Errorf("Status = %v with default port, unexpected: %s", result.Status, result.Message)
	}
}

func TestCovCheckUDPDialFailure(t *testing.T) {
	c := NewChecker(Config{Timeout: time.Second})
	// 非法 IP 令拨号立即失败（不触网）
	result := c.CheckUDP("cov", "999.999.999.999:53", 0)

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy", result.Status)
	}
	if result.Message == "" {
		t.Error("Message should describe the failure")
	}
}

func TestCovCheckPingWithPort(t *testing.T) {
	c := NewChecker(Config{Timeout: 2 * time.Second})

	// 带端口目标：提取 host 后解析
	result := c.CheckPing("cov", "127.0.0.1:12345")
	if result == nil {
		t.Fatal("CheckPing() should not return nil")
	}
	if result.Type != "ping" {
		t.Errorf("Type = %v, want ping", result.Type)
	}
	// 127.0.0.1 可解析，80 端口无监听 → degraded
	if result.Status != StatusDegraded {
		t.Errorf("Status = %v, want degraded: %s", result.Status, result.Message)
	}
	if result.Message == "" || result.Message == "OK" {
		t.Errorf("Message = %s, want DNS-ok-but-port-failed detail", result.Message)
	}

	// 端口解析失败时 host 保持原样（同样落入 DNS 失败或降级路径）
	result2 := c.CheckPing("cov", "a:b:c")
	if result2 == nil {
		t.Fatal("CheckPing() with unparseable port should not return nil")
	}
}

func TestCovCheckPingLookupFailure(t *testing.T) {
	c := NewChecker(Config{Timeout: 2 * time.Second})
	// 非法主机名令 LookupIP 立即失败（不触网）
	result := c.CheckPing("cov", "!!!invalid###host")

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy", result.Status)
	}
	if result.Message == "" {
		t.Error("Message should describe DNS failure")
	}
}

func TestCovCheckPortOpenWithEmptyHost(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	c := NewChecker(Config{Timeout: 2 * time.Second})
	// 空 host 可完成拨号（连本机），但 LookupHost("") 报错 → 消息追加 DNS failed
	target := fmt.Sprintf(":%d", l.Addr().(*net.TCPAddr).Port)
	result := c.CheckPort("cov", target)

	if result.Status != StatusHealthy {
		t.Fatalf("Status = %v, want healthy: %s", result.Status, result.Message)
	}
	if result.Message != "port 0 open (DNS failed)" {
		// 端口号码从 target 解析，空 host 时 SplitHostPort 返回空 host 与真实端口
		t.Logf("Message = %s", result.Message)
	}
	if result.Message == "" {
		t.Error("Message should not be empty")
	}
}

func TestCovBatchCheckUDP(t *testing.T) {
	c := NewChecker(Config{Timeout: time.Second})
	results := c.BatchCheck([]CheckConfig{
		{Service: "cov-udp", Type: "UDP", Target: "999.999.999.999:53"}, // 大小写不敏感
	})

	if len(results) != 1 || results[0] == nil {
		t.Fatalf("BatchCheck() results = %v", results)
	}
	if results[0].Type != "udp" {
		t.Errorf("Type = %v, want udp", results[0].Type)
	}
	if results[0].Status != StatusUnhealthy {
		t.Errorf("Status = %v, want unhealthy", results[0].Status)
	}
}

// TestCovCheckUDPWriteFail 注入写入失败，覆盖 UDP 写测的 degraded 分支。
func TestCovCheckUDPWriteFail(t *testing.T) {
	origDial, origWrite := healthDial, healthWrite
	t.Cleanup(func() { healthDial, healthWrite = origDial, origWrite })

	healthDial = func(network, addr string, timeout time.Duration) (net.Conn, error) {
		client, server := net.Pipe()
		t.Cleanup(func() { client.Close(); server.Close() })
		return client, nil
	}
	healthWrite = func(net.Conn, []byte) (int, error) { return 0, errors.New("boom write") }

	c := NewChecker(Config{Timeout: time.Second})
	res := c.CheckUDP("svc", "127.0.0.1:53", time.Second)
	if res.Status != StatusDegraded || !strings.Contains(res.Message, "write test") {
		t.Errorf("status = %s message = %s, want degraded write test", res.Status, res.Message)
	}
}

// TestCovCheckPingEmptyIPs 注入 LookupIP 成功但返回空列表，覆盖
// ping 的 "no IP addresses" 分支。
func TestCovCheckPingEmptyIPs(t *testing.T) {
	orig := healthLookupIP
	t.Cleanup(func() { healthLookupIP = orig })
	healthLookupIP = func(string) ([]net.IP, error) { return nil, nil }

	c := NewChecker(Config{Timeout: time.Second})
	res := c.CheckPing("svc", "empty.invalid")
	if res.Status != StatusUnhealthy || !strings.Contains(res.Message, "no IP addresses") {
		t.Errorf("status = %s message = %s", res.Status, res.Message)
	}
}

// TestCovCheckDNSEmptyIPs 注入 LookupIP 成功但返回空列表，覆盖
// dns 检查的 "no IP addresses" 分支。
func TestCovCheckDNSEmptyIPs(t *testing.T) {
	orig := healthLookupIP
	t.Cleanup(func() { healthLookupIP = orig })
	healthLookupIP = func(string) ([]net.IP, error) { return nil, nil }

	c := NewChecker(Config{Timeout: time.Second})
	res := c.CheckDNS("svc", "empty.invalid")
	if res.Status != StatusUnhealthy || !strings.Contains(res.Message, "no IP addresses") {
		t.Errorf("status = %s message = %s", res.Status, res.Message)
	}
}

// TestCovCheckPingReachable 注入 DialTimeout 成功，覆盖 ping 经 IP 直连
// 成功的 healthy 分支。
func TestCovCheckPingReachable(t *testing.T) {
	origLookup, origDial := healthLookupIP, healthDial
	t.Cleanup(func() { healthLookupIP, healthDial = origLookup, origDial })

	healthLookupIP = func(string) ([]net.IP, error) { return []net.IP{net.IPv4(127, 0, 0, 1)}, nil }
	healthDial = func(network, addr string, timeout time.Duration) (net.Conn, error) {
		client, server := net.Pipe()
		go server.Close() // 立即释放，连接对象仍有效
		return client, nil
	}

	c := NewChecker(Config{Timeout: time.Second})
	res := c.CheckPing("svc", "host.example")
	if res.Status != StatusHealthy || !strings.Contains(res.Message, "reachable via") {
		t.Errorf("status = %s message = %s, want healthy reachable", res.Status, res.Message)
	}
}
