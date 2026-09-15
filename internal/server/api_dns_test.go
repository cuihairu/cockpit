package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/dns"
	"github.com/cuihairu/cockpit/internal/storage"
)

// newDNSTestServer 带 mock Cloudflare 的测试 server；handle 按请求分发响应
func newDNSTestServer(t *testing.T, handle func(r *http.Request) (int, string)) *Server {
	t.Helper()
	s := newBackupTestServer(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, body := handle(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	s.dns = dns.NewCloudflareWithBase("tok-test", srv.URL)
	return s
}

func doDNS(s *Server, method, path, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, path, reader)
	w := httptest.NewRecorder()
	s.handleDNS(w, r)
	return w
}

func TestDNSAPINotConfigured(t *testing.T) {
	s := newBackupTestServer(t) // s.dns == nil

	// status 只返回布尔，不报错（前端引导用）
	w := doDNS(s, http.MethodGet, "/dns/status", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	// 业务端点统一 503 + 引导文案（不含 token）
	w = doDNS(s, http.MethodGet, "/dns/zones", "")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "CLOUDFLARE_API_TOKEN") {
		t.Fatalf("zones = %d %s", w.Code, w.Body.String())
	}
	w = doDNS(s, http.MethodGet, "/dns/zones/z1/records", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("records = %d, want 503", w.Code)
	}
}

func TestDNSZonesInCMDB(t *testing.T) {
	s := newDNSTestServer(t, func(r *http.Request) (int, string) {
		return 200, `{"success":true,"errors":[],"result":[
			{"id":"z1","name":"example.com","status":"active","name_servers":["a.ns.cf"]},
			{"id":"z2","name":"other.com","status":"active","name_servers":[]}
		],"result_info":{"page":1,"total_pages":1}}`
	})
	// CMDB 已登记 example.com
	if err := s.db.UpsertDomain(&storage.Domain{Domain: "example.com", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	w := doDNS(s, http.MethodGet, "/dns/zones", "")
	if w.Code != http.StatusOK {
		t.Fatalf("zones = %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"name":"example.com","status":"active","name_servers":["a.ns.cf"],"in_cmdb":true`) {
		t.Errorf("example.com should be in_cmdb=true: %s", body)
	}
	if !strings.Contains(body, `"name":"other.com","status":"active","name_servers":[],"in_cmdb":false`) {
		t.Errorf("other.com should be in_cmdb=false: %s", body)
	}
}

func TestDNSRecordsCRUDAndAudit(t *testing.T) {
	var gotMethod, gotPath string
	s := newDNSTestServer(t, func(r *http.Request) (int, string) {
		gotMethod, gotPath = r.Method, r.URL.Path
		switch r.Method {
		case http.MethodGet:
			return 200, `{"success":true,"errors":[],"result":[
				{"id":"r1","type":"A","name":"www.example.com","content":"1.2.3.4","ttl":1,"proxied":true}
			],"result_info":{"page":1,"total_pages":2}}`
		case http.MethodPost:
			return 200, `{"success":true,"errors":[],"result":{"id":"r9","type":"CNAME","name":"app.example.com","content":"target.com","ttl":1,"proxied":false}}`
		case http.MethodPut:
			return 200, `{"success":true,"errors":[],"result":{"id":"r1","type":"A","name":"www.example.com","content":"5.6.7.8","ttl":300,"proxied":false}}`
		case http.MethodDelete:
			return 200, `{"success":true,"errors":[],"result":{"id":"r1"}}`
		}
		return 500, `{"success":false}`
	})

	// 列表（query 透传）
	w := doDNS(s, http.MethodGet, "/dns/zones/z1/records?type=A&page=2", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"total_pages":2`) {
		t.Fatalf("list = %d %s", w.Code, w.Body.String())
	}

	// 创建
	w = doDNS(s, http.MethodPost, "/dns/zones/z1/records",
		`{"type":"cname","name":"app.example.com","content":"target.com","ttl":0}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"r9"`) {
		t.Fatalf("create = %d %s", w.Code, w.Body.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/zones/z1/dns_records" {
		t.Errorf("create upstream = %s %s", gotMethod, gotPath)
	}

	// 更新
	w = doDNS(s, http.MethodPut, "/dns/zones/z1/records/r1",
		`{"type":"A","name":"www.example.com","content":"5.6.7.8","ttl":300}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"content":"5.6.7.8"`) {
		t.Fatalf("update = %d %s", w.Code, w.Body.String())
	}
	if gotPath != "/zones/z1/dns_records/r1" {
		t.Errorf("update upstream path = %s", gotPath)
	}

	// 删除
	w = doDNS(s, http.MethodDelete, "/dns/zones/z1/records/r1", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"deleted":true`) {
		t.Fatalf("delete = %d %s", w.Code, w.Body.String())
	}

	// 审计三条变更动作
	logs, _, err := s.db.GetAuditLogs(0, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	for _, l := range logs {
		actions[l.Action] = true
	}
	if !actions["dns_create"] || !actions["dns_update"] || !actions["dns_delete"] {
		t.Fatalf("audit actions = %v", actions)
	}
}

func TestDNSRecordsValidationAndUpstreamError(t *testing.T) {
	s := newDNSTestServer(t, func(r *http.Request) (int, string) {
		return 400, `{"success":false,"errors":[{"code":1001,"message":"invalid zone"}],"result":null}`
	})

	// 入参校验 400（白名单外类型）
	w := doDNS(s, http.MethodPost, "/dns/zones/z1/records",
		`{"type":"PTR","name":"x","content":"y","ttl":1}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad type = %d %s, want 400", w.Code, w.Body.String())
	}
	// 上游错误 502（Cloudflare 400 透传为 BadGateway）
	w = doDNS(s, http.MethodGet, "/dns/zones/z1/records", "")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "invalid zone") {
		t.Fatalf("upstream error = %d %s", w.Code, w.Body.String())
	}
	// 错误消息不含 token
	if strings.Contains(w.Body.String(), "tok-test") {
		t.Fatalf("error leaked token: %s", w.Body.String())
	}

	// 未知路径 404
	w = doDNS(s, http.MethodGet, "/dns/other", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown path = %d, want 404", w.Code)
	}
	// 非法方法 405
	w = doDNS(s, http.MethodPatch, "/dns/zones/z1/records", "")
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("patch = %d, want 405", w.Code)
	}
}
