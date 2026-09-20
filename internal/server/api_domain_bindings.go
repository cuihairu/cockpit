package server

import (
	"context"
	"encoding/json"
	"errors"
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
//	GET    /api/domains/drift?agent=xxx 漂移检查（D6，实时逐条）
//	POST   /api/domains                 登记/更新（domain 唯一 upsert）
//	DELETE /api/domains/{domain}        删登记（D8：不动已下发产物）
//	POST   /api/domains/{domain}/apply  执行联动
//
// agent 侧 domains.list RPC（D7）见分期 P1。

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
	case path == "/drift":
		if r.Method == http.MethodGet {
			s.handleDomainBindingsDrift(w, r)
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

// domainDriftCheck 单路联动漂移结果（D6 细化见设计文档）：
// ok 无漂移 / status 确认漂移（missing|mismatch|foreign，附 expected/actual）/
// error 检查失败不可判定——三态语义，前端红黄区分
type domainDriftCheck struct {
	Checked  bool   `json:"checked"` // auto* 开才查
	OK       bool   `json:"ok"`
	Status   string `json:"status,omitempty"` // missing / mismatch / foreign
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Error    string `json:"error,omitempty"`
}

// domainDriftReport 单条绑定的三路检查报告
type domainDriftReport struct {
	Domain  string           `json:"domain"`
	Enabled bool             `json:"enabled"` // false = 整条未检查
	DNS     domainDriftCheck `json:"dns"`
	Proxy   domainDriftCheck `json:"proxy"`
	Cert    domainDriftCheck `json:"cert"`
}

// handleDomainBindingsDrift 漂移检查（D6）：漂移 = auto* 开关承诺的状态与实际的
// 差距——开关关没承诺就没漂移（checked=false），enabled=false 整条跳过。
// 实时逐条检查（DNS 打 provider API、proxy 打 agent RPC），故独立于列表端点
// 按需触发，不内嵌列表。
func (s *Server) handleDomainBindingsDrift(w http.ResponseWriter, r *http.Request) {
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
	items := make([]domainDriftReport, 0, len(list))
	for _, b := range list {
		rep := domainDriftReport{Domain: b.Domain, Enabled: b.Enabled}
		if b.Enabled {
			if b.AutoDNS {
				rep.DNS = s.checkBindingDNS(b)
			}
			if b.AutoProxy {
				rep.Proxy = s.checkBindingProxy(b)
			}
			if b.AutoCert {
				rep.Cert = s.checkBindingCert(b)
			}
		}
		items = append(items, rep)
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items, "checkedAt": time.Now().Unix(),
	})
}

// checkBindingDNS 期望 = agent 注册主地址（与 applyBindingDNS 同源）；
// 实际 = provider 内该域名 A 记录
func (s *Server) checkBindingDNS(b *storage.DomainBinding) domainDriftCheck {
	c := domainDriftCheck{Checked: true}
	if s.dns == nil {
		c.Error = "DNS provider not configured"
		return c
	}
	agent, err := s.db.GetAgent(b.AgentID)
	if err != nil {
		c.Error = "agent not found: " + b.AgentID
		return c
	}
	if agent.IP == "" {
		c.Error = "agent has no registered address"
		return c
	}
	c.Expected = agent.IP
	ctx := context.Background()
	zone, err := findDNSZoneForDomain(s.dns, ctx, b.Domain)
	if err != nil {
		c.Error = "zone lookup failed: " + err.Error()
		return c
	}
	rec, err := findDNSRecordInZone(s.dns, ctx, zone.ID, "A", b.Domain)
	if err != nil {
		c.Error = "record lookup failed: " + err.Error()
		return c
	}
	switch {
	case rec == nil:
		c.Status = "missing"
	case rec.Content != agent.IP:
		c.Status = "mismatch"
		c.Actual = rec.Content
	default:
		c.OK = true
	}
	return c
}

// checkBindingProxy site.get 查站点；「site not found」error 是确认未下发，
// 其余 RPC 错是检查失败；站点在则比对 upstream（docker:// 归一）与 serverNames
func (s *Server) checkBindingProxy(b *storage.DomainBinding) domainDriftCheck {
	c := domainDriftCheck{Checked: true}
	expected := strings.TrimPrefix(b.Target, "docker://")
	c.Expected = expected
	resp, err := s.CallAgent(b.AgentID, s.proxyRPCPrefix(b.AgentID)+"site.get",
		map[string]interface{}{"name": bindingSiteName(b.Domain)})
	if err != nil {
		c.Error = "agent unreachable: " + err.Error()
		return c
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		c.Error = "invalid agent response"
		return c
	}
	if rpcResp.Status == "error" {
		if strings.Contains(rpcResp.Error, "site not found") {
			c.Status = "missing"
			return c
		}
		c.Error = rpcResp.Error
		if c.Error == "" {
			c.Error = "agent rejected the operation"
		}
		return c
	}
	data, _ := rpcResp.Data.(map[string]interface{})
	site, _ := data["site"].(map[string]interface{})
	upstream, _ := site["upstream"].(string)
	switch {
	case upstream == "":
		c.Status = "mismatch"
		c.Actual = "(no upstream in site data)"
	case upstream != expected:
		c.Status = "mismatch"
		c.Actual = upstream
	case !serverNamesContain(site["serverNames"], b.Domain):
		c.Status = "mismatch"
		c.Actual = upstream + " (serverNames missing " + b.Domain + ")"
	default:
		c.OK = true
	}
	return c
}

// checkBindingCert 监控缺失 missing、被其他 agent 占 foreign（inventory Domain）
func (s *Server) checkBindingCert(b *storage.DomainBinding) domainDriftCheck {
	c := domainDriftCheck{Checked: true, Expected: b.AgentID}
	existing, err := s.db.GetDomainByName(b.Domain)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			c.Status = "missing"
			return c
		}
		c.Error = "lookup failed: " + err.Error()
		return c
	}
	if existing.AgentID == nil || *existing.AgentID != b.AgentID {
		c.Status = "foreign"
		if existing.AgentID != nil {
			c.Actual = *existing.AgentID
		} else {
			c.Actual = "(unassigned)"
		}
		return c
	}
	c.OK = true
	return c
}

// serverNamesContain RPC 数据里的 serverNames（[]interface{}）是否含 domain
// （不区分大小写，与 findDNSRecordInZone 的 name 比对同规）
func serverNamesContain(v interface{}, domain string) bool {
	list, ok := v.([]interface{})
	if !ok {
		return false
	}
	for _, item := range list {
		if s, ok := item.(string); ok && strings.EqualFold(s, domain) {
			return true
		}
	}
	return false
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

// handleAgentDomainsAPI D7 agent 域名引用：/agents/{id}/domains 清单与
// /agents/{id}/domains/snippet 配置片段（rest 形如 "{agentID}/domains..."）。
// agent 零协议变更——清单事实源在 server 库，消费方是人/脚本/配置生成。
func (s *Server) handleAgentDomainsAPI(w http.ResponseWriter, r *http.Request, rest string) {
	idx := strings.Index(rest, "/domains")
	agentID, sub := rest[:idx], rest[idx+len("/domains"):]
	// 清单先行：库故障 500；agent 不存在 → 404（空清单与坏 id 区分）
	list, err := s.db.ListDomainBindingsByAgent(agentID)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list domain bindings")
		return
	}
	if _, err := s.db.GetAgent(agentID); err != nil {
		s.handleError(w, r, http.StatusNotFound, "Agent not found")
		return
	}
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if sub == "/snippet" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(domainBindingsSnippet(agentID, list)))
		return
	}
	if sub != "" && sub != "/" {
		s.handleError(w, r, http.StatusNotFound, "Not found")
		return
	}
	s.writeList(w, list, len(list))
}

// domainBindingsSnippet 配置片段（text/plain）：env 行 + nginx server_name 行，
// 只取 enabled 绑定——curl 即得，agent 上的任意服务可 source/复制
func domainBindingsSnippet(agentID string, list []*storage.DomainBinding) string {
	var domains []string
	for _, b := range list {
		if b.Enabled {
			domains = append(domains, b.Domain)
		}
	}
	var sb strings.Builder
	sb.WriteString("# cockpit domains for agent " + agentID +
		" (generated " + time.Now().UTC().Format(time.RFC3339) + ")\n")
	if len(domains) == 0 {
		sb.WriteString("# no enabled domain bindings\n")
		return sb.String()
	}
	sb.WriteString("COCKPIT_AGENT_DOMAINS=\"" + strings.Join(domains, " ") + "\"\n")
	sb.WriteString("\n# nginx\n")
	for _, d := range domains {
		sb.WriteString("server_name " + d + ";\n")
	}
	return sb.String()
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
