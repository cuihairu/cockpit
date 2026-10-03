package server

// cov_api_routes_gaps_test.go 补 serveAPI 的路由分发块（/api 空路径、
// agents/cleanup、nas/overlay/smart 子路由）、handleAgentGet 的 DELETE 体、
// handleAgentsCleanup 全函数（sqlite 触发器注入 DELETE 失败）、
// overlay/smart 空子段 404、server.go 的 guac/terminal proxy 分支与
// readLoop 离线落库失败（closed DB）。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// TestServeAPIEmptyPath 覆盖 /api（空 path 归一化为 "/"）——路由不匹配，
// 走默认 404，不 panic
func TestServeAPIEmptyPath(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.serveAPI(rec, covReq(http.MethodGet, "/api", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api = %d, want 404", rec.Code)
	}
}

// TestServeAPIAgentSubRoutes 覆盖 nas/overlay/smart 三条子路由分发块：
// agent 不在册 → 各 handler 的 agent offline 503
func TestServeAPIAgentSubRoutes(t *testing.T) {
	s := covNewServer(t)
	for _, sub := range []string{"nas/x", "overlay/status", "smart/status"} {
		rec := covRec()
		s.serveAPI(rec, covReq(http.MethodGet, "/api/agents/agent-none/"+sub, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("GET /api/agents/agent-none/%s = %d, want 503", sub, rec.Code)
		}
	}
}

// TestAgentOverlaySmartEmptySub 覆盖 overlay/smart 分发器里 sub 为空的 404
// 分支（经 serveAPI 的尾斜杠归一化不可达，直接以 rest 调用）
func TestAgentOverlaySmartEmptySub(t *testing.T) {
	s := covNewServer(t)

	rec := covRec()
	s.handleAgentOverlayAPI(rec, covReq(http.MethodGet, "/api/agents/a1/overlay/", nil), "a1/overlay/")
	if rec.Code != http.StatusNotFound {
		t.Errorf("overlay with empty sub = %d, want 404", rec.Code)
	}

	rec = covRec()
	s.handleAgentSmartAPI(rec, covReq(http.MethodGet, "/api/agents/a1/smart/", nil), "a1/smart/")
	if rec.Code != http.StatusNotFound {
		t.Errorf("smart with empty sub = %d, want 404", rec.Code)
	}
}

// TestAgentsCleanupFullFlow 覆盖 handleAgentsCleanup 主路径：
// DB 中 online 但不在 registry 的 agent 被标 offline，随后离线 agent 被清理
func TestAgentsCleanupFullFlow(t *testing.T) {
	s := covNewServer(t)
	if err := s.db.UpsertAgent(&storage.Agent{ID: "stale", Hostname: "h", Status: "online"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertAgent(&storage.Agent{ID: "gone", Hostname: "h", Status: "offline"}); err != nil {
		t.Fatal(err)
	}

	rec := covRec()
	s.handleAgentsCleanup(rec, covReq(http.MethodPost, "/api/agents/cleanup", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cleanup = %d, body=%s", rec.Code, rec.Body.String())
	}
	// stale 先被标 offline 再连同 gone 一起清理——两个 id 都进 removed
	// 列表即证明「online 但不在 registry → 标 offline」的联动发生过
	for _, id := range []string{"stale", "gone"} {
		if !strings.Contains(rec.Body.String(), `"`+id+`"`) {
			t.Errorf("cleanup response missing removed agent %s: %s", id, rec.Body.String())
		}
	}

	// GET → 405（经 serveAPI 走 /agents/cleanup 路由分发）
	rec = covRec()
	s.serveAPI(rec, covReq(http.MethodGet, "/api/agents/cleanup", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET cleanup = %d, want 405", rec.Code)
	}
}

// TestAgentsCleanupListError 覆盖 ListAgents 错误（closed DB）→ 500。
// （CleanupOfflineAgents 的删除失败被其内部吞掉、Find 失败又被本 handler
// 前置的同表 ListAgents 遮蔽，见 known_uncoverable 登记项。）
func TestAgentsCleanupListError(t *testing.T) {
	s := covNewServer(t)
	covCloseDB(t, s)
	rec := covRec()
	s.handleAgentsCleanup(rec, covReq(http.MethodPost, "/api/agents/cleanup", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("cleanup on closed DB = %d, want 500", rec.Code)
	}
}

// TestAgentDeleteRoutes 覆盖 handleAgentGet 的 DELETE 体：
// 在册连接被拆、DB 删除成功；DB 删除失败（触发器）→ 500
func TestAgentDeleteRoutes(t *testing.T) {
	// 成功路径：registry 在册 + DB 有记录 → 拆连接、删库、200
	s := covNewServer(t)
	if err := s.db.UpsertAgent(&storage.Agent{ID: "del-1", Hostname: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := s.registry.Register(NewAgent("del-1", nil)); err != nil {
		t.Fatal(err)
	}
	rec := covRec()
	s.handleAgentGet(rec, covReq(http.MethodDelete, "/api/agents/del-1", nil), "del-1")
	if rec.Code != http.StatusOK {
		t.Errorf("DELETE agent = %d, want 200", rec.Code)
	}
	if _, ok := s.registry.Get("del-1"); ok {
		t.Error("agent still registered after DELETE")
	}

	// DB 删除失败（触发器阻断 DELETE）→ 500；agent 必须真实在库，
	// 否则 DELETE 无目标行、触发器不触发
	s2 := covNewServer(t)
	if err := s2.registry.Register(NewAgent("del-2", nil)); err != nil {
		t.Fatal(err)
	}
	if err := s2.db.UpsertAgent(&storage.Agent{ID: "del-2", Hostname: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := s2.db.Session().Exec(
		"CREATE TRIGGER block_agents_del2 BEFORE DELETE ON agents BEGIN SELECT RAISE(ABORT, 'blocked'); END",
	).Error; err != nil {
		t.Fatal(err)
	}
	rec = covRec()
	s2.handleAgentGet(rec, covReq(http.MethodDelete, "/api/agents/del-2", nil), "del-2")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("DELETE agent with blocked DELETE = %d, want 500", rec.Code)
	}
}

// TestProxyGuacBranchesViaRealDispatch 覆盖 server.go 的 guac 前缀分发：
// proxy_data 到不存在的 relay → deliver 错误仅记日志；proxy_error 同前缀
// 走 guacRelayHandleError；均不 panic
func TestProxyGuacBranchesViaRealDispatch(t *testing.T) {
	s := &Server{}
	agent := NewAgent("agent-guac-none", nil)

	s.handleProxyData(agent, protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
		"proxyId": "guac:no-such-relay", "connId": "c1", "data": []byte("x"),
	}))
	s.handleProxyClose(agent, protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
		"proxyId": "guac:no-such-relay", "connId": "c1", "reason": "r",
	}))
	s.handleProxyError(agent, protocol.NewMessage(protocol.MessageTypeProxyError, map[string]interface{}{
		"proxyId": "guac:no-such-relay", "connId": "c1", "error": "boom",
	}))
}

// TestProxyErrorTerminalForward 覆盖 terminal 前缀 proxy_error 转发给浏览器：
// 会话在册（ClientWS 已死，写失败仅被忽略）→ 分支体执行不 panic
func TestProxyErrorTerminalForward(t *testing.T) {
	defer covClearSessions()
	s := &Server{}
	covWSReady(s)

	tsess := &TerminalSession{
		ID: "cov-err-t", UserID: "1", Username: "cov", AgentID: "a", Protocol: "ssh",
		Host: "h", Port: 22, ClientWS: covDeadWS(t, s), ConnID: "cov-err-conn",
		CreatedAt: time.Now(), LastActive: time.Now(), done: make(chan struct{}),
	}
	terminalSessionsMu.Lock()
	terminalSessions[tsess.ID] = tsess
	terminalByConn[tsess.ConnID] = tsess
	terminalSessionsMu.Unlock()

	s.handleProxyError(NewAgent("a", nil), protocol.NewMessage(protocol.MessageTypeProxyError, map[string]interface{}{
		"proxyId": "terminal-cov-err-conn", "connId": "cov-err-conn", "error": "dial failed",
	}))
}

// TestReadLoopOfflineStatusDBError 覆盖 readLoop defer 里离线落库失败仅记
// 日志：真实 WS 注册后关库，客户端断开 → UpdateAgentStatus 报错不 panic
func TestReadLoopOfflineStatusDBError(t *testing.T) {
	s := covNewServer(t)
	covWSReady(s)
	httpSrv := httptest.NewServer(http.HandlerFunc(s.handleWebSocket))
	t.Cleanup(httpSrv.Close)

	conn := covDialWS(t, covWSStrip(httpSrv.URL)+"/ws", nil)
	covWSWriteJSON(t, conn, protocol.NewMessage(protocol.MessageTypeRegister, covRegisterPayload("agent-ws-off")))
	var resp protocol.Message
	covWSReadJSON(t, conn, &resp)
	if resp.Type != protocol.MessageTypeRegister {
		t.Fatalf("register response type = %s", resp.Type)
	}

	// 先关库再断开：readLoop defer 的 UpdateAgentStatus 必失败
	covCloseDB(t, s)
	conn.Close()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := s.registry.Get("agent-ws-off"); !ok {
			// Unregister 先于 UpdateAgentStatus（同一 defer 内顺序执行），
			// 给落库错误分支留出执行时间
			time.Sleep(100 * time.Millisecond)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent still registered after client close")
}

// TestServeAPITagRoutes 覆盖 serveAPI 的 /api/agent-tags 与
// /agents/{id}/tags 两条分发（case 与 tags 后缀拦截）
func TestServeAPITagRoutes(t *testing.T) {
	s := covNewServer(t)

	rec := covRec()
	s.serveAPI(rec, covReq(http.MethodGet, "/api/agent-tags", nil))
	covWantCode(t, "agent-tags via serveAPI", rec, http.StatusOK)

	rec = covRec()
	s.serveAPI(rec, covReq(http.MethodPut, "/api/agent-tags/nope",
		strings.NewReader(`{"name":"x"}`)))
	covWantCode(t, "tag put missing", rec, http.StatusNotFound)

	// /agents/{id}/tags：GET 走 assign 分发（a1 不在册也是空列表 200）
	rec = covRec()
	s.serveAPI(rec, covReq(http.MethodGet, "/api/agents/a1/tags", nil))
	covWantCode(t, "agent tags via serveAPI", rec, http.StatusOK)
}

// TestAgentsListTagSnapshotEnrichment 覆盖 handleAgentsList 的组装环：
// 每机标签注入（tagList append）+ 系统信息快照注入
func TestAgentsListTagSnapshotEnrichment(t *testing.T) {
	s := covNewServer(t)
	if err := s.db.UpsertAgent(&storage.Agent{ID: "ag-t", Hostname: "h-t"}); err != nil {
		t.Fatal(err)
	}
	tag, err := s.db.CreateTag("prod", "blue")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.SetAgentTags("ag-t", []string{tag.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpdateSystemInfoSnapshot(&storage.SystemInfoSnapshot{
		AgentID: "ag-t", OSName: "linux", OSVersion: "6.6", Arch: "amd64",
	}); err != nil {
		t.Fatal(err)
	}

	rec := covRec()
	s.handleAgentsList(rec, covReq(http.MethodGet, "/api/agents", nil))
	covWantCode(t, "agents list enriched", rec, http.StatusOK)
	body := rec.Body.String()
	if !strings.Contains(body, `"tags":[{"color":"blue"`) || !strings.Contains(body, `"name":"prod"`) {
		t.Errorf("tags not injected: %s", body)
	}
	if !strings.Contains(body, `"osName":"linux"`) || !strings.Contains(body, `"arch":"amd64"`) {
		t.Errorf("system info not injected: %s", body)
	}
}

// TestAgentsListEnrichmentErrors 标签/快照两段加载失败 → 各自 500
// （从表被删：ListAgents 查主表仍成功，精确落进对应错误分支）
func TestAgentsListEnrichmentErrors(t *testing.T) {
	// 标签加载失败
	s := covNewServer(t)
	if err := s.db.UpsertAgent(&storage.Agent{ID: "ag-e", Hostname: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Session().Exec("DROP TABLE agent_tag_assignments").Error; err != nil {
		t.Fatal(err)
	}
	rec := covRec()
	s.handleAgentsList(rec, covReq(http.MethodGet, "/api/agents", nil))
	covWantCode(t, "tags load error", rec, http.StatusInternalServerError)
	if !strings.Contains(rec.Body.String(), "Failed to load tags") {
		t.Errorf("body = %s", rec.Body.String())
	}

	// 快照加载失败
	s2 := covNewServer(t)
	if err := s2.db.UpsertAgent(&storage.Agent{ID: "ag-e2", Hostname: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := s2.db.Session().Exec("DROP TABLE system_info_snapshots").Error; err != nil {
		t.Fatal(err)
	}
	rec = covRec()
	s2.handleAgentsList(rec, covReq(http.MethodGet, "/api/agents", nil))
	covWantCode(t, "snapshots load error", rec, http.StatusInternalServerError)
	if !strings.Contains(rec.Body.String(), "Failed to load system info") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

// TestAgentsCleanupThresholdAPI 阈值三档：thresholdHours 按 last_seen 截断
// 清理（覆盖 status 残留 online 的死行），空体 = 存量全清语义；
// 坏 JSON / 越档值 → 400
func TestAgentsCleanupThresholdAPI(t *testing.T) {
	s := covNewServer(t)
	seed := []struct {
		id       string
		status   string
		lastSeen time.Time
	}{
		{"old-off", "offline", time.Now().Add(-4 * 24 * time.Hour)},
		{"dead-on", "online", time.Now().Add(-7 * 24 * time.Hour)},
		{"fresh-off", "offline", time.Now().Add(-2 * time.Hour)},
	}
	for _, x := range seed {
		if err := s.db.UpsertAgent(&storage.Agent{ID: x.id, Hostname: "h", Status: x.status}); err != nil {
			t.Fatal(err)
		}
		// BeforeCreate 强制 last_seen=now，落库后回拨到目标时刻
		if err := s.db.Session().Model(&storage.Agent{}).Where("id = ?", x.id).
			Update("last_seen", x.lastSeen).Error; err != nil {
			t.Fatal(err)
		}
	}

	// 72h 档：old-off 与 dead-on（last_seen 早于截断点）清掉，fresh-off 保留
	rec := covRec()
	s.handleAgentsCleanup(rec, covReq(http.MethodPost, "/api/agents/cleanup",
		strings.NewReader(`{"thresholdHours":72}`)))
	covWantCode(t, "cleanup 72h", rec, http.StatusOK)
	body := rec.Body.String()
	if !strings.Contains(body, `"old-off"`) || !strings.Contains(body, `"dead-on"`) {
		t.Errorf("72h cleanup should remove old-off & dead-on: %s", body)
	}
	if strings.Contains(body, `"fresh-off"`) {
		t.Errorf("72h cleanup must keep fresh-off: %s", body)
	}
	if !strings.Contains(body, `"count":2`) {
		t.Errorf("count = %s", body)
	}
	if _, err := s.db.GetAgent("fresh-off"); err != nil {
		t.Errorf("fresh-off should survive: %v", err)
	}

	// 越档值 → 400（前端三档枚举外直接拒）
	rec = covRec()
	s.handleAgentsCleanup(rec, covReq(http.MethodPost, "/api/agents/cleanup",
		strings.NewReader(`{"thresholdHours":5}`)))
	covWantCode(t, "bad threshold", rec, http.StatusBadRequest)

	// 坏 JSON → 400
	rec = covRec()
	s.handleAgentsCleanup(rec, covReq(http.MethodPost, "/api/agents/cleanup",
		strings.NewReader(`{oops`)))
	covWantCode(t, "bad json", rec, http.StatusBadRequest)
}

// TestAgentsCleanupClearTagsError 离线清理在摘标签一步失败 → handler 500
// （CleanupOfflineAgents 的 ClearAgentTags 错误不被吞，直接冒泡到 499 行）
func TestAgentsCleanupClearTagsError(t *testing.T) {
	s := covNewServer(t)
	if err := s.db.UpsertAgent(&storage.Agent{ID: "off-1", Hostname: "h", Status: "offline"}); err != nil {
		t.Fatal(err)
	}
	tag, err := s.db.CreateTag("prod", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.SetAgentTags("off-1", []string{tag.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Session().Exec(`CREATE TRIGGER assign_boom BEFORE DELETE ON agent_tag_assignments
		BEGIN SELECT RAISE(ABORT, 'assign delete boom'); END`).Error; err != nil {
		t.Fatal(err)
	}

	rec := covRec()
	s.handleAgentsCleanup(rec, covReq(http.MethodPost, "/api/agents/cleanup", nil))
	covWantCode(t, "cleanup clear tags error", rec, http.StatusInternalServerError)
	if !strings.Contains(rec.Body.String(), "Failed to cleanup agents") {
		t.Errorf("body = %s", rec.Body.String())
	}
}
