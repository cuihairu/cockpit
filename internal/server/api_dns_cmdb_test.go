package server

// api_dns_cmdb_test.go M3：Domain 台账写联动（dns-design.md D18-D20）。
// 覆盖：zone 显式登记/移除（同名 409 声明态保护、非 DNS 来源 409、三
// provider 识别）、解析记录 CRUD 跟随（类型过滤/FQDN 归一/更新跟随/删除
// 幂等）、provider 失败不写库、同名行不覆盖、zones 反向对账 orphans。

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/dns"
	"github.com/cuihairu/cockpit/internal/storage"
)

// cfCMDBStub cloudflare 桩：zones 固定两 zone；记录动作按 method 分发
func cfCMDBStub(t *testing.T, recordStatus int, recordBody string) *Server {
	t.Helper()
	return newDNSTestServer(t, func(r *http.Request) (int, string) {
		switch {
		case r.URL.Path == "/zones":
			return 200, `{"success":true,"errors":[],"result":[
				{"id":"z1","name":"example.com","status":"active","name_servers":null}
			],"result_info":{"page":1,"total_pages":1}}`
		default:
			return recordStatus, recordBody
		}
	})
}

func mustGetDomain(t *testing.T, s *Server, id string) *storage.Domain {
	t.Helper()
	dom, err := s.db.GetDomain(id)
	if err != nil {
		t.Fatalf("GetDomain(%s) = %v", id, err)
	}
	return dom
}

// TestDNSRecordFQDN 全名归一表驱动（D14 provider 侧已归一，此处兜底：
// 空/@/同名/已带后缀/裸子域/尾点）
func TestDNSRecordFQDN(t *testing.T) {
	cases := []struct{ name, zone, want string }{
		{"", "example.com", "example.com"},
		{"@", "example.com", "example.com"},
		{"example.com", "example.com", "example.com"},
		{"EXAMPLE.com.", "example.com", "example.com"},
		{"app.example.com", "example.com", "app.example.com"},
		{"app", "example.com", "app.example.com"},
		{"deep.sub", "example.com", "deep.sub.example.com"},
	}
	for _, c := range cases {
		if got := dnsRecordFQDN(c.name, c.zone); got != c.want {
			t.Errorf("dnsRecordFQDN(%q, %q) = %q, want %q", c.name, c.zone, got, c.want)
		}
	}
}

// TestDNSCMDBRouting 路由兜底：cmdb 只认 POST/DELETE；多余段与未知段 404
func TestDNSCMDBRouting(t *testing.T) {
	s := cfCMDBStub(t, 200, `{}`)
	if w := doDNS(s, http.MethodPut, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT cmdb = %d, want 405", w.Code)
	}
	if w := doDNS(s, http.MethodGet, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET cmdb = %d, want 405", w.Code)
	}
	if w := doDNS(s, http.MethodGet, "/dns/zones/z1/cmdb/extra", ""); w.Code != http.StatusNotFound {
		t.Errorf("cmdb extra seg = %d, want 404", w.Code)
	}
	if w := doDNS(s, http.MethodGet, "/dns/zones/z1/nope", ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown seg = %d, want 404", w.Code)
	}
	if w := doDNS(s, http.MethodGet, "/dns/zones//cmdb", ""); w.Code != http.StatusNotFound {
		t.Errorf("empty zone seg = %d, want 404", w.Code)
	}
}

// TestDNSCMDBLinkageGuards 联动防御分支：client 缺失/zone 不可解析时
// 静默跳过（DNS 已成功不因台账问题报错，D20）
func TestDNSCMDBLinkageGuards(t *testing.T) {
	// s.dns == nil：直接调用不 panic（handler 层 requireDNS 已挡，双保险）
	s := newBackupTestServer(t)
	s.linkDNSRecordDomain("create", "z1", &dns.Record{Type: "A", ID: "r1", Name: "app"})
	s.unlinkDNSRecordDomain("z1", "r1") // 行不存在幂等
	if _, err := s.db.GetDomain("dns-z1-r1"); err != storage.ErrNotFound {
		t.Errorf("nil-provider linkage must skip, GetDomain = %v", err)
	}

	// zone 不可解析（provider 列表里没有该 zone）→ create 成功但不联动
	s2 := newDNSTestServer(t, func(r *http.Request) (int, string) {
		if r.Method == http.MethodGet && r.URL.Path == "/zones" {
			return 200, `{"success":true,"errors":[],"result":[],"result_info":{"page":1,"total_pages":1}}`
		}
		return 200, `{"success":true,"errors":[],"result":{"id":"ra1","type":"A","name":"app.example.com","content":"1.2.3.4"}}`
	})
	if w := doDNS(s2, http.MethodPost, "/dns/zones/z1/records",
		`{"type":"A","name":"app.example.com","content":"1.2.3.4","ttl":0}`); w.Code != http.StatusOK {
		t.Fatalf("create with unresolvable zone = %d", w.Code)
	}
	if _, err := s2.db.GetDomain("dns-z1-ra1"); err != storage.ErrNotFound {
		t.Errorf("unresolvable zone must skip linkage, GetDomain = %v", err)
	}
	// 非解析类型记录（rec nil / type 不在白名单）提前返回
	s.linkDNSRecordDomain("create", "z1", nil)
	s.linkDNSRecordDomain("create", "z1", &dns.Record{Type: "TXT", ID: "rt1", Name: "example.com"})
}

func TestDNSZoneCMDBRegister(t *testing.T) {
	s := cfCMDBStub(t, 200, `{"success":true,"errors":[],"result":{"id":"r9","type":"A","name":"app.example.com","content":"1.2.3.4"}}`)

	// 未配置凭据 → 503（登记同样受 requireDNS 管辖）
	s2 := newBackupTestServer(t)
	if w := doDNS(s2, http.MethodPost, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured register = %d, want 503", w.Code)
	}

	// 成功登记：字段就近取值断言（D18）
	w := doDNS(s, http.MethodPost, "/dns/zones/z1/cmdb", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"registered":true`) {
		t.Fatalf("register = %d %s", w.Code, w.Body.String())
	}
	dom := mustGetDomain(t, s, "dns-z1")
	if dom.Domain != "example.com" || dom.Provider != "cloudflare" || dom.Status != "active" {
		t.Errorf("registered row = %+v", dom)
	}
	if dom.Labels["source"] != "dns" || dom.Labels["zone_id"] != "z1" {
		t.Errorf("labels = %v", dom.Labels)
	}

	// 重复登记 → 409（幂等保护）
	if w := doDNS(s, http.MethodPost, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusConflict {
		t.Fatalf("duplicate register = %d, want 409", w.Code)
	}

	// 同名 inventory/手工行 → 409 且原行不被改写（D20 声明态保护）
	if err := s.db.UpsertDomain(&storage.Domain{ID: "inv-1", Domain: "other-name.com", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	w = doDNS(s, http.MethodPost, "/dns/zones/unknown-z/cmdb", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown zone = %d, want 404", w.Code)
	}

	// 审计留痕
	logs, _, err := s.db.GetAuditLogs(0, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if l.Action == "dns_cmdb_register" && strings.Contains(l.ResourceID, "z1/example.com") {
			found = true
		}
	}
	if !found {
		t.Fatalf("dns_cmdb_register audit missing: %+v", logs)
	}
}

// TestDNSZoneCMDBRegisterProviders 三 provider 自动识别（D19）：provider
// 名从当前配置读取，三态登记的台账行 Provider 字段各归其位
func TestDNSZoneCMDBRegisterProviders(t *testing.T) {
	cases := []struct {
		provider string
		setup    func(t *testing.T) *Server
		zoneID   string
	}{
		{"cloudflare", func(t *testing.T) *Server { return cfCMDBStub(t, 200, `{}`) }, "z1"},
		{"dnspod", func(t *testing.T) *Server {
			s := newBackupTestServer(t)
			s.cfg = &config.Config{DNS: &config.DNSConfig{Provider: "dnspod"}}
			s.dns = dns.NewDNSPodWithBase("42,tok", newDNSPodAPIServer(t).URL) // Domain.List → example.com
			return s
		}, "example.com"},
		{"alidns", func(t *testing.T) *Server {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("Action") == "DescribeDomains" {
					_, _ = w.Write([]byte(`{"Domains":{"Domain":[{"DomainName":"example.com"}]}}`))
					return
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			t.Cleanup(srv.Close)
			s := newBackupTestServer(t)
			s.cfg = &config.Config{DNS: &config.DNSConfig{Provider: "alidns"}}
			s.dns = dns.NewAliDNSWithBase("ak", "sk", srv.URL)
			return s
		}, "example.com"},
	}
	for _, c := range cases {
		s := c.setup(t)
		w := doDNS(s, http.MethodPost, "/dns/zones/"+c.zoneID+"/cmdb", "")
		if w.Code != http.StatusOK {
			t.Errorf("provider %s: register = %d %s", c.provider, w.Code, w.Body.String())
			continue
		}
		dom := mustGetDomain(t, s, "dns-"+c.zoneID)
		if dom.Provider != c.provider {
			t.Errorf("provider %s: row.Provider = %q", c.provider, dom.Provider)
		}
		if dom.Domain != "example.com" {
			t.Errorf("provider %s: row.Domain = %q", c.provider, dom.Domain)
		}
	}
}

func TestDNSZoneCMDBUnregister(t *testing.T) {
	s := cfCMDBStub(t, 200, `{}`)

	// 未登记 → 404
	if w := doDNS(s, http.MethodDelete, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unregister missing = %d, want 404", w.Code)
	}
	// 登记 → 移除 → 行消失
	if w := doDNS(s, http.MethodPost, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusOK {
		t.Fatalf("register = %d", w.Code)
	}
	if w := doDNS(s, http.MethodDelete, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusOK {
		t.Fatalf("unregister = %d %s", w.Code, w.Body.String())
	}
	if _, err := s.db.GetDomain("dns-z1"); err != storage.ErrNotFound {
		t.Fatalf("after unregister GetDomain = %v, want ErrNotFound", err)
	}
	// 再登记并带记录级跟随行 → 移除时记录行一并清扫（D20）
	if w := doDNS(s, http.MethodPost, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusOK {
		t.Fatalf("re-register = %d", w.Code)
	}
	if err := s.db.UpsertDomain(&storage.Domain{
		ID: "dns-z1-ra1", Domain: "app.example.com", Provider: "cloudflare",
		Labels: map[string]string{"source": "dns", "zone_id": "z1", "zone": "example.com"},
	}); err != nil {
		t.Fatal(err)
	}
	if w := doDNS(s, http.MethodDelete, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusOK {
		t.Fatalf("unregister with records = %d %s", w.Code, w.Body.String())
	}
	if _, err := s.db.GetDomain("dns-z1-ra1"); err != storage.ErrNotFound {
		t.Errorf("record row must be swept with zone, GetDomain = %v", err)
	}
	// 审计
	logs, _, _ := s.db.GetAuditLogs(0, 50, nil)
	found := false
	for _, l := range logs {
		if l.Action == "dns_cmdb_unregister" {
			found = true
		}
	}
	if !found {
		t.Fatal("dns_cmdb_unregister audit missing")
	}

	// 非 DNS 来源行（ID 撞 dns- 前缀的声明态）→ 409 拒删（D20）
	if err := s.db.UpsertDomain(&storage.Domain{
		ID: "dns-z2", Domain: "declared.com", Status: "active", Labels: map[string]string{},
	}); err != nil {
		t.Fatal(err)
	}
	if w := doDNS(s, http.MethodDelete, "/dns/zones/z2/cmdb", ""); w.Code != http.StatusConflict {
		t.Fatalf("unregister declared row = %d, want 409", w.Code)
	}
	if dom := mustGetDomain(t, s, "dns-z2"); dom.Domain != "declared.com" {
		t.Errorf("declared row must survive: %+v", dom)
	}
}

func TestDNSRecordCMDBLinkage(t *testing.T) {
	s := newDNSTestServer(t, func(r *http.Request) (int, string) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path == "/zones" {
				return 200, `{"success":true,"errors":[],"result":[
					{"id":"z1","name":"example.com","status":"active","name_servers":null}
				],"result_info":{"page":1,"total_pages":1}}`
			}
			return 200, `{"success":true,"errors":[],"result":[],"result_info":{"page":1,"total_pages":1}}`
		case http.MethodPost:
			// create 的 type 在 JSON body 里，据此分流（A 裸子域测 FQDN
			// 补全；MX 测类型过滤）
			buf, _ := io.ReadAll(r.Body)
			if strings.Contains(string(buf), `"type":"MX"`) {
				return 200, `{"success":true,"errors":[],"result":{"id":"rm1","type":"MX","name":"example.com","content":"10 mail.example.com","ttl":600}}`
			}
			return 200, `{"success":true,"errors":[],"result":{"id":"ra1","type":"A","name":"app","content":"1.2.3.4","ttl":1}}`
		case http.MethodPut:
			return 200, `{"success":true,"errors":[],"result":{"id":"ra1","type":"CNAME","name":"renamed.example.com","content":"target.com","ttl":300}}`
		case http.MethodDelete:
			return 200, `{"success":true,"errors":[],"result":{"id":"ra1"}}`
		}
		return 500, `{"success":false}`
	})

	// A 记录创建 → 台账跟随（裸子域补全 FQDN；D18）
	w := doDNS(s, http.MethodPost, "/dns/zones/z1/records",
		`{"type":"A","name":"app","content":"1.2.3.4","ttl":0}`)
	if w.Code != http.StatusOK {
		t.Fatalf("create A = %d %s", w.Code, w.Body.String())
	}
	dom := mustGetDomain(t, s, "dns-z1-ra1")
	if dom.Domain != "app.example.com" || dom.Provider != "cloudflare" || dom.Status != "active" {
		t.Errorf("A linkage row = %+v", dom)
	}
	if dom.Labels["type"] != "A" || dom.Labels["record_id"] != "ra1" || dom.Labels["zone"] != "example.com" {
		t.Errorf("A linkage labels = %v", dom.Labels)
	}

	// MX 记录创建 → 不联动（非域名资产，D18）
	if w := doDNS(s, http.MethodPost, "/dns/zones/z1/records",
		`{"type":"MX","name":"@","content":"10 mail.example.com","ttl":600}`); w.Code != http.StatusOK {
		t.Fatalf("create MX = %d", w.Code)
	}
	if _, err := s.db.GetDomain("dns-z1-rm1"); err != storage.ErrNotFound {
		t.Errorf("MX must not link, GetDomain = %v", err)
	}

	// 更新（改 name/type）→ 行原地 upsert 跟随（D19 确定性 ID）
	w = doDNS(s, http.MethodPut, "/dns/zones/z1/records/ra1",
		`{"type":"CNAME","name":"renamed.example.com","content":"target.com","ttl":300}`)
	if w.Code != http.StatusOK {
		t.Fatalf("update = %d", w.Code)
	}
	dom = mustGetDomain(t, s, "dns-z1-ra1")
	if dom.Domain != "renamed.example.com" || dom.Labels["type"] != "CNAME" {
		t.Errorf("after update row = %+v labels=%v", dom, dom.Labels)
	}

	// 删除 → 行移除；重复 unlink（行已不在）幂等不报错
	if w := doDNS(s, http.MethodDelete, "/dns/zones/z1/records/ra1", ""); w.Code != http.StatusOK {
		t.Fatalf("delete = %d", w.Code)
	}
	if _, err := s.db.GetDomain("dns-z1-ra1"); err != storage.ErrNotFound {
		t.Errorf("after delete GetDomain = %v, want ErrNotFound", err)
	}
	s.unlinkDNSRecordDomain("z1", "ra1") // 幂等：无行不炸
}

// TestDNSRecordCMDBNoWriteOnFailure provider 失败不写库（D20：DNS 侧是
// 事实源，失败时联动根本不发生）
func TestDNSRecordCMDBNoWriteOnFailure(t *testing.T) {
	s := cfCMDBStub(t, 500, `{"success":false,"errors":[{"code":10000,"message":"upstream down"}]}`)
	w := doDNS(s, http.MethodPost, "/dns/zones/z1/records",
		`{"type":"A","name":"app","content":"1.2.3.4","ttl":0}`)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("failed create = %d, want 502", w.Code)
	}
	domains, err := s.db.ListDomains()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range domains {
		if strings.HasPrefix(d.ID, "dns-z1-") {
			t.Errorf("failed create must not write registry, got %s", d.ID)
		}
	}
}

// TestDNSRecordCMDBNameConflict 同名行归属他人（inventory/手工）→ provider
// 成功但台账不被覆盖（D20），原行原样保留
func TestDNSRecordCMDBNameConflict(t *testing.T) {
	s := cfCMDBStub(t, 200, `{"success":true,"errors":[],"result":{"id":"ra1","type":"A","name":"app.example.com","content":"1.2.3.4"}}`)
	if err := s.db.UpsertDomain(&storage.Domain{
		ID: "manual-1", Domain: "app.example.com", Status: "active", Labels: map[string]string{"owner": "me"},
	}); err != nil {
		t.Fatal(err)
	}
	w := doDNS(s, http.MethodPost, "/dns/zones/z1/records",
		`{"type":"A","name":"app.example.com","content":"1.2.3.4","ttl":0}`)
	if w.Code != http.StatusOK {
		t.Fatalf("create = %d %s", w.Code, w.Body.String())
	}
	dom := mustGetDomain(t, s, "manual-1")
	if dom.Labels["owner"] != "me" || dom.Provider != "" {
		t.Errorf("manual row must survive untouched: %+v", dom)
	}
	if _, err := s.db.GetDomain("dns-z1-ra1"); err != storage.ErrNotFound {
		t.Errorf("dns row must not be created on conflict: %v", err)
	}
}

// TestDNSCMDBErrorBranches 错误分支收口：DB 故障（closed db / 触发器阻断
// INSERT·DELETE）、上游 zone 列表失败、审计用户名分支。生产路径永不失败
// 的防御分支经注入覆盖（对齐 cov_inject_test.go 先例）。
func TestDNSCMDBErrorBranches(t *testing.T) {
	// 登记：同名预检查询故障（closed db）→ 500
	s := cfCMDBStub(t, 200, `{}`)
	_ = s.db.Close()
	if w := doDNS(s, http.MethodPost, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusInternalServerError {
		t.Errorf("register with closed db = %d %s, want 500", w.Code, w.Body.String())
	}

	// 登记：写库被触发器阻断 → 500（读路径正常，错误归因到写）
	s2 := cfCMDBStub(t, 200, `{}`)
	if err := s2.db.Session().Exec("CREATE TRIGGER block_domain_ins BEFORE INSERT ON domains BEGIN SELECT RAISE(ABORT, 'blocked'); END").Error; err != nil {
		t.Fatal(err)
	}
	if w := doDNS(s2, http.MethodPost, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusInternalServerError {
		t.Errorf("register blocked insert = %d, want 500", w.Code)
	}

	// 移除：未配置凭据 → requireDNS 拦下 503
	if w := doDNS(newBackupTestServer(t), http.MethodDelete, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusServiceUnavailable {
		t.Errorf("unregister unconfigured = %d, want 503", w.Code)
	}

	// 移除：删除被触发器阻断 → 500
	s3 := cfCMDBStub(t, 200, `{}`)
	if w := doDNS(s3, http.MethodPost, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusOK {
		t.Fatalf("seed register = %d", w.Code)
	}
	if err := s3.db.Session().Exec("CREATE TRIGGER block_domain_del BEFORE DELETE ON domains BEGIN SELECT RAISE(ABORT, 'blocked'); END").Error; err != nil {
		t.Fatal(err)
	}
	if w := doDNS(s3, http.MethodDelete, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusInternalServerError {
		t.Errorf("unregister blocked delete = %d, want 500", w.Code)
	}

	// 移除：台账读取故障（closed db）→ 500
	s4 := cfCMDBStub(t, 200, `{}`)
	_ = s4.db.Close()
	if w := doDNS(s4, http.MethodDelete, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusInternalServerError {
		t.Errorf("unregister with closed db = %d, want 500", w.Code)
	}

	// 移除：记录行清扫中途失败 → 500 且 zone 行保留（可重试语义）
	s4b := cfCMDBStub(t, 200, `{}`)
	if err := s4b.db.UpsertDomain(&storage.Domain{ID: "dns-z1", Domain: "example.com",
		Labels: map[string]string{"source": "dns", "zone_id": "z1"}}); err != nil {
		t.Fatal(err)
	}
	if err := s4b.db.UpsertDomain(&storage.Domain{ID: "dns-z1-rx", Domain: "x.example.com",
		Labels: map[string]string{"source": "dns", "zone_id": "z1"}}); err != nil {
		t.Fatal(err)
	}
	if err := s4b.db.Session().Exec("CREATE TRIGGER block_domain_del BEFORE DELETE ON domains BEGIN SELECT RAISE(ABORT, 'blocked'); END").Error; err != nil {
		t.Fatal(err)
	}
	if w := doDNS(s4b, http.MethodDelete, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusInternalServerError {
		t.Errorf("unregister sweep error = %d, want 500", w.Code)
	}
	if _, err := s4b.db.GetDomain("dns-z1"); err != nil {
		t.Errorf("zone row must survive failed sweep for retry: %v", err)
	}

	// 登记：zone 列表上游失败 → 502（handleDNSUpstreamError 透传）
	s5 := newDNSTestServer(t, func(r *http.Request) (int, string) {
		return 500, `{"success":false,"errors":[{"code":10000,"message":"zone list down"}],"result":null}`
	})
	if w := doDNS(s5, http.MethodPost, "/dns/zones/z1/cmdb", ""); w.Code != http.StatusBadGateway {
		t.Errorf("register zones upstream error = %d, want 502", w.Code)
	}

	// 审计用户名分支：带登录态的登记 → 审计行携带 username
	s6 := cfCMDBStub(t, 200, `{}`)
	r := httptest.NewRequest(http.MethodPost, "/dns/zones/z1/cmdb", strings.NewReader(""))
	r = r.WithContext(auth.ContextWithUser(r.Context(), "u1", "alice", "admin"))
	w6 := httptest.NewRecorder()
	s6.handleDNS(w6, r)
	if w6.Code != http.StatusOK {
		t.Fatalf("register with user = %d %s", w6.Code, w6.Body.String())
	}
	logs, _, err := s6.db.GetAuditLogs(0, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Username != "alice" || logs[0].Resource != "domain" {
		t.Fatalf("audit with user = %+v", logs)
	}

	// 联动查询故障：closed db → GetDomainByName 失败只记日志跳过
	s7 := cfCMDBStub(t, 200, `{}`)
	_ = s7.db.Close()
	s7.linkDNSRecordDomain("create", "z1", &dns.Record{Type: "A", ID: "r1", Name: "app"})

	// 联动写故障：INSERT 被阻断 → DNS 成功（200），台账跟随只记日志
	s8 := cfCMDBStub(t, 200, `{"success":true,"errors":[],"result":{"id":"ra9","type":"A","name":"shop.example.com","content":"1.2.3.4","ttl":1}}`)
	if err := s8.db.Session().Exec("CREATE TRIGGER block_domain_ins2 BEFORE INSERT ON domains BEGIN SELECT RAISE(ABORT, 'blocked'); END").Error; err != nil {
		t.Fatal(err)
	}
	if w := doDNS(s8, http.MethodPost, "/dns/zones/z1/records",
		`{"type":"A","name":"shop","content":"1.2.3.4","ttl":0}`); w.Code != http.StatusOK {
		t.Errorf("create with blocked linkage = %d, want 200", w.Code)
	}
	// unlink 删除故障：行存在 + DELETE 被阻断 → 只记日志不炸（行不存在时
	// gorm Delete 0 行不报错，须先落行；换独立 server 避开上面的 INSERT 阻断）
	s9 := cfCMDBStub(t, 200, `{}`)
	if err := s9.db.UpsertDomain(&storage.Domain{ID: "dns-z1-rx", Domain: "x.example.com", Labels: map[string]string{"source": "dns"}}); err != nil {
		t.Fatal(err)
	}
	if err := s9.db.Session().Exec("CREATE TRIGGER block_domain_del BEFORE DELETE ON domains BEGIN SELECT RAISE(ABORT, 'blocked'); END").Error; err != nil {
		t.Fatal(err)
	}
	s9.unlinkDNSRecordDomain("z1", "rx")

	// 反向对账读故障：closed db → orphans 为 nil（zones 响应不炸）
	s10 := cfCMDBStub(t, 200, `{}`)
	_ = s10.db.Close()
	if got := s10.listDNSOrphans([]dns.Zone{{ID: "z1", Name: "example.com"}}); got != nil {
		t.Errorf("orphans on closed db = %+v, want nil", got)
	}
}

// TestDNSZonesOrphans 反向对账（D20）：DNS 来源行且所属 zone 已不在列表 →
// orphans。归属口径：记录级行看 labels.zone（存活 zone 的记录行不能按自身
// 域名误判孤儿），zone 级行看自身域名；inventory 行永不参与。
func TestDNSZonesOrphans(t *testing.T) {
	s := cfCMDBStub(t, 200, `{}`)
	seed := []*storage.Domain{
		{ID: "dns-gone", Domain: "gone.com", Provider: "cloudflare",
			Labels: map[string]string{"source": "dns"}},
		{ID: "inv-other", Domain: "other.com", Status: "active",
			Labels: map[string]string{"source": "inventory"}},
		{ID: "dns-match", Domain: "example.com", Provider: "cloudflare",
			Labels: map[string]string{"source": "dns"}},
		// 记录级：存活 zone 的跟随行 → 不是孤儿（自身域名 ≠ zone 名）
		{ID: "dns-z1-ra1", Domain: "app.example.com", Provider: "cloudflare",
			Labels: map[string]string{"source": "dns", "zone": "example.com", "zone_id": "z1"}},
		// 记录级：zone 已从 provider 消失 → 孤儿
		{ID: "dns-zg-rp1", Domain: "shop.gone.com", Provider: "cloudflare",
			Labels: map[string]string{"source": "dns", "zone": "gone.com", "zone_id": "zg"}},
		// 记录级但非 DNS 来源（inventory 声明的同构行）→ 不参与对账
		{ID: "inv-gone-sub", Domain: "sub.gone.com", Status: "active",
			Labels: map[string]string{"zone": "gone.com"}},
	}
	for _, d := range seed {
		if err := s.db.UpsertDomain(d); err != nil {
			t.Fatal(err)
		}
	}
	w := doDNS(s, http.MethodGet, "/dns/zones", "")
	if w.Code != http.StatusOK {
		t.Fatalf("zones = %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// 两个 gone.com 来源行（zone 级 + 记录级）都算孤儿，摘要带 zone_id 供前端定向清理
	if !strings.Contains(body, `{"id":"dns-gone","domain":"gone.com","zone_id":""}`) {
		t.Errorf("orphan zone row wrong: %s", body)
	}
	if !strings.Contains(body, `{"id":"dns-zg-rp1","domain":"shop.gone.com","zone_id":"zg"}`) {
		t.Errorf("orphan record row wrong: %s", body)
	}
	// 存活记录行/匹配 zone 行/非 DNS 行不得出现
	for _, absent := range []string{`"id":"dns-z1-ra1"`, `"id":"dns-match"`, `"id":"inv-other"`, `"id":"inv-gone-sub"`} {
		if strings.Contains(body, absent) {
			t.Errorf("must not be orphan (%s): %s", absent, body)
		}
	}
}

// TestDNSZoneCMDBUnregisterCleansOrphans 孤儿清理出口（D20）：zone 已从
// provider 消失时 unregister 不查 provider，仍可把该 zone 名下全部 DNS
// 来源行（zone 行 + 记录行）一次清干净；非 DNS 来源的同 zone_id 行不动。
func TestDNSZoneCMDBUnregisterCleansOrphans(t *testing.T) {
	s := cfCMDBStub(t, 200, `{}`)
	seed := []*storage.Domain{
		{ID: "dns-zg", Domain: "gone.com", Provider: "cloudflare",
			Labels: map[string]string{"source": "dns", "zone_id": "zg"}},
		{ID: "dns-zg-rp1", Domain: "shop.gone.com", Provider: "cloudflare",
			Labels: map[string]string{"source": "dns", "zone_id": "zg", "zone": "gone.com"}},
		{ID: "inv-zg", Domain: "keep.gone.com", Status: "active",
			Labels: map[string]string{"source": "inventory", "zone_id": "zg"}},
	}
	for _, d := range seed {
		if err := s.db.UpsertDomain(d); err != nil {
			t.Fatal(err)
		}
	}
	w := doDNS(s, http.MethodDelete, "/dns/zones/zg/cmdb", "")
	if w.Code != http.StatusOK {
		t.Fatalf("unregister orphan zone = %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"records_removed":1`) {
		t.Errorf("resp = %s, want records_removed=1", w.Body.String())
	}
	for _, gone := range []string{"dns-zg", "dns-zg-rp1"} {
		if _, err := s.db.GetDomain(gone); err != storage.ErrNotFound {
			t.Errorf("%s must be removed, GetDomain = %v", gone, err)
		}
	}
	if _, err := s.db.GetDomain("inv-zg"); err != nil {
		t.Errorf("inventory row must survive: %v", err)
	}

	// 仅剩孤儿记录行（zone 行已先清掉）→ 照常清，unregistered=false
	s2 := cfCMDBStub(t, 200, `{}`)
	if err := s2.db.UpsertDomain(&storage.Domain{
		ID: "dns-zh-rp2", Domain: "old.host-here.com", Provider: "cloudflare",
		Labels: map[string]string{"source": "dns", "zone_id": "zh", "zone": "host-here.com"},
	}); err != nil {
		t.Fatal(err)
	}
	w2 := doDNS(s2, http.MethodDelete, "/dns/zones/zh/cmdb", "")
	if w2.Code != http.StatusOK || !strings.Contains(w2.Body.String(), `"unregistered":false`) ||
		!strings.Contains(w2.Body.String(), `"records_removed":1`) {
		t.Fatalf("sweep orphan records only = %d %s", w2.Code, w2.Body.String())
	}
	if _, err := s2.db.GetDomain("dns-zh-rp2"); err != storage.ErrNotFound {
		t.Errorf("orphan record row must be removed: %v", err)
	}
	// 清完再清 → 真正的未登记 404
	if w := doDNS(s2, http.MethodDelete, "/dns/zones/zh/cmdb", ""); w.Code != http.StatusNotFound {
		t.Errorf("double cleanup = %d, want 404", w.Code)
	}
}
