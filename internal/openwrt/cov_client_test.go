package openwrt

// cov_client_test.go 覆盖 client.go 的错误分支：
// call 的 marshal/建请求/网络/读响应错误与空 result.data 时的原始 body 返回、
// login 的建请求/网络/读响应错误、各 getter 的 call 错误与 JSON 解析错误。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// covWrt 构造 login 成功、后续 call 由 callResp 应答的 client
func covWrt(t *testing.T, callResp func(w http.ResponseWriter)) *Client {
	t.Helper()
	return newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"login"`) {
			_ = json.NewEncoder(w).Encode(loginResponse("cov-session"))
			return
		}
		callResp(w)
	})
}

// covWrtCallError login 后 call 返回 500：覆盖各 getter 的 call 错误分支
func covWrtCallError(t *testing.T) *Client {
	return covWrt(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
	})
}

// covWrtBadData login 后 call 返回 data[0]=123：数值 body 无法解析为目标结构
func covWrtBadData(t *testing.T) *Client {
	return covWrt(t, func(w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(callResponse(123))
	})
}

func TestCovNewClientPortZeroUsesHTTP(t *testing.T) {
	c := NewClient(Config{Host: "gw.example.com", Port: 0})
	if c.endpoint != "http://gw.example.com/ubus" {
		t.Errorf("endpoint = %s, want http://gw.example.com/ubus", c.endpoint)
	}
	if c.timeout != 30*1e9 { // 30s
		t.Errorf("timeout = %v", c.timeout)
	}
}

func TestCovCallMarshalError(t *testing.T) {
	c := covWrt(t, func(w http.ResponseWriter) {
		t.Error("no call request expected when marshal fails")
	})
	_, err := c.call("system", "info", make(chan int))
	if err == nil || !strings.Contains(err.Error(), "marshal request") {
		t.Fatalf("err = %v, want marshal request", err)
	}
}

func TestCovCallDoErrorViaRedirect(t *testing.T) {
	// login 成功；call 收到 302 跳转到不可达端口 → client.Do 失败
	c := covWrt(t, func(w http.ResponseWriter) {
		w.Header().Set("Location", "http://127.0.0.1:1/ubus")
		w.WriteHeader(http.StatusFound)
	})
	_, err := c.call("system", "info")
	if err == nil || !strings.Contains(err.Error(), "do request") {
		t.Fatalf("err = %v, want do request error", err)
	}
}

func TestCovCallReadError(t *testing.T) {
	// call 响应声明 1000 字节只写 2 字节：io.ReadAll 报 unexpected EOF
	c := covWrt(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte(`{}`))
	})
	_, err := c.call("system", "info")
	if err == nil || !strings.Contains(err.Error(), "read response") {
		t.Fatalf("err = %v, want read response error", err)
	}
}

func TestCovCallEmptyDataReturnsRawBody(t *testing.T) {
	// result.data 为空时 call 返回原始 body 而非 data[0]
	c := covWrt(t, func(w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(RPCResponse{
			Jsonrpc: "2.0",
			ID:      1,
			Result:  RPCResult{Status: "ok"},
		})
	})
	body, err := c.call("system", "info")
	if err != nil {
		t.Fatalf("call error = %v", err)
	}
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Errorf("body = %s, want raw envelope", body)
	}
}

func TestCovLoginCreateRequestError(t *testing.T) {
	c := &Client{endpoint: "://missing-scheme/ubus", username: "u", password: "p", client: http.DefaultClient}
	if _, err := c.call("system", "info"); err == nil || !strings.Contains(err.Error(), "login") {
		t.Fatalf("err = %v, want login failure", err)
	}
}

func TestCovLoginDoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	c := &Client{endpoint: srv.URL + "/ubus", username: "u", password: "p", client: http.DefaultClient}
	if _, err := c.login(); err == nil || !strings.Contains(err.Error(), "do login request") {
		t.Fatalf("err = %v, want do login request error", err)
	}
}

func TestCovLoginReadError(t *testing.T) {
	// login 响应声明 1000 字节只写 2 字节：读 body 失败
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	c := &Client{endpoint: srv.URL + "/ubus", username: "u", password: "p", client: srv.Client()}
	if _, err := c.login(); err == nil || !strings.Contains(err.Error(), "read login response") {
		t.Fatalf("err = %v, want read login response error", err)
	}
}

func TestCovGetSystemInfoParseError(t *testing.T) {
	c := covWrtBadData(t)
	if _, err := c.GetSystemInfo(); err == nil || !strings.Contains(err.Error(), "parse system info") {
		t.Fatalf("err = %v, want parse system info error", err)
	}
}

func TestCovGetInterfaceCallError(t *testing.T) {
	c := covWrtCallError(t)
	if _, err := c.GetInterface("wan"); err == nil {
		t.Fatal("call error should propagate")
	}
}

func TestCovListRoutesCallAndParseErrors(t *testing.T) {
	if _, err := covWrtCallError(t).ListRoutes(); err == nil {
		t.Fatal("call error should propagate")
	}
	if _, err := covWrtBadData(t).ListRoutes(); err == nil || !strings.Contains(err.Error(), "parse routes") {
		t.Fatal("numeric body should fail route parsing")
	}
}

func TestCovGetFirewallZonesCallAndParseErrors(t *testing.T) {
	if _, err := covWrtCallError(t).GetFirewallZones(); err == nil {
		t.Fatal("call error should propagate")
	}
	if _, err := covWrtBadData(t).GetFirewallZones(); err == nil || !strings.Contains(err.Error(), "parse zones") {
		t.Fatal("numeric body should fail zone parsing")
	}
}

func TestCovGetFirewallRulesCallAndParseErrors(t *testing.T) {
	if _, err := covWrtCallError(t).GetFirewallRules(); err == nil {
		t.Fatal("call error should propagate")
	}
	if _, err := covWrtBadData(t).GetFirewallRules(); err == nil || !strings.Contains(err.Error(), "parse rules") {
		t.Fatal("numeric body should fail rule parsing")
	}
}

func TestCovGetFirewallRedirectsCallAndParseErrors(t *testing.T) {
	if _, err := covWrtCallError(t).GetFirewallRedirects(); err == nil {
		t.Fatal("call error should propagate")
	}
	if _, err := covWrtBadData(t).GetFirewallRedirects(); err == nil || !strings.Contains(err.Error(), "parse redirects") {
		t.Fatal("numeric body should fail redirect parsing")
	}
}

func TestCovGetWirelessStatusCallAndParseErrors(t *testing.T) {
	if _, err := covWrtCallError(t).GetWirelessStatus(); err == nil {
		t.Fatal("call error should propagate")
	}
	if _, err := covWrtBadData(t).GetWirelessStatus(); err == nil || !strings.Contains(err.Error(), "parse wireless status") {
		t.Fatal("numeric body should fail wireless parsing")
	}
}

func TestCovGetDHCPLoadsCallAndParseErrors(t *testing.T) {
	if _, err := covWrtCallError(t).GetDHCPLoads(); err == nil {
		t.Fatal("call error should propagate")
	}
	if _, err := covWrtBadData(t).GetDHCPLoads(); err == nil || !strings.Contains(err.Error(), "parse dhcp config") {
		t.Fatal("numeric body should fail dhcp parsing")
	}
}

func TestCovReadFileCallAndParseErrors(t *testing.T) {
	if _, err := covWrtCallError(t).ReadFile("/etc/config/network"); err == nil {
		t.Fatal("call error should propagate")
	}
	if _, err := covWrtBadData(t).ReadFile("/etc/config/network"); err == nil || !strings.Contains(err.Error(), "parse file data") {
		t.Fatal("numeric body should fail file data parsing")
	}
}

func TestCovGetLEDStateCallAndParseErrors(t *testing.T) {
	if _, err := covWrtCallError(t).GetLEDState("power"); err == nil {
		t.Fatal("call error should propagate")
	}
	if _, err := covWrtBadData(t).GetLEDState("power"); err == nil || !strings.Contains(err.Error(), "parse LED state") {
		t.Fatal("numeric body should fail LED state parsing")
	}
}
