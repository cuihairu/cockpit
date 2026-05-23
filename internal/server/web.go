package server

import (
	"net/http"
	"path"

	"github.com/cuihairu/cockpit/embed"
)

// spaHandler 处理 SPA 路由
func (s *Server) spaHandler() http.Handler {
	sub, _ := embed.WebFS()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 检查文件是否存在
		f, err := sub.Open(path.Join("", r.URL.Path))
		if err != nil || f == nil {
			// 文件不存在，返回 index.html（SPA 路由）
			r.URL.Path = "/"
		} else {
			f.Close()
		}

		http.FileServer(http.FS(sub)).ServeHTTP(w, r)
	})
}
