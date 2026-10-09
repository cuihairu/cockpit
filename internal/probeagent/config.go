// Package probeagent 服务检测 agent 业务插件（agent-core ③）：目标清单
// 调度 + 防抖状态机（core/healthprobe）+ 本地/上行输出面。三层纪律与
// 目标格式见 docs/design/service-detect-agent.md。
package probeagent

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration 支持 "30s"/"1m" 字面量的 YAML duration（yaml.v3 原生只认整数
// 纳秒，探针配置要人读字面量）。
type Duration time.Duration

// UnmarshalYAML 字面量解析。
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("duration must be string like 30s: %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(v)
	return nil
}

// D 还原 time.Duration。
func (d Duration) D() time.Duration { return time.Duration(d) }

// 缺省值（简档 §1 全局面）。
const (
	DefaultInterval  = 30 * time.Second
	DefaultTimeout   = 5 * time.Second
	DefaultThreshold = 3
	DefaultRecovery  = 2
)

// TargetConfig 单探测目标。target 级 Interval/Timeout/Threshold/Recovery
// 为零值时回退全局，全局为零值回退缺省。
type TargetConfig struct {
	Name         string   `yaml:"name"`
	Type         string   `yaml:"type"` // http|tcp
	URL          string   `yaml:"url"`
	Addr         string   `yaml:"addr"`
	ExpectStatus int      `yaml:"expect_status"`
	Interval     Duration `yaml:"interval"`
	Timeout      Duration `yaml:"timeout"`
	Threshold    int      `yaml:"threshold"`
	Recovery     int      `yaml:"recovery"`
}

// Config 探测 agent 配置文件（简档 §1 目标格式）。
type Config struct {
	Server    string         `yaml:"server"`
	Secret    string         `yaml:"secret"`
	Interval  Duration       `yaml:"interval"`
	Timeout   Duration       `yaml:"timeout"`
	Threshold int            `yaml:"threshold"`
	Recovery  int            `yaml:"recovery"`
	Targets   []TargetConfig `yaml:"targets"`
}

// LoadConfig 读入 YAML（未知字段报错）、结构校验并回填缺省——返回后每个
// target 的字段都可直接使用。
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.resolve(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// resolve 校验 + 缺省回填（先全局后 target）。
func (c *Config) resolve() error {
	if len(c.Targets) == 0 {
		return errors.New("no probe targets")
	}
	if c.Interval == 0 {
		c.Interval = Duration(DefaultInterval)
	}
	if c.Timeout == 0 {
		c.Timeout = Duration(DefaultTimeout)
	}
	if c.Threshold <= 0 {
		c.Threshold = DefaultThreshold
	}
	if c.Recovery <= 0 {
		c.Recovery = DefaultRecovery
	}

	seen := map[string]bool{}
	for i := range c.Targets {
		t := &c.Targets[i]
		if t.Name == "" {
			return errors.New("target missing name")
		}
		if seen[t.Name] {
			return fmt.Errorf("duplicate target name %q", t.Name)
		}
		seen[t.Name] = true
		switch t.Type {
		case "http":
			if t.URL == "" {
				return fmt.Errorf("target %q: http requires url", t.Name)
			}
			if t.ExpectStatus == 0 {
				t.ExpectStatus = 200
			}
		case "tcp":
			if t.Addr == "" {
				return fmt.Errorf("target %q: tcp requires addr", t.Name)
			}
		default:
			return fmt.Errorf("target %q: unsupported type %q", t.Name, t.Type)
		}
		if t.Interval == 0 {
			t.Interval = c.Interval
		}
		if t.Timeout == 0 {
			t.Timeout = c.Timeout
		}
		if t.Threshold <= 0 {
			t.Threshold = c.Threshold
		}
		if t.Recovery <= 0 {
			t.Recovery = c.Recovery
		}
	}
	return nil
}
