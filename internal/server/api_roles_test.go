package server

// api_roles_test.go RBAC 笔 4 验收（rbac-design.md）：/api/roles CRUD
// 错误映射 + D13 自我保护（改自己角色 / 最后有效 admin）+ D14 403 入审计。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/storage"
)

// rolesReq 构造带认证上下文的角色/用户管理请求
func rolesReq(method, path, body, userID, username, role string) *http.Request {
	return covAuthReq(method, path, strings.NewReader(body), userID, username, role)
}

func TestRolesCRUD(t *testing.T) {
	s := newTestServerWithDB(t)

	// PATCH → 405（handleRoles 与 handleRoleActions 的兜底）
	rec := covRec()
	s.handleRoles(rec, covReq(http.MethodPatch, "/api/roles", nil))
	covWantCode(t, "roles 405", rec, http.StatusMethodNotAllowed)
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleRoleActions(w, r, "x")
	}, rolesReq(http.MethodPost, "/api/roles/x", "", "1", "admin", "admin"))
	covWantCode(t, "role actions 405", rec, http.StatusMethodNotAllowed)

	// 列表：内置三角色
	rec = covCallAuth(s, s.handleRolesList, rolesReq(http.MethodGet, "/api/roles", "", "1", "admin", "admin"))
	covWantCode(t, "list", rec, http.StatusOK)
	for _, name := range []string{"admin", "operator", "viewer"} {
		if !strings.Contains(rec.Body.String(), `"name":"`+name+`"`) {
			t.Errorf("list body missing builtin %s: %s", name, rec.Body.String())
		}
	}

	// 创建：bad json / 空名 / 非法权限点 → 400；撞名（含内置）→ 409
	rec = covCallAuth(s, s.handleRoleCreate, rolesReq(http.MethodPost, "/api/roles", "not-json", "1", "admin", "admin"))
	covWantCode(t, "create bad json", rec, http.StatusBadRequest)
	rec = covCallAuth(s, s.handleRoleCreate, rolesReq(http.MethodPost, "/api/roles", `{"name":"","permissions":["dns:read"]}`, "1", "admin", "admin"))
	covWantCode(t, "create empty name", rec, http.StatusBadRequest)
	rec = covCallAuth(s, s.handleRoleCreate, rolesReq(http.MethodPost, "/api/roles", `{"name":"admin","permissions":["dns:read"]}`, "1", "admin", "admin"))
	covWantCode(t, "create builtin name", rec, http.StatusConflict)
	rec = covCallAuth(s, s.handleRoleCreate, rolesReq(http.MethodPost, "/api/roles", `{"name":"bad-perm","permissions":["dns:execute"]}`, "1", "admin", "admin"))
	covWantCode(t, "create bad perm", rec, http.StatusBadRequest)

	// 创建成功 → 201；重名（自定义）→ 409
	rec = covCallAuth(s, s.handleRoleCreate, rolesReq(http.MethodPost, "/api/roles", `{"name":"dns-only","permissions":["dns:write"]}`, "1", "admin", "admin"))
	covWantCode(t, "create ok", rec, http.StatusCreated)
	rec = covCallAuth(s, s.handleRoleCreate, rolesReq(http.MethodPost, "/api/roles", `{"name":"dns-only","permissions":["dns:read"]}`, "1", "admin", "admin"))
	covWantCode(t, "create duplicate", rec, http.StatusConflict)

	// 更新：404 / 内置 400 / bad json / 非法权限点 / 成功
	update := func(name, body string) *httptest.ResponseRecorder {
		return covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
			s.handleRoleUpdate(w, r, name)
		}, rolesReq(http.MethodPut, "/api/roles/"+name, body, "1", "admin", "admin"))
	}
	covWantCode(t, "update missing", update("ghost", `{"permissions":["dns:read"]}`), http.StatusNotFound)
	covWantCode(t, "update builtin", update("viewer", `{"permissions":["dns:read"]}`), http.StatusBadRequest)
	covWantCode(t, "update bad json", update("dns-only", "not-json"), http.StatusBadRequest)
	covWantCode(t, "update bad perm", update("dns-only", `{"permissions":["no:write"]}`), http.StatusBadRequest)
	covWantCode(t, "update ok", update("dns-only", `{"permissions":["dns:read","ddns:read"]}`), http.StatusOK)

	// 删除：内置 400 / 404 / 被引用 409
	del := func(name string) *httptest.ResponseRecorder {
		return covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
			s.handleRoleDelete(w, r, name)
		}, rolesReq(http.MethodDelete, "/api/roles/"+name, "", "1", "admin", "admin"))
	}
	covWantCode(t, "delete builtin", del("operator"), http.StatusBadRequest)
	covWantCode(t, "delete missing", del("ghost"), http.StatusNotFound)
	covSeedUser(t, s, "dnsuser", "dns-pass-1", "dns-only")
	covWantCode(t, "delete in use", del("dns-only"), http.StatusConflict)

	// 解除引用后删除成功
	if err := s.db.DeleteUser(mustUserID(t, s, "dnsuser")); err != nil {
		t.Fatal(err)
	}
	covWantCode(t, "delete ok", del("dns-only"), http.StatusOK)

	// 经 handleRoles / handleRoleActions 的方法分发（GET/POST/PUT/DELETE）
	rec = covCallAuth(s, s.handleRoles, rolesReq(http.MethodGet, "/api/roles", "", "1", "admin", "admin"))
	covWantCode(t, "dispatch list", rec, http.StatusOK)
	rec = covCallAuth(s, s.handleRoles, rolesReq(http.MethodPost, "/api/roles", `{"name":"dispatch","permissions":["dns:read"]}`, "1", "admin", "admin"))
	covWantCode(t, "dispatch create", rec, http.StatusCreated)
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleRoleActions(w, r, "dispatch")
	}, rolesReq(http.MethodPut, "/api/roles/dispatch", `{"permissions":["dns:write"]}`, "1", "admin", "admin"))
	covWantCode(t, "dispatch update", rec, http.StatusOK)
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleRoleActions(w, r, "dispatch")
	}, rolesReq(http.MethodDelete, "/api/roles/dispatch", "", "1", "admin", "admin"))
	covWantCode(t, "dispatch delete", rec, http.StatusOK)

	// trigger 制造 UpdateRole 的 Updates 失败 → 500：GetRole/First 是
	// SELECT 不受影响，D13 判定后写库被 RAISE(ABORT) 拦下（closed db 手法
	// 会让 GetRole 先失败，走不到这个分支）
	rec = covCallAuth(s, s.handleRoleCreate, rolesReq(http.MethodPost, "/api/roles", `{"name":"blocked","permissions":["dns:read"]}`, "1", "admin", "admin"))
	covWantCode(t, "create blocked", rec, http.StatusCreated)
	if err := s.db.Session().Exec("CREATE TRIGGER block_role_update BEFORE UPDATE ON roles BEGIN SELECT RAISE(ABORT, 'blocked'); END").Error; err != nil {
		t.Fatal(err)
	}
	covWantCode(t, "update trigger 500", update("blocked", `{"permissions":["dns:write"]}`), http.StatusInternalServerError)
}

// TestRolesDBError closed db 上的错误分支
func TestRolesDBError(t *testing.T) {
	s := newTestServerWithDB(t)
	covCloseDB(t, s)

	rec := covCallAuth(s, s.handleRolesList, rolesReq(http.MethodGet, "/api/roles", "", "1", "admin", "admin"))
	covWantCode(t, "list db err", rec, http.StatusInternalServerError)

	// GetRole 报错（非 NotFound）放行 → CreateRole 写库失败 → 500
	rec = covCallAuth(s, s.handleRoleCreate, rolesReq(http.MethodPost, "/api/roles", `{"name":"r1","permissions":["dns:read"]}`, "1", "admin", "admin"))
	covWantCode(t, "create db err", rec, http.StatusInternalServerError)

	// 更新：GetRole 库错误 → 500
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleRoleUpdate(w, r, "r1")
	}, rolesReq(http.MethodPut, "/api/roles/r1", `{"permissions":["dns:read"]}`, "1", "admin", "admin"))
	covWantCode(t, "update db err", rec, http.StatusInternalServerError)

	// 删除：Count 引用即库错误 → 500
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleRoleDelete(w, r, "r1")
	}, rolesReq(http.MethodDelete, "/api/roles/r1", "", "1", "admin", "admin"))
	covWantCode(t, "delete db err", rec, http.StatusInternalServerError)
}

// TestD13SelfProtection D13 自我保护：不可改自己角色（真实可达）；
// 不可删/降级最后一个有效 admin（用户侧直调覆盖防御分支，角色侧真实可达）
func TestD13SelfProtection(t *testing.T) {
	s := newTestServerWithDB(t)
	admin := covSeedUser(t, s, "admin", "admin-pass-1", "admin")
	bob := covSeedUser(t, s, "bob", "bob-pass-12", "viewer")

	// 不可修改自己的角色（真实可达：admin 手滑自降）
	rec := covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserUpdate(w, r, admin.ID)
	}, rolesReq(http.MethodPut, "/api/users/"+admin.ID, `{"role":"viewer"}`, admin.ID, "admin", "admin"))
	covWantCode(t, "change own role", rec, http.StatusBadRequest)

	// 无认证上下文 → 401（D13 判定取当前用户）
	rec = covRec()
	s.handleUserUpdate(rec, covReq(http.MethodPut, "/api/users/"+bob.ID, strings.NewReader(`{"email":"x@y.z"}`)), bob.ID)
	covWantCode(t, "update no ctx", rec, http.StatusUnauthorized)

	// 改他人角色正常放行（bob → operator）
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserUpdate(w, r, bob.ID)
	}, rolesReq(http.MethodPut, "/api/users/"+bob.ID, `{"role":"operator"}`, admin.ID, "admin", "admin"))
	covWantCode(t, "change other role ok", rec, http.StatusOK)

	// 「删最后一个有效 admin」防御分支：直调绕过 RBAC，操作者无
	// users:admin，target 是唯一有效 admin → 拒
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserDelete(w, r, admin.ID)
	}, rolesReq(http.MethodDelete, "/api/users/"+admin.ID, "", bob.ID, "bob", "viewer"))
	covWantCode(t, "delete last admin", rec, http.StatusBadRequest)

	// 「降级最后一个有效 admin」防御分支：直调绕过 RBAC，bob（operator，
	// 无 users:admin）把唯一有效 admin 降为 viewer → 拒（此时 carol 尚未
	// 加入，admin 是唯一有效 admin）
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserUpdate(w, r, admin.ID)
	}, rolesReq(http.MethodPut, "/api/users/"+admin.ID, `{"role":"viewer"}`, bob.ID, "bob", "operator"))
	covWantCode(t, "demote last admin", rec, http.StatusBadRequest)

	// 「降级非最后一个 admin」放行：carol 加入后有两个有效 admin，
	// bob 降 carol → 200（排除目标后仍剩 admin）
	carol := covSeedUser(t, s, "carol", "carol-pass", "admin")
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserUpdate(w, r, carol.ID)
	}, rolesReq(http.MethodPut, "/api/users/"+carol.ID, `{"role":"operator"}`, bob.ID, "bob", "viewer"))
	covWantCode(t, "demote non-last admin ok", rec, http.StatusOK)

	// 角色侧 D13（真实可达）：boss 是唯一含 users:admin 的来源且唯一
	// 挂它的用户 erin 是全部有效 admin——削掉 users:admin 后系统再无人
	// 能管理用户 → 拒（操作者只需 roles:admin，不必持 users:admin）
	s2 := newTestServerWithDB(t)
	if err := s2.db.CreateRole(&storage.Role{Name: "boss", Permissions: []string{"users:admin", "roles:admin"}}); err != nil {
		t.Fatal(err)
	}
	covSeedUser(t, s2, "erin", "erin-pass-1", "boss")
	frank := covSeedUser(t, s2, "frank", "frank-pass-1", "viewer")
	cut := func(user *storage.User) *httptest.ResponseRecorder {
		return covCallAuth(s2, func(w http.ResponseWriter, r *http.Request) {
			s2.handleRoleUpdate(w, r, "boss")
		}, rolesReq(http.MethodPut, "/api/roles/boss", `{"permissions":["roles:admin"]}`, user.ID, user.Username, user.Role))
	}
	covWantCode(t, "demote last admin role", cut(frank), http.StatusBadRequest)

	// 补一个有效 admin（builtin admin 用户 gina）后再削 → 放行
	covSeedUser(t, s2, "gina", "gina-pass-1", "admin")
	covWantCode(t, "demote role with other admin ok", cut(frank), http.StatusOK)
}

// TestD14ForbiddenAudited D14：RBAC 403 入审计（GET 也记）
func TestD14ForbiddenAudited(t *testing.T) {
	s := newTestServerWithDB(t)
	covSeedUser(t, s, "vuser", "v-pass-1234", "viewer")

	handler := s.AuditMiddleware(s.RBACMiddleware(http.HandlerFunc(s.serveAPI)))
	token, err := s.authService().GenerateToken("u-v", "vuser", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := covRec()
	handler.ServeHTTP(rec, req)
	covWantCode(t, "viewer users 403", rec, http.StatusForbidden)

	logs, total, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{
		"status": audit.StatusFailure,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("403 audit logs = %d/%d, want 1", len(logs), total)
	}
	if logs[0].Username != "vuser" {
		t.Errorf("audit username = %q, want vuser", logs[0].Username)
	}
	if !strings.Contains(logs[0].Details, `"status_code":403`) {
		t.Errorf("audit details = %s, want status_code 403", logs[0].Details)
	}
}

// TestCountEffectiveAdminsHelper helper 直调：正常计数与 closed db err 分支
func TestCountEffectiveAdminsHelper(t *testing.T) {
	s := newTestServerWithDB(t)
	covSeedUser(t, s, "admin", "admin-pass-1", "admin")

	if !s.roleCoversPerm("admin", "users:admin") {
		t.Error("roleCoversPerm(admin, users:admin) = false, want true")
	}
	if s.roleCoversPerm("viewer", "users:admin") {
		t.Error("roleCoversPerm(viewer, users:admin) = true, want false")
	}
	if s.roleCoversPerm("ghost", "users:admin") {
		t.Error("roleCoversPerm(ghost, users:admin) = true, want false")
	}

	if n, err := s.countEffectiveAdmins("", ""); err != nil || n != 1 {
		t.Errorf("countEffectiveAdmins = %d, %v; want 1, nil", n, err)
	}
	// 排除唯一含 users:admin 的角色后零有效 admin 来源 → 0
	if n, err := s.countEffectiveAdmins("", "admin"); err != nil || n != 0 {
		t.Errorf("countEffectiveAdmins(skip admin role) = %d, %v; want 0, nil", n, err)
	}

	covCloseDB(t, s)
	if _, err := s.countEffectiveAdmins("", ""); err == nil {
		t.Error("countEffectiveAdmins on closed db = nil error, want error")
	}
}

func mustUserID(t *testing.T, s *Server, username string) string {
	t.Helper()
	u, err := s.db.GetUserByUsername(username)
	if err != nil {
		t.Fatalf("GetUserByUsername(%q): %v", username, err)
	}
	return u.ID
}
