package models

import (
	"time"

	"gorm.io/gorm"
)

// Fulfillment 交付记录表
type Fulfillment struct {
	ID            uint           `gorm:"primarykey" json:"id"`                 // 主键
	OrderID       uint           `gorm:"uniqueIndex;not null" json:"order_id"` // 订单ID
	Type          string         `gorm:"not null" json:"type"`                 // 交付类型（auto/manual）
	Status        string         `gorm:"not null" json:"status"`               // 交付状态（pending/delivered）
	Payload       string         `gorm:"type:text" json:"payload"`             // 交付内容
	LogisticsJSON JSON           `gorm:"type:json" json:"delivery_data"`       // 结构化交付信息
	DeliveredBy   *uint          `gorm:"index" json:"delivered_by,omitempty"`  // 交付管理员ID
	DeliveredAt   *time.Time     `gorm:"index" json:"delivered_at,omitempty"`  // 交付时间
	CreatedAt     time.Time      `gorm:"index" json:"created_at"`              // 创建时间
	UpdatedAt     time.Time      `gorm:"index" json:"updated_at"`              // 更新时间
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`                       // 软删除时间
}

// TableName 指定表名
func (Fulfillment) TableName() string {
	return "fulfillments"
}
