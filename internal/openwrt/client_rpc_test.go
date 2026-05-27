package openwrt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestOpenWrt creates a Client backed by httptest + call handler
func newTestOpenWrt(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	return &Client{
		endpoint: ts.URL + "/ubus",
		username: "root",
		password: "password",
		client:   ts.Client(),
	}
}

// loginResponse builds a login response that the Go client can parse
func loginResponse(session string) RPCResponse {
	return RPCResponse{
		Jsonrpc: "2.0",
		ID:      1,
		Result: RPCResult{
			Status: "ok",
			Data: []interface{}{
				map[string]interface{}{
					"ubus_rpc_session": session,
				},
			},
		},
	}
}

// callResponse builds a call response with data
func callResponse(data interface{}) RPCResponse {
	return RPCResponse{
		Jsonrpc: "2.0",
		ID:      1,
		Result: RPCResult{
			Status: "ok",
			Data:   []interface{}{data},
		},
	}
}

func TestLogin(t *testing.T) {
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(loginResponse("test-session-abc"))
	})

	sess, err := c.login()
	if err != nil {
		t.Fatalf("login() error = %v", err)
	}
	if sess != "test-session-abc" {
		t.Errorf("session = %v, want test-session-abc", sess)
	}
}

func TestLoginNoSession(t *testing.T) {
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(RPCResponse{
			Jsonrpc: "2.0",
			ID:      1,
			Result:  RPCResult{Status: "ok", Data: []interface{}{}},
		})
	})

	_, err := c.login()
	if err == nil {
		t.Error("login() should fail when session not in response")
	}
}

func TestLoginRPCError(t *testing.T) {
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(RPCResponse{
			Jsonrpc: "2.0",
			ID:      1,
			Error:   &RPCError{Code: -1, Message: "access denied"},
		})
	})

	_, err := c.login()
	if err == nil {
		t.Error("login() should fail on RPC error")
	}
}

func TestLoginHTTPError(t *testing.T) {
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.login()
	if err == nil {
		t.Error("login() should fail on HTTP 500")
	}
}

func TestRPCGetSystemInfo(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse(map[string]interface{}{
			"uptime":    12345,
			"load":      []float64{0.1, 0.2, 0.3},
			"localtime": 1700000000,
			"memory": map[string]interface{}{
				"total": 128 * 1024 * 1024,
				"free":  64 * 1024 * 1024,
			},
		}))
	})

	info, err := c.GetSystemInfo()
	if err != nil {
		t.Fatalf("GetSystemInfo() error = %v", err)
	}
	if info.Uptime != 12345 {
		t.Errorf("Uptime = %d, want 12345", info.Uptime)
	}
	if len(info.Load) != 3 {
		t.Errorf("Load length = %d, want 3", len(info.Load))
	}
	if info.Memory.Total != 128*1024*1024 {
		t.Errorf("Memory.Total = %d", info.Memory.Total)
	}
}

func TestRPCListInterfaces(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse(map[string]interface{}{
			"interface": []interface{}{
				map[string]interface{}{
					"interface": "lan",
					"up":        true,
					"l3_device": "br-lan",
				},
				map[string]interface{}{
					"interface": "wan",
					"up":        true,
					"l3_device": "eth0",
				},
			},
		}))
	})

	ifaces, err := c.ListInterfaces()
	if err != nil {
		t.Fatalf("ListInterfaces() error = %v", err)
	}
	if len(ifaces) != 2 {
		t.Fatalf("count = %d, want 2", len(ifaces))
	}
	if ifaces[0].Name != "lan" {
		t.Errorf("ifaces[0].Name = %v", ifaces[0].Name)
	}
}

func TestRPCGetInterface(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse(map[string]interface{}{
			"interface": "lan",
			"up":        true,
			"l3_device": "br-lan",
		}))
	})

	iface, err := c.GetInterface("lan")
	if err != nil {
		t.Fatalf("GetInterface() error = %v", err)
	}
	if iface.Name != "lan" {
		t.Errorf("Name = %v, want lan", iface.Name)
	}
}

func TestListRoutes(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse(map[string]interface{}{
			"route": []interface{}{
				map[string]interface{}{
					"target":  "0.0.0.0",
					"mask":    0,
					"nexthop": "192.168.1.1",
					"device":  "eth0",
				},
			},
		}))
	})

	routes, err := c.ListRoutes()
	if err != nil {
		t.Fatalf("ListRoutes() error = %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("count = %d, want 1", len(routes))
	}
	if routes[0].Gateway != "192.168.1.1" {
		t.Errorf("Gateway = %v", routes[0].Gateway)
	}
}

func TestGetFirewallZones(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse([]interface{}{
			map[string]interface{}{
				"name":   "lan",
				"input":  "ACCEPT",
				"output": "ACCEPT",
			},
		}))
	})

	zones, err := c.GetFirewallZones()
	if err != nil {
		t.Fatalf("GetFirewallZones() error = %v", err)
	}
	if len(zones) != 1 {
		t.Fatalf("count = %d, want 1", len(zones))
	}
	if zones[0].Name != "lan" {
		t.Errorf("Name = %v", zones[0].Name)
	}
}

func TestGetFirewallRules(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse([]interface{}{
			map[string]interface{}{
				"name":   "Allow-DHCP",
				"src":    "wan",
				"target": "ACCEPT",
			},
		}))
	})

	rules, err := c.GetFirewallRules()
	if err != nil {
		t.Fatalf("GetFirewallRules() error = %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("count = %d, want 1", len(rules))
	}
	if rules[0].Name != "Allow-DHCP" {
		t.Errorf("Name = %v", rules[0].Name)
	}
}

func TestGetFirewallRedirects(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse([]interface{}{
			map[string]interface{}{
				"name":      "redirect-1",
				"src_port":  "80",
				"dest_port": "8080",
			},
		}))
	})

	redirects, err := c.GetFirewallRedirects()
	if err != nil {
		t.Fatalf("GetFirewallRedirects() error = %v", err)
	}
	if len(redirects) != 1 {
		t.Fatalf("count = %d, want 1", len(redirects))
	}
}

func TestGetDHCPLoads(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse(map[string]interface{}{
			"dhcp": []interface{}{
				map[string]interface{}{
					"leasefile": "/tmp/dhcp.leases",
				},
			},
		}))
	})

	leases, err := c.GetDHCPLoads()
	if err != nil {
		t.Fatalf("GetDHCPLoads() error = %v", err)
	}
	// Returns empty list (file reading not implemented)
	if leases == nil {
		t.Error("leases should not be nil")
	}
}

func TestReadFile(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse(map[string]interface{}{
			"data": "file contents here",
		}))
	})

	content, err := c.ReadFile("/etc/config/network")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if content != "file contents here" {
		t.Errorf("content = %v", content)
	}
}

func TestWriteFile(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse(nil))
	})

	err := c.WriteFile("/tmp/test.txt", "hello", "0644")
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func TestReboot(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse(nil))
	})

	if err := c.Reboot(); err != nil {
		t.Fatalf("Reboot() error = %v", err)
	}
}

func TestGetLEDState(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse(map[string]interface{}{
			"name":  "led1",
			"state": "on",
		}))
	})

	state, err := c.GetLEDState("led1")
	if err != nil {
		t.Fatalf("GetLEDState() error = %v", err)
	}
	if state.Name != "led1" {
		t.Errorf("Name = %v", state.Name)
	}
	if state.State != "on" {
		t.Errorf("State = %v", state.State)
	}
}

func TestSetLEDState(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse(nil))
	})

	if err := c.SetLEDState("led1", "off"); err != nil {
		t.Fatalf("SetLEDState() error = %v", err)
	}
}

func TestCallRPCError(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(RPCResponse{
			Jsonrpc: "2.0",
			ID:      1,
			Error:   &RPCError{Code: -1, Message: "unknown namespace"},
		})
	})

	_, err := c.GetSystemInfo()
	if err == nil {
		t.Error("GetSystemInfo() should return error on RPC error")
	}
}

func TestCallHTTPError(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.GetSystemInfo()
	if err == nil {
		t.Error("GetSystemInfo() should return error on HTTP 500")
	}
}

func TestCallLoginFails(t *testing.T) {
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := c.GetSystemInfo()
	if err == nil {
		t.Error("GetSystemInfo() should fail when login fails")
	}
}

func TestCallInvalidJSON(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		w.Write([]byte("not json"))
	})

	_, err := c.GetSystemInfo()
	if err == nil {
		t.Error("GetSystemInfo() should fail on invalid JSON")
	}
}

func TestGetWirelessStatus(t *testing.T) {
	loginDone := false
	c := newTestOpenWrt(t, func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(loginResponse("sess1"))
			return
		}
		json.NewEncoder(w).Encode(callResponse([]interface{}{
			map[string]interface{}{
				"radios": []interface{}{
					map[string]interface{}{
						"name":      "radio0",
						"channel":   6,
						"frequency": 2437,
						"hwmode":    "11g",
						"htmode":    "HT20",
					},
				},
				"interfaces": []interface{}{
					map[string]interface{}{
						"ssid":       "MyNetwork",
						"encryption": "psk2",
						"disabled":   false,
					},
				},
			},
		}))
	})

	result, err := c.GetWirelessStatus()
	if err != nil {
		t.Fatalf("GetWirelessStatus() error = %v", err)
	}
	_ = result
}
