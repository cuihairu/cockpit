//go:build !unix

package rpc

import (
	"os/exec"
)

// hookCmd 构建 pre-hook 命令（非 unix 无进程组语义，直通；
// backup capability 仅 linux 注册，此文件只为全平台编译通过）
func hookCmd(command string) *exec.Cmd {
	return exec.Command("sh", "-c", command)
}

func killHookGroup(cmd *exec.Cmd) {
	// 无进程组：仅能终止 sh 自身，孙进程成为孤儿（能力未在非 unix 注册，不可达路径）
	_ = cmd
}
