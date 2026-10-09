//go:build darwin

// Package darwin macOS 平台适配（agent-core ②）：机器标识/路径/launchd
// 服务集成。平台分支只存在于本目录。
package darwin

import (
	"os/exec"
	"regexp"
)

var uuidRe = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`)

// Host macOS 平台事实实现。
type Host struct{}

// MachineID 读取 IOPlatformUUID（硬件级，绑定主板，重装系统不变）。
// 读取失败返回空字符串。
func (Host) MachineID() string {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return ""
	}
	m := uuidRe.FindSubmatch(out)
	if len(m) < 2 {
		return ""
	}
	return string(m[1])
}
