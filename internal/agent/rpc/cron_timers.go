package rpc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// systemd timer 只读列表（见 cron-design.md M3 D18-D20）：不走
// `systemctl list-timers`（表格式输出列随版本变），两段式稳定 API——
// list-unit-files 首列拿 unit 名，一条 show 全量属性查询后空行分段解析
// （每段自带 Id=<unit>，按 Id 归属，错误段无 Id 跳过）。

// timerPropShow systemctl show 的属性子集（稳定 bus 属性名）
var timerPropShow = []string{
	"Id", "Description", "ActiveState", "UnitFileState",
	"LastTriggerUSec", "NextElapseUSecRealtime",
	"TimersCalendar", "TimersMonotonic",
}

// ListTimers 列举系统 timer unit（只读）：unit 名 + 日程原文 + 触发时间。
// show 失败的 unit（如模板 x@.timer）仅列名、属性留空，不致命。
func (p *CronProvider) ListTimers() (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), nginxTestTimeout)
	defer cancel()
	out, stderr, err := p.run(ctx, "systemctl", "list-unit-files", "--type=timer", "--no-legend")
	if err != nil {
		return nil, fmt.Errorf("systemctl list-unit-files: %s", commandErrSummary(stderr, err))
	}
	units := parseFirstColumn(string(out))
	if len(units) == 0 {
		return map[string]interface{}{"timers": []interface{}{}}, nil
	}

	// 一条 show 带全部 unit；模板单元等失败不影响其余段的 stdout
	args := append([]string{"show", "--no-pager"}, units...)
	for _, prop := range timerPropShow {
		args = append(args, "-p", prop)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), nginxTestTimeout)
	defer cancel2()
	showOut, _, _ := p.run(ctx2, "systemctl", args...)
	props := parseSystemdShow(string(showOut))

	timers := make([]map[string]interface{}, 0, len(units))
	for _, u := range units {
		m := props[u]
		if m == nil {
			m = map[string]string{}
		}
		timers = append(timers, map[string]interface{}{
			"unit":          u,
			"description":   m["Description"],
			"schedule":      timerSchedule(m),
			"state":         m["ActiveState"],
			"unitFileState": m["UnitFileState"],
			"lastTrigger":   parseSystemdTime(m["LastTriggerUSec"]),
			"nextRun":       parseSystemdTime(m["NextElapseUSecRealtime"]),
		})
	}
	return map[string]interface{}{"timers": timers}, nil
}

// parseSystemdShow 解析多 unit 的 show 输出：段间空行分隔，段内
// Key=Value，按段内 Id= 归属到 unit；错误段（如模板单元的
// "Failed to get properties: ..."）不含 Id=，跳过
func parseSystemdShow(out string) map[string]map[string]string {
	props := map[string]map[string]string{}
	for _, block := range strings.Split(out, "\n\n") {
		m := map[string]string{}
		id := ""
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			for _, p := range timerPropShow {
				if k == p {
					m[k] = v
					if k == "Id" {
						id = v
					}
					break
				}
			}
		}
		if id != "" {
			props[id] = m
		}
	}
	return props
}

// timerSchedule 日程原文：TimersCalendar 的 OnCalendar= 优先，
// 否则拼 TimersMonotonic 各行（每行一个 { OnBootUSec=15min ; ... }）
func timerSchedule(m map[string]string) string {
	if v, ok := cutTimerValue(m["TimersCalendar"], "OnCalendar="); ok {
		return v
	}
	var parts []string
	for _, line := range strings.Split(m["TimersMonotonic"], "\n") {
		if v, ok := cutTimerKV(line, "On"); ok {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " | ")
}

// cutTimerKV 提取以 key 开头的 `K=v` 整段（保留键名，到 ` ;`/` }`/串尾）
func cutTimerKV(s, key string) (string, bool) {
	i := strings.Index(s, key)
	if i < 0 {
		return "", false
	}
	rest := s[i:]
	end := len(rest)
	for j := 0; j < len(rest); j++ {
		if rest[j] == ';' || rest[j] == '}' {
			end = j
			break
		}
	}
	return strings.TrimSpace(rest[:end]), true
}

// cutTimerValue 从 systemd 结构化属性串 `{ K1=v1 ; K2=v2 }` 中提取
// key 为前缀的首个值（到 ` ;` 或 ` }` 或串尾）
func cutTimerValue(s, key string) (string, bool) {
	i := strings.Index(s, key)
	if i < 0 {
		return "", false
	}
	rest := s[i+len(key):]
	end := len(rest)
	for j := 0; j < len(rest); j++ {
		if rest[j] == ';' || rest[j] == '}' {
			end = j
			break
		}
	}
	return strings.TrimSpace(rest[:end]), true
}

// parseSystemdTime systemctl show 的时间值 → unix 秒：
// "Sat 2026-09-19 17:10:29 UTC" 剥尾时区缩写后按本机本地时区解析
// （systemctl 与 agent 同机同 TZ，墙钟一致）；"@1695..." = epoch 微秒；
// n/a/空/解析失败归 0
func parseSystemdTime(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "n/a" {
		return 0
	}
	if strings.HasPrefix(s, "@") {
		if us, err := strconv.ParseFloat(s[1:], 64); err == nil {
			return int64(us / 1e6)
		}
		return 0
	}
	// 剥尾时区缩写（纯字母 2-5 字符，如 UTC/CST/CEST）
	if i := strings.LastIndexByte(s, ' '); i > 0 {
		tz := s[i+1:]
		if len(tz) >= 2 && len(tz) <= 5 && isAlphaTZ(tz) {
			s = s[:i]
		}
	}
	t, err := time.ParseInLocation("Mon 2006-01-02 15:04:05", s, time.Local)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func isAlphaTZ(s string) bool {
	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')) {
			return false
		}
	}
	return true
}
