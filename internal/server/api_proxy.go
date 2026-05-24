package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/google/uuid"
)

// handleProxies 获取代理列表
func (s *Server) handleProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentID := r.URL.Query().Get("agent_id")

	var proxies []*storage.Proxy
	var err error
	if agentID != "" {
		proxies, err = s.db.ListProxies(agentID)
	} else {
		proxies, err = s.db.ListProxies("")
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 获取状态信息
	result := make([]map[string]interface{}, len(proxies))
	for i, p := range proxies {
		result[i] = map[string]interface{}{
			"id":          p.ID,
			"name":        p.Name,
			"agentId":     p.AgentID,
			"proxyType":   p.ProxyType,
			"remotePort":  p.RemotePort,
			"target":      p.Target,
			"description": p.Description,
			"enabled":     p.Enabled,
			"createdAt":   p.CreatedAt,
			"updatedAt":   p.UpdatedAt,
		}

		// 添加状态信息（如果代理管理器可用）
		if s.proxyMgr != nil {
			if status, err := s.proxyMgr.GetProxyStatus(p.ID); err == nil {
				result[i]["status"] = status["status"]
				result[i]["connCount"] = status["connCount"]
			} else {
				result[i]["status"] = "stopped"
				result[i]["connCount"] = 0
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": result})
}

// handleProxyCreate 创建代理
func (s *Server) handleProxyCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name        string `json:"name"`
		AgentID     string `json:"agentId"`
		ProxyType   string `json:"proxyType"`
		RemotePort  int    `json:"remotePort"`
		Target      string `json:"target"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 验证
	if req.Name == "" || req.AgentID == "" || req.Target == "" || req.RemotePort <= 0 {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	if req.ProxyType != "tcp" && req.ProxyType != "udp" {
		req.ProxyType = "tcp" // 默认 TCP
	}

	// 检查 Agent 是否存在
	if _, err := s.db.GetAgent(req.AgentID); err != nil {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}

	// 检查端口是否已被占用
	if existing, _ := s.db.GetProxyByRemotePort(req.RemotePort); existing != nil {
		http.Error(w, "Port already in use", http.StatusConflict)
		return
	}

	// 创建代理配置
	proxy := &storage.Proxy{
		ID:          uuid.New().String(),
		Name:        req.Name,
		AgentID:     req.AgentID,
		ProxyType:   req.ProxyType,
		RemotePort:  req.RemotePort,
		Target:      req.Target,
		Description: req.Description,
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 获取当前用户
	userInfo, _ := auth.GetUserFromContext(r)
	proxy.CreatedBy = userInfo.Username

	if err := s.db.CreateProxy(proxy); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 如果启用，启动代理
	if proxy.Enabled && s.proxyMgr != nil {
		if err := s.proxyMgr.StartProxy(proxy); err != nil {
			// 记录错误但不返回，因为配置已保存
			http.Error(w, "Proxy created but failed to start: "+err.Error(), http.StatusAccepted)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(proxy)
}

// handleProxyUpdate 更新代理
func (s *Server) handleProxyUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从 URL 获取代理 ID
	// 假设 URL 格式为 /api/proxies/{id}
	// 这里简化处理，从请求体获取

	var req struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		AgentID     string `json:"agentId"`
		ProxyType   string `json:"proxyType"`
		RemotePort  int    `json:"remotePort"`
		Target      string `json:"target"`
		Description string `json:"description"`
		Enabled     *bool  `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "Missing proxy id", http.StatusBadRequest)
		return
	}

	// 获取现有代理
	proxy, err := s.db.GetProxy(req.ID)
	if err != nil {
		http.Error(w, "Proxy not found", http.StatusNotFound)
		return
	}

	// 更新字段
	if req.Name != "" {
		proxy.Name = req.Name
	}
	if req.AgentID != "" {
		proxy.AgentID = req.AgentID
	}
	if req.ProxyType != "" {
		proxy.ProxyType = req.ProxyType
	}
	if req.RemotePort > 0 {
		// 检查新端口是否已被占用
		if existing, _ := s.db.GetProxyByRemotePort(req.RemotePort); existing != nil && existing.ID != req.ID {
			http.Error(w, "Port already in use", http.StatusConflict)
			return
		}
		proxy.RemotePort = req.RemotePort
	}
	if req.Target != "" {
		proxy.Target = req.Target
	}
	if req.Description != "" {
		proxy.Description = req.Description
	}
	if req.Enabled != nil {
		proxy.Enabled = *req.Enabled
	}
	proxy.UpdatedAt = time.Now()

	if err := s.db.UpdateProxy(proxy); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 重新加载代理
	if s.proxyMgr != nil {
		s.proxyMgr.ReloadProxy(proxy)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(proxy)
}

// handleProxyDelete 删除代理
func (s *Server) handleProxyDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// 尝试从查询参数获取
		req.ID = r.URL.Query().Get("id")
	}

	if req.ID == "" {
		http.Error(w, "Missing proxy id", http.StatusBadRequest)
		return
	}

	// 先停止代理
	if s.proxyMgr != nil {
		s.proxyMgr.StopProxy(req.ID)
	}

	// 删除配置
	if err := s.db.DeleteProxy(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleProxyStatus 获取代理状态
func (s *Server) handleProxyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.proxyMgr == nil {
		http.Error(w, "Proxy manager not available", http.StatusServiceUnavailable)
		return
	}

	proxyID := r.URL.Query().Get("id")
	if proxyID != "" {
		// 获取单个代理状态
		status, err := s.proxyMgr.GetProxyStatus(proxyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	} else {
		// 获取所有代理状态
		status := s.proxyMgr.GetAllStatus()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": status})
	}
}

// registerProxyAPI 注册代理 API
func (s *Server) registerProxyAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/proxies", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			auth.Middleware(s.handleProxies)(w, r)
		case http.MethodPost:
			auth.Middleware(s.handleProxyCreate)(w, r)
		case http.MethodPut, http.MethodPatch:
			auth.Middleware(s.handleProxyUpdate)(w, r)
		case http.MethodDelete:
			auth.Middleware(s.handleProxyDelete)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/proxies/status", func(w http.ResponseWriter, r *http.Request) {
		auth.Middleware(s.handleProxyStatus)(w, r)
	})
}
