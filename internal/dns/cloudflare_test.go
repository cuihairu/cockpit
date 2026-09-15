package dns

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newMockCloudflare 起一个记录请求的 mock API；handle 按路径分发响应
func newMockCloudflare(t *testing.T, handle func(r *http.Request) (int, string)) Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, body := handle(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewCloudflareWithBase("tok-test", srv.URL)
}

// cfList 常规列表响应体
func cfList(results string, page, totalPages int) string {
	return `{"success":true,"errors":[],"result":` + results +
		`,"result_info":{"page":` + itoa(page) + `,"per_page":50,"count":1,"total_count":1,"total_pages":` + itoa(totalPages) + `}}`
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestNewCloudflareEmptyToken(t *testing.T) {
	if p := NewCloudflare(""); p != nil {
		t.Error("empty token should yield nil provider")
	}
}

func TestCloudflareAuthHeader(t *testing.T) {
	var gotAuth string
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		gotAuth = r.Header.Get("Authorization")
		return 200, cfList("[]", 1, 1)
	})
	if _, err := p.ListZones(context.Background()); err != nil {
		t.Fatalf("ListZones error = %v", err)
	}
	if gotAuth != "Bearer tok-test" {
		t.Errorf("auth header = %q", gotAuth)
	}
}

func TestCloudflareListZones(t *testing.T) {
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		if r.URL.Path != "/zones" {
			t.Errorf("path = %q, want /zones", r.URL.Path)
		}
		if r.URL.Query().Get("per_page") != "50" {
			t.Errorf("per_page = %q", r.URL.Query().Get("per_page"))
		}
		return 200, cfList(`[{"id":"z1","name":"example.com","status":"active","name_servers":["a.ns.cf","b.ns.cf"]}]`, 1, 1)
	})
	zones, err := p.ListZones(context.Background())
	if err != nil {
		t.Fatalf("ListZones error = %v", err)
	}
	if len(zones) != 1 || zones[0].ID != "z1" || zones[0].Name != "example.com" || len(zones[0].NameServers) != 2 {
		t.Errorf("zones = %+v", zones)
	}
}

func TestCloudflareListRecords(t *testing.T) {
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		if r.URL.Path != "/zones/z1/dns_records" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("page") != "2" || q.Get("type") != "A" {
			t.Errorf("query = %v", q)
		}
		return 200, cfList(`[{"id":"r1","type":"A","name":"www.example.com","content":"1.2.3.4","ttl":1,"proxied":true}]`, 2, 3)
	})
	page, err := p.ListRecords(context.Background(), "z1", "a", 2)
	if err != nil {
		t.Fatalf("ListRecords error = %v", err)
	}
	if len(page.Records) != 1 || page.Records[0].Content != "1.2.3.4" || !page.Records[0].Proxied {
		t.Errorf("records = %+v", page.Records)
	}
	if page.Page != 2 || page.TotalPage != 3 {
		t.Errorf("page info = %+v", page)
	}
}

func TestCloudflareListRecordsDefaultsPage(t *testing.T) {
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		if r.URL.Query().Get("page") != "1" {
			t.Errorf("page = %q, want 1", r.URL.Query().Get("page"))
		}
		return 200, cfList("[]", 1, 1)
	})
	if _, err := p.ListRecords(context.Background(), "z1", "", 0); err != nil {
		t.Fatalf("ListRecords error = %v", err)
	}
}

func TestCloudflareCreateRecord(t *testing.T) {
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		if r.Method != http.MethodPost || r.URL.Path != "/zones/z1/dns_records" {
			t.Errorf("req = %s %s", r.Method, r.URL.Path)
		}
		var in RecordInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if in.Type != "CNAME" || in.Name != "app.example.com" || in.TTL != 1 {
			t.Errorf("body = %+v", in)
		}
		return 200, `{"success":true,"errors":[],"result":{"id":"r9","type":"CNAME","name":"app.example.com","content":"target.com","ttl":1,"proxied":false}}`
	})
	rec, err := p.CreateRecord(context.Background(), "z1", RecordInput{Type: "cname", Name: "app.example.com", Content: "target.com", TTL: 1})
	if err != nil {
		t.Fatalf("CreateRecord error = %v", err)
	}
	if rec.ID != "r9" {
		t.Errorf("rec = %+v", rec)
	}
}

func TestCloudflareUpdateRecord(t *testing.T) {
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		if r.Method != http.MethodPut || r.URL.Path != "/zones/z1/dns_records/r2" {
			t.Errorf("req = %s %s", r.Method, r.URL.Path)
		}
		return 200, `{"success":true,"errors":[],"result":{"id":"r2","type":"A","name":"www.example.com","content":"5.6.7.8","ttl":300,"proxied":false}}`
	})
	rec, err := p.UpdateRecord(context.Background(), "z1", "r2", RecordInput{Type: "A", Name: "www.example.com", Content: "5.6.7.8", TTL: 300})
	if err != nil {
		t.Fatalf("UpdateRecord error = %v", err)
	}
	if rec.Content != "5.6.7.8" || rec.TTL != 300 {
		t.Errorf("rec = %+v", rec)
	}
}

func TestCloudflareDeleteRecord(t *testing.T) {
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		if r.Method != http.MethodDelete || r.URL.Path != "/zones/z1/dns_records/r2" {
			t.Errorf("req = %s %s", r.Method, r.URL.Path)
		}
		return 200, `{"success":true,"errors":[],"result":{"id":"r2"}}`
	})
	if err := p.DeleteRecord(context.Background(), "z1", "r2"); err != nil {
		t.Fatalf("DeleteRecord error = %v", err)
	}
}

func TestCloudflareErrorUnwrap(t *testing.T) {
	// Cloudflare 习惯返回 200/4xx + success=false + errors 数组
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		return 400, `{"success":false,"errors":[{"code":1001,"message":"invalid token"}],"result":null}`
	})
	_, err := p.ListZones(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "1001") || !strings.Contains(err.Error(), "invalid token") {
		t.Errorf("error should carry cf code/message, got %v", err)
	}
	if strings.Contains(err.Error(), "tok-test") {
		t.Errorf("error must not leak token, got %v", err)
	}
}

func TestCloudflareHTTPErrorNoBody(t *testing.T) {
	// 非 JSON 响应（如网关 502 HTML）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>bad gateway</html>"))
	}))
	t.Cleanup(srv.Close)
	p := NewCloudflareWithBase("tok", srv.URL)
	if _, err := p.ListZones(context.Background()); err == nil {
		t.Fatal("non-JSON response should be error")
	}
}

func TestValidateInput(t *testing.T) {
	ok := RecordInput{Type: "a", Name: "www.example.com", Content: "1.2.3.4", TTL: 1}
	if err := ValidateInput(ok); err != nil {
		t.Errorf("valid input rejected: %v", err)
	}
	bad := []RecordInput{
		{Type: "PTR", Name: "x", Content: "y", TTL: 1},    // 类型白名单外
		{Type: "A", Name: " ", Content: "1.2.3.4", TTL: 1}, // name 空
		{Type: "A", Name: "x", Content: "", TTL: 1},        // content 空
		{Type: "A", Name: "x", Content: "1.2.3.4", TTL: 30}, // ttl 非法区间
		{Type: "A", Name: "x", Content: "1.2.3.4", TTL: -1}, // ttl 负
	}
	for _, in := range bad {
		if err := ValidateInput(in); err == nil {
			t.Errorf("invalid input accepted: %+v", in)
		}
	}
	// ttl=300 合法；ttl=0（未填）合法（auto）
	if err := ValidateInput(RecordInput{Type: "A", Name: "x", Content: "1.2.3.4", TTL: 300}); err != nil {
		t.Errorf("ttl=300 rejected: %v", err)
	}
	if err := ValidateInput(RecordInput{Type: "A", Name: "x", Content: "1.2.3.4"}); err != nil {
		t.Errorf("ttl=0 rejected: %v", err)
	}
}
