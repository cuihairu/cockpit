//go:build !windows

package main

import (
	"fmt"
	"io"
)

// handleService 非 Windows 平台不支持服务化注册：systemd/launchd 形态由
// install.sh / 发行版机制承担，这里给出明确指引而非静默失败
func handleService(args []string, stdout io.Writer) int {
	_ = args
	fmt.Fprintln(stdout, "service 命令仅支持 Windows（服务注册/开机自启）。")
	fmt.Fprintln(stdout, "Linux/macOS 请使用 install.sh 或系统服务管理器（systemd/launchd）。")
	return 1
}

// redirectServiceStart 非 Windows 无 SCM 上下文，永不拦截 start
func redirectServiceStart(args []string, stdout io.Writer) bool {
	_ = args
	_ = stdout
	return false
}
