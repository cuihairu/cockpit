package auth

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
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

// defaultUser 默认用户（从环境变量读取）
var defaultUser = struct {
	Username string
	Password string
}{
	Username: getEnv("ADMIN_USERNAME", "admin"),
	Password: getEnv("ADMIN_PASSWORD", "admin"),
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Init 初始化认证
func Init() {
	if username := os.Getenv("ADMIN_USERNAME"); username != "" {
		defaultUser.Username = username
	}
	if password := os.Getenv("ADMIN_PASSWORD"); password != "" {
		defaultUser.Password = password
	}
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
	if req.Username != defaultUser.Username || req.Password != defaultUser.Password {
		http.Error(w, `{"error":"Invalid username or password"}`, http.StatusUnauthorized)
		return
	}

	// 生成 token
	token, err := GenerateToken("admin", req.Username)
	if err != nil {
		http.Error(w, `{"error":"Failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	// 返回 token
	response := LoginResponse{
		Token:     token,
		ExpiresAt: 0, // 前端可以根据返回时间计算
		UserID:    "admin",
		Username:  req.Username,
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

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	newToken, err := RefreshToken(tokenString)
	if err != nil {
		http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": newToken})
}
