package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/storage"
)

// TOTPGenerateResponse TOTP 生成响应
type TOTPGenerateResponse struct {
	Secret    string   `json:"secret"`
	QRCode    string   `json:"qr_code"`
	BackupCodes []string `json:"backup_codes"`
}

// TOTPVerifyRequest TOTP 验证请求
type TOTPVerifyRequest struct {
	Code      string `json:"code"`
	TmpToken  string `json:"tmp_token,omitempty"`
}

// TOTPVerifyResponse TOTP 验证响应
type TOTPVerifyResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
}

// TOTPEnableRequest TOTP 启用请求
type TOTPEnableRequest struct {
	Code string `json:"code"`
}

// TOTPDisableRequest TOTP 禁用请求
type TOTPDisableRequest struct {
	Code string `json:"code"`
}

// handleTOTPGenerate 处理 TOTP 生成请求
func (s *Server) handleTOTPGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// 从 JWT 中获取用户信息
	userID := r.Context().Value("user_id").(string)
	if userID == "" {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// 获取用户信息
	user, err := s.db.GetUserByID(userID)
	if err != nil {
		http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
		return
	}

	// 生成 TOTP 密钥和备份码
	secret, backupCodes, err := storage.GenerateTOTP()
	if err != nil {
		http.Error(w, `{"error":"Failed to generate TOTP"}`, http.StatusInternalServerError)
		return
	}

	// 加密存储密钥（临时保存，启用时才正式写入）
	encryptedSecret, err := storage.EncryptSecret(secret)
	if err != nil {
		http.Error(w, `{"error":"Failed to encrypt secret"}`, http.StatusInternalServerError)
		return
	}

	// 临时保存到上下文（这里使用简化方案，生产环境应使用 Redis）
	// 实际应用中可以将这个临时密钥存到带过期时间的存储中
	tmpStorageKey := "totp_tmp_" + userID
	s.totpTmpStore[tmpStorageKey] = &totpTmpData{
		Secret:       encryptedSecret,
		BackupCodes:  backupCodes,
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	// 生成 QR 码 URL（使用 Google Authenticator 格式）
	qrCodeURL := storage.GenerateQRCodeURL(user.Username, secret)

	response := TOTPGenerateResponse{
		Secret:     secret,
		QRCode:     qrCodeURL,
		BackupCodes: backupCodes,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTOTPEnable 处理 TOTP 启用请求
func (s *Server) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// 从 JWT 中获取用户信息
	userID := r.Context().Value("user_id").(string)
	if userID == "" {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req TOTPEnableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// 获取临时存储的 TOTP 数据
	tmpStorageKey := "totp_tmp_" + userID
	tmpData, exists := s.totpTmpStore[tmpStorageKey]
	if !exists {
		http.Error(w, `{"error":"TOTP setup not initiated. Please generate TOTP first."}`, http.StatusBadRequest)
		return
	}

	// 检查是否过期
	if time.Now().After(tmpData.ExpiresAt) {
		delete(s.totpTmpStore, tmpStorageKey)
		http.Error(w, `{"error":"TOTP setup expired. Please generate TOTP again."}`, http.StatusBadRequest)
		return
	}

	// 验证 TOTP 码
	valid, err := storage.ValidateTOTP(tmpData.Secret, req.Code)
	if err != nil || !valid {
		http.Error(w, `{"error":"Invalid TOTP code"}`, http.StatusBadRequest)
		return
	}

	// 启用 TOTP
	now := time.Now()
	err = s.db.EnableTOTP(userID, tmpData.Secret, tmpData.BackupCodes, &now)
	if err != nil {
		http.Error(w, `{"error":"Failed to enable TOTP"}`, http.StatusInternalServerError)
		return
	}

	// 清除临时数据
	delete(s.totpTmpStore, tmpStorageKey)

	// 记录审计日志
	s.audit.LogTOTPEnabled(userID, s.getClientIP(r), r.UserAgent())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "TOTP enabled successfully"})
}

// handleTOTPVerify 处理 TOTP 验证请求（登录时的二次验证）
func (s *Server) handleTOTPVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req TOTPVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// 验证临时令牌
	userID, valid := auth.ValidateTmpToken(req.TmpToken)
	if !valid {
		http.Error(w, `{"error":"Invalid or expired temporary token"}`, http.StatusUnauthorized)
		return
	}

	// 获取用户信息
	user, err := s.db.GetUserByID(userID)
	if err != nil {
		http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
		return
	}

	// 检查是否启用了 TOTP
	if !user.TOTPEnabled {
		http.Error(w, `{"error":"TOTP not enabled for this user"}`, http.StatusBadRequest)
		return
	}

	// 验证 TOTP 码或备份码
	isValid, isBackup, err := s.db.ValidateTOTPCode(userID, req.Code)
	if err != nil || !isValid {
		// 记录失败的审计日志
		s.audit.LogTOTPFailed(userID, s.getClientIP(r), r.UserAgent())
		http.Error(w, `{"error":"Invalid TOTP code"}`, http.StatusUnauthorized)
		return
	}

	// 消耗临时令牌
	auth.ConsumeTmpToken(req.TmpToken)

	// 如果是备份码，标记为已使用
	if isBackup {
		s.db.MarkBackupCodeUsed(userID, req.Code)
	}

	// 生成认证令牌
	token, err := auth.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		http.Error(w, `{"error":"Failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	// 记录成功的审计日志
	s.audit.LogTOTPVerified(userID, s.getClientIP(r), r.UserAgent(), isBackup)

	response := TOTPVerifyResponse{
		Token:     token,
		ExpiresAt: 0,
		UserID:    user.ID,
		Username:  user.Username,
		Role:      user.Role,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTOTPDisable 处理 TOTP 禁用请求
func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// 从 JWT 中获取用户信息
	userID := r.Context().Value("user_id").(string)
	if userID == "" {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req TOTPDisableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// 获取用户信息
	user, err := s.db.GetUserByID(userID)
	if err != nil {
		http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
		return
	}

	// 检查是否启用了 TOTP
	if !user.TOTPEnabled {
		http.Error(w, `{"error":"TOTP not enabled for this user"}`, http.StatusBadRequest)
		return
	}

	// 验证 TOTP 码
	isValid, _, err := s.db.ValidateTOTPCode(userID, req.Code)
	if err != nil || !isValid {
		http.Error(w, `{"error":"Invalid TOTP code"}`, http.StatusBadRequest)
		return
	}

	// 禁用 TOTP
	err = s.db.DisableTOTP(userID)
	if err != nil {
		http.Error(w, `{"error":"Failed to disable TOTP"}`, http.StatusInternalServerError)
		return
	}

	// 记录审计日志
	s.audit.LogTOTPDisabled(userID, s.getClientIP(r), r.UserAgent())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "TOTP disabled successfully"})
}

// validateTmpToken 验证临时令牌（从 auth 包导出的函数）
// 这个函数已在 auth/handler.go 中实现，这里只需要调用它

// totpTmpData TOTP 临时存储数据
type totpTmpData struct {
	Secret      string
	BackupCodes string
	ExpiresAt   time.Time
}

// totpTmpStore TOTP 临时存储
var totpTmpStore = make(map[string]*totpTmpData)
