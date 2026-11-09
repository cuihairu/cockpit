package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

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

	// 批量附标签与系统快照（系统/架构列），避免 N+1 查询
	agentIDs := make([]string, 0, len(agents))
	for _, a := range agents {
		agentIDs = append(agentIDs, a.ID)
	}
	tagsByAgent, err := s.db.TagsForAgents(agentIDs)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to load tags")
		return
	}
	snapshots, err := s.db.ListSystemInfoSnapshotsByAgent(agentIDs)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to load system info")
		return
	}

	result := make([]map[string]interface{}, 0, len(agents))
	for _, agent := range agents {
		item := storageAgentToResponse(agent)
		tags := tagsByAgent[agent.ID]
		tagList := make([]map[string]string, 0, len(tags))
		for _, t := range tags {
			tagList = append(tagList, map[string]string{"id": t.ID, "name": t.Name, "color": t.Color})
		}
		item["tags"] = tagList
		if snap, ok := snapshots[agent.ID]; ok {
			item["osName"] = snap.OSName
			item["osVersion"] = snap.OSVersion
			item["arch"] = snap.Arch
		}
		result = append(result, item)
	}

	s.writeJSON(w, http.StatusOK, result)
}

// handleAgentGet 获取单个 Agent 详情
func (s *Server) handleAgentGet(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		agent, err := s.db.GetAgent(id)
		if err != nil {
			s.handleError(w, r, http.StatusNotFound, "Agent not found")
			return
		}
		s.writeJSON(w, http.StatusOK, storageAgentToResponse(agent))

	case http.MethodDelete:
		// 先从内存 registry 断开活跃连接
		if a, ok := s.registry.Get(id); ok {
			a.Close()
			s.registry.Unregister(id)
		}
		if err := s.db.DeleteAgent(id); err != nil {
			s.handleError(w, r, http.StatusInternalServerError, "Failed to delete agent")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})

	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleAgentsCleanup 清理离线 Agent（RBAC：inventory:write）
// POST /api/agents/cleanup — body 可带 thresholdMinutes（分钟数，按 last_seen
// 截断，覆盖 status 残留 online 的死行）；缺省/空体/0 = 清理所有 offline
// （默认全清）。阈值由前端自由填（N 分钟/小时/天），服务端只认分钟数。
// 先将 DB 中 "online" 但不在内存 registry 的 agent 标记为 offline，再统一清理。
func (s *Server) handleAgentsCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// 可选 body：{"thresholdMinutes": N}；空体/缺字段 = 0（全部 offline）。
	// 负数无意义，直接 400 防误清。
	var body struct {
		ThresholdMinutes int `json:"thresholdMinutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.ThresholdMinutes < 0 {
		s.handleError(w, r, http.StatusBadRequest, "thresholdMinutes must be >= 0")
		return
	}

	// 1. 将 DB 中标记 online 但实际已断开的 agent 标记为 offline
	dbAgents, err := s.db.ListAgents()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list agents")
		return
	}
	for _, a := range dbAgents {
		if a.Status == "online" {
			if _, active := s.registry.Get(a.ID); !active {
				_ = s.db.UpdateAgentStatus(a.ID, "offline", a.LastSeen)
			}
		}
	}

	// 2. 按阈值清理（0 = 全部 offline）
	removed, err := s.db.CleanupOfflineAgents(time.Duration(body.ThresholdMinutes) * time.Minute)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to cleanup agents")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"removed": removed,
		"count":   len(removed),
	})
}

// handleAgentSecret 管理 Agent 密钥（RBAC：inventory:write）
func (s *Server) handleAgentSecret(w http.ResponseWriter, r *http.Request, id string) {
	agent, err := s.db.GetAgent(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "Agent not found")
		return
	}

	switch r.Method {
	case http.MethodPost:
		// 重新生成密钥
		newSecret, err := s.db.RegenerateAgentSecret(id)
		if err != nil {
			s.handleError(w, r, http.StatusInternalServerError, "Failed to regenerate secret")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"agentId": id,
			"secret":  newSecret,
			"message": "Agent secret regenerated. Save this secret, it won't be shown again.",
		})
	case http.MethodGet:
		// 检查密钥状态
		hasSecret := agent.SecretHash != ""
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"agentId":    id,
			"hasSecret":  hasSecret,
			"configured": hasSecret,
		})
	case http.MethodDelete:
		// 删除密钥（将允许无认证连接，不推荐）
		if err := s.db.UpdateAgentSecret(id, ""); err != nil {
			s.handleError(w, r, http.StatusInternalServerError, "Failed to remove secret")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Agent secret removed (authentication disabled)",
		})
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// storageAgentToResponse 将存储 Agent 转换为 API 响应格式
func storageAgentToResponse(agent *storage.Agent) map[string]interface{} {
	// capabilities 保持对象数组（type/version/endpoint/metadata）——web/mobile 的
	// Capability.fromJson 与 workbench getRemoteServices 都读 cap.type/cap.metadata，
	// 此前扁平化成 []string 导致全部前端能力判断失效（真机验收发现，2026-09-30）。
	capabilities := agent.Capabilities
	if capabilities == nil {
		capabilities = []storage.Capability{}
	}

	return map[string]interface{}{
		"id":           agent.ID,
		"hostname":     agent.Hostname,
		"ip":           agent.IP,
		"localIps":     agent.LocalIPs,
		"region":       agent.Region,
		"zone":         agent.Zone,
		"capabilities": capabilities,
		"status":       agent.Status,
		"lastSeen":     agent.LastSeen.Unix(),
		"labels":       agent.Labels,
		"metadata":     agent.Metadata,
		"version":      agent.Version,
		"startedAt":    agent.StartedAt.Unix(),
		"virtType":     agent.VirtType,
		"virtRole":     agent.VirtRole,
		"services":     agent.Services,
	}
}
