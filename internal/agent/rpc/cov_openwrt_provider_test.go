package rpc

// 覆盖率补充测试：openwrt_provider.go 全量 action 路由 / 成功 / 失败 / 参数校验。
//
// OpenWrtProvider 直接持有 openwrt.Client（HTTP ubus 客户端），无法接口注入，
// 故用 httptest TLS server 模拟 ubus：每次 call 先 login，再按 namespace.procedure
// 返回 Result.Data[0]。NewOpenWrtProvider 对非 80/0 端口固定走 https + InsecureTLS，
// 与 httptest.NewTLSServer 匹配。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

// covUbusServer 构造模拟 ubus 端点的 TLS server
func covUbusServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
		}
		_ = json.Unmarshal(body, &req)

		respond := func(data interface{}) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  map[string]interface{}{"data": []interface{}{data}},
			})
		}
		if len(req.Params) >= 3 {
			ns, _ := req.Params[1].(string)
			proc, _ := req.Params[2].(string)
			if ns == "session" && proc == "login" {
				respond(map[string]interface{}{"ubus_rpc_session": "cov-session"})
				return
			}
			switch ns + "." + proc {
			case "system.info":
				respond(map[string]interface{}{"uptime": 12345, "localtime": 1700000000})
			case "network.interface.dump":
				respond(map[string]interface{}{"interface": []interface{}{
					map[string]interface{}{"interface": "lan", "up": true, "enabled": true},
				}})
			case "network.interface.status":
				respond(map[string]interface{}{"interface": "lan", "up": true})
			case "network.route.dump":
				respond(map[string]interface{}{"route": []interface{}{
					map[string]interface{}{"target": "0.0.0.0", "mask": 0, "nexthop": "192.168.1.1"},
				}})
			case "firewall.get_zones":
				respond([]interface{}{map[string]interface{}{"name": "lan", "input": "ACCEPT"}})
			case "firewall.get_rules":
				respond([]interface{}{map[string]interface{}{"name": "rule1", "target": "ACCEPT"}})
			case "firewall.get_redirects":
				respond([]interface{}{map[string]interface{}{"name": "redir1", "target": "DNAT"}})
			case "network.wireless.status":
				respond([]interface{}{map[string]interface{}{
					"radios":     []interface{}{map[string]interface{}{"name": "radio0", "channel": 6}},
					"interfaces": []interface{}{map[string]interface{}{"ssid": "home"}},
				}})
			case "uci.get":
				respond(map[string]interface{}{"dhcp": []interface{}{}})
			case "file.read":
				respond(map[string]interface{}{"data": "file-content"})
			case "file.write", "system.reboot", "led.set":
				respond(map[string]interface{}{})
			case "led.get":
				respond(map[string]interface{}{"name": "led1", "state": "on"})
			default:
				respond(map[string]interface{}{})
			}
			return
		}
		respond(map[string]interface{}{})
	}))
	t.Cleanup(ts.Close)
	return ts
}

// covOpenWrtProvider 基于模拟 server 构造 provider
func covOpenWrtProvider(t *testing.T) *OpenWrtProvider {
	t.Helper()
	ts := covUbusServer(t)
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(u.Port())
	return NewOpenWrtProvider(u.Hostname(), port, "root", "pw")
}

func TestCovOpenWrtProviderAllActionsSuccess(t *testing.T) {
	p := covOpenWrtProvider(t)
	if p.Type() != "openwrt" {
		t.Fatalf("type = %q", p.Type())
	}

	actions := []struct {
		action string
		params map[string]interface{}
	}{
		{"system.info", nil},
		{"interfaces.list", nil},
		{"interfaces.get", map[string]interface{}{"name": "lan"}},
		{"routes.get", nil},
		{"firewall.zones", nil},
		{"firewall.rules", nil},
		{"firewall.redirects", nil},
		{"wireless.status", nil},
		{"dhcp.leases", nil},
		{"file.read", map[string]interface{}{"path": "/etc/config/network"}},
		{"file.write", map[string]interface{}{"path": "/tmp/x", "data": "d", "mode": "0600"}},
		{"reboot", nil},
		{"led.get", map[string]interface{}{"name": "led1"}},
		{"led.set", map[string]interface{}{"name": "led1", "state": "off"}},
	}
	for _, a := range actions {
		res, err := p.Call(a.action, a.params)
		if err != nil {
			t.Errorf("Call(%q) error = %v", a.action, err)
			continue
		}
		if res == nil {
			t.Errorf("Call(%q) result = nil", a.action)
		}
	}

	// file.write 缺省 mode 默认 0644 分支
	if _, err := p.WriteFile(map[string]interface{}{"path": "/tmp/y", "data": "z"}); err != nil {
		t.Errorf("write file default mode: %v", err)
	}
	// read 返回内容包一层 content
	res, err := p.ReadFile(map[string]interface{}{"path": "/etc/hosts"})
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if res.(map[string]interface{})["content"] != "file-content" {
		t.Errorf("content = %v", res)
	}
	// reboot 返回 rebooting
	res, err = p.Reboot(nil)
	if err != nil {
		t.Fatalf("reboot: %v", err)
	}
	if res.(map[string]interface{})["status"] != "rebooting" {
		t.Errorf("reboot = %v", res)
	}
}

func TestCovOpenWrtProviderAllActionsFailure(t *testing.T) {
	// server 恒 500 → login 失败 → 各方法报错
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())
	p := NewOpenWrtProvider(u.Hostname(), port, "root", "pw")

	actions := []string{
		"system.info", "interfaces.list", "interfaces.get", "routes.get",
		"firewall.zones", "firewall.rules", "firewall.redirects",
		"wireless.status", "dhcp.leases", "file.read", "file.write",
		"reboot", "led.get", "led.set",
	}
	params := map[string]map[string]interface{}{
		"interfaces.get": {"name": "lan"},
		"file.read":      {"path": "/x"},
		"file.write":     {"path": "/x", "data": "d"},
		"led.get":        {"name": "l"},
		"led.set":        {"name": "l", "state": "on"},
	}
	for _, action := range actions {
		if _, err := p.Call(action, params[action]); err == nil {
			t.Errorf("Call(%q) on 500 server should fail", action)
		}
	}
}

func TestCovOpenWrtProviderParamValidation(t *testing.T) {
	// 参数校验在发起请求之前，无需可达端点
	p := NewOpenWrtProvider("127.0.0.1", 1, "root", "pw")

	if _, err := p.Call("interfaces.get", map[string]interface{}{}); err == nil || err.Error() != "name required" {
		t.Errorf("get interface err = %v", err)
	}
	if _, err := p.Call("file.read", map[string]interface{}{}); err == nil || err.Error() != "path required" {
		t.Errorf("read file err = %v", err)
	}
	if _, err := p.Call("file.write", map[string]interface{}{"path": "/x"}); err == nil {
		t.Error("write file without data should fail")
	}
	if _, err := p.Call("led.get", map[string]interface{}{}); err == nil {
		t.Error("led get without name should fail")
	}
	if _, err := p.Call("led.set", map[string]interface{}{"name": "l"}); err == nil {
		t.Error("led set without state should fail")
	}
	// 未知 action
	if _, err := p.Call("bogus", nil); err == nil || err.Error() != "unknown openwrt action: bogus" {
		t.Errorf("unknown action err = %v", err)
	}
}
