//go:build unix

package rpc

import (
	"context"
	"os/exec"
	"syscall"
)

// newJobCmd 构造 unix 侧执行命令：sh -c，且进程组隔离——
// CommandContext 默认只在超时时杀直接子进程（sh），sh 派生的孙进程
// 会继续运行并把输出管道握到结束，CombinedOutput 因此无法按时返回。
// Setpgid 让 sh 成为进程组首领，Cancel 改为杀整组：sleep 这类孙进程
// 一并被 SIGKILL，管道立刻 EOF，超时语义才是真的超时。
func newJobCmd(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return cmd
}