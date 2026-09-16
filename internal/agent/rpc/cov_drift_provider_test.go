package rpc

// 覆盖率补充测试：drift_provider.go 错误分支与默认值路径。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCovNewDriftBaselinePathSources(t *testing.T) {
	t.Setenv("COCKPIT_DRIFT_BASELINE", "/from/env.json")
	if b := NewDriftBaseline(""); b.path != "/from/env.json" {
		t.Errorf("env path = %q", b.path)
	}
	t.Setenv("COCKPIT_DRIFT_BASELINE", "")
	if b := NewDriftBaseline(""); b.path != driftBaselineDefaultPath {
		t.Errorf("default path = %q", b.path)
	}
	if b := NewDriftBaseline("/explicit.json"); b.path != "/explicit.json" {
		t.Errorf("explicit path = %q", b.path)
	}
}

func TestCovDriftBaselineRecordForgetLogOnly(t *testing.T) {
	// /proc 下不可写：update 失败仅记日志，不 panic
	b := NewDriftBaseline("/proc/cockpit-cov/baseline.json")
	b.Record("nginx", "x", []byte("c"))
	b.Forget("nginx", "x")
}

func TestCovDriftBaselineLoadCorrupted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	os.WriteFile(path, []byte("{corrupt"), 0o600)
	b := NewDriftBaseline(path)
	if snap := b.snapshot(); len(snap) != 0 {
		t.Errorf("corrupted baseline snapshot = %v", snap)
	}

	// 合法 JSON 但无 entries → nil map 补空
	os.WriteFile(path, []byte(`{"version":1}`), 0o600)
	b2 := NewDriftBaseline(path)
	b2.Record("nginx", "x", []byte("c"))
	if snap := b2.snapshot(); len(snap) != 1 {
		t.Errorf("after record snapshot = %v", snap)
	}
	// 条目哈希可回读
	b3 := NewDriftBaseline(path)
	b3.Forget("nginx", "x")
	if snap := b3.snapshot(); len(snap) != 0 {
		t.Errorf("after forget snapshot = %v", snap)
	}
}

func TestCovDriftBaselineSaveErrors(t *testing.T) {
	// MkdirAll 失败：路径中段是普通文件
	middle := filepath.Join(t.TempDir(), "blocker")
	os.WriteFile(middle, []byte("x"), 0o644)
	b := NewDriftBaseline(filepath.Join(middle, "sub", "baseline.json"))
	if err := b.update("k", BaselineEntry{SHA256: "v"}); err == nil ||
		!strings.Contains(err.Error(), "create baseline dir") {
		t.Errorf("mkdir err = %v", err)
	}

	if os.Geteuid() != 0 {
		// WriteFile 失败：目录只读（非 root）
		dir := t.TempDir()
		os.Chmod(dir, 0o500)
		t.Cleanup(func() { os.Chmod(dir, 0o700) })
		b2 := NewDriftBaseline(filepath.Join(dir, "baseline.json"))
		if err := b2.update("k", BaselineEntry{}); err == nil ||
			!strings.Contains(err.Error(), "write baseline temp") {
			t.Errorf("write err = %v", err)
		}
	}

	// Rename 失败：目标路径已是目录
	base := t.TempDir()
	os.Mkdir(filepath.Join(base, "baseline.json"), 0o700)
	b3 := NewDriftBaseline(filepath.Join(base, "baseline.json"))
	if err := b3.update("k", BaselineEntry{}); err == nil ||
		!strings.Contains(err.Error(), "rename baseline") {
		t.Errorf("rename err = %v", err)
	}
	// 失败后临时文件被清理
	if _, err := os.Stat(filepath.Join(base, "baseline.json.tmp")); !os.IsNotExist(err) {
		t.Error("temp file should be removed after rename failure")
	}
}

func TestCovNewDriftProviderEnvAndDefaults(t *testing.T) {
	t.Setenv("COCKPIT_NGINX_CONF_DIR", "/env/conf.d")
	t.Setenv("COCKPIT_STACKS_DIR", "/env/stacks")
	p := NewDriftProvider(nil, DriftConfig{})
	if p.confDir != "/env/conf.d" || p.stacksDir != "/env/stacks" {
		t.Errorf("env dirs = %q %q", p.confDir, p.stacksDir)
	}
	t.Setenv("COCKPIT_NGINX_CONF_DIR", "")
	t.Setenv("COCKPIT_STACKS_DIR", "")
	p2 := NewDriftProvider(nil, DriftConfig{})
	if p2.confDir != "/etc/nginx/conf.d" || p2.stacksDir != "/var/lib/cockpit/stacks" {
		t.Errorf("default dirs = %q %q", p2.confDir, p2.stacksDir)
	}
	if p2.Type() != "drift" {
		t.Errorf("type = %q", p2.Type())
	}
	// SetCronRunner 注入默认执行器
	p2.SetCronRunner()
	if p2.cronRun == nil {
		t.Error("SetCronRunner should install defaultCommander")
	}
	// unknown action
	if _, err := p2.Call("bogus", nil); err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("unknown action err = %v", err)
	}
}

// covDriftCronRun 构造可编程的 cron Commander
func covDriftCronRun(out, stderr []byte, err error) Commander {
	return func(context.Context, string, ...string) ([]byte, []byte, error) {
		return out, stderr, err
	}
}

func TestCovDriftCheckErrorAndEdgeEntries(t *testing.T) {
	base := filepath.Join(t.TempDir(), "bl")
	os.MkdirAll(base, 0o700)
	b := NewDriftBaseline(filepath.Join(base, "baseline.json"))

	confDir := t.TempDir()
	stacksDir := t.TempDir()
	// nginx 片段是目录 → error 条目
	os.Mkdir(filepath.Join(confDir, "cockpit-site-broken.conf"), 0o700)
	// stack 目录：compose.yml 是目录（ReadFile 失败跳过）、.env 正常
	os.MkdirAll(filepath.Join(stacksDir, "app", "compose.yml"), 0o700)
	os.WriteFile(filepath.Join(stacksDir, "app", ".env"), []byte("K=1"), 0o644)
	// stackReservedDir 与普通文件都跳过
	os.MkdirAll(filepath.Join(stacksDir, stackReservedDir), 0o700)
	os.WriteFile(filepath.Join(stacksDir, "plainfile"), []byte("x"), 0o644)

	// cron 读取失败 → error 条目
	badCron := covDriftCronRun(nil, []byte("permission denied"), errors.New("exit 1"))
	p := NewDriftProvider(b, DriftConfig{
		ConfDir: confDir, StacksDir: stacksDir, CronRun: badCron,
	})
	// 预置 cron 基线：missing 循环里 cron/cockpit 被跳过
	b.Record("cron", "cockpit", []byte("z"))
	res := mustCheck(t, p)
	if it := driftFind(t, res, "nginx", "broken"); it.Status != "error" {
		t.Errorf("nginx broken = %+v", it)
	}
	if it := driftFind(t, res, "cron", "cockpit"); it.Status != "error" {
		t.Errorf("cron error = %+v", it)
	}
	if it := driftFind(t, res, "stack", "app/.env"); it.Status != "no_baseline" {
		t.Errorf("app/.env = %+v", it)
	}
}

func TestCovDriftCheckCronNoneAndMissing(t *testing.T) {
	base := filepath.Join(t.TempDir(), "bl")
	os.MkdirAll(base, 0o700)
	b := NewDriftBaseline(filepath.Join(base, "baseline.json"))
	p := NewDriftProvider(b, DriftConfig{
		ConfDir:   t.TempDir(),
		StacksDir: t.TempDir(),
		// crontab -l 返回 no crontab for → 空表
		CronRun: covDriftCronRun(nil, []byte("no crontab for cui"), errors.New("no crontab for cui")),
	})
	res := mustCheck(t, p)
	// 空表无基线：jobs 为 nil slice，marshal 得 "null" ≠ "[]"，走 no_baseline
	if it := driftFind(t, res, "cron", "cockpit"); it.Status != "no_baseline" {
		t.Errorf("cron empty = %+v", it)
	}
	// compare 直调覆盖 "none" 分支（current 恰为 "[]" 且无基线）
	if it := p.compare("cron", "cockpit", []byte("[]"), map[string]bool{}); it.Status != "none" {
		t.Errorf("compare none = %+v", it)
	}

	// 基线里有 nginx/stack 条目但磁盘上没有 → missing；cron 条目跳过
	b.Record("nginx", "gone", []byte("a"))
	b.Record("stack", "app/compose.yml", []byte("b"))
	b.Record("cron", "cockpit", []byte("c"))
	res = mustCheck(t, p)
	if it := driftFind(t, res, "nginx", "gone"); it.Status != "missing" || it.BaselineSHA == "" {
		t.Errorf("nginx missing = %+v", it)
	}
	if it := driftFind(t, res, "stack", "app/compose.yml"); it.Status != "missing" {
		t.Errorf("stack missing = %+v", it)
	}
}

func TestCovDriftReadCrontabErrors(t *testing.T) {
	// 其他错误 → 包装返回
	p := NewDriftProvider(nil, DriftConfig{
		CronRun: covDriftCronRun(nil, []byte("boom"), errors.New("exit 2")),
	})
	if _, err := p.readCrontab(); err == nil || !strings.Contains(err.Error(), "crontab -l") {
		t.Errorf("readCrontab err = %v", err)
	}
	// 成功路径
	p2 := NewDriftProvider(nil, DriftConfig{
		CronRun: covDriftCronRun([]byte("* * * * * x\n"), nil, nil),
	})
	if out, err := p2.readCrontab(); err != nil || out != "* * * * * x\n" {
		t.Errorf("readCrontab ok = %q %v", out, err)
	}
}
