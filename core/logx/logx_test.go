package logx

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestParseLevel 级别解析：四级别 + 大小写 + warning 别名 + 非法回退。
func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"Warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"verbose", slog.LevelInfo},
		{"  info  ", slog.LevelInfo},
	}
	for _, tc := range cases {
		if got := ParseLevel(tc.in); got != tc.want {
			t.Fatalf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestSetupTextAndLevel 过滤与文本形态（缺省 writer 之外的注入面）。
func TestSetupTextAndLevel(t *testing.T) {
	var buf bytes.Buffer
	l := Setup(Options{Level: "warn", Writer: &buf})
	l.Info("dropped", "k", "v")
	l.Warn("kept", "n", 1)
	out := buf.String()
	if strings.Contains(out, "dropped") {
		t.Fatalf("info should be filtered at warn level: %q", out)
	}
	if !strings.Contains(out, "kept") || !strings.Contains(out, "n=1") {
		t.Fatalf("warn record missing: %q", out)
	}
	if strings.Contains(out, `"msg"`) {
		t.Fatalf("text handler expected, got json shape: %q", out)
	}
}

// TestSetupJSON json 形态：字段带引号可反解。
func TestSetupJSON(t *testing.T) {
	var buf bytes.Buffer
	l := Setup(Options{Format: "JSON", Writer: &buf})
	l.Error("boom", "target", "a1")
	out := buf.String()
	if !strings.Contains(out, `"msg":"boom"`) || !strings.Contains(out, `"target":"a1"`) {
		t.Fatalf("json record missing: %q", out)
	}
}

// TestSetupDefaults 缺省参数装配可用（不 panic，info 放行）。
func TestSetupDefaults(t *testing.T) {
	var buf bytes.Buffer
	l := Setup(Options{Writer: &buf})
	l.Info("ok")
	if !strings.Contains(buf.String(), "ok") {
		t.Fatalf("default setup should pass info: %q", buf.String())
	}
}

// TestSetupNilWriter 落 stderr 缺省分支：Writer 为 nil 不 panic、可出记录。
func TestSetupNilWriter(t *testing.T) {
	l := Setup(Options{})
	l.Info("stderr-default")
	// 无断言面（os.Stderr 不回读）；到不了这里即 panic 回归
	if l == nil {
		t.Fatal("Setup should return logger")
	}
}

// TestSetupDefaultSwitch 切换全局默认后 slog.Default() 生效。
func TestSetupDefaultSwitch(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	var buf bytes.Buffer
	l := SetupDefault(Options{Writer: &buf})
	if l != slog.Default() {
		t.Fatal("SetupDefault should switch slog.Default()")
	}
	slog.Info("global")
	if !strings.Contains(buf.String(), "global") {
		t.Fatalf("global convenience call should hit new default: %q", buf.String())
	}
}
