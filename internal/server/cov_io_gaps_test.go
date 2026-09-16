package server

// cov_io_gaps_test.go 补测剩余 I/O 错误分支：认证路由分发（TOTP 设置
// 三路由）、已关闭 agent 的发送、审计 CSV 导出中途写失败、三类远控
// sendLoop 对 nil 连接的 panic 恢复、keepalive ping 写死连接失败，
// 以及备份调度循环 tick 与任务跟踪超时（注入短节奏）。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// ============ /api/ 认证路由分发（totp 设置三路由只经此处注册） ============

func TestCovAuthRouteDispatch(t *testing.T) {
	s := covLoopServer(t)
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	// totp/generate|enable|disable 无 JWT → 中间件 401（分发块本身命中）
	for _, path := range []string{"/api/auth/totp/generate", "/api/auth/totp/enable", "/api/auth/totp/disable"} {
		rec := covRec()
		mux.ServeHTTP(rec, covReq(http.MethodPost, path, strings.NewReader("{}")))
		covWantCode(t, "POST "+path, rec, http.StatusUnauthorized)
	}

	// 前缀命中但非白名单路径 → 前缀守卫直接 return（什么都不写，默认 200）
	rec := covRec()
	mux.ServeHTTP(rec, covReq(http.MethodPost, "/api/auth/logout", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("unrouted auth path = %d (body %d bytes), want silent 200", rec.Code, rec.Body.Len())
	}

	// 普通路径 → 走认证中间件（同样 401）
	rec = covRec()
	mux.ServeHTTP(rec, covReq(http.MethodGet, "/api/agents", nil))
	covWantCode(t, "GET /api/agents", rec, http.StatusUnauthorized)
}

// ============ 已关闭 agent 的 SendWithTimeout ============

func TestCovAgentClosedSendWithTimeout(t *testing.T) {
	a := NewAgent("agent-closed", nil)
	a.Close()
	msg := protocol.NewMessage(protocol.MessageTypeRPCRequest, nil)
	if err := a.SendWithTimeout(msg, time.Second); err == nil ||
		!strings.Contains(err.Error(), "agent-closed is closed") {
		t.Errorf("closed agent err = %v", err)
	}
}

// covFailResp 断崖式 ResponseWriter：分配额度用尽后所有 Write 失败
type covFailResp struct {
	*httptest.ResponseRecorder
	written int
	quota   int
}

func (w *covFailResp) Write(p []byte) (int, error) {
	if w.written+len(p) > w.quota {
		return 0, errCovWriteFail
	}
	w.written += len(p)
	return w.ResponseRecorder.Write(p)
}

var errCovWriteFail = errors.New("cov: response writer quota exceeded")

// ============ 审计 CSV 导出：行写中途失败 ============

func TestCovAuditExportRowWriteError(t *testing.T) {
	s := covLoopServer(t)
	logger := audit.NewLogger(s.db)

	// 25 行 × 长 details：csv bufio(4096) 必在行循环中触发底层写失败
	details := strings.Repeat("d", 300)
	for i := 0; i < 25; i++ {
		if err := logger.LogSuccess("cov-user", "login", "auth", "",
			details, "127.0.0.1", "cov-agent"); err != nil {
			t.Fatalf("seed audit log %d: %v", i, err)
		}
	}

	w := &covFailResp{ResponseRecorder: covRec(), quota: 0}
	s.handleAuditLogsExport(w, covReq(http.MethodGet, "/api/audit/logs/export", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("row write failure code = %d, want 500", w.Code)
	}
}

// ============ 三类 sendLoop：nil ClientWS → panic 恢复 ============

func TestCovSendLoopsPanicOnNilClientWS(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)

	d := &DesktopSession{ID: "cov-nil-d", UserID: "1", Username: "cov",
		AgentID: "agent-nil", Target: "h:5900", ConnID: "cov-cd",
		CreatedAt: time.Now(), LastActive: time.Now(), done: make(chan struct{})}
	go s.desktopSendLoop(d)

	tm := &TerminalSession{ID: "cov-nil-t", UserID: "1", Username: "cov",
		AgentID: "agent-nil", Protocol: "ssh", Host: "h", Port: 22, ConnID: "cov-ct",
		CreatedAt: time.Now(), LastActive: time.Now(), done: make(chan struct{})}
	go s.terminalSendLoop(tm)

	v := &VNCSession{ID: "cov-nil-v", UserID: "1", Username: "cov",
		AgentID: "agent-nil", Target: "h:5900", ConnID: "cov-cv",
		CreatedAt: time.Now(), LastActive: time.Now(), done: make(chan struct{})}
	go s.vncSendLoop(v)

	for desc, done := range map[string]chan struct{}{"desktop": d.done, "terminal": tm.done, "vnc": v.done} {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Errorf("sendLoop(nil ws) did not exit via panic-recover: %s", desc)
		}
	}
}

// ============ keepalive ping 写死连接失败（覆盖 ping 错误分支） ============

func TestCovKeepalivePingDeadConn(t *testing.T) {
	covShortIntervals(t) // desktop/terminal keepalive 200ms
	defer covClearSessions()
	s := covRemoteSetup(t)

	// LastActive 为当前时间：确保走 ping 失败退出而非 30 分钟超时
	d := &DesktopSession{ID: "cov-ping-d", UserID: "1", Username: "cov",
		AgentID: "agent-ping", Target: "h:5900", ConnID: "cov-pd",
		ClientWS:  covDeadWS(t, s),
		CreatedAt: time.Now(), LastActive: time.Now(), done: make(chan struct{})}
	tm := &TerminalSession{ID: "cov-ping-t", UserID: "1", Username: "cov",
		AgentID: "agent-ping", Protocol: "ssh", Host: "h", Port: 22, ConnID: "cov-pt",
		ClientWS:  covDeadWS(t, s),
		CreatedAt: time.Now(), LastActive: time.Now(), done: make(chan struct{})}

	exited := make(chan struct{}, 2)
	go func() { s.desktopKeepaliveLoop(d); exited <- struct{}{} }()
	go func() { s.terminalKeepaliveLoop(tm); exited <- struct{}{} }()

	for i := 0; i < 2; i++ {
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			t.Fatalf("keepalive loop did not exit on ping failure")
		}
	}
}

// ============ 备份调度循环 tick / 任务跟踪超时（注入短节奏） ============

func TestCovBackupLoopTick(t *testing.T) {
	save := backupCheckInterval
	backupCheckInterval = 50 * time.Millisecond
	t.Cleanup(func() { backupCheckInterval = save })

	s := covLoopServer(t)
	// 到期配置 + 离线 agent：tick 分发后记失败终态（可轮询断言）
	cfg := newBackupCfg("ghost-agent", "tick", "every:1h", true)
	cfg.NextRunAt = time.Now().Add(-time.Hour).Unix()
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}

	exited := covSpawnLoop(s.startBackupLoop)
	covWaitGone(t, "backup tick dispatched", func() bool {
		got, err := s.db.GetBackupConfig(cfg.ID)
		return err == nil && got.LastStatus == "failed"
	})
	s.cancel()
	covWaitExit(t, "backupLoop tick", exited)
}

func TestCovTrackBackupTaskDeadline(t *testing.T) {
	saveTO, savePoll := backupTrackTimeout, backupTrackInterval
	backupTrackTimeout, backupTrackInterval = 80*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { backupTrackTimeout, backupTrackInterval = saveTO, savePoll })

	s := covLoopServer(t)
	covFakeAgent(t, s, "agent-stuck", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		if method == "backup.run" {
			return covOKPayload(map[string]interface{}{"taskId": "task-stuck"})
		}
		return covOKPayload(map[string]interface{}{"status": "running"}) // 永远 running
	})
	cfg := covSeedBackupCfg(t, s, "agent-stuck", "stuck")
	runID, err := s.startBackupRun(cfg, "manual")
	if err != nil {
		t.Fatalf("startBackupRun: %v", err)
	}
	covWaitGone(t, "backup task timed out", func() bool {
		run, err := s.db.GetBackupRun(runID)
		if err != nil || run.Status != "failed" || run.Error != "backup task timed out" {
			return false
		}
		got, err := s.db.GetBackupConfig(cfg.ID)
		return err == nil && got.LastStatus == "failed"
	})
	// join 后台跟踪器后再恢复默认值，保证无竞争
	s.backupTrackWG.Wait()
}
