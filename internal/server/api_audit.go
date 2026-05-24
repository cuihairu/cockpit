package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/cuihairu/cockpit/internal/auth"
)

// handleAuditLogs 获取审计日志列表
func (s *Server) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	// 只允许 GET 请求
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析查询参数
	query := r.URL.Query()

	// 分页参数
	page, _ := strconv.Atoi(query.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(query.Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// 过滤参数
	filters := make(map[string]interface{})
	if action := query.Get("action"); action != "" {
		filters["action"] = action
	}
	if resource := query.Get("resource"); resource != "" {
		filters["resource"] = resource
	}
	if username := query.Get("username"); username != "" {
		filters["username"] = username
	}
	if status := query.Get("status"); status != "" {
		filters["status"] = status
	}

	// 查询数据
	logs, total, err := s.db.GetAuditLogs(offset, pageSize, filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 返回结果
	response := map[string]interface{}{
		"data": logs,
		"pagination": map[string]interface{}{
			"page":       page,
			"page_size":  pageSize,
			"total":      total,
			"total_page": (total + int64(pageSize) - 1) / int64(pageSize),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleAuditLogStats 获取审计日志统计
func (s *Server) handleAuditLogStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 获取统计数据
	stats, err := s.db.GetAuditLogStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// registerAuditAPI 注册审计日志 API
func (s *Server) registerAuditAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/audit/logs", func(w http.ResponseWriter, r *http.Request) {
		// 需要认证
		auth.Middleware(s.handleAuditLogs)(w, r)
	})
	mux.HandleFunc("/api/admin/audit/stats", func(w http.ResponseWriter, r *http.Request) {
		// 需要认证
		auth.Middleware(s.handleAuditLogStats)(w, r)
	})
}
