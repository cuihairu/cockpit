package storage

import (
	"time"
)

// 审批门（workflow-design M2d）：高危 Job 提交后先落审批记录，待批准才派发。
//
// 等待态落库不落 goroutine——M2b 立项时「等待态 goroutine 不可复活」是审批门
// 的阻塞点；SQLite 行在 server 重启后仍待批，由人显式批准推进，无孤儿可言。
// 与 jobs/workflow_runs 解耦：审批未过不建 Job 行（审计上「提交」与「执行」
// 两笔可分），批准后回写 JobID 供双向对账。

const (
	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
	ApprovalStatusExpired  = "expired"
)

// Approval 一条待决/已决审批。
type Approval struct {
	// ID 审批唯一标识。
	ID string `gorm:"primaryKey" json:"id"`
	// Kind 申请对象类型（当前恒 "job"；留字段供 workflow 定义级审批扩展）。
	Kind string `json:"kind"`
	// Payload jobCreatePayload 的 JSON（批准后原样派发，不落库改语义）。
	Payload string `json:"-"`
	// Summary 命令摘要，列表视图免解 JSON 用（截断后存）。
	Summary string `json:"summary,omitempty"`
	// Target 执行目标 agent。
	Target string `gorm:"index" json:"target"`
	// Actor 提交人。
	Actor string `json:"actor"`
	// Status pending / approved / rejected / expired。
	Status string `gorm:"index" json:"status"`
	// ExpiresAt 待批到期时刻（过期后 DecideApproval 不再命中，见下）。
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	// DecidedBy 决策人（与提交人可不同；个人场景同人二次确认亦合法，审计区分）。
	DecidedBy string `json:"decidedBy,omitempty"`
	// DecidedAt 决策时刻。
	DecidedAt *time.Time `json:"decidedAt,omitempty"`
	// JobID 批准后派发出的 Job（对账用，空=未派发）。
	JobID string `gorm:"index;default:''" json:"jobId,omitempty"`
	// CreatedAt 提交时刻。
	CreatedAt time.Time `json:"createdAt"`
}

// CreateApproval 新建审批记录。
func (d *DB) CreateApproval(a *Approval) error {
	return d.db.Create(a).Error
}

// GetApproval 按 ID 取单条。
func (d *DB) GetApproval(id string) (*Approval, error) {
	var a Approval
	if err := d.db.First(&a, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// ListApprovals 审批台账（新在前）；status 空=不过滤。
func (d *DB) ListApprovals(status string, limit int) ([]Approval, error) {
	if limit <= 0 {
		limit = 50
	}
	q := d.db.Model(&Approval{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var list []Approval
	if err := q.Order("created_at desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DecideApproval 条件终态回写 approved/rejected：仅当仍 pending 且未到期时
// 命中（过期同条件内闭合，无需先查后改两步）。返回实际命中行数——并发重复
// 批准时只有第一笔命中，其余 0 行即 409。
func (d *DB) DecideApproval(id, status, by string, at time.Time) (int64, error) {
	res := d.db.Model(&Approval{}).
		Where("id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", id, ApprovalStatusPending, at).
		Updates(map[string]interface{}{
			"status":     status,
			"decided_by": by,
			"decided_at": at,
		})
	return res.RowsAffected, res.Error
}

// SetApprovalJobID 批准后回写派发出的 Job ID（仅终态行，幂等对账用）。
func (d *DB) SetApprovalJobID(id, jobID string) error {
	return d.db.Model(&Approval{}).Where("id = ?", id).
		Updates(map[string]interface{}{"job_id": jobID}).Error
}

// ExpireApprovals 批量把已过期的 pending 落 expired；已决行不受影响。
// 与 DecideApproval 双向收敛：谁先跑都安全（条件互斥在 status）。
func (d *DB) ExpireApprovals(now time.Time) (int64, error) {
	res := d.db.Model(&Approval{}).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at <= ?", ApprovalStatusPending, now).
		Updates(map[string]interface{}{"status": ApprovalStatusExpired})
	return res.RowsAffected, res.Error
}
