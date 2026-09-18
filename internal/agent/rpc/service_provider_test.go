package rpc

import (
	"context"
	"fmt"
	"os"
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
	// unit 文件读写（D13）：show -p FragmentPath 与 cat 的预置输出
	fragmentPath string
	catOut       string
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
	case "daemon-reload":
		if len(args) != 1 {
			return nil, nil, fmt.Errorf("daemon-reload takes no unit argument: %v", args)
		}
		m.actions = append(m.actions, args[0])
		if m.actionErr != nil {
			return nil, []byte("Failed to reload daemon: " + m.actionErr.Error()), m.actionErr
		}
		return nil, nil, nil
	case "show":
		if len(args) < 2 || args[1] != "-p" {
			return nil, nil, fmt.Errorf("unexpected show args: %v", args)
		}
		return []byte(m.fragmentPath + "\n"), nil, nil
	case "cat":
		return []byte(m.catOut), nil, nil
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

func TestServiceDaemonReload(t *testing.T) {
	// D12：daemon-reload argv 直调 systemctl，无 unit 参数
	m := &mockSystemctl{unitsOut: sampleUnitsOut, filesOut: sampleFilesOut}
	p := NewServiceProvider(m.run)

	res, err := p.DaemonReload()
	if err != nil {
		t.Fatalf("DaemonReload: %v", err)
	}
	if res.(map[string]interface{})["reloaded"] != true {
		t.Errorf("res = %+v", res)
	}
	if len(m.actions) != 1 || m.actions[0] != "daemon-reload" {
		t.Errorf("actions = %v", m.actions)
	}

	// RPC 分发走 Call("daemon-reload") 同路径
	m2 := &mockSystemctl{unitsOut: sampleUnitsOut, filesOut: sampleFilesOut}
	p2 := NewServiceProvider(m2.run)
	if _, err := p2.Call("daemon-reload", nil); err != nil {
		t.Fatalf("Call(daemon-reload): %v", err)
	}
	if len(m2.actions) != 1 {
		t.Errorf("actions = %v", m2.actions)
	}

	// 失败透传 stderr 摘要
	m.actionErr = fmt.Errorf("exit status 1")
	if _, err := p.DaemonReload(); err == nil || !strings.Contains(err.Error(), "daemon-reload") {
		t.Errorf("error not passed through: %v", err)
	}
}

func TestServiceUnitFileIO(t *testing.T) {
	// D13：unit 文件读写。etcSystemdDir 注入临时目录（CI 无 /etc 写权限），
	// FragmentPath 由 mock 控制指向真实临时文件，保存走真实磁盘 IO
	tmp := t.TempDir()
	etcDir := tmp + "/etc/systemd/system"
	libDir := tmp + "/usr/lib/systemd/system"
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldEtc := etcSystemdDir
	// etcSystemdDir 覆写经 t.Cleanup 恢复，panic 也不会泄漏到其他测试；
	// 测试不得触碰真实 /etc
	t.Cleanup(func() { etcSystemdDir = oldEtc })

	t.Run("read", func(t *testing.T) {
		m := &mockSystemctl{fragmentPath: libDir + "/a.service", catOut: "[Unit]\nDescription=A\n"}
		p := NewServiceProvider(m.run)
		res, err := p.UnitFile("a.service")
		if err != nil {
			t.Fatalf("UnitFile: %v", err)
		}
		got := res.(map[string]interface{})
		if got["fragmentPath"] != libDir+"/a.service" || got["content"] != "[Unit]\nDescription=A\n" {
			t.Errorf("UnitFile = %+v", got)
		}
		// 坏名与未安装
		if _, err := p.UnitFile("../etc/passwd"); err == nil {
			t.Error("bad name should fail")
		}
		m2 := &mockSystemctl{fragmentPath: ""}
		p2 := NewServiceProvider(m2.run)
		if _, err := p2.UnitFile("ghost.service"); err == nil {
			t.Error("missing fragment should fail")
		}
	})

	t.Run("save copies package-owned unit to etc override", func(t *testing.T) {
		etcSystemdDir = etcDir
		libUnit := libDir + "/b.service"
		if err := os.WriteFile(libUnit, []byte("[Unit]\nDescription=B\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		m := &mockSystemctl{fragmentPath: libUnit}
		p := NewServiceProvider(m.run)

		res, err := p.SaveUnitFile("b.service", "[Unit]\nDescription=B2\n")
		if err != nil {
			t.Fatalf("SaveUnitFile: %v", err)
		}
		got := res.(map[string]interface{})
		wantTarget := etcDir + "/b.service"
		if got["path"] != wantTarget || got["reloaded"] != true {
			t.Errorf("save result = %+v", got)
		}
		data, err := os.ReadFile(wantTarget)
		if err != nil || string(data) != "[Unit]\nDescription=B2\n" {
			t.Errorf("override content = %q err = %v", data, err)
		}
		// 保存捆绑 daemon-reload
		m.mu.Lock()
		last := m.actions[len(m.actions)-1]
		m.mu.Unlock()
		if last != "daemon-reload" {
			t.Errorf("last action = %q, want daemon-reload", last)
		}
	})

	t.Run("save edits etc unit in place", func(t *testing.T) {
		etcSystemdDir = etcDir
		etcUnit := etcDir + "/c.service"
		if err := os.WriteFile(etcUnit, []byte("[Unit]\nDescription=C\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		m := &mockSystemctl{fragmentPath: etcUnit}
		p := NewServiceProvider(m.run)
		if _, err := p.SaveUnitFile("c.service", "[Unit]\nDescription=C2\n"); err != nil {
			t.Fatalf("SaveUnitFile: %v", err)
		}
		data, _ := os.ReadFile(etcUnit)
		if string(data) != "[Unit]\nDescription=C2\n" {
			t.Errorf("content = %q", data)
		}
	})

	t.Run("save rejects bad name and oversize", func(t *testing.T) {
		etcSystemdDir = etcDir
		m := &mockSystemctl{fragmentPath: etcDir + "/d.service"}
		p := NewServiceProvider(m.run)
		if _, err := p.SaveUnitFile("../x", "data"); err == nil {
			t.Error("bad name should fail")
		}
		if _, err := p.SaveUnitFile("d.service", strings.Repeat("x", unitFileContentLimit+1)); err == nil {
			t.Error("oversize content should fail")
		}
	})
}

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	target := dir + "/unit.file"
	if err := atomicWriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "hello" {
		t.Errorf("content = %q", data)
	}
	fi, _ := os.Stat(target)
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("perm = %v", fi.Mode().Perm())
	}
	// 无临时残留
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("leftover temp files: %d entries", len(entries))
	}
	// 覆盖写
	if err := atomicWriteFile(target, []byte("world"), 0o644); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	data, _ = os.ReadFile(target)
	if string(data) != "world" {
		t.Errorf("content = %q", data)
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
