package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestRemoteSessionManagerLifecycle(t *testing.T) {
	mgr := NewRemoteSessionManager()
	session := mgr.Create("u1", "admin", RemoteSessionRequest{
		AgentID:  "agent-1",
		Protocol: protocol.RemoteProtocolSSH,
		Host:     "127.0.0.1",
		Port:     22,
	})

	if session.Status != RemoteSessionStatusPending {
		t.Fatalf("status = %s, want pending", session.Status)
	}

	if _, ok := mgr.Get(session.ID); !ok {
		t.Fatal("expected session to exist")
	}

	if err := mgr.UpdateStatus(session.ID, RemoteSessionStatusConnected, ""); err != nil {
		t.Fatalf("UpdateStatus error: %v", err)
	}

	updated, _ := mgr.Get(session.ID)
	if updated.Status != RemoteSessionStatusConnected {
		t.Fatalf("status = %s, want connected", updated.Status)
	}

	if !mgr.Delete(session.ID) {
		t.Fatal("Delete should return true")
	}
}

func TestHandleRemoteSessionCreate(t *testing.T) {
	s := newTestServerWithDB(t)
	auth.InitAdmin(s.db, "admin", "admin123")
	token, _ := auth.GenerateToken("1", "admin", "admin")
	s.registry.Register(NewAgent("agent-1", nil))

	body := []byte(`{"agentId":"agent-1","protocol":"ssh","host":"127.0.0.1","port":22}`)
	req := httptest.NewRequest(http.MethodPost, "/api/remote/sessions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	auth.Middleware(s.handleRemoteSessions)(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var session RemoteSession
	if err := json.NewDecoder(rec.Body).Decode(&session); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if session.Protocol != protocol.RemoteProtocolSSH {
		t.Fatalf("protocol = %s, want ssh", session.Protocol)
	}
}

func TestHandleRemoteSessionListAndDelete(t *testing.T) {
	s := newTestServerWithDB(t)
	auth.InitAdmin(s.db, "admin", "admin123")
	token, _ := auth.GenerateToken("1", "admin", "admin")
	s.registry.Register(NewAgent("agent-1", nil))
	session := s.remoteSessions.Create("1", "admin", RemoteSessionRequest{
		AgentID:  "agent-1",
		Protocol: protocol.RemoteProtocolSSH,
		Host:     "127.0.0.1",
		Port:     22,
	})

	listReq := httptest.NewRequest(http.MethodGet, "/api/remote/sessions", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	auth.Middleware(s.handleRemoteSessions)(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/remote/sessions/"+session.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getRec := httptest.NewRecorder()
	auth.Middleware(s.handleRemoteSession)(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/remote/sessions/"+session.ID, nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delRec := httptest.NewRecorder()
	auth.Middleware(s.handleRemoteSession)(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", delRec.Code, http.StatusNoContent)
	}
}
