package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/audit"
)

// cronReq 构造对 /api/agents/{id}/cron/{sub} 的请求
func cronReq(method, agentID, sub, body string) *http.Request {
	return httptest.NewRequest(method, "/api/agents/"+agentID+"/cron/"+sub, strings.NewReader(body))
}

func TestCronStatusAndJobsForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		if method == "cron.status" {
			return map[string]interface{}{"user": "root", "cockpitCount": 2, "externalCount": 1}, ""
		}
		return map[string]interface{}{
			"jobs": []map[string]interface{}{
				{"name": "bk", "schedule": "0 3 * * *", "command": "/opt/bk.sh", "enabled": true},
			},
			"external": "0 * * * * /usr/bin/legacy.sh",
		}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentCronAPI(rec, cronReq(http.MethodGet, "a1", "status", ""), "a1/cron/status")
	if rec.Code != http.StatusOK || gotMethod != "cron.status" {
		t.Fatalf("status: code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	var st struct {
		User         string `json:"user"`
		CockpitCount int    `json:"cockpitCount"`
	}
	json.Unmarshal(rec.Body.Bytes(), &st)
	if st.User != "root" || st.CockpitCount != 2 {
		t.Fatalf("status = %+v", st)
	}

	rec = httptest.NewRecorder()
	s.handleAgentCronAPI(rec, cronReq(http.MethodGet, "a1", "jobs", ""), "a1/cron/jobs")
	if rec.Code != http.StatusOK || gotMethod != "cron.jobs" {
		t.Fatalf("jobs: code=%d method=%s", rec.Code, gotMethod)
	}
	var list struct {
		Jobs     []map[string]interface{} `json:"jobs"`
		External string                   `json:"external"`
	}
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Jobs) != 1 || list.Jobs[0]["name"] != "bk" || !strings.Contains(list.External, "legacy.sh") {
		t.Fatalf("jobs resp = %+v", list)
	}
}

func TestCronApplyValidatesBeforeForwarding(t *testing.T) {
	s := newBackupTestServer(t)
	dispatched := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		dispatched++
		return map[string]interface{}{"name": "ok"}, ""
	})

	// URL 与 body name 不一致 → 400
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{
		"name": "other", "schedule": "0 3 * * *", "command": "x", "enabled": true,
	})
	s.handleAgentCronAPI(rec, cronReq(http.MethodPut, "a1", "jobs/ok", string(body)), "a1/cron/jobs/ok")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("name mismatch: code = %d, want 400", rec.Code)
	}

	// 各类非法参数 → 400 且不下发
	bad := []map[string]interface{}{
		{"name": "ok", "schedule": "61 * * * *", "command": "x", "enabled": true},
		{"name": "ok", "schedule": "* * * *", "command": "x", "enabled": true},
		{"name": "ok", "schedule": "@nope", "command": "x", "enabled": true},
		{"name": "ok", "schedule": "0 3 * * *", "command": "", "enabled": true},
		{"name": "ok", "schedule": "0 3 * * *", "command": "a\nb", "enabled": true},
		{"name": "Bad Name", "schedule": "0 3 * * *", "command": "x", "enabled": true},
	}
	for i, b := range bad {
		body, _ := json.Marshal(b)
		rec := httptest.NewRecorder()
		s.handleAgentCronAPI(rec, cronReq(http.MethodPut, "a1", "jobs/ok", string(body)), "a1/cron/jobs/ok")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("case %d: code = %d, want 400", i, rec.Code)
		}
	}
	if dispatched != 0 {
		t.Fatal("invalid payloads must not reach agent")
	}

	// 合法（含 */步长 与范围形式）→ 转发
	body, _ = json.Marshal(map[string]interface{}{
		"name": "ok", "schedule": "1-10/2 * * * *", "command": "/opt/x.sh", "enabled": false,
	})
	rec = httptest.NewRecorder()
	s.handleAgentCronAPI(rec, cronReq(http.MethodPut, "a1", "jobs/ok", string(body)), "a1/cron/jobs/ok")
	if rec.Code != http.StatusOK || dispatched != 1 {
		t.Fatalf("apply: code = %d dispatched = %d body: %s", rec.Code, dispatched, rec.Body.String())
	}
}

func TestCronApplyAuditsAndForwardsError(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method != "cron.job.apply" {
			return nil, "unexpected method " + method
		}
		// 真实线路经 JSON 序列化，job 是 map；fake agent 直收内存对象，roundtrip 归一
		raw, _ := json.Marshal(params["job"])
		var job map[string]interface{}
		json.Unmarshal(raw, &job)
		if job["name"] != "t1" || job["schedule"] != "0 3 * * *" {
			return nil, "bad job payload"
		}
		// 模拟自检失败：外部条目会变的防御错误
		return nil, "safety check failed: external entries would change, aborting write"
	})

	body, _ := json.Marshal(map[string]interface{}{
		"name": "t1", "schedule": "0 3 * * *", "command": "/opt/t1.sh", "enabled": true,
	})
	rec := httptest.NewRecorder()
	s.handleAgentCronAPI(rec, cronReq(http.MethodPut, "a1", "jobs/t1", string(body)), "a1/cron/jobs/t1")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "safety check failed") {
		t.Fatalf("agent error not forwarded: %s", rec.Body.String())
	}

	// 成功路径审计（第二个 agent）
	withFakeBackupAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"name": "t2"}, ""
	})
	body2, _ := json.Marshal(map[string]interface{}{
		"name": "t2", "schedule": "@daily", "command": "/opt/t2.sh", "enabled": true,
	})
	rec = httptest.NewRecorder()
	s.handleAgentCronAPI(rec, cronReq(http.MethodPut, "a2", "jobs/t2", string(body2)), "a2/cron/jobs/t2")
	if rec.Code != http.StatusOK {
		t.Fatalf("apply2 code = %d body: %s", rec.Code, rec.Body.String())
	}
	logs, total, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{
		"action": audit.ActionCronApply, "resource": audit.ResourceCronJob,
	})
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("cron_apply audit missing: total=%d err=%v", total, err)
	}
	if logs[0].ResourceID != "t2" || !strings.Contains(logs[0].Details, "@daily") {
		t.Fatalf("audit entry = %+v", logs[0])
	}
}

func TestCronDeleteForwardsAndAudits(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method != "cron.job.delete" {
			return nil, "unexpected method " + method
		}
		if params["name"] != "old" {
			return nil, "bad name"
		}
		return map[string]interface{}{}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentCronAPI(rec, cronReq(http.MethodDelete, "a1", "jobs/old", ""), "a1/cron/jobs/old")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete code = %d body: %s", rec.Code, rec.Body.String())
	}
	logs, total, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{
		"action": audit.ActionCronDelete,
	})
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("cron_delete audit missing: total=%d err=%v", total, err)
	}
	if logs[0].ResourceID != "old" {
		t.Fatalf("audit resourceID = %q", logs[0].ResourceID)
	}

	// 非法名字 400 不下发
	rec = httptest.NewRecorder()
	s.handleAgentCronAPI(rec, cronReq(http.MethodDelete, "a1", "jobs/Bad_Name", ""), "a1/cron/jobs/Bad_Name")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad name code = %d, want 400", rec.Code)
	}
}

func TestCronAgentOfflineAndUnknownSubpath(t *testing.T) {
	s := newBackupTestServer(t) // 不注册 agent

	for _, tc := range []struct{ method, sub string }{
		{http.MethodGet, "status"}, {http.MethodGet, "jobs"},
		{http.MethodPut, "jobs/x"}, {http.MethodDelete, "jobs/x"},
	} {
		rec := httptest.NewRecorder()
		s.handleAgentCronAPI(rec, cronReq(tc.method, "ghost", tc.sub, `{}`), "ghost/cron/"+tc.sub)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: code = %d, want 503", tc.method, tc.sub, rec.Code)
		}
	}

	// 在线 agent + 未知子路径 → 404
	withFakeBackupAgent(t, s, "a1", nil)
	rec := httptest.NewRecorder()
	s.handleAgentCronAPI(rec, cronReq(http.MethodGet, "a1", "whatever", ""), "a1/cron/whatever")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown subpath code = %d, want 404", rec.Code)
	}
}
