package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/auth"
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
	case path == "/users":
		s.handleUsers(w, r)
	case strings.HasPrefix(path, "/users/"):
		s.handleUserActions(w, r, strings.TrimPrefix(path, "/users/"))
	case path == "/alerts" || path == "/alerts/read-all":
		s.handleAlertsList(w, r)
	case strings.HasPrefix(path, "/alerts/"):
		s.handleAlertActions(w, r, strings.TrimPrefix(path, "/alerts/"))
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

// handleUsers 处理用户列表和创建用户
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleUsersList(w, r)
	case http.MethodPost:
		s.handleUserCreate(w, r)
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleUsersList 获取用户列表
func (s *Server) handleUsersList(w http.ResponseWriter, r *http.Request) {
	users, err := s.db.ListUsers()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list users")
		return
	}

	s.writeJSON(w, http.StatusOK, users)
}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// handleUserCreate 创建用户
func (s *Server) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	user, ok := auth.GetUserFromContext(r)
	if !ok {
		s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// 只有管理员可以创建用户
	if user.Role != "admin" {
		s.handleError(w, r, http.StatusForbidden, "Only admin can create users")
		return
	}

	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// 验证输入
	if req.Username == "" || req.Password == "" {
		s.handleError(w, r, http.StatusBadRequest, "Username and password are required")
		return
	}

	// 设置默认角色
	if req.Role == "" {
		req.Role = "user"
	}

	// 检查用户名是否已存在
	_, err := s.db.GetUserByUsername(req.Username)
	if err == nil {
		s.handleError(w, r, http.StatusConflict, "Username already exists")
		return
	}

	// 哈希密码
	hashedPassword, err := storage.HashPassword(req.Password)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// 创建用户
	newUser := &storage.User{
		Username: req.Username,
		Password: hashedPassword,
		Email:    req.Email,
		Role:     req.Role,
	}

	if err := s.db.CreateUser(newUser); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to create user")
		return
	}

	// 清除密码后返回
	newUser.Password = ""
	s.writeJSON(w, http.StatusCreated, newUser)
}

// handleUserActions 处理用户操作
func (s *Server) handleUserActions(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(path, "/")

	if len(parts) == 0 {
		s.handleError(w, r, http.StatusNotFound, "User not specified")
		return
	}

	userID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "password" && r.Method == http.MethodPost:
		s.handleUserChangePassword(w, r, userID)
	case r.Method == http.MethodPut:
		s.handleUserUpdate(w, r, userID)
	case r.Method == http.MethodDelete:
		s.handleUserDelete(w, r, userID)
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleUserDelete 删除用户
func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request, id string) {
	// 获取当前用户
	user, ok := auth.GetUserFromContext(r)
	if !ok {
		s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// 查询目标用户
	targetUser, err := s.db.GetUserByID(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "User not found")
		return
	}

	// 不能删除自己
	if targetUser.Username == user.Username {
		s.handleError(w, r, http.StatusBadRequest, "Cannot delete yourself")
		return
	}

	// 只有管理员可以删除用户
	if user.Username != "admin" && user.Username != targetUser.Username {
		s.handleError(w, r, http.StatusForbidden, "Forbidden")
		return
	}

	if err := s.db.DeleteUser(id); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{"message": "User deleted"})
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// handleUserUpdate 更新用户
func (s *Server) handleUserUpdate(w http.ResponseWriter, r *http.Request, id string) {
	// 获取当前用户
	user, ok := auth.GetUserFromContext(r)
	if !ok {
		s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// 查询目标用户
	targetUser, err := s.db.GetUserByID(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "User not found")
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// 只有管理员可以修改角色
	if req.Role != "" && req.Role != targetUser.Role && user.Role != "admin" {
		s.handleError(w, r, http.StatusForbidden, "Only admin can change role")
		return
	}

	// 用户只能修改自己的邮箱，管理员可以修改任何人
	if req.Email != "" && user.Username != targetUser.Username && user.Role != "admin" {
		s.handleError(w, r, http.StatusForbidden, "Forbidden")
		return
	}

	// 更新用户
	targetUser.Email = req.Email
	if req.Role != "" && user.Role == "admin" {
		targetUser.Role = req.Role
	}

	if err := s.db.UpdateUser(targetUser); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to update user")
		return
	}

	targetUser.Password = ""
	s.writeJSON(w, http.StatusOK, targetUser)
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// handleUserChangePassword 修改用户密码
func (s *Server) handleUserChangePassword(w http.ResponseWriter, r *http.Request, id string) {
	// 获取当前用户
	user, ok := auth.GetUserFromContext(r)
	if !ok {
		s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// 查询目标用户
	targetUser, err := s.db.GetUserByID(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "User not found")
		return
	}

	// 只能修改自己的密码，管理员可以修改任何人的密码
	if user.Username != targetUser.Username && user.Role != "admin" {
		s.handleError(w, r, http.StatusForbidden, "Forbidden")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// 验证新密码
	if req.NewPassword == "" {
		s.handleError(w, r, http.StatusBadRequest, "New password is required")
		return
	}

	// 非管理员修改密码需要验证旧密码
	if user.Role != "admin" {
		// 验证旧密码
		_, err := s.db.VerifyPassword(targetUser.Username, req.OldPassword)
		if err != nil {
			s.handleError(w, r, http.StatusUnauthorized, "Invalid old password")
			return
		}
	}

	// 哈希新密码
	hashedPassword, err := storage.HashPassword(req.NewPassword)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// 更新密码
	if err := s.db.UpdatePassword(id, hashedPassword); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to update password")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Password updated"})
}

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

	parts := strings.Split(path, "/")

	if len(parts) == 0 {
		s.handleError(w, r, http.StatusNotFound, "Alert not specified")
		return
	}

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
