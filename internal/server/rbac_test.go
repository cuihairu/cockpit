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
		// agent 侧日志面同为读（真机验收发现：POST 泛化推导成 logs:write——
		// 合法权限集合只有 logs:read，admin 也 403，query/follow 整体不可达）
		{"viewer agent logs follow(post is read)", "viewer", "POST", "/api/agents/a1/logs/follow", 200},
		{"operator agent logs query", "operator", "POST", "/api/agents/a1/logs/query", 200},
		{"admin agent logs follow", "admin", "POST", "/api/agents/a1/logs/follow", 200},
		{"viewer agent logs sources", "viewer", "GET", "/api/agents/a1/logs/sources", 200},
		{"viewer audit export(post is read)", "viewer", "POST", "/api/admin/audit/export", 200},
		{"viewer users", "viewer", "GET", "/api/users", 403},
		{"viewer roles list", "viewer", "GET", "/api/roles", 403},
		{"viewer settings", "viewer", "GET", "/api/settings", 403},
		{"viewer terminal list", "viewer", "GET", "/api/remote/terminal", 403},
		{"viewer delete recording", "viewer", "DELETE", "/api/recordings/x", 403},
		{"viewer agent files read", "viewer", "GET", "/api/agents/a1/files/list", 200},
		{"viewer agent files write", "viewer", "POST", "/api/agents/a1/files/write", 403},
		{"viewer agent secret", "viewer", "POST", "/api/agents/a1/secret", 403},
		{"viewer agent detail", "viewer", "GET", "/api/agents/a1", 200},
		// 标签面：/api/agent-tags 与 /api/agents/{id}/tags 同归 inventory
		// （补规则前 /api/agent-tags 不匹配任何 resourceRule，governed=false
		// 整体放行——viewer 也能建删标签，权限点核对补上）
		{"viewer list tags", "viewer", "GET", "/api/agent-tags", 200},
		{"viewer create tag", "viewer", "POST", "/api/agent-tags", 403},
		{"viewer delete tag", "viewer", "DELETE", "/api/agent-tags/t1", 403},
		{"operator create tag", "operator", "POST", "/api/agent-tags", 200},
		{"operator rename tag", "operator", "PUT", "/api/agent-tags/t1", 200},
		{"viewer read agent tags", "viewer", "GET", "/api/agents/a1/tags", 200},
		{"viewer set agent tags", "viewer", "PUT", "/api/agents/a1/tags", 403},
		{"operator set agent tags", "operator", "PUT", "/api/agents/a1/tags", 200},
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
		{"operator create role", "operator", "POST", "/api/roles", 403},
		{"operator settings", "operator", "PUT", "/api/settings", 403},
		{"operator create user", "operator", "POST", "/api/users", 403},
		{"operator terminal", "operator", "GET", "/api/remote/terminal", 200},
		// overlay 本机侧（M3 D28）：读观测全角色放行；join/leave/service
		// 归 overlay:admin——operator 只有 read+write，被 D28 档位排除
		{"viewer overlay status", "viewer", "GET", "/api/agents/a1/overlay/status", 200},
		{"operator overlay daemon", "operator", "GET", "/api/agents/a1/overlay/daemon", 200},
		{"operator overlay cloud write", "operator", "POST", "/api/overlay/cloud/networks/n/members", 200},
		{"viewer overlay join", "viewer", "POST", "/api/agents/a1/overlay/networks/8056c2e21c000001/join", 403},
		{"operator overlay join", "operator", "POST", "/api/agents/a1/overlay/networks/8056c2e21c000001/join", 403},
		{"operator overlay leave", "operator", "POST", "/api/agents/a1/overlay/networks/-/leave", 403},
		{"operator overlay service", "operator", "POST", "/api/agents/a1/overlay/service", 403},
		{"operator overlay status post falls to write", "operator", "POST", "/api/agents/a1/overlay/status", 200},
		// admin：全量
		{"admin users", "admin", "GET", "/api/users", 200},
		{"admin roles list", "admin", "GET", "/api/roles", 200},
		{"admin settings", "admin", "PUT", "/api/settings", 200},
		{"admin domains write", "admin", "POST", "/api/domains", 200},
		{"admin drift config", "admin", "PUT", "/api/drift/config", 200},
		{"admin overlay join", "admin", "POST", "/api/agents/a1/overlay/networks/8056c2e21c000001/join", 200},
		{"admin overlay service", "admin", "POST", "/api/agents/a1/overlay/service", 200},
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

// TestRBACOverlayJoinTierD28 overlay:admin 档位独立于 write：自定义角色
// 有 overlay:write 也进不了本机 join/leave/service，显式补 admin 才放行
func TestRBACOverlayJoinTierD28(t *testing.T) {
	s := newTestServerWithDB(t)
	if err := s.db.CreateRole(&storage.Role{
		Name: "net-ops", Permissions: []string{"overlay:read", "overlay:write"},
	}); err != nil {
		t.Fatal(err)
	}
	join := "/api/agents/a1/overlay/networks/8056c2e21c000001/join"
	if got := rbacPair(t, s, "net-ops", "GET", "/api/agents/a1/overlay/status"); got != http.StatusOK {
		t.Errorf("net-ops overlay status = %d, want 200", got)
	}
	if got := rbacPair(t, s, "net-ops", "POST", join); got != http.StatusForbidden {
		t.Errorf("net-ops overlay join = %d, want 403 (overlay:write 不含 admin 档)", got)
	}
	// 升级角色补 overlay:admin → 放行
	if err := s.db.UpdateRole(&storage.Role{
		Name: "net-ops", Permissions: []string{"overlay:read", "overlay:write", "overlay:admin"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := rbacPair(t, s, "net-ops", "POST", join); got != http.StatusOK {
		t.Errorf("net-ops + overlay:admin join = %d, want 200", got)
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
