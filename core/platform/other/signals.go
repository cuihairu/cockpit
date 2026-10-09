//go:build !linux && !darwin && !windows

package other

import (
	"os"
	"syscall"
)

// GracefulSignals 兜底平台优雅退出信号集：unix 家族惯例（SIGTERM +
// SIGINT）。
func GracefulSignals() []os.Signal {
	return []os.Signal{syscall.SIGTERM, os.Interrupt}
}
