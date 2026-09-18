package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serviceReq 构造对 /api/agents/{id}/services/{sub} 的请求
func serviceReq(method, agentID, sub string) *http.Request {
	return httptest.NewRequest(method, "/api/agents/"+agentID+"/services/"+sub, nil)
}

func TestServiceListAndStatusForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		if method == "service.status" {
			return map[string]interface{}{
				"systemState": "degraded", "total": 6, "active": 3, "failed": 1, "enabled": 3,
			}, ""
		}
		return map[string]interface{}{
			"services": []map[string]interface{}{
				{"name": "nginx.service", "description": "web", "loadState": "loaded",
					"activeState": "active", "subState": "running", "unitFileState": "enabled"},
			},
		}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodGet, "a1", "status"), "a1/services/status")
	if rec.Code != http.StatusOK || gotMethod != "service.status" {
		t.Fatalf("status: code=%d method=%s", rec.Code, gotMethod)
	}
	if !strings.Contains(rec.Body.String(), `"systemState":"degraded"`) {
		t.Errorf("status body = %s", rec.Body.String())
	}

	// 列表：浏览不审计
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodGet, "a1", ""), "a1/services")
	if rec.Code != http.StatusOK || gotMethod != "service.list" {
		t.Fatalf("list: code=%d method=%s", rec.Code, gotMethod)
	}
	logs, _, _ := s.db.GetAuditLogs(0, 10, nil)
	for _, l := range logs {
		if l.Action == "service_action" {
			t.Error("list/status must not be audited")
		}
	}
}

func TestServiceActionValidatesAndAudits(t *testing.T) {
	s := newBackupTestServer(t)
	var gotParams map[string]interface{}
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotParams = params
		return map[string]interface{}{"name": "nginx.service", "action": "restart"}, ""
	})

	// 合法动作 → 转发参数正确 + 审计
	rec := httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodPost, "a1", "nginx.service/restart"), "a1/services/nginx.service/restart")
	if rec.Code != http.StatusOK {
		t.Fatalf("restart: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotParams["name"] != "nginx.service" || gotParams["action"] != "restart" {
		t.Errorf("params = %+v", gotParams)
	}
	logs, _, _ := s.db.GetAuditLogs(0, 10, nil)
	if len(logs) != 1 || logs[0].Action != "service_action" || logs[0].ResourceID != "nginx.service" {
		t.Fatalf("audit logs = %+v", logs)
	}

	// 非法请求：server 直接 400，不消耗 agent 往返
	before := len(gotParams)
	for _, sub := range []string{
		"nginx.service/mask",         // 白名单外动词
		"nginx.service/start;reboot", // 注入样例
		"nginx/start",                // 缺 .service
		"nginx.socket/start",         // 非 service 类型
	} {
		rec = httptest.NewRecorder()
		s.handleAgentServiceAPI(rec, serviceReq(http.MethodPost, "a1", sub), "a1/services/"+sub)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, want 400", sub, rec.Code)
		}
	}
	// 路径穿越：Split 出 4 段不构成 unit/action，404 拒绝（同样到不了 agent）
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodPost, "a1", "../etc/passwd/start"), "a1/services/../etc/passwd/start")
	if rec.Code != http.StatusNotFound {
		t.Errorf("path traversal: code = %d, want 404", rec.Code)
	}
	if before != len(gotParams) {
		t.Error("rejected requests must not reach the agent")
	}

	// agent 侧错误原样透传
	withFakeBackupAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "systemctl start backup.service: Unit is masked"
	})
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodPost, "a2", "backup.service/start"), "a2/services/backup.service/start")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "Unit is masked") {
		t.Errorf("agent error passthrough: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServiceAgentOfflineAndRouting(t *testing.T) {
	s := newBackupTestServer(t)

	// agent 离线 → 503
	rec := httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodGet, "ghost", ""), "ghost/services")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("offline: code = %d, want 503", rec.Code)
	}

	// method 不匹配 → 404/405
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{}, ""
	})
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodGet, "a1", ""), "a1/services")
	if rec.Code != http.StatusOK {
		t.Errorf("GET list: code = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodPost, "a1", "status"), "a1/services/status")
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST status: code = %d, want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodDelete, "a1", "nginx.service/stop"), "a1/services/nginx.service/stop")
	if rec.Code != http.StatusNotFound {
		t.Errorf("DELETE action: code = %d, want 404", rec.Code)
	}
	// 多余路径段 → 404
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodPost, "a1", "nginx.service/start/extra"), "a1/services/nginx.service/start/extra")
	if rec.Code != http.StatusNotFound {
		t.Errorf("extra path: code = %d, want 404", rec.Code)
	}
}

func TestValidateServiceAction(t *testing.T) {
	for _, name := range []string{"nginx.service", "user@1000.service", "openvpn@server.service", "my-app.service"} {
		for _, action := range []string{"start", "stop", "restart", "reload", "enable", "disable"} {
			if err := validateServiceAction(name, action); err != nil {
				t.Errorf("validate(%q, %q): %v", name, action, err)
			}
		}
	}
	for _, tc := range []struct{ unit, action string }{
		{"nginx", "start"}, {"nginx.socket", "start"}, {"", "start"},
		{"nginx.service", "mask"}, {"nginx.service", "STOP"}, {"a b.service", "start"},
	} {
		if err := validateServiceAction(tc.unit, tc.action); err == nil {
			t.Errorf("validate(%q, %q) should fail", tc.unit, tc.action)
		}
	}
}
