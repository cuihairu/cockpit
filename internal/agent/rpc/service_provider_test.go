package rpc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// mockSystemctl 模拟 systemctl 命令：按子命令返回预置输出。
// unitsOut/filesOut 为 list-units / list-unit-files 的输出；
// isRunningOut 为 is-system-running 输出（可配 exitErr 模拟 degraded）；
// actionErr 注入操作失败（stderr 摘要断言用）；actions 记录每次动词调用。
type mockSystemctl struct {
	mu           sync.Mutex
	unitsOut     string
	filesOut     string
	isRunningOut string
	isRunningErr error
	actionErr    error
	actions      []string
}

func (m *mockSystemctl) run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name != "systemctl" {
		return nil, nil, fmt.Errorf("unexpected command: %s", name)
	}
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("no subcommand")
	}
	switch args[0] {
	case "list-units":
		return []byte(m.unitsOut), nil, nil
	case "list-unit-files":
		return []byte(m.filesOut), nil, nil
	case "is-system-running":
		return []byte(m.isRunningOut), nil, m.isRunningErr
	case "start", "stop", "restart", "reload", "enable", "disable", "mask", "unmask":
		m.actions = append(m.actions, args[0]+" "+args[1])
		if m.actionErr != nil {
			return nil, []byte("Failed to " + args[0] + " " + args[1] + ": Unit is masked"), m.actionErr
		}
		return nil, nil, nil
	}
	return nil, nil, fmt.Errorf("unexpected systemctl subcommand: %v", args)
}

const sampleUnitsOut = `nginx.service loaded active running A high performance web server
cockpit-agent.service loaded active running Cockpit Agent (main process)
unbound.service loaded failed failed Validating, recursive, caching DNS resolver
transient-run.service loaded active running /tmp transient task
`

const sampleFilesOut = `nginx.service enabled enabled
cockpit-agent.service enabled enabled
unbound.service enabled enabled
docker.service disabled enabled
backup.service static -
transient-run.service alias -
`

func TestServiceListMerge(t *testing.T) {
	m := &mockSystemctl{unitsOut: sampleUnitsOut, filesOut: sampleFilesOut}
	p := NewServiceProvider(m.run)

	res, err := p.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	units := res.(map[string]interface{})["services"].([]ServiceUnit)
	if len(units) != 6 {
		t.Fatalf("units = %d, want 6", len(units))
	}

	byName := map[string]ServiceUnit{}
	for _, u := range units {
		byName[u.Name] = u
	}

	// 运行态 + 自启态合并（description 含空格）
	ng := byName["nginx.service"]
	if ng.ActiveState != "active" || ng.SubState != "running" || ng.LoadState != "loaded" ||
		ng.UnitFileState != "enabled" || ng.Description != "A high performance web server" {
		t.Errorf("nginx = %+v", ng)
	}

	// 未加载的安装项：activeState 兜底 inactive
	dk := byName["docker.service"]
	if dk.ActiveState != "inactive" || dk.UnitFileState != "disabled" || dk.Preset != "enabled" {
		t.Errorf("docker = %+v", dk)
	}

	// static + vendor preset "-"
	bs := byName["backup.service"]
	if bs.UnitFileState != "static" || bs.Preset != "" {
		t.Errorf("backup = %+v", bs)
	}

	// transient unit（只在 list-units 里出现）保留
	tr, ok := byName["transient-run.service"]
	if !ok || tr.ActiveState != "active" || tr.UnitFileState != "alias" {
		t.Errorf("transient = %+v ok = %v", tr, ok)
	}

	// 按名称排序
	for i := 1; i < len(units); i++ {
		if units[i-1].Name >= units[i].Name {
			t.Errorf("units not sorted: %s >= %s", units[i-1].Name, units[i].Name)
		}
	}
}

func TestServiceStatusCounts(t *testing.T) {
	m := &mockSystemctl{
		unitsOut:     sampleUnitsOut,
		filesOut:     sampleFilesOut,
		isRunningOut: "degraded\n",
		isRunningErr: fmt.Errorf("exit status 1"), // degraded 时退出码非 0 但 stdout 有值
	}
	p := NewServiceProvider(m.run)

	res, err := p.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	st := res.(map[string]interface{})
	if st["systemState"] != "degraded" {
		t.Errorf("systemState = %v", st["systemState"])
	}
	if st["total"] != 6 || st["active"] != 3 || st["failed"] != 1 || st["enabled"] != 3 {
		t.Errorf("counts = %+v", st)
	}
}

func TestServiceActionWhitelist(t *testing.T) {
	m := &mockSystemctl{unitsOut: sampleUnitsOut, filesOut: sampleFilesOut}
	p := NewServiceProvider(m.run)

	// 合法动作（M2 扩 mask/unmask）
	for _, action := range []string{"restart", "mask", "unmask"} {
		if _, err := p.DoAction("nginx.service", action); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	if len(m.actions) != 3 || m.actions[1] != "mask nginx.service" {
		t.Errorf("actions = %v", m.actions)
	}

	// 非法动词 / 危险 unit 名 / 缺 .service 后缀
	for _, tc := range []struct{ name, action string }{
		{"nginx.service", "start; reboot"}, // 注入样例
		{"nginx.service", "systemctl"},     // 白名单外动词
		{"../etc/passwd", "start"},         // 路径穿越
		{"nginx", "start"},                 // 缺 .service
		{"nginx.socket", "start"},          // 非 service 类型
		{"", "start"},
	} {
		if _, err := p.DoAction(tc.name, tc.action); err == nil {
			t.Errorf("DoAction(%q, %q) should fail", tc.name, tc.action)
		}
	}
	if len(m.actions) != 3 {
		t.Errorf("rejected calls must not reach systemctl, actions = %v", m.actions)
	}

	// 失败透传 stderr 摘要
	m.actionErr = fmt.Errorf("exit status 1")
	_, err := p.DoAction("unbound.service", "start")
	if err == nil || !strings.Contains(err.Error(), "Unit is masked") {
		t.Errorf("stderr not passed through: %v", err)
	}
}

func TestServiceActionTimeoutArgv(t *testing.T) {
	// argv 直传验证：动作与 unit 名各占一个参数，不经 shell 拼接
	m := &mockSystemctl{unitsOut: sampleUnitsOut, filesOut: sampleFilesOut}
	p := NewServiceProvider(m.run)
	if _, err := p.DoAction("my@template@1.service", "enable"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if len(m.actions) != 1 || m.actions[0] != "enable my@template@1.service" {
		t.Errorf("actions = %v", m.actions)
	}
}

func TestValidateServiceUnit(t *testing.T) {
	// 合法：模板化 unit 名 + 全部白名单动词
	for _, name := range []string{"nginx.service", "user@1000.service", "openvpn@server.service", "my-app.service"} {
		for _, action := range []string{"start", "stop", "restart", "reload", "enable", "disable"} {
			if err := validateServiceUnit(name, action); err != nil {
				t.Errorf("validate(%q, %q): %v", name, action, err)
			}
		}
	}
}
