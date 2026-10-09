//go:build windows

package rpc

import (
	"fmt"
	"sort"

	"github.com/cuihairu/cockpit/core/platform/windows"
)

// ============ Windows SCM 服务管理 Provider（service-design.md D9）============
//
// SCM 原生交互在 core/platform/windows（svc/mgr 直连、幂等、等待），本文件
// 只做动作分发与 ServiceUnit 观测映射（映射/校验纯函数见
// service_windows_model.go，无 build tag Linux CI 可测）。动作限 5 动词
// 白名单（reload 不支持）。

// WindowsServiceProvider Windows SCM 服务管理 Provider
type WindowsServiceProvider struct{}

func NewWindowsServiceProvider() *WindowsServiceProvider { return &WindowsServiceProvider{} }

func (p *WindowsServiceProvider) Type() string { return "service" }

func (p *WindowsServiceProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "list":
		return p.List()
	case "status":
		return p.Status()
	case "action":
		return p.DoAction(paramString(params, "name"), paramString(params, "action"))
	default:
		return nil, fmt.Errorf("unknown service action: %s", action)
	}
}

// listWinServices SCM 交互层观测 → rpc 采集 DTO（字符串口径逐字段搬运）
func listWinServices() ([]WinService, error) {
	svcs, err := windows.List()
	if err != nil {
		return nil, err
	}
	units := make([]WinService, 0, len(svcs))
	for _, s := range svcs {
		units = append(units, WinService{
			Name:        s.Name,
			DisplayName: s.DisplayName,
			Status:      s.Status,
			StartType:   s.StartType,
		})
	}
	return units, nil
}

// List 全量枚举服务并映射到统一 ServiceUnit
func (p *WindowsServiceProvider) List() (interface{}, error) {
	units, err := listWinServices()
	if err != nil {
		return nil, err
	}
	out := make([]ServiceUnit, 0, len(units))
	for _, w := range units {
		out = append(out, winServiceToUnit(w))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return map[string]interface{}{"services": out}, nil
}

// Status 概览：SCM 无 systemd 全局状态概念，systemState 恒 unknown（D9.6，
// 前端对 windows-scm 主机隐藏该项），failed 恒 0 与 systemd 侧字段对齐
func (p *WindowsServiceProvider) Status() (interface{}, error) {
	units, err := listWinServices()
	if err != nil {
		return nil, err
	}
	enabled, active := 0, 0
	for _, w := range units {
		if startTypeToUnitFileState(w.StartType) == "enabled" {
			enabled++
		}
		if state, _ := windowsStatusToActiveState(w.Status); state == "active" || state == "paused" {
			active++
		}
	}
	return map[string]interface{}{
		"systemState": "unknown",
		"total":       len(units),
		"active":      active,
		"failed":      0,
		"enabled":     enabled,
	}, nil
}

// DoAction 校验后走 SCM 动作（幂等/等待语义在 core/platform/windows）
func (p *WindowsServiceProvider) DoAction(name, action string) (interface{}, error) {
	if err := validateWindowsServiceUnit(name, action); err != nil {
		return nil, err
	}
	if action == "reload" {
		return nil, fmt.Errorf("reload is not supported on windows-scm backend")
	}
	if err := windows.Action(name, action, serviceActionTimeout); err != nil {
		return nil, err
	}
	return map[string]interface{}{"name": name, "action": action}, nil
}
