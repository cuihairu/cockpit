package overlay

// 云客户端错误分支收口测试（D20）：UpstreamError 摘要格式化、请求构造/
// 传输/响应体读取失败、坏 JSON 解析、生产构造函数。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpstreamErrorFormatting(t *testing.T) {
	if got := (&UpstreamError{StatusCode: 500}).Error(); got != "overlay cloud api status 500" {
		t.Errorf("empty body error = %q", got)
	}
	long := strings.Repeat("x", 250)
	got := (&UpstreamError{StatusCode: 418, Body: long}).Error()
	if !strings.HasPrefix(got, "overlay cloud api status 418: ") || len(got) != len("overlay cloud api status 418: ")+200 {
		t.Errorf("long body should truncate to 200, got len %d", len(got))
	}
}

func TestNewZeroTierProduction(t *testing.T) {
	c := NewZeroTier("tok")
	if c.base != "https://api.zerotier.com/api/v1" || c.token != "tok" || c.http == nil {
		t.Fatalf("NewZeroTier = %+v", c)
	}
}

func TestDoRequestBadURL(t *testing.T) {
	if _, err := doRequest(context.Background(), http.DefaultClient, "GET", "http://exa mple.com", "t", nil); err == nil {
		t.Fatal("invalid URL should fail at request construction")
	}
}

func TestDoRequestTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // 端口已停 → 传输层失败
	c := NewZeroTierWithBase("tok", srv.URL)
	if _, err := c.Networks(context.Background()); err == nil {
		t.Fatal("dead upstream should error")
	}
}

func TestDoRequestBodyReadError(t *testing.T) {
	// Content-Length 声明 100 只写 5 字节 → 客户端读 body unexpected EOF
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	defer srv.Close()
	c := NewZeroTierWithBase("tok", srv.URL)
	if _, err := c.Networks(context.Background()); err == nil {
		t.Fatal("truncated body should error")
	}
}

func TestZeroTierBadJSON(t *testing.T) {
	var wantList, wantMembers string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/network":
			wantList = "hit"
			_, _ = w.Write([]byte("not-json"))
		case "/network/n1/member":
			wantMembers = "hit"
			_, _ = w.Write([]byte("[broken"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewZeroTierWithBase("tok", srv.URL)

	if _, err := c.Networks(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "invalid zerotier network list") {
		t.Errorf("Networks bad json err = %v", err)
	}
	if _, err := c.members(context.Background(), "n1"); err == nil ||
		!strings.Contains(err.Error(), "invalid zerotier member list") {
		t.Errorf("members bad json err = %v", err)
	}
	if wantList != "hit" || wantMembers != "hit" {
		t.Fatal("handlers not reached")
	}
}

func TestTailscaleDevicesBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()
	c := NewTailscaleWithBase("tok", "-", srv.URL)
	if _, err := c.Devices(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "invalid tailscale device list") {
		t.Errorf("Devices bad json err = %v", err)
	}
}
