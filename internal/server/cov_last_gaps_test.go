package server

// cov_last_gaps_test.go 最后一轮：只读数据库目录触发"读成功写失败"分支、
// TOTP 无用户上下文、录制 finish 失败、备份名时间戳解析失败、
// handleProxyData 转发失败日志、desktop/vnc 非常规关闭码、
// 以及 30s 周期的 keepalive / cleanupLoop tick（注入短间隔后 -short 也跑）。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/proxy"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/gorilla/websocket"
	"github.com/pquerna/otp/totp"
)

// covROServer 构造数据库路径已知的 server（供只读 chmod 使用）
func covROServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ro.db")
	db, err := storage.Open(storage.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("open ro db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &Server{
		registry:       NewRegistry(),
		db:             db,
		audit:          audit.NewLogger(db),
		notifier:       notification.NewService(nil),
		remoteSessions: NewRemoteSessionManager(),
		ticketMgr:      NewTicketManager(),
		ctx:            ctx,
		cfg:            &config.Config{Database: &config.DatabaseConfig{Path: dbPath}},
	}
	return s, dbPath
}

// covReadOnlyDB 把数据库所在目录与文件 chmod 为只读：SELECT 仍可用，
// 任何写事务（建 journal）失败——用于覆盖"前置读成功、最终写失败"的 500 分支。
func covReadOnlyDB(t *testing.T, dir, dbPath string) {
	t.Helper()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod dir read-only: %v", err)
	}
	if err := os.Chmod(dbPath, 0o444); err != nil {
		t.Fatalf("chmod db read-only: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
		_ = os.Chmod(dbPath, 0o644)
	})
}

func TestCovReadOnlyDBWriteFailures(t *testing.T) {
	s, dbPath := covROServer(t)
	dir := filepath.Dir(dbPath)

	// ---- 可写阶段：造全量前置数据 ----
	u := covSeedUser(t, s, "covro", "cov-pass-123", "user")
	adminU := covSeedUser(t, s, "covro-admin", "cov-pass-456", "admin")
	adminNamed, gerr := s.db.GetUserByUsername("admin")
	if gerr != nil || adminNamed == nil {
		adminNamed = covSeedUser(t, s, "admin", "cov-pass-adm", "admin")
	}

	// TOTP 用户：enable 成功拿有效码（关只读后 disable 走写失败 500）
	tu := covSeedUser(t, s, "covro-totp", "cov-pass-t1", "user")
	rec := covCallAuth(s, s.handleTOTPGenerate,
		covAuthReq(http.MethodPost, "/x", nil, tu.ID, "covro-totp", "user"))
	var gen TOTPGenerateResponse
	if err := json.NewDecoder(rec.Body).Decode(&gen); err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(gen.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rec = covCallAuth(s, s.handleTOTPEnable,
		covAuthReq(http.MethodPost, "/x", strings.NewReader(`{"code":"`+code+`"}`), tu.ID, "covro-totp", "user"))
	covWantCode(t, "totp enable before ro", rec, http.StatusOK)

	// agent / 代理 / 备份 / 录制
	if err := s.db.UpsertAgent(&storage.Agent{ID: "agent-ro", Hostname: "h"}); err != nil {
		t.Fatal(err)
	}
	covFakeAgent(t, s, "agent-ro", nil, nil) // 注册表里的 agent（不依赖库）
	covSeedProxyAgent(t, s, "agent-px")
	covSeedProxy(t, s, "p-ro", 18700)
	bcfg := covSeedBackupCfg(t, s, "agent-ro", "rocfg")
	dueCfg := newBackupCfg("agent-ro", "ro-due", "every:1h", true)
	dueCfg.NextRunAt = time.Now().Add(-time.Hour).Unix()
	if err := s.db.CreateBackupConfig(dueCfg); err != nil {
		t.Fatal(err)
	}
	orphanCfg := newBackupCfg("agent-ro", "ro-orphan", "manual", true)
	if err := s.db.CreateBackupConfig(orphanCfg); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CreateBackupRun(&storage.BackupRun{
		ConfigID: orphanCfg.ID, Status: "running", StartedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	covSeedRecording(t, s, "sess-ro") // 仅元数据，无 .cast 文件

	// ---- 只读阶段 ----
	covReadOnlyDB(t, dir, dbPath)
	if err := s.db.SetSetting("cov_ro_probe", "1"); err == nil {
		t.Skipf("read-only chmod did not make sqlite writes fail; skipping")
	}

	authed := func(h http.HandlerFunc, method, target, body, uid, uname, role string) *httptest.ResponseRecorder {
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		return covCallAuth(s, h, covAuthReq(method, target, rd, uid, uname, role))
	}
	adminDo := func(h http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
		return authed(h, method, target, body, adminU.ID, "covro-admin", "admin")
	}

	// handleCurrentUserPassword：旧密码验证（读）通过 → UpdatePassword 失败
	rec = authed(s.handleCurrentUserPassword, http.MethodPut, "/x",
		`{"currentPassword":"cov-pass-123","newPassword":"cov-newpass-9"}`,
		u.ID, "covro", "user")
	covWantCode(t, "current password write fail", rec, http.StatusInternalServerError)

	// handleCurrentUserProfile：读用户成功 → UpdateUser 失败
	rec = authed(s.handleCurrentUserProfile, http.MethodPut, "/x",
		`{"email":"ro@example.com"}`, u.ID, "covro", "user")
	covWantCode(t, "profile write fail", rec, http.StatusInternalServerError)

	// handleUserDelete：管理员删他人通过权限检查 → DeleteUser 失败
	rec = authed(func(w http.ResponseWriter, r *http.Request) {
		s.handleUserDelete(w, r, u.ID)
	}, http.MethodDelete, "/x", "", adminNamed.ID, "admin", "admin")
	covWantCode(t, "user delete write fail", rec, http.StatusInternalServerError)

	// handleUserUpdate：管理员改他人 → UpdateUser 失败
	rec = authed(func(w http.ResponseWriter, r *http.Request) {
		s.handleUserUpdate(w, r, u.ID)
	}, http.MethodPut, "/x", `{"email":"z@example.com","role":"user"}`, adminU.ID, "covro-admin", "admin")
	covWantCode(t, "user update write fail", rec, http.StatusInternalServerError)

	// handleUserChangePassword：管理员免旧密码 → UpdatePassword 失败
	rec = authed(func(w http.ResponseWriter, r *http.Request) {
		s.handleUserChangePassword(w, r, u.ID)
	}, http.MethodPost, "/x", `{"new_password":"cov-pass-7"}`, adminU.ID, "covro-admin", "admin")
	covWantCode(t, "admin reset password write fail", rec, http.StatusInternalServerError)

	// handleAgentSecret：POST regenerate / DELETE remove 的写失败
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		rec = adminDo(func(w http.ResponseWriter, r *http.Request) {
			s.handleAgentSecret(w, r, "agent-ro")
		}, method, "/x", "")
		covWantCode(t, "agent secret "+method+" write fail", rec, http.StatusInternalServerError)
	}

	// handleProxyCreate / handleProxyUpdate 的写失败（前置读全通过）
	rec = adminDo(s.handleProxyCreate, http.MethodPost, "/api/proxies",
		`{"name":"ro-px","agentId":"agent-px","proxyType":"tcp","remotePort":18800,"target":"127.0.0.1:80","publicBind":true}`)
	covWantCode(t, "proxy create write fail", rec, http.StatusInternalServerError)
	rec = adminDo(s.handleProxyUpdate, http.MethodPut, "/api/proxies",
		`{"id":"p-ro","name":"renamed-ro","agentId":"agent-px","proxyType":"tcp","remotePort":18801,"target":"127.0.0.1:9","description":"d","enabled":false}`)
	covWantCode(t, "proxy update write fail", rec, http.StatusInternalServerError)

	// handleBackupConfigUpdate / Delete 的写失败
	updBody := `{"agent_id":"agent-ro","name":"rocfg2","sources":["/etc"],"dest_dir":"/tmp/ro","schedule":"manual"}`
	rec = adminDo(func(w http.ResponseWriter, r *http.Request) {
		s.handleBackupConfigUpdate(w, r, bcfg.ID)
	}, http.MethodPut, "/x", updBody)
	covWantCode(t, "backup update write fail", rec, http.StatusInternalServerError)
	rec = adminDo(func(w http.ResponseWriter, r *http.Request) {
		s.handleBackupConfigDelete(w, r, bcfg.ID)
	}, http.MethodDelete, "/x", "")
	covWantCode(t, "backup delete write fail", rec, http.StatusInternalServerError)

	// handleTOTPDisable：验码（读）通过 → DisableTOTP 写失败
	rec = authed(s.handleTOTPDisable, http.MethodPost, "/x",
		`{"code":"`+code+`"}`, tu.ID, "covro-totp", "user")
	covWantCode(t, "totp disable write fail", rec, http.StatusInternalServerError)

	// handleRecordingDelete：meta 存在、文件不存在 → DeleteTerminalRecording 失败
	rec = covRec()
	s.handleRecordingDelete(rec, covReq(http.MethodDelete, "/x", nil), "sess-ro")
	covWantCode(t, "recording delete write fail", rec, http.StatusInternalServerError)

	// registerAgentConnection：新 agent 入注册表成功 → 持久化失败仅记日志
	agent, rejection, err := s.registerAgentConnection(nil, &protocol.RegisterPayload{
		AgentID: "fresh-ro", Hostname: "fh",
	})
	if err != nil || rejection != nil || agent == nil {
		t.Fatalf("register under ro db = %v, %v, %v", agent, rejection, err)
	}
	s.registry.Unregister("fresh-ro")

	// 循环内写失败分支（仅记日志）
	s.recoverOrphanBackupRuns()  // UpdateBackupRun 失败 → continue
	s.dispatchDueBackups()       // UpdateBackupConfig 失败 → continue
	s.cleanupExpiredRecordings() // DeleteTerminalRecording 失败 → 仅日志
}

// ============ TOTP 无用户上下文 ============

func TestCovTOTPNoUserContext(t *testing.T) {
	s := covLoopServer(t)
	rec := covRec()
	s.handleTOTPEnable(rec, covReq(http.MethodPost, "/x", strings.NewReader(`{"code":"123456"}`)))
	covWantCode(t, "enable no user", rec, http.StatusUnauthorized)
	rec = covRec()
	s.handleTOTPDisable(rec, covReq(http.MethodPost, "/x", strings.NewReader(`{"code":"123456"}`)))
	covWantCode(t, "disable no user", rec, http.StatusUnauthorized)
}

// ============ 录制 finish 回调失败 ============

func TestCovRecordingFinishFailure(t *testing.T) {
	s := covLoopServer(t)
	session := &TerminalSession{
		ID: "sess-finishfail", Username: "u", AgentID: "a", Host: "h", Port: 22,
		Protocol: "ssh", CreatedAt: time.Now(),
	}
	rec := s.startRecording(session)
	if rec == nil {
		t.Fatal("recorder should start with writable db")
	}
	covCloseDB(t, s)
	rec.finish(120, 2048) // FinishTerminalRecording 失败 → 仅记日志
	rec.Close()
}

// ============ 备份名时间戳解析失败 ============

func TestCovLatestBackupBogusTimestamp(t *testing.T) {
	s := covLoopServer(t)
	// 名字满足 ^cockpit-\d{8}-\d{6}\.db$ 但时间非法（月 99）→ Parse 失败 → 零值
	bogus := filepath.Join(s.serverBackupDir(), "cockpit-99999999-999999.db")
	if err := os.MkdirAll(s.serverBackupDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bogus, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !s.latestServerBackupAt().IsZero() {
		t.Error("bogus timestamp should parse to zero time")
	}
}

// ============ handleProxyData 转发失败日志 ============

// covDeadWS 返回一个已完成握手但随即关闭的客户端连接（写必失败）
func covDeadWS(t *testing.T, s *Server) *websocket.Conn {
	t.Helper()
	conn, _ := covDirectWS(t, func(w http.ResponseWriter, r *http.Request) {
		c, err := s.upgrader.Upgrade(w, r, nil)
		if err == nil {
			c.Close()
		}
	}, "/ws", "", true)
	conn.Close()
	return conn
}

func TestCovProxyDataErrorLogs(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	s.proxyMgr = proxy.NewManager(s, s.db)
	covRegisterBareAgent(t, s, "agent-pxdata")
	agent := NewAgent("agent-pxdata", nil)

	// 终端会话：ClientWS 已死 → HandleTerminalData 写失败 → 仅记日志
	tsess := &TerminalSession{
		ID: "cov-px-t", UserID: "1", Username: "cov", AgentID: "agent-pxdata",
		Protocol: "ssh", Host: "h", Port: 22,
		ClientWS: covDeadWS(t, s), ConnID: "cov-conn-t",
		CreatedAt: time.Now(), LastActive: time.Now(), done: make(chan struct{}),
	}
	terminalSessionsMu.Lock()
	terminalSessions[tsess.ID] = tsess
	terminalByConn[tsess.ConnID] = tsess
	terminalSessionsMu.Unlock()

	s.handleProxyData(agent, protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "terminal-cov-conn-t", "connId": "cov-conn-t", "data": "x", "terminal": true,
	}))

	// VNC 会话：二进制写失败 → 仅记日志
	vsess := &VNCSession{
		ID: "cov-px-v", UserID: "1", Username: "cov", AgentID: "agent-pxdata",
		Target: "h:5900", ClientWS: covDeadWS(t, s), ConnID: "cov-conn-v",
		CreatedAt: time.Now(), LastActive: time.Now(), done: make(chan struct{}),
	}
	vncSessionsMu.Lock()
	vncSessions[vsess.ID] = vsess
	vncSessionsMu.Unlock()

	s.handleProxyData(agent, protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "vnc-cov-conn-v", "connId": "cov-conn-v", "data": "x",
	}))
}

// ============ desktop / vnc 非常规关闭码与 agent 消失 ============

func TestCovDesktopVNCUnexpectedClose(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)

	// desktop：关闭码 3000 → IsUnexpectedCloseError → 日志分支
	dAgent := covRegisterBareAgent(t, s, "agent-dclose")
	conn, _, dDone := covDirectWSJoined(t, s.handleDesktopWebSocket, "/api/remote/desktop",
		covTicket(t, s, map[string]string{
			"agent_id": "agent-dclose", "host": "127.0.0.1", "port": "3389", "protocol": "rdp",
		}))
	var connecting map[string]interface{}
	covWSReadJSON(t, conn, &connecting)
	newMsg := covAgentRecv(t, dAgent, "desktop_new")
	if newMsg.Type != protocol.MessageTypeDesktopNew {
		t.Fatalf("desktop_new type = %s", newMsg.Type)
	}
	if err := conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(3000, "cov-abnormal"), time.Now().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if m := covAgentRecv(t, dAgent, "desktop_close"); m.Type != protocol.MessageTypeDesktopClose {
		t.Fatalf("unexpected-close should still notify agent, got %s", m.Type)
	}
	// handler 退出 = keepaliveLoop 收尾完成，不留后台 goroutine 与清理竞态
	covWaitHandlerExit(t, "desktop handler after abnormal close", dDone)

	// vnc：关闭码 3000 → 日志分支 + 会话清理
	vAgent := covRegisterBareAgent(t, s, "agent-vclose")
	conn2, _, vDone := covDirectWSJoined(t, s.handleVNCWebSocket, "/api/remote/vnc",
		covTicket(t, s, map[string]string{
			"agent_id": "agent-vclose", "host": "127.0.0.1", "port": "5900", "protocol": "vnc",
		}))
	newMsg2 := covAgentRecv(t, vAgent, "proxy_new")
	connID2, _ := newMsg2.Payload["connId"].(string)
	if err := conn2.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(3000, "cov-abnormal"), time.Now().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	conn2.Close()
	covAgentRecv(t, vAgent, "proxy_close")
	covWaitGone(t, "vnc cleanup after abnormal close", func() bool { return !covVNCSessionByConn(connID2) })
	// 会话表删除先于审计落库（closeVNCSession 内部顺序），join handler 才能
	// 保证 auditRemoteEnd 的 db 写在 TempDir 清理前完成
	covWaitHandlerExit(t, "vnc handler after abnormal close", vDone)
}

func TestCovVNCAgentGoneDuringForward(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	gAgent := covRegisterBareAgent(t, s, "agent-vgone")

	conn, _, vDone := covDirectWSJoined(t, s.handleVNCWebSocket, "/api/remote/vnc",
		covTicket(t, s, map[string]string{
			"agent_id": "agent-vgone", "host": "127.0.0.1", "port": "5900", "protocol": "vnc",
		}))
	newMsg := covAgentRecv(t, gAgent, "proxy_new")
	connID, _ := newMsg.Payload["connId"].(string)

	// agent 先消失，浏览器再发二进制帧 → vncSendLoop 直接退出
	s.registry.Unregister("agent-vgone")
	if err := conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	covWaitGone(t, "vnc session cleanup after agent gone", func() bool { return !covVNCSessionByConn(connID) })
	conn.Close()
	// closeVNCSession 的审计写库在会话表删除之后，join handler 排干再返回
	covWaitHandlerExit(t, "vnc handler after agent gone", vDone)
}

// ============ 30s 周期分支（非 -short） ============

// covClearDeadline 解除 covDirectWS 设置的 10s 读写截止时间（tick 要等 30s）
func covClearDeadline(conn *websocket.Conn, raw interface{ SetDeadline(time.Time) error }) {
	_ = conn.SetReadDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Time{})
	_ = raw.SetDeadline(time.Time{})
}

// covDrain 持续读取连接直至出错（消费 keepalive ping / close 消息）
func covDrain(conn *websocket.Conn) {
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// covShortIntervals 把 30s/5s 周期与超时注入为毫秒级（须在会话/循环启动前
// 调用，ticker/After 创建时即取短值），测试结束恢复生产默认值。
func covShortIntervals(t *testing.T) {
	t.Helper()
	injected := []struct {
		ptr   *time.Duration
		short time.Duration
		prod  time.Duration
	}{
		{&cleanupLoopInterval, 200 * time.Millisecond, 30 * time.Second},
		{&desktopKeepaliveInterval, 200 * time.Millisecond, 30 * time.Second},
		{&terminalKeepaliveInterval, 200 * time.Millisecond, 30 * time.Second},
		{&vncKeepaliveInterval, 200 * time.Millisecond, 30 * time.Second},
		{&callAgentSendTimeout, 100 * time.Millisecond, 5 * time.Second},
		{&callAgentTimeout, 300 * time.Millisecond, 30 * time.Second},
	}
	for _, s := range injected {
		*s.ptr = s.short
	}
	t.Cleanup(func() {
		for _, s := range injected {
			*s.ptr = s.prod
		}
	})
}

func TestCovSlowKeepaliveAndCleanupTicks(t *testing.T) {
	// 30s 周期分支：注入短间隔后无需跳过 -short（会话/循环启动前注入，
	// ticker 创建即取短值；defer 恢复默认生产值）
	covShortIntervals(t)

	defer covClearSessions()
	s := covRemoteSetup(t)
	tAgent := covRegisterBareAgent(t, s, "agent-ka-t")
	dAgent := covRegisterBareAgent(t, s, "agent-ka-d")
	vAgent := covRegisterBareAgent(t, s, "agent-ka-v")
	silent := covRegisterBareAgent(t, s, "agent-ka-silent")
	silent.Capabilities = append(silent.Capabilities, protocol.Capability{Type: "docker"})

	// 终端会话
	conn1, rec1, tDone := covDirectWSJoined(t, s.handleTerminalWebSocket, "/api/remote/terminal",
		covTerminalTicket(t, s, "agent-ka-t"))
	var connect map[string]interface{}
	covWSReadJSON(t, conn1, &connect)
	newMsg := covAgentRecv(t, tAgent, "proxy_new")
	connID1, _ := newMsg.Payload["connId"].(string)
	covClearDeadline(conn1, rec1.conn)
	go covDrain(conn1)

	// 桌面会话
	conn2, rec2, dDone := covDirectWSJoined(t, s.handleDesktopWebSocket, "/api/remote/desktop",
		covTicket(t, s, map[string]string{
			"agent_id": "agent-ka-d", "host": "127.0.0.1", "port": "3389", "protocol": "rdp",
		}))
	var connecting map[string]interface{}
	covWSReadJSON(t, conn2, &connecting)
	dNew := covAgentRecv(t, dAgent, "desktop_new")
	deskSessID, _ := dNew.Payload["sessionId"].(string)
	covClearDeadline(conn2, rec2.conn)
	go covDrain(conn2)

	// VNC 会话
	conn3, rec3, vDone := covDirectWSJoined(t, s.handleVNCWebSocket, "/api/remote/vnc",
		covTicket(t, s, map[string]string{
			"agent_id": "agent-ka-v", "host": "127.0.0.1", "port": "5900", "protocol": "vnc",
		}))
	vNew := covAgentRecv(t, vAgent, "proxy_new")
	connID3, _ := vNew.Payload["connId"].(string)
	covClearDeadline(conn3, rec3.conn)
	go covDrain(conn3)

	// 三会话 LastActive 拨回 31 分钟前：keepalive tick（30s）后走超时清理分支
	terminalSessionsMu.Lock()
	if ts := terminalByConn[connID1]; ts != nil {
		ts.LastActive = time.Now().Add(-31 * time.Minute)
	}
	terminalSessionsMu.Unlock()
	desktopSessionsMu.Lock()
	if ds := desktopSessions[deskSessID]; ds != nil {
		ds.LastActive = time.Now().Add(-31 * time.Minute)
	}
	desktopSessionsMu.Unlock()
	vncSessionsMu.Lock()
	for _, vs := range vncSessions {
		if vs.ConnID == connID3 {
			vs.LastActive = time.Now().Add(-31 * time.Minute)
		}
	}
	vncSessionsMu.Unlock()

	// 并发：CallAgent 30s 响应超时（agent 在线但永不应答）
	stackDone := make(chan int, 1)
	go func() {
		r := covRec()
		s.handleAgentStacks(r, covReq(http.MethodGet, "/x", nil), "agent-ka-silent")
		stackDone <- r.Code
	}()

	// 并发：cleanupLoop 的 30s tick（独立 server；过期心跳的 agent 触发清理日志）
	s0 := covLoopServer(t)
	stale := NewAgent("agent-stale", nil)
	stale.mu.Lock()
	stale.LastSeen = time.Now().Add(-2 * time.Minute)
	stale.mu.Unlock()
	if err := s0.registry.Register(stale); err != nil {
		t.Fatal(err)
	}
	exited0 := covSpawnLoop(s0.cleanupLoop)

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		terminalSessionsMu.Lock()
		_, tGone := terminalByConn[connID1]
		terminalSessionsMu.Unlock()
		if !tGone && !covDesktopSessionExists(deskSessID) && !covVNCSessionByConn(connID3) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if covTerminalSessionByConn(connID1) || covDesktopSessionExists(deskSessID) || covVNCSessionByConn(connID3) {
		t.Fatal("keepalive tick did not time out idle sessions within 45s")
	}

	// cleanupLoop 的 30s tick 必须实际发生（过期心跳 agent 被清掉）再退出，
	// 否则满载时 cancel 可能抢在 tick 之前把循环杀掉
	cleanupDeadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(cleanupDeadline) {
		if _, ok := s0.registry.Get("agent-stale"); !ok {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, ok := s0.registry.Get("agent-stale"); ok {
		t.Fatal("cleanupLoop tick did not remove stale agent within 45s")
	}
	s0.cancel()
	covWaitExit(t, "cleanupLoop", exited0)

	select {
	case code := <-stackDone:
		if code != http.StatusGatewayTimeout {
			t.Errorf("silent agent stacks = %d, want 504 (response timeout)", code)
		}
	case <-time.After(35 * time.Second):
		t.Fatal("handleAgentStacks on silent agent did not finish")
	}

	// 三个会话的超时清理各在 handler goroutine 里收尾（审计/录制回填写库
	// 晚于会话表删除），join 排干后再返回，避免与 TempDir 清理竞态
	covWaitHandlerExit(t, "terminal handler after keepalive timeout", tDone)
	covWaitHandlerExit(t, "desktop handler after keepalive timeout", dDone)
	covWaitHandlerExit(t, "vnc handler after keepalive timeout", vDone)
}
