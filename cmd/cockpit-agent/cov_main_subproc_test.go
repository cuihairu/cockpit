package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 子进程覆盖入口：TestMain 检测到守卫环境变量后直接以目标参数调用 main()，
// 覆盖覆盖率无法在进程内触达的 main/成功退出路径；数据经继承的 GOCOVERDIR
// 并回父进程 profile。
func TestMain(m *testing.M) {
	switch os.Getenv("COCKPIT_AGENT_MAIN_SUBPROC") {
	case "version":
		os.Args = []string{"cockpit-agent", "version"}
		main() // 内部 os.Exit，不返回
	case "graceful":
		os.Args = []string{"cockpit-agent", "start", "-server", os.Getenv("COCKPIT_AGENT_MAIN_TEST_WS")}
		main()
	default:
		os.Exit(m.Run())
	}
}

func covSubprocEnv(mode string, extra ...string) []string {
	return append(os.Environ(),
		append([]string{"COCKPIT_AGENT_MAIN_SUBPROC=" + mode}, extra...)...)
}

// TestCovAgentMainEntry 用子进程覆盖 main()（version 路径，正常退出 0）。
func TestCovAgentMainEntry(t *testing.T) {
	if os.Getenv("COCKPIT_AGENT_MAIN_SUBPROC") != "" {
		t.Skip("subprocess child")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$", "-test.timeout=60s")
	cmd.Env = covSubprocEnv("version")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("subprocess: %v out: %s", err, out)
	}
}

// TestCovAgentMainGracefulExit 子进程真实启动 agent 连上假 WS 服务，
// 注册成功后由父进程发 SIGTERM，覆盖信号优雅退出与成功返回 0 的路径。
func TestCovAgentMainGracefulExit(t *testing.T) {
	if os.Getenv("COCKPIT_AGENT_MAIN_SUBPROC") != "" {
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
	cmd := exec.Command(os.Args[0], "-test.run=^$", "-test.timeout=60s")
	cmd.Env = covSubprocEnv("graceful", "COCKPIT_AGENT_MAIN_TEST_WS="+wsURL)
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
