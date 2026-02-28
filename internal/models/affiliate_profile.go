package models

import (
	"time"

	"gorm.io/gorm"
)

// AffiliateProfile 推广返利用户档案
type AffiliateProfile struct {
	ID            uint           `gorm:"primarykey" json:"id"`                              // 主键
	UserID        uint           `gorm:"not null;uniqueIndex" json:"user_id"`               // 用户ID
	AffiliateCode string         `gorm:"type:varchar(32);not null;uniqueIndex" json:"code"` // 联盟短ID
	Status        string         `gorm:"type:varchar(20);not null;index" json:"status"`     // 状态
	CreatedAt     time.Time      `gorm:"index" json:"created_at"`                           // 创建时间
	UpdatedAt     time.Time      `gorm:"index" json:"updated_at"`                           // 更新时间
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`                                    // 软删除时间

	User User `gorm:"foreignKey:UserID" json:"user,omitempty"` // 用户信息
}

// TableName 指定表名
func (AffiliateProfile) TableName() string {
	return "affiliate_profiles"
}
