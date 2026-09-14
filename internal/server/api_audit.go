package server

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
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
	filters := parseAuditFilters(r)

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

func (s *Server) handleAuditLogsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filters := parseAuditFilters(r)
	logs, _, err := s.db.GetAuditLogs(0, 10000, filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit-logs.csv"`)

	writer := csv.NewWriter(w)
	defer writer.Flush()

	if err := writer.Write([]string{
		"id",
		"created_at",
		"username",
		"action",
		"resource",
		"status",
		"ip",
		"resource_id",
		"details",
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, log := range logs {
		if err := writer.Write([]string{
			strconv.FormatUint(uint64(log.ID), 10),
			log.CreatedAt.Format(time.RFC3339),
			log.Username,
			log.Action,
			log.Resource,
			log.Status,
			log.IP,
			log.ResourceID,
			log.Details,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func parseAuditFilters(r *http.Request) map[string]interface{} {
	query := r.URL.Query()
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
	if startTime := query.Get("start_time"); startTime != "" {
		if parsed, err := time.Parse(time.RFC3339, startTime); err == nil {
			filters["start_time"] = parsed
		}
	}
	if endTime := query.Get("end_time"); endTime != "" {
		if parsed, err := time.Parse(time.RFC3339, endTime); err == nil {
			filters["end_time"] = parsed
		}
	}

	return filters
}

// registerAuditAPI 注册审计日志 API
func (s *Server) registerAuditAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/audit/logs", func(w http.ResponseWriter, r *http.Request) {
		// 需要认证
		s.authService().Middleware(s.handleAuditLogs)(w, r)
	})
	mux.HandleFunc("/api/admin/audit/export", func(w http.ResponseWriter, r *http.Request) {
		s.authService().Middleware(s.handleAuditLogsExport)(w, r)
	})
	mux.HandleFunc("/api/admin/audit/stats", func(w http.ResponseWriter, r *http.Request) {
		// 需要认证
		s.authService().Middleware(s.handleAuditLogStats)(w, r)
	})
}
