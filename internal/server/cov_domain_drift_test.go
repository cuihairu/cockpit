package server

// cov_domain_drift_test.go D6 漂移检查覆盖：三路检查的三态分支
// （ok / missing|mismatch|foreign / error）与检查范围语义
// （enabled、auto* 开关、?agent= 过滤、路由 405、库故障 500）。

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/dns"
	"github.com/cuihairu/cockpit/internal/storage"
)

// errTestDNSDown DNS 桩的通用故障注入
var errTestDNSDown = errors.New("dns backend down")

// driftGet 调 /api/domains/drift 并解码 items
func driftGet(t *testing.T, s *Server, query string) (int, map[string]interface{}) {
	t.Helper()
	_, req := doAuthenticatedRequest(s, http.MethodGet, "/api/domains/drift"+query, nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)
	var out map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&out)
	return rec.Code, out
}

// driftItem 取 items[i] 的单路检查结果
func driftItem(out map[string]interface{}, i int, kind string) map[string]interface{} {
	items, _ := out["items"].([]interface{})
	if i >= len(items) {
		return nil
	}
	item, _ := items[i].(map[string]interface{})
	c, _ := item[kind].(map[string]interface{})
	return c
}

// siteGetResponder 构造 site.get 的 RPC 响应 payload
func siteGetResponder(payload map[string]interface{}) func(string) map[string]interface{} {
	return func(method string) map[string]interface{} {
		if method != "traefik.site.get" {
			return map[string]interface{}{"status": "error", "error": "unexpected method " + method}
		}
		return payload
	}
}

func TestDomainBindingDriftAllOK(t *testing.T) {
	s := newTestServerWithDB(t)
	s.dns = &fakeDNSProvider{
		zones:   []dns.Zone{{ID: "z1", Name: "example.com"}},
		records: []dns.Record{{ID: "r1", Type: "A", Name: "blog.example.com", Content: "203.0.113.10"}},
	}
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	aid := "a1"
	if err := s.db.UpsertDomain(&storage.Domain{ID: "blog.example.com", Domain: "blog.example.com", Status: "active", AgentID: &aid}); err != nil {
		t.Fatal(err)
	}
	withAgentResponder(t, s, "a1", siteGetResponder(map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"name": "blog-example-com",
			"site": map[string]interface{}{
				"upstream":    "127.0.0.1:8080",
				"serverNames": []interface{}{"blog.example.com"},
			},
		},
	}))

	if rec := postBinding(t, s, bindingBody("blog.example.com", "a1", "127.0.0.1:8080")); rec.Code != http.StatusOK {
		t.Fatalf("save code = %d, body = %s", rec.Code, rec.Body)
	}

	code, out := driftGet(t, s, "")
	if code != http.StatusOK {
		t.Fatalf("drift code = %d, body = %v", code, out)
	}
	for _, kind := range []string{"dns", "proxy", "cert"} {
		c := driftItem(out, 0, kind)
		if c == nil {
			t.Fatalf("missing %s check in %v", kind, out)
		}
		if c["checked"] != true || c["ok"] != true {
			t.Errorf("%s should be checked+ok, got %v", kind, c)
		}
		if _, has := c["status"]; has {
			t.Errorf("%s should have no drift status, got %v", kind, c)
		}
	}

	// ?agent= 过滤：a2 另有一条登记
	seedBindingAgent(t, s, "a2", "203.0.113.11")
	if rec := postBinding(t, s, bindingBody("api.example.com", "a2", "127.0.0.1:9090")); rec.Code != http.StatusOK {
		t.Fatalf("save a2 code = %d", rec.Code)
	}
	_, out = driftGet(t, s, "?agent=a1")
	items, _ := out["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("filtered items = %v", out)
	}
}

func TestDomainBindingDriftDNS(t *testing.T) {
	mkBinding := func(t *testing.T, s *Server) {
		t.Helper()
		seedBindingAgent(t, s, "a1", "203.0.113.10")
		body := `{"domain":"blog.example.com","agentId":"a1","target":"127.0.0.1:80","autoDns":true,"autoProxy":false,"autoCert":false}`
		if rec := postBinding(t, s, body); rec.Code != http.StatusOK {
			t.Fatalf("save code = %d, body = %s", rec.Code, rec.Body)
		}
	}
	dnsCheck := func(t *testing.T, out map[string]interface{}) map[string]interface{} {
		t.Helper()
		c := driftItem(out, 0, "dns")
		if c == nil {
			t.Fatalf("no dns check in %v", out)
		}
		if c["checked"] != true {
			t.Fatalf("dns should be checked: %v", c)
		}
		return c
	}

	// provider 未配置
	s := newTestServerWithDB(t)
	mkBinding(t, s)
	_, out := driftGet(t, s, "")
	if c := dnsCheck(t, out); c["ok"] != false || !strings.Contains(c["error"].(string), "DNS provider not configured") {
		t.Errorf("no provider: %v", c)
	}

	// agent 已删
	s = newTestServerWithDB(t)
	s.dns = newBindingFakeDNS()
	mkBinding(t, s)
	_ = s.db.DeleteAgent("a1")
	_, out = driftGet(t, s, "")
	if c := dnsCheck(t, out); !strings.Contains(c["error"].(string), "agent not found") {
		t.Errorf("deleted agent: %v", c)
	}

	// agent 无主地址
	s = newTestServerWithDB(t)
	s.dns = newBindingFakeDNS()
	seedBindingAgent(t, s, "a1", "203.0.113.10")
	if err := s.db.UpsertAgent(&storage.Agent{ID: "a2", Hostname: "a2"}); err != nil {
		t.Fatal(err)
	}
	if rec := postBinding(t, s, `{"domain":"blog.example.com","agentId":"a2","target":"127.0.0.1:80","autoDns":true,"autoProxy":false,"autoCert":false}`); rec.Code != http.StatusOK {
		t.Fatalf("save code = %d", rec.Code)
	}
	_, out = driftGet(t, s, "")
	if c := dnsCheck(t, out); !strings.Contains(c["error"].(string), "no registered address") {
		t.Errorf("no ip agent: %v", c)
	}

	// zone 查询失败 / 记录查询失败
	s = newTestServerWithDB(t)
	fake := newBindingFakeDNS()
	fake.zonesErr = errTestDNSDown
	s.dns = fake
	mkBinding(t, s)
	_, out = driftGet(t, s, "")
	if c := dnsCheck(t, out); !strings.Contains(c["error"].(string), "zone lookup failed") {
		t.Errorf("zones err: %v", c)
	}

	s = newTestServerWithDB(t)
	fake = newBindingFakeDNS()
	fake.listErr = errTestDNSDown
	s.dns = fake
	mkBinding(t, s)
	_, out = driftGet(t, s, "")
	if c := dnsCheck(t, out); !strings.Contains(c["error"].(string), "record lookup failed") {
		t.Errorf("list err: %v", c)
	}

	// 记录缺失 missing
	s = newTestServerWithDB(t)
	s.dns = newBindingFakeDNS()
	mkBinding(t, s)
	_, out = driftGet(t, s, "")
	c := dnsCheck(t, out)
	if c["status"] != "missing" || c["expected"] != "203.0.113.10" {
		t.Errorf("missing record: %v", c)
	}

	// 内容不符 mismatch
	s = newTestServerWithDB(t)
	s.dns = &fakeDNSProvider{
		zones:   []dns.Zone{{ID: "z1", Name: "example.com"}},
		records: []dns.Record{{ID: "r1", Type: "A", Name: "blog.example.com", Content: "192.0.2.99"}},
	}
	mkBinding(t, s)
	_, out = driftGet(t, s, "")
	c = dnsCheck(t, out)
	if c["status"] != "mismatch" || c["expected"] != "203.0.113.10" || c["actual"] != "192.0.2.99" {
		t.Errorf("mismatch record: %v", c)
	}

	// 一致 ok（AllOK 已覆盖主线，这里验证独立路径）
	s = newTestServerWithDB(t)
	s.dns = &fakeDNSProvider{
		zones:   []dns.Zone{{ID: "z1", Name: "example.com"}},
		records: []dns.Record{{ID: "r1", Type: "A", Name: "blog.example.com", Content: "203.0.113.10"}},
	}
	mkBinding(t, s)
	_, out = driftGet(t, s, "")
	if c := dnsCheck(t, out); c["ok"] != true {
		t.Errorf("matching record: %v", c)
	}
}

func TestDomainBindingDriftProxy(t *testing.T) {
	mkBinding := func(t *testing.T, s *Server, target string) {
		t.Helper()
		seedBindingAgent(t, s, "a1", "203.0.113.10")
		body := `{"domain":"blog.example.com","agentId":"a1","target":"` + target + `","autoDns":false,"autoProxy":true,"autoCert":false}`
		if rec := postBinding(t, s, body); rec.Code != http.StatusOK {
			t.Fatalf("save code = %d, body = %s", rec.Code, rec.Body)
		}
	}
	proxyCheck := func(t *testing.T, out map[string]interface{}) map[string]interface{} {
		t.Helper()
		c := driftItem(out, 0, "proxy")
		if c == nil {
			t.Fatalf("no proxy check in %v", out)
		}
		return c
	}
	siteOK := func(upstream string, names ...string) map[string]interface{} {
		list := make([]interface{}, len(names))
		for i, n := range names {
			list[i] = n
		}
		return map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"site": map[string]interface{}{"upstream": upstream, "serverNames": list},
			},
		}
	}

	// agent 不在线（库里有登记、registry 无连接）
	s := newTestServerWithDB(t)
	mkBinding(t, s, "127.0.0.1:8080")
	_, out := driftGet(t, s, "")
	if c := proxyCheck(t, out); !strings.Contains(c["error"].(string), "agent unreachable") {
		t.Errorf("offline agent: %v", c)
	}

	// 站点未下发 missing
	s = newTestServerWithDB(t)
	mkBinding(t, s, "127.0.0.1:8080")
	withAgentResponder(t, s, "a1", siteGetResponder(map[string]interface{}{
		"status": "error", "error": "site not found: blog-example-com",
	}))
	_, out = driftGet(t, s, "")
	if c := proxyCheck(t, out); c["status"] != "missing" {
		t.Errorf("site not found: %v", c)
	}

	// RPC 拒绝（非 not found）→ 检查失败
	s = newTestServerWithDB(t)
	mkBinding(t, s, "127.0.0.1:8080")
	withAgentResponder(t, s, "a1", siteGetResponder(map[string]interface{}{"status": "error", "error": "corrupted meta"}))
	_, out = driftGet(t, s, "")
	if c := proxyCheck(t, out); c["error"] != "corrupted meta" {
		t.Errorf("rpc error: %v", c)
	}

	// 空错误串兜底
	s = newTestServerWithDB(t)
	mkBinding(t, s, "127.0.0.1:8080")
	withAgentResponder(t, s, "a1", siteGetResponder(map[string]interface{}{"status": "error"}))
	_, out = driftGet(t, s, "")
	if c := proxyCheck(t, out); c["error"] != "agent rejected the operation" {
		t.Errorf("empty rpc error: %v", c)
	}

	// 畸形响应（status 非字符串 → 解码失败）
	s = newTestServerWithDB(t)
	mkBinding(t, s, "127.0.0.1:8080")
	withAgentResponder(t, s, "a1", siteGetResponder(map[string]interface{}{"status": 123}))
	_, out = driftGet(t, s, "")
	if c := proxyCheck(t, out); c["error"] != "invalid agent response" {
		t.Errorf("malformed response: %v", c)
	}

	// data 里没有 upstream
	s = newTestServerWithDB(t)
	mkBinding(t, s, "127.0.0.1:8080")
	withAgentResponder(t, s, "a1", siteGetResponder(map[string]interface{}{"status": "success", "data": map[string]interface{}{}}))
	_, out = driftGet(t, s, "")
	if c := proxyCheck(t, out); c["status"] != "mismatch" || c["actual"] != "(no upstream in site data)" {
		t.Errorf("no upstream: %v", c)
	}

	// upstream 不符
	s = newTestServerWithDB(t)
	mkBinding(t, s, "127.0.0.1:8080")
	withAgentResponder(t, s, "a1", siteGetResponder(siteOK("127.0.0.1:9999", "blog.example.com")))
	_, out = driftGet(t, s, "")
	if c := proxyCheck(t, out); c["status"] != "mismatch" || c["actual"] != "127.0.0.1:9999" || c["expected"] != "127.0.0.1:8080" {
		t.Errorf("upstream mismatch: %v", c)
	}

	// serverNames 不含域名
	s = newTestServerWithDB(t)
	mkBinding(t, s, "127.0.0.1:8080")
	withAgentResponder(t, s, "a1", siteGetResponder(siteOK("127.0.0.1:8080", "other.example.com")))
	_, out = driftGet(t, s, "")
	if c := proxyCheck(t, out); c["status"] != "mismatch" || !strings.Contains(c["actual"].(string), "serverNames missing") {
		t.Errorf("serverNames mismatch: %v", c)
	}

	// docker:// 归一后一致 ok
	s = newTestServerWithDB(t)
	mkBinding(t, s, "docker://web")
	withAgentResponder(t, s, "a1", siteGetResponder(siteOK("web", "blog.example.com")))
	_, out = driftGet(t, s, "")
	if c := proxyCheck(t, out); c["ok"] != true || c["expected"] != "web" {
		t.Errorf("docker target ok: %v", c)
	}
}

func TestDomainBindingDriftCert(t *testing.T) {
	mkBinding := func(t *testing.T, s *Server) {
		t.Helper()
		seedBindingAgent(t, s, "a1", "203.0.113.10")
		seedBindingAgent(t, s, "a2", "203.0.113.11")
		body := `{"domain":"blog.example.com","agentId":"a1","target":"127.0.0.1:80","autoDns":false,"autoProxy":false,"autoCert":true}`
		if rec := postBinding(t, s, body); rec.Code != http.StatusOK {
			t.Fatalf("save code = %d, body = %s", rec.Code, rec.Body)
		}
	}
	certCheck := func(t *testing.T, out map[string]interface{}) map[string]interface{} {
		t.Helper()
		c := driftItem(out, 0, "cert")
		if c == nil {
			t.Fatalf("no cert check in %v", out)
		}
		return c
	}

	// 监控缺失
	s := newTestServerWithDB(t)
	mkBinding(t, s)
	_, out := driftGet(t, s, "")
	if c := certCheck(t, out); c["status"] != "missing" || c["expected"] != "a1" {
		t.Errorf("missing cert: %v", c)
	}

	// 被其他 agent 占
	s = newTestServerWithDB(t)
	mkBinding(t, s)
	aid := "a2"
	if err := s.db.UpsertDomain(&storage.Domain{ID: "blog.example.com", Domain: "blog.example.com", Status: "active", AgentID: &aid}); err != nil {
		t.Fatal(err)
	}
	_, out = driftGet(t, s, "")
	if c := certCheck(t, out); c["status"] != "foreign" || c["actual"] != "a2" || c["expected"] != "a1" {
		t.Errorf("foreign cert: %v", c)
	}

	// 存在但未归属
	s = newTestServerWithDB(t)
	mkBinding(t, s)
	if err := s.db.UpsertDomain(&storage.Domain{ID: "blog.example.com", Domain: "blog.example.com", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	_, out = driftGet(t, s, "")
	if c := certCheck(t, out); c["status"] != "foreign" || c["actual"] != "(unassigned)" {
		t.Errorf("unassigned cert: %v", c)
	}

	// 归属正确 ok
	s = newTestServerWithDB(t)
	mkBinding(t, s)
	aid = "a1"
	if err := s.db.UpsertDomain(&storage.Domain{ID: "blog.example.com", Domain: "blog.example.com", Status: "active", AgentID: &aid}); err != nil {
		t.Fatal(err)
	}
	_, out = driftGet(t, s, "")
	if c := certCheck(t, out); c["ok"] != true {
		t.Errorf("ok cert: %v", c)
	}

	// 库故障（非 NotFound）→ error 而非误报 missing：函数级直调，
	// 绕过端点的 ListBindings 先行失败（端点级场景见 ClosedDB 用例）
	s = newTestServerWithDB(t)
	mkBinding(t, s)
	b, err := s.db.GetDomainBinding("blog.example.com")
	if err != nil {
		t.Fatal(err)
	}
	s.db.Close()
	c := s.checkBindingCert(b)
	if !c.Checked || c.OK || c.Status != "" || !strings.Contains(c.Error, "lookup failed") {
		t.Errorf("closed db cert check = %+v, want error", c)
	}
}

func TestServerNamesContain(t *testing.T) {
	if serverNamesContain("not-a-list", "d.example.com") {
		t.Error("non-slice should be false")
	}
	if serverNamesContain(nil, "d.example.com") {
		t.Error("nil should be false")
	}
	if serverNamesContain([]interface{}{123, true}, "d.example.com") {
		t.Error("non-string items should be false")
	}
	if !serverNamesContain([]interface{}{"other.com", "Blog.Example.Com"}, "blog.example.com") {
		t.Error("case-insensitive match should be true")
	}
}

func TestDomainBindingDriftClosedDB(t *testing.T) {
	s := newTestServerWithDB(t)
	s.db.Close()
	if code, _ := driftGet(t, s, ""); code != http.StatusInternalServerError {
		t.Errorf("closed db code = %d, want 500", code)
	}
}

func TestDomainBindingDriftScope(t *testing.T) {
	s := newTestServerWithDB(t)
	seedBindingAgent(t, s, "a1", "203.0.113.10")

	// disabled：整条跳过
	if rec := postBinding(t, s, `{"domain":"off.example.com","agentId":"a1","target":"127.0.0.1:80","enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("save disabled code = %d, body = %s", rec.Code, rec.Body)
	}
	// 三路 auto 全关：checked=false
	if rec := postBinding(t, s, `{"domain":"manual.example.com","agentId":"a1","target":"127.0.0.1:80","autoDns":false,"autoProxy":false,"autoCert":false}`); rec.Code != http.StatusOK {
		t.Fatalf("save manual code = %d", rec.Code)
	}

	_, out := driftGet(t, s, "")
	items, _ := out["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("items = %v", out)
	}
	byDomain := map[string]map[string]interface{}{}
	for _, raw := range items {
		m, _ := raw.(map[string]interface{})
		byDomain[m["domain"].(string)] = m
	}
	off := byDomain["off.example.com"] // disabled：整条跳过
	if off["enabled"] != false {
		t.Errorf("off item = %v", off)
	}
	for _, kind := range []string{"dns", "proxy", "cert"} {
		c, _ := off[kind].(map[string]interface{})
		if c["checked"] != false {
			t.Errorf("disabled binding %s should be unchecked: %v", kind, c)
		}
	}
	manual := byDomain["manual.example.com"] // auto 全关：checked=false
	if manual["enabled"] != true {
		t.Errorf("manual item = %v", manual)
	}
	for _, kind := range []string{"dns", "proxy", "cert"} {
		c, _ := manual[kind].(map[string]interface{})
		if c["checked"] != false {
			t.Errorf("auto-off %s should be unchecked: %v", kind, c)
		}
	}

	// 非 GET → 405
	_, req := doAuthenticatedRequest(s, http.MethodPost, "/api/domains/drift", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST drift code = %d, want 405", rec.Code)
	}
}
