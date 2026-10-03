package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/storage"
)

// 服务器标签 API（2026-10-02）：
//   GET    /api/agent-tags            全部标签（含每标签服务器数）
//   POST   /api/agent-tags            新建 {name, color}
//   PUT    /api/agent-tags/{id}       重命名/改色 {name?, color?}
//   DELETE /api/agent-tags/{id}       删除（只摘关联，不动服务器）
//   GET    /api/agents/{id}/tags      某台服务器的标签
//   PUT    /api/agents/{id}/tags      覆盖设置 {tagIds: []}

// handleAgentTagsAPI 分发 /api/agent-tags[/{id}]
func (s *Server) handleAgentTagsAPI(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/agent-tags")
	rest = strings.TrimSuffix(rest, "/")

	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			s.listAgentTags(w, r)
		case http.MethodPost:
			s.createAgentTag(w, r)
		default:
			s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	id := strings.TrimPrefix(rest, "/")
	if strings.Contains(id, "/") {
		s.handleError(w, r, http.StatusNotFound, "Not found")
		return
	}
	switch r.Method {
	case http.MethodPut:
		s.updateAgentTag(w, r, id)
	case http.MethodDelete:
		s.deleteAgentTag(w, r, id)
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleAgentTagAssignAPI 分发 /api/agents/{id}/tags
func (s *Server) handleAgentTagAssignAPI(w http.ResponseWriter, r *http.Request, agentID string) {
	id := strings.TrimSuffix(agentID, "/tags")
	switch r.Method {
	case http.MethodGet:
		s.getAgentTags(w, r, id)
	case http.MethodPut:
		s.setAgentTags(w, r, id)
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) listAgentTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.db.ListTags()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list tags")
		return
	}
	counts, err := s.db.TagCounts()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to count tags")
		return
	}
	result := make([]map[string]interface{}, 0, len(tags))
	for _, t := range tags {
		result = append(result, map[string]interface{}{
			"id": t.ID, "name": t.Name, "color": t.Color,
			"agentCount": counts[t.ID], "createdAt": t.CreatedAt.Unix(),
		})
	}
	s.writeJSON(w, http.StatusOK, result)
}

func (s *Server) createAgentTag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		s.handleError(w, r, http.StatusBadRequest, "name is required")
		return
	}
	tag, err := s.db.CreateTag(body.Name, body.Color)
	if err == storage.ErrTagNameTaken {
		s.handleError(w, r, http.StatusConflict, "Tag name already exists")
		return
	}
	if err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, http.StatusCreated, map[string]interface{}{"id": tag.ID, "name": tag.Name, "color": tag.Color})
}

func (s *Server) updateAgentTag(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := s.db.UpdateTag(id, body.Name, body.Color); err == storage.ErrTagNotFound {
		s.handleError(w, r, http.StatusNotFound, "Tag not found")
		return
	} else if err == storage.ErrTagNameTaken {
		s.handleError(w, r, http.StatusConflict, "Tag name already exists")
		return
	} else if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to update tag")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "id": id})
}

func (s *Server) deleteAgentTag(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.db.DeleteTag(id); err == storage.ErrTagNotFound {
		s.handleError(w, r, http.StatusNotFound, "Tag not found")
		return
	} else if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to delete tag")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

func (s *Server) getAgentTags(w http.ResponseWriter, r *http.Request, agentID string) {
	tags, err := s.db.AgentTags(agentID)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list agent tags")
		return
	}
	result := make([]map[string]interface{}, 0, len(tags))
	for _, t := range tags {
		result = append(result, map[string]interface{}{"id": t.ID, "name": t.Name, "color": t.Color})
	}
	s.writeJSON(w, http.StatusOK, result)
}

func (s *Server) setAgentTags(w http.ResponseWriter, r *http.Request, agentID string) {
	var body struct {
		TagIDs []string `json:"tagIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := s.db.SetAgentTags(agentID, body.TagIDs); err == storage.ErrTagNotFound {
		s.handleError(w, r, http.StatusNotFound, "Agent or tag not found")
		return
	} else if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to set agent tags")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "agentId": agentID})
}
