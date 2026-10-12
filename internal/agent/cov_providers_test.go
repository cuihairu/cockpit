package agent

// cov_* 测试：覆盖率攻坚新增（不修改既有测试文件）。
// 覆盖 setupProviders / registerDockerProvider / registerStackProvider 的
// 原未覆盖分支。docker daemon 用 unix socket 上的最小 fake（/_ping、
// /version，形状同 scripts/local-acceptance/fake-docker-daemon.go）。

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/proxy"
)

// covFakeDockerDaemon 在临时 unix socket 上模拟 docker daemon 的
// 存活探测端点，返回 socket 路径。
func covFakeDockerDaemon(t *testing.T) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "docker.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("API-Version", "1.51")
		w.Header().Set("Ostype", "linux")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Version":       "27.3.1-fake",
			"ApiVersion":    "1.51",
			"MinAPIVersion": "1.24",
			"Os":            "linux",
		})
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close(); _ = ln.Close() })
	return sock
}

// covRegistered 判断 provider 是否已注册。
func covRegistered(a *Agent, typ string) bool {
	for _, t := range a.rpc.RegisteredTypes() {
		if t == typ {
			return true
		}
	}
	return false
}

func TestCovSetupProvidersNginxCapability(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "nginx-proxy"}}
	a.setupProviders()

	if !covRegistered(a, "nginx") {
		t.Errorf("nginx provider should register for nginx-proxy capability, got %v", a.rpc.RegisteredTypes())
	}
}

func TestCovHasCapabilityMiss(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "pve-api"}}
	if a.hasCapability("cron") {
		t.Error("hasCapability(cron) should be false")
	}
	if !a.hasCapability("pve-api") {
		t.Error("hasCapability(pve-api) should be true")
	}
}

func TestCovRegisterDockerProviderSuccess(t *testing.T) {
	sock := covFakeDockerDaemon(t)
	withEnv(t, map[string]string{"DOCKER_HOST": ""})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "docker-api", Endpoint: "unix://" + sock}}
	a.setupProviders()

	if !covRegistered(a, "docker") {
		t.Errorf("docker provider should register against fake daemon, got %v", a.rpc.RegisteredTypes())
	}
}

func TestCovRegisterStackProviderDaemonDown(t *testing.T) {
	// compose CLI 可用，但 daemon 不可达 → NewClient 失败，stack 跳过
	fakeBinDirCov(t, map[string]string{"docker": "exit 0"})
	withEnv(t, map[string]string{"DOCKER_HOST": ""})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{
		{Type: "docker-api", Endpoint: "unix:///definitely-not-a-real-daemon.sock"},
	}
	a.setupProviders()

	if covRegistered(a, "stack") {
		t.Errorf("stack provider should be skipped when daemon is down, got %v", a.rpc.RegisteredTypes())
	}
}

func TestCovRegisterStackProviderSuccess(t *testing.T) {
	sock := covFakeDockerDaemon(t)
	fakeBinDirCov(t, map[string]string{"docker": "exit 0"}) // `docker compose version` 退出 0
	stacksDir := t.TempDir()
	withEnv(t, map[string]string{
		"DOCKER_HOST":        "",
		"COCKPIT_STACKS_DIR": stacksDir,
	})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "docker-api", Endpoint: "unix://" + sock}}
	a.setupProviders()

	if !covRegistered(a, "stack") {
		t.Errorf("stack provider should register with compose + fake daemon, got %v", a.rpc.RegisteredTypes())
	}
	if !covRegistered(a, "docker") {
		t.Errorf("docker provider should register against fake daemon, got %v", a.rpc.RegisteredTypes())
	}
}

// fakeBinDirCov 创建只含 stub 可执行文件的目录并整体替换 PATH
// （agent 包版本的 fakeBinDir，不依赖 detector 包）。
func fakeBinDirCov(t *testing.T, scripts map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0755); err != nil {
			t.Fatalf("write stub %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir)
}

// TestCovStackFollowWiring 覆盖任务日志 follow 的 sender/closer 出口
// （registerStackProvider 内联闭包的具名化，P2 ②）：proxy_data 带
// proxyId=connId 双同值载荷，proxy_close 带 reason。
func TestCovStackFollowWiring(t *testing.T) {
	h := proxy.NewHandler()
	h.Start(nil) // AttachConn(nil) 仅置 running；真实发送走 SetSendFunc 捕获
	var mu sync.Mutex
	var msgs []*protocol.Message
	h.SetSendFunc(func(m *protocol.Message) error {
		mu.Lock()
		msgs = append(msgs, m)
		mu.Unlock()
		return nil
	})

	stackTaskFollowSender(h)("stacktask:f1", []byte("up -d web"))
	stackTaskFollowCloser(h)("stacktask:f1", "success")

	mu.Lock()
	defer mu.Unlock()
	if len(msgs) != 2 {
		t.Fatalf("msgs = %d, want 2", len(msgs))
	}
	if msgs[0].Type != protocol.MessageTypeProxyData {
		t.Errorf("sender type = %v, want proxy_data", msgs[0].Type)
	}
	for _, k := range []string{"proxyId", "connId"} {
		if msgs[0].Payload[k] != "stacktask:f1" {
			t.Errorf("sender %s = %v, want stacktask:f1", k, msgs[0].Payload[k])
		}
	}
	if msgs[1].Type != protocol.MessageTypeProxyClose || msgs[1].Payload["reason"] != "success" {
		t.Errorf("closer = %v / reason %v, want proxy_close success", msgs[1].Type, msgs[1].Payload["reason"])
	}
}
