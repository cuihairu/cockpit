package rpc

// 覆盖率补充测试：logs_provider.go 错误分支与探测路径。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// covLogsRunner 可注入 systemctl / docker ps 失败的 runner
type covLogsRunner struct {
	systemctlErr error
	dockerPsErr  error
}

func (m *covLogsRunner) run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	switch {
	case name == "systemctl":
		if m.systemctlErr != nil {
			return nil, []byte(m.systemctlErr.Error()), m.systemctlErr
		}
		return []byte("nginx.service loaded active running\n\nssh.service loaded active running\n"), nil, nil
	case name == "docker" && len(args) > 0 && args[0] == "ps":
		if m.dockerPsErr != nil {
			return nil, []byte(m.dockerPsErr.Error()), m.dockerPsErr
		}
		return []byte("web-1\ndaemon\n"), nil, nil
	case name == "journalctl":
		return []byte("journal line\n"), nil, nil
	case name == "docker" && len(args) > 0 && args[0] == "logs":
		return []byte("docker line\n"), nil, nil
	}
	return nil, nil, nil
}

func covLogsDetect(j, d bool) func() (bool, bool) {
	return func() (bool, bool) { return j, d }
}

func TestCovLogsNewDefaultsAndType(t *testing.T) {
	p := NewLogsProvider(nil)
	if p.run == nil {
		t.Error("nil runner should default to defaultCommander")
	}
	if p.Type() != "logs" {
		t.Errorf("type = %q", p.Type())
	}
}

func TestCovLogsCallParamErrors(t *testing.T) {
	p := NewLogsProvider((&covLogsRunner{}).run)
	// query 缺对象
	if _, err := p.Call("query", nil); err == nil || !strings.Contains(err.Error(), "query object required") {
		t.Errorf("missing query err = %v", err)
	}
	// 坏 payload：tail 为字符串
	if _, err := p.Call("query", map[string]interface{}{
		"query": map[string]interface{}{"type": "systemd", "source": "nginx.service", "tail": "abc"},
	}); err == nil || !strings.Contains(err.Error(), "bad query payload") {
		t.Errorf("bad query err = %v", err)
	}
	// unknown action
	if _, err := p.Call("bogus", nil); err == nil || !strings.Contains(err.Error(), "unknown logs action") {
		t.Errorf("unknown action err = %v", err)
	}
}

func TestCovLogsDetectFunctions(t *testing.T) {
	// 假 journalctl + docker → 双 true → LogsAvailable true
	bin := t.TempDir()
	for _, name := range []string{"journalctl", "docker"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	j, d := DetectLogs()
	if !j || !d || !LogsAvailable() {
		t.Errorf("detect = %v %v", j, d)
	}
	// 空目录 → 双 false → LogsAvailable false
	t.Setenv("PATH", t.TempDir())
	if j, d := DetectLogs(); j || d {
		t.Errorf("detect empty = %v %v", j, d)
	}
	if LogsAvailable() {
		t.Error("LogsAvailable should be false with no sources")
	}
}

func TestCovLogsStatusAndSources(t *testing.T) {
	p := NewLogsProvider((&covLogsRunner{}).run)
	p.detect = covLogsDetect(true, true)
	raw, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	m := raw.(map[string]interface{})
	if m["journalctl"] != true || m["docker"] != true {
		t.Errorf("status = %v", m)
	}
	res, err := p.Call("sources", nil)
	if err != nil {
		t.Fatalf("sources: %v", err)
	}
	sm := res.(map[string]interface{})
	if len(sm["systemd"].([]string)) != 2 || len(sm["docker"].([]string)) != 2 {
		t.Errorf("sources = %v", sm)
	}

	// 双探测关 → 空列表
	p.detect = covLogsDetect(false, false)
	res, err = p.Call("sources", nil)
	if err != nil {
		t.Fatalf("sources none: %v", err)
	}
	sm = res.(map[string]interface{})
	if len(sm["systemd"].([]string)) != 0 || len(sm["docker"].([]string)) != 0 {
		t.Errorf("empty sources = %v", sm)
	}
}

func TestCovLogsSourcesErrors(t *testing.T) {
	// systemctl 失败
	p := NewLogsProvider((&covLogsRunner{systemctlErr: errors.New("systemd down")}).run)
	p.detect = covLogsDetect(true, false)
	if _, err := p.Sources(); err == nil || !strings.Contains(err.Error(), "systemctl list-units") {
		t.Errorf("systemctl err = %v", err)
	}
	// docker ps 失败
	p2 := NewLogsProvider((&covLogsRunner{dockerPsErr: errors.New("daemon down")}).run)
	p2.detect = covLogsDetect(false, true)
	if _, err := p2.Sources(); err == nil || !strings.Contains(err.Error(), "docker ps") {
		t.Errorf("docker ps err = %v", err)
	}
}

func TestCovLogsExecQueryErrorBranches(t *testing.T) {
	// journalctl 失败
	p := NewLogsProvider(func(context.Context, string, ...string) ([]byte, []byte, error) {
		return nil, []byte("journal offline"), errors.New("exit 1")
	})
	if _, err := p.Query(&LogsQuery{Type: "systemd", Source: "nginx.service", Tail: 10}); err == nil ||
		!strings.Contains(err.Error(), "journalctl") {
		t.Errorf("journalctl err = %v", err)
	}
	// docker logs 失败且无 stdout
	p2 := NewLogsProvider(func(context.Context, string, ...string) ([]byte, []byte, error) {
		if len([]byte("")) == 0 {
			return nil, []byte("no such container"), errors.New("exit 125")
		}
		return nil, nil, nil
	})
	if _, err := p2.Query(&LogsQuery{Type: "docker", Source: "web-1", Tail: 10}); err == nil ||
		!strings.Contains(err.Error(), "docker logs") {
		t.Errorf("docker logs err = %v", err)
	}
}

func TestCovLogsQuerySinceAndTruncation(t *testing.T) {
	// since 参数构造（systemd 与 docker 两路）
	var seen [][]string
	p := NewLogsProvider(func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		if name == "journalctl" || name == "docker" {
			full := append([]string{name}, args...)
			seen = append(seen, full)
		}
		return []byte("ok\n"), nil, nil
	})
	q := &LogsQuery{Type: "systemd", Source: "nginx.service", Tail: 50, SinceMinutes: 30}
	if _, err := p.Query(q); err != nil {
		t.Fatalf("systemd query: %v", err)
	}
	if !containsArg(seen[0], "--since=-30min") {
		t.Errorf("systemd args = %v", seen[0])
	}
	q2 := &LogsQuery{Type: "docker", Source: "web-1", Tail: 50, SinceMinutes: 30}
	if _, err := p.Query(q2); err != nil {
		t.Fatalf("docker query: %v", err)
	}
	if !containsArg(seen[1], "--since") || !containsArg(seen[1], "30m") {
		t.Errorf("docker args = %v", seen[1])
	}

	// 输出超 1MB → 截断 + truncated 标记
	big := strings.Repeat("line-of-log\n", 100000) // ~1.2MB
	p3 := NewLogsProvider(func(context.Context, string, ...string) ([]byte, []byte, error) {
		return []byte(big), nil, nil
	})
	res, err := p3.Query(&LogsQuery{Type: "systemd", Source: "nginx.service", Tail: 2000})
	if err != nil {
		t.Fatalf("big query: %v", err)
	}
	m := res.(map[string]interface{})
	if m["truncated"] != true {
		t.Errorf("truncated = %v", m["truncated"])
	}
	if n := len(m["lines"].(string)); n > logsMaxBytes {
		t.Errorf("lines len = %d", n)
	}
}

func containsArg(argv []string, want string) bool {
	for _, a := range argv {
		if a == want {
			return true
		}
	}
	return false
}
