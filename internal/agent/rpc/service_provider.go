package rpc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ============ Service Provider ============
//
// systemd 服务管理（见 service-design.md）：只做 *.service unit 的观测与
// 六个动词操作（start/stop/restart/reload/enable/disable），argv 直调
// systemctl 不经 shell。探测 = systemctl 可执行 + /run/systemd/system 存在
// （D2），容器与非 systemd 平台自动跳过。

const (
	// serviceActionTimeout systemctl 操作超时（restart 数据库类服务可能慢）
	serviceActionTimeout = 30 * time.Second
)

var (
	// serviceUnitNameRe unit 名：字母数字与 @ . _ + - ，且必须 .service 结尾（D4）
	serviceUnitNameRe = regexp.MustCompile(`^[A-Za-z0-9@._+-]+\.service$`)
	// serviceActions systemctl 动词白名单（M2 再议 mask/unmask）
	serviceActions = map[string]bool{
		"start": true, "stop": true, "restart": true,
		"reload": true, "enable": true, "disable": true,
	}
)

// ServiceUnit 一个 systemd service unit 的观测快照
type ServiceUnit struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	LoadState     string `json:"loadState"`
	ActiveState   string `json:"activeState"`
	SubState      string `json:"subState"`
	UnitFileState string `json:"unitFileState"` // enabled/disabled/static/... （自启态）
	Preset        string `json:"preset"`
}

// validate unit 名与动作白名单校验（server 侧同规则，双端防御）
func validateServiceUnit(name, action string) error {
	if !serviceUnitNameRe.MatchString(name) {
		return fmt.Errorf("invalid unit name %q (expect *.service)", name)
	}
	if !serviceActions[action] {
		return fmt.Errorf("unsupported action %q (allowed: start stop restart reload enable disable)", action)
	}
	return nil
}

// ServiceProvider systemd 服务管理 Provider
type ServiceProvider struct {
	run Commander
}

func NewServiceProvider(run Commander) *ServiceProvider {
	if run == nil {
		run = defaultCommander
	}
	return &ServiceProvider{run: run}
}

func (p *ServiceProvider) Type() string { return "service" }

func (p *ServiceProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "status":
		return p.Status()
	case "list":
		return p.List()
	case "action":
		return p.DoAction(paramString(params, "name"), paramString(params, "action"))
	default:
		return nil, fmt.Errorf("unknown service action: %s", action)
	}
}

// DetectSystemd 探测 systemd 可用性：systemctl 可执行 + /run/systemd/system 存在
// （排除容器内装了 systemctl 二进制但无 systemd PID 1 的场景，D2）
func DetectSystemd() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	fi, err := os.Stat("/run/systemd/system")
	return err == nil && fi.IsDir()
}

// ============ RPC 实现 ============

// Status 概览：systemd 整体状态 + 服务计数
func (p *ServiceProvider) Status() (interface{}, error) {
	services, err := p.List()
	if err != nil {
		return nil, err
	}
	units := services.(map[string]interface{})["services"].([]ServiceUnit)

	// is-system-running：degraded 等状态下退出码非 0 但 stdout 有值，以 stdout
	// 为准；stdout 为空才视为探测失败
	ctx, cancel := context.WithTimeout(context.Background(), nginxTestTimeout)
	defer cancel()
	state := "unknown"
	out, stderr, err := p.run(ctx, "systemctl", "is-system-running")
	if line := strings.TrimSpace(string(out)); line != "" {
		state = line
	} else if err != nil {
		return nil, fmt.Errorf("systemctl is-system-running: %s", commandErrSummary(stderr, err))
	}

	active, failed, enabled := 0, 0, 0
	for i := range units {
		switch {
		case units[i].ActiveState == "active":
			active++
		case units[i].ActiveState == "failed":
			failed++
		}
		if units[i].UnitFileState == "enabled" {
			enabled++
		}
	}
	return map[string]interface{}{
		"systemState": state,
		"total":       len(units),
		"active":      active,
		"failed":      failed,
		"enabled":     enabled,
	}, nil
}

// List 服务列表：list-units（运行态）与 list-unit-files（安装项 + 自启态）合并
func (p *ServiceProvider) List() (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), serviceActionTimeout)
	defer cancel()

	unitsOut, stderr, err := p.run(ctx, "systemctl", "list-units", "--type=service", "--all", "--no-legend", "--no-pager")
	if err != nil {
		return nil, fmt.Errorf("systemctl list-units: %s", commandErrSummary(stderr, err))
	}
	filesOut, stderr, err := p.run(ctx, "systemctl", "list-unit-files", "--type=service", "--no-legend", "--no-pager")
	if err != nil {
		return nil, fmt.Errorf("systemctl list-unit-files: %s", commandErrSummary(stderr, err))
	}

	units := mergeServiceUnits(parseListUnits(string(unitsOut)), parseUnitFiles(string(filesOut)))
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })
	return map[string]interface{}{"services": units}, nil
}

// DoAction 执行 systemctl 动词（白名单校验后 argv 直传）
func (p *ServiceProvider) DoAction(name, action string) (interface{}, error) {
	if err := validateServiceUnit(name, action); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), serviceActionTimeout)
	defer cancel()
	_, stderr, err := p.run(ctx, "systemctl", action, name)
	if err != nil {
		return nil, fmt.Errorf("systemctl %s %s: %s", action, name, commandErrSummary(stderr, err))
	}
	return map[string]interface{}{"name": name, "action": action}, nil
}

// ============ 内部：systemctl 输出解析 ============

// parseListUnits 解析 list-units --no-legend 输出行：
// UNIT LOAD ACTIVE SUB DESCRIPTION（DESCRIPTION 含空格，取剩余列合并）
func parseListUnits(out string) map[string]*ServiceUnit {
	units := map[string]*ServiceUnit{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 4 || !strings.HasSuffix(fields[0], ".service") {
			continue
		}
		units[fields[0]] = &ServiceUnit{
			Name:        fields[0],
			LoadState:   fields[1],
			ActiveState: fields[2],
			SubState:    fields[3],
			Description: joinDesc(fields[4:]),
		}
	}
	return units
}

// parseUnitFiles 解析 list-unit-files --no-legend 输出行：
// UNIT FILE STATE VENDOR PRESET（VENDOR PRESET 新版才有，可能缺省）
func parseUnitFiles(out string) map[string]*ServiceUnit {
	units := map[string]*ServiceUnit{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || !strings.HasSuffix(fields[0], ".service") {
			continue
		}
		u := &ServiceUnit{
			Name:          fields[0],
			UnitFileState: fields[1],
			ActiveState:   "inactive",
		}
		// systemd 250+ 第 3 列为 VENDOR PRESET（"-" 表示无预设），旧版无此列
		if len(fields) >= 3 && fields[2] != "-" {
			u.Preset = fields[2]
		}
		units[u.Name] = u
	}
	return units
}

// mergeServiceUnits 运行态优先合并：list-units 有 description/load/active/sub，
// list-unit-files 补自启态；只出现在 unit-files 里的为未加载安装项，
// 只出现在 list-units 里的为 transient unit（无 unit file），照常保留
func mergeServiceUnits(running, files map[string]*ServiceUnit) []ServiceUnit {
	merged := make([]ServiceUnit, 0, len(running)+len(files))
	for name, r := range running {
		u := *r
		if f, ok := files[name]; ok {
			u.UnitFileState = f.UnitFileState
			u.Preset = f.Preset
		}
		merged = append(merged, u)
	}
	for name, f := range files {
		if _, ok := running[name]; !ok {
			merged = append(merged, *f)
		}
	}
	return merged
}

// joinDesc DESCRIPTION 列合并（list-units 的第 5 列起）
func joinDesc(rest []string) string {
	return strings.Join(rest, " ")
}
