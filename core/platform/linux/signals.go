//go:build linux

package linux

import (
	"os"
	"syscall"
)

// GracefulSignals 优雅退出信号集：systemd stop 发 SIGTERM，容器/前台
// Ctrl+C 发 SIGINT——两者都走 startcmd 的主动停路径（退出码 0）。
func GracefulSignals() []os.Signal {
	return []os.Signal{syscall.SIGTERM, os.Interrupt}
}
