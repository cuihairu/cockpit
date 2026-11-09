package server

import (
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/auth"
)

// ============ 警告/通知处理 ============

// handleAlertsList 获取警告列表或标记所有已读
func (s *Server) handleAlertsList(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	_, ok := auth.GetUserFromContext(r)
	if !ok {
		s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// 处理标记所有已读的请求
	if r.Method == http.MethodPut && r.URL.Path == "/api/alerts/read-all" {
		if err := s.db.MarkAllAlertsAsRead(); err != nil {
			s.handleError(w, r, http.StatusInternalServerError, "Failed to mark all alerts as read")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"message": "All alerts marked as read"})
		return
	}

	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	alerts, err := s.db.ListAlerts(50) // 最近50条
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list alerts")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  alerts,
		"total": len(alerts),
	})
}

// handleAlertActions 处理警告操作
func (s *Server) handleAlertActions(w http.ResponseWriter, r *http.Request, path string) {
	// 获取当前用户
	_, ok := auth.GetUserFromContext(r)
	if !ok {
		s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}

	parts := strings.Split(path, "/") // strings.Split 结果至少含一个元素

	alertID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "read" && r.Method == http.MethodPut:
		s.handleMarkAlertAsRead(w, r, alertID)
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleMarkAlertAsRead 标记警告为已读
func (s *Server) handleMarkAlertAsRead(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.db.MarkAlertAsRead(id); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to mark alert as read")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Alert marked as read"})
}
