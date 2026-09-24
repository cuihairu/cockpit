package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/auth"
)

// HTTP 方法一致性集成测试（P0 前后端协议一致性，2026-09-24）。
// web 测试 mock 整个 API 方法（绕过 HTTP 层），method 断测试抓不到——
// 静态 path→handler 映射难精确（serveAPI 子分发 + 各 API 文件内再分发），
// 故运行时验证：构造 web 调用的 (method, path) 调分发层，断言非 405
//（404/400/503 都算 method 被接受——只验 method 白名单不误报）。
func TestHTTPMethodDispatch(t *testing.T) {
	s := newTestServerWithDB(t)
	if err := auth.InitAdmin(s.db, "admin", "admin123"); err != nil {
		t.Fatalf("init admin: %v", err)
	}

	// web 调用矩阵（写操作为主——405 断功能最关键）
	cases := []struct {
		method string
		path   string
		body   string
	}{
		// 设置 / 个人资料
		{"PUT", "/api/settings", `{}`},
		{"PUT", "/api/me/profile", `{"email":"a@b.c"}`},
		{"PUT", "/api/me/password", `{"old_password":"x","new_password":"y"}`},
		// 用户 / 角色
		{"GET", "/api/users", ""},
		{"POST", "/api/users", `{"username":"u","password":"p","role":"user"}`},
		{"PUT", "/api/users/u1", `{"email":"a@b.c"}`},
		{"DELETE", "/api/users/u1", ""},
		{"POST", "/api/users/u1/password", `{"new_password":"p"}`},
		{"GET", "/api/roles", ""},
		{"POST", "/api/roles", `{"name":"r","permissions":["x:read"]}`},
		{"PUT", "/api/roles/r1", `{"permissions":["x:read"]}`},
		{"DELETE", "/api/roles/r1", ""},
		// 告警
		{"GET", "/api/alerts", ""},
		{"PUT", "/api/alerts/read-all", ""},
		{"PUT", "/api/alerts/a1/read", ""},
		// 备份
		{"GET", "/api/backups/configs", ""},
		{"POST", "/api/backups/configs", `{"agent_id":"a","name":"n","sources":["/tmp"],"dest_dir":"/d","schedule":"manual","retention":1,"enabled":true}`},
		{"PUT", "/api/backups/configs/1", `{"agent_id":"a","name":"n","sources":["/tmp"],"dest_dir":"/d","schedule":"manual","retention":1,"enabled":true}`},
		{"DELETE", "/api/backups/configs/1", ""},
		{"POST", "/api/backups/configs/1/run", ""},
		// 录制
		{"GET", "/api/recordings", ""},
		{"PUT", "/api/recordings/config", `{"enabled":true}`},
		{"DELETE", "/api/recordings/s1", ""},
		{"POST", "/api/recordings/s1/sync-remote", ""},
		// DNS
		{"GET", "/api/dns/zones", ""},
		{"POST", "/api/dns/zones/z1/records", `{"type":"A","name":"www","content":"1.2.3.4"}`},
		{"PUT", "/api/dns/zones/z1/records/r1", `{"type":"A","name":"www","content":"1.2.3.4"}`},
		{"DELETE", "/api/dns/zones/z1/records/r1", ""},
		// 域名绑定
		{"GET", "/api/domains", ""},
		{"POST", "/api/domains", `{"domain":"example.com","agentId":"a","target":"h:80"}`},
		{"DELETE", "/api/domains/example.com", ""},
		{"POST", "/api/domains/example.com/apply", ""},
		// 反代站点（agent 子分发）
		{"GET", "/api/agents/a1/proxy/sites", ""},
		{"PUT", "/api/agents/a1/proxy/sites/blog", `{"name":"blog","serverNames":["b.example.com"],"upstream":"127.0.0.1:3000","scheme":"http"}`},
		{"DELETE", "/api/agents/a1/proxy/sites/blog", ""},
		// 文件（agent 子分发）
		{"POST", "/api/agents/a1/files/list", `{"dir":"/"}`},
		{"POST", "/api/agents/a1/files/chmod", `{"path":"/tmp/f","mode":420}`},
		// 服务（agent 子分发）
		{"POST", "/api/agents/a1/services/nginx.service/restart", ""},
		{"PUT", "/api/agents/a1/services/nginx.service/file", `{"content":"[Unit]"}`},
		{"POST", "/api/agents/a1/services/daemon-reload", ""},
		// 容器 / Stacks
		{"POST", "/api/docker/agents/a1/containers/c1/start", ""},
		{"DELETE", "/api/docker/agents/a1/containers/c1", ""},
		{"POST", "/api/stacks/agents/a1/web/deploy", ""},
		{"PUT", "/api/stacks/agents/a1/web/compose", `{"compose":"services: {}"}`},
		{"DELETE", "/api/stacks/agents/a1/web", ""},
		// DDNS
		{"GET", "/api/ddns", ""},
		{"POST", "/api/ddns", `{"agentId":"a","zoneId":"z","recordName":"home.example.com","type":"A","enabled":true}`},
		{"PUT", "/api/ddns/1", `{"agentId":"a","zoneId":"z","recordName":"home.example.com","type":"A","enabled":true}`},
		{"DELETE", "/api/ddns/1", ""},
		{"POST", "/api/ddns/1/check", ""},
		{"PUT", "/api/ddns/config", `{"scan_interval_seconds":300}`},
		// ACME
		{"POST", "/api/acme/certs", `{"domains":["example.com"],"caDirectory":"staging"}`},
		{"POST", "/api/acme/certs/1/issue", ""},
		{"POST", "/api/acme/certs/1/deploy", ""},
		{"PUT", "/api/acme/certs/1", `{"domains":["example.com"],"caDirectory":"staging"}`},
		{"DELETE", "/api/acme/certs/1", ""},
		{"PUT", "/api/acme/account", `{"email":"a@b.c"}`},
		{"PUT", "/api/acme/config", `{"scan_interval_seconds":3600}`},
		// server 备份
		{"GET", "/api/server-backups", ""},
		{"POST", "/api/server-backups/run", ""},
		{"PUT", "/api/server-backups/config", `{"interval_hours":24}`},
		{"DELETE", "/api/server-backups/x.db", ""},
		{"POST", "/api/server-backups/x.db/sync-remote", ""},
		// 巡检配置
		{"PUT", "/api/drift/config", `{"scan_interval_seconds":1800}`},
		{"POST", "/api/agents/a1/drift/check", ""},
		{"PUT", "/api/probe/config", `{"interval_seconds":300}`},
		{"POST", "/api/notification/test", ""},
		{"PUT", "/api/nas/config", `{"scan_interval_seconds":1800}`},
		{"PUT", "/api/smart/config", `{"scan_interval_seconds":3600}`},
	}

	for _, c := range cases {
		var body *strings.Reader
		if c.body != "" {
			body = strings.NewReader(c.body)
		} else {
			body = strings.NewReader("")
		}
		req := authenticateAdminRequest(t, s, httptest.NewRequest(c.method, c.path, body))
		// callWithAuth = auth.Middleware 注入用户上下文后调 handler（同 api_test.go）
		rec := callWithAuth(s, s.serveAPI, req)
		if rec.Code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s: 405 Method Not Allowed（web 调用被拒）", c.method, c.path)
		}
	}
}
