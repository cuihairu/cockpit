package server

// DNS 管理 × Domain 台账写联动（dns-design.md M3，D18-D20）。
// M1 只有只读联动（zones 列表的 in_cmdb 标注）；本文件把写路径接上：
//   - zone 级显式登记/移除：POST/DELETE /api/dns/zones/{zid}/cmdb
//   - 解析记录（A/AAAA/CNAME）CRUD 成功后自动跟随台账行
//
// 原则（D20）：provider 侧是事实源且必须先成功，台账联动尽力而为——
// 联动失败只记日志，不回滚 DNS 操作、不影响 API 应答；inventory 声明或
// 手工登记的同名行永不被覆盖（唯一索引 + 预检双保险）。

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/dns"
	"github.com/cuihairu/cockpit/internal/storage"
)

// dnsCMDBSourceLabel DNS 来源台账行的 labels.source 值（孤儿判定依据）
const dnsCMDBSourceLabel = "dns"

// dnsCmdbRecordTypes 参与台账联动的记录类型：能「解析出一个域名」的三类。
// 口径对齐既有约定（DDNS 只写 A/AAAA、web proxied 仅 A/AAAA/CNAME）；
// MX/TXT/NS/SRV/CAA 是附属数据不是域名资产，不联动（D18，假设已注明）。
var dnsCmdbRecordTypes = map[string]bool{"A": true, "AAAA": true, "CNAME": true}

// dnsZoneDomainID zone 级台账行确定性 ID；记录级追加 -{rid}。
// 确定性 ID（同 acme-{id} 先例）让记录删除/更新时无需反查即可定位台账行。
func dnsZoneDomainID(zoneID string) string {
	return "dns-" + zoneID
}

func dnsRecordDomainID(zoneID, recordID string) string {
	return "dns-" + zoneID + "-" + recordID
}

// dnsRecordFQDN 记录名 → 全名域名。三家 provider 的 ListRecords 已在
// client 层归一为全名（D14：@ ↔ zone 名互转），这里只做防御性兜底：
// 空名/@ 视为 zone 本身；不带 zone 后缀的裸子域补全；尾点剥掉。
func dnsRecordFQDN(name, zoneName string) string {
	zoneName = strings.TrimSuffix(zoneName, ".")
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	if name == "" || name == "@" || strings.EqualFold(name, zoneName) {
		return zoneName
	}
	if strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(zoneName)) {
		return name
	}
	return name + "." + zoneName
}

// dnsNormDomain 台账比对用归一（与 zones in_cmdb 标注同口径）
func dnsNormDomain(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
}

// handleDNSZoneCMDB POST/DELETE /api/dns/zones/{zid}/cmdb——zone 级显式
// 登记/移除 Domain 台账行（D18）
func (s *Server) handleDNSZoneCMDB(w http.ResponseWriter, r *http.Request, zoneID string) {
	switch r.Method {
	case http.MethodPost:
		s.handleDNSZoneCMDBRegister(w, r, zoneID)
	case http.MethodDelete:
		s.handleDNSZoneCMDBUnregister(w, r, zoneID)
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleDNSZoneCMDBRegister 登记zone 到 Domain 台账。同名行已存在（无论是
// inventory 声明、手工登记还是历史 DNS 来源）一律 409 拒绝——声明态保护，
// 由用户先清理旧行再登记，面板绝不静默改写别人的行（D20）。
func (s *Server) handleDNSZoneCMDBRegister(w http.ResponseWriter, r *http.Request, zoneID string) {
	provider := s.requireDNS(w, r)
	if provider == nil {
		return
	}
	zone, ok := s.dnsZoneByID(w, r, provider, zoneID)
	if !ok {
		return
	}
	if existing, err := s.db.GetDomainByName(zone.Name); err == nil && existing != nil {
		s.handleError(w, r, http.StatusConflict,
			fmt.Sprintf("domain %q already registered (id=%s); remove it first if it should be re-registered from DNS", zone.Name, existing.ID))
		return
	} else if err != nil && err != storage.ErrNotFound {
		s.handleError(w, r, http.StatusInternalServerError, "query domain registry failed")
		return
	}
	dom := &storage.Domain{
		ID:       dnsZoneDomainID(zoneID),
		Domain:   strings.TrimSuffix(zone.Name, "."),
		Provider: s.dnsProviderName(),
		Status:   "active",
		Labels:   map[string]string{"source": dnsCMDBSourceLabel, "zone_id": zoneID},
	}
	if err := s.db.UpsertDomain(dom); err != nil {
		log.Printf("[dns-cmdb] register zone %s failed: %v", zoneID, err)
		s.handleError(w, r, http.StatusInternalServerError, "register domain failed")
		return
	}
	s.auditDNSCMDB(r, "dns_cmdb_register", zoneID, dom.Domain)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"registered": true, "id": dom.ID})
}

// handleDNSZoneCMDBUnregister 把 zone 从台账移除（D20）：zone 行 + 该 zone
// 名下全部 DNS 来源记录行（labels.zone_id 命中）一并清除——这也是孤儿行
// 的清理出口（zone 已从 provider 消失时本端点不查 provider，照样可清）。
// 只删 labels.source == dns 的行：ID 命中但来源不是 dns（如 inventory 恰好
// 声明了 dns- 前缀 ID）时 409 拒绝，避免误删声明态。
func (s *Server) handleDNSZoneCMDBUnregister(w http.ResponseWriter, r *http.Request, zoneID string) {
	provider := s.requireDNS(w, r)
	if provider == nil {
		return
	}
	id := dnsZoneDomainID(zoneID)
	// 单趟读台账：定位 zone 行 + 收集该 zone 的记录行（台账行数量级小，
	// 与 zones 的 orphans 对账同一读法）
	domains, err := s.db.ListDomains()
	if err != nil {
		log.Printf("[dns-cmdb] unregister %s list failed: %v", zoneID, err)
		s.handleError(w, r, http.StatusInternalServerError, "query domain registry failed")
		return
	}
	var zoneRow *storage.Domain
	for _, d := range domains {
		if d.ID == id {
			zoneRow = d
		}
	}
	// zone 行可能已不在（孤儿记录行残留而 zone 行先清掉了）：只要该 zone
	// 名下还有 DNS 来源行就照常清；两样都没有才是真正的未登记
	if zoneRow != nil && zoneRow.Labels["source"] != dnsCMDBSourceLabel {
		s.handleError(w, r, http.StatusConflict,
			fmt.Sprintf("domain id=%s is not DNS-managed (source=%q); remove it in inventory instead", id, zoneRow.Labels["source"]))
		return
	}
	// 先清记录级跟随行再删 zone 行：中途失败 500 且 zone 行保留，可重试
	removed := 0
	for _, d := range domains {
		if d.ID == id || d.Labels["source"] != dnsCMDBSourceLabel || d.Labels["zone_id"] != zoneID {
			continue
		}
		if err := s.db.DeleteDomain(d.ID); err != nil {
			log.Printf("[dns-cmdb] unregister %s sweep %s failed: %v", zoneID, d.ID, err)
			s.handleError(w, r, http.StatusInternalServerError, "unregister domain failed")
			return
		}
		removed++
	}
	if zoneRow != nil {
		if err := s.db.DeleteDomain(id); err != nil {
			log.Printf("[dns-cmdb] unregister %s failed: %v", id, err)
			s.handleError(w, r, http.StatusInternalServerError, "unregister domain failed")
			return
		}
	}
	if zoneRow == nil && removed == 0 {
		s.handleError(w, r, http.StatusNotFound, "domain not registered from this zone")
		return
	}
	domain := zoneID
	if zoneRow != nil {
		domain = zoneRow.Domain
	}
	s.auditDNSCMDB(r, "dns_cmdb_unregister", zoneID, domain)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"unregistered": zoneRow != nil, "records_removed": removed})
}

// dnsZoneByID 从 provider 拉全量 zone 找目标（联动与登记的非热路径，zone
// 数量小；cloudflare 翻页由 client 内聚）。找不到回 404。
func (s *Server) dnsZoneByID(w http.ResponseWriter, r *http.Request, provider dns.Provider, zoneID string) (dns.Zone, bool) {
	zones, err := provider.ListZones(r.Context())
	if err != nil {
		s.handleDNSUpstreamError(w, r, err)
		return dns.Zone{}, false
	}
	for _, z := range zones {
		if z.ID == zoneID {
			return z, true
		}
	}
	s.handleError(w, r, http.StatusNotFound, "zone not found on provider")
	return dns.Zone{}, false
}

// auditDNSCMDB 登记/移除审计（联动跟随不单独审计：dns_create/update/delete
// 已在同一动作留痕，避免翻倍；见 todo 假设注明）
func (s *Server) auditDNSCMDB(r *http.Request, action, zoneID, domainName string) {
	username := ""
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, action, audit.ResourceDomain,
		fmt.Sprintf("%s/%s", zoneID, domainName), nil, s.getClientIP(r), r.UserAgent())
}

// linkDNSRecordDomain 记录 CRUD 成功后的台账跟随（upsert）。action 仅用于
// 日志归因。任何一步失败只记日志——DNS 侧已成功，台账尽力而为（D20）。
func (s *Server) linkDNSRecordDomain(action, zoneID string, rec *dns.Record) {
	if rec == nil || !dnsCmdbRecordTypes[rec.Type] {
		return
	}
	zone, ok := s.dnsZoneByNameID(zoneID)
	if !ok {
		log.Printf("[dns-cmdb] %s: skip linkage, zone %s not resolvable", action, zoneID)
		return
	}
	fqdn := dnsRecordFQDN(rec.Name, zone.Name)
	id := dnsRecordDomainID(zoneID, rec.ID)
	existing, err := s.db.GetDomainByName(fqdn)
	if err != nil && err != storage.ErrNotFound {
		log.Printf("[dns-cmdb] %s: query %s failed: %v", action, fqdn, err)
		return
	}
	if existing != nil && existing.ID != id {
		// 同名行属于 inventory 声明/手工登记/其他记录：不覆盖（唯一索引
		// 是最终防线，这里预检以便留下可读日志）
		log.Printf("[dns-cmdb] %s: skip, %s already owned by id=%s", action, fqdn, existing.ID)
		return
	}
	dom := &storage.Domain{
		ID:       id,
		Domain:   fqdn,
		Provider: s.dnsProviderName(),
		Status:   "active",
		Labels: map[string]string{
			"source":    dnsCMDBSourceLabel,
			"zone_id":   zoneID,
			"zone":      zone.Name,
			"record_id": rec.ID,
			"type":      rec.Type,
		},
	}
	if err := s.db.UpsertDomain(dom); err != nil {
		log.Printf("[dns-cmdb] %s: upsert %s (%s) failed: %v", action, id, fqdn, err)
	}
}

// unlinkDNSRecordDomain 记录删除后的台账跟随（幂等：行不存在不报错）。
// 失败只记日志（D20）。
func (s *Server) unlinkDNSRecordDomain(zoneID, recordID string) {
	if err := s.db.DeleteDomain(dnsRecordDomainID(zoneID, recordID)); err != nil {
		log.Printf("[dns-cmdb] delete unlink %s failed: %v", dnsRecordDomainID(zoneID, recordID), err)
	}
}

// dnsZoneByNameID 供记录联动用的 zone 查找（找不到返回 false，调用方跳过）
func (s *Server) dnsZoneByNameID(zoneID string) (dns.Zone, bool) {
	if s.dns == nil {
		return dns.Zone{}, false
	}
	zones, err := s.dns.ListZones(context.Background())
	if err != nil {
		return dns.Zone{}, false
	}
	for _, z := range zones {
		if z.ID == zoneID {
			return z, true
		}
	}
	return dns.Zone{}, false
}

// listDNSOrphans 反向对账（D20）：台账中 DNS 来源、但所属 zone 已不在当前
// provider zone 列表的行（面板外删 zone / 切 provider 后残留）。归属判定
// 口径：记录级行看 labels.zone（自身域名是子域，不能拿去比 zone 名），
// zone 级行看自身域名。返回摘要列表；面板不自动删除——由用户经移除登记
// 处置（DELETE 会把该 zone 的全部 DNS 来源行一并清掉）。
// dnsOrphan 孤儿行摘要（struct 保证 JSON 键序稳定，便于端到端断言）。
// zone_id 供前端定向清理（孤儿 zone 不在 zone 下拉列表里）。
type dnsOrphan struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
	ZoneID string `json:"zone_id"`
}

func (s *Server) listDNSOrphans(zones []dns.Zone) []dnsOrphan {
	zoneNames := make(map[string]bool, len(zones))
	for _, z := range zones {
		zoneNames[dnsNormDomain(z.Name)] = true
	}
	domains, err := s.db.ListDomains()
	if err != nil {
		return nil
	}
	orphans := []dnsOrphan{}
	for _, d := range domains {
		if d.Labels["source"] != dnsCMDBSourceLabel {
			continue
		}
		owner := d.Domain
		if z, ok := d.Labels["zone"]; ok && z != "" {
			owner = z
		}
		if zoneNames[dnsNormDomain(owner)] {
			continue
		}
		orphans = append(orphans, dnsOrphan{ID: d.ID, Domain: d.Domain, ZoneID: d.Labels["zone_id"]})
	}
	return orphans
}
