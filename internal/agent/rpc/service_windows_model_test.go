package rpc

import "testing"

// ============ Windows SCM 模型层测试（service-design.md D9.7）============
//
// 映射与校验纯函数无 build tag，Linux CI 直接覆盖；SCM 真采集在 Windows
// 真机验收（todo.md）。

func TestWindowsStatusToActiveState(t *testing.T) {
	cases := []struct {
		status string
		active string
		sub    string
	}{
		{"Running", "active", "running"},
		{"Stopped", "inactive", "stopped"},
		{"Start Pending", "activating", "start_pending"},
		{"Continue Pending", "activating", "continue_pending"},
		{"Stop Pending", "deactivating", "stop_pending"},
		{"Paused", "paused", "paused"},
		{"Pause Pending", "paused", "pause_pending"},
		// 未知状态不扭曲语义：原词小写兜底
		{"Weird State", "unknown", "weird_state"},
		{"", "unknown", ""},
	}
	for _, c := range cases {
		active, sub := windowsStatusToActiveState(c.status)
		if active != c.active || sub != c.sub {
			t.Errorf("windowsStatusToActiveState(%q) = (%q,%q), want (%q,%q)",
				c.status, active, sub, c.active, c.sub)
		}
	}
}

func TestStartTypeToUnitFileState(t *testing.T) {
	cases := []struct {
		startType string
		want      string
	}{
		{"Automatic", "enabled"},
		{"Automatic (Delayed)", "enabled"},
		{"Manual", "disabled"},
		{"Disabled", "disabled"},
		{"Unknown", "unknown"},
		{"", "unknown"},
	}
	for _, c := range cases {
		if got := startTypeToUnitFileState(c.startType); got != c.want {
			t.Errorf("startTypeToUnitFileState(%q) = %q, want %q", c.startType, got, c.want)
		}
	}
}

func TestWinServiceToUnit(t *testing.T) {
	u := winServiceToUnit(WinService{
		Name:        "wuauserv",
		DisplayName: "Windows Update",
		Status:      "Running",
		StartType:   "Automatic (Delayed)",
	})
	if u.Name != "wuauserv" || u.Description != "Windows Update" {
		t.Errorf("name/description = %q/%q", u.Name, u.Description)
	}
	// D9.1：Windows 服务必然注册在 SCM，LoadState 恒 loaded、无 preset
	if u.LoadState != "loaded" || u.Preset != "" {
		t.Errorf("loadState/preset = %q/%q, want loaded/\"\"", u.LoadState, u.Preset)
	}
	if u.ActiveState != "active" || u.SubState != "running" || u.UnitFileState != "enabled" {
		t.Errorf("states = %s/%s/%s", u.ActiveState, u.SubState, u.UnitFileState)
	}
}

func TestValidateWindowsServiceUnit(t *testing.T) {
	// 合法：SCM 常见服务名形态（字母数字、点、下划线、连字符、空格）
	for _, name := range []string{"wuauserv", "Lenovo Vantage Service", "plc_net", "webclient-1.2", "Spooler.SQL.1"} {
		if err := validateWindowsServiceUnit(name, "restart"); err != nil {
			t.Errorf("validate(%q, restart) = %v, want nil", name, err)
		}
	}
	// 非法名：路径分隔、目录引用、控制字符、超长、空
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "svc\tname", "svc\x00name", "svc$name", string(make([]byte, 257))} {
		if err := validateWindowsServiceUnit(name, "start"); err == nil {
			t.Errorf("validate(%q) should reject", name)
		}
	}
	// 动作白名单：Windows 后端 5 动词，reload 明确拒绝（D9.3）
	for _, action := range []string{"start", "stop", "restart", "enable", "disable"} {
		if err := validateWindowsServiceUnit("wuauserv", action); err != nil {
			t.Errorf("validate action %q = %v, want nil", action, err)
		}
	}
	for _, action := range []string{"reload", "mask", "", "restart-force"} {
		if err := validateWindowsServiceUnit("wuauserv", action); err == nil {
			t.Errorf("validate action %q should reject", action)
		}
	}
}
