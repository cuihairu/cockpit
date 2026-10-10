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
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
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

func (fakeHost) Services() platform.ServiceManager { return nil }

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

// TestRunOnceIgnoresServer -once + server 配置：不装配上行链，纯本地探测
// 退出 0（快照模式契约）。
func TestRunOnceIgnoresServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfgPath := filepath.Join(t.TempDir(), "probe.yaml")
	cfgYaml := "server: ws://127.0.0.1:1/ws\nthreshold: 1\nrecovery: 1\ntargets:\n" +
		"  - name: live-http\n    type: http\n    url: " + srv.URL + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfgYaml), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"-config", cfgPath, "-once"}, &out, &errOut); code != 0 {
		t.Fatalf("code = %d, err = %q", code, errOut.String())
	}
}

// probeUpstreamStub 命令级 e2e 桩：收注册回 accepted，后续消息转发给用例。
type probeUpstreamStub struct {
	upgrader websocket.Upgrader
	regCh    chan struct{}
	msgs     chan *protocol.Message
}

func newProbeUpstreamStub() *probeUpstreamStub {
	return &probeUpstreamStub{
		regCh: make(chan struct{}, 4),
		msgs:  make(chan *protocol.Message, 16),
	}
}

func (s *probeUpstreamStub) handler(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	codec := protocol.NewCodec()
	msg, err := codec.ReadMessage(conn)
	if err != nil || msg.Type != protocol.MessageTypeRegister {
		return
	}
	s.regCh <- struct{}{}
	_ = codec.WriteMessage(conn, protocol.NewMessage(protocol.MessageTypeRegister, map[string]interface{}{
		"status":     "accepted",
		"serverTime": time.Now().Unix(),
	}))
	for {
		m, err := codec.ReadMessage(conn)
		if err != nil {
			return
		}
		select {
		case s.msgs <- m:
		default:
		}
	}
}

// TestRunDaemonUpstreamE2E 常驻模式命令级全链：装配上行链 → 注册 → 首轮
// 定性迁移触发 probe_report 上行 → SIGTERM 优雅退出 0。
func TestRunDaemonUpstreamE2E(t *testing.T) {
	stub := newProbeUpstreamStub()
	wsrv := httptest.NewServer(http.HandlerFunc(stub.handler))
	defer wsrv.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()

	cfgPath := filepath.Join(t.TempDir(), "probe.yaml")
	cfgYaml := "server: ws" + strings.TrimPrefix(wsrv.URL, "http") + "/ws\n" +
		"interval: 50ms\nthreshold: 1\nrecovery: 1\ntargets:\n" +
		"  - name: live-http\n    type: http\n    url: " + target.URL + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfgYaml), 0o644); err != nil {
		t.Fatal(err)
	}

	done := make(chan int, 1)
	go func() {
		done <- run([]string{"-config", cfgPath}, io.Discard, io.Discard)
	}()

	select {
	case <-stub.regCh:
	case <-time.After(5 * time.Second):
		t.Fatal("register never arrived")
	}

	// 迁移即发：unknown→healthy 定性后 probe_report 应到达
	deadline := time.Now().Add(5 * time.Second)
	gotReport := false
	for time.Now().Before(deadline) && !gotReport {
		select {
		case m := <-stub.msgs:
			if m.Type == protocol.MessageTypeProbeReport {
				gotReport = true
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
	if !gotReport {
		t.Fatal("probe_report never arrived after transition")
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

	// 轮询至状态文件定性 healthy（首探完成、迁移落盘）再发 SIGTERM——只看
	// 文件存在会竞态在首探前发信号，终态落成 unknown（启动写初态为 unknown）
	deadline := time.Now().Add(5 * time.Second)
	for {
		b, err := os.ReadFile(statusPath)
		if err == nil && bytes.Contains(b, []byte("healthy")) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("status never became healthy: %s", string(b))
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
