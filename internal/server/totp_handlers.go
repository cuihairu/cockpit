package server

import (
	"encoding/json"
	"net/http"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/storage"
)

// TOTPGenerateResponse TOTP 生成响应
type TOTPGenerateResponse struct {
	Secret      string   `json:"secret"`
	QRCode      string   `json:"qr_code"`
	BackupCodes []string `json:"backup_codes"`
}

// TOTPVerifyRequest TOTP 验证请求
type TOTPVerifyRequest struct {
	Code     string `json:"code"`
	TmpToken string `json:"tmp_token,omitempty"`
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

// totpTmpData TOTP 临时存储数据
type totpTmpData struct {
	Secret      string
	BackupCodes []string
}

// TOTP 临时存储 (生产环境应使用 Redis)
var totpTmpStore = make(map[string]*totpTmpData)

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

	// 生成 TOTP 密钥
	secret, err := auth.GenerateTOTPSecret(user.Username, "Cockpit")
	if err != nil {
		http.Error(w, `{"error":"Failed to generate TOTP"}`, http.StatusInternalServerError)
		return
	}

	// 生成备份码
	backupCodes, err := storage.GenerateBackupCodes()
	if err != nil {
		http.Error(w, `{"error":"Failed to generate backup codes"}`, http.StatusInternalServerError)
		return
	}

	// 加密密钥
	encryptedSecret, err := storage.Encrypt(secret)
	if err != nil {
		http.Error(w, `{"error":"Failed to encrypt secret"}`, http.StatusInternalServerError)
		return
	}

	// 哈希备份码
	hashedBackupCodes, err := storage.HashBackupCodes(backupCodes)
	if err != nil {
		http.Error(w, `{"error":"Failed to hash backup codes"}`, http.StatusInternalServerError)
		return
	}

	// 临时保存（启用时才正式写入）
	tmpStorageKey := "totp_tmp_" + userID
	totpTmpStore[tmpStorageKey] = &totpTmpData{
		Secret:      encryptedSecret,
		BackupCodes: hashedBackupCodes,
	}

	// 生成 QR 码 URL
	qrCodeURL, err := auth.GenerateTOTPURL(secret, user.Username, "Cockpit")
	if err != nil {
		http.Error(w, `{"error":"Failed to generate QR code"}`, http.StatusInternalServerError)
		return
	}

	response := TOTPGenerateResponse{
		Secret:      secret,
		QRCode:      qrCodeURL,
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
	tmpData, exists := totpTmpStore[tmpStorageKey]
	if !exists {
		http.Error(w, `{"error":"TOTP setup not initiated. Please generate TOTP first."}`, http.StatusBadRequest)
		return
	}

	// 解密密钥用于验证
	secret, err := storage.Decrypt(tmpData.Secret)
	if err != nil {
		delete(totpTmpStore, tmpStorageKey)
		http.Error(w, `{"error":"Invalid TOTP data"}`, http.StatusInternalServerError)
		return
	}

	// 验证 TOTP 码
	if !auth.ValidateTOTP(secret, req.Code) {
		http.Error(w, `{"error":"Invalid TOTP code"}`, http.StatusBadRequest)
		return
	}

	// 启用 TOTP
	err = s.db.EnableTOTP(userID, tmpData.Secret, tmpData.BackupCodes)
	if err != nil {
		http.Error(w, `{"error":"Failed to enable TOTP"}`, http.StatusInternalServerError)
		return
	}

	// 清除临时数据
	delete(totpTmpStore, tmpStorageKey)

	// 记录审计日志
	user, _ := s.db.GetUserByID(userID)
	s.audit.LogSuccess(user.Username, audit.ActionTOTPEnable, "totp", userID, nil, s.getClientIP(r), r.UserAgent())

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
		s.audit.LogFailure(user.Username, audit.ActionTOTPVerify, "totp", userID, nil, s.getClientIP(r), r.UserAgent())
		http.Error(w, `{"error":"Invalid TOTP code"}`, http.StatusUnauthorized)
		return
	}

	// 消耗临时令牌
	auth.ConsumeTmpToken(req.TmpToken)

	// 生成认证令牌
	token, err := auth.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		http.Error(w, `{"error":"Failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	// 记录成功的审计日志
	s.audit.LogSuccess(user.Username, audit.ActionTOTPVerify, "totp", userID, map[string]bool{"used_backup": isBackup}, s.getClientIP(r), r.UserAgent())

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
	s.audit.LogSuccess(user.Username, audit.ActionTOTPDisable, "totp", userID, nil, s.getClientIP(r), r.UserAgent())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "TOTP disabled successfully"})
}
