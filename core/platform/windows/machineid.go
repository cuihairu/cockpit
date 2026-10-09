//go:build windows

// Package windows Windows 平台适配（agent-core ②）：机器标识/路径/SCM
// 服务集成（SCM 全链收编见批次表）。平台分支只存在于本目录。
package windows

import (
	"golang.org/x/sys/windows/registry"
)

// Host Windows 平台事实实现。
type Host struct{}

// MachineID 读取 MachineGuid（HKLM\SOFTWARE\Microsoft\Cryptography，安装
// 系统时生成，持久化不变）。读取失败返回空字符串。
func (Host) MachineID() string {
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
