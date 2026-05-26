package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token        string `json:"token,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	Role         string `json:"role,omitempty"`
	RequiresTOTP bool   `json:"requires_totp"`
	TmpToken     string `json:"tmp_token,omitempty"`
}

// DB 存储接口（在运行时注入）
var DB *storage.DB

// InitDB 初始化数据库连接
func InitDB(db *storage.DB) {
	DB = db
}

// InitAdmin 初始化管理员用户
func InitAdmin(db *storage.DB, username, password string) error {
	return db.InitAdminUser(username, password)
}

// HandleLogin 处理登录
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// 验证用户名密码
	user, err := DB.VerifyPassword(req.Username, req.Password)
	if err != nil {
		http.Error(w, `{"error":"Invalid username or password"}`, http.StatusUnauthorized)
		return
	}

	// 检查是否启用了 TOTP
	if user.TOTPEnabled {
		tmpToken := generateTmpToken(user.ID)
		response := LoginResponse{
			UserID:       user.ID,
			Username:     user.Username,
			RequiresTOTP: true,
			TmpToken:     tmpToken,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// 生成 token
	token, err := GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		http.Error(w, `{"error":"Failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	// 返回 token
	response := LoginResponse{
		Token:     token,
		ExpiresAt: 0,
		UserID:    user.ID,
		Username:  user.Username,
		Role:      user.Role,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleRefresh 处理 token 刷新
func HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, `{"error":"Authorization header required"}`, http.StatusUnauthorized)
		return
	}

	tokenString := authHeader[7:] // Remove "Bearer " prefix

	newToken, err := RefreshToken(tokenString)
	if err != nil {
		http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": newToken})
}

// TmpTokenData 临时令牌数据
type TmpTokenData struct {
	UserID    string
	ExpiresAt time.Time
}

// 临时令牌存储 (生产环境应使用 Redis)
var tmpTokenStore = make(map[string]*TmpTokenData)
var tmpTokenStoreMutex sync.RWMutex

// generateTmpToken 生成临时令牌
func generateTmpToken(userID string) string {
	// 生成 32 字节随机数
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 如果随机数生成失败，使用时间戳作为后备
		return fmt.Sprintf("tmp_%d_%s", time.Now().UnixNano(), userID)
	}
	token := hex.EncodeToString(b)

	tmpTokenStoreMutex.Lock()
	defer tmpTokenStoreMutex.Unlock()

	tmpTokenStore[token] = &TmpTokenData{
		UserID:    userID,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	return token
}

// ValidateTmpToken 验证临时令牌（导出供外部使用）
func ValidateTmpToken(token string) (string, bool) {
	tmpTokenStoreMutex.RLock()
	defer tmpTokenStoreMutex.RUnlock()

	data, exists := tmpTokenStore[token]
	if !exists {
		return "", false
	}

	// 检查是否过期
	if time.Now().After(data.ExpiresAt) {
		return "", false
	}

	return data.UserID, true
}

// ConsumeTmpToken 消耗临时令牌（验证后删除，导出供外部使用）
func ConsumeTmpToken(token string) bool {
	tmpTokenStoreMutex.Lock()
	defer tmpTokenStoreMutex.Unlock()

	data, exists := tmpTokenStore[token]
	if !exists {
		return false
	}

	// 检查是否过期
	if time.Now().After(data.ExpiresAt) {
		delete(tmpTokenStore, token)
		return false
	}

	delete(tmpTokenStore, token)
	return true
}
