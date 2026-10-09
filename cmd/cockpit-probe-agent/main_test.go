package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/core/healthprobe"
	"github.com/cuihairu/cockpit/core/platform"
)

// TestMain 子进程覆盖入口：守卫环境变量命中时以注入参数调 main()，覆盖
// 进程内不可达的 main() 本体（os.Exit 出口）；子进程计数经继承的
// GOCOVERDIR 回父进程 profile（同 cmd/cockpit-agent 先例）。
func TestMain(m *testing.M) {
	if os.Getenv("COCKPIT_PROBE_MAIN_SUBPROC") == "once" {
		os.Args = append([]string{"cockpit-probe-agent"},
			strings.Fields(os.Getenv("COCKPIT_PROBE_MAIN_ARGS"))...)
		main() // 内部 os.Exit，不返回
	}
	os.Exit(m.Run())
}

// TestCovProbeMainEntry 子进程 -once 全链：真进程跑探测定性 + 状态文件，
// 覆盖 main() 入口块。
func TestCovProbeMainEntry(t *testing.T) {
	if os.Getenv("COCKPIT_PROBE_MAIN_SUBPROC") != "" {
		t.Skip("subprocess child")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")
	cfgYaml := "threshold: 1\nrecovery: 1\ntargets:\n" +
		"  - name: live-http\n    type: http\n    url: " + srv.URL + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfgYaml), 0o644); err != nil {
		t.Fatal(err)
	}
	statusPath := filepath.Join(dir, "status.json")

	cmd := exec.Command(os.Args[0], "-test.run=^$", "-test.timeout=60s")
	cmd.Env = append(os.Environ(),
		"COCKPIT_PROBE_MAIN_SUBPROC=once",
		"COCKPIT_PROBE_MAIN_ARGS=-config "+cfgPath+" -status-file "+statusPath+" -once",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("subprocess: %v out: %s", err, out)
	}
	b, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	var snaps []healthprobe.Snapshot
	if err := json.Unmarshal(b, &snaps); err != nil || len(snaps) != 1 || snaps[0].State != healthprobe.StateHealthy {
		t.Fatalf("subprocess status = %v, err = %v", snaps, err)
	}
}

// TestRunVersion 版本出口。
func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"-version"}, &out, &out); code != 0 || !strings.Contains(out.String(), "cockpit-probe-agent v") {
		t.Fatalf("code=%d out=%q", code, out.String())
	}
}

// TestRunMissingConfig 配置不可读 → 1。
func TestRunMissingConfig(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"-config", filepath.Join(t.TempDir(), "nope.yaml")}, &out, &out); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
}

// TestRunOnceAndStatusFile -once 全链：存活 http + 必败 tcp 各探一次，
// 状态文件 JSON 可回读且状态符合（tcp threshold=1 → faulty 开窗）。
func TestRunOnceAndStatusFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")
	cfgYaml := "threshold: 1\nrecovery: 1\ntargets:\n" +
		"  - name: live-http\n    type: http\n    url: " + srv.URL + "\n" +
		"  - name: dead-tcp\n    type: tcp\n    addr: 127.0.0.1:1\n"
	if err := os.WriteFile(cfgPath, []byte(cfgYaml), 0o644); err != nil {
		t.Fatal(err)
	}
	statusPath := filepath.Join(dir, "status.json")

	var out bytes.Buffer
	code := run([]string{"-config", cfgPath, "-status-file", statusPath, "-once"}, &out, &out)
	if code != 0 {
		t.Fatalf("code = %d, out = %q", code, out.String())
	}

	b, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	var snaps []healthprobe.Snapshot
	if err := json.Unmarshal(b, &snaps); err != nil {
		t.Fatalf("read back: %v", err)
	}
	states := map[string]healthprobe.Snapshot{}
	for _, s := range snaps {
		states[s.Target] = s
	}
	if states["live-http"].State != healthprobe.StateHealthy {
		t.Fatalf("live-http = %+v", states["live-http"])
	}
	d := states["dead-tcp"]
	if d.State != healthprobe.StateFaulty || d.Open == nil || d.LastError == "" {
		t.Fatalf("dead-tcp = %+v, want faulty with open window", d)
	}
}

// TestDefaultConfigPath 缺省路径挂 platform Paths 契约（linux 测试二进制
// 经 select 装配 → /etc/cockpit-agent/probe.yaml）。
func TestDefaultConfigPath(t *testing.T) {
	p := defaultConfigPath()
	if !strings.HasSuffix(p, "probe.yaml") || p == "probe.yaml" {
		t.Fatalf("defaultConfigPath = %q, want platform ConfigDir/probe.yaml", p)
	}
}

// TestGracefulSignals 平台信号集非空（nil Current 兜底分支由平台装配
// 保证不触发，此处断言生产路径）。
func TestGracefulSignals(t *testing.T) {
	if len(gracefulSignals()) == 0 {
		t.Fatal("gracefulSignals empty")
	}
}

// fakeHost 空 Signals/Paths 平台桩（触发调用方兜底分支）。
type fakeHost struct{}

func (fakeHost) MachineID() string     { return "" }
func (fakeHost) Paths() platform.Paths { return platform.Paths{} }
func (fakeHost) Signals() []os.Signal  { return nil }

// TestGracefulSignalsFallback Current 为 nil 或信号集为空 → unix 兜底全集；
// ConfigDir 为空 → 裸配置名兜底。
func TestGracefulSignalsFallback(t *testing.T) {
	orig := platform.Current()
	defer platform.Register(orig)

	platform.Register(nil)
	if got := gracefulSignals(); len(got) != 2 {
		t.Fatalf("nil host signals = %v, want SIGTERM+Interrupt", got)
	}
	if got := defaultConfigPath(); got != "probe.yaml" {
		t.Fatalf("nil host config path = %q, want bare fallback", got)
	}
	platform.Register(fakeHost{})
	if got := gracefulSignals(); len(got) != 2 {
		t.Fatalf("empty signals host → %v, want fallback", got)
	}
	if got := defaultConfigPath(); got != "probe.yaml" {
		t.Fatalf("empty ConfigDir → %q, want bare fallback", got)
	}
}

// TestRunFlagParseError 未知 flag → 2。
func TestRunFlagParseError(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"-bogus"}, &out, &out); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

// TestRunServerNote server 字段非空打 B5 降级提示，本地模式照常探测退出 0。
func TestRunServerNote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfgPath := filepath.Join(t.TempDir(), "probe.yaml")
	cfgYaml := "server: 127.0.0.1:9999\nthreshold: 1\nrecovery: 1\ntargets:\n" +
		"  - name: live-http\n    type: http\n    url: " + srv.URL + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfgYaml), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"-config", cfgPath, "-once"}, &out, &errOut); code != 0 {
		t.Fatalf("code = %d, err = %q", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "not wired yet") {
		t.Fatalf("stderr = %q, want downgrade note", errOut.String())
	}
}

// TestRunGracefulShutdown 常驻模式全链：启动写状态 → SIGTERM 优雅退出
// （退出码 0，终态落盘可回读）。
func TestRunGracefulShutdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")
	cfgYaml := "interval: 50ms\nthreshold: 1\nrecovery: 1\ntargets:\n" +
		"  - name: live-http\n    type: http\n    url: " + srv.URL + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfgYaml), 0o644); err != nil {
		t.Fatal(err)
	}
	statusPath := filepath.Join(dir, "status.json")

	done := make(chan int, 1)
	go func() {
		done <- run([]string{"-config", cfgPath, "-status-file", statusPath}, io.Discard, io.Discard)
	}()

	// 轮询至首份状态落盘（此时信号 handler 已注册——注册先于状态写出）
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(statusPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("status file never written")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("code = %d, want 0 on graceful stop", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return on SIGTERM")
	}

	b, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	var snaps []healthprobe.Snapshot
	if err := json.Unmarshal(b, &snaps); err != nil || len(snaps) != 1 || snaps[0].State != healthprobe.StateHealthy {
		t.Fatalf("final status = %v, err = %v, want healthy", snaps, err)
	}
}
