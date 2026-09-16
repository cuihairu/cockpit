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

// 探测失败阈值边界：连续失败多少次判定 down（过滤瞬时抖动）
const (
	MinFailThreshold     = 1
	MaxFailThreshold     = 10
	DefaultFailThreshold = 2
)

// IntervalSettingKey 探测间隔在 storage.Setting 表中的键
const IntervalSettingKey = "probe.interval_seconds"

// FailThresholdSettingKey 连续失败判定阈值在 storage.Setting 表中的键
const FailThresholdSettingKey = "probe.fail_threshold"

// probeHistoryRetention 拨测历史保留期；probeHistoryPruneInterval 清理频率
const (
	probeHistoryRetention     = 30 * 24 * time.Hour
	probeHistoryPruneInterval = 24 * time.Hour
)

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
	// failThreshold 连续失败判定 down 的次数，同 interval 的 atomic 模式
	failThreshold atomic.Int64
	notifier      *notification.Service
	// serviceStates 服务边沿状态；仅在探测循环 goroutine 内访问，无需加锁
	serviceStates map[string]*serviceState
	// lastPrune 上次历史清理时间；仅在探测循环 goroutine 内访问
	lastPrune time.Time
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
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
	r.failThreshold.Store(DefaultFailThreshold)
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

// FailThreshold 当前连续失败判定阈值
func (r *Runner) FailThreshold() int {
	n := r.failThreshold.Load()
	if n < MinFailThreshold || n > MaxFailThreshold {
		return DefaultFailThreshold
	}
	return int(n)
}

// SetFailThreshold 动态调整失败判定阈值（下一轮探测生效）；
// 超出 [Min, Max] 边界的值被夹紧，调用方（API 层）已先行校验。
func (r *Runner) SetFailThreshold(n int) {
	if n < MinFailThreshold {
		n = MinFailThreshold
	}
	if n > MaxFailThreshold {
		n = MaxFailThreshold
	}
	r.failThreshold.Store(int64(n))
}

// LoadFailThreshold 从 Setting 表读取持久化的失败阈值（启动时调用一次）。
// 与 SetFailThreshold 的夹紧语义不同：存储值仅在合法范围内才应用，
// 越界/非法值忽略并保持当前值。
func (r *Runner) LoadFailThreshold() {
	if r.db == nil {
		return
	}
	v, err := r.db.GetSetting(FailThresholdSettingKey)
	if err != nil || v == "" {
		return
	}
	if n, convErr := strconv.Atoi(v); convErr == nil && n >= MinFailThreshold && n <= MaxFailThreshold {
		r.failThreshold.Store(int64(n))
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

	// 历史落库 + 按需清理保留期外记录（失败只记日志，不阻断探测主流程）
	r.recordHistory(results.Results)
	r.maybePruneHistory()

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

// recordHistory 一轮探测结果批量写入历史表（D8/D9：三类目标全记，失败不阻断）
func (r *Runner) recordHistory(results []ProbeResult) {
	if r.db == nil || len(results) == 0 {
		return
	}
	rows := make([]*storage.ProbeResult, 0, len(results))
	for _, res := range results {
		rows = append(rows, &storage.ProbeResult{
			ResourceType: res.ResourceType,
			ResourceID:   res.ResourceID,
			Name:         res.Name,
			Status:       res.Status,
			LatencyMs:    int(res.LatencyMs),
			Message:      res.Message,
			CheckedAt:    res.CheckedAt,
		})
	}
	if err := r.db.CreateProbeResults(rows); err != nil {
		log.Printf("[probe] Failed to record history: %v", err)
	}
}

// maybePruneHistory 距上次清理超过 24h 时清一次保留期外历史（D10）；
// 仅探测循环内调用，lastPrune 无需加锁
func (r *Runner) maybePruneHistory() {
	if r.db == nil || time.Since(r.lastPrune) < probeHistoryPruneInterval {
		return
	}
	r.lastPrune = time.Now()
	cutoff := time.Now().Add(-probeHistoryRetention)
	if n, err := r.db.DeleteProbeResultsOlderThan(cutoff); err != nil {
		log.Printf("[probe] History prune failed: %v", err)
	} else if n > 0 {
		log.Printf("[probe] Pruned %d probe history rows older than %s", n, cutoff.Format("2006-01-02"))
	}
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
// 连续 FailThreshold() 次失败才判定 down（过滤瞬时抖动），
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
		// >= 而非 ==：阈值运行中被调小后仍能触发
		if st.failCount >= r.FailThreshold() && !st.notified {
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

// probeCertPort 证书探测端口。包级变量仅为测试可注入，默认值即生产取值。
var probeCertPort = 443

// probeCertificate 探测单个证书
func (r *Runner) probeCertificate(c *storage.Certificate) ProbeResult {
	pr := ProbeResult{
		ResourceType: "certificate",
		ResourceID:   c.ID,
		Name:         c.DomainName,
		CheckedAt:    time.Now(),
	}

	info, err := r.certMonitor.CheckDomain(c.DomainName, probeCertPort)
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
