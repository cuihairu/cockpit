package openwrt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestLoginSuccess 测试成功登录
func TestLoginSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle login request
		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Method != "call" {
			t.Errorf("expected method 'call', got '%s'", req.Method)
		}

		if len(req.Params) >= 3 && req.Params[1] == "session" && req.Params[2] == "login" {
			// Login response
			resp := RPCResponse{
				Jsonrpc: "2.0",
				ID:      1,
				Result: RPCResult{
					Data: []interface{}{
						map[string]interface{}{
							"ubus_rpc_session": "test-session-123",
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			// API call response (for other tests that use call())
			resp := RPCResponse{
				Jsonrpc: "2.0",
				ID:      1,
				Result:  RPCResult{Data: []interface{}{}},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	// Parse server URL to get host
	serverURL := server.URL
	if len(serverURL) > 7 {
		serverURL = serverURL[7:] // Remove "http://"
	}

	client := NewClient(Config{
		Host:     serverURL,
		Username: "root",
		Password: "password",
	})

	session, err := client.login()
	if err != nil {
		t.Fatalf("login() error = %v", err)
	}

	if session != "test-session-123" {
		t.Errorf("session = %s, want test-session-123", session)
	}
}

// TestLoginFailure 测试登录失败
func TestLoginFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			Jsonrpc: "2.0",
			ID:      1,
			Error: &RPCError{
				Code:    1,
				Message: "authentication failed",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		Host:     server.URL[7:],
		Port:     80,
		Username: "root",
		Password: "wrong",
	})

	_, err := client.login()
	if err == nil {
		t.Error("login() with wrong password should return error")
	}
}

// TestGetSystemInfo 测试获取系统信息
func TestGetSystemInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle login
		loginResp := RPCResponse{
			Jsonrpc: "2.0",
			ID:      1,
			Result: RPCResult{
				Data: []interface{}{
					map[string]interface{}{
						"ubus_rpc_session": "session-123",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(loginResp)
	}))

	client := NewClient(Config{
		Host:     server.URL[7:],
		Port:     80,
		Username: "root",
		Password: "password",
	})

	// Mock the call method by directly setting up a new server for the call
	callServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle login
		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Params) > 1 && req.Params[1] == "session" {
			loginResp := RPCResponse{
				Jsonrpc: "2.0",
				ID:      1,
				Result: RPCResult{
					Data: []interface{}{
						map[string]interface{}{
							"ubus_rpc_session": "session-123",
						},
					},
				},
			}
			json.NewEncoder(w).Encode(loginResp)
		} else {
			// System info response
			systemInfoResp := RPCResponse{
				Jsonrpc: "2.0",
				ID:      1,
				Result: RPCResult{
					Data: []interface{}{
						map[string]interface{}{
							"uptime":    86400,
							"load":      []float64{0.1, 0.2, 0.3},
							"localtime": 1234567890,
							"memory": map[string]interface{}{
								"total":     8000000000,
								"free":      4000000000,
								"available": 6000000000,
							},
							"swap": map[string]interface{}{
								"total": 0,
								"free":  0,
							},
						},
					},
				},
			}
			json.NewEncoder(w).Encode(systemInfoResp)
		}
	}))
	defer callServer.Close()

	client.endpoint = callServer.URL

	info, err := client.GetSystemInfo()
	if err != nil {
		t.Fatalf("GetSystemInfo() error = %v", err)
	}

	if info.Uptime != 86400 {
		t.Errorf("Uptime = %d, want 86400", info.Uptime)
	}

	if len(info.Load) != 3 {
		t.Errorf("Load length = %d, want 3", len(info.Load))
	}

	if info.Memory.Total != 8000000000 {
		t.Errorf("Memory.Total = %d, want 8000000000", info.Memory.Total)
	}
}

// TestListInterfaces 测试列出网络接口
func TestListInterfaces(t *testing.T) {
	callServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Params) > 1 && req.Params[1] == "session" {
			loginResp := RPCResponse{
				Jsonrpc: "2.0",
				ID:      1,
				Result: RPCResult{
					Data: []interface{}{
						map[string]interface{}{
							"ubus_rpc_session": "session-123",
						},
					},
				},
			}
			json.NewEncoder(w).Encode(loginResp)
		} else {
			// Interfaces response
			interfacesResp := RPCResponse{
				Jsonrpc: "2.0",
				ID:      1,
				Result: RPCResult{
					Data: []interface{}{
						map[string]interface{}{
							"interface": []interface{}{
								map[string]interface{}{
									"interface": "lan",
									"up":        true,
									"enabled":   true,
									"l3_device":  "br-lan",
									"metrics": map[string]interface{}{
										"rx_bytes":   1000000,
										"tx_bytes":   500000,
										"rx_packets": 10000,
										"tx_packets": 5000,
									},
									"ipv4-address": []interface{}{"192.168.1.1"},
								},
								map[string]interface{}{
									"interface": "wan",
									"up":        true,
									"enabled":   true,
									"l3_device":  "eth0",
									"metrics": map[string]interface{}{
										"rx_bytes":   5000000,
										"tx_bytes":   1000000,
										"rx_packets": 50000,
										"tx_packets": 10000,
									},
									"ipv4-address": []interface{}{"203.0.113.1"},
								},
							},
						},
					},
				},
			}
			json.NewEncoder(w).Encode(interfacesResp)
		}
	}))
	defer callServer.Close()

	client := NewClient(Config{
		Host:     "example.com",
		Port:     80,
		Username: "root",
		Password: "password",
	})
	client.endpoint = callServer.URL

	interfaces, err := client.ListInterfaces()
	if err != nil {
		t.Fatalf("ListInterfaces() error = %v", err)
	}

	if len(interfaces) != 2 {
		t.Errorf("len(interfaces) = %d, want 2", len(interfaces))
	}

	if interfaces[0].Name != "lan" {
		t.Errorf("interfaces[0].Name = %s, want lan", interfaces[0].Name)
	}

	if interfaces[0].Metrics.BytesReceived != 1000000 {
		t.Errorf("Metrics.BytesReceived = %d, want 1000000", interfaces[0].Metrics.BytesReceived)
	}
}

// TestGetInterface 测试获取单个接口信息
func TestGetInterface(t *testing.T) {
	callServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Params) > 1 && req.Params[1] == "session" {
			loginResp := RPCResponse{
				Jsonrpc: "2.0",
				ID:      1,
				Result: RPCResult{
					Data: []interface{}{
						map[string]interface{}{
							"ubus_rpc_session": "session-123",
						},
					},
				},
			}
			json.NewEncoder(w).Encode(loginResp)
		} else {
			// Interface status response
			interfaceResp := RPCResponse{
				Jsonrpc: "2.0",
				ID:      1,
				Result: RPCResult{
					Data: []interface{}{
						map[string]interface{}{
							"interface": "lan",
							"up":        true,
							"enabled":   true,
							"l3_device":  "br-lan",
							"metrics": map[string]interface{}{
								"rx_bytes": 1000000,
								"tx_bytes": 500000,
							},
							"ipv4-address": []interface{}{"192.168.1.1"},
							"dns-server":   []interface{}{"8.8.8.8", "8.8.4.4"},
						},
					},
				},
			}
			json.NewEncoder(w).Encode(interfaceResp)
		}
	}))
	defer callServer.Close()

	client := NewClient(Config{
		Host:     "example.com",
		Port:     80,
		Username: "root",
		Password: "password",
	})
	client.endpoint = callServer.URL

	iface, err := client.GetInterface("lan")
	if err != nil {
		t.Fatalf("GetInterface() error = %v", err)
	}

	if iface.Name != "lan" {
		t.Errorf("Name = %s, want lan", iface.Name)
	}

	if !iface.Up {
		t.Error("Up should be true")
	}

	if len(iface.DNS) != 2 {
		t.Errorf("DNS length = %d, want 2", len(iface.DNS))
	}
}

// TestHTTPError 测试HTTP错误处理
func TestHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "unauthorized"}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Host:     server.URL[7:],
		Port:     80,
		Username: "root",
		Password: "password",
	})

	_, err := client.login()
	if err == nil {
		t.Error("login() with HTTP error should return error")
	}
}

// TestRPCError 测试RPC错误处理
func TestRPCError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			Jsonrpc: "2.0",
			ID:      1,
			Error: &RPCError{
				Code:    6,
				Message: "permission denied",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		Host:     server.URL[7:],
		Port:     80,
		Username: "root",
		Password: "password",
	})

	_, err := client.login()
	if err == nil {
		t.Error("login() with RPC error should return error")
	}
}

// TestInvalidJSONResponse 测试无效JSON响应
func TestInvalidJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{invalid json}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Host:     server.URL[7:],
		Port:     80,
		Username: "root",
		Password: "password",
	})

	_, err := client.login()
	if err == nil {
		t.Error("login() with invalid JSON should return error")
	}
}

// TestEmptySession 测试空会话响应
func TestEmptySession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			Jsonrpc: "2.0",
			ID:      1,
			Result:  RPCResult{Data: []interface{}{}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		Host:     server.URL[7:],
		Port:     80,
		Username: "root",
		Password: "password",
	})

	_, err := client.login()
	if err == nil {
		t.Error("login() with empty session should return error")
	}
}

// TestClientTimeout 测试客户端超时
func TestClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		resp := RPCResponse{
			Jsonrpc: "2.0",
			ID:      1,
			Result: RPCResult{
				Data: []interface{}{
					map[string]interface{}{
						"ubus_rpc_session": "session-123",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		Host:     server.URL[7:],
		Port:     80,
		Username: "root",
		Password: "password",
		Timeout:  100 * time.Millisecond,
	})

	_, err := client.login()
	if err == nil {
		t.Error("login() with timeout should return error")
	}
}

// TestNewClientWithDefaults 测试使用默认值创建客户端
func TestNewClientWithDefaults(t *testing.T) {
	client := NewClient(Config{
		Host:     "192.168.1.1",
		Username: "root",
	})

	if client.endpoint != "http://192.168.1.1/ubus" {
		t.Errorf("endpoint = %s, want http://192.168.1.1/ubus", client.endpoint)
	}

	if client.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", client.timeout)
	}

	if client.client.Timeout != 30*time.Second {
		t.Errorf("HTTP client timeout = %v, want 30s", client.client.Timeout)
	}
}

// TestNewClientHTTPS 测试创建HTTPS客户端
func TestNewClientHTTPS(t *testing.T) {
	client := NewClient(Config{
		Host:     "192.168.1.1",
		Port:     443,
		Username: "root",
	})

	if client.endpoint != "https://192.168.1.1:443/ubus" {
		t.Errorf("endpoint = %s, want https://192.168.1.1:443/ubus", client.endpoint)
	}
}

// TestNewClientWithInsecureTLS 测试创建不安全TLS客户端
func TestNewClientWithInsecureTLS(t *testing.T) {
	client := NewClient(Config{
		Host:        "192.168.1.1",
		Port:        443,
		Username:    "root",
		InsecureTLS: true,
	})

	transport, ok := client.client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}

	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

// TestRPCErrorStruct 测试RPC错误结构
func TestRPCErrorStruct(t *testing.T) {
	err := &RPCError{
		Code:    6,
		Message: "permission denied",
	}

	if err.Code != 6 {
		t.Errorf("Code = %d, want 6", err.Code)
	}

	if err.Message != "permission denied" {
		t.Errorf("Message = %s, want permission denied", err.Message)
	}
}

// TestSystemInfoStruct 测试系统信息结构
func TestSystemInfoStruct(t *testing.T) {
	info := SystemInfo{
		Uptime:    86400,
		Load:      []float64{0.1, 0.2, 0.3},
		Localtime: 1234567890,
		Memory: MemoryInfo{
			Total:     8000000000,
			Free:      4000000000,
			Available: 6000000000,
		},
	}

	if info.Uptime != 86400 {
		t.Errorf("Uptime = %d, want 86400", info.Uptime)
	}

	if len(info.Load) != 3 {
		t.Errorf("Load length = %d, want 3", len(info.Load))
	}

	if info.Memory.Total != 8000000000 {
		t.Errorf("Memory.Total = %d, want 8000000000", info.Memory.Total)
	}
}

// TestMemoryInfoStruct 测试内存信息结构
func TestMemoryInfoStruct(t *testing.T) {
	mem := MemoryInfo{
		Total:     8000000000,
		Free:      4000000000,
		Available: 6000000000,
		Buffered:  1000000000,
		Cached:    2000000000,
	}

	if mem.Total != 8000000000 {
		t.Errorf("Total = %d, want 8000000000", mem.Total)
	}

	if mem.Free != 4000000000 {
		t.Errorf("Free = %d, want 4000000000", mem.Free)
	}
}

// TestInterfaceStruct 测试接口结构
func TestInterfaceStruct(t *testing.T) {
	iface := Interface{
		Name:      "lan",
		Up:        true,
		Enabled:   true,
		Device:    "br-lan",
		Autostart: true,
		Metrics: Metrics{
			BytesReceived:   1000000,
			BytesSent:       500000,
			PacketsReceived: 10000,
			PacketsSent:     5000,
		},
		IPv4: []string{"192.168.1.1"},
		DNS:  []string{"8.8.8.8"},
	}

	if iface.Name != "lan" {
		t.Errorf("Name = %s, want lan", iface.Name)
	}

	if !iface.Up {
		t.Error("Up should be true")
	}

	if iface.Metrics.BytesReceived != 1000000 {
		t.Errorf("Metrics.BytesReceived = %d, want 1000000", iface.Metrics.BytesReceived)
	}

	if len(iface.IPv4) != 1 {
		t.Errorf("IPv4 length = %d, want 1", len(iface.IPv4))
	}
}

// TestMetricsStruct 测试指标结构
func TestMetricsStruct(t *testing.T) {
	metrics := Metrics{
		BytesReceived:   1000000,
		BytesSent:       500000,
		PacketsReceived: 10000,
		PacketsSent:     5000,
	}

	if metrics.BytesReceived != 1000000 {
		t.Errorf("BytesReceived = %d, want 1000000", metrics.BytesReceived)
	}

	if metrics.BytesSent != 500000 {
		t.Errorf("BytesSent = %d, want 500000", metrics.BytesSent)
	}
}
