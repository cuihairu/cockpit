package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/cuihairu/cockpit/internal/auth"
)

// registerMetricsAPI 注册系统指标 API
func (s *Server) registerMetricsAPI(mux *http.ServeMux) {
	// 获取系统信息快照（所有 Agent）
	mux.HandleFunc("/api/metrics/snapshots", func(w http.ResponseWriter, r *http.Request) {
		auth.Middleware(s.handleSnapshots)(w, r)
	})

	// 获取单个 Agent 的系统信息
	mux.HandleFunc("/api/metrics/snapshot", func(w http.ResponseWriter, r *http.Request) {
		auth.Middleware(s.handleSnapshot)(w, r)
	})

	// 获取历史指标
	mux.HandleFunc("/api/metrics/history", func(w http.ResponseWriter, r *http.Request) {
		auth.Middleware(s.handleMetricsHistory)(w, r)
	})
}

// handleSnapshots 获取所有系统信息快照
func (s *Server) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snapshots, err := s.db.ListSystemInfoSnapshots()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": snapshots})
}

// handleSnapshot 获取单个 Agent 的系统信息
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		http.Error(w, "Missing agent_id parameter", http.StatusBadRequest)
		return
	}

	snapshot, err := s.db.GetSystemInfoSnapshot(agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot)
}

// handleMetricsHistory 获取历史指标
func (s *Server) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		http.Error(w, "Missing agent_id parameter", http.StatusBadRequest)
		return
	}

	// 解析时间范围
	var start, end time.Time
	if startStr := r.URL.Query().Get("start"); startStr != "" {
		if ts, err := strconv.ParseInt(startStr, 10, 64); err == nil {
			start = time.Unix(ts, 0)
		}
	} else {
		// 默认最近24小时
		start = time.Now().Add(-24 * time.Hour)
	}

	if endStr := r.URL.Query().Get("end"); endStr != "" {
		if ts, err := strconv.ParseInt(endStr, 10, 64); err == nil {
			end = time.Unix(ts, 0)
		}
	} else {
		end = time.Now()
	}

	// 解析 limit
	limit := 1000
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	metrics, err := s.db.GetSystemMetricsByTimeRange(agentID, start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 限制返回数量
	if len(metrics) > limit {
		metrics = metrics[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": metrics,
		"start": start.Unix(),
		"end": end.Unix(),
		"count": len(metrics),
	})
}
