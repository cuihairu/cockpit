package storage

import (
	"gorm.io/gorm/clause"
)

// ============ Stack 操作 ============
//
// Stack 表是 Compose Stack 的索引缓存，agent 上报即 upsert。

// UpsertStack 按 (agent_id, name) 插入或更新 stack 缓存
func (d *DB) UpsertStack(stack *Stack) error {
	return d.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "agent_id"}, {Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"running", "total", "last_action", "last_status", "last_deployed_at", "updated_at"}),
	}).Create(stack).Error
}

// ListStacks 获取全部 stack 缓存
func (d *DB) ListStacks() ([]*Stack, error) {
	var stacks []*Stack
	err := d.db.Order("agent_id, name").Find(&stacks).Error
	return stacks, err
}

// ListStacksByAgent 获取单个 agent 的 stack 缓存
func (d *DB) ListStacksByAgent(agentID string) ([]*Stack, error) {
	var stacks []*Stack
	err := d.db.Where("agent_id = ?", agentID).Order("name").Find(&stacks).Error
	return stacks, err
}

// DeleteStacksByAgent 删除某个 agent 的全部 stack 缓存（agent 被移除时调用）
func (d *DB) DeleteStacksByAgent(agentID string) error {
	return d.db.Where("agent_id = ?", agentID).Delete(&Stack{}).Error
}

// ============ StackDeployment 部署历史 ============

// CreateStackDeployment 插入一条 running 部署记录
func (d *DB) CreateStackDeployment(rec *StackDeployment) error {
	return d.db.Create(rec).Error
}

// FinishStackDeployment 按任务 ID 回填终态与结束时间
func (d *DB) FinishStackDeployment(taskID, status string, finishedAt int64) error {
	return d.db.Model(&StackDeployment{}).Where("task_id = ?", taskID).
		Updates(map[string]interface{}{"status": status, "finished_at": finishedAt}).Error
}

// ListStackDeployments 按 (agent, stack) 倒序取部署历史
func (d *DB) ListStackDeployments(agentID, stackName string, limit int) ([]*StackDeployment, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var list []*StackDeployment
	err := d.db.Where("agent_id = ? AND stack_name = ?", agentID, stackName).
		Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}

// DeleteStackDeploymentsByAgent 删除某个 agent 的全部部署历史
func (d *DB) DeleteStackDeploymentsByAgent(agentID string) error {
	return d.db.Where("agent_id = ?", agentID).Delete(&StackDeployment{}).Error
}
