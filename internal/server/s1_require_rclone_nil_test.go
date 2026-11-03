package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestS1RequireAgentRcloneNoCapability 验证 S1：agent 在线但完全没有
// backup capability 时，requireAgentRclone 必须返回 400（rclone not
// found），而不是对 nil capability 解引用 panic 成 500。
func TestS1RequireAgentRcloneNoCapability(t *testing.T) {
	s := newBackupTestServer(t)

	const agentID = "s1-no-cap"
	agent := NewAgent(agentID, nil)
	// 不添加任何 capability：GetCapability("backup") 返回 nil。
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register: %v", err)
	}

	rec := covRec()
	body := fmt.Sprintf(`{"agent_id":%q,"name":"bak","sources":["/etc"],"dest_dir":"/b","schedule":"manual","remote_dest":"r:bak"}`, agentID)
	s.handleBackupsAPI(rec, covReq(http.MethodPost, "/api/backups/configs", strings.NewReader(body)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no backup capability)", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] != "remote_dest requires rclone installed on the agent host (rclone not found)" {
		t.Errorf("error = %q, want rclone not found message", resp["error"])
	}
}
