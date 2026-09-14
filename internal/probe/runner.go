package probe

import (
	"context"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/cert"
	"github.com/cuihairu/cockpit/internal/health"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ProbeResult 单次探测结果
type ProbeResult struct {
	ResourceType string        `json:"resource_type"` // domain / service / certificate
	ResourceID   string        `json:"resource_id"`
	Name         string        `json:"name"`
	Status       string        `json:"status"`        // healthy / unhealthy / degraded
	LatencyMs    int64         `json:"latency_ms"`
	Message      string        `json:"message"`
	CheckedAt    time.Time     `json:"checked_at"`
	Error        string        `json:"error,omitempty"`
}

// ProbeResults 一轮探测的结果集
type ProbeResults struct {
	Results      []ProbeResult `json:"results"`
	Domains      int           `json:"domains"`
	Services     int           `json:"services"`
	Certificates int           `json:"certificates"`
	Healthy      int           `json:"healthy"`
	Unhealthy    int           `json:"unhealthy"`
	Duration     time.Duration `json:"duration"`
}

// Runner 自动健康探测运行器
type Runner struct {
	db            *storage.DB
	healthChecker *health.Checker
	certMonitor   *cert.Monitor
	interval      time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// NewRunner 创建探测运行器
func NewRunner(db *storage.DB, interval time.Duration) *Runner {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Runner{
		db:            db,
		healthChecker: health.NewChecker(health.Config{Timeout: 10 * time.Second}),
		certMonitor:   cert.NewMonitor(cert.Config{Timeout: 10 * time.Second}),
		interval:      interval,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start 启动定期探测循环
func (r *Runner) Start() {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		// 启动后 10 秒执行第一轮
		select {
		case <-time.After(10 * time.Second):
		case <-r.ctx.Done():
			return
		}
		r.RunAllChecks()

		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				r.RunAllChecks()
			case <-r.ctx.Done():
				return
			}
		}
	}()
}

// Stop 停止探测循环
func (r *Runner) Stop() {
	r.cancel()
	r.wg.Wait()
}

// RunAllChecks 执行一轮完整探测
func (r *Runner) RunAllChecks() *ProbeResults {
	start := time.Now()
	results := &ProbeResults{}

	var domainResults []ProbeResult
	var serviceResults []ProbeResult
	var certResults []ProbeResult

	// 并行探测三类资源
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		domainResults = r.checkDomains()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		serviceResults = r.checkServices()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		certResults = r.checkCertificates()
	}()

	wg.Wait()

	// 汇总结果
	results.Results = append(results.Results, domainResults...)
	results.Results = append(results.Results, serviceResults...)
	results.Results = append(results.Results, certResults...)
	results.Domains = len(domainResults)
	results.Services = len(serviceResults)
	results.Certificates = len(certResults)

	for _, res := range results.Results {
		if res.Status == "healthy" || res.Status == "active" || res.Status == "valid" {
			results.Healthy++
		} else if res.Error != "" || res.Status == "unhealthy" || res.Status == "down" || res.Status == "error" {
			results.Unhealthy++
		}
	}

	results.Duration = time.Since(start)

	// 如果探测了资源，打印摘要日志
	total := results.Domains + results.Services + results.Certificates
	if total > 0 {
		log.Printf("[probe] Completed: %d resources checked (%d domains, %d services, %d certs) in %v — %d healthy, %d unhealthy",
			total, results.Domains, results.Services, results.Certificates, results.Duration, results.Healthy, results.Unhealthy)
	} else {
		log.Printf("[probe] No resources to probe")
	}

	return results
}

// checkDomains 探测所有域名
func (r *Runner) checkDomains() []ProbeResult {
	domains, err := r.db.ListDomains()
	if err != nil || len(domains) == 0 {
		return nil
	}

	var results []ProbeResult
	for _, d := range domains {
		result := r.probeDomain(d)
		results = append(results, result)

		// 更新数据库中的域名状态
		status := "active"
		if result.Error != "" {
			status = "down"
		}
		if err := r.db.UpdateDomainStatus(d.ID, status); err != nil {
			log.Printf("[probe] Failed to update domain %s: %v", d.Domain, err)
		}
	}
	return results
}

// probeDomain 探测单个域名
func (r *Runner) probeDomain(d *storage.Domain) ProbeResult {
	// DNS 解析检查
	dnsResult := r.healthChecker.CheckDNS(d.Domain, d.Domain)

	// HTTP HEAD 检查（仅当域名可能解析时）
	var httpStatus health.Status
	if dnsResult.Status == health.StatusHealthy {
		httpResult := r.healthChecker.CheckHTTP(d.Domain, "https://"+d.Domain, 0)
		httpStatus = httpResult.Status

		// 如果 HTTPS 不可达，降级到 HTTP
		if httpResult.Status == health.StatusUnhealthy {
			httpResult = r.healthChecker.CheckHTTP(d.Domain, "http://"+d.Domain, 0)
			httpStatus = httpResult.Status
		}
	}

	now := time.Now()
	pr := ProbeResult{
		ResourceType: "domain",
		ResourceID:   d.ID,
		Name:         d.Domain,
		CheckedAt:    now,
	}

	if dnsResult.Status != health.StatusHealthy {
		pr.Status = "down"
		pr.Message = dnsResult.Message
		pr.Error = dnsResult.Message
		return pr
	}

	if httpStatus != health.StatusHealthy {
		pr.Status = "degraded"
		pr.Message = "DNS resolves but HTTP failed"
		return pr
	}

	pr.Status = "active"
	pr.Message = "DNS resolves, HTTP reachable"
	pr.LatencyMs = dnsResult.Latency.Milliseconds()
	return pr
}

// checkServices 探测所有服务
func (r *Runner) checkServices() []ProbeResult {
	services, err := r.db.ListServices()
	if err != nil || len(services) == 0 {
		return nil
	}

	var results []ProbeResult
	for _, s := range services {
		result := r.probeService(s)
		results = append(results, result)

		// 更新服务状态
		if err := r.db.UpdateServiceStatus(s.ID, result.Status, int(result.LatencyMs), time.Now()); err != nil {
			log.Printf("[probe] Failed to update service %s: %v", s.Name, err)
		}
	}
	return results
}

// probeService 探测单个服务
func (r *Runner) probeService(s *storage.Service) ProbeResult {
	pr := ProbeResult{
		ResourceType: "service",
		ResourceID:   s.ID,
		Name:         s.Name,
		CheckedAt:    time.Now(),
	}

	target := s.URL
	if target == "" {
		target = s.Name
	}

	switch s.Type {
	case "http", "https":
		result := r.healthChecker.CheckHTTP(s.Name, target, 0)
		pr.LatencyMs = result.Latency.Milliseconds()
		if result.Status == health.StatusHealthy {
			pr.Status = "up"
			pr.Message = "HTTP " + result.Message
		} else {
			pr.Status = "down"
			pr.Error = result.Message
			pr.Message = result.Message
		}
	default:
		// TCP 端口检查：从 URL 或名称解析 host:port
		host, port := parseHostPort(target)
		if host == "" {
			host = target
			port = "80"
		}
		addr := net.JoinHostPort(host, port)
		result := r.healthChecker.CheckTCP(s.Name, addr)
		pr.LatencyMs = result.Latency.Milliseconds()
		if result.Status == health.StatusHealthy {
			pr.Status = "up"
			pr.Message = "TCP connected"
		} else {
			pr.Status = "down"
			pr.Error = result.Message
			pr.Message = result.Message
		}
	}

	return pr
}

// checkCertificates 探测所有证书
func (r *Runner) checkCertificates() []ProbeResult {
	certs, err := r.db.ListCertificates()
	if err != nil || len(certs) == 0 {
		return nil
	}

	var results []ProbeResult
	for _, c := range certs {
		result := r.probeCertificate(c)
		results = append(results, result)

		// 更新证书状态
		if err := r.db.UpdateCertificateStatus(c.ID, result.Status, c.ExpiresAt); err != nil {
			log.Printf("[probe] Failed to update certificate %s: %v", c.ID, err)
		}
	}
	return results
}

// probeCertificate 探测单个证书
func (r *Runner) probeCertificate(c *storage.Certificate) ProbeResult {
	pr := ProbeResult{
		ResourceType: "certificate",
		ResourceID:   c.ID,
		Name:         c.DomainName,
		CheckedAt:    time.Now(),
	}

	info, err := r.certMonitor.CheckDomain(c.DomainName, 443)
	if err != nil {
		pr.Status = "error"
		pr.Error = err.Error()
		pr.Message = "TLS handshake failed"
		return pr
	}

	pr.LatencyMs = int64(time.Until(info.NotAfter).Abs().Seconds())

	if info.IsExpired {
		pr.Status = "expired"
		pr.Message = "Certificate expired"
	} else if info.DaysLeft < 30 {
		pr.Status = "expiring"
		pr.Message = "Expires in " + strconv.Itoa(info.DaysLeft) + " days"
	} else {
		pr.Status = "valid"
		pr.Message = "Expires in " + strconv.Itoa(info.DaysLeft) + " days"
	}

	// 如果 cert monitor 返回了实际过期时间，也更新到 storage
	if !info.NotAfter.IsZero() {
		c.ExpiresAt = info.NotAfter
	}

	return pr
}

// parseHostPort 从 URL 或 host:port 解析主机和端口
func parseHostPort(target string) (host, port string) {
	// 移除协议前缀
	target = strings.TrimPrefix(target, "http://")
	target = strings.TrimPrefix(target, "https://")
	target = strings.TrimPrefix(target, "tcp://")

	// 分割路径
	if idx := strings.Index(target, "/"); idx >= 0 {
		target = target[:idx]
	}

	h, p, err := net.SplitHostPort(target)
	if err == nil {
		return h, p
	}

	// 没有指定端口，根据协议推断
	if strings.HasPrefix(target, "https://") {
		return target, "443"
	}
	return target, "80"
}
