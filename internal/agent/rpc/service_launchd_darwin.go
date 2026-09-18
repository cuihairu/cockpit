//go:build darwin

package rpc

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ============ macOS launchd 服务管理 Provider（service-design.md D10）============
//
// 只管 system 域 LaunchDaemons（agent 以 root 运行前提与 systemd 侧同）：
// launchctl list + plist 目录扫描合并观测，launchctl 现代动词执行动作
// （bootout 即 stop、kickstart -k 即 restart、enable/disable 写 disabled DB）。

// launchdDaemonDirs 扫描的 LaunchDaemons 目录（D10.1：不含用户域 LaunchAgents）
var launchdDaemonDirs = []string{
	"/Library/LaunchDaemons",
	"/System/Library/LaunchDaemons",
}

// LaunchdServiceProvider launchd 服务管理 Provider
type LaunchdServiceProvider struct {
	run Commander
}

func NewLaunchdServiceProvider(run Commander) *LaunchdServiceProvider {
	if run == nil {
		run = defaultCommander
	}
	return &LaunchdServiceProvider{run: run}
}

func (p *LaunchdServiceProvider) Type() string { return "service" }

func (p *LaunchdServiceProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
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

// List 观测：launchctl list（运行态）+ LaunchDaemons 目录扫描（安装项 +
// RunAtLoad/Disabled）按 Label 合并；仅出现在 launchctl list 的为 launchd
// 内置服务（无 plist），Path 留空照常保留
func (p *LaunchdServiceProvider) List() (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), serviceActionTimeout)
	defer cancel()
	out, stderr, err := p.run(ctx, "launchctl", "list")
	if err != nil {
		return nil, fmt.Errorf("launchctl list: %s", commandErrSummary(stderr, err))
	}
	runtimes := parseLaunchctlList(string(out))

	plists := p.scanPlistDaemons()
	units := make([]ServiceUnit, 0, len(plists)+len(runtimes))
	seen := map[string]bool{}
	for _, svc := range plists {
		rt := runtimes[svc.Label]
		units = append(units, launchdToUnit(svc, &rt))
		seen[svc.Label] = true
	}
	for label, rt := range runtimes {
		if seen[label] {
			continue
		}
		units = append(units, launchdToUnit(LaunchdService{Label: label}, &rt))
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })
	return map[string]interface{}{"services": units}, nil
}

// scanPlistDaemons 扫 LaunchDaemons 目录解析安装项：单文件解析失败跳过
// （log），目录不存在忽略（防御 /System 缺失的非常规环境）
func (p *LaunchdServiceProvider) scanPlistDaemons() []LaunchdService {
	var out []LaunchdService
	for _, dir := range launchdDaemonDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".plist") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				log.Printf("launchd scan: read %s: %v", path, err)
				continue
			}
			svc, err := parsePlistDaemon(data)
			if err != nil {
				log.Printf("launchd scan: parse %s: %v", path, err)
				continue
			}
			if svc.Label == "" {
				continue // 无 Label 的 plist 无法操作，跳过
			}
			svc.Path = path
			out = append(out, svc)
		}
	}
	return out
}

// Status 概览：launchd 无全局状态概念（PID 1 恒在），systemState 恒 unknown
// （D10.5，前端按 backend 隐藏该项），只提供计数
func (p *LaunchdServiceProvider) Status() (interface{}, error) {
	listRaw, err := p.List()
	if err != nil {
		return nil, err
	}
	units := listRaw.(map[string]interface{})["services"].([]ServiceUnit)
	active, failed, enabled := 0, 0, 0
	for _, u := range units {
		switch u.ActiveState {
		case "active":
			active++
		case "failed":
			failed++
		}
		if u.UnitFileState == "enabled" {
			enabled++
		}
	}
	return map[string]interface{}{
		"systemState": "unknown",
		"total":       len(units),
		"active":      active,
		"failed":      failed,
		"enabled":     enabled,
	}, nil
}

// DoAction 执行 launchctl 动词（白名单校验后 argv 直调，system 域服务目标）
func (p *LaunchdServiceProvider) DoAction(name, action string) (interface{}, error) {
	if err := validateLaunchdUnit(name, action); err != nil {
		return nil, err
	}
	if action == "reload" {
		return nil, fmt.Errorf("reload is not supported on launchd backend")
	}
	target := "system/" + name
	ctx, cancel := context.WithTimeout(context.Background(), serviceActionTimeout)
	defer cancel()

	runLaunch := func(args ...string) error {
		_, stderr, err := p.run(ctx, "launchctl", args...)
		if err != nil {
			return fmt.Errorf("launchctl %s: %s", strings.Join(args, " "), commandErrSummary(stderr, err))
		}
		return nil
	}

	switch action {
	case "start":
		// kickstart 仅对已加载服务有效；bootout 过的回退 bootstrap plist（D10.3）
		if err := runLaunch("kickstart", target); err != nil {
			plist, findErr := p.findLaunchdPlist(name)
			if findErr != nil {
				return nil, err // 保留 kickstart 原错误
			}
			if err := runLaunch("bootstrap", "system", plist); err != nil {
				return nil, err
			}
		}
	case "stop":
		if err := runLaunch("bootout", target); err != nil {
			return nil, err
		}
	case "restart":
		if err := runLaunch("kickstart", "-k", target); err != nil {
			return nil, err
		}
	case "enable":
		if err := runLaunch("enable", target); err != nil {
			return nil, err
		}
	case "disable":
		if err := runLaunch("disable", target); err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{"name": name, "action": action}, nil
}

// findLaunchdPlist 按 Label 反查 plist 路径（start 的 bootstrap 回退用）
func (p *LaunchdServiceProvider) findLaunchdPlist(label string) (string, error) {
	for _, svc := range p.scanPlistDaemons() {
		if svc.Label == label && svc.Path != "" {
			return svc.Path, nil
		}
	}
	return "", fmt.Errorf("no plist found for label %s", label)
}
