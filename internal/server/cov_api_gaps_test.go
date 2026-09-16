package server

// 第二轮覆盖缺口补测：针对 go tool cover 块级数据定位的残余未覆盖分支。
// 涉及 api.go（handleStatus/handleUsers）、api_backups.go（task get 闭包分发）、
// api_probe.go（config PUT bad json）、api_stacks.go（handleAgentStacks 离线）、
// api_server_backup.go（config nil 字段 / delete 目录占位）、server.go（handleProxyError）。

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// ============ api.go ============
// 注：handleStatus 的 GetStats err 分支不可达——storage.GetStats 忽略全部
// Count 错误恒返回 nil error（closed db 也返回全 0 成功结果）。

// handleUsers：POST 分发到 handleUserCreate（admin 上下文 → 201）；
// 未知方法 → 405。
func TestCovUsersPostCreate(t *testing.T) {
	s := covNewServer(t)

	req := covAuthReq(http.MethodPost, "/api/users",
		strings.NewReader(`{"username":"covnew","password":"covpass123"}`), "1", "admin", "admin")
	rec := covCallAuth(s, s.handleUsers, req)
	covWantCode(t, "users create", rec, http.StatusCreated)

	// PATCH 等未知方法 → 405
	rec = covRec()
	s.handleUsers(rec, covReq(http.MethodPatch, "/api/users", nil))
	covWantCode(t, "users unknown method", rec, http.StatusMethodNotAllowed)
}

// ============ api_backups.go ============

// GET /configs/{id}/tasks/{task}：真实 config + 在线 agent → withBackupConfig
// 闭包分发到 handleBackupTaskGet（agent 应答 → 200）。
func TestCovBackupTaskGetDispatch(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"taskId": params["taskId"], "status": "success"}, ""
	})

	// 先创建 config 拿真实 id
	rec := covRec()
	s.handleBackupConfigCreate(rec, covReq(http.MethodPost, "/api/backups/configs", strings.NewReader(validBackupReq())))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create config: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var created backupConfigView
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	path := "/api/backups/configs/" + strconv.Itoa(int(created.ID)) + "/tasks/cov-task-1"
	rec = covRec()
	s.handleBackupsAPI(rec, covReq(http.MethodGet, path, nil))
	covWantCode(t, "task get ok", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "cov-task-1") {
		t.Errorf("task get body = %s", rec.Body.String())
	}
}

// ============ api_probe.go ============

// handleProbeConfigPut：bad json → 400
func TestCovProbeConfigPutBadJSON(t *testing.T) {
	s := newProbeTestServer(t)

	rec := covRec()
	s.handleProbeConfigPut(rec, covReq(http.MethodPut, "/api/probe/config", strings.NewReader("not-json")))
	covWantCode(t, "probe config bad json", rec, http.StatusBadRequest)
}

// ============ api_stacks.go ============

// handleAgentStacks：agent 离线 → CallAgent 返回 ErrAgentNotFound → 404
func TestCovAgentStacksGhostAgent(t *testing.T) {
	s := covNewServer(t)

	rec := covRec()
	s.handleAgentStacks(rec, covReq(http.MethodGet, "/api/stacks/agents/ghost", nil), "ghost")
	covWantCode(t, "agent stacks ghost", rec, http.StatusNotFound)
}

// ============ api_server_backup.go ============

// config PUT：interval/retention 均未传（nil 指针）→ validate 直接放行 → 200
func TestCovServerBackupConfigNilFields(t *testing.T) {
	s := newServerBackupTestServer(t)

	rec := covRec()
	s.handleServerBackups(rec, covReq(http.MethodPut, "/server-backups/config", strings.NewReader(`{}`)))
	covWantCode(t, "config nil fields", rec, http.StatusOK)
}

// delete：备份名被非空目录占位 → os.Remove 失败（非 NotExist）→ 500
func TestCovServerBackupDeleteDirOccupied(t *testing.T) {
	s := newServerBackupTestServer(t)
	const name = "cockpit-20260101-010203.db"
	occupied := filepath.Join(s.serverBackupDir(), name, "child")
	if err := os.MkdirAll(occupied, 0o755); err != nil {
		t.Fatal(err)
	}

	rec := covRec()
	s.handleServerBackups(rec, covReq(http.MethodDelete, "/server-backups/"+name, nil))
	covWantCode(t, "delete occupied dir", rec, http.StatusInternalServerError)
}

// ============ server.go ============

// handleProxyError：payload 字段类型非法 → DecodeProxyError err → log 分支（不 panic）
func TestCovProxyErrorDecodeFailure(t *testing.T) {
	s := covNewServer(t)
	agent := NewAgent("a1", nil)

	bad := protocol.NewMessage(protocol.MessageTypeProxyError, map[string]interface{}{
		"proxyId": 123, "error": "boom",
	})
	s.handleProxyError(agent, bad) // proxyId 非字符串 → decode err → log

	// 正常 payload（对照，确保直调路径可用）
	good := protocol.NewMessage(protocol.MessageTypeProxyError, map[string]interface{}{
		"proxyId": "p1", "connId": "c1", "error": "upstream broke",
	})
	s.handleProxyError(agent, good)
}
