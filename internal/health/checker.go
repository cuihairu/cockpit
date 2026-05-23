package health

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Status health check status
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusDegraded  Status = "degraded"
	StatusUnknown   Status = "unknown"
)

// Result health check result
type Result struct {
	Service     string        `json:"service"`
	Type        string        `json:"type"`
	Target      string        `json:"target"`
	Status      Status        `json:"status"`
	Latency     time.Duration `json:"latency"`
	Message     string        `json:"message"`
	StatusCode  int           `json:"status_code,omitempty"`
	CheckedAt   time.Time     `json:"checked_at"`
}

// Checker health checker
type Checker struct {
	httpClient *http.Client
	timeout    time.Duration
}

// Config checker configuration
type Config struct {
	Timeout     time.Duration
	SkipTLSVerify bool
}

// NewChecker creates health checker
func NewChecker(cfg Config) *Checker {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &Checker{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: cfg.SkipTLSVerify,
				},
				DialContext: (&net.Dialer{
					Timeout:   timeout,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		timeout: timeout,
	}
}

// CheckHTTP checks HTTP endpoint
func (c *Checker) CheckHTTP(service, target string, expectedStatus int) *Result {
	start := time.Now()

	// Validate URL
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return &Result{
			Service:   service,
			Type:      "http",
			Target:    target,
			Status:    StatusUnhealthy,
			Latency:   time.Since(start),
			Message:   fmt.Sprintf("create request: %v", err),
			CheckedAt: time.Now(),
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &Result{
			Service:   service,
			Type:      "http",
			Target:    target,
			Status:    StatusUnhealthy,
			Latency:   time.Since(start),
			Message:   fmt.Sprintf("request failed: %v", err),
			CheckedAt: time.Now(),
		}
	}
	defer resp.Body.Close()

	latency := time.Since(start)
	status := StatusHealthy
	message := "OK"

	if expectedStatus > 0 && resp.StatusCode != expectedStatus {
		status = StatusUnhealthy
		message = fmt.Sprintf("unexpected status: %d (expected %d)", resp.StatusCode, expectedStatus)
	} else if resp.StatusCode >= 500 {
		status = StatusUnhealthy
		message = fmt.Sprintf("server error: %d", resp.StatusCode)
	} else if resp.StatusCode >= 400 {
		status = StatusDegraded
		message = fmt.Sprintf("client error: %d", resp.StatusCode)
	}

	return &Result{
		Service:    service,
		Type:       "http",
		Target:     target,
		Status:     status,
		Latency:    latency,
		Message:    message,
		StatusCode: resp.StatusCode,
		CheckedAt:  time.Now(),
	}
}

// CheckTCP checks TCP connection
func (c *Checker) CheckTCP(service, target string) *Result {
	start := time.Now()

	// Parse host:port
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		// Try adding default port
		if strings.Contains(target, ":") {
			return &Result{
				Service:   service,
				Type:      "tcp",
				Target:    target,
				Status:    StatusUnhealthy,
				Latency:   time.Since(start),
				Message:   fmt.Sprintf("invalid address: %v", err),
				CheckedAt: time.Now(),
			}
		}
		host = target
		port = "80"
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return &Result{
			Service:   service,
			Type:      "tcp",
			Target:    target,
			Status:    StatusUnhealthy,
			Latency:   time.Since(start),
			Message:   fmt.Sprintf("connection failed: %v", err),
			CheckedAt: time.Now(),
		}
	}
	defer conn.Close()

	return &Result{
		Service:   service,
		Type:      "tcp",
		Target:    target,
		Status:    StatusHealthy,
		Latency:   time.Since(start),
		Message:   "connected",
		CheckedAt: time.Now(),
	}
}

// CheckUDP checks UDP connection
func (c *Checker) CheckUDP(service, target string, timeout time.Duration) *Result {
	start := time.Now()

	if timeout == 0 {
		timeout = c.timeout
	}

	host, port, err := net.SplitHostPort(target)
	if err != nil {
		if strings.Contains(target, ":") {
			return &Result{
				Service:   service,
				Type:      "udp",
				Target:    target,
				Status:    StatusUnhealthy,
				Latency:   time.Since(start),
				Message:   fmt.Sprintf("invalid address: %v", err),
				CheckedAt: time.Now(),
			}
		}
		host = target
		port = "53"
	}

	conn, err := net.DialTimeout("udp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return &Result{
			Service:   service,
			Type:      "udp",
			Target:    target,
			Status:    StatusUnhealthy,
			Latency:   time.Since(start),
			Message:   fmt.Sprintf("connection failed: %v", err),
			CheckedAt: time.Now(),
		}
	}
	defer conn.Close()

	// Set write deadline
	conn.SetWriteDeadline(time.Now().Add(timeout))

	// Try to write data
	_, err = conn.Write([]byte(""))
	if err != nil {
		return &Result{
			Service:   service,
			Type:      "udp",
			Target:    target,
			Status:    StatusDegraded,
			Latency:   time.Since(start),
			Message:   fmt.Sprintf("write test: %v", err),
			CheckedAt: time.Now(),
		}
	}

	return &Result{
		Service:   service,
		Type:      "udp",
		Target:    target,
		Status:    StatusHealthy,
		Latency:   time.Since(start),
		Message:   "connected",
		CheckedAt: time.Now(),
	}
}

// CheckPing checks ICMP ping
func (c *Checker) CheckPing(service, target string) *Result {
	start := time.Now()

	// Parse target
	host := target
	if strings.Contains(target, ":") {
		h, _, err := net.SplitHostPort(target)
		if err == nil {
			host = h
		}
	}

	// Resolve hostname
 IPs, err := net.LookupIP(host)
	if err != nil {
		return &Result{
			Service:   service,
			Type:      "ping",
			Target:    target,
			Status:    StatusUnhealthy,
			Latency:   time.Since(start),
			Message:   fmt.Sprintf("DNS lookup failed: %v", err),
			CheckedAt: time.Now(),
		}
	}

	if len(IPs) == 0 {
		return &Result{
			Service:   service,
			Type:      "ping",
			Target:    target,
			Status:    StatusUnhealthy,
			Latency:   time.Since(start),
			Message:   "no IP addresses found",
			CheckedAt: time.Now(),
		}
	}

	// Use first IP
	ip := IPs[0].String()

	// Try to connect (simple check)
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "80"), 2*time.Second)
	if err == nil {
		conn.Close()
		return &Result{
			Service:   service,
			Type:      "ping",
			Target:    target,
			Status:    StatusHealthy,
			Latency:   time.Since(start),
			Message:   fmt.Sprintf("reachable via %s", ip),
			CheckedAt: time.Now(),
		}
	}

	// DNS worked but TCP failed - degraded
	return &Result{
		Service:   service,
		Type:      "ping",
		Target:    target,
		Status:    StatusDegraded,
		Latency:   time.Since(start),
		Message:   fmt.Sprintf("DNS OK (%s) but port check failed", ip),
		CheckedAt: time.Now(),
	}
}

// CheckDNS checks DNS resolution
func (c *Checker) CheckDNS(service, target string) *Result {
	start := time.Now()

	IPs, err := net.LookupIP(target)
	if err != nil {
		return &Result{
			Service:   service,
			Type:      "dns",
			Target:    target,
			Status:    StatusUnhealthy,
			Latency:   time.Since(start),
			Message:   fmt.Sprintf("DNS lookup failed: %v", err),
			CheckedAt: time.Now(),
		}
	}

	if len(IPs) == 0 {
		return &Result{
			Service:   service,
			Type:      "dns",
			Target:    target,
			Status:    StatusUnhealthy,
			Latency:   time.Since(start),
			Message:   "no IP addresses found",
			CheckedAt: time.Now(),
		}
	}

	ipStrs := make([]string, len(IPs))
	for i, ip := range IPs {
		ipStrs[i] = ip.String()
	}

	return &Result{
		Service:   service,
		Type:      "dns",
		Target:    target,
		Status:    StatusHealthy,
		Latency:   time.Since(start),
		Message:   fmt.Sprintf("resolved to %s", strings.Join(ipStrs, ", ")),
		CheckedAt: time.Now(),
	}
}

// CheckPort checks if port is open
func (c *Checker) CheckPort(service, target string) *Result {
	start := time.Now()

	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return &Result{
			Service:   service,
			Type:      "port",
			Target:    target,
			Status:    StatusUnhealthy,
			Latency:   time.Since(start),
			Message:   fmt.Sprintf("invalid address: %v", err),
			CheckedAt: time.Now(),
		}
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return &Result{
			Service:   service,
			Type:      "port",
			Target:    target,
			Status:    StatusUnhealthy,
			Latency:   time.Since(start),
			Message:   fmt.Sprintf("invalid port: %v", err),
			CheckedAt: time.Now(),
		}
	}

	_, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	conn, err := net.DialTimeout("tcp", target, c.timeout)
	if err != nil {
		return &Result{
			Service:   service,
			Type:      "port",
			Target:    target,
			Status:    StatusUnhealthy,
			Latency:   time.Since(start),
			Message:   fmt.Sprintf("port %d closed: %v", port, err),
			CheckedAt: time.Now(),
		}
	}
	defer conn.Close()

	// Try to resolve hostname too
	_, err = net.LookupHost(host)
	message := fmt.Sprintf("port %d open", port)
	if err != nil {
		message += " (DNS failed)"
	}

	return &Result{
		Service:   service,
		Type:      "port",
		Target:    target,
		Status:    StatusHealthy,
		Latency:   time.Since(start),
		Message:   message,
		CheckedAt: time.Now(),
	}
}

// CheckConfig check configuration
type CheckConfig struct {
	Service        string
	Type           string
	Target         string
	ExpectedStatus int
}

// BatchCheck checks multiple services
func (c *Checker) BatchCheck(configs []CheckConfig) []*Result {
	results := make([]*Result, len(configs))

	for i, cfg := range configs {
		switch strings.ToLower(cfg.Type) {
		case "http", "https":
			results[i] = c.CheckHTTP(cfg.Service, cfg.Target, cfg.ExpectedStatus)
		case "tcp", "port":
			results[i] = c.CheckPort(cfg.Service, cfg.Target)
		case "udp":
			results[i] = c.CheckUDP(cfg.Service, cfg.Target, 0)
		case "ping":
			results[i] = c.CheckPing(cfg.Service, cfg.Target)
		case "dns":
			results[i] = c.CheckDNS(cfg.Service, cfg.Target)
		default:
			// Auto-detect
			if strings.HasPrefix(cfg.Target, "http") {
				results[i] = c.CheckHTTP(cfg.Service, cfg.Target, cfg.ExpectedStatus)
			} else if strings.Contains(cfg.Target, ":") {
				results[i] = c.CheckPort(cfg.Service, cfg.Target)
			} else {
				results[i] = c.CheckDNS(cfg.Service, cfg.Target)
			}
		}
	}

	return results
}

// GetOverallStatus gets overall status from multiple results
func GetOverallStatus(results []*Result) Status {
	if len(results) == 0 {
		return StatusUnknown
	}

	hasUnhealthy := false
	hasDegraded := false

	for _, r := range results {
		if r.Status == StatusUnhealthy {
			hasUnhealthy = true
		} else if r.Status == StatusDegraded {
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return StatusUnhealthy
	}
	if hasDegraded {
		return StatusDegraded
	}
	return StatusHealthy
}

// IsHealthy returns true if status is healthy
func (r *Result) IsHealthy() bool {
	return r.Status == StatusHealthy
}

// IsUnhealthy returns true if status is unhealthy
func (r *Result) IsUnhealthy() bool {
	return r.Status == StatusUnhealthy
}

// ShouldAlert returns true if should trigger alert
func (r *Result) ShouldAlert() bool {
	return r.Status == StatusUnhealthy || r.Status == StatusDegraded
}

// String returns string representation
func (s Status) String() string {
	return string(s)
}

// ParseURL parses target as URL
func ParseURL(target string) (*url.URL, error) {
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}
	return url.Parse(target)
}
