//go:build windows

package agent

import (
	"golang.org/x/sys/windows/registry"
)

// machineID 读取 Windows MachineGuid（HKLM\SOFTWARE\Microsoft\Cryptography，
// 安装系统时生成，持久化不变）。
// 读取失败时返回空字符串，调用方应 fallback 到 hostname + random。
func machineID() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Cryptography`, registry.READ)
	if err != nil {
		return ""
	}
	defer k.Close()

	guid, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return ""
	}
	return guid
}