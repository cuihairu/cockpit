package server

import (
	"encoding/json"
	"net/http"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/storage"
)

// handleForgotPassword 处理忘记密码请求
func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req auth.ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Username == "" {
		http.Error(w, `{"error":"Username is required"}`, http.StatusBadRequest)
		return
	}

	// 查找用户
	user, err := s.db.GetUserByUsername(req.Username)
	if err != nil {
		// 为了安全，即使用户不存在也返回成功消息
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "如果该用户存在，重置邮件已发送",
		})
		return
	}

	// 检查用户是否有邮箱
	if user.Email == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "该账户未绑定邮箱，请联系管理员重置密码",
		})
		return
	}

	// 生成重置令牌和验证码
	token, code, err := auth.GenerateResetToken(user.ID, user.Email)
	if err != nil {
		http.Error(w, `{"error":"Failed to generate reset token"}`, http.StatusInternalServerError)
		return
	}

	// 发送邮件（异步，不阻塞响应）
	go func() {
		if err := auth.SendPasswordResetEmail(user.Email, user.Username, code, token); err != nil {
			// 记录错误但不暴露给用户
			printf("Failed to send reset email: %v", err)
		}
	}()

	// 返回脱敏邮箱
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(auth.ForgotPasswordResponse{
		Email:       user.Email,
		MaskedEmail: auth.MaskEmail(user.Email),
		Message:     "重置邮件已发送到您的邮箱",
	})
}

// handleResetPassword 处理密码重置请求
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req auth.ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Token == "" || req.NewPassword == "" {
		http.Error(w, `{"error":"Token and new password are required"}`, http.StatusBadRequest)
		return
	}

	if len(req.NewPassword) < 6 {
		http.Error(w, `{"error":"Password must be at least 6 characters"}`, http.StatusBadRequest)
		return
	}

	// 验证令牌
	data, err := auth.ValidateResetToken(req.Token)
	if err != nil {
		http.Error(w, `{"error":"Invalid or expired reset token"}`, http.StatusUnauthorized)
		return
	}

	// 如果提供了验证码，也需要验证
	if req.Code != "" {
		_, err = auth.ValidateResetCode(req.Token, req.Code)
		if err != nil {
			http.Error(w, `{"error":"Invalid verification code"}`, http.StatusUnauthorized)
			return
		}
	}

	// 哈希新密码
	hashedPassword, err := storage.HashPassword(req.NewPassword)
	if err != nil {
		http.Error(w, `{"error":"Failed to process password"}`, http.StatusInternalServerError)
		return
	}

	// 更新密码
	if err := s.db.UpdatePassword(data.UserID, hashedPassword); err != nil {
		http.Error(w, `{"error":"Failed to update password"}`, http.StatusInternalServerError)
		return
	}

	// 消费令牌（一次性使用）
	auth.ConsumeResetToken(req.Token)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "密码已成功重置，请使用新密码登录",
	})
}

// handleVerifyResetCode 验证重置验证码
func (s *Server) handleVerifyResetCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Token string `json:"token"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// 验证令牌和验证码
	_, err := auth.ValidateResetCode(req.Token, req.Code)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid": false,
			"error": "Invalid or expired code",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid":   true,
		"message": "验证码正确",
	})
}

// printf 打印日志（替代 log.Printf）
func printf(format string, args ...interface{}) {
	// 在实际实现中应该使用日志系统
}
