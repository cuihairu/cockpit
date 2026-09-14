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
