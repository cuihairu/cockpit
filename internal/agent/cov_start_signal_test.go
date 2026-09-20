package agent

// cov_start_signal_test.go SIGTERM 落在 connect/register 窗口时 Start 应
// 返回 nil（信号驱动的退出不是错误，否则 systemd stop 偶发记 failed，
// CI 的 TestCovMainAgentGracefulExit 偶发 exit 1 即此窗口）。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestStartNilWhenStoppedDuringConnect(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://127.0.0.1:1/unreachable"})
	a.Stop() // 先请求退出：connect 失败应是退出而非报错
	if err := a.Start(); err != nil {
		t.Fatalf("Start after Stop should return nil, got %v", err)
	}
}

func TestStartNilWhenStoppedDuringRegister(t *testing.T) {
	up := websocket.Upgrader{}
	// 接受连接但 register 不回响应：agent 卡在 ReadMessage 等待窗口
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	a := NewAgent(Config{ServerURL: "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"})
	startErr := make(chan error, 1)
	go func() { startErr <- a.Start() }()

	// 轮询 connect 完成（此时 register 正阻塞在读），再 Stop 复现窗口
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.RLock()
		connected := a.connected
		a.mu.RUnlock()
		if connected {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	a.mu.RLock()
	connected := a.connected
	a.mu.RUnlock()
	if !connected {
		t.Fatal("agent never connected to fake server")
	}

	a.Stop()
	select {
	case err := <-startErr:
		if err != nil {
			t.Fatalf("Start stopped during register should return nil, got %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}
