package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// spaHandler 返回静态文件 handler，未配置 StaticDir 时返回 API 提示
func (s *Server) spaHandler() http.Handler {
	staticDir := s.staticDir()
	if staticDir == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"message": "Cockpit API server is running. Set STATIC_DIR to serve the web UI.",
			})
		})
	}

	fileServer := http.FileServer(http.Dir(staticDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// 跳过 API 和 WebSocket 路径
		if strings.HasPrefix(path, "/api/") || path == "/ws" {
			http.NotFound(w, r)
			return
		}

		// 清理路径，防止目录遍历
		cleanPath := filepath.Clean(strings.TrimPrefix(path, "/"))
		if cleanPath == "." || cleanPath == ".." {
			cleanPath = "index.html"
		}

		fsPath := filepath.Join(staticDir, cleanPath)
		if info, err := os.Stat(fsPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: 文件不存在则返回 index.html
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// staticDir 获取静态文件目录，优先环境变量，其次配置文件
func (s *Server) staticDir() string {
	if dir := os.Getenv("STATIC_DIR"); dir != "" {
		return dir
	}
	if s.cfg != nil && s.cfg.Server != nil && s.cfg.Server.StaticDir != "" {
		return s.cfg.Server.StaticDir
	}
	return ""
}
