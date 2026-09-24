package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/dns"
)

// DNS 管理 API（见 docs/guide/dns-design.md）。server 直连 Cloudflare
// API v4，Agent 不参与；zone 与 record 都是 Cloudflare 侧事实源，不落库。
//
//	GET    /api/dns/status                       配置探测（只返回布尔，不含 token）
//	GET    /api/dns/zones                        zone 列表（含 in_cmdb 标注）
//	GET    /api/dns/zones/{zid}/records?type=&page=   记录分页
//	POST   /api/dns/zones/{zid}/records          创建记录（审计 dns_create）
//	PUT    /api/dns/zones/{zid}/records/{rid}    更新记录（审计 dns_update）
//	DELETE /api/dns/zones/{zid}/records/{rid}    删除记录（审计 dns_delete）

// dnsZoneOut zone 列表响应行（in_cmdb = 该域名是否在 Domain 表，只读联动）
type dnsZoneOut struct {
	dns.Zone
	InCMDB bool `json:"in_cmdb"`
}

// handleDNS 分发 /api/dns 及子路径
func (s *Server) handleDNS(w http.ResponseWriter, r *http.Request) {
	sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/dns"), "/")
	switch {
	case sub == "status":
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"configured": s.dns != nil,
			"provider":   s.dnsProviderName(),
		})
	case sub == "zones":
		s.handleDNSZones(w, r)
	case strings.HasPrefix(sub, "zones/"):
		s.handleDNSZoneRecords(w, r, strings.TrimPrefix(sub, "zones/"))
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// dnsProviderName 当前 dns.provider 名（空 = cloudflare 向后兼容，D11）
func (s *Server) dnsProviderName() string {
	if s.cfg == nil || s.cfg.DNS == nil || s.cfg.DNS.Provider == "" {
		return "cloudflare"
	}
	return s.cfg.DNS.Provider
}

// requireDNS 未配置凭据时统一 503 + 引导（D2/D7/D16：按 provider 各报各的
// 键，文案不含凭据值）
func (s *Server) requireDNS(w http.ResponseWriter, r *http.Request) dns.Provider {
	if s.dns == nil {
		var msg string
		switch name := s.dnsProviderName(); name {
		case "dnspod":
			msg = "DNS provider not configured: set dns.dnspod.login_token in config.yaml or DNSPOD_LOGIN_TOKEN env"
		case "alidns":
			msg = "DNS provider not configured: set dns.alidns.access_key and secret_key in config.yaml or ALIYUN_ACCESS_KEY / ALIYUN_ACCESS_KEY_SECRET env"
		case "cloudflare":
			msg = "DNS provider not configured: set dns.cloudflare.api_token in config.yaml or CLOUDFLARE_API_TOKEN env"
		default:
			msg = fmt.Sprintf("unknown DNS provider %q: set dns.provider to cloudflare, dnspod or alidns", name)
		}
		s.handleError(w, r, http.StatusServiceUnavailable, msg)
		return nil
	}
	return s.dns
}

// handleDNSZones GET /api/dns/zones——拉 zone 列表并对照 Domain 表标注
func (s *Server) handleDNSZones(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	provider := s.requireDNS(w, r)
	if provider == nil {
		return
	}
	zones, err := provider.ListZones(r.Context())
	if err != nil {
		s.handleDNSUpstreamError(w, r, err)
		return
	}
	cmdb := map[string]bool{}
	if domains, err := s.db.ListDomains(); err == nil {
		for _, d := range domains {
			cmdb[strings.ToLower(strings.TrimSuffix(d.Domain, "."))] = true
		}
	}
	out := make([]dnsZoneOut, 0, len(zones))
	for _, z := range zones {
		out = append(out, dnsZoneOut{Zone: z, InCMDB: cmdb[strings.ToLower(z.Name)]})
	}
	// {data} 包装对齐 web getDNSZones 的 resp.data 解构（P0 响应结构一致性）
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": out})
}

// handleDNSZoneRecords /api/dns/zones/{zid}/records[/{rid}]
func (s *Server) handleDNSZoneRecords(w http.ResponseWriter, r *http.Request, sub string) {
	// sub 形态：{zid}/records 或 {zid}/records/{rid}
	parts := strings.Split(strings.Trim(sub, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] != "records" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	zoneID := parts[0]
	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			s.handleDNSRecordsList(w, r, zoneID)
		case http.MethodPost:
			s.handleDNSRecordCreate(w, r, zoneID)
		default:
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(parts) != 3 || parts[2] == "" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	recordID := parts[2]
	switch r.Method {
	case http.MethodPut:
		s.handleDNSRecordUpdate(w, r, zoneID, recordID)
	case http.MethodDelete:
		s.handleDNSRecordDelete(w, r, zoneID, recordID)
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleDNSRecordsList GET records?type=&page=
func (s *Server) handleDNSRecordsList(w http.ResponseWriter, r *http.Request, zoneID string) {
	provider := s.requireDNS(w, r)
	if provider == nil {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	result, err := provider.ListRecords(r.Context(), zoneID, r.URL.Query().Get("type"), page)
	if err != nil {
		s.handleDNSUpstreamError(w, r, err)
		return
	}
	// {data} 包装对齐 web getDNSRecords 的 resp.data 解构（P0 响应结构一致性）
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": result})
}

// handleDNSRecordCreate POST records
func (s *Server) handleDNSRecordCreate(w http.ResponseWriter, r *http.Request, zoneID string) {
	provider := s.requireDNS(w, r)
	if provider == nil {
		return
	}
	var input dns.RecordInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "invalid JSON body")
		return
	}
	rec, err := provider.CreateRecord(r.Context(), zoneID, input)
	if err != nil {
		s.handleDNSUpstreamError(w, r, err)
		return
	}
	s.auditDNS(r, "dns_create", zoneID, input.Name, input)
	s.writeJSON(w, http.StatusOK, rec)
}

// handleDNSRecordUpdate PUT records/{rid}
func (s *Server) handleDNSRecordUpdate(w http.ResponseWriter, r *http.Request, zoneID, recordID string) {
	provider := s.requireDNS(w, r)
	if provider == nil {
		return
	}
	var input dns.RecordInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "invalid JSON body")
		return
	}
	rec, err := provider.UpdateRecord(r.Context(), zoneID, recordID, input)
	if err != nil {
		s.handleDNSUpstreamError(w, r, err)
		return
	}
	s.auditDNS(r, "dns_update", zoneID, input.Name, input)
	s.writeJSON(w, http.StatusOK, rec)
}

// handleDNSRecordDelete DELETE records/{rid}
func (s *Server) handleDNSRecordDelete(w http.ResponseWriter, r *http.Request, zoneID, recordID string) {
	provider := s.requireDNS(w, r)
	if provider == nil {
		return
	}
	if err := provider.DeleteRecord(r.Context(), zoneID, recordID); err != nil {
		s.handleDNSUpstreamError(w, r, err)
		return
	}
	s.auditDNS(r, "dns_delete", zoneID, recordID, nil)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

// handleDNSUpstreamError 上游错误统一 502；入参校验类错误 400。
// 错误消息来自 dns 包（只含 Cloudflare 返回的 code/message，不含 token）
func (s *Server) handleDNSUpstreamError(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()
	if strings.Contains(msg, "unsupported record type") ||
		strings.Contains(msg, "name and content are required") ||
		strings.Contains(msg, "ttl must be") ||
		strings.Contains(msg, "content must have") { // MX/SRV 拆装格式错（dnspod/alidns client）
		s.handleError(w, r, http.StatusBadRequest, msg)
		return
	}
	s.handleError(w, r, http.StatusBadGateway, msg)
}

// auditDNS DNS 变更审计（resourceID = zoneID/记录名；details 不含 token）
func (s *Server) auditDNS(r *http.Request, action, zoneID, recordName string, details interface{}) {
	username := ""
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, action, audit.ResourceDNSRecord,
		fmt.Sprintf("%s/%s", zoneID, recordName),
		details, s.getClientIP(r), r.UserAgent())
}
