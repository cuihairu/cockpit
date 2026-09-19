package storage

import (
	"gorm.io/gorm/clause"
)

// Setting 通用键值设置表（服务端运行时配置，如拨测间隔）
type Setting struct {
	Key       string `gorm:"primarykey;size:128" json:"key"`
	Value     string `gorm:"size:4096" json:"value"`
	UpdatedAt int64  `json:"updatedAt"`
}

// GetSetting 读取设置值；不存在时返回空串与 gorm.ErrRecordNotFound
func (d *DB) GetSetting(key string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var s Setting
	err := d.db.Where("`key` = ?", key).First(&s).Error
	if err != nil {
		return "", err
	}
	return s.Value, nil
}

// SetSetting 写入设置（upsert）
func (d *DB) SetSetting(key, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&Setting{
		Key:   key,
		Value: value,
	}).Error
}

// DeleteSetting 删除设置；不存在时 gorm 删除 0 行同样返回 nil（幂等）。
// 注：gorm 的 Delete 不会返回 ErrRecordNotFound，无需特判。
func (d *DB) DeleteSetting(key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.db.Where("`key` = ?", key).Delete(&Setting{}).Error
}
