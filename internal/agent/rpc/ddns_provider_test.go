package rpc

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// withDDNSSources 临时替换回显服务源列表为测试地址
func withDDNSSources(t *testing.T, v4, v6 []string) {
	t.Helper()
	old4, old6 := ddnsIPv4Sources, ddnsIPv6Sources
	ddnsIPv4Sources, ddnsIPv6Sources = v4, v6
	t.Cleanup(func() { ddnsIPv4Sources, ddnsIPv6Sources = old4, old6 })
}

func TestDDNSProbeIPsBothFamilies(t *testing.T) {
	v4srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("203.0.113.7\n"))
	}))
	defer v4srv.Close()
	v6srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(" 2001:db8::1 "))
	}))
	defer v6srv.Close()
	withDDNSSources(t, []string{v4srv.URL}, []string{v6srv.URL})

	p := NewDDNSProvider(&http.Client{Timeout: time.Second})
	resp, err := p.Call("ip", nil)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	m := resp.(map[string]interface{})
	if m["ipv4"] != "203.0.113.7" {
		t.Errorf("ipv4 = %v, want 203.0.113.7（尾随换行应被 trim）", m["ipv4"])
	}
	if m["ipv6"] != "2001:db8::1" {
		t.Errorf("ipv6 = %v, want 2001:db8::1", m["ipv6"])
	}
}

func TestDDNSProbeFallbackOrder(t *testing.T) {
	// 首源 404，次源坏响应，三源成功 → 按序兜底
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			w.WriteHeader(http.StatusNotFound)
		case 2:
			_, _ = w.Write([]byte("not-an-ip"))
		default:
			_, _ = w.Write([]byte("198.51.100.9"))
		}
	}))
	defer srv.Close()
	// 同一 handler 复制为三个源，验证按序兜底（404 → 坏响应 → 成功）
	withDDNSSources(t, []string{srv.URL, srv.URL, srv.URL}, nil)

	p := NewDDNSProvider(&http.Client{Timeout: time.Second})
	resp, _ := p.Call("ip", nil)
	m := resp.(map[string]interface{})
	if m["ipv4"] != "198.51.100.9" {
		t.Errorf("ipv4 = %v, want 198.51.100.9", m["ipv4"])
	}
	if calls != 3 {
		t.Errorf("source calls = %d, want 3", calls)
	}
	if _, ok := m["ipv6"]; ok {
		t.Errorf("ipv6 should be omitted, got %v", m["ipv6"])
	}
}

func TestDDNSProbeFamilyMismatchRejected(t *testing.T) {
	// IPv4 源回显了 v6 地址 → 拒绝（回显服务配置错误的兜底，D12）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("2001:db8::1"))
	}))
	defer srv.Close()
	withDDNSSources(t, []string{srv.URL}, nil)

	p := NewDDNSProvider(&http.Client{Timeout: time.Second})
	resp, _ := p.Call("ip", nil)
	if _, ok := resp.(map[string]interface{})["ipv4"]; ok {
		t.Error("v4 源回显 v6 地址应被拒绝")
	}
}

func TestDDNSProbeAllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	withDDNSSources(t, []string{srv.URL}, []string{srv.URL})

	p := NewDDNSProvider(&http.Client{Timeout: time.Second})
	resp, err := p.Call("ip", nil)
	if err != nil {
		t.Fatalf("全源失败不应返回 error（D4）: %v", err)
	}
	m := resp.(map[string]interface{})
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestDDNSUnsupportedAction(t *testing.T) {
	p := NewDDNSProvider(nil)
	if _, err := p.Call("set", nil); err == nil {
		t.Fatal("不支持的动作应返回 error")
	}
	if p.Type() != "ddns" {
		t.Errorf("Type() = %q, want ddns", p.Type())
	}
}
