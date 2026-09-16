package pve

// cov_client_test.go 覆盖 client.go 的错误分支与未覆盖的成功路径：
// doRequest 的建请求/HTTP 错误、post/put 的 marshal 错误、put 的成功路径、
// ListNodes/GetNodeStatus/ListVMs/GetVM 的完整场景与 GetContainer 的各错误分支。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// covPVE 起 mock PVE API server 并返回指向它的 client
func covPVE(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(Config{Endpoint: srv.URL, TokenID: "cov@pve!t", TokenSecret: "s"})
}

// covPVEAlways 固定状态码 + body
func covPVEAlways(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
}

func TestCovDoRequestCreateError(t *testing.T) {
	c := NewClient(Config{Endpoint: "http://127.0.0.1:1"})
	if _, err := c.doRequest("GE T", "/", nil); err == nil {
		t.Fatal("invalid method should error")
	}
}

func TestCovDoRequestReadError(t *testing.T) {
	// 声明 1000 字节但只写 2 字节：客户端 io.ReadAll 报 unexpected EOF
	c := covPVE(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte(`{}`))
	})
	if _, err := c.doRequest("GET", "/api2/json/x", nil); err == nil {
		t.Fatal("truncated body should fail io.ReadAll")
	}
}

func TestCovDoRequestHTTPError(t *testing.T) {
	c := covPVE(t, covPVEAlways(500, `boom`))
	if _, err := c.doRequest("GET", "/api2/json/whatever", nil); err == nil {
		t.Fatal("500 should error")
	} else if !strings.Contains(err.Error(), "PVE API error (status 500)") {
		t.Errorf("err = %v, want PVE API error", err)
	}
}

func TestCovPostMarshalError(t *testing.T) {
	c := NewClient(Config{Endpoint: "http://127.0.0.1:1"})
	if _, err := c.post("/x", make(chan int)); err == nil {
		t.Fatal("unmarshalable body should error")
	}
}

func TestCovPutMarshalError(t *testing.T) {
	c := NewClient(Config{Endpoint: "http://127.0.0.1:1"})
	if _, err := c.put("/x", make(chan int)); err == nil {
		t.Fatal("unmarshalable body should error")
	}
}

func TestCovPutSuccess(t *testing.T) {
	var method, body string
	c := covPVE(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		buf := make([]byte, 64)
		n, _ := r.Body.Read(buf)
		body = string(buf[:n])
		_, _ = w.Write([]byte(`{"data":{"ok":1}}`))
	})
	out, err := c.put("/api2/json/cov", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("put error = %v", err)
	}
	if method != "PUT" || !strings.Contains(body, `"k":"v"`) {
		t.Errorf("method = %s body = %s", method, body)
	}
	if !strings.Contains(string(out), `"ok":1`) {
		t.Errorf("put output = %s", out)
	}
}

func TestCovListNodes(t *testing.T) {
	c := covPVE(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "PVEAPIToken=cov@pve!t=s" {
			t.Errorf("auth = %s", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":[{"node":"pve1","status":"online","maxcpu":8}]}`))
	})
	nodes, err := c.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes error = %v", err)
	}
	if len(nodes) != 1 || nodes[0].Node != "pve1" || nodes[0].MaxCPU != 8 {
		t.Errorf("nodes = %+v", nodes)
	}
}

func TestCovListNodesErrors(t *testing.T) {
	c := covPVE(t, covPVEAlways(500, `boom`))
	if _, err := c.ListNodes(); err == nil {
		t.Fatal("500 should error")
	}
	c2 := covPVE(t, covPVEAlways(200, `not json`))
	if _, err := c2.ListNodes(); err == nil {
		t.Fatal("invalid json should error")
	}
}

func TestCovGetNodeStatus(t *testing.T) {
	c := covPVE(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve1/status" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"node":"pve1","status":"online","uptime":42}}`))
	})
	node, err := c.GetNodeStatus("pve1")
	if err != nil {
		t.Fatalf("GetNodeStatus error = %v", err)
	}
	if node.Node != "pve1" || node.Uptime != 42 {
		t.Errorf("node = %+v", node)
	}
}

func TestCovGetNodeStatusError(t *testing.T) {
	c := covPVE(t, covPVEAlways(500, `boom`))
	if _, err := c.GetNodeStatus("pve1"); err == nil {
		t.Fatal("500 should error")
	}
}

func TestCovListVMs(t *testing.T) {
	c := covPVE(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve1/qemu" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"vmid":100,"name":"web","status":"running"}]}`))
	})
	vms, err := c.ListVMs("pve1")
	if err != nil {
		t.Fatalf("ListVMs error = %v", err)
	}
	if len(vms) != 1 || vms[0].VMID != 100 {
		t.Errorf("vms = %+v", vms)
	}
}

func TestCovListVMsNoNode(t *testing.T) {
	c := NewClient(Config{Endpoint: "http://127.0.0.1:1"}) // 无默认节点
	if _, err := c.ListVMs(""); err == nil {
		t.Fatal("empty node should error")
	}
}

func TestCovListVMsErrors(t *testing.T) {
	c := covPVE(t, covPVEAlways(500, `boom`))
	if _, err := c.ListVMs("pve1"); err == nil {
		t.Fatal("500 should error")
	}
	c2 := covPVE(t, covPVEAlways(200, `not json`))
	if _, err := c2.ListVMs("pve1"); err == nil {
		t.Fatal("invalid json should error")
	}
}

func TestCovGetVMSuccess(t *testing.T) {
	c := covPVE(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve1/qemu/100/status/current":
			_, _ = w.Write([]byte(`{"data":{"vm":{"vmid":100,"name":"web","status":"running"},"lock":"","qmpstatus":"running"}}`))
		case "/api2/json/nodes/pve1/qemu/100/config":
			_, _ = w.Write([]byte(`{"data":{"cores":4,"memory":8192,"name":"web","ostype":"l26"}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	cfg, err := c.GetVM("pve1", 100)
	if err != nil {
		t.Fatalf("GetVM error = %v", err)
	}
	if cfg.VM.VMID != 100 || cfg.Config.Cores != 4 || cfg.Config.Memory != 8192 || cfg.QMPStatus != "running" {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestCovGetVMErrors(t *testing.T) {
	// status 接口 500
	c := covPVE(t, covPVEAlways(500, `boom`))
	if _, err := c.GetVM("pve1", 100); err == nil {
		t.Fatal("status 500 should error")
	}
	// status 返回非法 JSON
	c2 := covPVE(t, covPVEAlways(200, `not json`))
	if _, err := c2.GetVM("pve1", 100); err == nil {
		t.Fatal("status bad json should error")
	}
	// status 成功但 config 接口 500
	c3 := covPVE(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/config") {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`boom`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"vm":{"vmid":100},"lock":"","qmpstatus":"running"}}`))
	})
	if _, err := c3.GetVM("pve1", 100); err == nil {
		t.Fatal("config 500 should error")
	}
	// config 返回非法 JSON
	c4 := covPVE(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/config") {
			_, _ = w.Write([]byte(`not json`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"vm":{"vmid":100},"lock":"","qmpstatus":"running"}}`))
	})
	if _, err := c4.GetVM("pve1", 100); err == nil {
		t.Fatal("config bad json should error")
	}
}

func TestCovGetVMNoNode(t *testing.T) {
	c := NewClient(Config{Endpoint: "http://127.0.0.1:1"})
	if _, err := c.GetVM("", 100); err == nil {
		t.Fatal("empty node should error")
	}
}

func TestCovListContainersError(t *testing.T) {
	c := covPVE(t, covPVEAlways(500, `boom`))
	if _, err := c.ListContainers("pve1"); err == nil {
		t.Fatal("500 should error")
	}
}

func TestCovGetContainerErrors(t *testing.T) {
	// status 500
	c := covPVE(t, covPVEAlways(500, `boom`))
	if _, err := c.GetContainer("pve1", 200); err == nil {
		t.Fatal("status 500 should error")
	}
	// status 非法 JSON
	c2 := covPVE(t, covPVEAlways(200, `not json`))
	if _, err := c2.GetContainer("pve1", 200); err == nil {
		t.Fatal("status bad json should error")
	}
	// config 500
	c3 := covPVE(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/config") {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`boom`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"vmid":200,"name":"ct"}}`))
	})
	if _, err := c3.GetContainer("pve1", 200); err == nil {
		t.Fatal("config 500 should error")
	}
	// config 非法 JSON
	c4 := covPVE(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/config") {
			_, _ = w.Write([]byte(`not json`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"vmid":200,"name":"ct"}}`))
	})
	if _, err := c4.GetContainer("pve1", 200); err == nil {
		t.Fatal("config bad json should error")
	}
}

func TestCovGetContainerNoNode(t *testing.T) {
	c := NewClient(Config{Endpoint: "http://127.0.0.1:1"})
	if _, err := c.GetContainer("", 200); err == nil {
		t.Fatal("empty node should error")
	}
}
