//go:build unix

package rpc

import (
	"os/exec"
	"syscall"
)

// hookCmd 构建 pre-hook 命令（unix：独立进程组，超时可整组回收，M3 D29）
func hookCmd(command string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// killHookGroup 杀掉 pre-hook 整个进程组（sh + dump 等孙进程）
func killHookGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
