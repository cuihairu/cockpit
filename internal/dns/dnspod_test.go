package dns

// dnspod_test.go DNSPod provider 的全操作 httptest 测试：公共参数、
// 分页、子域↔全名互转、MX/SRV 拆装、错误码归一，M2 D12/D14/D15。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newMockDNSPod 起一个记录请求的 mock API；handle 按 action 分发响应
func newMockDNSPod(t *testing.T, handle func(action string, form url.Values) (int, string)) Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		code, body := handle(strings.TrimPrefix(r.URL.Path, "/"), r.PostForm)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewDNSPodWithBase("42,tok-test", srv.URL)
}

// dpOk 公共成功壳（extra 为业务数据，可空）
func dpOk(extra string) string {
	if extra != "" {
		extra = "," + extra
	}
	return `{"status":{"code":"1","message":"Action completed successful"}` + extra + `}`
}

func TestNewDNSPodEmptyToken(t *testing.T) {
	if p := NewDNSPod(""); p != nil {
		t.Error("empty token should yield nil provider")
	}
}

// TestDNSPodCommonParams 每个 action 都带公共参数与 POST form
func TestDNSPodCommonParams(t *testing.T) {
	var gotAction string
	var gotForm url.Values
	p := newMockDNSPod(t, func(action string, form url.Values) (int, string) {
		gotAction, gotForm = action, form
		return 200, dpOk(`"domains":[]`)
	})
	if _, err := p.ListZones(context.Background()); err != nil {
		t.Fatalf("ListZones: %v", err)
	}
	if gotAction != "Domain.List" {
		t.Errorf("action = %q", gotAction)
	}
	if got := gotForm.Get("login_token"); got != "42,tok-test" {
		t.Errorf("login_token = %q", got)
	}
	if got := gotForm.Get("format"); got != "json" {
		t.Errorf("format = %q", got)
	}
}

// TestDNSPodListZones 域名即 zone id（D14）
func TestDNSPodListZones(t *testing.T) {
	var gotForm url.Values
	p := newMockDNSPod(t, func(action string, form url.Values) (int, string) {
		gotForm = form
		return 200, dpOk(`"domains":[{"id":123,"name":"example.com","status":"enable"},{"id":456,"name":"test.cn"}]`)
	})
	zones, err := p.ListZones(context.Background())
	if err != nil {
		t.Fatalf("ListZones: %v", err)
	}
	if len(zones) != 2 || zones[0].ID != "example.com" || zones[0].Name != "example.com" || zones[0].Status != "active" {
		t.Fatalf("zones = %+v", zones)
	}
	if got := gotForm.Get("length"); got != "400" {
		t.Errorf("length = %q, want 400 (单页上限)", got)
	}
}

// TestDNSPodListRecords 分页 offset、record_type 透传、子域全名互转、
// MX 优先级拼回与 TotalPage 自算
func TestDNSPodListRecords(t *testing.T) {
	var gotForm url.Values
	p := newMockDNSPod(t, func(action string, form url.Values) (int, string) {
		gotForm = form
		return 200, dpOk(`"info":{"record_total":120},"records":[` +
			`{"id":"1","name":"@","type":"A","value":"1.2.3.4","ttl":600},` +
			`{"id":"2","name":"mail","type":"MX","value":"mx1.example.com","ttl":600,"mx":10},` +
			`{"id":"3","name":"*","type":"CNAME","value":"example.com","ttl":1}]`)
	})
	page, err := p.ListRecords(context.Background(), "example.com", "A", 2)
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if got := gotForm.Get("offset"); got != "50" {
		t.Errorf("offset = %q, want 50（第 2 页）", got)
	}
	if got := gotForm.Get("record_type"); got != "A" {
		t.Errorf("record_type = %q", got)
	}
	if page.Page != 2 || page.TotalPage != 3 { // ceil(120/50)
		t.Fatalf("page = %+v", page)
	}
	if len(page.Records) != 3 {
		t.Fatalf("records = %+v", page.Records)
	}
	r0, r1 := page.Records[0], page.Records[1]
	if r0.Name != "example.com" || r0.Content != "1.2.3.4" { // @ → 根域全名
		t.Errorf("root record = %+v", r0)
	}
	if r1.Name != "mail.example.com" || r1.Content != "10 mx1.example.com" { // MX 拼回
		t.Errorf("mx record = %+v", r1)
	}
}

// TestDNSPodCreateRecord MX 拆装、sub_domain、默认线路、TTL 省略与传递
func TestDNSPodCreateRecord(t *testing.T) {
	var gotForm url.Values
	p := newMockDNSPod(t, func(action string, form url.Values) (int, string) {
		gotForm = form
		return 200, dpOk(`"record":{"id":"9","name":"mail","type":"MX","value":"mx1.example.com","ttl":600,"mx":10}`)
	})

	rec, err := p.CreateRecord(context.Background(), "example.com", RecordInput{
		Type: "mx", Name: "mail.example.com", Content: "10 mx1.example.com", TTL: 0,
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if gotAction := gotForm.Get("sub_domain"); gotAction != "mail" {
		t.Errorf("sub_domain = %q", gotAction)
	}
	if got := gotForm.Get("record_type"); got != "MX" {
		t.Errorf("record_type = %q（type 大写归一）", got)
	}
	if got := gotForm.Get("record_line"); got != "默认" {
		t.Errorf("record_line = %q", got)
	}
	if got := gotForm.Get("mx"); got != "10" {
		t.Errorf("mx = %q", got)
	}
	if got := gotForm.Get("value"); got != "mx1.example.com" {
		t.Errorf("value = %q", got)
	}
	if _, ok := gotForm["ttl"]; ok {
		t.Errorf("TTL=1(auto) 应省略 ttl 参数，got %q", gotForm.Get("ttl"))
	}
	if rec.ID != "9" || rec.Content != "10 mx1.example.com" {
		t.Fatalf("record = %+v", rec)
	}

	// TTL > 1 传递
	if _, err := p.CreateRecord(context.Background(), "example.com", RecordInput{
		Type: "A", Name: "@", Content: "1.2.3.4", TTL: 600,
	}); err != nil {
		t.Fatalf("CreateRecord ttl: %v", err)
	}
	if got := gotForm.Get("ttl"); got != "600" {
		t.Errorf("ttl = %q", got)
	}
}

// TestDNSPodCreateRecordSRV SRV 四段拆分：mx=priority、value=weight port target
func TestDNSPodCreateRecordSRV(t *testing.T) {
	var gotForm url.Values
	p := newMockDNSPod(t, func(action string, form url.Values) (int, string) {
		gotForm = form
		return 200, dpOk(`"record":{"id":"10","name":"_sip._tcp","type":"SRV","value":"0 5060 sip.example.com","mx":5}`)
	})
	if _, err := p.CreateRecord(context.Background(), "example.com", RecordInput{
		Type: "SRV", Name: "_sip._tcp.example.com", Content: "5 0 5060 sip.example.com",
	}); err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if got := gotForm.Get("mx"); got != "5" {
		t.Errorf("mx = %q", got)
	}
	if got := gotForm.Get("value"); got != "0 5060 sip.example.com" {
		t.Errorf("value = %q", got)
	}
}

// TestDNSPodMXContentErrors MX/SRV 段数不足 → 校验错误（server 侧 400）
func TestDNSPodMXContentErrors(t *testing.T) {
	p := newMockDNSPod(t, func(action string, form url.Values) (int, string) { return 200, dpOk(`"record":{}`) })
	cases := []struct {
		recordType, content string
	}{
		{"MX", "10"},        // 缺目标
		{"SRV", "5 0 5060"}, // 缺 target
	}
	for _, c := range cases {
		if _, err := p.CreateRecord(context.Background(), "example.com", RecordInput{
			Type: c.recordType, Name: "@", Content: c.content,
		}); err == nil || !strings.Contains(err.Error(), "content must have") {
			t.Errorf("%s %q: expect content error, got %v", c.recordType, c.content, err)
		}
	}
}

// TestDNSPodUpdateDelete Modify 带 record_id；Remove 带域名与 id
func TestDNSPodUpdateDelete(t *testing.T) {
	var gotAction string
	var gotForm url.Values
	p := newMockDNSPod(t, func(action string, form url.Values) (int, string) {
		gotAction, gotForm = action, form
		return 200, dpOk("")
	})
	rec, err := p.UpdateRecord(context.Background(), "example.com", "7", RecordInput{
		Type: "A", Name: "www", Content: "1.2.3.4", TTL: 1,
	})
	if err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}
	if gotAction != "Record.Modify" || gotForm.Get("record_id") != "7" || gotForm.Get("sub_domain") != "www" {
		t.Fatalf("modify form = %s %v", gotAction, gotForm)
	}
	if rec.ID != "7" || rec.Name != "www" {
		t.Fatalf("record = %+v", rec)
	}

	if err := p.DeleteRecord(context.Background(), "example.com", "7"); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}
	if gotAction != "Record.Remove" || gotForm.Get("domain") != "example.com" || gotForm.Get("record_id") != "7" {
		t.Fatalf("remove form = %s %v", gotAction, gotForm)
	}
}

// TestDNSPodErrors status.code != "1" 归一；无 status 的网关响应兜底
func TestDNSPodErrors(t *testing.T) {
	p := newMockDNSPod(t, func(action string, form url.Values) (int, string) {
		return 200, `{"status":{"code":"3","message":"Incorrect request"}}`
	})
	if _, err := p.ListZones(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "dnspod error 3: Incorrect request") {
		t.Fatalf("expect dnspod error, got %v", err)
	}

	gateway := newMockDNSPod(t, func(action string, form url.Values) (int, string) {
		return 502, `{"error":"bad gateway"}` // 合法 JSON 但无 status → 兜底报文
	})
	if _, err := gateway.ListRecords(context.Background(), "example.com", "", 1); err == nil ||
		!strings.Contains(err.Error(), "dnspod error (status 502)") {
		t.Fatalf("expect gateway fallback error, got %v", err)
	}
}

// TestDNSNameMapping 子域↔全名互转表驱动（两家共用 helper，D14）
func TestDNSNameMapping(t *testing.T) {
	cases := []struct{ name, zone, sub, full string }{
		{"www.example.com", "example.com", "www", "www.example.com"},
		{"example.com", "example.com", "@", "example.com"},
		{"@", "example.com", "@", "example.com"},
		{"WWW.EXAMPLE.COM", "example.com", "www", "www.example.com"},
		{"example.com.", "example.com", "@", "example.com"},
		{"www", "example.com", "www", "www.example.com"}, // 裸子域输入兼容
		{"_sip._tcp.example.com", "example.com", "_sip._tcp", "_sip._tcp.example.com"},
	}
	for _, c := range cases {
		if got := dnsSubDomain(c.name, c.zone); got != c.sub {
			t.Errorf("dnsSubDomain(%q,%q) = %q, want %q", c.name, c.zone, got, c.sub)
		}
		if got := dnsFullName(c.sub, c.zone); got != c.full {
			t.Errorf("dnsFullName(%q,%q) = %q, want %q", c.sub, c.zone, got, c.full)
		}
	}
}

// TestDNSNewFactory 工厂按 provider 分流（D11）；凭据缺失/未知返回 nil
func TestDNSNewFactory(t *testing.T) {
	if New("", "tok", "", "", "") == nil {
		t.Error("empty provider should default to cloudflare")
	}
	if New("cloudflare", "tok", "", "", "") == nil {
		t.Error("cloudflare with token should be non-nil")
	}
	if New("cloudflare", "", "", "", "") != nil {
		t.Error("cloudflare without token should be nil")
	}
	if New("dnspod", "", "42,tok", "", "") == nil {
		t.Error("dnspod with token should be non-nil")
	}
	if New("dnspod", "", "", "", "") != nil {
		t.Error("dnspod without token should be nil")
	}
	if New("alidns", "", "", "ak", "sk") == nil {
		t.Error("alidns with keys should be non-nil")
	}
	if New("alidns", "", "", "ak", "") != nil {
		t.Error("alidns missing secret should be nil")
	}
	if New("bogus", "tok", "tok", "ak", "sk") != nil {
		t.Error("unknown provider should be nil")
	}
}
