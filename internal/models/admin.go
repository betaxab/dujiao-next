package models

import (
	"time"

	"gorm.io/gorm"
)

// Admin 管理员表
type Admin struct {
	ID                 uint           `gorm:"primarykey" json:"id"`                         // 主键
	Username           string         `gorm:"uniqueIndex;not null" json:"username"`         // 管理员账号
	PasswordHash       string         `gorm:"not null" json:"-"`                            // 密码哈希（不返回给前端）
	TokenVersion       uint64         `gorm:"not null;default:0" json:"-"`                  // Token 版本（用于全量失效）
	TokenInvalidBefore *time.Time     `gorm:"index" json:"-"`                               // 该时间点前签发的 Token 失效
	IsSuper            bool           `gorm:"not null;default:false;index" json:"is_super"` // 是否超级管理员（免权限校验）
	LastLoginAt        *time.Time     `json:"last_login_at"`                                // 最后登录时间
	CreatedAt          time.Time      `gorm:"index" json:"created_at"`                      // 创建时间
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`                               // 软删除时间
}

// TableName 指定表名
func (Admin) TableName() string {
	return "admins"
}
