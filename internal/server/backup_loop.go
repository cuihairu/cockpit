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
	backupCheckInterval = time.Minute
	backupTrackTimeout  = 30 * time.Minute // 大目录打包可能很久
	backupMaxRetention  = 365
	backupMaxIntervalH  = 168 // every:Nh 上限一周
)

var (
	// backupTrackInterval 任务终态轮询间隔；var 便于测试缩短等待
	backupTrackInterval = 5 * time.Second
	// backupNameRe 备份名约束（与 agent 侧一致）
	backupNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	// backupFileNameRe 删除文件时的文件名校验（与 agent 侧一致）
	backupFileNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.tar\.gz$`)
)

// ValidBackupName 校验备份名
func ValidBackupName(name string) bool { return backupNameRe.MatchString(name) }

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
		"configId":  float64(cfg.ID),
		"name":      cfg.Name,
		"sources":   sourcesToIface(sources),
		"destDir":   destDir,
		"retention": float64(cfg.Retention),
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
	go s.trackBackupTask(cfg.ID, cfg.AgentID, taskID, run.ID)
	return run.ID, nil
}

// trackBackupTask 轮询 agent 任务状态直到终态/超时/server 关闭，回填历史并通知
func (s *Server) trackBackupTask(configID uint, agentID, taskID string, runID uint) {
	deadline := time.Now().Add(backupTrackTimeout)
	status := "failed"
	var file string
	var size int64
	var errMsg string
	finishedAt := time.Now().Unix()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(backupTrackInterval):
		}

		st, f, sz, e, done := s.pollBackupTask(agentID, taskID)
		if !done {
			if time.Now().After(deadline) {
				status, finishedAt, errMsg = "failed", time.Now().Unix(), "backup task timed out"
			} else {
				continue
			}
		} else {
			status, file, size, errMsg = st, f, sz, e
			finishedAt = time.Now().Unix()
		}

		if run, err := s.db.GetBackupRun(runID); err == nil {
			run.Status = status
			run.File = file
			run.Size = size
			run.Error = errMsg
			run.FinishedAt = finishedAt
			_ = s.db.UpdateBackupRun(run)
		}
		if cfg, err := s.db.GetBackupConfig(configID); err == nil {
			cfg.LastStatus = status
			_ = s.db.UpdateBackupConfig(cfg)
			if status == "failed" {
				s.notifyBackupFailed(cfg, errMsg)
			}
		}
		log.Printf("Backup task finished: config=%d task=%s status=%s file=%s size=%d",
			configID, taskID, status, file, size)
		return
	}
}

// pollBackupTask 查询一次任务状态，返回 (status, file, size, err, 是否终态)。
// agent 离线保持非终态等待重连；unknown task（agent 重启丢内存态）视为 failed。
func (s *Server) pollBackupTask(agentID, taskID string) (string, string, int64, string, bool) {
	resp, err := s.CallAgent(agentID, "backup.task.get", map[string]interface{}{"taskId": taskID})
	if err == ErrAgentNotFound {
		// agent 离线：任务态未知，保持 running 等待重连
		return "", "", 0, "", false
	}
	if err != nil {
		return "failed", "", 0, truncateErr(err), true
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		return "", "", 0, "", false
	}
	if rpcResp.Status == "error" {
		if strings.Contains(rpcResp.Error, "not found") {
			return "failed", "", 0, "task lost on agent", true
		}
		return "", "", 0, "", false
	}
	data, ok := rpcResp.Data.(map[string]interface{})
	if !ok {
		return "", "", 0, "", false
	}
	switch mapString(data, "status") {
	case "success":
		return "success", mapString(data, "file"), mapInt64(data, "size"), "", true
	case "failed":
		errMsg := mapString(data, "error")
		if errMsg == "" {
			errMsg = "backup failed on agent"
		}
		return "failed", mapString(data, "file"), mapInt64(data, "size"), errMsg, true
	default: // running / 未知状态
		return "", "", 0, "", false
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
