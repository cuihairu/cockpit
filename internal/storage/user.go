package storage

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User 用户表
type User struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"uniqueIndex;not null" json:"username"`
	Password  string    `gorm:"not null" json:"-"` // 永不返回密码
	Email     string    `json:"email"`
	Role      string    `gorm:"index;default:user" json:"role"` // admin, user
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate GORM hook
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return nil
}

// CreateUser 创建用户
func (d *DB) CreateUser(user *User) error {
	return d.db.Create(user).Error
}

// GetUserByUsername 根据用户名获取用户
func (d *DB) GetUserByUsername(username string) (*User, error) {
	var user User
	err := d.db.Where("username = ?", username).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &user, err
}

// GetUserByID 根据 ID 获取用户
func (d *DB) GetUserByID(id string) (*User, error) {
	var user User
	err := d.db.First(&user, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &user, err
}

// ListUsers 列出所有用户（不含密码）
func (d *DB) ListUsers() ([]User, error) {
	var users []User
	err := d.db.Select("id", "username", "email", "role", "created_at", "updated_at").
		Order("created_at DESC").
		Find(&users).Error
	return users, err
}

// UpdateUser 更新用户
func (d *DB) UpdateUser(user *User) error {
	return d.db.Model(&User{}).
		Where("id = ?", user.ID).
		Updates(map[string]interface{}{
			"email": user.Email,
			"role":  user.Role,
		}).Error
}

// UpdatePassword 更新密码
func (d *DB) UpdatePassword(userID, hashedPassword string) error {
	return d.db.Model(&User{}).
		Where("id = ?", userID).
		Update("password", hashedPassword).Error
}

// DeleteUser 删除用户
func (d *DB) DeleteUser(id string) error {
	return d.db.Delete(&User{}, "id = ?", id).Error
}

// VerifyPassword 验证用户密码
func (d *DB) VerifyPassword(username, password string) (*User, error) {
	user, err := d.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}

	if !verifyPassword(user.Password, password) {
		return nil, ErrNotFound
	}

	// 清除密码字段后返回
	user.Password = ""
	return user, nil
}

// InitAdminUser 初始化管理员用户
func (d *DB) InitAdminUser(username, password string) error {
	// 检查是否已存在
	_, err := d.GetUserByUsername(username)
	if err == nil {
		return nil // 已存在，无需创建
	}

	// 创建管理员
	hashedPassword, err := hashPassword(password)
	if err != nil {
		return err
	}

	admin := &User{
		Username: username,
		Password: hashedPassword,
		Role:     "admin",
	}

	return d.CreateUser(admin)
}
