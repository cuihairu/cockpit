package server

import (
	"log"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
)

// cleanupLoop 定期清理离线 Agent 与过期终端录制
// cleanupLoop 间隔。包级变量仅为测试可注入，默认值即生产取值。
var cleanupLoopInterval = 30 * time.Second

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
		now := time.Now()
		nextCleanup := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
		if nextCleanup.Before(now) {
			nextCleanup = nextCleanup.Add(24 * time.Hour)
		}
		return time.Until(nextCleanup)
	}
)

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
