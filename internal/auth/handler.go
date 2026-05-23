package auth

import (
	"encoding/json"
	"net/http"

	"github.com/cuihairu/cockpit/internal/storage"
)

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
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

	// 生成 token
	token, err := GenerateToken(user.ID, user.Username)
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
