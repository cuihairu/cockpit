package agent

// cov_gaps2_test.go 补齐 2026-10-02 feat 提交引入分支的覆盖率：
// Start 的 SSH 密钥告警、register 的 bias/fallback 分支、localip 与
// machineid 的错误路径（后两者经包级注入点，见各源文件注释）。

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// setPublicIPEndpoints 注入 publicIP 探测端点并注册恢复（注入点见 localip.go）
func setPublicIPEndpoints(t *testing.T, endpoints []string) {
	t.Helper()
	saved := publicIPEndpoints
	publicIPEndpoints = endpoints
	t.Cleanup(func() { publicIPEndpoints = saved })
}

// TestCovStartSSHKeyWarning 覆盖 Start 里 EnsureSSHKeys 失败仅告警继续的
// 分支：SSHKeys 指向不可写路径（/dev/null 下建目录必败）。
func TestCovStartSSHKeyWarning(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1/nope", SSHKeys: "/dev/null/no-such-dir"})
	defer a.Stop()

	err := a.Start()
	if err == nil || !strings.Contains(err.Error(), "connect failed") {
		t.Errorf("Start() = %v, want connect failed (SSH key warning must not abort start)", err)
	}
}

// TestCovRegisterManualBias 覆盖手动 AgentID + Bias>0 追加后缀分支；
// 无 conn 时 writeToConn 先失败，register 返回错误但 agentID 已定。
func TestCovRegisterManualBias(t *testing.T) {
	setPublicIPEndpoints(t, []string{"http://127.0.0.1:1/"})

	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1", AgentID: "cov-manual", Bias: 2, Region: "r", Zone: "z"})
	defer a.Stop()

	if err := a.register(); err == nil {
		t.Fatal("register() without conn should fail")
	}
	if a.agentID != "cov-manual-2" {
		t.Errorf("agentID = %q, want %q", a.agentID, "cov-manual-2")
	}
}

// TestCovRegisterAutoFallbackBias 覆盖自动 ID 的两条分支：machineID 读不到
// 时 fallback 到 GenerateIDWithPrefix，以及 Bias>0 的后缀追加。
func TestCovRegisterAutoFallbackBias(t *testing.T) {
	setPublicIPEndpoints(t, []string{"http://127.0.0.1:1/"})

	// platform 收编后 machineID 读取走 core/platform 装配，注入点移至 machineIDFn
	saved := machineIDFn
	machineIDFn = func() string { return "" }
	t.Cleanup(func() { machineIDFn = saved })

	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1", Bias: 3, Region: "r", Zone: "z"})
	defer a.Stop()

	if err := a.register(); err == nil {
		t.Fatal("register() without conn should fail")
	}
	hostname, _ := os.Hostname()
	if !strings.HasPrefix(a.agentID, "agent-"+hostname) {
		t.Errorf("agentID = %q, want prefix %q", a.agentID, "agent-"+hostname)
	}
	if !strings.HasSuffix(a.agentID, "-3") {
		t.Errorf("agentID = %q, want bias suffix %q", a.agentID, "-3")
	}
}

// TestCovLocalIPsError 覆盖 InterfaceAddrs 失败返回 nil 的分支
// （interfaceAddrs 注入点，见 localip.go）
func TestCovLocalIPsError(t *testing.T) {
	saved := interfaceAddrs
	interfaceAddrs = func() ([]net.Addr, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { interfaceAddrs = saved })

	if got := localIPs(); got != nil {
		t.Errorf("localIPs() = %v, want nil on error", got)
	}
}

// TestCovPublicIPBranches 覆盖 publicIP 的 Get 失败 / ReadAll 失败 /
// 全部端点失败 / 正常返回四条路径（publicIPEndpoints 注入点）。
func TestCovPublicIPBranches(t *testing.T) {
	// 找一个必然关闭的本地端口：监听后立即关闭，端口进入可复用状态，
	// 连接一律被拒绝（Get 错误分支）
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := ln.Addr().String()
	ln.Close()
	refused := "http://" + closedPort + "/"

	// 1) 全部端点失败 → 空串（同时覆盖 Get 错误 continue 与末尾 return ""）
	setPublicIPEndpoints(t, []string{refused, refused})
	if got := publicIP(); got != "" {
		t.Errorf("publicIP() all failed = %q, want empty", got)
	}

	// 2) ReadAll 失败：响应头声明 Content-Length 但提前断开，
	// 客户端读到 unexpected EOF → continue 到下一个端点
	badBody := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.(http.Flusher).Flush()
		w.Write([]byte("x"))
		// handler 返回时 net/http 服务端因未写满 Content-Length 直接
		// 关闭连接，客户端 ReadAll 得到 unexpected EOF
	}))
	defer badBody.Close()
	setPublicIPEndpoints(t, []string{badBody.URL, refused})
	if got := publicIP(); got != "" {
		t.Errorf("publicIP() short body = %q, want empty", got)
	}

	// 3) 正常：返回本机端点的 body（TrimSpace 后）
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(" 203.0.113.7 \n"))
	}))
	defer ok.Close()
	setPublicIPEndpoints(t, []string{ok.URL})
	if got := publicIP(); got != "203.0.113.7" {
		t.Errorf("publicIP() = %q, want %q", got, "203.0.113.7")
	}
}
