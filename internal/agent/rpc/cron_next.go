package rpc

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cron 下次触发计算（见 docs/guide/cron-design.md M2 D14-D16）：
// 解析域与 validateCronExpr 同源（cronFieldRe 形态 + cronFieldRanges 范围 +
// cronAtExtensions 白名单），在服务器本地时区逐分钟扫描。

// cronSchedule 展开后的字段位集合（bit0 对应字段最小值：dom bit0=1 日等）
type cronSchedule struct {
	min    uint64 // 60 bit
	hour   uint32 // 24 bit
	dom    uint32 // 31 bit（1..31 日）
	mon    uint16 // 12 bit（1..12 月）
	dow    uint8  // 7 bit（0..6，0=周日；7 已归一）
	domAny bool
	dowAny bool
}

// cronAtSchedule @ 简写 → 等价 5 字段（crontab(5) 语义）；@reboot 单独处理
var cronAtSchedule = map[string]string{
	"@hourly":   "0 * * * *",
	"@daily":    "0 0 * * *",
	"@weekly":   "0 0 * * 0",
	"@monthly":  "0 0 1 * *",
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
}

// nextCronRun 计算 expr 在 from 之后的下一次触发（服务器本地时区），
// 返回 unix 秒；@reboot / 解析失败 / 5 年内无触发返回 0
func nextCronRun(expr string, from time.Time) int64 {
	s, err := parseCronSchedule(expr)
	if err != nil {
		return 0
	}
	// 从下一整分钟开始（当前分钟正在过的不算「下一次」）
	t := from.Truncate(time.Minute).Add(time.Minute)
	limit := from.AddDate(5, 0, 0)
	for t.Before(limit) {
		if s.matches(t) {
			return t.Unix()
		}
		t = t.Add(time.Minute)
	}
	return 0
}

// matches Vixie cron 触发判定：dom/dow 都受限时 OR，任一为 * 时 AND
func (s *cronSchedule) matches(t time.Time) bool {
	if s.mon&(1<<uint(t.Month())) == 0 {
		return false
	}
	domHit := s.dom&(1<<uint(t.Day())) != 0
	dowHit := s.dow&(1<<uint(t.Weekday())) != 0
	switch {
	case s.domAny && s.dowAny:
		// 两边都任意恒真
	case s.domAny:
		if !dowHit {
			return false
		}
	case s.dowAny:
		if !domHit {
			return false
		}
	default:
		if !domHit && !dowHit {
			return false
		}
	}
	return s.hour&(1<<uint(t.Hour())) != 0 &&
		s.min&(1<<uint(t.Minute())) != 0
}

// parseCronSchedule 表达式 → 位集合；语法域与 validateCronExpr 一致
func parseCronSchedule(expr string) (*cronSchedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("schedule is required")
	}
	if strings.HasPrefix(expr, "@") {
		if expr == "@reboot" {
			return nil, fmt.Errorf("@reboot has no next run")
		}
		if eq, ok := cronAtSchedule[expr]; ok {
			expr = eq
		} else {
			return nil, fmt.Errorf("unsupported @extension %q", expr)
		}
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("schedule must have 5 fields")
	}
	s := &cronSchedule{}
	for i, f := range fields {
		lo, hi := cronFieldRanges[i][0], cronFieldRanges[i][1]
		for _, part := range strings.Split(f, ",") {
			m := cronFieldRe.FindStringSubmatch(part)
			if m == nil {
				return nil, fmt.Errorf("invalid schedule field %d: %q", i+1, part)
			}
			start, end := lo, hi
			if m[1] != "*" {
				n, err := strconv.Atoi(m[1])
				if err != nil || n < lo || n > hi {
					return nil, fmt.Errorf("invalid schedule field %d: %q", i+1, part)
				}
				start = n
				end = n
			}
			if m[2] != "" {
				n, err := strconv.Atoi(strings.TrimPrefix(m[2], "-"))
				if err != nil || n < lo || n > hi {
					return nil, fmt.Errorf("invalid schedule field %d: %q", i+1, part)
				}
				if m[1] == "*" {
					start = lo
				}
				end = n
				if start > end {
					return nil, fmt.Errorf("invalid range in field %d: %q", i+1, part)
				}
			}
			step := 1
			if m[3] != "" {
				n, err := strconv.Atoi(strings.TrimPrefix(m[3], "/"))
				if err != nil || n < 1 {
					return nil, fmt.Errorf("invalid step in field %d: %q", i+1, part)
				}
				step = n
			}
			for v := start; v <= end; v += step {
				switch i {
				case 0:
					s.min |= 1 << uint(v)
				case 1:
					s.hour |= 1 << uint(v)
				case 2:
					s.dom |= 1 << uint(v)
				case 3:
					s.mon |= 1 << uint(v)
				case 4:
					d := v
					if d == 7 {
						d = 0 // dow 7 视同 0（周日）；不修改 v 以免连累步长推进
					}
					s.dow |= 1 << uint(d)
				}
			}
		}
	}
	s.domAny = fields[2] == "*"
	s.dowAny = fields[4] == "*"
	return s, nil
}
