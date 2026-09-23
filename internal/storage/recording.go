package storage

import (
	"time"
)

// ============ TerminalRecording 远控终端会话录制 ============
//
// 元数据索引，录制内容（asciinema v2 .cast）在 server 本地
// data/recordings/<session_id>.cast；会话开始时插入，结束时回填
// duration_ms / bytes。见 docs/guide/recording-design.md。

// TerminalRecording 终端会话录制元数据
type TerminalRecording struct {
	ID        uint   `gorm:"primarykey" json:"id"`
	SessionID string `gorm:"uniqueIndex;size:64" json:"sessionId"`
	Username  string `gorm:"index;size:64" json:"username"`
	AgentID   string `gorm:"size:64" json:"agentId"`
	Host      string `gorm:"size:255" json:"host"`
	Port      int    `json:"port"`
	Protocol  string `gorm:"size:16" json:"protocol"`
	// Format 录制内容形态：cast（asciinema v2 终端流，默认，空值兼容旧数据）
	// 或 guac（Guacamole 会话流，guacd 落盘后收集，见 todo.md M3 D1）
	Format     string    `gorm:"size:8" json:"format"`
	StartedAt  time.Time `json:"startedAt"`
	DurationMs int64     `json:"durationMs"` // 结束时回填，0 = 进行中
	Bytes      int64     `json:"bytes"`      // 结束时回填
}

// CreateTerminalRecording 会话开始时登记录制元数据
func (d *DB) CreateTerminalRecording(rec *TerminalRecording) error {
	return d.db.Create(rec).Error
}

// FinishTerminalRecording 结束时回填时长与字节数
func (d *DB) FinishTerminalRecording(sessionID string, durationMs, bytes int64) error {
	return d.db.Model(&TerminalRecording{}).Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{"duration_ms": durationMs, "bytes": bytes}).Error
}

// ListTerminalRecordings 倒序取录制列表（进行中的也在列）
func (d *DB) ListTerminalRecordings(limit int) ([]*TerminalRecording, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var list []*TerminalRecording
	err := d.db.Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}

// GetTerminalRecording 按 session ID 取单条
func (d *DB) GetTerminalRecording(sessionID string) (*TerminalRecording, error) {
	var rec TerminalRecording
	if err := d.db.Where("session_id = ?", sessionID).First(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// DeleteTerminalRecording 删除录制元数据（文件由调用方删除）
func (d *DB) DeleteTerminalRecording(sessionID string) error {
	return d.db.Where("session_id = ?", sessionID).Delete(&TerminalRecording{}).Error
}

// ListExpiredTerminalRecordings 取 started_at 早于 cutoff 的录制
// （retention 清理用；进行中但超时的会话同样回收——文件已无活跃写入方）
func (d *DB) ListExpiredTerminalRecordings(cutoff time.Time) ([]*TerminalRecording, error) {
	var list []*TerminalRecording
	err := d.db.Where("started_at < ?", cutoff).Find(&list).Error
	return list, err
}
