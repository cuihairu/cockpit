package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// proxyReq 构造对 /api/agents/{id}/proxy/{sub} 的请求
func proxyReq(method, agentID, sub, body string) *http.Request {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	return httptest.NewRequest(method, "/api/agents/"+agentID+"/proxy/"+sub, reader)
}

func TestProxyStatusAndSitesForward(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		if method == "nginx.status" {
			return map[string]interface{}{
				"installed": true, "version": "nginx/1.24.0",
				"confDir": "/etc/nginx/conf.d", "siteCount": 1, "reloadMode": "systemctl",
			}, ""
		}
		return map[string]interface{}{
			"sites": []map[string]interface{}{
				{"name": "blog", "serverNames": []string{"blog.example.com"},
					"upstream": "127.0.0.1:3000", "scheme": "http", "websocket": false},
			},
		}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentProxyAPI(rec, proxyReq(http.MethodGet, "a1", "status", ""), "a1/proxy/status")
	if rec.Code != http.StatusOK || gotMethod != "nginx.status" {
		t.Fatalf("status: code=%d method=%s body=%s", rec.Code, gotMethod, rec.Body.String())
	}
	var st struct {
		Version string `json:"version"`
	}
	json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Version != "nginx/1.24.0" {
		t.Fatalf("version = %q", st.Version)
	}

	rec = httptest.NewRecorder()
	s.handleAgentProxyAPI(rec, proxyReq(http.MethodGet, "a1", "sites", ""), "a1/proxy/sites")
	if rec.Code != http.StatusOK || gotMethod != "nginx.sites" {
		t.Fatalf("sites: code=%d method=%s", rec.Code, gotMethod)
	}
	var list struct {
		Sites []map[string]interface{} `json:"sites"`
	}
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Sites) != 1 || list.Sites[0]["name"] != "blog" {
		t.Fatalf("sites = %v", list.Sites)
	}
}

func TestProxyApplyValidatesBeforeForwarding(t *testing.T) {
	s := newBackupTestServer(t)
	dispatched := 0
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		dispatched++
		return map[string]interface{}{"name": "ok", "file": "cockpit-site-ok.conf"}, ""
	})

	// URL 与 body name 不一致 → 400
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{
		"name": "other", "serverNames": []string{"a.com"},
		"upstream": "127.0.0.1:80", "scheme": "http",
	})
	s.handleAgentProxyAPI(rec, proxyReq(http.MethodPut, "a1", "sites/ok", string(body)), "a1/proxy/sites/ok")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("name mismatch: code = %d, want 400", rec.Code)
	}

	// 各类非法参数 → 400 且不下发
	bad := []map[string]interface{}{
		{"name": "ok", "serverNames": []string{"a b"}, "upstream": "h:1", "scheme": "http"},
		{"name": "ok", "serverNames": []string{}, "upstream": "h:1", "scheme": "http"},
		{"name": "ok", "serverNames": []string{"a.com"}, "upstream": "h p", "scheme": "http"},
		{"name": "ok", "serverNames": []string{"a.com"}, "upstream": "h:1", "scheme": "ftp"},
		{"name": "ok", "serverNames": []string{"a.com"}, "upstream": "h:1", "scheme": "https",
			"tlsCert": "rel.pem", "tlsKey": "/k.pem"},
		{"name": "Bad", "serverNames": []string{"a.com"}, "upstream": "h:1", "scheme": "http"},
	}
	for i, b := range bad {
		body, _ := json.Marshal(b)
		rec := httptest.NewRecorder()
		s.handleAgentProxyAPI(rec, proxyReq(http.MethodPut, "a1", "sites/ok", string(body)), "a1/proxy/sites/ok")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("case %d: code = %d, want 400", i, rec.Code)
		}
	}
	if dispatched != 0 {
		t.Fatal("invalid payloads must not reach agent")
	}

	// 合法 → 转发 + 审计
	body, _ = json.Marshal(map[string]interface{}{
		"name": "ok", "serverNames": []string{"a.com", "*.b.org"},
		"upstream": "127.0.0.1:8080", "scheme": "https",
		"tlsCert": "/etc/ssl/a.pem", "tlsKey": "/etc/ssl/a.key", "websocket": true,
	})
	rec = httptest.NewRecorder()
	s.handleAgentProxyAPI(rec, proxyReq(http.MethodPut, "a1", "sites/ok", string(body)), "a1/proxy/sites/ok")
	if rec.Code != http.StatusOK || dispatched != 1 {
		t.Fatalf("apply: code = %d dispatched = %d body: %s", rec.Code, dispatched, rec.Body.String())
	}
}

func TestProxyApplyAuditsAndForwardsError(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method != "nginx.site.apply" {
			return nil, "unexpected method " + method
		}
		// 真实线路经 JSON 序列化，site 是 map；fake agent 直收内存对象，roundtrip 归一
		raw, _ := json.Marshal(params["site"])
		var site map[string]interface{}
		json.Unmarshal(raw, &site)
		if site["name"] != "t1" || site["upstream"] != "127.0.0.1:3000" {
			return nil, "bad site payload"
		}
		// 模拟 nginx -t 失败：stderr 摘要作为 RPC error 返回
		return nil, "nginx -t failed: emerg: invalid condition in /etc/nginx/nginx.conf:99"
	})

	body, _ := json.Marshal(map[string]interface{}{
		"name": "t1", "serverNames": []string{"t.example.com"},
		"upstream": "127.0.0.1:3000", "scheme": "http",
	})
	rec := httptest.NewRecorder()
	s.handleAgentProxyAPI(rec, proxyReq(http.MethodPut, "a1", "sites/t1", string(body)), "a1/proxy/sites/t1")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "nginx -t failed") {
		t.Fatalf("stderr summary not forwarded: %s", rec.Body.String())
	}

	// 应用成功后审计可见（用第二个 agent 走成功路径）
	withFakeBackupAgent(t, s, "a2", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"name": "t2", "file": "cockpit-site-t2.conf"}, ""
	})
	body2, _ := json.Marshal(map[string]interface{}{
		"name": "t2", "serverNames": []string{"t2.example.com"},
		"upstream": "127.0.0.1:4000", "scheme": "http",
	})
	rec = httptest.NewRecorder()
	s.handleAgentProxyAPI(rec, proxyReq(http.MethodPut, "a2", "sites/t2", string(body2)), "a2/proxy/sites/t2")
	if rec.Code != http.StatusOK {
		t.Fatalf("apply2 code = %d body: %s", rec.Code, rec.Body.String())
	}
	logs, total, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{
		"action": audit.ActionProxyApply, "resource": audit.ResourceProxySite,
	})
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("proxy_apply audit missing: total=%d err=%v", total, err)
	}
	if logs[0].ResourceID != "t2" || !strings.Contains(logs[0].Details, "t2.example.com") {
		t.Fatalf("audit entry = %+v", logs[0])
	}
}

func TestProxyDeleteForwardsAndAudits(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method != "nginx.site.delete" {
			return nil, "unexpected method " + method
		}
		if params["name"] != "old" {
			return nil, "bad name"
		}
		return map[string]interface{}{}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentProxyAPI(rec, proxyReq(http.MethodDelete, "a1", "sites/old", ""), "a1/proxy/sites/old")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete code = %d body: %s", rec.Code, rec.Body.String())
	}
	logs, total, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{
		"action": audit.ActionProxyDelete,
	})
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("proxy_delete audit missing: total=%d err=%v", total, err)
	}
	if logs[0].ResourceID != "old" {
		t.Fatalf("audit resourceID = %q", logs[0].ResourceID)
	}

	// 非法名字 400 不下发
	rec = httptest.NewRecorder()
	s.handleAgentProxyAPI(rec, proxyReq(http.MethodDelete, "a1", "sites/Bad_Name", ""), "a1/proxy/sites/Bad_Name")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad name code = %d, want 400", rec.Code)
	}
}

func TestProxyAgentOffline(t *testing.T) {
	s := newBackupTestServer(t) // 不注册 agent

	for _, tc := range []struct{ method, sub string }{
		{http.MethodGet, "status"}, {http.MethodGet, "sites"},
		{http.MethodPut, "sites/x"}, {http.MethodDelete, "sites/x"},
	} {
		rec := httptest.NewRecorder()
		s.handleAgentProxyAPI(rec, proxyReq(tc.method, "ghost", tc.sub, `{}`), "ghost/proxy/"+tc.sub)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: code = %d, want 503", tc.method, tc.sub, rec.Code)
		}
	}
}

func TestProxyUnknownSubpath(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil) // 在线 agent 才能走到子路径分发
	rec := httptest.NewRecorder()
	s.handleAgentProxyAPI(rec, proxyReq(http.MethodGet, "a1", "whatever", ""), "a1/proxy/whatever")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

// 防止 protocol 未使用（payload 构造经由 fake agent，与 backup_test 一致）
var _ = protocol.MessageTypeRPCResponse
