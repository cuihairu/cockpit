package embed

import (
	"embed"
	"io/fs"
)

//go:embed web/dist
var webFS embed.FS

// WebFS 返回 Web 文件系统
func WebFS() (fs.FS, error) {
	return fs.Sub(webFS, "web/dist")
}
