package storage

// role_test.go RBAC 笔 1（rbac-design.md 实现状态表）：内置角色 seed
// 幂等与覆盖策略、D9 存量迁移、自定义角色 CRUD 白名单与保护。

import (
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// openSeededDB Open 即自动 seed 内置角色（migrate 尾部调 SeedRoles）
func openSeededDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSeedRolesBuiltin(t *testing.T) {
	db := openSeededDB(t)

	for _, b := range builtinRoles {
		got, err := db.GetRole(b.name)
		if err != nil {
			t.Fatalf("builtin %s missing after Open: %v", b.name, err)
		}
		if !got.Builtin {
			t.Errorf("builtin %s not marked builtin", b.name)
		}
		if len(got.Permissions) != len(b.permissions) {
			t.Errorf("%s permissions = %d, want %d", b.name, len(got.Permissions), len(b.permissions))
		}
	}

	// 内置角色构成性质（D8）
	admin, _ := db.GetRole("admin")
	if len(admin.Permissions) != len(allPermissions()) {
		t.Errorf("admin should carry all permissions, got %d", len(admin.Permissions))
	}
	operator, _ := db.GetRole("operator")
	for _, p := range operator.Permissions {
		if p == "users:admin" || p == "roles:admin" || p == "settings:admin" {
			t.Errorf("operator must not carry %s", p)
		}
	}
	if !contains(operator.Permissions, "terminal:write") || !contains(operator.Permissions, "acme:admin") {
		t.Errorf("operator should carry terminal:write and acme:admin, got %v", operator.Permissions)
	}
	viewer, _ := db.GetRole("viewer")
	for _, p := range viewer.Permissions {
		if !strings.HasSuffix(p, ":read") {
			t.Errorf("viewer must be read-only, got %s", p)
		}
	}
	if !contains(viewer.Permissions, "audit:read") || contains(viewer.Permissions, "terminal:write") {
		t.Errorf("viewer should carry audit:read but no terminal:write, got %v", viewer.Permissions)
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func TestSeedRolesIdempotentAndOverrides(t *testing.T) {
	db := openSeededDB(t)

	// 直改内置角色 permissions（模拟绕过 API 的库层改动）→ 重复 seed 覆盖恢复
	admin, _ := db.GetRole("admin")
	admin.Permissions = []string{"dns:read"}
	db.db.Save(admin)
	if err := db.SeedRoles(); err != nil {
		t.Fatal(err)
	}
	admin, _ = db.GetRole("admin")
	if len(admin.Permissions) != len(allPermissions()) {
		t.Errorf("seed should restore builtin permissions, got %d", len(admin.Permissions))
	}

	// 撞内置角色名的自定义角色 → seed 收编为 builtin（定义以代码为准）
	db.db.Exec("UPDATE roles SET builtin = 0 WHERE name = 'viewer'")
	if err := db.SeedRoles(); err != nil {
		t.Fatal(err)
	}
	viewer, _ := db.GetRole("viewer")
	if !viewer.Builtin {
		t.Error("seed should reclaim custom role named after builtin")
	}
}

func TestSeedRolesMigratesLegacyUser(t *testing.T) {
	db := openSeededDB(t)
	db.CreateUser(&User{ID: "u-legacy", Username: "legacy", Password: "h", Role: "user"})
	db.CreateUser(&User{ID: "u-admin", Username: "root", Password: "h", Role: "admin"})

	if err := db.SeedRoles(); err != nil {
		t.Fatal(err)
	}
	legacy, _ := db.GetUserByUsername("legacy")
	if legacy.Role != "viewer" {
		t.Errorf("legacy role=user should migrate to viewer, got %q", legacy.Role)
	}
	root, _ := db.GetUserByUsername("root")
	if root.Role != "admin" {
		t.Errorf("role=admin must stay, got %q", root.Role)
	}
}

func TestRoleCRUD(t *testing.T) {
	db := openSeededDB(t)

	// 自定义角色创建成功
	role := &Role{Name: "deployer", Permissions: []string{"stack:write", "docker:read"}}
	if err := db.CreateRole(role); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetRole("deployer")
	if !contains(got.Permissions, "stack:write") {
		t.Errorf("create lost permissions: %v", got.Permissions)
	}

	// 空名 / 撞内置名 / 非法权限点
	if err := db.CreateRole(&Role{Name: "", Permissions: nil}); !errors.Is(err, ErrEmptyRoleName) {
		t.Errorf("empty name: got %v", err)
	}
	if err := db.CreateRole(&Role{Name: "admin", Permissions: nil}); !errors.Is(err, ErrBuiltinRole) {
		t.Errorf("builtin name collision: got %v", err)
	}
	if err := db.CreateRole(&Role{Name: "bad", Permissions: []string{"dns:execute"}}); !errors.Is(err, ErrInvalidPermission) {
		t.Errorf("invalid permission: got %v", err)
	}
	if err := db.CreateRole(&Role{Name: "bad2", Permissions: []string{"hack:read"}}); !errors.Is(err, ErrInvalidPermission) {
		t.Errorf("unknown resource: got %v", err)
	}

	// 更新：自定义 OK、内置拒绝、非法权限拒绝
	if err := db.UpdateRole(&Role{Name: "deployer", Permissions: []string{"cron:write"}}); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetRole("deployer")
	if len(got.Permissions) != 1 || got.Permissions[0] != "cron:write" {
		t.Errorf("update lost permissions: %v", got.Permissions)
	}
	if err := db.UpdateRole(&Role{Name: "operator", Permissions: nil}); !errors.Is(err, ErrBuiltinRole) {
		t.Errorf("update builtin: got %v", err)
	}
	if err := db.UpdateRole(&Role{Name: "deployer", Permissions: []string{"x:y"}}); !errors.Is(err, ErrInvalidPermission) {
		t.Errorf("update invalid permission: got %v", err)
	}
	if err := db.UpdateRole(&Role{Name: "ghost", Permissions: nil}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: got %v", err)
	}

	// 删除：内置拒绝、被引用拒绝、成功
	if err := db.DeleteRole("viewer"); !errors.Is(err, ErrBuiltinRole) {
		t.Errorf("delete builtin: got %v", err)
	}
	db.CreateUser(&User{ID: "u-dep", Username: "dep", Password: "h", Role: "deployer"})
	if err := db.DeleteRole("deployer"); !errors.Is(err, ErrRoleInUse) {
		t.Errorf("delete in-use role: got %v", err)
	}
	if err := db.DeleteRole("ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing: got %v", err)
	}
	db.db.Exec("UPDATE users SET role = 'viewer' WHERE id = 'u-dep'")
	if err := db.DeleteRole("deployer"); err != nil {
		t.Fatalf("delete unreferenced role: %v", err)
	}
	if _, err := db.GetRole("deployer"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted role should be gone, got %v", err)
	}

	// 列表排序
	roles, err := db.ListRoles()
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(roles); i++ {
		if roles[i-1].Name > roles[i].Name {
			t.Errorf("ListRoles not sorted: %v", roles)
		}
	}
}

// TestCountUsersByRoles D13 有效 admin 计数的查询半边：集合过滤与
// skipUserID 排除目标用户本身
func TestCountUsersByRoles(t *testing.T) {
	db := openSeededDB(t)
	db.CreateUser(&User{ID: "u1", Username: "a1", Password: "h", Role: "admin"})
	db.CreateUser(&User{ID: "u2", Username: "a2", Password: "h", Role: "admin"})
	db.CreateUser(&User{ID: "u3", Username: "v1", Password: "h", Role: "viewer"})

	if n, err := db.CountUsersByRoles([]string{"admin"}, ""); err != nil || n != 2 {
		t.Errorf("count admin = %d, %v; want 2, nil", n, err)
	}
	// 排除目标用户本身（假想变更后不计自己）
	if n, err := db.CountUsersByRoles([]string{"admin"}, "u1"); err != nil || n != 1 {
		t.Errorf("count admin skip u1 = %d, %v; want 1, nil", n, err)
	}
	// 多角色集合
	if n, err := db.CountUsersByRoles([]string{"admin", "viewer"}, ""); err != nil || n != 3 {
		t.Errorf("count admin+viewer = %d, %v; want 3, nil", n, err)
	}
	// 空集合（countEffectiveAdmins 筛后无人时提前 return，不走到这）
	if n, err := db.CountUsersByRoles([]string{"ghost"}, ""); err != nil || n != 0 {
		t.Errorf("count ghost = %d, %v; want 0, nil", n, err)
	}

	db.Close()
	if _, err := db.CountUsersByRoles([]string{"admin"}, ""); err == nil {
		t.Error("CountUsersByRoles on closed db should fail")
	}
}

func TestPermissionValid(t *testing.T) {
	for _, p := range []string{"dns:write", "acme:admin", "logs:read", "terminal:write"} {
		if !PermissionValid(p) {
			t.Errorf("%s should be valid", p)
		}
	}
	for _, p := range []string{"dns:execute", "hack:read", "dns", ":write", "dns:", ""} {
		if PermissionValid(p) {
			t.Errorf("%s should be invalid", p)
		}
	}
}

func TestSeedRolesWriteFailures(t *testing.T) {
	db := openSeededDB(t)
	boom := errors.New("boom")

	// Create 失败：删一行走 NotFound→Create，注入 create 回调失败
	db.db.Exec("DELETE FROM roles WHERE name = 'viewer'")
	db.db.Callback().Create().Before("gorm:create").Register("test/boom", func(tx *gorm.DB) {
		tx.AddError(boom)
	})
	err := db.SeedRoles()
	db.db.Callback().Create().Remove("test/boom")
	if !errors.Is(err, boom) {
		t.Fatalf("seed should fail on create error, got %v", err)
	}

	// Save 失败：制造与代码不一致（覆盖 Save 分支），注入 update 回调失败
	admin, _ := db.GetRole("admin")
	admin.Permissions = []string{"dns:read"}
	db.db.Save(admin)
	db.db.Callback().Update().Before("gorm:update").Register("test/boom", func(tx *gorm.DB) {
		tx.AddError(boom)
	})
	err = db.SeedRoles()
	db.db.Callback().Update().Remove("test/boom")
	if !errors.Is(err, boom) {
		t.Fatalf("seed should fail on save error, got %v", err)
	}
}

func TestSeedRolesLegacyCountFailure(t *testing.T) {
	db := openSeededDB(t)
	boom := errors.New("boom")

	// 稳态（角色一致、零写）走到 D9 迁移的 Count；只对 users 表注入
	// 查询失败（roles 表的 First 不受影响）
	db.db.Callback().Query().Before("gorm:query").Register("test/count-boom", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(boom)
		}
	})
	err := db.SeedRoles()
	db.db.Callback().Query().Remove("test/count-boom")
	if !errors.Is(err, boom) {
		t.Fatalf("seed should fail on legacy count error, got %v", err)
	}
}

func TestSamePermissions(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{[]string{"dns:read", "dns:write"}, []string{"dns:write", "dns:read"}, true},
		{[]string{"dns:read"}, []string{"dns:read", "dns:write"}, false},
		{[]string{"dns:read", "dns:write"}, []string{"dns:read", "hack:read"}, false},
		{nil, nil, true},
	}
	for _, c := range cases {
		if got := samePermissions(c.a, c.b); got != c.want {
			t.Errorf("samePermissions(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestRoleClosedDB(t *testing.T) {
	db := openSeededDB(t)
	db.Close()

	if _, err := db.ListRoles(); err == nil {
		t.Error("ListRoles on closed db should fail")
	}
	if _, err := db.GetRole("admin"); err == nil {
		t.Error("GetRole on closed db should fail")
	}
	role := &Role{Name: "x", Permissions: []string{"dns:read"}}
	if err := db.CreateRole(role); err == nil {
		t.Error("CreateRole on closed db should fail")
	}
	if err := db.UpdateRole(role); err == nil {
		t.Error("UpdateRole on closed db should fail")
	}
	if err := db.DeleteRole("admin"); err == nil {
		t.Error("DeleteRole on closed db should fail")
	}
	if err := db.SeedRoles(); err == nil {
		t.Error("SeedRoles on closed db should fail")
	}
}
