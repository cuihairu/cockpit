package server

import (
	"log"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
)

// cleanupLoop 定期清理离线 Agent
func (s *Server) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			removed := s.registry.CleanupOffline()
			if len(removed) > 0 {
				log.Printf("Cleaned up offline agents: %v", removed)
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// alertCheckLoop 定期检查并生成警告
func (s *Server) alertCheckLoop() {
	// 启动时立即执行一次
	go func() {
		time.Sleep(5 * time.Second) // 等待服务完全启动
		s.runAlertChecks()
	}()

	// 每小时检查一次
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// 每天凌晨2点清理旧警告
	cleanupTicker := time.NewTicker(24 * time.Hour)
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
	generator := alert.NewGenerator(s.db, s.notification, s.cfg.Notification)
	generator.CheckAllChecks()
	log.Println("Alert checks completed")
}

// cleanupOldAlerts 清理旧警告
func (s *Server) cleanupOldAlerts() {
	generator := alert.NewGenerator(s.db, s.notification, s.cfg.Notification)
	generator.CleanupOldAlerts(30 * 24 * time.Hour) // 保留30天
	log.Println("Old alerts cleaned up")
}

// metricsCleanupLoop 清理旧的系统指标
func (s *Server) metricsCleanupLoop() {
	// 每天凌晨3点清理
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// 启动时先等待到下次清理时间
	now := time.Now()
	nextCleanup := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	if nextCleanup.Before(now) {
		nextCleanup = nextCleanup.Add(24 * time.Hour)
	}
	time.Sleep(time.Until(nextCleanup))

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
