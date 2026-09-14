package probe

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cuihairu/cockpit/internal/cert"
	"github.com/cuihairu/cockpit/internal/health"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 探测间隔边界（秒）：下限防自 DDoS，上限不如关闭拨测
const (
	MinIntervalSeconds = 30
	MaxIntervalSeconds = 3600
	DefaultInterval    = 5 * time.Minute
)

// IntervalSettingKey 探测间隔在 storage.Setting 表中的键
const IntervalSettingKey = "probe.interval_seconds"

// serviceFailureThreshold 连续失败多少次才判定 down 并通知（过滤瞬时抖动）
const serviceFailureThreshold = 2

// ProbeResult 单次探测结果
type ProbeResult struct {
	ResourceType string    `json:"resource_type"` // domain / service / certificate
	ResourceID   string    `json:"resource_id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"` // healthy / unhealthy / degraded
	LatencyMs    int64     `json:"latency_ms"`
	Message      string    `json:"message"`
	CheckedAt    time.Time `json:"checked_at"`
	Error        string    `json:"error,omitempty"`
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

// serviceState 单个服务的连续失败计数与已通知标记（拨测边沿通知用）
type serviceState struct {
	failCount int
	notified  bool
}

// Runner 自动健康探测运行器
type Runner struct {
	db            *storage.DB
	healthChecker *health.Checker
	certMonitor   *cert.Monitor
	// intervalSeconds 探测间隔（秒）。atomic 读写：API 线程 SetInterval，
	// 探测循环 time.After(Interval()) 每轮重读，间隔变化下一轮生效。
	intervalSeconds atomic.Int64
	notifier        *notification.Service
	// serviceStates 服务边沿状态；仅在探测循环 goroutine 内访问，无需加锁
	serviceStates map[string]*serviceState
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// NewRunner 创建探测运行器
func NewRunner(db *storage.DB, interval time.Duration, notifier *notification.Service) *Runner {
	if interval <= 0 {
		interval = DefaultInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{
		db:            db,
		healthChecker: health.NewChecker(health.Config{Timeout: 10 * time.Second}),
		certMonitor:   cert.NewMonitor(cert.Config{Timeout: 10 * time.Second}),
		notifier:      notifier,
		serviceStates: make(map[string]*serviceState),
		ctx:           ctx,
		cancel:        cancel,
	}
	r.intervalSeconds.Store(int64(interval / time.Second))
	return r
}

// Interval 当前探测间隔
func (r *Runner) Interval() time.Duration {
	secs := r.intervalSeconds.Load()
	if secs <= 0 {
		secs = int64(DefaultInterval / time.Second)
	}
	return time.Duration(secs) * time.Second
}

// SetInterval 动态调整探测间隔（下一轮探测生效）；非正值忽略。
// 超出 [Min, Max] 边界的值被夹紧，调用方（API 层）已先行校验。
func (r *Runner) SetInterval(d time.Duration) {
	secs := int64(d / time.Second)
	if secs <= 0 {
		return
	}
	if secs < MinIntervalSeconds {
		secs = MinIntervalSeconds
	}
	if secs > MaxIntervalSeconds {
		secs = MaxIntervalSeconds
	}
	r.intervalSeconds.Store(secs)
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

		for {
			// 每轮重读间隔：SetInterval 后下一轮天然生效（Ticker 做不到）
			select {
			case <-time.After(r.Interval()):
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

		// 边沿通知：连续失败达阈值发 down，恢复发 up（仅在探测循环 goroutine 内执行）
		r.trackServiceEdge(result)
	}
	return results
}

// trackServiceEdge 服务状态边沿检测与即时通知。
// 连续 serviceFailureThreshold 次失败才判定 down（过滤瞬时抖动），
// 通知发出后不再重复；恢复（探测成功）时发 service.up 恢复通知。
// 事件是否真正投递由 notification.Service 按 cfg.Events 白名单过滤。
func (r *Runner) trackServiceEdge(res ProbeResult) {
	key := res.ResourceType + "/" + res.ResourceID
	st := r.serviceStates[key]
	if st == nil {
		st = &serviceState{}
		r.serviceStates[key] = st
	}

	if res.Error != "" || res.Status == "down" {
		st.failCount++
		if st.failCount == serviceFailureThreshold && !st.notified {
			st.notified = true
			r.notifyService(res, notification.ServiceDown,
				fmt.Sprintf("服务 %s 连续 %d 次探测失败：%s", res.Name, st.failCount, res.Message))
		}
		return
	}

	// 探测成功：失败计数归零；此前已通知过 down 则补发恢复通知
	if st.notified {
		st.notified = false
		st.failCount = 0
		r.notifyService(res, notification.ServiceUp,
			fmt.Sprintf("服务 %s 已恢复（延迟 %dms）", res.Name, res.LatencyMs))
		return
	}
	st.failCount = 0
}

// notifyService 通过通知服务异步发送服务状态变化通知
func (r *Runner) notifyService(res ProbeResult, eventType, message string) {
	if r.notifier == nil {
		return
	}
	level := "info"
	if eventType == notification.ServiceDown {
		level = "error"
	}
	r.notifier.SendNonBlocking(&notification.Notification{
		EventType:    eventType,
		Title:        res.Name,
		Message:      message,
		Level:        level,
		ResourceType: res.ResourceType,
		ResourceID:   res.ResourceID,
		Time:         time.Now(),
	})
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
