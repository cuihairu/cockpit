package docker

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCovNewClientInvalidHost(t *testing.T) {
	_, err := NewClient(Config{Host: "%%%invalid-host-format", Timeout: 2 * time.Second})
	if err == nil {
		t.Fatal("NewClient() should fail for invalid host format")
	}
	if !strings.Contains(err.Error(), "create docker client") {
		t.Errorf("error = %v, want create docker client failure", err)
	}
}

func TestCovNewClientDefaultTimeoutPingFail(t *testing.T) {
	// Timeout 为 0 时使用默认值；Host 合法但无守护进程监听 → Ping 失败
	_, err := NewClient(Config{Host: "tcp://127.0.0.1:1"})
	if err == nil {
		t.Fatal("NewClient() should fail when docker daemon is unreachable")
	}
	if !strings.Contains(err.Error(), "connect to docker daemon") {
		t.Errorf("error = %v, want connect failure", err)
	}
}

// covFailHandler 对所有请求返回 500
func covFailHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("cov simulated failure"))
}

func TestCovContainerActionErrors(t *testing.T) {
	c := newTestClient(t, covFailHandler)

	timeout := 5
	if err := c.StopContainer("cov-c", &timeout); err == nil {
		t.Error("StopContainer() should fail on 500")
	}
	if err := c.RestartContainer("cov-c", &timeout); err == nil {
		t.Error("RestartContainer() should fail on 500")
	}
	if err := c.RemoveContainer("cov-c", true, true); err == nil {
		t.Error("RemoveContainer() should fail on 500")
	}
	if err := c.PauseContainer("cov-c"); err == nil {
		t.Error("PauseContainer() should fail on 500")
	}
	if err := c.UnpauseContainer("cov-c"); err == nil {
		t.Error("UnpauseContainer() should fail on 500")
	}
}

func TestCovGetLogsRequestError(t *testing.T) {
	c := newTestClient(t, covFailHandler)

	_, err := c.GetLogs("cov-c", "100", "", false, false, true, true)
	if err == nil {
		t.Fatal("GetLogs() should fail on 500")
	}
	if !strings.Contains(err.Error(), "get logs") {
		t.Errorf("error = %v, want get logs failure", err)
	}
}

func TestCovGetLogsReadError(t *testing.T) {
	// 返回 200 并声明 100 字节 body 但一个字节都不写，随后 RST 连接：
	// 请求本身成功（ContainerLogs 不报错），首次 Read 无数据可得且非 EOF
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("server does not support hijacking")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		if tcp, ok := conn.(*net.TCPConn); ok {
			tcp.SetLinger(0)
		}
		conn.Close()
	})

	_, err := c.GetLogs("cov-c", "", "", false, false, true, true)
	if err == nil {
		t.Fatal("GetLogs() should fail when log body is missing")
	}
	if !strings.Contains(err.Error(), "read logs") {
		t.Errorf("error = %v, want read logs failure", err)
	}
}

func TestCovNewClientSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ping 端点（API 版本协商也依赖它）
		w.Header().Set("Api-Version", "1.43")
		w.Header().Set("Ostype", "linux")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer ts.Close()

	c, err := NewClient(Config{Host: ts.URL, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer c.Close()
}

func TestCovPullImageSuccess(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/create"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"Pull complete"}` + "\n"))
		case strings.Contains(r.URL.Path, "/images/"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"Id":"sha256:covimage123","RepoTags":["cov-image:latest"]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	id, err := c.PullImage("cov-image:latest")
	if err != nil {
		t.Fatalf("PullImage() error = %v", err)
	}
	if id != "sha256:covimage123" {
		t.Errorf("PullImage() = %q, want sha256:covimage123", id)
	}
}

func TestCovGetContainerStatsError(t *testing.T) {
	c := newTestClient(t, covFailHandler)

	_, err := c.GetContainerStats("cov-c")
	if err == nil {
		t.Fatal("GetContainerStats() should fail on 500")
	}
	if !strings.Contains(err.Error(), "get container stats") {
		t.Errorf("error = %v, want get container stats failure", err)
	}
}

func TestCovPullImageRequestError(t *testing.T) {
	c := newTestClient(t, covFailHandler)

	_, err := c.PullImage("cov-image:latest")
	if err == nil {
		t.Fatal("PullImage() should fail on 500")
	}
	if !strings.Contains(err.Error(), "pull image") {
		t.Errorf("error = %v, want pull image failure", err)
	}
}

func TestCovPullImageInspectError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/create"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"Pulling from cov-image"}`))
		default:
			// /images/{ref}/json 失败
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, err := c.PullImage("cov-image:latest")
	if err == nil {
		t.Fatal("PullImage() should fail when image inspect fails")
	}
	if !strings.Contains(err.Error(), "inspect image") {
		t.Errorf("error = %v, want inspect image failure", err)
	}
}
