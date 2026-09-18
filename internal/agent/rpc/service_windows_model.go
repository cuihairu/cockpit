package rpc

import (
	"fmt"
	"regexp"
	"strings"
)

// ============ Windows SCM 服务模型（无 build tag，跨平台可测）============
//
// service-design.md D9：Windows 服务与 systemd 共用 ServiceUnit 观测模型，
// 本文件放 SCM DTO 与映射/校验纯函数（Linux CI 可测）；真采集在
// service_windows_scm.go（//go:build windows），非 Windows 编译为 stub。

// WinService SCM 采集层的一条服务观测（字符串化，映射函数不感知枚举）
type WinService struct {
	Name        string // SCM 服务名（操作一律按它，如 wuauserv）
	DisplayName string // 显示名（→ ServiceUnit.Description）
	Status      string // SCM 状态原词：Running / Stopped / Start Pending / ...
	StartType   string // Automatic / Automatic (Delayed) / Manual / Disabled
}

// windowsServiceActions Windows 后端动词白名单（无 reload，D9.3）
var windowsServiceActions = map[string]bool{
	"start": true, "stop": true, "restart": true,
	"enable": true, "disable": true,
}

// windowsServiceNameRe SCM 注册表键名字符集；`.` `..` 在正则内合法但语义
// 是目录引用，由 validateWindowsServiceUnit 显式拒绝（D9.4）
var windowsServiceNameRe = regexp.MustCompile(`^[A-Za-z0-9_.\- ]{1,256}$`)

// validateWindowsServiceUnit Windows 后端 unit 名与动作校验（与 systemd 侧
// validateServiceUnit 分开：服务名无 .service 后缀，字符集含空格）
func validateWindowsServiceUnit(name, action string) error {
	if !windowsServiceNameRe.MatchString(name) || name == "." || name == ".." {
		return fmt.Errorf("invalid windows service name %q", name)
	}
	if !windowsServiceActions[action] {
		return fmt.Errorf("unsupported action %q (allowed: start stop restart enable disable)", action)
	}
	return nil
}

// windowsStatusToActiveState SCM 状态 → ServiceUnit 运行态映射（D9.3）。
// paused 是 Windows 特有语义，不扭曲为 active，前端按未知值灰色徽标显示
func windowsStatusToActiveState(status string) (activeState, subState string) {
	switch status {
	case "Running":
		return "active", "running"
	case "Stopped":
		return "inactive", "stopped"
	case "Start Pending":
		return "activating", "start_pending"
	case "Continue Pending":
		return "activating", "continue_pending"
	case "Stop Pending":
		return "deactivating", "stop_pending"
	case "Paused":
		return "paused", "paused"
	case "Pause Pending":
		return "paused", "pause_pending"
	default:
		return "unknown", strings.ToLower(strings.ReplaceAll(status, " ", "_"))
	}
}

// startTypeToUnitFileState SCM StartType → 自启态映射（D9.3）：
// 前端只用 enabled/disabled 两态，Manual 与 Disabled 同为「手动」
func startTypeToUnitFileState(startType string) string {
	switch startType {
	case "Automatic", "Automatic (Delayed)":
		return "enabled"
	case "Manual", "Disabled":
		return "disabled"
	default:
		return "unknown"
	}
}

// winServiceToUnit SCM DTO → 统一 ServiceUnit（D9.1 映射表）：
// Windows 服务必然注册在 SCM，LoadState 恒 loaded；无 vendor preset 概念
func winServiceToUnit(w WinService) ServiceUnit {
	active, sub := windowsStatusToActiveState(w.Status)
	return ServiceUnit{
		Name:          w.Name,
		Description:   w.DisplayName,
		LoadState:     "loaded",
		ActiveState:   active,
		SubState:      sub,
		UnitFileState: startTypeToUnitFileState(w.StartType),
	}
}
