package dash

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/oob"
	"github.com/cuihairu/cockpit/internal/oob/wsman"
)

const dashIdentify = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:w="http://schemas.dmtf.org/wbem/wsman/identity/1/wsidentity">
  <s:Body>
    <w:IdentifyResponse>
      <w:ProductVendor>AMD</w:ProductVendor>
      <w:ProductName>AMD DASH CIM</w:ProductName>
      <w:ProductVersion>1.0.0.0</w:ProductVersion>
    </w:IdentifyResponse>
  </s:Body>
</s:Envelope>`

const amtIdentify = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:w="http://schemas.dmtf.org/wbem/wsman/identity/1/wsidentity">
  <s:Body>
    <w:IdentifyResponse>
      <w:ProductVendor>Intel Corporation</w:ProductVendor>
      <w:ProductName>Intel(r) Active Management Technology</w:ProductName>
    </w:IdentifyResponse>
  </s:Body>
</s:Envelope>`

func newWsmanServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestNormalizeBase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"192.168.1.5", "http://192.168.1.5:16992"},                 // 无 scheme 无 port → 默认明文 16992
		{"192.168.1.5:16993", "http://192.168.1.5:16993"},           // 显式 port 保留
		{"https://192.168.1.5", "https://192.168.1.5"},              // 有 scheme 不补 port
		{"https://192.168.1.5:16993/", "https://192.168.1.5:16993"}, // 尾斜杠去掉
	}
	for _, c := range cases {
		if got := normalizeBase(c.in); got != c.want {
			t.Errorf("normalizeBase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestProbeDetectsDASH(t *testing.T) {
	ts := newWsmanServer(t, dashIdentify, 0)
	p := New(Options{})
	info, err := p.Probe(context.Background(), oob.Target{Address: ts.URL})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if p.Name() != "dash" || info.Provider != "dash" {
		t.Fatalf("names: provider=%q info=%q", p.Name(), info.Provider)
	}
	if len(info.Capabilities) != 0 {
		// Identify 只证硬件在位与服务可达；启用态/逐能力面需认证 wsman，
		// 凭据随操作片落位——此处多报一个能力都是猜。
		t.Fatalf("identify-only probe must not claim capabilities: %+v", info.Capabilities)
	}
}

func TestProbeIntegratesWithRegistryMerge(t *testing.T) {
	ts := newWsmanServer(t, dashIdentify, 0)
	r := oob.NewRegistry()
	if err := r.Register(New(Options{})); err != nil {
		t.Fatal(err)
	}
	results := r.ProbeAll(context.Background(), oob.Target{Address: ts.URL})
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("ProbeAll: %+v", results)
	}
	view := oob.Merge(results)
	if len(view.Providers) != 1 || view.Providers[0] != "dash" {
		t.Fatalf("view providers: %v", view.Providers)
	}
}

func TestProbeRejectsNonDASHWsman(t *testing.T) {
	// wsman 端点在但不是 DASH（AMT/通用 WBEM 服务器）：provider 不匹配要报错，
	// 不许错认（错认 = 后续操作面打到错误协议上）。
	for name, body := range map[string]string{
		"amt":     amtIdentify,
		"generic": `<?xml version="1.0" encoding="UTF-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><w:IdentifyResponse xmlns:w="urn:x"><w:ProductName>Generic WBEM Server</w:ProductName></w:IdentifyResponse></s:Body></s:Envelope>`,
	} {
		t.Run(name, func(t *testing.T) {
			ts := newWsmanServer(t, body, 0)
			_, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
			if err == nil || !strings.Contains(err.Error(), "not DASH") {
				t.Fatalf("non-DASH wsman must be rejected: %v", err)
			}
		})
	}
}

func TestProbeAuthRequiredIsReachableButUndetermined(t *testing.T) {
	// 401：端点在但本会话无凭据（凭据面随操作片经 vault）——与 AMT/redfish 同口径，
	// 空能力集如实上账，不算探测错误。
	ts := newWsmanServer(t, "", http.StatusUnauthorized)
	info, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
	if err != nil {
		t.Fatalf("401 should not be a probe error: %v", err)
	}
	if info.Provider != "dash" || len(info.Capabilities) != 0 {
		t.Fatalf("undetermined probe: %+v", info)
	}
}

func TestProbeForbiddenIsReachableButUndetermined(t *testing.T) {
	// 403 与 401 同口径：端点可达但拒绝无凭据访问——可达未定，不算错误。
	ts := newWsmanServer(t, "", http.StatusForbidden)
	info, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
	if err != nil {
		t.Fatalf("403 should not be a probe error: %v", err)
	}
	if info.Provider != "dash" || len(info.Capabilities) != 0 {
		t.Fatalf("undetermined probe: %+v", info)
	}
}

func TestProbeRejectsNonWsmanEndpoint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	_, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
	if err == nil || !strings.Contains(err.Error(), "wsman") {
		t.Fatalf("non-wsman endpoint must error: %v", err)
	}
}

func TestProbeTransportFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close()
	_, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
	if err == nil {
		t.Fatal("transport failure must surface")
	}
	if !strings.Contains(err.Error(), "dash") {
		t.Fatalf("error should be wrapped with provider name: %v", err)
	}
}

func TestProbeHonorsContextCancel(t *testing.T) {
	ts := newWsmanServer(t, dashIdentify, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(Options{}).Probe(ctx, oob.Target{Address: ts.URL})
	if err == nil {
		t.Fatal("ctx cancel must surface")
	}
}

func TestProbeUsesDefaultPortWhenMissing(t *testing.T) {
	// 无 port 地址默认走 16992：对本地未监听 16992 的环境表现为传输失败（不挂死）。
	_, err := New(Options{}).Probe(context.Background(), oob.Target{Address: "127.0.0.1"})
	if err == nil {
		t.Fatal("closed default port must surface transport error")
	}
}

func TestSharedWsmanClientInjection(t *testing.T) {
	// wsman.Client 注入面：DASH Provider 默认自建，也可显式共享（测试/多 provider 复用）。
	ts := newWsmanServer(t, dashIdentify, 0)
	p := New(Options{Wsman: wsman.Options{}})
	info, err := p.Probe(context.Background(), oob.Target{Address: ts.URL})
	if err != nil || info.Provider != "dash" {
		t.Fatalf("shared client path: %v", err)
	}
}
