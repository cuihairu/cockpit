//go:build linux

package linux

// PathDirs Linux 标准目录三元组：沿 install.sh 既有家族（unit
// ReadWritePaths 已含 /var/lib/cockpit-agent 数据面，env 面在
// /etc/default/cockpit-agent）。openwrt 走 linux 包，/var 为易失区——
// 数据目录随 ipk/procd 约定届时再议（契约只声明，消费方落盘前自建）。
func PathDirs() (configDir, dataDir, logDir string) {
	return "/etc/cockpit-agent", "/var/lib/cockpit-agent", "/var/log/cockpit-agent"
}
