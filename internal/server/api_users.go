package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/storage"
)

// handleUsers 处理用户列表和创建用户
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleUsersList(w, r)
	case http.MethodPost:
		s.handleUserCreate(w, r)
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleUsersList 获取用户列表
func (s *Server) handleUsersList(w http.ResponseWriter, r *http.Request) {
	users, err := s.db.ListUsers()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list users")
		return
	}

	s.writeJSON(w, http.StatusOK, users)
}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// handleUserCreate 创建用户（RBAC：users:admin）
func (s *Server) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// 验证输入
	if req.Username == "" || req.Password == "" {
		s.handleError(w, r, http.StatusBadRequest, "Username and password are required")
		return
	}

	// 设置默认角色（viewer = D9 迁移后 user 的对应语义；user 角色已不存在）
	if req.Role == "" {
		req.Role = "viewer"
	}
	// 角色必须是角色表已存在的名字，否则新用户会被 fail-closed 全拒。
	// 只拒「确认不存在」；库故障等其他错误放行——后续写库自然报 500
	if _, err := s.db.GetRole(req.Role); errors.Is(err, storage.ErrNotFound) {
		s.handleError(w, r, http.StatusBadRequest, "Unknown role")
		return
	}

	// 检查用户名是否已存在
	_, err := s.db.GetUserByUsername(req.Username)
	if err == nil {
		s.handleError(w, r, http.StatusConflict, "Username already exists")
		return
	}

	// 哈希密码
	hashedPassword, err := storage.HashPassword(req.Password)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// 创建用户
	newUser := &storage.User{
		Username: req.Username,
		Password: hashedPassword,
		Email:    req.Email,
		Role:     req.Role,
	}

	if err := s.db.CreateUser(newUser); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to create user")
		return
	}

	// 清除密码后返回
	newUser.Password = ""
	s.writeJSON(w, http.StatusCreated, newUser)
}

// handleUserActions 处理用户操作
func (s *Server) handleUserActions(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(path, "/") // strings.Split 结果至少含一个元素

	userID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "password" && r.Method == http.MethodPost:
		s.handleUserChangePassword(w, r, userID)
	case r.Method == http.MethodPut:
		s.handleUserUpdate(w, r, userID)
	case r.Method == http.MethodDelete:
		s.handleUserDelete(w, r, userID)
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleUserDelete 删除用户（RBAC：users:admin；D13 自我保护——不能删
// 自己、不能删最后一个有效 admin）
func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request, id string) {
	// 获取当前用户
	user, ok := auth.GetUserFromContext(r)
	if !ok {
		s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// 查询目标用户
	targetUser, err := s.db.GetUserByID(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "User not found")
		return
	}

	// 不能删除自己
	if targetUser.Username == user.Username {
		s.handleError(w, r, http.StatusBadRequest, "Cannot delete yourself")
		return
	}

	// D13：不能删除最后一个有效 admin（角色存在且覆盖 users:admin）。
	// 到达者持 users:admin、自身即有效 admin，正常路径下排除目标后至少
	// 剩自己，此分支不可达，作为判定层变化时的防御纵深保留。count 出错
	// 时同样拒绝——保护检查 fail-safe，宁可拒绝一次管理操作
	if s.roleCoversPerm(targetUser.Role, "users:admin") {
		if n, _ := s.countEffectiveAdmins(targetUser.ID, ""); n == 0 {
			s.handleError(w, r, http.StatusBadRequest, "Cannot delete the last admin")
			return
		}
	}

	if err := s.db.DeleteUser(id); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{"message": "User deleted"})
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// handleUserUpdate 更新用户（RBAC：users:admin——到达者皆持权限，
// 散落的 admin 字符串判定已收敛；角色名合法性在角色表校验防锁号；
// D13：不可改自己角色、不可降级最后一个有效 admin）
func (s *Server) handleUserUpdate(w http.ResponseWriter, r *http.Request, id string) {
	// D13 改自己角色的判定需要当前用户
	user, ok := auth.GetUserFromContext(r)
	if !ok {
		s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// 查询目标用户
	targetUser, err := s.db.GetUserByID(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "User not found")
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// 更新用户
	targetUser.Email = req.Email
	if req.Role != "" {
		if _, err := s.db.GetRole(req.Role); errors.Is(err, storage.ErrNotFound) {
			s.handleError(w, r, http.StatusBadRequest, "Unknown role")
			return
		}
		// D13：不可修改自己的角色
		if user.UserID == targetUser.ID {
			s.handleError(w, r, http.StatusBadRequest, "Cannot change your own role")
			return
		}
		// D13：不可降级最后一个有效 admin。到达者持 users:admin、自身
		// 即有效 admin，正常路径下排除目标后至少剩自己，此分支不可达，
		// 作为判定层变化时的防御纵深保留；count 出错时同样拒绝（fail-safe）
		if req.Role != targetUser.Role &&
			s.roleCoversPerm(targetUser.Role, "users:admin") &&
			!s.roleCoversPerm(req.Role, "users:admin") {
			if n, _ := s.countEffectiveAdmins(targetUser.ID, ""); n == 0 {
				s.handleError(w, r, http.StatusBadRequest, "Cannot demote the last admin")
				return
			}
		}
		targetUser.Role = req.Role
	}

	if err := s.db.UpdateUser(targetUser); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to update user")
		return
	}

	targetUser.Password = ""
	s.writeJSON(w, http.StatusOK, targetUser)
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// handleUserChangePassword 修改用户密码（RBAC：users:admin——到达者皆持
// 权限；免验旧密码的判定同步从 admin 字符串改为 users:admin 权限点）
func (s *Server) handleUserChangePassword(w http.ResponseWriter, r *http.Request, id string) {
	// 查询目标用户
	targetUser, err := s.db.GetUserByID(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "User not found")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// 验证新密码
	if req.NewPassword == "" {
		s.handleError(w, r, http.StatusBadRequest, "New password is required")
		return
	}

	// 无 users:admin 权限（如未来受限角色进入此端点）需要验证旧密码
	if !s.userHasPerm(r, "users:admin") {
		_, err := s.db.VerifyPassword(targetUser.Username, req.OldPassword)
		if err != nil {
			s.handleError(w, r, http.StatusUnauthorized, "Invalid old password")
			return
		}
	}

	// 哈希新密码
	hashedPassword, err := storage.HashPassword(req.NewPassword)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// 更新密码
	if err := s.db.UpdatePassword(id, hashedPassword); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to update password")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Password updated"})
}
