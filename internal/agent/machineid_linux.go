//go:build linux

package agent

import (
	"os"
	"strings"
)

// machineIDPath 覆盖率注入点（同 agent.go goos 范式）：默认生产路径，
// 同包测试注入不存在/空文件路径覆盖错误分支，注入前后 defer 恢复。
var machineIDPath = "/etc/machine-id"

// machineID 读取 /etc/machine-id（systemd 标准，首次启动时生成，持久化不变）。
// 读取失败时返回空字符串，调用方应 fallback 到 hostname + random。
func machineID() string {
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
