//go:build windows

package windows

import "os"

// GracefulSignals 优雅退出信号集：仅 os.Interrupt（控制台 Ctrl+C）。
// SCM Stop/Shutdown 不产生信号量，svc handler 收控制码后关 stop 通道走
// RunService 的主动停路径（见 internal/agent StartCmd.RunService）。
func GracefulSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
