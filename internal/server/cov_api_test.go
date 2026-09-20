package server

// api.go 的覆盖率补充测试：serveAPI 全路由分支 + 各 handler 错误/成功分支。

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ serveAPI 路由分支 ============

func TestCovServeAPIRoutes(t *testing.T) {
	s := covNewServer(t)

	cases := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"options preflight", http.MethodOptions, "/api/status", http.StatusOK},
		{"status get", http.MethodGet, "/api/status", http.StatusOK},
		{"status wrong method", http.MethodPost, "/api/status", http.StatusMethodNotAllowed},
		{"me wrong method", http.MethodPost, "/api/me", http.StatusMethodNotAllowed},
		{"me get", http.MethodGet, "/api/me", http.StatusUnauthorized},
		{"me profile wrong method", http.MethodGet, "/api/me/profile", http.StatusMethodNotAllowed},
		{"me password wrong method", http.MethodGet, "/api/me/password", http.StatusMethodNotAllowed},
		{"me password", http.MethodPost, "/api/me/password", http.StatusUnauthorized},
		{"settings bad body", http.MethodPut, "/api/settings", http.StatusBadRequest},
		{"agents list wrong method", http.MethodPost, "/api/agents", http.StatusMethodNotAllowed},
		{"agents list", http.MethodGet, "/api/agents", http.StatusOK},
		{"drift config", http.MethodGet, "/api/drift/config", http.StatusOK},
		// handleRecordings/handleServerBackups/handleDNS 以 r.URL.Path 判路径，
		// 经 serveAPI 进入时带 /api 前缀，均落到各自的 404 兜底分支
		{"recordings", http.MethodGet, "/api/recordings", http.StatusNotFound},
		{"recordings sub", http.MethodGet, "/api/recordings/xyz", http.StatusNotFound},
		{"server-backups", http.MethodGet, "/api/server-backups", http.StatusNotFound},
		{"dns", http.MethodGet, "/api/dns", http.StatusNotFound},
		{"agent files", http.MethodPost, "/api/agents/a1/files/list", http.StatusServiceUnavailable},
		{"agent proxy", http.MethodGet, "/api/agents/a1/proxy/status", http.StatusServiceUnavailable},
		{"agent cron", http.MethodGet, "/api/agents/a1/cron/jobs", http.StatusServiceUnavailable},
		{"agent logs", http.MethodPost, "/api/agents/a1/logs/query", http.StatusServiceUnavailable},
		{"agent drift", http.MethodPost, "/api/agents/a1/drift/check", http.StatusServiceUnavailable},
		{"agent secret", http.MethodGet, "/api/agents/a1/secret", http.StatusNotFound},
		{"agent get", http.MethodGet, "/api/agents/a1", http.StatusNotFound},
		{"resources domains", http.MethodGet, "/api/resources/domains", http.StatusOK},
		{"resources wrong method", http.MethodPatch, "/api/resources/domains", http.StatusMethodNotAllowed},
		{"resources unknown type", http.MethodGet, "/api/resources/unknown", http.StatusNotFound},
		{"users", http.MethodGet, "/api/users", http.StatusOK},
		{"users sub wrong method", http.MethodPatch, "/api/users/1", http.StatusMethodNotAllowed},
		{"users sub", http.MethodPut, "/api/users/1", http.StatusUnauthorized},
		{"roles list", http.MethodGet, "/api/roles", http.StatusOK},
		{"roles wrong method", http.MethodPatch, "/api/roles", http.StatusMethodNotAllowed},
		{"roles sub", http.MethodDelete, "/api/roles/ghost", http.StatusNotFound},
		{"alerts", http.MethodGet, "/api/alerts", http.StatusUnauthorized},
		{"alerts read-all", http.MethodPut, "/api/alerts/read-all", http.StatusUnauthorized},
		{"alerts action", http.MethodPut, "/api/alerts/1/read", http.StatusUnauthorized},
		{"not found", http.MethodGet, "/api/whatever", http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := covRec()
			s.serveAPI(rec, covReq(c.method, c.path, nil))
			covWantCode(t, c.name, rec, c.want)
		})
	}
}

// ============ handleSettings ============

func TestCovHandleSettings(t *testing.T) {
	s := covNewServer(t)

	// 非 PUT/POST → 405
	rec := covRec()
	s.handleSettings(rec, covReq(http.MethodGet, "/api/settings", nil))
	covWantCode(t, "get", rec, http.StatusMethodNotAllowed)

	// 非法 JSON → 400
	rec = covRec()
	s.handleSettings(rec, covReq(http.MethodPut, "/api/settings", strings.NewReader("not-json")))
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// PUT 成功
	rec = covRec()
	s.handleSettings(rec, covReq(http.MethodPut, "/api/settings", strings.NewReader(`{"theme":"dark"}`)))
	covWantCode(t, "put ok", rec, http.StatusOK)

	// POST 成功
	rec = covRec()
	s.handleSettings(rec, covReq(http.MethodPost, "/api/settings", strings.NewReader(`{}`)))
	covWantCode(t, "post ok", rec, http.StatusOK)
}

// ============ db 错误分支（关闭数据库触发）============

func TestCovHandlersDBError(t *testing.T) {
	s := covNewServer(t)
	covCloseDB(t, s)

	// handleAgentsList → 500
	rec := covRec()
	s.handleAgentsList(rec, covReq(http.MethodGet, "/api/agents", nil))
	covWantCode(t, "agents list", rec, http.StatusInternalServerError)

	// 资源列表 err → 500
	listCases := []struct {
		name string
		path string
		h    func(w http.ResponseWriter, r *http.Request)
	}{
		{"compute-instances", "/api/resources/compute-instances", s.handleComputeInstancesList},
		{"domains", "/api/resources/domains", s.handleDomainsList},
		{"certificates", "/api/resources/certificates", s.handleCertificatesList},
		{"services", "/api/resources/services", s.handleServicesList},
		{"gateways", "/api/resources/gateways", s.handleGatewaysList},
		{"storages", "/api/resources/storages", s.handleStoragesList},
	}
	for _, c := range listCases {
		rec := covRec()
		c.h(rec, covReq(http.MethodGet, c.path, nil))
		covWantCode(t, c.name, rec, http.StatusInternalServerError)
	}

	// handleUsersList → 500
	rec = covRec()
	s.handleUsersList(rec, covReq(http.MethodGet, "/api/users", nil))
	covWantCode(t, "users list", rec, http.StatusInternalServerError)

	// handleAlertsList → 500
	req := covAuthReq(http.MethodGet, "/api/alerts", nil, "1", "admin", "admin")
	rec = covCallAuth(s, s.handleAlertsList, req)
	covWantCode(t, "alerts list", rec, http.StatusInternalServerError)

	// handleAlertsList read-all → 500
	req = covAuthReq(http.MethodPut, "/api/alerts/read-all", nil, "1", "admin", "admin")
	rec = covCallAuth(s, s.handleAlertsList, req)
	covWantCode(t, "alerts read-all", rec, http.StatusInternalServerError)

	// handleMarkAlertAsRead → 500
	req = covAuthReq(http.MethodPut, "/api/alerts/a1/read", nil, "1", "admin", "admin")
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleMarkAlertAsRead(w, r, "a1")
	}, req)
	covWantCode(t, "mark read", rec, http.StatusInternalServerError)
}

// ============ handleCurrentUser 分支 ============

func TestCovHandleCurrentUserBranches(t *testing.T) {
	s := covNewServer(t)
	admin := covSeedUser(t, s, "admin", "admin12345", "admin")

	// 非 GET → 405
	rec := covRec()
	s.handleCurrentUser(rec, covReq(http.MethodPost, "/api/me", nil))
	covWantCode(t, "wrong method", rec, http.StatusMethodNotAllowed)

	// 无用户上下文 → 401
	rec = covRec()
	s.handleCurrentUser(rec, covReq(http.MethodGet, "/api/me", nil))
	covWantCode(t, "no ctx", rec, http.StatusUnauthorized)

	// 用户不存在 → 404
	rec = covCallAuth(s, s.handleCurrentUser, covAuthReq(http.MethodGet, "/api/me", nil, "ghost", "ghost", "admin"))
	covWantCode(t, "user not found", rec, http.StatusNotFound)

	// 正常（P1 笔 5：permissions 查角色表展开）
	rec = covCallAuth(s, s.handleCurrentUser, covAuthReq(http.MethodGet, "/api/me", nil, admin.ID, "admin", "admin"))
	covWantCode(t, "ok", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"permissions"`) || !strings.Contains(rec.Body.String(), `"users:admin"`) {
		t.Errorf("me body missing permissions expansion: %s", rec.Body.String())
	}

	// 幽灵角色用户 → permissions 空清单（fail-closed 一致）
	covSeedUser(t, s, "weird", "weird-pass-1", "ghost-role")
	weirdID := mustUserID(t, s, "weird")
	rec = covCallAuth(s, s.handleCurrentUser, covAuthReq(http.MethodGet, "/api/me", nil, weirdID, "weird", "ghost-role"))
	covWantCode(t, "ghost role", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"permissions":[]`) {
		t.Errorf("ghost role permissions should be empty: %s", rec.Body.String())
	}

	// 空权限自定义角色 → 空数组（null 会破坏前端 string[] 类型）
	if err := s.db.CreateRole(&storage.Role{Name: "no-op", Permissions: nil}); err != nil {
		t.Fatal(err)
	}
	covSeedUser(t, s, "nobody", "nobody-pass", "no-op")
	rec = covCallAuth(s, s.handleCurrentUser, covAuthReq(http.MethodGet, "/api/me", nil, mustUserID(t, s, "nobody"), "nobody", "no-op"))
	covWantCode(t, "empty role", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"permissions":[]`) {
		t.Errorf("empty role permissions should be []: %s", rec.Body.String())
	}
}

// ============ handleCurrentUserPassword 分支 ============

func TestCovHandleCurrentUserPasswordBranches(t *testing.T) {
	s := covNewServer(t)
	admin := covSeedUser(t, s, "admin", "admin12345", "admin")

	// 非 POST/PUT → 405
	rec := covRec()
	s.handleCurrentUserPassword(rec, covReq(http.MethodGet, "/api/me/password", nil))
	covWantCode(t, "wrong method", rec, http.StatusMethodNotAllowed)

	// 无用户上下文 → 401
	rec = covRec()
	s.handleCurrentUserPassword(rec, covReq(http.MethodPost, "/api/me/password", nil))
	covWantCode(t, "no ctx", rec, http.StatusUnauthorized)

	// 用户不存在 → 404
	rec = covCallAuth(s, s.handleCurrentUserPassword,
		covAuthReq(http.MethodPost, "/api/me/password", nil, "ghost", "ghost", "admin"))
	covWantCode(t, "user not found", rec, http.StatusNotFound)

	// 非法 JSON → 400
	rec = covCallAuth(s, s.handleCurrentUserPassword,
		covAuthReq(http.MethodPost, "/api/me/password", strings.NewReader("{"), admin.ID, "admin", "admin"))
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// 新密码为空 → 400
	rec = covCallAuth(s, s.handleCurrentUserPassword,
		covAuthReq(http.MethodPost, "/api/me/password", strings.NewReader(`{"currentPassword":"admin12345"}`), admin.ID, "admin", "admin"))
	covWantCode(t, "empty new password", rec, http.StatusBadRequest)

	// 新密码过短 → 400
	rec = covCallAuth(s, s.handleCurrentUserPassword,
		covAuthReq(http.MethodPost, "/api/me/password", strings.NewReader(`{"currentPassword":"admin12345","newPassword":"abc"}`), admin.ID, "admin", "admin"))
	covWantCode(t, "short new password", rec, http.StatusBadRequest)

	// 旧密码错误 → 401
	rec = covCallAuth(s, s.handleCurrentUserPassword,
		covAuthReq(http.MethodPost, "/api/me/password", strings.NewReader(`{"currentPassword":"wrong-old","newPassword":"newpass123"}`), admin.ID, "admin", "admin"))
	covWantCode(t, "wrong current password", rec, http.StatusUnauthorized)

	// currentPassword 字段命名 + 成功
	body := `{"currentPassword":"admin12345","newPassword":"newpass123"}`
	rec = covCallAuth(s, s.handleCurrentUserPassword,
		covAuthReq(http.MethodPut, "/api/me/password", strings.NewReader(body), admin.ID, "admin", "admin"))
	covWantCode(t, "alt fields ok", rec, http.StatusOK)

	// 超长新密码 → 哈希失败 500（密码已改为 newpass123）
	long := `{"old_password":"newpass123","newPassword":"` + strings.Repeat("x", 100) + `"}`
	rec = covCallAuth(s, s.handleCurrentUserPassword,
		covAuthReq(http.MethodPost, "/api/me/password", strings.NewReader(long), admin.ID, "admin", "admin"))
	covWantCode(t, "hash error", rec, http.StatusInternalServerError)
}

// ============ handleCurrentUserProfile 分支 ============

func TestCovHandleCurrentUserProfileBranches(t *testing.T) {
	s := covNewServer(t)
	admin := covSeedUser(t, s, "admin", "admin12345", "admin")

	// 405
	rec := covRec()
	s.handleCurrentUserProfile(rec, covReq(http.MethodGet, "/api/me/profile", nil))
	covWantCode(t, "wrong method", rec, http.StatusMethodNotAllowed)

	// 401
	rec = covRec()
	s.handleCurrentUserProfile(rec, covReq(http.MethodPut, "/api/me/profile", nil))
	covWantCode(t, "no ctx", rec, http.StatusUnauthorized)

	// 用户不存在 → 404
	rec = covCallAuth(s, s.handleCurrentUserProfile,
		covAuthReq(http.MethodPut, "/api/me/profile", nil, "ghost", "ghost", "admin"))
	covWantCode(t, "user not found", rec, http.StatusNotFound)

	// 非法 JSON → 400
	rec = covCallAuth(s, s.handleCurrentUserProfile,
		covAuthReq(http.MethodPut, "/api/me/profile", strings.NewReader("not-json"), admin.ID, "admin", "admin"))
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// 指针字段清空 email
	rec = covCallAuth(s, s.handleCurrentUserProfile,
		covAuthReq(http.MethodPut, "/api/me/profile", strings.NewReader(`{"email":"","phone":"123","department":"ops"}`), admin.ID, "admin", "admin"))
	covWantCode(t, "clear email", rec, http.StatusOK)

	// POST 成功（字段全不传）
	rec = covCallAuth(s, s.handleCurrentUserProfile,
		covAuthReq(http.MethodPost, "/api/me/profile", strings.NewReader(`{}`), admin.ID, "admin", "admin"))
	covWantCode(t, "post ok", rec, http.StatusOK)
}

// ============ handleAgentGet / handleAgentSecret ============

func TestCovHandleAgentGetWrongMethod(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleAgentGet(rec, covReq(http.MethodPost, "/api/agents/a1", nil), "a1")
	covWantCode(t, "agent get", rec, http.StatusMethodNotAllowed)
}

func TestCovHandleAgentSecretBranches(t *testing.T) {
	s := covNewServer(t)
	if err := s.db.UpsertAgent(&storage.Agent{ID: "ag1", Status: "online", Hostname: "h1"}); err != nil {
		t.Fatal(err)
	}

	// 管理员 + agent 不存在 → 404
	rec := covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleAgentSecret(w, r, "ghost")
	}, covAuthReq(http.MethodGet, "/api/agents/ghost/secret", nil, "1", "admin", "admin"))
	covWantCode(t, "agent not found", rec, http.StatusNotFound)

	// GET 查看密钥状态
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleAgentSecret(w, r, "ag1")
	}, covAuthReq(http.MethodGet, "/api/agents/ag1/secret", nil, "1", "admin", "admin"))
	covWantCode(t, "get status", rec, http.StatusOK)

	// POST 重新生成密钥
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleAgentSecret(w, r, "ag1")
	}, covAuthReq(http.MethodPost, "/api/agents/ag1/secret", nil, "1", "admin", "admin"))
	covWantCode(t, "regenerate", rec, http.StatusOK)

	// DELETE 删除密钥
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleAgentSecret(w, r, "ag1")
	}, covAuthReq(http.MethodDelete, "/api/agents/ag1/secret", nil, "1", "admin", "admin"))
	covWantCode(t, "remove", rec, http.StatusOK)

	// 其他方法 → 405
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleAgentSecret(w, r, "ag1")
	}, covAuthReq(http.MethodPatch, "/api/agents/ag1/secret", nil, "1", "admin", "admin"))
	covWantCode(t, "wrong method", rec, http.StatusMethodNotAllowed)
}

// ============ 资源 get 成功 / 不存在分支 ============

func TestCovResourceGetSuccessAndMissing(t *testing.T) {
	s := covNewServer(t)

	now := time.Now()
	if err := s.db.UpsertComputeInstance(&storage.ComputeInstance{ID: "ci-1", Name: "vm", AgentID: "a1", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertDomain(&storage.Domain{ID: "d-1", Domain: "example.com", Status: "active", ExpiresAt: &now}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertCertificate(&storage.Certificate{ID: "c-1", DomainName: "example.com", Status: "valid", ExpiresAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertService(&storage.Service{ID: "s-1", Name: "web", Status: "up"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertGateway(&storage.Gateway{ID: "g-1", Name: "gw", AgentID: "a1", Status: "up"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertStorage(&storage.Storage{ID: "st-1", Name: "disk", AgentID: "a1", Status: "ok"}); err != nil {
		t.Fatal(err)
	}

	gets := []struct {
		name  string
		rtype string
		path  string
		id    string
		h     func(w http.ResponseWriter, r *http.Request, id string)
	}{
		{"compute instance", "compute-instances", "/api/resources/compute-instances/ci-1", "ci-1", s.handleComputeInstanceGet},
		{"domain", "domains", "/api/resources/domains/d-1", "d-1", s.handleDomainGet},
		{"certificate", "certificates", "/api/resources/certificates/c-1", "c-1", s.handleCertificateGet},
		{"service", "services", "/api/resources/services/s-1", "s-1", s.handleServiceGet},
		{"gateway", "gateways", "/api/resources/gateways/g-1", "g-1", s.handleGatewayGet},
		{"storage", "storages", "/api/resources/storages/st-1", "st-1", s.handleStorageGet},
	}
	for _, c := range gets {
		rec := covRec()
		c.h(rec, covReq(http.MethodGet, c.path, nil), c.id)
		covWantCode(t, c.name+" get", rec, http.StatusOK)

		rec = covRec()
		c.h(rec, covReq(http.MethodGet, c.path, nil), "missing-id")
		covWantCode(t, c.name+" missing", rec, http.StatusNotFound)

		// 经 handleResources 分发：列表与单查
		rec = covRec()
		s.handleResources(rec, covReq(http.MethodGet, c.path, nil), c.rtype)
		covWantCode(t, c.name+" list dispatch", rec, http.StatusOK)

		rec = covRec()
		s.handleResources(rec, covReq(http.MethodGet, c.path, nil), c.rtype+"/"+c.id)
		covWantCode(t, c.name+" get dispatch", rec, http.StatusOK)
	}
}

// handleAgentGet 200 与 agents 列表循环体（storageAgentToResponse）
func TestCovAgentGetAndListWithAgent(t *testing.T) {
	s := covNewServer(t)
	if err := s.db.UpsertAgent(&storage.Agent{ID: "ag1", Status: "online", Hostname: "h1", IP: "1.2.3.4"}); err != nil {
		t.Fatal(err)
	}

	rec := covRec()
	s.handleAgentGet(rec, covReq(http.MethodGet, "/api/agents/ag1", nil), "ag1")
	covWantCode(t, "agent get ok", rec, http.StatusOK)

	rec = covRec()
	s.handleAgentsList(rec, covReq(http.MethodGet, "/api/agents", nil))
	covWantCode(t, "agents list ok", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "ag1") {
		t.Errorf("agents list body missing ag1: %s", rec.Body.String())
	}
}

// ============ 用户管理分支 ============

func covSeedUser(t *testing.T, s *Server, username, password, role string) *storage.User {
	t.Helper()
	hashed, err := storage.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	u := &storage.User{Username: username, Password: hashed, Role: role, Email: username + "@test.local"}
	if err := s.db.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestCovHandleUsersWrongMethod(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleUsers(rec, covReq(http.MethodPatch, "/api/users", nil))
	covWantCode(t, "patch users", rec, http.StatusMethodNotAllowed)
}

func TestCovHandleUserCreateBranches(t *testing.T) {
	s := covNewServer(t)
	admin := covSeedUser(t, s, "admin", "admin12345", "admin")
	// 401/403 判定已收敛到外层 auth + RBAC 中间件（rbac_test.go 矩阵）

	// 非法 JSON → 400
	rec := covCallAuth(s, s.handleUserCreate,
		covAuthReq(http.MethodPost, "/api/users", strings.NewReader("not-json"), admin.ID, "admin", "admin"))
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// 缺用户名/密码 → 400
	rec = covCallAuth(s, s.handleUserCreate,
		covAuthReq(http.MethodPost, "/api/users", strings.NewReader(`{}`), admin.ID, "admin", "admin"))
	covWantCode(t, "empty fields", rec, http.StatusBadRequest)

	// 幽灵角色 → 400（RBAC fail-closed 防锁号）
	rec = covCallAuth(s, s.handleUserCreate,
		covAuthReq(http.MethodPost, "/api/users", strings.NewReader(`{"username":"u1","password":"goodpass","role":"user"}`), admin.ID, "admin", "admin"))
	covWantCode(t, "unknown role", rec, http.StatusBadRequest)

	// 用户名已存在 → 409
	rec = covCallAuth(s, s.handleUserCreate,
		covAuthReq(http.MethodPost, "/api/users", strings.NewReader(`{"username":"admin","password":"pass12345"}`), admin.ID, "admin", "admin"))
	covWantCode(t, "duplicate username", rec, http.StatusConflict)

	// 超长密码 → 哈希失败 500
	long := `{"username":"u1","password":"` + strings.Repeat("p", 100) + `"}`
	rec = covCallAuth(s, s.handleUserCreate,
		covAuthReq(http.MethodPost, "/api/users", strings.NewReader(long), admin.ID, "admin", "admin"))
	covWantCode(t, "hash error", rec, http.StatusInternalServerError)

	// 创建成功 → 201
	rec = covCallAuth(s, s.handleUserCreate,
		covAuthReq(http.MethodPost, "/api/users", strings.NewReader(`{"username":"u1","password":"goodpass","role":"viewer"}`), admin.ID, "admin", "admin"))
	covWantCode(t, "created", rec, http.StatusCreated)

	// closed db：GetRole 非 NotFound 放行、CreateUser 失败 → 500
	s2 := covNewServer(t)
	covCloseDB(t, s2)
	rec = covCallAuth(s2, s2.handleUserCreate,
		covAuthReq(http.MethodPost, "/api/users", strings.NewReader(`{"username":"u1","password":"goodpass"}`), "1", "admin", "admin"))
	covWantCode(t, "create error", rec, http.StatusInternalServerError)
}

func TestCovHandleUserDeleteBranches(t *testing.T) {
	s := covNewServer(t)
	admin := covSeedUser(t, s, "admin", "admin12345", "admin")
	bob := covSeedUser(t, s, "bob", "bobpass123", "user")

	// 无上下文 → 401
	rec := covRec()
	s.handleUserDelete(rec, covReq(http.MethodDelete, "/api/users/"+bob.ID, nil), bob.ID)
	covWantCode(t, "no ctx", rec, http.StatusUnauthorized)

	// 目标不存在 → 404
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserDelete(w, r, "ghost")
	}, covAuthReq(http.MethodDelete, "/api/users/ghost", nil, admin.ID, "admin", "admin"))
	covWantCode(t, "not found", rec, http.StatusNotFound)

	// 删除自己 → 400
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserDelete(w, r, admin.ID)
	}, covAuthReq(http.MethodDelete, "/api/users/"+admin.ID, nil, admin.ID, "admin", "admin"))
	covWantCode(t, "delete self", rec, http.StatusBadRequest)

	// admin 删除他人 → 200（非 admin 判定已收敛到 RBAC 中间件）
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserDelete(w, r, bob.ID)
	}, covAuthReq(http.MethodDelete, "/api/users/"+bob.ID, nil, admin.ID, "admin", "admin"))
	covWantCode(t, "deleted", rec, http.StatusOK)
}

func TestCovHandleUserChangePasswordBranches(t *testing.T) {
	s := covNewServer(t)
	admin := covSeedUser(t, s, "admin", "admin12345", "admin")
	bob := covSeedUser(t, s, "bob", "bobpass123", "user")
	// 401 语义收敛到外层 auth + RBAC 中间件（rbac_test.go 矩阵）

	// 无认证上下文：userHasPerm 判 false → 走验旧密码分支 → 401
	rec := covRec()
	s.handleUserChangePassword(rec, covReq(http.MethodPost, "/api/users/"+bob.ID+"/password",
		strings.NewReader(`{"old_password":"wrong","new_password":"newpass123"}`)), bob.ID)
	covWantCode(t, "no ctx wrong old password", rec, http.StatusUnauthorized)

	// 目标不存在 → 404
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserChangePassword(w, r, "ghost")
	}, covAuthReq(http.MethodPost, "/api/users/ghost/password", nil, admin.ID, "admin", "admin"))
	covWantCode(t, "not found", rec, http.StatusNotFound)

	// 无 users:admin 权限改他人密码 → 走验旧密码分支，旧密码错 → 401
	body := `{"old_password":"x","new_password":"newpass123"}`
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserChangePassword(w, r, admin.ID)
	}, covAuthReq(http.MethodPost, "/api/users/"+admin.ID+"/password", strings.NewReader(body), bob.ID, "bob", "user"))
	covWantCode(t, "no perm wrong old password", rec, http.StatusUnauthorized)

	// 非法 JSON → 400
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserChangePassword(w, r, bob.ID)
	}, covAuthReq(http.MethodPost, "/api/users/"+bob.ID+"/password", strings.NewReader("not-json"), bob.ID, "bob", "user"))
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// 新密码为空 → 400
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserChangePassword(w, r, bob.ID)
	}, covAuthReq(http.MethodPost, "/api/users/"+bob.ID+"/password", strings.NewReader(`{}`), bob.ID, "bob", "user"))
	covWantCode(t, "empty new password", rec, http.StatusBadRequest)

	// 非 admin 修改自己密码、旧密码错 → 401
	body = `{"old_password":"wrong-old","new_password":"newpass123"}`
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserChangePassword(w, r, bob.ID)
	}, covAuthReq(http.MethodPost, "/api/users/"+bob.ID+"/password", strings.NewReader(body), bob.ID, "bob", "user"))
	covWantCode(t, "wrong old password", rec, http.StatusUnauthorized)

	// 非 admin 修改自己密码、旧密码正确 → 200
	body = `{"old_password":"bobpass123","new_password":"bobpass456"}`
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserChangePassword(w, r, bob.ID)
	}, covAuthReq(http.MethodPost, "/api/users/"+bob.ID+"/password", strings.NewReader(body), bob.ID, "bob", "user"))
	covWantCode(t, "self change ok", rec, http.StatusOK)

	// admin 修改他人密码、超长新密码 → 哈希失败 500
	long := `{"new_password":"` + strings.Repeat("p", 100) + `"}`
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserChangePassword(w, r, bob.ID)
	}, covAuthReq(http.MethodPost, "/api/users/"+bob.ID+"/password", strings.NewReader(long), admin.ID, "admin", "admin"))
	covWantCode(t, "admin hash error", rec, http.StatusInternalServerError)

	// admin 修改他人密码成功（免旧密码分支）
	body = `{"new_password":"bobpass789"}`
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserChangePassword(w, r, bob.ID)
	}, covAuthReq(http.MethodPost, "/api/users/"+bob.ID+"/password", strings.NewReader(body), admin.ID, "admin", "admin"))
	covWantCode(t, "admin change ok", rec, http.StatusOK)
}

func TestCovHandleUserUpdateBranches(t *testing.T) {
	s := covNewServer(t)
	admin := covSeedUser(t, s, "admin", "admin12345", "admin")
	bob := covSeedUser(t, s, "bob", "bobpass123", "user")
	// 401/403 语义收敛到外层 auth + RBAC 中间件（rbac_test.go 矩阵）

	// 目标不存在 → 404
	rec := covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserUpdate(w, r, "ghost")
	}, covAuthReq(http.MethodPut, "/api/users/ghost", nil, admin.ID, "admin", "admin"))
	covWantCode(t, "not found", rec, http.StatusNotFound)

	// 非法 JSON → 400
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserUpdate(w, r, bob.ID)
	}, covAuthReq(http.MethodPut, "/api/users/"+bob.ID, strings.NewReader("not-json"), bob.ID, "bob", "user"))
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	// 幽灵角色 → 400（角色表校验防锁号）
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserUpdate(w, r, bob.ID)
	}, covAuthReq(http.MethodPut, "/api/users/"+bob.ID, strings.NewReader(`{"role":"user"}`), bob.ID, "bob", "user"))
	covWantCode(t, "unknown role", rec, http.StatusBadRequest)

	// bob 更新自己邮箱（不改角色）→ 200
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserUpdate(w, r, bob.ID)
	}, covAuthReq(http.MethodPut, "/api/users/"+bob.ID, strings.NewReader(`{"email":"bob2@test.local"}`), bob.ID, "bob", "user"))
	covWantCode(t, "self update ok", rec, http.StatusOK)

	// admin 更新他人角色 → 200
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleUserUpdate(w, r, bob.ID)
	}, covAuthReq(http.MethodPut, "/api/users/"+bob.ID, strings.NewReader(`{"role":"viewer","email":"bob3@test.local"}`), admin.ID, "admin", "admin"))
	covWantCode(t, "admin update ok", rec, http.StatusOK)
}

// ============ 警告分支 ============

func TestCovHandleAlertsBranches(t *testing.T) {
	s := covNewServer(t)

	// 无上下文 GET → 401
	rec := covRec()
	s.handleAlertsList(rec, covReq(http.MethodGet, "/api/alerts", nil))
	covWantCode(t, "list no ctx", rec, http.StatusUnauthorized)

	// 无上下文 read-all → 401
	rec = covRec()
	s.handleAlertsList(rec, covReq(http.MethodPut, "/api/alerts/read-all", nil))
	covWantCode(t, "read-all no ctx", rec, http.StatusUnauthorized)

	// 有上下文 + 非 GET/read-all → 405
	rec = covCallAuth(s, s.handleAlertsList,
		covAuthReq(http.MethodPost, "/api/alerts", nil, "1", "admin", "admin"))
	covWantCode(t, "method not allowed", rec, http.StatusMethodNotAllowed)

	// read-all 成功 → 200
	rec = covCallAuth(s, s.handleAlertsList,
		covAuthReq(http.MethodPut, "/api/alerts/read-all", nil, "1", "admin", "admin"))
	covWantCode(t, "read-all ok", rec, http.StatusOK)

	// 列表成功 → 200
	rec = covCallAuth(s, s.handleAlertsList,
		covAuthReq(http.MethodGet, "/api/alerts", nil, "1", "admin", "admin"))
	covWantCode(t, "list ok", rec, http.StatusOK)
}

func TestCovHandleAlertActionsBranches(t *testing.T) {
	s := covNewServer(t)
	if err := s.db.CreateAlert(&storage.Alert{Type: "warning", Title: "t", Message: "m"}); err != nil {
		t.Fatal(err)
	}
	logs, err := s.db.ListAlerts(10)
	if err != nil || len(logs) == 0 {
		t.Fatalf("seed alert: %v %d", err, len(logs))
	}
	alertID := logs[0].ID

	// 无上下文 → 401
	rec := covRec()
	s.handleAlertActions(rec, covReq(http.MethodPut, "/api/alerts/"+alertID+"/read", nil), alertID+"/read")
	covWantCode(t, "no ctx", rec, http.StatusUnauthorized)

	// 标记已读成功 → 200
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleAlertActions(w, r, alertID+"/read")
	}, covAuthReq(http.MethodPut, "/api/alerts/"+alertID+"/read", nil, "1", "admin", "admin"))
	covWantCode(t, "mark read ok", rec, http.StatusOK)

	// 其他 action → 405
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleAlertActions(w, r, alertID+"/other")
	}, covAuthReq(http.MethodPut, "/api/alerts/"+alertID+"/other", nil, "1", "admin", "admin"))
	covWantCode(t, "unknown action", rec, http.StatusMethodNotAllowed)
}
