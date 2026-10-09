//go:build darwin

package darwin

import "os"

// PathDirs macOS 标准目录三元组：launchd 用户态服务挂 ~/Library 家族
// （Application Support 数据面 / Preferences 配置面 / Logs 日志面）。
// 取 home 失败返回空串字段，调用方自行兜底。
func PathDirs() (configDir, dataDir, logDir string) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", "", ""
	}
	return home + "/Library/Preferences/cockpit-agent",
		home + "/Library/Application Support/cockpit-agent",
		home + "/Library/Logs/cockpit-agent"
}
