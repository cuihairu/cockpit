package pve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestListNodes 测试列出节点
func TestListNodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		auth := r.Header.Get("Authorization")
		if auth != "PVEAPIToken=test@pve!token=secret" {
			t.Errorf("unexpected auth: %s", auth)
		}

		resp := map[string]interface{}{
			"data": []Node{
				{Node: "pve1", Status: "online", CPU: 0.1, MaxCPU: 4, Mem: 1000, MaxMem: 8000},
				{Node: "pve2", Status: "online", CPU: 0.2, MaxCPU: 8, Mem: 2000, MaxMem: 16000},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	nodes, err := client.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}

	if len(nodes) != 2 {
		t.Errorf("len(nodes) = %d, want 2", len(nodes))
	}

	if nodes[0].Node != "pve1" {
		t.Errorf("nodes[0].Node = %s, want pve1", nodes[0].Node)
	}
}

// TestGetNodeStatus 测试获取节点状态
func TestGetNodeStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve1/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"data": Node{
				Node:   "pve1",
				Status: "online",
				CPU:    0.15,
				MaxCPU: 4,
				Mem:    2000000000,
				MaxMem: 8000000000,
				Uptime: 86400,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	status, err := client.GetNodeStatus("pve1")
	if err != nil {
		t.Fatalf("GetNodeStatus() error = %v", err)
	}

	if status.Node != "pve1" {
		t.Errorf("Node = %s, want pve1", status.Node)
	}

	if status.Status != "online" {
		t.Errorf("Status = %s, want online", status.Status)
	}
}

// TestListVMs 测试列出虚拟机
func TestListVMs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve1/qemu" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"data": []VM{
				{VMID: 100, Name: "vm-100", Status: "running", CPUs: 2, Mem: 2000000000},
				{VMID: 101, Name: "vm-101", Status: "stopped", CPUs: 4, Mem: 4000000000},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	vms, err := client.ListVMs("pve1")
	if err != nil {
		t.Fatalf("ListVMs() error = %v", err)
	}

	if len(vms) != 2 {
		t.Errorf("len(vms) = %d, want 2", len(vms))
	}

	if vms[0].VMID != 100 {
		t.Errorf("vms[0].VMID = %d, want 100", vms[0].VMID)
	}
}

// TestListVMsWithDefaultNode 测试使用默认节点
func TestListVMsWithDefaultNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []VM{
				{VMID: 100, Name: "test", Status: "running"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
		Node:        "pve1",
	})

	vms, err := client.ListVMs("")
	if err != nil {
		t.Fatalf("ListVMs() error = %v", err)
	}

	if len(vms) != 1 {
		t.Errorf("len(vms) = %d, want 1", len(vms))
	}
}

// TestListVMsWithoutNodeError 测试没有节点时的错误
func TestListVMsWithoutNodeError(t *testing.T) {
	client := NewClient(Config{
		Endpoint:    "https://pve.example.com:8006",
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	_, err := client.ListVMs("")
	if err == nil {
		t.Error("ListVMs() without node should return error")
	}
}

// TestStartVM 测试启动虚拟机
func TestStartVM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api2/json/nodes/pve1/qemu/100/status/start" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	err := client.StartVM("pve1", 100)
	if err != nil {
		t.Errorf("StartVM() error = %v", err)
	}
}

// TestStopVM 测试停止虚拟机
func TestStopVM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api2/json/nodes/pve1/qemu/100/status/stop" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	err := client.StopVM("pve1", 100)
	if err != nil {
		t.Errorf("StopVM() error = %v", err)
	}
}

// TestRestartVM 测试重启虚拟机
func TestRestartVM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api2/json/nodes/pve1/qemu/100/status/reboot" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	err := client.RestartVM("pve1", 100)
	if err != nil {
		t.Errorf("RestartVM() error = %v", err)
	}
}

// TestGetVM 测试获取虚拟机详情
func TestGetVM(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if callCount == 1 {
			// Status call
			if r.URL.Path != "/api2/json/nodes/pve1/qemu/100/status/current" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"vm": VM{
						VMID:   100,
						Name:   "test-vm",
						Status: "running",
						CPUs:   2,
						Mem:    2000000000,
					},
					"qmpstatus": "running",
				},
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			// Config call
			if r.URL.Path != "/api2/json/nodes/pve1/qemu/100/config" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			resp := map[string]interface{}{
				"data": VMConfigData{
					Cores:  2,
					Memory: 2048,
					Name:   "test-vm",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	vm, err := client.GetVM("pve1", 100)
	if err != nil {
		t.Fatalf("GetVM() error = %v", err)
	}

	if vm.VM.Name != "test-vm" {
		t.Errorf("VM.Name = %s, want test-vm", vm.VM.Name)
	}

	if vm.Config.Cores != 2 {
		t.Errorf("Config.Cores = %d, want 2", vm.Config.Cores)
	}
}

// TestHTTPRequestError 测试HTTP请求错误
func TestHTTPRequestError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errors": "authentication failed"}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "wrong-secret",
	})

	_, err := client.ListNodes()
	if err == nil {
		t.Error("ListNodes() with wrong credentials should return error")
	}
}

// TestHTTPRequestTimeout 测试请求超时
func TestHTTPRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"data": []}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})
	client.httpClient.Timeout = 100 * time.Millisecond

	_, err := client.ListNodes()
	if err == nil {
		t.Error("ListNodes() with timeout should return error")
	}
}

// TestClientWithInsecureTLS 测试不安全TLS配置
func TestClientWithInsecureTLS(t *testing.T) {
	client := NewClient(Config{
		Endpoint:    "https://pve.example.com:8006",
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
		InsecureTLS: true,
	})

	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("httpClient.Transport is not *http.Transport")
	}

	if transport.TLSClientConfig == nil {
		t.Error("TLSClientConfig should be set")
	}

	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

// TestGetRequest 测试GET请求
func TestGetRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}

		resp := map[string]interface{}{
			"data": []Node{
				{Node: "pve1", Status: "online"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	body, err := client.get("/api2/json/nodes")
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}

	if len(body) == 0 {
		t.Error("response body should not be empty")
	}
}

// TestPostRequest 测试POST请求
func TestPostRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", r.Header.Get("Content-Type"))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": "OK"}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	body, err := client.post("/api2/json/test", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("post() error = %v", err)
	}

	if len(body) == 0 {
		t.Error("response body should not be empty")
	}
}

// TestPutRequest 测试PUT请求
func TestPutRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": "OK"}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	_, err := client.put("/api2/json/test", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("put() error = %v", err)
	}
}

// TestDeleteRequest 测试DELETE请求
func TestDeleteRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": "OK"}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	_, err := client.del("/api2/json/test/123")
	if err != nil {
		t.Fatalf("del() error = %v", err)
	}
}

// TestListContainers 测试列出容器
func TestListContainers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve1/lxc" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"data": []Container{
				{VMID: 100, Name: "ct-100", Status: "running"},
				{VMID: 101, Name: "ct-101", Status: "stopped"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	containers, err := client.ListContainers("pve1")
	if err != nil {
		t.Fatalf("ListContainers() error = %v", err)
	}

	if len(containers) != 2 {
		t.Errorf("len(containers) = %d, want 2", len(containers))
	}

	if containers[0].VMID != 100 {
		t.Errorf("containers[0].VMID = %d, want 100", containers[0].VMID)
	}
}

// TestInvalidJSONResponse 测试无效JSON响应
func TestInvalidJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{invalid json}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	_, err := client.ListNodes()
	if err == nil {
		t.Error("ListNodes() with invalid JSON should return error")
	}
}

// TestClientTimeout 测试客户端超时设置
func TestClientTimeout(t *testing.T) {
	client := NewClient(Config{
		Endpoint:    "https://pve.example.com:8006",
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	if client.httpClient.Timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", client.httpClient.Timeout)
	}
}

// TestEmptyResponse 测试空响应
func TestEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []Node{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:    server.URL,
		TokenID:     "test@pve!token",
		TokenSecret: "secret",
	})

	nodes, err := client.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}

	if len(nodes) != 0 {
		t.Errorf("len(nodes) = %d, want 0", len(nodes))
	}
}
