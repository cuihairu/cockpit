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
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusSuccess   = "success"
	JobStatusFailed    = "failed"
	JobStatusCancelled = "cancelled"

	// JobMaxOutput 输出保留上限（字节）：执行输出只留尾部
	JobMaxOutput = 64 * 1024
)

// Job 一次执行的持久化记录
type Job struct {
	ID         string `gorm:"primaryKey" json:"id"`
	Type       string `gorm:"index" json:"type"`
	Target     string `gorm:"index" json:"target"` // agent id
	Actor      string `json:"actor"`
	Parameters string `json:"-"` // JSON 串入参（读取时在 API 层解析回对象）
	Status     string `gorm:"index" json:"status"`
	ExitCode   int    `json:"exitCode,omitempty"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	// WorkflowRunID 所属 Workflow run（空 = 独立 Job）；workflow-design W5
	WorkflowRunID string     `gorm:"index;default:''" json:"workflowRunId,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
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

// CancelJob 条件终态回写 cancelled（C6）：仅当当前状态匹配 fromStatus 时生效，
// 返回实际命中行数。调用方不再依赖先查后改的两步（check-then-update）——
// pending→running 翻转竞态在 UPDATE 内闭合。
func (d *DB) CancelJob(id, fromStatus string, finished time.Time) (int64, error) {
	res := d.db.Model(&Job{}).Where("id = ? AND status = ?", id, fromStatus).
		Updates(map[string]interface{}{"status": JobStatusCancelled, "finished_at": finished})
	return res.RowsAffected, res.Error
}

// FailJob 条件终态回写 failed（孤儿清扫）：仅当当前状态匹配 fromStatus 时
// 生效，错误摘要与终态一并落账。server 重启清扫遗留 pending/running 行时
// 使用，与 CancelJob 同款条件 UPDATE——恢复扫描绝不改写已落终态的行。
func (d *DB) FailJob(id, fromStatus string, finished time.Time, errMsg string) (int64, error) {
	res := d.db.Model(&Job{}).Where("id = ? AND status = ?", id, fromStatus).
		Updates(map[string]interface{}{"status": JobStatusFailed, "error": errMsg, "finished_at": finished})
	return res.RowsAffected, res.Error
}

// MarkJobCancelledOutput 在 Job 已落 cancelled 终态时仅回写执行结果（C7 守卫
// 的终态侧）：exec 被击杀后 dispatchJobAsync 会收到 killed 结果，output/
// exit_code/error 需补进台账，但 status 必须保持 cancelled。
func (d *DB) MarkJobCancelledOutput(id, output, errMsg string, exitCode int) error {
	return d.db.Model(&Job{}).Where("id = ? AND status = ?", id, JobStatusCancelled).
		Updates(map[string]interface{}{"output": output, "exit_code": exitCode, "error": errMsg}).Error
}
