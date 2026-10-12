package server

// cov_remote_session_lifecycle_test.go 覆盖远控会话生命周期登记（D-2026-10-12-4）：
// RemoteSessionManager.Register 显式 ID 分支 / terminal 会话台账接线
// （pending→connected→closed）/ guacamole 会话台账接线 / terminateLiveSession
// 三分支（terminal 命中 / guac 命中 / 均未命中）。

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// covClearGuacSessions 清理 guacSessions 全局表（与 covClearSessions 对称）
func covClearGuacSessions() {
	guacSessionsMu.Lock()
	guacSessions = make(map[string]*GuacamoleSession)
	guacSessionsMu.Unlock()
}

// covRemoteSessionByID 从 remoteSessions 台账取会话
func covRemoteSessionByID(t *testing.T, s *Server, id string) *RemoteSession {
	t.Helper()
	sess, ok := s.remoteSessions.Get(id)
	if !ok {
		t.Fatalf("remote session %q not found", id)
	}
	return sess
}

// ============ Register 显式 ID ============

func TestRemoteSessionRegisterExplicitID(t *testing.T) {
	mgr := NewRemoteSessionManager()
	sess := mgr.Register("my-explicit-id", "u1", "admin", RemoteSessionRequest{
		AgentID:  "agent-1",
		Protocol: protocol.RemoteProtocolSSH,
		Host:     "127.0.0.1",
		Port:     22,
	})
	if sess.ID != "my-explicit-id" {
		t.Fatalf("Register explicit ID = %q, want my-explicit-id", sess.ID)
	}
	if sess.Status != RemoteSessionStatusPending {
		t.Fatalf("status = %s, want pending", sess.Status)
	}
	// Create 走 id=="" 分支生成 UUID
	sess2 := mgr.Create("u1", "admin", RemoteSessionRequest{
		AgentID: "agent-1", Protocol: "ssh", Host: "h", Port: 22,
	})
	if sess2.ID == "" || sess2.ID == "my-explicit-id" {
		t.Fatalf("Create ID = %q, want generated UUID", sess2.ID)
	}
}

// ============ Terminal 会话台账接线 ============

func TestCovTerminalSessionLifecycle(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	agent := covRegisterBareAgent(t, s, "agent-lc")

	conn, _, tDone := covDirectWSJoined(t, s.handleTerminalWebSocket, "/api/remote/terminal",
		covTerminalTicket(t, s, "agent-lc"))
	t.Cleanup(func() { conn.Close() })

	var connect map[string]interface{}
	covWSReadJSON(t, conn, &connect)

	// 从 remoteSessions 台账取 sessionID（connect 消息不含 session_id）
	sessions := s.remoteSessions.List()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 remote session, got %d", len(sessions))
	}
	sessionID := sessions[0].ID

	// 等 agent 收到 proxy_new → UpdateStatus(connected)
	newMsg := covAgentRecv(t, agent, "proxy_new")
	_ = newMsg

	// connected：proxy_new 已下发
	sess := covRemoteSessionByID(t, s, sessionID)
	if sess.Status != RemoteSessionStatusConnected {
		t.Fatalf("after proxy_new: status = %s, want connected", sess.Status)
	}

	// 关闭 → closeTerminalSession → UpdateStatus(closed)
	if err := conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(3000, "test-close"), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	covWaitGone(t, "terminal session lifecycle cleanup", func() bool {
		terminalSessionsMu.Lock()
		defer terminalSessionsMu.Unlock()
		_, ok := terminalSessions[sessionID]
		return !ok
	})

	// closed：closeTerminalSession 已回填
	sess = covRemoteSessionByID(t, s, sessionID)
	if sess.Status != RemoteSessionStatusClosed {
		t.Fatalf("after close: status = %s, want closed", sess.Status)
	}

	covWaitHandlerExit(t, "terminal lifecycle handler exit", tDone)
}

// ============ Guacamole 会话台账接线 ============

func TestCovGuacamoleSessionLifecycle(t *testing.T) {
	defer covClearSessions()
	defer covClearGuacSessions()
	s := covRemoteSetup(t)
	g := covStartGuacd(t)

	conn, _, _ := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole",
		covTicket(t, s, map[string]string{
			"agent_id": "a", "host": "10.0.0.9", "port": "3389", "protocol": "rdp",
		}))
	t.Cleanup(func() { conn.Close() })

	// 读 uuid 帧 → 取 sessionID
	_, frame, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	// uuid 帧格式: "0.,<len>.<uuid>;"（INTERNAL_DATA 指令，opcode 为空）
	uuidFrame := string(frame)
	if !strings.HasSuffix(uuidFrame, ";") {
		t.Fatalf("uuid frame = %q, want ;-terminated", uuidFrame)
	}
	// 去掉 "0.," 前缀和 ";" 后缀，再按 "." 拆出 <len> 和 <uuid>
	trimmed := strings.TrimSuffix(uuidFrame, ";")
	if !strings.HasPrefix(trimmed, "0.,") {
		t.Fatalf("uuid frame = %q, want 0., prefix", uuidFrame)
	}
	lenAndUUID := trimmed[3:] // 去掉 "0.,"
	dotIdx := strings.IndexByte(lenAndUUID, '.')
	if dotIdx < 0 {
		t.Fatalf("uuid frame = %q, want <len>.<uuid>", uuidFrame)
	}
	sessionID := lenAndUUID[dotIdx+1:]

	// pending：Register 已调用
	sess := covRemoteSessionByID(t, s, sessionID)
	if sess.Status != RemoteSessionStatusPending {
		t.Fatalf("after uuid frame: status = %s, want pending", sess.Status)
	}

	// 等握手完成 → auditRemoteStart → UpdateStatus(connected)
	covWaitGone(t, "guac handshake for lifecycle", func() bool {
		return strings.Contains(g.handshakeText(), "7.connect")
	})

	// connected：auditRemoteStart 已落
	sess = covRemoteSessionByID(t, s, sessionID)
	if sess.Status != RemoteSessionStatusConnected {
		t.Fatalf("after handshake: status = %s, want connected", sess.Status)
	}

	// 关闭 → closeGuacamoleSession → UpdateStatus(closed) + 移出 guacSessions
	if err := conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(3000, "test-close"), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	time.Sleep(500 * time.Millisecond)

	// closed：closeGuacamoleSession 已回填
	sess = covRemoteSessionByID(t, s, sessionID)
	if sess.Status != RemoteSessionStatusClosed {
		t.Fatalf("after close: status = %s, want closed", sess.Status)
	}

	// guacSessions 已移出
	guacSessionsMu.Lock()
	_, stillInGuac := guacSessions[sessionID]
	guacSessionsMu.Unlock()
	if stillInGuac {
		t.Fatal("guacSessions still contains closed session")
	}
}

// ============ terminateLiveSession 三分支 ============

func TestCovTerminateLiveSessionTerminal(t *testing.T) {
	defer covClearSessions()
	s := covRemoteSetup(t)
	agent := covRegisterBareAgent(t, s, "agent-term-term")

	conn, _, tDone := covDirectWSJoined(t, s.handleTerminalWebSocket, "/api/remote/terminal",
		covTerminalTicket(t, s, "agent-term-term"))
	t.Cleanup(func() { conn.Close() })

	var connect map[string]interface{}
	covWSReadJSON(t, conn, &connect)

	// 从 remoteSessions 台账取 sessionID
	sessions := s.remoteSessions.List()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 remote session, got %d", len(sessions))
	}
	sessionID := sessions[0].ID

	covAgentRecv(t, agent, "proxy_new")

	// DELETE 命中 terminal 分支 → terminateLiveSession → closeTerminalSession
	delReq := covReq(http.MethodDelete, "/api/remote/sessions/"+sessionID, nil)
	delReq = covAuthReq(http.MethodDelete, "/api/remote/sessions/"+sessionID, nil, "1", "u", "admin")
	delRec := covRec()
	s.handleRemoteSession(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE terminal live session: code = %d, want %d", delRec.Code, http.StatusNoContent)
	}

	covWaitHandlerExit(t, "terminal terminate handler exit", tDone)
}

func TestCovTerminateLiveSessionGuac(t *testing.T) {
	defer covClearSessions()
	defer covClearGuacSessions()
	s := covRemoteSetup(t)
	g := covStartGuacd(t)

	conn, _, _ := covDirectWSJoined(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole",
		covTicket(t, s, map[string]string{
			"agent_id": "a", "host": "10.0.0.9", "port": "3389", "protocol": "rdp",
		}))
	t.Cleanup(func() { conn.Close() })

	_, frame, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read uuid frame: %v", err)
	}
	uuidFrame := string(frame)
	trimmed := strings.TrimSuffix(uuidFrame, ";")
	lenAndUUID := trimmed[3:] // 去掉 "0.,"
	dotIdx := strings.IndexByte(lenAndUUID, '.')
	sessionID := lenAndUUID[dotIdx+1:]

	covWaitGone(t, "guac handshake for terminate", func() bool {
		return strings.Contains(g.handshakeText(), "7.connect")
	})

	// DELETE 命中 guac 分支 → terminateLiveSession → closeGuacamoleSession
	delReq := covAuthReq(http.MethodDelete, "/api/remote/sessions/"+sessionID, nil, "1", "u", "admin")
	delRec := covRec()
	s.handleRemoteSession(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE guac live session: code = %d, want %d", delRec.Code, http.StatusNoContent)
	}

	time.Sleep(500 * time.Millisecond)
}

func TestCovTerminateLiveSessionNeither(t *testing.T) {
	defer covClearSessions()
	defer covClearGuacSessions()
	s := covRemoteSetup(t)
	covRegisterBareAgent(t, s, "agent-neither")

	// 仅登记台账（无活会话）→ DELETE 走软删分支
	sess := s.remoteSessions.Create("1", "u", RemoteSessionRequest{
		AgentID: "agent-neither", Protocol: protocol.RemoteProtocolSSH, Host: "h", Port: 22,
	})

	delReq := covAuthReq(http.MethodDelete, "/api/remote/sessions/"+sess.ID, nil, "1", "u", "admin")
	delRec := covRec()
	s.handleRemoteSession(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE ledger-only session: code = %d, want %d", delRec.Code, http.StatusNoContent)
	}

	// 确认已删除
	if _, ok := s.remoteSessions.Get(sess.ID); ok {
		t.Fatal("session should be deleted")
	}
}
