package server

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ 备份调度循环 ============
//
// server 是唯一全局时钟：每分钟扫描到期的启用配置，下发 backup.run 给
// agent，并跟踪任务终态回填 BackupRun（见 docs/guide/backup-design.md D3）。

const (
	backupMaxRetention = 365
	backupMaxIntervalH = 168 // every:Nh 上限一周
)

var (
	// backupCheckInterval 调度扫描周期；包级变量仅为测试可注入，
	// 默认值即生产取值
	backupCheckInterval = time.Minute
	// backupTrackTimeout 任务跟踪总时限（大目录打包可能很久）；
	// 包级变量仅为测试可注入，默认值即生产取值
	backupTrackTimeout = 30 * time.Minute
	// backupTrackInterval 任务终态轮询间隔；var 便于测试缩短等待
	backupTrackInterval = 5 * time.Second
	// backupNameRe 备份名约束（与 agent 侧一致）
	backupNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	// backupRemoteDestRe rclone 远端目标 remote:path（M2 D22，与 agent 侧同规则：
	// remote 名语法上不以 - 开头防 flag 混淆，path 段禁空白与控制字符）
	backupRemoteDestRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*:[^\x00-\x20\x7f]+$`)
	// backupFileNameRe 删除文件时的文件名校验（与 agent 侧一致）
	backupFileNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.tar\.gz$`)
)

// ValidBackupName 校验备份名
func ValidBackupName(name string) bool { return backupNameRe.MatchString(name) }

// ValidBackupRemoteDest 校验 rclone 远端目标（M2 D22；空值由调用方按
// 「不启用异地」处理）
func ValidBackupRemoteDest(dest string) bool { return backupRemoteDestRe.MatchString(dest) }

// ValidBackupSchedule 校验 schedule 形态：manual / daily@HH:mm / every:Nh
func ValidBackupSchedule(s string) bool {
	switch {
	case s == "manual":
		return true
	case strings.HasPrefix(s, "daily@"):
		parts := strings.SplitN(strings.TrimPrefix(s, "daily@"), ":", 2)
		if len(parts) != 2 {
			return false
		}
		h, err1 := strconv.Atoi(parts[0])
		m, err2 := strconv.Atoi(parts[1])
		return err1 == nil && err2 == nil && h >= 0 && h <= 23 && m >= 0 && m <= 59 && len(parts[0]) <= 2 && len(parts[1]) <= 2
	case strings.HasPrefix(s, "every:"):
		raw := strings.TrimSuffix(strings.TrimPrefix(s, "every:"), "h")
		n, err := strconv.Atoi(raw)
		return err == nil && n >= 1 && n <= backupMaxIntervalH && raw != ""
	default:
		return false
	}
}

// ValidBackupSources 校验源路径列表：非空且全为绝对路径
func ValidBackupSources(sources []string) bool {
	if len(sources) == 0 {
		return false
	}
	for _, s := range sources {
		if !strings.HasPrefix(s, "/") {
			return false
		}
	}
	return true
}

// ParseBackupSources 解析配置里的 sources JSON
func ParseBackupSources(raw string) []string {
	var out []string
	if raw == "" {
		return out
	}
	json.Unmarshal([]byte(raw), &out)
	return out
}

// NextBackupRunAt 计算下次运行时间（server 本地时区）；manual 返回 0
func NextBackupRunAt(schedule string, now time.Time) int64 {
	switch {
	case schedule == "manual":
		return 0
	case strings.HasPrefix(schedule, "daily@"):
		parts := strings.SplitN(strings.TrimPrefix(schedule, "daily@"), ":", 2)
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		next := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		return next.Unix()
	case strings.HasPrefix(schedule, "every:"):
		raw := strings.TrimSuffix(strings.TrimPrefix(schedule, "every:"), "h")
		n, _ := strconv.Atoi(raw)
		return now.Add(time.Duration(n) * time.Hour).Unix()
	default:
		return 0
	}
}

// startBackupLoop 启动备份调度循环
func (s *Server) startBackupLoop() {
	go func() {
		ticker := time.NewTicker(backupCheckInterval)
		defer ticker.Stop()

		s.recoverOrphanBackupRuns()
		for {
			select {
			case <-ticker.C:
				s.dispatchDueBackups()
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

// recoverOrphanBackupRuns server 重启后把遗留的 running 记录标记为失败
// （任务态已丢失，继续等待只会永远 running）
func (s *Server) recoverOrphanBackupRuns() {
	runs, err := s.db.RunningBackupRuns()
	if err != nil {
		return
	}
	for _, run := range runs {
		run.Status = "failed"
		run.Error = "server restarted during backup"
		run.FinishedAt = time.Now().Unix()
		if err := s.db.UpdateBackupRun(run); err != nil {
			log.Printf("recover backup run %d: %v", run.ID, err)
			continue
		}
		if cfg, err := s.db.GetBackupConfig(run.ConfigID); err == nil && cfg.LastStatus == "running" {
			cfg.LastStatus = "failed"
			_ = s.db.UpdateBackupConfig(cfg)
		}
	}
	if len(runs) > 0 {
		log.Printf("Recovered %d orphan backup runs", len(runs))
	}
}

// dispatchDueBackups 扫描到期配置并下发
func (s *Server) dispatchDueBackups() {
	due, err := s.db.DueBackupConfigs(time.Now().Unix())
	if err != nil {
		log.Printf("list due backups: %v", err)
		return
	}
	for _, cfg := range due {
		// 先推进下次运行时间，防执行失败后每分钟重试打爆 agent
		cfg.NextRunAt = NextBackupRunAt(cfg.Schedule, time.Now())
		if err := s.db.UpdateBackupConfig(cfg); err != nil {
			log.Printf("advance next_run_at for config %d: %v", cfg.ID, err)
			continue
		}
		if _, err := s.startBackupRun(cfg, "scheduled"); err != nil {
			log.Printf("dispatch backup config %d (%s): %v", cfg.ID, cfg.Name, err)
		}
	}
}

// startBackupRun 下发一次备份运行（调度与手动共用）。返回运行记录 ID。
func (s *Server) startBackupRun(cfg *storage.BackupConfig, trigger string) (uint, error) {
	if cfg.LastStatus == "running" {
		return 0, fmt.Errorf("backup %s already running", cfg.Name)
	}
	// server 侧同样校验（agent 侧有第二道防线）
	if !ValidBackupName(cfg.Name) {
		return 0, fmt.Errorf("invalid backup name: %q", cfg.Name)
	}
	destDir := cfg.DestDir
	if destDir == "" {
		return 0, fmt.Errorf("destDir required")
	}
	sources := ParseBackupSources(cfg.Sources)

	now := time.Now()
	run := &storage.BackupRun{
		ConfigID:  cfg.ID,
		Status:    "running",
		StartedAt: now.Unix(),
	}
	if err := s.db.CreateBackupRun(run); err != nil {
		return 0, fmt.Errorf("create run record: %w", err)
	}

	// agent 离线等下发失败：记录失败终态，不置 running
	resp, err := s.CallAgent(cfg.AgentID, "backup.run", map[string]interface{}{
		"configId":   float64(cfg.ID),
		"name":       cfg.Name,
		"sources":    sourcesToIface(sources),
		"destDir":    destDir,
		"retention":  float64(cfg.Retention),
		"remoteDest": cfg.RemoteDest, // 空=不推送（M2 D18）
		"preHook":    cfg.PreHook,    // 空=不执行（M3 D26）
	})
	if err != nil {
		run.Status = "failed"
		run.Error = truncateErr(err)
		run.FinishedAt = now.Unix()
		_ = s.db.UpdateBackupRun(run)
		cfg.LastStatus = "failed"
		cfg.LastRunAt = now.Unix()
		_ = s.db.UpdateBackupConfig(cfg)
		s.notifyBackupFailed(cfg, run.Error)
		return run.ID, fmt.Errorf("dispatch to agent: %w", err)
	}

	taskID := extractTaskID(resp)
	run.TaskID = taskID
	_ = s.db.UpdateBackupRun(run)

	cfg.LastStatus = "running"
	cfg.LastRunAt = now.Unix()
	_ = s.db.UpdateBackupConfig(cfg)

	log.Printf("Backup dispatched: config=%d name=%s agent=%s task=%s trigger=%s",
		cfg.ID, cfg.Name, cfg.AgentID, taskID, trigger)
	s.backupTrackWG.Add(1)
	go func() {
		defer s.backupTrackWG.Done()
		s.trackBackupTask(cfg.ID, cfg.AgentID, taskID, run.ID)
	}()
	return run.ID, nil
}

// backupTaskResult agent 任务终态快照（M2 扩 Remote*：rclone 推送结果）
type backupTaskResult struct {
	Status       string
	File         string
	Size         int64
	Error        string
	RemoteStatus string // ok / failed（RemoteDest 未启用为空）
	RemoteError  string
}

// trackBackupTask 轮询 agent 任务状态直到终态/超时/server 关闭，回填历史并通知
func (s *Server) trackBackupTask(configID uint, agentID, taskID string, runID uint) {
	deadline := time.Now().Add(backupTrackTimeout)
	// 轮询间隔启动时读到局部量：后台 goroutine 不再反复读包级变量，
	// 测试注入/恢复默认值与之并发安全（退出由 backupTrackWG 可观测）
	poll := backupTrackInterval
	result := backupTaskResult{Status: "failed"}
	finishedAt := time.Now().Unix()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(poll):
		}

		res, done := s.pollBackupTask(agentID, taskID)
		if !done {
			if time.Now().After(deadline) {
				result.Status, finishedAt, result.Error = "failed", time.Now().Unix(), "backup task timed out"
			} else {
				continue
			}
		} else {
			result = res
			finishedAt = time.Now().Unix()
		}

		if run, err := s.db.GetBackupRun(runID); err == nil {
			run.Status = result.Status
			run.File = result.File
			run.Size = result.Size
			run.Error = result.Error
			run.RemoteStatus = result.RemoteStatus
			run.RemoteError = result.RemoteError
			run.FinishedAt = finishedAt
			_ = s.db.UpdateBackupRun(run)
		}
		if cfg, err := s.db.GetBackupConfig(configID); err == nil {
			cfg.LastStatus = result.Status
			_ = s.db.UpdateBackupConfig(cfg)
			if result.Status == "failed" {
				s.notifyBackupFailed(cfg, result.Error)
			}
			// M2 D21：本地成功但异地推送失败 → 独立通知，不改任务终态
			if result.Status == "success" && result.RemoteStatus == "failed" {
				s.notifyBackupRemoteFailed(cfg, result.RemoteError)
			}
		}
		log.Printf("Backup task finished: config=%d task=%s status=%s file=%s size=%d remote=%s",
			configID, taskID, result.Status, result.File, result.Size, result.RemoteStatus)
		return
	}
}

// pollBackupTask 查询一次任务状态，返回 (终态快照, 是否终态)。
// agent 离线保持非终态等待重连；unknown task（agent 重启丢内存态）视为 failed。
func (s *Server) pollBackupTask(agentID, taskID string) (backupTaskResult, bool) {
	resp, err := s.CallAgent(agentID, "backup.task.get", map[string]interface{}{"taskId": taskID})
	if err == ErrAgentNotFound {
		// agent 离线：任务态未知，保持 running 等待重连
		return backupTaskResult{}, false
	}
	if err != nil {
		return backupTaskResult{Status: "failed", Error: truncateErr(err)}, true
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		return backupTaskResult{}, false
	}
	if rpcResp.Status == "error" {
		if strings.Contains(rpcResp.Error, "not found") {
			return backupTaskResult{Status: "failed", Error: "task lost on agent"}, true
		}
		return backupTaskResult{}, false
	}
	data, ok := rpcResp.Data.(map[string]interface{})
	if !ok {
		return backupTaskResult{}, false
	}
	switch mapString(data, "status") {
	case "success":
		return backupTaskResult{
			Status:       "success",
			File:         mapString(data, "file"),
			Size:         mapInt64(data, "size"),
			RemoteStatus: mapString(data, "remoteStatus"),
			RemoteError:  mapString(data, "remoteError"),
		}, true
	case "failed":
		errMsg := mapString(data, "error")
		if errMsg == "" {
			errMsg = "backup failed on agent"
		}
		return backupTaskResult{
			Status: "failed",
			File:   mapString(data, "file"),
			Size:   mapInt64(data, "size"),
			Error:  errMsg,
		}, true
	default: // running / 未知状态
		return backupTaskResult{}, false
	}
}

// notifyBackupFailed 备份失败通知（走事件白名单，未启用不发送）
func (s *Server) notifyBackupFailed(cfg *storage.BackupConfig, errMsg string) {
	s.notifier.SendNonBlocking(&notification.Notification{
		EventType:    notification.BackupFailed,
		Title:        fmt.Sprintf("备份失败: %s", cfg.Name),
		Message:      errMsg,
		Level:        "error",
		ResourceType: "backup",
		ResourceID:   fmt.Sprintf("%d", cfg.ID),
		Time:         time.Now(),
	})
}

// notifyBackupRemoteFailed 异地推送失败通知（M2 D21：本地备份已成功，
// 仅远端链路断；走独立事件白名单）
func (s *Server) notifyBackupRemoteFailed(cfg *storage.BackupConfig, errMsg string) {
	if errMsg == "" {
		errMsg = "rclone copy failed"
	}
	s.notifier.SendNonBlocking(&notification.Notification{
		EventType:    notification.BackupRemoteFailed,
		Title:        fmt.Sprintf("备份异地推送失败: %s", cfg.Name),
		Message:      errMsg,
		Level:        "error",
		ResourceType: "backup",
		ResourceID:   fmt.Sprintf("%d", cfg.ID),
		Time:         time.Now(),
	})
}

// ============ helpers ============

func sourcesToIface(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func extractTaskID(resp *protocol.Message) string {
	if resp == nil {
		return ""
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		return ""
	}
	m, ok := rpcResp.Data.(map[string]interface{})
	if !ok {
		return ""
	}
	return mapString(m, "taskId")
}

func truncateErr(err error) string {
	s := err.Error()
	if len(s) > 480 {
		s = s[:480]
	}
	return s
}
