//go:build windows

package rpc

import (
	"fmt"
	"sort"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// ============ Windows SCM 服务管理 Provider（service-design.md D9）============
//
// golang.org/x/sys/windows/svc/mgr 直连服务控制管理器（本地 RPC），观测映射
// 到统一 ServiceUnit 模型（service_windows_model.go），动作限 5 动词白名单
// （reload 不支持）。权限边界由 Windows ACL 决定，cockpit 不提权。

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

// svcStateName SCM 状态枚举 → 原词字符串（WinService.Status 口径）
var svcStateName = map[svc.State]string{
	svc.Stopped:         "Stopped",
	svc.StartPending:    "Start Pending",
	svc.StopPending:     "Stop Pending",
	svc.Running:         "Running",
	svc.ContinuePending: "Continue Pending",
	svc.PausePending:    "Pause Pending",
	svc.Paused:          "Paused",
}

// svcStartTypeName StartType + DelayedAutoStart → 字符串（WinService.StartType 口径）
func svcStartTypeName(cfg mgr.Config) string {
	switch cfg.StartType {
	case mgr.StartAutomatic:
		if cfg.DelayedAutoStart {
			return "Automatic (Delayed)"
		}
		return "Automatic"
	case mgr.StartManual:
		return "Manual"
	case mgr.StartDisabled:
		return "Disabled"
	default:
		return "Unknown"
	}
}

// List 全量枚举服务：逐个开句柄取 Query + Config（数百服务本地 RPC 毫秒级）。
// 单个服务读取失败降级跳过（如驱动服务权限受限），全失败才报错
func (p *WindowsServiceProvider) List() (interface{}, error) {
	units, err := p.listWinServices()
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

func (p *WindowsServiceProvider) listWinServices() ([]WinService, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("scm connect: %w", err)
	}
	defer m.Disconnect()

	names, err := m.ListServices()
	if err != nil {
		return nil, fmt.Errorf("scm list: %w", err)
	}
	units := make([]WinService, 0, len(names))
	readErr := 0
	for _, name := range names {
		s, err := m.OpenService(name)
		if err != nil {
			readErr++
			continue
		}
		st, err := s.Query()
		if err != nil {
			s.Close()
			readErr++
			continue
		}
		cfg, err := s.Config()
		s.Close()
		if err != nil {
			readErr++
			continue
		}
		units = append(units, WinService{
			Name:        name,
			DisplayName: cfg.DisplayName,
			Status:      svcStateName[st.State],
			StartType:   svcStartTypeName(cfg),
		})
	}
	if readErr == len(names) {
		return nil, fmt.Errorf("scm: all %d services unreadable", readErr)
	}
	return units, nil
}

// Status 概览：SCM 无 systemd 全局状态概念，systemState 恒 unknown（D9.6，
// 前端对 windows-scm 主机隐藏该项），failed 恒 0 与 systemd 侧字段对齐
func (p *WindowsServiceProvider) Status() (interface{}, error) {
	units, err := p.listWinServices()
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

// DoAction 执行 SCM 动作（白名单校验后走 mgr API）。SCM 启停是异步请求，
// start/stop 即发即返，状态收敛靠前端 30s 轮询；restart 等待停止完成再拉起。
// start/stop 先查询做幂等（SCM 对已 Running/Stopped 会报错，systemd 则幂等）
func (p *WindowsServiceProvider) DoAction(name, action string) (interface{}, error) {
	if err := validateWindowsServiceUnit(name, action); err != nil {
		return nil, err
	}
	if action == "reload" {
		return nil, fmt.Errorf("reload is not supported on windows-scm backend")
	}

	m, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("scm connect: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return nil, fmt.Errorf("open service %s: %w", name, err)
	}
	defer s.Close()

	switch action {
	case "start":
		if scmRunning(s) {
			return map[string]interface{}{"name": name, "action": action}, nil
		}
		if err := s.Start(); err != nil {
			return nil, fmt.Errorf("start %s: %w", name, err)
		}
	case "stop":
		if scmStopped(s) {
			return map[string]interface{}{"name": name, "action": action}, nil
		}
		if _, err := s.Control(svc.Stop); err != nil {
			return nil, fmt.Errorf("stop %s: %w", name, err)
		}
	case "restart":
		if err := scmStopAndWait(s, name, serviceActionTimeout); err != nil {
			return nil, err
		}
		if err := s.Start(); err != nil {
			return nil, fmt.Errorf("start %s: %w", name, err)
		}
	case "enable":
		if err := scmSetStartType(s, mgr.StartAutomatic); err != nil {
			return nil, fmt.Errorf("enable %s: %w", name, err)
		}
	case "disable":
		if err := scmSetStartType(s, mgr.StartDisabled); err != nil {
			return nil, fmt.Errorf("disable %s: %w", name, err)
		}
	}
	return map[string]interface{}{"name": name, "action": action}, nil
}

func scmRunning(s *mgr.Service) bool {
	st, err := s.Query()
	return err == nil && st.State == svc.Running
}

func scmStopped(s *mgr.Service) bool {
	st, err := s.Query()
	return err == nil && st.State == svc.Stopped
}

// scmStopAndWait 发起停止并轮询至 Stopped（SCM 无原生 restart）
func scmStopAndWait(s *mgr.Service, name string, timeout time.Duration) error {
	if scmStopped(s) {
		return nil
	}
	if _, err := s.Control(svc.Stop); err != nil {
		return fmt.Errorf("stop %s: %w", name, err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, err := s.Query()
		if err != nil {
			return fmt.Errorf("query %s: %w", name, err)
		}
		if st.State == svc.Stopped {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("service %s did not stop within %s", name, timeout)
}

// scmSetStartType 修改自启类型：UpdateConfig 覆盖全字段，先取现有配置
// 再只改 StartType（其余字段原值回写，mgr 对空值参数不改动）
func scmSetStartType(s *mgr.Service, startType uint32) error {
	cfg, err := s.Config()
	if err != nil {
		return err
	}
	cfg.StartType = startType
	return s.UpdateConfig(cfg)
}
