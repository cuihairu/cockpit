package probeagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeCfg(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "probe.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestLoadConfigValid 合法配置：缺省回填（全局→target）、字面量 duration、
// expect_status 缺省 200。
func TestLoadConfigValid(t *testing.T) {
	p := writeCfg(t, `
interval: 10s
timeout: 3s
threshold: 2
targets:
  - name: h
    type: http
    url: http://127.0.0.1:1/h
  - name: p
    type: tcp
    addr: 127.0.0.1:1
    interval: 5s
    timeout: 1s
    threshold: 4
    recovery: 3
    expect_status: 204
`)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interval.D() != 10_000_000_000 || cfg.Timeout.D() != 3_000_000_000 {
		t.Fatalf("global defaults = %v/%v", cfg.Interval, cfg.Timeout)
	}
	h, tcp := cfg.Targets[0], cfg.Targets[1]
	if h.Interval.D() != 10_000_000_000 || h.Timeout.D() != 3_000_000_000 ||
		h.Threshold != 2 || h.Recovery != DefaultRecovery || h.ExpectStatus != 200 {
		t.Fatalf("http target defaults = %+v", h)
	}
	if tcp.Interval.D() != 5_000_000_000 || tcp.Timeout.D() != 1_000_000_000 ||
		tcp.Threshold != 4 || tcp.Recovery != 3 || tcp.ExpectStatus != 204 {
		t.Fatalf("tcp target overrides = %+v", tcp)
	}
}

// TestLoadConfigErrors 非法配置矩阵：结构/字段/类型/引用全覆盖。
func TestLoadConfigErrors(t *testing.T) {
	cases := map[string]string{
		"missing file":   "",
		"no targets":     "targets: []\n",
		"unknown field":  "targets:\n  - name: x\n    type: tcp\n    addr: 1.2.3.4:80\n    bogus: 1\n",
		"missing name":   "targets:\n  - type: tcp\n    addr: 1.2.3.4:80\n",
		"duplicate name": "targets:\n  - name: x\n    type: tcp\n    addr: 1.2.3.4:80\n  - name: x\n    type: tcp\n    addr: 1.2.3.5:80\n",
		"bad type":       "targets:\n  - name: x\n    type: ftp\n",
		"http no url":    "targets:\n  - name: x\n    type: http\n",
		"tcp no addr":    "targets:\n  - name: x\n    type: tcp\n",
		"bad duration":   "interval: 30x\ntargets:\n  - name: x\n    type: tcp\n    addr: 1.2.3.4:80\n",
		"int duration":   "interval: 30\ntargets:\n  - name: x\n    type: tcp\n    addr: 1.2.3.4:80\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			var path string
			if name != "missing file" {
				path = writeCfg(t, content)
			} else {
				path = filepath.Join(t.TempDir(), "nope.yaml")
			}
			if _, err := LoadConfig(path); err == nil {
				t.Fatalf("%s: want error", name)
			} else if name == "bad duration" && !strings.Contains(err.Error(), "invalid duration") {
				t.Fatalf("bad duration err = %v", err)
			}
		})
	}
}

// TestDurationUnmarshalRejectsNonScalar 非标量节点触发 node.Decode(string)
// 错误分支（yaml.v3 把整数标量宽松解码进 string，整数走的是 ParseDuration
// 分支，故须用序列节点）。
func TestDurationUnmarshalRejectsNonScalar(t *testing.T) {
	var c Config
	err := yaml.Unmarshal([]byte("interval:\n  - 30\n"), &c)
	if err == nil || !strings.Contains(err.Error(), "duration must be string") {
		t.Fatalf("err = %v, want duration literal error", err)
	}
}
