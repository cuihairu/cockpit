package server

// rbac_test.go RBAC 笔 2 验收（rbac-design.md P0 验收标准）：三内置角色
// 的 200/403 矩阵 + AND 语义（domain-binding D9）+ 未知角色 fail-closed +
// 认证优先于鉴权（无/坏 token 放行给内层 auth）。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cuihairu/cockpit/internal/storage"
)

func rbacPair(t *testing.T, s *Server, role, method, path string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if role == "bad" {
		req.Header.Set("Authorization", "Bearer not-a-jwt")
	} else if role != "" {
		token, err := s.authService().GenerateToken("u-"+role, role, role)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.RBACMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)
	return rec.Code
}

func TestRBACMatrix(t *testing.T) {
	s := newTestServerWithDB(t)

	cases := []struct {
		name   string
		role   string
		method string
		path   string
		want   int
	}{
		// viewer：全模块只读、无 terminal、无管理域
		{"viewer read dns", "viewer", "GET", "/api/dns", 200},
		{"viewer write dns", "viewer", "POST", "/api/dns/zones/z/records", 403},
		{"viewer logs search(post is read)", "viewer", "POST", "/api/logs/search", 200},
		{"viewer audit export(post is read)", "viewer", "POST", "/api/admin/audit/export", 200},
		{"viewer users", "viewer", "GET", "/api/users", 403},
		{"viewer settings", "viewer", "GET", "/api/settings", 403},
		{"viewer terminal list", "viewer", "GET", "/api/remote/terminal", 403},
		{"viewer delete recording", "viewer", "DELETE", "/api/recordings/x", 403},
		{"viewer agent files read", "viewer", "GET", "/api/agents/a1/files/list", 200},
		{"viewer agent files write", "viewer", "POST", "/api/agents/a1/files/write", 403},
		{"viewer agent secret", "viewer", "POST", "/api/agents/a1/secret", 403},
		{"viewer agent detail", "viewer", "GET", "/api/agents/a1", 200},
		{"viewer me", "viewer", "GET", "/api/me", 200},
		{"viewer status", "viewer", "GET", "/api/status", 200},
		// operator：模块 read+write 全放（含 acme 签发）、管理域全拒
		{"operator write dns", "operator", "POST", "/api/dns/zones/z/records", 200},
		{"operator acme issue", "operator", "POST", "/api/acme/certs/1/issue", 200},
		{"operator acme deploy", "operator", "POST", "/api/acme/certs/1/deploy", 200},
		{"operator acme list", "operator", "GET", "/api/acme/certs", 200},
		{"operator stack up", "operator", "POST", "/api/stacks/agents/a1/x/up", 200},
		{"operator docker stop", "operator", "POST", "/api/docker/agents/a1/containers/c/stop", 200},
		{"operator users", "operator", "GET", "/api/users", 403},
		{"operator settings", "operator", "PUT", "/api/settings", 403},
		{"operator create user", "operator", "POST", "/api/users", 403},
		{"operator terminal", "operator", "GET", "/api/remote/terminal", 200},
		// admin：全量
		{"admin users", "admin", "GET", "/api/users", 200},
		{"admin settings", "admin", "PUT", "/api/settings", 200},
		{"admin domains write", "admin", "POST", "/api/domains", 200},
		{"admin drift config", "admin", "PUT", "/api/drift/config", 200},
		// 认证优先于鉴权：无/坏 token 放行给内层 auth（出 401 而非 403）
		{"no token", "", "GET", "/api/dns", 200},
		{"bad token", "bad", "GET", "/api/dns", 200},
		// 未知角色 fail-closed
		{"ghost role", "ghost", "GET", "/api/dns", 403},
		// 未登记路径放行（handler 404 兜底）
		{"unknown path", "viewer", "GET", "/api/no-such-module", 200},
		{"non-api path", "viewer", "GET", "/health", 200},
	}
	for _, c := range cases {
		if got := rbacPair(t, s, c.role, c.method, c.path); got != c.want {
			t.Errorf("%s: %s %s role=%s = %d, want %d", c.name, c.method, c.path, c.role, got, c.want)
		}
	}
}

func TestRBACDomainWriteRequiresBoth(t *testing.T) {
	s := newTestServerWithDB(t)
	// 自定义角色只有 dns:write：dns 写放、domains 写（AND dns:write+proxy:write）拒
	if err := s.db.CreateRole(&storage.Role{
		Name: "dns-only", Permissions: []string{"dns:write"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := rbacPair(t, s, "dns-only", "POST", "/api/dns/zones/z/records"); got != http.StatusOK {
		t.Errorf("dns-only write dns = %d, want 200", got)
	}
	if got := rbacPair(t, s, "dns-only", "POST", "/api/domains"); got != http.StatusForbidden {
		t.Errorf("dns-only write domains = %d, want 403 (needs proxy:write too)", got)
	}
	// 只读清单不需要 proxy:write
	if got := rbacPair(t, s, "dns-only", "GET", "/api/domains"); got != http.StatusOK {
		t.Errorf("dns-only read domains = %d, want 200", got)
	}
}

func TestRoleCovers(t *testing.T) {
	have := []string{"dns:write", "acme:admin", "terminal:write"}
	for _, ok := range []string{"dns:read", "dns:write", "acme:read", "acme:write", "acme:admin", "terminal:read"} {
		if !roleCovers(have, ok) {
			t.Errorf("roleCovers(%v, %s) = false, want true", have, ok)
		}
	}
	for _, no := range []string{"proxy:read", "dns:admin", "users:admin", "terminal:admin", "dns:execute", "malformed"} {
		if roleCovers(have, no) {
			t.Errorf("roleCovers(%v, %s) = true, want false", have, no)
		}
	}
}
