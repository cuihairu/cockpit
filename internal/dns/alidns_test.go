package dns

// alidns_test.go 阿里云 provider 的测试：RPC V1 签名纯函数表驱动 +
// 全操作 httptest（桩侧复算签名验证参数配套），M2 D13/D15。

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestAliPercentEncode 阿里云规则：unreserved 不转义、空格 %20、
// 特殊字符 !'()* 全编码且大写 HEX
func TestAliPercentEncode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc-_.~123", "abc-_.~123"},
		{"a b", "a%20b"},
		{"!", "%21"},
		{"'", "%27"},
		{"(", "%28"},
		{")", "%29"},
		{"*", "%2A"},
		{"2015-01-09T15:04:05Z", "2015-01-09T15%3A04%3A05Z"},
		{"a+b", "a%2Bb"},
	}
	for _, c := range cases {
		if got := aliPercentEncode(c.in); got != c.want {
			t.Errorf("aliPercentEncode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestAliCanonicalQuery 参数按名称排序后逐个编码
func TestAliCanonicalQuery(t *testing.T) {
	q := aliCanonicalQuery(url.Values{
		"Action":   {"DescribeDomains"},
		"PageSize": {"50"},
		"Format":   {"json"},
	})
	want := "Action=DescribeDomains&Format=json&PageSize=50"
	if q != want {
		t.Errorf("canonical = %q, want %q", q, want)
	}
	if got := aliCanonicalQuery(nil); got != "" {
		t.Errorf("nil params = %q", got)
	}
}

// TestAliSign 与独立实现的 HMAC-SHA1(secret+"&") 对拍
func TestAliSign(t *testing.T) {
	got := aliSign("GET&%2F&abc", "testsecret")
	mac := hmac.New(sha1.New, []byte("testsecret&"))
	mac.Write([]byte("GET&%2F&abc"))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Errorf("aliSign = %q, want %q", got, want)
	}
}

// TestAliNonce 唯一数非空且两次生成不同
func TestAliNonce(t *testing.T) {
	a, b := aliNonce(), aliNonce()
	if a == "" || a == b {
		t.Errorf("nonce should be non-empty and unique, got %q %q", a, b)
	}
}

// newMockAliDNS 起一个记录请求的 mock API；handle 按 action 分发响应，
// 桩侧对每次请求复算签名，验证 do() 组装的 query 参数与 Signature 配套
func newMockAliDNS(t *testing.T, handle func(action string, q url.Values) (int, string)) Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		params := url.Values{}
		for k, v := range q {
			if k != "Signature" {
				params[k] = v
			}
		}
		stringToSign := "GET&%2F&" + aliPercentEncode(aliCanonicalQuery(params))
		if want := aliSign(stringToSign, "sk-test"); q.Get("Signature") != want {
			t.Errorf("signature mismatch: got %q, want %q", q.Get("Signature"), want)
		}
		code, body := handle(q.Get("Action"), q)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewAliDNSWithBase("ak-test", "sk-test", srv.URL)
}

// requireAliCommon 公共参数断言（签名已由桩侧复算覆盖）
func requireAliCommon(t *testing.T, action string, q url.Values) {
	t.Helper()
	if q.Get("Action") != action {
		t.Errorf("Action = %q, want %q", q.Get("Action"), action)
	}
	for _, k := range []string{"Format", "Version", "AccessKeyId", "SignatureMethod", "SignatureVersion", "SignatureNonce", "Timestamp"} {
		if q.Get(k) == "" {
			t.Errorf("missing common param %s", k)
		}
	}
	if q.Get("AccessKeyId") != "ak-test" || q.Get("SignatureMethod") != "HMAC-SHA1" || q.Get("Version") != "2015-01-09" {
		t.Errorf("common param values wrong: %v", q)
	}
}

func TestNewAliDNSEmptyKeys(t *testing.T) {
	if NewAliDNS("", "sk") != nil || NewAliDNS("ak", "") != nil {
		t.Error("missing key should yield nil provider")
	}
}

// TestAliListZones DescribeDomains：域名即 zone id（D14）
func TestAliListZones(t *testing.T) {
	p := newMockAliDNS(t, func(action string, q url.Values) (int, string) {
		requireAliCommon(t, "DescribeDomains", q)
		if q.Get("PageSize") != "50" {
			t.Errorf("PageSize = %q", q.Get("PageSize"))
		}
		return 200, `{"Domains":{"TotalCount":2,"Domain":[{"DomainId":"id1","DomainName":"example.com"},{"DomainName":"test.cn"}]}}`
	})
	zones, err := p.ListZones(context.Background())
	if err != nil {
		t.Fatalf("ListZones: %v", err)
	}
	if len(zones) != 2 || zones[0].ID != "example.com" || zones[0].Name != "example.com" || zones[0].Status != "active" {
		t.Fatalf("zones = %+v", zones)
	}
}

// TestAliListRecords 分页、client 侧 type 过滤、RR 全名互转、Priority 拼回
func TestAliListRecords(t *testing.T) {
	p := newMockAliDNS(t, func(action string, q url.Values) (int, string) {
		requireAliCommon(t, "DescribeDomainRecords", q)
		if q.Get("DomainName") != "example.com" || q.Get("PageNumber") != "2" || q.Get("PageSize") != "50" {
			t.Errorf("query = %v", q)
		}
		return 200, `{"Records":{"TotalCount":120,"Record":[` +
			`{"RecordId":"1","Type":"A","RR":"@","Value":"1.2.3.4","TTL":600},` +
			`{"RecordId":"2","Type":"MX","RR":"mail","Value":"mx1.example.com","TTL":600,"Priority":10},` +
			`{"RecordId":"3","Type":"AAAA","RR":"v6","Value":"::1","TTL":600}]}}`
	})
	page, err := p.ListRecords(context.Background(), "example.com", "MX", 2)
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if page.Page != 2 || page.TotalPage != 3 { // ceil(120/50)，含被过滤记录
		t.Fatalf("page = %+v", page)
	}
	if len(page.Records) != 1 { // AAAA 被 client 侧过滤
		t.Fatalf("records = %+v", page.Records)
	}
	rec := page.Records[0]
	if rec.Name != "mail.example.com" || rec.Content != "10 mx1.example.com" {
		t.Fatalf("mx record = %+v", rec)
	}
}

// TestAliCreateRecord MX 拆装（Value 不带优先级 + Priority 参数）、
// RR 子域、TTL 省略与传递
func TestAliCreateRecord(t *testing.T) {
	var gotQuery url.Values
	p := newMockAliDNS(t, func(action string, q url.Values) (int, string) {
		requireAliCommon(t, "AddDomainRecord", q)
		gotQuery = q
		return 200, `{"RecordId":"100","RequestId":"r"}`
	})

	rec, err := p.CreateRecord(context.Background(), "example.com", RecordInput{
		Type: "mx", Name: "mail.example.com", Content: "10 mx1.example.com", TTL: 0,
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if gotQuery.Get("RR") != "mail" || gotQuery.Get("Type") != "MX" || gotQuery.Get("DomainName") != "example.com" {
		t.Fatalf("query = %v", gotQuery)
	}
	if gotQuery.Get("Value") != "mx1.example.com" || gotQuery.Get("Priority") != "10" {
		t.Fatalf("mx split wrong: %v", gotQuery)
	}
	if _, ok := gotQuery["TTL"]; ok {
		t.Errorf("TTL=1(auto) 应省略 TTL 参数")
	}
	if rec.ID != "100" || rec.Content != "10 mx1.example.com" || rec.TTL != 1 {
		t.Fatalf("record = %+v", rec)
	}

	if _, err := p.CreateRecord(context.Background(), "example.com", RecordInput{
		Type: "A", Name: "@", Content: "1.2.3.4", TTL: 600,
	}); err != nil {
		t.Fatalf("CreateRecord ttl: %v", err)
	}
	if gotQuery.Get("RR") != "@" || gotQuery.Get("TTL") != "600" || gotQuery.Get("Priority") != "" {
		t.Fatalf("a record query = %v", gotQuery)
	}
}

// TestAliCreateRecordSRV SRV：Priority + Value 三段（weight port target）
func TestAliCreateRecordSRV(t *testing.T) {
	var gotQuery url.Values
	p := newMockAliDNS(t, func(action string, q url.Values) (int, string) {
		requireAliCommon(t, "AddDomainRecord", q)
		gotQuery = q
		return 200, `{"RecordId":"101"}`
	})
	if _, err := p.CreateRecord(context.Background(), "example.com", RecordInput{
		Type: "SRV", Name: "_sip._tcp.example.com", Content: "5 0 5060 sip.example.com",
	}); err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if gotQuery.Get("Priority") != "5" || gotQuery.Get("Value") != "0 5060 sip.example.com" {
		t.Fatalf("srv split wrong: %v", gotQuery)
	}

	if _, err := p.CreateRecord(context.Background(), "example.com", RecordInput{
		Type: "SRV", Name: "@", Content: "5 0",
	}); err == nil || !strings.Contains(err.Error(), "content must have") {
		t.Fatalf("expect srv content error, got %v", err)
	}
}

// TestAliUpdateDelete UpdateDomainRecord 带 RecordId；DeleteDomainRecord 只带 id
func TestAliUpdateDelete(t *testing.T) {
	var gotAction string
	var gotQuery url.Values
	p := newMockAliDNS(t, func(action string, q url.Values) (int, string) {
		gotAction = action
		gotQuery = q
		return 200, `{"RequestId":"r"}`
	})
	rec, err := p.UpdateRecord(context.Background(), "example.com", "7", RecordInput{
		Type: "A", Name: "www", Content: "1.2.3.4", TTL: 600,
	})
	if err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}
	if gotAction != "UpdateDomainRecord" || gotQuery.Get("RecordId") != "7" || gotQuery.Get("RR") != "www" {
		t.Fatalf("update query = %s %v", gotAction, gotQuery)
	}
	if rec.ID != "7" || rec.TTL != 600 {
		t.Fatalf("record = %+v", rec)
	}

	if err := p.DeleteRecord(context.Background(), "example.com", "7"); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}
	if gotAction != "DeleteDomainRecord" || gotQuery.Get("RecordId") != "7" {
		t.Fatalf("delete query = %s %v", gotAction, gotQuery)
	}
}

// TestAliErrors RPC 错误归一：400 带 Code/Message；无 Code 兜底报文；
// 200 坏 JSON 报 unmarshal
func TestAliErrors(t *testing.T) {
	p := newMockAliDNS(t, func(action string, q url.Values) (int, string) {
		return 400, `{"RequestId":"r","Code":"InvalidDomainName.NoExist","Message":"The specified domain name does not exist."}`
	})
	if _, err := p.ListZones(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "alidns error InvalidDomainName.NoExist: The specified domain name does not exist.") {
		t.Fatalf("expect alidns error, got %v", err)
	}

	raw := newMockAliDNS(t, func(action string, q url.Values) (int, string) {
		return 403, `AccessDenied`
	})
	if _, err := raw.ListRecords(context.Background(), "example.com", "", 1); err == nil ||
		!strings.Contains(err.Error(), "alidns error: AccessDenied") {
		t.Fatalf("expect raw fallback error, got %v", err)
	}

	bad := newMockAliDNS(t, func(action string, q url.Values) (int, string) {
		return 200, `{"Domains":`
	})
	if _, err := bad.ListZones(context.Background()); err == nil || !strings.Contains(err.Error(), "unmarshal domains") {
		t.Fatalf("expect unmarshal error, got %v", err)
	}
}
