package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// covWriteFile 测试辅助：写入文件（自动创建父目录）。
func covWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func covRun(args ...string) (int, string) {
	var buf bytes.Buffer
	code := run(append([]string{"cockpit"}, args...), &buf)
	return code, buf.String()
}

// --- loadConfig ---

func TestCovLoadConfigExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cockpit.yaml")
	covWriteFile(t, path, "server:\n  host: 10.1.2.3\n  port: 9999\n")

	var buf bytes.Buffer
	cfg, err := loadConfig(path, &buf)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Server == nil || cfg.Server.Host != "10.1.2.3" || cfg.Server.Port != 9999 {
		t.Fatalf("unexpected cfg: %+v", cfg.Server)
	}
}

func TestCovLoadConfigExplicitMissing(t *testing.T) {
	var buf bytes.Buffer
	if _, err := loadConfig(filepath.Join(t.TempDir(), "missing.yaml"), &buf); err == nil {
		t.Fatal("expected error for missing explicit path")
	}
}

func TestCovLoadConfigExplicitBadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	covWriteFile(t, path, "{{{{ not yaml")

	var buf bytes.Buffer
	if _, err := loadConfig(path, &buf); err == nil {
		t.Fatal("expected error for bad yaml")
	}
}

// 默认路径探测：目录里没有任何默认配置 → 走 LoadOrDefault
func TestCovLoadConfigDefaultNone(t *testing.T) {
	t.Chdir(t.TempDir())

	var buf bytes.Buffer
	cfg, err := loadConfig("", &buf)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !strings.Contains(buf.String(), "No config found, using defaults") {
		t.Fatalf("expected defaults hint, got %q", buf.String())
	}
	if cfg.Server == nil || cfg.Server.Host != "127.0.0.1" {
		t.Fatalf("expected default host 127.0.0.1, got %+v", cfg.Server)
	}
}

// 默认路径探测：./cockpit.yaml 存在 → 加载它
func TestCovLoadConfigDefaultFound(t *testing.T) {
	t.Chdir(t.TempDir())
	covWriteFile(t, "cockpit.yaml", "server:\n  host: 10.9.8.7\n")

	var buf bytes.Buffer
	cfg, err := loadConfig("", &buf)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Server == nil || cfg.Server.Host != "10.9.8.7" {
		t.Fatalf("expected host from ./cockpit.yaml, got %+v", cfg.Server)
	}
}

// 默认路径探测：优先级最高的 ./config/cockpit.yaml 存在但内容损坏 → 跳过，命中 ./cockpit.yaml
func TestCovLoadConfigDefaultBrokenSkipped(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	covWriteFile(t, filepath.Join(dir, "config", "cockpit.yaml"), "{{{{ broken")
	covWriteFile(t, filepath.Join(dir, "cockpit.yaml"), "server:\n  host: 10.0.0.5\n")

	var buf bytes.Buffer
	cfg, err := loadConfig("", &buf)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Server == nil || cfg.Server.Host != "10.0.0.5" {
		t.Fatalf("expected fallback to ./cockpit.yaml, got %+v", cfg.Server)
	}
	if strings.Contains(buf.String(), "No config found") {
		t.Fatalf("should not hit defaults, got %q", buf.String())
	}
}

// --- run 分发 ---

func TestCovRunVersionVariants(t *testing.T) {
	for _, arg := range []string{"version", "-v", "--version"} {
		code, out := covRun(arg)
		if code != 0 {
			t.Fatalf("run %s: code=%d", arg, code)
		}
		if !strings.Contains(out, "Cockpit v") {
			t.Fatalf("run %s: unexpected output %q", arg, out)
		}
	}
}

func TestCovRunUnknownCommand(t *testing.T) {
	code, out := covRun("bogus")
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
	if !strings.Contains(out, "Unknown command: bogus") || !strings.Contains(out, "Usage") {
		t.Fatalf("unexpected output %q", out)
	}
}

// 无命令直接跟 flag（default 分支 args[1][0]=='-'）→ 按 server 默认参数解析
func TestCovRunFlagDefaultVersion(t *testing.T) {
	code, out := covRun("-version")
	if code != 0 || !strings.Contains(out, "Cockpit v") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestCovRunFlagDefaultBadConfig(t *testing.T) {
	code, _ := covRun("-config", filepath.Join(t.TempDir(), "missing.yaml"))
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

// --- runServer ---

func TestCovRunServerHelp(t *testing.T) {
	code, out := covRun("server", "-h")
	if code != 0 || !strings.Contains(out, "Start Cockpit Server") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestCovRunServerBadConfig(t *testing.T) {
	code, _ := covRun("server", "-config", filepath.Join(t.TempDir(), "missing.yaml"))
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

// --- runAgent ---

func TestCovRunAgentHelp(t *testing.T) {
	code, out := covRun("agent", "-h")
	if code != 0 || !strings.Contains(out, "Start Cockpit Agent") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestCovRunAgentValidateFail(t *testing.T) {
	code, out := covRun("agent")
	if code != 1 || !strings.Contains(out, "missing required -server flag") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

// `cockpit agent start ...` 兼容形式：start 前缀剥离后同样走校验
func TestCovRunAgentStartPrefixValidateFail(t *testing.T) {
	code, out := covRun("agent", "start")
	if code != 1 || !strings.Contains(out, "missing required -server flag") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

// 指向必然拒绝连接的地址：agent.StartCmd.Run 返回 connect error → 退出码 1
func TestCovRunAgentConnectRefused(t *testing.T) {
	code, _ := covRun("agent", "-server", "ws://127.0.0.1:1/ws")
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

// --- runInit ---

func TestCovRunInitHelp(t *testing.T) {
	code, out := covRun("init", "-h")
	if code != 0 || !strings.Contains(out, "Initialize Cockpit configuration") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestCovRunInitSuccess(t *testing.T) {
	dir := t.TempDir()
	code, _ := covRun("init", "-dir", dir, "-example")
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "cockpit.yaml")); err != nil {
		t.Fatalf("config not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "inventory", "example.yaml")); err != nil {
		t.Fatalf("example inventory not created: %v", err)
	}

	// 重复 init：config 已存在分支（⊙ Config exists）
	code, _ = covRun("init", "-dir", dir)
	if code != 0 {
		t.Fatalf("re-init expected 0, got %d", code)
	}
}

// 目标是一个普通文件：MkdirAll 失败 → 退出码 1
func TestCovRunInitMkdirFail(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	covWriteFile(t, blocker, "x")

	code, _ := covRun("init", "-dir", filepath.Join(blocker, "sub"))
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

// --- runSync ---

func TestCovRunSyncHelp(t *testing.T) {
	code, out := covRun("sync", "-h")
	if code != 0 || !strings.Contains(out, "Sync inventory to database") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

// 缺 -inventory 且配置无 inventory.path → "inventory path required"
func TestCovRunSyncNoInventory(t *testing.T) {
	dir := t.TempDir()
	code, _ := covRun("sync", "-db", filepath.Join(dir, "test.db"))
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestCovRunSyncBadInventory(t *testing.T) {
	dir := t.TempDir()
	code, _ := covRun("sync", "-inventory", filepath.Join(dir, "missing.yaml"), "-db", filepath.Join(dir, "test.db"))
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

// 真实同步 examples/inventory.yaml 到临时库
func TestCovRunSyncSuccess(t *testing.T) {
	dir := t.TempDir()
	invPath := filepath.Join("..", "..", "examples", "inventory.yaml")
	if _, err := os.Stat(invPath); err != nil {
		t.Skipf("examples/inventory.yaml not available: %v", err)
	}

	code, _ := covRun("sync", "-inventory", invPath, "-db", filepath.Join(dir, "test.db"))
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "test.db")); err != nil {
		t.Fatalf("db not created: %v", err)
	}
}

// --- runStatus ---

func TestCovRunStatusHelp(t *testing.T) {
	code, out := covRun("status", "-h")
	if code != 0 || !strings.Contains(out, "Show Cockpit status") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

// 空库（storage.Open 自动建库）→ 正常输出摘要，退出码 0
func TestCovRunStatusEmptyDB(t *testing.T) {
	code, _ := covRun("status", "-db", filepath.Join(t.TempDir(), "status.db"))
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}

// 指向无法建库的路径 → 退出码 1
func TestCovRunStatusBadDB(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	covWriteFile(t, blocker, "x")

	code, _ := covRun("status", "-db", filepath.Join(blocker, "sub", "s.db"))
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

// --- printUsage/printVersion 直测 ---

func TestCovPrintHelpers(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)
	if !strings.Contains(buf.String(), "cockpit [command]") {
		t.Fatalf("usage output: %q", buf.String())
	}

	buf.Reset()
	printVersion(&buf)
	if !strings.Contains(buf.String(), version) {
		t.Fatalf("version output: %q", buf.String())
	}
}
