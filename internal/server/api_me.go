package server

import (
	"encoding/json"
	"net/http"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/storage"
)

// handleCurrentUser 获取当前用户信息
func (s *Server) handleCurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	authUser, ok := auth.GetUserFromContext(r)
	if !ok || authUser.UserID == "" {
		s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := s.db.GetUserByID(authUser.UserID)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "User not found")
		return
	}

	// 返回用户信息（不包含密码）。permissions 查角色表展开（P1 笔 5：
	// 前端菜单裁剪与按钮显隐的数据源）；幽灵角色/库故障 → 空清单，
	// 与判定层 fail-closed 语义一致（前端全裁剪，不会多显示出入口）
	permissions := make([]string, 0)
	if role, err := s.db.GetRole(user.Role); err == nil && len(role.Permissions) > 0 {
		permissions = role.Permissions
	}

	response := map[string]interface{}{
		"id":            user.ID,
		"username":      user.Username,
		"email":         user.Email,
		"phone":         user.Phone,
		"department":    user.Department,
		"role":          user.Role,
		"permissions":   permissions,
		"totp_enabled":  user.TOTPEnabled,
		"totp_setup_at": user.TOTPSetupAt,
		"created_at":    user.CreatedAt,
		"updated_at":    user.UpdatedAt,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleCurrentUserPassword 修改当前用户密码
func (s *Server) handleCurrentUserPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userInfo, ok := auth.GetUserFromContext(r)
	if !ok || userInfo.UserID == "" {
		s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := s.db.GetUserByID(userInfo.UserID)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "User not found")
		return
	}

	// 支持两种字段命名（前端使用 currentPassword/newPassword）
	type ChangePasswordRequestAlt struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
		OldPassword     string `json:"old_password,omitempty"`
	}

	var req ChangePasswordRequestAlt
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	oldPassword := req.OldPassword
	if oldPassword == "" {
		oldPassword = req.CurrentPassword
	}

	// 验证新密码
	if req.NewPassword == "" {
		s.handleError(w, r, http.StatusBadRequest, "New password is required")
		return
	}

	if len(req.NewPassword) < 6 {
		s.handleError(w, r, http.StatusBadRequest, "New password must be at least 6 characters")
		return
	}

	// 验证旧密码
	_, err = s.db.VerifyPassword(user.Username, oldPassword)
	if err != nil {
		s.handleError(w, r, http.StatusUnauthorized, "Invalid current password")
		return
	}

	// 哈希新密码
	hashedPassword, err := storage.HashPassword(req.NewPassword)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// 更新密码
	if err := s.db.UpdatePassword(userInfo.UserID, hashedPassword); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to update password")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Password updated successfully"})
}

// handleCurrentUserProfile 更新当前用户信息（email/phone/department）
//
// 对应前端 PUT /api/me/profile。
func (s *Server) handleCurrentUserProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userInfo, ok := auth.GetUserFromContext(r)
	if !ok || userInfo.UserID == "" {
		s.handleError(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := s.db.GetUserByID(userInfo.UserID)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "User not found")
		return
	}

	var req struct {
		Email      *string `json:"email"`
		Phone      *string `json:"phone"`
		Department *string `json:"department"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// 使用指针区分“字段未传”和“显式清空”。
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Phone != nil {
		user.Phone = *req.Phone
	}
	if req.Department != nil {
		user.Department = *req.Department
	}

	if err := s.db.UpdateUser(user); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to update profile")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":    "Profile updated successfully",
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"phone":      user.Phone,
		"department": user.Department,
	})
}

// handleSettings 保存前端 UI 偏好设置
//
// 对应前端 PUT /api/settings。
// 本轮设计：前端 UI 偏好实际持久化在前端 localStorage，
// 后端仅返回 success 以兼容前端调用；预留扩展点供未来服务端持久化。
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// 解析 body 验证格式合法
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Settings saved",
	})
}
