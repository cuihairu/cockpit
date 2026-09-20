package server

// cov_domain_bindings_test.go 服务域名绑定 API 覆盖：CRUD 校验分支与
// apply 三联动（DNS fake provider / fake agent RPC / inventory upsert）。

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/dns"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// fakeDNSProvider 复用 ddns_scan_test.go 的桩（zones 字段供 zone 匹配用）

// newBindingFakeDNS 带 example.com zone 的 DNS 桩
func newBindingFakeDNS() *fakeDNSProvider {
	return &fakeDNSProvider{zones: []dns.Zone{{ID: "z1", Name: "example.com"}}}
}

// findRecord 按名字查桩内记录（不区分大小写）
func (f *fakeDNSProvider) findRecord(name string) *dns.Record {
	for i := range f.records {
		if strings.EqualFold(f.records[i].Name, name) {
			return &f.records[i]
		}
	}
	return nil
}

// seedBindingAgent 建库内 agent（带主地址）。先删后建：GORM 的
// Assign(struct) 对已存在记录不落库，二次 UpsertAgent 改不动 IP，
// 测试模拟「agent 换 IP」需走删除重建。
func seedBindingAgent(t *testing.T, s *Server, id, ip string) {
	t.Helper()
	_ = s.db.DeleteAgent(id)
	if err := s.db.UpsertAgent(&storage.Agent{ID: id, Hostname: id, IP: ip, Status: "online"}); err != nil {
		t.Fatalf("upsert agent: %v", err)
	}
}

// postBinding POST /api/domains（token 直发鉴权，兼容只读/已关库场景）
func postBinding(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec, req := doAuthenticatedRequest(s, http.MethodPost, "/api/domains", []byte(body))
	s.serveAPI(rec, req)
	return rec
}

func bindingBody(domain, agent, target string) string {
	return fmt.Sprintf(`{"domain":%q,"agentId":%q,"target":%q}`, domain, agent, target)
}

func TestDomainBindingAPISaveAndList(t *testing.T) {
	s := newTestServerWithDB(t)
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	seedBindingAgent(t, s, "a2", "203.0.113.11")

	if rec := postBinding(t, s, bindingBody("blog.example.com", "a1", "127.0.0.1:8080")); rec.Code != http.StatusOK {
		t.Fatalf("save code = %d, body = %s", rec.Code, rec.Body)
	}
	if rec := postBinding(t, s, bindingBody("api.example.com", "a2", "docker://web")); rec.Code != http.StatusOK {
		t.Fatalf("save a2 code = %d", rec.Code)
	}

	// 全量列表
	_, req := doAuthenticatedRequest(s, http.MethodGet, "/api/domains", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list code = %d", rec.Code)
	}
	var list struct {
		Data []*storage.DomainBinding `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list.Data) != 2 {
		t.Fatalf("list len = %d, want 2", len(list.Data))
	}

	// ?agent= 过滤
	_, req = doAuthenticatedRequest(s, http.MethodGet, "/api/domains?agent=a2", nil)
	rec = httptest.NewRecorder()
	s.serveAPI(rec, req)
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list.Data) != 1 || list.Data[0].AgentID != "a2" {
		t.Fatalf("filtered list = %+v", list.Data)
	}
}

func TestDomainBindingAPISaveValidation(t *testing.T) {
	s := newTestServerWithDB(t)
	seedBindingAgent(t, s, "a1", "203.0.113.10")

	cases := []struct {
		name, body string
	}{
		{"bad domain", bindingBody("NotAHost", "a1", "127.0.0.1:80")},
		{"single label", bindingBody("localhost", "a1", "127.0.0.1:80")},
		{"empty agent", bindingBody("blog.example.com", "", "127.0.0.1:80")},
		{"bad target", bindingBody("blog.example.com", "a1", "just-a-name")},
		{"bad docker target", bindingBody("blog.example.com", "a1", "docker://")},
		{"unknown agent", bindingBody("blog.example.com", "ghost", "127.0.0.1:80")},
		{"broken json", "{not-json"},
	}
	for _, tc := range cases {
		if rec := postBinding(t, s, tc.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, want 400", tc.name, rec.Code)
		}
	}
}

func TestDomainBindingAPIMethodNotAllowed(t *testing.T) {
	s := newTestServerWithDB(t)

	for _, tc := range []struct{ method, path string }{
		{http.MethodPut, "/api/domains"},
		{http.MethodDelete, "/api/domains"},
		{http.MethodGet, "/api/domains/blog.example.com/apply"},
		{http.MethodPut, "/api/domains/blog.example.com"},
		{http.MethodPost, "/api/domains/blog.example.com"},
	} {
		_, req := doAuthenticatedRequest(s, tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		s.serveAPI(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: code = %d, want 405", tc.method, tc.path, rec.Code)
		}
	}
}

func TestDomainBindingAPIDelete(t *testing.T) {
	s := newTestServerWithDB(t)
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	postBinding(t, s, bindingBody("blog.example.com", "a1", "127.0.0.1:8080"))

	// 404
	_, req := doAuthenticatedRequest(s, http.MethodDelete, "/api/domains/none.example.com", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing code = %d", rec.Code)
	}

	// 成功
	_, req = doAuthenticatedRequest(s, http.MethodDelete, "/api/domains/blog.example.com", nil)
	rec = httptest.NewRecorder()
	s.serveAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete code = %d, body = %s", rec.Code, rec.Body)
	}
	if _, err := s.db.GetDomainBinding("blog.example.com"); err == nil {
		t.Error("binding should be deleted")
	}
}

func TestDomainBindingApplyDNS(t *testing.T) {
	s := newTestServerWithDB(t)
	fake := newBindingFakeDNS()
	s.dns = fake
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	postBinding(t, s, `{"domain":"blog.example.com","agentId":"a1","target":"127.0.0.1:8080","autoDns":true,"autoProxy":false,"autoCert":false}`)

	apply := func() (int, map[string]interface{}) {
		_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/domains/blog.example.com/apply", nil)
		rec := httptest.NewRecorder()
		s.serveAPI(rec, req)
		var out map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&out)
		return rec.Code, out
	}

	// 首次：建 A 记录
	code, out := apply()
	if code != http.StatusOK || out["status"] != "ok" {
		t.Fatalf("apply#1 code=%d out=%v", code, out)
	}
	rec := fake.findRecord("blog.example.com")
	if rec == nil || rec.Content != "203.0.113.10" {
		t.Fatalf("A record = %+v", rec)
	}

	// 幂等：再 apply 不报错
	if _, out := apply(); out["status"] != "ok" {
		t.Fatalf("apply#2 out = %v", out)
	}

	// agent IP 变了 → 更新记录
	seedBindingAgent(t, s, "a1", "203.0.113.99")
	if _, out := apply(); out["status"] != "ok" {
		t.Fatalf("apply#3 out = %v", out)
	}
	if rec := fake.findRecord("blog.example.com"); rec.Content != "203.0.113.99" {
		t.Fatalf("updated content = %q", rec.Content)
	}

	// apply 快照已回写
	b, _ := s.db.GetDomainBinding("blog.example.com")
	if b.LastApplyStatus != "ok" || b.AppliedAt == 0 {
		t.Fatalf("snapshot = %+v", b)
	}
}

func TestDomainBindingApplyDNSErrors(t *testing.T) {
	// provider 未配置
	s1 := newTestServerWithDB(t)
	seedBindingAgent(t, s1, "a1", "203.0.113.10")
	postBinding(t, s1, `{"domain":"blog.example.com","agentId":"a1","target":"127.0.0.1:80","autoDns":true,"autoProxy":false,"autoCert":false}`)
	_, req := doAuthenticatedRequest(s1, http.MethodPost, "/api/domains/blog.example.com/apply", nil)
	rec := httptest.NewRecorder()
	s1.serveAPI(rec, req)
	var out map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&out)
	if out["status"] != "failed" {
		t.Fatalf("no provider: out = %v", out)
	}

	// 域名不落任何 zone
	s := newTestServerWithDB(t)
	s.dns = newBindingFakeDNS()
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	postBinding(t, s, `{"domain":"blog.other.org","agentId":"a1","target":"127.0.0.1:80","autoDns":true,"autoProxy":false,"autoCert":false}`)
	_, req = doAuthenticatedRequest(s, http.MethodPost, "/api/domains/blog.other.org/apply", nil)
	rec = httptest.NewRecorder()
	s.serveAPI(rec, req)
	json.NewDecoder(rec.Body).Decode(&out)
	if out["status"] != "failed" {
		t.Fatalf("no zone: out = %v", out)
	}

	// agent 无主地址
	seedBindingAgent(t, s, "noip", "")
	postBinding(t, s, `{"domain":"nano.example.com","agentId":"noip","target":"127.0.0.1:80","autoDns":true,"autoProxy":false,"autoCert":false}`)
	_, req = doAuthenticatedRequest(s, http.MethodPost, "/api/domains/nano.example.com/apply", nil)
	rec = httptest.NewRecorder()
	s.serveAPI(rec, req)
	json.NewDecoder(rec.Body).Decode(&out)
	if out["status"] != "failed" {
		t.Fatalf("no ip: out = %v", out)
	}
}

func TestDomainBindingApplyProxy(t *testing.T) {
	s := newTestServerWithDB(t)
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	seen := withFakeProxyAgent(t, s, "a1", "traefik-proxy")

	postBinding(t, s, `{"domain":"blog.example.com","agentId":"a1","target":"docker://web:8080","autoDns":false,"autoProxy":true,"autoCert":false}`)
	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/domains/blog.example.com/apply", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)

	var out map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&out)
	if rec.Code != http.StatusOK || out["status"] != "ok" {
		t.Fatalf("code=%d out=%v", rec.Code, out)
	}
	// agent 收到 traefik.site.apply，upstream 已剥 docker:// 前缀
	found := false
	for _, m := range *seen {
		if m == "traefik.site.apply" {
			found = true
		}
	}
	if !found {
		t.Fatalf("agent methods = %v", *seen)
	}
}

func TestDomainBindingApplyProxyOffline(t *testing.T) {
	s := newTestServerWithDB(t)
	seedBindingAgent(t, s, "a1", "203.0.113.10") // 库里有、registry 不在线

	postBinding(t, s, `{"domain":"blog.example.com","agentId":"a1","target":"127.0.0.1:80","autoDns":false,"autoProxy":true,"autoCert":false}`)
	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/domains/blog.example.com/apply", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)

	var out map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&out)
	if out["status"] != "failed" {
		t.Fatalf("offline agent: out = %v", out)
	}
}

func TestDomainBindingApplyCert(t *testing.T) {
	s := newTestServerWithDB(t)
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	seedBindingAgent(t, s, "a2", "203.0.113.11")

	// 新建监控项
	postBinding(t, s, `{"domain":"blog.example.com","agentId":"a1","target":"127.0.0.1:80","autoDns":false,"autoProxy":false,"autoCert":true}`)
	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/domains/blog.example.com/apply", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)
	var out map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&out)
	if out["status"] != "ok" {
		t.Fatalf("cert create: out = %v", out)
	}
	d, err := s.db.GetDomainByName("blog.example.com")
	if err != nil || d == nil || d.AgentID == nil || *d.AgentID != "a1" {
		t.Fatalf("monitor entry = %+v, %v", d, err)
	}

	// 已存在但绑定别的 agent → apply 校准归属
	postBinding(t, s, `{"domain":"blog.example.com","agentId":"a2","target":"127.0.0.1:80","autoDns":false,"autoProxy":false,"autoCert":true}`)
	_, req = doAuthenticatedRequest(s, http.MethodPost, "/api/domains/blog.example.com/apply", nil)
	rec = httptest.NewRecorder()
	s.serveAPI(rec, req)
	d, _ = s.db.GetDomainByName("blog.example.com")
	if d.AgentID == nil || *d.AgentID != "a2" {
		t.Fatalf("rebind = %+v", d)
	}
}

func TestDomainBindingApplyEdgeCases(t *testing.T) {
	s := newTestServerWithDB(t)

	// 404：binding 不存在
	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/domains/none.example.com/apply", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("apply missing code = %d", rec.Code)
	}

	// disabled → 400
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	postBinding(t, s, `{"domain":"blog.example.com","agentId":"a1","target":"127.0.0.1:80","enabled":false}`)
	_, req = doAuthenticatedRequest(s, http.MethodPost, "/api/domains/blog.example.com/apply", nil)
	rec = httptest.NewRecorder()
	s.serveAPI(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("disabled code = %d, body = %s", rec.Code, rec.Body)
	}

	// 全 auto 关 → failed（no auto* switch）
	postBinding(t, s, `{"domain":"api.example.com","agentId":"a1","target":"127.0.0.1:80","autoDns":false,"autoProxy":false,"autoCert":false}`)
	_, req = doAuthenticatedRequest(s, http.MethodPost, "/api/domains/api.example.com/apply", nil)
	rec = httptest.NewRecorder()
	s.serveAPI(rec, req)
	var out map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&out)
	if out["status"] != "failed" {
		t.Fatalf("no switch out = %v", out)
	}
	b, _ := s.db.GetDomainBinding("api.example.com")
	if b.LastApplyStatus != "failed" {
		t.Fatalf("snapshot = %+v", b)
	}
}

func TestBindingSiteName(t *testing.T) {
	if got := bindingSiteName("blog.example.com"); got != "blog-example-com" {
		t.Errorf("bindingSiteName = %q", got)
	}
}

// withAgentResponder 注册带 capability 的假 agent，按 method 回自定义 RPC
// payload（withFakeProxyAgent 只会回 success，这里覆盖 error/畸形响应）
func withAgentResponder(t *testing.T, s *Server, agentID string, respond func(method string) map[string]interface{}) {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Hostname = "host-" + agentID
	agent.Capabilities = []protocol.Capability{{Type: "traefik-proxy"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	go func() {
		for reqMsg := range agent.Send {
			if reqMsg.Type != protocol.MessageTypeRPCRequest {
				continue
			}
			method, _ := reqMsg.Payload["method"].(string)
			resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, respond(method))
			resp.ID = reqMsg.ID
			s.handleRPCResponse(resp)
		}
	}()
	t.Cleanup(func() { agent.Close() })
}

// applyBinding 调 /apply 并解码 JSON 响应（错误分支复用）
func applyBinding(t *testing.T, s *Server, domain string) (int, map[string]interface{}) {
	t.Helper()
	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/domains/"+domain+"/apply", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)
	var out map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&out)
	return rec.Code, out
}

// applyStepErr 取 steps 里第一个失败步骤的 error
func applyStepErr(out map[string]interface{}) string {
	steps, _ := out["steps"].([]interface{})
	for _, st := range steps {
		m, _ := st.(map[string]interface{})
		if ok, _ := m["ok"].(bool); !ok {
			e, _ := m["error"].(string)
			return e
		}
	}
	return ""
}

func TestDomainBindingApplyDNSMoreErrors(t *testing.T) {
	// agent 在 apply 前被删
	s := newTestServerWithDB(t)
	s.dns = newBindingFakeDNS()
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	postBinding(t, s, `{"domain":"blog.example.com","agentId":"a1","target":"127.0.0.1:80","autoDns":true,"autoProxy":false,"autoCert":false}`)
	_ = s.db.DeleteAgent("a1")
	if _, out := applyBinding(t, s, "blog.example.com"); !strings.Contains(applyStepErr(out), "agent not found") {
		t.Errorf("deleted agent: out = %v", out)
	}

	// ListZones 失败 / ListRecords 失败 / Create 失败 / 翻页命中
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	fake := newBindingFakeDNS()
	fake.zonesErr = errors.New("zones down")
	s.dns = fake
	if _, out := applyBinding(t, s, "blog.example.com"); !strings.Contains(applyStepErr(out), "zone lookup failed") {
		t.Errorf("zones err: out = %v", out)
	}

	fake = newBindingFakeDNS()
	fake.listErr = errors.New("records down")
	s.dns = fake
	if _, out := applyBinding(t, s, "blog.example.com"); !strings.Contains(applyStepErr(out), "record lookup failed") {
		t.Errorf("list err: out = %v", out)
	}

	fake = newBindingFakeDNS()
	fake.createErr = errors.New("create boom")
	s.dns = fake
	if _, out := applyBinding(t, s, "blog.example.com"); !strings.Contains(applyStepErr(out), "create record failed") {
		t.Errorf("create err: out = %v", out)
	}

	fake = newBindingFakeDNS()
	fake.paginate = true
	fake.records = []dns.Record{{ID: "pre", Type: "A", Name: "blog.example.com", Content: "203.0.113.1"}}
	s.dns = fake
	if _, out := applyBinding(t, s, "blog.example.com"); out["status"] != "ok" {
		t.Errorf("paginate: out = %v", out)
	}

	// Update 失败：先正常建记录，再注入 updateErr + 换 IP 重 apply
	fake = newBindingFakeDNS()
	s.dns = fake
	if _, out := applyBinding(t, s, "blog.example.com"); out["status"] != "ok" {
		t.Fatalf("seed record: out = %v", out)
	}
	fake.updateErr = errors.New("update boom")
	seedBindingAgent(t, s, "a1", "203.0.113.99")
	if _, out := applyBinding(t, s, "blog.example.com"); !strings.Contains(applyStepErr(out), "update record failed") {
		t.Errorf("update err: out = %v", out)
	}
}

func TestDomainBindingApplyProxyErrors(t *testing.T) {
	// agent 回显式 error
	s := newTestServerWithDB(t)
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	withAgentResponder(t, s, "a1", func(method string) map[string]interface{} {
		return map[string]interface{}{"status": "error", "error": "boom"}
	})
	postBinding(t, s, `{"domain":"blog.example.com","agentId":"a1","target":"127.0.0.1:80","autoDns":false,"autoProxy":true,"autoCert":false}`)
	if _, out := applyBinding(t, s, "blog.example.com"); applyStepErr(out) != "boom" {
		t.Errorf("rpc error: out = %v", out)
	}

	// error 但无 error 字段 → 兜底文案
	s2 := newTestServerWithDB(t)
	seedBindingAgent(t, s2, "a1", "203.0.113.10")
	withAgentResponder(t, s2, "a1", func(method string) map[string]interface{} {
		return map[string]interface{}{"status": "error"}
	})
	postBinding(t, s2, `{"domain":"blog.example.com","agentId":"a1","target":"127.0.0.1:80","autoDns":false,"autoProxy":true,"autoCert":false}`)
	if _, out := applyBinding(t, s2, "blog.example.com"); applyStepErr(out) != "agent rejected the operation" {
		t.Errorf("empty error: out = %v", out)
	}

	// 畸形 payload（NaN 使 DecodeRPCResponse 失败）
	s3 := newTestServerWithDB(t)
	seedBindingAgent(t, s3, "a1", "203.0.113.10")
	withAgentResponder(t, s3, "a1", func(method string) map[string]interface{} {
		return map[string]interface{}{"status": math.NaN()}
	})
	postBinding(t, s3, `{"domain":"blog.example.com","agentId":"a1","target":"127.0.0.1:80","autoDns":false,"autoProxy":true,"autoCert":false}`)
	if _, out := applyBinding(t, s3, "blog.example.com"); applyStepErr(out) != "invalid agent response" {
		t.Errorf("bad payload: out = %v", out)
	}
}

func TestDomainBindingQueryOnlyDB(t *testing.T) {
	// 只读库覆盖「读成功→写失败」三个分支：
	// save 500 / delete 500 / cert 联动的 upsert 与 rebind 失败
	s := covPctQueryOnlyDB(t, func(db *storage.DB) {
		_ = db.UpsertAgent(&storage.Agent{ID: "a1", Hostname: "a1", IP: "203.0.113.10", Status: "online"})
		_ = db.SaveDomainBinding(&storage.DomainBinding{Domain: "blog.example.com", AgentID: "a1", Target: "127.0.0.1:80", Enabled: true, AutoDNS: false, AutoProxy: false, AutoCert: true})
		_ = db.SaveDomainBinding(&storage.DomainBinding{Domain: "api.example.com", AgentID: "a1", Target: "127.0.0.1:81", Enabled: true, AutoDNS: false, AutoProxy: false, AutoCert: true})
		// blog 已有监控项且属别的 agent → rebind 路径；api 无监控项 → 新建路径
		other := "a2"
		_ = db.UpsertDomain(&storage.Domain{ID: "blog.example.com", Domain: "blog.example.com", Status: "pending", AgentID: &other})
	})
	// 该基建只带 db；audit 缺失会 nil panic，补上（只读库上写失败被忽略）
	s.audit = audit.NewLogger(s.db)

	if rec := postBinding(t, s, bindingBody("blog.example.com", "a1", "127.0.0.1:8080")); rec.Code != http.StatusInternalServerError {
		t.Errorf("save on ro db: code = %d, want 500", rec.Code)
	}

	_, req := doAuthenticatedRequest(s, http.MethodDelete, "/api/domains/blog.example.com", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("delete on ro db: code = %d, want 500", rec.Code)
	}

	if _, out := applyBinding(t, s, "blog.example.com"); !strings.Contains(applyStepErr(out), "rebind existing domain failed") {
		t.Errorf("cert rebind on ro db: out = %v", out)
	}
	if _, out := applyBinding(t, s, "api.example.com"); !strings.Contains(applyStepErr(out), "create monitor entry failed") {
		t.Errorf("cert create on ro db: out = %v", out)
	}
}

func TestDomainBindingClosedDB(t *testing.T) {
	// closed db 覆盖「首个 DB 操作即失败」：列表 500（全量 + agent 过滤）
	s := newTestServerWithDB(t)
	s.db.Close() // t.Cleanup 里的二次 Close 返回错误，无副作用

	for _, q := range []string{"", "?agent=a1"} {
		_, req := doAuthenticatedRequest(s, http.MethodGet, "/api/domains"+q, nil)
		rec := httptest.NewRecorder()
		s.serveAPI(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("list%s on closed db: code = %d, want 500", q, rec.Code)
		}
	}
}

func TestDomainBindingAuditWithoutAuth(t *testing.T) {
	// 不带认证头的请求：serveAPI 不做鉴权（中间件在外层），handler 正常
	// 执行、audit 落 unknown 用户名分支
	s := newTestServerWithDB(t)
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	postBinding(t, s, bindingBody("blog.example.com", "a1", "127.0.0.1:80"))

	req := httptest.NewRequest(http.MethodDelete, "/api/domains/blog.example.com", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("no-auth delete code = %d, body = %s", rec.Code, rec.Body)
	}

	// 带 token 且经 auth.Middleware 注入用户上下文：audit 取真实用户名
	postBinding(t, s, bindingBody("api.example.com", "a1", "127.0.0.1:81"))
	_, areq := doAuthenticatedRequest(s, http.MethodDelete, "/api/domains/api.example.com", nil)
	rec = callWithAuth(s, s.serveAPI, areq)
	if rec.Code != http.StatusOK {
		t.Fatalf("authed delete code = %d, body = %s", rec.Code, rec.Body)
	}
}
