package server

// api_dns_providers_test.go M2：/api/dns/status 回显真实 provider、
// requireDNS 按 provider 分流的 503 文案（D16）、DNSPod/阿里云 client
// 端到端穿 server handler、审计不回归（D6/D11）。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/dns"
	"github.com/cuihairu/cockpit/internal/storage"
)

// TestDNSStatusReflectsProvider status 回显真实 provider（未配置同样回显，
// 前端引导卡按 provider 分流，D16）；cfg 缺省兜底 cloudflare
func TestDNSStatusReflectsProvider(t *testing.T) {
	s := newBackupTestServer(t) // cfg 未注入 → cloudflare 兜底
	w := doDNS(s, http.MethodGet, "/dns/status", "")
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, `"configured":false`) ||
		!strings.Contains(body, `"provider":"cloudflare"`) {
		t.Fatalf("status fallback = %d %s", w.Code, body)
	}

	s.cfg = &config.Config{DNS: &config.DNSConfig{Provider: "dnspod"}}
	w = doDNS(s, http.MethodGet, "/dns/status", "")
	if !strings.Contains(w.Body.String(), `"provider":"dnspod"`) ||
		!strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatalf("status dnspod unconfigured = %s", w.Body.String())
	}

	s.dns = dns.NewDNSPodWithBase("42,tok", "http://127.0.0.1:1") // 构造期不发请求
	w = doDNS(s, http.MethodGet, "/dns/status", "")
	if !strings.Contains(w.Body.String(), `"configured":true`) {
		t.Fatalf("status dnspod configured = %s", w.Body.String())
	}
}

// TestDNS503MessagePerProvider 未配置时按 provider 各报各的配置键（D16）
func TestDNS503MessagePerProvider(t *testing.T) {
	cases := []struct{ provider, want string }{
		{"dnspod", "DNSPOD_LOGIN_TOKEN"},
		{"alidns", "ALIYUN_ACCESS_KEY"},
		{"cloudflare", "CLOUDFLARE_API_TOKEN"},
	}
	for _, c := range cases {
		s := newBackupTestServer(t)
		s.cfg = &config.Config{DNS: &config.DNSConfig{Provider: c.provider}}
		w := doDNS(s, http.MethodGet, "/dns/zones", "")
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), c.want) {
			t.Errorf("provider %s: = %d %s, want 503 with %s", c.provider, w.Code, w.Body.String(), c.want)
		}
	}

	// 未知 provider → 引导改 provider 键
	s := newBackupTestServer(t)
	s.cfg = &config.Config{DNS: &config.DNSConfig{Provider: "bogus"}}
	w := doDNS(s, http.MethodGet, "/dns/zones", "")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "unknown DNS provider") {
		t.Fatalf("bogus provider = %d %s", w.Code, w.Body.String())
	}
}

// newDNSPodAPIServer 独立 DNSPod 桩（server 测试不走 dns 包内部 helper）
func newDNSPodAPIServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/Domain.List":
			_, _ = w.Write([]byte(`{"status":{"code":"1","message":"ok"},"domains":[{"id":1,"name":"example.com"}]}`))
		case "/Record.Create":
			_, _ = w.Write([]byte(`{"status":{"code":"1","message":"ok"},"record":{"id":"9","name":"app","type":"A","value":"1.2.3.4"}}`))
		default:
			_, _ = w.Write([]byte(`{"status":{"code":"-15","message":"unexpected action"}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestDNSAPIViaDNSPod DNSPod client 端到端：zones 列表 + CMDB 标注 +
// 创建 + 审计落库（D6 审计语义对 provider 不变）
func TestDNSAPIViaDNSPod(t *testing.T) {
	s := newBackupTestServer(t)
	s.cfg = &config.Config{DNS: &config.DNSConfig{Provider: "dnspod"}}
	s.dns = dns.NewDNSPodWithBase("42,tok", newDNSPodAPIServer(t).URL)

	if err := s.db.UpsertDomain(&storage.Domain{Domain: "example.com", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	w := doDNS(s, http.MethodGet, "/dns/zones", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"example.com","status":"active","name_servers":null,"in_cmdb":true`) {
		t.Fatalf("zones = %d %s", w.Code, w.Body.String())
	}

	w = doDNS(s, http.MethodPost, "/dns/zones/example.com/records",
		`{"type":"A","name":"app.example.com","content":"1.2.3.4","ttl":0}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"9"`) {
		t.Fatalf("create = %d %s", w.Code, w.Body.String())
	}
	logs, _, err := s.db.GetAuditLogs(0, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if l.Action == "dns_create" && strings.Contains(l.ResourceID, "example.com") {
			found = true
		}
	}
	if !found {
		t.Fatalf("dns_create audit missing: %+v", logs)
	}
}

// TestDNSAPIViaAliDNS 阿里云 client 端到端：records 列表 MX 优先级拼回
// 穿 server、SRV 格式错 400（content must have 进 400 判定）
func TestDNSAPIViaAliDNS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("Action") {
		case "DescribeDomainRecords":
			_, _ = w.Write([]byte(`{"Records":{"TotalCount":1,"Record":[` +
				`{"RecordId":"1","Type":"MX","RR":"mail","Value":"mx1.example.com","TTL":600,"Priority":10}]}}`))
		case "AddDomainRecord":
			_, _ = w.Write([]byte(`{"RecordId":"10"}`))
		default:
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"Code":"Bad","Message":"unexpected"}`))
		}
	}))
	t.Cleanup(srv.Close)

	s := newBackupTestServer(t)
	s.cfg = &config.Config{DNS: &config.DNSConfig{Provider: "alidns"}}
	s.dns = dns.NewAliDNSWithBase("ak", "sk", srv.URL)

	w := doDNS(s, http.MethodGet, "/dns/zones/example.com/records", "")
	if w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), `"name":"mail.example.com","content":"10 mx1.example.com"`) {
		t.Fatalf("records = %d %s", w.Code, w.Body.String())
	}

	// SRV 段数不足 → 400（而非 502）
	w = doDNS(s, http.MethodPost, "/dns/zones/example.com/records",
		`{"type":"SRV","name":"_sip._tcp.example.com","content":"5 0"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "content must have") {
		t.Fatalf("srv bad content = %d %s, want 400", w.Code, w.Body.String())
	}

	// 合法创建
	w = doDNS(s, http.MethodPost, "/dns/zones/example.com/records",
		`{"type":"A","name":"app.example.com","content":"1.2.3.4","ttl":0}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"10"`) {
		t.Fatalf("create = %d %s", w.Code, w.Body.String())
	}
}
