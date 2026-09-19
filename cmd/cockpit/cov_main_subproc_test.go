package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 子进程覆盖入口：TestMain 检测到守卫环境变量后直接以目标参数调用 main()，
// 覆盖进程内无法触达的 main()/无参 server/成功退出路径；覆盖率数据经继承的
// GOCOVERDIR 并回父进程 profile。
func TestMain(m *testing.M) {
	switch os.Getenv("COCKPIT_MAIN_SUBPROC") {
	case "version":
		os.Args = []string{"cockpit", "version"}
		main() // 内部 os.Exit，不返回
	case "noargs":
		// 无命令进入默认 server 启动；CWD 为临时目录、未设 ADMIN_PASSWORD，
		// NewServer 打开库后 Start 的安全检查 log.Fatal 退出（非零码）
		os.Args = []string{"cockpit"}
		main()
	case "server-bindfail":
		// 强密码已设置但端口被父进程占用：Start 返回错误 → 退出码 1
		os.Args = []string{"cockpit", "server", "-config", os.Getenv("COCKPIT_MAIN_TEST_CONFIG")}
		main()
	case "agent":
		os.Args = []string{"cockpit", "agent", "-server", os.Getenv("COCKPIT_MAIN_TEST_WS")}
		main()
	default:
		os.Exit(m.Run())
	}
}

func covSubprocEnv(mode string, extra ...string) []string {
	return append(os.Environ(),
		append([]string{"COCKPIT_MAIN_SUBPROC=" + mode}, extra...)...)
}

func covRunSubproc(t *testing.T, dir, mode string, extraEnv ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$", "-test.timeout=60s")
	cmd.Dir = dir
	cmd.Env = covSubprocEnv(mode, extraEnv...)
	return cmd
}

// TestCovMainEntry 子进程覆盖 main()（version 路径，正常退出 0）。
func TestCovMainEntry(t *testing.T) {
	if os.Getenv("COCKPIT_MAIN_SUBPROC") != "" {
		t.Skip("subprocess child")
	}
	cmd := covRunSubproc(t, t.TempDir(), "version")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("subprocess: %v out: %s", err, out)
	}
}

// TestCovMainNoArgsDefaultsToServer 子进程覆盖无命令默认进入 server 的分支；
// 未设 ADMIN_PASSWORD 时 Start 的安全检查 log.Fatal 非零退出。
func TestCovMainNoArgsDefaultsToServer(t *testing.T) {
	if os.Getenv("COCKPIT_MAIN_SUBPROC") != "" {
		t.Skip("subprocess child")
	}
	cmd := covRunSubproc(t, t.TempDir(), "noargs")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit from ADMIN_PASSWORD check, out: %s", out)
	}
	if !strings.Contains(string(out), "SECURITY ERROR") && !strings.Contains(string(out), "ADMIN_PASSWORD") {
		t.Fatalf("out = %s, want ADMIN_PASSWORD security check failure", out)
	}
}

// TestCovMainServerStartError 子进程以强密码启动，但端口被父进程占用，
// 覆盖 startServer 中 Start 返回错误 → 退出码 1 的路径。
func TestCovMainServerStartError(t *testing.T) {
	if os.Getenv("COCKPIT_MAIN_SUBPROC") != "" {
		t.Skip("subprocess child")
	}

	// 父进程占位端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	port := ln.Addr().(*net.TCPAddr).Port

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cockpit.yaml")
	covWriteFile(t, cfgPath, "server:\n  host: 127.0.0.1\n  port: "+itoa(port)+"\ndatabase:\n  path: "+filepath.Join(dir, "cov.db")+"\n")

	cmd := covRunSubproc(t, dir, "server-bindfail",
		"COCKPIT_MAIN_TEST_CONFIG="+cfgPath,
		"ADMIN_PASSWORD=cov-admin-passphrase-9x7K",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit on bind failure, out: %s", out)
	}
	if !strings.Contains(string(out), "Server error") {
		t.Fatalf("out = %s, want server bind error", out)
	}
}

// TestCovMainAgentGracefulExit 子进程 `cockpit agent` 真实连上假 WS 服务，
// 注册成功后父进程发 SIGTERM，覆盖 cli 信号处理与成功返回 0 路径。
func TestCovMainAgentGracefulExit(t *testing.T) {
	if os.Getenv("COCKPIT_MAIN_SUBPROC") != "" {
		t.Skip("subprocess child")
	}

	up := websocket.Upgrader{}
	registered := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg struct {
				Type    string `json:"type"`
				Payload map[string]any
			}
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			if msg.Type == "register" {
				resp := map[string]any{"id": "resp-1", "type": "register", "payload": map[string]any{"ok": true}}
				conn.WriteJSON(resp)
				select {
				case registered <- struct{}{}:
				default:
				}
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	cmd := covRunSubproc(t, t.TempDir(), "agent", "COCKPIT_MAIN_TEST_WS="+wsURL)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-registered:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("child agent never registered")
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("send SIGTERM: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child should exit 0 after graceful SIGTERM, got %v", err)
	}
}

// itoa 简单 int 转字符串（避免额外导入）。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
