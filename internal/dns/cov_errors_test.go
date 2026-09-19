package dns

// do() 传输层/解码层错误分支与记录操作参数校验的收口测试（覆盖率补齐）。
// 错误注入三件套：坏 base URL（请求构造失败）、已关闭的桩服务（传输失败）、
// Content-Length 与实际写入不符（响应体读取失败）。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func deadAliDNS(t *testing.T) *aliProvider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // 端口已停 → 传输层失败
	return NewAliDNSWithBase("ak-test", "sk-test", srv.URL).(*aliProvider)
}

func deadDNSPod(t *testing.T) *dnspodProvider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	return NewDNSPodWithBase("42,tok-test", srv.URL).(*dnspodProvider)
}

// TestAliDoTransportErrors 请求构造失败、传输失败、响应体读取失败三条路径
func TestAliDoTransportErrors(t *testing.T) {
	ctx := context.Background()

	bad := NewAliDNSWithBase("ak", "sk", "http://exa mple.com")
	if _, err := bad.ListZones(ctx); err == nil || !strings.Contains(err.Error(), "create request") {
		t.Errorf("create request err = %v", err)
	}

	dead := deadAliDNS(t)
	if _, err := dead.ListZones(ctx); err == nil || !strings.Contains(err.Error(), "alidns request") {
		t.Errorf("transport err = %v", err)
	}

	trunc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	defer trunc.Close()
	p := NewAliDNSWithBase("ak", "sk", trunc.URL)
	if _, err := p.ListZones(ctx); err == nil || !strings.Contains(err.Error(), "read alidns response") {
		t.Errorf("read err = %v", err)
	}
}

// TestAliDoNilParams do 的 params==nil 兜底（公共操作均传非 nil，直调覆盖）
func TestAliDoNilParams(t *testing.T) {
	dead := deadAliDNS(t)
	if _, err := dead.do(context.Background(), "Ping", nil); err == nil ||
		!strings.Contains(err.Error(), "alidns request") {
		t.Errorf("nil params err = %v", err)
	}
}

// TestAliDecodeErrors ListRecords/CreateRecord 的响应 JSON 解包失败
func TestAliDecodeErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Records":`))
	}))
	defer srv.Close()
	p := NewAliDNSWithBase("ak", "sk", srv.URL)
	ctx := context.Background()

	if _, err := p.ListRecords(ctx, "example.com", "", 1); err == nil ||
		!strings.Contains(err.Error(), "unmarshal records") {
		t.Errorf("ListRecords = %v", err)
	}
	if _, err := p.CreateRecord(ctx, "example.com", RecordInput{Name: "www", Type: "A", Content: "1.2.3.4"}); err == nil ||
		!strings.Contains(err.Error(), "unmarshal record") {
		t.Errorf("CreateRecord = %v", err)
	}
}

// TestAliRecordValidationAndPropagation 参数校验在发请求前挡下；do 错误原样上抛
func TestAliRecordValidationAndPropagation(t *testing.T) {
	dead := deadAliDNS(t)
	ctx := context.Background()
	aInput := RecordInput{Name: "www", Type: "A", Content: "1.2.3.4"}
	mxBad := RecordInput{Name: "@", Type: "MX", Content: "just-host"} // 缺优先级

	// normalizeInput（ValidateInput）挡下：非法类型
	if _, err := dead.CreateRecord(ctx, "example.com", RecordInput{Name: "www"}); err == nil ||
		!strings.Contains(err.Error(), "unsupported record type") {
		t.Errorf("CreateRecord normalize = %v", err)
	}
	if _, err := dead.UpdateRecord(ctx, "example.com", "1", RecordInput{Name: "www"}); err == nil ||
		!strings.Contains(err.Error(), "unsupported record type") {
		t.Errorf("UpdateRecord normalize = %v", err)
	}

	// aliRecordValue 挡下：MX 内容缺优先级
	if _, err := dead.CreateRecord(ctx, "example.com", mxBad); err == nil ||
		!strings.Contains(err.Error(), "content must have 2 fields") {
		t.Errorf("CreateRecord MX = %v", err)
	}
	if _, err := dead.UpdateRecord(ctx, "example.com", "1", mxBad); err == nil ||
		!strings.Contains(err.Error(), "content must have 2 fields") {
		t.Errorf("UpdateRecord MX = %v", err)
	}

	// do 错误原样上抛
	if _, err := dead.CreateRecord(ctx, "example.com", aInput); err == nil ||
		!strings.Contains(err.Error(), "alidns request") {
		t.Errorf("CreateRecord do = %v", err)
	}
	if _, err := dead.UpdateRecord(ctx, "example.com", "1", aInput); err == nil ||
		!strings.Contains(err.Error(), "alidns request") {
		t.Errorf("UpdateRecord do = %v", err)
	}
}

// TestAliListRecordsPaging page<1 归一 + 空集 TotalPage 收敛到 1
func TestAliListRecordsPaging(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Records":{"TotalCount":0,"Record":[]}}`))
	}))
	defer srv.Close()
	p := NewAliDNSWithBase("ak", "sk", srv.URL)
	out, err := p.ListRecords(context.Background(), "example.com", "A", 0)
	if err != nil {
		t.Fatal(err)
	}
	if out.Page != 1 || out.TotalPage != 1 {
		t.Fatalf("page/total = %d/%d, want 1/1", out.Page, out.TotalPage)
	}
}

// TestDNSPodDoErrors 请求构造失败、传输失败、响应体读取失败、decode 失败、
// form==nil 兜底，以及 Create/Update 对 do 错误的原样上抛
func TestDNSPodDoErrors(t *testing.T) {
	ctx := context.Background()

	bad := NewDNSPodWithBase("42,tok", "http://exa mple.com")
	if _, err := bad.ListZones(ctx); err == nil || !strings.Contains(err.Error(), "create request") {
		t.Errorf("create request err = %v", err)
	}

	dead := deadDNSPod(t)
	aInput := RecordInput{Name: "www", Type: "A", Content: "1.2.3.4"}
	if _, err := dead.ListZones(ctx); err == nil || !strings.Contains(err.Error(), "dnspod request") {
		t.Errorf("transport err = %v", err)
	}
	if _, err := dead.CreateRecord(ctx, "example.com", aInput); err == nil ||
		!strings.Contains(err.Error(), "dnspod request") {
		t.Errorf("CreateRecord do = %v", err)
	}
	if _, err := dead.UpdateRecord(ctx, "example.com", "7", aInput); err == nil ||
		!strings.Contains(err.Error(), "dnspod request") {
		t.Errorf("UpdateRecord do = %v", err)
	}

	trunc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	defer trunc.Close()
	p := NewDNSPodWithBase("42,tok", trunc.URL)
	if _, err := p.ListZones(ctx); err == nil || !strings.Contains(err.Error(), "read dnspod response") {
		t.Errorf("read err = %v", err)
	}

	notJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer notJSON.Close()
	np := NewDNSPodWithBase("42,tok", notJSON.URL).(*dnspodProvider)
	if _, err := np.ListZones(ctx); err == nil || !strings.Contains(err.Error(), "decode dnspod response") {
		t.Errorf("decode err = %v", err)
	}
	if _, err := np.do(ctx, "Ping", nil); err == nil || !strings.Contains(err.Error(), "decode dnspod response") {
		t.Errorf("nil form err = %v", err)
	}
}

// TestDNSPodDecodeErrors envelope 壳成功但业务字段形态非预期的三处二次解包
func TestDNSPodDecodeErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/") {
		case "Domain.List":
			_, _ = w.Write([]byte(`{"status":{"code":"1"},"domains":123}`))
		case "Record.List":
			_, _ = w.Write([]byte(`{"status":{"code":"1"},"records":123}`))
		case "Record.Create":
			_, _ = w.Write([]byte(`{"status":{"code":"1"},"record":123}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p := NewDNSPodWithBase("42,tok", srv.URL)
	ctx := context.Background()

	if _, err := p.ListZones(ctx); err == nil || !strings.Contains(err.Error(), "unmarshal domains") {
		t.Errorf("ListZones = %v", err)
	}
	if _, err := p.ListRecords(ctx, "example.com", "", 1); err == nil ||
		!strings.Contains(err.Error(), "unmarshal records") {
		t.Errorf("ListRecords = %v", err)
	}
	if _, err := p.CreateRecord(ctx, "example.com", RecordInput{Name: "www", Type: "A", Content: "1.2.3.4"}); err == nil ||
		!strings.Contains(err.Error(), "unmarshal record") {
		t.Errorf("CreateRecord = %v", err)
	}
}

// TestDNSPodValidationAndPaging 参数校验在发请求前挡下；page<1 归一、
// record_type 透传、空集 TotalPage 收敛到 1
func TestDNSPodValidationAndPaging(t *testing.T) {
	dead := deadDNSPod(t)
	ctx := context.Background()

	if _, err := dead.CreateRecord(ctx, "example.com", RecordInput{Name: "www"}); err == nil ||
		!strings.Contains(err.Error(), "unsupported record type") {
		t.Errorf("CreateRecord normalize = %v", err)
	}
	if _, err := dead.UpdateRecord(ctx, "example.com", "7", RecordInput{Name: "www"}); err == nil ||
		!strings.Contains(err.Error(), "unsupported record type") {
		t.Errorf("UpdateRecord normalize = %v", err)
	}
	if _, err := dead.UpdateRecord(ctx, "example.com", "7", RecordInput{Name: "@", Type: "MX", Content: "just-host"}); err == nil ||
		!strings.Contains(err.Error(), "content must have 2 fields") {
		t.Errorf("UpdateRecord MX = %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":{"code":"1"},"records":[],"info":{"record_total":0}}`))
	}))
	defer srv.Close()
	p := NewDNSPodWithBase("42,tok", srv.URL)
	out, err := p.ListRecords(ctx, "example.com", "A", 0)
	if err != nil {
		t.Fatal(err)
	}
	if out.Page != 1 || out.TotalPage != 1 {
		t.Fatalf("page/total = %d/%d, want 1/1", out.Page, out.TotalPage)
	}
}
