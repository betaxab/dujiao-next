package models

import (
	"time"

	"gorm.io/gorm"
)

// User 用户表
type User struct {
	ID                 uint           `gorm:"primarykey" json:"id"`              // 主键
	Email              string         `gorm:"uniqueIndex;not null" json:"email"` // 邮箱
	PasswordHash       string         `gorm:"not null" json:"-"`                 // 密码哈希（不返回给前端）
	DisplayName        string         `gorm:"default:''" json:"display_name"`    // 昵称
	Locale             string         `gorm:"default:'zh-CN'" json:"locale"`     // 语言偏好
	Status             string         `gorm:"default:'active'" json:"status"`    // 账号状态
	TokenVersion       uint64         `gorm:"not null;default:0" json:"-"`       // Token 版本（用于全量失效）
	TokenInvalidBefore *time.Time     `gorm:"index" json:"-"`                    // 该时间点前签发的 Token 失效
	EmailVerifiedAt    *time.Time     `json:"email_verified_at"`                 // 邮箱验证时间
	LastLoginAt        *time.Time     `json:"last_login_at"`                     // 最后登录时间
	CreatedAt          time.Time      `gorm:"index" json:"created_at"`           // 创建时间
	UpdatedAt          time.Time      `gorm:"index" json:"updated_at"`           // 更新时间
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`                    // 软删除时间
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}
