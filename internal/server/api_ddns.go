package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/dns"
	"github.com/cuihairu/cockpit/internal/storage"
)

// DDNS 配置 API（设计见 docs/guide/ddns-design.md D10）。
//
//	GET    /api/ddns               配置列表
//	POST   /api/ddns               新建配置（审计 ddns_create）
//	PUT    /api/ddns/{id}          更新配置（审计 ddns_update）
//	DELETE /api/ddns/{id}          删除配置（审计 ddns_delete）
//	POST   /api/ddns/{id}/check    立即检查单条（不等巡检周期）
//
// 配置创建只校验 Type 白名单与名称非空（此时尚无 IP，Content 校验
// 由 Cloudflare 在首次写入时兜底）；token 不出现在任何响应（D12）。

// handleDDNS 分发 /api/ddns[...]
func (s *Server) handleDDNS(w http.ResponseWriter, r *http.Request) {
	sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/ddns"), "/")
	switch {
	case sub == "":
		switch r.Method {
		case http.MethodGet:
			s.handleDDNSList(w, r)
		case http.MethodPost:
			s.handleDDNSCreate(w, r)
		default:
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	case strings.HasSuffix(sub, "/check"):
		idStr := strings.TrimSuffix(sub, "/check")
		s.handleDDNSCheck(w, r, idStr)
	default:
		id, err := strconv.Atoi(sub)
		if err != nil || id <= 0 {
			s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
			return
		}
		switch r.Method {
		case http.MethodPut:
			s.handleDDNSUpdate(w, r, uint(id))
		case http.MethodDelete:
			s.handleDDNSDelete(w, r, uint(id))
		default:
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// ddnsInput 创建/更新请求体
type ddnsInput struct {
	AgentID    string `json:"agentId"`
	ZoneID     string `json:"zoneId"`
	ZoneName   string `json:"zoneName"`
	RecordName string `json:"recordName"`
	Type       string `json:"type"`
	Enabled    bool   `json:"enabled"`
}

// validateDDNSInput 配置校验（D10）：Type 限地址记录、名称非空
func validateDDNSInput(in ddnsInput) error {
	t := strings.ToUpper(strings.TrimSpace(in.Type))
	if t != "A" && t != "AAAA" {
		return errDDNSTypeUnsupported
	}
	if strings.TrimSpace(in.AgentID) == "" {
		return errDDNSAgentRequired
	}
	if strings.TrimSpace(in.ZoneID) == "" {
		return errDDNSZoneRequired
	}
	if strings.TrimSpace(in.RecordName) == "" {
		return errDDNSNameRequired
	}
	return nil
}

var (
	errDDNSTypeUnsupported = errString("type must be A or AAAA")
	errDDNSAgentRequired   = errString("agentId is required")
	errDDNSZoneRequired    = errString("zoneId is required")
	errDDNSNameRequired    = errString("recordName is required")
)

// errString 轻量 error（本文件内校验用）
type errString string

func (e errString) Error() string { return string(e) }

func (s *Server) handleDDNSList(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListDDNSConfigs()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to list DDNS configs")
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleDDNSCreate(w http.ResponseWriter, r *http.Request) {
	var in ddnsInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := validateDDNSInput(in); err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	cfg := &storage.DDNSConfig{
		AgentID:    strings.TrimSpace(in.AgentID),
		ZoneID:     strings.TrimSpace(in.ZoneID),
		ZoneName:   strings.TrimSpace(in.ZoneName),
		RecordName: strings.TrimSpace(in.RecordName),
		Type:       strings.ToUpper(strings.TrimSpace(in.Type)),
		Enabled:    in.Enabled,
		LastStatus: "never",
	}
	if err := s.db.CreateDDNSConfig(cfg); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to create DDNS config")
		return
	}
	s.auditDNS(r, "ddns_create", cfg.ZoneID, cfg.RecordName, in)
	s.writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleDDNSUpdate(w http.ResponseWriter, r *http.Request, id uint) {
	cfg, err := s.db.GetDDNSConfig(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "DDNS config not found")
		return
	}
	var in ddnsInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := validateDDNSInput(in); err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	cfg.AgentID = strings.TrimSpace(in.AgentID)
	cfg.ZoneID = strings.TrimSpace(in.ZoneID)
	cfg.ZoneName = strings.TrimSpace(in.ZoneName)
	cfg.RecordName = strings.TrimSpace(in.RecordName)
	cfg.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	cfg.Enabled = in.Enabled
	if err := s.db.UpdateDDNSConfig(cfg); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to update DDNS config")
		return
	}
	s.auditDNS(r, "ddns_update", cfg.ZoneID, cfg.RecordName, in)
	s.writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleDDNSDelete(w http.ResponseWriter, r *http.Request, id uint) {
	cfg, err := s.db.GetDDNSConfig(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "DDNS config not found")
		return
	}
	if err := s.db.DeleteDDNSConfig(id); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to delete DDNS config")
		return
	}
	s.auditDNS(r, "ddns_delete", cfg.ZoneID, cfg.RecordName, map[string]string{"type": cfg.Type})
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleDDNSCheck 立即检查单条：同步执行一轮比对并返回结果
// （巡检缓存不共享，单次 ListRecords 自行拉取）
func (s *Server) handleDDNSCheck(w http.ResponseWriter, r *http.Request, idStr string) {
	if r.Method != http.MethodPost {
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	cfg, err := s.db.GetDDNSConfig(uint(id))
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "DDNS config not found")
		return
	}
	if s.dns == nil {
		s.handleError(w, r, http.StatusServiceUnavailable,
			"DNS provider not configured: set dns.cloudflare.api_token in config.yaml or CLOUDFLARE_API_TOKEN env")
		return
	}
	var notifCfg *config.NotificationConfig
	if s.cfg != nil {
		notifCfg = s.cfg.Notification
	}
	generator := alert.NewGenerator(s.db, s.notifier, notifCfg)
	ip, changed := s.runDDNSCheck(cfg, s.dns, map[string][]dns.Record{}, generator)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"ip":      ip,
		"changed": changed,
		"status":  cfg.LastStatus,
		"error":   cfg.LastError,
	})
}
