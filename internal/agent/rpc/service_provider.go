package rpc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ============ Service Provider（systemd 后端）============
//
// 服务管理 systemd 后端（见 service-design.md D2/D9）：只做 *.service unit
// 的观测与六个动词操作（start/stop/restart/reload/enable/disable），argv
// 直调 systemctl 不经 shell。探测 = systemctl 可执行 + /run/systemd/system
// 存在（D2），容器与非 systemd 平台自动跳过；Windows 主机走 windows-scm
// 后端（service_windows_*.go）。

const (
	// serviceActionTimeout systemctl 操作超时（restart 数据库类服务可能慢）
	serviceActionTimeout = 30 * time.Second
)

var (
	// serviceUnitNameRe unit 名：字母数字与 @ . _ + - ，且必须 .service 结尾（D4）
	serviceUnitNameRe = regexp.MustCompile(`^[A-Za-z0-9@._+-]+\.service$`)
	// serviceActions systemctl 动词白名单（M2 扩 mask/unmask，D3）
	serviceActions = map[string]bool{
		"start": true, "stop": true, "restart": true,
		"reload": true, "enable": true, "disable": true,
		"mask": true, "unmask": true,
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
		return fmt.Errorf("unsupported action %q (allowed: start stop restart reload enable disable mask unmask)", action)
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
	case "daemon-reload":
		return p.DaemonReload()
	case "unitfile":
		return p.UnitFile(paramString(params, "name"))
	case "unitsave":
		return p.SaveUnitFile(paramString(params, "name"), paramString(params, "content"))
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

// DaemonReload 刷新 systemd manager 配置（systemctl daemon-reload，D12）：
// unit 文件变更后执行；全局操作不针对 unit，与 DoAction 分开
func (p *ServiceProvider) DaemonReload() (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), serviceActionTimeout)
	defer cancel()
	_, stderr, err := p.run(ctx, "systemctl", "daemon-reload")
	if err != nil {
		return nil, fmt.Errorf("systemctl daemon-reload: %s", commandErrSummary(stderr, err))
	}
	return map[string]interface{}{"reloaded": true}, nil
}

// ============ unit 文件查看与编辑（D13）============

// unitFileContentLimit 保存内容上限（unit 文件通常几 KB）
const unitFileContentLimit = 256 << 10

// etcSystemdDir systemd 管理员覆盖目录：/etc 优先级高于 /usr/lib，
// 保存语义对齐 systemctl edit --full（D13）；var 仅为测试可注入
// （CI 无法写真实 /etc），生产代码不得修改
var etcSystemdDir = "/etc/systemd/system"

// validateServiceUnitName 仅 unit 名校验（unit 文件读写无动作白名单）
func validateServiceUnitName(name string) error {
	if !serviceUnitNameRe.MatchString(name) {
		return fmt.Errorf("invalid unit name %q (expect *.service)", name)
	}
	return nil
}

// systemctlRun systemd 命令快捷封装（超时同 serviceActionTimeout）
func (p *ServiceProvider) systemctlRun(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), serviceActionTimeout)
	defer cancel()
	stdout, stderr, err := p.run(ctx, "systemctl", args...)
	if err != nil {
		return nil, fmt.Errorf("systemctl %s: %s", args[0], commandErrSummary(stderr, err))
	}
	return stdout, nil
}

// fragmentPath 查询 unit 主文件真实路径（未安装/找不到返回空）
func (p *ServiceProvider) fragmentPath(name string) (string, error) {
	out, err := p.systemctlRun("show", "-p", "FragmentPath", "--value", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]), nil
}

// UnitFile 读 unit 文件有效视图（systemctl cat = 主文件 + drop-in 全文，D13）
func (p *ServiceProvider) UnitFile(name string) (interface{}, error) {
	if err := validateServiceUnitName(name); err != nil {
		return nil, err
	}
	frag, err := p.fragmentPath(name)
	if err != nil {
		return nil, err
	}
	if frag == "" {
		return nil, fmt.Errorf("unit %s is not installed (no fragment path)", name)
	}
	content, err := p.systemctlRun("cat", name)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"name":         name,
		"fragmentPath": frag,
		"content":      string(content),
	}, nil
}

// SaveUnitFile 保存 unit 文件（D13）：路径由 FragmentPath 决定不收用户参数；
// 包管文件（/usr/lib 等）先复制到 /etc/systemd/system（/etc 优先级更高，
// systemctl edit --full 同款行为）再写入；临时文件 + rename 原子落盘；
// 自动 daemon-reload 使改动即生效
func (p *ServiceProvider) SaveUnitFile(name, content string) (interface{}, error) {
	if err := validateServiceUnitName(name); err != nil {
		return nil, err
	}
	if len(content) > unitFileContentLimit {
		return nil, fmt.Errorf("unit file content too large: %d bytes (limit %d)", len(content), unitFileContentLimit)
	}
	frag, err := p.fragmentPath(name)
	if err != nil {
		return nil, err
	}
	if frag == "" {
		return nil, fmt.Errorf("unit %s is not installed (no fragment path)", name)
	}

	// 包管文件 → 复制到 /etc 覆盖位（已存在 /etc 版本时以 /etc 为准）
	target := frag
	if !strings.HasPrefix(frag, etcSystemdDir+"/") {
		override := etcSystemdDir + "/" + name
		if data, err := os.ReadFile(frag); err == nil {
			if err := atomicWriteFile(override, data, 0o644); err != nil {
				return nil, fmt.Errorf("copy %s to %s: %w", frag, override, err)
			}
		}
		target = override
	}
	if err := atomicWriteFile(target, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", target, err)
	}
	if _, err := p.DaemonReload(); err != nil {
		return nil, err
	}
	return map[string]interface{}{"name": name, "path": target, "reloaded": true}, nil
}

// atomicWriteFile 临时文件（同目录）+ rename 原子写，不留半截文件

// 文件原语注入点：生产即真实实现，仅供测试覆盖写/权限失败的防御分支
// （本地临时文件这两步实际不会失败）。
var (
	fileWrite = func(f *os.File, b []byte) (int, error) { return f.Write(b) }
	fileChmod = func(f *os.File, m os.FileMode) error { return f.Chmod(m) }
)

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".cockpit-unit-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := fileWrite(tmp, data); err != nil {
		tmp.Close()
		return err
	}
	if err := fileChmod(tmp, perm); err != nil {
		tmp.Close()
		return err
	}
	if err := fileClose(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = "" // rename 成功后无需清理
	return nil
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
