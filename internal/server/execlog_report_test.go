package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// execLogFixture 组一份 exec_log 负载（三条覆盖：成功带 target/params、
// 失败带 error 且 target 缺省回落 execId、未知状态按 failure 收）。
func execLogFixture(now int64) map[string]interface{} {
	return map[string]interface{}{
		"entries": []map[string]interface{}{
			{
				"id": "exec-1", "time": now, "provider": "service", "action": "restart",
				"target": "nginx.service", "status": "success", "durationMs": 812,
				"params": map[string]string{"unit": "nginx.service"},
			},
			{
				"id": "exec-2", "time": now, "provider": "file", "action": "delete",
				"status": "failure", "error": "permission denied",
			},
			{
				"id": "exec-3", "time": now, "provider": "job", "action": "run",
				"status": "weird",
			},
		},
	}
}

// TestHandleExecLogReportWrites 全链：负载 → 三条审计落库，逐条断言
// action/resource/resourceID/status 与 details 内容。
func TestHandleExecLogReportWrites(t *testing.T) {
	db := testServerDB(t)
	s := &Server{db: db, audit: audit.NewLogger(db)}
	agent := NewAgent("exec-x", nil)

	now := time.Now().Unix()
	s.handleExecLogReport(agent, &protocol.Message{Type: protocol.MessageTypeExecLog, Payload: execLogFixture(now)})

	logs, _, err := db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionExec})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 3 {
		t.Fatalf("exec logs = %d, want 3", len(logs))
	}
	rows := map[string]struct {
		resource   string
		resourceID string
		status     string
		details    map[string]interface{}
	}{}
	for _, l := range logs {
		var d map[string]interface{}
		if err := json.Unmarshal([]byte(l.Details), &d); err != nil {
			t.Fatalf("details not json: %v", err)
		}
		execID, _ := d["execId"].(string)
		rows[execID] = struct {
			resource   string
			resourceID string
			status     string
			details    map[string]interface{}
		}{l.Resource, l.ResourceID, l.Status, d}
		if l.Username != "cockpit-agent" || l.UserAgent != "cockpit-agent" {
			t.Fatalf("exec %s source = %s/%s, want cockpit-agent both", execID, l.Username, l.UserAgent)
		}
	}

	r1 := rows["exec-1"]
	if r1.resource != "service" || r1.resourceID != "nginx.service" || r1.status != "success" {
		t.Fatalf("exec-1 = %+v", r1)
	}
	if r1.details["target"] != "nginx.service" || r1.details["durationMs"] != float64(812) {
		t.Fatalf("exec-1 details = %+v", r1.details)
	}
	if _, ok := r1.details["error"]; ok {
		t.Fatalf("exec-1 success must not carry error: %+v", r1.details)
	}
	params, ok := r1.details["params"].(map[string]interface{})
	if !ok || params["unit"] != "nginx.service" {
		t.Fatalf("exec-1 params = %+v", r1.details["params"])
	}

	r2 := rows["exec-2"]
	if r2.resource != "file" || r2.status != "failure" {
		t.Fatalf("exec-2 = %+v", r2)
	}
	if r2.resourceID != "exec-2" { // target 缺省回落 execId
		t.Fatalf("exec-2 resourceID = %q, want exec-2", r2.resourceID)
	}
	if r2.details["error"] != "permission denied" {
		t.Fatalf("exec-2 details = %+v", r2.details)
	}
	if _, ok := r2.details["params"]; ok {
		t.Fatalf("exec-2 must not carry params key: %+v", r2.details)
	}

	// 未知 status 按 failure 收（审计宁可误报失败）
	if r3 := rows["exec-3"]; r3.status != "failure" || r3.resource != "job" {
		t.Fatalf("exec-3 = %+v", r3)
	}
}

// TestHandleExecLogReportGuards nil 审计器/nil db 双守卫分支、nil 负载与
// 非法负载类型不落行不 panic。
func TestHandleExecLogReportGuards(t *testing.T) {
	db := testServerDB(t)
	agent := NewAgent("exec-x", nil)

	s := &Server{db: db} // audit nil：第一守卫
	s.handleExecLogReport(agent, &protocol.Message{Type: protocol.MessageTypeExecLog, Payload: execLogFixture(1)})

	s2 := &Server{audit: audit.NewLogger(db)} // db nil：第二守卫
	s2.handleExecLogReport(agent, &protocol.Message{Type: protocol.MessageTypeExecLog, Payload: execLogFixture(1)})

	s3 := &Server{db: db, audit: audit.NewLogger(db)}
	s3.handleExecLogReport(agent, &protocol.Message{Type: protocol.MessageTypeExecLog}) // nil payload
	s3.handleExecLogReport(agent, &protocol.Message{
		Type:    protocol.MessageTypeExecLog,
		Payload: map[string]interface{}{"entries": "not-an-array"},
	})

	logs, _, err := db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionExec})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 {
		t.Fatalf("exec logs = %d, want 0", len(logs))
	}

	// 空批合法：不落行
	s3.handleExecLogReport(agent, &protocol.Message{
		Type:    protocol.MessageTypeExecLog,
		Payload: map[string]interface{}{"entries": []interface{}{}},
	})
	logs, _, _ = db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionExec})
	if len(logs) != 0 {
		t.Fatalf("empty batch rows = %d, want 0", len(logs))
	}
}

// TestHandleExecLogReportDBClosed 落库失败（库已关）走错误分支不 panic。
func TestHandleExecLogReportDBClosed(t *testing.T) {
	db := testServerDB(t)
	s := &Server{db: db, audit: audit.NewLogger(db)}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s.handleExecLogReport(NewAgent("exec-x", nil), &protocol.Message{
		Type:    protocol.MessageTypeExecLog,
		Payload: execLogFixture(time.Now().Unix()),
	})
}

// TestHandleMessageDispatchesExecLog 分发器接线：exec_log 经 handleMessage
// 路由到归集（与直调同效果）。
func TestHandleMessageDispatchesExecLog(t *testing.T) {
	db := testServerDB(t)
	s := &Server{db: db, audit: audit.NewLogger(db)}
	s.handleMessage(NewAgent("exec-x", nil), &protocol.Message{
		Type:    protocol.MessageTypeExecLog,
		Payload: map[string]interface{}{"entries": []interface{}{}},
	})
	logs, _, _ := db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionExec})
	if len(logs) != 0 {
		t.Fatalf("rows = %d, want 0", len(logs))
	}
}
