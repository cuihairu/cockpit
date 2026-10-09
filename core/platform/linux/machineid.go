//go:build linux

// Package linux Linux 平台适配（agent-core ②）：机器标识/路径/systemd
// 服务集成。平台分支只存在于本目录。
package linux

import (
	"os"
	"strings"
)

// machineIDPath 覆盖率注入点（同 agent.go goos 范式）：默认生产路径，
// 同包测试注入不存在/空文件路径覆盖错误分支，注入前后 defer 恢复。
var machineIDPath = "/etc/machine-id"

// Host Linux 平台事实实现。
type Host struct{}

// MachineID 读取 /etc/machine-id（systemd 标准，首次启动时生成，持久化
// 不变）。读取失败返回空字符串，调用方回退 hostname+random。
func (Host) MachineID() string {
	b, err := os.ReadFile(machineIDPath)
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(b))
	if id == "" {
		return ""
	}
	return id
}
