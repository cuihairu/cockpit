package storage

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 角色与权限点（见 docs/guide/rbac-design.md）：用户 → 角色 → 权限点。
// 本文件是权限点合法集合的单一事实源（resourceActions 表），内置角色
// 的权限清单与角色写入校验都从它派生；server 侧路由权限表按字符串引用。

var (
	// ErrBuiltinRole 内置角色不可修改/删除（D12：定义以代码为准）
	ErrBuiltinRole = errors.New("builtin role cannot be modified or deleted")
	// ErrRoleInUse 角色仍被用户引用（软引用删除会让这些用户 fail-closed 全拒）
	ErrRoleInUse = errors.New("role is still assigned to users")
	// ErrInvalidPermission 权限点不在合法集合内（自定义角色白名单校验）
	ErrInvalidPermission = errors.New("permission not in the allowed set")
	// ErrEmptyRoleName 角色名为空
	ErrEmptyRoleName = errors.New("role name is required")
)

// resourceActions D3 权限点清单：resource → 允许的 action 集。
// action ∈ read/write/admin；约定 write ⊇ read、admin ⊇ write（判定在 server 侧）。
var resourceActions = map[string][]string{
	"inventory": {"read", "write"},
	"files":     {"read", "write"},
	"logs":      {"read"},
	"terminal":  {"write"},
	"docker":    {"read", "write"},
	"stack":     {"read", "write"},
	"cron":      {"read", "write"},
	"backup":    {"read", "write"},
	"acme":      {"read", "write", "admin"},
	"dns":       {"read", "write"},
	"ddns":      {"read", "write"},
	"proxy":     {"read", "write"},
	"overlay":   {"read", "write"},
	"drift":     {"read", "write"},
	"nas":       {"read", "write"},
	"alerts":    {"read", "write"},
	"audit":     {"read"},
	"users":     {"admin"},
	"roles":     {"admin"},
	"settings":  {"admin"},
}

// PermissionValid 权限点是否落在 D3 清单内
func PermissionValid(perm string) bool {
	i := strings.LastIndex(perm, ":")
	if i <= 0 {
		return false
	}
	for _, a := range resourceActions[perm[:i]] {
		if perm[i+1:] == a {
			return true
		}
	}
	return false
}

// allPermissions 全部合法权限点（admin 角色即此清单）
func allPermissions() []string {
	var out []string
	// 固定 resource 顺序使 seed 结果稳定（map 遍历乱序）
	for _, res := range []string{
		"inventory", "files", "logs", "terminal", "docker", "stack", "cron",
		"backup", "acme", "dns", "ddns", "proxy", "overlay", "drift", "nas",
		"alerts", "audit", "users", "roles", "settings",
	} {
		for _, a := range resourceActions[res] {
			out = append(out, res+":"+a)
		}
	}
	return out
}

// operatorPermissions 全部模块 read+write（含 terminal:write、acme:admin），
// 无 users/roles/settings（D8）
func operatorPermissions() []string {
	out := make([]string, 0, len(allPermissions()))
	for _, p := range allPermissions() {
		switch p {
		case "users:admin", "roles:admin", "settings:admin":
		default:
			out = append(out, p)
		}
	}
	return out
}

// viewerPermissions 全模块只读，无 terminal（D8）
func viewerPermissions() []string {
	var out []string
	for _, p := range allPermissions() {
		if strings.HasSuffix(p, ":read") {
			out = append(out, p)
		}
	}
	return out
}

// builtinRoles 内置角色定义（D8）；seed 时 builtin 角色 permissions 以此为准
var builtinRoles = []struct {
	name        string
	permissions []string
}{
	{"admin", allPermissions()},
	{"operator", operatorPermissions()},
	{"viewer", viewerPermissions()},
}

// Role 角色表（D4）：name 主键、permissions JSON、builtin 标记。
// User.role 沿用字符串存角色名（软引用，无外键）。
type Role struct {
	Name        string    `gorm:"primaryKey;size:64" json:"name"`
	Permissions []string  `gorm:"serializer:json" json:"permissions"`
	Builtin     bool      `json:"builtin"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListRoles 全部角色（按名字排序）
func (d *DB) ListRoles() ([]*Role, error) {
	var list []*Role
	if err := d.db.Order("name").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// GetRole 按名字取角色
func (d *DB) GetRole(name string) (*Role, error) {
	var r Role
	if err := d.db.First(&r, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &r, nil
}

// CreateRole 新建自定义角色：名字非空且不撞内置角色、权限点全部合法
func (d *DB) CreateRole(role *Role) error {
	if err := validateCustomRole(role); err != nil {
		return err
	}
	return d.db.Create(role).Error
}

// UpdateRole 更新自定义角色的权限清单（内置角色拒绝）
func (d *DB) UpdateRole(role *Role) error {
	existing, err := d.GetRole(role.Name)
	if err != nil {
		return err
	}
	if existing.Builtin {
		return ErrBuiltinRole
	}
	if err := validatePermissions(role.Permissions); err != nil {
		return err
	}
	// serializer:json 字段不走 Updates(map) 的 schema 序列化，手动 JSON
	// （同 agentUpdateFields 先例）；[]string 序列化必成功，忽略 err
	b, _ := json.Marshal(role.Permissions)
	return d.db.Model(existing).Updates(map[string]interface{}{
		"permissions": string(b),
	}).Error
}

// DeleteRole 删除自定义角色（内置拒绝；仍被用户引用拒绝——删了会让
// 这些用户查不到角色而 fail-closed 全拒，等价锁号）。引用计数在角色
// 读取之前（顺序不影响语义，且关库路径先落 Count 的错误分支）
func (d *DB) DeleteRole(name string) error {
	var count int64
	if err := d.db.Model(&User{}).Where("role = ?", name).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrRoleInUse
	}
	existing, err := d.GetRole(name)
	if err != nil {
		return err
	}
	if existing.Builtin {
		return ErrBuiltinRole
	}
	return d.db.Delete(&Role{}, "name = ?", name).Error
}

// SeedRoles 幂等 seed 内置角色（Open 时自动执行）：不存在的创建；已存在的
// builtin 角色按代码定义覆盖 permissions（内置角色用户不可改——D12，版本
// 升级调整内置清单必须能生效；撞名的自定义角色同样收编为 builtin）。
// 随后执行 D9 存量迁移：role=user → viewer（现状 user 即无写权限，语义保持）。
func (d *DB) SeedRoles() error {
	for _, b := range builtinRoles {
		role := &Role{Name: b.name, Permissions: b.permissions, Builtin: true}
		var existing Role
		err := d.db.First(&existing, "name = ?", b.name).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := d.db.Create(role).Error; err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			existing.Permissions = b.permissions
			existing.Builtin = true
			if err := d.db.Save(&existing).Error; err != nil {
				return err
			}
		}
	}
	return d.db.Model(&User{}).Where("role = ?", "user").Update("role", "viewer").Error
}

// validateCustomRole 新建校验：名字非空、不撞内置角色名（防冒名顶替）、权限点合法
func validateCustomRole(role *Role) error {
	if role.Name == "" {
		return ErrEmptyRoleName
	}
	for _, b := range builtinRoles {
		if role.Name == b.name {
			return ErrBuiltinRole
		}
	}
	return validatePermissions(role.Permissions)
}

// validatePermissions 权限点白名单校验（D12：自定义角色权限点必须落在清单内）
func validatePermissions(perms []string) error {
	for _, p := range perms {
		if !PermissionValid(p) {
			return ErrInvalidPermission
		}
	}
	return nil
}
