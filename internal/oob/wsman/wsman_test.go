package wsman

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 假 WS-Management 端点：记录收到的请求供契约断言，按配置回 Identify 响应。
type fakeWsman struct {
	status  int    // /wsman 状态码（缺省 200）
	body    string // 响应原文
	lastReq *capturedReq
}

type capturedReq struct {
	method      string
	path        string
	contentType string
	body        string
}

const identifyResponse = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://www.w3.org/2005/08/addressing" xmlns:w="http://schemas.dmtf.org/wbem/wsman/identity/1/wsidentity">
  <s:Header><a:Action>http://schemas.dmtf.org/wbem/wsman/identity/1/wsidentityResponse</a:Action></s:Header>
  <s:Body>
    <w:IdentifyResponse>
      <w:ProductVendor>Intel Corporation</w:ProductVendor>
      <w:ProductName>Intel(r) Active Management Technology</w:ProductName>
      <w:ProductVersion>17.0.0.1</w:ProductVersion>
      <w:ProtocolVersion>1.0</w:ProtocolVersion>
    </w:IdentifyResponse>
  </s:Body>
</s:Envelope>`

func (f *fakeWsman) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.lastReq = &capturedReq{
			method:      r.Method,
			path:        r.URL.Path,
			contentType: r.Header.Get("Content-Type"),
			body:        string(body),
		}
		if r.URL.Path != "/wsman" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		status := f.status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		if f.body != "" {
			_, _ = w.Write([]byte(f.body))
		}
	}
}

func newServer(t *testing.T, f *fakeWsman) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(f.handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestIdentifyRequestContract(t *testing.T) {
	f := &fakeWsman{body: identifyResponse}
	ts := newServer(t, f)
	resp, status, err := New(Options{}).Identify(context.Background(), ts.URL)
	if err != nil || status != http.StatusOK {
		t.Fatalf("Identify() = %d, %v", status, err)
	}
	req := f.lastReq
	if req.method != http.MethodPost {
		t.Errorf("must POST, got %s", req.method)
	}
	if req.path != "/wsman" {
		t.Errorf("endpoint must be /wsman, got %s", req.path)
	}
	if !strings.HasPrefix(req.contentType, "application/soap+xml") {
		t.Errorf("content type must be SOAP 1.2, got %q", req.contentType)
	}
	if !strings.Contains(req.body, "wsidentity") {
		t.Errorf("body must carry wsidentity action: %q", req.body)
	}
	if resp.ProductName != "Intel(r) Active Management Technology" {
		t.Errorf("ProductName = %q", resp.ProductName)
	}
	if resp.ProductVendor != "Intel Corporation" || resp.ProductVersion != "17.0.0.1" || resp.ProtocolVersion != "1.0" {
		t.Errorf("IdentifyResponse fields = %+v", resp)
	}
}

func TestIdentifyNamespacedElementsParse(t *testing.T) {
	// 不同实现换命名空间前缀/URI：解码按本地名匹配，前缀漂移不炸。
	body := `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
	  <s:Body><x:IdentifyResponse xmlns:x="urn:any"><x:ProductName>AMD DASH CIM</x:ProductName></x:IdentifyResponse></s:Body>
	</s:Envelope>`
	f := &fakeWsman{body: body}
	ts := newServer(t, f)
	resp, _, err := New(Options{}).Identify(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("prefix drift must still parse: %v", err)
	}
	if resp.ProductName != "AMD DASH CIM" {
		t.Fatalf("ProductName = %q", resp.ProductName)
	}
}

func TestIdentifyRejectsNonIdentifyBody(t *testing.T) {
	// 200 但不是 IdentifyResponse 文档（空 Body / 无产品字段）：判为非 wsman 端点。
	for name, body := range map[string]string{
		"empty":       `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body/></s:Envelope>`,
		"not soap":    `<html>hello</html>`,
		"wrong shape": `{"product":"nope"}`,
	} {
		t.Run(name, func(t *testing.T) {
			ts := newServer(t, &fakeWsman{body: body})
			_, status, err := New(Options{}).Identify(context.Background(), ts.URL)
			if err == nil {
				t.Fatalf("non-identify body must fail: status=%d", status)
			}
			if !strings.Contains(err.Error(), "wsman") {
				t.Fatalf("error should name wsman: %v", err)
			}
		})
	}
}

func TestIdentifySurfacesHTTPStatus(t *testing.T) {
	ts := newServer(t, &fakeWsman{status: http.StatusNotFound, body: "nope"})
	_, status, err := New(Options{}).Identify(context.Background(), ts.URL)
	if err == nil || status != http.StatusNotFound {
		t.Fatalf("404 must surface: status=%d err=%v", status, err)
	}
}

func TestIdentifyTransportFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close()
	_, status, err := New(Options{}).Identify(context.Background(), ts.URL)
	if err == nil || status != 0 {
		t.Fatalf("transport failure must be status 0 + error: %d %v", status, err)
	}
}

func TestIdentifyHonorsContextCancel(t *testing.T) {
	ts := newServer(t, &fakeWsman{body: identifyResponse})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := New(Options{}).Identify(ctx, ts.URL)
	if err == nil {
		t.Fatal("ctx cancel must surface")
	}
}

func TestIdentifyRejectsMalformedURL(t *testing.T) {
	// 带控制字符的基址：请求构造层失败（status==0），上抛不吞。
	_, status, err := New(Options{}).Identify(context.Background(), "http://127.0.0.1\x7f:1")
	if err == nil || status != 0 {
		t.Fatalf("malformed URL must surface as transport-level error: %d %v", status, err)
	}
}

func TestIdentifyBodyReadFailure(t *testing.T) {
	// Content-Length 谎报（声明 100 实写 5）：客户端读体阶段 unexpected EOF。
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	defer ts.Close()
	_, status, err := New(Options{}).Identify(context.Background(), ts.URL)
	if err == nil || status != http.StatusOK {
		t.Fatalf("truncated body must surface at read stage: %d %v", status, err)
	}
}
