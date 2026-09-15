package rpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 防漂移检测（见 docs/guide/drift-design.md）：
// 基线 = cockpit 面板最后一次成功写入的内容（写路径挂钩 Record），检查时
// 实时读盘比对 sha256。agent 本地 JSON 存储，server 不落库纯转发。

// baseline 文件默认路径（COCKPIT_DRIFT_BASELINE 覆盖）
const driftBaselineDefaultPath = "/var/lib/cockpit/drift-baseline.json"

// drift check 单命令超时（仅 crontab -l 一处外呼）
const driftCmdTimeout = 10 * time.Second

// BaselineEntry 单条基线
type BaselineEntry struct {
	SHA256    string `json:"sha256"`
	UpdatedAt int64  `json:"updated_at"`
}

// baselineFile 基线文件结构
type baselineFile struct {
	Version int                      `json:"version"`
	Entries map[string]BaselineEntry `json:"entries"`
}

// BaselineRecorder 写路径挂钩接口：provider 写成功后 Record、删除成功后
// Forget。实现必须并发安全且**不得让业务写路径失败**（内部吞错记日志）。
type BaselineRecorder interface {
	Record(kind, name string, content []byte)
	Forget(kind, name string)
}

// DriftBaseline 基线存储：mutex + 临时文件 rename 原子替换
type DriftBaseline struct {
	mu   sync.Mutex
	path string
}

// NewDriftBaseline path 为空时用默认路径（含 env 覆盖）
func NewDriftBaseline(path string) *DriftBaseline {
	if path == "" {
		path = os.Getenv("COCKPIT_DRIFT_BASELINE")
		if path == "" {
			path = driftBaselineDefaultPath
		}
	}
	return &DriftBaseline{path: path}
}

// Record 记录/更新条目；失败仅记日志（写路径成功不能被基线失败拖垮）
func (b *DriftBaseline) Record(kind, name string, content []byte) {
	sum := sha256.Sum256(content)
	if err := b.update(kind+"/"+name, BaselineEntry{
		SHA256:    hex.EncodeToString(sum[:]),
		UpdatedAt: time.Now().Unix(),
	}); err != nil {
		log.Printf("drift baseline record %s/%s: %v", kind, name, err)
	}
}

// Forget 删除条目；条目不存在属正常（幂等）
func (b *DriftBaseline) Forget(kind, name string) {
	if err := b.update(kind+"/"+name, BaselineEntry{}, true); err != nil {
		log.Printf("drift baseline forget %s/%s: %v", kind, name, err)
	}
}

// snapshot 拷贝当前全部条目
func (b *DriftBaseline) snapshot() map[string]BaselineEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	f, err := b.load()
	if err != nil {
		return map[string]BaselineEntry{}
	}
	out := make(map[string]BaselineEntry, len(f.Entries))
	for k, v := range f.Entries {
		out[k] = v
	}
	return out
}

func (b *DriftBaseline) update(key string, entry BaselineEntry, remove ...bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	f, err := b.load()
	if err != nil {
		f = baselineFile{Version: 1, Entries: map[string]BaselineEntry{}}
	}
	if len(remove) > 0 && remove[0] {
		delete(f.Entries, key)
	} else {
		f.Entries[key] = entry
	}
	return b.save(f)
}

func (b *DriftBaseline) load() (baselineFile, error) {
	raw, err := os.ReadFile(b.path)
	if err != nil {
		return baselineFile{}, err
	}
	var f baselineFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return baselineFile{}, fmt.Errorf("parse baseline: %w", err)
	}
	if f.Entries == nil {
		f.Entries = map[string]BaselineEntry{}
	}
	return f, nil
}

// save 原子写：目录确保存在 → 临时文件 → rename（D3）
func (b *DriftBaseline) save(f baselineFile) error {
	if err := os.MkdirAll(filepath.Dir(b.path), 0o700); err != nil {
		return fmt.Errorf("create baseline dir: %w", err)
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write baseline temp: %w", err)
	}
	if err := os.Rename(tmp, b.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename baseline: %w", err)
	}
	return nil
}

// DriftConfig DriftProvider 配置（零值字段回退 env / 默认值）
type DriftConfig struct {
	ConfDir   string    // nginx 片段目录，默认 /etc/nginx/conf.d
	StacksDir string    // stacks 根目录，默认 /var/lib/cockpit/stacks
	CronRun   Commander // 非 nil 才检查 cron 段
}

// DriftProvider drift.check：按类枚举当前对象，与基线比对出四态清单
type DriftProvider struct {
	baseline  *DriftBaseline
	confDir   string
	stacksDir string
	cronRun   Commander
}

func NewDriftProvider(baseline *DriftBaseline, cfg DriftConfig) *DriftProvider {
	if cfg.ConfDir == "" {
		if v := os.Getenv("COCKPIT_NGINX_CONF_DIR"); v != "" {
			cfg.ConfDir = v
		} else {
			cfg.ConfDir = "/etc/nginx/conf.d"
		}
	}
	if cfg.StacksDir == "" {
		if v := os.Getenv("COCKPIT_STACKS_DIR"); v != "" {
			cfg.StacksDir = v
		} else {
			cfg.StacksDir = "/var/lib/cockpit/stacks"
		}
	}
	return &DriftProvider{
		baseline:  baseline,
		confDir:   cfg.ConfDir,
		stacksDir: cfg.StacksDir,
		cronRun:   cfg.CronRun,
	}
}

func (p *DriftProvider) Type() string { return "drift" }

// SetCronRunner 启用 cron 段检查（crontab 命令可用时由 providers.go 调用；
// 默认 Commander 为包内 exec 实现，故经方法注入而非 cfg）
func (p *DriftProvider) SetCronRunner() { p.cronRun = defaultCommander }

func (p *DriftProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "check":
		return p.Check()
	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
}

// driftItem 检查结果单条（status 四态见设计 D5；读取失败为 error）
type driftItem struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	BaselineSHA string `json:"baseline_sha"`
	CurrentSHA  string `json:"current_sha"`
}

// Check 全量检查：nginx 片段文件 / cron cockpit 段 / stack compose+.env。
// 检查只读，任何一类失败不影响其他类（该类报 error 条目）。
func (p *DriftProvider) Check() (interface{}, error) {
	items := make([]driftItem, 0, 8)
	seen := map[string]bool{} // 已枚举到的基线 key（用于补 missing）

	// nginx：对片段文件原始字节 hash——meta 注释被改坏同样是漂移（D6）
	matches, _ := filepath.Glob(filepath.Join(p.confDir, "cockpit-site-*.conf"))
	for _, m := range matches {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "cockpit-site-"), ".conf")
		content, err := os.ReadFile(m)
		if err != nil {
			items = append(items, driftItem{Kind: "nginx", Name: name, Status: "error"})
			continue
		}
		items = append(items, p.compare("nginx", name, content, seen))
	}

	// cron：cockpit 任务列表 marshal hash；外部条目不在检测范围（D2）
	if p.cronRun != nil {
		content, err := p.readCrontab()
		if err != nil {
			items = append(items, driftItem{Kind: "cron", Name: "cockpit", Status: "error"})
		} else {
			_, jobs, _ := splitCockpit(content)
			encoded, _ := json.Marshal(jobs)
			items = append(items, p.compare("cron", "cockpit", encoded, seen))
		}
	}

	// stack：逐目录查 compose.yml 与 .env（不存在属正常，不产生条目）
	if entries, err := os.ReadDir(p.stacksDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() || e.Name() == stackReservedDir {
				continue
			}
			for _, f := range []string{"compose.yml", ".env"} {
				content, err := os.ReadFile(filepath.Join(p.stacksDir, e.Name(), f))
				if err != nil {
					continue
				}
				items = append(items, p.compare("stack", e.Name()+"/"+f, content, seen))
			}
		}
	}

	// 反向补 missing：基线里有、本轮没枚举到（文件被手删）
	for key, entry := range p.baseline.snapshot() {
		if seen[key] {
			continue
		}
		kind, name, ok := strings.Cut(key, "/")
		if !ok || (kind != "nginx" && kind != "stack") {
			continue // cron 单条目无 missing 态（drifted 覆盖全删场景）
		}
		items = append(items, driftItem{
			Kind: kind, Name: name, Status: "missing", BaselineSHA: entry.SHA256,
		})
	}

	return map[string]interface{}{
		"items":      items,
		"checked_at": time.Now().Unix(),
	}, nil
}

// compare 单对象比对：seen 登记基线 key，返回状态条目
func (p *DriftProvider) compare(kind, name string, current []byte, seen map[string]bool) driftItem {
	sum := sha256.Sum256(current)
	item := driftItem{Kind: kind, Name: name, CurrentSHA: hex.EncodeToString(sum[:])}

	entry, has := p.baseline.snapshot()[kind+"/"+name]
	if !has {
		// cron 从未通过面板管过任务（空段且无基线）不产生噪音条目
		if kind == "cron" && string(current) == "[]" {
			return driftItem{Kind: kind, Name: name, Status: "none"}
		}
		item.Status = "no_baseline"
		return item
	}
	seen[kind+"/"+name] = true
	item.BaselineSHA = entry.SHA256
	if item.CurrentSHA == entry.SHA256 {
		item.Status = "ok"
	} else {
		item.Status = "drifted"
	}
	return item
}

// readCrontab crontab -l；无 crontab 视为空（与 CronProvider 同语义）
func (p *DriftProvider) readCrontab() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), driftCmdTimeout)
	defer cancel()
	out, stderr, err := p.cronRun(ctx, "crontab", "-l")
	if err != nil {
		if strings.Contains(strings.ToLower(commandErrSummary(stderr, err)), "no crontab for") {
			return "", nil
		}
		return "", fmt.Errorf("crontab -l: %s", commandErrSummary(stderr, err))
	}
	return string(out), nil
}
