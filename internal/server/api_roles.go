package server

// api_roles.go 角色 CRUD API（rbac-design.md 笔 4）：/api/roles 整体
// 仅 roles:admin（RBAC 权限表登记，含 GET）。角色增删改本身是非 GET
// 请求，审计由 AuditMiddleware 统一记录（D14）；自定义角色的写约束
// （拒内置、权限点白名单、拒被引用删除）在 storage 层（见 role.go），
// 这里只做错误映射与 D13 自我保护。

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/storage"
)

// handleRoles GET 角色列表 / POST 创建自定义角色
func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleRolesList(w, r)
	case http.MethodPost:
		s.handleRoleCreate(w, r)
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleRolesList 角色列表（含内置角色）
func (s *Server) handleRolesList(w http.ResponseWriter, r *http.Request) {
	roles, err := s.db.ListRoles()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list roles")
		return
	}
	s.writeJSON(w, http.StatusOK, roles)
}

// handleRoleCreate 创建自定义角色
func (s *Server) handleRoleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// 撞已有角色（内置或自定义）→ 409；内置名的冒名由 CreateRole 拒
	// （语义不同：内置不可创建同名）。库故障放行——CreateRole 自然报错
	if _, err := s.db.GetRole(req.Name); err == nil {
		s.handleError(w, r, http.StatusConflict, "Role already exists")
		return
	}

	role := &storage.Role{Name: req.Name, Permissions: req.Permissions}
	if err := s.db.CreateRole(role); err != nil {
		switch {
		case errors.Is(err, storage.ErrEmptyRoleName),
			errors.Is(err, storage.ErrBuiltinRole),
			errors.Is(err, storage.ErrInvalidPermission):
			s.handleError(w, r, http.StatusBadRequest, err.Error())
		default:
			s.handleError(w, r, http.StatusInternalServerError, "Failed to create role")
		}
		return
	}

	s.writeJSON(w, http.StatusCreated, role)
}

// handleRoleActions /api/roles/{name} 的 PUT/DELETE
func (s *Server) handleRoleActions(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(path, "/") // strings.Split 结果至少含一个元素
	name := parts[0]

	switch r.Method {
	case http.MethodPut:
		s.handleRoleUpdate(w, r, name)
	case http.MethodDelete:
		s.handleRoleDelete(w, r, name)
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleRoleUpdate 更新自定义角色权限（内置拒绝——D12 定义以代码为准）
func (s *Server) handleRoleUpdate(w http.ResponseWriter, r *http.Request, name string) {
	existing, err := s.db.GetRole(name)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.handleError(w, r, http.StatusNotFound, "Role not found")
			return
		}
		s.handleError(w, r, http.StatusInternalServerError, "Failed to get role")
		return
	}

	var req struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// D13：不可降级最后一个有效 admin。削掉 users:admin 后若再无
	// 挂有效角色的 admin 用户（roles:admin 操作者不必持 users:admin，
	// 此路径真实可达），拒绝——否则系统再无人能管理用户；count 出错
	// 时同样拒绝（fail-safe）
	if roleCovers(existing.Permissions, "users:admin") && !roleCovers(req.Permissions, "users:admin") {
		if n, _ := s.countEffectiveAdmins("", name); n == 0 {
			s.handleError(w, r, http.StatusBadRequest, "Cannot demote the last admin")
			return
		}
	}

	updated := &storage.Role{Name: name, Permissions: req.Permissions}
	if err := s.db.UpdateRole(updated); err != nil {
		switch {
		case errors.Is(err, storage.ErrBuiltinRole),
			errors.Is(err, storage.ErrInvalidPermission):
			s.handleError(w, r, http.StatusBadRequest, err.Error())
		default:
			s.handleError(w, r, http.StatusInternalServerError, "Failed to update role")
		}
		return
	}

	s.writeJSON(w, http.StatusOK, updated)
}

// handleRoleDelete 删除自定义角色（内置拒绝；仍被引用拒绝——删了会让
// 这些用户 fail-closed 全拒，等价锁号）
func (s *Server) handleRoleDelete(w http.ResponseWriter, r *http.Request, name string) {
	if err := s.db.DeleteRole(name); err != nil {
		switch {
		case errors.Is(err, storage.ErrBuiltinRole):
			s.handleError(w, r, http.StatusBadRequest, err.Error())
		case errors.Is(err, storage.ErrRoleInUse):
			s.handleError(w, r, http.StatusConflict, "Role is still assigned to users")
		case errors.Is(err, storage.ErrNotFound):
			s.handleError(w, r, http.StatusNotFound, "Role not found")
		default:
			s.handleError(w, r, http.StatusInternalServerError, "Failed to delete role")
		}
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Role deleted"})
}
