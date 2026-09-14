package storage

import (
	"gorm.io/gorm"
)

// ============ BackupConfig 备份配置 ============
//
// 备份执行在 Agent 侧本地（见 docs/guide/backup-design.md），此表是 server
// 控制面的调度与展示数据源。

// CreateBackupConfig 插入一条备份配置
func (d *DB) CreateBackupConfig(cfg *BackupConfig) error {
	return d.db.Create(cfg).Error
}

// UpdateBackupConfig 按主键更新配置全部字段
func (d *DB) UpdateBackupConfig(cfg *BackupConfig) error {
	return d.db.Save(cfg).Error
}

// GetBackupConfig 按主键取配置
func (d *DB) GetBackupConfig(id uint) (*BackupConfig, error) {
	var cfg BackupConfig
	if err := d.db.First(&cfg, id).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ListBackupConfigs 全部备份配置
func (d *DB) ListBackupConfigs() ([]*BackupConfig, error) {
	var list []*BackupConfig
	err := d.db.Order("id").Find(&list).Error
	return list, err
}

// DueBackupConfigs 到期待调度的启用配置
func (d *DB) DueBackupConfigs(now int64) ([]*BackupConfig, error) {
	var list []*BackupConfig
	err := d.db.Where("enabled = ? AND next_run_at > 0 AND next_run_at <= ? AND last_status <> ?",
		true, now, "running").Find(&list).Error
	return list, err
}

// DeleteBackupConfig 删除配置并级联删除运行历史
func (d *DB) DeleteBackupConfig(id uint) error {
	return d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("config_id = ?", id).Delete(&BackupRun{}).Error; err != nil {
			return err
		}
		return tx.Delete(&BackupConfig{}, id).Error
	})
}

// ============ BackupRun 运行历史 ============

// CreateBackupRun 插入一条 running 运行记录
func (d *DB) CreateBackupRun(run *BackupRun) error {
	return d.db.Create(run).Error
}

// UpdateBackupRun 回填运行记录终态
func (d *DB) UpdateBackupRun(run *BackupRun) error {
	return d.db.Save(run).Error
}

// GetBackupRun 按主键取运行记录
func (d *DB) GetBackupRun(id uint) (*BackupRun, error) {
	var run BackupRun
	if err := d.db.First(&run, id).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

// ListBackupRuns 运行历史，configID>0 时按配置过滤，倒序取最近 limit 条
func (d *DB) ListBackupRuns(configID uint, limit int) ([]*BackupRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := d.db.Order("id DESC").Limit(limit)
	if configID > 0 {
		q = q.Where("config_id = ?", configID)
	}
	var list []*BackupRun
	err := q.Find(&list).Error
	return list, err
}

// RunningBackupRuns 查询仍处于 running 的运行记录（server 重启后标记失败用）
func (d *DB) RunningBackupRuns() ([]*BackupRun, error) {
	var list []*BackupRun
	err := d.db.Where("status = ?", "running").Find(&list).Error
	return list, err
}
