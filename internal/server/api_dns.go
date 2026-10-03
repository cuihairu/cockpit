package server

import (
	"context"
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
//	GET    /api/dns/zones                        zone 列表（in_cmdb 标注 + 反向对账 orphans）
//	GET    /api/dns/zones/{zid}/records?type=&page=   记录分页
//	POST   /api/dns/zones/{zid}/records          创建记录（审计 dns_create + 台账联动）
//	PUT    /api/dns/zones/{zid}/records/{rid}    更新记录（审计 dns_update + 台账联动）
//	DELETE /api/dns/zones/{zid}/records/{rid}    删除记录（审计 dns_delete + 台账联动）
//	GET    /api/dns/zones/{zid}/records/export   批量导出（M4，审计 dns_export）
//	POST   /api/dns/zones/{zid}/records/import   批量导入（M4，审计 dns_import + 台账联动）
//	POST   /api/dns/zones/{zid}/cmdb             zone 登记 Domain 台账（审计 dns_cmdb_register）
//	DELETE /api/dns/zones/{zid}/cmdb             台账移除（仅 DNS 来源行；审计 dns_cmdb_unregister）

// dnsZoneOut zone 列表响应行（in_cmdb = 该域名是否在 Domain 表，只读联动）
type dnsZoneOut struct {
	dns.Zone
	InCMDB bool `json:"in_cmdb"`
}

// handleDNS 分发 /api/dns 及子路径
func (s *Server) handleDNS(w http.ResponseWriter, r *http.Request) {
	// 经 serveAPI 进入时 r.URL.Path 带 /api 前缀，须先剥再比对（否则全部
	// 落 404 兜底，Web DNS 页不可用；与 handleRecordings 同类缺陷，
	// 真机验收发现 2026-09-30）
	sub := strings.Trim(strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api"), "/dns"), "/")
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
	// {data} 包装对齐 web getDNSZones 的 resp.data 解构（P0 响应结构一致性）；
	// M3 反向对账：orphans = 台账中 DNS 来源但已不在 zone 列表的行（D20）
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":    out,
		"orphans": s.listDNSOrphans(zones),
	})
}

// handleDNSZoneRecords /api/dns/zones/{zid}/records[/{rid}] 与
// /api/dns/zones/{zid}/cmdb（M3 写联动，api_dns_cmdb.go）
func (s *Server) handleDNSZoneRecords(w http.ResponseWriter, r *http.Request, sub string) {
	// sub 形态：{zid}/records[/{rid}] 或 {zid}/cmdb
	parts := strings.Split(strings.Trim(sub, "/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	zoneID := parts[0]
	if parts[1] == "cmdb" {
		if len(parts) != 2 {
			s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
			return
		}
		s.handleDNSZoneCMDB(w, r, zoneID)
		return
	}
	if parts[1] != "records" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
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
	// M4 批量导入/导出：export/import 是 records 下的保留字（三家 provider
	// 的记录 ID 均为十六进制/数字串，无撞名面；方法不符 405，D21）
	switch parts[2] {
	case "export":
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleDNSRecordsExport(w, r, zoneID)
	case "import":
		if r.Method != http.MethodPost {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleDNSRecordsImport(w, r, zoneID)
	default:
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
	// M3 写联动：解析记录创建成功后跟随登记台账（失败只记日志不回滚，D20）
	s.linkDNSRecordDomain("create", zoneID, rec)
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
	// M3 写联动：更新后台账行跟随最新 name/type（确定性 ID 原地 upsert，D19）
	s.linkDNSRecordDomain("update", zoneID, rec)
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
	// M3 写联动：记录删除后台账行随之移除（幂等；按类型过滤在联动函数内）
	s.unlinkDNSRecordDomain(zoneID, recordID)
	s.auditDNS(r, "dns_delete", zoneID, recordID, nil)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

// dnsImportMaxRecords 单次导入上限（防误传超大文件拖垮上游与配额，D24）
const dnsImportMaxRecords = 500

// fetchAllDNSRecords 翻全量记录（M4：export 聚合与 import 现状比对共用；
// TotalPage 已由 client 归一，空 zone 的 TotalPage 0/1 都在首页后终止）
func fetchAllDNSRecords(ctx context.Context, provider dns.Provider, zoneID string) ([]dns.Record, error) {
	var all []dns.Record
	for page := 1; ; page++ {
		result, err := provider.ListRecords(ctx, zoneID, "", page)
		if err != nil {
			return nil, err
		}
		all = append(all, result.Records...)
		if page >= result.TotalPage {
			break
		}
	}
	return all, nil
}

// handleDNSRecordsExport GET records/export（M4 D22）：翻全量聚合为便携
// RecordInput 形态（无 provider id/locked），导出文件可直接作导入输入；
// GET 不经审计中间件，取内容审计先例（recordings cast）手动记 dns_export
func (s *Server) handleDNSRecordsExport(w http.ResponseWriter, r *http.Request, zoneID string) {
	provider := s.requireDNS(w, r)
	if provider == nil {
		return
	}
	records, err := fetchAllDNSRecords(r.Context(), provider, zoneID)
	if err != nil {
		s.handleDNSUpstreamError(w, r, err)
		return
	}
	out := make([]dns.RecordInput, 0, len(records))
	for _, rec := range records {
		out = append(out, dns.RecordInput{
			Type: rec.Type, Name: rec.Name, Content: rec.Content,
			TTL: rec.TTL, Proxied: rec.Proxied,
		})
	}
	s.auditDNS(r, "dns_export", zoneID, "export", map[string]int{"count": len(out)})
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"zone_id":  zoneID,
		"provider": s.dnsProviderName(),
		"count":    len(out),
		"records":  out,
	})
}

// dnsRecordMatchKey 导入匹配键（D23）：name 归一（去空白/尾点/小写）+
// type 大写 + content 去空白精确
func dnsRecordMatchKey(name, recordType, content string) string {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	return strings.ToUpper(strings.TrimSpace(recordType)) + "|" + name + "|" + strings.TrimSpace(content)
}

// dnsImportFailed 单条上游失败明细（D24）
type dnsImportFailed struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

// handleDNSRecordsImport POST records/import（M4 D23/D24）：body
// {records:[RecordInput...]}；先整体校验（任一非法 400）再逐条
// 匹配-创建/更新/跳过，单条上游失败不中断；created/updated 走 M3 台账联动
func (s *Server) handleDNSRecordsImport(w http.ResponseWriter, r *http.Request, zoneID string) {
	provider := s.requireDNS(w, r)
	if provider == nil {
		return
	}
	var req struct {
		Records []dns.RecordInput `json:"records"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Records) == 0 {
		s.handleError(w, r, http.StatusBadRequest, "records must not be empty")
		return
	}
	if len(req.Records) > dnsImportMaxRecords {
		s.handleError(w, r, http.StatusBadRequest,
			fmt.Sprintf("too many records: %d exceeds limit %d", len(req.Records), dnsImportMaxRecords))
		return
	}
	// 先整体校验再落上游：带下标报错，最多列 5 条（D24）
	var invalid []string
	for i, in := range req.Records {
		if err := dns.ValidateInput(in); err != nil && len(invalid) < 5 {
			invalid = append(invalid, fmt.Sprintf("records[%d]: %v", i, err))
		}
	}
	if len(invalid) > 0 {
		s.handleError(w, r, http.StatusBadRequest,
			fmt.Sprintf("invalid records: %s", strings.Join(invalid, "; ")))
		return
	}
	existing, err := fetchAllDNSRecords(r.Context(), provider, zoneID)
	if err != nil {
		s.handleDNSUpstreamError(w, r, err)
		return
	}
	byKey := make(map[string]*dns.Record, len(existing))
	for i := range existing {
		byKey[dnsRecordMatchKey(existing[i].Name, existing[i].Type, existing[i].Content)] = &existing[i]
	}
	isCF := s.dnsProviderName() == "cloudflare"
	created, updated, skipped := 0, 0, 0
	var failed []dnsImportFailed
	for i, in := range req.Records {
		key := dnsRecordMatchKey(in.Name, in.Type, in.Content)
		cur, ok := byKey[key]
		// 命中后的写判定（D23）：显式 TTL（>=60）须精确相等，auto（0/1）
		// 表示「provider 决定」不触发更新；proxied 仅 cloudflare 比较
		needWrite := !ok
		if ok {
			if in.TTL >= 60 && cur.TTL != in.TTL {
				needWrite = true
			}
			if isCF && in.Proxied != cur.Proxied {
				needWrite = true
			}
		}
		if !needWrite {
			skipped++
			continue
		}
		if ok {
			rec, err := provider.UpdateRecord(r.Context(), zoneID, cur.ID, in)
			if err != nil {
				failed = append(failed, dnsImportFailed{Index: i, Name: in.Name, Error: err.Error()})
				continue
			}
			updated++
			byKey[key] = rec
			// M3 写联动：更新后台账行跟随（失败只记日志不回滚，D20）
			s.linkDNSRecordDomain("update", zoneID, rec)
		} else {
			rec, err := provider.CreateRecord(r.Context(), zoneID, in)
			if err != nil {
				failed = append(failed, dnsImportFailed{Index: i, Name: in.Name, Error: err.Error()})
				continue
			}
			created++
			// 同请求内重复条目写回索引后即 skip（幂等重导，D23）
			byKey[key] = rec
			s.linkDNSRecordDomain("create", zoneID, rec)
		}
	}
	// 审计 details 只含计数不含记录内容（token/内容不进审计的既有纪律）
	s.auditDNS(r, "dns_import", zoneID, "batch", map[string]int{
		"total": len(req.Records), "created": created, "updated": updated,
		"skipped": skipped, "failed": len(failed),
	})
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"total": len(req.Records), "created": created, "updated": updated,
		"skipped": skipped, "failed": failed,
	})
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
