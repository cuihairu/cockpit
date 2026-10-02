//go:build linux

package agent

import (
	"os"
	"strings"
)

// machineID 读取 /etc/machine-id（systemd 标准，首次启动时生成，持久化不变）。
// 读取失败时返回空字符串，调用方应 fallback 到 hostname + random。
func machineID() string {
	b, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(b))
	if id == "" {
		return ""
	}
	return id
}