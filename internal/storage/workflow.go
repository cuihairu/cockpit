package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Workflow 与异步 Job 模型（设计见 docs/guide/workflow-design.md）。
//
// Workflow 是「一组 Job 的编排」：定义（Workflow）持久化一条有序步骤链，
// 每次运行（WorkflowRun）逐步建真实 Job 进统一台账。run 状态机：
//
//	running → success | failed | cancelled
//
// 步骤运行态以 JSON 快照存 run.Steps，历史 run 在定义删除后仍可读。
const (
	WorkflowRunStatusRunning   = "running"
	WorkflowRunStatusSuccess   = "success"
	WorkflowRunStatusFailed    = "failed"
	WorkflowRunStatusCancelled = "cancelled"

	// WorkflowMaxSteps 单个 workflow 步骤上限
	WorkflowMaxSteps = 20
)

// Workflow 编排定义
type Workflow struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Steps       string    `json:"-"` // JSON 串步骤数组（API 层解析回对象）
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// WorkflowRun 一次运行实例（WorkflowName 冗余，定义删除后台账仍可读）
type WorkflowRun struct {
	ID           string     `gorm:"primaryKey" json:"id"`
	WorkflowID   string     `gorm:"index" json:"workflowId"`
	WorkflowName string     `json:"workflowName"`
	Status       string     `gorm:"index" json:"status"`
	Actor        string     `json:"actor"`
	Steps        string     `json:"-"` // 运行态快照 JSON
	CreatedAt    time.Time  `json:"createdAt"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
}

// CreateWorkflow 新建定义
func (d *DB) CreateWorkflow(wf *Workflow) error {
	return d.db.Create(wf).Error
}

// GetWorkflow 按 ID 取定义
func (d *DB) GetWorkflow(id string) (*Workflow, error) {
	var wf Workflow
	if err := d.db.First(&wf, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &wf, nil
}

// ListWorkflows 定义列表（新在前）
func (d *DB) ListWorkflows() ([]Workflow, error) {
	var wfs []Workflow
	if err := d.db.Order("created_at desc").Find(&wfs).Error; err != nil {
		return nil, err
	}
	return wfs, nil
}

// UpdateWorkflow 回写定义（name/description/steps/updated_at）
func (d *DB) UpdateWorkflow(wf *Workflow) error {
	return d.db.Model(wf).Save(wf).Error
}

// DeleteWorkflow 删除定义（历史 run 保留，不级联）
func (d *DB) DeleteWorkflow(id string) error {
	return d.db.Delete(&Workflow{}, "id = ?", id).Error
}

// CreateWorkflowRun 新建运行实例
func (d *DB) CreateWorkflowRun(run *WorkflowRun) error {
	return d.db.Create(run).Error
}

// GetWorkflowRun 按 ID 取运行实例
func (d *DB) GetWorkflowRun(id string) (*WorkflowRun, error) {
	var run WorkflowRun
	if err := d.db.First(&run, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

// ListWorkflowRuns 某定义的运行台账（新在前）
func (d *DB) ListWorkflowRuns(workflowID string) ([]WorkflowRun, error) {
	var runs []WorkflowRun
	if err := d.db.Where("workflow_id = ?", workflowID).
		Order("created_at desc").Find(&runs).Error; err != nil {
		return nil, err
	}
	return runs, nil
}

// UpdateWorkflowRun 回写运行态（status/steps/finished_at）
func (d *DB) UpdateWorkflowRun(run *WorkflowRun) error {
	return d.db.Model(run).Save(run).Error
}

// ActiveWorkflowRun 返回该定义进行中的 run（无则 nil，不视为错误）
func (d *DB) ActiveWorkflowRun(workflowID string) (*WorkflowRun, error) {
	var run WorkflowRun
	err := d.db.Where("workflow_id = ? AND status = ?", workflowID, WorkflowRunStatusRunning).
		First(&run).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

// ListJobsFiltered 带过滤的 Job 台账（状态/目标/类型/所属 run；空值不过滤）
func (d *DB) ListJobsFiltered(status, target, jobType, runID string, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	q := d.db.Model(&Job{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if target != "" {
		q = q.Where("target = ?", target)
	}
	if jobType != "" {
		q = q.Where("type = ?", jobType)
	}
	if runID != "" {
		q = q.Where("workflow_run_id = ?", runID)
	}
	var jobs []Job
	if err := q.Order("created_at desc").Limit(limit).Find(&jobs).Error; err != nil {
		return nil, err
	}
	return jobs, nil
}
