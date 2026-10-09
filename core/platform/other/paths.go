//go:build !linux && !darwin && !windows

package other

// PathDirs 兜底平台目录三元组：沿 unix 家族惯例（BSD 等裸机部署与
// linux 目录语义一致）。
func PathDirs() (configDir, dataDir, logDir string) {
	return "/etc/cockpit-agent", "/var/lib/cockpit-agent", "/var/log/cockpit-agent"
}
