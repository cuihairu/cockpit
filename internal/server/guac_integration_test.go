package server

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Guacamole 集成测试：真 guacd 容器 + 假 VNC server（127.0.0.1 loopback）
// 验证 Go 网关 → guacd → VNC 的完整链路。
//
// 跑法（本地，需 podman 或 docker；镜像加速已配）：
//
//	go test -run TestGuacIntegration ./internal/server/ -v -timeout 5m
//
// -short 跳过（CI 走 `go test -short ./...` 保持绿）。
// SSH 不走 Guacamole（api_guacamole.go:261 只承接 rdp/vnc，ssh/telnet 走
// xterm.js 的 internal/proxy/ssh_session.go，其 e2e 已覆盖 SSH loopback）。

const (
	guacIntegrationImage = "guacamole/guacd:1.5.5"
	guacIntegrationWait  = 30 * time.Second
)

// findContainerRuntime 找 podman 或 docker
func findContainerRuntime(t *testing.T) string {
	t.Helper()
	for _, bin := range []string{"podman", "docker"} {
		if path, err := exec.LookPath(bin); err == nil {
			return path
		}
	}
	t.Skip("neither podman nor docker available; skip guacd integration")
	return ""
}

// startGuacdContainer 起 guacamole/guacd:1.5.5 容器，返回 127.0.0.1:<port>
func startGuacdContainer(t *testing.T) string {
	t.Helper()
	rt := findContainerRuntime(t)

	// 容器网络下 127.0.0.1 是容器自己的 loopback，连不上宿主机的假 VNC。
	// --add-host host.internal:host-gateway 注入宿主网关 IP：guacd 连
	// host.internal:<vnc-port> 即达宿主假 VNC（podman/docker 均支持
	// host-gateway 特殊值；rootless podman 的 --network host 受限不可用）。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	name := fmt.Sprintf("cockpit-guac-test-%d", time.Now().UnixNano()%1e6)
	out, err := exec.Command(rt, "run", "-d", "--rm",
		"--name", name,
		"-p", fmt.Sprintf("127.0.0.1:%d:4822", port),
		"--add-host", "host.internal:host-gateway",
		"-e", "GUACD_LOG_LEVEL=info",
		guacIntegrationImage,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("start guacd container: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command(rt, "rm", "-f", name).Run()
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	waitForTCP(t, addr, "guacd 4822")
	return addr
}

// waitForTCP 等 TCP 端口可连
func waitForTCP(t *testing.T, addr, desc string) {
	t.Helper()
	deadline := time.Now().Add(guacIntegrationWait)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s at %s", desc, addr)
}

// fakeVNC 假 VNC server：RFB 握手（None auth）+ 一帧 framebuffer。
// guacd 作为 VNC client 连它，验证协议会话建立。
type fakeVNC struct {
	ln    net.Listener
	conns int32
}

func startFakeVNC(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeVNC{ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String()
}

// serve RFB 3.8 握手：None 认证 → ServerInit → 一帧 framebuffer update
func (f *fakeVNC) serve(c net.Conn) {
	defer c.Close()
	fmt.Printf("[fakeVNC] conn from %s\n", c.RemoteAddr())
	// 1. 协议版本
	if _, err := c.Write([]byte("RFB 003.008\n")); err != nil {
		fmt.Printf("[fakeVNC] write ver: %v\n", err)
		return
	}
	ver := make([]byte, 12)
	if _, err := io.ReadFull(c, ver); err != nil {
		return
	}
	// 2. 安全类型：仅 None(0)
	if _, err := c.Write([]byte{1, 0}); err != nil {
		return
	}
	sel := make([]byte, 1)
	if _, err := io.ReadFull(c, sel); err != nil {
		return
	}
	// 3. SecurityResult = OK
	if _, err := c.Write([]byte{0, 0, 0, 0}); err != nil {
		return
	}
	// 4. ClientInit（shared flag）
	if _, err := io.ReadFull(c, make([]byte, 1)); err != nil {
		return
	}
	// 5. ServerInit：800x600, 32bpp, name "fake-vnc"
	name := "fake-vnc"
	buf := make([]byte, 24+len(name))
	binary.BigEndian.PutUint16(buf[0:], 800)
	binary.BigEndian.PutUint16(buf[2:], 600)
	// pixel format (16 bytes)：bits-per-pixel=32, depth=24, true-color=1
	buf[4] = 32
	buf[5] = 24
	buf[6] = 1
	buf[7] = 0 // big-endian
	binary.BigEndian.PutUint16(buf[8:], 255)  // red-max
	binary.BigEndian.PutUint16(buf[10:], 255) // green-max
	binary.BigEndian.PutUint16(buf[12:], 255) // blue-max
	buf[14] = 16 // red-shift
	buf[15] = 8  // green-shift
	buf[16] = 0  // blue-shift
	binary.BigEndian.PutUint32(buf[20:], uint32(len(name)))
	copy(buf[24:], name)
	if _, err := c.Write(buf); err != nil {
		return
	}
	// 6. 一帧 FramebufferUpdate（1 rect: 0,0,800,600, raw）
	// message-type=0, padding=0, num-rects=1
	upd := []byte{0, 0, 0, 1}
	// rect header: x=0,y=0,w=800,h=600, encoding=0(raw)
	hdr := make([]byte, 12)
	binary.BigEndian.PutUint16(hdr[0:], 0)
	binary.BigEndian.PutUint16(hdr[2:], 0)
	binary.BigEndian.PutUint16(hdr[4:], 800)
	binary.BigEndian.PutUint16(hdr[6:], 600)
	binary.BigEndian.PutUint32(hdr[8:], 0) // raw
	// raw 像素数据 800*600*4 = 1.92MB —— 太大；发 0x0 即可（guacd 读多少算多少）
	if _, err := c.Write(append(upd, hdr...)); err != nil {
		return
	}
	// 7. 保持连接开着直到对端关
	_, _ = io.Copy(io.Discard, c)
}

// TestGuacIntegrationGuacdContainer 容器起来 + 端口通 + Go 网关能拨通
func TestGuacIntegrationGuacdContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("guacd integration: skip in -short (CI)")
	}
	addr := startGuacdContainer(t)
	t.Setenv("GUACD_ADDR", addr)

	s := covRemoteSetup(t)
	ticket := covTicket(t, s, map[string]string{
		"agent_id": "ag-guac", "host": "127.0.0.1", "port": "5900", "protocol": "vnc",
	})
	conn, _, tDone := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole", ticket)
	t.Cleanup(func() { conn.Close() })

	// 首条 = tunnel UUID（INTERNAL_DATA 单元素指令）
	_, uuidFrame, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	if !strings.HasPrefix(string(uuidFrame), "0.,") {
		t.Errorf("tunnel uuid frame should be INTERNAL_DATA, got %q", uuidFrame)
	}
	conn.Close()
	covWaitHandlerExit(t, "guacd container session", tDone)
}

// TestGuacIntegrationVNC 完整链路：Go 网关 → guacd 容器 → 假 VNC（loopback）
func TestGuacIntegrationVNC(t *testing.T) {
	if testing.Short() {
		t.Skip("guacd integration: skip in -short (CI)")
	}
	defer covClearSessions()

	vncAddr := startFakeVNC(t)
	_, portStr, _ := net.SplitHostPort(vncAddr)
	host := "host.internal" // 容器内解析为宿主网关 IP（--add-host 注入）

	guacdAddr := startGuacdContainer(t)
	t.Setenv("GUACD_ADDR", guacdAddr)

	s := covRemoteSetup(t)
	ticket := covTicket(t, s, map[string]string{
		"agent_id": "ag-vnc", "host": host, "port": portStr, "protocol": "vnc",
		"password": "vncpw",
	})
	conn, _, tDone := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole", ticket)
	t.Cleanup(func() { conn.Close() })

	// 首条 = tunnel UUID
	_, uuidFrame, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	if !strings.HasPrefix(string(uuidFrame), "0.,") {
		t.Errorf("tunnel uuid frame should be INTERNAL_DATA, got %q", uuidFrame)
	}

	// guacd 连上假 VNC 并建立 RFB 会话后，会推指令流（sync/loop 等）。
	// 一帧非内部指令即证明完整链路通（浏览器→网关→guacd→VNC→guacd→网关→浏览器）。
	// 注意 gorilla 读超时后连接不可再用，单次 ReadMessage 带长超时，不重试。
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	_, frame, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read instruction frame from guacd: %v", err)
	}
	t.Logf("guacd->gateway frame: %s", frame)
	if !strings.HasSuffix(string(frame), ";") {
		t.Errorf("frame should end with ';', got %q", frame)
	}
	if strings.HasPrefix(string(frame), "0.,") {
		t.Errorf("expected non-internal instruction, got tunnel uuid %q", frame)
	}

	// 浏览器 → guacd：WS 文本帧直写 TCP（4.nop 无害指令）
	if err := conn.WriteMessage(websocket.TextMessage, []byte("4.nop,1.x;")); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.Close()
	covWaitHandlerExit(t, "vnc integration session", tDone)
}
