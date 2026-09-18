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

	// 非法请求：server 直接 400，不消耗 agent 往返（D9.4 并集白名单：
	// 无后缀/带点的名字属 Windows 名形态已放行，这里只留双后端都拒的样例）
	before := len(gotParams)
	for _, sub := range []string{
		"nginx.service/daemon-reload", // 白名单外动词
		"nginx.service/start;reboot",  // 注入样例（分号不在两套字符集）
		"../start",                    // .. 目录引用
	} {
		rec = httptest.NewRecorder()
		s.handleAgentServiceAPI(rec, serviceReq(http.MethodPost, "a1", sub), "a1/services/"+sub)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, want 400", sub, rec.Code)
		}
	}
	// 空字节以 %00 形态到达后解码进 rest，校验拒绝（target 需编码否则无法构造）
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodPost, "a1", "svc%00name/start"), "a1/services/svc\x00name/start")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("null byte: code = %d, want 400", rec.Code)
	}
	if before != len(gotParams) {
		t.Error("rejected requests must not reach the agent")
	}

	// Windows 服务名（无 .service 后缀）按并集白名单放行转发
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodPost, "a1", "wuauserv/restart"), "a1/services/wuauserv/restart")
	if rec.Code != http.StatusOK {
		t.Errorf("windows service name: code = %d, want 200", rec.Code)
	}
	if gotParams["name"] != "wuauserv" || gotParams["action"] != "restart" {
		t.Errorf("windows params = %+v", gotParams)
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

func TestServiceDaemonReload(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		return map[string]interface{}{"reloaded": true}, ""
	})

	// POST 转发 service.daemon-reload + 记审计（D12）
	rec := httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodPost, "a1", "daemon-reload"), "a1/services/daemon-reload")
	if rec.Code != http.StatusOK || gotMethod != "service.daemon-reload" {
		t.Fatalf("daemon-reload: code=%d method=%s", rec.Code, gotMethod)
	}
	logs, _, _ := s.db.GetAuditLogs(0, 10, nil)
	if len(logs) != 1 || logs[0].Action != "service_action" || logs[0].ResourceID != "daemon-reload" {
		t.Fatalf("audit logs = %+v", logs)
	}

	// 非 POST / 其余子路径不受特判影响
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodGet, "a1", "daemon-reload"), "a1/services/daemon-reload")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET daemon-reload: code = %d, want 404", rec.Code)
	}
	// 带后缀的 unit 动作仍走两段路由
	withFakeBackupAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		return map[string]interface{}{}, ""
	})
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodPost, "a2", "nginx.service/restart"), "a2/services/nginx.service/restart")
	if rec.Code != http.StatusOK || gotMethod != "service.action" {
		t.Errorf("unit action: code=%d method=%s", rec.Code, gotMethod)
	}
}

func TestServiceUnitFileAPI(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	var gotParams map[string]interface{}
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod, gotParams = method, params
		if method == "service.unitfile" {
			return map[string]interface{}{"name": "nginx.service", "fragmentPath": "/etc/x", "content": "[Unit]\n"}, ""
		}
		return map[string]interface{}{"name": "nginx.service", "path": "/etc/x", "reloaded": true}, ""
	})

	// GET：转发 service.unitfile，浏览不审计（D13）
	rec := httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodGet, "a1", "nginx.service/file"), "a1/services/nginx.service/file")
	if rec.Code != http.StatusOK || gotMethod != "service.unitfile" {
		t.Fatalf("get file: code=%d method=%s", rec.Code, gotMethod)
	}
	if gotParams["name"] != "nginx.service" {
		t.Errorf("params = %+v", gotParams)
	}
	logs, _, _ := s.db.GetAuditLogs(0, 10, nil)
	if len(logs) != 0 {
		t.Errorf("read must not be audited, logs = %+v", logs)
	}

	// PUT：转发 service.unitsave（含 content），记审计
	put := httptest.NewRequest(http.MethodPut, "/api/agents/a1/services/nginx.service/file",
		strings.NewReader(`{"content":"[Unit]\nDescription=x\n"}`))
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, put, "a1/services/nginx.service/file")
	if rec.Code != http.StatusOK || gotMethod != "service.unitsave" {
		t.Fatalf("save file: code=%d method=%s", rec.Code, gotMethod)
	}
	if gotParams["content"] != "[Unit]\nDescription=x\n" {
		t.Errorf("save params = %+v", gotParams)
	}
	logs, _, _ = s.db.GetAuditLogs(0, 10, nil)
	if len(logs) != 1 || logs[0].ResourceID != "nginx.service" {
		t.Fatalf("audit logs = %+v", logs)
	}

	// 超限 content → 413；坏 JSON → 400；多余路径段 → 404；坏名 → 400
	big := `{"content":"` + strings.Repeat("x", unitFileBodyLimit+1) + `"}`
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, httptest.NewRequest(http.MethodPut, "/api/agents/a1/services/nginx.service/file",
		strings.NewReader(big)), "a1/services/nginx.service/file")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize: code = %d, want 413", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, httptest.NewRequest(http.MethodPut, "/api/agents/a1/services/nginx.service/file",
		strings.NewReader(`not-json`)), "a1/services/nginx.service/file")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json: code = %d, want 400", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodGet, "a1", "nginx.service/file/extra"), "a1/services/nginx.service/file/extra")
	if rec.Code != http.StatusNotFound {
		t.Errorf("extra segment: code = %d, want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleAgentServiceAPI(rec, serviceReq(http.MethodGet, "a1", "../x/file"), "a1/services/../x/file")
	// ../x 含路径分隔符，先被「仅允许恰好两段」拒绝（404），到不了名字校验
	if rec.Code != http.StatusNotFound {
		t.Errorf("bad unit name: code = %d, want 404", rec.Code)
	}
	if gotMethod != "service.unitsave" {
		t.Errorf("rejected requests must not reach the agent, last method = %s", gotMethod)
	}
}

func TestValidateServiceAction(t *testing.T) {
	// systemd unit 名：字母数字与 @ . _ + - 且 .service 结尾
	for _, name := range []string{"nginx.service", "user@1000.service", "openvpn@server.service", "my-app.service"} {
		for _, action := range []string{"start", "stop", "restart", "reload", "enable", "disable", "mask", "unmask"} {
			if err := validateServiceAction(name, action); err != nil {
				t.Errorf("validate(%q, %q): %v", name, action, err)
			}
		}
	}
	// Windows 服务名（D9.4 并集放行）：无后缀、可含空格
	for _, tc := range []struct{ unit, action string }{
		{"wuauserv", "restart"}, {"nginx", "start"}, {"nginx.socket", "stop"},
		{"Lenovo Vantage Service", "stop"}, {"a b.service", "start"},
	} {
		if err := validateServiceAction(tc.unit, tc.action); err != nil {
			t.Errorf("validate(%q, %q): %v", tc.unit, tc.action, err)
		}
	}
	for _, tc := range []struct{ unit, action string }{
		{"", "start"}, {"svc;rm", "start"}, {"svc\x00", "start"},
		{".", "start"}, {"..", "start"},
		{"nginx.service", "daemon-reload"}, {"nginx.service", "STOP"},
	} {
		if err := validateServiceAction(tc.unit, tc.action); err == nil {
			t.Errorf("validate(%q, %q) should fail", tc.unit, tc.action)
		}
	}
}
