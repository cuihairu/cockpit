package storage

import (
	"time"
)

// 执行 Job 模型（设计见 docs/guide/jobs-design.md）。
//
// Job 是控制面的统一执行抽象：任何「点击执行一个运维动作」都落成一条
// 持久化的 Job 记录。type 决定动作语义（agent.exec 为第一个内置类型），
// target 为执行目标 agent，Parameters 为 JSON 化的入参。状态机：
//
//	pending → running → success | failed
//
// 输出与退出码在完成后回写；Parameters/Output 只存尾部，防表膨胀
// （输出上限 JobMaxOutput）。
const (
	JobStatusPending = "pending"
	JobStatusRunning = "running"
	JobStatusSuccess = "success"
	JobStatusFailed  = "failed"

	// JobMaxOutput 输出保留上限（字节）：执行输出只留尾部
	JobMaxOutput = 64 * 1024
)

// Job 一次执行的持久化记录
type Job struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	Type       string     `gorm:"index" json:"type"`
	Target     string     `gorm:"index" json:"target"` // agent id
	Actor      string     `json:"actor"`
	Parameters string     `json:"-"`    // JSON 串入参（读取时在 API 层解析回对象）
	Status     string     `gorm:"index" json:"status"`
	ExitCode   int        `json:"exitCode,omitempty"`
	Output     string     `json:"output,omitempty"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// CreateJob 新建 Job（调用方负责填充 ID/Type/Target/Actor/Status）
func (d *DB) CreateJob(job *Job) error {
	return d.db.Create(job).Error
}

// GetJob 按 ID 取单条
func (d *DB) GetJob(id string) (*Job, error) {
	var job Job
	if err := d.db.First(&job, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// ListJobs 最近 limit 条（时间倒序，新在前）
func (d *DB) ListJobs(limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	var jobs []Job
	if err := d.db.Order("created_at desc").Limit(limit).Find(&jobs).Error; err != nil {
		return nil, err
	}
	return jobs, nil
}

// UpdateJob 回写执行结果（状态/退出码/输出/错误/时间戳）
func (d *DB) UpdateJob(job *Job) error {
	return d.db.Model(job).Save(job).Error
}