package server

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/config"
)

// cleanupLoop 定期清理离线 Agent 与过期终端录制
// cleanupLoop 间隔。包级变量仅为测试可注入，默认值即生产取值。
var cleanupLoopInterval = 30 * time.Second

// applyAgentExpireEnv 让 AGENT_EXPIRE_MINUTES 环境变量覆盖 yaml 的
// agent.expire_minutes（部署面习惯用 env 调运行参数，见 /etc/default/cockpit-server）。
func applyAgentExpireEnv(cfg *config.Config) {
	raw := os.Getenv("AGENT_EXPIRE_MINUTES")
	if raw == "" || cfg == nil || cfg.Agent == nil {
		return
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("Invalid AGENT_EXPIRE_MINUTES %q, keep %d: %v", raw, cfg.Agent.ExpireMinutes, err)
		return
	}
	cfg.Agent.ExpireMinutes = v
}

// agentExpireThreshold Agent 过期判定阈值；0 = 自动过期已关闭（配置为负值）。
// nil-safe：测试直接构造 &Server{} 不带 cfg 时也返回默认 5 分钟。
func (s *Server) agentExpireThreshold() time.Duration {
	const def = 5
	minutes := def
	if s.cfg != nil && s.cfg.Agent != nil {
		minutes = s.cfg.Agent.ExpireMinutes
	}
	if minutes < 0 {
		return 0
	}
	return time.Duration(minutes) * time.Minute
}

// expireStaleAgents 过期 Agent 自动判定（D-2026-10-08-3，用户「清理过期
// agent」闭环）：把 DB 里 last_seen 超过阈值仍标 online 的假在线行批量标
// offline，并顺带释放其内存注册表资源（关连接）。列表侧的过期隐藏由 web
// 按 /api/status 下发的 agentExpireMinutes 执行——DB 行不删，密钥/标签/
// 档案保留，物理清除走既有手动「清理离线 agent」钮。
func (s *Server) expireStaleAgents() {
	threshold := s.agentExpireThreshold()
	if threshold <= 0 {
		return
	}
	stale, err := s.db.MarkStaleAgentsOffline(time.Now().Add(-threshold))
	if err != nil {
		log.Printf("Agent expiry sweep failed: %v", err)
		return
	}
	for _, id := range stale {
		// 资源释放：Unregister 内部已 Close（关 Send 通道 + WS 连接）
		s.registry.Unregister(id)
	}
	if len(stale) > 0 {
		log.Printf("Expired %d agent(s) with no heartbeat for %s: %v",
			len(stale), threshold, stale)
	}
}

func (s *Server) cleanupLoop() {
	ticker := time.NewTicker(cleanupLoopInterval)
	defer ticker.Stop()

	var lastRecordingCleanup time.Time

	for {
		select {
		case <-ticker.C:
			removed := s.registry.CleanupOffline()
			if len(removed) > 0 {
				log.Printf("Cleaned up offline agents: %v", removed)
			}
			// 过期判定：心跳/上报超阈值 → 假在线标离线 + 资源释放（有日志）
			s.expireStaleAgents()
			// 过期终端录制清理（小时节流，见 recording-design.md D6）
			if time.Since(lastRecordingCleanup) > recordingCleanupInterval {
				lastRecordingCleanup = time.Now()
				s.cleanupExpiredRecordings()
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// alertCheckLoop 定期检查并生成警告
// 各间隔/延迟。包级变量仅为测试可注入，默认值即生产取值。
var (
	alertCheckStartDelay = 5 * time.Second // 等待服务完全启动
	alertCheckInterval   = time.Hour       // 每小时检查一次
	alertCleanupInterval = 24 * time.Hour  // 每天清理旧警告
)

func (s *Server) alertCheckLoop() {
	// 启动时立即执行一次（延迟先读到局部量：该 goroutine 不再读包级变量，
	// 测试注入/恢复与之并发安全；ctx 取消时不再补跑）
	delay := alertCheckStartDelay
	go func() {
		select {
		case <-time.After(delay):
			s.runAlertChecks()
		case <-s.ctx.Done():
		}
	}()

	ticker := time.NewTicker(alertCheckInterval)
	defer ticker.Stop()

	cleanupTicker := time.NewTicker(alertCleanupInterval)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runAlertChecks()
		case <-cleanupTicker.C:
			s.cleanupOldAlerts()
		case <-s.ctx.Done():
			return
		}
	}
}

// runAlertChecks 执行警告检查
func (s *Server) runAlertChecks() {
	generator := alert.NewGenerator(s.db, s.notifier, s.cfg.Notification)
	generator.CheckAllChecks()
	log.Println("Alert checks completed")
}

// cleanupOldAlerts 清理旧警告
func (s *Server) cleanupOldAlerts() {
	generator := alert.NewGenerator(s.db, s.notifier, s.cfg.Notification)
	generator.CleanupOldAlerts(30 * 24 * time.Hour) // 保留30天
	log.Println("Old alerts cleaned up")
}

// metricsCleanupLoop 清理旧的系统指标
var (
	metricsCleanupInterval = 24 * time.Hour // 每天凌晨3点清理
	// metricsCleanupFirstWait 计算距下次凌晨 3 点的等待时长。
	// 包级变量仅为测试可注入，默认值即生产取值。
	metricsCleanupFirstWait = func() time.Duration {
		return nextMetricsCleanupWait(time.Now())
	}
)

// nextMetricsCleanupWait 纯函数版首等时长：传入 now 计算距下一个凌晨 3 点的
// 等待（已过今天 3 点则顺延明天）。抽出为可注入时间源的纯函数，使「顺延」
// 分支可在任意钟点被确定性测试（原闭包体内分支只在 03:00 后运行才执行）。
func nextMetricsCleanupWait(now time.Time) time.Duration {
	nextCleanup := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	if nextCleanup.Before(now) {
		nextCleanup = nextCleanup.Add(24 * time.Hour)
	}
	return nextCleanup.Sub(now)
}

func (s *Server) metricsCleanupLoop() {
	ticker := time.NewTicker(metricsCleanupInterval)
	defer ticker.Stop()

	// 启动时先等待到下次清理时间
	time.Sleep(metricsCleanupFirstWait())

	for {
		// 清理30天前的数据
		count, err := s.db.CleanupOldMetrics(30 * 24 * time.Hour)
		if err != nil {
			log.Printf("Failed to cleanup old metrics: %v", err)
		} else {
			log.Printf("Cleaned up %d old metric records", count)
		}

		select {
		case <-ticker.C:
			// 继续下一次清理
		case <-s.ctx.Done():
			return
		}
	}
}
