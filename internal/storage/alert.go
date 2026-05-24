package storage

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Alert 警告/通知模型
type Alert struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	Type         string    `gorm:"index;not null" json:"type"` // info, warning, error, success
	Title        string    `gorm:"not null" json:"title"`
	Message      string    `gorm:"not null" json:"message"`
	ResourceID   *string   `json:"resource_id,omitempty"`
	ResourceType *string   `json:"resource_type,omitempty"`
	Read         bool      `gorm:"index;default:false" json:"read"`
	CreatedAt    time.Time `json:"created_at"`
}

// BeforeCreate GORM hook
func (a *Alert) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}

// CreateAlert 创建警告
func (d *DB) CreateAlert(alert *Alert) error {
	return d.db.Create(alert).Error
}

// GetAlert 获取单个警告
func (d *DB) GetAlert(id string) (*Alert, error) {
	var alert Alert
	err := d.db.First(&alert, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &alert, err
}

// ListAlerts 获取警告列表
func (d *DB) ListAlerts(limit int) ([]*Alert, error) {
	var alerts []*Alert
	query := d.db.Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&alerts).Error
	return alerts, err
}

// ListUnreadAlerts 获取未读警告
func (d *DB) ListUnreadAlerts() ([]*Alert, error) {
	var alerts []*Alert
	err := d.db.Where("read = ?", false).Order("created_at DESC").Find(&alerts).Error
	return alerts, err
}

// MarkAlertAsRead 标记警告为已读
func (d *DB) MarkAlertAsRead(id string) error {
	return d.db.Model(&Alert{}).
		Where("id = ?", id).
		Update("read", true).Error
}

// MarkAllAlertsAsRead 标记所有警告为已读
func (d *DB) MarkAllAlertsAsRead() error {
	return d.db.Model(&Alert{}).
		Where("read = ?", false).
		Update("read", true).Error
}

// DeleteAlert 删除警告
func (d *DB) DeleteAlert(id string) error {
	return d.db.Delete(&Alert{}, "id = ?", id).Error
}

// DeleteOldAlerts 删除旧警告
func (d *DB) DeleteOldAlerts(olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	return d.db.Where("created_at < ?", cutoff).Delete(&Alert{}).Error
}

// CreateSystemAlert 创建系统警告
func (d *DB) CreateSystemAlert(alertType, title, message string) error {
	alert := &Alert{
		Type:    alertType,
		Title:   title,
		Message: message,
		Read:    false,
	}
	return d.CreateAlert(alert)
}

// GetUnreadCount 获取未读警告数量
func (d *DB) GetUnreadCount() (int64, error) {
	var count int64
	err := d.db.Model(&Alert{}).Where("read = ?", false).Count(&count).Error
	return count, err
}
