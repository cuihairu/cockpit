package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/docker"
)

// ============ Compose Stack Provider ============
//
// 管理 Agent 主机上的 Docker Compose Stack（compose-file-first：
// <dir>/<name>/compose.yml 是真相源，数据库只做索引缓存）。
// 部署动作 shell out 到 `docker compose` CLI（Dockge/Komodo 同款做法），
// up/down/remove 以异步任务执行，规避 server 端 30s RPC 响应超时。

var (
	// stackNameRe stack 名称约束：同时满足 compose project name 的安全子集，
	// 且排除路径分隔符/点号，杜绝路径穿越
	stackNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

	// 识别为 stack 的 compose 文件，按优先级排列；保存时统一规范为 compose.yml
	stackComposeFiles = []string{"compose.yml", "compose.yaml", "docker-compose.yml", "docker-compose.yaml"}
)

const (
	stackFileLimit = 512 * 1024 // compose/.env 单文件上限
	stackLogLimit  = 200 * 1024 // 任务日志内存环形缓冲上限
	stackMaxTasks  = 4          // 同时运行的任务数上限
	stackLogTail   = 1 << 20    // 同步读取 compose logs 的上限 1MB

	stackTaskRunning = "running"
	stackTaskSuccess = "success"
	stackTaskFailed  = "failed"

	stackReservedDir = ".cockpit" // 每 stack 的保留目录（部署日志、临时文件）
)

// ComposeAvailable 检测 docker compose CLI 是否可用
func ComposeAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "compose", "version")
	return cmd.Run() == nil
}

// StackConfig Stack Provider 配置
type StackConfig struct {
	Dir        string           // stacks 根目录，默认 /var/lib/cockpit/stacks
	Docker     docker.DockerAPI // 用于按 compose label 过滤容器状态
	ComposeBin []string         // compose 命令，默认 ["docker", "compose"]；测试可注入
	Now        func() time.Time // 可注入时钟
	NewID      func() string    // 可注入任务 ID 生成器
}

// StackProvider Compose Stack Provider
type StackProvider struct {
	dir        string
	docker     docker.DockerAPI
	composeBin []string
	now        func() time.Time
	newID      func() string
	baseline   BaselineRecorder // 漂移基线挂钩（见 drift-design.md D7），nil 不记录

	mu         sync.Mutex
	tasks      map[string]*stackTask
	stackLocks map[string]*sync.Mutex
	sem        chan struct{}
}

// SetBaseline 注入漂移基线挂钩（providers.go 接线用）
func (p *StackProvider) SetBaseline(b BaselineRecorder) { p.baseline = b }

func NewStackProvider(cfg StackConfig) *StackProvider {
	dir := cfg.Dir
	if dir == "" {
		dir = "/var/lib/cockpit/stacks"
	}
	composeBin := cfg.ComposeBin
	if len(composeBin) == 0 {
		composeBin = []string{"docker", "compose"}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	newID := cfg.NewID
	if newID == nil {
		newID = func() string {
			b := make([]byte, 8)
			if _, err := randRead(b); err != nil {
				return fmt.Sprintf("t%d", time.Now().UnixNano())
			}
			return "t" + hex.EncodeToString(b)
		}
	}
	return &StackProvider{
		dir:        dir,
		docker:     cfg.Docker,
		composeBin: composeBin,
		now:        now,
		newID:      newID,
		tasks:      make(map[string]*stackTask),
		stackLocks: make(map[string]*sync.Mutex),
		sem:        make(chan struct{}, stackMaxTasks),
	}
}

func (p *StackProvider) Type() string { return "stack" }

func (p *StackProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "list":
		return p.ListStacks()
	case "status":
		return p.StatusStack(stackParam(params))
	case "file.get":
		return p.GetStackFile(stackParam(params))
	case "file.save":
		return p.SaveStackFile(params)
	case "up":
		return p.UpStack(stackParam(params))
	case "down":
		return p.DownStack(stackParam(params))
	case "restart":
		return p.RestartStack(stackParam(params))
	case "pull":
		return p.PullStack(stackParam(params))
	case "info":
		return p.InfoStack(), nil
	case "logs":
		return p.StackLogs(params)
	case "task.get":
		return p.GetTask(paramString(params, "taskId"))
	case "remove":
		return p.RemoveStack(stackParam(params))
	default:
		return nil, fmt.Errorf("unknown stack action: %s", action)
	}
}

func stackParam(params map[string]interface{}) string {
	return paramString(params, "name")
}

func paramString(params map[string]interface{}, key string) string {
	v, _ := params[key].(string)
	return v
}

// ============ 数据结构（JSON 输出统一 camelCase） ============

// StackService stack 内的单个服务容器
type StackService struct {
	Name        string `json:"name"`
	Image       string `json:"image"`
	State       string `json:"state"`
	Status      string `json:"status"`
	ContainerID string `json:"containerId"`
}

// StackSummary 单个 stack 的状态总览
type StackSummary struct {
	Name       string         `json:"name"`
	Running    int            `json:"running"`
	Total      int            `json:"total"`
	Services   []StackService `json:"services"`
	LastAction string         `json:"lastAction,omitempty"`
	LastStatus string         `json:"lastStatus,omitempty"`
	// LastDeployedAt 最近一次部署完成时间（unix 秒，0 = 从未部署）
	LastDeployedAt int64  `json:"lastDeployedAt"`
	ComposeFile    string `json:"composeFile,omitempty"`
}

// StackFile stack 的 compose/.env 文件内容
type StackFile struct {
	Name        string `json:"name"`
	Compose     string `json:"compose"`
	Env         string `json:"env"`
	ComposeFile string `json:"composeFile"`
	ModifiedAt  int64  `json:"modifiedAt"`
}

// stackTask 异步部署任务
type stackTask struct {
	ID         string
	Stack      string
	Action     string
	Status     string
	Log        *cappedBuffer
	StartedAt  time.Time
	FinishedAt time.Time
}

// stackLastTask 落盘的任务元数据（.cockpit/last-task.json），
// agent 重启后任务内存态丢失，靠它维持列表里的最近部署信息
type stackLastTask struct {
	Action     string `json:"action"`
	Status     string `json:"status"`
	FinishedAt int64  `json:"finishedAt"`
}

// cappedBuffer 只保留最后 limit 字节的环形缓冲。Write/String 会被不同
// 协程并发调用（任务执行协程写日志、GetTask 轮询读），内部自持锁。
type cappedBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.limit {
		b.buf = b.buf[len(b.buf)-b.limit:]
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// ============ 路径安全 ============

// stackDir 校验名称并返回 stack 目录（名称正则排除分隔符，Join 后无需再校验前缀）
func (p *StackProvider) stackDir(name string) (string, error) {
	if !stackNameRe.MatchString(name) {
		return "", fmt.Errorf("invalid stack name: %q", name)
	}
	return filepath.Join(p.dir, name), nil
}

// findComposeFile 返回 stack 目录内优先级最高的 compose 文件名，不存在返回 ""
func findComposeFile(dir string) string {
	for _, f := range stackComposeFiles {
		if st, err := os.Stat(filepath.Join(dir, f)); err == nil && !st.IsDir() {
			return f
		}
	}
	return ""
}

// ============ 列表与状态 ============

// ListStacks 列出所有 stack（目录即 stack，无 compose 文件的目录跳过）
func (p *StackProvider) ListStacks() ([]StackSummary, error) {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []StackSummary{}, nil
		}
		return nil, fmt.Errorf("read stacks dir: %w", err)
	}

	// 一次容器列表调用覆盖所有 stack（按 compose project label 分组）
	byProject := map[string][]docker.ContainerInfo{}
	if p.docker != nil {
		if containers, err := p.docker.ListContainers(true); err == nil {
			for _, cnt := range containers {
				project := cnt.Labels["com.docker.compose.project"]
				if project != "" {
					byProject[project] = append(byProject[project], cnt)
				}
			}
		}
	}

	result := make([]StackSummary, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		dir := filepath.Join(p.dir, name)
		file := findComposeFile(dir)
		if file == "" {
			continue
		}
		result = append(result, p.buildSummary(name, file, byProject[name]))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// StatusStack 单个 stack 的状态明细
func (p *StackProvider) StatusStack(name string) (*StackSummary, error) {
	dir, err := p.stackDir(name)
	if err != nil {
		return nil, err
	}
	file := findComposeFile(dir)
	if file == "" {
		return nil, fmt.Errorf("stack %s not found", name)
	}

	var containers []docker.ContainerInfo
	if p.docker != nil {
		all, err := p.docker.ListContainers(true)
		if err != nil {
			return nil, fmt.Errorf("list containers: %w", err)
		}
		for _, cnt := range all {
			if cnt.Labels["com.docker.compose.project"] == name {
				containers = append(containers, cnt)
			}
		}
	}
	summary := p.buildSummary(name, file, containers)
	return &summary, nil
}

func (p *StackProvider) buildSummary(name, composeFile string, containers []docker.ContainerInfo) StackSummary {
	summary := StackSummary{
		Name:        name,
		Services:    make([]StackService, 0, len(containers)),
		ComposeFile: composeFile,
	}
	for _, cnt := range containers {
		state := strings.ToLower(cnt.State)
		if state == "running" {
			summary.Running++
		}
		summary.Total++
		summary.Services = append(summary.Services, StackService{
			// 服务名取 compose service label，缺失时退化为容器名
			Name:        firstNonEmpty(cnt.Labels["com.docker.compose.service"], cnt.Name),
			Image:       cnt.Image,
			State:       state,
			Status:      cnt.Status,
			ContainerID: cnt.ID,
		})
	}
	if lt := readLastTask(filepath.Join(p.dir, name, stackReservedDir)); lt != nil {
		summary.LastAction = lt.Action
		summary.LastStatus = lt.Status
		summary.LastDeployedAt = lt.FinishedAt
	}
	return summary
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func readLastTask(cockpitDir string) *stackLastTask {
	data, err := os.ReadFile(filepath.Join(cockpitDir, "last-task.json"))
	if err != nil {
		return nil
	}
	var lt stackLastTask
	if json.Unmarshal(data, &lt) != nil {
		return nil
	}
	return &lt
}

func (p *StackProvider) persistLastTask(stack string, task *stackTask) {
	stackDir := filepath.Join(p.dir, stack)
	// stack 已被删除（remove 任务）时不重建目录
	if _, err := os.Stat(stackDir); err != nil {
		return
	}
	dir := filepath.Join(stackDir, stackReservedDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	lt := stackLastTask{Action: task.Action, Status: task.Status, FinishedAt: task.FinishedAt.Unix()}
	data, _ := json.Marshal(lt) // 纯标量结构体，Marshal 不会失败
	_ = os.WriteFile(filepath.Join(dir, "last-task.json"), data, 0o644)
}

// ============ 文件读写 ============

// GetStackFile 读取 compose/.env 内容
func (p *StackProvider) GetStackFile(name string) (*StackFile, error) {
	dir, err := p.stackDir(name)
	if err != nil {
		return nil, err
	}
	file := findComposeFile(dir)
	if file == "" {
		return nil, fmt.Errorf("stack %s not found", name)
	}

	result := &StackFile{Name: name, ComposeFile: file}
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}
	result.Compose = string(data)

	if st, err := os.Stat(filepath.Join(dir, file)); err == nil {
		result.ModifiedAt = st.ModTime().Unix()
	}
	if env, err := os.ReadFile(filepath.Join(dir, ".env")); err == nil {
		result.Env = string(env)
	}
	return result, nil
}

// SaveStackFile 保存 compose/.env，保存前先过 `docker compose config -q` 校验，
// 失败不落盘。首次保存时返回 created=true 供 server 记审计。
func (p *StackProvider) SaveStackFile(params map[string]interface{}) (map[string]interface{}, error) {
	name := stackParam(params)
	compose, _ := params["compose"].(string)
	envPtr := func() *string {
		if v, ok := params["env"].(string); ok {
			return &v
		}
		return nil
	}()

	dir, err := p.stackDir(name)
	if err != nil {
		return nil, err
	}
	if compose == "" {
		return nil, fmt.Errorf("compose content required")
	}
	if len(compose) > stackFileLimit {
		return nil, fmt.Errorf("compose file too large (max %d bytes)", stackFileLimit)
	}
	if envPtr != nil && len(*envPtr) > stackFileLimit {
		return nil, fmt.Errorf(".env file too large (max %d bytes)", stackFileLimit)
	}

	existed := findComposeFile(dir) != ""

	cockpitDir := filepath.Join(dir, stackReservedDir)
	if err := os.MkdirAll(cockpitDir, 0o700); err != nil {
		return nil, fmt.Errorf("create stack dir: %w", err)
	}

	// 先写临时文件再校验，避免校验失败留下损坏的 compose.yml
	tmp := filepath.Join(cockpitDir, "compose.yml.tmp")
	if err := os.WriteFile(tmp, []byte(compose), 0o644); err != nil {
		return nil, fmt.Errorf("write compose temp file: %w", err)
	}
	if err := p.validateCompose(dir, tmp); err != nil {
		os.Remove(tmp)
		return nil, err
	}

	// 统一规范为 compose.yml，清理旧命名
	for _, f := range stackComposeFiles {
		if f != "compose.yml" {
			os.Remove(filepath.Join(dir, f))
		}
	}
	if err := os.Rename(tmp, filepath.Join(dir, "compose.yml")); err != nil {
		return nil, fmt.Errorf("save compose file: %w", err)
	}
	if p.baseline != nil {
		p.baseline.Record("stack", name+"/compose.yml", []byte(compose))
	}

	if envPtr != nil {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(*envPtr), 0o600); err != nil {
			return nil, fmt.Errorf("save .env file: %w", err)
		}
		if p.baseline != nil {
			p.baseline.Record("stack", name+"/.env", []byte(*envPtr))
		}
	}

	return map[string]interface{}{"status": "saved", "created": !existed}, nil
}

// validateCompose 用 `docker compose config -q` 校验 YAML；
// --project-directory 保证文件内的相对路径按 stack 目录解析
func (p *StackProvider) validateCompose(dir, composeFile string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.composeBin[0],
		p.composeArgs("--project-directory", dir, "-f", composeFile, "config", "-q")...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("compose validation failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// composeArgs 拼接 compose 子命令参数（composeBin[1:] 之后的部分）
func (p *StackProvider) composeArgs(args ...string) []string {
	rest := append([]string{}, p.composeBin[1:]...)
	return append(rest, args...)
}

// ============ 异步任务 ============

func (p *StackProvider) stackLock(name string) *sync.Mutex {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stackLocks[name] == nil {
		p.stackLocks[name] = &sync.Mutex{}
	}
	return p.stackLocks[name]
}

// UpStack 异步执行 compose up -d
func (p *StackProvider) UpStack(name string) (map[string]interface{}, error) {
	dir, err := p.stackDir(name)
	if err != nil {
		return nil, err
	}
	if findComposeFile(dir) == "" {
		return nil, fmt.Errorf("stack %s not found", name)
	}
	return p.launchTask(name, "up", func(task *stackTask) error {
		return p.runCompose(task, dir, "up", "-d", "--remove-orphans")
	})
}

// DownStack 异步执行 compose down
func (p *StackProvider) DownStack(name string) (map[string]interface{}, error) {
	dir, err := p.stackDir(name)
	if err != nil {
		return nil, err
	}
	if findComposeFile(dir) == "" {
		return nil, fmt.Errorf("stack %s not found", name)
	}
	return p.launchTask(name, "down", func(task *stackTask) error {
		return p.runCompose(task, dir, "down", "--remove-orphans")
	})
}

// RestartStack 异步执行 compose restart（仅作用于已在运行的服务）
func (p *StackProvider) RestartStack(name string) (map[string]interface{}, error) {
	dir, err := p.stackDir(name)
	if err != nil {
		return nil, err
	}
	if findComposeFile(dir) == "" {
		return nil, fmt.Errorf("stack %s not found", name)
	}
	return p.launchTask(name, "restart", func(task *stackTask) error {
		return p.runCompose(task, dir, "restart")
	})
}

// PullStack 异步执行 compose pull（拉取最新镜像，不重建容器）
func (p *StackProvider) PullStack(name string) (map[string]interface{}, error) {
	dir, err := p.stackDir(name)
	if err != nil {
		return nil, err
	}
	if findComposeFile(dir) == "" {
		return nil, fmt.Errorf("stack %s not found", name)
	}
	return p.launchTask(name, "pull", func(task *stackTask) error {
		return p.runCompose(task, dir, "pull")
	})
}

// StackInfo agent 侧自检信息（M1.5）：stacks 目录状态 + compose CLI 版本
type StackInfo struct {
	Dir         string `json:"dir"`
	DirWritable bool   `json:"dirWritable"`
	DirError    string `json:"dirError,omitempty"`
	// ComposeVersion docker compose 版本输出，探测失败为空
	ComposeVersion string `json:"composeVersion,omitempty"`
}

// InfoStack 检查 stacks 目录可写性与 compose CLI 版本。
// 低频调用（列表页自检提示），每次实时探测、不做缓存。
func (p *StackProvider) InfoStack() *StackInfo {
	info := &StackInfo{Dir: p.dir}
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		info.DirError = fmt.Sprintf("create dir: %v", err)
		return info
	}
	probe, err := os.CreateTemp(p.dir, ".cockpit-probe-*")
	if err != nil {
		info.DirError = fmt.Sprintf("write probe: %v", err)
		return info
	}
	probe.Close()
	_ = os.Remove(probe.Name())
	info.DirWritable = true

	info.ComposeVersion = p.probeComposeVersion()
	return info
}

// probeComposeVersion 返回 docker compose 版本字符串，失败返回空。
// composeBin 形如 ["docker", "compose"]：executable 取首位，其余拼接
// "compose version --short"。
func (p *StackProvider) probeComposeVersion() string {
	args := append([]string{}, p.composeBin[1:]...)
	args = append(args, "version", "--short")
	out, err := exec.Command(p.composeBin[0], args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// RemoveStack 异步删除：先 down（失败则中止，保留目录），再删目录
func (p *StackProvider) RemoveStack(name string) (map[string]interface{}, error) {
	dir, err := p.stackDir(name)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("stack %s not found", name)
	}
	return p.launchTask(name, "remove", func(task *stackTask) error {
		if findComposeFile(dir) != "" {
			if err := p.runCompose(task, dir, "down", "--remove-orphans"); err != nil {
				return fmt.Errorf("down before remove: %w", err)
			}
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove stack dir: %w", err)
		}
		if p.baseline != nil {
			p.baseline.Forget("stack", name+"/compose.yml")
			p.baseline.Forget("stack", name+"/.env")
		}
		return nil
	})
}

// launchTask 启动异步任务：每 stack 一把互斥锁（TryLock 忙拒绝）+
// 全局信号量限并发。锁在同步段获取，保证「启动后立刻再启动」必得 busy。
func (p *StackProvider) launchTask(stack, action string, run func(*stackTask) error) (map[string]interface{}, error) {
	lock := p.stackLock(stack)
	if !lock.TryLock() {
		return nil, fmt.Errorf("stack %s busy", stack)
	}
	select {
	case p.sem <- struct{}{}:
	default:
		lock.Unlock()
		return nil, fmt.Errorf("too many concurrent stack tasks")
	}

	task := &stackTask{
		ID:        p.newID(),
		Stack:     stack,
		Action:    action,
		Status:    stackTaskRunning,
		Log:       &cappedBuffer{limit: stackLogLimit},
		StartedAt: p.now(),
	}
	p.mu.Lock()
	p.tasks[task.ID] = task
	p.pruneTasksLocked()
	p.mu.Unlock()

	go func() {
		defer func() { <-p.sem }()
		defer lock.Unlock()

		err := run(task)
		// 终态字段在锁内更新：GetTask 轮询会并发读取。
		// last-task.json 也在锁内落盘：保证轮询观察到终态时文件已写完，
		// 否则 GetTask 返回 failed 后立刻读文件会看到旧内容/不存在。
		p.mu.Lock()
		task.FinishedAt = p.now()
		if err != nil {
			task.Status = stackTaskFailed
			fmt.Fprintf(task.Log, "\n[error] %v\n", err)
		} else {
			task.Status = stackTaskSuccess
		}
		p.persistLastTask(stack, task)
		p.mu.Unlock()
	}()

	return map[string]interface{}{"taskId": task.ID, "status": "started"}, nil
}

// runCompose 执行 compose 子命令，输出写入任务日志
func (p *StackProvider) runCompose(task *stackTask, dir string, args ...string) error {
	full := p.composeArgs("-p", task.Stack, "--project-directory", dir, "-f", findComposeFile(dir))
	full = append(full, args...)
	cmd := exec.Command(p.composeBin[0], full...)
	cmd.Dir = dir
	cmd.Stdout = task.Log
	cmd.Stderr = task.Log
	return cmd.Run()
}

// pruneTasksLocked 清理最早的已完成任务，防止 tasks map 无限增长。
// 调用方需持有 p.mu。
func (p *StackProvider) pruneTasksLocked() {
	const keep = 100
	if len(p.tasks) <= keep {
		return
	}
	var finished []*stackTask
	for _, t := range p.tasks {
		if t.Status != stackTaskRunning {
			finished = append(finished, t)
		}
	}
	sort.Slice(finished, func(i, j int) bool {
		return finished[i].FinishedAt.Before(finished[j].FinishedAt)
	})
	for i := 0; i < len(finished) && len(p.tasks) > keep; i++ {
		delete(p.tasks, finished[i].ID)
	}
}

// GetTask 查询任务状态
func (p *StackProvider) GetTask(taskID string) (map[string]interface{}, error) {
	if taskID == "" {
		return nil, fmt.Errorf("taskId required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	task, ok := p.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	// 任务字段在锁内读取：执行协程会并发写终态（cappedBuffer 自持锁
	// 只保护日志缓冲本身）
	return map[string]interface{}{
		"id":         task.ID,
		"stack":      task.Stack,
		"action":     task.Action,
		"status":     task.Status,
		"log":        task.Log.String(),
		"startedAt":  task.StartedAt.Unix(),
		"finishedAt": task.FinishedAt.Unix(),
	}, nil
}

// ============ 同步日志 ============

// StackLogs 读取 compose logs（同步，30s 超时）
func (p *StackProvider) StackLogs(params map[string]interface{}) (map[string]interface{}, error) {
	name := stackParam(params)
	dir, err := p.stackDir(name)
	if err != nil {
		return nil, err
	}
	file := findComposeFile(dir)
	if file == "" {
		return nil, fmt.Errorf("stack %s not found", name)
	}

	tail := paramString(params, "tail")
	if tail == "" {
		tail = "200"
	} else if n, err := strconv.Atoi(tail); err != nil || n < 0 || n > 100000 {
		return nil, fmt.Errorf("invalid tail: %q", tail)
	}

	args := p.composeArgs("-p", name, "--project-directory", dir, "-f", file,
		"logs", "--no-log-prefix", "--tail", tail)
	if service := paramString(params, "service"); service != "" {
		args = append(args, service)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.composeBin[0], args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("compose logs: %w", err)
	}
	logs := string(out)
	if len(logs) > stackLogTail {
		logs = logs[len(logs)-stackLogTail:]
	}
	return map[string]interface{}{"logs": logs}, nil
}
