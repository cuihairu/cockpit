package dns

// cov_cloudflare_test.go 覆盖 cloudflare.go 的错误分支：
// do 的 marshal/建请求/网络错误、success=false 无 detail、
// 各接口的 unmarshal 错误与 normalizeInput 错误传播。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// covReadJSONBody 解码请求体（忽略错误，仅用于断言）
func covReadJSONBody(r *http.Request, v interface{}) {
	_ = json.NewDecoder(r.Body).Decode(v)
}

// covDeadProvider 指向已关闭服务的 provider：所有请求网络层失败
func covDeadProvider(t *testing.T) Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	return NewCloudflareWithBase("tok", srv.URL)
}

func TestCovNewCloudflareWithToken(t *testing.T) {
	if p := NewCloudflare("tok"); p == nil {
		t.Error("non-empty token should yield a provider")
	}
}

func TestCovDoMarshalError(t *testing.T) {
	p := covDeadProvider(t).(*cloudflareProvider)
	if _, err := p.do(context.Background(), http.MethodGet, "/", make(chan int)); err == nil {
		t.Fatal("unmarshalable body should error")
	} else if !strings.Contains(err.Error(), "marshal request") {
		t.Errorf("err = %v, want marshal request", err)
	}
}

func TestCovDoCreateRequestError(t *testing.T) {
	p := covDeadProvider(t).(*cloudflareProvider)
	// 非法 method 让 http.NewRequestWithContext 失败
	if _, err := p.do(context.Background(), "GE T", "/", nil); err == nil {
		t.Fatal("invalid method should error")
	} else if !strings.Contains(err.Error(), "create request") {
		t.Errorf("err = %v, want create request", err)
	}
}

func TestCovDoRequestNetworkError(t *testing.T) {
	p := covDeadProvider(t)
	if _, err := p.ListZones(context.Background()); err == nil {
		t.Fatal("dead server should error")
	}
	if _, err := p.ListRecords(context.Background(), "z1", "", 1); err == nil {
		t.Fatal("dead server should error (records)")
	}
}

func TestCovDoEnvelopeErrorNoDetail(t *testing.T) {
	// success=false 且 errors 为空：兜底 "cloudflare error (status N)"
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		return 403, `{"success":false,"errors":[]}`
	})
	_, err := p.ListZones(context.Background())
	if err == nil || !strings.Contains(err.Error(), "cloudflare error (status 403)") {
		t.Errorf("err = %v, want fallback envelope error", err)
	}
}

func TestCovListZonesUnmarshalError(t *testing.T) {
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		return 200, `{"success":true,"errors":[],"result":{}}`
	})
	if _, err := p.ListZones(context.Background()); err == nil {
		t.Fatal("object result should not unmarshal into []Zone")
	}
}

func TestCovListRecordsUnmarshalError(t *testing.T) {
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		return 200, `{"success":true,"errors":[],"result":"nope"}`
	})
	if _, err := p.ListRecords(context.Background(), "z1", "", 1); err == nil {
		t.Fatal("string result should not unmarshal into []Record")
	}
}

func TestCovCreateRecordInvalidInput(t *testing.T) {
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		t.Error("no request expected on invalid input")
		return 500, `{}`
	})
	if _, err := p.CreateRecord(context.Background(), "z1", RecordInput{Type: "PTR", Name: "x", Content: "y"}); err == nil {
		t.Fatal("invalid type should be rejected before request")
	}
}

func TestCovCreateRecordRequestError(t *testing.T) {
	p := covDeadProvider(t)
	if _, err := p.CreateRecord(context.Background(), "z1", RecordInput{Type: "A", Name: "x", Content: "1.2.3.4"}); err == nil {
		t.Fatal("dead server should error (create)")
	}
}

func TestCovCreateRecordUnmarshalError(t *testing.T) {
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		return 200, `{"success":true,"errors":[],"result":[1,2]}`
	})
	if _, err := p.CreateRecord(context.Background(), "z1", RecordInput{Type: "A", Name: "x", Content: "1.2.3.4"}); err == nil {
		t.Fatal("array result should not unmarshal into Record")
	}
}

func TestCovUpdateRecordInvalidInput(t *testing.T) {
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		t.Error("no request expected on invalid input")
		return 500, `{}`
	})
	if _, err := p.UpdateRecord(context.Background(), "z1", "r1", RecordInput{Type: "A", Name: "x", Content: "1.2.3.4", TTL: 30}); err == nil {
		t.Fatal("invalid ttl should be rejected before request")
	}
}

func TestCovUpdateRecordRequestError(t *testing.T) {
	p := covDeadProvider(t)
	if _, err := p.UpdateRecord(context.Background(), "z1", "r1", RecordInput{Type: "A", Name: "x", Content: "1.2.3.4"}); err == nil {
		t.Fatal("dead server should error (update)")
	}
}

func TestCovUpdateRecordUnmarshalError(t *testing.T) {
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		return 200, `{"success":true,"errors":[],"result":false}`
	})
	if _, err := p.UpdateRecord(context.Background(), "z1", "r1", RecordInput{Type: "A", Name: "x", Content: "1.2.3.4"}); err == nil {
		t.Fatal("bool result should not unmarshal into Record")
	}
}

func TestCovDeleteRecordRequestError(t *testing.T) {
	p := covDeadProvider(t)
	if err := p.DeleteRecord(context.Background(), "z1", "r1"); err == nil {
		t.Fatal("dead server should error (delete)")
	}
}

func TestCovNormalizeInputTTLZeroBecomesAuto(t *testing.T) {
	var gotTTL int
	p := newMockCloudflare(t, func(r *http.Request) (int, string) {
		var in RecordInput
		covReadJSONBody(r, &in)
		gotTTL = in.TTL
		return 200, `{"success":true,"errors":[],"result":{"id":"r1"}}`
	})
	if _, err := p.CreateRecord(context.Background(), "z1", RecordInput{Type: "A", Name: "x", Content: "1.2.3.4"}); err != nil {
		t.Fatalf("CreateRecord error = %v", err)
	}
	if gotTTL != 1 {
		t.Errorf("ttl=0 should be sent as 1 (auto), got %d", gotTTL)
	}
}
