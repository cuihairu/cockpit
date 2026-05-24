package server

import (
	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"net/http"
	"strings"
)

// responseWriter 响应写入器
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (r *responseWriter) WriteHeader(code int) {
	if !r.written {
		r.statusCode = code
		r.written = true
		r.ResponseWriter.WriteHeader(code)
	}
}

func (r *responseWriter) Write(b []byte) (int, error) {
	if !r.written {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// AuditMiddleware 审计日志中间件
func (s *Server) AuditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 保存原始响应写入器
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// 获取用户信息
		username := "anonymous"
		userID := uint(0)
		if userInfo, ok := auth.GetUserFromContext(r); ok {
			username = userInfo.Username
			// UserID 是 string 类型
		}

		// 处理请求
		next.ServeHTTP(wrapped, r)

		// 记录审计日志（只记录需要审计的路径）
		if s.shouldAudit(r.Method, r.URL.Path) {
			action := s.getActionFromMethod(r.Method)
			resource := s.getResourceFromPath(r.URL.Path)

			details := map[string]interface{}{
				"method":      r.Method,
				"path":        r.URL.Path,
				"query":       r.URL.RawQuery,
				"status_code": wrapped.statusCode,
			}

			s.audit.Log(&audit.LogEntry{
				UserID:     userID,
				Username:   username,
				Action:     action,
				Resource:   resource,
				ResourceID: s.getResourceIDFromPath(r.URL.Path),
				Details:    details,
				IP:         s.getClientIP(r),
				UserAgent:  r.UserAgent(),
				Status:     s.getStatusFromStatusCode(wrapped.statusCode),
			})
		}
	})
}

// shouldAudit 判断是否需要审计
func (s *Server) shouldAudit(method, path string) bool {
	// 不记录健康检查、静态资源等
	if path == "/health" || path == "/api/status" {
		return false
	}

	// 不记录 GET 请求（查询操作）
	if method == "GET" && !strings.HasPrefix(path, "/api/admin") {
		return false
	}

	// 记录所有 API 请求
	return strings.HasPrefix(path, "/api/")
}

// getActionFromMethod 从 HTTP 方法获取操作类型
func (s *Server) getActionFromMethod(method string) string {
	switch method {
	case "GET":
		return audit.ActionView
	case "POST":
		return audit.ActionCreate
	case "PUT", "PATCH":
		return audit.ActionUpdate
	case "DELETE":
		return audit.ActionDelete
	default:
		return "unknown"
	}
}

// getResourceFromPath 从路径获取资源类型
func (s *Server) getResourceFromPath(path string) string {
	if !strings.HasPrefix(path, "/api/") {
		return "unknown"
	}

	// 移除 /api/ 前缀
	parts := strings.Split(strings.TrimPrefix(path, "/api/"), "/")
	if len(parts) > 0 {
		return parts[0]
	}

	return "unknown"
}

// getResourceIDFromPath 从路径获取资源ID
func (s *Server) getResourceIDFromPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// getClientIP 获取客户端IP
func (s *Server) getClientIP(r *http.Request) string {
	// 检查 X-Forwarded-For 头
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// 取第一个IP
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return xff
	}

	// 检查 X-Real-IP 头
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// 使用 RemoteAddr
	return r.RemoteAddr
}

// getStatusFromStatusCode 从状态码获取状态
func (s *Server) getStatusFromStatusCode(statusCode int) string {
	if statusCode >= 200 && statusCode < 300 {
		return audit.StatusSuccess
	}
	return audit.StatusFailure
}
