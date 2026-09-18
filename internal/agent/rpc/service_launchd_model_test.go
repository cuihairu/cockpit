package rpc

import (
	"strings"
	"testing"
)

// ============ macOS launchd 模型层测试（service-design.md D10.6）============

const samplePlistFull = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.example.web</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/local/bin/web</string>
		<key>label-inside-array-must-skip</key>
		<string>x</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
</dict>
</plist>`

func TestParsePlistDaemon(t *testing.T) {
	// 完整 plist：Label 提取、RunAtLoad 命中、array/dict 复杂值整树跳过
	// （array 内的 key 不干扰外层状态机）
	svc, err := parsePlistDaemon([]byte(samplePlistFull))
	if err != nil {
		t.Fatalf("parse full: %v", err)
	}
	if svc.Label != "com.example.web" || !svc.RunAtLoad || svc.Disabled {
		t.Errorf("full = %+v", svc)
	}

	// Disabled=true / RunAtLoad 缺省组合
	svc, err = parsePlistDaemon([]byte(`<plist><dict>
		<key>Label</key><string>a.b</string>
		<key>Disabled</key><true/>
	</dict></plist>`))
	if err != nil || svc.Label != "a.b" || svc.RunAtLoad || !svc.Disabled {
		t.Errorf("disabled = %+v, err %v", svc, err)
	}

	// 缺 Label（无法操作）
	svc, _ = parsePlistDaemon([]byte(`<plist><dict><key>RunAtLoad</key><true/></dict></plist>`))
	if svc.Label != "" {
		t.Errorf("missing label = %+v", svc)
	}

	// 格式坏文件报错
	if _, err := parsePlistDaemon([]byte(`<plist><dict>`)); err == nil {
		t.Error("truncated plist should error")
	}
	// 空 dict 合法（全零值）
	if svc, err := parsePlistDaemon([]byte(`<plist><dict></dict></plist>`)); err != nil || svc.Label != "" {
		t.Errorf("empty dict = %+v, err %v", svc, err)
	}
}

func TestParseLaunchctlList(t *testing.T) {
	out := "123\t0\tcom.apple.diskarbitrationd\n" +
		"-\t-\tcom.example.never-ran\n" +
		"456\t5\tcom.example.crashed\n" +
		"garbage-line\n"
	m := parseLaunchctlList(out)
	if len(m) != 3 {
		t.Fatalf("entries = %d, want 3", len(m))
	}
	if m["com.example.never-ran"].PID != "-" || m["com.example.never-ran"].Status != "-" {
		t.Errorf("never-ran = %+v", m["com.example.never-ran"])
	}
	if m["com.example.crashed"].PID != "456" || m["com.example.crashed"].Status != "5" {
		t.Errorf("crashed = %+v", m["com.example.crashed"])
	}
}

func TestLaunchdActiveState(t *testing.T) {
	cases := []struct{ pid, status, want string }{
		{"123", "0", "active"},
		{"123", "7", "active"}, // PID 在即 active（上次退出码不影响当前态）
		{"-", "-", "inactive"},
		{"-", "0", "inactive"}, // 上次正常退出
		{"-", "5", "failed"},
		{"", "5", "failed"},
		{"", "", "inactive"},
	}
	for _, c := range cases {
		if got := launchdActiveState(c.pid, c.status); got != c.want {
			t.Errorf("launchdActiveState(%q,%q) = %q, want %q", c.pid, c.status, got, c.want)
		}
	}
}

func TestLaunchdToUnit(t *testing.T) {
	// 自启：RunAtLoad && !Disabled → enabled；运行中
	u := launchdToUnit(LaunchdService{Label: "com.example.web", Path: "/Library/LaunchDaemons/web.plist", RunAtLoad: true},
		&LaunchdRuntime{PID: "9", Status: "0"})
	if u.Name != "com.example.web" || u.Description != "/Library/LaunchDaemons/web.plist" ||
		u.ActiveState != "active" || u.UnitFileState != "enabled" || u.LoadState != "loaded" || u.Preset != "" {
		t.Errorf("unit = %+v", u)
	}
	// Disabled 或无 RunAtLoad → disabled；无运行态记录 → inactive
	for _, svc := range []LaunchdService{
		{Label: "a.b", RunAtLoad: true, Disabled: true},
		{Label: "a.b"},
	} {
		if u := launchdToUnit(svc, nil); u.UnitFileState != "disabled" || u.ActiveState != "inactive" {
			t.Errorf("svc %+v → %+v", svc, u)
		}
	}
}

func TestValidateLaunchdUnit(t *testing.T) {
	// 合法：反向域名惯例 label
	for _, label := range []string{"com.apple.diskarbitrationd", "homebrew.mxcl.nginx", "sshd_1", "a"} {
		if err := validateLaunchdUnit(label, "restart"); err != nil {
			t.Errorf("validate(%q, restart) = %v, want nil", label, err)
		}
	}
	// 非法：分隔符、空格、空、点开头、超长
	for _, label := range []string{"", ".com.x", "a/b", "com ex", "a\nb", strings.Repeat("a", 256)} {
		if err := validateLaunchdUnit(label, "start"); err == nil {
			t.Errorf("validate(%q) should reject", label)
		}
	}
	// 动作白名单：5 动词过、reload 明确拒绝（D10.3）
	for _, action := range []string{"start", "stop", "restart", "enable", "disable"} {
		if err := validateLaunchdUnit("a.b", action); err != nil {
			t.Errorf("validate action %q = %v, want nil", action, err)
		}
	}
	for _, action := range []string{"reload", "bootstrap", "", "kickstart"} {
		if err := validateLaunchdUnit("a.b", action); err == nil {
			t.Errorf("validate action %q should reject", action)
		}
	}
}
