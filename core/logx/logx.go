// Package logx slog 装配（agent-core ① 通用能力层，标准库零三方依赖）。
// 落底选型：新代码用 logx，存量 log.Printf 不强迁（架构文档 §8，随批次
// 渐进）。零业务、零平台分支。
package logx

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options 装配参数。Level 缺省 info（非法值回退 info）；Format 缺省
// text（json 供采集器对接）；Writer 缺省 os.Stderr（与 stdlib log 默认
// 去向一致）。
type Options struct {
	Level  string
	Format string
	Writer io.Writer
}

// Setup 按参数装配 *slog.Logger。
func Setup(o Options) *slog.Logger {
	w := o.Writer
	if w == nil {
		w = os.Stderr
	}
	hopts := &slog.HandlerOptions{Level: ParseLevel(o.Level)}
	var h slog.Handler
	if strings.EqualFold(strings.TrimSpace(o.Format), "json") {
		h = slog.NewJSONHandler(w, hopts)
	} else {
		h = slog.NewTextHandler(w, hopts)
	}
	return slog.New(h)
}

// SetupDefault 装配并切换 slog 默认 logger（log/slog 顶层便捷函数与
// slog.Default() 的消费方一并生效）。
func SetupDefault(o Options) *slog.Logger {
	l := Setup(o)
	slog.SetDefault(l)
	return l
}

// ParseLevel 解析日志级别串（大小写不敏感；debug/info/warn/error，
// 空串与非法值回退 info——装配面宁降不崩）。
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
