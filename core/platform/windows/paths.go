//go:build windows

package windows

import (
	"os"
	"path/filepath"
)

// PathDirs Windows 标准目录三元组：服务形态挂 ProgramData（机器级、
// 服务账户可写），config/data/logs 三分目录。
func PathDirs() (configDir, dataDir, logDir string) {
	base := filepath.Join(os.Getenv("ProgramData"), "cockpit-agent")
	return filepath.Join(base, "config"), filepath.Join(base, "data"), filepath.Join(base, "logs")
}
