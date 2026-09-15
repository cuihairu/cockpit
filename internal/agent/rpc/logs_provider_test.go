package rpc

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// mockLogsRunner 按 argv 首个参数分派：
//   - journalctl → journalOut（可注 journalErr）
//   - docker ps → psOut；docker logs → logsOut（可注 dockerErr）
//   - systemctl list-units → unitsOut
//   - 其余命令（LookPath 不经 runner）返回空
type mockLogsRunner struct {
	mu         sync.Mutex
	unitsOut   string
	psOut      string
	journalOut string
	logsOut    string
	journalErr error
	dockerErr  error
	lastArgs   []string
}

func (m *mockLogsRunner) run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastArgs = append([]string{name}, args...)
	switch name {
	case "systemctl":
		return []byte(m.unitsOut), nil, nil
	case "journalctl":
		if m.journalErr != nil {
			return nil, []byte(m.journalErr.Error()), m.journalErr
		}
		return []byte(m.journalOut), nil, nil
	case "docker":
		if len(args) > 0 && args[0] == "ps" {
			return []byte(m.psOut), nil, nil
		}
		if len(args) > 0 && args[0] == "logs" {
			if m.dockerErr != nil {
				// 停止容器场景：exit 非零但 stdout 有历史日志
				return []byte(m.logsOut), []byte(m.dockerErr.Error()), m.dockerErr
			}
			return []byte(m.logsOut), nil, nil
		}
	}
	return nil, nil, nil
}

func TestLogsQueryValidate(t *testing.T) {
	base := &LogsQuery{Type: "systemd", Source: "nginx.service", Tail: 200}
	if err := base.validate(); err != nil {
		t.Fatalf("base should pass: %v", err)
	}
	for _, c := range []struct {
		desc string
		mut  func(*LogsQuery)
	}{
		{"bad type", func(q *LogsQuery) { q.Type = "file" }},
		{"bad source", func(q *LogsQuery) { q.Source = "a; rm -rf" }},
		{"source with slash", func(q *LogsQuery) { q.Source = "../etc" }},
		{"tail too big", func(q *LogsQuery) { q.Tail = 5000 }},
		{"tail negative", func(q *LogsQuery) { q.Tail = -1 }},
		{"since too big", func(q *LogsQuery) { q.SinceMinutes = 9999 }},
		{"grep too long", func(q *LogsQuery) { q.Grep = strings.Repeat("x", 300) }},
		{"grep newline", func(q *LogsQuery) { q.Grep = "a\nb" }},
	} {
		q := *base
		c.mut(&q)
		if err := q.validate(); err == nil {
			t.Errorf("%s: should be rejected", c.desc)
		}
	}

	// tail=0 归一为默认值；docker 容器名带点/横线/下划线合法
	dockerQ := &LogsQuery{Type: "docker", Source: "web-1.proxy_2"}
	if err := dockerQ.validate(); err != nil {
		t.Fatalf("docker name should pass: %v", err)
	}
	if dockerQ.Tail != logsDefaultTail {
		t.Errorf("tail = %d, want default %d", dockerQ.Tail, logsDefaultTail)
	}
}

func TestLogsStatusAndSources(t *testing.T) {
	runner := &mockLogsRunner{
		unitsOut:   "nginx.service loaded active running web\nssh.service  loaded active running ssh\n\n",
		psOut:      "web-1\nproxy.2\n\n",
		journalOut: "x",
	}
	p := NewLogsProvider(runner.run)
	p.detect = func() (bool, bool) { return true, true }

	st, err := p.Call("status", nil)
	if err != nil {
		t.Fatal(err)
	}
	sm := st.(map[string]interface{})
	if sm["journalctl"] != true || sm["docker"] != true {
		t.Fatalf("status = %v", sm)
	}

	src, err := p.Call("sources", nil)
	if err != nil {
		t.Fatal(err)
	}
	sources := src.(map[string]interface{})
	units := sources["systemd"].([]string)
	if len(units) != 2 || units[0] != "nginx.service" {
		t.Fatalf("systemd units = %v", units)
	}
	names := sources["docker"].([]string)
	if len(names) != 2 || names[1] != "proxy.2" {
		t.Fatalf("docker names = %v", names)
	}
}

func TestLogsQuerySystemdArgs(t *testing.T) {
	runner := &mockLogsRunner{journalOut: "2026-09-15T10:00:00+08:00 host nginx[1]: ready\n"}
	p := NewLogsProvider(runner.run)

	res, err := p.Call("query", map[string]interface{}{
		"query": map[string]interface{}{
			"type": "systemd", "source": "nginx.service",
			"tail": 500, "since_minutes": 60, "grep": "ready",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]interface{})
	if m["truncated"] != false || !strings.Contains(m["lines"].(string), "ready") {
		t.Fatalf("result = %v", m)
	}

	// 命令参数：-u source / -n tail / short-iso / --since=-60min
	got := strings.Join(runner.lastArgs, " ")
	for _, want := range []string{"-u nginx.service", "-n 500", "short-iso", "--since=-60min", "--no-pager", "-q"} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q missing %q", got, want)
		}
	}
}

func TestLogsQueryDockerSinceAndStoppedContainer(t *testing.T) {
	// 停止容器：dockerErr 非零但 stdout 有历史日志 → 不当错误
	runner := &mockLogsRunner{logsOut: "2026-09-15T02:00:00.000000000Z old log\n", dockerErr: errors.New("container stopped")}
	p := NewLogsProvider(runner.run)

	res, err := p.Call("query", map[string]interface{}{
		"query": map[string]interface{}{
			"type": "docker", "source": "web-1", "tail": 100, "since_minutes": 30,
		},
	})
	if err != nil {
		t.Fatalf("stopped container with output should pass: %v", err)
	}
	if !strings.Contains(res.(map[string]interface{})["lines"].(string), "old log") {
		t.Errorf("history log lost: %v", res)
	}
	got := strings.Join(runner.lastArgs, " ")
	for _, want := range []string{"--tail 100", "--timestamps", "--since 30m", "web-1"} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q missing %q", got, want)
		}
	}

	// 完全失败且无输出 → 报错
	runner2 := &mockLogsRunner{dockerErr: errors.New("no such container")}
	p2 := NewLogsProvider(runner2.run)
	if _, err := p2.Call("query", map[string]interface{}{
		"query": map[string]interface{}{"type": "docker", "source": "ghost"},
	}); err == nil || !strings.Contains(err.Error(), "no such container") {
		t.Fatalf("err = %v, want docker failure surfaced", err)
	}
}

func TestLogsQueryGrepFilterAndTruncate(t *testing.T) {
	// grep 过滤：只留命中行
	multi := "line one\nerror happened\nline three\nanother error\n"
	runner := &mockLogsRunner{journalOut: multi}
	p := NewLogsProvider(runner.run)

	res, err := p.Call("query", map[string]interface{}{
		"query": map[string]interface{}{"type": "systemd", "source": "app.service", "grep": "error"},
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := res.(map[string]interface{})["lines"].(string)
	if strings.Contains(lines, "line one") || !strings.Contains(lines, "error happened") || !strings.Contains(lines, "another error") {
		t.Errorf("grep filter wrong: %q", lines)
	}

	// grep 无命中 → 空结果非错误
	res, err = p.Call("query", map[string]interface{}{
		"query": map[string]interface{}{"type": "systemd", "source": "app.service", "grep": "nomatch"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]interface{})["lines"] != "" {
		t.Errorf("no match should give empty lines, got %v", res)
	}

	// 超限截断：>1MB 截到行边界并标记 truncated
	big := strings.Repeat("x", 1024) + "\n"
	runner2 := &mockLogsRunner{journalOut: strings.Repeat(big, 1200)} // ~1.2MB
	p2 := NewLogsProvider(runner2.run)
	res, err = p2.Call("query", map[string]interface{}{
		"query": map[string]interface{}{"type": "systemd", "source": "big.service"},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]interface{})
	if m["truncated"] != true {
		t.Error("oversized output should be truncated")
	}
	if got := len(m["lines"].(string)); got > logsMaxBytes {
		t.Errorf("truncated size %d > limit %d", got, logsMaxBytes)
	}
	if !strings.HasSuffix(m["lines"].(string), "\n") {
		t.Error("truncation should cut at line boundary")
	}
}

func TestLogsQueryValidatesBeforeCommand(t *testing.T) {
	runner := &mockLogsRunner{}
	p := NewLogsProvider(runner.run)

	if _, err := p.Call("query", map[string]interface{}{
		"query": map[string]interface{}{"type": "file", "source": "x"},
	}); err == nil {
		t.Fatal("invalid type should be rejected")
	}
	if _, err := p.Call("query", map[string]interface{}{
		"query": map[string]interface{}{"source": "no-type"},
	}); err == nil {
		t.Fatal("missing type should be rejected")
	}
	// 校验失败不应执行命令
	if runner.lastArgs != nil {
		t.Errorf("command should not run on validation failure, got %v", runner.lastArgs)
	}
}

func TestLogsJournalFailureSurfacesError(t *testing.T) {
	runner := &mockLogsRunner{journalErr: errors.New("permission denied")}
	p := NewLogsProvider(runner.run)
	_, err := p.Call("query", map[string]interface{}{
		"query": map[string]interface{}{"type": "systemd", "source": "x.service"},
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("err = %v, want journal failure surfaced", err)
	}
}

// 单源主机：docker 不可用时 sources 只返回 systemd，docker 恒为空数组
func TestLogsSourcesSingleSource(t *testing.T) {
	runner := &mockLogsRunner{unitsOut: "app.service loaded active running x\n"}
	p := NewLogsProvider(runner.run)
	p.detect = func() (bool, bool) { return true, false }

	src, err := p.Call("sources", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := src.(map[string]interface{})
	if len(m["systemd"].([]string)) != 1 || len(m["docker"].([]string)) != 0 {
		t.Fatalf("sources = %v", m)
	}

	st, _ := p.Call("status", nil)
	sm := st.(map[string]interface{})
	if sm["docker"] != false {
		t.Errorf("status = %v, want docker=false", sm)
	}
}
