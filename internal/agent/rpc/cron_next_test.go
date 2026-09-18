package rpc

import (
	"encoding/json"
	"testing"
	"time"
)

// 固定基准：2026-09-18 10:30:15 本地时间（不含分钟整点，验证取下一整分钟）
func nextBase() time.Time {
	return time.Date(2026, 9, 18, 10, 30, 15, 0, time.Local)
}

func at(t *testing.T, unix int64) time.Time {
	t.Helper()
	return time.Unix(unix, 0)
}

func TestNextCronRunBasic(t *testing.T) {
	base := nextBase()

	// 每分钟 → 下一整分钟 10:31
	if got := at(t, nextCronRun("* * * * *", base)); got.Format("15:04") != "10:31" {
		t.Fatalf("every minute = %s", got)
	}
	// 固定分钟 35 * * * * → 当天 10:35
	if got := at(t, nextCronRun("35 * * * *", base)); got.Format("15:04") != "10:35" {
		t.Fatalf("minute 35 = %s", got)
	}
	// 步长 */15 → 10:45
	if got := at(t, nextCronRun("*/15 * * * *", base)); got.Format("15:04") != "10:45" {
		t.Fatalf("*/15 = %s", got)
	}
	// 范围步长 5-25/10 → 10:35（5,15,25 中下一个是 35? 不——5-25 范围内 35 不在；下一个是明天 00:05）
	// 修正：5-25/10 = {5,15,25}，当天 10:30 后无 → 11:05? 时字段是 * 所以每小时 → 10:35 不在集合 → 11:05 不对：分钟集合 {5,15,25}，11:05 最早
	if got := at(t, nextCronRun("5-25/10 * * * *", base)); got.Format("15:04") != "11:05" {
		t.Fatalf("5-25/10 = %s", got)
	}
	// 时+分组合 0 9-17/2 * * * → 11:00（9,11,13...；10:30 之后下一个满足时=11 分=0）
	if got := at(t, nextCronRun("0 9-17/2 * * *", base)); got.Format("15:04") != "11:00" {
		t.Fatalf("0 9-17/2 = %s", got)
	}
}

func TestNextCronRunAtExtensions(t *testing.T) {
	base := nextBase()

	// @daily → 明天 00:00（当天 00:00 已过）
	if got := at(t, nextCronRun("@daily", base)); got.Format("2006-01-02 15:04") != "2026-09-19 00:00" {
		t.Fatalf("@daily = %s", got)
	}
	// @hourly → 11:00
	if got := at(t, nextCronRun("@hourly", base)); got.Format("15:04") != "11:00" {
		t.Fatalf("@hourly = %s", got)
	}
	// @weekly → 下个周日 00:00（2026-09-18 是周五）
	if got := at(t, nextCronRun("@weekly", base)); got.Format("2006-01-02 15:04") != "2026-09-20 00:00" {
		t.Fatalf("@weekly = %s", got)
	}
	// @monthly → 10-01 00:00；@yearly/@annually → 2027-01-01
	if got := at(t, nextCronRun("@monthly", base)); got.Format("2006-01-02") != "2026-10-01" {
		t.Fatalf("@monthly = %s", got)
	}
	if got := at(t, nextCronRun("@yearly", base)); got.Format("2006-01-02") != "2027-01-01" {
		t.Fatalf("@yearly = %s", got)
	}
	// @reboot → 0（无下次触发）
	if got := nextCronRun("@reboot", base); got != 0 {
		t.Fatalf("@reboot = %d", got)
	}
}

func TestNextCronRunDomDowSemantics(t *testing.T) {
	base := nextBase() // 2026-09-18 周五

	// dom 与 dow 都受限 → OR：0 0 1 * 1 = 每月 1 号或每周一
	// 下一个周一 09-21、下个 1 号 10-01 → 取 09-21
	if got := at(t, nextCronRun("0 0 1 * 1", base)); got.Format("2006-01-02") != "2026-09-21" {
		t.Fatalf("OR semantics = %s", got)
	}
	// 仅 dom 受限 → AND（dow=* 恒真）：0 0 1 * * → 10-01
	if got := at(t, nextCronRun("0 0 1 * *", base)); got.Format("2006-01-02") != "2026-10-01" {
		t.Fatalf("dom only = %s", got)
	}
	// 仅 dow 受限：0 0 * * 0（周日）→ 09-20
	if got := at(t, nextCronRun("0 0 * * 0", base)); got.Format("2006-01-02") != "2026-09-20" {
		t.Fatalf("dow only = %s", got)
	}
	// dow 7 视同 0（周日）
	if got := at(t, nextCronRun("0 0 * * 7", base)); got.Format("2006-01-02") != "2026-09-20" {
		t.Fatalf("dow 7 = %s", got)
	}
	// dow 范围 1-5（周一到周五）→ 周五当天在集合内：12:00
	if got := at(t, nextCronRun("0 12 * * 1-5", base)); got.Format("2006-01-02 15:04") != "2026-09-18 12:00" {
		t.Fatalf("dow range = %s", got)
	}
	// dow 范围 1-4（周一到周四）→ 周五不在集合，下一个是 09-21（周一）
	if got := at(t, nextCronRun("0 12 * * 1-4", base)); got.Format("2006-01-02") != "2026-09-21" {
		t.Fatalf("dow range 1-4 = %s", got)
	}
}

func TestNextCronRunLeapYear(t *testing.T) {
	// 0 0 29 2 *：2026 起扫要跨到 2028-02-29（闰年）
	base := nextBase()
	if got := at(t, nextCronRun("0 0 29 2 *", base)); got.Format("2006-01-02") != "2028-02-29" {
		t.Fatalf("leap year = %s", got)
	}
}

func TestNextCronRunEdgeCases(t *testing.T) {
	base := nextBase()

	// 整分钟时刻：下一分钟（不含当前）
	exact := time.Date(2026, 9, 18, 10, 30, 0, 0, time.Local)
	if got := at(t, nextCronRun("30 * * * *", exact)); got.Format("15:04") != "11:30" {
		t.Fatalf("exact minute = %s", got)
	}

	// 非法表达式 → 0 不致命
	for _, expr := range []string{"", "not cron", "* 99 * * *", "*/0 * * * *", "@nonsense", "60 * * * *", "* * * *"} {
		if got := nextCronRun(expr, base); got != 0 {
			t.Errorf("expr %q = %d, want 0", expr, got)
		}
	}

	// 5 年内无触发（2 月 30 不存在）→ 0
	if got := nextCronRun("0 0 30 2 *", base); got != 0 {
		t.Fatalf("impossible date = %d", got)
	}
}

func TestCronJobsNextRunShape(t *testing.T) {
	// Jobs() 集成：enabled 任务带 next_run>0，disabled 为 0（meta 对 = 注释行 + 命令行）
	enabled := `# cockpit:job {"name":"tick","schedule":"* * * * *","command":"/bin/x","enabled":true}` + "\n* * * * * /bin/x"
	disabled := `# cockpit:job {"name":"off","schedule":"* * * * *","command":"/bin/y","enabled":false}` + "\n#* * * * * /bin/y"
	runner := &mockCronRunner{hasFile: true, content: enabled + "\n" + disabled + "\n"}

	p := NewCronProvider(runner.run)
	res, err := p.Jobs()
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]interface{})
	jobsRaw, _ := json.Marshal(m["jobs"])
	type jobRow struct {
		Name    string `json:"name"`
		NextRun int64  `json:"next_run"`
		Enabled bool   `json:"enabled"`
	}
	var jobs []jobRow
	if err := json.Unmarshal(jobsRaw, &jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v", jobs)
	}
	for _, j := range jobs {
		if j.Name == "tick" && (j.NextRun <= 0 || !j.Enabled) {
			t.Fatalf("enabled job = %+v", j)
		}
		if j.Name == "off" && (j.NextRun != 0 || j.Enabled) {
			t.Fatalf("disabled job = %+v", j)
		}
	}
}
