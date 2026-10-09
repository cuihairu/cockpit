//go:build darwin

package darwin

import (
	"os"
	"syscall"
)

// GracefulSignals 优雅退出信号集：launchd Stop 发 SIGTERM（KeepAlive
// 语义下非 0 退出会被拉起，主动停必须走 0 退出码），前台 Ctrl+C 发
// SIGINT。
func GracefulSignals() []os.Signal {
	return []os.Signal{syscall.SIGTERM, os.Interrupt}
}
