package server

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/dns"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 服务域名绑定 API（设计见 docs/guide/domain-binding-design.md）。
// DomainBinding 一等资源：登记「域名 → agent 目标服务」唯一事实源，
// apply 按三个 auto 开关各自独立联动（幂等，可重复执行）。
//
//	GET    /api/domains?agent=xxx       列表
//	POST   /api/domains                 登记/更新（domain 唯一 upsert）
//	DELETE /api/domains/{domain}        删登记（D8：不动已下发产物）
//	POST   /api/domains/{domain}/apply  执行联动
//
// 漂移检查（D6）与 agent 侧 domains.list RPC（D7）见分期 P1。

var (
	domainBindingRe = regexp.MustCompile(
		`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)
	// target 两种形态（D2）：host:port 或 docker://服务名[:port]
	domainTargetHostRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*:[0-9]{1,5}$`)
	domainTargetDockerRe = regexp.MustCompile(`^docker://[A-Za-z0-9][A-Za-z0-9_.-]*(:[0-9]{1,5})?$`)
)

type domainBindingPayload struct {
	Domain    string `json:"domain"`
	AgentID   string `json:"agentId"`
	Target    string `json:"target"`
	Enabled   *bool  `json:"enabled"`
	AutoDNS   *bool  `json:"autoDns"`
	AutoProxy *bool  `json:"autoProxy"`
	AutoCert  *bool  `json:"autoCert"`
}

func validateDomainBinding(p *domainBindingPayload) error {
	if !domainBindingRe.MatchString(p.Domain) {
		return errDomainBindingInput("invalid domain (lowercase, at least two labels)")
	}
	if p.AgentID == "" {
		return errDomainBindingInput("agentId is required")
	}
	if !domainTargetHostRe.MatchString(p.Target) && !domainTargetDockerRe.MatchString(p.Target) {
		return errDomainBindingInput("invalid target (host:port or docker://name[:port])")
	}
	return nil
}

type errDomainBindingInput string

func (e errDomainBindingInput) Error() string { return string(e) }

// handleDomainBindings /api/domains 入口
func (s *Server) handleDomainBindings(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/domains")
	switch {
	case path == "" || path == "/":
		if r.Method == http.MethodGet {
			s.handleDomainBindingsList(w, r)
			return
		}
		if r.Method == http.MethodPost {
			s.handleDomainBindingSave(w, r)
			return
		}
	case strings.HasSuffix(path, "/apply"):
		if r.Method == http.MethodPost {
			s.handleDomainBindingApply(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/apply"))
			return
		}
	default:
		if r.Method == http.MethodDelete {
			s.handleDomainBindingDelete(w, r, strings.TrimPrefix(path, "/"))
			return
		}
	}
	s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
}

// handleDomainBindingsList 列表；?agent=xxx 过滤
func (s *Server) handleDomainBindingsList(w http.ResponseWriter, r *http.Request) {
	var list []*storage.DomainBinding
	var err error
	if agentID := r.URL.Query().Get("agent"); agentID != "" {
		list, err = s.db.ListDomainBindingsByAgent(agentID)
	} else {
		list, err = s.db.ListDomainBindings()
	}
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list domain bindings")
		return
	}
	s.writeList(w, list, len(list))
}

// handleDomainBindingSave 登记/更新
func (s *Server) handleDomainBindingSave(w http.ResponseWriter, r *http.Request) {
	var p domainBindingPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := validateDomainBinding(&p); err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	// agent 必须已注册（binding 指向真实节点）
	if _, err := s.db.GetAgent(p.AgentID); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "agent not found: "+p.AgentID)
		return
	}

	b := &storage.DomainBinding{
		Domain: p.Domain, AgentID: p.AgentID, Target: p.Target,
		Enabled: boolOrTrue(p.Enabled), AutoDNS: boolOrTrue(p.AutoDNS),
		AutoProxy: boolOrTrue(p.AutoProxy), AutoCert: boolOrTrue(p.AutoCert),
	}
	if err := s.db.SaveDomainBinding(b); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to save domain binding")
		return
	}

	s.auditDomainBinding(r, audit.ActionUpdate, p.Domain, map[string]interface{}{
		"agent": p.AgentID, "target": p.Target,
		"autoDns": b.AutoDNS, "autoProxy": b.AutoProxy, "autoCert": b.AutoCert,
	})
	s.writeJSON(w, http.StatusOK, b)
}

// handleDomainBindingDelete 删登记（D8）
func (s *Server) handleDomainBindingDelete(w http.ResponseWriter, r *http.Request, domain string) {
	if _, err := s.db.GetDomainBinding(domain); err != nil {
		s.handleError(w, r, http.StatusNotFound, "Domain binding not found")
		return
	}
	if err := s.db.DeleteDomainBinding(domain); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to delete domain binding")
		return
	}
	s.auditDomainBinding(r, audit.ActionDelete, domain, map[string]interface{}{})
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "binding deleted; DNS records / proxy sites / cert monitor entries are left as-is (clean up manually)",
	})
}

// domainApplyStep 单项联动结果
type domainApplyStep struct {
	Name string `json:"name"` // dns / proxy / cert
	OK   bool   `json:"ok"`
	Err  string `json:"error,omitempty"`
}

// handleDomainBindingApply 执行联动：按 auto* 开关逐项执行并聚合回写
func (s *Server) handleDomainBindingApply(w http.ResponseWriter, r *http.Request, domain string) {
	b, err := s.db.GetDomainBinding(domain)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "Domain binding not found")
		return
	}
	if !b.Enabled {
		s.handleError(w, r, http.StatusBadRequest, "binding is disabled")
		return
	}

	steps := s.applyDomainBinding(b)

	status, lastErr := "ok", ""
	for _, st := range steps {
		if !st.OK {
			status = "failed"
			lastErr = st.Name + ": " + st.Err
		}
	}
	if len(steps) == 0 {
		status, lastErr = "failed", "no auto* switch enabled"
	}
	_ = s.db.UpdateDomainBindingApply(domain, status, lastErr, time.Now().Unix())

	s.auditDomainBinding(r, audit.ActionUpdate, domain, map[string]interface{}{
		"apply": status, "steps": steps,
	})
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"domain": domain, "status": status, "steps": steps,
	})
}

// applyDomainBinding 三个联动按需执行；联动间独立（一个失败不阻断其余）
func (s *Server) applyDomainBinding(b *storage.DomainBinding) []domainApplyStep {
	var steps []domainApplyStep
	if b.AutoDNS {
		steps = append(steps, s.applyBindingDNS(b))
	}
	if b.AutoProxy {
		steps = append(steps, s.applyBindingProxy(b))
	}
	if b.AutoCert {
		steps = append(steps, s.applyBindingCert(b))
	}
	return steps
}

// applyBindingDNS 建/改 A 记录（D3：内容 = agent 注册主地址）。
// 记录存在且一致则 no-op（幂等）；更新保留原 TTL/Proxied（DDNS 同规）。
func (s *Server) applyBindingDNS(b *storage.DomainBinding) domainApplyStep {
	step := domainApplyStep{Name: "dns"}
	if s.dns == nil {
		step.Err = "DNS provider not configured"
		return step
	}
	agent, err := s.db.GetAgent(b.AgentID)
	if err != nil {
		step.Err = "agent not found: " + b.AgentID
		return step
	}
	if agent.IP == "" {
		step.Err = "agent has no registered address"
		return step
	}
	ctx := context.Background()

	zone, err := findDNSZoneForDomain(s.dns, ctx, b.Domain)
	if err != nil {
		step.Err = "zone lookup failed: " + err.Error()
		return step
	}

	rec, err := findDNSRecordInZone(s.dns, ctx, zone.ID, "A", b.Domain)
	if err != nil {
		step.Err = "record lookup failed: " + err.Error()
		return step
	}
	if rec == nil {
		input := dns.RecordInput{Type: "A", Name: b.Domain, Content: agent.IP}
		if _, err := s.dns.CreateRecord(ctx, zone.ID, input); err != nil {
			step.Err = "create record failed: " + err.Error()
			return step
		}
		step.OK = true
		return step
	}
	if rec.Content == agent.IP {
		step.OK = true // 已一致，安静幂等
		return step
	}
	input := dns.RecordInput{Type: "A", Name: b.Domain, Content: agent.IP, TTL: rec.TTL, Proxied: rec.Proxied}
	if _, err := s.dns.UpdateRecord(ctx, zone.ID, rec.ID, input); err != nil {
		step.Err = "update record failed: " + err.Error()
		return step
	}
	step.OK = true
	return step
}

// applyBindingProxy 下发反代站点（D4：复用 P1 site.apply，server 纯转发）。
// docker://name[:port] 的 name 即 agent 本机 docker 网络内可解析的服务名。
func (s *Server) applyBindingProxy(b *storage.DomainBinding) domainApplyStep {
	step := domainApplyStep{Name: "proxy"}
	upstream := strings.TrimPrefix(b.Target, "docker://")
	site := map[string]interface{}{
		"name":        bindingSiteName(b.Domain),
		"serverNames": []string{b.Domain},
		"upstream":    upstream,
	}
	resp, err := s.CallAgent(b.AgentID, s.proxyRPCPrefix(b.AgentID)+"site.apply",
		map[string]interface{}{"site": site})
	if err != nil {
		step.Err = "agent unreachable: " + err.Error()
		return step
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		step.Err = "invalid agent response"
		return step
	}
	if rpcResp.Status == "error" {
		step.Err = rpcResp.Error
		if step.Err == "" {
			step.Err = "agent rejected the operation"
		}
		return step
	}
	step.OK = true
	return step
}

// applyBindingCert 纳入证书监控（D5：inventory Domain 即 probe 域名清单；
// 存在则校准归属，缺失则建 pending 记录）
func (s *Server) applyBindingCert(b *storage.DomainBinding) domainApplyStep {
	step := domainApplyStep{Name: "cert"}
	existing, err := s.db.GetDomainByName(b.Domain)
	if err == nil && existing != nil {
		if existing.AgentID == nil || *existing.AgentID != b.AgentID {
			if err := s.db.UpdateDomainAgentID(existing.ID, b.AgentID); err != nil {
				step.Err = "rebind existing domain failed: " + err.Error()
				return step
			}
		}
		step.OK = true
		return step
	}
	d := &storage.Domain{ID: b.Domain, Domain: b.Domain, Status: "pending"}
	agentID := b.AgentID
	d.AgentID = &agentID
	if err := s.db.UpsertDomain(d); err != nil {
		step.Err = "create monitor entry failed: " + err.Error()
		return step
	}
	step.OK = true
	return step
}

// bindingSiteName 域名 → 站点名（proxySiteNameRe 同规则：点换横杠）
func bindingSiteName(domain string) string {
	return strings.ReplaceAll(domain, ".", "-")
}

// findDNSZoneForDomain 最长后缀匹配 zone（blog.example.com → example.com）
func findDNSZoneForDomain(p dns.Provider, ctx context.Context, domain string) (*dns.Zone, error) {
	zones, err := p.ListZones(ctx)
	if err != nil {
		return nil, err
	}
	var best *dns.Zone
	for i := range zones {
		z := zones[i]
		if !strings.HasSuffix(domain, z.Name) {
			continue
		}
		if best == nil || len(z.Name) > len(best.Name) {
			best = &z
		}
	}
	if best == nil {
		return nil, errDomainBindingInput("no DNS zone matches " + domain)
	}
	return best, nil
}

// findDNSRecordInZone 分页拉全 zone 下 type 记录，找 name 匹配（不区分大小写）
func findDNSRecordInZone(p dns.Provider, ctx context.Context, zoneID, recordType, name string) (*dns.Record, error) {
	for page := 1; ; page++ {
		rp, err := p.ListRecords(ctx, zoneID, recordType, page)
		if err != nil {
			return nil, err
		}
		for i := range rp.Records {
			if strings.EqualFold(rp.Records[i].Name, name) {
				return &rp.Records[i], nil
			}
		}
		if page >= rp.TotalPage {
			return nil, nil
		}
	}
}

func boolOrTrue(v *bool) bool {
	if v == nil {
		return true
	}
	return *v
}

func (s *Server) auditDomainBinding(r *http.Request, action, domain string, details map[string]interface{}) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	details["domain"] = domain
	s.audit.LogResource(username, action, audit.ResourceDomainBinding, domain,
		details, s.getClientIP(r), r.UserAgent())
}
