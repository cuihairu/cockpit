package server

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 远控终端会话录制（见 docs/guide/recording-design.md）：server 侧挂在
// agent→浏览器的转发管道上（HandleTerminalData），asciinema v2 格式落盘，
// 元数据入 TerminalRecording 表。agent 零变更。只录输出（D4：输入含密码）。
// M2：录制结束异步 rclone 归档异地（remote:path），失败只通知不改本地行为。

// Setting 键：开关、保留天数与异地归档目标（每轮读取免缓存）
const (
	RecordingEnabledSettingKey    = "recording.enabled"
	RecordingRetentionSettingKey  = "recording.retention_days"
	RecordingRemoteDestSettingKey = "recording.remote_dest"
)

// sessionID 严格形态（uuid.NewString()）：cast 文件按 sid 寻址，防穿越
var recordingSessionIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

const (
	recordingDefaultRetentionDays = 7
	recordingMaxRetentionDays     = 365
	recordingCleanupInterval      = time.Hour // 清理执行节流（挂 30s cleanupLoop）
)

// recordingRemoteDest 异地归档目标；空=关闭。脏值（写入口之后被手改）
// 视为未配置并记日志（与 server 备份 M2 D13 同规则同源正则）。
func (s *Server) recordingRemoteDest() string {
	v, err := s.db.GetSetting(RecordingRemoteDestSettingKey)
	if err != nil || v == "" {
		return ""
	}
	if len(v) > serverBackupRemoteDestMaxLen ||
		!regexp.MustCompile(serverBackupRemoteDestRe).MatchString(v) {
		log.Printf("Recording remote_dest invalid, treating as unset: %q", v)
		return ""
	}
	return v
}

// recordingExt 按录制内容形态定文件后缀（M3 D4）：guac = Guacamole 会话流，
// 其余（含空值）= asciinema v2 .cast（兼容 M1/M2 旧数据）。
func recordingExt(format string) string {
	if format == "guac" {
		return ".guac"
	}
	return ".cast"
}

// pushRecordingRemote 录制结束后的异地归档（M2 D16 异步调用：不拖会话
// 出口路径）。remote_dest 未配置时静默跳过；失败发 recording.remote-failed。
// 后缀按录制形态（M3 D4：.cast / .guac 同目录同索引，仅内容形态不同）。
func (s *Server) pushRecordingRemote(sessionID string) {
	remoteDest := s.recordingRemoteDest()
	if remoteDest == "" {
		return
	}
	ext := ".cast"
	if rec, err := s.db.GetTerminalRecording(sessionID); err == nil && rec != nil {
		ext = recordingExt(rec.Format)
	}
	local := filepath.Join(s.recordingsDir(), sessionID+ext)
	log.Printf("[remote] rclone copy %s%s → %s", sessionID, ext, remoteDest)
	if err := rcloneCopyLocalFile(local, remoteDest); err != nil {
		log.Printf("[remote] recording archive push failed: %v", err)
		if s.notifier != nil {
			s.notifier.SendNonBlocking(&notification.Notification{
				EventType:    notification.RecordingRemoteFailed,
				Title:        "会话录制异地归档失败",
				Message:      fmt.Sprintf("%s%s: %v", sessionID, ext, err),
				Level:        "warning",
				ResourceType: "recording",
				ResourceID:   sessionID,
				Time:         time.Now(),
			})
		}
		return
	}
	log.Printf("[remote] recording archived: %s%s", sessionID, ext)
}

// recordingEnabled 录制开关，默认开启
func (s *Server) recordingEnabled() bool {
	v, err := s.db.GetSetting(RecordingEnabledSettingKey)
	return err != nil || v != "false"
}

// recordingRetentionDays 保留天数；0=永久；非法回默认
func (s *Server) recordingRetentionDays() int {
	v, err := s.db.GetSetting(RecordingRetentionSettingKey)
	if err != nil || v == "" {
		return recordingDefaultRetentionDays
	}
	n := 0
	if _, convErr := fmt.Sscanf(v, "%d", &n); convErr != nil {
		return recordingDefaultRetentionDays
	}
	if n < 0 {
		n = 0
	}
	if n > recordingMaxRetentionDays {
		n = recordingMaxRetentionDays
	}
	return n
}

// recordingsDir 录制文件目录：数据库同目录下 recordings/
func (s *Server) recordingsDir() string {
	dbPath := "./data/cockpit.db"
	if s.cfg != nil && s.cfg.Database.Path != "" {
		dbPath = s.cfg.Database.Path
	}
	return filepath.Join(filepath.Dir(dbPath), "recordings")
}

// castHeader asciinema v2 头（D12：固定 80x24，回放端自适应）
type castHeader struct {
	Version   int               `json:"version"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Timestamp int64             `json:"timestamp"`
	Title     string            `json:"title,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// castRecorder 单会话录制器：追加写 [dt,"o",data] 行，Close 幂等（D8）
type castRecorder struct {
	mu        sync.Mutex
	once      sync.Once
	f         *os.File
	start     time.Time
	bytes     int64
	sessionID string
	finish    func(durationMs, bytes int64) // 回填 DB，Close 时调用一次
}

// newCastRecorder 创建录制文件并写 header；目录不存在则建
func newCastRecorder(path, sessionID, title string, startedAt time.Time) (*castRecorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create recordings dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("create cast file: %w", err)
	}
	header := castHeader{
		Version:   2,
		Width:     80,
		Height:    24,
		Timestamp: startedAt.Unix(),
		Title:     title,
		Env:       map[string]string{"TERM": "xterm-256color"},
	}
	raw, _ := json.Marshal(header)
	if _, err := f.Write(append(raw, '\n')); err != nil {
		f.Close()
		return nil, fmt.Errorf("write cast header: %w", err)
	}
	return &castRecorder{
		f:         f,
		start:     startedAt,
		bytes:     int64(len(raw) + 1),
		sessionID: sessionID,
	}, nil
}

// WriteOutput 记一条输出事件（nil receiver 安全）；写失败停录不影响转发
func (r *castRecorder) WriteOutput(data []byte) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return
	}
	dt := time.Since(r.start).Seconds()
	// 标量切片 Marshal 恒成功
	line, _ := json.Marshal([]interface{}{dt, "o", string(data)})
	line = append(line, '\n')
	n, err := r.f.Write(line)
	r.bytes += int64(n)
	if err != nil {
		// 磁盘满/文件系统错误：停录，远控照常（D7）
		log.Printf("Recording %s stopped: write: %v", r.sessionID, err)
		r.f.Close()
		r.f = nil
	}
}

// Close 结束录制并回填元数据；幂等，两个会话出口都可安全调用（D8）
func (r *castRecorder) Close() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		r.mu.Lock()
		if r.f != nil {
			r.f.Close()
			r.f = nil
		}
		r.mu.Unlock()
		if r.finish != nil {
			r.finish(time.Since(r.start).Milliseconds(), r.bytes)
		}
	})
}

// startRecording 为终端会话开启录制：建文件 + 登记元数据
func (s *Server) startRecording(session *TerminalSession) *castRecorder {
	path := filepath.Join(s.recordingsDir(), session.ID+".cast")
	title := fmt.Sprintf("cockpit %s@%s", session.Username, session.Host)
	rec, err := newCastRecorder(path, session.ID, title, session.CreatedAt)
	if err != nil {
		log.Printf("Recording start failed for session %s: %v", session.ID, err)
		return nil
	}
	meta := &storage.TerminalRecording{
		SessionID: session.ID,
		Username:  session.Username,
		AgentID:   session.AgentID,
		Host:      session.Host,
		Port:      session.Port,
		Protocol:  string(session.Protocol),
		StartedAt: session.CreatedAt,
	}
	if err := s.db.CreateTerminalRecording(meta); err != nil {
		log.Printf("Recording meta insert failed for session %s: %v", session.ID, err)
		rec.Close()
		return nil
	}
	rec.finish = func(durationMs, bytes int64) {
		if err := s.db.FinishTerminalRecording(session.ID, durationMs, bytes); err != nil {
			log.Printf("Recording finish failed for session %s: %v", session.ID, err)
		}
		// M2 D16：回填后异步归档，不拖会话出口路径（.cast 已关闭完整可推）
		go s.pushRecordingRemote(session.ID)
	}
	return rec
}

// cleanupExpiredRecordings 按保留天数清理过期录制（文件 + 记录）
func (s *Server) cleanupExpiredRecordings() {
	days := s.recordingRetentionDays()
	if days <= 0 {
		return // 永久保留
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	expired, err := s.db.ListExpiredTerminalRecordings(cutoff)
	if err != nil {
		log.Printf("List expired recordings failed: %v", err)
		return
	}
	for _, rec := range expired {
		path := filepath.Join(s.recordingsDir(), rec.SessionID+recordingExt(rec.Format))
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("Remove recording file %s failed: %v", path, err)
			continue
		}
		if err := s.db.DeleteTerminalRecording(rec.SessionID); err != nil {
			log.Printf("Delete recording meta %s failed: %v", rec.SessionID, err)
		}
	}
	if len(expired) > 0 {
		log.Printf("Cleaned up %d expired terminal recordings", len(expired))
	}
}
