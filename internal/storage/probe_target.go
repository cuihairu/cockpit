package storage

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ProbeTargetSnapshot 探针 agent（cockpit-probe-agent）上报的目标观测
// 快照（服务检测 agent B5）：每 agent+target 一行 upsert，回查「现在各
// 目标什么状态」。与拨测历史的 ProbeResult（server 侧逐轮记录）互补：
// 本表是 agent 侧权威快照的最新值。
type ProbeTargetSnapshot struct {
	ID      string `gorm:"primaryKey" json:"id"`
	AgentID string `gorm:"uniqueIndex:idx_probe_target_snapshot,priority:1" json:"agentId"`
	Target  string `gorm:"uniqueIndex:idx_probe_target_snapshot,priority:2" json:"target"`
	// State 三态：unknown（冷区）/ healthy / faulty（与 core/healthprobe 一致）
	State       string    `gorm:"index" json:"state"`
	Since       time.Time `json:"since"`
	LastChecked time.Time `json:"lastChecked"`
	LastError   string    `json:"lastError"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// BeforeCreate GORM hook
func (p *ProbeTargetSnapshot) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

// ProbeWindow 探针 agent 上报的故障窗口（回查「什么时间段不可用」，
// croupier system 角色探针口径）：(agent,target,started_at) 唯一 upsert，
// 关窗上报把 EndedAt 从 nil 刷成时刻。保留期外清理同 ProbeResult（D10）。
type ProbeWindow struct {
	ID        string     `gorm:"primaryKey" json:"id"`
	AgentID   string     `gorm:"index:idx_probe_window_agent,priority:1;uniqueIndex:idx_probe_window_key,priority:1" json:"agentId"`
	Target    string     `gorm:"index:idx_probe_window_agent,priority:2;uniqueIndex:idx_probe_window_key,priority:2" json:"target"`
	StartedAt time.Time  `gorm:"index:idx_probe_window_agent,priority:3;uniqueIndex:idx_probe_window_key,priority:3" json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt"`
	LastError string     `json:"lastError"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// BeforeCreate GORM hook
func (p *ProbeWindow) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

// UpsertProbeTargets 批量 upsert 目标快照（同 agent+target 覆盖）。
func (d *DB) UpsertProbeTargets(agentID string, rows []ProbeTargetSnapshot) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now()
	for i := range rows {
		rows[i].AgentID = agentID
		rows[i].UpdatedAt = now
	}
	return d.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "agent_id"}, {Name: "target"}},
		DoUpdates: clause.AssignmentColumns([]string{"state", "since", "last_checked", "last_error", "updated_at"}),
	}).Create(&rows).Error
}

// UpsertProbeWindows 批量 upsert 故障窗口（同 agent+target+started_at 覆盖；
// 重复上报幂等，关窗上报刷新 ended_at）。
func (d *DB) UpsertProbeWindows(agentID string, rows []ProbeWindow) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now()
	for i := range rows {
		rows[i].AgentID = agentID
		rows[i].UpdatedAt = now
	}
	return d.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "agent_id"}, {Name: "target"}, {Name: "started_at"}},
		DoUpdates: clause.AssignmentColumns([]string{"ended_at", "last_error", "updated_at"}),
	}).Create(&rows).Error
}

// ListProbeTargets 全部目标快照（面板回查，agent/target 序稳定）。
func (d *DB) ListProbeTargets() ([]*ProbeTargetSnapshot, error) {
	var rows []*ProbeTargetSnapshot
	err := d.db.Order("agent_id, target").Find(&rows).Error
	return rows, err
}

// ListProbeWindows 故障窗口回查：agent/target 可选过滤，started_at 倒序。
func (d *DB) ListProbeWindows(agentID, target string, limit int) ([]*ProbeWindow, error) {
	if limit <= 0 {
		limit = 50
	}
	q := d.db.Model(&ProbeWindow{})
	if agentID != "" {
		q = q.Where("agent_id = ?", agentID)
	}
	if target != "" {
		q = q.Where("target = ?", target)
	}
	var rows []*ProbeWindow
	err := q.Order("started_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

// DeleteProbeWindowsOlderThan 清理保留期外窗口（与 ProbeResult 同 D10 节奏）。
func (d *DB) DeleteProbeWindowsOlderThan(cutoff time.Time) (int64, error) {
	res := d.db.Where("started_at < ?", cutoff).Delete(&ProbeWindow{})
	return res.RowsAffected, res.Error
}
