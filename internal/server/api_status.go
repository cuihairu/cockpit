package server

import (
	"net/http"
	"time"
)

// handleStatus 获取系统状态
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	stats := s.db.GetStats()

	status := map[string]interface{}{
		"services": map[string]interface{}{
			"running": stats.ServicesUp,
			"down":    stats.ServicesDown,
			"unknown": 0,
		},
		"domains": map[string]interface{}{
			"valid":    stats.DomainsActive,
			"expiring": 0,
		},
		"certificates": map[string]interface{}{
			"valid":    stats.CertificatesValid,
			"expiring": stats.CertificatesExpiring,
		},
		"infrastructure": map[string]interface{}{
			"total":  stats.AgentsTotal,
			"online": stats.AgentsOnline,
		},
		// Agent 过期阈值（分钟，0=自动过期已关闭）：web 列表据此隐藏过期
		// 行并在清理钮旁展示口径（D-2026-10-08-3）
		"agentExpireMinutes": int(s.agentExpireThreshold() / time.Minute),
	}

	s.writeJSON(w, http.StatusOK, status)
}
