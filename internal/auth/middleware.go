package auth

import (
	"context"
	"net/http"
	"strings"
)

// Middleware 认证中间件
func Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 从 Authorization header 获取 token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"Authorization header required"}`, http.StatusUnauthorized)
			return
		}

		// 检查 Bearer 前缀
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"Invalid authorization format"}`, http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// 验证 token
		claims, err := ValidateToken(tokenString)
		if err != nil {
			http.Error(w, `{"error":"Invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		// 将用户信息存入 request context
		ctx := r.Context()
		ctx = contextWithUser(ctx, claims.UserID, claims.Username)
		r = r.WithContext(ctx)

		next(w, r)
	}
}

// OptionalMiddleware 可选认证中间件（不强制要求登录）
func OptionalMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			next(w, r)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			next(w, r)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := ValidateToken(tokenString)
		if err != nil {
			next(w, r)
			return
		}

		ctx := r.Context()
		ctx = contextWithUser(ctx, claims.UserID, claims.Username)
		r = r.WithContext(ctx)

		next(w, r)
	}
}

// 用户上下文 key
type contextKey string

const userKey contextKey = "user"

func contextWithUser(ctx context.Context, userID, username string) context.Context {
	return context.WithValue(ctx, userKey, UserInfo{UserID: userID, Username: username})
}

// GetUserFromContext 从 context 获取用户信息
func GetUserFromContext(r *http.Request) (UserInfo, bool) {
	user, ok := r.Context().Value(userKey).(UserInfo)
	return user, ok
}

// UserInfo 用户信息
type UserInfo struct {
	UserID   string
	Username string
}
