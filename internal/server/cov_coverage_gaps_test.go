package server

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 覆盖率缺口补测（只补测试零业务改动）：guacdAddr 默认分支 / copyFile 三错误
// 分支 / collectGuacRecording 三错误分支 / handleGuacamoleWebSocket 的 Upgrade
// 失败·handshake 写失败·录制元数据登记失败 / wsToGuacd 写失败 / handleTicketCreate
// domain 分支 / Start 的 DNS provider 启用分支。

func TestGuacdAddrDefaultAndEnv(t *testing.T) {
	t.Setenv("GUACD_ADDR", "")
	if got := guacdAddr(); got != guacdDefaultAddr {
		t.Fatalf("guacdAddr() default = %q, want %q", got, guacdDefaultAddr)
	}
	t.Setenv("GUACD_ADDR", "10.0.0.5:4822")
	if got := guacdAddr(); got != "10.0.0.5:4822" {
		t.Fatalf("guacdAddr() env = %q, want 10.0.0.5:4822", got)
	}
}

func TestCopyFileErrorBranches(t *testing.T) {
	dir := t.TempDir()

	// os.Open 失败：src 不存在
	if err := copyFile(filepath.Join(dir, "nope"), filepath.Join(dir, "out"), 0600); err == nil {
		t.Fatal("copyFile(missing src): want error, got nil")
	}

	// os.OpenFile 失败：dst 是目录
	src := filepath.Join(dir, "src.bin")
	if err := os.WriteFile(src, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	dstDir := filepath.Join(dir, "dst-dir")
	if err := os.MkdirAll(dstDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dstDir, 0600); err == nil {
		t.Fatal("copyFile(dst is dir): want error, got nil")
	}

	// io.Copy 失败：src 是目录（Open 成功但 Read 失败）
	srcDir := filepath.Join(dir, "src-dir")
	if err := os.MkdirAll(srcDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(srcDir, filepath.Join(dir, "out2"), 0600); err == nil {
		t.Fatal("copyFile(src is dir): want error, got nil")
	}

	// 成功路径（Sync 返回 nil）
	dst := filepath.Join(dir, "ok.bin")
	if err := copyFile(src, dst, 0600); err != nil {
		t.Fatalf("copyFile ok: %v", err)
	}
}

func TestCollectGuacRecordingErrorBranches(t *testing.T) {
	// MkdirAll 失败：recordingsDir 的父路径上有个同名文件
	s := newTestServerWithDB(t)
	s.cfg = &config.Config{
		Database: &config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "sub", "cockpit.db")},
	}
	recDir := s.recordingsDir() // <tmp>/sub/recordings
	if err := os.MkdirAll(filepath.Dir(recDir), 0700); err != nil {
		t.Fatal(err)
	}
	// 让 recordings 路径本身是文件 → MkdirAll(<dir of dst>) 失败
	if err := os.WriteFile(recDir, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	guacDir := t.TempDir()
	t.Setenv("GUACD_RECORDING_PATH", guacDir)
	gs := &GuacamoleSession{ID: "sess-mkdir-fail", Created: time.Now()}
	if err := os.WriteFile(filepath.Join(guacDir, gs.ID+".guac"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	s.collectGuacRecording(gs) // 只验证不 panic、错误分支被走到

	// copyFile 失败：src 是目录（Stat 成功、Open/Copy 失败）
	s2 := newTestServerWithDB(t)
	s2.cfg = &config.Config{Database: &config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "cockpit.db")}}
	guacDir2 := t.TempDir()
	t.Setenv("GUACD_RECORDING_PATH", guacDir2)
	gs2 := &GuacamoleSession{ID: "sess-copy-fail", Created: time.Now()}
	if err := os.MkdirAll(filepath.Join(guacDir2, gs2.ID+".guac"), 0700); err != nil {
		t.Fatal(err)
	}
	s2.collectGuacRecording(gs2)

	// FinishTerminalRecording 失败：db 已关闭
	s3 := newTestServerWithDB(t)
	s3.cfg = &config.Config{Database: &config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "cockpit.db")}}
	guacDir3 := t.TempDir()
	t.Setenv("GUACD_RECORDING_PATH", guacDir3)
	gs3 := &GuacamoleSession{ID: "sess-finish-fail", Created: time.Now()}
	if err := os.WriteFile(filepath.Join(guacDir3, gs3.ID+".guac"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	s3.db.Close()
	s3.collectGuacRecording(gs3)
}

func TestGuacamoleUpgradeFail(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	covStartGuacd(t) // guacd 可连，走到 Upgrade 才失败

	rec := covRec()
	s.handleGuacamoleWebSocket(rec, guacReqWithTicket(covTicket(t, s, map[string]string{
		"agent_id": "a", "host": "10.0.0.9", "port": "3389", "protocol": "rdp",
	})))
	// httptest.ResponseRecorder 无 http.Hijacker → Upgrade 失败
	if rec.Code == http.StatusSwitchingProtocols {
		t.Fatalf("upgrade on ResponseRecorder: want failure, got 101")
	}
}

func TestGuacamoleHandshakeWriteFail(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	// guacd 接受连接后立即关闭 → handshake 写失败
	t.Setenv("GUACD_ADDR", startClosingGuacd(t))

	conn, _, _ := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole",
		covTicket(t, s, map[string]string{
			"agent_id": "a", "host": "10.0.0.9", "port": "3389", "protocol": "rdp",
		}))
	t.Cleanup(func() { conn.Close() })
	// net.Pipe 同步无缓冲：writeWS(uuid) 阻塞到读完，必须先消费首帧放行
	_, _, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	// 分支触发即达成覆盖（select 响应后 RST → handshake write 失败 → 记日志）；
	// handler 退出时机受 RST 传播影响，不强等（net.Pipe deadline 10s 兜底）
	time.Sleep(500 * time.Millisecond)
}

func TestGuacamoleCreateRecordingMetaFail(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	g := covStartGuacd(t)
	t.Setenv("GUACD_RECORDING_PATH", t.TempDir())
	// 开录制（默认开）→ 走 CreateTerminalRecording；db 关闭使其失败
	if err := s.db.CreateTerminalRecording(&storage.TerminalRecording{
		SessionID: "seed", Username: "u", Protocol: "rdp", Format: "guac", StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	s.db.Close()

	conn, _, _ := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole",
		covTicket(t, s, map[string]string{
			"agent_id": "a", "host": "10.0.0.9", "port": "3389", "protocol": "rdp",
		}))
	t.Cleanup(func() { conn.Close() })
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	covWaitGone(t, "handshake", func() bool {
		return strings.Contains(g.handshakeText(), "7.connect")
	})
}

func TestWsToGuacdWriteFail(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	g := covStartGuacd(t)

	conn, _, _ := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole",
		covTicket(t, s, map[string]string{
			"agent_id": "a", "host": "10.0.0.9", "port": "3389", "protocol": "rdp",
		}))
	t.Cleanup(func() { conn.Close() })
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	covWaitGone(t, "handshake", func() bool {
		return strings.Contains(g.handshakeText(), "7.connect")
	})
	// guacd 端关闭连接后，浏览器发数据 → gs.guacd.Write 失败
	g.mu.Lock()
	if g.conn != nil {
		if tc, ok := g.conn.(interface{ SetLinger(int) error }); ok {
			_ = tc.SetLinger(0) // RST：Write 立即失败
		}
		_ = g.conn.Close()
	}
	g.mu.Unlock()
	_ = conn.WriteMessage(1, []byte("4.nop,1.x"))
	// 分支触发即达成覆盖；handler 退出时机不强等
	time.Sleep(500 * time.Millisecond)
}

func TestHandleTicketCreateWithDomain(t *testing.T) {
	s := covNewServer(t)
	s.cfg = &config.Config{RemoteControl: &config.RemoteControlConfig{AllowArbitraryTarget: true}}
	covFakeAgent(t, s, "covgap-a", nil, nil)
	user := covSeedUser(t, s, "covgap-remote", "cov-pass-123", "admin")

	body := `{"agent_id":"covgap-a","host":"10.0.0.9","port":3389,"protocol":"rdp","domain":"CORP"}`
	req := covAuthReq(http.MethodPost, "/api/remote/tickets", strings.NewReader(body),
		user.ID, "covgap-remote", "admin")
	rec := covCallAuth(s, s.handleTicketCreate, req)
	covWantCode(t, "ticket create with domain", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "ticket") {
		t.Fatalf("handleTicketCreate domain: body = %s", rec.Body.String())
	}
}

func TestStartWithDNSProviderEnabled(t *testing.T) {
	t.Chdir(t.TempDir())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	oldValidate := storageValidateKey
	storageValidateKey = func() error { return nil }
	t.Cleanup(func() { storageValidateKey = oldValidate })

	cfg := &config.Config{
		Server: &config.ServerConfig{Host: "127.0.0.1", Port: port},
		JWT:    &config.JWTConfig{Secret: "cov-dns-secret", Expiration: time.Hour},
		DNS: &config.DNSConfig{
			Provider: "cloudflare",
			Cloudflare: &config.CloudflareDNSConfig{APIToken: "cov-cf-token"},
		},
	}
	s := NewServer(cfg)
	defer s.Shutdown()

	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "cov-dns-strong-pass")
	if err := s.Start(); err == nil {
		t.Fatal("Start: want error (port occupied), got nil")
	}
	if s.dns == nil {
		t.Fatal("Start: dns provider not constructed")
	}
	if got := s.dnsProviderName(); got != "cloudflare" {
		t.Fatalf("dnsProviderName = %q, want cloudflare", got)
	}
}

// startClosingGuacd 假 guacd：回 select 响应后立即 RST 关闭——让网关的
// select write + read select 成功、handshake（size+connect）write 失败，
// 确定性触发 handshake-write-fail 分支（不依赖 RST 传播时序）。
func startClosingGuacd(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// 先回 select 响应（真 guacd 行为）：网关 read select 成功后
			// 才发 size+connect——此时 RST 让 handshake write 失败
			_, _ = c.Write([]byte("6.select,8.hostname,4.port;"))
			// SetLinger(0) 让 Close 发 RST 而非 FIN：
			// 否则首次 Write 进内核缓冲不报错，handshake 写失败分支走不到
			if tc, ok := c.(*net.TCPConn); ok {
				_ = tc.SetLinger(0)
			}
			_ = c.Close()
		}
	}()
	return ln.Addr().String()
}
