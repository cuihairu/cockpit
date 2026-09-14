package rpc

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============ Backup Provider ============
//
// Agent 主机本地文件备份：把源路径列表打成一个 tar.gz 落到目标目录
// （本机第二块盘 / NAS 挂载点），server 只做调度与历史记录，备份数据流
// 不经 server（Agent 主动出站 WebSocket 传不了大数据，见 backup-design.md D1）。
// 打包耗时分钟级，复用 stack provider 的异步任务模型规避 30s RPC 超时。

var (
	// backupNameRe 备份名（文件名前缀）约束，同 stackNameRe
	backupNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	// backupFileNameRe backup.delete 只接受文件名（非路径），从根上杜绝穿越
	backupFileNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.tar\.gz$`)
)

const (
	backupLogLimit = 200 * 1024 // 任务日志环形缓冲上限
	// BackupMaxTasks 并发备份任务上限（IO 密集，比 stack 保守）
	BackupMaxTasks   = 2
	backupTimeFormat = "20060102-150405"
)

const (
	backupTaskRunning = "running"
	backupTaskSuccess = "success"
	backupTaskFailed  = "failed"
)

// BackupConfig Backup Provider 配置
type BackupConfig struct {
	MaxTasks int              // 并发任务上限，默认 BackupMaxTasks
	Now      func() time.Time // 可注入时钟
	NewID    func() string    // 可注入任务 ID 生成器
}

// BackupProvider 文件备份 Provider
type BackupProvider struct {
	now   func() time.Time
	newID func() string

	mu    sync.Mutex
	tasks map[string]*backupTask
	locks map[string]*sync.Mutex // 按备份名互斥：同一备份不允许并发（retention 清理会互踩）
	sem   chan struct{}
}

func NewBackupProvider(cfg BackupConfig) *BackupProvider {
	max := cfg.MaxTasks
	if max <= 0 {
		max = BackupMaxTasks
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	newID := cfg.NewID
	if newID == nil {
		newID = func() string {
			b := make([]byte, 8)
			if _, err := rand.Read(b); err != nil {
				return fmt.Sprintf("b%d", time.Now().UnixNano())
			}
			return "b" + hex.EncodeToString(b)
		}
	}
	return &BackupProvider{
		now:   now,
		newID: newID,
		tasks: make(map[string]*backupTask),
		locks: make(map[string]*sync.Mutex),
		sem:   make(chan struct{}, max),
	}
}

func (p *BackupProvider) Type() string { return "backup" }

func (p *BackupProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "run":
		return p.RunBackup(params)
	case "task.get":
		return p.GetTask(paramString(params, "taskId"))
	case "list":
		return p.ListFiles(paramString(params, "dir"))
	case "delete":
		return p.DeleteFile(paramString(params, "dir"), paramString(params, "name"))
	default:
		return nil, fmt.Errorf("unknown backup action: %s", action)
	}
}

// backupTask 一次备份任务的内存态
type backupTask struct {
	ID         string
	Name       string
	ConfigID   int64
	Status     string
	Error      string
	File       string // 成功时的备份文件名（不含目录）
	Size       int64
	Log        *cappedBuffer
	StartedAt  time.Time
	FinishedAt time.Time
}

// ============ 动作实现 ============

// RunBackup 校验参数后异步启动打包任务，立即返回 taskId
func (p *BackupProvider) RunBackup(params map[string]interface{}) (interface{}, error) {
	name := paramString(params, "name")
	if !backupNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid backup name: %q", name)
	}
	destDir := paramString(params, "destDir")
	if !filepath.IsAbs(filepath.Clean(destDir)) {
		return nil, fmt.Errorf("destDir must be absolute: %q", destDir)
	}
	sources, err := backupSources(params)
	if err != nil {
		return nil, err
	}
	retention := int(paramFloat(params, "retention"))
	configID := int64(paramFloat(params, "configId"))

	lock, ok := p.nameLock(name)
	if !ok {
		return nil, fmt.Errorf("backup %s busy", name)
	}
	select {
	case p.sem <- struct{}{}:
	default:
		lock.Unlock()
		return nil, fmt.Errorf("too many concurrent backup tasks")
	}

	task := &backupTask{
		ID:        p.newID(),
		Name:      name,
		ConfigID:  configID,
		Status:    backupTaskRunning,
		Log:       &cappedBuffer{limit: backupLogLimit},
		StartedAt: p.now(),
	}
	p.mu.Lock()
	p.tasks[task.ID] = task
	p.pruneTasksLocked()
	p.mu.Unlock()

	go func() {
		defer func() { <-p.sem }()
		defer lock.Unlock()

		file, size, err := p.pack(task, name, sources, destDir, retention)
		task.FinishedAt = p.now()
		task.File = file
		task.Size = size
		if err != nil {
			task.Status = backupTaskFailed
			task.Error = err.Error()
			fmt.Fprintf(task.Log, "\n[error] %v\n", err)
		} else {
			task.Status = backupTaskSuccess
		}
	}()

	return map[string]interface{}{"taskId": task.ID, "status": "started"}, nil
}

// GetTask 查询任务状态（server 轮询用）
func (p *BackupProvider) GetTask(taskID string) (map[string]interface{}, error) {
	if taskID == "" {
		return nil, fmt.Errorf("taskId required")
	}
	p.mu.Lock()
	task, ok := p.tasks[taskID]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return map[string]interface{}{
		"taskId":     task.ID,
		"name":       task.Name,
		"configId":   task.ConfigID,
		"status":     task.Status,
		"error":      task.Error,
		"file":       task.File,
		"size":       task.Size,
		"log":        task.Log.String(),
		"startedAt":  task.StartedAt.Unix(),
		"finishedAt": task.FinishedAt.Unix(),
	}, nil
}

// BackupFile 目标目录下的备份产物
type BackupFile struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
}

// ListFiles 列目录下的 *.tar.gz 普通文件（备份产物浏览）
func (p *BackupProvider) ListFiles(dir string) (map[string]interface{}, error) {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("dir must be absolute: %q", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}
	var files []BackupFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		files = append(files, BackupFile{Name: e.Name(), Size: info.Size(), Mtime: info.ModTime().Unix()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Mtime > files[j].Mtime })
	return map[string]interface{}{"files": files}, nil
}

// DeleteFile 删除目录下指定的备份文件；name 必须是纯文件名
func (p *BackupProvider) DeleteFile(dir, name string) (map[string]interface{}, error) {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("dir must be absolute: %q", dir)
	}
	if !backupFileNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid backup file name: %q", name)
	}
	path := filepath.Join(dir, name)
	if filepath.Dir(path) != dir { // 双保险：Join 后仍在 dir 内
		return nil, fmt.Errorf("invalid backup file name: %q", name)
	}
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("remove %s: %w", name, err)
	}
	return map[string]interface{}{"deleted": name}, nil
}

// ============ 打包 ============

// pack 逐源打包为 {destDir}/{name}-{timestamp}.tar.gz，随后按 retention 清理旧包。
// 返回文件名与大小；出错时尽力删除半成品。
func (p *BackupProvider) pack(task *backupTask, name string, sources []string, destDir string, retention int) (string, int64, error) {
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", 0, fmt.Errorf("create dest dir: %w", err)
	}
	fileName := fmt.Sprintf("%s-%s.tar.gz", name, p.now().Format(backupTimeFormat))
	outPath := filepath.Join(destDir, fileName)
	if _, err := os.Stat(outPath); err == nil {
		return "", 0, fmt.Errorf("backup file already exists: %s", fileName)
	}

	fmt.Fprintf(task.Log, "[backup] %s → %s (%d sources)\n", name, destDir, len(sources))
	f, err := os.Create(outPath)
	if err != nil {
		return "", 0, fmt.Errorf("create file: %w", err)
	}
	if _, packErr := writeArchive(task, f, sources); packErr != nil {
		f.Close()
		os.Remove(outPath) // 半成品不留
		return "", 0, packErr
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		os.Remove(outPath)
		return "", 0, fmt.Errorf("stat file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(outPath)
		return "", 0, fmt.Errorf("close file: %w", err)
	}
	size := info.Size()
	fmt.Fprintf(task.Log, "[backup] done: %s (%d bytes)\n", fileName, size)

	if retention > 0 {
		p.pruneOldBackups(task, destDir, name, retention)
	}
	return fileName, size, nil
}

// writeArchive 把全部 sources 写入 w。每个源以 basename 为顶层目录，
// 避免多源互相覆盖；符号链接记录为链接本身不跟随；单个源失败跳过继续。
// 返回写入的条目总数。
func writeArchive(task *backupTask, w io.Writer, sources []string) (int, error) {
	gz := gzip.NewWriter(w)
	gz.Name = "backup"
	tw := tar.NewWriter(gz)

	written := 0
	for _, src := range sources {
		n, err := addSource(tw, task.Log, src)
		if err != nil {
			fmt.Fprintf(task.Log, "[warn] skip %s: %v\n", src, err)
			continue
		}
		written += n
		fmt.Fprintf(task.Log, "[backup] %s: %d entries\n", src, n)
	}
	if written == 0 {
		tw.Close()
		gz.Close()
		return 0, fmt.Errorf("no source could be archived")
	}
	if err := tw.Close(); err != nil {
		gz.Close()
		return written, fmt.Errorf("finalize tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		return written, fmt.Errorf("finalize gzip: %w", err)
	}
	return written, nil
}

// addSource 递归把一个源路径写入 tar，返回写入的条目数
func addSource(tw *tar.Writer, log io.Writer, src string) (int, error) {
	src = filepath.Clean(src)
	if !filepath.IsAbs(src) {
		return 0, fmt.Errorf("source must be absolute")
	}
	if _, err := os.Lstat(src); err != nil { // Lstat：不跟随顶层符号链接
		return 0, err
	}
	top := filepath.Base(src)
	count := 0

	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if path == src {
				return err // 顶层失败才中断，子项失败跳过
			}
			fmt.Fprintf(log, "[warn] %s: %v\n", path, err)
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return nil
		}
		name := top
		if rel != "." {
			name = filepath.ToSlash(filepath.Join(top, rel))
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			fmt.Fprintf(log, "[warn] header %s: %v\n", path, err)
			return nil
		}
		hdr.Name = name
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return nil
			}
			hdr.Linkname = link
			hdr.Typeflag = tar.TypeSymlink
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				fmt.Fprintf(log, "[warn] open %s: %v\n", path, err)
				return nil
			}
			_, err = io.Copy(tw, f)
			f.Close()
			if err != nil {
				return err
			}
		}
		count++
		return nil
	})
	return count, err
}

// pruneOldBackups 按 mtime 保留最近 retention 个 {name}-*.tar.gz，删除更旧的
func (p *BackupProvider) pruneOldBackups(task *backupTask, destDir, name string, retention int) {
	matches, err := filepath.Glob(filepath.Join(destDir, name+"-*.tar.gz"))
	if err != nil || len(matches) <= retention {
		return
	}
	sort.Slice(matches, func(i, j int) bool {
		li, _ := os.Stat(matches[i])
		lj, _ := os.Stat(matches[j])
		if li == nil || lj == nil {
			return matches[i] < matches[j]
		}
		return li.ModTime().After(lj.ModTime())
	})
	for _, old := range matches[retention:] {
		if err := os.Remove(old); err != nil {
			fmt.Fprintf(task.Log, "[warn] prune %s: %v\n", filepath.Base(old), err)
			continue
		}
		fmt.Fprintf(task.Log, "[backup] pruned old: %s\n", filepath.Base(old))
	}
}

// ============ helpers ============

// backupSources 解析并校验 sources 数组
func backupSources(params map[string]interface{}) ([]string, error) {
	raw, _ := params["sources"].([]interface{})
	if len(raw) == 0 {
		return nil, fmt.Errorf("sources required")
	}
	var out []string
	for _, v := range raw {
		s, _ := v.(string)
		s = filepath.Clean(s)
		if !filepath.IsAbs(s) {
			return nil, fmt.Errorf("source must be absolute: %q", v)
		}
		out = append(out, s)
	}
	return out, nil
}

func paramFloat(params map[string]interface{}, key string) float64 {
	v, _ := params[key].(float64)
	return v
}

func (p *BackupProvider) nameLock(name string) (*sync.Mutex, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.locks == nil {
		p.locks = make(map[string]*sync.Mutex)
	}
	lock, ok := p.locks[name]
	if !ok {
		lock = &sync.Mutex{}
		p.locks[name] = lock
	}
	if !lock.TryLock() {
		return nil, false
	}
	return lock, true
}

// pruneTasksLocked 清理最早的已完成任务，调用方需持有 p.mu
func (p *BackupProvider) pruneTasksLocked() {
	const keep = 50
	if len(p.tasks) <= keep {
		return
	}
	var finished []*backupTask
	for _, t := range p.tasks {
		if t.Status != backupTaskRunning {
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
