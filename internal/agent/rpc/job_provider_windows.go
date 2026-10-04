//go:build windows

package rpc

import (
	"context"
	"os/exec"
)

// newJobCmd 构造 windows 侧执行命令：cmd /C（无 /bin/sh）。
// Windows 无进程组 SIGKILL 语义，沿用 CommandContext 默认杀直接进程；
// cmd /C 通常直接 exec 目标命令，超时语义由父链保证。
func newJobCmd(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd", "/C", command)
}