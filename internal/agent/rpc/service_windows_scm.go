package rpc

import (
	"fmt"
	"sort"

	"github.com/cuihairu/cockpit/core/platform"
)

// ============ Windows SCM 服务管理 Provider（service-design.md D9）============
//
// SCM 原生交互经 Host 服务挂约（P7b）：platform.Current().Services() 在
// windows 返回 SCM 实现（svc/mgr 枚举/幂等/等待在 core/platform/windows），
// 其余 GOOS 恒 nil。本文件因此无 build tag，Linux CI 全链可测（原非
// Windows stub 退役——防御语义由 nil 挂约路径承担，报错文案不变）。动作限
// 5 动词白名单（reload 不支持）；映射/校验纯函数见 service_windows_model.go
// （无 build tag Linux CI 可测）。

// serviceWindowsMgr 服务管理面取用（测试注入点；生产恒走 Host 挂约）
var serviceWindowsMgr = func() platform.ServiceManager { return platform.Current().Services() }

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

// listWinServices 挂约实体观测 → rpc 采集 DTO（字符串口径逐字段搬运）
func listWinServices() ([]WinService, error) {
	mgr := serviceWindowsMgr()
	if mgr == nil {
		return nil, fmt.Errorf("windows-scm service backend requires a windows agent build")
	}
	svcs, err := mgr.List()
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

// DoAction 校验后走挂约动作（幂等/等待语义在 core/platform/windows SCM 层）
func (p *WindowsServiceProvider) DoAction(name, action string) (interface{}, error) {
	// 动词白名单（含 reload 拒绝）在 validateWindowsServiceUnit 内，此处不重复
	if err := validateWindowsServiceUnit(name, action); err != nil {
		return nil, err
	}
	mgr := serviceWindowsMgr()
	if mgr == nil {
		return nil, fmt.Errorf("windows-scm service backend requires a windows agent build")
	}
	if err := mgr.Action(name, action, serviceActionTimeout); err != nil {
		return nil, err
	}
	return map[string]interface{}{"name": name, "action": action}, nil
}
