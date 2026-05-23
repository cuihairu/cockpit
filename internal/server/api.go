package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/storage"
)

// serveAPI 处理 API 请求
func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	// 设置 CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// 处理 OPTIONS 预检请求
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 解析路径
	path := strings.TrimPrefix(r.URL.Path, "/api")

	// 路由分发
	switch {
	case path == "/status":
		s.handleStatus(w, r)
	case path == "/agents":
		s.handleAgentsList(w, r)
	case strings.HasPrefix(path, "/agents/"):
		s.handleAgentGet(w, r, strings.TrimPrefix(path, "/agents/"))
	case strings.HasPrefix(path, "/resources/"):
		s.handleResources(w, r, strings.TrimPrefix(path, "/resources/"))
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// handleStatus 获取系统状态
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	stats, err := s.db.GetStats()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to get stats")
		return
	}

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
	}

	s.writeJSON(w, http.StatusOK, status)
}

// handleAgentsList 获取 Agent 列表
func (s *Server) handleAgentsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	agents, err := s.db.ListAgents()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list agents")
		return
	}

	result := make([]map[string]interface{}, 0, len(agents))
	for _, agent := range agents {
		result = append(result, storageAgentToResponse(agent))
	}

	s.writeJSON(w, http.StatusOK, result)
}

// handleAgentGet 获取单个 Agent 详情
func (s *Server) handleAgentGet(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	agent, err := s.db.GetAgent(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "Agent not found")
		return
	}

	s.writeJSON(w, http.StatusOK, storageAgentToResponse(agent))
}

// handleResources 处理资源请求
func (s *Server) handleResources(w http.ResponseWriter, r *http.Request, resourcePath string) {
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// 解析资源类型和可能的 ID
	parts := strings.Split(resourcePath, "/")
	resourceType := parts[0]

	switch resourceType {
	case "compute-instances":
		if len(parts) > 1 {
			s.handleComputeInstanceGet(w, r, parts[1])
		} else {
			s.handleComputeInstancesList(w, r)
		}
	case "domains":
		if len(parts) > 1 {
			s.handleDomainGet(w, r, parts[1])
		} else {
			s.handleDomainsList(w, r)
		}
	case "certificates":
		if len(parts) > 1 {
			s.handleCertificateGet(w, r, parts[1])
		} else {
			s.handleCertificatesList(w, r)
		}
	case "services":
		if len(parts) > 1 {
			s.handleServiceGet(w, r, parts[1])
		} else {
			s.handleServicesList(w, r)
		}
	case "gateways":
		if len(parts) > 1 {
			s.handleGatewayGet(w, r, parts[1])
		} else {
			s.handleGatewaysList(w, r)
		}
	case "storages":
		if len(parts) > 1 {
			s.handleStorageGet(w, r, parts[1])
		} else {
			s.handleStoragesList(w, r)
		}
	default:
		s.handleError(w, r, http.StatusNotFound, "Resource type not found")
	}
}

// handleComputeInstancesList 计算实例列表
func (s *Server) handleComputeInstancesList(w http.ResponseWriter, r *http.Request) {
	instances, err := s.db.ListComputeInstances(nil)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list compute instances")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       instances,
		"total":      len(instances),
		"page":       1,
		"pageSize":   len(instances),
		"totalPages": 1,
	})
}

// handleComputeInstanceGet 获取单个计算实例
func (s *Server) handleComputeInstanceGet(w http.ResponseWriter, r *http.Request, id string) {
	s.handleError(w, r, http.StatusNotImplemented, "Not implemented yet")
}

// handleDomainsList 域名列表
func (s *Server) handleDomainsList(w http.ResponseWriter, r *http.Request) {
	domains, err := s.db.ListDomains()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list domains")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       domains,
		"total":      len(domains),
		"page":       1,
		"pageSize":   len(domains),
		"totalPages": 1,
	})
}

// handleDomainGet 获取单个域名
func (s *Server) handleDomainGet(w http.ResponseWriter, r *http.Request, id string) {
	s.handleError(w, r, http.StatusNotImplemented, "Not implemented yet")
}

// handleCertificatesList 证书列表
func (s *Server) handleCertificatesList(w http.ResponseWriter, r *http.Request) {
	certificates, err := s.db.ListCertificates()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list certificates")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       certificates,
		"total":      len(certificates),
		"page":       1,
		"pageSize":   len(certificates),
		"totalPages": 1,
	})
}

// handleCertificateGet 获取单个证书
func (s *Server) handleCertificateGet(w http.ResponseWriter, r *http.Request, id string) {
	s.handleError(w, r, http.StatusNotImplemented, "Not implemented yet")
}

// handleServicesList 服务列表
func (s *Server) handleServicesList(w http.ResponseWriter, r *http.Request) {
	services, err := s.db.ListServices()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list services")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       services,
		"total":      len(services),
		"page":       1,
		"pageSize":   len(services),
		"totalPages": 1,
	})
}

// handleServiceGet 获取单个服务
func (s *Server) handleServiceGet(w http.ResponseWriter, r *http.Request, id string) {
	s.handleError(w, r, http.StatusNotImplemented, "Not implemented yet")
}

// handleGatewaysList 网关列表
func (s *Server) handleGatewaysList(w http.ResponseWriter, r *http.Request) {
	gateways, err := s.db.ListGateways()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list gateways")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       gateways,
		"total":      len(gateways),
		"page":       1,
		"pageSize":   len(gateways),
		"totalPages": 1,
	})
}

// handleGatewayGet 获取单个网关
func (s *Server) handleGatewayGet(w http.ResponseWriter, r *http.Request, id string) {
	s.handleError(w, r, http.StatusNotImplemented, "Not implemented yet")
}

// handleStoragesList 存储列表
func (s *Server) handleStoragesList(w http.ResponseWriter, r *http.Request) {
	storages, err := s.db.ListStorages()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list storages")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       storages,
		"total":      len(storages),
		"page":       1,
		"pageSize":   len(storages),
		"totalPages": 1,
	})
}

// handleStorageGet 获取单个存储
func (s *Server) handleStorageGet(w http.ResponseWriter, r *http.Request, id string) {
	s.handleError(w, r, http.StatusNotImplemented, "Not implemented yet")
}

// storageAgentToResponse 将存储 Agent 转换为 API 响应格式
func storageAgentToResponse(agent *storage.Agent) map[string]interface{} {
	capabilities := make([]string, 0, len(agent.Capabilities))
	for _, cap := range agent.Capabilities {
		capabilities = append(capabilities, cap.Type)
	}

	return map[string]interface{}{
		"id":           agent.ID,
		"hostname":     agent.Hostname,
		"ip":           agent.IP,
		"location": map[string]string{
			"region": agent.Region,
			"zone":   agent.Zone,
		},
		"capabilities": capabilities,
		"status":       agent.Status,
		"lastSeen":     agent.LastSeen.Unix(),
	}
}

// writeJSON 写入 JSON 响应
func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// handleError 处理错误
func (s *Server) handleError(w http.ResponseWriter, r *http.Request, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
	})
}
