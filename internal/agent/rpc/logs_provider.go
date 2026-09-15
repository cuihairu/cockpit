package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ============ Logs Provider ============
//
// 远程日志查询器（见 logs-design.md）：查询式（pull 型），agent 不采集不存储，
// 每次查询实时执行 journalctl / docker logs。源名单白名单校验 + argv 直传
// 不经 shell（D5 纵深防御），输出经 grep 过滤与 1MB 截断。

const (
	// logsMaxBytes 单次查询输出上限（超限截断到最后一行边界）
	logsMaxBytes = 1 << 20
	// logsDefaultTail / logsMaxTail 默认与最大返回行数
	logsDefaultTail = 200
	logsMaxTail     = 2000
	// logsMaxSinceMinutes since 上限（1440 = 24h），0 表示不限
	logsMaxSinceMinutes = 1440
	// logsMaxGrep 关键词长度上限
	logsMaxGrep = 256
)

// logsCmdTimeout 单次查询/枚举命令超时
const logsCmdTimeout = 30 * time.Second

// logsSourceRe 日志源白名单：systemd unit 名（nginx.service / user@1000.service）
// 与 docker 容器名（web-1）；不含斜杠/空白/shell 元字符
var logsSourceRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.@+-]{0,127}$`)

// LogsQuery 一次日志查询的声明
type LogsQuery struct {
	Type         string `json:"type"` // systemd / docker
	Source       string `json:"source"`
	Tail         int    `json:"tail"`
	SinceMinutes int    `json:"since_minutes"`
	Grep         string `json:"grep"`
}

// validate 查询参数校验（server 侧同规则，双端防御）
func (q *LogsQuery) validate() error {
	if q.Type != "systemd" && q.Type != "docker" {
		return fmt.Errorf("type must be systemd or docker, got %q", q.Type)
	}
	if !logsSourceRe.MatchString(q.Source) {
		return fmt.Errorf("invalid source name %q", q.Source)
	}
	if q.Tail == 0 {
		q.Tail = logsDefaultTail
	}
	if q.Tail < 1 || q.Tail > logsMaxTail {
		return fmt.Errorf("tail out of range [1, %d]", logsMaxTail)
	}
	if q.SinceMinutes < 0 || q.SinceMinutes > logsMaxSinceMinutes {
		return fmt.Errorf("since_minutes out of range [0, %d]", logsMaxSinceMinutes)
	}
	if len(q.Grep) > logsMaxGrep {
		return fmt.Errorf("grep too long (max %d bytes)", logsMaxGrep)
	}
	if strings.ContainsAny(q.Grep, "\r\n") {
		return fmt.Errorf("grep must not contain newlines")
	}
	return nil
}

// LogsProvider 日志查询 Provider
type LogsProvider struct {
	run Commander
	// detect 可注入的源探测（默认 DetectLogs 的 LookPath），测试用
	detect func() (journalctl, docker bool)
}

func NewLogsProvider(run Commander) *LogsProvider {
	if run == nil {
		run = defaultCommander
	}
	return &LogsProvider{run: run, detect: DetectLogs}
}

func (p *LogsProvider) Type() string { return "logs" }

func (p *LogsProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "status":
		return p.Status()
	case "sources":
		return p.Sources()
	case "query":
		raw, ok := params["query"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("query object required")
		}
		q, err := queryFromMap(raw)
		if err != nil {
			return nil, err
		}
		return p.Query(q)
	default:
		return nil, fmt.Errorf("unknown logs action: %s", action)
	}
}

// queryFromMap JSON 反序列化后的 map 形状 → LogsQuery（数字可能为 float64）
func queryFromMap(raw map[string]interface{}) (*LogsQuery, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("bad query payload: %w", err)
	}
	var q LogsQuery
	if err := json.Unmarshal(b, &q); err != nil {
		return nil, fmt.Errorf("bad query payload: %w", err)
	}
	return &q, nil
}

// DetectLogs 探测 journalctl 与 docker 可用性
func DetectLogs() (journalctl, docker bool) {
	_, err := exec.LookPath("journalctl")
	journalctl = err == nil
	_, err = exec.LookPath("docker")
	docker = err == nil
	return
}

// LogsAvailable 任一日志源可用（capability 探测用）
func LogsAvailable() bool {
	j, d := DetectLogs()
	return j || d
}

// ============ RPC 实现 ============

// Status 两类日志源可用性
func (p *LogsProvider) Status() (interface{}, error) {
	j, d := p.detect()
	return map[string]interface{}{"journalctl": j, "docker": d}, nil
}

// Sources 运行中的 systemd 服务与 docker 容器（D10：只列运行中对象）
func (p *LogsProvider) Sources() (interface{}, error) {
	result := map[string]interface{}{"systemd": []string{}, "docker": []string{}}

	j, d := p.detect()
	if j {
		units, err := p.listSystemdUnits()
		if err != nil {
			return nil, err
		}
		result["systemd"] = units
	}
	if d {
		names, err := p.listDockerContainers()
		if err != nil {
			return nil, err
		}
		result["docker"] = names
	}
	return result, nil
}

// listSystemdUnits systemctl list-units --type=service --no-legend --plain
func (p *LogsProvider) listSystemdUnits() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), logsCmdTimeout)
	defer cancel()
	out, stderr, err := p.run(ctx, "systemctl", "list-units", "--type=service", "--no-legend", "--plain")
	if err != nil {
		return nil, fmt.Errorf("systemctl list-units: %s", commandErrSummary(stderr, err))
	}
	return parseFirstColumn(string(out)), nil
}

// listDockerContainers docker ps --format '{{.Names}}'
func (p *LogsProvider) listDockerContainers() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), logsCmdTimeout)
	defer cancel()
	out, stderr, err := p.run(ctx, "docker", "ps", "--format", "{{.Names}}")
	if err != nil {
		return nil, fmt.Errorf("docker ps: %s", commandErrSummary(stderr, err))
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && logsSourceRe.MatchString(line) {
			names = append(names, line)
		}
	}
	return names, nil
}

// parseFirstColumn 逐行取第一列，跳过空行（systemctl --no-legend --plain 输出）
func parseFirstColumn(out string) []string {
	var items []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			items = append(items, fields[0])
		}
	}
	return items
}

// Query 执行日志查询：命令构造 → 输出读取 → grep 过滤 → 截断（D4/D5）
func (p *LogsProvider) Query(q *LogsQuery) (interface{}, error) {
	if err := q.validate(); err != nil {
		return nil, err
	}

	out, err := p.execQuery(q)
	if err != nil {
		return nil, err
	}

	lines := p.filter(string(out), q.Grep)
	truncated := false
	if len(lines) > logsMaxBytes {
		cut := lines[:logsMaxBytes]
		// 截断到最后一行边界（不含残行）
		if idx := strings.LastIndexByte(cut, '\n'); idx > 0 {
			cut = cut[:idx+1]
		}
		lines = cut
		truncated = true
	}
	return map[string]interface{}{"lines": lines, "truncated": truncated}, nil
}

// execQuery 按源类型构造并执行命令
func (p *LogsProvider) execQuery(q *LogsQuery) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), logsCmdTimeout)
	defer cancel()

	if q.Type == "systemd" {
		args := []string{"-u", q.Source, "-n", strconv.Itoa(q.Tail), "--no-pager", "-q", "-o", "short-iso"}
		if q.SinceMinutes > 0 {
			args = append(args, "--since=-"+strconv.Itoa(q.SinceMinutes)+"min")
		}
		out, stderr, err := p.run(ctx, "journalctl", args...)
		if err != nil {
			return nil, fmt.Errorf("journalctl: %s", commandErrSummary(stderr, err))
		}
		return out, nil
	}

	args := []string{"logs", "--tail", strconv.Itoa(q.Tail), "--timestamps"}
	if q.SinceMinutes > 0 {
		args = append(args, "--since", strconv.Itoa(q.SinceMinutes)+"m")
	}
	args = append(args, q.Source)
	out, stderr, err := p.run(ctx, "docker", args...)
	if err != nil {
		// 容器已停止时 docker logs 仍能输出历史日志（exit 非零但 stdout 有内容）；
		// 有输出就不当错误，避免查不到刚挂掉的容器
		if len(out) > 0 {
			return out, nil
		}
		return nil, fmt.Errorf("docker logs: %s", commandErrSummary(stderr, err))
	}
	return out, nil
}

// filter 逐行 contains 过滤；无关键词原样返回
func (p *LogsProvider) filter(text, grep string) string {
	if grep == "" {
		return text
	}
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, grep) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
