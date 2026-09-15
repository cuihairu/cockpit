package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ============ Cron Provider ============
//
// Crontab 任务可视化（见 cron-design.md）：cockpit 只管理自己名下的任务对
// （meta 注释行 + 命令行），写回 = 读全量 → 只替换 cockpit 段 → 其余行
// 逐行原样保留（D2），写回前自检非 cockpit 行未变。元数据内嵌在注释行里
// （D3），回读自己渲染的产物，无需解析 cron 表达式语义。

const (
	// cronMetaPrefix 任务对首行注释前缀
	cronMetaPrefix = "# cockpit:job "
	// cronMaxCommand 命令直通上限
	cronMaxCommand = 4 * 1024
)

var (
	// cronJobNameRe 任务名（proxyNameRe 同款）
	cronJobNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	// cronFieldRe 单字段片段：数字/星号 + 可选范围与步长
	cronFieldRe = regexp.MustCompile(`^(\*|[0-9]+)(-[0-9]+)?(/[0-9]+)?$`)
)

// @extensions crontab(5) 支持的简写（白名单）
var cronAtExtensions = map[string]bool{
	"@reboot": true, "@hourly": true, "@daily": true,
	"@weekly": true, "@monthly": true, "@yearly": true, "@annually": true,
}

// cronFieldRanges 五字段允许的数字范围（min, max）；dow 同时接受 7（视同 0）
var cronFieldRanges = [5][2]int{
	{0, 59}, // 分
	{0, 23}, // 时
	{1, 31}, // 日
	{1, 12}, // 月
	{0, 7},  // 周
}

// CronJob 一个定时任务的声明
type CronJob struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Enabled  bool   `json:"enabled"`
}

// validate 任务参数校验（server 侧同规则，双端防御）
func (j *CronJob) validate() error {
	if !cronJobNameRe.MatchString(j.Name) {
		return fmt.Errorf("invalid job name %q", j.Name)
	}
	if err := validateCronExpr(j.Schedule); err != nil {
		return err
	}
	if j.Command == "" {
		return fmt.Errorf("command is required")
	}
	if len(j.Command) > cronMaxCommand {
		return fmt.Errorf("command too large (max %d bytes)", cronMaxCommand)
	}
	if strings.ContainsAny(j.Command, "\r\n") {
		return fmt.Errorf("command must not contain newlines (crontab is line-based)")
	}
	return nil
}

// validateCronExpr cron 表达式校验：@ 白名单或 5 字段范围校验（D5）
func validateCronExpr(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("schedule is required")
	}
	if strings.HasPrefix(expr, "@") {
		if cronAtExtensions[expr] {
			return nil
		}
		return fmt.Errorf("unsupported @extension %q", expr)
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("schedule must have 5 fields (min hour dom mon dow), got %d", len(fields))
	}
	for i, f := range fields {
		lo, hi := cronFieldRanges[i][0], cronFieldRanges[i][1]
		for _, part := range strings.Split(f, ",") {
			// m[1]=基值或*，m[2]=可选范围（带-），m[3]=可选步长（带/）
			m := cronFieldRe.FindStringSubmatch(part)
			if m == nil {
				return fmt.Errorf("invalid schedule field %d: %q", i+1, part)
			}
			tokens := []string{}
			if m[1] != "*" {
				tokens = append(tokens, m[1])
			}
			if m[2] != "" {
				tokens = append(tokens, strings.TrimPrefix(m[2], "-"))
			}
			// 步长必须 ≥1（*/0 除零语义，cron 实现均拒绝）；范围部分照常校验
			if m[3] != "" {
				if sn, err := strconv.Atoi(strings.TrimPrefix(m[3], "/")); err != nil || sn < 1 || sn > hi {
					return fmt.Errorf("invalid step %q in schedule field %d", m[3], i+1)
				}
			}
			for _, tok := range tokens {
				n, err := strconv.Atoi(tok)
				if err != nil || n < lo || n > hi {
					return fmt.Errorf("schedule field %d value %s out of range [%d,%d]", i+1, tok, lo, hi)
				}
			}
		}
	}
	return nil
}

// CronProvider Crontab 管理 Provider
type CronProvider struct {
	mu       sync.Mutex // 读改写临界区串行化（D6）
	run      Commander
	baseline BaselineRecorder // 漂移基线挂钩（见 drift-design.md D7），nil 不记录
}

// SetBaseline 注入漂移基线挂钩（providers.go 接线用）
func (p *CronProvider) SetBaseline(b BaselineRecorder) { p.baseline = b }

func NewCronProvider(run Commander) *CronProvider {
	if run == nil {
		run = defaultCommander
	}
	return &CronProvider{run: run}
}

func (p *CronProvider) Type() string { return "cron" }

func (p *CronProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "status":
		return p.Status()
	case "jobs":
		return p.Jobs()
	case "job.apply":
		job, err := jobFromParams(params)
		if err != nil {
			return nil, err
		}
		return p.ApplyJob(job)
	case "job.delete":
		return p.DeleteJob(paramString(params, "name"))
	default:
		return nil, fmt.Errorf("unknown cron action: %s", action)
	}
}

func jobFromParams(params map[string]interface{}) (*CronJob, error) {
	raw, ok := params["job"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("job object required")
	}
	b, _ := json.Marshal(raw)
	var job CronJob
	if err := json.Unmarshal(b, &job); err != nil {
		return nil, fmt.Errorf("bad job payload: %w", err)
	}
	return &job, nil
}

// DetectCron 探测 crontab 命令可用性
func DetectCron() bool {
	_, err := exec.LookPath("crontab")
	return err == nil
}

// ============ RPC 实现 ============

// Status 概览：运行用户 + cockpit/外部条目计数
func (p *CronProvider) Status() (interface{}, error) {
	content, err := p.readCrontab()
	if err != nil {
		return nil, err
	}
	_, cockpit, external := splitCockpit(content)
	user := ""
	if u, err := exec.LookPath("whoami"); err == nil {
		out, _, err := p.run(context.Background(), u)
		if err == nil {
			user = strings.TrimSpace(string(out))
		}
	}
	return map[string]interface{}{
		"user":          user,
		"cockpitCount":  len(cockpit),
		"externalCount": len(external),
	}, nil
}

// Jobs cockpit 名下任务列表 + 外部条目原文（只读展示）
func (p *CronProvider) Jobs() (interface{}, error) {
	content, err := p.readCrontab()
	if err != nil {
		return nil, err
	}
	_, cockpit, external := splitCockpit(content)
	jobs := make([]map[string]interface{}, 0, len(cockpit))
	for i := range cockpit {
		j := &cockpit[i]
		jobs = append(jobs, map[string]interface{}{
			"name":     j.Name,
			"schedule": j.Schedule,
			"command":  j.Command,
			"enabled":  j.Enabled,
		})
	}
	return map[string]interface{}{
		"jobs":     jobs,
		"external": strings.Join(external, "\n"),
	}, nil
}

// ApplyJob 校验 → 读全量 → 替换 cockpit 段 → 自检 → 写回（D2）
func (p *CronProvider) ApplyJob(job *CronJob) (interface{}, error) {
	if err := job.validate(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	old, err := p.readCrontab()
	if err != nil {
		return nil, err
	}
	newContent := upsertJob(old, job)
	if err := p.writeCrontab(old, newContent); err != nil {
		return nil, err
	}
	// 基线只对 cockpit 任务段 hash（外部条目变更不算漂移，D2）
	if p.baseline != nil {
		_, jobs, _ := splitCockpit(newContent)
		encoded, _ := json.Marshal(jobs)
		p.baseline.Record("cron", "cockpit", encoded)
	}
	return map[string]interface{}{"name": job.Name}, nil
}

// DeleteJob 从 cockpit 段移除任务对 → 自检 → 写回
func (p *CronProvider) DeleteJob(name string) (interface{}, error) {
	if !cronJobNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid job name %q", name)
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	old, err := p.readCrontab()
	if err != nil {
		return nil, err
	}
	newContent, found := removeJob(old, name)
	if !found {
		return nil, fmt.Errorf("job not found: %s", name)
	}
	if err := p.writeCrontab(old, newContent); err != nil {
		return nil, err
	}
	if p.baseline != nil {
		_, jobs, _ := splitCockpit(newContent)
		encoded, _ := json.Marshal(jobs)
		p.baseline.Record("cron", "cockpit", encoded)
	}
	return map[string]interface{}{}, nil
}

// ============ 内部：crontab 读写与段落操作 ============

// readCrontab crontab -l；无 crontab（exit 1 且提示没有 crontab）视为空
func (p *CronProvider) readCrontab() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), nginxTestTimeout)
	defer cancel()
	out, stderr, err := p.run(ctx, "crontab", "-l")
	if err != nil {
		// 首次使用尚无 crontab 属正常，视为空表
		msg := strings.ToLower(commandErrSummary(stderr, err))
		if strings.Contains(msg, "no crontab for") {
			return "", nil
		}
		return "", fmt.Errorf("crontab -l: %s", commandErrSummary(stderr, err))
	}
	return string(out), nil
}

// writeCrontab 自检后写回；自检保证非 cockpit 行与旧内容一致（D2 兜底）
func (p *CronProvider) writeCrontab(old, newContent string) error {
	keptOld, _, _ := splitCockpit(old)
	keptNew, _, _ := splitCockpit(newContent)
	if keptOld != keptNew {
		return fmt.Errorf("safety check failed: external entries would change, aborting write")
	}
	// crontab - 需要 stdin，Commander 只收 argv；落临时文件用 `crontab <file>` 写回，语义等价
	return p.writeViaFile(newContent)
}

// writeViaFile 经临时文件写回：crontab 接受文件参数
func (p *CronProvider) writeViaFile(content string) error {
	f, err := os.CreateTemp("", "cockpit-crontab-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), nginxTestTimeout)
	defer cancel()
	_, stderr, err := p.run(ctx, "crontab", path)
	if err != nil {
		return fmt.Errorf("crontab write: %s", commandErrSummary(stderr, err))
	}
	return nil
}

// splitCockpit 拆分 crontab 内容：过滤 cockpit 任务对后的其余行（保持原序）、
// cockpit 任务列表、外部行列表。任务对 = meta 注释行 + 紧随的命令行。
func splitCockpit(content string) (kept string, jobs []CronJob, external []string) {
	lines := strings.Split(content, "\n")
	var keptLines, extLines []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !strings.HasPrefix(line, cronMetaPrefix) {
			keptLines = append(keptLines, line)
			if strings.TrimSpace(line) != "" {
				extLines = append(extLines, line)
			}
			continue
		}
		var job CronJob
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, cronMetaPrefix)), &job); err != nil {
			// 标记行损坏：按普通行保留（不吞用户文件内容）
			keptLines = append(keptLines, line)
			extLines = append(extLines, line)
			continue
		}
		jobs = append(jobs, job)
		i++ // 跳过命令行（任务对的第二行）
	}
	return strings.Join(keptLines, "\n"), jobs, extLines
}

// upsertJob 替换同名任务对或追加到段尾；其余行原样
func upsertJob(content string, job *CronJob) string {
	lines := strings.Split(content, "\n")
	pair := renderJobPair(job)
	replaced := false
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, cronMetaPrefix) {
			var existing CronJob
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, cronMetaPrefix)), &existing); err == nil && existing.Name == job.Name {
				out = append(out, pair...)
				replaced = true
				i++ // 跳过旧命令行
				continue
			}
		}
		out = append(out, line)
	}
	if !replaced {
		out = append(out, pair...)
	}
	return strings.Join(out, "\n")
}

// removeJob 删除任务对；found=false 表示任务不存在
func removeJob(content, name string) (string, bool) {
	lines := strings.Split(content, "\n")
	var out []string
	found := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, cronMetaPrefix) {
			var existing CronJob
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, cronMetaPrefix)), &existing); err == nil && existing.Name == name {
				found = true
				i++ // 跳过命令行
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n"), found
}

// renderJobPair 渲染任务对：meta 注释行 + 命令行（停用时命令行整体注释）
func renderJobPair(j *CronJob) []string {
	meta, _ := json.Marshal(j)
	cmdLine := j.Schedule + " " + j.Command
	if !j.Enabled {
		cmdLine = "#" + cmdLine
	}
	return []string{cronMetaPrefix + string(meta), cmdLine}
}
