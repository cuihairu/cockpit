package rpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// 时间解析各形态：UTC/本地时区尾缀剥除、@epoch 微秒、n/a、垃圾输入
func TestParseSystemdTime(t *testing.T) {
	// 剥尾时区缩写按本地时区解析：本地墙钟反解应还原同一时刻
	local := time.Date(2026, 9, 19, 17, 10, 29, 0, time.Local)
	want := local.Unix()
	if got := parseSystemdTime("Sat 2026-09-19 17:10:29 UTC"); got != want {
		t.Fatalf("UTC suffix = %d, want %d", got, want)
	}
	if got := parseSystemdTime("Sat 2026-09-19 17:10:29 CST"); got != want {
		t.Fatalf("CST suffix = %d, want %d", got, want)
	}
	// 无时区尾缀
	if got := parseSystemdTime("Sat 2026-09-19 17:10:29"); got != want {
		t.Fatalf("no suffix = %d, want %d", got, want)
	}
	// @epoch 微秒
	if got := parseSystemdTime("@1797745829123456"); got != 1797745829 {
		t.Fatalf("@epoch = %d", got)
	}
	// 异常形态归 0
	for _, s := range []string{"", "n/a", "garbage", "@notanumber", "Sat 13:99:00 UTC", "2026-09-19"} {
		if got := parseSystemdTime(s); got != 0 {
			t.Errorf("parseSystemdTime(%q) = %d, want 0", s, got)
		}
	}
}

// timers RPC：list-unit-files 首列 + show 多段解析（Id 归属、错误段跳过、
// 模板单元仅列名、多余段忽略、n/a 归 0）
func TestListTimers(t *testing.T) {
	listOut := `apt-daily.timer  enabled enabled
chrony-dnssrv@.timer disabled enabled
fstrim.timer  enabled enabled
`
	showOut := `Id=apt-daily.timer
Description=Daily apt download activities
ActiveState=active
UnitFileState=enabled
LastTriggerUSec=Fri 2026-09-18 18:20:35 UTC
NextElapseUSecRealtime=Sat 2026-09-19 17:10:29 UTC
TimersCalendar={ OnCalendar=*-*-* 06,18:00:00 ; next_elapse=Sat 2026-09-19 06:00:00 UTC }

Failed to get properties: Unit name chrony-dnssrv@.timer is neither a valid invocation ID nor unit name.

Id=fstrim.timer
Description=Discard unused filesystem blocks once a week
ActiveState=active
UnitFileState=enabled
LastTriggerUSec=n/a
NextElapseUSecRealtime=n/a
TimersCalendar={ OnCalendar=Mon *-*-* 00:00:00 ; next_elapse=Mon 2026-09-21 00:00:00 UTC }

Id=not-requested.timer
Description=Segment not in unit list
`
	run := func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		if name != "systemctl" {
			return nil, nil, nil
		}
		if args[0] == "list-unit-files" {
			return []byte(listOut), nil, nil
		}
		return []byte(showOut), nil, nil
	}

	p := NewCronProvider(run)
	res, err := p.ListTimers()
	if err != nil {
		t.Fatal(err)
	}
	timers := res.(map[string]interface{})["timers"].([]map[string]interface{})
	if len(timers) != 3 {
		t.Fatalf("timers = %d entries", len(timers))
	}

	// apt-daily：日程原文 OnCalendar + 墙钟触发时间（本地时区往返自洽）
	a := timers[0]
	if a["unit"] != "apt-daily.timer" || a["description"] != "Daily apt download activities" {
		t.Fatalf("apt-daily head = %+v", a)
	}
	if a["schedule"] != "*-*-* 06,18:00:00" {
		t.Fatalf("apt-daily schedule = %v", a["schedule"])
	}
	if a["nextRun"].(int64) != parseSystemdTime("Sat 2026-09-19 17:10:29 UTC") {
		t.Fatalf("apt-daily nextRun = %v", a["nextRun"])
	}
	if a["lastTrigger"].(int64) != parseSystemdTime("Fri 2026-09-18 18:20:35 UTC") {
		t.Fatalf("apt-daily lastTrigger = %v", a["lastTrigger"])
	}

	// 模板单元：show 失败仅列名、属性留空（不致命）
	c := timers[1]
	if c["unit"] != "chrony-dnssrv@.timer" || c["description"] != "" || c["nextRun"].(int64) != 0 {
		t.Fatalf("template unit = %+v", c)
	}

	// fstrim：n/a 归 0
	f := timers[2]
	if f["unit"] != "fstrim.timer" || f["lastTrigger"].(int64) != 0 || f["nextRun"].(int64) != 0 {
		t.Fatalf("fstrim = %+v", f)
	}
	if f["schedule"] != "Mon *-*-* 00:00:00" {
		t.Fatalf("fstrim schedule = %v", f["schedule"])
	}
}

// monotonic 型日程：多行 { OnBootUSec=... } 拼接，不泄漏 next_elapse
func TestListTimersMonotonicSchedule(t *testing.T) {
	m := map[string]string{
		"TimersMonotonic": "{ OnUnitActiveUSec=1d ; next_elapse=2d 15min }\n{ OnBootUSec=15min ; next_elapse=15min }",
	}
	got := timerSchedule(m)
	if !strings.Contains(got, "OnUnitActiveUSec=1d") || !strings.Contains(got, "OnBootUSec=15min") {
		t.Fatalf("monotonic schedule = %q", got)
	}
	if strings.Contains(got, "next_elapse") {
		t.Fatalf("monotonic schedule leaks next_elapse: %q", got)
	}
}

func TestListTimersNoSystemd(t *testing.T) {
	run := func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		return nil, []byte("System has not been booted with systemd"), errors.New("no systemd")
	}
	p := NewCronProvider(run)
	if _, err := p.ListTimers(); err == nil {
		t.Fatal("want error when systemctl unavailable")
	}
}
