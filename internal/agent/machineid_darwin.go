//go:build darwin

package agent

import (
	"os/exec"
	"regexp"
)

var uuidRe = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`)

// machineID 读取 macOS IOPlatformUUID（硬件级，绑定主板，重装系统不变）。
// 读取失败时返回空字符串，调用方应 fallback 到 hostname + random。
func machineID() string {
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