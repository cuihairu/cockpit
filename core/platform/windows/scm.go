//go:build windows

// Package windows Windows 平台适配（agent-core ②）：机器标识/SCM 服务交互。
// 平台分支只存在于本目录。
package windows

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// ============ Windows SCM 交互层（service-design.md D9，P4 平台收编）============
//
// golang.org/x/sys/windows/svc/mgr 直连服务控制管理器（本地 RPC）。本文件
// 只做 SCM 原生交互（枚举/动词/幂等/等待），DTO 全字符串化；ServiceUnit
// 映射与 unit 名/动词校验留在 rpc 层（无 build tag，Linux CI 可测）。权限
// 边界由 Windows ACL 决定，cockpit 不提权。

// Service SCM 采集层的一条服务观测（字符串化，映射函数不感知枚举）
type Service struct {
	Name        string // SCM 服务名（操作一律按它，如 wuauserv）
	DisplayName string // 显示名
	Status      string // SCM 状态原词：Running / Stopped / Start Pending / ...
	StartType   string // Automatic / Automatic (Delayed) / Manual / Disabled
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

// svcStartTypeName StartType + DelayedAutoStart → 字符串（Service.StartType 口径）
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
func List() ([]Service, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("scm connect: %w", err)
	}
	defer m.Disconnect()

	names, err := m.ListServices()
	if err != nil {
		return nil, fmt.Errorf("scm list: %w", err)
	}
	units := make([]Service, 0, len(names))
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
		units = append(units, Service{
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

// Action 执行 SCM 动作（unit 名/动词校验由调用方先行）。SCM 启停是异步请求，
// start/stop 即发即返，状态收敛靠前端 30s 轮询；restart 等待停止完成再拉起。
// start/stop 先查询做幂等（SCM 对已 Running/Stopped 会报错，systemd 则幂等）。
// reload 不支持由调用方拒绝（白名单无此动词）
func Action(name, action string, timeout time.Duration) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("scm connect: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("open service %s: %w", name, err)
	}
	defer s.Close()

	switch action {
	case "start":
		if scmRunning(s) {
			return nil
		}
		if err := s.Start(); err != nil {
			return fmt.Errorf("start %s: %w", name, err)
		}
	case "stop":
		if scmStopped(s) {
			return nil
		}
		if _, err := s.Control(svc.Stop); err != nil {
			return fmt.Errorf("stop %s: %w", name, err)
		}
	case "restart":
		if err := scmStopAndWait(s, name, timeout); err != nil {
			return err
		}
		if err := s.Start(); err != nil {
			return fmt.Errorf("start %s: %w", name, err)
		}
	case "enable":
		if err := scmSetStartType(s, mgr.StartAutomatic); err != nil {
			return fmt.Errorf("enable %s: %w", name, err)
		}
	case "disable":
		if err := scmSetStartType(s, mgr.StartDisabled); err != nil {
			return fmt.Errorf("disable %s: %w", name, err)
		}
	}
	return nil
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
