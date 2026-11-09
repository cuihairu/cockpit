package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

// serveAPI 处理 API 请求
func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	// 设置 CORS
	setCORSHeaders(w)

	// 处理 OPTIONS 预检请求
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 解析路径（归一化尾斜杠，/api/agents/ → /api/agents）
	path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api"), "/")
	if path == "" {
		path = "/"
	}

	// 路由分发
	switch {
	case path == "/status":
		s.handleStatus(w, r)
	case path == "/me":
		s.handleCurrentUser(w, r)
	case path == "/me/profile":
		s.handleCurrentUserProfile(w, r)
	case path == "/me/password":
		s.handleCurrentUserPassword(w, r)
	case path == "/settings":
		s.handleSettings(w, r)
	case path == "/agents":
		s.handleAgentsList(w, r)
	case path == "/agents/cleanup":
		s.handleAgentsCleanup(w, r)
	case path == "/drift/config":
		s.handleDriftConfig(w, r)
	case path == "/jobs" || strings.HasPrefix(path, "/jobs/"):
		// 执行 Job（见 api_jobs.go，jobs-design.md）
		s.handleJobsAPI(w, r, path)
	case path == "/workflows" || strings.HasPrefix(path, "/workflows/") ||
		path == "/workflow-runs" || strings.HasPrefix(path, "/workflow-runs/"):
		// Workflow 编排（见 api_workflow.go，workflow-design.md）
		s.handleWorkflowAPI(w, r, path)
	case path == "/approvals" || strings.HasPrefix(path, "/approvals/"):
		// 审批门（见 api_approvals.go，workflow-design.md M2d）
		s.handleApprovalsAPI(w, r, path)
	case path == "/inventory/consistency":
		// CMDB 一致性（见 api_inventory.go，drift-design.md M6 D28/D30）
		s.handleInventoryConsistency(w, r)
	case path == "/smart/config":
		s.handleSmartConfig(w, r)
	case path == "/nas/config":
		s.handleNASConfig(w, r)
	case path == "/agent-tags" || strings.HasPrefix(path, "/agent-tags/"):
		s.handleAgentTagsAPI(w, r)
	case path == "/recordings":
		s.handleRecordings(w, r)
	case strings.HasPrefix(path, "/recordings/"):
		s.handleRecordings(w, r)
	case path == "/server-backups" || strings.HasPrefix(path, "/server-backups/"):
		s.handleServerBackups(w, r)
	case path == "/dns" || strings.HasPrefix(path, "/dns/"):
		s.handleDNS(w, r)
	case path == "/domains" || strings.HasPrefix(path, "/domains/"):
		// 服务域名绑定（见 api_domain_bindings.go，domain-binding-design.md）
		s.handleDomainBindings(w, r)
	case path == "/overlay/cloud" || strings.HasPrefix(path, "/overlay/cloud/"):
		// 组网云管理面（见 api_overlay_cloud.go，M2 D11-D19）
		s.handleOverlayCloud(w, r)
	case path == "/ddns" || strings.HasPrefix(path, "/ddns/"):
		s.handleDDNS(w, r)
	case path == "/acme" || strings.HasPrefix(path, "/acme/"):
		s.handleACME(w, r)
	case path == "/logs/search":
		// 跨机日志联邦检索（全局端点，见 api_logs_search.go M3/D11）
		s.handleLogsSearch(w, r)
	case strings.HasPrefix(path, "/agents/"):
		agentID := strings.TrimPrefix(path, "/agents/")
		// agent 标签 GET/PUT /agents/{id}/tags（见 api_tags.go）
		if strings.HasSuffix(agentID, "/tags") {
			s.handleAgentTagAssignAPI(w, r, agentID)
			return
		}
		// 远程文件管理 /agents/{id}/files/...（见 api_files.go）
		if strings.Contains(agentID, "/files/") {
			s.handleAgentFilesAPI(w, r, agentID)
			return
		}
		// 反向代理管理 /agents/{id}/proxy/...（见 api_proxy_sites.go）
		if strings.Contains(agentID, "/proxy/") {
			s.handleAgentProxyAPI(w, r, agentID)
			return
		}
		// 定时任务管理 /agents/{id}/cron/...（见 api_cron.go）
		if strings.Contains(agentID, "/cron/") {
			s.handleAgentCronAPI(w, r, agentID)
			return
		}
		// systemd 服务管理 /agents/{id}/services/...（见 api_service.go；
		// 裸 GET /agents/{id}/services 是服务列表端点，HasSuffix 兜底——
		// 真机验收逮到：仅 Contains "/services/" 时裸列表掉进 handleAgentGet
		// 报 404 Agent not found，Web 服务页列表整体不可达）
		if strings.HasSuffix(agentID, "/services") || strings.Contains(agentID, "/services/") {
			s.handleAgentServiceAPI(w, r, agentID)
			return
		}
		// 服务健康探针 /agents/{id}/health[/probes/{id}/check]（见 api_service_health.go）
		if strings.HasSuffix(agentID, "/health") || strings.Contains(agentID, "/health/") {
			s.handleAgentHealthAPI(w, r, agentID)
			return
		}
		// NAS 存储观测 /agents/{id}/nas/...（见 api_nas.go）
		if strings.Contains(agentID, "/nas/") {
			s.handleAgentNASAPI(w, r, agentID)
			return
		}
		// 远程日志查询 /agents/{id}/logs/...（见 api_logs.go）
		if strings.Contains(agentID, "/logs/") {
			s.handleAgentLogsAPI(w, r, agentID)
			return
		}
		// 防漂移检测 /agents/{id}/drift/...（见 api_drift.go）
		if strings.Contains(agentID, "/drift/") {
			s.handleAgentDriftAPI(w, r, agentID)
			return
		}
		// agent 域名清单 /agents/{id}/domains[/snippet]（见 api_domain_bindings.go，D7）
		if strings.HasSuffix(agentID, "/domains") || strings.Contains(agentID, "/domains/") {
			s.handleAgentDomainsAPI(w, r, agentID)
			return
		}
		// 组网工具观测 /agents/{id}/overlay/...（见 api_overlay.go）
		if strings.Contains(agentID, "/overlay/") {
			s.handleAgentOverlayAPI(w, r, agentID)
			return
		}
		// SMART 磁盘健康 /agents/{id}/smart/...（见 api_smart.go）
		if strings.Contains(agentID, "/smart/") {
			s.handleAgentSmartAPI(w, r, agentID)
			return
		}
		// 本地虚拟化只读面 /agents/{id}/virt/...（见 api_virt.go）
		if strings.Contains(agentID, "/virt/") {
			s.handleAgentVirtAPI(w, r, agentID)
			return
		}
		// 防火墙规则集快照 /agents/{id}/firewall/...（见 api_firewall.go）
		if strings.Contains(agentID, "/firewall/") {
			s.handleAgentFirewallAPI(w, r, agentID)
			return
		}
		// Handle /agents/{id}/secret sub-path
		if strings.HasSuffix(agentID, "/secret") {
			s.handleAgentSecret(w, r, strings.TrimSuffix(agentID, "/secret"))
			return
		}
		s.handleAgentGet(w, r, agentID)
	case strings.HasPrefix(path, "/resources/"):
		s.handleResources(w, r, strings.TrimPrefix(path, "/resources/"))
	case path == "/users":
		s.handleUsers(w, r)
	case strings.HasPrefix(path, "/users/"):
		s.handleUserActions(w, r, strings.TrimPrefix(path, "/users/"))
	case path == "/roles":
		// 角色 CRUD（见 api_roles.go，rbac-design.md 笔 4）
		s.handleRoles(w, r)
	case strings.HasPrefix(path, "/roles/"):
		s.handleRoleActions(w, r, strings.TrimPrefix(path, "/roles/"))
	case path == "/alerts" || path == "/alerts/read-all":
		s.handleAlertsList(w, r)
	case strings.HasPrefix(path, "/alerts/"):
		s.handleAlertActions(w, r, strings.TrimPrefix(path, "/alerts/"))
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
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

// writeList 输出标准分页列表响应（page=1, single page）。
// 用于资源列表这种一次性返回全量数据的端点，避免 6 处重复样板（DRY）。
func (s *Server) writeList(w http.ResponseWriter, items interface{}, count int) {
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       items,
		"total":      count,
		"page":       1,
		"pageSize":   count,
		"totalPages": 1,
	})
}
