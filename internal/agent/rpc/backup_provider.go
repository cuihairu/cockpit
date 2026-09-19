package rpc

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	// backupRemoteDestRe rclone 远端目标 remote:path（M2 D22）：前段对应
	// rclone.conf 的 section 名（字母数字开头，语法上不可能以 - 开头，
	// 杜绝 exec 时被 rclone 误当 flag）；path 段禁空白与控制字符
	backupRemoteDestRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*:[^\x00-\x20\x7f]+$`)
)

const (
	backupLogLimit = 200 * 1024 // 任务日志环形缓冲上限
	// BackupMaxTasks 并发备份任务上限（IO 密集，比 stack 保守）
	BackupMaxTasks = 2
	// backupReadChunkLimit backup.read 单块字节上限（server 侧实际用更小分块）
	backupReadChunkLimit = 1024 * 1024
	backupTimeFormat     = "20060102-150405"
	// backupHookMaxLen pre-hook 命令长度上限（与 BackupConfig.PreHook gorm size 一致）
	backupHookMaxLen = 1024
)

// backupHookTimeout pre-hook 单次执行上限（M3 D29：dump 大库可能分钟级，
// 但任务后面还有打包+推送）；var 便于测试注入
var backupHookTimeout = 5 * time.Minute

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
	now       func() time.Time
	newID     func() string
	rcloneBin string // rclone 可执行文件，测试注入 fake（M2 D18）

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
		now:       now,
		newID:     newID,
		rcloneBin: "rclone",
		tasks:     make(map[string]*backupTask),
		locks:     make(map[string]*sync.Mutex),
		sem:       make(chan struct{}, max),
	}
}

func (p *BackupProvider) Type() string { return "backup" }

func (p *BackupProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "run":
		return p.RunBackup(params)
	case "restore":
		return p.RunRestore(params)
	case "task.get":
		return p.GetTask(paramString(params, "taskId"))
	case "read":
		return p.ReadChunk(params)
	case "list":
		return p.ListFiles(paramString(params, "dir"))
	case "delete":
		return p.DeleteFile(paramString(params, "dir"), paramString(params, "name"))
	case "remote.sync":
		return p.SyncRemote(params)
	default:
		return nil, fmt.Errorf("unknown backup action: %s", action)
	}
}

// backupTask 一次备份/恢复任务的内存态
type backupTask struct {
	ID           string
	Action       string // backup / restore
	Name         string
	ConfigID     int64
	Status       string
	Error        string
	File         string // 成功时的备份文件名（不含目录）
	Size         int64
	RemoteDest   string // rclone 远端目标，空=不推送（M2 D20）
	RemoteStatus string // ok / failed（RemoteDest 启用时；失败不改任务终态，D21）
	RemoteError  string
	Log          *cappedBuffer
	StartedAt    time.Time
	FinishedAt   time.Time
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
	remoteDest := paramString(params, "remoteDest")
	if remoteDest != "" && !backupRemoteDestRe.MatchString(remoteDest) {
		return nil, fmt.Errorf("invalid remoteDest: %q", remoteDest)
	}
	preHook := paramString(params, "preHook")
	if len(preHook) > backupHookMaxLen {
		return nil, fmt.Errorf("preHook too long: %d bytes (max %d)", len(preHook), backupHookMaxLen)
	}

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
		ID:         p.newID(),
		Action:     "backup",
		Name:       name,
		ConfigID:   configID,
		Status:     backupTaskRunning,
		RemoteDest: remoteDest,
		Log:        &cappedBuffer{limit: backupLogLimit},
		StartedAt:  p.now(),
	}
	p.mu.Lock()
	p.tasks[task.ID] = task
	p.pruneTasksLocked()
	p.mu.Unlock()

	go func() {
		defer func() { <-p.sem }()
		defer lock.Unlock()

		// M3 D26-D28：打包前执行数据库热备命令；失败即中止（不一致快照
		// 不如不备），不进 pack、不推送远端、retention 不动
		err := p.runHook(task, preHook)
		var file string
		var size int64
		if err == nil {
			file, size, err = p.pack(task, name, sources, destDir, retention)
		}
		// M2 D18：本地打包成功且有异地目标时追加 rclone 推送；
		// 推送失败不改任务终态（D21），只记 RemoteStatus 供 server 通知
		remoteStatus, remoteErr := "", error(nil)
		if err == nil && remoteDest != "" {
			remoteStatus, remoteErr = p.pushRemote(task, filepath.Join(destDir, file), remoteDest)
		}
		// 终态字段在锁内更新：GetTask 轮询（server 侧）会并发读取
		p.mu.Lock()
		task.FinishedAt = p.now()
		task.File = file
		task.Size = size
		task.RemoteStatus = remoteStatus
		if remoteErr != nil {
			task.RemoteError = remoteErr.Error()
		}
		if err != nil {
			task.Status = backupTaskFailed
			task.Error = err.Error()
			fmt.Fprintf(task.Log, "\n[error] %v\n", err)
		} else {
			task.Status = backupTaskSuccess
		}
		p.mu.Unlock()
	}()

	return map[string]interface{}{"taskId": task.ID, "status": "started"}, nil
}

// GetTask 查询任务状态（server 轮询用）
func (p *BackupProvider) GetTask(taskID string) (map[string]interface{}, error) {
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
		"taskId":       task.ID,
		"action":       task.Action,
		"name":         task.Name,
		"configId":     task.ConfigID,
		"status":       task.Status,
		"error":        task.Error,
		"file":         task.File,
		"size":         task.Size,
		"remoteStatus": task.RemoteStatus,
		"remoteError":  task.RemoteError,
		"log":          task.Log.String(),
		"startedAt":    task.StartedAt.Unix(),
		"finishedAt":   task.FinishedAt.Unix(),
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
// pushRemote rclone copy 推送本地文件到远端（M2 D18/D24 共用）。
// rclone 的失败原因在 stderr，摘要进日志与错误消息（exit status 无解释力）。
func (p *BackupProvider) pushRemote(task *backupTask, localPath, remoteDest string) (string, error) {
	fmt.Fprintf(task.Log, "[remote] rclone copy → %s\n", remoteDest)
	out, err := exec.Command(p.rcloneBin, "copy", "--transfers", "2", localPath, remoteDest).CombinedOutput()
	if summary := strings.TrimSpace(string(out)); summary != "" {
		if len(summary) > 2048 {
			summary = summary[:2048] + "…"
		}
		fmt.Fprintf(task.Log, "[remote] %s\n", summary)
	}
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if len(detail) > 512 {
			detail = detail[:512]
		}
		if detail != "" {
			return "failed", fmt.Errorf("rclone copy: %v: %s", err, detail)
		}
		return "failed", fmt.Errorf("rclone copy: %v", err)
	}
	fmt.Fprintf(task.Log, "[remote] done\n")
	return "ok", nil
}

// runHook 执行打包前置的数据库热备命令（M3 D26-D29）。sh -c 保留重定向与
// 引号语义（cron 写回同为先例，agent 本就是受信控制面）；空命令快速返回。
// 非零退出/超时返回错误，由调用方短路本次任务（D28：不一致快照不如不备）。
func (p *BackupProvider) runHook(task *backupTask, command string) error {
	if command == "" {
		return nil
	}
	fmt.Fprintf(task.Log, "[hook] %s\n", command)
	ctx, cancel := context.WithTimeout(context.Background(), backupHookTimeout)
	defer cancel()

	cmd := hookCmd(command)
	out := &bytes.Buffer{}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pre-hook start: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		killHookGroup(cmd) // 整组杀：CommandContext 只杀 sh，dump 孙进程会变孤儿
		<-done
		return fmt.Errorf("pre-hook timed out after %s", backupHookTimeout)
	}
	if summary := strings.TrimSpace(out.String()); summary != "" {
		if len(summary) > 2048 {
			summary = summary[:2048] + "…"
		}
		fmt.Fprintf(task.Log, "[hook] %s\n", summary)
	}
	if err != nil {
		detail := strings.TrimSpace(out.String())
		if len(detail) > 512 {
			detail = detail[:512]
		}
		if detail != "" {
			return fmt.Errorf("pre-hook failed: %v: %s", err, detail)
		}
		return fmt.Errorf("pre-hook failed: %v", err)
	}
	return nil
}

// SyncRemote 手动补传：把本地已有备份文件推到远端（M2 D24）。同步执行，
// 单文件 copy 在 RPC 超时窗口内；幂等（rclone 对已存在同尺寸文件跳过）。
func (p *BackupProvider) SyncRemote(params map[string]interface{}) (interface{}, error) {
	dir := paramString(params, "dir")
	if !filepath.IsAbs(filepath.Clean(dir)) {
		return nil, fmt.Errorf("dir must be absolute: %q", dir)
	}
	name := paramString(params, "name")
	if !backupFileNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid backup file name: %q", name)
	}
	remoteDest := paramString(params, "remoteDest")
	if !backupRemoteDestRe.MatchString(remoteDest) {
		return nil, fmt.Errorf("invalid remoteDest: %q", remoteDest)
	}
	local := filepath.Join(filepath.Clean(dir), name)
	if info, err := os.Stat(local); err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("backup file not found: %s", name)
	}
	task := &backupTask{Log: &cappedBuffer{limit: backupLogLimit}}
	if _, err := p.pushRemote(task, local, remoteDest); err != nil {
		return nil, err
	}
	return map[string]interface{}{"synced": true}, nil
}

// RcloneAvailable 异地推送的执行前提（capability metadata 感知，M2 D23）
func RcloneAvailable() bool {
	_, err := exec.LookPath("rclone")
	return err == nil
}

// DeleteFile 删除目录内指定备份文件
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

// ============ 恢复（restore） ============

// RunRestore 校验参数后异步解包备份文件到独立目录（M1.5，见 backup-design.md D9/D11）。
// destDir 必须不存在或为空目录——恢复绝不覆盖现有数据，迁移回原位是用户显式动作。
func (p *BackupProvider) RunRestore(params map[string]interface{}) (interface{}, error) {
	dir := paramString(params, "dir")
	name := paramString(params, "name")
	destDir := filepath.Clean(paramString(params, "destDir"))
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("dir must be absolute: %q", dir)
	}
	if !backupFileNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid backup file name: %q", name)
	}
	if !filepath.IsAbs(destDir) || destDir == string(filepath.Separator) {
		return nil, fmt.Errorf("destDir must be an absolute path other than /")
	}
	archive := filepath.Join(dir, name)
	info, err := os.Stat(archive)
	if err != nil {
		return nil, fmt.Errorf("backup file not found: %s", name)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("backup file is not a regular file")
	}
	if entries, err := os.ReadDir(destDir); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("destDir %s exists and is not empty; restore must target an empty directory", destDir)
	}

	// restore 与 backup 分开按名互斥（同 name 前缀），语义互不影响
	lock, ok := p.nameLock("restore:" + name)
	if !ok {
		return nil, fmt.Errorf("restore %s busy", name)
	}
	select {
	case p.sem <- struct{}{}:
	default:
		lock.Unlock()
		return nil, fmt.Errorf("too many concurrent backup tasks")
	}

	task := &backupTask{
		ID:        p.newID(),
		Action:    "restore",
		Name:      name,
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

		err := p.unpack(task, dir, name, destDir)
		// 终态字段在锁内更新：GetTask 轮询会并发读取
		p.mu.Lock()
		task.Size = info.Size()
		task.FinishedAt = p.now()
		task.File = name
		if err != nil {
			task.Status = backupTaskFailed
			task.Error = err.Error()
			fmt.Fprintf(task.Log, "\n[error] %v\n", err)
		} else {
			task.Status = backupTaskSuccess
		}
		p.mu.Unlock()
	}()

	return map[string]interface{}{"taskId": task.ID, "status": "started"}, nil
}

// unpack 把 {dir}/{name} 解包到 destDir。Zip Slip 防护：条目目标必须仍在
// destDir 内；仅处理目录/普通文件/符号链接，其余类型跳过；单条目失败不中断。
func (p *BackupProvider) unpack(task *backupTask, dir, name, destDir string) error {
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	destClean := filepath.Clean(destDir)
	if err := os.MkdirAll(destClean, 0o700); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	fmt.Fprintf(task.Log, "[restore] %s → %s\n", name, destClean)

	count := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		target := filepath.Join(destClean, hdr.Name)
		if target != destClean && !strings.HasPrefix(target, destClean+string(filepath.Separator)) {
			fmt.Fprintf(task.Log, "[warn] skip unsafe entry %q (outside destDir)\n", hdr.Name)
			continue
		}
		if err := p.unpackEntry(tr, hdr, target); err != nil {
			fmt.Fprintf(task.Log, "[warn] entry %q: %v\n", hdr.Name, err)
			continue
		}
		count++
	}
	fmt.Fprintf(task.Log, "[restore] done: %d entries\n", count)
	return nil
}

// unpackEntry 写入单个 tar 条目
func (p *BackupProvider) unpackEntry(tr *tar.Reader, hdr *tar.Header, target string) error {
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, os.FileMode(hdr.Mode)&0o777)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	case tar.TypeSymlink:
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		_ = os.Remove(target) // 空目录内重名链接（理论不可达）兜底
		return os.Symlink(hdr.Linkname, target)
	default:
		return fmt.Errorf("unsupported entry type %q, skipped", string(rune(hdr.Typeflag)))
	}
}

// ============ 分块读取（下载通道） ============

// ReadChunk 读取备份文件的一个字节块并 base64 编码，供 server 流式转发下载。
// 单块上限 backupReadChunkLimit；offset 超出文件长度返回 eof=true 且空数据。
func (p *BackupProvider) ReadChunk(params map[string]interface{}) (interface{}, error) {
	dir := paramString(params, "dir")
	name := paramString(params, "name")
	offset := int64(paramFloat(params, "offset"))
	length := int64(paramFloat(params, "length"))
	if !filepath.IsAbs(filepath.Clean(dir)) {
		return nil, fmt.Errorf("dir must be absolute: %q", dir)
	}
	if !backupFileNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid backup file name: %q", name)
	}
	if length <= 0 || length > backupReadChunkLimit {
		length = backupReadChunkLimit
	}
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("open backup file: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat backup file: %w", err)
	}
	total := info.Size()
	if offset >= total {
		return map[string]interface{}{"data": "", "size": total, "eof": true}, nil
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek: %w", err)
	}
	if remaining := total - offset; length > remaining {
		length = remaining
	}
	buf := make([]byte, length)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("read: %w", err)
	}
	return map[string]interface{}{
		"data": base64.StdEncoding.EncodeToString(buf[:n]),
		"size": total,
		"eof":  offset+int64(n) >= total,
	}, nil
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
